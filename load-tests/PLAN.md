# SimaOps AI — Constant-Load Stress Test Plan

> **Status:** Plan approved 2026-05-27. Execution deferred. Resume by running `bash load-tests/scripts/run-validation.sh` after Tasks 1–9 are implemented.

## Problem Statement

Validate that the deployed SimaOps AI platform (running on OKE, exposed via `*.161.118.244.229.sslip.io`) can sustain realistic production traffic without errors, scales appropriately under load, and remains stable over multi-hour runs. Specifically: drive the full E2E pipeline (lot creation → QC upload → AI processing → reviewer approval → warehouse assignment) from a laptop through public HTTPS ingress, capture autoscaling behavior on api/ai-worker HPAs, and surface any leaks or backpressure issues during a 2-hour soak.

## Requirements

| Requirement | Decision |
|---|---|
| Goals | Validate HPA scaling **and** soak for stability |
| Scope | Full E2E pipeline (lot → QC → review → assign), all 5 services exercised |
| Load source | Laptop → public HTTPS ingress (`*.161.118.244.229.sslip.io`) |
| Validation run | 20 VUs, 15 min — should trigger HPA api 2→3, ai-worker 1→2 |
| Soak run | 5 VUs, 2 hours — ~7,200 lots, surface leaks |
| Pass criteria | Zero 5xx, p95 < 500ms (non-AI), HPA scaled, zero pod restarts, JetStream lag < 100, all lots reach terminal state |

## Background — System Constraints

1. **Keycloak brute-force protection enabled** — 5 failures lock the user 60s. Test MUST cache tokens per VU (one login per VU, refresh ~10s before expiry).
2. **Keycloak default access token lifespan** — 5 min. With 5–20 VUs, that's ~1 login every 15s on average; well under the 5-failure threshold.
3. **AI worker is the bottleneck** — mock latency ~1s per QC job, max 3 replicas via HPA. End-to-end completion rate caps at **~3 lots/sec**. At 20 VUs we'll create lag in JetStream — that's the point of the validation run.
4. **MinIO has no PVC** — buckets `simaops-qc-images`, `simaops-qc-results`, `simaops-reports`, `simaops-model-artifacts` get wiped on pod restart. Pre-flight must verify buckets exist; if MinIO restarts mid-test the run is invalid.
5. **Outbox publisher is singleton** (leader-elected, max 1 replica) — cannot scale; must keep up by itself or backlog grows.
6. **TiDB single-instance in dev** — no HA failover; if it dies the test stops.
7. **Existing Grafana dashboards** — `simaops-ops`, `simaops-api`, `simaops-ai`, plus 6 PrometheusRule alerts already defined. We don't need to build dashboards; just point at them.
8. **Idempotency** — `CreateLot` requires unique `idempotencyKey`. We'll use `${VU}-${ITER}-${timestamp}` to guarantee uniqueness across parallel VUs.
9. **BFF proxy** — browser flow goes through `/api/v1/*` on the web pod, but for load tests we hit `api.161.118.244.229.sslip.io` directly with Bearer tokens (more efficient — skips a hop).

## Tool Choice — k6

Why k6 over Locust / Vegeta / JMeter:
- JS scripts (matches our stack, easy to read), no JVM
- Built-in `scenarios` config — smoke + validation + soak in one file with shared code
- Built-in thresholds (auto-fail run if p95 > 500ms or error rate > 1%)
- Native Prometheus remote-write support — pushes k6 metrics straight into our existing observability stack so Grafana shows app metrics + load metrics on one timeline
- Connect-RPC is just JSON-over-POST — no special library needed

## Architecture

```
┌────────────────────┐                    ┌─────────────────────┐
│  Your laptop       │                    │  OKE cluster        │
│                    │                    │                     │
│  ┌──────────────┐  │  HTTPS public      │  ┌───────────────┐  │
│  │  k6 binary   │──┼──────────────────► │  │ ingress-nginx │  │
│  │              │  │                    │  └───────┬───────┘  │
│  │  scenarios:  │  │                    │          │          │
│  │  - smoke     │  │                    │     ┌────▼────┐     │
│  │  - validation│  │                    │     │   api   │ ←HPA│
│  │  - soak      │  │                    │     │ (2→5)   │     │
│  └──────┬───────┘  │                    │     └────┬────┘     │
│         │          │                    │          │ outbox   │
│         │          │                    │     ┌────▼────┐     │
│         ▼          │                    │     │  NATS   │     │
│  ┌──────────────┐  │  Prometheus        │     └────┬────┘     │
│  │ k6 summary   │  │  remote_write      │     ┌────▼────┐     │
│  │ JSON + HTML  │  │  (auth via         │     │ ai-     │ ←HPA│
│  │ report       │  │   ingress)         │     │ worker  │     │
│  └──────────────┘  │                    │     │ (1→3)   │     │
│                    │                    │     └─────────┘     │
└────────────────────┘                    └─────────────────────┘
                                              ▲ Grafana scrapes
                                              │ + k6 metrics
```

