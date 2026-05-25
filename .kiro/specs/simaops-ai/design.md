# SimaOps AI — Design

> Reads from: [`requirements.md`](./requirements.md)
> Drives: [`tasks.md`](./tasks.md)

## 1. Architecture

```mermaid
flowchart TB
  Browser["User Browser<br/>(SvelteKit + Connect-ES)"]
  BFF["SvelteKit Server<br/>(BFF: OIDC PKCE,<br/>HttpOnly cookie session)"]
  API["Go API<br/>(connect-go)<br/>OIDC verify · RBAC · audit ·<br/>idempotency · workflow FSM"]
  TiDB[("TiDB Cluster<br/>3 PD · 3 TiKV · 2 TiDB")]
  MinIO[("MinIO<br/>S3-compatible buckets")]
  NATS{{"NATS JetStream<br/>durable streams + DLQ"}}
  Outbox["Outbox Publisher<br/>(Go, leader-elected singleton)"]
  Worker["Python AI Worker<br/>FastAPI · OpenCV ·<br/>ONNX Runtime · YOLOv8n"]
  KC["Keycloak<br/>OIDC IdP (Helm)"]
  Obs["Observability bundle<br/>OTel Collector · Prometheus ·<br/>Grafana · Loki · Tempo"]
  Vault["OpenBao<br/>(Vault-compat)"]

  Browser -- "Connect/JSON + cookie" --> BFF
  BFF -- "Connect/JSON + bearer JWT" --> API
  Browser -. "redirect for login" .-> KC
  BFF <-. OIDC .-> KC
  API --> TiDB
  API --> MinIO
  API -- "tx-write outbox row" --> TiDB
  Outbox -- "poll outbox_events" --> TiDB
  Outbox -- "publish qc.job.created" --> NATS
  NATS -- "consume" --> Worker
  Worker -- "GET image" --> MinIO
  Worker -- "PUT annotated image" --> MinIO
  Worker -- "INSERT qc_results" --> TiDB
  Worker -- "publish qc.job.completed/.failed" --> NATS
  API -- "secrets fetch" --> Vault
  Worker -- "secrets fetch" --> Vault
  Outbox -- "secrets fetch" --> Vault
  API -- "OTLP" --> Obs
  Worker -- "OTLP" --> Obs
  Outbox -- "OTLP" --> Obs
  BFF -- "OTLP" --> Obs
```

## 2. Repository Layout

```
cyberhack2026/                           # repo root, preserves .kiro/{steering,skills}
  apps/
    web/                                 # SvelteKit + Tailwind + shadcn-svelte + Connect-ES + TanStack Query + Zod + i18n
    api/                                 # Go connect-go API
    ai-worker/                           # Python FastAPI + OpenCV + ONNX + YOLOv8n (strategy: mock|pretrained|custom)
    outbox-publisher/                    # Go singleton, k8s Lease leader election
  proto/simaops/
    lot/v1/lot.proto
    qc/v1/qc.proto
    warehouse/v1/warehouse.proto
    audit/v1/audit.proto
    dashboard/v1/dashboard.proto
    admin/v1/admin.proto
  db/
    migrations/                          # Atlas .sql files
    seed/
      images/{raw_botanical,extract,powder}/
      data.sql
  deploy/
    helm/
      simaops-web/
      simaops-api/
      simaops-ai-worker/
      simaops-outbox-publisher/
    keycloak/
      simaops-realm.json
    k8s/
      base/
      overlays/{dev,staging,production}/
  infra/
    sst/sst.config.ts
    values/{dev,staging,production}.yaml
  docs/
    architecture.md
    api-contract.md
    rbac.md
    audit-log.md
    deployment.md
    demo-script.md
  .github/workflows/
    build.yaml
    deploy-staging.yaml
    deploy-production.yaml
  docker-compose.yml
  buf.yaml
  buf.gen.yaml
  Makefile
  README.md
  package.json                           # root, pnpm workspace
  go.work                                # Go workspace
```

## 3. Service Boundaries

| Service                  | Language | Responsibility                                                                                  |
| ------------------------ | -------- | ----------------------------------------------------------------------------------------------- |
| `apps/web`               | TS       | SvelteKit BFF: OIDC PKCE flow, HttpOnly cookie session, server-side bearer forwarding, UI       |
| `apps/api`               | Go       | Connect RPC handlers; OIDC + RBAC + audit + idempotency middleware; sqlc DB access; outbox writes |
| `apps/ai-worker`         | Python   | NATS JetStream consumer; image fetch + OpenCV + ONNX inference + findings mapping; result write  |
| `apps/outbox-publisher`  | Go       | Singleton via `Lease` leader election; polls `outbox_events`; publishes to NATS; idempotent     |

## 4. Connect RPC Services

Six services (signatures locked in `proto/simaops/{lot,qc,warehouse,audit,dashboard,admin}/v1/*.proto`).

