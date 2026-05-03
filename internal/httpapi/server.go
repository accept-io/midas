package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/accept-io/midas/internal/adminaudit"
	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/authz"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/approval"
	cpTypes "github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/externalref"
	"github.com/accept-io/midas/internal/governancecoverage"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/oidc"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

const (
	maxRequestBodyBytes  = 1 << 20  // 1 MiB
	maxApplyBodyBytes    = 10 << 20 // 10 MiB for YAML bundles
	defaultRequestSource = "api"
	maxIdentifierLength  = 255
)

// orchestrator defines the narrow application contract required by the HTTP API.
// It is intentionally owned by the consumer (httpapi) rather than the producer (decision).
type orchestrator interface {
	Evaluate(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error)
	Simulate(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error)
	ResolveEscalation(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error)
	GetEnvelopeByID(ctx context.Context, id string) (*envelope.Envelope, error)
	GetEnvelopeByRequestScope(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error)
	ListEnvelopes(ctx context.Context) ([]*envelope.Envelope, error)
	ListEnvelopesByState(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error)
}

// controlPlaneService defines the contract for control plane operations.
// This is optional - if nil, control plane endpoints return 501 Not Implemented.
type controlPlaneService interface {
	// ApplyBundle parses and applies a YAML bundle. actor identifies who initiated
	// the apply (e.g. from X-MIDAS-ACTOR header); empty string falls back to "system".
	ApplyBundle(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error)
	PlanBundle(ctx context.Context, bundle []byte) (*apply.ApplyPlan, error)
}

type approvalService interface {
	ApproveSurface(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error)
	DeprecateSurface(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error)
	ApproveProfile(ctx context.Context, profileID string, version int, approvedBy string) (*authority.AuthorityProfile, error)
	DeprecateProfile(ctx context.Context, profileID string, version int, deprecatedBy string) (*authority.AuthorityProfile, error)
	ApproveGovernanceExpectation(ctx context.Context, expectationID string, version int, approvedBy string) (*governanceexpectation.GovernanceExpectation, error)
}

// introspectionService defines the read-only operator visibility contract.
// If nil, the introspection endpoints return 501 Not Implemented.
type introspectionService interface {
	GetSurface(ctx context.Context, id string) (*surface.DecisionSurface, error)
	ListSurfaceVersions(ctx context.Context, id string) ([]*surface.DecisionSurface, error)
	GetSurfaceImpact(ctx context.Context, id string) (*SurfaceImpactResult, error)
	ListProfilesBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error)
	GetProfile(ctx context.Context, id string) (*authority.AuthorityProfile, error)
	ListProfileVersions(ctx context.Context, id string) ([]*authority.AuthorityProfile, error)
	GetAgent(ctx context.Context, id string) (*agent.Agent, error)
	GetGrant(ctx context.Context, id string) (*authority.AuthorityGrant, error)
	ListGrantsByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error)
	ListGrantsByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error)
	// Recovery endpoints — read-only state analysis.
	GetSurfaceRecovery(ctx context.Context, id string) (*SurfaceRecoveryResult, error)
	GetProfileRecovery(ctx context.Context, id string) (*ProfileRecoveryResult, error)
}

// controlAuditService defines the read-only interface for the control-plane
// audit trail. If nil, the audit endpoint returns 501 Not Implemented.
type controlAuditService interface {
	ListAudit(ctx context.Context, f controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error)
}

// coverageReadService defines the read-only interface for governance
// coverage records. If nil, /v1/coverage returns 501 Not Implemented.
type coverageReadService interface {
	ListCoverage(ctx context.Context, f governancecoverage.CoverageFilter) ([]*governancecoverage.CoverageRecord, error)
}

// grantLifecycleService manages operational grant lifecycle: suspend, revoke, reinstate.
// If nil, the grant lifecycle endpoints return 501 Not Implemented.
type grantLifecycleService interface {
	SuspendGrant(ctx context.Context, grantID, suspendedBy, reason string) (*authority.AuthorityGrant, error)
	RevokeGrant(ctx context.Context, grantID, revokedBy, reason string) (*authority.AuthorityGrant, error)
	ReinstateGrant(ctx context.Context, grantID, reinstatedBy string) (*authority.AuthorityGrant, error)
}

// structuralService defines the read-only interface for structural entities
// (Capability, Process, BusinessService). If nil, the structural endpoints return 501.
type structuralService interface {
	GetCapability(ctx context.Context, id string) (*capability.Capability, error)
	ListCapabilities(ctx context.Context) ([]*capability.Capability, error)
	GetProcess(ctx context.Context, id string) (*process.Process, error)
	ListProcesses(ctx context.Context) ([]*process.Process, error)
	// ListSurfacesByProcess returns (nil, false, nil) when process not found,
	// or (surfs, true, nil) including empty slice when found.
	ListSurfacesByProcess(ctx context.Context, processID string) ([]*surface.DecisionSurface, bool, error)
	// GetBusinessService returns nil, nil when not found or BS reader not configured.
	GetBusinessService(ctx context.Context, id string) (*businessservice.BusinessService, error)
	// ListBusinessServices returns an empty slice when BS reader not configured.
	ListBusinessServices(ctx context.Context) ([]*businessservice.BusinessService, error)

	// ListRelationshipsForBusinessService partitions BSR rows for the given
	// service into outgoing (source) and incoming (target). Returns
	// found=false when the BS does not exist (→ 404 in the handler).
	// (Epic 1, PR 1)
	ListRelationshipsForBusinessService(ctx context.Context, businessServiceID string) (outgoing, incoming []*businessservice.BusinessServiceRelationship, found bool, err error)
	// HasBusinessServiceRelationships reports whether the BSR reader is
	// wired — used to distinguish 501 (not configured) from 200-with-empty
	// (configured but no rows). (Epic 1, PR 1)
	HasBusinessServiceRelationships() bool

	// AI System Registration read endpoints (Epic 1, PR 2). Three
	// repositories — system, version, binding — back the four (or five)
	// /v1/aisystems/* paths.
	HasAISystems() bool
	HasAISystemVersions() bool
	HasAISystemBindings() bool
	GetAISystem(ctx context.Context, id string) (*aisystem.AISystem, error)
	ListAISystems(ctx context.Context) ([]*aisystem.AISystem, error)
	GetAISystemVersion(ctx context.Context, aiSystemID string, version int) (*aisystem.AISystemVersion, bool, error)
	ListAISystemVersions(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemVersion, bool, error)
	ListAISystemBindings(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemBinding, bool, error)
}

// explicitModeValidator is the narrow interface required for explicit-mode
// structural validation in handleEvaluateWith. *ExplicitValidationService satisfies this.
type explicitModeValidator interface {
	GetProcess(ctx context.Context, id string) (*process.Process, error)
	FindLatestSurface(ctx context.Context, id string) (*surface.DecisionSurface, error)
}

// validationErr is an unexported sentinel type for request-level structural
// validation failures that map to HTTP 400. Returned by validateExplicitStructure.
type validationErr string

func (e validationErr) Error() string { return string(e) }

// oidcProvider is the narrow interface required by the OIDC browser-login handlers.
// *oidc.Service satisfies this interface; tests may inject a stub.
type oidcProvider interface {
	UsePKCE() bool
	AuthURL(state, pkceChallenge string) string
	Exchange(ctx context.Context, code, pkceVerifier string) (*oidc.Claims, error)
	BuildPrincipal(claims *oidc.Claims) (*identity.Principal, error)
}

type Server struct {
	mux                  *http.ServeMux
	orchestrator         orchestrator
	controlPlane         controlPlaneService
	approval             approvalService
	introspection        introspectionService
	controlAudit         controlAuditService
	grantLifecycle       grantLifecycleService
	structural           structuralService
	authenticator        auth.Authenticator
	authMode             config.AuthMode             // set via WithAuthMode; must be called at startup with cfg.Auth.Mode
	policyMode           string                      // e.g. "noop" — set via WithPolicyMeta at boot
	policyEvaluatorName  string                      // human-readable evaluator name for health responses
	readyFn              func(context.Context) error // nil means always ready (memory mode)
	explorerEnabled      bool                        // set via WithExplorerEnabled; registers /explorer routes when true
	storeBackend         string                      // e.g. "memory" or "postgres" — set via WithStoreBackend at boot
	explorerDemoSeeded   *bool                       // nil = unknown, &true = seeded, &false = not seeded
	seedDemoUser         bool                        // set via WithSeedDemoUser; mirrors cfg.Dev.SeedDemoUser
	explorerOrchestrator orchestrator                // isolated in-memory orchestrator for POST /explorer
	explorerAudit        audit.AuditEventRepository  // Explorer-isolated audit repo, disjoint from production audit; backs explorerCoverageRead (Issue #56)
	explorerCoverageRead coverageReadService         // Explorer-isolated coverage read service for GET /explorer/coverage; backed by explorerAudit (Issue #56)
	localIAM             *localiam.Service           // nil when local IAM is disabled
	oidcService          oidcProvider                // nil when OIDC is disabled
	secureCookiesFlag    bool                        // mirrors LocalIAM.SecureCookies; used by OIDC helper cookies
	structuralMode       config.StructuralMode       // set via WithStructuralMode; empty/unset treated as permissive
	explicitValidator    explicitModeValidator       // nil when explicit-mode validation is not wired
	adminAudit           adminaudit.Repository       // nil when admin-audit is not wired (Issue #41)
	coverageRead         coverageReadService         // nil when coverage read service is not wired (Issue #56)
	governanceMap        governanceMapReadService    // nil when governance map read service is not wired (Epic 1, PR 4)
}

type approveSurfaceRequest struct {
	SubmittedBy  string `json:"submitted_by"`
	ApproverID   string `json:"approver_id"`
	ApproverName string `json:"approver_name,omitempty"`
}

type approveSurfaceResponse struct {
	SurfaceID  string `json:"surface_id"`
	Status     string `json:"status"`
	ApprovedBy string `json:"approved_by"`
}

type deprecateSurfaceRequest struct {
	DeprecatedBy string `json:"deprecated_by"`
	Reason       string `json:"reason"`
	SuccessorID  string `json:"successor_id,omitempty"`
}

type deprecateSurfaceResponse struct {
	SurfaceID          string `json:"surface_id"`
	Status             string `json:"status"`
	DeprecationReason  string `json:"deprecation_reason,omitempty"`
	SuccessorSurfaceID string `json:"successor_surface_id,omitempty"`
}

type approveProfileRequest struct {
	Version    int    `json:"version"`
	ApprovedBy string `json:"approved_by"`
}

type approveProfileResponse struct {
	ProfileID  string `json:"profile_id"`
	Version    int    `json:"version"`
	Status     string `json:"status"`
	ApprovedBy string `json:"approved_by"`
}

type deprecateProfileRequest struct {
	Version      int    `json:"version"`
	DeprecatedBy string `json:"deprecated_by"`
}

type deprecateProfileResponse struct {
	ProfileID string `json:"profile_id"`
	Version   int    `json:"version"`
	Status    string `json:"status"`
}

// approveExpectationRequest mirrors approveProfileRequest's shape.
// version is required; approved_by is body-supplied with
// actorFromContext fallback at the handler.
type approveExpectationRequest struct {
	Version    int    `json:"version"`
	ApprovedBy string `json:"approved_by"`
}

// approveExpectationResponse mirrors approveProfileResponse: id, version,
// status, approved_by. approved_at is intentionally omitted to match
// Profile's wire shape — callers can re-fetch the resource if they need
// the timestamp; it's persisted on the row.
type approveExpectationResponse struct {
	ExpectationID string `json:"expectation_id"`
	Version       int    `json:"version"`
	Status        string `json:"status"`
	ApprovedBy    string `json:"approved_by"`
}

// surfaceResponse is the wire format for GET /v1/surfaces/{id}.
type surfaceResponse struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	Version            int        `json:"version"`
	EffectiveFrom      time.Time  `json:"effective_from"`
	ApprovedAt         *time.Time `json:"approved_at,omitempty"`
	ApprovedBy         string     `json:"approved_by,omitempty"`
	SuccessorSurfaceID string     `json:"successor_surface_id,omitempty"`
	DeprecationReason  string     `json:"deprecation_reason,omitempty"`
	Domain             string     `json:"domain"`
	BusinessOwner      string     `json:"business_owner"`
	TechnicalOwner     string     `json:"technical_owner"`
}

// surfaceVersionResponse is one item in the GET /v1/surfaces/{id}/versions list.
type surfaceVersionResponse struct {
	Version       int        `json:"version"`
	Status        string     `json:"status"`
	EffectiveFrom time.Time  `json:"effective_from"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty"`
	ApprovedBy    string     `json:"approved_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// profileResponse is one item in the GET /v1/profiles?surface_id={id} list.
type profileResponse struct {
	ID                  string     `json:"id"`
	Version             int        `json:"version"`
	SurfaceID           string     `json:"surface_id"`
	Name                string     `json:"name"`
	Description         string     `json:"description,omitempty"`
	Status              string     `json:"status"`
	EffectiveDate       time.Time  `json:"effective_date"`
	ConfidenceThreshold float64    `json:"confidence_threshold"`
	EscalationMode      string     `json:"escalation_mode"`
	FailMode            string     `json:"fail_mode"`
	PolicyReference     string     `json:"policy_reference,omitempty"`
	RequiredContextKeys []string   `json:"required_context_keys,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ApprovedBy          string     `json:"approved_by,omitempty"`
	ApprovedAt          *time.Time `json:"approved_at,omitempty"`
}

// capabilityResponse is the wire format for GET /v1/capabilities/{id} and list items.
type capabilityResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Owner       string    `json:"owner,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// processResponse is the wire format for GET /v1/processes/{id} and list items.
type processResponse struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	BusinessServiceID string    `json:"business_service_id"`
	Description       string    `json:"description,omitempty"`
	Status            string    `json:"status"`
	Owner             string    `json:"owner,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// businessServiceResponse is the wire format for GET /v1/businessservices/{id} and list items.
