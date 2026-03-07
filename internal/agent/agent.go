package agent

import (
	"context"
	"time"
)

// AgentType classifies what kind of actor an agent is.
type AgentType string

const (
	AgentTypeAI       AgentType = "ai"
	AgentTypeService  AgentType = "service"
	AgentTypeOperator AgentType = "operator"
)

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
