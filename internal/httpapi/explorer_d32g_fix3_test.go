package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32g_fix3_test.go — D32g-fix-3 Authority Graph connector
// geometry contract tests. Pins:
//
//   • curvePath is direction-aware (control points chosen along the
//     dominant axis) — fixes the operator-observed "connector stops
//     short / routes toward the wrong part" symptom for Authority's
//     horizontal governance edges.
//   • Endpoints (M and final point) of every curve are exactly the
//     supplied (x1,y1) and (x2,y2) — control-point calculation must
//     not alter where the path starts or terminates.
//   • lensAgnosticConnectorPath (fallback) mirrors the same fix so
//     test-isolation loads produce the same geometry.
//   • pickAnchorSides helper exists, faces the source anchor toward
//     the target's actual position, and is wired into the Authority
//     view's governance-edge anchor selection.
//   • Authority view drops edges whose endpoints are missing from the
//     position map (structural-edge guardrail).
//   • Connector paths still carry fill: none (D32g-fix-1 preserved).
//   • No backend / seed / static-fallback regressions.

// TestExplorer_D32gFix3_CurvePathIsDirectionAware pins the dominant-
// axis branch in curvePath. Horizontal-dominant input (|dx| > |dy|)
// must produce control points offset along X; vertical-dominant
// input must keep the pre-D32g behaviour (control points offset along
// Y) so Context Graph's strict top-down flow is preserved.
func TestExplorer_D32gFix3_CurvePathIsDirectionAware(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layout.js")

	fnIdx := strings.Index(js, "function curvePath(x1, y1, x2, y2) {")
	if fnIdx < 0 {
		t.Fatal("D32g-fix-3: curvePath function must exist in layout.js")
	}
	endRel := strings.Index(js[fnIdx:], "\n  }\n")
	if endRel <= 0 {
		t.Fatal("D32g-fix-3: cannot bound curvePath body")
	}
	body := js[fnIdx : fnIdx+endRel]

	// Direction split must exist.
	if !strings.Contains(body, "const dx = x2 - x1") {
		t.Error("D32g-fix-3: curvePath must compute dx = x2 - x1")
	}
	if !strings.Contains(body, "const dy = y2 - y1") {
		t.Error("D32g-fix-3: curvePath must compute dy = y2 - y1")
	}
	if !strings.Contains(body, "const adx = Math.abs(dx)") {
		t.Error("D32g-fix-3: curvePath must derive adx for dominant-axis comparison")
	}
	if !strings.Contains(body, "const ady = Math.abs(dy)") {
		t.Error("D32g-fix-3: curvePath must derive ady for dominant-axis comparison")
	}
	if !strings.Contains(body, "if (adx > ady)") {
		t.Error("D32g-fix-3: curvePath must branch on (adx > ady) — horizontal-dominant vs vertical-dominant")
	}
	// Horizontal-dominant control points offset along X.
	if !strings.Contains(body, "(x1 + sgn * ctrl) + ' ' + y1") {
		t.Error("D32g-fix-3: horizontal-dominant branch must place first control point at (x1 + sgn*ctrl, y1) — same y as source anchor")
	}
	if !strings.Contains(body, "(x2 - sgn * ctrl) + ' ' + y2") {
		t.Error("D32g-fix-3: horizontal-dominant branch must place second control point at (x2 - sgn*ctrl, y2) — same y as target anchor")
	}
	// Vertical-dominant control points still offset along Y.
	if !strings.Contains(body, "x1 + ' ' + (y1 + sgn * ctrl)") {
		t.Error("D32g-fix-3: vertical-dominant branch must place first control point at (x1, y1 + sgn*ctrl) — Context Graph compatibility")
	}
	if !strings.Contains(body, "x2 + ' ' + (y2 - sgn * ctrl)") {
		t.Error("D32g-fix-3: vertical-dominant branch must place second control point at (x2, y2 - sgn*ctrl) — Context Graph compatibility")
	}
}

