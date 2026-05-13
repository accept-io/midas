package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32f_impl1_test.go — D32f-impl-1 Authority Graph
// productisation contract tests. Pins:
//
//   - the overlays module surface (legend + layer chips + summary pills)
//   - the view's data-attribute decoration for diagnostics + posture
//   - the adapter's posture-source badge wiring
//   - the inspector's diagnostics + posture subsections
//   - lens isolation: no static frontend fallback data
//
// All tests read the production extracted modules via getExplorerAsset
// or the conceptual JS surface via getExplorerAllJS.

// TestExplorer_D32fImpl1_OverlaysModuleServed pins the new
// authority-graph-overlays.js module is fetched, exposes its
// namespace, and is loaded after the view (the view dispatches into
// it after a successful paint).
func TestExplorer_D32fImpl1_OverlaysModuleServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	if !strings.Contains(js, "window.MIDASExplorerGraph.authorityOverlays") {
		t.Error("D32f-impl-1: authority-graph-overlays.js must register on window.MIDASExplorerGraph.authorityOverlays")
	}
	if !strings.Contains(js, "function render(payload)") {
		t.Error("D32f-impl-1: authority-graph-overlays.js must export a render(payload) entry point")
	}
}

// TestExplorer_D32fImpl1_LegendLabelsAllNodeKinds confirms the
// overlays module references every adapter NODE_KIND label so the
// legend covers all seven authority node kinds.
func TestExplorer_D32fImpl1_LegendLabelsAllNodeKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	// The overlays module reads labels via adapter.nodeKindLabel() at
	// render time, so the labels themselves never appear as string
	// literals in this file. Instead, pin the read pattern: the legend
	// iterates over adapter.NODE_KINDS and looks each label up. This
	// keeps the legend automatically in sync with the adapter — any
	// future backend node-kind addition flows through the adapter's
	// NODE_KIND_LABELS table and lands on the legend without code
	// duplication.
	if !strings.Contains(js, "adapter.NODE_KINDS") {
		t.Error("D32f-impl-1: legend must iterate adapter.NODE_KINDS (single source of truth)")
	}
	if !strings.Contains(js, "adapter.nodeKindLabel") {
		t.Error("D32f-impl-1: legend must resolve each node-kind label via adapter.nodeKindLabel(k)")
	}
	if !strings.Contains(js, "adapter.EDGE_KINDS") {
		t.Error("D32f-impl-1: legend must iterate adapter.EDGE_KINDS for edge swatches")
	}
	if !strings.Contains(js, "adapter.edgeKindLabel") {
		t.Error("D32f-impl-1: legend must resolve each edge-kind label via adapter.edgeKindLabel(k)")
	}
}

// TestExplorer_D32fImpl1_LegendDiagnosticAndPostureSwatches pins the
// fixed swatches for diagnostic severity (critical/warning/info) and
// fail-mode posture (override/inherited/missing/dangling).
func TestExplorer_D32fImpl1_LegendDiagnosticAndPostureSwatches(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	// Diagnostic severity swatches.
	for _, sev := range []string{
		`data-diagnostic-severity="critical"`,
		`data-diagnostic-severity="warning"`,
		`data-diagnostic-severity="info"`,
	} {
		if !strings.Contains(js, sev) {
			t.Errorf("D32f-impl-1: legend must include diagnostic swatch %q", sev)
		}
	}
	// Posture swatches.
	for _, status := range []string{
		`data-fmp-status="override"`,
		`data-fmp-status="inherited"`,
		`data-fmp-status="missing"`,
		`data-fmp-status="dangling"`,
	} {
		if !strings.Contains(js, status) {
			t.Errorf("D32f-impl-1: legend must include posture swatch %q", status)
		}
	}
}

