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

## Realtime updates

The Web UI receives live updates over Server-Sent Events. Mutations on the
API publish events to NATS via the outbox publisher; the API fans them out
to all locally-connected SSE clients with role-based and owner-scoped
filtering.

```
Outbox → NATS → API hub → /events SSE → BFF passthrough → EventSource
                                                               │
                                                               ├─→ TanStack Query invalidation
                                                               ├─→ Row-highlight action
                                                               ├─→ Toaster (role-targeted)
                                                               └─→ Live nav badges
```

**Endpoint:** `GET /events` (Authorization: Bearer required). The browser
hits `/api/v1/events` which is forwarded by the SvelteKit BFF.

**Envelope schema** (`apps/api/internal/events/envelope.go`):

```json
{
  "event_id":      "uuid",
  "event_type":    "qc.job.created",
  "occurred_at":   "2026-05-27T04:10:00Z",
  "actor_id":      "kc-user-id",
  "owner_user_id": "kc-user-id",
  "resource_id":   "lot-id-or-qc-job-id",
  "payload":       { ... }
}
```

**Subjects:** `lot.created`, `lot.status_changed`, `lot.ready_for_production`,
`qc.job.created`, `qc.job.completed`, `qc.job.needs_human_review`,
`qc.job.reviewed`, `qc.job.failed`, `qc.job.approved`,
`warehouse.slot_assigned`, `dispatch.created`, `dispatch.status_changed`,
`audit.log_created`.

**Role / owner filtering** (`apps/api/internal/events/filter.go`):

| Role | Allowed subjects | Owner-scoped |
|------|------------------|--------------|
| `OPERATOR` | `lot.>`, `warehouse.slot_assigned`, `qc.job.failed`, `qc.job.completed`, `dispatch.>` | yes — only events for lots they created |
| `QC_SUPERVISOR` | `lot.>`, `qc.>` | no |
| `WAREHOUSE_STAFF` | `lot.>`, `warehouse.>`, `qc.job.approved`, `qc.job.completed`, `dispatch.>` | no |
| `MANAGER`, `ADMIN` | everything | no |

**Auth-refresh resilience.** Browsers hit `/auth/heartbeat` every 60s so
the access cookie rotates before the API forces a reconnect. On any 401,
the client runs a three-tier recovery:

1. **Force refresh** (`/auth/heartbeat?force=true`) — exchanges the refresh
   token for a fresh access token.
2. **Silent OIDC renew** — invisible iframe at `/auth/login?silent=1` does a
   `prompt=none` OIDC roundtrip if the Keycloak SSO session is still alive.
3. **Popup login** — `SessionExpiredModal` opens `/auth/login?popup=1` in a
   popup; on success the parent window reconnects with no work loss.

Form drafts (new lot, QC review, slot assignment) are auto-saved to
`localStorage` every 500ms and survive any of the above tier transitions.

**Verification scripts:**

```bash
bash scripts/e2e-realtime.sh         # full pipeline + role-filter assertions
bash scripts/e2e-token-refresh.sh    # 4-min test, lowers TTL to 120s
```

**Observability:** the `simaops-api` Grafana dashboard's *SSE / Realtime* row
shows active connections per role, events/sec by subject, drops by reason,
hub dispatch p99, and clock skew vs Keycloak. Alerts:
`SimaopsSSEHighSlowClientDrops`, `SimaopsSSEHighEvictions`,
`SimaopsSSEDispatchPanics`, `SimaopsAPIClockSkewHigh`.

## License

Proprietary — Sima Arome / CYBERHACK 2026.
