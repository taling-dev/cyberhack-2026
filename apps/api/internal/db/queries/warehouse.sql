-- name: ListWarehouseLocations :many
SELECT * FROM warehouse_locations ORDER BY zone, code;

-- name: ListWarehouseLocationsByZone :many
SELECT * FROM warehouse_locations WHERE zone = ? ORDER BY code;

-- name: ListAvailableLocations :many
SELECT * FROM warehouse_locations WHERE current_status = 'AVAILABLE' AND capacity > 0 ORDER BY zone, code;

-- name: GetWarehouseLocation :one
SELECT * FROM warehouse_locations WHERE id = ?;

-- name: UpdateLocationStatus :exec
UPDATE warehouse_locations SET current_status = ? WHERE id = ?;

-- name: DecrementLocationCapacity :exec
UPDATE warehouse_locations SET capacity = capacity - 1 WHERE id = ? AND capacity > 0;

-- name: DecrementLocationCapacityAtomic :execrows
UPDATE warehouse_locations SET capacity = capacity - 1 WHERE id = ? AND capacity > 0;

-- name: IncrementLocationCapacity :execrows
UPDATE warehouse_locations SET capacity = capacity + 1 WHERE id = ?;

-- name: CreateWarehouseAssignment :exec
INSERT INTO warehouse_assignments (id, lot_id, location_id, assigned_by)
VALUES (?, ?, ?, ?);

-- name: GetWarehouseAssignmentByLot :one
SELECT * FROM warehouse_assignments WHERE lot_id = ? AND status = 'ACTIVE';

-- name: ListWarehouseAssignments :many
SELECT * FROM warehouse_assignments ORDER BY assigned_at DESC LIMIT ? OFFSET ?;

-- name: ZoneCapacityMetrics :many
-- `capacity` is REMAINING slots (AssignSlot decrements it), so it equals
-- "available". Occupancy is the count of ACTIVE assignments for the zone's
-- locations; total = remaining + occupied, which stays stable as lots are
-- assigned (available shrinks, occupied grows). Ordered alphabetically by zone.
SELECT l.zone,
       SUM(l.capacity) + COALESCE(SUM(ac.cnt), 0) AS total_capacity,
       COALESCE(SUM(ac.cnt), 0) AS occupied,
       SUM(l.capacity) AS available
FROM warehouse_locations l
LEFT JOIN (
  SELECT location_id, COUNT(*) AS cnt
  FROM warehouse_assignments WHERE status = 'ACTIVE' GROUP BY location_id
) ac ON ac.location_id = l.id
GROUP BY l.zone ORDER BY l.zone;
