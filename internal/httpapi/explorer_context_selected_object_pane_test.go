package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37o-impl-6 — Context Selected-Object Pane Foundation tests
//
// Pins the asset presence, DOM shape, CSS scoping, naming discipline,
// public surface, bridge getter restoration, section rendering, mode
// model, open/close + ESC + Focus Mode + lens-switch behaviour,
// multi-signal gating, drawer + tray + Authority + spike preservation,
// bridge wiring, no backend fetch, no legacy DOM scraping, no
// canvas-edge terminology in new Context pane surfaces, no temporary
// renderer identities. Mirrors the asset-text + structural-pin
// patterns established by D37m and D37o-impl-5.

const (
	d37oImpl6PaneAsset           = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37oImpl6PaneCss             = "/explorer/assets/css/context-selected-object-pane.css"
	d37oImpl6BridgeAsset         = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37oImpl6LegacyView          = "/explorer/assets/js/graph/context/context-graph-view.js"
	d37oImpl6AuthorityCanvasEdge = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37oImpl6ViewportHost        = "/explorer/assets/js/graph/graph-viewport.js"
	d37oImpl6DormantSpike        = "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js"
	d37oImpl6EvidenceTrayAsset   = "/explorer/assets/js/graph/context/context-evidence-tray.js"
)

// ── A. Asset presence and load order ─────────────────────────────────

func TestExplorer_D37oImpl6_PaneJsAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	if len(js) == 0 {
		t.Fatal("D37o-impl-6: context-selected-object-pane.js must be served")
	}
}

func TestExplorer_D37oImpl6_PaneCssAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl6PaneCss)
	if len(css) == 0 {
		t.Fatal("D37o-impl-6: context-selected-object-pane.css must be served")
	}
}

// TestExplorer_D37oImpl6_PaneScriptLoadsAfterBridge pins that the
// pane script is loaded strictly after context-selection-bridge.js
// so the bridge's public surface is available when the pane's IIFE
// runs.
func TestExplorer_D37oImpl6_PaneScriptLoadsAfterBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	bridgeIdx := strings.Index(body, "context-selection-bridge.js")
	paneIdx   := strings.Index(body, "context-selected-object-pane.js")
	if bridgeIdx < 0 || paneIdx < 0 {
		t.Fatal("D37o-impl-6: both bridge and pane scripts must appear in index.html")
	}
	if bridgeIdx >= paneIdx {
		t.Errorf("D37o-impl-6: pane script must load AFTER context-selection-bridge.js (bridge idx=%d, pane idx=%d)", bridgeIdx, paneIdx)
	}
}

func TestExplorer_D37oImpl6_PaneCssLinkPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, `href="/explorer/assets/css/context-selected-object-pane.css"`) {
		t.Errorf("D37o-impl-6: index.html must include <link> for context-selected-object-pane.css")
	}
}

// ── B. DOM shape ─────────────────────────────────────────────────────

func TestExplorer_D37oImpl6_WrapperPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, "data-context-selected-object-pane") {
		t.Errorf("D37o-impl-6: wrapper marker `data-context-selected-object-pane` must appear in index.html")
	}
	if !strings.Contains(body, `class="gmap-context-selected-object-pane"`) {
		t.Errorf("D37o-impl-6: wrapper class `gmap-context-selected-object-pane` must appear in index.html")
	}
}

func TestExplorer_D37oImpl6_HeaderBodyFooterPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		"data-context-selected-object-pane-header",
		"data-context-selected-object-pane-body",
		"data-context-selected-object-pane-footer",
		`class="gmap-context-selected-object-pane-header"`,
		`class="gmap-context-selected-object-pane-body"`,
		`class="gmap-context-selected-object-pane-footer"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-6: wrapper must include %q", want)
		}
	}
}

// TestExplorer_D37oImpl6_SectionMarkersPresent pins that the pane
// module source contains the locked section identifiers (rendered
// at runtime as `data-pane-section="..."`).
func TestExplorer_D37oImpl6_SectionMarkersPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, want := range []string{
		`'summary'`,
		`'details'`,
		`'actions'`,
		`'relationships'`,
		`'evidence'`,
		`setAttribute('data-pane-section'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-6: pane module must reference section id %q", want)
		}
	}
}

