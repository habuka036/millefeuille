package model

import "errors"

// Sentinel errors returned by repository and store operations. They are defined
// in internal/model so that internal/sqlite can return them without importing
// the root package. The root millefeuille package re-exports these variables to
// preserve the public API.
var (
	// ErrNotFound is returned when no observation exists for a given event_id.
	ErrNotFound = errors.New("observation not found")

	// ErrConflict is returned when an observation with the same event_id already
	// exists but its content differs from the new submission.
	ErrConflict = errors.New("conflicting observation for event_id")

	// ErrInvalidObservation is returned when a required field is missing, empty,
	// exceeds its maximum length, or the payload is not valid JSON.
	ErrInvalidObservation = errors.New("invalid observation")

	// ErrPayloadTooLarge is returned when the payload byte length exceeds 1 MiB.
	ErrPayloadTooLarge = errors.New("payload exceeds 1 MiB limit")

	// ErrInvalidFilter is returned when Filter.Limit is negative.
	ErrInvalidFilter = errors.New("invalid query filter")
)
