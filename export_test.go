package millefeuille

import (
	"context"
	"database/sql"
	"fmt"
)

// This file exposes internal hooks to the external test package
// (millefeuille_test) for white-box integration testing. It is only compiled
// during tests (the _test.go suffix) and is not part of the public API.

// SetFailingMigrationForTest replaces the migration runner with one that fails
// after confirming the *sql.DB is reachable. It returns a restore function and a
// pointer to a bool that reports whether the injected failure saw an open,
// usable connection (used to assert the failure occurred post-open).
//
// The test uses this to verify that Open closes the DB connection when
// migrations fail, so no connection leaks.
func SetFailingMigrationForTest() (restore func(), sawOpenConn *bool) {
	prev := runMigrations
	flag := new(bool)
	runMigrations = func(ctx context.Context, db *sql.DB) error {
		// Confirm the connection is open and usable before failing.
		if err := db.PingContext(ctx); err == nil {
			*flag = true
		}
		return fmt.Errorf("injected migration failure for test")
	}
	return func() { runMigrations = prev }, flag
}
