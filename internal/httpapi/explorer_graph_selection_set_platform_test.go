package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-graph-selection-set-platform-impl — shared selection-set platform.
//
// These tests pin the reusable, lens-neutral selection-set surface that
// future Multi-Select / Box Select tranches will use. The bridge remains the
// canonical platform owner; no Context-local multi-select state or Cytoscape
// box-selection wiring is introduced here.

const (
	d37pSelectionSetBridgeJS    = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37pSelectionSetPaneJS      = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37pSelectionSetLegacyJS    = "/explorer/assets/js/graph/graph-selection.js"
	d37pSelectionSetContextJS   = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37pSelectionSetAuthorityJS = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37pSelectionSetOverlayJS   = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js"
	d37pSelectionSetStageJS     = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37pSelectionSetConnectorJS = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pSelectionSetModeToolbar = "/explorer/assets/js/graph/graph-platform/graph-interaction-mode-toolbar.js"
)

func TestExplorer_D37pGraphSelectionSet_PlatformAPIExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSelectionSetBridgeJS)

	for _, want := range []string{
		"selectSet:",
		"replaceSelectionSet:",
		"extendSelectionSet:",
		"toggleSelectionInSet:",
		"clearSelectionSet:",
		"getSelectionSet:",
		"subscribeSelectionSet:",
		"function selectSet",
		"function replaceSelectionSet",
		"function extendSelectionSet",
		"function toggleSelectionInSet",
		"function clearSelectionSet",
		"function getSelectionSet",
		"function subscribeSelectionSet",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-graph-selection-set-platform-impl: graphSelectionBridge must expose selection-set API %q", want)
		}
	}
}

func TestExplorer_D37pGraphSelectionSet_NormalisationContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSelectionSetBridgeJS)

	for _, want := range []string{
		"function _normaliseSelectionSet",
		"function _normaliseSelectionSetItem",
		"function _cloneSelectionSet",
		"function _emptySelectionSet",
		"ids:",
		"items:",
		"primaryId:",
		"mode:",
		"selectedAt:",
		"lens:",
		"seen = {}",
		"seen[item.id]",
		"if (!item || !item.id || seen[item.id]) return;",
		"if (!id) return null;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-graph-selection-set-platform-impl: selection-set normalisation must include %q", want)
		}
	}
	if !regexp.MustCompile(`mode\s*=\s*_str\(payload\.mode \|\| opts\.mode \|\| 'replace'\)`).MatchString(js) {
		t.Errorf("D37p-graph-selection-set-platform-impl: selection-set mode must default to replace")
	}
	if !regexp.MustCompile(`primaryId[\s\S]*ids\.length \? ids\[0\] : null`).MatchString(js) {
		t.Errorf("D37p-graph-selection-set-platform-impl: primaryId must be null or one of the normalised ids")
	}
}

func TestExplorer_D37pGraphSelectionSet_EventsAndMutators(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSelectionSetBridgeJS)

	for _, want := range []string{
		"'selection_set_changed'",
		"'selection_set_cleared'",
		"type:         'selection_set_changed'",
		"type:         'selection_set_cleared'",
		"mode:      'replace'",
		"mode:      'extend'",
		"mode:      'toggle'",
		"_notify(event);",
		"_notifySelectionSet(event);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-graph-selection-set-platform-impl: selection-set event/mutator contract must include %q", want)
		}
	}
	if !regexp.MustCompile(`(?s)function extendSelectionSet[\s\S]*current\.items\.concat\(incoming\)`).MatchString(js) {
		t.Errorf("D37p-graph-selection-set-platform-impl: extendSelectionSet must merge with the current set before normalisation de-duplicates")
	}
	if !regexp.MustCompile(`(?s)function toggleSelectionInSet[\s\S]*removed = true[\s\S]*nextItems\.push\(normItem\)`).MatchString(js) {
		t.Errorf("D37p-graph-selection-set-platform-impl: toggleSelectionInSet must remove existing items or add absent items")
	}
}

