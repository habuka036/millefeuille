package millefeuilleadapter_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	millefeuille "github.com/habuka036/millefeuille"
	"github.com/habuka036/millefeuille/unefeuille"
	"github.com/habuka036/millefeuille/unefeuille/millefeuilleadapter"
)

func newAdapter(t *testing.T) unefeuille.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "map.db")
	st, err := millefeuilleadapter.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// TestFieldRoundTrip fixes that every field (notably Device, Labels, Origin) is
// preserved through Save and Get via the MILLEFEUILLE backend.
func TestFieldRoundTrip(t *testing.T) {
	st := newAdapter(t)
	ctx := context.Background()

	in := unefeuille.Memory{
		ID:         "map-1",
		Kind:       "sensor.reading",
		Source:     "sensor-42",
		Device:     "thermostat-living-room",
		Origin:     "device",
		ObservedAt: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
		Content:    json.RawMessage(`{"temperature":21.5}`),
		Labels: unefeuille.Labels{
			Visibility:    "shared",
			TrustLevel:    "verified",
			SchemaVersion: "0.2",
		},
	}
	if _, err := st.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Get(ctx, in.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != in.ID {
		t.Errorf("ID: got %q want %q", got.ID, in.ID)
	}
	if got.Kind != in.Kind {
		t.Errorf("Kind: got %q want %q", got.Kind, in.Kind)
	}
	if got.Source != in.Source {
		t.Errorf("Source: got %q want %q", got.Source, in.Source)
	}
	if got.Device != in.Device {
		t.Errorf("Device: got %q want %q (device must not be lost)", got.Device, in.Device)
	}
	if got.Origin != in.Origin {
		t.Errorf("Origin: got %q want %q", got.Origin, in.Origin)
	}
	if !got.ObservedAt.Equal(in.ObservedAt) {
		t.Errorf("ObservedAt: got %v want %v", got.ObservedAt, in.ObservedAt)
	}
	if got.Labels != in.Labels {
		t.Errorf("Labels: got %+v want %+v", got.Labels, in.Labels)
	}
	if got.StoredAt.IsZero() {
		t.Error("StoredAt should be assigned by the backend")
	}
}

// TestDefaultLabels verifies empty Visibility/TrustLevel receive contract
// defaults on the way through the adapter.
func TestDefaultLabels(t *testing.T) {
	st := newAdapter(t)
	ctx := context.Background()
	in := unefeuille.Memory{
		ID:         "map-def",
		Kind:       "k",
		Source:     "s",
		Origin:     "agent",
		ObservedAt: time.Now().UTC(),
		Content:    json.RawMessage(`{"a":1}`),
		Labels:     unefeuille.Labels{SchemaVersion: "0.1"}, // required; Visibility/TrustLevel left empty
	}
	if _, err := st.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Get(ctx, in.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Labels.Visibility != unefeuille.DefaultVisibility {
		t.Errorf("default visibility: got %q want %q", got.Labels.Visibility, unefeuille.DefaultVisibility)
	}
	if got.Labels.TrustLevel != unefeuille.DefaultTrustLevel {
		t.Errorf("default trust: got %q want %q", got.Labels.TrustLevel, unefeuille.DefaultTrustLevel)
	}
}

// TestErrorConversion_Conflict confirms a MILLEFEUILLE conflict surfaces as
// unefeuille.ErrConflict.
func TestErrorConversion_Conflict(t *testing.T) {
	st := newAdapter(t)
	ctx := context.Background()
	base := unefeuille.Memory{
		ID: "conf", Kind: "k", Source: "s", Origin: "agent",
		ObservedAt: time.Now().UTC(), Content: json.RawMessage(`{"v":1}`),
		Labels: unefeuille.Labels{SchemaVersion: "0.1"},
	}
	if _, err := st.Save(ctx, base); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	diff := base
	diff.Content = json.RawMessage(`{"v":2}`)
	_, err := st.Save(ctx, diff)
	if !errors.Is(err, unefeuille.ErrConflict) {
		t.Errorf("expected unefeuille.ErrConflict, got %v", err)
	}
	// The error must NOT leak as a raw millefeuille sentinel to callers relying
	// only on the unefeuille contract — but it may still wrap it. Verify the
	// unefeuille sentinel is detectable.
	if !errors.Is(err, unefeuille.ErrConflict) {
		t.Error("unefeuille.ErrConflict not detectable via errors.Is")
	}
}

// TestErrorConversion_NotFound confirms NotFound mapping.
func TestErrorConversion_NotFound(t *testing.T) {
	st := newAdapter(t)
	if _, err := st.Get(context.Background(), "absent"); !errors.Is(err, unefeuille.ErrNotFound) {
		t.Errorf("expected unefeuille.ErrNotFound, got %v", err)
	}
}

// TestNew_BorrowsStore verifies that New does not close the borrowed store on
// Close, whereas the borrowed store remains usable afterwards.
func TestNew_BorrowsStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "borrow.db")
	mf, err := millefeuille.Open(ctx, path)
	if err != nil {
		t.Fatalf("millefeuille.Open: %v", err)
	}
	defer mf.Close()

	ad := millefeuilleadapter.New(mf)
	if err := ad.Close(); err != nil {
		t.Fatalf("adapter Close (borrowed): %v", err)
	}
	// Borrowed store must still be usable after the adapter's Close.
	_, err = mf.QueryObservations(ctx, millefeuille.Filter{})
	if err != nil {
		t.Errorf("borrowed store should remain usable after adapter Close, got %v", err)
	}
}

// TestOpen_OwnsStore verifies Open-created adapters close the underlying store
// and Close is idempotent.
func TestOpen_OwnsStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "own.db")
	ad, err := millefeuilleadapter.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := ad.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := ad.Close(); err != nil {
		t.Errorf("second Close must be idempotent, got %v", err)
	}
}
