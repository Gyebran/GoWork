# GoWork

Work Order Management REST API — a backend engineering portfolio project.

**Status: Milestones 0–10 complete; unified CI passed and intentional test-failure gating verified.**

GoWork will manage operational assets, technician assignments and work-order lifecycles with database-backed permissions and transactional auditing. It is a modular monolith with no frontend or ORM.

Currently implemented: Go configuration, chi router, JSON slog logging, request IDs, bounded HTTP timeouts, JSON routing/errors, `/health`, and graceful shutdown. PostgreSQL migrations, pgx/sqlc, transactions and readiness are now implemented; authentication now includes login, current-user lookup, JWT, bcrypt, admin bootstrap and local demo seeding. Database-backed RBAC and user list/detail/create/update are implemented in M4. Asset CRUD, filters, technician scope and transactional auditing are implemented in M5. Work-order creation, scoped reads, metadata updates, assignment and lifecycle transitions are implemented in M6. M7 adds permission-checked audit reads, filters, pagination and snapshot sanitization. The OpenAPI file describes the intended V1 API, not a claim that every route exists.

## Local development and verification

Container setup (Docker Compose v2): copy `.env.example` to `.env`, set JWT_SECRET to a fresh `openssl rand -hex 32` value, then run `docker compose up --build -d --wait`. Check `http://localhost:8080/ready`. Accounts are created explicitly with `make docker-seed` or bootstrap; startup never seeds them automatically. See [Milestone 9 Docker setup](docs/milestone-9.md) for commands and the runtime contract.

Milestone 2 adds the PostgreSQL foundation. Follow [Milestone 2 setup](docs/milestone-2.md) to start the local database, run migrations, grant runtime access and start the API. DATABASE_URL and JWT_SECRET are required. Follow [Milestone 3 setup](docs/milestone-3.md) to configure signing and create initial accounts. GET /health is process liveness; GET /ready checks database and migration readiness.

`make check` runs formatting, vet, race unit tests and API/bootstrap/seed/migration/probe builds. `make test-integration` additionally requires APP_ENV=test and an explicit disposable TEST_DATABASE_URL. `make sqlc` regenerates typed database code. Database tests do not silently pass when the database is missing.

## Continuous integration

The `CI` workflow checks Go formatting/vet/race tests/builds, the OpenAPI contract, sqlc reproducibility, PostgreSQL integration and Docker lifecycle. `CI required` passes only when every prerequisite succeeds. Actions are pinned to commit SHAs; no production credentials are used. See [M10 verification](docs/milestone-10.md) for the successful run and the separate intentional failure demonstration. Requiring this check before merging is a separate GitHub branch-protection setting, not enabled by the workflow itself.

## Project documents

- [Architecture and decision record](docs/milestone-0.md)
- [Database schema plan and ERD](docs/database.md)
- [Authorization, lifecycle, and HTTP contracts](docs/contracts.md)
- [OpenAPI contract](docs/openapi.yaml)
- [Environment, testing, deployment, and milestones](docs/execution.md)
- [Milestone 8 hardening and scenario inventory](docs/milestone-8.md)
- [Milestone 9 Docker packaging](docs/milestone-9.md)
- [Milestone 10 CI pipeline](docs/milestone-10.md)
- [Milestone 11 API documentation](docs/milestone-11.md)
- [Milestone 7 audit system](docs/milestone-7.md)
- [Milestone 6 work orders](docs/milestone-6.md)
- [Milestone 5 asset management](docs/milestone-5.md)
- [Milestone 4 RBAC and user management](docs/milestone-4.md)
- [Milestone 3 authentication and verification](docs/milestone-3.md)
- [Milestone 2 setup and verification](docs/milestone-2.md)
- [Milestone 1 implementation and verification](docs/milestone-1.md)
- [Historical Milestone 0 verification](docs/verification.md)
- [Original project brief](docs/project-brief.txt)

Planned next additions: API documentation and Swagger UI (M11), production deployment (M12), and portfolio polish (M13). PostgreSQL, pgx and sqlc are part of M2. See the architecture record for why these were selected and the milestone checklist for their introduction.
