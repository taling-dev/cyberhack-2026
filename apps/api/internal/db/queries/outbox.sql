-- name: CreateOutboxEvent :exec
INSERT INTO outbox_events (id, event_type, payload_json, status)
VALUES (?, ?, ?, 'PENDING');

-- name: ListPendingOutboxEvents :many
SELECT * FROM outbox_events WHERE status = 'PENDING' ORDER BY created_at ASC LIMIT ?;

-- name: MarkOutboxPublished :exec
UPDATE outbox_events SET status = 'PUBLISHED', published_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: MarkOutboxFailed :exec
UPDATE outbox_events SET status = 'FAILED', retry_count = retry_count + 1 WHERE id = ?;

-- name: IncrementOutboxRetry :exec
UPDATE outbox_events SET retry_count = retry_count + 1 WHERE id = ?;

-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys WHERE key_hash = ?;

-- name: CreateIdempotencyKey :exec
INSERT INTO idempotency_keys (key_hash, user_id, operation, response_json)
VALUES (?, ?, ?, ?);

-- name: UpdateIdempotencyResponse :exec
UPDATE idempotency_keys SET response_json = ? WHERE key_hash = ?;

-- name: DeleteExpiredIdempotencyKeys :exec
DELETE FROM idempotency_keys WHERE created_at < DATE_SUB(NOW(), INTERVAL 24 HOUR);
