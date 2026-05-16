package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl5f_test.go — D33a-spike-2g-impl-5f.
//
// Inspector ownership cleanup. The Authority Inspector tab now owns
// ONLY the selected object's identity + direct attributes (Kind /
// ID / Label / Connected edges + per-kind specific fields +
// collapsed Technical details). Per-node diagnostics are owned by
// the Diagnostics tab (`authority-diagnostics-panel.js`); per-
// surface posture is owned by the Posture & Help tab
// (`authority-surface-posture-panel.js`). The Inspector no longer
// composes a governance overlay; the retired helpers
// (`_buildOverlayHTML`, `_diagnosticsForNode`, `_postureForSurface`,
// `_axisAttr`) and the `setGovernance(_buildOverlayHTML(...))` call
// must be gone from the inspector module.
//
// The dead "Map summary" `.gmap-details-section` in the Inspector
// pane is collapsed via a CSS `:has()` rule when its summary slot
// is empty — the DOM scaffold itself is preserved because the
// Context Graph lens still populates `#gmap-details-summary` via
// `setSummary`.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style.

const (
	d33aSpike2gImpl5fInspectorPath = "explorer/assets/js/graph/authority/authority-graph-inspector.js"
	d33aSpike2gImpl5fDiagPanelPath = "explorer/assets/js/graph/authority/authority-diagnostics-panel.js"
	d33aSpike2gImpl5fPosturePath   = "explorer/assets/js/graph/authority/authority-surface-posture-panel.js"
	d33aSpike2gImpl5fOverlaysPath  = "explorer/assets/js/graph/authority/authority-graph-overlays.js"
	d33aSpike2gImpl5fViewJsPath    = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl5fPocPath       = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl5fCssPath       = "explorer/assets/css/governance-map.css"
	d33aSpike2gImpl5fIndexPath     = "explorer/index.html"
)

func d33aSpike2gImpl5fRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5f: cannot read %s: %v", path, err)
	}
	return string(b)
}

// ── 1. Inspector no longer renders governance overlay ────────────────

// TestExplorer_D33aSpike2gImpl5f_InspectorNoLongerRendersGovernanceOverlay
// pins the core impl-5f contract: the Authority Inspector module no
// longer composes diagnostics/posture overlay HTML, and the
// `_renderInto` path no longer asks the shared inspector frame to
// inject overlay HTML into the governance slot.
func TestExplorer_D33aSpike2gImpl5f_InspectorNoLongerRendersGovernanceOverlay(t *testing.T) {
	js := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fInspectorPath)
	for _, gone := range []string{
		"function _buildOverlayHTML(",
		"function _diagnosticsForNode(",
		"function _postureForSurface(",
		"function _axisAttr(",
		"authority-inspector-section-diagnostics",
		"authority-inspector-section-posture",
		"authority-inspector-diagnostics-list",
		"authority-inspector-posture-row",
		"authority-inspector-posture-key",
		"authority-inspector-posture-val",
	} {
		if strings.Contains(js, gone) {
			t.Errorf("D33a-spike-2g-impl-5f: inspector must no longer carry overlay symbol %q (impl-5f retires the Inspector governance overlay)", gone)
		}
	}
	// `_lastAuthorityProjection` was read ONLY by `_buildOverlayHTML`
	// — after impl-5f the inspector must not touch it. The PoC and
	// workbench still write/read it for their own purposes; only
	// the inspector path is in scope here.
	if strings.Contains(js, "_lastAuthorityProjection") {
		t.Error("D33a-spike-2g-impl-5f: inspector must no longer read window.MIDASExplorerGraph._lastAuthorityProjection (its only consumer was the retired overlay)")
	}
	// The `setGovernance` call must not pass overlay HTML.
	if strings.Contains(js, "setGovernance(_buildOverlayHTML") {
		t.Error("D33a-spike-2g-impl-5f: inspector must not call setGovernance(_buildOverlayHTML(...)) — the Inspector tab no longer owns governance overlay content")
	}
	// The call should still exist as `setGovernance('')` so that
	// selection changes clear any prior Authority content.
	if !strings.Contains(js, "insp.setGovernance('')") {
		t.Error("D33a-spike-2g-impl-5f: inspector should call setGovernance('') to clear any prior Authority overlay content on each selection")
	}
}

