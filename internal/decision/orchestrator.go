package decision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Domain error sentinels
// ---------------------------------------------------------------------------

var (
	ErrNilOrchestratorDependency = errors.New("orchestrator dependency is nil")
	ErrEmptyIdentifier           = errors.New("identifier must not be empty")
	ErrEnvelopeNotFound          = errors.New("envelope not found")
	ErrEnvelopeNotAwaitingReview = errors.New("envelope is not awaiting review")
	ErrEnvelopeAlreadyClosed     = errors.New("envelope is already closed")
	ErrInvalidReviewDecision     = errors.New("decision must be APPROVED or REJECTED")

	// ErrScopedRequestConflict is returned when a duplicate (request_source,
	// request_id) pair is submitted with a different payload hash than the
	// original. This indicates request identity reuse with a mutated body and
	// is always a caller error.
	ErrScopedRequestConflict = errors.New("scoped request conflict: same (request_source, request_id) submitted with a different payload")
)

// FailureCategory represents a typed classification of evaluation failures
// for observability. These categories enable precise monitoring and alerting
// without relying on fragile string matching.
type FailureCategory string

const (
	FailureCategoryEnvelopePersistence FailureCategory = "envelope_persistence"
	FailureCategoryAuditAppend         FailureCategory = "audit_append"
	FailureCategoryInvalidTransition   FailureCategory = "invalid_transition"
	FailureCategoryPolicyEvaluation    FailureCategory = "policy_evaluation"
	FailureCategoryAuthorityResolution FailureCategory = "authority_resolution"
	FailureCategoryResolveReview       FailureCategory = "resolve_review"
	FailureCategoryIdempotencyConflict FailureCategory = "idempotency_conflict"
	FailureCategoryUnknown             FailureCategory = "unknown"
)

// categorizedError wraps an error with an explicit failure category.
// Use wrapFailure() to create these.
type categorizedError struct {
	category FailureCategory
	err      error
}

func (e *categorizedError) Error() string {
	return e.err.Error()
}

func (e *categorizedError) Unwrap() error {
	return e.err
}

func (e *categorizedError) Category() FailureCategory {
	return e.category
}

// wrapFailure wraps an error with an explicit failure category, preserving
// the original error chain for errors.Is/As checks.
func wrapFailure(category FailureCategory, err error) error {
	if err == nil {
		return nil
	}
	return &categorizedError{category: category, err: err}
}

// categorizePersistErr inspects an accumulator persist error and wraps it with
// the appropriate failure category. Persist errors can originate from three
// repository calls — Create, Audit.Append, or Update — with distinct categories.
// The error message prefixes are stable internal strings produced by the accumulator.
func categorizePersistErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "audit append ") {
		return wrapFailure(FailureCategoryAuditAppend, err)
	}
	// "create envelope" and "persist final envelope state" are both envelope persistence.
	return wrapFailure(FailureCategoryEnvelopePersistence, err)
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// EvaluationResult is the typed result returned by the orchestrator.
type EvaluationResult struct {
	Outcome     eval.Outcome
	ReasonCode  eval.ReasonCode
	EnvelopeID  string
	State       envelope.EnvelopeState
	Explanation string
}

// RepositoryStore abstracts transactional repository access.
type RepositoryStore interface {
	Repositories() (*store.Repositories, error)
	WithTx(ctx context.Context, operation string, fn func(*store.Repositories) error) error
}

// Clock allows time.Now to be injected for deterministic testing.
type Clock func() time.Time

// EscalationResolution is the input to ResolveEscalation.
type EscalationResolution struct {
	EnvelopeID   string
	Decision     envelope.ReviewDecision
	ReviewerID   string
	ReviewerKind string
	Notes        string
}

// ---------------------------------------------------------------------------
// Orchestrator
// ---------------------------------------------------------------------------

// Orchestrator coordinates the MIDAS evaluation flow.
type Orchestrator struct {
	store    RepositoryStore
	policies policy.PolicyEvaluator
	metrics  EvaluationRecorder
	clock    Clock
}

// NewOrchestrator constructs an Orchestrator with a real clock.
func NewOrchestrator(
	store RepositoryStore,
	policies policy.PolicyEvaluator,
	metrics EvaluationRecorder,
) (*Orchestrator, error) {
	return NewOrchestratorWithClock(store, policies, metrics, time.Now)
}

// NewOrchestratorWithClock constructs an Orchestrator with an injected clock.
func NewOrchestratorWithClock(
	store RepositoryStore,
	policies policy.PolicyEvaluator,
	metrics EvaluationRecorder,
	clock Clock,
) (*Orchestrator, error) {
	if store == nil || policies == nil {
		return nil, ErrNilOrchestratorDependency
	}
	if metrics == nil {
		metrics = NoOpEvaluationRecorder{}
	}
	if clock == nil {
		clock = time.Now
	}
	return &Orchestrator{store: store, policies: policies, metrics: metrics, clock: clock}, nil
}

