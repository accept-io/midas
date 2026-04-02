package envelope

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/eval"
)

// SchemaVersion identifies the envelope format version.
// Increment when the persisted structure changes in a non-additive way.
const SchemaVersion = 1

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// EnvelopeState represents the lifecycle state of an Envelope.
type EnvelopeState string

const (
	EnvelopeStateReceived        EnvelopeState = "received"
	EnvelopeStateEvaluating      EnvelopeState = "evaluating"
	EnvelopeStateOutcomeRecorded EnvelopeState = "outcome_recorded"
	EnvelopeStateEscalated       EnvelopeState = "escalated"
	EnvelopeStateAwaitingReview  EnvelopeState = "awaiting_review"
	EnvelopeStateClosed          EnvelopeState = "closed"
)

// validTransitions defines the allowed state machine edges.
var validTransitions = map[EnvelopeState][]EnvelopeState{
	EnvelopeStateReceived:        {EnvelopeStateEvaluating},
	EnvelopeStateEvaluating:      {EnvelopeStateOutcomeRecorded, EnvelopeStateEscalated},
	EnvelopeStateOutcomeRecorded: {EnvelopeStateClosed},
	EnvelopeStateEscalated:       {EnvelopeStateAwaitingReview},
	EnvelopeStateAwaitingReview:  {EnvelopeStateClosed},
}

// Sentinel errors
var (
	ErrInvalidTransition  = errors.New("invalid envelope state transition")
	ErrMissingExplanation = errors.New("explanation must be set before recording outcome")
	ErrMissingOutcome     = errors.New("outcome and reason must be set before closing")
	ErrMissingReview      = errors.New("escalation review must be set before closing an escalated envelope")
	ErrEnvelopeClosed     = errors.New("envelope is closed and cannot be mutated")
	ErrNotEscalated       = errors.New("envelope is not in an escalated state")
)

// ---------------------------------------------------------------------------
// Section 1: Identity
// ---------------------------------------------------------------------------

// Identity holds the stable, immutable identifiers for an envelope.
type Identity struct {
	ID            string `json:"id"`
	RequestSource string `json:"request_source"` // Schema v2.1: source system identifier
	RequestID     string `json:"request_id"`
	SchemaVersion int    `json:"schema_version"`
}

// ---------------------------------------------------------------------------
// Section 2: Submitted
//
// Verbatim snapshot of the original API request as received.
// Stored as a raw JSON blob so it is immutable, hashable, and decoupled
// from any internal type mapping. This is the canonical record of what
// the caller asked MIDAS to govern.
// ---------------------------------------------------------------------------

// Submitted holds the original governance envelope exactly as submitted.
type Submitted struct {
	// Raw is the verbatim JSON payload received at the API boundary.
	// It is set once at envelope creation and never modified.
	Raw json.RawMessage `json:"raw"`

	// ReceivedAt is the wall-clock time the request was accepted by MIDAS.
	ReceivedAt time.Time `json:"received_at"`
}

// ---------------------------------------------------------------------------
// Section 3: Resolved
//
// Facts that MIDAS determined during evaluation — distinct from claims
// the caller supplied. These are the authoritative internal references.
// ---------------------------------------------------------------------------

// ResolvedAuthority holds versioned references to the authority chain
// MIDAS resolved for this evaluation. These are internal facts, not
// caller-supplied claims.
type ResolvedAuthority struct {
	SurfaceID      string `json:"surface_id"`
	SurfaceVersion int    `json:"surface_version"`
	ProfileID      string `json:"profile_id"`
	ProfileVersion int    `json:"profile_version"`
	AgentID        string `json:"agent_id"`
	GrantID        string `json:"grant_id"`
}

// RequestMetadata holds governance fields extracted from the submitted
// envelope for convenience. These are caller-supplied claims — present
// for readability and audit enrichment, but their authority derives from
// the Submitted.Raw blob, not from this struct.
type RequestMetadata struct {
	ActionType          string `json:"action_type,omitempty"`
	ActionDescription   string `json:"action_description,omitempty"`
	AgentKind           string `json:"agent_kind,omitempty"`
	AgentRuntimeModel   string `json:"agent_runtime_model,omitempty"`
	AgentRuntimeVersion string `json:"agent_runtime_version,omitempty"`
}

