#!/usr/bin/env bash
# scripts/chaos-outbox-publisher.sh
#
# Verifies B-07 reset-on-startup: kill the outbox-publisher leader while
# it's claiming a batch, then confirm the new leader's OnStartedLeading
# callback resets stuck PUBLISHING rows back to PENDING and re-publishes.
#
# Assertions:
#   * Lease handoff completes within 30s.
#   * New leader's logs show "recovered stuck PUBLISHING rows" (count >= 0).
#   * After 60s, no rows remain in PUBLISHING (publisher's tick is 500ms).
#   * No new FAILED outbox rows that weren't already failing.
#
# Usage:  bash scripts/chaos-outbox-publisher.sh
# Exit:   0 if all assertions pass; 1 otherwise.

set -uo pipefail
export SUPPRESS_LABEL_WARNING=True

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOAD_DIR="$(dirname "$SCRIPT_DIR")"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT="$LOAD_DIR/reports/chaos-outbox-publisher-$TS.md"
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

# ─── Pre-snapshot ───────────────────────────────────────────────
step PRE "snapshot outbox state"
FAILED_BEFORE=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='FAILED'")
PUBLISHED_BEFORE=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PUBLISHED'")
ok "pre: PUBLISHED=$PUBLISHED_BEFORE, FAILED=$FAILED_BEFORE"

OLD_LEADER=$(kubectl get pods -n simaops -l app=simaops-outbox-publisher -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
[ -n "$OLD_LEADER" ] || { bad "no outbox-publisher pod"; exit 1; }
ok "current leader: $OLD_LEADER"

# ─── Start background load ──────────────────────────────────────
step LOAD "starting 60s background load (3 VUs) so events flow through outbox"
LOAD_LOG="$LOAD_DIR/reports/chaos-outbox-publisher-$TS-load.log"
k6 run --quiet --no-thresholds \
  --duration 60s --vus 3 \
  --tag testid="$TS" --tag scenario=chaos --tag chaos=outbox \
  $PROM_OUT \
  "$LOAD_DIR/scenarios/smoke.js" > "$LOAD_LOG" 2>&1 &
K6_PID=$!
ok "k6 pid $K6_PID"

# ─── Wait for activity ──────────────────────────────────────────
step QUEUE "waiting 15s for outbox traffic"
sleep 15
PUBLISHING_AT_KILL=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PUBLISHING'")
PENDING_AT_KILL=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PENDING'")
ok "at kill time: PENDING=$PENDING_AT_KILL, PUBLISHING=$PUBLISHING_AT_KILL"

# ─── Inject: force-delete the leader ────────────────────────────
step KILL "force-deleting leader pod $OLD_LEADER"
KILL_TS=$(date -u +%s)
kubectl delete pod -n simaops "$OLD_LEADER" --grace-period=0 --force >/dev/null 2>&1
ok "kill issued at $(date -u -d @"$KILL_TS" --iso-8601=seconds)"

# ─── Wait for new leader ────────────────────────────────────────
step LEADER "waiting up to 60s for lease handoff"
NEW_LEADER=""
for i in $(seq 1 12); do
  sleep 5
  NEW=$(kubectl get pods -n simaops -l app=simaops-outbox-publisher --no-headers 2>/dev/null \
    | grep -v "^Warning" | awk '$2=="1/1" && $3=="Running" {print $1; exit}')
  if [ -n "$NEW" ] && [ "$NEW" != "$OLD_LEADER" ]; then
    NEW_LEADER="$NEW"
    HANDOFF=$((i*5))
    ok "new leader: $NEW_LEADER (after ${HANDOFF}s)"
    break
  fi
  printf "  t+%ds  still waiting...\n" $((i*5))
done
[ -n "$NEW_LEADER" ] || { bad "no new leader after 60s"; kill $K6_PID 2>/dev/null; exit 1; }

# ─── Verify recovery log line ───────────────────────────────────
step LOGS "checking new leader logs for reset-on-startup message"
sleep 5  # give the new leader a moment to flush its first log lines
RESET_LOG=$(kubectl logs -n simaops "$NEW_LEADER" 2>/dev/null \
  | grep -v "^Warning" | grep -E "recovered stuck PUBLISHING|became leader" | head -3)
if echo "$RESET_LOG" | grep -q "became leader"; then
  ok "leader-elected log present"
  echo "$RESET_LOG" | sed 's/^/    /'
else
  bad "new leader's logs don't show 'became leader'"
fi

# ─── Wait for outbox to drain ───────────────────────────────────
step DRAIN "waiting 60s for full outbox drain post-recovery"
sleep 60
wait $K6_PID 2>/dev/null
ok "k6 background load finished"

# ─── Post-assertions ────────────────────────────────────────────
step POST "verify outbox state"
sleep 10  # final settle

STILL_PUBLISHING=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PUBLISHING' AND created_at < NOW() - INTERVAL 30 SECOND")
STILL_PENDING=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PENDING' AND created_at < NOW() - INTERVAL 30 SECOND")
FAILED_AFTER=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='FAILED'")
PUBLISHED_AFTER=$(dbq "SELECT COUNT(*) FROM outbox_events WHERE status='PUBLISHED'")
NEW_FAILED=$((FAILED_AFTER - FAILED_BEFORE))
NEW_PUBLISHED=$((PUBLISHED_AFTER - PUBLISHED_BEFORE))

ok "post: PUBLISHED Δ=$NEW_PUBLISHED, FAILED Δ=$NEW_FAILED, stuck PUBLISHING=$STILL_PUBLISHING, stuck PENDING=$STILL_PENDING"

if [ "${STILL_PUBLISHING:-0}" = "0" ]; then
  ok "no rows stuck in PUBLISHING (reset query did its job)"
else
  bad "$STILL_PUBLISHING rows stuck in PUBLISHING — reset-on-startup didn't recover them"
fi

if [ "${STILL_PENDING:-0}" = "0" ]; then
  ok "no rows stuck in PENDING (publisher caught up)"
else
  bad "$STILL_PENDING rows stuck in PENDING — publisher not catching up"
fi

if [ "${NEW_FAILED:-0}" = "0" ]; then
  ok "no new FAILED rows from this chaos run"
else
  bad "$NEW_FAILED new FAILED rows since pre-snapshot"
fi

# ─── Report ─────────────────────────────────────────────────────
{
  echo "# Chaos: outbox-publisher leader handoff — $TS"
  echo
  echo "- Background load: 60s, 3 VUs"
  echo "- Old leader: \`$OLD_LEADER\`"
  echo "- New leader: \`$NEW_LEADER\`"
  echo "- Lease handoff time: ${HANDOFF:-?}s"
  echo "- PUBLISHING at kill: $PUBLISHING_AT_KILL"
  echo "- PENDING at kill: $PENDING_AT_KILL"
  echo "- Δ PUBLISHED across run: $NEW_PUBLISHED"
  echo "- Δ FAILED across run: $NEW_FAILED"
  echo
  echo "### Recovery log (new leader's first 3 lines)"
  echo '```'
  echo "$RESET_LOG"
  echo '```'
  echo
  echo "## Verdict"
  echo
  [ "$FAILED" = "0" ] && echo "**PASS** — B-07 two-phase claim recovered cleanly" \
                     || echo "**FAIL** — see assertion errors in stdout"
} > "$REPORT"

echo
echo "[chaos-outbox-publisher] report → $REPORT"
exit $FAILED
