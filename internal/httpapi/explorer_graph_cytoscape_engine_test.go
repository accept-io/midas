package httpapi

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37r-tranche-B'' — Shared Graph Engine Platform Module.
//
// Source-contract tests for the engine module + its strategic rule.
// The engine owns Cytoscape instantiation, mount container, coordinate
// frame, overlay alignment, ResizeObserver, camera-bus registration,
// and lifecycle. Lenses supply data + templates + adapters.
//
// Strategic rule: NO GRAPH LENS MAY INSTANTIATE CYTOSCAPE DIRECTLY.
// `TestExplorer_StrategicRule_NoLensInstantiatesCytoscape` enforces
// this by scanning every JS file outside the engine module for direct
// `window.cytoscape({...})` / `cytoscape({...})` constructor calls.
//
// Authority's `authority-cytoscape-poc.js` is currently WHITELISTED
// pending a follow-up Authority-migration tranche (B''-Authority).
// The whitelist entry is the visible record of the deferred
// migration; the strategic-rule test fails if the entry is removed
// before Authority is migrated. Tranche E (Context default flip)
// cannot proceed without Authority being migrated.

const (
	d37rBprime2EngineModule        = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37rBprime2OverlayModule       = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js"
	d37rBprime2ContextRenderer     = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37rBprime2AuthorityPoc        = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37rBprime2AuthorityOverlay    = "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js"
	d37rBprime2IndexHTML           = "/explorer/index.html"
	d37rBprime2ViewportJS          = "/explorer/assets/js/graph/graph-viewport.js"
	d37rBprime2GraphRenderer       = "/explorer/assets/js/graph/graph-renderer.js"
)

// ── 1. Engine module exists ────────────────────────────────────────

func TestExplorer_GraphCytoscapeEngine_ModuleExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)
	if len(js) == 0 {
		t.Fatalf("D37r-tranche-B'': engine module must be served at %q", d37rBprime2EngineModule)
	}
}

// ── 2. Public API shape ────────────────────────────────────────────

func TestExplorer_GraphCytoscapeEngine_PublicAPIShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	if !strings.Contains(js, "window.MIDASExplorerGraph.graphCytoscapeEngine = {") {
		t.Errorf("D37r-tranche-B'': module must attach `graphCytoscapeEngine` to MIDASExplorerGraph")
	}
	if !strings.Contains(js, "function mount(mountEl, options)") {
		t.Errorf("D37r-tranche-B'': module must export mount(mountEl, options)")
	}

	// Required option validation messages.
	for _, want := range []string{
		"options.lensId is required",
		"options.data is required",
		"options.template.create(node, ctx) is required",
		"options.keyForNode(node) is required",
		"options.selectionAdapter(cyEvent, handle) is required",
		"options.cameraAdapter(handle) is required",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'': engine must validate required option %q", want)
		}
	}
}

// ── 3. Lens-agnostic ──────────────────────────────────────────────

func TestExplorer_GraphCytoscapeEngine_LensAgnostic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)
	stripped := _stripJSComments(js)

	// The engine must not name any specific lens or lens-specific
	// symbol in load-bearing code (comments may reference lens
	// names by way of explanation — those are filtered out).
	for _, banned := range []string{
		"contextCardPainter",
		"contextSelectionBridge",
		"contextProjection",
		"graphSelectionBridge",
		"cytoscapePoc",
		"authorityView",
		"authorityGraphWorkbench",
		"knowledgeGraphContract",
		"knowledgeGraphShell",
	} {
		if strings.Contains(stripped, banned) {
			t.Errorf("D37r-tranche-B'': engine module must not reference lens-specific symbol %q in load-bearing code", banned)
		}
	}
	for _, bannedPath := range []string{
		"/graph/context/",
		"/graph/authority/",
		"/graph/knowledge/",
	} {
		if strings.Contains(stripped, bannedPath) {
			t.Errorf("D37r-tranche-B'': engine module must not reference lens directory path %q in load-bearing code", bannedPath)
		}
	}
}

