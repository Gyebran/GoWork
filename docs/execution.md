# Environment, testing, deployment and execution

## Environment plan

No actual secrets or runnable .env file are created in M0. Future .env.example contains placeholders only; ignore .env and credential files from the first code commit. M1 validates only foundation settings; DB settings become required at M2 and JWT settings at M3. Full V1 validates all settings at startup and exits with a sanitized error on invalid configuration.

| Variable | Default / required | Validation and purpose |
|---|---|---|
| APP_ENV | development | development/test/production only |
| PORT | 8080 | Integer 1–65535; bind 0.0.0.0 |
| LOG_LEVEL | INFO | DEBUG/INFO/WARN/ERROR; structured slog |
| DATABASE_URL | Required M2+ | Runtime pgx connection string; never logged; production certificate-verifying TLS |
| MIGRATION_DATABASE_URL | Required for migration command only | Schema-owning credentials; never injected into API container |
| TEST_DATABASE_URL | Required integration suite only | Isolated expendable test DB, never fallback to DATABASE_URL |
| DB_MAX_CONNS | 10 | Integer 1–50; sum across replicas below provider limit |
| JWT_SECRET | Required M3+ | At least 32 random bytes, no default; secret manager |
| JWT_ISSUER | gowork | Nonempty; must match verification |
| JWT_AUDIENCE | gowork-api | Nonempty; must match verification |
| BOOTSTRAP_ADMIN_NAME | Bootstrap invocation only | Trimmed 1–100 |
| BOOTSTRAP_ADMIN_EMAIL | Bootstrap invocation only | Valid normalized ASCII email |
| BOOTSTRAP_ADMIN_PASSWORD | Bootstrap invocation only | 12–72 UTF-8 bytes; never persisted in env example/logs |

Keep fixed policy constants out of unnecessary environment knobs: access token TTL 15m; clock tolerance 30s; bcrypt cost 12; request timeout 10s; ReadHeaderTimeout 5s; ReadTimeout 15s; WriteTimeout 20s; IdleTimeout 60s; shutdown grace 20s; ready DB ping timeout 2s; body 1 MiB. Write timeout exceeds handler deadline. Integration tests configure time/clock through narrow test seams, not production global variables.

Server stops readiness on SIGTERM/SIGINT, calls net/http Shutdown with bounded independent context, allows active requests to drain, then closes pool. If drain expires, close remaining connections and exit with a logged reason. `/health` reflects process liveness only; `/ready` checks DB and expected schema version (M2+) and shutdown state, returning sanitized 503. M1 delivers `/health`; `/ready` waits for M2 rather than claiming database readiness early.

## Testing strategy

No application tests can run in M0 because no application exists. Each implementation milestone must add behavior tests as it introduces behavior. M8 is consolidation and concurrency testing, not the first test pass. Aim for at least 40 meaningful scenarios, including these:

| Group | Required scenarios |
|---|---|
| Foundation (1–5) | health shape/header; config rejection; request ID sanitization; timeout/context cancellation; graceful shutdown drains request |
| Database (6–9) | up/down/up fresh DB; catalog mappings exact; FK/check rejection; sqlc generation reproducible |
| Auth (10–16) | correct login; indistinguishable invalid/missing/inactive credentials; expired JWT; invalid signature/algorithm; issuer/audience/missing claims; no hashes/secrets returned; multibyte bcrypt limit |
| Users/RBAC (17–22) | admin create allowed/manager forbidden; manager user read; technician user read forbidden; deactivation affects token; self/last-admin guard; concurrent deactivation versus assignment |
| Assets (23–28) | admin/manager create; technician create forbidden; only admin delete; referenced asset delete conflict; scoped technician list/detail/count; literal search+filters+pagination |
| Orders (29–35) | server-owned creation fields rejected; valid assignment; invalid target; technician cannot modify other's order; allowed lifecycle and disallowed jumps; reassignment/terminal rejection; scoped list cannot be widened |
| Atomicity (36–40) | forced audit failure rolls back asset/user/order writes (subcases); simultaneous completion one success; concurrent unique number creation; list+count consistent snapshot; audit sanitization and permission enforcement |
| Additional contract cases | duplicate keys/unknown/null/empty patches; bad queries; asset retire/create race; same-value metadata no-op; missing assets; no mutation on authorization failure; ambiguous commit response handling |

Unit tests cover service decisions with small fakes and injected clock where necessary. Full HTTP integration uses httptest with actual chi middleware/service/pgx and isolated PostgreSQL; mock-only tests do not prove SQL transactions. Force audit insert failure in a dedicated test connection/fixture and assert business rows are unchanged. Concurrency tests use controlled barriers, not sleep-based races. Exercise runtime DB role to prove audit UPDATE/DELETE are denied.

Test database cleanup must be explicitly targeted to a dedicated database (name ending `_test` or unique disposable database), refuse production APP_ENV, and never reuse production credentials. Parallel tests use isolated databases/schemas; no shared mutable fixtures. Do not skip missing-DB tests silently in CI: required integration job fails on missing infrastructure.

Future verification commands: `gofmt` check, `go vet ./...`, `go test -race ./...`, full integration suite with TEST_DATABASE_URL, `go build ./cmd/api`, migrations up/down/up, `sqlc generate` plus clean generated diff, OpenAPI validation, Docker build. M2 may introduce a DB-only Compose service to make database verification possible before full M9 packaging. Go/Docker/PostgreSQL binaries were not available on PATH in the M0 environment; provision the needed runtime at the corresponding milestone rather than claiming tests passed.

