package unefeuille

import "errors"

// Sentinel errors returned by Store implementations. Callers use errors.Is
// against these variables. Backend adapters convert backend-specific errors
// into these so callers can判定 the error kind regardless of backend.
var (
	// ErrNotFound is returned by Get when no memory exists for the given ID.
	ErrNotFound = errors.New("unefeuille: memory not found")

	// ErrConflict is returned by Save when a memory with the same ID already
	// exists but its (normalised) content differs.
	ErrConflict = errors.New("unefeuille: id exists with different content")

	// ErrInvalidMemory is returned by Save when a required field is missing or
	// the content is not valid JSON.
	ErrInvalidMemory = errors.New("unefeuille: invalid memory")

	// ErrContentTooLarge is returned by Save when Content exceeds MaxContentBytes.
	ErrContentTooLarge = errors.New("unefeuille: content exceeds 1 MiB")

	// ErrInvalidQuery is returned by Search when Query.Limit is negative.
	ErrInvalidQuery = errors.New("unefeuille: invalid query")
)
