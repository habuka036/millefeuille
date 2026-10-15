package memfake_test

import (
	"testing"

	"github.com/habuka036/millefeuille/unefeuille"
	"github.com/habuka036/millefeuille/unefeuille/contracttest"
	"github.com/habuka036/millefeuille/unefeuille/memfake"
)

// TestFake_Contract applies the implementation-independent contract suite to the
// in-memory fake, confirming it satisfies the Une Feuille Store contract.
func TestFake_Contract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T) unefeuille.Store {
		return memfake.New()
	})
}
