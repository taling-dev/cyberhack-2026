"""SimaOps AI Worker — NATS JetStream consumer with switchable QC strategy.

Reads `qc.job.created` events (in standardized envelope format), runs AI
inference, persists results to TiDB, and publishes lifecycle events back to
NATS so the SSE bridge can fan them out to subscribed web clients.

Events published by this worker (all envelope-formatted, subject == event_type):
  qc.job.completed             — AI inference finished cleanly
  qc.job.needs_human_review    — recommendation is REVIEW (PASS/FAIL auto-decided)
  qc.job.failed                — exception during processing (after final retry)
  lot.status_changed           — lot transitioned to QC_APPROVED / QC_REJECTED (auto) or QC_REVIEW
"""

import asyncio
import datetime as _dt
import json
import os
import time
import uuid
import signal
from abc import ABC, abstractmethod
from contextlib import asynccontextmanager

import nats
import nats.errors
import pymysql
from dbutils.pooled_db import PooledDB
import uvicorn
from fastapi import FastAPI, Response
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from opentelemetry import trace, context as otel_context
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.propagators.textmap import DefaultGetter
from opentelemetry.propagate import extract, inject

# ─── OTel Init ────────────────────────────────────────────────────

resource = Resource.create({"service.name": "simaops-ai-worker"})
provider = TracerProvider(resource=resource)
otlp_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=otlp_endpoint, insecure=True)))
trace.set_tracer_provider(provider)
tracer = trace.get_tracer("simaops-ai-worker")

# ─── Config ──────────────────────────────────────────────────────

NATS_URL = os.getenv("NATS_URL", "nats://localhost:4222")
TIDB_HOST = os.getenv("TIDB_HOST", "localhost")
TIDB_PORT = int(os.getenv("TIDB_PORT", "4000"))
TIDB_USER = os.getenv("TIDB_USER", "root")
TIDB_PASSWORD = os.getenv("TIDB_PASSWORD", "")
TIDB_DB = os.getenv("TIDB_DB", "simaops")
QC_STRATEGY = os.getenv("QC_STRATEGY", "mock")
MODEL_VERSION = os.getenv("MODEL_VERSION", "mock-v0.1.0")

# ─── Prometheus Metrics ──────────────────────────────────────────

jobs_total = Counter(
    "simaops_ai_worker_jobs_total",
    "Total QC jobs processed by the AI worker",
    ["status", "recommendation"],
)

inference_duration = Histogram(
    "simaops_ai_worker_inference_duration_seconds",
    "Time spent running AI inference per job",
    ["material_type"],
    buckets=(0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0),
)

job_failures = Counter(
    "simaops_ai_worker_job_failures_total",
    "Total job processing failures",
    ["reason"],
)

# ─── Strategy Interface ──────────────────────────────────────────


class QCStrategy(ABC):
    @abstractmethod
    async def analyze(self, image_key: str, material_type: str) -> dict:
        """Returns {recommendation, confidence, findings}."""
        ...


class MockStrategy(QCStrategy):
    async def analyze(self, image_key: str, material_type: str) -> dict:
        # Simulate brief processing latency
        await asyncio.sleep(0.5)
        findings = [
            {"class_name": "bottle", "mapped_finding": "foreign_matter", "confidence": 0.87, "is_anomaly": True},
            {"class_name": "banana", "mapped_finding": "ripeness_signal", "confidence": 0.92, "is_anomaly": False},
        ]
        return {"recommendation": "REVIEW", "confidence": 0.82, "findings": findings}


# ─── Classifier strategy (trained YOLOv8-cls + fuzzy grader) ─────
#
# Real QC: download the lot's QC image from MinIO, run the trained fruit
# disease classifier, then grade with a fuzzy controller that combines the
# healthy-confidence and defect-confidence signals into a 0-100 quality score,
# mapped to the app's PASS / REVIEW / FAIL recommendation.

