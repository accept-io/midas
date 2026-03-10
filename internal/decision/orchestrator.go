package decision

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/surface"
	"github.com/google/uuid"
)

var ErrNilOrchestratorDependency = errors.New("orchestrator dependency is nil")

// EvaluationResult is the typed result returned by the orchestrator.
type EvaluationResult struct {
	Outcome    eval.Outcome
	ReasonCode eval.ReasonCode
	EnvelopeID string
}
type RepositoryStore interface {
	Repositories() (*store.Repositories, error)
	WithTx(ctx context.Context, operation string, fn func(*store.Repositories) error) error
}

// Orchestrator coordinates the MIDAS evaluation flow.
type Orchestrator struct {
	store    RepositoryStore
	policies policy.PolicyEvaluator
	metrics  EvaluationRecorder
}

// NewOrchestrator constructs an Orchestrator with required dependencies.
func NewOrchestrator(
	store RepositoryStore,
	policies policy.PolicyEvaluator,
	metrics EvaluationRecorder,
) (*Orchestrator, error) {
	if store == nil || policies == nil {
		return nil, ErrNilOrchestratorDependency
	}
	if metrics == nil {
		metrics = NoOpEvaluationRecorder{}
	}

	return &Orchestrator{
		store:    store,
		policies: policies,
		metrics:  metrics,
	}, nil
}

// evaluate contains the full MIDAS authority evaluation flow.
// This method runs inside a database transaction initiated by Evaluate().
// All repository operations will commit together or roll back together.
//
// Sequence:
// 1. Create envelope
// ...
// Evaluate executes the MIDAS authority evaluation flow inside a transaction.

func (o *Orchestrator) Evaluate(ctx context.Context, req eval.DecisionRequest) (EvaluationResult, error) {
	start := time.Now()
	var result EvaluationResult

	err := o.store.WithTx(ctx, "evaluation", func(repos *store.Repositories) error {
		var err error
		result, err = o.evaluate(ctx, repos, req)
		return err
	})

	duration := time.Since(start) // ✅ Calculate once, immediately after WithTx

	if err != nil {
		o.metrics.IncrementEvaluationFailure("persistence")
		return result, err
	}

	o.metrics.RecordEvaluationDuration(string(result.Outcome), duration)
	o.metrics.IncrementEvaluationOutcome(string(result.Outcome), string(result.ReasonCode))

	return result, nil
}

