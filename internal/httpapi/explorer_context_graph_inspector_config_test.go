package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

const (
	d37pContextGraphInspectorPaneAsset    = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37pContextGraphInspectorBridgeAsset  = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37pContextGraphInspectorTrayAsset    = "/explorer/assets/js/graph/context/context-evidence-tray.js"
	d40bContextGraphInspectorCSSAsset     = "/explorer/assets/css/graph-inspector.css"
	d37pContextGraphInspectorOverlayAsset = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js"
	d37pContextGraphInspectorStageAsset   = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37pContextGraphInspectorPolicyAsset  = "/explorer/assets/js/graph/graph-platform/graph-footprint-policy.js"
)

func TestExplorer_D37pContextGraphInspectorConfig_RegistersContextCustomer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextGraphInspectorPaneAsset)

	for _, want := range []string{
		`CONTEXT_GRAPH_INSPECTOR_ID = 'context-node-inspector'`,
		`CONTEXT_GRAPH_INSPECTOR_DEFAULT_CONTROL = 'inspector'`,
		`platform.registerInspector(config)`,
		`id: CONTEXT_GRAPH_INSPECTOR_ID`,
		`name: 'Context node inspector'`,
		`rendererId: 'context'`,
		`lensId: 'context'`,
		`defaultControlId: CONTEXT_GRAPH_INSPECTOR_DEFAULT_CONTROL`,
		`getSelectedObject: _getCurrentCard`,
		`_graphInspectorRegistered && typeof platform.activate === 'function'`,
		`platform.activate(CONTEXT_GRAPH_INSPECTOR_ID)`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p Context graph inspector config must contain %q", want)
		}
	}
}

func TestExplorer_D37pContextGraphInspectorConfig_ControlsAreProductControlsOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextGraphInspectorPaneAsset)
	block := d37pContextGraphInspectorBlock(t, js, `controls: [`, `handoffs: {`)

	for _, want := range []string{
		`id: 'inspector'`,
		`label: 'Inspector'`,
		`ariaLabel: 'Inspector'`,
		`id: 'context'`,
		`label: 'Context'`,
		`ariaLabel: 'Context'`,
		`id: 'evidence'`,
		`label: 'Evidence'`,
		`ariaLabel: 'Evidence'`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37p Context graph inspector controls must contain %q", want)
		}
	}
	for _, forbidden := range []string{
		`label: 'Summary'`,
		`label: 'Details'`,
		`label: 'Relationships'`,
		`label: 'Actions'`,
		`id: 'summary'`,
		`id: 'details'`,
		`id: 'relationships'`,
		`id: 'actions'`,
	} {
		if strings.Contains(block, forbidden) {
			t.Errorf("D37p Context graph inspector toolbar must not expose %q", forbidden)
		}
	}
	if strings.Count(block, `label: `) != 3 {
		t.Errorf("D37p Context graph inspector toolbar must expose exactly three labels; got block:\n%s", block)
	}
}

func TestExplorer_D37pContextGraphInspectorConfig_SelectionBridgeDrivesPlatform(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	pane := getExplorerAsset(t, srv, d37pContextGraphInspectorPaneAsset)
	bridge := getExplorerAsset(t, srv, d37pContextGraphInspectorBridgeAsset)

	for _, want := range []string{
		`g.contextSelectionBridge.getCurrentCard()`,
		`_refreshGraphInspectorPlatform(card, !!card && _paneMode !== 'hidden')`,
		`platform.open(activeControl)`,
		`platform.render()`,
		`_setOpen(false);`,
	} {
		if !strings.Contains(pane, want) {
			t.Errorf("D37p Context graph inspector selection path must contain %q", want)
		}
	}
	for _, want := range []string{
		`function getCurrentCard()`,
		`getCurrentCard:         getCurrentCard`,
		`subscribe:              subscribe`,
	} {
		if !strings.Contains(bridge, want) {
			t.Errorf("D37p Context graph inspector must continue to use selection bridge API %q", want)
		}
	}
}

