package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-selection-1 — Shared Selection Bridge Contract tests
//
// Pins the new `graph-platform/graph-selection-bridge.js` module and
// the Context-facade integration:
//
//   • shared bridge served + script tag wired before
//     context-selection-bridge.js;
//   • public surface (registerLens / unregisterLens / selectCard /
//     clearSelection / getSelected / getCurrentCard /
//     getCurrentNodeRef / getActiveLens / subscribe / subscribeLens /
//     handleAction / registerActionHandler / unregisterActionHandler /
//     setActiveLens / destroy / _constants);
//   • locked EVENT vocabulary;
//   • normalised selection payload shape;
//   • lens registry semantics + active-lens tracking;
//   • action dispatch routing + error handling;
//   • purity (no DOM, no graph-engine, no lens-specific kinds, no
//     drawer / pane / tray / Authority surface coupling);
//   • Context facade still exposes its existing surface;
//   • Context facade now pushes to the shared bridge after its own
//     side effects;
//   • Context facade registers as the 'context' lens delegate on
//     the shared bridge;
//   • drawer / evidence-tray side effects remain in the Context
//     facade, not in the shared bridge;
//   • foundation preserved (Authority / pane / drawer / projection
//     provider / strategic renderer identity / default renderer
//     behaviour all intact).

const (
	d37pSel1SharedAsset    = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37pSel1ContextAsset   = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37pSel1RendererAsset  = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pSel1PaneAsset      = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37pSel1AuthorityPoc   = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37pSel1AuthorityEdge  = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37pSel1Drawer         = "/explorer/assets/js/graph/graph-drawer.js"
	d37pSel1Provider       = "/explorer/assets/js/graph/context/context-projection-provider.js"
	d37pSel1CameraBusAsset = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37pSel1StageAsset     = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
)

// ── A. Shared bridge asset + load order ─────────────────────────────

func TestExplorer_D37pSelection1_SharedBridge_AssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)
	if len(js) == 0 {
		t.Fatal("D37p-selection-1: graph-selection-bridge.js must be served")
	}
}

func TestExplorer_D37pSelection1_SharedBridge_ScriptTagWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, `src="/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"`) {
		t.Errorf("D37p-selection-1: index.html must include <script> for graph-selection-bridge.js")
	}
	if c := strings.Count(body, `src="/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"`); c != 1 {
		t.Errorf("D37p-selection-1: shared bridge script must be included exactly once (found %d)", c)
	}
}

// TestExplorer_D37pSelection1_SharedBridge_LoadOrder pins:
//
//   graph-selection-bridge.js → context-selection-bridge.js
//
// so the Context facade can register itself as the 'context' lens
// delegate at module init.
func TestExplorer_D37pSelection1_SharedBridge_LoadOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	sharedIdx  := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"`)
	contextIdx := strings.Index(body, `src="/explorer/assets/js/graph/context/context-selection-bridge.js"`)
	rendererIdx := strings.Index(body, `src="/explorer/assets/js/graph/context/context-cytoscape-renderer.js"`)
	paneIdx := strings.Index(body, `src="/explorer/assets/js/graph/context/context-selected-object-pane.js"`)
	if sharedIdx < 0 || contextIdx < 0 || rendererIdx < 0 || paneIdx < 0 {
		t.Fatal("D37p-selection-1: required scripts must all be present in index.html")
	}
	if sharedIdx >= contextIdx {
		t.Errorf("D37p-selection-1: graph-selection-bridge.js must load BEFORE context-selection-bridge.js (shared idx=%d, context idx=%d)", sharedIdx, contextIdx)
	}
	if sharedIdx >= rendererIdx {
		t.Errorf("D37p-selection-1: graph-selection-bridge.js must load BEFORE context-cytoscape-renderer.js (shared idx=%d, renderer idx=%d)", sharedIdx, rendererIdx)
	}
	if sharedIdx >= paneIdx {
		t.Errorf("D37p-selection-1: graph-selection-bridge.js must load BEFORE context-selected-object-pane.js (shared idx=%d, pane idx=%d)", sharedIdx, paneIdx)
	}
}

// ── B. Public surface ───────────────────────────────────────────────

