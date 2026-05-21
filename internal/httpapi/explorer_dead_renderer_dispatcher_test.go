package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-clean-1 — Dead Renderer Dispatcher Retirement tests.
//
// The legacy `MIDASExplorerGraph.renderer.register / render / clear`
// dispatcher was an alternate per-lens dispatch table that predated
// the GraphViewport host. Diagnostic findings recorded across D37o /
// D37p assessments confirmed that the dispatch surface had zero live
// callers at runtime — every consumer had migrated to:
//
//   • `MIDASExplorerGraph.viewport.register / activateById /
//     deactivate / adoptExisting` for renderer lifecycle;
//   • `MIDASExplorerGraph.graphCameraBus` for the camera command
//     vocabulary;
//   • `MIDASExplorerGraph.graphSelectionBridge` for selection events;
//   • `MIDASExplorerGraph.graphSelectedObjectPane` for the per-lens
//     selected-object pane provider registry.
//
// D37p-clean-1 retires the three dead dispatch functions
// (`register / render / clear`) on `MIDASExplorerGraph.renderer`, the
// dead `_impls` registry, the Context legacy view's `lensImpl` and
// `_publishToProjectionHandoff` helper, the Authority legacy view's
// `lensImpl`, and the Cytoscape PoC's `lensImpl` + deferred-register
// IIFE. The lens-agnostic helpers exposed on
// `MIDASExplorerGraph.renderer.*` (path math, node / connector
// builders, visibility filters, `clearCanvas`) stay because legacy
// Context still consumes them.
//
// These tests pin the retirement plus the preservation invariants
// for every live platform surface.

const (
	d37pClean1RendererAsset    = "/explorer/assets/js/graph/graph-renderer.js"
	d37pClean1ContextViewAsset = "/explorer/assets/js/graph/context/context-graph-view.js"
	d37pClean1AuthorityView    = "/explorer/assets/js/graph/authority/authority-graph-view.js"
	d37pClean1AuthorityPoc     = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37pClean1ViewportAsset    = "/explorer/assets/js/graph/graph-viewport.js"
	d37pClean1CameraBusAsset   = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37pClean1SelBridgeAsset   = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37pClean1PaneShellAsset   = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37pClean1ContextRenderer  = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pClean1ContextSelBridge = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37pClean1ContextPane      = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37pClean1ContextProvider  = "/explorer/assets/js/graph/context/context-projection-provider.js"
	d37pClean1AuthorityEdge    = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37pClean1AuthorityWb      = "/explorer/assets/js/graph/authority/authority-graph-workbench.js"
	d37pClean1DrawerAsset      = "/explorer/assets/js/graph/graph-drawer.js"
)

// ── A. Dispatcher functions removed from graph-renderer.js ─────────

func TestExplorer_D37pClean1_DispatcherFunctionsRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1RendererAsset)

	// The three dispatch functions and the internal _impls registry
	// are retired.
	if regexp.MustCompile(`function\s+register\s*\(\s*lens\s*,\s*impl\s*\)`).MatchString(js) {
		t.Errorf("D37p-clean-1: graph-renderer.js must not define a register(lens, impl) function anymore")
	}
	if regexp.MustCompile(`function\s+render\s*\(\s*lens\s*,\s*payload\s*,\s*mount\s*\)`).MatchString(js) {
		t.Errorf("D37p-clean-1: graph-renderer.js must not define a render(lens, payload, mount) dispatch function anymore")
	}
	if regexp.MustCompile(`function\s+clear\s*\(\s*lens\s*,\s*mount\s*\)`).MatchString(js) {
		t.Errorf("D37p-clean-1: graph-renderer.js must not define a clear(lens, mount) dispatch function anymore")
	}
	if regexp.MustCompile(`var\s+_impls\s*=\s*\{\}`).MatchString(js) {
		t.Errorf("D37p-clean-1: graph-renderer.js must not keep an internal _impls dispatch registry anymore")
	}
}