MODEL_PATH = os.getenv("MODEL_PATH", "models/best.pt")
# Defect keywords match the trained dataset's class names
# (Apple___Black_rot, Banana cordana, Citrus Fruit disease, Guava Red Rust,
#  Mango Bacterial Canker, Papaya RingSpot, "Disease … Fruit", etc.).
_DEFECT_KEYWORDS = ("disease", "rot", "rust", "canker", "cordana", "ringspot")
_HEALTHY_KEYWORDS = ("healthy", "healthly")  # dataset has a "Healthly" typo class


class ClassifierStrategy(QCStrategy):
    """YOLOv8-cls fruit-disease classifier + scikit-fuzzy quality grader.

    Heavy deps (ultralytics, skfuzzy, opencv, minio) are imported lazily so the
    worker process still boots — and /healthz still serves — even if the
    inference image layer is somehow incomplete. The model is loaded once on
    first use and cached.
    """

    def __init__(self):
        self._model = None
        self._fuzzy = None
        self._minio = None

    def _get_minio(self):
        if self._minio is None:
            from minio import Minio
            endpoint = os.getenv("MINIO_ENDPOINT", "localhost:9000")
            self._minio = Minio(
                endpoint,
                access_key=os.getenv("MINIO_ACCESS_KEY", "simaops"),
                secret_key=os.getenv("MINIO_SECRET_KEY", "simaops-dev-secret"),
                secure=os.getenv("MINIO_USE_SSL", "false") == "true",
            )
        return self._minio

    def _get_model(self):
        if self._model is None:
            from ultralytics import YOLO
            self._model = YOLO(MODEL_PATH)
        return self._model

    def _get_fuzzy(self):
        if self._fuzzy is None:
            self._fuzzy = _build_fuzzy_system()
        return self._fuzzy

    def _download(self, image_key: str) -> str:
        """Download the QC image from MinIO to a tmp path; returns the path."""
        import tempfile
        bucket = os.getenv("QC_IMAGES_BUCKET", "simaops-qc-images")
        ext = os.path.splitext(image_key)[1] or ".jpg"
        fd, path = tempfile.mkstemp(suffix=ext)
        os.close(fd)
        self._get_minio().fget_object(bucket, image_key, path)
        return path

    async def analyze(self, image_key: str, material_type: str) -> dict:
        # Run the blocking download + CPU inference off the event loop.
        return await asyncio.to_thread(self._analyze_sync, image_key, material_type)

    def _analyze_sync(self, image_key: str, material_type: str) -> dict:
        path = self._download(image_key)
        try:
            results = self._get_model()(path, verbose=False)[0]
            names = results.names
            probs = results.probs.data.tolist()
        finally:
            try:
                os.remove(path)
            except OSError:
                pass

        healthy_score = 0.0
        defect_score = 0.0
        findings = []
        for i, score in enumerate(probs):
            name = names[i]
            low = name.lower()
            is_defect = any(k in low for k in _DEFECT_KEYWORDS)
            is_healthy = (not is_defect) and any(k in low for k in _HEALTHY_KEYWORDS)
            # Sum (not max) the softmax mass across all healthy / all defect
            # classes. The model has duplicate classes per fruit (e.g.
            # "Apple___healthy" AND "Healthy Apple Fruit") from two merged
            # datasets, so a confident prediction is SPLIT across them. Taking
            # the max would discard half the signal and under-report confidence;
            # summing recovers the true "probability this fruit is healthy".
            if is_healthy:
                healthy_score += score
            elif is_defect:
                defect_score += score
            # Only surface the meaningful detections (top class + any notable signal).
            if score >= 0.10:
                findings.append({
                    "class_name": name,
                    "mapped_finding": "defect" if is_defect else ("healthy" if is_healthy else "other"),
                    "confidence": round(float(score), 4),
                    "is_anomaly": bool(is_defect),
                })
        # Probabilities are a partition of 1.0, so the summed group scores are
        # already bounded by 1.0; clamp defensively against float drift.
        healthy_score = min(healthy_score, 1.0)
        defect_score = min(defect_score, 1.0)
        findings.sort(key=lambda f: f["confidence"], reverse=True)
        findings = findings[:5]

        quality = _grade(self._get_fuzzy(), healthy_score, defect_score)
        # Map the 0-100 fuzzy quality to the app's PASS/REVIEW/FAIL contract,
        # using admin-configurable thresholds (app_settings, defaults 75/40).
        pass_min, review_min = get_qc_thresholds()
        if quality >= pass_min:
            recommendation = "PASS"
        elif quality >= review_min:
            recommendation = "REVIEW"
        else:
            recommendation = "FAIL"
        # Headline confidence = the dominant aggregated signal.
        confidence = max(healthy_score, defect_score) if (healthy_score or defect_score) else 0.0
        return {
            "recommendation": recommendation,
            "confidence": round(float(confidence), 4),
            "findings": findings,
        }


