package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37as-strategic-node-pane-contract-impl — Strategic Node Pane Contract tests.
//
// D37ar's read-only platform assessment concluded:
//   • The shared selected-object pane shell (graph-selected-object-pane.js)
//     already exists and Authority + Context are formal providers.
//   • Rendering today is monolithic per provider — the shell calls
//     provider.render(opts) once and the provider dispatches internally on
//     its own active-tab state. The per-section `provider` field in the tab
//     config (e.g. 'authority.details') is reserved metadata that the
//     shell does not look up.
//   • Lens registrations are scattered across 3-4 files per lens. A
//     composite helper is justified.
//
// This tranche tightens the platform contract by adding:
//
//   • Optional `provider.renderTab(ctx)` per-section dispatch in the shell,
//     with `provider.render(opts)` as the unchanged fallback.
//   • Optional `provider.getEmptyState(ctx)` accessor; shell exposes
//     `getEmptyState(lensId?)` to ask the active provider.
//   • Optional `provider.onActivate(ctx) / onDeactivate(ctx)` lifecycle
//     hooks fired on every shell-level active-tab transition.
//   • `graphLensRegistry.register({...})` composite helper for future
//     graph lenses — Authority and Context NOT migrated in this tranche.
//
// Hard invariants pinned by this file:
//
//   • The shell module remains lens-agnostic and renderer-agnostic.
//   • No Authority node kinds, Context node kinds, Cytoscape references,
//     HTML-card-overlay references, or path imports from graph/authority/
//     graph/context/ appear in the shell or the new registry module.
//   • Authority's AUTHORITY_TAB_CONFIG and provider shape are unchanged.
//   • Context's CONTEXT_TAB_CONFIG and provider shape are unchanged.
//   • The 7 locked selection-bridge event names are unchanged.

const (
	d37asShellAsset    = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37asRegistryAsset = "/explorer/assets/js/graph/graph-platform/graph-lens-registry.js"
	d37asBridgeAsset   = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37asAuthorityEdge = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37asContextPane   = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
)

// ── 1. renderTab(ctx) per-section dispatch ─────────────────────────

func TestExplorer_D37as_Shell_RenderTabDispatch_PerSection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asShellAsset)

	// The shell's render(opts) path must check for provider.renderTab
	// BEFORE falling back to the monolithic provider.render(opts).
	if !regexp.MustCompile(`(?s)function render\(opts\)[\s\S]*?provider\.renderTab[\s\S]*?_callProvider\('render'`).MatchString(js) {
		t.Errorf("D37as-strategic-node-pane-contract-impl: render(opts) must check provider.renderTab first, then fall back to _callProvider('render', [opts])")
	}
	// The ctx builder must exist.
	if !strings.Contains(js, "function _buildRenderTabContext(opts)") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: _buildRenderTabContext(opts) must be defined")
	}
	// Ctx must include every documented field.
	startIdx := strings.Index(js, "function _buildRenderTabContext(opts)")
	if startIdx < 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: _buildRenderTabContext must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37as-strategic-node-pane-contract-impl: _buildRenderTabContext body must be well-formed")
	}
	body := tail[:endIdx+1]

	for _, want := range []string{
		"lensId:      lensId,",
		"activeTabId: activeTabId,",
		"tab:         tab,",
		"selection:   selectionCtx.selection,",
		"isStale:     !!selectionCtx.isStale,",
		"provider:    provider,",
		"copy:        (provider && _isPlainObject(provider.copy)) ? provider.copy : null,",
		"mount:       null,",
		"signal:      signal,",
		"opts:        _isPlainObject(opts) ? opts : null,",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: GraphTabRenderContext must declare %q", want)
		}
	}
	// AbortController integration: previous controller aborted on
	// next render; signal exposed as ctx.signal.
	for _, want := range []string{
		"typeof AbortController === 'function'",
		"_renderTabController.abort()",
		"_renderTabController = new AbortController()",
		"signal = _renderTabController.signal",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: per-call AbortController integration must include %q", want)
		}
	}
}

// ── 2. Fallback path: provider.render(opts) preserved ──────────────

