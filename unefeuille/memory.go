// Package unefeuille defines a backend-independent memory abstraction for
// callers (such as NAVETTE and Imagorium) so they can save, get, and search
// memories through a common contract without depending on a specific memory
// backend.
//
// This package intentionally does NOT import the root millefeuille package.
// Backend-specific adapters (for example the millefeuilleadapter subpackage)
// depend on unefeuille, not the other way around, keeping the dependency
// direction one-way and the contract backend-independent.
//
// Security notice: Une Feuille performs no authentication or authorisation of
// its own. Labels such as Visibility are stored and returned verbatim and are
// NEVER used as access-control filters. TrustLevel is stored as supplied and is
// never automatically inferred or upgraded. Enforce access control at the
// hosting application boundary (trusted-caller model).
package unefeuille

import (
	"encoding/json"
	"time"
)

// Memory is a single unit of memory handled by Une Feuille. It is a
// backend-independent type; backend-specific representations are converted by
// adapters.
type Memory struct {
	// ID is caller-assigned. An empty ID is invalid input.
	ID string
	// Kind is the memory category (maps to MILLEFEUILLE event_type).
	Kind string
	// Source is the originator of the memory (maps to source_id).
	Source string
	// Device is the originating device (optional; maps to device_id).
	Device string
	// Origin is the provenance origin (required; maps to provenance.origin).
	Origin string
	// ObservedAt is the caller-supplied observation time (normalised to UTC).
	ObservedAt time.Time
	// StoredAt is the backend-assigned commit time. It is ignored on Save input
	// and populated on Get/Search results.
	StoredAt time.Time
	// Content is the memory payload: valid JSON, at most 1 MiB.
	Content json.RawMessage
	// Labels carries metadata labels (visibility, trust level, schema version).
	Labels Labels
}

// Labels carries metadata labels attached to a memory. These are NOT used for
// access control (SEC-001).
type Labels struct {
	// Visibility is a verbatim label. Default "private". Not used as a filter.
	Visibility string
	// TrustLevel is the caller's trust assessment. Default "unverified". Never
	// automatically upgraded or inferred (SEC-002).
	TrustLevel string
	// SchemaVersion is the schema version of Content (maps to schema_version).
	SchemaVersion string
}

// SaveReceipt is the result of a successful Save.
//
// The MVP public contract intentionally excludes any backend-specific content
// hash: callers must not depend on backend hash values. If observability needs
// arise, a backend-independent opaque identifier may be added in a
// backward-compatible way.
type SaveReceipt struct {
	ID       string
	StoredAt time.Time
}

// Query is a backend-independent, neutral metadata filter for Search. It uses
// neutral field names and does not expose MILLEFEUILLE-specific names.
type Query struct {
	// Source constrains by originator; empty means no constraint.
	Source string
	// Kind constrains by memory category; empty means no constraint.
	Kind string
	// From is the inclusive lower bound on StoredAt; zero means no bound.
	From time.Time
	// To is the inclusive upper bound on StoredAt; zero means no bound.
	To time.Time
	// Limit bounds the number of results: 0 means the common-contract default
	// of 100, values > 1000 are capped to 1000, and a negative value yields
	// ErrInvalidQuery.
	Limit int
}

// Defaults and bounds that form part of the Une Feuille common contract across
// all backends.
const (
	// DefaultLimit is applied when Query.Limit is 0.
	DefaultLimit = 100
	// MaxLimit caps Query.Limit values greater than it.
	MaxLimit = 1000
	// MaxContentBytes is the maximum allowed Content size: 1 MiB.
	MaxContentBytes = 1 << 20
)

// Default label values applied when the caller leaves them empty.
const (
	DefaultVisibility = "private"
	DefaultTrustLevel = "unverified"
)