def _build_fuzzy_system():
    """Build the fuzzy quality controller (ported from ai-training)."""
    import numpy as np
    import skfuzzy as fuzz
    from skfuzzy import control as ctrl

    healthy = ctrl.Antecedent(np.arange(0, 1.01, 0.01), "healthy")
    defect = ctrl.Antecedent(np.arange(0, 1.01, 0.01), "defect")
    quality = ctrl.Consequent(np.arange(0, 101, 1), "quality")

    healthy["low"] = fuzz.trimf(healthy.universe, [0, 0, 0.5])
    healthy["medium"] = fuzz.trimf(healthy.universe, [0.3, 0.6, 0.8])
    healthy["high"] = fuzz.trimf(healthy.universe, [0.6, 1, 1])
    defect["none"] = fuzz.trimf(defect.universe, [0, 0, 0.3])
    defect["minor"] = fuzz.trimf(defect.universe, [0.2, 0.5, 0.7])
    defect["major"] = fuzz.trimf(defect.universe, [0.5, 1, 1])
    quality["reject"] = fuzz.trapmf(quality.universe, [0, 0, 20, 40])
    quality["standard"] = fuzz.trimf(quality.universe, [30, 60, 80])
    quality["premium"] = fuzz.trapmf(quality.universe, [70, 90, 100, 100])

    rules = [
        ctrl.Rule(healthy["high"] & defect["none"], quality["premium"]),
        ctrl.Rule(healthy["medium"] & defect["minor"], quality["standard"]),
        ctrl.Rule(defect["major"], quality["reject"]),
        ctrl.Rule(healthy["low"], quality["reject"]),
    ]
    return ctrl.ControlSystem(rules)


def _grade(control_system, healthy_score: float, defect_score: float) -> float:
    from skfuzzy import control as ctrl
    sim = ctrl.ControlSystemSimulation(control_system)
    sim.input["healthy"] = float(healthy_score)
    sim.input["defect"] = float(defect_score)
    try:
        sim.compute()
        return float(sim.output["quality"])
    except Exception:
        # No rule fired (e.g. ambiguous mid-range signals) — treat as REVIEW.
        return 50.0


def get_strategy() -> QCStrategy:
    if QC_STRATEGY == "mock":
        return MockStrategy()
    if QC_STRATEGY == "classifier":
        return ClassifierStrategy()
    # Unknown values fail loudly rather than silently falling back to mock —
    # otherwise a misconfigured env var would mask real-strategy bugs.
    raise ValueError(
        f"Unknown QC_STRATEGY={QC_STRATEGY!r}. Supported values: 'mock', 'classifier'"
    )


# ─── Database ─────────────────────────────────────────────────────


# Pooled DB — created once at module load, shared across all handlers.
# mincached=2 keeps idle connections; maxconnections=20 caps total concurrent connections.
#
# autocommit=False here because write_qc_result performs three related writes
# (insert qc_results, update qc_jobs, update lots) that MUST be atomic — a
# crash between them would leave the system in an inconsistent state with no
# automatic recovery (the NATS message is acked only after commit). Per-call
# auto-commit can be re-enabled inside individual functions that don't need
# transactional semantics.
_db_pool = PooledDB(
    creator=pymysql,
    maxconnections=20,
    mincached=2,
    maxcached=5,
    blocking=True,
    host=TIDB_HOST,
    port=TIDB_PORT,
    user=TIDB_USER,
    password=TIDB_PASSWORD,
    database=TIDB_DB,
    autocommit=False,
)


