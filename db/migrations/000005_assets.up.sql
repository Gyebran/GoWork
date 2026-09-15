CREATE TABLE assets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 asset_code text NOT NULL UNIQUE CHECK (asset_code ~ '^[A-Z0-9][A-Z0-9_-]{0,49}$'),
 name text NOT NULL CHECK (name = btrim(name) AND length(name) BETWEEN 1 AND 150),
 category text NOT NULL CHECK (category = btrim(category) AND length(category) BETWEEN 1 AND 100),
 location text NOT NULL CHECK (location = btrim(location) AND length(location) BETWEEN 1 AND 200),
 status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','MAINTENANCE','RETIRED')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now() CHECK (updated_at >= created_at)
);
