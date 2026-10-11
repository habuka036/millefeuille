package millefeuille

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/habuka036/millefeuille/internal/hash"
)

// maxPayloadBytes is the maximum allowed payload size: 1 MiB.
const maxPayloadBytes = 1 << 20 // 1,048,576

// defaultVisibility and defaultTrustLevel are applied when the caller leaves the
// corresponding field empty.
const (
	defaultVisibility = "private"
	defaultTrustLevel = "unverified"
)

// maxFieldLengths defines the maximum byte length for each required/optional
// string field (REQ-005 AC-6).
var maxFieldLengths = map[string]int{
	"event_id":               128,
	"schema_version":         32,
	"source_id":              256,
	"device_id":              256,
	"event_type":             128,
	"visibility":             64,
	"provenance.origin":      64,
	"provenance.trust_level": 64,
}

// AppendObservation validates the input, applies defaults, assigns received_at,
// computes the content hash, and delegates persistence (including idempotency and
// conflict detection) to the repository.
func (s *store) AppendObservation(ctx context.Context, obs InputObservation) (Receipt, error) {
	// Apply defaults before validation of enumerable fields so that defaulted
	// values are validated too.
	if obs.Visibility == "" {
		obs.Visibility = defaultVisibility
	}
	if obs.Provenance.TrustLevel == "" {
		obs.Provenance.TrustLevel = defaultTrustLevel
	}

	if err := validateInput(obs); err != nil {
		return Receipt{}, err
	}

	// Assign received_at after validation passes and the observation is accepted
	// for storage. This is the library acceptance time, not the DB commit time.
	receivedAt := s.clock().UTC()

	contentHash, err := hash.Canonical(obs)
	if err != nil {
		return Receipt{}, fmt.Errorf("AppendObservation: compute content hash: %w", err)
	}

	stored := Observation{
		EventID:       obs.EventID,
		SchemaVersion: obs.SchemaVersion,
		SourceID:      obs.SourceID,
		DeviceID:      obs.DeviceID,
		EventType:     obs.EventType,
		ObservedAt:    obs.ObservedAt.UTC(),
		ReceivedAt:    receivedAt,
		Payload:       obs.Payload, // raw input bytes, stored as-is
		Visibility:    obs.Visibility,
		Provenance:    obs.Provenance,
	}

	// The repository owns the transaction and the idempotency/conflict logic.
	// Its (Receipt, error) result is returned directly to the caller.
	return s.repo.Insert(ctx, stored, contentHash)
}

// validateInput checks required fields, string lengths, and payload validity.
func validateInput(obs InputObservation) error {
	// Required non-empty string fields.
	required := []struct {
		name  string
		value string
	}{
		{"event_id", obs.EventID},
		{"schema_version", obs.SchemaVersion},
		{"source_id", obs.SourceID},
		{"event_type", obs.EventType},
		{"provenance.origin", obs.Provenance.Origin},
	}
	for _, f := range required {
		if f.value == "" {
			return fmt.Errorf("missing required field %q: %w", f.name, ErrInvalidObservation)
		}
	}

	// observed_at must be set.
	if obs.ObservedAt.IsZero() {
		return fmt.Errorf("missing required field %q: %w", "observed_at", ErrInvalidObservation)
	}

	// Length checks for all bounded string fields (including defaulted ones).
	lengths := map[string]string{
		"event_id":               obs.EventID,
		"schema_version":         obs.SchemaVersion,
		"source_id":              obs.SourceID,
		"device_id":              obs.DeviceID,
		"event_type":             obs.EventType,
		"visibility":             obs.Visibility,
		"provenance.origin":      obs.Provenance.Origin,
		"provenance.trust_level": obs.Provenance.TrustLevel,
	}
	for name, value := range lengths {
		if max, ok := maxFieldLengths[name]; ok && len(value) > max {
			return fmt.Errorf("field %q exceeds maximum length %d: %w", name, max, ErrInvalidObservation)
		}
	}

	// Payload presence.
	if len(obs.Payload) == 0 {
		return fmt.Errorf("missing required field %q: %w", "payload", ErrInvalidObservation)
	}

	// Payload size limit (byte length of the input JSON).
	if len(obs.Payload) > maxPayloadBytes {
		return ErrPayloadTooLarge
	}

	// Payload must be valid JSON.
	if !json.Valid(obs.Payload) {
		return fmt.Errorf("payload is not valid JSON: %w", ErrInvalidObservation)
	}

	return nil
}
