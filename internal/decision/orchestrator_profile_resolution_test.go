package decision_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

// TestEvaluate_ProfileInReview_RejectsWithProfileNotFound verifies that a profile
// in review status is not resolvable at evaluation time. The orchestrator's
// FindActiveAt query filters by status=active, so a review-state profile produces
// PROFILE_NOT_FOUND.
func TestEvaluate_ProfileInReview_RejectsWithProfileNotFound(t *testing.T) {
	r := newRepos()
	orch := newOrchestrator(t, r)

	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")

	// Seed profile in review status (not active)
	err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  "profile-review",
		SurfaceID:           "surface-1",
		Name:                "review profile",
		Status:              authority.ProfileStatusReview,
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		FailMode:      authority.FailModeOpen,
		Version:       1,
		EffectiveDate: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-review")

	req := baseRequest("surface-1", "agent-1")
	result, err := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	assertResult(t, result, eval.OutcomeReject, eval.ReasonProfileNotFound)
}

// TestEvaluate_ProfileDeprecated_RejectsWithProfileNotFound verifies that a
// deprecated profile is not resolvable at evaluation time.
func TestEvaluate_ProfileDeprecated_RejectsWithProfileNotFound(t *testing.T) {
	r := newRepos()
	orch := newOrchestrator(t, r)

	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")

	err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  "profile-deprecated",
		SurfaceID:           "surface-1",
		Name:                "deprecated profile",
		Status:              authority.ProfileStatusDeprecated,
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		FailMode:      authority.FailModeOpen,
		Version:       1,
		EffectiveDate: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-deprecated")

	req := baseRequest("surface-1", "agent-1")
	result, err := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	assertResult(t, result, eval.OutcomeReject, eval.ReasonProfileNotFound)
}

// TestEvaluate_ProfileActive_Succeeds verifies that an active profile with
// a valid effective window is resolvable and produces a successful evaluation.
func TestEvaluate_ProfileActive_Succeeds(t *testing.T) {
	r := newRepos()
	orch := newOrchestrator(t, r)

	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-active", "surface-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-active")

	req := baseRequest("surface-1", "agent-1")
	result, err := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	assertResult(t, result, eval.OutcomeAccept, eval.ReasonWithinAuthority)
}