func TestExplorer_D37pSelection1_SharedBridge_PublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.graphSelectionBridge",
		"registerLens:",
		"unregisterLens:",
		"selectCard:",
		"clearSelection:",
		"getSelected:",
		"getCurrentCard:",
		"getCurrentNodeRef:",
		"getActiveLens:",
		"subscribe:",
		"subscribeLens:",
		"handleAction:",
		"registerActionHandler:",
		"unregisterActionHandler:",
		"setActiveLens:",
		"destroy:",
		"_constants:",
		"EVENTS:",
		"DEFAULT_ACTION_SURFACE:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-selection-1: shared bridge public surface must declare %q", want)
		}
	}
}

// ── C. Locked event vocabulary ──────────────────────────────────────

func TestExplorer_D37pSelection1_SharedBridge_LockedEventVocabulary(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	for _, want := range []string{
		"'selection_changed'",
		"'selection_cleared'",
		"'action_dispatched'",
		"'action_error'",
		"'lens_registered'",
		"'lens_unregistered'",
		"'active_lens_changed'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-selection-1: EVENTS vocabulary must declare %q", want)
		}
	}
}

// ── D. Normalised selection payload shape ──────────────────────────

func TestExplorer_D37pSelection1_SharedBridge_NormalisedShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	if !strings.Contains(js, "function _normalise") {
		t.Errorf("D37p-selection-1: shared bridge must declare a normalisation helper for selection payloads")
	}
	for _, want := range []string{
		"lens:",
		"cardId:",
		"kind:",
		"sourceNodeRef:",
		"card:",
		"selectedAt:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-selection-1: normalised payload must carry %q", want)
		}
	}
}

// ── E. Delegate registry + active-lens tracking ────────────────────

func TestExplorer_D37pSelection1_SharedBridge_DelegateRegistrySemantics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	if !strings.Contains(js, "var _registry") {
		t.Errorf("D37p-selection-1: shared bridge must keep an internal delegate registry")
	}
	// REPLACE policy on re-register + delete on unregister.
	if !strings.Contains(js, "_registry[lensId] = delegate") {
		t.Errorf("D37p-selection-1: registerLens must REPLACE the delegate for an existing lens id")
	}
	if !strings.Contains(js, "delete _registry[lensId]") {
		t.Errorf("D37p-selection-1: unregisterLens must remove the delegate entry")
	}
}

func TestExplorer_D37pSelection1_SharedBridge_ActiveLensTracking(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	if !strings.Contains(js, "var _activeLensId") {
		t.Errorf("D37p-selection-1: shared bridge must store active lens id")
	}
	if !strings.Contains(js, "function setActiveLens") {
		t.Errorf("D37p-selection-1: shared bridge must expose setActiveLens")
	}
	// Active-lens tracking must NOT enter the renderer lifecycle.
	for _, banned := range []string{
		"viewport.activateById",
		"viewport.activate(",
		"viewport.deactivate",
		"viewport.register",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-selection-1: shared bridge must NOT call GraphViewport lifecycle (%q)", banned)
		}
	}
}

// ── F. Subscription + bad-subscriber catch ─────────────────────────

func TestExplorer_D37pSelection1_SharedBridge_SubscriptionPattern(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	if !strings.Contains(js, "var _subscribers") {
		t.Errorf("D37p-selection-1: shared bridge must keep a subscriber list")
	}
	if !strings.Contains(js, "function subscribeLens") {
		t.Errorf("D37p-selection-1: shared bridge must support lens-scoped subscribers via subscribeLens")
	}
	// Bad-subscriber try/catch.
	if !regexp.MustCompile(`(?s)try \{ entry\.handler\(event\); \}\s*catch \(_\) \{`).MatchString(js) {
		t.Errorf("D37p-selection-1: subscriber dispatch must be wrapped in try/catch so a bad subscriber cannot stop the rest")
	}
}

// ── G. Action dispatch ─────────────────────────────────────────────

func TestExplorer_D37pSelection1_SharedBridge_ActionDispatch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	if !strings.Contains(js, "var _actionHandlers") {
		t.Errorf("D37p-selection-1: shared bridge must keep an action handler registry")
	}
	for _, want := range []string{
		"function handleAction",
		"function registerActionHandler",
		"function unregisterActionHandler",
		"_actionHandlers[kind]",
		"del.handleAction",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-selection-1: action dispatch must include %q", want)
		}
	}
	// Action errors must be caught + emit `action_error`. The
	// catch form may span a newline (`}\n      catch (err) {`) so
	// we use a flexible whitespace-tolerant regex.
	if !strings.Contains(js, "'action_error'") {
		t.Errorf("D37p-selection-1: action dispatch must emit 'action_error' on handler exception")
	}
	if !regexp.MustCompile(`\}\s*catch\s*\(err`).MatchString(js) {
		t.Errorf("D37p-selection-1: action dispatch must catch handler exceptions")
	}
}

