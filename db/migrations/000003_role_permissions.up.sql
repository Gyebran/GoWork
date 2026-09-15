CREATE TABLE role_permissions (
 role_id smallint NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
 permission_id smallint NOT NULL REFERENCES permissions(id) ON DELETE RESTRICT,
 PRIMARY KEY (role_id, permission_id)
);
