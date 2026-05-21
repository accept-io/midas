package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-impl-4 — Camera Toolbar Adapter via Active Camera Bus tests
//
// Pins the end-state of the camera-toolbar convergence:
//
//   • new `graph-camera-toolbar-adapter.js` module is served and
//     loaded after the bus + before lens renderers;
//   • the adapter binds every camera-cluster button to
//     `graphCameraBus.<command>()` (no direct legacy / Authority /
//     strategic-camera calls);
//   • the adapter registers the `'native-context'` delegate that
//     wraps the legacy `MIDASExplorerGraph.camera` surface;
//   • the strategic Context renderer registers a `'context'`
//     delegate when its camera is created and unregisters on
//     destroy;
//   • the Authority module registers an `'authority'` delegate that
//     wraps the existing `cytoscapePoc.*` camera methods;
//   • the Authority capture-phase camera intercepts are retired
//     (only `gmap-authority-context-button` + `gmap-focus-toggle`
//     remain wired in `authority-cytoscape-toolbar.js`);
//   • the three legacy inline camera-button IIFEs in `index.html`
//     are retired (no double-dispatch);
//   • the bus's public surface remains intact;
//   • legacy / Authority / Selected-Object Pane / projection
//     provider / drawer / tray / backend all unchanged.

const (
	d37pImpl4AdapterAsset    = "/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js"
	d37pImpl4BusAsset        = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37pImpl4RendererAsset   = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pImpl4AuthorityPoc    = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37pImpl4AuthorityBridge = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
	d37pImpl4LegacyCamera    = "/explorer/assets/js/graph/graph-camera.js"
	d37pImpl4PaneAsset       = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37pImpl4Provider        = "/explorer/assets/js/graph/context/context-projection-provider.js"
)

// ── A. Adapter asset + load order ────────────────────────────────────

func TestExplorer_D37pImpl4_GraphCameraToolbar_AdapterAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4AdapterAsset)
	if len(js) == 0 {
		t.Fatal("D37p-impl-4: graph-camera-toolbar-adapter.js must be served")
	}
}

func TestExplorer_D37pImpl4_GraphCameraToolbar_ScriptTagWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js"`) {
		t.Errorf("D37p-impl-4: index.html must include <script> for graph-camera-toolbar-adapter.js")
	}
	if c := strings.Count(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js"`); c != 1 {
		t.Errorf("D37p-impl-4: adapter script must be included exactly once (found %d)", c)
	}
}

func TestExplorer_D37pImpl4_GraphCameraToolbar_LoadOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	busIdx       := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"`)
	adapterIdx   := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js"`)
	rendererIdx  := strings.Index(body, `src="/explorer/assets/js/graph/context/context-cytoscape-renderer.js"`)
	if busIdx < 0 || adapterIdx < 0 || rendererIdx < 0 {
		t.Fatal("D37p-impl-4: bus, adapter, and renderer scripts must all be present")
	}
	if busIdx >= adapterIdx {
		t.Errorf("D37p-impl-4: toolbar adapter must load AFTER graph-camera-bus.js (bus idx=%d, adapter idx=%d)", busIdx, adapterIdx)
	}
	if adapterIdx >= rendererIdx {
		t.Errorf("D37p-impl-4: toolbar adapter must load BEFORE context-cytoscape-renderer.js (adapter idx=%d, renderer idx=%d)", adapterIdx, rendererIdx)
	}
}

// ── B. Adapter wires toolbar buttons through the bus ─────────────────

