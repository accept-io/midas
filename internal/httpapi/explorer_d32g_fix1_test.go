package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32g_fix1_test.go — D32g-fix-1 Authority Graph UI de-clutter
// contract tests. Pins:
//
//   - the canvas toolbar is compact (high-priority pills + Layers/Info
//     buttons only — no expanded legend, no chip row above the canvas)
//   - full legend, full summary counts, and layer toggles live in the
//     shared right-side drawer's existing tabs
//   - the lens body attribute (`data-graph-lens`) is set by the store
//     subscription so legacy lens-specific chrome can be hidden
//   - the legacy Context-Graph bottom-centre connector legend hides
//     when the operator is in Authority lens
//   - Authority SVG connectors carry `fill: none` (fixes the curved
//     black-band artefact bug)
//   - Diagnostic markers use a subtle left-accent stripe (the pre-fix
//     thick box-shadow ring is gone)
//   - Posture badges use shortened labels (No FMP / Inherited / etc.)
//   - Layer toggles remain client-side (no refetch)
//   - Context Graph behaviour does not regress

// TestExplorer_D32gFix2_NoAuthorityToolbar (replaces D32g-fix-1's
// ToolbarIsCompact pin) — the Authority-specific horizontal toolbar
// element is GONE. D32g-fix-2 reuses the existing graph shell chrome
// (graph header + right-side drawer) instead of injecting a second
// menu bar. The overlays module must NOT emit any toolbar markup or
// pill/button containers.
func TestExplorer_D32gFix2_NoAuthorityToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	// Banned markup — the previous toolbar wrapper + section ids must
	// not appear in JS source, so no DOM injection can resurrect them.
	for _, banned := range []string{
		`'authority-graph-toolbar'`,
		`"authority-graph-toolbar"`,
		`class="authority-graph-toolbar"`,
		`data-overlay-pills`,
		`data-overlay-buttons`,
		`data-overlay-chips`,
		`data-overlay-legend`,
		`data-overlay-button="layers"`,
		`data-overlay-button="info"`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-2: Authority toolbar removed — overlays module must NOT emit %q", banned)
		}
	}
	// And the CSS rules for the toolbar wrapper are gone too — no
	// orphan styling that could un-hide a re-introduced element.
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, banned := range []string{
		`.authority-graph-toolbar {`,
		`.authority-graph-toolbar-pills`,
		`.authority-graph-toolbar-buttons`,
		`.authority-graph-toolbar-button {`,
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D32g-fix-2: Authority toolbar CSS must be removed; found %q", banned)
		}
	}
}

// TestExplorer_D32gFix2_LayersButtonInterceptor pins that the
// overlays module installs a capture-phase click interceptor on the
// EXISTING #gmap-layers-button. When the Authority lens is active,
// the click opens the shared drawer's "config" tab (Posture & Help)
// instead of the Context-only .gmap-layers-panel popover.
//
// Capture-phase + stopImmediatePropagation prevents the inline
// wireGmapLayersButton handler from also firing — operators see ONE
// Layers control opening ONE surface.
func TestExplorer_D32gFix2_LayersButtonInterceptor(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	if !strings.Contains(js, `document.getElementById('gmap-layers-button')`) {
		t.Error("D32g-fix-2: overlays module must resolve the existing #gmap-layers-button")
	}
	if !strings.Contains(js, `_ensureLayersButtonInterceptor`) {
		t.Error("D32g-fix-2: overlays module must define _ensureLayersButtonInterceptor")
	}
	if !strings.Contains(js, `'click', _interceptorHandler, true`) {
		t.Error("D32g-fix-2: interceptor must register in capture-phase (`true` flag) so it runs BEFORE the Context-Graph panel handler")
	}
	if !strings.Contains(js, `e.stopImmediatePropagation`) {
		t.Error("D32g-fix-2: interceptor must stopImmediatePropagation so the Context-Graph panel handler does not also fire")
	}
	if !strings.Contains(js, `drawer.open('config')`) {
		t.Error("D32g-fix-2: interceptor must open the shared drawer to the 'config' slot (Posture & Help)")
	}
	// The redirect is lens-aware — only fires when Authority is the
	// active lens. Context Graph behaviour is untouched.
	if !strings.Contains(js, `_activeLens() !== 'authority'`) {
		t.Error("D32g-fix-2: interceptor must guard on selectedGraphLens === 'authority' so Context Graph Layers button behaviour does not regress")
	}
}

