package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37m-impl-1 — Authority Canvas-Edge Context Tabs Implementation
//
// Implements the v1 design locked in
//   docs/design/D37m-design-1-authority-canvas-edge-tab-information-architecture.md
//
// Three compact right-side canvas-edge tabs (Details, Authority,
// Evidence) for the Authority Cytoscape graph. Hosts the modular
// boundary the design contract requires:
//
//   Section A — Shell controller     (DOM, ARIA, keyboard, lifecycle)
//   Section B — Authority adapter    (pure model builders)
//   Section C — Tab renderers        (DOM from prepared models)
//   Section D — Workbench bridge     (thin setActiveTab wrapper)
//   Section E — Public surface       (window.MIDASExplorerGraph.authorityCanvasEdgeTabs)
//   Section F — Lazy lifecycle bootstrap
//
// Asset-text pins per the §14 plan in the design document. Browser
// validation remains operator-driven.

const (
	d37mShellAsset   = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37mTabsAsset    = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37mTabsCSSPath  = "/explorer/assets/css/authority-canvas-edge-tabs.css"
	d37mToolbarAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
)

func readD37mFunctionBody(t *testing.T, js, signature string) string {
	t.Helper()
	start := strings.Index(js, signature)
	if start < 0 {
		t.Fatalf("D37m: function %q not found", signature)
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatalf("D37m: cannot bound function %q", signature)
	}
	return js[start : start+end]
}

// ── 14.1 Module presence and lifecycle ────────────────────────────────

// TestExplorer_D37m_ModuleAssetIsServed pins that the new JS module
// is served at the expected path.
func TestExplorer_D37m_ModuleAssetIsServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	if len(js) == 0 {
		t.Fatal("D37m: authority-canvas-edge-tabs.js must be served (length 0)")
	}
}

// TestExplorer_D37m_CssAssetIsServed pins that the new CSS file is
// served at the expected path.
func TestExplorer_D37m_CssAssetIsServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37mTabsCSSPath)
	if len(css) == 0 {
		t.Fatal("D37m: authority-canvas-edge-tabs.css must be served (length 0)")
	}
}

// TestExplorer_D37m_IndexIncludesBothAssets pins that index.html
// includes both new assets.
func TestExplorer_D37m_IndexIncludesBothAssets(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`href="/explorer/assets/css/authority-canvas-edge-tabs.css"`,
		`src="/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: index.html must include %q", want)
		}
	}
}

// TestExplorer_D37m_ExactlyOneWrapper pins that there is exactly
// one canvas-edge tabs wrapper in the markup.
func TestExplorer_D37m_ExactlyOneWrapper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	count := strings.Count(body, `data-authority-canvas-edge-tabs`)
	if count != 1 {
		t.Errorf("D37m: expected exactly 1 `data-authority-canvas-edge-tabs` wrapper in index.html, found %d", count)
	}
}

// ── 14.2 DOM shape ────────────────────────────────────────────────────

// TestExplorer_D37m_ThreeTabButtonsPresent pins the three tab
// buttons with their stable data attributes.
func TestExplorer_D37m_ThreeTabButtonsPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`data-canvas-edge-tab="details"`,
		`data-canvas-edge-tab="authority"`,
		`data-canvas-edge-tab="evidence"`,
		`id="gmap-canvas-edge-tab-details"`,
		`id="gmap-canvas-edge-tab-authority"`,
		`id="gmap-canvas-edge-tab-evidence"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: tab button markup must include %q", want)
		}
	}
}

// TestExplorer_D37m_TabsCarryAriaAttributes pins ARIA on the tab
// buttons.
func TestExplorer_D37m_TabsCarryAriaAttributes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`role="tab"`,
		`aria-selected="false"`,
		`aria-controls="gmap-canvas-edge-pane"`,
		`aria-disabled="true"`,
		`tabindex="-1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: tab buttons must carry ARIA attribute %q", want)
		}
	}

	// Also pin the role="tablist" + vertical orientation on the strip.
	for _, want := range []string{
		`role="tablist"`,
		`aria-orientation="vertical"`,
		`aria-label="Authority selected-object context tabs"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: tab strip must carry %q", want)
		}
	}
}

// TestExplorer_D37m_PaneCarriesRoleAndHidden pins the pane element's
// role, hidden attribute, and tabindex.
func TestExplorer_D37m_PaneCarriesRoleAndHidden(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Locate the pane element opening tag.
	paneIdx := strings.Index(body, `id="gmap-canvas-edge-pane"`)
	if paneIdx < 0 {
		t.Fatal("D37m: pane element with id `gmap-canvas-edge-pane` missing")
	}
	// Bound the assertion to the opening tag — find the next `>`.
	tagEnd := strings.Index(body[paneIdx:], ">")
	if tagEnd < 0 {
		t.Fatal("D37m: cannot bound pane opening tag")
	}
	tag := body[paneIdx-200 : paneIdx+tagEnd+1] // include a little context backwards
	if !strings.Contains(tag, `role="tabpanel"`) {
		t.Errorf("D37m: pane must declare role=\"tabpanel\" — tag context:\n%s", tag)
	}
	if !strings.Contains(tag, `tabindex="-1"`) {
		t.Errorf("D37m: pane must declare tabindex=\"-1\"")
	}
	if !strings.Contains(tag, "hidden") {
		t.Errorf("D37m: pane must have the `hidden` attribute initially")
	}
}

// TestExplorer_D37m_PaneHasHeaderBodyFooter pins the three
// sub-containers of the pane.
func TestExplorer_D37m_PaneHasHeaderBodyFooter(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`data-canvas-edge-tabs-header`,
		`data-canvas-edge-tabs-body`,
		`data-canvas-edge-tabs-footer`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: pane container marker %q must be present", want)
		}
	}
}

// TestExplorer_D37m_TabsStartDisabled pins that all 3 tab buttons
// start `aria-disabled="true"` + `disabled`.
func TestExplorer_D37m_TabsStartDisabled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, tabID := range []string{"details", "authority", "evidence"} {
		needle := `data-canvas-edge-tab="` + tabID + `"`
		idx := strings.Index(body, needle)
		if idx < 0 {
			t.Fatalf("D37m: tab button %q not found", tabID)
		}
		// Look at the opening tag — bounded between the tag-open `<` before idx
		// and the next `>`. Find the `<button` start.
		tagStart := strings.LastIndex(body[:idx], "<button")
		tagEnd := strings.Index(body[idx:], ">")
		if tagStart < 0 || tagEnd < 0 {
			t.Fatalf("D37m: cannot bound tab button %q opening tag", tabID)
		}
		tag := body[tagStart : idx+tagEnd+1]
		if !strings.Contains(tag, `aria-disabled="true"`) {
			t.Errorf("D37m: tab %q must start aria-disabled=\"true\" — tag: %s", tabID, tag)
		}
		if !strings.Contains(tag, ` disabled`) {
			t.Errorf("D37m: tab %q must start with `disabled` attribute — tag: %s", tabID, tag)
		}
	}
}