## Test Layout

```
load-tests/
├── PLAN.md                      # this file
├── README.md                    # how to run, prerequisites
├── lib/
│   ├── auth.js                  # token cache per VU
│   ├── helpers.js               # uuid, sleep, fixtures
│   └── pipeline.js              # one-shot E2E iteration
├── scenarios/
│   ├── smoke.js                 # 1 VU, 1 min — sanity check
│   ├── validation.js            # 20 VUs, 15 min — HPA validation
│   └── soak.js                  # 5 VUs, 2 hr — stability
├── fixtures/
│   └── qc-image.jpg             # 1 KB placeholder JPG
├── scripts/
│   ├── preflight.sh             # check pods, buckets, HPA, users
│   ├── run-validation.sh        # k6 run + capture report
│   ├── run-soak.sh              # k6 run + capture report
│   └── postflight.sh            # DB counts, stuck lots, HPA events
└── reports/                     # output directory (gitignored)
```

## Pass / Fail Thresholds (codified in k6)

```javascript
thresholds: {
  http_req_failed: ['rate<0.01'],                       // < 1% errors
  'http_req_duration{rpc:non_ai}': ['p(95)<500'],       // p95 < 500ms
  'http_req_duration{rpc:list_lots}': ['p(95)<300'],
  pipeline_e2e_completed: ['rate>0.95'],                // 95%+ pipelines complete
  iteration_duration: ['p(99)<10000'],                  // p99 iter < 10s
}
```

If any threshold breaches, k6 exits non-zero and the run is marked failed.

---

## Task Breakdown

### Task 1: Set up `load-tests/` skeleton and install k6

- Create `load-tests/` directory with `lib/`, `scenarios/`, `fixtures/`, `scripts/`, `reports/`
- Add `reports/` to `.gitignore`
- Install k6 binary on the laptop (`apt install k6` via the official Grafana repo, or download static binary)
- Add a `load-tests/README.md` explaining prerequisites and how to invoke each scenario
- **Test**: `k6 version` succeeds; `tree load-tests/` shows the structure
- **Demo**: Empty `k6 run --vus 1 --duration 5s scenarios/smoke.js` prints "[OK] smoke ran" — proves harness is functional

### Task 2: Implement auth helper with token cache (`lib/auth.js`)

- Function `login(username, password)` → POST to Keycloak token endpoint, returns `{access_token, expires_at}`
- Per-VU cache (k6 `vu.idInTest` keyed Map at module scope) — one token per VU, lazy-refresh 10s before expiry
- Round-robin across the 5 demo users (budi, siti, agus, dewi, admin) so each role is exercised
- Reject any 4xx from token endpoint with a clear error (lockout will manifest as 401 on token endpoint)
- **Test**: `k6 run --vus 5 --iterations 100 lib/auth.test.js` — 100 iterations, 5 VUs, only 5 token requests issued (verified via `auth_token_requests` counter)
- **Demo**: stdout shows `auth_token_requests: 5` after 100 iterations — proves cache works

### Task 3: Generate test fixtures (`fixtures/`, `lib/helpers.js`)

- Create `fixtures/qc-image.jpg` — a 1×1 white pixel JPG (~125 bytes) for fast uploads
- `helpers.js`:
  - `uuidV7()` — for idempotency keys
  - `randomLotData(vu, iter)` — returns a `CreateLotRequest` with varied supplier names from a fixed list (`Sumber Tani`, `Cardamom Co`, etc.) and material names (`Cardamom`, `Sage`, `Lemongrass`, …)
  - `qcImageBytes()` — load fixture as `ArrayBuffer`
- **Test**: print 3 sample requests via `k6 run scenarios/sample-data.js`
- **Demo**: shows 3 distinct lot payloads with unique `idempotencyKey`s

### Task 4: Implement one-shot E2E pipeline (`lib/pipeline.js`)

A single `runPipeline(operatorToken, supervisorToken, warehouseToken)` function that performs:
1. `CreateLot` (operator) → captures `lot.id`
2. `CreateQCUploadUrl` (operator) → captures presigned PUT URL
3. `PUT image` to MinIO via the presigned URL — tags the request with `rpc:minio_upload`
4. Wait for AI worker to process — poll `GetLot` every 1s up to 15s, until `status=AWAITING_REVIEW`. Tag waits separately so we can measure AI latency end-to-end.
5. `ReviewQC` (supervisor) → `decision=APPROVED`
6. `AssignSlot` (warehouse) → picks any READY slot
7. `GetLotTimeline` → asserts ≥ 3 entries (validates audit chain)

