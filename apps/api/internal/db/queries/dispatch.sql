-- name: CreateDispatch :exec
INSERT INTO dispatches (id, dispatch_number, lot_id, destination, carrier, quantity, unit, scheduled_at, notes, status, created_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING', ?);

-- name: GetDispatch :one
SELECT * FROM dispatches WHERE id = ?;

-- name: GetDispatchForUpdate :one
-- Locks the row for the duration of the transaction (TiDB pessimistic mode).
-- Used by UpdateDispatchStatus to close the same TOCTOU race the lot FSM
-- guards against: without FOR UPDATE two concurrent status changes could both
-- pass the FSM check against the same source state.
SELECT * FROM dispatches WHERE id = ? FOR UPDATE;

-- name: CountActiveDispatchesForLot :one
-- A lot may only have one non-cancelled dispatch at a time. Used to reject a
-- duplicate CreateDispatch for a lot already being shipped.
SELECT COUNT(*) FROM dispatches WHERE lot_id = ? AND status != 'CANCELLED';

-- name: ListDispatches :many
SELECT * FROM dispatches ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: ListDispatchesByStatus :many
SELECT * FROM dispatches WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: ListDispatchesByLot :many
SELECT * FROM dispatches WHERE lot_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: CountDispatches :one
SELECT COUNT(*) FROM dispatches;

-- name: CountDispatchesByStatus :one
SELECT COUNT(*) FROM dispatches WHERE status = ?;

-- name: UpdateDispatchStatus :exec
UPDATE dispatches SET status = ? WHERE id = ?;
