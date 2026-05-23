package httpapi

import (
	"net/http"
	"regexp"
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

// ---------------------------------------------------------------------------
// D37am-context-tabs-config-impl — Context enabled on the global graph-
// native tab platform.
//
// Context becomes the second formal consumer of the GraphTabConfig
// contract commissioned by D37ak (Authority was first). This tranche
// is declarative-only: it adds a `CONTEXT_TAB_CONFIG` constant and a
// `tabs: CONTEXT_TAB_CONFIG` field on the existing Context provider
// literal. Render flow, section IDs, copy, selection bridge, pane
// mode, ESC handling, and focus-mode behaviour remain unchanged.
//
// Invariants pinned by this section:
//
//   • CONTEXT_TAB_CONFIG declared as a module-level constant with
//     `enabled: true` and `defaultTab: 'summary'`.
//   • Exactly five tab items: summary / details / relationships /
//     actions / evidence — labels are Title-cased.
//   • Each item has id / label / provider / supports.
//   • summary / relationships / actions / evidence carry the `['*']`
//     wildcard so they render for every selected kind.
//   • The details tab enumerates the authoritative Context card-kind
//     list owned by `context-card-model.js` NODE_KINDS — the test
//     cross-checks both lists.
//   • Provider exposes `tabs: CONTEXT_TAB_CONFIG`.
//   • Provider's existing `sections` array remains intact.
//   • Public surface exposes `_CONTEXT_TAB_CONFIG` for diagnostics
//     (mirrors Authority's `_AUTHORITY_TAB_CONFIG` precedent).
//   • All existing renderers / copy / pane behaviour preserved.
//   • Legacy right-side letterbox / drawer / inspector modules
//     untouched.
//   • Authority module untouched.
//   • Engine / viewport / camera files untouched.

const (
	d37amCardModelAsset = "/explorer/assets/js/graph/context/context-card-model.js"
)

// ── X. CONTEXT_TAB_CONFIG declaration ──────────────────────────────

func TestExplorer_D37am_Context_TabConfigDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "var CONTEXT_TAB_CONFIG = {") {
		t.Errorf("D37am-context-tabs-config-impl: CONTEXT_TAB_CONFIG must be declared as a module-level constant")
	}
	for _, want := range []string{
		"enabled:    true,",
		"defaultTab: 'summary',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37am-context-tabs-config-impl: CONTEXT_TAB_CONFIG must declare %q", want)
		}
	}
}

// ── Y. Tab item shape, ids, labels ─────────────────────────────────

func TestExplorer_D37am_Context_TabItemsShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	startIdx := strings.Index(js, "var CONTEXT_TAB_CONFIG = {")
	if startIdx < 0 {
		t.Fatal("D37am-context-tabs-config-impl: CONTEXT_TAB_CONFIG must be declared")
	}
	tail := js[startIdx:]
	// The config sits before the provider literal — use a robust
	// delimiter that follows the config block.
	endMarker := "// ── Module state (UI only)"
	endIdx := strings.Index(tail, endMarker)
	if endIdx < 0 {
		t.Fatalf("D37am-context-tabs-config-impl: CONTEXT_TAB_CONFIG block must precede the module-state section")
	}
	cfg := tail[:endIdx]

	for _, want := range []string{
		"id:       'summary',",
		"id:       'details',",
		"id:       'relationships',",
		"id:       'actions',",
		"id:       'evidence',",
		"label:    'Summary',",
		"label:    'Details',",
		"label:    'Relationships',",
		"label:    'Actions',",
		"label:    'Evidence',",
		"provider: 'context.summary',",
		"provider: 'context.details',",
		"provider: 'context.relationships',",
		"provider: 'context.actions',",
		"provider: 'context.evidence',",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("D37am-context-tabs-config-impl: CONTEXT_TAB_CONFIG.items must declare %q", want)
		}
	}
	// Exactly five tab items.
	if got := strings.Count(cfg, "supports:"); got != 5 {
		t.Errorf("D37am-context-tabs-config-impl: CONTEXT_TAB_CONFIG must declare exactly 5 tab items (one supports list per item); got %d", got)
	}
}

// ── Z. Supports wildcards for generic tabs ─────────────────────────

func TestExplorer_D37am_Context_GenericTabsUseWildcard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	startIdx := strings.Index(js, "var CONTEXT_TAB_CONFIG = {")
	if startIdx < 0 {
		t.Fatal("D37am-context-tabs-config-impl: CONTEXT_TAB_CONFIG must be declared")
	}
	endIdx := strings.Index(js[startIdx:], "// ── Module state (UI only)")
	if endIdx < 0 {
		t.Fatalf("D37am-context-tabs-config-impl: CONTEXT_TAB_CONFIG block must be well-formed")
	}
	cfg := js[startIdx : startIdx+endIdx]

	// summary / relationships / actions / evidence each carry the
	// universal wildcard. Four occurrences of the wildcard literal.
	if got := strings.Count(cfg, "supports: ['*'],"); got != 4 {
		t.Errorf("D37am-context-tabs-config-impl: 4 generic tabs must each declare `supports: ['*']`; got %d wildcard literals", got)
	}
}

// ── AA. Details tab supports the authoritative Context kind list ───

func TestExplorer_D37am_Context_DetailsSupportsCardModelKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	modelJs := getExplorerAsset(t, srv, d37amCardModelAsset)

	// Pull the details tab's supports list out of the pane config.
	detailsIdx := strings.Index(paneJs, "id:       'details',")
	if detailsIdx < 0 {
		t.Fatal("D37am-context-tabs-config-impl: details tab item must be declared")
	}
	tail := paneJs[detailsIdx:]
	supportsIdx := strings.Index(tail, "supports: [")
	if supportsIdx < 0 {
		t.Fatal("D37am-context-tabs-config-impl: details tab must declare a supports list")
	}
	closeIdx := strings.Index(tail[supportsIdx:], "],")
	if closeIdx < 0 {
		t.Fatal("D37am-context-tabs-config-impl: details supports list must close with `],`")
	}
	supportsBlock := tail[supportsIdx : supportsIdx+closeIdx]

	// Authoritative kind list from context-card-model.js NODE_KINDS.
	expectedKinds := []string{
		"business_service",
		"related_business_service",
		"capability",
		"process",
		"decision_surface",
		"ai_system",
		"ai_system_binding",
		"authority_summary",
		"coverage",
	}
	for _, kind := range expectedKinds {
		// Each kind must be quoted inside the supports list.
		needle := "'" + kind + "',"
		if !strings.Contains(supportsBlock, needle) {
			t.Errorf("D37am-context-tabs-config-impl: details.supports must include %q", needle)
		}
	}
	// Cross-check that every expected kind is also declared in the
	// authoritative card-model NODE_KINDS frozen list. If one of the
	// two lists drifts in a future change, this assertion fires.
	for _, kind := range expectedKinds {
		needle := "'" + kind + "',"
		if !strings.Contains(modelJs, needle) {
			t.Errorf("D37am-context-tabs-config-impl: expected kind %q must also appear in context-card-model.js NODE_KINDS (lockstep alignment)", needle)
		}
	}
	// And the details supports list must contain exactly the 9 kinds
	// — no extras, no omissions.
	if got := strings.Count(supportsBlock, "'"); got != 2*len(expectedKinds) {
		t.Errorf("D37am-context-tabs-config-impl: details.supports must contain exactly %d kind literals (2 quote chars per kind); got %d quote chars", 2*len(expectedKinds), got)
	}
}

// ── BB. Provider exposes the formal tabs field ─────────────────────

func TestExplorer_D37am_Context_ProviderExposesTabsField(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// Find the provider literal start (registered as the 'context'
	// lens provider in _registerWithSharedShell).
	startIdx := strings.Index(js, "id:    'context',")
	if startIdx < 0 {
		t.Fatal("D37am-context-tabs-config-impl: Context provider literal must declare id 'context'")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail, "shell.registerLensProvider('context'")
	if endIdx < 0 {
		t.Fatalf("D37am-context-tabs-config-impl: provider literal must precede registerLensProvider call")
	}
	providerBlock := tail[:endIdx]

	if !strings.Contains(providerBlock, "tabs: CONTEXT_TAB_CONFIG") {
		t.Errorf("D37am-context-tabs-config-impl: Context provider must declare `tabs: CONTEXT_TAB_CONFIG`")
	}
	// The legacy `sections` field is preserved for backwards-compat.
	if !strings.Contains(providerBlock, "sections:") {
		t.Errorf("D37am-context-tabs-config-impl: provider must retain its existing `sections` field (D37o-impl-6 invariant)")
	}
}

// ── CC. Diagnostic export ──────────────────────────────────────────

func TestExplorer_D37am_Context_DiagnosticExportsTabConfig(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "_CONTEXT_TAB_CONFIG: CONTEXT_TAB_CONFIG,") {
		t.Errorf("D37am-context-tabs-config-impl: contextSelectedObjectPane public surface must export _CONTEXT_TAB_CONFIG (Authority precedent)")
	}
}

// ── DD. Existing Context behaviour preserved ───────────────────────

func TestExplorer_D37am_Context_ExistingBehaviourPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// All five section renderers remain.
	for _, want := range []string{
		"function _renderSummary",
		"function _renderDetails",
		"function _renderActions",
		"function _renderRelationships",
		"function _renderEvidence",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37am-context-tabs-config-impl: section renderer %q must remain unchanged", want)
		}
	}
	// Locked copy constants remain verbatim.
	for _, want := range []string{
		"noSelection:        'Select an object to inspect it.',",
		"noRelationships:    'No relationships for the selected object.',",
		"noDetails:          'No primary details available for this object.',",
		"evidenceDeferral:   'Detailed evidence remains available in the bottom Evidence tab.',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37am-context-tabs-config-impl: locked copy %q must remain unchanged", want)
		}
	}
	// SECTION_IDS array unchanged.
	if !strings.Contains(js, "var SECTION_IDS  = ['summary', 'details', 'actions', 'relationships', 'evidence'];") {
		t.Errorf("D37am-context-tabs-config-impl: SECTION_IDS module constant must remain unchanged")
	}
	// PANE_MODES unchanged.
	if !strings.Contains(js, "var PANE_MODES   = ['auto', 'pinned', 'hidden'];") {
		t.Errorf("D37am-context-tabs-config-impl: PANE_MODES module constant must remain unchanged")
	}
	// STORAGE_KEY unchanged.
	if !strings.Contains(js, "var STORAGE_KEY  = 'midas.context.selectedObjectPane.mode';") {
		t.Errorf("D37am-context-tabs-config-impl: STORAGE_KEY must remain unchanged (Context's per-lens key separate from shell's STORAGE_KEY)")
	}
}

// ── EE. Public surface format preserved ────────────────────────────

