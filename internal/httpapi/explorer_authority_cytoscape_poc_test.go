package httpapi

import (
	"net/http"
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

// TestExplorer_CytoscapePoc_ModuleServed pins that the Authority
// renderer module source is served at the expected path. The
// pre-D37b docblock header `"Cytoscape.js Authority Graph PoC"`
// was retired in D37b when the module became the production
// Authority renderer; the new docblock identifies it as the
// Cytoscape HTML-card Authority renderer on GraphViewport. The
// internal symbols `mapProjectionToElements` and `_isPocActive`
// remain defined (internal naming debt acknowledged).
func TestExplorer_CytoscapePoc_ModuleServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// D37b — Production module identity.
		"Authority Graph — Cytoscape HTML-card renderer on GraphViewport",
		"function mapProjectionToElements(projection)",
		"function _isPocActive()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: Authority renderer module must declare %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_ToggleGatedByQueryParam pinned the
// pre-D37b `?cytoscape=1` activation gate. D37b RETIRED that gate
// because the Cytoscape HTML-card renderer is now the production
// Authority renderer (registered with GraphViewport under the id
// `'authority'`) and runs unconditionally. This test is preserved
// (renamed in spirit, not in symbol so other test runners keep
// working) to pin the post-D37b invariants:
//   • `_isPocActive` is preserved as a public symbol (returns true
//     unconditionally now) so any stale call sites do not break.
//   • The pre-D37b URL-flag check `sp.get('cytoscape') === '1'`
//     must NOT remain in the executable code.
//   • The pre-D37b early-return `if (!_isPocActive()) { return; }`
//     guard at IIFE top must NOT remain.
func TestExplorer_CytoscapePoc_ToggleGatedByQueryParam(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	exec := stripJSComments(js)

	// Positive — `_isPocActive` symbol still defined (back-compat for
	// any stale callers).
	if !strings.Contains(js, "function _isPocActive()") {
		t.Error("D37b: _isPocActive function must still be defined (preserved as public symbol after gate retirement)")
	}
	// Positive — the function now returns true unconditionally.
	if !strings.Contains(exec, "return true;") {
		t.Error("D37b: _isPocActive must return true unconditionally (gate retired)")
	}

	// Negative — pre-D37b URL-flag activation gate must be retired
	// from executable code.
	for _, banned := range []string{
		"sp.get('cytoscape') === '1'",
		"if (!_isPocActive()) {",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D37b: pre-D37b ?cytoscape=1 gate %q must be retired from executable code", banned)
		}
	}
}

// TestExplorer_CytoscapePoc_RegistersAsAuthorityLens pins that the
// module owns Authority lens activation. D37p-clean-1 retired the
// dead `MIDASExplorerGraph.renderer.register('authority', lensImpl)`
// dispatcher path; the live Authority activation now flows through:
//   • `viewport.register('authority', _authorityRendererFactory)` —
//     GraphViewport host registration (D35g) is the platform-level
//     activation seam.
//   • `_patchAuthorityViewRefresh()` — patches
//     `authorityView.refresh` so the production refresh routes through
//     `_pocRefresh` and renders inside the host's renderer slot.
// Both call sites must remain present.
func TestExplorer_CytoscapePoc_RegistersAsAuthorityLens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"vp.register('authority', _authorityRendererFactory)",
		"function _patchAuthorityViewRefresh()",
		"av.refresh = _pocRefresh;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC: must own Authority lens activation via the live platform seams — missing %q", want)
		}
	}
	// The dead dispatcher path must be gone.
	if strings.Contains(js, "rendered.register('authority', lensImpl);") {
		t.Errorf("D37p-clean-1: dead rendered.register('authority', lensImpl) call must be removed from authority-cytoscape-poc.js")
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
// PoC stylesheet's rules are gated by host-owned renderer identity.
//
// D35f-retire-transitional-renderer-debt — the gate moved from the
// pre-D35f body-class `body.cytoscape-poc-active` to the host-owned
// `.midas-graph-viewport[data-active-renderer="authority"]`
// attribute selector. Every rule now begins with this selector.
func TestExplorer_CytoscapePoc_CSSScopedToActiveBodyClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// Strip CSS comments to avoid false positives on documentation
	// references to selectors.
	css = stripCSSComments(css)

	const expectedPrefix = `.midas-graph-viewport[data-active-renderer="authority"]`

	// Each rule should begin with the host-owned renderer-identity
	// selector. Walk all rule openings ('{') and check the
	// preceding selector slice.
	for i := 0; i < len(css); i++ {
		if css[i] != '{' {
			continue
		}
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
		if !strings.HasPrefix(selector, expectedPrefix) {
			t.Errorf("Cytoscape PoC: every CSS rule must be scoped under %s — rogue selector %q", expectedPrefix, selector)
		}
	}

	// Negative pin: D35f retired the pre-D35f body-class gate. No
	// rule should still use `body.cytoscape-poc-active`.
	if strings.Contains(css, "body.cytoscape-poc-active") {
		t.Error("D35f: `body.cytoscape-poc-active` body-class gate must be retired — found in executable CSS")
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

// stripJSComments removes both `/* … */` block comments and `// …`
// line comments from a JS source string. Used by D34h tests that
// need to assert tokens are absent from EXECUTABLE JS only — the
// spike module's headers and inline comments legitimately reference
// retired tokens (e.g. "the deleted `_wireCardDrag`…") as
// documentation, and those references must not trip a negative pin.
func stripJSComments(s string) string {
	out := make([]byte, 0, len(s))
	i := 0
	n := len(s)
	for i < n {
		// Block comment.
		if i+1 < n && s[i] == '/' && s[i+1] == '*' {
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				break
			}
			i += 2 + j + 2
			continue
		}
		// Line comment — consume up to (but not including) the
		// newline so line counts are preserved.
		if i+1 < n && s[i] == '/' && s[i+1] == '/' {
			j := strings.Index(s[i:], "\n")
			if j < 0 {
				break
			}
			i += j
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
//
// D33x-fit-zoom-root — The settle path now delegates from `_settleFit`
// to `_fitToAvailableCanvas(_cy)`, which uses per-side overlay-aware
// insets via `cy.viewport({zoom, pan})` instead of the old symmetric
// `cy.fit(undefined, _safeAreaPadding())`. The new test pin asserts
// the rAF wrapper, the `cy.resize()` call, AND the delegation to the
// new helper. `_safeAreaPadding` itself remains exported (headless
// paths still call it) but is no longer the runtime fit budget.
func TestExplorer_CytoscapePoc_PostInitResizeFitGuard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"window.requestAnimationFrame(function ()",
		"_cy.resize();",
		// The settle path must call into the new asymmetric fit
		// helper. The previous pin (`_cy.fit(undefined,
		// _safeAreaPadding())`) was retired with D33x-fit-zoom-root.
		"_fitToAvailableCanvas(_cy)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("Cytoscape PoC blank-canvas hotfix: post-init resize/fit guard missing %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_FitHelperUsesAsymmetricViewport pins the
// new D33x-fit-zoom-root fit contract: the helper must read the
// elements' bounding box and apply zoom + pan via `cy.viewport()`
// rather than the symmetric `cy.fit(eles, padding)` API. That's how
// it honours per-side overlay insets (legend left, inspector right,
// toolbar bottom-right).
func TestExplorer_CytoscapePoc_FitHelperUsesAsymmetricViewport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _fitToAvailableCanvas(",
		"boundingBox()",
		// The asymmetric fit applies zoom + pan via cy.viewport so
		// per-side insets are honoured. cy.fit(eles, padding) is
		// symmetric and would force the largest single-side overlay
		// onto every side.
		"cy.viewport({",
		"zoom: z",
		"pan: { x:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-fit-zoom-root: _fitToAvailableCanvas must use asymmetric viewport — missing %q", want)
		}
	}
	// Negative pin: the fit helper must not fall back to symmetric
	// cy.fit() inside its own body. Bound the check to the helper's
	// body so other call sites (toolbar bridge, etc.) aren't matched.
	start := strings.Index(js, "function _fitToAvailableCanvas(")
	if start < 0 {
		t.Fatal("D33x-fit-zoom-root: _fitToAvailableCanvas function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D33x-fit-zoom-root: cannot bound _fitToAvailableCanvas body")
	}
	body := js[start : start+end]
	if strings.Contains(body, "cy.fit(") {
		t.Error("D33x-fit-zoom-root: _fitToAvailableCanvas must not delegate to symmetric cy.fit() — that defeats the per-side inset model")
	}
}

// TestExplorer_CytoscapePoc_CenterOnRootHelper pins the centre-on-
// root contract: a function that (a) locates the projection's root
// by isRoot first then business_service fallback, (b) bumps zoom to
// a readable level only when the current zoom is below threshold,
// and (c) pans the root to the canvas centre via `cy.viewport`.
func TestExplorer_CytoscapePoc_CenterOnRootHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _findRootNode(",
		"function _centerOnRoot(",
		// Root resolution: isRoot first, business_service fallback.
		"isRoot",
		"'business_service'",
		// Centre-on-root preserves the operator's zoom when already
		// readable, otherwise bumps to CENTRE_READABLE_ZOOM.
		"CENTRE_READABLE_ZOOM",
		// Public surface entry points used by the toolbar bridge.
		"centerOnRoot:",
		"findRootNode:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-fit-zoom-root: centre-on-root contract missing %q", want)
		}
	}
}

// TestExplorer_CytoscapePoc_ZoomByHelperCentreAnchored pins the
// zoom-in / zoom-out contract: multiply current zoom by the factor,
// anchored at the canvas centre (`cy.zoom({level, renderedPosition})`
// with renderedPosition at cw/2, ch/2). That preserves the apparent
// graph centre across +/- presses, matching the legacy MIDAS feel.
func TestExplorer_CytoscapePoc_ZoomByHelperCentreAnchored(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _zoomBy(",
		"cy.zoom({",
		"level: next",
		"renderedPosition: { x: cw / 2, y: ch / 2 }",
		"ZOOM_STEP_FACTOR",
		"zoomBy:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-fit-zoom-root: _zoomBy contract missing %q", want)
		}
	}
}

// TestExplorer_CytoscapeToolbar_ModuleServed pins that the new
// toolbar-bridge module is served by the Explorer asset route, links
// in index.html after the PoC script, and registers the expected
// public surface.
func TestExplorer_CytoscapeToolbar_ModuleServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js")
	for _, want := range []string{
		"window.MIDASExplorerGraph.cytoscapeToolbar",
		"function _onZoomIn(",
		"function _onZoomOut(",
		"function _onFit(",
		"function _onCentre(",
		"function _onFocusToggle(",
		"function refit()",
		"function wire()",
		// Capture phase + stopImmediatePropagation are the precise
		// mechanism that lets the bridge override the legacy bubble-
		// phase handlers when the PoC is active.
		"addEventListener('click', bindings[i].handler, true)",
		"stopImmediatePropagation",
		// Body-class observer for drawer / focus-mode toggles.
		"MutationObserver",
		"'gmap-inspector-collapsed'",
		"'gmap-focus-mode'",
		// Window resize observer.
		"window.addEventListener('resize'",
		// PoC-active gate: every callback must check body class so
		// the bridge is a no-op when the PoC is not the active
		// renderer.
		"cytoscape-poc-active",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-fit-zoom-root: cytoscape-toolbar.js missing %q", want)
		}
	}

	// The script must be linked from index.html AFTER the PoC script
	// so `window.MIDASExplorerGraph.cytoscapePoc.getCy` is defined
	// at the time the bridge wires up.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	pocIdx := strings.Index(body, `src="/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"`)
	tbIdx := strings.Index(body, `src="/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"`)
	if pocIdx < 0 {
		t.Fatal("D33x-fit-zoom-root: authority-cytoscape-poc.js script tag missing from index.html")
	}
	if tbIdx < 0 {
		t.Fatal("D33x-fit-zoom-root: authority-cytoscape-toolbar.js script tag missing from index.html")
	}
	if pocIdx > tbIdx {
		t.Error("D33x-fit-zoom-root: authority-cytoscape-toolbar.js must be loaded AFTER authority-cytoscape-poc.js")
	}
}

// TestExplorer_CytoscapePoc_PublicSurfaceExposesToolbarAPI pins that
// the PoC's public surface includes the helpers the toolbar bridge
// needs: getCy, fit, zoomBy, centerOnRoot, findRootNode,
// ZOOM_STEP_FACTOR.
func TestExplorer_CytoscapePoc_PublicSurfaceExposesToolbarAPI(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"getCy:",
		"fit:",
		"zoomBy:",
		"centerOnRoot:",
		"findRootNode:",
		"ZOOM_STEP_FACTOR:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-fit-zoom-root: PoC public surface must export %q for the toolbar bridge", want)
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

// ── D33x-list-mode contract tests ──────────────────────────────────────

// TestExplorer_D33xListMode_FloatingCardRetired pins that the floating
// PoC inspector aside has been fully removed from the PoC module.
// The carrier-DOM contract that feeds the PRODUCTION right drawer is
// independent and must remain (`_renderInspectorCarriers`,
// `cytoscape-poc-inspector-carrier`, `hooks.selectNode`).
func TestExplorer_D33xListMode_FloatingCardRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// Floating-card surface (the only thing this tranche removed).
	for _, gone := range []string{
		"function _renderInspector(node)",
		"function _renderInspector(",
		"function _renderInspectorEmpty(",
		"function _wireInspectorToggle(",
		"function _setInspectorExpanded(",
		"var _inspectorEl",
		"var _inspectorExpanded",
		"var INSPECTOR_W_COMPACT",
		"var INSPECTOR_W_EXPANDED",
		// Markup that was only ever used inside the floating
		// aside (the carrier marker class
		// `cytoscape-poc-inspector-carrier` shares a prefix and
		// must NOT be matched here).
		`className = 'cytoscape-poc-inspector'`,
		`data-poc-toggle="inspector"`,
	} {
		if strings.Contains(js, gone) {
			t.Errorf("D33x-list-mode: floating PoC card render path must be retired — found %q", gone)
		}
	}

	// Production carrier-DOM + right-drawer routing must remain.
	for _, want := range []string{
		"function _renderInspectorCarriers(",
		"function _clearInspectorCarriers(",
		"function _detailsForCarrier(",
		"'cytoscape-poc-inspector-carrier'",
		"hooks.selectNode(nodeId)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-list-mode: production right-drawer wiring must remain — missing %q", want)
		}
	}
}

// TestExplorer_D33xListMode_PublicSurfaceExportsListModeAPI pins the
// new public surface entries used by index.html's lens-aware Form /
// Records branch.
func TestExplorer_D33xListMode_PublicSurfaceExportsListModeAPI(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"setViewMode:",
		"getViewMode:",
		"applyListLayout:",
		"applyGraphLayout:",
		"_computeListPositions:",
		"LIST_GROUP_ORDER:",
		"LIST_MAX_COLUMNS:",
		// The view-mode state itself.
		"var _viewMode = 'graph';",
		"var _savedGraphPositions = null;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-list-mode: PoC public surface must export %q", want)
		}
	}
}

// TestExplorer_D33xListMode_ListLayoutHidesEdgesAndRestoresThem pins
// the edge-visibility contract: applyListLayout sets edges'
// `display` to `none`; applyGraphLayout removes that style so edges
// return to their stylesheet defaults.
func TestExplorer_D33xListMode_ListLayoutHidesEdgesAndRestoresThem(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// Bound the list-layout body so we don't accidentally match
	// edge style mutations elsewhere.
	listStart := strings.Index(js, "function applyListLayout()")
	if listStart < 0 {
		t.Fatal("D33x-list-mode: applyListLayout function definition missing")
	}
	listEnd := strings.Index(js[listStart:], "\n  }")
	if listEnd < 0 {
		t.Fatal("D33x-list-mode: cannot bound applyListLayout body")
	}
	listBody := js[listStart : listStart+listEnd]
	if !strings.Contains(listBody, `_cy.edges().style('display', 'none')`) {
		t.Error("D33x-list-mode: applyListLayout must hide edges via _cy.edges().style('display', 'none')")
	}
	// Position snapshot must happen before edges are hidden so
	// switching back to graph mode can restore positions faithfully.
	if !strings.Contains(listBody, "_savedGraphPositions") {
		t.Error("D33x-list-mode: applyListLayout must snapshot positions into _savedGraphPositions before mutating the graph")
	}

	graphStart := strings.Index(js, "function applyGraphLayout()")
	if graphStart < 0 {
		t.Fatal("D33x-list-mode: applyGraphLayout function definition missing")
	}
	graphEnd := strings.Index(js[graphStart:], "\n  }")
	if graphEnd < 0 {
		t.Fatal("D33x-list-mode: cannot bound applyGraphLayout body")
	}
	graphBody := js[graphStart : graphStart+graphEnd]
	if !strings.Contains(graphBody, `_cy.edges().removeStyle('display')`) {
		t.Error("D33x-list-mode: applyGraphLayout must restore edges via _cy.edges().removeStyle('display')")
	}
	// Position restore must happen so the spine layout returns.
	if !strings.Contains(graphBody, "_savedGraphPositions[id]") {
		t.Error("D33x-list-mode: applyGraphLayout must restore each node's saved position")
	}
}

// TestExplorer_D33xListMode_ListPositionsRootFirstAndGrouped pins the
// ordering contract: nodes are grouped in the documented order with
// the root sorted first inside its kind bucket. The sort comparator
// inside `_computeListPositions` checks `isRoot` first.
func TestExplorer_D33xListMode_ListPositionsRootFirstAndGrouped(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// The documented group order must appear inside LIST_GROUP_ORDER
	// in the documented sequence.
	groupBlock := func(t *testing.T) string {
		t.Helper()
		start := strings.Index(js, "var LIST_GROUP_ORDER = [")
		if start < 0 {
			t.Fatal("D33x-list-mode: LIST_GROUP_ORDER declaration missing")
		}
		end := strings.Index(js[start:], "];")
		if end < 0 {
			t.Fatal("D33x-list-mode: cannot bound LIST_GROUP_ORDER literal")
		}
		return js[start : start+end]
	}(t)
	wantOrder := []string{
		"'business_service'",
		"'decision_surface'",
		"'authority_profile'",
		"'authority_grant'",
		"'agent'",
		"'fail_mode_policy'",
		"'escalation_target'",
	}
	last := -1
	for _, k := range wantOrder {
		idx := strings.Index(groupBlock, k)
		if idx < 0 {
			t.Errorf("D33x-list-mode: LIST_GROUP_ORDER must contain %q", k)
			continue
		}
		if idx <= last {
			t.Errorf("D33x-list-mode: LIST_GROUP_ORDER must list %q AFTER the previous group", k)
		}
		last = idx
	}

	// Root-first sort comparator must check isRoot before label/id.
	if !strings.Contains(js, "var ar = a.data('isRoot') ? 0 : 1;") {
		t.Error("D33x-list-mode: list-mode sort comparator must place isRoot:true nodes ahead of their bucket peers")
	}
}

// TestExplorer_D33xListMode_ColumnWrappingRule pins the column-
// wrapping math: derive rowsPerCol from the canvas height, cap at
// `LIST_MAX_COLUMNS = 4`, and step `col++` only when the current
// column has filled to `perCol`.
func TestExplorer_D33xListMode_ColumnWrappingRule(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"var LIST_MAX_COLUMNS    = 4;",
		"Math.ceil(effectiveRows / rowsPerCol)",
		"if (cols > LIST_MAX_COLUMNS) cols = LIST_MAX_COLUMNS;",
		"var perCol = Math.ceil(effectiveRows / cols);",
		"if (row >= perCol && col < cols - 1) {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-list-mode: column-wrapping math missing %q", want)
		}
	}
}