// ── 2. Inspector preserves selected-node identity fields ─────────────

// TestExplorer_D33aSpike2gImpl5f_InspectorPreservesSelectedNodeFields
// pins that the selected-node identity model from impl-5e survives
// — Primary block (Kind / ID / Label / Connected edges) + per-kind
// `specific` and `technical` partitions remain unchanged.
func TestExplorer_D33aSpike2gImpl5f_InspectorPreservesSelectedNodeFields(t *testing.T) {
	js := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fInspectorPath)
	for _, want := range []string{
		"function _primaryRows(",
		"['Kind',",
		"['ID',",
		"['Label',",
		"['Connected edges',",
		"function _renderFieldRowsHtml(",
		"function _renderTechnicalDetailsHtml(",
		// Per-kind formatters all return { specific, technical }.
		"function _formatBusinessService(",
		"function _formatDecisionSurface(",
		"function _formatAuthorityProfile(",
		"function _formatAuthorityGrant(",
		"function _formatAgent(",
		"function _formatFailModePolicy(",
		"function _formatEscalationTarget(",
		"specific:",
		"technical:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-5f: inspector must preserve impl-5e selected-node model symbol %q", want)
		}
	}
}

// ── 3. Diagnostics tab untouched ─────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5f_DiagnosticsTabUntouched pins that
// the Diagnostics tab module still renders the diagnostics list
// with severity / kind / message / node_refs. impl-5f is an
// ownership cleanup; the Diagnostics tab is explicitly out of
// scope and must not regress.
func TestExplorer_D33aSpike2gImpl5f_DiagnosticsTabUntouched(t *testing.T) {
	js := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fDiagPanelPath)
	for _, want := range []string{
		"authorityDiagnosticsPanel",
		"diagnostics",
		"severity",
		"node_refs",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-5f: diagnostics panel must still reference %q (impl-5f did not redesign Diagnostics)", want)
		}
	}
	// The tab registration must still bind the diagnostics renderer
	// to the right-drawer `evidence` slot with label "Diagnostics".
	view := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fViewJsPath)
	for _, want := range []string{
		"id: 'evidence'",
		"label: 'Diagnostics'",
		"_authorityRenderDiagnosticsIntoDrawer",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("D33a-spike-2g-impl-5f: view must still register the Diagnostics tab (looking for %q)", want)
		}
	}
}

// ── 4. Posture & Help tab untouched ──────────────────────────────────

// TestExplorer_D33aSpike2gImpl5f_PostureHelpTabUntouched pins that
// the Posture & Help tab still mounts the surface posture panel +
// summary / layer chips / legend overlays. Future tranches may
// redesign this tab; impl-5f must not.
func TestExplorer_D33aSpike2gImpl5f_PostureHelpTabUntouched(t *testing.T) {
	view := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fViewJsPath)
	for _, want := range []string{
		"id: 'config'",
		"label: 'Posture & Help'",
		"_authorityRenderPostureAndHelpIntoDrawer",
		"authoritySurfacePosturePanel",
		"renderSummaryInto",
		"renderLayerChipsInto",
		"renderLegendInto",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("D33a-spike-2g-impl-5f: view must still register the Posture & Help tab (looking for %q)", want)
		}
	}
	// The posture panel module must still exist and surface all six
	// posture axes.
	panel := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fPosturePath)
	for _, axis := range []string{
		"authority_status",
		"profile_status",
		"grant_status",
		"agent_status",
		"fail_mode_policy_status",
		"escalation_status",
	} {
		if !strings.Contains(panel, axis) {
			t.Errorf("D33a-spike-2g-impl-5f: posture panel must still surface axis %q", axis)
		}
	}
	// Overlays module must still exist.
	overlays := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fOverlaysPath)
	if !strings.Contains(overlays, "authorityOverlays") {
		t.Error("D33a-spike-2g-impl-5f: authority overlays module must still register window.MIDASExplorerGraph.authorityOverlays")
	}
}

