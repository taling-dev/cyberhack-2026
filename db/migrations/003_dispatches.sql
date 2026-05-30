-- 003_dispatches.sql
--
-- Adds the `dispatches` table — the final stage of the Integrated Operations
-- System (hackathon Focus Area 1). A dispatch tracks a production-ready lot
-- leaving the facility. Each dispatch references the lot it ships, carries its
-- own lifecycle FSM (PENDING → SCHEDULED → IN_TRANSIT → DELIVERED, with
-- CANCELLED as a side-rail), and is created only from a lot that has reached
-- READY_FOR_PRODUCTION. This closes the loop intake → QC → warehouse →
-- production handoff → dispatch in one auditable source of truth.

CREATE TABLE IF NOT EXISTS dispatches (
  id              VARCHAR(36) PRIMARY KEY,
  dispatch_number VARCHAR(64) NOT NULL UNIQUE,            -- DSP-YYYY-MM-DD-XXXXXXXX
  lot_id          VARCHAR(36) NOT NULL,
  destination     VARCHAR(255) NOT NULL,
  carrier         VARCHAR(255) NOT NULL DEFAULT '',
  quantity        DECIMAL(14,3) NOT NULL,
  unit            VARCHAR(16) NOT NULL,
  scheduled_at    TIMESTAMP NULL,
  notes           TEXT,
  status          ENUM('PENDING','SCHEDULED','IN_TRANSIT','DELIVERED','CANCELLED')
                    NOT NULL DEFAULT 'PENDING',
  created_by      VARCHAR(64) NOT NULL,
  created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (lot_id) REFERENCES lots(id) ON DELETE RESTRICT,
  INDEX idx_dispatch_lot_id (lot_id),
  INDEX idx_dispatch_status (status),
  INDEX idx_dispatch_created_at (created_at)
);
