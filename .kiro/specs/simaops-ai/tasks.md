# SARI (Sima Arome Resource Intelligence) — Task Breakdown

> Reads from: [`requirements.md`](./requirements.md) and [`design.md`](./design.md)
> Plan revision: **v2** (Sima Arome refinements folded into Tasks 5, 7, 12, 26, 27, 28, 30)

Each task is a vertical, demoable increment. No big jumps in complexity; no orphaned code. Tests are part of every task.

## Phase 1 — Foundation (Tasks 1–6)

### Task 1: Bootstrap monorepo and preserve `.kiro` knowledge
- **Objective:** turn the repo into the SimaOps monorepo skeleton without losing the `.kiro` steering/skills.
- **Implementation:** remove Buildpad-era artifacts (`README.md`, `amplify.yml`, `.env.local`, `.github/copilot-instructions.md`, `kirocli-x86_64-linux.zip*`, the two WhatsApp images). Keep `.kiro/`, `.vscode/`, `.git*`. Create `apps/`, `proto/`, `db/`, `deploy/`, `infra/`, `docs/`. Add root `package.json` (pnpm workspace), `go.work`, `Makefile` (placeholder targets `lint gen build test up down`), root `.gitignore`, fresh `README.md`, `.editorconfig`, `.tool-versions` (Node 20, Go 1.22, Python 3.12, pnpm 9).
- **Tests:** `make lint` exits 0; placeholder CI workflow `lint.yaml` runs on PR.
- **Demo:** `git status` shows the new structure; `make` prints help; `.kiro` skills still load in Kiro IDE.

### Task 2: Define Protobuf service contracts and Buf code-gen pipeline
- **Objective:** lock the API contract before any handler code, with schema-validated requests.
- **Implementation:** write `.proto` files under `proto/simaops/{lot,qc,warehouse,audit,dashboard,admin}/v1/` covering all RPCs from the spec, with Protovalidate annotations for required fields and idempotency-key formats. Configure `buf.yaml`, `buf.gen.yaml` to generate Go (`apps/api/internal/gen/`), TS Connect-ES (`apps/web/src/lib/gen/`), Python types (`apps/ai-worker/simaops_proto/`). `make gen` runs `buf lint && buf format -w && buf generate`.
- **Tests:** `buf lint` clean; CI guard asserts committed generated code matches `make gen`.
- **Demo:** `make gen` produces full Go/TS/Python stubs for all 25+ RPCs; `git diff` is empty.

### Task 3: Local platform stack via Docker Compose
- **Objective:** every developer has a one-command local stack matching the cloud topology.
- **Implementation:** `docker-compose.yml` brings up TiDB single-node (`pingcap/tidb:v7.5`), MinIO with `simaops-*` buckets created by `mc` init container, NATS JetStream (`-js`), Keycloak with `--import-realm` mounting `deploy/keycloak/simaops-realm.json`, OTel Collector + Jaeger all-in-one for local tracing. Healthchecks on each service; Make targets `stack-up`, `stack-down`, `stack-reset`.
- **Tests:** `make stack-up` then `scripts/wait-for-stack.sh` polls each service's health endpoint; CI integration job runs the wait script.
- **Demo:** MinIO console at :9001, Keycloak at :8080, Jaeger at :16686, TiDB at :4000 reachable; realm exists.

### Task 4: Go API skeleton with connect-go, health endpoints, structured logging
- **Objective:** a Go service that compiles, serves Connect RPCs, has health/ready endpoints, and emits structured JSON logs.
- **Implementation:** `apps/api/cmd/api/main.go` wires `chi` + Connect handler registration; embed a no-op `LotService` returning `Unimplemented`. Implement `/healthz` (always 200) and `/readyz` (TiDB ping, MinIO HEAD, NATS connect, Keycloak JWKS reachability). `slog` JSON handler with request_id middleware. Multi-stage distroless Dockerfile. Make targets `make build-api`, `make run-api`.
- **Tests:** Go unit tests for health handlers (testify); Connect smoke test using `connectrpc.com/connect` test client confirms `Unimplemented`; CI runs `go test ./...`.
- **Demo:** `curl /healthz` → 200; `buf curl` to `LotService/CreateLot` returns `Unimplemented`; logs are JSON.

### Task 5 (revised): TiDB schema, Atlas migrations, sqlc codegen
- **Objective:** the full schema lives in versioned migrations with material-class enum and structured cold-chain JSON.
- **Implementation:** Atlas SQL migrations in `db/migrations/` for the eleven tables. Indexes (`outbox_events(status, created_at)`, `audit_logs(entity_type, entity_id)`, `lots(status)`). Sima-specific shape:
  - `lots.material_type` ENUM(`RAW_BOTANICAL`, `EXTRACT`, `POWDER`, `OTHER`) NOT NULL.
  - `lots.storage_requirement` JSON `{ "temperature_range": "ambient"|"cold"|"deep_freeze", "hazard_class": null|"IBC"|"IPPC" }`.
  - `warehouse_locations.temperature_min/max` allow negatives down to −25 °C.
  - `warehouse_locations.hazard_allowed` JSON array.
  - `warehouse_locations.drum_compatibility` JSON array (`["IBC","IPPC"]`).
  Document JSON shapes in `db/migrations/README.md`. `sqlc.yaml` MySQL dialect; queries in `apps/api/internal/db/queries/*.sql`. Make targets `db-migrate`, `db-rollback`, `sqlc`.
