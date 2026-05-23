package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl5d_test.go — D33a-spike-2g-impl-5d.
//
// Inspector header row correction:
//   1. Vertical drawer tab label is `Inspector` (the earlier
//      rename to `Showcase` is reverted).
//   2. The standalone top X close action above the inspector
//      content is hidden — `.gmap-right-rail-header` is
//      `display: none` in the base rule.
//   3. The selected-node title `#gmap-details-name` is wrapped in
//      a `.gmap-details-name-row` flex container with a right-
//      aligned `|->` close/collapse button
//      (`.gmap-details-name-close`).
//   4. The new close button shares the existing
//      `closeGmapRightRail` handler via a widened selector
//      (`[data-rail-close]`) so the in-content glyph triggers
//      the same canonical drawer close as the (now hidden)
//      original X.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl5dCssPath       = "explorer/assets/css/governance-map.css"
	d33aSpike2gImpl5dViewJsPath    = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl5dIndexHtmlPath = "explorer/index.html"
	d33aSpike2gImpl5dPocPath       = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

func d33aSpike2gImpl5dRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5d: cannot read %s: %v", path, err)
	}
	return string(b)
}

// d33aSpike2gImpl5dRule bounds a CSS rule body so per-rule
// assertions stay scoped.
func d33aSpike2gImpl5dRule(t *testing.T, css, selector string) string {
	t.Helper()
	start := strings.Index(css, selector+" {")
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-5d: CSS rule %q missing", selector)
	}
	end := strings.Index(css[start:], "}")
	if end < 0 {
		t.Fatalf("D33a-spike-2g-impl-5d: could not bound rule %q", selector)
	}
	return css[start : start+end]
}

// ── 1. Tab label is Inspector ────────────────────────────────────────

func TestExplorer_D33aSpike2gImpl5d_TabLabelIsInspector(t *testing.T) {
	view := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dViewJsPath)
	if !strings.Contains(view, "id: 'inspector', label: 'Inspector'") {
		t.Error("D33a-spike-2g-impl-5d: Authority drawer-lens inspector tab must declare `id: 'inspector', label: 'Inspector'`")
	}
	if strings.Contains(view, "id: 'inspector', label: 'Showcase'") {
		t.Error("D33a-spike-2g-impl-5d: Authority drawer-lens must no longer declare `label: 'Showcase'`")
	}
	// D37av2-prereq-authority-rail-decommission-content-impl dropped
	// the Diagnostics + Posture & Help slot registrations from
	// authority-graph-view.js. Only the inspector slot remains; the
	// previous positive pins on `label: 'Diagnostics'` and
	// `label: 'Posture & Help'` are flipped to negative pins.
	for _, banned := range []string{
		"label: 'Diagnostics'",
		"label: 'Posture & Help'",
	} {
		if strings.Contains(view, banned) {
			t.Errorf("D37av2-prereq: Authority drawer registration must not declare %q — slot decommissioned", banned)
		}
	}
}

// ── 2. Standalone top X close action removed (header hidden) ─────────

func TestExplorer_D33aSpike2gImpl5d_RemovesStandaloneTopCloseX(t *testing.T) {
	css := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dCssPath)
	headerBody := d33aSpike2gImpl5dRule(t, css, ".gmap-right-rail-header")
	if !strings.Contains(headerBody, "display: none") {
		t.Error("D33a-spike-2g-impl-5d: base `.gmap-right-rail-header` rule must declare `display: none` so the standalone top X close action is hidden")
	}
	// The pre-impl-5d `display: flex` is the artefact being
	// removed; pin its absence as a regression guard.
	if strings.Contains(headerBody, "display: flex") {
		t.Error("D33a-spike-2g-impl-5d: base `.gmap-right-rail-header` must not still declare `display: flex` (replaced by `display: none`)")
	}
}

// ── 3. Selected-node name is the first usable row ────────────────────

