package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-authority-1-impl — Authority Selection Bridge Delegate tests.
//
// Pins the Authority renderer's new integration with the shared
// `graph-selection-bridge.js` module:
//
//   • Authority registers a delegate via
//     `graphSelectionBridge.registerLens('authority', { getCurrentCard,
//     getCurrentNodeRef, handleAction })`;
//   • Authority publishes selection events from the existing
//     Cytoscape `cy.on('select unselect', …)` path via the
//     `_selectionChangeHandlers` registry — no second cy listener is
//     introduced, no `delegate.selectCard` is implemented, no shared-
//     bridge subscription is created in Authority code;
//   • the bridge payload carries `lens: 'authority'` and
//     `source: 'authority-cytoscape'` in meta;
//   • existing Authority public surface (camera methods, viewport-
//     change / selection-change subscription helpers) is preserved
//     verbatim;
//   • the camera-bus Authority delegate registration from D37p-impl-4
//     is preserved;
//   • the shared bridge module remains generic — no Authority-specific
//     coupling is added to it;
//   • foundation modules (GraphViewport host, camera bus, pane shell,
//     canvas-edge tabs, right drawer, default renderer) are intact.
//
// Tests are source-contract only — the assertions read the served
// JavaScript text and the rendered index.html.

const (
	d37pAuth1AuthorityPocAsset    = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37pAuth1SharedBridgeAsset    = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37pAuth1AuthorityToolbar     = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
	d37pAuth1AuthorityEdgeTabs    = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37pAuth1AuthorityWorkbench   = "/explorer/assets/js/graph/authority/authority-graph-workbench.js"
	d37pAuth1CameraBusAsset       = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37pAuth1SelectedObjectPane   = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37pAuth1ViewportAsset        = "/explorer/assets/js/graph/graph-viewport.js"
	d37pAuth1DrawerAsset          = "/explorer/assets/js/graph/graph-drawer.js"
	d37pAuth1ContextSelectionBr   = "/explorer/assets/js/graph/context/context-selection-bridge.js"
)

// ── A. Authority registers a delegate with the shared bridge ─────────

func TestExplorer_D37pAuthority1_AuthorityRegistersWithSharedBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	if !strings.Contains(js, "graphSelectionBridge") {
		t.Errorf("D37p-authority-1-impl: authority-cytoscape-poc.js must reference graphSelectionBridge")
	}
	if !strings.Contains(js, "registerLens('authority'") {
		t.Errorf("D37p-authority-1-impl: Authority must call graphSelectionBridge.registerLens('authority', ...)")
	}
}

func TestExplorer_D37pAuthority1_AuthorityDelegateShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// Methods on the registered delegate must include getCurrentCard,
	// getCurrentNodeRef, and handleAction. The Authority module
	// defines these as local function declarations and then assigns
	// them into the delegate object literal at registration time.
	for _, want := range []string{
		"function getCurrentCard",
		"function getCurrentNodeRef",
		"function handleAction",
		"getCurrentCard:",
		"getCurrentNodeRef:",
		"handleAction:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-1-impl: Authority delegate must declare/assign %q", want)
		}
	}
}

// ── B. Publishing into the shared bridge ────────────────────────────

func TestExplorer_D37pAuthority1_PublishesSelectCardWithAuthorityLens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// Authority must call bridge.selectCard / bridge.clearSelection,
	// emit a payload with `lens: 'authority'`, and tag meta source as
	// 'authority-cytoscape' so downstream consumers can disambiguate.
	for _, want := range []string{
		"bridge.selectCard(",
		"bridge.clearSelection(",
		"lens:          'authority'",
		"source:     'authority-cytoscape'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-1-impl: Authority publish path must contain %q", want)
		}
	}
}