func TestExplorer_D37am_Context_PublicSurfacePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// Existing D37o-impl-6 public surface entries remain in their
	// pinned aligned format.
	for _, want := range []string{
		"window.MIDASExplorerGraph.contextSelectedObjectPane",
		"init:        init",
		"destroy:     destroy",
		"open:        open",
		"close:       close",
		"toggle:      toggle",
		"isOpen:      isOpen",
		"setPaneMode: setPaneMode",
		"getPaneMode: getPaneMode",
		"_constants:",
		"PANE_MODES:  PANE_MODES.slice()",
		"SECTION_IDS: SECTION_IDS.slice()",
		"COPY:        COPY",
		"STORAGE_KEY: STORAGE_KEY",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37am-context-tabs-config-impl: D37o-impl-6 public surface entry %q must remain unchanged", want)
		}
	}
}

// ── FF. Legacy right-side letterbox untouched ──────────────────────

func TestExplorer_D37am_Context_LegacyLetterboxUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-details"`,
		`class="governance-map-details gmap-right-rail"`,
		`data-rail-tab="inspector"`,
		`data-rail-tab="evidence"`,
		`data-rail-tab="config"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37am-context-tabs-config-impl: legacy right-rail markup %q must remain (this tranche is declarative-only)", want)
		}
	}
	for _, asset := range []string{
		"/explorer/assets/js/graph/graph-drawer.js",
		"/explorer/assets/js/graph/graph-inspector.js",
		"/explorer/assets/js/graph/context/context-graph-inspector.js",
		"/explorer/assets/js/graph/context/context-selection-bridge.js",
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37am-context-tabs-config-impl: %q must remain served", asset)
		}
	}
}

// ── GG. Authority untouched ────────────────────────────────────────

func TestExplorer_D37am_Authority_Untouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJs := getExplorerAsset(t, srv, d37oImpl6AuthorityCanvasEdge)

	// Authority's AUTHORITY_TAB_CONFIG (D37ak) remains intact.
	if !strings.Contains(authJs, "var AUTHORITY_TAB_CONFIG = {") {
		t.Errorf("D37am-context-tabs-config-impl: Authority's AUTHORITY_TAB_CONFIG must remain unchanged (D37ak invariant)")
	}
	if !strings.Contains(authJs, "tabs: AUTHORITY_TAB_CONFIG") {
		t.Errorf("D37am-context-tabs-config-impl: Authority provider must still expose `tabs: AUTHORITY_TAB_CONFIG`")
	}
	// Authority must NOT have absorbed Context's tab config.
	for _, banned := range []string{
		"CONTEXT_TAB_CONFIG",
		"_CONTEXT_TAB_CONFIG",
		"context.summary",
		"context.details",
		"context.relationships",
		"context.actions",
		"context.evidence",
	} {
		if strings.Contains(authJs, banned) {
			t.Errorf("D37am-context-tabs-config-impl: Authority module must not reference Context tab tokens (%q)", banned)
		}
	}
}

// ── HH. Engine / viewport / camera files untouched ─────────────────

func TestExplorer_D37am_EngineSurfaceUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Engine markers from D37u / D37ah / D37ai / D37ak must remain.
	engineJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	for _, want := range []string{
		"function _runFitPipeline(source, phase)",
		"function _onContainerResize()",
		"function _armInitialSafetyCap()",
		"DIAG_ENGINE_INITIAL_REVEAL",
		"INITIAL_FIT_SAFETY_MS",
	} {
		if !strings.Contains(engineJs, want) {
			t.Errorf("D37am-context-tabs-config-impl: engine marker %q must remain (this tranche does not modify the engine)", want)
		}
	}
	// Engine, viewport, camera files must not reference tab-contract
	// or CONTEXT_TAB_CONFIG.
	for _, asset := range []string{
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js",
		"/explorer/assets/js/graph/graph-platform/graph-stage.js",
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/graph-camera.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"CONTEXT_TAB_CONFIG",
			"AUTHORITY_TAB_CONFIG",
			"graphSelectedObjectPane.getTabs",
			"graphSelectedObjectPane.setActiveTab",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37am-context-tabs-config-impl: %q must not contain tab-contract reference %q", asset, banned)
			}
		}
	}
}

// ── II. Shared shell stays generic (no Context strings introduced) ──

func TestExplorer_D37am_SharedShellStaysGeneric(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js")

	// The shared shell must not have learned Context vocabulary as a
	// side effect of Context's migration. (Authority's D37ak
	// migration also verified this; we re-pin against Context.) Note:
	// the shell's pre-existing docblock contains an illustrative
	// `contextSelectedObjectPane` reference — that stays out of this
	// ban list (matches D37ak's `Shell_StaysLensAgnostic` precedent).
	for _, banned := range []string{
		"CONTEXT_TAB_CONFIG",
		"'context'",
		"\"context\"",
		"context.summary",
		"context.details",
		"context.relationships",
		"context.actions",
		"context.evidence",
		"contextSelectionBridge",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37am-context-tabs-config-impl: shared shell must remain lens-agnostic; banned token %q found", banned)
		}
	}
}

// ---------------------------------------------------------------------------
// D37an-context-tabs-summary-rollup — Map-level summary rollup migrated
// into the Context graph-native Summary section.
//
// The legacy right-side letterbox's map-level rollup (driven by
// `inspector.setSummary(...)` from
// `context-graph-view.js:597-627`) is now ALSO rendered inside the
// Context graph-native pane's Summary section. The pane reads the
// same projection data via the existing `contextProjection` handoff
// (`getCurrentProjection()` + `getLastMeta()`), so no new fetch and
// no index.html injection point is required. The rollup row
// vocabulary is byte-identical to the legacy setSummary call site,
// branching on `meta.view` ('service' | 'ai_system' |
// 'decision_surface').
//
// Invariants pinned by this section:
//
//   • `_buildSummaryRollupRows(projection, meta)` exported on the
//     pane's diagnostic surface as `_buildSummaryRollupRows`.
//   • Helper returns `[]` for missing/invalid projection.
//   • Helper returns the legacy 8-row Service rollup when
//     `meta.view === 'service'` (or meta is absent) and
//     `projection.business_service` is present.
//   • Helper returns the legacy 5-row AI System rollup when
//     `meta.view === 'ai_system'` and a root AI system matches
//     `meta.rootId`.
//   • Helper returns the legacy 5-row Decision Surface rollup when
//     `meta.view === 'decision_surface'` and a root surface matches
//     `meta.rootId`.
//   • `_renderSummary` calls the helper and appends a rollup block
//     after the existing identity content. The block carries a
//     `data-pane-summary-rollup` marker for testability and a
//     "Map summary" heading.
//   • Existing selected-object summary content (subtitle / top-2
//     badges / identity echo) remains intact.
//   • Legacy `inspector.setSummary(...)` call sites in
//     `context-graph-view.js` and the `context-selection-bridge.js`
//     drawer-setter wiring remain unchanged (preparation, not
//     migration).
//   • CONTEXT_TAB_CONFIG remains unchanged (D37am invariants).
//   • Locked copy remains unchanged.
//   • Authority untouched.
//   • Engine / viewport / camera untouched.

// ── JJ. Helper exported on the diagnostic surface ──────────────────

func TestExplorer_D37an_Context_BuildSummaryRollupRowsExported(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "function _buildSummaryRollupRows(projection, meta)") {
		t.Errorf("D37an-context-tabs-summary-rollup: pane must define _buildSummaryRollupRows(projection, meta)")
	}
	if !strings.Contains(js, "_buildSummaryRollupRows: _buildSummaryRollupRows,") {
		t.Errorf("D37an-context-tabs-summary-rollup: pane's public diagnostic surface must export _buildSummaryRollupRows")
	}
}

// ── KK. Service view rollup rows ───────────────────────────────────

func TestExplorer_D37an_Context_RollupServiceViewRows(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	startIdx := strings.Index(js, "function _buildSummaryRollupRows(projection, meta)")
	if startIdx < 0 {
		t.Fatal("D37an-context-tabs-summary-rollup: helper must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37an-context-tabs-summary-rollup: helper body must be well-formed")
	}
	body := tail[:endIdx+1]

	// Service view labels (byte-identical to context-graph-view.js
	// legacy setSummary call site, rows 615-625).
	for _, want := range []string{
		"'Business service',",
		"'Outgoing relationships',",
		"'Capabilities',",
		"'Processes',",
		"'Surfaces (active)',",
		"'AI systems',",
		"'Authority profiles',",
		"'Coverage gaps',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: service-view rollup must include row label %q", want)
		}
	}
	// Service-view data accessors.
	for _, want := range []string{
		"projection.business_service",
		"projection.relationships && projection.relationships.outgoing",
		"projection.authority_summary",
		"projection.coverage",
		"auth.active_profile_count",
		"cov.surfaces_with_no_ai_binding",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: service-view rollup must read %q", want)
		}
	}
}

// ── LL. AI System view rollup rows ────────────────────────────────

func TestExplorer_D37an_Context_RollupAISystemViewRows(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	startIdx := strings.Index(js, "function _buildSummaryRollupRows(projection, meta)")
	if startIdx < 0 {
		t.Fatal("D37an-context-tabs-summary-rollup: helper must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37an-context-tabs-summary-rollup: helper body must be well-formed")
	}
	body := tail[:endIdx+1]

	// AI system view branch + labels (byte-identical to
	// context-graph-view.js legacy setSummary call site, rows
	// 598-605).
	if !strings.Contains(body, "view === 'ai_system'") {
		t.Errorf("D37an-context-tabs-summary-rollup: helper must branch on view === 'ai_system'")
	}
	for _, want := range []string{
		"'Root AI system',",
		"'Capabilities',",
		"'Processes',",
		"'Surfaces',",
		"'Bindings',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: AI-system-view rollup must include row label %q", want)
		}
	}
	// Root AI lookup uses projection.ai_systems matched on rootId.
	if !strings.Contains(body, "projection.ai_systems") {
		t.Errorf("D37an-context-tabs-summary-rollup: AI-system-view rollup must read projection.ai_systems")
	}
	if !strings.Contains(body, "s.id === rootId") {
		t.Errorf("D37an-context-tabs-summary-rollup: AI-system-view rollup must look up root by `s.id === rootId`")
	}
	if !strings.Contains(body, "rootAI.bindings") {
		t.Errorf("D37an-context-tabs-summary-rollup: AI-system-view rollup must read rootAI.bindings for the Bindings count")
	}
}

// ── MM. Decision Surface view rollup rows ──────────────────────────

func TestExplorer_D37an_Context_RollupDecisionSurfaceViewRows(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	startIdx := strings.Index(js, "function _buildSummaryRollupRows(projection, meta)")
	if startIdx < 0 {
		t.Fatal("D37an-context-tabs-summary-rollup: helper must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37an-context-tabs-summary-rollup: helper body must be well-formed")
	}
	body := tail[:endIdx+1]

	// Decision-surface branch + labels (byte-identical to
	// context-graph-view.js legacy setSummary call site, rows
	// 607-614).
	if !strings.Contains(body, "view === 'decision_surface'") {
		t.Errorf("D37an-context-tabs-summary-rollup: helper must branch on view === 'decision_surface'")
	}
	for _, want := range []string{
		"'Root decision surface',",
		"'Parent process',",
		"'Surface version',",
		"'AI bindings (direct)',",
		"'AI systems',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: decision-surface-view rollup must include row label %q", want)
		}
	}
	// Root surface lookup uses projection.surfaces matched on rootId.
	if !strings.Contains(body, "projection.surfaces") {
		t.Errorf("D37an-context-tabs-summary-rollup: decision-surface-view rollup must read projection.surfaces")
	}
	if !strings.Contains(body, "rootSurf.ai_bindings") {
		t.Errorf("D37an-context-tabs-summary-rollup: decision-surface-view rollup must read rootSurf.ai_bindings")
	}
	if !strings.Contains(body, "rootSurf.process_id") {
		t.Errorf("D37an-context-tabs-summary-rollup: decision-surface-view rollup must read rootSurf.process_id")
	}
	if !strings.Contains(body, "rootSurf.version") {
		t.Errorf("D37an-context-tabs-summary-rollup: decision-surface-view rollup must read rootSurf.version")
	}
}

// ── NN. Missing data handling ──────────────────────────────────────

func TestExplorer_D37an_Context_RollupHandlesMissingData(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	startIdx := strings.Index(js, "function _buildSummaryRollupRows(projection, meta)")
	if startIdx < 0 {
		t.Fatal("D37an-context-tabs-summary-rollup: helper must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37an-context-tabs-summary-rollup: helper body must be well-formed")
	}
	body := tail[:endIdx+1]

	// Missing projection short-circuits to [].
	if !strings.Contains(body, "if (!projection || typeof projection !== 'object') return [];") {
		t.Errorf("D37an-context-tabs-summary-rollup: helper must return [] when projection is missing")
	}
	// Missing meta defaults view to 'service' (matches legacy default
	// at context-graph-view.js:114).
	if !strings.Contains(body, "? meta.view : 'service'") {
		t.Errorf("D37an-context-tabs-summary-rollup: helper must default missing meta.view to 'service'")
	}
	// Service view returns [] when business_service is absent.
	if !strings.Contains(body, "var bs = projection.business_service;") {
		t.Errorf("D37an-context-tabs-summary-rollup: service-view helper must read projection.business_service")
	}
	if !strings.Contains(body, "if (!bs || typeof bs !== 'object') return [];") {
		t.Errorf("D37an-context-tabs-summary-rollup: service-view helper must return [] when business_service is absent")
	}
	// AI / Surface branches return [] when root not found.
	aiBranchIdx := strings.Index(body, "view === 'ai_system'")
	surfBranchIdx := strings.Index(body, "view === 'decision_surface'")
	if aiBranchIdx < 0 || surfBranchIdx < 0 {
		t.Fatal("D37an-context-tabs-summary-rollup: both AI and Surface branches must exist")
	}
	if !strings.Contains(body[aiBranchIdx:surfBranchIdx], "if (!rootAI) return [];") {
		t.Errorf("D37an-context-tabs-summary-rollup: AI-system-view helper must return [] when root AI is not found")
	}
	if !strings.Contains(body[surfBranchIdx:], "if (!rootSurf) return [];") {
		t.Errorf("D37an-context-tabs-summary-rollup: decision-surface-view helper must return [] when root surface is not found")
	}
}

// ── OO. _renderSummary appends the rollup block ────────────────────

func TestExplorer_D37an_Context_RenderSummaryAppendsRollup(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	startIdx := strings.Index(js, "function _renderSummary(card)")
	if startIdx < 0 {
		t.Fatal("D37an-context-tabs-summary-rollup: _renderSummary must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37an-context-tabs-summary-rollup: _renderSummary body must be well-formed")
	}
	body := tail[:endIdx+1]

	for _, want := range []string{
		"_buildSummaryRollupRows(_getCurrentProjection(), _getLastProjectionMeta())",
		"if (rollupRows.length > 0)",
		"_renderSummaryRollupBlock(rollupRows)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: _renderSummary must contain %q", want)
		}
	}

	// Existing identity content remains.
	for _, want := range []string{
		"gmap-context-selected-object-pane-summary-subtitle",
		"gmap-context-selected-object-pane-summary-badges",
		"gmap-context-selected-object-pane-summary-echo",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: existing identity content %q must remain in _renderSummary", want)
		}
	}
}

// ── PP. Rollup block DOM contract ──────────────────────────────────

func TestExplorer_D37an_Context_RollupBlockDomContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "function _renderSummaryRollupBlock(rows)") {
		t.Errorf("D37an-context-tabs-summary-rollup: pane must define _renderSummaryRollupBlock(rows)")
	}
	for _, want := range []string{
		"'gmap-context-selected-object-pane-summary-rollup'",
		"'data-pane-summary-rollup'",
		"'gmap-context-selected-object-pane-summary-rollup-heading'",
		"'gmap-context-selected-object-pane-summary-rollup-list'",
		"'Map summary'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: rollup DOM contract must declare %q", want)
		}
	}
}

// ── QQ. _getLastProjectionMeta helper ──────────────────────────────

func TestExplorer_D37an_Context_LastProjectionMetaAccessor(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "function _getLastProjectionMeta()") {
		t.Errorf("D37an-context-tabs-summary-rollup: pane must define _getLastProjectionMeta()")
	}
	// Defensive: must use the existing contextProjection.getLastMeta
	// surface; must NOT fetch or mutate state.
	if !strings.Contains(js, "g.contextProjection.getLastMeta") {
		t.Errorf("D37an-context-tabs-summary-rollup: _getLastProjectionMeta must read contextProjection.getLastMeta")
	}
}

// ── RR. Legacy drawer summary path untouched ───────────────────────

func TestExplorer_D37an_LegacyDrawerSummaryPathUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// The legacy setSummary call sites in context-graph-view.js
	// (rows 597-627) remain intact — the rollup is added to the
	// pane, not removed from the drawer.
	viewJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(viewJs, "if (typeof ctx.setSummary === 'function') {") {
		t.Errorf("D37an-context-tabs-summary-rollup: legacy context-graph-view.js setSummary guard must remain")
	}
	for _, want := range []string{
		"['Root AI system',",
		"['Root decision surface',",
		"['Business service',",
		"['Outgoing relationships',",
		"['Authority profiles',",
		"['Coverage gaps',",
	} {
		if !strings.Contains(viewJs, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: legacy setSummary call site must still emit %q (this tranche is parity preparation, not drawer retirement)", want)
		}
	}
	// graph-inspector.js setSummary frame setter remains served.
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	if !strings.Contains(inspectorJs, "function setSummary(rows)") {
		t.Errorf("D37an-context-tabs-summary-rollup: graph-inspector.js setSummary frame setter must remain")
	}
	if !strings.Contains(inspectorJs, "document.getElementById('gmap-details-summary')") {
		t.Errorf("D37an-context-tabs-summary-rollup: drawer #gmap-details-summary target must remain")
	}
	if !strings.Contains(inspectorJs, "setSummary:       setSummary,") {
		t.Errorf("D37an-context-tabs-summary-rollup: graph-inspector.js setSummary export must remain")
	}
}

// ── SS. CONTEXT_TAB_CONFIG and copy unchanged ──────────────────────

func TestExplorer_D37an_Context_TabConfigAndCopyPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// D37am invariants — tab config unchanged.
	for _, want := range []string{
		"var CONTEXT_TAB_CONFIG = {",
		"enabled:    true,",
		"defaultTab: 'summary',",
		"id:       'summary',",
		"id:       'details',",
		"id:       'relationships',",
		"id:       'actions',",
		"id:       'evidence',",
		"tabs: CONTEXT_TAB_CONFIG",
		"_CONTEXT_TAB_CONFIG: CONTEXT_TAB_CONFIG,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: D37am invariant %q must remain unchanged", want)
		}
	}
	// Locked copy unchanged.
	for _, want := range []string{
		"noSelection:        'Select an object to inspect it.',",
		"noRelationships:    'No relationships for the selected object.',",
		"noDetails:          'No primary details available for this object.',",
		"evidenceDeferral:   'Detailed evidence remains available in the bottom Evidence tab.',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: locked copy %q must remain unchanged", want)
		}
	}
}

// ── TT. Authority and engine untouched ─────────────────────────────

func TestExplorer_D37an_AuthorityAndEngineUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	authJs := getExplorerAsset(t, srv, d37oImpl6AuthorityCanvasEdge)
	if !strings.Contains(authJs, "var AUTHORITY_TAB_CONFIG = {") {
		t.Errorf("D37an-context-tabs-summary-rollup: Authority AUTHORITY_TAB_CONFIG must remain unchanged")
	}
	if !strings.Contains(authJs, "tabs: AUTHORITY_TAB_CONFIG") {
		t.Errorf("D37an-context-tabs-summary-rollup: Authority provider must still expose `tabs: AUTHORITY_TAB_CONFIG`")
	}
	// Authority must not have absorbed the rollup helper or its
	// label vocabulary.
	for _, banned := range []string{
		"_buildSummaryRollupRows",
		"_renderSummaryRollupBlock",
		"_getLastProjectionMeta",
		"Map summary",
	} {
		if strings.Contains(authJs, banned) {
			t.Errorf("D37an-context-tabs-summary-rollup: Authority module must not contain rollup vocabulary (%q)", banned)
		}
	}

	// Engine markers from D37u / D37ah / D37ai / D37ak remain.
	engineJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	for _, want := range []string{
		"function _runFitPipeline(source, phase)",
		"function _onContainerResize()",
		"function _armInitialSafetyCap()",
	} {
		if !strings.Contains(engineJs, want) {
			t.Errorf("D37an-context-tabs-summary-rollup: engine marker %q must remain (this tranche does not modify the engine)", want)
		}
	}
}

// ── UU. Shared shell unchanged ─────────────────────────────────────

func TestExplorer_D37an_SharedShellUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js")

	// The shared shell must not have absorbed the rollup helper or
	// any Context-specific data accessors.
	for _, banned := range []string{
		"_buildSummaryRollupRows",
		"_renderSummaryRollupBlock",
		"_getLastProjectionMeta",
		"contextProjection",
		"business_service",
		"ai_systems",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37an-context-tabs-summary-rollup: shared shell must not contain Context-specific rollup token (%q)", banned)
		}
	}
}

// ---------------------------------------------------------------------------
// D37ao-context-tabs-governance-section — Fail-Mode Policy governance migrated
// into the Context graph-native Details section.
//
// The legacy right-side letterbox renders a Fail-Mode Policy reference
// block via `contextInspector.buildFailModePolicySection(nodeKind,
// details, data)` from `context-graph-inspector.js:53-96`. The
// renderer is pure and produces HTML for `nodeKind === 'business'`
// (Business Service) and `nodeKind === 'surface'` (Decision Surface)
// only; any other nodeKind returns ''.
//
// This tranche re-uses that legacy renderer from the Context graph-
// native pane (no duplication) and renders the same HTML inside
// the pane's Details section after the existing key-value rows /
// residual badges / metrics. ContextCard kinds (`business_service` /
// `decision_surface`) are mapped to the legacy nodeKind vocabulary
// before calling the renderer.
//
// Invariants pinned by this section:
//
//   • _mapCardKindToLegacyNodeKind maps 'business_service' → 'business',
//     'decision_surface' → 'surface', everything else → ''.
//   • _buildGovernanceHtml short-circuits to '' for non-governance
//     kinds; delegates to `contextInspector.buildFailModePolicySection`
//     for governance kinds.
//   • _renderDetails appends a `data-pane-details-governance` block
//     only when the helper returns a non-empty string.
//   • Legacy renderer `buildFailModePolicySection` source remains
//     byte-identical (label vocabulary + class names locked in).
//   • Legacy drawer setGovernance call path remains unchanged
//     (preparation, not retirement).
//   • CONTEXT_TAB_CONFIG remains unchanged (no Governance tab; the
//     governance content lives inside Details).
//   • Authority + engine + shared shell untouched.

// ── VV. Kind mapping helper ────────────────────────────────────────

func TestExplorer_D37ao_Context_KindMappingHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "function _mapCardKindToLegacyNodeKind(kind)") {
		t.Errorf("D37ao-context-tabs-governance-section: pane must define _mapCardKindToLegacyNodeKind(kind)")
	}
	startIdx := strings.Index(js, "function _mapCardKindToLegacyNodeKind(kind)")
	if startIdx < 0 {
		t.Fatal("D37ao-context-tabs-governance-section: kind mapping helper must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37ao-context-tabs-governance-section: kind mapping helper body must be well-formed")
	}
	body := tail[:endIdx+1]

	for _, want := range []string{
		"if (kind === 'business_service') return 'business';",
		"if (kind === 'decision_surface') return 'surface';",
		"return '';",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ao-context-tabs-governance-section: kind mapping must contain %q", want)
		}
	}
}