// DelegationEvidence holds delegation claims extracted from the submitted
// envelope. These are caller assertions — MIDAS records them but does not
// currently independently verify them beyond scope cross-check.
type DelegationEvidence struct {
	InitiatedBy     string   `json:"initiated_by,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	Chain           []string `json:"chain,omitempty"`
	Scope           []string `json:"scope,omitempty"`
	AuthorizedAt    string   `json:"authorized_at,omitempty"`
	AuthorizedUntil string   `json:"authorized_until,omitempty"`
}

// DecisionSubject identifies the entity that is the target of the governed
// action. Capturing it as a first-class field enables subject-based audit
// trails, cross-envelope correlation, and regulator-friendly traceability
// without requiring context map parsing.
//
// Examples: a customer whose loan is being approved, an account on which a
// payment is being executed, a file being modified by an automated process.
type DecisionSubject struct {
	// Type identifies the kind of entity, e.g. "customer", "account", "file".
	Type string `json:"type"`

	// ID is the primary identifier for the subject within its type.
	ID string `json:"id"`

	// SecondaryIDs holds additional identifiers for the same subject.
	// Keys should be namespaced, e.g. "crm:id", "ledger:account_ref".
	SecondaryIDs map[string]string `json:"secondary_ids,omitempty"`
}

// Resolved groups all facts determined by MIDAS during evaluation.
type Resolved struct {
	Authority  ResolvedAuthority   `json:"authority"`
	Metadata   RequestMetadata     `json:"metadata,omitempty"`
	Delegation *DelegationEvidence `json:"delegation,omitempty"`
	Subject    *DecisionSubject    `json:"subject,omitempty"`
}

// ---------------------------------------------------------------------------
// Section 4: Evaluation
//
// The outcome, rationale, and full decision explanation.
// ---------------------------------------------------------------------------

// DecisionExplanation records the threshold and policy evaluation that
// produced the outcome. It captures both inputs and thresholds so the
// reasoning is self-contained and reproducible from the record alone.
type DecisionExplanation struct {
	// Confidence
	ConfidenceProvided  float64 `json:"confidence_provided"`
	ConfidenceThreshold float64 `json:"confidence_threshold"`

	// Consequence submitted by caller
	ConsequenceProvidedType       string  `json:"consequence_provided_type,omitempty"`
	ConsequenceProvidedAmount     float64 `json:"consequence_provided_amount,omitempty"`
	ConsequenceProvidedCurrency   string  `json:"consequence_provided_currency,omitempty"`
	ConsequenceProvidedRiskRating string  `json:"consequence_provided_risk_rating,omitempty"`
	ConsequenceReversible         bool    `json:"consequence_reversible,omitempty"`

	// Consequence threshold from authority profile
	ConsequenceThresholdType       string  `json:"consequence_threshold_type,omitempty"`
	ConsequenceThresholdAmount     float64 `json:"consequence_threshold_amount,omitempty"`
	ConsequenceThresholdCurrency   string  `json:"consequence_threshold_currency,omitempty"`
	ConsequenceThresholdRiskRating string  `json:"consequence_threshold_risk_rating,omitempty"`

	// Policy evaluation
	PolicyEvaluated      bool   `json:"policy_evaluated"`
	PolicyReference      string `json:"policy_reference,omitempty"`
	PolicyPackageVersion string `json:"policy_package_version,omitempty"`
	PolicyDecisionID     string `json:"policy_decision_id,omitempty"`

	// Governance checks
	DelegationValidated   bool   `json:"delegation_validated"`
	ActionWithinScope     bool   `json:"action_within_scope"`
	AuthorizationActiveAt string `json:"authorization_active_at,omitempty"`

	// OutcomeDriver identifies which evaluation step determined the result.
	// Values: "threshold.confidence", "threshold.consequence", "policy",
	//         "delegation", "context", "authority"
	OutcomeDriver string `json:"outcome_driver,omitempty"`

	// Final result
	Result string `json:"result"`
	Reason string `json:"reason"`
}

// Evaluation groups the outcome and its full rationale.
type Evaluation struct {
	Outcome     eval.Outcome         `json:"outcome,omitempty"`
	ReasonCode  eval.ReasonCode      `json:"reason_code,omitempty"`
	Explanation *DecisionExplanation `json:"explanation,omitempty"`
	EvaluatedAt *time.Time           `json:"evaluated_at,omitempty"`
}

// ---------------------------------------------------------------------------
// EscalationReview — reviewer decision on an escalated envelope
// ---------------------------------------------------------------------------

// ReviewDecision is the outcome of a human or system review of an escalation.
type ReviewDecision string

const (
	ReviewDecisionApproved ReviewDecision = "APPROVED"
	ReviewDecisionRejected ReviewDecision = "REJECTED"
)

// EscalationReview records the review decision made on an escalated envelope.
// It is set by the ResolveEscalation operation and must be present before
// an escalated envelope can be closed.
type EscalationReview struct {
	// Decision is the reviewer's outcome: APPROVED or REJECTED.
	Decision ReviewDecision `json:"decision"`

	// ReviewerID identifies who or what recorded the review decision.
	ReviewerID string `json:"reviewer_id"`

	// ReviewerKind distinguishes human reviewers from automated systems.
	// Values: "human", "system"
	ReviewerKind string `json:"reviewer_kind,omitempty"`

	// Notes is an optional free-text justification for the decision.
	Notes string `json:"notes,omitempty"`

	// ReviewedAt is the wall-clock time the review was recorded.
	ReviewedAt time.Time `json:"reviewed_at"`
}

// ---------------------------------------------------------------------------
// Section 5: Integrity
//
// Audit linkage and hash chain references. These make the envelope
// tamper-evident beyond the application record layer.
// ---------------------------------------------------------------------------

// IntegrityRecord holds the audit event chain references and hash anchors
// for this envelope. Together with the append-only audit log, this makes
// the envelope verifiable independently of the application database.
type IntegrityRecord struct {
	// AuditEventIDs is the ordered list of audit event IDs produced
	// during this evaluation, used to reconstruct the full event chain.
	AuditEventIDs []string `json:"audit_event_ids,omitempty"`

	// FirstEventHash is the hash of the first audit event in the chain,
	// used as an external anchor for integrity verification.
	FirstEventHash string `json:"first_event_hash,omitempty"`

	// FinalEventHash is the hash of the last audit event appended,
	// representing the terminal state of the chain for this envelope.
	FinalEventHash string `json:"final_event_hash,omitempty"`

	// SubmittedHash is a SHA-256 hex digest of Submitted.Raw.
	// It proves the original request has not been altered since receipt.
	SubmittedHash string `json:"submitted_hash,omitempty"`
}

// ---------------------------------------------------------------------------
// Envelope — the complete governance record
// ---------------------------------------------------------------------------

// Envelope is the durable governance artifact for a single evaluation.
// It is structured in five explicit sections:
//
//	Identity   — stable immutable identifiers and schema version
//	Submitted  — verbatim original request snapshot (raw JSON blob)
//	Resolved   — facts MIDAS determined (authority chain, grant, profile versions)
//	Evaluation — outcome, rationale, and decision explanation
//	Integrity  — audit event linkage and hash anchors
//
// Once CLOSED, the envelope must not be mutated. All fields are designed
// to be self-contained: an auditor should be able to reconstruct the full
// governance picture from a single envelope record without needing to join
// against other tables.
type Envelope struct {
	// Section 1: Identity
	Identity Identity `json:"identity"`

	// Section 2: Submitted
	Submitted Submitted `json:"submitted"`

	// Section 3: Resolved
	Resolved Resolved `json:"resolved"`

	// Section 4: Evaluation
	Evaluation Evaluation `json:"evaluation"`

	// Section 5: Integrity
	Integrity IntegrityRecord `json:"integrity"`

	// Review is populated only for escalated envelopes, once a reviewer
	// has recorded their decision via ResolveEscalation.
	Review *EscalationReview `json:"review,omitempty"`

	// Lifecycle
	State     EnvelopeState `json:"state"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	ClosedAt  *time.Time    `json:"closed_at,omitempty"`

	// ---------------------------------------------------------------------------
	// Schema v2.1: Denormalized Authority Chain
	//
	// These fields duplicate data from Resolved.Authority for database-level
	// indexing, querying, and foreign key constraints. The JSON section remains
	// the canonical source for API responses and audit records.
	//
	// Populated during evaluation after authority resolution completes.
	// ---------------------------------------------------------------------------
	ResolvedSurfaceID      string `json:"-"` // Denormalized from Resolved.Authority.SurfaceID
	ResolvedSurfaceVersion int    `json:"-"` // Denormalized from Resolved.Authority.SurfaceVersion
	ResolvedProfileID      string `json:"-"` // Denormalized from Resolved.Authority.ProfileID
	ResolvedProfileVersion int    `json:"-"` // Denormalized from Resolved.Authority.ProfileVersion
	ResolvedGrantID        string `json:"-"` // Denormalized from Resolved.Authority.GrantID
	ResolvedAgentID        string `json:"-"` // Denormalized from Resolved.Authority.AgentID
	ResolvedSubjectID      string `json:"-"` // Denormalized from Resolved.Subject.ID (if present)
}

