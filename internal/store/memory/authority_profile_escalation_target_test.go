package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/value"
)

// TestProfileRepo_EscalationTargetID_RoundTrip pins the D31k-impl-1
// additive field round-trips through the in-memory profile repository
// via Create + Update + FindByID + FindByIDAndVersion.
func TestProfileRepo_EscalationTargetID_RoundTrip(t *testing.T) {
	repo := NewProfileRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &authority.AuthorityProfile{
		ID:                  "prof-et-rt",
		Version:             1,
		SurfaceID:           "surf-1",
		Name:                "profile with target",
		Status:              authority.ProfileStatusActive,
		EffectiveDate:       now,
		ConfidenceThreshold: 0.85,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingMedium,
		},
		EscalationMode:     authority.EscalationModeAuto,
		EscalationTargetID: "et-governance-approver",
		FailMode:           authority.FailModeClosed,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindByID(ctx, "prof-et-rt")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.EscalationTargetID != "et-governance-approver" {
		t.Errorf("EscalationTargetID round-trip: want %q, got %+v", "et-governance-approver", got)
	}

	got, _ = repo.FindByIDAndVersion(ctx, "prof-et-rt", 1)
	if got == nil || got.EscalationTargetID != "et-governance-approver" {
		t.Errorf("FindByIDAndVersion: want target, got %+v", got)
	}
}

// TestProfileRepo_EscalationTargetID_EmptyByDefault pins that the
// additive field is optional — profiles built without it carry the
// empty string, indicating no explicit target.
func TestProfileRepo_EscalationTargetID_EmptyByDefault(t *testing.T) {
	repo := NewProfileRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &authority.AuthorityProfile{
		ID: "prof-no-et", Version: 1, SurfaceID: "surf-1",
		Name: "profile without target", Status: authority.ProfileStatusActive,
		EffectiveDate: now, ConfidenceThreshold: 0.5,
		ConsequenceThreshold: authority.Consequence{Type: value.ConsequenceTypeRiskRating, RiskRating: value.RiskRatingMedium},
		EscalationMode:       authority.EscalationModeAuto,
		FailMode:             authority.FailModeOpen,
		CreatedAt:            now, UpdatedAt: now,
	}
	_ = repo.Create(ctx, p)
	got, _ := repo.FindByID(ctx, "prof-no-et")
	if got == nil {
		t.Fatal("FindByID returned nil")
	}
	if got.EscalationTargetID != "" {
		t.Errorf("EscalationTargetID must be empty by default; got %q", got.EscalationTargetID)
	}
}
