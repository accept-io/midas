package envelope

import (
	"context"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/eval"
)

// EnvelopeState represents the lifecycle state of an Envelope.
type EnvelopeState string

const (
	EnvelopeStateReceived        EnvelopeState = "RECEIVED"
	EnvelopeStateEvaluating      EnvelopeState = "EVALUATING"
	EnvelopeStateOutcomeRecorded EnvelopeState = "OUTCOME_RECORDED"
	EnvelopeStateEscalated       EnvelopeState = "ESCALATED"
	EnvelopeStateClosed          EnvelopeState = "CLOSED"
)

// validTransitions defines the allowed state machine edges.
var validTransitions = map[EnvelopeState][]EnvelopeState{
	EnvelopeStateReceived:        {EnvelopeStateEvaluating},
	EnvelopeStateEvaluating:      {EnvelopeStateOutcomeRecorded, EnvelopeStateEscalated},
	EnvelopeStateOutcomeRecorded: {EnvelopeStateClosed},
	EnvelopeStateEscalated:       {EnvelopeStateClosed},
}

// ErrInvalidTransition is returned when a state transition is not permitted.
var ErrInvalidTransition = errors.New("invalid envelope state transition")

// Evidence holds versioned references to the resolved authority chain.
// References are stored, not copies of full configuration.
type Evidence struct {
	SurfaceID      string
	SurfaceVersion int
	ProfileID      string
	ProfileVersion int
	AgentID        string
}
type DecisionExplanation struct {
	SurfaceID string `json:"surface_id"`
	AgentID   string `json:"agent_id"`

	ConfidenceProvided  float64 `json:"confidence_provided"`
	ConfidenceThreshold float64 `json:"confidence_threshold"`

	ConsequenceProvidedType       string  `json:"consequence_provided_type,omitempty"`
	ConsequenceProvidedAmount     float64 `json:"consequence_provided_amount,omitempty"`
	ConsequenceProvidedCurrency   string  `json:"consequence_provided_currency,omitempty"`
	ConsequenceProvidedRiskRating string  `json:"consequence_provided_risk_rating,omitempty"`

	ConsequenceThresholdType       string  `json:"consequence_threshold_type,omitempty"`
	ConsequenceThresholdAmount     float64 `json:"consequence_threshold_amount,omitempty"`
	ConsequenceThresholdCurrency   string  `json:"consequence_threshold_currency,omitempty"`
	ConsequenceThresholdRiskRating string  `json:"consequence_threshold_risk_rating,omitempty"`

	PolicyEvaluated bool   `json:"policy_evaluated"`
	Result          string `json:"result"`
	Reason          string `json:"reason"`
}

// Envelope is the lifecycle object for a single evaluation request.
type Envelope struct {
	ID          string
	RequestID   string
	State       EnvelopeState
	Evidence    Evidence
	Explanation *DecisionExplanation
	Outcome     eval.Outcome
	ReasonCode  eval.ReasonCode
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
}

// Transition advances the envelope to the next state.
func (e *Envelope) Transition(next EnvelopeState) error {
	allowed := validTransitions[e.State]
	for _, s := range allowed {
		if s == next {
			now := time.Now().UTC()
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

// EnvelopeRepository defines persistence operations for Envelope.
type EnvelopeRepository interface {
	GetByID(ctx context.Context, id string) (*Envelope, error)
	GetByRequestID(ctx context.Context, requestID string) (*Envelope, error)
	List(ctx context.Context) ([]*Envelope, error)
	Create(ctx context.Context, env *Envelope) error
	Update(ctx context.Context, env *Envelope) error
}