// ── WW. Governance HTML helper delegates to legacy renderer ────────

func TestExplorer_D37ao_Context_BuildGovernanceHtmlDelegatesToLegacyRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "function _buildGovernanceHtml(card)") {
		t.Errorf("D37ao-context-tabs-governance-section: pane must define _buildGovernanceHtml(card)")
	}
	startIdx := strings.Index(js, "function _buildGovernanceHtml(card)")
	if startIdx < 0 {
		t.Fatal("D37ao-context-tabs-governance-section: governance helper must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37ao-context-tabs-governance-section: governance helper body must be well-formed")
	}
	body := tail[:endIdx+1]

	for _, want := range []string{
		"var legacyKind = _mapCardKindToLegacyNodeKind(card.kind);",
		"if (!legacyKind) return '';",
		"g && g.contextInspector",
		"typeof ctxIns.buildFailModePolicySection !== 'function'",
		"_getCurrentProjection()",
		"ctxIns.buildFailModePolicySection(legacyKind, details, data)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ao-context-tabs-governance-section: governance helper must contain %q", want)
		}
	}
}

// ── XX. Helpers exported on diagnostic surface ─────────────────────

func TestExplorer_D37ao_Context_GovernanceHelpersExported(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	for _, want := range []string{
		"_mapCardKindToLegacyNodeKind: _mapCardKindToLegacyNodeKind,",
		"_buildGovernanceHtml:         _buildGovernanceHtml,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ao-context-tabs-governance-section: pane's public diagnostic surface must export %q", want)
		}
	}
}