// TestExplorer_D32gFix3_CurvePathEndpointsExact pins that the curve
// starts at exactly (x1, y1) and ends at exactly (x2, y2) regardless
// of which dominant-axis branch is taken. Control-point math must
// never alter the endpoints.
func TestExplorer_D32gFix3_CurvePathEndpointsExact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layout.js")

	fnIdx := strings.Index(js, "function curvePath(x1, y1, x2, y2) {")
	if fnIdx < 0 {
		t.Fatal("D32g-fix-3: curvePath function missing")
	}
	endRel := strings.Index(js[fnIdx:], "\n  }\n")
	body := js[fnIdx : fnIdx+endRel]

	// Both branches MUST emit "M ' + x1 + ' ' + y1" (Bézier start)
	// and "... x2 + ' ' + y2" (final endpoint). The exact start +
	// end of the path strings appear identically in both branches.
	startCount := strings.Count(body, "'M ' + x1 + ' ' + y1")
	if startCount < 2 {
		t.Errorf("D32g-fix-3: curvePath must start at (x1,y1) in BOTH branches; got %d occurrences", startCount)
	}
	endCount := strings.Count(body, "x2 + ' ' + y2")
	if endCount < 2 {
		t.Errorf("D32g-fix-3: curvePath must end at (x2,y2) in BOTH branches; got %d occurrences", endCount)
	}
}

// TestExplorer_D32gFix3_CurvePathSignedControlOffsets pins that
// control-point offsets carry the SIGN of dx (horizontal) or dy
// (vertical). Signed offsets keep the curve bowing outward correctly
// for both forward and reverse directions; an unsigned offset would
// create a loop on reverse edges.
func TestExplorer_D32gFix3_CurvePathSignedControlOffsets(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layout.js")

	if !strings.Contains(js, "const sgn = Math.sign(dx) || 1") {
		t.Error("D32g-fix-3: horizontal-dominant branch must use signed sgn = Math.sign(dx)")
	}
	if !strings.Contains(js, "const sgn = Math.sign(dy) || 1") {
		t.Error("D32g-fix-3: vertical-dominant branch must use signed sgn = Math.sign(dy)")
	}
}

// TestExplorer_D32gFix3_PickAnchorSidesHelperExposed pins that the
// new pickAnchorSides pure helper is exported from layout.js so the
// Authority view (and future lenses with mixed-direction edges) can
// resolve anchor sides from real node positions.
func TestExplorer_D32gFix3_PickAnchorSidesHelperExposed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layout.js")

	if !strings.Contains(js, "function pickAnchorSides(srcPos, dstPos)") {
		t.Error("D32g-fix-3: layout.js must export the pickAnchorSides(srcPos, dstPos) helper")
	}
	if !strings.Contains(js, "window.MIDASGovernanceMap.pickAnchorSides = pickAnchorSides") {
		t.Error("D32g-fix-3: layout.js must publish pickAnchorSides on MIDASGovernanceMap")
	}
	// The helper picks the dominant axis from |Δx| vs |Δy| between
	// node centres — same heuristic the curvePath formula uses, so
	// anchor side and curve direction agree.
	if !strings.Contains(js, "Math.abs(dx) >= Math.abs(dy)") {
		t.Error("D32g-fix-3: pickAnchorSides must use the dominant-axis heuristic (|dx| vs |dy|)")
	}
	// Both directions of every axis must be reachable so an anchor
	// can face EITHER side of the source node.
	for _, want := range []string{
		`['right', 'left']`,
		`['left', 'right']`,
		`['bottom', 'top']`,
		`['top', 'bottom']`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32g-fix-3: pickAnchorSides must be able to return %q", want)
		}
	}
}

// TestExplorer_D32gFix3_AuthorityViewUsesPickAnchorSides pins that
// the Authority view threads pickAnchorSides into governance-edge
// anchor selection (so source faces target wherever the target
// sits) and continues to use the fixed ['bottom', 'top'] pair for
// spine edges.
func TestExplorer_D32gFix3_AuthorityViewUsesPickAnchorSides(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// _anchorsForEdge now accepts srcPos + dstPos.
	if !strings.Contains(js, "function _anchorsForEdge(edge, srcPos, dstPos)") {
		t.Error("D32g-fix-3: _anchorsForEdge must accept (edge, srcPos, dstPos) — needed by pickAnchorSides")
	}
	// Governance branch delegates to pickAnchorSides.
	if !strings.Contains(js, "pick(srcPos, dstPos)") {
		t.Error("D32g-fix-3: governance-edge branch must delegate to pickAnchorSides(srcPos, dstPos)")
	}
	// Spine edges keep the fixed pair.
	if !strings.Contains(js, "if (!govEdges[edge.kind]) return ['bottom', 'top'];") {
		t.Error("D32g-fix-3: spine edges must keep the fixed ['bottom', 'top'] anchor pair")
	}
	// Edge paint loop passes positions into _anchorsForEdge.
	if !strings.Contains(js, "_anchorsForEdge(edge, positions[srcKey], positions[dstKey])") {
		t.Error("D32g-fix-3: edge paint loop must thread the resolved positions into _anchorsForEdge")
	}
}

