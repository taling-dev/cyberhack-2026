#!/usr/bin/env bash
# scripts/postflight.sh
#
# Post-run cluster sanity checks. Writes a markdown report to stdout for
# the runner to capture into reports/<scenario>-postflight.md.
#
# Usage:
#   bash scripts/postflight.sh <scenario> <run_started_iso> [k6_exit_code]
#
#   bash scripts/postflight.sh smoke 2026-05-28T05:30:00Z 0
#
# All informational checks are best-effort — we never `exit 1` from the
# data-collection sections. The verdict at the end now considers:
#   * the k6 process exit code (threshold breaches → exit 99)
#   * any container restarts since pod creation
#   * any lots stuck in transient states longer than 10 minutes that
#     were created during this run
# and prints a single Result: PASS / FAIL line consumed by the runner
# scripts and CI.

set -uo pipefail
export SUPPRESS_LABEL_WARNING=True

SCENARIO="${1:-unknown}"
RUN_STARTED="${2:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
K6_EXIT="${3:-}"
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
TIDB_PASS_FILE=/tmp/simaops-tidb-pass

cat <<EOF
# Postflight — $SCENARIO

- Run started: $RUN_STARTED
- Postflight at: $NOW
- Cluster: OKE (\`*.161.118.244.229.sslip.io\`)

## Pod state

\`\`\`
$(kubectl get pods -n simaops 2>&1 | grep -v "^Warning")
\`\`\`

### Restart count delta (since pod creation, all simaops containers)
\`\`\`
$(kubectl get pods -n simaops -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[*].restartCount}{"\n"}{end}' 2>&1 | grep -v "^Warning")
\`\`\`

## HPA state

\`\`\`
$(kubectl get hpa -n simaops 2>&1 | grep -v "^Warning")
\`\`\`

### Scaling events during the run
\`\`\`
$(kubectl get events -n simaops --field-selector reason=SuccessfulRescale --sort-by='.lastTimestamp' 2>&1 \
  | grep -v "^Warning" | tail -20)
\`\`\`

## Database state
EOF

if [ -r "$TIDB_PASS_FILE" ]; then
  PASS=$(cat "$TIDB_PASS_FILE")
  cat <<EOF

### Lots by status
\`\`\`
$(kubectl run -n platform mysql-pf-$RANDOM --rm -i --restart=Never --image=mysql:8.0 -- \
    mysql -h simaops-tidb -P 4000 -u simaops -p"$PASS" simaops -B \
    -e "SELECT status, COUNT(*) AS count FROM lots GROUP BY status ORDER BY count DESC" 2>&1 \
    | grep -v "^Warning\|warning:\|^If\|All commands")
\`\`\`

### Stuck lots (in transient state >10 min — should be 0)
\`\`\`
$(kubectl run -n platform mysql-pf-$RANDOM --rm -i --restart=Never --image=mysql:8.0 -- \
    mysql -h simaops-tidb -P 4000 -u simaops -p"$PASS" simaops -B \
    -e "SELECT id, lot_number, status, updated_at FROM lots
        WHERE status IN ('AI_PROCESSING','QC_REVIEW')
        AND updated_at < NOW() - INTERVAL 10 MINUTE
        ORDER BY updated_at DESC LIMIT 20" 2>&1 \
    | grep -v "^Warning\|warning:\|^If\|All commands")
\`\`\`

### Outbox events by status
\`\`\`
$(kubectl run -n platform mysql-pf-$RANDOM --rm -i --restart=Never --image=mysql:8.0 -- \
    mysql -h simaops-tidb -P 4000 -u simaops -p"$PASS" simaops -B \
    -e "SELECT status, COUNT(*) AS count FROM outbox_events GROUP BY status" 2>&1 \
    | grep -v "^Warning\|warning:\|^If\|All commands")
\`\`\`

### Stuck outbox events (PENDING/PUBLISHING >30s — should be 0)
\`\`\`
$(kubectl run -n platform mysql-pf-$RANDOM --rm -i --restart=Never --image=mysql:8.0 -- \
    mysql -h simaops-tidb -P 4000 -u simaops -p"$PASS" simaops -B \
    -e "SELECT id, event_type, status, retry_count, created_at FROM outbox_events
        WHERE status IN ('PENDING','PUBLISHING')
        AND created_at < NOW() - INTERVAL 30 SECOND
        LIMIT 20" 2>&1 \
    | grep -v "^Warning\|warning:\|^If\|All commands")
\`\`\`

### Permanently FAILED outbox events (should be 0)
\`\`\`
$(kubectl run -n platform mysql-pf-$RANDOM --rm -i --restart=Never --image=mysql:8.0 -- \
    mysql -h simaops-tidb -P 4000 -u simaops -p"$PASS" simaops -B \
    -e "SELECT COUNT(*) AS failed_events FROM outbox_events WHERE status='FAILED'" 2>&1 \
    | grep -v "^Warning\|warning:\|^If\|All commands")
\`\`\`
EOF
else
  echo
  echo "_TiDB password not in $TIDB_PASS_FILE; skipped DB checks._"
fi

cat <<EOF

## NATS / JetStream state

### Stream and consumer
\`\`\`
$(kubectl run -n platform nats-pf-$RANDOM --rm -i --restart=Never --image=natsio/nats-box:0.14.5 -- \
    sh -c "nats --server nats://nats:4222 stream info SIMAOPS 2>&1 | grep -E 'Messages:|Bytes:|Consumers:|Last Sequence'" 2>&1 \
    | grep -v "^Warning\|warning:\|^If\|All commands\|nats-pf")
\`\`\`

### AI-worker consumer lag (Pending Acks + Unprocessed)
\`\`\`
$(kubectl run -n platform nats-pf-$RANDOM --rm -i --restart=Never --image=natsio/nats-box:0.14.5 -- \
    sh -c "nats --server nats://nats:4222 consumer info SIMAOPS simaops-ai-worker 2>&1 \
           | grep -E 'Pull Mode|Outstanding|Redelivered|Unprocessed|Active Interest'" 2>&1 \
    | grep -v "^Warning\|warning:\|^If\|All commands\|nats-pf")
\`\`\`

## Verdict

EOF

# Verdict — fail if any of:
#   * k6 returned a non-zero exit code (threshold breach)
#   * any simaops container has restarted
#   * lots created during this run are stuck in a transient state
PROBLEMS=0

if [ -n "$K6_EXIT" ]; then
  if [ "$K6_EXIT" = "0" ]; then
    echo "- ✓ k6 exit code 0 (all thresholds met)"
  else
    echo "- ❌ k6 exit code $K6_EXIT (threshold breach or runtime error)"
    PROBLEMS=$((PROBLEMS+1))
  fi
else
  echo "- ⚠️  k6 exit code not provided to postflight (caller did not pass arg 3)"
fi

RESTARTED=$(kubectl get pods -n simaops -o jsonpath='{range .items[*]}{.status.containerStatuses[*].restartCount}{" "}{end}' 2>/dev/null | tr ' ' '\n' | awk '$1+0>0' | wc -l)
if [ "$RESTARTED" -gt 0 ]; then
  echo "- ❌ $RESTARTED container restart(s) detected"
  PROBLEMS=$((PROBLEMS+1))
else
  echo "- ✓ no container restarts"
fi

# Stuck lots — only count lots created on or after the run start, so we
# don't penalize this run for orphans left behind by earlier runs.
if [ -r "$TIDB_PASS_FILE" ]; then
  PASS=$(cat "$TIDB_PASS_FILE")
  STUCK_THIS_RUN=$(kubectl run -n platform "mysql-pf-$RANDOM" --rm -i --restart=Never --image=mysql:8.0 -- \
      mysql -h simaops-tidb -P 4000 -u simaops -p"$PASS" simaops -BN \
      -e "SELECT COUNT(*) FROM lots
            WHERE status IN ('AI_PROCESSING','QC_REVIEW','QC_APPROVED')
              AND updated_at < NOW() - INTERVAL 10 MINUTE
              AND created_at >= '${RUN_STARTED//[TZ]/ }'" 2>/dev/null \
      | grep -E '^[0-9]+$' | head -1)
  STUCK_THIS_RUN=${STUCK_THIS_RUN:-?}
  if [ "$STUCK_THIS_RUN" = "?" ]; then
    echo "- ⚠️  could not query stuck-lots count (DB unreachable)"
  elif [ "$STUCK_THIS_RUN" -gt 0 ]; then
    echo "- ❌ $STUCK_THIS_RUN lot(s) created in this run still stuck (>10 min in transient state, includes QC_APPROVED without slot)"
    PROBLEMS=$((PROBLEMS+1))
  else
    echo "- ✓ no lots from this run stuck >10 min"
  fi
else
  echo "- ⚠️  TiDB password missing; stuck-lot check skipped"
fi

if [ "$PROBLEMS" -eq 0 ]; then
  echo
  echo "**Result: PASS**"
else
  echo
  echo "**Result: FAIL ($PROBLEMS issue(s))**"
fi
