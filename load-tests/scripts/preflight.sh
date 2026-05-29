#!/usr/bin/env bash
# scripts/preflight.sh
#
# Hard prerequisites for any load-test run. Exits non-zero on any failure
# so the runner aborts before consuming time + cluster resources.
#
# Checks:
#   1. kubectl context points at the OKE cluster (sees `simaops` namespace).
#   2. All 6 simaops pods Running and Ready.
#   3. All 4 ingresses (app, api, auth, grafana) respond 200/302 over HTTPS.
#   4. MinIO has the 4 expected buckets.
#   5. All 5 demo users authenticate cleanly (sequential, no lockout risk).
#   6. metrics-server returns kubectl top results — required for HPA.
#   7. All 3 HPAs report ScalingActive=True.
#   8. Records baseline replica counts to reports/preflight-<ts>.json.
#   9. Resets warehouse_locations.capacity so load runs do not exhaust
#      the (production-realistic) 92-slot seed and start failing
#      RecommendSlot mid-run.
#
# Usage: bash scripts/preflight.sh

set -euo pipefail

export SUPPRESS_LABEL_WARNING=True
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOAD_DIR="$(dirname "$SCRIPT_DIR")"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT="$LOAD_DIR/reports/preflight-$TS.json"
mkdir -p "$LOAD_DIR/reports"

green() { printf "\033[32m  ✓ %s\033[0m\n" "$*"; }
red()   { printf "\033[31m  ✗ %s\033[0m\n" "$*" >&2; }
section(){ printf "\n\033[1;36m[%s]\033[0m %s\n" "$1" "$2"; }

fail() { red "$*"; exit 1; }

# 1. kubectl context
section 1 "kubectl context"
kubectl get ns simaops -o name >/dev/null 2>&1 \
  || fail "cannot reach simaops namespace; check kubectl context"
green "namespace simaops reachable"

# 2. pod readiness
section 2 "simaops pod readiness"
NOT_READY=$(kubectl get pods -n simaops --no-headers 2>/dev/null \
  | awk '{print $2}' | grep -v "^Warning" | awk -F/ '$1!=$2 {print}' | wc -l)
if [ "$NOT_READY" -gt 0 ]; then
  kubectl get pods -n simaops 2>&1 | grep -v "^Warning"
  fail "$NOT_READY pods not ready"
fi
green "all simaops pods Ready"

# 3. ingress reachability
section 3 "ingress endpoints"
declare -A INGRESS_PATH=(
  [app]="/"
  [api]="/readyz"
  [auth]="/realms/simaops"
  [grafana]="/api/health"
)
for host in app api auth grafana; do
  url="https://${host}.161.118.244.229.sslip.io${INGRESS_PATH[$host]}"
  status=$(curl -sk -o /dev/null -w "%{http_code}" --max-time 5 "$url")
  case "$status" in
    200|302|401|403) green "$host${INGRESS_PATH[$host]} → $status" ;;
    *) fail "$host${INGRESS_PATH[$host]} returned $status" ;;
  esac
done

# 4. MinIO buckets
section 4 "MinIO buckets"
TIDB_PASS_FILE=/tmp/simaops-tidb-pass
[ -r "$TIDB_PASS_FILE" ] || fail "TiDB password file missing at $TIDB_PASS_FILE — run setup first"
EXPECTED_BUCKETS="simaops-qc-images simaops-qc-results simaops-reports simaops-model-artifacts"
# The MinIO image only ships a minimal toolset (no awk, sed, or grep).
# Get the raw `mc ls` output and parse on the laptop where awk is available.
RAW=$(kubectl exec -n platform deploy/minio -- sh -c \
  "mc alias set local http://localhost:9000 \$MINIO_ROOT_USER \$MINIO_ROOT_PASSWORD >/dev/null 2>&1 && mc ls local/ 2>/dev/null" \
  2>/dev/null | grep -v "^Warning" || true)
# `mc ls` rows look like:  [2026-05-27 03:23:19 UTC]     0B simaops-qc-images/
ACTUAL=$(echo "$RAW" | awk '{print $NF}' | tr -d '/')
for b in $EXPECTED_BUCKETS; do
  if echo "$ACTUAL" | grep -qx "$b"; then green "bucket $b"; else fail "bucket $b missing"; fi
done

# 5. demo users — sequential to avoid lockout
section 5 "demo user auth"
for u in budi siti agus dewi admin; do
  status=$(curl -sk -o /dev/null -w "%{http_code}" -X POST \
    https://auth.161.118.244.229.sslip.io/realms/simaops/protocol/openid-connect/token \
    -d "client_id=simaops-web&username=$u&password=password123&grant_type=password")
  [ "$status" = "200" ] && green "$u → 200" || fail "$u → $status (lockout?)"
  sleep 0.2
