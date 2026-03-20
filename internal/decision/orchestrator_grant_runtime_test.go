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

// TestGrantRuntime_SuspendedGrant_Rejected verifies that a suspended grant
// causes the orchestrator to reject evaluation with NO_ACTIVE_GRANT.
func TestGrantRuntime_SuspendedGrant_Rejected(t *testing.T) {
	r := newRepos()

	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")

	// Create a suspended grant
	err := r.grants.Create(context.Background(), &authority.AuthorityGrant{
		ID:            "grant-1",
		AgentID:       "agent-1",
		ProfileID:     "profile-1",
		Status:        authority.GrantStatusSuspended,
		EffectiveDate: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	orch := newOrchestrator(t, r)
	req := baseRequest("surface-1", "agent-1")

	result, err := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != eval.OutcomeReject {
		t.Errorf("expected Reject, got %s", result.Outcome)
	}
	// The orchestrator iterates grants and skips non-active ones.
	// With no active grants, it returns NO_ACTIVE_GRANT or PROFILE_NOT_FOUND.
	if result.ReasonCode != eval.ReasonNoActiveGrant && result.ReasonCode != eval.ReasonProfileNotFound {
		t.Errorf("expected NO_ACTIVE_GRANT or PROFILE_NOT_FOUND, got %s", result.ReasonCode)
	}
}

// TestGrantRuntime_RevokedGrant_Rejected verifies that a revoked grant
// causes the orchestrator to reject evaluation.
func TestGrantRuntime_RevokedGrant_Rejected(t *testing.T) {
	r := newRepos()

	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")

	now := time.Now()
	err := r.grants.Create(context.Background(), &authority.AuthorityGrant{
		ID:            "grant-1",
		AgentID:       "agent-1",
		ProfileID:     "profile-1",
		Status:        authority.GrantStatusRevoked,
		EffectiveDate: now.Add(-time.Hour),
		RevokedAt:     &now,
		RevokedBy:     "admin-1",
	})
	if err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	orch := newOrchestrator(t, r)
	req := baseRequest("surface-1", "agent-1")

	result, err := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != eval.OutcomeReject {
		t.Errorf("expected Reject, got %s", result.Outcome)
	}
}

// TestGrantRuntime_ExpiredGrant_StillAccepted documents that the orchestrator
// does NOT currently enforce temporal expiry on grants during evaluation.
// The orchestrator uses ListByAgent + status check, not FindActiveByAgentAndProfile.
// Temporal enforcement exists at the FindActiveByAgentAndProfile level (used by
// postgres queries) but not in the orchestrator's iteration logic. This is a
// known gap; a future change may add temporal enforcement to the orchestrator.
func TestGrantRuntime_ExpiredGrant_StillAccepted(t *testing.T) {
	r := newRepos()

	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")

	expired := time.Now().Add(-time.Minute)
	err := r.grants.Create(context.Background(), &authority.AuthorityGrant{
		ID:            "grant-1",
		AgentID:       "agent-1",
		ProfileID:     "profile-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
		ExpiresAt:     &expired,
	})
	if err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	orch := newOrchestrator(t, r)
	req := baseRequest("surface-1", "agent-1")

	result, err := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Current behavior: expired grants with status=active are still accepted
	// because the orchestrator only checks g.Status, not temporal scope.
	if result.Outcome != eval.OutcomeAccept {
		t.Errorf("expected Accept (current behavior), got %s", result.Outcome)
	}
}

// TestGrantRuntime_ActiveGrant_Succeeds verifies that an active grant
// allows evaluation to proceed (and succeed if thresholds are met).
func TestGrantRuntime_ActiveGrant_Succeeds(t *testing.T) {
	r := newRepos()

	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-1")

	orch := newOrchestrator(t, r)
	req := baseRequest("surface-1", "agent-1")

	result, err := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != eval.OutcomeAccept {
		t.Errorf("expected Accept, got %s (reason: %s)", result.Outcome, result.ReasonCode)
	}
}

// TestGrantRuntime_ReinstatedGrant_Succeeds verifies that a reinstated grant
// (status=active after having been suspended) is treated as active.
func TestGrantRuntime_ReinstatedGrant_Succeeds(t *testing.T) {
	r := newRepos()

	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")

	// Create a grant, suspend it, then reinstate it.
	err := r.grants.Create(context.Background(), &authority.AuthorityGrant{
		ID:            "grant-1",
		AgentID:       "agent-1",
		ProfileID:     "profile-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	_ = r.grants.Suspend(context.Background(), "grant-1")
	_ = r.grants.Reactivate(context.Background(), "grant-1")

	orch := newOrchestrator(t, r)
	req := eval.DecisionRequest{
		RequestSource: "test-source",
		SurfaceID:     "surface-1",
		AgentID:       "agent-1",
		Confidence:    0.9,
		Consequence: &eval.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingMedium,
		},
	}

	result, err := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != eval.OutcomeAccept {
		t.Errorf("expected Accept for reinstated grant, got %s (reason: %s)", result.Outcome, result.ReasonCode)
	}
}
