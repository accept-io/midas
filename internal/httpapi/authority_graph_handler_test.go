package httpapi

// authority_graph_handler_test.go — HTTP-layer tests for
// GET /v1/graphs/authority (D31f).
//
// The handler is intentionally thin (parse query → delegate to the
// projection service → marshal). These tests pin: validation status
// codes (400 / 404 / 405), the unwired-service 501, the auth gate
// (mirrors all other /v1/* read endpoints), and a happy-path body
// shape sanity check. Detailed projection semantics (node/edge kinds,
// directionality, sorting) are pinned in
// internal/graph/authority/service_test.go and are not duplicated here.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/config"
	authoritygraph "github.com/accept-io/midas/internal/graph/authority"
)

// stubAuthorityGraph satisfies authorityGraphService for handler
// tests. Either the canned projection or the canned err is returned;
// canned err takes precedence when set so we can drive 400/404/500
// paths independently of input parsing.
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

// withAuthorityGraph builds a Server with the stub installed.
// AuthMode is left at the default (open) for most tests; the
// auth-gate test switches to required separately.
func withAuthorityGraph(t *testing.T, stub *stubAuthorityGraph) *Server {
	t.Helper()
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithAuthorityGraph(stub)
	return srv
}

// makeOKAuthProjection returns a minimal but realistic projection
// for happy-path body assertions.
func makeOKAuthProjection() *authoritygraph.Projection {
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
	stub := &stubAuthorityGraph{projection: makeOKAuthProjection()}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1&depth=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotView != authoritygraph.ViewService {
		t.Errorf("view: want %q, got %q", authoritygraph.ViewService, stub.gotView)
	}
	if stub.gotID != "bs-1" {
		t.Errorf("id: want bs-1, got %q", stub.gotID)
	}
	if stub.gotDepth != 2 {
		t.Errorf("depth: want 2, got %d", stub.gotDepth)
	}
	body := rec.Body.String()
	for _, want := range []string{`"root":`, `"view":`, `"depth":`, `"nodes":`, `"edges":`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
}

func TestHandleGetAuthorityGraph_DepthDefaultsTo4(t *testing.T) {
	stub := &stubAuthorityGraph{projection: makeOKAuthProjection()}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotDepth != authoritygraph.DefaultDepth {
		t.Errorf("default depth: want %d, got %d", authoritygraph.DefaultDepth, stub.gotDepth)
	}
	if authoritygraph.DefaultDepth != 4 {
		t.Errorf("Authority Graph DefaultDepth must be 4 (full chain visibility); got %d", authoritygraph.DefaultDepth)
	}
}

func TestHandleGetAuthorityGraph_DepthClamping(t *testing.T) {
	stub := &stubAuthorityGraph{projection: makeOKAuthProjection()}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1&depth=999", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotDepth != authoritygraph.MaxDepth {
		t.Errorf("clamped depth: want %d, got %d", authoritygraph.MaxDepth, stub.gotDepth)
	}
}

