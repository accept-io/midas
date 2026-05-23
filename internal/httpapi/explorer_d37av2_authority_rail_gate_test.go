package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d37av2_authority_rail_gate_test.go —
// D37av2-authority-right-rail-gate-impl.
//
// This tranche hides the Authority right rail (#gmap-details) by
// default when the Authority renderer is active, mirrors the D37aq
// Context-gating pattern but for Authority full-rail gating, and
// makes the rail visibility restorable through a fallback flag
// `?legacyAuthorityRail=1`. The fallback is visibility-only: no
// decommissioned content (Diagnostics / Posture & Help / Layer chips
// / Legend / Help framing) returns under the flag. Surface Posture
// remains in the Workbench Posture tab regardless of flag state.
//
// All decommissioned content was removed by
// D37av2-prereq-authority-rail-decommission-content-impl. This tranche
// is a gating tranche, not a deletion tranche — the physical
// #gmap-details DOM, Authority Inspector slot registration,
// authority-graph-inspector.js, and graph-drawer.js / graph-inspector.js
// all remain in place behind the fallback flag.

const (
	d37av2RailGateCanvasEdgePath = "explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37av2RailGateShellCssPath   = "explorer/assets/css/shell.css"
	d37av2RailGateIndexHtmlPath  = "explorer/index.html"
	d37av2RailGateViewPath       = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d37av2RailGateContextBridge  = "explorer/assets/js/graph/context/context-selection-bridge.js"
	d37av2RailGateContextPane    = "explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37av2RailGateWorkbenchPath  = "explorer/assets/js/graph/authority/authority-graph-workbench.js"
	d37av2RailGateInspectorPath  = "explorer/assets/js/graph/authority/authority-graph-inspector.js"
)

func d37av2RailGateRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("D37av2-rail-gate: cannot read %s: %v", path, err)
	}
	return string(b)
}

// ── 1. Flag parser declared ──────────────────────────────────────────

// TestExplorer_D37av2AuthorityRailGate_FlagParserDeclared pins that
// authority-canvas-edge-tabs.js declares its own fallback flag and
// parser, distinct from D37aq's Context fallback. The flag is
// engineering-grade — visibility-only restoration of the legacy
// Authority rail.
func TestExplorer_D37av2AuthorityRailGate_FlagParserDeclared(t *testing.T) {
	js := d37av2RailGateRead(t, d37av2RailGateCanvasEdgePath)

	if !strings.Contains(js, "LEGACY_AUTHORITY_RAIL_QUERY_PARAM") {
		t.Error("D37av2-rail-gate: canvas-edge module must declare LEGACY_AUTHORITY_RAIL_QUERY_PARAM constant")
	}
	if !strings.Contains(js, "'legacyAuthorityRail'") {
		t.Error("D37av2-rail-gate: fallback flag name must be 'legacyAuthorityRail'")
	}
	if !strings.Contains(js, "function _hasLegacyAuthorityRailFlag()") {
		t.Error("D37av2-rail-gate: canvas-edge module must define _hasLegacyAuthorityRailFlag() parser")
	}
	// Parser reads window.location.search.
	if !strings.Contains(js, "window.location && window.location.search") {
		t.Error("D37av2-rail-gate: flag parser must read window.location.search")
	}
	// Truthy variant detection — accept `=1`.
	if !strings.Contains(js, "decodeURIComponent(pair[1] || '') === '1'") {
		t.Error("D37av2-rail-gate: flag parser must accept legacyAuthorityRail=1")
	}
	// Distinct from the Context flag.
	if strings.Contains(js, "'legacyContextInspector'") {
		t.Error("D37av2-rail-gate: Authority canvas-edge module must NOT use the Context flag 'legacyContextInspector' — Authority owns its own flag")
	}
}

// ── 2. Body-attribute lifecycle ──────────────────────────────────────