type businessServiceResponse struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Description     string               `json:"description,omitempty"`
	ServiceType     string               `json:"service_type"`
	RegulatoryScope string               `json:"regulatory_scope,omitempty"`
	Status          string               `json:"status"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
	ExternalRef     *externalRefResponse `json:"external_ref"`
}

// businessServiceRelationshipResponse is one item in the
// GET /v1/businessservices/{id}/relationships response (Epic 1, PR 1).
type businessServiceRelationshipResponse struct {
	ID                      string               `json:"id"`
	SourceBusinessServiceID string               `json:"source_business_service_id"`
	TargetBusinessServiceID string               `json:"target_business_service_id"`
	RelationshipType        string               `json:"relationship_type"`
	Description             string               `json:"description,omitempty"`
	CreatedAt               time.Time            `json:"created_at"`
	CreatedBy               string               `json:"created_by,omitempty"`
	ExternalRef             *externalRefResponse `json:"external_ref"`
}

// businessServiceRelationshipsResponse is the top-level wire format for the
// /v1/businessservices/{id}/relationships endpoint. Outgoing and Incoming
// arrays are always present (never null) so callers can iterate without
// nil checks.
type businessServiceRelationshipsResponse struct {
	BusinessServiceID string                                `json:"business_service_id"`
	Outgoing          []businessServiceRelationshipResponse `json:"outgoing"`
	Incoming          []businessServiceRelationshipResponse `json:"incoming"`
}

// externalRefResponse is the wire format for the optional ExternalRef
// field that five entity responses gained in Epic 1, PR 3.
//
// Posture: parent responses include `external_ref` as a *pointer* —
// rendering as JSON null when no external reference is recorded. Mirrors
// PR 2's nullable binding context fields.
//
// SourceURL, SourceVersion, and LastSyncedAt use `omitempty` because
// each is independently optional; SourceSystem and SourceID are required
// once the field is non-nil (enforced by the consistency CHECK upstream).
// LastSyncedAt is rendered as RFC3339 in UTC. A *string pointer keeps
// "no timestamp" distinguishable from "the zero time" on the wire.
type externalRefResponse struct {
	SourceSystem  string  `json:"source_system"`
	SourceID      string  `json:"source_id"`
	SourceURL     string  `json:"source_url,omitempty"`
	SourceVersion string  `json:"source_version,omitempty"`
	LastSyncedAt  *string `json:"last_synced_at,omitempty"`
}

// ---------------------------------------------------------------------------
// AI System Registration wire formats (Epic 1, PR 2)
// ---------------------------------------------------------------------------

// aiSystemResponse is the wire format for GET /v1/aisystems/{id} and list items.
type aiSystemResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Owner       string               `json:"owner,omitempty"`
	Vendor      string               `json:"vendor,omitempty"`
	SystemType  string               `json:"system_type,omitempty"`
	Status      string               `json:"status"`
	Origin      string               `json:"origin"`
	Managed     bool                 `json:"managed"`
	Replaces    *string              `json:"replaces"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	CreatedBy   string               `json:"created_by,omitempty"`
	ExternalRef *externalRefResponse `json:"external_ref"`
}

// aiSystemsResponse wraps the list endpoint payload. The `ai_systems`
// envelope keeps the response self-describing and matches the brief.
type aiSystemsResponse struct {
	AISystems []aiSystemResponse `json:"ai_systems"`
}

// aiSystemVersionResponse is one item in the versions list.
// EffectiveUntil and RetiredAt are pointer-typed so JSON null marks
// "still effective" / "not retired" cleanly.
type aiSystemVersionResponse struct {
	AISystemID           string               `json:"ai_system_id"`
	Version              int                  `json:"version"`
	ReleaseLabel         string               `json:"release_label,omitempty"`
	ModelArtifact        string               `json:"model_artifact,omitempty"`
	ModelHash            string               `json:"model_hash,omitempty"`
	Endpoint             string               `json:"endpoint,omitempty"`
	Status               string               `json:"status"`
	EffectiveFrom        time.Time            `json:"effective_from"`
	EffectiveUntil       *time.Time           `json:"effective_until"`
	RetiredAt            *time.Time           `json:"retired_at"`
	ComplianceFrameworks []string             `json:"compliance_frameworks"`
	DocumentationURL     string               `json:"documentation_url,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
	CreatedBy            string               `json:"created_by,omitempty"`
	ExternalRef          *externalRefResponse `json:"external_ref"`
}

// aiSystemVersionsResponse is the top-level wire format for the
// versions list endpoint. The `versions` array is always present
// (never null).
type aiSystemVersionsResponse struct {
	AISystemID string                    `json:"ai_system_id"`
	Versions   []aiSystemVersionResponse `json:"versions"`
}

// aiSystemBindingResponse is one item in the bindings list. SurfaceID,
// CapabilityID, ProcessID and BusinessServiceID are pointer-typed so
// JSON null distinguishes "field unset" from "empty string".
// AISystemVersion is an int pointer for the same reason.
type aiSystemBindingResponse struct {
	ID                string               `json:"id"`
	AISystemID        string               `json:"ai_system_id"`
	AISystemVersion   *int                 `json:"ai_system_version"`
	BusinessServiceID *string              `json:"business_service_id"`
	CapabilityID      *string              `json:"capability_id"`
	ProcessID         *string              `json:"process_id"`
	SurfaceID         *string              `json:"surface_id"`
	Role              string               `json:"role,omitempty"`
	Description       string               `json:"description,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
	CreatedBy         string               `json:"created_by,omitempty"`
	ExternalRef       *externalRefResponse `json:"external_ref"`
}

// aiSystemBindingsResponse is the top-level wire format for the
// bindings list endpoint.
type aiSystemBindingsResponse struct {
	AISystemID string                    `json:"ai_system_id"`
	Bindings   []aiSystemBindingResponse `json:"bindings"`
}

// agentResponse is the wire format for GET /v1/agents/{id}.
type agentResponse struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	Owner            string    `json:"owner"`
	ModelVersion     string    `json:"model_version,omitempty"`
	Endpoint         string    `json:"endpoint,omitempty"`
	OperationalState string    `json:"operational_state"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// grantResponse is one item in the GET /v1/grants list.
type grantResponse struct {
	ID            string     `json:"id"`
	AgentID       string     `json:"agent_id"`
	ProfileID     string     `json:"profile_id"`
	Status        string     `json:"status"`
	GrantedBy     string     `json:"granted_by"`
	EffectiveDate time.Time  `json:"effective_from"`
	ExpiresAt     *time.Time `json:"effective_until,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Impact Analysis wire format — GET /v1/surfaces/{id}/impact
// ---------------------------------------------------------------------------

// surfaceImpactResponse is the top-level wire format for the impact endpoint.
// Sections are always present, even when empty, so callers never need to
// null-check. Warnings is an empty array (not null) when no warnings apply.
type surfaceImpactResponse struct {
	Surface  surfaceImpactSurfaceEntry `json:"surface"`
	Profiles []impactProfileEntry      `json:"profiles"`
	Grants   []impactGrantEntry        `json:"grants"`
	Agents   []impactAgentEntry        `json:"agents"`
	Summary  impactSummaryResponse     `json:"summary"`
	Warnings []string                  `json:"warnings"`
}

// surfaceImpactSurfaceEntry is the surface metadata section of the impact response.
type surfaceImpactSurfaceEntry struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	Version            int        `json:"version"`
	Domain             string     `json:"domain"`
	BusinessOwner      string     `json:"business_owner"`
	TechnicalOwner     string     `json:"technical_owner"`
	ApprovedBy         string     `json:"approved_by,omitempty"`
	ApprovedAt         *time.Time `json:"approved_at,omitempty"`
	DeprecationReason  string     `json:"deprecation_reason,omitempty"`
	SuccessorSurfaceID string     `json:"successor_surface_id,omitempty"`
}

// impactProfileEntry is one profile in the impact profiles section.
type impactProfileEntry struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	Version         int    `json:"version"`
	PolicyReference string `json:"policy_reference,omitempty"`
	FailMode        string `json:"fail_mode"`
	EscalationMode  string `json:"escalation_mode"`
}