| Service             | Methods                                                                            |
| ------------------- | ---------------------------------------------------------------------------------- |
| `LotService`        | CreateLot, GetLot, ListLots, UpdateLotStatus, GetLotTimeline                       |
| `QCService`         | CreateQCUploadUrl, CreateQCJob, GetQCJob, GetQCResult, ReviewQC, RetryQCJob        |
| `WarehouseService`  | ListLocations, RecommendSlot, AssignSlot, GetWarehouseAssignments                  |
| `AuditService`      | ListAuditLogs, GetEntityAuditTrail                                                 |
| `DashboardService`  | GetOpsDashboard, GetQCMetrics, GetWarehouseMetrics                                 |
| `AdminService`      | ListUsers, AssignRole, RevokeRole, ListRoles                                       |

Mutating RPCs (every `Create*`, `Update*`, `Assign*`, `Review*`, `Retry*`, `Revoke*`) carry an `idempotency_key` field.

## 5. Workflow State Machine

### Lot status

```
DRAFT
 → PENDING_QC
 → AI_PROCESSING
 → QC_REVIEW
 → QC_APPROVED ──┐
 → QC_REJECTED   │
                 ▼
          WAREHOUSE_ASSIGNED
                 ▼
          READY_FOR_PRODUCTION
(BLOCKED is a side-rail any state can transition into)
```

### QC job status

```
QUEUED → PROCESSING → AI_COMPLETED → NEEDS_HUMAN_REVIEW → APPROVED | REJECTED | FAILED
```

### Transition rules

- Only `OPERATOR` or `ADMIN` may create lot or upload QC image.
- Only `QC_SUPERVISOR` or `ADMIN` may approve / reject / recheck QC.
- Only `WAREHOUSE_STAFF` or `ADMIN` may assign a slot.
- Lot cannot reach `WAREHOUSE_ASSIGNED` before `QC_APPROVED`; cannot reach `READY_FOR_PRODUCTION` before `WAREHOUSE_ASSIGNED`.
- Manual override of an AI recommendation requires a non-empty `reason`.
- Every state-changing RPC writes an audit-log row.
- Every mutating RPC is idempotent on `(user_id, operation, idempotency_key)`.

## 6. Domain Entity Shapes (key tables)

### `lots`

```sql
CREATE TABLE lots (
  id              VARCHAR(36)  PRIMARY KEY,
  lot_number      VARCHAR(64)  NOT NULL UNIQUE,                -- LOT-YYYY-MMDD-XXX
  supplier_name   VARCHAR(255) NOT NULL,
  material_name   VARCHAR(255) NOT NULL,
  material_type   ENUM('RAW_BOTANICAL','EXTRACT','POWDER','OTHER') NOT NULL,
  quantity        DECIMAL(14,3) NOT NULL,
  unit            VARCHAR(16)   NOT NULL,                      -- kg, L, ...
  arrival_date    DATE          NOT NULL,
  storage_requirement JSON      NOT NULL,                      -- { temperature_range, hazard_class }
  status          ENUM('DRAFT','PENDING_QC','AI_PROCESSING','QC_REVIEW',
                       'QC_APPROVED','QC_REJECTED','WAREHOUSE_ASSIGNED',
                       'READY_FOR_PRODUCTION','BLOCKED') NOT NULL DEFAULT 'DRAFT',
  created_by      VARCHAR(64)  NOT NULL,
  created_at      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_status (status),
  INDEX idx_material_type (material_type)
);
```

`storage_requirement` JSON shape:

```json
{
  "temperature_range": "ambient | cold | deep_freeze",
  "hazard_class":      "null | IBC | IPPC"
}
```

### `warehouse_locations`

```sql
CREATE TABLE warehouse_locations (
  id                  VARCHAR(36) PRIMARY KEY,
  code                VARCHAR(32) NOT NULL UNIQUE,
  zone                VARCHAR(32) NOT NULL,
  temperature_min     DECIMAL(5,2) NOT NULL,                   -- supports negatives down to -25
  temperature_max     DECIMAL(5,2) NOT NULL,
  hazard_allowed      JSON NOT NULL DEFAULT (JSON_ARRAY()),    -- ["IBC","IPPC"] or []
  drum_compatibility  JSON NOT NULL DEFAULT (JSON_ARRAY()),    -- ["IBC","IPPC"]
  capacity            INT NOT NULL DEFAULT 0,
  current_status      ENUM('AVAILABLE','OCCUPIED','MAINTENANCE') NOT NULL DEFAULT 'AVAILABLE'
);
```

### Other entities (per spec)

`qc_jobs`, `qc_results`, `warehouse_assignments`, `audit_logs`, `outbox_events`, `idempotency_keys`, `users_profile`, `roles`, `user_roles` — see spec; minimum SQL stubs land in `db/migrations/` during Task 5.

