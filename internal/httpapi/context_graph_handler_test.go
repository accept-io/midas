package httpapi

// context_graph_handler_test.go — HTTP-layer tests for
// GET /v1/graphs/context (D31d).
//
// The handler is intentionally thin (parse query → delegate to the
// projection service → marshal). These tests pin: validation status
// codes (400 / 404 / 405), the unwired-service 501, the auth gate
// (mirrors all other /v1/* read endpoints), and one happy-path body
// shape sanity check. Detailed projection semantics (node/edge kinds,
// directionality, sorting) are pinned in
// internal/graph/context/service_test.go and are not duplicated here.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/config"
	contextgraph "github.com/accept-io/midas/internal/graph/context"
)

// stubContextGraph satisfies contextGraphService for handler tests.
// Either the canned projection or the canned err is returned; canned
// err takes precedence when set so we can drive 400/404/500 paths
// independently of input parsing.
type stubContextGraph struct {
	projection *contextgraph.Projection
	err        error
	gotView    string
	gotID      string
	gotDepth   int
}

func (s *stubContextGraph) Project(_ context.Context, view, id string, depth int) (*contextgraph.Projection, error) {
	s.gotView, s.gotID, s.gotDepth = view, id, depth
	if s.err != nil {
		return nil, s.err
	}
	return s.projection, nil
}

// withContextGraph builds a Server with the stub installed. AuthMode
// is left at the default (open) for most tests; the auth-gate test
// switches to required separately.
func withContextGraph(t *testing.T, stub *stubContextGraph) *Server {
	t.Helper()
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithContextGraph(stub)
	return srv
}

// makeOKProjection returns a minimal but realistic projection for
// happy-path body assertions.
func makeOKProjection() *contextgraph.Projection {
	return &contextgraph.Projection{
		Root:  contextgraph.NodeRef{Kind: contextgraph.NodeKindBusinessService, ID: "bs-1"},
		View:  contextgraph.ViewService,
		Depth: contextgraph.DefaultDepth,
		Nodes: []contextgraph.Node{
			{Kind: contextgraph.NodeKindBusinessService, ID: "bs-1", Label: "BS One"},
		},
		Edges: []contextgraph.Edge{},
	}
}

// ---------------------------------------------------------------------------
// Happy path
// ---------------------------------------------------------------------------

func TestHandleGetContextGraph_HappyPath_200WithProjection(t *testing.T) {
	stub := &stubContextGraph{projection: makeOKProjection()}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1&depth=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Confirm parameters threaded through to Project().
	if stub.gotView != contextgraph.ViewService {
		t.Errorf("view: want %q, got %q", contextgraph.ViewService, stub.gotView)
	}
	if stub.gotID != "bs-1" {
		t.Errorf("id: want bs-1, got %q", stub.gotID)
	}
	if stub.gotDepth != 2 {
		t.Errorf("depth: want 2, got %d", stub.gotDepth)
	}
	// Body has the projection wire-shape keys.
	body := rec.Body.String()
	for _, want := range []string{`"root":`, `"view":`, `"depth":`, `"nodes":`, `"edges":`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
	// Forbidden wire keys (no generic attrs / truncated signal).
	for _, illegal := range []string{`"attrs"`, `"truncated"`} {
		if strings.Contains(body, illegal) {
			t.Errorf("response must not contain %q: %s", illegal, body)
		}
	}
}

// TestHandleGetContextGraph_DepthDefaultsTo3 pins that an absent depth
// query parameter is parsed as contextgraph.DefaultDepth (3) at the
// handler layer (via contextgraph.ParseDepth).
func TestHandleGetContextGraph_DepthDefaultsTo3(t *testing.T) {
	stub := &stubContextGraph{projection: makeOKProjection()}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotDepth != contextgraph.DefaultDepth {
		t.Errorf("default depth: want %d, got %d", contextgraph.DefaultDepth, stub.gotDepth)
	}
}

// TestHandleGetContextGraph_DepthClamping pins that depth > MaxDepth
// is silently clamped to contextgraph.MaxDepth (5) without changing
// the success status.
func TestHandleGetContextGraph_DepthClamping(t *testing.T) {
	stub := &stubContextGraph{projection: makeOKProjection()}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1&depth=999", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotDepth != contextgraph.MaxDepth {
		t.Errorf("clamped depth: want %d, got %d", contextgraph.MaxDepth, stub.gotDepth)
	}
}

