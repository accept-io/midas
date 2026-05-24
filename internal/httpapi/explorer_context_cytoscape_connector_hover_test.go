package httpapi

import (
	"strings"
	"testing"
)

const d37pContextConnectorHoverRenderer = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"

func TestExplorer_ContextCytoscapeConnectorHover_UsesEngineLifecycleHook(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextConnectorHoverRenderer)

	for _, want := range []string{
		"onReady: function (readyCtx) {",
		"var hoverCleanup = _wireContextConnectorHover(readyCtx);",
		"hoverCleanup();",
		"function _wireContextConnectorHover(readyCtx)",
		"var cy = readyCtx && readyCtx.cytoscape;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Context connector hover: Context must consume the shared engine lifecycle hook via %q", want)
		}
	}
}

func TestExplorer_ContextCytoscapeConnectorHover_WiresEdgeEventsAndCleanup(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextConnectorHoverRenderer)

	for _, want := range []string{
		"cy.on('mouseover', 'edge', onOver)",
		"cy.on('mousemove', 'edge', onMove)",
		"cy.on('mouseout', 'edge', onOut)",
		"cy.on('pan zoom layoutstart', hide)",
		"cy.off('mouseover', 'edge', onOver)",
		"cy.off('mousemove', 'edge', onMove)",
		"cy.off('mouseout', 'edge', onOut)",
		"cy.off('pan zoom layoutstart', hide)",
		"_hideContextConnectorTooltip();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Context connector hover: edge event wiring/cleanup must include %q", want)
		}
	}
}

func TestExplorer_ContextCytoscapeConnectorHover_UsesSemanticTooltipContent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextConnectorHoverRenderer)

	for _, want := range []string{
		"function _str(value)",
		"var label = data('label') || data('connectorLabel') || _humanizeConnectorType(data('connectorType') || data('edgeKind'));",
		"var summary = data('hoverSummary') || data('accessibilityLabel');",
		"summary = sourceLabel + ' -> ' + targetLabel;",
		"kind.textContent = payload.label;",
		"route.textContent = payload.summary || '';",
		"document.getElementById('gmap-connector-tooltip')",
		"document.getElementsByClassName('governance-map-body')[0]",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Context connector hover: semantic tooltip content must include %q", want)
		}
	}

	if strings.Contains(js, "edge.id()") && strings.Contains(js, "kind.textContent = edge.id()") {
		t.Errorf("D37p Context connector hover: raw edge id must not be the primary visible tooltip label")
	}
}

func TestExplorer_ContextCytoscapeConnectorHover_EmphasisIsReversibleAndLabelsStayHoverOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextConnectorHoverRenderer)
	style := sliceStyleBuilderBody(t, js)

	for _, want := range []string{
		"selector: 'edge.context-edge-hovered'",
		"'width':                     1.8",
		"'opacity':                   1",
		"'arrow-scale':               1.5",
		"'target-distance-from-node': 20",
		"edge.addClass('context-edge-hovered')",
		"_hoveredConnectorEdge.removeClass('context-edge-hovered')",
	} {
		if !strings.Contains(js, want) && !strings.Contains(style, want) {
			t.Errorf("D37p Context connector hover: reversible hover emphasis must include %q", want)
		}
	}

	if strings.Contains(style, "'label':                'data(label)'") ||
		strings.Contains(style, "'label': 'data(label)'") {
		t.Errorf("D37p Context connector hover: persistent Cytoscape edge labels must remain disabled")
	}
}

func TestExplorer_ContextCytoscapeConnectorHover_DisallowedSurfacesRemainUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	disallowedAssets := []string{
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js",
		"/explorer/assets/js/graph/graph-platform/graph-inspector-platform.js",
		"/explorer/assets/js/graph/graph-platform/graph-stage.js",
		"/explorer/assets/js/graph/graph-platform/graph-footprint-policy.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/authority/authority-graph-connectors.js",
	}

	for _, asset := range disallowedAssets {
		body := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"context-edge-hovered",
			"_wireContextConnectorHover",
			"gmap-connector-tooltip",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D37p Context connector hover: disallowed/reference asset %s must not carry hover implementation marker %q", asset, banned)
			}
		}
	}

	renderer := getExplorerAsset(t, srv, d37pContextConnectorHoverRenderer)
	for _, banned := range []string{
		"openRelationshipBrowser",
		"connectorInspector",
		"openConnectorModal",
		"zoomControl",
		"fitControl",
		"reframeControl",
	} {
		if strings.Contains(renderer, banned) {
			t.Errorf("D37p Context connector hover: hover implementation must not add relationship browser, inspector chrome, or graph control %q", banned)
		}
	}
}
