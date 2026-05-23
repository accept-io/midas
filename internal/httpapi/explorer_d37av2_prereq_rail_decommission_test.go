package httpapi

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// explorer_d37av2_prereq_rail_decommission_test.go —
// D37av2-prereq-authority-rail-decommission-content-impl.
//
// This tranche moves the Authority projection-wide Surface Posture
// clickable list out of the legacy right-rail "Posture & Help" tab
// and into a new Authority Workbench tab (`posture`), and
// decommissions the rest of the Authority right-rail content as
// strategic Authority surfaces:
//
//   • Diagnostics tab (duplicated by Workbench Overview + Evidence)
//   • Summary pills (duplicated by Workbench Overview)
//   • Layer chips (not migrated as live runtime UI in this tranche)
//   • Graph legend (moved to OSS Help; not migrated as live runtime UI)
//   • Help framing (OSS Help is the Help surface)
//   • Posture & Help right-rail tab itself
//
// The Inspector slot remains registered for the next tranche's
// gating work; the physical #gmap-details DOM is not removed.
//
// These tests pin the new strategic surface (Workbench Posture tab)
// and the negative-pin decommission contract on the Authority drawer
// registration. They complement the in-place flips applied to the
// historical D32a / D32g / D33aSpike2gImpl5f tests, which were
// converted from positive to negative pins on the removed mounts.

const (
	d37av2PrereqWorkbenchPath = "explorer/assets/js/graph/authority/authority-graph-workbench.js"
	d37av2PrereqPosturePath   = "explorer/assets/js/graph/authority/authority-surface-posture-panel.js"
	d37av2PrereqViewPath      = "explorer/assets/js/graph/authority/authority-graph-view.js"
)

func d37av2PrereqReadWorkbench(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d37av2PrereqWorkbenchPath)
	if err != nil {
		t.Fatalf("D37av2-prereq: cannot read workbench at %s: %v", d37av2PrereqWorkbenchPath, err)
	}
	return string(b)
}

func d37av2PrereqReadView(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d37av2PrereqViewPath)
	if err != nil {
		t.Fatalf("D37av2-prereq: cannot read view at %s: %v", d37av2PrereqViewPath, err)
	}
	return string(b)
}

// ── 1. Workbench has a Posture tab in the required order ─────────────

// TestExplorer_D37av2Prereq_WorkbenchPostureTabRegistered asserts that
// the Authority Workbench's TAB_IDS array includes the new `posture`
// tab in the required order:
//
//   overview, posture, fail-mode, escalation, grants, evidence
//
// Surface Posture sits second so operators commonly reference the
// projection-wide posture grid after Overview and before drilling
// into per-surface Fail Mode detail. Reordering is out of scope.
func TestExplorer_D37av2Prereq_WorkbenchPostureTabRegistered(t *testing.T) {
	js := d37av2PrereqReadWorkbench(t)

	// Positive pin on the full TAB_IDS array, in order.
	wantArr := "Object.freeze(['overview', 'posture', 'fail-mode', 'escalation', 'grants', 'evidence'])"
	if !strings.Contains(js, wantArr) {
		t.Errorf("D37av2-prereq: workbench TAB_IDS must be %q (Posture sits second per the tranche-mandated order)", wantArr)
	}

	// Sanity — the dispatcher must wire the new tab to its renderer.
	for _, want := range []string{
		"function _renderPosture()",
		"case 'posture':",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37av2-prereq: workbench must declare %q (Posture tab render dispatch)", want)
		}
	}
}

// ── 2. Workbench Posture tab renders Surface Posture content ─────────

