# Milestone 2 — database foundation

Implementation prepared 2026-09-15. Final PostgreSQL verification is recorded below when the database workflow completes.

## Delivered

Nine paired up/down migrations define seven application tables, checks, restricted foreign keys, a global work-order sequence, indexes, three roles, fourteen permissions and twenty-eight mappings. No password fixtures or user-facing business features are added.

pgx v5 provides the connection pool. Configuration requires DATABASE_URL and validates DB_MAX_CONNS (1–50, default 10); production URLs require sslmode=verify-full. Pool creation is lazy so the process can stay alive during a database outage. GET /ready runs a bounded query that checks schema_migrations version=9 and dirty=false. This query also exercises an actual database connection. Missing tables, dirty/wrong version, unavailable DB and shutdown return sanitized 503. GET /health remains process-only.

sqlc v1.31.1 generates typed pgx code from migrations and explicit catalog SQL. The transaction runner supplies both pgx.Tx and transaction-bound sqlc queries, rolls back using an independent bounded cleanup context, and reports ambiguous commit failure without automatic retries. Application audit writing starts in M3; M2 tests establish the atomicity needed for it.

The migration CLI uses golang-migrate v4; credentials come from MIGRATION_DATABASE_URL and are never command-line arguments. Up applies pending migrations; down reverses one migration only and requires explicit development/test environment. Production rollback is not automated. API startup never runs migrations or seeds.

## Local setup

Requirements: Go 1.27.1, Docker with Compose, Make optional.

```bash
cp .env.example .env
set -a
. ./.env
set +a
make db-up
make migrate-up
make db-grants
make run
```

Then `curl -i http://localhost:8080/ready` should return 200 with `{"status":"ready"}`. M2 Compose starts PostgreSQL only; API container packaging remains M9. Initialization creates a separate runtime role and a disposable gowork_test database only for a fresh local volume. Published development passwords are not production credentials. `make db-down` retains the volume.

The runtime role receives catalog/schema-version SELECT, business-table DML, sequence USAGE, and audit SELECT/INSERT only. Apply db/local/grants.sql after migrations. Adapt provisioning to your production role names; no hardcoded cluster roles appear in schema migrations.

```bash
make check
make sqlc
APP_ENV=test make test-integration
```

The integration suite requires TEST_DATABASE_URL explicitly ending in `_test` and APP_ENV=test. It refuses existing application tables before its up/down/up cycle, never falls back to DATABASE_URL, and fails rather than silently skipping a missing DB. Tests clean their schema afterward; role permission checks roll back their temporary role. Test credentials need schema ownership and CREATEROLE (the isolated CI service uses its disposable owner).

## Verification design

A narrowly scoped database-foundation GitHub Actions workflow was introduced ahead of M10 because this workspace cannot install/run native PostgreSQL or Docker. It uses an ephemeral PostgreSQL 16 service, no production secrets, and read-only GitHub token permissions. Full CI packaging/deployment remains M10/M12.

Checks: formatting/vet/race unit tests/build; reproducible sqlc generation; real PostgreSQL up/down/up/no-change; exact catalogue grants; readiness HTTP and dirty schema; CHECK/FK failures; commit, forced audit-error rollback, cancellation rollback; runtime audit UPDATE/DELETE denial. Database-dependent results must come from the recorded workflow, not mocked checks.

## Verification result

Pending workflow execution. This file must not be treated as evidence of a passing database run until updated with the run URL and result.
