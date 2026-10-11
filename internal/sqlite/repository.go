package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/habuka036/millefeuille/internal/model"
)

// ObservationRepository is the internal persistence contract.
// All methods accept context.Context and surface ctx.Err() on cancellation.
type ObservationRepository interface {
	// Insert persists obs within a single transaction and returns a Receipt.
	//
	// Insert owns the full transaction lifecycle and all idempotency/conflict
	// logic:
	//   - New row inserted  → Receipt for the new row, nil error.
	//   - Duplicate event_id, same content_hash → stored Receipt, nil error (idempotent).
	//   - Duplicate event_id, different content_hash → Receipt{}, ErrConflict.
	Insert(ctx context.Context, obs model.Observation, contentHash string) (model.Receipt, error)

	// FindByEventID returns the Observation for eventID, or ErrNotFound if absent.
	FindByEventID(ctx context.Context, eventID string) (*model.Observation, error)

	// Query returns observations matching filter, ordered by received_at DESC,
	// event_id DESC, bounded by filter.Limit (after normalisation).
	// Returns ErrInvalidFilter if filter.Limit < 0.
	Query(ctx context.Context, filter model.Filter) ([]model.Observation, error)
}

// sqliteRepository implements ObservationRepository using *sql.DB.
type sqliteRepository struct {
	db *sql.DB
}

// NewRepository returns an ObservationRepository backed by db.
func NewRepository(db *sql.DB) ObservationRepository {
	return &sqliteRepository{db: db}
}

// ---- Insert ----------------------------------------------------------------

func (r *sqliteRepository) Insert(
	ctx context.Context,
	obs model.Observation,
	contentHash string,
) (model.Receipt, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Receipt{}, fmt.Errorf("repository.Insert: begin: %w", err)
	}

	provenanceJSON, err := json.Marshal(obs.Provenance)
	if err != nil {
		_ = tx.Rollback()
		return model.Receipt{}, fmt.Errorf("repository.Insert: marshal provenance: %w", err)
	}

	const insertSQL = `
INSERT INTO observations
  (event_id, schema_version, source_id, device_id, event_type,
   observed_at, received_at, payload, visibility, provenance, content_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(event_id) DO NOTHING`

	res, err := tx.ExecContext(ctx, insertSQL,
		obs.EventID,
		obs.SchemaVersion,
		obs.SourceID,
		obs.DeviceID,
		obs.EventType,
		formatTime(obs.ObservedAt),
		formatTime(obs.ReceivedAt),
		string(obs.Payload),
		obs.Visibility,
		string(provenanceJSON),
		contentHash,
	)
	if err != nil {
		_ = tx.Rollback()
		return model.Receipt{}, fmt.Errorf("repository.Insert: exec: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return model.Receipt{}, fmt.Errorf("repository.Insert: rows affected: %w", err)
	}

	if rowsAffected == 1 {
		// New row inserted successfully.
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return model.Receipt{}, fmt.Errorf("repository.Insert: commit: %w", err)
		}
		return model.Receipt{
			EventID:     obs.EventID,
			ReceivedAt:  obs.ReceivedAt,
			ContentHash: contentHash,
		}, nil
	}

	// Duplicate event_id: read stored hash and received_at for comparison.
	var storedHash, storedReceivedAtStr string
	err = tx.QueryRowContext(ctx,
		"SELECT content_hash, received_at FROM observations WHERE event_id = ?",
		obs.EventID,
	).Scan(&storedHash, &storedReceivedAtStr)
	if err != nil {
		_ = tx.Rollback()
		return model.Receipt{}, fmt.Errorf("repository.Insert: read stored: %w", err)
	}

	// Always rollback — nothing was inserted in this branch.
	if rbErr := tx.Rollback(); rbErr != nil {
		// Rollback after a read-only query path; discard error per design.
		_ = rbErr
	}

	if storedHash != contentHash {
		return model.Receipt{}, model.ErrConflict
	}

	// Idempotent success: return the original Receipt.
	storedReceivedAt, err := parseTime(storedReceivedAtStr)
	if err != nil {
		return model.Receipt{}, fmt.Errorf("repository.Insert: parse stored received_at: %w", err)
	}
	return model.Receipt{
		EventID:     obs.EventID,
		ReceivedAt:  storedReceivedAt,
		ContentHash: storedHash,
	}, nil
}

