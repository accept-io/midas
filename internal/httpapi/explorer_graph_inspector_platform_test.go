package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

const (
	d37pGraphInspectorPlatformJS  = "/explorer/assets/js/graph/graph-platform/graph-inspector-platform.js"
	d37pGraphInspectorPlatformCSS = "/explorer/assets/css/graph-inspector.css"
)

func TestExplorer_D37pGraphInspectorPlatform_AssetsAndMountAreWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	html := d37pGraphInspectorExplorerHTML(t, srv)

	for _, want := range []string{
		`/explorer/assets/css/graph-inspector.css`,
		`/explorer/assets/js/graph/graph-platform/graph-inspector-platform.js`,
		`class="graph-inspector-platform" data-graph-inspector-platform aria-hidden="true" hidden`,
		`class="graph-inspector-toolbar" data-graph-inspector-toolbar role="toolbar"`,
		`class="graph-inspector-panel" data-graph-inspector-panel role="complementary"`,
		`data-graph-inspector-panel-header`,
		`data-graph-inspector-panel-body`,
		`data-graph-inspector-panel-footer`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("D37p graph inspector platform: index.html must contain %q", want)
		}
	}
	if !strings.Contains(html, `data-authority-canvas-edge-tabs`) {
		t.Errorf("D37p graph inspector platform: Authority mount must remain present")
	}
	if !strings.Contains(html, `data-context-selected-object-pane`) {
		t.Errorf("D37p graph inspector platform: Context pane mount must remain present")
	}
}

func TestExplorer_D37pGraphInspectorPlatform_CSSLocksAuthorityDerivedChrome(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pGraphInspectorPlatformCSS)

	for _, want := range []string{
		`.midas-graph-viewport .graph-inspector-platform {`,
		`top: 8px;`,
		`right: 0;`,
		`bottom: 64px;`,
		`pointer-events: none;`,
		`.midas-graph-viewport .graph-inspector-toolbar {`,
		`width: 40px;`,
		`gap: 4px;`,
		`padding: 6px 4px;`,
		`z-index: 6;`,
		`pointer-events: auto;`,
		`.midas-graph-viewport .graph-inspector-control {`,
		`width: 32px;`,
		`height: 32px;`,
		`.midas-graph-viewport .graph-inspector-panel {`,
		`right: 40px;`,
		`width: 300px;`,
		`z-index: 8;`,
		`font-family: var(--font-display, Inter, 'Segoe UI', system-ui, sans-serif);`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37p graph inspector platform: shared CSS must preserve invariant %q", want)
		}
	}
	if strings.Contains(css, `data-active-renderer="authority"`) ||
		strings.Contains(css, `data-active-renderer="context"`) {
		t.Errorf("D37p graph inspector platform: shared CSS must not be scoped to Authority or Context")
	}
}

func TestExplorer_D37pGraphInspectorPlatform_PublicAPIAndConfigContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pGraphInspectorPlatformJS)

	for _, want := range []string{
		`window.MIDASExplorerGraph.graphInspectorPlatform = {`,
		`init: init,`,
		`destroy: destroy,`,
		`registerInspector: registerInspector,`,
		`unregisterInspector: unregisterInspector,`,
		`activate: activate,`,
		`getActiveInspectorId: getActiveInspectorId,`,
		`open: open,`,
		`close: close,`,
		`toggle: toggle,`,
		`setActiveControl: setActiveControl,`,
		`getActiveControl: getActiveControl,`,
		`render: render,`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p graph inspector platform: public API missing %q", want)
		}
	}

	configBlock := d37pGraphInspectorBoundedBlock(t, js, "function _normaliseConfig(config)", "function _controlById")
	for _, want := range []string{
		`id: id,`,
		`name: _str(config.name || id),`,
		`rendererId: _str(config.rendererId),`,
		`lensId: _str(config.lensId),`,
		`enabled: config.enabled,`,
		`defaultControlId: defaultControlId,`,
		`getSelectedObject: config.getSelectedObject,`,
		`getPanelTitle: config.getPanelTitle,`,
		`getPanelSubtitle: config.getPanelSubtitle,`,
		`controls: controls,`,
		`onControlSelect: config.onControlSelect,`,
		`onClose: config.onClose,`,
		`handoffs: _isPlainObject(config.handoffs) ? config.handoffs : {},`,
	} {
		if !strings.Contains(configBlock, want) {
			t.Errorf("D37p graph inspector platform: config contract missing %q", want)
		}
	}
}

