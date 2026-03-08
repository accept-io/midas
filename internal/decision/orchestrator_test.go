package decision_test

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// testRepos bundles all in-memory repos for a single test.
type testRepos struct {
	surfaces  *memory.SurfaceRepo
	profiles  *memory.ProfileRepo
	grants    *memory.GrantRepo
	agents    *memory.AgentRepo
	envelopes *memory.EnvelopeRepo
}

func newRepos() testRepos {
	return testRepos{
		surfaces:  memory.NewSurfaceRepo(),
		profiles:  memory.NewProfileRepo(),
		grants:    memory.NewGrantRepo(),
		agents:    memory.NewAgentRepo(),
		envelopes: memory.NewEnvelopeRepo(),
	}
}

func newOrchestrator(t *testing.T, r testRepos) *decision.Orchestrator {
	t.Helper()
	orch, err := decision.NewOrchestrator(
		r.surfaces, r.profiles, r.grants, r.agents, r.envelopes,
		policy.NoOpPolicyEvaluator{},
	)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	return orch
}

// seedActiveSurface adds an active surface with the given ID.
func seedActiveSurface(t *testing.T, r testRepos, id string) {
	t.Helper()
	err := r.surfaces.Create(context.Background(), &surface.DecisionSurface{
		ID:            id,
		Name:          "test surface",
		Status:        surface.SurfaceStatusActive,
		Version:       1,
		EffectiveDate: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed surface %q: %v", id, err)
	}
}

// seedAgent adds an agent with the given ID.
func seedAgent(t *testing.T, r testRepos, id string) {
	t.Helper()
	err := r.agents.Create(context.Background(), &agent.Agent{
		ID:               id,
		Name:             "test agent",
		Type:             agent.AgentTypeAI,
		OperationalState: agent.OperationalStateActive,
	})
	if err != nil {
		t.Fatalf("seed agent %q: %v", id, err)
	}
}

// seedProfile adds a profile with the given ID pointing to surfaceID.
// Default thresholds: confidence 0.8, consequence risk_rating/high.
func seedProfile(t *testing.T, r testRepos, id, surfaceID string) {
	t.Helper()
	err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  id,
		SurfaceID:           surfaceID,
		Name:                "test profile",
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
		t.Fatalf("seed profile %q: %v", id, err)
	}
}

// seedActiveGrant creates an active grant linking agentID to profileID.
func seedActiveGrant(t *testing.T, r testRepos, id, agentID, profileID string) {
	t.Helper()
	err := r.grants.Create(context.Background(), &authority.AuthorityGrant{
		ID:            id,
		AgentID:       agentID,
		ProfileID:     profileID,
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed grant %q: %v", id, err)
	}
}

// baseRequest returns a request that passes all default thresholds.
func baseRequest(surfaceID, agentID string) eval.DecisionRequest {
	return eval.DecisionRequest{
		SurfaceID:  surfaceID,
		AgentID:    agentID,
		Confidence: 0.9,
		Consequence: &eval.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingMedium,
		},
	}
}

func assertResult(t *testing.T, got decision.EvaluationResult, wantOutcome eval.Outcome, wantReason eval.ReasonCode) {
	t.Helper()
	if got.Outcome != wantOutcome {
		t.Errorf("outcome: got %q, want %q", got.Outcome, wantOutcome)
	}
	if got.ReasonCode != wantReason {
		t.Errorf("reason code: got %q, want %q", got.ReasonCode, wantReason)
	}
	if got.EnvelopeID == "" {
		t.Error("EnvelopeID must not be empty")
	}
}

// TestEvaluate_WithinAuthority covers the full happy path where all checks pass.
func TestEvaluate_WithinAuthority(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeExecute, eval.ReasonWithinAuthority)
}

// TestEvaluate_SurfaceNotFound covers a request where the surface ID is unknown.
func TestEvaluate_SurfaceNotFound(t *testing.T) {
	r := newRepos()
	// surface not seeded

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-missing", "agent-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceNotFound)
}

// TestEvaluate_SurfaceInactive covers a request against a surface that has been deactivated.
func TestEvaluate_SurfaceInactive(t *testing.T) {
	r := newRepos()
	if err := r.surfaces.Create(context.Background(), &surface.DecisionSurface{
		ID:     "surf-1",
		Name:   "retired surface",
		Status: surface.SurfaceStatusInactive,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceInactive)
}

// TestEvaluate_AgentNotFound covers a request where the agent ID is unknown.
func TestEvaluate_AgentNotFound(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	// agent not seeded

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-missing"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonAgentNotFound)
}

// TestEvaluate_NoActiveGrant covers an agent with no grants at all.
func TestEvaluate_NoActiveGrant(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	// no grants seeded for agent-1

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonNoActiveGrant)
}

// TestEvaluate_ProfileNotFound covers an agent with an active grant whose profile
// cannot be resolved (profile ID not present in the repository).
func TestEvaluate_ProfileNotFound(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	// grant references a profile that does not exist in the repo
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-missing")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonProfileNotFound)
}

// TestEvaluate_GrantProfileSurfaceMismatch covers an agent whose grant resolves to a
// profile that belongs to a different surface than the one being requested.
func TestEvaluate_GrantProfileSurfaceMismatch(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	// profile is registered against surf-2, not the requested surf-1
	seedProfile(t, r, "prof-1", "surf-2")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonGrantProfileSurfaceMismatch)
}

// TestEvaluate_InsufficientContext covers a request that is missing context keys
// declared as required by the authority profile.
func TestEvaluate_InsufficientContext(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	if err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  "prof-1",
		SurfaceID:           "surf-1",
		Name:                "contextual profile",
		ConfidenceThreshold: 0.8,
		RequiredContextKeys: []string{"transaction_id"},
		FailMode:            authority.FailModeOpen,
		Version:             1,
		EffectiveDate:       time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	req := baseRequest("surf-1", "agent-1")
	req.Context = map[string]any{} // transaction_id is required but not supplied

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeRequestClarification, eval.ReasonInsufficientContext)
}

// TestEvaluate_ConfidenceBelowThreshold covers a request whose confidence score
// falls below the profile's configured threshold.
func TestEvaluate_ConfidenceBelowThreshold(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1") // threshold = 0.8
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	req := baseRequest("surf-1", "agent-1")
	req.Confidence = 0.5 // below 0.8

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)
}

// TestEvaluate_ConsequenceExceedsLimit covers a request whose consequence severity
// exceeds the profile's configured threshold.
func TestEvaluate_ConsequenceExceedsLimit(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1") // threshold = risk_rating/high
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	req := baseRequest("surf-1", "agent-1")
	req.Consequence = &eval.Consequence{
		Type:       value.ConsequenceTypeRiskRating,
		RiskRating: value.RiskRatingCritical, // exceeds high
	}

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeEscalate, eval.ReasonConsequenceExceedsLimit)
}
