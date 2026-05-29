-- name: CreateOutboxEvent :exec
INSERT INTO outbox_events (id, event_type, payload_json, status)
VALUES (?, ?, ?, 'PENDING');

-- name: ListPendingOutboxEvents :many
SELECT * FROM outbox_events WHERE status = 'PENDING' ORDER BY created_at ASC LIMIT ?;

-- name: ClaimOutboxBatch :execrows
-- Atomically transitions a batch of PENDING events to PUBLISHING. Combined
-- with `ListClaimedOutboxEvents` immediately after, this gives the leader
-- exclusive ownership of a batch — preventing any racing leader (during
-- election handoff) or this same leader (after a recovered crash) from
-- re-publishing a row that was already taken.
UPDATE outbox_events
SET status = 'PUBLISHING'
WHERE status = 'PENDING'
ORDER BY created_at ASC
LIMIT ?;

-- name: ListClaimedOutboxEvents :many
-- Reads the rows just claimed by ClaimOutboxBatch. Singleton-leader semantics
-- mean we are the only writer; the SELECT cannot see anyone else's claim
-- because no one else is allowed to claim.
SELECT id, event_type, payload_json, retry_count
FROM outbox_events
WHERE status = 'PUBLISHING'
ORDER BY created_at ASC
LIMIT ?;

-- name: MarkOutboxPublished :exec
UPDATE outbox_events
SET status = 'PUBLISHED', published_at = CURRENT_TIMESTAMP
WHERE id = ? AND status = 'PUBLISHING';

-- name: ReleaseClaimedToPending :exec
-- Returns a single claimed event back to PENDING with retry_count incremented.
-- Used when the publish call to NATS fails but the event hasn't yet exhausted
-- its retry budget — the next poll cycle will pick it up again.
UPDATE outbox_events
SET status = 'PENDING', retry_count = retry_count + 1
WHERE id = ? AND status = 'PUBLISHING';

-- name: MarkOutboxFailed :exec
-- Final failure path: retry budget exhausted. Status moves PUBLISHING → FAILED
-- so the row leaves the active queue and only resurfaces if an operator
-- manually resets it (e.g., after a NATS / config fix).
UPDATE outbox_events
SET status = 'FAILED', retry_count = retry_count + 1
WHERE id = ? AND status = 'PUBLISHING';

-- name: ResetStuckPublishingEvents :execrows
-- Called once on publisher startup. Recovers events left in PUBLISHING by a
-- prior leader that crashed mid-publish. They go back to PENDING and the
-- new leader's next poll cycle re-claims and re-publishes them. NATS
-- `Nats-Msg-Id` dedup ensures stream consumers don't see duplicates.
UPDATE outbox_events
SET status = 'PENDING'
WHERE status = 'PUBLISHING';

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