// impactGrantEntry is one grant in the impact grants section.
type impactGrantEntry struct {
	ID             string     `json:"id"`
	AgentID        string     `json:"agent_id"`
	ProfileID      string     `json:"profile_id"`
	Status         string     `json:"status"`
	GrantedBy      string     `json:"granted_by"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveUntil *time.Time `json:"effective_until,omitempty"`
}

// impactAgentEntry is one agent in the impact agents section (deduplicated across all grants).
type impactAgentEntry struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Owner            string `json:"owner"`
	OperationalState string `json:"operational_state"`
}

// impactSummaryResponse is the aggregate count block in the impact response.
type impactSummaryResponse struct {
	ProfileCount       int `json:"profile_count"`
	GrantCount         int `json:"grant_count"`
	AgentCount         int `json:"agent_count"`
	ActiveProfileCount int `json:"active_profile_count"`
	ActiveGrantCount   int `json:"active_grant_count"`
	ActiveAgentCount   int `json:"active_agent_count"`
}

func NewServer(orchestrator orchestrator) *Server {
	return NewServerWithControlPlane(orchestrator, nil)
}

func (s *Server) handleSurfaceActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	const prefix = "/v1/controlplane/surfaces/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	surfaceID := strings.TrimSpace(parts[0])
	if surfaceID == "" || !isValidIdentifier(surfaceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid surface id"})
		return
	}

	action := parts[1]
	switch action {
	case "approve":
		// surface:approve — granted to governance.approver and platform.admin
		// bundles. Preserves the existing maker–checker split.
		s.requirePermission(authz.PermSurfaceApprove)(func(w http.ResponseWriter, r *http.Request) {
			s.handleApproveSurface(w, r, surfaceID)
		})(w, r)
	case "deprecate":
		// surface:deprecate — admin-only today; the bundle composition keeps
		// it out of the approver role to preserve the lifecycle boundary.
		s.requirePermission(authz.PermSurfaceDeprecate)(func(w http.ResponseWriter, r *http.Request) {
			s.handleDeprecateSurface(w, r, surfaceID)
		})(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleApproveSurface processes POST /v1/controlplane/surfaces/{id}/approve.
// It transitions a surface from review to active status, enforcing the
// maker-checker approval policy.
func (s *Server) handleApproveSurface(w http.ResponseWriter, r *http.Request, surfaceID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "approval service not configured",
		})
		return
	}

	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req approveSurfaceRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.SubmittedBy = strings.TrimSpace(req.SubmittedBy)
	req.ApproverID = strings.TrimSpace(req.ApproverID)
	req.ApproverName = strings.TrimSpace(req.ApproverName)

	// Resolve the approver identity: use authenticated principal when available,
	// fall back to the body-supplied approver_id for unauthenticated deployments.
	approverID := actorFromContext(r.Context(), req.ApproverID)
	if !isValidIdentifier(approverID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "approver_id must be a valid identifier",
		})
		return
	}

	if req.SubmittedBy != "" && !isValidIdentifier(req.SubmittedBy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "submitted_by must be a valid identifier",
		})
		return
	}

	submitter := identity.Principal{
		ID: req.SubmittedBy,
	}
	// Build the approver principal. When an authenticated principal is available its
	// roles are preserved so that platform.admin callers bypass the owner-match check.
	// In unauthenticated (open) deployments the body-supplied ID is used and the
	// caller is treated as a governance.approver (subject to owner-match rules).
	approver := identity.Principal{
		ID:    approverID,
		Name:  req.ApproverName,
		Roles: []string{identity.RoleGovernanceApprover},
	}
	if p := PrincipalFromContext(r.Context()); p != nil {
		approver.Roles = p.Roles
	}

	updated, err := s.approval.ApproveSurface(r.Context(), surfaceID, submitter, approver)
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, approveSurfaceResponse{
		SurfaceID:  updated.ID,
		Status:     string(updated.Status),
		ApprovedBy: updated.ApprovedBy,
	})
}

// handleDeprecateSurface processes POST /v1/controlplane/surfaces/{id}/deprecate.
// It transitions a surface from active to deprecated status. Deprecated surfaces
// remain operational for existing grants but signal that migration to a successor
// surface is expected.
func (s *Server) handleDeprecateSurface(w http.ResponseWriter, r *http.Request, surfaceID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "approval service not configured",
		})
		return
	}

	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req deprecateSurfaceRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.DeprecatedBy = strings.TrimSpace(req.DeprecatedBy)
	req.Reason = strings.TrimSpace(req.Reason)
	req.SuccessorID = strings.TrimSpace(req.SuccessorID)

	deprecatedBy := actorFromContext(r.Context(), req.DeprecatedBy)
	if !isValidIdentifier(deprecatedBy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "deprecated_by must be a valid identifier",
		})
		return
	}

	if req.Reason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "reason is required",
		})
		return
	}

	if req.SuccessorID != "" && !isValidIdentifier(req.SuccessorID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "successor_id must be a valid identifier",
		})
		return
	}

	updated, err := s.approval.DeprecateSurface(r.Context(), surfaceID, deprecatedBy, req.Reason, req.SuccessorID)
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, deprecateSurfaceResponse{
		SurfaceID:          updated.ID,
		Status:             string(updated.Status),
		DeprecationReason:  updated.DeprecationReason,
		SuccessorSurfaceID: updated.SuccessorSurfaceID,
	})
}

// handleProfileActions dispatches POST /v1/controlplane/profiles/{id}/{action}.
func (s *Server) handleProfileActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	const prefix = "/v1/controlplane/profiles/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	profileID := strings.TrimSpace(parts[0])
	if profileID == "" || !isValidIdentifier(profileID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid profile id"})
		return
	}

	action := parts[1]
	switch action {
	case "approve":
		// profile:approve — granted to governance.approver and platform.admin.
		s.requirePermission(authz.PermProfileApprove)(func(w http.ResponseWriter, r *http.Request) {
			s.handleApproveProfile(w, r, profileID)
		})(w, r)
	case "deprecate":
		// profile:deprecate — admin-only today; deliberate lifecycle boundary.
		s.requirePermission(authz.PermProfileDeprecate)(func(w http.ResponseWriter, r *http.Request) {
			s.handleDeprecateProfile(w, r, profileID)
		})(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleApproveProfile processes POST /v1/controlplane/profiles/{id}/approve.
// It transitions a profile version from review to active status.
func (s *Server) handleApproveProfile(w http.ResponseWriter, r *http.Request, profileID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "approval service not configured"})
		return
	}

	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req approveProfileRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.ApprovedBy = strings.TrimSpace(req.ApprovedBy)
	approvedBy := actorFromContext(r.Context(), req.ApprovedBy)
	if !isValidIdentifier(approvedBy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approved_by must be a valid identifier"})
		return
	}
	if req.Version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be >= 1"})
		return
	}

	updated, err := s.approval.ApproveProfile(r.Context(), profileID, req.Version, approvedBy)
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, approveProfileResponse{
		ProfileID:  updated.ID,
		Version:    updated.Version,
		Status:     string(updated.Status),
		ApprovedBy: updated.ApprovedBy,
	})
}

// handleDeprecateProfile processes POST /v1/controlplane/profiles/{id}/deprecate.
// It transitions a profile version from active to deprecated status.
func (s *Server) handleDeprecateProfile(w http.ResponseWriter, r *http.Request, profileID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "approval service not configured"})
		return
	}

	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req deprecateProfileRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.DeprecatedBy = strings.TrimSpace(req.DeprecatedBy)
	deprecatedBy := actorFromContext(r.Context(), req.DeprecatedBy)
	if !isValidIdentifier(deprecatedBy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deprecated_by must be a valid identifier"})
		return
	}
	if req.Version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be >= 1"})
		return
	}

	updated, err := s.approval.DeprecateProfile(r.Context(), profileID, req.Version, deprecatedBy)
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, deprecateProfileResponse{
		ProfileID: updated.ID,
		Version:   updated.Version,
		Status:    string(updated.Status),
	})
}

// ---------------------------------------------------------------------------
// GovernanceExpectation Lifecycle Actions (#57)
// ---------------------------------------------------------------------------
//
// Mirrors handleProfileActions: dispatches POST
// /v1/controlplane/expectations/{id}/{action}. Today only the "approve"
// action is shipped; "deprecate" routes through the default 404 branch
// until a future issue adds it. The dispatcher shape is forward-
// compatible — adding a new action means adding one case here.

// handleExpectationActions dispatches POST /v1/controlplane/expectations/{id}/{action}.
func (s *Server) handleExpectationActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	const prefix = "/v1/controlplane/expectations/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	expectationID := strings.TrimSpace(parts[0])
	if expectationID == "" || !isValidIdentifier(expectationID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expectation id"})
		return
	}

	action := parts[1]
	switch action {
	case "approve":
		// governanceexpectation:approve — granted to governance.approver and
		// platform.admin bundles. Mirrors profile:approve / surface:approve.
		s.requirePermission(authz.PermGovernanceExpectationApprove)(func(w http.ResponseWriter, r *http.Request) {
			s.handleApproveGovernanceExpectation(w, r, expectationID)
		})(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleApproveGovernanceExpectation processes
// POST /v1/controlplane/expectations/{id}/approve. It transitions a
// GovernanceExpectation version from review to active status. Mirrors
// handleApproveProfile in shape: strict JSON decode, version >= 1,
// approved_by from authenticated principal with body fallback.
func (s *Server) handleApproveGovernanceExpectation(w http.ResponseWriter, r *http.Request, expectationID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "approval service not configured"})
		return
	}

	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req approveExpectationRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.ApprovedBy = strings.TrimSpace(req.ApprovedBy)
	approvedBy := actorFromContext(r.Context(), req.ApprovedBy)
	if !isValidIdentifier(approvedBy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approved_by must be a valid identifier"})
		return
	}
	if req.Version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be >= 1"})
		return
	}

	updated, err := s.approval.ApproveGovernanceExpectation(r.Context(), expectationID, req.Version, approvedBy)
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, approveExpectationResponse{
		ExpectationID: updated.ID,
		Version:       updated.Version,
		Status:        string(updated.Status),
		ApprovedBy:    updated.ApprovedBy,
	})
}

// ---------------------------------------------------------------------------
// Grant Lifecycle Actions
// ---------------------------------------------------------------------------

type suspendGrantRequest struct {
	SuspendedBy string `json:"suspended_by"`
	Reason      string `json:"reason,omitempty"`
}

type revokeGrantRequest struct {
	RevokedBy string `json:"revoked_by"`
	Reason    string `json:"reason,omitempty"`
}

type reinstateGrantRequest struct {
	ReinstatedBy string `json:"reinstated_by"`
}

type grantLifecycleResponse struct {
	GrantID string `json:"grant_id"`
	Status  string `json:"status"`
	AgentID string `json:"agent_id"`
}

// handleGrantActions dispatches POST /v1/controlplane/grants/{id}/{action}.
func (s *Server) handleGrantActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	const prefix = "/v1/controlplane/grants/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	grantID := strings.TrimSpace(parts[0])
	action := parts[1]

	if grantID == "" || !isValidIdentifier(grantID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid grant id"})
		return
	}

	if s.grantLifecycle == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "grant lifecycle service not configured",
		})
		return
	}

	// Grant lifecycle actions are independently permissioned so they can be
	// granted separately in future role compositions. Today platform.admin's
	// bundle holds all three; no other default role holds any.
	switch action {
	case "suspend":
		s.requirePermission(authz.PermGrantSuspend)(func(w http.ResponseWriter, r *http.Request) {
			s.handleSuspendGrant(w, r, grantID)
		})(w, r)
	case "revoke":
		s.requirePermission(authz.PermGrantRevoke)(func(w http.ResponseWriter, r *http.Request) {
			s.handleRevokeGrant(w, r, grantID)
		})(w, r)
	case "reinstate":
		s.requirePermission(authz.PermGrantReinstate)(func(w http.ResponseWriter, r *http.Request) {
			s.handleReinstateGrant(w, r, grantID)
		})(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) handleSuspendGrant(w http.ResponseWriter, r *http.Request, grantID string) {
	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req suspendGrantRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.SuspendedBy = strings.TrimSpace(req.SuspendedBy)
	suspendedBy := actorFromContext(r.Context(), req.SuspendedBy)
	if !isValidIdentifier(suspendedBy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "suspended_by must be a valid identifier"})
		return
	}

	updated, err := s.grantLifecycle.SuspendGrant(r.Context(), grantID, suspendedBy, req.Reason)
	if err != nil {
		code, resp := mapGrantError(err)
		writeJSON(w, code, resp)
		return
	}

	writeJSON(w, http.StatusOK, grantLifecycleResponse{
		GrantID: updated.ID,
		Status:  string(updated.Status),
		AgentID: updated.AgentID,
	})
}

func (s *Server) handleRevokeGrant(w http.ResponseWriter, r *http.Request, grantID string) {
	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req revokeGrantRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.RevokedBy = strings.TrimSpace(req.RevokedBy)
	revokedBy := actorFromContext(r.Context(), req.RevokedBy)
	if !isValidIdentifier(revokedBy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "revoked_by must be a valid identifier"})
		return
	}

	updated, err := s.grantLifecycle.RevokeGrant(r.Context(), grantID, revokedBy, req.Reason)
	if err != nil {
		code, resp := mapGrantError(err)
		writeJSON(w, code, resp)
		return
	}

	writeJSON(w, http.StatusOK, grantLifecycleResponse{
		GrantID: updated.ID,
		Status:  string(updated.Status),
		AgentID: updated.AgentID,
	})
}

func (s *Server) handleReinstateGrant(w http.ResponseWriter, r *http.Request, grantID string) {
	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req reinstateGrantRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.ReinstatedBy = strings.TrimSpace(req.ReinstatedBy)
	reinstatedBy := actorFromContext(r.Context(), req.ReinstatedBy)
	if !isValidIdentifier(reinstatedBy) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reinstated_by must be a valid identifier"})
		return
	}

	updated, err := s.grantLifecycle.ReinstateGrant(r.Context(), grantID, reinstatedBy)
	if err != nil {
		code, resp := mapGrantError(err)
		writeJSON(w, code, resp)
		return
	}

	writeJSON(w, http.StatusOK, grantLifecycleResponse{
		GrantID: updated.ID,
		Status:  string(updated.Status),
		AgentID: updated.AgentID,
	})
}

func mapGrantError(err error) (int, map[string]string) {
	switch {
	case errors.Is(err, approval.ErrGrantNotFound):
		return http.StatusNotFound, map[string]string{"error": "grant not found"}
	case errors.Is(err, approval.ErrGrantNotActive):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	case errors.Is(err, approval.ErrGrantNotSuspended):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	case errors.Is(err, approval.ErrGrantRevoked):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	case errors.Is(err, approval.ErrInvalidGrantTransition):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	default:
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
}

func NewServerWithServices(orchestrator orchestrator, controlPlane controlPlaneService, approvalSvc approvalService) *Server {
	return NewServerWithAllServices(orchestrator, controlPlane, approvalSvc, nil)
}

// NewServerWithAllServices constructs a Server with all optional services wired in.
// Any service may be nil; its endpoints will return 501 Not Implemented.
func NewServerWithAllServices(
	orch orchestrator,
	controlPlane controlPlaneService,
	approvalSvc approvalService,
	introspectionSvc introspectionService,
) *Server {
	return NewServerFull(orch, controlPlane, approvalSvc, introspectionSvc, nil, nil)
}

// NewServerFull constructs a Server with all services including the control-plane
// audit and grant lifecycle services. Any service may be nil; its endpoints
// will return 501 Not Implemented.
func NewServerFull(
	orch orchestrator,
	controlPlane controlPlaneService,
	approvalSvc approvalService,
	introspectionSvc introspectionService,
	controlAuditSvc controlAuditService,
	grantSvc grantLifecycleService,
) *Server {
	mux := http.NewServeMux()

	s := &Server{
		mux:            mux,
		orchestrator:   orch,
		controlPlane:   controlPlane,
		approval:       approvalSvc,
		introspection:  introspectionSvc,
		controlAudit:   controlAuditSvc,
		grantLifecycle: grantSvc,
		authMode:       config.AuthModeOpen, // default matches config default; override with WithAuthMode
	}
	s.routes()

	return s
}

// WithStructural attaches a structural service to the server, enabling
// the /v1/capabilities and /v1/processes endpoints.
func (s *Server) WithStructural(svc structuralService) *Server {
	s.structural = svc
	return s
}

// WithStructuralMode sets the structural enforcement mode. In permissive mode
// (the default when not called), process_id is optional on /v1/evaluate.
// In enforced mode, process_id is required.
func (s *Server) WithStructuralMode(mode config.StructuralMode) *Server {
	s.structuralMode = mode
	return s
}

// WithExplicitValidator attaches a structural validation service used to
// verify explicit process_id requests on the governed /v1/evaluate path.
func (s *Server) WithExplicitValidator(svc explicitModeValidator) *Server {
	s.explicitValidator = svc
	return s
}

// WithAdminAudit attaches the platform-administrative audit repository used
// to record apply invocations, password changes, and bootstrap admin
// creation. When nil (or not called) the admin audit is a no-op — this is
// the safe default for tests and dev deployments. See Issue #41.
func (s *Server) WithAdminAudit(repo adminaudit.Repository) *Server {
	s.adminAudit = repo
	return s
}

// WithCoverageReadService attaches the governance coverage read service
// that powers GET /v1/coverage. When nil (or not called) the endpoint
// returns 501 Not Implemented. See Issue #56.
func (s *Server) WithCoverageReadService(svc coverageReadService) *Server {
	s.coverageRead = svc
	return s
}

// appendAdminAudit persists an administrative audit record. It is a no-op
// when the repository is not configured. Append errors are logged but never
// fail the calling action — the audit trail's value here is investigability,
// not gate semantics.
func (s *Server) appendAdminAudit(ctx context.Context, rec *adminaudit.AdminAuditRecord) {
	if s.adminAudit == nil || rec == nil {
		return
	}
	if err := s.adminAudit.Append(ctx, rec); err != nil {
		slog.Warn("admin_audit_append_failed",
			"action", string(rec.Action),
			"outcome", string(rec.Outcome),
			"actor_id", rec.ActorID,
			"error", err,
		)
	}
}

// clientIPFromRequest returns a best-effort client IP for admin-audit
// records. It prefers the first entry in X-Forwarded-For when present
// (operator-configured proxies set this), falling back to r.RemoteAddr.
// Never empty on real requests; only empty if both sources are absent.
func clientIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return xff
	}
	return r.RemoteAddr
}

func NewServerWithControlPlane(orchestrator orchestrator, controlPlane controlPlaneService) *Server {
	mux := http.NewServeMux()

	s := &Server{
		mux:          mux,
		orchestrator: orchestrator,
		controlPlane: controlPlane,
		authMode:     config.AuthModeOpen, // default matches config default; override with WithAuthMode
	}
	s.routes()

	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReady)
	// Evaluation — platform.operator or platform.admin role required.
	s.mux.HandleFunc("/v1/evaluate", s.requireAuth(s.requireRole(identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleEvaluate)))

	// Escalation review — platform.admin or governance.reviewer.
	s.mux.HandleFunc("/v1/reviews", s.requireAuth(s.requireRole(identity.RolePlatformAdmin, identity.RoleGovernanceReviewer)(s.handleCreateReview)))

	// Runtime read — platform.viewer or above.
	s.mux.HandleFunc("/v1/envelopes/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetEnvelope)))
	s.mux.HandleFunc("/v1/envelopes", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListEnvelopes)))
	s.mux.HandleFunc("/v1/escalations", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListEscalations)))
	s.mux.HandleFunc("/v1/decisions/request/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetDecisionByRequestID)))

	// Control plane — governed endpoints. Write paths use scoped permissions
	// (internal/authz). On /v1/controlplane/apply the middleware gate is the
	// coarse "may invoke apply" permission; fine-grained per-document Kind
	// checks are enforced inside the apply planner. See docs/authorization.md.
	s.mux.HandleFunc("/v1/controlplane/apply", s.requireAuth(s.requirePermission(authz.PermControlplaneApply)(s.handleApplyBundle)))
	s.mux.HandleFunc("/v1/controlplane/plan", s.requireAuth(s.requirePermission(authz.PermControlplanePlan)(s.handlePlanBundle)))
	// audit read: platform.viewer or above.
	s.mux.HandleFunc("/v1/controlplane/audit", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListControlAudit)))
	// Governance coverage read (Issue #56): platform.viewer or above —
	// mirrors the controlplane audit read scope; both are read-only
	// observability of platform-internal events.
	s.mux.HandleFunc("/v1/coverage", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListCoverage)))
	// Platform admin-audit read (Issue #41): admin-only because the trail
	// includes password-change and bootstrap events, which are more
	// sensitive than the resource-oriented control-plane audit.
	s.mux.HandleFunc("/v1/platform/admin-audit", s.requireAuth(s.requireRole(identity.RolePlatformAdmin)(s.handleListAdminAudit)))
	// Resource lifecycle — role enforcement applied per-action inside each dispatcher.
	s.mux.HandleFunc("/v1/controlplane/surfaces/", s.requireAuth(s.handleSurfaceActions))
	s.mux.HandleFunc("/v1/controlplane/profiles/", s.requireAuth(s.handleProfileActions))
	s.mux.HandleFunc("/v1/controlplane/expectations/", s.requireAuth(s.handleExpectationActions))
	s.mux.HandleFunc("/v1/controlplane/grants/", s.requireAuth(s.handleGrantActions))

	// Operator introspection — platform.viewer or above.
	s.mux.HandleFunc("/v1/surfaces/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetSurfaceOrVersions)))
	s.mux.HandleFunc("/v1/profiles/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetProfileOrVersions)))
	s.mux.HandleFunc("/v1/profiles", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListProfiles)))
	s.mux.HandleFunc("/v1/agents/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetAgent)))
	s.mux.HandleFunc("/v1/grants/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetGrant)))
	s.mux.HandleFunc("/v1/grants", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListGrants)))

	// Structural entities — platform.viewer or above.
	s.mux.HandleFunc("/v1/capabilities/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetCapability)))
	s.mux.HandleFunc("/v1/capabilities", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListCapabilities)))
	s.mux.HandleFunc("/v1/processes/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetProcessOrSurfaces)))
	s.mux.HandleFunc("/v1/processes", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListProcesses)))
	s.mux.HandleFunc("/v1/businessservices/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetBusinessService)))
	s.mux.HandleFunc("/v1/businessservices", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListBusinessServices)))

	// AI System Registration read endpoints (Epic 1, PR 2). Same auth
	// posture as the rest of the structural surface: viewer or above.
	s.mux.HandleFunc("/v1/aisystems/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetAISystemOrSubpath)))
	s.mux.HandleFunc("/v1/aisystems", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListAISystems)))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}

// ---------------------------------------------------------------------------
// Health/Readiness
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	resp := map[string]string{
		"status":  "ok",
		"service": "midas",
	}
	if s.policyMode != "" {
		resp["policy_mode"] = s.policyMode
	}
	if s.policyEvaluatorName != "" {
		resp["policy_evaluator"] = s.policyEvaluatorName
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.readyFn != nil {
		if err := s.readyFn(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
				"reason": "database unreachable",
			})
			return
		}
	}

	resp := map[string]string{
		"status":  "ready",
		"service": "midas",
	}
	if s.policyMode != "" {
		resp["policy_mode"] = s.policyMode
	}
	if s.policyEvaluatorName != "" {
		resp["policy_evaluator"] = s.policyEvaluatorName
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Evaluate
// ---------------------------------------------------------------------------

type evaluateRequest struct {
	SurfaceID     string               `json:"surface_id"`
	AgentID       string               `json:"agent_id"`
	Confidence    float64              `json:"confidence"`
	Consequence   *evaluateConsequence `json:"consequence,omitempty"`
	Context       map[string]any       `json:"context,omitempty"`
	RequestSource string               `json:"request_source,omitempty"`
	RequestID     string               `json:"request_id,omitempty"`
	ProcessID     string               `json:"process_id,omitempty"`
}

type evaluateConsequence struct {
	Type       value.ConsequenceType `json:"type"`
	Amount     float64               `json:"amount,omitempty"`
	Currency   string                `json:"currency,omitempty"`
	RiskRating value.RiskRating      `json:"risk_rating,omitempty"`
}

type evaluateResponse struct {
	Outcome     string `json:"outcome"`
	Reason      string `json:"reason"`
	EnvelopeID  string `json:"envelope_id,omitempty"`
	Explanation string `json:"explanation,omitempty"`

	// Simulated is true when this result was produced by the non-persistent
	// simulate path. Omitted (false) on normal evaluate responses.
	Simulated bool `json:"simulated,omitempty"`

	// Policy transparency fields — informational only, never affect the outcome.
	PolicyMode      string `json:"policy_mode,omitempty"`
	PolicyReference string `json:"policy_reference,omitempty"`
	PolicySkipped   bool   `json:"policy_skipped,omitempty"`
}

// validateExplicitStructure checks that the provided processID and surfaceID are
// structurally consistent:
//  1. the process exists
//  2. the surface exists (latest version)
//  3. the surface belongs to the process
//
// Returns a validationErr (maps to HTTP 400) for structural failures.
// Returns a wrapped system error (maps to HTTP 500) for unexpected lookup failures.
// Returns nil when all checks pass.
func (s *Server) validateExplicitStructure(ctx context.Context, surfaceID, processID string) error {
	if s.explicitValidator == nil {
		return fmt.Errorf("explicit validation service not configured")
	}

	proc, err := s.explicitValidator.GetProcess(ctx, processID)
	if err != nil {
		return fmt.Errorf("process lookup failed: %w", err)
	}
	if proc == nil {
		return validationErr(fmt.Sprintf("process %q does not exist", processID))
	}

	surf, err := s.explicitValidator.FindLatestSurface(ctx, surfaceID)
	if err != nil {
		return fmt.Errorf("surface lookup failed: %w", err)
	}
	if surf == nil {
		return validationErr(fmt.Sprintf("surface %q does not exist", surfaceID))
	}

	if surf.ProcessID != processID {
		return validationErr(fmt.Sprintf(
			"surface %q belongs to process %q, not requested process %q",
			surfaceID, surf.ProcessID, processID,
		))
	}

	return nil
}

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}
	s.handleEvaluateWith(w, r, s.orchestrator, true)
}

// handleEvaluateWith contains the shared evaluation logic used by both
// /v1/evaluate (main orchestrator) and POST /explorer (Explorer orchestrator).
// Callers are responsible for method and nil checks before calling.
//
// requireRequestID controls whether an absent request_id is rejected with
// HTTP 400. Set true for the governed /v1/evaluate path; false for Explorer
// and other non-governed callers that tolerate auto-generated identifiers.
func (s *Server) handleEvaluateWith(w http.ResponseWriter, r *http.Request, orch orchestrator, requireRequestID bool) {
	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{
			"error": err.Error(),
		})
		return
	}

	var req evaluateRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	req.SurfaceID = strings.TrimSpace(req.SurfaceID)
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.RequestSource = strings.TrimSpace(req.RequestSource)
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.ProcessID = strings.TrimSpace(req.ProcessID)
	if req.Consequence != nil {
		req.Consequence.Currency = strings.TrimSpace(req.Consequence.Currency)
	}

	if req.SurfaceID == "" || req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "surface_id and agent_id are required",
		})
		return
	}

	if req.Confidence < 0 || req.Confidence > 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "confidence must be between 0 and 1",
		})
		return
	}

	if req.RequestSource == "" {
		req.RequestSource = defaultRequestSource
	}

	if req.RequestID == "" {
		if requireRequestID {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "request_id is required",
			})
			return
		}
		req.RequestID = uuid.NewString()
	} else if !isValidIdentifier(req.RequestID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request_id contains invalid characters or exceeds length limit",
		})
		return
	}

	// Explicit-mode structural validation: only on the governed path, only when process_id is provided.
	if requireRequestID && req.ProcessID != "" {
		if err := s.validateExplicitStructure(r.Context(), req.SurfaceID, req.ProcessID); err != nil {
			var ve validationErr
			if errors.As(err, &ve) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			} else {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "explicit validation failed"})
			}
			return
		}
	}

	// Enforced-mode guard: on the governed /v1/evaluate path, process_id is
	// required. Permissive mode (default) still allows omission for the
	// orchestrator's empty-process_id handling.
	if requireRequestID && req.ProcessID == "" && s.structuralMode == config.StructuralModeEnforced {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "process_id is required",
		})
		return
	}

	result, err := orch.Evaluate(r.Context(), toEvalRequest(req), json.RawMessage(rawBody))
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEvaluation)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, evaluateResponse{
		Outcome:         string(result.Outcome),
		Reason:          string(result.ReasonCode),
		EnvelopeID:      result.EnvelopeID,
		Explanation:     result.Explanation,
		PolicyMode:      result.PolicyMode,
		PolicyReference: result.PolicyReference,
		PolicySkipped:   result.PolicySkipped,
	})
}

// handleSimulateWith runs the same request parsing as handleEvaluateWith but
// calls orch.Simulate() instead of orch.Evaluate(). The response omits
// envelope_id and includes simulated:true. No envelope, audit events, or
// outbox messages are written. Callers must verify method and nil before calling.
func (s *Server) handleSimulateWith(w http.ResponseWriter, r *http.Request, orch orchestrator) {
	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	var req evaluateRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.SurfaceID = strings.TrimSpace(req.SurfaceID)
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.RequestSource = strings.TrimSpace(req.RequestSource)
	if req.Consequence != nil {
		req.Consequence.Currency = strings.TrimSpace(req.Consequence.Currency)
	}

	if req.SurfaceID == "" || req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "surface_id and agent_id are required",
		})
		return
	}

	if req.Confidence < 0 || req.Confidence > 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "confidence must be between 0 and 1",
		})
		return
	}

	// request_source is used for logging only in simulation; default to "explorer".
	if req.RequestSource == "" {
		req.RequestSource = "explorer"
	}

	result, err := orch.Simulate(r.Context(), toEvalRequest(req), json.RawMessage(rawBody))
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEvaluation)
		writeJSON(w, statusCode, errResp)
		return
	}

	// envelope_id is intentionally omitted — simulation produces no persistent record.
	writeJSON(w, http.StatusOK, evaluateResponse{
		Outcome:         string(result.Outcome),
		Reason:          string(result.ReasonCode),
		Simulated:       true,
		PolicyMode:      result.PolicyMode,
		PolicyReference: result.PolicyReference,
		PolicySkipped:   result.PolicySkipped,
	})
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

type reviewRequest struct {
	EnvelopeID string `json:"envelope_id"`
	Decision   string `json:"decision"`
	Reviewer   string `json:"reviewer"`
	Notes      string `json:"notes,omitempty"`
}

type reviewResponse struct {
	EnvelopeID string `json:"envelope_id"`
	Status     string `json:"status"`
}

func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{
			"error": err.Error(),
		})
		return
	}

	var req reviewRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	req.EnvelopeID = strings.TrimSpace(req.EnvelopeID)
	req.Decision = strings.TrimSpace(req.Decision)
	req.Reviewer = strings.TrimSpace(req.Reviewer)
	req.Notes = strings.TrimSpace(req.Notes)

	if req.EnvelopeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "envelope_id is required",
		})
		return
	}

	if req.Decision == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "decision is required",
		})
		return
	}

	// Resolve reviewer: authenticated principal overrides the body field when
	// auth is configured; falls back to the body value for unauthenticated deployments.
	reviewer := actorFromContext(r.Context(), req.Reviewer)
	if !isValidIdentifier(reviewer) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "reviewer must be a valid identifier (1-255 characters, no control characters)",
		})
		return
	}

	var reviewDecision envelope.ReviewDecision
	switch strings.ToLower(req.Decision) {
	case "accept", "approve", "approved":
		reviewDecision = envelope.ReviewDecisionApproved
	case "reject", "deny", "denied":
		reviewDecision = envelope.ReviewDecisionRejected
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "decision must be 'accept'/'approve' or 'reject'/'deny'",
		})
		return
	}

	resolvedEnvelope, err := s.orchestrator.ResolveEscalation(r.Context(), decision.EscalationResolution{
		EnvelopeID:   req.EnvelopeID,
		Decision:     reviewDecision,
		ReviewerID:   reviewer,
		ReviewerKind: "human",
		Notes:        req.Notes,
	})
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityReview)
		writeJSON(w, statusCode, errResp)
		return
	}

	if resolvedEnvelope != nil && resolvedEnvelope.ID() != "" && resolvedEnvelope.ID() != req.EnvelopeID {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "envelope identity invariant violated",
		})
		return
	}

	writeJSON(w, http.StatusOK, reviewResponse{
		EnvelopeID: req.EnvelopeID,
		Status:     "resolved",
	})
}

// ---------------------------------------------------------------------------
// Envelope Retrieval
// ---------------------------------------------------------------------------

func (s *Server) handleGetEnvelope(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	const prefix = "/v1/envelopes/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "not found",
		})
		return
	}

	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing envelope id",
		})
		return
	}

	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid envelope id",
		})
		return
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	env, err := s.orchestrator.GetEnvelopeByID(r.Context(), id)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope)
		writeJSON(w, statusCode, errResp)
		return
	}

	if env == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "envelope not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, env)
}

// handleListEnvelopes processes GET /v1/envelopes.
// An optional ?state= query parameter filters by envelope lifecycle state.
// Valid states: received, evaluating, outcome_recorded, escalated, awaiting_review, closed.
// Omitting the parameter returns all envelopes.
func (s *Server) handleListEnvelopes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	stateParam := strings.TrimSpace(r.URL.Query().Get("state"))
	if stateParam != "" && !isValidEnvelopeState(envelope.EnvelopeState(stateParam)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid state filter: must be one of received, evaluating, outcome_recorded, escalated, awaiting_review, closed",
		})
		return
	}

	envs, err := s.orchestrator.ListEnvelopesByState(r.Context(), envelope.EnvelopeState(stateParam))
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, envs)
}

// handleListEscalations processes GET /v1/escalations.
// Returns all envelopes in the awaiting_review state — the operator's
// pending escalation queue. These are evaluations that produced an Escalate
// outcome and are waiting for a reviewer to submit a decision via POST /v1/reviews.
func (s *Server) handleListEscalations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	envs, err := s.orchestrator.ListEnvelopesByState(r.Context(), envelope.EnvelopeStateAwaitingReview)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, envs)
}

func (s *Server) handleGetDecisionByRequestID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	const prefix = "/v1/decisions/request/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "not found",
		})
		return
	}

	requestID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if requestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing request id",
		})
		return
	}

	if !isValidIdentifier(requestID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request id",
		})
		return
	}

	requestSource := strings.TrimSpace(r.URL.Query().Get("source"))
	if requestSource == "" {
		requestSource = defaultRequestSource
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	env, err := s.orchestrator.GetEnvelopeByRequestScope(r.Context(), requestSource, requestID)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityDecision)
		writeJSON(w, statusCode, errResp)
		return
	}

	if env == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "decision not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, env)
}

// ---------------------------------------------------------------------------
// Control Plane - Apply Bundle
// ---------------------------------------------------------------------------

func (s *Server) handleApplyBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if s.controlPlane == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "control plane not configured",
		})
		return
	}

	if !isAllowedYAMLContentType(r.Header.Get("Content-Type")) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"error": "content-type must be application/yaml, application/x-yaml, or text/yaml",
		})
		return
	}

	rawBody, err := readRequestBody(w, r, maxApplyBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{
			"error": err.Error(),
		})
		return
	}

	actor := actorFromContext(r.Context(), strings.TrimSpace(r.Header.Get("X-MIDAS-ACTOR")))
	ctx := s.applyCtxWithKindAuthorizer(r.Context())
	result, err := s.controlPlane.ApplyBundle(ctx, rawBody, actor)
	s.emitApplyAdminAudit(r, actor, rawBody, result, err)
	if err != nil {
		statusCode, errResp := mapApplyError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// emitApplyAdminAudit writes one request-level administrative audit record
// for an apply invocation. This is additive to the per-resource controlaudit
// rows written by the apply executor — it captures the request itself, not
// the resources created by it. See Issue #41.
func (s *Server) emitApplyAdminAudit(
	r *http.Request,
	actor string,
	rawBody []byte,
	result *cpTypes.ApplyResult,
	applyErr error,
) {
	if s.adminAudit == nil {
		return
	}
	rec := adminaudit.NewRecord(
		adminaudit.ActionApplyInvoked,
		adminaudit.OutcomeSuccess,
		actorType(actor),
	)
	rec.ActorID = actor
	rec.TargetType = adminaudit.TargetTypeBundle
	rec.RequestID = strings.TrimSpace(r.Header.Get("X-Request-Id"))
	rec.ClientIP = clientIPFromRequest(r)
	rec.RequiredPermission = string(authz.PermControlplaneApply)

	details := &adminaudit.Details{BundleBytes: len(rawBody)}
	if applyErr != nil {
		rec.Outcome = adminaudit.OutcomeFailure
		details.Error = applyErr.Error()
	} else if result != nil {
		// Treat presence of validation errors or resource errors as a
		// failure at the admin-audit level even though the HTTP response
		// may carry a 200 with details. This matches the operator's
		// notion of "did the apply succeed".
		if result.HasValidationErrors() || result.ApplyErrorCount() > 0 {
			rec.Outcome = adminaudit.OutcomeFailure
		}
		details.CreatedCount = result.CreatedCount()
		details.ConflictCount = result.ConflictCount()
		details.UnchangedCount = result.UnchangedCount()
		details.ErrorCount = result.ApplyErrorCount()
	}
	rec.Details = details

	s.appendAdminAudit(r.Context(), rec)
}

// actorType classifies the actor string for an administrative audit record.
// An empty or literal "system" actor is treated as system-initiated;
// everything else is a user action.
func actorType(actor string) adminaudit.ActorType {
	a := strings.TrimSpace(actor)
	if a == "" || strings.HasPrefix(a, "system") {
		return adminaudit.ActorTypeSystem
	}
	return adminaudit.ActorTypeUser
}

// applyCtxWithKindAuthorizer returns ctx enriched with an apply.KindAuthorizer
// constructed from the request principal and the internal/authz permission
// model. This is the fine-grained, per-document layer of the two-tier
// authorization on /v1/controlplane/apply: the middleware gate has already
// enforced controlplane:apply, and the planner will now enforce
// <kind>:write for each document in the bundle.
//
// Behaviour:
//   - AuthModeOpen: returns a permissive authorizer that allows every Kind,
//     preserving the open-mode pass-through contract.
//   - Authenticated mode with principal in context: returns an authorizer
//     that consults authz.HasPermission against the principal's normalised
//     roles.
//   - Authenticated mode with no principal: returns a deny-all authorizer.
//     In practice this branch is unreachable because the middleware would
//     have already returned 401; the defensive deny keeps the planner safe
//     if called outside the middleware chain.
func (s *Server) applyCtxWithKindAuthorizer(ctx context.Context) context.Context {
	if s.authMode == config.AuthModeOpen {
		return apply.WithKindAuthorizer(ctx, func(string) (bool, string) { return true, "" })
	}

	p := PrincipalFromContext(ctx)
	return apply.WithKindAuthorizer(ctx, func(kind string) (bool, string) {
		required := authz.KindToWritePermission(kind)
		if required == "" {
			// Unknown Kind — deny by default rather than leak an empty
			// permission name into the response. Unknown Kinds also fail
			// parsing earlier, so this branch is defensive.
			return false, "unknown-kind"
		}
		if p == nil {
			return false, string(required)
		}
		if !authz.HasPermission(p, required) {
			return false, string(required)
		}
		return true, ""
	})
}

// ---------------------------------------------------------------------------
// Control Plane - Plan Bundle (dry-run)
// ---------------------------------------------------------------------------

// handlePlanBundle accepts the same YAML bundle format as handleApplyBundle and
// returns a structured plan describing what would happen if the bundle were
// applied. No writes occur.
//
// POST /v1/controlplane/plan
func (s *Server) handlePlanBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if s.controlPlane == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "control plane not configured",
		})
		return
	}

	if !isAllowedYAMLContentType(r.Header.Get("Content-Type")) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"error": "content-type must be application/yaml, application/x-yaml, or text/yaml",
		})
		return
	}

	rawBody, err := readRequestBody(w, r, maxApplyBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Plan shares the same planner pipeline as Apply (see buildApplyPlan), so
	// we inject the same per-document authorizer. Under the default five
	// roles this is observationally a no-op — only platform.admin holds
	// controlplane:plan, and platform.admin holds every <kind>:write — but
	// the injection makes the two-tier model uniform: any custom role
	// composition that grants controlplane:plan without some <kind>:write
	// will see that Kind as invalid in the plan response rather than as a
	// false-positive "create".
	ctx := s.applyCtxWithKindAuthorizer(r.Context())
	plan, err := s.controlPlane.PlanBundle(ctx, rawBody)
	if err != nil {
		statusCode, errResp := mapApplyError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	result := apply.PlanResultFromPlan(*plan)
	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Operator Introspection
// ---------------------------------------------------------------------------

// handleGetSurfaceOrVersions dispatches:
//   - GET /v1/surfaces/{id}          → handleGetSurface
//   - GET /v1/surfaces/{id}/versions → handleGetSurfaceVersions
func (s *Server) handleGetSurfaceOrVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	const prefix = "/v1/surfaces/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	rest = strings.Trim(rest, "/")

	if rest == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing surface id"})
		return
	}

	parts := strings.SplitN(rest, "/", 2)
	surfaceID := strings.TrimSpace(parts[0])

	if !isValidIdentifier(surfaceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid surface id"})
		return
	}

	if len(parts) == 2 && parts[1] == "versions" {
		s.handleGetSurfaceVersions(w, r, surfaceID)
		return
	}

	if len(parts) == 2 && parts[1] == "impact" {
		s.handleGetSurfaceImpact(w, r, surfaceID)
		return
	}

	if len(parts) == 2 && parts[1] == "recovery" {
		s.handleGetSurfaceRecovery(w, r, surfaceID)
		return
	}

	if len(parts) > 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	s.handleGetSurface(w, r, surfaceID)
}

// handleGetSurfaceImpact processes GET /v1/surfaces/{id}/impact.
// It returns the full dependency graph for the surface: profiles, grants,
// distinct agents, aggregate counts, and operator warnings. The response
// shape is always complete — every array is present even when empty.
func (s *Server) handleGetSurfaceImpact(w http.ResponseWriter, r *http.Request, surfaceID string) {
	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	result, err := s.introspection.GetSurfaceImpact(r.Context(), surfaceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "surface not found"})
		return
	}

	writeJSON(w, http.StatusOK, toSurfaceImpactResponse(result))
}

// handleGetSurfaceRecovery processes GET /v1/surfaces/{id}/recovery.
// Returns a read-only recovery analysis: version history state, active/latest
// distinction, successor links, and deterministic recommended next actions.
func (s *Server) handleGetSurfaceRecovery(w http.ResponseWriter, r *http.Request, surfaceID string) {
	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	result, err := s.introspection.GetSurfaceRecovery(r.Context(), surfaceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "surface not found"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetProfileRecovery processes GET /v1/profiles/{id}/recovery.
// Returns a read-only recovery analysis: version history state, active/latest
// distinction, grant counts, capability notes, and deterministic recommended next actions.
func (s *Server) handleGetProfileRecovery(w http.ResponseWriter, r *http.Request, profileID string) {
	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	result, err := s.introspection.GetProfileRecovery(r.Context(), profileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetSurface(w http.ResponseWriter, r *http.Request, surfaceID string) {
	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	surf, err := s.introspection.GetSurface(r.Context(), surfaceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if surf == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "surface not found"})
		return
	}

	writeJSON(w, http.StatusOK, toSurfaceResponse(surf))
}

func (s *Server) handleGetSurfaceVersions(w http.ResponseWriter, r *http.Request, surfaceID string) {
	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	versions, err := s.introspection.ListSurfaceVersions(r.Context(), surfaceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(versions) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "surface not found"})
		return
	}

	out := make([]surfaceVersionResponse, 0, len(versions))
	for _, v := range versions {
		out = append(out, toSurfaceVersionResponse(v))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListProfiles processes GET /v1/profiles?surface_id={id}.
func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	surfaceID := strings.TrimSpace(r.URL.Query().Get("surface_id"))
	if surfaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "surface_id query parameter is required"})
		return
	}

	if !isValidIdentifier(surfaceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid surface_id"})
		return
	}

	profiles, err := s.introspection.ListProfilesBySurface(r.Context(), surfaceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	out := make([]profileResponse, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, toProfileResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetProfileOrVersions dispatches:
//   - GET /v1/profiles/{id}          → handleGetProfile
//   - GET /v1/profiles/{id}/versions → handleGetProfileVersions
func (s *Server) handleGetProfileOrVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	const prefix = "/v1/profiles/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	rest = strings.Trim(rest, "/")

	if rest == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing profile id"})
		return
	}

	parts := strings.SplitN(rest, "/", 2)
	profileID := strings.TrimSpace(parts[0])

	if !isValidIdentifier(profileID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid profile id"})
		return
	}

	if len(parts) == 2 && parts[1] == "versions" {
		s.handleGetProfileVersions(w, r, profileID)
		return
	}

	if len(parts) == 2 && parts[1] == "recovery" {
		s.handleGetProfileRecovery(w, r, profileID)
		return
	}

	if len(parts) > 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	s.handleGetProfile(w, r, profileID)
}

// handleGetProfile processes GET /v1/profiles/{id}.
// Returns the latest version of the profile with the given logical ID.
func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request, profileID string) {
	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	profile, err := s.introspection.GetProfile(r.Context(), profileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if profile == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
		return
	}

	writeJSON(w, http.StatusOK, toProfileResponse(profile))
}

// handleGetProfileVersions processes GET /v1/profiles/{id}/versions.
// Returns all versions of the profile ordered by version descending (latest first).
// Returns 404 when no profile with that logical ID exists.
func (s *Server) handleGetProfileVersions(w http.ResponseWriter, r *http.Request, profileID string) {
	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	versions, err := s.introspection.ListProfileVersions(r.Context(), profileID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(versions) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
		return
	}

	out := make([]profileResponse, 0, len(versions))
	for _, v := range versions {
		out = append(out, toProfileResponse(v))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetAgent processes GET /v1/agents/{id}.
func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	const prefix = "/v1/agents/"
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing agent id"})
		return
	}

	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	ag, err := s.introspection.GetAgent(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if ag == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	writeJSON(w, http.StatusOK, toAgentResponse(ag))
}

// handleListGrants processes GET /v1/grants?agent_id={id} or GET /v1/grants?profile_id={id}.
// Exactly one of agent_id or profile_id must be provided.
func (s *Server) handleListGrants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	profileID := strings.TrimSpace(r.URL.Query().Get("profile_id"))

	if agentID == "" && profileID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "agent_id or profile_id query parameter is required",
		})
		return
	}

	if agentID != "" && profileID != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "only one of agent_id or profile_id may be specified",
		})
		return
	}

	var (
		grants []*authority.AuthorityGrant
		err    error
	)

	if agentID != "" {
		if !isValidIdentifier(agentID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
			return
		}
		grants, err = s.introspection.ListGrantsByAgent(r.Context(), agentID)
	} else {
		if !isValidIdentifier(profileID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid profile_id"})
			return
		}
		grants, err = s.introspection.ListGrantsByProfile(r.Context(), profileID)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	out := make([]grantResponse, 0, len(grants))
	for _, g := range grants {
		out = append(out, toGrantResponse(g))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetGrant processes GET /v1/grants/{id}.
// Returns the grant with the given ID.
func (s *Server) handleGetGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.introspection == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "introspection service not configured",
		})
		return
	}

	const prefix = "/v1/grants/"
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing grant id"})
		return
	}

	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid grant id"})
		return
	}

	g, err := s.introspection.GetGrant(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if g == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "grant not found"})
		return
	}

	writeJSON(w, http.StatusOK, toGrantResponse(g))
}

// toSurfaceResponse maps a DecisionSurface to its wire format.
func toSurfaceResponse(s *surface.DecisionSurface) surfaceResponse {
	return surfaceResponse{
		ID:                 s.ID,
		Name:               s.Name,
		Status:             string(s.Status),
		Version:            s.Version,
		EffectiveFrom:      s.EffectiveFrom,
		ApprovedAt:         s.ApprovedAt,
		ApprovedBy:         s.ApprovedBy,
		SuccessorSurfaceID: s.SuccessorSurfaceID,
		DeprecationReason:  s.DeprecationReason,
		Domain:             s.Domain,
		BusinessOwner:      s.BusinessOwner,
		TechnicalOwner:     s.TechnicalOwner,
	}
}

// toSurfaceVersionResponse maps a DecisionSurface version to its wire format.
func toSurfaceVersionResponse(s *surface.DecisionSurface) surfaceVersionResponse {
	return surfaceVersionResponse{
		Version:       s.Version,
		Status:        string(s.Status),
		EffectiveFrom: s.EffectiveFrom,
		ApprovedAt:    s.ApprovedAt,
		ApprovedBy:    s.ApprovedBy,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

// toProfileResponse maps an AuthorityProfile to its wire format.
func toProfileResponse(p *authority.AuthorityProfile) profileResponse {
	return profileResponse{
		ID:                  p.ID,
		Version:             p.Version,
		SurfaceID:           p.SurfaceID,
		Name:                p.Name,
		Description:         p.Description,
		Status:              string(p.Status),
		EffectiveDate:       p.EffectiveDate,
		ConfidenceThreshold: p.ConfidenceThreshold,
		EscalationMode:      string(p.EscalationMode),
		FailMode:            string(p.FailMode),
		PolicyReference:     p.PolicyReference,
		RequiredContextKeys: p.RequiredContextKeys,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
		ApprovedBy:          p.ApprovedBy,
		ApprovedAt:          p.ApprovedAt,
	}
}

// toAgentResponse maps an Agent to its wire format.
func toAgentResponse(a *agent.Agent) agentResponse {
	return agentResponse{
		ID:               a.ID,
		Name:             a.Name,
		Type:             string(a.Type),
		Owner:            a.Owner,
		ModelVersion:     a.ModelVersion,
		Endpoint:         a.Endpoint,
		OperationalState: string(a.OperationalState),
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
	}
}

// toSurfaceImpactResponse maps a SurfaceImpactResult to its wire format.
// All slice fields default to empty arrays (never null) so callers can iterate
// without nil checks.
func toSurfaceImpactResponse(r *SurfaceImpactResult) surfaceImpactResponse {
	profiles := make([]impactProfileEntry, 0, len(r.Profiles))
	for _, p := range r.Profiles {
		profiles = append(profiles, impactProfileEntry{
			ID:              p.ID,
			Name:            p.Name,
			Status:          string(p.Status),
			Version:         p.Version,
			PolicyReference: p.PolicyReference,
			FailMode:        string(p.FailMode),
			EscalationMode:  string(p.EscalationMode),
		})
	}

	grants := make([]impactGrantEntry, 0, len(r.Grants))
	for _, g := range r.Grants {
		grants = append(grants, impactGrantEntry{
			ID:             g.ID,
			AgentID:        g.AgentID,
			ProfileID:      g.ProfileID,
			Status:         string(g.Status),
			GrantedBy:      g.GrantedBy,
			EffectiveFrom:  g.EffectiveDate,
			EffectiveUntil: g.ExpiresAt,
		})
	}

	agents := make([]impactAgentEntry, 0, len(r.Agents))
	for _, a := range r.Agents {
		agents = append(agents, impactAgentEntry{
			ID:               a.ID,
			Name:             a.Name,
			Type:             string(a.Type),
			Owner:            a.Owner,
			OperationalState: string(a.OperationalState),
		})
	}

	surf := r.Surface
	warnings := r.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	return surfaceImpactResponse{
		Surface: surfaceImpactSurfaceEntry{
			ID:                 surf.ID,
			Name:               surf.Name,
			Status:             string(surf.Status),
			Version:            surf.Version,
			Domain:             surf.Domain,
			BusinessOwner:      surf.BusinessOwner,
			TechnicalOwner:     surf.TechnicalOwner,
			ApprovedBy:         surf.ApprovedBy,
			ApprovedAt:         surf.ApprovedAt,
			DeprecationReason:  surf.DeprecationReason,
			SuccessorSurfaceID: surf.SuccessorSurfaceID,
		},
		Profiles: profiles,
		Grants:   grants,
		Agents:   agents,
		Summary: impactSummaryResponse{
			ProfileCount:       r.Summary.ProfileCount,
			GrantCount:         r.Summary.GrantCount,
			AgentCount:         r.Summary.AgentCount,
			ActiveProfileCount: r.Summary.ActiveProfileCount,
			ActiveGrantCount:   r.Summary.ActiveGrantCount,
			ActiveAgentCount:   r.Summary.ActiveAgentCount,
		},
		Warnings: warnings,
	}
}

// toGrantResponse maps an AuthorityGrant to its wire format.
func toGrantResponse(g *authority.AuthorityGrant) grantResponse {
	return grantResponse{
		ID:            g.ID,
		AgentID:       g.AgentID,
		ProfileID:     g.ProfileID,
		Status:        string(g.Status),
		GrantedBy:     g.GrantedBy,
		EffectiveDate: g.EffectiveDate,
		ExpiresAt:     g.ExpiresAt,
		CreatedAt:     g.CreatedAt,
		UpdatedAt:     g.UpdatedAt,
	}
}

// toCapabilityResponse maps a Capability to its wire format.
func toCapabilityResponse(c *capability.Capability) capabilityResponse {
	return capabilityResponse{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		Status:      c.Status,
		Owner:       c.Owner,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

// toProcessResponse maps a Process to its wire format.
func toProcessResponse(p *process.Process) processResponse {
	return processResponse{
		ID:                p.ID,
		Name:              p.Name,
		BusinessServiceID: p.BusinessServiceID,
		Description:       p.Description,
		Status:            p.Status,
		Owner:             p.Owner,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

// toExternalRefResponse maps a domain *externalref.ExternalRef into the
// wire shape (Epic 1, PR 3). The fourth canonicalisation point: returns
// nil for nil-or-IsZero refs, matching the contract enforced by the
// memory and Postgres storage layers and the apply mapper.
//
// LastSyncedAt is rendered as RFC3339 in UTC. The pointer wrapper keeps
// "no timestamp recorded" distinguishable from the zero time on the
// wire — callers can branch on the field's presence.
func toExternalRefResponse(ref *externalref.ExternalRef) *externalRefResponse {
	if ref.IsZero() {
		return nil
	}
	out := &externalRefResponse{
		SourceSystem:  ref.SourceSystem,
		SourceID:      ref.SourceID,
		SourceURL:     ref.SourceURL,
		SourceVersion: ref.SourceVersion,
	}
	if ref.LastSyncedAt != nil {
		s := ref.LastSyncedAt.UTC().Format(time.RFC3339)
		out.LastSyncedAt = &s
	}
	return out
}

// toBusinessServiceResponse maps a BusinessService to its wire format.
func toBusinessServiceResponse(s *businessservice.BusinessService) businessServiceResponse {
	return businessServiceResponse{
		ID:              s.ID,
		Name:            s.Name,
		Description:     s.Description,
		ServiceType:     string(s.ServiceType),
		RegulatoryScope: s.RegulatoryScope,
		Status:          s.Status,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
		ExternalRef:     toExternalRefResponse(s.ExternalRef),
	}
}

// toBusinessServiceRelationshipResponse maps one domain BSR row into the
// wire shape (Epic 1, PR 1).
func toBusinessServiceRelationshipResponse(rel *businessservice.BusinessServiceRelationship) businessServiceRelationshipResponse {
	return businessServiceRelationshipResponse{
		ID:                      rel.ID,
		SourceBusinessServiceID: rel.SourceBusinessService,
		TargetBusinessServiceID: rel.TargetBusinessService,
		RelationshipType:        rel.RelationshipType,
		Description:             rel.Description,
		CreatedAt:               rel.CreatedAt,
		CreatedBy:               rel.CreatedBy,
		ExternalRef:             toExternalRefResponse(rel.ExternalRef),
	}
}

// toBusinessServiceRelationshipsResponse builds the top-level response for
// GET /v1/businessservices/{id}/relationships. Outgoing/Incoming are always
// non-nil arrays so callers can iterate without nil checks (Epic 1, PR 1).
func toBusinessServiceRelationshipsResponse(businessServiceID string, outgoing, incoming []*businessservice.BusinessServiceRelationship) businessServiceRelationshipsResponse {
	resp := businessServiceRelationshipsResponse{
		BusinessServiceID: businessServiceID,
		Outgoing:          make([]businessServiceRelationshipResponse, 0, len(outgoing)),
		Incoming:          make([]businessServiceRelationshipResponse, 0, len(incoming)),
	}
	for _, rel := range outgoing {
		resp.Outgoing = append(resp.Outgoing, toBusinessServiceRelationshipResponse(rel))
	}
	for _, rel := range incoming {
		resp.Incoming = append(resp.Incoming, toBusinessServiceRelationshipResponse(rel))
	}
	return resp
}

// ---------------------------------------------------------------------------
// AI System Registration response helpers (Epic 1, PR 2)
// ---------------------------------------------------------------------------

func toAISystemResponse(sys *aisystem.AISystem) aiSystemResponse {
	out := aiSystemResponse{
		ID:          sys.ID,
		Name:        sys.Name,
		Description: sys.Description,
		Owner:       sys.Owner,
		Vendor:      sys.Vendor,
		SystemType:  sys.SystemType,
		Status:      sys.Status,
		Origin:      sys.Origin,
		Managed:     sys.Managed,
		CreatedAt:   sys.CreatedAt,
		UpdatedAt:   sys.UpdatedAt,
		CreatedBy:   sys.CreatedBy,
		ExternalRef: toExternalRefResponse(sys.ExternalRef),
	}
	if sys.Replaces != "" {
		s := sys.Replaces
		out.Replaces = &s
	}
	return out
}

func toAISystemsResponse(systems []*aisystem.AISystem) aiSystemsResponse {
	resp := aiSystemsResponse{AISystems: make([]aiSystemResponse, 0, len(systems))}
	for _, sys := range systems {
		resp.AISystems = append(resp.AISystems, toAISystemResponse(sys))
	}
	return resp
}

func toAISystemVersionResponse(ver *aisystem.AISystemVersion) aiSystemVersionResponse {
	frameworks := ver.ComplianceFrameworks
	if frameworks == nil {
		frameworks = []string{}
	}
	return aiSystemVersionResponse{
		AISystemID:           ver.AISystemID,
		Version:              ver.Version,
		ReleaseLabel:         ver.ReleaseLabel,
		ModelArtifact:        ver.ModelArtifact,
		ModelHash:            ver.ModelHash,
		Endpoint:             ver.Endpoint,
		Status:               ver.Status,
		EffectiveFrom:        ver.EffectiveFrom,
		EffectiveUntil:       ver.EffectiveUntil,
		RetiredAt:            ver.RetiredAt,
		ComplianceFrameworks: frameworks,
		DocumentationURL:     ver.DocumentationURL,
		CreatedAt:            ver.CreatedAt,
		UpdatedAt:            ver.UpdatedAt,
		CreatedBy:            ver.CreatedBy,
		ExternalRef:          toExternalRefResponse(ver.ExternalRef),
	}
}

func toAISystemVersionsResponse(aiSystemID string, versions []*aisystem.AISystemVersion) aiSystemVersionsResponse {
	resp := aiSystemVersionsResponse{
		AISystemID: aiSystemID,
		Versions:   make([]aiSystemVersionResponse, 0, len(versions)),
	}
	for _, ver := range versions {
		resp.Versions = append(resp.Versions, toAISystemVersionResponse(ver))
	}
	return resp
}

// nullableStringPtr returns nil when the input is empty, otherwise a
// pointer to a copy. Used by toAISystemBindingResponse so that JSON
// marshalling produces null for unset context fields rather than "".
func nullableStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	cp := s
	return &cp
}

func toAISystemBindingResponse(b *aisystem.AISystemBinding) aiSystemBindingResponse {
	var version *int
	if b.AISystemVersion != nil {
		v := *b.AISystemVersion
		version = &v
	}
	return aiSystemBindingResponse{
		ID:                b.ID,
		AISystemID:        b.AISystemID,
		AISystemVersion:   version,
		BusinessServiceID: nullableStringPtr(b.BusinessServiceID),
		CapabilityID:      nullableStringPtr(b.CapabilityID),
		ProcessID:         nullableStringPtr(b.ProcessID),
		SurfaceID:         nullableStringPtr(b.SurfaceID),
		Role:              b.Role,
		Description:       b.Description,
		CreatedAt:         b.CreatedAt,
		CreatedBy:         b.CreatedBy,
		ExternalRef:       toExternalRefResponse(b.ExternalRef),
	}
}

func toAISystemBindingsResponse(aiSystemID string, bindings []*aisystem.AISystemBinding) aiSystemBindingsResponse {
	resp := aiSystemBindingsResponse{
		AISystemID: aiSystemID,
		Bindings:   make([]aiSystemBindingResponse, 0, len(bindings)),
	}
	for _, b := range bindings {
		resp.Bindings = append(resp.Bindings, toAISystemBindingResponse(b))
	}
	return resp
}

// ---------------------------------------------------------------------------
// Structural handlers — /v1/capabilities and /v1/processes
// ---------------------------------------------------------------------------

// handleListCapabilities serves GET /v1/capabilities.
func (s *Server) handleListCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.structural == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "structural service not configured"})
		return
	}
	caps, err := s.structural.ListCapabilities(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]capabilityResponse, 0, len(caps))
	for _, c := range caps {
		out = append(out, toCapabilityResponse(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetCapability serves GET /v1/capabilities/{id}.
//
// In the v1 service-led model this handler no longer dispatches a sub-path:
// the previous /v1/capabilities/{id}/processes endpoint has been removed
// because Process is no longer owned by Capability. The route registration
// for the prefix /v1/capabilities/ is preserved so that {id} requests
// continue to resolve via the net/http subtree match.
func (s *Server) handleGetCapability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/capabilities/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing capability id"})
		return
	}
	if strings.Contains(rest, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := strings.TrimSpace(rest)
	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid capability id"})
		return
	}
	if s.structural == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "structural service not configured"})
		return
	}
	cap, err := s.structural.GetCapability(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cap == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "capability not found"})
		return
	}
	writeJSON(w, http.StatusOK, toCapabilityResponse(cap))
}

// handleListProcesses serves GET /v1/processes.
func (s *Server) handleListProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.structural == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "structural service not configured"})
		return
	}
	procs, err := s.structural.ListProcesses(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]processResponse, 0, len(procs))
	for _, p := range procs {
		out = append(out, toProcessResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetProcessOrSurfaces dispatches:
//
//	GET /v1/processes/{id}           → handleGetProcess
//	GET /v1/processes/{id}/surfaces  → handleGetProcessSurfaces
func (s *Server) handleGetProcessOrSurfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/processes/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing process id"})
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id := strings.TrimSpace(parts[0])
	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid process id"})
		return
	}
	if len(parts) == 2 && parts[1] == "surfaces" {
		s.handleGetProcessSurfaces(w, r, id)
		return
	}
	if len(parts) > 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	s.handleGetProcess(w, r, id)
}

func (s *Server) handleGetProcess(w http.ResponseWriter, r *http.Request, id string) {
	if s.structural == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "structural service not configured"})
		return
	}
	proc, err := s.structural.GetProcess(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if proc == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "process not found"})
		return
	}
	writeJSON(w, http.StatusOK, toProcessResponse(proc))
}

func (s *Server) handleGetProcessSurfaces(w http.ResponseWriter, r *http.Request, processID string) {
	if s.structural == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "structural service not configured"})
		return
	}
	surfs, found, err := s.structural.ListSurfacesByProcess(r.Context(), processID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "process not found"})
		return
	}
	out := make([]surfaceResponse, 0, len(surfs))
	for _, surf := range surfs {
		out = append(out, toSurfaceResponse(surf))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListBusinessServices serves GET /v1/businessservices.
func (s *Server) handleListBusinessServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.structural == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "structural service not configured"})
		return
	}
	svcs, err := s.structural.ListBusinessServices(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]businessServiceResponse, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, toBusinessServiceResponse(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetBusinessService serves GET /v1/businessservices/{id} and the
// sub-path GET /v1/businessservices/{id}/relationships (Epic 1, PR 1).
//
// Sub-path routing mirrors handleGetSurfaceOrVersions / handleGetProcessOrSurfaces:
// the path tail after /{id}/ is matched against a small, explicit set of
// supported sub-paths. Unknown sub-paths return 404.
func (s *Server) handleGetBusinessService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.structural == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "structural service not configured"})
		return
	}
	const prefix = "/v1/businessservices/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	parts := strings.Split(tail, "/")
	id := parts[0]

	if len(parts) == 2 && parts[1] == "relationships" {
		s.handleGetBusinessServiceRelationships(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "governance-map" {
		s.handleGetBusinessServiceGovernanceMap(w, r, id)
		return
	}
	if len(parts) > 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	svc, err := s.structural.GetBusinessService(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if svc == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "business service not found"})
		return
	}
	writeJSON(w, http.StatusOK, toBusinessServiceResponse(svc))
}

// handleGetBusinessServiceRelationships serves
// GET /v1/businessservices/{id}/relationships.
//
// Response shape: { business_service_id, outgoing[], incoming[] }. Outgoing
// rows are those where the queried service is the source; incoming rows are
// those where it is the target. Both arrays are always present (never null).
//
// Status codes:
//   - 200 OK on success (including empty outgoing/incoming arrays)
//   - 404 Not Found when the queried business_service_id does not exist
//   - 500 Internal Server Error on repository failure
//   - 501 Not Implemented when the BSR reader is not configured
//
// Auth: enforced by the same middleware as the parent endpoint
// (requireAuth + requireRole(PlatformViewer | PlatformOperator | PlatformAdmin)).
func (s *Server) handleGetBusinessServiceRelationships(w http.ResponseWriter, r *http.Request, businessServiceID string) {
	if !s.structural.HasBusinessServiceRelationships() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "business service relationships reader not configured",
		})
		return
	}
	outgoing, incoming, found, err := s.structural.ListRelationshipsForBusinessService(r.Context(), businessServiceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "business service not found"})
		return
	}
	writeJSON(w, http.StatusOK, toBusinessServiceRelationshipsResponse(businessServiceID, outgoing, incoming))
}

// ---------------------------------------------------------------------------
// AI System Registration handlers (Epic 1, PR 2)
// ---------------------------------------------------------------------------

// handleListAISystems serves GET /v1/aisystems.
//
// Status codes:
//   - 200 OK on success (always — empty array when no systems)
//   - 500 on repository failure
//   - 501 when the AISystem reader is not configured
func (s *Server) handleListAISystems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.structural == nil || !s.structural.HasAISystems() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "ai system reader not configured",
		})
		return
	}
	systems, err := s.structural.ListAISystems(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toAISystemsResponse(systems))
}

// handleGetAISystemOrSubpath serves GET /v1/aisystems/{id} and the
// sub-paths:
//
//	GET /v1/aisystems/{id}/versions
//	GET /v1/aisystems/{id}/versions/{version}
//	GET /v1/aisystems/{id}/bindings
//
// Sub-path routing mirrors handleGetBusinessService. Unknown sub-paths
// return 404. Method-not-allowed (any non-GET) returns 405.
func (s *Server) handleGetAISystemOrSubpath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.structural == nil || !s.structural.HasAISystems() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "ai system reader not configured",
		})
		return
	}
	const prefix = "/v1/aisystems/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	parts := strings.Split(tail, "/")
	id := parts[0]

	switch {
	case len(parts) == 1:
		s.handleGetAISystem(w, r, id)
	case len(parts) == 2 && parts[1] == "versions":
		s.handleListAISystemVersions(w, r, id)
	case len(parts) == 2 && parts[1] == "bindings":
		s.handleListAISystemBindings(w, r, id)
	case len(parts) == 3 && parts[1] == "versions":
		version, err := strconv.Atoi(parts[2])
		if err != nil || version < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be a positive integer"})
			return
		}
		s.handleGetAISystemVersion(w, r, id, version)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleGetAISystem serves GET /v1/aisystems/{id}.
func (s *Server) handleGetAISystem(w http.ResponseWriter, r *http.Request, id string) {
	sys, err := s.structural.GetAISystem(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if sys == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ai system not found"})
		return
	}
	writeJSON(w, http.StatusOK, toAISystemResponse(sys))
}

// handleListAISystemVersions serves GET /v1/aisystems/{id}/versions.
//
// Status codes:
//   - 200 OK on success (versions array always present, possibly empty)
//   - 404 when the parent ai system does not exist
//   - 500 on repository failure
//   - 501 when the version reader is not configured (parent reader may
//     still be wired)
func (s *Server) handleListAISystemVersions(w http.ResponseWriter, r *http.Request, aiSystemID string) {
	if !s.structural.HasAISystemVersions() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "ai system version reader not configured",
		})
		return
	}
	versions, found, err := s.structural.ListAISystemVersions(r.Context(), aiSystemID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ai system not found"})
		return
	}
	writeJSON(w, http.StatusOK, toAISystemVersionsResponse(aiSystemID, versions))
}

// handleGetAISystemVersion serves GET /v1/aisystems/{id}/versions/{version}.
//
// Status codes:
//   - 200 OK on success
//   - 400 when {version} is not a positive integer (handled upstream)
//   - 404 when either the parent ai system or the requested version
//     does not exist (the message distinguishes the two)
//   - 500 on repository failure
//   - 501 when the version reader is not configured
func (s *Server) handleGetAISystemVersion(w http.ResponseWriter, r *http.Request, aiSystemID string, version int) {
	if !s.structural.HasAISystemVersions() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "ai system version reader not configured",
		})
		return
	}
	ver, parentFound, err := s.structural.GetAISystemVersion(r.Context(), aiSystemID, version)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !parentFound {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ai system not found"})
		return
	}
	if ver == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ai system version not found"})
		return
	}
	writeJSON(w, http.StatusOK, toAISystemVersionResponse(ver))
}

// handleListAISystemBindings serves GET /v1/aisystems/{id}/bindings.
//
// Status codes:
//   - 200 OK on success (bindings array always present, possibly empty)
//   - 404 when the parent ai system does not exist
//   - 500 on repository failure
//   - 501 when the binding reader is not configured
func (s *Server) handleListAISystemBindings(w http.ResponseWriter, r *http.Request, aiSystemID string) {
	if !s.structural.HasAISystemBindings() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "ai system binding reader not configured",
		})
		return
	}
	bindings, found, err := s.structural.ListAISystemBindings(r.Context(), aiSystemID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ai system not found"})
		return
	}
	writeJSON(w, http.StatusOK, toAISystemBindingsResponse(aiSystemID, bindings))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const (
	entityEvaluation = "evaluation"
	entityReview     = "review"
	entityEnvelope   = "envelope"
	entityDecision   = "decision"
)

func toEvalRequest(req evaluateRequest) eval.DecisionRequest {
	var consequence *eval.Consequence
	if req.Consequence != nil {
		consequence = &eval.Consequence{
			Type:       req.Consequence.Type,
			Amount:     req.Consequence.Amount,
			Currency:   req.Consequence.Currency,
			RiskRating: req.Consequence.RiskRating,
		}
	}

	return eval.DecisionRequest{
		SurfaceID:     req.SurfaceID,
		AgentID:       req.AgentID,
		Confidence:    req.Confidence,
		Consequence:   consequence,
		Context:       req.Context,
		RequestSource: req.RequestSource,
		RequestID:     req.RequestID,
	}
}

// mapDomainError translates domain errors to HTTP status codes and response bodies.
func mapDomainError(err error, entityType string) (int, map[string]string) {
	if err == nil {
		return http.StatusOK, nil
	}

	switch {
	case errors.Is(err, decision.ErrEnvelopeNotFound):
		msg := entityType + " not found"
		if entityType == entityReview {
			msg = "envelope not found"
		}
		return http.StatusNotFound, map[string]string{"error": msg}

	case errors.Is(err, decision.ErrEnvelopeNotAwaitingReview),
		errors.Is(err, decision.ErrEnvelopeAlreadyClosed):
		return http.StatusConflict, map[string]string{"error": err.Error()}

	case errors.Is(err, decision.ErrEmptyIdentifier),
		errors.Is(err, decision.ErrInvalidReviewDecision):
		return http.StatusBadRequest, map[string]string{"error": err.Error()}

	case errors.Is(err, decision.ErrScopedRequestConflict):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	}

	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "not found"):
		return http.StatusNotFound, map[string]string{"error": errMsg}
	case strings.Contains(errMsg, "invalid state"),
		strings.Contains(errMsg, "already resolved"),
		strings.Contains(errMsg, "already closed"):
		return http.StatusConflict, map[string]string{"error": errMsg}
	case strings.Contains(errMsg, "self-review"),
		strings.Contains(errMsg, "insufficient authority"):
		return http.StatusForbidden, map[string]string{"error": errMsg}
	case strings.Contains(errMsg, "duplicate"):
		return http.StatusConflict, map[string]string{"error": errMsg}
	default:
		return http.StatusInternalServerError, map[string]string{"error": errMsg}
	}
}

// mapApprovalError translates approval and deprecation service errors to HTTP status codes.
// Typed sentinel errors take precedence over string-matching fallbacks.
func mapApprovalError(err error) (int, map[string]string) {
	if err == nil {
		return http.StatusOK, nil
	}

	switch {
	case errors.Is(err, approval.ErrSurfaceNotFound):
		return http.StatusNotFound, map[string]string{"error": "surface not found"}
	case errors.Is(err, approval.ErrProfileNotFound):
		return http.StatusNotFound, map[string]string{"error": "profile not found"}
	case errors.Is(err, approval.ErrApprovalForbidden):
		return http.StatusForbidden, map[string]string{"error": "approver is not authorized to approve this surface"}
	case errors.Is(err, approval.ErrInvalidStatus):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	case errors.Is(err, approval.ErrInvalidTransition):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	case errors.Is(err, approval.ErrProfileNotInReview):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	case errors.Is(err, approval.ErrProfileNotActive):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	case errors.Is(err, approval.ErrGovernanceExpectationNotFound):
		return http.StatusNotFound, map[string]string{"error": "governance expectation not found"}
	case errors.Is(err, approval.ErrGovernanceExpectationNotInReview):
		return http.StatusConflict, map[string]string{"error": err.Error()}
	default:
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
}

// mapApplyError translates control-plane apply errors to HTTP status codes.
func mapApplyError(err error) (int, map[string]string) {
	if err == nil {
		return http.StatusOK, nil
	}

	switch {
	case errors.Is(err, apply.ErrInvalidBundle),
		errors.Is(err, apply.ErrValidationFailed),
		errors.Is(err, apply.ErrDuplicateResource),
		errors.Is(err, apply.ErrUnsupportedUpdate):
		return http.StatusBadRequest, map[string]string{"error": err.Error()}

	case errors.Is(err, apply.ErrResourceConflict),
		errors.Is(err, apply.ErrVersionConflict):
		return http.StatusConflict, map[string]string{"error": err.Error()}

	case errors.Is(err, apply.ErrReferentialIntegrity):
		return http.StatusUnprocessableEntity, map[string]string{"error": err.Error()}

	default:
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
		"error": "method not allowed",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var errRequestBodyTooLarge = errors.New("request body too large")

func readRequestBody(w http.ResponseWriter, r *http.Request, maxBytes int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, errRequestBodyTooLarge
		}
		return nil, errors.New("failed to read request body")
	}

	if len(bytes.TrimSpace(rawBody)) == 0 {
		return nil, errors.New("request body must not be empty")
	}

	return rawBody, nil
}

func decodeStrictJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON payload")
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("invalid JSON payload")
	}

	return nil
}

func isAllowedYAMLContentType(contentType string) bool {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return true
	}

	return strings.HasPrefix(contentType, "application/yaml") ||
		strings.HasPrefix(contentType, "application/x-yaml") ||
		strings.HasPrefix(contentType, "text/yaml")
}

// isValidEnvelopeState reports whether state is a recognised envelope lifecycle state.
func isValidEnvelopeState(state envelope.EnvelopeState) bool {
	switch state {
	case envelope.EnvelopeStateReceived,
		envelope.EnvelopeStateEvaluating,
		envelope.EnvelopeStateOutcomeRecorded,
		envelope.EnvelopeStateEscalated,
		envelope.EnvelopeStateAwaitingReview,
		envelope.EnvelopeStateClosed:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Control Plane Audit
// ---------------------------------------------------------------------------

// controlAuditEntryResponse is the wire format for a single control-plane audit record.
type controlAuditEntryResponse struct {
	ID              string            `json:"id"`
	OccurredAt      time.Time         `json:"occurred_at"`
	Actor           string            `json:"actor"`
	Action          string            `json:"action"`
	ResourceKind    string            `json:"resource_kind"`
	ResourceID      string            `json:"resource_id"`
	ResourceVersion *int              `json:"resource_version,omitempty"`
	Summary         string            `json:"summary"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// controlAuditListResponse is the wire format for GET /v1/controlplane/audit.
type controlAuditListResponse struct {
	Entries []controlAuditEntryResponse `json:"entries"`
}

// handleListControlAudit serves GET /v1/controlplane/audit.
// Supports query parameters: resource_kind, resource_id, actor, action, limit.
// limit must be a positive integer not exceeding MaxListLimit; if omitted the
// default (DefaultListLimit) is used; values above MaxListLimit return 400.
func (s *Server) handleListControlAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.controlAudit == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "control-plane audit not configured",
		})
		return
	}

	q := r.URL.Query()

	limitStr := strings.TrimSpace(q.Get("limit"))
	limit := 0
	if limitStr != "" {
		parsed, err := parsePositiveInt(limitStr)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit must be a positive integer",
			})
			return
		}
		if parsed > controlaudit.MaxListLimit {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit exceeds maximum allowed value",
			})
			return
		}
		limit = parsed
	}

	f := controlaudit.ListFilter{
		ResourceKind: strings.TrimSpace(q.Get("resource_kind")),
		ResourceID:   strings.TrimSpace(q.Get("resource_id")),
		Actor:        strings.TrimSpace(q.Get("actor")),
		Action:       controlaudit.Action(strings.TrimSpace(q.Get("action"))),
		Limit:        limit,
	}

	records, err := s.controlAudit.ListAudit(r.Context(), f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list control-plane audit records",
		})
		return
	}

	resp := controlAuditListResponse{
		Entries: make([]controlAuditEntryResponse, 0, len(records)),
	}
	for _, rec := range records {
		resp.Entries = append(resp.Entries, toControlAuditEntryResponse(rec))
	}
	writeJSON(w, http.StatusOK, resp)
}