func TestExplorer_D37pImpl4_GraphCameraToolbar_BindsButtonsToBus(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4AdapterAsset)

	// Locked button-id → command mapping must be present.
	for _, want := range []string{
		"'gmap-zoom-in'",
		"'gmap-zoom-out'",
		"'gmap-fit-button'",
		"'gmap-zoom-selected-button'",
		"'gmap-centre-button'",
		"'gmap-reset-view-button'",
		"'zoomIn'",
		"'zoomOut'",
		"'fit'",
		"'focusSelected'",
		"'focusRoot'",
		"'reset'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-4: adapter must declare locked button/command token %q", want)
		}
	}

	// Dispatch path through the bus.
	if !strings.Contains(js, "bus[command]()") {
		t.Errorf("D37p-impl-4: adapter must dispatch via bus[command]() rather than per-engine APIs")
	}

	// Defensive guards against missing bus / missing command.
	if !strings.Contains(js, "if (!bus || typeof bus[command]") {
		t.Errorf("D37p-impl-4: adapter must guard against a missing bus or unknown command before dispatching")
	}
}

// TestExplorer_D37pImpl4_GraphCameraToolbar_AdapterDoesNotCallLegacyOrEngineDirectly
// pins that the adapter does not bypass the bus by calling legacy
// camera helpers, Authority Cytoscape methods, or strategic Context
// camera methods directly.
func TestExplorer_D37pImpl4_GraphCameraToolbar_AdapterDoesNotCallLegacyOrEngineDirectly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4AdapterAsset)

	for _, banned := range []string{
		"setGmapZoom",
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"cytoscapePoc.zoomBy",
		"cytoscapePoc.fit(",
		"cytoscapePoc.centerOnRoot",
		"cytoscapePoc.zoomToSelected",
		"cytoscapePoc.resetView",
		"graphCameraController",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-4: adapter must dispatch via bus, not call %q directly", banned)
		}
	}
}

// ── C. Native-context delegate ───────────────────────────────────────

func TestExplorer_D37pImpl4_GraphCameraToolbar_NativeContextDelegateRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4AdapterAsset)

	if !strings.Contains(js, "registerLens('native-context'") {
		t.Errorf("D37p-impl-4: adapter must register a 'native-context' delegate on the bus")
	}
	// Delegate must wrap the existing legacy camera surface, not
	// reinvent zoom math.
	for _, want := range []string{
		"MIDASExplorerGraph.camera",
		"cam.setZoom",
		"cam.getZoom",
		"cam.fitToBounds",
		"cam.focusRoot",
		"cam.applyFitMode",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-4: native-context delegate must wrap the legacy camera surface (%q)", want)
		}
	}
	// Uses GMAP_ZOOM for step / default values rather than hard-coding.
	if !strings.Contains(js, "GMAP_ZOOM") {
		t.Errorf("D37p-impl-4: native-context delegate must consult window.MIDASGovernanceMap.GMAP_ZOOM for step / default bounds")
	}
}

// ── D. Strategic Context delegate ────────────────────────────────────

func TestExplorer_D37pImpl4_StrategicContext_RegistersDelegateInSpatialPath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4RendererAsset)

	if !strings.Contains(js, "_registerCameraBusDelegate") {
		t.Errorf("D37p-impl-4: strategic Context renderer must declare a camera-bus delegate registration helper")
	}
	if !strings.Contains(js, "bus.registerLens(RENDERER_ID, delegate)") {
		t.Errorf("D37p-impl-4: strategic Context renderer must register its delegate against the renderer's canonical RENDERER_ID")
	}
	if !strings.Contains(js, "_unregisterCameraBusDelegate") {
		t.Errorf("D37p-impl-4: strategic Context renderer must declare a delegate unregister helper")
	}
}

func TestExplorer_D37pImpl4_StrategicContext_RegistersOnCameraCreate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4RendererAsset)

	// Find the _ensureCamera function body and require the bus
	// registration call inside it.
	idx := strings.Index(js, "function _ensureCamera()")
	if idx < 0 {
		t.Fatal("D37p-impl-4: _ensureCamera function not found in strategic renderer")
	}
	end := strings.Index(js[idx:], "\n  function ")
	if end < 0 {
		t.Fatal("D37p-impl-4: _ensureCamera body not delimited in strategic renderer")
	}
	body := js[idx : idx+end]
	if !strings.Contains(body, "_registerCameraBusDelegate()") {
		t.Errorf("D37p-impl-4: _ensureCamera() must call _registerCameraBusDelegate() after a camera instance is created")
	}
}