// TestExplorer_D37av2AuthorityRailGate_BodyAttributeLifecycle pins the
// body-attribute lifecycle for the Authority right-rail gate. The
// attribute is set under Authority+no-flag, cleared otherwise, and
// re-applied synchronously on every viewport / body class change
// (both observers re-apply via the same MutationObserver callback
// so lens flips do not flicker).
func TestExplorer_D37av2AuthorityRailGate_BodyAttributeLifecycle(t *testing.T) {
	js := d37av2RailGateRead(t, d37av2RailGateCanvasEdgePath)

	if !strings.Contains(js, "STRATEGIC_AUTHORITY_RAIL_BODY_ATTR") {
		t.Error("D37av2-rail-gate: canvas-edge module must declare STRATEGIC_AUTHORITY_RAIL_BODY_ATTR constant")
	}
	if !strings.Contains(js, "'data-strategic-authority-rail'") {
		t.Error("D37av2-rail-gate: body attribute must be 'data-strategic-authority-rail'")
	}
	if !strings.Contains(js, "'hidden'") {
		t.Error("D37av2-rail-gate: body attribute value must be 'hidden'")
	}
	if !strings.Contains(js, "function _applyStrategicAuthorityRailAttribute()") {
		t.Error("D37av2-rail-gate: canvas-edge module must define _applyStrategicAuthorityRailAttribute() lifecycle helper")
	}
	// The helper ANDs the active-renderer check with the fallback-flag
	// absence — both must hold to set the attribute.
	if !strings.Contains(js, "_isAuthorityLensActive() && !_hasLegacyAuthorityRailFlag()") {
		t.Error("D37av2-rail-gate: helper must guard set-attribute on _isAuthorityLensActive() AND not _hasLegacyAuthorityRailFlag()")
	}
	// set / remove paths.
	if !strings.Contains(js, "document.body.setAttribute(\n        STRATEGIC_AUTHORITY_RAIL_BODY_ATTR,\n        STRATEGIC_AUTHORITY_RAIL_HIDDEN_VALUE)") &&
		!strings.Contains(js, "document.body.setAttribute(STRATEGIC_AUTHORITY_RAIL_BODY_ATTR, STRATEGIC_AUTHORITY_RAIL_HIDDEN_VALUE)") {
		t.Error("D37av2-rail-gate: helper must set body attribute via document.body.setAttribute(STRATEGIC_AUTHORITY_RAIL_BODY_ATTR, STRATEGIC_AUTHORITY_RAIL_HIDDEN_VALUE)")
	}
	if !strings.Contains(js, "document.body.removeAttribute(STRATEGIC_AUTHORITY_RAIL_BODY_ATTR)") {
		t.Error("D37av2-rail-gate: helper must clear body attribute via document.body.removeAttribute(STRATEGIC_AUTHORITY_RAIL_BODY_ATTR)")
	}
	// Apply on init() and clear on destroy().
	if !strings.Contains(js, "_applyStrategicAuthorityRailAttribute();") {
		t.Error("D37av2-rail-gate: lifecycle helper must be called from init() so the gate applies before first interaction")
	}
	// Synchronous re-apply on viewport observer callback (no render-
	// frame delay; same MutationObserver callback that observes
	// data-active-renderer flips).
	vpStart := strings.Index(js, "function _bindViewportObserver()")
	if vpStart < 0 {
		t.Fatal("D37av2-rail-gate: _bindViewportObserver must exist in canvas-edge module")
	}
	vpEnd := strings.Index(js[vpStart:], "\n  }\n")
	if vpEnd < 0 {
		t.Fatal("D37av2-rail-gate: could not bound _bindViewportObserver body")
	}
	vpBody := js[vpStart : vpStart+vpEnd]
	if !strings.Contains(vpBody, "_applyStrategicAuthorityRailAttribute()") {
		t.Error("D37av2-rail-gate: viewport MutationObserver callback must call _applyStrategicAuthorityRailAttribute() so renderer-identity flips clear the attribute synchronously (no flicker, no race)")
	}
	// Synchronous re-apply on body observer callback as well — the
	// body attribute often flips BEFORE the viewport attribute on
	// lens switch.
	bodyStart := strings.Index(js, "function _bindBodyClassObserver()")
	if bodyStart < 0 {
		t.Fatal("D37av2-rail-gate: _bindBodyClassObserver must exist in canvas-edge module")
	}
	bodyEnd := strings.Index(js[bodyStart:], "\n  }\n")
	if bodyEnd < 0 {
		t.Fatal("D37av2-rail-gate: could not bound _bindBodyClassObserver body")
	}
	bodyBody := js[bodyStart : bodyStart+bodyEnd]
	if !strings.Contains(bodyBody, "_applyStrategicAuthorityRailAttribute()") {
		t.Error("D37av2-rail-gate: body MutationObserver callback must call _applyStrategicAuthorityRailAttribute() so early-lens-switch signals (data-graph-lens) update the gate")
	}
	// Cleanup on destroy.
	destroyStart := strings.Index(js, "function destroy() {")
	if destroyStart < 0 {
		t.Fatal("D37av2-rail-gate: destroy() must exist in canvas-edge module")
	}
	destroyEnd := strings.Index(js[destroyStart:], "\n  }\n")
	if destroyEnd < 0 {
		t.Fatal("D37av2-rail-gate: could not bound destroy() body")
	}
	destroyBody := js[destroyStart : destroyStart+destroyEnd]
	if !strings.Contains(destroyBody, "removeAttribute(STRATEGIC_AUTHORITY_RAIL_BODY_ATTR)") {
		t.Error("D37av2-rail-gate: destroy() must clear the Authority rail body attribute on teardown")
	}
}

