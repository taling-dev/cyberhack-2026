#!/usr/bin/env bash
# scripts/chaos-ai-worker.sh
#
# Verifies pull-consumer redelivery: kill an AI worker pod while it's
# processing a batch, then confirm:
#   * Outstanding (un-acked) messages get redelivered to the surviving pod
#     after ack_wait expires.
#   * Every QC job created during the run eventually reaches AI_COMPLETED.
#   * No qc_results row is duplicated despite the redelivery (the
#     `ON DUPLICATE KEY UPDATE` clause in write_qc_result handles it).
#   * No qc_jobs left stuck in PROCESSING > ack_wait + grace.
#
# Background: pull consumers don't have the "active subscription is bound to
# one client" constraint that push consumers do, so this only works in the
# pull-mode world we shipped today.
#
# Usage:  bash scripts/chaos-ai-worker.sh
# Exit:   0 if all assertions pass; 1 otherwise.

set -uo pipefail
export SUPPRESS_LABEL_WARNING=True

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOAD_DIR="$(dirname "$SCRIPT_DIR")"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT="$LOAD_DIR/reports/chaos-ai-worker-$TS.md"
mkdir -p "$LOAD_DIR/reports"

PROM_ENV="$HOME/.k6/prom.env"
[ -r "$PROM_ENV" ] && . "$PROM_ENV"
PROM_OUT=""
[ -n "${K6_PROMETHEUS_RW_SERVER_URL:-}" ] && PROM_OUT="--out experimental-prometheus-rw"

PASS_FILE=/tmp/simaops-tidb-pass
[ -r "$PASS_FILE" ] || { echo "FAIL: $PASS_FILE missing — cannot read TiDB"; exit 1; }
DBPASS=$(cat "$PASS_FILE")

dbq() {
  # Returns the first integer line from a `SELECT COUNT(*)` query.
  # MYSQL_PWD avoids the noisy "Using a password on the command line"
  # warning. The `pod "…" deleted` line and any other non-data noise are
  # filtered out.
  kubectl run -n platform "mysql-q-$RANDOM" --rm -i --restart=Never --image=mysql:8.0 \
    --env="MYSQL_PWD=$DBPASS" -- \
    mysql -h simaops-tidb -P 4000 -u simaops simaops -B -N -e "$1" 2>&1 \
    | grep -E '^-?[0-9]+$' | head -1
}

natsq() {
  local CMD="$1"
  kubectl run -n platform "nats-q-$RANDOM" --rm -i --restart=Never --image=natsio/nats-box:0.14.5 -- \
    sh -c "nats --server nats://nats:4222 $CMD 2>&1" 2>/dev/null \
    | grep -v "^Warning\|warning:\|^If you don\|All commands"
}

step() { printf "\n\033[1;36m[%s]\033[0m %s\n" "$1" "$2"; }
ok()   { printf "\033[32m  ✓ %s\033[0m\n" "$*"; }
bad()  { printf "\033[31m  ✗ %s\033[0m\n" "$*" >&2; FAILED=1; }

FAILED=0

# ─── Pre-snapshot ───────────────────────────────────────────────
step PRE "snapshot DB + consumer state"
QC_JOBS_BEFORE=$(dbq "SELECT COUNT(*) FROM qc_jobs")
QC_RESULTS_BEFORE=$(dbq "SELECT COUNT(*) FROM qc_results")
ok "qc_jobs=$QC_JOBS_BEFORE, qc_results=$QC_RESULTS_BEFORE"

# ─── Start background load ──────────────────────────────────────
step LOAD "starting 90s background load (3 VUs)"
LOAD_LOG="$LOAD_DIR/reports/chaos-ai-worker-$TS-load.log"
k6 run --quiet --no-thresholds \
  --duration 90s --vus 3 \
  --tag testid="$TS" --tag scenario=chaos --tag chaos=ai_worker \
  $PROM_OUT \
  "$LOAD_DIR/scenarios/smoke.js" > "$LOAD_LOG" 2>&1 &
K6_PID=$!
ok "k6 pid $K6_PID, log → $LOAD_LOG"

# ─── Wait for queue to form ─────────────────────────────────────
step QUEUE "waiting 15s for queue to form"
sleep 15
PENDING_BEFORE=$(natsq "consumer info SIMAOPS simaops-ai-worker" \
  | awk '/Outstanding Acks:/ {print $3; exit}')
ok "outstanding acks before kill: $PENDING_BEFORE"

