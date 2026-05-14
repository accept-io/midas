package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32h_fix2f_test.go — D32h-fix-2f Authority Graph Structural
// Layout Implementation. Tier-1 source-string pins for the structural
// contracts the tranche introduces. No execution-tier tests (no goja /
// JSDOM); manual browser verification is the gate for visual no-overlap
// and centroid-fallback readability per the design specification §16.
//
// Scope (per the approved Step 0 sign-off):
//
//   • new constants in governance-map/constants.js
//   • derived y-anchor values for GMAP.AUTHORITY_LAYERS
//   • layout helper consumes AUTHORITY_LANE_GAP for lane stride
//   • layout helper derives y per layer from
//     AUTHORITY_TOP_MARGIN + i * AUTHORITY_VERTICAL_STEP
//   • Business Service centred above lanes
//   • visibleNodes entry carries missingBelow when chain truncates
//   • visibleNodes entry carries sharedBy when node is shared
//   • centroid fallback conditional (1.5× threshold OR same-level
//     collision)
//   • same-level collision detection
//   • sidecar owner-relative placement
//   • sidecar collision offset (NODE_H + 16)
//   • canvasH uses AUTHORITY_BOTTOM_MARGIN
//   • view propagates data-missing-below via setAttribute
//   • view propagates data-shared-by via setAttribute
//   • D32h-fix-2c visibleNodes / visibleEdges contract preserved
//   • coordinate contract preserved
//   • D32g-fix-3 connector invariants preserved
//   • spine anchor pair ['bottom','top'] preserved
//   • layout helper signature preserved
//   • defensive empty-spec handling preserved
//
// Negative pins:
//
//   • no hard-coded y values in layout.js outside the constants module
//   • no category colour CSS additions in this tranche
//   • no new badge CSS classes in this tranche
//   • adapter signature byte-identical
//   • workbench module API byte-identical
//   • drawer module byte-identical
//   • selection path byte-identical

// ── Constants ────────────────────────────────────────────────────────

// TestExplorer_D32hFix2f_ConstantsDeclareAuthorityLaneGap pins the
// spec §5.2 named constant alongside the existing AUTHORITY_CHAIN_GAP.
func TestExplorer_D32hFix2f_ConstantsDeclareAuthorityLaneGap(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	if !strings.Contains(js, "GMAP.AUTHORITY_LANE_GAP") {
		t.Error("D32h-fix-2f: governance-map/constants.js must declare GMAP.AUTHORITY_LANE_GAP (spec §5.2)")
	}
	// AUTHORITY_LANE_GAP must alias AUTHORITY_CHAIN_GAP so the two names
	// remain in lockstep. The literal assignment binds the alias to the
	// chain-gap value declared above it.
	if !strings.Contains(js,
		"window.MIDASGovernanceMap.GMAP.AUTHORITY_LANE_GAP =\n    window.MIDASGovernanceMap.GMAP.AUTHORITY_CHAIN_GAP;") {
		t.Error("D32h-fix-2f: AUTHORITY_LANE_GAP must alias AUTHORITY_CHAIN_GAP — preserving the D32h-impl-1 pin while exposing the spec-named constant")
	}
}

// TestExplorer_D32hFix2f_ConstantsDeclareVerticalStep pins
// AUTHORITY_VERTICAL_STEP = NODE_H + 40 (spec §5.5).
func TestExplorer_D32hFix2f_ConstantsDeclareVerticalStep(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	if !strings.Contains(js, "GMAP.AUTHORITY_VERTICAL_STEP") {
		t.Error("D32h-fix-2f: governance-map/constants.js must declare GMAP.AUTHORITY_VERTICAL_STEP (spec §5.5)")
	}
	if !strings.Contains(js,
		"window.MIDASGovernanceMap.GMAP.AUTHORITY_VERTICAL_STEP =\n    window.MIDASGovernanceMap.GMAP.NODE_H + 40;") {
		t.Error("D32h-fix-2f: AUTHORITY_VERTICAL_STEP must equal NODE_H + 40 per spec §5.5")
	}
}

// TestExplorer_D32hFix2f_ConstantsDeclareTopAndBottomMargin pins the
// two margin constants the spec uses for canvas bounds (spec §5.2).
func TestExplorer_D32hFix2f_ConstantsDeclareTopAndBottomMargin(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	for _, want := range []string{
		"GMAP.AUTHORITY_TOP_MARGIN = 40",
		"GMAP.AUTHORITY_BOTTOM_MARGIN = 60",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: governance-map/constants.js must declare %q (spec §5.2 layout-constants table)", want)
		}
	}
}

