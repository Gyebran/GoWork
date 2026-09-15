-- Run as database owner after migrations; change the role for your deployment.
GRANT USAGE ON SCHEMA public TO gowork_app;
GRANT SELECT ON roles, permissions, role_permissions, schema_migrations TO gowork_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON users, assets, work_orders TO gowork_app;
GRANT SELECT, INSERT ON audit_logs TO gowork_app;
GRANT USAGE ON SEQUENCE work_order_number_seq TO gowork_app;
