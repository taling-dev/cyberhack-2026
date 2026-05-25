-- name: CreateLot :exec
INSERT INTO lots (id, lot_number, supplier_name, material_name, material_type, quantity, unit, arrival_date, storage_requirement, status, created_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'DRAFT', ?);

-- name: GetLot :one
SELECT * FROM lots WHERE id = ?;

-- name: GetLotByNumber :one
SELECT * FROM lots WHERE lot_number = ?;

-- name: ListLots :many
SELECT * FROM lots ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: ListLotsByStatus :many
SELECT * FROM lots WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: ListLotsByMaterialType :many
SELECT * FROM lots WHERE material_type = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: CountLots :one
SELECT COUNT(*) FROM lots;

-- name: CountLotsByStatus :one
SELECT COUNT(*) FROM lots WHERE status = ?;

-- name: UpdateLotStatus :exec
UPDATE lots SET status = ? WHERE id = ?;

-- name: CountLotsByStatusGroup :many
SELECT status, COUNT(*) as count FROM lots GROUP BY status;