- **Tests:** Atlas dry-run + idempotent migrate-up/down integration test against local TiDB; sqlc generation idempotency check; constraint test rejects unknown `material_type`.
- **Demo:** `make db-migrate` creates all tables; `DESCRIBE lots` shows the enum and JSON columns; sqlc-generated Go code compiles inside `apps/api`.

### Task 6: SvelteKit shell with Tailwind, shadcn-svelte, i18n, Connect-ES, TanStack Query, Zod
- **Objective:** runnable frontend with the full app chrome, EN/ID locale switcher, and a typed Connect-ES client wired to the local Go API.
- **Implementation:** `pnpm create svelte` with TypeScript + Node adapter (BFF needs server). Add Tailwind, shadcn-svelte (button, card, table, dialog, sheet, dropdown), `@connectrpc/connect-web` + generated client, `@tanstack/svelte-query`, `zod`, `svelte-i18n` with `en.json` + `id.json`. Layout: top bar (locale switcher + user-menu placeholder), sidebar (role-aware nav placeholders), main content slot. Routes scaffolded but empty: `/lots`, `/qc`, `/warehouse`, `/audit`, `/dashboard`, `/admin`. Dockerfile (Node 20 alpine).
- **Tests:** Vitest test for locale switcher (toggles store, persists in cookie); Playwright smoke loads `/` and asserts layout in both locales.
- **Demo:** `pnpm dev` opens `:5173`; locale switcher swaps EN/ID; DevTools shows Connect-ES making a (currently Unimplemented) call to the API.

## Phase 2 — Core workflow (Tasks 7–14)

### Task 7 (revised): LotService — Create / Get / List with material-aware UI
- **Objective:** an operator can create lots of any material class; the form captures cold-chain + drum-class requirements that drive later warehouse routing.
- **Implementation:** Go handlers backed by sqlc; Protovalidate enforces `material_type` enum and the `storage_requirement` JSON. `lot_number` autogenerated `LOT-YYYY-MMDD-XXX`. SvelteKit `/lots/new` form: material-class radio (RAW_BOTANICAL → ambient default no drum; EXTRACT/POWDER → cold default + drum-class radio; deep_freeze selectable for any class). Zod schema mirrors proto. `/lots` list with material-type + status filters. Detail page shows structured storage requirement.
- **Tests:** Go handler tests (validation per material_type); Vitest for conditional form fields; Playwright e2e creates one lot of each material type and asserts JSON persists exactly.
- **Demo:** operator creates `LOT-2026-...-001` (raw botanical, ambient), `-002` (extract, cold, IBC), `-003` (powder, deep_freeze, IPPC); all three appear in filterable list.

### Task 8: QCService.CreateQCUploadUrl + browser-side MinIO upload
- **Objective:** operator uploads QC images directly to MinIO via presigned URL — never through the API.
- **Implementation:** Go uses MinIO Go SDK to generate 15-minute PUT presigned URL into `simaops-qc-images/{lot_id}/{uuid}.{ext}`; returns `object_key + upload_url + expires_at`. SvelteKit upload component uses `fetch` PUT against the URL with progress bar; on success, stores `object_key` in component state. Lot detail page gets "Upload QC image" button (disabled unless lot status ∈ {DRAFT, PENDING_QC, QC_REJECTED}).
- **Tests:** Go test asserts presigned URL is valid against local MinIO; Playwright e2e uploads a fixture and asserts file appears via `mc ls`.
- **Demo:** operator opens lot detail, uploads a JPEG; file appears in MinIO console under `simaops-qc-images/<lot_id>/`.

### Task 9: QCService.CreateQCJob + inline mock AI result (transitional)
- **Objective:** close the loop on QC creation so the supervisor review screen has data, before NATS exists.
- **Implementation:** handler creates `qc_jobs` row (`status=QUEUED`), advances lot to `PENDING_QC`. Inline, synchronously: a mock function reads image bytes (to confirm fetchability), produces a deterministic stub QCResult (`recommendation=REVIEW`, confidence 0.82, fixed findings from a `qcfixtures` package), inserts into `qc_results`, advances job to `AI_COMPLETED`, advances lot to `QC_REVIEW`. **This inline mock is removed in Task 19** when NATS + Python worker take over.
- **Tests:** handler test; integration test confirms a row in `qc_results` after `CreateQCJob`.
- **Demo:** after upload from Task 8, clicking "Start QC" triggers an immediate mock result; lot status visibly progresses `DRAFT → PENDING_QC → QC_REVIEW`.

