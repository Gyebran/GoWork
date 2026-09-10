# Authorization and HTTP contracts

The [OpenAPI file](openapi.yaml) defines payloads and paths. This document defines cross-field, authorization and transactional semantics that schemas cannot express. Both form the M0 contract; later changes must update both together.

## Role-permission seed matrix

Y grants permission; scoped grants additionally enforce the rule below; — denies. No implicit administrator bypass: ADMIN has explicit rows for all permissions.

| ID | Permission | ADMIN | MANAGER | TECHNICIAN |
|---|---|---|---|---|
| 1 | user:create | Y | — | — |
| 2 | user:read | Y | Y | — |
| 3 | user:update | Y | — | — |
| 4 | asset:create | Y | Y | — |
| 5 | asset:read | Y | Y | scoped |
| 6 | asset:update | Y | Y | — |
| 7 | asset:delete | Y | — | — |
| 8 | work_order:create | Y | Y | — |
| 9 | work_order:read | Y | Y | scoped |
| 10 | work_order:update | Y | Y | — |
| 11 | work_order:assign | Y | Y | — |
| 12 | work_order:status:update | Y | Y | scoped |
| 13 | work_order:cancel | Y | Y | — |
| 14 | audit:read | Y | Y | — |

There are 14 active permissions. User role selection on creation is ADMIN/MANAGER/TECHNICIAN; only ADMIN can create users of any role. No role/permission management API. `/auth/me` requires authentication but no user:read permission.

Technician scope: work-order `assigned_to == current_user.id`; list SQL and total count apply this predicate. Asset visibility uses EXISTS on such work orders (no duplicate assets/counts). Terminal assignments still qualify. A technician-supplied different `assigned_to` filter returns 403. Detail access to an existing but out-of-scope order/asset returns 403 per brief; unknown IDs return 404 after coarse permission checking. This intentionally allows limited existence disclosure to authenticated technicians.

## Endpoint matrix

All business routes use `/api/v1`; liveness/readiness/docs do not. All write requests are JSON.

| Method/path | Permission | Success | Notes |
|---|---|---|---|
| GET /health | Public | 200 | `{ "status": "ok" }`; no DB ping |
| GET /ready | Public | 200 | `{ "status": "ready" }`; 503 when DB unavailable/migrations pending/shutting down |
| GET /docs | Public | 200 HTML | Swagger UI in M11 |
| GET /openapi.yaml | Public | 200 YAML | Same versioned contract served in M11 |
| POST /api/v1/auth/login | Public | 200 | Access token and sanitized current user |
| GET /api/v1/auth/me | Authenticated | 200 | Current DB-backed user |
| GET /api/v1/users | user:read | 200 | Filters role, is_active; paginated |
| GET /api/v1/users/{id} | user:read | 200 | Sanitized user |
| POST /api/v1/users | user:create | 201 | Creates active account; Location header |
| PATCH /api/v1/users/{id} | user:update | 200 | name/email/password/is_active; role immutable |
| GET /api/v1/assets | asset:read | 200 | Filters status, category, search; scope applies |
| GET /api/v1/assets/{id} | asset:read | 200 | Scope applies |
| POST /api/v1/assets | asset:create | 201 | Location header |
| PATCH /api/v1/assets/{id} | asset:update | 200 | Mutable fields only; asset_code immutable |
| DELETE /api/v1/assets/{id} | asset:delete | 204 | Only unreferenced assets; audit retained |
| GET /api/v1/work-orders | work_order:read | 200 | Filters status, priority, assigned_to, asset_id |
| GET /api/v1/work-orders/{id} | work_order:read | 200 | Scope applies |
| POST /api/v1/work-orders | work_order:create | 201 | Server sets OPEN, creator and number; Location header |
| PATCH /api/v1/work-orders/{id} | work_order:update | 200 | title/description/priority; nonterminal only |
| PATCH /api/v1/work-orders/{id}/assign | work_order:assign | 200 | `{assigned_to: UUID}`; no unassignment |
| PATCH /api/v1/work-orders/{id}/status | work_order:status:update or work_order:cancel | 200 | Permission chosen from requested target status |
| GET /api/v1/audit-logs | audit:read | 200 | Filters actor_id, action, entity_type, entity_id; paginated |

