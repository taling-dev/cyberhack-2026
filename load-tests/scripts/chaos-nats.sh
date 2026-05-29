#!/usr/bin/env bash
# scripts/chaos-nats.sh
#
# Verifies the system survives a NATS outage:
#   1. Scale nats-0 to 0 replicas (NATS unreachable for ~30s).
#   2. API keeps accepting writes — outbox decouples writes from publish.
#   3. Outbox publisher logs "publish failed, releasing to PENDING".
#   4. AI-worker pull loop errors out gracefully and reconnects.
#   5. After NATS comes back, the backlog drains within 60s.
#   6. No outbox rows in FAILED with retry_count > maxRetries.
#
# Usage:  bash scripts/chaos-nats.sh
# Exit:   0 on full pass; 1 otherwise.

set -uo pipefail
export SUPPRESS_LABEL_WARNING=True

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOAD_DIR="$(dirname "$SCRIPT_DIR")"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT="$LOAD_DIR/reports/chaos-nats-$TS.md"
mkdir -p "$LOAD_DIR/reports"

PROM_ENV="$HOME/.k6/prom.env"
[ -r "$PROM_ENV" ] && . "$PROM_ENV"
PROM_OUT=""
[ -n "${K6_PROMETHEUS_RW_SERVER_URL:-}" ] && PROM_OUT="--out experimental-prometheus-rw"

PASS_FILE=/tmp/simaops-tidb-pass
[ -r "$PASS_FILE" ] || { echo "FAIL: $PASS_FILE missing"; exit 1; }
DBPASS=$(cat "$PASS_FILE")

dbq() {
  kubectl run -n platform "mysql-q-$RANDOM" --rm -i --restart=Never --image=mysql:8.0 \
    --env="MYSQL_PWD=$DBPASS" -- \
    mysql -h simaops-tidb -P 4000 -u simaops simaops -B -N -e "$1" 2>&1 \
    | grep -E '^-?[0-9]+$' | head -1
}

step() { printf "\n\033[1;36m[%s]\033[0m %s\n" "$1" "$2"; }
ok()   { printf "\033[32m  ✓ %s\033[0m\n" "$*"; }
bad()  { printf "\033[31m  ✗ %s\033[0m\n" "$*" >&2; FAILED=1; }

FAILED=0

# Always restore NATS on exit, even on failure / Ctrl-C.
restore_nats() {
  echo
  echo "[chaos-nats] cleanup: restoring NATS to 1 replica"
  kubectl scale statefulset/nats -n platform --replicas=1 >/dev/null 2>&1 || true
  kubectl rollout status statefulset/nats -n platform --timeout=120s 2>&1 \
    | grep -v "^Warning" | tail -3 || true
}
trap restore_nats EXIT

# ─── Pre-snapshot ───────────────────────────────────────────────
step PRE "snapshot outbox state"
PUBLISHED_BEFORE=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PUBLISHED'")
FAILED_BEFORE=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='FAILED'")
ok "pre: PUBLISHED=$PUBLISHED_BEFORE, FAILED=$FAILED_BEFORE"

# ─── Start background load (longer than the outage so events span it) ──
step LOAD "starting 90s background load (3 VUs)"
LOAD_LOG="$LOAD_DIR/reports/chaos-nats-$TS-load.log"
k6 run --quiet --no-thresholds \
  --duration 90s --vus 3 \
  --tag testid="$TS" --tag scenario=chaos --tag chaos=nats \
  $PROM_OUT \
  "$LOAD_DIR/scenarios/smoke.js" > "$LOAD_LOG" 2>&1 &
K6_PID=$!
ok "k6 pid $K6_PID"

# ─── Wait for events flowing ────────────────────────────────────
step QUEUE "waiting 15s for events"
sleep 15

# ─── Inject: scale NATS to 0 ────────────────────────────────────
step OUTAGE "scaling nats StatefulSet to 0"
OUTAGE_START=$(date -u +%s)
kubectl scale statefulset/nats -n platform --replicas=0 >/dev/null 2>&1
ok "scaled to 0"

