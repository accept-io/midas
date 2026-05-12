package httpapi

import (
	"context"
	"errors"
	"net/http"

	contextgraph "github.com/accept-io/midas/internal/graph/context"
)

// contextGraphService is the narrow interface the handler depends on.
// *contextgraph.Service satisfies it. Defined here (not in the
// contextgraph package) so tests can swap a stub double without
// dragging the full package surface.
type contextGraphService interface {
	Project(ctx context.Context, view, id string, depth int) (*contextgraph.Projection, error)
}

// WithContextGraph attaches the Context Graph projection service to
// this Server. Returns the receiver for chaining. Pass nil to disable
// the endpoint, which then returns 501 with "context graph projection
// service not configured".
func (s *Server) WithContextGraph(svc contextGraphService) *Server {
	s.contextGraph = svc
	return s
}

// handleGetContextGraph serves
//
//	GET /v1/graphs/context?view={view}&id={id}&depth={n}
//
// Supported view values: service | ai_system | decision_surface.
// The handler is intentionally thin: parse the query, delegate to the
// projection service, marshal the resulting *contextgraph.Projection
// (which already carries JSON tags matching the wire schema in
// api/openapi/v1.yaml).
//
// Status codes:
//
//	200 OK on success (depth silently clamped to contextgraph.MaxDepth)
//	400 Bad Request when view/id/depth fail validation
//	404 Not Found when the root entity does not exist
//	405 Method Not Allowed for non-GET methods
//	500 Internal Server Error on read service failure
//	501 Not Implemented when the projection service is not configured
//
// Auth is enforced upstream at route registration via requireAuth +
// requireRole(viewer | operator | admin) — the same gate as every
// other /v1/* read endpoint.
//
// This endpoint replaces the prior /v1/authority-graph and
// /v1/businessservices/{id}/governance-map endpoints (removed in
// D31d). Future siblings /v1/graphs/authority and /v1/graphs/knowledge
// are reserved (see api/openapi/v1.yaml for documentation).
func (s *Server) handleGetContextGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.contextGraph == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "context graph projection service not configured",
		})
		return
	}

	q := r.URL.Query()
	view := q.Get("view")
	id := q.Get("id")
	depth, err := contextgraph.ParseDepth(q.Get("depth"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	p, err := s.contextGraph.Project(r.Context(), view, id, depth)
	if err != nil {
		switch {
		case errors.Is(err, contextgraph.ErrInvalidView),
			errors.Is(err, contextgraph.ErrInvalidID),
			errors.Is(err, contextgraph.ErrInvalidDepth):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, contextgraph.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}