// TestExplorer_D37oImpl6_WrapperIsInsideViewport pins that the
// wrapper is a child of `.midas-graph-viewport`.
func TestExplorer_D37oImpl6_WrapperIsInsideViewport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	wrapperIdx := strings.Index(body, "data-context-selected-object-pane")
	if wrapperIdx < 0 {
		t.Fatal("D37o-impl-6: wrapper marker missing")
	}
	viewportOpen := strings.LastIndex(body[:wrapperIdx], `class="midas-graph-viewport"`)
	if viewportOpen < 0 {
		t.Fatal("D37o-impl-6: wrapper must be preceded by .midas-graph-viewport opening tag")
	}
	viewportClose := strings.Index(body[wrapperIdx:], `</div><!-- /.midas-graph-viewport -->`)
	if viewportClose < 0 {
		t.Fatal("D37o-impl-6: wrapper must precede the .midas-graph-viewport closing marker")
	}
}

func TestExplorer_D37oImpl6_WrapperStartsHidden(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	idx := strings.Index(body, `data-context-selected-object-pane `)
	if idx < 0 {
		idx = strings.Index(body, `data-context-selected-object-pane>`)
	}
	if idx < 0 {
		t.Fatal("D37o-impl-6: wrapper marker missing")
	}
	tagStart := strings.LastIndex(body[:idx], "<aside")
	tagEnd := strings.Index(body[idx:], ">")
	if tagStart < 0 || tagEnd < 0 {
		t.Fatal("D37o-impl-6: cannot bound wrapper opening tag")
	}
	tag := body[tagStart : idx+tagEnd+1]
	if !strings.Contains(tag, ` hidden`) {
		t.Errorf("D37o-impl-6: wrapper opening tag must declare `hidden` initially — tag: %s", tag)
	}
	if !strings.Contains(tag, `aria-hidden="true"`) {
		t.Errorf("D37o-impl-6: wrapper opening tag must declare aria-hidden=\"true\" initially — tag: %s", tag)
	}
	if !strings.Contains(tag, `data-pane-mode="auto"`) {
		t.Errorf("D37o-impl-6: wrapper opening tag must declare data-pane-mode=\"auto\" initially — tag: %s", tag)
	}
	if !strings.Contains(tag, `role="complementary"`) {
		t.Errorf("D37o-impl-6: wrapper opening tag must declare role=\"complementary\" — tag: %s", tag)
	}
	if !strings.Contains(tag, `aria-label="Selected object"`) {
		t.Errorf("D37o-impl-6: wrapper opening tag must declare aria-label=\"Selected object\" — tag: %s", tag)
	}
}

// ── C. CSS scoping ───────────────────────────────────────────────────

// TestExplorer_D37oImpl6_CssScopedToContext pins that every CSS rule
// in the new pane stylesheet scopes under
// `.midas-graph-viewport[data-active-renderer="context"]` and the
// pane wrapper class — never a global selector.
func TestExplorer_D37oImpl6_CssScopedToContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl6PaneCss)
	cssExec := stripCSSComments(css)

	prefix := `.midas-graph-viewport[data-active-renderer="context"]`
	for i := 0; i < len(cssExec); i++ {
		if cssExec[i] != '{' {
			continue
		}
		start := strings.LastIndexAny(cssExec[:i], "}")
		if start < 0 {
			start = 0
		} else {
			start++
		}
		selector := strings.TrimSpace(cssExec[start:i])
		if selector == "" {
			continue
		}
		if !strings.HasPrefix(selector, prefix) {
			t.Errorf("D37o-impl-6: every pane CSS rule must scope under %s — rogue selector %q", prefix, selector)
		}
	}
}

// ── D. Naming discipline ────────────────────────────────────────────