func TestExplorer_D37pImpl4_StrategicContext_UnregistersOnCameraDestroy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4RendererAsset)

	idx := strings.Index(js, "function _destroyCamera()")
	if idx < 0 {
		t.Fatal("D37p-impl-4: _destroyCamera function not found in strategic renderer")
	}
	end := strings.Index(js[idx:], "\n  }")
	if end < 0 {
		t.Fatal("D37p-impl-4: _destroyCamera body not delimited in strategic renderer")
	}
	body := js[idx : idx+end]
	if !strings.Contains(body, "_unregisterCameraBusDelegate()") {
		t.Errorf("D37p-impl-4: _destroyCamera() must call _unregisterCameraBusDelegate() before destroying the camera")
	}
}

// ── E. Authority delegate ────────────────────────────────────────────

func TestExplorer_D37pImpl4_Authority_RegistersDelegateAtModuleInit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4AuthorityPoc)

	if !strings.Contains(js, "registerLens('authority'") {
		t.Errorf("D37p-impl-4: Authority module must register an 'authority' delegate on the bus")
	}
	// The delegate must wrap the existing Authority camera methods,
	// not reinvent any.
	for _, want := range []string{
		"poc.zoomBy",
		"poc.fit",
		"poc.centerOnRoot",
		"poc.zoomToSelected",
		"poc.resetView",
		"poc.getZoomPercent",
		"poc.ZOOM_STEP_FACTOR",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-4: Authority delegate must wrap the existing camera surface (%q)", want)
		}
	}
}

// ── F. Authority capture-phase camera intercept retired ──────────────

func TestExplorer_D37pImpl4_Authority_CapturePhaseCameraIntercepRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4AuthorityBridge)

	// The Authority bridge's _bindCameraCluster must no longer
	// reference camera button ids in its bindings array. (Helper
	// functions _onZoomIn etc. may remain declared as unused
	// vestiges; the load-bearing change is the bindings array.)
	idx := strings.Index(js, "function _bindCameraCluster()")
	if idx < 0 {
		t.Fatal("D37p-impl-4: _bindCameraCluster function not found in Authority toolbar bridge")
	}
	end := strings.Index(js[idx:], "\n  }")
	if end < 0 {
		t.Fatal("D37p-impl-4: _bindCameraCluster body not delimited in Authority toolbar bridge")
	}
	body := js[idx : idx+end]
	for _, banned := range []string{
		"id: 'gmap-zoom-in'",
		"id: 'gmap-zoom-out'",
		"id: 'gmap-fit-button'",
		"id: 'gmap-centre-button'",
		"id: 'gmap-zoom-selected-button'",
		"id: 'gmap-reset-view-button'",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37p-impl-4: Authority's _bindCameraCluster must NOT bind the camera button %q (retired in favour of the shared bus)", banned)
		}
	}
	// Non-camera bindings stay.
	for _, want := range []string{
		"id: 'gmap-authority-context-button'",
		"id: 'gmap-focus-toggle'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37p-impl-4: Authority's _bindCameraCluster must KEEP the non-camera binding %q", want)
		}
	}
}

// ── G. Legacy inline camera IIFEs removed from index.html ────────────

func TestExplorer_D37pImpl4_IndexHtml_LegacyInlineCameraIIFEsRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// The three legacy IIFEs must no longer exist in markup.
	for _, banned := range []string{
		"wireGmapZoomControls",
		"wireGmapCentreButton",
		"wireGmapFitButton",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37p-impl-4: legacy inline IIFE %q must be retired (toolbar adapter now owns this binding)", banned)
		}
	}
	// And the unique camera-button id lookups the removed IIFEs
	// used must no longer appear in markup. (Substrings tighter
	// than the bare variable names so they don't false-positive
	// against unrelated `signOutBtn` / similar element names; and
	// not so broad that they catch the legitimate non-toolbar
	// callers of `fitGmapToBounds` in the focus-mode handler.)
	for _, banned := range []string{
		`const inBtn = document.getElementById('gmap-zoom-in')`,
		`const outBtn = document.getElementById('gmap-zoom-out')`,
		`document.getElementById('gmap-zoom-reset')`,
		`focusGmapOnRoot(prefix + currentGraphRootId)`,
		`btn.addEventListener('click', () => fitGmapToBounds())`,
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37p-impl-4: legacy inline camera-button binding %q must be retired (toolbar adapter now owns this)", banned)
		}
	}
}

