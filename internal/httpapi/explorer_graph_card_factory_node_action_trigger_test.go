package httpapi

import (
	"strings"
	"testing"
)

const d37qGraphRendererAsset = "/explorer/assets/js/graph/graph-renderer.js"

func TestExplorer_GraphCardFactoryNodeActionTrigger_FactoryReadsSpecAndRegistry(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qGraphRendererAsset)

	required := []string{
		"function _nodeActionContextFromSpec(spec)",
		"ctx.lensId = String(spec.lensId || ctx.lensId || '')",
		"ctx.nodeKind = String(spec.nodeKind || ctx.nodeKind || spec.kind || '')",
		"ctx.nodeLabel = String(ctx.nodeLabel || spec.name || spec.label || spec.id || 'node')",
		"function _appendNodeActionTrigger(node, spec)",
		"var lensId = String((spec && spec.lensId) || '')",
		"var nodeKind = String((spec && spec.nodeKind) || '')",
		"registry.hasActions(lensId, nodeKind, context)",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("graph card factory must read node action spec/registry field %q", needle)
		}
	}
}

func TestExplorer_GraphCardFactoryNodeActionTrigger_TriggerDomAndMenuDelegation(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qGraphRendererAsset)

	required := []string{
		"trigger = document.createElement('button')",
		"trigger.type = 'button'",
		"trigger.className = 'graph-node-action-trigger'",
		"trigger.setAttribute('data-graph-node-action-trigger', 'true')",
		"trigger.setAttribute('aria-haspopup', 'menu')",
		"trigger.setAttribute('aria-expanded', 'false')",
		"trigger.setAttribute('aria-label', 'More actions for '",
		"trigger.textContent = '\\u2026'",
		"trigger.addEventListener('pointerdown'",
		"trigger.addEventListener('mousedown'",
		"trigger.addEventListener('click'",
		"event.preventDefault();",
		"event.stopPropagation();",
		"menu.openForNode(trigger, ctx)",
		"node.appendChild(trigger)",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("graph card factory trigger contract missing %q", needle)
		}
	}

	forbidden := []string{
		"data-graph-node-action-menu",
		"role', 'menu'",
		"role=\"menu\"",
		"graph-node-action-menu-surface",
		"graph-node-action-menu-item",
	}
	for _, needle := range forbidden {
		if strings.Contains(js, needle) {
			t.Fatalf("graph card factory must not construct menu DOM; found %q", needle)
		}
	}
}

func TestExplorer_GraphCardFactoryNodeActionTrigger_NoTriggerFallbacks(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qGraphRendererAsset)

	required := []string{
		"if (!lensId || !nodeKind) return;",
		"if (!registry || typeof registry.hasActions !== 'function') return;",
		"catch (_) { hasActions = false; }",
		"if (!hasActions) return;",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("graph card factory must safely omit trigger via %q", needle)
		}
	}
}

func TestExplorer_GraphCardFactoryNodeActionTrigger_ContextRendererPopulatesLiveSpec(t *testing.T) {
	srv, _ := d37qExplorer(t)
	renderer := getExplorerAsset(t, srv, d37qContextRendererAsset)

	required := []string{
		"var spec = _contextCardToNativeSpec(card)",
		"rendererSurface.buildNodeCardElement(spec)",
		"lensId:  'context'",
		"nodeKind: card.kind || ''",
		"actionContext: {",
		"nodeId:        card.id",
		"nodeKind:      card.kind || ''",
		"nodeLabel:     card.name || card.label || card.id || 'node'",
		"sourceNodeRef: card.sourceNodeRef || null",
		"cardMetadata:  card",
	}
	for _, needle := range required {
		if !strings.Contains(renderer, needle) {
			t.Fatalf("live Strategic Context path must populate factory node-action spec field %q", needle)
		}
	}
}