// ── 3. CSS hides the Authority rail ──────────────────────────────────

// TestExplorer_D37av2AuthorityRailGate_CssHidesAuthorityRail pins the
// shell.css rule that hides #gmap-details when the Authority body
// attribute is set. The selector is Authority-specific and uses
// (1,3,1)+ specificity to win against the (1,1,1) default `body.gmap-
// mode #gmap-details { display: flex; }` rule, matching the D37aq fix
// pattern.
func TestExplorer_D37av2AuthorityRailGate_CssHidesAuthorityRail(t *testing.T) {
	css := d37av2RailGateRead(t, d37av2RailGateShellCssPath)

	// Hide rule.
	if !strings.Contains(css, `body.gmap-mode[data-strategic-authority-rail="hidden"] #gmap-details.gmap-right-rail {`) {
		t.Error("D37av2-rail-gate: shell.css must declare the Authority rail hide selector `body.gmap-mode[data-strategic-authority-rail=\"hidden\"] #gmap-details.gmap-right-rail`")
	}
	// Authority-specific: must NOT use the Context attribute.
	authorityHideIdx := strings.Index(css, `body.gmap-mode[data-strategic-authority-rail="hidden"] #gmap-details.gmap-right-rail`)
	if authorityHideIdx < 0 {
		t.Fatal("D37av2-rail-gate: Authority hide selector missing")
	}
	// Bound the next 200 chars to extract the rule body.
	end := authorityHideIdx + 200
	if end > len(css) {
		end = len(css)
	}
	ruleSlice := css[authorityHideIdx:end]
	if !strings.Contains(ruleSlice, "display: none;") {
		t.Error("D37av2-rail-gate: Authority rail hide rule must declare `display: none;`")
	}
	// Specificity check — count the elements/classes/attributes/ids in
	// the selector. Expected:
	//   body                                        — 1 element
	//   .gmap-mode                                  — 1 class
	//   [data-strategic-authority-rail="hidden"]    — 1 attribute (= 1 class for specificity)
	//   #gmap-details                               — 1 id
	//   .gmap-right-rail                            — 1 class
	// Total: (1, 3, 1) — element=1, class/attr=3, id=1.
	// We verify structurally (selector substring) since CSS test
	// harnesses don't have a built-in specificity calculator.
	wantSelectorParts := []string{
		"body",
		".gmap-mode",
		`[data-strategic-authority-rail="hidden"]`,
		"#gmap-details",
		".gmap-right-rail",
	}
	for _, part := range wantSelectorParts {
		if !strings.Contains(`body.gmap-mode[data-strategic-authority-rail="hidden"] #gmap-details.gmap-right-rail`, part) {
			t.Errorf("D37av2-rail-gate: selector specificity check — part %q is required so the selector reaches at least (1,3,1)", part)
		}
	}
	// Negative pin — no global #gmap-details hide.
	if strings.Contains(css, "#gmap-details {\n    display: none;") {
		t.Error("D37av2-rail-gate: shell.css must NOT introduce a global `#gmap-details { display: none }` rule — only the Authority-attribute-gated rule")
	}
}

