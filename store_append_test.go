package millefeuille_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	millefeuille "github.com/habuka036/millefeuille"
)

// fixedClock returns a ClockFunc that always returns t.
func fixedClock(t time.Time) millefeuille.ClockFunc {
	return func() time.Time { return t }
}

// advancingClock returns a ClockFunc that advances by step on each call.
func advancingClock(start time.Time, step time.Duration) millefeuille.ClockFunc {
	cur := start
	return func() time.Time {
		v := cur
		cur = cur.Add(step)
		return v
	}
}

// newStore opens a file-based store in a temp dir with the given options.
func newStore(t *testing.T, opts ...millefeuille.Option) millefeuille.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := millefeuille.Open(context.Background(), path, opts...)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// validInput returns a valid InputObservation for use as a test baseline.
func validInput() millefeuille.InputObservation {
	return millefeuille.InputObservation{
		EventID:       "evt-001",
		SchemaVersion: "0.1",
		SourceID:      "src-001",
		DeviceID:      "dev-001",
		EventType:     "device.status",
		ObservedAt:    time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC),
		Payload:       json.RawMessage(`{"temperature":21.5}`),
		Visibility:    "private",
		Provenance:    millefeuille.Provenance{Origin: "device", TrustLevel: "unverified"},
	}
}

// --- REQ-001 AC-1: Receipt fields -------------------------------------------

func TestAppend_ReturnsReceipt(t *testing.T) {
	ctx := context.Background()
	fixed := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	st := newStore(t, millefeuille.WithClock(fixedClock(fixed)))

	rcpt, err := st.AppendObservation(ctx, validInput())
	if err != nil {
		t.Fatalf("AppendObservation: %v", err)
	}
	if rcpt.EventID != "evt-001" {
		t.Errorf("EventID: got %q", rcpt.EventID)
	}
	if !rcpt.ReceivedAt.Equal(fixed) {
		t.Errorf("ReceivedAt: got %v, want %v", rcpt.ReceivedAt, fixed)
	}
	if rcpt.ContentHash == "" {
		t.Error("ContentHash is empty")
	}
}

// --- REQ-001 AC-2: survives restart -----------------------------------------

func TestAppend_SurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	st1, err := millefeuille.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	if _, err := st1.AppendObservation(ctx, validInput()); err != nil {
		t.Fatalf("AppendObservation: %v", err)
	}
	if err := st1.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}

	st2, err := millefeuille.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	defer st2.Close()

	got, err := st2.GetObservation(ctx, "evt-001")
	if err != nil {
		t.Fatalf("GetObservation after restart: %v", err)
	}
	if got.EventID != "evt-001" {
		t.Errorf("unexpected observation after restart: %+v", got)
	}
}

// --- REQ-001 AC-3: cancelled context ----------------------------------------

func TestAppend_CancelledContext(t *testing.T) {
	st := newStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := st.AppendObservation(ctx, validInput())
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}

	// Confirm no row was written.
	got, getErr := st.GetObservation(context.Background(), "evt-001")
	if !errors.Is(getErr, millefeuille.ErrNotFound) {
		t.Errorf("expected ErrNotFound after cancelled append, got obs=%v err=%v", got, getErr)
	}
}

// --- REQ-001 AC-4: payload size boundary ------------------------------------

func TestAppend_PayloadExactly1MiB(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	// Build a JSON payload whose byte length is exactly 1 MiB (1,048,576).
	in := validInput()
	in.Payload = makeJSONPayloadOfSize(t, 1<<20)
	if len(in.Payload) != 1<<20 {
		t.Fatalf("payload size setup error: got %d bytes", len(in.Payload))
	}

	if _, err := st.AppendObservation(ctx, in); err != nil {
		t.Fatalf("expected exactly-1-MiB payload to be accepted, got %v", err)
	}
}

func TestAppend_PayloadOver1MiB(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	in := validInput()
	in.Payload = makeJSONPayloadOfSize(t, (1<<20)+1)
	if len(in.Payload) != (1<<20)+1 {
		t.Fatalf("payload size setup error: got %d bytes", len(in.Payload))
	}

	_, err := st.AppendObservation(ctx, in)
	if !errors.Is(err, millefeuille.ErrPayloadTooLarge) {
		t.Fatalf("expected ErrPayloadTooLarge, got %v", err)
	}
}

// makeJSONPayloadOfSize builds a valid JSON object literal of exactly n bytes.
// Format: {"d":"<padding>"} where padding is adjusted so total length == n.
func makeJSONPayloadOfSize(t *testing.T, n int) json.RawMessage {
	t.Helper()
	const prefix = `{"d":"`
	const suffix = `"}`
	overhead := len(prefix) + len(suffix)
	if n < overhead {
		t.Fatalf("requested size %d smaller than overhead %d", n, overhead)
	}
	padding := strings.Repeat("a", n-overhead)
	raw := prefix + padding + suffix
	if len(raw) != n {
		t.Fatalf("constructed payload size %d != requested %d", len(raw), n)
	}
	return json.RawMessage(raw)
}

