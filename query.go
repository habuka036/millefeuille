package millefeuille

import "context"

// GetObservation returns the Observation for eventID. It returns ErrNotFound
// (detectable with errors.Is) if no such observation exists, or a context error
// if ctx is cancelled.
func (s *store) GetObservation(ctx context.Context, eventID string) (*Observation, error) {
	return s.repo.FindByEventID(ctx, eventID)
}

// QueryObservations returns a bounded, ordered page of observations matching
// filter. It returns ErrInvalidFilter if filter.Limit is negative. The limit is
// normalised (0 → 100, > 1000 → 1000) and results are ordered by
// received_at DESC, event_id DESC. NextCursor is always empty in M0.1.
func (s *store) QueryObservations(ctx context.Context, filter Filter) (Page, error) {
	items, err := s.repo.Query(ctx, filter)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, NextCursor: ""}, nil
}
