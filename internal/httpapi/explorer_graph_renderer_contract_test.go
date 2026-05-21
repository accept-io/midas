package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37q-viewport-7-impl — Graph Renderer Contract Test Suite.
//
// This file is the platform-level source-contract test suite for graph
// renderers. It aggregates the per-tranche invariants established
// across D37o / D37p / D37q into one renderer-compliance checklist
// that:
//
//   (a) pins the current state of every production renderer against
//       the shared platform seams (GraphViewport, camera bus,
//       selection bridge, selected-object pane shell, drawer);
//
//   (b) captures intentional omissions as an allow-list so future
//       tranches don't accidentally broaden them;
//
//   (c) documents the future renderer plug-in checklist so new graph
//       types onboarding to the strategic viewport have a single
//       reference point.
//
// The suite is source-contract only — it inspects the served JS / CSS
// / HTML and asserts architectural invariants. No browser execution
// is required.
//
// Production renderer ids covered:
//
//   - native-context  : legacy DOM/SVG Context (baseline; adopted by
//                       GraphViewport at module init)
//   - context         : strategic Context renderer (opt-in via
//                       ?contextRenderer=strategic)
//   - authority       : Authority Cytoscape renderer
//   - knowledge-graph : Knowledge shell (proof of GraphViewport reuse;
//                       intentionally minimal — no camera, selection,
//                       pane, or drawer wiring yet)
//
// Dormant / spike registrations:
//
//   - context-cytoscape : context-cytoscape-overlay-spike.js, gated
//                         off behind ?cytoscape=1&contextHtmlCards=1.
//                         Not a production renderer.

const (
	d37qV7ViewportAsset            = "/explorer/assets/js/graph/graph-viewport.js"
	d37qV7CameraBusAsset           = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37qV7SelectionBridgeAsset     = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37qV7PaneShellAsset           = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37qV7StageAsset               = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37qV7DrawerAsset              = "/explorer/assets/js/graph/graph-drawer.js"
	d37qV7CameraToolbarAdapter     = "/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js"
	d37qV7ContextRendererAsset     = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37qV7ContextSelectionBridge   = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37qV7ContextPaneAsset         = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37qV7ContextConnectorPainter  = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37qV7AuthorityPocAsset        = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37qV7AuthorityEdgeAsset       = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37qV7AuthorityViewAsset       = "/explorer/assets/js/graph/authority/authority-graph-view.js"
	d37qV7AuthorityWorkbenchAsset  = "/explorer/assets/js/graph/authority/authority-graph-workbench.js"
	d37qV7KnowledgeRendererAsset   = "/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js"
	d37qV7AuthorityEdgeCss         = "/explorer/assets/css/authority-canvas-edge-tabs.css"
	d37qV7ContextPaneCss           = "/explorer/assets/css/context-selected-object-pane.css"
	d37qV7ContextRendererCss       = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37qV7KnowledgeCss             = "/explorer/assets/css/knowledge-graph.css"
	d37qV7AuthorityPocCss          = "/explorer/assets/css/authority-cytoscape-poc.css"
)

// productionRendererIds is the explicit, source-pinned list of
// renderer ids that participate in the strategic viewport platform.
// New entries belong here only when the renderer is a real production
// surface (not a dormant spike).
var productionRendererIds = []string{
	"native-context",
	"context",
	"authority",
	"knowledge-graph",
}

// ── 1. GraphViewport host contract present ─────────────────────────

func TestExplorer_D37qViewport7_GraphViewportHostContractPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV7ViewportAsset)

	for _, want := range []string{
		// Public registry / lifecycle surface.
		"function register(rendererId, factory)",
		"function unregister(rendererId)",
		"function hasRenderer(rendererId)",
		"function listRegistered()",
		"function activateById(rendererId)",
		"function deactivate()",
		"function getActiveRendererId()",
		"function adoptExisting(rendererId, handle)",
		// Active-renderer identity attribute name.
		"ACTIVE_RENDERER_ATTR = 'data-active-renderer'",
		// Renderer factory contract: mount(slotEl, ctx).
		"factory.mount(slotEl, ctx)",
		// Host context includes safe-area + viewport metrics.
		"getViewportRect:",
		"getSafeArea:",
		"onResize:",
		// Renderer-slot lookup.
		"RENDERER_SLOT_CLASS = 'midas-graph-renderer-slot'",
		"VIEWPORT_CLASS      = 'midas-graph-viewport'",
		// Native-context baseline adoption.
		"_baselineId = 'native-context';",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-7-impl: GraphViewport host contract missing %q", want)
		}
	}
}

