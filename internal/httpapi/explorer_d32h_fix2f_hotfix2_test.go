package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32h_fix2f_hotfix2_test.go — D32h-fix-2f-hotfix-2 Authority
// Connector Routing and Anchor Correction. Tier-1 source-string pins
// for the new opt-in Authority path generator + the renderer's
// backward-compatible pathFn parameter + the view's pass-through.
//
// Scope (per approved Step 0):
//
//   • Authority spine edges still use ['bottom','top'] anchors
//   • Authority sidecar edges still use ['right','left'] anchors
//   • View honours explicit visibleEdge.anchors
//   • View still uses pickAnchorSides only for the 'pick' sentinel
//   • Connector endpoints read positions[srcKey] / positions[dstKey]
//   • D32g-fix-3 structural guardrail intact
//   • Context connector code unchanged (governance-map/layout.js curvePath)
//   • No raw projection.edges walk reintroduced
//   • Authority layout constants unchanged from D32h-fix-2f
//   • No visual semantics added
//
//   • Authority connectors module is published and loads first
//   • Renderer's addLiveConnector accepts an optional sixth pathFn
//   • Authority view passes the pathFn through to addLiveConnector
//   • Same-lane same-x bottom→top: straight line ('L' command)
//   • Cross-lane bottom→top: midline-Y Bezier (control x's match anchor x's)
//   • Horizontal right→left / left→right: midline-X Bezier (control y's match anchor y's)

// ── Preserved anchor contracts ───────────────────────────────────────

