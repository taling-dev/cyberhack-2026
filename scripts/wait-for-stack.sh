#!/usr/bin/env bash
set -euo pipefail

TIMEOUT=${1:-120}
INTERVAL=3
ELAPSED=0

check() {
  local name="$1" url="$2"
  if curl -sf --max-time 2 "$url" > /dev/null 2>&1; then
    echo "  ✓ $name"
    return 0
  fi
  return 1
}

echo "Waiting for platform stack (timeout: ${TIMEOUT}s)..."

while true; do
  ALL_UP=true

  check "TiDB"      "http://localhost:10080/status" || ALL_UP=false
  check "MinIO"     "http://localhost:9000/minio/health/live" || ALL_UP=false
  check "NATS"      "http://localhost:8222/healthz" || ALL_UP=false
  check "Keycloak"  "http://localhost:8080/health/ready" || ALL_UP=false
  check "Jaeger"    "http://localhost:16686/" || ALL_UP=false

  if [ "$ALL_UP" = true ]; then
    echo ""
    echo "All services healthy! (${ELAPSED}s)"
    exit 0
  fi

  if [ "$ELAPSED" -ge "$TIMEOUT" ]; then
    echo ""
    echo "ERROR: Timed out after ${TIMEOUT}s waiting for services."
    exit 1
  fi

  sleep "$INTERVAL"
  ELAPSED=$((ELAPSED + INTERVAL))
  echo "  ... retrying (${ELAPSED}s elapsed)"
done
