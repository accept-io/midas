package httpapi

import (
	"strings"
	"testing"
)

// explorer_d33a_impl1_test.go — D33a-impl-1 Cytoscape PoC Cleanup and
// Chrome Containment. Tier-1 source-string pins for:
//
//   • _renderPayload clears any prior unavailable / loading overlay
//     before initialising Cytoscape (no more stale "Loading…" tail)
//   • Cytoscape fit padding reads MIDAS's --gmap-overlay-inset-*
//     tokens AND reserves space for the PoC's legend / inspector
//   • Legend and inspector are collapsible (data-expanded toggle)
//   • lensImpl.clear performs a full PoC teardown (overlays + mount
//     + legend + inspector + body class)
//   • The PoC no longer hijacks #gmap-status with "— Cytoscape PoC"
//   • All CSS rules remain scoped under body.cytoscape-poc-active
//   • The mount no longer carries the misleading min-height: 720px
//   • Hard-coded fit padding 60 is replaced
//   • Production Authority Graph adapter / layout / view signatures
//     untouched; the PoC remains opt-in
//
// No JS execution; manual browser verification gates the visual
// checks listed in the deliverable.

// ── Stale overlay cleanup ───────────────────────────────────────────

// TestExplorer_D33aImpl1_CytoscapePayloadClearsUnavailableOverlay
// pins that _renderPayload removes any prior loading/unavailable
// overlay before initialising Cytoscape. Without this, a "Loading…"
// message left from _pocRefresh's pre-fetch state could remain
// visible over the rendered graph.
func TestExplorer_D33aImpl1_CytoscapePayloadClearsUnavailableOverlay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _clearOverlays(mount)",
		// Loop removes ALL .cytoscape-poc-unavailable nodes so repeated
		// renders cannot accumulate overlays.
		"mount.querySelectorAll('.cytoscape-poc-unavailable')",
		// _renderPayload calls _clearOverlays before Cytoscape init.
		"_clearOverlays(mount);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1: stale overlay cleanup missing %q", want)
		}
	}
}

// ── Safe-area fit padding ───────────────────────────────────────────

// TestExplorer_D33aImpl1_CytoscapeUsesSafeAreaFitPadding pins that
// Cytoscape fit padding is computed from MIDAS's existing safe-area
// CSS variables plus the PoC's own legend/inspector reserved widths.
// Replaces the pre-tranche hard-coded value of 60.
func TestExplorer_D33aImpl1_CytoscapeUsesSafeAreaFitPadding(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// D33a-impl-1a — Signature grew an optional `dims` arg so the
		// clamp can read the live mount dimensions. Intent of the pin
		// (the helper exists and is the source of fit padding) is
		// preserved.
		"function _safeAreaPadding(dims)",
		"--gmap-overlay-inset-top",
		"--gmap-overlay-inset-right",
		"--gmap-overlay-inset-bottom",
		"--gmap-overlay-inset-left",
		// Padding accounts for legend + inspector reserved widths.
		"LEGEND_W_EXPANDED",
		"INSPECTOR_W_EXPANDED",
		// The padding is passed to Cytoscape's layout config and the
		// rAF re-fit. No more hard-coded 60.
		"padding: fitPadding",
		"_cy.fit(undefined, _safeAreaPadding())",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1: safe-area fit padding missing %q", want)
		}
	}
	// Negative pin: the pre-tranche hard-coded fit padding is gone.
	for _, banned := range []string{
		"_cy.fit(undefined, 60)",
		"padding: 60,",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-impl-1: hard-coded fit padding %q must be replaced by safe-area computation", banned)
		}
	}
}

// TestExplorer_D33aImpl1_NoHardcodedMinHeight720 pins that the mount
// no longer carries the misleading min-height: 720px that pushed
// nodes below the viewport on shorter screens. The JS hack that set
// minHeight inline also goes.
func TestExplorer_D33aImpl1_NoHardcodedMinHeight720(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// JS no longer forces a 720 min-height on the mount.
	if strings.Contains(js, "mount.style.minHeight = '720px'") {
		t.Error("D33a-impl-1: inline minHeight = '720px' hack must be removed — parent grid drives mount height")
	}
	// CSS no longer hard-codes 720px in EXECUTABLE rules. Strip
	// comments first so explanatory documentation referencing the
	// dropped value doesn't trip the pin.
	cssExec := stripCSSComments(css)
	if strings.Contains(cssExec, "min-height: 720px") {
		t.Error("D33a-impl-1: mount must not carry min-height: 720px — drop the misleading floor; rely on parent height")
	}
	// CSS height now follows parent (height: 100%) with a small
	// fallback for very short viewports.
	if !strings.Contains(css, "height: 100%") {
		t.Error("D33a-impl-1: mount must use height: 100% so the parent grid drives the viewport")
	}
	if !strings.Contains(css, "min-height: 480px") {
		t.Error("D33a-impl-1: mount must keep a small min-height fallback (480px) for very short viewports")
	}
}

// ── Collapsible legend / inspector ──────────────────────────────────

