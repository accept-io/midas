package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37q-viewport-4-impl — Safe-Area Camera Contract Hardening tests.
//
// Two bounded gaps identified by D37q-viewport-1 are closed by this
// tranche:
//
//   1. graphCameraBus `getZoom()` canonical unit is RATIO. Authority's
//      bus delegate previously returned an integer percent (e.g. 100)
//      via `poc.getZoomPercent()`; D37q-viewport-4-impl converts that
//      to a ratio (e.g. 1.0) at the delegate boundary while keeping
//      `poc.getZoomPercent()` itself unchanged for the toolbar zoom
//      badge.
//
//   2. Authority safe-area consumption is per-edge at the host
//      boundary (`_safeAreaPadding` reads all four edges from
//      `_rendererCtx.getSafeArea()` and the runtime fit
//      `_fitToAvailableCanvas` applies per-edge insets through
//      `cy.viewport({zoom, pan})`). The scalar collapse at the
//      Cytoscape preset-layout boundary is forced by Cytoscape's API
//      (which accepts only a scalar `padding` value at construction);
//      this limitation is documented in the new comment block in
//      `authority-cytoscape-poc.js` and pinned here as the current
//      state. A future tranche could bypass the preset-layout API to
//      achieve per-edge at the initial mount too — explicitly out of
//      scope per the D37q-viewport-4 directive.

const (
	d37qV4ViewportAsset            = "/explorer/assets/js/graph/graph-viewport.js"
	d37qV4CameraBusAsset           = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37qV4CameraControllerAsset    = "/explorer/assets/js/graph/graph-platform/graph-camera-controller.js"
	d37qV4CameraToolbarAdapter     = "/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js"
	d37qV4ContextRendererAsset     = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37qV4AuthorityPocAsset        = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37qV4AuthorityToolbarAsset    = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
	d37qV4AuthorityEdgeAsset       = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37qV4ContextConnectorPainter  = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37qV4StageAsset               = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37qV4LegacyCameraAsset        = "/explorer/assets/js/graph/graph-camera.js"
)

// ── 1. GraphViewport safe-area is per-edge ────────────────────────

func TestExplorer_D37qViewport4_GraphViewportSafeAreaIsPerEdge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV4ViewportAsset)

	// The host must expose `getSafeArea` and produce an object with
	// per-edge keys. We assert the load-bearing substrings — both the
	// function definition and the four return-shape keys — without
	// over-pinning the implementation arithmetic.
	if !strings.Contains(js, "function getSafeArea()") {
		t.Errorf("D37q-viewport-4-impl: GraphViewport must expose getSafeArea()")
	}
	// Per-edge return-shape keys appear inside the rounded-output
	// object literal at the end of getSafeArea.
	if !regexp.MustCompile(`return\s*\{\s*top:[\s\S]+right:[\s\S]+bottom:[\s\S]+left:`).MatchString(js) {
		t.Errorf("D37q-viewport-4-impl: getSafeArea() must return per-edge {top, right, bottom, left}")
	}
	// Chrome-aware inset computation must still walk per-edge.
	//
	// D37s-viewport-fit-1-impl flip: the pre-tranche implementation
	// computed per-edge insets via variables named `topInset`,
	// `leftInset`, `bottomInset`, `rightInset`. The new strategic
	// implementation replaces the quadrant heuristic with an
	// intersection-rectangle + aspect-based attribution algorithm,
	// using variables named `topReach`, `leftReach`, `bottomReach`,
	// `rightReach` (each describing how far the chrome's overlap
	// reaches into the viewport from that edge). The CONTRACT — per-
	// edge computation, four-side awareness — is preserved; only the
	// variable names changed. Pin the new names so a regression to
	// the broken quadrant heuristic would be visible.
	for _, want := range []string{
		"topReach",
		"rightReach",
		"bottomReach",
		"leftReach",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37s-viewport-fit-1-impl (flipped): GraphViewport safe-area must compute per-edge reach %q via intersection-rect + aspect-based attribution (replaces pre-tranche quadrant heuristic)", want)
		}
	}
}

// ── 2. Strategic Context consumes per-edge safe-area ──────────────