// TestExplorer_D33xListMode_SetViewModeIdempotentAndBounded pins the
// `setViewMode(mode)` contract: only 'graph' / 'list' are accepted,
// re-applying the same mode is safe (idempotent), and the function
// always finishes by calling the existing fit helper.
func TestExplorer_D33xListMode_SetViewModeIdempotentAndBounded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "function setViewMode(mode)")
	if start < 0 {
		t.Fatal("D33x-list-mode: setViewMode function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D33x-list-mode: cannot bound setViewMode body")
	}
	body := js[start : start+end]
	if !strings.Contains(body, `if (mode !== 'graph' && mode !== 'list') return _viewMode;`) {
		t.Error("D33x-list-mode: setViewMode must reject unknown modes (return current mode unchanged)")
	}
	if !strings.Contains(body, "applyListLayout()") || !strings.Contains(body, "applyGraphLayout()") {
		t.Error("D33x-list-mode: setViewMode must dispatch to applyListLayout / applyGraphLayout")
	}
	// Both apply* helpers end with _fitToAvailableCanvas calls so
	// the new layout fits the visible canvas after switching.
	if !strings.Contains(js, "_fitToAvailableCanvas(_cy, { elements: _cy.nodes() })") {
		t.Error("D33x-list-mode: applyListLayout must end with _fitToAvailableCanvas(_cy, { elements: _cy.nodes() })")
	}
}

// TestExplorer_D33xLeftPocPanel_LegendRetired pins that the
// floating left "Authority Graph" PoC overlay panel — header
// "Authority Graph" + theme chip "AUTHORITY-THIN-CARD-V1", body
// "NODE KINDS" / "FUTURE OVERLAYS" / "Drift" / "Resilience" /
// "Diagnostics" / "Runtime evidence" — has been removed end-to-end:
// the visible strings are absent from PoC JS, the related CSS
// selectors are absent from PoC CSS, and the JS state / helpers
// that composed the panel are gone. List Mode + carrier-DOM
// integration + node tap selection are explicitly preserved.
func TestExplorer_D33xLeftPocPanel_LegendRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// 1. Visible strings the user named in the brief. Comments in
	//    the JS retirement-marker block may legitimately mention
	//    these as historical references; pin against quoted-string
	//    literals only (single or double quotes).
	for _, gone := range []string{
		// "NODE KINDS" + "FUTURE OVERLAYS" appeared as the visible
		// section headers (rendered uppercase via CSS). The source
		// strings were "Node kinds" / "Future overlays".
		">Node kinds<",
		">Future overlays<",
		// "AUTHORITY-THIN-CARD-V1" appeared inside the theme chip,
		// fed by `_escHtml(_activeTheme)`. The visible string came
		// from the chip markup; remove the chip and the visible
		// string disappears with it.
		"cytoscape-poc-theme-chip",
		// "Drift" / "Resilience" / "Diagnostics" / "Runtime
		// evidence" appeared in the "FUTURE OVERLAYS" placeholder
		// list, each as a literal `<li>` in `_renderLegend`'s
		// innerHTML. Pin against the placeholder-class markup
		// shape so we don't accidentally match other features
		// named "Diagnostics" elsewhere.
		`class="cytoscape-poc-placeholder"`,
		`'<li><span class="cytoscape-poc-placeholder">●</span> Drift</li>'`,
		`'<li><span class="cytoscape-poc-placeholder">●</span> Resilience</li>'`,
		`'<li><span class="cytoscape-poc-placeholder">●</span> Diagnostics</li>'`,
		`'<li><span class="cytoscape-poc-placeholder">●</span> Runtime evidence</li>'`,
	} {
		if strings.Contains(js, gone) {
			t.Errorf("D33x-left-poc-panel: floating left PoC overlay must remain retired — found %q in PoC JS", gone)
		}
	}

	// 2. JS state + helpers that composed the panel.
	for _, gone := range []string{
		"function _setLegendExpanded(",
		"function _renderLegend(",
		"function _legendRow(",
		"var _legendEl",
		"var _legendExpanded",
		"var LEGEND_W_COMPACT",
		"var LEGEND_W_EXPANDED",
		`_legendEl.className = 'cytoscape-poc-legend'`,
	} {
		if strings.Contains(js, gone) {
			t.Errorf("D33x-left-poc-panel: legend JS state/helper must remain retired — found %q", gone)
		}
	}

	// 3. CSS selectors that styled the panel. Retirement-marker
	//    comments may still mention class names; pin against the
	//    executable rule shape `<sel> {`.
	cssExec := stripCSSComments(css)
	for _, banned := range []string{
		".cytoscape-poc-legend {",
		".cytoscape-poc-legend[data-expanded",
		".cytoscape-poc-legend-body",
		".cytoscape-poc-legend-title",
		".cytoscape-poc-legend-kinds",
		".cytoscape-poc-legend-future",
		".cytoscape-poc-swatch {",
		".cytoscape-poc-placeholder {",
		".cytoscape-poc-status-chip {",
		".cytoscape-poc-theme-chip {",
		".cytoscape-poc-toggle {",
	} {
		if strings.Contains(cssExec, banned) {
			t.Errorf("D33x-left-poc-panel: legend CSS selector must remain retired — found %q", banned)
		}
	}

	// 4. List Mode + carrier-DOM integration + tap selection must
	//    remain wired. These are the explicitly-preserved contracts.
	for _, want := range []string{
		// Carrier DOM contract (feeds the MIDAS production right drawer).
		"function _renderInspectorCarriers(",
		"function _clearInspectorCarriers(",
		"'cytoscape-poc-inspector-carrier'",
		// Renderer-hook dispatch — node tap selects in the
		// production drawer.
		"hooks.selectNode(nodeId)",
		// List Mode public surface.
		"setViewMode:",
		"getViewMode:",
		"applyListLayout:",
		"applyGraphLayout:",
		"LIST_GROUP_ORDER:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-left-poc-panel: preserved PoC contract missing — %q (must NOT be touched by left-panel removal)", want)
		}
	}
}

// TestExplorer_D33xListMode_FormButtonLensBranching pins the lens-
// aware branch in index.html's `setWorkbenchMode('form')` handler:
// the Form / Records button toggles List Mode ONLY when the
// Cytoscape PoC is active AND the active lens is `authority`.
// Outside that combination the legacy `showRecord` path runs
// unchanged so Context Graph + non-PoC sessions are not regressed.
func TestExplorer_D33xListMode_FormButtonLensBranching(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Bound the form branch so we don't match elsewhere.
	start := strings.Index(body, "if (mode === 'form') {")
	if start < 0 {
		t.Fatal("D33x-list-mode: setWorkbenchMode('form') branch missing from index.html")
	}
	// The form branch ends at the following `if (mode === 'context'`.
	end := strings.Index(body[start:], "if (mode === 'context'")
	if end < 0 {
		t.Fatal("D33x-list-mode: cannot bound the form branch in index.html")
	}
	formBranch := body[start : start+end]

	for _, want := range []string{
		// PoC-active gate. D37h-fix-1 migrated this from the
		// retired `body.cytoscape-poc-active` class to the
		// GraphViewport host renderer-identity signal (preferred
		// public API `viewport.getActiveRendererId() === 'authority'`,
		// with a `data-active-renderer="authority"` DOM fallback).
		"getActiveRendererId",
		`data-active-renderer="authority"`,
		// Authority-lens gate.
		"lens === 'authority'",
		// List-mode dispatch.
		"poc.setViewMode('list')",
		// Legacy fallback still calls showRecord for non-PoC /
		// non-authority sessions (Context Graph + plain Explorer).
		"MIDASExplorerServices.showRecord(serviceId)",
	} {
		if !strings.Contains(formBranch, want) {
			t.Errorf("D33x-list-mode: setWorkbenchMode('form') branch must contain %q", want)
		}
	}

	// D37h-fix-1 — Negative pin: the retired body-class gate must
	// not return as a compatibility shim. The brief is explicit
	// about not re-introducing it.
	if strings.Contains(formBranch, "classList.contains('cytoscape-poc-active')") {
		t.Error("D37h-fix-1: setWorkbenchMode('form') List-Mode gate must NOT reintroduce the retired `cytoscape-poc-active` body class")
	}

	// Clicking Authority again must exit List Mode (return to graph
	// layout). The call sits inside the `mode === 'authority'`
	// branch.
	authStart := strings.Index(body, "if (mode === 'authority') {")
	if authStart < 0 {
		t.Fatal("D33x-list-mode: setWorkbenchMode('authority') branch missing")
	}
	authEnd := strings.Index(body[authStart:], "\n    }")
	if authEnd < 0 {
		t.Fatal("D33x-list-mode: cannot bound the authority branch")
	}
	authBranch := body[authStart : authStart+authEnd]
	if !strings.Contains(authBranch, "_pocOnAuthority.setViewMode('graph')") {
		t.Error("D33x-list-mode: clicking Authority must call setViewMode('graph') to exit List Mode")
	}
}

// ── D34a-cytoscape-html-overlay-spike contract tests ─────────────────

// TestExplorer_D34aHtmlOverlay_ModuleServed pins the MIDAS-owned
// HTML overlay module is served at the expected Explorer asset path,
// is linked from index.html AFTER the PoC script, and exposes the
// documented public surface.
//
// D37r-tranche-B' flip: the overlay-mechanism mechanics (rAF-coalesced
// position sync, `cy.on(SYNC_EVENTS, ...)` subscription, ResizeObserver)
// moved out of this file into the shared platform module at
// /explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js.
// This file remains Authority's TEMPLATE module (the `_buildCard` +
// `_wireCardClick` + dim/restore + install/destroy/refresh public
// surface). The mechanism pins below are now pinned against the
// shared module; this test continues to pin Authority's public
// surface unchanged.
func TestExplorer_D34aHtmlOverlay_ModuleServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")
	for _, want := range []string{
		"window.MIDASExplorerGraph.cytoscapeHtmlOverlay",
		"isActive:",
		"install:",
		"destroy:",
		"refresh:",
		// The module must self-gate on BOTH query params so installing
		// is a no-op when the spike flag is absent.
		"sp.get('cytoscape') === '1' && sp.get('htmlCards') === '1'",
		// D37r-tranche-B' flip — `_sync` retained as a no-op shim
		// for diagnostic back-compat; the live sync lives in the
		// shared module.
		"function _sync(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34a-html-overlay: module missing %q", want)
		}
	}

	// D37r-tranche-B' flip — the live position-sync mechanism is now
	// owned by the shared platform module. Pin its presence there.
	sharedJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	for _, want := range []string{
		"cy.on(SYNC_EVENTS",
		"requestAnimationFrame",
		"ResizeObserver",
	} {
		if !strings.Contains(sharedJS, want) {
			t.Errorf("D34a-html-overlay (post-D37r-tranche-B'): shared overlay module missing %q", want)
		}
	}

	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	pocIdx := strings.Index(body, `src="/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"`)
	ovIdx  := strings.Index(body, `src="/explorer/assets/js/graph/authority/cytoscape-html-overlay.js"`)
	if pocIdx < 0 {
		t.Fatal("D34a-html-overlay: PoC script tag missing from index.html")
	}
	if ovIdx < 0 {
		t.Fatal("D34a-html-overlay: cytoscape-html-overlay.js script tag missing from index.html")
	}
	if pocIdx > ovIdx {
		t.Error("D34a-html-overlay: overlay script must load AFTER the PoC script")
	}
	// Stylesheet too.
	if !strings.Contains(body, `href="/explorer/assets/css/cytoscape-html-overlay.css"`) {
		t.Error("D34a-html-overlay: cytoscape-html-overlay.css <link> missing from index.html")
	}
}

// TestExplorer_D34aHtmlOverlay_GatingFlag pins the spike gate. The
// install function is wrapped by an `isActive()` check that returns
// false unless both `?cytoscape=1` AND `?htmlCards=1` are set.
// Behaviour outside that combo must be a no-op so the spike cannot
// regress existing Context Graph or non-PoC sessions.
func TestExplorer_D34aHtmlOverlay_GatingFlag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")

	// install must early-return when the gate is closed.
	startIdx := strings.Index(js, "function install(cy, options) {")
	if startIdx < 0 {
		t.Fatal("D34a-html-overlay: install function definition missing")
	}
	endIdx := strings.Index(js[startIdx:], "\n  }")
	if endIdx < 0 {
		t.Fatal("D34a-html-overlay: cannot bound install body")
	}
	body := js[startIdx : startIdx+endIdx]
	if !strings.Contains(body, "if (!_isActive()) return;") {
		t.Error("D34a-html-overlay: install() must early-return when _isActive() is false")
	}

	// The PoC must call install + destroy through the public surface.
	pocJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	for _, want := range []string{
		"window.MIDASExplorerGraph.cytoscapeHtmlOverlay",
		"_htmlOverlay.install(_cy, { mount: mount, elements: elements })",
		"_htmlOverlayMod.destroy()",
	} {
		if !strings.Contains(pocJS, want) {
			t.Errorf("D34a-html-overlay: PoC integration missing %q", want)
		}
	}
}

// TestExplorer_D34aHtmlOverlay_NoThirdPartyDependency pins the
// "MIDAS-owned" constraint: the module must not import, install,
// vendor, or reference the cytoscape-node-html-label extension or
// any other third-party dependency.
func TestExplorer_D34aHtmlOverlay_NoThirdPartyDependency(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")

	// No ES-module imports, no script-tag URLs to a CDN, no
	// references to the specific extension name.
	for _, banned := range []string{
		"import ",
		"require(",
		"cytoscape-node-html-label",
		"unpkg.com",
		"cdn.jsdelivr.net",
		// `cy.nodeHtmlLabel(` is the extension's bound entry point;
		// reject it explicitly in case future iterations are tempted.
		"cy.nodeHtmlLabel",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D34a-html-overlay: module must remain MIDAS-owned — found third-party signal %q", banned)
		}
	}
}

// TestExplorer_D34aHtmlOverlay_TextContentUsedForUserData pins the
// security contract: user/projection-supplied strings (label, name,
// status, badge text) must be set via textContent, never via
// innerHTML interpolation. A local escape helper exists for the few
// places textContent can't apply (CSS class tokens) and is
// available on the public surface for tests.
func TestExplorer_D34aHtmlOverlay_TextContentUsedForUserData(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")

	// Bound the _buildCard body and assert textContent is the
	// assignment path for the user-data fields.
	startIdx := strings.Index(js, "function _buildCard(data) {")
	if startIdx < 0 {
		t.Fatal("D34a-html-overlay: _buildCard function definition missing")
	}
	endIdx := strings.Index(js[startIdx:], "\n  }")
	if endIdx < 0 {
		t.Fatal("D34a-html-overlay: cannot bound _buildCard body")
	}
	body := js[startIdx : startIdx+endIdx]

	for _, want := range []string{
		"eyebrow.textContent",
		"name.textContent",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34a-html-overlay: _buildCard must set %q (textContent path)", want)
		}
	}

	// Negative pin: no innerHTML assignment inside _buildCard. The
	// extension's pattern of `el.innerHTML = template` is unsafe
	// without escaping; we forbid it here entirely.
	if strings.Contains(body, ".innerHTML =") {
		t.Error("D34a-html-overlay: _buildCard must NOT assign innerHTML — user/projection data flows through textContent")
	}

	// Local escape helper exists.
	for _, want := range []string{
		"function _escHtml(s)",
		"replace(/&/g, '&amp;')",
		"replace(/</g, '&lt;')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34a-html-overlay: local HTML-escape helper missing %q", want)
		}
	}
}

// TestExplorer_D34aHtmlOverlay_DestroyRemovesDomAndListeners pins
// the lifecycle contract: destroy() releases the overlay layer +
// listeners and restores the native cy node opacity.
//
// D37r-tranche-B' flip: the listener-detach / rAF-cancel / layer-DIV
// removal mechanics moved to the shared platform module. Authority's
// destroy() now delegates to the shared module's
// `handle.destroy()` and continues to restore native cy node opacity
// (a lens-specific concern that remains in this file). The pre-
// tranche-B' pins on `cancelAnimationFrame(_syncRaf)`, `_cy.off(
// SYNC_EVENTS`, `window.removeEventListener('resize'`,
// `_resizeObs.disconnect()`, `_layerEl.parentNode.removeChild(
// _layerEl)` are flipped here to assert the new shape.
func TestExplorer_D34aHtmlOverlay_DestroyRemovesDomAndListeners(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")

	startIdx := strings.Index(js, "function destroy() {")
	if startIdx < 0 {
		t.Fatal("D34a-html-overlay: destroy function definition missing")
	}
	endIdx := strings.Index(js[startIdx:], "\n  }")
	if endIdx < 0 {
		t.Fatal("D34a-html-overlay: cannot bound destroy body")
	}
	body := js[startIdx : startIdx+endIdx]
	// Authority destroy() delegates listener / layer cleanup to the
	// shared module's handle.
	for _, want := range []string{
		"_handle.destroy()",
		"_restoreNativeNodes(_cy)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34a-html-overlay (post-D37r-tranche-B'): destroy() must include %q", want)
		}
	}
	// The pre-flip in-Authority mechanics are gone — listener
	// detach / rAF cancel / layer-DIV removal now live in the
	// shared module.
	sharedJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	for _, want := range []string{
		"cy.off(SYNC_EVENTS",
		"cy.off('select', 'node',",
		"cy.off('mouseover', 'node',",
		"_resizeObs.disconnect()",
		"_layerEl.parentNode.removeChild(_layerEl)",
	} {
		if !strings.Contains(sharedJS, want) {
			t.Errorf("D34a-html-overlay (post-D37r-tranche-B'): shared overlay module destroy path must include %q", want)
		}
	}
}

// TestExplorer_D34aHtmlOverlay_CardClickRoutesThroughRendererHook
// pins that a card click triggers the same production-drawer
// dispatch as a native cy tap — both go through
// `_rendererHooks.selectNode(nodeId)`.
func TestExplorer_D34aHtmlOverlay_CardClickRoutesThroughRendererHook(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")

	startIdx := strings.Index(js, "function _wireCardClick(card, nodeId) {")
	if startIdx < 0 {
		t.Fatal("D34a-html-overlay: _wireCardClick function definition missing")
	}
	endIdx := strings.Index(js[startIdx:], "\n  }")
	if endIdx < 0 {
		t.Fatal("D34a-html-overlay: cannot bound _wireCardClick body")
	}
	body := js[startIdx : startIdx+endIdx]
	for _, want := range []string{
		"card.addEventListener('click'",
		"_cy.elements().unselect()",
		"n.select()",
		"h.selectNode(nodeId)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34a-html-overlay: card click handler must include %q", want)
		}
	}
}

// TestExplorer_D34aHtmlOverlay_ContextGraphUntouched pins that the
// spike does NOT alter any Context Graph code. The overlay module
// lives under the authority/ tree and is loaded after the PoC, but
// neither the context-graph-view module's source nor its asset URL
// has been touched by this tranche.
func TestExplorer_D34aHtmlOverlay_ContextGraphUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	// Context Graph view module still serves and still registers
	// its own renderer hook. No `cytoscapeHtmlOverlay` reference
	// inside it.
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if strings.Contains(js, "cytoscapeHtmlOverlay") {
		t.Error("D34a-html-overlay: Context Graph view must NOT reference the spike's overlay module")
	}
	if !strings.Contains(js, "renderer.addNode") {
		t.Error("D34a-html-overlay: Context Graph view must keep its native renderer.addNode path")
	}
}