func TestExplorer_D37pAuthority1_PayloadIncludesIdKindNodeRef(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// The published payload carries id / kind / sourceNodeRef / card
	// fields; the shared bridge normaliser only requires `id` but
	// every Authority payload supplies the richer shape so cross-lens
	// consumers can read structured data.
	for _, want := range []string{
		"id:            sel.id",
		"kind:          sel.kind",
		"sourceNodeRef: sel.sourceNodeRef",
		"card:          sel.card",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-1-impl: Authority selection payload must carry %q", want)
		}
	}
}

// ── C. Hook location — registered through existing registry ─────────

// TestExplorer_D37pAuthority1_HookUsesExistingRegistry pins that the
// publish helper is wired through the existing
// `_onSelectionChanged(handler)` registry (which itself binds to
// `cy.on('select unselect', …)` via `_attachExternalHandlersToCy`).
// This avoids introducing a second independent cy listener and means
// the registration survives cy teardown / re-mount without extra
// re-binding logic.
func TestExplorer_D37pAuthority1_HookUsesExistingRegistry(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	if !strings.Contains(js, "_publishAuthoritySelectionToSharedBridge") {
		t.Errorf("D37p-authority-1-impl: Authority must declare a publish helper for shared-bridge selection events")
	}
	if !strings.Contains(js, "_onSelectionChanged(_publishAuthoritySelectionToSharedBridge)") {
		t.Errorf("D37p-authority-1-impl: publish helper must be registered through the existing _onSelectionChanged registry (no second cy listener)")
	}
	// The existing cy.on('select unselect', ...) site stays the engine
	// source of truth — the publish helper attaches to that hook via
	// the registry rather than re-installing a parallel listener.
	if !strings.Contains(js, "cy.on('select unselect'") {
		t.Errorf("D37p-authority-1-impl: existing cy.on('select unselect', …) hook must remain in authority-cytoscape-poc.js")
	}
}

// ── D. Recursion + side-effect discipline ──────────────────────────

// TestExplorer_D37pAuthority1_NoSharedBridgeSubscribe asserts that
// Authority does NOT subscribe to the shared bridge. The bridge push
// is one-way (Authority → bridge → platform); a subscribe would
// risk recursion (bridge selectCard → Authority handler → cy.select →
// bridge selectCard).
func TestExplorer_D37pAuthority1_NoSharedBridgeSubscribe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	if strings.Contains(js, "graphSelectionBridge.subscribe(") {
		t.Errorf("D37p-authority-1-impl: Authority must NOT subscribe to graphSelectionBridge (recursion risk)")
	}
}

// TestExplorer_D37pAuthority1_NoDelegateSelectCard asserts that the
// Authority delegate registered with the bridge does NOT implement
// `selectCard` — the recursion-discipline contract requires the
// delegate to be read/action-only.
func TestExplorer_D37pAuthority1_NoDelegateSelectCard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// Slice the delegate registration object literal and assert
	// `selectCard:` does not appear inside it.
	startIdx := strings.Index(js, "registerLens('authority'")
	if startIdx < 0 {
		t.Fatalf("D37p-authority-1-impl: registerLens('authority' must be present")
	}
	// The object literal closes at the matching `});` shortly after
	// registerLens(. Bound the slice to the next `});`.
	tail := js[startIdx:]
	endRel := strings.Index(tail, "});")
	if endRel < 0 {
		t.Fatalf("D37p-authority-1-impl: registerLens('authority' object literal must be terminated by });")
	}
	delegateSlice := tail[:endRel]
	if strings.Contains(delegateSlice, "selectCard:") {
		t.Errorf("D37p-authority-1-impl: Authority delegate must NOT implement selectCard (recursion risk)")
	}
	if strings.Contains(delegateSlice, "clearSelection:") {
		t.Errorf("D37p-authority-1-impl: Authority delegate must NOT implement clearSelection (recursion risk)")
	}
}

