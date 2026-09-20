-- name: GetLoginUser :one
SELECT u.*, r.name AS role FROM users u JOIN roles r ON r.id=u.role_id WHERE u.email=$1;

-- name: GetCurrentUser :one
SELECT u.*, r.name AS role FROM users u JOIN roles r ON r.id=u.role_id WHERE u.id=$1;

-- name: LockUserAdministration :exec
SELECT pg_advisory_xact_lock(716493001);

-- name: HasAdministrator :one
SELECT EXISTS(SELECT 1 FROM users u JOIN roles r ON r.id=u.role_id WHERE r.name='ADMIN');

-- name: InsertUser :one
INSERT INTO users (name,email,password_hash,role_id)
VALUES ($1,$2,$3,(SELECT id FROM roles WHERE name=sqlc.arg(role)::text)) RETURNING *;

-- name: UserHasPermission :one
SELECT EXISTS(SELECT 1 FROM users u JOIN role_permissions rp ON rp.role_id=u.role_id JOIN permissions p ON p.id=rp.permission_id WHERE u.id=$1 AND u.is_active AND p.code=$2);

-- name: LockUser :one
SELECT * FROM users WHERE id=$1 FOR UPDATE;

-- name: CountActiveAdmins :one
SELECT count(*) FROM users u JOIN roles r ON r.id=u.role_id WHERE u.is_active AND r.name='ADMIN';

-- name: UserHasActiveOrders :one
SELECT EXISTS(SELECT 1 FROM work_orders WHERE assigned_to=$1 AND status IN ('ASSIGNED','IN_PROGRESS'));

-- name: UpdateUser :one
UPDATE users SET name=$2,email=$3,password_hash=$4,is_active=$5,updated_at=clock_timestamp() WHERE id=$1 RETURNING *;

-- name: ListUsers :many
SELECT u.*,r.name AS role FROM users u JOIN roles r ON r.id=u.role_id
WHERE (sqlc.arg(role_filter)::text='' OR r.name=sqlc.arg(role_filter)::text)
AND (sqlc.narg(active_filter)::boolean IS NULL OR u.is_active=sqlc.narg(active_filter)::boolean)
ORDER BY u.created_at DESC,u.id DESC LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: CountUsers :one
SELECT count(*) FROM users u JOIN roles r ON r.id=u.role_id
WHERE (sqlc.arg(role_filter)::text='' OR r.name=sqlc.arg(role_filter)::text)
AND (sqlc.narg(active_filter)::boolean IS NULL OR u.is_active=sqlc.narg(active_filter)::boolean);