// ── D34b-context-cytoscape-html-overlay-card-parity-spike tests ──────

// TestExplorer_D34bContextOverlay_ModuleServed pins the spike asset
// is served at the documented path, exposes its public surface, and
// is linked into index.html AFTER the production Context view (so
// the spike's observer wires up on the rendered scene rather than
// racing it).
func TestExplorer_D34bContextOverlay_ModuleServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	for _, want := range []string{
		"window.MIDASExplorerGraph.contextCytoscapeOverlaySpike",
		"isActive:",
		"install:",
		"destroy:",
		// The two-flag gate.
		"sp.get('cytoscape') === '1' && sp.get('contextHtmlCards') === '1'",
		// Observer + install pattern.
		"MutationObserver",
		"_observeScene",
		"function install(",
		"function destroy()",
		// D34i — Sync wiring is split into two tiers: pan/zoom →
		// LAYER_SYNC_EVENTS, position/select → CARDS_SYNC_EVENTS.
		// Each is bound to its own rAF-coalesced handler.
		"cy.on(LAYER_SYNC_EVENTS",
		"cy.on(CARDS_SYNC_EVENTS",
		// The historical union string is still defined as
		// `SYNC_EVENTS` for back-compat; the substring below
		// matches that constant.
		"render pan zoom position layoutstop",
		"requestAnimationFrame",
		"ResizeObserver",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-context-overlay: module missing %q", want)
		}
	}

	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	ctxIdx    := strings.Index(body, `src="/explorer/assets/js/graph/context/context-graph-view.js"`)
	spikeIdx  := strings.Index(body, `src="/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js"`)
	if ctxIdx < 0 {
		t.Fatal("D34b-context-overlay: context-graph-view.js script tag missing from index.html")
	}
	if spikeIdx < 0 {
		t.Fatal("D34b-context-overlay: context-cytoscape-overlay-spike.js script tag missing from index.html")
	}
	if ctxIdx > spikeIdx {
		t.Error("D34b-context-overlay: spike script must load AFTER context-graph-view.js")
	}
	if !strings.Contains(body, `href="/explorer/assets/css/context-cytoscape-overlay-spike.css"`) {
		t.Error("D34b-context-overlay: context-cytoscape-overlay-spike.css <link> missing from index.html")
	}
}

// TestExplorer_D34bContextOverlay_GatingFlag pins the spike's self-
// gate: when `?cytoscape=1` is absent OR `?contextHtmlCards=1` is
// absent, the module's IIFE early-returns and only the `isActive`
// surface is exposed. Loading the script with the gate closed must
// not run install/destroy logic.
func TestExplorer_D34bContextOverlay_GatingFlag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// The IIFE must guard with `if (!_isActive())`.
	if !strings.Contains(js, "if (!_isActive()) {") {
		t.Error("D34b-context-overlay: IIFE must guard with `if (!_isActive())`")
	}
	// When the gate is closed, the module exposes only `isActive` on
	// the public surface — full install/destroy is only wired when
	// active.
	if !strings.Contains(js, "isActive: _isActive,\n    };\n    return;") {
		t.Error("D34b-context-overlay: closed-gate path must expose only isActive and return")
	}
}

// TestExplorer_D34bContextOverlay_NoThirdPartyDependency pins the
// MIDAS-owned constraint: no imports, no third-party extension
// method calls, no CDN URLs.
func TestExplorer_D34bContextOverlay_NoThirdPartyDependency(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, banned := range []string{
		"import ",
		"require(",
		"unpkg.com",
		"cdn.jsdelivr.net",
		"cy.nodeHtmlLabel",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D34b-context-overlay: module must remain MIDAS-owned — found third-party signal %q", banned)
		}
	}
}

// TestExplorer_D34bContextOverlay_CardParityViaDomClone pins the
// exact-parity strategy: the spike CLONES already-rendered
// `.gmap-node` elements out of `#gmap-scene` rather than rebuilding
// them. This is the only way to guarantee bit-for-bit card visual
// parity (width, padding, accent strip, typography, badges, hover/
// focus/selected state) without duplicating CSS values.
func TestExplorer_D34bContextOverlay_CardParityViaDomClone(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		// The capture queries the Context renderer's marker class.
		".querySelectorAll('.gmap-node[data-node-id]')",
		// The cloning helper exists and uses deep cloneNode(true).
		"function _cloneCard(",
		"srcEl.cloneNode(true)",
		// The clone is reset for transform-driven positioning.
		"clone.style.left = '0';",
		"clone.style.top  = '0';",
		"clone.style.transform =",
		// The mirrored selected-state hook reuses the existing
		// MIDAS `.selected` CSS hook (no new class invented).
		"card.classList.add('selected')",
		"card.classList.remove('selected')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-context-overlay: card-parity clone contract missing %q", want)
		}
	}

	// The spike's CSS must NOT duplicate any production card
	// dimensions / typography / accent values — every visual rule
	// must come from the production `.gmap-node` CSS in
	// governance-map.css.
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	for _, banned := range []string{
		// These are the kinds of declarations whose values must
		// stay in the production stylesheet, not be duplicated
		// (and thus drifted) here.
		"width: 200px;",
		"width: 220px;",
		"width: 240px;",
		"font-family:",
		"font-size: 13px",
		"font-size: 14px",
		"border-radius: 6px;",
		"border-radius: 8px;",
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D34b-context-overlay: spike CSS must NOT duplicate production card values — found %q", banned)
		}
	}
	// Positive pin: the spike CSS reuses `.gmap-node` as the card
	// class (no new card class invented).
	if !strings.Contains(css, ".gmap-node") {
		t.Error("D34b-context-overlay: spike CSS must reuse the production `.gmap-node` class for card parity")
	}
}

// TestExplorer_D34bContextOverlay_TextContentAndEscape pins the
// security contract. Even though the spike's primary visual path is
// DOM cloning (which inherits user data already escaped by the
// production renderer), the small DOM that the spike constructs
// itself must use textContent / DOM-creation, and a local escape
// helper must exist.
func TestExplorer_D34bContextOverlay_TextContentAndEscape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _escHtml(s)",
		"replace(/&/g, '&amp;')",
		"replace(/</g, '&lt;')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-context-overlay: local HTML-escape helper missing %q", want)
		}
	}
	// Negative pin: no innerHTML interpolation with user data. The
	// spike legitimately reads `el.style.left` etc. (DOM property
	// access — never user input), but must not assign to .innerHTML.
	if strings.Contains(js, ".innerHTML =") {
		t.Error("D34b-context-overlay: module must not assign innerHTML — clone + DOM-creation only")
	}
}

// TestExplorer_D34bContextOverlay_ClickRoutesThroughHook pins the
// selection-routing contract.
//
// D34h-context-cytoscape-native-graph-management — the contract
// shape moved from a per-card click handler to a cy-native tap
// delegation. Mouse selection routes through `cy.on('tap', 'node',
// …)` inside `_wireCytoscapeInteraction`; keyboard activation
// routes through `_wireCardKeyboardActivation` which fires only on
// Enter/Space (pointer-events:none on cards blocks mouse clicks
// from reaching the card). BOTH paths converge on the same
// `_rendererHooks.selectNode(id)` hook so the right drawer
// updates identically.
func TestExplorer_D34bContextOverlay_ClickRoutesThroughHook(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Mouse path: cy tap delegation inside `_wireCytoscapeInteraction`.
	cyStart := strings.Index(js, "function _wireCytoscapeInteraction(cy)")
	if cyStart < 0 {
		t.Fatal("D34h: _wireCytoscapeInteraction function definition missing")
	}
	cyEnd := strings.Index(js[cyStart:], "\n  }")
	if cyEnd < 0 {
		t.Fatal("D34h: cannot bound _wireCytoscapeInteraction body")
	}
	cyBody := js[cyStart : cyStart+cyEnd]
	for _, want := range []string{
		"cy.on('tap', 'node'",
		"cy.elements().unselect()",
		"n.select()",
		"h.selectNode(id)",
	} {
		if !strings.Contains(cyBody, want) {
			t.Errorf("D34h: cy-tap selection wiring must include %q", want)
		}
	}

	// Keyboard path: `_wireCardKeyboardActivation` must still route
	// to the same hook so Tab + Enter/Space continues to work.
	kbStart := strings.Index(js, "function _wireCardKeyboardActivation(card, nodeId)")
	if kbStart < 0 {
		t.Fatal("D34h: _wireCardKeyboardActivation function definition missing")
	}
	kbEnd := strings.Index(js[kbStart:], "\n  }")
	if kbEnd < 0 {
		t.Fatal("D34h: cannot bound _wireCardKeyboardActivation body")
	}
	kbBody := js[kbStart : kbStart+kbEnd]
	for _, want := range []string{
		"card.addEventListener('click'",
		"card.addEventListener('keydown'",
		"_cy.elements().unselect()",
		"n.select()",
		"h.selectNode(nodeId)",
	} {
		if !strings.Contains(kbBody, want) {
			t.Errorf("D34h: keyboard-activation handler must include %q", want)
		}
	}

	// Negative pin: the predecessor `_wireCardClick` is GONE.
	if strings.Contains(js, "function _wireCardClick(") {
		t.Error("D34h: _wireCardClick must be removed — selection lives in cy tap delegation now")
	}
}

// TestExplorer_D34bContextOverlay_DragUpdatesCyPosition pins that
// dragging a card moves the underlying cy node — but as of
// D34h-context-cytoscape-native-graph-management this is achieved
// by Cytoscape's NATIVE node-drag, not by a custom DOM pointer
// handler. Cards are pointer-events: none; pointerdown reaches
// cy's canvas; cy's hit-test maps to the node (whose bounding box
// equals the card footprint via `data(cardWidth/cardHeight)`); cy
// drags the node; cy fires `position`; the overlay re-syncs.
//
// What this test verifies:
//   • The custom DOM drag function `_wireCardDrag` is GONE.
//   • Cy nodes default to `grabbable: true` (cy's default; we do
//     NOT pass `autoungrabify: true` — that flag never appears in
//     the source).
//   • The position event is in SYNC_EVENTS so cards follow native
//     drag in real time.
func TestExplorer_D34bContextOverlay_DragUpdatesCyPosition(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Negative pins — the OLD DOM drag path is gone. Strip comments
	// first so the spike module's own documentation referencing the
	// retired tokens (e.g. the header comment listing removals)
	// does not trip the assertion.
	exec := stripJSComments(js)
	for _, banned := range []string{
		"function _wireCardDrag(",
		"DRAG_THRESHOLD_PX",
		"dragSet[di].node.position(",
		"document.addEventListener('pointermove'",
		"document.addEventListener('pointerup'",
		"setPointerCapture(ev.pointerId)",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D34h: DOM drag artefact must be removed — found %q", banned)
		}
	}

	// Positive pins — native cy drag prerequisites.
	//   • SYNC_EVENTS includes `position` so cards follow real-time.
	//   • No `autoungrabify` configuration — cy nodes stay grabbable.
	if !strings.Contains(js, "render pan zoom position layoutstop") {
		t.Error("D34h: SYNC_EVENTS must include `position` so the overlay follows native cy drag")
	}
	if strings.Contains(js, "autoungrabify: true") {
		t.Error("D34h: cy must NOT disable node grabbing (autoungrabify must stay default false)")
	}
}

// TestExplorer_D34bContextOverlay_DestroyRemovesDomAndListeners pins
// the lifecycle contract.
func TestExplorer_D34bContextOverlay_DestroyRemovesDomAndListeners(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// D35e — Destroy work moved into `_teardownResources()`
	// (extracted helper). Public `destroy()` routes through
	// `viewport.deactivate()` when host owns activation, else
	// calls `_teardownResources()` directly. The cleanup contract
	// (rAF cancellation, cy.off, ResizeObserver disconnect, window
	// removeEventListener, cy.destroy, mount removal) all lives in
	// `_teardownResources`. Body-class removal stays in the public
	// `destroy()` boundary.
	trStart := strings.Index(js, "function _teardownResources()")
	if trStart < 0 {
		t.Fatal("D35e: _teardownResources definition missing")
	}
	trEnd := strings.Index(js[trStart:], "\n  }\n")
	if trEnd < 0 {
		t.Fatal("D35e: cannot bound _teardownResources body")
	}
	body := js[trStart : trStart+trEnd]
	for _, want := range []string{
		// D34i — two-tier rAF cancellation + two-tier listener off.
		"cancelAnimationFrame(_syncLayerRaf)",
		"cancelAnimationFrame(_syncCardsRaf)",
		"_cy.off(LAYER_SYNC_EVENTS",
		"_cy.off(CARDS_SYNC_EVENTS",
		"_resizeObs.disconnect()",
		"window.removeEventListener('resize'",
		"_cy.destroy()",
		"_mountEl.parentNode.removeChild(_mountEl)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34b-context-overlay: _teardownResources must include %q", want)
		}
	}

	// D35f-retire-transitional-renderer-debt: the body-class
	// removal from `destroy()` was retired. Renderer identity is
	// now host-owned via `data-active-renderer` and the host's
	// `viewport.deactivate()` clears/restores it. We pin that
	// `destroy()` no longer flips the body class.
	deStart := strings.Index(js, "function destroy() {")
	if deStart < 0 {
		t.Fatal("D35f: destroy definition missing")
	}
	deEnd := strings.Index(js[deStart:], "\n  }\n")
	if deEnd < 0 {
		t.Fatal("D35f: cannot bound destroy body")
	}
	deBody := js[deStart : deStart+deEnd]
	if strings.Contains(deBody, "classList.remove(BODY_FLAG_CLASS)") {
		t.Error("D35f: destroy() must NOT flip body class (host owns renderer identity via data-active-renderer)")
	}
}

// TestExplorer_D34bContextOverlay_ContextRendererPreserved pins that
// the spike does NOT modify the production Context renderer in any
// way. The renderer module must still serve, still call
// `renderer.addNode`, and must NOT reference the spike module.
func TestExplorer_D34bContextOverlay_ContextRendererPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(js, "renderer.addNode") {
		t.Error("D34b-context-overlay: production Context view must keep its renderer.addNode path")
	}
	if strings.Contains(js, "contextCytoscapeOverlaySpike") {
		t.Error("D34b-context-overlay: production Context view must NOT reference the spike module")
	}
	// And graph-renderer.js itself is untouched.
	rendererJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")
	if strings.Contains(rendererJS, "contextCytoscapeOverlaySpike") {
		t.Error("D34b-context-overlay: graph-renderer.js must NOT reference the spike module")
	}
}

// TestExplorer_D34bContextOverlay_BodyFlagAndScenesHidden pins the
// CSS contract: when `body.context-cy-spike-active` is set, the
// production `#gmap-scene` + `#gmap-svg` are hidden (visibility),
// and the spike's mount + overlay layers are positioned correctly
// inside `#gmap-canvas`.
//
// D34h-context-cytoscape-native-graph-management — pointer-events
// on cards FLIPPED from `auto` to `none`. Cytoscape now owns
// pointer interaction; pointer events must pass THROUGH the card
// to cy's canvas underneath. The card-level `pointer-events: auto`
// is intentionally absent.
func TestExplorer_D34bContextOverlay_BodyFlagAndScenesHidden(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	// D34k — The hide moved from `#gmap-scene + #gmap-svg
	// visibility:hidden` to `#gmap-canvas display:none`. With the
	// canvas removed from layout, the scene + svg (its descendants)
	// are removed too — no need to hide them individually, and the
	// `visibility: hidden` rule is gone.
	for _, want := range []string{
		`.midas-graph-viewport[data-active-renderer="context-cytoscape"] #gmap-canvas`,
		"display: none !important",
		".context-cy-spike-mount",
		".context-cy-spike-overlay",
		"pointer-events: none",
		// Cards stay pointer-transparent so cy receives interaction.
		".context-cy-spike-overlay .gmap-node",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D34k: spike CSS missing %q", want)
		}
	}

	// D34h — Negative pin: NO `pointer-events: auto` anywhere in the
	// spike CSS (would re-enable card pointer capture and re-break
	// the native cy interaction model). Comments are stripped first
	// so explanatory prose mentioning the old value is ignored.
	exec := stripCSSComments(css)
	if strings.Contains(exec, "pointer-events: auto") {
		t.Error("D34h: spike CSS must NOT set `pointer-events: auto` anywhere — cy owns pointer interaction")
	}
}

// ── D34b-fix-context-html-overlay-activation contract tests ─────────

// TestExplorer_D34bFix_BodyFlagDeferredUntilInstallSucceeds — the
// underlying invariant ("the spike's renderer-state flag is not set
// speculatively at module load — only on successful install, and
// cleared on destroy") survives in D35f, but the implementation
// no longer uses a body class. D35f moved renderer-state ownership
// to the GraphViewport host's `data-active-renderer` attribute on
// `.midas-graph-viewport`, set by `viewport.activate(...)` and
// cleared by `viewport.deactivate()`. The spike no longer flips a
// body class in install/destroy.
//
// This test now pins the post-D35f contract:
//   • The spike must NOT add `context-cy-spike-active` at module
//     load (eager-add is forbidden).
//   • The spike must NOT add `context-cy-spike-active` inside
//     `install()` (host owns activation).
//   • The spike must NOT remove `context-cy-spike-active` inside
//     `destroy()` (host owns deactivation).
//   • Install still routes through the host (D35e contract).
//     D35g changed the call shape from
//     `viewport.activate(id, factory)` to
//     `viewport.activateById(id)` (the factory is registered with
//     the host at module init via `viewport.register`).
func TestExplorer_D34bFix_BodyFlagDeferredUntilInstallSucceeds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Negative pin: NO body-class flip anywhere in the spike module.
	// D35f retired body-class renderer identity entirely.
	exec := stripJSComments(js)
	for _, banned := range []string{
		"document.body.classList.add(BODY_FLAG_CLASS)",
		"document.body.classList.remove(BODY_FLAG_CLASS)",
		"document.body.classList.add('context-cy-spike-active')",
		"document.body.classList.remove('context-cy-spike-active')",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D35f: spike module must NOT flip body class — found %q (renderer identity is owned by the GraphViewport host's data-active-renderer attribute)", banned)
		}
	}

	// Positive pin: install() still routes through the host. This
	// is the strategic activation point that REPLACES the body-class
	// flip. D35g switched the host call shape from
	// `vp.activate(id, factory)` to `vp.activateById(id)` (factories
	// are registered with the host at module init).
	if !strings.Contains(js, "vp.activateById('context-cytoscape')") {
		t.Error("D35f/D35g: install() must route activation through viewport.activateById('context-cytoscape')")
	}
	if !strings.Contains(js, "vp.register('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35f/D35g: spike must register its factory with the host (vp.register)")
	}
}

// TestExplorer_D34bFix_LensGuardedInstall pins that `install()`
// checks the active lens and bails (returns false) when the lens
// is set to something other than Context. Authority lens, in
// particular, must not trigger the spike — even though Authority's
// PoC paints `.gmap-node` carrier elements, those live under
// `#gmap-canvas`, never under `#gmap-scene`. The lens guard is
// belt-and-braces against a future regression that might widen
// the carrier surface.
func TestExplorer_D34bFix_LensGuardedInstall(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _activeLens()",
		"MIDASExplorerStore.getState()",
		"s.selectedGraphLens",
		// install() bails when lens is set and not 'context'. The
		// guard's body grew in D34b-browser-diagnostic to write the
		// `_lastInstallReason` token + emit a debug log before
		// returning false, so the pin checks the guard opener now.
		"if (lens && lens !== 'context') {",
		`_lastInstallReason = 'lens-not-context:' + lens;`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-fix: lens-guarded install contract missing %q", want)
		}
	}
}

