package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/authority"
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
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/inference"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/oidc"
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
	// ListProcessesByCapability returns (nil, false, nil) when capability not found,
	// or (procs, true, nil) including empty slice when found.
	ListProcessesByCapability(ctx context.Context, capabilityID string) ([]*process.Process, bool, error)
	// ListSurfacesByProcess returns (nil, false, nil) when process not found,
	// or (surfs, true, nil) including empty slice when found.
	ListSurfacesByProcess(ctx context.Context, processID string) ([]*surface.DecisionSurface, bool, error)
	// GetBusinessService returns nil, nil when not found or BS reader not configured.
	GetBusinessService(ctx context.Context, id string) (*businessservice.BusinessService, error)
	// ListBusinessServices returns an empty slice when BS reader not configured.
	ListBusinessServices(ctx context.Context) ([]*businessservice.BusinessService, error)
}

// inferenceService is the narrow interface required by the inference wiring in
// handleEvaluateWith. *inference.Service satisfies this interface.
type inferenceService interface {
	EnsureInferredStructure(ctx context.Context, surfaceID string) (inference.InferenceResult, error)
}

// explicitModeValidator is the narrow interface required for explicit-mode
// structural validation in handleEvaluateWith. *ExplicitValidationService satisfies this.
type explicitModeValidator interface {
	GetProcess(ctx context.Context, id string) (*process.Process, error)
	FindLatestSurface(ctx context.Context, id string) (*surface.DecisionSurface, error)
}

// promotionService is the narrow interface required for the promote endpoint.
// *inference.PromoteService satisfies this interface.
type promotionService interface {
	Promote(ctx context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error)
}

