package httpapi

import (
	"strings"
	"testing"
)

func TestExplorer_GraphNodeActionMenu_LoadedAfterRegistry(t *testing.T) {
	_, body := d37qExplorer(t)
	registryIdx := strings.Index(body, "graph-node-action-registry.js")
	menuIdx := strings.Index(body, "graph-node-action-menu.js")
	if registryIdx < 0 || menuIdx < 0 {
		t.Fatalf("registry and menu modules must be loaded by index.html")
	}
	if menuIdx <= registryIdx {
		t.Fatalf("menu module must load after registry module")
	}
}

func TestExplorer_GraphNodeActionMenu_PublicSurfaceAndActionResolution(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qNodeActionMenuAsset)

	required := []string{
		"window.MIDASExplorerGraph.nodeActionMenu",
		"openForNode: openForNode",
		"close: close",
		"isOpen: isOpen",
		"registry.resolveActions(ctx.lensId, ctx.nodeKind, ctx)",
		"document.querySelector('.midas-graph-viewport')",
		"_portalRoot().appendChild(menu)",
		"role', 'menu'",
		"role', 'menuitem'",
		"aria-label', 'Actions for '",
		"aria-haspopup', 'menu'",
		"aria-expanded', 'true'",
		"aria-expanded', 'false'",
		"Escape",
		"ArrowDown",
		"ArrowUp",
		"Home",
		"End",
		"Enter",
		"node_action_menu_opened",
		"node_action_menu_closed",
		"node_action_invoked",
		"node_action_handler_error",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("menu module must contain %q", needle)
		}
	}
}

func TestExplorer_GraphNodeActionMenu_RendersPortalNotInsideCard(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qNodeActionMenuAsset)
	if strings.Contains(js, ".context-card") || strings.Contains(js, "graph-cytoscape-overlay-card") {
		t.Fatal("menu platform must not render into a card or overlay card wrapper")
	}
	if !strings.Contains(js, "_portalRoot().appendChild(menu)") {
		t.Fatal("menu must render into a portal root")
	}
}

func TestExplorer_GraphNodeActionMenu_NoCytoscapeDependency(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qNodeActionMenuAsset)
	forbidden := []string{"cy.", "cytoscape", "Cytoscape"}
	for _, needle := range forbidden {
		if strings.Contains(js, needle) {
			t.Fatalf("menu module must not depend on Cytoscape; found %q", needle)
		}
	}
}
