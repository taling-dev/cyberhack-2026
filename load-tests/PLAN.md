# SimaOps AI — Constant-Load Stress Test Plan

> **Status:** Plan approved 2026-05-27. Updated 2026-05-28 with pre-execution
> delta corrections and review issues #1, #2, #5, #6, #7. Execution still
> deferred. Resume by running `bash load-tests/scripts/run-validation.sh`
> after Tasks 1–9 are implemented.

## Revision History

| Date | Change |
|------|--------|
| 2026-05-27 | Initial plan approved. |
| 2026-05-28 | Updated for system changes since approval (MinIO PVC, lot-status enum, two-phase outbox claim, AI-worker pull-mode + HPA min=2, hourly cleanup goroutines, BFF Origin allowlist). Loosened JetStream lag threshold (issue #1), extended validation steady-state to 20 min (issue #2), wired k6→Prometheus remote-write into the runner scripts (issue #5), reworked soak leak budget to be throughput-relative (issue #6), and added Task 13 for chaos / failure injection (issue #7). |

## Problem Statement

Validate that the deployed SimaOps AI platform (running on OKE, exposed via `*.161.118.244.229.sslip.io`) can sustain realistic production traffic without errors, scales appropriately under load, and remains stable over multi-hour runs. Specifically: drive the full E2E pipeline (lot creation → QC upload → AI processing → reviewer approval → warehouse assignment) from a laptop through public HTTPS ingress, capture autoscaling behavior on api/ai-worker HPAs, and surface any leaks or backpressure issues during a 2-hour soak.

## Requirements

| Requirement | Decision |
|---|---|
| Goals | Validate HPA scaling **and** soak for stability |
| Scope | Full E2E pipeline (lot → QC → review → assign), all 5 services exercised |
| Load source | Laptop → public HTTPS ingress (`*.161.118.244.229.sslip.io`) |
| Validation run | 20 VUs, 22 min (1 ramp-up + 20 steady + 1 ramp-down) — should trigger HPA api 2→3+ and ai-worker 2→3, then scale-down within 10 min after ramp |
| Soak run | 5 VUs, 2 hours — ~7,200 lots, surface leaks |
| Pass criteria | Zero 5xx, p95 < 500ms (non-AI), HPA scaled, zero pod restarts, JetStream lag < 100, all lots reach terminal state |

## Background — System Constraints

> **Note** — items marked **(2026-05-28)** reflect changes since this plan
> was first approved. Pre-execution sanity-check still recommended.

1. **Keycloak brute-force protection enabled** — 5 failures lock the user 60s. Test MUST cache tokens per VU (one login per VU, refresh ~10s before expiry).
2. **Keycloak default access token lifespan** — 5 min. With 5–20 VUs, that's ~1 login every 15s on average; well under the 5-failure threshold.
3. **AI worker is the bottleneck** — mock latency ~1s per QC job, max 3 replicas via HPA. End-to-end completion rate caps at **~3 lots/sec**. At 20 VUs we'll create lag in JetStream — that's the point of the validation run.  
   **(2026-05-28)** AI worker now uses a **JetStream pull consumer** with HPA `minReplicas=2, maxReplicas=3`. Pull mode lets all replicas compete on the same durable, so multi-pod throughput is real (verified 3/3 split under a 6-job burst). Baseline is **2 ai-worker replicas**, not 1 — validation should observe scale-up to 3, not 1→2.
4. **MinIO has no PVC** — buckets `simaops-qc-images`, `simaops-qc-results`, `simaops-reports`, `simaops-model-artifacts` get wiped on pod restart. Pre-flight must verify buckets exist; if MinIO restarts mid-test the run is invalid.  
   **(2026-05-28)** MinIO **now has a PVC** (commit `6f8e7d3`). Buckets persist across MinIO pod restarts. Pre-flight bucket existence check stays as a sanity gate, but a MinIO restart during the run no longer invalidates results — only impacts whatever was uploaded during the brief unavailability window.
5. **Outbox publisher is singleton** (leader-elected, max 1 replica) — cannot scale; must keep up by itself or backlog grows.  
   **(2026-05-28)** Outbox publisher now uses a **two-phase claim pattern** (B-07): a row goes PENDING → PUBLISHING → PUBLISHED. On crash recovery (lease handoff), the new leader resets any stuck PUBLISHING rows back to PENDING via `OnStartedLeading` callback. Postflight stuck-row query must include `'PUBLISHING'` in addition to `'PENDING'` (see Pass-Criteria SQL below).
6. **TiDB single-instance in dev** — no HA failover; if it dies the test stops. **(2026-05-28)** TiDB now requires authentication: services connect as the `simaops` user (not root) using a 32-char password from the `simaops-infra-creds` Secret. Test scripts that hit the DB directly (postflight) must use these credentials — the password is in the Secret, not hardcoded anywhere.
7. **Existing Grafana dashboards** — `simaops-ops`, `simaops-api`, `simaops-ai`, plus 6 PrometheusRule alerts already defined. We don't need to build dashboards; just point at them.
8. **Idempotency** — `CreateLot` requires unique `idempotencyKey`. We'll use `${VU}-${ITER}-${timestamp}` to guarantee uniqueness across parallel VUs. **(2026-05-28)** API now runs an hourly cleanup goroutine (`startCleanupWorker` in `apps/api/cmd/api/main.go`) that deletes `idempotency_keys` rows older than 24h and `outbox_events` rows with `status='PUBLISHED'` older than 7 days. **Heap should stay flat for the 2-hour soak** — if it doesn't, that's a real leak signal, not the cleanup running behind.
9. **BFF proxy** — browser flow goes through `/api/v1/*` on the web pod, but for load tests we hit `api.161.118.244.229.sslip.io` directly with Bearer tokens (more efficient — skips a hop).  
   **(2026-05-28)** BFF now enforces an Origin allowlist on `/api/v1/*` (F-05 fix). The plan's choice to bypass the BFF is **still correct** — the API itself does NOT enforce Origin (only the BFF does), so direct calls with a Bearer token continue to work. Going through the BFF without a matching Origin would 403.
10. **(2026-05-28) Lot status enum** — the API uses `LOT_STATUS_QC_REVIEW`, not `LOT_STATUS_AWAITING_REVIEW` (the plan's earlier wording). Postflight stuck-lot SQL must filter on `'AI_PROCESSING'` and `'QC_REVIEW'` (the two states a lot can dwell in if the pipeline gets stuck).
11. **(2026-05-28) `material_type` is an integer enum**, not a string. The API expects `1`=RAW_BOTANICAL, `2`=EXTRACT, `3`=POWDER, `4`=OTHER. `randomLotData` in `lib/helpers.js` must pick from `[1,2,3,4]`, not from a list of strings.
12. **(2026-05-28) Content-type on QC uploads** — the API now allow-lists `image/jpeg|image/png|image/webp` (B-03). The `qc-image.jpg` fixture and matching `Content-Type: image/jpeg` header are unchanged — still works. Sending any other type would 400 invalid_argument.

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
│   ├── validation.js            # 20 VUs, 22 min — HPA validation (was 15min)
│   └── soak.js                  # 5 VUs, 2 hr — stability
├── fixtures/
│   └── qc-image.jpg             # 1 KB placeholder JPG
├── grafana/
│   └── k6-overlay-dashboard.json   # k6 + app metrics on one timeline (issue #5)
├── scripts/
│   ├── preflight.sh             # check pods, buckets, HPA, users
│   ├── run-validation.sh        # k6 run + capture report (incl. PromRW)
│   ├── run-soak.sh              # k6 run + capture report (incl. PromRW)
│   ├── postflight.sh            # DB counts, stuck lots, HPA events, JetStream lag
│   ├── chaos-ai-worker.sh       # 13a — kill ai-worker mid-batch (issue #7)
│   ├── chaos-outbox-publisher.sh# 13b — force lease handoff (issue #7)
│   └── chaos-nats.sh            # 13c — NATS unreachable for 30s (issue #7)
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

### JetStream lag thresholds (postflight, not k6)

> **(2026-05-28, issue #1)** The original plan said "JetStream lag < 100"
> as a hard cap. With 20 VUs offering ~5 lots/sec and 3 ai-workers at ~1s
> mock latency = 3 lots/sec capacity, lag will exceed 100 within seconds —
> that's literally the point of the validation run (queue forms → HPA
> scales → queue drains). Replaced with a **shape-based** assertion:

| Phase | Lag check | Rationale |
|---|---|---|
| Steady-state peak | `< 1000` | Bounds runaway backlog. ~3-min worst-case drain at 3 lots/sec. |
| Last 60s of run | `< 100` | Drain phase — workers should have caught up after rampdown started. |
| End-of-run + 60s grace | `< 50` | Confirms backlog cleared after ramp completes. |

These are checked in `scripts/postflight.sh` against the
`nats_consumer_pending_count{durable_name="simaops-ai-worker"}` Prometheus
series, not as k6 thresholds (k6 doesn't see consumer lag).

### Postflight stuck-state SQL

> **(2026-05-28)** Updated for the lot-status enum + outbox PUBLISHING state.

```sql
-- Lots stuck mid-pipeline (>10 min in transient state)
SELECT id, lot_number, status, updated_at
FROM lots
WHERE status IN ('AI_PROCESSING', 'QC_REVIEW')
  AND updated_at < NOW() - INTERVAL 10 MINUTE;
-- expected: 0 rows post-test

-- Outbox events stuck (anything not PUBLISHED after 30s grace)
SELECT id, event_type, status, retry_count, created_at
FROM outbox_events
WHERE status IN ('PENDING', 'PUBLISHING')
  AND created_at < NOW() - INTERVAL 30 SECOND;
-- expected: 0 rows post-test (publisher's tick interval is 500ms)

-- Permanently FAILED outbox events
SELECT COUNT(*) FROM outbox_events WHERE status = 'FAILED';
-- expected: 0; >0 means NATS publish failed 10× retries — investigate.
```

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

> **(2026-05-28, issue #2)** Steady-state extended from 13 min to 20 min so we
> reliably observe both scale-up and (importantly) scale-down. HPA scale-down
> stabilization defaults to 5 min, so a 13-min steady + 1-min rampdown won't
> let scale-down fire — and we need to see scale-down to know the system
> recovers from peak load. Total run is now ~22 min.

- Three k6 stages: ramp 0→20 VUs over 1 min, hold 20 VUs for **20 min**, ramp down to 0 over 1 min
- Each VU loops `runPipeline` with 100ms jitter — sustains ~5 lots/sec offered load
- Stages tagged with `phase:rampup|steady|rampdown` — so we can compare steady-state metrics only
- Pre-test snapshot: `kubectl get hpa -n simaops -o json > reports/validation-hpa-before.json`
- Post-test snapshot: same → `validation-hpa-after.json`
- Capture HPA scaling events: `kubectl get events -n simaops --field-selector reason=SuccessfulRescale > reports/validation-hpa-events.txt`

**(2026-05-28) Scale-up + scale-down assertions** — added to postflight:
1. **Scale-up**: at least one `SuccessfulRescale` event with the api or ai-worker deployment going **above** baseline replicas during steady (api 2→3+, ai-worker 2→3).
2. **Scale-down**: at least one `SuccessfulRescale` event with replicas **decreasing** within 10 min after rampdown starts. If we don't see this, HPA stabilization is misconfigured or load wasn't sustained long enough — both are bugs.
3. **End state**: replica counts return to HPA `minReplicas` within 10 min of test end (api=2, ai-worker=2). Run held open by `wait-for-scaledown.sh` for up to 10 min before postflight emits its final report.

- **Test**: scenario file syntax check via `k6 inspect scenarios/validation.js`
- **Demo**: ~22-min run completes; report shows api scaled 2→3 (or higher) during steady AND scaled back to 2 during rampdown, ai-worker similarly

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
  2. Source the Prometheus remote-write credentials from `~/.k6/prom.env`
     (issue #5 above) so k6 can push metrics into the cluster's Prometheus.
     If the file isn't present, fall back to local-only output and emit a
     warning — runs still work, but Grafana won't have the load overlay.
  3. `K6_PROMETHEUS_RW_SERVER_URL=… k6 run --out experimental-prometheus-rw \
     --tag testid=$(date +%Y%m%d-%H%M%S) \
     --out json=reports/<name>-raw.json \
     --summary-export=reports/<name>-summary.json \
     scenarios/<name>.js | tee reports/<name>-stdout.txt`
  4. `bash postflight.sh > reports/<name>-postflight.md`
  5. Generate consolidated `reports/<name>-report.md` with: scenario config, k6 summary, HPA timeline, pod restart count, DB stats, screenshots/links to Grafana dashboards (filtered by `testid`), and the JetStream lag time-series from postflight.
- Exit code = k6's exit code (so CI / shell can check)
- **Test**: `bash scripts/run-validation.sh --duration 60s` (override for quick smoke of harness)
- **Demo**: produces a single markdown report file the user can open and read top-to-bottom; Grafana link drops you into the run's exact time window with `testid` pre-filtered

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

  **(2026-05-28, issue #6)** The original budget said "any Go service growing
  > 50 MB/hr is suspect". That's an absolute bar that doesn't make sense
  when offered load is constant: a healthy service running 0.28 ops/sec
  shouldn't grow at all, and a real leak would be visible at any rate.
  Reworked as throughput-relative shape assertions — what we actually want
  is "memory should be flat when ops/sec is constant":

  | Signal | Healthy shape | Leak shape |
  |---|---|---|
  | API heap (`go_memstats_heap_inuse_bytes`) | Flat ±10% over the 2h run, with the constant 0.28 ops/sec offered. Cleanup goroutine drops idempotency_keys hourly — visible as a small sawtooth, not a trend. | Monotonic upward trend, no sawtooth from cleanup, OR a sudden step that doesn't recover. |
  | AI-worker RSS (`process_resident_memory_bytes`) | Flat — Python doesn't return memory to the OS but mock workload allocates nothing significant. | Steady upward drift; investigate `gc.get_objects()` count via /metrics. |
  | Outbox publisher heap | Flat. Two-phase claim doesn't accumulate state — every batch reclaimed is fully released after publish. | Drift means leader elected but never released claimed-rows reference (B-07 fix bug). |
  | DB pool checkout count (`go_sql_stats_open_connections`) | Bounded by pool config; should oscillate between 0 and pool max | Saturates at max and stays — connection leak. |
  | NATS / JetStream stream growth (`nats_stream_total_bytes`) | Bounded by `MaxAge=7d` + `MaxBytes=1GiB` retention | Unbounded growth means retention not honored. |
  | MinIO bucket size | Soak generates ~3,600 small uploads (~450 KB total); harmless | n/a (cosmetic) |

- After soak, leave the cluster running 5 minutes idle then sample again — confirms metrics return to baseline (heap drops back, pool empties, lag goes to 0).

- **Cross-check**: the ratio `(heap delta) / (total iterations)` should be ≈ 0 ± noise, NOT a stable positive number. If 7,200 iterations leaves heap +50 MB, that's ~7 KB per iteration — small enough to look noisy on the dashboard but a real leak in aggregate. Postflight calculates and prints this ratio explicitly.

- Generate `reports/soak-report.md` with same structure as validation, plus a "Findings" section that names each shape from the table above as PASS / FAIL / SUSPECT and links to the relevant Grafana panel for the run's time window.

- **Test**: report exists with all sections populated
- **Demo**: open `reports/soak-report.md`; it shows latency time-series, replica count over time, no leaks signal correlated with load

### Task 12: Final summary + commit

- Single combined report at `load-tests/RESULTS.md` linking validation + soak findings
- Commit `load-tests/` directory (without `reports/` artifacts beyond a single representative run if user wants reference) with a clear commit message
- Add a section to the project's main `README.md` pointing to `load-tests/README.md`
- **Test**: `bash load-tests/scripts/run-validation.sh --duration 30s` re-runs cleanly from a fresh git clone of the repo
- **Demo**: a new contributor can clone the repo and reproduce the load test

### Task 13: Failure injection / chaos (added 2026-05-28, issue #7)

> **Why** — the recent backend refactors (B-02 `SELECT ... FOR UPDATE` on
> lot transitions, B-05 single-tx capacity decrement, B-07 outbox two-phase
> claim with reset-on-startup, AI-worker pull consumer with multi-replica
> work distribution) are all *recovery* features. The validation and soak
> runs prove the system is stable under steady traffic — they don't prove
> the recovery code paths actually work. Chaos run does that.

This task runs **after** validation and soak pass clean, and **on top of**
a low-rate steady-state load (5 VUs, similar to soak's offered rate) so
mid-flight failures actually hit work in progress. The load is the carrier;
the chaos is the test.

#### Three injection scenarios (run sequentially, one per script invocation)

##### 13a. AI worker mid-batch kill — verifies pull-consumer redelivery

```bash
# scripts/chaos-ai-worker.sh
#  1. Start a 5-VU steady load via k6 in a background process.
#  2. Wait 30s for queue to form.
#  3. Pick a random ai-worker pod; record its name.
#  4. `kubectl delete pod <name> --grace-period=0 --force`
#     — simulates an OOM kill, no graceful drain.
#  5. Wait 60s; sample NATS consumer state and DB.
#  6. Assertions:
#     - In-flight messages at kill time were redelivered (NATS
#       `redelivered` counter increased by ≥ MAX_INFLIGHT for that pod).
#     - No qc_jobs left in PROCESSING for > ack_wait + 30s grace.
#     - All lots that entered AI_PROCESSING during the run reach
#       QC_REVIEW or later by end-of-script.
#     - No duplicate qc_results rows (ON DUPLICATE KEY UPDATE working).
#  7. Stop the k6 background process; emit findings to
#     reports/chaos-ai-worker.md.
```

Expected behavior (the thing we're proving): the killed pod's outstanding
acks expire after `ack_wait=120s` and JetStream redelivers each message.
Surviving pods (HPA min=2) pick up the redelivery and process. Ratio of
duplicate processing should be exactly `(messages in-flight at kill) / (total messages)`.

##### 13b. Outbox publisher lease handoff — verifies B-07 reset-on-startup

```bash
# scripts/chaos-outbox-publisher.sh
#  1. 5-VU steady load (same as 13a).
#  2. Wait 30s; check outbox_events table — should have rows in
#     PENDING/PUBLISHING/PUBLISHED.
#  3. Snapshot current PUBLISHING count: P1.
#  4. `kubectl delete pod -l app=simaops-outbox-publisher
#       --grace-period=0 --force` — kills the leader mid-batch.
#  5. Wait for leader election handoff (~5s lease TTL +
#     election cycle).
#  6. Verify in logs of new leader pod:
#     - "starting leader election" / "Successfully acquired lease"
#     - "recovered stuck PUBLISHING rows" with count >= 0
#       (count == P1 if kill was mid-batch, 0 if kill was at idle)
#  7. After 30s, snapshot outbox state again. Assertions:
#     - All previously-PUBLISHING rows now PUBLISHED (or back in
#       PENDING with retry_count++ then PUBLISHED next cycle).
#     - No row stuck in PUBLISHING after 60s.
#     - No row in FAILED that wasn't already failing before the kill.
#     - JetStream stream message count increased by exactly the
#       number of PENDING rows that got published — no duplicates
#       on the stream (Nats-Msg-Id dedup on JetStream is the safety
#       net for in-flight publishes the killed leader did before
#       crashing but after sending).
```

##### 13c. NATS unreachable for 30s — verifies reconnect + outbox backpressure

```bash
# scripts/chaos-nats.sh
#  1. 5-VU steady load.
#  2. Wait 30s.
#  3. Apply a NetworkPolicy that blocks egress to nats.platform from
#     the simaops namespace, OR scale `nats-0` StatefulSet to 0
#     replicas — simpler and reversible. We use the latter.
#  4. Hold for 30s. Observe:
#     - API: keeps accepting writes (outbox decouples NATS from API
#       writes). DB writes succeed. /readyz remains 200.
#     - Outbox publisher: tries to publish, fails, releases rows
#       back to PENDING with retry_count++. Logs should show the
#       new "publish failed, releasing to PENDING for retry"
#       message.
#     - AI worker: fetch loop times out / errors. Pods stay alive
#       (no panic), reconnect attempts visible in logs.
#  5. Restore NATS: `kubectl scale statefulset nats -n platform
#     --replicas=1`. Wait 30s.
#  6. Assertions:
#     - Within 60s of NATS recovery, all PENDING outbox rows from
#       the outage window have status=PUBLISHED.
#     - All QC jobs created during the outage eventually reach
#       AI_COMPLETED (eventually = within ack_wait * max_deliver
#       worst case = 120s × 4 = 8 min).
#     - No outbox rows in FAILED with retry_count > maxRetries
#       caused by the outage (outage wasn't long enough — but we
#       check anyway because the threshold is 10 retries × 500ms
#       polls = 5s; a 30s outage exceeds that, so retry_count
#       should be observed climbing then dropping back to 0 after
#       successful publish).
#     - No data loss: the count of qc.job.created events emitted
#       during the run equals the count successfully published
#       (sum from API audit logs vs NATS stream sequence delta).
```

#### Task 13 outputs

- `scripts/chaos-{ai-worker,outbox-publisher,nats}.sh` — three independent
  scripts, each idempotent and self-contained.
- `reports/chaos-<scenario>-<timestamp>.md` for each run.
- A combined `reports/chaos-summary.md` listing pass/fail per scenario
  with grafana time-window links.

#### Pass criteria for the chaos task

All three scenarios must pass independently. A failure in any is treated
as a regression of the corresponding refactor (B-07, AI-worker pull
mode, NATS reconnect/dedup) — investigate before promoting changes.

- **Test**: `bash scripts/chaos-ai-worker.sh` (and the other two) each run
  to completion and emit a green report. Run order doesn't matter.
- **Demo**: open `reports/chaos-summary.md` — three rows, all PASS, with
  links to the time-windows in Grafana that show the failure injection
  and recovery.

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
3. ~~Should the runner push k6 metrics to Prometheus remote-write~~ **(2026-05-28, issue #5: decided YES — see "k6 → Prometheus" section below.)**

## k6 → Prometheus remote-write (decided 2026-05-28)

We push k6 metrics into the cluster's Prometheus so Grafana shows the load
profile (RPS / VUs / latency) overlaid on the app's own metrics on a single
timeline. Without this, postmortem requires eyeballing two separate windows
and aligning timestamps by hand — slow and error-prone.

### Endpoint

The cluster's Prometheus instance accepts `remote_write` at:

```
https://prometheus.161.118.244.229.sslip.io/api/v1/write
```

This ingress doesn't exist today — we'll add it in Task 1 alongside the
harness setup. It points at `kube-prometheus-stack-prometheus.observability:9090`
with `--web.enable-remote-write-receiver` enabled (already set in our
`kube-prometheus-stack` Helm values; verify in pre-flight).

### Auth

- Basic auth — same `admin:prom-operator` as Grafana (already provisioned in
  `kube-prometheus-stack` defaults). Out of band for non-prod; in production
  this would be Kubernetes-issued tokens or OIDC-bridged.
- Credentials live in `~/.k6/prom.env` (gitignored), exported via `direnv` /
  `source` before running. The repo never carries the secret.

### k6 invocation

```bash
K6_PROMETHEUS_RW_SERVER_URL=https://prometheus.161.118.244.229.sslip.io/api/v1/write \
K6_PROMETHEUS_RW_TREND_STATS=p(95),p(99),min,max,avg \
K6_PROMETHEUS_RW_PUSH_INTERVAL=5s \
K6_PROMETHEUS_RW_INSECURE_SKIP_TLS_VERIFY=true \
K6_PROMETHEUS_RW_HEADERS_AUTHORIZATION="Basic $(echo -n admin:prom-operator | base64)" \
k6 run --out experimental-prometheus-rw scenarios/validation.js
```

The `K6_*` env vars are k6's native names — no flag wrangling. Push interval
of 5s matches Prometheus's default 30s scrape interval comfortably.

### Tags exposed to Grafana

k6 tags every series it pushes with `testid` (build via `--tag testid=…`),
`scenario` (smoke / validation / soak), `phase` (rampup / steady /
rampdown), and `rpc` (create_lot / review_qc / …). All four become
Prometheus labels — Grafana variables work over them out of the box. The
runner scripts inject `testid=$(date +%Y%m%d-%H%M%S)` so concurrent runs
don't collide.

### Dashboard

A starter dashboard JSON ships at
`load-tests/grafana/k6-overlay-dashboard.json` (created in Task 1) — has
two rows: **k6 view** (RPS, VUs, p95) and **app overlay** (api request
rate, ai-worker job rate, JetStream lag). Same time picker, same
`testid` filter. The dashboard is provisioned alongside the existing
ones — see `deploy/helm/simaops-api/templates/grafana-dashboard-*.yaml`
for the pattern.

---

## To Resume

When you're ready to actually run the tests, say something like:
- "Run the load test plan starting at Task 1" — start the full implementation
- "Just do Tasks 1–4 for now" — partial implementation
- "Skip to running the validation scenario" — assumes harness exists
- "Run only the chaos task" — assumes harness + steady-state scenario
  exist, runs Task 13a/13b/13c (issue #7)

The plan above is the source of truth. Open this file in your editor to review or amend before starting.
