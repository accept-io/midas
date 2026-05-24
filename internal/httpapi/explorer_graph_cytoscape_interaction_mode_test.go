package httpapi

import (
	"strings"
	"testing"
)

func TestExplorer_D37pGraphCytoscapeInteractionMode_EngineExposesGenericAPI(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")

	for _, want := range []string{
		`setInteractionMode: function (modeId, modeOptions) {`,
		`return _applyInteractionMode(modeId, modeOptions);`,
		`getInteractionMode: function () {`,
		`return _interactionModeId;`,
		`getInteractionModeOptions: function () {`,
		`setNodesGrabbable: function (enabled) {`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Cytoscape interaction mode: engine handle API missing %q", want)
		}
	}
}

func TestExplorer_D37pGraphCytoscapeInteractionMode_AppliesPanAndSelectSemantics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")

	block := d37pCytoscapeInteractionBoundedBlock(t, js, "function _normaliseInteractionOptions(modeId, modeOptions)", "function _applyInteractionMode")
	for _, want := range []string{
		`if (id === 'select') {`,
		`userPanningEnabled: cyOpts.userPanningEnabled !== false,`,
		`boxSelectionEnabled: cyOpts.boxSelectionEnabled === true,`,
		`nodesGrabbable: cyOpts.nodesGrabbable === true,`,
		`autounselectify: cyOpts.autounselectify === true,`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37p Cytoscape interaction mode: normalised mode semantics missing %q", want)
		}
	}

	applyBlock := d37pCytoscapeInteractionBoundedBlock(t, js, "function _applyInteractionMode(modeId, modeOptions)", "function _resetInteractionMode")
	for _, want := range []string{
		`_setCyBoolean('userPanningEnabled', nextOptions.userPanningEnabled);`,
		`_setCyBoolean('boxSelectionEnabled', nextOptions.boxSelectionEnabled);`,
		`_setCyBoolean('autounselectify', nextOptions.autounselectify);`,
		`_setNodesGrabbable(nextOptions.nodesGrabbable);`,
		`container.setAttribute('data-interaction-mode', nextId);`,
		`container.setAttribute('data-nodes-grabbable', nextOptions.nodesGrabbable ? 'true' : 'false');`,
	} {
		if !strings.Contains(applyBlock, want) {
			t.Errorf("D37p Cytoscape interaction mode: apply path missing %q", want)
		}
	}
}

func TestExplorer_D37pGraphCytoscapeInteractionMode_CleanupRestoresSafeDefaults(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")

	block := d37pCytoscapeInteractionBoundedBlock(t, js, "function _resetInteractionMode()", "var handle = {")
	for _, want := range []string{
		`_interactionModeId = '';`,
		`_interactionModeOptions = {};`,
		`_setCyBoolean('userPanningEnabled', true);`,
		`_setCyBoolean('boxSelectionEnabled', false);`,
		`_setCyBoolean('autounselectify', false);`,
		`_setNodesGrabbable(false);`,
		`container.removeAttribute('data-interaction-mode');`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37p Cytoscape interaction mode: cleanup/default path missing %q", want)
		}
	}
	if !strings.Contains(js, `_resetInteractionMode();`) {
		t.Errorf("D37p Cytoscape interaction mode: destroy must reset interaction mode before cy teardown")
	}
}

func TestExplorer_D37pGraphCytoscapeInteractionMode_RemainsLensNeutralAndDoesNotTouchOverlay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	engine := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	overlay := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")

	controllerBlock := d37pCytoscapeInteractionBoundedBlock(t, engine, "Generic Cytoscape", "var handle = {")
	for _, forbidden := range []string{`context`, `authority`, `Inspector`, `connector taxonomy`} {
		if strings.Contains(controllerBlock, forbidden) {
			t.Errorf("D37p Cytoscape interaction mode: controller must be lens-neutral, found %q", forbidden)
		}
	}
	if strings.Contains(overlay, "graphInteractionModeToolbar") ||
		strings.Contains(overlay, "data-graph-interaction-mode-toolbar") {
		t.Errorf("D37p Cytoscape interaction mode: overlay projection must remain decoupled from interaction toolbar")
	}
	if !strings.Contains(overlay, `var SYNC_EVENTS         = 'render pan zoom position layoutstop';`) {
		t.Errorf("D37p Cytoscape interaction mode: overlay must already sync on Cytoscape position events for drag-follow")
	}
}

func d37pCytoscapeInteractionBoundedBlock(t *testing.T, body, start, end string) string {
	t.Helper()
	startIdx := strings.Index(body, start)
	if startIdx < 0 {
		t.Fatalf("D37p Cytoscape interaction mode: start marker %q missing", start)
	}
	endIdx := strings.Index(body[startIdx:], end)
	if endIdx < 0 {
		t.Fatalf("D37p Cytoscape interaction mode: end marker %q missing after %q", end, start)
	}
	return body[startIdx : startIdx+endIdx]
}