// Convenience accessors keep callers readable without reaching into nested structs.
func (e *Envelope) ID() string                  { return e.Identity.ID }
func (e *Envelope) RequestSource() string       { return e.Identity.RequestSource }
func (e *Envelope) RequestID() string           { return e.Identity.RequestID }
func (e *Envelope) Outcome() eval.Outcome       { return e.Evaluation.Outcome }
func (e *Envelope) ReasonCode() eval.ReasonCode { return e.Evaluation.ReasonCode }

// ---------------------------------------------------------------------------
// Transition — state machine with content invariants
// ---------------------------------------------------------------------------

// Transition advances the envelope to the next state, enforcing both
// structural (allowed edge) and content (required fields) invariants.
// now is passed explicitly for deterministic testing.
func (e *Envelope) Transition(next EnvelopeState, now time.Time) error {
	if e.State == EnvelopeStateClosed {
		return ErrEnvelopeClosed
	}

	allowed := validTransitions[e.State]
	for _, s := range allowed {
		if s == next {
			if err := e.checkInvariantsFor(next); err != nil {
				return err
			}
			e.State = next
			e.UpdatedAt = now
			if next == EnvelopeStateClosed {
				e.ClosedAt = &now
			}
			return nil
		}
	}

	return ErrInvalidTransition
}

