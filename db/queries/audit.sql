-- name: InsertAudit :exec
INSERT INTO audit_logs (actor_id,action,entity_type,entity_id,old_value,new_value,request_id)
VALUES ($1,$2,$3,$4,$5,$6,$7);

-- name: InsertUserUpdateAudit :exec
INSERT INTO audit_logs(actor_id,action,entity_type,entity_id,old_value,new_value,request_id)
VALUES($1,'USER_UPDATED','user',$2,$3,$4,$5);

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE (sqlc.narg(actor_filter)::uuid IS NULL OR actor_id=sqlc.narg(actor_filter)::uuid)
AND (sqlc.arg(action_filter)::text='' OR action=sqlc.arg(action_filter)::text)
AND (sqlc.arg(type_filter)::text='' OR entity_type=sqlc.arg(type_filter)::text)
AND (sqlc.narg(entity_filter)::uuid IS NULL OR entity_id=sqlc.narg(entity_filter)::uuid)
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: CountAuditLogs :one
SELECT count(*) FROM audit_logs
WHERE (sqlc.narg(actor_filter)::uuid IS NULL OR actor_id=sqlc.narg(actor_filter)::uuid)
AND (sqlc.arg(action_filter)::text='' OR action=sqlc.arg(action_filter)::text)
AND (sqlc.arg(type_filter)::text='' OR entity_type=sqlc.arg(type_filter)::text)
AND (sqlc.narg(entity_filter)::uuid IS NULL OR entity_id=sqlc.narg(entity_filter)::uuid);