def get_db():
    return _db_pool.connection()


def get_qc_thresholds() -> tuple[int, int]:
    """Read configurable (pass_min, review_min) from app_settings; fall back to
    the defaults (75/40) if unset or on any error. Read per job so admin changes
    take effect without a worker restart."""
    pass_min, review_min = 75, 40
    try:
        db = get_db()
        try:
            with db.cursor() as cur:
                cur.execute(
                    "SELECT setting_key, setting_value FROM app_settings "
                    "WHERE setting_key IN ('qc_pass_min','qc_review_min')"
                )
                for k, v in cur.fetchall():
                    if k == 'qc_pass_min':
                        pass_min = int(v)
                    elif k == 'qc_review_min':
                        review_min = int(v)
        finally:
            db.close()
    except Exception:
        pass
    return pass_min, review_min


def mark_job_processing(job_id: str):
    db = get_db()
    try:
        with db.cursor() as cur:
            cur.execute(
                "UPDATE qc_jobs SET status='PROCESSING', started_at=NOW() WHERE id=%s AND status='QUEUED'",
                (job_id,),
            )
        db.commit()
    except Exception:
        db.rollback()
        raise
    finally:
        db.close()


def write_qc_result(job_id: str, lot_id: str, result: dict):
    """Atomically write the QC result and advance the lot to QC_REVIEW.

    Wraps the three statements in a single transaction so either ALL writes
    succeed or NONE do. The caller (message_handler) only acks the NATS
    message after this returns successfully, so a crash mid-write triggers
    NATS redelivery (and the `ON DUPLICATE KEY UPDATE` clauses make the retry
    idempotent).
    """
    db = get_db()
    # Auto-decision: PASS -> auto-approve, FAIL -> auto-reject, REVIEW -> human review.
    rec = result["recommendation"]
    if rec == "PASS":
        _job_status, _lot_status, _auto_decision = "APPROVED", "QC_APPROVED", "APPROVED"
    elif rec == "FAIL":
        _job_status, _lot_status, _auto_decision = "REJECTED", "QC_REJECTED", "REJECTED"
    else:
        _job_status, _lot_status, _auto_decision = "AI_COMPLETED", "QC_REVIEW", None
    try:
        with db.cursor() as cur:
            cur.execute(
                """INSERT INTO qc_results (id, qc_job_id, lot_id, recommendation, confidence, findings_json, model_version, supervisor_decision, reviewed_by, reviewed_at)
                   VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, NOW())
                   ON DUPLICATE KEY UPDATE
                       recommendation=VALUES(recommendation),
                       confidence=VALUES(confidence),
                       findings_json=VALUES(findings_json),
                       model_version=VALUES(model_version),
                       supervisor_decision=VALUES(supervisor_decision),
                       reviewed_by=VALUES(reviewed_by),
                       reviewed_at=VALUES(reviewed_at)""",
                (str(uuid.uuid4()), job_id, lot_id, rec,
                 f"{result['confidence']:.4f}", json.dumps(result["findings"]), MODEL_VERSION,
                 _auto_decision, "ai-auto" if _auto_decision else None),
            )
            cur.execute("UPDATE qc_jobs SET status=%s, completed_at=NOW() WHERE id=%s", (_job_status, job_id))
            cur.execute("UPDATE lots SET status=%s WHERE id=%s", (_lot_status, lot_id))
        db.commit()
    except Exception:
        db.rollback()
        raise
    finally:
        db.close()