// TestExplorer_D34bFix_StoreSubscriptionDrivesInstallAndDestroy pins
// the canonical activation hook: the spike subscribes to
// `MIDASExplorerStore` for `selectedGraphLens` changes, schedules
// install when the lens transitions to Context, and destroys the
// overlay when the lens transitions away.
func TestExplorer_D34bFix_StoreSubscriptionDrivesInstallAndDestroy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _onStoreChange(state)",
		"MIDASExplorerStore.subscribe(_onStoreChange)",
		// Install branch.
		"if (lens === 'context') {",
		"_scheduleInstall()",
		// Destroy branch.
		"} else if (lens && _cy) {",
		"destroy();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-fix: store-subscription activation contract missing %q", want)
		}
	}
}

// TestExplorer_D34bFix_InstallReturnsFalseWhenNoCards pins that
// install() returns false (and DOES NOT add the body flag) when
// the scene has no `.gmap-node` elements. This is the critical
// safety property — install never hides the production Context
// Graph speculatively.
func TestExplorer_D34bFix_InstallReturnsFalseWhenNoCards(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// D35e-port-context-cytoscape-to-graphviewport: the install
	// pipeline split into a public `install()` (host routing +
	// body-class flip) and `_installResources(parentEl)` (the
	// actual install body, including the empty-specs guard).
	//
	// The CONTRACT this test enforces is unchanged: empty specs
	// MUST short-circuit BEFORE the body class is added. The
	// architecture that satisfies the contract now spans two
	// functions:
	//   1. `_installResources` returns false on `specs.length === 0`
	//      with `_lastInstallReason = 'no-cards'`.
	//   2. Public `install()` only adds the body class if both the
	//      host-routed activation AND the legacy fallback returned
	//      true (`if (!ok) return false;` precedes the body-class
	//      add).
	resourcesStart := strings.Index(js, "function _installResources(parentEl)")
	if resourcesStart < 0 {
		t.Fatal("D35e: _installResources function definition missing")
	}
	resourcesEnd := strings.Index(js[resourcesStart:], "\n  }\n")
	if resourcesEnd < 0 {
		t.Fatal("D35e: cannot bound _installResources body")
	}
	resourcesBody := js[resourcesStart : resourcesStart+resourcesEnd]
	if !strings.Contains(resourcesBody, "if (specs.length === 0) {") {
		t.Error("D35e: _installResources must guard `specs.length === 0` with an early return")
	}
	emptyIdx := strings.Index(resourcesBody, "if (specs.length === 0) {")
	if emptyIdx < 0 {
		t.Fatal("D35e: empty-specs guard not located in _installResources")
	}
	guardSlice := resourcesBody[emptyIdx:]
	if len(guardSlice) > 500 {
		guardSlice = guardSlice[:500]
	}
	if !strings.Contains(guardSlice, "return false;") {
		t.Error("D34b-fix: empty-specs guard must `return false` so the scene observer can retry")
	}
	if !strings.Contains(guardSlice, `_lastInstallReason = 'no-cards'`) {
		t.Error("D34b-debug: empty-specs guard must write _lastInstallReason='no-cards'")
	}

	// Public `install()` returns false on `_installResources`
	// failure. D35f retired the body-class flip from install(), so
	// the underlying invariant ("no premature hide") is now
	// expressed as: `if (!ok) return false;` is the last guard
	// before `return true;`. We pin both presence and ordering.
	installStart := strings.Index(js, "function install(options) {")
	if installStart < 0 {
		t.Fatal("D35f: install function definition missing")
	}
	installEnd := strings.Index(js[installStart:], "\n  }\n")
	if installEnd < 0 {
		t.Fatal("D35f: cannot bound install body")
	}
	installBody := js[installStart : installStart+installEnd]
	// D35f — guard is multi-line: `if (!ok) { … return false; }`.
	failIdx := strings.LastIndex(installBody, "if (!ok) {")
	retIdx := strings.LastIndex(installBody, "return true;")
	if failIdx < 0 || retIdx < 0 || failIdx >= retIdx {
		t.Error("D35f: install() must guard `if (!ok) { ... return false; }` BEFORE `return true;` (no premature success)")
	}
}

// TestExplorer_D34bFix_BodyObserverFallback pins the robust
// activation: when `#gmap-scene` is not present at bootstrap
// (catalogue view, deferred renderer), a body-scoped
// MutationObserver fires when the element appears and triggers a
// fresh install attempt. The observer disconnects after the scene
// shows up so it doesn't spin perpetually.
func TestExplorer_D34bFix_BodyObserverFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _ensureBodyObserver()",
		"_bodyObserver.observe(document.body, { childList: true, subtree: true })",
		"document.getElementById('gmap-scene')",
		// Auto-disconnect on success so the observer doesn't leak.
		"_bodyObserver.disconnect()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-fix: body-observer fallback contract missing %q", want)
		}
	}
}

// TestExplorer_D34bFix_ReinstallOnServiceSwitch pins the service-
// switch contract: when the spike is already installed and the
// scene observer fires with a `.gmap-node` id set that no longer
// matches the captured set, the spike destroys the current
// overlay and schedules a fresh install. This is what makes
// "switch back to Context Graph" work after a service change.
func TestExplorer_D34bFix_ReinstallOnServiceSwitch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"_capturedIds",
		// Mismatch detection in the scene-observer schedule.
		"if (_cy && _capturedIds) {",
		"if (mismatch) {",
		"destroy();",
		"_scheduleInstall();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-fix: service-switch reinstall contract missing %q", want)
		}
	}
}

// ── D34b-browser-diagnostic contract tests ───────────────────────────

// TestExplorer_D34bDebug_DebugStateExposed pins the `debugState()`
// DevTools probe entry on the open-gate public surface. The probe
// is the only way to inspect activation state from the browser
// without source-level breakpoints.
func TestExplorer_D34bDebug_DebugStateExposed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _debugState()",
		"debugState: _debugState,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-debug: debugState surface missing %q", want)
		}
	}
}

// TestExplorer_D34bDebug_DebugStateReturnsRequiredKeys pins every
// signal the brief required: gate flag, lens state, scene presence
// + node count, svg presence + connector count, spike body class,
// cy mount + overlay layer + card count, last install status +
// reason, scene/svg computed visibility.
func TestExplorer_D34bDebug_DebugStateReturnsRequiredKeys(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Bound the _debugState body so we don't accidentally match
	// these keys elsewhere.
	start := strings.Index(js, "function _debugState()")
	if start < 0 {
		t.Fatal("D34b-debug: _debugState function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34b-debug: cannot bound _debugState body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		"activeFlag:",
		"selectedGraphLens:",
		"sceneExists:",
		"sceneNodeCount:",
		"svgExists:",
		"connectorCount:",
		"spikeBodyClassPresent:",
		"cyMountExists:",
		"overlayLayerExists:",
		"overlayCardCount:",
		"lastInstallStatus:",
		"lastInstallReason:",
		"sceneVisibility:",
		"svgVisibility:",
		// Additional signals beyond the brief's minimum — useful
		// for distinguishing "bootstrap never ran" from
		// "bootstrap ran but observer didn't fire".
		"storeAvailable:",
		"storeSubscribed:",
		"installAttemptCount:",
		"bodyObserverActive:",
		"sceneObserverActive:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34b-debug: debugState() must return key %q", want)
		}
	}
}

// TestExplorer_D34bDebug_InstallReasonsTracked pins that every
// install bail path writes a specific `_lastInstallReason` token so
// `debugState().lastInstallReason` identifies exactly which stage
// failed. The reasons must cover every failure mode the brief
// listed (wrong lens / scene missing / zero cards / Cytoscape
// unavailable / capture failure).
func TestExplorer_D34bDebug_InstallReasonsTracked(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		`_lastInstallReason = 'lens-not-context:'`,
		`_lastInstallReason = 'scene-missing'`,
		`_lastInstallReason = 'no-cards'`,
		// D35f-retire-transitional-renderer-debt — the legacy
		// fallback (which used `.governance-map-canvas-scroll` as a
		// mount parent and emitted `'no-scroll-wrapper'` on failure)
		// has been removed. The strategic path goes through the
		// GraphViewport host; if the host fails, the new reason is
		// `'host-unavailable'` (set via `_lastInstallReason || ...`).
		`'host-unavailable'`,
		`_lastInstallReason = 'cytoscape-unavailable'`,
		// Install resources can fail with a missing mount parent
		// (when the host returns no slot).
		`_lastInstallReason = 'no-mount-parent'`,
		// Success path must clear status + reason.
		`_lastInstallStatus = 'success'`,
		`_lastInstallReason = ''`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-debug: install-reason tracking missing %q", want)
		}
	}
}

// ── D34b-fix-overlay-installed-but-invisible contract tests ──────────

// TestExplorer_D34bInvisFix_MountInlineSizedBeforeCyInit pins the
// fix for the 0×0 mount race. The CSS rule that sizes the mount is
// keyed on `body.context-cy-spike-active`, but the body class is
// added only AFTER install succeeds. Cytoscape captures container
// dimensions at init time, so without inline mount styles cy would
// read 0×0 and `renderedPosition()` would collapse to the origin.
// The fix: set inline `position:absolute` + edge insets on the
// mount BEFORE `_buildCytoscape` is called.
// TestExplorer_D34bInvisFix_MountInlineSizedBeforeCyInit — RETIRED.
//
// D34k-context-cytoscape-authority-mount-pattern moved the spike
// off the `#gmap-canvas`-mounted, inline-absolute-sized pattern
// onto Authority Cytoscape's in-flow mount pattern. CSS now owns
// the mount geometry (`position: relative; width/height: 100%;
// min-height: 480px`), and the inline `position/left/top/right/
// bottom = '0'` block is gone.
//
// Coverage moved to `TestExplorer_D34kMount_AppendedToScrollWrapper`
// (asserts the mount is appended to `.governance-map-canvas-scroll`)
// and `TestExplorer_D34kCSS_MountGeometry` (asserts the in-flow
// geometry rule).
func TestExplorer_D34bInvisFix_MountInlineSizedBeforeCyInit(t *testing.T) {
	t.Skip("D34k: retired — mount geometry moved from inline absolute to CSS in-flow. See TestExplorer_D34kCSS_MountGeometry.")
}

// TestExplorer_D34bInvisFix_SettlePatternAfterBodyClass pins the
// settle pattern that runs after the body class is added: cy.resize()
// re-reads the container, cy.fit() recomputes the layout, and the
// rAF tick re-syncs cards. This catches any residual race where cy
// captured stale dimensions during initial install.
func TestExplorer_D34bInvisFix_SettlePatternAfterBodyClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// D35e — Pre-D35e: install() flipped the body class then ran
	// the settle pattern inline. Post-D35e: install() splits into
	// `_installResources(parentEl)` (does the settle pattern) and a
	// public boundary that flips the body class AFTER
	// `_installResources` returns true. The architecture preserves
	// the invariant — settle's deferred ticks (rAF + 120ms timeout)
	// fire AFTER the synchronous body-class flip completes — but
	// the two halves live in different functions.
	//
	// We pin:
	//   (a) The settle pattern lives in `_installResources` and
	//       includes function _settle + cy.resize + _sync +
	//       requestAnimationFrame + 120ms timeout.
	//   (b) `_installResources` is called from `install()` BEFORE
	//       the body-class flip (so `_installResources` schedules
	//       settle, then `install()` flips the class, then settle
	//       fires).
	installResourcesStart := strings.Index(js, "function _installResources(parentEl)")
	if installResourcesStart < 0 {
		t.Fatal("D35e: _installResources definition missing")
	}
	installResourcesEnd := strings.Index(js[installResourcesStart:], "\n  }\n")
	if installResourcesEnd < 0 {
		t.Fatal("D35e: cannot bound _installResources body")
	}
	body := js[installResourcesStart : installResourcesStart+installResourcesEnd]
	for _, want := range []string{
		"function _settle()",
		"_cy.resize();",
		"_sync()",
		"window.requestAnimationFrame(",
		"setTimeout(_settle, 120)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34b-invis-fix: settle pattern in _installResources must include %q", want)
		}
	}
	// D35e — Padding now composed via fitPadding from _midasFit
	// Padding() floor and host safe-area ceiling.
	if !strings.Contains(body, "_cy.fit(undefined, fitPadding)") {
		t.Error("D34b-invis-fix: settle pattern must call _cy.fit(undefined, fitPadding) (D35e-composed)")
	}
	if !strings.Contains(body, "_midasFitPadding()") {
		t.Error("D34b-invis-fix: settle pattern must source the MIDAS padding floor")
	}

	// (b) D35f — Body-class flip RETIRED. The strategic ordering
	// invariant ("install resources before any state-flip side
	// effect") is now expressed via the host's `viewport.activate`
	// boundary: factory.mount calls _installResources, then the
	// host sets the data-active-renderer attribute on success. The
	// public install() function itself contains only the host-
	// activation call followed by the `if (!ok) return false;`
	// guard and `return true;`.
	installStart := strings.Index(js, "function install(options) {")
	if installStart < 0 {
		t.Fatal("D35f: install definition missing")
	}
	installEnd := strings.Index(js[installStart:], "\n  }\n")
	if installEnd < 0 {
		t.Fatal("D35f: cannot bound install body")
	}
	installBody := js[installStart : installStart+installEnd]
	if !strings.Contains(installBody, "vp.activateById('context-cytoscape')") {
		t.Error("D35f/D35g: install() must route through viewport.activateById('context-cytoscape') (host owns activation; D35g registry-based)")
	}
	if strings.Contains(installBody, "vp.activate('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35g: install() must NOT pass the factory directly to vp.activate — use vp.activateById (factory registered at module init)")
	}
	// Negative — no body-class flip in install() any more.
	if strings.Contains(installBody, "document.body.classList.add(BODY_FLAG_CLASS)") {
		t.Error("D35f: install() must NOT add a body class (host owns renderer identity via data-active-renderer)")
	}
}

// TestExplorer_D34bInvisFix_OverridesAuthorityPocCanvasHide pins
// the canvas-hide rule.
//
// D34k-context-cytoscape-authority-mount-pattern: the spike now
// ADOPTS Authority's hide (rather than overriding it). The spike
// CSS still scopes its rule to `body.context-cy-spike-active` so
// destroy() reverts to whatever Authority's rule was — but the
// declaration is now `display: none !important`, mirroring
// Authority's rule for the Context lens. The pin protects:
//   • Authority's rule still exists with `display: none !important`
//     (so Authority lens behaviour is preserved when the spike
//     gate is closed);
//   • the spike CSS declares the same `display: none !important`
//     on the same `#gmap-canvas` selector scoped to the spike's
//     body class;
//   • the spike stylesheet remains linked AFTER Authority's so the
//     cascade behaves identically across spike active/inactive.
func TestExplorer_D34bInvisFix_OverridesAuthorityPocCanvasHide(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// 1. Authority PoC's hide rule still exists. D35f-retire-
	//    transitional-renderer-debt: the gate moved from
	//    `body.cytoscape-poc-active` to
	//    `.midas-graph-viewport[data-active-renderer="authority-
	//    cytoscape"]`. The rule still hides `#gmap-canvas`; only the
	//    activation key changed.
	pocCSS := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	if !strings.Contains(pocCSS, `.midas-graph-viewport[data-active-renderer="authority"] #gmap-canvas`) {
		t.Error("D35f: Authority PoC CSS must scope its #gmap-canvas hide to the host renderer-identity selector")
	}
	if !strings.Contains(pocCSS, "display: none !important;") {
		t.Error("D34k: Authority PoC CSS must still use `display: none !important` on #gmap-canvas")
	}

	// 2. Spike CSS declares the same hide for the Context lens.
	//    D35f: also re-keyed onto host renderer-identity selector.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if !strings.Contains(spikeCSS, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] #gmap-canvas`) {
		t.Error("D35f: spike CSS must scope a #gmap-canvas rule to the host renderer-identity selector")
	}
	spikeExec := stripCSSComments(spikeCSS)
	overrideIdx := strings.Index(spikeExec, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] #gmap-canvas`)
	if overrideIdx < 0 {
		t.Fatal("D34k: spike rule for #gmap-canvas missing (after comment strip)")
	}
	overrideEnd := strings.Index(spikeExec[overrideIdx:], "}")
	if overrideEnd < 0 {
		t.Fatal("D34k: cannot bound spike's #gmap-canvas rule body")
	}
	overrideBody := spikeExec[overrideIdx : overrideIdx+overrideEnd]
	if !strings.Contains(overrideBody, "display: none !important;") {
		t.Errorf("D34k: spike rule must declare `display: none !important` (Authority-style hide); got:\n%s", overrideBody)
	}
	// Negative pin — the OLD `display: block !important` override
	// must be GONE. If it crept back in, the spike would re-enter
	// the geometry-conflict failure mode that D34k retired.
	if strings.Contains(overrideBody, "display: block !important;") {
		t.Errorf("D34k: spike rule must NOT use `display: block !important` (that was the pre-D34k failure mode); got:\n%s", overrideBody)
	}

	// 3. Source order — D34k spike stylesheet is still linked AFTER
	//    authority-cytoscape-poc.css. With both rules declaring
	//    `display: none !important` on the same `#gmap-canvas` with
	//    equal specificity, the cascade is irrelevant for the Context
	//    lens — but the link order pin is kept so any future
	//    divergence is caught.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	pocIdx   := strings.Index(body, `href="/explorer/assets/css/authority-cytoscape-poc.css"`)
	spikeIdx := strings.Index(body, `href="/explorer/assets/css/context-cytoscape-overlay-spike.css"`)
	if pocIdx < 0 {
		t.Fatal("D34k: authority-cytoscape-poc.css <link> missing from index.html")
	}
	if spikeIdx < 0 {
		t.Fatal("D34k: context-cytoscape-overlay-spike.css <link> missing from index.html")
	}
	if pocIdx > spikeIdx {
		t.Error("D34k: context-cytoscape-overlay-spike.css must be linked AFTER authority-cytoscape-poc.css")
	}
}

// TestExplorer_D34bInvisFix_OverlayZIndexAboveCytoscape pins the
// stacking model.
//
// D34k-context-cytoscape-authority-mount-pattern: the overlay
// continues to sit at z-index 100 (above cy's internal canvases at
// z-index 0..3) — but the MOUNT no longer declares a high z-index.
// Instead, the mount uses `isolation: isolate` to create a local
// stacking context, so the overlay's z-index 100 is LOCAL to the
// mount and does not escape upward to compete with the body-level
// MIDAS chrome (`.gmap-legend-overlay`, `.gmap-mode-rail`,
// `.gmap-camera-cluster` at z-index 5 against `.governance-map-
// body`).
func TestExplorer_D34bInvisFix_OverlayZIndexAboveCytoscape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	// Find the overlay rule and assert z-index is at least 100.
	idx := strings.Index(css, ".context-cy-spike-overlay {")
	if idx < 0 {
		t.Fatal("D34b-invis-fix: .context-cy-spike-overlay rule missing")
	}
	end := strings.Index(css[idx:], "}")
	if end < 0 {
		t.Fatal("D34b-invis-fix: cannot bound .context-cy-spike-overlay rule body")
	}
	overlayBlock := css[idx : idx+end]
	if !strings.Contains(overlayBlock, "z-index: 100;") &&
		!strings.Contains(overlayBlock, "z-index: 1000;") {
		t.Errorf("D34b-invis-fix: overlay must declare an explicit high z-index (>=100); rule body was:\n%s", overlayBlock)
	}

	// D34k — Mount must NOT declare a non-auto z-index that would
	// escape to the parent stacking context and cover MIDAS body-
	// level chrome. The mount uses `isolation: isolate` to create
	// a stacking context without a z-index.
	mountIdx := strings.Index(css, ".context-cy-spike-mount {")
	if mountIdx < 0 {
		t.Fatal("D34k: .context-cy-spike-mount rule missing")
	}
	mountEnd := strings.Index(css[mountIdx:], "}")
	if mountEnd < 0 {
		t.Fatal("D34k: cannot bound .context-cy-spike-mount rule body")
	}
	mountBlock := css[mountIdx : mountIdx+mountEnd]
	if strings.Contains(mountBlock, "z-index: 3;") ||
		strings.Contains(mountBlock, "z-index: 10;") ||
		strings.Contains(mountBlock, "z-index: 100;") {
		t.Errorf("D34k: mount must NOT declare a high z-index — use `isolation: isolate` instead so the overlay's z-index 100 stays local; body was:\n%s", mountBlock)
	}
	if !strings.Contains(mountBlock, "isolation: isolate") {
		t.Errorf("D34k: mount must declare `isolation: isolate` to contain the overlay z-index; body was:\n%s", mountBlock)
	}
}

