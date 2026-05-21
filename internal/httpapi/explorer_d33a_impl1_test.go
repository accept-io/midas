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
//   • _uninstallPoc performs a full PoC teardown (overlays + mount
//     + legend + inspector + GraphViewport host deactivate);
//     D37p-clean-1 retired the dead lensImpl.clear delegation, but
//     _uninstallPoc is still reachable via the public _uninstall
//     surface and the host's factory destroy path
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
		// D33x-list-mode — `INSPECTOR_W_*` reservation was retired
		// along with the floating PoC card.
		// D33x-left-poc-panel — `LEGEND_W_*` reservation was
		// retired along with the floating left PoC panel. The
		// padding pin now only asserts the `--gmap-overlay-inset-*`
		// token reads + the padding being plumbed into Cytoscape's
		// layout config below.
		//
		// The padding is passed to Cytoscape's layout config; the
		// rAF re-fit now delegates to the asymmetric helper
		// (D33x-fit-zoom-root) rather than calling cy.fit directly.
		"padding: fitPadding",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1: safe-area fit padding missing %q", want)
		}
	}
	// Negative pins:
	//   • Pre-D33a-impl-1 hard-coded fit padding stays banned.
	//   • D33x-list-mode — `INSPECTOR_W_*` constants must NOT come back.
	//   • D33x-left-poc-panel — `LEGEND_W_*` constants must NOT come back.
	for _, banned := range []string{
		"_cy.fit(undefined, 60)",
		"padding: 60,",
		"INSPECTOR_W_EXPANDED",
		"INSPECTOR_W_COMPACT",
		"LEGEND_W_EXPANDED",
		"LEGEND_W_COMPACT",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-impl-1: %q must not appear — superseded (floating PoC card + left legend panel retired)", banned)
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

// TestExplorer_D33aImpl1_LegendInspectorCollapsible — superseded.
//
// D33x-list-mode — The Inspector half was retired along with the
// floating PoC card.
// D33x-left-poc-panel — The Legend half was retired along with the
// floating left "Authority Graph" PoC panel (NODE KINDS / FUTURE
// OVERLAYS / AUTHORITY-THIN-CARD-V1). The MIDAS Posture & Help
// drawer tab owns the equivalent posture + legend information.
//
// The test now asserts the inverse contract for both halves: every
// legend + inspector overlay symbol must remain GONE from the PoC
// JS, and every related CSS selector must remain GONE from the PoC
// CSS.
func TestExplorer_D33aImpl1_LegendInspectorCollapsible(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// JS — retired overlay symbols must not return.
	for _, banned := range []string{
		// Floating left legend panel (D33x-left-poc-panel).
		"function _setLegendExpanded(state)",
		"function _renderLegend(",
		"function _legendRow(",
		`data-poc-toggle="legend"`,
		// Floating right inspector card (D33x-list-mode).
		"function _setInspectorExpanded(state)",
		"function _renderInspector(",
		"function _renderInspectorEmpty(",
		"function _wireInspectorToggle(",
		`data-poc-toggle="inspector"`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33x-left-poc-panel / D33x-list-mode: retired PoC overlay JS symbol %q must NOT come back", banned)
		}
	}
	// CSS — retired overlay selectors must not return. Comments may
	// still reference the retired class names (in the retirement-
	// marker block); pin against the executable rule shape `<sel> {`.
	cssExec := stripCSSComments(css)
	for _, banned := range []string{
		// Floating left legend panel (D33x-left-poc-panel).
		".cytoscape-poc-legend {",
		".cytoscape-poc-legend[data-expanded",
		".cytoscape-poc-legend-body",
		".cytoscape-poc-legend-title",
		".cytoscape-poc-legend-kinds",
		".cytoscape-poc-legend-future",
		".cytoscape-poc-swatch",
		".cytoscape-poc-placeholder",
		// Floating right inspector card (D33x-list-mode).
		".cytoscape-poc-inspector {",
		".cytoscape-poc-inspector[data-expanded",
		".cytoscape-poc-inspector-body",
		".cytoscape-poc-inspector-fields",
	} {
		if strings.Contains(cssExec, banned) {
			t.Errorf("D33x-left-poc-panel / D33x-list-mode: retired PoC overlay CSS selector %q must NOT come back", banned)
		}
	}
}

// ── Teardown ────────────────────────────────────────────────────────

// TestExplorer_D33aImpl1_CytoscapeTeardownRemovesPocDom pins the
// full teardown path. _uninstallPoc removes overlays, the mount, the
// legend, the inspector, and routes through the GraphViewport host's
// deactivate(). Repeated calls are idempotent. D37p-clean-1 retired
// the dead `lensImpl.clear` delegation; the public `_uninstall`
// surface and the GraphViewport factory destroy path are the
// remaining live callers.
func TestExplorer_D33aImpl1_CytoscapeTeardownRemovesPocDom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _uninstallPoc()",
		"_destroyCy();",
		"_mountEl.parentNode.removeChild(_mountEl);",
		// D35f — body-class removal RETIRED. Renderer identity now
		// host-owned via data-active-renderer; the host's
		// `viewport.deactivate()` clears it. _uninstallPoc routes
		// through that path.
		"vp.deactivate",
		// _uninstall surfaced for tests and manual diagnostics.
		"_uninstall:               _uninstallPoc,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1: full teardown missing %q", want)
		}
	}
	// D35f — body-class flip retired.
	if strings.Contains(js, "document.body.classList.remove('cytoscape-poc-active');") {
		t.Error("D35f: _uninstallPoc must NOT flip body class (host owns renderer identity)")
	}
}

