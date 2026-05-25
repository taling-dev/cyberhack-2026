# SimaOps AI

Enterprise-grade, cloud-portable operations platform for **Sima Arome** — an Indonesian natural-extracts manufacturer powering F&B, cosmetics, and wellness brands.

SimaOps AI unifies lot intake, AI-assisted QC, human QC approval, cold-chain warehouse assignment, and production-readiness dashboards into a single auditable system.

## Architecture

```
[SvelteKit BFF] ──Connect/JSON──▶ [Go API] ──▶ [TiDB]
                                      │
                                      ├──▶ [MinIO]
                                      └──▶ [NATS JetStream] ──▶ [Python AI Worker]
```

- **Frontend:** SvelteKit + Tailwind + shadcn-svelte + Connect-ES + TanStack Query
- **Backend:** Go + connect-go + sqlc + Atlas migrations
- **AI Worker:** Python + FastAPI + OpenCV + ONNX Runtime (YOLOv8n)
- **Auth:** Keycloak (OIDC) with BFF cookie session
- **Infra:** GKE Standard via SST; Helm charts for all components

## Quickstart (local dev)

```bash
# Prerequisites: Node 20+, Go 1.22+, Python 3.12+, pnpm 9+, Docker

# Start platform services (TiDB, MinIO, NATS, Keycloak, Jaeger)
make stack-up

# Run database migrations
make db-migrate

# Generate proto stubs
make gen

# Start the Go API
cd apps/api && go run ./cmd/api

# Start the SvelteKit frontend (separate terminal)
cd apps/web && pnpm dev
```

## Make targets

```bash
make help    # Show all available targets
```

## Project structure

```
apps/
  web/                  SvelteKit frontend (BFF)
  api/                  Go Connect RPC backend
  ai-worker/            Python AI inference worker
  outbox-publisher/     Go outbox → NATS publisher
proto/simaops/          Protobuf service definitions
db/                     Migrations + seed data
deploy/                 Helm charts + k8s overlays
infra/                  SST config + env values
docs/                   Architecture, API, RBAC, deployment docs
```

## Documentation

- [Architecture](docs/architecture.md)
- [API Contract](docs/api-contract.md)
- [RBAC](docs/rbac.md)
- [Audit Log](docs/audit-log.md)
- [Deployment](docs/deployment.md)
- [Demo Script](docs/demo-script.md)

## License

Proprietary — Sima Arome / CYBERHACK 2026.
