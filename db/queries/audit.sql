-- name: InsertAudit :exec
INSERT INTO audit_logs (actor_id,action,entity_type,entity_id,old_value,new_value,request_id)
VALUES ($1,$2,$3,$4,$5,$6,$7);

-- name: InsertUserUpdateAudit :exec
INSERT INTO audit_logs(actor_id,action,entity_type,entity_id,old_value,new_value,request_id)
VALUES($1,'USER_UPDATED','user',$2,$3,$4,$5);
