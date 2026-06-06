package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-platform-3 — Active Graph Camera Command Bus tests
//
// Pins the platform contract for the new
// `graph-platform/graph-camera-bus.js` module:
//
//   • asset is served;
//   • script tag is wired exactly once, after
//     graph-camera-controller.js and before
//     context-cytoscape-renderer.js;
//   • public surface: registration, active-lens, dispatch, lifecycle;
//   • locked COMMANDS allowlist;
//   • delegate-contract pattern (replace on re-register, idempotent
//     unregister, command no-op on missing delegate, try/catch around
//     delegate calls);
//   • active-renderer tracking via the host's `getActiveRendererId`
//     + `data-active-renderer` attribute observation;
//   • purity (no DOM mutation, no graph-engine APIs, no lens
//     coupling, no backend coupling, no GraphViewport lifecycle);
//   • no temporary renderer identities;
//   • no delegates registered in this tranche (renderers still own
//     their own camera code);
//   • foundation preserved (controller / stage / renderer / Authority
//     / pane / projection-provider / legacy camera all unchanged).
//
// Browser behaviour does not change in this tranche; tests are
// source-contract pins (asset-text + load-order + foundation).

const (
	d37pPlatform3BusAsset      = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37pPlatform3CameraAsset   = "/explorer/assets/js/graph/graph-platform/graph-camera-controller.js"
	d37pPlatform3StageAsset    = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37pPlatform3RendererAsset = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pPlatform3LegacyCamera  = "/explorer/assets/js/graph/graph-camera.js"
	d37pPlatform3PaneAsset     = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37pPlatform3SelBridge     = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37pPlatform3Provider      = "/explorer/assets/js/graph/context/context-projection-provider.js"
	d37pPlatform3Handoff       = "/explorer/assets/js/graph/context/context-projection-handoff.js"
	d37pPlatform3Authority     = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

// ── A. Asset presence + load order ──────────────────────────────────

func TestExplorer_D37pPlatform3_GraphCameraBus_AssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)
	if len(js) == 0 {
		t.Fatal("D37p-platform-3: graph-camera-bus.js must be served")
	}
}

func TestExplorer_D37pPlatform3_GraphCameraBus_ScriptTagWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"`) {
		t.Errorf("D37p-platform-3: index.html must include <script> for graph-camera-bus.js")
	}
	if c := strings.Count(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"`); c != 1 {
		t.Errorf("D37p-platform-3: graph-camera-bus.js must be included exactly once (found %d)", c)
	}
}

// TestExplorer_D37pPlatform3_GraphCameraBus_LoadOrder pins:
//
//   graph-stage.js
//   → graph-camera-controller.js
//   → graph-camera-bus.js
//   → context-cytoscape-renderer.js
//
// The bus must be parsed after the controller (so lens delegates
// that wrap controller instances can be registered) and before any
// lens renderer (so the renderer can opt into registration in a
// later tranche).
func TestExplorer_D37pPlatform3_GraphCameraBus_LoadOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	stageIdx    := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-stage.js"`)
	cameraIdx   := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-controller.js"`)
	busIdx      := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"`)
	rendererIdx := strings.Index(body, `src="/explorer/assets/js/graph/context/context-cytoscape-renderer.js"`)
	if stageIdx < 0 || cameraIdx < 0 || busIdx < 0 || rendererIdx < 0 {
		t.Fatal("D37p-platform-3: stage, camera-controller, bus, and renderer <script> tags must all be present")
	}
	if cameraIdx >= busIdx {
		t.Errorf("D37p-platform-3: graph-camera-bus.js must load AFTER graph-camera-controller.js (camera idx=%d, bus idx=%d)", cameraIdx, busIdx)
	}
	if busIdx >= rendererIdx {
		t.Errorf("D37p-platform-3: graph-camera-bus.js must load BEFORE context-cytoscape-renderer.js (bus idx=%d, renderer idx=%d)", busIdx, rendererIdx)
	}
}

// ── B. Public surface ───────────────────────────────────────────────

