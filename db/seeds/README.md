# Seed data

Migration 009 installs the fixed role/permission catalogue. It creates no accounts.

Milestone 3 provides `make seed-demo`: explicit development/test-only Go command with real bcrypt hashes, three demo roles, transactional audits and no overwriting of existing accounts. See docs/milestone-3.md for the local-only credentials and exact semantics.

For a non-demo administrator use `make bootstrap`. Neither command is invoked by API startup or migrations.