## Deployment strategy

Target: Render Docker web service + same-region managed PostgreSQL, separate development/test/production databases. This is a proposed target, not an existing deployment. At M12 verify available plans, PostgreSQL version, connection limits, backup/restore support and actual cost before provisioning paid resources. No pricing, free-tier persistence or always-on guarantee is assumed.

M9: multi-stage pinned Go builder, small nonroot runtime with CA roots, executable runs directly for signal handling, no secrets in build args/layers, HTTP bind 0.0.0.0:$PORT. Local Compose has postgres healthcheck, one-shot migration runner, then API; local migrations use only local credentials. Repeat startup should not re-run demo bootstrap or reset data.

M10: pull requests/push run formatting, vet, race/unit tests, isolated Postgres integration, reproducible sqlc and contract checks, Go build, Docker build; tests must gate success. Pin action versions, minimize workflow permissions, no production secrets for PR checks. CI is not automatic production deployment permission.

M12 release sequence: successful CI → build immutable commit-tagged image → backup/verify restore procedure → run additive migrations once with separate credentials → deploy same image → poll /ready → verify public HTTPS /health and /docs → authenticated smoke check on dedicated safe data → record release SHA and URLs. If migration fails do not release. Rollback application to previous image only when schema-compatible; avoid automatic destructive down migrations. Platform restart policy should restart failed processes; liveness should not create DB-outage restart loops. Require production-ready TLS termination and credential-verified DB TLS; never expose database port publicly merely for the API.

Public docs may describe auth, but no writable demo administrator password is published. Bootstrap credentials travel through a one-time secure job and are removed afterward. No CORS wildcard or cookie auth is needed for same-origin Swagger UI. Login rate limiting remains V2 application scope per brief; before public deployment choose provider/edge throttling with trusted proxy configuration or record it as a release blocker. Operational logs redact credentials and request bodies. Domain is optional; actual public HTTPS URL is recorded only after verification.

## Milestone execution checklist

Original numbering 0–13 remains intact (14 milestones total). M0 is approved; M1 is implemented (verification in milestone-1.md). M2–M13 remain planned. Before each milestone inspect repository status and instructions, explain changes/dependencies, implement only its slice, verify, fix, and update docs.

| Milestone | Dependencies | Scope and exit gate |
|---|---|---|
| 0 Planning | Original brief + repository | Architecture, structure, ERD/schema, permission matrix, API/error/lifecycle, env/tests/deploy/checklist; validate cross-document consistency; owner reviews before M1 |
| 1 Go Foundation | Accepted M0 | Go module/toolchain, config, chi, slog, request ID, bounded server and shutdown; GET /health verified locally |
| 2 Database Foundation | M1 | 7 tables, constraints/indexes/catalog seeds, migrations, pgx, sqlc and transaction runner; DB-only local Compose if needed; /ready verifies DB/schema; up/down/up and generation pass |
| 3 Authentication | M2 | bcrypt/JWT/login/me/middleware; bootstrap command and minimal transactional audit writer; dev users fixture; auth and bootstrap atomicity tests |
| 4 RBAC + User Management | M3 | DB permission checks, ownership policy functions; user GET/list/POST/PATCH with active-account guards; user mutations audited; RBAC and user tests pass |
| 5 Assets | M4 | Asset CRUD, filters/search/pagination/scope, DB guards; all writes audited; asset integration and permissions tests |
| 6 Work Orders | M5 | Create/list/detail/metadata/assign/status, numbering, row locks, ownership; transactional audit; lifecycle and concurrency tests |
| 7 Audit System | M6 | Audit read API, filtering/sanitization/permission checks; review all earlier write paths for rollback guarantees |
| 8 Testing Hardening | M7 | At least 40 behavior scenarios, full HTTP integration/race/rollback cases; fix concrete defects; no artificial coverage-only cases |
| 9 Docker | M8 | Multi-stage Dockerfile, full Compose startup, migration dependency, nonroot; fresh docker compose up works |
| 10 CI | M9 | Required formatting/vet/test/integration/generation/contract/build/Docker checks; intentional test failure demonstrated to fail job |
| 11 API Documentation | M10 | Validate implementation against M0 OpenAPI, examples, serve /docs and /openapi.yaml; contract reflects actual behavior |
| 12 Production Deployment | M11 | Confirm account/plan, backups, separate DB, secrets/TLS, safe migrations; actual public /health /ready and authenticated smoke pass |
| 13 Portfolio Polish | M12 | Complete README requirements, accurate feature claims, ERD/architecture/tradeoffs, tested setup, real deployment/Swagger URLs, clean repository |

## M0 acceptance

- [x] Repository state inspected and source brief preserved.
- [x] Architecture, future folder structure and dependency ownership specified.
- [x] Seven-table ERD, columns, constraints, indexes, migrations and seeds specified.
- [x] Permission matrix, ownership and lifecycle fixed.
- [x] API schema and HTTP/error conventions specified.
- [x] Environment, testing and deployment plans specified.
- [x] Blueprint weaknesses reconciled explicitly.
- [x] Milestone dependencies/checklist defined; application implementation not started.
- [x] Owner accepted the plan on 2026-09-10 and authorized continuation (brief §60 gate satisfied).

M0 documentation preparation and approval are complete. Milestone 1 implements the foundation; its runtime verification is recorded separately in milestone-1.md.
