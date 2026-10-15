package unefeuille_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/habuka036/millefeuille/unefeuille"
)

func validMemory() unefeuille.Memory {
	return unefeuille.Memory{
		ID:         "mem-1",
		Kind:       "navette.run.log",
		Source:     "navette",
		Origin:     "agent",
		ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Content:    []byte(`{"k":"v"}`),
		Labels:     unefeuille.Labels{SchemaVersion: "0.1"},
	}
}

func TestValidateForSave_RequiredFields(t *testing.T) {
	cases := map[string]func(m *unefeuille.Memory){
		"missing id":          func(m *unefeuille.Memory) { m.ID = "" },
		"missing kind":        func(m *unefeuille.Memory) { m.Kind = "" },
		"missing source":      func(m *unefeuille.Memory) { m.Source = "" },
		"missing origin":      func(m *unefeuille.Memory) { m.Origin = "" },
		"missing schema_ver":  func(m *unefeuille.Memory) { m.Labels.SchemaVersion = "" },
		"missing observed_at": func(m *unefeuille.Memory) { m.ObservedAt = time.Time{} },
		"missing content":     func(m *unefeuille.Memory) { m.Content = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := validMemory()
			mutate(&m)
			err := unefeuille.ValidateForSave(m)
			if !errors.Is(err, unefeuille.ErrInvalidMemory) {
				t.Errorf("expected ErrInvalidMemory, got %v", err)
			}
		})
	}
}

func TestValidateForSave_DeviceOptional(t *testing.T) {
	m := validMemory() // Device left empty
	if err := unefeuille.ValidateForSave(m); err != nil {
		t.Errorf("expected valid memory with empty Device, got %v", err)
	}
}

func TestValidateForSave_InvalidJSON(t *testing.T) {
	m := validMemory()
	m.Content = []byte(`{not json}`)
	if err := unefeuille.ValidateForSave(m); !errors.Is(err, unefeuille.ErrInvalidMemory) {
		t.Errorf("expected ErrInvalidMemory for invalid JSON, got %v", err)
	}
}

func TestValidateForSave_ContentTooLarge(t *testing.T) {
	m := validMemory()
	// Build a valid JSON string payload exceeding 1 MiB.
	big := make([]byte, unefeuille.MaxContentBytes+10)
	for i := range big {
		big[i] = 'a'
	}
	m.Content = []byte(`"` + string(big) + `"`)
	if err := unefeuille.ValidateForSave(m); !errors.Is(err, unefeuille.ErrContentTooLarge) {
		t.Errorf("expected ErrContentTooLarge, got %v", err)
	}
}

func TestNormaliseContent_WhitespaceAndKeyOrderEqual(t *testing.T) {
	a, err := unefeuille.NormaliseContent([]byte(`{"b":1,  "a": 2}`))
	if err != nil {
		t.Fatalf("normalise a: %v", err)
	}
	b, err := unefeuille.NormaliseContent([]byte(`{"a":2,"b":1}`))
	if err != nil {
		t.Fatalf("normalise b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("expected whitespace/key-order differences to normalise equal: %s vs %s", a, b)
	}
}

func TestNormaliseContent_DifferentValuesNotEqual(t *testing.T) {
	a, _ := unefeuille.NormaliseContent([]byte(`{"a":1}`))
	b, _ := unefeuille.NormaliseContent([]byte(`{"a":2}`))
	if bytes.Equal(a, b) {
		t.Error("expected different values to normalise unequal")
	}
}

func TestNormaliseContent_TrailingData(t *testing.T) {
	_, err := unefeuille.NormaliseContent([]byte(`{"a":1} garbage`))
	if err == nil {
		t.Error("expected error for trailing data after JSON value")
	}
}

func TestApplyDefaults(t *testing.T) {
	m := unefeuille.ApplyDefaults(validMemory())
	if m.Labels.Visibility != unefeuille.DefaultVisibility {
		t.Errorf("expected default visibility %q, got %q", unefeuille.DefaultVisibility, m.Labels.Visibility)
	}
	if m.Labels.TrustLevel != unefeuille.DefaultTrustLevel {
		t.Errorf("expected default trust %q, got %q", unefeuille.DefaultTrustLevel, m.Labels.TrustLevel)
	}
	// Non-empty values are preserved.
	m2 := validMemory()
	m2.Labels.Visibility = "shared"
	m2 = unefeuille.ApplyDefaults(m2)
	if m2.Labels.Visibility != "shared" {
		t.Errorf("expected preserved visibility, got %q", m2.Labels.Visibility)
	}
}
