# Milestone 1 — Go foundation

Completed 2026-09-10 following the owner's explicit approval of Milestone 0.

## Delivered

- Module `github.com/Gyebran/GoWork`, Go 1.27.1, chi v5.3.2 pinned with checksums.
- Composition root in `cmd/api`; validated APP_ENV, PORT and LOG_LEVEL.
- net/http server binding 0.0.0.0 with read-header/read/write/idle timeouts.
- chi router serving GET /health with exact `{"status":"ok"}` JSON.
- UUID v4 request IDs or validated incoming IDs; duplicate headers replaced.
- JSON slog access logs with correlation ID, matched route, method, status, duration.
- JSON 404/405 responses, dynamic Allow header, sanitized panic recovery.
- 10-second request timeout with cancellation and a correlated JSON 503.
- SIGINT/SIGTERM handling, 20-second graceful drain, forced close on deadline.
- `.env.example`, `.gitignore`, Makefile, local setup instructions and behavior tests.

## Decisions and limits

Timeout wraps the matched endpoint, after chi finishes routing. This avoids reading mutable routing state from access logging while a timed-out handler is still unwinding. HTTP TimeoutHandler buffers the endpoint response so a panic/timeout cannot leak partial JSON; this foundation is for bounded REST responses, not streaming. A handler that ignores context can keep running after the client receives a timeout; future service/DB code must propagate and honor context cancellation.

The process signal context does not parent request contexts: graceful shutdown must allow existing requests to finish. An independent bounded context controls drain. If it expires, server.Close cancels active connections; main exits nonzero on the shutdown error. Future pool cleanup belongs after HTTP drain.

Panic values, raw request paths, query strings, bodies and authorization headers are not written to access logs. Unknown routes log `[unmatched]`; matched endpoints log route templates. Passwords and other secrets have no role in this milestone. No .env autoload or DB/JWT placeholders masquerade as functional integrations.

M1 introduces no endpoints accepting JSON bodies. The 1 MiB limit and strict JSON decoding in the contract will be implemented when the first body-consuming auth endpoint arrives in M3. `/ready` remains 404 until actual database readiness exists in M2. The complete OpenAPI remains a V1 contract; only `/health` is implemented today. No database, authentication, assets, work orders, container deployment, or hosted documentation is claimed.

## Verification performed

Environment: Linux amd64, Go 1.27.1. Toolchain archive SHA-256 checked against the official download checksum before extraction. Dependency resolution pinned chi v5.3.2; no floating version remains in go.mod.

`make check` passed:

- gofmt check;
- `go vet ./...`;
- `go test -race ./...`;
- `go build -trimpath -o bin/gowork-api ./cmd/api`.

Tests cover configuration defaults/overrides/rejections; valid/generated/invalid/duplicate request IDs; health JSON; missing routes and 405 Allow; log redaction; request timeout and parent cancellation; sanitized panic after partial buffered output; graceful drain with a live TCP request; shutdown deadline force-close; listener failure propagation. Shutdown tests synchronize request start/finish with channels rather than sleep-based guesses.

Separate live compiled-process smoke checks passed:

1. GET /health over loopback HTTP → 200 and exact JSON, preserving `smoke-check` request ID.
2. POST /health → JSON 405 and Allow: GET.
3. SIGTERM and SIGINT each → clean process exit 0 with server_stopped log.
4. Invalid PORT → exit 1 without echoing the supplied value.
5. Occupied port → exit 1 with listen_failed log.

The smoke processes were stopped after verification. No permanently running or publicly hosted service was created. Unit tests for the main package are not claimed; process behavior was checked using the compiled executable. Database/SQL/container/CI/deployment verification belongs to later milestones.

## Next milestone

M2: real PostgreSQL setup, schema/catalog migrations, pgx pool, sqlc, transaction runner and /ready. Reuse this HTTP foundation; preserve the approved database and transactional audit contracts.
