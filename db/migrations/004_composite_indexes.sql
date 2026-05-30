-- 004_composite_indexes.sql
--
-- Index tuning based on EXPLAIN against live query patterns (apps/api/internal/db/queries/*.sql).
-- Two goals:
--   1. Add a covering index for the dashboard QC-metrics queries, which were
--      doing a full table scan of qc_results on every dashboard load.
--   2. Add (filter_col, created_at) composites so the status/material/lot list
--      queries satisfy `WHERE x=? ORDER BY created_at` from the index range
--      scan alone, eliminating the in-memory TopN sort.
-- Then drop the single-column indexes that become redundant leftmost prefixes
-- of the new composites (only non-FK ones — FK-backing lot_id indexes and the
-- bare created_at indexes used by unfiltered ORDER BY are kept).
--
-- All ADD/DROP use IF [NOT] EXISTS so the migration is idempotent. ADD INDEX in
-- TiDB is an online operation (no table lock, no row rewrite) — safe to run on
-- a live cluster.

-- ── qc_results: fixes TableFullScan on GetQCMetrics (CountQCByRecommendation +
--    AvgQCConfidence both filter created_at >= ?). Covering: created_at range
--    + recommendation (GROUP BY) + confidence (AVG) all served from the index.
ALTER TABLE qc_results ADD INDEX IF NOT EXISTS idx_qc_results_created_rec_conf (created_at, recommendation, confidence);

-- ── lots: ListLotsByStatus / ListLotsByMaterialType both ORDER BY created_at.
ALTER TABLE lots ADD INDEX IF NOT EXISTS idx_lots_status_created (status, created_at);
ALTER TABLE lots ADD INDEX IF NOT EXISTS idx_lots_material_created (material_type, created_at);
ALTER TABLE lots DROP INDEX IF EXISTS idx_lots_status;
ALTER TABLE lots DROP INDEX IF EXISTS idx_lots_material_type;

-- ── qc_jobs: ListQCJobsByLot / ListQCJobsByStatus both ORDER BY created_at.
ALTER TABLE qc_jobs ADD INDEX IF NOT EXISTS idx_qc_jobs_lot_created (lot_id, created_at);
ALTER TABLE qc_jobs ADD INDEX IF NOT EXISTS idx_qc_jobs_status_created (status, created_at);
ALTER TABLE qc_jobs DROP INDEX IF EXISTS idx_qc_jobs_status;
-- idx_qc_jobs_lot_id kept (backs the lot_id foreign key).

-- ── dispatches: ListDispatchesByLot / ListDispatchesByStatus ORDER BY created_at.
--    (lot_id, created_at) also serves CountActiveDispatchesForLot (few rows/lot).
ALTER TABLE dispatches ADD INDEX IF NOT EXISTS idx_dispatch_lot_created (lot_id, created_at);
ALTER TABLE dispatches ADD INDEX IF NOT EXISTS idx_dispatch_status_created (status, created_at);
ALTER TABLE dispatches DROP INDEX IF EXISTS idx_dispatch_status;
-- idx_dispatch_lot_id kept (backs the lot_id foreign key).

-- ── audit_logs: GetEntityAuditTrail (entity_type,entity_id ORDER BY created_at)
--    and ListAuditLogsByActor (actor_user_id ORDER BY created_at DESC).
ALTER TABLE audit_logs ADD INDEX IF NOT EXISTS idx_audit_entity_created (entity_type, entity_id, created_at);
ALTER TABLE audit_logs ADD INDEX IF NOT EXISTS idx_audit_actor_created (actor_user_id, created_at);
ALTER TABLE audit_logs DROP INDEX IF EXISTS idx_audit_entity;
ALTER TABLE audit_logs DROP INDEX IF EXISTS idx_audit_actor;

-- ── warehouse_assignments: GetWarehouseAssignmentByLot filters lot_id + status.
ALTER TABLE warehouse_assignments ADD INDEX IF NOT EXISTS idx_wa_lot_status (lot_id, status);
-- idx_wa_lot_id kept (backs the lot_id foreign key).