// TestExplorer_D34bInvisFix_OverlayVisibilityVisibleExplicit pins
// the defensive `visibility: visible !important` rule on the
// overlay layer + cards. This guarantees that no future CSS rule
// keyed on a parent class can cascade `hidden` down and invisibly
// kill the overlay.
func TestExplorer_D34bInvisFix_OverlayVisibilityVisibleExplicit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	for _, want := range []string{
		// On overlay layer.
		".context-cy-spike-overlay {",
		// On cards inside the overlay layer.
		".context-cy-spike-overlay .gmap-node {",
		// The explicit visibility:visible !important must appear at
		// least once in each block.
		"visibility: visible !important",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D34b-invis-fix: explicit-visibility contract missing %q", want)
		}
	}
	// Belt-and-braces — two occurrences of `visibility: visible
	// !important` (one for overlay layer, one for card).
	if got := strings.Count(css, "visibility: visible !important"); got < 2 {
		t.Errorf("D34b-invis-fix: expected `visibility: visible !important` on BOTH overlay + card rules; got %d occurrences", got)
	}
}

// TestExplorer_D34bInvisFix_HidingOnlyTargetsSceneAndSvg — RETIRED.
//
// D34k-context-cytoscape-authority-mount-pattern removed
// `visibility: hidden` from the spike CSS entirely. The hide moved
// from `#gmap-scene + #gmap-svg { visibility: hidden }` (which kept
// them in layout) to `#gmap-canvas { display: none !important }`
// (which removes them and all descendants from layout entirely).
//
// The scope-of-hidden invariant is now trivially satisfied:
// no `visibility: hidden` exists in the executable CSS, so it
// cannot target the wrong element. The new
// `TestExplorer_D34kCSS_NoVisibilityHidden` test pins this.
func TestExplorer_D34bInvisFix_HidingOnlyTargetsSceneAndSvg(t *testing.T) {
	t.Skip("D34k: retired — visibility:hidden replaced by display:none on #gmap-canvas. See TestExplorer_D34kCSS_NoVisibilityHidden.")
}

// TestExplorer_D34bInvisFix_NoCardStylingDuplication pins the
// non-duplication invariant. The spike's CSS must not redeclare
// any of the production `.gmap-node` card values (width, height,
// padding, border-radius, font-family, font-size, font-weight,
// color). All card styling must continue to flow from the
// production `governance-map.css`.
func TestExplorer_D34bInvisFix_NoCardStylingDuplication(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	for _, banned := range []string{
		// Card-dimension values.
		"width: 200px",
		"width: 220px",
		"width: 240px",
		"height: 80px",
		"height: 100px",
		"padding: 8px",
		"padding: 12px",
		// Card-typography values.
		"font-family:",
		"font-size: 13px",
		"font-size: 14px",
		"font-weight: 500",
		"font-weight: 600",
		// Card-shape values.
		"border-radius: 6px",
		"border-radius: 8px",
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D34b-invis-fix: spike CSS must NOT duplicate production card styling — found %q", banned)
		}
	}
}

// TestExplorer_D34bDebug_DebugStateExtendedKeys pins the new
// visibility/stacking diagnostic keys added by this tranche.
func TestExplorer_D34bDebug_DebugStateExtendedKeys(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugState()")
	if start < 0 {
		t.Fatal("D34b-debug-extended: _debugState function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34b-debug-extended: cannot bound _debugState body")
	}
	body := js[start : start+end]
	for _, want := range []string{
		"overlayLayerRect:",
		"overlayComputedZIndex:",
		"firstOverlayCardRect:",
		"firstOverlayCardComputedVisibility:",
		"firstOverlayCardComputedDisplay:",
		"firstOverlayCardComputedOpacity:",
		"firstOverlayCardTransform:",
		"cyContainerRect:",
		"cyPan:",
		"cyZoom:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34b-debug-extended: debugState() must return key %q", want)
		}
	}
}

// TestExplorer_D34bDebug_ConsoleDebugGatedToFlag pins the gated
// `console.debug` logging helper. The helper is reachable only
// from the open-gate path (the entire module's namespace replaces
// itself with a closed-gate stub when `?contextHtmlCards=1` is
// absent), so no console noise is produced when the spike is off.
func TestExplorer_D34bDebug_ConsoleDebugGatedToFlag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _debugLog(msg, extra)",
		"console.debug('[D34b]', msg",
		// Bootstrap + install + store-change all emit a debug line.
		"_debugLog('bootstrap:",
		"_debugLog('install",
		"_debugLog('store:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-debug: console.debug logging contract missing %q", want)
		}
	}
}

// TestExplorer_D34bFix_BootstrapKicksOffInstallEagerly pins that
// the bootstrap runs at DOMContentLoaded (or immediately if the
// document is already past loading), subscribes to the store, and
// kicks off a scheduled install attempt + body observer. Without
// this the spike would only react to FUTURE lens changes — a user
// landing directly on a Context URL would never see the overlay.
func TestExplorer_D34bFix_BootstrapKicksOffInstallEagerly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _bootstrap()",
		"MIDASExplorerStore.subscribe(_onStoreChange)",
		// Eager scheduling on bootstrap.
		"if (!lens || lens === 'context') {",
		"_scheduleInstall();",
		"_ensureBodyObserver();",
		// Boot entry deferred to DOMContentLoaded.
		"document.addEventListener('DOMContentLoaded', _bootstrap)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34b-fix: bootstrap contract missing %q", want)
		}
	}
}

// ── D34c-context-cytoscape-card-footprint-layout contract tests ──────

// TestExplorer_D34cCardFootprint_RealDimensionsMeasured pins that
// `_captureSceneNodes` measures the real rendered card footprint
// via `getBoundingClientRect()` (with `offsetWidth/Height` as a
// scale-invariant fallback) and returns `width` + `height` on each
// spec. The previous spike hardcoded `200×80` for every Cytoscape
// node, producing the compressed/overlapping layout observed in
// the browser.
func TestExplorer_D34cCardFootprint_RealDimensionsMeasured(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _captureSceneNodes(scene)")
	if start < 0 {
		t.Fatal("D34c: _captureSceneNodes function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34c: cannot bound _captureSceneNodes body")
	}
	body := js[start : start+end]
	for _, want := range []string{
		"el.getBoundingClientRect()",
		"el.offsetWidth",
		"el.offsetHeight",
		"width:  w,",
		"height: h,",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34c: _captureSceneNodes must measure real dimensions — missing %q", want)
		}
	}
}

// TestExplorer_D34cCardFootprint_CentreCoordinateConversion pins
// the top-left → centre conversion. Cytoscape positions are
// centre-based, but the production Context renderer writes top-
// left coordinates into `style.left` / `style.top`. The previous
// spike fed these directly to cy without conversion, so every
// card was visually shifted by half a card-width/height into its
// neighbours.
func TestExplorer_D34cCardFootprint_CentreCoordinateConversion(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _captureSceneNodes(scene)")
	if start < 0 {
		t.Fatal("D34c: _captureSceneNodes function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34c: cannot bound _captureSceneNodes body")
	}
	body := js[start : start+end]
	for _, want := range []string{
		"var centreX = leftModel + w / 2;",
		"var centreY = topModel  + h / 2;",
		"x:      centreX,",
		"y:      centreY,",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34c: centre coordinate conversion missing %q", want)
		}
	}
}

// TestExplorer_D34cCardFootprint_CytoscapeNodeDataDrivenDimensions
// pins that the Cytoscape node style draws width + height from the
// per-node `data(cardWidth)` / `data(cardHeight)`, not the prior
// hardcoded `200×80` constants. With data-driven dimensions cy.fit()
// scales the bounding box against the real card footprint.
func TestExplorer_D34cCardFootprint_CytoscapeNodeDataDrivenDimensions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Positive pins inside _buildCytoscape's style block.
	for _, want := range []string{
		"cardWidth:  s.width,",
		"cardHeight: s.height,",
		"'width':          'data(cardWidth)',",
		"'height':         'data(cardHeight)',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34c: data-driven cy node dimensions missing %q", want)
		}
	}

	// Negative pin: the prior hardcoded 200×80 must not appear as
	// the runtime node style. (The fallback constants in
	// `_captureSceneNodes` are documented and don't reach the cy
	// style block — they appear in `|| 220` / `|| 64` form.)
	for _, banned := range []string{
		"'width':          200,",
		"'height':         80,",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D34c: hardcoded cy node dimension %q must be removed (data-driven now)", banned)
		}
	}
}

// TestExplorer_D34cCardFootprint_ConnectorKindToEdgeClass pins that
// the `data-connector-kind` attribute on the production SVG
// connector flows through to a cy edge class. This is how the
// per-kind styling below targets the right edges.
func TestExplorer_D34cCardFootprint_ConnectorKindToEdgeClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		// Capture keeps reading `data-connector-kind`.
		"el.getAttribute('data-connector-kind')",
		// Build step wraps the kind in a `cy-conn-<kind>` class.
		"'cy-conn-' + safe",
		// Edge element carries the class.
		"classes: safe ? ",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34c: connector-kind → cy edge class missing %q", want)
		}
	}
}

// TestExplorer_D34cCardFootprint_PerKindEdgeStyles pins that every
// Context connector kind has a matching cy edge style. The Context
// `gmapConnectorKindFromCls` vocabulary is:
//   service, ai_binding, coverage_gap, authority, evidence
// Coverage gap remains dashed; AI binding stays blue; coverage gap
// stays amber; authority stays green; evidence stays slate.
func TestExplorer_D34cCardFootprint_PerKindEdgeStyles(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"selector: 'edge.cy-conn-service'",
		"selector: 'edge.cy-conn-ai_binding'",
		"selector: 'edge.cy-conn-coverage_gap'",
		"selector: 'edge.cy-conn-authority'",
		"selector: 'edge.cy-conn-evidence'",
		// Coverage gap MUST remain dashed.
		"'line-style':         'dashed'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34c: per-kind edge style missing %q", want)
		}
	}
}

// TestExplorer_D34cCardFootprint_FitUsesRealDimensions pins that
// the preset layout still runs with `fit: true` + a padding, so
// cy's automatic fit now scales the bbox against the real (data-
// driven) node dimensions. No explicit numeric width/height in
// the layout config (that would override the data-driven style).
func TestExplorer_D34cCardFootprint_FitUsesRealDimensions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// The layout block must keep `fit: true` and a padding so the
	// initial viewport shows the whole graph at the largest
	// readable scale without browser scrollbars.
	for _, want := range []string{
		"name:     'preset',",
		// D34f — Preset layout no longer auto-fits at init time
		// (was `fit: true`). The premature fit ran against a 0×0
		// mount because the body class that un-hides #gmap-canvas
		// flips only AFTER install succeeds. The canonical fit
		// now lives in the settle pattern below, which runs after
		// the body-class flip + cy.resize(). Either shape satisfies
		// the "preset layout has a fit setting" contract — old
		// `fit: true` or the D34f corrected `fit: false`.
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34c: preset layout missing %q", want)
		}
	}
	if !strings.Contains(js, "fit:      true,") &&
		!strings.Contains(js, "fit:      false,") {
		t.Error("D34c: preset layout must declare a `fit` setting (D34f changed true → false)")
	}
	if !strings.Contains(js, "padding:  _midasFitPadding(),") &&
		!strings.Contains(js, "padding:  60,") {
		t.Error("D34c: preset layout must declare a fit padding (MIDAS-derived or fallback 60)")
	}
}

// ── D34d-context-cytoscape-overlay-canvas-extent contract tests ──────

// TestExplorer_D34dCanvasExtent_OverridesFixedCanvasSize pins the
// CSS override of `#gmap-canvas`'s production fixed sizing (1180×720
// from `.governance-map-canvas` in governance-map.css). Without
// this override the spike's mount inherits the 720 px height cap
// and cards dragged below ~720 px land outside the overlay and
// get clipped by `overflow: hidden`. The override is scoped to
// `body.context-cy-spike-active` so it applies only when the
// spike is installed; lens switch removes the body class and
// production sizing returns automatically.
// TestExplorer_D34dCanvasExtent_OverridesFixedCanvasSize — RETIRED.
//
// D34k-context-cytoscape-authority-mount-pattern removed the
// `width: 100% !important; height: 100% !important; min-width: 0
// !important` override on `#gmap-canvas` entirely. The spike no
// longer mounts inside `#gmap-canvas`; it mounts inside
// `.governance-map-canvas-scroll` (Authority's pattern). The
// production `1180×720` fixed size on `.governance-map-canvas` is
// now irrelevant because `#gmap-canvas` itself is `display: none`
// while the spike is active.
//
// Replaced by `TestExplorer_D34kMount_AppendedToScrollWrapper` and
// `TestExplorer_D34kCSS_MountGeometry`.
func TestExplorer_D34dCanvasExtent_OverridesFixedCanvasSize(t *testing.T) {
	t.Skip("D34k: retired — #gmap-canvas size override replaced by display:none. See TestExplorer_D34kCSS_MountGeometry.")
}

// TestExplorer_D34dCanvasExtent_MountInlineFillsParent pins the
// belt-and-braces inline sizing on the spike mount. With the CSS
// override above, the mount's `inset: 0` correctly fills the
// canvas — but explicit `width: 100% / height: 100%` inline
// makes the dependency on inset's behaviour with dynamic-sized
// parents explicit (and survives any future browser quirk).
// TestExplorer_D34dCanvasExtent_MountInlineFillsParent — RETIRED.
//
// D34k-context-cytoscape-authority-mount-pattern removed all
// inline mount sizing. The mount's geometry is now owned by CSS
// (`position: relative; width: 100%; height: 100%; min-height:
// 480px`), matching Authority Cytoscape's pattern verbatim.
//
// Replaced by `TestExplorer_D34kCSS_MountGeometry`.
func TestExplorer_D34dCanvasExtent_MountInlineFillsParent(t *testing.T) {
	t.Skip("D34k: retired — inline mount sizing removed; CSS owns geometry. See TestExplorer_D34kCSS_MountGeometry.")
}

// TestExplorer_D34dCanvasExtent_ResizeIsTriggeredOnSettleAndObservers
// pins that `cy.resize()` runs in (a) the rAF settle after the body
// class flips and (b) the ResizeObserver callback on the mount.
// This is what makes Cytoscape pick up the new (larger) container
// size when the override-CSS kicks in.
func TestExplorer_D34dCanvasExtent_ResizeIsTriggeredOnSettleAndObservers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// D35e — The settle pattern moved from the public `install()`
	// into `_installResources` (the shared install body called by
	// the host-routed factory AND the legacy fallback). The body-
	// class flip and the settle pattern are now in DIFFERENT
	// functions: install() flips the class after _installResources
	// returns true, and _installResources runs settle inside its
	// own rAF chain. We pin the settle pattern's invariants by
	// bounding on the `_settle` definition directly.
	settleStart := strings.Index(js, "function _settle() {")
	if settleStart < 0 {
		t.Fatal("D34d: _settle definition missing")
	}
	settleEnd := strings.Index(js[settleStart:], "\n    }")
	if settleEnd < 0 {
		t.Fatal("D34d: cannot bound _settle body")
	}
	settleBody := js[settleStart : settleStart+settleEnd]
	if !strings.Contains(settleBody, "_cy.resize();") {
		t.Error("D34d: settle pattern must call _cy.resize()")
	}
	// D35e — Padding is composed from `_midasFitPadding()` floor +
	// host `ctx.getSafeArea()` ceiling, stored in `fitPadding`.
	// The fit call shape is now `_cy.fit(undefined, fitPadding)`.
	if !strings.Contains(settleBody, "_cy.fit(undefined, fitPadding)") {
		t.Error("D34d: settle pattern must call _cy.fit() after resize (with D35e-composed fitPadding)")
	}
	if !strings.Contains(settleBody, "_midasFitPadding()") {
		t.Error("D34d: settle pattern must source the MIDAS padding floor via _midasFitPadding()")
	}

	// ResizeObserver on the mount keeps cy sized as the mount
	// changes (right drawer toggle, window resize). D34i — the
	// observer callback now triggers `_syncLayerBound` (a resize
	// only changes layer transform, not per-card model positions).
	// Per-tier wiring is intentional: layer-only on resize avoids
	// the unnecessary per-card walk that pre-D34i did.
	for _, want := range []string{
		"new window.ResizeObserver(_syncLayerBound)",
		"_resizeObs.observe(_mountEl)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34d: resize observation must wire %q", want)
		}
	}
}

// TestExplorer_D34dDebug_DebugStateCanvasExtentKeys pins every
// new debugState() field the brief required for diagnosing the
// canvas-extent problem.
func TestExplorer_D34dDebug_DebugStateCanvasExtentKeys(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugState()")
	if start < 0 {
		t.Fatal("D34d: _debugState function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34d: cannot bound _debugState body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		// Rects.
		"canvasRect:",
		"sceneRect:",
		"svgRect:",
		"cyMountRect:",
		// Cytoscape internal dimensions.
		"cyWidth:",
		"cyHeight:",
		"cyExtent:",
		// Computed styles for mount.
		"mountComputedPosition:",
		"mountComputedWidth:",
		"mountComputedHeight:",
		"mountComputedOverflow:",
		// Computed styles for overlay.
		"overlayComputedPosition:",
		"overlayComputedWidth:",
		"overlayComputedHeight:",
		"overlayComputedOverflow:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34d: debugState() must return %q for canvas-extent diagnostics", want)
		}
	}
}

// TestExplorer_D34dCanvasExtent_HidingSceneSvgUnchanged pins the
// invariant that production scene + svg are still hidden by the
// spike (so the spike's mount is the only visible layer), AND
// that the hide rule still targets ONLY those two ids. The D34d
// fix must not have widened the hide scope by accident.
// TestExplorer_D34dCanvasExtent_HidingSceneSvgUnchanged — RETIRED.
//
// D34k-context-cytoscape-authority-mount-pattern: hide moved from
// scene/svg `visibility: hidden` to `#gmap-canvas display: none`.
// The scene + svg are no longer individually hidden because they
// are inside the now-display:none canvas. Service-switch
// observation still works: the MutationObserver in the spike
// watches `#gmap-scene` regardless of display state (`display: none`
// preserves DOM identity).
//
// Replaced by `TestExplorer_D34kCSS_NoVisibilityHidden`.
func TestExplorer_D34dCanvasExtent_HidingSceneSvgUnchanged(t *testing.T) {
	t.Skip("D34k: retired — visibility:hidden replaced by display:none on canvas. See TestExplorer_D34kCSS_NoVisibilityHidden.")
}

