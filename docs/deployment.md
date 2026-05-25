# Deployment

## Prerequisites

- GCP project `taling-cyberhack` with billing enabled
- `gcloud` CLI authenticated (`gcloud auth application-default login`)
- Node.js 20+, pnpm 9+
- Helm 3.x
- GitHub repo secrets configured (see below)

## Local Development

```bash
make stack-up          # Start TiDB, MinIO, NATS, Keycloak, Jaeger
make db-migrate        # Run schema migrations
make gen               # Generate proto stubs
cd apps/api && go run ./cmd/api          # Start Go API on :8080
cd apps/web && pnpm dev                  # Start SvelteKit on :5173
cd apps/ai-worker && python main.py      # Start AI worker on :8081
cd apps/outbox-publisher && go run ./cmd/publisher  # Start outbox publisher
```

## GKE Deployment (SST)

```bash
cd infra/sst
npx sst deploy --stage staging
```

This provisions:
- VPC + subnet
- GKE Standard cluster (system pool: e2-standard-4 × 3, app pool: e2-standard-8 × 3-6)
- Global static IP for ingress
- DNS via sslip.io (until a real domain is configured)

## Helm Releases (after cluster is ready)

Platform services are deployed via Helm:

```bash
# Platform namespace
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx -n platform --create-namespace
helm upgrade --install cert-manager jetstack/cert-manager -n platform --set installCRDs=true
helm upgrade --install keycloak bitnami/keycloak -n platform
helm upgrade --install tidb-operator pingcap/tidb-operator -n platform
helm upgrade --install minio bitnami/minio -n platform
helm upgrade --install nats nats/nats -n platform

# Observability namespace
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack -n observability --create-namespace
helm upgrade --install loki grafana/loki-stack -n observability
helm upgrade --install tempo grafana/tempo -n observability

# App namespace
for app in api web ai-worker outbox-publisher; do
  helm upgrade --install simaops-$app deploy/helm/simaops-$app -n simaops --create-namespace
done
```

## CI/CD (GitHub Actions)

| Workflow | Trigger | Action |
|---|---|---|
| `build.yaml` | Push to main / PR | Build + push images to ghcr.io |
| `deploy-staging.yaml` | After build succeeds on main | Deploy to GKE staging |
| `deploy-production.yaml` | Release tag published | Deploy to GKE production (manual approval) |

### Required GitHub Secrets

| Secret | Value |
|---|---|
| `GCP_PROJECT_ID` | `taling-cyberhack` |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | From WIF setup |
| `GCP_SERVICE_ACCOUNT` | `simaops-github-ci@taling-cyberhack.iam.gserviceaccount.com` |
| `GCP_REGION` | `asia-southeast2` |

## DNS Strategy

Currently using `sslip.io` (free wildcard DNS):
- `app.<static-ip>.sslip.io` → SvelteKit frontend
- `api.<static-ip>.sslip.io` → Go API
- `auth.<static-ip>.sslip.io` → Keycloak

When a real domain is available, update `infra/values/staging.yaml` to use Cloud DNS.