// ── 14.3 Modularity guardrails ────────────────────────────────────────

// TestExplorer_D37m_NoLargeInlineImplementationInIndexHtml pins
// that no `<script>` block inside index.html carries the canvas-edge
// implementation. The wrapper markup is allowed (~12 lines), and
// the per-button SVG icons are allowed. Inline scripts must not
// contain the implementation symbols.
func TestExplorer_D37m_NoLargeInlineImplementationInIndexHtml(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Search every `<script>...</script>` inline block (excluding
	// external `<script src=...></script>` references). Inline
	// scripts must not reference any of the implementation
	// identifiers.
	pos := 0
	for {
		open := strings.Index(body[pos:], "<script")
		if open < 0 {
			break
		}
		open += pos
		closeTag := strings.Index(body[open:], "</script>")
		if closeTag < 0 {
			break
		}
		block := body[open : open+closeTag+len("</script>")]
		pos = open + closeTag + len("</script>")
		// Skip external script tags (no closing body content).
		if strings.Contains(block, " src=") || strings.Contains(block, " src='") {
			continue
		}
		for _, banned := range []string{
			"renderDetailsTab",
			"renderAuthorityTab",
			"renderEvidenceTab",
			"_buildDetailsModel",
			"_buildAuthorityModel",
			"_buildEvidenceModel",
			"launchWorkbenchTab",
			"MIDASExplorerGraph.authorityCanvasEdgeTabs",
		} {
			if strings.Contains(block, banned) {
				t.Errorf("D37m: inline <script> in index.html must not contain %q (modularity)", banned)
			}
		}
	}
}

// TestExplorer_D37m_NotEmbeddedInAuthorityCytoscapePoc pins that
// `authority-cytoscape-poc.js` does NOT host the canvas-edge tabs
// implementation. Wrong file = test failure.
func TestExplorer_D37m_NotEmbeddedInAuthorityCytoscapePoc(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mShellAsset)

	for _, banned := range []string{
		"renderDetailsTab",
		"renderAuthorityTab",
		"renderEvidenceTab",
		"_buildDetailsModel",
		"_buildAuthorityModel",
		"_buildEvidenceModel",
		"MIDASExplorerGraph.authorityCanvasEdgeTabs",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37m: authority-cytoscape-poc.js must NOT host canvas-edge tabs — found %q", banned)
		}
	}
}

// TestExplorer_D37m_SectionMarkersPresent pins the six locked
// section markers in the new module.
func TestExplorer_D37m_SectionMarkersPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	for _, want := range []string{
		"Section A — Shell controller",
		"Section B — Authority data adapter",
		"Section C — Tab renderers",
		"Section D — Workbench bridge",
		"Section E — Public surface",
		"Section F — Lazy lifecycle bootstrap",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m: section marker %q must appear in the module", want)
		}
	}
}

// TestExplorer_D37m_RendererPurity is the critical adapter-purity
// guardrail. Renderer function bodies must NOT read global state —
// no `_lastAuthorityProjection`, no `cy.elements`, no `getCy(`, no
// `onSelectionChanged`. The renderers receive prepared models and
// nothing else.
func TestExplorer_D37m_RendererPurity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	renderers := []string{
		"function renderDetailsTab(bodyEl, footerEl, model)",
		"function renderAuthorityTab(bodyEl, footerEl, model)",
		"function renderEvidenceTab(bodyEl, footerEl, model)",
	}
	bannedGlobals := []string{
		"_lastAuthorityProjection",
		"cy.elements",
		"getCy(",
		"onSelectionChanged",
		"onAuthorityContextChanged",
	}
	for _, fnSig := range renderers {
		body := readD37mFunctionBody(t, js, fnSig)
		for _, banned := range bannedGlobals {
			if strings.Contains(body, banned) {
				t.Errorf("D37m: %s must NOT read global %q (adapter purity)", fnSig, banned)
			}
		}
	}
}

// ── 14.4 Tab content contracts ────────────────────────────────────────

// TestExplorer_D37m_FieldDefsCoverSevenKinds pins that the
// per-kind field-defs map covers all 7 Authority node kinds with
// `specific` + `technical` arrays.
func TestExplorer_D37m_FieldDefsCoverSevenKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	for _, kind := range []string{
		"business_service",
		"decision_surface",
		"authority_profile",
		"authority_grant",
		"agent",
		"fail_mode_policy",
		"escalation_target",
	} {
		if !strings.Contains(js, kind+":") {
			t.Errorf("D37m: FIELD_DEFS must include kind key %q", kind)
		}
	}
}

// TestExplorer_D37m_DetailsUsesDetailsDisclosure pins that the
// Details renderer uses a `<details>` element for the Technical
// disclosure (collapsed by default).
func TestExplorer_D37m_DetailsUsesDetailsDisclosure(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function renderDetailsTab(bodyEl, footerEl, model)")

	if !strings.Contains(body, "createElement('details')") {
		t.Errorf("D37m: Details renderer must use <details> for the Technical disclosure — body:\n%s", body)
	}
	if !strings.Contains(body, "createElement('summary')") {
		t.Errorf("D37m: Details renderer must use <summary> inside <details>")
	}
}

// TestExplorer_D37m_AuthorityUsesComputeAuthorityContext pins that
// the Authority chain builder calls the D37j context helper.
func TestExplorer_D37m_AuthorityUsesComputeAuthorityContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	if !strings.Contains(js, "_computeAuthorityContext") {
		t.Errorf("D37m: Authority builder must invoke `_computeAuthorityContext`")
	}
	// And it should appear in the chain-computation helper body.
	body := readD37mFunctionBody(t, js, "function _computeChainForCtx(ctx)")
	if !strings.Contains(body, "_computeAuthorityContext") {
		t.Errorf("D37m: _computeChainForCtx must call _computeAuthorityContext — body:\n%s", body)
	}
}