// evaluate contains the full MIDAS authority evaluation flow.
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
func (o *Orchestrator) evaluate(
	ctx context.Context,
	repos *store.Repositories,
	req eval.DecisionRequest,
) (EvaluationResult, error) {
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

	if err := repos.Envelopes.Create(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	if err := o.appendAuditEvent(ctx, repos.Audit, env, audit.AuditEventEnvelopeCreated, nil); err != nil {
		return EvaluationResult{}, err
	}

	if err := env.Transition(envelope.EnvelopeStateEvaluating); err != nil {
		return EvaluationResult{}, err
	}
	if err := repos.Envelopes.Update(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	if err := o.appendAuditEvent(ctx, repos.Audit, env, audit.AuditEventStateTransitioned, map[string]any{
		"from_state": string(envelope.EnvelopeStateReceived),
		"to_state":   string(envelope.EnvelopeStateEvaluating),
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Step 1: Surface resolution
	s, outcome, reason, err := o.resolveSurface(ctx, repos.Surfaces, req.SurfaceID, now)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, repos, env, outcome, reason)
	}

	if err := o.appendResolutionEvent(ctx, repos.Audit, env, audit.AuditEventSurfaceResolved, map[string]any{
		"surface_id":      s.ID,
		"surface_version": s.Version,
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Step 2: Agent resolution
	a, outcome, reason, err := o.resolveAgent(ctx, repos.Agents, req.AgentID)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, repos, env, outcome, reason)
	}

	if err := o.appendResolutionEvent(ctx, repos.Audit, env, audit.AuditEventAgentResolved, map[string]any{
		"agent_id": a.ID,
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Step 3: Authority chain resolution (grant + profile + chain validation)
	g, p, outcome, reason, err := o.resolveAuthorityChain(ctx, repos.Grants, repos.Profiles, req.AgentID, req.SurfaceID, now)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, repos, env, outcome, reason)
	}

	if err := o.appendAuthorityChainResolvedEvent(ctx, repos.Audit, env, g, p); err != nil {
		return EvaluationResult{}, err
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

	if err := repos.Envelopes.Update(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	// Step 4: Context validation
	if err := o.appendContextValidatedEvent(ctx, repos.Audit, env, p.RequiredContextKeys, req.Context); err != nil {
		return EvaluationResult{}, err
	}

	if !hasRequiredContext(req.Context, p.RequiredContextKeys) {
		return o.finish(ctx, repos, env, eval.OutcomeRequestClarification, eval.ReasonInsufficientContext)
	}

	// Step 5: Confidence threshold
	if err := o.appendConfidenceCheckedEvent(ctx, repos.Audit, env, req.Confidence, p.ConfidenceThreshold); err != nil {
		return EvaluationResult{}, err
	}

	if req.Confidence < p.ConfidenceThreshold {
		return o.finish(ctx, repos, env, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)
	}

	// Step 6: Consequence threshold
	if err := o.appendConsequenceCheckedEvent(ctx, repos.Audit, env, req.Consequence, p.ConsequenceThreshold); err != nil {
		return EvaluationResult{}, err
	}

	if authority.ExceedsConsequenceThreshold(req.Consequence, p.ConsequenceThreshold) {
		return o.finish(ctx, repos, env, eval.OutcomeEscalate, eval.ReasonConsequenceExceedsLimit)
	}

	// Step 7: Policy evaluation
	policyOutcome, policyReason, err := o.evaluatePolicy(ctx, req, p)
	if err != nil {
		return EvaluationResult{}, err
	}

	if p.PolicyReference != "" {
		if err := o.appendPolicyEvaluatedEvent(ctx, repos.Audit, env, p.PolicyReference, policyOutcome, policyReason); err != nil {
			return EvaluationResult{}, err
		}
	}

	if policyOutcome != "" {
		return o.finish(ctx, repos, env, policyOutcome, policyReason)
	}

	// Step 8: All checks passed
	return o.finish(ctx, repos, env, eval.OutcomeExecute, eval.ReasonWithinAuthority)
}

func (o *Orchestrator) resolveSurface(
	ctx context.Context,
	surfaces surface.SurfaceRepository,
	surfaceID string,
	at time.Time,
) (*surface.DecisionSurface, eval.Outcome, eval.ReasonCode, error) {
	s, err := surfaces.FindActiveAt(ctx, surfaceID, at)
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
	agents agent.AgentRepository,
	agentID string,
) (*agent.Agent, eval.Outcome, eval.ReasonCode, error) {
	a, err := agents.GetByID(ctx, agentID)
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
	grants authority.GrantRepository,
	profiles authority.ProfileRepository,
	agentID string,
	surfaceID string,
	at time.Time,
) (*authority.AuthorityGrant, *authority.AuthorityProfile, eval.Outcome, eval.ReasonCode, error) {
	agentGrants, err := grants.ListByAgent(ctx, agentID)
	if err != nil {
		return nil, nil, "", "", err
	}
	if len(agentGrants) == 0 {
		return nil, nil, eval.OutcomeReject, eval.ReasonNoActiveGrant, nil
	}

	var foundProfile bool
	for _, g := range agentGrants {
		if g == nil || g.Status != authority.GrantStatusActive {
			continue
		}

		p, err := profiles.FindActiveAt(ctx, g.ProfileID, at)
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
	repos *store.Repositories,
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

	if err := o.appendOutcomeRecordedEvent(ctx, repos.Audit, env, outcome, reason); err != nil {
		return EvaluationResult{}, err
	}

	prevState := env.State

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

	if err := o.appendStateTransitionEvent(ctx, repos.Audit, env, prevState, env.State); err != nil {
		return EvaluationResult{}, err
	}

	if err := repos.Envelopes.Update(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	prevState = env.State

	if err := env.Transition(envelope.EnvelopeStateClosed); err != nil {
		return EvaluationResult{}, err
	}

	if err := o.appendStateTransitionEvent(ctx, repos.Audit, env, prevState, envelope.EnvelopeStateClosed); err != nil {
		return EvaluationResult{}, err
	}

	if err := repos.Envelopes.Update(ctx, env); err != nil {
		return EvaluationResult{}, err
	}

	return EvaluationResult{
		Outcome:    outcome,
		ReasonCode: reason,
		EnvelopeID: env.ID,
	}, nil
}

func (o *Orchestrator) appendStateTransitionEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
	env *envelope.Envelope,
	from envelope.EnvelopeState,
	to envelope.EnvelopeState,
) error {
	return o.appendAuditEvent(ctx, auditRepo, env, audit.AuditEventStateTransitioned, map[string]any{
		"from_state": string(from),
		"to_state":   string(to),
	})
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
	repos, err := o.store.Repositories()
	if err != nil {
		return nil, err
	}
	return repos.Envelopes.GetByID(ctx, id)
}

func (o *Orchestrator) GetEnvelopeByRequestID(ctx context.Context, requestID string) (*envelope.Envelope, error) {
	if requestID == "" {
		return nil, nil
	}
	repos, err := o.store.Repositories()
	if err != nil {
		return nil, err
	}
	return repos.Envelopes.GetByRequestID(ctx, requestID)
}

func (o *Orchestrator) ListEnvelopes(ctx context.Context) ([]*envelope.Envelope, error) {
	repos, err := o.store.Repositories()
	if err != nil {
		return nil, err
	}
	return repos.Envelopes.List(ctx)
}

func (o *Orchestrator) appendAuditEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
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

	return auditRepo.Append(ctx, ev)
}

func (o *Orchestrator) appendResolutionEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
	env *envelope.Envelope,
	eventType audit.AuditEventType,
	payload map[string]any,
) error {
	return o.appendAuditEvent(ctx, auditRepo, env, eventType, payload)
}

func (o *Orchestrator) appendAuthorityChainResolvedEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
	env *envelope.Envelope,
	g *authority.AuthorityGrant,
	p *authority.AuthorityProfile,
) error {
	return o.appendAuditEvent(ctx, auditRepo, env, audit.AuditEventAuthorityChainResolved, map[string]any{
		"grant_id":        g.ID,
		"profile_id":      p.ID,
		"profile_version": p.Version,
		"agent_id":        g.AgentID,
	})
}

func (o *Orchestrator) appendContextValidatedEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
	env *envelope.Envelope,
	requiredKeys []string,
	contextMap map[string]any,
) error {
	providedKeys := make([]string, 0, len(contextMap))
	for k := range contextMap {
		providedKeys = append(providedKeys, k)
	}
	sort.Strings(providedKeys)

	required := append([]string(nil), requiredKeys...)
	sort.Strings(required)

	return o.appendAuditEvent(ctx, auditRepo, env, audit.AuditEventContextValidated, map[string]any{
		"required_keys": required,
		"provided_keys": providedKeys,
		"passed":        hasRequiredContext(contextMap, requiredKeys),
	})
}

func (o *Orchestrator) appendConfidenceCheckedEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
	env *envelope.Envelope,
	provided float64,
	threshold float64,
) error {
	return o.appendAuditEvent(ctx, auditRepo, env, audit.AuditEventConfidenceChecked, map[string]any{
		"confidence_provided":  provided,
		"confidence_threshold": threshold,
		"passed":               provided >= threshold,
	})
}

