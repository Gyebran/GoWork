CREATE TABLE audit_logs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 actor_id uuid REFERENCES users(id) ON DELETE RESTRICT,
 action text NOT NULL CHECK (action IN ('USER_BOOTSTRAPPED','USER_CREATED','USER_UPDATED','ASSET_CREATED','ASSET_UPDATED','ASSET_DELETED','WORK_ORDER_CREATED','WORK_ORDER_ASSIGNED','WORK_ORDER_STATUS_CHANGED','WORK_ORDER_UPDATED')),
 entity_type text NOT NULL CHECK (entity_type IN ('user','asset','work_order')),
 entity_id uuid NOT NULL,
 old_value jsonb CHECK (old_value IS NULL OR jsonb_typeof(old_value) = 'object'),
 new_value jsonb CHECK (new_value IS NULL OR jsonb_typeof(new_value) = 'object'),
 request_id text NOT NULL CHECK (length(request_id) BETWEEN 1 AND 64),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK (old_value IS NOT NULL OR new_value IS NOT NULL),
 CHECK ((action = 'USER_BOOTSTRAPPED') = (actor_id IS NULL)),
 CHECK ((entity_type = 'user' AND action IN ('USER_BOOTSTRAPPED','USER_CREATED','USER_UPDATED'))
 OR (entity_type = 'asset' AND action IN ('ASSET_CREATED','ASSET_UPDATED','ASSET_DELETED'))
 OR (entity_type = 'work_order' AND action IN ('WORK_ORDER_CREATED','WORK_ORDER_ASSIGNED','WORK_ORDER_STATUS_CHANGED','WORK_ORDER_UPDATED')))
);