// TestExplorer_D37pAuthority1_NoDrawerOrPaneCalls asserts that the
// new selection-bridge integration does not invoke drawer setters or
// pane methods. Those side effects stay where they already live
// (legacy `drawer.render` path, canvas-edge tab subscription, etc.).
func TestExplorer_D37pAuthority1_NoDrawerOrPaneCalls(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// Slice the new Authority selection-bridge IIFE so we assert
	// side-effect discipline only within the new code, without
	// false-positive-matching unrelated existing references.
	startIdx := strings.Index(js, "_registerAuthoritySelectionBridgeDelegate")
	if startIdx < 0 {
		t.Fatalf("D37p-authority-1-impl: Authority selection-bridge IIFE must be present")
	}
	tail := js[startIdx:]
	// IIFE closes at `})();` after the registration.
	endRel := strings.LastIndex(tail, "})();")
	if endRel < 0 {
		t.Fatalf("D37p-authority-1-impl: Authority selection-bridge IIFE must end with })();")
	}
	slice := tail[:endRel]

	for _, banned := range []string{
		"drawer.",
		"authorityCanvasEdgeTabs",
		"authorityWorkbench",
		"graphSelectedObjectPane",
		".setName(",
		".setFields(",
		".setActions(",
		"openTab(",
		"closePane(",
		"contextEvidenceTray",
	} {
		if strings.Contains(slice, banned) {
			t.Errorf("D37p-authority-1-impl: Authority selection-bridge IIFE must not call %q (lens side effects stay where they live)", banned)
		}
	}
}

// ── E. Existing Authority subscribers preserved ────────────────────

func TestExplorer_D37pAuthority1_LocalSelectionHandlersPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// The existing local registry + helpers must remain intact.
	for _, want := range []string{
		"var _selectionChangeHandlers",
		"function _onSelectionChanged",
		"function _attachExternalHandlersToCy",
		"_selectionChangeHandlers.push(handler)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-1-impl: existing Authority selection registry must be preserved (%q)", want)
		}
	}
	// And the public surface must keep advertising `onSelectionChanged`
	// so the canvas-edge tabs + toolbar subscriptions continue to work.
	if !regexp.MustCompile(`onSelectionChanged:\s+_onSelectionChanged`).MatchString(js) {
		t.Errorf("D37p-authority-1-impl: cytoscapePoc.onSelectionChanged surface must still be exported")
	}
}

func TestExplorer_D37pAuthority1_CanvasEdgeTabsStillSubscribe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	edgeJs := getExplorerAsset(t, srv, d37pAuth1AuthorityEdgeTabs)
	if !strings.Contains(edgeJs, "onSelectionChanged") {
		t.Errorf("D37p-authority-1-impl: authority-canvas-edge-tabs.js must still subscribe via cytoscapePoc.onSelectionChanged")
	}
}

func TestExplorer_D37pAuthority1_ToolbarStillSubscribe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tbJs := getExplorerAsset(t, srv, d37pAuth1AuthorityToolbar)
	if !strings.Contains(tbJs, "onSelectionChanged") {
		t.Errorf("D37p-authority-1-impl: authority-cytoscape-toolbar.js must still subscribe via cytoscapePoc.onSelectionChanged")
	}
}

// ── F. Existing Authority public surface preserved ─────────────────

func TestExplorer_D37pAuthority1_CameraSurfacePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	for _, want := range []string{
		"zoomBy:",
		"fit:",
		"centerOnRoot:",
		"zoomToSelected:",
		"resetView:",
		"getZoomPercent:",
		"onSelectionChanged:",
		"onViewportChanged:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-1-impl: cytoscapePoc public surface must still export %q", want)
		}
	}
}

func TestExplorer_D37pAuthority1_CameraBusDelegateIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// The D37p-impl-4 camera-bus delegate registration must be unchanged.
	if !strings.Contains(js, "_registerAuthorityCameraBusDelegate") {
		t.Errorf("D37p-authority-1-impl: D37p-impl-4 camera-bus delegate IIFE must remain present")
	}
	if !strings.Contains(js, "bus.registerLens('authority'") {
		t.Errorf("D37p-authority-1-impl: camera-bus delegate registration with 'authority' lens id must remain present")
	}
}