### Task 10: QCService.GetQCJob / GetQCResult + supervisor review page
- **Objective:** a QC supervisor sees AI recommendation and original image side by side.
- **Implementation:** SvelteKit `/qc` lists jobs (`status=NEEDS_HUMAN_REVIEW` ≈ lot `QC_REVIEW` for now); detail `/qc/[id]` shows the image (presigned GET URL via Go), AI recommendation, confidence, findings list, Approve/Reject/Recheck buttons (no-op until Task 11).
- **Tests:** Go handler tests; Vitest for review-card component; Playwright loads page after Task 9 and asserts image + findings visible.
- **Demo:** supervisor opens `/qc`, sees the just-created job, opens it, image renders with AI advisory text alongside.

### Task 11: QCService.ReviewQC — supervisor decisions with reason
- **Objective:** human-in-the-loop approval/rejection with mandatory reason on overrides; lot status transitions correctly.
- **Implementation:** Go handler validates state machine (only `QC_REVIEW` lots can be reviewed); updates `qc_results.supervisor_decision`, `reviewed_by`, `reviewed_at`, `review_reason`; advances lot to `QC_APPROVED` or `QC_REJECTED`; sets `qc_jobs.status` accordingly. Validation: rejection requires non-empty reason; approval requires reason if AI recommended FAIL/REVIEW (override). SvelteKit modal collects reason.
- **Tests:** Go handler tests (state transitions, reason-required); Playwright e2e: approve happy path + reject-without-reason rejected by API + override-with-reason accepted.
- **Demo:** supervisor approves the lot; lot detail shows `QC_APPROVED`; "Awaiting QC" filter on `/lots` empties.

### Task 12 (revised): WarehouseService — list, recommend, assign with cold-chain + drum filters
- **Objective:** warehouse staff assigns approved lots to slots that fit temperature and drum requirements; lot reaches `READY_FOR_PRODUCTION`.
- **Implementation:** seed `warehouse_locations` (`db/seed/data.sql`) — 12 slots across 3 zones: Zone A ambient (15–25 °C, no hazard, IBC+IPPC), Zone B cold (2–8 °C, IBC+IPPC, no hazard), Zone C deep-freeze (−20 to −4 °C, IBC only, hazard `["IBC"]`). `RecommendSlot` filters by (a) `lot.storage_requirement.temperature_range` mapped to `(min,max)` band intersecting slot range, (b) `hazard_class ∈ slot.hazard_allowed` (or both null), (c) `hazard_class ∈ slot.drum_compatibility` when non-null, (d) `capacity > 0`; sort ambient < cold < deep_freeze (cost). `AssignSlot` creates `warehouse_assignments`; advances lot through `WAREHOUSE_ASSIGNED → READY_FOR_PRODUCTION` transactionally. SvelteKit `/warehouse` shows queue grouped by zone with recommendation pre-selected and "why this slot?" tooltip.
- **Tests:** Go handler tests (no-compatible-slot, capacity=0, hazard mismatch, deep-freeze routing); Playwright e2e for each of the three demo lots ends in the right zone.
- **Demo:** warehouse staff sees the deep-freeze powder lot recommended into Zone C with tooltip "matches deep_freeze (−20 to −4 °C) + IPPC drum"; assignment lands lot in `READY_FOR_PRODUCTION`.

### Task 13: Audit middleware + AuditService + per-entity timeline UI
- **Objective:** every state-changing RPC writes an audit log; UI shows lot timeline.
- **Implementation:** Connect interceptor wraps mutation handlers, captures actor (dev token until Task 15), action name, entity type/id, before/after JSON snapshots, request_id, trace_id. Implement `AuditService.ListAuditLogs` (paginated) and `GetEntityAuditTrail(entity_type, entity_id)`. SvelteKit `/audit` route + "Timeline" tab on lot detail.
- **Tests:** interceptor unit test verifies entry format; integration test asserts `CreateLot` produces exactly one audit row; Playwright asserts timeline appears on lot detail.
- **Demo:** the lot from prior tasks shows a chronological audit trail (created → uploaded → QC submitted → AI completed → approved → assigned → ready) on its detail page.

### Task 14: DashboardService + manager dashboard
- **Objective:** manager view that ties operational KPIs together.
- **Implementation:** `GetOpsDashboard` returns lot counts by status, today's intake, lots awaiting QC, lots ready for production. `GetQCMetrics` returns 24h pass/review/fail rates and average AI confidence. `GetWarehouseMetrics` returns occupancy by zone and free capacity. SvelteKit `/dashboard` renders cards + status-distribution donut (vanilla SVG to avoid heavy deps).
- **Tests:** Go handler tests with seeded data assert metric arithmetic; Playwright snapshot.
- **Demo:** manager opens `/dashboard`, sees today's lot counts and that 1 lot just reached `READY_FOR_PRODUCTION`.

