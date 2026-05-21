package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl5c_test.go — D33a-spike-2g-impl-5c.
//
// MIDAS right-drawer field-layout + scrollbar correction.
// impl-5b's fixed `140px 1fr` row layout left long values cramped
// and produced both horizontal and vertical scrollbars in the
// normal selected-node inspector state. impl-5c switches the row
// to a value-dominant `minmax(96px, 120px) minmax(0, 1fr)` grid,
// adds `min-width: 0` to the value cell so it can shrink inside
// the grid track, and locks the rail panel to `overflow-x: hidden`
// so wrapping long values never opens a horizontal scrollbar.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl5cCssPath       = "explorer/assets/css/governance-map.css"
	d33aSpike2gImpl5cInspectorPath = "explorer/assets/js/graph/authority/authority-graph-inspector.js"
	d33aSpike2gImpl5cPocPath       = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl5cViewPath      = "explorer/assets/js/graph/authority/authority-graph-view.js"
)

func d33aSpike2gImpl5cRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5c: cannot read %s: %v", path, err)
	}
	return string(b)
}

// Bound a single-selector rule block (`.foo { … }`) so assertions
// stay scoped to that rule.
func d33aSpike2gImpl5cRuleBody(t *testing.T, css, selector string) string {
	t.Helper()
	start := strings.Index(css, selector+" {")
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-5c: CSS rule %q missing", selector)
	}
	end := strings.Index(css[start:], "}")
	if end < 0 {
		t.Fatalf("D33a-spike-2g-impl-5c: could not bound rule body for %q", selector)
	}
	return css[start : start+end]
}

// ── 1. Compact value-dominant row layout ─────────────────────────────

// TestExplorer_D33aSpike2gImpl5c_FieldRowsUseValueDominantGrid pins
// the impl-5c grid: a minmax(86-120 px) label column + minmax(0,
// 1fr) value column. Rejects the impl-5b fixed 140 px column and
// equal-split 1fr 1fr.
func TestExplorer_D33aSpike2gImpl5c_FieldRowsUseValueDominantGrid(t *testing.T) {
	css := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cCssPath)
	body := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-details-row")

	if !strings.Contains(body, "display: grid") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-row must use display: grid")
	}
	if !strings.Contains(body, "minmax(96px, 120px) minmax(0, 1fr)") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-row must use grid-template-columns: minmax(96px, 120px) minmax(0, 1fr) (value-dominant)")
	}
	// Reject 1fr 1fr or label columns wider than 140 px.
	if strings.Contains(body, "1fr 1fr") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-row must NOT use 1fr 1fr (creates 50/50 columns)")
	}
	if strings.Contains(body, "grid-template-columns: 140px") ||
		strings.Contains(body, "grid-template-columns: 150px") ||
		strings.Contains(body, "grid-template-columns: 160px") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-row must NOT use a fixed label column ≥ 140 px (cramps long values)")
	}
}

// ── 2. Value cell shrinks and wraps long content ────────────────────

// TestExplorer_D33aSpike2gImpl5c_FieldValuesWrapLongContent pins
// `min-width: 0` + `overflow-wrap: anywhere` on the value cell.
// Without `min-width: 0`, the grid track's `auto` minimum refuses
// to shrink the value below its content's intrinsic width, which
// forced the horizontal scrollbar impl-5b's review surfaced.
func TestExplorer_D33aSpike2gImpl5c_FieldValuesWrapLongContent(t *testing.T) {
	css := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cCssPath)
	body := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-details-val")

	if !strings.Contains(body, "min-width: 0") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-val must declare `min-width: 0` so the value cell can shrink inside the grid track")
	}
	if !strings.Contains(body, "overflow-wrap: anywhere") &&
		!strings.Contains(body, "overflow-wrap: break-word") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-val must declare overflow-wrap so long hyphenated IDs wrap")
	}
}

// ── 3. Inspector suppresses horizontal scrollbar ─────────────────────

