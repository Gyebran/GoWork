# Milestone 5 — assets

Completed and verified on 2026-09-21.

## Delivered behavior

GET /api/v1/assets, GET /api/v1/assets/{id}, POST /api/v1/assets, PATCH /api/v1/assets/{id}, and DELETE /api/v1/assets/{id} are registered behind authentication and explicit database permissions. ADMIN/MANAGER can create and update; only ADMIN can delete. Technicians can read only assets connected to their assigned work orders, including terminal orders. The SQL uses EXISTS, so multiple matching work orders never duplicate an asset or inflate the total. Unknown assets return 404; existing out-of-scope assets return 403 after coarse permission checking.

Lists combine status, exact case-sensitive category, and literal case-insensitive search across name/code/location. Percent, underscore and backslash are escaped before parameterized ILIKE. Pagination defaults to page 1/limit 20, is bounded, and sorts by created_at DESC,id DESC. Count and rows use the same scoped predicate and one REPEATABLE READ transaction. Empty pages return [] with the real total. Invalid/repeated/unknown query parameters return 422.

Asset codes are validated, unique and immutable. Names/category/location are trimmed and bounded; status defaults to ACTIVE only when omitted on creation. Strict JSON validation preserves the shared 1 MiB limit and 400/413/415/422 conventions. Identical metadata patches do not change timestamps or create audit rows. Create returns 201 and Location; delete returns 204 with no body.

## Guards, locks and auditing

Mutations lock the acting user FOR SHARE and recheck active account and permission inside the transaction, then lock an existing target asset FOR UPDATE. Asset status can otherwise change freely; entering INACTIVE or RETIRED is blocked if an OPEN/ASSIGNED/IN_PROGRESS work order exists. Deletion is blocked by any referencing work order, including terminal history. Foreign keys additionally protect against bypasses of the application guard.

M6 order creation must lock the asset FOR SHARE before checking availability and inserting. The M5 integration test uses this protocol in a controlled SQL fixture and observes the waiting row lock before committing a competing order; retirement then sees that order and fails. No work-order HTTP features are introduced here. Future mutations should acquire any acting-user lock before resource locks and preserve the existing asset-before-work-order lock ordering.

ASSET_CREATED, ASSET_UPDATED and ASSET_DELETED audit rows are written in the same transaction as the asset mutation. They contain complete public before/after snapshots with request ID. Failed writes and no-ops add no audit. Deletion preserves its audit because entity_id has no asset foreign key. Known uniqueness/FK conflicts return stable domain errors; ambiguous commits are not automatically retried. No schema migration or new environment variable is required.

## Verification and local use

Use M2 database setup and M3 authentication setup, then `make run`. Login as a bootstrap administrator or explicit local demo manager and use the returned Bearer token. Local demo users do not automatically create assets or work orders; an unassigned technician sees an empty list.

Required checks: formatting, vet, race tests, all binaries, reproducible sqlc, and actual PostgreSQL HTTP tests for permissions, immutable fields, search escaping, pagination, deduplicated technician scope, terminal history, no-op timestamps/audits, status/delete guards, audit rollback on all three write paths, preserved delete audit, and concurrent retirement/order creation.

Local `make check` passed with Go 1.27.1, and the integration suite compiled with its test-database guard passing. [GitHub Actions run 35588444121](https://github.com/Gyebran/GoWork/actions/runs/35588444121) passed `make check`, reproducible sqlc generation, and the complete PostgreSQL 16.10 integration suite on implementation commit `15df3cd1415794f174e9a331b4d8e443cc76eb3c`. This includes M2–M4 regression checks and the M5 scenarios above. The completion commit changes documentation only.

No public deployment is claimed. M6 work-order features have not started.