func TestExplorer_D37pContextGraphInspectorConfig_ContentIsCompactAndNotGraphControls(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextGraphInspectorPaneAsset)
	block := d37pContextGraphInspectorBlock(t, js, `function _registerWithGraphInspectorPlatform()`, `function open(sectionId)`)

	for _, want := range []string{
		`function _renderGraphInspectorInspectorContent(card)`,
		`function _renderGraphInspectorContextContent(card)`,
		`function _renderGraphInspectorEvidenceContent(card)`,
		`context-graph-inspector-content`,
		`data-context-graph-inspector-content`,
		`context-graph-inspector-section`,
		`context-graph-inspector-section-title`,
		`Core facts`,
		`context-graph-inspector-facts`,
		`data-context-graph-inspector-facts`,
		`context-graph-inspector-badge`,
		`context-graph-inspector-badge-dot`,
		`context-graph-inspector-value-muted`,
		`context-graph-inspector-value-mono`,
		`context-graph-inspector-note`,
		`function _newContextGraphInspectorCoreFacts(card)`,
		`function _newContextGraphInspectorNote(card)`,
		`Detailed evidence and drift exploration remain in the bottom Evidence tray.`,
		`contextEvidenceTray`,
		`openEvidenceTray`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37p Context graph inspector compact content must contain %q", want)
		}
	}
	for _, forbidden := range []string{
		`Zoom`,
		`Fit`,
		`Focus`,
		`Reframe`,
		`Camera`,
		`Layout`,
		`graph camera`,
		`relationship browser`,
		`gmap-evidence-tray-panel`,
		`gmap-ctx-pane-header`,
		`gmap-ctx-pane-kebab`,
		`View full record`,
	} {
		if strings.Contains(block, forbidden) {
			t.Errorf("D37p Context graph inspector compact config must not hardcode graph/workbench control %q", forbidden)
		}
	}
}

func TestExplorer_D40bContextGraphInspectorCompactContent_FieldMapAndValueHandling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pContextGraphInspectorPaneAsset)
	block := d37pContextGraphInspectorBlock(t, js, `var CONTEXT_GRAPH_INSPECTOR_FACTS_BY_KIND`, `function _renderGraphInspectorContextContent(card)`)

	expectedByKind := map[string][]string{
		`business_service`:         {`Status`, `ID`, `Owner`, `Service type`, `Regulatory`, `Fail mode`},
		`related_business_service`: {`Status`, `ID`, `Relationship`, `Target`},
		`capability`:               {`Status`, `ID`, `Owner`},
		`process`:                  {`Status`, `ID`, `Owner`, `Service`},
		`decision_surface`:         {`Status`, `ID`, `Version`, `Profiles`, `Grants`, `Agents`, `AI bindings`, `Fail mode`},
		`ai_system`:                {`Status`, `ID`, `Vendor`, `Type`, `Version`},
		`ai_system_binding`:        {`Status`, `ID`, `AI system`, `Scope`, `Role`},
		`authority_summary`:        {`Surfaces`, `Profiles`, `Grants`, `Agents`},
		`coverage`:                 {`Status`, `Surfaces`, `Direct AI`, `Scoped AI`, `No binding`, `Has gap`},
	}
	for kind, labels := range expectedByKind {
		if !strings.Contains(block, kind+`: [`) {
			t.Errorf("D40b Context graph inspector facts must define kind %s", kind)
		}
		for _, label := range labels {
			if label == `Status` {
				continue
			}
			if !strings.Contains(block, `label: '`+label+`'`) && !strings.Contains(block, `textContent = '`+label+`'`) {
				t.Errorf("D40b Context graph inspector facts for %s must include label %q", kind, label)
			}
		}
	}

	for _, want := range []string{
		`_appendContextGraphInspectorFact(dl, 'Status', statusNode)`,
		`badge.setAttribute('data-status', normalised)`,
		`card.kind === 'authority_summary'`,
		`card.kind === 'coverage'`,
		`return _newContextGraphInspectorMutedValue(options.missingText || '\u2014')`,
		`if (count && typeof value === 'number' && value === 0) return true`,
		`typeof value === 'number') text = value.toLocaleString()`,
		`typeof value === 'boolean') text = value ? 'Yes' : 'No'`,
		`span.className = 'context-graph-inspector-value-mono'`,
		`span.className = 'context-graph-inspector-value-muted'`,
		`p.textContent = _truncateContextGraphInspectorNote(description)`,
		`if (s.length <= 140) return s`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D40b Context graph inspector compact value handling missing %q", want)
		}
	}

	renderBlock := d37pContextGraphInspectorBlock(t, js, `function _renderGraphInspectorInspectorContent(card)`, `function _newContextGraphInspectorCoreFacts(card)`)
	for _, forbidden := range []string{
		`_buildNodeSummary(card)`,
		`_newGraphInspectorParagraph(_buildNodeSummary(card))`,
		`_newGraphInspectorFactList(_keyFacts(card))`,
		`undefined`,
		`null`,
	} {
		if strings.Contains(renderBlock, forbidden) {
			t.Errorf("D40b Context graph inspector compact render must not contain %q", forbidden)
		}
	}
}