func TestExplorer_D37qViewport4_ContextConsumesPerEdgeSafeArea(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV4ContextRendererAsset)

	// The strategic Context renderer plumbs `ctx.getSafeArea` into
	// the camera target so the controller's fit math can consume the
	// host's per-edge values.
	if !regexp.MustCompile(`getSafeArea:\s*function\s*\(\s*\)\s*\{[\s\S]*?_resolveSafeArea\(\)`).MatchString(js) {
		t.Errorf("D37q-viewport-4-impl: strategic Context camera target must expose getSafeArea that resolves the host's per-edge insets")
	}
	if !strings.Contains(js, "function _resolveSafeArea()") {
		t.Errorf("D37q-viewport-4-impl: strategic Context must declare _resolveSafeArea() helper to read host safe-area")
	}
}

// ── 3. Authority consumes per-edge safe-area at the host boundary ─

// TestExplorer_D37qViewport4_AuthorityConsumesPerEdgeSafeArea asserts
// that Authority reads ALL FOUR edges of the host's getSafeArea —
// not just one collapsed scalar. The downstream scalar collapse at
// the Cytoscape preset-layout boundary is forced by Cytoscape's API
// (separately pinned by
// TestExplorer_D37qViewport4_AuthorityCytoscapeScalarLimitationDocumented).
func TestExplorer_D37qViewport4_AuthorityConsumesPerEdgeSafeArea(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV4AuthorityPocAsset)

	// Authority must resolve the host safe-area via ctx.getSafeArea().
	if !strings.Contains(js, "_rendererCtx.getSafeArea()") {
		t.Errorf("D37q-viewport-4-impl: Authority must consume the host safe-area via _rendererCtx.getSafeArea()")
	}
	// Authority must read all four edges from the returned object.
	for _, want := range []string{
		"sa.top",
		"sa.right",
		"sa.bottom",
		"sa.left",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-4-impl: Authority must read per-edge safe-area value %q from the host", want)
		}
	}
	// The runtime fit (`_fitToAvailableCanvas`) applies per-edge
	// insets through cy.viewport({zoom, pan}). Pin the per-edge
	// variable names + the viewport application.
	for _, want := range []string{
		"function _fitToAvailableCanvas(cy, opts)",
		"var L = FIT_SIDE_BUFFER_PX",
		"var R = FIT_SIDE_BUFFER_PX",
		"var T = FIT_TOP_BUFFER_PX",
		"var B = TOOLBAR_BOTTOM_RESERVED_PX",
		"cy.viewport(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-4-impl: Authority runtime fit must keep per-edge L/R/T/B insets routed through cy.viewport({zoom, pan}) (%q)", want)
		}
	}
}

// ── 4. Authority safe-area floor preserved ────────────────────────

// TestExplorer_D37qViewport4_AuthoritySafeAreaFloorPreserved pins
// the CSS-token floor + clamp behaviour that protects against
// pathological mount dimensions. These are minimum-padding
// guarantees that survive any per-edge / scalar collapse decision.
func TestExplorer_D37qViewport4_AuthoritySafeAreaFloorPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV4AuthorityPocAsset)

	for _, want := range []string{
		// CSS-token floor (legacy `--gmap-overlay-inset-*` reads).
		"--gmap-overlay-inset-top",
		"--gmap-overlay-inset-right",
		"--gmap-overlay-inset-bottom",
		"--gmap-overlay-inset-left",
		// Mount-dimension clamp + floor constants.
		"FIT_PADDING_CAP_DIVISOR",
		"FIT_PADDING_FLOOR",
		"FIT_PADDING_HEADLESS",
		// Degenerate-viewport guard inside _fitToAvailableCanvas.
		"FIT_MIN_VISIBLE_PX",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-4-impl: Authority safe-area floor must remain (%q)", want)
		}
	}
}

// TestExplorer_D37qViewport4_AuthorityCytoscapeScalarLimitationDocumented
// pins the explicit comment block that documents WHY Authority's
// `_safeAreaPadding` collapses per-edge insets to a scalar at the
// Cytoscape preset-layout boundary. This is a Cytoscape API
// limitation, not architectural debt; the comment records the
// rationale so a future tranche knows the deferred work.
func TestExplorer_D37qViewport4_AuthorityCytoscapeScalarLimitationDocumented(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV4AuthorityPocAsset)

	if !strings.Contains(js, "D37q-viewport-4-impl") {
		t.Errorf("D37q-viewport-4-impl: Authority safe-area path must carry a D37q-viewport-4-impl explanatory block")
	}
	// The comment block must mention all three load-bearing facts.
	for _, want := range []string{
		"SCALAR padding only",
		"_fitToAvailableCanvas",
		"per-edge",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-4-impl: limitation comment must mention %q", want)
		}
	}
}

