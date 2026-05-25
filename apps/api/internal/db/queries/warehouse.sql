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

-- name: CreateWarehouseAssignment :exec
INSERT INTO warehouse_assignments (id, lot_id, location_id, assigned_by)
VALUES (?, ?, ?, ?);

-- name: GetWarehouseAssignmentByLot :one
SELECT * FROM warehouse_assignments WHERE lot_id = ? AND status = 'ACTIVE';

-- name: ListWarehouseAssignments :many
SELECT * FROM warehouse_assignments ORDER BY assigned_at DESC LIMIT ? OFFSET ?;

-- name: ZoneCapacityMetrics :many
SELECT zone, SUM(capacity) as total_capacity,
       SUM(CASE WHEN current_status = 'OCCUPIED' THEN 1 ELSE 0 END) as occupied,
       SUM(CASE WHEN current_status = 'AVAILABLE' THEN capacity ELSE 0 END) as available
FROM warehouse_locations GROUP BY zone;
