package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32g_fix6_test.go — D32g-fix-6 SVG viewBox alignment for
// the Authority Graph. Pins:
//
//   • renderAuthorityGraph looks up #gmap-svg and updates its viewBox
//     to match the dynamically-computed canvasW (the same pattern
//     context-graph-view.js uses).
//   • The viewBox width is NOT hardcoded — it uses `canvasW` so an
//     Authority projection that grows the canvas beyond 1180 px paints
//     SVG connectors in the same coordinate space as HTML node cards.
//   • The update happens AFTER canvasW is calculated.
//   • Context Graph's existing viewBox update is preserved.
//   • No broader layout / connector / renderer refactor leaks into
//     this minimal fix.

// TestExplorer_D32gFix6_AuthorityViewSetsSVGViewBox pins the core
// fix: the Authority view's render function calls
// svg.setAttribute('viewBox', …) with the dynamic canvasW.
func TestExplorer_D32gFix6_AuthorityViewSetsSVGViewBox(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// Must look up the SVG element. Reuses the same element id Context
	// uses (#gmap-svg), so the static markup is unchanged.
	if !strings.Contains(js, `document.getElementById('gmap-svg')`) {
		t.Error("D32g-fix-6: authority-graph-view.js must resolve #gmap-svg in renderAuthorityGraph (mirrors Context Graph pattern)")
	}

	// Must set viewBox to `'0 0 ' + canvasW + ' ' + GMAP.CANVAS_H`.
	// Same string-concat form Context uses at context-graph-view.js:196
	// so the pattern is byte-identical and any future renderer
	// convergence can extract one helper.
	if !strings.Contains(js, `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)`) {
		t.Error("D32g-fix-6: authority-graph-view.js must call svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H) — exact mirror of Context Graph's pattern")
	}
}

// TestExplorer_D32gFix6_ViewBoxIsDynamicNotHardcoded confirms the
// Authority view does NOT hardcode the 1180-wide static viewBox from
// index.html. A regression that replaced canvasW with a fixed
// number would re-introduce the connector-drift defect.
func TestExplorer_D32gFix6_ViewBoxIsDynamicNotHardcoded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// The static viewBox values from index.html's hardcoded
	// declaration must not appear in the Authority view's render
	// path. Specifically, the literal '0 0 1180 720' would reintroduce
	// the pre-fix bug.
	for _, banned := range []string{
		`setAttribute('viewBox', '0 0 1180 720')`,
		`setAttribute('viewBox', "0 0 1180 720")`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-6: authority-graph-view.js must NOT hardcode the static 1180-wide viewBox; banned form: %q", banned)
		}
	}
}

// TestExplorer_D32gFix6_ViewBoxUpdateAfterCanvasWComputed pins the
// ordering: the viewBox update must come AFTER canvasW is computed
// (otherwise the dynamic value is undefined or stale).
func TestExplorer_D32gFix6_ViewBoxUpdateAfterCanvasWComputed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	canvasWIdx := strings.Index(js, "var canvasW = Math.max(")
	if canvasWIdx < 0 {
		t.Fatal("D32g-fix-6: canvasW computation missing from authority-graph-view.js")
	}
	viewBoxIdx := strings.Index(js, "svg.setAttribute('viewBox'")
	if viewBoxIdx < 0 {
		t.Fatal("D32g-fix-6: viewBox setter missing from authority-graph-view.js")
	}
	if viewBoxIdx <= canvasWIdx {
		t.Errorf("D32g-fix-6: viewBox setter (offset %d) must appear AFTER canvasW computation (offset %d)", viewBoxIdx, canvasWIdx)
	}
}

// TestExplorer_D32gFix6_AuthorityMirrorsContextViewBoxPattern pins
// that the Authority pattern is byte-identical to the Context pattern.
// Future renderer convergence (D32g-analysis-2 Option B) can extract
// one shared helper; this pin ensures the two strings stay in sync
// until that refactor happens.
func TestExplorer_D32gFix6_AuthorityMirrorsContextViewBoxPattern(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")

	pattern := `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)`
	if !strings.Contains(ctxJS, pattern) {
		t.Error("D32g-fix-6: Context view must continue to declare the viewBox update pattern (regression guard)")
	}
	if !strings.Contains(authJS, pattern) {
		t.Error("D32g-fix-6: Authority view must mirror Context's viewBox update pattern exactly")
	}
}

// TestExplorer_D32gFix6_NoBroaderRefactor confirms this is a
// minimal fix: no renderer convergence helper added, no layout
// strategy change, no annotation refactor. The Authority view's
// surface should be unchanged apart from the viewBox setter.
func TestExplorer_D32gFix6_NoBroaderRefactor(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// Existing public surface must be intact.
	for _, want := range []string{
		"function renderAuthorityGraph(payload, ctx)",
		"function _anchorsForEdge(edge, srcPos, dstPos)",
		"_computeNodeOverlays(projection)",
		"var ROWS = ['business_service', 'decision_surface', 'authority_profile', 'authority_grant', 'agent']",
		"var GOV  = ['fail_mode_policy', 'escalation_target']",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32g-fix-6 must not remove existing Authority view surface: %q", want)
		}
	}
	// Hot signals of a broader refactor that this tranche explicitly
	// excludes (Options B-E in D32g-analysis-2).
	for _, banned := range []string{
		"renderer.setCanvasViewBox(",       // Option B: helper extraction (deferred)
		"shell.refresh.setViewBox(",        // Option C: shell-level promotion
		"function annotateNode(",           // Option E: annotation-instead-of-node refactor
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-6 is a minimal fix — must NOT introduce %q (deferred to a future tranche)", banned)
		}
	}
}

// TestExplorer_D32gFix6_ContextViewBoxBehaviourUnchanged pins that
// Context Graph's existing viewBox setter at context-graph-view.js:196
// (the D32g-analysis-2 reference implementation) is unchanged. The
// minimal-fix scope of D32g-fix-6 must not touch the Context view.
func TestExplorer_D32gFix6_ContextViewBoxBehaviourUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")

	if !strings.Contains(js, `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)`) {
		t.Error("D32g-fix-6: Context view's existing viewBox setter must be preserved unchanged")
	}
	if !strings.Contains(js, "canvas.dataset.baseWidth = canvasW") {
		t.Error("D32g-fix-6: Context view's canvas.dataset.baseWidth setter must remain (drives camera fit-to-view)")
	}
}