// coverageRecordResponse is the wire format for a single merged coverage
// record returned by GET /v1/coverage. See Issue #56.
type coverageRecordResponse struct {
	RequestSource      string         `json:"request_source"`
	RequestID          string         `json:"request_id"`
	EnvelopeID         string         `json:"envelope_id"`
	ProcessID          string         `json:"process_id"`
	ExpectationID      string         `json:"expectation_id"`
	ExpectationVersion int            `json:"expectation_version"`
	ConditionType      string         `json:"condition_type,omitempty"`
	RequiredSurfaceID  string         `json:"required_surface_id,omitempty"`
	MissingSurfaceID   string         `json:"missing_surface_id,omitempty"`
	ActualSurfaceID    string         `json:"actual_surface_id,omitempty"`
	Status             string         `json:"status"`
	Gap                bool           `json:"gap"`
	Partial            bool           `json:"partial"`
	Summary            map[string]any `json:"summary,omitempty"`
	CorrelationBasis   map[string]any `json:"correlation_basis,omitempty"`
	DetectedAt         *time.Time     `json:"detected_at,omitempty"`
	GapDetectedAt      *time.Time     `json:"gap_detected_at,omitempty"`
}

// coverageListResponse is the wire format for GET /v1/coverage.
//
// limitations is a fixed string array describing the read model's
// scope and correlation semantics. Surfacing it on every response is
// deliberate: callers must be able to interpret the absence of a gap
// record (no false negatives in scope; no claims about other scopes).
// The strings are stable contract — change them only with a wire
// version bump.
type coverageListResponse struct {
	Records     []coverageRecordResponse `json:"records"`
	Limitations []string                 `json:"limitations"`
}

