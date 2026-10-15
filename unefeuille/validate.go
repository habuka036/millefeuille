package unefeuille

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// ValidateForSave checks the required fields and content constraints for a Save
// input, independent of any backend. The required set is
// {ID, Kind, Source, Origin, SchemaVersion, ObservedAt, Content}; Device is
// optional. Origin and SchemaVersion are required because the MILLEFEUILLE
// backend requires provenance.origin and schema_version; Une Feuille enforces
// them uniformly so that behaviour is identical across backends (the error is
// returned before reaching any backend).
func ValidateForSave(m Memory) error {
	required := []struct {
		name  string
		value string
	}{
		{"id", m.ID},
		{"kind", m.Kind},
		{"source", m.Source},
		{"origin", m.Origin},
		{"schema_version", m.Labels.SchemaVersion},
	}
	for _, f := range required {
		if f.value == "" {
			return fmt.Errorf("missing required field %q: %w", f.name, ErrInvalidMemory)
		}
	}

	if m.ObservedAt.IsZero() {
		return fmt.Errorf("missing required field %q: %w", "observed_at", ErrInvalidMemory)
	}

	if len(m.Content) == 0 {
		return fmt.Errorf("missing required field %q: %w", "content", ErrInvalidMemory)
	}

	// Size check before JSON validity: oversized content is a distinct error.
	if len(m.Content) > MaxContentBytes {
		return ErrContentTooLarge
	}

	if !json.Valid(m.Content) {
		return fmt.Errorf("content is not valid JSON: %w", ErrInvalidMemory)
	}

	return nil
}

// NormaliseContent normalises a JSON payload using the same rule as the
// MILLEFEUILLE backend's content hashing: decode with UseNumber (preserving
// numeric tokens and resolving duplicate keys last-wins) then re-encode with
// json.Marshal (which sorts map keys lexicographically at every level and
// removes insignificant whitespace). Trailing data after the JSON value is an
// error. The returned bytes are a canonical form suitable for equality-based
// idempotency and conflict detection.
//
// Keeping this rule identical to MILLEFEUILLE ensures the in-memory fake and
// the adapter exhibit the same observable idempotency/conflict behaviour.
func NormaliseContent(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("normalise content: decode: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("normalise content: trailing data after JSON value")
	}

	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("normalise content: marshal: %w", err)
	}
	return out, nil
}

// ApplyDefaults fills empty label fields with their contract defaults. It
// returns a copy with defaults applied; it does not mutate the input.
func ApplyDefaults(m Memory) Memory {
	if m.Labels.Visibility == "" {
		m.Labels.Visibility = DefaultVisibility
	}
	if m.Labels.TrustLevel == "" {
		m.Labels.TrustLevel = DefaultTrustLevel
	}
	return m
}
