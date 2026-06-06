package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32h_fix1_test.go — D32h-fix-1 pins the lens-aware bottom
// workbench, the Authority Workbench module, and the governance layer
// default-OFF behaviour. Source-string pins are deliberately limited
// to the *structural* contracts that browser verification cannot
// substitute (DOM element ids, CSS selectors that route lens
// visibility, layer-chip default state). The visual / readability
// acceptance criteria for the Authority canvas remain the user's
// browser-verified responsibility, since this environment cannot
// drive a browser.

// TestExplorer_D32hFix1_AuthorityWorkbenchDOMPresent pins that the
// lens-aware bottom workbench element is rendered in the explorer
// shell alongside the existing Drift Analytics tray.
func TestExplorer_D32hFix1_AuthorityWorkbenchDOMPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	if rec.Code != 200 {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	for _, want := range []string{
		`id="gmap-authority-workbench"`,
		`class="gmap-authority-workbench"`,
		`id="gmap-authority-workbench-toggle"`,
		`id="gmap-authority-workbench-body"`,
		`id="gmap-authority-workbench-panel"`,
		`data-authority-workbench-title`,
		`data-authority-workbench-subtitle`,
		`data-authority-workbench-panel`,
		`data-authority-tab="overview"`,
		`data-authority-tab="fail-mode"`,
		`data-authority-tab="escalation"`,
		`data-authority-tab="grants"`,
		`data-authority-tab="evidence"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("D32h-fix-1: explorer shell must render Authority Workbench markup %q", want)
		}
	}
}

// TestExplorer_D32hFix1_AuthorityWorkbenchHiddenByDefault pins the
// `hidden` attribute on the Authority Workbench section so the
// element does not render under the Context lens before the CSS
// loads.
func TestExplorer_D32hFix1_AuthorityWorkbenchHiddenByDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	html := rec.Body.String()
	// Look for the section opening tag including the hidden attribute.
	idx := strings.Index(html, `id="gmap-authority-workbench"`)
	if idx < 0 {
		t.Fatal("D32h-fix-1: Authority Workbench section missing")
	}
	// Scan forward to the end of the opening tag.
	end := strings.Index(html[idx:], ">")
	if end < 0 {
		t.Fatal("D32h-fix-1: malformed Authority Workbench opening tag")
	}
	openTag := html[idx : idx+end+1]
	if !strings.Contains(openTag, "hidden") {
		t.Errorf("D32h-fix-1: Authority Workbench section must default to hidden so the Context lens stays unaffected. Got: %q", openTag)
	}
}

// TestExplorer_D32hFix1_AuthorityWorkbenchScriptLoaded pins that the
// new workbench module is served and loaded after the overlays
// module (the workbench reads from the same _lastAuthorityProjection
// cache the view writes).
func TestExplorer_D32hFix1_AuthorityWorkbenchScriptLoaded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	html := rec.Body.String()
	overlaysIdx := strings.Index(html, `/explorer/assets/js/graph/authority/authority-graph-overlays.js`)
	workbenchIdx := strings.Index(html, `/explorer/assets/js/graph/authority/authority-graph-workbench.js`)
	if overlaysIdx < 0 {
		t.Fatal("D32h-fix-1: overlays script must remain loaded")
	}
	if workbenchIdx < 0 {
		t.Fatal("D32h-fix-1: authority-graph-workbench.js must be served from index.html")
	}
	if workbenchIdx <= overlaysIdx {
		t.Errorf("D32h-fix-1: workbench script (offset %d) must load AFTER overlays script (offset %d)", workbenchIdx, overlaysIdx)
	}
}

// TestExplorer_D32hFix1_AuthorityWorkbenchModuleExports pins the
// public surface of the workbench module.
func TestExplorer_D32hFix1_AuthorityWorkbenchModuleExports(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-workbench.js")
	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityWorkbench",
		"init:                    init,",
		"render:                  render,",
		"setActiveTab:            setActiveTab,",
		"notifySelectionChanged:  notifySelectionChanged,",
		"_TAB_IDS:                TAB_IDS,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-1: authority workbench module must expose %q", want)
		}
	}
}

// TestExplorer_D32hFix1_AuthorityWorkbenchHasFiveTabs pins the
// five-tab contract the user specified: overview / fail-mode /
// escalation / grants / evidence.
func TestExplorer_D32hFix1_AuthorityWorkbenchHasFiveTabs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-workbench.js")
	for _, want := range []string{
		"'overview'", "'fail-mode'", "'escalation'", "'grants'", "'evidence'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-1: authority workbench must declare tab id %s", want)
		}
	}
	// All five tab renderers must exist.
	for _, want := range []string{
		"function _renderOverview()",
		"function _renderFailMode()",
		"function _renderEscalation()",
		"function _renderGrants()",
		"function _renderEvidence()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-1: authority workbench must declare renderer %s", want)
		}
	}
}

// TestExplorer_D32hFix1_AuthorityWorkbenchDoesNotFetch pins that the
// workbench is projection-derived only. It must not perform fetches
// or otherwise reach for the backend; that decouples the new content
// from the backend projection refresh cadence and matches the
// inspector + overlays modules' contract.
func TestExplorer_D32hFix1_AuthorityWorkbenchDoesNotFetch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-workbench.js")
	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"MIDASExplorerAPI",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32h-fix-1: authority workbench must be projection-derived only — must not reference %q", banned)
		}
	}
}

// TestExplorer_D32hFix1_AuthorityWorkbenchEvidenceTabIsHonest pins
// that the Evidence tab does NOT fabricate runtime evidence counters.
// The user explicitly required: "Do not invent runtime evidence
// counters."
func TestExplorer_D32hFix1_AuthorityWorkbenchEvidenceTabIsHonest(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-workbench.js")
	if !strings.Contains(js, "Runtime evidence overlay is not wired yet") {
		t.Error("D32h-fix-1: Evidence tab must declare runtime evidence is not wired yet (do not fabricate counters)")
	}
}

// TestExplorer_D32hFix1_LensAwareCSSRoutesVisibility pins the CSS
// rules that route bottom-workbench visibility from body[data-graph-
// lens]. This is what protects Context Drift Analytics: the lens
// attribute alone toggles which element is visible; neither module
// imperatively shows/hides the other.
func TestExplorer_D32hFix1_LensAwareCSSRoutesVisibility(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, want := range []string{
		`body[data-graph-lens="authority"] #gmap-evidence-tray`,
		`body[data-graph-lens="authority"] #gmap-authority-workbench`,
		`body[data-graph-lens="context"] #gmap-authority-workbench`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32h-fix-1: authority-graph.css must declare lens-routing selector %q", want)
		}
	}
}