// TestExplorer_D32hFix2fHotfix2_AuthoritySpineEdgesUseBottomTopAnchors
// re-pins the four spine edge kinds emitting explicit ['bottom','top']
// anchors at the layout helper. Connector routing relies on these
// anchors being present BEFORE the path generator runs — without
// explicit anchors the view would fall back to pickAnchorSides which
// is wrong for spine edges.
func TestExplorer_D32hFix2fHotfix2_AuthoritySpineEdgesUseBottomTopAnchors(t *testing.T) {
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
			t.Errorf("D32h-fix-2f-hotfix-2: spine edge must declare explicit ['bottom','top'] anchors — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthoritySidecarEdgesUseRightLeftAnchors
// pins sidecar routing via _anchorsForEdge → pickAnchorSides. The
// layout helper emits 'pick' for sidecars; the view's _anchorsForEdge
// then routes them through pickAnchorSides (which returns ['right',
// 'left'] for the canonical "owner left of sidecar" geometry).
func TestExplorer_D32hFix2fHotfix2_AuthoritySidecarEdgesUseRightLeftAnchors(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	layoutJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// Layout: sidecar edges emit anchors === 'pick' so the view runs
	// pickAnchorSides at emit time.
	for _, want := range []string{
		"pushVisibleEdge(ownKeyV, fmpKeyV, edgeKindV, 'pick');",
		"pushVisibleEdge(etOwnKeyV, etKeyV, 'profile_escalates_to', 'pick');",
	} {
		if !strings.Contains(layoutJS, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: sidecar edge must emit 'pick' for runtime anchor selection — missing %q", want)
		}
	}
	// View: _anchorsForEdge defaults to ['right','left'] when the
	// pickAnchorSides helper is unavailable (defensive fallback).
	// And calls pickAnchorSides when present.
	for _, want := range []string{
		"return pick(srcPos, dstPos);",
		"return ['right', 'left'];",
	} {
		if !strings.Contains(viewJS, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: _anchorsForEdge must route sidecars through pickAnchorSides with right/left fallback — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthorityViewHonoursExplicitVisibleEdgeAnchors
// pins that emitVisibleEdge uses the explicit anchors when they are an
// array of length 2 (the spine path). Without this branch, spine
// anchors would be ignored and every edge would route through 'pick'.
func TestExplorer_D32hFix2fHotfix2_AuthorityViewHonoursExplicitVisibleEdgeAnchors(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"} else if (e.anchors && e.anchors.length === 2) {",
		"anchors = e.anchors;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: view must honour explicit visibleEdge.anchors when length === 2 — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthorityViewStillUsesPickOnlyWhenRequested
// pins that 'pick' is only used for the sentinel — not for spine edges
// — by checking the spine-emission lines themselves do not contain the
// 'pick' literal, while the governance-emission lines do.
func TestExplorer_D32hFix2fHotfix2_AuthorityViewStillUsesPickOnlyWhenRequested(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	// Spine emission lines must NOT contain 'pick' — they should hold
	// explicit ['bottom','top'] anchors.
	for _, banned := range []string{
		"pushVisibleEdge(rKey, sKey, 'business_service_has_surface', 'pick');",
		"pushVisibleEdge(sKey, pKey, 'surface_uses_profile', 'pick');",
		"pushVisibleEdge(pKey, gKey, 'profile_has_grant', 'pick');",
		"pushVisibleEdge(gKey, aKey, 'grant_authorises_agent', 'pick');",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32h-fix-2f-hotfix-2: spine edge must NOT use 'pick' — found %q", banned)
		}
	}
	// Governance edges MUST use 'pick' (verifies the sentinel is still
	// in active use where required).
	for _, want := range []string{
		"edgeKindV, 'pick'",
		"'profile_escalates_to', 'pick'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: governance edge must still use 'pick' sentinel — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthorityConnectorEndpointsUseVisiblePositions
// pins that source/target positions are read from positions[srcKey] /
// positions[dstKey] before the connector is added. Without this, the
// renderer's effectiveGmapPosition lookup could resolve to a stale
// position.
func TestExplorer_D32hFix2fHotfix2_AuthorityConnectorEndpointsUseVisiblePositions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"_state().positions[srcKey] = positions[srcKey];",
		"_state().positions[dstKey] = positions[dstKey];",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: connector endpoints must mirror positions into renderer state — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthorityConnectorGuardrailPreserved
// pins the D32g-fix-3 structural guardrail that short-circuits emit
// when either endpoint is unpositioned. The guard catches the
// "connector approaches but does not terminate" symptom.
func TestExplorer_D32hFix2fHotfix2_AuthorityConnectorGuardrailPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	if !strings.Contains(js, "if (!positions[srcKey] || !positions[dstKey]) return;") {
		t.Error("D32h-fix-2f-hotfix-2: D32g-fix-3 guardrail must remain — connector emit must short-circuit on unpositioned endpoints")
	}
}

// ── Context preservation ─────────────────────────────────────────────

// TestExplorer_D32hFix2fHotfix2_ContextConnectorCodeUnchanged pins
// that governance-map/layout.js's curvePath retains its dominant-axis
// Bezier behaviour. Context relies on this exact geometry; the hotfix
// must NOT modify it.
func TestExplorer_D32hFix2fHotfix2_ContextConnectorCodeUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layout.js")

	for _, want := range []string{
		// Function declaration preserved verbatim.
		"function curvePath(x1, y1, x2, y2)",
		// Dominant-axis selection preserved.
		"if (adx > ady) {",
		// Vertical-dominant control-point math preserved.
		"const ctrl = Math.max(40, ady * 0.45);",
		// Horizontal-dominant control-point math preserved.
		"const ctrl = Math.max(40, adx * 0.45);",
		// Export preserved.
		"window.MIDASGovernanceMap.curvePath     = curvePath;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: Context's governance-map/layout.js curvePath must remain byte-identical — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_NoRawProjectionEdgesWalk re-pins that
// the Authority view never walks projection.edges directly. The layout
// helper's visibleEdges contract is the single source of truth for
// edges to render.
func TestExplorer_D32hFix2fHotfix2_NoRawProjectionEdgesWalk(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	if strings.Contains(js, "for (var ei = 0; ei < projection.edges.length; ei++)") {
		t.Error("D32h-fix-2f-hotfix-2: view must NOT walk projection.edges directly — visibleEdges is the contract")
	}
}

// TestExplorer_D32hFix2fHotfix2_NoLayoutSpacingChange pins that the
// four layout constants from D32h-fix-2f are unchanged. Connector
// routing fixes must not regress layout geometry.
func TestExplorer_D32hFix2fHotfix2_NoLayoutSpacingChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	for _, want := range []string{
		"GMAP.AUTHORITY_TOP_MARGIN = 40",
		"GMAP.AUTHORITY_BOTTOM_MARGIN = 60",
		"window.MIDASGovernanceMap.GMAP.AUTHORITY_VERTICAL_STEP =\n    window.MIDASGovernanceMap.GMAP.NODE_H + 40;",
		"window.MIDASGovernanceMap.GMAP.AUTHORITY_LANE_GAP =\n    window.MIDASGovernanceMap.GMAP.AUTHORITY_CHAIN_GAP;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: D32h-fix-2f layout constant %q must remain unchanged", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_NoVisualSemanticsAdded pins that no
// category colour or styled badge CSS was added in this hotfix.
// Visual semantics belong in D32h-fix-2e.
func TestExplorer_D32hFix2fHotfix2_NoVisualSemanticsAdded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	for _, banned := range []string{
		"border-left: 4px solid var(--primary)",
		"border-left: 4px solid var(--badge-info)",
		"border-left: 4px solid var(--badge-good)",
		"border-left: 4px solid var(--badge-warn)",
		"border-left: 4px solid var(--badge-bad)",
		".authority-badge-shared-by",
		".authority-badge-missing-below",
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D32h-fix-2f-hotfix-2: visual-semantics rule %q is out-of-scope — defer to D32h-fix-2e", banned)
		}
	}
}

// ── New module: Authority connectors path generator ──────────────────

// TestExplorer_D32hFix2fHotfix2_AuthorityConnectorsModulePublished
// pins the new module loads under window.MIDASExplorerGraph.authority
// Connectors with a `path` function.
func TestExplorer_D32hFix2fHotfix2_AuthorityConnectorsModulePublished(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-connectors.js")

	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityConnectors = {",
		"path: path,",
		"function path(p1, p2, srcAnchor, dstAnchor)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: authority connectors module must publish path generator — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthorityConnectorsModuleLoadedBeforeView
// pins the index.html script-tag order so the view resolves the
// authorityConnectors namespace at render time.
func TestExplorer_D32hFix2fHotfix2_AuthorityConnectorsModuleLoadedBeforeView(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	if rec.Code != 200 {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	connectorsIdx := strings.Index(html, "/explorer/assets/js/graph/authority/authority-graph-connectors.js")
	viewIdx := strings.Index(html, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if connectorsIdx < 0 {
		t.Fatal("D32h-fix-2f-hotfix-2: index.html must load authority-graph-connectors.js")
	}
	if viewIdx < 0 {
		t.Fatal("D32h-fix-2f-hotfix-2: index.html must load authority-graph-view.js")
	}
	if connectorsIdx >= viewIdx {
		t.Errorf("D32h-fix-2f-hotfix-2: authority-graph-connectors.js (offset %d) must load BEFORE authority-graph-view.js (offset %d)", connectorsIdx, viewIdx)
	}
}

// TestExplorer_D32hFix2fHotfix2_RendererAddLiveConnectorAcceptsOptionalPathFn
// pins the renderer's new optional sixth parameter. Backward-compat:
// existing five-arg call sites continue to work because pathFn is
// undefined, falling through to the shared _curvePath.
func TestExplorer_D32hFix2fHotfix2_RendererAddLiveConnectorAcceptsOptionalPathFn(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	for _, want := range []string{
		"function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls, pathFn)",
		"function addConnector(p1, p2, cls, srcAnchor, dstAnchor, pathFn)",
		"function _resolvePathD(p1, p2, srcAnchor, dstAnchor, pathFn)",
		"if (typeof pathFn === 'function')",
		"return _curvePath(p1[0], p1[1], p2[0], p2[1]);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: renderer must accept optional pathFn with backward-compatible fallback — missing %q", want)
		}
	}
	// Visible path + hit target MUST use the same pathFn so hover and
	// stroke align.
	hitTargetSig := "function addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId, srcAnchor, dstAnchor, pathFn"
	if !strings.Contains(js, hitTargetSig) {
		t.Errorf("D32h-fix-2f-hotfix-2: addConnectorHitTarget must thread the same pathFn so visible and hit paths align — missing %q", hitTargetSig)
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthorityViewPassesPathFnToAddLiveConnector
// pins the view resolving the Authority path generator and passing it
// as the sixth argument to addLiveConnector.
func TestExplorer_D32hFix2fHotfix2_AuthorityViewPassesPathFnToAddLiveConnector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityConnectors",
		"authorityConnectors.path",
		"renderer.addLiveConnector(srcKey, anchors[0], dstKey, anchors[1], cls, authPathFn);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: Authority view must resolve authorityConnectors.path and pass as the sixth argument — missing %q", want)
		}
	}
}

// ── Path rules (the actual geometry fix) ─────────────────────────────

// TestExplorer_D32hFix2fHotfix2_AuthorityConnectorPathSameLaneIsStraightLine
// pins that for ['bottom','top'] with x1 === x2 the generator emits a
// straight 'L' command, not a Bezier. This is the fix for the
// same-lane S-knot symptom.
func TestExplorer_D32hFix2fHotfix2_AuthorityConnectorPathSameLaneIsStraightLine(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-connectors.js")

	for _, want := range []string{
		"srcAnchor === 'bottom' && dstAnchor === 'top'",
		"if (x1 === x2) {",
		"return 'M ' + x1 + ' ' + y1 + ' L ' + x2 + ' ' + y2;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: same-lane same-x bottom→top must emit straight 'L' line — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthorityConnectorPathCrossLaneUsesMidlineControlPoints
// pins the midline-Y Bezier for cross-lane bottom→top spine edges
// (BS fan-out, shared-node cross-lane). Control points are at
// (x1, midY) and (x2, midY) — anchor x values preserved, midline Y
// used for both control points.
func TestExplorer_D32hFix2fHotfix2_AuthorityConnectorPathCrossLaneUsesMidlineControlPoints(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-connectors.js")

	for _, want := range []string{
		"var midY = (y1 + y2) / 2;",
		"' C ' + x1 + ' ' + midY + ', ' +\n                    x2 + ' ' + midY + ', ' +\n                    x2 + ' ' + y2;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: cross-lane bottom→top must use midline-Y Bezier — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2fHotfix2_AuthorityConnectorPathHorizontalUsesMidlineControlPoints
// pins the midline-X Bezier for horizontal sidecar edges
// (right→left and left→right). Control points are at (midX, y1) and
// (midX, y2) — anchor y values preserved, midline X used for both
// control points.
func TestExplorer_D32hFix2fHotfix2_AuthorityConnectorPathHorizontalUsesMidlineControlPoints(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-connectors.js")

	for _, want := range []string{
		"(srcAnchor === 'right' && dstAnchor === 'left') ||\n        (srcAnchor === 'left'  && dstAnchor === 'right')",
		"var midX = (x1 + x2) / 2;",
		"' C ' + midX + ' ' + y1 + ', ' +\n                    midX + ' ' + y2 + ', ' +\n                    x2 + ' ' + y2;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2f-hotfix-2: horizontal right↔left must use midline-X Bezier — missing %q", want)
		}
	}
}
