-- name: GetAsset :one
SELECT * FROM assets WHERE id=$1;
-- name: LockAsset :one
SELECT * FROM assets WHERE id=$1 FOR UPDATE;
-- name: InsertAsset :one
INSERT INTO assets(asset_code,name,category,location,status) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: UpdateAsset :one
UPDATE assets SET name=$2,category=$3,location=$4,status=$5,updated_at=clock_timestamp() WHERE id=$1 RETURNING *;
-- name: DeleteAsset :exec
DELETE FROM assets WHERE id=$1;
-- name: AssetHasOrders :one
SELECT EXISTS(SELECT 1 FROM work_orders WHERE asset_id=$1);
-- name: AssetHasOpenOrders :one
SELECT EXISTS(SELECT 1 FROM work_orders WHERE asset_id=$1 AND status IN ('OPEN','ASSIGNED','IN_PROGRESS'));
-- name: AssetVisibleTo :one
SELECT EXISTS(SELECT 1 FROM work_orders WHERE asset_id=$1 AND assigned_to=$2);
-- name: ListAssets :many
SELECT a.* FROM assets a
WHERE (sqlc.arg(status_filter)::text='' OR a.status=sqlc.arg(status_filter)::text)
AND (sqlc.arg(category_filter)::text='' OR a.category=sqlc.arg(category_filter)::text)
AND (sqlc.arg(search_pattern)::text='' OR a.name ILIKE sqlc.arg(search_pattern)::text OR a.asset_code ILIKE sqlc.arg(search_pattern)::text OR a.location ILIKE sqlc.arg(search_pattern)::text)
AND (NOT sqlc.arg(scoped)::boolean OR EXISTS(SELECT 1 FROM work_orders w WHERE w.asset_id=a.id AND w.assigned_to=sqlc.arg(actor_id)::uuid))
ORDER BY a.created_at DESC,a.id DESC LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;
-- name: CountAssets :one
SELECT count(*) FROM assets a
WHERE (sqlc.arg(status_filter)::text='' OR a.status=sqlc.arg(status_filter)::text)
AND (sqlc.arg(category_filter)::text='' OR a.category=sqlc.arg(category_filter)::text)
AND (sqlc.arg(search_pattern)::text='' OR a.name ILIKE sqlc.arg(search_pattern)::text OR a.asset_code ILIKE sqlc.arg(search_pattern)::text OR a.location ILIKE sqlc.arg(search_pattern)::text)
AND (NOT sqlc.arg(scoped)::boolean OR EXISTS(SELECT 1 FROM work_orders w WHERE w.asset_id=a.id AND w.assigned_to=sqlc.arg(actor_id)::uuid));
