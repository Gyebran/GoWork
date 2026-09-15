CREATE SEQUENCE work_order_number_seq AS bigint START WITH 1 NO CYCLE;
CREATE TABLE work_orders (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 work_order_number text NOT NULL UNIQUE CHECK (work_order_number ~ '^WO-[0-9]{4}-[0-9]{6,}$'),
 asset_id uuid NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
 title text NOT NULL CHECK (title = btrim(title) AND length(title) BETWEEN 1 AND 200),
 description text NOT NULL DEFAULT '' CHECK (length(description) <= 5000),
 priority text NOT NULL DEFAULT 'MEDIUM' CHECK (priority IN ('LOW','MEDIUM','HIGH','CRITICAL')),
 status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','ASSIGNED','IN_PROGRESS','COMPLETED','CANCELLED')),
 created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 assigned_to uuid REFERENCES users(id) ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now() CHECK (updated_at >= created_at),
 completed_at timestamptz,
 CHECK (status <> 'OPEN' OR assigned_to IS NULL),
 CHECK (status NOT IN ('ASSIGNED','IN_PROGRESS','COMPLETED') OR assigned_to IS NOT NULL),
 CHECK ((status = 'COMPLETED') = (completed_at IS NOT NULL)),
 CHECK (completed_at IS NULL OR completed_at >= created_at)
);
