CREATE TABLE permissions (
 id smallint PRIMARY KEY,
 code text NOT NULL UNIQUE CHECK (length(code) BETWEEN 1 AND 80),
 description text NOT NULL CHECK (length(description) BETWEEN 1 AND 250)
);
