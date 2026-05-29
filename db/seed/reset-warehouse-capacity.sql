-- db/seed/reset-warehouse-capacity.sql
--
-- LOAD-TEST RESET: bumps every warehouse_locations.capacity to a
-- deliberately huge value so load runs can issue tens of thousands of
-- AssignSlot calls without exhausting the (production-realistic) seed
-- capacity of 92 slots.
--
-- This is idempotent — run it before every load test.
--
-- DO NOT run this against any environment with real production data.
-- It is intended exclusively for the load-test cluster. The CALLER
-- (load-tests/scripts/preflight.sh) is responsible for asserting that
-- the table only contains the 12 seeded location codes before invoking
-- this script. We keep this file pure DML because TiDB does not support
-- stored procedures (DROP PROCEDURE / SIGNAL — error 8108).
--
-- Triggered automatically by `load-tests/scripts/preflight.sh` after it
-- verifies the safety assertion.

-- 1. Restore capacity. Set high (1,000,000) so even a 2-hour soak run at
--    1 ops/sec (~7,200 iters) or a 22-min validation run at ~9 ops/sec
--    (~12,000 iters) cannot drain it.
UPDATE warehouse_locations
   SET capacity = 1000000
 WHERE code IN (
   'A-01','A-02','A-03','A-04',
   'B-01','B-02','B-03','B-04',
   'C-01','C-02','C-03','C-04'
 );

-- 2. Mark every location AVAILABLE so RecommendSlot will surface it.
UPDATE warehouse_locations
   SET current_status = 'AVAILABLE'
 WHERE code IN (
   'A-01','A-02','A-03','A-04',
   'B-01','B-02','B-03','B-04',
   'C-01','C-02','C-03','C-04'
 );

-- 3. Echo the result so the preflight log captures it.
SELECT code, zone, capacity, current_status
  FROM warehouse_locations
 ORDER BY zone, code;
