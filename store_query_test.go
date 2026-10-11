package millefeuille_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	millefeuille "github.com/habuka036/millefeuille"
)

// appendWith is a helper that appends an observation with the given event_id and
// source/event metadata, using the store's configured clock for received_at.
func appendWith(t *testing.T, st millefeuille.Store, eventID, sourceID, eventType string) {
	t.Helper()
	in := millefeuille.InputObservation{
		EventID:       eventID,
		SchemaVersion: "0.1",
		SourceID:      sourceID,
		EventType:     eventType,
		ObservedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Payload:       json.RawMessage(`{"k":"v"}`),
		Provenance:    millefeuille.Provenance{Origin: "device"},
	}
	if _, err := st.AppendObservation(context.Background(), in); err != nil {
		t.Fatalf("append %s: %v", eventID, err)
	}
}

// --- REQ-003 AC-1: found ----------------------------------------------------

func TestGet_Found(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	appendWith(t, st, "evt-1", "src-1", "device.status")

	got, err := st.GetObservation(ctx, "evt-1")
	if err != nil {
		t.Fatalf("GetObservation: %v", err)
	}
	if got.EventID != "evt-1" || got.SourceID != "src-1" {
		t.Errorf("unexpected observation: %+v", got)
	}
}

// --- REQ-003 AC-2: not found ------------------------------------------------

func TestGet_NotFound(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	_, err := st.GetObservation(ctx, "missing")
	if !errors.Is(err, millefeuille.ErrNotFound) {
		t.Fatalf("expected ErrNotFound (via errors.Is), got %v", err)
	}
}

// --- REQ-003 AC-3: cancelled context ----------------------------------------

func TestGet_CancelledContext(t *testing.T) {
	st := newStore(t)
	appendWith(t, st, "evt-1", "src-1", "device.status")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := st.GetObservation(ctx, "evt-1"); err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- REQ-004 AC-1: ordering -------------------------------------------------

func TestQuery_OrderingReceivedAtDesc(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := newStore(t, millefeuille.WithClock(advancingClock(start, time.Hour)))

	appendWith(t, st, "evt-a", "src-1", "device.status") // received_at = start+0h
	appendWith(t, st, "evt-b", "src-1", "device.status") // received_at = start+1h
	appendWith(t, st, "evt-c", "src-1", "device.status") // received_at = start+2h

	page, err := st.QueryObservations(ctx, millefeuille.Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(page.Items))
	}
	// received_at DESC → evt-c, evt-b, evt-a.
	want := []string{"evt-c", "evt-b", "evt-a"}
	for i, w := range want {
		if page.Items[i].EventID != w {
			t.Errorf("position %d: got %q, want %q", i, page.Items[i].EventID, w)
		}
	}
}

// --- REQ-004 AC-1 (same received_at → event_id DESC) ------------------------

func TestQuery_SameReceivedAtOrderByEventIDDesc(t *testing.T) {
	ctx := context.Background()
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// All appends share the same received_at via a fixed clock.
	st := newStore(t, millefeuille.WithClock(fixedClock(fixed)))

	appendWith(t, st, "evt-a", "src-1", "device.status")
	appendWith(t, st, "evt-b", "src-1", "device.status")
	appendWith(t, st, "evt-c", "src-1", "device.status")

	page, err := st.QueryObservations(ctx, millefeuille.Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(page.Items))
	}
	// Same received_at → tie broken by event_id DESC: evt-c, evt-b, evt-a.
	want := []string{"evt-c", "evt-b", "evt-a"}
	for i, w := range want {
		if page.Items[i].EventID != w {
			t.Errorf("position %d: got %q, want %q", i, page.Items[i].EventID, w)
		}
	}
}

// --- REQ-004 AC-2: default limit --------------------------------------------

func TestQuery_DefaultLimit(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := newStore(t, millefeuille.WithClock(advancingClock(start, time.Second)))

	// Insert 150 observations; default limit is 100.
	for i := 0; i < 150; i++ {
		appendWith(t, st, eid(i), "src-1", "device.status")
	}

	page, err := st.QueryObservations(ctx, millefeuille.Filter{Limit: 0})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 100 {
		t.Errorf("default limit: expected 100 items, got %d", len(page.Items))
	}
}

// --- REQ-004 AC-3: limit cap ------------------------------------------------

func TestQuery_LimitCap(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := newStore(t, millefeuille.WithClock(advancingClock(start, time.Second)))

	// Insert 1005 observations; max limit is 1000.
	for i := 0; i < 1005; i++ {
		appendWith(t, st, eid(i), "src-1", "device.status")
	}

	page, err := st.QueryObservations(ctx, millefeuille.Filter{Limit: 9999})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 1000 {
		t.Errorf("limit cap: expected 1000 items, got %d", len(page.Items))
	}
}

// --- REQ-004 AC-4/5: exact-match filters ------------------------------------