// ── 5. Camera-bus canonical getZoom() unit documented ─────────────

// TestExplorer_D37qViewport4_CameraBusGetZoomCanonicalUnitDocumented
// pins the canonical unit at the test-suite level (the bus module
// itself does not declare a unit per command — it dispatches opaque
// payloads). The canonical-unit decision is recorded here so future
// renderers can find it.
func TestExplorer_D37qViewport4_CameraBusGetZoomCanonicalUnitDocumented(t *testing.T) {
	for _, want := range []string{
		"canonical",
		"ratio",
		"1.0 = 100%",
	} {
		if !strings.Contains(cameraBusGetZoomCanonicalUnit, want) {
			t.Errorf("D37q-viewport-4-impl: canonical getZoom() unit documentation must mention %q", want)
		}
	}
}

// cameraBusGetZoomCanonicalUnit captures the canonical unit decision
// for the camera-bus `getZoom()` command. New renderer delegates MUST
// return a ratio (e.g. 0.5, 1.0, 2.0) from their `getZoom()`
// implementation. Display layers may convert ratio → percent for
// UI (toolbar zoom-percent badge); the conversion is a display
// concern, not a contract concern.
const cameraBusGetZoomCanonicalUnit = `
graphCameraBus.getZoom() canonical unit (D37q-viewport-4-impl)
==============================================================

Canonical unit: ratio.
  1.0 = 100% (no zoom)
  0.5 = 50%  (zoomed out)
  2.0 = 200% (zoomed in)

Every renderer delegate's getZoom() function MUST return a ratio.

Display layers (e.g. the gmap-zoom-percent badge) may convert ratio
to percent for UI presentation. The conversion is a display concern,
not a contract concern. Toolbar code reads percent through the
engine's own per-percent helper (e.g. cytoscapePoc.getZoomPercent)
where one exists; the bus delegate returns ratio.

Cross-lens consumers reading graphCameraBus.getZoom() get a single
consistent unit across native-context, context, and authority.
`

// ── 6. native-context delegate returns ratio ──────────────────────

// TestExplorer_D37qViewport4_NativeContextGetZoomReturnsRatio pins
// that the toolbar adapter's `native-context` delegate returns the
// legacy camera's `cam.getZoom()` value. The legacy camera stores
// zoom as a ratio (`_state.zoom` defaults to 1.0 in graph-camera.js),
// so the delegate returns a ratio by construction.
func TestExplorer_D37qViewport4_NativeContextGetZoomReturnsRatio(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Toolbar adapter's native-context delegate returns cam.getZoom()
	// directly — no percent conversion.
	adapter := getExplorerAsset(t, srv, d37qV4CameraToolbarAdapter)
	if !regexp.MustCompile(`getZoom:\s*function\s*\(\s*\)\s*\{[\s\S]*?return cam\.getZoom\(\);`).MatchString(adapter) {
		t.Errorf("D37q-viewport-4-impl: native-context delegate getZoom() must return cam.getZoom() (ratio)")
	}

	// Legacy graph-camera.js stores zoom as a ratio (default 1.0).
	legacy := getExplorerAsset(t, srv, d37qV4LegacyCameraAsset)
	if !regexp.MustCompile(`function getZoom\(\)\s*\{\s*return _state\.zoom;\s*\}`).MatchString(legacy) {
		t.Errorf("D37q-viewport-4-impl: legacy graph-camera.js getZoom() must return _state.zoom (ratio)")
	}
	if !strings.Contains(legacy, "_state.zoom = 1.0") {
		t.Errorf("D37q-viewport-4-impl: legacy camera default zoom must be 1.0 (ratio identity)")
	}
}

// ── 7. Context delegate returns ratio ─────────────────────────────

