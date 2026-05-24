package httpapi

import (
	"strings"
	"testing"
)

const (
	d37pContextInteractionRendererJS = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pContextInteractionToolbarJS  = "/explorer/assets/js/graph/graph-platform/graph-interaction-mode-toolbar.js"
	d37pContextInteractionEngineJS   = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
)

func TestExplorer_D37pContextInteractionToolbar_RegistersSharedPlatformCustomer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)

	for _, want := range []string{
		`var INTERACTION_TOOLBAR_ID = 'context-interaction-toolbar';`,
		`function _registerContextInteractionToolbar(readyCtx)`,
		`var toolbar = g.graphInteractionModeToolbar;`,
		`toolbar.register({`,
		`id: INTERACTION_TOOLBAR_ID,`,
		`rendererId: RENDERER_ID,`,
		`lensId: RENDERER_ID,`,
		`defaultMode: 'pan',`,
		`modes: _contextInteractionModes(),`,
		`getController: function () { return _interactionToolbarEngineHandle; },`,
		`enabled: _contextInteractionToolbarActive,`,
		`toolbar.activate(INTERACTION_TOOLBAR_ID);`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Context interaction toolbar: shared platform registration missing %q", want)
		}
	}
}

func TestExplorer_D37pContextInteractionToolbar_ModesDelegateToEngineOptions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)
	modesBlock := d37pContextInteractionBoundedBlock(t, js, "function _contextInteractionModes()", "function _contextInteractionToolbarActive")

	for _, want := range []string{
		`id: 'pan',`,
		`label: 'Pan Canvas',`,
		`tooltip: 'Pan Canvas',`,
		`ariaLabel: 'Pan canvas',`,
		`userPanningEnabled: true,`,
		`nodesGrabbable: false,`,
		`boxSelectionEnabled: false,`,
		`id: 'select',`,
		`label: 'Select Nodes',`,
		`tooltip: 'Select Nodes',`,
		`ariaLabel: 'Select nodes',`,
		`userPanningEnabled: false,`,
		`nodesGrabbable: true,`,
	} {
		if !strings.Contains(modesBlock, want) {
			t.Errorf("D37p Context interaction toolbar: mode contract missing %q", want)
		}
	}
	if strings.Contains(modesBlock, `id: 'lasso'`) {
		t.Errorf("D37p Context interaction toolbar: lasso must remain deferred in this tranche")
	}
}

func TestExplorer_D37pContextInteractionToolbar_EngineReadyLifecycleAndCleanup(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)

	readyBlock := d37pContextInteractionBoundedBlock(t, js, "onReady: function (readyCtx)", "});")
	for _, want := range []string{
		`var hoverCleanup = _wireContextConnectorHover(readyCtx);`,
		`var toolbarCleanup = _registerContextInteractionToolbar(readyCtx);`,
		`toolbarCleanup();`,
		`hoverCleanup();`,
	} {
		if !strings.Contains(readyBlock, want) {
			t.Errorf("D37p Context interaction toolbar: engine-ready lifecycle missing %q", want)
		}
	}

	registerBlock := d37pContextInteractionBoundedBlock(t, js, "function _registerContextInteractionToolbar(readyCtx)", "function _mountCytoscapeOverlayViaSharedModule")
	for _, want := range []string{
		`if (!handle || typeof handle.setInteractionMode !== 'function') return null;`,
		`_interactionToolbarEngineHandle = handle;`,
		`toolbar.unregister(INTERACTION_TOOLBAR_ID);`,
		`_interactionToolbarEngineHandle = null;`,
	} {
		if !strings.Contains(registerBlock, want) {
			t.Errorf("D37p Context interaction toolbar: cleanup/delegation missing %q", want)
		}
	}
}

func TestExplorer_D37pContextInteractionToolbar_RemainsScopedAndDoesNotBypassController(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)

	for _, forbidden := range []string{
		`.gmap-mode-rail`,
		`document.getElementById('gmap-pan-mode-button')`,
		`document.getElementById('gmap-select-mode-button')`,
		`.grabify(`,
		`.ungrabify(`,
		`.userPanningEnabled(`,
		`.boxSelectionEnabled(`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D37p Context interaction toolbar: strategic Context must not bypass shared toolbar/controller via %q", forbidden)
		}
	}
}

func TestExplorer_D37pContextInteractionToolbar_Phase2LassoRemainsConfigViable(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	toolbarJS := getExplorerAsset(t, srv, d37pContextInteractionToolbarJS)
	engineJS := getExplorerAsset(t, srv, d37pContextInteractionEngineJS)

	for _, want := range []string{
		`cytoscapeOptions: _isPlainObject(mode.cytoscapeOptions) ? mode.cytoscapeOptions : {},`,
		`controller.setInteractionMode(mode.id, mode.cytoscapeOptions || {});`,
	} {
		if !strings.Contains(toolbarJS, want) {
			t.Errorf("D37p Context interaction toolbar: shared toolbar must keep future mode config viable via %q", want)
		}
	}
	for _, want := range []string{
		`boxSelectionEnabled: cyOpts.boxSelectionEnabled === true,`,
		`_setCyBoolean('boxSelectionEnabled', nextOptions.boxSelectionEnabled);`,
	} {
		if !strings.Contains(engineJS, want) {
			t.Errorf("D37p Context interaction toolbar: engine must keep future box/lasso mode viable via %q", want)
		}
	}
}

func d37pContextInteractionBoundedBlock(t *testing.T, body, start, end string) string {
	t.Helper()
	startIdx := strings.Index(body, start)
	if startIdx < 0 {
		t.Fatalf("D37p Context interaction toolbar: start marker %q missing", start)
	}
	endIdx := strings.Index(body[startIdx:], end)
	if endIdx < 0 {
		t.Fatalf("D37p Context interaction toolbar: end marker %q missing after %q", end, start)
	}
	return body[startIdx : startIdx+endIdx]
}