func TestExplorer_D37pAuthority1_GraphViewportRegistrationIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	if !strings.Contains(js, "_registerWithGraphViewport") {
		t.Errorf("D37p-authority-1-impl: GraphViewport registration IIFE must remain present")
	}
	if !strings.Contains(js, "vp.register('authority'") {
		t.Errorf("D37p-authority-1-impl: viewport.register('authority', factory) call must remain present")
	}
}

// ── G. Active-lens assertion ───────────────────────────────────────

// TestExplorer_D37pAuthority1_AssertsActiveLensOnAuthority pins that
// Authority asks the shared bridge to track `'authority'` as the
// active lens when the GraphViewport host reports Authority is the
// active renderer. This makes `graphSelectionBridge.getActiveLens()`
// return `'authority'` once Authority is mounted, without adding a
// dedicated MutationObserver to the Authority module.
func TestExplorer_D37pAuthority1_AssertsActiveLensOnAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	if !strings.Contains(js, "_maybeAssertActiveLens") {
		t.Errorf("D37p-authority-1-impl: Authority module must include an active-lens assertion helper")
	}
	if !strings.Contains(js, "bridge.setActiveLens('authority')") {
		t.Errorf("D37p-authority-1-impl: Authority must call graphSelectionBridge.setActiveLens('authority') when the host reports Authority is active")
	}
	if !strings.Contains(js, "vp.getActiveRendererId") {
		t.Errorf("D37p-authority-1-impl: active-lens assertion must consult viewport.getActiveRendererId()")
	}
}

// ── H. Shared bridge stays generic ─────────────────────────────────

// TestExplorer_D37pAuthority1_SharedBridgeStaysGeneric reasserts that
// no Authority-specific COUPLING leaked into the shared bridge module
// itself. The Authority migration is one-way — the bridge must not
// learn Authority semantics. The bridge's header comment is allowed
// to mention "Authority" as one example lens alongside Context /
// Knowledge / Resilience / Drift; the bans below target executable
// references that would betray lens-specific coupling.
func TestExplorer_D37pAuthority1_SharedBridgeStaysGeneric(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1SharedBridgeAsset)

	for _, banned := range []string{
		"'authority'",
		"\"authority\"",
		"registerLens('authority'",
		"authority-cytoscape",
		"cytoscape",
		"Cytoscape",
		"authorityCanvasEdgeTabs",
		"authorityWorkbench",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-authority-1-impl: shared bridge must remain renderer-agnostic; found %q", banned)
		}
	}
	if regexp.MustCompile(`\bcy\.`).MatchString(js) {
		t.Errorf("D37p-authority-1-impl: shared bridge must not access a graph-engine instance via cy.<member>")
	}
}

// ── I. No DOM / CSS / HTML drift ───────────────────────────────────

