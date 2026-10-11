// Package millefeuille provides a policy-aware, multi-layered memory storage
// library for evolving AI agents. Stage 0 (M0.1) implements durable observation
// storage backed by SQLite.
package millefeuille

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/habuka036/millefeuille/internal/sqlite"
	_ "modernc.org/sqlite"
)

// Store is the public API for durable observation storage.
// All methods respect context cancellation and deadlines.
//
// Security notice: Millefeuille performs no authentication or authorisation.
// It must be called from a trusted application layer (e.g., IMAGO Agent with
// Guardian authorization). See README for full security guidance.
type Store interface {
	// AppendObservation persists obs to durable storage after input validation
	// and returns a Receipt. Idempotent for identical re-submissions; returns
	// ErrConflict if the same event_id is re-submitted with different content.
	AppendObservation(ctx context.Context, obs InputObservation) (Receipt, error)

	// GetObservation returns the Observation for the given eventID.
	// Returns ErrNotFound (via errors.Is) if no such observation exists.
	GetObservation(ctx context.Context, eventID string) (*Observation, error)

	// QueryObservations returns a bounded, ordered page of observations matching
	// filter. Returns ErrInvalidFilter if filter.Limit is negative.
	QueryObservations(ctx context.Context, filter Filter) (Page, error)

	// Close releases all resources held by the Store.
	Close() error
}

// Option is a functional option for Open.
type Option func(*storeConfig)

// storeConfig holds configuration assembled from Options before Open returns.
type storeConfig struct {
	clock ClockFunc
}

// WithClock injects a custom clock function. Used in tests for deterministic
// timestamps. Production code uses the default (time.Now).
func WithClock(fn ClockFunc) Option {
	return func(c *storeConfig) {
		c.clock = fn
	}
}

// store is the concrete implementation of Store backed by SQLite.
type store struct {
	db    *sql.DB
	repo  sqlite.ObservationRepository
	clock ClockFunc
}

// compile-time assertion that *store satisfies Store.
var _ Store = (*store)(nil)

// Open constructs a Store backed by a file-based SQLite database at the given
// path. It applies connection PRAGMAs via the DSN, runs pending migrations, and
// verifies WAL mode before returning.
//
// In-memory SQLite databases (":memory:" or URIs with mode=memory) are not
// supported and cause Open to return an error. On any initialisation failure,
// the underlying database connection is closed before Open returns, so no
// connection leaks.
func Open(ctx context.Context, path string, opts ...Option) (Store, error) {
	if err := rejectInMemory(path); err != nil {
		return nil, err
	}

	cfg := storeConfig{clock: time.Now}
	for _, opt := range opts {
		opt(&cfg)
	}

	dsn := buildDSN(path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("millefeuille.Open: sql.Open: %w", err)
	}

	// Single logical writer; serialise all DB access within the process.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	// From here on, any failure must close db to avoid a connection leak.
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("millefeuille.Open: ping: %w", err)
	}

	if err := verifyWAL(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := runMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("millefeuille.Open: migrate: %w", err)
	}

	return &store{
		db:    db,
		repo:  sqlite.NewRepository(db),
		clock: cfg.clock,
	}, nil
}

// runMigrations applies pending schema migrations. It is a package-level variable
// so tests can inject a failing migration to verify that Open closes the database
// connection on failure (no connection leak). Production always uses the real
// migrator.
var runMigrations = func(ctx context.Context, db *sql.DB) error {
	return sqlite.NewMigrator(db).Migrate(ctx)
}

// Close releases the database connection. It does not force a WAL checkpoint;
// SQLite re-opens the database (including any un-checkpointed WAL pages)
// correctly on the next Open.
func (s *store) Close() error {
	return s.db.Close()
}

// rejectInMemory returns an error if path denotes an in-memory SQLite database.
func rejectInMemory(path string) error {
	lower := strings.ToLower(path)
	if lower == ":memory:" || strings.Contains(lower, ":memory:") {
		return fmt.Errorf("millefeuille.Open: in-memory databases are not supported (path %q)", path)
	}
	if strings.Contains(lower, "mode=memory") {
		return fmt.Errorf("millefeuille.Open: in-memory databases are not supported (path %q)", path)
	}
	return nil
}

// buildDSN constructs a modernc.org/sqlite DSN with the required PRAGMAs applied
// to every connection via _pragma query parameters.
func buildDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(ON)")
	return "file:" + path + "?" + q.Encode()
}

// verifyWAL confirms the database is operating in WAL journal mode.
func verifyWAL(ctx context.Context, db *sql.DB) error {
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("millefeuille.Open: query journal_mode: %w", err)
	}
	if strings.ToLower(mode) != "wal" {
		return fmt.Errorf("millefeuille.Open: expected WAL journal mode, got %q", mode)
	}
	return nil
}
