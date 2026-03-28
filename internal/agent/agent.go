package agent

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidAgentType is returned when an AgentType value is not one of the
// canonical domain values (ai, service, operator).
var ErrInvalidAgentType = errors.New("invalid agent type")

// AgentType classifies what kind of actor an agent is.
type AgentType string

const (
	AgentTypeAI       AgentType = "ai"
	AgentTypeService  AgentType = "service"
	AgentTypeOperator AgentType = "operator"
)

// IsValid reports whether t is one of the canonical AgentType values.
func (t AgentType) IsValid() bool {
	switch t {
	case AgentTypeAI, AgentTypeService, AgentTypeOperator:
		return true
	}
	return false
}

// Validate returns ErrInvalidAgentType (wrapped with the offending value) if
// the type is not canonical. Callers should use errors.Is to check.
func (t AgentType) Validate() error {
	if !t.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidAgentType, string(t))
	}
	return nil
}

// OperationalState represents whether an agent is permitted to act globally.
// This is independent of authority grants, which are managed separately.
type OperationalState string

const (
	OperationalStateActive    OperationalState = "active"
	OperationalStateSuspended OperationalState = "suspended"
	OperationalStateRevoked   OperationalState = "revoked"
)

// Agent represents an autonomous actor (AI agent, automated service, human operator).
type Agent struct {
	ID               string
	Name             string
	Type             AgentType
	Owner            string
	ModelVersion     string // applicable when Type == AgentTypeAI
	Endpoint         string // service or model endpoint used for invocation (if applicable)
	OperationalState OperationalState
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AgentRepository is the persistence interface for Agent.
// All implementations live in internal/store/postgres.
type AgentRepository interface {
	GetByID(ctx context.Context, id string) (*Agent, error)
	Create(ctx context.Context, a *Agent) error
	Update(ctx context.Context, a *Agent) error
	List(ctx context.Context) ([]*Agent, error)
}