func TestExplorer_D37oImpl6_NoCanvasEdgeOrTemporaryNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{d37oImpl6PaneAsset, d37oImpl6PaneCss} {
		body := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"canvas-edge",
			"canvas_edge",
			"context-v2",
			"context-strategic",
			"new-context",
			"context-new",
			"context-next",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D37o-impl-6: %s must NOT contain %q (forbidden vocabulary)", asset, banned)
			}
		}
	}
}

// ── E. Public surface ───────────────────────────────────────────────

func TestExplorer_D37oImpl6_PublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	declStart := strings.Index(js, "window.MIDASExplorerGraph.contextSelectedObjectPane = {")
	if declStart < 0 {
		t.Fatal("D37o-impl-6: contextSelectedObjectPane public-surface registration missing")
	}
	declEnd := strings.Index(js[declStart:], "};")
	if declEnd < 0 {
		t.Fatal("D37o-impl-6: cannot bound contextSelectedObjectPane declaration")
	}
	block := js[declStart : declStart+declEnd]

	for _, want := range []string{
		"init:",
		"destroy:",
		"open:",
		"close:",
		"toggle:",
		"isOpen:",
		"setPaneMode:",
		"getPaneMode:",
		"_constants:",
		"PANE_MODES:",
		"SECTION_IDS:",
		"COPY:",
		"STORAGE_KEY:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37o-impl-6: pane public surface must expose %q", want)
		}
	}
}

// ── F. Bridge getter restored ───────────────────────────────────────

func TestExplorer_D37oImpl6_BridgeGetCurrentCardExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)

	for _, want := range []string{
		"function getCurrentCard()",
		"return _currentCard;",
		"getCurrentCard:         getCurrentCard,",
		"_currentCard   = card;",
		"_currentCard   = null;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-6: selection bridge must restore %q", want)
		}
	}
}

// ── G. Section rendering ────────────────────────────────────────────

func TestExplorer_D37oImpl6_SectionRenderersPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, want := range []string{
		"function _renderSummary(card)",
		"function _renderDetails(card)",
		"function _renderActions(card)",
		"function _renderRelationships(card)",
		"function _renderEvidence(card)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-6: pane must define %q", want)
		}
	}
}

// TestExplorer_D37oImpl6_RelationshipsUsesConnectorModel pins that
// Relationships consumes the connector model, never SVG/DOM, never
// Cytoscape, never duplicates edge-kind classification.
func TestExplorer_D37oImpl6_RelationshipsUsesConnectorModel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "contextModels.connector.buildConnectorsFromProjection(projection)") {
		t.Errorf("D37o-impl-6: Relationships must build from connector model")
	}
	if !strings.Contains(js, "VISUAL_CLASS_ORDER") {
		t.Errorf("D37o-impl-6: Relationships must use a locked visual-class order")
	}
}

// TestExplorer_D37oImpl6_ActionsWhitelist pins that the Actions
// section accepts only the three MVP action kinds and drops others
// at render time.
func TestExplorer_D37oImpl6_ActionsWhitelist(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "var ALLOWED_ACTION_KINDS = ['reframe-around-this', 'view-business-service-record', 'view-capability-record']") {
		t.Errorf("D37o-impl-6: pane must define the locked ALLOWED_ACTION_KINDS whitelist")
	}
}

// ── H. Mode model ────────────────────────────────────────────────────

func TestExplorer_D37oImpl6_ModeModel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "var PANE_MODES   = ['auto', 'pinned', 'hidden']") {
		t.Errorf("D37o-impl-6: pane must declare PANE_MODES list ['auto','pinned','hidden']")
	}
	if !strings.Contains(js, "var STORAGE_KEY  = 'midas.context.selectedObjectPane.mode'") {
		t.Errorf("D37o-impl-6: pane must declare STORAGE_KEY 'midas.context.selectedObjectPane.mode'")
	}
	if !strings.Contains(js, "_paneMode = _readStoredMode();") {
		t.Errorf("D37o-impl-6: pane init must read stored mode")
	}
	// localStorage reads + writes guarded by try/catch.
	if !strings.Contains(js, "try {") {
		t.Errorf("D37o-impl-6: pane must use try/catch around localStorage access")
	}
}

// ── I. Open / close ─────────────────────────────────────────────────