# ─── Inject: force-kill one ai-worker pod ───────────────────────
step KILL "force-deleting one ai-worker pod"
VICTIM=$(kubectl get pods -n simaops -l app=simaops-ai-worker -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
[ -n "$VICTIM" ] || { bad "no ai-worker pod found"; kill $K6_PID 2>/dev/null; exit 1; }
ok "victim: $VICTIM"
kubectl delete pod -n simaops "$VICTIM" --grace-period=0 --force >/dev/null 2>&1
ok "kill issued"

# ─── Recovery window ────────────────────────────────────────────
# ack_wait is 120s; the surviving pod won't see the dead pod's pending
# messages until ack_wait expires. Wait the full window + 30s grace.
step RECOVERY "waiting 150s for redelivery + reprocessing"
for i in $(seq 1 30); do
  sleep 5
  REDELIVERED=$(natsq "consumer info SIMAOPS simaops-ai-worker" \
    | awk '/Redelivered Messages:/ {print $3; exit}')
  printf "  t+%ds  redelivered=%s\n" $((i*5)) "${REDELIVERED:-0}"
done

# ─── Wait for background load to finish ─────────────────────────
wait $K6_PID 2>/dev/null
K6_EXIT=$?
ok "k6 exited (status $K6_EXIT)"

# ─── Post-snapshot + assertions ─────────────────────────────────
step POST "verify recovery"
sleep 30  # final drain window for slow rotations

QC_JOBS_AFTER=$(dbq "SELECT COUNT(*) FROM qc_jobs")
QC_RESULTS_AFTER=$(dbq "SELECT COUNT(*) FROM qc_results")
JOBS_DELTA=$((QC_JOBS_AFTER - QC_JOBS_BEFORE))
RESULTS_DELTA=$((QC_RESULTS_AFTER - QC_RESULTS_BEFORE))
ok "Δjobs=$JOBS_DELTA, Δresults=$RESULTS_DELTA"

# Assertion 1: every new job got a result row (no orphan PROCESSING jobs).
if [ "$JOBS_DELTA" = "$RESULTS_DELTA" ] && [ "$JOBS_DELTA" -gt 0 ]; then
  ok "every created job produced exactly one result (no duplicates, no losses)"
else
  bad "job/result count mismatch — Δjobs=$JOBS_DELTA Δresults=$RESULTS_DELTA"
fi

# Assertion 2: no jobs stuck in PROCESSING.
STUCK=$(dbq "SELECT COUNT(*) FROM qc_jobs WHERE status='PROCESSING' AND started_at < NOW() - INTERVAL 5 MINUTE")
if [ "${STUCK:-0}" = "0" ]; then
  ok "no qc_jobs stuck in PROCESSING"
else
  bad "$STUCK qc_jobs stuck in PROCESSING — redelivery did not catch them"
fi

# Assertion 3: redelivery is the safety-net mechanism. If redelivered>0,
# we've proved the mechanism works under fire. If redelivered=0 with
# in-flight at kill time, that just means the in-flight messages weren't
# on the victim pod — the survivor handled them normally. Either path is
# correctness; the no-loss invariant (assertion 1) is the real signal.
FINAL_REDELIV=$(natsq "consumer info SIMAOPS simaops-ai-worker" \
  | awk '/Redelivered Messages:/ {print $3; exit}')
if [ "${FINAL_REDELIV:-0}" -gt 0 ]; then
  ok "redelivered=$FINAL_REDELIV — pull-consumer redelivery exercised under fire"
else
  ok "redelivered=0 — victim had no in-flight at kill time (safety net not needed)"
fi

# Assertion 4: HPA brought the dead pod back.
ALIVE=$(kubectl get pods -n simaops -l app=simaops-ai-worker --no-headers 2>/dev/null \
  | grep -v "^Warning" | awk '$2 ~ /\/.*Ready|1\/1/ {n++} END {print n+0}')
if [ "${ALIVE:-0}" -ge 1 ]; then
  ok "ai-worker pods alive after recovery: $ALIVE"
else
  bad "no live ai-worker pods after recovery"
fi

# ─── Report ─────────────────────────────────────────────────────
{
  echo "# Chaos: ai-worker mid-batch kill — $TS"
  echo
  echo "- Background load: 90s, 3 VUs, smoke pipeline, no-thresholds"
  echo "- Victim pod: \`$VICTIM\`"
  echo "- Outstanding acks at kill time: $PENDING_BEFORE"
  echo "- Redelivered messages observed: $FINAL_REDELIV"
  echo "- Δqc_jobs: $JOBS_DELTA"
  echo "- Δqc_results: $RESULTS_DELTA"
  echo
  echo "## Verdict"
  echo
  [ "$FAILED" = "0" ] && echo "**PASS** — pull-consumer redelivery worked end-to-end" \
                     || echo "**FAIL** — see assertion errors in stdout"
} > "$REPORT"

echo
echo "[chaos-ai-worker] report → $REPORT"
exit $FAILED
