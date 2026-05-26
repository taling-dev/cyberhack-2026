# Deployment

## Prerequisites

- OCI tenancy with a compartment for SimaOps
- OCI CLI authenticated (`oci setup config`)
- Node.js 20+, pnpm 9+
- Helm 3.x
- GitHub repo secrets configured (see below)

## OCI Setup (one-time)

### 1. Create OCI account + tenancy

Sign up at https://signup.oraclecloud.com (Always Free tier sufficient for staging).

### 2. Create an API key

```bash
mkdir -p ~/.oci
oci setup config
# Follow prompts: tenancy OCID, user OCID, region (ap-singapore-1)
# It will generate ~/.oci/oci_api_key.pem and add the public key to your user
```

Or generate manually:

```bash
openssl genrsa -out ~/.oci/oci_api_key.pem 2048
chmod 600 ~/.oci/oci_api_key.pem
openssl rsa -pubout -in ~/.oci/oci_api_key.pem -out ~/.oci/oci_api_key_public.pem
```

Then in OCI Console: User → API Keys → Add Public Key → paste the public key contents. Note the **fingerprint** that's displayed.

### 3. Create a compartment

OCI Console → Identity & Security → Compartments → Create Compartment (e.g., `simaops`). Note the **compartment OCID**.

### 4. Set environment variables

```bash
export OCI_TENANCY_OCID="ocid1.tenancy.oc1..aaaa..."
export OCI_USER_OCID="ocid1.user.oc1..aaaa..."
export OCI_FINGERPRINT="aa:bb:cc:dd:..."
export OCI_PRIVATE_KEY="$(cat ~/.oci/oci_api_key.pem)"
export OCI_REGION="ap-singapore-1"
export OCI_COMPARTMENT_OCID="ocid1.compartment.oc1..aaaa..."
```

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

## OKE Deployment (SST)

```bash
cd infra/sst
pnpm install
npx sst deploy --stage staging
```

This provisions:
- VCN `10.0.0.0/16` + public subnet + Internet Gateway + Route Table + Security List
- OKE Basic Cluster (free control plane, Kubernetes v1.30.x)
- Node pool: 2× `VM.Standard.E4.Flex` (AMD x86_64), 2 OCPU / 16 GB each, BASELINE_1_8 burstable

After deploy completes, populate kubeconfig:

```bash
oci ce cluster create-kubeconfig \
  --cluster-id $(npx sst output clusterOcid --stage staging) \
  --file ~/.kube/config \
  --region ap-singapore-1 \
  --token-version 2.0.0 \
  --kube-endpoint PUBLIC_ENDPOINT

kubectl get nodes  # should show 2 ready nodes
```

## Helm Releases (after cluster is ready)

```bash
# Add helm repos
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add jetstack https://charts.jetstack.io
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add pingcap https://charts.pingcap.org/
helm repo add nats https://nats-io.github.io/k8s/helm/charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

# Platform namespace
kubectl create namespace platform
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx -n platform
helm upgrade --install cert-manager jetstack/cert-manager -n platform --set installCRDs=true
helm upgrade --install keycloak bitnami/keycloak -n platform
helm upgrade --install tidb-operator pingcap/tidb-operator -n platform
helm upgrade --install minio bitnami/minio -n platform
helm upgrade --install nats nats/nats -n platform --set jetstream.enabled=true

# Observability namespace
kubectl create namespace observability
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack -n observability
helm upgrade --install loki grafana/loki-stack -n observability
helm upgrade --install tempo grafana/tempo -n observability

# App namespace
for app in api web ai-worker outbox-publisher; do
  helm upgrade --install simaops-$app deploy/helm/simaops-$app \
    -n simaops --create-namespace
done
```

## Get LoadBalancer Public IP

```bash
kubectl get svc -n platform ingress-nginx-controller \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

URLs (using sslip.io):
- `https://app.<lb-ip>.sslip.io` → SvelteKit frontend
- `https://api.<lb-ip>.sslip.io` → Go API
- `https://auth.<lb-ip>.sslip.io` → Keycloak
- `https://grafana.<lb-ip>.sslip.io` → Grafana

## CI/CD (GitHub Actions)

| Workflow | Trigger | Action |
|---|---|---|
| `build.yaml` | Push to main / PR | Build + push images to ghcr.io |
| `deploy-staging.yaml` | After build succeeds on main | Deploy to OKE staging |
| `deploy-production.yaml` | Release tag published | Deploy to OKE production (manual approval) |

### Required GitHub Secrets

| Secret | Value |
|---|---|
| `OCI_TENANCY_OCID` | Your tenancy OCID |
| `OCI_USER_OCID` | Your user OCID |
| `OCI_FINGERPRINT` | API key fingerprint |
| `OCI_PRIVATE_KEY` | Contents of `~/.oci/oci_api_key.pem` |
| `OCI_REGION` | `ap-singapore-1` |
| `OCI_COMPARTMENT_OCID` | Target compartment OCID |
| `OCI_CLUSTER_OCID` | OKE cluster OCID (after first `sst deploy`) |
| `OCI_PRIVATE_KEY_PROD` | Production API key (separate from staging) |
| `OCI_CLUSTER_OCID_PROD` | Production OKE cluster OCID |

## DNS Strategy

Currently using `sslip.io` (free wildcard DNS): `<lb-ip>.sslip.io` resolves to `<lb-ip>`.

When a real domain is available, update `infra/values/staging.yaml`:

```yaml
dns:
  strategy: oci_dns
  domain: simaops.example.com
```

And update Helm releases to use the real domain in ingress hostnames.