// ── 5. Dead Map Summary heading suppressed when empty ────────────────

// TestExplorer_D33aSpike2gImpl5f_InspectorDoesNotRenderEmptyMapSummary
// pins that the "Map summary" `.gmap-details-section` collapses to
// zero height when its `#gmap-details-summary` slot is empty. The
// scaffold itself stays in the DOM so the Context Graph lens can
// continue to populate it.
func TestExplorer_D33aSpike2gImpl5f_InspectorDoesNotRenderEmptyMapSummary(t *testing.T) {
	css := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fCssPath)
	if !strings.Contains(css, "#gmap-details-summary:empty") {
		t.Error("D33a-spike-2g-impl-5f: governance-map.css must hide the empty Map summary scaffold via a `:has(> #gmap-details-summary:empty)` rule")
	}
	if !strings.Contains(css, ".gmap-details-section:has(> #gmap-details-summary:empty)") {
		t.Error("D33a-spike-2g-impl-5f: governance-map.css must scope the empty-Map-summary hide to the parent `.gmap-details-section` (so the heading collapses with the slot)")
	}
	// DOM scaffold must remain so Context Graph's setSummary call
	// still has a mount point.
	idx := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fIndexPath)
	if !strings.Contains(idx, `<div id="gmap-details-summary"></div>`) {
		t.Error("D33a-spike-2g-impl-5f: index.html must keep the #gmap-details-summary scaffold (Context Graph lens populates it via setSummary)")
	}
	// Inspector must explicitly clear the slot on every selection
	// so the `:has(:empty)` rule fires.
	js := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fInspectorPath)
	if !strings.Contains(js, "insp.setSummary([])") {
		t.Error("D33a-spike-2g-impl-5f: Authority inspector must clear #gmap-details-summary via setSummary([]) on each selection")
	}
}

// ── 6. Field layout preserved (impl-5c invariants) ───────────────────

// TestExplorer_D33aSpike2gImpl5f_FieldLayoutPreserved pins that the
// impl-5c value-dominant grid + min-width: 0 + overflow-wrap
// invariants survive the impl-5f cleanup unchanged. Removing the
// overlay must not regress the field-row layout.
func TestExplorer_D33aSpike2gImpl5f_FieldLayoutPreserved(t *testing.T) {
	css := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fCssPath)
	// Bound the canonical `.gmap-details-row` rule body so an
	// unrelated compound selector with the same prefix can't
	// accidentally satisfy the contains check.
	body := d33aSpike2gImpl5fBoundRule(t, css, ".gmap-details-row")
	for _, want := range []string{
		"grid-template-columns: minmax(96px, 120px) minmax(0, 1fr)",
		"padding: 3px 0",
		"min-width: 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-5f: .gmap-details-row must keep impl-5c value-dominant grid contract — missing %q", want)
		}
	}
	valBody := d33aSpike2gImpl5fBoundRule(t, css, ".gmap-details-val")
	for _, want := range []string{
		"min-width: 0",
		"overflow-wrap: anywhere",
	} {
		if !strings.Contains(valBody, want) {
			t.Errorf("D33a-spike-2g-impl-5f: .gmap-details-val must keep impl-5c wrapping contract — missing %q", want)
		}
	}
}

// ── 7. Header simplification preserved (impl-5d invariants) ──────────

