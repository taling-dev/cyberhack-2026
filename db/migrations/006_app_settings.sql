-- SARI: configurable app settings (key/value).
-- Holds the AI QC grading thresholds so admins can tune PASS/REVIEW cutoffs
-- without a redeploy. Both the API (admin RPC) and the AI worker read these.

CREATE TABLE IF NOT EXISTS app_settings (
  setting_key   VARCHAR(64) PRIMARY KEY,
  setting_value VARCHAR(255) NOT NULL,
  updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Defaults match the worker's previous hardcoded cutoffs: quality >=75 PASS,
-- >=40 REVIEW, else FAIL (0-100 scale).
INSERT IGNORE INTO app_settings (setting_key, setting_value) VALUES
  ('qc_pass_min', '75'),
  ('qc_review_min', '40');
