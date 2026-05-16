package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl5_precheck_test.go — D33a-spike-2g-impl-5-precheck.
//
// Validation pass on the Cytoscape PoC → MIDAS right-side inspector
// handoff. The PoC's node-tap handler now ALSO routes the selected
// ref key through `MIDASExplorerGraph._rendererHooks.selectNode(id)`
// — the same lens-aware selection function the production graph
// uses — and the PoC's render path caches the active projection on
// `MIDASExplorerGraph._lastAuthorityProjection` so the production
// inspector overlay + authority workbench can read diagnostics +
// surface_posture while the PoC is active.
//
// The PoC's own inspector aside is NOT removed in this tranche;
// the validation proves the upstream selection wiring without
// disturbing the PoC visual.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl5PrecheckPocPath  = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl5PrecheckViewPath = "explorer/assets/js/graph/authority/authority-graph-view.js"
)

func d33aSpike2gImpl5PrecheckReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl5PrecheckPocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5-precheck: cannot read PoC at %s: %v", d33aSpike2gImpl5PrecheckPocPath, err)
	}
	return string(b)
}

// ── 1. Cytoscape mapped node carries inspector-shaped data ────────────

// TestExplorer_D33aSpike2gImpl5Precheck_CytoscapeNodeDataContainsInspectorFields
// pins that `mapProjectionToElements` emits each mapped node with the
// fields the MIDAS right-side inspector consumes when it would
// otherwise read them off a DOM element: id (kind:id ref key), kind,
// label, and the raw backend node payload under `raw:`.
func TestExplorer_D33aSpike2gImpl5Precheck_CytoscapeNodeDataContainsInspectorFields(t *testing.T) {
	js := d33aSpike2gImpl5PrecheckReadPoc(t)

	// Locate the mapper.
	start := strings.Index(js, "function mapProjectionToElements(")
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5-precheck: mapProjectionToElements helper missing")
	}
	// Body extends to the next blank-line + 2-space `function ` decl.
	end := strings.Index(js[start+1:], "\n  function ")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-5-precheck: could not bound mapProjectionToElements body")
	}
	body := js[start : start+1+end]

	// The mapper must build `id = n.kind + ':' + n.id` — the same
	// `<kind>:<id>` refKey shape the production `_refKey({kind,id})`
	// helper produces.
	if !strings.Contains(body, "var id = n.kind + ':' + n.id") {
		t.Error("D33a-spike-2g-impl-5-precheck: mapped node id must be n.kind + ':' + n.id (production refKey format)")
	}

	// Each mapped node must carry the inspector-shaped fields.
	for _, want := range []string{
		"id:    id",
		"kind:  n.kind",
		"label: labelText",
		"raw:   n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-5-precheck: mapped node data must include %q", want)
		}
	}
}

// ── 2. Tap handler routes selection to the MIDAS inspector ────────────

// TestExplorer_D33aSpike2gImpl5Precheck_CytoscapeTapRoutesSelectionToMidasInspector
// pins that the Cytoscape node tap handler calls into the production
// renderer-hooks shim — `MIDASExplorerGraph._rendererHooks.selectNode(id)`
// — which in turn lens-dispatches to `authorityInspector.selectNode`
// when the active lens is 'authority'. The hook reads the same
// refKey shape the production graph uses, so no payload translation
// is needed.
func TestExplorer_D33aSpike2gImpl5Precheck_CytoscapeTapRoutesSelectionToMidasInspector(t *testing.T) {
	js := d33aSpike2gImpl5PrecheckReadPoc(t)

	// Locate the tap-on-node handler so the assertion is bounded to
	// the handler body, not the whole file.
	const opener = "_cy.on('tap', 'node', function (evt) {"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-5-precheck: cy.on('tap', 'node', …) handler missing")
	}
	// Bound at the next top-level `_cy.on(` declaration.
	end := strings.Index(js[start+len(opener):], "_cy.on('tap'")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-5-precheck: could not bound the tap handler body")
	}
	body := js[start : start+len(opener)+end]

	for _, want := range []string{
		"_rendererHooks",
		"hooks.selectNode(nodeId)",
		"node.data('id')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-5-precheck: tap handler must route selection via %q", want)
		}
	}

	// The selection routing must be wrapped in a try/catch so an
	// unexpected hook failure cannot break the existing PoC tap
	// behaviour (focus, root-path emphasis, PoC inspector render).
	if !strings.Contains(body, "try {") || !strings.Contains(body, "} catch (_) {") {
		t.Error("D33a-spike-2g-impl-5-precheck: tap-handler selection routing must be guarded with try/catch so a missing hook does not break tap behaviour")
	}
}