// TestExplorer_D32gFix1_LegendInDrawerNotToolbar pins that the full
// legend lives behind a drawer-rendered method (renderLegendInto)
// and is wired into the Posture & Help tab. Updated for D32g-fix-2:
// the legend renderer survives unchanged; only the toolbar wrapper
// that USED to expose a Legend button is gone.
func TestExplorer_D32gFix1_LegendInDrawerNotToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	// New API surface — legend renderable into an arbitrary mount.
	if !strings.Contains(js, "function renderLegendInto(mount)") {
		t.Error("D32g-fix-1: overlays module must expose renderLegendInto(mount) for the drawer")
	}
	if !strings.Contains(js, "function renderSummaryInto(mount, payload)") {
		t.Error("D32g-fix-1: overlays module must expose renderSummaryInto(mount, payload) for the drawer")
	}
	if !strings.Contains(js, "function renderLayerChipsInto(mount)") {
		t.Error("D32g-fix-1: overlays module must expose renderLayerChipsInto(mount) for the drawer")
	}
	// The legend itself still references the adapter (single source
	// of truth for node-kind / edge-kind labels).
	if !strings.Contains(js, "adapter.NODE_KINDS") {
		t.Error("D32g-fix-1: renderLegendInto must still iterate adapter.NODE_KINDS")
	}
	if !strings.Contains(js, "adapter.EDGE_KINDS") {
		t.Error("D32g-fix-1: renderLegendInto must still iterate adapter.EDGE_KINDS")
	}

	// D37av2-prereq — these positive pins were flipped to negative
	// pins after the Posture & Help drawer tab was decommissioned.
	// The overlays.renderLegendInto / renderSummaryInto /
	// renderLayerChipsInto functions still exist in the overlays
	// module (pinned above) but are no longer wired into the Authority
	// drawer view. The drawer's Posture & Help tab registration was
	// dropped; the only remaining Authority drawer slot is
	// `inspector`. Surface Posture moved to the Workbench Posture
	// tab; the legend is owned by OSS Help (/help/static/graphs/
	// authority-graph/); summary pills are duplicated by the
	// Workbench Overview tab; layer chips are not migrated as live
	// runtime UI in this tranche.
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, banned := range []string{
		`overlays.renderLegendInto`,
		`overlays.renderSummaryInto`,
		`overlays.renderLayerChipsInto`,
		`label: 'Posture & Help'`,
		`data-authority-legend`,
		`data-authority-summary-mount`,
		`data-authority-layer-chips`,
	} {
		if strings.Contains(viewJS, banned) {
			t.Errorf("D37av2-prereq: Authority drawer view must not wire %q — Posture & Help drawer tab decommissioned", banned)
		}
	}
}