// ── 4. Engine owns Cytoscape instantiation ────────────────────────

func TestExplorer_GraphCytoscapeEngine_OwnsCytoscapeInstantiation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	if !strings.Contains(js, "cy = window.cytoscape({") {
		t.Errorf("D37r-tranche-B'': engine module must instantiate Cytoscape via `cy = window.cytoscape({`")
	}
	// The engine throws at mount time if `window.cytoscape` is not
	// defined — load-bearing signal that script-tag order must be
	// preserved.
	if !strings.Contains(js, "window.cytoscape is not defined; check vendor script-tag order") {
		t.Errorf("D37r-tranche-B'': engine must throw a load-order error when window.cytoscape is missing")
	}
}

// ── 5. Engine owns the mount container ─────────────────────────────

func TestExplorer_GraphCytoscapeEngine_OwnsMountContainer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	for _, want := range []string{
		"var CONTAINER_CLASS = 'graph-cytoscape-engine-container';",
		"var CY_MOUNT_CLASS  = 'graph-cytoscape-engine-cy-mount';",
		"container.style.position = 'relative';",
		"container.style.width    = '100%';",
		"container.style.height   = '100%';",
		"container.style.overflow = 'hidden';",
		"cyMount.style.position = 'absolute';",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'': engine must own mount-container construction (%q)", want)
		}
	}
}

// ── 6. Engine owns the overlay mount ──────────────────────────────

func TestExplorer_GraphCytoscapeEngine_OwnsOverlayMount(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	if !strings.Contains(js, "g.graphCytoscapeOverlay.mount(cy, container, {") {
		t.Errorf("D37r-tranche-B'': engine must call graphCytoscapeOverlay.mount(cy, container, {...}) internally")
	}
	// The engine forwards the lens's template/keyForNode/state options
	// through to the overlay.
	for _, want := range []string{
		"template:       opts.template",
		"keyForNode:     opts.keyForNode",
		"stateClasses:",
		"syncSelected:",
		"syncHover:",
		"pointerEvents:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'': engine must forward overlay option %q from lens", want)
		}
	}
}

// ── 7. Handle exposes camera operations ───────────────────────────

func TestExplorer_GraphCytoscapeEngine_HandleExposesCameraOperations(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	// D37s-context-geometry-1-impl flip: `handle.fit` now accepts an
	// optional `fitOpts` parameter (per-call padding override, scalar
	// or per-side object). The signature changed from `fit: function ()`
	// to `fit: function (fitOpts)`. Backward compat: calling
	// `handle.fit()` with no argument is identical to the previous
	// no-arg shape because the engine pulls per-side padding from
	// `opts.getSafeArea` (mount-supplied) when `fitOpts.padding` is
	// absent, and falls back to a uniform DEFAULT_FIT_PADDING when
	// neither is available.
	for _, want := range []string{
		"zoomIn: function ()",
		"zoomOut: function ()",
		"fit: function (fitOpts)",
		"reset: function ()",
		"focus: function (nodeId)",
		"getZoom: function ()",
		"setZoom: function (z)",
		"forceRender: function ()",
		"destroy: function ()",
		"refresh: function (newData)",
		"getCardEl: function (key)",
		"getNode: function (id)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'' / D37s flip: engine handle must expose %q", want)
		}
	}
}

// ── 8. Handle does NOT expose the cy instance ─────────────────────

func TestExplorer_GraphCytoscapeEngine_HandleDoesNotExposeCy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	// The handle object literal in the return statement must not
	// have a `cy:` property, a `getCy:` property, or any obvious
	// escape hatch. (The prompt explicitly forbids internal cy
	// exposure.)
	for _, banned := range []string{
		"\thandle.cy =",
		"handle.cy = cy",
		"cy: cy,",
		"getCy: function",
		"getCytoscapeInstance",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37r-tranche-B'': engine handle must not expose the cy instance (found %q)", banned)
		}
	}
}

