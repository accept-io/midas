package httpapi

// authority_graph_handler_test.go — HTTP-layer tests for
// GET /v1/authority-graph (Phase 1).
//
// The handler is intentionally thin (parse query → delegate to the
// projection service → marshal). These tests pin: validation status
// codes (400 / 404 / 405), the unwired-service 501, the auth gate
// (mirrors all other /v1/* read endpoints), and one happy-path body
// shape sanity check. Detailed projection semantics (node/edge kinds,
// directionality, sorting) are pinned in
// internal/authoritygraph/service_test.go and are not duplicated here.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/authoritygraph"
	"github.com/accept-io/midas/internal/config"
)

// stubAuthorityGraph satisfies authorityGraphService for handler tests.
// Either the canned projection or the canned err is returned; canned
// err takes precedence when set so we can drive 400/404/500 paths
// independently of input parsing.
type stubAuthorityGraph struct {
	projection *authoritygraph.Projection
	err        error
	gotView    string
	gotID      string
	gotDepth   int
}

func (s *stubAuthorityGraph) Project(_ context.Context, view, id string, depth int) (*authoritygraph.Projection, error) {
	s.gotView, s.gotID, s.gotDepth = view, id, depth
	if s.err != nil {
		return nil, s.err
	}
	return s.projection, nil
}

// withAuthorityGraph builds a Server with the stub installed. AuthMode
// is left at the default (open) for most tests; the auth-gate test
// switches to required separately.
func withAuthorityGraph(t *testing.T, stub *stubAuthorityGraph) *Server {
	t.Helper()
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithAuthorityGraph(stub)
	return srv
}

// makeOKProjection returns a minimal but realistic Phase 1 projection
// for happy-path body assertions.
func makeOKProjection() *authoritygraph.Projection {
	return &authoritygraph.Projection{
		Root:  authoritygraph.NodeRef{Kind: authoritygraph.NodeKindBusinessService, ID: "bs-1"},
		View:  authoritygraph.ViewService,
		Depth: authoritygraph.DefaultDepth,
		Nodes: []authoritygraph.Node{
			{Kind: authoritygraph.NodeKindBusinessService, ID: "bs-1", Label: "BS One"},
		},
		Edges: []authoritygraph.Edge{},
	}
}

// ---------------------------------------------------------------------------
// Happy path
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_HappyPath_200WithProjection(t *testing.T) {
	stub := &stubAuthorityGraph{projection: makeOKProjection()}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1&depth=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Confirm parameters threaded through to Project().
	if stub.gotView != authoritygraph.ViewService {
		t.Errorf("view: want %q, got %q", authoritygraph.ViewService, stub.gotView)
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
	// Forbidden Phase 1 wire keys.
	for _, illegal := range []string{`"attrs"`, `"truncated"`} {
		if strings.Contains(body, illegal) {
			t.Errorf("response must not contain %q: %s", illegal, body)
		}
	}
}

// TestHandleGetAuthorityGraph_DepthDefaultsTo3 pins that an absent
// depth query parameter is parsed as authoritygraph.DefaultDepth (3)
// at the handler layer (via authoritygraph.ParseDepth).
func TestHandleGetAuthorityGraph_DepthDefaultsTo3(t *testing.T) {
	stub := &stubAuthorityGraph{projection: makeOKProjection()}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotDepth != authoritygraph.DefaultDepth {
		t.Errorf("default depth: want %d, got %d", authoritygraph.DefaultDepth, stub.gotDepth)
	}
}

// TestHandleGetAuthorityGraph_DepthClamping pins that depth > MaxDepth
// is silently clamped to authoritygraph.MaxDepth (5) without changing
// the success status.
func TestHandleGetAuthorityGraph_DepthClamping(t *testing.T) {
	stub := &stubAuthorityGraph{projection: makeOKProjection()}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1&depth=999", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotDepth != authoritygraph.MaxDepth {
		t.Errorf("clamped depth: want %d, got %d", authoritygraph.MaxDepth, stub.gotDepth)
	}
}

