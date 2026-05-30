#!/usr/bin/env bash
# scripts/e2e-realtime.sh
#
# End-to-end realtime acceptance test. Verifies:
#   - SSE bridge: events flow from API → browser via /events
#   - Role filter: each user role only sees the subjects their role allows
#   - Pipeline integrity: full lot → QC → review → warehouse flow works
#
# Test users (actual realm role assignments — verified via Keycloak admin):
#   budi   - OPERATOR
#   siti   - QC_SUPERVISOR
#   agus   - WAREHOUSE_STAFF
#   dewi   - MANAGER
#   admin  - ADMIN
#
# All passwords = password123.
#
# Expectations matrix:
#                          | budi (op) | siti (sup) | agus (wh) | dewi (mgr) | admin
#   lot.created            |   ✓ (own) |     ✓      |    ✓      |     ✓      |   ✓
#   qc.job.created         |   ✗       |     ✓      |    ✗      |     ✓      |   ✓
#   qc.job.completed       |   ✗       |     ✓      |    ✓      |     ✓      |   ✓
#   qc.job.reviewed        |   ✗       |     ✓      |    ✗      |     ✓      |   ✓
#   warehouse.slot_assigned|   ✓ (own) |     ✓      |    ✓      |     ✓      |   ✓
#   lot.ready_for_production|  ✓ (own) |     ✓      |    ✓      |     ✓      |   ✓
#   dispatch.created       |   ✓ (own) |     ✗      |    ✓      |     ✓      |   ✓
#   dispatch.status_changed|   ✓ (own) |     ✗      |    ✓      |     ✓      |   ✓
#   audit.log_created      |   ✗       |     ✗      |    ✗      |     ✓      |   ✓
#
# (✓ own = OPERATOR sees only events for lots they created)
#
# Usage:
#   bash scripts/e2e-realtime.sh
#   API=https://api.example.com KC=https://auth.example.com bash scripts/e2e-realtime.sh
set -euo pipefail

export API="${API:-https://api.161.118.244.229.sslip.io}"
export KC="${KC:-https://auth.161.118.244.229.sslip.io}"

exec python3 - <<'PY'
import json, os, sys, threading, time
import urllib.parse, urllib.request

KC = os.environ["KC"]
API = os.environ["API"]


def get_token(user: str, password: str = "password123") -> str:
    data = urllib.parse.urlencode({
        "client_id": "simaops-web",
        "username": user,
        "password": password,
        "grant_type": "password",
    }).encode()
    with urllib.request.urlopen(f"{KC}/realms/simaops/protocol/openid-connect/token", data=data, timeout=10) as r:
        return json.loads(r.read())["access_token"]


def open_sse(name: str, token: str, sink: list, stop: threading.Event):
    """Reads SSE events into `sink` until stop is set."""
    req = urllib.request.Request(f"{API}/events", headers={"Authorization": f"Bearer {token}"})
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            current_event = None
            for raw in r:
                if stop.is_set():
                    break
                line = raw.decode().rstrip("\n")
                if line.startswith("event: "):
                    current_event = line[7:]
                elif line.startswith("data: ") and current_event:
                    try:
                        data = json.loads(line[6:])
                    except Exception:
                        data = {"raw": line[6:]}
                    sink.append({"subject": current_event, "data": data})
                    current_event = None
                elif line == "":
                    current_event = None
    except Exception as e:
        sink.append({"subject": "_ERROR", "data": str(e)})


def call(token: str, path: str, body: dict) -> dict:
    req = urllib.request.Request(
        f"{API}{path}",
        data=json.dumps(body).encode(),
        method="POST",
        headers={
            "Content-Type": "application/json",
            "Connect-Protocol-Version": "1",
            "Authorization": f"Bearer {token}",
        },
    )
    with urllib.request.urlopen(req, timeout=15) as r:
        return json.loads(r.read())


def color(s, code):
    return f"\033[{code}m{s}\033[0m"