// TestExplorer_D34dCanvasExtent_OverrideRevertsOnDestroy pins that
// the canvas-extent override is body-class-scoped, so destroy()
// (which removes the body class) automatically reverts to the
// production fixed sizing. No JS work needed — purely via cascade.
func TestExplorer_D34dCanvasExtent_OverrideRevertsOnDestroy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// D35f-retire-transitional-renderer-debt — the canvas-extent
	// override (and all spike CSS) is now keyed on host-owned
	// `data-active-renderer="context-cytoscape"`. The destroy path
	// routes through `viewport.deactivate()` which clears (or
	// restores baseline) on the host, automatically reverting the
	// override via the cascade. No body-class flip needed.
	start := strings.Index(js, "function destroy() {")
	if start < 0 {
		t.Fatal("D35f: destroy function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D35f: cannot bound destroy body")
	}
	body := js[start : start+end]
	if !strings.Contains(body, "vp.deactivate") {
		t.Error("D35f: destroy() must route through viewport.deactivate() so the host clears/restores data-active-renderer")
	}
	// Negative — body-class flip retired.
	if strings.Contains(body, "document.body.classList.remove(BODY_FLAG_CLASS)") {
		t.Error("D35f: destroy() must NOT flip body class (host owns renderer identity)")
	}
}

// ── D34e-context-cytoscape-layout-parameter-port contract tests ──────

// TestExplorer_D34eLayoutPort_MidasConstantsExist pins that the
// canonical MIDAS layout constants the spike now reads are still
// declared on `window.MIDASGovernanceMap.GMAP` in the production
// `governance-map/constants.js`. If a future tranche moves /
// renames these constants, the spike's helpers (`_midasGmap`,
// `_midasFallbackDims`, `_midasMinGap`, `_midasFitPadding`) will
// silently fall back to defaults — this test surfaces that
// regression at the source.
func TestExplorer_D34eLayoutPort_MidasConstantsExist(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	for _, want := range []string{
		"window.MIDASGovernanceMap.GMAP",
		"NODE_W: 220,",
		"NODE_H: 64,",
		"NODE_GAP: 32,",
		"EDGE_PAD: 72,",
		"LAYERS:",
		"BUSINESS: { y:",
		"CAP_PROC: { y:",
		"SURFACE:  { y:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34e: MIDAS layout constant %q must still be declared on window.MIDASGovernanceMap.GMAP", want)
		}
	}
}

// TestExplorer_D34eLayoutPort_SpikeAccessorsReadMidasConstants pins
// the four spike helpers that read the MIDAS constants. The
// helpers' bodies must reference the GMAP property names, not
// duplicate the values inline — that's the whole point of the
// port (single source of truth in production code).
func TestExplorer_D34eLayoutPort_SpikeAccessorsReadMidasConstants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		// The four accessor functions.
		"function _midasGmap()",
		"function _midasFallbackDims()",
		"function _midasMinGap()",
		"function _midasFitPadding()",
		// Each reads from the canonical MIDAS namespace.
		"window.MIDASGovernanceMap.GMAP",
		"g.NODE_W",
		"g.NODE_H",
		"g.NODE_GAP",
		"g.EDGE_PAD",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34e: spike accessor must read %q from MIDAS GMAP namespace", want)
		}
	}
}

// TestExplorer_D34eLayoutPort_CaptureFallbacksUseMidas pins that
// the dimension fallback inside `_captureSceneNodes` reads MIDAS
// constants (`_midasFallbackDims()` → `GMAP.NODE_W / NODE_H`)
// rather than duplicating the values inline. The literal `220` /
// `64` may still appear as a last-resort fallback inside the
// helper itself, but the capture path must go through the helper.
func TestExplorer_D34eLayoutPort_CaptureFallbacksUseMidas(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _captureSceneNodes(scene)")
	if start < 0 {
		t.Fatal("D34e: _captureSceneNodes function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34e: cannot bound _captureSceneNodes body")
	}
	body := js[start : start+end]
	for _, want := range []string{
		"var fb = _midasFallbackDims();",
		"fb.width",
		"fb.height",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34e: capture fallback path must go through _midasFallbackDims — missing %q", want)
		}
	}
}

// TestExplorer_D34eLayoutPort_FitPaddingUsesMidasEdgePad pins that
// the Cytoscape `fit` padding is sourced from `_midasFitPadding()`
// (which reads `GMAP.EDGE_PAD = 72`), not from a hardcoded 60.
// The preset layout call and the settle pattern both use the
// helper.
func TestExplorer_D34eLayoutPort_FitPaddingUsesMidasEdgePad(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		// Preset layout uses the helper.
		"padding:  _midasFitPadding(),",
		// D35e — Settle pattern still sources its padding floor from
		// `_midasFitPadding()` (MIDAS EDGE_PAD); the cy.fit call now
		// uses `fitPadding` which is `Math.max(_midasFitPadding(),
		// host ctx.getSafeArea())`. The MIDAS source is preserved.
		"var fitPadding = _midasFitPadding();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34e: fit padding must be sourced from MIDAS EDGE_PAD — missing %q", want)
		}
	}
}

// TestExplorer_D34eLayoutPort_CollisionResolutionPreservesOrder
// pins the defensive same-row collision-resolution pass. The
// algorithm must:
//   • bucket specs by y-band (rounded for float drift);
//   • sort each band by x ascending;
//   • walk left-to-right and push apart neighbours only when the
//     centre-to-centre gap is less than `(prev.w/2 + cur.w/2 +
//     NODE_GAP)`;
//   • never reorder bands.
func TestExplorer_D34eLayoutPort_CollisionResolutionPreservesOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _applyMidasSpacingContract(specs)")
	if start < 0 {
		t.Fatal("D34e: _applyMidasSpacingContract function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34e: cannot bound _applyMidasSpacingContract body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		// Reads the MIDAS gap.
		"var minGap = _midasMinGap();",
		// Y-band bucketing with 8 px tolerance.
		"Math.round(specs[i].y / 8) * 8",
		// Sort by x ascending within each band.
		"row.sort(function (a, b) { return a.x - b.x; });",
		// Required centre-to-centre gap formula.
		"(prev.width / 2) + (cur.width / 2) + minGap",
		// Push-right semantics (preserves left card's position).
		"cur.x = prev.x + requiredCentreGap;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34e: collision-resolution algorithm missing %q", want)
		}
	}
}

// TestExplorer_D34eLayoutPort_InstallCallsCollisionPass pins that
// `install()` invokes the collision-resolution pass between
// capture and cy build, so the corrected positions flow through
// to Cytoscape's preset layout.
func TestExplorer_D34eLayoutPort_InstallCallsCollisionPass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	captureIdx := strings.Index(js, "specs = _captureSceneNodes(scene);")
	if captureIdx < 0 {
		t.Fatal("D34e: capture call site missing from install")
	}
	cyBuildIdx := strings.Index(js[captureIdx:], "_cy = _buildCytoscape(")
	if cyBuildIdx < 0 {
		t.Fatal("D34e: _buildCytoscape call site missing")
	}
	between := js[captureIdx : captureIdx+cyBuildIdx]
	if !strings.Contains(between, "specs = _applyMidasSpacingContract(specs);") {
		t.Error("D34e: install() must call _applyMidasSpacingContract(specs) between capture and cy build")
	}
}

// ── D34f-context-cytoscape-node-footprint-fit contract tests ─────────

// TestExplorer_D34fFootprintFit_PresetLayoutDoesNotAutoFit pins the
// fit-timing fix. Preset layout's `fit: true` ran synchronously at
// `cytoscape()` init — BEFORE the body class flipped `#gmap-canvas`
// from `display: none` to `display: block`. With a 0×0 mount, the
// initial fit computed a degenerate viewport. The fix is `fit: false`
// in the preset config; the canonical fit lives in the settle
// pattern which runs after the body-class flip + cy.resize().
func TestExplorer_D34fFootprintFit_PresetLayoutDoesNotAutoFit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Bound the preset layout config inside _buildCytoscape and
	// confirm `fit: false` is the active setting. The bound runs
	// to the next sibling config key (`wheelSensitivity`) so the
	// inner `},` of the positions function doesn't terminate the
	// slice early.
	layoutIdx := strings.Index(js, "layout: {")
	if layoutIdx < 0 {
		t.Fatal("D34f: preset layout config missing")
	}
	layoutEnd := strings.Index(js[layoutIdx:], "wheelSensitivity")
	if layoutEnd < 0 {
		t.Fatal("D34f: cannot bound preset layout block (no wheelSensitivity sibling found)")
	}
	layoutBlock := js[layoutIdx : layoutIdx+layoutEnd]
	if !strings.Contains(layoutBlock, "fit:      false,") {
		t.Errorf("D34f: preset layout must use `fit: false` (D34f); layout block was:\n%s", layoutBlock)
	}
	if strings.Contains(layoutBlock, "fit:      true,") {
		t.Error("D34f: preset layout must NOT auto-fit at init (the 0×0 mount race is the original bug)")
	}
}

// TestExplorer_D34fFootprintFit_CanonicalFitPathInSettle pins that
// the settle pattern remains the single canonical fit path: resize
// the container first, fit second, sync third. After D34f this is
// the ONLY fit cy performs (preset layout no longer fits).
func TestExplorer_D34fFootprintFit_CanonicalFitPathInSettle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	settleIdx := strings.Index(js, "function _settle() {")
	if settleIdx < 0 {
		t.Fatal("D34f: _settle function definition missing")
	}
	settleEnd := strings.Index(js[settleIdx:], "\n    }")
	if settleEnd < 0 {
		t.Fatal("D34f: cannot bound _settle body")
	}
	body := js[settleIdx : settleIdx+settleEnd]

	// Order check: resize must precede fit, fit must precede sync.
	// D35e — fit call shape evolved from
	// `_cy.fit(undefined, _midasFitPadding())` to
	// `_cy.fit(undefined, fitPadding)` where `fitPadding` is
	// composed from MIDAS floor + host safe-area. The order
	// invariant (resize → fit → sync) is preserved.
	resizeIdx := strings.Index(body, "_cy.resize();")
	fitIdx    := strings.Index(body, "_cy.fit(undefined, fitPadding)")
	syncIdx   := strings.Index(body, "_sync();")
	if resizeIdx < 0 {
		t.Error("D34f: _settle must call _cy.resize() before fit")
	}
	if fitIdx < 0 {
		t.Error("D34f: _settle must call _cy.fit(undefined, fitPadding) (D35e-composed padding)")
	}
	if syncIdx < 0 {
		t.Error("D34f: _settle must call _sync() after fit")
	}
	if !(resizeIdx < fitIdx && fitIdx < syncIdx) {
		t.Errorf("D34f: _settle order must be resize → fit → sync (got resize=%d fit=%d sync=%d)", resizeIdx, fitIdx, syncIdx)
	}

	// State tracking — fit success path must update _lastFitReason +
	// _fitTimingState. A throw path must update _lastFitSkippedReason.
	// D35e renamed the success reason from 'settle-after-body-class'
	// to 'settle-after-install' because the body class is no longer
	// flipped inside `_installResources` (moved to the public
	// `install()` boundary). The semantic — "fit ran inside settle
	// after install completed" — is unchanged.
	for _, want := range []string{
		"_lastFitReason     = 'settle-after-install';",
		"_fitTimingState    = 'fitted';",
		"_lastFitSkippedReason = 'fit-threw:'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34f: _settle must track fit outcome via %q", want)
		}
	}
}

// TestExplorer_D34fFootprintFit_BoundsValidatorExists pins the
// `_validateCytoscapeCardBounds` diagnostic helper that proves cy's
// `boundingBox()` reflects the data-driven card dimensions. The
// validator's `valid: true` path requires:
//   • nodes present;
//   • bbox is non-empty;
//   • first node's cy-reported width matches `data(cardWidth)`;
//   • first node's cy-reported height matches `data(cardHeight)`;
//   • bbox.w ≥ first card's width, bbox.h ≥ first card's height.
// If any check fails, the `reason` field carries a specific token
// so a DevTools probe can pinpoint which assumption broke.
func TestExplorer_D34fFootprintFit_BoundsValidatorExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _validateCytoscapeCardBounds(cy)",
		"cy.elements().boundingBox()",
		"first.data('cardWidth')",
		"first.data('cardHeight')",
		// Reason tokens.
		"reason: 'no-cy'",
		"reason: 'no-nodes'",
		"reason: 'empty-bbox'",
		"reason = 'width-mismatch'",
		"reason = 'height-mismatch'",
		"reason = 'bbox-narrower-than-card'",
		"reason = 'bbox-shorter-than-card'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34f: _validateCytoscapeCardBounds must include %q", want)
		}
	}

	// Validator is exposed on the public surface for browser probes
	// + tests.
	if !strings.Contains(js, "_validateCytoscapeCardBounds: _validateCytoscapeCardBounds,") {
		t.Error("D34f: _validateCytoscapeCardBounds must be on the public surface")
	}
}

// TestExplorer_D34fDebug_DebugStateExposesBoundingBox pins every
// new debugState field the brief requested for diagnosing
// fit-against-real-dimensions.
func TestExplorer_D34fDebug_DebugStateExposesBoundingBox(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugState()")
	if start < 0 {
		t.Fatal("D34f: _debugState function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34f: cannot bound _debugState body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		"cyElementsBoundingBox:",
		"firstNodeId:",
		"firstNodePosition:",
		"firstNodeDataCardWidth:",
		"firstNodeDataCardHeight:",
		"firstNodeRenderedBoundingBox:",
		"nodeDimensionStyleMode: 'data(cardWidth/cardHeight)',",
		"fitTimingState:",
		"lastFitReason:",
		"lastFitSkippedReason:",
		"cardBoundsValidation:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34f: debugState() must return %q for fit/dimension diagnostics", want)
		}
	}
}

// TestExplorer_D34fFootprintFit_NodeDimensionsRemainDataDriven pins
// that D34f did NOT regress the data-driven node dimensions from
// D34c. The cy node style must still read `data(cardWidth)` and
// `data(cardHeight)`, and the spec → node-data plumbing must still
// populate those fields from the measured rect.
func TestExplorer_D34fFootprintFit_NodeDimensionsRemainDataDriven(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"cardWidth:  s.width,",
		"cardHeight: s.height,",
		"'width':          'data(cardWidth)',",
		"'height':         'data(cardHeight)',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34f: data-driven node dimensions must remain — missing %q", want)
		}
	}

	// Negative pin: no return of the hardcoded 200×80 runtime style.
	for _, banned := range []string{
		"'width':          200,",
		"'height':         80,",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D34f: hardcoded cy node dimension %q must NOT return — node geometry stays data-driven", banned)
		}
	}
}

// ── D34g-cytoscape-html-overlay-geometry-investigation tests ─────────
//
// Investigation tranche — these tests only pin the new diagnostic
// surface. NO behaviour change is asserted; if the investigation
// proves a root cause and a fix is implemented in a follow-up
// tranche, those fix-contract tests live with that tranche.

// TestExplorer_D34gDebug_ExposesBrowserGeometryFields pins the
// browser-level geometry fields the brief requested. These let a
// DevTools probe identify which element in the parent chain
// (window / body / canvas / mount / overlay) is overflowing when
// browser scrollbars appear.
func TestExplorer_D34gDebug_ExposesBrowserGeometryFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugState()")
	if start < 0 {
		t.Fatal("D34g: _debugState function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34g: cannot bound _debugState body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		"windowScrollX:",
		"windowScrollY:",
		"viewportWidth:",
		"viewportHeight:",
		"bodyScrollWidth:",
		"bodyScrollHeight:",
		"firstNodeRenderedPosition:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34g: debugState() must return %q for browser-geometry diagnostics", want)
		}
	}
}

// TestExplorer_D34gDebug_ExposesDragObservationFields pins the
// drag-observation fields. `backgroundPanDuringCardDrag: true` is
// the brief's "smoking gun" for the suspected pointerdown leak
// from card to cy. `activeDragState` carries the full drag-time
// snapshot. The investigation does NOT mutate the drag handler's
// behaviour — only observes it.
func TestExplorer_D34gDebug_ExposesDragObservationFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		// Module-level drag tracking state vars.
		"var _dragState = {",
		"var _lastPointerDownTarget = null;",
		// debugState fields.
		"activeDragState:",
		"lastPointerDownTarget:",
		"lastDragDeltaPx:",
		"lastDragDeltaModel:",
		"backgroundPanDuringCardDrag:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34g: drag-observation surface must include %q", want)
		}
	}
}

// TestExplorer_D34gDebug_DragTrackingIsObservationOnly — RETIRED.
//
// D34h-context-cytoscape-native-graph-management removed the DOM
// drag handler entirely; cy now owns dragging. There is no longer
// a code path that could mutate cy.pan / cy.fit during a card
// drag. The negative + positive pins this test enforced are
// preserved structurally by D34h tests below:
//   • `TestExplorer_D34hNative_NoCustomDomDragPath` asserts the
//     whole DOM drag surface is gone.
//   • `TestExplorer_D34hNative_DebugStateNativeFields` asserts the
//     diagnostic `customDomDragEnabled: false` field documents
//     the contract.
//
// The previous body is replaced with a single skipped marker so
// any tooling tracking historic test names sees an explicit
// retirement reason rather than a silent disappearance.
func TestExplorer_D34gDebug_DragTrackingIsObservationOnly(t *testing.T) {
	t.Skip("D34h: retired — DOM drag handler removed; see TestExplorer_D34hNative_NoCustomDomDragPath")
}

// TestExplorer_D34cCardFootprint_GroupDragSnapshot — RETIRED.
//
// D34h-context-cytoscape-native-graph-management removed the
// custom DOM group-drag path. Group drag is now Cytoscape native:
// when multiple cy nodes are :selected and the user grabs one of
// them, cy moves all selected nodes together by the pointer delta
// internally — no JS we own runs in this path. Replaced by:
//   • `TestExplorer_D34hNative_BoxSelectionEnabled` — cy native
//     box-selection (the gesture by which a user picks the multi-
//     selection in the first place).
//   • `TestExplorer_D34hNative_NoCustomDomDragPath` — proves the
//     custom dragSet snapshot loop is gone.
func TestExplorer_D34cCardFootprint_GroupDragSnapshot(t *testing.T) {
	t.Skip("D34h: retired — DOM group-drag removed; cy owns group drag natively. See TestExplorer_D34hNative_BoxSelectionEnabled.")
}

// ── D34h-context-cytoscape-native-graph-management tests ─────────────
//
// Architectural rework. Cytoscape now owns graph interaction; the
// HTML overlay is a passive view. These tests pin the new contract
// at the asset-text level: overlay mount/stacking, pointer-events
// transparency, removal of the DOM drag surface, cy-native
// selection delegation, debugState native-management fields, and
// that no third-party dependency was added.