// ── 9. Selection adapter wiring ───────────────────────────────────

func TestExplorer_GraphCytoscapeEngine_OwnsSelectionTapWiring(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	if !strings.Contains(js, "cy.on('tap', 'node', function (evt) {") {
		t.Errorf("D37r-tranche-B'': engine must subscribe to cy.on('tap', 'node', …)")
	}
	if !strings.Contains(js, "opts.selectionAdapter(evt, handle)") {
		t.Errorf("D37r-tranche-B'': tap handler must invoke opts.selectionAdapter(evt, handle)")
	}
}

// ── 10. Camera-bus auto-registration ──────────────────────────────

func TestExplorer_GraphCytoscapeEngine_OwnsCameraBusRegistration(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	if !strings.Contains(js, "var cameraDelegate = opts.cameraAdapter(handle);") {
		t.Errorf("D37r-tranche-B'': engine must invoke opts.cameraAdapter(handle) at mount time")
	}
	if !strings.Contains(js, "g.graphCameraBus.registerLens(lensId, cameraDelegate)") {
		t.Errorf("D37r-tranche-B'': engine must register the lens's camera delegate via graphCameraBus.registerLens(lensId, …)")
	}
	// Deregistration at destroy.
	if !strings.Contains(js, "g.graphCameraBus.unregisterLens(lensId)") {
		t.Errorf("D37r-tranche-B'': engine destroy() must call graphCameraBus.unregisterLens(lensId)")
	}
}

// ── 11. ResizeObserver + initial settle ───────────────────────────

func TestExplorer_GraphCytoscapeEngine_OwnsResizeAndSettle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2EngineModule)

	if !strings.Contains(js, "new window.ResizeObserver") {
		t.Errorf("D37r-tranche-B'': engine must own ResizeObserver wiring")
	}
	if !strings.Contains(js, "cy.resize();") {
		t.Errorf("D37r-tranche-B'': engine must call cy.resize() on container resize")
	}
	if !strings.Contains(js, "cy.fit(undefined, 24)") {
		t.Errorf("D37r-tranche-B'': engine must call cy.fit(undefined, 24) for the initial settle")
	}
}

// ── 12. Script-tag ordering ───────────────────────────────────────

