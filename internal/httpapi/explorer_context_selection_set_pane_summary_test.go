package httpapi

import (
	"strings"
	"testing"
)

const (
	d37pContextSelectionSetPaneJS  = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37pContextSelectionSetPaneCSS = "/explorer/assets/css/context-selected-object-pane.css"
)

func TestExplorer_D37pContextSelectionSetPaneSummary_ProviderHookRendersMultiSummary(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextSelectionSetPaneJS)

	for _, want := range []string{
		`notifySelectionSetChanged: function (selectionSet, event)`,
		`_onSelectionSetChanged(selectionSet, event);`,
		`function _renderSelectionSetSummary(selectionSet)`,
		`name.textContent = COPY.multipleSelected;`,
		`label.textContent = 'Selection set';`,
		`ref.textContent = _selectionCountText(selectionSet.items.length);`,
		`_renderSelectionSetOverview(set);`,
		`_renderSelectionSetObjectList(set);`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Context selection-set pane summary: provider/render hook missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectionSetPaneSummary_ContentModelAndFallbacks(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextSelectionSetPaneJS)

	for _, want := range []string{
		`multipleSelected:   'Multiple selected',`,
		`clearSelection:     'Clear selection',`,
		`return 'Selected: ' + count + ' ' + (count === 1 ? 'object' : 'objects');`,
		`function _pluralKindLabel(kind, count)`,
		`'business service': 'Business Services',`,
		`'capability': 'Capabilities',`,
		`'process': 'Processes',`,
		`'decision surface': 'Decision Surfaces',`,
		`'authority profile': 'Authority Profiles',`,
		`return _text(item && (item.kind || item.type), 'Object');`,
		`return _text(item && (item.label || item.name || item.title || item.id), 'Object');`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Context selection-set pane summary: content/fallback contract missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectionSetPaneSummary_CompactListCapsAtFive(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextSelectionSetPaneJS)
	block := d37pContextInteractionBoundedBlock(t, js, "function _renderSelectionSetObjectList(selectionSet)", "function _emptyMessageForNoSelection()")

	for _, want := range []string{
		`var max = 5;`,
		`i < selectionSet.items.length && i < max`,
		`li.textContent = _selectionSetItemLabel(selectionSet.items[i]);`,
		`if (selectionSet.items.length > max)`,
		`overflow.textContent = '+' + (selectionSet.items.length - max) + ' more';`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37p Context selection-set pane summary: compact list cap missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectionSetPaneSummary_PreservesSingleAndEmptyPrecedence(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextSelectionSetPaneJS)
	block := d37pContextInteractionBoundedBlock(t, js, "function _onSelectionSetChanged(selectionSet, event)", "function _onBodyAttributesChanged()")

	for _, want := range []string{
		`_currentSelectionSet = _isMultiSelectionSet(set) ? set : null;`,
		`if (_isMultiSelectionSet(set))`,
		`_renderSelectionSetSummary(set);`,
		`if (!set.items.length || (event && event.type === 'selection_set_cleared'))`,
		`_renderAll(null);`,
		`if (set.items.length === 1)`,
		`if (card) _onSelectionChanged(card);`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37p Context selection-set pane summary: selection-count precedence missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectionSetPaneSummary_ClearUsesSharedRendererCommand(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	paneJS := getExplorerAsset(t, srv, d37pContextSelectionSetPaneJS)
	rendererJS := getExplorerAsset(t, srv, d37pContextInteractionRendererJS)
	engineJS := getExplorerAsset(t, srv, d37pContextInteractionEngineJS)

	paneBlock := d37pContextInteractionBoundedBlock(t, paneJS, "function _clearSelectionSetFromPane()", "function _renderSelectionSetSummary(selectionSet)")
	for _, want := range []string{
		`var renderer = g.contextRenderer;`,
		`renderer.clearSelectionSet();`,
		`data-context-selection-set-clear`,
	} {
		if !strings.Contains(paneJS, want) && !strings.Contains(paneBlock, want) {
			t.Errorf("D37p Context selection-set pane summary: clear action missing shared renderer usage %q", want)
		}
	}
	for _, forbidden := range []string{
		`cy.nodes`,
		`.unselect(`,
		`document.querySelector(".graph-cytoscape-engine-cy-mount")`,
		`bridge.clearSelectionSet`,
	} {
		if strings.Contains(paneBlock, forbidden) {
			t.Errorf("D37p Context selection-set pane summary: pane clear must not bypass renderer/engine via %q", forbidden)
		}
	}

	for _, want := range []string{
		`function clearSelectionSet()`,
		`_engineHandle.clearSelectionSet();`,
		`clearSelectionSet:   clearSelectionSet,`,
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("D37p Context selection-set pane summary: renderer clear command missing %q", want)
		}
	}
	for _, want := range []string{
		`clearSelectionSet: function ()`,
		`_selectionSetClearRequester`,
		`cy.nodes(':selected').unselect();`,
		`clearVisualSelectionSet`,
	} {
		if !strings.Contains(engineJS, want) {
			t.Errorf("D37p Context selection-set pane summary: engine clear command missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectionSetPaneSummary_CssIsScopedAndAccessible(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pContextSelectionSetPaneCSS)
	js := getExplorerAsset(t, srv, d37pContextSelectionSetPaneJS)

	for _, want := range []string{
		`.midas-graph-viewport[data-active-renderer="context"] .context-selection-set-summary`,
		`.midas-graph-viewport[data-active-renderer="context"] .context-selection-set-count`,
		`.midas-graph-viewport[data-active-renderer="context"] .context-selection-set-kind-list`,
		`.midas-graph-viewport[data-active-renderer="context"] .context-selection-set-object-list`,
		`.midas-graph-viewport[data-active-renderer="context"] .context-selection-set-overflow`,
		`.midas-graph-viewport[data-active-renderer="context"] .context-selection-set-clear`,
		`.context-selection-set-clear:focus-visible`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37p Context selection-set pane summary: scoped CSS missing %q", want)
		}
	}
	for _, want := range []string{
		`button.type = 'button';`,
		`button.textContent = COPY.clearSelection;`,
		`_wrapperEl.setAttribute('aria-labelledby', name.id);`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Context selection-set pane summary: accessibility affordance missing %q", want)
		}
	}
}

func TestExplorer_D37pContextSelectionSetPaneSummary_NoBulkActionsOrLegacyReuse(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextSelectionSetPaneJS)

	for _, forbidden := range []string{
		`bulk approve`,
		`bulk revoke`,
		`bulk edit`,
		`bulk evidence`,
		`relationship browser`,
		`.gmap-multi-selected`,
		`selectedNodeIds`,
		`dragOverrides`,
		`gmap-lasso-rect`,
	} {
		if strings.Contains(strings.ToLower(js), strings.ToLower(forbidden)) {
			t.Errorf("D37p Context selection-set pane summary: out-of-scope bulk/legacy construct found %q", forbidden)
		}
	}
}
