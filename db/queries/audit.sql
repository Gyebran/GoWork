-- name: InsertAudit :exec
INSERT INTO audit_logs (actor_id,action,entity_type,entity_id,old_value,new_value,request_id)
VALUES ($1,$2,$3,$4,$5,$6,$7);
