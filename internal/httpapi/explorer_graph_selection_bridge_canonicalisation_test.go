package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37q-viewport-5-impl — Graph Selection Bridge Canonicalisation tests.
//
// This file is documentation- and contract-only. It does not change
// runtime behaviour. Its job is to lock in the strategic selection
// contract for the multi-graph viewport platform:
//
//   1. `graphSelectionBridge` is the CANONICAL cross-lens selection
//      event path. Platform-level consumers and any future graph
//      integration must read selection state through the bridge.
//
//   2. `graphSelectedObjectPane` subscribes to `graphSelectionBridge`
//      and forwards normalised selection events to whichever lens
//      provider is currently active.
//
//   3. Context publishes selection into the bridge via the Context
//      selection bridge facade (`context-selection-bridge.js`).
//
//   4. Authority publishes normalised selection into the bridge from
//      `authority-cytoscape-poc.js`. The Cytoscape PoC module is the
//      approved owner of raw Cytoscape select / unselect events; it
//      translates those events into bridge-normalised payloads.
//
//   5. Direct renderer-local engine subscriptions are PERMITTED in a
//      narrow allow-list (Authority canvas-edge tabs, Authority
//      toolbar, Authority Cytoscape PoC engine owner) because they
//      need engine-coupled state before normalisation OR they ARE the
//      engine owner. They are deliberately NOT removed in this
//      tranche.
//
//   6. Platform-level modules and Context modules must NOT subscribe
//      directly to `cytoscapePoc.onSelectionChanged(...)`. Doing so
//      would bypass the canonical normalisation path.
//
//   7. Authority pane provider's `notifySelectionChanged` is a
//      deliberate no-op (see double-render avoidance note in
//      authority-canvas-edge-tabs.js). Context pane provider's
//      `notifySelectionChanged` may also be a deliberate no-op
//      because Context already subscribes to its own
//      `contextSelectionBridge`.
//
// These tests pin those contracts so future tranches cannot
// accidentally regress them.

const (
	d37qV5SelectionBridgeAsset    = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37qV5SelectedObjectPaneShell = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37qV5ContextSelectionBridge  = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37qV5ContextPaneAsset        = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37qV5ContextRendererAsset    = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37qV5AuthorityPocAsset       = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37qV5AuthorityEdgeAsset      = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37qV5AuthorityToolbarAsset   = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
	d37qV5AuthorityViewAsset      = "/explorer/assets/js/graph/authority/authority-graph-view.js"
	d37qV5AuthorityWorkbenchAsset = "/explorer/assets/js/graph/authority/authority-graph-workbench.js"
)

// authorityEngineSubscriptionAllowList is the explicit, source-contract
// allow-list for direct calls to `cytoscapePoc.onSelectionChanged(...)`.
// Every entry is a renderer-local consumer that needs engine-coupled
// state before the bridge's normalised event would fire, OR is the
// Authority engine owner itself.
var authorityEngineSubscriptionAllowList = []string{
	d37qV5AuthorityEdgeAsset,    // canvas-edge tabs need raw selection sync
	d37qV5AuthorityToolbarAsset, // toolbar button enablement reads cy.elements(':selected')
	d37qV5AuthorityPocAsset,     // engine owner; defines onSelectionChanged + publishes via bridge
}

// ── 1. Selection bridge public contract preserved ─────────────────

func TestExplorer_D37qViewport5_SelectionBridgePublicContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV5SelectionBridgeAsset)

	// Public surface assignments. The bridge exports the canonical
	// cross-lens vocabulary.
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
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-5-impl: graphSelectionBridge must keep public surface entry %q", want)
		}
	}

	// Locked event vocabulary inside the EVENTS frozen array.
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
			t.Errorf("D37q-viewport-5-impl: graphSelectionBridge EVENTS vocabulary must declare %q", want)
		}
	}
}

// ── 2. Selected-object pane subscribes to the bridge ──────────────

func TestExplorer_D37qViewport5_SelectedObjectPaneUsesSelectionBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV5SelectedObjectPaneShell)

	// The shell must look up the bridge and subscribe to it. The
	// implementation lives inside `_bindToSelectionBridge`; we pin
	// the load-bearing substrings rather than the full block.
	if !strings.Contains(js, "graphSelectionBridge") {
		t.Errorf("D37q-viewport-5-impl: graphSelectedObjectPane shell must reference graphSelectionBridge")
	}
	if !regexp.MustCompile(`bridge\.subscribe\s*\(`).MatchString(js) {
		t.Errorf("D37q-viewport-5-impl: graphSelectedObjectPane shell must call bridge.subscribe(...) for cross-lens selection forwarding")
	}
	// Forward both selection lifecycle event types to the active
	// provider's notifySelectionChanged hook.
	for _, want := range []string{
		"'selection_changed'",
		"'selection_cleared'",
		"notifySelectionChanged",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-5-impl: graphSelectedObjectPane shell must forward %q to the active provider", want)
		}
	}
}