// TestExplorer_D32hFix2f_AuthorityLayersDerivedFromConstants pins that
// the AUTHORITY_LAYERS table object is preserved (D32h-impl-1 contract)
// but its y values are computed from AUTHORITY_TOP_MARGIN + n *
// AUTHORITY_VERTICAL_STEP rather than declared as fixed literals.
func TestExplorer_D32hFix2f_AuthorityLayersDerivedFromConstants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	for _, want := range []string{
		// The table object still exists (D32h-impl-1 pin).
		"window.MIDASGovernanceMap.GMAP.AUTHORITY_LAYERS = {",
		// Each row's y is derived, not literal.
		"BUSINESS: { y: top + 0 * step }",
		"SURFACE:  { y: top + 1 * step }",
		"PROFILE:  { y: top + 2 * step }",
		"GRANT:    { y: top + 3 * step }",
		"AGENT:    { y: top + 4 * step }",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: AUTHORITY_LAYERS must be derived from AUTHORITY_TOP_MARGIN + n * AUTHORITY_VERTICAL_STEP — missing %q", want)
		}
	}

	// Negative pin: the pre-tranche fixed-y literals are GONE.
	for _, banned := range []string{
		"BUSINESS: { y:  24 }",
		"SURFACE:  { y: 144 },\n    PROFILE:  { y: 264 }",
		"PROFILE:  { y: 264 }",
		"GRANT:    { y: 384 }",
		"AGENT:    { y: 504 }",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32h-fix-2f: pre-tranche fixed-y literal %q must be replaced by the derived expression", banned)
		}
	}
}

// TestExplorer_D32hFix2f_AuthoritySidecarGapPreserved pins that the
// 36-px sidecar gap (existing D32h-impl-1 contract) survives the
// constants refactor — spec §5.2 mentions a fallback of NODE_W/2 but
// the smaller value is the authoritative Authority sidecar geometry.
func TestExplorer_D32hFix2f_AuthoritySidecarGapPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	if !strings.Contains(js, "GMAP.AUTHORITY_SIDECAR_GAP = 36") {
		t.Error("D32h-fix-2f: GMAP.AUTHORITY_SIDECAR_GAP must remain 36 (the authoritative Authority sidecar geometry)")
	}
}

// ── Layout helper: lane stride and y derivation ──────────────────────

// TestExplorer_D32hFix2f_LayoutHelperConsumesLaneGap pins that the
// layout helper reads AUTHORITY_LANE_GAP (spec name) for lane stride.
// The AUTHORITY_CHAIN_GAP fallback preserves the D32h-impl-1 pin path.
func TestExplorer_D32hFix2f_LayoutHelperConsumesLaneGap(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	if !strings.Contains(js, "GMAP.AUTHORITY_LANE_GAP") {
		t.Error("D32h-fix-2f: layout helper must consume GMAP.AUTHORITY_LANE_GAP for lane stride (spec §5.4)")
	}
	// Stride formula uses the (now renamed-locally) LANE_GAP constant.
	if !strings.Contains(js, "spineStart + ci * (NODE_W + LANE_GAP)") {
		t.Error("D32h-fix-2f: lane assignment must use NODE_W + LANE_GAP stride (spec §5.4)")
	}
	// Negative pin: the pre-tranche stride literal naming is gone.
	if strings.Contains(js, "spineStart + ci * (NODE_W + CHAIN_GAP)") {
		t.Error("D32h-fix-2f: lane stride must reference LANE_GAP, not the local CHAIN_GAP name")
	}
}

// TestExplorer_D32hFix2f_LayoutHelperDerivesYFromConstants pins the
// _layerY helper deriving its return value from AUTHORITY_TOP_MARGIN +
// n * AUTHORITY_VERTICAL_STEP rather than the pre-tranche fixed-rhythm
// fallback (24 + n * (NODE_H + 56)).
func TestExplorer_D32hFix2f_LayoutHelperDerivesYFromConstants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"GMAP.AUTHORITY_TOP_MARGIN",
		"GMAP.AUTHORITY_VERTICAL_STEP",
		"return top + (idx || 0) * step;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: layout helper _layerY must derive y from AUTHORITY_TOP_MARGIN + idx * AUTHORITY_VERTICAL_STEP — missing %q", want)
		}
	}
	// Negative pin: the pre-tranche fallback rhythm is gone.
	if strings.Contains(js, "return 24 + (idx || 0) * (NH + 56);") {
		t.Error("D32h-fix-2f: layout helper _layerY fallback must use the derived expression, not the pre-tranche 24 + n*(NH+56) literal")
	}
}

