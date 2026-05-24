package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

const d37qContextNodeActionsAsset = "/explorer/assets/js/graph/context/context-node-actions.js"

var d37qContextNodeActionNegativeKinds = []string{
	"business_service",
	"related_business_service",
	"capability",
	"process",
	"ai_system",
	"ai_system_binding",
	"authority_summary",
	"coverage",
}

func TestExplorer_ContextCopyReference_LoadedAfterNodeActionPlatform(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	order := []string{
		`src="/explorer/assets/js/graph/graph-platform/graph-node-action-registry.js"`,
		`src="/explorer/assets/js/graph/graph-platform/graph-node-action-menu.js"`,
		`src="/explorer/assets/js/graph/graph-platform/graph-object-reference-formatter.js"`,
		`src="/explorer/assets/js/graph/context/context-node-actions.js"`,
		`src="/explorer/assets/js/graph/context/context-cytoscape-renderer.js"`,
	}
	last := -1
	for _, asset := range order {
		idx := strings.Index(body, asset)
		if idx < 0 {
			t.Fatalf("%s must be loaded by index.html", asset)
		}
		if idx <= last {
			t.Fatalf("%s must load after the previous node action prerequisite", asset)
		}
		last = idx
	}
	if strings.Contains(body, "graph-node-action-fixture.test-only.js") {
		t.Fatal("test-only node action fixture must not be loaded by index.html")
	}
}

func TestExplorer_ContextCopyReference_RegistersExactlyDecisionSurfaceCopyReference(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qContextNodeActionsAsset)

	required := []string{
		"LENS_ID = 'context'",
		"DECISION_SURFACE_KIND = 'decision_surface'",
		"COPY_REFERENCE_ACTION_ID = 'copy-reference'",
		"registry.registerActions({",
		"lensId: LENS_ID",
		"nodeKind: DECISION_SURFACE_KIND",
		"id: COPY_REFERENCE_ACTION_ID",
		"label: 'Copy reference'",
		"enabled: hasStableId",
		"run: copyReferenceForDecisionSurface",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("context copy reference module must contain %q", needle)
		}
	}
	if strings.Count(js, "registry.registerActions({") != 1 {
		t.Fatalf("context module must register exactly one action set")
	}
	if strings.Count(js, "id: COPY_REFERENCE_ACTION_ID") != 1 {
		t.Fatalf("context module must register exactly one action")
	}
	if strings.Contains(js, "open-details") || strings.Contains(js, "Open details") {
		t.Fatal("Open details must not be exposed as a Strategic Context node action")
	}
	for _, kind := range d37qContextNodeActionNegativeKinds {
		if strings.Contains(js, "nodeKind: '"+kind+"'") || strings.Contains(js, "nodeKind: \""+kind+"\"") {
			t.Fatalf("%s must not receive a Context node action in this tranche", kind)
		}
	}
}

func TestExplorer_ContextCopyReference_DelegatesFormattingToPlatformFormatter(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qContextNodeActionsAsset)

	required := []string{
		"window.MIDASExplorerGraph.objectReferenceFormatter",
		"formatter.formatDecisionSurfaceReference(node)",
		"function _formatDecisionSurfaceReference(node)",
		"return !!_formatDecisionSurfaceReference(ctx)",
		"var text = _formatDecisionSurfaceReference(ctx)",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("context copy reference module must delegate formatting via %q", needle)
		}
	}
	forbidden := []string{
		"function formatDecisionSurfaceReference(",
		"formatDecisionSurfaceReference: formatDecisionSurfaceReference",
		"stableDecisionSurfaceId: stableDecisionSurfaceId",
		"function stableDecisionSurfaceId(",
		"function _extractDecisionSurfaceId(",
		"function _looksLikeDomOrLayoutId(",
	}
	for _, needle := range forbidden {
		if strings.Contains(js, needle) {
			t.Fatalf("context module must not own formatter implementation; found %q", needle)
		}
	}
}

func TestExplorer_ContextCopyReference_EligibilityRequiresStableId(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qContextNodeActionsAsset)

	required := []string{
		"function hasStableId(ctx)",
		"return !!_formatDecisionSurfaceReference(ctx)",
		"formatter.formatDecisionSurfaceReference(node)",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("stable-id eligibility must contain %q", needle)
		}
	}
}

func TestExplorer_ContextCopyReference_ClipboardDiagnosticsAndFailureContainment(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qContextNodeActionsAsset)

	required := []string{
		"navigator.clipboard.writeText(text)",
		"node_action_copy_reference_succeeded",
		"node_action_copy_reference_failed",
		"clipboard_unavailable",
		"clipboard_rejected",
		"clipboard_error",
		"missing_stable_id",
		"__midasNodeActionDiagnostics",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("copy action must contain clipboard diagnostic contract %q", needle)
		}
	}
}

func TestExplorer_ContextCopyReference_NoDomCyOrMenuFork(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qContextNodeActionsAsset)

	forbidden := []string{
		"document",
		"querySelector",
		"getElementById",
		"createElement",
		"data-graph-node-action-menu",
		"role=\"menu\"",
		"role', 'menu'",
		"graph-node-action-menu-surface",
		"popover",
		"cytoscape",
		".cy",
		"cy.",
	}
	for _, needle := range forbidden {
		if strings.Contains(js, needle) {
			t.Fatalf("context action module must not access DOM/Cytoscape or fork menu DOM; found %q", needle)
		}
	}
}

func TestExplorer_ContextCopyReference_PointerEventsContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := stripCSSComments(getExplorerAsset(t, srv, d37qNodeActionCSSAsset))

	required := []string{
		".graph-cytoscape-overlay-layer {\n  pointer-events: none;",
		".graph-cytoscape-overlay-card {\n  pointer-events: none;",
		".graph-cytoscape-overlay-card .context-card,\n.graph-cytoscape-overlay-card .context-card-body {\n  pointer-events: none;",
		"[data-graph-node-action-trigger]",
		".graph-node-action-menu-surface",
	}
	for _, needle := range required {
		if !strings.Contains(css, needle) {
			t.Fatalf("pointer-events contract must preserve %q", needle)
		}
	}
	if count := strings.Count(css, "pointer-events: auto;"); count != 2 {
		t.Fatalf("only ellipsis and menu surface may receive pointer-events:auto, got %d", count)
	}
}

func TestExplorer_ContextCopyReference_AuthorityAndRendererPreservationPins(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	renderer := getExplorerAsset(t, srv, d37qContextRendererAsset)
	if strings.Contains(renderer, "context-node-actions") ||
		strings.Contains(renderer, "copy-reference") ||
		strings.Contains(renderer, "Copy reference") ||
		strings.Contains(renderer, "nodeActionRegistry") {
		t.Fatal("context-cytoscape-renderer.js must not own Copy reference registration")
	}

	for _, asset := range []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, needle := range []string{"copy-reference", "Copy reference", "context-node-actions", "nodeActionRegistry"} {
			if strings.Contains(js, needle) {
				t.Fatalf("%s must remain untouched by Context node action registration; found %q", asset, needle)
			}
		}
	}
}