// ── H. Bus public surface unchanged ──────────────────────────────────

func TestExplorer_D37pImpl4_GraphCameraBus_SurfaceUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl4BusAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.graphCameraBus",
		"registerLens:",
		"unregisterLens:",
		"setActiveLens:",
		"getActiveLens:",
		"getRegisteredLensIds:",
		"getActiveDelegate:",
		"zoomIn:",
		"zoomOut:",
		"fit:",
		"reset:",
		"focusRoot:",
		"focusSelected:",
		"setZoom:",
		"getZoom:",
		"dispatch:",
		"subscribe:",
		"destroy:",
		"_constants:",
		"COMMANDS:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-4: bus public surface must remain intact (%q)", want)
		}
	}
}

// ── I. Foundation preservation ───────────────────────────────────────

func TestExplorer_D37pImpl4_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Legacy camera unchanged.
	legacyJS := getExplorerAsset(t, srv, d37pImpl4LegacyCamera)
	for _, want := range []string{
		`getElementById('gmap-canvas')`,
		`getElementById('gmap-scene')`,
		"state.positions",
	} {
		if !strings.Contains(legacyJS, want) {
			t.Errorf("D37p-impl-4: legacy graph-camera.js must still reference %q (no behavioural change)", want)
		}
	}

	// Strategic Context renderer identity intact.
	rendererJS := getExplorerAsset(t, srv, d37pImpl4RendererAsset)
	if !strings.Contains(rendererJS, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37p-impl-4: strategic Context renderer canonical id must remain 'context'")
	}
	if !strings.Contains(rendererJS, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37p-impl-4: strategic Context activation query param must remain 'contextRenderer'")
	}

	// Authority module preserved (registers + camera surface still
	// exported).
	authorityJS := getExplorerAsset(t, srv, d37pImpl4AuthorityPoc)
	for _, want := range []string{
		"window.MIDASExplorerGraph.cytoscapePoc",
		"vp.register('authority'",
	} {
		if !strings.Contains(authorityJS, want) {
			t.Errorf("D37p-impl-4: Authority module must still expose %q", want)
		}
	}

	// Selected-Object Pane wrapper still present in markup; projection
	// provider still wired.
	if !strings.Contains(body, "gmap-context-selected-object-pane") {
		t.Errorf("D37p-impl-4: Selected-Object Pane wrapper must remain present")
	}
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-projection-provider.js"`) {
		t.Errorf("D37p-impl-4: context-projection-provider.js script tag must remain")
	}

	// No temporary renderer identities introduced.
	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37p-impl-4: must not introduce temporary renderer name %q", banned)
		}
	}
}

// TestExplorer_D37pImpl4_IndexHtml_WithinCeiling — the removed inline
// IIFEs net ~25 lines off; with the new adapter script tag added, the
// final count should be comfortably under the 7820 ceiling.
func TestExplorer_D37pImpl4_IndexHtml_WithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 7820 {
		t.Errorf("D37p-impl-4: index.html line count %d exceeds the existing 7820 ceiling", lines)
	}
}

// ── J. Sibling assets still served ───────────────────────────────────

func TestExplorer_D37pImpl4_PlatformAndLensAssetsServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, asset := range []string{
		d37pImpl4AdapterAsset,
		d37pImpl4BusAsset,
		d37pImpl4RendererAsset,
		d37pImpl4AuthorityPoc,
		d37pImpl4AuthorityBridge,
		d37pImpl4LegacyCamera,
		d37pImpl4PaneAsset,
		d37pImpl4Provider,
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37p-impl-4: asset %q must remain served", asset)
		}
	}
}
