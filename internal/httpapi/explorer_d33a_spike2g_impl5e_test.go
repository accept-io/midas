package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl5e_test.go — D33a-spike-2g-impl-5e.
//
// Selected-node inspector content model:
//
//   1. Primary block (always, fixed order): Kind / ID / Label /
//      Connected edges.
//   2. Node-specific block (per-kind formatter `specific`): only
//      meaningful, human-useful fields.
//   3. Technical details (per-kind formatter `technical`, wrapped
//      in `<details>` collapsed by default): raw / debug / numeric
//      fields that should not dominate the primary visible pane.
//
// `_renderInto` composes the field-rows HTML directly into
// `#gmap-details-fields`, bypassing the shared `inspector.setFields`
// uniform-row renderer for this slot only (setName / setSummary /
// setGovernance still go through the shared helper).
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style.

const (
	d33aSpike2gImpl5eInspectorPath = "explorer/assets/js/graph/authority/authority-graph-inspector.js"
	d33aSpike2gImpl5eCssPath       = "explorer/assets/css/governance-map.css"
	d33aSpike2gImpl5eViewJsPath    = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl5eIndexPath     = "explorer/index.html"
	d33aSpike2gImpl5ePocPath       = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

func d33aSpike2gImpl5eRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5e: cannot read %s: %v", path, err)
	}
	return string(b)
}

// Bound a JS function body so per-helper assertions stay scoped.
func d33aSpike2gImpl5eFn(t *testing.T, js, opener string) string {
	t.Helper()
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-5e: JS opener %q missing", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n  }")
	if end < 0 {
		t.Fatalf("D33a-spike-2g-impl-5e: could not bound function body for %q", opener)
	}
	return tail[:end]
}

// ── 1. Primary block is fixed ────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5e_PrimarySelectedNodeFieldsAreFixed
// pins the four primary rows in their exact order and pins the
// helper that builds them. The helper is the one place the order
// is documented; tests + future readers can re-derive the
// contract from it.
func TestExplorer_D33aSpike2gImpl5e_PrimarySelectedNodeFieldsAreFixed(t *testing.T) {
	js := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eInspectorPath)
	body := d33aSpike2gImpl5eFn(t, js, "function _primaryRows(")

	for _, want := range []string{
		"['Kind',",
		"['ID',",
		"['Label',",
		"['Connected edges',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-5e: _primaryRows must emit %q row", want)
		}
	}
	// Order: Kind → ID → Label → Connected edges.
	kIdx := strings.Index(body, "['Kind',")
	idIdx := strings.Index(body, "['ID',")
	lbIdx := strings.Index(body, "['Label',")
	ceIdx := strings.Index(body, "['Connected edges',")
	if !(kIdx < idIdx && idIdx < lbIdx && lbIdx < ceIdx) {
		t.Errorf("D33a-spike-2g-impl-5e: primary rows must be ordered Kind → ID → Label → Connected edges (got Kind=%d, ID=%d, Label=%d, Connected edges=%d)", kIdx, idIdx, lbIdx, ceIdx)
	}
	// Connected edges row is conditional on the carrier-supplied
	// count (production view doesn't emit it).
	if !strings.Contains(body, "connectedEdges != null") {
		t.Error("D33a-spike-2g-impl-5e: _primaryRows must guard the Connected edges row with a non-null check")
	}

	// _renderInto must call _primaryRows BEFORE the formatter.
	renderBody := d33aSpike2gImpl5eFn(t, js, "function _renderInto(")
	primaryIdx := strings.Index(renderBody, "_primaryRows(")
	formattedIdx := strings.Index(renderBody, "formatter(details")
	if primaryIdx < 0 {
		t.Fatal("D33a-spike-2g-impl-5e: _renderInto must call _primaryRows")
	}
	if formattedIdx > 0 && !(primaryIdx < formattedIdx) {
		t.Error("D33a-spike-2g-impl-5e: _primaryRows must be computed BEFORE the per-kind formatter so primary block renders first")
	}
}

// ── 2. Raw fields excluded from primary block ────────────────────────

// TestExplorer_D33aSpike2gImpl5e_RawTechnicalFieldsExcludedFromPrimaryBlock
// pins that the _primaryRows helper never emits raw / debug
// fields.
func TestExplorer_D33aSpike2gImpl5e_RawTechnicalFieldsExcludedFromPrimaryBlock(t *testing.T) {
	js := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eInspectorPath)
	body := d33aSpike2gImpl5eFn(t, js, "function _primaryRows(")
	for _, banned := range []string{
		"version",
		"process_id",
		"business_service_id",
		"effective_policy_source",
		"effective_policy_id",
		"rule_count_by_class",
		"origin",
		"managed",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-5e: _primaryRows must not reference raw field %q (belongs in node-specific or technical block)", banned)
		}
	}
}

// ── 3. Per-kind specific/technical mapping covers all 7 kinds ────────