// ── YY. _renderDetails appends the governance block ────────────────

func TestExplorer_D37ao_Context_RenderDetailsAppendsGovernance(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	startIdx := strings.Index(js, "function _renderDetails(card)")
	if startIdx < 0 {
		t.Fatal("D37ao-context-tabs-governance-section: _renderDetails must exist")
	}
	tail := js[startIdx:]
	endIdx := strings.Index(tail[1:], "\n  function ")
	if endIdx < 0 {
		t.Fatalf("D37ao-context-tabs-governance-section: _renderDetails body must be well-formed")
	}
	body := tail[:endIdx+1]

	for _, want := range []string{
		"var governanceHtml = _buildGovernanceHtml(card);",
		"if (governanceHtml)",
		"_renderGovernanceBlock(governanceHtml)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ao-context-tabs-governance-section: _renderDetails must contain %q", want)
		}
	}

	// Existing details content remains.
	for _, want := range []string{
		"COPY.noDetails",
		"gmap-context-selected-object-pane-details-list",
		"gmap-context-selected-object-pane-details-badges",
		"gmap-context-selected-object-pane-details-metrics",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ao-context-tabs-governance-section: existing details content %q must remain in _renderDetails", want)
		}
	}
}

// ── ZZ. Governance block DOM contract ──────────────────────────────

func TestExplorer_D37ao_Context_GovernanceBlockDomContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	if !strings.Contains(js, "function _renderGovernanceBlock(html)") {
		t.Errorf("D37ao-context-tabs-governance-section: pane must define _renderGovernanceBlock(html)")
	}
	for _, want := range []string{
		"'gmap-context-selected-object-pane-details-governance'",
		"'data-pane-details-governance'",
		"wrap.innerHTML = html;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ao-context-tabs-governance-section: governance block DOM contract must declare %q", want)
		}
	}
}

// ── AAA. Legacy renderer source unchanged ──────────────────────────

func TestExplorer_D37ao_LegacyGovernanceRendererUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-inspector.js")

	// Signature unchanged.
	if !strings.Contains(js, "function buildFailModePolicySection(nodeKind, details, data)") {
		t.Errorf("D37ao-context-tabs-governance-section: buildFailModePolicySection signature must remain (nodeKind, details, data)")
	}
	// Kind branches.
	for _, want := range []string{
		"if (nodeKind === 'business')",
		"} else if (nodeKind === 'surface')",
		// Any other kind falls through to return ''.
		"return '';",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ao-context-tabs-governance-section: legacy renderer kind branching %q must remain", want)
		}
	}
	// Locked label vocabulary (renderer emits these inside row arrays
	// and inside HTML literals; `>Fail Mode Policy<` is the HTML
	// substring form because the renderer concatenates it inline).
	for _, want := range []string{
		">Fail Mode Policy<",
		"'Default policy'",
		"'Source'",
		"'Surface override'",
		"'Inherited default'",
		"'Effective source'",
		"'Business service default'",
		"'None configured'",
		"'Runtime effect'",
		"'Evidence only'",
		"'Soft/open'",
		"'Not enabled'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ao-context-tabs-governance-section: legacy renderer label %q must remain", want)
		}
	}
	// Locked CSS class vocabulary (renderer emits these inside HTML
	// string literals, e.g. `class="gmap-fmp-reference-code"`, so we
	// match the bare class name without surrounding single quotes).
	for _, want := range []string{
		"gmap-fmp-reference-code",
		"gmap-fmp-reference-row",
		"gmap-fmp-reference-key",
		"gmap-fmp-reference-val",
		"gmap-fmp-reference",
		"gmap-details-section",
		"gmap-details-title",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ao-context-tabs-governance-section: legacy renderer class %q must remain", want)
		}
	}
	// Public-surface export still present.
	if !strings.Contains(js, "buildFailModePolicySection: buildFailModePolicySection,") {
		t.Errorf("D37ao-context-tabs-governance-section: contextInspector.buildFailModePolicySection public-surface export must remain")
	}
}

// ── BBB. Legacy drawer setGovernance path untouched ────────────────

func TestExplorer_D37ao_LegacyDrawerGovernancePathUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// graph-inspector.js frame setter remains.
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	if !strings.Contains(inspectorJs, "function setGovernance(html)") {
		t.Errorf("D37ao-context-tabs-governance-section: graph-inspector.js setGovernance frame setter must remain")
	}
	if !strings.Contains(inspectorJs, "document.getElementById('gmap-details-governance')") {
		t.Errorf("D37ao-context-tabs-governance-section: drawer #gmap-details-governance target must remain")
	}
	if !strings.Contains(inspectorJs, "setGovernance:    setGovernance,") {
		t.Errorf("D37ao-context-tabs-governance-section: graph-inspector.js setGovernance export must remain")
	}

	// context-selection-bridge.js still calls inspector.setGovernance.
	bridgeJs := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)
	if !strings.Contains(bridgeJs, "insp.setGovernance(html)") {
		t.Errorf("D37ao-context-tabs-governance-section: context-selection-bridge.js must still call insp.setGovernance(html)")
	}
	if !strings.Contains(bridgeJs, "ctxIns.buildFailModePolicySection(") {
		t.Errorf("D37ao-context-tabs-governance-section: context-selection-bridge.js must still invoke ctxIns.buildFailModePolicySection(...)")
	}

	// context-graph-inspector.js legacy selectNode path remains.
	legacyInspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-inspector.js")
	if !strings.Contains(legacyInspectorJs, "insp.setGovernance(buildFailModePolicySection(selectedNode.dataset.nodeKind || '', details, data));") {
		t.Errorf("D37ao-context-tabs-governance-section: legacy selectNode path must still call insp.setGovernance(buildFailModePolicySection(...))")
	}

	// #gmap-details-governance markup remains in index.html.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `id="gmap-details-governance"`) {
		t.Errorf("D37ao-context-tabs-governance-section: legacy #gmap-details-governance markup must remain in index.html")
	}
}

// ── CCC. CONTEXT_TAB_CONFIG unchanged (no Governance tab) ──────────

func TestExplorer_D37ao_Context_TabConfigUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// CONTEXT_TAB_CONFIG must remain D37am-shape — no new Governance
	// tab item, no relabel.
	for _, want := range []string{
		"var CONTEXT_TAB_CONFIG = {",
		"enabled:    true,",
		"defaultTab: 'summary',",
		"id:       'summary',",
		"id:       'details',",
		"id:       'relationships',",
		"id:       'actions',",
		"id:       'evidence',",
		"tabs: CONTEXT_TAB_CONFIG",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ao-context-tabs-governance-section: D37am invariant %q must remain", want)
		}
	}
	// No 'governance' tab id literal anywhere in the pane file.
	if strings.Contains(js, "id:       'governance'") {
		t.Errorf("D37ao-context-tabs-governance-section: pane must not declare a Governance tab item (governance lives inside Details)")
	}
	// Locked copy unchanged.
	for _, want := range []string{
		"noSelection:        'Select an object to inspect it.',",
		"noRelationships:    'No relationships for the selected object.',",
		"noDetails:          'No primary details available for this object.',",
		"evidenceDeferral:   'Detailed evidence remains available in the bottom Evidence tab.',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ao-context-tabs-governance-section: locked copy %q must remain", want)
		}
	}
}

// ── DDD. Authority + engine + shared shell untouched ───────────────

func TestExplorer_D37ao_AuthorityAndEngineAndShellUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	authJs := getExplorerAsset(t, srv, d37oImpl6AuthorityCanvasEdge)
	if !strings.Contains(authJs, "var AUTHORITY_TAB_CONFIG = {") {
		t.Errorf("D37ao-context-tabs-governance-section: Authority AUTHORITY_TAB_CONFIG must remain unchanged")
	}
	for _, banned := range []string{
		"_buildGovernanceHtml",
		"_mapCardKindToLegacyNodeKind",
		"buildFailModePolicySection",
		"Fail Mode Policy",
	} {
		if strings.Contains(authJs, banned) {
			t.Errorf("D37ao-context-tabs-governance-section: Authority module must not contain Context governance vocabulary (%q)", banned)
		}
	}

	// Engine markers remain.
	engineJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	for _, want := range []string{
		"function _runFitPipeline(source, phase)",
		"function _onContainerResize()",
		"function _armInitialSafetyCap()",
	} {
		if !strings.Contains(engineJs, want) {
			t.Errorf("D37ao-context-tabs-governance-section: engine marker %q must remain", want)
		}
	}

	// Shared shell must not have absorbed Context governance helpers.
	shellJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js")
	for _, banned := range []string{
		"_buildGovernanceHtml",
		"_mapCardKindToLegacyNodeKind",
		"buildFailModePolicySection",
		"Fail Mode Policy",
		"business_service",
		"decision_surface",
	} {
		if strings.Contains(shellJs, banned) {
			t.Errorf("D37ao-context-tabs-governance-section: shared shell must remain lens-agnostic; banned token %q", banned)
		}
	}
}