func TestExplorer_D37pPlatform3_GraphCameraBus_PublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

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
		"ACTIVE_RENDERER_ATTRIBUTE:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-platform-3: bus public surface must declare %q", want)
		}
	}
}

// ── C. Locked command allowlist ─────────────────────────────────────

func TestExplorer_D37pPlatform3_GraphCameraBus_LockedCommandAllowlist(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	// All eight locked commands must appear as string literals in
	// the COMMANDS array.
	for _, want := range []string{
		"'zoomIn'",
		"'zoomOut'",
		"'fit'",
		"'reset'",
		"'focusRoot'",
		"'focusSelected'",
		"'setZoom'",
		"'getZoom'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-platform-3: COMMANDS array must declare command %q", want)
		}
	}
	// The allowlist gate must exist.
	if !strings.Contains(js, "_isCommandAllowed") {
		t.Errorf("D37p-platform-3: dispatch must gate commands through a locked allowlist")
	}
}

// TestExplorer_D37pPlatform3_GraphCameraBus_DispatchUsedByPublicMethods
// pins that every public command method routes through dispatch(),
// so toolbar / hotkey / DevTools consumers all share one execution
// path.
func TestExplorer_D37pPlatform3_GraphCameraBus_DispatchUsedByPublicMethods(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	for _, want := range []string{
		"return dispatch('zoomIn')",
		"return dispatch('zoomOut')",
		"return dispatch('fit')",
		"return dispatch('reset')",
		"return dispatch('focusRoot')",
		"return dispatch('focusSelected')",
		"return dispatch('setZoom'",
		"return dispatch('getZoom')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-platform-3: public method must delegate via dispatch (%q)", want)
		}
	}
}

// ── D. Delegate contract ────────────────────────────────────────────

func TestExplorer_D37pPlatform3_GraphCameraBus_DelegateRegistrySemantics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	if !strings.Contains(js, "var _registry") {
		t.Errorf("D37p-platform-3: bus must keep an internal delegate registry")
	}
	// REPLACE policy on re-register.
	if !strings.Contains(js, "_registry[lensId] = delegate") {
		t.Errorf("D37p-platform-3: registerLens must REPLACE the delegate for an existing lens id")
	}
	// Idempotent unregister.
	if !strings.Contains(js, "delete _registry[lensId]") {
		t.Errorf("D37p-platform-3: unregisterLens must remove the delegate entry")
	}
	// Notify pattern present.
	for _, want := range []string{
		"'lens_registered'",
		"'lens_unregistered'",
		"'active_lens_changed'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-platform-3: bus must emit %q events", want)
		}
	}
}

func TestExplorer_D37pPlatform3_GraphCameraBus_DispatchHandlesMissingAndThrowing(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	// Missing delegate → return null. The dispatch source must
	// short-circuit on missing delegate / missing method.
	if !strings.Contains(js, "if (!delegate) return null;") {
		t.Errorf("D37p-platform-3: dispatch must return null when no active delegate is registered")
	}
	if !strings.Contains(js, "if (typeof fn !== 'function') return null;") {
		t.Errorf("D37p-platform-3: dispatch must return null when the delegate does not implement the command")
	}
	// Throwing delegate → caught + 'command_error' event emitted.
	if !strings.Contains(js, "} catch (err) {") {
		t.Errorf("D37p-platform-3: dispatch must catch delegate exceptions")
	}
	if !strings.Contains(js, "'command_error'") {
		t.Errorf("D37p-platform-3: dispatch must emit a 'command_error' event on delegate exception")
	}
}

// ── E. Active renderer tracking ─────────────────────────────────────