// TestExplorer_D37pClean1_RendererPublicSurfaceSlimmed pins that the
// dead dispatch methods are no longer exported on
// `MIDASExplorerGraph.renderer` while the live helpers remain.
func TestExplorer_D37pClean1_RendererPublicSurfaceSlimmed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1RendererAsset)

	// Slice the public-surface object literal so we assert against
	// the export shape, not against comments / banned-substring
	// scoping issues elsewhere in the file.
	startIdx := strings.Index(js, "window.MIDASExplorerGraph.renderer = {")
	if startIdx < 0 {
		t.Fatalf("D37p-clean-1: graph-renderer.js must still expose window.MIDASExplorerGraph.renderer")
	}
	tail := js[startIdx:]
	endRel := strings.Index(tail, "};")
	if endRel < 0 {
		t.Fatalf("D37p-clean-1: graph-renderer.js public surface literal must be terminated")
	}
	surface := tail[:endRel]

	for _, banned := range []string{
		"register:",
		"render:",
		"clear:",
	} {
		if strings.Contains(surface, banned) {
			t.Errorf("D37p-clean-1: dead dispatch method %q must be removed from MIDASExplorerGraph.renderer public surface", banned)
		}
	}

	// Live helpers must remain.
	for _, want := range []string{
		"lensAgnosticConnectorPath:",
		"lensAgnosticNodePosition:",
		"clearCanvas:",
		"addNode:",
		"addConnector:",
		"addConnectorHitTarget:",
		"addLiveConnector:",
		"addMoreNode:",
		"effectiveGmapPosition:",
		"applyVisibilityFilters:",
	} {
		if !strings.Contains(surface, want) {
			t.Errorf("D37p-clean-1: live helper %q must remain on MIDASExplorerGraph.renderer", want)
		}
	}
}

// ── B. Context view: dead lensImpl + helper retired ────────────────

func TestExplorer_D37pClean1_ContextLensImplRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1ContextViewAsset)

	// Dead dispatcher registration call must be gone.
	if strings.Contains(js, "window.MIDASExplorerGraph.renderer.register('context', lensImpl)") {
		t.Errorf("D37p-clean-1: dead renderer.register('context', lensImpl) call must be removed from context-graph-view.js")
	}
	// The dead `lensImpl` object literal that wrapped render / clear
	// against the dispatcher must be gone. We assert against the
	// distinctive declaration pattern instead of bare substring so
	// the explanatory comments referencing `lensImpl` still pass.
	if regexp.MustCompile(`var\s+lensImpl\s*=\s*\{\s*render:\s*function\s*\(payload,\s*mount\)`).MatchString(js) {
		t.Errorf("D37p-clean-1: dead lensImpl object literal must be removed from context-graph-view.js")
	}
	// Dead `_publishToProjectionHandoff` helper (only reachable from
	// the now-removed lensImpl.render) must be gone.
	if regexp.MustCompile(`function\s+_publishToProjectionHandoff\s*\(payload,\s*ctx\)`).MatchString(js) {
		t.Errorf("D37p-clean-1: dead _publishToProjectionHandoff helper must be removed from context-graph-view.js")
	}
}

// TestExplorer_D37pClean1_ContextViewLivePathPreserved pins the live
// Context lens entry points the production refresh consumes.
func TestExplorer_D37pClean1_ContextViewLivePathPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1ContextViewAsset)

	for _, want := range []string{
		"function renderContextGraph(data, ctx)",
		"function renderContextGraphEmpty(message, bsId, ctx)",
		"function renderContextGraphError(message, ctx)",
		"window.MIDASExplorerGraph.contextView",
		"renderContextGraph:",
		"renderContextGraphEmpty:",
		"renderContextGraphError:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-1: live Context lens entry point %q must remain in context-graph-view.js", want)
		}
	}

	// The Context inspector dispatcher was a separate namespace from
	// `MIDASExplorerGraph.renderer` and was out of scope for
	// D37p-clean-1. D37p-clean-2 retired it as well; the call site
	// must no longer be present.
	if strings.Contains(js, "MIDASExplorerGraph.inspector.register('context', inspectorImpl)") {
		t.Errorf("D37p-clean-2: dead Context inspector dispatcher registration must NOT be present (retired in D37p-clean-2)")
	}
}

// ── C. Authority view: dead lensImpl retired ───────────────────────

func TestExplorer_D37pClean1_AuthorityViewLensImplRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1AuthorityView)

	if strings.Contains(js, "window.MIDASExplorerGraph.renderer.register('authority', lensImpl)") {
		t.Errorf("D37p-clean-1: dead renderer.register('authority', lensImpl) call must be removed from authority-graph-view.js")
	}
	if regexp.MustCompile(`var\s+lensImpl\s*=\s*\{\s*render:\s*function\s*\(payload,\s*mount\)`).MatchString(js) {
		t.Errorf("D37p-clean-1: dead lensImpl object literal must be removed from authority-graph-view.js")
	}
	if strings.Contains(js, "_lensImpl:                   lensImpl,") {
		t.Errorf("D37p-clean-1: dead _lensImpl export must be removed from authority-graph-view.js")
	}
}

