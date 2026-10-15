// Package unefeuille provides a backend-independent memory abstraction so that
// callers (such as NAVETTE and Imagorium) can save, get, and search memories
// through a common contract without depending on a specific memory backend.
//
// # Contract summary
//
//   - Save is append-only and idempotent: re-saving the same ID with the same
//     (normalised) content returns the original receipt; re-saving the same ID
//     with different content returns ErrConflict and does not overwrite.
//   - Required fields on Save: ID, Kind, Source, Origin, Labels.SchemaVersion,
//     ObservedAt, Content. Device is optional. Content must be valid JSON and at
//     most 1 MiB.
//   - Get returns ErrNotFound for an unknown ID.
//   - Search filters by neutral fields (Source, Kind, time range) and returns
//     results ordered by StoredAt descending with a deterministic ID-descending
//     tie-break. Limit 0 means the common-contract default of 100; values above
//     1000 are capped; a negative limit returns ErrInvalidQuery.
//   - All errors are errors.Is-comparable sentinels so callers can detect the
//     error kind regardless of backend.
//
// # Security notice (trusted-caller model)
//
// Une Feuille performs NO authentication or authorisation of its own. It is
// designed to be called from a trusted application layer.
//
//   - Labels.Visibility is a verbatim label, not an access control. Une Feuille
//     never filters results based on it. The hosting application must enforce
//     access control at its boundary.
//   - Labels.TrustLevel is stored as supplied (default "unverified") and is
//     never automatically inferred or upgraded.
//
// Do not expose a Store directly to untrusted callers.
package unefeuille
