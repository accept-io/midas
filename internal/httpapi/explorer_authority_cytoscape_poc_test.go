package httpapi

import (
	"strings"
	"testing"
)

// explorer_authority_cytoscape_poc_test.go — Cytoscape.js Authority
// Graph PoC tests. Tier-1 source-string contracts only:
//   • the vendored library is served and is the genuine Cytoscape
//     distribution (not an empty placeholder)
//   • the PoC module is served and gated on ?cytoscape=1
//   • the PoC module registers itself as the 'authority' lens
//     implementation
//   • the mapper is pure (no DOM, no fetch)
//   • index.html loads the vendor library before the PoC module, and
//     loads the PoC after the production Authority view so its lens
//     registration override wins
//   • the PoC CSS is gated entirely by body.cytoscape-poc-active
//   • the production Authority view + adapter + layout are byte-
//     unchanged (no PoC contamination of production paths)
//
// No JS execution; manual browser verification gates the
// interaction/visual checks documented in the deliverable.

// ── Vendor library ────────────────────────────────────────────────────

// TestExplorer_CytoscapePoc_VendorLibraryServed pins that the vendored
// Cytoscape distribution is served, recognisable, and not a stub.
func TestExplorer_CytoscapePoc_VendorLibraryServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/vendor/cytoscape.min.js")

	// Header sanity — the official distribution begins with a
	// "Copyright (c) ... The Cytoscape Consortium" banner.
	if !strings.Contains(js, "The Cytoscape Consortium") {
		t.Error("Cytoscape PoC: vendored cytoscape.min.js must be the genuine distribution (missing Consortium copyright banner)")
	}
	// Library size sanity — anything under 50 KB is almost certainly
	// a stub or truncated download.
	if len(js) < 50_000 {
		t.Errorf("Cytoscape PoC: vendored cytoscape.min.js looks too small (%d bytes); expected a full minified distribution (~370 KB)", len(js))
	}
}

// ── PoC module ────────────────────────────────────────────────────────

// TestExplorer_CytoscapePoc_ModuleServed pins that the PoC source is
// served at the expected path.
func TestExplorer_CytoscapePoc_ModuleServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"Cytoscape.js Authority Graph PoC",
		"function mapProjectionToElements(projection)",
		"function _isPocActive()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: module must declare %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_ToggleGatedByQueryParam pins that the
// activation gate reads ?cytoscape=1 from the URL and that the module
// short-circuits when the gate is closed. Closing the gate at module
// init time is what makes the PoC a strict opt-in: with the flag off
// the production Authority view's lens registration stands.
func TestExplorer_CytoscapePoc_ToggleGatedByQueryParam(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"new URLSearchParams(window.location.search)",
		"sp.get('cytoscape') === '1'",
		"if (!_isPocActive()) {",
		"return;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: activation gate must check ?cytoscape=1 and short-circuit at module init — missing %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_RegistersAsAuthorityLens pins that the
// module overrides the renderer's 'authority' lens registration when
// active. The override is what makes the existing fetch path flow
// payloads into the PoC's render() instead of the production view.
func TestExplorer_CytoscapePoc_RegistersAsAuthorityLens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"rendered.register('authority', lensImpl);",
		"render: function (payload, mount)",
		"clear: function (mount)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: must register a lensImpl on the 'authority' renderer slot — missing %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_MapperIsPure pins that the mapper does
// not touch the DOM, fetch, or any module state. The mapper is the
// only piece of the PoC that's expected to be reusable in a future
// non-PoC integration.
func TestExplorer_CytoscapePoc_MapperIsPure(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// Locate the mapper body.
	start := strings.Index(js, "function mapProjectionToElements(projection)")
	if start < 0 {
		t.Fatal("Cytoscape PoC: mapProjectionToElements missing")
	}
	// Roughly bound the body — next top-level function declaration.
	end := strings.Index(js[start+1:], "\n  function ")
	if end < 0 {
		t.Fatal("Cytoscape PoC: could not bound mapProjectionToElements body")
	}
	body := js[start : start+1+end]

	for _, banned := range []string{
		"document.",
		"window.",
		"fetch(",
		"_state",
		"MIDASExplorer",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("Cytoscape PoC: mapProjectionToElements must remain pure — contains forbidden token %q", banned)
		}
	}
}

