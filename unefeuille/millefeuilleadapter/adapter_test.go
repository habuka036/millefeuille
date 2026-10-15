package millefeuilleadapter_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/habuka036/millefeuille/unefeuille"
	"github.com/habuka036/millefeuille/unefeuille/contracttest"
	"github.com/habuka036/millefeuille/unefeuille/millefeuilleadapter"
)

// TestAdapter_Contract applies the implementation-independent contract suite to
// the MILLEFEUILLE adapter backed by a real file-based SQLite database. This
// confirms the adapter and the in-memory fake exhibit the same observable
// behaviour.
func TestAdapter_Contract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T) unefeuille.Store {
		path := filepath.Join(t.TempDir(), "contract.db")
		st, err := millefeuilleadapter.Open(context.Background(), path)
		if err != nil {
			t.Fatalf("Open adapter: %v", err)
		}
		return st
	})
}
