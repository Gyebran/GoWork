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