// ---- FindByEventID ---------------------------------------------------------

func (r *sqliteRepository) FindByEventID(
	ctx context.Context,
	eventID string,
) (*model.Observation, error) {
	const q = `
SELECT event_id, schema_version, source_id, device_id, event_type,
       observed_at, received_at, payload, visibility, provenance
FROM observations
WHERE event_id = ?`

	row := r.db.QueryRowContext(ctx, q, eventID)
	obs, err := scanObservation(row)
	if err == sql.ErrNoRows {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repository.FindByEventID: %w", err)
	}
	return obs, nil
}

// ---- Query -----------------------------------------------------------------

const (
	defaultLimit = 100
	maxLimit     = 1000
)

func (r *sqliteRepository) Query(
	ctx context.Context,
	filter model.Filter,
) ([]model.Observation, error) {
	if filter.Limit < 0 {
		return nil, model.ErrInvalidFilter
	}

	limit := filter.Limit
	switch {
	case limit == 0:
		limit = defaultLimit
	case limit > maxLimit:
		limit = maxLimit
	}

	// Build WHERE conditions and parameter list dynamically.
	// All user-supplied values are bound as parameters (no string interpolation).
	conditions := make([]string, 0, 4)
	args := make([]any, 0, 5)

	if filter.SourceID != "" {
		conditions = append(conditions, "source_id = ?")
		args = append(args, filter.SourceID)
	}
	if filter.EventType != "" {
		conditions = append(conditions, "event_type = ?")
		args = append(args, filter.EventType)
	}
	if !filter.From.IsZero() {
		conditions = append(conditions, "received_at >= ?")
		args = append(args, formatTime(filter.From))
	}
	if !filter.To.IsZero() {
		conditions = append(conditions, "received_at <= ?")
		args = append(args, formatTime(filter.To))
	}

	q := `SELECT event_id, schema_version, source_id, device_id, event_type,
       observed_at, received_at, payload, visibility, provenance
FROM observations`
	if len(conditions) > 0 {
		q += "\nWHERE "
		for i, c := range conditions {
			if i > 0 {
				q += " AND "
			}
			q += c
		}
	}
	q += "\nORDER BY received_at DESC, event_id DESC"
	q += "\nLIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("repository.Query: %w", err)
	}
	defer rows.Close()

	var result []model.Observation
	for rows.Next() {
		obs, err := scanObservation(rows)
		if err != nil {
			return nil, fmt.Errorf("repository.Query scan: %w", err)
		}
		result = append(result, *obs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository.Query rows: %w", err)
	}
	return result, nil
}

// ---- helpers ---------------------------------------------------------------

// scanner abstracts *sql.Row and *sql.Rows so scanObservation works for both.
type scanner interface {
	Scan(dest ...any) error
}

func scanObservation(s scanner) (*model.Observation, error) {
	var (
		observedAtStr string
		receivedAtStr string
		payloadStr    string
		provenanceStr string
	)
	obs := &model.Observation{}
	err := s.Scan(
		&obs.EventID,
		&obs.SchemaVersion,
		&obs.SourceID,
		&obs.DeviceID,
		&obs.EventType,
		&observedAtStr,
		&receivedAtStr,
		&payloadStr,
		&obs.Visibility,
		&provenanceStr,
	)
	if err != nil {
		return nil, err
	}

	obs.ObservedAt, err = parseTime(observedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse observed_at %q: %w", observedAtStr, err)
	}
	obs.ReceivedAt, err = parseTime(receivedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse received_at %q: %w", receivedAtStr, err)
	}
	obs.Payload = json.RawMessage(payloadStr)

	if err := json.Unmarshal([]byte(provenanceStr), &obs.Provenance); err != nil {
		return nil, fmt.Errorf("parse provenance %q: %w", provenanceStr, err)
	}
	return obs, nil
}

// formatTime serialises t to the fixed nanosecond-precision UTC format.
func formatTime(t time.Time) string {
	return t.UTC().Format(timestampFormat)
}

// parseTime deserialises s from the fixed nanosecond-precision UTC format.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timestampFormat, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