Emit a custom counter `pipeline_e2e_completed{result=success|failed}` and a `pipeline_e2e_duration` histogram. Tag each HTTP request with `rpc:create_lot|review_qc|...` so thresholds can target individual RPCs.

- **Test**: `k6 run --vus 1 --iterations 1 scenarios/smoke.js` completes one full pipeline; assert all 7 steps succeeded
- **Demo**: stdout `✓ pipeline completed in 4.2s | lot LOT-2026-05-27-XXXXXX → READY_FOR_PRODUCTION`

### Task 5: Smoke scenario (`scenarios/smoke.js`)

- 1 VU, 1 minute, no ramp
- Imports `runPipeline` from `lib/pipeline.js`, runs it in a loop with `sleep(1)` between iterations
- Tight thresholds — 0 errors, p95 < 1s for non-AI RPCs
- Purpose: catches any regression before the longer runs
- **Test**: `bash scripts/preflight.sh && k6 run scenarios/smoke.js`
- **Demo**: full green output, 1-min run, ~10–15 successful pipelines

### Task 6: Validation scenario (`scenarios/validation.js`) — HPA test

- Three k6 stages: ramp 0→20 VUs over 1 min, hold 20 VUs for 13 min, ramp down to 0 over 1 min
- Each VU loops `runPipeline` with 100ms jitter — sustains ~5 lots/sec offered load
- Stages tagged with `phase:rampup|steady|rampdown` — so we can compare steady-state metrics only
- Pre-test snapshot: `kubectl get hpa -n simaops -o json > reports/validation-hpa-before.json`
- Post-test snapshot: same → `validation-hpa-after.json`
- Capture HPA scaling events: `kubectl get events -n simaops --field-selector reason=SuccessfulRescale > reports/validation-hpa-events.txt`
- **Test**: scenario file syntax check via `k6 inspect scenarios/validation.js`
- **Demo**: 15-min run completes; report shows api scaled from 2→3 (or higher) replicas during steady phase

### Task 7: Soak scenario (`scenarios/soak.js`) — stability test

- 5 VUs constant for 2 hours, no ramp
- Uses `constant-arrival-rate` executor at 1 iter/sec (bounded — protects against runaway client)
- Same `runPipeline` body, longer sleeps — total target ~3,600 completed pipelines
- Periodic in-test checks (every 10 min) via custom `setup`/`teardown`-style logic in a separate VU:
  - Snapshot heap-relevant metrics (no direct access — relies on Prometheus scrape)
- **Test**: dry-run with `--duration 60s` to verify rate executor works
- **Demo**: 2-hour run; report shows steady p95 latency over time (no upward drift), zero pod restarts

### Task 8: Pre-flight & post-flight scripts