func TestExplorer_D37as_Shell_RenderFallback_PreservesExistingProviders(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asShellAsset)

	// The render(opts) fallback path must continue to invoke
	// _callProvider('render', [opts]) for providers without
	// renderTab. This is the back-compat hinge for Authority and
	// Context.
	idx := strings.Index(js, "function render(opts)")
	if idx < 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: render(opts) must exist on shell")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n  function ")
	if end < 0 {
		t.Fatalf("D37as-strategic-node-pane-contract-impl: render(opts) body must be well-formed")
	}
	body := tail[:end+1]

	for _, want := range []string{
		"typeof provider.renderTab === 'function'",
		"} else {",
		"_callProvider('render', [opts]);",
		"_notify({ type: 'pane_rendered', lens: _activeLensId });",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: render(opts) must contain %q", want)
		}
	}

	// Authority's provider does NOT declare renderTab in this tranche
	// — pin that the fallback path still applies to Authority.
	authJs := getExplorerAsset(t, srv, d37asAuthorityEdge)
	authProviderStart := strings.Index(authJs, "var _paneProvider = {")
	authProviderEnd   := strings.Index(authJs[authProviderStart:], "function _registerWithSharedPaneShell")
	if authProviderStart < 0 || authProviderEnd < 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: Authority _paneProvider block must be locatable")
	}
	authProvider := authJs[authProviderStart : authProviderStart+authProviderEnd]
	if strings.Contains(authProvider, "renderTab:") || strings.Contains(authProvider, "renderTab :") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: Authority provider must NOT declare renderTab in this tranche (back-compat fallback in use)")
	}
	if !strings.Contains(authProvider, "render: function (opts)") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: Authority provider must still declare the monolithic render(opts) fallback")
	}
}

// ── 3. getEmptyState contract is optional ──────────────────────────

func TestExplorer_D37as_Shell_EmptyStateContract_Optional(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asShellAsset)

	// Public function defined.
	if !strings.Contains(js, "function getEmptyState(lensId)") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: shell must define getEmptyState(lensId)")
	}
	// Exposed on public surface.
	if !strings.Contains(js, "getEmptyState:          getEmptyState,") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: shell public surface must export getEmptyState")
	}
	// Returns null when provider does not implement the hook —
	// optional contract.
	idx := strings.Index(js, "function getEmptyState(lensId)")
	if idx < 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: getEmptyState must exist")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n  function ")
	if end < 0 {
		t.Fatalf("D37as-strategic-node-pane-contract-impl: getEmptyState body must be well-formed")
	}
	body := tail[:end+1]

	for _, want := range []string{
		"if (!provider || typeof provider.getEmptyState !== 'function') return null;",
		"var ctx = _buildRenderTabContext(null);",
		"var empty = provider.getEmptyState(ctx);",
		"return _isPlainObject(empty) ? empty : null;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: getEmptyState body must contain %q", want)
		}
	}
}

// ── 4. Optional lifecycle hooks ────────────────────────────────────

func TestExplorer_D37as_Shell_OptionalLifecycleHooks(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asShellAsset)

	// Hook dispatcher function exists.
	if !strings.Contains(js, "function _fireTabLifecycleHooks(lensId, prevTabId, newTabId)") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: shell must define _fireTabLifecycleHooks(lensId, prevTabId, newTabId)")
	}
	// Both setActiveTab AND setActiveLens fire hooks on transitions.
	if !regexp.MustCompile(`(?s)function setActiveTab\([^)]*\)[\s\S]*?_fireTabLifecycleHooks\(target, prev, next\)`).MatchString(js) {
		t.Errorf("D37as-strategic-node-pane-contract-impl: setActiveTab must call _fireTabLifecycleHooks(target, prev, next)")
	}
	if !regexp.MustCompile(`(?s)function setActiveLens\([^)]*\)[\s\S]*?_fireTabLifecycleHooks\(lensId, prevTabForLens, nextDefault\)`).MatchString(js) {
		t.Errorf("D37as-strategic-node-pane-contract-impl: setActiveLens must call _fireTabLifecycleHooks(lensId, prevTabForLens, nextDefault)")
	}
	// Both hooks are optional + wrapped in try/catch.
	idx := strings.Index(js, "function _fireTabLifecycleHooks(lensId, prevTabId, newTabId)")
	if idx < 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: _fireTabLifecycleHooks must exist")
	}
	tail := js[idx:]
	// Hook body extends to end of file (last function before the
	// export block). Use the next IIFE/export boundary marker.
	endMarker := "// ── Public API: subscription"
	if !strings.Contains(tail, endMarker) {
		t.Fatal("D37as-strategic-node-pane-contract-impl: hook block must precede the subscription section")
	}
	end := strings.Index(tail, endMarker)
	body := tail[:end]

	for _, want := range []string{
		"if (typeof provider.onDeactivate === 'function' && prevTabId)",
		"deactivateCtx.activeTabId = prevTabId;",
		"provider.onDeactivate(deactivateCtx);",
		"if (typeof provider.onActivate === 'function' && newTabId)",
		"activateCtx.activeTabId = newTabId;",
		"provider.onActivate(activateCtx);",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: lifecycle hook body must contain %q", want)
		}
	}
	// Defensive: hook errors must not block the transition.
	if strings.Count(body, "try {") < 2 || strings.Count(body, "} catch (_) {") < 2 {
		t.Errorf("D37as-strategic-node-pane-contract-impl: both onDeactivate and onActivate calls must be wrapped in try/catch")
	}
}