// TestExplorer_D32fImpl1_LayerChipsDeclared pins the five canonical
// layer chips registered by the overlays module.
func TestExplorer_D32fImpl1_LayerChipsDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	for _, chipID := range []string{
		"'authority-spine'",
		"'diagnostics'",
		"'surface-posture'",
		"'escalation'",
		"'fail-mode'",
	} {
		if !strings.Contains(js, chipID) {
			t.Errorf("D32f-impl-1: layer chip %s must be declared", chipID)
		}
	}
	// authority-spine is always-on (no off state).
	if !strings.Contains(js, "alwaysOn: true") {
		t.Error("D32f-impl-1: at least one layer chip must declare alwaysOn (the Authority spine)")
	}
	// Chip click handler delegates state, doesn't fetch.
	if strings.Contains(js, "fetch(") || strings.Contains(js, "ExplorerAPI.graphs.authority(") {
		t.Error("D32f-impl-1: layer chip toggle must NOT trigger a backend refetch")
	}
	if !strings.Contains(js, "_applyLayerState") {
		t.Error("D32f-impl-1: layer chips must dispatch through _applyLayerState (CSS-class toggle)")
	}
}

// TestExplorer_D32fImpl1_LayerHideCSS pins the CSS rules that hide
// the four togglable layers when their chip is off.
func TestExplorer_D32fImpl1_LayerHideCSS(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, rule := range []string{
		".authority-layer-diagnostics-off",
		".authority-layer-surface-posture-off",
		".authority-layer-escalation-off",
		".authority-layer-fail-mode-off",
	} {
		if !strings.Contains(css, rule) {
			t.Errorf("D32f-impl-1: authority-graph.css must declare layer-hide rule %q", rule)
		}
	}
	// Fail-mode layer must hide BOTH the fail_mode_policy nodes AND
	// the fail-mode-policy edges.
	if !strings.Contains(css, `.authority-layer-fail-mode-off .gmap-node[data-projection-kind="fail_mode_policy"]`) {
		t.Error("D32f-impl-1: fail-mode layer-hide must target fail_mode_policy nodes")
	}
	if !strings.Contains(css, `.authority-layer-fail-mode-off .authority-connector-surface_has_fail_mode_policy`) {
		t.Error("D32f-impl-1: fail-mode layer-hide must target surface_has_fail_mode_policy connectors")
	}
	// Escalation layer must hide BOTH escalation_target nodes AND
	// profile_escalates_to edges.
	if !strings.Contains(css, `.authority-layer-escalation-off .gmap-node[data-projection-kind="escalation_target"]`) {
		t.Error("D32f-impl-1: escalation layer-hide must target escalation_target nodes")
	}
	if !strings.Contains(css, `.authority-layer-escalation-off .authority-connector-profile_escalates_to`) {
		t.Error("D32f-impl-1: escalation layer-hide must target profile_escalates_to connectors")
	}
}

// TestExplorer_D32fImpl1_ViewWritesDiagnosticSeverity pins the view's
// per-node data-diagnostic-severity attribute (driven by
// projection.diagnostics[] severity precedence: critical > warning > info).
func TestExplorer_D32fImpl1_ViewWritesDiagnosticSeverity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, "setAttribute('data-diagnostic-severity'") {
		t.Error("D32f-impl-1: view must set data-diagnostic-severity on nodes named in projection.diagnostics[]")
	}
	if !strings.Contains(js, "_computeNodeOverlays") {
		t.Error("D32f-impl-1: view must compute per-node overlay indexes once per render via _computeNodeOverlays")
	}
	if !strings.Contains(js, "_severityWins") {
		t.Error("D32f-impl-1: view must enforce severity precedence (critical > warning > info) via _severityWins")
	}
}

// TestExplorer_D32fImpl1_ViewWritesSurfacePosture pins the view's
// per-surface posture data-attributes.
func TestExplorer_D32fImpl1_ViewWritesSurfacePosture(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, attr := range []string{
		`setAttribute('data-fmp-status'`,
		`setAttribute('data-agent-status'`,
		`setAttribute('data-profile-status'`,
		`setAttribute('data-grant-status'`,
	} {
		if !strings.Contains(js, attr) {
			t.Errorf("D32f-impl-1: view must set %s on surface nodes from projection.surface_posture[]", attr)
		}
	}
}

