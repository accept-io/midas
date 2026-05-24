package httpapi

import (
	"strings"
	"testing"
)

func TestExplorer_D37pContextSelectModeParity_ToolbarRemainsTwoModes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)
	modesBlock := d37pContextInteractionBoundedBlock(t, js, "function _contextInteractionModes()", "function _contextInteractionToolbarActive")

	if got := strings.Count(modesBlock, "id: '"); got != 2 {
		t.Fatalf("D37p Context select parity: toolbar must stay two-mode only, got %d mode ids", got)
	}
	for _, want := range []string{
		`id: 'pan',`,
		`id: 'select',`,
		`label: 'Pan Canvas',`,
		`label: 'Select Nodes',`,
	} {
		if !strings.Contains(modesBlock, want) {
			t.Errorf("D37p Context select parity: mode contract missing %q", want)
		}
	}
	for _, forbidden := range []string{
		`id: 'multi-select'`,
		`id: 'multi_select'`,
		`id: 'lasso'`,
		`id: 'box-select'`,
		`id: 'box_select'`,
	} {
		if strings.Contains(modesBlock, forbidden) {
			t.Errorf("D37p Context select parity: must not add third multi-select/lasso mode %q", forbidden)
		}
	}
}

func TestExplorer_D37pContextSelectModeParity_SelectUsesCombinedNativeOptions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)
	modesBlock := d37pContextInteractionBoundedBlock(t, js, "function _contextInteractionModes()", "function _contextInteractionToolbarActive")

	panBlock := d37pContextInteractionBoundedBlock(t, modesBlock, "id: 'pan',", "id: 'select',")
	for _, want := range []string{
		`userPanningEnabled: true,`,
		`nodesGrabbable: false,`,
		`boxSelectionEnabled: false,`,
	} {
		if !strings.Contains(panBlock, want) {
			t.Errorf("D37p Context select parity: Pan mode changed or missing %q", want)
		}
	}

	selectBlock := d37pContextInteractionBoundedBlock(t, modesBlock, "id: 'select',", "];")
	for _, want := range []string{
		`userPanningEnabled: false,`,
		`nodesGrabbable: true,`,
		`boxSelectionEnabled: true,`,
	} {
		if !strings.Contains(selectBlock, want) {
			t.Errorf("D37p Context select parity: Select mode must use Candidate A option %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectModeParity_SelectionSetAdapterPublishesGlobalBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)
	adapterBlock := d37pContextInteractionBoundedBlock(t, js, "function _contextSelectionSetBridge()", "// _readCssVar")

	for _, want := range []string{
		`var bridge = g.graphSelectionBridge;`,
		`bridge.replaceSelectionSet(items, {`,
		`lens:      RENDERER_ID,`,
		`primaryId: (evt && evt.primaryId) || items[0].id || null,`,
		`bridge.clearSelectionSet({ lens: RENDERER_ID });`,
		`_contextSelectionSetItemsFromNodes(evt && evt.selectedNodes)`,
	} {
		if !strings.Contains(adapterBlock, want) {
			t.Errorf("D37p Context select parity: selection-set adapter missing %q", want)
		}
	}
	if !strings.Contains(js, `selectionSetAdapter: function (evt, handle)`) {
		t.Errorf("D37p Context select parity: Context must supply a shared-engine selectionSetAdapter callback")
	}

	itemBlock := d37pContextInteractionBoundedBlock(t, js, "function _contextSelectionSetItemFromNode(node)", "function _contextSelectionSetItemsFromCy(cy)")
	for _, want := range []string{
		`id:            id,`,
		`kind:`,
		`label:`,
		`sourceNodeRef:`,
		`card:          card,`,
	} {
		if !strings.Contains(itemBlock, want) {
			t.Errorf("D37p Context select parity: selection-set item descriptor missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectModeParity_PreservesSingleClickSelection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)
	mountBlock := d37pContextInteractionBoundedBlock(t, js, "selectionAdapter: function (evt, handle)", "cameraAdapter: function (handle)")

	for _, want := range []string{
		`var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;`,
		`if (!bridge || typeof bridge.selectCard !== 'function') return;`,
		`try { bridge.selectCard(card); } catch (_) { /* swallow */ }`,
	} {
		if !strings.Contains(mountBlock, want) {
			t.Errorf("D37p Context select parity: single-click contextSelectionBridge path missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectModeParity_CoalescesAndCleansSelectionSet(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)
	engineJS := getExplorerAsset(t, srv, d37pContextInteractionEngineJS)
	engineBlock := d37pContextInteractionBoundedBlock(t, engineJS, "Selection-set routing", "Camera-bus registration")
	readyBlock := d37pContextInteractionBoundedBlock(t, js, "onReady: function (readyCtx)", "});")

	for _, want := range []string{
		`window.requestAnimationFrame(publishSelectionSet)`,
		`window.setTimeout(publishSelectionSet, 0)`,
		`window.cancelAnimationFrame(selectionSetFrame)`,
		`try { cy.on('select unselect', 'node', onSelectionChange); }`,
		`try { cy.on('tap', onCoreTap); }`,
		`try { window.addEventListener('keydown', onKeydown); }`,
		`try { window.removeEventListener('keydown', onKeydown); }`,
		`opts.selectionSetAdapter({`,
		`type:          'clear',`,
	} {
		if !strings.Contains(engineBlock, want) {
			t.Errorf("D37p Context select parity: engine-owned coalesced lifecycle missing %q", want)
		}
	}
	for _, want := range []string{
		`var selectionSetCleanup = _registerContextSelectionSetCleanup();`,
		`selectionSetCleanup();`,
	} {
		if !strings.Contains(readyBlock, want) {
			t.Errorf("D37p Context select parity: engine-ready cleanup missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectModeParity_DoesNotReuseLegacyDomMultiSelect(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)

	for _, forbidden := range []string{
		`.gmap-multi-selected`,
		`selectedNodeIds`,
		`dragOverrides`,
		`gmap-lasso-rect`,
		`gmapSelectedNodeIds`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D37p Context select parity: strategic Context must not reuse legacy DOM multi-select via %q", forbidden)
		}
	}
}
