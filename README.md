# GoWork

Work Order Management REST API — a backend engineering portfolio project.

**Status: Milestones 0–3 complete; authentication and PostgreSQL verification passed.**

GoWork will manage operational assets, technician assignments and work-order lifecycles with database-backed permissions and transactional auditing. It is a modular monolith with no frontend or ORM.

Currently implemented: Go configuration, chi router, JSON slog logging, request IDs, bounded HTTP timeouts, JSON routing/errors, `/health`, and graceful shutdown. PostgreSQL migrations, pgx/sqlc, transactions and readiness are now implemented; authentication now includes login, current-user lookup, JWT, bcrypt, admin bootstrap and local demo seeding. Business modules follow later. The OpenAPI file describes the intended V1 API, not a claim that every route exists.

## Local development and verification

Milestone 2 adds the PostgreSQL foundation. Follow [Milestone 2 setup](docs/milestone-2.md) to start the local database, run migrations, grant runtime access and start the API. DATABASE_URL and JWT_SECRET are required. Follow [Milestone 3 setup](docs/milestone-3.md) to configure signing and create initial accounts. GET /health is process liveness; GET /ready checks database and migration readiness.

`make check` runs formatting, vet, race unit tests and API/bootstrap/seed builds. `make test-integration` additionally requires APP_ENV=test and an explicit disposable TEST_DATABASE_URL. `make sqlc` regenerates typed database code. Database tests do not silently pass when the database is missing.

## Project documents

- [Architecture and decision record](docs/milestone-0.md)
- [Database schema plan and ERD](docs/database.md)
- [Authorization, lifecycle, and HTTP contracts](docs/contracts.md)
- [OpenAPI contract](docs/openapi.yaml)
- [Environment, testing, deployment, and milestones](docs/execution.md)
- [Milestone 3 authentication and verification](docs/milestone-3.md)
- [Milestone 2 setup and verification](docs/milestone-2.md)
- [Milestone 1 implementation and verification](docs/milestone-1.md)
- [Historical Milestone 0 verification](docs/verification.md)
- [Original project brief](docs/project-brief.txt)

Planned next additions: RBAC and user management (M4), followed by business modules, application Docker packaging, the full CI pipeline, and Swagger UI. PostgreSQL, pgx and sqlc are part of M2. See the architecture record for why these were selected and the milestone checklist for their introduction.