// TestExplorer_CytoscapePoc_NodeKindsCoverAuthorityProjection pins
// that the mapper emits Cytoscape data for every Authority node kind.
// Missing any kind would leave the PoC unable to render parts of the
// live projection.
func TestExplorer_CytoscapePoc_NodeKindsCoverAuthorityProjection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"business_service:",
		"decision_surface:",
		"authority_profile:",
		"authority_grant:",
		"agent:",
		"fail_mode_policy:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: kind style table must cover Authority node kind %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_EdgeKindsCoverAuthorityProjection pins
// that the sidecar classifier knows every governance edge kind so
// dashed-line styling applies correctly.
func TestExplorer_CytoscapePoc_EdgeKindsCoverAuthorityProjection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"'surface_has_fail_mode_policy'",
		"'business_service_has_fail_mode_policy'",
		"'profile_escalates_to'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: governance edge classifier must include %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_InteractionHandlersWired pins the
// presence of the four core interaction handlers required by the
// PoC scope: node hover, edge hover, click, and background tap.
func TestExplorer_CytoscapePoc_InteractionHandlersWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"_cy.on('mouseover', 'node'",
		"_cy.on('mouseout',  'node'",
		"_cy.on('mouseover', 'edge'",
		"_cy.on('tap', 'node'",
		"_cy.on('tap', function (evt)",
		"_focusNode(",
		"_focusEdge(",
		"_emphasiseRootPath(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: required interaction wiring missing — %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_LifecycleDestroyAvailable pins that the
// module exposes a destroy/cleanup path. The PoC scope explicitly
// requires this: "Destroy/cleanup the Cytoscape instance when
// leaving/re-rendering the view if applicable."
func TestExplorer_CytoscapePoc_LifecycleDestroyAvailable(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _destroyCy()",
		"_cy.destroy()",
		"_cy = null;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: destroy/cleanup path missing — %q", want)
		}
	}
}

// ── Index.html load order + CSS gating ───────────────────────────────

// TestExplorer_CytoscapePoc_IndexLoadOrder pins that index.html loads
// the vendor library before the PoC module, and loads the PoC after
// authority-graph-view.js so the PoC's lens-registration override
// wins for ?cytoscape=1 sessions.
func TestExplorer_CytoscapePoc_IndexLoadOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	if rec.Code != 200 {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	vendorIdx := strings.Index(html, "/explorer/assets/js/vendor/cytoscape.min.js")
	pocIdx := strings.Index(html, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	viewIdx := strings.Index(html, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	cssIdx := strings.Index(html, "/explorer/assets/css/authority-cytoscape-poc.css")

	if vendorIdx < 0 {
		t.Fatal("Cytoscape PoC: index.html must load the vendor library")
	}
	if pocIdx < 0 {
		t.Fatal("Cytoscape PoC: index.html must load the PoC module")
	}
	if viewIdx < 0 {
		t.Fatal("Cytoscape PoC: index.html must still load the production Authority view")
	}
	if cssIdx < 0 {
		t.Fatal("Cytoscape PoC: index.html must load the PoC stylesheet")
	}
	if vendorIdx >= pocIdx {
		t.Errorf("Cytoscape PoC: vendor library (offset %d) must load BEFORE PoC module (offset %d)", vendorIdx, pocIdx)
	}
	if viewIdx >= pocIdx {
		t.Errorf("Cytoscape PoC: production Authority view (offset %d) must load BEFORE PoC module (offset %d) so the PoC's lens override wins", viewIdx, pocIdx)
	}
}

// TestExplorer_CytoscapePoc_CSSScopedToActiveBodyClass pins that the
// PoC stylesheet's rules are gated by body.cytoscape-poc-active so
// disabling the PoC drops every rule. The body class is set only by
// the PoC module when the ?cytoscape=1 toggle is active.
func TestExplorer_CytoscapePoc_CSSScopedToActiveBodyClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// Strip CSS comments to avoid false positives on documentation
	// references to selectors.
	css = stripCSSComments(css)

	// Each rule should begin with body.cytoscape-poc-active. Walk all
	// rule openings ('{') and check the preceding selector slice.
	for i := 0; i < len(css); i++ {
		if css[i] != '{' {
			continue
		}
		// Find the start of this selector (preceding '}' or top of file).
		start := strings.LastIndexAny(css[:i], "}")
		if start < 0 {
			start = 0
		} else {
			start++
		}
		selector := strings.TrimSpace(css[start:i])
		if selector == "" {
			continue
		}
		if !strings.HasPrefix(selector, "body.cytoscape-poc-active") {
			t.Errorf("Cytoscape PoC: every CSS rule must be scoped under body.cytoscape-poc-active — rogue selector %q", selector)
		}
	}
}

func stripCSSComments(s string) string {
	out := make([]byte, 0, len(s))
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				break
			}
			i += 2 + j + 2
			continue
		}
		out = append(out, s[i])
		i++
	}
	return string(out)
}