No public registration, user DELETE, work-order DELETE, work-order asset change, password self-service, refresh/logout, or reopen endpoints. Unregistered paths return 404; unsupported methods on registered paths return 405 with Allow.

## Lifecycle

| Current | Operation / target | Actor | Result and audit |
|---|---|---|---|
| New | Create | ADMIN/MANAGER | OPEN, assigned_to/completed_at null; WORK_ORDER_CREATED |
| OPEN | assign active technician | ADMIN/MANAGER | ASSIGNED; WORK_ORDER_ASSIGNED and WORK_ORDER_STATUS_CHANGED in one transaction |
| ASSIGNED | assign different active technician | ADMIN/MANAGER | ASSIGNED; WORK_ORDER_ASSIGNED |
| ASSIGNED | status IN_PROGRESS | ADMIN/MANAGER or assigned TECHNICIAN | IN_PROGRESS; WORK_ORDER_STATUS_CHANGED |
| IN_PROGRESS | status COMPLETED | ADMIN/MANAGER or assigned TECHNICIAN | COMPLETED; set completed_at from DB; WORK_ORDER_STATUS_CHANGED |
| OPEN/ASSIGNED/IN_PROGRESS | status CANCELLED | ADMIN/MANAGER | CANCELLED; preserve assignee, completed_at null; WORK_ORDER_STATUS_CHANGED |
| Any nonterminal | metadata patch | ADMIN/MANAGER | Same status; WORK_ORDER_UPDATED if changed |
| COMPLETED/CANCELLED | Any write | Nobody | 409 WORK_ORDER_TERMINAL |

All unlisted transitions return 409 INVALID_STATUS_TRANSITION. Status endpoint accepts only IN_PROGRESS, COMPLETED, CANCELLED: ASSIGNED occurs only through assignment, OPEN only through creation. Reassignment during IN_PROGRESS returns 409 INVALID_ASSIGNMENT_STATE. Same-assignee reassignment returns 409 ALREADY_ASSIGNED. Null assignment is 422. Ownership and permissions are checked before transition details are exposed. Same-status repeats return 409, not a success with duplicate audit.

## Authentication

Login email is normalized exactly like user creation. Password 12–72 UTF-8 bytes for creation/change; login accepts nonempty up to 72 bytes. Never silently truncate or normalize passwords. Missing user/wrong password/inactive user returns the same 401 INVALID_CREDENTIALS; dummy bcrypt work for a missing user reduces obvious timing differences. bcrypt cost 12 baseline, benchmark at M3; tests may use a lower cost without changing production defaults.

JWT: HS256 allowlist only; secret at least 32 random bytes; require sub UUID, iss=`gowork`, aud=`gowork-api`, iat, exp; exp <= iat+15m; enforce exp and reasonable future-iat check with max 30s clock tolerance. Reject alg none, wrong signature/issuer/audience/missing claims. No role claim is needed. Bearer header only. Access tokens expire after 900 seconds, no refresh storage or blacklist. Password change does not revoke an already-issued token; deactivation blocks the next authenticated request. Rotating the signing secret invalidates all tokens. Response `Cache-Control: no-store` on auth endpoints. Never log authorization headers, passwords, hashes, or token responses.

## Serialization, pagination and validation