// TestExplorer_D32hFix2f_LayoutHelperConsumesBottomMargin pins that
// canvasH derivation uses AUTHORITY_BOTTOM_MARGIN instead of the
// hardcoded 24-px tail.
func TestExplorer_D32hFix2f_LayoutHelperConsumesBottomMargin(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	if !strings.Contains(js, "GMAP.AUTHORITY_BOTTOM_MARGIN") {
		t.Error("D32h-fix-2f: layout helper must read GMAP.AUTHORITY_BOTTOM_MARGIN (spec §15)")
	}
	if !strings.Contains(js, "maxY + NODE_H + BOTTOM_MARGIN") {
		t.Error("D32h-fix-2f: canvasH tail must add BOTTOM_MARGIN, not a hard-coded literal")
	}
	// Negative pin: the pre-tranche hardcoded 24 tail is gone.
	if strings.Contains(js, "maxY + NODE_H + 24") {
		t.Error("D32h-fix-2f: canvasH must NOT regress to the hardcoded `+ 24` tail")
	}
}

// ── Business Service centring ────────────────────────────────────────

// TestExplorer_D32hFix2f_BusinessServiceCentredAboveLanes pins the
// spec §5.4 centring rule. The current implementation uses
// (firstX + lastX) / 2; survives verbatim.
func TestExplorer_D32hFix2f_BusinessServiceCentredAboveLanes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"var firstX = chainX[chainOrder[0]];",
		"var lastX  = chainX[chainOrder[chainOrder.length - 1]];",
		"rootX = (firstX + lastX) / 2;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: Business Service must be centred above lanes — missing %q (spec §5.4)", want)
		}
	}
}

// ── visibleNode metadata: missingBelow + sharedBy ────────────────────

// TestExplorer_D32hFix2f_VisibleNodeCarriesMissingBelow pins the
// missingBelow metadata path. The layout helper precomputes a map from
// upstream refKey to truncation marker, and pushVisibleNode forwards it
// onto the entry.
func TestExplorer_D32hFix2f_VisibleNodeCarriesMissingBelow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"var missingBelowByKey = {};",
		"missingBelowByKey[_refKey(mch.surface)] = 'profile';",
		"missingBelowByKey[_refKey(mch.profile)] = 'grant';",
		"missingBelowByKey[_refKey(mch.grant)] = 'agent';",
		"if (missingBelowByKey[k]) entry.missingBelow = missingBelowByKey[k];",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: layout helper must emit missingBelow metadata for truncating chains — missing %q (spec §5.7)", want)
		}
	}
}

// TestExplorer_D32hFix2f_VisibleNodeCarriesSharedBy pins the sharedBy
// metadata path. Population only for ownerChains length > 1; the entry
// shape carries the numeric count.
func TestExplorer_D32hFix2f_VisibleNodeCarriesSharedBy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"var sharedByByKey = {};",
		"function recordSharedBy(refKey, ownerChainIds)",
		"if (!refKey || !ownerChainIds || ownerChainIds.length <= 1) return;",
		"sharedByByKey[refKey] = ownerChainIds.length;",
		"if (sharedByByKey[k])     entry.sharedBy     = sharedByByKey[k];",
		"recordSharedBy(profKey, pOwners);",
		"recordSharedBy(grantKey, gOwners);",
		"recordSharedBy(agentKey, aOwners);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: layout helper must emit sharedBy metadata for shared spine nodes — missing %q (spec §5.8)", want)
		}
	}
}

// ── Centroid fallback: distance threshold + collision ────────────────

// TestExplorer_D32hFix2f_CentroidFallbackDistanceThreshold pins the
// 1.5 * (NODE_W + AUTHORITY_LANE_GAP) threshold rule (spec §5.9 +
// user-approved clarification).
func TestExplorer_D32hFix2f_CentroidFallbackDistanceThreshold(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"var FALLBACK_THRESHOLD = 1.5 * (NODE_W + LANE_GAP);",
		"function nearestOwnerX(",
		"function leftmostOwnerX(",
		"distanceTrips",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: centroid fallback must implement the 1.5× distance threshold — missing %q (spec §5.9)", want)
		}
	}
}

