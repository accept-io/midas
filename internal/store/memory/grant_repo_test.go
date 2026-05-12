package memory

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/value"
)

// TestGrantRepo_Create_CapabilitiesAndConstraints_DeepCopy pins that
// the in-memory grant repository deep-copies the Capabilities and
// Constraints slices on Create, so subsequent mutation of the
// caller's slice does not race into the stored grant. The Postgres
// repo achieves the same property by round-tripping through JSONB;
// this test guards the memory-repo parity.
func TestGrantRepo_Create_CapabilitiesAndConstraints_DeepCopy(t *testing.T) {
	repo := NewGrantRepo()
	caps := []authority.Capability{authority.CapabilityRecommend, authority.CapabilityApprove}
	constraints := []authority.Constraint{
		{Kind: authority.ConstraintKindConfidenceThresholdMin, MinConfidence: 0.8},
		{Kind: authority.ConstraintKindHumanOnly},
	}
	g := &authority.AuthorityGrant{
		ID:            "g-1",
		AgentID:       "a-1",
		ProfileID:     "p-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
		Capabilities:  caps,
		Constraints:   constraints,
	}
	if err := repo.Create(context.Background(), g); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Mutate the caller's slices and verify the stored grant is
	// untouched.
	caps[0] = authority.CapabilityStop
	constraints[0].MinConfidence = 0.0

	got, err := repo.FindByID(context.Background(), "g-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Capabilities[0] != authority.CapabilityRecommend {
		t.Errorf("Capabilities[0]: caller mutation leaked into stored grant; got %q", got.Capabilities[0])
	}
	if got.Constraints[0].MinConfidence != 0.8 {
		t.Errorf("Constraints[0].MinConfidence: caller mutation leaked; got %v", got.Constraints[0].MinConfidence)
	}
}

// TestGrantRepo_RoundTrip_AllConstraintKinds pins the memory repo
// preserves every constraint Kind shape. Paired with the Postgres
// JSONB round-trip test in grant_repo_test.go, these together pin
// the memory↔Postgres parity for the D31i additions.
func TestGrantRepo_RoundTrip_AllConstraintKinds(t *testing.T) {
	repo := NewGrantRepo()
	winStart := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	winEnd := winStart.Add(8 * time.Hour)

	want := []authority.Constraint{
		{Kind: authority.ConstraintKindConfidenceThresholdMin, MinConfidence: 0.95},
		{Kind: authority.ConstraintKindConsequenceThresholdMax, MaxConsequence: authority.Consequence{
			Type: value.ConsequenceTypeMonetary, Amount: 5000, Currency: "EUR",
		}},
		{Kind: authority.ConstraintKindAIOnly},
		{Kind: authority.ConstraintKindTimeWindow, StartTime: winStart, EndTime: winEnd},
	}
	g := &authority.AuthorityGrant{
		ID:            "g-all-kinds",
		AgentID:       "a-1",
		ProfileID:     "p-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: winStart.Add(-time.Hour),
		Capabilities: []authority.Capability{
			authority.CapabilityRecommend, authority.CapabilityApprove,
			authority.CapabilityReject, authority.CapabilityEscalate,
			authority.CapabilityStop,
		},
		Constraints: want,
	}
	if err := repo.Create(context.Background(), g); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindByID(context.Background(), "g-all-kinds")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !reflect.DeepEqual(got.Capabilities, g.Capabilities) {
		t.Errorf("Capabilities mismatch:\nwant %#v\ngot  %#v", g.Capabilities, got.Capabilities)
	}
	if !reflect.DeepEqual(got.Constraints, want) {
		t.Errorf("Constraints mismatch:\nwant %#v\ngot  %#v", want, got.Constraints)
	}
}

// TestGrantRepo_Update_RewritesCapabilities pins that Update also
// rewrites Capabilities + Constraints (not just lifecycle fields).
func TestGrantRepo_Update_RewritesCapabilities(t *testing.T) {
	repo := NewGrantRepo()
	g := &authority.AuthorityGrant{
		ID:            "g-upd",
		AgentID:       "a-1",
		ProfileID:     "p-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
		Capabilities:  []authority.Capability{authority.CapabilityRecommend},
	}
	if err := repo.Create(context.Background(), g); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Build a NEW grant struct (not aliasing the original) and update.
	updated := &authority.AuthorityGrant{
		ID:            "g-upd",
		AgentID:       "a-1",
		ProfileID:     "p-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
		Capabilities:  []authority.Capability{authority.CapabilityApprove, authority.CapabilityStop},
		Constraints:   []authority.Constraint{{Kind: authority.ConstraintKindAIOnly}},
	}
	if err := repo.Update(context.Background(), updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.FindByID(context.Background(), "g-upd")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !reflect.DeepEqual(got.Capabilities, updated.Capabilities) {
		t.Errorf("Capabilities after Update: want %#v, got %#v", updated.Capabilities, got.Capabilities)
	}
	if !reflect.DeepEqual(got.Constraints, updated.Constraints) {
		t.Errorf("Constraints after Update: want %#v, got %#v", updated.Constraints, got.Constraints)
	}
}
