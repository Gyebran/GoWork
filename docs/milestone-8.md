# Milestone 8 — testing hardening

Implementation prepared on 2026-09-22; verification pending.

## Concrete fixes and added evidence

1. Read-only transactions previously used the same uncertain-commit classification as mutations. A network error on their commit could incorrectly become WRITE_OUTCOME_UNKNOWN. InTx now applies that marker only to write transactions. A narrow BeginTx interface permits deterministic commit-boundary fault injection without changing actual pool behavior.
2. User names and bootstrap names previously admitted U+0000, which PostgreSQL text rejects. They now reject it during validation, before a query. The HTTP regression expects 422 rather than an internal database error.
3. Transaction tests explicitly cover uncertain write commit, server-rejected commit, aborted commit, read-only commit failure, successful commit, callback failure with cancelled request, panic cleanup, and no automatic replay. Rollback receives a fresh bounded cleanup context. Commit fault injection is a unit test seam; it does not claim a live network-partition test.
4. A new httptest TCP server mounts the complete API and uses the documented database grants under a disposable NOLOGIN runtime role via SET ROLE on each connection. It verifies the effective current_user, successful login/create/assign/progress/audit, permission denials, error correlation, and denial of audit UPDATE/DELETE and DDL. This verifies effective SQL privileges, not deployed login credentials.
5. A pgx tracing barrier commits an asset insert after the actual CountAssets query and before ListAssets. The response retains its original count and rows, while the next request sees the insertion. This proves the actual asset list service's REPEATABLE READ behavior without sleep-based timing or a production-only test hook. The other paginated services use the same isolation strategy and retain their existing endpoint checks.

Existing regression tests already exercise forced audit failures, concurrent bootstrap/deactivation, assignment-versus-deactivation, retirement-versus-order creation, concurrent completion and sequence allocation. M8 adds missing evidence rather than replacing those tests with synthetic coverage targets. No new application feature or database migration is introduced.

## Scenario inventory

Each row below identifies an independently asserted behavior; table-driven cases and integration subcases are included. This is a traceability inventory, not a code-coverage percentage.

