// Package hash provides canonical content hashing for idempotency detection.
package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/habuka036/millefeuille/internal/model"
)

// timestampFormat is the fixed nanosecond-precision UTC format used for all
// timestamp serialisation within the canonical projection.
const timestampFormat = "2006-01-02T15:04:05.000000000Z"

// canonicalProjection is the struct serialised to produce the hash input.
// Field order is fixed; json.Marshal sorts map keys inside nested JSON objects.
type canonicalProjection struct {
	EventID              string `json:"event_id"`
	SchemaVersion        string `json:"schema_version"`
	SourceID             string `json:"source_id"`
	DeviceID             string `json:"device_id"`
	EventType            string `json:"event_type"`
	ObservedAt           string `json:"observed_at"`
	Payload              any    `json:"payload"`
	Visibility           string `json:"visibility"`
	ProvenanceOrigin     string `json:"provenance_origin"`
	ProvenanceTrustLevel string `json:"provenance_trust_level"`
}

// Canonical computes a SHA-256 hex digest over the canonical projection of obs.
//
// The projection includes all caller-supplied fields and explicitly excludes
// ReceivedAt (library-assigned). The payload is normalised through a
// decode-then-encode cycle using UseNumber() so that:
//   - Whitespace differences are eliminated.
//   - Map key order is normalised lexicographically at every nesting level
//     (json.Marshal sorts map keys when marshalling map[string]any).
//   - Numeric values are preserved as json.Number (no float64 conversion).
//   - Duplicate keys resolve via last-key-wins on decode.
//
// The decoder is checked to confirm the entire input is consumed; if trailing
// data follows the valid JSON value, Canonical returns an error.
func Canonical(obs model.InputObservation) (string, error) {
	normPayload, err := normalisePayload(obs.Payload)
	if err != nil {
		return "", fmt.Errorf("hash.Canonical: normalise payload: %w", err)
	}

	proj := canonicalProjection{
		EventID:              obs.EventID,
		SchemaVersion:        obs.SchemaVersion,
		SourceID:             obs.SourceID,
		DeviceID:             obs.DeviceID,
		EventType:            obs.EventType,
		ObservedAt:           obs.ObservedAt.UTC().Format(timestampFormat),
		Payload:              normPayload,
		Visibility:           obs.Visibility,
		ProvenanceOrigin:     obs.Provenance.Origin,
		ProvenanceTrustLevel: obs.Provenance.TrustLevel,
	}

	data, err := marshalSorted(proj)
	if err != nil {
		return "", fmt.Errorf("hash.Canonical: marshal projection: %w", err)
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// normalisePayload decodes the raw JSON payload using UseNumber() to preserve
// numeric tokens, then re-encodes it. json.Marshal sorts map keys at every
// level when marshalling map[string]any, achieving full key-order normalisation.
// Trailing data after the JSON value causes an error.
func normalisePayload(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	// Confirm the entire input was consumed.
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("payload has trailing data after valid JSON")
	}

	return v, nil
}

// marshalSorted marshals v to JSON. For the canonicalProjection struct, Go's
// encoding/json uses field order from the struct definition. For any map[string]any
// values nested inside (from the payload), json.Marshal sorts keys lexicographically.
func marshalSorted(v any) ([]byte, error) {
	return json.Marshal(v)
}
