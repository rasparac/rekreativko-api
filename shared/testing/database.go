package testing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/testcontainers/testcontainers-go"
	testcontainersPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	postgresImage    = "postgres:16-alpine"
	defaultDBName    = "testdb"
	defaultUser      = "testuser"
	defaultPassword  = "testpass"
	startupTimeout   = 60 * time.Second
	shutdownTimeout  = 10 * time.Second
)

// TestDatabase represents a test database instance
type TestDatabase struct {
	Container *testcontainersPostgres.PostgresContainer
	Pool      *pgxpool.Pool
	DSN       string
	t         *testing.T
}

// NewTestDatabase creates a new PostgreSQL test database using testcontainers
func NewTestDatabase(t *testing.T, migrations ...string) *TestDatabase {
	t.Helper()

	ctx := context.Background()

	// Create PostgreSQL container
	container, err := testcontainersPostgres.Run(ctx,
		postgresImage,
		testcontainersPostgres.WithDatabase(defaultDBName),
		testcontainersPostgres.WithUsername(defaultUser),
		testcontainersPostgres.WithPassword(defaultPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(startupTimeout),
		),
	)
	if err != nil {
		t.Fatalf("failed to start PostgreSQL container: %v", err)
	}

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("WARNING: failed to terminate container after connection error: %v", termErr)
		}
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Create connection pool
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("WARNING: failed to terminate container after parse error: %v", termErr)
		}
		t.Fatalf("failed to parse pool config: %v", err)
	}

	// Configure pool for testing
	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("WARNING: failed to terminate container after pool creation error: %v", termErr)
		}
		t.Fatalf("failed to create connection pool: %v", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("WARNING: failed to terminate container after ping error: %v", termErr)
		}
		t.Fatalf("failed to ping database: %v", err)
	}

	db := &TestDatabase{
		Container: container,
		Pool:      pool,
		DSN:       dsn,
		t:         t,
	}

	// Run migrations if provided
	if len(migrations) > 0 {
		db.RunMigrations(migrations...)
	}

	// Register cleanup
	t.Cleanup(func() {
		db.Cleanup()
	})

	return db
}

// RunMigrations executes SQL migration files
func (db *TestDatabase) RunMigrations(migrationFiles ...string) {
	db.t.Helper()

	ctx := context.Background()

	for _, file := range migrationFiles {
		db.t.Logf("Running migration: %s", file)

		// Read migration file
		// In real implementation, you'd read from file
		// For now, we'll expect SQL content to be passed
		_, err := db.Pool.Exec(ctx, file)
		if err != nil {
			db.t.Fatalf("failed to run migration %s: %v", file, err)
		}
	}
}

// RunSQL executes raw SQL (useful for test setup)
func (db *TestDatabase) RunSQL(sql string, args ...any) {
	db.t.Helper()

	ctx := context.Background()
	_, err := db.Pool.Exec(ctx, sql, args...)
	if err != nil {
		db.t.Fatalf("failed to execute SQL: %v\nSQL: %s", err, sql)
	}
}

// TruncateTables truncates specified tables (useful for cleanup between tests)
func (db *TestDatabase) TruncateTables(tables ...string) {
	db.t.Helper()

	ctx := context.Background()

	for _, table := range tables {
		sql := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
		_, err := db.Pool.Exec(ctx, sql)
		if err != nil {
			db.t.Fatalf("failed to truncate table %s: %v", table, err)
		}
	}
}

// RunMigrationFile loads and executes a migration file from the service's migrations directory
// This ensures tests use the actual migration files as the single source of truth
// Example: db.RunMigrationFile("000001_activity_schema.up.sql")
func (db *TestDatabase) RunMigrationFile(filename string) {
	db.t.Helper()

	// Migrations are now co-located with each service's persistence layer
	// When running tests, the working directory is the package directory
	// Migrations are in ./migrations/ subdirectory
	migrationPath := filepath.Join("migrations", filename)

	content, err := os.ReadFile(migrationPath)
	if err != nil {
		db.t.Fatalf("failed to read migration file %s: %v", migrationPath, err)
	}

	db.RunSQL(string(content))
}

// RunMigrationsMatching loads and executes all migration files matching a glob pattern
// Files are executed in alphabetical order (which matches migration numbering)
// Example: db.RunMigrationsMatching("*.up.sql") - runs all up migrations in service's migrations directory
func (db *TestDatabase) RunMigrationsMatching(pattern string) {
	db.t.Helper()

	migrationsDir := "migrations"
	fullPattern := filepath.Join(migrationsDir, pattern)

	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		db.t.Fatalf("failed to glob migration pattern %s: %v", pattern, err)
	}

	if len(matches) == 0 {
		db.t.Fatalf("no migration files found matching pattern: %s in %s", pattern, migrationsDir)
	}

	// Sort to ensure migrations run in order
	sort.Strings(matches)

	for _, path := range matches {
		content, err := os.ReadFile(path)
		if err != nil {
			db.t.Fatalf("failed to read migration file %s: %v", path, err)
		}

		db.RunSQL(string(content))
	}
}

// RunMigrationsForService loads and executes all migrations for the current service
// Since migrations are co-located with each service, this runs all .up.sql files in ./migrations/
// Example: db.RunMigrationsForService() - runs all migrations for current service
func (db *TestDatabase) RunMigrationsForService() {
	db.t.Helper()

	// Run all .up.sql migrations in the local migrations directory
	db.RunMigrationsMatching("*.up.sql")
}

// CreateTransactionManager creates a transaction manager for testing
func (db *TestDatabase) CreateTransactionManager() *postgres.TransactionManager {
	return postgres.NewTransactionManager(db.Pool)
}

// WithTransaction runs a test function within a transaction that gets rolled back
// This is useful for isolated test cases
func (db *TestDatabase) WithTransaction(fn func(ctx context.Context, tx pgx.Tx)) {
	db.t.Helper()

	ctx := context.Background()

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		db.t.Fatalf("failed to begin transaction: %v", err)
	}

	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			db.t.Logf("failed to rollback transaction: %v", err)
		}
	}()

	fn(ctx, tx)
}

// Cleanup closes the database connection and terminates the container
func (db *TestDatabase) Cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if db.Pool != nil {
		// Close pool with timeout monitoring
		done := make(chan struct{})
		go func() {
			db.Pool.Close()
			close(done)
		}()

		select {
		case <-done:
			// Pool closed successfully
		case <-ctx.Done():
			db.t.Logf("WARNING: pool close timeout exceeded")
		}
	}

	if db.Container != nil {
		if err := db.Container.Terminate(ctx); err != nil {
			db.t.Logf("failed to terminate container: %v", err)
		}
	}
}

// CreateLogger creates a test logger
func CreateLogger() *logger.Logger {
	return logger.New("debug", "text")
}
