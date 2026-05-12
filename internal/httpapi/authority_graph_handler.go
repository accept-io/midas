package httpapi

import (
	"context"
	"errors"
	"net/http"

	authoritygraph "github.com/accept-io/midas/internal/graph/authority"
)

// authorityGraphService is the narrow interface the handler depends
// on. *authoritygraph.Service satisfies it. Defined here (not in the
// authoritygraph package) so tests can swap a stub double without
// dragging the full package surface.
type authorityGraphService interface {
	Project(ctx context.Context, view, id string, depth int) (*authoritygraph.Projection, error)
}

// WithAuthorityGraph attaches the Authority Graph projection service
// to this Server. Returns the receiver for chaining. Pass nil to
// disable the endpoint, which then returns 501 with "authority graph
// projection service not configured".
//
// The Authority Graph is a separate projection from the Context
// Graph. Both endpoints can be wired independently — wiring one does
// not enable the other.
func (s *Server) WithAuthorityGraph(svc authorityGraphService) *Server {
	s.authorityGraph = svc
	return s
}

// handleGetAuthorityGraph serves
//
//	GET /v1/graphs/authority?view=service&id={business_service_id}&depth={n}
//
// MVP supports view=service only. The handler is intentionally
// thin: parse the query, delegate to the projection service, marshal
// the resulting *authoritygraph.Projection (which already carries
// JSON tags matching the wire schema in api/openapi/v1.yaml).
//
// Status codes:
//
//	200 OK on success (depth silently clamped to authoritygraph.MaxDepth)
//	400 Bad Request when view/id/depth fail validation
//	404 Not Found when the root entity does not exist
//	405 Method Not Allowed for non-GET methods
//	500 Internal Server Error on read service failure
//	501 Not Implemented when the projection service is not configured
//
// Auth is enforced upstream at route registration via requireAuth +
// requireRole(viewer | operator | admin) — the same gate as every
// other /v1/* read endpoint and as the sibling /v1/graphs/context.
//
// The Authority Graph is distinct from the Context Graph: it
// projects business_service, decision_surface, authority_profile,
// authority_grant, agent, and fail_mode_policy nodes. The Context
// Graph projects a different node set (capabilities, processes,
// AI systems, AI system bindings, plus rollups). The two endpoints
// answer different operator questions and have separate wire
// schemas.
func (s *Server) handleGetAuthorityGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.authorityGraph == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "authority graph projection service not configured",
		})
		return
	}

	q := r.URL.Query()
	view := q.Get("view")
	id := q.Get("id")
	depth, err := authoritygraph.ParseDepth(q.Get("depth"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	p, err := s.authorityGraph.Project(r.Context(), view, id, depth)
	if err != nil {
		switch {
		case errors.Is(err, authoritygraph.ErrInvalidView),
			errors.Is(err, authoritygraph.ErrInvalidID),
			errors.Is(err, authoritygraph.ErrInvalidDepth):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, authoritygraph.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}
