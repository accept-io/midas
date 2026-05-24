package httpapi

import (
	"strings"
	"testing"
)

const (
	d37pContextPaneActivationPane   = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37pContextPaneActivationBridge = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37pContextPaneActivationRender = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
)

func TestExplorer_D37pContextPaneActivation_AlignsStaleBodyLens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextPaneActivationPane)

	for _, want := range []string{
		"function _alignContextLensForStrategicRenderer()",
		"if (!_isStrategicContextActive()) return false;",
		"document.body.setAttribute('data-graph-lens', 'context');",
		"return _alignContextLensForStrategicRenderer();",
		"_alignContextLensForStrategicRenderer();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-context-pane-activation: pane must align stale/missing body lens via %q", want)
		}
	}
}

func TestExplorer_D37pContextPaneActivation_RefreshesMissedSelectionAfterGateChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextPaneActivationPane)

	bodyHandler := boundedBlock(t, js, "function _onBodyAttributesChanged()", "function _onViewportAttributesChanged()")
	for _, want := range []string{
		"_applyStrategicContextInspectorAttribute();",
		"var paneActive = _isPaneActive();",
		"if (!paneActive)",
		"_refreshAfterMaybeMissedEvents();",
	} {
		if !strings.Contains(bodyHandler, want) {
			t.Errorf("D37p-context-pane-activation: body/viewport gate changes must refresh selected card via %q", want)
		}
	}
	if !strings.Contains(bodyHandler, "if (_paneMode === 'pinned' && _getCurrentCard())") {
		t.Errorf("D37p-context-pane-activation: Focus Mode pinned handling must remain explicit")
	}
}

func TestExplorer_D37pContextPaneActivation_SharedProviderDeliversSelectionFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextPaneActivationPane)

	provider := boundedBlock(t, js, "function _registerWithSharedShell()", "sections: [")
	for _, want := range []string{
		"notifySelectionChanged: function (selection, _event)",
		"selection.lens !== 'context'",
		"_onSelectionChanged(null);",
		"_onSelectionChanged(selection.card || _getCurrentCard());",
	} {
		if !strings.Contains(provider, want) {
			t.Errorf("D37p-context-pane-activation: shared provider must forward Context selection fallback via %q", want)
		}
	}
	if strings.Contains(provider, "intentionally no-ops") {
		t.Errorf("D37p-context-pane-activation: provider notification must no longer be a no-op")
	}
}

func TestExplorer_D37pContextPaneActivation_HiddenModeStillWins(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextPaneActivationPane)

	selectionHandler := boundedBlock(t, js, "function _onSelectionChanged(card)", "function _onBodyAttributesChanged()")
	if !strings.Contains(selectionHandler, "if (_paneMode === 'hidden') return;") {
		t.Errorf("D37p-context-pane-activation: user-hidden pane mode must still block selection auto-open")
	}
	refreshHandler := boundedBlock(t, js, "function _refreshAfterMaybeMissedEvents()", "function _ensureHeaderStaticChrome()")
	for _, want := range []string{
		"if (_paneMode === 'auto' && card)",
		"} else if (_paneMode === 'pinned')",
		"_setOpen(false);",
	} {
		if !strings.Contains(refreshHandler, want) {
			t.Errorf("D37p-context-pane-activation: missed-event refresh must preserve hidden/no-card handling via %q", want)
		}
	}
}

func TestExplorer_D37pContextPaneActivation_SelectionBridgeStillNotifiesPanePath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	bridge := getExplorerAsset(t, srv, d37pContextPaneActivationBridge)

	selectCard := boundedBlock(t, bridge, "function selectCard(card)", "function clearSelection()")
	for _, want := range []string{
		"_currentCard   = card;",
		"if (!_isLegacyContextDrawerSuppressed())",
		"_notify(card);",
		"_pushSelectionToSharedBridge(card);",
	} {
		if !strings.Contains(selectCard, want) {
			t.Errorf("D37p-context-pane-activation: selection bridge must preserve selected-object pane delivery via %q", want)
		}
	}
}

func TestExplorer_D37pContextPaneActivation_RawAndOverlaySelectionSourcesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextPaneActivationRender)

	for _, want := range []string{
		"selectionAdapter: function",
		"bridge.selectCard(card);",
		"_wireCytoscapeSelectionTap",
		"_cy.on('tap', 'node'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-context-pane-activation: raw/overlay Context selection source must remain wired via %q", want)
		}
	}
}

func boundedBlock(t *testing.T, body, start, end string) string {
	t.Helper()
	startIdx := strings.Index(body, start)
	if startIdx < 0 {
		t.Fatalf("D37p-context-pane-activation: start marker %q missing", start)
	}
	endIdx := strings.Index(body[startIdx:], end)
	if endIdx < 0 {
		t.Fatalf("D37p-context-pane-activation: end marker %q missing after %q", end, start)
	}
	return body[startIdx : startIdx+endIdx]
}