// ── 3. Context publishes selection through the bridge ─────────────

func TestExplorer_D37qViewport5_ContextPublishesSelectionThroughBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV5ContextSelectionBridge)

	for _, want := range []string{
		"graphSelectionBridge",
		"_pushSelectionToSharedBridge",
		"_pushClearToSharedBridge",
		"bridge.selectCard(",
		"bridge.clearSelection()",
		// Context registers itself as the 'context' lens delegate so
		// the bridge can route action dispatches back to Context.
		"bridge.registerLens('context'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-5-impl: context-selection-bridge.js must publish into graphSelectionBridge (%q)", want)
		}
	}
}

// ── 4. Authority publishes selection through the bridge ───────────

func TestExplorer_D37qViewport5_AuthorityPublishesSelectionThroughBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV5AuthorityPocAsset)

	for _, want := range []string{
		"graphSelectionBridge",
		"_publishAuthoritySelectionToSharedBridge",
		"bridge.selectCard(",
		"bridge.clearSelection(",
		"bridge.registerLens('authority'",
		// The publish helper is wired into the existing
		// _selectionChangeHandlers registry so it fires on every
		// engine select / unselect event without introducing a
		// second cy listener.
		"_onSelectionChanged(_publishAuthoritySelectionToSharedBridge)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-5-impl: authority-cytoscape-poc.js must publish into graphSelectionBridge (%q)", want)
		}
	}
}

// ── 5. Direct subscriptions are confined to the Authority allow-list ─

// TestExplorer_D37qViewport5_AuthorityDirectSelectionSubscriptionsAreAllowListed
// asserts that calls to `cytoscapePoc.onSelectionChanged(...)` appear
// only in the explicit allow-list: Authority canvas-edge tabs, Authority
// toolbar, and the Authority Cytoscape PoC engine owner itself.
func TestExplorer_D37qViewport5_AuthorityDirectSelectionSubscriptionsAreAllowListed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range authorityEngineSubscriptionAllowList {
		js := getExplorerAsset(t, srv, asset)
		if !strings.Contains(js, "onSelectionChanged") {
			t.Errorf("D37q-viewport-5-impl: allow-list entry %q must still reference cytoscapePoc.onSelectionChanged (kept for engine-coupled selection state)", asset)
		}
	}
}

// ── 6. Platform modules do not bypass the bridge ──────────────────

func TestExplorer_D37qViewport5_PlatformModulesDoNotSubscribeToAuthorityEngineSelection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37qV5SelectionBridgeAsset,
		d37qV5SelectedObjectPaneShell,
		"/explorer/assets/js/graph/graph-platform/graph-stage.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-bus.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-controller.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js",
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/graph-drawer.js",
		"/explorer/assets/js/graph/graph-inspector.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		if strings.Contains(js, "cytoscapePoc.onSelectionChanged(") {
			t.Errorf("D37q-viewport-5-impl: platform module %q must not subscribe directly to cytoscapePoc.onSelectionChanged (use graphSelectionBridge.subscribe instead)", asset)
		}
		// Raw cy listeners belong to the engine owner only. Use a
		// regex so 'select unselect'-style listeners are caught even
		// if formatted differently.
		if regexp.MustCompile(`cy\.on\(\s*['"]select`).MatchString(js) {
			t.Errorf("D37q-viewport-5-impl: platform module %q must not subscribe directly to raw Cytoscape selection events", asset)
		}
	}
}

// ── 7. Context modules do not bypass the bridge ───────────────────

func TestExplorer_D37qViewport5_ContextDoesNotSubscribeToAuthorityEngineSelection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37qV5ContextSelectionBridge,
		d37qV5ContextPaneAsset,
		d37qV5ContextRendererAsset,
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-inspector.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
		"/explorer/assets/js/graph/context/context-projection-provider.js",
		"/explorer/assets/js/graph/context/context-projection-handoff.js",
		"/explorer/assets/js/graph/context/context-card-model.js",
		"/explorer/assets/js/graph/context/context-connector-model.js",
		"/explorer/assets/js/graph/context/context-connector-painter.js",
		"/explorer/assets/js/graph/context/context-html-card-painter.js",
		"/explorer/assets/js/graph/context/context-layout-model.js",
		"/explorer/assets/js/graph/context/context-evidence-tray.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		if strings.Contains(js, "cytoscapePoc.onSelectionChanged(") {
			t.Errorf("D37q-viewport-5-impl: Context module %q must not subscribe directly to cytoscapePoc.onSelectionChanged (cross-lens path is graphSelectionBridge.subscribe)", asset)
		}
		if regexp.MustCompile(`cy\.on\(\s*['"]select`).MatchString(js) {
			t.Errorf("D37q-viewport-5-impl: Context module %q must not subscribe directly to raw Cytoscape selection events", asset)
		}
	}
}