func TestQuery_FilterBySourceID(t *testing.T) {
	ctx := context.Background()
	st := newStore(t, millefeuille.WithClock(advancingClock(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Second)))

	appendWith(t, st, "evt-1", "src-A", "device.status")
	appendWith(t, st, "evt-2", "src-B", "device.status")

	page, err := st.QueryObservations(ctx, millefeuille.Filter{SourceID: "src-A"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].SourceID != "src-A" {
		t.Errorf("source_id filter failed: %+v", page.Items)
	}
}

func TestQuery_FilterByEventType(t *testing.T) {
	ctx := context.Background()
	st := newStore(t, millefeuille.WithClock(advancingClock(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Second)))

	appendWith(t, st, "evt-1", "src-1", "device.status")
	appendWith(t, st, "evt-2", "src-1", "device.alert")

	page, err := st.QueryObservations(ctx, millefeuille.Filter{EventType: "device.alert"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].EventType != "device.alert" {
		t.Errorf("event_type filter failed: %+v", page.Items)
	}
}

// --- REQ-004 AC-6: time range -----------------------------------------------

func TestQuery_TimeRange(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := newStore(t, millefeuille.WithClock(advancingClock(start, time.Hour)))

	appendWith(t, st, "evt-0", "src-1", "device.status") // t = start+0h
	appendWith(t, st, "evt-1", "src-1", "device.status") // t = start+1h
	appendWith(t, st, "evt-2", "src-1", "device.status") // t = start+2h

	// Range covering only the middle observation.
	from := start.Add(30 * time.Minute)
	to := start.Add(90 * time.Minute)
	page, err := st.QueryObservations(ctx, millefeuille.Filter{From: from, To: to})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].EventID != "evt-1" {
		t.Errorf("time range filter failed: %+v", page.Items)
	}
}

// --- REQ-004 AC-6 (From > To → empty) ---------------------------------------

func TestQuery_FromAfterTo(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := newStore(t, millefeuille.WithClock(advancingClock(start, time.Hour)))

	appendWith(t, st, "evt-0", "src-1", "device.status")
	appendWith(t, st, "evt-1", "src-1", "device.status")

	// From is after To: impossible range → zero rows.
	page, err := st.QueryObservations(ctx, millefeuille.Filter{
		From: start.Add(10 * time.Hour),
		To:   start.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("From>To must yield empty page, got %d items", len(page.Items))
	}
}

// --- REQ-004 AC-7: AND composition ------------------------------------------

func TestQuery_MultipleFiltersAND(t *testing.T) {
	ctx := context.Background()
	st := newStore(t, millefeuille.WithClock(advancingClock(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Second)))

	appendWith(t, st, "evt-1", "src-A", "device.status")
	appendWith(t, st, "evt-2", "src-A", "device.alert")
	appendWith(t, st, "evt-3", "src-B", "device.status")

	page, err := st.QueryObservations(ctx, millefeuille.Filter{
		SourceID:  "src-A",
		EventType: "device.status",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].EventID != "evt-1" {
		t.Errorf("AND filter failed: %+v", page.Items)
	}
}

// --- REQ-004 AC-8: empty result ---------------------------------------------

func TestQuery_EmptyResult(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	appendWith(t, st, "evt-1", "src-1", "device.status")

	page, err := st.QueryObservations(ctx, millefeuille.Filter{SourceID: "nonexistent"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("expected empty page, got %d items", len(page.Items))
	}
}

// --- REQ-004 AC-9: NextCursor always empty ----------------------------------

func TestQuery_NextCursorAlwaysEmpty(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := newStore(t, millefeuille.WithClock(advancingClock(start, time.Second)))

	// Insert more than the default limit to confirm NextCursor stays empty even
	// when more results exist beyond the page.
	for i := 0; i < 150; i++ {
		appendWith(t, st, eid(i), "src-1", "device.status")
	}

	page, err := st.QueryObservations(ctx, millefeuille.Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if page.NextCursor != "" {
		t.Errorf("NextCursor must be empty in M0.1, got %q", page.NextCursor)
	}
}

// --- Negative limit → ErrInvalidFilter --------------------------------------

func TestQuery_NegativeLimit(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	_, err := st.QueryObservations(ctx, millefeuille.Filter{Limit: -1})
	if !errors.Is(err, millefeuille.ErrInvalidFilter) {
		t.Fatalf("expected ErrInvalidFilter, got %v", err)
	}
}

// eid builds a zero-padded event id so that lexicographic order matches numeric
// order (not strictly needed for these tests, but keeps ids unique).
func eid(i int) string {
	const digits = "0123456789"
	// 4-digit zero-padded suffix.
	b := []byte("evt-0000")
	n := i
	for pos := len(b) - 1; pos >= len("evt-"); pos-- {
		b[pos] = digits[n%10]
		n /= 10
	}
	return string(b)
}
