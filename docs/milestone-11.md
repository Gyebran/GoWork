# Milestone 11 — API documentation and Swagger UI

Implementation prepared; full CI/browser verification pending.

## Use the documentation

Start the application with the [M9 setup](milestone-9.md), then open `http://localhost:8080/docs`. The raw contract is `http://localhost:8080/openapi.yaml`. These are local addresses, not a public deployment. Both endpoints are public and require no database query themselves; normal application startup still requires valid configuration.

1. Create an initial account with bootstrap, or explicitly seed local demo accounts as described in M3/M9.
2. Expand POST `/api/v1/auth/login`, click Try it out, replace the example with your own credentials and Execute.
3. Copy `data.access_token`. Click Authorize and paste the token only; Swagger adds the Bearer prefix.
4. Try GET `/api/v1/auth/me`, then the endpoints permitted to your role. Reloading the page clears Swagger authorization; log in again after the 900-second token lifetime.

Try it out performs real operations on the server you are viewing. Example account passwords/IDs in the schema are illustrations, not provisioned accounts. Create resources and substitute returned IDs before assignment/status calls. The server URL `/` means same origin, including a custom host port or a future HTTPS deployment; it does not force requests to localhost in production.

## Implementation and contract review

- The checked-in `docs/openapi.yaml` and Swagger HTML are embedded into the Go binary. There is no second copied/generated specification and no runtime file-path dependency. Docker build inputs explicitly include these three documentation source files.
- GET /docs returns HTML; GET /openapi.yaml returns application/yaml. Both retain shared request-ID/logging/recovery middleware, disable MIME sniffing and use no-cache. Unsupported methods preserve the router's JSON 405/Allow contract.
- Swagger UI 5.33.0 JavaScript/CSS are loaded from exact jsDelivr URLs with SHA-384 integrity checks. Browser access to that CDN is required for the interactive UI; downloading the YAML does not require CDN access. Token persistence, external validator calls and query-string configuration overrides are disabled. API requests are restricted to the current origin. No wildcard CORS or example account auto-login is added.
- All 22 operations now include readable summaries/descriptions. The contract documents roles, technician scope, list filters, pagination, lifecycle rules, immutable fields, write atomicity, token behavior, error responses and request/response examples.
- Review found missing 422 responses on resource-ID routes for invalid UUIDs/unsupported queries, and an unused 409 response on login. OpenAPI now reflects those existing behaviors. Planned documentation descriptions and the fixed localhost server URL were replaced. No business behavior or migration was changed.
- Successful examples and referenced component examples are checked against OpenAPI schemas. Password lengths remain explicitly specified in UTF-8 bytes because OpenAPI character length alone cannot express bcrypt's byte limit.

## Verification

`cmd/api/routes_test.go` walks the same router factory used by production and compares every method/path with the embedded contract, rejecting extra/missing routes. It also requests both public documentation endpoints, checks the exact served specification and request ID, and verifies unsupported-method handling.

`make docs-smoke` extends the isolated M9 Docker test project. It validates successful HTTP responses for all 22 documented operations, successful request bodies, response schemas/content types, documented headers, Location, request correlation, nullable order fields, order lifecycle and audit output. Representative 400/401/403/404/409/413/415/422 responses are schema-checked. Existing integration tests retain deeper permission/rollback/race/error coverage; these representative checks do not claim exhaustive semantic proof for every possible request.

Chromium then loads the actual /docs page, checks all operations rendered, executes /health through Try it out, authorizes a test token through the UI, executes /auth/me, reloads and verifies authorization was cleared. Python/browser dependencies are version-pinned in `scripts/requirements-docs.txt` and used only in tests, never in the application image. The CI Docker job installs Chromium and runs this combined check; the other required CI jobs remain enabled.

To run locally (Docker and Python 3.12 required):

```sh
python3 -m venv /tmp/gowork-docs
/tmp/gowork-docs/bin/python -m pip install -r scripts/requirements-docs.txt
/tmp/gowork-docs/bin/python -m playwright install --with-deps chromium
PATH=/tmp/gowork-docs/bin:$PATH make docs-smoke
```

The smoke command creates and removes its own disposable database project. It is not intended for production targets. `make docker-smoke` remains available for the original packaging-only check without browser dependencies.

Reference: [Swagger UI configuration](https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/).