// TestExplorer_D32gFix2_RiskCountsLiveInDrawer (replaces D32g-fix-1's
// HighPriorityPillsOnly pin) — risk / gap counts are NOT rendered as
// a canvas-overlay pill row. They appear inside the shared drawer's
// Diagnostics + Posture & Help tabs.
//
// The drawer's Posture & Help summary still emits the gap pills (for
// completeness when the operator opens the drawer); the canvas
// chrome stays clean.
func TestExplorer_D32gFix2_RiskCountsLiveInDrawer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	// _renderHighPriorityPills no longer exists — the in-toolbar
	// high-priority pill block was removed entirely in D32g-fix-2.
	if strings.Contains(js, "function _renderHighPriorityPills") {
		t.Error("D32g-fix-2: in-toolbar high-priority pill renderer must be removed (risk counts live in drawer only)")
	}
	if strings.Contains(js, "function _renderToolbarButtons") {
		t.Error("D32g-fix-2: in-toolbar button renderer must be removed (Layers button is the existing #gmap-layers-button)")
	}

	// The drawer-summary renderer still emits the gap pill labels —
	// operators see them inside the Posture & Help tab.
	summaryFn := strings.Index(js, "function renderSummaryInto(mount, payload)")
	if summaryFn < 0 {
		t.Fatal("D32g-fix-2: renderSummaryInto must still exist (drawer Summary content provider)")
	}
	summarySlice := js[summaryFn:]
	for _, want := range []string{
		`'Surfaces missing profile'`,
		`'Surfaces without fail-mode policy'`,
		`'Critical diagnostics'`,
		`'Warning diagnostics'`,
	} {
		if !strings.Contains(summarySlice, want) {
			t.Errorf("D32g-fix-2: drawer summary must emit gap pill %s (risk counts move into drawer)", want)
		}
	}
}

// TestExplorer_D32gFix1_BodyDataLensSubscription pins that the inline
// IIFE mirrors selectedGraphLens onto body[data-graph-lens] so CSS
// can hide the legacy bottom-centre connector legend on Authority
// lens (and re-show it on Context lens).
func TestExplorer_D32gFix1_BodyDataLensSubscription(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequestStr(t, srv, "/explorer")
	if !strings.Contains(body, `document.body.setAttribute('data-graph-lens', state.selectedGraphLens)`) {
		t.Error("D32g-fix-1: index.html's store subscription must mirror selectedGraphLens onto body[data-graph-lens]")
	}
}

// TestExplorer_D32gFix1_LegacyLegendHiddenOnAuthority pins the CSS
// rule that hides the bottom-centre Context-Graph connector legend
// when body[data-graph-lens="authority"] is set.
func TestExplorer_D32gFix1_LegacyLegendHiddenOnAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	if !strings.Contains(css, `body[data-graph-lens="authority"] .gmap-legend-overlay`) {
		t.Error("D32g-fix-1: authority-graph.css must hide .gmap-legend-overlay when body[data-graph-lens=\"authority\"]")
	}
	if !strings.Contains(css, `display: none`) {
		t.Error("D32g-fix-1: authority-graph.css must declare display: none somewhere (sanity for the lens-hide rule)")
	}
}

// TestExplorer_D32gFix2_OnlyOneLayersButton pins that the Authority
// lens does NOT introduce a second Layers button. The existing
// #gmap-layers-button in the shared graph header is the single
// Layers control across both lenses.
//
// The overlays module is forbidden from emitting a button with a
// label of "Layers" or any wrapper class implying a top-level
// Authority Layers control.
func TestExplorer_D32gFix2_OnlyOneLayersButton(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	// The legitimate references are inside renderLayerChipsInto's
	// drawer-section title ("Layers" header text). The forbidden
	// references are second buttons / second control wrappers.
	for _, banned := range []string{
		`<button type="button" class="authority-graph-toolbar-button"`,
		`>Layers</button>`,
		`data-overlay-button="layers"`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-2: there must be only one Layers control (the existing #gmap-layers-button); overlays module must not declare %q", banned)
		}
	}
	// The drawer-section heading inside the Posture & Help tab is
	// allowed — it labels the chip list, not a button.
	if !strings.Contains(js, `<h4 class="authority-drawer-section-title">Layers</h4>`) {
		t.Error("D32g-fix-2: drawer's Posture & Help tab must still label its layer-chip section 'Layers'")
	}
}

