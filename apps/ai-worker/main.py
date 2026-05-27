"""SimaOps AI Worker — NATS JetStream consumer with switchable QC strategy."""

import asyncio
import json
import os
import time
import uuid
import signal
from abc import ABC, abstractmethod
from contextlib import asynccontextmanager

import nats
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
from opentelemetry.propagate import extract

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


def get_strategy() -> QCStrategy:
    if QC_STRATEGY == "mock":
        return MockStrategy()
    return MockStrategy()


# ─── Database ─────────────────────────────────────────────────────


# Pooled DB — created once at module load, shared across all handlers.
# mincached=2 keeps idle connections; maxconnections=20 caps total concurrent connections.
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
    autocommit=True,
)


def get_db():
    return _db_pool.connection()


def mark_job_processing(job_id: str):
    db = get_db()
    try:
        with db.cursor() as cur:
            cur.execute(
                "UPDATE qc_jobs SET status='PROCESSING', started_at=NOW() WHERE id=%s AND status='QUEUED'",
                (job_id,),
            )
    finally:
        db.close()


def write_qc_result(job_id: str, lot_id: str, result: dict):
    db = get_db()
    try:
        with db.cursor() as cur:
            cur.execute(
                """INSERT INTO qc_results (id, qc_job_id, lot_id, recommendation, confidence, findings_json, model_version)
                   VALUES (%s, %s, %s, %s, %s, %s, %s)
                   ON DUPLICATE KEY UPDATE
                       recommendation=VALUES(recommendation),
                       confidence=VALUES(confidence),
                       findings_json=VALUES(findings_json),
                       model_version=VALUES(model_version)""",
                (str(uuid.uuid4()), job_id, lot_id, result["recommendation"],
                 f"{result['confidence']:.4f}", json.dumps(result["findings"]), MODEL_VERSION),
            )
            cur.execute("UPDATE qc_jobs SET status='AI_COMPLETED', completed_at=NOW() WHERE id=%s", (job_id,))
            cur.execute("UPDATE lots SET status='QC_REVIEW' WHERE id=%s", (lot_id,))
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
    finally:
        db.close()


# ─── NATS Consumer ────────────────────────────────────────────────

strategy = get_strategy()
shutdown_event = asyncio.Event()


async def message_handler(msg):
    """Callback for each NATS message — processes one QC job."""
    job_id = "?"
    try:
        # Extract trace context from NATS headers (traceparent set by outbox-publisher)
        carrier = {}
        if msg.headers:
            for k, v in msg.headers.items():
                carrier[k.lower()] = v
        parent_ctx = extract(carrier)

        payload = json.loads(msg.data)
        job_id = payload["qc_job_id"]
        lot_id = payload["lot_id"]
        image_key = payload["image_object_key"]
        material_type = payload.get("material_type", "RAW_BOTANICAL")

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

            jobs_total.labels(status="completed", recommendation=result["recommendation"]).inc()
            await msg.ack()
            print(f"[worker] completed job={job_id} recommendation={result['recommendation']}", flush=True)

    except Exception as e:
        print(f"[worker] error processing job={job_id}: {e}", flush=True)
        job_failures.labels(reason=type(e).__name__).inc()
        jobs_total.labels(status="failed", recommendation="none").inc()
        try:
            if job_id != "?":
                mark_job_failed(job_id, str(e))
        except Exception as inner:
            print(f"[worker] failed to mark job FAILED: {inner}", flush=True)
        try:
            await msg.nak(delay=10)
        except Exception:
            pass


async def run_consumer():
    """Connect to NATS, ensure stream exists, subscribe with push consumer."""
    while not shutdown_event.is_set():
        try:
            nc = await nats.connect(NATS_URL, name="simaops-ai-worker", reconnect_time_wait=2)
            js = nc.jetstream()

            # Subscribe — push consumer with manual ack and DLQ semantics
            # max_deliver=4: NATS retries up to 4 times, then drops (consumer DLQ pattern)
            # ack_wait=60s: long enough for image fetch + inference
            # The qc.job.dlq subject collects failures via NATS advisory or app-level publish
            from nats.js.api import ConsumerConfig
            sub = await js.subscribe(
                "qc.job.created",
                durable="simaops-ai-worker",
                stream="SIMAOPS",
                manual_ack=True,
                cb=message_handler,
                config=ConsumerConfig(
                    max_deliver=4,
                    ack_wait=120,
                ),
            )
            print(
                f"[worker] subscribed (strategy={QC_STRATEGY}, model={MODEL_VERSION}, max_deliver=4, ack_wait=60s)",
                flush=True,
            )

            await shutdown_event.wait()

            await sub.unsubscribe()
            await nc.drain()
            print("[worker] consumer shut down cleanly", flush=True)
            return

        except Exception as e:
            print(f"[worker] consumer error: {e} — reconnecting in 5s", flush=True)
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