// TestExplorer_D32gFix3_GovernanceEdgeKindsList pins the three
// edge-kind names the Authority view treats as governance crossings
// (right-column endpoints). A future tranche that introduces a new
// governance edge kind must update this list together with the
// projection.
func TestExplorer_D32gFix3_GovernanceEdgeKindsList(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, kind := range []string{
		"surface_has_fail_mode_policy:          true",
		"business_service_has_fail_mode_policy: true",
		"profile_escalates_to:                  true",
	} {
		if !strings.Contains(js, kind) {
			t.Errorf("D32g-fix-3: governance-edge catalogue must include %s", kind)
		}
	}
}

// TestExplorer_D32gFix3_StructuralEdgeGuardrail pins that the view's
// edge paint loop drops edges whose endpoints are missing from the
// position map. Without this guardrail a node that was filtered or
// removed mid-render leaves a "half-connected" path that visually
// ends in empty space.
func TestExplorer_D32gFix3_StructuralEdgeGuardrail(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, "if (!positions[srcKey] || !positions[dstKey]) continue;") {
		t.Error("D32g-fix-3: edge paint loop must skip edges with missing endpoints (structural guardrail)")
	}
}

// TestExplorer_D32gFix3_FallbackPathMatchesPrimary pins that the
// lensAgnosticConnectorPath fallback in graph-renderer.js mirrors
// the same direction-aware fix. Without this, asset loading races
// could leave the renderer using a different curve formula from
// the layout helper.
func TestExplorer_D32gFix3_FallbackPathMatchesPrimary(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	fnIdx := strings.Index(js, "function lensAgnosticConnectorPath(srcAnchor, dstAnchor) {")
	if fnIdx < 0 {
		t.Fatal("D32g-fix-3: lensAgnosticConnectorPath must exist")
	}
	endRel := strings.Index(js[fnIdx:], "\n  }\n")
	body := js[fnIdx : fnIdx+endRel]

	for _, want := range []string{
		"var dx = x2 - x1",
		"var dy = y2 - y1",
		"var adx = Math.abs(dx)",
		"var ady = Math.abs(dy)",
		"if (adx > ady)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32g-fix-3: fallback path must contain %q (direction-aware mirror of layout.js)", want)
		}
	}
	// Endpoints stay literal (x1, y1) and (x2, y2) in both branches.
	for _, want := range []string{
		"'M ' + x1 + ' ' + y1",
		"x2 + ' ' + y2",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32g-fix-3: fallback path endpoints must be %q", want)
		}
	}
}

// TestExplorer_D32gFix3_ConnectorFillNoneStillPresent confirms the
// D32g-fix-1 fill:none correction survives this geometry pass.
// Without it, the bezier interior would once again paint as a
// black region and visually masquerade as an oversized connector.
func TestExplorer_D32gFix3_ConnectorFillNoneStillPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, kind := range []string{
		"business_service_has_surface",
		"surface_uses_profile",
		"profile_has_grant",
		"grant_authorises_agent",
		"surface_has_fail_mode_policy",
		"business_service_has_fail_mode_policy",
		"profile_escalates_to",
	} {
		ruleIdx := strings.Index(css, ".authority-connector-"+kind)
		if ruleIdx < 0 {
			t.Errorf("D32g-fix-3: connector rule .authority-connector-%s missing", kind)
			continue
		}
		end := ruleIdx + 280
		if end > len(css) {
			end = len(css)
		}
		seg := css[ruleIdx:end]
		if !strings.Contains(seg, "fill: none") {
			t.Errorf("D32g-fix-3: .authority-connector-%s must still declare fill: none", kind)
		}
	}
}