// --- REQ-001 AC-5: required fields, invalid JSON, max length ----------------

func TestAppend_MissingRequiredFields(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	cases := map[string]func(*millefeuille.InputObservation){
		"event_id":          func(in *millefeuille.InputObservation) { in.EventID = "" },
		"schema_version":    func(in *millefeuille.InputObservation) { in.SchemaVersion = "" },
		"source_id":         func(in *millefeuille.InputObservation) { in.SourceID = "" },
		"event_type":        func(in *millefeuille.InputObservation) { in.EventType = "" },
		"provenance.origin": func(in *millefeuille.InputObservation) { in.Provenance.Origin = "" },
		"observed_at":       func(in *millefeuille.InputObservation) { in.ObservedAt = time.Time{} },
		"payload":           func(in *millefeuille.InputObservation) { in.Payload = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			mutate(&in)
			_, err := st.AppendObservation(ctx, in)
			if !errors.Is(err, millefeuille.ErrInvalidObservation) {
				t.Errorf("field %q omitted: expected ErrInvalidObservation, got %v", name, err)
			}
		})
	}
}

func TestAppend_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	in := validInput()
	in.Payload = json.RawMessage(`{not valid json`)
	_, err := st.AppendObservation(ctx, in)
	if !errors.Is(err, millefeuille.ErrInvalidObservation) {
		t.Fatalf("expected ErrInvalidObservation for invalid JSON, got %v", err)
	}
}

func TestAppend_MaxLengthExceeded(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	cases := map[string]func(*millefeuille.InputObservation){
		"event_id":               func(in *millefeuille.InputObservation) { in.EventID = strings.Repeat("x", 129) },
		"schema_version":         func(in *millefeuille.InputObservation) { in.SchemaVersion = strings.Repeat("x", 33) },
		"source_id":              func(in *millefeuille.InputObservation) { in.SourceID = strings.Repeat("x", 257) },
		"device_id":              func(in *millefeuille.InputObservation) { in.DeviceID = strings.Repeat("x", 257) },
		"event_type":             func(in *millefeuille.InputObservation) { in.EventType = strings.Repeat("x", 129) },
		"visibility":             func(in *millefeuille.InputObservation) { in.Visibility = strings.Repeat("x", 65) },
		"provenance.origin":      func(in *millefeuille.InputObservation) { in.Provenance.Origin = strings.Repeat("x", 65) },
		"provenance.trust_level": func(in *millefeuille.InputObservation) { in.Provenance.TrustLevel = strings.Repeat("x", 65) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			mutate(&in)
			_, err := st.AppendObservation(ctx, in)
			if !errors.Is(err, millefeuille.ErrInvalidObservation) {
				t.Errorf("field %q over max length: expected ErrInvalidObservation, got %v", name, err)
			}
		})
	}
}

// --- REQ-002 AC-1: idempotent re-append -------------------------------------

func TestAppend_IdempotentIdentical(t *testing.T) {
	ctx := context.Background()
	st := newStore(t, millefeuille.WithClock(advancingClock(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Hour)))

	first, err := st.AppendObservation(ctx, validInput())
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := st.AppendObservation(ctx, validInput())
	if err != nil {
		t.Fatalf("second append (idempotent): %v", err)
	}

	if first.ContentHash != second.ContentHash {
		t.Errorf("content hash differs: %q vs %q", first.ContentHash, second.ContentHash)
	}

	// Only one row must exist.
	page, err := st.QueryObservations(ctx, millefeuille.Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Items) != 1 {
		t.Errorf("expected 1 stored observation, got %d", len(page.Items))
	}
}

// --- REQ-002 AC-3/4: idempotent returns original received_at ----------------

func TestAppend_IdempotentReturnsOriginalReceivedAt(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := newStore(t, millefeuille.WithClock(advancingClock(start, time.Hour)))

	first, err := st.AppendObservation(ctx, validInput())
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := st.AppendObservation(ctx, validInput())
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if !second.ReceivedAt.Equal(first.ReceivedAt) {
		t.Errorf("idempotent re-append must keep original received_at: first %v, second %v",
			first.ReceivedAt, second.ReceivedAt)
	}
}

// --- REQ-002 AC-2: conflict on content change -------------------------------

func TestAppend_ConflictOnPayloadChange(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	if _, err := st.AppendObservation(ctx, validInput()); err != nil {
		t.Fatalf("first append: %v", err)
	}
	in := validInput()
	in.Payload = json.RawMessage(`{"temperature":99.9}`)
	_, err := st.AppendObservation(ctx, in)
	if !errors.Is(err, millefeuille.ErrConflict) {
		t.Fatalf("expected ErrConflict on payload change, got %v", err)
	}
}