func TestExplorer_GraphCardFactoryNodeActionTrigger_LiveDecisionSurfaceRegressionPin(t *testing.T) {
	srv, _ := d37qExplorer(t)
	factory := getExplorerAsset(t, srv, d37qGraphRendererAsset)
	renderer := getExplorerAsset(t, srv, d37qContextRendererAsset)
	registration := getExplorerAsset(t, srv, d37qContextNodeActionsAsset)
	formatter := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-object-reference-formatter.js")

	for _, needle := range []string{
		"DECISION_SURFACE_KIND = 'decision_surface'",
		"LENS_ID = 'context'",
		"id: COPY_REFERENCE_ACTION_ID",
		"enabled: hasStableId",
	} {
		if !strings.Contains(registration, needle) {
			t.Fatalf("Context Copy reference registration must remain wired for live decision_surface trigger; missing %q", needle)
		}
	}
	if !strings.Contains(formatter, "sourceRef && _trim(sourceRef.kind) === DECISION_SURFACE_KIND") ||
		!strings.Contains(formatter, "_extractDecisionSurfaceId(sourceRef.id)") {
		t.Fatal("Decision Surface formatter must still accept stable sourceNodeRef metadata")
	}
	if !strings.Contains(renderer, "sourceNodeRef: card.sourceNodeRef || null") ||
		!strings.Contains(renderer, "cardMetadata:  card") {
		t.Fatal("Strategic Context live spec must pass sourceNodeRef/cardMetadata into the shared factory")
	}
	if !strings.Contains(factory, "registry.hasActions(lensId, nodeKind, context)") ||
		!strings.Contains(factory, "trigger.setAttribute('data-graph-node-action-trigger', 'true')") {
		t.Fatal("shared factory must resolve registered actions and render the global trigger")
	}
}

func TestExplorer_GraphCardFactoryNodeActionTrigger_IneligibleKindsAndAuthorityStaySilent(t *testing.T) {
	srv, _ := d37qExplorer(t)
	registration := getExplorerAsset(t, srv, d37qContextNodeActionsAsset)

	for _, kind := range d37qContextNodeActionNegativeKinds {
		if strings.Contains(registration, "nodeKind: '"+kind+"'") || strings.Contains(registration, "nodeKind: \""+kind+"\"") {
			t.Fatalf("%s must not receive Copy reference registration", kind)
		}
	}

	for _, asset := range []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, needle := range []string{"nodeActionRegistry", "nodeActionMenu", "data-graph-node-action-trigger", "copy-reference"} {
			if strings.Contains(js, needle) {
				t.Fatalf("%s must remain isolated from node actions; found %q", asset, needle)
			}
		}
	}
}

func TestExplorer_GraphCardFactoryNodeActionTrigger_PointerEventsAndPainterPreserved(t *testing.T) {
	srv, _ := d37qExplorer(t)
	css := stripCSSComments(getExplorerAsset(t, srv, d37qNodeActionCSSAsset))
	painter := getExplorerAsset(t, srv, d37qContextPainterAsset)

	for _, needle := range []string{
		".graph-cytoscape-overlay-layer {\n  pointer-events: none;",
		".graph-cytoscape-overlay-card {\n  pointer-events: none;",
		".graph-cytoscape-overlay-card .context-card,\n.graph-cytoscape-overlay-card .context-card-body {\n  pointer-events: none;",
		"[data-graph-node-action-trigger]",
		".graph-node-action-menu-surface",
	} {
		if !strings.Contains(css, needle) {
			t.Fatalf("pointer-events contract must remain pinned by %q", needle)
		}
	}
	if count := strings.Count(css, "pointer-events: auto;"); count != 2 {
		t.Fatalf("only ellipsis and menu surface may receive pointer-events:auto, got %d", count)
	}

	for _, needle := range []string{
		"_renderNodeActionTrigger(actionContext, hasNodeActions)",
		"registry.hasActions(context.lensId, context.nodeKind, context)",
		"menu.openForNode(button, ctx)",
	} {
		if !strings.Contains(painter, needle) {
			t.Fatalf("context-html-card-painter parallel implementation must remain intact; missing %q", needle)
		}
	}
}

