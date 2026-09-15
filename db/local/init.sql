-- Local development only. Never reuse this published password in production.
CREATE ROLE gowork_app LOGIN PASSWORD 'gowork_dev_only';
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
CREATE DATABASE gowork_test;
