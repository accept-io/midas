package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32h_fix2c_test.go — D32h-fix-2c pins the layer-state
// contract: authorityOverlays.getLayerState() is the read API, the
// layout helper accepts layerState as a third arg and returns
// visibleNodes + visibleEdges, the view paints from visibleNodes and
// emits from visibleEdges, and canvas bounds derive from visible
// nodes only. CSS hide-rules remain as a defensive fallback.
//
// All tests are source-string pins against the served JS — Tier-1
// coverage. Tier-2 (executed-JS) substrate is a separate tranche.

// TestExplorer_D32hFix2c_OverlaysExposeLayerState pins the public
// getLayerState read API on the overlays module.
func TestExplorer_D32hFix2c_OverlaysExposeLayerState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	for _, want := range []string{
		"function getLayerState()",
		"getLayerState:        getLayerState,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2c: authority overlays must declare %q", want)
		}
	}
}

// TestExplorer_D32hFix2c_GetLayerStateUsesLayerChipDefinitions pins
// that getLayerState iterates the existing LAYER_CHIPS table rather
// than duplicating hardcoded chip ids — preventing definitions from
// drifting across multiple call-sites.
func TestExplorer_D32hFix2c_GetLayerStateUsesLayerChipDefinitions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	getLayerStateIdx := strings.Index(js, "function getLayerState()")
	if getLayerStateIdx < 0 {
		t.Fatal("D32h-fix-2c: getLayerState declaration missing")
	}
	// The function body must reference LAYER_CHIPS (the canonical chip
	// table) and use the existing _layerClassFor helper.
	tail := js[getLayerStateIdx:]
	for _, want := range []string{
		"LAYER_CHIPS",
		"_layerClassFor",
		"chip.defaultOn",
		"chip.alwaysOn",
	} {
		if !strings.Contains(tail, want) {
			t.Errorf("D32h-fix-2c: getLayerState must reuse %q from LAYER_CHIPS — found at offset %d, body missing token", want, getLayerStateIdx)
		}
	}
}

// TestExplorer_D32hFix2c_LayoutHelperAcceptsLayerState pins the
// three-arg signature.
func TestExplorer_D32hFix2c_LayoutHelperAcceptsLayerState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	if !strings.Contains(js, "function computeAuthorityLayout(spec, GMAP, layerState)") {
		t.Error("D32h-fix-2c: computeAuthorityLayout must accept layerState as a third argument")
	}
	// Defensive default for missing/invalid layerState (all-visible).
	if !strings.Contains(js, "function normaliseLayerState(layerState)") {
		t.Error("D32h-fix-2c: layout helper must declare normaliseLayerState — defensive default for missing/invalid layerState")
	}
}

// TestExplorer_D32hFix2c_LayoutHelperReturnsVisibleNodesAndVisibleEdges
// pins that the helper's return object includes visibleNodes and
// visibleEdges as first-class fields.
func TestExplorer_D32hFix2c_LayoutHelperReturnsVisibleNodesAndVisibleEdges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"visibleNodes: visibleNodes",
		"visibleEdges: visibleEdges",
		"function pushVisibleNode(",
		"function pushVisibleEdge(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2c: layout helper must build and return %q", want)
		}
	}
}