// TestExplorer_D33aSpike2gImpl5e_NodeSpecificFieldMappingExists pins
// that each per-kind formatter returns the `{ specific, technical }`
// shape — both lists are explicitly declared so the
// `_renderInto` composer can iterate them safely.
func TestExplorer_D33aSpike2gImpl5e_NodeSpecificFieldMappingExists(t *testing.T) {
	js := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eInspectorPath)
	for _, formatter := range []string{
		"function _formatBusinessService(",
		"function _formatDecisionSurface(",
		"function _formatAuthorityProfile(",
		"function _formatAuthorityGrant(",
		"function _formatAgent(",
		"function _formatFailModePolicy(",
		"function _formatEscalationTarget(",
	} {
		body := d33aSpike2gImpl5eFn(t, js, formatter)
		if !strings.Contains(body, "specific:") {
			t.Errorf("D33a-spike-2g-impl-5e: %q must return a `specific:` list", formatter)
		}
		if !strings.Contains(body, "technical:") {
			t.Errorf("D33a-spike-2g-impl-5e: %q must return a `technical:` list", formatter)
		}
	}
}

// ── 4. rule_count_by_class moved out of the primary visible pane ─────

// TestExplorer_D33aSpike2gImpl5e_FailModePolicyLargeRawFieldsMovedToTechnicalDetails
// pins that the fail-mode-policy formatter places
// `rule_count_by_class` in the `technical` list, not the
// `specific` list.
func TestExplorer_D33aSpike2gImpl5e_FailModePolicyLargeRawFieldsMovedToTechnicalDetails(t *testing.T) {
	js := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eInspectorPath)
	body := d33aSpike2gImpl5eFn(t, js, "function _formatFailModePolicy(")
	if !strings.Contains(body, "rule_count_by_class") {
		t.Fatal("D33a-spike-2g-impl-5e: _formatFailModePolicy must mention rule_count_by_class (in technical block)")
	}
	// rule_count_by_class must sit in the `technical:` half of the
	// return object — i.e. AFTER the `technical:` key. Compute the
	// indices and assert ordering.
	specificIdx := strings.Index(body, "specific:")
	technicalIdx := strings.Index(body, "technical:")
	rccIdx := strings.Index(body, "rule_count_by_class")
	if !(technicalIdx < rccIdx) {
		t.Error("D33a-spike-2g-impl-5e: rule_count_by_class must appear AFTER `technical:` (i.e. inside the technical list)")
	}
	// And NOT inside the specific half.
	if specificIdx > 0 && rccIdx > specificIdx && rccIdx < technicalIdx {
		t.Error("D33a-spike-2g-impl-5e: rule_count_by_class must NOT appear inside the `specific:` list")
	}
}

// ── 5. Technical details section exists ──────────────────────────────

