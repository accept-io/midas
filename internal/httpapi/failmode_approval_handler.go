package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/accept-io/midas/internal/authz"
)

// FailModePolicy approval/deprecation HTTP handlers (D27j-impl-1c).
//
// Mirrors handleProfileActions / handleApproveProfile / handleDeprecateProfile
// (server.go:888-1028). Routing and behaviour follow the same pattern:
//
//   - POST /v1/controlplane/fail_mode_policies/{id}/approve
//   - POST /v1/controlplane/fail_mode_policies/{id}/deprecate
//
// Permissions: fail_mode_policy:approve and fail_mode_policy:deprecate.
// The approver/deprecator identity follows the actorFromContext pattern.
//
// Deprecation reason note: failmode.FailModePolicy has no DeprecationReason
// or DeprecatedBy fields, so the operator-supplied reason is captured in
// the control-audit record only (Metadata.DeprecationReason). The HTTP
// response returns the policy's status/version/id without echoing the
// reason — same shape as deprecateProfileResponse.

type approveFailModePolicyRequest struct {
	Version    int    `json:"version"`
	ApprovedBy string `json:"approved_by"`
}

type approveFailModePolicyResponse struct {
	PolicyID   string `json:"policy_id"`
	Version    int    `json:"version"`
	Status     string `json:"status"`
	ApprovedBy string `json:"approved_by"`
}

type deprecateFailModePolicyRequest struct {
	Version      int    `json:"version"`
	DeprecatedBy string `json:"deprecated_by"`
	Reason       string `json:"reason"`
}

type deprecateFailModePolicyResponse struct {
	PolicyID string `json:"policy_id"`
	Version  int    `json:"version"`
	Status   string `json:"status"`
}

// handleFailModePolicyActions dispatches POST
// /v1/controlplane/fail_mode_policies/{id}/{action}. Mirrors
// handleProfileActions: permission gate is per-action, the dispatcher
// itself does not enforce a single permission.
func (s *Server) handleFailModePolicyActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	const prefix = "/v1/controlplane/fail_mode_policies/"
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

	policyID := strings.TrimSpace(parts[0])
	if policyID == "" || !isValidIdentifier(policyID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid fail mode policy id"})
		return
	}

	action := parts[1]

	switch action {
	case "approve":
		// fail_mode_policy:approve — granted to governance.approver and platform.admin.
		s.requirePermission(authz.PermFailModePolicyApprove)(func(w http.ResponseWriter, r *http.Request) {
			s.handleApproveFailModePolicy(w, r, policyID)
		})(w, r)
	case "deprecate":
		// fail_mode_policy:deprecate — admin-only today; deliberate lifecycle boundary.
		s.requirePermission(authz.PermFailModePolicyDeprecate)(func(w http.ResponseWriter, r *http.Request) {
			s.handleDeprecateFailModePolicy(w, r, policyID)
		})(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleApproveFailModePolicy processes
// POST /v1/controlplane/fail_mode_policies/{id}/approve. It transitions a
// specific FailModePolicy version from review to active.
func (s *Server) handleApproveFailModePolicy(w http.ResponseWriter, r *http.Request, policyID string) {
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

	var req approveFailModePolicyRequest
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

	updated, err := s.approval.ApproveFailModePolicy(r.Context(), policyID, req.Version, approvedBy)
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, approveFailModePolicyResponse{
		PolicyID:   updated.ID,
		Version:    updated.Version,
		Status:     string(updated.Status),
		ApprovedBy: updated.ApprovedBy,
	})
}

// handleDeprecateFailModePolicy processes
// POST /v1/controlplane/fail_mode_policies/{id}/deprecate. It transitions
// a specific FailModePolicy version from active to deprecated. The
// operator-supplied reason is recorded in the control-audit event only
// (failmode.FailModePolicy has no DeprecationReason field).
func (s *Server) handleDeprecateFailModePolicy(w http.ResponseWriter, r *http.Request, policyID string) {
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

	var req deprecateFailModePolicyRequest
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

	updated, err := s.approval.DeprecateFailModePolicy(r.Context(), policyID, req.Version, deprecatedBy, strings.TrimSpace(req.Reason))
	if err != nil {
		statusCode, errResp := mapApprovalError(err)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, deprecateFailModePolicyResponse{
		PolicyID: updated.ID,
		Version:  updated.Version,
		Status:   string(updated.Status),
	})
}
