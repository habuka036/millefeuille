// Package model defines the domain types for Millefeuille Stage 0.
// It has no dependencies on any other package in this module, allowing both the
// root millefeuille package and the internal/sqlite and internal/hash packages
// to reference these types without creating an import cycle.
package model

import (
	"encoding/json"
	"time"
)

// InputObservation is the caller-supplied input to AppendObservation.
// ReceivedAt is absent; the library assigns it after input validation passes.
type InputObservation struct {
	EventID       string
	SchemaVersion string
	SourceID      string
	DeviceID      string // optional; empty string if absent
	EventType     string
	ObservedAt    time.Time
	Payload       json.RawMessage // must be valid JSON; max 1 MiB
	Visibility    string          // optional; defaults to "private"
	Provenance    Provenance
}

// Observation is the stored and returned form; includes library-assigned fields.
type Observation struct {
	EventID       string
	SchemaVersion string
	SourceID      string
	DeviceID      string
	EventType     string
	ObservedAt    time.Time
	ReceivedAt    time.Time // library-assigned UTC timestamp
	Payload       json.RawMessage
	Visibility    string
	Provenance    Provenance
}

// Provenance records the origin and trust metadata of an observation.
type Provenance struct {
	Origin     string // "device" | "agent" | "simulator" | "external"
	TrustLevel string // caller's trust assessment; default "unverified"
}

// Receipt is returned by a successful AppendObservation call.
type Receipt struct {
	EventID     string
	ReceivedAt  time.Time
	ContentHash string // hex-encoded SHA-256 of the canonical content projection (excludes ReceivedAt)
}

// Filter specifies constraints for QueryObservations.
type Filter struct {
	SourceID  string
	EventType string
	From      time.Time // inclusive lower bound on ReceivedAt; zero = no bound
	To        time.Time // inclusive upper bound on ReceivedAt; zero = no bound
	Limit     int       // 0 = default 100; > 1000 = capped to 1000; negative = ErrInvalidFilter
}

// Page is the result type for QueryObservations.
type Page struct {
	Items      []Observation
	NextCursor string // always "" in M0.1; reserved for M0.2 cursor pagination
}