- Object responses: `{ "data": <resource> }`; list: `{ "data": [], "meta": {"page":1,"limit":20,"total":0,"total_pages":0} }`. Health/readiness are the explicit envelope exception; DELETE 204 has no body.
- UUID strings, UTC RFC3339 timestamps, uppercase enum strings; nullable work-order fields present as null. Responses never expose password_hash, raw SQL errors, or database role IDs. User `role` is a role name.
- Default page=1, limit=20; page range 1–100000 and limit 1–100. Invalid, repeated or unknown query parameters return 422. Stable fixed sort created_at DESC,id DESC; no arbitrary sorting. Empty/out-of-range page returns empty list and real total. total_pages=ceil(total/limit), zero for empty set.
- Filters combine with AND; scope always intersects filters. category exact case-sensitive match; search length 1–100 trimmed, literal case-insensitive substring across asset name/code/location. Empty search is 422. Audit entity_id requires entity_type; UUID filters validate before SQL. role/is_active user filters match exact values (`true`/`false`).
- JSON body max 1 MiB; media type application/json required on writes. Reject malformed JSON, duplicate object keys, trailing second JSON value and wrong JSON types with 400. Unknown fields, empty PATCH, null on non-nullable fields, bad enum/UUID/bounds return 422. Reject unknown fields rather than permitting mass assignment.
- Patch omission means unchanged; description may be cleared with empty string. No PATCH field accepts null. Password is writeOnly and absent from returned objects. Status/assignee/creator/number/timestamps/role cannot be smuggled into generic patches.
- Every response includes X-Request-ID. Accept inbound request IDs only matching `^[A-Za-z0-9._-]{1,64}$`; otherwise generate UUID. Use sanitized path templates in logs, avoid query/body dumping.

## Errors

```json
{"error":{"code":"VALIDATION_ERROR","message":"Request validation failed","details":{"title":"title is required"}},"request_id":"9b8e245c-46a7-4d18-b5cc-6c2143f98bb8"}
```

`error.details` optional map string→string, never submitted passwords. `request_id` always equals response header. Codes are stable; messages are descriptive but clients branch on codes.

| HTTP | Code / mapping |
|---|---|
| 400 | INVALID_JSON (syntax, duplicate keys, wrong JSON type) |
| 401 | UNAUTHENTICATED (missing/invalid/expired token or inactive current user); INVALID_CREDENTIALS for login |
| 403 | FORBIDDEN (missing permission or failed ownership) |
| 404 | USER_NOT_FOUND, ASSET_NOT_FOUND, WORK_ORDER_NOT_FOUND; NOT_FOUND for unknown route |
| 405 | METHOD_NOT_ALLOWED |
| 409 | EMAIL_CONFLICT, ASSET_CODE_CONFLICT, ASSET_IN_USE, ASSET_HAS_OPEN_WORK_ORDERS, ASSET_NOT_AVAILABLE, INVALID_ASSIGNEE, USER_HAS_ACTIVE_WORK_ORDERS, SELF_DEACTIVATION, LAST_ADMIN, WORK_ORDER_TERMINAL, INVALID_STATUS_TRANSITION, INVALID_ASSIGNMENT_STATE, ALREADY_ASSIGNED, CONCURRENT_MODIFICATION |
| 413 | PAYLOAD_TOO_LARGE |
| 415 | UNSUPPORTED_MEDIA_TYPE |
| 422 | VALIDATION_ERROR (field/query validation) |
| 500 | INTERNAL_ERROR (unexpected failure; audit insert failure with healthy DB) |
| 503 | SERVICE_UNAVAILABLE (DB unavailable, deadline, readiness failure); WRITE_OUTCOME_UNKNOWN (ambiguous commit) |

Known FK/unique failures map to domain conflicts; do not expose SQLSTATE to clients. Unknown internal errors log request ID and sanitized cause. A nonexistent referenced asset on order creation returns 404 ASSET_NOT_FOUND; nonexistent/inactive/nontechnician assignment target returns 409 INVALID_ASSIGNEE. A nonexisting primary resource on PATCH/DELETE returns its 404.

## Audit contract

Actions: USER_BOOTSTRAPPED, USER_CREATED, USER_UPDATED, ASSET_CREATED, ASSET_UPDATED, ASSET_DELETED, WORK_ORDER_CREATED, WORK_ORDER_ASSIGNED, WORK_ORDER_STATUS_CHANGED, WORK_ORDER_UPDATED.

Creation has old_value=null, new_value=sanitized snapshot; deletion the reverse. User snapshots include id/name/email/role/is_active, never password or hash. Password-only updates record `password_changed: true` in the new snapshot without secret material. Asset/work-order snapshots contain public resource fields; update records contain before/after sanitized values. Assignment from OPEN writes both assignment and status events atomically. `request_id` groups events; timestamps/UUID ordering is deterministic for listing but is not a causal order within the transaction. Failed/no-op writes create no mutation audit rows. Security failures go to operational logs. Managers/admins may read all audit rows; technicians may not.
