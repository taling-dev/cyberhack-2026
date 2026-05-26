-- name: CreateAuditLog :exec
INSERT INTO audit_logs (id, actor_user_id, actor_role, action, entity_type, entity_id, before_json, after_json, request_id, trace_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditLogs :many
SELECT id, actor_user_id, actor_role, action, entity_type, entity_id,
       COALESCE(before_json, JSON_OBJECT()) AS before_json,
       COALESCE(after_json, JSON_OBJECT())  AS after_json,
       request_id, trace_id, created_at
FROM audit_logs ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: ListAuditLogsByEntity :many
SELECT id, actor_user_id, actor_role, action, entity_type, entity_id,
       COALESCE(before_json, JSON_OBJECT()) AS before_json,
       COALESCE(after_json, JSON_OBJECT())  AS after_json,
       request_id, trace_id, created_at
FROM audit_logs WHERE entity_type = ? AND entity_id = ? ORDER BY created_at ASC;

-- name: ListAuditLogsByActor :many
SELECT id, actor_user_id, actor_role, action, entity_type, entity_id,
       COALESCE(before_json, JSON_OBJECT()) AS before_json,
       COALESCE(after_json, JSON_OBJECT())  AS after_json,
       request_id, trace_id, created_at
FROM audit_logs WHERE actor_user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: CountAuditLogs :one
SELECT COUNT(*) FROM audit_logs;