// TestExplorer_D33aImpl1_LegendInspectorCollapsible pins the new
// collapse/expand toggle DOM and CSS. Reduces occlusion of graph
// content while keeping the legend reachable.
func TestExplorer_D33aImpl1_LegendInspectorCollapsible(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// JS exposes toggle setters and writes data-expanded.
	for _, want := range []string{
		"function _setLegendExpanded(state)",
		"function _setInspectorExpanded(state)",
		"data-poc-toggle=\"legend\"",
		"data-poc-toggle=\"inspector\"",
		"setAttribute('data-expanded'",
		// Inspector auto-expands on node tap; auto-collapses on bg tap.
		"if (!_inspectorExpanded) _setInspectorExpanded(true);",
		"if (_inspectorExpanded) _setInspectorExpanded(false);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1: collapsible toggle wiring missing %q", want)
		}
	}
	// CSS declares collapsed state for both panels.
	for _, want := range []string{
		".cytoscape-poc-legend[data-expanded=\"false\"]",
		".cytoscape-poc-inspector[data-expanded=\"false\"]",
		"transition: width",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D33a-impl-1: collapsed-state CSS missing %q", want)
		}
	}
}

// ── Teardown ────────────────────────────────────────────────────────

// TestExplorer_D33aImpl1_CytoscapeTeardownRemovesPocDom pins the new
// full teardown path. _uninstallPoc removes overlays, the mount, the
// legend, the inspector, and the body class. Repeated calls are
// idempotent. lensImpl.clear routes through this teardown.
func TestExplorer_D33aImpl1_CytoscapeTeardownRemovesPocDom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _uninstallPoc()",
		"_destroyCy();",
		"_mountEl.parentNode.removeChild(_mountEl);",
		"document.body.classList.remove('cytoscape-poc-active');",
		// lensImpl.clear delegates to the full teardown.
		"_uninstallPoc();",
		// _uninstall surfaced for tests and manual diagnostics.
		"_uninstall:               _uninstallPoc,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1: full teardown missing %q", want)
		}
	}
}

// ── Production status surface ───────────────────────────────────────

// TestExplorer_D33aImpl1_CytoscapeDoesNotHijackProductionStatus pins
// the removal of the `— Cytoscape PoC` text hijack. The PoC indicator
// moves into the legend chip via .cytoscape-poc-status-chip.
func TestExplorer_D33aImpl1_CytoscapeDoesNotHijackProductionStatus(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// Negative pin: status hijack is gone.
	for _, banned := range []string{
		"status.textContent = '— Cytoscape PoC'",
		"getElementById('gmap-status')",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-impl-1: production #gmap-status hijack must be removed — found %q", banned)
		}
	}
	// Positive pin: the PoC indicator moved to the legend chip.
	for _, want := range []string{
		"cytoscape-poc-status-chip",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1: PoC status badge must move into the legend chip — missing %q in JS", want)
		}
		if !strings.Contains(css, ".cytoscape-poc-status-chip") {
			t.Errorf("D33a-impl-1: PoC status badge CSS missing %q", ".cytoscape-poc-status-chip")
		}
	}
}

// ── CSS scoping (re-pin) ────────────────────────────────────────────

// TestExplorer_D33aImpl1_CytoscapeCssScopedToActiveBodyClass re-pins
// the CSS-scoping invariant after the chrome containment changes.
// Every PoC selector remains under body.cytoscape-poc-active so
// removing the body class fully suppresses every rule. Identical
// rule to TestExplorer_CytoscapePoc_CSSScopedToActiveBodyClass but
// re-run here as a regression guard for the D33a-impl-1 CSS edits.
func TestExplorer_D33aImpl1_CytoscapeCssScopedToActiveBodyClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	css = stripCSSComments(css)

	for i := 0; i < len(css); i++ {
		if css[i] != '{' {
			continue
		}
		start := strings.LastIndexAny(css[:i], "}")
		if start < 0 {
			start = 0
		} else {
			start++
		}
		selector := strings.TrimSpace(css[start:i])
		if selector == "" {
			continue
		}
		if !strings.HasPrefix(selector, "body.cytoscape-poc-active") {
			t.Errorf("D33a-impl-1: every CSS rule must be scoped under body.cytoscape-poc-active — rogue selector %q", selector)
		}
	}
}

// ── Production preservation ─────────────────────────────────────────

// TestExplorer_D33aImpl1_ProductionAuthorityPathStillDefault pins
// that the production Authority Graph pipeline is unchanged: adapter
// `mapToCardLayout`, layout helper `computeAuthorityLayout`, and the
// view's `renderAuthorityGraph` retain their signatures. The view
// must not reference Cytoscape. With ?cytoscape=1 absent, no PoC
// body class can be set because the PoC's gate short-circuits at
// init.
func TestExplorer_D33aImpl1_ProductionAuthorityPathStillDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	layoutJS  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")
	viewJS    := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	pocJS     := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	if !strings.Contains(adapterJS, "function mapToCardLayout(projection, view)") {
		t.Error("D33a-impl-1: production adapter mapToCardLayout signature must remain intact")
	}
	if !strings.Contains(layoutJS, "function computeAuthorityLayout(spec, GMAP, layerState)") {
		t.Error("D33a-impl-1: production layout helper signature must remain intact")
	}
	if !strings.Contains(viewJS, "function renderAuthorityGraph(payload, ctx)") {
		t.Error("D33a-impl-1: production view renderAuthorityGraph must remain intact")
	}
	if strings.Contains(viewJS, "cytoscape") {
		t.Error("D33a-impl-1: production Authority view must NOT reference Cytoscape — PoC remains opt-in")
	}
	// PoC's gate still short-circuits at init when ?cytoscape=1 is
	// absent, so the body class is never set.
	for _, want := range []string{
		"if (!_isPocActive()) {",
		"return;",
		"sp.get('cytoscape') === '1'",
	} {
		if !strings.Contains(pocJS, want) {
			t.Errorf("D33a-impl-1: PoC activation gate must remain — missing %q", want)
		}
	}
}