| # | Scenario | Test source under repository root |
|---|---|---|
| 1 | Health status/envelope and valid/invalid request IDs | `internal/platform/httpx/router_test.go` |
| 2 | Duplicate request IDs replaced | `internal/platform/httpx/router_test.go` |
| 3 | 404/405 JSON and Allow header | `internal/platform/httpx/router_test.go` |
| 4 | Logs omit request bodies, query data and credentials | `internal/platform/httpx/router_test.go` |
| 5 | Timeout cancels handler context and returns JSON | `internal/platform/httpx/router_test.go` |
| 6 | Panic discards buffered output and sanitizes response | `internal/platform/httpx/router_test.go` |
| 7 | Graceful shutdown drains requests | `internal/platform/httpx/server_test.go` |
| 8 | Shutdown deadline forcibly closes connections | `internal/platform/httpx/server_test.go` |
| 9 | Configuration and production DB TLS rejection | `internal/platform/config/config_test.go; internal/platform/database/pool_test.go` |
| 10 | Fresh migration up/down/up and dirty-schema readiness | `tests/integration/database_test.go` |
| 11 | Role/permission catalogue and constraints match contract | `tests/integration/database_test.go` |
| 12 | Test database guard refuses unsafe targets | `tests/integration/database_test.go` |
| 13 | JWT signature, algorithm, issuer/audience and required claims | `internal/auth/auth_test.go` |
| 14 | Expired/future/overlong-lifetime token rejection | `internal/auth/auth_test.go` |
| 15 | Missing, incorrect and inactive login credentials | `tests/integration/auth_test.go` |
| 16 | Malformed, duplicate, unknown, null and oversized login JSON | `internal/auth/auth_test.go` |
| 17 | Multibyte bcrypt password bounds | `internal/auth/auth_test.go` |
| 18 | Bootstrap audit failure rolls back user | `tests/integration/auth_test.go` |
| 19 | Simultaneous bootstrap permits exactly one administrator | `tests/integration/auth_test.go` |
| 20 | Demo seed is idempotent and production is refused | `tests/integration/auth_test.go` |
| 21 | Role permissions and live revocation have no admin bypass | `tests/integration/users_test.go` |
| 22 | User creation email uniqueness and normalization | `tests/integration/users_test.go` |
| 23 | User pagination and filters preserve real empty-page totals | `tests/integration/users_test.go` |
| 24 | User PATCH no-op and safe password audit | `tests/integration/users_test.go` |
| 25 | Self-deactivation and last-admin guards | `internal/users/validation_test.go; tests/integration/users_test.go` |
| 26 | Concurrent admin deactivation retains an active admin | `tests/integration/users_test.go` |
| 27 | User create/update roll back on audit failure | `tests/integration/users_test.go` |
| 28 | Assignment lock blocks deactivation with active order | `tests/integration/users_test.go` |
| 29 | Immutable asset code and role-restricted writes | `tests/integration/assets_test.go` |
| 30 | Literal percent/underscore/backslash asset search | `tests/integration/assets_test.go` |
| 31 | Technician asset scope deduplicates terminal assignments | `tests/integration/assets_test.go` |
| 32 | Asset no-op preserves audit count and timestamp | `tests/integration/assets_test.go` |
| 33 | Active orders block asset retirement and historical orders block deletion | `tests/integration/assets_test.go` |
| 34 | All three asset mutation paths roll back on audit failure | `tests/integration/assets_test.go` |
| 35 | Asset deletion preserves complete audit snapshot | `tests/integration/assets_test.go` |
| 36 | Retirement waits and detects concurrent order creation | `tests/integration/assets_test.go` |
| 37 | Server-owned work-order fields are rejected | `tests/integration/work_orders_test.go` |
| 38 | Unavailable/missing assets reject order creation | `tests/integration/work_orders_test.go` |
| 39 | Inactive/missing/nontechnician assignees are rejected | `tests/integration/work_orders_test.go` |
| 40 | OPEN assignment emits exactly two atomic audits | `tests/integration/work_orders_test.go` |
| 41 | Technician ownership and cancellation restriction | `tests/integration/work_orders_test.go` |
| 42 | Lifecycle matrix, reassignment and terminal guards | `internal/workorders/policy_test.go; tests/integration/work_orders_test.go` |
| 43 | Order metadata no-op and clearing description | `tests/integration/work_orders_test.go` |
| 44 | Scoped list filters cannot widen technician access | `tests/integration/work_orders_test.go` |
| 45 | Failure of second assignment audit rolls back first event and mutation | `tests/integration/work_orders_test.go` |
| 46 | Two concurrent completions produce one completion audit | `tests/integration/work_orders_test.go` |
| 47 | Concurrent numbers remain unique and expand beyond six digits | `tests/integration/work_orders_test.go` |
| 48 | Assignment waits and rejects a concurrently disabled technician | `tests/integration/work_orders_test.go` |
| 49 | Creation waits and rejects a concurrently retired asset | `tests/integration/work_orders_test.go` |
| 50 | Audit role access, live revocation and inactive-account denial | `tests/integration/audit_logs_test.go` |
| 51 | Audit filters, UUID tie ordering and nullable bootstrap actor | `tests/integration/audit_logs_test.go` |
| 52 | Real user/asset/order snapshots and request correlation | `tests/integration/audit_logs_test.go` |
| 53 | Unknown secret fields stripped; malformed public fields fail closed | `internal/auditlog/service_test.go; tests/integration/audit_logs_test.go` |
| 54 | Commit outcome classification and no replay | `internal/platform/database/transaction_test.go` |
| 55 | Cancelled callback and panic still roll back | `internal/platform/database/transaction_test.go` |
| 56 | User/bootstrap NUL name rejection | `internal/users/validation_test.go; internal/auth/auth_test.go; tests/integration/hardening_test.go` |
| 57 | TCP HTTP lifecycle under runtime grants | `tests/integration/hardening_test.go` |
| 58 | Runtime audit UPDATE/DELETE and DDL denied | `tests/integration/hardening_test.go` |
| 59 | Real concurrent insert between count/list does not mix snapshots | `tests/integration/hardening_test.go` |
| 60 | Denied HTTP writes leave row and audit unchanged | `tests/integration/hardening_test.go` |

## Verification

Evidence pending. Required gates: formatting, vet, race unit tests, binaries, reproducible sqlc and complete PostgreSQL integration suite. No public deployment or load-test performance claim. M9 Docker packaging remains unstarted.