// ── H. Purity — no DOM mutation, no DOM access ─────────────────────

func TestExplorer_D37pSelection1_SharedBridge_NoDomAccess(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	for _, banned := range []string{
		"document.",
		"querySelector",
		"getElementById",
		"createElement",
		"appendChild",
		"innerHTML",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-selection-1: shared bridge must not access DOM (%q)", banned)
		}
	}
}

func TestExplorer_D37pSelection1_SharedBridge_NoBackendCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/",
		"MIDASExplorerAPI",
		"publishProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-selection-1: shared bridge must not couple to backend / projection (%q)", banned)
		}
	}
}

func TestExplorer_D37pSelection1_SharedBridge_NoGraphEngineCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	for _, banned := range []string{
		"cytoscape",
		"Cytoscape",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-selection-1: shared bridge must not reference graph engine (%q)", banned)
		}
	}
	if regexp.MustCompile(`\bcy\.`).MatchString(js) {
		t.Errorf("D37p-selection-1: shared bridge must not access a graph-engine instance via cy.<member>")
	}
}

func TestExplorer_D37pSelection1_SharedBridge_NoDrawerOrPaneOrTrayCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	for _, banned := range []string{
		".setName(",
		".setFields(",
		".setGovernance(",
		".setActions(",
		".setInlineActions(",
		"contextEvidenceTray",
		"contextSelectedObjectPane",
		"authority-canvas-edge",
		"authorityWorkbench",
		"#gmap-canvas",
		"#gmap-scene",
		"#gmap-svg",
		".gmap-node",
		".context-card",
		".context-renderer-stage",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-selection-1: shared bridge must not couple to lens-specific surfaces (%q)", banned)
		}
	}
}

func TestExplorer_D37pSelection1_SharedBridge_NoLensSpecificKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	for _, banned := range []string{
		"business_service",
		"decision_surface",
		"ai_system",
		"authority_profile",
		"authority_grant",
		"fail_mode_policy",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-selection-1: shared bridge must remain lens-agnostic; found kind %q", banned)
		}
	}
	// "agent" alone would false-positive across natural-English words
	// (e.g. "Activate by id"), so use a word-boundary regex anchored
	// to a context-likely-lens-specific neighbour.
	if regexp.MustCompile(`\b(authority_)?agent\b`).MatchString(js) {
		t.Errorf("D37p-selection-1: shared bridge must not reference the `agent` lens kind")
	}
}

func TestExplorer_D37pSelection1_SharedBridge_NoTemporaryRendererNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1SharedAsset)

	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-selection-1: shared bridge must not introduce temporary renderer name %q", banned)
		}
	}
}

// ── I. Context facade integration ──────────────────────────────────

func TestExplorer_D37pSelection1_ContextFacade_PreservesPublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1ContextAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.contextSelectionBridge",
		"selectCard:",
		"clearSelection:",
		"getSelected:",
		"getCurrentCard:",
		"subscribe:",
		"handleAction:",
		"legacyActionsFromCard:",
		"toLegacyActionShape:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-selection-1: Context facade must still expose %q", want)
		}
	}
}

func TestExplorer_D37pSelection1_ContextFacade_RegistersWithSharedBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1ContextAsset)

	if !strings.Contains(js, "graphSelectionBridge") {
		t.Errorf("D37p-selection-1: Context facade must reference graphSelectionBridge")
	}
	if !strings.Contains(js, "bridge.registerLens('context'") {
		t.Errorf("D37p-selection-1: Context facade must register itself as the 'context' lens delegate")
	}
	// Facade pushes to shared bridge after side effects.
	for _, want := range []string{
		"_pushSelectionToSharedBridge",
		"_pushClearToSharedBridge",
		"bridge.selectCard(",
		"bridge.clearSelection()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-selection-1: Context facade must push selection state to the shared bridge (%q)", want)
		}
	}
}

