package httpapi

// failmode_policy_handler.go — D29d Part A. Read-only HTTP API for
// FailModePolicy. Backs three GET routes:
//
//   GET /v1/fail_mode_policies/{id}
//   GET /v1/fail_mode_policies/{id}/versions
//   GET /v1/fail_mode_policies/{id}/versions/{version}
//
// Runtime-inert: handlers read from a FailModePolicyReader only and
// never reach any mutating method on the underlying repository. The
// lifecycle transitions live in failmode_approval_handler.go behind
// /v1/controlplane/fail_mode_policies/. Source-pin tests in
// failmode_policy_handler_test.go assert the absence of write/lifecycle
// substrings in this file.

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/accept-io/midas/internal/failmode"
)

// ---------------------------------------------------------------------------
// Reader interface — narrow read-only subset of failmode.PolicyRepository.
// ---------------------------------------------------------------------------

// FailModePolicyReader is the read-side subset of
// failmode.PolicyRepository required to back the D29d read endpoints.
// Implementations must not surface write methods through this type.
type FailModePolicyReader interface {
	FindByID(ctx context.Context, id string) (*failmode.FailModePolicy, error)
	FindByIDAndVersion(ctx context.Context, id string, version int) (*failmode.FailModePolicy, error)
	ListVersions(ctx context.Context, id string) ([]*failmode.FailModePolicy, error)
}

// failModePolicyReadService is the Server-internal contract consulted
// by handleFailModePoliciesPrefix. Owned by this package so handlers
// can be tested with a stub.
type failModePolicyReadService interface {
	HasPolicies() bool
	GetPolicy(ctx context.Context, id string) (*failmode.FailModePolicy, error)
	GetPolicyVersion(ctx context.Context, id string, version int) (*failmode.FailModePolicy, error)
	ListPolicyVersions(ctx context.Context, id string) ([]*failmode.FailModePolicy, error)
}

// FailModePolicyReadService satisfies failModePolicyReadService by
// delegating to the supplied reader. A nil reader leaves HasPolicies
// returning false; every route then responds 501 Not Implemented.
type FailModePolicyReadService struct {
	policies FailModePolicyReader
}

// NewFailModePolicyReadService constructs a read service with the
// supplied reader. A nil reader is supported — HasPolicies() returns
// false and the routes return 501.
func NewFailModePolicyReadService(policies FailModePolicyReader) *FailModePolicyReadService {
	return &FailModePolicyReadService{policies: policies}
}

func (s *FailModePolicyReadService) HasPolicies() bool {
	return s != nil && s.policies != nil
}

func (s *FailModePolicyReadService) GetPolicy(ctx context.Context, id string) (*failmode.FailModePolicy, error) {
	return s.policies.FindByID(ctx, id)
}

func (s *FailModePolicyReadService) GetPolicyVersion(ctx context.Context, id string, version int) (*failmode.FailModePolicy, error) {
	return s.policies.FindByIDAndVersion(ctx, id, version)
}

func (s *FailModePolicyReadService) ListPolicyVersions(ctx context.Context, id string) ([]*failmode.FailModePolicy, error) {
	return s.policies.ListVersions(ctx, id)
}

var _ failModePolicyReadService = (*FailModePolicyReadService)(nil)

// ---------------------------------------------------------------------------
// Prefix dispatcher.
// ---------------------------------------------------------------------------

// handleFailModePoliciesPrefix dispatches:
//
//	GET /v1/fail_mode_policies/{id}
//	GET /v1/fail_mode_policies/{id}/versions
//	GET /v1/fail_mode_policies/{id}/versions/{version}
//
// Non-GET methods receive 405. Unknown paths receive 404. The /v1
// prefix here does not collide with /v1/controlplane/fail_mode_policies/
// because ServeMux selects the longest matching pattern.
func (s *Server) handleFailModePoliciesPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/fail_mode_policies/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	parts := strings.Split(tail, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	switch {
	case len(parts) == 1:
		s.handleGetFailModePolicy(w, r, id)
	case len(parts) == 2 && parts[1] == "versions":
		s.handleListFailModePolicyVersions(w, r, id)
	case len(parts) == 3 && parts[1] == "versions":
		s.handleGetFailModePolicyVersion(w, r, id, parts[2])
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// ---------------------------------------------------------------------------
// Per-route handlers.
// ---------------------------------------------------------------------------

func (s *Server) handleGetFailModePolicy(w http.ResponseWriter, r *http.Request, id string) {
	if s.failModePolicyRead == nil || !s.failModePolicyRead.HasPolicies() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "fail mode policy reader not configured"})
		return
	}
	p, err := s.failModePolicyRead.GetPolicy(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "fail mode policy not found"})
		return
	}
	writeJSON(w, http.StatusOK, toFailModePolicyResponse(p))
}

func (s *Server) handleListFailModePolicyVersions(w http.ResponseWriter, r *http.Request, id string) {
	if s.failModePolicyRead == nil || !s.failModePolicyRead.HasPolicies() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "fail mode policy reader not configured"})
		return
	}
	versions, err := s.failModePolicyRead.ListPolicyVersions(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(versions) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "fail mode policy not found"})
		return
	}
	writeJSON(w, http.StatusOK, toFailModePolicyListResponse(versions))
}

func (s *Server) handleGetFailModePolicyVersion(w http.ResponseWriter, r *http.Request, id, versionStr string) {
	if s.failModePolicyRead == nil || !s.failModePolicyRead.HasPolicies() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "fail mode policy reader not configured"})
		return
	}
	v, err := strconv.Atoi(versionStr)
	if err != nil || v < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version path segment must be a positive integer"})
		return
	}
	p, err := s.failModePolicyRead.GetPolicyVersion(r.Context(), id, v)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "fail mode policy version not found"})
		return
	}
	writeJSON(w, http.StatusOK, toFailModePolicyResponse(p))
}