def mark_job_failed(job_id: str, reason: str):
    db = get_db()
    try:
        with db.cursor() as cur:
            cur.execute(
                "UPDATE qc_jobs SET status='FAILED', failure_reason=%s, completed_at=NOW() WHERE id=%s",
                (reason[:500], job_id),
            )
        db.commit()
    except Exception:
        db.rollback()
        raise
    finally:
        db.close()


def fetch_lot_number(lot_id: str) -> str:
    """Best-effort fetch of lot.lot_number for inclusion in published events.
    Returns empty string on failure (event still publishes; UI just lacks the
    pretty number)."""
    if not lot_id:
        return ""
    db = get_db()
    try:
        with db.cursor() as cur:
            cur.execute("SELECT lot_number FROM lots WHERE id=%s", (lot_id,))
            row = cur.fetchone()
            return row[0] if row else ""
    except Exception:
        return ""
    finally:
        db.close()


# ─── NATS Consumer ────────────────────────────────────────────────

strategy = get_strategy()
shutdown_event = asyncio.Event()

# NATS connection handle, set in run_consumer() and used by publish_envelope().
_nc = None  # type: ignore[assignment]


def build_envelope(event_type: str, actor_id: str, owner_user_id: str,
                   resource_id: str, payload: dict) -> bytes:
    """Build a SimaOps event envelope as JSON bytes ready for NATS publish.

    Schema must match apps/api/internal/events/envelope.go exactly.
    """
    env = {
        "event_id": str(uuid.uuid4()),
        "event_type": event_type,
        "occurred_at": _dt.datetime.now(_dt.timezone.utc).isoformat().replace("+00:00", "Z"),
        "actor_id": actor_id,
        "owner_user_id": owner_user_id,
        "resource_id": resource_id,
        "payload": payload,
    }
    return json.dumps(env).encode("utf-8")


async def publish_envelope(subject: str, data: bytes):
    """Publish a pre-serialized envelope to NATS with current trace context."""
    if _nc is None:
        print(f"[worker] cannot publish {subject}: no NATS connection", flush=True)
        return
    headers: dict[str, str] = {}
    inject(headers)
    try:
        await _nc.publish(subject, data, headers=headers)
    except Exception as e:
        print(f"[worker] publish {subject} failed: {e}", flush=True)