// TestExplorer_D32hFix2f_CentroidFallbackSameLevelCollision pins the
// same-level collision detection added per user clarification. Two
// nodes at the same y collide if |x1 - x2| < NODE_W.
func TestExplorer_D32hFix2f_CentroidFallbackSameLevelCollision(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"function collidesAtLevel(x, y, excludeKey)",
		"if (p.y !== y) continue;",
		"if (Math.abs(p.x - x) < NODE_W) return true;",
		"collisionTrips",
		"if (distanceTrips || collisionTrips)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: centroid fallback must also trip on same-level collision — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2f_ResolveSharedXReturnsFallbackOrCentroid pins
// the resolver shape. If either trip fires, return leftmost owner x;
// otherwise return the centroid.
func TestExplorer_D32hFix2f_ResolveSharedXReturnsFallbackOrCentroid(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"function resolveSharedX(ownerChainIds, fallback, levelY, refKey)",
		"return leftmostOwnerX(ownerChainIds, fallback);",
		"return cx;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: resolveSharedX must return either leftmost-owner fallback or centroid — missing %q", want)
		}
	}
}

// ── Sidecar placement (preserved) ────────────────────────────────────

// TestExplorer_D32hFix2f_SidecarOwnerRelativePlacement pins that the
// sidecar slot is computed at owner.x + NODE_W + SIDECAR_GAP per spec
// §5.10.
func TestExplorer_D32hFix2f_SidecarOwnerRelativePlacement(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"sidecarSlots[surfKey] = { x: laneX + NODE_W + SIDECAR_GAP, y: ySurf };",
		"sidecarSlots[profKey] = { x: px + NODE_W + SIDECAR_GAP, y: yProf };",
		"sidecarSlots[grantKey] = { x: gx + NODE_W + SIDECAR_GAP, y: yGrant };",
		"sidecarSlots[agentKey] = { x: ax + NODE_W + SIDECAR_GAP, y: yAgent };",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: sidecar slot must be owner.x + NODE_W + SIDECAR_GAP — missing %q (spec §5.10)", want)
		}
	}
}

// TestExplorer_D32hFix2f_SidecarCollisionOffset pins the NODE_H + 16
// vertical offset for sidecar slot collisions (spec §5.10).
func TestExplorer_D32hFix2f_SidecarCollisionOffset(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	if !strings.Contains(js, "y += NODE_H + 16;") {
		t.Error("D32h-fix-2f: sidecar slot collision must offset by NODE_H + 16 (spec §5.10)")
	}
}

// ── View: data-attribute propagation ─────────────────────────────────

// TestExplorer_D32hFix2f_ViewPropagatesDataMissingBelow pins the view
// writing the missingBelow metadata as a data-missing-below attribute.
func TestExplorer_D32hFix2f_ViewPropagatesDataMissingBelow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	if !strings.Contains(js, "nodeEl.setAttribute('data-missing-below', visibleEntry.missingBelow);") {
		t.Error("D32h-fix-2f: view must propagate missingBelow as data-missing-below attribute")
	}
}

// TestExplorer_D32hFix2f_ViewPropagatesDataSharedBy pins the view
// writing the sharedBy metadata as a data-shared-by attribute.
func TestExplorer_D32hFix2f_ViewPropagatesDataSharedBy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"typeof visibleEntry.sharedBy === 'number'",
		"nodeEl.setAttribute('data-shared-by', String(visibleEntry.sharedBy));",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: view must propagate sharedBy as data-shared-by attribute — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2f_ViewForwardsVisibleEntryToPaintNode pins the
// paint loop forwarding the entry so _paintNode can read the structural
// metadata.
func TestExplorer_D32hFix2f_ViewForwardsVisibleEntryToPaintNode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"_paintNode(ventry.node, vpos, renderer, adapter, overlays, ventry);",
		"function _paintNode(node, pos, renderer, adapter, overlays, visibleEntry)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: view must forward the visibleNode entry to _paintNode — missing %q", want)
		}
	}
}

// ── Preserved contracts (re-pins) ────────────────────────────────────