func TestExplorer_D37pGraphInspectorPlatform_ToolbarAndPanelAreConfigDriven(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pGraphInspectorPlatformJS)

	toolbarBlock := d37pGraphInspectorBoundedBlock(t, js, "function _renderToolbar(config, selected, ctx)", "function _renderEmpty")
	for _, want := range []string{
		`for (var i = 0; i < config.controls.length; i++)`,
		`var control = config.controls[i];`,
		`btn.setAttribute('data-graph-inspector-control', control.id);`,
		`btn.setAttribute('title', control.tooltip);`,
		`btn.setAttribute('aria-label', control.ariaLabel);`,
		`btn.setAttribute('aria-pressed', control.id === _activeControlId ? 'true' : 'false');`,
		`_appendIcon(btn, control.icon, control.label);`,
	} {
		if !strings.Contains(toolbarBlock, want) {
			t.Errorf("D37p graph inspector platform: toolbar rendering must be config-driven via %q", want)
		}
	}

	panelBlock := d37pGraphInspectorBoundedBlock(t, js, "function _renderPanel(config, control, selected, ctx)", "function _onToolbarClick")
	for _, want := range []string{
		`config.getPanelTitle`,
		`config.getPanelSubtitle`,
		`control.render`,
		`_bodyEl.appendChild(rendered);`,
		`_bodyEl.innerHTML = rendered;`,
	} {
		if !strings.Contains(panelBlock, want) {
			t.Errorf("D37p graph inspector platform: panel rendering must delegate via %q", want)
		}
	}
	if !strings.Contains(js, `control.emptyState`) {
		t.Errorf("D37p graph inspector platform: panel empty state must delegate to control.emptyState")
	}

	for _, forbidden := range []string{`Details`, `Authority`, `Inspector`, `Context`, `Evidence`} {
		if strings.Contains(toolbarBlock, forbidden) || strings.Contains(panelBlock, forbidden) {
			t.Errorf("D37p graph inspector platform: substrate must not hardcode graph/product control %q", forbidden)
		}
	}
}

func TestExplorer_D37pGraphInspectorPlatform_KeyboardAndLifecycleAreGeneric(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pGraphInspectorPlatformJS)

	for _, want := range []string{
		`key === 'ArrowDown' || key === 'ArrowUp'`,
		`key === 'Enter' || key === ' ' || key === 'Spacebar'`,
		`if (!ev || ev.key !== 'Escape') return;`,
		`_toolbarEl.addEventListener('click', _onToolbarClick);`,
		`_toolbarEl.addEventListener('keydown', _onToolbarKeydown);`,
		`_panelEl.addEventListener('keydown', _onPanelKeydown);`,
		`_toolbarEl.removeEventListener('click', _onToolbarClick);`,
		`_toolbarEl.removeEventListener('keydown', _onToolbarKeydown);`,
		`_panelEl.removeEventListener('keydown', _onPanelKeydown);`,
		`_viewportObserver.disconnect();`,
		`_bodyObserver.disconnect();`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p graph inspector platform: keyboard/lifecycle contract missing %q", want)
		}
	}
}

func TestExplorer_D37pGraphInspectorPlatform_RemainsSubstrateOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pGraphInspectorPlatformJS)
	html := d37pGraphInspectorExplorerHTML(t, srv)

	for _, forbidden := range []string{
		`context-node-inspector`,
		`authority-node-inspector`,
		`data-active-renderer="authority"`,
		`data-active-renderer="context"`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D37p graph inspector platform: substrate JS must not register graph customer/gate %q", forbidden)
		}
	}

	platformScriptPos := strings.Index(html, `/explorer/assets/js/graph/graph-platform/graph-inspector-platform.js`)
	contextPanePos := strings.Index(html, `/explorer/assets/js/graph/context/context-selected-object-pane.js`)
	authorityTabsPos := strings.Index(html, `/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js`)
	if platformScriptPos < 0 || contextPanePos < 0 || authorityTabsPos < 0 {
		t.Fatalf("D37p graph inspector platform: expected platform, Context, and Authority scripts in index")
	}
	if !(platformScriptPos < contextPanePos && platformScriptPos < authorityTabsPos) {
		t.Errorf("D37p graph inspector platform: platform script must load before future graph-specific registrations")
	}
}

func TestExplorer_D37pGraphInspectorPlatform_NoOverlayOrGraphControlCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pGraphInspectorPlatformJS)
	overlay := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")

	if strings.Contains(overlay, "graphInspectorPlatform") ||
		strings.Contains(overlay, "data-graph-inspector-platform") {
		t.Errorf("D37p graph inspector platform: overlay projection file must not couple to inspector platform")
	}

	for _, forbidden := range []string{`'Zoom'`, `'Fit'`, `'Focus'`, `'Reframe'`, `'Camera'`, `'Layout'`, `"Zoom"`, `"Fit"`, `"Focus"`, `"Reframe"`, `"Camera"`, `"Layout"`} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D37p graph inspector platform: graph control label %q must not be hardcoded", forbidden)
		}
	}
}

func d37pGraphInspectorExplorerHTML(t *testing.T, srv *Server) string {
	t.Helper()
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func d37pGraphInspectorBoundedBlock(t *testing.T, body, start, end string) string {
	t.Helper()
	startIdx := strings.Index(body, start)
	if startIdx < 0 {
		t.Fatalf("D37p graph inspector platform: start marker %q missing", start)
	}
	endIdx := strings.Index(body[startIdx:], end)
	if endIdx < 0 {
		t.Fatalf("D37p graph inspector platform: end marker %q missing after %q", end, start)
	}
	return body[startIdx : startIdx+endIdx]
}
