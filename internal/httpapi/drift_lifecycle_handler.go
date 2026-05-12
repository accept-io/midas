package httpapi

// drift_lifecycle_handler.go — Drift-1e lifecycle / approval HTTP
// handlers for DriftDefinition. Mirrors handleFailModePolicyActions
// (failmode_approval_handler.go) in shape:
//
//   - POST /v1/controlplane/drift_definitions/{id}/submit
//   - POST /v1/controlplane/drift_definitions/{id}/approve
//   - POST /v1/controlplane/drift_definitions/{id}/reject
//   - POST /v1/controlplane/drift_definitions/{id}/deprecate
//   - POST /v1/controlplane/drift_definitions/{id}/retire
//
// Path is single-{id}; version is in the request body. Each action
// has its own permission gate (PermDriftDefinitionSubmit etc.).
// Approver / submitter / etc. identity follows the actorFromContext
// pattern with a body-supplied fallback.
//
// Response on every success: the full Drift-1d DriftDefinition DTO.
//
// Cross-version atomic deprecation (auto-deprecate prior active when
// activating a new revision) is NOT performed — Drift-1e mirrors
// Profile / FailModePolicy / GovernanceExpectation, which all defer
// that behaviour. Operators must explicitly deprecate prior active
// revisions through the deprecate endpoint.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/accept-io/midas/internal/authz"
)

type driftSubmitRequest struct {
	Version     int    `json:"version"`
	SubmittedBy string `json:"submitted_by,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type driftApproveRequest struct {
	Version    int    `json:"version"`
	ApprovedBy string `json:"approved_by,omitempty"`
}

type driftRejectRequest struct {
	Version    int    `json:"version"`
	RejectedBy string `json:"rejected_by,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type driftDeprecateRequest struct {
	Version               int    `json:"version"`
	DeprecatedBy          string `json:"deprecated_by,omitempty"`
	Reason                string `json:"reason,omitempty"`
	SuccessorDefinitionID string `json:"successor_definition_id,omitempty"`
	SuccessorVersion      int    `json:"successor_version,omitempty"`
}

