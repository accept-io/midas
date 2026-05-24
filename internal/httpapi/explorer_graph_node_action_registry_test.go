package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

const (
	d37qNodeActionRegistryAsset = "/explorer/assets/js/graph/graph-platform/graph-node-action-registry.js"
	d37qNodeActionMenuAsset     = "/explorer/assets/js/graph/graph-platform/graph-node-action-menu.js"
	d37qNodeActionFixtureAsset  = "/explorer/assets/js/graph/graph-platform/graph-node-action-fixture.test-only.js"
	d37qNodeActionCSSAsset      = "/explorer/assets/css/graph-node-action-menu.css"
	d37qContextPainterAsset     = "/explorer/assets/js/graph/context/context-html-card-painter.js"
	d37qContextRendererAsset    = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37qOverlayAsset            = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js"
)

func d37qExplorer(t *testing.T) (*Server, string) {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	return srv, body
}

func TestExplorer_GraphNodeActionRegistry_LoadedAfterPlatformPrerequisites(t *testing.T) {
	_, body := d37qExplorer(t)
	order := []string{
		"graph-footprint-measurement-sink.js",
		"graph-footprint-measurement-adapter.js",
		"graph-node-action-registry.js",
		"graph-node-action-menu.js",
	}
	last := -1
	for _, asset := range order {
		idx := strings.Index(body, asset)
		if idx < 0 {
			t.Fatalf("%s must be loaded by index.html", asset)
		}
		if idx <= last {
			t.Fatalf("%s must load after the previous platform prerequisite", asset)
		}
		last = idx
	}
}

func TestExplorer_GraphNodeActionRegistry_PublicSurfaceAndSemantics(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qNodeActionRegistryAsset)

	required := []string{
		"window.MIDASExplorerGraph.nodeActionRegistry",
		"registerActions: registerActions",
		"resolveActions: resolveActions",
		"hasActions: hasActions",
		"clearForLens: clearForLens",
		"REGISTRY[key].actions[action.id] = action",
		"!!action.enabled(context || {})",
		"node_action_enabled_resolver_error",
		"node_action_registry_error",
		"delete REGISTRY[keys[i]]",
		"__midasNodeActionDiagnostics",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("registry must contain %q", needle)
		}
	}
}

func TestExplorer_GraphNodeActionRegistry_IsPurePlatformNoDomOrCyAccess(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qNodeActionRegistryAsset)
	forbidden := []string{
		"document.",
		"querySelector",
		"createElement",
		"cy.",
		"cytoscape",
		".contains(",
	}
	for _, needle := range forbidden {
		if strings.Contains(js, needle) {
			t.Fatalf("registry must not depend on DOM or Cytoscape; found %q", needle)
		}
	}
}

func TestExplorer_GraphNodeActionRegistry_TestOnlyFixtureIsOnlyRegistration(t *testing.T) {
	srv, body := d37qExplorer(t)
	fixture := getExplorerAsset(t, srv, d37qNodeActionFixtureAsset)

	for _, needle := range []string{
		"lensId: '__fixture__'",
		"nodeKind: '__fixture__'",
		"id: 'fixture-noop'",
		"label: 'Fixture action'",
		"registry.registerActions",
	} {
		if !strings.Contains(fixture, needle) {
			t.Fatalf("fixture must contain %q", needle)
		}
	}
	if strings.Contains(body, "graph-node-action-fixture.test-only.js") {
		t.Fatal("test-only fixture must not be loaded by index.html")
	}

	graphAssets := []string{
		d37qNodeActionMenuAsset,
		d37qContextPainterAsset,
		d37qContextRendererAsset,
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js",
	}
	for _, asset := range graphAssets {
		js := getExplorerAsset(t, srv, asset)
		if strings.Contains(js, ".registerActions(") || strings.Contains(js, "registerActions({") {
			t.Fatalf("%s must not register production node actions", asset)
		}
	}
}