func TestExplorer_D37oImpl6_CloseButtonsMarkup(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, want := range []string{
		"function _newCloseButton(suffix)",
		"data-context-selected-object-pane-close-",
		"function _ensureHeaderStaticChrome()",
		"function _ensureFooterStaticChrome()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-6: pane must produce close-button markup via %q", want)
		}
	}
}

func TestExplorer_D37oImpl6_EscapeHandlingPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, `ev.key !== 'Escape'`) {
		t.Errorf("D37o-impl-6: pane must handle Escape key")
	}
	if !strings.Contains(js, "function _wirePaneKeydown()") {
		t.Errorf("D37o-impl-6: pane must wire keydown handler")
	}
}

// ── J. Focus Mode ────────────────────────────────────────────────────

func TestExplorer_D37oImpl6_FocusModeWiring(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// Body observer filters on `class` to catch gmap-focus-mode flips.
	if !strings.Contains(js, "attributeFilter: ['class', 'data-graph-lens']") {
		t.Errorf("D37o-impl-6: body observer must filter on class + data-graph-lens")
	}
	if !strings.Contains(js, "gmap-focus-mode") {
		t.Errorf("D37o-impl-6: pane must reference gmap-focus-mode class")
	}
	if !strings.Contains(js, "_wasOpenBeforeFocusMode") {
		t.Errorf("D37o-impl-6: pane must track pre-Focus-Mode open state for Pinned reopen")
	}
	if !strings.Contains(js, "_paneMode === 'pinned'") {
		t.Errorf("D37o-impl-6: pane must branch on Pinned for Focus-Mode-exit reopen")
	}
}

// ── K. Lens switch ──────────────────────────────────────────────────

func TestExplorer_D37oImpl6_LensSwitchWiring(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "data-graph-lens") {
		t.Errorf("D37o-impl-6: pane must reference body data-graph-lens for lens-switch gating")
	}
	if !strings.Contains(js, "data-active-renderer") {
		t.Errorf("D37o-impl-6: pane must reference viewport data-active-renderer for gating")
	}
	if !strings.Contains(js, "function _isPaneActive()") {
		t.Errorf("D37o-impl-6: pane must define _isPaneActive() gating helper")
	}
}

// ── L. Multi-signal gating ──────────────────────────────────────────

func TestExplorer_D37oImpl6_MultiSignalGating(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// At least two distinct signals: viewport.getActiveRendererId() AND body[data-graph-lens].
	if !strings.Contains(js, "getActiveRendererId") {
		t.Errorf("D37o-impl-6: pane gating must consult viewport.getActiveRendererId()")
	}
	if !strings.Contains(js, `getAttribute('data-graph-lens')`) {
		t.Errorf("D37o-impl-6: pane gating must consult body[data-graph-lens]")
	}
	if !strings.Contains(js, `'.midas-graph-viewport[data-active-renderer="context"]'`) {
		t.Errorf("D37o-impl-6: pane gating must include DOM-attribute fallback selector")
	}
}

// ── M. Drawer preservation ──────────────────────────────────────────

