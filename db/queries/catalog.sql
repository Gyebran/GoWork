-- name: ListRoles :many
SELECT id, name FROM roles ORDER BY id;

-- name: ListPermissions :many
SELECT id, code, description FROM permissions ORDER BY id;

-- name: ListRolePermissions :many
SELECT role_id, permission_id FROM role_permissions ORDER BY role_id, permission_id;
