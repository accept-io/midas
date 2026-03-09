package decision

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
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
	surfaces  surface.SurfaceRepository
	profiles  authority.ProfileRepository
	grants    authority.GrantRepository
	agents    agent.AgentRepository
	envelopes envelope.EnvelopeRepository
	audit     audit.AuditEventRepository
	policies  policy.PolicyEvaluator
}

// NewOrchestrator constructs an Orchestrator with required dependencies.
func NewOrchestrator(
	surfaces surface.SurfaceRepository,
	profiles authority.ProfileRepository,
	grants authority.GrantRepository,
	agents agent.AgentRepository,
	envelopes envelope.EnvelopeRepository,
	auditRepo audit.AuditEventRepository,
	policies policy.PolicyEvaluator,
) (*Orchestrator, error) {
	if surfaces == nil || profiles == nil || grants == nil ||
		agents == nil || envelopes == nil || auditRepo == nil || policies == nil {
		return nil, ErrNilOrchestratorDependency
	}

	return &Orchestrator{
		surfaces:  surfaces,
		profiles:  profiles,
		grants:    grants,
		agents:    agents,
		envelopes: envelopes,
		audit:     auditRepo,
		policies:  policies,
	}, nil
}

// Evaluate executes the MIDAS authority evaluation flow.
//
// Sequence:
// 1. Create envelope
// 2. Resolve surface
// 3. Resolve agent
// 4. Resolve authority chain (grant + profile + chain validation)
// 5. Validate required context
// 6. Evaluate confidence threshold
// 7. Evaluate consequence threshold
// 8. Evaluate policy
// 9. Record outcome and close envelope
func (o *Orchestrator) Evaluate(ctx context.Context, req eval.DecisionRequest) (EvaluationResult, error) {
	now := time.Now().UTC()

	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	}

	env := &envelope.Envelope{
		ID:        uuid.NewString(),
		RequestID: req.RequestID,
		State:     envelope.EnvelopeStateReceived,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := o.envelopes.Create(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	if err := o.appendAuditEvent(ctx, env, audit.AuditEventEnvelopeCreated, nil); err != nil {
		return EvaluationResult{}, err
	}

	if err := env.Transition(envelope.EnvelopeStateEvaluating); err != nil {
		return EvaluationResult{}, err
	}
	if err := o.envelopes.Update(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	if err := o.appendAuditEvent(ctx, env, audit.AuditEventStateTransitioned, map[string]any{
		"from_state": envelope.EnvelopeStateReceived,
		"to_state":   envelope.EnvelopeStateEvaluating,
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Step 1: Surface resolution
	s, outcome, reason, err := o.resolveSurface(ctx, req.SurfaceID, now)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, env, outcome, reason)
	}

	if err := o.appendResolutionEvent(ctx, env, audit.AuditEventSurfaceResolved, map[string]any{
		"surface_id":      s.ID,
		"surface_version": s.Version,
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Step 2: Agent resolution
	a, outcome, reason, err := o.resolveAgent(ctx, req.AgentID)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, env, outcome, reason)
	}

	if err := o.appendResolutionEvent(ctx, env, audit.AuditEventAgentResolved, map[string]any{
		"agent_id": a.ID,
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Step 3: Authority chain resolution (grant + profile + chain validation)
	_, p, outcome, reason, err := o.resolveAuthorityChain(ctx, req.AgentID, req.SurfaceID, now)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, env, outcome, reason)
	}

	// Record evidence references on the envelope
	env.Evidence = envelope.Evidence{
		SurfaceID:      s.ID,
		SurfaceVersion: s.Version,
		ProfileID:      p.ID,
		ProfileVersion: p.Version,
		AgentID:        a.ID,
	}

	env.Explanation = &envelope.DecisionExplanation{
		SurfaceID:                      s.ID,
		AgentID:                        a.ID,
		ConfidenceProvided:             req.Confidence,
		ConfidenceThreshold:            p.ConfidenceThreshold,
		PolicyEvaluated:                p.PolicyReference != "",
		ConsequenceThresholdType:       string(p.ConsequenceThreshold.Type),
		ConsequenceThresholdAmount:     p.ConsequenceThreshold.Amount,
		ConsequenceThresholdCurrency:   p.ConsequenceThreshold.Currency,
		ConsequenceThresholdRiskRating: string(p.ConsequenceThreshold.RiskRating),
	}

	if req.Consequence != nil {
		env.Explanation.ConsequenceProvidedType = string(req.Consequence.Type)
		env.Explanation.ConsequenceProvidedAmount = req.Consequence.Amount
		env.Explanation.ConsequenceProvidedCurrency = req.Consequence.Currency
		env.Explanation.ConsequenceProvidedRiskRating = string(req.Consequence.RiskRating)
	}

	if err := o.envelopes.Update(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	// Step 4: Context validation
	if !hasRequiredContext(req.Context, p.RequiredContextKeys) {
		return o.finish(ctx, env, eval.OutcomeRequestClarification, eval.ReasonInsufficientContext)
	}

	// Step 5: Confidence threshold
	if req.Confidence < p.ConfidenceThreshold {
		return o.finish(ctx, env, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)
	}

	// Step 6: Consequence threshold
	if authority.ExceedsConsequenceThreshold(req.Consequence, p.ConsequenceThreshold) {
		return o.finish(ctx, env, eval.OutcomeEscalate, eval.ReasonConsequenceExceedsLimit)
	}

	// Step 7: Policy evaluation
	policyOutcome, policyReason, err := o.evaluatePolicy(ctx, req, p)
	if err != nil {
		return EvaluationResult{}, err
	}
	if policyOutcome != "" {
		return o.finish(ctx, env, policyOutcome, policyReason)
	}

	// Step 8: All checks passed
	return o.finish(ctx, env, eval.OutcomeExecute, eval.ReasonWithinAuthority)
}

func (o *Orchestrator) resolveSurface(
	ctx context.Context,
	surfaceID string,
	at time.Time,
) (*surface.DecisionSurface, eval.Outcome, eval.ReasonCode, error) {
	s, err := o.surfaces.FindActiveAt(ctx, surfaceID, at)
	if err != nil {
		return nil, "", "", err
	}
	if s == nil {
		return nil, eval.OutcomeReject, eval.ReasonSurfaceNotFound, nil
	}
	if s.Status != surface.SurfaceStatusActive {
		return nil, eval.OutcomeReject, eval.ReasonSurfaceInactive, nil
	}

	return s, "", "", nil
}

func (o *Orchestrator) resolveAgent(
	ctx context.Context,
	agentID string,
) (*agent.Agent, eval.Outcome, eval.ReasonCode, error) {
	a, err := o.agents.GetByID(ctx, agentID)
	if err != nil {
		return nil, "", "", err
	}
	if a == nil {
		return nil, eval.OutcomeReject, eval.ReasonAgentNotFound, nil
	}

	return a, "", "", nil
}

func (o *Orchestrator) resolveAuthorityChain(
	ctx context.Context,
	agentID string,
	surfaceID string,
	at time.Time,
) (*authority.AuthorityGrant, *authority.AuthorityProfile, eval.Outcome, eval.ReasonCode, error) {
	grants, err := o.grants.ListByAgent(ctx, agentID)
	if err != nil {
		return nil, nil, "", "", err
	}
	if len(grants) == 0 {
		return nil, nil, eval.OutcomeReject, eval.ReasonNoActiveGrant, nil
	}

	var foundProfile bool
	for _, g := range grants {
		if g == nil || g.Status != authority.GrantStatusActive {
			continue
		}

		p, err := o.profiles.FindActiveAt(ctx, g.ProfileID, at)
		if err != nil {
			return nil, nil, "", "", err
		}
		if p == nil {
			continue
		}

		foundProfile = true

		if p.SurfaceID != surfaceID {
			continue
		}

		return g, p, "", "", nil
	}

	if foundProfile {
		return nil, nil, eval.OutcomeReject, eval.ReasonGrantProfileSurfaceMismatch, nil
	}

	return nil, nil, eval.OutcomeReject, eval.ReasonProfileNotFound, nil
}

func (o *Orchestrator) evaluatePolicy(
	ctx context.Context,
	req eval.DecisionRequest,
	p *authority.AuthorityProfile,
) (eval.Outcome, eval.ReasonCode, error) {
	if p.PolicyReference == "" {
		return "", "", nil
	}

	result, err := o.policies.Evaluate(ctx, policy.PolicyInput{
		SurfaceID: req.SurfaceID,
		AgentID:   req.AgentID,
		Context:   req.Context,
	})
	if err != nil {
		if p.FailMode == authority.FailModeOpen {
			return "", "", nil
		}
		return eval.OutcomeEscalate, eval.ReasonPolicyError, nil
	}

	if !result.Allowed {
		return eval.OutcomeEscalate, eval.ReasonPolicyDeny, nil
	}

	return "", "", nil
}

// finish records the outcome on the envelope and transitions to the terminal state.
//
// TODO: week 4 — escalated envelopes should remain open until review is recorded.
// Currently all envelopes are auto-closed in the same request.
func (o *Orchestrator) finish(
	ctx context.Context,
	env *envelope.Envelope,
	outcome eval.Outcome,
	reason eval.ReasonCode,
) (EvaluationResult, error) {
	env.Outcome = outcome
	env.ReasonCode = reason

	if env.Explanation == nil {
		env.Explanation = &envelope.DecisionExplanation{}
	}
	env.Explanation.Result = string(outcome)
	env.Explanation.Reason = string(reason)

	switch outcome {
	case eval.OutcomeEscalate:
		if err := env.Transition(envelope.EnvelopeStateEscalated); err != nil {
			return EvaluationResult{}, err
		}
	default:
		if err := env.Transition(envelope.EnvelopeStateOutcomeRecorded); err != nil {
			return EvaluationResult{}, err
		}
	}

	if err := o.envelopes.Update(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	if err := env.Transition(envelope.EnvelopeStateClosed); err != nil {
		return EvaluationResult{}, err
	}
	if err := o.envelopes.Update(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	return EvaluationResult{
		Outcome:    outcome,
		ReasonCode: reason,
		EnvelopeID: env.ID,
	}, nil
}

func hasRequiredContext(ctxMap map[string]any, required []string) bool {
	if len(required) == 0 {
		return true
	}
	if ctxMap == nil {
		return false
	}

	for _, key := range required {
		if _, ok := ctxMap[key]; !ok {
			return false
		}
	}

	return true
}

func (o *Orchestrator) GetEnvelopeByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	if id == "" {
		return nil, nil
	}
	return o.envelopes.GetByID(ctx, id)
}

func (o *Orchestrator) GetEnvelopeByRequestID(ctx context.Context, requestID string) (*envelope.Envelope, error) {
	if requestID == "" {
		return nil, nil
	}
	return o.envelopes.GetByRequestID(ctx, requestID)
}

func (o *Orchestrator) ListEnvelopes(ctx context.Context) ([]*envelope.Envelope, error) {
	return o.envelopes.List(ctx)
}

func (o *Orchestrator) appendAuditEvent(
	ctx context.Context,
	env *envelope.Envelope,
	eventType audit.AuditEventType,
	payload map[string]any,
) error {
	ev := audit.NewEvent(
		env.ID,
		env.RequestID,
		eventType,
		audit.EventPerformerSystem,
		"midas-orchestrator",
		payload,
	)

	return o.audit.Append(ctx, ev)
}

func (o *Orchestrator) appendResolutionEvent(
	ctx context.Context,
	env *envelope.Envelope,
	eventType audit.AuditEventType,
	payload map[string]any,
) error {
	return o.appendAuditEvent(ctx, env, eventType, payload)
}
