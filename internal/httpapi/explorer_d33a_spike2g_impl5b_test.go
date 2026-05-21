package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl5b_test.go — D33a-spike-2g-impl-5b.
//
// MIDAS right-drawer Inspector layout refinement. Three coordinated
// edits:
//
//   1. CSS in governance-map.css: switch `.gmap-details-row` from a
//      space-between flex to a compact two-column grid; bump
//      `.gmap-details-name` to 18 px; sans/13 px replace the mono/
//      11 px key+val; add overflow-wrap for long IDs.
//   2. Inspector in authority-graph-inspector.js: append a
//      "Connected edges" row when `details._connected_edges` is
//      present (conditional — production view doesn't emit it
//      today and continues to render cleanly).
//   3. PoC carrier in authority-cytoscape-poc.js:
//      `_renderInspectorCarriers` pre-computes per-node connected
//      edge counts and passes them to `_detailsForCarrier`, which
//      embeds `_connected_edges` in the carrier JSON.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl5bCssPath       = "explorer/assets/css/governance-map.css"
	d33aSpike2gImpl5bInspectorPath = "explorer/assets/js/graph/authority/authority-graph-inspector.js"
	d33aSpike2gImpl5bPocPath       = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl5bViewPath      = "explorer/assets/js/graph/authority/authority-graph-view.js"
)

func d33aSpike2gImpl5bRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5b: cannot read %s: %v", path, err)
	}
	return string(b)
}

// ── 1. Drawer layout class / selectors exist ─────────────────────────

// TestExplorer_D33aSpike2gImpl5b_RightDrawerInspectorLayoutRefined
// pins the refined selectors exist in the drawer stylesheet.
func TestExplorer_D33aSpike2gImpl5b_RightDrawerInspectorLayoutRefined(t *testing.T) {
	css := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bCssPath)
	for _, want := range []string{
		".gmap-details-title",
		".gmap-details-name",
		".gmap-details-row",
		".gmap-details-key",
		".gmap-details-val",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D33a-spike-2g-impl-5b: drawer CSS must declare %q", want)
		}
	}
}

// ── 2. Field rows use a compact two-column layout ────────────────────

// TestExplorer_D33aSpike2gImpl5b_FieldRowsUseCompactTwoColumnLayout
// pins that `.gmap-details-row` is a grid with a bounded label
// column and column-gap, and that the old `justify-content:
// space-between` (which produced huge horizontal gaps) is gone.
func TestExplorer_D33aSpike2gImpl5b_FieldRowsUseCompactTwoColumnLayout(t *testing.T) {
	css := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bCssPath)
	rowStart := strings.Index(css, ".gmap-details-row {")
	if rowStart < 0 {
		t.Fatal("D33a-spike-2g-impl-5b: .gmap-details-row rule missing")
	}
	rowEnd := strings.Index(css[rowStart:], "}")
	if rowEnd < 0 {
		t.Fatal("D33a-spike-2g-impl-5b: could not bound .gmap-details-row body")
	}
	rowBody := css[rowStart : rowStart+rowEnd]

	if !strings.Contains(rowBody, "display: grid") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-row must use display: grid (compact two-column)")
	}
	// Accept either the impl-5b fixed-label grid OR the impl-5c
	// value-dominant minmax grid. Both satisfy the impl-5b intent
	// (bounded label column ≤ 140 px); the per-tranche layout
	// tests (impl-5b vs impl-5c) own the exact shape.
	if !strings.Contains(rowBody, "grid-template-columns: 140px 1fr") &&
		!strings.Contains(rowBody, "grid-template-columns: minmax(96px, 120px) minmax(0, 1fr)") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-row must use a bounded label column (140px 1fr or minmax(96px, 120px) minmax(0, 1fr))")
	}
	// Gap must be present and small — the brief targets 14-24 px.
	if !strings.Contains(rowBody, "column-gap: 14px") &&
		!strings.Contains(rowBody, "column-gap: 16px") &&
		!strings.Contains(rowBody, "column-gap: 20px") &&
		!strings.Contains(rowBody, "column-gap: 24px") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-row must declare a bounded column-gap (14-24 px)")
	}
	// The pre-impl-5b `justify-content: space-between` produced the
	// visual artefact the brief specifically calls out — it must be
	// gone from the row rule.
	if strings.Contains(rowBody, "justify-content: space-between") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-row must NOT use justify-content: space-between (creates huge horizontal gap)")
	}
}