// TestExplorer_D37av2Prereq_WorkbenchPostureRendersSurfacePosture
// pins that the Workbench Posture tab stamps a
// [data-authority-surface-posture] container into the workbench panel
// mount and that the post-render dispatch calls the existing
// authoritySurfacePosturePanel.render(...) module. The Surface Posture
// panel's click-to-select / focus behaviour is preserved via the
// existing module (no duplicate rendering logic introduced).
func TestExplorer_D37av2Prereq_WorkbenchPostureRendersSurfacePosture(t *testing.T) {
	js := d37av2PrereqReadWorkbench(t)

	// The Posture tab renderer stamps the panel's mount container.
	if !strings.Contains(js, "data-authority-surface-posture") {
		t.Error("D37av2-prereq: workbench Posture tab must stamp a [data-authority-surface-posture] container so authoritySurfacePosturePanel.render(...) can fill it")
	}
	// The post-render dispatch invokes the existing panel module.
	if !strings.Contains(js, "authoritySurfacePosturePanel") {
		t.Error("D37av2-prereq: workbench Posture tab must dispatch to window.MIDASExplorerGraph.authoritySurfacePosturePanel (existing module owns rendering + click-to-select)")
	}
	if !strings.Contains(js, "_activeTab === 'posture'") {
		t.Error("D37av2-prereq: workbench render() must call the posture panel only when the Posture tab is active")
	}
	// Camel-case / snake-case bridge — the workbench's cached spec
	// uses `surfacePosture` while the panel module reads
	// `surface_posture`; the bridge preserves operator behaviour
	// regardless of which producer set _lastAuthorityProjection.
	if !strings.Contains(js, "surface_posture: spec.surface_posture || spec.surfacePosture") {
		t.Error("D37av2-prereq: workbench Posture dispatch must bridge `surface_posture` / `surfacePosture` so the panel renders correctly under both the Authority view and the Cytoscape PoC projection caches")
	}
}

// ── 3. Surface Posture panel module is preserved + reused ────────────

// TestExplorer_D37av2Prereq_SurfacePosturePanelModulePreserved pins
// that the existing authority-surface-posture-panel.js module remains
// the rendering owner — the Workbench Posture tab reuses it rather
// than duplicating rendering logic. The panel's [data-authority-
// surface-posture] querySelector contract is unchanged; only the
// container's host moved from the right rail to the workbench panel.
func TestExplorer_D37av2Prereq_SurfacePosturePanelModulePreserved(t *testing.T) {
	b, err := os.ReadFile(d37av2PrereqPosturePath)
	if err != nil {
		t.Fatalf("D37av2-prereq: cannot read posture panel at %s: %v", d37av2PrereqPosturePath, err)
	}
	js := string(b)

	for _, want := range []string{
		"window.MIDASExplorerGraph.authoritySurfacePosturePanel",
		"document.querySelector('[data-authority-surface-posture]')",
		"projection.surface_posture",
		// Click-to-select is preserved through the module's existing
		// _selectSurface() handler.
		"_selectSurface",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37av2-prereq: authority-surface-posture-panel.js must preserve %q (panel module is reused by the new Workbench Posture tab without duplicating logic)", want)
		}
	}
}

// ── 4. Workbench Posture tab button present in index.html ────────────

// TestExplorer_D37av2Prereq_WorkbenchPostureTabButtonInIndex pins
// that the new Posture tab button sits between Overview and Fail Mode
// in the workbench tab strip per the required tab order.
func TestExplorer_D37av2Prereq_WorkbenchPostureTabButtonInIndex(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Each tab button has a `data-authority-tab="<id>"` attribute.
	overviewIdx := strings.Index(body, `data-authority-tab="overview"`)
	postureIdx := strings.Index(body, `data-authority-tab="posture"`)
	failModeIdx := strings.Index(body, `data-authority-tab="fail-mode"`)

	if overviewIdx < 0 {
		t.Fatal("D37av2-prereq: workbench tab strip must contain the Overview tab button")
	}
	if postureIdx < 0 {
		t.Fatal("D37av2-prereq: workbench tab strip must contain a Posture tab button (data-authority-tab=\"posture\")")
	}
	if failModeIdx < 0 {
		t.Fatal("D37av2-prereq: workbench tab strip must contain the Fail Mode tab button")
	}
	if !(overviewIdx < postureIdx && postureIdx < failModeIdx) {
		t.Errorf("D37av2-prereq: workbench tab order must be overview → posture → fail-mode (got overview@%d, posture@%d, fail-mode@%d)", overviewIdx, postureIdx, failModeIdx)
	}

	// The button must declare the visible label "Posture" between the
	// closing tag of the opening <button …> and the </button> close.
	// The body slice up to the next </button> includes the label text;
	// `>Posture<` is the closing-attribute → label → open-close-tag
	// sequence we expect.
	postureBlock := body[postureIdx:]
	closeIdx := strings.Index(postureBlock, "</button>")
	if closeIdx < 0 {
		t.Fatal("D37av2-prereq: Posture tab button must be a closed <button> element")
	}
	// Include the </button> in the slice so the `<` of `</button>` is
	// part of what we scan — that way `>Posture<` is matchable.
	if !strings.Contains(postureBlock[:closeIdx+len("</button>")], ">Posture<") {
		t.Error("D37av2-prereq: Posture tab button must carry visible label 'Posture'")
	}
}

