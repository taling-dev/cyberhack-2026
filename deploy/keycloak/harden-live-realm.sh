#!/usr/bin/env bash
# Harden the LIVE simaops Keycloak realm to match the secured realm import.
# Realm import only applies on first creation; the running realm must be fixed
# in place. Run when ready (this is a live change: it rotates credentials and
# enforces TLS, which affects active logins).
#
# Required env (no secrets are hardcoded here):
#   KC_ADMIN_USER, KC_ADMIN_PASSWORD          Keycloak master admin
#   SIMAOPS_API_CLIENT_SECRET                 new simaops-api client secret
#   SIMAOPS_SEED_OPERATOR_PASSWORD
#   SIMAOPS_SEED_QC_PASSWORD
#   SIMAOPS_SEED_WAREHOUSE_PASSWORD
#   SIMAOPS_SEED_MANAGER_PASSWORD
#   SIMAOPS_SEED_ADMIN_PASSWORD
#
# Usage:
#   export KC_ADMIN_USER=admin KC_ADMIN_PASSWORD=... SIMAOPS_API_CLIENT_SECRET=...
#   export SIMAOPS_SEED_OPERATOR_PASSWORD=... (etc.)
#   bash deploy/keycloak/harden-live-realm.sh
#
# After running: update the SIMAOPS_API_CLIENT_SECRET in the simaops-infra-creds
# secret + redeploy the API, or it will fail client_credentials auth.
set -euo pipefail

: "${KC_ADMIN_USER:?}"; : "${KC_ADMIN_PASSWORD:?}"; : "${SIMAOPS_API_CLIENT_SECRET:?}"
: "${SIMAOPS_SEED_OPERATOR_PASSWORD:?}"; : "${SIMAOPS_SEED_QC_PASSWORD:?}"
: "${SIMAOPS_SEED_WAREHOUSE_PASSWORD:?}"; : "${SIMAOPS_SEED_MANAGER_PASSWORD:?}"
: "${SIMAOPS_SEED_ADMIN_PASSWORD:?}"

NS="${KC_NAMESPACE:-platform}"
REALM="simaops"
KCADM=/opt/keycloak/bin/kcadm.sh

POD=$(kubectl -n "$NS" get pods -l app=keycloak -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
if [ -z "$POD" ]; then
  POD=$(kubectl -n "$NS" get pods 2>/dev/null | grep -i keycloak | awk '{print $1}' | head -1 || true)
fi
[ -z "$POD" ] && { echo "keycloak pod not found in ns $NS" >&2; exit 1; }
echo "Using Keycloak pod: $POD"

kx() { kubectl -n "$NS" exec "$POD" -- "$@"; }

# Authenticate kcadm against the master realm.
kx "$KCADM" config credentials --server http://localhost:8080 --realm master \
  --user "$KC_ADMIN_USER" --password "$KC_ADMIN_PASSWORD"

# 1. Enforce TLS for external requests.
echo "==> sslRequired=external"
kx "$KCADM" update "realms/$REALM" -s sslRequired=EXTERNAL

# 2. Rotate the simaops-api confidential client secret.
echo "==> rotating simaops-api client secret"
CID=$(kx "$KCADM" get clients -r "$REALM" -q clientId=simaops-api --fields id --format csv --noquotes | tail -1)
[ -z "$CID" ] && { echo "simaops-api client not found" >&2; exit 1; }
kx "$KCADM" update "clients/$CID" -r "$REALM" -s "secret=$SIMAOPS_API_CLIENT_SECRET"

# 3. Rotate seed-user passwords as TEMPORARY (forces reset at next login).
rotate() { # <username> <password>
  echo "==> rotating password for $1 (temporary)"
  kx "$KCADM" set-password -r "$REALM" --username "$1" --new-password "$2" --temporary
}
# Note: live realm usernames are budi/siti/agus/dewi/admin (the deployed realm
# diverged from the import's operator/qc_supervisor/... names).
rotate budi  "$SIMAOPS_SEED_OPERATOR_PASSWORD"
rotate siti  "$SIMAOPS_SEED_QC_PASSWORD"
rotate agus  "$SIMAOPS_SEED_WAREHOUSE_PASSWORD"
rotate dewi  "$SIMAOPS_SEED_MANAGER_PASSWORD"
rotate admin "$SIMAOPS_SEED_ADMIN_PASSWORD"

echo
echo "Done. Next steps:"
echo "  - Update SIMAOPS_API_CLIENT_SECRET in the simaops-infra-creds secret and"
echo "    redeploy simaops-api (client_credentials will fail until it matches)."
echo "  - Ensure Keycloak is fronted by TLS; sslRequired=EXTERNAL now blocks"
echo "    plaintext logins from non-private addresses."
