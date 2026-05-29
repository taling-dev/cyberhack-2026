#!/usr/bin/env bash
# scripts/run-soak.sh — preflight, soak (5 VUs x 2 hr), 5-min idle drain,
# postflight, report. Plan with this taking ~2h10m total.

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
  echo "[run-soak] sourced $PROM_ENV — pushing metrics to Prometheus"
else
  K6_OUT=""
  echo "[run-soak] $PROM_ENV missing — running without remote-write"
fi

echo "[run-soak] preflight"
bash "$SCRIPT_DIR/preflight.sh"

echo "[run-soak] k6 run starting (testid=$TS)"
set +e
k6 run \
  --tag testid="$TS" \
  --tag scenario=soak \
  --summary-export="$REPORTS/soak-$TS-summary.json" \
  $K6_OUT \
  "$LOAD_DIR/scenarios/soak.js" 2>&1 | tee "$REPORTS/soak-$TS-stdout.txt"
K6_EXIT=${PIPESTATUS[0]}
set -e

echo "[run-soak] k6 finished (exit=$K6_EXIT). Holding 5 min idle to confirm metrics return to baseline."
sleep 300

echo "[run-soak] postflight"
bash "$SCRIPT_DIR/postflight.sh" soak "$RUN_STARTED_ISO" "$K6_EXIT" \
  > "$REPORTS/soak-$TS-postflight.md"

REPORT="$REPORTS/soak-$TS-report.md"
{
  echo "# Soak run — $TS"
  echo
  echo "- Started: $RUN_STARTED_ISO"
  echo "- k6 exit code: $K6_EXIT"
  echo "- 5-min idle drain after k6 exit before postflight."
  echo "- Reports: \`$REPORTS/soak-$TS-*\`"
  echo
  echo "## k6 summary"
  echo
  echo '```'
  tail -60 "$REPORTS/soak-$TS-stdout.txt"
  echo '```'
  echo
  cat "$REPORTS/soak-$TS-postflight.md"
} > "$REPORT"

echo "[run-soak] report → $REPORT"
exit $K6_EXIT