// ── Production Authority view untouched ──────────────────────────────

// TestExplorer_CytoscapePoc_ProductionAuthorityViewByteUnchanged pins
// the load-bearing entry points of the production Authority view: the
// adapter's mapToCardLayout, the layout helper's computeAuthority
// Layout signature, and the view's renderAuthorityGraph function name.
// The PoC must not depend on any modification of these.
func TestExplorer_CytoscapePoc_ProductionAuthorityViewByteUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	layoutJS  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")
	viewJS    := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	if !strings.Contains(adapterJS, "function mapToCardLayout(projection, view)") {
		t.Error("Cytoscape PoC: production adapter mapToCardLayout signature must remain intact")
	}
	if !strings.Contains(layoutJS, "function computeAuthorityLayout(spec, GMAP, layerState)") {
		t.Error("Cytoscape PoC: production layout helper signature must remain intact")
	}
	if !strings.Contains(viewJS, "function renderAuthorityGraph(payload, ctx)") {
		t.Error("Cytoscape PoC: production view renderAuthorityGraph must remain intact")
	}
	if strings.Contains(viewJS, "cytoscape") {
		t.Error("Cytoscape PoC: production Authority view must NOT reference Cytoscape — PoC is opt-in via the separate module")
	}
}

// TestExplorer_CytoscapePoc_PatchesAuthorityViewRefresh pins the
// blank-canvas hotfix: the PoC overrides authorityView.refresh on the
// namespace so the two known production call sites
// (index.html L2062-2064 route entry, L3251-3252 service-list click)
// route through the PoC's _pocRefresh instead of the production view's
// refresh. The lens-registration override remains as a defensive
// fallback for any future tranche that routes through renderer.render.
func TestExplorer_CytoscapePoc_PatchesAuthorityViewRefresh(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _pocRefresh(opts)",
		"function _patchAuthorityViewRefresh()",
		"av._pocOriginalRefresh = av._pocOriginalRefresh || av.refresh;",
		"av.refresh = _pocRefresh;",
		// _pocRefresh must fetch via the same Authority adapter as the
		// production path so the live endpoint is the data source.
		"window.MIDASExplorerGraph.authorityAdapter",
		"adapter.fetch({ view: 'service', id: rootId, depth: depth })",
		// Loading state so the user never sees a blank canvas while
		// the projection is fetched.
		"_renderUnavailable(_ensureMount(), 'Loading…');",
		// Empty-state message for missing rootId.
		"'Select a business service to view the Authority Graph.'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC blank-canvas hotfix: authorityView.refresh override path missing %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_PostInitResizeFitGuard pins the
// requestAnimationFrame-deferred resize + fit. Cytoscape captures
// container dimensions at init time; if the mount is still laying out
// (display: none sibling toggles + flex/grid containers), the canvas
// can come up with zero render area. The rAF-deferred resize/fit
// recovers from that race.
func TestExplorer_CytoscapePoc_PostInitResizeFitGuard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"window.requestAnimationFrame(function ()",
		"_cy.resize();",
		// D33a-impl-1 — Hard-coded `_cy.fit(undefined, 60);` was
		// replaced by a safe-area-aware computation. Pin the new shape;
		// the rAF guard remains in place.
		"_cy.fit(undefined, _safeAreaPadding());",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC blank-canvas hotfix: post-init resize/fit guard missing %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_PublicSurfaceExposed pins the test-visible
// surface so future tranches can build on the mapper without
// re-implementing it.
func TestExplorer_CytoscapePoc_PublicSurfaceExposed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"window.MIDASExplorerGraph.cytoscapePoc = {",
		"isActive:",
		"mapProjectionToElements:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: public surface entry missing — %q", want)
		}
	}
}
