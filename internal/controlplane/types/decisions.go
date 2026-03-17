package types

// API version and resource kind constants for MIDAS control plane.
const (
	APIVersionV1 = "midas.accept.io/v1"

	KindSurface = "Surface"
	KindAgent   = "Agent"
	KindProfile = "Profile"
	KindGrant   = "Grant"
)

// Document is the common interface implemented by all control plane documents.
type Document interface {
	GetKind() string
	GetID() string
}

// DocumentMetadata contains common metadata fields for all control plane resources.
type DocumentMetadata struct {
	ID     string            `json:"id" yaml:"id"`
	Name   string            `json:"name,omitempty" yaml:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// ---------------------------------------------------------------------------
// Surface
// ---------------------------------------------------------------------------

// SurfaceDocument defines a governed action boundary.
type SurfaceDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       SurfaceSpec      `json:"spec" yaml:"spec"`
}

// SurfaceSpec contains the specification for a decision surface.
type SurfaceSpec struct {
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Category    string `json:"category,omitempty" yaml:"category,omitempty"`   // financial | customer_data | compliance | operational
	RiskTier    string `json:"risk_tier,omitempty" yaml:"risk_tier,omitempty"` // high | medium | low
	Status      string `json:"status,omitempty" yaml:"status,omitempty"`       // active | inactive | deprecated
}

// GetKind returns the resource kind.
func (s SurfaceDocument) GetKind() string { return s.Kind }

// GetID returns the resource ID.
func (s SurfaceDocument) GetID() string { return s.Metadata.ID }

// ---------------------------------------------------------------------------
// Agent
// ---------------------------------------------------------------------------

// AgentDocument defines a non-human or system identity.
type AgentDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       AgentSpec        `json:"spec" yaml:"spec"`
}

// AgentSpec contains the specification for an agent.
type AgentSpec struct {
	Type    string       `json:"type,omitempty" yaml:"type,omitempty"` // llm_agent | workflow | automation | copilot | rpa
	Runtime AgentRuntime `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Status  string       `json:"status,omitempty" yaml:"status,omitempty"` // active | inactive
}

// AgentRuntime contains runtime metadata for an agent.
type AgentRuntime struct {
	Model    string `json:"model,omitempty" yaml:"model,omitempty"`
	Version  string `json:"version,omitempty" yaml:"version,omitempty"`
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// GetKind returns the resource kind.
func (a AgentDocument) GetKind() string { return a.Kind }

// GetID returns the resource ID.
func (a AgentDocument) GetID() string { return a.Metadata.ID }

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

// ProfileDocument defines an authority policy envelope.
type ProfileDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       ProfileSpec      `json:"spec" yaml:"spec"`
}

// ProfileSpec contains the specification for an authority profile.
type ProfileSpec struct {
	SurfaceID         string                   `json:"surface_id" yaml:"surface_id"`
	Authority         ProfileAuthority         `json:"authority,omitempty" yaml:"authority,omitempty"`
	InputRequirements ProfileInputRequirements `json:"input_requirements,omitempty" yaml:"input_requirements,omitempty"`
	Policy            ProfilePolicy            `json:"policy,omitempty" yaml:"policy,omitempty"`
	Lifecycle         ProfileLifecycle         `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`
}

// ProfileAuthority defines the authority limits for a profile.
type ProfileAuthority struct {
	DecisionConfidenceThreshold float64              `json:"decision_confidence_threshold,omitempty" yaml:"decision_confidence_threshold,omitempty"`
	ConsequenceThreshold        ConsequenceThreshold `json:"consequence_threshold,omitempty" yaml:"consequence_threshold,omitempty"`
}

// ConsequenceThreshold defines limits on decision consequences.
type ConsequenceThreshold struct {
	Type       string  `json:"type,omitempty" yaml:"type,omitempty"`               // monetary | data_access | risk_rating
	Amount     float64 `json:"amount,omitempty" yaml:"amount,omitempty"`           // for monetary type
	Currency   string  `json:"currency,omitempty" yaml:"currency,omitempty"`       // for monetary type
	RiskRating string  `json:"risk_rating,omitempty" yaml:"risk_rating,omitempty"` // for risk_rating type
}

// ProfileInputRequirements defines required context for decisions.
type ProfileInputRequirements struct {
	RequiredContext []string `json:"required_context,omitempty" yaml:"required_context,omitempty"`
}

// ProfilePolicy defines policy evaluation settings.
type ProfilePolicy struct {
	Reference string `json:"reference,omitempty" yaml:"reference,omitempty"` // e.g. rego://payments/auto_approve_v1
	FailMode  string `json:"fail_mode,omitempty" yaml:"fail_mode,omitempty"` // closed | open
}

// ProfileLifecycle defines lifecycle metadata for a profile.
type ProfileLifecycle struct {
	Status         string `json:"status,omitempty" yaml:"status,omitempty"`                   // active | inactive | deprecated
	EffectiveFrom  string `json:"effective_from,omitempty" yaml:"effective_from,omitempty"`   // RFC3339 timestamp
	EffectiveUntil string `json:"effective_until,omitempty" yaml:"effective_until,omitempty"` // RFC3339 timestamp
	Version        int    `json:"version,omitempty" yaml:"version,omitempty"`
}

// GetKind returns the resource kind.
func (p ProfileDocument) GetKind() string { return p.Kind }

// GetID returns the resource ID.
func (p ProfileDocument) GetID() string { return p.Metadata.ID }

// ---------------------------------------------------------------------------
// Grant
// ---------------------------------------------------------------------------

// GrantDocument assigns a profile to an agent for a time period.
type GrantDocument struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Spec       GrantSpec        `json:"spec" yaml:"spec"`
}

// GrantSpec contains the specification for a grant.
type GrantSpec struct {
	AgentID        string            `json:"agent_id" yaml:"agent_id"`
	ProfileID      string            `json:"profile_id" yaml:"profile_id"`
	GrantedBy      string            `json:"granted_by,omitempty" yaml:"granted_by,omitempty"`
	GrantedAt      string            `json:"granted_at,omitempty" yaml:"granted_at,omitempty"`
	EffectiveFrom  string            `json:"effective_from,omitempty" yaml:"effective_from,omitempty"`
	EffectiveUntil string            `json:"effective_until,omitempty" yaml:"effective_until,omitempty"`
	Status         string            `json:"status,omitempty" yaml:"status,omitempty"` // active | suspended | revoked | expired
	Metadata       map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// GetKind returns the resource kind.
func (g GrantDocument) GetKind() string { return g.Kind }

// GetID returns the resource ID.
func (g GrantDocument) GetID() string { return g.Metadata.ID }
