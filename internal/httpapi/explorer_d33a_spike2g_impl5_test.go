package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl5_test.go — D33a-spike-2g-impl-5.
//
// Cytoscape PoC inspector carrier DOM. The PoC paints hidden
// `.gmap-node[data-node-id][data-node-name][data-node-details]`
// elements under #gmap-canvas so the production right-side inspector
// (`authority-graph-inspector.js`) can find a target for the selected
// Cytoscape node and run its existing per-kind formatter pipeline.
//
// Carriers are presentation-free (hidden + display:none + aria-hidden),
// PoC-only (marked with `data-cytoscape-poc-carrier`), idempotent
// (cleared before every render + on teardown), and read-only (no
// event listeners). The production inspector + view code is unchanged.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl5PocPath        = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl5ViewPath       = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl5InspectorPath  = "explorer/assets/js/graph/authority/authority-graph-inspector.js"
	d33aSpike2gImpl5ThemeName      = "authority-thin-card-v1"
)

func d33aSpike2gImpl5ReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl5PocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5: cannot read PoC at %s: %v", d33aSpike2gImpl5PocPath, err)
	}
	return string(b)
}

// d33aSpike2gImpl5CarrierBlock returns the source slice containing the
// inspector carrier helpers — bounded by the section header and the
// next existing function (`_destroyCy`). Used for assertions that
// must be scoped to the carrier code.
func d33aSpike2gImpl5CarrierBlock(t *testing.T, js string) string {
	t.Helper()
	const header = "// ── D33a-spike-2g-impl-5 — Inspector carrier DOM ─"
	start := strings.Index(js, header)
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5: carrier-DOM section header missing from PoC")
	}
	const tailMarker = "function _destroyCy("
	end := strings.Index(js[start:], tailMarker)
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-5: could not bound carrier-DOM block")
	}
	return js[start : start+end]
}

// ── 1. Carrier helpers declared ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5_CarrierDomHelperDeclared pins the
// three carrier helpers exist with the documented names.
func TestExplorer_D33aSpike2gImpl5_CarrierDomHelperDeclared(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)
	for _, want := range []string{
		"function _detailsForCarrier(",
		"function _clearInspectorCarriers(",
		"function _renderInspectorCarriers(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-5: carrier helper %q missing", want)
		}
	}
}

// ── 2. Carriers render under #gmap-canvas with the right markers ─────

// TestExplorer_D33aSpike2gImpl5_CarriersRenderUnderProductionCanvas
// pins that the carrier helper queries the production
// `#gmap-canvas` container, uses the production `.gmap-node` class,
// and marks each carrier with the PoC-only `data-cytoscape-poc-carrier`
// attribute so cleanup can target them precisely.
func TestExplorer_D33aSpike2gImpl5_CarriersRenderUnderProductionCanvas(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)
	block := d33aSpike2gImpl5CarrierBlock(t, js)

	if !strings.Contains(block, "document.getElementById('gmap-canvas')") {
		t.Error("D33a-spike-2g-impl-5: carrier helper must query #gmap-canvas via getElementById")
	}
	if !strings.Contains(block, "'gmap-node '") && !strings.Contains(block, "'gmap-node'") {
		t.Error("D33a-spike-2g-impl-5: carrier element must carry the production .gmap-node class")
	}
	if !strings.Contains(block, "cytoscape-poc-inspector-carrier") {
		t.Error("D33a-spike-2g-impl-5: carrier element must carry the cytoscape-poc-inspector-carrier marker class")
	}
	if !strings.Contains(block, "data-cytoscape-poc-carrier") {
		t.Error("D33a-spike-2g-impl-5: carrier element must carry the data-cytoscape-poc-carrier marker attribute (used by _clearInspectorCarriers)")
	}
}

// ── 3. Carrier dataset matches production inspector contract ─────────

// TestExplorer_D33aSpike2gImpl5_CarriersExposeProductionInspectorDataset
// pins the dataset.* contract the production inspector reads. The
// inspector's `selectNode(nodeId)` searches by `data-node-id` and
// reads `data-node-name` + `data-node-details` (JSON). Carriers
// must set all three attributes; the details JSON must include the
// `_kind` / `_id` / `_label` keys the production `_detailsFor` emits.
func TestExplorer_D33aSpike2gImpl5_CarriersExposeProductionInspectorDataset(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)
	block := d33aSpike2gImpl5CarrierBlock(t, js)

	for _, want := range []string{
		"setAttribute('data-node-id'",
		"setAttribute('data-node-name'",
		"setAttribute('data-node-details'",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D33a-spike-2g-impl-5: carrier must call %q so production inspector finds the dataset entry", want)
		}
	}

	// Details JSON construction must include the production
	// inspector keys: _kind / _id / _label. The production builder
	// (authority-graph-view.js:_detailsFor) uses these exact key
	// names; the inspector's per-kind formatters read them via
	// `details._kind` / `details._id` / `details._label`.
	for _, want := range []string{
		"_kind:",
		"_id:",
		"_label:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D33a-spike-2g-impl-5: carrier details JSON must include key %q (production inspector contract)", want)
		}
	}
}