// coverageLimitations is the fixed limitations array surfaced on every
// /v1/coverage response. Documents the read model's scope and the
// correlation semantics it does and does not implement (see #53/#54/#55):
//
//   - scope=process: matching is process-scoped only; business_service
//     and capability scopes are out of scope for this iteration.
//   - correlation=same-evaluation: detected/gap pairs are correlated
//     by (envelope_id, expectation_id, expectation_version) within the
//     same evaluation. No cross-evaluation joins.
//   - no-bypass-detection: this endpoint reports detected/gap events
//     only; it does not infer "expectation should have fired but
//     didn't" from envelope state.
//   - no-time-window-correlation: correlation is keyed strictly on the
//     merge triple, not on temporal proximity.
var coverageLimitations = []string{
	"scope=process",
	"correlation=same-evaluation",
	"no-bypass-detection",
	"no-time-window-correlation",
}

// handleListCoverage serves GET /v1/coverage against the production
// coverage read service. Authorization: viewer-or-higher, mirroring
// /v1/controlplane/audit. See serveCoverageList for response semantics.
func (s *Server) handleListCoverage(w http.ResponseWriter, r *http.Request) {
	s.serveCoverageList(w, r, s.coverageRead)
}

// handleExplorerCoverage serves GET /explorer/coverage against the
// Explorer's isolated coverage read service. The Explorer's audit
// repository is constructed in initExplorerRuntime and is disjoint
// from the production audit repository — this disjointness is the
// load-bearing isolation property pinned by TestExplorerCoverage_*
// in coverage_handler_test.go (Issue #56).
func (s *Server) handleExplorerCoverage(w http.ResponseWriter, r *http.Request) {
	s.serveCoverageList(w, r, s.explorerCoverageRead)
}

