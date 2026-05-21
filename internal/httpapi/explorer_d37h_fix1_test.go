package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37h-fix-1 — Authority Cytoscape Toolbar Runtime Wiring Fix
//
// Pins the migration of the toolbar bridge's `_pocActive()` gate and
// the inline List Mode gate in index.html from the **retired**
// `body.cytoscape-poc-active` body class to the GraphViewport host's
// renderer-identity signal:
//
//   • Preferred: `viewport.getActiveRendererId() === 'authority'`
//   • DOM fallback: `.midas-graph-viewport[data-active-renderer="authority"]`
//
// The D37h-audit traced every camera-toolbar click handler bailing
// on the stale gate. This file pins:
//
//   (1) `_pocActive()` reads the current renderer-identity signal.
//   (2) The retired body class never appears as an executable
//       condition in the toolbar bridge.
//   (3) The inline `pocActive` check at the Form/Records → List
//       Mode branch in index.html uses the same signal.
//   (4) A viewport-attribute MutationObserver re-runs
//       `_ensureSubscriptions` (and a refit) when the host swaps the
//       active renderer.
//   (5) The new gate must support the public-API path AND the DOM
//       fallback so the toolbar still works in early-load scenarios.
//
// All other D37h contracts continue to pass (the gate is the only
// behavioural change in this fix tranche).

const (
	d37hFix1ToolbarAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
)

// readD37hFix1PocActiveBody bounds assertions to the `_pocActive`
// function body so stray references elsewhere don't false-match.
func readD37hFix1PocActiveBody(t *testing.T, js string) string {
	t.Helper()
	start := strings.Index(js, "function _pocActive()")
	if start < 0 {
		t.Fatal("D37h-fix-1: _pocActive definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37h-fix-1: cannot bound _pocActive body")
	}
	return js[start : start+end]
}

// TestExplorer_D37hFix1_PocActiveUsesViewportApi pins the preferred
// path: `_pocActive()` calls
// `window.MIDASExplorerGraph.viewport.getActiveRendererId()` and
// compares to the string `'authority'`.
func TestExplorer_D37hFix1_PocActiveUsesViewportApi(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hFix1ToolbarAsset)
	body := readD37hFix1PocActiveBody(t, js)

	for _, want := range []string{
		"window.MIDASExplorerGraph",
		"graph.viewport",
		"getActiveRendererId",
		"=== 'authority'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37h-fix-1: _pocActive() must use GraphViewport host signal — missing %q\nbody:\n%s", want, body)
		}
	}
}

// TestExplorer_D37hFix1_PocActiveHasDomFallback pins the DOM-probe
// fallback: when the public API is unavailable, the gate reads
// `.midas-graph-viewport[data-active-renderer="authority"]`.
func TestExplorer_D37hFix1_PocActiveHasDomFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hFix1ToolbarAsset)
	body := readD37hFix1PocActiveBody(t, js)

	if !strings.Contains(body, `document.querySelector('.midas-graph-viewport[data-active-renderer="authority"]')`) {
		t.Errorf("D37h-fix-1: _pocActive() must carry a DOM-probe fallback querying `.midas-graph-viewport[data-active-renderer=\"authority\"]`\nbody:\n%s", body)
	}
}

// TestExplorer_D37hFix1_PocActiveDoesNotReadRetiredBodyClass is the
// negative pin against re-introducing the D35f-retired body class.
// The `_pocActive()` body must not check `'cytoscape-poc-active'`.
func TestExplorer_D37hFix1_PocActiveDoesNotReadRetiredBodyClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hFix1ToolbarAsset)
	body := readD37hFix1PocActiveBody(t, js)

	if strings.Contains(body, "cytoscape-poc-active") {
		t.Errorf("D37h-fix-1: _pocActive() must NOT reference the retired `cytoscape-poc-active` body class\nbody:\n%s", body)
	}
}

// TestExplorer_D37hFix1_ToolbarExecutableJsHasNoRetiredBodyClassCheck
// is a stronger negative pin: NO executable JS in the toolbar bridge
// should test for `body.cytoscape-poc-active`. Documentation
// comments are allowed (the retirement story lives in code review),
// so we strip JS comments first.
func TestExplorer_D37hFix1_ToolbarExecutableJsHasNoRetiredBodyClassCheck(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hFix1ToolbarAsset)
	exec := stripJSComments(js)

	for _, banned := range []string{
		`classList.contains('cytoscape-poc-active')`,
		`classList.contains("cytoscape-poc-active")`,
		`body.cytoscape-poc-active`, // any executable string reference is also a red flag
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D37h-fix-1: toolbar executable JS must not gate on retired body class — found %q", banned)
		}
	}
}

