-- name: ListUsers :many
SELECT u.*, GROUP_CONCAT(r.name SEPARATOR ',') AS role_names
FROM users_profile u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
GROUP BY u.id, u.username, u.email, u.full_name, u.active, u.created_at, u.updated_at
ORDER BY u.username
LIMIT ? OFFSET ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users_profile;

-- name: GetUserByUsername :one
SELECT * FROM users_profile WHERE username = ?;

-- name: GetUserByID :one
SELECT * FROM users_profile WHERE id = ?;

-- name: ListRoles :many
SELECT * FROM roles ORDER BY name;

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = ?;

-- name: AssignUserRole :exec
INSERT IGNORE INTO user_roles (user_id, role_id) VALUES (?, ?);

-- name: RevokeUserRole :exec
DELETE FROM user_roles WHERE user_id = ? AND role_id = ?;

-- name: ListUserRoleNames :many
SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = ? ORDER BY r.name;