// TestExplorer_D33aSpike2gImpl5c_InspectorSuppressesHorizontalScrollbar
// pins that the rail panel container declares `overflow-x: hidden`
// as a defensive guard so even an unanticipated wide child cannot
// produce a horizontal scrollbar.
func TestExplorer_D33aSpike2gImpl5c_InspectorSuppressesHorizontalScrollbar(t *testing.T) {
	css := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cCssPath)
	body := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-right-rail-panel")
	if !strings.Contains(body, "overflow-x: hidden") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-right-rail-panel must declare `overflow-x: hidden` (no horizontal scrollbar in the inspector)")
	}
}

// ── 4. Inspector does not force a vertical scrollbar ─────────────────

// TestExplorer_D33aSpike2gImpl5c_InspectorDoesNotForceVerticalScrollbar
// pins that the rail panel does NOT declare `overflow-y: scroll`
// (always-visible vertical scrollbar). `overflow-y: auto` is
// permitted as a defensive fallback but the brief explicitly
// rejects a forced vertical scrollbar as the normal visual state.
func TestExplorer_D33aSpike2gImpl5c_InspectorDoesNotForceVerticalScrollbar(t *testing.T) {
	css := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cCssPath)
	body := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-right-rail-panel")
	if strings.Contains(body, "overflow-y: scroll") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-right-rail-panel must NOT use overflow-y: scroll (always-visible scrollbar)")
	}
	if strings.Contains(body, "overflow: scroll") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-right-rail-panel must NOT use overflow: scroll (always-visible scrollbar)")
	}
}

// ── 5. Compact vertical spacing ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5c_InspectorUsesCompactVerticalSpacing
// pins that row padding stays compact (3 px vertical) so the
// selected-node section fits in the normal drawer height without
// forcing a vertical scrollbar.
func TestExplorer_D33aSpike2gImpl5c_InspectorUsesCompactVerticalSpacing(t *testing.T) {
	css := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cCssPath)
	rowBody := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-details-row")
	if !strings.Contains(rowBody, "padding: 3px 0") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-row must use compact `padding: 3px 0` (impl-5b 4 px tightened so normal selected-node fits without vertical scroll)")
	}

	// Rail panel left padding tightened from `space-3` (12 px) to
	// `space-2` (8 px) so the visible gutter between the tab rail
	// and content lands in the 24-32 px target band.
	panelBody := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-right-rail-panel")
	if !strings.Contains(panelBody, "calc(var(--gmap-right-rail-handle-width) + var(--space-2))") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-right-rail-panel left padding must be `calc(handle-width + var(--space-2))` so the gap between tab rail and content stays inside the 24-32 px target")
	}
}

// ── 6. Field typography compact ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5c_InspectorFieldTypographyCompact pins
// that field labels / values are 13-14 px (not title-sized) and
// the selected-node title is 18-20 px.
func TestExplorer_D33aSpike2gImpl5c_InspectorFieldTypographyCompact(t *testing.T) {
	css := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cCssPath)

	keyBody := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-details-key")
	if !strings.Contains(keyBody, "font-size: 13px") && !strings.Contains(keyBody, "font-size: 14px") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-key font-size must be 13/14 px (not title-sized)")
	}
	valBody := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-details-val")
	if !strings.Contains(valBody, "font-size: 13px") && !strings.Contains(valBody, "font-size: 14px") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-val font-size must be 13/14 px (not title-sized)")
	}
	nameBody := d33aSpike2gImpl5cRuleBody(t, css, ".gmap-details-name")
	if !strings.Contains(nameBody, "font-size: 18px") && !strings.Contains(nameBody, "font-size: 20px") {
		t.Error("D33a-spike-2g-impl-5c: .gmap-details-name font-size must be 18-20 px")
	}
}

// ── 7. Connected edges row appears when available ────────────────────

