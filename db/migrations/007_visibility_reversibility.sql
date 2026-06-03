-- Migration: 007_visibility_reversibility.sql
-- Purpose: Add visibility and reversibility for automatic decisions
--   - slot_decisions: audit trail for slot assignments
--   - review_requests: allow operators/warehouse staff to request supervisor review
--   - decision_type column on warehouse_assignments to track AUTO/MANUAL/OVERRIDE

-- ============================================================
-- 1. slot_decisions: audit trail for all slot assignments
-- ============================================================
CREATE TABLE IF NOT EXISTS slot_decisions (
  id            VARCHAR(36) PRIMARY KEY,
  lot_id        VARCHAR(36) NOT NULL,
  location_id   VARCHAR(36) NOT NULL,
  decision_type ENUM('AUTO','MANUAL','OVERRIDE') NOT NULL,
  reason        TEXT,
  actor_id      VARCHAR(64) NOT NULL,
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (lot_id) REFERENCES lots(id) ON DELETE RESTRICT,
  FOREIGN KEY (location_id) REFERENCES warehouse_locations(id) ON DELETE RESTRICT
);

CREATE INDEX idx_slot_decisions_lot_id ON slot_decisions(lot_id);
CREATE INDEX idx_slot_decisions_created_at ON slot_decisions(created_at);

-- ============================================================
-- 2. review_requests: allow operators/warehouse staff to
--    request supervisor review of warehouse/slot decisions
-- ============================================================
CREATE TABLE IF NOT EXISTS review_requests (
  id             VARCHAR(36) PRIMARY KEY,
  lot_id         VARCHAR(36) NOT NULL,
  requester_id   VARCHAR(64) NOT NULL,
  requester_role VARCHAR(32) NOT NULL,
  target_role    VARCHAR(32) NOT NULL,  -- 'QC_SUPERVISOR', 'MANAGER', etc.
  request_type   ENUM('WAREHOUSE_REASSIGN','STATUS_CHANGE') NOT NULL,
  reason         TEXT NOT NULL,
  status         ENUM('PENDING','APPROVED','REJECTED','CANCELLED') NOT NULL DEFAULT 'PENDING',
  reviewed_by    VARCHAR(64) DEFAULT NULL,
  review_note    TEXT DEFAULT NULL,
  created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  reviewed_at    TIMESTAMP NULL,
  FOREIGN KEY (lot_id) REFERENCES lots(id) ON DELETE RESTRICT
);

CREATE INDEX idx_review_requests_status ON review_requests(status);
CREATE INDEX idx_review_requests_target_role ON review_requests(target_role);
CREATE INDEX idx_review_requests_lot_id ON review_requests(lot_id);

-- ============================================================
-- 3. decision_type column on warehouse_assignments
--    Tracks how the slot was assigned: AUTO (AI), MANUAL (human), OVERRIDE (human override)
-- ============================================================
ALTER TABLE warehouse_assignments
  ADD COLUMN decision_type ENUM('AUTO','MANUAL','OVERRIDE') NOT NULL DEFAULT 'MANUAL'
  AFTER status;

-- Migrate existing assignments as MANUAL (they were human-initiated)
UPDATE warehouse_assignments SET decision_type = 'MANUAL' WHERE decision_type IS NULL;