// ── 5. Authority drawer registration: only inspector slot remains ────

// TestExplorer_D37av2Prereq_AuthorityDrawerOnlyInspectorRegistered
// pins the post-tranche state of the Authority drawer's lens
// registration: only the inspector slot is registered. The previous
// Diagnostics (evidence) and Posture & Help (config) slot entries
// were dropped; their content moved (Surface Posture → Workbench
// Posture; Diagnostics summary → Workbench Overview; Diagnostics list
// → Workbench Evidence; summary pills → Workbench Overview; layer
// chips + legend retired as live runtime UI; help framing → OSS Help).
func TestExplorer_D37av2Prereq_AuthorityDrawerOnlyInspectorRegistered(t *testing.T) {
	js := d37av2PrereqReadView(t)

	if !strings.Contains(js, "window.MIDASExplorerGraph.drawer.registerLens('authority'") {
		t.Fatal("D37av2-prereq: Authority drawer registration call must remain (the inspector slot still registers)")
	}
	if !strings.Contains(js, "id: 'inspector', label: 'Inspector'") {
		t.Error("D37av2-prereq: Authority drawer must still register the inspector slot with label 'Inspector'")
	}

	// Negative pins — Diagnostics + Posture & Help slot registrations
	// were dropped. The previous render functions were removed.
	for _, banned := range []string{
		"id: 'evidence'",
		"id: 'config'",
		"label: 'Diagnostics'",
		"label: 'Posture & Help'",
		"_authorityRenderDiagnosticsIntoDrawer",
		"_authorityRenderPostureAndHelpIntoDrawer",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37av2-prereq: Authority drawer registration must not contain %q — slot decommissioned by this tranche", banned)
		}
	}
}

// ── 6. Workbench Overview duplicates Diagnostics + Summary content ───

// TestExplorer_D37av2Prereq_WorkbenchOverviewCoversDuplicates pins
// that the Workbench Overview tab continues to render the diagnostic
// summary counts AND the projection summary pills / coverage gaps.
// These are the strategic duplicates that make the right-rail
// Diagnostics tab and Summary pills decommissionable today.
func TestExplorer_D37av2Prereq_WorkbenchOverviewCoversDuplicates(t *testing.T) {
	js := d37av2PrereqReadWorkbench(t)

	// Diagnostic counts (critical / warning / info) are rendered in
	// the Overview tab.
	for _, want := range []string{
		"'Critical diagnostics'",
		"'Warning diagnostics'",
		"'Info diagnostics'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37av2-prereq: Workbench Overview must continue to render diagnostic counts %q (duplicates the decommissioned drawer Diagnostics summary)", want)
		}
	}

	// Summary stats + coverage gaps from projection.summary are
	// rendered in the Overview tab.
	for _, want := range []string{
		"'Active profiles'",
		"'Active grants'",
		"'Active agents'",
		"'Surfaces missing profile'",
		"'Profiles without grants'",
		"'Grants missing agent'",
		"'Surfaces without fail-mode policy'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37av2-prereq: Workbench Overview must continue to render summary stat %q (duplicates the decommissioned drawer Summary pills)", want)
		}
	}
}

// ── 7. Workbench Evidence duplicates projection-wide Diagnostics ─────

