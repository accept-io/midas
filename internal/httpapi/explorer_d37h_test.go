package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37h-authority-cytoscape-navigation-toolbar-rewiring-and-enhancement
//
// Camera/navigation-only tranche. Pins:
//
//   (1) Existing `.gmap-camera-cluster` controls still exist and are
//       Cytoscape-backed via the toolbar bridge.
//   (2) New camera/navigation controls exist with stable IDs:
//         • #gmap-zoom-percent           — current zoom display
//         • #gmap-zoom-selected-button   — camera focus on selected node
//         • #gmap-reset-view-button      — safe-area-aware default view
//   (3) Renderer exposes the camera-navigation API the toolbar relies on:
//         • cytoscapePoc.zoomToSelected
//         • cytoscapePoc.resetView
//         • cytoscapePoc.getZoomPercent
//         • cytoscapePoc.onViewportChanged
//         • cytoscapePoc.onSelectionChanged
//   (4) `cy.on('dbltap', 'node', ...)` is wired in `_wireInteractions`
//       (double-click a card/node = zoom to that node).
//   (5) `zoomToSelected` reads `cy.elements(':selected')`, not card DOM.
//   (6) `resetView` does NOT call raw `cy.reset()` — it uses the
//       safe-area-aware fit path.
//   (7) Toolbar bridge code does NOT directly write to any
//       `.cytoscape-poc-html-card` `.style.transform` — card
//       positioning remains the D37f two-tier overlay's job.
//   (8) New icon-only buttons carry aria-label + title, and the icons
//       are action-metaphors — they do NOT reuse Authority node-kind
//       class names (`authority-html-card-icon`, etc.) or `data-kind`
//       hooks.
//   (9) D37b / D37d / D37f contracts remain.
//
// D37h is camera-only: no selection mode, no dependency view, no
// context menu, no filters, no bulk actions, no graph mutation.

const (
	d37hAuthorityShellAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37hAuthorityToolbarAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
	d37hAuthorityCSSPath    = "/explorer/assets/css/authority-cytoscape-poc.css"
)

// readD37hWireInteractionsBody bounds assertions to the
// `_wireInteractions` body inside the renderer so unrelated `cy.on`
// calls elsewhere don't false-match.
func readD37hWireInteractionsBody(t *testing.T, js string) string {
	t.Helper()
	start := strings.Index(js, "function _wireInteractions()")
	if start < 0 {
		t.Fatal("D37h: _wireInteractions definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37h: cannot bound _wireInteractions body")
	}
	return js[start : start+end]
}