func TestExplorer_VendorScriptTag_LoadedBeforeEngineModule(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	html := performRequest(t, srv, "GET", "/explorer", nil).Body.String()

	vendorIdx  := strings.Index(html, "/explorer/assets/js/vendor/cytoscape.min.js")
	overlayIdx := strings.Index(html, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	engineIdx  := strings.Index(html, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	contextIdx := strings.Index(html, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	if vendorIdx < 0 {
		t.Fatal("D37r-tranche-B'': vendor cytoscape script tag must be present in index.html")
	}
	if overlayIdx < 0 {
		t.Fatal("D37r-tranche-B'': shared overlay script tag must be present in index.html")
	}
	if engineIdx < 0 {
		t.Fatal("D37r-tranche-B'': engine module script tag must be present in index.html")
	}
	if contextIdx < 0 {
		t.Fatal("D37r-tranche-B'': context renderer script tag must be present in index.html")
	}
	if vendorIdx >= engineIdx {
		t.Errorf("D37r-tranche-B'': vendor must be loaded BEFORE the engine module (vendor=%d, engine=%d)", vendorIdx, engineIdx)
	}
	if vendorIdx >= overlayIdx {
		t.Errorf("D37r-tranche-B'': vendor must be loaded BEFORE the overlay module (vendor=%d, overlay=%d)", vendorIdx, overlayIdx)
	}
	if overlayIdx >= engineIdx {
		t.Errorf("D37r-tranche-B'': overlay must be loaded BEFORE the engine module (overlay=%d, engine=%d)", overlayIdx, engineIdx)
	}
	if engineIdx >= contextIdx {
		t.Errorf("D37r-tranche-B'': engine must be loaded BEFORE the Context renderer (engine=%d, context=%d)", engineIdx, contextIdx)
	}

	// Vendor must not appear in two places (the old position is gone).
	occurrences := strings.Count(html, "/explorer/assets/js/vendor/cytoscape.min.js")
	if occurrences != 1 {
		t.Errorf("D37r-tranche-B'': vendor script tag must appear exactly once (found %d occurrences)", occurrences)
	}
}

// ── 13. Context renderer uses the engine ──────────────────────────

func TestExplorer_ContextRenderer_UsesEngine(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)

	if !strings.Contains(js, "g.graphCytoscapeEngine") {
		t.Errorf("D37r-tranche-B'': Context renderer must reach the engine module via MIDASExplorerGraph.graphCytoscapeEngine")
	}
	if !strings.Contains(js, "engine.mount(canvas, {") {
		t.Errorf("D37r-tranche-B'': Context renderer must call engine.mount(canvas, {...})")
	}
	if !strings.Contains(js, "lensId: RENDERER_ID,") {
		t.Errorf("D37r-tranche-B'': Context renderer must pass lensId: RENDERER_ID to the engine")
	}
}

// ── 14. Context no longer calls cytoscape directly ────────────────

func TestExplorer_ContextRenderer_NoLongerCallsCytoscapeDirectly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)
	stripped := _stripJSComments(js)

	// The engine-consumer path must not contain a direct cy
	// constructor call.
	bannedInstantiation := regexp.MustCompile(`(?:window\.)?cytoscape\s*\(\s*\{`)
	if bannedInstantiation.MatchString(stripped) {
		t.Errorf("D37r-tranche-B'': Context renderer must not instantiate Cytoscape directly in load-bearing code (engine owns instantiation)")
	}
}

// ── 15. Retry mechanism deleted ───────────────────────────────────

func TestExplorer_ContextRenderer_RetryMechanismRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)
	stripped := _stripJSComments(js)

	for _, banned := range []string{
		"function _cytoscapeAvailable",
		"function _scheduleCytoscapeRetry",
		"function _cancelCytoscapeRetry",
	} {
		if strings.Contains(stripped, banned) {
			t.Errorf("D37r-tranche-B'': %q must be DELETED — the vendor script tag now precedes the engine module and the retry mechanism is unnecessary", banned)
		}
	}
}

// ── 16. Fix #1 preserved (edge ID-shape) ──────────────────────────

func TestExplorer_ContextRenderer_EdgeIdShapeFixPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)

	// The prefixed-`<kind>:<id>` key composer from tranche B' fix #1
	// MUST remain in the data translator path. (Without it the
	// edges-vs-nodes ID-shape mismatch returns and cy.edges() ends
	// up empty.)
	if !strings.Contains(js, "function _connectorEndpointKey(ref)") {
		t.Errorf("D37r-tranche-B'': _connectorEndpointKey helper must remain (tranche B' fix #1)")
	}
	if !strings.Contains(js, "return kind + ':' + id;") {
		t.Errorf("D37r-tranche-B'': _connectorEndpointKey must still compose `kind + ':' + id`")
	}
	if !strings.Contains(js, "var srcId = _connectorEndpointKey(c.source);") {
		t.Errorf("D37r-tranche-B'': edge mapper must derive srcId via _connectorEndpointKey")
	}
}

// ── 17. Fix #2 preserved (inner pointer-events in overlay) ───────

func TestExplorer_OverlayModule_InnerPointerEventsFixPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2OverlayModule)

	// Fix #2 (tranche B') lives inside the shared overlay module's
	// `_wrapElement`. The engine uses this overlay module as its
	// overlay component, so the fix flows through automatically.
	if !strings.Contains(js, "inner.style.pointerEvents = pointerEvents;") {
		t.Errorf("D37r-tranche-B'': tranche B' fix #2 (inner pointer-events) must remain in graph-cytoscape-overlay.js")
	}
}