// TestExplorer_D32gFix1_ConnectorFillNoneFix pins the edge-rendering
// bug fix. The SVG `<path>` default `fill: black` produces a thick
// curved band when the bezier `d` attribute encloses a curve area —
// the "large black artefact" symptom that masked node cards. Every
// authority connector class must explicitly declare `fill: none`.
func TestExplorer_D32gFix1_ConnectorFillNoneFix(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	// Base rule covering all authority connectors.
	if !strings.Contains(css, `[class*="authority-connector-"]`) {
		t.Error("D32g-fix-1: authority-graph.css must declare a [class*=\"authority-connector-\"] base rule")
	}
	// Each named connector class must carry fill: none.
	for _, kind := range []string{
		"business_service_has_surface",
		"surface_uses_profile",
		"profile_has_grant",
		"grant_authorises_agent",
		"surface_has_fail_mode_policy",
		"business_service_has_fail_mode_policy",
		"profile_escalates_to",
	} {
		rule := ".authority-connector-" + kind
		idx := strings.Index(css, rule)
		if idx < 0 {
			t.Errorf("D32g-fix-1: %s rule missing", rule)
			continue
		}
		// Look for `fill: none` within the next 200 chars.
		end := idx + 280
		if end > len(css) {
			end = len(css)
		}
		slice := css[idx:end]
		if !strings.Contains(slice, "fill: none") {
			t.Errorf("D32g-fix-1: %s must declare fill: none (otherwise the curved bezier area paints black)", rule)
		}
	}
}

// TestExplorer_D32gFix1_DiagnosticTreatmentIsSubtle pins that the
// diagnostic severity marker uses a subtle left-edge accent stripe
// instead of the pre-fix thick full-card box-shadow ring.
func TestExplorer_D32gFix1_DiagnosticTreatmentIsSubtle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	// New subtle treatment uses a ::before pseudo-element on the node.
	if !strings.Contains(css, `.gmap-node[data-diagnostic-severity]::before`) {
		t.Error("D32g-fix-1: diagnostic marker must use a ::before accent stripe, not a thick box-shadow ring")
	}
	// Pre-fix loud treatment is gone.
	if strings.Contains(css, `.gmap-node[data-diagnostic-severity="critical"] {`) {
		// Allow the block if it ONLY references ::after for the corner dot;
		// but the loud 2px+12px shadow rule must NOT match.
		segIdx := strings.Index(css, `.gmap-node[data-diagnostic-severity="critical"] {`)
		end := segIdx + 200
		if end > len(css) {
			end = len(css)
		}
		seg := css[segIdx:end]
		if strings.Contains(seg, "box-shadow: 0 0 0 2px") && strings.Contains(seg, "0 0 12px") {
			t.Error("D32g-fix-1: pre-fix loud critical box-shadow ring must be removed")
		}
	}
	// Severity hierarchy still distinguishable.
	for _, sev := range []string{
		`.gmap-node[data-diagnostic-severity="critical"]::before`,
		`.gmap-node[data-diagnostic-severity="warning"]::before`,
		`.gmap-node[data-diagnostic-severity="info"]::before`,
	} {
		if !strings.Contains(css, sev) {
			t.Errorf("D32g-fix-1: severity rule %s must exist (hierarchy preserved)", sev)
		}
	}
}

// TestExplorer_D32gFix1_PostureBadgeShortLabels pins the shortened
// posture badge labels emitted by the view (No FMP / Inherited /
// Override / Dangling / Blocked / No profile / No grant).
func TestExplorer_D32gFix1_PostureBadgeShortLabels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, want := range []string{
		`text: 'Dangling'`,
		`text: 'No FMP'`,
		`text: 'Blocked'`,
		`text: 'No profile'`,
		`text: 'No grant'`,
	} {
		if !strings.Contains(viewJS, want) {
			t.Errorf("D32g-fix-1: posture badge must use short label %s", want)
		}
	}
	// Pre-fix verbose labels are gone.
	for _, banned := range []string{
		`text: 'FMP dangling'`,
		`text: 'FMP missing'`,
		`text: 'agent blocked'`,
		`text: 'no profile'`,
		`text: 'no grant'`,
	} {
		if strings.Contains(viewJS, banned) {
			t.Errorf("D32g-fix-1: pre-fix verbose label %s must be removed (replaced with shorter form)", banned)
		}
	}

	// The adapter's posture-source badge also shortens to Override /
	// Inherited / No FMP.
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	for _, want := range []string{
		`text: 'Override'`,
		`text: 'Inherited'`,
		`text: 'No FMP'`,
	} {
		if !strings.Contains(adapterJS, want) {
			t.Errorf("D32g-fix-1: adapter posture-source badge must use short label %s", want)
		}
	}
}

