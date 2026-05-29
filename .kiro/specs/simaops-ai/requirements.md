# SimaOps AI — Requirements

> Status: **APPROVED — pending execution kickoff**
> Repo: `/home/dharon/cyberhack2026`
> Plan revision: v2 (Sima Arome refinements folded in)

## 1. Problem Statement

Sima Arome — Indonesian natural-extracts manufacturer for F&B, cosmetics, and wellness brands — runs end-to-end raw-material intake, manual QC clearance, warehouse storage with hazard segregation and cold-chain (−4 °C to −20 °C), and PPIC-scheduled production handoff. Today, operators re-enter the same data across multiple tools, QC throughput stalls when a trained eye is unavailable, drum placement is tracked in spreadsheets, and PPIC schedules / lot histories / sample dispatches live in notebooks and chats. The result is slowdowns, rework, missed batches, and zero auditable traceability.

SimaOps AI is a single enterprise platform that:

1. Streamlines operator → AI-triage → human-QC-approval → warehouse → production-readiness as one event-driven flow.
2. Captures full audit trails, idempotency, and durable async processing (no double work, no lost events).
3. Runs as cloud-portable open components on Kubernetes, with Google Cloud as the hosting substrate only — no application-level cloud lock-in, so the same workload can move clouds without rewrite.
4. Uses AI strictly as **assistive** triage; final QC approval is always a human decision.

## 2. Hackathon Focus-Area Coverage (CYBERHACK 2026)

| # | Focus Area                              | SimaOps Coverage                                                                                            |
| - | --------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| 1 | Integrated Operations System            | Unified data model + Connect RPC services + audit log + manager dashboard. Kills double-entry.              |
| 2 | AI for Fruit & Raw-Material QC          | AI worker `pretrained` strategy with `RAW_BOTANICAL` findings vocabulary (ripeness, color, foreign matter). |
| 3 | AI for Extract & Powder QC              | Same worker with `EXTRACT_POWDER` findings vocabulary (color uniformity, contamination, lumping).           |
| 4 | AI-Assisted Warehousing & Cold-Chain    | Warehouse recommender filtering on temperature range + drum class (IBC/IPPC) + hazard rules.                |

**Out of scope for v1:** sample-dispatch tracking, lab-instrument sensor ingestion, PPIC schedule generation. (v1 stops at `READY_FOR_PRODUCTION` handoff signal.)

## 3. Hard Constraints (from spec)

- Use **SST** for deployment orchestration.
- **Google Cloud** primary host; **GKE Standard** is the runtime.
- **Avoid cloud lock-in.** Do not build around AWS or Google application services.
- **Kubernetes is the portability boundary.** Core app services run as containers / Helm charts.
- **Connect RPC** for frontend ↔ backend.
- Backend enforces **RBAC, workflow state machine, idempotency, audit logging**.
- AI is **assistive only**; final QC approval is human-controlled.
- **No direct browser ↔ database** access. **No direct browser ↔ queue** access.
- All cross-origin browser calls go through SvelteKit (BFF pattern).

## 4. Final Tech Stack

| Layer            | Choice                                                                                                        |
| ---------------- | ------------------------------------------------------------------------------------------------------------- |
| Frontend         | SvelteKit (Node adapter), Tailwind, shadcn-svelte, Connect-ES, TanStack Query, Zod, svelte-i18n (EN + ID)     |
| API contract     | Protobuf, Buf, Connect RPC, Protovalidate                                                                     |
| Backend          | Go (`connect-go`), `chi` router, `slog` JSON logs                                                             |
| DB access        | `sqlc` (preferred), MySQL dialect targeting TiDB                                                              |
| Migrations       | Atlas                                                                                                         |
| Database         | TiDB cluster via TiDB Operator (production sizing: 3 PD + 3 TiKV + 2 TiDB, anti-affinity, PDBs)               |
| Auth IdP         | Keycloak (Helm) — realm import file checked into git, auto-imported on startup                                |
| Object storage   | MinIO (Helm) — buckets: `simaops-qc-images`, `simaops-qc-results`, `simaops-reports`, `simaops-model-artifacts` |
| Queue / events   | NATS JetStream (Helm) — durable consumers, max_deliver=4, DLQ subject `qc.job.dlq`                            |
| AI worker        | Python 3.12, FastAPI, OpenCV, ONNX Runtime CPU, YOLOv8n; switchable strategy (`mock` / `pretrained` / `custom`) |
| Observability    | `kube-prometheus-stack` + `loki-stack` + `tempo` + `opentelemetry-collector` (single bundle)                  |
| Secrets          | OpenBao (production); Kubernetes Secrets (dev/staging fallback)                                               |
| Container reg    | `ghcr.io` (cloud-neutral)                                                                                     |
| CI/CD            | GitHub Actions: build matrix → `ghcr.io`; `sst deploy --stage staging` on `main`; manual approval → production |
| Deployment tool  | SST v4 with `@pulumi/oci` provider                                                                            |
| Tests — Go       | `testing` + `testify` + `connectrpc.com/connect` test client                                                  |
| Tests — TS/Svelte| Vitest (unit) + Playwright (e2e)                                                                              |
| Tests — Python   | pytest + NATS + MinIO test containers                                                                         |
| GitOps           | None for v1 (Argo CD / Flux deferred)                                                                         |