# ─── 1. Auth ──────────────────────────────────────────────────────
print(color("[1] Auth as 5 users", "1;36"))
USERS = ["budi", "siti", "agus", "dewi", "admin"]
tokens = {u: get_token(u) for u in USERS}
for u, t in tokens.items():
    print(f"  {u:6s} -> token len {len(t)}")


# ─── 2. Open 5 SSE streams ────────────────────────────────────────
print(color("\n[2] Opening 5 SSE streams", "1;36"))
sinks = {u: [] for u in tokens}
stop = threading.Event()
threads = {}
for u, t in tokens.items():
    th = threading.Thread(target=open_sse, args=(u, t, sinks[u], stop), daemon=True)
    th.start()
    threads[u] = th
time.sleep(2)
print("  streams open, sleeping 2s for handshake")


# ─── 3. Run pipeline as budi ──────────────────────────────────────
print(color("\n[3] Pipeline as budi (OPERATOR)", "1;36"))
ts = int(time.time())

lot_resp = call(tokens["budi"], "/simaops.lot.v1.LotService/CreateLot", {
    "supplierName": f"E2E Realtime {ts}",
    "materialName": "Cardamom",
    "materialType": 1,
    "quantity": 50,
    "unit": "kg",
    "arrivalDate": "2026-05-27",
    "storageRequirement": {"temperatureRange": 1, "hazardClass": 1},
    "idempotencyKey": f"e2e-{ts}",
})
lot_id = lot_resp["lot"]["id"]
print(f"  CreateLot     -> {lot_id}")

up_resp = call(tokens["budi"], "/simaops.qc.v1.QCService/CreateQCUploadUrl", {
    "lotId": lot_id,
    "filename": "test.jpg",
    "contentType": "image/jpeg",
    "idempotencyKey": f"e2e-up-{ts}",
})
object_key = up_resp["objectKey"]

req = urllib.request.Request(up_resp["uploadUrl"], data=b"fake image data",
                             method="PUT", headers={"Content-Type": "image/jpeg"})
with urllib.request.urlopen(req, timeout=10):
    pass
print(f"  Upload        -> {object_key}")

qc_resp = call(tokens["budi"], "/simaops.qc.v1.QCService/CreateQCJob", {
    "lotId": lot_id,
    "imageObjectKey": object_key,
    "idempotencyKey": f"e2e-qc-{ts}",
})
qc_job_id = qc_resp["job"]["id"]
print(f"  CreateQCJob   -> {qc_job_id}")

# Wait for AI worker to process (mock takes ~0.5s + NATS round trip)
print("  waiting 4s for AI processing")
time.sleep(4)

call(tokens["siti"], "/simaops.qc.v1.QCService/ReviewQC", {
    "qcJobId": qc_job_id,
    "decision": 1,  # APPROVED
    "reason": "looks good",
    "idempotencyKey": f"e2e-rev-{ts}",
})
print(f"  ReviewQC      -> APPROVED")

# Pick a warehouse slot — RecommendSlot needs WAREHOUSE_STAFF (agus) or ADMIN.
loc_resp = call(tokens["agus"], "/simaops.warehouse.v1.WarehouseService/RecommendSlot", {
    "lotId": lot_id,
})
loc_id = loc_resp["recommendations"][0]["location"]["id"]

assign_resp = call(tokens["agus"], "/simaops.warehouse.v1.WarehouseService/AssignSlot", {
    "lotId": lot_id,
    "locationId": loc_id,
    "idempotencyKey": f"e2e-as-{ts}",
})
print(f"  AssignSlot    -> {assign_resp['assignment']['locationCode']}")

# Lot is now READY_FOR_PRODUCTION — ship it. CreateDispatch + advance the FSM.
disp_resp = call(tokens["agus"], "/simaops.dispatch.v1.DispatchService/CreateDispatch", {
    "lotId": lot_id,
    "destination": "Jakarta DC",
    "carrier": "E2E Logistics",
    "quantity": 50,
    "unit": "kg",
    "idempotencyKey": f"e2e-disp-{ts}",
})
dispatch_id = disp_resp["dispatch"]["id"]
print(f"  CreateDispatch-> {disp_resp['dispatch']['dispatchNumber']}")

