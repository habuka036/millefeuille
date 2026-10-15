package unefeuille

import "context"

// Store is the backend-independent memory API. All of Save, Get, and Search
// accept a context.Context and respect cancellation and deadlines. Close does
// not take a context because it releases resources and must not be interrupted
// mid-operation (NFR-002 applies to Save/Get/Search only).
type Store interface {
	// Save persists m and returns a SaveReceipt. It is idempotent: re-saving the
	// same ID with the same (normalised) content returns the original receipt
	// without creating a duplicate. Re-saving the same ID with different content
	// returns ErrConflict and does not modify the stored memory. Invalid input
	// returns ErrInvalidMemory, and oversized content returns ErrContentTooLarge.
	Save(ctx context.Context, m Memory) (SaveReceipt, error)

	// Get returns the Memory for id, or ErrNotFound if none exists.
	Get(ctx context.Context, id string) (Memory, error)

	// Search returns memories matching q, ordered by StoredAt descending with a
	// deterministic tie-break by ID descending for equal StoredAt, up to the
	// effective limit. No matches returns an empty slice and a nil error. A
	// negative Query.Limit returns ErrInvalidQuery.
	Search(ctx context.Context, q Query) ([]Memory, error)

	// Close releases resources held by the Store. It is safe to call multiple
	// times (idempotent).
	Close() error
}

// NormaliseLimit applies the common-contract limit rules: 0 -> DefaultLimit,
// values greater than MaxLimit -> MaxLimit. A negative limit is invalid and
// must be rejected with ErrInvalidQuery by the caller before calling this.
func NormaliseLimit(limit int) int {
	switch {
	case limit == 0:
		return DefaultLimit
	case limit > MaxLimit:
		return MaxLimit
	default:
		return limit
	}
}