// serveCoverageList implements the shared GET handler for both
// /v1/coverage and /explorer/coverage. The two endpoints differ only
// in which read service they consult; everything else — parameter
// parsing, status code mapping, response shape, the limitations
// vocabulary — is identical.
//
// Query parameters (all optional):
//   - request_source, request_id, envelope_id  — exact match
//   - process_id, expectation_id               — exact match (top-level
//     payload key on the underlying audit events)
//   - since, until                             — RFC3339; since is
//     inclusive, until is exclusive
//   - limit                                    — positive integer,
//     <= MaxListLimit; default DefaultListLimit; oversize returns 400
//
// Responses:
//   - 200 OK with coverageListResponse
//   - 400 on invalid limit / unparseable since|until
//   - 405 on non-GET methods
//   - 501 when svc is nil (mirrors /v1/controlplane/audit's behaviour)
//   - 500 on service error
func (s *Server) serveCoverageList(w http.ResponseWriter, r *http.Request, svc coverageReadService) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if svc == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "coverage read service not configured",
		})
		return
	}

	q := r.URL.Query()

	limitStr := strings.TrimSpace(q.Get("limit"))
	limit := 0
	if limitStr != "" {
		parsed, err := parsePositiveInt(limitStr)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit must be a positive integer",
			})
			return
		}
		if parsed > audit.MaxListLimit {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit exceeds maximum allowed value",
			})
			return
		}
		limit = parsed
	}

	since, err := parseRFC3339Param(q.Get("since"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "since must be an RFC3339 timestamp",
		})
		return
	}
	until, err := parseRFC3339Param(q.Get("until"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "until must be an RFC3339 timestamp",
		})
		return
	}

	f := governancecoverage.CoverageFilter{
		RequestSource: strings.TrimSpace(q.Get("request_source")),
		RequestID:     strings.TrimSpace(q.Get("request_id")),
		EnvelopeID:    strings.TrimSpace(q.Get("envelope_id")),
		ProcessID:     strings.TrimSpace(q.Get("process_id")),
		ExpectationID: strings.TrimSpace(q.Get("expectation_id")),
		Since:         since,
		Until:         until,
		Limit:         limit,
	}

	records, err := svc.ListCoverage(r.Context(), f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list coverage records",
		})
		return
	}

	resp := coverageListResponse{
		Records:     make([]coverageRecordResponse, 0, len(records)),
		Limitations: coverageLimitations,
	}
	for _, rec := range records {
		resp.Records = append(resp.Records, toCoverageRecordResponse(rec))
	}
	writeJSON(w, http.StatusOK, resp)
}