## 5. Locked Decisions (from Q&A)

| #  | Decision                          | Choice                                                                                                                            |
| -- | --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| 1  | Repo strategy                     | Wipe Buildpad app artifacts; preserve `.kiro/steering`, `.kiro/skills`, `.vscode/`, `.git/`, `.gitignore`.                         |
| 2  | Deployment target                 | Cloud-first; OKE Basic Cluster on Oracle Cloud Infrastructure via SST on day 1.                                                   |
| 3  | AI realism                        | Switchable strategy. Default `pretrained` (YOLOv8n COCO from MinIO). `mock` for CI/tests. `custom` slot reserved.                  |
| 4  | OCI / DNS readiness               | OCI tenancy `ap-singapore-1`. User supplies tenancy OCID, user OCID, API key fingerprint, private key, compartment OCID. Domain TBD — sslip.io fallback. |
| 5  | TiDB                              | Operator with demo sizing initially (1 PD + 1 TiKV + 1 TiDB on free tier); production sizing (3+3+2) when scaling out.            |
| 6  | Auth model                        | BFF — SvelteKit hooks own OIDC PKCE; HttpOnly Secure SameSite=Lax cookies; SvelteKit server forwards bearer JWT to Go.            |
| 7  | Compute shape                     | `VM.Standard.E4.Flex` burstable (AMD x86_64), 2 OCPU / 16 GB per node, BASELINE_1_8 utilization. No GPU.                          |
| 8  | Keycloak seeding                  | `simaops-realm.json` checked into git, auto-imported by Helm; 5 demo users (one per role) with passwords from K8s Secret.         |
| 9  | CI/CD                             | GitHub Actions + `ghcr.io` (registry stays cloud-neutral); OCI API key auth via repo secrets.                                     |
| 10 | Domain reality                    | Sima Arome — natural extracts manufacturer; both raw botanicals AND extract/powder products; cold-chain to −20 °C; IBC/IPPC drums.|
| 11 | Demo images                       | User supplies images at runtime; AI worker `findings_map.yaml` is config-driven so vocab swaps without code changes.              |
| 12 | Observability                     | `kube-prometheus-stack` + `loki-stack` + `tempo` + `opentelemetry-collector` (bundle, not individual charts).                     |
| 13 | Multi-tenancy                     | Single tenant, no `tenant_id` columns.                                                                                            |
| 14 | i18n                              | EN + ID locales from day 1.                                                                                                       |
| 15 | Model storage                     | ONNX model in MinIO `simaops-model-artifacts`; AI worker downloads at pod startup; cached on tmpfs.                               |
| 16 | Topology (production)             | web×2, api×2, ai-worker 1→3 HPA on NATS lag, outbox-publisher singleton via Kubernetes `Lease` leader election.                   |
| 17 | PDBs                              | `minAvailable: 1` on web/api; none on outbox-publisher (singleton).                                                               |
| 18 | Alerting                          | PrometheusRule set wired to Alertmanager with log/console output (no PagerDuty/Slack).                                            |
| 19 | Connect transport                 | Connect/JSON for browser↔api (DevTools-debuggable); Connect/Protobuf service↔service.                                            |
| 20 | Sponsors (Xtremax/AWS/Buildpad)   | Branding only; no implementation impact.                                                                                          |

## 6. Domain Reality (Sima Arome)

- **Lot material classes (enum):** `RAW_BOTANICAL`, `EXTRACT`, `POWDER`, `OTHER`.
- **Storage requirement (structured JSON on `lots`):**
  - `temperature_range`: `"ambient"` (15–25 °C), `"cold"` (2–8 °C), `"deep_freeze"` (−20 to −4 °C).
  - `hazard_class`: `null`, `"IBC"`, or `"IPPC"`.
