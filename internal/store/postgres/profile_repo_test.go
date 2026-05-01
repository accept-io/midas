package postgres

// profile_repo_test.go — integration tests for ProfileRepo. Gated on
// DATABASE_URL via openTestDB; the suite skips when not set, mirroring
// the pattern used by every other Postgres repo test in this package.
//
// The first test pins Profile.Create against the real schema's CHECK
// constraints (escalation_mode, fail_mode, status, consequence_type,
// confidence). It exists because the apply path silently produced
// CHECK-violating rows for some time — the existing in-memory stub
// in internal/controlplane/apply/ never exercised the constraints.

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/value"
)

// TestProfileRepo_Create_PersistsAndRoundTrips inserts a fully-populated
// AuthorityProfile through ProfileRepo.Create and reads it back via
// FindByIDAndVersion. The assertion focuses on the fields that have
// CHECK constraints or NOT NULL clauses on authority_profiles —
// EscalationMode, FailMode, Status, ConsequenceThreshold, etc. —
// because those are what would reject a row at the storage layer.
//
// This test would have caught the chk_profiles_escalation_mode
// regression: an AuthorityProfile with EscalationMode == "" fails
// Create here.
func TestProfileRepo_Create_PersistsAndRoundTrips(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	repo, err := NewProfileRepo(db)
	if err != nil {
		t.Fatalf("NewProfileRepo: %v", err)
	}

	id := "profile-rt-escalation-mode"
	const ver = 1
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_profiles WHERE id = $1`, id)
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	effectiveFrom := now.Add(-time.Hour)

	want := &authority.AuthorityProfile{
		ID:                  id,
		Version:             ver,
		SurfaceID:           "surf-rt-escalation-mode",
		Name:                "Round-trip Test Profile",
		Description:         "round-trip test for chk_profiles_escalation_mode and friends",
		Status:              authority.ProfileStatusReview,
		EffectiveDate:       effectiveFrom,
		ConfidenceThreshold: 0.85,
		ConsequenceThreshold: authority.Consequence{
			// risk_rating is in both the domain enum and the schema CHECK
			// (chk_profiles_consequence_type allows risk_rating).
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		PolicyReference:     "rego://test/round-trip",
		EscalationMode:      authority.EscalationModeAuto,
		FailMode:            authority.FailModeClosed,
		RequiredContextKeys: []string{"customer_id", "request_amount"},
		CreatedAt:           now,
		UpdatedAt:           now,
		CreatedBy:           "test-roundtrip",
	}

	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindByIDAndVersion(ctx, id, ver)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil {
		t.Fatal("FindByIDAndVersion returned nil; expected the just-created profile")
	}

	// Core fields per the brief.
	if got.ID != want.ID {
		t.Errorf("ID: want %q, got %q", want.ID, got.ID)
	}
	if got.Version != want.Version {
		t.Errorf("Version: want %d, got %d", want.Version, got.Version)
	}
	if got.SurfaceID != want.SurfaceID {
		t.Errorf("SurfaceID: want %q, got %q", want.SurfaceID, got.SurfaceID)
	}
	if got.Status != want.Status {
		t.Errorf("Status: want %q, got %q", want.Status, got.Status)
	}
	if !got.EffectiveDate.Equal(want.EffectiveDate) {
		t.Errorf("EffectiveDate: want %s, got %s", want.EffectiveDate, got.EffectiveDate)
	}
	if got.ConfidenceThreshold != want.ConfidenceThreshold {
		t.Errorf("ConfidenceThreshold: want %v, got %v", want.ConfidenceThreshold, got.ConfidenceThreshold)
	}
	if got.ConsequenceThreshold.Type != want.ConsequenceThreshold.Type {
		t.Errorf("ConsequenceThreshold.Type: want %q, got %q",
			want.ConsequenceThreshold.Type, got.ConsequenceThreshold.Type)
	}
	if got.ConsequenceThreshold.RiskRating != want.ConsequenceThreshold.RiskRating {
		t.Errorf("ConsequenceThreshold.RiskRating: want %q, got %q",
			want.ConsequenceThreshold.RiskRating, got.ConsequenceThreshold.RiskRating)
	}
	if got.PolicyReference != want.PolicyReference {
		t.Errorf("PolicyReference: want %q, got %q", want.PolicyReference, got.PolicyReference)
	}
	if got.EscalationMode != want.EscalationMode {
		t.Errorf("EscalationMode: want %q, got %q (chk_profiles_escalation_mode)",
			want.EscalationMode, got.EscalationMode)
	}
	if got.FailMode != want.FailMode {
		t.Errorf("FailMode: want %q, got %q (chk_profiles_fail_mode)",
			want.FailMode, got.FailMode)
	}
	if got.CreatedBy != want.CreatedBy {
		t.Errorf("CreatedBy: want %q, got %q", want.CreatedBy, got.CreatedBy)
	}
}

// TestProfileRepo_Create_RejectsEmptyEscalationMode pins the regression
// directly. With an empty EscalationMode (the Go zero value) Postgres'
// chk_profiles_escalation_mode CHECK must reject the row. If this test
// stops failing on empty input, it means either the schema relaxed the
// check or the repo started defaulting on the storage side — either is a
// load-bearing change worth noticing.
func TestProfileRepo_Create_RejectsEmptyEscalationMode(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	repo, err := NewProfileRepo(db)
	if err != nil {
		t.Fatalf("NewProfileRepo: %v", err)
	}

	id := "profile-rt-empty-escalation"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_profiles WHERE id = $1`, id)
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	bad := &authority.AuthorityProfile{
		ID:                  id,
		Version:             1,
		SurfaceID:           "surf-rt-empty-escalation",
		Name:                "Empty Escalation Test",
		Status:              authority.ProfileStatusReview,
		EffectiveDate:       now.Add(-time.Hour),
		ConfidenceThreshold: 0.5,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingLow,
		},
		FailMode:  authority.FailModeClosed,
		CreatedAt: now,
		UpdatedAt: now,
		// EscalationMode intentionally left empty.
	}

	err = repo.Create(ctx, bad)
	if err == nil {
		t.Fatal("Create accepted an empty EscalationMode; expected chk_profiles_escalation_mode to reject")
	}
}
