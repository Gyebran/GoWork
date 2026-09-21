-- name: GetWorkOrder :one
SELECT * FROM work_orders WHERE id=$1;
-- name: LockWorkOrder :one
SELECT * FROM work_orders WHERE id=$1 FOR UPDATE;
-- name: ShareAsset :one
SELECT * FROM assets WHERE id=$1 FOR SHARE;
-- name: ShareUser :one
SELECT * FROM users WHERE id=$1 FOR SHARE;
-- name: InsertWorkOrder :one
INSERT INTO work_orders(work_order_number,asset_id,title,description,priority,created_by)
VALUES ((SELECT 'WO-'||to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY')||'-'||lpad(n::text,greatest(6,length(n::text)),'0') FROM (SELECT nextval('work_order_number_seq') AS n) seq),$1,$2,$3,$4,$5) RETURNING *;
-- name: UpdateWorkOrder :one
UPDATE work_orders SET title=sqlc.arg(title),description=sqlc.arg(description),priority=sqlc.arg(priority),status=sqlc.arg(status),assigned_to=sqlc.narg(assigned_to),
completed_at=CASE WHEN sqlc.arg(status)::text='COMPLETED' THEN clock_timestamp() ELSE NULL END,updated_at=clock_timestamp()
WHERE id=sqlc.arg(id) RETURNING *;
-- name: ListWorkOrders :many
SELECT w.* FROM work_orders w
WHERE (sqlc.arg(status_filter)::text='' OR w.status=sqlc.arg(status_filter)::text)
AND (sqlc.arg(priority_filter)::text='' OR w.priority=sqlc.arg(priority_filter)::text)
AND (sqlc.narg(asset_filter)::uuid IS NULL OR w.asset_id=sqlc.narg(asset_filter)::uuid)
AND (sqlc.narg(assignee_filter)::uuid IS NULL OR w.assigned_to=sqlc.narg(assignee_filter)::uuid)
AND (NOT sqlc.arg(scoped)::boolean OR w.assigned_to=sqlc.arg(actor_id)::uuid)
ORDER BY w.created_at DESC,w.id DESC LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;
-- name: CountWorkOrders :one
SELECT count(*) FROM work_orders w
WHERE (sqlc.arg(status_filter)::text='' OR w.status=sqlc.arg(status_filter)::text)
AND (sqlc.arg(priority_filter)::text='' OR w.priority=sqlc.arg(priority_filter)::text)
AND (sqlc.narg(asset_filter)::uuid IS NULL OR w.asset_id=sqlc.narg(asset_filter)::uuid)
AND (sqlc.narg(assignee_filter)::uuid IS NULL OR w.assigned_to=sqlc.narg(assignee_filter)::uuid)
AND (NOT sqlc.arg(scoped)::boolean OR w.assigned_to=sqlc.arg(actor_id)::uuid);