// ---------------------------------------------------------------------------
// Evaluate
// ---------------------------------------------------------------------------

// Evaluate executes the full MIDAS authority evaluation flow inside a
// database transaction. All repository and audit operations commit together
// or roll back together.
func (o *Orchestrator) Evaluate(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (EvaluationResult, error) {
	startedAt := o.clock()
	var result EvaluationResult

	slog.Info("evaluation_started",
		"request_source", req.RequestSource,
		"request_id", req.RequestID,
		"surface_id", req.SurfaceID,
		"agent_id", req.AgentID,
	)

	err := o.store.WithTx(ctx, "evaluation", func(repos *store.Repositories) error {
		var err error
		result, err = o.evaluate(ctx, repos, req, raw)
		return err
	})

	endedAt := o.clock()
	duration := endedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}

	if err != nil {
		failureKind := classifyFailure(err)
		slog.Error("evaluation_failed",
			"request_source", req.RequestSource,
			"request_id", req.RequestID,
			"surface_id", req.SurfaceID,
			"agent_id", req.AgentID,
			"failure_kind", failureKind,
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		o.metrics.IncrementEvaluationFailure(failureKind)
		return result, err
	}

	slog.Info("evaluation_completed",
		"request_source", req.RequestSource,
		"request_id", req.RequestID,
		"envelope_id", result.EnvelopeID,
		"outcome", result.Outcome,
		"reason_code", result.ReasonCode,
		"explanation", result.Explanation,
		"duration_ms", duration.Milliseconds(),
	)
	o.metrics.RecordEvaluationDuration(string(result.Outcome), duration)
	o.metrics.IncrementEvaluationOutcome(string(result.Outcome), string(result.ReasonCode))

	return result, nil
}

// evaluate runs inside the transaction opened by Evaluate.
// All state transitions and audit events are accumulated in-memory via
// evaluationAccumulator; acc.persistNew() at the end of finish() issues the
// sole database write: Envelopes.Create → N×Audit.Append → Envelopes.Update.
//
// Sequence:
//  1. Create in-memory envelope and evaluationAccumulator
//  2. Queue envelope.created; transition RECEIVED → EVALUATING in memory
//  3. Resolve surface
//  4. Resolve agent
//  5. Resolve authority chain
//  6. Populate Resolved and Evaluation sections
//  7. Validate required context
//  8. Evaluate confidence threshold
//  9. Evaluate consequence threshold
// 10. Evaluate policy
// 11. Delegate to finish() to record outcome and flush via acc.persistNew()
func (o *Orchestrator) evaluate(
	ctx context.Context,
	repos *store.Repositories,
	req eval.DecisionRequest,
	raw json.RawMessage,
) (EvaluationResult, error) {
	now := o.clock().UTC()

	// request_source is required for scoped idempotency
	if req.RequestSource == "" {
		return EvaluationResult{}, errors.New("request_source is required")
	}
	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	}

	submittedHashBytes := sha256.Sum256(raw)
	submittedHash := hex.EncodeToString(submittedHashBytes[:])

	// ---------------------------------------------------------------------------
	// Scoped idempotency check
	//
	// Look up (request_source, request_id) before creating a new envelope.
	// Two cases:
	//   1. Exact replay (same payload hash): return the existing result without
	//      creating a second envelope.
	//   2. Hash mismatch: the caller is reusing a scoped identity with a mutated
	//      body. This is always a caller error; return ErrScopedRequestConflict.
	// ---------------------------------------------------------------------------
	existing, err := repos.Envelopes.GetByRequestScope(ctx, req.RequestSource, req.RequestID)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("idempotency lookup: %w", err)
	}
	if existing != nil {
		if existing.Integrity.SubmittedHash == submittedHash {
			// Exact replay — return the original decision without side effects.
			slog.Info("evaluation_replayed",
				"request_source", req.RequestSource,
				"request_id", req.RequestID,
				"envelope_id", existing.ID(),
				"outcome", existing.Evaluation.Outcome,
				"reason_code", existing.Evaluation.ReasonCode,
			)
			return resultFromEnvelope(existing), nil
		}
		// Same scope, different payload — deterministic conflict error.
		slog.Warn("idempotency_conflict",
			"request_source", req.RequestSource,
			"request_id", req.RequestID,
			"existing_envelope_id", existing.ID(),
			"existing_hash", existing.Integrity.SubmittedHash,
			"submitted_hash", submittedHash,
		)
		return EvaluationResult{}, wrapFailure(FailureCategoryIdempotencyConflict, ErrScopedRequestConflict)
	}

	env, err := envelope.New(uuid.NewString(), req.RequestSource, req.RequestID, raw, now)
	if err != nil {
		return EvaluationResult{}, err
	}
	env.Integrity.SubmittedHash = submittedHash

	acc, err := newEvaluationAccumulator(env)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("create accumulator: %w", err)
	}

	// Queue envelope.created. The accumulator's persist() will Create the
	// envelope row and Append all events in a single atomic sequence at the
	// end of finish(). No DB writes happen here.
	if err := acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeCreated,
		buildGovernancePayload(req, submittedHash),
	); err != nil {
		return EvaluationResult{}, fmt.Errorf("record envelope.created: %w", err)
	}

	// RECEIVED → EVALUATING
	from := env.State
	if err := acc.transition(envelope.EnvelopeStateEvaluating, now); err != nil {
		return EvaluationResult{}, wrapFailure(FailureCategoryInvalidTransition, err)
	}
	if err := acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEvaluationStarted,
		map[string]any{"from_state": string(from), "to_state": string(envelope.EnvelopeStateEvaluating)},
	); err != nil {
		return EvaluationResult{}, err
	}

	// Step 1: Surface
	s, outcome, reason, err := o.resolveSurface(ctx, repos.Surfaces, req.SurfaceID, now)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, repos, acc, env, outcome, reason)
	}
	slog.Debug("surface_resolved", "request_id", req.RequestID, "surface_id", s.ID)
	if err := acc.recordObservation(env.RequestSource(), env.RequestID(), audit.AuditEventSurfaceResolved, map[string]any{
		"surface_id": s.ID, "surface_version": s.Version,
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Step 2: Agent
	a, outcome, reason, err := o.resolveAgent(ctx, repos.Agents, req.AgentID)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, repos, acc, env, outcome, reason)
	}
	slog.Debug("agent_resolved", "request_id", req.RequestID, "agent_id", a.ID)
	if err := acc.recordObservation(env.RequestSource(), env.RequestID(), audit.AuditEventAgentResolved, map[string]any{
		"agent_id": a.ID,
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Step 3: Authority chain
	g, p, outcome, reason, err := o.resolveAuthorityChain(ctx, repos.Grants, repos.Profiles, req.AgentID, req.SurfaceID, now)
	if err != nil {
		return EvaluationResult{}, err
	}
	if outcome != "" {
		return o.finish(ctx, repos, acc, env, outcome, reason)
	}
	slog.Debug("authority_chain_resolved", "request_id", req.RequestID, "grant_id", g.ID, "profile_id", p.ID)
	if err := acc.recordObservation(env.RequestSource(), env.RequestID(), audit.AuditEventAuthorityChainResolved, map[string]any{
		"grant_id": g.ID, "profile_id": p.ID, "profile_version": p.Version, "agent_id": g.AgentID,
	}); err != nil {
		return EvaluationResult{}, err
	}

	// Populate Resolved section
	env.Resolved = envelope.Resolved{
		Authority: envelope.ResolvedAuthority{
			SurfaceID:      s.ID,
			SurfaceVersion: s.Version,
			ProfileID:      p.ID,
			ProfileVersion: p.Version,
			AgentID:        a.ID,
			GrantID:        g.ID,
		},
		Metadata: envelope.RequestMetadata{
			ActionType:        req.ActionType,
			ActionDescription: req.ActionDesc,
			AgentKind:         req.AgentKind,
		},
	}
	if req.Runtime != nil {
		env.Resolved.Metadata.AgentRuntimeModel = req.Runtime.Model
		env.Resolved.Metadata.AgentRuntimeVersion = req.Runtime.Version
	}
	if req.Delegation != nil {
		env.Resolved.Delegation = &envelope.DelegationEvidence{
			InitiatedBy:     req.Delegation.InitiatedBy,
			SessionID:       req.Delegation.SessionID,
			Chain:           req.Delegation.Chain,
			Scope:           req.Delegation.Scope,
			AuthorizedAt:    req.Delegation.AuthorizedAt,
			AuthorizedUntil: req.Delegation.AuthorizedUntil,
		}
	}
	if req.Subject != nil {
		env.Resolved.Subject = &envelope.DecisionSubject{
			Type:         req.Subject.Type,
			ID:           req.Subject.ID,
			SecondaryIDs: req.Subject.SecondaryIDs,
		}
	}

	// Populate denormalized authority chain fields for database indexing.
	env.ResolvedSurfaceID = s.ID
	env.ResolvedSurfaceVersion = s.Version
	env.ResolvedProfileID = p.ID
	env.ResolvedProfileVersion = p.Version
	env.ResolvedGrantID = g.ID
	env.ResolvedAgentID = a.ID
	if req.Subject != nil {
		env.ResolvedSubjectID = req.Subject.ID
	}

	// Seed Evaluation section
	evaluatedAt := now
	env.Evaluation = envelope.Evaluation{
		EvaluatedAt: &evaluatedAt,
		Explanation: &envelope.DecisionExplanation{
			ConfidenceProvided:             req.Confidence,
			ConfidenceThreshold:            p.ConfidenceThreshold,
			PolicyEvaluated:                p.PolicyReference != "",
			PolicyReference:                p.PolicyReference,
			ConsequenceThresholdType:       string(p.ConsequenceThreshold.Type),
			ConsequenceThresholdAmount:     p.ConsequenceThreshold.Amount,
			ConsequenceThresholdCurrency:   p.ConsequenceThreshold.Currency,
			ConsequenceThresholdRiskRating: string(p.ConsequenceThreshold.RiskRating),
			DelegationValidated:            req.Delegation != nil,
			ActionWithinScope:              true,
		},
	}
	if req.Consequence != nil {
		env.Evaluation.Explanation.ConsequenceProvidedType = string(req.Consequence.Type)
		env.Evaluation.Explanation.ConsequenceProvidedAmount = req.Consequence.Amount
		env.Evaluation.Explanation.ConsequenceProvidedCurrency = req.Consequence.Currency
		env.Evaluation.Explanation.ConsequenceProvidedRiskRating = string(req.Consequence.RiskRating)
		env.Evaluation.Explanation.ConsequenceReversible = req.Consequence.Reversible
	}
	// Resolved and Evaluation sections are populated in-memory; acc.persistNew()
	// at the end of finish() flushes everything in a single Envelopes.Update.

	// Step 4: Context
	if err := o.appendContextValidatedEvent(acc, env, p.RequiredContextKeys, req.Context); err != nil {
		return EvaluationResult{}, err
	}
	if !hasRequiredContext(req.Context, p.RequiredContextKeys) {
		return o.finish(ctx, repos, acc, env, eval.OutcomeRequestClarification, eval.ReasonInsufficientContext)
	}

	// Step 5: Confidence
	if err := o.appendConfidenceCheckedEvent(acc, env, req.Confidence, p.ConfidenceThreshold); err != nil {
		return EvaluationResult{}, err
	}
	if req.Confidence < p.ConfidenceThreshold {
		return o.finish(ctx, repos, acc, env, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)
	}

	// Step 6: Consequence
	if err := o.appendConsequenceCheckedEvent(acc, env, req.Consequence, p.ConsequenceThreshold); err != nil {
		return EvaluationResult{}, err
	}
	if authority.ExceedsConsequenceThreshold(req.Consequence, p.ConsequenceThreshold) {
		return o.finish(ctx, repos, acc, env, eval.OutcomeEscalate, eval.ReasonConsequenceExceedsLimit)
	}

	// Step 7: Policy
	policyOutcome, policyReason, err := o.evaluatePolicy(ctx, req, p)
	if err != nil {
		return EvaluationResult{}, err
	}
	if p.PolicyReference != "" {
		if err := o.appendPolicyEvaluatedEvent(acc, env, p.PolicyReference, policyOutcome, policyReason); err != nil {
			return EvaluationResult{}, err
		}
	}
	if policyOutcome != "" {
		return o.finish(ctx, repos, acc, env, policyOutcome, policyReason)
	}

	return o.finish(ctx, repos, acc, env, eval.OutcomeAccept, eval.ReasonWithinAuthority)
}

// ---------------------------------------------------------------------------
// finish
// ---------------------------------------------------------------------------

// finish records the evaluation outcome, drives the envelope to its terminal
// state via the accumulator, and flushes everything to the database atomically.
//
// Escalated:     EVALUATING → ESCALATED → AWAITING_REVIEW
// Non-escalated: EVALUATING → OUTCOME_RECORDED → CLOSED
//
// acc.persist() is the sole DB write for the entire evaluation: it creates the
// envelope row, appends all queued events (lifecycle + observational) in order,
// and writes the final envelope state in one transaction-safe sequence.
func (o *Orchestrator) finish(
	ctx context.Context,
	repos *store.Repositories,
	acc *evaluationAccumulator,
	env *envelope.Envelope,
	outcome eval.Outcome,
	reason eval.ReasonCode,
) (EvaluationResult, error) {
	now := o.clock().UTC()

	env.Evaluation.Outcome = outcome
	env.Evaluation.ReasonCode = reason
	if env.Evaluation.Explanation == nil {
		env.Evaluation.Explanation = &envelope.DecisionExplanation{}
	}
	env.Evaluation.Explanation.Result = string(outcome)
	env.Evaluation.Explanation.Reason = string(reason)

	explanationText := buildExplanationText(env, outcome, reason)

	if outcome == eval.OutcomeEscalate {
		// EVALUATING → ESCALATED
		from := env.State
		if err := acc.transition(envelope.EnvelopeStateEscalated, now); err != nil {
			return EvaluationResult{}, wrapFailure(FailureCategoryInvalidTransition, err)
		}
		if err := acc.recordLifecycle(env.RequestSource(), env.RequestID(),
			audit.AuditEventOutcomeRecorded,
			map[string]any{
				"outcome": string(outcome), "reason_code": string(reason),
				"from_state": string(from), "to_state": string(envelope.EnvelopeStateEscalated),
			},
		); err != nil {
			return EvaluationResult{}, err
		}

		// ESCALATED → AWAITING_REVIEW
		from = env.State
		if err := acc.transition(envelope.EnvelopeStateAwaitingReview, now); err != nil {
			return EvaluationResult{}, wrapFailure(FailureCategoryInvalidTransition, err)
		}
		if err := acc.recordLifecycle(env.RequestSource(), env.RequestID(),
			audit.AuditEventEscalationPending,
			map[string]any{
				"from_state": string(from), "to_state": string(envelope.EnvelopeStateAwaitingReview),
			},
		); err != nil {
			return EvaluationResult{}, err
		}

		if err := acc.persistNew(ctx, repos); err != nil {
			return EvaluationResult{}, categorizePersistErr(fmt.Errorf("persist evaluation: %w", err))
		}
		return EvaluationResult{
			Outcome:     outcome,
			ReasonCode:  reason,
			EnvelopeID:  env.ID(),
			State:       env.State,
			Explanation: explanationText,
		}, nil
	}

	// EVALUATING → OUTCOME_RECORDED
	from := env.State
	if err := acc.transition(envelope.EnvelopeStateOutcomeRecorded, now); err != nil {
		return EvaluationResult{}, wrapFailure(FailureCategoryInvalidTransition, err)
	}
	if err := acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventOutcomeRecorded,
		map[string]any{
			"outcome": string(outcome), "reason_code": string(reason),
			"from_state": string(from), "to_state": string(envelope.EnvelopeStateOutcomeRecorded),
		},
	); err != nil {
		return EvaluationResult{}, err
	}

	// OUTCOME_RECORDED → CLOSED
	from = env.State
	if err := acc.transition(envelope.EnvelopeStateClosed, now); err != nil {
		return EvaluationResult{}, wrapFailure(FailureCategoryInvalidTransition, err)
	}
	if err := acc.recordLifecycle(env.RequestSource(), env.RequestID(),
		audit.AuditEventEnvelopeClosed,
		map[string]any{
			"from_state": string(from), "to_state": string(envelope.EnvelopeStateClosed),
		},
	); err != nil {
		return EvaluationResult{}, err
	}

	if err := acc.persistNew(ctx, repos); err != nil {
		return EvaluationResult{}, categorizePersistErr(fmt.Errorf("persist evaluation: %w", err))
	}
	return EvaluationResult{
		Outcome:     outcome,
		ReasonCode:  reason,
		EnvelopeID:  env.ID(),
		State:       env.State,
		Explanation: explanationText,
	}, nil
}