// ---------------------------------------------------------------------------
// D37ap-context-actions-parity-tests — Action dispatch parity between the
// legacy drawer path and the graph-native pane path.
//
// Both paths converge on `window.MIDASExplorerGraph._actionDispatcher` with
// an identical snake_case wire payload `{ kind, target_id, target_view,
// label }`. The conversion is done by the shared
// `context-selection-bridge.js` helper `toLegacyActionShape(action)`:
//
//   • Drawer path: `_populateInspector(card)` calls
//     `legacyActionsFromCard(card)` → snake_case array → `insp.setActions`
//     which renders buttons whose click handler invokes `dispatch(action)`
//     with the snake_case shape directly. Drawer-side whitelist + filters
//     live in `graph-inspector.js:setActions`.
//
//   • Pane path: `_renderActions(card)` reads camelCase ContextCard
//     actions, filters via the same `ALLOWED_ACTION_KINDS` whitelist plus
//     the `targetView` requirement for `reframe-around-this`, and renders
//     buttons with `data-action-*` attributes. The pane's body click
//     delegation rebuilds a camelCase action object and calls
//     `contextSelectionBridge.handleAction(action)`, which runs the same
//     `toLegacyActionShape` converter before calling `_actionDispatcher`.
//
// This tranche pins both paths' supported-kind sets, whitelist
// behaviour, filters, and dispatcher route. It also includes one minimal
// parity fix: the pane's `_renderActions` now drops
// `reframe-around-this` actions missing `targetView` so its filter is
// byte-equivalent to the drawer's. The shared converter guarantees the
// wire shape is identical at the dispatcher.

// ── EEE. Whitelist parity — both paths share the same kind set ──────

func TestExplorer_D37ap_Actions_WhitelistParity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Pane whitelist literal (locked by D37o-impl-6).
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	if !strings.Contains(paneJs, "var ALLOWED_ACTION_KINDS = ['reframe-around-this', 'view-business-service-record', 'view-capability-record']") {
		t.Errorf("D37ap-context-actions-parity-tests: pane ALLOWED_ACTION_KINDS must remain locked to the three MVP kinds")
	}

	// Drawer renderer whitelist (graph-inspector.js:setActions). The
	// drawer branches on action.kind for the same three kinds.
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	for _, want := range []string{
		"action.kind === 'view-business-service-record'",
		"action.kind === 'view-capability-record'",
		"action.kind === 'reframe-around-this'",
	} {
		if !strings.Contains(inspectorJs, want) {
			t.Errorf("D37ap-context-actions-parity-tests: legacy drawer setActions must branch on %q", want)
		}
	}

	// Card model's ACTION_KINDS constant — the authoritative source.
	cardJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-card-model.js")
	if !strings.Contains(cardJs, "var ACTION_KINDS = Object.freeze([") {
		t.Errorf("D37ap-context-actions-parity-tests: context-card-model.js must declare frozen ACTION_KINDS constant")
	}
	for _, want := range []string{
		"'reframe-around-this'",
		"'view-business-service-record'",
		"'view-capability-record'",
	} {
		if !strings.Contains(cardJs, want) {
			t.Errorf("D37ap-context-actions-parity-tests: ACTION_KINDS must include %q", want)
		}
	}
}

// ── FFF. Wire shape parity — both paths route through toLegacyActionShape ──

func TestExplorer_D37ap_Actions_WireShapeParity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	bridgeJs := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)

	// The bridge converter is the single source of truth for the wire
	// shape both paths produce.
	if !strings.Contains(bridgeJs, "function toLegacyActionShape(action)") {
		t.Fatal("D37ap-context-actions-parity-tests: context-selection-bridge.js must define toLegacyActionShape(action)")
	}
	idx := strings.Index(bridgeJs, "function toLegacyActionShape(action)")
	tail := bridgeJs[idx:]
	end := strings.Index(tail[1:], "\n  function ")
	if end < 0 {
		t.Fatalf("D37ap-context-actions-parity-tests: toLegacyActionShape body must be well-formed")
	}
	body := tail[:end+1]

	// Locked wire-shape mapping: camelCase → snake_case.
	for _, want := range []string{
		"kind:        action.kind        || '',",
		"target_id:   action.targetId    || '',",
		"target_view: action.targetView  || '',",
		"label:       action.label       || '',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ap-context-actions-parity-tests: toLegacyActionShape must emit %q", want)
		}
	}
}

// ── GGG. Drawer dispatch path — legacy preserved ───────────────────

func TestExplorer_D37ap_Actions_DrawerDispatchPathPinned(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	bridgeJs := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)

	// `_populateInspector` translates ContextCard actions into the
	// legacy wire shape and pushes them through `insp.setActions`.
	for _, want := range []string{
		"function legacyActionsFromCard(card)",
		"out.push(toLegacyActionShape(card.actions[i]));",
		"insp.setActions(legacyActionsFromCard(card))",
	} {
		if !strings.Contains(bridgeJs, want) {
			t.Errorf("D37ap-context-actions-parity-tests: drawer dispatch path must include %q", want)
		}
	}

	// graph-inspector.js setActions reads target_id / target_view and
	// dispatches via _actionDispatcher. Pin both view-record and
	// reframe branches.
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	if !strings.Contains(inspectorJs, "function setActions(actions)") {
		t.Errorf("D37ap-context-actions-parity-tests: graph-inspector.js setActions must remain defined")
	}
	for _, want := range []string{
		// view-record requires target_id only.
		"if (!action.target_id) return;",
		// reframe requires BOTH target_id and target_view.
		"if (!action.target_id || !action.target_view) return;",
		// Click handler dispatches the snake_case action directly.
		"if (dispatch) dispatch(action);",
	} {
		if !strings.Contains(inspectorJs, want) {
			t.Errorf("D37ap-context-actions-parity-tests: drawer renderer must contain %q", want)
		}
	}
	// Default labels locked.
	for _, want := range []string{
		"action.label || 'View record'",
		"action.label || 'Reframe around this'",
	} {
		if !strings.Contains(inspectorJs, want) {
			t.Errorf("D37ap-context-actions-parity-tests: drawer renderer default label %q must remain", want)
		}
	}
	// Dispatcher resolved from the canonical namespace.
	if !strings.Contains(inspectorJs, "window.MIDASExplorerGraph._actionDispatcher") {
		t.Errorf("D37ap-context-actions-parity-tests: drawer dispatcher must resolve via window.MIDASExplorerGraph._actionDispatcher")
	}
}

// ── HHH. Pane dispatch path — pinned end-to-end ─────────────────────

func TestExplorer_D37ap_Actions_PaneDispatchPathPinned(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// _renderActions reads camelCase ContextCard actions, filters via
	// ALLOWED_ACTION_KINDS, requires targetId for all kinds, plus
	// targetView specifically for reframe-around-this.
	startIdx := strings.Index(paneJs, "function _renderActions(card)")
	if startIdx < 0 {
		t.Fatal("D37ap-context-actions-parity-tests: pane _renderActions(card) must exist")
	}
	tail := paneJs[startIdx:]
	end := strings.Index(tail[1:], "\n  function ")
	if end < 0 {
		t.Fatalf("D37ap-context-actions-parity-tests: _renderActions body must be well-formed")
	}
	body := tail[:end+1]

	for _, want := range []string{
		"if (!a || !a.kind || !a.targetId) continue;",
		"if (ALLOWED_ACTION_KINDS.indexOf(a.kind) < 0) continue;",
		// D37ap parity fix — reframe-around-this also requires targetView.
		"if (a.kind === 'reframe-around-this' && !a.targetView) continue;",
		// Buttons carry the four data-action-* attributes the click
		// delegation rebuilds into a camelCase action object.
		"btn.setAttribute('data-action-kind', String(act.kind));",
		"btn.setAttribute('data-action-target-id', String(act.targetId));",
		"btn.setAttribute('data-action-target-view', String(act.targetView));",
		"btn.setAttribute('data-action-label',       String(act.label));",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ap-context-actions-parity-tests: pane _renderActions must contain %q", want)
		}
	}

	// Body click delegation reads data-action-* and routes via the
	// shared bridge's handleAction (which runs toLegacyActionShape
	// before calling _actionDispatcher).
	dIdx := strings.Index(paneJs, "function _wireBodyClickDelegation()")
	if dIdx < 0 {
		t.Fatal("D37ap-context-actions-parity-tests: pane _wireBodyClickDelegation must exist")
	}
	dTail := paneJs[dIdx:]
	dEnd := strings.Index(dTail[1:], "\n  function ")
	if dEnd < 0 {
		t.Fatalf("D37ap-context-actions-parity-tests: _wireBodyClickDelegation body must be well-formed")
	}
	dBody := dTail[:dEnd+1]

	for _, want := range []string{
		"actionEl.getAttribute('data-action-kind')",
		"actionEl.getAttribute('data-action-target-id')",
		"actionEl.getAttribute('data-action-target-view')",
		"actionEl.getAttribute('data-action-label')",
		"window.MIDASExplorerGraph.contextSelectionBridge",
		"bridge.handleAction(action)",
	} {
		if !strings.Contains(dBody, want) {
			t.Errorf("D37ap-context-actions-parity-tests: pane click delegation must contain %q", want)
		}
	}
}

// ── III. Bridge handleAction routes through dispatcher ──────────────

func TestExplorer_D37ap_Actions_BridgeHandleActionRoutesToDispatcher(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	bridgeJs := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)

	startIdx := strings.Index(bridgeJs, "function handleAction(action)")
	if startIdx < 0 {
		t.Fatal("D37ap-context-actions-parity-tests: bridge handleAction must exist")
	}
	tail := bridgeJs[startIdx:]
	end := strings.Index(tail[1:], "\n  function ")
	if end < 0 {
		t.Fatalf("D37ap-context-actions-parity-tests: handleAction body must be well-formed")
	}
	body := tail[:end+1]

	for _, want := range []string{
		"var dispatch = window.MIDASExplorerGraph._actionDispatcher;",
		"if (typeof dispatch !== 'function') return;",
		"dispatch(toLegacyActionShape(action));",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ap-context-actions-parity-tests: bridge handleAction must contain %q", want)
		}
	}
}

// ── JJJ. Dispatcher namespace parity ───────────────────────────────