// ── 2. Production renderers registered with GraphViewport ─────────

func TestExplorer_D37qViewport7_ProductionRenderersRegisteredWithViewport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// native-context — adopted by the host at module init.
	vp := getExplorerAsset(t, srv, d37qV7ViewportAsset)
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37q-viewport-7-impl: native-context must be adopted as the GraphViewport baseline")
	}

	// context — strategic renderer.
	ctxJs := getExplorerAsset(t, srv, d37qV7ContextRendererAsset)
	if !regexp.MustCompile(`g\.viewport\.register\(RENDERER_ID,\s*_factoryFor\(\)\)`).MatchString(ctxJs) {
		t.Errorf("D37q-viewport-7-impl: strategic Context must register with GraphViewport via viewport.register(RENDERER_ID, _factoryFor())")
	}
	if !strings.Contains(ctxJs, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37q-viewport-7-impl: strategic Context renderer id must be 'context'")
	}

	// authority — Cytoscape renderer.
	authJs := getExplorerAsset(t, srv, d37qV7AuthorityPocAsset)
	if !strings.Contains(authJs, "vp.register('authority', _authorityRendererFactory)") {
		t.Errorf("D37q-viewport-7-impl: Authority must register 'authority' with GraphViewport")
	}

	// knowledge-graph — shell.
	knJs := getExplorerAsset(t, srv, d37qV7KnowledgeRendererAsset)
	if !strings.Contains(knJs, "vp.register('knowledge-graph', _knowledgeGraphRendererFactory)") {
		t.Errorf("D37q-viewport-7-impl: Knowledge shell must register 'knowledge-graph' with GraphViewport")
	}
}

// ── 3. Renderer ids are stable strings ────────────────────────────

func TestExplorer_D37qViewport7_RendererIdsAreStable(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vp := getExplorerAsset(t, srv, d37qV7ViewportAsset)

	// The host knows the baseline literal.
	if !strings.Contains(vp, "'native-context'") {
		t.Errorf("D37q-viewport-7-impl: 'native-context' renderer id must appear in graph-viewport.js")
	}

	// Each production renderer module owns its literal.
	ctxJs := getExplorerAsset(t, srv, d37qV7ContextRendererAsset)
	if !strings.Contains(ctxJs, "'context'") {
		t.Errorf("D37q-viewport-7-impl: 'context' renderer id literal must appear in strategic Context module")
	}
	authJs := getExplorerAsset(t, srv, d37qV7AuthorityPocAsset)
	if !strings.Contains(authJs, "'authority'") {
		t.Errorf("D37q-viewport-7-impl: 'authority' renderer id literal must appear in Authority Cytoscape module")
	}
	knJs := getExplorerAsset(t, srv, d37qV7KnowledgeRendererAsset)
	if !strings.Contains(knJs, "'knowledge-graph'") {
		t.Errorf("D37q-viewport-7-impl: 'knowledge-graph' renderer id literal must appear in Knowledge shell module")
	}

	// No temporary rollout-mode words leak into renderer ids.
	for _, banned := range []string{
		"'context-v2'",
		"'context-strategic'",
		"'context-next'",
		"'authority-v2'",
		"'authority-new'",
		"'knowledge-v2'",
	} {
		for _, asset := range []string{d37qV7ViewportAsset, d37qV7ContextRendererAsset, d37qV7AuthorityPocAsset, d37qV7KnowledgeRendererAsset} {
			body := getExplorerAsset(t, srv, asset)
			if strings.Contains(body, banned) {
				t.Errorf("D37q-viewport-7-impl: temporary renderer id %q must not appear in %q", banned, asset)
			}
		}
	}
}

// ── 4. Active-renderer identity is the shared contract ────────────

