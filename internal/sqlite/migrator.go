package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"
)

//go:embed migrations/001_initial_schema.sql
var migration001 string

// timestampFormat is the fixed nanosecond-precision UTC format used for all
// timestamps stored in SQLite TEXT columns.
const timestampFormat = "2006-01-02T15:04:05.000000000Z"

// Migration represents a single versioned schema migration.
type Migration struct {
	Version int
	SQL     string
}

// productionMigrations is the canonical ordered list of migrations shipped with
// the library. The Migrator validates that this list is contiguous before applying.
var productionMigrations = []Migration{
	{Version: 1, SQL: migration001},
}

// Migrator applies versioned SQL migrations to a SQLite database.
type Migrator struct {
	db         *sql.DB
	migrations []Migration
}

// NewMigrator returns a Migrator that uses the production migration list.
func NewMigrator(db *sql.DB) *Migrator {
	return &Migrator{db: db, migrations: productionMigrations}
}

// NewMigratorWithSQL returns a Migrator using a custom migration list.
// Used in tests to inject failing or non-standard migrations.
func NewMigratorWithSQL(db *sql.DB, migrations []Migration) *Migrator {
	return &Migrator{db: db, migrations: migrations}
}

// Migrate applies any pending migrations in ascending version order.
// It validates that the migration list is contiguous and sorted before
// executing anything. Each migration runs in its own transaction and is
// rolled back on failure.
func (m *Migrator) Migrate(ctx context.Context) error {
	if err := validateMigrations(m.migrations); err != nil {
		return fmt.Errorf("migrator: %w", err)
	}

	// Ensure the schema_migrations table exists before we query it.
	// This bootstrap DDL is always safe to run (IF NOT EXISTS).
	const ensureTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER NOT NULL PRIMARY KEY,
		applied_at TEXT    NOT NULL
	)`
	if _, err := m.db.ExecContext(ctx, ensureTable); err != nil {
		return fmt.Errorf("migrator: ensure schema_migrations: %w", err)
	}

	// Collect already-applied versions.
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("migrator: query applied versions: %w", err)
	}

	for _, mig := range m.migrations {
		if applied[mig.Version] {
			continue // already applied; skip
		}
		if err := m.applyOne(ctx, mig); err != nil {
			return fmt.Errorf("migrator: apply version %d: %w", mig.Version, err)
		}
	}
	return nil
}

// applyOne executes a single migration inside a transaction. On failure the
// transaction is rolled back and an error is returned.
func (m *Migrator) applyOne(ctx context.Context, mig Migration) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	if _, err := tx.ExecContext(ctx, mig.SQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("exec: %w", err)
	}

	appliedAt := time.Now().UTC().Format(timestampFormat)
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		mig.Version, appliedAt,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// appliedVersions returns the set of migration versions already recorded in
// schema_migrations.
func (m *Migrator) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := m.db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// validateMigrations checks that the migration list is non-empty, sorted in
// ascending order, and contiguous (no version gaps).
func validateMigrations(migrations []Migration) error {
	if len(migrations) == 0 {
		return fmt.Errorf("migration list is empty")
	}
	for i, mig := range migrations {
		expected := i + 1
		if mig.Version != expected {
			return fmt.Errorf("migration list is not contiguous: index %d has version %d, expected %d",
				i, mig.Version, expected)
		}
	}
	return nil
}
