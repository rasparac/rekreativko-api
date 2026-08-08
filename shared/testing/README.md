# Integration Testing Framework

This package provides common testing utilities for integration tests across all services.

## Features

### 🐘 Database Testing (`database.go`)
- **Testcontainers Integration**: Automatically spins up PostgreSQL containers for isolated tests
- **Transaction Support**: Run tests in transactions that automatically rollback
- **Migration Support**: Apply migrations from actual migration files (single source of truth)
- **Connection Pooling**: Proper connection pool management
- **Cleanup**: Automatic container cleanup after tests

### 📁 Migration Loading Methods

Migrations are co-located with each service in `internal/<service>/infrastructure/persistence/migrations/`

Three ways to load migrations in tests:

1. **`RunMigrationsForService()`** - ⭐ RECOMMENDED
   - Loads all migrations for the current service
   - Runs all `.up.sql` files in the local `./migrations/` directory
   - Example: `db.RunMigrationsForService()` loads all migrations in `./migrations/`
   - Use when: Testing a service (most common case)

2. **`RunMigrationFile(filename)`**
   - Loads a specific migration file from the local `./migrations/` directory
   - Example: `db.RunMigrationFile("000001_activity_schema.up.sql")`
   - Use when: You only need one specific migration

3. **`RunMigrationsMatching(pattern)`**
   - Loads all migrations matching a glob pattern from `./migrations/`
   - Example: `db.RunMigrationsMatching("*.up.sql")`
   - Use when: You need fine-grained control over which migrations to load

### 🔧 Test Helpers (`helpers.go`)
- **Pointer Helpers**: `Ptr[T](v)` for creating pointers
- **UUID Helpers**: `MustUUID()` for parsing UUIDs with automatic test failure
- **Time Helpers**: `MustTime()`, `AssertTimeAlmostEqual()`, `NowUTC()`, `TimeUTC()`
- **Assertions**: Custom assertions for common test scenarios

## Usage

### Basic Integration Test

```go
//go:build integration

package persistence

import (
    "testing"
    testutil "github.com/rasparac/rekreativko-api/internal/shared/testing"
)

func TestMyRepository(t *testing.T) {
    // Setup database with actual migration files (single source of truth)
    db := testutil.NewTestDatabase(t)

    // Option 1: Run all migrations for current service (RECOMMENDED)
    db.RunMigrationsForService()

    // Option 2: Run specific migration file
    // db.RunMigrationFile("000001_activity_schema.up.sql")

    // Option 3: Run migrations matching a pattern
    // db.RunMigrationsMatching("*.up.sql")

    // Create repository
    txManager := db.CreateTransactionManager()
    repo := NewMyRepository(txManager, testutil.CreateLogger())

    // Your test logic here
}
```

### With Transaction Rollback

```go
func TestMyRepository_WithRollback(t *testing.T) {
    db := testutil.NewTestDatabase(t)

    db.WithTransaction(func(ctx context.Context, tx pgx.Tx) {
        // All database operations here will be rolled back
        // Perfect for isolated test cases
    })
}
```

### Test Data Builders

```go
// Create test data with fluent API
template := NewSessionTemplateBuilder().
    WithTitle("Test Template").
    WithCapacity(20).
    WithWeeklyRecurrence(time.Monday, 18, 30).
    Build()
```

## Running Integration Tests

Integration tests are tagged with `//go:build integration` to separate them from unit tests.

### Run Only Unit Tests (default)
```bash
go test ./...
```

### Run Only Integration Tests
```bash
go test -tags=integration ./...
```

### Run All Tests
```bash
go test -tags=integration ./... && go test ./...
```

### Run Integration Tests for Specific Package
```bash
go test -tags=integration ./internal/activity/infrastructure/persistence/...
```

### Run with Verbose Output
```bash
go test -tags=integration -v ./internal/activity/infrastructure/persistence/...
```

### Run Specific Test
```bash
go test -tags=integration -run TestSessionTemplateRepository_CreateAndGet ./internal/activity/infrastructure/persistence/...
```

## Requirements

### Dependencies
```bash
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
go get github.com/stretchr/testify
```

### Docker
Integration tests require Docker to be running (for testcontainers).

```bash
# Check Docker is running
docker info
```

## Common Patterns

### Setup and Teardown

```go
func setupTestDB(t *testing.T) *testutil.TestDatabase {
    t.Helper()

    db := testutil.NewTestDatabase(t)
    db.RunSQL(migrationSQL)

    return db
}

func TestMyFeature(t *testing.T) {
    db := setupTestDB(t)
    // db will be automatically cleaned up after test
}
```

### Creating Test Data

```go
func createTestActivityGroup(t *testing.T, db *testutil.TestDatabase) uuid.UUID {
    t.Helper()

    groupID := uuid.New()
    db.RunSQL(
        `INSERT INTO activity.activity_group (id, name) VALUES ($1, $2)`,
        groupID, "Test Group",
    )
    return groupID
}
```

### Table Cleanup Between Tests

```go
func TestMultipleScenarios(t *testing.T) {
    db := setupTestDB(t)

    t.Run("scenario 1", func(t *testing.T) {
        // test logic

        // Cleanup for next test
        db.TruncateTables("activity.session_template", "activity.activity_group")
    })

    t.Run("scenario 2", func(t *testing.T) {
        // starts with clean tables
    })
}
```

## Best Practices

### ✅ DO

1. **Use actual migration files** as single source of truth:
   ```go
   db.RunMigrationsForService()  // ⭐ RECOMMENDED - loads ./migrations/*.up.sql
   // OR: db.RunMigrationFile("000001_activity_schema.up.sql")
   // NOT: db.RunSQL(hardcodedSQL)
   ```

2. **Use build tags** for integration tests:
   ```go
   //go:build integration
   ```

3. **Use builders** for test data:
   ```go
   template := NewSessionTemplateBuilder().WithTitle("Test").Build()
   ```

4. **Use table-driven tests**:
   ```go
   tests := []struct {
       name string
       // ...
   }{
       {name: "case 1", /* ... */},
       {name: "case 2", /* ... */},
   }
   ```

5. **Use subtests** for organization:
   ```go
   t.Run("creates successfully", func(t *testing.T) { /* ... */ })
   ```

6. **Use helpers** with `t.Helper()`:
   ```go
   func setupData(t *testing.T) {
       t.Helper()
       // ...
   }
   ```

### ❌ DON'T

1. **Don't duplicate SQL** - Use `RunMigrationFile()` instead of hardcoding SQL in tests
2. **Don't share database state** between tests without cleanup
3. **Don't use hardcoded times** - use `NowUTC()` or `TimeUTC()`
4. **Don't ignore errors** in setup code - use `require.NoError()`
5. **Don't forget cleanup** - testcontainers handles this automatically

## Continuous Integration

### GitHub Actions Example

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  integration:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Run integration tests
        run: go test -tags=integration -v ./...
```

## Troubleshooting

### Docker Permission Issues
```bash
# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

### Port Conflicts
Testcontainers automatically assigns random ports, so conflicts are rare. If you encounter issues:
```bash
# Kill all testcontainers
docker ps | grep testcontainers | awk '{print $1}' | xargs docker kill
```

### Slow Tests
Integration tests are slower than unit tests. Optimize by:
- Using `WithTransaction()` for rollback instead of truncating
- Running setup SQL only once per test suite
- Using parallel tests with `t.Parallel()` when safe

## Examples

See real-world examples:
- `/internal/activity/infrastructure/persistence/session_template_repo_integration_test.go`
- `/internal/activity/infrastructure/persistence/fixtures.go` (builders)