// ---------------------------------------------------------------------------
// 400 paths — query validation and service-side validation
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_MissingView_400(t *testing.T) {
	stub := &stubAuthorityGraph{err: errors.New("unreachable: handler must reject before delegating")}
	// Wait — the handler doesn't pre-validate view; it delegates to
	// Project, which returns ErrInvalidView. So we plumb that error
	// through.
	stub.err = authoritygraph.ErrInvalidView
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?id=bs-1&depth=1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing view: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetAuthorityGraph_UnsupportedView_400(t *testing.T) {
	stub := &stubAuthorityGraph{err: authoritygraph.ErrInvalidView}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=agent&id=ai-1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unsupported view: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetAuthorityGraph_MissingID_400(t *testing.T) {
	stub := &stubAuthorityGraph{err: authoritygraph.ErrInvalidID}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing id: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetAuthorityGraph_NegativeDepth_400(t *testing.T) {
	srv := withAuthorityGraph(t, &stubAuthorityGraph{projection: makeOKProjection()})

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1&depth=-1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("negative depth: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetAuthorityGraph_NonNumericDepth_400(t *testing.T) {
	srv := withAuthorityGraph(t, &stubAuthorityGraph{projection: makeOKProjection()})

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1&depth=abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric depth: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 404 path
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_NotFoundID_404(t *testing.T) {
	stub := &stubAuthorityGraph{err: authoritygraph.ErrNotFound}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("not found: want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 405 path — non-GET methods
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_NonGET_405(t *testing.T) {
	srv := withAuthorityGraph(t, &stubAuthorityGraph{projection: makeOKProjection()})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, method, "/v1/authority-graph?view=service&id=bs-1", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d: %s", method, rec.Code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// 501 path — service not configured (mirror of the gmap 501)
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_NotImplemented_WhenServiceUnconfigured(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil) // no WithAuthorityGraph

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Auth gate — mirrors the explorer/auth_chain pattern
// ---------------------------------------------------------------------------

// TestHandleGetAuthorityGraph_Unauthenticated_RequiredMode_401 confirms
// the route is wired through requireAuth: a request without
// credentials under AuthModeRequired returns 401, identical to every
// other /v1/* read endpoint.
func TestHandleGetAuthorityGraph_Unauthenticated_RequiredMode_401(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil).
		WithAuthorityGraph(&stubAuthorityGraph{projection: makeOKProjection()}).
		WithAuthMode(config.AuthModeRequired)
	// No authenticator wired → the requireAuth fail-closed branch fires.

	req := httptest.NewRequest(http.MethodGet, "/v1/authority-graph?view=service&id=bs-1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 unauthenticated, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Internal error path — generic 500
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_InternalError_500(t *testing.T) {
	stub := &stubAuthorityGraph{err: errors.New("boom: repo unavailable")}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("internal: want 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Sanity: response decodes back into the typed projection
// ---------------------------------------------------------------------------

// TestHandleGetAuthorityGraph_ResponseDecodes pins that the response
// body decodes into authoritygraph.Projection without surprises (i.e.
// the JSON tags on the wire match the Go types — no orphan fields).
func TestHandleGetAuthorityGraph_ResponseDecodes(t *testing.T) {
	stub := &stubAuthorityGraph{projection: makeOKProjection()}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got authoritygraph.Projection
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Root.Kind != authoritygraph.NodeKindBusinessService || got.Root.ID != "bs-1" {
		t.Errorf("root: %+v", got.Root)
	}
	if got.View != authoritygraph.ViewService {
		t.Errorf("view: %q", got.View)
	}
	if got.Depth != authoritygraph.DefaultDepth {
		t.Errorf("depth: %d", got.Depth)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].Kind != authoritygraph.NodeKindBusinessService {
		t.Errorf("nodes: %+v", got.Nodes)
	}
}