// parseRFC3339Param parses an optional RFC3339 timestamp from a query
// parameter. Empty input returns the zero time without error.
func parseRFC3339Param(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func toCoverageRecordResponse(rec *governancecoverage.CoverageRecord) coverageRecordResponse {
	return coverageRecordResponse{
		RequestSource:      rec.RequestSource,
		RequestID:          rec.RequestID,
		EnvelopeID:         rec.EnvelopeID,
		ProcessID:          rec.ProcessID,
		ExpectationID:      rec.ExpectationID,
		ExpectationVersion: rec.ExpectationVersion,
		ConditionType:      rec.ConditionType,
		RequiredSurfaceID:  rec.RequiredSurfaceID,
		MissingSurfaceID:   rec.MissingSurfaceID,
		ActualSurfaceID:    rec.ActualSurfaceID,
		Status:             string(rec.Status),
		Gap:                rec.Gap,
		Partial:            rec.Partial,
		Summary:            rec.Summary,
		CorrelationBasis:   rec.CorrelationBasis,
		DetectedAt:         rec.DetectedAt,
		GapDetectedAt:      rec.GapDetectedAt,
	}
}

// adminAuditEntryResponse is the wire format for a single admin-audit record.
type adminAuditEntryResponse struct {
	ID                 string              `json:"id"`
	OccurredAt         time.Time           `json:"occurred_at"`
	Action             string              `json:"action"`
	Outcome            string              `json:"outcome"`
	ActorType          string              `json:"actor_type"`
	ActorID            string              `json:"actor_id,omitempty"`
	TargetType         string              `json:"target_type,omitempty"`
	TargetID           string              `json:"target_id,omitempty"`
	RequestID          string              `json:"request_id,omitempty"`
	ClientIP           string              `json:"client_ip,omitempty"`
	RequiredPermission string              `json:"required_permission,omitempty"`
	Details            *adminaudit.Details `json:"details,omitempty"`
}

// adminAuditListResponse is the wire format for GET /v1/platform/admin-audit.
type adminAuditListResponse struct {
	Entries []adminAuditEntryResponse `json:"entries"`
}

// handleListAdminAudit serves GET /v1/platform/admin-audit.
// Supports query parameters: action, outcome, actor_id, target_type,
// target_id, limit. See Issue #41.
func (s *Server) handleListAdminAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.adminAudit == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "admin-audit not configured",
		})
		return
	}

	q := r.URL.Query()

	limitStr := strings.TrimSpace(q.Get("limit"))
	limit := 0
	if limitStr != "" {
		parsed, err := parsePositiveInt(limitStr)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit must be a positive integer",
			})
			return
		}
		if parsed > adminaudit.MaxListLimit {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit exceeds maximum allowed value",
			})
			return
		}
		limit = parsed
	}

	f := adminaudit.ListFilter{
		Action:     adminaudit.Action(strings.TrimSpace(q.Get("action"))),
		Outcome:    adminaudit.Outcome(strings.TrimSpace(q.Get("outcome"))),
		ActorID:    strings.TrimSpace(q.Get("actor_id")),
		TargetType: strings.TrimSpace(q.Get("target_type")),
		TargetID:   strings.TrimSpace(q.Get("target_id")),
		Limit:      limit,
	}

	records, err := s.adminAudit.List(r.Context(), f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list admin-audit records",
		})
		return
	}

	resp := adminAuditListResponse{
		Entries: make([]adminAuditEntryResponse, 0, len(records)),
	}
	for _, rec := range records {
		resp.Entries = append(resp.Entries, toAdminAuditEntryResponse(rec))
	}
	writeJSON(w, http.StatusOK, resp)
}

