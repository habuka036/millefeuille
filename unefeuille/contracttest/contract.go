// Package contracttest provides an implementation-independent contract test
// suite for unefeuille.Store. It verifies only externally observable behaviour
// (round-trip field preservation, JSON identity, time precision, idempotency,
// conflict, invalid input, not-found, search filtering/ordering/limits,
// pre-call cancellation side-effects, and Close idempotency). It does not
// depend on any backend's internal representation or hashing algorithm.
//
// Apply it to any Store implementation (for example the in-memory fake and the
// MILLEFEUILLE adapter) so that all implementations exhibit the same contract.
package contracttest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/habuka036/millefeuille/unefeuille"
)

// Factory produces a fresh Store for each (sub)test. The returned Store is
// closed by the suite via t.Cleanup.
type Factory func(t *testing.T) unefeuille.Store

// Run executes the full contract suite against stores produced by newStore.
func Run(t *testing.T, newStore Factory) {
	t.Helper()

	t.Run("SaveGetRoundTrip", func(t *testing.T) { testRoundTrip(t, newStore) })
	t.Run("Idempotent", func(t *testing.T) { testIdempotent(t, newStore) })
	t.Run("Conflict", func(t *testing.T) { testConflict(t, newStore) })
	t.Run("InvalidInput", func(t *testing.T) { testInvalidInput(t, newStore) })
	t.Run("NotFound", func(t *testing.T) { testNotFound(t, newStore) })
	t.Run("SearchFilterAndOrder", func(t *testing.T) { testSearchFilterAndOrder(t, newStore) })
	t.Run("SearchLimits", func(t *testing.T) { testSearchLimits(t, newStore) })
	t.Run("SearchSameTimestampDeterministic", func(t *testing.T) { testSameTimestampOrder(t, newStore) })
	t.Run("CancelBeforeCallNoSideEffect", func(t *testing.T) { testCancelNoSideEffect(t, newStore) })
	t.Run("CloseIdempotent", func(t *testing.T) { testCloseIdempotent(t, newStore) })
}

