package httpapi

import (
	"strings"
	"testing"
)

// explorer_d33a_impl1a_test.go — D33a-impl-1a regression fixes for the
// Cytoscape PoC blank-canvas defect introduced by D33a-impl-1. Tier-1
// source-string pins for the structural changes that restore visible
// nodes / edges.
//
// Defect summary: D33a-impl-1's _safeAreaPadding returned ~332 px in
// the default state (legend expanded). Cytoscape's `fit(eles, padding)`
// applies that padding uniformly to all four sides — on a typical
// mount height of ~500 px this left negative vertical space and the
// graph fit into a degenerate area, rendering nothing visible.
//
// Fix: clamp the padding against the live mount dimensions, defensively
// reject empty-element payloads, and strengthen the post-init fit with
// double rAF + a late fallback tick.

// TestExplorer_D33aImpl1a_CytoscapeFitPaddingIsClamped pins the clamp
// against mount dimensions. With the clamp, padding is capped to
// `min(mountW, mountH) / FIT_PADDING_CAP_DIVISOR` so 2×padding never
// exceeds the smaller mount dimension (graph always gets ≥ 60 % of
// that dimension to render in).
func TestExplorer_D33aImpl1a_CytoscapeFitPaddingIsClamped(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// Clamp constants declared.
		"FIT_PADDING_CAP_DIVISOR",
		"FIT_PADDING_FLOOR",
		"FIT_PADDING_HEADLESS",
		// _safeAreaPadding accepts an optional dims arg so callers
		// can pin against the actual mount, not a stale module ref.
		"function _safeAreaPadding(dims)",
		// Cap computation.
		"Math.min(w, h) / FIT_PADDING_CAP_DIVISOR",
		// Headless fallback used when mount is unmeasured.
		"FIT_PADDING_HEADLESS",
		// _renderPayload passes live mount dims to _safeAreaPadding.
		"_safeAreaPadding({",
		"width:  mount.clientWidth",
		"height: mount.clientHeight",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1a: fit padding clamp missing %q", want)
		}
	}
}

// TestExplorer_D33aImpl1a_RenderPayloadDoesNotLeaveBlankCanvasForEmptyElements
// pins the defensive empty-elements check. If the projection contains
// zero usable nodes, the PoC now renders an explicit overlay rather
// than initialising Cytoscape with no data and leaving a black box.
func TestExplorer_D33aImpl1a_RenderPayloadDoesNotLeaveBlankCanvasForEmptyElements(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"if (!elements.nodes.length) {",
		"_renderUnavailable(mount, 'Authority Graph has no nodes for this service.');",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1a: empty-elements safety net missing %q", want)
		}
	}
}

// TestExplorer_D33aImpl1a_RenderPayloadInitialisesCyAfterMountIsMeasurable
// pins the strengthened post-init fit. Double rAF (next frame + one
// after) + a 120 ms fallback covers the cases where the parent grid
// track sizes only on a subsequent pass.
//
// D33x-fit-zoom-root — `_settleFit` now delegates from the retired
// symmetric `_cy.fit(undefined, _safeAreaPadding())` budget to the
// new asymmetric `_fitToAvailableCanvas(_cy)` helper (which uses
// `cy.viewport({zoom, pan})` to honour per-side overlay insets).
// The rAF/setTimeout settle contract is unchanged; only the fit
// implementation moved.
func TestExplorer_D33aImpl1a_RenderPayloadInitialisesCyAfterMountIsMeasurable(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _settleFit()",
		"_cy.resize();",
		// D33x-fit-zoom-root — settle path delegates to the new
		// asymmetric helper instead of the retired symmetric
		// cy.fit(undefined, _safeAreaPadding()) shape.
		"_fitToAvailableCanvas(_cy)",
		// Outer rAF + nested rAF.
		"window.requestAnimationFrame(function ()",
		"window.requestAnimationFrame(_settleFit)",
		// 120 ms fallback tick.
		"setTimeout(_settleFit, 120)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1a: double-rAF + late fallback fit guard missing %q", want)
		}
	}
}

// TestExplorer_D33aImpl1a_ClearOverlaysDoesNotRemoveCytoscapeMount
// pins that _clearOverlays only targets .cytoscape-poc-unavailable
// overlays. The Cytoscape canvas is appended by the library as a
// child of the mount with library-specific class names; this test
// catches any future change that broadens the cleanup selector and
// would inadvertently remove the canvas.
func TestExplorer_D33aImpl1a_ClearOverlaysDoesNotRemoveCytoscapeMount(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// _clearOverlays must target the unavailable overlay class only.
	if !strings.Contains(js, "mount.querySelectorAll('.cytoscape-poc-unavailable')") {
		t.Error("D33a-impl-1a: _clearOverlays must query only .cytoscape-poc-unavailable")
	}
	// Negative pin: must not broaden to a generic child-removal sweep.
	for _, banned := range []string{
		"mount.innerHTML = ''",
		"while (mount.firstChild)",
		"mount.querySelectorAll('*')",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-impl-1a: _clearOverlays must NOT broaden to a generic child sweep — found %q", banned)
		}
	}
}

// TestExplorer_D33aImpl1a_CytoscapeMountKeepsMeasurableHeight pins
// that the Authority Cytoscape mount has a usable measurable height.
//
// D33a-impl-1a originally pinned `height: 100%` + `min-height: 480px`.
// D37d (authority-cytoscape-mount-visibility-fix) replaced
// `position: relative; width: 100%; height: 100%;` with
// `position: absolute; inset: 0;` to overlay the slot rather than
// stack below the legacy `.governance-map-canvas-scroll` sibling.
// `inset: 0` anchors the mount to the slot's four edges so the
// mount has measurable width AND height without an explicit
// `height: 100%`. `min-height: 480px` remains as the floor when
// the parent grid track collapses. The pre-tranche 720 floor is
// still gone. See docs/design/D37c-authority-cytoscape-blank-state-
// root-cause-assessment.md.
func TestExplorer_D33aImpl1a_CytoscapeMountKeepsMeasurableHeight(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	css = stripCSSComments(css)

	// D37d — mount is anchored via `inset: 0` (not `height: 100%`).
	// Mount still declares a min-height floor.
	for _, want := range []string{
		"inset: 0",
		"min-height: 480px",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D33a-impl-1a/D37d: mount must keep %q so the canvas has measurable height", want)
		}
	}
	if strings.Contains(css, "min-height: 720px") {
		t.Error("D33a-impl-1a: mount must NOT regress to the pre-tranche min-height: 720px")
	}
}

// TestExplorer_D33aImpl1a_FitPaddingFloorProtectsAgainstZero pins the
// FIT_PADDING_FLOOR constant. Without a floor, the clamp could resolve
// to zero on degenerate dimensions and node borders would touch the
// container edge. The floor guarantees a small breathing buffer.
func TestExplorer_D33aImpl1a_FitPaddingFloorProtectsAgainstZero(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"FIT_PADDING_FLOOR       = 24",
		"Math.max(FIT_PADDING_FLOOR,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-impl-1a: fit padding floor missing %q", want)
		}
	}
}