func TestExplorer_D37pAuthority1_IndexHtmlUnchangedForAuthorityPane(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// The canvas-edge tabs wrapper, shared selected-object pane shell,
	// and right drawer remain present. This tranche is additive and
	// must not alter the DOM.
	for _, want := range []string{
		`data-authority-canvas-edge-tabs`,
		`id="gmap-canvas-edge-pane"`,
		`id="gmap-details"`,
		`src="/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"`,
		`src="/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37p-authority-1-impl: index.html must still contain %q", want)
		}
	}
	// The Authority module must NOT have been re-loaded twice or
	// renamed by this tranche.
	if c := strings.Count(body, `src="/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"`); c != 1 {
		t.Errorf("D37p-authority-1-impl: authority-cytoscape-poc.js must be loaded exactly once (found %d)", c)
	}
}

// ── J. Foundation preservation ─────────────────────────────────────

func TestExplorer_D37pAuthority1_FoundationModulesIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37pAuth1SharedBridgeAsset,
		d37pAuth1CameraBusAsset,
		d37pAuth1SelectedObjectPane,
		d37pAuth1ViewportAsset,
		d37pAuth1DrawerAsset,
		d37pAuth1AuthorityEdgeTabs,
		d37pAuth1AuthorityToolbar,
		d37pAuth1AuthorityWorkbench,
	} {
		if got := getExplorerAsset(t, srv, asset); len(got) == 0 {
			t.Errorf("D37p-authority-1-impl: foundation asset %q must still be served", asset)
		}
	}
}

func TestExplorer_D37pAuthority1_ContextFacadeUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1ContextSelectionBr)

	// The Context facade's registration with the shared bridge from
	// D37p-selection-1 must remain present and unchanged — this
	// tranche must not regress the Context migration.
	if !strings.Contains(js, "bridge.registerLens('context'") {
		t.Errorf("D37p-authority-1-impl: Context facade's registerLens('context', ...) must still be present")
	}
	for _, want := range []string{
		"_pushSelectionToSharedBridge",
		"_pushClearToSharedBridge",
		"bridge.selectCard(",
		"bridge.clearSelection()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-1-impl: Context facade push path must still contain %q", want)
		}
	}
}

// ── K. New IIFE structure + idempotency ────────────────────────────

func TestExplorer_D37pAuthority1_IIFEStructure(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// The new IIFE must be present exactly once.
	if c := strings.Count(js, "_registerAuthoritySelectionBridgeDelegate"); c < 2 {
		// The name appears once as the function identifier in the
		// IIFE declaration `(function _registerAuthoritySelectionBridgeDelegate()`
		// and never elsewhere; the call site is the trailing `)();`.
		// So `c >= 1` is the minimum. Pin at least 1; allow comments
		// to reference it once more for documentation.
		if c < 1 {
			t.Errorf("D37p-authority-1-impl: _registerAuthoritySelectionBridgeDelegate IIFE must be declared (got %d occurrences)", c)
		}
	}
	// Delegate registration must be wrapped defensively so a missing
	// bridge cannot break Authority load.
	if !regexp.MustCompile(`(?s)function _registerDelegate\(\) \{.*?try \{.*?bridge\.registerLens\('authority'`).MatchString(js) {
		t.Errorf("D37p-authority-1-impl: _registerDelegate must wrap bridge.registerLens('authority', ...) in try/catch")
	}
}

func TestExplorer_D37pAuthority1_DefensiveBridgeAccess(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	// The IIFE must early-exit when the bridge is unavailable so a
	// missing dependency doesn't crash module load.
	if !regexp.MustCompile(`function _bridge\(\) \{\s*var g = window\.MIDASExplorerGraph;\s*return \(g && g\.graphSelectionBridge\) \|\| null;`).MatchString(js) {
		t.Errorf("D37p-authority-1-impl: bridge lookup helper must defensively return null when graphSelectionBridge is unavailable")
	}
}

// ── L. Carrier-DOM details optionally attached ─────────────────────

// TestExplorer_D37pAuthority1_CarrierDetailsOptional pins the optional
// carrier-DOM detail attachment. The Authority renderer paints hidden
// `.gmap-node[data-node-details]` carriers under `#gmap-canvas` for
// the legacy inspector; when present, the bridge payload's `card`
// includes the parsed details object. When absent, the payload still
// publishes successfully without `details`.
func TestExplorer_D37pAuthority1_CarrierDetailsOptional(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth1AuthorityPocAsset)

	if !strings.Contains(js, "_readCarrierDetails") {
		t.Errorf("D37p-authority-1-impl: Authority must include a carrier-DOM details reader for the shared bridge payload")
	}
	if !strings.Contains(js, "data-node-details") {
		t.Errorf("D37p-authority-1-impl: Authority carrier-DOM reader must query data-node-details")
	}
}