// ---------------------------------------------------------------------------
// 400 paths
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_MissingView_400(t *testing.T) {
	stub := &stubAuthorityGraph{err: authoritygraph.ErrInvalidView}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?id=bs-1&depth=1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing view: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetAuthorityGraph_UnsupportedView_400(t *testing.T) {
	stub := &stubAuthorityGraph{err: authoritygraph.ErrInvalidView}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=agent&id=ai-1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unsupported view: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetAuthorityGraph_MissingID_400(t *testing.T) {
	stub := &stubAuthorityGraph{err: authoritygraph.ErrInvalidID}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing id: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetAuthorityGraph_NegativeDepth_400(t *testing.T) {
	srv := withAuthorityGraph(t, &stubAuthorityGraph{projection: makeOKAuthProjection()})

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1&depth=-1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("negative depth: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetAuthorityGraph_NonNumericDepth_400(t *testing.T) {
	srv := withAuthorityGraph(t, &stubAuthorityGraph{projection: makeOKAuthProjection()})

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1&depth=abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric depth: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 404
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_NotFoundID_404(t *testing.T) {
	stub := &stubAuthorityGraph{err: authoritygraph.ErrNotFound}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("not found: want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 405
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_NonGET_405(t *testing.T) {
	srv := withAuthorityGraph(t, &stubAuthorityGraph{projection: makeOKAuthProjection()})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, method, "/v1/graphs/authority?view=service&id=bs-1", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d: %s", method, rec.Code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// 501
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_NotImplemented_WhenServiceUnconfigured(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil) // no WithAuthorityGraph

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Auth gate
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_Unauthenticated_RequiredMode_401(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil).
		WithAuthorityGraph(&stubAuthorityGraph{projection: makeOKAuthProjection()}).
		WithAuthMode(config.AuthModeRequired)

	req := httptest.NewRequest(http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 unauthenticated, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 500
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_InternalError_500(t *testing.T) {
	stub := &stubAuthorityGraph{err: errors.New("boom: repo unavailable")}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("internal: want 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Sanity: response decodes
// ---------------------------------------------------------------------------

func TestHandleGetAuthorityGraph_ResponseDecodes(t *testing.T) {
	stub := &stubAuthorityGraph{projection: makeOKAuthProjection()}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
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
}

// ---------------------------------------------------------------------------
// Distinctness from Context Graph
// ---------------------------------------------------------------------------

// TestAuthorityGraph_DistinctFromContextGraph confirms the two
// graph endpoints are truly distinct: each has its own server
// field, its own handler, and its own route. Wiring Context Graph
// alone leaves Authority Graph unconfigured (501) and vice versa.
func TestAuthorityGraph_DistinctFromContextGraph(t *testing.T) {
	// Server with ONLY Authority Graph wired. The Context Graph
	// route should report 501 because contextGraph remains nil.
	srv := NewServerFull(nil, nil, nil, nil, nil, nil).
		WithAuthorityGraph(&stubAuthorityGraph{projection: makeOKAuthProjection()})

	recCtx := performRequest(t, srv, http.MethodGet, "/v1/graphs/context?view=service&id=bs-1", nil)
	if recCtx.Code != http.StatusNotImplemented {
		t.Errorf("Context Graph must NOT be enabled by Authority Graph wiring alone; want 501, got %d", recCtx.Code)
	}

	recAuth := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
	if recAuth.Code != http.StatusOK {
		t.Errorf("Authority Graph (only wired endpoint) should serve; got %d: %s", recAuth.Code, recAuth.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Cross-graph regression — D31d boundary
// ---------------------------------------------------------------------------

// TestAuthorityGraph_LegacyRoutesStillRemoved confirms D31d's
// removal of /v1/authority-graph and /v1/businessservices/{id}/governance-map
// has NOT been reverted by D31f. A regression that reintroduced
// either endpoint would surface here.
func TestAuthorityGraph_LegacyRoutesStillRemoved(t *testing.T) {
	srv := withAuthorityGraph(t, &stubAuthorityGraph{projection: makeOKAuthProjection()})

	rec := performRequest(t, srv, http.MethodGet, "/v1/authority-graph?view=service&id=bs-1", nil)
	if rec.Code == http.StatusOK {
		t.Errorf("/v1/authority-graph must remain removed; got 200")
	}

	rec = performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/governance-map", nil)
	if rec.Code == http.StatusOK {
		t.Errorf("/v1/businessservices/{id}/governance-map must remain removed; got 200")
	}
}

// ---------------------------------------------------------------------------
// D31g — Summary + Diagnostics wire round-trip
// ---------------------------------------------------------------------------

// TestHandleGetAuthorityGraph_RoundTripsSummary pins that a
// projection carrying a Summary serialises onto the wire and a
// client can decode it back without surprises. Detailed summary
// semantics are pinned in service_test.go.
func TestHandleGetAuthorityGraph_RoundTripsSummary(t *testing.T) {
	p := makeOKAuthProjection()
	p.Summary = &authoritygraph.Summary{
		SurfaceCount:               2,
		ActiveProfileCount:         3,
		ActiveGrantCount:           3,
		ActiveAgentCount:           1,
		FailModePolicyCount:        1,
		CompleteAuthorityPaths:     1,
		IncompleteAuthorityPaths:   1,
		SurfacesWithPolicyOverride: 1,
		SurfacesInheritingBSPolicy: 1,
	}
	stub := &stubAuthorityGraph{projection: p}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"summary":{`,
		`"surface_count":2`,
		`"active_profile_count":3`,
		`"complete_authority_paths":1`,
		`"surfaces_with_policy_override":1`,
		`"surfaces_inheriting_bs_policy":1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q; got %s", want, body)
		}
	}
	// Decode round-trip.
	var got authoritygraph.Projection
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Summary == nil {
		t.Fatal("decoded projection lost Summary")
	}
	if got.Summary.SurfaceCount != 2 {
		t.Errorf("Summary.SurfaceCount round-trip: want 2, got %d", got.Summary.SurfaceCount)
	}
}

// TestHandleGetAuthorityGraph_RoundTripsDiagnostics pins the
// wire-shape of the diagnostics array.
func TestHandleGetAuthorityGraph_RoundTripsDiagnostics(t *testing.T) {
	p := makeOKAuthProjection()
	p.Diagnostics = []authoritygraph.Diagnostic{
		{
			Kind:     authoritygraph.DiagnosticKindBusinessServiceHasNoActiveSurface,
			Severity: authoritygraph.DiagnosticSeverityWarning,
			NodeRefs: []authoritygraph.NodeRef{{Kind: authoritygraph.NodeKindBusinessService, ID: "bs-1"}},
			Message:  "no active surfaces",
		},
	}
	stub := &stubAuthorityGraph{projection: p}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"diagnostics":[`,
		`"kind":"business_service_has_no_active_surface"`,
		`"severity":"warning"`,
		`"message":"no active surfaces"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q; got %s", want, body)
		}
	}
}

// TestHandleGetAuthorityGraph_OmitsSummaryAndDiagnosticsWhenAbsent
// confirms D31f-compatible wire shape for clients that don't
// consume the new fields. A projection with nil Summary and empty
// Diagnostics must not introduce stray keys on the wire.
func TestHandleGetAuthorityGraph_OmitsSummaryAndDiagnosticsWhenAbsent(t *testing.T) {
	p := makeOKAuthProjection() // Summary nil, Diagnostics empty.
	stub := &stubAuthorityGraph{projection: p}
	srv := withAuthorityGraph(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/graphs/authority?view=service&id=bs-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `"summary":`) {
		t.Errorf("response must omit summary when nil; got %s", body)
	}
	if strings.Contains(body, `"diagnostics":`) {
		t.Errorf("response must omit diagnostics when empty; got %s", body)
	}
}
