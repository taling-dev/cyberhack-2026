# Architecture

## System Overview

SimaOps AI is a cloud-portable enterprise operations platform running on Kubernetes (GKE Standard). All application services are containerized and deployed via Helm charts; no Google-native application services are used for core logic.

```
[Browser] → [SvelteKit BFF] → [Go API] → [TiDB]
                                   ├──→ [MinIO]
                                   └──→ [NATS JetStream] → [Python AI Worker]
```

## Focus Area Coverage (CYBERHACK 2026)

| # | Focus Area | SimaOps Component |
|---|---|---|
| 1 | Integrated Operations System | Unified data model (12 tables), Connect RPC (29 RPCs, 7 services), audit log, manager dashboard, **dispatch stage** closing intake → QC → warehouse → production handoff → dispatch in one flow |
| 2 | AI for Fruit & Raw-Material QC | AI worker `pretrained` strategy with `RAW_BOTANICAL` findings vocabulary |
| 3 | AI for Extract & Powder QC | Same worker with `EXTRACT_POWDER` findings vocabulary |
| 4 | AI-Assisted Warehousing & Cold-Chain | Warehouse recommender filtering on temperature range + drum class (IBC/IPPC) |

## Service Boundaries

| Service | Language | Port | Responsibility |
|---|---|---|---|
| `simaops-web` | TypeScript (SvelteKit) | 3000 | BFF: OIDC PKCE, cookie session, UI |
| `simaops-api` | Go (connect-go) | 8080 | Connect RPC handlers, RBAC, audit, idempotency |
| `simaops-ai-worker` | Python (FastAPI) | 8081 | NATS consumer, OpenCV + ONNX inference |
| `simaops-outbox-publisher` | Go | — | Singleton: polls outbox → publishes to NATS |

## Platform Components (Helm)

| Component | Purpose |
|---|---|
| TiDB Operator + Cluster | Distributed SQL (MySQL-compatible) |
| MinIO | S3-compatible object storage (QC images, models) |
| NATS JetStream | Durable async event bus |
| Keycloak | OIDC identity provider |
| kube-prometheus-stack | Metrics + alerting + Grafana |
| Loki + Tempo | Logs + traces |
| OpenTelemetry Collector | OTLP receiver/router |
| cert-manager + ingress-nginx | TLS + routing |

## Security Model

- JWT verification on every RPC (Keycloak JWKS)
- Table-driven RBAC (25 RPCs × 5 roles)
- HttpOnly cookie session (browser never sees tokens)
- MinIO buckets private (presigned URLs only)
- Idempotency keys prevent duplicate mutations
- Audit log on every state change

## Production Handoff & Dispatch

The final stage of the Integrated Operations System. When a lot reaches
`READY_FOR_PRODUCTION` (via slot assignment or a direct status update), the API
emits a dedicated `lot.ready_for_production` event carrying the full lot payload
(material, quantity, storage requirement, assigned slot). This is a distinct
subject — separate from the generic `lot.status_changed` firehose — so
downstream consumers (dispatch, and a future PPIC scheduler) can subscribe to
exactly the handoff signal, gated by the SSE role filter.

`DispatchService` (Create/Get/List/UpdateDispatchStatus) records shipments of
production-ready lots. A dispatch is created only from a `READY_FOR_PRODUCTION`
lot, a lot may have at most one active (non-cancelled) dispatch, and the
dispatch FSM is `PENDING → SCHEDULED → IN_TRANSIT → DELIVERED` with `CANCELLED`
as a side-rail from any non-terminal state. Every transition writes an outbox
event (`dispatch.created`, `dispatch.status_changed`) and an audit-log row in
the same transaction as the state change.

## Out of Scope (v1)

- Lab-instrument sensor ingestion
- PPIC schedule generation — but `lot.ready_for_production` now provides the
  clean integration seam a PPIC scheduler would consume.