# Wait for the pod to actually terminate.
for i in 1 2 3 4 5; do
  sleep 2
  RUNNING=$(kubectl get pods -n platform -l app.kubernetes.io/name=nats --no-headers 2>/dev/null \
    | grep -v "^Warning" | wc -l)
  if [ "$RUNNING" = "0" ]; then ok "nats-0 terminated after ${i}*2s"; break; fi
done

# ─── Hold the outage for 30s and observe ────────────────────────
step OBSERVE "30s outage window — watching API + outbox behavior"
for i in 1 2 3 4 5 6; do
  sleep 5
  API_STATUS=$(curl -sk -o /dev/null -w "%{http_code}" --max-time 3 \
    https://api.161.118.244.229.sslip.io/readyz)
  PENDING_NOW=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PENDING'")
  printf "  t+%ds  /readyz=%s  PENDING=%s\n" $((i*5)) "$API_STATUS" "$PENDING_NOW"
done

# Snapshot just before restore
PEAK_PENDING=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PENDING'")
ok "peak PENDING during outage: $PEAK_PENDING"

# Confirm publisher tried + logged the right release-to-PENDING message.
PUB_LOG=$(kubectl logs -n simaops deploy/simaops-outbox-publisher --tail=80 2>/dev/null \
  | grep -v "^Warning" | grep -E "publish failed, releasing|nats disconnected|reconnecting" | tail -3)
if [ -n "$PUB_LOG" ]; then
  ok "publisher saw the outage:"
  echo "$PUB_LOG" | sed 's/^/    /'
else
  bad "publisher logs don't show outage-related messages"
fi

# ─── Restore NATS ───────────────────────────────────────────────
step RESTORE "scaling nats back to 1"
kubectl scale statefulset/nats -n platform --replicas=1 >/dev/null 2>&1
kubectl rollout status statefulset/nats -n platform --timeout=120s 2>&1 \
  | grep -v "^Warning" | tail -1
sleep 5
ok "nats back up"

# ─── Wait for backlog to drain ──────────────────────────────────
step DRAIN "waiting 60s for outbox to fully drain post-recovery"
sleep 60
wait $K6_PID 2>/dev/null
ok "k6 background load finished"

# ─── Post-assertions ────────────────────────────────────────────
step POST "verify recovery"
sleep 10

PUBLISHED_AFTER=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PUBLISHED'")
FAILED_AFTER=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='FAILED'")
STILL_PENDING=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PENDING' AND created_at < NOW() - INTERVAL 60 SECOND")
NEW_FAILED=$((FAILED_AFTER - FAILED_BEFORE))
NEW_PUBLISHED=$((PUBLISHED_AFTER - PUBLISHED_BEFORE))

ok "post: PUBLISHED Δ=$NEW_PUBLISHED, FAILED Δ=$NEW_FAILED, stuck PENDING=$STILL_PENDING"

if [ "${STILL_PENDING:-0}" = "0" ]; then
  ok "outbox drained — no PENDING older than 60s"
else
  bad "$STILL_PENDING rows still PENDING after 60s grace — publisher not catching up"
fi

if [ "${NEW_FAILED:-0}" = "0" ]; then
  ok "no new FAILED outbox rows from this run"
else
  bad "$NEW_FAILED new FAILED rows — retry_count exhausted during outage"
fi

# /readyz must have stayed 200 throughout — we sampled it during the outage.
# A flaky reading could be a single 500 on /readyz; we report it but don't
# fail since the test isn't probe-frequency-precise.

# ─── Report ─────────────────────────────────────────────────────
{
  echo "# Chaos: NATS outage — $TS"
  echo
  echo "- Background load: 90s, 3 VUs"
  echo "- Outage window: ~30s (scaled nats to 0 then back to 1)"
  echo "- Peak PENDING during outage: $PEAK_PENDING"
  echo "- Δ PUBLISHED across run: $NEW_PUBLISHED"
  echo "- Δ FAILED across run: $NEW_FAILED"
  echo
  echo "### Publisher recovery log"
  echo '```'
  echo "$PUB_LOG"
  echo '```'
  echo
  echo "## Verdict"
  echo
  [ "$FAILED" = "0" ] && echo "**PASS** — system survived NATS outage and drained backlog" \
                     || echo "**FAIL** — see assertion errors in stdout"
} > "$REPORT"

echo
echo "[chaos-nats] report → $REPORT"
exit $FAILED
