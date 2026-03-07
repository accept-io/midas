package decision

import (
	"context"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/surface"
)

var ErrNilOrchestratorDependency = errors.New("orchestrator dependency is nil")

// EvaluationResult is the typed result returned by the orchestrator.
type EvaluationResult struct {
	Outcome    eval.Outcome
	ReasonCode eval.ReasonCode
	EnvelopeID string
}

// Orchestrator coordinates the MIDAS evaluation flow.
// It depends only on domain repository interfaces and policy boundary interfaces.
type Orchestrator struct {
	Surfaces  surface.SurfaceRepository
	Profiles  authority.ProfileRepository
	Grants    authority.GrantRepository
	Agents    agent.AgentRepository
	Envelopes envelope.EnvelopeRepository
	Policies  policy.PolicyEvaluator
}

// NewOrchestrator constructs an Orchestrator with required dependencies.
func NewOrchestrator(
	surfaces surface.SurfaceRepository,
	profiles authority.ProfileRepository,
	grants authority.GrantRepository,
	agents agent.AgentRepository,
	envelopes envelope.EnvelopeRepository,
	policies policy.PolicyEvaluator,
) (*Orchestrator, error) {
	if surfaces == nil || profiles == nil || grants == nil || agents == nil || envelopes == nil || policies == nil {
		return nil, ErrNilOrchestratorDependency
	}

	return &Orchestrator{
		Surfaces:  surfaces,
		Profiles:  profiles,
		Grants:    grants,
		Agents:    agents,
		Envelopes: envelopes,
		Policies:  policies,
	}, nil
}

// Evaluate executes the MIDAS authority evaluation flow.
//
// Planned sequence:
// 1. Surface & Profile Resolution
// 2. Authority Chain Validation
// 3. Context Validation
// 4. Threshold Evaluation
// 5. Policy Check
// 6. Outcome Recording
func (o *Orchestrator) Evaluate(ctx context.Context, req eval.DecisionRequest) (EvaluationResult, error) {
	now := time.Now().UTC()

	_ = now
	_ = ctx
	_ = req

	// TODO: create envelope in RECEIVED state
	// TODO: resolve surface by logical ID and effective time
	// TODO: resolve agent
	// TODO: resolve grant
	// TODO: resolve active profile by effective time
	// TODO: validate authority chain
	// TODO: validate required context keys
	// TODO: evaluate confidence threshold
	// TODO: evaluate consequence threshold
	// TODO: call policy evaluator
	// TODO: record final outcome and update envelope state

	return EvaluationResult{
		Outcome:    eval.OutcomeReject,
		ReasonCode: eval.ReasonProfileNotFound,
		EnvelopeID: "",
	}, nil
}