// TestExplorer_D32hFix2f_VisibleNodesVisibleEdgesContractPreserved
// re-pins the D32h-fix-2c contract that visibleNodes / visibleEdges are
// first-class outputs.
func TestExplorer_D32hFix2f_VisibleNodesVisibleEdgesContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"visibleNodes: visibleNodes",
		"visibleEdges: visibleEdges",
		"function pushVisibleNode(",
		"function pushVisibleEdge(",
		"for (var vni = 0; vni < visibleNodes.length; vni++)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: D32h-fix-2c visibleNodes / visibleEdges contract must survive — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2f_CoordinateContractPreserved re-pins the
// D32g-fix-6/7 / D32h-impl-1 dataset.baseWidth + viewBox two-liner.
func TestExplorer_D32hFix2f_CoordinateContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	baseWidthIdx := strings.Index(js, "canvas.dataset.baseWidth = canvasW;")
	viewBoxIdx := strings.Index(js, "svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)")
	if baseWidthIdx < 0 {
		t.Error("D32h-fix-2f: dataset.baseWidth setter missing (D32g-fix-7 contract)")
	}
	if viewBoxIdx < 0 {
		t.Error("D32h-fix-2f: viewBox setter missing (D32g-fix-6/7 contract)")
	}
	if baseWidthIdx >= 0 && viewBoxIdx >= 0 && baseWidthIdx >= viewBoxIdx {
		t.Errorf("D32h-fix-2f: dataset.baseWidth (%d) must still precede viewBox (%d)", baseWidthIdx, viewBoxIdx)
	}
}

// TestExplorer_D32hFix2f_D32gFix3InvariantsPreserved re-pins the
// D32g-fix-3 connector invariants: structural-edge guardrail and
// _anchorsForEdge threading.
func TestExplorer_D32hFix2f_D32gFix3InvariantsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"if (!positions[srcKey] || !positions[dstKey]) return;",
		"_anchorsForEdge({ kind: edgeKind }, positions[srcKey], positions[dstKey])",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: D32g-fix-3 invariant %q must survive byte-identical", want)
		}
	}
}

// TestExplorer_D32hFix2f_SpineAnchorPairPreserved re-pins the
// ['bottom', 'top'] anchor pair the helper emits for spine edges.
func TestExplorer_D32hFix2f_SpineAnchorPairPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"pushVisibleEdge(rKey, sKey, 'business_service_has_surface', ['bottom','top']);",
		"pushVisibleEdge(sKey, pKey, 'surface_uses_profile',         ['bottom','top']);",
		"pushVisibleEdge(pKey, gKey, 'profile_has_grant',            ['bottom','top']);",
		"pushVisibleEdge(gKey, aKey, 'grant_authorises_agent',       ['bottom','top']);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: spine edge anchor pair must remain ['bottom','top'] — missing %q (spec §7)", want)
		}
	}
}

// TestExplorer_D32hFix2f_LayoutHelperSignaturePreserved re-pins the
// three-arg signature.
func TestExplorer_D32hFix2f_LayoutHelperSignaturePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	if !strings.Contains(js, "function computeAuthorityLayout(spec, GMAP, layerState)") {
		t.Error("D32h-fix-2f: layout helper signature must remain computeAuthorityLayout(spec, GMAP, layerState)")
	}
}

// TestExplorer_D32hFix2f_EmptySpecHandlingPreserved re-pins the
// defensive return for invalid spec input.
func TestExplorer_D32hFix2f_EmptySpecHandlingPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"if (!spec || typeof spec !== 'object') return empty;",
		"visibleNodes: [],",
		"visibleEdges: [],",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: defensive empty-spec handling must survive — missing %q", want)
		}
	}
}

// ── Negative pins ────────────────────────────────────────────────────

// TestExplorer_D32hFix2f_NoBareChainGapIdentifier (hotfix-1) pins that
// no bare CHAIN_GAP JavaScript identifier survives in the layout
// helper. The D32h-fix-2f rename of the local `CHAIN_GAP` var to
// `LANE_GAP` missed one site in the orphan-row distribution, producing
// a runtime `ReferenceError: CHAIN_GAP is not defined` in the browser
// whenever the spec carried unlinked nodes. Without this pin a regex
// search could re-miss a future rename. The GMAP property names
// AUTHORITY_CHAIN_GAP / AUTHORITY_LANE_GAP remain allowed — the ban
// is on the standalone identifier only.
func TestExplorer_D32hFix2f_NoBareChainGapIdentifier(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	// Strip the AUTHORITY_CHAIN_GAP / AUTHORITY_LANE_GAP property names
	// (both are legal references to the GMAP table). Anything left that
	// reads as a bare `CHAIN_GAP` identifier is the bug we're guarding
	// against.
	stripped := strings.ReplaceAll(js, "AUTHORITY_CHAIN_GAP", "")
	stripped = strings.ReplaceAll(stripped, "AUTHORITY_LANE_GAP", "")
	if strings.Contains(stripped, "CHAIN_GAP") {
		t.Error("D32h-fix-2f-hotfix-1: authority-graph-layout.js must NOT contain a bare CHAIN_GAP identifier — rename to LANE_GAP (browser threw 'CHAIN_GAP is not defined' until this was fixed)")
	}
}