func TestExplorer_D37qViewport7_ActiveRendererIdentityIsSharedContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// GraphViewport host writes data-active-renderer.
	vp := getExplorerAsset(t, srv, d37qV7ViewportAsset)
	if !strings.Contains(vp, "setAttribute(ACTIVE_RENDERER_ATTR, rendererId)") {
		t.Errorf("D37q-viewport-7-impl: GraphViewport host must write data-active-renderer on activation")
	}

	// Renderer-specific CSS scopes through the attribute.
	for _, scope := range []struct {
		asset string
		scope string
	}{
		{d37qV7AuthorityEdgeCss, `.midas-graph-viewport[data-active-renderer="authority"]`},
		{d37qV7AuthorityPocCss, `[data-active-renderer="authority"]`},
		{d37qV7ContextPaneCss, `.midas-graph-viewport[data-active-renderer="context"]`},
		{d37qV7ContextRendererCss, `[data-active-renderer="context"]`},
		{d37qV7KnowledgeCss, `.midas-graph-viewport[data-active-renderer="knowledge-graph"]`},
	} {
		css := getExplorerAsset(t, srv, scope.asset)
		if !strings.Contains(css, scope.scope) {
			t.Errorf("D37q-viewport-7-impl: CSS %q must scope renderer-specific rules under %q", scope.asset, scope.scope)
		}
	}
}

// ── 5. Camera delegates explicit per renderer ─────────────────────

func TestExplorer_D37qViewport7_CameraDelegatesAreExplicit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// native-context — registered by the toolbar adapter.
	adapter := getExplorerAsset(t, srv, d37qV7CameraToolbarAdapter)
	if !strings.Contains(adapter, "bus.registerLens('native-context', _legacyDelegate())") {
		t.Errorf("D37q-viewport-7-impl: native-context camera-bus delegate must be registered by the toolbar adapter")
	}

	// context — registered by the strategic Context renderer at mount
	// time (D37q-viewport-3-impl removed the spatial-mode gate).
	ctxJs := getExplorerAsset(t, srv, d37qV7ContextRendererAsset)
	if !strings.Contains(ctxJs, "function _buildContextCameraDelegate(camera)") {
		t.Errorf("D37q-viewport-7-impl: strategic Context must build a camera delegate for the spatial-mode path")
	}
	if !strings.Contains(ctxJs, "function _buildFallbackContextCameraDelegate()") {
		t.Errorf("D37q-viewport-7-impl: strategic Context must build a fallback camera delegate for non-spatial mode")
	}
	if !strings.Contains(ctxJs, "bus.registerLens(RENDERER_ID, delegate)") {
		t.Errorf("D37q-viewport-7-impl: strategic Context must call bus.registerLens(RENDERER_ID, delegate)")
	}

	// authority — registered by Authority module.
	authJs := getExplorerAsset(t, srv, d37qV7AuthorityPocAsset)
	if !strings.Contains(authJs, "bus.registerLens('authority',") {
		t.Errorf("D37q-viewport-7-impl: Authority must register an 'authority' camera-bus delegate")
	}

	// knowledge-graph — INTENTIONALLY no camera delegate (shell renderer).
	knJs := getExplorerAsset(t, srv, d37qV7KnowledgeRendererAsset)
	if regexp.MustCompile(`registerLens\s*\(\s*['"]knowledge`).MatchString(knJs) {
		t.Errorf("D37q-viewport-7-impl: Knowledge shell intentionally registers no camera-bus delegate; remove the registration or upgrade the shell first")
	}
}

// ── 6. Selection integration explicit per renderer ────────────────

func TestExplorer_D37qViewport7_SelectionIntegrationIsExplicit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Context publishes + registers a delegate.
	ctxBridge := getExplorerAsset(t, srv, d37qV7ContextSelectionBridge)
	for _, want := range []string{
		"graphSelectionBridge",
		"bridge.selectCard(",
		"bridge.clearSelection()",
		"bridge.registerLens('context'",
	} {
		if !strings.Contains(ctxBridge, want) {
			t.Errorf("D37q-viewport-7-impl: Context selection bridge must publish + register via %q", want)
		}
	}

	// Authority publishes + registers a delegate.
	authJs := getExplorerAsset(t, srv, d37qV7AuthorityPocAsset)
	for _, want := range []string{
		"_publishAuthoritySelectionToSharedBridge",
		"bridge.selectCard(",
		"bridge.clearSelection(",
		"bridge.registerLens('authority'",
		// Authority's engine-local subscriber wires through the
		// existing onSelectionChanged registry (no second cy
		// listener).
		"_onSelectionChanged(_publishAuthoritySelectionToSharedBridge)",
	} {
		if !strings.Contains(authJs, want) {
			t.Errorf("D37q-viewport-7-impl: Authority selection bridge integration must contain %q", want)
		}
	}

	// Knowledge shell — INTENTIONALLY no bridge integration.
	knJs := getExplorerAsset(t, srv, d37qV7KnowledgeRendererAsset)
	if strings.Contains(knJs, "graphSelectionBridge") {
		t.Errorf("D37q-viewport-7-impl: Knowledge shell intentionally does not integrate with graphSelectionBridge yet; remove the reference or upgrade the shell first")
	}
}