// TestExplorer_D37qViewport4_ContextGetZoomReturnsRatio pins that
// both the spatial-mode delegate and the non-spatial fallback return
// a ratio. The spatial builder wraps `graphCameraController.create()`
// (default 1.0 ratio); the fallback returns the constant 1.
func TestExplorer_D37qViewport4_ContextGetZoomReturnsRatio(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Spatial builder's getZoom returns camera.getZoom() directly.
	ctxJs := getExplorerAsset(t, srv, d37qV4ContextRendererAsset)
	if !regexp.MustCompile(`getZoom:\s*function\s*\(\s*\)\s*\{\s*return\s*\(camera && typeof camera\.getZoom === 'function'\) \? camera\.getZoom\(\) : null;\s*\}`).MatchString(ctxJs) {
		t.Errorf("D37q-viewport-4-impl: strategic Context spatial delegate getZoom() must return camera.getZoom() (ratio)")
	}

	// Fallback builder's getZoom returns the constant 1.
	if !regexp.MustCompile(`getZoom:\s*function\s*\(\s*\)\s*\{\s*return 1;\s*\}`).MatchString(ctxJs) {
		t.Errorf("D37q-viewport-4-impl: strategic Context fallback delegate getZoom() must return 1 (ratio identity)")
	}

	// Camera controller's getZoom returns _transform.zoom (ratio).
	ctrl := getExplorerAsset(t, srv, d37qV4CameraControllerAsset)
	if !regexp.MustCompile(`function getZoom\(\)\s*\{\s*return _transform\.zoom;\s*\}`).MatchString(ctrl) {
		t.Errorf("D37q-viewport-4-impl: graphCameraController.getZoom() must return _transform.zoom (ratio)")
	}
	if !strings.Contains(ctrl, "var DEFAULT_ZOOM                = 1.0;") {
		t.Errorf("D37q-viewport-4-impl: graphCameraController DEFAULT_ZOOM must be 1.0 (ratio identity)")
	}
}

// ── 8. Authority delegate returns ratio (converts from percent) ───

// TestExplorer_D37qViewport4_AuthorityGetZoomReturnsRatio pins the
// authoritative change: Authority's bus delegate must convert the
// engine's percent (returned by `poc.getZoomPercent()`) to a ratio
// before returning it through the camera bus. The display helper
// `poc.getZoomPercent()` remains unchanged for the toolbar badge.
func TestExplorer_D37qViewport4_AuthorityGetZoomReturnsRatio(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV4AuthorityPocAsset)

	// The Authority delegate body must invoke the engine's percent
	// helper AND divide by 100 to convert to ratio.
	if !strings.Contains(js, "poc.getZoomPercent()") {
		t.Errorf("D37q-viewport-4-impl: Authority delegate must still read the engine's percent helper")
	}
	if !regexp.MustCompile(`return pct / 100;`).MatchString(js) {
		t.Errorf("D37q-viewport-4-impl: Authority delegate must convert percent to ratio (pct / 100)")
	}
	// Guard against non-numeric / non-finite / non-positive returns.
	if !regexp.MustCompile(`pct\s*<=\s*0`).MatchString(js) {
		t.Errorf("D37q-viewport-4-impl: Authority delegate must guard against non-positive zoom percent")
	}
}

// ── 9. Toolbar zoom badge still shows percent ─────────────────────

// TestExplorer_D37qViewport4_ToolbarZoomDisplayStillConvertsForDisplay
// pins that the Authority toolbar's zoom-percent badge continues to
// read from `poc.getZoomPercent()` (integer percent). The bus
// delegate's unit change does NOT alter the visible badge — the
// display layer keeps its own conversion.
func TestExplorer_D37qViewport4_ToolbarZoomDisplayStillConvertsForDisplay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tbJs := getExplorerAsset(t, srv, d37qV4AuthorityToolbarAsset)

	// The toolbar's zoom badge code still reads percent directly
	// from the engine helper.
	for _, want := range []string{
		"poc.getZoomPercent",
		"renderZoomPercent",
	} {
		if !strings.Contains(tbJs, want) {
			t.Errorf("D37q-viewport-4-impl: Authority toolbar zoom badge must continue reading the engine's percent helper (%q)", want)
		}
	}
	// Render the badge as `percent + '%'` — the toolbar still owns
	// the percent → display conversion.
	if !regexp.MustCompile(`percent\s*\+\s*'%'`).MatchString(tbJs) {
		t.Errorf("D37q-viewport-4-impl: Authority toolbar must continue to render the zoom badge as `percent + '%%'`")
	}
}