// cleanupService is the narrow interface required for the cleanup endpoint.
// *inference.CleanupService satisfies this interface.
type cleanupService interface {
	CleanupInferredEntities(ctx context.Context, cutoff time.Time) (inference.CleanupResult, error)
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
	mux                 *http.ServeMux
	orchestrator        orchestrator
	controlPlane        controlPlaneService
	approval            approvalService
	introspection       introspectionService
	controlAudit        controlAuditService
	grantLifecycle      grantLifecycleService
	structural          structuralService
	authenticator       auth.Authenticator
	authMode            config.AuthMode              // set via WithAuthMode; must be called at startup with cfg.Auth.Mode
	policyMode          string                       // e.g. "noop" — set via WithPolicyMeta at boot
	policyEvaluatorName string                       // human-readable evaluator name for health responses
	readyFn             func(context.Context) error  // nil means always ready (memory mode)
	explorerEnabled      bool                         // set via WithExplorerEnabled; registers /explorer routes when true
	storeBackend         string                       // e.g. "memory" or "postgres" — set via WithStoreBackend at boot
	explorerDemoSeeded   *bool                        // nil = unknown, &true = seeded, &false = not seeded
	seedDemoUser         bool                         // set via WithSeedDemoUser; mirrors cfg.Dev.SeedDemoUser
	explorerOrchestrator orchestrator                 // isolated in-memory orchestrator for POST /explorer
	localIAM             *localiam.Service            // nil when local IAM is disabled
	oidcService          oidcProvider                 // nil when OIDC is disabled
	secureCookiesFlag    bool                         // mirrors LocalIAM.SecureCookies; used by OIDC helper cookies
	inferenceSvc         inferenceService             // nil when inference is not wired
	inferenceEnabled     bool                         // set via WithInference; mirrors cfg.Inference.Enabled
	structuralMode       config.StructuralMode        // set via WithStructuralMode; empty/unset treated as permissive
	explicitValidator    explicitModeValidator        // nil when explicit-mode validation is not wired
	promotionSvc         promotionService             // nil when promotion is not wired
	cleanupSvc           cleanupService               // nil when cleanup is not wired
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
	ID                   string     `json:"id"`
	Version              int        `json:"version"`
	SurfaceID            string     `json:"surface_id"`
	Name                 string     `json:"name"`
	Description          string     `json:"description,omitempty"`
	Status               string     `json:"status"`
	EffectiveDate        time.Time  `json:"effective_date"`
	ConfidenceThreshold  float64    `json:"confidence_threshold"`
	EscalationMode       string     `json:"escalation_mode"`
	FailMode             string     `json:"fail_mode"`
	PolicyReference      string     `json:"policy_reference,omitempty"`
	RequiredContextKeys  []string   `json:"required_context_keys,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ApprovedBy           string     `json:"approved_by,omitempty"`
	ApprovedAt           *time.Time `json:"approved_at,omitempty"`
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
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	CapabilityID string    `json:"capability_id"`
	Description  string    `json:"description,omitempty"`
	Status       string    `json:"status"`
	Owner        string    `json:"owner,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// businessServiceResponse is the wire format for GET /v1/businessservices/{id} and list items.
type businessServiceResponse struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	ServiceType     string    `json:"service_type"`
	RegulatoryScope string    `json:"regulatory_scope,omitempty"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
	ID            string     `json:"id"`
	AgentID       string     `json:"agent_id"`
	ProfileID     string     `json:"profile_id"`
	Status        string     `json:"status"`
	GrantedBy     string     `json:"granted_by"`
	EffectiveFrom time.Time  `json:"effective_from"`
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
		// platform.admin or governance.approver may approve surfaces.
		s.requireRole(identity.RolePlatformAdmin, identity.RoleGovernanceApprover)(func(w http.ResponseWriter, r *http.Request) {
			s.handleApproveSurface(w, r, surfaceID)
		})(w, r)
	case "deprecate":
		// platform.admin only may deprecate surfaces (destructive lifecycle change).
		s.requireRole(identity.RolePlatformAdmin)(func(w http.ResponseWriter, r *http.Request) {
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
		// platform.admin or governance.approver may approve profiles.
		s.requireRole(identity.RolePlatformAdmin, identity.RoleGovernanceApprover)(func(w http.ResponseWriter, r *http.Request) {
			s.handleApproveProfile(w, r, profileID)
		})(w, r)
	case "deprecate":
		// platform.admin only may deprecate profiles (destructive lifecycle change).
		s.requireRole(identity.RolePlatformAdmin)(func(w http.ResponseWriter, r *http.Request) {
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

	// All grant lifecycle actions (suspend, revoke, reinstate) are platform.admin only —
	// these are destructive/irreversible operational changes.
	switch action {
	case "suspend":
		s.requireRole(identity.RolePlatformAdmin)(func(w http.ResponseWriter, r *http.Request) {
			s.handleSuspendGrant(w, r, grantID)
		})(w, r)
	case "revoke":
		s.requireRole(identity.RolePlatformAdmin)(func(w http.ResponseWriter, r *http.Request) {
			s.handleRevokeGrant(w, r, grantID)
		})(w, r)
	case "reinstate":
		s.requireRole(identity.RolePlatformAdmin)(func(w http.ResponseWriter, r *http.Request) {
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

// WithInference attaches an inference service and enables/disables automatic
// structure inference on the governed /v1/evaluate path.
func (s *Server) WithInference(svc inferenceService, enabled bool) *Server {
	s.inferenceSvc = svc
	s.inferenceEnabled = enabled
	return s
}

// WithStructuralMode sets the structural enforcement mode. In permissive mode
// (the default when not called), process_id is optional on /v1/evaluate.
// In enforced mode, process_id is required when inference is not enabled.
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

// WithPromotion attaches a promotion service for the POST /v1/controlplane/promote endpoint.
func (s *Server) WithPromotion(svc promotionService) *Server {
	s.promotionSvc = svc
	return s
}

// WithCleanup attaches a cleanup service for the POST /v1/controlplane/cleanup endpoint.
func (s *Server) WithCleanup(svc cleanupService) *Server {
	s.cleanupSvc = svc
	return s
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

	// Control plane — governed endpoints.
	// apply/plan: platform.admin only (mutates or previews configuration).
	s.mux.HandleFunc("/v1/controlplane/apply", s.requireAuth(s.requireRole(identity.RolePlatformAdmin)(s.handleApplyBundle)))
	s.mux.HandleFunc("/v1/controlplane/plan", s.requireAuth(s.requireRole(identity.RolePlatformAdmin)(s.handlePlanBundle)))
	s.mux.HandleFunc("/v1/controlplane/promote", s.requireAuth(s.requireRole(identity.RolePlatformAdmin)(s.handlePromote)))
	s.mux.HandleFunc("/v1/controlplane/cleanup", s.requireAuth(s.requireRole(identity.RolePlatformAdmin)(s.handleCleanup)))
	// audit read: platform.viewer or above.
	s.mux.HandleFunc("/v1/controlplane/audit", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListControlAudit)))
	// Resource lifecycle — role enforcement applied per-action inside each dispatcher.
	s.mux.HandleFunc("/v1/controlplane/surfaces/", s.requireAuth(s.handleSurfaceActions))
	s.mux.HandleFunc("/v1/controlplane/profiles/", s.requireAuth(s.handleProfileActions))
	s.mux.HandleFunc("/v1/controlplane/grants/", s.requireAuth(s.handleGrantActions))

	// Operator introspection — platform.viewer or above.
	s.mux.HandleFunc("/v1/surfaces/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetSurfaceOrVersions)))
	s.mux.HandleFunc("/v1/profiles/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetProfileOrVersions)))
	s.mux.HandleFunc("/v1/profiles", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListProfiles)))
	s.mux.HandleFunc("/v1/agents/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetAgent)))
	s.mux.HandleFunc("/v1/grants/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetGrant)))
	s.mux.HandleFunc("/v1/grants", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListGrants)))

	// Structural entities — platform.viewer or above.
	s.mux.HandleFunc("/v1/capabilities/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetCapabilityOrProcesses)))
	s.mux.HandleFunc("/v1/capabilities", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListCapabilities)))
	s.mux.HandleFunc("/v1/processes/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetProcessOrSurfaces)))
	s.mux.HandleFunc("/v1/processes", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListProcesses)))
	s.mux.HandleFunc("/v1/businessservices/", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleGetBusinessService)))
	s.mux.HandleFunc("/v1/businessservices", s.requireAuth(s.requireRole(identity.RolePlatformViewer, identity.RolePlatformOperator, identity.RolePlatformAdmin)(s.handleListBusinessServices)))
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

// inferenceMeta carries the structural inference result attached to evaluate
// responses when automatic inference was triggered on the /v1/evaluate path.
type inferenceMeta struct {
	CapabilityID      string `json:"capability_id"`
	ProcessID         string `json:"process_id"`
	SurfaceID         string `json:"surface_id"`
	CapabilityCreated bool   `json:"capability_created"`
	ProcessCreated    bool   `json:"process_created"`
	SurfaceCreated    bool   `json:"surface_created"`
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

	// Inference is populated when structural inference was triggered automatically.
	// Omitted when process_id was supplied explicitly or when inference is disabled.
	Inference *inferenceMeta `json:"inference,omitempty"`
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
	// Inference is skipped entirely in explicit mode.
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

	// Inference / permissive-mode routing: only on the governed /v1/evaluate path,
	// only when process_id is absent.
	// When process_id is provided, skip this block entirely (explicit mode, unchanged behaviour).
	var infMeta *inferenceMeta
	if requireRequestID && req.ProcessID == "" {
		if s.inferenceEnabled && s.inferenceSvc != nil {
			// Inference path: infer capability/process/surface from surface_id.
			if err := inference.ValidateSurfaceID(req.SurfaceID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "invalid surface_id: " + err.Error(),
				})
				return
			}
			infResult, err := s.inferenceSvc.EnsureInferredStructure(r.Context(), req.SurfaceID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "inference failed: " + err.Error(),
				})
				return
			}
			infMeta = &inferenceMeta{
				CapabilityID:      infResult.CapabilityID,
				ProcessID:         infResult.ProcessID,
				SurfaceID:         infResult.SurfaceID,
				CapabilityCreated: infResult.CapabilityCreated,
				ProcessCreated:    infResult.ProcessCreated,
				SurfaceCreated:    infResult.SurfaceCreated,
			}
		} else if s.structuralMode == config.StructuralModeEnforced {
			// Enforced mode: process_id is required when inference is not available.
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "process_id is required when inference is not enabled",
			})
			return
		}
		// Permissive mode (default): allow evaluation with an absent process_id.
		// The orchestrator handles empty process_id consistently with Explorer behaviour.
	}

	result, err := orch.Evaluate(r.Context(), toEvalRequest(req), json.RawMessage(rawBody))
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEvaluation)
		writeJSON(w, statusCode, errResp)
		return
	}

	if infMeta != nil {
		w.Header().Set("X-MIDAS-Inference-Used", "true")
		w.Header().Set("X-MIDAS-Inferred-Capability", infMeta.CapabilityID)
		w.Header().Set("X-MIDAS-Inferred-Process", infMeta.ProcessID)
	}
	writeJSON(w, http.StatusOK, evaluateResponse{
		Outcome:         string(result.Outcome),
		Reason:          string(result.ReasonCode),
		EnvelopeID:      result.EnvelopeID,
		Explanation:     result.Explanation,
		PolicyMode:      result.PolicyMode,
		PolicyReference: result.PolicyReference,
		PolicySkipped:   result.PolicySkipped,
		Inference:       infMeta,
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

// promoteRequestBody is the JSON body for POST /v1/controlplane/promote.
type promoteRequestBody struct {
	From promoteSpec `json:"from"`
	To   promoteSpec `json:"to"`
}

// promoteSpec identifies a capability/process pair in a promote request or response.
type promoteSpec struct {
	CapabilityID string `json:"capability_id"`
	ProcessID    string `json:"process_id"`
}

// promoteResponseBody is the JSON response for POST /v1/controlplane/promote.
type promoteResponseBody struct {
	From             promoteSpec `json:"from"`
	To               promoteSpec `json:"to"`
	SurfacesMigrated int         `json:"surfaces_migrated"`
}

// handlePromote processes POST /v1/controlplane/promote.
// It promotes an inferred capability/process pair to managed equivalents, migrating
// all attached surfaces to the new process and deprecating the old inferred entities.
func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if s.promotionSvc == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "promotion service not configured",
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

	var req promoteRequestBody
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	promReq := inference.PromoteRequest{
		FromCapabilityID: strings.TrimSpace(req.From.CapabilityID),
		FromProcessID:    strings.TrimSpace(req.From.ProcessID),
		ToCapabilityID:   strings.TrimSpace(req.To.CapabilityID),
		ToProcessID:      strings.TrimSpace(req.To.ProcessID),
	}

	resp, err := s.promotionSvc.Promote(r.Context(), promReq)
	if err != nil {
		var pe inference.PromoteErr
		if errors.As(err, &pe) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "promotion failed"})
		return
	}

	writeJSON(w, http.StatusOK, promoteResponseBody{
		From:             promoteSpec{CapabilityID: resp.FromCapabilityID, ProcessID: resp.FromProcessID},
		To:               promoteSpec{CapabilityID: resp.ToCapabilityID, ProcessID: resp.ToProcessID},
		SurfacesMigrated: resp.SurfacesMigrated,
	})
}

// cleanupRequestBody is the JSON body for POST /v1/controlplane/cleanup.
type cleanupRequestBody struct {
	OlderThanDays int `json:"older_than_days"`
}

// cleanupResponseBody is the JSON body returned by POST /v1/controlplane/cleanup.
type cleanupResponseBody struct {
	ProcessesDeleted    []string `json:"processes_deleted"`
	CapabilitiesDeleted []string `json:"capabilities_deleted"`
}

// handleCleanup processes POST /v1/controlplane/cleanup.
// It deletes deprecated inferred capabilities and processes that are no longer
// referenced, subject to an optional age threshold.
func (s *Server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if s.cleanupSvc == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "cleanup service not configured",
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

	var req cleanupRequestBody
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if req.OlderThanDays < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "older_than_days must be >= 0",
		})
		return
	}

	var cutoff time.Time
	if req.OlderThanDays > 0 {
		cutoff = time.Now().UTC().Add(-time.Duration(req.OlderThanDays) * 24 * time.Hour)
	}

	result, err := s.cleanupSvc.CleanupInferredEntities(r.Context(), cutoff)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cleanup failed"})
		return
	}

	writeJSON(w, http.StatusOK, cleanupResponseBody{
		ProcessesDeleted:    result.ProcessesDeleted,
		CapabilitiesDeleted: result.CapabilitiesDeleted,
	})
}

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
	result, err := s.controlPlane.ApplyBundle(r.Context(), rawBody, actor)
	if err != nil {
		statusCode, errResp := mapApplyError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, result)
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

	plan, err := s.controlPlane.PlanBundle(r.Context(), rawBody)
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
		ID:           p.ID,
		Name:         p.Name,
		CapabilityID: p.CapabilityID,
		Description:  p.Description,
		Status:       p.Status,
		Owner:        p.Owner,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
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
	}
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

// handleGetCapabilityOrProcesses dispatches:
//
//	GET /v1/capabilities/{id}            → handleGetCapability
//	GET /v1/capabilities/{id}/processes  → handleGetCapabilityProcesses
func (s *Server) handleGetCapabilityOrProcesses(w http.ResponseWriter, r *http.Request) {
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
	parts := strings.SplitN(rest, "/", 2)
	id := strings.TrimSpace(parts[0])
	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid capability id"})
		return
	}
	if len(parts) == 2 && parts[1] == "processes" {
		s.handleGetCapabilityProcesses(w, r, id)
		return
	}
	if len(parts) > 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	s.handleGetCapability(w, r, id)
}

func (s *Server) handleGetCapability(w http.ResponseWriter, r *http.Request, id string) {
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

func (s *Server) handleGetCapabilityProcesses(w http.ResponseWriter, r *http.Request, capabilityID string) {
	if s.structural == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "structural service not configured"})
		return
	}
	procs, found, err := s.structural.ListProcessesByCapability(r.Context(), capabilityID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "capability not found"})
		return
	}
	out := make([]processResponse, 0, len(procs))
	for _, p := range procs {
		out = append(out, toProcessResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
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

// handleGetBusinessService serves GET /v1/businessservices/{id}.
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
	id := strings.TrimPrefix(r.URL.Path, prefix)
	id = strings.Trim(id, "/")
	if id == "" {
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
