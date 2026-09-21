# Milestone 6 — work orders

Implementation prepared; verification pending.

## Delivered API

| Method / path | Permission | Behavior |
|---|---|---|
| POST /api/v1/work-orders | work_order:create | OPEN order; server-owned number, creator and timestamps |
| GET /api/v1/work-orders | work_order:read | Paginated, scoped, filtered list |
| GET /api/v1/work-orders/{id} | work_order:read | Scoped detail |
| PATCH /api/v1/work-orders/{id} | work_order:update | Nonterminal title/description/priority only |
| PATCH /api/v1/work-orders/{id}/assign | work_order:assign | Active TECHNICIAN only |
| PATCH /api/v1/work-orders/{id}/status | Target-dependent status:update or cancel | Locked lifecycle transition |

No delete, reopen, asset-change, unassignment or arbitrary status patch is provided. Input validation uses the existing strict JSON decoder, UUID format, pagination bounds and unknown/repeated query rejection. Description can be cleared with an empty string. Optional priority defaults to MEDIUM only when omitted. Creation requires an ACTIVE or MAINTENANCE asset. API responses include explicit null assigned_to/completed_at where appropriate.

## Lifecycle and scope

OPEN assignment becomes ASSIGNED and atomically records WORK_ORDER_ASSIGNED plus WORK_ORDER_STATUS_CHANGED. Reassignment to a different technician is allowed only in ASSIGNED and emits one assignment audit. Same-target assignment is ALREADY_ASSIGNED. ASSIGNED → IN_PROGRESS → COMPLETED is the execution path. ADMIN/MANAGER can cancel any nonterminal order, preserving assignment history; TECHNICIAN cannot cancel. Completion time comes from PostgreSQL. All writes to terminal orders fail. Repeated same-state requests are conflicts, not successful replays.

Technicians see and progress only their assigned orders. List and count intersect that scope with status/priority/asset_id/assigned_to filters in one REPEATABLE READ snapshot; a different assignee filter is forbidden. Detail and mutation ownership checks precede lifecycle disclosure. Unknown resource IDs return 404 after coarse permission checking; existing out-of-scope IDs return 403. Current database permissions are required even for ADMIN.

## Numbering, transactions and lock order

Insert obtains one global sequence nextval and database UTC year. Numbers have at least six digits and expand beyond 999999 without truncation. The sequence is never reset yearly, and rollback gaps are expected. Clients cannot set number/status/creator/assignment/completion fields during creation. Request IDs remain correlation IDs, not idempotency keys; ambiguous commits are not retried.

Mutations lock the acting user FOR SHARE and recheck active status/permission. Assignment prelocks actor and target user in ascending UUID order before locking the work-order row FOR UPDATE; target existence/role/activity is validated after the order's ownership/lifecycle checks. This refines the earlier general locking description to keep user locks consistently ahead of work-order locks. It does not acquire the user-administration advisory lock. Deactivation either waits for assignment and detects its active order, or commits first and assignment rejects the inactive target. Creation locks its asset FOR SHARE; asset retirement/deletion uses FOR UPDATE, preserving M5's guard. No operation locks an asset after a work order.

Every mutation validates current locked state. Two concurrent completions result in one success and one terminal conflict. Concurrent metadata writes preserve omitted fields. Identical metadata updates return the current row without timestamp/audit changes. Business rows and all corresponding full public before/after audit snapshots commit together; a failure of even the second OPEN-assignment audit rolls back both events and assignment.

## Verification and local use

No new migration, environment variable or dependency is required. Use M2 database setup and M3 authentication setup; create an asset, POST an order as ADMIN/MANAGER, assign an active technician, then progress it using the technician's token. Audit read endpoints remain M7.

Required checks include all prior milestone regressions; lifecycle matrix, server-owned field rejection, asset availability, assignee validity, role/ownership checks, scoped filters/count, reassignment and cancellation, terminal guards, metadata no-ops, audit rollback across all write paths, concurrent completion, concurrent sequence allocation above six digits, and assignment waiting for deactivation.

Evidence pending. No public deployment is claimed. M7 has not started.
