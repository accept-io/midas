package decision_test

// orchestrator_failmode_deployment_default_test.go — D29d Part B.
// Pins that when the orchestrator carries a non-empty
// deploymentDefaultPolicyID (via WithFailModeDeploymentDefaultPolicyID),
// failmode.ResolveWithPath sees that id and the resolver records a
// Source=deployment_default EffectiveFailModePolicy on evaluations
// where neither the Surface nor its BusinessService declares an
// override. The runtime resolver is evidence-only — outcome must
// remain Accept.
//
// Negative pin: with an empty deploymentDefaultPolicyID (the default
// for any orchestrator constructed without the option) and no
// surface or BS override, EffectiveFailModePolicy is nil. This
// matches the pre-D29d behaviour and guards against accidental
// runtime leakage of the new field.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
)

// TestEvaluate_FailModeResolver_DeploymentDefault pins that
// WithFailModeDeploymentDefaultPolicyID drives the level-3 resolution
// path of failmode.ResolveWithPath. The configured id must resolve to
// an active policy; the resolver attaches an EffectiveFailModePolicy
// with Source=deployment_default and Outcome remains Accept.
func TestEvaluate_FailModeResolver_DeploymentDefault(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-deploy")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-deploy")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-deployment", 1, failmode.FailModePolicyStatusActive)

	orch := newOrchestrator(t, r).WithFailModeDeploymentDefaultPolicyID("fmp-deployment")
	res, err := orch.Evaluate(context.Background(),
		baseRequest("surf-fmp-deploy", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be unaffected by resolver; got %q", res.Outcome)
	}
	if res.EffectiveFailModePolicy == nil {
		t.Fatal("EffectiveFailModePolicy must be populated when deployment default resolves")
	}
	if res.EffectiveFailModePolicy.PolicyID != "fmp-deployment" ||
		res.EffectiveFailModePolicy.Version != 1 ||
		res.EffectiveFailModePolicy.Source != failmode.ResolutionSourceDeploymentDefault {
		t.Errorf("unexpected resolved policy: %+v", res.EffectiveFailModePolicy)
	}
}

// TestEvaluate_FailModeResolver_DeploymentDefaultEmptyKeepsLegacyBehaviour
// pins that without the new option (or with an explicit empty value)
// the resolver does NOT invent a deployment default. The pre-D29d
// outcome is byte-identical: EffectiveFailModePolicy is nil.
func TestEvaluate_FailModeResolver_DeploymentDefaultEmptyKeepsLegacyBehaviour(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-deploy-empty")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-deploy-empty")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	// Seed a policy that COULD match the deployment default, but do
	// not wire the option. The resolver must not invent the link.
	seedFailModePolicyForResolver(t, r, "fmp-deployment", 1, failmode.FailModePolicyStatusActive)

	orch := newOrchestrator(t, r) // no WithFailModeDeploymentDefaultPolicyID
	res, err := orch.Evaluate(context.Background(),
		baseRequest("surf-fmp-deploy-empty", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be Accept; got %q", res.Outcome)
	}
	if res.EffectiveFailModePolicy != nil {
		t.Errorf("EffectiveFailModePolicy must be nil without deployment-default wiring; got %+v",
			res.EffectiveFailModePolicy)
	}
}

// TestEvaluate_FailModeResolver_DeploymentDefaultLosesToSurface pins
// the resolution hierarchy when both a deployment default and a
// surface override are configured. The surface override wins.
func TestEvaluate_FailModeResolver_DeploymentDefaultLosesToSurface(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-both")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-both")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-deployment", 1, failmode.FailModePolicyStatusActive)
	seedFailModePolicyForResolver(t, r, "fmp-surface", 1, failmode.FailModePolicyStatusActive)
	setSurfaceFailModePolicyID(t, r, "surf-fmp-both", "fmp-surface")

	orch := newOrchestrator(t, r).WithFailModeDeploymentDefaultPolicyID("fmp-deployment")
	res, err := orch.Evaluate(context.Background(),
		baseRequest("surf-fmp-both", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.EffectiveFailModePolicy == nil {
		t.Fatal("EffectiveFailModePolicy must be populated from surface override")
	}
	if res.EffectiveFailModePolicy.PolicyID != "fmp-surface" ||
		res.EffectiveFailModePolicy.Source != failmode.ResolutionSourceSurface {
		t.Errorf("expected surface override to win; got %+v", res.EffectiveFailModePolicy)
	}
}

// TestEvaluate_FailModeResolver_DeploymentDefaultLosesToBusinessService
// pins that BS-default beats deployment-default in the resolution
// hierarchy.
func TestEvaluate_FailModeResolver_DeploymentDefaultLosesToBusinessService(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-bs-vs-deploy")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-bs-vs-deploy")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	seedFailModePolicyForResolver(t, r, "fmp-deployment", 1, failmode.FailModePolicyStatusActive)
	seedFailModePolicyForResolver(t, r, "fmp-bs-default", 1, failmode.FailModePolicyStatusActive)
	setBusinessServiceFailModePolicyID(t, r, "bs-test", "fmp-bs-default")

	orch := newOrchestrator(t, r).WithFailModeDeploymentDefaultPolicyID("fmp-deployment")
	res, err := orch.Evaluate(context.Background(),
		baseRequest("surf-fmp-bs-vs-deploy", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.EffectiveFailModePolicy == nil {
		t.Fatal("EffectiveFailModePolicy must be populated from BS default")
	}
	if res.EffectiveFailModePolicy.PolicyID != "fmp-bs-default" ||
		res.EffectiveFailModePolicy.Source != failmode.ResolutionSourceBusinessService {
		t.Errorf("expected BS default to win; got %+v", res.EffectiveFailModePolicy)
	}
}

// TestEvaluate_FailModeResolver_DeploymentDefaultMissingPolicyLeavesNoEffective
// pins the resolver-error posture for the deployment-default path:
// when the configured id does not resolve to an active policy, the
// resolver logs a warning, EffectiveFailModePolicy stays nil, the
// evaluation succeeds, and Outcome is Accept. This mirrors the
// surface/BS error behaviour established by D27j-impl-2.
func TestEvaluate_FailModeResolver_DeploymentDefaultMissingPolicyLeavesNoEffective(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-fmp-deploy-missing")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-fmp-deploy-missing")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")
	// Do not seed any failmode policy at all — the deployment-default
	// reference must point at nothing.

	orch := newOrchestrator(t, r).WithFailModeDeploymentDefaultPolicyID("fmp-deployment-unknown")
	res, err := orch.Evaluate(context.Background(),
		baseRequest("surf-fmp-deploy-missing", "agent-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate must not fail on resolver error: %v", err)
	}
	if res.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome must be Accept on resolver error; got %q", res.Outcome)
	}
	if res.EffectiveFailModePolicy != nil {
		t.Errorf("EffectiveFailModePolicy must be nil when deployment default fails to resolve; got %+v",
			res.EffectiveFailModePolicy)
	}
}