// ── Production status surface ───────────────────────────────────────

// TestExplorer_D33aImpl1_CytoscapeDoesNotHijackProductionStatus pins
// the removal of the `— Cytoscape PoC` text hijack on the production
// status pill.
//
// D33x-left-poc-panel — The PoC status badge previously lived inside
// the floating left legend's header chip (`cytoscape-poc-status-chip`)
// — that whole panel has been retired. The "no production status
// hijack" half of the contract still matters; the "indicator lives
// in the legend chip" half is now stale, because the legend chip is
// gone. The test now asserts:
//   • the production status pill is NOT hijacked (unchanged);
//   • the legend status / theme chip surface is RETIRED, not present.
func TestExplorer_D33aImpl1_CytoscapeDoesNotHijackProductionStatus(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// Negative pin: production status hijack is gone.
	for _, banned := range []string{
		"status.textContent = '— Cytoscape PoC'",
		"getElementById('gmap-status')",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-impl-1: production #gmap-status hijack must be removed — found %q", banned)
		}
	}
	// D33x-left-poc-panel — the legend-chip status/theme markup is
	// retired. JS no longer emits the chip; CSS rules for the chip
	// classes are gone. Comments in the CSS retirement-marker block
	// may still mention the class names; pin against the executable
	// rule shape `<sel> {`.
	cssExec := stripCSSComments(css)
	for _, banned := range []string{
		"cytoscape-poc-status-chip",
		"cytoscape-poc-theme-chip",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33x-left-poc-panel: legend-chip JS markup must remain retired — found %q", banned)
		}
		if strings.Contains(cssExec, "."+banned+" {") {
			t.Errorf("D33x-left-poc-panel: legend-chip CSS rule must remain retired — found `.%s {`", banned)
		}
	}
}

// ── CSS scoping (re-pin) ────────────────────────────────────────────

// TestExplorer_D33aImpl1_CytoscapeCssScopedToActiveBodyClass re-pins
// the CSS-scoping invariant after the chrome containment changes.
// Every PoC selector remains under host-owned renderer identity.
//
// D35f-retire-transitional-renderer-debt — the gate moved from
// `body.cytoscape-poc-active` (pre-D35f) to
// `.midas-graph-viewport[data-active-renderer="authority"]`.
// The underlying invariant ("disabling the PoC fully suppresses
// every rule") is preserved; only the activation key changed.
func TestExplorer_D33aImpl1_CytoscapeCssScopedToActiveBodyClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	css = stripCSSComments(css)
	const expectedPrefix = `.midas-graph-viewport[data-active-renderer="authority"]`

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
		if !strings.HasPrefix(selector, expectedPrefix) {
			t.Errorf("D35f: every CSS rule must be scoped under %s — rogue selector %q", expectedPrefix, selector)
		}
	}
}

// ── Production preservation ─────────────────────────────────────────

// TestExplorer_D33aImpl1_ProductionAuthorityPathStillDefault was a
// pre-D37b invariant that pinned the production native renderer as
// the DEFAULT Authority path AND the PoC as strictly opt-in via
// `?cytoscape=1`. D37b RETIRED that invariant: the Cytoscape
// HTML-card renderer is now the production Authority path
// (registered with GraphViewport under id `'authority'`), and the
// pre-D37b URL gate is gone.
//
// The test is retained (renamed in spirit, not in symbol so other
// runners keep working) to pin the post-D37b state:
//   • The legacy native modules (`mapToCardLayout`,
//     `computeAuthorityLayout`, `renderAuthorityGraph`) still exist
//     as internal fallback / diagnostic code — removal would have
//     been too broad for D37b.
//   • `authority-graph-view.js` still does NOT reference Cytoscape
//     (Cytoscape lives entirely in `authority-cytoscape-poc.js`).
//   • The pre-D37b PoC gate (`if (!_isPocActive()) { return; }`,
//     `sp.get('cytoscape') === '1'`) MUST NOT remain in the
//     executable code of the Authority renderer module.
func TestExplorer_D33aImpl1_ProductionAuthorityPathStillDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	layoutJS  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")
	viewJS    := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	pocJS     := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	if !strings.Contains(adapterJS, "function mapToCardLayout(projection, view)") {
		t.Error("D33a-impl-1: legacy native adapter mapToCardLayout signature must remain (internal fallback after D37b)")
	}
	if !strings.Contains(layoutJS, "function computeAuthorityLayout(spec, GMAP, layerState)") {
		t.Error("D33a-impl-1: legacy native layout helper signature must remain (internal fallback after D37b)")
	}
	if !strings.Contains(viewJS, "function renderAuthorityGraph(payload, ctx)") {
		t.Error("D33a-impl-1: legacy native renderAuthorityGraph must remain (internal fallback after D37b)")
	}
	if strings.Contains(viewJS, "cytoscape") {
		t.Error("D33a-impl-1: legacy native Authority view must NOT reference Cytoscape — Cytoscape lives in authority-cytoscape-poc.js")
	}
	// D37b — pre-D37b activation gate MUST be retired from executable
	// code in the Authority renderer module.
	pocExec := stripJSComments(pocJS)
	for _, banned := range []string{
		"if (!_isPocActive()) {",
		"sp.get('cytoscape') === '1'",
	} {
		if strings.Contains(pocExec, banned) {
			t.Errorf("D33a-impl-1/D37b: pre-D37b PoC gate %q must be retired from executable code", banned)
		}
	}
}