// TestExplorer_D37av2Prereq_WorkbenchEvidenceCoversDiagnosticsList
// pins that the Workbench Evidence tab continues to render the
// projection-wide diagnostics list. This is the strategic duplicate
// of the decommissioned right-rail Diagnostics list.
func TestExplorer_D37av2Prereq_WorkbenchEvidenceCoversDiagnosticsList(t *testing.T) {
	js := d37av2PrereqReadWorkbench(t)

	// The Evidence tab reads spec.diagnostics and emits per-row
	// severity / kind / message markup.
	for _, want := range []string{
		"function _renderEvidence()",
		"spec.diagnostics",
		"authority-workbench-diagnostic",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37av2-prereq: Workbench Evidence must continue to render projection-wide diagnostics %q (duplicates the decommissioned drawer Diagnostics list)", want)
		}
	}
}

// ── 8. OSS Help route + toolbar button preserved ─────────────────────

// TestExplorer_D37av2Prereq_OssHelpStillReachable pins that the OSS
// Help module remains operator-accessible after right-rail Help
// framing decommission. OSS Help is now the Help surface; the
// drawer's "Posture & Help" tab name is no longer in service.
func TestExplorer_D37av2Prereq_OssHelpStillReachable(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Toolbar Help button.
	if !strings.Contains(body, `id="gmap-help-button"`) {
		t.Error("D37av2-prereq: toolbar Help button (#gmap-help-button) must remain (OSS Help is the Help surface)")
	}
	// help-context.js + help-links.js are loaded.
	for _, want := range []string{
		`src="/explorer/assets/js/help/help-links.js"`,
		`src="/explorer/assets/js/help/help-context.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37av2-prereq: help wiring must remain (looking for %q)", want)
		}
	}

	// Authority Graph help page exists on disk (the OSS Help server
	// route serves these static MkDocs files). We check the file
	// directly rather than via HTTP because directory-index serving
	// depends on the help mux configuration which is out of scope
	// here.
	if _, err := os.Stat("help/static/graphs/authority-graph/index.html"); err != nil {
		t.Errorf("D37av2-prereq: OSS Help Authority Graph page must exist at help/static/graphs/authority-graph/index.html: %v", err)
	}
	// The Authority Graph help page must cover the legend categories
	// (this tranche added a Legend section to the docs source at
	// userguide/src/graphs/authority-graph.md). We check the served
	// static HTML for at least one legend-category heading.
	pageBytes, err := os.ReadFile("help/static/graphs/authority-graph/index.html")
	if err != nil {
		t.Fatalf("D37av2-prereq: cannot read Authority Graph help page: %v", err)
	}
	pageStr := string(pageBytes)
	// The page should at least describe the seven Authority kinds
	// (this is conceptual legend coverage; the visual swatch legend
	// is rendered from the docs source after the next docs build).
	for _, want := range []string{
		"Business Service",
		"Decision Surface",
		"Authority Profile",
		"Authority Grant",
		"Fail-mode policy",
		"Escalation target",
	} {
		// Match case-insensitively because the docs page uses a mix
		// of sentence case and title case for kind names.
		if !strings.Contains(strings.ToLower(pageStr), strings.ToLower(want)) {
			t.Errorf("D37av2-prereq: Authority Graph help page must describe Authority node kind %q (legend coverage)", want)
		}
	}
}

// ── 9. Right rail no longer authoritative Surface Posture home ───────

// TestExplorer_D37av2Prereq_RightRailNotAuthoritativePostureHome pins
// that the Surface Posture clickable list no longer has a duplicate
// authoritative live surface in the right rail. The Workbench Posture
// tab is the sole authoritative home post-tranche. Negative pin: the
// Authority drawer view must not emit a [data-authority-surface-
// posture] mount.
func TestExplorer_D37av2Prereq_RightRailNotAuthoritativePostureHome(t *testing.T) {
	js := d37av2PrereqReadView(t)

	if strings.Contains(js, "data-authority-surface-posture") {
		t.Error("D37av2-prereq: authority-graph-view.js must not stamp [data-authority-surface-posture] — that container belongs to the Workbench Posture tab now")
	}
}