// ── 7. Selected-object pane providers explicit per renderer ───────

func TestExplorer_D37qViewport7_SelectedObjectProvidersAreExplicit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Context registers a provider.
	ctxPane := getExplorerAsset(t, srv, d37qV7ContextPaneAsset)
	if !strings.Contains(ctxPane, "shell.registerLensProvider('context'") {
		t.Errorf("D37q-viewport-7-impl: Context must register a 'context' provider with graphSelectedObjectPane")
	}

	// Authority registers a provider through canvas-edge tabs.
	edge := getExplorerAsset(t, srv, d37qV7AuthorityEdgeAsset)
	if !strings.Contains(edge, "shell.registerLensProvider('authority', _paneProvider)") {
		t.Errorf("D37q-viewport-7-impl: Authority canvas-edge tabs must register an 'authority' provider with graphSelectedObjectPane")
	}

	// Authority provider's notifySelectionChanged is a deliberate
	// no-op (double-render avoidance) — pinned by D37q-viewport-5;
	// reasserted here as part of the renderer-contract aggregate.
	if !regexp.MustCompile(`notifySelectionChanged:\s*function\s*\(\s*selection,\s*event\s*\)\s*\{\s*void selection; void event;\s*\}`).MatchString(edge) {
		t.Errorf("D37q-viewport-7-impl: Authority pane provider notifySelectionChanged must remain a deliberate no-op (double-render avoidance)")
	}

	// Knowledge shell — INTENTIONALLY no provider.
	knJs := getExplorerAsset(t, srv, d37qV7KnowledgeRendererAsset)
	if strings.Contains(knJs, "graphSelectedObjectPane") {
		t.Errorf("D37q-viewport-7-impl: Knowledge shell intentionally does not register a selected-object pane provider yet; remove the reference or upgrade the shell first")
	}
}

// ── 8. Drawer registrations explicit per renderer ─────────────────

func TestExplorer_D37qViewport7_DrawerRegistrationsAreExplicit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Context drawer registration lives inline in index.html.
	if !strings.Contains(body, "MIDASExplorerGraph.drawer.registerLens('context'") {
		t.Errorf("D37q-viewport-7-impl: Context must register drawer slots via MIDASExplorerGraph.drawer.registerLens('context', ...)")
	}

	// Authority drawer registration lives in the Authority view.
	authView := getExplorerAsset(t, srv, d37qV7AuthorityViewAsset)
	if !strings.Contains(authView, "MIDASExplorerGraph.drawer.registerLens('authority', {") {
		t.Errorf("D37q-viewport-7-impl: Authority must register drawer slots via MIDASExplorerGraph.drawer.registerLens('authority', ...)")
	}

	// Drawer module public surface intact.
	drawer := getExplorerAsset(t, srv, d37qV7DrawerAsset)
	for _, want := range []string{
		"window.MIDASExplorerGraph.drawer",
		"registerLens:",
		"setActiveLens:",
		"setActiveTab:",
	} {
		if !strings.Contains(drawer, want) {
			t.Errorf("D37q-viewport-7-impl: graph-drawer.js must expose %q", want)
		}
	}

	// Knowledge shell — INTENTIONALLY no drawer registration.
	knJs := getExplorerAsset(t, srv, d37qV7KnowledgeRendererAsset)
	if strings.Contains(knJs, "drawer.registerLens") {
		t.Errorf("D37q-viewport-7-impl: Knowledge shell intentionally does not register drawer slots yet; remove the reference or upgrade the shell first")
	}
}

// ── 9. Workbench surfaces remain lens-specific ────────────────────

