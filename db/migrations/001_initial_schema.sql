-- SARI (Sima Arome Resource Intelligence): Initial Schema
-- Target: TiDB v7.5 (MySQL-compatible)

CREATE TABLE IF NOT EXISTS roles (
  id          VARCHAR(36) PRIMARY KEY,
  name        VARCHAR(64) NOT NULL UNIQUE,
  description VARCHAR(255) NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS users_profile (
  id         VARCHAR(36) PRIMARY KEY,
  username   VARCHAR(128) NOT NULL UNIQUE,
  email      VARCHAR(255) NOT NULL UNIQUE,
  full_name  VARCHAR(255) NOT NULL DEFAULT '',
  active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_roles (
  user_id VARCHAR(36) NOT NULL,
  role_id VARCHAR(36) NOT NULL,
  PRIMARY KEY (user_id, role_id),
  FOREIGN KEY (user_id) REFERENCES users_profile(id) ON DELETE CASCADE,
  FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS lots (
  id                  VARCHAR(36) PRIMARY KEY,
  lot_number          VARCHAR(64) NOT NULL UNIQUE,
  supplier_name       VARCHAR(255) NOT NULL,
  material_name       VARCHAR(255) NOT NULL,
  material_type       ENUM('RAW_BOTANICAL','EXTRACT','POWDER','OTHER') NOT NULL,
  quantity            DECIMAL(14,3) NOT NULL,
  unit                VARCHAR(16) NOT NULL,
  arrival_date        DATE NOT NULL,
  storage_requirement JSON NOT NULL,
  status              ENUM('DRAFT','PENDING_QC','AI_PROCESSING','QC_REVIEW',
                           'QC_APPROVED','QC_REJECTED','WAREHOUSE_ASSIGNED',
                           'READY_FOR_PRODUCTION','BLOCKED') NOT NULL DEFAULT 'DRAFT',
  created_by          VARCHAR(64) NOT NULL,
  created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_lots_status (status),
  INDEX idx_lots_material_type (material_type),
  INDEX idx_lots_created_at (created_at)
);

CREATE TABLE IF NOT EXISTS qc_jobs (
  id               VARCHAR(36) PRIMARY KEY,
  lot_id           VARCHAR(36) NOT NULL,
  image_object_key VARCHAR(512) NOT NULL,
  status           ENUM('QUEUED','PROCESSING','AI_COMPLETED','NEEDS_HUMAN_REVIEW',
                        'APPROVED','REJECTED','FAILED') NOT NULL DEFAULT 'QUEUED',
  requested_by     VARCHAR(64) NOT NULL,
  failure_reason   TEXT,
  started_at       TIMESTAMP NULL,
  completed_at     TIMESTAMP NULL,
  created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (lot_id) REFERENCES lots(id) ON DELETE RESTRICT,
  INDEX idx_qc_jobs_lot_id (lot_id),
  INDEX idx_qc_jobs_status (status)
);

CREATE TABLE IF NOT EXISTS qc_results (
  id                  VARCHAR(36) PRIMARY KEY,
  qc_job_id           VARCHAR(36) NOT NULL UNIQUE,
  lot_id              VARCHAR(36) NOT NULL,
  recommendation      ENUM('PASS','REVIEW','FAIL') NOT NULL,
  confidence          DECIMAL(5,4) NOT NULL,
  findings_json       JSON NOT NULL,
  model_version       VARCHAR(128) NOT NULL DEFAULT '',
  annotated_image_key VARCHAR(512) DEFAULT NULL,
  supervisor_decision ENUM('APPROVED','REJECTED','RECHECK') DEFAULT NULL,
  reviewed_by         VARCHAR(64) DEFAULT NULL,
  review_reason       TEXT DEFAULT NULL,
  reviewed_at         TIMESTAMP NULL,
  created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (qc_job_id) REFERENCES qc_jobs(id) ON DELETE CASCADE,
  FOREIGN KEY (lot_id) REFERENCES lots(id) ON DELETE RESTRICT,
  INDEX idx_qc_results_lot_id (lot_id)
);

CREATE TABLE IF NOT EXISTS warehouse_locations (
  id                 VARCHAR(36) PRIMARY KEY,
  code               VARCHAR(32) NOT NULL UNIQUE,
  zone               VARCHAR(32) NOT NULL,
  temperature_min    DECIMAL(5,2) NOT NULL,
  temperature_max    DECIMAL(5,2) NOT NULL,
  hazard_allowed     JSON NOT NULL,
  drum_compatibility JSON NOT NULL,
  capacity           INT NOT NULL DEFAULT 0,
  current_status     ENUM('AVAILABLE','OCCUPIED','MAINTENANCE') NOT NULL DEFAULT 'AVAILABLE',
  INDEX idx_wh_zone (zone),
  INDEX idx_wh_status (current_status)
);

CREATE TABLE IF NOT EXISTS warehouse_assignments (
  id          VARCHAR(36) PRIMARY KEY,
  lot_id      VARCHAR(36) NOT NULL,
  location_id VARCHAR(36) NOT NULL,
  assigned_by VARCHAR(64) NOT NULL,
  assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  status      ENUM('ACTIVE','RELEASED') NOT NULL DEFAULT 'ACTIVE',
  FOREIGN KEY (lot_id) REFERENCES lots(id) ON DELETE RESTRICT,
  FOREIGN KEY (location_id) REFERENCES warehouse_locations(id) ON DELETE RESTRICT,
  INDEX idx_wa_lot_id (lot_id),
  INDEX idx_wa_location_id (location_id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id            VARCHAR(36) PRIMARY KEY,
  actor_user_id VARCHAR(64) NOT NULL,
  actor_role    VARCHAR(64) NOT NULL,
  action        VARCHAR(64) NOT NULL,
  entity_type   VARCHAR(64) NOT NULL,
  entity_id     VARCHAR(64) NOT NULL,
  before_json   JSON,
  after_json    JSON,
  request_id    VARCHAR(64),
  trace_id      VARCHAR(128),
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_audit_entity (entity_type, entity_id),
  INDEX idx_audit_actor (actor_user_id),
  INDEX idx_audit_created_at (created_at)
);

CREATE TABLE IF NOT EXISTS outbox_events (
  id           VARCHAR(36) PRIMARY KEY,
  event_type   VARCHAR(64) NOT NULL,
  payload_json JSON NOT NULL,
  status       ENUM('PENDING','PUBLISHED','FAILED') NOT NULL DEFAULT 'PENDING',
  retry_count  INT NOT NULL DEFAULT 0,
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  published_at TIMESTAMP NULL,
  INDEX idx_outbox_status_created (status, created_at)
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
  key_hash      VARCHAR(128) PRIMARY KEY,
  user_id       VARCHAR(64) NOT NULL,
  operation     VARCHAR(64) NOT NULL,
  response_json JSON,
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_idem_created_at (created_at)
);