// TestExplorer_D32hFix1_AuthorityWorkbenchStyledShell pins the
// presence of styling for the workbench shell + tabs + sections so
// the UI is not a chrome-free div. These are structural CSS classes
// the module's HTML output references.
func TestExplorer_D32hFix1_AuthorityWorkbenchStyledShell(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, want := range []string{
		`.gmap-authority-workbench`,
		`.gmap-authority-workbench-header`,
		`.gmap-authority-workbench-tabs`,
		`.gmap-authority-workbench-tab`,
		`.gmap-authority-workbench-panel`,
		`.authority-workbench-section`,
		`.authority-workbench-stat`,
		`.authority-workbench-empty`,
		`.authority-workbench-diagnostic`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32h-fix-1: authority-graph.css must style %q", want)
		}
	}
}

// TestExplorer_D32hFix1_GovernanceLayersDefaultOff pins the Part C
// product decision: fail-mode and escalation layers default OFF so
// the Authority canvas reads as the authority spine; operators
// inspect fail-mode + escalation detail in the Authority Workbench or
// drawer.
func TestExplorer_D32hFix1_GovernanceLayersDefaultOff(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	for _, want := range []string{
		// LAYER_CHIPS declares defaultOn per chip.
		`{ id: 'authority-spine', label: 'Authority spine', alwaysOn: true, defaultOn: true }`,
		`{ id: 'diagnostics',     label: 'Diagnostics',                       defaultOn: true }`,
		`{ id: 'surface-posture', label: 'Surface posture',                   defaultOn: true }`,
		`{ id: 'escalation',      label: 'Escalation',                        defaultOn: false }`,
		`{ id: 'fail-mode',       label: 'Fail-mode policy',                  defaultOn: false }`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-1: overlays LAYER_CHIPS must declare %q", want)
		}
	}
	// Apply-once helper that puts default-off layers into the off state on
	// first paint, before the operator opens the drawer.
	if !strings.Contains(js, "function _applyLayerDefaultsOnce()") {
		t.Error("D32h-fix-1: overlays must declare _applyLayerDefaultsOnce so default-off layers are applied on first Authority render")
	}
	if !strings.Contains(js, "_applyLayerDefaultsOnce();") {
		t.Error("D32h-fix-1: overlays render() must call _applyLayerDefaultsOnce")
	}
}

// TestExplorer_D32hFix1_LayerChipInputRespectsDefaultOn pins that
// the chip input markup branches on chip.defaultOn — the rendered
// checkbox is `checked` only when defaultOn is not false.
func TestExplorer_D32hFix1_LayerChipInputRespectsDefaultOn(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	if !strings.Contains(js, "var checkedAttr = chip.defaultOn === false ? '' : ' checked';") {
		t.Error("D32h-fix-1: overlays renderLayerChipsInto must branch on chip.defaultOn for the initial checked attribute")
	}
}