func TestExplorer_D40bContextGraphInspectorCompactContent_CSSAndPlatformGeometry(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d40bContextGraphInspectorCSSAsset)

	for _, want := range []string{
		`.midas-graph-viewport .graph-inspector-platform {`,
		`top: 8px;`,
		`right: 0;`,
		`bottom: 64px;`,
		`pointer-events: none;`,
		`.midas-graph-viewport .graph-inspector-toolbar {`,
		`width: 40px;`,
		`.midas-graph-viewport .graph-inspector-panel {`,
		`right: 40px;`,
		`width: 300px;`,
		`height: 100%;`,
		`z-index: 8;`,
		`.midas-graph-viewport .graph-inspector-panel-header {`,
		`.midas-graph-viewport .graph-inspector-panel-body {`,
		`.midas-graph-viewport .graph-inspector-panel-footer {`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D40b Context graph inspector platform geometry/chrome invariant missing %q", want)
		}
	}

	for _, want := range []string{
		`D40b - Context inspector content`,
		`.context-graph-inspector-content[data-context-graph-inspector-content="inspector"]`,
		`padding: var(--layout-card-padding);`,
		`gap: var(--layout-element-gap);`,
		`.context-graph-inspector-section`,
		`.context-graph-inspector-section-title`,
		`.context-graph-inspector-facts`,
		`grid-template-columns: 90px 1fr;`,
		`column-gap: var(--layout-element-gap);`,
		`.context-graph-inspector-badge`,
		`.context-graph-inspector-badge[data-status="active"]`,
		`.context-graph-inspector-note`,
		`padding: var(--layout-inline-gap) var(--layout-element-gap);`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D40b Context graph inspector compact CSS missing %q", want)
		}
	}
}

func TestExplorer_D37pContextGraphInspectorConfig_EvidenceTrayHasMinimalHandoffAPI(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tray := getExplorerAsset(t, srv, d37pContextGraphInspectorTrayAsset)

	for _, want := range []string{
		`function openEvidenceTray()`,
		`gmapEvidenceTrayExpanded = true`,
		`gmapEvidenceTrayActiveTab = 'evidence'`,
		`applyGmapEvidenceTrayState()`,
		`openEvidenceTray:        openEvidenceTray`,
	} {
		if !strings.Contains(tray, want) {
			t.Errorf("D37p Context graph inspector Evidence handoff API must contain %q", want)
		}
	}
}

func TestExplorer_D37pContextGraphInspectorConfig_ContextRoutesShareSameConfig(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	html := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	js := getExplorerAsset(t, srv, d37pContextGraphInspectorPaneAsset)

	if !strings.Contains(html, `/explorer/assets/js/graph/graph-platform/graph-inspector-platform.js`) {
		t.Fatalf("D37p Context graph inspector requires graph inspector platform script")
	}
	if !strings.Contains(html, `/explorer/assets/js/graph/context/context-selected-object-pane.js`) {
		t.Fatalf("D37p Context graph inspector requires context pane script")
	}
	if !strings.Contains(js, `rendererId: 'context'`) || !strings.Contains(js, `lensId: 'context'`) {
		t.Fatalf("D37p Context graph inspector config must gate by Context renderer/lens, not route-specific overlay state")
	}
	if strings.Contains(js, `contextOverlay`) || strings.Contains(js, `html-cards`) {
		t.Errorf("D37p Context graph inspector config must not fork raw and HTML overlay Context routes")
	}
}

func TestExplorer_D37pContextGraphInspectorConfig_DisallowedPlatformAndProjectionFilesUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, asset := range []string{
		d37pContextGraphInspectorOverlayAsset,
		d37pContextGraphInspectorStageAsset,
		d37pContextGraphInspectorPolicyAsset,
	} {
		body := getExplorerAsset(t, srv, asset)
		if strings.Contains(body, `context-node-inspector`) ||
			strings.Contains(body, `Context node inspector`) {
			t.Errorf("D37p Context graph inspector config leaked into disallowed asset %s", asset)
		}
	}
}

func d37pContextGraphInspectorBlock(t *testing.T, body, start, end string) string {
	t.Helper()
	startIdx := strings.Index(body, start)
	if startIdx < 0 {
		t.Fatalf("start marker %q not found", start)
	}
	rest := body[startIdx:]
	endIdx := strings.Index(rest, end)
	if endIdx < 0 {
		t.Fatalf("end marker %q not found after %q", end, start)
	}
	return rest[:endIdx]
}