// ── 4. CSS reclaims width for Authority canvas ───────────────────────

// TestExplorer_D37av2AuthorityRailGate_CssReclaimsWidth pins the
// shell-width override rules for the Authority right-rail gate.
// Mirrors the D37aq Context pattern (`right: 0` / `margin-right: 0`
// on .shell-header / .shell-footer / .shell-main) so the Authority
// graph canvas reclaims the width the rail used to consume.
func TestExplorer_D37av2AuthorityRailGate_CssReclaimsWidth(t *testing.T) {
	css := d37av2RailGateRead(t, d37av2RailGateShellCssPath)

	authorityOverrides := []string{
		`body[data-strategic-authority-rail="hidden"].gmap-mode .shell-header { right: 0; }`,
		`body[data-strategic-authority-rail="hidden"].gmap-mode .shell-footer { right: 0; }`,
		`body[data-strategic-authority-rail="hidden"].gmap-mode .shell-main   { margin-right: 0; }`,
		`body[data-strategic-authority-rail="hidden"].gmap-mode.inspector-collapsed .shell-header { right: 0; }`,
		`body[data-strategic-authority-rail="hidden"].gmap-mode.inspector-collapsed .shell-footer { right: 0; }`,
		`body[data-strategic-authority-rail="hidden"].gmap-mode.inspector-collapsed .shell-main   { margin-right: 0; }`,
	}
	for _, want := range authorityOverrides {
		if !strings.Contains(css, want) {
			t.Errorf("D37av2-rail-gate: shell.css must declare Authority width override %q (Authority canvas reclaims right-rail width)", want)
		}
	}

	// D37aq Context width overrides must still exist (Context
	// isolation regression guard).
	contextOverrides := []string{
		`body[data-strategic-context-inspector="graph-pane"].gmap-mode .shell-header { right: 0; }`,
		`body[data-strategic-context-inspector="graph-pane"].gmap-mode .shell-footer { right: 0; }`,
		`body[data-strategic-context-inspector="graph-pane"].gmap-mode .shell-main   { margin-right: 0; }`,
	}
	for _, want := range contextOverrides {
		if !strings.Contains(css, want) {
			t.Errorf("D37av2-rail-gate: D37aq Context width override %q must remain unchanged (Context isolation)", want)
		}
	}
}

// ── 5. Fallback flag restores rail visibility (visibility-only) ──────

// TestExplorer_D37av2AuthorityRailGate_FallbackRestoresRailVisibility
// pins that the fallback flag prevents the body attribute from being
// set, the CSS hide rule depends on the attribute (not a flag-aware
// JS branch), and the physical rail DOM remains in index.html.
func TestExplorer_D37av2AuthorityRailGate_FallbackRestoresRailVisibility(t *testing.T) {
	js := d37av2RailGateRead(t, d37av2RailGateCanvasEdgePath)

	// The lifecycle helper guards on the flag — when the flag is
	// present, the AND short-circuits and the attribute is removed.
	if !strings.Contains(js, "_isAuthorityLensActive() && !_hasLegacyAuthorityRailFlag()") {
		t.Error("D37av2-rail-gate: lifecycle must AND the active-renderer check with the fallback-flag absence — flag present means attribute absent")
	}
	// Physical rail DOM remains in index.html.
	html := d37av2RailGateRead(t, d37av2RailGateIndexHtmlPath)
	if !strings.Contains(html, `<aside id="gmap-details"`) {
		t.Error("D37av2-rail-gate: index.html must still contain the physical right-rail <aside id=\"gmap-details\"> element — this tranche gates visibility, not code existence")
	}
	if !strings.Contains(html, `class="governance-map-details gmap-right-rail"`) {
		t.Error("D37av2-rail-gate: physical rail markup must retain its `governance-map-details gmap-right-rail` classes so the existing default `display: flex` rule applies under the fallback flag")
	}
}

// ── 6. Fallback restores VISIBILITY ONLY ─────────────────────────────

