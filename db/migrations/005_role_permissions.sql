-- SARI (Sima Arome Resource Intelligence): Data-driven RBAC
-- Roles become assignable permission sets. ADMIN keeps a hardcoded bypass in
-- code; public (any-authenticated) procedures also stay in code. This table
-- holds the per-role RPC grants for every other role, builtin or custom.

ALTER TABLE roles ADD COLUMN is_system BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE roles SET is_system = TRUE
  WHERE name IN ('OPERATOR', 'QC_SUPERVISOR', 'WAREHOUSE_STAFF', 'MANAGER', 'ADMIN');

CREATE TABLE IF NOT EXISTS role_permissions (
  role_id  VARCHAR(36) NOT NULL,
  rpc_path VARCHAR(255) NOT NULL,
  PRIMARY KEY (role_id, rpc_path),
  FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);

-- Seed grants to exactly reproduce the previous hardcoded rpcRoles map.
-- ADMIN is intentionally omitted: it bypasses RBAC in code, so it needs no rows.
INSERT IGNORE INTO role_permissions (role_id, rpc_path)
SELECT r.id, p.rpc_path
FROM roles r
JOIN (
  SELECT 'OPERATOR' AS name, '/simaops.lot.v1.LotService/CreateLot' AS rpc_path
  UNION ALL SELECT 'OPERATOR', '/simaops.lot.v1.LotService/UpdateLotStatus'
  UNION ALL SELECT 'OPERATOR', '/simaops.qc.v1.QCService/CreateQCUploadUrl'
  UNION ALL SELECT 'OPERATOR', '/simaops.qc.v1.QCService/CreateQCJob'
  UNION ALL SELECT 'QC_SUPERVISOR', '/simaops.qc.v1.QCService/ReviewQC'
  UNION ALL SELECT 'QC_SUPERVISOR', '/simaops.qc.v1.QCService/RetryQCJob'
  UNION ALL SELECT 'WAREHOUSE_STAFF', '/simaops.warehouse.v1.WarehouseService/RecommendSlot'
  UNION ALL SELECT 'WAREHOUSE_STAFF', '/simaops.warehouse.v1.WarehouseService/AssignSlot'
  UNION ALL SELECT 'WAREHOUSE_STAFF', '/simaops.dispatch.v1.DispatchService/CreateDispatch'
  UNION ALL SELECT 'WAREHOUSE_STAFF', '/simaops.dispatch.v1.DispatchService/UpdateDispatchStatus'
  UNION ALL SELECT 'MANAGER', '/simaops.dispatch.v1.DispatchService/CreateDispatch'
  UNION ALL SELECT 'MANAGER', '/simaops.dispatch.v1.DispatchService/UpdateDispatchStatus'
  UNION ALL SELECT 'MANAGER', '/simaops.audit.v1.AuditService/ListAuditLogs'
  UNION ALL SELECT 'MANAGER', '/simaops.audit.v1.AuditService/GetEntityAuditTrail'
) p ON r.name = p.name;