func TestAppend_ConflictOnVisibilityChange(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	if _, err := st.AppendObservation(ctx, validInput()); err != nil {
		t.Fatalf("first append: %v", err)
	}
	in := validInput()
	in.Visibility = "public"
	_, err := st.AppendObservation(ctx, in)
	if !errors.Is(err, millefeuille.ErrConflict) {
		t.Fatalf("expected ErrConflict on visibility change, got %v", err)
	}
}

func TestAppend_ConflictOnProvenanceOriginChange(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	if _, err := st.AppendObservation(ctx, validInput()); err != nil {
		t.Fatalf("first append: %v", err)
	}
	in := validInput()
	in.Provenance.Origin = "agent"
	_, err := st.AppendObservation(ctx, in)
	if !errors.Is(err, millefeuille.ErrConflict) {
		t.Fatalf("expected ErrConflict on provenance.origin change, got %v", err)
	}
}

// --- REQ-002 AC-2 (default equivalence) -------------------------------------

func TestAppend_DefaultVisibilityEquivalence(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	// First: visibility omitted (defaults to "private").
	in1 := validInput()
	in1.Visibility = ""
	if _, err := st.AppendObservation(ctx, in1); err != nil {
		t.Fatalf("first append: %v", err)
	}

	// Second: visibility explicitly "private".
	in2 := validInput()
	in2.Visibility = "private"
	if _, err := st.AppendObservation(ctx, in2); err != nil {
		t.Fatalf("explicit private must be idempotent with defaulted private, got %v", err)
	}
}

func TestAppend_DefaultTrustLevelEquivalence(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	in1 := validInput()
	in1.Provenance.TrustLevel = ""
	if _, err := st.AppendObservation(ctx, in1); err != nil {
		t.Fatalf("first append: %v", err)
	}

	in2 := validInput()
	in2.Provenance.TrustLevel = "unverified"
	if _, err := st.AppendObservation(ctx, in2); err != nil {
		t.Fatalf("explicit unverified must be idempotent with defaulted unverified, got %v", err)
	}
}

// --- REQ-005 AC-3: fixed clock ----------------------------------------------

func TestAppend_FixedClockReceivedAt(t *testing.T) {
	ctx := context.Background()
	fixed := time.Date(2026, 7, 7, 7, 7, 7, 7, time.UTC)
	st := newStore(t, millefeuille.WithClock(fixedClock(fixed)))

	rcpt, err := st.AppendObservation(ctx, validInput())
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !rcpt.ReceivedAt.Equal(fixed) {
		t.Errorf("received_at: got %v, want %v", rcpt.ReceivedAt, fixed)
	}
}

// --- SEC-001 / SEC-002: defaults --------------------------------------------

func TestAppend_VisibilityDefault(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	in := validInput()
	in.Visibility = ""
	if _, err := st.AppendObservation(ctx, in); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, err := st.GetObservation(ctx, in.EventID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Visibility != "private" {
		t.Errorf("visibility default: got %q, want %q", got.Visibility, "private")
	}
}

func TestAppend_TrustLevelDefault(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	in := validInput()
	in.Provenance.TrustLevel = ""
	if _, err := st.AppendObservation(ctx, in); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, err := st.GetObservation(ctx, in.EventID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Provenance.TrustLevel != "unverified" {
		t.Errorf("trust_level default: got %q, want %q", got.Provenance.TrustLevel, "unverified")
	}
}

// --- NFR-004: UTC roundtrip and nanosecond precision ------------------------

func TestAppend_UTCRoundtrip(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	// ObservedAt in a non-UTC timezone.
	loc := time.FixedZone("JST", 9*3600)
	in := validInput()
	in.ObservedAt = time.Date(2026, 3, 4, 5, 6, 7, 0, loc)

	if _, err := st.AppendObservation(ctx, in); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, err := st.GetObservation(ctx, in.EventID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Stored observed_at must represent the same instant, in UTC.
	if !got.ObservedAt.Equal(in.ObservedAt) {
		t.Errorf("observed_at instant mismatch: got %v, want %v", got.ObservedAt, in.ObservedAt)
	}
	if got.ObservedAt.Location() != time.UTC {
		t.Errorf("observed_at must be UTC, got location %v", got.ObservedAt.Location())
	}
}

func TestAppend_NanosecondPrecision(t *testing.T) {
	ctx := context.Background()
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC)
	st := newStore(t, millefeuille.WithClock(fixedClock(fixed)))

	in := validInput()
	in.ObservedAt = time.Date(2026, 2, 2, 2, 2, 2, 987654321, time.UTC)

	if _, err := st.AppendObservation(ctx, in); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, err := st.GetObservation(ctx, in.EventID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ObservedAt.Nanosecond() != 987654321 {
		t.Errorf("observed_at nanosecond lost: got %d", got.ObservedAt.Nanosecond())
	}
	if got.ReceivedAt.Nanosecond() != 123456789 {
		t.Errorf("received_at nanosecond lost: got %d", got.ReceivedAt.Nanosecond())
	}
}