func newWithCleanup(t *testing.T, newStore Factory) unefeuille.Store {
	t.Helper()
	st := newStore(t)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func sampleMemory(id string) unefeuille.Memory {
	return unefeuille.Memory{
		ID:         id,
		Kind:       "navette.run.log",
		Source:     "navette",
		Device:     "dev-1",
		Origin:     "agent",
		ObservedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Content:    json.RawMessage(`{"stage":"plan","status":"ok"}`),
		Labels: unefeuille.Labels{
			Visibility:    "private",
			TrustLevel:    "unverified",
			SchemaVersion: "0.1",
		},
	}
}

func testRoundTrip(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx := context.Background()
	in := sampleMemory("mem-rt")

	rec, err := st.Save(ctx, in)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if rec.ID != in.ID {
		t.Errorf("receipt ID = %q, want %q", rec.ID, in.ID)
	}
	if rec.StoredAt.IsZero() {
		t.Error("receipt StoredAt is zero; backend must assign a commit time")
	}

	got, err := st.Get(ctx, in.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Field preservation (including Device and Labels).
	if got.Kind != in.Kind || got.Source != in.Source || got.Device != in.Device ||
		got.Origin != in.Origin {
		t.Errorf("field mismatch: got %+v want kind/source/device/origin of %+v", got, in)
	}
	if got.Labels != in.Labels {
		t.Errorf("labels mismatch: got %+v want %+v", got.Labels, in.Labels)
	}
	if !got.ObservedAt.Equal(in.ObservedAt) {
		t.Errorf("ObservedAt mismatch: got %v want %v", got.ObservedAt, in.ObservedAt)
	}
	// StoredAt must round-trip consistently with the receipt (time precision).
	if !got.StoredAt.Equal(rec.StoredAt) {
		t.Errorf("StoredAt mismatch: Get %v vs receipt %v", got.StoredAt, rec.StoredAt)
	}
	if got.StoredAt.Location() != time.UTC {
		t.Errorf("StoredAt not UTC: %v", got.StoredAt.Location())
	}
	// JSON identity: semantic equality (whitespace/key-order independent).
	assertJSONEqual(t, got.Content, in.Content)
}

func testIdempotent(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx := context.Background()
	in := sampleMemory("mem-idem")

	rec1, err := st.Save(ctx, in)
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	// Re-save with semantically identical content but different whitespace/key order.
	in2 := in
	in2.Content = json.RawMessage(`{"status":"ok",  "stage":"plan"}`)
	rec2, err := st.Save(ctx, in2)
	if err != nil {
		t.Fatalf("idempotent re-Save: %v", err)
	}
	if !rec2.StoredAt.Equal(rec1.StoredAt) {
		t.Errorf("idempotent re-Save changed StoredAt: %v vs %v", rec2.StoredAt, rec1.StoredAt)
	}

	// Only one memory should exist.
	res, err := st.Search(ctx, unefeuille.Query{Source: in.Source})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 {
		t.Errorf("expected exactly 1 memory after idempotent re-save, got %d", len(res))
	}
}

func testConflict(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx := context.Background()
	in := sampleMemory("mem-conflict")

	if _, err := st.Save(ctx, in); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	conflicting := in
	conflicting.Content = json.RawMessage(`{"stage":"plan","status":"CHANGED"}`)
	_, err := st.Save(ctx, conflicting)
	if !errors.Is(err, unefeuille.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	// Original memory must be preserved (not overwritten).
	got, err := st.Get(ctx, in.ID)
	if err != nil {
		t.Fatalf("Get after conflict: %v", err)
	}
	assertJSONEqual(t, got.Content, in.Content)
}

func testInvalidInput(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx := context.Background()

	// Missing required field (origin).
	m := sampleMemory("mem-invalid")
	m.Origin = ""
	if _, err := st.Save(ctx, m); !errors.Is(err, unefeuille.ErrInvalidMemory) {
		t.Errorf("missing origin: expected ErrInvalidMemory, got %v", err)
	}

	// Missing required field (schema version).
	mSV := sampleMemory("mem-invalid-sv")
	mSV.Labels.SchemaVersion = ""
	if _, err := st.Save(ctx, mSV); !errors.Is(err, unefeuille.ErrInvalidMemory) {
		t.Errorf("missing schema_version: expected ErrInvalidMemory, got %v", err)
	}

	// Invalid JSON content.
	m2 := sampleMemory("mem-invalid2")
	m2.Content = json.RawMessage(`{nope}`)
	if _, err := st.Save(ctx, m2); !errors.Is(err, unefeuille.ErrInvalidMemory) {
		t.Errorf("invalid JSON: expected ErrInvalidMemory, got %v", err)
	}

	// Oversized content.
	m3 := sampleMemory("mem-invalid3")
	big := make([]byte, unefeuille.MaxContentBytes+10)
	for i := range big {
		big[i] = 'a'
	}
	m3.Content = json.RawMessage(`"` + string(big) + `"`)
	if _, err := st.Save(ctx, m3); !errors.Is(err, unefeuille.ErrContentTooLarge) {
		t.Errorf("oversized content: expected ErrContentTooLarge, got %v", err)
	}
}

func testNotFound(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx := context.Background()
	if _, err := st.Get(ctx, "does-not-exist"); !errors.Is(err, unefeuille.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func testSearchFilterAndOrder(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx := context.Background()

	// Three memories with distinct kinds/sources.
	for i, m := range []unefeuille.Memory{
		mutate(sampleMemory("s-1"), "alpha", "sensor"),
		mutate(sampleMemory("s-2"), "beta", "sensor"),
		mutate(sampleMemory("s-3"), "alpha", "other"),
	} {
		if _, err := st.Save(ctx, m); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	// Filter by Source.
	res, err := st.Search(ctx, unefeuille.Query{Source: "sensor"})
	if err != nil {
		t.Fatalf("Search by source: %v", err)
	}
	if len(res) != 2 {
		t.Errorf("source filter: expected 2, got %d", len(res))
	}

	// Filter by Kind.
	res, err = st.Search(ctx, unefeuille.Query{Kind: "alpha"})
	if err != nil {
		t.Fatalf("Search by kind: %v", err)
	}
	if len(res) != 2 {
		t.Errorf("kind filter: expected 2, got %d", len(res))
	}

	// No match returns empty, not error.
	res, err = st.Search(ctx, unefeuille.Query{Source: "nonexistent"})
	if err != nil {
		t.Fatalf("Search no match: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("no-match: expected empty, got %d", len(res))
	}

	// Results ordered by StoredAt descending (most recent first).
	all, err := st.Search(ctx, unefeuille.Query{})
	if err != nil {
		t.Fatalf("Search all: %v", err)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].StoredAt.Before(all[i].StoredAt) {
			t.Errorf("results not StoredAt-descending at index %d", i)
		}
	}
}

func testSearchLimits(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx := context.Background()

	// Negative limit -> ErrInvalidQuery.
	if _, err := st.Search(ctx, unefeuille.Query{Limit: -1}); !errors.Is(err, unefeuille.ErrInvalidQuery) {
		t.Errorf("negative limit: expected ErrInvalidQuery, got %v", err)
	}

	// Save 3 memories; Limit smaller than count bounds results.
	for i := 0; i < 3; i++ {
		m := sampleMemory("lim-" + string(rune('a'+i)))
		if _, err := st.Save(ctx, m); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	res, err := st.Search(ctx, unefeuille.Query{Limit: 2})
	if err != nil {
		t.Fatalf("Search limit 2: %v", err)
	}
	if len(res) != 2 {
		t.Errorf("limit 2: expected 2 results, got %d", len(res))
	}
	// Limit 0 applies the default (>= our 3 saved).
	res, err = st.Search(ctx, unefeuille.Query{Limit: 0})
	if err != nil {
		t.Fatalf("Search limit 0: %v", err)
	}
	if len(res) != 3 {
		t.Errorf("limit 0 (default): expected 3 results, got %d", len(res))
	}
}

func testSameTimestampOrder(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx := context.Background()

	// Save several memories; even if the backend assigns distinct StoredAt,
	// ordering must be deterministic and stable across repeated Search calls.
	ids := []string{"t-a", "t-b", "t-c", "t-d"}
	for _, id := range ids {
		if _, err := st.Save(ctx, sampleMemory(id)); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	first, err := st.Search(ctx, unefeuille.Query{})
	if err != nil {
		t.Fatalf("Search 1: %v", err)
	}
	second, err := st.Search(ctx, unefeuille.Query{})
	if err != nil {
		t.Fatalf("Search 2: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("result count differs between calls: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("non-deterministic order at %d: %q vs %q", i, first[i].ID, second[i].ID)
		}
	}
	// Verify the documented tie-break: for equal StoredAt, ID descending.
	for i := 1; i < len(first); i++ {
		if first[i-1].StoredAt.Equal(first[i].StoredAt) && first[i-1].ID < first[i].ID {
			t.Errorf("equal StoredAt not ID-descending at %d: %q then %q", i, first[i-1].ID, first[i].ID)
		}
	}
}

func testCancelNoSideEffect(t *testing.T, newStore Factory) {
	st := newWithCleanup(t, newStore)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before any call

	in := sampleMemory("mem-cancel")
	if _, err := st.Save(ctx, in); err == nil {
		t.Error("expected error from Save with pre-cancelled ctx")
	}
	// No partial memory must remain.
	if _, err := st.Get(context.Background(), in.ID); !errors.Is(err, unefeuille.ErrNotFound) {
		t.Errorf("expected ErrNotFound after cancelled Save, got %v", err)
	}
	// Get and Search with pre-cancelled ctx should also error.
	if _, err := st.Get(ctx, "whatever"); err == nil {
		t.Error("expected error from Get with pre-cancelled ctx")
	}
	if _, err := st.Search(ctx, unefeuille.Query{}); err == nil {
		t.Error("expected error from Search with pre-cancelled ctx")
	}
}

func testCloseIdempotent(t *testing.T, newStore Factory) {
	st := newStore(t)
	if err := st.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Errorf("second Close must be safe (idempotent): %v", err)
	}
}

// ---- helpers ----

func mutate(m unefeuille.Memory, kind, source string) unefeuille.Memory {
	m.Kind = kind
	m.Source = source
	return m
}

func assertJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	gn, err := unefeuille.NormaliseContent(got)
	if err != nil {
		t.Fatalf("normalise got: %v", err)
	}
	wn, err := unefeuille.NormaliseContent(want)
	if err != nil {
		t.Fatalf("normalise want: %v", err)
	}
	if string(gn) != string(wn) {
		t.Errorf("JSON content mismatch: got %s want %s", gn, wn)
	}
}