// TestExplorer_D37hFix1_InlineListModeGateMigrated pins that the
// inline Form/Records → List Mode branch in index.html no longer
// gates on the retired body class and instead uses the host signal.
func TestExplorer_D37hFix1_InlineListModeGateMigrated(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Locate the List Mode branch via its stable comment so the
	// assertion is bounded.
	branchStart := strings.Index(body, `D33x-list-mode — When PoC is active`)
	if branchStart < 0 {
		t.Fatal("D37h-fix-1: D33x-list-mode branch comment not found in index.html")
	}
	branchEnd := strings.Index(body[branchStart:], "_showAuthorityPanels(false)")
	if branchEnd < 0 {
		t.Fatal("D37h-fix-1: cannot bound D33x-list-mode branch")
	}
	branch := body[branchStart : branchStart+branchEnd]

	// Positive pins: branch uses the host signal.
	for _, want := range []string{
		"getActiveRendererId",
		"'authority'",
		`data-active-renderer="authority"`,
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D37h-fix-1: inline List Mode gate must use the host renderer-identity signal — missing %q", want)
		}
	}

	// Negative pin: the retired body class must not be used as the
	// gate inside this branch.
	if strings.Contains(branch, `classList.contains('cytoscape-poc-active')`) ||
		strings.Contains(branch, `classList.contains("cytoscape-poc-active")`) {
		t.Errorf("D37h-fix-1: inline List Mode gate must NOT check the retired `cytoscape-poc-active` body class")
	}
}

// TestExplorer_D37hFix1_ViewportRendererObserverInstalled pins the
// new MutationObserver on `.midas-graph-viewport[data-active-renderer]`
// so the toolbar re-runs subscriptions and refit when the host
// swaps the active renderer. Without this observer the stale-gate
// regression that prompted D37h-fix-1 could come back via a
// subscription-not-installed pathway.
func TestExplorer_D37hFix1_ViewportRendererObserverInstalled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hFix1ToolbarAsset)

	for _, want := range []string{
		"function _bindViewportRendererObserver()",
		`document.querySelector('.midas-graph-viewport')`,
		`attributeFilter: ['data-active-renderer']`,
		"_ensureSubscriptions()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37h-fix-1: viewport-attribute observer must include %q", want)
		}
	}

	// And it must be called from wire().
	wireStart := strings.Index(js, "function wire()")
	if wireStart < 0 {
		t.Fatal("D37h-fix-1: wire() definition not found")
	}
	wireEnd := strings.Index(js[wireStart:], "\n  }\n")
	if wireEnd < 0 {
		t.Fatal("D37h-fix-1: cannot bound wire() body")
	}
	wireBody := js[wireStart : wireStart+wireEnd]
	if !strings.Contains(wireBody, "_bindViewportRendererObserver()") {
		t.Errorf("D37h-fix-1: wire() must call _bindViewportRendererObserver — wire() body:\n%s", wireBody)
	}
}

// TestExplorer_D37hFix1_ViewportObserverIsIdempotent pins the
// idempotence guard: a module-scope flag prevents double-install.
func TestExplorer_D37hFix1_ViewportObserverIsIdempotent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hFix1ToolbarAsset)

	// Bound the observer body so the idempotence pin doesn't
	// false-match unrelated `if (...)` returns elsewhere.
	start := strings.Index(js, "function _bindViewportRendererObserver()")
	if start < 0 {
		t.Fatal("D37h-fix-1: observer function not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37h-fix-1: cannot bound observer body")
	}
	body := js[start : start+end]

	if !strings.Contains(body, "if (_viewportObserver) return;") {
		t.Errorf("D37h-fix-1: observer must be idempotent (guard via `if (_viewportObserver) return;`) — body:\n%s", body)
	}
}

// TestExplorer_D37hFix1_PreservesD37hToolbarPublicSurface confirms
// the D37h public surface is unchanged; the fix is a gate migration
// only.
func TestExplorer_D37hFix1_PreservesD37hToolbarPublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hFix1ToolbarAsset)

	for _, want := range []string{
		"wire:",
		"refit:",
		"isWired:",
		"renderZoomPercent:",
		"syncZoomSelectedEnabled:",
		"ensureSubscriptions:",
		// Camera handlers route through PoC. (D37p-impl-4 retired the
		// capture-phase camera button bindings; the helper functions
		// remain as the Authority delegate's call target via
		// cytoscapePoc.*, so the `poc.zoomToSelected()` /
		// `poc.resetView()` calls still appear in source.)
		"poc.zoomToSelected()",
		"poc.resetView()",
		// Zoom-selected button id still referenced via
		// `_zoomSelectedBtnEl()` for the disabled-state mirror.
		"'gmap-zoom-selected-button'",
		"poc.getZoomPercent",
		"poc.onViewportChanged",
		"poc.onSelectionChanged",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37h-fix-1: D37h public surface symbol %q must remain", want)
		}
	}
}

// TestExplorer_D37hFix1_NoD37iFeaturesIntroduced is the scope-creep
// guard: this tranche must NOT bring in D37i selection mode,
// dependency view, context menus, or filters.
func TestExplorer_D37hFix1_NoD37iFeaturesIntroduced(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hFix1ToolbarAsset)
	exec := stripJSComments(js)

	for _, banned := range []string{
		"boxSelectionEnabled(true",
		"selectionType('additive')",
		// Dependency-view APIs would only appear in a later tranche.
		".predecessors(",
		".successors(",
		// Context menu (cxttap binding from the toolbar) is D37k.
		"on('cxttap'",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D37h-fix-1: scope creep — D37i/D37j/D37k feature %q must not appear in this fix tranche", banned)
		}
	}
}