// TestExplorer_D37m_NoSecondCytoscapeInstance pins the negative:
// the new module must NOT instantiate a Cytoscape graph.
func TestExplorer_D37m_NoSecondCytoscapeInstance(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	for _, banned := range []string{
		"new window.cytoscape(",
		"window.cytoscape({",
		"new cytoscape(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37m: module must NOT instantiate Cytoscape — found %q", banned)
		}
	}
}

// TestExplorer_D37m_AuthorityViewContextCallsToggle pins that the
// Authority "View context" footer button calls
// `cytoscapePoc.toggleAuthorityContext`.
func TestExplorer_D37m_AuthorityViewContextCallsToggle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function renderAuthorityTab(bodyEl, footerEl, model)")

	for _, want := range []string{
		"toggleAuthorityContext",
		`'view-context'`,
		`aria-pressed`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: renderAuthorityTab must include %q (View context wiring)", want)
		}
	}
}

// TestExplorer_D37m_EvidenceFiltersByNodeRefs pins that the
// Evidence model builder filters projection diagnostics by
// `node_refs` containing the focal node's kind+id.
func TestExplorer_D37m_EvidenceFiltersByNodeRefs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _buildEvidenceModel(ctx, projection)")

	for _, want := range []string{
		"projection.diagnostics",
		"node_refs",
		"ref.kind === ctx.kind",
		"ref.id === ctx.id",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: _buildEvidenceModel must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37m_EvidenceRuntimeNoticePresent pins the exact
// "Runtime evidence overlay is not yet wired" copy.
func TestExplorer_D37m_EvidenceRuntimeNoticePresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	if !strings.Contains(js, "Runtime evidence overlay is not yet wired for the Authority lens.") {
		t.Errorf("D37m: module must include the locked runtime-not-wired copy")
	}
}

// TestExplorer_D37m_NoBackendCallsFromTabs pins that the module
// makes no HTTP calls and does not reference the evidence backend.
func TestExplorer_D37m_NoBackendCallsFromTabs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	exec := stripJSComments(js)

	for _, banned := range []string{
		"fetch(",
		"/v1/evidence",
		"/v1/graphs",
		"XMLHttpRequest",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D37m: module must NOT make HTTP calls — found %q in executable JS", banned)
		}
	}
}

// ── 14.5 Workbench bridge ─────────────────────────────────────────────

// TestExplorer_D37m_LaunchWorkbenchTabExists pins the bridge
// function and its call to authorityWorkbench.setActiveTab — the
// actual public surface registered by authority-graph-workbench.js
// (D37m-fix-1: corrected from authorityGraphWorkbench).
func TestExplorer_D37m_LaunchWorkbenchTabExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	if !strings.Contains(js, "function launchWorkbenchTab(kind, canvasEdgeTab)") {
		t.Errorf("D37m: launchWorkbenchTab function must exist")
	}
	body := readD37mFunctionBody(t, js, "function launchWorkbenchTab(kind, canvasEdgeTab)")
	for _, want := range []string{
		"authorityWorkbench",
		"setActiveTab",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: launchWorkbenchTab must reference %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37m_WorkbenchMappingHasAllCombinations pins the
// 7×3 = 21 combinations of (focal kind, canvas-edge tab) → workbench tab.
func TestExplorer_D37m_WorkbenchMappingHasAllCombinations(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	// Bound to the WORKBENCH_MAPPING declaration.
	start := strings.Index(js, "var WORKBENCH_MAPPING = {")
	if start < 0 {
		t.Fatal("D37m: WORKBENCH_MAPPING not found")
	}
	end := strings.Index(js[start:], "};")
	if end < 0 {
		t.Fatal("D37m: cannot bound WORKBENCH_MAPPING")
	}
	block := js[start : start+end]

	// Per-kind mapping per design §9.
	mapping := []struct {
		kind         string
		expectStrings []string
	}{
		{"business_service",  []string{`details: 'overview'`,    `authority: 'overview'`,  `evidence: 'evidence'`}},
		{"decision_surface",  []string{`details: 'fail-mode'`,   `authority: 'grants'`,    `evidence: 'evidence'`}},
		{"authority_profile", []string{`details: 'escalation'`,  `authority: 'grants'`,    `evidence: 'evidence'`}},
		{"authority_grant",   []string{`details: 'grants'`,      `authority: 'grants'`,    `evidence: 'evidence'`}},
		{"agent",             []string{`details: 'grants'`,      `authority: 'grants'`,    `evidence: 'evidence'`}},
		{"fail_mode_policy",  []string{`details: 'fail-mode'`,   `authority: 'fail-mode'`, `evidence: 'evidence'`}},
		{"escalation_target", []string{`details: 'escalation'`,  `authority: 'escalation'`, `evidence: 'evidence'`}},
	}
	for _, m := range mapping {
		// Each kind appears once as a top-level map entry. Find the kind's row.
		kindStart := strings.Index(block, m.kind+":")
		if kindStart < 0 {
			t.Errorf("D37m: WORKBENCH_MAPPING must contain kind %q", m.kind)
			continue
		}
		// Bound to closing brace of that row.
		rowEnd := strings.Index(block[kindStart:], "},")
		if rowEnd < 0 {
			t.Errorf("D37m: cannot bound row for %q", m.kind)
			continue
		}
		row := block[kindStart : kindStart+rowEnd]
		for _, want := range m.expectStrings {
			if !strings.Contains(row, want) {
				t.Errorf("D37m: WORKBENCH_MAPPING[%q] must include %q — row:\n%s", m.kind, want, row)
			}
		}
	}
}

// TestExplorer_D37m_WorkbenchCopyCovers5Targets pins the 5 button
// copy strings.
func TestExplorer_D37m_WorkbenchCopyCovers5Targets(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	for _, want := range []string{
		"Open Overview in workbench →",
		"Open Fail-Mode in workbench →",
		"Open Escalation in workbench →",
		"Open Grants in workbench →",
		"Open Evidence in workbench →",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m: workbench launcher copy must include %q", want)
		}
	}
}

// ── 14.6 Public surface ───────────────────────────────────────────────