func (o *Orchestrator) appendConsequenceCheckedEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
	env *envelope.Envelope,
	submitted *eval.Consequence,
	threshold authority.Consequence,
) error {
	payload := map[string]any{
		"threshold_type":        string(threshold.Type),
		"threshold_amount":      threshold.Amount,
		"threshold_currency":    threshold.Currency,
		"threshold_risk_rating": string(threshold.RiskRating),
		"passed":                !authority.ExceedsConsequenceThreshold(submitted, threshold),
	}

	if submitted != nil {
		payload["submitted_type"] = string(submitted.Type)
		payload["submitted_amount"] = submitted.Amount
		payload["submitted_currency"] = submitted.Currency
		payload["submitted_risk_rating"] = string(submitted.RiskRating)
	}

	return o.appendAuditEvent(ctx, auditRepo, env, audit.AuditEventConsequenceChecked, payload)
}

func (o *Orchestrator) appendPolicyEvaluatedEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
	env *envelope.Envelope,
	policyRef string,
	outcome eval.Outcome,
	reason eval.ReasonCode,
) error {
	return o.appendAuditEvent(ctx, auditRepo, env, audit.AuditEventPolicyEvaluated, map[string]any{
		"policy_reference": policyRef,
		"outcome":          string(outcome),
		"reason_code":      string(reason),
		"allowed":          outcome == "",
	})
}

func (o *Orchestrator) appendOutcomeRecordedEvent(
	ctx context.Context,
	auditRepo audit.AuditEventRepository,
	env *envelope.Envelope,
	outcome eval.Outcome,
	reason eval.ReasonCode,
) error {
	return o.appendAuditEvent(ctx, auditRepo, env, audit.AuditEventOutcomeRecorded, map[string]any{
		"outcome":     string(outcome),
		"reason_code": string(reason),
	})
}