// TestExplorer_D34hNative_OverlayMountedInsideCyContainer pins that
// the overlay layer is appended as a CHILD of the cy mount (the
// element passed as `container:` to Cytoscape). This guarantees the
// overlay shares the cy container's coordinate system, clipping
// box, and stacking context — the structural fix for the D34g
// "overlay and cy can disagree about their screen origin" failure
// mode.
func TestExplorer_D34hNative_OverlayMountedInsideCyContainer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Cy is built with the mount as its container, and the overlay
	// element is appended to the SAME mount.
	for _, want := range []string{
		"_cy = _buildCytoscape(_mountEl",
		"_mountEl.appendChild(_overlayEl)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34h: install must wire overlay as child of cy mount — missing %q", want)
		}
	}
	// debugState exposes the structural assertion `overlayParent ===
	// cy.container()` so a runtime probe can verify it too.
	if !strings.Contains(js, "overlayParentIsCyContainer:") {
		t.Error("D34h: debugState must expose overlayParentIsCyContainer for runtime verification")
	}
}

// TestExplorer_D34hNative_MountClipsAndContains pins the CSS
// contract that the mount has `overflow: hidden`.
//
// D34k-context-cytoscape-authority-mount-pattern: `contain: layout
// paint` was REMOVED. With `#gmap-canvas display: none` the hidden
// production scene no longer contributes to ancestor scroll, so
// the D34h containment defence is no longer load-bearing. The
// mount keeps `overflow: hidden` to clip its OWN cy canvas layers
// (cy may briefly render outside the visible extent during rapid
// pan/zoom, which we want clipped).
//
// `isolation: isolate` replaces `contain: layout paint`'s
// secondary role of containing the overlay's z-index 100 to a
// local stacking context (so body-level MIDAS chrome at z-index 5
// remains above the mount).
func TestExplorer_D34hNative_MountClipsAndContains(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	idx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] .context-cy-spike-mount {`)
	if idx < 0 {
		t.Fatal("D34k: mount CSS rule missing")
	}
	end := strings.Index(css[idx:], "\n}")
	if end < 0 {
		t.Fatal("D34k: cannot bound mount CSS rule")
	}
	block := css[idx : idx+end]
	for _, want := range []string{
		"overflow: hidden",
		"isolation: isolate",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D34k: mount rule must contain %q; body was:\n%s", want, block)
		}
	}
}

// TestExplorer_D34hNative_OverlayPointerEventsNone pins that BOTH
// the overlay layer and the cloned cards inside it carry
// `pointer-events: none`. This is the structural change that
// passes mouse/touch interaction through to cy's hit-testable
// canvas — cy then maps the pointer to the correct node by its
// data-driven bounding box.
func TestExplorer_D34hNative_OverlayPointerEventsNone(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	// Overlay layer rule.
	overlayIdx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] .context-cy-spike-overlay {`)
	if overlayIdx < 0 {
		t.Fatal("D34h: overlay layer CSS rule missing")
	}
	overlayEnd := strings.Index(css[overlayIdx:], "\n}")
	if overlayEnd < 0 {
		t.Fatal("D34h: cannot bound overlay layer CSS rule")
	}
	overlayBlock := css[overlayIdx : overlayIdx+overlayEnd]
	if !strings.Contains(overlayBlock, "pointer-events: none") {
		t.Error("D34h: overlay layer rule must include `pointer-events: none`")
	}

	// Card rule inside the overlay.
	cardIdx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] .context-cy-spike-overlay .gmap-node {`)
	if cardIdx < 0 {
		t.Fatal("D34h: overlay card CSS rule missing")
	}
	cardEnd := strings.Index(css[cardIdx:], "\n}")
	if cardEnd < 0 {
		t.Fatal("D34h: cannot bound overlay card CSS rule")
	}
	cardBlock := css[cardIdx : cardIdx+cardEnd]
	if !strings.Contains(cardBlock, "pointer-events: none") {
		t.Error("D34h: overlay card rule must include `pointer-events: none` (cy owns pointer interaction)")
	}
	if strings.Contains(cardBlock, "pointer-events: auto") {
		t.Error("D34h: overlay card rule must NOT re-enable pointer events")
	}
}

// TestExplorer_D34hNative_NoCustomDomDragPath pins the removal of
// the entire DOM-pointer drag surface. None of these tokens may
// appear anywhere in the spike module body (excluding comments,
// which legitimately reference what was removed).
//
// `_wireCardDrag` and `_wireCardClick` are removed entirely;
// `DRAG_THRESHOLD_PX` is removed since no DOM-side threshold is
// needed; `setPointerCapture` and document-level pointermove/up
// listeners are removed because cy handles pointer capture.
func TestExplorer_D34hNative_NoCustomDomDragPath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	exec := stripJSComments(js)

	for _, banned := range []string{
		"function _wireCardDrag(",
		"function _wireCardClick(",
		"DRAG_THRESHOLD_PX",
		"setPointerCapture(",
		"releasePointerCapture(",
		"document.addEventListener('pointermove'",
		"document.addEventListener('pointerup'",
		"document.addEventListener('pointercancel'",
		// The custom group-drag snapshot loop is gone too.
		"dragSet[di].node.position(",
		"_cy.nodes(':selected').forEach",
		"isInSelection",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D34h: DOM-drag artefact must be removed from executable JS — found %q", banned)
		}
	}
}

// TestExplorer_D34hNative_CySelectionWiredOnce pins that the cy
// native selection delegation is wired in `install()` exactly
// once via `_wireCytoscapeInteraction(_cy)`, and that the
// function itself attaches `cy.on('tap', 'node', …)` (not on the
// background, not on every element).
func TestExplorer_D34hNative_CySelectionWiredOnce(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	if !strings.Contains(js, "_wireCytoscapeInteraction(_cy)") {
		t.Error("D34h: install() must invoke _wireCytoscapeInteraction(_cy) once")
	}
	if got := strings.Count(stripJSComments(js), "_wireCytoscapeInteraction(_cy)"); got != 1 {
		t.Errorf("D34h: _wireCytoscapeInteraction must be invoked exactly once; got %d", got)
	}
	cyStart := strings.Index(js, "function _wireCytoscapeInteraction(cy)")
	if cyStart < 0 {
		t.Fatal("D34h: _wireCytoscapeInteraction definition missing")
	}
	cyEnd := strings.Index(js[cyStart:], "\n  }")
	if cyEnd < 0 {
		t.Fatal("D34h: cannot bound _wireCytoscapeInteraction body")
	}
	body := js[cyStart : cyStart+cyEnd]
	if !strings.Contains(body, "cy.on('tap', 'node'") {
		t.Error("D34h: _wireCytoscapeInteraction must wire `cy.on('tap', 'node', …)` for selection")
	}
}

// TestExplorer_D34hNative_BoxSelectionEnabled pins that cy native
// box selection stays on — the spike does not pass `false` for
// `boxSelectionEnabled`. Combined with cy's default node grab,
// shift+drag on background paints a selection rectangle, and a
// subsequent grab on any of the selected nodes drags the group.
func TestExplorer_D34hNative_BoxSelectionEnabled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	if !strings.Contains(js, "boxSelectionEnabled: true") {
		t.Error("D34h: cy must enable native box selection (boxSelectionEnabled: true)")
	}
	if strings.Contains(js, "boxSelectionEnabled: false") {
		t.Error("D34h: spike must NOT disable cy native box selection")
	}
	if strings.Contains(js, "autounselectify:    true") ||
		strings.Contains(js, "autounselectify: true") {
		t.Error("D34h: cy must allow unselect (autounselectify must stay false)")
	}
}

// TestExplorer_D34hNative_NodeDimensionsDataDriven pins that cy
// node dimensions remain data-driven from the measured MIDAS card
// footprint. Cy's native fit/layout depends on these dimensions
// being present BEFORE layout runs — the D34c contract is
// preserved unchanged.
func TestExplorer_D34hNative_NodeDimensionsDataDriven(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"cardWidth:  s.width",
		"cardHeight: s.height",
		"'width':          'data(cardWidth)'",
		"'height':         'data(cardHeight)'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34h: cy node dimensions must be data-driven — missing %q", want)
		}
	}
}

// TestExplorer_D34hNative_FitIsCytoscapeNative pins that fit
// remains cy native (`_cy.fit(undefined, _midasFitPadding())`)
// and runs only inside the `_settle` path after the body class
// has flipped and `cy.resize()` has been called. No custom MIDAS
// viewport engine.
func TestExplorer_D34hNative_FitIsCytoscapeNative(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	settleStart := strings.Index(js, "function _settle()")
	if settleStart < 0 {
		t.Fatal("D34h: _settle definition missing")
	}
	settleEnd := strings.Index(js[settleStart:], "\n    }")
	if settleEnd < 0 {
		t.Fatal("D34h: cannot bound _settle body")
	}
	body := js[settleStart : settleStart+settleEnd]
	for _, want := range []string{
		"_cy.resize()",
		// D35e — `_cy.fit(undefined, ...)` continues to be the
		// canonical cy-native fit call. The padding argument is now
		// composed from `_midasFitPadding()` (MIDAS floor) and the
		// host's `ctx.getSafeArea()` (D35e ceiling). The composed
		// value is stored in `fitPadding` immediately before the
		// fit call, so we pin BOTH the floor source AND the cy
		// fit call shape.
		"_midasFitPadding()",
		"_cy.fit(undefined, fitPadding)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34h: _settle must include %q (cy native fit ordering, D35e-composed padding)", want)
		}
	}
	// Preset layout still defers fit so the canonical fit is the
	// one in _settle, not the one inside the layout init.
	if !strings.Contains(js, "fit:      false") {
		t.Error("D34h: preset layout must still pass `fit: false` so cy's only fit is the settle-driven one")
	}
}

// TestExplorer_D34hNative_DebugStateNativeFields pins the new
// diagnostic surface that proves the native-management contract
// at runtime.
func TestExplorer_D34hNative_DebugStateNativeFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugState()")
	if start < 0 {
		t.Fatal("D34h: _debugState definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34h: cannot bound _debugState body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		"overlayParentIsCyContainer:",
		"nativeDraggingEnabled:",
		"boxSelectionEnabled:",
		"customDomDragEnabled: false",
		"selectedNodeCount:",
		"scrollbarOverflowDetected:",
		// D34g browser-geometry fields stay so DevTools probes
		// retain their existing surface.
		"viewportWidth:",
		"viewportHeight:",
		"bodyScrollWidth:",
		"bodyScrollHeight:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34h: debugState must expose %q", want)
		}
	}
}

// TestExplorer_D34hNative_PublicSurfaceUpdated pins the public
// API change: the old card-handler exports are gone, the new cy-
// wiring exports are in, and `DRAG_THRESHOLD_PX` is removed.
func TestExplorer_D34hNative_PublicSurfaceUpdated(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	exec := stripJSComments(js)

	for _, want := range []string{
		"_wireCytoscapeInteraction:    _wireCytoscapeInteraction",
		"_wireCardKeyboardActivation:  _wireCardKeyboardActivation",
	} {
		if !strings.Contains(exec, want) {
			t.Errorf("D34h: public surface must export %q", want)
		}
	}
	for _, banned := range []string{
		"_wireCardClick:",
		"_wireCardDrag:",
		"DRAG_THRESHOLD_PX:",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D34h: public surface must no longer export %q", banned)
		}
	}
}

// TestExplorer_D34hNative_GatedByContextHtmlCardsFlag pins that
// the architectural rework did NOT alter the gating. The spike
// stays behind `?cytoscape=1&contextHtmlCards=1` and the closed-
// gate IIFE still early-returns.
func TestExplorer_D34hNative_GatedByContextHtmlCardsFlag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	if !strings.Contains(js, "sp.get('cytoscape') === '1' && sp.get('contextHtmlCards') === '1'") {
		t.Error("D34h: gating logic must be preserved verbatim")
	}
	if !strings.Contains(js, "if (!_isActive()) {") {
		t.Error("D34h: closed-gate early-return must remain")
	}
}

// TestExplorer_D34hNative_NoNewThirdPartyDependency pins that the
// rework did NOT add a new npm/vendored library. Cy is the only
// external graph library on the page, loaded via the existing
// Authority PoC script tag.
func TestExplorer_D34hNative_NoNewThirdPartyDependency(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Any unfamiliar library script would show up here. We pin
	// that the index.html still references only the existing
	// cytoscape distribution (not cytoscape-node-html-label,
	// cytoscape-cola, cytoscape-cose, etc).
	for _, banned := range []string{
		"cytoscape-node-html-label",
		"cytoscape-cola",
		"cytoscape-cose",
		"cytoscape-dagre",
		"cytoscape-popper",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D34h: no third-party cy extension may be added — found %q in index.html", banned)
		}
	}
	// Spike module itself must not require/import anything new.
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	for _, banned := range []string{
		"require(",
		"import ",
		"from '",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D34h: spike must remain a self-contained IIFE — found %q", banned)
		}
	}
}

// TestExplorer_D34hNative_ContextGraphAndAuthorityPreserved pins
// that the rework did NOT alter the production Context Graph,
// Authority Cytoscape, or Authority List Mode. The surfaces in
// each of those owners must continue to exist verbatim.
func TestExplorer_D34hNative_ContextGraphAndAuthorityPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Production Context view + renderer untouched.
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(ctxJS, "renderer.addNode") {
		t.Error("D34h: production Context view must keep its renderer.addNode path")
	}
	if strings.Contains(ctxJS, "contextCytoscapeOverlaySpike") {
		t.Error("D34h: production Context view must NOT reference the spike module")
	}

	// Authority Cytoscape PoC module still served + surface
	// preserved (exposed under window.MIDASExplorerGraph.cytoscapePoc).
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	if !strings.Contains(authJS, "window.MIDASExplorerGraph.cytoscapePoc") {
		t.Error("D34h: Authority Cytoscape PoC public surface must remain")
	}

	// Authority HTML overlay (D34a) preserved.
	authOverlayJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")
	if !strings.Contains(authOverlayJS, "function _wireCardClick(card, nodeId) {") {
		t.Error("D34h: Authority HTML overlay (D34a) must remain unchanged — its _wireCardClick still exists")
	}
}

// ── D34h-precheck-cytoscape-node-vs-html-card-footprint tests ────────
//
// Diagnostic-only tranche. These tests pin the new `debugFootprints()`
// surface and guarantee it is pure observation (no cy state mutation).
// The function compares cy native node dimensions with HTML overlay
// card dimensions per node and reports deltas.

// TestExplorer_D34hPrecheck_FootprintHelperExposed pins the helper
// is on the public surface AND only available when the gate is open
// (the closed-gate IIFE substitutes a stub namespace that omits it).
func TestExplorer_D34hPrecheck_FootprintHelperExposed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	if !strings.Contains(js, "debugFootprints: _debugFootprints") {
		t.Error("D34h-precheck: contextCytoscapeOverlaySpike must expose debugFootprints()")
	}
	if !strings.Contains(js, "function _debugFootprints()") {
		t.Error("D34h-precheck: _debugFootprints function definition missing")
	}
	// Gated like the rest of the module — closed-gate path serves a
	// stub namespace and never reaches the public surface block.
	if !strings.Contains(js, "if (!_isActive()) {") {
		t.Error("D34h-precheck: gate guard preserved")
	}
}

// TestExplorer_D34hPrecheck_FootprintReadsCyAndCardDims pins that the
// helper reads from both cy and the HTML card layer using the
// documented APIs from the brief.
func TestExplorer_D34hPrecheck_FootprintReadsCyAndCardDims(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugFootprints()")
	if start < 0 {
		t.Fatal("D34h-precheck: _debugFootprints definition missing")
	}
	end := strings.Index(js[start:], "\n  }\n\n  function _debugState()")
	if end < 0 {
		t.Fatal("D34h-precheck: cannot bound _debugFootprints body")
	}
	body := js[start : start+end]

	// Cy reads (every API in the brief).
	for _, want := range []string{
		"_cy.nodes()",
		"n.width()",
		"n.height()",
		"n.boundingBox()",
		"n.renderedBoundingBox()",
		"n.position()",
		"n.renderedPosition()",
		"n.data('cardWidth')",
		"n.data('cardHeight')",
		"_cy.elements().boundingBox()",
		"_cy.zoom()",
		"_cy.pan()",
		"_cy.width()",
		"_cy.height()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34h-precheck: footprint helper must read %q", want)
		}
	}
	// Card reads — DOM API.
	if !strings.Contains(body, "el.getBoundingClientRect()") {
		t.Error("D34h-precheck: footprint helper must measure card with getBoundingClientRect()")
	}
}

// TestExplorer_D34hPrecheck_FootprintShapeFields pins the exact
// field names from the brief on both the per-node records and the
// graph-level record.
func TestExplorer_D34hPrecheck_FootprintShapeFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugFootprints()")
	if start < 0 {
		t.Fatal("D34h-precheck: _debugFootprints definition missing")
	}
	body := js[start:]

	// Per-node fields exactly as named in the brief.
	for _, want := range []string{
		"id:",
		"kind:",
		"cyDataCardWidth:",
		"cyDataCardHeight:",
		"cyNodeWidth:",
		"cyNodeHeight:",
		"cyNodeBoundingBox:",
		"cyNodeRenderedBoundingBox:",
		"overlayCardRect:",
		"overlayCardWidth:",
		"overlayCardHeight:",
		"widthDelta:",
		"heightDelta:",
		"modelPosition:",
		"renderedPosition:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34h-precheck: per-node record must include %q", want)
		}
	}
	// Graph-level fields.
	for _, want := range []string{
		"cyElementsBoundingBox:",
		"cyZoom:",
		"cyPan:",
		"cyWidth:",
		"cyHeight:",
		"cyNodeCount:",
		"overlayCardCount:",
		"overlayCardUnionRect:",
		"maxWidthDelta:",
		"maxHeightDelta:",
		"averageWidthDelta:",
		"averageHeightDelta:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34h-precheck: graph-level record must include %q", want)
		}
	}
}

// TestExplorer_D34hPrecheck_FootprintIsObservationOnly pins that
// the helper does not mutate cy state — no fit, no resize, no
// layout, no position write, no card style write. Pure read.
func TestExplorer_D34hPrecheck_FootprintIsObservationOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugFootprints()")
	if start < 0 {
		t.Fatal("D34h-precheck: _debugFootprints definition missing")
	}
	end := strings.Index(js[start:], "\n  }\n\n  function _debugState()")
	if end < 0 {
		t.Fatal("D34h-precheck: cannot bound _debugFootprints body")
	}
	body := js[start : start+end]

	for _, banned := range []string{
		"_cy.fit(",
		"_cy.resize(",
		"_cy.layout(",
		"_cy.pan({",
		"_cy.zoom({",
		// Card style WRITES — assignment shape only. D34i added a
		// `card.style.transform` READ to surface the inline
		// transform in diagnostics; reads are legitimate.
		"card.style.transform =",
		"card.style.display =",
		// Position writes.
		".position({",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D34h-precheck: footprint helper must not call %q (observation only)", banned)
		}
	}
}

// TestExplorer_D34hPrecheck_FootprintHandlesNoCy pins the early-exit
// when cy is not built yet (called before install, lens wrong, etc).
func TestExplorer_D34hPrecheck_FootprintHandlesNoCy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		`reason: 'no-cy'`,
		`reason: 'no-nodes'`,
		`reason: 'ok'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34h-precheck: footprint helper must define %q early-exit", want)
		}
	}
}

// TestExplorer_D34hPrecheck_ProductionContextGraphUntouched pins
// the diagnostic does not leak into production. Adding a helper
// must not modify the production Context view or graph-renderer.
func TestExplorer_D34hPrecheck_ProductionContextGraphUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(ctxJS, "renderer.addNode") {
		t.Error("D34h-precheck: production Context view must keep its renderer.addNode path")
	}
	if strings.Contains(ctxJS, "debugFootprints") {
		t.Error("D34h-precheck: production Context view must NOT reference the precheck helper")
	}

	rendererJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")
	if strings.Contains(rendererJS, "debugFootprints") {
		t.Error("D34h-precheck: graph-renderer.js must NOT reference the precheck helper")
	}
}