// ── 4. Carrier details include raw projection data ──────────────────

// TestExplorer_D33aSpike2gImpl5_CarrierDetailsIncludeRawProjectionNode
// pins that the details JSON is built from the full backend
// projection node carried under `nodeData.raw` (not just from
// kind / id / label). The production `_detailsFor` flattens the
// kind-specific typed-data nested object — `_detailsForCarrier`
// must do the same so per-kind formatters see real fields.
func TestExplorer_D33aSpike2gImpl5_CarrierDetailsIncludeRawProjectionNode(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)
	block := d33aSpike2gImpl5CarrierBlock(t, js)

	// The helper must read from the mapped node's `raw` field.
	if !strings.Contains(block, "nodeData.raw") {
		t.Error("D33a-spike-2g-impl-5: _detailsForCarrier must read from nodeData.raw (full backend projection node)")
	}
	// And it must flatten the kind-specific typed-data nested
	// object — `raw[kind]` (decision_surface, authority_profile,
	// etc.) — matching the production _detailsFor() shape.
	if !strings.Contains(block, "raw[kind]") {
		t.Error("D33a-spike-2g-impl-5: _detailsForCarrier must flatten raw[kind] (typed-data nested object) — mirrors production _detailsFor")
	}
	// Object.keys iteration over the typed-data block — same
	// pattern as authority-graph-view.js:647-657.
	if !strings.Contains(block, "Object.keys(typed)") {
		t.Error("D33a-spike-2g-impl-5: _detailsForCarrier must iterate Object.keys(typed) to flatten the typed-data nested object")
	}
}

// ── 5. Carrier id uses the Cytoscape refKey verbatim ─────────────────

// TestExplorer_D33aSpike2gImpl5_CarrierNodeIdUsesCytoscapeRefKey
// pins that the carrier's `data-node-id` is the Cytoscape mapped
// node `data.id` — which is `<kind>:<id>` (production refKey
// format). No bespoke id format is invented in the carrier helper.
func TestExplorer_D33aSpike2gImpl5_CarrierNodeIdUsesCytoscapeRefKey(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)
	block := d33aSpike2gImpl5CarrierBlock(t, js)

	// The carrier must source the dataset id from the entry's
	// `data.id` (the Cytoscape mapped node's id, set in
	// mapProjectionToElements as `n.kind + ':' + n.id`).
	if !strings.Contains(block, "setAttribute('data-node-id', String(d.id))") {
		t.Error("D33a-spike-2g-impl-5: carrier data-node-id must come from the Cytoscape mapped node's d.id (production refKey)")
	}

	// And the underlying mapping must still produce the refKey
	// format — pin the existing production-compatible builder.
	if !strings.Contains(js, "var id = n.kind + ':' + n.id;") {
		t.Error("D33a-spike-2g-impl-5: mapProjectionToElements must still build id as n.kind + ':' + n.id (production _refKey shape)")
	}
}