- **Warehouse locations:** support negative temperatures down to −25 °C; `hazard_allowed` and `drum_compatibility` are JSON arrays (e.g., `["IBC","IPPC"]`).
- **AI findings vocabulary (config-driven, per material class):**
  - `RAW_BOTANICAL`: ripeness signals, color anomaly, foreign matter, human artifact.
  - `EXTRACT_POWDER`: container visible, color inconsistency, contamination artifact, lumping, human artifact.

## 7. RBAC Roles

| Role             | Primary capabilities                                              |
| ---------------- | ----------------------------------------------------------------- |
| OPERATOR         | Create lot, upload QC image, trigger QC job, view own lots        |
| QC_SUPERVISOR    | Approve / reject / recheck QC; override AI with reason            |
| WAREHOUSE_STAFF  | Assign warehouse slot                                             |
| MANAGER          | Read-only dashboards, audit views                                 |
| ADMIN            | All of the above + user/role management; bypass restrictions      |

## 8. Reliability Requirements

- Idempotency keys on every mutating RPC.
- Outbox pattern for DB → NATS atomicity.
- NATS JetStream persistence + replay; DLQ for poison messages.
- `/healthz` (liveness) and `/readyz` (deps) on every service.
- Atlas migrations versioned and reversible.
- Structured JSON logs everywhere; OpenTelemetry traces span browser → BFF → api → outbox-publisher → NATS → AI worker.
- HPA on web/api/ai-worker; PDBs on web/api; outbox-publisher singleton via `Lease`.
- Readiness depends on TiDB ping, MinIO HEAD, NATS connect, Keycloak JWKS reachability.

## 9. Security Requirements

- All frontend ↔ Go API requests carry a Keycloak-signed JWT verified server-side (signature, issuer, audience, exp).
- Frontend role checks are UX-only; authorization is server-side, deny-by-default.
- MinIO buckets are private; access is exclusively via short-lived presigned URLs.
- Secrets: OpenBao on production; Kubernetes Secrets in dev/staging.
- Audit log entry for every state-changing RPC. Manual QC override requires a written reason.
- NetworkPolicy denies pod-to-pod traffic except declared edges.

## 10. Plan Versioning & Approvals

| Revision | Date         | Notes                                                                       |
| -------- | ------------ | --------------------------------------------------------------------------- |
| v1       | 2026-05-25   | Initial 30-task plan from spec + Q&A rounds 1–4.                            |
| v2       | 2026-05-26   | Sima Arome domain refinements folded into Tasks 5, 7, 12, 26, 27, 28, 30.   |
| v3       | 2026-05-26   | Cloud target switched from GCP/GKE to OCI/OKE (Singapore, E4.Flex burstable, OCI API key auth). |

**Awaiting:** explicit `go` from the user before Task 1 (monorepo bootstrap) executes.

**Version Policy:** Always use the latest stable versions of all dependencies, runtimes, and platform services (Kubernetes, Helm charts, npm packages, Go modules, Python packages). Never hardcode outdated versions — query the provider/registry for supported versions at deploy time.

## Realtime UI updates (added in plan v5)

- **R-RT-1** Web UI must reflect lot/QC/warehouse state changes within ≤2 seconds without manual refresh, on Dashboard, Lots, Lot detail, QC queue, Warehouse, and Audit pages.
- **R-RT-2** Sidebar nav badges (QC review pending, warehouse pending) must update live in under 2 seconds when an event affects them.
- **R-RT-3** Operators must see realtime events only for lots they created (owner-scoped). Supervisors and managers see all events in their role's permission scope.
- **R-RT-4** Role-targeted toast notifications (e.g. "QC needs review", "Lot ready for slot") must be deduped across browser tabs.
- **R-RT-5** Mid-stream access-token rotation must not disconnect the user or lose in-flight form data. Three-tier recovery: force refresh → silent OIDC renew → popup login. Form drafts persisted to localStorage every 500ms.
- **R-RT-6** API↔Keycloak clock skew >30s must fail startup probe; >60s for 3 consecutive readiness polls fails readiness (after JWT 60s leeway).
- **R-RT-7** Per-user SSE connection cap (default 10) prevents runaway tabs / DDoS; admin role-change immediately kicks affected user's open streams.
