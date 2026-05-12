package httpapi

// escalation_target_handler.go — D31k-impl-1. Read-only HTTP API for
// EscalationTarget. Backs four GET routes:
//
//   GET /v1/escalation_targets
//   GET /v1/escalation_targets/{id}
//   GET /v1/escalation_targets/{id}/versions
//   GET /v1/escalation_targets/{id}/versions/{version}
//
// Runtime-inert: handlers read from an EscalationTargetReader only and
// never reach any mutating method on the underlying repository.
// Mirrors failmode_policy_handler.go in posture: prefix dispatcher,
// 405 on non-GET, 404 on unknown ids, 400 on bad version segments,
// 501 when the read service is unwired.
//
// D31k-impl-1 intentionally does NOT add write or approval endpoints.
// The approval lifecycle (review → active → deprecated) for escalation
// targets is deferred — see the implementation report for rationale.

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/accept-io/midas/internal/escalation"
)

// EscalationTargetReader is the read-side subset of
// escalation.Repository required to back the D31k-impl-1 read
// endpoints. Implementations must not surface write methods through
// this type.
type EscalationTargetReader interface {
	FindByID(ctx context.Context, id string) (*escalation.EscalationTarget, error)
	FindByIDAndVersion(ctx context.Context, id string, version int) (*escalation.EscalationTarget, error)
	List(ctx context.Context) ([]*escalation.EscalationTarget, error)
	ListVersions(ctx context.Context, id string) ([]*escalation.EscalationTarget, error)
}

// escalationTargetReadService is the Server-internal contract
// consulted by handleEscalationTargetsPrefix / handleListEscalationTargets.
// Owned by this package so handlers can be tested with a stub.
type escalationTargetReadService interface {
	HasTargets() bool
	GetTarget(ctx context.Context, id string) (*escalation.EscalationTarget, error)
	GetTargetVersion(ctx context.Context, id string, version int) (*escalation.EscalationTarget, error)
	ListTargets(ctx context.Context) ([]*escalation.EscalationTarget, error)
	ListTargetVersions(ctx context.Context, id string) ([]*escalation.EscalationTarget, error)
}

// EscalationTargetReadService satisfies escalationTargetReadService by
// delegating to the supplied reader. A nil reader leaves HasTargets
// returning false; every route then responds 501 Not Implemented —
// matching the FailModePolicyReadService posture.
type EscalationTargetReadService struct {
	targets EscalationTargetReader
}

// NewEscalationTargetReadService constructs a read service with the
// supplied reader. A nil reader is supported — HasTargets() returns
// false and the routes return 501.
func NewEscalationTargetReadService(targets EscalationTargetReader) *EscalationTargetReadService {
	return &EscalationTargetReadService{targets: targets}
}

func (s *EscalationTargetReadService) HasTargets() bool {
	return s != nil && s.targets != nil
}

func (s *EscalationTargetReadService) GetTarget(ctx context.Context, id string) (*escalation.EscalationTarget, error) {
	return s.targets.FindByID(ctx, id)
}

func (s *EscalationTargetReadService) GetTargetVersion(ctx context.Context, id string, version int) (*escalation.EscalationTarget, error) {
	return s.targets.FindByIDAndVersion(ctx, id, version)
}

func (s *EscalationTargetReadService) ListTargets(ctx context.Context) ([]*escalation.EscalationTarget, error) {
	return s.targets.List(ctx)
}

func (s *EscalationTargetReadService) ListTargetVersions(ctx context.Context, id string) ([]*escalation.EscalationTarget, error) {
	return s.targets.ListVersions(ctx, id)
}

var _ escalationTargetReadService = (*EscalationTargetReadService)(nil)

// ---------------------------------------------------------------------------
// List / prefix dispatchers.
// ---------------------------------------------------------------------------

// handleListEscalationTargetsRoot processes GET /v1/escalation_targets.
// Returns a 200 list response wrapping the latest version per target.
func (s *Server) handleListEscalationTargetsRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.escalationTargetRead == nil || !s.escalationTargetRead.HasTargets() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "escalation target reader not configured"})
		return
	}
	items, err := s.escalationTargetRead.ListTargets(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toEscalationTargetListResponse(items))
}

// handleEscalationTargetsPrefix dispatches:
//
//	GET /v1/escalation_targets/{id}
//	GET /v1/escalation_targets/{id}/versions
//	GET /v1/escalation_targets/{id}/versions/{version}
//
// Non-GET methods receive 405. Unknown paths receive 404.
func (s *Server) handleEscalationTargetsPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/escalation_targets/"
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
		s.handleGetEscalationTarget(w, r, id)
	case len(parts) == 2 && parts[1] == "versions":
		s.handleListEscalationTargetVersions(w, r, id)
	case len(parts) == 3 && parts[1] == "versions":
		s.handleGetEscalationTargetVersion(w, r, id, parts[2])
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// ---------------------------------------------------------------------------
// Per-route handlers.
// ---------------------------------------------------------------------------

func (s *Server) handleGetEscalationTarget(w http.ResponseWriter, r *http.Request, id string) {
	if s.escalationTargetRead == nil || !s.escalationTargetRead.HasTargets() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "escalation target reader not configured"})
		return
	}
	t, err := s.escalationTargetRead.GetTarget(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if t == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "escalation target not found"})
		return
	}
	writeJSON(w, http.StatusOK, toEscalationTargetResponse(t))
}

func (s *Server) handleListEscalationTargetVersions(w http.ResponseWriter, r *http.Request, id string) {
	if s.escalationTargetRead == nil || !s.escalationTargetRead.HasTargets() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "escalation target reader not configured"})
		return
	}
	versions, err := s.escalationTargetRead.ListTargetVersions(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(versions) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "escalation target not found"})
		return
	}
	writeJSON(w, http.StatusOK, toEscalationTargetListResponse(versions))
}

func (s *Server) handleGetEscalationTargetVersion(w http.ResponseWriter, r *http.Request, id, versionStr string) {
	if s.escalationTargetRead == nil || !s.escalationTargetRead.HasTargets() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "escalation target reader not configured"})
		return
	}
	v, err := strconv.Atoi(versionStr)
	if err != nil || v < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version path segment must be a positive integer"})
		return
	}
	t, err := s.escalationTargetRead.GetTargetVersion(r.Context(), id, v)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if t == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "escalation target version not found"})
		return
	}
	writeJSON(w, http.StatusOK, toEscalationTargetResponse(t))
}