// ── D34i-context-cytoscape-overlay-two-tier-transform tests ──────────
//
// The projection model is now split into a layer tier and a card
// tier (plugin-inspired design from D34i-precheck review; no
// plugin code copied). These tests pin the resulting structure at
// the source level.

// TestExplorer_D34iTwoTier_LayerSyncAppliesPanZoom pins that the
// layer-tier sync writes a single transform on the overlay element
// using cy.pan() + cy.zoom(). The transform must be `translate(...,
// ...) scale(...)` — exactly the cytoscape projection formula.
func TestExplorer_D34iTwoTier_LayerSyncAppliesPanZoom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _syncLayer()")
	if start < 0 {
		t.Fatal("D34i: _syncLayer function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34i: cannot bound _syncLayer body")
	}
	body := js[start : start+end]
	for _, want := range []string{
		"_cy.pan()",
		"_cy.zoom()",
		"'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')'",
		"transformOrigin = 'top left'",
		"_overlayEl.style",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34i: _syncLayer must include %q", want)
		}
	}
}

// TestExplorer_D34iTwoTier_CardSyncUsesModelPosition pins that the
// card-tier sync writes per-card transforms in MODEL coordinates
// (cy.node.position()), NOT rendered coordinates. The card is
// centred via `translate(-50%, -50%)`. No per-card scale.
func TestExplorer_D34iTwoTier_CardSyncUsesModelPosition(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _syncCards()")
	if start < 0 {
		t.Fatal("D34i: _syncCards function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34i: cannot bound _syncCards body")
	}
	body := js[start : start+end]

	// Positive — MODEL coords, centring translate.
	for _, want := range []string{
		"n.position()",
		"translate(-50%, -50%)",
		"'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)'",
		"card.style.transform",
		// Mirrored selection state still lives here.
		"n.selected()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34i: _syncCards must include %q", want)
		}
	}

	// Negative — _syncCards must NOT use rendered coords or
	// re-apply scale (layer scale already projects).
	for _, banned := range []string{
		"renderedPosition",
		"scale(",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D34i: _syncCards must NOT include %q (layer owns scale/rendered projection)", banned)
		}
	}
}

// TestExplorer_D34iTwoTier_EventBindingsSplit pins the per-tier
// event bindings + constant definitions.
func TestExplorer_D34iTwoTier_EventBindingsSplit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		// Constants.
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
		// Bindings.
		"_cy.on(LAYER_SYNC_EVENTS, _syncLayerBound)",
		"_cy.on(CARDS_SYNC_EVENTS, _syncCardsBound)",
		// Per-tier rAF flags.
		"_syncLayerRaf",
		"_syncCardsRaf",
		// Per-tier bound handlers.
		"_syncLayerBound",
		"_syncCardsBound",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34i: two-tier wiring must include %q", want)
		}
	}
}

// TestExplorer_D34iTwoTier_PanZoomDoesNotWalkCards pins that
// `_syncLayer` does NOT iterate `_cardsByKey` — pan/zoom must cost
// O(1) per event regardless of node count.
func TestExplorer_D34iTwoTier_PanZoomDoesNotWalkCards(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _syncLayer()")
	if start < 0 {
		t.Fatal("D34i: _syncLayer function definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D34i: cannot bound _syncLayer body")
	}
	body := js[start : start+end]
	for _, banned := range []string{
		"_cardsByKey",
		"for (",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D34i: _syncLayer must NOT include %q (pan/zoom must be O(1) per event)", banned)
		}
	}
}

// TestExplorer_D34iTwoTier_NoDomDragRegression pins that the
// architectural change did NOT reintroduce a DOM pointer-delta
// drag path (D34h removed it; D34i must keep it removed).
func TestExplorer_D34iTwoTier_NoDomDragRegression(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	exec := stripJSComments(js)
	for _, banned := range []string{
		"function _wireCardDrag(",
		"DRAG_THRESHOLD_PX",
		"document.addEventListener('pointermove'",
		"document.addEventListener('pointerup'",
		"setPointerCapture(",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D34i: DOM drag artefact must stay removed — found %q", banned)
		}
	}
}

// TestExplorer_D34iTwoTier_CSSOverlayTransformOrigin pins the CSS
// contract that the overlay layer carries `transform-origin: top
// left`. Without this, the layer's `translate(pan)` does not align
// with cy's canvas, and every card visually offsets by
// (1-zoom)*containerSize/2.
func TestExplorer_D34iTwoTier_CSSOverlayTransformOrigin(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	idx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] .context-cy-spike-overlay {`)
	if idx < 0 {
		t.Fatal("D34i: overlay CSS rule missing")
	}
	end := strings.Index(css[idx:], "\n}")
	if end < 0 {
		t.Fatal("D34i: cannot bound overlay CSS rule")
	}
	block := css[idx : idx+end]
	if !strings.Contains(block, "transform-origin: top left") {
		t.Errorf("D34i: overlay rule must declare `transform-origin: top left`; body was:\n%s", block)
	}
}

// TestExplorer_D34iTwoTier_DebugStateAndFootprintFields pins the
// new diagnostic fields. `overlayProjectionModel` identifies the
// active strategy; rendered-delta fields are the load-bearing
// numbers a probe consults to confirm parity.
func TestExplorer_D34iTwoTier_DebugStateAndFootprintFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// debugState fields.
	dsStart := strings.Index(js, "function _debugState()")
	if dsStart < 0 {
		t.Fatal("D34i: _debugState definition missing")
	}
	dsEnd := strings.Index(js[dsStart:], "\n  }\n")
	if dsEnd < 0 {
		t.Fatal("D34i: cannot bound _debugState body")
	}
	dsBody := js[dsStart : dsStart+dsEnd]
	for _, want := range []string{
		"overlayProjectionModel:",
		"overlayLayerTransform:",
		"overlayLayerTransformOrigin:",
	} {
		if !strings.Contains(dsBody, want) {
			t.Errorf("D34i: debugState must expose %q", want)
		}
	}

	// debugFootprints per-node fields + graph-level rendered deltas
	// + projection-model identifier.
	fpStart := strings.Index(js, "function _debugFootprints()")
	if fpStart < 0 {
		t.Fatal("D34i: _debugFootprints definition missing")
	}
	fpBody := js[fpStart:]
	for _, want := range []string{
		// Per-node.
		"overlayCardRenderedWidth:",
		"overlayCardRenderedHeight:",
		"renderedWidthDelta:",
		"renderedHeightDelta:",
		"cardModelTransform:",
		// Graph-level.
		"maxRenderedWidthDelta:",
		"maxRenderedHeightDelta:",
		"averageRenderedWidthDelta:",
		"averageRenderedHeightDelta:",
		"overlayProjectionModel:",
		"overlayLayerTransform:",
		"overlayLayerTransformOrigin:",
	} {
		if !strings.Contains(fpBody, want) {
			t.Errorf("D34i: debugFootprints must include %q", want)
		}
	}

	// Projection model identifier string must match the spec name.
	if !strings.Contains(js, `"layer-pan-zoom-card-model-position"`) &&
		!strings.Contains(js, "'layer-pan-zoom-card-model-position'") {
		t.Error("D34i: PROJECTION_MODEL identifier must be 'layer-pan-zoom-card-model-position'")
	}
}

// TestExplorer_D34iTwoTier_PublicSurfaceExposed pins that the new
// two-tier sync surface + projection constants are exported.
func TestExplorer_D34iTwoTier_PublicSurfaceExposed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"_syncLayer:         _syncLayer",
		"_syncCards:         _syncCards",
		"LAYER_SYNC_EVENTS:  LAYER_SYNC_EVENTS",
		"CARDS_SYNC_EVENTS:  CARDS_SYNC_EVENTS",
		"PROJECTION_MODEL:   PROJECTION_MODEL",
		"debugFootprints: _debugFootprints",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34i: public surface must export %q", want)
		}
	}
}

// TestExplorer_D34iTwoTier_OverlayPointerEventsStillNone pins that
// the projection-model rework did not regress D34h's pointer-
// transparency contract.
func TestExplorer_D34iTwoTier_OverlayPointerEventsStillNone(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	exec := stripCSSComments(css)
	if strings.Contains(exec, "pointer-events: auto") {
		t.Error("D34i: overlay must remain pointer-passive (no pointer-events: auto)")
	}
	if !strings.Contains(exec, "pointer-events: none") {
		t.Error("D34i: overlay must declare `pointer-events: none`")
	}
}

// TestExplorer_D34iTwoTier_AuthorityAndContextProductionUntouched
// pins the rework did NOT alter the production Context view,
// Authority Cytoscape PoC, Authority List Mode, or D34a Authority
// HTML overlay.
func TestExplorer_D34iTwoTier_AuthorityAndContextProductionUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(ctxJS, "renderer.addNode") {
		t.Error("D34i: production Context view must keep its renderer.addNode path")
	}
	if strings.Contains(ctxJS, "contextCytoscapeOverlaySpike") {
		t.Error("D34i: production Context view must NOT reference the spike module")
	}

	authPocJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	if !strings.Contains(authPocJS, "window.MIDASExplorerGraph.cytoscapePoc") {
		t.Error("D34i: Authority Cytoscape PoC public surface must remain")
	}

	authOverlayJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")
	if !strings.Contains(authOverlayJS, "function _wireCardClick(card, nodeId) {") {
		t.Error("D34i: Authority HTML overlay (D34a) must remain unchanged")
	}
}

// ── D34k-context-cytoscape-authority-mount-pattern tests ─────────────
//
// The spike now adopts Authority Cytoscape's mount pattern:
// `#gmap-canvas display: none`, mount inside `.governance-map-
// canvas-scroll` with `position: relative; width/height: 100%;
// min-height: 480px; overflow: hidden; isolation: isolate`. These
// tests pin the new contract at the asset-text level.

// TestExplorer_D34kCSS_CanvasHiddenViaDisplayNone pins the
// canvas-hide change.
func TestExplorer_D34kCSS_CanvasHiddenViaDisplayNone(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	exec := stripCSSComments(css)

	idx := strings.Index(exec, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] #gmap-canvas`)
	if idx < 0 {
		t.Fatal("D34k: spike CSS must scope a rule on #gmap-canvas")
	}
	end := strings.Index(exec[idx:], "}")
	if end < 0 {
		t.Fatal("D34k: cannot bound #gmap-canvas rule body")
	}
	body := exec[idx : idx+end]
	if !strings.Contains(body, "display: none !important") {
		t.Errorf("D34k: #gmap-canvas rule must declare `display: none !important`; got:\n%s", body)
	}
	// Negative — the pre-D34k overrides MUST be absent.
	for _, banned := range []string{
		"display: block !important",
		"width:     100% !important",
		"height:    100% !important",
		"min-width: 0    !important",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D34k: pre-D34k canvas override %q must be removed; got:\n%s", banned, body)
		}
	}
}

// TestExplorer_D34kCSS_NoVisibilityHidden pins that the executable
// spike CSS no longer uses `visibility: hidden`. Comments may
// legitimately reference the historic rule.
func TestExplorer_D34kCSS_NoVisibilityHidden(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	exec := stripCSSComments(css)
	if strings.Contains(exec, "visibility: hidden") {
		t.Error("D34k: executable spike CSS must NOT contain `visibility: hidden` (replaced by display:none on #gmap-canvas)")
	}
}

// TestExplorer_D34kCSS_MountGeometry pins the Authority-style
// in-flow mount geometry.
func TestExplorer_D34kCSS_MountGeometry(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	idx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] .context-cy-spike-mount {`)
	if idx < 0 {
		t.Fatal("D34k: mount CSS rule missing")
	}
	end := strings.Index(css[idx:], "\n}")
	if end < 0 {
		t.Fatal("D34k: cannot bound mount CSS rule")
	}
	block := css[idx : idx+end]

	for _, want := range []string{
		"position: relative",
		"width: 100%",
		"height: 100%",
		"min-height: 480px",
		"overflow: hidden",
		"isolation: isolate",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D34k: mount rule must include %q (Authority-style in-flow geometry); body was:\n%s", want, block)
		}
	}
	// Negative — the pre-D34k inline-absolute mount shape must NOT
	// reappear in CSS, and the high z-index that obscured MIDAS
	// chrome must NOT reappear.
	for _, banned := range []string{
		"position: absolute",
		"inset: 0",
		"z-index: 10",
		"z-index: 100;",
		"contain: layout paint",
	} {
		if strings.Contains(block, banned) {
			t.Errorf("D34k: mount rule must NOT include %q (pre-D34k pattern); body was:\n%s", banned, block)
		}
	}
}

// TestExplorer_D34kJS_MountAppendedToScrollWrapper pins the JS
// mount-location contract.
//
// D35f-retire-transitional-renderer-debt — the legacy fallback that
// mounted into `.governance-map-canvas-scroll` directly is GONE.
// The mount always goes through the GraphViewport host's slot via
// `_installResources(slotEl)` → `parentEl.appendChild(_mountEl)`,
// where `slotEl` is the host-supplied `.midas-graph-renderer-slot`.
// Pre-D35f the `getElementsByClassName('governance-map-canvas-
// scroll')[0]` lookup also existed as a fallback parent; that
// fallback is retired in D35f.
func TestExplorer_D34kJS_MountAppendedToScrollWrapper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		// `_installResources(parentEl)` is the shared install body.
		// It is called from the factory's mount with the host's
		// `slotEl` as parentEl (the strategic mount target).
		"function _installResources(parentEl)",
		"parentEl.appendChild(_mountEl)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35f: install must include %q (host-routed mount pattern)", want)
		}
	}

	// Negative — the legacy scroll-wrapper fallback is RETIRED
	// from the install path. Diagnostic-only lookups (e.g.
	// _debugState reporting the scroll wrapper's rect) remain.
	// Pin the absence inside the install function body specifically.
	exec := stripJSComments(js)
	installStart := strings.Index(exec, "function install(options) {")
	if installStart < 0 {
		t.Fatal("D35f: install definition missing in exec")
	}
	installEnd := strings.Index(exec[installStart:], "\n  }\n")
	if installEnd < 0 {
		t.Fatal("D35f: cannot bound install body in exec")
	}
	installBody := exec[installStart : installStart+installEnd]
	if strings.Contains(installBody, "getElementsByClassName('governance-map-canvas-scroll')[0]") {
		t.Error("D35f: legacy `.governance-map-canvas-scroll` fallback in install() must be retired (host is always available)")
	}
	if strings.Contains(installBody, "_lastInstallReason = 'no-scroll-wrapper'") {
		t.Error("D35f: 'no-scroll-wrapper' failure reason is for the retired fallback; D35f uses 'host-unavailable'")
	}

	// Negative pins — the pre-D34k append-to-canvas + inline
	// absolute sizing must be gone from EXECUTABLE JS. (Comments
	// legitimately reference the retired pattern as documentation.)
	// Reuses the `exec` variable declared above for the negative
	// scroll-wrapper-fallback pin.
	for _, banned := range []string{
		"canvas.appendChild(_mountEl)",
		"_mountEl.style.position = 'absolute'",
		"_mountEl.style.left     = '0'",
		"_mountEl.style.top      = '0'",
		"_mountEl.style.right    = '0'",
		"_mountEl.style.bottom   = '0'",
		"_mountEl.style.width    = '100%'",
		"_mountEl.style.height   = '100%'",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D34k: executable JS must NOT include pre-D34k mount pattern %q", banned)
		}
	}
}

// TestExplorer_D34kJS_DebugStateGeometryFields pins the new
// canvas-alignment diagnostic fields on debugState().
func TestExplorer_D34kJS_DebugStateGeometryFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	start := strings.Index(js, "function _debugState()")
	if start < 0 {
		t.Fatal("D34k: _debugState definition missing")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D34k: cannot bound _debugState body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		"mountParentSelector:",
		"canvasDisplay:",
		"sceneDisplay:",
		"sceneVisibility:",
		"scrollWrapperRect:",
		"scrollWrapperScrollWidth:",
		"scrollWrapperScrollHeight:",
		"scrollWrapperClientWidth:",
		"scrollWrapperClientHeight:",
		"scrollWrapperOverflowX:",
		"scrollWrapperOverflowY:",
		"mountRect:",
		"legendOverlayRect:",
		"cameraClusterRect:",
		"modeRailRect:",
		"legendOverlayVisible:",
		"cameraClusterVisible:",
		"modeRailVisible:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D34k: debugState must expose %q", want)
		}
	}
}

// TestExplorer_D34kPreserved_D34iTwoTierTransform pins that D34i's
// two-tier transform model was NOT changed by the D34k rework.
func TestExplorer_D34kPreserved_D34iTwoTierTransform(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Layer-tier transform.
	for _, want := range []string{
		"function _syncLayer()",
		"_cy.pan()",
		"_cy.zoom()",
		"'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')'",
		"_overlayEl.style.transform",
		"transformOrigin = 'top left'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34k: D34i two-tier layer transform regressed — missing %q", want)
		}
	}
	// Card-tier transform.
	for _, want := range []string{
		"function _syncCards()",
		"n.position()",
		"'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)'",
		// Projection model identifier — single-quoted in source.
		"'layer-pan-zoom-card-model-position'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34k: D34i two-tier card transform regressed — missing %q", want)
		}
	}
}

// TestExplorer_D34kPreserved_ConnectorLogic pins that the
// connector capture / kind / per-kind style code was NOT changed
// by D34k. The brief forbids connector changes in this tranche.
func TestExplorer_D34kPreserved_ConnectorLogic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"function _captureSceneEdges(svg, validIds)",
		"el.getAttribute('data-source-node-id')",
		"el.getAttribute('data-target-node-id')",
		"el.getAttribute('data-connector-kind')",
		"'cy-conn-' + safe",
		// Per-kind palette retained.
		"selector: 'edge.cy-conn-service'",
		"selector: 'edge.cy-conn-ai_binding'",
		"selector: 'edge.cy-conn-coverage_gap'",
		"selector: 'edge.cy-conn-authority'",
		"selector: 'edge.cy-conn-evidence'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D34k: connector logic regressed — missing %q", want)
		}
	}
}

// TestExplorer_D34kPreserved_AuthorityAndContextProductionUntouched
// pins that production Context view, Authority Cytoscape PoC, and
// D34a Authority HTML overlay remain intact.
func TestExplorer_D34kPreserved_AuthorityAndContextProductionUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(ctxJS, "renderer.addNode") {
		t.Error("D34k: production Context view must keep its renderer.addNode path")
	}
	if strings.Contains(ctxJS, "contextCytoscapeOverlaySpike") {
		t.Error("D34k: production Context view must NOT reference the spike module")
	}

	authPocJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	if !strings.Contains(authPocJS, "window.MIDASExplorerGraph.cytoscapePoc") {
		t.Error("D34k: Authority Cytoscape PoC public surface must remain")
	}

	authOverlayJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")
	if !strings.Contains(authOverlayJS, "function _wireCardClick(card, nodeId) {") {
		t.Error("D34k: Authority HTML overlay (D34a) must remain unchanged")
	}

	// Authority Cytoscape CSS keeps its #gmap-canvas hide; D35f
	// re-keyed it from body-class to host-owned renderer identity.
	authPocCSS := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	if !strings.Contains(authPocCSS, `.midas-graph-viewport[data-active-renderer="authority"] #gmap-canvas`) {
		t.Error("D35f: Authority PoC CSS must keep its #gmap-canvas hide (re-keyed onto host renderer identity)")
	}
}
