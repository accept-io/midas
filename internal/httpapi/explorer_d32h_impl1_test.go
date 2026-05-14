package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32h_impl1_test.go — D32h-impl-1 pins the Authority Graph
// card-layout adapter, the pure layout helper, the spec-driven view,
// the shared layer classifier extensions, and the ctx-hook integration.
//
// These tests are deliberately fewer in count than the D32g family's
// source-string pins; D32g-analysis-4 documented that string-only pins
// proved insufficient. Where possible we pin BEHAVIOURAL contracts
// (presence of helper functions, ordering of two-line coordinate
// contract, shared-classifier coverage of every Authority kind, removal
// of the obsolete kind-bucketed planner). A future tranche
// (D32h-test-1) will add a goja/JSDOM harness for true layout-spec
// snapshot tests; until then the surface assertions guard the smallest
// observable invariants.

// TestExplorer_D32hImpl1_AdapterDeclaresMapToCardLayout pins that the
// Authority adapter publishes the typed layout-spec mapper — the
// methodological mirror of Context's adapter.mapToCardLayout.
func TestExplorer_D32hImpl1_AdapterDeclaresMapToCardLayout(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	for _, want := range []string{
		"function mapToCardLayout(projection, view)",
		"mapToCardLayout:        mapToCardLayout,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority adapter must declare %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_AdapterEmitsChainAndGovernanceSlots pins the
// shape of the layout spec. The adapter must emit chains[], governance
// (with failModePolicies / escalationTargets), and the helper owner-
// chain maps the layout planner relies on for centroid placement.
func TestExplorer_D32hImpl1_AdapterEmitsChainAndGovernanceSlots(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	for _, want := range []string{
		"chains:             chains,",
		"governance:         { failModePolicies: fmpSpec, escalationTargets: etSpec, unlinked: [] },",
		"profileOwnerChains:",
		"grantOwnerChains:",
		"agentOwnerChains:",
		"nodesByRef:",
		"unlinked:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority adapter mapToCardLayout must emit %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_AdapterChainWalksSpineEdges pins that the
// adapter walks the four spine edge kinds (business_service_has_surface,
// surface_uses_profile, profile_has_grant, grant_authorises_agent) and
// the three governance edge kinds when building chains. Missing any
// edge kind would silently drop the corresponding chain link.
func TestExplorer_D32hImpl1_AdapterChainWalksSpineEdges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	for _, want := range []string{
		"case 'business_service_has_surface':",
		"case 'surface_uses_profile':",
		"case 'profile_has_grant':",
		"case 'grant_authorises_agent':",
		"case 'surface_has_fail_mode_policy':",
		"case 'business_service_has_fail_mode_policy':",
		"case 'profile_escalates_to':",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority adapter must classify edge kind %q during chain extraction", want)
		}
	}
}

// TestExplorer_D32hImpl1_AdapterTracksSharedNodes pins that profiles,
// grants, and agents shared between multiple chains are detected. The
// `seenSharedProfile / seenSharedGrant / seenSharedAgent` maps support
// the layout planner's centroid placement rule.
func TestExplorer_D32hImpl1_AdapterTracksSharedNodes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	for _, want := range []string{
		"seenSharedProfile",
		"seenSharedGrant",
		"seenSharedAgent",
		"profileShared:",
		"grantShared:",
		"agentShared:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority adapter must track shared chain node %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_AdapterStableOrdering pins backend-emission
// ordering for chains. Hash-iteration / random re-ordering would cause
// the layout to flicker between renders.
func TestExplorer_D32hImpl1_AdapterStableOrdering(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	if !strings.Contains(js, "for (var si = 0; si < byKind.decision_surface.length; si++)") {
		t.Error("D32h-impl-1: authority adapter must walk decision_surface array in backend emission order (stable chain ordering)")
	}
}

// TestExplorer_D32hImpl1_LayoutHelperPublished pins that the pure
// layout module is served and registers under the expected namespace.
func TestExplorer_D32hImpl1_LayoutHelperPublished(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityLayout",
		// D32h-fix-2c — relaxed from "function computeAuthorityLayout(spec, GMAP)"
		// (closing paren) to the open form so the new three-arg signature
		// `(spec, GMAP, layerState)` still matches this pin. Both signatures
		// satisfy the substring check.
		"function computeAuthorityLayout(spec, GMAP",
		"computeAuthorityLayout: computeAuthorityLayout,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority layout helper must declare %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_LayoutHelperLoadedFromIndex pins that
// index.html includes the layout helper script BEFORE the view
// (which depends on it).
func TestExplorer_D32hImpl1_LayoutHelperLoadedFromIndex(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, "GET", "/explorer", nil)
	if rec.Code != 200 {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	html := rec.Body.String()
	layoutIdx := strings.Index(html, "/explorer/assets/js/graph/authority/authority-graph-layout.js")
	viewIdx   := strings.Index(html, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if layoutIdx < 0 {
		t.Fatal("D32h-impl-1: index.html must load authority-graph-layout.js")
	}
	if viewIdx < 0 {
		t.Fatal("D32h-impl-1: index.html must load authority-graph-view.js")
	}
	if layoutIdx >= viewIdx {
		t.Errorf("D32h-impl-1: authority-graph-layout.js (offset %d) must load BEFORE authority-graph-view.js (offset %d)", layoutIdx, viewIdx)
	}
}

// TestExplorer_D32hImpl1_LayoutHelperImplementsTopologyRules pins the
// four topology rules called out in the file header (R1 chain
// alignment, R2 shared-node centroid, R3 sidecar adjacency, R4
// unlinked band).
func TestExplorer_D32hImpl1_LayoutHelperImplementsTopologyRules(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-layout.js")

	for _, want := range []string{
		"function centroidX(",                // R2 centroid
		"function ownerSidecar(",             // R3 sidecar adjacency resolution
		"AUTHORITY_CHAIN_GAP",                // R1/R3 lane stride
		"AUTHORITY_SIDECAR_GAP",              // R3 gap between owner and sidecar
		"unlinkedY",                          // R4 unlinked band
		"slotOccupied",                       // R3 sidecar collision handling
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority layout helper must declare %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_AuthorityConstantsAdded pins the new
// per-Authority layout constants. Y-row anchor table + chain gap +
// sidecar gap.
func TestExplorer_D32hImpl1_AuthorityConstantsAdded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	for _, want := range []string{
		"GMAP.AUTHORITY_LAYERS",
		"BUSINESS:",
		"SURFACE:",
		"PROFILE:",
		"GRANT:",
		"AGENT:",
		"GMAP.AUTHORITY_CHAIN_GAP",
		"GMAP.AUTHORITY_SIDECAR_GAP",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: governance-map/constants.js must publish %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_AuthorityConstantsDontReplaceContextLayers
// pins that the Context-lens GMAP.LAYERS table is unchanged. D32h is
// additive; it must not break Context.
func TestExplorer_D32hImpl1_AuthorityConstantsDontReplaceContextLayers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")

	// Context's per-layer y anchors must still be declared in
	// GMAP.LAYERS (note: BUSINESS appears in both tables, hence the
	// substring match below targets RELATED/CAP_PROC/AI which are
	// Context-only).
	for _, want := range []string{
		"RELATED:",
		"CAP_PROC:",
		"AI:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: Context GMAP.LAYERS entry %q must remain (D32h is additive, must not break Context)", want)
		}
	}
}

// TestExplorer_D32hImpl1_ViewConsumesSpecNotProjection pins that the
// view obtains chains from the spec rather than re-walking
// projection.edges. The kind-bucketed `var ROWS = [...]` / `var GOV =
// [...]` planner was removed.
func TestExplorer_D32hImpl1_ViewConsumesSpecNotProjection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// Required: spec-driven layout call path.
	for _, want := range []string{
		// D32h-fix-2c — relaxed from `(spec, GMAP)` (closing paren) to
		// the open form so the three-arg `(spec, GMAP, layerState)`
		// signature also satisfies this pin. Both forms continue to
		// match the substring check.
		"layout.computeAuthorityLayout(spec, GMAP",
		"adapter.mapToCardLayout(payload, ctx.view",
		// D32h-fix-2c — the view no longer walks spec.chains /
		// spec.governance / spec.unlinked directly; those collections
		// are now consumed inside computeAuthorityLayout, which emits
		// visibleNodes / visibleEdges. The view still passes spec into
		// the helper (pinned by the computeAuthorityLayout literal
		// above) and references spec.root for ctx hook orchestration.
		// The original pins for spec.chains / spec.governance /
		// spec.unlinked were view-internal walk pins, now satisfied by
		// the visibleNodes/visibleEdges contract pinned in
		// TestExplorer_D32hImpl1_ViewEmitsConnectorsByWalkingSpec and
		// the new D32h-fix-2c test file.
		"spec.root",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority view must declare spec-driven layout call: %q", want)
		}
	}
	// Forbidden: the obsolete kind-bucketed planner.
	for _, banned := range []string{
		"var ROWS = ['business_service', 'decision_surface', 'authority_profile', 'authority_grant', 'agent']",
		"var GOV  = ['fail_mode_policy', 'escalation_target']",
		"byKind[kind]",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32h-impl-1: authority view must NOT keep the obsolete kind-bucketed planner: %q", banned)
		}
	}
}

// TestExplorer_D32hImpl1_ViewEmitsConnectorsByWalkingSpec pins that
// spine connectors come from spec-derived layout output, not from a
// flat projection.edges walk.
//
// D32h-fix-2c — Updated: the view's emit loop now iterates
// layoutResult.visibleEdges produced by the (spec → layout helper)
// pipeline. The old emitSpine / emitGovernance helpers were merged
// into a single emitVisibleEdge helper that handles both spine
// (anchors = ['bottom','top']) and governance (anchors = 'pick')
// edges. The structural invariants (no flat projection.edges walk,
// no kind-bucketed planner) remain unchanged.
func TestExplorer_D32hImpl1_ViewEmitsConnectorsByWalkingSpec(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// The view must consume layoutResult.visibleEdges (D32h-fix-2c
	// contract) — a topology-derived edge list the layout helper
	// produced from spec.chains + spec.governance + layerState.
	for _, want := range []string{
		"function emitVisibleEdge(",
		"layoutResult.visibleEdges",
		"visibleEdges[vei]",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority view must emit connectors via the layout helper's visibleEdges output: %q", want)
		}
	}
	// The pre-D32h flat-edge iteration block is gone.
	for _, banned := range []string{
		"for (var ei = 0; ei < projection.edges.length; ei++)",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32h-impl-1: authority view must NOT iterate projection.edges directly: %q", banned)
		}
	}
}

