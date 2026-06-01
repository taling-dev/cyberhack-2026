# SARI (Sima Arome Resource Intelligence) — Load Test Harness

k6 + Prometheus remote-write harness that drives the live cluster
through the full lot → QC → review → assign pipeline. Three scenarios
ship: smoke, validation, soak. A fourth chaos task injects failures
into the running cluster (see [PLAN.md](./PLAN.md) Task 13).

> The plan is at [`PLAN.md`](./PLAN.md). It's the source of truth for
> goals, thresholds, and decisions. This README only documents *how to
> run* the harness once it's in place.

## Prerequisites

Verify each before running anything:

| Item | Check |
|---|---|
| k6 binary | `k6 version` (need ≥ v1.0; we developed against v2.0.0) |
| kubectl context | points at the OKE cluster — `kubectl get ns` shows `simaops`, `platform`, `observability` |
| Prometheus creds | `~/.k6/prom.env` exists with `K6_PROMETHEUS_RW_*` vars (see PLAN.md → "k6 → Prometheus remote-write") |
| Cluster | all 6 simaops pods Running; preflight script verifies this automatically |
| TiDB password | `/tmp/simaops-tidb-pass` (load from the `simaops-infra-creds` Secret — see "Setup" below) |

## Setup

The preflight and postflight scripts read the TiDB password from
`/tmp/simaops-tidb-pass`. Hydrate it once per laptop:

```bash
kubectl get secret simaops-infra-creds -n simaops \
    -o jsonpath='{.data.TIDB_PASSWORD}' | base64 -d > /tmp/simaops-tidb-pass
chmod 600 /tmp/simaops-tidb-pass
```

## Layout

```
load-tests/
├── PLAN.md                            # decisions, thresholds, task breakdown
├── README.md                          # ← you are here
├── lib/
│   ├── auth.js                        # per-VU token cache, lazy refresh
│   ├── helpers.js                     # uuid, randomLotData, qc image bytes
│   └── pipeline.js                    # one full E2E iteration
├── scenarios/
│   ├── smoke.js                       # 1 VU × 1 min — sanity
│   ├── validation.js                  # 20 VUs × 22 min — HPA test
│   └── soak.js                        # 5 VUs × 2 hr — stability
├── fixtures/
│   └── qc-image.jpg                   # tiny placeholder JPG
├── grafana/
│   └── k6-overlay-dashboard.json      # k6 + app metrics on one timeline
├── scripts/
│   ├── preflight.sh                   # cluster + DB + bucket checks
│   ├── postflight.sh                  # stuck-row + JetStream lag checks
│   ├── run-smoke.sh                   # full smoke run with reporting
│   ├── run-validation.sh              # full validation run with reporting
│   ├── run-soak.sh                    # full soak run with reporting
│   ├── chaos-ai-worker.sh             # Task 13a — kill ai-worker mid-batch
│   ├── chaos-outbox-publisher.sh      # Task 13b — force lease handoff
│   └── chaos-nats.sh                  # Task 13c — NATS unreachable for 30s
└── reports/                           # gitignored output directory
```

## Running

### Smoke (1 min, sanity check)

```bash
bash scripts/run-smoke.sh
```

Expected output: green k6 summary, `reports/smoke-<timestamp>-report.md`,
no stuck rows, no pod restarts. Use this every time the harness changes.

### Validation (~22 min, HPA scaling test)

```bash
bash scripts/run-validation.sh
```

Drives 20 VUs into the system, hold 20 min, ramp down. Exercises
api HPA `2→3+` and ai-worker HPA `2→3`, then waits 10 min for
scale-down so we observe both directions. Postflight asserts the
JetStream backlog cleared and no rows are stuck.

### Soak (2 hr, leak / stability test)

```bash
bash scripts/run-soak.sh
```

5 VUs at constant ~0.28 ops/sec for 2 hr. Heap should be flat,
pool checkouts bounded, no pod restarts. Postflight calculates
the `(heap delta) / (iterations)` ratio explicitly — a real leak
shows up as a stable positive number even when it looks noisy.

### Chaos (Task 13)

Run after validation + soak pass clean. Each script is idempotent
and runs against a low-rate steady load it manages itself.

```bash
bash scripts/chaos-ai-worker.sh        # kills ai-worker pod mid-batch
bash scripts/chaos-outbox-publisher.sh # forces leader handoff
bash scripts/chaos-nats.sh             # blocks NATS for 30s
```