**Pre-flight checks** (`scripts/preflight.sh` — must pass before any run):
- All 6 simaops pods `Running` and `Ready`
- All 5 ingresses respond 200/302
- MinIO buckets exist: `simaops-qc-images`, `simaops-qc-results`, `simaops-reports`, `simaops-model-artifacts`
- All 5 demo users can authenticate (one quick `password123` test, sequential — won't trigger lockout)
- `kubectl top pods -n simaops` returns metrics (confirms metrics-server still up)
- HPA `ScalingActive: True` for all 3 HPAs
- Print baseline replica counts

**Post-flight reports** (`scripts/postflight.sh`):
- Pod status diff: any `Running → CrashLoopBackOff` since pre-flight
- Pod restart counter — fail if any non-zero
- HPA replica history (from event log)
- DB row counts: `SELECT COUNT(*), status FROM lots GROUP BY status` — fail if `LOT_STATUS_AI_PROCESSING` or `LOT_STATUS_AWAITING_REVIEW` count is non-zero (stuck lots)
- Outbox table backlog: `SELECT COUNT(*) FROM outbox_events WHERE published_at IS NULL` — must be 0
- JetStream consumer lag (via NATS exporter Prometheus): max lag during run
- Captured Grafana dashboard PNG via Grafana's image rendering API (link in report instead of inline if rendering not available)
- **Test**: run pre-flight against current cluster — passes
- **Demo**: `bash scripts/preflight.sh` prints a green checklist; `bash scripts/postflight.sh` produces `reports/postflight-<timestamp>.md`

### Task 9: Runner scripts + report consolidation (`scripts/run-validation.sh`, `run-soak.sh`)

- Each runner script:
  1. `bash preflight.sh` (abort on failure)
  2. `k6 run --out json=reports/<name>-raw.json --summary-export=reports/<name>-summary.json scenarios/<name>.js | tee reports/<name>-stdout.txt`
  3. `bash postflight.sh > reports/<name>-postflight.md`
  4. Generate consolidated `reports/<name>-report.md` with: scenario config, k6 summary, HPA timeline, pod restart count, DB stats, screenshots/links to Grafana dashboards
- Exit code = k6's exit code (so CI / shell can check)
- **Test**: `bash scripts/run-validation.sh --duration 60s` (override for quick smoke of harness)
- **Demo**: produces a single markdown report file the user can open and read top-to-bottom

### Task 10: Run validation, capture results, document findings

- Execute the full 15-min validation run
- Inspect: did api HPA scale? did ai-worker HPA scale? Was JetStream lag bounded? Any 5xx?
- If any threshold failed, root-cause and document. Common likely issues:
  - **AI worker stuck at min replicas** — CPU not high enough; mock latency is sleep, not CPU. Document as expected; HPA on CPU won't trigger for mock. Note: real inference would burn CPU.
  - **Ingress 504** — would mean api or web pod is overloaded; check pod CPU
  - **Lot stuck in AWAITING_REVIEW** — means E2E iteration didn't complete review step; bug in pipeline.js
- Commit the full `reports/validation-*.{md,json}` set to `load-tests/reports-archive/` (a separate gitignored-by-default but archived path) **only if user wants the artifacts in git**; otherwise just leave on disk
- **Test**: report contains all sections; thresholds visible
- **Demo**: open `reports/validation-report.md` — full pass/fail summary, links to Grafana time range

### Task 11: Run soak, capture results, document findings

- Execute the 2-hour soak run
- Watch for patterns over time (the unique value of soak vs validation):
  - **Memory drift** — any Go service growing > 50 MB/hr is suspect
  - **DB pool exhaustion** — symptom: latency cliff after ~30 min
  - **NATS / JetStream stream growth** — bound by ack policy; if not, leak
  - **MinIO bucket size** — soak generates ~3,600 small uploads (~450 KB total); harmless but worth noting
- After soak, leave the cluster running 5 minutes idle then sample again — confirms metrics return to baseline
- Generate `reports/soak-report.md` with same structure as validation, plus a "Findings" section
- **Test**: report exists with all sections populated
- **Demo**: open `reports/soak-report.md`; it shows latency time-series, replica count over time, no leaks

### Task 12: Final summary + commit

- Single combined report at `load-tests/RESULTS.md` linking validation + soak findings
- Commit `load-tests/` directory (without `reports/` artifacts beyond a single representative run if user wants reference) with a clear commit message
- Add a section to the project's main `README.md` pointing to `load-tests/README.md`
- **Test**: `bash load-tests/scripts/run-validation.sh --duration 30s` re-runs cleanly from a fresh git clone of the repo
- **Demo**: a new contributor can clone the repo and reproduce the load test

---

## Risk Notes

| Risk | Mitigation |
|---|---|
| Brute-force lockout during test | Token cache; round-robin across 5 users; never log a wrong password |
| MinIO bucket gone after pod restart | Pre-flight check; abort run if missing; recreate before retry |
| Test creates 7,200+ lots in `simaops` DB | Document; user can `TRUNCATE lots` after if needed (separate cleanup script) |
| Laptop bandwidth limits load | Document — stop if k6 reports `dial: connection refused` repeatedly (means client-side saturation) |
| HPA on AI worker won't trigger with mock | Note in report — mock is sleep-bound, not CPU-bound; real inference would scale |
| Public ingress rate limits | Use HTTP keep-alive (k6 default); monitor 429s |

---

## Open Questions / Things You Might Want to Adjust

1. **Should we cleanup test data?** Default is *no* cleanup so the data is available for postmortem. Add a `scripts/cleanup.sh` that runs `TRUNCATE lots, qc_jobs, audit_logs, outbox_events`?
2. **Should the validation+soak run from a Kubernetes Job too** (option 3a from earlier), as a sanity comparison? Adds Task 13 if yes.
3. **Should the runner push k6 metrics to Prometheus remote-write** so Grafana shows live load alongside app metrics? Adds prometheus auth setup. Cleaner reports but more setup.

---

## To Resume

When you're ready to actually run the tests, say something like:
- "Run the load test plan starting at Task 1" — start the full implementation
- "Just do Tasks 1–4 for now" — partial implementation
- "Skip to running the validation scenario" — assumes harness exists

The plan above is the source of truth. Open this file in your editor to review or amend before starting.
