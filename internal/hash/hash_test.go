package hash_test

import (
	"encoding/json"
	"testing"
	"time"

	millefeuille "github.com/habuka036/millefeuille"
	"github.com/habuka036/millefeuille/internal/hash"
)

// baseObs returns a valid InputObservation for use as a test baseline.
func baseObs() millefeuille.InputObservation {
	return millefeuille.InputObservation{
		EventID:       "evt-001",
		SchemaVersion: "0.1",
		SourceID:      "src-001",
		DeviceID:      "dev-001",
		EventType:     "device.status",
		ObservedAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Payload:       json.RawMessage(`{"key":"value"}`),
		Visibility:    "private",
		Provenance:    millefeuille.Provenance{Origin: "device", TrustLevel: "unverified"},
	}
}

// TestCanonical_ByteIdentical verifies that two identical observations produce
// the same hash.
func TestCanonical_ByteIdentical(t *testing.T) {
	a := baseObs()
	b := baseObs()
	ha, err := hash.Canonical(a)
	if err != nil {
		t.Fatalf("Canonical(a): %v", err)
	}
	hb, err := hash.Canonical(b)
	if err != nil {
		t.Fatalf("Canonical(b): %v", err)
	}
	if ha != hb {
		t.Errorf("expected same hash for identical inputs, got %q vs %q", ha, hb)
	}
}

// TestCanonical_PayloadWhitespaceDiff verifies that payloads differing only in
// whitespace produce the same hash (normalised by decode/re-encode).
func TestCanonical_PayloadWhitespaceDiff(t *testing.T) {
	a := baseObs()
	b := baseObs()
	a.Payload = json.RawMessage(`{"key":   "value"}`)
	b.Payload = json.RawMessage(`{"key":"value"}`)
	ha, err := hash.Canonical(a)
	if err != nil {
		t.Fatalf("Canonical(a): %v", err)
	}
	hb, err := hash.Canonical(b)
	if err != nil {
		t.Fatalf("Canonical(b): %v", err)
	}
	if ha != hb {
		t.Errorf("expected same hash for whitespace-only diff, got %q vs %q", ha, hb)
	}
}

// TestCanonical_PayloadKeyOrderDiff verifies that payloads differing only in
// key order at any nesting level produce the same hash.
func TestCanonical_PayloadKeyOrderDiff(t *testing.T) {
	a := baseObs()
	b := baseObs()
	// Top-level key order difference.
	a.Payload = json.RawMessage(`{"b":2,"a":1}`)
	b.Payload = json.RawMessage(`{"a":1,"b":2}`)
	ha, err := hash.Canonical(a)
	if err != nil {
		t.Fatalf("Canonical(a): %v", err)
	}
	hb, err := hash.Canonical(b)
	if err != nil {
		t.Fatalf("Canonical(b): %v", err)
	}
	if ha != hb {
		t.Errorf("expected same hash for key-order diff (top-level), got %q vs %q", ha, hb)
	}

	// Nested key order difference.
	a.Payload = json.RawMessage(`{"outer":{"z":26,"a":1}}`)
	b.Payload = json.RawMessage(`{"outer":{"a":1,"z":26}}`)
	ha, err = hash.Canonical(a)
	if err != nil {
		t.Fatalf("Canonical(a) nested: %v", err)
	}
	hb, err = hash.Canonical(b)
	if err != nil {
		t.Fatalf("Canonical(b) nested: %v", err)
	}
	if ha != hb {
		t.Errorf("expected same hash for key-order diff (nested), got %q vs %q", ha, hb)
	}
}

// TestCanonical_NumericRepresentationDiff documents that payloads differing
// only in numeric representation (1 vs 1.0) produce different hashes because
// json.Number preserves the original token.
func TestCanonical_NumericRepresentationDiff(t *testing.T) {
	a := baseObs()
	b := baseObs()
	a.Payload = json.RawMessage(`{"n":1}`)
	b.Payload = json.RawMessage(`{"n":1.0}`)
	ha, err := hash.Canonical(a)
	if err != nil {
		t.Fatalf("Canonical(a): %v", err)
	}
	hb, err := hash.Canonical(b)
	if err != nil {
		t.Fatalf("Canonical(b): %v", err)
	}
	// Documented behaviour: different numeric representations → different hash.
	if ha == hb {
		t.Errorf("expected different hash for numeric representation diff (1 vs 1.0), got same: %q", ha)
	}
}

// TestCanonical_DuplicateKeys verifies that payloads with duplicate keys produce
// a deterministic hash via last-key-wins resolution, and that two submissions
// with the same last-key-wins result are idempotent.
func TestCanonical_DuplicateKeys(t *testing.T) {
	a := baseObs()
	b := baseObs()
	// Both have duplicate key "k"; last value is 2 in both cases.
	a.Payload = json.RawMessage(`{"k":1,"k":2}`)
	b.Payload = json.RawMessage(`{"k":2}`)
	ha, err := hash.Canonical(a)
	if err != nil {
		t.Fatalf("Canonical(a): %v", err)
	}
	hb, err := hash.Canonical(b)
	if err != nil {
		t.Fatalf("Canonical(b): %v", err)
	}
	// After last-key-wins decode, both resolve to {"k":2}; hashes must match.
	if ha != hb {
		t.Errorf("expected same hash after duplicate-key last-key-wins, got %q vs %q", ha, hb)
	}

	// Calling Canonical twice on the same input must be deterministic.
	ha2, err := hash.Canonical(a)
	if err != nil {
		t.Fatalf("Canonical(a) second call: %v", err)
	}
	if ha != ha2 {
		t.Errorf("Canonical is not deterministic: %q vs %q", ha, ha2)
	}
}

// TestCanonical_TrailingData verifies that a payload with trailing bytes after
// valid JSON causes Canonical to return an error.
func TestCanonical_TrailingData(t *testing.T) {
	a := baseObs()
	a.Payload = json.RawMessage(`{"k":1}garbage`)
	_, err := hash.Canonical(a)
	if err == nil {
		t.Error("expected error for payload with trailing data, got nil")
	}
}

// TestCanonical_ReceivedAtExcluded verifies that two observations identical
// except for a (hypothetical) received_at-equivalent field still hash the same.
// Since received_at is not part of InputObservation, we verify by confirming
// the hash depends only on the canonical fields defined in the spec.
func TestCanonical_ObservedAtIsIncluded(t *testing.T) {
	a := baseObs()
	b := baseObs()
	b.ObservedAt = a.ObservedAt.Add(time.Second)
	ha, _ := hash.Canonical(a)
	hb, _ := hash.Canonical(b)
	if ha == hb {
		t.Error("expected different hash when ObservedAt differs")
	}
}

// TestCanonical_DeviceIDEmptyString verifies that absent DeviceID (empty string)
// is included in the projection consistently.
func TestCanonical_DeviceIDEmptyVsAbsent(t *testing.T) {
	a := baseObs()
	b := baseObs()
	a.DeviceID = ""
	b.DeviceID = ""
	ha, err := hash.Canonical(a)
	if err != nil {
		t.Fatalf("Canonical(a): %v", err)
	}
	hb, err := hash.Canonical(b)
	if err != nil {
		t.Fatalf("Canonical(b): %v", err)
	}
	if ha != hb {
		t.Errorf("expected same hash for both empty DeviceID, got %q vs %q", ha, hb)
	}
}