// ── 3. Typography tuned ──────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5b_InspectorTypographyTuned pins the
// refined typography:
//   - selected-node title is 18-20 px / 700;
//   - field key/val are 13-14 px (key muted, val 600);
//   - section title remains uppercase and muted.
func TestExplorer_D33aSpike2gImpl5b_InspectorTypographyTuned(t *testing.T) {
	css := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bCssPath)

	// .gmap-details-name 18 px / 700.
	nameStart := strings.Index(css, ".gmap-details-name {")
	if nameStart < 0 {
		t.Fatal("D33a-spike-2g-impl-5b: .gmap-details-name rule missing")
	}
	nameEnd := strings.Index(css[nameStart:], "}")
	nameBody := css[nameStart : nameStart+nameEnd]
	if !strings.Contains(nameBody, "font-size: 18px") && !strings.Contains(nameBody, "font-size: 20px") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-name font-size must be 18 px or 20 px")
	}
	if !strings.Contains(nameBody, "font-weight: 700") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-name must keep font-weight: 700")
	}

	// .gmap-details-key — 13/14 px, muted (slate-500 / -variant).
	keyStart := strings.Index(css, ".gmap-details-key {")
	keyEnd := strings.Index(css[keyStart:], "}")
	keyBody := css[keyStart : keyStart+keyEnd]
	if !strings.Contains(keyBody, "font-size: 13px") && !strings.Contains(keyBody, "font-size: 14px") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-key font-size must be 13 px or 14 px")
	}
	if !strings.Contains(keyBody, "var(--slate-500)") &&
		!strings.Contains(keyBody, "var(--on-surface-variant)") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-key must use a muted palette token (slate-500 / on-surface-variant)")
	}

	// .gmap-details-val — 13/14 px, weight 600.
	valStart := strings.Index(css, ".gmap-details-val {")
	valEnd := strings.Index(css[valStart:], "}")
	valBody := css[valStart : valStart+valEnd]
	if !strings.Contains(valBody, "font-size: 13px") && !strings.Contains(valBody, "font-size: 14px") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-val font-size must be 13 px or 14 px")
	}
	if !strings.Contains(valBody, "font-weight: 600") && !strings.Contains(valBody, "font-weight: 700") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-val font-weight must be 600 or 700")
	}

	// .gmap-details-title — uppercase + muted (unchanged invariants).
	titleStart := strings.Index(css, ".gmap-details-title {")
	titleEnd := strings.Index(css[titleStart:], "}")
	titleBody := css[titleStart : titleStart+titleEnd]
	if !strings.Contains(titleBody, "text-transform: uppercase") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-title must remain uppercase")
	}
	if !strings.Contains(titleBody, "var(--slate-500)") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-title must remain muted (slate-500)")
	}
}

// ── 4. Long values do not overflow ───────────────────────────────────

// TestExplorer_D33aSpike2gImpl5b_LongValuesDoNotOverflow pins that
// .gmap-details-val (and .gmap-details-name for the selected-node
// title) declare an overflow-wrap / word-break strategy so long
// hyphenated IDs wrap inside the value column instead of pushing
// the row horizontally.
func TestExplorer_D33aSpike2gImpl5b_LongValuesDoNotOverflow(t *testing.T) {
	css := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bCssPath)

	valStart := strings.Index(css, ".gmap-details-val {")
	valEnd := strings.Index(css[valStart:], "}")
	valBody := css[valStart : valStart+valEnd]
	if !strings.Contains(valBody, "overflow-wrap") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-val must declare overflow-wrap so long IDs wrap")
	}
	if !strings.Contains(valBody, "word-break") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-val must declare word-break (defence for browsers without overflow-wrap:anywhere)")
	}

	nameStart := strings.Index(css, ".gmap-details-name {")
	nameEnd := strings.Index(css[nameStart:], "}")
	nameBody := css[nameStart : nameStart+nameEnd]
	if !strings.Contains(nameBody, "overflow-wrap") {
		t.Error("D33a-spike-2g-impl-5b: .gmap-details-name must declare overflow-wrap (long labels)")
	}
}

// ── 5. Connected edges row added in inspector ────────────────────────

// TestExplorer_D33aSpike2gImpl5b_SelectedNodeShowsConnectedEdgesWhenAvailable
// pins that the production inspector module appends a "Connected
// edges" row when `details._connected_edges` is present, and that
// the row is conditional (production view doesn't emit the field
// today, so the inspector must keep working without it).
func TestExplorer_D33aSpike2gImpl5b_SelectedNodeShowsConnectedEdgesWhenAvailable(t *testing.T) {
	js := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bInspectorPath)
	if !strings.Contains(js, "'Connected edges'") {
		t.Error("D33a-spike-2g-impl-5b: inspector must include a 'Connected edges' row label")
	}
	if !strings.Contains(js, "details._connected_edges") {
		t.Error("D33a-spike-2g-impl-5b: inspector must read details._connected_edges (carrier-supplied)")
	}
	// The append must be conditional — null/undefined guard so
	// production renders cleanly without the field. impl-5b inlined
	// `details._connected_edges != null`; impl-5e factored the
	// primary block into `_primaryRows` and the same value is guarded
	// via the `connectedEdges` parameter as `connectedEdges != null`.
	// Either literal satisfies the contract.
	if !strings.Contains(js, "details._connected_edges != null") &&
		!strings.Contains(js, "connectedEdges != null") {
		t.Error("D33a-spike-2g-impl-5b: inspector must guard the Connected edges row with a non-null check (production compatibility)")
	}
}