// ---------------------------------------------------------------------------
// ResolveEscalation
// ---------------------------------------------------------------------------

// ResolveEscalation records a reviewer's decision on an escalated envelope
// and closes it. The envelope must be in AWAITING_REVIEW state.
//
// Event sequence:
//
//	AuditEventEscalationReviewed — semantic record of the review decision
//	AuditEventEnvelopeClosed     — uniform close event (same as non-escalated path)
//
// The two-event sequence means:
//   - "all closed envelopes" queries on AuditEventEnvelopeClosed work uniformly,
//   - "all reviewed escalations" queries on AuditEventEscalationReviewed work independently.
func (o *Orchestrator) ResolveEscalation(ctx context.Context, res EscalationResolution) (*envelope.Envelope, error) {
	if res.EnvelopeID == "" {
		return nil, ErrEmptyIdentifier
	}
	if res.Decision != envelope.ReviewDecisionApproved && res.Decision != envelope.ReviewDecisionRejected {
		return nil, ErrInvalidReviewDecision
	}
	if res.ReviewerID == "" {
		return nil, ErrEmptyIdentifier
	}

	var result *envelope.Envelope

	err := o.store.WithTx(ctx, "resolve_escalation", func(repos *store.Repositories) error {
		now := o.clock().UTC()

		env, err := repos.Envelopes.GetByID(ctx, res.EnvelopeID)
		if err != nil {
			return err
		}
		if env == nil {
			return ErrEnvelopeNotFound
		}
		if env.State == envelope.EnvelopeStateClosed {
			return ErrEnvelopeAlreadyClosed
		}
		if env.State != envelope.EnvelopeStateAwaitingReview {
			return fmt.Errorf("envelope %s is in state %s: %w",
				res.EnvelopeID, env.State, ErrEnvelopeNotAwaitingReview)
		}

		// Set Review before transition so Transition()'s content invariant
		// can verify the review is present before allowing AWAITING_REVIEW → CLOSED.
		env.Review = &envelope.EscalationReview{
			Decision:     res.Decision,
			ReviewerID:   res.ReviewerID,
			ReviewerKind: res.ReviewerKind,
			Notes:        res.Notes,
			ReviewedAt:   now,
		}

		acc, err := newExistingEnvelopeAccumulator(env)
		if err != nil {
			return fmt.Errorf("create accumulator: %w", err)
		}

		// Queue ESCALATION_REVIEWED: semantic record of the review decision.
		if err := acc.recordObservation(env.RequestSource(), env.RequestID(),
			audit.AuditEventEscalationReviewed,
			map[string]any{
				"decision":      string(res.Decision),
				"reviewer_id":   res.ReviewerID,
				"reviewer_kind": res.ReviewerKind,
				"notes":         res.Notes,
			},
		); err != nil {
			return fmt.Errorf("record escalation_reviewed: %w", err)
		}

		// AWAITING_REVIEW → CLOSED
		from := env.State
		if err := acc.transition(envelope.EnvelopeStateClosed, now); err != nil {
			return wrapFailure(FailureCategoryInvalidTransition, err)
		}

		// Queue ENVELOPE_CLOSED to match the non-escalated close path exactly.
		if err := acc.recordLifecycle(env.RequestSource(), env.RequestID(),
			audit.AuditEventEnvelopeClosed,
			map[string]any{
				"from_state": string(from),
				"to_state":   string(envelope.EnvelopeStateClosed),
			},
		); err != nil {
			return fmt.Errorf("record envelope_closed: %w", err)
		}

		if err := acc.persistExisting(ctx, repos); err != nil {
			return categorizePersistErr(fmt.Errorf("persist escalation resolution: %w", err))
		}

		slog.Info("escalation_resolved",
			"envelope_id", env.ID(),
			"request_source", env.RequestSource(),
			"request_id", env.RequestID(),
			"decision", string(res.Decision),
			"reviewer_id", res.ReviewerID,
		)

		result = env
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Retrieval
// ---------------------------------------------------------------------------

func (o *Orchestrator) GetEnvelopeByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	if id == "" {
		return nil, ErrEmptyIdentifier
	}
	repos, err := o.store.Repositories()
	if err != nil {
		return nil, err
	}
	return repos.Envelopes.GetByID(ctx, id)
}

func (o *Orchestrator) GetEnvelopeByRequestID(ctx context.Context, requestID string) (*envelope.Envelope, error) {
	if requestID == "" {
		return nil, ErrEmptyIdentifier
	}
	repos, err := o.store.Repositories()
	if err != nil {
		return nil, err
	}
	return repos.Envelopes.GetByRequestID(ctx, requestID)
}

// GetEnvelopeByRequestScope retrieves an envelope by (request_source, request_id) composite key.
// This is the preferred lookup for scoped idempotency checks.
func (o *Orchestrator) GetEnvelopeByRequestScope(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
	if requestSource == "" || requestID == "" {
		return nil, ErrEmptyIdentifier
	}
	repos, err := o.store.Repositories()
	if err != nil {
		return nil, err
	}
	return repos.Envelopes.GetByRequestScope(ctx, requestSource, requestID)
}

func (o *Orchestrator) ListEnvelopes(ctx context.Context) ([]*envelope.Envelope, error) {
	repos, err := o.store.Repositories()
	if err != nil {
		return nil, err
	}
	return repos.Envelopes.List(ctx)
}

// ---------------------------------------------------------------------------
// Audit helpers
//
// Two kinds of audit events in the Evaluate and ResolveEscalation paths:
//   - Lifecycle events: queued via acc.recordLifecycle(). Accompany envelope
//     state transitions.
//   - Observational events: queued via acc.recordObservation() and the typed
//     helpers below. Record facts (surface resolved, confidence checked, etc.)
//     without changing state.
//
// All events are flushed atomically by acc.persistNew() (new evaluations) or
// acc.persistExisting() (escalation resolution) at the end of each path.
// ---------------------------------------------------------------------------

func (o *Orchestrator) appendContextValidatedEvent(
	acc *evaluationAccumulator,
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
	return acc.recordObservation(env.RequestSource(), env.RequestID(), audit.AuditEventContextValidated, map[string]any{
		"required_keys": required,
		"provided_keys": providedKeys,
		"passed":        hasRequiredContext(contextMap, requiredKeys),
	})
}

func (o *Orchestrator) appendConfidenceCheckedEvent(
	acc *evaluationAccumulator,
	env *envelope.Envelope,
	provided float64,
	threshold float64,
) error {
	return acc.recordObservation(env.RequestSource(), env.RequestID(), audit.AuditEventConfidenceChecked, map[string]any{
		"confidence_provided":  provided,
		"confidence_threshold": threshold,
		"passed":               provided >= threshold,
	})
}

func (o *Orchestrator) appendConsequenceCheckedEvent(
	acc *evaluationAccumulator,
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
	return acc.recordObservation(env.RequestSource(), env.RequestID(), audit.AuditEventConsequenceChecked, payload)
}

func (o *Orchestrator) appendPolicyEvaluatedEvent(
	acc *evaluationAccumulator,
	env *envelope.Envelope,
	policyRef string,
	outcome eval.Outcome,
	reason eval.ReasonCode,
) error {
	return acc.recordObservation(env.RequestSource(), env.RequestID(), audit.AuditEventPolicyEvaluated, map[string]any{
		"policy_reference": policyRef,
		"outcome":          string(outcome),
		"reason_code":      string(reason),
		"allowed":          outcome == "",
	})
}

// ---------------------------------------------------------------------------
// Resolution helpers
// ---------------------------------------------------------------------------

func (o *Orchestrator) resolveSurface(
	ctx context.Context,
	surfaces surface.SurfaceRepository,
	surfaceID string,
	at time.Time,
) (*surface.DecisionSurface, eval.Outcome, eval.ReasonCode, error) {
	// First check if the surface exists at all using GetByID
	// (FindActiveAt would return nil for both non-existent and inactive surfaces)
	s, err := surfaces.FindLatestByID(ctx, surfaceID)
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

// resolveAuthorityChain finds the active grant and profile for an agent
// on the given surface at the given time. Checks both effective date
// (FindActiveAt) and profile.Status == active.
//
// Outcome semantics:
//   - NO_ACTIVE_GRANT: agent has no grants at all
//   - PROFILE_NOT_FOUND: grants exist, but none have an active profile
//   - GRANT_PROFILE_SURFACE_MISMATCH: active profile exists, but doesn't match the surface
//   - Success: returns grant + profile
//
// PROFILE_NOT_FOUND indicates incomplete configuration (missing profile);
// GRANT_PROFILE_SURFACE_MISMATCH indicates wrong configuration (agent authorized
// for a different surface). Preserve this distinction in tests.
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

		if p.Status != authority.ProfileStatusActive {
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
	startedAt := o.clock()
	result, err := o.policies.Evaluate(ctx, policy.PolicyInput{
		SurfaceID: req.SurfaceID,
		AgentID:   req.AgentID,
		Context:   req.Context,
	})
	endedAt := o.clock()
	duration := endedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}

	if err != nil {
		slog.Error("policy_evaluation_failed",
			"request_id", req.RequestID,
			"policy_reference", p.PolicyReference,
			"fail_mode", p.FailMode,
			"duration_ms", duration.Milliseconds(),
			"error", err,
		)
		if p.FailMode == authority.FailModeOpen {
			slog.Warn("policy_fail_open_applied", "request_id", req.RequestID, "policy_reference", p.PolicyReference)
			return "", "", nil
		}
		return eval.OutcomeEscalate, eval.ReasonPolicyError, nil
	}
	if !result.Allowed {
		slog.Info("policy_denied", "request_id", req.RequestID, "policy_reference", p.PolicyReference,
			"duration_ms", duration.Milliseconds())
		return eval.OutcomeEscalate, eval.ReasonPolicyDeny, nil
	}
	return "", "", nil
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------
func buildExplanationText(env *envelope.Envelope, outcome eval.Outcome, reason eval.ReasonCode) string {
	if env == nil || env.Evaluation.Explanation == nil {
		return string(reason)
	}

	exp := env.Evaluation.Explanation

	switch reason {
	case eval.ReasonWithinAuthority:
		return "Request is within granted authority and may proceed."

	case eval.ReasonInsufficientContext:
		return "Required context is missing; more information is needed before a decision can be made."

	case eval.ReasonConfidenceBelowThreshold:
		return fmt.Sprintf(
			"Confidence %.2f is below required threshold %.2f.",
			exp.ConfidenceProvided,
			exp.ConfidenceThreshold,
		)

	case eval.ReasonConsequenceExceedsLimit:
		// Prefer monetary wording when amounts are present.
		if exp.ConsequenceProvidedType == string(value.ConsequenceTypeMonetary) ||
			exp.ConsequenceThresholdType == string(value.ConsequenceTypeMonetary) {
			return fmt.Sprintf(
				"Consequence %.2f %s exceeds allowed threshold %.2f %s.",
				exp.ConsequenceProvidedAmount,
				exp.ConsequenceProvidedCurrency,
				exp.ConsequenceThresholdAmount,
				exp.ConsequenceThresholdCurrency,
			)
		}

		// Fall back to risk-rating wording.
		if exp.ConsequenceProvidedRiskRating != "" || exp.ConsequenceThresholdRiskRating != "" {
			return fmt.Sprintf(
				"Consequence risk rating %s exceeds allowed threshold %s.",
				exp.ConsequenceProvidedRiskRating,
				exp.ConsequenceThresholdRiskRating,
			)
		}

		return "Submitted consequence exceeds the allowed threshold."

	case eval.ReasonPolicyError:
		if exp.PolicyReference != "" {
			return fmt.Sprintf(
				"Policy evaluation failed for policy %s and fail-closed handling requires escalation.",
				exp.PolicyReference,
			)
		}
		return "Policy evaluation failed and requires escalation."

	case eval.ReasonPolicyDeny:
		if exp.PolicyReference != "" {
			return fmt.Sprintf(
				"Policy %s did not permit the request and escalation is required.",
				exp.PolicyReference,
			)
		}
		return "Policy evaluation did not permit the request and escalation is required."

	case eval.ReasonSurfaceNotFound:
		return "The requested decision surface was not found."

	case eval.ReasonSurfaceInactive:
		return "The requested decision surface is not active."

	case eval.ReasonAgentNotFound:
		return "The specified agent was not found."

	case eval.ReasonNoActiveGrant:
		return "The agent does not have an active authority grant."

	case eval.ReasonProfileNotFound:
		return "No active authority profile could be resolved for the agent."

	case eval.ReasonGrantProfileSurfaceMismatch:
		return "The agent's authority profile does not apply to the requested surface."

	default:
		// Keep a safe fallback for newer reason codes.
		if outcome != "" {
			return fmt.Sprintf("Outcome %s was returned for reason %s.", outcome, reason)
		}
		return string(reason)
	}
}

func buildGovernancePayload(req eval.DecisionRequest, submittedHash string) map[string]any {
	p := map[string]any{
		"request_source": req.RequestSource,
		"surface_id":     req.SurfaceID,
		"agent_id":       req.AgentID,
		"submitted_hash": submittedHash,
	}
	if req.ActionType != "" {
		p["action_type"] = req.ActionType
	}
	if req.ActionDesc != "" {
		p["action_description"] = req.ActionDesc
	}
	if req.AgentKind != "" {
		p["agent_kind"] = req.AgentKind
	}
	if req.Runtime != nil {
		p["agent_runtime_model"] = req.Runtime.Model
		p["agent_runtime_version"] = req.Runtime.Version
	}
	if req.Delegation != nil {
		p["delegation_initiated_by"] = req.Delegation.InitiatedBy
		p["delegation_session_id"] = req.Delegation.SessionID
		p["delegation_chain"] = req.Delegation.Chain
		p["delegation_scope"] = req.Delegation.Scope
	}
	if req.ExpiresAt != "" {
		p["expires_at"] = req.ExpiresAt
	}
	if req.Subject != nil {
		p["subject_type"] = req.Subject.Type
		p["subject_id"] = req.Subject.ID
	}
	return p
}

// resultFromEnvelope reconstructs an EvaluationResult from a persisted
// envelope. Used on the exact-replay idempotency path to return the original
// decision without creating a second envelope.
func resultFromEnvelope(env *envelope.Envelope) EvaluationResult {
	var explanation string
	if env.Evaluation.Explanation != nil {
		explanation = env.Evaluation.Explanation.Reason
	}
	return EvaluationResult{
		Outcome:     env.Evaluation.Outcome,
		ReasonCode:  env.Evaluation.ReasonCode,
		EnvelopeID:  env.ID(),
		State:       env.State,
		Explanation: explanation,
	}
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

// classifyFailure extracts the failure category from an error.
//
// Priority order:
//  1. Explicit categorizedError wrapper (set by wrapFailure at orchestrator
//     boundaries for transition and persist failures).
//  2. Known sentinel errors (ErrEnvelopeNotFound, etc.).
//  3. Heuristic string matching — retained only for authority-resolution
//     repository errors and policy errors that are returned raw without wrapping.
func classifyFailure(err error) string {
	if err == nil {
		return ""
	}

	// First priority: Explicit category wrapper.
	var catErr *categorizedError
	if errors.As(err, &catErr) {
		return string(catErr.Category())
	}

	// Second priority: Known sentinel errors.
	switch {
	case errors.Is(err, ErrEnvelopeNotFound),
		errors.Is(err, ErrEnvelopeNotAwaitingReview),
		errors.Is(err, ErrEnvelopeAlreadyClosed):
		return string(FailureCategoryResolveReview)
	case errors.Is(err, ErrScopedRequestConflict):
		return string(FailureCategoryIdempotencyConflict)
	}

	// Third priority: Heuristic fallback for raw repository errors from
	// authority resolution (surface, agent, grant, profile lookups) and
	// policy evaluation. Transition and persist failures are now covered by
	// explicit wrappers above, so those strings are no longer needed here.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "policy"):
		return string(FailureCategoryPolicyEvaluation)

	case strings.Contains(msg, "authority") ||
		strings.Contains(msg, "grant") ||
		strings.Contains(msg, "profile") ||
		strings.Contains(msg, "surface") ||
		strings.Contains(msg, "agent"):
		return string(FailureCategoryAuthorityResolution)

	default:
		return string(FailureCategoryUnknown)
	}
}