// ── 3. PoC inspector still present ───────────────────────────────────

// TestExplorer_D33aSpike2gImpl5Precheck_PocInspectorStillPresent pins
// that this tranche has NOT removed the Cytoscape PoC's own
// inspector aside. The PoC inspector remains the visible source of
// truth for selected-node data while the right-side inspector wiring
// is validated. Removal is reserved for a later tranche.
func TestExplorer_D33aSpike2gImpl5Precheck_PocInspectorStillPresent(t *testing.T) {
	js := d33aSpike2gImpl5PrecheckReadPoc(t)
	for _, want := range []string{
		"function _renderInspector(node)",
		"function _renderInspectorEmpty(",
		"_inspectorEl",
		"cytoscape-poc-inspector",
		"_renderInspector(node)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-5-precheck: PoC inspector surface element %q must remain (removal deferred to a later tranche)", want)
		}
	}
}

// ── 4. _lastAuthorityProjection populated from the PoC render ────────

// TestExplorer_D33aSpike2gImpl5Precheck_LastAuthorityProjectionPopulatedIfRequired
// pins that the PoC's `_renderPayload` writes the active projection
// to `window.MIDASExplorerGraph._lastAuthorityProjection`. The
// production `authority-graph-inspector._buildOverlayHTML` and the
// production `authority-graph-workbench._projection` both read this
// cache for diagnostics + surface_posture; without it, those
// overlays render empty under the PoC lens.
func TestExplorer_D33aSpike2gImpl5Precheck_LastAuthorityProjectionPopulatedIfRequired(t *testing.T) {
	js := d33aSpike2gImpl5PrecheckReadPoc(t)

	// The cache must be written from within the PoC's render path.
	if !strings.Contains(js, "window.MIDASExplorerGraph._lastAuthorityProjection = payload") {
		t.Error("D33a-spike-2g-impl-5-precheck: PoC render must populate window.MIDASExplorerGraph._lastAuthorityProjection from the active payload")
	}

	// Verify the production view continues to own the cache write
	// for its own render path (the PoC cache write does not replace
	// the production write; they both target the same namespace
	// slot under their respective lens-render paths).
	viewBody, err := os.ReadFile(d33aSpike2gImpl5PrecheckViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5-precheck: cannot read Authority view at %s: %v", d33aSpike2gImpl5PrecheckViewPath, err)
	}
	if !strings.Contains(string(viewBody), "window.MIDASExplorerGraph._lastAuthorityProjection = spec") {
		t.Error("D33a-spike-2g-impl-5-precheck: production view must still write _lastAuthorityProjection for its own render path (no production change in this tranche)")
	}
}

// ── 5. Production Authority view untouched ───────────────────────────

// TestExplorer_D33aSpike2gImpl5Precheck_ProductionAuthorityGraphUnaffected
// pins that the production Authority view file has NOT been
// modified to reach into the Cytoscape PoC: no PoC theme name,
// no PoC helper, no `cytoscape` / `cyTheme` reference, no
// reference to `MIDASExplorerIcons` / `cytoscapeDataURI` from
// the PoC icon vocabulary.
func TestExplorer_D33aSpike2gImpl5Precheck_ProductionAuthorityGraphUnaffected(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl5PrecheckViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-5-precheck: cannot read Authority view at %s: %v", d33aSpike2gImpl5PrecheckViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		"authority-thin-card-v1",
		"_strategicSymbolsForNode",
		"_effectiveLayout",
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-5-precheck: production Authority view must not reference %q", banned)
		}
	}
}