// TestExplorer_D37av2AuthorityRailGate_FallbackVisibilityOnly pins
// that the fallback flag does not re-introduce any decommissioned
// content. Under ?legacyAuthorityRail=1 the rail is visible, but
// only the Inspector slot is registered; Diagnostics, Posture & Help,
// Layer chips, Legend, and Help framing remain decommissioned (their
// dispatch identifiers must be absent from view.js regardless of
// flag state).
func TestExplorer_D37av2AuthorityRailGate_FallbackVisibilityOnly(t *testing.T) {
	view := d37av2RailGateRead(t, d37av2RailGateViewPath)

	// Inspector slot remains registered.
	if !strings.Contains(view, "id: 'inspector', label: 'Inspector'") {
		t.Error("D37av2-rail-gate: Authority drawer must still register the Inspector slot (the only remaining slot post-prereq)")
	}

	// Decommissioned content must NOT return under any state. These
	// negative pins are byte-identical to the prereq-tranche pins —
	// they are re-asserted here as part of the rail-gate contract.
	bannedView := []string{
		"id: 'evidence'",
		"id: 'config'",
		"label: 'Diagnostics'",
		"label: 'Posture & Help'",
		"_authorityRenderDiagnosticsIntoDrawer",
		"_authorityRenderPostureAndHelpIntoDrawer",
		"data-authority-diagnostic-summary",
		"data-authority-diagnostics",
		"data-authority-surface-posture",
		"data-authority-summary-mount",
		"data-authority-layer-chips",
		"data-authority-legend",
		"overlays.renderLegendInto",
		"overlays.renderSummaryInto",
		"overlays.renderLayerChipsInto",
	}
	for _, banned := range bannedView {
		if strings.Contains(view, banned) {
			t.Errorf("D37av2-rail-gate: authority-graph-view.js must not contain decommissioned right-rail content %q — fallback flag is visibility-only", banned)
		}
	}
}

// ── 7. Fallback does not re-register dropped tabs ────────────────────

// TestExplorer_D37av2AuthorityRailGate_FallbackDoesNotReregister pins
// that the Authority registerLens call is byte-identical between
// flag-present and flag-absent code paths. There must be NO
// conditional logic that branches on the flag and re-adds tabs.
// The fallback is CSS / body-attribute scope only.
func TestExplorer_D37av2AuthorityRailGate_FallbackDoesNotReregister(t *testing.T) {
	view := d37av2RailGateRead(t, d37av2RailGateViewPath)

	// No reference to the flag in view.js — Authority's drawer
	// registration must be unaware of the flag.
	if strings.Contains(view, "legacyAuthorityRail") {
		t.Error("D37av2-rail-gate: authority-graph-view.js must not reference the legacyAuthorityRail flag — registerLens is byte-identical across flag states")
	}
	// No conditional registration logic.
	if strings.Contains(view, "_hasLegacyAuthorityRailFlag") {
		t.Error("D37av2-rail-gate: authority-graph-view.js must not call the flag parser — registration must not branch on flag state")
	}

	// The flag parser lives in the canvas-edge module only.
	js := d37av2RailGateRead(t, d37av2RailGateCanvasEdgePath)
	if !strings.Contains(js, "function _hasLegacyAuthorityRailFlag()") {
		t.Error("D37av2-rail-gate: flag parser must live in authority-canvas-edge-tabs.js (the strategic Authority pane module)")
	}
}

// ── 8. Surface Posture single home ───────────────────────────────────

// TestExplorer_D37av2AuthorityRailGate_SurfacePostureSingleHome pins
// that Surface Posture renders ONLY in the Workbench Posture tab,
// regardless of fallback flag state. No [data-authority-surface-
// posture] container under #gmap-details receives authoritative
// content under any state.
func TestExplorer_D37av2AuthorityRailGate_SurfacePostureSingleHome(t *testing.T) {
	view := d37av2RailGateRead(t, d37av2RailGateViewPath)
	if strings.Contains(view, "data-authority-surface-posture") {
		t.Error("D37av2-rail-gate: authority-graph-view.js must not stamp [data-authority-surface-posture] — Workbench Posture tab is the sole authoritative home (per prereq tranche)")
	}

	// Workbench Posture tab still stamps the container.
	wb := d37av2RailGateRead(t, d37av2RailGateWorkbenchPath)
	if !strings.Contains(wb, "data-authority-surface-posture") {
		t.Error("D37av2-rail-gate: Workbench Posture tab must continue to stamp [data-authority-surface-posture] container (sole authoritative Surface Posture home)")
	}
	if !strings.Contains(wb, "function _renderPosture()") {
		t.Error("D37av2-rail-gate: Workbench Posture tab renderer _renderPosture() must remain")
	}
}

