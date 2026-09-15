# Seeds

Migration 009 reproducibly installs the fixed role/permission catalogue (3 roles, 14 permissions, 28 mappings). It creates no users or passwords.

Demo users and business fixtures arrive in M3 with real Go bcrypt hashing and an explicit development-only command. Never run demo fixtures automatically at API startup or in production. M2 integration fixtures run only in an explicitly designated disposable `_test` database and roll back their data.