// TestExplorer_D32hFix1_ViewCallsWorkbenchAfterOverlays pins that
// the Authority view dispatches into the workbench's render() after
// the overlays render(), inside the post-paint block. The workbench
// reads from window.MIDASExplorerGraph._lastAuthorityProjection (set
// upstream in renderAuthorityGraph), so the call ordering matters.
func TestExplorer_D32hFix1_ViewCallsWorkbenchAfterOverlays(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	overlaysIdx := strings.Index(js, "overlaysModule.render(payload)")
	workbenchIdx := strings.Index(js, "workbenchModule.render()")
	if overlaysIdx < 0 {
		t.Fatal("D32h-fix-1: authority view must continue to call overlaysModule.render(payload)")
	}
	if workbenchIdx < 0 {
		t.Fatal("D32h-fix-1: authority view must call workbenchModule.render()")
	}
	if workbenchIdx <= overlaysIdx {
		t.Errorf("D32h-fix-1: workbench render (offset %d) must come AFTER overlays render (offset %d)", workbenchIdx, overlaysIdx)
	}
}

// TestExplorer_D32hFix1_InspectorHooksFanOutToWorkbench pins the
// selection-change fan-out in index.html. The inspector emits a
// single selection-change pulse; we route it to BOTH the Context
// evidence tray AND the Authority workbench. Each module gates on
// the active lens internally, so this is safe under either lens.
func TestExplorer_D32hFix1_InspectorHooksFanOutToWorkbench(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	html := rec.Body.String()
	if !strings.Contains(html, "window.MIDASExplorerGraph.contextEvidenceTray.notifySelectionChanged()") {
		t.Error("D32h-fix-1: inspector hook must continue to notify the Context evidence tray (Context lens preserved)")
	}
	if !strings.Contains(html, "window.MIDASExplorerGraph.authorityWorkbench.notifySelectionChanged()") {
		t.Error("D32h-fix-1: inspector hook must also notify the Authority workbench so per-selection tabs (Fail Mode / Escalation / Grants) re-render")
	}
}

// TestExplorer_D32hFix1_ContextEvidenceTrayUntouched pins that the
// Context evidence tray DOM, its initial markup, and the Context
// view itself are untouched by D32h-fix-1. The user's non-negotiable:
// "Context Graph is stable product behaviour."
func TestExplorer_D32hFix1_ContextEvidenceTrayUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	html := rec.Body.String()
	// Drift Analytics tray remains.
	for _, want := range []string{
		`id="gmap-evidence-tray"`,
		`data-drift-analytics-title>DRIFT ANALYTICS`,
		`data-drift-analytics-subtitle`,
		`data-drift-analytics-demo-badge`,
		`data-drift-analytics-severity-badge`,
		`id="gmap-evidence-tray-toggle"`,
		`data-tab="drift"`,
		`data-tab="evidence"`,
		`data-tab="activity"`,
		`data-tab="overview"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("D32h-fix-1: Context Drift Analytics tray markup must remain untouched: %q", want)
		}
	}

	// Context view + adapter signatures unchanged.
	ctxView := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	for _, want := range []string{
		"function renderContextGraph(data, ctx)",
		"canvas.dataset.baseWidth = canvasW",
		"svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)",
		"if (typeof ctx.applyZoom === 'function') ctx.applyZoom();",
		"if (typeof ctx.scheduleFitToView === 'function') ctx.scheduleFitToView();",
	} {
		if !strings.Contains(ctxView, want) {
			t.Errorf("D32h-fix-1: Context view contract must remain: %q", want)
		}
	}
}

// TestExplorer_D32hFix1_AuthorityWorkbenchEmptyStatesAreClear pins
// human-readable empty states for the per-selection tabs so an
// unselected canvas doesn't render a blank box.
func TestExplorer_D32hFix1_AuthorityWorkbenchEmptyStatesAreClear(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-workbench.js")
	for _, want := range []string{
		"Select a business service or decision surface to inspect fail-mode posture.",
		"Select an authority profile or decision surface to inspect escalation.",
		"Select a surface, profile, grant, or agent to inspect authorisation.",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-1: workbench must surface helpful empty-state copy: %q", want)
		}
	}
}

// TestExplorer_D32hFix1_DefaultLayersHidesGovernanceClass pins the
// CSS class names that hide governance nodes when the corresponding
// layer is off. Combined with Part C (default-off chips), this
// produces the spine-only default render the user asked for.
func TestExplorer_D32hFix1_DefaultLayersHidesGovernanceClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, want := range []string{
		`.authority-layer-fail-mode-off .gmap-node[data-projection-kind="fail_mode_policy"]`,
		`.authority-layer-fail-mode-off .authority-connector-surface_has_fail_mode_policy`,
		`.authority-layer-fail-mode-off .authority-connector-business_service_has_fail_mode_policy`,
		`.authority-layer-escalation-off .gmap-node[data-projection-kind="escalation_target"]`,
		`.authority-layer-escalation-off .authority-connector-profile_escalates_to`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32h-fix-1: CSS layer-off rules must remain to hide governance branches: %q", want)
		}
	}
}