// TestExplorer_D32gFix3_HitTargetRemainsInvisible pins that the
// invisible hit-target stroke (12px wide, transparent) is unchanged.
// A regression that gave it a visible stroke would re-introduce the
// fat "extra connector" artefact behind every real connector.
//
// The hit-target rule is the standalone .gmap-connector-hit-target
// declaration (not the panning/lassoing selector group that targets
// the same class). We anchor on a unique substring inside the rule.
func TestExplorer_D32gFix3_HitTargetRemainsInvisible(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Anchor on the standalone rule by requiring the "stroke-width: 12"
	// declaration that is unique to the hit-target rule.
	hitIdx := strings.Index(css, "stroke-width: 12")
	if hitIdx < 0 {
		t.Fatal("D32g-fix-3: hit-target rule's 12px stroke-width declaration must exist")
	}
	// Walk back to find the enclosing rule header so we can scope the
	// fill/stroke check to that rule only.
	preamble := css[:hitIdx]
	openIdx := strings.LastIndex(preamble, ".gmap-connector-hit-target {")
	if openIdx < 0 {
		t.Fatal("D32g-fix-3: standalone .gmap-connector-hit-target { rule must exist")
	}
	closeIdx := strings.Index(css[openIdx:], "}")
	if closeIdx < 0 {
		t.Fatal("D32g-fix-3: hit-target rule has no closing brace")
	}
	rule := css[openIdx : openIdx+closeIdx]
	if !strings.Contains(rule, "stroke: transparent") {
		t.Error("D32g-fix-3: hit-target stroke must remain transparent")
	}
	if !strings.Contains(rule, "fill: none") {
		t.Error("D32g-fix-3: hit-target fill must remain none")
	}
}

// TestExplorer_D32gFix3_ContextGraphCurveUnchangedForVerticalFlow
// pins backward compatibility for Context Graph. The Context Graph
// uses only ['bottom', 'top'] anchor pairs (vertical flow), which
// lands in the vertical-dominant branch. The vertical-dominant
// branch's formula is mathematically identical to the pre-D32g
// formula for forward-direction edges (sgn(dy) = +1) where the
// target is below the source — the canonical Context Graph case.
func TestExplorer_D32gFix3_ContextGraphCurveUnchangedForVerticalFlow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layout.js")

	// Pin the exact form of the vertical-dominant control-point math:
	//   C  x1  (y1 + sgn * ctrl), x2 (y2 - sgn * ctrl), x2 y2
	// For Context Graph's typical top-down edges (y2 > y1, so sgn = +1),
	// this produces the SAME path string the pre-fix formula did.
	if !strings.Contains(js, "x1 + ' ' + (y1 + sgn * ctrl)") {
		t.Error("D32g-fix-3: vertical-dominant control point form must remain (x1, y1 + sgn*ctrl) so Context Graph's downward curves are unchanged")
	}
	if !strings.Contains(js, "x2 + ' ' + (y2 - sgn * ctrl)") {
		t.Error("D32g-fix-3: vertical-dominant control point form must remain (x2, y2 - sgn*ctrl)")
	}
	// 40px minimum + 0.45 scale factor preserved across both branches.
	if strings.Count(js, "Math.max(40,") < 2 {
		t.Error("D32g-fix-3: both branches must preserve the 40px minimum control offset")
	}
	if strings.Count(js, "* 0.45") < 2 {
		t.Error("D32g-fix-3: both branches must preserve the 0.45 control-offset scale factor")
	}
}

// TestExplorer_D32gFix3_NoStaticFrontendFallback confirms the
// geometry fix did not introduce any hardcoded demo IDs or fallback
// data. Geometry helpers are pure math — they should reference no
// product entities.
func TestExplorer_D32gFix3_NoStaticFrontendFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, path := range []string{
		"/explorer/assets/js/governance-map/layout.js",
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/authority/authority-graph-view.js",
	} {
		js := getExplorerAsset(t, srv, path)
		for _, banned := range []string{
			"STRUCTURAL_CONTEXT",
			"'bs-cards'",
			"'bs-demo-authority-showcase'",
			"'surf-demo-",
			"'profile-demo-",
			"'grant-demo-",
			"'fmp-demo-",
			"'agent-v2-",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D32g-fix-3: %s must NOT hardcode %q (geometry is data-agnostic)", path, banned)
			}
		}
	}
}