// TestExplorer_D33aSpike2gImpl5f_HeaderSimplificationPreserved pins
// that the impl-5d header simplifications survive — Inspector tab
// label, top X close hidden, `|->` close glyph in the title row.
func TestExplorer_D33aSpike2gImpl5f_HeaderSimplificationPreserved(t *testing.T) {
	view := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fViewJsPath)
	if !strings.Contains(view, "label: 'Inspector'") {
		t.Error("D33a-spike-2g-impl-5f: view must still register the Inspector tab with label 'Inspector'")
	}
	idx := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fIndexPath)
	if !strings.Contains(idx, `class="gmap-details-name-row"`) {
		t.Error("D33a-spike-2g-impl-5f: Inspector pane must keep the impl-5d `.gmap-details-name-row` title row")
	}
	if !strings.Contains(idx, `class="gmap-details-name-close"`) {
		t.Error("D33a-spike-2g-impl-5f: Inspector pane must keep the impl-5d `.gmap-details-name-close` close glyph")
	}
	if !strings.Contains(idx, `|-&gt;`) {
		t.Error("D33a-spike-2g-impl-5f: Inspector close glyph must remain `|->` (HTML-encoded `|-&gt;`)")
	}
	css := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fCssPath)
	headerBody := d33aSpike2gImpl5fBoundRule(t, css, ".gmap-right-rail-header")
	if !strings.Contains(headerBody, "display: none") {
		t.Error("D33a-spike-2g-impl-5f: `.gmap-right-rail-header` must remain hidden at the base rule (impl-5d invariant)")
	}
}

// ── 8. PoC inspector still present ───────────────────────────────────

// TestExplorer_D33aSpike2gImpl5f_PocInspectorStillPresent pins that
// the PoC inspector aside (`.cytoscape-poc-inspector`) and its
// `_renderInspector` entry point still exist. The PoC inspector
// remains until the MIDAS Inspector pane is visually accepted.
func TestExplorer_D33aSpike2gImpl5f_PocInspectorStillPresent(t *testing.T) {
	js := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fPocPath)
	for _, want := range []string{
		"function _renderInspector(",
		"cytoscape-poc-inspector",
		"cytoscape-poc-inspector-body",
		"cytoscape-poc-inspector-fields",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-5f: PoC inspector aside must remain — missing %q", want)
		}
	}
}

// ── 9. Card rendering unchanged ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5f_CardRenderingUnchanged pins that
// the authority-thin-card-v1 theme and its strategic-symbol
// approach remain unchanged by impl-5f. The cleanup is right-drawer
// only; graph cards must not regress.
func TestExplorer_D33aSpike2gImpl5f_CardRenderingUnchanged(t *testing.T) {
	js := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fPocPath)
	if !strings.Contains(js, "authority-thin-card-v1") {
		t.Error("D33a-spike-2g-impl-5f: authority-thin-card-v1 theme must remain")
	}
	// Negative pin: the thin-card single-line contract must not
	// have regrown a second `name-row`.
	if strings.Contains(js, "authority-card-name-row-2") {
		t.Error("D33a-spike-2g-impl-5f: thin card must remain single-line — no second name row")
	}
}

// ── 10. Carrier DOM preserved ───────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5f_CarrierDomPreserved pins the
// impl-5/5a carrier-DOM bridge (hidden `.gmap-node` carriers under
// `#gmap-canvas`) and the selection routing it enables remain
// intact after impl-5f.
func TestExplorer_D33aSpike2gImpl5f_CarrierDomPreserved(t *testing.T) {
	js := d33aSpike2gImpl5fRead(t, d33aSpike2gImpl5fPocPath)
	for _, want := range []string{
		"function _renderInspectorCarriers(",
		"function _clearInspectorCarriers(",
		"data-cytoscape-poc-carrier",
		"cytoscape-poc-inspector-carrier",
		"function _detailsForCarrier(",
		"_renderInspectorCarriers(elements)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-5f: carrier DOM contract must remain — missing %q", want)
		}
	}
}

// ── Helper: bound a CSS rule body ────────────────────────────────────

// d33aSpike2gImpl5fBoundRule returns the body of the first CSS rule
// whose opener is exactly `selector + " {"`. Bounding by the
// `selector + " {"` literal avoids matching compound selectors
// (e.g. `.gmap-details-technical-body .gmap-details-row { ... }`)
// when checking the canonical rule's contract.
func d33aSpike2gImpl5fBoundRule(t *testing.T, css, selector string) string {
	t.Helper()
	opener := selector + " {"
	start := strings.Index(css, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-5f: CSS opener %q missing", opener)
	}
	tail := css[start:]
	end := strings.Index(tail, "}")
	if end < 0 {
		t.Fatalf("D33a-spike-2g-impl-5f: could not bound CSS rule body for %q", opener)
	}
	return tail[:end]
}