// TestExplorer_D37m_PublicSurfaceKeys pins the registered public
// API on `window.MIDASExplorerGraph.authorityCanvasEdgeTabs`.
func TestExplorer_D37m_PublicSurfaceKeys(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	declStart := strings.Index(js, "window.MIDASExplorerGraph.authorityCanvasEdgeTabs = {")
	if declStart < 0 {
		t.Fatal("D37m: public-surface registration not found")
	}
	declEnd := strings.Index(js[declStart:], "};")
	if declEnd < 0 {
		t.Fatal("D37m: cannot bound public-surface declaration")
	}
	block := js[declStart : declStart+declEnd]

	for _, want := range []string{
		"init:",
		"destroy:",
		"render:",
		"openTab:",
		"closePane:",
		"syncSelection:",
		"isOpen:",
		// Diagnostic surface.
		"_renderDetailsTab:",
		"_renderAuthorityTab:",
		"_renderEvidenceTab:",
		"_buildDetailsModel:",
		"_buildAuthorityModel:",
		"_buildEvidenceModel:",
		"_mapWorkbenchTarget:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37m: public surface must expose %q", want)
		}
	}
}

// ── 14.7 Scope guards ─────────────────────────────────────────────────

// TestExplorer_D37m_OnlyReadsProjectionNeverWrites pins that the
// module reads _lastAuthorityProjection but never writes to it.
func TestExplorer_D37m_OnlyReadsProjectionNeverWrites(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	// Pin a read access exists.
	if !strings.Contains(js, "_lastAuthorityProjection") {
		t.Errorf("D37m: module must reference _lastAuthorityProjection (read)")
	}
	// Pin no writes.
	for _, banned := range []string{
		"_lastAuthorityProjection =",
		"_lastAuthorityProjection=",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37m: module must NOT write to _lastAuthorityProjection — found %q", banned)
		}
	}
}

// TestExplorer_D37m_NoNewDependency pins the no-new-dependency
// boundary.
func TestExplorer_D37m_NoNewDependency(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	exec := stripJSComments(js)

	for _, banned := range []string{
		"require('qtip",
		"require('popper",
		"require('@floating-ui",
		"require('tippy",
		"import 'qtip",
		"import 'popper",
		"import '@floating-ui",
		"import 'tippy",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D37m: no new dependency permitted — found %q", banned)
		}
	}
}

// TestExplorer_D37m_RightDrawerUntouched pins that the right
// drawer's registration and rail markup remain.
func TestExplorer_D37m_RightDrawerUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-details"`,
		`gmap-right-rail`,
		`id="gmap-rail-panel-inspector"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m: right drawer markup %q must remain (no removal in this tranche)", want)
		}
	}
}

// ── 14.8 Foundation preservation ──────────────────────────────────────

// TestExplorer_D37m_PreservesD37fContracts pins D37f constants.
func TestExplorer_D37m_PreservesD37fContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mShellAsset)

	for _, want := range []string{
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
		"var PROJECTION_MODEL  = 'layer-pan-zoom-card-model-position'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m: D37f contract %q must remain", want)
		}
	}
}

// TestExplorer_D37m_PreservesD37jContracts pins D37j authority-
// context API.
func TestExplorer_D37m_PreservesD37jContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mShellAsset)

	for _, want := range []string{
		"viewAuthorityContext:",
		"exitAuthorityContext:",
		"toggleAuthorityContext:",
		"function _computeAuthorityContext(cy, node)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m: D37j contract %q must remain", want)
		}
	}
}

// TestExplorer_D37m_PreservesD37kContracts pins D37k edge-label
// overlay.
func TestExplorer_D37m_PreservesD37kContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mShellAsset)

	for _, want := range []string{
		"_installEdgeLabelOverlay:",
		"_showEdgeLabel:",
		"_hideEdgeLabel:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m: D37k contract %q must remain", want)
		}
	}
}

// TestExplorer_D37m_PreservesD37hToolbar pins the D37h toolbar's
// public surface.
func TestExplorer_D37m_PreservesD37hToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mToolbarAsset)

	for _, want := range []string{
		"wire:",
		"refit:",
		"renderZoomPercent:",
		"syncZoomSelectedEnabled:",
		"syncAuthorityContextButton:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m: D37h toolbar surface %q must remain", want)
		}
	}
}

// ── 14.10 Empty-state copy pins ───────────────────────────────────────

// TestExplorer_D37m_EmptyStateCopyPresent pins the exact locked
// empty-state and caveat strings.
func TestExplorer_D37m_EmptyStateCopyPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	for _, want := range []string{
		"Select a graph node to view its details, authority context, or evidence.",
		"No primary fields available for this node.",
		"No diagnostics for this node in the current projection.",
		"This is the projection root. Use the bottom workbench Overview tab for the full subtree.",
		"Showing context within the loaded Business Service. Cross-BS references are not yet supported.",
		"Showing references within the loaded Business Service. Cross-BS policy applicability requires a future tranche.",
		"Showing references within the loaded Business Service. Cross-BS escalation references require a future tranche.",
		"Runtime evidence overlay is not yet wired for the Authority lens. Per-node operational envelopes will arrive in a future tranche.",
		"Authority projection not yet loaded.",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m: locked copy %q must appear in module", want)
		}
	}
}

// ── CSS contracts ─────────────────────────────────────────────────────

// TestExplorer_D37m_CssIsScoped pins that CSS rules are scoped
// under `.midas-graph-viewport[data-active-renderer="authority"]`.
func TestExplorer_D37m_CssIsScoped(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37mTabsCSSPath)
	cssExec := stripCSSComments(css)

	// Walk all rule openings and assert each selector starts with the
	// renderer-active scope. Mirrors the D35f scoping test in
	// explorer_authority_cytoscape_poc_test.go.
	prefix := `.midas-graph-viewport[data-active-renderer="authority"]`
	for i := 0; i < len(cssExec); i++ {
		if cssExec[i] != '{' {
			continue
		}
		start := strings.LastIndexAny(cssExec[:i], "}")
		if start < 0 {
			start = 0
		} else {
			start++
		}
		selector := strings.TrimSpace(cssExec[start:i])
		if selector == "" {
			continue
		}
		if !strings.HasPrefix(selector, prefix) {
			t.Errorf("D37m: every CSS rule must be scoped under %s — rogue selector %q", prefix, selector)
		}
	}
}

// TestExplorer_D37m_CssZIndexLayering pins the z-index contract:
// strip = 6 (above cards' 5), pane = 8 (above D37k chip's 7).
func TestExplorer_D37m_CssZIndexLayering(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37mTabsCSSPath)

	// Strip rule contains z-index: 6.
	stripIdx := strings.Index(css, ".gmap-canvas-edge-tabs-strip {")
	if stripIdx < 0 {
		t.Fatal("D37m: strip CSS rule missing")
	}
	stripEnd := strings.Index(css[stripIdx:], "}")
	if stripEnd < 0 {
		t.Fatal("D37m: cannot bound strip rule")
	}
	stripBlock := css[stripIdx : stripIdx+stripEnd]
	if !strings.Contains(stripBlock, "z-index: 6") {
		t.Errorf("D37m: strip must declare z-index: 6 — block:\n%s", stripBlock)
	}

	// Pane rule contains z-index: 8.
	paneIdx := strings.Index(css, ".gmap-canvas-edge-tabs-pane {")
	if paneIdx < 0 {
		t.Fatal("D37m: pane CSS rule missing")
	}
	paneEnd := strings.Index(css[paneIdx:], "}")
	if paneEnd < 0 {
		t.Fatal("D37m: cannot bound pane rule")
	}
	paneBlock := css[paneIdx : paneIdx+paneEnd]
	if !strings.Contains(paneBlock, "z-index: 8") {
		t.Errorf("D37m: pane must declare z-index: 8 — block:\n%s", paneBlock)
	}
}

// TestExplorer_D37m_CssWrapperPointerEventsNone pins that the
// wrapper is `pointer-events: none` (children opt in) so the
// existing canvas/card interaction surface is not blocked.
func TestExplorer_D37m_CssWrapperPointerEventsNone(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37mTabsCSSPath)

	wrapperIdx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="authority"] .gmap-canvas-edge-tabs {`)
	if wrapperIdx < 0 {
		t.Fatal("D37m: wrapper rule missing")
	}
	wrapperEnd := strings.Index(css[wrapperIdx:], "}")
	if wrapperEnd < 0 {
		t.Fatal("D37m: cannot bound wrapper rule")
	}
	block := css[wrapperIdx : wrapperIdx+wrapperEnd]
	if !strings.Contains(block, "pointer-events: none") {
		t.Errorf("D37m: wrapper must declare pointer-events: none — block:\n%s", block)
	}
}