func TestExplorer_D37oImpl6_DrawerPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-details"`,
		`gmap-right-rail`,
		`id="gmap-rail-panel-inspector"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-6: drawer markup %q must remain in index.html", want)
		}
	}

	bridgeJs := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)
	// Inspector setter calls must still live in the bridge.
	for _, want := range []string{
		"insp.setName(name)",
		"insp.setFields(rows)",
		"insp.setGovernance(html)",
		"insp.setActions(",
	} {
		if !strings.Contains(bridgeJs, want) {
			t.Errorf("D37o-impl-6: bridge must continue to call inspector setter %q", want)
		}
	}

	// Pane must NOT call drawer setters.
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	for _, banned := range []string{
		"setName(",
		"setFields(",
		"setGovernance(",
		"setActions(",
		"setInlineActions(",
	} {
		if strings.Contains(paneJs, banned) {
			t.Errorf("D37o-impl-6: pane source must NOT contain drawer setter %q", banned)
		}
	}
}

// ── N. Evidence tray preservation ───────────────────────────────────

func TestExplorer_D37oImpl6_EvidenceTrayPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-evidence-tray"`,
		`id="gmap-evidence-tray-panel"`,
		`data-tab="drift"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-6: evidence tray markup %q must remain", want)
		}
	}

	tray := getExplorerAsset(t, srv, d37oImpl6EvidenceTrayAsset)
	if len(tray) == 0 {
		t.Errorf("D37o-impl-6: context-evidence-tray.js must remain served")
	}

	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	for _, banned := range []string{
		"#gmap-evidence-tray",
		"contextEvidenceTray.notifySelectionChanged",
	} {
		if strings.Contains(paneJs, banned) {
			t.Errorf("D37o-impl-6: pane source must NOT contain %q (bridge owns tray notification)", banned)
		}
	}
}

// ── O. Authority preservation ───────────────────────────────────────

func TestExplorer_D37oImpl6_AuthorityCanvasEdgeUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, "data-authority-canvas-edge-tabs") {
		t.Errorf("D37o-impl-6: Authority canvas-edge wrapper must remain in index.html")
	}

	authJs := getExplorerAsset(t, srv, d37oImpl6AuthorityCanvasEdge)
	if len(authJs) == 0 {
		t.Errorf("D37o-impl-6: authority-canvas-edge-tabs.js must remain served")
	}

	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	for _, banned := range []string{
		"authorityCanvasEdgeTabs",
		"authority-canvas-edge",
		"authorityWorkbench",
	} {
		if strings.Contains(paneJs, banned) {
			t.Errorf("D37o-impl-6: pane source must NOT reference Authority module %q", banned)
		}
	}
}

// ── P. Bridge wiring ────────────────────────────────────────────────

func TestExplorer_D37oImpl6_BridgeAndProjectionSubscriptions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, want := range []string{
		"g.contextSelectionBridge.subscribe",
		"g.contextProjection.subscribe",
		"_bridgeUnsubscribe()",
		"_projectionUnsubscribe()",
		"function destroy()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-6: pane must wire subscription / teardown via %q", want)
		}
	}
}

// TestExplorer_D37oImpl6_PaneReadsBridgeGetter pins that the pane
// reads selection state via `contextSelectionBridge.getCurrentCard()`,
// not via any private legacy global.
func TestExplorer_D37oImpl6_PaneReadsBridgeGetter(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "g.contextSelectionBridge.getCurrentCard()") {
		t.Errorf("D37o-impl-6: pane must read selection via contextSelectionBridge.getCurrentCard()")
	}
	for _, banned := range []string{
		"_lastContextProjection",
		"_lastProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-6: pane must NOT read private legacy projection state %q", banned)
		}
	}
}

// ── Q. No backend fetch ────────────────────────────────────────────

func TestExplorer_D37oImpl6_NoBackendFetch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/graphs/context",
		"/v1/",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-6: pane must NOT perform a backend fetch — found %q", banned)
		}
	}
}

// ── R. No legacy DOM scraping ──────────────────────────────────────

func TestExplorer_D37oImpl6_NoLegacyDomScraping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, banned := range []string{
		"#gmap-canvas",
		"#gmap-svg",
		"#gmap-scene",
		"gmap-canvas",
		"gmap-svg",
		"gmap-scene",
		"cy.elements",
		"cytoscape",
		"Cytoscape",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-6: pane source must NOT contain legacy DOM / graph-engine token %q", banned)
		}
	}
}

// ── S. No spike import ─────────────────────────────────────────────

func TestExplorer_D37oImpl6_NoSpikeImport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if strings.Contains(js, "context-cytoscape-overlay-spike") {
		t.Errorf("D37o-impl-6: pane source must NOT reference the dormant Cytoscape spike")
	}
	// Spike module still served (preservation).
	spike := getExplorerAsset(t, srv, d37oImpl6DormantSpike)
	if len(spike) == 0 {
		t.Errorf("D37o-impl-6: dormant spike module must remain served")
	}
}

// ── T. Locked-copy contract ────────────────────────────────────────
//
// TestExplorer_D37oImpl6_CopyMatchesLockedContract pins the exact
// COPY key/value pairs in the pane source. This is a copy-contract
// alignment test (D37o-impl-6-copy-fix): the assertions match each
// `key: 'value',` line verbatim so a future drift in either the
// key name or the user-visible string fails loudly.
//
// The `{lens}` token in pinnedLensSwitch is kept literal in source
// and substituted at render time by the pane.

func TestExplorer_D37oImpl6_CopyMatchesLockedContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, want := range []string{
		"noSelection:        'Select an object to inspect it.',",
		"noRelationships:    'No relationships for the selected object.',",
		"noDetails:          'No primary details available for this object.',",
		"evidenceDeferral:   'Detailed evidence remains available in the bottom Evidence tab.',",
		"closeButtonLabel:   'Close selected-object pane',",
		"paneAriaLabel:      'Selected object',",
		"pinnedLensSwitch:   'Switched to {lens} — select an object',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-6 copy contract: locked source line %q must appear in pane source", want)
		}
	}
}

// ── U. ARIA / accessibility ────────────────────────────────────────

func TestExplorer_D37oImpl6_AriaAccessibility(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// Static markup carries role + aria-label + body tabindex.
	if !strings.Contains(body, `role="complementary"`) {
		t.Errorf("D37o-impl-6: wrapper must carry role=\"complementary\"")
	}
	if !strings.Contains(body, `aria-label="Selected object"`) {
		t.Errorf("D37o-impl-6: wrapper must carry aria-label=\"Selected object\"")
	}
	if !strings.Contains(body, `tabindex="-1"`) {
		t.Errorf("D37o-impl-6: pane body must carry tabindex=\"-1\"")
	}

	// Header identity emits an <h2>; close buttons use aria-label;
	// action buttons are native <button type="button">.
	if !strings.Contains(js, "document.createElement('h2')") {
		t.Errorf("D37o-impl-6: pane header must render an <h2> for selected-object name")
	}
	if !strings.Contains(js, `setAttribute('aria-label', COPY.closeButtonLabel)`) {
		t.Errorf("D37o-impl-6: close buttons must set aria-label to COPY.closeButtonLabel")
	}
	if !strings.Contains(js, `btn.type = 'button';`) {
		t.Errorf("D37o-impl-6: pane action buttons must be <button type=\"button\">")
	}
}

// ── V. Bootstrap ───────────────────────────────────────────────────

func TestExplorer_D37oImpl6_BootstrapWindowLoadSafetyNet(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, want := range []string{
		"document.readyState === 'loading'",
		"DOMContentLoaded",
		"window.addEventListener('load',",
		"init()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-6: bootstrap must include %q (DOMContentLoaded + window.load safety net)", want)
		}
	}
}

// ── W. Foundation preservation ─────────────────────────────────────

func TestExplorer_D37oImpl6_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	view := getExplorerAsset(t, srv, d37oImpl6LegacyView)
	// D37p-clean-1 retired the dead `renderer.register('context', lensImpl)`
	// dispatcher call and the unreachable `_publishToProjectionHandoff`
	// helper. The live legacy Context entry point is the `contextView`
	// export; the live producer for projections is
	// `contextProjectionProvider` (separate file). The inspector
	// dispatcher namespace remains alive and unchanged.
	if !strings.Contains(view, "window.MIDASExplorerGraph.contextView") {
		t.Errorf("D37o-impl-6: legacy context-graph-view.js must still expose contextView entry points")
	}
	// D37p-clean-2 retired the dead inspector dispatcher; the live
	// inspector frame setters remain reachable via
	// `MIDASExplorerGraph.inspector.set*`.
	if strings.Contains(view, "MIDASExplorerGraph.inspector.register('context', inspectorImpl)") {
		t.Errorf("D37p-clean-2: dead inspector.register('context', inspectorImpl) call must be removed from context-graph-view.js")
	}

	vp := getExplorerAsset(t, srv, d37oImpl6ViewportHost)
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37o-impl-6: GraphViewport native-context baseline adoption must remain")
	}
}