// TestExplorer_D33aSpike2gImpl5c_SelectedNodeShowsConnectedEdgesWhenAvailable
// pins that the inspector still appends the `Connected edges` row
// when `details._connected_edges` is present (impl-5b feature
// preserved through impl-5c).
func TestExplorer_D33aSpike2gImpl5c_SelectedNodeShowsConnectedEdgesWhenAvailable(t *testing.T) {
	js := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cInspectorPath)
	if !strings.Contains(js, "'Connected edges'") {
		t.Error("D33a-spike-2g-impl-5c: inspector must still include a 'Connected edges' row label")
	}
	if !strings.Contains(js, "details._connected_edges") {
		t.Error("D33a-spike-2g-impl-5c: inspector must still read details._connected_edges")
	}
	// Guard pattern: original impl-5b inlined `details._connected_edges
	// != null`; impl-5e factored the primary block into `_primaryRows`,
	// where the same value is passed as the `connectedEdges` parameter
	// and guarded as `connectedEdges != null`. Either literal satisfies
	// the contract — the row must still be conditional on a non-null
	// connected-edges value.
	if !strings.Contains(js, "details._connected_edges != null") &&
		!strings.Contains(js, "connectedEdges != null") {
		t.Error("D33a-spike-2g-impl-5c: inspector must still guard the Connected edges row with a non-null check")
	}
}

// ── 8. Carrier still supplies the connected-edge count ───────────────

// TestExplorer_D33aSpike2gImpl5c_CarrierIncludesConnectedEdgeCount
// pins that the PoC carrier continues to pre-compute the count
// from `elements.edges` and embed `_connected_edges` in the
// carrier JSON.
func TestExplorer_D33aSpike2gImpl5c_CarrierIncludesConnectedEdgeCount(t *testing.T) {
	js := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cPocPath)
	for _, want := range []string{
		"connectedByRef",
		"elements.edges",
		"_detailsForCarrier(d, connected)",
		"out._connected_edges = connectedCount",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-5c: PoC carrier must keep %q", want)
		}
	}
}

// ── 9. PoC inspector aside still present ─────────────────────────────

// TestExplorer_D33aSpike2gImpl5c_PocInspectorStillPresent — superseded
// by D33x-list-mode. Asserts the inverse contract: floating-card
// render path is GONE, carrier-DOM contract REMAINS.
func TestExplorer_D33aSpike2gImpl5c_PocInspectorStillPresent(t *testing.T) {
	js := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cPocPath)
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

// ── 10. Authority thin-card rendering unchanged ──────────────────────

// TestExplorer_D33aSpike2gImpl5c_CardRenderingUnchanged pins the
// single-line card contract.
func TestExplorer_D33aSpike2gImpl5c_CardRenderingUnchanged(t *testing.T) {
	js := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cPocPath)
	if !strings.Contains(js, "'authority-thin-card-v1'") {
		t.Error("D33a-spike-2g-impl-5c: authority-thin-card-v1 must remain registered")
	}
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5c: thin-card branch missing")
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	branch := tail[:end]
	if !strings.Contains(branch, "_displayTitle(ele") {
		t.Error("D33a-spike-2g-impl-5c: thin-card branch must still bind label via _displayTitle (single-line)")
	}
	if strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-5c: thin-card branch must NOT reintroduce _displayCardLabel")
	}
	if !strings.Contains(branch, "icons.cytoscapeDataURI(syms[") {
		t.Error("D33a-spike-2g-impl-5c: strategic symbols must remain")
	}
}

// ── 11. Production graph unchanged ───────────────────────────────────

// TestExplorer_D33aSpike2gImpl5c_ProductionGraphUnaffected pins
// that production `authority-graph-view.js` carries no PoC
// carrier names, no impl-5b/c helper references.
func TestExplorer_D33aSpike2gImpl5c_ProductionGraphUnaffected(t *testing.T) {
	body := d33aSpike2gImpl5cRead(t, d33aSpike2gImpl5cViewPath)
	for _, banned := range []string{
		"cytoscape-poc-inspector-carrier",
		"data-cytoscape-poc-carrier",
		"_renderInspectorCarriers",
		"_clearInspectorCarriers",
		"_detailsForCarrier",
		"_connected_edges",
		"authority-thin-card-v1",
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-5c: production Authority view must not reference %q", banned)
		}
	}
}