// ── Module-level bootstrap pin ────────────────────────────────────────

// TestExplorer_D37m_BootstrapInitOnDomContentLoaded pins Section F.
func TestExplorer_D37m_BootstrapInitOnDomContentLoaded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	for _, want := range []string{
		"document.readyState === 'loading'",
		"DOMContentLoaded",
		"init()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m: Section F bootstrap must include %q", want)
		}
	}
}

// ── D37m-fix-1 — Contract alignment ──────────────────────────────────
//
// The pins below were added after D37m-impl-1 review identified that
// several behaviours had drifted from D37m-design-1. Each test
// targets one of the six contract-alignment fixes and would fail if
// the behaviour regressed.

// ---- Fix 1: Focus Mode behaviour ------------------------------------

// TestExplorer_D37mFix1_FocusModeBodyObserverClosesPaneOnly pins
// that the body-class observer's mutation callback only calls
// closePane() — it must not touch the wrapper's hidden/aria-hidden
// state, must not touch the strip, and must not re-open the pane on
// Focus Mode exit.
func TestExplorer_D37mFix1_FocusModeBodyObserverClosesPaneOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _bindBodyClassObserver()")

	if !strings.Contains(body, "closePane()") {
		t.Errorf("D37m-fix-1: Focus Mode entry must call closePane() — body:\n%s", body)
	}
	for _, banned := range []string{
		"_wrapperEl.setAttribute('aria-hidden'",
		"_wrapperEl.setAttribute('hidden'",
		"_wrapperEl.removeAttribute('hidden'",
		"_stripEl",
		"openTab(",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37m-fix-1: Focus Mode body observer must NOT do %q (strip stays visible; no auto-reopen) — body:\n%s", banned, body)
		}
	}
}

// TestExplorer_D37mFix1_FocusModeNoWrapperHideCss pins that no CSS
// rule hides the canvas-edge tabs wrapper or strip when
// body.gmap-focus-mode is set.
func TestExplorer_D37mFix1_FocusModeNoWrapperHideCss(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37mTabsCSSPath)

	for _, banned := range []string{
		"body.gmap-focus-mode .gmap-canvas-edge-tabs",
		".gmap-focus-mode .gmap-canvas-edge-tabs",
		"body.gmap-focus-mode .gmap-canvas-edge-tabs-strip",
		".gmap-focus-mode .gmap-canvas-edge-tabs-strip",
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D37m-fix-1: CSS must NOT hide canvas-edge tabs under Focus Mode — found %q", banned)
		}
	}
}

// TestExplorer_D37mFix1_WrapperVisibilityIgnoresFocusMode pins that
// _refreshState (the only writer of the wrapper's hidden /
// aria-hidden attributes) drives visibility from
// _isAuthorityLensActive() and does not consult _isFocusMode().
func TestExplorer_D37mFix1_WrapperVisibilityIgnoresFocusMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _refreshState()")

	if !strings.Contains(body, "_isAuthorityLensActive()") {
		t.Errorf("D37m-fix-1: _refreshState must drive visibility from _isAuthorityLensActive() — body:\n%s", body)
	}
	if strings.Contains(body, "_isFocusMode()") {
		t.Errorf("D37m-fix-1: _refreshState must NOT consult _isFocusMode() for visibility — body:\n%s", body)
	}
}