// TestExplorer_D32gFix1_LayerToggleStillClientSide pins that toggling
// a layer chip does not refetch from the backend. The chip handler
// must apply a CSS class via _applyLayerState, NOT call shell.refresh
// or the API client.
func TestExplorer_D32gFix1_LayerToggleStillClientSide(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	if !strings.Contains(js, "_applyLayerState") {
		t.Error("D32g-fix-1: layer toggle path must dispatch through _applyLayerState (CSS-class only)")
	}
	for _, banned := range []string{
		"shell.refresh",
		"ExplorerAPI.graphs.authority(",
		"fetch('/v1",
		"fetch(\"/v1",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-1: overlays module must NOT call %s (chip toggle is client-side only)", banned)
		}
	}
}

// TestExplorer_D32gFix1_LayerHideRulesStillPresent pins the four CSS
// layer-off rules that the chip toggles depend on. These rules
// existed pre-D32g; this test confirms they survive the corrective
// pass.
func TestExplorer_D32gFix1_LayerHideRulesStillPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, rule := range []string{
		".authority-layer-diagnostics-off",
		".authority-layer-surface-posture-off",
		".authority-layer-escalation-off",
		".authority-layer-fail-mode-off",
	} {
		if !strings.Contains(css, rule) {
			t.Errorf("D32g-fix-1: layer-hide rule %s must remain after the corrective pass", rule)
		}
	}
}

// TestExplorer_D32gFix1_ContextGraphLegendUnregressed pins that
// Context Graph's bottom-centre connector legend STILL exists in
// index.html (it's only the lens-aware hide that's new — Context
// behaviour must not regress).
func TestExplorer_D32gFix1_ContextGraphLegendUnregressed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequestStr(t, srv, "/explorer")
	if !strings.Contains(body, `class="gmap-legend-overlay"`) {
		t.Error("D32g-fix-1: Context Graph connector legend must remain in the DOM (only the lens-aware CSS rule hides it on Authority)")
	}
	// Same body must NOT carry an authority-only replacement legend
	// element — the full legend lives in the drawer.
	if strings.Contains(body, `class="authority-legend-overlay"`) {
		t.Error("D32g-fix-1: there must not be a duplicate Authority-specific legend container in the DOM")
	}
}

// TestExplorer_D32gFix1_OverlaysModulePreservedSurface confirms the
// public surface of the overlays module (render, clear, _LAYER_CHIPS,
// _layerClassFor) survives the corrective pass — these are pinned by
// existing D32f tests and continue to be required by the drawer
// integration.
func TestExplorer_D32gFix1_OverlaysModulePreservedSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	for _, want := range []string{
		"render:               render",
		"clear:                clear",
		"renderLegendInto:     renderLegendInto",
		"renderSummaryInto:    renderSummaryInto",
		"renderLayerChipsInto: renderLayerChipsInto",
		"_LAYER_CHIPS:         LAYER_CHIPS",
		"_layerClassFor:       _layerClassFor",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32g-fix-1: overlays module public surface must declare %q", want)
		}
	}
}

// performRequestStr is a tiny helper that returns the body of a
// performRequest GET as a string. Used by tests that only need to
// substring-match index.html or a fetched asset.
func performRequestStr(t *testing.T, srv *Server, path string) string {
	t.Helper()
	rec := performRequest(t, srv, "GET", path, nil)
	return rec.Body.String()
}
