package httpapi

import (
	"strings"
	"testing"
)

func TestExplorer_GraphNodeActionCardContract_EllipsisButtonGatedByRegistry(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qContextPainterAsset)

	required := []string{
		"NODE_ACTION_TRIGGER_ATTR = 'data-graph-node-action-trigger'",
		"nodeActionRegistry",
		"registry.hasActions(context.lensId, context.nodeKind, context)",
		"has-node-actions",
		"_renderNodeActionTrigger(actionContext, hasNodeActions)",
		"button = _el('button', 'graph-node-action-trigger')",
		"button.type = 'button'",
		"button.setAttribute(NODE_ACTION_TRIGGER_ATTR, 'true')",
		"More actions for ",
		"aria-haspopup', 'menu'",
		"aria-expanded', 'false'",
		"button.textContent = '\\u2026'",
		"if (!visible) button.hidden = true",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("card painter must contain ellipsis contract %q", needle)
		}
	}
}

func TestExplorer_GraphNodeActionCardContract_DelegatesToMenuAndStopsDragStart(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qContextPainterAsset)

	required := []string{
		"_on(button, 'pointerdown'",
		"event.stopPropagation();",
		"_on(button, 'mousedown'",
		"_on(button, 'click'",
		"event.preventDefault();",
		"menu.openForNode(button, ctx)",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("ellipsis must delegate safely; missing %q", needle)
		}
	}
}

func TestExplorer_GraphNodeActionCardContract_DoesNotConstructMenuDom(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qContextPainterAsset)

	forbidden := []string{
		"data-graph-node-action-menu",
		"role', 'menu'",
		"role=\"menu\"",
		"graph-node-action-menu-surface",
		"graph-node-action-menu-item",
	}
	for _, needle := range forbidden {
		if strings.Contains(js, needle) {
			t.Fatalf("card painter must not construct menu DOM; found %q", needle)
		}
	}
}

func TestExplorer_GraphNodeActionCardContract_ContextRendererOnlyPopulatesFactorySpec(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qContextRendererAsset)
	required := []string{
		"lensId:  'context'",
		"nodeKind: card.kind || ''",
		"actionContext: {",
		"sourceNodeRef: card.sourceNodeRef || null",
		"cardMetadata:  card",
		"rendererSurface.buildNodeCardElement(spec)",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("context renderer must populate node action factory spec field %q", needle)
		}
	}
	forbidden := []string{
		"nodeActionMenu",
		"nodeActionRegistry",
		"registerActions",
		"data-graph-node-action-menu",
		"data-graph-node-action-trigger",
		"graph-node-action-trigger",
		"openForNode",
	}
	for _, needle := range forbidden {
		if strings.Contains(js, needle) {
			t.Fatalf("context renderer may populate spec fields but must not own node action platform behaviour; found %q", needle)
		}
	}
}

func TestExplorer_GraphNodeActionCardContract_AuthorityModulesDoNotRenderEllipsis(t *testing.T) {
	srv, _ := d37qExplorer(t)
	for _, asset := range []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		forbidden := []string{
			"data-graph-node-action-trigger",
			"nodeActionRegistry",
			"nodeActionMenu",
		}
		for _, needle := range forbidden {
			if strings.Contains(js, needle) {
				t.Fatalf("%s must remain isolated from node action platform; found %q", asset, needle)
			}
		}
	}
}