// TestExplorer_D37mFix1_WrapperHiddenAttrToggling pins that the
// wrapper's HTML `hidden` attribute is set/cleared in _refreshState
// based on lens activity. Without the `hidden` attribute we would
// have visible-but-aria-hidden controls outside the Authority lens.
func TestExplorer_D37mFix1_WrapperHiddenAttrToggling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _refreshState()")

	for _, want := range []string{
		"removeAttribute('hidden')",
		"setAttribute('hidden'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m-fix-1: _refreshState must toggle wrapper `hidden` via %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37mFix1_WrapperStartsHidden pins the static wrapper
// markup starts with the HTML `hidden` attribute set (initial lens
// is not Authority).
func TestExplorer_D37mFix1_WrapperStartsHidden(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	idx := strings.Index(body, `data-authority-canvas-edge-tabs`)
	if idx < 0 {
		t.Fatal("D37m-fix-1: wrapper marker missing")
	}
	tagStart := strings.LastIndex(body[:idx], "<div")
	tagEnd := strings.Index(body[idx:], ">")
	if tagStart < 0 || tagEnd < 0 {
		t.Fatal("D37m-fix-1: cannot bound wrapper opening tag")
	}
	tag := body[tagStart : idx+tagEnd+1]
	if !strings.Contains(tag, " hidden") {
		t.Errorf("D37m-fix-1: wrapper opening tag must declare `hidden` initially — tag: %s", tag)
	}
	if !strings.Contains(tag, `aria-hidden="true"`) {
		t.Errorf("D37m-fix-1: wrapper opening tag must declare aria-hidden=\"true\" initially — tag: %s", tag)
	}
}

// ---- Fix 2: Runtime evidence copy -----------------------------------

// TestExplorer_D37mFix1_RuntimeCopyExact pins the exact locked
// runtime-evidence copy AND pins that the earlier
// "Runtime telemetry not yet wired" wording does not appear.
func TestExplorer_D37mFix1_RuntimeCopyExact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	const locked = "Runtime evidence overlay is not yet wired for the Authority lens. Per-node operational envelopes will arrive in a future tranche."
	if !strings.Contains(js, locked) {
		t.Errorf("D37m-fix-1: locked runtime evidence copy missing:\n  want: %q", locked)
	}
	for _, banned := range []string{
		"Runtime telemetry not yet wired",
		"placeholders only",
		"telemetry",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37m-fix-1: module must not contain old/prohibited copy %q (use the locked wording)", banned)
		}
	}
}

// ---- Fix 3: Evidence filtering scope --------------------------------

// TestExplorer_D37mFix1_EvidenceFilterIsSelectedNodeOnly pins that
// _buildEvidenceModel filters by the SELECTED node's kind/id only,
// and does not walk the Authority chain when building its filtered
// diagnostic list.
func TestExplorer_D37mFix1_EvidenceFilterIsSelectedNodeOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _buildEvidenceModel(ctx, projection)")

	// Positive: filter must compare a NodeRef's kind/id against the
	// selected ctx.
	for _, want := range []string{
		"projection.diagnostics",
		"node_refs",
		"ref.kind === ctx.kind",
		"ref.id === ctx.id",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m-fix-1: _buildEvidenceModel must filter by selected node via %q — body:\n%s", want, body)
		}
	}
	// Negative: must NOT walk the Authority chain to choose
	// diagnostics. (`_computeChainForCtx` and `_indexDiagnosticsByNodeRef`
	// are used by the Authority tab, NOT the Evidence list.)
	for _, banned := range []string{
		"_computeChainForCtx",
		"_indexDiagnosticsByNodeRef",
		"chain",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37m-fix-1: _buildEvidenceModel must NOT consult the Authority chain — found %q in body:\n%s", banned, body)
		}
	}
}

// ---- Fix 4: Workbench public surface name ---------------------------

// TestExplorer_D37mFix1_WorkbenchBridgeUsesRealPublicSurface pins
// that the module consumes the existing
// window.MIDASExplorerGraph.authorityWorkbench public surface (the
// real name registered in authority-graph-workbench.js) and does NOT
// reference the fictitious authorityGraphWorkbench surface.
func TestExplorer_D37mFix1_WorkbenchBridgeUsesRealPublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	if !strings.Contains(js, "authorityWorkbench") {
		t.Errorf("D37m-fix-1: module must consume window.MIDASExplorerGraph.authorityWorkbench")
	}
	if strings.Contains(js, "authorityGraphWorkbench") {
		t.Errorf("D37m-fix-1: module must NOT reference the non-existent authorityGraphWorkbench surface")
	}

	// Cross-check: the real surface is what authority-graph-workbench
	// registers — if that ever changes, this test is the canary.
	workbenchAsset := "/explorer/assets/js/graph/authority/authority-graph-workbench.js"
	wb := getExplorerAsset(t, srv, workbenchAsset)
	if !strings.Contains(wb, "window.MIDASExplorerGraph.authorityWorkbench = {") {
		t.Errorf("D37m-fix-1: authority-graph-workbench.js no longer registers window.MIDASExplorerGraph.authorityWorkbench — the canvas-edge bridge target may have shifted")
	}
}

// ---- Fix 5: Authority Context View API ------------------------------

// TestExplorer_D37mFix1_AuthorityContextUsesD37jApi pins that the
// "View context" footer button uses the existing D37j API
// (toggleAuthorityContext / canViewAuthorityContext /
// isAuthorityContextActive) and does NOT introduce a new
// setAuthorityContextFilter API.
func TestExplorer_D37mFix1_AuthorityContextUsesD37jApi(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	// Positive: D37j surface usage.
	for _, want := range []string{
		"toggleAuthorityContext",
		"canViewAuthorityContext",
		"isAuthorityContextActive",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m-fix-1: module must use D37j API %q", want)
		}
	}
	// Negative: invented context-filter API must not appear.
	for _, banned := range []string{
		"setAuthorityContextFilter",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37m-fix-1: module must NOT introduce %q — use existing D37j toggleAuthorityContext", banned)
		}
	}
}

// ---- Fix 6: Workbench launcher mapping ------------------------------

// TestExplorer_D37mFix1_DecisionSurfaceDetailsMapsToFailMode pins
// the most regression-prone single mapping: decision_surface +
// details must map to the fail-mode workbench tab (not overview).
func TestExplorer_D37mFix1_DecisionSurfaceDetailsMapsToFailMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	// Bound to the decision_surface row inside WORKBENCH_MAPPING.
	mapStart := strings.Index(js, "var WORKBENCH_MAPPING = {")
	if mapStart < 0 {
		t.Fatal("D37m-fix-1: WORKBENCH_MAPPING not found")
	}
	mapEnd := strings.Index(js[mapStart:], "};")
	if mapEnd < 0 {
		t.Fatal("D37m-fix-1: cannot bound WORKBENCH_MAPPING")
	}
	mapping := js[mapStart : mapStart+mapEnd]

	rowStart := strings.Index(mapping, "decision_surface:")
	if rowStart < 0 {
		t.Fatal("D37m-fix-1: decision_surface row not found in WORKBENCH_MAPPING")
	}
	rowEnd := strings.Index(mapping[rowStart:], "},")
	if rowEnd < 0 {
		t.Fatal("D37m-fix-1: cannot bound decision_surface row")
	}
	row := mapping[rowStart : rowStart+rowEnd]

	if !strings.Contains(row, "details: 'fail-mode'") {
		t.Errorf("D37m-fix-1: decision_surface + details MUST map to 'fail-mode' (not 'overview'). row:\n%s", row)
	}
	// Spot-check the other two columns to anchor the row.
	if !strings.Contains(row, "authority: 'grants'") {
		t.Errorf("D37m-fix-1: decision_surface + authority MUST map to 'grants'. row:\n%s", row)
	}
	if !strings.Contains(row, "evidence: 'evidence'") {
		t.Errorf("D37m-fix-1: decision_surface + evidence MUST map to 'evidence'. row:\n%s", row)
	}
}