func TestExplorer_D37ap_Actions_BothPathsTargetSameDispatcherNamespace(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// graph-inspector.js (drawer) resolves dispatcher via:
	//   `window.MIDASExplorerGraph._actionDispatcher`
	// context-selection-bridge.js (pane → bridge) resolves it via:
	//   `window.MIDASExplorerGraph._actionDispatcher`
	// Both must reference the SAME canonical namespace path.
	for _, asset := range []string{
		"/explorer/assets/js/graph/graph-inspector.js",
		d37oImpl6BridgeAsset,
	} {
		js := getExplorerAsset(t, srv, asset)
		if !strings.Contains(js, "window.MIDASExplorerGraph._actionDispatcher") {
			t.Errorf("D37ap-context-actions-parity-tests: %q must resolve the dispatcher via window.MIDASExplorerGraph._actionDispatcher", asset)
		}
	}

	// The pane itself MUST NOT bypass the bridge by calling the
	// dispatcher directly — that would create a second dispatch path
	// and break the parity guarantee.
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	if strings.Contains(paneJs, "_actionDispatcher") {
		t.Errorf("D37ap-context-actions-parity-tests: pane must NOT reference _actionDispatcher directly — actions route through contextSelectionBridge.handleAction")
	}
}

// ── KKK. view-business-service-record parity ───────────────────────

func TestExplorer_D37ap_Actions_ViewBusinessServiceRecordParity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Drawer renderer renders this kind when target_id is present.
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	if !regexp.MustCompile(`(?s)action\.kind === 'view-business-service-record'[\s\S]*?if \(!action\.target_id\) return;`).MatchString(inspectorJs) {
		t.Errorf("D37ap-context-actions-parity-tests: drawer view-business-service-record branch must require target_id only")
	}

	// Pane renderer accepts kind via ALLOWED_ACTION_KINDS whitelist
	// and requires targetId via the shared filter. No additional
	// guard required for this kind.
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	if !strings.Contains(paneJs, "'view-business-service-record'") {
		t.Errorf("D37ap-context-actions-parity-tests: pane ALLOWED_ACTION_KINDS must declare 'view-business-service-record'")
	}
}

// ── LLL. view-capability-record parity ─────────────────────────────

func TestExplorer_D37ap_Actions_ViewCapabilityRecordParity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	if !regexp.MustCompile(`(?s)action\.kind === 'view-capability-record'[\s\S]*?if \(!action\.target_id\) return;`).MatchString(inspectorJs) {
		t.Errorf("D37ap-context-actions-parity-tests: drawer view-capability-record branch must require target_id only")
	}

	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	if !strings.Contains(paneJs, "'view-capability-record'") {
		t.Errorf("D37ap-context-actions-parity-tests: pane ALLOWED_ACTION_KINDS must declare 'view-capability-record'")
	}
}

// ── MMM. reframe-around-this parity (target_id + target_view) ──────

func TestExplorer_D37ap_Actions_ReframeAroundThisParity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Drawer requires BOTH target_id and target_view.
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	if !regexp.MustCompile(`(?s)action\.kind === 'reframe-around-this'[\s\S]*?if \(!action\.target_id \|\| !action\.target_view\) return;`).MatchString(inspectorJs) {
		t.Errorf("D37ap-context-actions-parity-tests: drawer reframe-around-this branch must require both target_id and target_view")
	}

	// Pane (after D37ap parity fix) also requires targetView for
	// reframe-around-this in addition to the general targetId
	// requirement.
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	if !strings.Contains(paneJs, "if (a.kind === 'reframe-around-this' && !a.targetView) continue;") {
		t.Errorf("D37ap-context-actions-parity-tests: pane _renderActions must drop reframe-around-this actions missing targetView (parity with drawer)")
	}
}

// ── NNN. Unsupported action filtering parity ───────────────────────

func TestExplorer_D37ap_Actions_UnsupportedKindsFilteredInBothPaths(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Drawer setActions branches on the three known kinds; any other
	// kind falls through the if/else if chain without rendering a
	// button. Pin the branching structure.
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	if !regexp.MustCompile(`(?s)action\.kind === 'view-business-service-record' \|\|[\s\S]*?action\.kind === 'view-capability-record'[\s\S]*?\} else if \(action\.kind === 'reframe-around-this'\)`).MatchString(inspectorJs) {
		t.Errorf("D37ap-context-actions-parity-tests: drawer setActions must branch on the three known kinds in sequence (unsupported kinds fall through)")
	}

	// Pane explicit whitelist filter.
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	if !strings.Contains(paneJs, "if (ALLOWED_ACTION_KINDS.indexOf(a.kind) < 0) continue;") {
		t.Errorf("D37ap-context-actions-parity-tests: pane _renderActions must whitelist via ALLOWED_ACTION_KINDS")
	}
}

// ── OOO. Default labels preserved ──────────────────────────────────

func TestExplorer_D37ap_Actions_LabelsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")

	// Drawer default labels (used when action.label is empty).
	for _, want := range []string{
		"'View record'",
		"'Reframe around this'",
	} {
		if !strings.Contains(inspectorJs, want) {
			t.Errorf("D37ap-context-actions-parity-tests: drawer default label %q must remain", want)
		}
	}

	// Pane button text uses act.label || act.kind — labels are
	// passed through unchanged. Pin the construction.
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	if !strings.Contains(paneJs, "btn.textContent = String(act.label || act.kind);") {
		t.Errorf("D37ap-context-actions-parity-tests: pane button text must use `String(act.label || act.kind)` (labels passed through unchanged)")
	}
}

// ── PPP. CONTEXT_TAB_CONFIG + section markers unchanged ────────────

func TestExplorer_D37ap_Actions_ContextTabConfigUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// D37am tab config invariants.
	for _, want := range []string{
		"var CONTEXT_TAB_CONFIG = {",
		"enabled:    true,",
		"defaultTab: 'summary',",
		"id:       'actions',",
		"label:    'Actions',",
		"provider: 'context.actions',",
		"tabs: CONTEXT_TAB_CONFIG",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ap-context-actions-parity-tests: D37am tab-config invariant %q must remain", want)
		}
	}

	// D37an summary rollup + D37ao governance helpers remain.
	for _, want := range []string{
		"function _buildSummaryRollupRows(projection, meta)",
		"function _buildGovernanceHtml(card)",
		"function _renderGovernanceBlock(html)",
		"_buildSummaryRollupRows: _buildSummaryRollupRows,",
		"_mapCardKindToLegacyNodeKind: _mapCardKindToLegacyNodeKind,",
		"_buildGovernanceHtml:         _buildGovernanceHtml,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ap-context-actions-parity-tests: prior tranche marker %q must remain", want)
		}
	}
}

// ── QQQ. Engine + Authority + shared shell untouched ───────────────

func TestExplorer_D37ap_Actions_PlatformUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Engine markers remain.
	engineJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	for _, want := range []string{
		"function _runFitPipeline(source, phase)",
		"function _onContainerResize()",
		"function _armInitialSafetyCap()",
	} {
		if !strings.Contains(engineJs, want) {
			t.Errorf("D37ap-context-actions-parity-tests: engine marker %q must remain", want)
		}
	}
	// Engine must not reference Context action vocabulary.
	for _, banned := range []string{
		"_actionDispatcher",
		"ALLOWED_ACTION_KINDS",
		"toLegacyActionShape",
		"handleAction",
	} {
		if strings.Contains(engineJs, banned) {
			t.Errorf("D37ap-context-actions-parity-tests: engine must not contain action-dispatch token %q", banned)
		}
	}

	// Authority untouched.
	authJs := getExplorerAsset(t, srv, d37oImpl6AuthorityCanvasEdge)
	if !strings.Contains(authJs, "var AUTHORITY_TAB_CONFIG = {") {
		t.Errorf("D37ap-context-actions-parity-tests: Authority AUTHORITY_TAB_CONFIG must remain unchanged")
	}
	for _, banned := range []string{
		"toLegacyActionShape",
		"ALLOWED_ACTION_KINDS",
		"contextSelectionBridge",
	} {
		if strings.Contains(authJs, banned) {
			t.Errorf("D37ap-context-actions-parity-tests: Authority module must not contain Context action vocabulary (%q)", banned)
		}
	}

	// Shared shell stays lens-agnostic — must not handle Context
	// action dispatch.
	shellJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js")
	for _, banned := range []string{
		"_actionDispatcher",
		"toLegacyActionShape",
		"handleAction",
		"ALLOWED_ACTION_KINDS",
		"target_id",
		"target_view",
	} {
		if strings.Contains(shellJs, banned) {
			t.Errorf("D37ap-context-actions-parity-tests: shared shell must remain lens-agnostic; banned token %q", banned)
		}
	}
}

// ── RRR. Legacy drawer remains in service ──────────────────────────

