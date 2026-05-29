#!/usr/bin/env bash
# scripts/run-validation.sh — preflight, validation (20 VUs x 22 min),
# 10-min HPA scale-down observation window, postflight, report.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOAD_DIR="$(dirname "$SCRIPT_DIR")"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_STARTED_ISO="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

REPORTS="$LOAD_DIR/reports"
mkdir -p "$REPORTS"

PROM_ENV="${HOME}/.k6/prom.env"
if [ -r "$PROM_ENV" ]; then
  # shellcheck source=/dev/null
  . "$PROM_ENV"
  K6_OUT="--out experimental-prometheus-rw"
  echo "[run-validation] sourced $PROM_ENV — pushing metrics to Prometheus"
else
  K6_OUT=""
  echo "[run-validation] $PROM_ENV missing — running without remote-write"
fi

echo "[run-validation] preflight"
bash "$SCRIPT_DIR/preflight.sh"

echo "[run-validation] k6 run starting (testid=$TS)"
set +e
k6 run \
  --tag testid="$TS" \
  --tag scenario=validation \
  --summary-export="$REPORTS/validation-$TS-summary.json" \
  $K6_OUT \
  "$LOAD_DIR/scenarios/validation.js" 2>&1 | tee "$REPORTS/validation-$TS-stdout.txt"
K6_EXIT=${PIPESTATUS[0]}
set -e

echo "[run-validation] k6 finished (exit=$K6_EXIT). Holding 10 min for HPA scale-down observation."
sleep 600

echo "[run-validation] postflight"
bash "$SCRIPT_DIR/postflight.sh" validation "$RUN_STARTED_ISO" "$K6_EXIT" \
  > "$REPORTS/validation-$TS-postflight.md"

REPORT="$REPORTS/validation-$TS-report.md"
{
  echo "# Validation run — $TS"
  echo
  echo "- Started: $RUN_STARTED_ISO"
  echo "- k6 exit code: $K6_EXIT"
  echo "- Held 10 min after k6 exit for HPA scale-down stabilization."
  echo "- Reports: \`$REPORTS/validation-$TS-*\`"
  echo
  echo "## k6 summary"
  echo
  echo '```'
  tail -50 "$REPORTS/validation-$TS-stdout.txt"
  echo '```'
  echo
  cat "$REPORTS/validation-$TS-postflight.md"
} > "$REPORT"

echo "[run-validation] report → $REPORT"
exit $K6_EXIT