// checkInvariantsFor enforces content requirements before allowing a transition.
func (e *Envelope) checkInvariantsFor(next EnvelopeState) error {
	switch next {
	case EnvelopeStateOutcomeRecorded, EnvelopeStateEscalated:
		if e.Evaluation.Explanation == nil {
			return ErrMissingExplanation
		}
	case EnvelopeStateClosed:
		if e.Evaluation.Outcome == "" || e.Evaluation.ReasonCode == "" {
			return ErrMissingOutcome
		}
		// Escalated envelopes must have a recorded review before closing.
		if e.State == EnvelopeStateAwaitingReview && e.Review == nil {
			return ErrMissingReview
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// New creates a new Envelope in the RECEIVED state with the submitted
// raw request blob captured. Returns an error if raw is not valid JSON.
//
// Schema v2.1: requestSource scopes the idempotency key.
func New(id, requestSource, requestID string, raw json.RawMessage, now time.Time) (*Envelope, error) {
	if !json.Valid(raw) {
		return nil, errors.New("submitted raw payload is not valid JSON")
	}

	return &Envelope{
		Identity: Identity{
			ID:            id,
			RequestSource: requestSource,
			RequestID:     requestID,
			SchemaVersion: SchemaVersion,
		},
		Submitted: Submitted{
			Raw:        raw,
			ReceivedAt: now,
		},
		State:     EnvelopeStateReceived,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ---------------------------------------------------------------------------
// Repository
// ---------------------------------------------------------------------------

// ErrEnvelopeScopeConflict is returned by EnvelopeRepository.Create when a
// concurrent insert races to the DB UNIQUE constraint on (request_source,
// request_id) and loses. The caller should treat this as an idempotency
// collision rather than a generic persistence failure.
var ErrEnvelopeScopeConflict = errors.New("envelope already exists for this request scope")

// EnvelopeRepository defines persistence operations for Envelope.
type EnvelopeRepository interface {
	GetByID(ctx context.Context, id string) (*Envelope, error)
	// GetByRequestID retrieves by (request_source, request_id) composite key in schema v2.1
	GetByRequestID(ctx context.Context, requestID string) (*Envelope, error)
	// GetByRequestScope retrieves by (request_source, request_id) - preferred for schema v2.1
	GetByRequestScope(ctx context.Context, requestSource, requestID string) (*Envelope, error)
	List(ctx context.Context) ([]*Envelope, error)
	// ListByState returns all envelopes in the given lifecycle state.
	// An empty state returns all envelopes (equivalent to List).
	ListByState(ctx context.Context, state EnvelopeState) ([]*Envelope, error)
	Create(ctx context.Context, env *Envelope) error
	Update(ctx context.Context, env *Envelope) error
}