// TestExplorer_D37h_ExistingCameraControlsPresent pins that every
// pre-D37h control is still in the markup. The brief explicitly
// forbids removing or moving them.
func TestExplorer_D37h_ExistingCameraControlsPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
		`class="gmap-camera-cluster"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37h: existing camera control %q must remain in index.html", want)
		}
	}
}

// TestExplorer_D37h_NewControlsPresentInsideCluster pins that the
// three new D37h controls live inside `.gmap-camera-cluster` (i.e.
// no parallel toolbar was created).
func TestExplorer_D37h_NewControlsPresentInsideCluster(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	clusterStart := strings.Index(body, `class="gmap-camera-cluster"`)
	if clusterStart < 0 {
		t.Fatal("D37h: `.gmap-camera-cluster` wrapper missing")
	}
	clusterEnd := strings.Index(body[clusterStart:], `</div>`)
	if clusterEnd < 0 {
		t.Fatal("D37h: cannot bound `.gmap-camera-cluster` to first `</div>`")
	}
	cluster := body[clusterStart : clusterStart+clusterEnd]

	for _, want := range []string{
		`id="gmap-zoom-percent"`,
		`id="gmap-zoom-selected-button"`,
		`id="gmap-reset-view-button"`,
	} {
		if !strings.Contains(cluster, want) {
			t.Errorf("D37h: new camera control %q must live INSIDE `.gmap-camera-cluster` — markup was:\n%s", want, cluster)
		}
	}
}

// TestExplorer_D37h_NewControlsHaveAccessibleLabels pins that every
// new icon-only control carries `aria-label` AND `title`. The text
// must match the documented D37h action semantics.
func TestExplorer_D37h_NewControlsHaveAccessibleLabels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Zoom percentage display — non-interactive but still needs an
	// accessible name.
	for _, want := range []string{
		`id="gmap-zoom-percent"`,
		`role="status"`,
		`aria-live="polite"`,
		`aria-label="Current zoom"`,
		`title="Current zoom"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37h: zoom-percent display must carry %q", want)
		}
	}

	// Zoom to selected button — must start disabled.
	for _, want := range []string{
		`id="gmap-zoom-selected-button"`,
		`aria-label="Zoom to selected"`,
		`title="Zoom to selected"`,
		`aria-disabled="true"`,
		`disabled`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37h: zoom-to-selected button must carry %q", want)
		}
	}

	// Reset view button — enabled by default; safe-area-aware fit.
	for _, want := range []string{
		`id="gmap-reset-view-button"`,
		`aria-label="Reset view"`,
		`title="Reset view"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37h: reset-view button must carry %q", want)
		}
	}
}

// TestExplorer_D37h_RendererExposesNavigationAPI pins that the
// Authority Cytoscape renderer exposes the camera/navigation surface
// the toolbar bridge calls into.
func TestExplorer_D37h_RendererExposesNavigationAPI(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)

	for _, want := range []string{
		// Public-surface keys.
		"zoomToSelected:",
		"resetView:",
		"getZoomPercent:",
		"onViewportChanged:",
		"onSelectionChanged:",
		"isReady:",
		"zoomToNode:",
		// Implementation helpers.
		"function _zoomToSelected(cy)",
		"function _resetView(cy)",
		"function _getZoomPercent(cy)",
		"function _onViewportChanged(handler)",
		"function _onSelectionChanged(handler)",
		"function _zoomToNode(cy, node)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37h: renderer must expose %q", want)
		}
	}
}

// TestExplorer_D37h_ZoomToSelectedReadsCytoscapeSelection pins that
// `_zoomToSelected` sources from `cy.elements(':selected')` — NOT
// from HTML card DOM state.
func TestExplorer_D37h_ZoomToSelectedReadsCytoscapeSelection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)

	// Bound to the _zoomToSelected function body.
	start := strings.Index(js, "function _zoomToSelected(cy)")
	if start < 0 {
		t.Fatal("D37h: _zoomToSelected definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37h: cannot bound _zoomToSelected body")
	}
	body := js[start : start+end]

	if !strings.Contains(body, "cy.elements(':selected')") {
		t.Errorf("D37h: _zoomToSelected must read `cy.elements(':selected')` — body was:\n%s", body)
	}
	// Negative pin — must not reach into HTML card DOM for selection.
	for _, banned := range []string{
		"document.querySelector",
		"document.getElementById",
		"cytoscape-poc-html-card",
		"authority-html-card",
		".classList",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37h: _zoomToSelected must not touch HTML card DOM (found %q in body)", banned)
		}
	}
}

// TestExplorer_D37h_ResetViewDoesNotCallRawCyReset pins that
// `_resetView` does NOT call `cy.reset()` (which would ignore
// MIDAS safe-area chrome). It must delegate to
// `_fitToAvailableCanvas` like the Fit button.
func TestExplorer_D37h_ResetViewDoesNotCallRawCyReset(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)

	start := strings.Index(js, "function _resetView(cy)")
	if start < 0 {
		t.Fatal("D37h: _resetView definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37h: cannot bound _resetView body")
	}
	body := js[start : start+end]

	if !strings.Contains(body, "_fitToAvailableCanvas(cy)") {
		t.Errorf("D37h: _resetView must delegate to `_fitToAvailableCanvas(cy)` — body was:\n%s", body)
	}
	if strings.Contains(body, "cy.reset()") {
		t.Errorf("D37h: _resetView must NOT call raw `cy.reset()` (ignores MIDAS safe-area chrome) — body was:\n%s", body)
	}
}

// TestExplorer_D37h_DoubleTapZoomWiredThroughCytoscape pins that the
// `dbltap` binding is on the underlying Cytoscape node — NOT on the
// HTML card DOM.
func TestExplorer_D37h_DoubleTapZoomWiredThroughCytoscape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)
	body := readD37hWireInteractionsBody(t, js)

	if !strings.Contains(body, "_cy.on('dbltap', 'node'") {
		t.Errorf("D37h: _wireInteractions must bind `_cy.on('dbltap', 'node', ...)` — body was:\n%s", body)
	}
	// Negative pin — must not bind a DOM double-click on HTML cards.
	if strings.Contains(body, "addEventListener('dblclick'") {
		t.Errorf("D37h: must NOT bind DOM dblclick on HTML cards — pointer-passive overlay must remain")
	}
}

// TestExplorer_D37h_GetZoomPercentReadsCyZoom pins that the zoom
// percentage helper sources from `cy.zoom()`, returns null when no
// cy is mounted, and rounds to a whole percentage.
func TestExplorer_D37h_GetZoomPercentReadsCyZoom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)

	start := strings.Index(js, "function _getZoomPercent(cy)")
	if start < 0 {
		t.Fatal("D37h: _getZoomPercent definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37h: cannot bound _getZoomPercent body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		"cy.zoom()",
		"Math.round(z * 100)",
		"return null",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37h: _getZoomPercent must include %q — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37h_ViewportSubscriptionUsesCytoscapeEvents pins that
// the renderer's external viewport-change subscription binds to the
// Cytoscape `zoom pan resize` events (not a polling loop or DOM
// reads).
func TestExplorer_D37h_ViewportSubscriptionUsesCytoscapeEvents(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)

	for _, want := range []string{
		"cy.on('zoom pan resize'",
		"cy.on('select unselect'",
		"_viewportChangeHandlers",
		"_selectionChangeHandlers",
		"_attachExternalHandlersToCy(_cy)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37h: viewport/selection subscription wiring must include %q", want)
		}
	}
}

// TestExplorer_D37h_ToolbarBridgeCallsCytoscapeApi pins that the
// toolbar bridge's handlers route to the renderer's camera surface
// — not to HTML card DOM manipulation. (D37p-impl-4 retired the
// bridge's capture-phase camera intercepts in favour of the shared
// graphCameraBus + graph-camera-toolbar-adapter. The bridge's
// camera-API helper functions remain in source as the Authority
// delegate's call target via `cytoscapePoc.*`; that's what this
// test now pins.)
func TestExplorer_D37h_ToolbarBridgeCallsCytoscapeApi(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityToolbarAsset)

	for _, want := range []string{
		// Camera handlers still route through PoC.
		"poc.zoomBy",
		"poc.centerOnRoot",
		"poc.fit",
		"poc.zoomToSelected()",
		"poc.resetView()",
		// Zoom % render reads from PoC, not DOM.
		"poc.getZoomPercent",
		// Zoom-selected button id still referenced (the bridge still
		// owns the disabled-state mirror via _zoomSelectedBtnEl).
		"'gmap-zoom-selected-button'",
		// Subscription helpers.
		"poc.onViewportChanged",
		"poc.onSelectionChanged",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37h: toolbar bridge must include %q", want)
		}
	}
}

// TestExplorer_D37h_ToolbarBridgeDoesNotTouchHtmlCardDom pins the
// architectural separation: the toolbar bridge is camera/navigation
// only. It must NOT directly manipulate HTML card DOM positions —
// the D37f two-tier overlay owns that.
func TestExplorer_D37h_ToolbarBridgeDoesNotTouchHtmlCardDom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityToolbarAsset)

	for _, banned := range []string{
		".cytoscape-poc-html-card",
		"authority-html-card",
		"style.transform",
		"querySelector('.cytoscape-poc",
		"querySelectorAll('.cytoscape-poc",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37h: toolbar bridge must NOT contain %q — card positioning is the D37f overlay's job", banned)
		}
	}
}

// TestExplorer_D37h_NewIconsAreActionMetaphorsNotNodeKindIcons pins
// the icon policy split: toolbar action icons must NOT reuse the
// Authority node-kind icon classes or `data-kind` hooks.
func TestExplorer_D37h_NewIconsAreActionMetaphorsNotNodeKindIcons(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Bound the assertion to the camera cluster markup so unrelated
	// occurrences elsewhere in the page (e.g. inside Authority HTML
	// cards) don't trip the check.
	clusterStart := strings.Index(body, `class="gmap-camera-cluster"`)
	if clusterStart < 0 {
		t.Fatal("D37h: `.gmap-camera-cluster` wrapper missing")
	}
	clusterEnd := strings.Index(body[clusterStart:], `</div>`)
	if clusterEnd < 0 {
		t.Fatal("D37h: cannot bound `.gmap-camera-cluster`")
	}
	cluster := body[clusterStart : clusterStart+clusterEnd]

	for _, banned := range []string{
		"authority-html-card-icon",
		"data-kind=",
		"authorityBusinessService",
		"authorityProfile",
		"authorityGrant",
		"authorityAgent",
		"authorityFailModePolicy",
		"authorityDecisionSurface",
		"authorityEscalationTarget",
	} {
		if strings.Contains(cluster, banned) {
			t.Errorf("D37h: camera-cluster toolbar must NOT reuse Authority node-kind icon hook %q — cluster was:\n%s", banned, cluster)
		}
	}
}

// TestExplorer_D37h_BadgeCssIsPresent pins the new zoom-percentage
// badge styling. The badge lives alongside the existing
// `.gmap-camera-cluster button` rules in governance-map.css because
// `authority-cytoscape-poc.css` is renderer-scoped and the badge is a
// host-level camera-cluster element (the existing camera buttons are
// also styled in governance-map.css). The new D37h `<button>`
// elements inherit the `.gmap-camera-cluster button` shape rules and
// need no new CSS of their own.
func TestExplorer_D37h_BadgeCssIsPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		".gmap-camera-cluster .gmap-zoom-percent",
		"font-variant-numeric: tabular-nums",
		"height: 32px",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37h: zoom-percent badge CSS must include %q in governance-map.css", want)
		}
	}
}

// TestExplorer_D37h_CentreButtonSemanticPreserved pins the audited
// outcome (Option 1 per the brief): the existing #gmap-centre-button
// preserves its legacy `centerOnRoot()` call and its current label.
// The brief allowed temporarily preserving this behaviour and
// reporting it as legacy/ambiguous. A reset/default-view button is
// added separately rather than reinterpreting Centre.
func TestExplorer_D37h_CentreButtonSemanticPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	js := getExplorerAsset(t, srv, d37hAuthorityToolbarAsset)

	if !strings.Contains(body, `id="gmap-centre-button"`) {
		t.Error("D37h: existing centre button must remain")
	}
	// Bridge still calls centerOnRoot — i.e. behaviour preserved.
	if !strings.Contains(js, "poc.centerOnRoot()") {
		t.Error("D37h: legacy centre-button handler must continue to call `poc.centerOnRoot()` (preserve outcome)")
	}
	// The new reset/default-view button is a SEPARATE control, not
	// a reinterpretation of Centre.
	if !strings.Contains(body, `id="gmap-reset-view-button"`) {
		t.Error("D37h: reset-view must be a separate control, not a reinterpretation of Centre")
	}
}

// TestExplorer_D37h_DisabledStateGuardedByCyState pins that the
// zoom-to-selected button's disabled state is sourced from
// Cytoscape selection state (`cy.elements(':selected').length`),
// not a DOM-card heuristic.
func TestExplorer_D37h_DisabledStateGuardedByCyState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityToolbarAsset)

	start := strings.Index(js, "function _syncZoomSelectedEnabled()")
	if start < 0 {
		t.Fatal("D37h: _syncZoomSelectedEnabled definition not found in toolbar bridge")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37h: cannot bound _syncZoomSelectedEnabled body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		"cy.elements(':selected')",
		`btn.setAttribute('disabled'`,
		`btn.removeAttribute('disabled')`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37h: _syncZoomSelectedEnabled must include %q — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37h_NoRootCenteringPinnedAsDefault pins the brief's
// guard against entrenching root-centering as the default reset.
// `_resetView` must NOT call `_centerOnRoot`.
func TestExplorer_D37h_NoRootCenteringPinnedAsDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)

	start := strings.Index(js, "function _resetView(cy)")
	if start < 0 {
		t.Fatal("D37h: _resetView definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37h: cannot bound _resetView body")
	}
	body := js[start : start+end]

	if strings.Contains(body, "_centerOnRoot") {
		t.Errorf("D37h: _resetView must NOT root-centre by default — body was:\n%s", body)
	}
}

// TestExplorer_D37h_PreservesD37fOverlayContracts is the foundation
// preservation check.
func TestExplorer_D37h_PreservesD37fOverlayContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)

	for _, want := range []string{
		// D37f two-tier transform constants.
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
		"var PROJECTION_MODEL  = 'layer-pan-zoom-card-model-position'",
		// D37f default-theme promotion.
		"var DEFAULT_THEME  = 'html-card'",
		// D37f install / sync / destroy lifecycle.
		"function _installHtmlCardOverlay(cy, mount, elements)",
		"function _syncLayer()",
		"function _syncCards()",
		"function _destroyHtmlCardOverlay()",
		// D37b registration.
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
		// D37f rich-card icon plumbing (D37f-rich-card tranche).
		"_AUTHORITY_KIND_ICON_KEYS",
		"'authority-html-card-icon'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37h: foundation contract %q must remain", want)
		}
	}
}

// TestExplorer_D37h_PreservesD37bD37dContracts pins the wider
// D37b/D37d activation + mount contracts.
func TestExplorer_D37h_PreservesD37bD37dContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37hAuthorityShellAsset)
	css := getExplorerAsset(t, srv, d37hAuthorityCSSPath)

	for _, want := range []string{
		"vp.register('authority', _authorityRendererFactory)",
		`_mountEl.setAttribute('aria-label', 'Authority Graph')`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37h: D37b contract %q must remain", want)
		}
	}

	cssExec := stripCSSComments(css)
	if !strings.Contains(cssExec, "position: absolute;") || !strings.Contains(cssExec, "inset: 0;") {
		t.Error("D37h: D37d mount positioning (position: absolute; inset: 0;) must remain")
	}
}
