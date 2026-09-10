# GoWork

Work Order Management REST API — a backend engineering portfolio project.

**Status: Milestone 0 approved; Milestone 1 HTTP foundation implemented.**

GoWork will manage operational assets, technician assignments and work-order lifecycles with database-backed permissions and transactional auditing. It is a modular monolith with no frontend or ORM.

Currently implemented: Go configuration, chi router, JSON slog logging, request IDs, bounded HTTP timeouts, JSON routing/errors, `/health`, and graceful shutdown. Database, authentication and business modules follow in later milestones. The OpenAPI file describes the intended V1 API, not a claim that every route exists.

## Local development

Install Go 1.27.1 and a C compiler to run race tests. No database or Docker is needed for Milestone 1. chi v5.3.2 is pinned in go.mod/go.sum.

```bash
go mod download
go run ./cmd/api
```

In a second terminal:

```bash
curl -i http://localhost:8080/health
```

Expected: HTTP 200, `Content-Type: application/json`, `X-Request-ID` and `{"status":"ok"}`. An incoming valid request ID is preserved. Ctrl+C or SIGTERM drains active requests before exit.

Configuration uses process environment variables; `.env.example` documents defaults. `.env` files are not automatically loaded.

| Variable | Default | Accepted values |
|---|---|---|
| APP_ENV | development | development / test / production |
| PORT | 8080 | 1–65535 |
| LOG_LEVEL | INFO | DEBUG / INFO / WARN / ERROR |

Example in Bash: `PORT=9000 LOG_LEVEL=DEBUG go run ./cmd/api`. In PowerShell: `$env:PORT="9000"` then `go run ./cmd/api`. Invalid configuration exits nonzero without echoing submitted values.

## Verification and build

```bash
go vet ./...
go test -race ./...
go build -trimpath -o bin/gowork-api ./cmd/api
```

With Make installed, `make check` runs formatting checks, vet, race tests and build. Other targets: `make run`, `make fmt`, `make test`, `make build`. `make fmt-check` reports files without modifying them. Start the compiled binary directly (`./bin/gowork-api`) for process/signal testing.

`/ready` starts in M2 with real DB checks. `/docs` and `/openapi.yaml` serving arrive in M11. Unknown paths currently return JSON 404; unsupported methods on `/health` return JSON 405 and Allow. No public deployment exists yet.

## Project documents

- [Architecture and decision record](docs/milestone-0.md)
- [Database schema plan and ERD](docs/database.md)
- [Authorization, lifecycle, and HTTP contracts](docs/contracts.md)
- [OpenAPI contract](docs/openapi.yaml)
- [Environment, testing, deployment, and milestones](docs/execution.md)
- [Milestone 1 implementation and verification](docs/milestone-1.md)
- [Historical Milestone 0 verification](docs/verification.md)
- [Original project brief](docs/project-brief.txt)

Planned next stack additions: PostgreSQL 16, pgx v5, sqlc, explicit SQL, JWT, bcrypt, Docker, GitHub Actions, and Swagger UI. See the architecture record for why these were selected and the milestone checklist for their introduction.
