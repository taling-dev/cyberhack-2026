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

-- name: GetUserByEmail :one
SELECT * FROM users_profile WHERE email = ?;

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

-- name: ListAllRolePermissions :many
SELECT r.name AS role_name, rp.rpc_path
FROM role_permissions rp JOIN roles r ON r.id = rp.role_id;

-- name: CreateRole :exec
INSERT INTO roles (id, name, description, is_system) VALUES (?, ?, ?, FALSE);

-- name: AddRolePermission :exec
INSERT IGNORE INTO role_permissions (role_id, rpc_path) VALUES (?, ?);

-- name: DeleteRole :exec
DELETE FROM roles WHERE id = ? AND is_system = FALSE;

-- name: GetRoleByID :one
SELECT * FROM roles WHERE id = ?;

-- name: CountRoleMembers :one
SELECT COUNT(*) FROM user_roles WHERE role_id = ?;

-- name: CreateUserProfile :exec
INSERT INTO users_profile (id, username, email, full_name, active) VALUES (?, ?, ?, ?, TRUE);

-- name: UpdateUserProfile :exec
UPDATE users_profile SET full_name = ?, email = ?, active = ? WHERE id = ?;

-- name: UpdateRoleDescription :exec
UPDATE roles SET description = ? WHERE id = ? AND is_system = FALSE;

-- name: ClearRolePermissions :exec
DELETE FROM role_permissions WHERE role_id = ?;
