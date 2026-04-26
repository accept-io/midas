package decision_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newOrchestratorWithPolicy(t *testing.T, r testRepos, pe policy.PolicyEvaluator) *decision.Orchestrator {
	t.Helper()
	memStore := memory.NewStoreWithRepositories(&store.Repositories{
		Surfaces:                    r.surfaces,
		Agents:                      r.agents,
		Profiles:                    r.profiles,
		Grants:                      r.grants,
		Envelopes:                   r.envelopes,
		Audit:                       r.audit,
		Processes:                   r.processes,
		BusinessServices:            r.businessServices,
		BusinessServiceCapabilities: r.bscLinks,
		Capabilities:                r.capabilities,
	})
	orch, err := decision.NewOrchestrator(memStore, pe, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	return orch
}

func rawPayload(t *testing.T) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{"test": true})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// PolicyMode field
// ---------------------------------------------------------------------------

func TestEvaluate_PolicyMode_IsNoop_WhenNoOpEvaluatorUsed(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	orch := newOrchestratorWithPolicy(t, r, policy.NoOpPolicyEvaluator{})
	result, err := orch.Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.PolicyMode != policy.PolicyModeNoop {
		t.Errorf("PolicyMode: want %q, got %q", policy.PolicyModeNoop, result.PolicyMode)
	}
}

func TestEvaluate_PolicyMode_IsUnknown_WhenEvaluatorDoesNotImplementPolicyModer(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	orch := newOrchestratorWithPolicy(t, r, &anonymousPolicyEvaluator{})
	result, err := orch.Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.PolicyMode != policy.PolicyModeUnknown {
		t.Errorf("PolicyMode: want %q, got %q", policy.PolicyModeUnknown, result.PolicyMode)
	}
}

// anonymousPolicyEvaluator implements PolicyEvaluator but NOT PolicyModer.
// Used to verify the "unknown" fallback path.
type anonymousPolicyEvaluator struct{}

func (anonymousPolicyEvaluator) Evaluate(_ context.Context, _ policy.PolicyInput) (policy.PolicyResult, error) {
	return policy.PolicyResult{Allowed: true, Reason: "anonymous"}, nil
}

// ---------------------------------------------------------------------------
// PolicySkipped field
// ---------------------------------------------------------------------------

func TestEvaluate_PolicySkipped_True_WhenProfileHasPolicyRef_AndNoopActive(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfileWithPolicy(t, r, "prof-1", "surf-1", "rego://payments/limits")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	orch := newOrchestratorWithPolicy(t, r, policy.NoOpPolicyEvaluator{})
	result, err := orch.Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !result.PolicySkipped {
		t.Error("PolicySkipped: want true when profile has policy_ref and noop is active")
	}
}

func TestEvaluate_PolicySkipped_False_WhenProfileHasNoPolicyRef(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1") // no policy_ref
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	orch := newOrchestratorWithPolicy(t, r, policy.NoOpPolicyEvaluator{})
	result, err := orch.Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.PolicySkipped {
		t.Error("PolicySkipped: want false when profile has no policy_ref")
	}
}

func TestEvaluate_PolicySkipped_False_WhenNonNoopEvaluator(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfileWithPolicy(t, r, "prof-1", "surf-1", "rego://payments/limits")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	// anonymousPolicyEvaluator does not implement PolicyModer → mode "unknown"
	orch := newOrchestratorWithPolicy(t, r, &anonymousPolicyEvaluator{})
	result, err := orch.Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// PolicySkipped is only true for noop mode; "unknown" mode should be false.
	if result.PolicySkipped {
		t.Error("PolicySkipped: want false when evaluator mode is not noop")
	}
}

// ---------------------------------------------------------------------------
// PolicyReference field
// ---------------------------------------------------------------------------

func TestEvaluate_PolicyReference_EchoesProfileRef_WhenSet(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfileWithPolicy(t, r, "prof-1", "surf-1", "rego://payments/limits")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	orch := newOrchestratorWithPolicy(t, r, policy.NoOpPolicyEvaluator{})
	result, err := orch.Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	const wantRef = "rego://payments/limits"
	if result.PolicyReference != wantRef {
		t.Errorf("PolicyReference: want %q, got %q", wantRef, result.PolicyReference)
	}
}

func TestEvaluate_PolicyReference_Empty_WhenProfileHasNoPolicyRef(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	orch := newOrchestratorWithPolicy(t, r, policy.NoOpPolicyEvaluator{})
	result, err := orch.Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.PolicyReference != "" {
		t.Errorf("PolicyReference: want empty, got %q", result.PolicyReference)
	}
}

// ---------------------------------------------------------------------------
// Early-exit paths still carry PolicyMode
// ---------------------------------------------------------------------------

func TestEvaluate_PolicyMode_PresentOnEarlyExit_SurfaceNotFound(t *testing.T) {
	r := newRepos()
	// No surface seeded — evaluation will exit early with SURFACE_NOT_FOUND.

	orch := newOrchestratorWithPolicy(t, r, policy.NoOpPolicyEvaluator{})
	result, err := orch.Evaluate(context.Background(), baseRequest("nonexistent-surf", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.PolicyMode != policy.PolicyModeNoop {
		t.Errorf("PolicyMode: want %q on early exit, got %q", policy.PolicyModeNoop, result.PolicyMode)
	}
	if result.Outcome != eval.OutcomeReject {
		t.Errorf("Outcome: want Reject, got %q", result.Outcome)
	}
}