## Phase 3 — Enterprise reliability (Tasks 15–20)

### Task 15: Keycloak BFF auth — SvelteKit cookies + bearer forwarding
- **Objective:** real OIDC PKCE login replacing the dev token; HttpOnly cookie session; Go gets a verified JWT on every call.
- **Implementation:** `apps/web/src/hooks.server.ts` runs OIDC code+PKCE flow against the Keycloak realm; on callback exchanges code for `access_token` + `refresh_token`; sets two HttpOnly Secure SameSite=Lax cookies (`sa_access`, `sa_refresh`) signed by SvelteKit. Server-side fetch wrapper attaches `Authorization: Bearer ${cookie.access}` to outbound Connect-ES; on 401, runs refresh once then retries. Login page redirects `/auth/login` → Keycloak → `/auth/callback`. Logout clears cookies + Keycloak session.
- **Tests:** Vitest hook test mocks Keycloak responses; Playwright e2e: full login flow with `operator/Operator123!` lands on `/lots` authenticated.
- **Demo:** hitting `/lots` while logged out redirects to Keycloak; logging in as `operator` lands back on `/lots`; cookies are HttpOnly and access token isn't visible in page.

### Task 16: Go OIDC verification + RBAC + AdminService
- **Objective:** backend enforces who can do what; admin can manage role assignments.
- **Implementation:** middleware fetches Keycloak JWKS, verifies signature/issuer/audience/exp; extracts `sub`, `preferred_username`, `realm_access.roles`. Table-driven RBAC (`internal/auth/rbac.go`): each RPC declares required roles; decorator denies with `permission_denied` otherwise. Implement `AdminService.ListUsers/AssignRole/RevokeRole/ListRoles` (writes `users_profile`, `user_roles`; mirrors changes to Keycloak via Admin API). Only `ADMIN` may call AdminService. SvelteKit `/admin/users` (visible only when role list contains ADMIN).
- **Tests:** middleware unit tests (valid/expired/wrong-audience); RBAC table tests for every RPC × role; Playwright: operator gets 403 on `/admin/users`; admin assigns WAREHOUSE_STAFF to operator and operator gains warehouse access on next login.
- **Demo:** switching between the 5 demo users visibly reshapes navigation; unauthorized RPCs return 403.

### Task 17: Idempotency middleware + idempotency_keys table
- **Objective:** replays of mutating RPCs return cached responses, never duplicate state.
- **Implementation:** Go middleware on mutating RPCs hashes `(user_id, operation, idempotency_key)` to `key_hash`; on first call writes a row in a transactional `INSERT idempotency_keys ... <handler>... UPDATE response_json ... COMMIT` flow; on conflict returns cached `response_json`. Hash includes request body to catch key reuse with different payload (return `failed_precondition`). 24-hour TTL via `created_at` index. SvelteKit Connect interceptor auto-generates `idempotency_key = uuid_v4()` per user action and persists across retries.
- **Tests:** Go test (concurrent identical requests → one DB write + same response); mismatched body with same key returns `failed_precondition`; Playwright double-submit CreateLot via fast double-click → only one lot.
- **Demo:** clicking "Create lot" twice produces one row; replaying via `buf curl` with same key returns cached response.

### Task 18: Outbox pattern + outbox-publisher with leader election
- **Objective:** at-least-once event delivery from TiDB to NATS without two-phase commit.
- **Implementation:** mutating handlers that should emit a domain event write to `outbox_events` inside the same transaction (`status=PENDING`). New `apps/outbox-publisher/` Go service: `client-go` `leaderelection` using a `Lease` in the app namespace; leader polls `outbox_events WHERE status='PENDING' ORDER BY created_at LIMIT 100` every 500 ms; publishes each to NATS JetStream with `Nats-Msg-Id = outbox_events.id` (NATS dedupes), then `UPDATE status='PUBLISHED', published_at=NOW()`. On publish failure: `retry_count++`, exponential backoff cap 60s; after `retry_count >= 10` mark `FAILED` and emit a metric.
- **Tests:** integration test (write row → assert NATS receives); chaos test kills publisher mid-flight → no duplicate publish; leader-election test (two replicas → only one publishes).
- **Demo:** triggering CreateQCJob writes both a `qc_jobs` row and an `outbox_events` row; within ~1 s the publisher emits to subject `qc.job.created`; row flips to `PUBLISHED`.

