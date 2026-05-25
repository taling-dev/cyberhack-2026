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
| 1 | Integrated Operations System | Unified data model (11 tables), Connect RPC (25 RPCs, 6 services), audit log, manager dashboard |
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

## Out of Scope (v1)

- Sample-dispatch tracking
- Lab-instrument sensor ingestion
- PPIC schedule generation (v1 stops at READY_FOR_PRODUCTION)
