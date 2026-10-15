// Package millefeuilleadapter adapts a MILLEFEUILLE v0.1.0 Store to the
// backend-independent unefeuille.Store contract. It converts between Une
// Feuille's own domain types and MILLEFEUILLE's observation types, and
// translates MILLEFEUILLE sentinel errors into unefeuille sentinels.
//
// This package depends on both unefeuille and the root millefeuille package.
// The dependency direction is one-way: unefeuille itself never imports
// millefeuille.
package millefeuilleadapter

import (
	"context"
	"errors"
	"fmt"

	millefeuille "github.com/habuka036/millefeuille"
	"github.com/habuka036/millefeuille/unefeuille"
)

// adapter implements unefeuille.Store backed by a millefeuille.Store.
type adapter struct {
	st millefeuille.Store
	// ownStore reports whether this adapter owns the lifecycle of st (created
	// via Open) and must therefore Close it. When false (created via New), st is
	// borrowed and Close must not close it.
	ownStore bool
	closed   bool
}

var _ unefeuille.Store = (*adapter)(nil)

// New wraps an existing millefeuille.Store. The caller retains ownership of st:
// the returned Store's Close does NOT close st (st is borrowed). Use this when
// the millefeuille.Store lifecycle is managed elsewhere.
func New(st millefeuille.Store) unefeuille.Store {
	return &adapter{st: st, ownStore: false}
}

// Open creates a millefeuille.Store at path and wraps it. The returned Store
// owns that millefeuille.Store: its Close closes the underlying store. Close is
// idempotent.
func Open(ctx context.Context, path string, opts ...millefeuille.Option) (unefeuille.Store, error) {
	st, err := millefeuille.Open(ctx, path, opts...)
	if err != nil {
		return nil, fmt.Errorf("millefeuilleadapter.Open: %w", err)
	}
	return &adapter{st: st, ownStore: true}, nil
}

// Save converts m to a millefeuille.InputObservation and appends it. Input is
// validated against the Une Feuille contract first so that behaviour (for
// example, Origin being required) is identical regardless of backend and errors
// are returned before reaching MILLEFEUILLE.
func (a *adapter) Save(ctx context.Context, m unefeuille.Memory) (unefeuille.SaveReceipt, error) {
	if err := ctx.Err(); err != nil {
		return unefeuille.SaveReceipt{}, err
	}
	if err := unefeuille.ValidateForSave(m); err != nil {
		return unefeuille.SaveReceipt{}, err
	}

	m = unefeuille.ApplyDefaults(m)

	in := millefeuille.InputObservation{
		EventID:       m.ID,
		SchemaVersion: m.Labels.SchemaVersion,
		SourceID:      m.Source,
		DeviceID:      m.Device,
		EventType:     m.Kind,
		ObservedAt:    m.ObservedAt,
		Payload:       m.Content,
		Visibility:    m.Labels.Visibility,
		Provenance: millefeuille.Provenance{
			Origin:     m.Origin,
			TrustLevel: m.Labels.TrustLevel,
		},
	}

	receipt, err := a.st.AppendObservation(ctx, in)
	if err != nil {
		return unefeuille.SaveReceipt{}, convertError(err)
	}
	return unefeuille.SaveReceipt{ID: receipt.EventID, StoredAt: receipt.ReceivedAt}, nil
}

// Get fetches an observation by id and converts it to a Memory.
func (a *adapter) Get(ctx context.Context, id string) (unefeuille.Memory, error) {
	if err := ctx.Err(); err != nil {
		return unefeuille.Memory{}, err
	}
	obs, err := a.st.GetObservation(ctx, id)
	if err != nil {
		return unefeuille.Memory{}, convertError(err)
	}
	return observationToMemory(obs), nil
}

// Search converts q to a millefeuille.Filter and converts the resulting page.
func (a *adapter) Search(ctx context.Context, q unefeuille.Query) ([]unefeuille.Memory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.Limit < 0 {
		// Return the unefeuille sentinel directly for the same observable
		// behaviour as the fake, without depending on backend validation.
		return nil, unefeuille.ErrInvalidQuery
	}
	filter := millefeuille.Filter{
		SourceID:  q.Source,
		EventType: q.Kind,
		From:      q.From,
		To:        q.To,
		Limit:     q.Limit,
	}
	page, err := a.st.QueryObservations(ctx, filter)
	if err != nil {
		return nil, convertError(err)
	}
	out := make([]unefeuille.Memory, 0, len(page.Items))
	for i := range page.Items {
		obs := page.Items[i]
		out = append(out, observationToMemory(&obs))
	}
	return out, nil
}

// Close releases the underlying store only if this adapter owns it (created via
// Open). It is idempotent.
func (a *adapter) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	if a.ownStore {
		return a.st.Close()
	}
	return nil
}

// observationToMemory maps a stored observation back to a Memory, preserving all
// fields including Device and Labels.
func observationToMemory(obs *millefeuille.Observation) unefeuille.Memory {
	return unefeuille.Memory{
		ID:         obs.EventID,
		Kind:       obs.EventType,
		Source:     obs.SourceID,
		Device:     obs.DeviceID,
		Origin:     obs.Provenance.Origin,
		ObservedAt: obs.ObservedAt,
		StoredAt:   obs.ReceivedAt,
		Content:    obs.Payload,
		Labels: unefeuille.Labels{
			Visibility:    obs.Visibility,
			TrustLevel:    obs.Provenance.TrustLevel,
			SchemaVersion: obs.SchemaVersion,
		},
	}
}

// convertError maps MILLEFEUILLE sentinel errors to unefeuille sentinels,
// preserving the original error's context via %w so callers can inspect either
// with errors.Is.
func convertError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, millefeuille.ErrNotFound):
		return fmt.Errorf("%w: %v", unefeuille.ErrNotFound, err)
	case errors.Is(err, millefeuille.ErrConflict):
		return fmt.Errorf("%w: %v", unefeuille.ErrConflict, err)
	case errors.Is(err, millefeuille.ErrInvalidObservation):
		return fmt.Errorf("%w: %v", unefeuille.ErrInvalidMemory, err)
	case errors.Is(err, millefeuille.ErrPayloadTooLarge):
		return fmt.Errorf("%w: %v", unefeuille.ErrContentTooLarge, err)
	case errors.Is(err, millefeuille.ErrInvalidFilter):
		return fmt.Errorf("%w: %v", unefeuille.ErrInvalidQuery, err)
	default:
		// Context cancellation and other errors pass through unchanged.
		return err
	}
}