// TestExplorer_D32hImpl1_ViewUsesCtxHookSequence pins that the view
// dispatches the Context-style six-call ctx hook sequence at the end
// of render — mirror of context-graph-view.js:628-636.
func TestExplorer_D32hImpl1_ViewUsesCtxHookSequence(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"ctx.setCurrentRoot",
		"ctx.selectNode",
		"ctx.applyZoom",
		"ctx.focusOnRoot",
		"ctx.applyFitMode",
		"ctx.scheduleFitToView",
		"ctx.applyMultiSelection",
	} {
		if !strings.Contains(js, "typeof "+want+" === 'function'") {
			t.Errorf("D32h-impl-1: authority view must dispatch %s via the shared ctx hook bag (mirror of Context's contract)", want)
		}
	}
}

// TestExplorer_D32hImpl1_ViewWiresRenderCtxThroughRefresh pins that
// refresh() flows the shared _gmapRenderCtx hook bag into render so
// camera + selection + summary are driven by the inline workbench
// hooks, not by direct camera-module reaches.
func TestExplorer_D32hImpl1_ViewWiresRenderCtxThroughRefresh(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		"function _renderCtx()",
		"window.MIDASExplorerGraph._renderCtx",
		"var sharedCtx = _renderCtx() || {};",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority view must resolve the shared render-context: %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_ViewCoordinateContractPreserved pins the
// dataset.baseWidth + viewBox two-liner from D32g-fix-7 survives the
// planner refactor. Coordinate plumbing is upstream of the planner
// change.
func TestExplorer_D32hImpl1_ViewCoordinateContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	baseWidthIdx := strings.Index(js, "canvas.dataset.baseWidth = canvasW;")
	viewBoxIdx   := strings.Index(js, "svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)")
	if baseWidthIdx < 0 {
		t.Error("D32h-impl-1: authority view must continue to set canvas.dataset.baseWidth = canvasW (D32g-fix-7 contract)")
	}
	if viewBoxIdx < 0 {
		t.Error("D32h-impl-1: authority view must continue to set the SVG viewBox to '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H (D32g-fix-6/7 contract)")
	}
	if baseWidthIdx >= 0 && viewBoxIdx >= 0 && baseWidthIdx >= viewBoxIdx {
		t.Errorf("D32h-impl-1: canvas.dataset.baseWidth (offset %d) must precede the viewBox setter (offset %d) — D32g-fix-7 ordering", baseWidthIdx, viewBoxIdx)
	}
}