async def message_handler(msg):
    """Callback for each NATS message — processes one QC job."""
    job_id = "?"
    lot_id = "?"
    owner_user_id = ""
    actor_id = "ai-worker"
    lot_number = ""
    try:
        # Extract trace context from NATS headers (traceparent set by outbox-publisher)
        carrier = {}
        if msg.headers:
            for k, v in msg.headers.items():
                carrier[k.lower()] = v
        parent_ctx = extract(carrier)

        # Parse envelope. The actual job data lives in envelope.payload.
        envelope = json.loads(msg.data)
        owner_user_id = envelope.get("owner_user_id", "")
        actor_id = envelope.get("actor_id", "ai-worker")
        payload = envelope.get("payload", {})
        # The API stores payload as a stringified JSON RawMessage in some cases —
        # accept both shapes for robustness across releases.
        if isinstance(payload, str):
            payload = json.loads(payload)

        job_id = payload["qc_job_id"]
        lot_id = payload["lot_id"]
        image_key = payload["image_object_key"]
        material_type = payload.get("material_type", "RAW_BOTANICAL")
        # owner_user_id may also be inside the payload (CreateQCJob mirrors it
        # there for downstream consumers). Prefer the envelope-level value but
        # fall back to the payload if missing.
        if not owner_user_id:
            owner_user_id = payload.get("owner_user_id", "")

        with tracer.start_as_current_span(
            "qc.process",
            context=parent_ctx,
            attributes={
                "qc.job_id": job_id,
                "qc.lot_id": lot_id,
                "qc.material_type": material_type,
            },
        ):
            print(f"[worker] processing job={job_id} image={image_key} material={material_type}", flush=True)

            # Mark job as PROCESSING (only succeeds if still QUEUED — prevents re-processing)
            mark_job_processing(job_id)

            t0 = time.monotonic()
            result = await strategy.analyze(image_key, material_type)
            inference_duration.labels(material_type=material_type).observe(time.monotonic() - t0)
            write_qc_result(job_id, lot_id, result)
            lot_number = fetch_lot_number(lot_id)

            jobs_total.labels(status="completed", recommendation=result["recommendation"]).inc()
            await msg.ack()
            print(f"[worker] completed job={job_id} recommendation={result['recommendation']}", flush=True)

            # Fan out lifecycle events to NATS so the SSE bridge can update web UI.
            recommendation = result["recommendation"]
            base_payload = {
                "qc_job_id":      job_id,
                "lot_id":         lot_id,
                "lot_number":     lot_number,
                "lot_created_by": owner_user_id,
                "recommendation": recommendation,
                "confidence":     result["confidence"],
                "model_version":  MODEL_VERSION,
            }
            await publish_envelope(
                "qc.job.completed",
                build_envelope("qc.job.completed", actor_id, owner_user_id, job_id, base_payload),
            )
            _lot_to = "QC_APPROVED" if recommendation == "PASS" else "QC_REJECTED" if recommendation == "FAIL" else "QC_REVIEW"
            await publish_envelope(
                "lot.status_changed",
                build_envelope(
                    "lot.status_changed", actor_id, owner_user_id, lot_id,
                    {
                        "lot_id":     lot_id,
                        "lot_number": lot_number,
                        "from":       "AI_PROCESSING",
                        "to":         _lot_to,
                        "reason":     "ai-auto-decision" if recommendation in ("PASS", "FAIL") else "ai-completed",
                        "created_by": owner_user_id,
                        "actor_id":   actor_id,
                    },
                ),
            )
            if recommendation == "REVIEW":
                await publish_envelope(
                    "qc.job.needs_human_review",
                    build_envelope(
                        "qc.job.needs_human_review", actor_id, owner_user_id, job_id,
                        base_payload,
                    ),
                )

    except Exception as e:
        print(f"[worker] error processing job={job_id}: {e}", flush=True)
        job_failures.labels(reason=type(e).__name__).inc()
        jobs_total.labels(status="failed", recommendation="none").inc()
        try:
            if job_id != "?":
                mark_job_failed(job_id, str(e))
        except Exception as inner:
            print(f"[worker] failed to mark job FAILED: {inner}", flush=True)
        # Best-effort failure event so the web UI shows the FAILED status without polling.
        if job_id != "?":
            try:
                await publish_envelope(
                    "qc.job.failed",
                    build_envelope(
                        "qc.job.failed", actor_id, owner_user_id, job_id,
                        {
                            "qc_job_id":      job_id,
                            "lot_id":         lot_id,
                            "lot_number":     lot_number,
                            "lot_created_by": owner_user_id,
                            "reason":         str(e)[:500],
                        },
                    ),
                )
            except Exception:
                pass
        try:
            await msg.nak(delay=10)
        except Exception:
            pass