// ── 9. Context isolation ─────────────────────────────────────────────

// TestExplorer_D37av2AuthorityRailGate_ContextIsolation pins that the
// D37aq Context-gating mechanism remains untouched and that the
// Authority and Context attributes / flags are distinct.
func TestExplorer_D37av2AuthorityRailGate_ContextIsolation(t *testing.T) {
	// D37aq Context flag + attribute remain in context-side modules.
	contextBridge := d37av2RailGateRead(t, d37av2RailGateContextBridge)
	if !strings.Contains(contextBridge, "LEGACY_CONTEXT_INSPECTOR_QUERY_PARAM = 'legacyContextInspector'") {
		t.Error("D37av2-rail-gate: D37aq Context flag declaration must remain unchanged in context-selection-bridge.js")
	}
	contextPane := d37av2RailGateRead(t, d37av2RailGateContextPane)
	if !strings.Contains(contextPane, "STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR  = 'data-strategic-context-inspector'") {
		t.Error("D37av2-rail-gate: D37aq Context body attribute declaration must remain unchanged in context-selected-object-pane.js")
	}

	// Authority does not reference Context's flag or attribute.
	authorityJS := d37av2RailGateRead(t, d37av2RailGateCanvasEdgePath)
	if strings.Contains(authorityJS, "legacyContextInspector") {
		t.Error("D37av2-rail-gate: authority-canvas-edge-tabs.js must not reference Context's `legacyContextInspector` flag")
	}
	if strings.Contains(authorityJS, "data-strategic-context-inspector") {
		t.Error("D37av2-rail-gate: authority-canvas-edge-tabs.js must not reference Context's `data-strategic-context-inspector` attribute")
	}

	// Context modules must not reference the Authority flag or
	// attribute.
	for _, path := range []string{d37av2RailGateContextBridge, d37av2RailGateContextPane} {
		ctx := d37av2RailGateRead(t, path)
		if strings.Contains(ctx, "legacyAuthorityRail") {
			t.Errorf("D37av2-rail-gate: Context module %s must not reference the Authority `legacyAuthorityRail` flag", path)
		}
		if strings.Contains(ctx, "data-strategic-authority-rail") {
			t.Errorf("D37av2-rail-gate: Context module %s must not reference the Authority `data-strategic-authority-rail` attribute", path)
		}
	}
}

// ── 10. Replacement surfaces remain intact ───────────────────────────

// TestExplorer_D37av2AuthorityRailGate_ReplacementSurfacesRemain pins
// that the strategic Authority surfaces — canvas-edge tabs, Workbench
// (with Posture tab), OSS Help — remain operator-accessible regardless
// of the rail-gate state.
func TestExplorer_D37av2AuthorityRailGate_ReplacementSurfacesRemain(t *testing.T) {
	// Canvas-edge tabs DOM skeleton remains in index.html.
	html := d37av2RailGateRead(t, d37av2RailGateIndexHtmlPath)
	if !strings.Contains(html, `data-authority-canvas-edge-tabs`) {
		t.Error("D37av2-rail-gate: canvas-edge tabs DOM (`data-authority-canvas-edge-tabs`) must remain in index.html")
	}

	// Workbench Posture tab button remains in index.html.
	if !strings.Contains(html, `data-authority-tab="posture"`) {
		t.Error("D37av2-rail-gate: Workbench Posture tab button (`data-authority-tab=\"posture\"`) must remain in index.html")
	}

	// OSS Help button remains in index.html.
	if !strings.Contains(html, `id="gmap-help-button"`) {
		t.Error("D37av2-rail-gate: toolbar Help button (#gmap-help-button) must remain (OSS Help is the Help surface)")
	}

	// Workbench TAB_IDS still includes 'posture' in the required
	// position (regression guard against accidental tab reorder).
	wb := d37av2RailGateRead(t, d37av2RailGateWorkbenchPath)
	if !strings.Contains(wb, "Object.freeze(['overview', 'posture', 'fail-mode', 'escalation', 'grants', 'evidence'])") {
		t.Error("D37av2-rail-gate: Workbench TAB_IDS must remain `['overview', 'posture', 'fail-mode', 'escalation', 'grants', 'evidence']`")
	}
}