// TestExplorer_D37qViewport7_WorkbenchSurfacesRemainLensSpecific pins
// the current intentional state: no shared workbench shell exists.
// Authority workbench and Context evidence tray are sibling lens-
// specific modules. This test prevents an accidental shared-shell
// abstraction from being introduced without a deliberate tranche.
func TestExplorer_D37qViewport7_WorkbenchSurfacesRemainLensSpecific(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Authority workbench is lens-specific.
	wb := getExplorerAsset(t, srv, d37qV7AuthorityWorkbenchAsset)
	if !strings.Contains(wb, "window.MIDASExplorerGraph.authorityWorkbench") {
		t.Errorf("D37q-viewport-7-impl: Authority workbench module must expose MIDASExplorerGraph.authorityWorkbench")
	}
	// And it must NOT register itself with a (non-existent) shared
	// workbench shell.
	for _, banned := range []string{
		"graphWorkbench",
		"registerLensWorkbench",
		"registerWorkbenchProvider",
	} {
		if strings.Contains(wb, banned) {
			t.Errorf("D37q-viewport-7-impl: Authority workbench must remain lens-specific; %q implies a shared shell not yet designed", banned)
		}
	}

	// No platform module declares a shared workbench shell.
	for _, asset := range []string{
		d37qV7ViewportAsset,
		d37qV7CameraBusAsset,
		d37qV7SelectionBridgeAsset,
		d37qV7PaneShellAsset,
		d37qV7StageAsset,
		d37qV7DrawerAsset,
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"graphWorkbench =",
			"graphWorkbench:",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37q-viewport-7-impl: no shared workbench shell may be introduced without an explicit design tranche; %q found in %q", banned, asset)
			}
		}
	}
}

// ── 10. Safe-area contract exposed + consumed ─────────────────────

func TestExplorer_D37qViewport7_SafeAreaContractIsExposedAndConsumed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Host exposes per-edge safe-area insets.
	vp := getExplorerAsset(t, srv, d37qV7ViewportAsset)
	for _, want := range []string{
		"function getSafeArea()",
		"top:",
		"right:",
		"bottom:",
		"left:",
		// Chrome-aware: the host considers visible chrome elements
		// when computing insets.
		"CHROME_CLASSES",
	} {
		if !strings.Contains(vp, want) {
			t.Errorf("D37q-viewport-7-impl: GraphViewport host must expose a per-edge safe-area contract (%q)", want)
		}
	}

	// Strategic Context consumes host safe-area through ctx.getSafeArea.
	ctxJs := getExplorerAsset(t, srv, d37qV7ContextRendererAsset)
	if !strings.Contains(ctxJs, "getSafeArea:") {
		t.Errorf("D37q-viewport-7-impl: strategic Context camera target must expose getSafeArea")
	}

	// Authority consumes host safe-area through ctx.getSafeArea.
	// D37q-viewport-4 will later improve per-edge handling; this
	// tranche only pins current consumption.
	authJs := getExplorerAsset(t, srv, d37qV7AuthorityPocAsset)
	if !strings.Contains(authJs, "_rendererCtx.getSafeArea()") {
		t.Errorf("D37q-viewport-7-impl: Authority must consume host getSafeArea via _rendererCtx.getSafeArea()")
	}
}

// ── 11. Stage + connector current state pinned ────────────────────