// TestExplorer_D32fImpl1_AdapterPostureBadgeSourceFixed pins the
// posture-source badge wiring on decision_surface nodes. The pre-D32f
// adapter checked 'business_service' which never matched the projection's
// 'business_service_default' enum; the fix expands the match to cover all
// three canonical EffectivePolicySource* values.
func TestExplorer_D32fImpl1_AdapterPostureBadgeSourceFixed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")

	// Inherited posture must match the canonical "business_service_default"
	// projection enum, not the legacy mis-spelled "business_service".
	if !strings.Contains(js, `d.effective_policy_source === 'business_service_default'`) {
		t.Error("D32f-impl-1: adapter must match effective_policy_source === 'business_service_default' for the inherited badge (was 'business_service' pre-fix)")
	}
	if strings.Contains(js, `d.effective_policy_source === 'business_service' ?`) {
		// Negative pin — the broken short-circuit form must not return.
		t.Error("D32f-impl-1: adapter must NOT match the legacy 'business_service' value (pre-D32f bug)")
	}
	// The three canonical posture source values must all be reachable.
	for _, want := range []string{`'override'`, `'business_service_default'`, `'none'`} {
		if !strings.Contains(js, want) {
			t.Errorf("D32f-impl-1: adapter must reference effective_policy_source value %s", want)
		}
	}
}

// TestExplorer_D32fImpl1_InspectorRendersDiagnostics pins the
// inspector's Diagnostics subsection (selected-node diagnostics
// surfaced from projection.diagnostics[]).
func TestExplorer_D32fImpl1_InspectorRendersDiagnostics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-inspector.js")
	if !strings.Contains(js, "_diagnosticsForNode") {
		t.Error("D32f-impl-1: inspector must define _diagnosticsForNode helper")
	}
	if !strings.Contains(js, "authority-inspector-section-diagnostics") {
		t.Error("D32f-impl-1: inspector Diagnostics subsection must use the .authority-inspector-section-diagnostics class")
	}
	if !strings.Contains(js, "_lastAuthorityProjection") {
		t.Error("D32f-impl-1: inspector must read MIDASExplorerGraph._lastAuthorityProjection for selected-node overlays")
	}
}

// TestExplorer_D32fImpl1_InspectorRendersPosture pins the inspector's
// Surface Posture subsection for decision_surface selections.
func TestExplorer_D32fImpl1_InspectorRendersPosture(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-inspector.js")
	if !strings.Contains(js, "_postureForSurface") {
		t.Error("D32f-impl-1: inspector must define _postureForSurface helper")
	}
	if !strings.Contains(js, "authority-inspector-section-posture") {
		t.Error("D32f-impl-1: inspector Surface Posture subsection must use the .authority-inspector-section-posture class")
	}
	// Posture must surface all six axes.
	for _, axis := range []string{
		"authority_status",
		"profile_status",
		"grant_status",
		"agent_status",
		"fail_mode_policy_status",
		"escalation_status",
	} {
		if !strings.Contains(js, axis) {
			t.Errorf("D32f-impl-1: inspector posture subsection must surface axis %q", axis)
		}
	}
}

// TestExplorer_D32fImpl1_NoStaticFrontendFallback pins the negative
// requirement: NO authority module is allowed to introduce hardcoded
// demo entity IDs, STRUCTURAL_CONTEXT references, or static fallback
// graph nodes.
func TestExplorer_D32fImpl1_NoStaticFrontendFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, path := range []string{
		"/explorer/assets/js/graph/authority/authority-graph-adapter.js",
		"/explorer/assets/js/graph/authority/authority-graph-view.js",
		"/explorer/assets/js/graph/authority/authority-graph-inspector.js",
		"/explorer/assets/js/graph/authority/authority-graph-overlays.js",
		"/explorer/assets/js/graph/authority/authority-diagnostics-panel.js",
		"/explorer/assets/js/graph/authority/authority-surface-posture-panel.js",
	} {
		js := getExplorerAsset(t, srv, path)
		for _, forbidden := range []string{
			"STRUCTURAL_CONTEXT",
			"'bs-cards'",
			"'bs-retail-banking'",
			"'bs-consumer-lending'",
			"'bs-demo-authority-showcase'",
			"'surf-demo-",
			"'profile-demo-",
			"'grant-demo-",
			"'fmp-demo-",
			"'agent-v2-",
		} {
			if strings.Contains(js, forbidden) {
				t.Errorf("D32f-impl-1: %s must NOT hardcode %q (no static frontend data)", path, forbidden)
			}
		}
	}
}

