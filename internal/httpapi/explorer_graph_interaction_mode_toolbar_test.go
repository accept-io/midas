package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

const (
	d37pInteractionToolbarJS  = "/explorer/assets/js/graph/graph-platform/graph-interaction-mode-toolbar.js"
	d37pInteractionToolbarCSS = "/explorer/assets/css/graph-interaction-mode-toolbar.css"
	d37pInteractionContextCSS = "/explorer/assets/css/context-cytoscape-renderer.css"
)

func TestExplorer_D37pGraphInteractionModeToolbar_AssetsAndMountAreWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	html := d37pInteractionToolbarExplorerHTML(t, srv)

	for _, want := range []string{
		`/explorer/assets/css/graph-interaction-mode-toolbar.css`,
		`/explorer/assets/js/graph/graph-platform/graph-interaction-mode-toolbar.js`,
		`class="graph-interaction-mode-toolbar" data-graph-interaction-mode-toolbar role="toolbar" aria-label="Graph interaction mode" aria-hidden="true" hidden`,
		`class="gmap-mode-rail"`,
		`data-graph-inspector-platform`,
		`data-authority-canvas-edge-tabs`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("D37p interaction mode toolbar: index.html must contain %q", want)
		}
	}
}

func TestExplorer_D37pGraphInteractionModeToolbar_CSSMirrorsLegacyModeRail(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pInteractionToolbarCSS)

	for _, want := range []string{
		`.midas-graph-viewport [data-graph-interaction-mode-toolbar] {`,
		`top: 8px;`,
		`left: 16px;`,
		`z-index: 5;`,
		`display: flex;`,
		`flex-direction: column;`,
		`gap: 4px;`,
		`padding: 4px;`,
		`background: var(--surface-container-high);`,
		`border: 1px solid var(--outline-variant);`,
		`border-radius: 8px;`,
		`box-shadow: 0 4px 12px rgba(0, 0, 0, 0.30);`,
		`.midas-graph-viewport [data-graph-interaction-mode-button] {`,
		`width: 32px;`,
		`height: 32px;`,
		`color: var(--slate-300);`,
		`border-radius: 4px;`,
		`outline: 2px solid var(--primary, #4ea1ff);`,
		`[data-graph-interaction-mode-button][aria-pressed="true"]`,
		`border: 1px solid var(--primary, #4ea1ff);`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37p interaction mode toolbar: CSS must preserve legacy invariant %q", want)
		}
	}
	for _, forbidden := range []string{`data-active-renderer="context"`, `data-active-renderer="authority"`} {
		if strings.Contains(css, forbidden) {
			t.Errorf("D37p interaction mode toolbar: shared CSS must not use graph-specific selector %q", forbidden)
		}
	}
}

func TestExplorer_D37pGraphInteractionModeToolbar_PublicAPIAndConfigDrivenRendering(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pInteractionToolbarJS)

	for _, want := range []string{
		`window.MIDASExplorerGraph.graphInteractionModeToolbar = {`,
		`init: init,`,
		`destroy: destroy,`,
		`register: register,`,
		`unregister: unregister,`,
		`activate: activate,`,
		`deactivate: deactivate,`,
		`setMode: setMode,`,
		`getMode: getMode,`,
		`render: render,`,
		`refresh: refresh,`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p interaction mode toolbar: public API missing %q", want)
		}
	}

	configBlock := d37pInteractionToolbarBoundedBlock(t, js, "function _normaliseConfig(config)", "function _ensureRoot")
	for _, want := range []string{
		`rendererId: _str(config.rendererId),`,
		`lensId: _str(config.lensId),`,
		`enabled: config.enabled,`,
		`defaultMode: defaultMode,`,
		`modes: modes,`,
		`getController: config.getController,`,
		`getCytoscapeHandle: config.getCytoscapeHandle,`,
		`onModeChange: config.onModeChange,`,
		`cleanup: config.cleanup,`,
	} {
		if !strings.Contains(configBlock, want) {
			t.Errorf("D37p interaction mode toolbar: config contract missing %q", want)
		}
	}

	renderBlock := d37pInteractionToolbarBoundedBlock(t, js, "function render()", "function setMode(modeId)")
	for _, want := range []string{
		`for (var i = 0; i < config.modes.length; i++)`,
		`btn.setAttribute(BUTTON_ATTR, '');`,
		`btn.setAttribute(MODE_ATTR, mode.id);`,
		`btn.setAttribute('title', mode.tooltip);`,
		`btn.setAttribute('aria-label', mode.ariaLabel);`,
		`btn.setAttribute('aria-pressed', mode.id === _activeModeId ? 'true' : 'false');`,
		`_appendIcon(btn, mode.icon, mode.label);`,
	} {
		if !strings.Contains(renderBlock, want) {
			t.Errorf("D37p interaction mode toolbar: render must be config-driven via %q", want)
		}
	}
}

func TestExplorer_D37pGraphInteractionModeToolbar_KeyboardLifecycleAndSubstrateOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pInteractionToolbarJS)

	for _, want := range []string{
		`key === 'ArrowDown' || key === 'ArrowRight'`,
		`key === 'ArrowUp' || key === 'ArrowLeft'`,
		`key === 'Enter' || key === ' ' || key === 'Spacebar'`,
		`root.addEventListener('click', _onClick);`,
		`root.addEventListener('keydown', _onKeydown);`,
		`root.removeEventListener('click', _onClick);`,
		`root.removeEventListener('keydown', _onKeydown);`,
		`_viewportObserver.disconnect();`,
		`_bodyObserver.disconnect();`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p interaction mode toolbar: keyboard/lifecycle contract missing %q", want)
		}
	}

	for _, forbidden := range []string{
		`context-interaction-toolbar`,
		`authority-interaction-toolbar`,
		`context-node-inspector`,
		`authority-node-inspector`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D37p interaction mode toolbar: substrate must not register graph customer %q", forbidden)
		}
	}
}

func TestExplorer_D37pGraphInteractionModeToolbar_StrategicContextHidesLegacyRail(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pInteractionContextCSS)

	for _, want := range []string{
		`.midas-graph-viewport[data-active-renderer="context"] .gmap-mode-rail {`,
		`display: none;`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37p interaction mode toolbar: strategic Context CSS must hide legacy mode rail via %q", want)
		}
	}

}

func d37pInteractionToolbarExplorerHTML(t *testing.T, srv *Server) string {
	t.Helper()
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func d37pInteractionToolbarBoundedBlock(t *testing.T, body, start, end string) string {
	t.Helper()
	startIdx := strings.Index(body, start)
	if startIdx < 0 {
		t.Fatalf("D37p interaction mode toolbar: start marker %q missing", start)
	}
	endIdx := strings.Index(body[startIdx:], end)
	if endIdx < 0 {
		t.Fatalf("D37p interaction mode toolbar: end marker %q missing after %q", end, start)
	}
	return body[startIdx : startIdx+endIdx]
}