type driftRetireRequest struct {
	Version   int    `json:"version"`
	RetiredBy string `json:"retired_by,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// handleDriftDefinitionActions dispatches POST
// /v1/controlplane/drift_definitions/{id}/{action}. The dispatcher
// itself is unauthenticated — auth is enforced by the surrounding
// requireAuth wrapper at the route registration point. Per-action
// permission gating is applied inside this dispatcher.
func (s *Server) handleDriftDefinitionActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	const prefix = "/v1/controlplane/drift_definitions/"
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

	definitionID := strings.TrimSpace(parts[0])
	if definitionID == "" || !isValidIdentifier(definitionID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid drift definition id"})
		return
	}

	action := parts[1]
	switch action {
	case "submit":
		s.requirePermission(authz.PermDriftDefinitionSubmit)(func(w http.ResponseWriter, r *http.Request) {
			s.handleSubmitDriftDefinition(w, r, definitionID)
		})(w, r)
	case "approve":
		s.requirePermission(authz.PermDriftDefinitionApprove)(func(w http.ResponseWriter, r *http.Request) {
			s.handleApproveDriftDefinition(w, r, definitionID)
		})(w, r)
	case "reject":
		s.requirePermission(authz.PermDriftDefinitionReject)(func(w http.ResponseWriter, r *http.Request) {
			s.handleRejectDriftDefinition(w, r, definitionID)
		})(w, r)
	case "deprecate":
		s.requirePermission(authz.PermDriftDefinitionDeprecate)(func(w http.ResponseWriter, r *http.Request) {
			s.handleDeprecateDriftDefinition(w, r, definitionID)
		})(w, r)
	case "retire":
		s.requirePermission(authz.PermDriftDefinitionRetire)(func(w http.ResponseWriter, r *http.Request) {
			s.handleRetireDriftDefinition(w, r, definitionID)
		})(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) handleSubmitDriftDefinition(w http.ResponseWriter, r *http.Request, definitionID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "approval service not configured"})
		return
	}
	body, err := readDriftLifecycleBody(w, r)
	if err != nil {
		return
	}
	var req driftSubmitRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, ok := resolveDriftActor(w, r, req.SubmittedBy)
	if !ok {
		return
	}
	if req.Version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be >= 1"})
		return
	}
	updated, err := s.approval.SubmitDriftDefinition(r.Context(), definitionID, req.Version, actor, strings.TrimSpace(req.Reason))
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}
	writeJSON(w, http.StatusOK, toDriftDefinitionResponse(updated))
}

func (s *Server) handleApproveDriftDefinition(w http.ResponseWriter, r *http.Request, definitionID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "approval service not configured"})
		return
	}
	body, err := readDriftLifecycleBody(w, r)
	if err != nil {
		return
	}
	var req driftApproveRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, ok := resolveDriftActor(w, r, req.ApprovedBy)
	if !ok {
		return
	}
	if req.Version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be >= 1"})
		return
	}
	updated, err := s.approval.ApproveDriftDefinition(r.Context(), definitionID, req.Version, actor)
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}
	writeJSON(w, http.StatusOK, toDriftDefinitionResponse(updated))
}

func (s *Server) handleRejectDriftDefinition(w http.ResponseWriter, r *http.Request, definitionID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "approval service not configured"})
		return
	}
	body, err := readDriftLifecycleBody(w, r)
	if err != nil {
		return
	}
	var req driftRejectRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, ok := resolveDriftActor(w, r, req.RejectedBy)
	if !ok {
		return
	}
	if req.Version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be >= 1"})
		return
	}
	updated, err := s.approval.RejectDriftDefinition(r.Context(), definitionID, req.Version, actor, strings.TrimSpace(req.Reason))
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}
	writeJSON(w, http.StatusOK, toDriftDefinitionResponse(updated))
}

func (s *Server) handleDeprecateDriftDefinition(w http.ResponseWriter, r *http.Request, definitionID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "approval service not configured"})
		return
	}
	body, err := readDriftLifecycleBody(w, r)
	if err != nil {
		return
	}
	var req driftDeprecateRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, ok := resolveDriftActor(w, r, req.DeprecatedBy)
	if !ok {
		return
	}
	if req.Version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be >= 1"})
		return
	}
	if req.SuccessorVersion < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "successor_version must be >= 0"})
		return
	}
	if id := strings.TrimSpace(req.SuccessorDefinitionID); id != "" && !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "successor_definition_id must be a valid identifier"})
		return
	}
	updated, err := s.approval.DeprecateDriftDefinition(
		r.Context(),
		definitionID,
		req.Version,
		actor,
		strings.TrimSpace(req.Reason),
		strings.TrimSpace(req.SuccessorDefinitionID),
		req.SuccessorVersion,
	)
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}
	writeJSON(w, http.StatusOK, toDriftDefinitionResponse(updated))
}

func (s *Server) handleRetireDriftDefinition(w http.ResponseWriter, r *http.Request, definitionID string) {
	if s.approval == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "approval service not configured"})
		return
	}
	body, err := readDriftLifecycleBody(w, r)
	if err != nil {
		return
	}
	var req driftRetireRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, ok := resolveDriftActor(w, r, req.RetiredBy)
	if !ok {
		return
	}
	if req.Version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version must be >= 1"})
		return
	}
	updated, err := s.approval.RetireDriftDefinition(r.Context(), definitionID, req.Version, actor, strings.TrimSpace(req.Reason))
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}
	writeJSON(w, http.StatusOK, toDriftDefinitionResponse(updated))
}

// readDriftLifecycleBody reads + size-bounds the request body, writing
// a 400 / 413 response on read failures and returning a sentinel
// error so the caller short-circuits.
func readDriftLifecycleBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	body, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return nil, err
	}
	return body, nil
}

// resolveDriftActor returns the authenticated actor (with body
// fallback) or writes 400 and returns ok=false when invalid.
func resolveDriftActor(w http.ResponseWriter, r *http.Request, bodyFallback string) (string, bool) {
	actor := actorFromContext(r.Context(), strings.TrimSpace(bodyFallback))
	if !isValidIdentifier(actor) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "actor must be a valid identifier"})
		return "", false
	}
	return actor, true
}
