package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/habuka036/millefeuille/internal/sqlite"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "test.db") +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(FULL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// TestMigrator_FreshDB verifies that migrations are applied to an empty database
// and the schema_migrations table contains version 1.
func TestMigrator_FreshDB(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	m := sqlite.NewMigrator(db)

	if err := m.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// schema_migrations must contain version 1.
	var version int
	if err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	// observations table must exist.
	var name string
	err := db.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='observations'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("observations table not found: %v", err)
	}
}

// TestMigrator_Idempotent verifies that calling Migrate twice does not duplicate
// version rows or fail.
func TestMigrator_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	m := sqlite.NewMigrator(db)

	if err := m.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := m.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 migration row, got %d", count)
	}
}

// TestMigrator_FailingMigration verifies that a failing migration is rolled back
// and the schema is left unchanged.
func TestMigrator_FailingMigration(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	// Inject a migrator with a bad second migration.
	m := sqlite.NewMigratorWithSQL(db, []sqlite.Migration{
		{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER NOT NULL PRIMARY KEY, applied_at TEXT NOT NULL);`},
		{Version: 2, SQL: `THIS IS NOT VALID SQL AND WILL FAIL`},
	})

	// First migration should succeed.
	if err := m.Migrate(ctx); err == nil {
		t.Fatal("expected error from failing migration, got nil")
	}

	// schema_migrations must not contain version 2.
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version=2").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 0 {
		t.Errorf("expected version 2 to be absent after rollback, found %d row(s)", count)
	}
}

// TestMigrator_NonContiguousVersions verifies that a migration list with a gap
// is rejected at startup.
func TestMigrator_NonContiguousVersions(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	m := sqlite.NewMigratorWithSQL(db, []sqlite.Migration{
		{Version: 1, SQL: `SELECT 1`},
		{Version: 3, SQL: `SELECT 1`}, // gap: version 2 missing
	})

	if err := m.Migrate(ctx); err == nil {
		t.Error("expected error for non-contiguous migration versions, got nil")
	}
}
