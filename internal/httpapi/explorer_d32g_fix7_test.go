package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32g_fix7_test.go — D32g-fix-7 completes the Authority
// Graph dynamic canvas-width contract by adding the
// `canvas.dataset.baseWidth = canvasW` companion to the
// `svg.setAttribute('viewBox', …)` setter that D32g-fix-6 added on
// its own. Without dataset.baseWidth, graph-camera.js applyZoom()
// reads MIN_CANVAS_W (1180) and clamps scene.style.width — the SVG
// inherits a 1180px width, viewBox claims canvasW, and content
// shrinks by 1180/canvasW. D32g-analysis-3 documented this.
//
// These tests pin the completed two-line contract.

// TestExplorer_D32gFix7_AuthorityViewSetsDatasetBaseWidth pins the
// load-bearing companion that D32g-fix-6 missed.
func TestExplorer_D32gFix7_AuthorityViewSetsDatasetBaseWidth(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, `canvas.dataset.baseWidth = canvasW;`) {
		t.Error("D32g-fix-7: renderAuthorityGraph must set canvas.dataset.baseWidth = canvasW — graph-camera.js applyZoom() reads this to size the scene")
	}
}

// TestExplorer_D32gFix7_DatasetBaseWidthPrecedesViewBox pins the
// ordering: dataset.baseWidth must be set BEFORE the viewBox setter
// so applyZoom (which can be triggered by the viewBox change in some
// reflow paths) sees the correct value on its first read.
func TestExplorer_D32gFix7_DatasetBaseWidthPrecedesViewBox(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	baseWidthIdx := strings.Index(js, `canvas.dataset.baseWidth = canvasW;`)
	viewBoxIdx := strings.Index(js, `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)`)
	if baseWidthIdx < 0 {
		t.Fatal("D32g-fix-7: dataset.baseWidth setter missing")
	}
	if viewBoxIdx < 0 {
		t.Fatal("D32g-fix-7: viewBox setter missing (D32g-fix-6 regression)")
	}
	if baseWidthIdx >= viewBoxIdx {
		t.Errorf("D32g-fix-7: canvas.dataset.baseWidth (offset %d) must precede svg.setAttribute('viewBox', …) (offset %d)",
			baseWidthIdx, viewBoxIdx)
	}
}

// TestExplorer_D32gFix7_AuthorityMirrorsContextTwoLineContract pins
// that the Authority view declares BOTH lines of Context's coordinate
// contract — dataset.baseWidth AND viewBox — using the canonical
// canvasW string. Future renderer-convergence work can extract one
// shared helper without merge friction so long as both views keep
// these exact strings.
func TestExplorer_D32gFix7_AuthorityMirrorsContextTwoLineContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")

	// Both lines, byte-for-byte, in BOTH views.
	for _, line := range []string{
		`canvas.dataset.baseWidth = canvasW;`,
		`svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)`,
	} {
		if !strings.Contains(authJS, line) {
			t.Errorf("D32g-fix-7: Authority view must declare contract line %q", line)
		}
		if !strings.Contains(ctxJS, line) {
			t.Errorf("D32g-fix-7: Context view must continue to declare contract line %q (regression guard)", line)
		}
	}
}

// TestExplorer_D32gFix7_D32gFix6ViewBoxPreserved confirms that
// completing D32g-fix-7 did not accidentally remove the D32g-fix-6
// viewBox setter.
func TestExplorer_D32gFix7_D32gFix6ViewBoxPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, `document.getElementById('gmap-svg')`) {
		t.Error("D32g-fix-7: D32g-fix-6's #gmap-svg lookup must remain")
	}
	if !strings.Contains(js, `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)`) {
		t.Error("D32g-fix-7: D32g-fix-6's viewBox setter must remain")
	}
}

// TestExplorer_D32gFix7_BaseWidthIsDynamicNotHardcoded confirms the
// dataset.baseWidth value is the dynamic canvasW, not the static
// 1180 fallback that the camera defaults to.
func TestExplorer_D32gFix7_BaseWidthIsDynamicNotHardcoded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, banned := range []string{
		`canvas.dataset.baseWidth = 1180`,
		`canvas.dataset.baseWidth = "1180"`,
		`canvas.dataset.baseWidth = '1180'`,
		`canvas.dataset.baseWidth = GMAP.MIN_CANVAS_W`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-7: dataset.baseWidth must be dynamic canvasW, not %q", banned)
		}
	}
}

// TestExplorer_D32gFix7_NoBroaderRefactor confirms the fix is minimal.
// No shared renderer helper extracted, no layout rewrite, no annotation
// change. Existing Authority view surface must be intact apart from
// the one-line addition.
func TestExplorer_D32gFix7_NoBroaderRefactor(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// D32h-impl-1 — The kind-bucketed planner (var ROWS / var GOV)
	// was replaced by the chain-aware spec-driven planner. The
	// remaining "no broader refactor" pins guard against renderer
	// convergence helpers that D32g-fix-7 itself did not introduce.
	for _, want := range []string{
		"function renderAuthorityGraph(payload, ctx)",
		"function _anchorsForEdge(edge, srcPos, dstPos)",
		"_computeNodeOverlays",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32g-fix-7 must not remove existing Authority view surface: %q", want)
		}
	}
	for _, banned := range []string{
		"renderer.setCanvasDimensions(", // would indicate Option C extraction
		"renderer.setCanvasViewBox(",    // alternative shared-helper extraction
		"function annotateNode(",        // would indicate Option E annotation refactor
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-7 is a minimal contract completion — must NOT introduce %q (deferred to a future tranche)", banned)
		}
	}
}

// TestExplorer_D32gFix7_ContextCoordinateContractUnchanged pins that
// Context Graph's two-line contract at context-graph-view.js:195-196
// is unchanged. The completion of D32g-fix-7 must not touch Context.
func TestExplorer_D32gFix7_ContextCoordinateContractUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")

	baseWidthIdx := strings.Index(js, `canvas.dataset.baseWidth = canvasW;`)
	viewBoxIdx := strings.Index(js, `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)`)
	if baseWidthIdx < 0 {
		t.Error("D32g-fix-7: Context view's canvas.dataset.baseWidth setter must remain")
	}
	if viewBoxIdx < 0 {
		t.Error("D32g-fix-7: Context view's viewBox setter must remain")
	}
	if baseWidthIdx >= 0 && viewBoxIdx >= 0 && baseWidthIdx >= viewBoxIdx {
		t.Errorf("D32g-fix-7: Context view's dataset.baseWidth (offset %d) must precede its viewBox setter (offset %d)",
			baseWidthIdx, viewBoxIdx)
	}
}