// ── 10. Connector routing untouched ───────────────────────────────

func TestExplorer_D37qViewport4_NoConnectorRoutingChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Context connector painter public surface intact.
	painter := getExplorerAsset(t, srv, d37qV4ContextConnectorPainter)
	for _, want := range []string{
		"window.MIDASExplorerGraph.contextConnectorPainter",
		"paintConnectors",
	} {
		if !strings.Contains(painter, want) {
			t.Errorf("D37q-viewport-4-impl: Context connector painter must remain intact (%q)", want)
		}
	}

	// graphStage anchor contract intact.
	stage := getExplorerAsset(t, srv, d37qV4StageAsset)
	for _, want := range []string{
		"window.MIDASExplorerGraph.graphStage",
		"compose",
		"anchorOf",
		"fitBoundsOf",
	} {
		if !strings.Contains(stage, want) {
			t.Errorf("D37q-viewport-4-impl: graphStage contract must remain intact (%q)", want)
		}
	}

	// D37q-viewport-2-impl flipped this temporal pin: the painter now
	// prefers stage anchors when an `opts.stage` StageModel is
	// supplied, while preserving the DOM-measured centroid path as a
	// per-endpoint fallback. The detailed contract tests for the
	// stage-anchor preference live in
	// explorer_context_connector_stage_contract_test.go.
	if !strings.Contains(painter, "stageMod.anchorOf") {
		t.Errorf("D37q-viewport-2-impl: Context connector painter must now resolve endpoints via the shared graphStage anchor contract (stageMod.anchorOf)")
	}
}

// ── 11. Authority canvas-edge untouched ───────────────────────────

func TestExplorer_D37qViewport4_AuthorityCanvasEdgeUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV4AuthorityEdgeAsset)

	// Canvas-edge module public surface intact.
	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityCanvasEdgeTabs",
		"registerLensProvider('authority',",
		"init:",
		"destroy:",
		"openTab:",
		"closePane:",
		"syncSelection:",
		"isOpen:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-4-impl: Authority canvas-edge module must remain intact (%q)", want)
		}
	}
}

// ── 12. Renderer-contract suite compatible with new behaviour ─────

// TestExplorer_D37qViewport4_RendererContractTestsStillCompatible
// reasserts the load-bearing invariants from D37q-viewport-7's
// renderer-contract suite that intersect with this tranche. They
// must all continue to hold after D37q-viewport-4-impl's small
// changes.
func TestExplorer_D37qViewport4_RendererContractTestsStillCompatible(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Camera-bus is still the dispatch surface; locked commands
	// include getZoom.
	bus := getExplorerAsset(t, srv, d37qV4CameraBusAsset)
	if !strings.Contains(bus, "'getZoom'") {
		t.Errorf("D37q-viewport-4-impl: graphCameraBus must still declare 'getZoom' in its locked command vocabulary")
	}

	// Authority retains its lens registration on the bus.
	authJs := getExplorerAsset(t, srv, d37qV4AuthorityPocAsset)
	if !strings.Contains(authJs, "bus.registerLens('authority',") {
		t.Errorf("D37q-viewport-4-impl: Authority must still register an 'authority' camera-bus delegate")
	}

	// Context retains its lens registration on the bus (strategic +
	// fallback paths).
	ctxJs := getExplorerAsset(t, srv, d37qV4ContextRendererAsset)
	if !strings.Contains(ctxJs, "bus.registerLens(RENDERER_ID, delegate)") {
		t.Errorf("D37q-viewport-4-impl: strategic Context must still register a camera-bus delegate via bus.registerLens(RENDERER_ID, ...)")
	}

	// Native-context delegate intact.
	adapter := getExplorerAsset(t, srv, d37qV4CameraToolbarAdapter)
	if !strings.Contains(adapter, "bus.registerLens('native-context', _legacyDelegate())") {
		t.Errorf("D37q-viewport-4-impl: native-context camera-bus delegate must remain registered by the toolbar adapter")
	}
}
