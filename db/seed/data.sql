-- SimaOps AI: Seed Data
-- Run after migrations: mysql -h 127.0.0.1 -P 4000 -u root simaops < db/seed/data.sql

-- ─── Roles ───────────────────────────────────────────────────────
INSERT IGNORE INTO roles (id, name, description) VALUES
  ('r-operator',       'OPERATOR',        'Lot intake and QC image upload'),
  ('r-qc-supervisor',  'QC_SUPERVISOR',   'Approve/reject QC results'),
  ('r-warehouse',      'WAREHOUSE_STAFF', 'Assign warehouse slots'),
  ('r-manager',        'MANAGER',         'View dashboards and audit logs'),
  ('r-admin',          'ADMIN',           'Full system access');

-- ─── Demo Users (UUIDs match Keycloak sub claims after first login) ──
INSERT IGNORE INTO users_profile (id, username, email, full_name) VALUES
  ('u-operator',      'operator',       'operator@simaops.local',  'Budi Operator'),
  ('u-qc-supervisor', 'qc_supervisor',  'qc@simaops.local',       'Siti QC Supervisor'),
  ('u-warehouse',     'warehouse',      'warehouse@simaops.local', 'Agus Warehouse'),
  ('u-manager',       'manager',        'manager@simaops.local',   'Dewi Manager'),
  ('u-admin',         'admin',          'admin@simaops.local',     'Andi Admin');

INSERT IGNORE INTO user_roles (user_id, role_id) VALUES
  ('u-operator',      'r-operator'),
  ('u-qc-supervisor', 'r-qc-supervisor'),
  ('u-warehouse',     'r-warehouse'),
  ('u-manager',       'r-manager'),
  ('u-admin',         'r-admin');

-- ─── Warehouse Locations (12 slots, 3 zones) ────────────────────
-- Zone A: Ambient (15–25 °C), no hazard, IBC+IPPC drums OK
INSERT IGNORE INTO warehouse_locations (id, code, zone, temperature_min, temperature_max, hazard_allowed, drum_compatibility, capacity) VALUES
  ('wh-a01', 'A-01', 'Zone A - Ambient',  15.00, 25.00, '[]', '["IBC","IPPC"]', 10),
  ('wh-a02', 'A-02', 'Zone A - Ambient',  15.00, 25.00, '[]', '["IBC","IPPC"]', 10),
  ('wh-a03', 'A-03', 'Zone A - Ambient',  15.00, 25.00, '[]', '["IBC","IPPC"]', 10),
  ('wh-a04', 'A-04', 'Zone A - Ambient',  15.00, 25.00, '[]', '["IBC","IPPC"]', 10);

-- Zone B: Cold (2–8 °C), IBC+IPPC drums, no hazard
INSERT IGNORE INTO warehouse_locations (id, code, zone, temperature_min, temperature_max, hazard_allowed, drum_compatibility, capacity) VALUES
  ('wh-b01', 'B-01', 'Zone B - Cold',  2.00, 8.00, '[]', '["IBC","IPPC"]', 8),
  ('wh-b02', 'B-02', 'Zone B - Cold',  2.00, 8.00, '[]', '["IBC","IPPC"]', 8),
  ('wh-b03', 'B-03', 'Zone B - Cold',  2.00, 8.00, '[]', '["IBC","IPPC"]', 8),
  ('wh-b04', 'B-04', 'Zone B - Cold',  2.00, 8.00, '[]', '["IBC","IPPC"]', 8);

-- Zone C: Deep-freeze (−20 to −4 °C), IBC only, hazard IBC
INSERT IGNORE INTO warehouse_locations (id, code, zone, temperature_min, temperature_max, hazard_allowed, drum_compatibility, capacity) VALUES
  ('wh-c01', 'C-01', 'Zone C - Deep Freeze', -20.00, -4.00, '["IBC"]', '["IBC"]', 5),
  ('wh-c02', 'C-02', 'Zone C - Deep Freeze', -20.00, -4.00, '["IBC"]', '["IBC"]', 5),
  ('wh-c03', 'C-03', 'Zone C - Deep Freeze', -20.00, -4.00, '["IBC"]', '["IBC"]', 5),
  ('wh-c04', 'C-04', 'Zone C - Deep Freeze', -20.00, -4.00, '["IBC"]', '["IBC"]', 5);