func TestExplorer_D37pPlatform3_GraphCameraBus_TracksActiveRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	// Reads the host's identity API first.
	if !strings.Contains(js, "g.viewport.getActiveRendererId") {
		t.Errorf("D37p-platform-3: bus must read MIDASExplorerGraph.viewport.getActiveRendererId() when available")
	}
	// Falls back to the data-active-renderer attribute.
	if !strings.Contains(js, "'data-active-renderer'") {
		t.Errorf("D37p-platform-3: bus must reference 'data-active-renderer' as the host attribute it observes")
	}
	if !strings.Contains(js, "ACTIVE_RENDERER_ATTRIBUTE") {
		t.Errorf("D37p-platform-3: bus must declare an ACTIVE_RENDERER_ATTRIBUTE constant")
	}
	// MutationObserver wiring.
	if !strings.Contains(js, "MutationObserver") {
		t.Errorf("D37p-platform-3: bus must observe attribute changes via MutationObserver")
	}
	if !strings.Contains(js, "attributeFilter: [ACTIVE_RENDERER_ATTRIBUTE]") {
		t.Errorf("D37p-platform-3: bus's MutationObserver must filter on the active-renderer attribute")
	}
	if !strings.Contains(js, "'midas-graph-viewport'") {
		t.Errorf("D37p-platform-3: bus must look up the host element by class 'midas-graph-viewport'")
	}
}

func TestExplorer_D37pPlatform3_GraphCameraBus_DoesNotCallViewportLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	for _, banned := range []string{
		"viewport.activate",
		"activateById",
		"viewport.register",
		"viewport.deactivate",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-platform-3: bus must not enter the GraphViewport lifecycle (%q)", banned)
		}
	}
}

// ── F. Purity ───────────────────────────────────────────────────────

func TestExplorer_D37pPlatform3_GraphCameraBus_NoDomMutation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	for _, banned := range []string{
		"createElement",
		"appendChild",
		"innerHTML",
		"replaceChild",
		"removeChild",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-platform-3: bus must not mutate DOM (%q)", banned)
		}
	}
}

func TestExplorer_D37pPlatform3_GraphCameraBus_NoBackendCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/",
		"MIDASExplorerAPI",
		"publishProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-platform-3: bus must not couple to backend / projection (%q)", banned)
		}
	}
}

func TestExplorer_D37pPlatform3_GraphCameraBus_NoGraphEngineCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	for _, banned := range []string{
		"cytoscape",
		"Cytoscape",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-platform-3: bus must not reference any graph engine (%q)", banned)
		}
	}
	if regexp.MustCompile(`\bcy\.`).MatchString(js) {
		t.Errorf("D37p-platform-3: bus must not access a graph-engine instance via cy.<member>")
	}
}

func TestExplorer_D37pPlatform3_GraphCameraBus_NoLensSpecificCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	for _, banned := range []string{
		"contextProjection",
		"contextSelectionBridge",
		"contextSelectedObjectPane",
		"contextEvidenceTray",
		"authorityWorkbench",
		"authority-canvas-edge",
		".setName(",
		".setFields(",
		".setGovernance(",
		".setActions(",
		".setInlineActions(",
		"#gmap-canvas",
		"#gmap-scene",
		"#gmap-svg",
		".context-card",
		".gmap-node",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-platform-3: bus must not contain lens-specific coupling (%q)", banned)
		}
	}
}

func TestExplorer_D37pPlatform3_GraphCameraBus_NoTemporaryRendererNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-platform-3: bus must not introduce temporary renderer name %q", banned)
		}
	}
}

// ── G. No delegate pre-registered in this tranche ───────────────────

// TestExplorer_D37pPlatform3_GraphCameraBus_NoDelegatesYet pins that
// the bus does NOT auto-register any lens delegate. Delegate
// registrations are the next tranche's job (D37p-impl-4); shipping
// them in the platform module would defeat the lens-agnostic intent.
func TestExplorer_D37pPlatform3_GraphCameraBus_NoDelegatesYet(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3BusAsset)

	for _, banned := range []string{
		"registerLens('context'",
		"registerLens('authority'",
		"registerLens('native-context'",
		"registerLens('knowledge-graph'",
		`registerLens("context"`,
		`registerLens("authority"`,
		`registerLens("native-context"`,
		`registerLens("knowledge-graph"`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-platform-3: bus must not pre-register any lens delegate (%q)", banned)
		}
	}
}