func TestExplorer_D33aSpike2gImpl5d_SelectedNodeNameIsFirstUsableRow(t *testing.T) {
	html := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dIndexHtmlPath)

	// The static `Selected node` subheading (the legacy
	// `<div class="gmap-details-title">Selected node</div>` sibling
	// inside the inspector panel) must be hidden via the existing
	// scoped CSS rule, OR removed from index.html. We accept either
	// path so a future tranche that physically removes the line
	// passes this test.
	css := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dCssPath)
	const cssHide = "#gmap-rail-panel-inspector > .gmap-details-section:first-child > .gmap-details-title {"
	htmlHasStatic := strings.Contains(html, `<div class="gmap-details-title">Selected node</div>`)
	if htmlHasStatic && !strings.Contains(css, cssHide) {
		t.Error("D33a-spike-2g-impl-5d: the static `Selected node` subheading is still in index.html but the scoped CSS hide rule is missing")
	}

	// The shared rail title text is hidden too (impl-5d-prev rule);
	// only the in-content selected-node name should now read as a
	// heading.
	titleBody := d33aSpike2gImpl5dRule(t, css, "#gmap-right-rail-title")
	if !strings.Contains(titleBody, "display: none") {
		t.Error("D33a-spike-2g-impl-5d: `#gmap-right-rail-title` must remain `display: none`")
	}

	// And the selected-node name container must still be present
	// in the inspector panel.
	if !strings.Contains(html, `id="gmap-details-name"`) {
		t.Error("D33a-spike-2g-impl-5d: `#gmap-details-name` selected-node name container must remain in index.html")
	}
}

// ── 4. Title row reserves the right-aligned close slot ───────────────

func TestExplorer_D33aSpike2gImpl5d_TitleRowHasRightAlignedCloseSlot(t *testing.T) {
	html := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dIndexHtmlPath)

	// The selected-node name must sit inside the new title-row
	// container.
	if !strings.Contains(html, `class="gmap-details-name-row"`) {
		t.Error("D33a-spike-2g-impl-5d: index.html must declare a `.gmap-details-name-row` container wrapping the selected-node name + close glyph")
	}
	// The close glyph itself.
	if !strings.Contains(html, `class="gmap-details-name-close"`) {
		t.Error("D33a-spike-2g-impl-5d: index.html must declare a `.gmap-details-name-close` button inside the title row")
	}
	// The `|->` glyph (`&gt;` encodes `>`).
	if !strings.Contains(html, "|-&gt;") {
		t.Error("D33a-spike-2g-impl-5d: close button must carry the `|->` glyph text content (encoded as `|-&gt;`)")
	}
	// Accessible label.
	if !strings.Contains(html, `aria-label="Collapse inspector"`) {
		t.Error("D33a-spike-2g-impl-5d: close button must declare `aria-label=\"Collapse inspector\"`")
	}
	// Button-typed control.
	if !strings.Contains(html, `<button type="button" class="gmap-details-name-close"`) {
		t.Error("D33a-spike-2g-impl-5d: close button must be a `<button type=\"button\">`")
	}

	// CSS layout — row uses flex/grid with right-aligned slot, the
	// close button is non-shrinking, and both name + button can
	// share a baseline without forcing premature title wrap.
	css := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dCssPath)
	rowBody := d33aSpike2gImpl5dRule(t, css, ".gmap-details-name-row")
	if !strings.Contains(rowBody, "display: flex") && !strings.Contains(rowBody, "display: grid") {
		t.Error("D33a-spike-2g-impl-5d: `.gmap-details-name-row` must use display: flex (or grid)")
	}
	if !strings.Contains(rowBody, "justify-content: space-between") {
		t.Error("D33a-spike-2g-impl-5d: `.gmap-details-name-row` must use `justify-content: space-between` to anchor the close glyph at the row's right edge")
	}
}

// ── 5. Close glyph reuses the existing drawer close handler ──────────

func TestExplorer_D33aSpike2gImpl5d_CloseSymbolUsesExistingDrawerCloseHandler(t *testing.T) {
	html := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dIndexHtmlPath)

	// The in-content close button carries `data-rail-close="true"`.
	if !strings.Contains(html, `data-rail-close="true"`) {
		t.Error("D33a-spike-2g-impl-5d: close button must carry `data-rail-close=\"true\"` so it is bound by the canonical drawer-close wiring")
	}

	// The wireGmapRightRailClose IIFE binds every `[data-rail-close]`
	// element to the existing `closeGmapRightRail` handler.
	if !strings.Contains(html, "wireGmapRightRailClose") {
		t.Fatal("D33a-spike-2g-impl-5d: wireGmapRightRailClose IIFE missing from index.html")
	}
	if !strings.Contains(html, `querySelectorAll('[data-rail-close]')`) {
		t.Error("D33a-spike-2g-impl-5d: wireGmapRightRailClose must query `[data-rail-close]` so the in-content glyph + the old X share the handler")
	}
	if !strings.Contains(html, "addEventListener('click', closeGmapRightRail)") {
		t.Error("D33a-spike-2g-impl-5d: wireGmapRightRailClose must bind `closeGmapRightRail` as the click handler")
	}
}