async def run_consumer():
    """Connect to NATS and pull QC jobs in a fetch loop.

    Uses a JetStream PULL consumer (not push). Pull mode supports multiple
    competing subscribers on the same durable name — JetStream load-balances
    pending messages across whichever pods happen to be calling fetch().
    This is the unlock for horizontal scaling: with N replicas, all N can
    simultaneously consume from the same durable and JetStream guarantees
    each message is delivered to exactly one of them at a time (with
    redelivery if the holder doesn't ack within ack_wait).

    Concurrency model:
      * Outer loop: fetch up to FETCH_BATCH messages with FETCH_TIMEOUT.
      * Inner: process the batch concurrently with asyncio.gather, capped
        at MAX_INFLIGHT via a semaphore so we never overwhelm the DB pool
        (PooledDB maxconnections=20).
      * Same message_handler is reused unchanged — it already handles ack /
        nak correctly, idempotent DB writes, and lifecycle event emission.

    Recovery semantics:
      * max_deliver=4 — same as before; after 4 failed deliveries NATS drops
        the message (it's not redelivered further).
      * ack_wait=120s — same.
      * On NATS disconnect, the outer fetch raises; we sleep 5s and reconnect.
    """
    global _nc

    # Tuneables. Values chosen so a single pod processes ≤ FETCH_BATCH
    # messages at a time without saturating the DB pool, and the outer
    # fetch returns promptly on idle so shutdown is responsive.
    FETCH_BATCH = 8
    FETCH_TIMEOUT = 5.0  # seconds — short so shutdown_event is checked often
    MAX_INFLIGHT = 4     # parallel handlers; well under DB pool max=20

    while not shutdown_event.is_set():
        try:
            nc = await nats.connect(NATS_URL, name="simaops-ai-worker", reconnect_time_wait=2)
            _nc = nc
            js = nc.jetstream()

            from nats.js.api import ConsumerConfig
            # pull_subscribe creates the consumer if it doesn't exist (using
            # `config`), or attaches to the existing one (config ignored).
            # We deliberately do NOT pass `cb=` — that's the push-mode flag.
            sub = await js.pull_subscribe(
                "qc.job.created",
                durable="simaops-ai-worker",
                stream="SIMAOPS",
                config=ConsumerConfig(
                    max_deliver=4,
                    ack_wait=120,
                ),
            )
            print(
                f"[worker] subscribed (mode=pull, strategy={QC_STRATEGY}, model={MODEL_VERSION}, "
                f"max_deliver=4, ack_wait=120s, batch={FETCH_BATCH}, in_flight={MAX_INFLIGHT})",
                flush=True,
            )

            sem = asyncio.Semaphore(MAX_INFLIGHT)

            async def _handle_one(m):
                async with sem:
                    await message_handler(m)

            while not shutdown_event.is_set():
                try:
                    msgs = await sub.fetch(batch=FETCH_BATCH, timeout=FETCH_TIMEOUT)
                except (asyncio.TimeoutError, nats.errors.TimeoutError):
                    # Idle period — no messages within FETCH_TIMEOUT. Loop back
                    # so we promptly notice shutdown_event without waiting.
                    continue
                except Exception as fetch_err:
                    # Includes nats.errors.ConnectionClosedError and the
                    # nats-py specific errors that surface during a JetStream
                    # blip. Log + reconnect via outer loop.
                    print(f"[worker] fetch error: {fetch_err} — reconnecting", flush=True)
                    raise

                if not msgs:
                    continue

                # Dispatch the batch concurrently. asyncio.gather waits for all
                # handlers (each of which acks/naks itself) before pulling the
                # next batch. This keeps in-flight count bounded by FETCH_BATCH
                # × MAX_INFLIGHT (since handlers within a batch share the sem).
                await asyncio.gather(
                    *(_handle_one(m) for m in msgs),
                    return_exceptions=True,
                )

            # Graceful shutdown — drain rather than unsubscribe so any handler
            # currently inside the semaphore has a chance to finish + ack.
            await nc.drain()
            _nc = None
            print("[worker] consumer shut down cleanly", flush=True)
            return

        except Exception as e:
            print(f"[worker] consumer error: {e} — reconnecting in 5s", flush=True)
            _nc = None
            await asyncio.sleep(5)


# ─── FastAPI Health Endpoints ─────────────────────────────────────


@asynccontextmanager
async def lifespan(app: FastAPI):
    task = asyncio.create_task(run_consumer())
    yield
    shutdown_event.set()
    try:
        await asyncio.wait_for(task, timeout=5)
    except asyncio.TimeoutError:
        task.cancel()


app = FastAPI(title="SimaOps AI Worker", lifespan=lifespan)


@app.get("/healthz")
def healthz():
    return {"status": "ok", "strategy": QC_STRATEGY, "model": MODEL_VERSION}


@app.get("/readyz")
def readyz():
    try:
        db = get_db()
        db.close()
        return {"status": "ready"}
    except Exception as e:
        return {"status": "not_ready", "error": str(e)}


@app.get("/metrics")
def metrics():
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8081)
