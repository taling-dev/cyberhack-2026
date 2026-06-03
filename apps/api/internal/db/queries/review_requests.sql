-- name: CreateReviewRequest :exec
INSERT INTO review_requests (id, lot_id, requester_id, requester_role, target_role, request_type, reason)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListReviewRequests :many
SELECT * FROM review_requests ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: ListReviewRequestsByStatus :many
SELECT * FROM review_requests WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: ListReviewRequestsByTargetRole :many
SELECT * FROM review_requests WHERE target_role = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: GetReviewRequest :one
SELECT * FROM review_requests WHERE id = ?;

-- name: UpdateReviewRequestStatus :exec
UPDATE review_requests SET status = ?, reviewed_by = ?, review_note = ?, reviewed_at = NOW() WHERE id = ?;