### Task 19: NATS-driven Python AI worker (replaces Task 9 inline mock)
- **Objective:** real event-driven AI processing with retries and DLQ; Task 9 inline mock is removed and the supervisor flow continues unchanged.
- **Implementation:** Go API stops calling inline mock — `CreateQCJob` only writes job + outbox event. `apps/ai-worker/` Python FastAPI exposes `/healthz`/`/readyz`; runs a NATS JetStream consumer (durable `simaops-ai-worker`, `max_deliver=4`, `ack_wait=60s`, DLQ subject `qc.job.dlq`). Strategy interface `QCStrategy` with `MockStrategy` (default in this task) returning the same fixture as before. On each message: download image from MinIO, run strategy, upsert `qc_results` (idempotent by `qc_job_id`), advance `qc_jobs.status`, publish `qc.job.completed` or `qc.job.failed`. Failures > 4 deliveries land in DLQ. New RPC `QCService.RetryQCJob` republishes a DLQ'd job to the main subject.
- **Tests:** pytest with NATS test container (happy path + DLQ path); integration test confirms Task 9 UI flow still works; chaos test kills worker mid-processing → no double-write.
- **Demo:** full flow re-runs end-to-end via NATS; Jaeger shows trace crossing api → publisher → NATS → worker; deleting the image from MinIO before worker pulls lands the job in DLQ; retry RPC recovers it.

### Task 20: OpenTelemetry tracing + structured JSON logs across all four services
- **Objective:** a single `trace_id` follows a click from browser → SvelteKit BFF → Go API → outbox-publisher → NATS → Python worker.
- **Implementation:** SvelteKit emits trace context via `traceparent` header; Go API propagates via `otelhttp` + Connect interceptor; outbox-publisher carries trace via NATS message header (`traceparent` in JetStream headers); Python worker extracts the header and continues the trace. Audit logs and idempotency rows record `trace_id`. OTLP exporter targets local Jaeger (compose) and OTel Collector (cloud). All four services log JSON with `trace_id` + `request_id`.
- **Tests:** integration test asserts a single span tree of ≥6 spans for one CreateQCJob across services; log assertions for trace_id presence.
- **Demo:** searching Jaeger by the request_id shown in a SvelteKit toast surfaces a single trace with realistic span durations across the entire flow.

## Phase 4 — Deployment (Tasks 21–27)

### Task 21: Helm charts for the four app services
- **Objective:** each app deployable with a single `helm upgrade --install`.
- **Implementation:** under `deploy/helm/simaops-{web,api,ai-worker,outbox-publisher}` create charts with: `Deployment`, `Service`, `ConfigMap`, `Secret` ref (`envFrom: secretRef`), `ServiceAccount`, `HorizontalPodAutoscaler` (web/api on CPU; ai-worker on a custom NATS-lag metric via `prometheus-adapter`; CPU fallback with TODO if needed for hackathon timeline), `PodDisruptionBudget` (web/api), `NetworkPolicy` allowing only ingress→web, web→api, api→tidb/minio/nats, worker→nats/minio/tidb, all egress to OTel Collector + Vault, `ServiceMonitor` (kube-prometheus-stack CRD), `PrometheusRule` template. Outbox-publisher chart enforces `replicas=1` with `Lease` name parameterized. `make helm-lint` runs `helm lint` on all charts.
- **Tests:** `helm template` produces valid YAML for `dev`, `staging`, `production` value sets; `kubeconform` validates against matching k8s API; smoke `helm install` against local k3d in CI verifies pods reach Ready.
- **Demo:** `make k3d-up && make helm-deploy-dev` brings up full app stack on local k3d; `kubectl get pods -n simaops` shows all four services Ready.

### Task 22 (revised — OCI): SST config — provision OKE Basic + VCN + node pool
- **Objective:** from a clean OCI tenancy, `sst deploy --stage staging` creates the OKE cluster ready for Helm releases.
- **Implementation:** `infra/sst/sst.config.ts` with `@pulumi/oci` provider; provision (a) VCN `10.0.0.0/16`, public subnet, Internet Gateway, Route Table, Security List (ingress 80/443; egress all), (b) OKE Basic Cluster (`v1.30.x`, free control plane, public Kubernetes API endpoint), (c) Node Pool — `VM.Standard.E4.Flex` (AMD x86_64), 2 OCPU / 16 GB, BASELINE_1_8 burstable, 2 nodes pinned to first AD, latest OKE Oracle-Linux-8 image, (d) outputs `clusterOcid`, `vcnId`, `subnetId`, `kubeconfig`. Stages: `dev` (k3d, no OCI), `staging`, `production` (`protect: true`). Auth via local `~/.oci/config` (dev) or env vars `OCI_*` (CI). Reads compartment OCID from `infra/values/{stage}.yaml`.
- **Tests:** `sst diff --stage staging` produces a clean plan; CI dry-run runs `sst diff` on PRs.
- **Demo:** `sst deploy --stage staging` (after `oci setup config` on local machine) ends with a working cluster; `oci ce cluster create-kubeconfig` populates `~/.kube/config`; `kubectl get nodes` shows 2 ready nodes.