func TestExplorer_D37ap_Actions_LegacyDrawerStillInService(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// The legacy right-rail letterbox markup remains intact.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`id="gmap-details"`,
		`id="gmap-details-actions"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ap-context-actions-parity-tests: legacy drawer markup %q must remain (this tranche does not retire the drawer)", want)
		}
	}

	// graph-inspector.js + graph-drawer.js still served.
	for _, asset := range []string{
		"/explorer/assets/js/graph/graph-inspector.js",
		"/explorer/assets/js/graph/graph-drawer.js",
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37ap-context-actions-parity-tests: %q must remain served", asset)
		}
	}
}

// ---------------------------------------------------------------------------
// D37aq-strategic-context-drawer-gate — Feature-flag gate of the legacy
// right-side drawer for strategic Context.
//
// Two coordinated suppressions, both scoped to the strategic Context
// renderer ONLY:
//
//   1. JS gate in `context-selection-bridge.js`: short-circuits the
//      `_populateInspector(card)` call when the strategic Context
//      renderer is active AND the URL does not carry
//      `?legacyContextInspector=1`. All other selection plumbing
//      (shared selection state, bottom evidence tray, subscribers,
//      shared-bridge push, action dispatch) runs unchanged. The
//      graph-native pane stays in charge of selected-object detail.
//
//   2. CSS gate in `shell.css`, keyed off
//      `body[data-strategic-context-inspector="graph-pane"]`. The
//      pane sets this attribute on init + every observed body /
//      viewport attribute change when the strategic renderer is
//      active and the fallback flag is absent; the attribute is
//      removed when the fallback flag is present, when the active
//      renderer flips away from strategic Context, or on destroy.
//      The scoped rules zero the right-rail width reservation
//      (`shell.css:248-250` + the inspector-collapsed variant) so
//      the graph canvas reclaims the width the legacy drawer used
//      to occupy.
//
// Authority, legacy/native Context, and non-graph views never
// carry the body attribute and never reach the strategic
// renderer's bridge gate, so their behaviour is unchanged. The
// legacy drawer markup, JS modules, frame setters, and CSS rules
// remain served and reachable; the fallback flag restores the
// pre-D37aq behaviour for support and comparison.

// ── SSS. Flag parser lives in both consumers ───────────────────────

func TestExplorer_D37aq_FlagParserDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Bridge owns the JS suppression gate.
	bridgeJs := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)
	for _, want := range []string{
		"var LEGACY_CONTEXT_INSPECTOR_QUERY_PARAM = 'legacyContextInspector';",
		"function _hasLegacyContextInspectorFlag()",
		"decodeURIComponent(pair[0]) === LEGACY_CONTEXT_INSPECTOR_QUERY_PARAM",
		"decodeURIComponent(pair[1] || '') === '1'",
	} {
		if !strings.Contains(bridgeJs, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: bridge must declare flag parser %q", want)
		}
	}

	// Pane owns the body-attribute lifecycle (CSS suppression gate).
	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	for _, want := range []string{
		"var LEGACY_CONTEXT_INSPECTOR_QUERY_PARAM   = 'legacyContextInspector';",
		"function _hasLegacyContextInspectorFlag()",
		"decodeURIComponent(pair[0]) === LEGACY_CONTEXT_INSPECTOR_QUERY_PARAM",
		"decodeURIComponent(pair[1] || '') === '1'",
	} {
		if !strings.Contains(paneJs, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: pane must declare flag parser %q", want)
		}
	}
}

// ── TTT. Bridge suppression predicate ──────────────────────────────

func TestExplorer_D37aq_BridgeSuppressionPredicate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)

	// The strategic-context-renderer detection helper.
	if !strings.Contains(js, "function _isStrategicContextRendererActive()") {
		t.Errorf("D37aq-strategic-context-drawer-gate: bridge must define _isStrategicContextRendererActive()")
	}
	// Bridge detection uses ONLY the GraphViewport active-renderer
	// getter — the bridge must not scrape legacy DOM per D37o-impl-5.
	if !strings.Contains(js, "g.viewport.getActiveRendererId() === 'context'") {
		t.Errorf("D37aq-strategic-context-drawer-gate: bridge detection must include the GraphViewport active-renderer getter")
	}
	// Defensive fail-open: missing viewport API returns false (legacy
	// drawer still populated).
	if !strings.Contains(js, "if (g && g.viewport && typeof g.viewport.getActiveRendererId === 'function')") {
		t.Errorf("D37aq-strategic-context-drawer-gate: bridge detection must guard the viewport getter")
	}
	// The combined suppression predicate.
	if !strings.Contains(js, "function _isLegacyContextDrawerSuppressed()") {
		t.Errorf("D37aq-strategic-context-drawer-gate: bridge must define _isLegacyContextDrawerSuppressed()")
	}
	if !strings.Contains(js, "_isStrategicContextRendererActive() && !_hasLegacyContextInspectorFlag()") {
		t.Errorf("D37aq-strategic-context-drawer-gate: bridge predicate must AND strategic-renderer detection with the fallback-flag absence")
	}
}

// ── UUU. Bridge gates _populateInspector ───────────────────────────

func TestExplorer_D37aq_BridgeGatesPopulateInspector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)

	// selectCard must wrap _populateInspector in the suppression
	// predicate. Pin the exact source pattern.
	if !regexp.MustCompile(`(?s)function selectCard\(card\)[\s\S]*?if \(!_isLegacyContextDrawerSuppressed\(\)\)[\s\S]*?_populateInspector\(card\);`).MatchString(js) {
		t.Errorf("D37aq-strategic-context-drawer-gate: selectCard must guard _populateInspector with !_isLegacyContextDrawerSuppressed()")
	}

	// The non-drawer side-effects (shared selection, evidence tray,
	// subscribers, shared bridge push) must remain UNCONDITIONAL —
	// they belong to selection plumbing that always fires.
	for _, want := range []string{
		"sel.setSelected(card.id)",
		"tray.notifySelectionChanged()",
		"_notify(card);",
		"_pushSelectionToSharedBridge(card)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: selectCard must still run %q (non-drawer plumbing remains unconditional)", want)
		}
	}
}

// ── VVV. Pane body-attribute lifecycle ─────────────────────────────

func TestExplorer_D37aq_PaneBodyAttributeLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl6PaneAsset)

	// Constants pinning the attribute name + applied value.
	for _, want := range []string{
		"var STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR  = 'data-strategic-context-inspector';",
		"var STRATEGIC_CONTEXT_INSPECTOR_PANE_VALUE = 'graph-pane';",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: pane must declare %q", want)
		}
	}
	// Apply helper sets/removes the attribute based on the same
	// detection + flag predicate.
	if !strings.Contains(js, "function _applyStrategicContextInspectorAttribute()") {
		t.Errorf("D37aq-strategic-context-drawer-gate: pane must define _applyStrategicContextInspectorAttribute()")
	}
	if !strings.Contains(js, "if (_isStrategicContextActive() && !_hasLegacyContextInspectorFlag())") {
		t.Errorf("D37aq-strategic-context-drawer-gate: attribute setter must AND strategic-renderer detection with the fallback-flag absence")
	}
	for _, want := range []string{
		"document.body.setAttribute(\n        STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR,\n        STRATEGIC_CONTEXT_INSPECTOR_PANE_VALUE);",
		"document.body.removeAttribute(STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: attribute apply helper must set/remove via %q", want)
		}
	}
	// init() applies the attribute up front.
	if !regexp.MustCompile(`(?s)_inited = true;[\s\S]*?_applyStrategicContextInspectorAttribute\(\);`).MatchString(js) {
		t.Errorf("D37aq-strategic-context-drawer-gate: init() must call _applyStrategicContextInspectorAttribute() after _inited = true")
	}
	// _onBodyAttributesChanged re-applies on every observed change.
	if !regexp.MustCompile(`(?s)function _onBodyAttributesChanged\(\)[\s\S]*?_applyStrategicContextInspectorAttribute\(\);`).MatchString(js) {
		t.Errorf("D37aq-strategic-context-drawer-gate: _onBodyAttributesChanged must call _applyStrategicContextInspectorAttribute()")
	}
	// destroy() clears the attribute.
	if !regexp.MustCompile(`(?s)function destroy\(\)[\s\S]*?document\.body\.removeAttribute\(STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR\)`).MatchString(js) {
		t.Errorf("D37aq-strategic-context-drawer-gate: destroy() must clear the strategic-context-inspector body attribute")
	}
}

// ── WWW. Scoped CSS override in shell.css ──────────────────────────

func TestExplorer_D37aq_ShellCssScopedOverride(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")

	// The original right-rail width reservation rules remain — this
	// tranche does not delete legacy CSS.
	for _, want := range []string{
		"body.gmap-mode .shell-header { right: var(--inspector-width); }",
		"body.gmap-mode .shell-footer { right: var(--inspector-width); }",
		"body.gmap-mode .shell-main   { margin-right: var(--inspector-width); }",
		"body.gmap-mode.inspector-collapsed .shell-header { right: var(--gmap-right-rail-handle-width); }",
		"body.gmap-mode.inspector-collapsed .shell-footer { right: var(--gmap-right-rail-handle-width); }",
		"body.gmap-mode.inspector-collapsed .shell-main   { margin-right: var(--gmap-right-rail-handle-width); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: original right-rail rule %q must remain (legacy CSS preserved)", want)
		}
	}
	// New scoped overrides — both the standard and inspector-collapsed
	// variants zero the right inset / margin for strategic Context
	// default mode.
	for _, want := range []string{
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode .shell-header { right: 0; }",
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode .shell-footer { right: 0; }",
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode .shell-main   { margin-right: 0; }",
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode.inspector-collapsed .shell-header { right: 0; }",
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode.inspector-collapsed .shell-footer { right: 0; }",
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode.inspector-collapsed .shell-main   { margin-right: 0; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: scoped right-rail-zero rule %q must be present", want)
		}
	}
}

// ── XXX. Authority paths unaffected ────────────────────────────────

func TestExplorer_D37aq_AuthorityUnaffected(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	authJs := getExplorerAsset(t, srv, d37oImpl6AuthorityCanvasEdge)
	// Authority module must contain no flag/gate vocabulary.
	for _, banned := range []string{
		"legacyContextInspector",
		"data-strategic-context-inspector",
		"_hasLegacyContextInspectorFlag",
		"_isLegacyContextDrawerSuppressed",
		"_applyStrategicContextInspectorAttribute",
	} {
		if strings.Contains(authJs, banned) {
			t.Errorf("D37aq-strategic-context-drawer-gate: Authority module must not contain Context-gate token %q", banned)
		}
	}
	// Authority's own tab config is unchanged.
	if !strings.Contains(authJs, "var AUTHORITY_TAB_CONFIG = {") {
		t.Errorf("D37aq-strategic-context-drawer-gate: Authority AUTHORITY_TAB_CONFIG must remain unchanged")
	}
}

// ── YYY. Engine / shared shell / viewport / camera unaffected ──────

func TestExplorer_D37aq_PlatformUnaffected(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js",
		"/explorer/assets/js/graph/graph-platform/graph-stage.js",
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/graph-camera.js",
		"/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js",
		"/explorer/assets/js/graph/context/context-cytoscape-renderer.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"legacyContextInspector",
			"data-strategic-context-inspector",
			"_isLegacyContextDrawerSuppressed",
			"_applyStrategicContextInspectorAttribute",
			"STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37aq-strategic-context-drawer-gate: %q must not contain Context-gate token %q (gate is Context-scoped)", asset, banned)
			}
		}
	}

	// Engine markers from prior tranches remain.
	engineJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	for _, want := range []string{
		"function _runFitPipeline(source, phase)",
		"function _onContainerResize()",
		"function _armInitialSafetyCap()",
	} {
		if !strings.Contains(engineJs, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: engine marker %q must remain", want)
		}
	}
}

// ── ZZZ. Legacy drawer + inspector + native Context preserved ──────

func TestExplorer_D37aq_LegacyDrawerStillServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Markup remains.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`id="gmap-details"`,
		`id="gmap-details-actions"`,
		`id="gmap-details-summary"`,
		`id="gmap-details-governance"`,
		`class="governance-map-details gmap-right-rail"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: legacy drawer markup %q must remain (no markup removal)", want)
		}
	}

	// Drawer + inspector + legacy Context inspector modules remain
	// served and define their public surfaces.
	for _, asset := range []string{
		"/explorer/assets/js/graph/graph-drawer.js",
		"/explorer/assets/js/graph/graph-inspector.js",
		"/explorer/assets/js/graph/context/context-graph-inspector.js",
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37aq-strategic-context-drawer-gate: %q must remain served", asset)
		}
	}
	// Legacy native Context inspector still defines selectNode +
	// buildFailModePolicySection (legacy/native Context path).
	legacyCtxJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-inspector.js")
	for _, want := range []string{
		"function selectNode(nodeId)",
		"function buildFailModePolicySection(nodeKind, details, data)",
		"buildFailModePolicySection: buildFailModePolicySection,",
	} {
		if !strings.Contains(legacyCtxJs, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: legacy native Context inspector must still define %q", want)
		}
	}
	// graph-inspector.js frame setters remain.
	inspectorJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	for _, want := range []string{
		"function setName(name)",
		"function setFields(rows)",
		"function setSummary(rows)",
		"function setGovernance(html)",
		"function setActions(actions)",
	} {
		if !strings.Contains(inspectorJs, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: graph-inspector.js %q must remain", want)
		}
	}
}