// ── 6. Carrier lifecycle cleanup ─────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5_CarriersCleanedOnRerenderAndTeardown
// pins that `_clearInspectorCarriers()` is called before every
// re-render AND from the PoC teardown paths (`_destroyCy`,
// `_uninstallPoc`). The cleanup removes elements by the PoC
// marker attribute so it cannot accidentally delete production-
// painted `.gmap-node` cards.
func TestExplorer_D33aSpike2gImpl5_CarriersCleanedOnRerenderAndTeardown(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)

	// Helper bodies — bound to their function blocks for scoped
	// assertions.
	renderStart := strings.Index(js, "function _renderInspectorCarriers(")
	if renderStart < 0 {
		t.Fatal("D33a-spike-2g-impl-5: _renderInspectorCarriers helper missing")
	}
	renderEnd := strings.Index(js[renderStart:], "\n  }")
	renderBody := js[renderStart : renderStart+renderEnd]
	if !strings.Contains(renderBody, "_clearInspectorCarriers()") {
		t.Error("D33a-spike-2g-impl-5: _renderInspectorCarriers must call _clearInspectorCarriers() before re-render")
	}

	// _destroyCy must clear carriers before tearing down Cytoscape.
	destroyStart := strings.Index(js, "function _destroyCy(")
	if destroyStart < 0 {
		t.Fatal("D33a-spike-2g-impl-5: _destroyCy missing")
	}
	destroyEnd := strings.Index(js[destroyStart:], "\n  }")
	destroyBody := js[destroyStart : destroyStart+destroyEnd]
	if !strings.Contains(destroyBody, "_clearInspectorCarriers()") {
		t.Error("D33a-spike-2g-impl-5: _destroyCy must call _clearInspectorCarriers()")
	}

	// _uninstallPoc must clear carriers as part of the full teardown.
	uninstallStart := strings.Index(js, "function _uninstallPoc(")
	if uninstallStart < 0 {
		t.Fatal("D33a-spike-2g-impl-5: _uninstallPoc missing")
	}
	uninstallEnd := strings.Index(js[uninstallStart:], "\n  }")
	uninstallBody := js[uninstallStart : uninstallStart+uninstallEnd]
	if !strings.Contains(uninstallBody, "_clearInspectorCarriers()") {
		t.Error("D33a-spike-2g-impl-5: _uninstallPoc must call _clearInspectorCarriers()")
	}

	// The cleanup itself must select by the PoC marker attribute
	// (never by `.gmap-node` alone — that would risk deleting
	// production-painted cards if production rendering ever shares
	// the same #gmap-canvas). The function body references the
	// `_CARRIER_MARKER_ATTR` constant which is bound to
	// `'data-cytoscape-poc-carrier'` at the top of the carrier
	// block.
	clearStart := strings.Index(js, "function _clearInspectorCarriers(")
	clearEnd := strings.Index(js[clearStart:], "\n  }")
	clearBody := js[clearStart : clearStart+clearEnd]
	if !strings.Contains(clearBody, "_CARRIER_MARKER_ATTR") {
		t.Error("D33a-spike-2g-impl-5: _clearInspectorCarriers must select by the PoC marker attribute (_CARRIER_MARKER_ATTR), not by the bare .gmap-node class")
	}
	// And the constant itself must resolve to the documented
	// attribute name — this keeps the marker contract explicit.
	if !strings.Contains(js, `var _CARRIER_MARKER_ATTR = 'data-cytoscape-poc-carrier'`) {
		t.Error("D33a-spike-2g-impl-5: _CARRIER_MARKER_ATTR must be bound to 'data-cytoscape-poc-carrier' so cleanup matches the marker attribute set by _renderInspectorCarriers")
	}
}

// ── 7. Carriers are hidden + aria-hidden ─────────────────────────────

// TestExplorer_D33aSpike2gImpl5_CarriersAreHiddenAndAriaHidden pins
// the non-visual contract: every carrier sets HTML5 `hidden`,
// inline `display: none`, and `aria-hidden="true"`. The carrier
// must never affect layout or accessibility tooling.
func TestExplorer_D33aSpike2gImpl5_CarriersAreHiddenAndAriaHidden(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)
	block := d33aSpike2gImpl5CarrierBlock(t, js)

	if !strings.Contains(block, "carrier.hidden = true") {
		t.Error("D33a-spike-2g-impl-5: carrier must set HTML5 `hidden = true`")
	}
	if !strings.Contains(block, "carrier.setAttribute('aria-hidden', 'true')") {
		t.Error("D33a-spike-2g-impl-5: carrier must set aria-hidden='true'")
	}
	if !strings.Contains(block, "carrier.style.display = 'none'") {
		t.Error("D33a-spike-2g-impl-5: carrier must set inline display:none (defence against CSS resets)")
	}
}

// ── 8. Tap routing through the renderer hook preserved ───────────────

// TestExplorer_D33aSpike2gImpl5_TapStillRoutesThroughRendererHook
// pins that the precheck's selection routing survives impl-5 — the
// node tap handler still calls `_rendererHooks.selectNode(nodeId)`
// with try/catch safety, AND the PoC inspector aside continues to
// render alongside the production-inspector handoff.
func TestExplorer_D33aSpike2gImpl5_TapStillRoutesThroughRendererHook(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)

	const opener = "_cy.on('tap', 'node', function (evt) {"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5: cy.on('tap', 'node', …) handler missing")
	}
	end := strings.Index(js[start+len(opener):], "_cy.on('tap'")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-5: could not bound the tap handler body")
	}
	body := js[start : start+len(opener)+end]

	for _, want := range []string{
		"_rendererHooks",
		"hooks.selectNode(nodeId)",
		"_renderInspector(node)", // PoC inspector still renders
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-5: tap handler must still contain %q", want)
		}
	}
	if !strings.Contains(body, "try {") || !strings.Contains(body, "} catch (_) {") {
		t.Error("D33a-spike-2g-impl-5: tap-handler hook routing must remain wrapped in try/catch")
	}
}

// ── 9. PoC inspector still present ───────────────────────────────────

// TestExplorer_D33aSpike2gImpl5_PocInspectorStillPresent pins that
// the PoC inspector aside has NOT been removed in this tranche.
// Removal is reserved for a later tranche after browser verification.
func TestExplorer_D33aSpike2gImpl5_PocInspectorStillPresent(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)
	for _, want := range []string{
		"function _renderInspector(node)",
		"function _renderInspectorEmpty(",
		"_inspectorEl",
		"cytoscape-poc-inspector",
		"_renderInspector(node)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-5: PoC inspector surface element %q must remain", want)
		}
	}
}

