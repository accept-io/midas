package decision_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/surface"
)

// seedSurface creates a surface with the given status and stores it.
// Use this helper for lifecycle tests that require surfaces in specific states.
func seedSurface(t *testing.T, r testRepos, id string, status surface.SurfaceStatus) {
	t.Helper()

	now := time.Now()
	err := r.surfaces.Create(context.Background(), &surface.DecisionSurface{
		ID:             id,
		Name:           "lifecycle test surface",
		Status:         status,
		Version:        1,
		EffectiveFrom:  now.Add(-time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "tech@example.com",
		Domain:         "test",
	})
	if err != nil {
		t.Fatalf("seed surface %q with status %q: %v", id, status, err)
	}
}

// TestDecision_RejectsDraftSurface verifies that an evaluation request against
// a surface in draft status is rejected as SURFACE_INACTIVE.
//
// Draft surfaces are not yet submitted for governance review and must not
// participate in evaluation flows.
func TestDecision_RejectsDraftSurface(t *testing.T) {
	r := newRepos()
	seedSurface(t, r, "surf-draft", surface.SurfaceStatusDraft)

	req := baseRequest("surf-draft", "agent-1")
	result, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		req,
		json.RawMessage(`{"surface_id":"surf-draft","agent_id":"agent-1","confidence":0.9}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceInactive)
}

// TestDecision_RejectsReviewSurface verifies that an evaluation request against
// a surface in review status is rejected as SURFACE_INACTIVE.
//
// Surfaces in review have been submitted for governance approval but have not
// yet received it. They must not be usable for decisions until active.
func TestDecision_RejectsReviewSurface(t *testing.T) {
	r := newRepos()
	seedSurface(t, r, "surf-review", surface.SurfaceStatusReview)

	req := baseRequest("surf-review", "agent-1")
	result, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		req,
		json.RawMessage(`{"surface_id":"surf-review","agent_id":"agent-1","confidence":0.9}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceInactive)
}

// TestDecision_RejectsDeprecatedSurface verifies that an evaluation request
// against a deprecated surface is rejected as SURFACE_INACTIVE.
//
// Deprecated surfaces have been superseded. Evaluations against them are
// rejected to encourage migration to the successor surface.
func TestDecision_RejectsDeprecatedSurface(t *testing.T) {
	r := newRepos()
	seedSurface(t, r, "surf-deprecated", surface.SurfaceStatusDeprecated)

	req := baseRequest("surf-deprecated", "agent-1")
	result, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		req,
		json.RawMessage(`{"surface_id":"surf-deprecated","agent_id":"agent-1","confidence":0.9}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceInactive)
}

// TestDecision_RejectsRetiredSurface verifies that an evaluation request against
// a retired surface is rejected as SURFACE_INACTIVE. This test sits alongside
// the draft/review/deprecated cases to document the complete set.
func TestDecision_RejectsRetiredSurface(t *testing.T) {
	r := newRepos()
	seedSurface(t, r, "surf-retired", surface.SurfaceStatusRetired)

	req := baseRequest("surf-retired", "agent-1")
	result, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		req,
		json.RawMessage(`{"surface_id":"surf-retired","agent_id":"agent-1","confidence":0.9}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceInactive)
}

// TestDecision_AcceptsActiveSurface confirms the positive case: only a surface
// in active status succeeds through the surface resolution step.
func TestDecision_AcceptsActiveSurface(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-active")
	seedAgent(t, r, "agent-active")
	seedProfile(t, r, "prof-active", "surf-active")
	seedActiveGrant(t, r, "grant-active", "agent-active", "prof-active")

	req := baseRequest("surf-active", "agent-active")
	result, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		req,
		json.RawMessage(`{"surface_id":"surf-active","agent_id":"agent-active","confidence":0.9}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertResult(t, result, eval.OutcomeAccept, eval.ReasonWithinAuthority)
}

// TestDecision_AllNonActiveStatusesRejected is a table-driven test confirming
// that every non-active status produces SURFACE_INACTIVE. This is the
// authoritative boundary check for the decision runtime surface gate.
func TestDecision_AllNonActiveStatusesRejected(t *testing.T) {
	statuses := []surface.SurfaceStatus{
		surface.SurfaceStatusDraft,
		surface.SurfaceStatusReview,
		surface.SurfaceStatusDeprecated,
		surface.SurfaceStatusRetired,
	}

	for _, status := range statuses {
		status := status // capture for parallel subtests
		t.Run(string(status), func(t *testing.T) {
			r := newRepos()
			surfaceID := "surf-" + string(status)
			seedSurface(t, r, surfaceID, status)

			req := baseRequest(surfaceID, "agent-1")
			result, err := newOrchestrator(t, r).Evaluate(
				context.Background(),
				req,
				json.RawMessage(`{"surface_id":"`+surfaceID+`","agent_id":"agent-1","confidence":0.9}`),
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceInactive)
		})
	}
}