// TestExplorer_D32hFix2f_NoHardcodedYInLayoutHelper pins that the
// layout helper does NOT regress to the pre-tranche fixed-y rhythm
// fallback. The only y values the file should still mention are inside
// quoted comments that document the change; the EXECUTABLE path must
// use derived expressions.
func TestExplorer_D32hFix2f_NoHardcodedYInLayoutHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	// The pre-tranche fallback formula must not survive in any form.
	for _, banned := range []string{
		"return 24 + (idx || 0) * (NH + 56);",
		"24 + (idx || 0) * (NODE_H + 56)",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32h-fix-2f: layout helper must NOT keep the pre-tranche fixed-y fallback formula %q", banned)
		}
	}
}

// TestExplorer_D32hFix2f_NoCategoryColourCSSAdded pins that the
// authority-graph.css stylesheet has no new category colour rules in
// this tranche. Visual semantics belong in D32h-fix-2e per spec §17.
func TestExplorer_D32hFix2f_NoCategoryColourCSSAdded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	// Negative pin: the spec §4.1 left-border colour rule scaffolding
	// must NOT appear yet. A 4px left border keyed to category tokens
	// is the D32h-fix-2e contract.
	for _, banned := range []string{
		"border-left: 4px solid var(--primary)",
		"border-left: 4px solid var(--badge-info)",
		"border-left: 4px solid var(--badge-good)",
		"border-left: 4px solid var(--badge-warn)",
		"border-left: 4px solid var(--badge-bad)",
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D32h-fix-2f: category-colour CSS rule %q is out-of-scope for this tranche — defer to D32h-fix-2e", banned)
		}
	}
}

// TestExplorer_D32hFix2f_NoNewBadgeCSSClasses pins that no new badge
// CSS classes are introduced in this tranche. The structural metadata
// (data-missing-below, data-shared-by) is propagated to the DOM, but
// styled badges are deferred to D32h-fix-2e.
func TestExplorer_D32hFix2f_NoNewBadgeCSSClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	for _, banned := range []string{
		".authority-badge-shared-by",
		".authority-badge-missing-below",
		"[data-shared-by]",
		"[data-missing-below]",
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D32h-fix-2f: badge CSS for %q is out-of-scope for this tranche — defer to D32h-fix-2e", banned)
		}
	}
}

// TestExplorer_D32hFix2f_AdapterSignatureByteIdentical pins that the
// adapter public surface is unchanged. The structural layout work lives
// entirely in the layout helper + view; the adapter spec is already
// sufficient (sharedBy is derived from the existing ownerChains maps).
func TestExplorer_D32hFix2f_AdapterSignatureByteIdentical(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	for _, want := range []string{
		"function mapToCardLayout(projection, view)",
		"profileOwnerChains: profileOwnerChains,",
		"grantOwnerChains:   grantOwnerChains,",
		"agentOwnerChains:   agentOwnerChains,",
		"nodesByRef:         nodesByRef,",
		"chains:             chains,",
		"governance:         { failModePolicies: fmpSpec, escalationTargets: etSpec, unlinked: [] },",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f: adapter public contract must remain byte-identical — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2f_SelectionPathByteIdentical pins that the
// D32h-fix-2b lens-aware selection dispatch is unchanged.
func TestExplorer_D32hFix2f_SelectionPathByteIdentical(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	if rec.Code != 200 {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	// The D32h-fix-2b lens dispatch shape: read selectedGraphLens, dispatch
	// to authorityInspector when lens === 'authority', fall through to
	// contextInspector otherwise.
	for _, want := range []string{
		"function selectGovernanceMapNode(",
		"selectedGraphLens",
		"authorityInspector.selectNode",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("D32h-fix-2f: D32h-fix-2b selection-path contract must remain — missing %q", want)
		}
	}
}