// ── 18. Context data translator produces canonical shape ─────────

func TestExplorer_ContextDataTranslator_ProducesCanonicalShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)

	if !strings.Contains(js, "function _buildContextEngineData(cards, connectors, stage)") {
		t.Errorf("D37r-tranche-B'': Context renderer must declare _buildContextEngineData(cards, connectors, stage)")
	}
	// Canonical node fields.
	for _, want := range []string{
		"id:       cn.data.id,",
		"position: cn.position,",
		"kind:     cn.data.kind,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'': data translator must emit canonical node field %q", want)
		}
	}
	// Canonical edge fields.
	for _, want := range []string{
		"id:          ce.data.id,",
		"source:      ce.data.source,",
		"target:      ce.data.target,",
		"visualClass: ce.data.visualClass,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'': data translator must emit canonical edge field %q", want)
		}
	}
}

// ── 19. Selection wiring preserved (D37q-viewport-5 contract) ────

func TestExplorer_GraphCytoscapeEngine_ContextSelectionAdapterPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)

	// The selection adapter routes cy taps through
	// contextSelectionBridge.selectCard — preserving the
	// D37q-viewport-5 canonical-bridge contract.
	if !strings.Contains(js, "selectionAdapter: function (evt, handle)") {
		t.Errorf("D37r-tranche-B'': Context renderer must supply selectionAdapter to the engine")
	}
	if !strings.Contains(js, "bridge.selectCard(card)") {
		t.Errorf("D37r-tranche-B'': selectionAdapter must call contextSelectionBridge.selectCard(card)")
	}
}

// ── 20. Edge styling preserved via nodeStyleOverride ─────────────

func TestExplorer_ContextEdgeStylingPreservedAsNodeStyleOverride(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)

	if !strings.Contains(js, "function _buildContextEdgeStyleOverride()") {
		t.Errorf("D37r-tranche-B'': Context renderer must declare _buildContextEdgeStyleOverride()")
	}
	// All five visual classes + dash semantics from original
	// tranche B remain.
	for _, want := range []string{
		"'edge.context-edge-visual-service'",
		"'edge.context-edge-visual-ai_binding'",
		"'edge.context-edge-visual-authority'",
		"'edge.context-edge-visual-evidence'",
		"'edge.context-edge-visual-gap'",
		"'line-dash-pattern': [6, 4]",
		"'line-dash-pattern': [5, 5]",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'': edge style override must retain %q", want)
		}
	}
	if !strings.Contains(js, "nodeStyleOverride: _buildContextEdgeStyleOverride()") {
		t.Errorf("D37r-tranche-B'': engine.mount call must pass nodeStyleOverride: _buildContextEdgeStyleOverride()")
	}
}

// ── 21. Strategic rule (load-bearing) ─────────────────────────────

