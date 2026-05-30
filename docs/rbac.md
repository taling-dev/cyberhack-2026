# RBAC

## Roles

| Role | Description |
|---|---|
| OPERATOR | Lot intake, QC image upload, trigger QC jobs |
| QC_SUPERVISOR | Approve/reject/recheck QC results |
| WAREHOUSE_STAFF | Assign warehouse slots; create and advance dispatches |
| MANAGER | View dashboards and audit logs |
| ADMIN | Full access + user/role management |

## RPC × Role Matrix

| RPC | OPERATOR | QC_SUPERVISOR | WAREHOUSE_STAFF | MANAGER | ADMIN |
|---|---|---|---|---|---|
| CreateLot | ✓ | | | | ✓ |
| GetLot | ✓ | ✓ | ✓ | ✓ | ✓ |
| ListLots | ✓ | ✓ | ✓ | ✓ | ✓ |
| UpdateLotStatus | ✓ | | | | ✓ |
| CreateQCUploadUrl | ✓ | | | | ✓ |
| CreateQCJob | ✓ | | | | ✓ |
| GetQCJob | ✓ | ✓ | ✓ | ✓ | ✓ |
| GetQCResult | ✓ | ✓ | ✓ | ✓ | ✓ |
| ReviewQC | | ✓ | | | ✓ |
| RetryQCJob | | ✓ | | | ✓ |
| ListLocations | ✓ | ✓ | ✓ | ✓ | ✓ |
| RecommendSlot | | | ✓ | | ✓ |
| AssignSlot | | | ✓ | | ✓ |
| GetWarehouseAssignments | ✓ | ✓ | ✓ | ✓ | ✓ |
| CreateDispatch | | | ✓ | ✓ | ✓ |
| GetDispatch | ✓ | ✓ | ✓ | ✓ | ✓ |
| ListDispatches | ✓ | ✓ | ✓ | ✓ | ✓ |
| UpdateDispatchStatus | | | ✓ | ✓ | ✓ |
| ListAuditLogs | | | | ✓ | ✓ |
| GetEntityAuditTrail | | | | ✓ | ✓ |
| GetOpsDashboard | | | | ✓ | ✓ |
| GetQCMetrics | | | | ✓ | ✓ |
| GetWarehouseMetrics | | | | ✓ | ✓ |
| ListUsers | | | | | ✓ |
| AssignRole | | | | | ✓ |
| RevokeRole | | | | | ✓ |
| ListRoles | | | | | ✓ |

## Enforcement

- **Server-side only** — frontend role checks are UX hints, never authorization
- **Deny by default** — unknown RPCs return 403
- **JWT-based** — roles extracted from `realm_access.roles` in Keycloak token
- **Table-driven** — `internal/auth/rbac.go` maps every RPC to required roles
