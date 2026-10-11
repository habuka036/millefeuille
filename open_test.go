package millefeuille_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	millefeuille "github.com/habuka036/millefeuille"
)

// TestOpen_RejectsInMemory verifies that in-memory database paths are rejected.
func TestOpen_RejectsInMemory(t *testing.T) {
	ctx := context.Background()
	for _, path := range []string{":memory:", "file::memory:", "file:test.db?mode=memory"} {
		if _, err := millefeuille.Open(ctx, path); err == nil {
			t.Errorf("Open(%q) expected error for in-memory path, got nil", path)
		}
	}
}

// TestOpen_FileBasedSucceeds verifies Open works on a file-based path and the
// returned Store can be closed.
func TestOpen_FileBasedSucceeds(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := millefeuille.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestOpen_WithClock verifies the WithClock option is accepted.
func TestOpen_WithClock(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st, err := millefeuille.Open(ctx, path, millefeuille.WithClock(func() time.Time { return fixed }))
	if err != nil {
		t.Fatalf("Open with clock: %v", err)
	}
	defer st.Close()
}

// TestOpen_MigrationFailureNoLeak verifies that when migrations fail during Open,
// the database connection is closed (no leak). It confirms this by:
//  1. injecting a failing migration that first checks the connection is open,
//  2. asserting Open returns an error,
//  3. asserting the same database file can be opened successfully afterwards
//     (which would fail or block if the previous connection were leaked under
//     SetMaxOpenConns(1) with the file still locked).
func TestOpen_MigrationFailureNoLeak(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "leak.db")

	restore, sawOpenConn := millefeuille.SetFailingMigrationForTest()
	_, err := millefeuille.Open(ctx, path)
	restore()

	if err == nil {
		t.Fatal("expected Open to fail due to injected migration failure")
	}
	if !*sawOpenConn {
		t.Error("injected migration did not observe an open connection; failure may be pre-open")
	}

	// If the first Open leaked its connection, this second Open (now with the
	// real migrator) could not fully initialise the database. Success here
	// demonstrates the connection was released.
	st, err := millefeuille.Open(ctx, path)
	if err != nil {
		t.Fatalf("re-open after failed Open: %v (possible connection leak)", err)
	}
	if err := st.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestOpen_NoPartialRowOnCancel verifies NFR-001: a context cancelled during the
// append transaction leaves no partial row.
func TestOpen_NoPartialRowOnCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "txn.db")
	st, err := millefeuille.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the append begins

	in := millefeuille.InputObservation{
		EventID:       "evt-cancel",
		SchemaVersion: "0.1",
		SourceID:      "src-1",
		EventType:     "device.status",
		ObservedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Payload:       []byte(`{"k":"v"}`),
		Provenance:    millefeuille.Provenance{Origin: "device"},
	}
	if _, err := st.AppendObservation(ctx, in); err == nil {
		t.Fatal("expected error from cancelled append, got nil")
	}

	// No row should have been persisted.
	_, getErr := st.GetObservation(context.Background(), "evt-cancel")
	if !errors.Is(getErr, millefeuille.ErrNotFound) {
		t.Errorf("expected ErrNotFound after cancelled append, got %v", getErr)
	}
}