done

# 6. metrics-server
section 6 "metrics-server"
TOP=$(kubectl top pods -n simaops --no-headers 2>&1 | grep -v "^Warning" | wc -l)
[ "$TOP" -gt 0 ] && green "metrics-server returned $TOP pods" \
  || fail "metrics-server unhealthy — HPAs cannot scale"

# 7. HPA readiness
section 7 "HPA scaling-active"
for hpa in simaops-api simaops-ai-worker simaops-web; do
  active=$(kubectl get hpa -n simaops "$hpa" -o jsonpath='{.status.conditions[?(@.type=="ScalingActive")].status}' 2>/dev/null)
  [ "$active" = "True" ] && green "$hpa active" || fail "$hpa ScalingActive=$active"
done

# 8. baseline snapshot
section 8 "baseline snapshot → $REPORT"
kubectl get hpa -n simaops -o json > "$REPORT" 2>/dev/null
SUMMARY=$(kubectl get deploy -n simaops -o json \
  | python3 -c "
import json, sys
d = json.loads(sys.stdin.read())
for it in d['items']:
    print(f\"  {it['metadata']['name']}: replicas={it['spec']['replicas']}\")")
echo "$SUMMARY"
green "preflight passed (8/9 sections)"

# 9. warehouse capacity reset — pipes the SQL via stdin to a throwaway
#    mysql client pod. Without this, load runs deplete capacity within
#    ~92 successful AssignSlot calls and every subsequent RecommendSlot
#    returns an empty list. See db/seed/reset-warehouse-capacity.sql.
#
#    Safety assertion (in bash, since TiDB does not support stored
#    procedures): refuse to proceed if the warehouse_locations table
#    contains anything other than the 12 seeded codes. This prevents
#    accidental capacity overwrites in non-load-test environments.
section 9 "warehouse capacity reset"
RESET_SQL="$LOAD_DIR/../db/seed/reset-warehouse-capacity.sql"
[ -r "$RESET_SQL" ] || fail "reset SQL missing at $RESET_SQL"
PASS=$(cat "$TIDB_PASS_FILE")

EXPECTED_CODES="A-01,A-02,A-03,A-04,B-01,B-02,B-03,B-04,C-01,C-02,C-03,C-04"
ASSERTION_SQL="
SELECT COUNT(*) AS unexpected
  FROM warehouse_locations
 WHERE code NOT IN ('A-01','A-02','A-03','A-04',
                    'B-01','B-02','B-03','B-04',
                    'C-01','C-02','C-03','C-04');
SELECT COUNT(*) AS total FROM warehouse_locations;"

ASSERTION_OUT=$(kubectl run -n platform "mysql-assert-$RANDOM" \
    --rm -i --restart=Never --image=mysql:8.0 \
    --command -- mysql -h simaops-tidb -P 4000 -u simaops -p"$PASS" simaops -BN \
    -e "$ASSERTION_SQL" 2>&1)
# Filter to numeric lines only — mysql/warning/pod-status chatter is
# noise. Output is two integers, one per line: unexpected, total.
NUMERIC_LINES=$(echo "$ASSERTION_OUT" | grep -E '^[0-9]+$')
UNEXPECTED=$(echo "$NUMERIC_LINES" | sed -n '1p')
TOTAL=$(echo "$NUMERIC_LINES" | sed -n '2p')
if [ -z "$UNEXPECTED" ] || [ -z "$TOTAL" ]; then
  echo "$ASSERTION_OUT"
  fail "could not assert warehouse_locations seed signature (TiDB unreachable?)"
fi
if [ "$UNEXPECTED" -gt 0 ] || [ "$TOTAL" -ne 12 ]; then
  echo "$ASSERTION_OUT"
  fail "warehouse_locations does not match the load-test seed signature ($TOTAL rows, $UNEXPECTED unexpected codes); refusing to overwrite capacity"
fi
green "warehouse_locations seed signature OK ($TOTAL rows, expected codes: $EXPECTED_CODES)"

RESET_OUT=$(kubectl run -n platform "mysql-reset-$RANDOM" \
    --rm -i --restart=Never --image=mysql:8.0 \
    --command -- mysql -h simaops-tidb -P 4000 -u simaops -p"$PASS" simaops \
    < "$RESET_SQL" 2>&1 \
  | grep -v "^Warning\|warning:\|^If\|All commands\|pod " || true)
echo "$RESET_OUT"
if echo "$RESET_OUT" | grep -qE "ERROR"; then
  fail "warehouse capacity reset failed (see output above)"
fi
green "warehouse capacity reset to 1,000,000 per slot"

green "preflight passed"
