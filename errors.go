package millefeuille

import "github.com/habuka036/millefeuille/internal/model"

// Sentinel errors returned by Store operations. They are defined in
// internal/model (so internal packages can return them without importing the
// root package) and re-exported here to preserve the public API. Callers use
// errors.Is against these variables.
var (
	// ErrNotFound is returned by GetObservation when no observation exists for
	// the given event_id.
	ErrNotFound = model.ErrNotFound

	// ErrConflict is returned by AppendObservation when an observation with the
	// same event_id already exists but its content differs.
	ErrConflict = model.ErrConflict

	// ErrInvalidObservation is returned when a required field is missing, empty,
	// exceeds its maximum length, or the payload is not valid JSON.
	ErrInvalidObservation = model.ErrInvalidObservation

	// ErrPayloadTooLarge is returned when the payload byte length exceeds 1 MiB.
	ErrPayloadTooLarge = model.ErrPayloadTooLarge

	// ErrInvalidFilter is returned by QueryObservations when Filter.Limit is
	// negative.
	ErrInvalidFilter = model.ErrInvalidFilter
)