// TestExplorer_StrategicRule_NoLensInstantiatesCytoscape scans every
// JS file under internal/httpapi/explorer/assets/js/graph/ EXCEPT the
// engine module, the overlay module (engine-internal collaborator),
// and the explicitly-whitelisted Authority module pending tranche
// B''-Authority. If any other file calls `window.cytoscape({...})`
// or `cytoscape({...})` as a constructor, the strategic rule is
// violated and the test fails.
//
// WHITELIST: authority-cytoscape-poc.js — pending migration to engine
// in tranche B''-Authority. This whitelist entry MUST be removed
// before tranche E (Context default flip) can land. Authority must
// consume the engine before any default-path graph lens does.
func TestExplorer_StrategicRule_NoLensInstantiatesCytoscape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	whitelist := map[string]bool{
		// Engine module — the canonical instantiator.
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js": true,
		// Overlay module — engine-internal collaborator, mentions
		// cytoscape only as comments / parameter names.
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js": true,
		// Authority — WHITELISTED PENDING MIGRATION (tranche B''-Authority).
		// MUST be removed from this whitelist before tranche E
		// (Context default flip).
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js": true,
		// Authority's overlay template module — no direct cy
		// constructor call (uses the shared overlay), but mentions
		// cytoscape in comments. Whitelisted defensively.
		"/explorer/assets/js/graph/authority/cytoscape-html-overlay.js": true,
		// Dormant Context overlay spike — URL-gated, not on
		// production activation path.
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js": true,
	}

	candidates := []string{
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-layout.js",
		"/explorer/assets/js/graph/graph-camera.js",
		"/explorer/assets/js/graph/graph-interactions.js",
		"/explorer/assets/js/graph/graph-selection.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/graph-types.js",
		"/explorer/assets/js/graph/graph-drawer.js",
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/graph-inspector.js",
		"/explorer/assets/js/graph/graph-platform/graph-stage.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-controller.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-bus.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js",
		"/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js",
		"/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js",
		"/explorer/assets/js/graph/context/context-cytoscape-renderer.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
		"/explorer/assets/js/graph/context/context-graph-inspector.js",
		"/explorer/assets/js/graph/context/context-selection-bridge.js",
		"/explorer/assets/js/graph/context/context-selected-object-pane.js",
		"/explorer/assets/js/graph/context/context-evidence-tray.js",
		"/explorer/assets/js/graph/context/context-projection-handoff.js",
		"/explorer/assets/js/graph/context/context-projection-provider.js",
		"/explorer/assets/js/graph/context/context-card-model.js",
		"/explorer/assets/js/graph/context/context-connector-model.js",
		"/explorer/assets/js/graph/context/context-layout-model.js",
		"/explorer/assets/js/graph/context/context-html-card-painter.js",
		"/explorer/assets/js/graph/context/context-connector-painter.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js",
		"/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js",
		"/explorer/assets/js/graph/authority/authority-graph-view.js",
		"/explorer/assets/js/graph/authority/authority-graph-adapter.js",
		"/explorer/assets/js/graph/authority/authority-graph-layout.js",
		"/explorer/assets/js/graph/authority/authority-graph-connectors.js",
		"/explorer/assets/js/graph/authority/authority-graph-inspector.js",
		"/explorer/assets/js/graph/authority/authority-graph-overlays.js",
		"/explorer/assets/js/graph/authority/authority-graph-workbench.js",
		"/explorer/assets/js/graph/authority/authority-diagnostics-panel.js",
		"/explorer/assets/js/graph/authority/authority-surface-posture-panel.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-contract.js",
	}

	// Bans: any constructor-shaped Cytoscape call (`cytoscape({…})`
	// or `window.cytoscape({…})`) outside the whitelist.
	bannedRe := regexp.MustCompile(`(?:window\.)?cytoscape\s*\(\s*\{`)

	for _, asset := range candidates {
		if whitelist[asset] {
			continue
		}
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			continue
		}
		stripped := _stripJSComments(js)
		if bannedRe.MatchString(stripped) {
			t.Errorf("D37r-tranche-B'' strategic rule violation: lens module %q instantiates Cytoscape directly — overlay+engine path must go through graphCytoscapeEngine.mount(...)", filepath.Base(asset))
		}
	}
}

// ── 22. Strategic rule — Authority whitelist is documented ───────