// TestExplorer_D32fImpl1_OverlaysRenderedAfterPaint pins the view's
// dispatch into the overlays module immediately after a successful
// graph paint.
func TestExplorer_D32fImpl1_OverlaysRenderedAfterPaint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, "MIDASExplorerGraph.authorityOverlays") {
		t.Error("D32f-impl-1: view must reach into MIDASExplorerGraph.authorityOverlays after paint")
	}
	if !strings.Contains(js, "overlaysModule.render(payload)") {
		t.Error("D32f-impl-1: view must invoke overlaysModule.render(payload) after a successful paint")
	}
}

// TestExplorer_D32fImpl1_LayerToggleNoRefetch pins that the overlays
// module's chip toggle path does not call into the API client or
// shell.refresh. Toggling a layer is a client-side CSS change.
func TestExplorer_D32fImpl1_LayerToggleNoRefetch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	for _, forbidden := range []string{
		"shell.refresh",
		"ExplorerAPI.graphs",
		"fetch('/v1",
		"fetch(\"/v1",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D32f-impl-1: overlays module must NOT call %s (chip toggle is client-side only)", forbidden)
		}
	}
}

// TestExplorer_D32fImpl1_SummaryPillSourceFields pins that the summary
// pill rendering reads from projection.summary + projection.diagnostic_summary
// fields and not from invented client-side counts.
func TestExplorer_D32fImpl1_SummaryPillSourceFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	for _, field := range []string{
		"summary.surface_count",
		"summary.active_profile_count",
		"summary.active_grant_count",
		"summary.active_agent_count",
		"summary.surfaces_without_profiles",
		"summary.profiles_without_grants",
		"summary.grants_without_agents",
		"summary.surfaces_without_effective_fail_mode_policy",
		"summary.policies_missing_active_version",
		"summary.grants_with_stop_capability",
		"summary.profiles_with_dangling_escalation_target",
		"diagSummary.critical",
		"diagSummary.warning",
		"diagSummary.info",
	} {
		if !strings.Contains(js, field) {
			t.Errorf("D32f-impl-1: summary pill must read backend field %q (no client-derived counts)", field)
		}
	}
}

// TestExplorer_D32fImpl1_DataProjectionKindAttribute pins the view's
// data-projection-kind attribute on node cards (used by CSS layer-hide
// selectors and by the legend's swatch indexing).
func TestExplorer_D32fImpl1_DataProjectionKindAttribute(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, `setAttribute('data-projection-kind', node.kind)`) {
		t.Error("D32f-impl-1: view must set data-projection-kind on every authority node (drives layer-hide selectors)")
	}
}

// TestExplorer_D32fImpl1_OverlaysModuleListedInTestHarness confirms
// the new overlays module is registered in explorerGraphJSFiles so
// getExplorerAllJS picks it up for conceptual-surface tests.
func TestExplorer_D32fImpl1_OverlaysModuleListedInTestHarness(t *testing.T) {
	var found bool
	for _, name := range explorerGraphJSFiles {
		if name == "graph/authority/authority-graph-overlays" {
			found = true
			break
		}
	}
	if !found {
		t.Error("D32f-impl-1: explorerGraphJSFiles must include graph/authority/authority-graph-overlays so getExplorerAllJS reads it")
	}
}