// TestExplorer_D32hImpl1_ViewDropsFallbackDistribute pins that the
// `_fallbackDistribute` helper (only ever reached when distributeRow
// was unavailable in the kind-bucketed planner) is removed. The new
// view goes through the pure layout helper for all positioning.
func TestExplorer_D32hImpl1_ViewDropsFallbackDistribute(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	if strings.Contains(js, "function _fallbackDistribute(") {
		t.Error("D32h-impl-1: _fallbackDistribute must be removed — the kind-bucketed planner is gone")
	}
}

// TestExplorer_D32hImpl1_LayersClassifierKnowsAuthorityKinds pins
// shared classifier coverage of every Authority node kind. Pre-D32h
// the classifier returned '' for Authority kinds, so chip toggles had
// no effect on the Authority lens.
func TestExplorer_D32hImpl1_LayersClassifierKnowsAuthorityKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layers.js")

	for _, want := range []string{
		"case 'decision_surface':",
		"case 'authority_profile':",
		"case 'authority_grant':",
		"case 'agent':",
		"case 'fail_mode_policy':",
		"case 'escalation_target':",
		"case 'business_service':",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: layers.js gmapNodeCategoryFromKind must handle %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_LayersClassifierKnowsAuthorityConnectors
// pins shared classifier coverage of every Authority connector CSS
// class. Pre-D32h all Authority connectors returned {kind: 'unknown',
// label: 'Connector'} in the hover tooltip.
func TestExplorer_D32hImpl1_LayersClassifierKnowsAuthorityConnectors(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layers.js")

	for _, want := range []string{
		"authority-connector-business_service_has_surface",
		"authority-connector-surface_uses_profile",
		"authority-connector-profile_has_grant",
		"authority-connector-grant_authorises_agent",
		"authority-connector-surface_has_fail_mode_policy",
		"authority-connector-business_service_has_fail_mode_policy",
		"authority-connector-profile_escalates_to",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: layers.js gmapConnectorKindFromCls must classify %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_LayersClassifierContextUnchanged pins that
// the Context-lens classifier branches are unchanged.
func TestExplorer_D32hImpl1_LayersClassifierContextUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layers.js")

	for _, want := range []string{
		"case 'business':",
		"case 'related':",
		"case 'cap':",
		"case 'proc':",
		"case 'surface':",
		"case 'ai':",
		"case 'authority':",
		"case 'coverage':",
		"connector-ai-binding",
		"connector-authority",
		"connector-evidence",
		"connector-gap",
		"connector-service",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: Context lens classifier branch must remain: %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_AdapterUnlinkedComputed pins that the
// adapter assembles an unlinked array for the layout planner's R4
// orphan band.
func TestExplorer_D32hImpl1_AdapterUnlinkedComputed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	for _, want := range []string{
		"var assignedIds = {};",
		"function markAssigned(node)",
		"var unlinked = [];",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority adapter must compute the unlinked array: %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_AdapterPreservesPassthroughFields pins that
// the layout spec is a superset of the projection — diagnostics,
// surface_posture, summary, etc. are passed through so the inspector
// and overlays continue to read the cached _lastAuthorityProjection
// without changes.
func TestExplorer_D32hImpl1_AdapterPreservesPassthroughFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	for _, want := range []string{
		"diagnostics:        Array.isArray(projection.diagnostics)",
		"surfacePosture:     Array.isArray(projection.surface_posture)",
		"summary:            projection.summary",
		"diagnosticSummary:  projection.diagnostic_summary",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-impl-1: authority adapter mapToCardLayout must pass through %q", want)
		}
	}
}

// TestExplorer_D32hImpl1_NoHardcodedDemoIds pins that the new code
// path contains no hard-coded showcase / demo IDs. The chain layout
// is data-driven; never service-specific.
func TestExplorer_D32hImpl1_NoHardcodedDemoIds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, path := range []string{
		"/explorer/assets/js/graph/authority/authority-graph-adapter.js",
		"/explorer/assets/js/graph/authority/authority-graph-layout.js",
		"/explorer/assets/js/graph/authority/authority-graph-view.js",
	} {
		js := getExplorerAsset(t, srv, path)
		for _, banned := range []string{
			"bs-demo-authority-showcase",
			"surf-demo-blocked-agent",
			"profile-demo-",
			"grant-demo-",
			"fmp-demo-",
			"et-governance-approver",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D32h-impl-1: %s must not contain hard-coded demo id %q (the chain planner is data-driven)", path, banned)
			}
		}
	}
}

// TestExplorer_D32hImpl1_ContextGraphUnchanged pins that the Context
// view, adapter, and `_gmapRenderCtx` definition in index.html are
// unchanged. D32h is Authority-only.
func TestExplorer_D32hImpl1_ContextGraphUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	contextJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	for _, want := range []string{
		"canvas.dataset.baseWidth = canvasW",
		"svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)",
		"if (typeof ctx.applyZoom === 'function') ctx.applyZoom();",
		"if (typeof ctx.scheduleFitToView === 'function') ctx.scheduleFitToView();",
	} {
		if !strings.Contains(contextJS, want) {
			t.Errorf("D32h-impl-1: Context lens must remain unchanged — missing %q", want)
		}
	}
}
