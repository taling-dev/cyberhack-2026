#!/usr/bin/env bash
# scripts/run-smoke.sh — preflight, smoke (1 VU x 1 min), postflight, report.

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
  echo "[run-smoke] sourced $PROM_ENV — pushing metrics to Prometheus"
else
  K6_OUT=""
  echo "[run-smoke] $PROM_ENV missing — running without remote-write"
fi

echo "[run-smoke] preflight"
bash "$SCRIPT_DIR/preflight.sh"

echo "[run-smoke] k6 run (testid=$TS)"
set +e
k6 run \
  --tag testid="$TS" \
  --tag scenario=smoke \
  --summary-export="$REPORTS/smoke-$TS-summary.json" \
  $K6_OUT \
  "$LOAD_DIR/scenarios/smoke.js" 2>&1 | tee "$REPORTS/smoke-$TS-stdout.txt"
K6_EXIT=${PIPESTATUS[0]}
set -e

echo "[run-smoke] postflight"
bash "$SCRIPT_DIR/postflight.sh" smoke "$RUN_STARTED_ISO" "$K6_EXIT" \
  > "$REPORTS/smoke-$TS-postflight.md"

REPORT="$REPORTS/smoke-$TS-report.md"
{
  echo "# Smoke run — $TS"
  echo
  echo "- Started: $RUN_STARTED_ISO"
  echo "- k6 exit code: $K6_EXIT"
  echo "- Reports: \`$REPORTS/smoke-$TS-*\`"
  echo
  echo "## k6 summary"
  echo
  echo '```'
  tail -40 "$REPORTS/smoke-$TS-stdout.txt"
  echo '```'
  echo
  cat "$REPORTS/smoke-$TS-postflight.md"
} > "$REPORT"

echo
echo "[run-smoke] report → $REPORT"
exit $K6_EXIT
