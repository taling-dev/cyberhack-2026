# Demo Script (10 minutes)

## Setup

Ensure the staging cluster is running and seeded (`make demo-reset` if needed).

Login URL: `https://app.<ip>.sslip.io/auth/login`

## Flow

### 1. Operator — Lot Intake (2 min) [Focus Area 1]

- Login as `operator / Operator123!`
- Navigate to **Lots → Create New Lot**
- Create: "Patchouli Oil" / Raw Botanical / 50 kg / Ambient / No drum
- Create: "Vanilla Extract" / Extract / 25 L / Cold / IBC
- Show the lot list with both entries in DRAFT status

### 2. Operator — QC Image Upload (1 min) [Focus Area 2]

- Open the Patchouli Oil lot detail
- Upload a QC image (drag & drop)
- Click **Start QC** → lot advances to PENDING_QC
- Note: the AI worker processes asynchronously via NATS

### 3. QC Supervisor — Review (2 min) [Focus Areas 2 & 3]

- Login as `qc_supervisor / QCSupervisor123!`
- Navigate to **Quality Control**
- Open the Patchouli Oil QC review
- Show AI findings: foreign_matter (anomaly), ripeness_signal
- Show recommendation: REVIEW, confidence: 82%, model: mock-v0.1.0
- **Approve** with reason: "Foreign matter is packaging artifact, acceptable"
- Lot advances to QC_APPROVED

### 4. Warehouse Staff — Slot Assignment (2 min) [Focus Area 4]

- Login as `warehouse / Warehouse123!`
- Navigate to **Warehouse**
- See the approved Patchouli Oil lot in the queue
- Click **Assign Slot** → see recommendations filtered by temperature (Ambient → Zone A)
- Assign to A-01 → lot advances to READY_FOR_PRODUCTION
- Show the "why this slot?" tooltip explaining the match
- Note: the assignment also emits a `lot.ready_for_production` handoff event

### 5. Warehouse Staff — Dispatch (1.5 min) [Focus Area 1]

- Still as `warehouse / Warehouse123!`, navigate to **Dispatch**
- Click **+ New Dispatch** → the Patchouli Oil lot appears in the
  production-ready picker (driven by the handoff event)
- Enter destination "Jakarta DC", carrier, quantity → **Create Dispatch**
- Advance the dispatch: PENDING → SCHEDULED → IN_TRANSIT → DELIVERED
- Talking point: this closes the loop — intake → QC → warehouse →
  production handoff → dispatch, all in one auditable source of truth

### 6. Manager — Dashboard (1.5 min) [Focus Area 1]

- Login as `manager / Manager123!`
- Navigate to **Dashboard**
- Show KPI cards: total lots, awaiting QC, ready for production, pass rate
- Show QC metrics: pass/review/fail breakdown
- Show warehouse capacity by zone

### 7. Audit Trail (1 min) [Focus Area 1]

- Navigate to **Audit Log** — show the full chronological trail
- Open the Patchouli Oil lot detail → Timeline tab
- Show: created → uploaded → QC submitted → AI completed → approved → assigned → ready

### 8. Admin — RBAC (0.5 min)

- Login as `admin / Admin123!`
- Navigate to **Admin** → show user list with roles
- Demonstrate that operator cannot access /admin (403)

## Key Talking Points

- **Single source of truth** — no double data entry across systems
- **AI is assistive** — human supervisor always has final say
- **Cold-chain aware** — warehouse recommender matches temperature + drum requirements
- **Fully auditable** — every action traced with actor, role, timestamp
- **Cloud-portable** — runs on any Kubernetes cluster, not locked to GCP services
- **Enterprise-grade** — idempotency, outbox pattern, NATS JetStream, OpenTelemetry