// ── 11. Legacy rail code preserved for fallback ──────────────────────

// TestExplorer_D37av2AuthorityRailGate_LegacyRailCodePreserved pins
// that this gating tranche does not delete any legacy rail code.
// #gmap-details remains, Authority Inspector slot registration
// remains, authority-graph-inspector.js still writes selected-object
// fields, and graph-drawer.js / graph-inspector.js remain present.
func TestExplorer_D37av2AuthorityRailGate_LegacyRailCodePreserved(t *testing.T) {
	html := d37av2RailGateRead(t, d37av2RailGateIndexHtmlPath)
	if !strings.Contains(html, `<aside id="gmap-details"`) {
		t.Error("D37av2-rail-gate: #gmap-details rail must remain in index.html")
	}

	// Authority Inspector slot remains registered.
	view := d37av2RailGateRead(t, d37av2RailGateViewPath)
	if !strings.Contains(view, "drawer.registerLens('authority'") {
		t.Error("D37av2-rail-gate: Authority drawer lens registration must remain (the Inspector slot is registered through this call)")
	}
	if !strings.Contains(view, "id: 'inspector', label: 'Inspector'") {
		t.Error("D37av2-rail-gate: Authority Inspector slot registration must remain (id: 'inspector', label: 'Inspector')")
	}

	// Authority inspector module remains and still writes selected-
	// object fields.
	insp := d37av2RailGateRead(t, d37av2RailGateInspectorPath)
	if !strings.Contains(insp, "window.MIDASExplorerGraph.authorityInspector") {
		t.Error("D37av2-rail-gate: authority-graph-inspector.js must still publish window.MIDASExplorerGraph.authorityInspector")
	}
	// Inspector frame-setters still called.
	for _, want := range []string{
		"insp.setName",
		"insp.setFields",
		"insp.setActions",
		"insp.setInlineActions",
	} {
		if !strings.Contains(insp, want) {
			t.Errorf("D37av2-rail-gate: authority-graph-inspector.js must still call %q (Inspector slot remains operator-visible behind fallback)", want)
		}
	}

	// Shared drawer + inspector modules remain.
	for _, path := range []string{
		"explorer/assets/js/graph/graph-drawer.js",
		"explorer/assets/js/graph/graph-inspector.js",
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("D37av2-rail-gate: shared module %s must remain present (this tranche gates visibility, not code existence): %v", path, err)
		}
	}
}

// ── 12. No decommissioned content returns under any flag state ───────

// TestExplorer_D37av2AuthorityRailGate_NoDecommissionedContentReturns
// pins that Authority's drawer registration still excludes all the
// decommissioned tabs and mounts regardless of fallback flag state.
// This is the broadest negative-pin defense against accidental
// reintroduction during the rail-gate work.
func TestExplorer_D37av2AuthorityRailGate_NoDecommissionedContentReturns(t *testing.T) {
	view := d37av2RailGateRead(t, d37av2RailGateViewPath)

	// All decommissioned identifiers must be absent.
	banned := []string{
		// Slot ids
		"id: 'evidence'",
		"id: 'config'",
		// Labels
		"label: 'Diagnostics'",
		"label: 'Posture & Help'",
		// Render functions
		"_authorityRenderDiagnosticsIntoDrawer",
		"_authorityRenderPostureAndHelpIntoDrawer",
		// Drawer-side mount data attributes
		"data-authority-diagnostic-summary",
		"data-authority-diagnostics",
		"data-authority-surface-posture",
		"data-authority-summary-mount",
		"data-authority-layer-chips",
		"data-authority-legend",
		// Overlays dispatch calls
		"overlays.renderLegendInto",
		"overlays.renderSummaryInto",
		"overlays.renderLayerChipsInto",
	}
	for _, b := range banned {
		if strings.Contains(view, b) {
			t.Errorf("D37av2-rail-gate: authority-graph-view.js must not restore decommissioned right-rail identifier %q under any flag state", b)
		}
	}
}