// ── 10. _lastAuthorityProjection still populated from PoC ────────────

// TestExplorer_D33aSpike2gImpl5_LastAuthorityProjectionStillPopulated
// pins that the precheck's projection cache write survives impl-5.
// The production inspector's `_buildOverlayHTML` and the Authority
// Workbench both read this cache.
func TestExplorer_D33aSpike2gImpl5_LastAuthorityProjectionStillPopulated(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)
	if !strings.Contains(js, "window.MIDASExplorerGraph._lastAuthorityProjection = payload") {
		t.Error("D33a-spike-2g-impl-5: PoC _renderPayload must still write _lastAuthorityProjection")
	}
}

// ── 11. Production inspector untouched (DOM contract preserved) ──────

// TestExplorer_D33aSpike2gImpl5_ProductionInspectorUnchanged pins
// that impl-5 chose the carrier-DOM path over an inspector refactor.
// The production inspector source must still query `.gmap-node`,
// read `dataset.nodeDetails`, and early-return when no selected node
// is found — the exact contract the carrier DOM satisfies.
func TestExplorer_D33aSpike2gImpl5_ProductionInspectorUnchanged(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl5InspectorPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5: cannot read inspector at %s: %v", d33aSpike2gImpl5InspectorPath, err)
	}
	body := string(b)
	for _, want := range []string{
		"querySelectorAll('.gmap-node')",
		"dataset.nodeDetails",
		"if (!selectedNode) return;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-5: production inspector source must still contain %q (impl-5 chose carrier DOM, not inspector refactor)", want)
		}
	}
	// Production inspector must NOT have learned about the PoC
	// carriers — the wiring must remain a one-way producer/consumer
	// contract.
	for _, banned := range []string{
		"cytoscape-poc-inspector-carrier",
		"data-cytoscape-poc-carrier",
		"_renderInspectorCarriers",
		"_clearInspectorCarriers",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-5: production inspector must not reference PoC carrier name %q", banned)
		}
	}
}

// ── 12. Production view untouched ────────────────────────────────────

// TestExplorer_D33aSpike2gImpl5_ProductionAuthorityViewUnaffected
// pins that production `authority-graph-view.js` was not touched by
// impl-5 — no carrier references, no PoC theme name, no Cytoscape
// helper invocations.
func TestExplorer_D33aSpike2gImpl5_ProductionAuthorityViewUnaffected(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl5ViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5: cannot read Authority view at %s: %v", d33aSpike2gImpl5ViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		"cytoscape-poc-inspector-carrier",
		"data-cytoscape-poc-carrier",
		"_renderInspectorCarriers",
		"_clearInspectorCarriers",
		"_detailsForCarrier",
		d33aSpike2gImpl5ThemeName,
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-5: production Authority view must not reference %q", banned)
		}
	}
}

// ── 13. Existing card theme preserved ────────────────────────────────

// TestExplorer_D33aSpike2gImpl5_AuthorityThinCardThemeUnchanged pins
// that the impl-4c → impl-4e single-line thin-card contract is not
// regressed by the carrier-DOM addition. The visible label binding
// stays on `_displayTitle`, no `_displayCardLabel`, and the
// strategic-symbol helper remains in source.
func TestExplorer_D33aSpike2gImpl5_AuthorityThinCardThemeUnchanged(t *testing.T) {
	js := d33aSpike2gImpl5ReadPoc(t)

	if !strings.Contains(js, "'"+d33aSpike2gImpl5ThemeName+"'") {
		t.Errorf("D33a-spike-2g-impl-5: _THEMES must still contain %q", d33aSpike2gImpl5ThemeName)
	}

	// Locate the thin-card style branch and pin the single-line
	// label binding (and the explicit absence of the composer).
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-5: `%s` branch missing", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-5: could not bound thin-card branch")
	}
	branch := tail[:end]

	if !strings.Contains(branch, "_displayTitle(ele") {
		t.Error("D33a-spike-2g-impl-5: thin-card branch must still bind label via _displayTitle(ele, …)")
	}
	if strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-5: thin-card branch must NOT reintroduce _displayCardLabel — single-line contract")
	}

	// Strategic symbol model remains.
	if !strings.Contains(js, "function _strategicSymbolsForNode(") {
		t.Error("D33a-spike-2g-impl-5: _strategicSymbolsForNode helper must remain")
	}
	if !strings.Contains(branch, "icons.cytoscapeDataURI(syms[") {
		t.Error("D33a-spike-2g-impl-5: thin-card branch must still resolve symbol icons via icons.cytoscapeDataURI(...)")
	}
}
