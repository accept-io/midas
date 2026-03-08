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

// Envelope is the lifecycle object for a single evaluation request.
type Envelope struct {
	ID         string
	RequestID  string
	State      EnvelopeState
	Evidence   Evidence
	Outcome    eval.Outcome
	ReasonCode eval.ReasonCode
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ClosedAt   *time.Time
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
	List(ctx context.Context) ([]*Envelope, error)
	Create(ctx context.Context, env *Envelope) error
	Update(ctx context.Context, env *Envelope) error
}
