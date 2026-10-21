---
paths:
  - "**/*_test.go"
  - "shared/testing/**"
---

# Testing

- `testify`: `require` when the test cannot continue, `assert` for the checks.
- **Which test**: a change in the persistence layer (SQL, migrations, repositories) gets an integration test
  against a real database. Everything else gets unit tests with fakes - small structs implementing the narrow
  interface the code depends on (map-based fakes are common).
- **Integration tests** start with `//go:build integration`, usually live in `package <pkg>_test` next to the
  repositories, and build the real services on a container database:
  ```go
  db := testutil.NewTestDatabase(t)   // shared/testing, testcontainers - needs Docker
  db.RunMigrationsForService()        // this service's 000001 migration
  txManager := db.CreateTransactionManager()
  ```
  Run them with `go test -tags integration ./<service>/...`; check they compile with
  `go vet -tags integration ./<service>/...`. gopls ignores tagged files, so editor diagnostics there mean nothing.
- `metrics.New()` registers globally (promauto) and panics the second time: share one instance per test binary
  (the packages have a `testMetrics()` helper with `sync.Once`). Unit tests build `&metrics.Metrics{...}` with
  unregistered collectors instead.
- Outbox purge behavior is checked per schema with `testutil.VerifyOutboxPurge` / `VerifyDeadLetterPurge`.
- Redis is faked with `miniredis`.
- Name tests after the behavior: `TestAccountErasure_ARedeliveredEventIsANoOp`, not `TestErase2`.