// ── 8. Authority provider notifySelectionChanged remains a no-op ──

// TestExplorer_D37qViewport5_AuthorityProviderNoDoubleRenderContractPreserved
// pins that the Authority pane provider's `notifySelectionChanged`
// callback remains a deliberate no-op. The existing
// `cytoscapePoc.onSelectionChanged(syncSelection)` subscription
// already drives every canvas-edge repaint on Cytoscape select /
// unselect events; forwarding the shell's `notifySelectionChanged`
// here as well would double-render. The hook signature exists so the
// shell can call it safely.
func TestExplorer_D37qViewport5_AuthorityProviderNoDoubleRenderContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV5AuthorityEdgeAsset)

	// Whitespace-tolerant pin on the deliberate no-op pattern.
	if !regexp.MustCompile(`notifySelectionChanged:\s*function\s*\(\s*selection,\s*event\s*\)\s*\{\s*void selection; void event;\s*\}`).MatchString(js) {
		t.Errorf("D37q-viewport-5-impl: Authority pane provider notifySelectionChanged must remain a deliberate no-op (double-render avoidance)")
	}
}

// ── 9. Context provider notifySelectionChanged remains wired ──────

// TestExplorer_D37qViewport5_ContextProviderSelectionContractPreserved
// pins that the Context pane provider still declares a
// `notifySelectionChanged` callback, and that Context owns its own
// selection subscription through `contextSelectionBridge`. The
// callback body is documented in source as intentionally a no-op
// for the same double-render-avoidance reason; we pin the structure,
// not the body, so future tranches may legitimately turn it into a
// non-no-op if needed without tripping this test.
func TestExplorer_D37qViewport5_ContextProviderSelectionContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV5ContextPaneAsset)

	if !regexp.MustCompile(`notifySelectionChanged:\s*function`).MatchString(js) {
		t.Errorf("D37q-viewport-5-impl: Context pane provider must still declare notifySelectionChanged")
	}
	// Context owns its own selection model via contextSelectionBridge;
	// the pane stays in sync through that path, not through the shell's
	// forwarding. Pin the referenced module so the contract holds.
	if !strings.Contains(js, "contextSelectionBridge") {
		t.Errorf("D37q-viewport-5-impl: Context pane provider must still consume contextSelectionBridge for its own selection updates")
	}
}

// ── 10. Lens registrations against the bridge remain intact ───────

// TestExplorer_D37qViewport5_LensRegistrationsIntact pins that both
// active lens delegates are registered against the bridge. This is
// the cross-lens contract: any consumer that resolves
// `graphSelectionBridge.getActiveLens()` / `getActiveDelegate()`
// finds the right lens.
func TestExplorer_D37qViewport5_LensRegistrationsIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	ctxBridge := getExplorerAsset(t, srv, d37qV5ContextSelectionBridge)
	if !strings.Contains(ctxBridge, "bridge.registerLens('context'") {
		t.Errorf("D37q-viewport-5-impl: Context must register a 'context' delegate on graphSelectionBridge")
	}

	authority := getExplorerAsset(t, srv, d37qV5AuthorityPocAsset)
	if !strings.Contains(authority, "bridge.registerLens('authority'") {
		t.Errorf("D37q-viewport-5-impl: Authority must register an 'authority' delegate on graphSelectionBridge")
	}
}

// ── 11. Authority workbench + view do not bypass the bridge ───────

// TestExplorer_D37qViewport5_AuthorityWorkbenchAndViewDoNotBypassBridge
// pins that the two Authority modules outside the explicit
// engine-subscription allow-list (`authority-graph-view.js`,
// `authority-graph-workbench.js`) do not subscribe directly to
// `cytoscapePoc.onSelectionChanged` either. They consume the cached
// projection + the legacy carrier-DOM path; they do not need
// engine-coupled selection events.
func TestExplorer_D37qViewport5_AuthorityWorkbenchAndViewDoNotBypassBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37qV5AuthorityViewAsset,
		d37qV5AuthorityWorkbenchAsset,
	} {
		js := getExplorerAsset(t, srv, asset)
		if strings.Contains(js, "cytoscapePoc.onSelectionChanged(") {
			t.Errorf("D37q-viewport-5-impl: Authority module %q must not subscribe directly to cytoscapePoc.onSelectionChanged (only canvas-edge tabs, toolbar, and the engine owner are allow-listed)", asset)
		}
	}
}
