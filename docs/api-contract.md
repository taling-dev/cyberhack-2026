# API Contract

## Protocol

- **Browser → API:** Connect RPC over JSON (HTTP/1.1, debuggable in DevTools)
- **Service → Service:** Connect RPC over Protobuf (binary, efficient)
- **Base URL:** `https://api.<ip>.sslip.io` (staging)

## Services

### LotService (`simaops.lot.v1`)

| Method | Description | Roles |
|---|---|---|
| `CreateLot` | Create a new lot | OPERATOR, ADMIN |
| `GetLot` | Get lot by ID | Any authenticated |
| `ListLots` | Paginated list with filters | Any authenticated |
| `UpdateLotStatus` | Change lot status | OPERATOR, ADMIN |
| `GetLotTimeline` | Audit trail for a lot | Any authenticated |

### QCService (`simaops.qc.v1`)

| Method | Description | Roles |
|---|---|---|
| `CreateQCUploadUrl` | Get presigned PUT URL for MinIO | OPERATOR, ADMIN |
| `CreateQCJob` | Submit image for AI QC | OPERATOR, ADMIN |
| `GetQCJob` | Get job status | Any authenticated |
| `GetQCResult` | Get AI result + findings | Any authenticated |
| `ReviewQC` | Supervisor approve/reject | QC_SUPERVISOR, ADMIN |
| `RetryQCJob` | Retry a failed/DLQ'd job | QC_SUPERVISOR, ADMIN |

### WarehouseService (`simaops.warehouse.v1`)

| Method | Description | Roles |
|---|---|---|
| `ListLocations` | All warehouse locations | Any authenticated |
| `RecommendSlot` | AI-recommended slots for a lot | WAREHOUSE_STAFF, ADMIN |
| `AssignSlot` | Assign lot to location | WAREHOUSE_STAFF, ADMIN |
| `GetWarehouseAssignments` | List assignments | Any authenticated |

### AuditService (`simaops.audit.v1`)

| Method | Description | Roles |
|---|---|---|
| `ListAuditLogs` | Paginated audit log | MANAGER, ADMIN |
| `GetEntityAuditTrail` | Trail for entity | MANAGER, ADMIN |

### DashboardService (`simaops.dashboard.v1`)

| Method | Description | Roles |
|---|---|---|
| `GetOpsDashboard` | Lot counts, KPIs | MANAGER, ADMIN |
| `GetQCMetrics` | Pass/review/fail rates | MANAGER, ADMIN |
| `GetWarehouseMetrics` | Zone capacity | MANAGER, ADMIN |

### AdminService (`simaops.admin.v1`)

| Method | Description | Roles |
|---|---|---|
| `ListUsers` | All users with roles | ADMIN |
| `AssignRole` | Add role to user | ADMIN |
| `RevokeRole` | Remove role from user | ADMIN |
| `ListRoles` | Available roles | ADMIN |

## Idempotency

All mutation RPCs accept an `idempotency_key` field. Replaying the same key returns the cached response (24h TTL).

## Proto Definitions

Source: `proto/simaops/{lot,qc,warehouse,audit,dashboard,admin}/v1/*.proto`

Generate stubs: `make gen` (requires `buf` CLI)