## Conventions

- **All scripts source** `~/.k6/prom.env` if present. Without it, runs
  still work but Grafana won't show the load overlay.
- **Reports land in** `reports/<scenario>-<UTC-iso8601>-*.{md,json,txt}`.
  The `reports/` directory is gitignored.
- **k6 tags every series** with `testid` (timestamp), `scenario`
  (smoke / validation / soak / chaos), `phase` (rampup / steady /
  rampdown), and `rpc` (create_lot / review_qc / …). Filter Grafana
  by `testid` to see one run in isolation.
- **Per-VU independent auth sessions** — `lib/auth.js` gives every VU
  its own Keycloak session per user. The realm enforces 5-min access
  tokens, single-use rotating refresh tokens
  (`revokeRefreshToken=true`, `refreshTokenMaxReuse=0`), and
  brute-force protection (`failureFactor=5`, 1s quick-login window).
  Two earlier strategies failed because they SHARED one token set via
  `setup()`: the first VU to refresh consumed the single-use refresh
  token, the other VUs 401'd and fell back to `password` grants, and
  the resulting concurrent same-user logins tripped brute-force
  detection. The current design establishes each VU's session lazily
  (no shared tokens), refreshes with that VU's OWN rotating refresh
  token, and retries the initial `password` grant up to 4× with >1.3s
  spacing (above the 1s quick-login window) to absorb the rare ramp-up
  collision. Verified under 20-VU load: 100 password + 100 refresh
  grants, 0 failures. Counters in the summary:
  `auth_token_password_grants`, `auth_token_refresh_grants`,
  `auth_token_grant_failures` (expected 0).
- **Warehouse capacity reset** — preflight section 9 sets every
  `warehouse_locations.capacity` row to 1,000,000 via
  `db/seed/reset-warehouse-capacity.sql`. The seeded capacity (92
  total slots) was being drained by the first ~92 successful
  `AssignSlot` calls in any sustained run, after which every
  `RecommendSlot` returned an empty list and the rest of the run
  failed at the warehouse step. Preflight refuses to proceed if the
  table contains anything other than the 12 seeded location codes.
- **PASS/FAIL verdict** — `run-*.sh` scripts pass `$K6_EXIT` to
  `postflight.sh` as the third argument. The Verdict block now flags
  any of: non-zero k6 exit, container restarts, or lots created
  during this run that are stuck >10 min in `AI_PROCESSING`,
  `QC_REVIEW`, or `QC_APPROVED`.
- **No cleanup by default.** After a soak you'll have ~7,200 lots.
  `scripts/cleanup.sh` (TODO if you want it) would `TRUNCATE` the
  test tables; PLAN.md Open Question #1 covers the trade-off.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `auth.js failed: 401` on token endpoint | A demo user got locked out (5 wrong passwords in 60s) | Wait 60s, check there's no other harness running. With the `setupAuth()` pre-warm + per-VU jittered leeway this should now be very rare; if it persists, look for a stale harness still hitting Keycloak. |
| `TiDB password file missing at /tmp/simaops-tidb-pass` | Password file not hydrated on this laptop | Run the `kubectl get secret … TIDB_PASSWORD` command in the **Setup** section above. |
| `recommend slot ok` 0% pass rate, all `RecommendSlot` calls return empty | `warehouse_locations.capacity` exhausted from a prior run | Re-run preflight — section 9 will reset capacity to 1,000,000 per slot. |
| Postflight prints `Result: PASS` despite k6 failures | `run-*.sh` not passing `$K6_EXIT` to `postflight.sh` arg 3 | Verify the runner script is current — all three runners must invoke `postflight.sh <scenario> <iso> "$K6_EXIT"`. |
| `MinIO PUT 403 SignatureDoesNotMatch` | Time skew between laptop and cluster > 15 min | `timedatectl set-ntp on` on the laptop |
| Lots stuck in `PENDING_QC` after smoke | AI worker not consuming — check `kubectl logs -n simaops deploy/simaops-ai-worker` | Most likely the durable consumer needs to be deleted (see PLAN.md → Background note 5) |
| `lag never drops below 50` postflight failure | HPA didn't scale — check `kubectl describe hpa -n simaops simaops-ai-worker` for events | Usually means metrics-server is unhealthy; not a load-test issue |
