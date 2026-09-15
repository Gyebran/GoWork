CREATE TABLE users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL CHECK (name = btrim(name) AND length(name) BETWEEN 1 AND 100),
 email text NOT NULL UNIQUE CHECK (email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 254),
 password_hash text NOT NULL CHECK (length(password_hash) = 60),
 role_id smallint NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
 is_active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now() CHECK (updated_at >= created_at)
);
