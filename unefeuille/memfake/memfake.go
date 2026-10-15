// Package memfake provides an in-memory implementation of unefeuille.Store for
// contract and integration testing. It has no external dependencies and must
// not be used as a production store (it does not persist across process
// restarts and does not replace NAVETTE's SQLite persistence).
package memfake

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/habuka036/millefeuille/unefeuille"
)

// storedMemory holds a memory together with its normalised content, used for
// deterministic idempotency/conflict detection. The normalisation rule matches
// the MILLEFEUILLE backend (see unefeuille.NormaliseContent) so that the fake
// and the adapter exhibit the same observable behaviour.
type storedMemory struct {
	mem        unefeuille.Memory
	normalised string
}

// Fake is an in-memory unefeuille.Store.
type Fake struct {
	mu   sync.RWMutex
	data map[string]storedMemory
	// seq provides a strictly increasing commit time so StoredAt ordering is
	// deterministic even when wall-clock resolution would collide.
	seq int64
	// base is a fixed origin for synthesised commit times.
	base time.Time
}

// compile-time assertion.
var _ unefeuille.Store = (*Fake)(nil)

// New returns a fresh in-memory Store.
func New() *Fake {
	return &Fake{
		data: make(map[string]storedMemory),
		base: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// nextStoredAt returns a strictly increasing UTC timestamp. Caller holds mu.
func (f *Fake) nextStoredAt() time.Time {
	f.seq++
	return f.base.Add(time.Duration(f.seq) * time.Millisecond).UTC()
}

// Save implements unefeuille.Store.
func (f *Fake) Save(ctx context.Context, m unefeuille.Memory) (unefeuille.SaveReceipt, error) {
	if err := ctx.Err(); err != nil {
		return unefeuille.SaveReceipt{}, err
	}
	if err := unefeuille.ValidateForSave(m); err != nil {
		return unefeuille.SaveReceipt{}, err
	}

	norm, err := unefeuille.NormaliseContent(m.Content)
	if err != nil {
		// Content already passed json.Valid in ValidateForSave; a normalise
		// failure here means trailing data etc. Treat as invalid memory.
		return unefeuille.SaveReceipt{}, err
	}

	m = unefeuille.ApplyDefaults(m)
	m.ObservedAt = m.ObservedAt.UTC()

	f.mu.Lock()
	defer f.mu.Unlock()

	if existing, ok := f.data[m.ID]; ok {
		if existing.normalised == string(norm) {
			// Idempotent: return the original receipt.
			return unefeuille.SaveReceipt{ID: existing.mem.ID, StoredAt: existing.mem.StoredAt}, nil
		}
		return unefeuille.SaveReceipt{}, unefeuille.ErrConflict
	}

	m.StoredAt = f.nextStoredAt()
	f.data[m.ID] = storedMemory{mem: m, normalised: string(norm)}
	return unefeuille.SaveReceipt{ID: m.ID, StoredAt: m.StoredAt}, nil
}

// Get implements unefeuille.Store.
func (f *Fake) Get(ctx context.Context, id string) (unefeuille.Memory, error) {
	if err := ctx.Err(); err != nil {
		return unefeuille.Memory{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()

	s, ok := f.data[id]
	if !ok {
		return unefeuille.Memory{}, unefeuille.ErrNotFound
	}
	return s.mem, nil
}

// Search implements unefeuille.Store.
func (f *Fake) Search(ctx context.Context, q unefeuille.Query) ([]unefeuille.Memory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.Limit < 0 {
		return nil, unefeuille.ErrInvalidQuery
	}
	limit := unefeuille.NormaliseLimit(q.Limit)

	f.mu.RLock()
	matched := make([]unefeuille.Memory, 0, len(f.data))
	for _, s := range f.data {
		if q.Source != "" && s.mem.Source != q.Source {
			continue
		}
		if q.Kind != "" && s.mem.Kind != q.Kind {
			continue
		}
		if !q.From.IsZero() && s.mem.StoredAt.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && s.mem.StoredAt.After(q.To) {
			continue
		}
		matched = append(matched, s.mem)
	}
	f.mu.RUnlock()

	// Order by StoredAt descending, ID descending as a deterministic tie-break.
	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].StoredAt.Equal(matched[j].StoredAt) {
			return matched[i].StoredAt.After(matched[j].StoredAt)
		}
		return matched[i].ID > matched[j].ID
	})

	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

// Close implements unefeuille.Store. It is a safe, idempotent no-op.
func (f *Fake) Close() error {
	return nil
}