// ── 5. graphLensRegistry composite helper ──────────────────────────

func TestExplorer_D37as_LensRegistry_CompositeHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asRegistryAsset)

	if len(js) == 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: graph-lens-registry.js must be served")
	}
	// Public export.
	if !strings.Contains(js, "window.MIDASExplorerGraph.graphLensRegistry = {") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: graphLensRegistry must be exported on MIDASExplorerGraph")
	}
	for _, want := range []string{
		"register:   register,",
		"unregister: unregister,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: graphLensRegistry must declare %q", want)
		}
	}
	// register(spec) must accept the documented optional fields and
	// short-circuit when a surface is missing.
	for _, want := range []string{
		"function register(spec)",
		"var lensId     = (typeof spec.lensId === 'string' && spec.lensId.length > 0) ? spec.lensId : null;",
		"if (!lensId) return null;",
		"if (_isPlainObject(spec.viewport))",
		"vp.register(rendererId, factory)",
		"if (spec.selection !== undefined && spec.selection !== null)",
		"br.registerLens(lensId, spec.selection)",
		"if (spec.camera !== undefined && spec.camera !== null)",
		"bus.registerLens(lensId, spec.camera)",
		"if (spec.pane !== undefined && spec.pane !== null)",
		"pn.registerLensProvider(lensId, spec.pane)",
		"if (spec.drawer !== undefined && spec.drawer !== null)",
		"dr.registerLens(lensId, spec.drawer)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: registry register(spec) must contain %q", want)
		}
	}
	// Script tag wired in index.html.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `src="/explorer/assets/js/graph/graph-platform/graph-lens-registry.js"`) {
		t.Errorf("D37as-strategic-node-pane-contract-impl: index.html must load graph-lens-registry.js")
	}
}

// ── 6. Shell + registry stay generic ───────────────────────────────

func TestExplorer_D37as_Shell_StaysGeneric(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{d37asShellAsset, d37asRegistryAsset} {
		js := getExplorerAsset(t, srv, asset)

		// Renderer + engine vocabulary bans.
		for _, banned := range []string{
			"cytoscape",
			"Cytoscape",
			"html-card",
			"cy.viewport(",
			"cy.fit(",
			"cy.pan(",
			"cy.zoom(",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37as-strategic-node-pane-contract-impl: %q must not contain renderer-specific token %q", asset, banned)
			}
		}

		// Authority + Context node kind vocabulary bans.
		for _, banned := range []string{
			"authority_profile",
			"authority_grant",
			"decision_surface",
			"business_service",
			"fail_mode_policy",
			"escalation_target",
			"ai_system",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37as-strategic-node-pane-contract-impl: %q must not contain lens-specific kind %q", asset, banned)
			}
		}

		// Authority + Context wrapper CSS bans.
		for _, banned := range []string{
			"gmap-canvas-edge-tabs",
			"gmap-context-selected-object-pane",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37as-strategic-node-pane-contract-impl: %q must not contain lens-wrapper class %q", asset, banned)
			}
		}

		// Path-level import bans — assert the shared modules do not
		// reference graph/authority/ or graph/context/ source paths.
		for _, banned := range []string{
			"graph/authority/",
			"graph/context/",
			"explorer/assets/js/graph/authority",
			"explorer/assets/js/graph/context",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37as-strategic-node-pane-contract-impl: %q must not import from %q (shared shell must remain lens-neutral)", asset, banned)
			}
		}
	}
}

// ── 7. Authority provider — no behavioural change ──────────────────

