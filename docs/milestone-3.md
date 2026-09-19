# Milestone 3 — authentication

Implementation in progress on 2026-09-19. Verification evidence will be added after tests complete.

## Delivered scope

POST /api/v1/auth/login and GET /api/v1/auth/me; bcrypt password hashing; HS256 JWT issuance/verification; current-active-user middleware; explicit administrator bootstrap; development-only demo seed; transaction-bound audit writer. Business permission enforcement and user-management endpoints remain M4.

Tokens require UUID sub, issuer, audience, iat and exp. Lifetime is at most 15 minutes with 30-second clock tolerance. Only HS256 is accepted. The application loads the current active account and current role from PostgreSQL for authenticated requests, so stale roles in a token cannot grant access and disabled accounts fail immediately on their next request. There is no refresh token, token blacklist, logout or public registration endpoint in V1. Password changes alone do not revoke existing tokens; changing JWT_SECRET invalidates them all.

Login normalizes ASCII email, keeps password bytes unchanged, compares bcrypt hashes, and returns the same INVALID_CREDENTIALS response for missing user, incorrect password and inactive user. Missing users undergo a dummy bcrypt comparison at the same configured cost. This reduces a simple timing discrepancy; it is not a guarantee of identical request timings. Database failures return sanitized 503. Auth responses include Cache-Control: no-store. Request/operational logs omit submitted email, token, passwords and hashes.

JSON input is bounded to 1 MiB. The login DTO rejects duplicate fields, trailing JSON, invalid types, unknown fields, nulls, missing/invalid email and oversized passwords. Creation/bootstrap passwords require 12–72 UTF-8 bytes; login accepts 1–72 bytes, with no truncation. Bootstrap names are trimmed, valid UTF-8 and 1–100 characters.

## Bootstrap and transaction boundaries

`make bootstrap` reads DATABASE_URL and BOOTSTRAP_ADMIN_NAME/EMAIL/PASSWORD from the environment. It checks schema readiness, hashes at bcrypt cost 12, acquires transaction-scoped user-administration advisory lock 716493001, refuses if any ADMIN exists (including inactive), and atomically inserts the user and USER_BOOTSTRAPPED audit. The audit actor is null for that one-time command. Audit snapshots have an explicit safe structure with only id/name/email/role/is_active. The command prints success or sanitized failure, not credentials.

The same lock must be reused by M4 user administration. A failed audit write rolls back the inserted user. Database errors are not retried automatically after uncertain commit results. Bootstrap uses runtime permissions, not migration-owner credentials.

## Running locally

Start and migrate PostgreSQL using the M2 guide, then export a newly generated signing secret before `make run`:

```bash
export JWT_SECRET="$(openssl rand -hex 32)"
export JWT_ISSUER=gowork
export JWT_AUDIENCE=gowork-api
```

There is no usable default JWT secret in .env.example. The API fails startup if the secret is shorter than 32 bytes or issuer/audience is empty. Generation above yields a new key, so changing it invalidates prior tokens.

For an empty local demo database, run `make seed-demo` with APP_ENV=development and DATABASE_URL set to the runtime URL from .env.example. It creates:

| Email | Role |
|---|---|
| admin@gowork.dev | ADMIN |
| manager@gowork.dev | MANAGER |
| technician@gowork.dev | TECHNICIAN |

The deliberately public local-only password is `GoWork-demo-only-2026!`. Do not publish these accounts on a public service. The command refuses production environment, never runs automatically, preserves existing users, and is idempotent for compatible demo accounts. It refuses to bootstrap a new demo admin if a different admin already exists. If a preserved demo account has a changed password, the command does not reset it.

For your own initial admin, use `make bootstrap` instead. Set the three BOOTSTRAP_ADMIN_* variables through your local secure environment; clear BOOTSTRAP_ADMIN_PASSWORD afterward. Do not put real passwords in the repository or command-line arguments. Choose bootstrap or demo seed for the initial database, not both.

Example local demo login:

```bash
curl -i http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@gowork.dev","password":"GoWork-demo-only-2026!"}'
```

The response includes data.access_token, token_type, expires_in and a sanitized user. Pass that token using `Authorization: Bearer <access_token>` to GET /api/v1/auth/me.

## Verification

Required checks: formatting, vet, race tests, API/bootstrap/seed builds, reproducible sqlc, token rejection cases, JSON validation, password bounds, missing/inactive account handling, real HTTP-to-PostgreSQL login/me, simultaneous bootstrap, audit-failure rollback, audit sanitization and demo seed idempotency. Tests run against the same PostgreSQL 16 workflow used in M2 because this workspace does not provide native PostgreSQL/Docker.

Evidence pending. No public deployment is claimed.

## References

- [golang-jwt parser options](https://golang-jwt.github.io/jwt/usage/parse/) for algorithm and claim validation.
- [Go bcrypt documentation](https://pkg.go.dev/golang.org/x/crypto/bcrypt) for hashing and its 72-byte input limit.