## 7. AI Worker — `findings_map.yaml`

Selected at runtime by `lot.material_type` carried in the NATS message:

```yaml
RAW_BOTANICAL:
  classes:
    banana: { mapped_to: ripeness_signal,    anomaly: false }
    apple:  { mapped_to: ripeness_signal,    anomaly: false }
    bottle: { mapped_to: foreign_matter,     anomaly: true  }
    person: { mapped_to: human_artifact,     anomaly: true  }
  rules:
    pass:   "max_confidence >= 0.85 AND no_anomaly"
    review: "max_confidence >= 0.50 AND any_anomaly"
    fail:   "anomaly_count >= 2"

EXTRACT_POWDER:
  classes:
    bowl:    { mapped_to: container_visible,      anomaly: false }
    cup:     { mapped_to: container_visible,      anomaly: false }
    bottle:  { mapped_to: contamination_artifact, anomaly: true  }
    person:  { mapped_to: human_artifact,         anomaly: true  }
  rules:
    pass:   "max_confidence >= 0.80 AND no_anomaly"
    review: "max_confidence >= 0.50 AND any_anomaly"
    fail:   "anomaly_count >= 2 OR any_class('person')"
```

Stored at `simaops-model-artifacts/findings_map.yaml`. Worker validates schema on load.

## 8. Helm Chart Matrix

| Namespace       | Helm release                  | Source / Notes                                              |
| --------------- | ----------------------------- | ----------------------------------------------------------- |
| `platform`      | `ingress-nginx`               | upstream                                                    |
| `platform`      | `cert-manager`                | upstream + `ClusterIssuer` for Let's Encrypt HTTP-01        |
| `platform`      | `keycloak`                    | bitnami; mounts `simaops-realm.json` ConfigMap              |
| `platform`      | `tidb-operator`               | pingcap                                                     |
| `platform`      | `TidbCluster` (CR)            | 3 PD + 3 TiKV + 2 TiDB; PDBs `minAvailable: 2/2/1`          |
| `platform`      | `minio`                       | bitnami; init Job creates buckets                           |
| `platform`      | `nats`                        | upstream; JetStream enabled; 3 replicas                     |
| `observability` | `kube-prometheus-stack`       | bundles Prometheus + Alertmanager + Grafana + node-exporter |
| `observability` | `loki-stack`                  | logs                                                        |
| `observability` | `tempo`                       | traces                                                      |
| `observability` | `opentelemetry-collector`     | OTLP receiver                                               |
| `platform`      | `openbao`                     | secrets (production); KV v2 + transit                       |
| `simaops`       | `simaops-web` (local chart)   | SvelteKit Node container                                    |
| `simaops`       | `simaops-api` (local chart)   | Go API                                                      |
| `simaops`       | `simaops-ai-worker` (local)   | Python worker                                               |
| `simaops`       | `simaops-outbox-publisher`    | Go singleton with `Lease`                                   |

App charts ship with `Deployment`, `Service`, `ConfigMap`, `Secret` ref, `ServiceAccount`, `HPA`, `PDB`, `NetworkPolicy`, `ServiceMonitor`, `PrometheusRule` templates.

## 9. SST Orchestration Scope

`infra/sst/sst.config.ts` provisions:

- **GCP:** VPC + subnet, GKE Standard cluster (regional control plane), 2 node pools — `system` (`e2-standard-4`, fixed 3) and `app` (`e2-standard-8`, autoscale 3–6); static IP for ingress; Cloud DNS A records for `app|api|auth|grafana|minio.<domain>`.
- **Kubernetes:** namespaces `platform`, `simaops`, `observability`; all Helm releases above; ConfigMaps + Secret refs; HPAs; PDBs; NetworkPolicies.

Stages:

- `dev` — k3d local; no GCP resources.
- `staging` — full GCP + GKE deploy on push to `main`.
- `production` — same shape, `protect: true` + `removal: retain`; manual approval gate.

Outputs: `frontend_url`, `api_url`, `keycloak_url`, `minio_console_url`, `grafana_url`, `nats_endpoint_internal`, `tidb_endpoint_internal`, `kubeconfig`, `static_ip`.

## 10. CI/CD

- `.github/workflows/build.yaml` — matrix on `apps/{web,api,ai-worker,outbox-publisher}`; build, unit-test, push to `ghcr.io/<owner>/simaops-<app>:<sha>` and `:<branch>`.
- `.github/workflows/deploy-staging.yaml` — on push to `main` after `build` succeeds, runs `sst deploy --stage staging` with image tags pinned to commit SHA.
- `.github/workflows/deploy-production.yaml` — on `release` tag, requires `environment: production` approval, runs `sst deploy --stage production`.

GCP auth via Workload Identity Federation (no long-lived service-account keys).