// ── BBBB. Right-rail visual suppression (D37aq-fix) ───────────────
//
// D37aq dropped the right-rail width reservation when strategic
// Context default mode is active, but the legacy rail element
// itself (`#gmap-details` / `.gmap-right-rail`) stayed painted at
// the canvas edge — the Inspector / Evidence / Config tab strip
// remained visible. This fix completes the gate with a scoped
// `display: none` keyed off the same body attribute, so:
//
//   • default strategic Context — rail visually hidden, no
//     pointer/event surface, canvas reclaims the width.
//   • fallback (`?legacyContextInspector=1`) — body attribute
//     absent, rail visible as before.
//   • Authority, legacy/native Context, non-graph views — body
//     attribute never set, rail unaffected.

func TestExplorer_D37aq_RightRailHiddenInGraphPaneMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")

	// The scoped suppression rule must hide #gmap-details in
	// graph-pane mode. D37aq-fix2 raised the selector specificity
	// from (1,1,1) → (1,3,1) by adding `.gmap-mode` (class) and
	// `.gmap-right-rail` (class) so the rule unambiguously beats
	// the competing `body.gmap-mode #gmap-details { display: flex; }`
	// rule at (1,1,1) in governance-map.css regardless of source
	// order between the two stylesheets.
	want := "body.gmap-mode[data-strategic-context-inspector=\"graph-pane\"] #gmap-details.gmap-right-rail {\n    display: none;\n  }"
	if !strings.Contains(css, want) {
		t.Errorf("D37aq-fix-right-rail-visual-suppression: shell.css must declare the scoped rail-hide rule:\n%s", want)
	}

	// Defensive structural check: the selector must include
	// `[data-strategic-context-inspector="graph-pane"]` so the rule
	// can never hide #gmap-details outside graph-pane mode.
	railHideIdx := strings.Index(css, "#gmap-details.gmap-right-rail {\n    display: none;\n  }")
	if railHideIdx < 0 {
		t.Fatal("D37aq-fix-right-rail-visual-suppression: rail-hide block must be present")
	}
	// Walk backwards to the rule's selector and confirm scoping.
	preamble := css[:railHideIdx]
	lastBrace := strings.LastIndex(preamble, "}")
	scopeStart := lastBrace
	if scopeStart < 0 {
		scopeStart = 0
	}
	selector := strings.TrimSpace(preamble[scopeStart:])
	if !strings.Contains(selector, "[data-strategic-context-inspector=\"graph-pane\"]") {
		t.Errorf("D37aq-fix-right-rail-visual-suppression: rail-hide selector must be scoped to [data-strategic-context-inspector=\"graph-pane\"]; got %q", selector)
	}

	// The pre-existing right-rail rules must remain — the legacy
	// drawer stays available under the fallback flag.
	for _, want := range []string{
		"body.gmap-mode .shell-main   { margin-right: var(--inspector-width); }",
		"body.gmap-mode.inspector-collapsed .shell-main   { margin-right: var(--gmap-right-rail-handle-width); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37aq-fix-right-rail-visual-suppression: pre-existing right-rail rule %q must remain", want)
		}
	}

	// And the D37aq width-zero override rules from the previous
	// tranche must remain — this fix is additive.
	for _, want := range []string{
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode .shell-main   { margin-right: 0; }",
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode .shell-header { right: 0; }",
		"body[data-strategic-context-inspector=\"graph-pane\"].gmap-mode .shell-footer { right: 0; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37aq-fix-right-rail-visual-suppression: D37aq width-zero override %q must remain", want)
		}
	}

	// Legacy drawer markup remains served — the fix does not delete
	// the element.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`id="gmap-details"`,
		`class="governance-map-details gmap-right-rail"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37aq-fix-right-rail-visual-suppression: legacy drawer markup %q must remain (this fix hides, does not delete)", want)
		}
	}
}

// ── BBBB-fix2. Cascade beats display:flex competitor ──────────────
//
// D37aq-fix2-right-rail-css-cascade — the prior fix's selector
// (specificity (1,1,1)) was TIED with the existing
// `body.gmap-mode #gmap-details { display: flex; }` rule in
// governance-map.css ((1,1,1)). shell.css loads before
// governance-map.css, so the later `display: flex` won and the
// rail stayed visible.
//
// This test pins:
//   • the new selector form
//     `body.gmap-mode[data-strategic-context-inspector="graph-pane"] #gmap-details.gmap-right-rail`,
//   • the competing legacy rule remains intact (we don't delete
//     legacy CSS),
//   • no global `display: none !important` or unconditional rail
//     hiding crept in,
//   • no `!important` is used on the suppression rule itself
//     (specificity alone solves the cascade).

func TestExplorer_D37aq_RightRailHideRuleWinsCascade(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	shellCss      := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	governanceCss := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// The legacy `body.gmap-mode #gmap-details { display: flex; }`
	// must remain (fallback / legacy/native Context path).
	if !strings.Contains(governanceCss, "body.gmap-mode #gmap-details {\n    display: flex;\n  }") {
		t.Errorf("D37aq-fix2-right-rail-css-cascade: legacy `body.gmap-mode #gmap-details { display: flex; }` rule must remain in governance-map.css")
	}

	// The new suppression rule must be the (1,3,1) form so it beats
	// the (1,1,1) `display: flex` rule by specificity alone.
	newRule := "body.gmap-mode[data-strategic-context-inspector=\"graph-pane\"] #gmap-details.gmap-right-rail {\n    display: none;\n  }"
	if !strings.Contains(shellCss, newRule) {
		t.Errorf("D37aq-fix2-right-rail-css-cascade: shell.css must declare the higher-specificity suppression rule:\n%s", newRule)
	}

	// The old (1,1,1) form MUST NOT remain — leaving it in would be
	// dead-code clutter; the new rule supersedes it.
	oldRule := "body[data-strategic-context-inspector=\"graph-pane\"] #gmap-details {\n    display: none;\n  }"
	if strings.Contains(shellCss, oldRule) {
		t.Errorf("D37aq-fix2-right-rail-css-cascade: the prior (1,1,1) suppression rule must be replaced by the (1,3,1) form, not coexist with it")
	}

	// No `!important` on the suppression rule — specificity alone
	// must win the cascade.
	if strings.Contains(shellCss, "display: none !important") {
		t.Errorf("D37aq-fix2-right-rail-css-cascade: suppression rule must not use !important; specificity alone is sufficient")
	}

	// No broad `display: none !important` hiding. The legacy
	// `#gmap-details { display: none; }` first-paint default at
	// governance-map.css:1862-1872 IS legitimate (it's the
	// pre-gmap-mode state and is re-enabled by the (1,1,1)
	// `body.gmap-mode #gmap-details { display: flex; }` rule), so
	// we only ban rules that use !important to force-hide globally.
	for _, banned := range []string{
		"#gmap-details { display: none !important; }",
		".gmap-right-rail { display: none !important; }",
	} {
		for _, css := range []string{shellCss, governanceCss} {
			if strings.Contains(css, banned) {
				t.Errorf("D37aq-fix2-right-rail-css-cascade: banned !important drawer-hide token %q must not appear", banned)
			}
		}
	}
	// Defensive: ensure no rule whose selector is just
	// `.gmap-right-rail` (no qualifiers) hides the rail globally —
	// such a rule would also hide it under the fallback flag and
	// in legacy/native Context. The chained selector
	// `#gmap-details.gmap-right-rail` from the suppression rule is
	// fine (it's a qualified compound) — we detect the bad form by
	// looking for `.gmap-right-rail` as a complete selector
	// terminated by whitespace or `{`, preceded by whitespace.
	if regexp.MustCompile(`(?m)^\s*\.gmap-right-rail\s*\{`).MatchString(shellCss) ||
		regexp.MustCompile(`(?m)^\s*\.gmap-right-rail\s*\{`).MatchString(governanceCss) {
		// Such a rule could exist legitimately if it does NOT include
		// `display: none`. Check the property next.
		for _, css := range []string{shellCss, governanceCss} {
			if regexp.MustCompile(`(?ms)^\s*\.gmap-right-rail\s*\{[^}]*display:\s*none`).MatchString(css) {
				t.Errorf("D37aq-fix2-right-rail-css-cascade: an unqualified `.gmap-right-rail { display: none }` rule must not exist (would hide the rail in fallback / legacy/native Context too)")
			}
		}
	}

	// Cross-file source order: shell.css link must come BEFORE
	// governance-map.css link in index.html. This pins the load
	// order this fix accounts for. (Specificity (1,3,1) > (1,1,1)
	// solves the cascade regardless, but documenting the order
	// keeps a future refactor honest.)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	shellIdx := strings.Index(body, `href="/explorer/assets/css/shell.css"`)
	govIdx   := strings.Index(body, `href="/explorer/assets/css/governance-map.css"`)
	if shellIdx < 0 || govIdx < 0 {
		t.Fatal("D37aq-fix2-right-rail-css-cascade: both shell.css and governance-map.css link tags must be present")
	}
	if !(shellIdx < govIdx) {
		t.Errorf("D37aq-fix2-right-rail-css-cascade: shell.css must load before governance-map.css (shellIdx=%d, govIdx=%d)", shellIdx, govIdx)
	}
}

// ── AAAA. Prior tranche markers (D37am/D37an/D37ao/D37ap) preserved ──

func TestExplorer_D37aq_PriorTranchesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	paneJs := getExplorerAsset(t, srv, d37oImpl6PaneAsset)
	for _, want := range []string{
		// D37am
		"var CONTEXT_TAB_CONFIG = {",
		"defaultTab: 'summary',",
		"tabs: CONTEXT_TAB_CONFIG",
		"_CONTEXT_TAB_CONFIG: CONTEXT_TAB_CONFIG,",
		// D37an
		"function _buildSummaryRollupRows(projection, meta)",
		"function _renderSummaryRollupBlock(rows)",
		"function _getLastProjectionMeta()",
		"_buildSummaryRollupRows: _buildSummaryRollupRows,",
		// D37ao
		"function _mapCardKindToLegacyNodeKind(kind)",
		"function _buildGovernanceHtml(card)",
		"function _renderGovernanceBlock(html)",
		"_mapCardKindToLegacyNodeKind: _mapCardKindToLegacyNodeKind,",
		"_buildGovernanceHtml:         _buildGovernanceHtml,",
	} {
		if !strings.Contains(paneJs, want) {
			t.Errorf("D37aq-strategic-context-drawer-gate: prior tranche marker %q must remain in pane", want)
		}
	}

	// D37ap parity — pane keeps reframe targetView check, bridge
	// keeps toLegacyActionShape converter, drawer still served.
	if !strings.Contains(paneJs, "if (a.kind === 'reframe-around-this' && !a.targetView) continue;") {
		t.Errorf("D37aq-strategic-context-drawer-gate: D37ap reframe targetView parity check must remain in pane")
	}
	bridgeJs := getExplorerAsset(t, srv, d37oImpl6BridgeAsset)
	if !strings.Contains(bridgeJs, "function toLegacyActionShape(action)") {
		t.Errorf("D37aq-strategic-context-drawer-gate: D37ap toLegacyActionShape converter must remain in bridge")
	}
}
