# Milestone 4 — RBAC and user management

Completed and verified on 2026-09-20.

## Delivered scope

| Endpoint | Database permission | Behavior |
|---|---|---|
| GET /api/v1/users | user:read | role/is_active filters, fixed pagination and sort |
| GET /api/v1/users/{id} | user:read | sanitized user or USER_NOT_FOUND |
| POST /api/v1/users | user:create | active account, 201 with Location, USER_CREATED audit |
| PATCH /api/v1/users/{id} | user:update | name/email/password/is_active, USER_UPDATED audit |

The existing authentication middleware loads an active account. RBAC reads explicit role_permissions rows from PostgreSQL; ADMIN has no implicit bypass. Checks run before request validation/resource lookup. Services independently enforce permissions, so another transport cannot accidentally bypass authorization. `/auth/me` still needs no user:read permission. Role is immutable, user deletion and self-service password endpoints are absent.

Lists use one REPEATABLE READ read-only transaction for authorization, count and rows. Empty/out-of-range pages keep the real total and return an empty array. Repeated/unknown/invalid query parameters are 422. Writes share the strict bounded JSON decoder: malformed/duplicate/trailing/wrong-type inputs are 400, unknown/null/empty patches or field bounds are 422. Passwords remain 12–72 UTF-8 bytes and bcrypt cost 12; email is normalized consistently with login.

## Transaction and concurrency contract

All user mutations first acquire advisory lock **716493001**, also used by M3 bootstrap and demo seeding. They recheck current active actor and database permissions after acquiring it. Updates then lock the target user FOR UPDATE. A deactivation refuses the caller (SELF_DEACTIVATION), the last active administrator (LAST_ADMIN), or a user with ASSIGNED/IN_PROGRESS orders (USER_HAS_ACTIVE_WORK_ORDERS). Self-deactivation has priority if both self and last-admin guards apply. In the seeded policy only administrators can update users, so the last-admin guard is defensive in addition to the active-actor and self guards.

The M6 assignment operation must lock the target user FOR SHARE and recheck active TECHNICIAN before writing, without acquiring the administration advisory lock. If assignment commits first, deactivation sees its active order after waiting; if deactivation commits first, assignment sees an inactive target. This milestone tests that database protocol with an assignment fixture; the work-order application endpoint remains M6.

User row plus audit insert commit atomically. An audit failure rolls back creation/update. Snapshots omit password/hash; a password update adds only password_changed=true. Identical metadata patches leave updated_at and audit unchanged. Unique email conflicts return EMAIL_CONFLICT. Deadlock/serialization conflicts return CONCURRENT_MODIFICATION; uncertain mutation commits return WRITE_OUTCOME_UNKNOWN and are not retried automatically. Logs contain only request ID and a stable error code.

`internal/rbac` also provides explicit assigned-resource and assignment-filter scope functions. M5/M6 must combine them with database permission checks and apply the equivalent scope inside SQL list/count queries. These helpers alone do not authorize access to an asset or order.

## Local use

Use the M2 database setup and M3 JWT/bootstrap setup. No new environment variables or schema migration are required. Start with `make run`, login as your administrator, and send the returned token in the Authorization Bearer header. GET /api/v1/users is a quick smoke check; the local demo manager can read users, while the local demo technician receives 403.

## Verification

Required: formatting/vet/build, race unit tests, reproducible sqlc, PostgreSQL integration for permission matrix and live revocation, current account state, create/conflict/input validation, filtered pagination, no-op audit behavior, password audit sanitization, forced create/update audit rollback, concurrent administrator deactivations, and assignment/deactivation locking.

Local `make check` passed with Go 1.27.1; the integration suite also compiles and its test-database guard passes locally. [GitHub Actions run 35512700574](https://github.com/Gyebran/GoWork/actions/runs/35512700574) passed `make check`, reproducible sqlc generation and the complete PostgreSQL 16.10 integration suite on implementation commit `f9e1a6fe80d4cdc75f098dfeb575c6023def3b44`. Tests include earlier M2/M3 checks plus the M4 cases listed above. The completion commit changes documentation only.

No public deployment is claimed. M5 asset features have not started.