func TestExplorer_D37pGraphSelectionSet_SubscriberLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSelectionSetBridgeJS)

	for _, want := range []string{
		"var _selectionSetSubscribers",
		"function _notifySelectionSet",
		"_selectionSetSubscribers.slice()",
		"function unsubscribe()",
		"_selectionSetSubscribers.splice",
		"_selectionSetSubscribers.length = 0",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-graph-selection-set-platform-impl: selection-set subscribers must include %q", want)
		}
	}
	if !regexp.MustCompile(`try \{ entry\.handler\(event\); \}\s*catch \(_\) \{ /\* one bad selection-set subscriber must not stop the rest \*/ \}`).MatchString(js) {
		t.Errorf("D37p-graph-selection-set-platform-impl: selection-set subscriber dispatch must isolate failing subscribers")
	}
}

func TestExplorer_D37pGraphSelectionSet_SingleSelectionCompatibilityPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSelectionSetBridgeJS)

	for _, want := range []string{
		"selectCard:",
		"clearSelection:",
		"getSelected:",
		"getCurrentCard:",
		"getCurrentNodeRef:",
		"'selection_changed'",
		"'selection_cleared'",
		"function _normalise",
		"selection: norm",
		"card:      norm.card",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-graph-selection-set-platform-impl: existing single-selection contract must remain intact (%q)", want)
		}
	}
}

func TestExplorer_D37pGraphSelectionSet_SelectedObjectPaneForwardsSafely(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSelectionSetPaneJS)

	for _, want := range []string{
		"'selection_set_changed'",
		"'selection_set_cleared'",
		"_callProvider('notifySelectionSetChanged'",
		"selectionSet: event.selectionSet || null",
		"provider:     _activeProvider()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-graph-selection-set-platform-impl: selected-object pane must safely forward or ignore selection-set events (%q)", want)
		}
	}
	if !regexp.MustCompile(`(?s)function _callProvider[\s\S]*typeof p\[method\] !== 'function'\) return null`).MatchString(js) {
		t.Errorf("D37p-graph-selection-set-platform-impl: providers without selection-set handlers must be ignored safely")
	}
}

func TestExplorer_D37pGraphSelectionSet_LensNeutralAndDoesNotReuseLegacyModel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pSelectionSetBridgeJS)
	legacy := getExplorerAsset(t, srv, d37pSelectionSetLegacyJS)

	for _, banned := range []string{
		"gmap-multi-selected",
		"#gmap-canvas",
		"selectedNodeIds",
		"contextSelectionBridge",
		"contextSelectedObjectPane",
		"contextEvidenceTray",
		"authorityCanvasEdgeTabs",
		"business_service",
		"decision_surface",
		"authority_profile",
		"authority_grant",
		"fail_mode_policy",
		"boxSelectionEnabled",
		"cytoscape",
		"Cytoscape",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-graph-selection-set-platform-impl: shared selection-set bridge must stay lens-neutral and engine-agnostic; found %q", banned)
		}
	}
	if !strings.Contains(legacy, "gmap-multi-selected") {
		t.Fatalf("D37p-graph-selection-set-platform-impl: legacy graph-selection.js fixture no longer contains expected native multi-select marker")
	}
	if strings.Contains(js, "gmap-multi-selected") {
		t.Errorf("D37p-graph-selection-set-platform-impl: shared selection-set platform must not reuse legacy native DOM multi-select")
	}
}

func TestExplorer_D37pGraphSelectionSet_NoOutOfScopeFilesChangedByContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// These assets remain served and are not the owner of the new
	// selection-set state in this tranche.
	for _, asset := range []string{
		d37pSelectionSetContextJS,
		d37pSelectionSetAuthorityJS,
		d37pSelectionSetOverlayJS,
		d37pSelectionSetStageJS,
		d37pSelectionSetConnectorJS,
		d37pSelectionSetModeToolbar,
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37p-graph-selection-set-platform-impl: expected existing asset %q to remain served", asset)
		}
	}

	contextJS := getExplorerAsset(t, srv, d37pSelectionSetContextJS)
	if strings.Contains(contextJS, "replaceSelectionSet(") ||
		strings.Contains(contextJS, "extendSelectionSet(") ||
		strings.Contains(contextJS, "toggleSelectionInSet(") {
		t.Errorf("D37p-graph-selection-set-platform-impl: Context must not gain local selection-set wiring in this tranche")
	}
}