// ---- Fix 7: ARIA hidden vs disabled semantics -----------------------

// TestExplorer_D37mFix1_NoVisibleAriaHiddenStripStyling pins that the
// CSS does NOT keep a visible-but-aria-hidden strip — when the
// wrapper is `aria-hidden=true` the JS also adds `hidden`, and
// browsers' UA stylesheet applies display:none. No explicit CSS rule
// is required; this test pins the negative — there must not be any
// CSS that tries to show `[aria-hidden="true"]` content.
func TestExplorer_D37mFix1_NoVisibleAriaHiddenStripStyling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37mTabsCSSPath)

	// No rule should override the UA `[hidden]` display:none on the wrapper.
	if strings.Contains(css, ".gmap-canvas-edge-tabs[hidden]") {
		t.Errorf("D37m-fix-1: CSS must not override [hidden] on the wrapper — let the UA stylesheet hide it")
	}
	// No rule should keep the strip visible when wrapper is aria-hidden.
	if strings.Contains(css, `.gmap-canvas-edge-tabs[aria-hidden="true"] .gmap-canvas-edge-tabs-strip { display:`) {
		t.Errorf("D37m-fix-1: CSS must not force the strip visible while wrapper is aria-hidden=\"true\"")
	}
}

// TestExplorer_D37mFix1_TabDisabledIsBothAriaAndHtml pins that the
// "disabled" state uses BOTH aria-disabled and the HTML disabled
// attribute (not just one) — assistive tech needs aria-disabled,
// pointer/keyboard activation needs HTML disabled.
func TestExplorer_D37mFix1_TabDisabledIsBothAriaAndHtml(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _refreshState()")

	for _, want := range []string{
		"setAttribute('aria-disabled'",
		"setAttribute('disabled'",
		"removeAttribute('disabled')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m-fix-1: _refreshState must manage tab disabled state via %q — body:\n%s", want, body)
		}
	}
}

// ── D37m-diagnose-1 — Runtime visibility ─────────────────────────────
//
// Runtime visibility failure mode: the strip never appeared in the
// browser even though the asset-text pins passed. Root cause was
// single-signal lens gating that depended on the
// `data-active-renderer` attribute on the viewport being set BEFORE
// `_refreshState` was last called by an observer. Race conditions
// (deep-link to Authority on initial load; lens flips that mirror to
// `body[data-graph-lens]` before the cytoscape mount activates the
// renderer) could leave the wrapper `hidden`.
//
// The pins below would have failed before the D37m-diagnose-1 fix
// and continue to pin the multi-signal robustness contract.

// TestExplorer_D37mDiag1_IsAuthorityLensActiveUsesMultipleSignals
// pins that `_isAuthorityLensActive` checks at least three signals
// — the viewport host's public API, the viewport DOM attribute, and
// `body[data-graph-lens="authority"]` — so a single missed update
// can never leave the wrapper hidden when the Authority lens is the
// user-visible state.
func TestExplorer_D37mDiag1_IsAuthorityLensActiveUsesMultipleSignals(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _isAuthorityLensActive()")

	for _, want := range []string{
		"getActiveRendererId",                      // viewport host public API
		`'authority'`,                              // string we compare against
		`data-active-renderer="authority"`,         // DOM probe selector
		`data-graph-lens`,                          // body lens-level signal
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37m-diag-1: _isAuthorityLensActive must check %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37mDiag1_BodyObserverWatchesGraphLens pins that the
// body observer's attributeFilter includes `data-graph-lens` so a
// lens flip that hasn't yet propagated to the viewport's
// `data-active-renderer` still triggers a state refresh.
func TestExplorer_D37mDiag1_BodyObserverWatchesGraphLens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _bindBodyClassObserver()")

	if !strings.Contains(body, "data-graph-lens") {
		t.Errorf("D37m-diag-1: body observer attributeFilter must include 'data-graph-lens' — body:\n%s", body)
	}
	if !strings.Contains(body, "_refreshState()") {
		t.Errorf("D37m-diag-1: body observer must call _refreshState() on lens flips — body:\n%s", body)
	}
}

// TestExplorer_D37mDiag1_WindowLoadSafetyNet pins that the module
// re-runs `_refreshState` (or re-attempts init) on the `window.load`
// event, so a deep-link / restore-state path that sets the lens
// before our observers were attached still results in a visible
// strip.
func TestExplorer_D37mDiag1_WindowLoadSafetyNet(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)

	for _, want := range []string{
		"window.addEventListener('load'",
		"_refreshState()",
		"_ensureSubscriptions()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37m-diag-1: module must wire a window.load safety net containing %q", want)
		}
	}
}

// TestExplorer_D37mDiag1_NoSelectionDoesNotHideWrapper pins that
// `_refreshState` does NOT couple wrapper visibility to the
// selection state — the strip stays visible (lens-only gating) and
// only the tab buttons are disabled when no eligible node is
// selected. The brief explicitly called this out as the preferred
// runtime contract.
func TestExplorer_D37mDiag1_NoSelectionDoesNotHideWrapper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _refreshState()")

	// Positive: visibility is driven by lens activity, not selection.
	visIdx := strings.Index(body, "setAttribute('aria-hidden'")
	if visIdx < 0 {
		t.Fatalf("D37m-diag-1: _refreshState must set aria-hidden — body:\n%s", body)
	}
	hiddenIdx := strings.Index(body, "setAttribute('hidden'")
	if hiddenIdx < 0 {
		t.Fatalf("D37m-diag-1: _refreshState must set hidden — body:\n%s", body)
	}
	// Negative: hidden / aria-hidden writes must NOT be inside a
	// branch keyed on `canEnable` / `ctx` (the selection signal).
	// We approximate by asserting the hidden/aria-hidden writes appear
	// BEFORE the `canEnable` branch starts.
	canEnableIdx := strings.Index(body, "canEnable")
	if canEnableIdx >= 0 {
		if visIdx > canEnableIdx {
			t.Errorf("D37m-diag-1: aria-hidden write must come BEFORE the canEnable branch (lens-only gating); idx %d vs %d", visIdx, canEnableIdx)
		}
		if hiddenIdx > canEnableIdx {
			t.Errorf("D37m-diag-1: hidden write must come BEFORE the canEnable branch (lens-only gating); idx %d vs %d", hiddenIdx, canEnableIdx)
		}
	}
}

