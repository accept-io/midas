package memory

import "testing"

// TestNewRepositories_FailModePolicies_NonNil pins that the memory aggregator
// constructs a non-nil FailModePolicies repository (D27j-impl-1a). The
// aggregator field is nil-safe (no runtime path consults it in this
// tranche), but tests that build a full Repositories via NewRepositories
// should always observe a working repository here.
func TestNewRepositories_FailModePolicies_NonNil(t *testing.T) {
	repos := NewRepositories()
	if repos == nil {
		t.Fatal("NewRepositories returned nil")
	}
	if repos.FailModePolicies == nil {
		t.Error("Repositories.FailModePolicies must be non-nil after NewRepositories")
	}
}