func TestExplorer_D37qViewport7_StageAndConnectorContractCurrentStatePinned(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// graphStage exposes its current public contract.
	stage := getExplorerAsset(t, srv, d37qV7StageAsset)
	for _, want := range []string{
		"window.MIDASExplorerGraph.graphStage",
		"compose",
		"anchorOf",
		"fitBoundsOf",
		"normaliseCardFootprints",
	} {
		if !strings.Contains(stage, want) {
			t.Errorf("D37q-viewport-7-impl: graphStage must expose %q", want)
		}
	}

	// Strategic Context spatial mode consumes graphStage.compose.
	ctxJs := getExplorerAsset(t, srv, d37qV7ContextRendererAsset)
	if !strings.Contains(ctxJs, "graphStage.compose(layout, footprints, safeArea, {})") {
		t.Errorf("D37q-viewport-7-impl: strategic Context spatial mode must consume graphStage.compose(...)")
	}

	// Context connector painter still consumes pre-built connector
	// specs + a caller-supplied getCardElement callback. D37q-viewport-2
	// added the shared-stage anchor preference: when an `opts.stage`
	// StageModel is supplied, the painter resolves endpoints via the
	// platform stage's `anchorOf(stage, cardKey, 'centre')` contract
	// first, with DOM-measured centroids retained as a per-endpoint
	// fallback. Visual output is unchanged in spatial mode (the
	// stage's `centre` anchor equals the card's DOM centroid).
	painter := getExplorerAsset(t, srv, d37qV7ContextConnectorPainter)
	if !strings.Contains(painter, "window.MIDASExplorerGraph.contextConnectorPainter") {
		t.Errorf("D37q-viewport-7-impl: Context connector painter public surface must remain present")
	}
	if !strings.Contains(painter, "stageMod.anchorOf") {
		t.Errorf("D37q-viewport-7-impl: Context connector painter must now reference the shared stage anchor contract (stageMod.anchorOf) after D37q-viewport-2-impl")
	}

	// Authority does NOT consume graphStage — Cytoscape owns
	// coordinates. Intentional and architecturally documented.
	authJs := getExplorerAsset(t, srv, d37qV7AuthorityPocAsset)
	if strings.Contains(authJs, "graphStage.compose") {
		t.Errorf("D37q-viewport-7-impl: Authority intentionally does not consume graphStage.compose (Cytoscape owns coordinates); remove the reference or document a design change")
	}
}

// ── 12. Renderer omissions are intentional ────────────────────────

// TestExplorer_D37qViewport7_RendererOmissionsAreIntentional captures
// the explicit allow-list of omissions per renderer. These are NOT
// architectural debt; they are deliberate design decisions documented
// across D37o / D37p / D37q assessments. This test fails loudly if a
// future tranche accidentally broadens an omission (e.g. removes
// Authority's selection bridge integration) AND fails loudly if a
// future tranche silently adds a registration that should have been
// proposed in its own design tranche (e.g. wires Knowledge shell into
// the camera bus without a Knowledge feature implementation).
func TestExplorer_D37qViewport7_RendererOmissionsAreIntentional(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	type omission struct {
		renderer string
		asset    string
		// substring whose ABSENCE is the intentional omission.
		mustNotContain string
		rationale      string
	}

	for _, o := range []omission{
		{
			renderer:       "knowledge-graph",
			asset:          d37qV7KnowledgeRendererAsset,
			mustNotContain: "graphCameraBus",
			rationale:      "Knowledge shell is a proof-of-reuse renderer with no camera controls yet",
		},
		{
			renderer:       "knowledge-graph",
			asset:          d37qV7KnowledgeRendererAsset,
			mustNotContain: "graphSelectionBridge",
			rationale:      "Knowledge shell does not yet support selection",
		},
		{
			renderer:       "knowledge-graph",
			asset:          d37qV7KnowledgeRendererAsset,
			mustNotContain: "graphSelectedObjectPane",
			rationale:      "Knowledge shell does not yet have a selected-object surface",
		},
		{
			renderer:       "knowledge-graph",
			asset:          d37qV7KnowledgeRendererAsset,
			mustNotContain: "drawer.registerLens",
			rationale:      "Knowledge shell has no drawer content yet",
		},
		{
			renderer:       "authority",
			asset:          d37qV7AuthorityPocAsset,
			mustNotContain: "graphStage.compose",
			rationale:      "Authority lets Cytoscape own coordinates — graphStage is for DOM/SVG renderers",
		},
	} {
		js := getExplorerAsset(t, srv, o.asset)
		if strings.Contains(js, o.mustNotContain) {
			t.Errorf("D37q-viewport-7-impl: renderer %q intentionally omits %q (%s); broadening this requires a design tranche, not a silent change to %q",
				o.renderer, o.mustNotContain, o.rationale, o.asset)
		}
	}
}

// ── 13. Future renderer onboarding checklist (test-pinned doc) ────