// TestExplorer_D37pSelection1_ContextFacade_RetainsSideEffects pins
// that the Context facade still owns the drawer / tray side effects
// that D37o-impl-5 established. These remain in the LENS facade,
// not in the shared bridge.
func TestExplorer_D37pSelection1_ContextFacade_RetainsSideEffects(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSel1ContextAsset)

	for _, want := range []string{
		"MIDASExplorerGraph.selection",         // legacy multi-select state
		"_populateInspector",                   // drawer inspector frame
		"insp.setName",
		"insp.setFields",
		"insp.setGovernance",
		"insp.setActions",
		"contextEvidenceTray",                  // bottom tray
		"tray.notifySelectionChanged",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-selection-1: Context facade must retain D37o-impl-5 side effect %q", want)
		}
	}
}

// ── J. Foundation preservation ─────────────────────────────────────

func TestExplorer_D37pSelection1_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Selected-Object Pane, drawer, Authority canvas-edge, camera
	// bus, stage assets all still served.
	for _, asset := range []string{
		d37pSel1RendererAsset,
		d37pSel1PaneAsset,
		d37pSel1AuthorityPoc,
		d37pSel1AuthorityEdge,
		d37pSel1Drawer,
		d37pSel1Provider,
		d37pSel1CameraBusAsset,
		d37pSel1StageAsset,
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37p-selection-1: foundation asset %q must remain served", asset)
		}
	}

	// Strategic Context renderer identity unchanged.
	rendererJS := getExplorerAsset(t, srv, d37pSel1RendererAsset)
	if !strings.Contains(rendererJS, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37p-selection-1: strategic Context renderer canonical id must remain 'context'")
	}
	if !strings.Contains(rendererJS, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37p-selection-1: strategic activation query param must remain 'contextRenderer'")
	}

	// Selected-Object Pane wrapper still present in markup.
	if !strings.Contains(body, "gmap-context-selected-object-pane") {
		t.Errorf("D37p-selection-1: Selected-Object Pane wrapper must remain present")
	}
	// Evidence tray markup still present.
	if !strings.Contains(body, "gmap-evidence-tray") {
		t.Errorf("D37p-selection-1: evidence tray markup must remain present")
	}
	// Right drawer markup still present.
	if !strings.Contains(body, "gmap-details") {
		t.Errorf("D37p-selection-1: right drawer markup must remain present")
	}
	// Authority canvas-edge wrapper still present.
	if !strings.Contains(body, "gmap-canvas-edge-tabs") {
		t.Errorf("D37p-selection-1: Authority canvas-edge wrapper must remain present")
	}
	// No temporary renderer names introduced.
	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37p-selection-1: must not introduce temporary renderer name %q", banned)
		}
	}
}

// TestExplorer_D37pSelection1_AuthorityMigrated converts the previous
// temporal pin (D37p-selection-1 era: "Authority must not reference
// graphSelectionBridge yet") into its now-correct inverse following
// D37p-authority-1-impl. The Authority module is the FIRST non-
// Context lens to register a delegate with the shared bridge. The
// detailed Authority delegate / publish / recursion-discipline pins
// live in explorer_authority_selection_bridge_test.go; this test
// just guards against a regression that silently removes the
// Authority↔bridge wiring.
func TestExplorer_D37pSelection1_AuthorityMigrated(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authorityJS := getExplorerAsset(t, srv, d37pSel1AuthorityPoc)

	if !strings.Contains(authorityJS, "graphSelectionBridge") {
		t.Errorf("D37p-selection-1: Authority module must reference graphSelectionBridge (migrated by D37p-authority-1-impl)")
	}
	if !strings.Contains(authorityJS, "registerLens('authority'") {
		t.Errorf("D37p-selection-1: Authority module must register a delegate via graphSelectionBridge.registerLens('authority', ...) (migrated by D37p-authority-1-impl)")
	}
}

// ── K. Index.html line count ───────────────────────────────────────

func TestExplorer_D37pSelection1_IndexHtmlWithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 7820 {
		t.Errorf("D37p-selection-1: index.html line count %d exceeds the existing 7820 ceiling", lines)
	}
}

// ── L. Sanity — bridge sibling assets still served ─────────────────

func TestExplorer_D37pSelection1_PlatformAssetsServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, asset := range []string{
		d37pSel1SharedAsset,
		d37pSel1CameraBusAsset,
		d37pSel1StageAsset,
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37p-selection-1: platform asset %q must remain served", asset)
		}
	}
}
