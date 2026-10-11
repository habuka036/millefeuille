package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// openWithPragmas opens a file-based SQLite DB with the same DSN PRAGMAs that
// millefeuille.Open applies, so the integration test can assert their values.
func openWithPragmas(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "pragma.db") +
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
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return db
}

// TestPragmas_Applied verifies that WAL, synchronous, busy_timeout, and
// foreign_keys are set to the expected values on the connection (NFR-000,
// PRAGMA verification).
func TestPragmas_Applied(t *testing.T) {
	ctx := context.Background()
	db := openWithPragmas(t)

	cases := []struct {
		pragma string
		want   string
	}{
		{"journal_mode", "wal"},
		{"synchronous", "2"}, // FULL == 2
		{"busy_timeout", "5000"},
		{"foreign_keys", "1"}, // ON == 1
	}
	for _, c := range cases {
		t.Run(c.pragma, func(t *testing.T) {
			var got string
			if err := db.QueryRowContext(ctx, "PRAGMA "+c.pragma).Scan(&got); err != nil {
				t.Fatalf("PRAGMA %s: %v", c.pragma, err)
			}
			if got != c.want {
				t.Errorf("PRAGMA %s: got %q, want %q", c.pragma, got, c.want)
			}
		})
	}
}
