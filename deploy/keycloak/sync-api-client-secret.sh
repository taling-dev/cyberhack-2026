#!/usr/bin/env bash
# Follow-up to harden-live-realm.sh: sync the rotated simaops-api client secret
# into the k8s secret and redeploy the API. Run IMMEDIATELY after the realm
# hardening script rotates the client secret, or the API's client_credentials
# auth (Keycloak role sync) stays broken until it matches.
#
# Required env:
#   SIMAOPS_API_CLIENT_SECRET   the SAME new secret used by harden-live-realm.sh
#
# Usage:
#   export SIMAOPS_API_CLIENT_SECRET=...   # must match what you set in Keycloak
#   bash deploy/keycloak/sync-api-client-secret.sh
set -euo pipefail

: "${SIMAOPS_API_CLIENT_SECRET:?}"
NS="${SIMAOPS_NAMESPACE:-simaops}"
SECRET="${SIMAOPS_INFRA_SECRET:-simaops-infra-creds}"
KEY="${SIMAOPS_SECRET_KEY:-KEYCLOAK_ADMIN_CLIENT_SECRET}"

echo "==> patching $SECRET ($KEY) in ns $NS"
kubectl -n "$NS" patch secret "$SECRET" --type merge \
  -p "{\"stringData\":{\"$KEY\":\"$SIMAOPS_API_CLIENT_SECRET\"}}"

# Restart the API to pick up the new secret (envFrom secretRef is read at boot).
echo "==> restarting simaops-api"
kubectl -n "$NS" rollout restart deploy/simaops-api
kubectl -n "$NS" rollout status deploy/simaops-api --timeout=180s

echo "Done. Verify role sync: assign a role in the admin UI and confirm it"
echo "lands in Keycloak (no 'keycloak role sync' 500s in the API logs)."