// TestExplorer_D37pClean1_AuthorityViewLivePathPreserved pins the live
// Authority lens entry points.
func TestExplorer_D37pClean1_AuthorityViewLivePathPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1AuthorityView)

	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityView",
		"refresh:",
		"renderAuthorityGraph:",
		"renderAuthorityGraphEmpty:",
		"renderAuthorityGraphError:",
		"setAuthorityGraphStatus:",
		"computeNodeOverlays:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-1: live Authority lens entry point %q must remain in authority-graph-view.js", want)
		}
	}
}

// ── D. Cytoscape PoC: dead lensImpl + register IIFE retired ────────

func TestExplorer_D37pClean1_AuthorityPocLensImplRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1AuthorityPoc)

	if strings.Contains(js, "rendered.register('authority', lensImpl);") {
		t.Errorf("D37p-clean-1: dead rendered.register('authority', lensImpl) call must be removed from authority-cytoscape-poc.js")
	}
	if regexp.MustCompile(`function\s+_registerWhenReady\s*\(\)`).MatchString(js) {
		t.Errorf("D37p-clean-1: dead _registerWhenReady deferred-register IIFE must be removed from authority-cytoscape-poc.js")
	}
	if regexp.MustCompile(`var\s+lensImpl\s*=\s*\{\s*render:\s*function\s*\(payload,\s*mount\)`).MatchString(js) {
		t.Errorf("D37p-clean-1: dead lensImpl object literal must be removed from authority-cytoscape-poc.js")
	}
	// The dead `_lensImpl: lensImpl` export must be gone from the
	// cytoscapePoc public surface.
	startIdx := strings.Index(js, "window.MIDASExplorerGraph.cytoscapePoc = {")
	if startIdx < 0 {
		t.Fatalf("D37p-clean-1: cytoscapePoc public surface must remain present")
	}
	tail := js[startIdx:]
	endRel := strings.Index(tail, "};")
	if endRel < 0 {
		t.Fatalf("D37p-clean-1: cytoscapePoc public surface literal must be terminated")
	}
	surface := tail[:endRel]
	if strings.Contains(surface, "_lensImpl:") {
		t.Errorf("D37p-clean-1: dead _lensImpl export must be removed from cytoscapePoc public surface")
	}
}

// TestExplorer_D37pClean1_AuthorityPocLivePathPreserved pins the live
// Authority activation seams in authority-cytoscape-poc.js.
func TestExplorer_D37pClean1_AuthorityPocLivePathPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1AuthorityPoc)

	for _, want := range []string{
		// GraphViewport host registration (D35g, promoted in D37b) is
		// the platform-level activation seam.
		"vp.register('authority', _authorityRendererFactory)",
		// authorityView.refresh patch keeps the production refresh
		// routed through Cytoscape.
		"function _patchAuthorityViewRefresh()",
		"av.refresh = _pocRefresh;",
		"function _pocRefresh(opts)",
		// Camera bus delegate (D37p-impl-4).
		"bus.registerLens('authority'",
		// Selection bridge delegate (D37p-authority-1-impl).
		"_registerAuthoritySelectionBridgeDelegate",
		"bridge.registerLens('authority'",
		// Public surface still exports the Cytoscape PoC API.
		"window.MIDASExplorerGraph.cytoscapePoc",
		"getCy:",
		"fit:",
		"zoomBy:",
		"centerOnRoot:",
		"zoomToSelected:",
		"resetView:",
		"getZoomPercent:",
		"onSelectionChanged:",
		"onViewportChanged:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-1: live Authority activation seam %q must remain in authority-cytoscape-poc.js", want)
		}
	}
}

// ── E. GraphViewport host preserved ────────────────────────────────

