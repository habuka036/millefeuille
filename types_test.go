package millefeuille_test

import (
	"reflect"
	"testing"

	millefeuille "github.com/habuka036/millefeuille"
)

// TestInputObservationHasNoReceivedAt verifies at runtime that InputObservation
// does not contain a ReceivedAt field. If ReceivedAt is accidentally added, this
// test fails immediately.
func TestInputObservationHasNoReceivedAt(t *testing.T) {
	t.Helper()
	typ := reflect.TypeOf(millefeuille.InputObservation{})
	if _, ok := typ.FieldByName("ReceivedAt"); ok {
		t.Fatalf("InputObservation must not have a ReceivedAt field, but it does")
	}
}

// TestStoreInterfaceHasNoMutationMethods verifies that the Store interface does
// not expose Update or Delete methods, enforcing append-only semantics (REQ-006).
// This is a compile-time structural check: if a method named Update or Delete
// appears on Store, the assertion below fails.
func TestStoreInterfaceHasNoMutationMethods(t *testing.T) {
	t.Helper()
	typ := reflect.TypeOf((*millefeuille.Store)(nil)).Elem()
	for _, name := range []string{"Update", "Delete"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("Store interface must not expose a %q method (append-only semantics)", name)
		}
	}
}
