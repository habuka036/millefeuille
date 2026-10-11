package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	millefeuille "github.com/habuka036/millefeuille"
	"github.com/habuka036/millefeuille/internal/sqlite"
)

// newMigratedRepo returns a repository backed by a freshly migrated test DB.
func newMigratedRepo(t *testing.T) (*sql.DB, sqlite.ObservationRepository) {
	t.Helper()
	db := openTestDB(t)
	if err := sqlite.NewMigrator(db).Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db, sqlite.NewRepository(db)
}

func sampleObs(eventID string, receivedAt time.Time) millefeuille.Observation {
	return millefeuille.Observation{
		EventID:       eventID,
		SchemaVersion: "0.1",
		SourceID:      "src-001",
		DeviceID:      "dev-001",
		EventType:     "device.status",
		ObservedAt:    time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC),
		ReceivedAt:    receivedAt,
		Payload:       json.RawMessage(`{"k":"v"}`),
		Visibility:    "private",
		Provenance:    millefeuille.Provenance{Origin: "device", TrustLevel: "unverified"},
	}
}

// TestInsert_NewRow verifies a new observation is inserted and a Receipt returned.
func TestInsert_NewRow(t *testing.T) {
	ctx := context.Background()
	_, repo := newMigratedRepo(t)

	obs := sampleObs("evt-1", time.Now().UTC())
	rcpt, err := repo.Insert(ctx, obs, "hash-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if rcpt.EventID != "evt-1" || rcpt.ContentHash != "hash-1" {
		t.Errorf("unexpected receipt: %+v", rcpt)
	}
}

// TestInsert_IdempotentSameHash verifies a duplicate event_id with the same hash
// returns the original Receipt without error.
func TestInsert_IdempotentSameHash(t *testing.T) {
	ctx := context.Background()
	_, repo := newMigratedRepo(t)

	rt1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	obs := sampleObs("evt-1", rt1)
	first, err := repo.Insert(ctx, obs, "hash-1")
	if err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	// Re-submit with a different received_at but the same hash.
	obs2 := sampleObs("evt-1", rt1.Add(time.Hour))
	second, err := repo.Insert(ctx, obs2, "hash-1")
	if err != nil {
		t.Fatalf("second Insert (idempotent): %v", err)
	}

	// The returned received_at must be the ORIGINAL one.
	if !second.ReceivedAt.Equal(first.ReceivedAt) {
		t.Errorf("expected original received_at %v, got %v", first.ReceivedAt, second.ReceivedAt)
	}
}

// TestInsert_ConflictDifferentHash verifies a duplicate event_id with a different
// hash returns ErrConflict and leaves the stored row unchanged.
func TestInsert_ConflictDifferentHash(t *testing.T) {
	ctx := context.Background()
	db, repo := newMigratedRepo(t)

	obs := sampleObs("evt-1", time.Now().UTC())
	if _, err := repo.Insert(ctx, obs, "hash-1"); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	_, err := repo.Insert(ctx, obs, "hash-DIFFERENT")
	if !errors.Is(err, millefeuille.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	// Stored content_hash must remain the original.
	var stored string
	if err := db.QueryRowContext(ctx,
		"SELECT content_hash FROM observations WHERE event_id = ?", "evt-1",
	).Scan(&stored); err != nil {
		t.Fatalf("read stored hash: %v", err)
	}
	if stored != "hash-1" {
		t.Errorf("stored hash changed: got %q, want %q", stored, "hash-1")
	}

	// Only one row must exist.
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM observations").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

// TestFindByEventID_Found verifies retrieval of a stored observation.
func TestFindByEventID_Found(t *testing.T) {
	ctx := context.Background()
	_, repo := newMigratedRepo(t)

	obs := sampleObs("evt-1", time.Now().UTC())
	if _, err := repo.Insert(ctx, obs, "hash-1"); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := repo.FindByEventID(ctx, "evt-1")
	if err != nil {
		t.Fatalf("FindByEventID: %v", err)
	}
	if got.EventID != "evt-1" || got.SourceID != "src-001" {
		t.Errorf("unexpected observation: %+v", got)
	}
	if !got.ObservedAt.Equal(obs.ObservedAt) {
		t.Errorf("observed_at mismatch: got %v want %v", got.ObservedAt, obs.ObservedAt)
	}
}

// TestFindByEventID_NotFound verifies ErrNotFound for an unknown event_id.
func TestFindByEventID_NotFound(t *testing.T) {
	ctx := context.Background()
	_, repo := newMigratedRepo(t)

	_, err := repo.FindByEventID(ctx, "missing")
	if !errors.Is(err, millefeuille.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestQuery_NegativeLimit verifies ErrInvalidFilter for a negative limit.
func TestQuery_NegativeLimit(t *testing.T) {
	ctx := context.Background()
	_, repo := newMigratedRepo(t)

	_, err := repo.Query(ctx, millefeuille.Filter{Limit: -1})
	if !errors.Is(err, millefeuille.ErrInvalidFilter) {
		t.Fatalf("expected ErrInvalidFilter, got %v", err)
	}
}

// TestQuery_OrderingAndFilter verifies ordering and basic filtering.
func TestQuery_OrderingAndFilter(t *testing.T) {
	ctx := context.Background()
	_, repo := newMigratedRepo(t)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Insert three observations with increasing received_at.
	for i, id := range []string{"evt-a", "evt-b", "evt-c"} {
		obs := sampleObs(id, base.Add(time.Duration(i)*time.Hour))
		if _, err := repo.Insert(ctx, obs, "hash-"+id); err != nil {
			t.Fatalf("Insert %s: %v", id, err)
		}
	}

	page, err := repo.Query(ctx, millefeuille.Filter{SourceID: "src-001"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(page))
	}
	// Ordered by received_at DESC: evt-c, evt-b, evt-a.
	if page[0].EventID != "evt-c" || page[2].EventID != "evt-a" {
		t.Errorf("unexpected order: %s, %s, %s", page[0].EventID, page[1].EventID, page[2].EventID)
	}
}

// TestInsert_ContextCancelled verifies that a cancelled context aborts the insert.
func TestInsert_ContextCancelled(t *testing.T) {
	_, repo := newMigratedRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	obs := sampleObs("evt-1", time.Now().UTC())
	_, err := repo.Insert(ctx, obs, "hash-1")
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}