### Task 23 (revised — OCI): Helm releases — full platform on OKE
- **Objective:** every platform component the spec requires runs in-cluster on OKE; URLs are reachable over TLS.
- **Implementation:** Apply Helm releases in dependency order via `helm upgrade --install`: namespace creation (`platform`, `simaops`, `observability`) → `ingress-nginx` (auto-provisions OCI Load Balancer with public IP) + `cert-manager` (with a `ClusterIssuer` for Let's Encrypt HTTP-01) → `keycloak` (mounting `simaops-realm.json` from a ConfigMap) → `tidb-operator` → `TidbCluster` CR (demo sizing for free tier: 1 PD + 1 TiKV + 1 TiDB; bump to 3+3+2 for production) → `minio` (Helm chart, single-disk for demo; 4 disks × 100Gi for production with `oci-bv` StorageClass; buckets pre-created by an init Job) → `nats` (JetStream, 1 replica for demo; 3 for production) → `kube-prometheus-stack` + `loki-stack` + `tempo` + `opentelemetry-collector` → `openbao` (single replica staging; raft 3-replica production) → app charts from Task 21. Outputs: `frontend_url`, `api_url`, `keycloak_url`, `minio_console_url`, `grafana_url` (all `<lb-ip>.sslip.io` until a real domain is configured).
- **Tests:** Integration test asserts every Helm release reaches `deployed`; post-deploy `scripts/verify-cluster.sh` curls each external URL and asserts 200/302.
- **Demo:** opening `https://app.<lb-ip>.sslip.io` shows SvelteKit shell over TLS; Keycloak login works; Grafana, MinIO console, Jaeger UI reachable through the same ingress.

### Task 24 (revised — OCI): GitHub Actions CI/CD — build, push to ghcr.io, deploy to OKE
- **Objective:** every commit produces images; merges to `main` deploy to staging; tagged releases promote to production.
- **Implementation:** `.github/workflows/build.yaml` matrix on `apps/{web,api,ai-worker,outbox-publisher}`: build, run unit tests, build OCI image with `docker/build-push-action`, push `ghcr.io/<owner>/simaops-<app>:<sha>` and `:<branch>`. `deploy-staging.yaml`: triggered on push to `main`, configures OCI CLI from repo secrets via `oracle-actions/configure-oci-cli`, runs `oci ce cluster create-kubeconfig` to populate kubeconfig, then `helm upgrade --install` with image tags pinned to commit SHA. `deploy-production.yaml`: triggered on `release` tag, requires `environment: production` approval. GHCR auth via `GITHUB_TOKEN`; OCI auth via repo secrets (`OCI_TENANCY_OCID`, `OCI_USER_OCID`, `OCI_FINGERPRINT`, `OCI_PRIVATE_KEY`, `OCI_REGION`, `OCI_COMPARTMENT_OCID`, `OCI_CLUSTER_OCID`).
- **Tests:** `act` local CI dry-run; first PR demonstrates builds + pushes succeed.
- **Demo:** pushing a trivial change to `main` triggers green pipeline; new pods on staging OKE; `kubectl describe pod` shows the new image tag.

### Task 25: Grafana dashboards + PrometheusRule alerts
- **Objective:** managers and on-call have one place to see operational health and one place where alerts fire.
- **Implementation:** ship 3 Grafana dashboards as ConfigMaps with `grafana_dashboard` label so they auto-load: (a) **Operations** — lots by status over time, today's throughput, QC queue depth, warehouse occupancy; (b) **API** — RPC request rate, error rate, p50/p95/p99 latency per RPC, idempotency hit rate; (c) **AI worker** — NATS consumer lag, inference duration histogram, pass/review/fail breakdown, DLQ depth. PrometheusRule set: `APIErrorRateHigh` (>2% 5m), `APIp95LatencyHigh` (>1s 5m), `NATSConsumerLagHigh` (>100 5m), `OutboxBacklog` (max(now−created_at) for PENDING > 5m), `AIWorkerFailureRateHigh` (>10% 15m), `LotsStuckInQCReview` (>30m without supervisor decision). Alertmanager configured with `null` receiver (logs only).
- **Tests:** `dashboard-linter` validates dashboards; `promtool` rule check; injection test posts 100 high-error responses and asserts alert fires in Alertmanager log.
- **Demo:** Grafana shows live data after a Playwright run hits the cluster; manually inducing 5xx in api fires the alert visibly in Alertmanager UI.

### Task 26 (revised): Demo seed Job — model + Sima-realistic seed data
- **Objective:** a fresh deploy is demo-ready in one step with data that tells the four-focus-area story.
- **Implementation:** one-shot Kubernetes `Job` (`simaops-seed`) as Helm post-install hook; idempotent `mc cp --quiet`-style uploads:
  - `simaops-model-artifacts/yolov8n.onnx` from Ultralytics' release (single source of truth for v1).
  - `simaops-model-artifacts/findings_map.yaml` (per-material-class config from Task 28).
  - User-supplied images from `db/seed/images/{raw_botanical,extract,powder}/*` into matching subprefixes of `simaops-qc-images/seed/`.
  - Runs `db/seed/data.sql`: 5 demo users in `users_profile` (uuid matched to Keycloak `sub`); 12 warehouse locations from Task 12; three demo lots — one `RAW_BOTANICAL` (ambient), one `EXTRACT` (cold + IBC), one `POWDER` (deep_freeze + IPPC) — all in `DRAFT`.
- **Tests:** Job logs success; integration test confirms `mc ls simaops-model-artifacts` returns the model, `SELECT COUNT(*) FROM warehouse_locations` returns 12, `SELECT COUNT(*) FROM lots` returns 3, and grouping by material_type shows one of each.
- **Demo:** fresh `sst deploy --stage staging` followed by zero manual steps — `/dashboard` shows seeded lots, `/warehouse` shows locations across three zones, `/admin/users` shows all five demo users.

### Task 27 (revised): Documentation pack with focus-area traceability
- **Objective:** the docs the spec demands exist; `architecture.md` maps SimaOps capabilities back to Sima Arome's four hackathon focus areas.
- **Implementation:** six docs:
  - `docs/architecture.md` — system diagram + explanations + a "Focus Area Coverage" section: (1) Integrated Operations System → unified data model + Connect RPC + audit + dashboard; (2) AI for Fruit & Raw-Material QC → AI worker `pretrained` strategy with `RAW_BOTANICAL` vocabulary; (3) AI for Extract & Powder QC → same strategy with `EXTRACT_POWDER` vocabulary; (4) AI-Assisted Warehousing & Cold-Chain → Warehouse recommender with temperature/hazard/drum constraints. Out-of-scope for v1: sample-dispatch tracking, lab-instrument sensor ingestion, PPIC schedule generation.
  - `docs/api-contract.md` — Connect RPC method index with request/response examples generated from `buf` reflection.
  - `docs/rbac.md` — role × RPC matrix.
  - `docs/audit-log.md` — entry shape + retention.
  - `docs/deployment.md` — SST runbook from clean GCP project to live URL, including Workload Identity Federation setup.
  - `docs/demo-script.md` — restructured 10-minute single end-to-end story (see Task 30).
  README links to all six.
- **Tests:** `markdownlint` clean; CI link-check across the six documents.
- **Demo:** a teammate following `docs/deployment.md` from a clean GCP project reaches a working URL in <1 hour; `architecture.md` answers "which Sima Arome challenge does each component solve?" without ambiguity.

## Phase 5 — AI polish (Tasks 28–30)

### Task 28 (revised): Pretrained ONNX strategy with material-class findings vocabularies
- **Objective:** real YOLOv8n inference produces findings whose names match Sima Arome's QC reality, with vocabulary config-driven so the model can be swapped without code changes.
- **Implementation:** `apps/ai-worker/config/findings_map.yaml` ships with two vocabularies (RAW_BOTANICAL + EXTRACT_POWDER — see `design.md` §7). `PretrainedStrategy` selects vocabulary by `lot.material_type` carried in NATS message; on first message of a model, downloads `yolov8n.onnx` and `findings_map.yaml` from MinIO (cached on tmpfs); OpenCV preprocess (resize 640×640, BGR→RGB, NCHW); ONNX Runtime CPU inference; NMS; map detections to findings + recommendation per rules; render annotated PNG with bounding boxes labeled by mapped finding name; upload to `simaops-qc-results/{qc_job_id}.png`. `model_version` env var (e.g., `yolov8n-coco-v8.1.0`) recorded on every QCResult. `mock` strategy remains for CI/tests.
- **Tests:** pytest fixtures for one image of each material class assert deterministic findings against a known model + seed; integration test confirms annotated image is uploaded and reachable via presigned GET; YAML schema validation rejects malformed `findings_map.yaml`.
- **Demo:** uploading a raw-botanical image yields findings from the `RAW_BOTANICAL` vocabulary; uploading a powder image yields findings from `EXTRACT_POWDER`; supervisor sees the right vocabulary in both cases; `model_version` visible in audit trail.

### Task 29: QC review UI — annotated image viewer + model version display
- **Objective:** the supervisor sees the AI's reasoning, not just a verdict.
- **Implementation:** `/qc/[id]` toggles between original and annotated images via a switch; lists each finding as a chip; displays `model_version`, confidence, and per-class confidence breakdown; shows audit-reason field prominently when a manual override is being recorded.
- **Tests:** Vitest component test renders both image variants; Playwright snapshot of the review page with annotations.
- **Demo:** supervisor sees boxes around detected objects, can toggle to compare with original, and the model version is visible for traceability.

### Task 30 (revised): End-to-end smoke test on staging GKE + four-focus-area demo rehearsal
- **Objective:** prove the system works on the cloud target and the demo walks all four Sima Arome focus areas in 10 minutes without manual intervention.
- **Implementation:** Playwright e2e suite runs against `https://app.<domain>` via a manually-triggered GitHub Actions workflow; logs in as each of the 5 demo users in sequence and runs the scripted flow:
  1. **Operator** intakes a raw-botanical lot → `Start QC` (focus areas 1 + 2).
  2. **QC Supervisor** opens the resulting `QC_REVIEW`, sees AI findings drawn from `RAW_BOTANICAL` vocabulary, approves with reason.
  3. **Warehouse Staff** sees the approved lot routed to Zone A (ambient), assigns it (focus area 4).
  4. **Operator** intakes an extract-powder lot, uploads image (focus area 3 with powder vocabulary).
  5. **QC Supervisor** rejects this one with reason (negative path; audit).
  6. **Operator** retries with a corrected image; AI returns PASS.
  7. **Warehouse Staff** assigns the powder lot to Zone C (deep-freeze, IPPC).
  8. **Manager** opens `/dashboard` — both lots visible, focus-area-1 unified view; opens audit trail of either lot — full traceability.
  9. **Admin** demonstrates role assignment (`/admin/users`) — proves RBAC controls.
  Plus a chaos path the e2e suite exercises (not in the live demo): kill the AI worker pod mid-message → assert NATS re-delivery and result still lands; force a failure to DLQ → recover via `RetryQCJob`. `make demo-reset` wipes lots/jobs/results and re-runs Task 26 seed.
- **Tests:** the Playwright e2e suite itself; manual rehearsal checklist in `docs/demo-script.md` cross-references each step to the focus area it demonstrates.
- **Demo:** running `make demo-rehearse` from a laptop reproduces the full 10-minute pitch flow against staging GKE without manual intervention; the chaos path runs in CI but not on stage.

---

## Phase Summary

| Phase | Tasks   | Theme                       | Exit criteria                                                                          |
| ----- | ------- | --------------------------- | -------------------------------------------------------------------------------------- |
| 1     | 1–6     | Foundation                  | Monorepo + protos + local stack + Go skeleton + DB + SvelteKit shell.                  |
| 2     | 7–14    | Core workflow               | Operator → mock-QC → supervisor → warehouse → manager flow demoable end-to-end.        |
| 3     | 15–20   | Enterprise reliability      | Real auth, RBAC, idempotency, outbox, NATS-driven AI worker, OpenTelemetry tracing.     |
| 4     | 21–27   | Deployment                  | Helm charts, SST + GKE, full platform stack, CI/CD, alerts, seed Job, docs.             |
| 5     | 28–30   | AI polish                   | Real ONNX inference with material-class vocabularies, annotated images, demo rehearsal. |

## Execution Status

| Date         | Status                                                              |
| ------------ | ------------------------------------------------------------------- |
| 2026-05-26   | Plan v2 saved to `.kiro/specs/simaops-ai/`. Awaiting `go` from user. |

## Plan v5: Realtime Web UI (added 2026-05-27)

All 23 tasks complete; deployed via tag (build/push/helm-upgrade) and verified
end-to-end with `scripts/e2e-realtime.sh`.

| # | Task | Status |
|---|------|--------|
| 1 | Standardize outbox event envelope (envelope.go + qc/lot/warehouse callers + AI worker) | ✓ |
| 2 | API event hub + core NATS subscriptions + filter (in-process fan-out) | ✓ |
| 3 | GET /events SSE handler (connection-info + heartbeats + drain frame) | ✓ |
| 4 | BFF SSE pass-through (forward Accept, Last-Event-ID, X-Refresh-Expires-At, AbortSignal) | ✓ |
| 5 | /auth/heartbeat endpoint + transient/permanent refresh distinction | ✓ |
| 5b | Silent OIDC renew + popup login flows (postMessage HTML callbacks) | ✓ |
| 5c | SessionExpiredModal component | ✓ |
| 6 | Realtime client store + state machine + tier dispatch | ✓ |
| 6b | Form draft persistence (useDraft + sweepOldDrafts) | ✓ |
| 7 | JWT middleware leeway + structured 401 reasons (X-Auth-Failure-Reason) | ✓ |
| 8 | Clock-skew startup probe + fatal /readyz sub-check (3-poll hysteresis) | ✓ |
| 9 | Wire realtime invalidation to all data pages | ✓ |
| 10 | Subtle highlight on changed/new rows (highlightOnChange action) | ✓ |
| 11 | Toaster + role-targeted dispatch + localStorage event_id dedup | ✓ |
| 12 | Live nav badges in sidebar | ✓ |
| 13 | Observability: SSE metrics, Grafana row, alerts | ✓ |
| 14 | Helm tuning: ingress, terminationGracePeriod, preStop, startupProbe, MAX_SSE_PER_USER | ✓ |
| 15 | NTP cloud-init for OKE node template | ✓ |
| 16 | Admin SSE kick endpoint + role-change auto-kick | ✓ |
| 17 | Realm bootstrap: SSO timeouts + refresh rotation | ✓ |
| 18 | scripts/e2e-realtime.sh (full pipeline + 5-user filter assertions) | ✓ |
| 19 | scripts/e2e-token-refresh.sh (4-min rotation resilience) | ✓ |
| 20 | Documentation + spec update | ✓ |