// TestExplorer_D32hFix2c_LayoutHelperSkipsFailModeWhenLayerOff pins
// the visibility filter: when fail-mode layer is off, fail_mode_policy
// nodes and FMP edges are excluded from visibleNodes / visibleEdges.
func TestExplorer_D32hFix2c_LayoutHelperSkipsFailModeWhenLayerOff(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"isFailModeVisible",
		"if (failModeOn)",
		// Orphan-band gating uses the same flag (so orphan fail_mode_policy
		// nodes are not surfaced when the layer is off).
		"un.kind === 'fail_mode_policy'  && !failModeOn",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2c: layout helper must gate fail_mode_policy visibility on failModeOn — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2c_LayoutHelperSkipsEscalationWhenLayerOff pins
// the mirror of the fail-mode filter for escalation_target.
func TestExplorer_D32hFix2c_LayoutHelperSkipsEscalationWhenLayerOff(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"isEscalationVisible",
		"if (escalationOn)",
		"un.kind === 'escalation_target' && !escalationOn",
		"profile_escalates_to",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2c: layout helper must gate escalation_target visibility on escalationOn — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2c_LayoutHelperCanvasBoundsUseVisibleNodes pins
// that canvasW / canvasH iterate visibleNodes only — NOT every key in
// positions. This is the load-bearing change that prevents hidden
// governance nodes from inflating canvas width.
func TestExplorer_D32hFix2c_LayoutHelperCanvasBoundsUseVisibleNodes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	// The canvas-bounds loop must iterate visibleNodes (not Object.keys
	// of positions).
	if !strings.Contains(js, "for (var vni = 0; vni < visibleNodes.length; vni++)") {
		t.Error("D32h-fix-2c: canvasW / canvasH loop must iterate visibleNodes, not all positions")
	}
	if !strings.Contains(js, "positions[visibleNodes[vni].refKey]") {
		t.Error("D32h-fix-2c: canvas-bounds loop must look up positions via visibleNodes[*].refKey")
	}
	// The pre-D32h-fix-2c form (Object.keys(positions) iteration for
	// bounds) must not regress.
	if strings.Contains(js, "var ks = Object.keys(positions);") {
		t.Error("D32h-fix-2c: canvas-bounds loop must NOT regress to Object.keys(positions) — hidden governance nodes would inflate canvasW again")
	}
}

// TestExplorer_D32hFix2c_ViewPassesLayerStateToLayout pins the view
// reading layer state from the overlays module and threading it into
// the helper.
func TestExplorer_D32hFix2c_ViewPassesLayerStateToLayout(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"overlaysForLayerState.getLayerState()",
		"layout.computeAuthorityLayout(spec, GMAP, layerState)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2c: view must call layout with layerState — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2c_ViewPaintsVisibleNodes pins the view's
// paint loop iterating layoutResult.visibleNodes (not walking
// spec.chains / spec.governance / spec.unlinked itself).
func TestExplorer_D32hFix2c_ViewPaintsVisibleNodes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"var visibleNodes = layoutResult.visibleNodes",
		"for (var vni = 0; vni < visibleNodes.length; vni++)",
		// D32h-fix-2f — _paintNode gained a sixth `visibleEntry` arg so
		// the view can read structural metadata (missingBelow / sharedBy)
		// off the layout helper's entry. The contract pinned here is that
		// the view paints from the visibleNodes entry (ventry.node + vpos);
		// the entry forwarding is a structural addition, not a regression.
		"_paintNode(ventry.node, vpos, renderer, adapter, overlays, ventry);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2c: view must paint from layoutResult.visibleNodes — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2c_ViewEmitsVisibleEdges pins the view's emit
// loop iterating layoutResult.visibleEdges via a unified
// emitVisibleEdge helper.
func TestExplorer_D32hFix2c_ViewEmitsVisibleEdges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"var visibleEdges = layoutResult.visibleEdges",
		"function emitVisibleEdge(e)",
		"for (var vei = 0; vei < visibleEdges.length; vei++)",
		"emitVisibleEdge(visibleEdges[vei]);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2c: view must emit connectors from layoutResult.visibleEdges — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2c_ViewPreservesD32gFix3Invariants pins that
// the D32g-fix-3 invariants (structural-edge guardrail + pickAnchorSides
// threading via _anchorsForEdge) survive the refactor byte-for-byte.
// The unified emitVisibleEdge helper retains both literal substrings.
func TestExplorer_D32hFix2c_ViewPreservesD32gFix3Invariants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"if (!positions[srcKey] || !positions[dstKey]) return;",
		"_anchorsForEdge({ kind: edgeKind }, positions[srcKey], positions[dstKey])",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2c: emitVisibleEdge must preserve the D32g-fix-3 invariant substring %q", want)
		}
	}
}

// TestExplorer_D32hFix2c_ViewPreservesCoordinateContract pins the
// D32g-fix-7 / D32h-impl-1 dataset.baseWidth + viewBox two-liner
// survives the visibleNodes refactor. The view's coordinate contract
// is upstream of the paint/emit loops.
func TestExplorer_D32hFix2c_ViewPreservesCoordinateContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	baseWidthIdx := strings.Index(js, "canvas.dataset.baseWidth = canvasW;")
	viewBoxIdx := strings.Index(js, "svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)")
	if baseWidthIdx < 0 {
		t.Error("D32h-fix-2c: coordinate-contract dataset.baseWidth setter missing (D32g-fix-7 contract)")
	}
	if viewBoxIdx < 0 {
		t.Error("D32h-fix-2c: coordinate-contract viewBox setter missing (D32g-fix-6/7 contract)")
	}
	if baseWidthIdx >= 0 && viewBoxIdx >= 0 && baseWidthIdx >= viewBoxIdx {
		t.Errorf("D32h-fix-2c: dataset.baseWidth (offset %d) must still precede the viewBox setter (offset %d) — D32g-fix-7 ordering", baseWidthIdx, viewBoxIdx)
	}
}