// TestExplorer_D37qViewport7_FutureRendererChecklistDocumentedInTests
// asserts that this test file itself documents the future renderer
// plug-in checklist as a single, discoverable source. The checklist
// is the authoritative onboarding doc for a new graph type; it lives
// here so it cannot drift away from the test suite that enforces it.
//
// The test inspects this file's own served-via-Go content indirectly
// by reading the canonical checklist constant below.
func TestExplorer_D37qViewport7_FutureRendererChecklistDocumentedInTests(t *testing.T) {
	for _, want := range []string{
		"stable renderer id",
		"GraphViewport",
		"mount(slotEl, ctx)",
		"data-active-renderer",
		"ctx.getSafeArea",
		"graphCameraBus",
		"graphSelectionBridge",
		"graphSelectedObjectPane",
		"drawer slots",
		"graphStage",
		"engine-local subscriptions",
		"browser validation",
	} {
		if !strings.Contains(futureRendererPlugInChecklist, want) {
			t.Errorf("D37q-viewport-7-impl: future renderer plug-in checklist must mention %q", want)
		}
	}
}

// futureRendererPlugInChecklist is the source-pinned onboarding
// checklist for a new graph type plugging into the strategic
// multi-graph viewport platform. Each numbered item maps to one of
// the platform seams pinned by the tests in this suite.
//
// New graph types should treat this list as authoritative. A renderer
// that satisfies steps 1–4 is platform-compliant at the minimum
// "shell/proof" level (Knowledge shell sits here today). Steps 5–11
// upgrade the renderer to a full interactive lens. Step 12 covers
// the test discipline required to keep the renderer aligned over
// time.
const futureRendererPlugInChecklist = `
Future renderer plug-in checklist (D37q-viewport-7-impl)
========================================================

1. Define a stable renderer id (snake-case, no rollout-mode words,
   no version suffixes). The id is the canonical identity used by
   CSS scoping and lens-aware code.

2. Register a factory with GraphViewport at module init:
       window.MIDASExplorerGraph.viewport.register(rendererId, factory)
   The factory must expose mount(slotEl, ctx) returning
   { destroy(): void }. ctx provides viewportEl, slotEl,
   getViewportRect, getSafeArea, onResize, and hooks.

3. Honour the renderer lifecycle. mount() owns its DOM; destroy()
   tears it down cleanly. Never leak listeners, observers, or
   bridge subscriptions across destroy.

4. Scope every renderer-specific CSS rule under
       .midas-graph-viewport[data-active-renderer="<rendererId>"]
   so rules are inert on other lenses. data-active-renderer is
   owned by the GraphViewport host; never set it from the renderer.

5. Consume ctx.getSafeArea() when computing fit / pan / zoom so the
   renderer's content lands inside the visible chrome insets.

6. Register a camera-bus delegate via
       window.MIDASExplorerGraph.graphCameraBus.registerLens(
         rendererId, delegate)
   if the renderer has any camera controls. The delegate must
   implement the locked command vocabulary (zoomIn, zoomOut, fit,
   reset, focusRoot, focusSelected, setZoom, getZoom) — unsupported
   commands may be safe no-ops; missing methods are silent at the
   bus's dispatch layer.

7. Publish normalised selection through graphSelectionBridge:
       bridge.selectCard({ lens, id, kind, sourceNodeRef, card, meta })
       bridge.clearSelection()
   Register a delegate via bridge.registerLens(rendererId, ...) for
   action routing. Cross-lens consumers read selection through the
   bridge; renderer-internal engine-coupled subscribers may keep
   their direct subscriptions where documented (see
   D37q-viewport-5).

8. Register a graphSelectedObjectPane provider if the renderer has
   selected-object content:
       shell.registerLensProvider(rendererId, provider)
   The provider may own its own DOM or piggy-back on the shell's
   generic [data-selected-object-pane] scope. notifySelectionChanged
   may be a deliberate no-op if engine-coupled subscribers already
   drive the pane to avoid double-rendering.

9. Register drawer slots if the renderer has drawer content:
       MIDASExplorerGraph.drawer.registerLens(rendererId, { tabs })
   The drawer's three slot ids (inspector / evidence / config) are
   stable. Per-tab labels + render callbacks are lens-owned.

10. Use graphStage / connector router for DOM/SVG graph geometry.
    Engine-backed renderers (Cytoscape and similar) may bypass
    graphStage because their engines own coordinates. Future
    connector routing should consume graphStage.anchorOf rather
    than DOM-measured centroids.

11. Keep engine-local subscriptions inside renderer-owned modules
    only. Platform modules and other lenses must consume the
    bridge / bus / pane shell instead of subscribing directly to
    a graph engine.

12. Add source-contract coverage to internal/httpapi/ and run the
    full renderer-contract suite (this file). Include a manual
    browser validation checklist for any UX-visible work.
`