// ── 6. Field layout from impl-5c preserved ───────────────────────────

func TestExplorer_D33aSpike2gImpl5d_FieldLayoutFromImpl5cPreserved(t *testing.T) {
	css := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dCssPath)
	rowBody := d33aSpike2gImpl5dRule(t, css, ".gmap-details-row")
	if !strings.Contains(rowBody, "minmax(96px, 120px) minmax(0, 1fr)") {
		t.Error("D33a-spike-2g-impl-5d: `.gmap-details-row` must keep the impl-5c value-dominant grid")
	}
	valBody := d33aSpike2gImpl5dRule(t, css, ".gmap-details-val")
	if !strings.Contains(valBody, "min-width: 0") {
		t.Error("D33a-spike-2g-impl-5d: `.gmap-details-val` must keep `min-width: 0`")
	}
	if !strings.Contains(valBody, "overflow-wrap: anywhere") {
		t.Error("D33a-spike-2g-impl-5d: `.gmap-details-val` must keep `overflow-wrap: anywhere`")
	}
	panelBody := d33aSpike2gImpl5dRule(t, css, ".gmap-right-rail-panel")
	if !strings.Contains(panelBody, "overflow-x: hidden") {
		t.Error("D33a-spike-2g-impl-5d: `.gmap-right-rail-panel` must keep `overflow-x: hidden`")
	}
}

// ── 7. PoC inspector aside still present ─────────────────────────────

// TestExplorer_D33aSpike2gImpl5d_PocInspectorStillPresent — superseded
// by D33x-list-mode. Floating card retired; carrier contract remains.
func TestExplorer_D33aSpike2gImpl5d_PocInspectorStillPresent(t *testing.T) {
	js := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dPocPath)
	if strings.Contains(js, "function _renderInspector(node)") {
		t.Error("D33x-list-mode: floating PoC inspector aside must remain retired — found _renderInspector function")
	}
	for _, want := range []string{
		"_renderInspectorCarriers",
		"cytoscape-poc-inspector-carrier",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33x-list-mode: production right-drawer wiring must remain — missing %q", want)
		}
	}
}

// ── 8. Authority thin-card rendering unchanged ───────────────────────

func TestExplorer_D33aSpike2gImpl5d_CardRenderingUnchanged(t *testing.T) {
	js := d33aSpike2gImpl5dRead(t, d33aSpike2gImpl5dPocPath)
	if !strings.Contains(js, "'authority-thin-card-v1'") {
		t.Error("D33a-spike-2g-impl-5d: authority-thin-card-v1 must remain registered")
	}
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5d: thin-card branch missing")
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	branch := tail[:end]
	if !strings.Contains(branch, "_displayTitle(ele") {
		t.Error("D33a-spike-2g-impl-5d: thin-card branch must still bind label via _displayTitle (single-line)")
	}
	if strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-5d: thin-card branch must NOT reintroduce _displayCardLabel")
	}
	if !strings.Contains(branch, "icons.cytoscapeDataURI(syms[") {
		t.Error("D33a-spike-2g-impl-5d: strategic symbols must remain")
	}
}

// ── 9. Production graph unchanged in the spots impl-5d touched ───────

func TestExplorer_D33aSpike2gImpl5d_ProductionGraphUnaffected(t *testing.T) {
	const inspectorPath = "explorer/assets/js/graph/authority/authority-graph-inspector.js"
	inspector := d33aSpike2gImpl5dRead(t, inspectorPath)
	for _, banned := range []string{
		"cytoscape-poc-inspector-carrier",
		"data-cytoscape-poc-carrier",
		"_renderInspectorCarriers",
		"_clearInspectorCarriers",
		"gmap-details-name-row",
		"gmap-details-name-close",
		"authority-thin-card-v1",
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(inspector, banned) {
			t.Errorf("D33a-spike-2g-impl-5d: production inspector module must not reference %q", banned)
		}
	}
}