func TestExplorer_D37as_Authority_NoBehaviouralChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asAuthorityEdge)

	// AUTHORITY_TAB_CONFIG unchanged.
	if !strings.Contains(js, "var AUTHORITY_TAB_CONFIG = {") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: Authority AUTHORITY_TAB_CONFIG must remain")
	}
	for _, want := range []string{
		"enabled:    true,",
		"defaultTab: 'details',",
		"id:       'details',",
		"id:       'authority',",
		"id:       'evidence',",
		"tabs: AUTHORITY_TAB_CONFIG",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: Authority invariant %q must remain", want)
		}
	}
	// Provider shape unchanged — no new D37as method on the provider
	// literal (Authority NOT migrated in this tranche).
	startIdx := strings.Index(js, "var _paneProvider = {")
	endIdx := strings.Index(js[startIdx:], "function _registerWithSharedPaneShell")
	if startIdx < 0 || endIdx < 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: Authority _paneProvider must be locatable")
	}
	providerBlock := js[startIdx : startIdx+endIdx]
	for _, banned := range []string{
		"renderTab:",
		"getEmptyState:",
		"onActivate:",
		"onDeactivate:",
	} {
		if strings.Contains(providerBlock, banned) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: Authority provider must NOT declare %q in this tranche (no provider migration)", banned)
		}
	}
}

// ── 8. Context provider — no behavioural change ────────────────────

func TestExplorer_D37as_Context_NoBehaviouralChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asContextPane)

	// CONTEXT_TAB_CONFIG unchanged.
	if !strings.Contains(js, "var CONTEXT_TAB_CONFIG = {") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: Context CONTEXT_TAB_CONFIG must remain")
	}
	for _, want := range []string{
		"enabled:    true,",
		"defaultTab: 'summary',",
		"id:       'summary',",
		"id:       'details',",
		"id:       'relationships',",
		"id:       'actions',",
		"id:       'evidence',",
		"tabs: CONTEXT_TAB_CONFIG",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: Context invariant %q must remain", want)
		}
	}
	// Provider shape unchanged — no new D37as method on the provider
	// literal (Context NOT migrated in this tranche).
	startIdx := strings.Index(js, "var provider = {")
	if startIdx < 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: Context provider literal must be locatable")
	}
	endIdx := strings.Index(js[startIdx:], "shell.registerLensProvider('context'")
	if endIdx < 0 {
		t.Fatal("D37as-strategic-node-pane-contract-impl: Context provider literal must precede registerLensProvider call")
	}
	providerBlock := js[startIdx : startIdx+endIdx]
	for _, banned := range []string{
		"renderTab:",
		"getEmptyState:",
		"onActivate:",
		"onDeactivate:",
	} {
		if strings.Contains(providerBlock, banned) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: Context provider must NOT declare %q in this tranche (no provider migration)", banned)
		}
	}
}

// ── 9. Selection-bridge event vocabulary unchanged ─────────────────

func TestExplorer_D37as_SelectionBridgeEventVocabularyUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asBridgeAsset)

	// The 7 locked selection-bridge events from D37p-selection-1
	// must remain intact — no new events added, no events renamed.
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
			t.Errorf("D37as-strategic-node-pane-contract-impl: locked selection-bridge event %q must remain", want)
		}
	}
}

// ── 10. Shell new public surface ───────────────────────────────────

func TestExplorer_D37as_Shell_NewPublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asShellAsset)

	// getEmptyState exported; existing tab-contract methods still
	// present (D37ak invariants).
	for _, want := range []string{
		"getEmptyState:          getEmptyState,",
		"getTabConfig:           getTabConfig,",
		"getTabs:                getTabs,",
		"getDefaultTab:          getDefaultTab,",
		"getActiveTab:           getActiveTab,",
		"setActiveTab:           setActiveTab,",
		"tabSupportsKind:        tabSupportsKind,",
		"buildSelectionContext:  buildSelectionContext,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37as-strategic-node-pane-contract-impl: shell public surface must include %q", want)
		}
	}
}

// ── 11. Per-call AbortController state + destroy cleanup ───────────

func TestExplorer_D37as_Shell_AbortControllerLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37asShellAsset)

	// Module state slot for the controller.
	if !strings.Contains(js, "var _renderTabController     = null;") {
		t.Errorf("D37as-strategic-node-pane-contract-impl: shell must declare _renderTabController module-state slot")
	}
	// destroy() must abort the pending controller and clear it.
	if !regexp.MustCompile(`(?s)function destroy\(\)[\s\S]*?_renderTabController\.abort\(\)[\s\S]*?_renderTabController = null`).MatchString(js) {
		t.Errorf("D37as-strategic-node-pane-contract-impl: destroy() must abort + clear _renderTabController")
	}
}