call(tokens["agus"], "/simaops.dispatch.v1.DispatchService/UpdateDispatchStatus", {
    "dispatchId": dispatch_id,
    "newStatus": 2,  # SCHEDULED
    "idempotencyKey": f"e2e-disp-adv-{ts}",
})
print(f"  AdvanceDispatch-> SCHEDULED")

print(color("\n[4] Waiting 4s for events to drain", "1;36"))
time.sleep(4)
stop.set()


# ─── 5. Verify expectations ───────────────────────────────────────
print(color("\n[5] Filter assertions", "1;36"))


def subjects_for(user: str) -> set:
    return {e["subject"] for e in sinks[user] if e["subject"] != "connection-info"}


s_budi = subjects_for("budi")
s_siti = subjects_for("siti")
s_agus = subjects_for("agus")
s_dewi = subjects_for("dewi")
s_admin = subjects_for("admin")

failures = []


def assert_in(user: str, subjects: set, want: str):
    if want not in subjects:
        failures.append(f"{user} missing {want}: got {sorted(subjects)}")


def assert_not_in(user: str, subjects: set, want: str):
    if want in subjects:
        failures.append(f"{user} should NOT see {want}: got {sorted(subjects)}")


# Budi (operator, owner-scoped) — sees own lot events.
assert_in("budi", s_budi, "lot.created")
assert_in("budi", s_budi, "lot.status_changed")
assert_in("budi", s_budi, "warehouse.slot_assigned")
assert_in("budi", s_budi, "lot.ready_for_production")
# Operator has dispatch.> (owner-scoped) — sees dispatch events for own lots.
assert_in("budi", s_budi, "dispatch.created")
assert_in("budi", s_budi, "dispatch.status_changed")
# Operator does NOT see qc.job.* (subject filter excludes it)
assert_not_in("budi", s_budi, "qc.job.created")
assert_not_in("budi", s_budi, "qc.job.reviewed")

# Siti (QC supervisor) — sees lot.* + qc.*, no warehouse, no dispatch.
assert_in("siti", s_siti, "lot.created")
assert_in("siti", s_siti, "qc.job.created")
assert_in("siti", s_siti, "qc.job.reviewed")
assert_in("siti", s_siti, "lot.ready_for_production")  # lot.> grants this
# QC supervisor doesn't have warehouse.> or dispatch.> in role perm.
assert_not_in("siti", s_siti, "warehouse.slot_assigned")
assert_not_in("siti", s_siti, "dispatch.created")

# Agus (warehouse staff) — sees lot.*, warehouse.*, dispatch.*, qc.job.approved/completed.
assert_in("agus", s_agus, "lot.created")
assert_in("agus", s_agus, "warehouse.slot_assigned")
assert_in("agus", s_agus, "lot.status_changed")
assert_in("agus", s_agus, "lot.ready_for_production")
assert_in("agus", s_agus, "dispatch.created")
assert_in("agus", s_agus, "dispatch.status_changed")
# Warehouse staff doesn't see general qc.* (only the specific allowed ones).
assert_not_in("agus", s_agus, "qc.job.created")
assert_not_in("agus", s_agus, "qc.job.reviewed")

# Dewi (manager) — sees everything.
for want in ["lot.created", "qc.job.created", "qc.job.reviewed", "warehouse.slot_assigned",
             "lot.ready_for_production", "dispatch.created", "dispatch.status_changed"]:
    assert_in("dewi", s_dewi, want)

# Admin — also sees everything.
for want in ["lot.created", "qc.job.created", "qc.job.reviewed", "warehouse.slot_assigned",
             "lot.ready_for_production", "dispatch.created", "dispatch.status_changed"]:
    assert_in("admin", s_admin, want)

# Print results
for u in USERS:
    print(f"  {u:6s} sees: {sorted(subjects_for(u))}")

if failures:
    print(color(f"\n✗ {len(failures)} assertions failed:", "1;31"))
    for f in failures:
        print(f"  {f}")
    sys.exit(1)

print(color("\n✓ All realtime assertions passed", "1;32"))
PY
