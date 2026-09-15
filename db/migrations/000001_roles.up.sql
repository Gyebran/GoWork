CREATE TABLE roles (
 id smallint PRIMARY KEY,
 name text NOT NULL UNIQUE CHECK (name IN ('ADMIN','MANAGER','TECHNICIAN'))
);
