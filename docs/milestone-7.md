# Milestone 7 — audit system

Implementation prepared on 2026-09-22; verification pending.

## Read API

GET /api/v1/audit-logs requires an active authenticated account and explicit audit:read permission from PostgreSQL. The seeded ADMIN and MANAGER roles have access; TECHNICIAN does not. Permission checking precedes query validation and is repeated inside the read transaction. There is no administrator bypass or audit mutation API.

Filters actor_id, action, entity_type and entity_id combine with AND. entity_id requires entity_type. UUIDs, fixed action/type catalogues, unknown/repeated parameters, and page/limit bounds follow the established contract. Valid but incompatible filters return an empty result. Actor filtering accepts UUIDs, not the text null; bootstrap rows can be selected with action=USER_BOOTSTRAPPED.

List/count use identical predicates in one read-only REPEATABLE READ transaction, ordered by created_at DESC,id DESC. Default page/limit is 1/20; maximum is 100000/100. Empty or out-of-range pages return [] and the real total. UUID ordering resolves equal timestamps deterministically; it does not imply event causality within a transaction. Read failures never claim an uncertain business write.

## Snapshot boundary

The read API lives in internal/auditlog; internal/audit remains the transaction-bound writer used by authentication/bootstrap, avoiding a dependency cycle. Public entries include id, nullable actor_id, action, entity_type, entity_id, nullable old_value/new_value, request_id and UTC created_at.

Each snapshot is projected through a type-specific allowlist before serialization. User snapshots expose only id/name/email/role/is_active and optional boolean password_changed. Asset and work-order snapshots expose their public resource fields, including explicit nulls for absent assignment/completion. Unknown stored fields are omitted, including nested content. Wrong types in recognized fields fail the whole response with sanitized 500 rather than emitting raw JSON. SQL null snapshots remain JSON null. Public free-text values remain unchanged; this boundary prevents secret fields, not arbitrary secrets someone might type into a name or description.

No stored audit is rewritten by the API. Defensive read sanitization supplements the existing safe writer DTOs. Runtime database privileges remain INSERT/SELECT on audit_logs, with UPDATE/DELETE and DDL unavailable. Database owners can still alter storage; this is application audit integrity, not tamper-proof storage.

## Review of existing mutation paths

| Mutation path | Transaction/audit review | Existing PostgreSQL evidence rerun in M7 |
|---|---|---|
| Bootstrap | User plus safe USER_BOOTSTRAPPED under administration lock | Forced audit failure leaves no user; concurrent bootstrap has one success |
| Explicit demo seed | All inserted users and safe audits share one transaction; existing users preserved | Repeat seed produces no duplicate users/audits; production refused |
| User create/update | Safe snapshots, password_changed flag only, no-op omitted | Forced create/update audit failure rolls back users; password omitted |
| Asset create/update/delete | Public before/after snapshots; deletion retains history | Forced audit failure rolls back all three paths; no-op timestamps unchanged |
| Work-order create/metadata/status | Locked state and full public snapshots | Forced audit failure rolls back each mutation; terminal/ownership failures have no successful writes |
| OPEN assignment | Assignment and both audit events share one transaction | Failure of second audit rolls back assignment and first audit |
| Reassignment | Same locked mutation runner with one assignment audit | Valid reassignment and same-target conflict checked |

All reviewed business write services use database.InTx and propagate audit errors before commit. There is no automatic retry on uncertain commit results. Security denials go to sanitized operational logs; failed/no-op writes do not create mutation audits. The read endpoint introduces no business write path.

## Local use and verification

Use existing M2 database and M3 authentication setup, then `make run`. Login as administrator or manager and GET /api/v1/audit-logs with the Bearer token. Example filter: `/api/v1/audit-logs?entity_type=asset&action=ASSET_DELETED&page=1&limit=20`. No new environment variable, dependency or migration is needed.

M7 tests cover role access and live permission revocation, inactive accounts, all filters and invalid inputs, tie ordering, empty pages, bootstrap nulls, actual user password-change/asset deletion/work-order assignment snapshots, request correlation, unknown secret fields and malformed snapshot rejection. The full prior suite verifies transaction rollback and runtime audit permissions.

Evidence pending. No public deployment is claimed. M8 testing hardening has not started.