func toAdminAuditEntryResponse(rec *adminaudit.AdminAuditRecord) adminAuditEntryResponse {
	return adminAuditEntryResponse{
		ID:                 rec.ID,
		OccurredAt:         rec.OccurredAt,
		Action:             string(rec.Action),
		Outcome:            string(rec.Outcome),
		ActorType:          string(rec.ActorType),
		ActorID:            rec.ActorID,
		TargetType:         rec.TargetType,
		TargetID:           rec.TargetID,
		RequestID:          rec.RequestID,
		ClientIP:           rec.ClientIP,
		RequiredPermission: rec.RequiredPermission,
		Details:            rec.Details,
	}
}

func toControlAuditEntryResponse(rec *controlaudit.ControlAuditRecord) controlAuditEntryResponse {
	e := controlAuditEntryResponse{
		ID:              rec.ID,
		OccurredAt:      rec.OccurredAt,
		Actor:           rec.Actor,
		Action:          string(rec.Action),
		ResourceKind:    rec.ResourceKind,
		ResourceID:      rec.ResourceID,
		ResourceVersion: rec.ResourceVersion,
		Summary:         rec.Summary,
	}
	if rec.Metadata != nil {
		m := make(map[string]string)
		if rec.Metadata.SurfaceID != "" {
			m["surface_id"] = rec.Metadata.SurfaceID
		}
		if rec.Metadata.DeprecationReason != "" {
			m["deprecation_reason"] = rec.Metadata.DeprecationReason
		}
		if rec.Metadata.SuccessorSurfaceID != "" {
			m["successor_surface_id"] = rec.Metadata.SuccessorSurfaceID
		}
		if len(m) > 0 {
			e.Metadata = m
		}
	}
	return e
}

// parsePositiveInt parses a decimal string into a non-negative int.
func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Identifier validation
// ---------------------------------------------------------------------------

// isValidIdentifier validates that an identifier is safe for use in URLs and storage.
// Rejects empty strings, excessive length, path traversal characters, and control characters.
func isValidIdentifier(id string) bool {
	if id == "" || len(id) > maxIdentifierLength {
		return false
	}

	for _, r := range id {
		if r == '/' || r == '\\' || r == 0 || r < 32 || r == 127 {
			return false
		}
	}

	return true
}