// ---------------------------------------------------------------------------
// 400 paths — query validation and service-side validation
// ---------------------------------------------------------------------------

func TestHandleGetContextGraph_MissingView_400(t *testing.T) {
	stub := &stubContextGraph{err: contextgraph.ErrInvalidView}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?id=bs-1&depth=1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing view: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetContextGraph_UnsupportedView_400(t *testing.T) {
	stub := &stubContextGraph{err: contextgraph.ErrInvalidView}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=agent&id=ai-1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unsupported view: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetContextGraph_MissingID_400(t *testing.T) {
	stub := &stubContextGraph{err: contextgraph.ErrInvalidID}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing id: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetContextGraph_NegativeDepth_400(t *testing.T) {
	srv := withContextGraph(t, &stubContextGraph{projection: makeOKProjection()})

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1&depth=-1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("negative depth: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetContextGraph_NonNumericDepth_400(t *testing.T) {
	srv := withContextGraph(t, &stubContextGraph{projection: makeOKProjection()})

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1&depth=abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric depth: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 404 path
// ---------------------------------------------------------------------------

func TestHandleGetContextGraph_NotFoundID_404(t *testing.T) {
	stub := &stubContextGraph{err: contextgraph.ErrNotFound}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("not found: want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 405 path — non-GET methods
// ---------------------------------------------------------------------------

func TestHandleGetContextGraph_NonGET_405(t *testing.T) {
	srv := withContextGraph(t, &stubContextGraph{projection: makeOKProjection()})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, method, "/v1/graphs/context?view=service&id=bs-1", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d: %s", method, rec.Code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// 501 path — service not configured
// ---------------------------------------------------------------------------

func TestHandleGetContextGraph_NotImplemented_WhenServiceUnconfigured(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil) // no WithContextGraph

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Auth gate — mirrors the explorer/auth_chain pattern
// ---------------------------------------------------------------------------

func TestHandleGetContextGraph_Unauthenticated_RequiredMode_401(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil).
		WithContextGraph(&stubContextGraph{projection: makeOKProjection()}).
		WithAuthMode(config.AuthModeRequired)
	// No authenticator wired → the requireAuth fail-closed branch fires.

	req := httptest.NewRequest(http.MethodGet, "/v1/graphs/context?view=service&id=bs-1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 unauthenticated, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Internal error path — generic 500
// ---------------------------------------------------------------------------

func TestHandleGetContextGraph_InternalError_500(t *testing.T) {
	stub := &stubContextGraph{err: errors.New("boom: repo unavailable")}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("internal: want 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Sanity: response decodes back into the typed projection
// ---------------------------------------------------------------------------

func TestHandleGetContextGraph_ResponseDecodes(t *testing.T) {
	stub := &stubContextGraph{projection: makeOKProjection()}
	srv := withContextGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got contextgraph.Projection
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Root.Kind != contextgraph.NodeKindBusinessService || got.Root.ID != "bs-1" {
		t.Errorf("root: %+v", got.Root)
	}
	if got.View != contextgraph.ViewService {
		t.Errorf("view: %q", got.View)
	}
	if got.Depth != contextgraph.DefaultDepth {
		t.Errorf("depth: %d", got.Depth)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].Kind != contextgraph.NodeKindBusinessService {
		t.Errorf("nodes: %+v", got.Nodes)
	}
}

// ---------------------------------------------------------------------------
// D31d: old endpoints removed — taxonomy regression
// ---------------------------------------------------------------------------

// TestLegacyGraphRoutesRemoved confirms the D31d taxonomy realignment
// removed /v1/authority-graph and /v1/businessservices/{id}/governance-map
// as live endpoints. Neither path should reach the Context Graph
// handler. /v1/authority-graph has no handler (404 via mux);
// /v1/businessservices/{id}/governance-map is rejected by the
// businessservices prefix handler's "unknown sub-path" branch.
func TestLegacyGraphRoutesRemoved(t *testing.T) {
	srv := withContextGraph(t, &stubContextGraph{projection: makeOKProjection()})

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1", nil)
	if rec.Code == http.StatusOK {
		t.Errorf("/v1/authority-graph must no longer be a live endpoint; got 200")
	}

	rec = performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/governance-map", nil)
	if rec.Code == http.StatusOK {
		t.Errorf("/v1/businessservices/{id}/governance-map must no longer be a live endpoint; got 200")
	}
}