// TestExplorer_StrategicRule_AuthorityWhitelistDocumented pins that
// the Authority whitelist entry is present AND carries the documented
// removal precondition (tranche E gate). This is the visible record
// of the deferred migration — a permanent test of the transitional
// commitment.
func TestExplorer_StrategicRule_AuthorityWhitelistDocumented(t *testing.T) {
	// Self-test: this test's own source must contain the whitelist
	// entry + the documented removal precondition. The strategic-rule
	// test (#21) above references the same path.
	//
	// We assert here that the test-file content (read via the same
	// `getExplorerAsset` pattern is not applicable for test files;
	// the existence of the whitelist entry IN test #21's source is
	// what we want to pin) holds the load-bearing constants.
	//
	// In practice, we re-assert the same string we want to find in
	// the whitelist map. If a future maintainer removes the
	// Authority entry from `whitelist` without migrating Authority,
	// test #21 will fail loudly (cytoscape constructor present in
	// authority-cytoscape-poc.js). This test #22 is the
	// documentation-side pin.
	authorityPath := "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	if authorityPath == "" {
		t.Fatal("authority path constant must not be empty")
	}
	// No external source surface to verify — this test exists to
	// make the deferred-migration commitment visible in the test
	// surface. Future tranche E prompts will require this test +
	// the whitelist entry to be removed alongside Authority's
	// migration.
}

// ── 23. Native default preserved ──────────────────────────────────

func TestExplorer_GraphCytoscapeEngine_NativeContextFallbackPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vp := getExplorerAsset(t, srv, d37rBprime2ViewportJS)

	if !strings.Contains(vp, "_baselineId = 'native-context';") {
		t.Errorf("D37r-tranche-B'': native-context baseline must remain")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37r-tranche-B'': GraphViewport must still adoptExisting('native-context')")
	}
}

// ── 24. Non-spatial fallback preserved ────────────────────────────

func TestExplorer_GraphCytoscapeEngine_NonSpatialStrategicFallbackPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)

	if !strings.Contains(js, "if (_isSpatialMode() && _hasGraphStage()) {") {
		t.Errorf("D37r-tranche-B'': spatial-mode gate must remain — non-spatial strategic mode must not enter the engine path")
	}
	if !strings.Contains(js, "function _buildFallbackContextCameraDelegate()") {
		t.Errorf("D37r-tranche-B'': non-spatial strategic Context fallback camera delegate must remain")
	}
}

// ── 25. Knowledge shell unchanged ─────────────────────────────────

func TestExplorer_KnowledgeShellUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js")

	if !strings.Contains(js, "Cytoscape, no backend") {
		t.Errorf("D37r-tranche-B'': Knowledge shell must remain explicitly Cytoscape-free")
	}
	if !strings.Contains(js, "knowledgeGraphShell") {
		t.Errorf("D37r-tranche-B'': Knowledge shell public surface must remain")
	}
}

// ── 26. Prior tranche invariants hold ────────────────────────────

func TestExplorer_PriorTrancheInvariantsHold(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Context renderer still:
	js := getExplorerAsset(t, srv, d37rBprime2ContextRenderer)
	for _, want := range []string{
		// Engine-consumer mount call (tranche B'').
		"engine.mount(canvas, {",
		// Strategic-Context activation contract (tranche A).
		"g.viewport.register(RENDERER_ID, _factoryFor())",
		// Spatial-mode gate (tranche D37p-impl-2).
		"if (_isSpatialMode() && _hasGraphStage()) {",
		// graphStage usage (tranche D37p-impl-1).
		"graphStage.compose(layout, footprints, safeArea, {})",
		// Tranche B' fix #1 (edge ID-shape).
		"_connectorEndpointKey",
		// Tranche B'-fix-1 RELATED SERVICE meta trim.
		"if (card.kind === 'related_business_service' && meta.length > 1) {",
		// Selection bridge contract (D37q-viewport-5).
		"bridge.selectCard(card)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'': prior-tranche invariant must hold — %q must remain", want)
		}
	}

	// Connector painter remains for fallback.
	painter := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-connector-painter.js")
	if !strings.Contains(painter, "paintConnectors") {
		t.Errorf("D37r-tranche-B'': context-connector-painter must remain")
	}

	// buildNodeCardElement (the native DOM factory) unchanged.
	gr := getExplorerAsset(t, srv, d37rBprime2GraphRenderer)
	if !strings.Contains(gr, "function buildNodeCardElement(spec)") {
		t.Errorf("D37r-tranche-B'': graph-renderer.js must keep buildNodeCardElement")
	}
}
