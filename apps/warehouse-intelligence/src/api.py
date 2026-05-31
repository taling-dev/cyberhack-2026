"""FastAPI app for the Warehouse Intelligence service.

Exposes the slotting decision (moved out of the Go API) and a cold-chain
status endpoint fed by a dummy sensor source. Heavy ML deps are optional;
this app only needs numpy at import time.
"""

import asyncio
import logging
import random
from contextlib import asynccontextmanager
from datetime import datetime
from typing import List, Optional

from fastapi import FastAPI
from pydantic import BaseModel

from .cold_chain_monitoring import ColdChainMonitoringEngine, SensorReading

logging.basicConfig(level=logging.INFO)

# ── Slotting decision (ported from the Go RecommendSlot rules) ──────────────

# Mirror of Go tempBounds(): required (min,max) °C per range.
_TEMP_BOUNDS = {
    "TEMPERATURE_RANGE_AMBIENT": (15.0, 25.0),
    "TEMPERATURE_RANGE_COLD": (2.0, 8.0),
    "TEMPERATURE_RANGE_DEEP_FREEZE": (-20.0, -4.0),
}


class StorageRequirement(BaseModel):
    temperature_range: str = "TEMPERATURE_RANGE_AMBIENT"
    hazard_class: Optional[str] = None


class Location(BaseModel):
    id: str
    code: str = ""
    zone: str = ""
    temperature_min: float = 0.0
    temperature_max: float = 0.0
    capacity: int = 0
    hazard_allowed: List[str] = []      # bare drum codes, e.g. ["IBC"]
    drum_compatibility: List[str] = []  # bare drum codes


class RecommendRequest(BaseModel):
    storage_requirement: StorageRequirement
    locations: List[Location]


class Recommendation(BaseModel):
    location_id: str
    reason: str
    score: float


class RecommendResponse(BaseModel):
    recommendations: List[Recommendation]


def _recommend(req: RecommendRequest, coldchain: ColdChainMonitoringEngine) -> List[Recommendation]:
    min_temp, max_temp = _TEMP_BOUNDS.get(
        req.storage_requirement.temperature_range, (15.0, 25.0)
    )
    hz = req.storage_requirement.hazard_class
    drum = ""
    if hz and hz not in ("", "HAZARD_CLASS_NONE"):
        drum = hz.replace("HAZARD_CLASS_", "")

    recs: List[Recommendation] = []
    for loc in req.locations:
        # Temp containment: location range must cover the lot's required range.
        if loc.temperature_min > min_temp or loc.temperature_max < max_temp:
            continue
        # Drum + hazard segregation rules (same as the Go filter).
        if drum:
            if drum not in loc.drum_compatibility:
                continue
            if loc.hazard_allowed and drum not in loc.hazard_allowed:
                continue

        # Score: capacity, tighter temp-fit, and cold-chain zone health.
        fit_bonus = max(0.0, 10.0 - (loc.temperature_max - loc.temperature_min))
        health = coldchain.get_equipment_health_score(loc.zone)
        health_score = health.get("score", 0) or 0
        health_bonus = (health_score - 80) / 10.0  # healthy zones boosted, sick zones penalized
        score = float(loc.capacity) + fit_bonus + health_bonus

        reason = f"matches {req.storage_requirement.temperature_range} ({loc.temperature_min:.0f} to {loc.temperature_max:.0f} °C)"
        if drum:
            reason += f" + {drum} drum"
        if health.get("status") not in (None, "NO_DATA"):
            reason += f"; zone {loc.zone} health {health_score}"
        recs.append(Recommendation(location_id=loc.id, reason=reason, score=score))

    recs.sort(key=lambda r: r.score, reverse=True)
    return recs


# ── Dummy cold-chain sensor source ──────────────────────────────────────────

_ENGINE = ColdChainMonitoringEngine()
_ZONES = ["A", "B", "C"]
_latest: dict = {}  # zone -> {"temperature", "timestamp", "alert"}


async def _sensor_loop():
    """Emit a synthetic reading per zone every few seconds (demo source)."""
    while True:
        for zone in _ZONES:
            # Cold-chain target ~ -3°C; occasional excursion to exercise alerts.
            temp = round(random.gauss(-3.0, 1.0), 2)
            if random.random() < 0.05:
                temp = round(random.uniform(3.0, 6.0), 2)  # excursion
            reading = SensorReading(
                sensor_id=f"sensor-{zone}", equipment_id=zone,
                temperature=temp, timestamp=datetime.now(),
            )
            alert = _ENGINE.process_sensor_reading(reading)
            _latest[zone] = {
                "temperature": temp,
                "timestamp": reading.timestamp.isoformat(),
                "alert": {
                    "severity": alert.severity.value,
                    "message": alert.message,
                    "requires_immediate_action": alert.requires_immediate_action,
                } if alert else None,
            }
        await asyncio.sleep(5)


@asynccontextmanager
async def lifespan(app: FastAPI):
    task = asyncio.create_task(_sensor_loop())
    yield
    task.cancel()


app = FastAPI(title="Warehouse Intelligence", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/slotting/recommend", response_model=RecommendResponse)
async def slotting_recommend(req: RecommendRequest):
    return RecommendResponse(recommendations=_recommend(req, _ENGINE))


@app.get("/coldchain/status")
async def coldchain_status():
    equipment = []
    for zone in _ZONES:
        latest = _latest.get(zone, {})
        equipment.append({
            "equipment_id": zone,
            "latest_temperature": latest.get("temperature"),
            "timestamp": latest.get("timestamp"),
            "latest_alert": latest.get("alert"),
            "health": _ENGINE.get_equipment_health_score(zone),
        })
    return {
        "equipment": equipment,
        "active_alerts": len(_ENGINE.alerts),
        "critical_alerts": sum(1 for a in _ENGINE.alerts if a.requires_immediate_action),
    }