func TestExplorer_D37pClean1_GraphViewportPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean1ViewportAsset)

	// GraphViewport host preserves its registry + activation surface
	// + native-context baseline. The Authority renderer id is
	// registered from authority-cytoscape-poc.js (via vp.register('authority',
	// factory)) — it does not appear as a literal in the host.
	for _, want := range []string{
		"register:",
		"unregister:",
		"hasRenderer:",
		"listRegistered:",
		"activateById:",
		"deactivate:",
		"adoptExisting:",
		"data-active-renderer",
		"adoptExisting('native-context')",
		"_baselineId = 'native-context';",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-1: GraphViewport host must still expose %q", want)
		}
	}

	// Authority registers from its own module against the host.
	auth := getExplorerAsset(t, srv, d37pClean1AuthorityPoc)
	if !strings.Contains(auth, "vp.register('authority', _authorityRendererFactory)") {
		t.Errorf("D37p-clean-1: Authority module must still register the 'authority' renderer with the host")
	}
}

// ── F. Live platform surfaces preserved ────────────────────────────

func TestExplorer_D37pClean1_PlatformAssetsServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37pClean1ViewportAsset,
		d37pClean1CameraBusAsset,
		d37pClean1SelBridgeAsset,
		d37pClean1PaneShellAsset,
		d37pClean1ContextRenderer,
		d37pClean1ContextSelBridge,
		d37pClean1ContextPane,
		d37pClean1ContextProvider,
		d37pClean1AuthorityEdge,
		d37pClean1AuthorityWb,
		d37pClean1DrawerAsset,
	} {
		if got := getExplorerAsset(t, srv, asset); len(got) == 0 {
			t.Errorf("D37p-clean-1: platform asset %q must remain served", asset)
		}
	}
}

func TestExplorer_D37pClean1_PlatformContractsIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Camera bus + selection bridge + pane shell expose their core
	// surface unchanged.
	for _, want := range []string{"registerLens:", "subscribe:"} {
		if !strings.Contains(getExplorerAsset(t, srv, d37pClean1CameraBusAsset), want) {
			t.Errorf("D37p-clean-1: graphCameraBus must still export %q", want)
		}
		if !strings.Contains(getExplorerAsset(t, srv, d37pClean1SelBridgeAsset), want) {
			t.Errorf("D37p-clean-1: graphSelectionBridge must still export %q", want)
		}
	}
	if !strings.Contains(getExplorerAsset(t, srv, d37pClean1PaneShellAsset), "registerLensProvider:") {
		t.Errorf("D37p-clean-1: graphSelectedObjectPane must still export registerLensProvider")
	}

	// Drawer still exports its lens registration API.
	if !strings.Contains(getExplorerAsset(t, srv, d37pClean1DrawerAsset), "registerLens:") {
		t.Errorf("D37p-clean-1: graph-drawer.js must still export registerLens")
	}
}

// ── G. DOM markup preserved (no default flip, no retirement) ───────

func TestExplorer_D37pClean1_DomMarkupPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		// Right drawer
		`id="gmap-details"`,
		// Authority canvas-edge tabs wrapper
		`data-authority-canvas-edge-tabs`,
		// Authority workbench
		`id="gmap-authority-workbench"`,
		// Context selected-object pane wrapper
		`data-context-selected-object-pane`,
		// Native graph canvas (legacy Context default surface)
		`id="gmap-canvas"`,
		`id="gmap-scene"`,
		`id="gmap-svg"`,
		// All live platform module scripts still referenced.
		`src="/explorer/assets/js/graph/graph-renderer.js"`,
		`src="/explorer/assets/js/graph/context/context-graph-view.js"`,
		`src="/explorer/assets/js/graph/authority/authority-graph-view.js"`,
		`src="/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37p-clean-1: index.html must still contain %q (no markup retirement in this tranche)", want)
		}
	}
}

// ── H. No default renderer flip ────────────────────────────────────

func TestExplorer_D37pClean1_DefaultRendererUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vp := getExplorerAsset(t, srv, d37pClean1ViewportAsset)

	// Native Context remains the baseline; the strategic Context
	// renderer remains opt-in via the contextRenderer URL flag.
	if !strings.Contains(vp, "_baselineId = 'native-context';") {
		t.Errorf("D37p-clean-1: GraphViewport host must still adopt native-context as the baseline renderer (no default flip)")
	}
	strategic := getExplorerAsset(t, srv, d37pClean1ContextRenderer)
	if !strings.Contains(strategic, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37p-clean-1: strategic Context renderer activation must remain opt-in via the 'contextRenderer' query param")
	}
}

// ── I. Index.html line ceiling ─────────────────────────────────────

func TestExplorer_D37pClean1_IndexHtmlWithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 7820 {
		t.Errorf("D37p-clean-1: index.html line count %d exceeds the existing 7820 ceiling", lines)
	}
}