// TestExplorer_D33aSpike2gImpl5e_TechnicalDetailsSectionExistsOrRawFieldsOmitted
// pins the `_renderTechnicalDetailsHtml` helper exists and emits
// a native `<details>` element so the section is collapsed by
// default. The CSS rule for `details.gmap-details-technical`
// must accompany the helper.
func TestExplorer_D33aSpike2gImpl5e_TechnicalDetailsSectionExistsOrRawFieldsOmitted(t *testing.T) {
	js := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eInspectorPath)
	if !strings.Contains(js, "function _renderTechnicalDetailsHtml(") {
		t.Fatal("D33a-spike-2g-impl-5e: _renderTechnicalDetailsHtml helper missing")
	}
	body := d33aSpike2gImpl5eFn(t, js, "function _renderTechnicalDetailsHtml(")
	for _, want := range []string{
		`<details class="gmap-details-technical">`,
		`<summary>Technical details</summary>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-5e: _renderTechnicalDetailsHtml must emit %q", want)
		}
	}
	// Helper is called from _renderInto.
	renderBody := d33aSpike2gImpl5eFn(t, js, "function _renderInto(")
	if !strings.Contains(renderBody, "_renderTechnicalDetailsHtml(technicalRows)") {
		t.Error("D33a-spike-2g-impl-5e: _renderInto must call _renderTechnicalDetailsHtml(technicalRows)")
	}

	// CSS rule for the <details> wrapper.
	css := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eCssPath)
	if !strings.Contains(css, "details.gmap-details-technical {") {
		t.Error("D33a-spike-2g-impl-5e: CSS rule `details.gmap-details-technical` missing")
	}
	if !strings.Contains(css, "details.gmap-details-technical > summary {") {
		t.Error("D33a-spike-2g-impl-5e: CSS rule for `details.gmap-details-technical > summary` missing")
	}
}

// ── 6. No duplicate field rendering ──────────────────────────────────

// TestExplorer_D33aSpike2gImpl5e_NoDuplicatePrimaryFieldsInTechnicalDetails
// pins that the per-kind formatters do not duplicate primary
// fields (`Kind`, `ID`, `Label`, `Connected edges`) inside their
// `specific` or `technical` lists.
func TestExplorer_D33aSpike2gImpl5e_NoDuplicatePrimaryFieldsInTechnicalDetails(t *testing.T) {
	js := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eInspectorPath)
	formatters := []string{
		"function _formatBusinessService(",
		"function _formatDecisionSurface(",
		"function _formatAuthorityProfile(",
		"function _formatAuthorityGrant(",
		"function _formatAgent(",
		"function _formatFailModePolicy(",
		"function _formatEscalationTarget(",
	}
	for _, opener := range formatters {
		body := d33aSpike2gImpl5eFn(t, js, opener)
		for _, banned := range []string{
			"['Kind',",
			"['ID',",
			"['Label',",
			"['Connected edges',",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D33a-spike-2g-impl-5e: %q must not duplicate primary row %q", opener, banned)
			}
		}
	}
}

// ── 7. Header row from impl-5d preserved ─────────────────────────────

// TestExplorer_D33aSpike2gImpl5e_HeaderRowFromImpl5dPreserved pins
// that impl-5d's header-row contract survives impl-5e.
func TestExplorer_D33aSpike2gImpl5e_HeaderRowFromImpl5dPreserved(t *testing.T) {
	view := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eViewJsPath)
	if !strings.Contains(view, "id: 'inspector', label: 'Inspector'") {
		t.Error("D33a-spike-2g-impl-5e: impl-5d tab label `Inspector` must remain")
	}
	css := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eCssPath)
	if !strings.Contains(css, ".gmap-right-rail-header {\n    /* D33a-spike-2g-impl-5d") {
		// We don't need the exact comment, just confirm the impl-5d
		// rule survived. Fall back to a softer check.
		headerStart := strings.Index(css, ".gmap-right-rail-header {")
		headerEnd := strings.Index(css[headerStart:], "}")
		headerBody := css[headerStart : headerStart+headerEnd]
		if !strings.Contains(headerBody, "display: none") {
			t.Error("D33a-spike-2g-impl-5e: impl-5d `.gmap-right-rail-header { display: none }` must remain")
		}
	}
	html := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eIndexPath)
	if !strings.Contains(html, `class="gmap-details-name-row"`) {
		t.Error("D33a-spike-2g-impl-5e: impl-5d `.gmap-details-name-row` container must remain in index.html")
	}
	if !strings.Contains(html, "|-&gt;") {
		t.Error("D33a-spike-2g-impl-5e: impl-5d `|->` close glyph must remain")
	}
}

// ── 8. Layout from impl-5c preserved ─────────────────────────────────

// TestExplorer_D33aSpike2gImpl5e_DrawerLayoutFromImpl5cPreserved pins
// the impl-5c value-dominant grid + overflow-x: hidden invariants.
func TestExplorer_D33aSpike2gImpl5e_DrawerLayoutFromImpl5cPreserved(t *testing.T) {
	css := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eCssPath)
	for _, want := range []string{
		"minmax(96px, 120px) minmax(0, 1fr)",
		"min-width: 0",
		"overflow-wrap: anywhere",
		"overflow-x: hidden",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D33a-spike-2g-impl-5e: drawer CSS must keep impl-5c declaration %q", want)
		}
	}
	if strings.Contains(css, "overflow-y: scroll") {
		t.Error("D33a-spike-2g-impl-5e: drawer CSS must NOT introduce overflow-y: scroll (forced vertical scrollbar)")
	}
}

// ── 9. PoC inspector aside still present ─────────────────────────────

// TestExplorer_D33aSpike2gImpl5e_PocInspectorStillPresent — superseded
// by D33x-list-mode. Floating card retired; carrier contract remains.
func TestExplorer_D33aSpike2gImpl5e_PocInspectorStillPresent(t *testing.T) {
	js := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5ePocPath)
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

func TestExplorer_D33aSpike2gImpl5e_CardRenderingUnchanged(t *testing.T) {
	js := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5ePocPath)
	if !strings.Contains(js, "'authority-thin-card-v1'") {
		t.Error("D33a-spike-2g-impl-5e: authority-thin-card-v1 must remain registered")
	}
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5e: thin-card branch missing")
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	branch := tail[:end]
	if !strings.Contains(branch, "_displayTitle(ele") {
		t.Error("D33a-spike-2g-impl-5e: thin-card branch must still bind label via _displayTitle (single-line)")
	}
	if strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-5e: thin-card branch must NOT reintroduce _displayCardLabel")
	}
	if !strings.Contains(branch, "icons.cytoscapeDataURI(syms[") {
		t.Error("D33a-spike-2g-impl-5e: strategic symbols must remain")
	}
}

// ── 11. Production graph view unaffected ─────────────────────────────

// TestExplorer_D33aSpike2gImpl5e_ProductionGraphUnaffected pins
// that production `authority-graph-view.js` carries no impl-5e
// helper / class references.
func TestExplorer_D33aSpike2gImpl5e_ProductionGraphUnaffected(t *testing.T) {
	body := d33aSpike2gImpl5eRead(t, d33aSpike2gImpl5eViewJsPath)
	for _, banned := range []string{
		"_primaryRows",
		"_renderFieldRowsHtml",
		"_renderTechnicalDetailsHtml",
		"gmap-details-technical",
		"gmap-details-technical-body",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-5e: production view must not reference impl-5e helper/class %q", banned)
		}
	}
}