// TestExplorer_D37pPlatform3_BusConsumersScopedCorrectly pins which
// modules talk to the bus from D37p-impl-4 onwards. The original
// temporal pin ("no module calls the bus yet") expired when
// D37p-impl-4 wired the Authority and strategic-Context delegates.
// This is the permanent successor:
//
//   • the strategic Context renderer DOES register a delegate;
//   • the Authority module DOES register a delegate;
//   • every other lens / platform module the bus interacts with
//     remains free of `graphCameraBus` references.
//
// Adding a future delegate (e.g. Knowledge) requires updating the
// allow-list below in the same tranche that wires the registration.
func TestExplorer_D37pPlatform3_BusConsumersScopedCorrectly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Modules that ARE expected to consume the bus from D37p-impl-4 on.
	allowed := map[string]bool{
		d37pPlatform3RendererAsset: true, // strategic Context renderer
		d37pPlatform3Authority:     true, // Authority Cytoscape module
	}

	for _, asset := range []string{
		d37pPlatform3RendererAsset,
		d37pPlatform3Authority,
		d37pPlatform3LegacyCamera,
		d37pPlatform3PaneAsset,
		d37pPlatform3SelBridge,
		d37pPlatform3Provider,
		d37pPlatform3Handoff,
		d37pPlatform3CameraAsset,
		d37pPlatform3StageAsset,
	} {
		js := getExplorerAsset(t, srv, asset)
		mentions := strings.Contains(js, "graphCameraBus")
		if allowed[asset] && !mentions {
			t.Errorf("D37p-impl-4: expected bus consumer %q to register a delegate via graphCameraBus", asset)
		}
		if !allowed[asset] && mentions {
			t.Errorf("D37p-impl-4: module %q must not call graphCameraBus (not on the allow-list)", asset)
		}
	}
}

// ── H. Foundation preservation ──────────────────────────────────────

func TestExplorer_D37pPlatform3_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Existing platform assets still served.
	for _, asset := range []string{
		d37pPlatform3StageAsset,
		d37pPlatform3CameraAsset,
		d37pPlatform3RendererAsset,
		d37pPlatform3LegacyCamera,
		d37pPlatform3PaneAsset,
		d37pPlatform3SelBridge,
		d37pPlatform3Provider,
		d37pPlatform3Handoff,
		d37pPlatform3Authority,
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37p-platform-3: existing asset %q must remain served", asset)
		}
	}

	// Strategic Context renderer identity intact.
	rendererJS := getExplorerAsset(t, srv, d37pPlatform3RendererAsset)
	if !strings.Contains(rendererJS, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37p-platform-3: strategic Context renderer canonical id must remain 'context'")
	}
	if !strings.Contains(rendererJS, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37p-platform-3: strategic activation query param must remain 'contextRenderer'")
	}

	// Selected-Object Pane wrapper still present in markup.
	if !strings.Contains(body, "gmap-context-selected-object-pane") {
		t.Errorf("D37p-platform-3: Selected-Object Pane wrapper must remain present")
	}
	// Projection provider still wired.
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-projection-provider.js"`) {
		t.Errorf("D37p-platform-3: context-projection-provider.js script tag must remain")
	}
	// No temporary renderer identities introduced anywhere in markup.
	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37p-platform-3: must not introduce temporary renderer name %q", banned)
		}
	}
}

// TestExplorer_D37pPlatform3_LegacyCameraIntact reasserts that the
// legacy camera module is unchanged — D37p-impl-4 will register it
// as a delegate, but the source itself is not modified in this
// tranche.
func TestExplorer_D37pPlatform3_LegacyCameraIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPlatform3LegacyCamera)

	for _, want := range []string{
		`getElementById('gmap-canvas')`,
		`getElementById('gmap-scene')`,
		"state.positions",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-platform-3: legacy camera must still reference %q", want)
		}
	}
}

// ── I. Index.html line count ────────────────────────────────────────

func TestExplorer_D37pPlatform3_IndexHtmlWithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 8000 {
		t.Errorf("D37p-platform-3: index.html line count %d exceeds the existing 8000 ceiling — pin bump required", lines)
	}
}
