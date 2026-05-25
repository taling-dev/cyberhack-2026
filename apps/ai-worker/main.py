"""SimaOps AI Worker — NATS JetStream consumer with switchable QC strategy."""

import asyncio
import json
import os
import uuid
from abc import ABC, abstractmethod
from contextlib import asynccontextmanager
from datetime import datetime, timezone

import nats
import pymysql
import uvicorn
from fastapi import FastAPI

# ─── Config ──────────────────────────────────────────────────────

NATS_URL = os.getenv("NATS_URL", "nats://localhost:4222")
TIDB_HOST = os.getenv("TIDB_HOST", "localhost")
TIDB_PORT = int(os.getenv("TIDB_PORT", "4000"))
TIDB_USER = os.getenv("TIDB_USER", "root")
TIDB_PASSWORD = os.getenv("TIDB_PASSWORD", "")
TIDB_DB = os.getenv("TIDB_DB", "simaops")
QC_STRATEGY = os.getenv("QC_STRATEGY", "mock")  # mock | pretrained | custom
MODEL_VERSION = os.getenv("MODEL_VERSION", "mock-v0.1.0")

# ─── Strategy Interface ──────────────────────────────────────────


class QCStrategy(ABC):
    @abstractmethod
    async def analyze(self, image_key: str, material_type: str) -> dict:
        """Returns {recommendation, confidence, findings}."""
        ...


class MockStrategy(QCStrategy):
    async def analyze(self, image_key: str, material_type: str) -> dict:
        findings = [
            {"class_name": "bottle", "mapped_finding": "foreign_matter", "confidence": 0.87, "is_anomaly": True},
            {"class_name": "banana", "mapped_finding": "ripeness_signal", "confidence": 0.92, "is_anomaly": False},
        ]
        return {"recommendation": "REVIEW", "confidence": 0.82, "findings": findings}


def get_strategy() -> QCStrategy:
    if QC_STRATEGY == "mock":
        return MockStrategy()
    # pretrained and custom strategies added in Task 28
    return MockStrategy()


# ─── Database ─────────────────────────────────────────────────────


def get_db():
    return pymysql.connect(
        host=TIDB_HOST, port=TIDB_PORT, user=TIDB_USER,
        password=TIDB_PASSWORD, database=TIDB_DB, autocommit=True,
    )


def write_qc_result(job_id: str, lot_id: str, result: dict):
    db = get_db()
    try:
        with db.cursor() as cur:
            cur.execute(
                """INSERT INTO qc_results (id, qc_job_id, lot_id, recommendation, confidence, findings_json, model_version)
                   VALUES (%s, %s, %s, %s, %s, %s, %s)
                   ON DUPLICATE KEY UPDATE recommendation=VALUES(recommendation)""",
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
            cur.execute("UPDATE qc_jobs SET status='FAILED', failure_reason=%s, completed_at=NOW() WHERE id=%s", (reason, job_id))
    finally:
        db.close()


# ─── NATS Consumer ────────────────────────────────────────────────

strategy = get_strategy()


async def process_message(msg):
    try:
        payload = json.loads(msg.data)
        job_id = payload["qc_job_id"]
        lot_id = payload["lot_id"]
        image_key = payload["image_object_key"]
        material_type = payload.get("material_type", "RAW_BOTANICAL")

        print(f"[worker] processing job={job_id} image={image_key} material={material_type}")

        result = await strategy.analyze(image_key, material_type)
        write_qc_result(job_id, lot_id, result)

        # Publish completion event
        nc = msg._client
        await nc.publish("qc.job.completed", json.dumps({"qc_job_id": job_id, "lot_id": lot_id}).encode())

        await msg.ack()
        print(f"[worker] completed job={job_id} recommendation={result['recommendation']}")

    except Exception as e:
        print(f"[worker] error processing message: {e}")
        # NAK for retry (NATS will redeliver up to max_deliver)
        await msg.nak()


async def run_consumer():
    nc = await nats.connect(NATS_URL)
    js = nc.jetstream()

    # Subscribe to qc.job.created with durable consumer
    try:
        sub = await js.subscribe(
            "qc.job.created",
            durable="simaops-ai-worker",
            stream="SIMAOPS",
        )
        print(f"[worker] subscribed to qc.job.created (strategy={QC_STRATEGY})")

        async for msg in sub.messages:
            await process_message(msg)

    except Exception as e:
        print(f"[worker] consumer error: {e}")
    finally:
        await nc.close()


# ─── FastAPI Health Endpoints ─────────────────────────────────────


@asynccontextmanager
async def lifespan(app: FastAPI):
    task = asyncio.create_task(run_consumer())
    yield
    task.cancel()


app = FastAPI(title="SimaOps AI Worker", lifespan=lifespan)


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


@app.get("/readyz")
def readyz():
    try:
        db = get_db()
        db.close()
        return {"status": "ready"}
    except Exception as e:
        return {"status": "not_ready", "error": str(e)}


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8081)