// ── 6. Carrier supplies the connected-edge count ─────────────────────

// TestExplorer_D33aSpike2gImpl5b_CarrierIncludesConnectedEdgeCount
// pins that `_renderInspectorCarriers` pre-computes per-node
// connected-edge counts from `elements.edges`, and that
// `_detailsForCarrier` embeds the count in the carrier JSON when
// passed.
func TestExplorer_D33aSpike2gImpl5b_CarrierIncludesConnectedEdgeCount(t *testing.T) {
	js := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bPocPath)

	// _renderInspectorCarriers walks elements.edges to count per-node.
	renderStart := strings.Index(js, "function _renderInspectorCarriers(")
	if renderStart < 0 {
		t.Fatal("D33a-spike-2g-impl-5b: _renderInspectorCarriers helper missing")
	}
	renderEnd := strings.Index(js[renderStart:], "\n  }")
	renderBody := js[renderStart : renderStart+renderEnd]
	for _, want := range []string{
		"connectedByRef",
		"elements.edges",
		"connectedByRef[ed.source]",
		"connectedByRef[ed.target]",
	} {
		if !strings.Contains(renderBody, want) {
			t.Errorf("D33a-spike-2g-impl-5b: _renderInspectorCarriers must compute connected-edge counts (missing %q)", want)
		}
	}
	if !strings.Contains(renderBody, "_detailsForCarrier(d, connected)") {
		t.Error("D33a-spike-2g-impl-5b: _renderInspectorCarriers must pass `connected` count into _detailsForCarrier")
	}

	// _detailsForCarrier accepts the count + attaches _connected_edges.
	detStart := strings.Index(js, "function _detailsForCarrier(")
	detEnd := strings.Index(js[detStart:], "\n  }")
	detBody := js[detStart : detStart+detEnd]
	if !strings.Contains(detBody, "function _detailsForCarrier(nodeData, connectedCount)") {
		t.Error("D33a-spike-2g-impl-5b: _detailsForCarrier must accept (nodeData, connectedCount)")
	}
	if !strings.Contains(detBody, "out._connected_edges = connectedCount") {
		t.Error("D33a-spike-2g-impl-5b: _detailsForCarrier must attach out._connected_edges = connectedCount")
	}
}

// ── 7. PoC inspector remains ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5b_PocInspectorStillPresent — superseded
// by D33x-list-mode. The floating PoC inspector aside has been
// retired in favour of the production right drawer.
func TestExplorer_D33aSpike2gImpl5b_PocInspectorStillPresent(t *testing.T) {
	js := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bPocPath)
	for _, gone := range []string{
		"function _renderInspector(node)",
		"function _renderInspectorEmpty(",
	} {
		if strings.Contains(js, gone) {
			t.Errorf("D33x-list-mode: floating PoC inspector aside must remain retired — found %q", gone)
		}
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

// TestExplorer_D33aSpike2gImpl5b_AuthorityThinCardUnchanged pins
// that the impl-4c → impl-4e single-line card contract is not
// disturbed by impl-5b's drawer-only refinement.
func TestExplorer_D33aSpike2gImpl5b_AuthorityThinCardUnchanged(t *testing.T) {
	js := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bPocPath)
	if !strings.Contains(js, "'authority-thin-card-v1'") {
		t.Error("D33a-spike-2g-impl-5b: authority-thin-card-v1 must remain registered")
	}

	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5b: thin-card branch missing")
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	branch := tail[:end]

	if !strings.Contains(branch, "_displayTitle(ele") {
		t.Error("D33a-spike-2g-impl-5b: thin-card branch must still bind label via _displayTitle (single-line contract)")
	}
	if strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-5b: thin-card branch must NOT reintroduce _displayCardLabel")
	}
	if !strings.Contains(branch, "icons.cytoscapeDataURI(syms[") {
		t.Error("D33a-spike-2g-impl-5b: strategic symbol rendering must remain")
	}
}

// ── 9. Production graph unchanged ────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5b_ProductionAuthorityGraphUnaffected
// pins that production `authority-graph-view.js` carries no PoC
// carrier names, no impl-5b helper references, and no Cytoscape
// theme references.
func TestExplorer_D33aSpike2gImpl5b_ProductionAuthorityGraphUnaffected(t *testing.T) {
	body := d33aSpike2gImpl5bRead(t, d33aSpike2gImpl5bViewPath)
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
			t.Errorf("D33a-spike-2g-impl-5b: production Authority view must not reference %q", banned)
		}
	}
}
