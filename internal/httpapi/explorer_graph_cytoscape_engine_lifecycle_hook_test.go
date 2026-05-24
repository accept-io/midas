package httpapi

import (
	"strings"
	"testing"
)

const d37pEdgeHookEngineAsset = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"

func TestExplorer_GraphCytoscapeEngine_LifecycleHookContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pEdgeHookEngineAsset)

	for _, want := range []string{
		"var _lifecycleCleanups      = [];",
		"function _registerLifecycleCleanup(fn)",
		"function _runLifecycleCleanups()",
		"_runLifecycleCleanups();",
		"if (_isFn(opts.onReady))",
		"var readyCleanup = opts.onReady({",
		"cytoscape: cy,",
		"handle: handle,",
		"container: container,",
		"registerCleanup: _registerLifecycleCleanup,",
		"if (_isFn(readyCleanup)) _registerLifecycleCleanup(readyCleanup);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Cytoscape edge event hook: engine lifecycle hook contract must include %q", want)
		}
	}
}

func TestExplorer_GraphCytoscapeEngine_LifecycleHookIsLensNeutral(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := _stripJSComments(getExplorerAsset(t, srv, d37pEdgeHookEngineAsset))

	for _, banned := range []string{
		"context-edge",
		"context connector",
		"connectorType",
		"hoverSummary",
		"gmap-connector-tooltip",
		"service_contains_capability",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p Cytoscape edge event hook: shared engine hook must stay lens-neutral and not include %q", banned)
		}
	}
}

func TestExplorer_GraphCytoscapeEngine_LifecycleHookPreservesExistingEngineContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pEdgeHookEngineAsset)

	for _, want := range []string{
		"cy.on('tap', 'node', function (evt) {",
		"opts.selectionAdapter(evt, handle)",
		"var cameraDelegate = opts.cameraAdapter(handle);",
		"g.graphCameraBus.registerLens(lensId, cameraDelegate)",
		"g.graphCytoscapeOverlay.mount(cy, container, {",
		"cy.fit(undefined, 24)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Cytoscape edge event hook: existing engine contract must remain present: %q", want)
		}
	}
}