// TestExplorer_D37mDiag1_WrapperIsInsideMidasGraphViewport pins the
// browser-oriented static contract: the wrapper element is a child
// of `.midas-graph-viewport` (so the renderer-identity-scoped CSS
// actually matches) and NOT inside the renderer slot, a template,
// or a different graph container.
func TestExplorer_D37mDiag1_WrapperIsInsideMidasGraphViewport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	wrapperIdx := strings.Index(body, `data-authority-canvas-edge-tabs`)
	if wrapperIdx < 0 {
		t.Fatal("D37m-diag-1: wrapper marker missing")
	}
	// Find the closest preceding `class="midas-graph-viewport"`.
	viewportOpen := strings.LastIndex(body[:wrapperIdx], `class="midas-graph-viewport"`)
	if viewportOpen < 0 {
		t.Fatal("D37m-diag-1: wrapper must be preceded by .midas-graph-viewport opening tag")
	}
	// Find the closing tag (the marker comment / explicit close).
	viewportClose := strings.Index(body[wrapperIdx:], `</div><!-- /.midas-graph-viewport -->`)
	if viewportClose < 0 {
		t.Fatal("D37m-diag-1: wrapper must precede the .midas-graph-viewport closing tag (so it is a child)")
	}
	// Between viewportOpen and wrapperIdx there must be no nested
	// `class="midas-graph-viewport"` (i.e. the wrapper must be DIRECTLY
	// inside the outer viewport, not inside a nested template/copy).
	if strings.Count(body[viewportOpen:wrapperIdx], `class="midas-graph-viewport"`) != 1 {
		t.Errorf("D37m-diag-1: wrapper must be a child of the single .midas-graph-viewport (no nested template)")
	}
	// Wrapper must NOT be inside the renderer slot — the slot is
	// owned by the active renderer and may be re-rendered.
	rendererSlotOpen := strings.LastIndex(body[:wrapperIdx], `class="midas-graph-renderer-slot"`)
	if rendererSlotOpen > viewportOpen {
		rendererSlotClose := strings.Index(body[rendererSlotOpen:], `</div>`)
		if rendererSlotClose >= 0 && (rendererSlotOpen+rendererSlotClose) > wrapperIdx {
			t.Errorf("D37m-diag-1: wrapper must NOT live inside .midas-graph-renderer-slot — that DOM is owned by the active renderer and may be re-mounted")
		}
	}
}

// TestExplorer_D37mDiag1_NoCssRuleHidesWrapperUnderActiveAuthority
// pins that no rule keeps the wrapper hidden when the authority lens
// is active. Specifically: there is no `[data-active-renderer="authority"]
// .gmap-canvas-edge-tabs { display: none }` rule, no `[hidden]`
// override, and no `visibility: hidden` selector that fires inside
// the active scope.
func TestExplorer_D37mDiag1_NoCssRuleHidesWrapperUnderActiveAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37mTabsCSSPath)

	for _, banned := range []string{
		`.gmap-canvas-edge-tabs { display: none`,
		`.gmap-canvas-edge-tabs{display:none`,
		`.gmap-canvas-edge-tabs { visibility: hidden`,
		`.gmap-canvas-edge-tabs{visibility:hidden`,
		`.gmap-canvas-edge-tabs[hidden] { display: block`,
		`.gmap-canvas-edge-tabs-strip { display: none`,
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D37m-diag-1: CSS must not hide the wrapper/strip — found %q", banned)
		}
	}

	// Positive: the wrapper rule must actually position the strip
	// somewhere visible inside the viewport.
	if !strings.Contains(css, `.midas-graph-viewport[data-active-renderer="authority"] .gmap-canvas-edge-tabs {`) {
		t.Error("D37m-diag-1: scoped wrapper positioning rule missing")
	}
}

// TestExplorer_D37mDiag1_LensSignalNamesMatchRepository pins that
// the names referenced by the module are the actual names the
// repository uses at runtime. If somebody renames `'authority'` to
// `'authority-cytoscape'` (or vice versa) in the GraphViewport
// registration, this canary fires.
func TestExplorer_D37mDiag1_LensSignalNamesMatchRepository(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// The canvas-edge module's expectation:
	tabs := getExplorerAsset(t, srv, d37mTabsAsset)
	if !strings.Contains(tabs, `=== 'authority'`) && !strings.Contains(tabs, `data-active-renderer="authority"`) {
		t.Error("D37m-diag-1: canvas-edge module must gate on the literal 'authority' renderer id")
	}

	// The authority-cytoscape-poc's actual registration / activation:
	poc := getExplorerAsset(t, srv, d37mShellAsset)
	if !strings.Contains(poc, `register('authority',`) {
		t.Error("D37m-diag-1: authority-cytoscape-poc.js must register the renderer factory under id 'authority' (this is the runtime signal the canvas-edge module reads)")
	}
	if !strings.Contains(poc, `activateById('authority')`) {
		t.Error("D37m-diag-1: authority-cytoscape-poc.js must activate via activateById('authority')")
	}

	// The body[data-graph-lens] attribute mirror in the inline shell
	// code — fallback signal the canvas-edge module also consults.
	idx := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(idx, `setAttribute('data-graph-lens', state.selectedGraphLens)`) {
		t.Error("D37m-diag-1: index.html must mirror selectedGraphLens onto body[data-graph-lens] (fallback signal for canvas-edge tabs)")
	}
}

// TestExplorer_D37mDiag1_PaneIsOpenedSeparatelyFromStripVisibility
// pins that opening the pane is gated by the tab-click handler /
// `openTab` and is independent of the lens-driven wrapper hiding.
// The pane must NOT be auto-opened by `_refreshState` when the lens
// becomes active.
func TestExplorer_D37mDiag1_PaneIsOpenedSeparatelyFromStripVisibility(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37mTabsAsset)
	body := readD37mFunctionBody(t, js, "function _refreshState()")

	// _refreshState may call render() (refreshing an open pane), but
	// must NOT call openTab() — that would auto-open on lens flip.
	if strings.Contains(body, "openTab(") {
		t.Errorf("D37m-diag-1: _refreshState must NOT call openTab — the pane opens only on user click. body:\n%s", body)
	}
}
