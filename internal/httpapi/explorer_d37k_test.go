package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37k-impl-1 — Authority Graph HTML Relationship Label Overlay
//
// Two coordinated changes:
//
//   (Option A — canvas fallback) The default html-card theme now
//   gets a friendly edge-label canvas style (12px, weight 500,
//   _displayEdgeLabel, round-rectangle chip, full-contrast colour).
//   Mirrors the authority-thin-card-v1 styling so the fallback is
//   readable if the HTML overlay fails.
//
//   (Option B — HTML overlay) A new tier sibling to the cards
//   overlay holds a single shared chip element positioned at the
//   hovered edge's midpoint in model coordinates. The chip is
//   projected via the same `_syncLayer` transform that drives the
//   cards overlay (one extra style write — still O(1)). The chip
//   is hover-only: shown by `_focusEdge`, hidden by
//   `_clearInteractionState`. The cards-tier sync re-positions the
//   chip when a node drag moves an edge endpoint. Hidden cy edges
//   (authority-context view filtering) cause the chip to auto-hide.
//
// D37k-impl-1 makes no backend, schema, OpenAPI, projection, or
// toolbar change. It is a render-tier-only enhancement, entirely
// scoped to the Authority Cytoscape PoC assets.

const (
	d37kAuthorityShellAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37kAuthorityCSSPath    = "/explorer/assets/css/authority-cytoscape-poc.css"
)

func readD37kFunctionBody(t *testing.T, js, signature string) string {
	t.Helper()
	start := strings.Index(js, signature)
	if start < 0 {
		t.Fatalf("D37k: function %q not found", signature)
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatalf("D37k: cannot bound function %q", signature)
	}
	return js[start : start+end]
}

// ── Option A — canvas-label fallback in the html-card theme branch ──

// TestExplorer_D37k_HtmlCardEdgeLabelUsesDisplayEdgeLabel pins that
// the default html-card theme appends an `edge.cy-focused` rule that
// uses the friendly `_displayEdgeLabel(ele)` lookup, not the raw
// `data(label)` underscore-replaced kind. The fallback also covers
// `edge.cy-on-root-path` so the spine-emphasis labels match.
func TestExplorer_D37k_HtmlCardEdgeLabelUsesDisplayEdgeLabel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)

	// Locate the html-card theme branch.
	branchStart := strings.Index(js, "if (themeName === 'html-card') {")
	if branchStart < 0 {
		t.Fatal("D37k: `if (themeName === 'html-card')` branch not found")
	}
	// Bound to the next theme branch.
	branchEnd := strings.Index(js[branchStart:], "if (themeName === 'object-tile-v3')")
	if branchEnd < 0 {
		// Fall back to the next sibling theme check.
		branchEnd = strings.Index(js[branchStart:], "if (themeName === 'authority-thin-card-v1')")
	}
	if branchEnd < 0 {
		t.Fatal("D37k: cannot bound the html-card theme branch")
	}
	branch := js[branchStart : branchStart+branchEnd]

	for _, want := range []string{
		"selector: 'edge.cy-focused'",
		"selector: 'edge.cy-on-root-path'",
		"_displayEdgeLabel(ele)",
		"'font-size':                        '12px'",
		"'font-weight':                      '500'",
		"'text-background-shape':            'round-rectangle'",
		"'text-background-padding':          '5px'",
		"pal.onSurface",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D37k: html-card theme edge-label fallback must include %q — branch:\n%s", want, branch)
		}
	}

	// Negative pin: the fallback must NOT use the muted variant
	// colour or the raw `data(label)` expression.
	if strings.Contains(branch, "pal.onSurfaceMut") {
		t.Errorf("D37k: html-card theme edge-label fallback must use full-contrast `pal.onSurface`, not the muted variant")
	}
}

// ── Option B — HTML overlay tier ──────────────────────────────────

// TestExplorer_D37k_OverlayInstallHelperExists pins the new install
// helper + its DOM-shape contract: single overlay element, single
// chip child, aria-hidden + hidden attributes.
func TestExplorer_D37k_OverlayInstallHelperExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _installEdgeLabelOverlay(cy, mount)")

	for _, want := range []string{
		"_destroyEdgeLabelOverlay()",
		"document.createElement('div')",
		"'cytoscape-poc-edge-label-overlay'",
		"'cytoscape-poc-edge-label-chip'",
		"setAttribute('aria-hidden', 'true')",
		"setAttribute('hidden', '')",
		"mount.appendChild(_edgeLabelOverlayEl)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37k: _installEdgeLabelOverlay must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37k_OverlaySingleSharedChip pins the architectural
// invariant: one overlay + one chip, NEVER iterated, NEVER per-edge.
// Mirrors the "single chip, never per-edge DOM" requirement from
// the D37k-assess report.
func TestExplorer_D37k_OverlaySingleSharedChip(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _installEdgeLabelOverlay(cy, mount)")

	// Exactly one `document.createElement('div')` for the overlay
	// and exactly one for the chip — two total. Any more would
	// suggest per-edge or per-something iteration.
	count := strings.Count(body, "document.createElement('div')")
	if count != 2 {
		t.Errorf("D37k: _installEdgeLabelOverlay must create exactly one overlay + one chip (2x createElement). Got %d.\nbody:\n%s", count, body)
	}

	// No iteration constructs (per-edge DOM would require them).
	for _, banned := range []string{
		"for (var i = 0",
		"forEach(function",
		".edges().forEach",
		"_cy.edges()",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37k: _installEdgeLabelOverlay must NOT iterate (single shared chip) — found %q", banned)
		}
	}
}

// TestExplorer_D37k_ShowEdgeLabelHelper pins the show contract.
func TestExplorer_D37k_ShowEdgeLabelHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _showEdgeLabel(edge)")

	for _, want := range []string{
		"_displayEdgeLabel(edge)",
		"_edgeLabelChipEl.textContent = text",
		"_syncEdgeLabelPosition()",
		"removeAttribute('hidden')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37k: _showEdgeLabel must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37k_HideEdgeLabelHelper pins the hide contract.
func TestExplorer_D37k_HideEdgeLabelHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _hideEdgeLabel()")

	for _, want := range []string{
		`_edgeLabelFocusedEdgeId = ''`,
		"setAttribute('hidden', '')",
		"_edgeLabelChipEl.textContent = ''",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37k: _hideEdgeLabel must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37k_SyncPositionUsesModelMidpoint pins that the
// chip is positioned in MODEL coordinates via `edge.midpoint()` (or
// a fallback computed from connected node positions) with the same
// `translate(x, y) translate(-50%, -50%)` shape the cards-tier
// sync uses. Negative pin: never `renderedPosition` / `scale(` /
// `cy.zoom()` inside this function (the layer transform projects).
func TestExplorer_D37k_SyncPositionUsesModelMidpoint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _syncEdgeLabelPosition()")

	for _, want := range []string{
		"edge.midpoint()",
		"'translate(' + mid.x + 'px,' + mid.y + 'px) translate(-50%, -50%)'",
		"_edgeLabelChipEl.style.transform",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37k: _syncEdgeLabelPosition must include %q — body:\n%s", want, body)
		}
	}

	for _, banned := range []string{
		"renderedPosition",
		"scale(",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37k: _syncEdgeLabelPosition must NOT use %q (layer transform owns projection) — body:\n%s", banned, body)
		}
	}
}

// TestExplorer_D37k_SyncPositionHidesOnHiddenEdge pins that the
// chip auto-hides when its focused edge is hidden (authority-context
// view) or has been removed (re-render). No stale chip pinned to a
// dead edge.
func TestExplorer_D37k_SyncPositionHidesOnHiddenEdge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _syncEdgeLabelPosition()")

	for _, want := range []string{
		"!edge.length",
		"_hideEdgeLabel()",
		"!edge.visible()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37k: _syncEdgeLabelPosition must defensively hide on missing/hidden edges — missing %q", want)
		}
	}
}

// TestExplorer_D37k_DestroyClearsChipState pins the teardown
// contract: _destroyEdgeLabelOverlay removes DOM, clears state, and
// is safe to call multiple times.
func TestExplorer_D37k_DestroyClearsChipState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _destroyEdgeLabelOverlay()")

	for _, want := range []string{
		`_edgeLabelFocusedEdgeId = ''`,
		"_edgeLabelOverlayEl.parentNode.removeChild(_edgeLabelOverlayEl)",
		"_edgeLabelOverlayEl = null",
		"_edgeLabelChipEl    = null",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37k: _destroyEdgeLabelOverlay must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37k_FocusEdgeWiresChip pins that `_focusEdge` calls
// `_showEdgeLabel(edge)` so hovering an edge produces a chip.
func TestExplorer_D37k_FocusEdgeWiresChip(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _focusEdge(edge)")

	if !strings.Contains(body, "_showEdgeLabel(edge)") {
		t.Errorf("D37k: _focusEdge must call _showEdgeLabel(edge) — body:\n%s", body)
	}
}

// TestExplorer_D37k_ClearInteractionStateHidesChip pins that every
// interaction-state reset (mouseout, background tap, _focusNode
// taking over) hides the chip. This makes the chip hover-only.
func TestExplorer_D37k_ClearInteractionStateHidesChip(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _clearInteractionState()")

	if !strings.Contains(body, "_hideEdgeLabel()") {
		t.Errorf("D37k: _clearInteractionState must call _hideEdgeLabel() — body:\n%s", body)
	}
}

// TestExplorer_D37k_LayerSyncProjectsEdgeOverlay pins that the
// extended `_syncLayer` writes the SAME transform to the edge-label
// overlay as to the cards overlay. Two style writes, one transform
// computation, still O(1).
func TestExplorer_D37k_LayerSyncProjectsEdgeOverlay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _syncLayer()")

	for _, want := range []string{
		"_edgeLabelOverlayEl",
		"_edgeLabelOverlayEl.style",
		"els.transformOrigin = 'top left'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37k: _syncLayer must project the edge-label overlay — missing %q\nbody:\n%s", want, body)
		}
	}

	// D37f O(1) guard: still no iteration in the layer-tier body.
	for _, banned := range []string{
		"_htmlCardsByKey",
		"Object.keys(",
		"for (var i = 0",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37k: _syncLayer must remain O(1) — must NOT contain %q", banned)
		}
	}
}

// TestExplorer_D37k_CardsSyncRePositionsChip pins that the cards-
// tier sync (which fires on node `position` events during drag)
// also re-positions the chip so a dragged edge's chip tracks the
// moving midpoint.
func TestExplorer_D37k_CardsSyncRePositionsChip(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _syncCards()")

	if !strings.Contains(body, "_syncEdgeLabelPosition()") {
		t.Errorf("D37k: _syncCards must call _syncEdgeLabelPosition() so drag tracks the chip — body:\n%s", body)
	}
}

// TestExplorer_D37k_RenderPayloadInstallsOverlay pins that the
// edge-label overlay is installed inside the renderer's payload
// path, AFTER the cards overlay, so DOM order matches z-index
// intent.
func TestExplorer_D37k_RenderPayloadInstallsOverlay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)

	if !strings.Contains(js, "_installEdgeLabelOverlay(_cy, mount)") {
		t.Error("D37k: renderer must call `_installEdgeLabelOverlay(_cy, mount)` in the payload-render path")
	}
	// Install order: cards overlay first, edge overlay after.
	cardsIdx := strings.Index(js, "_installHtmlCardOverlay(_cy, mount, elements);")
	edgeIdx  := strings.Index(js, "_installEdgeLabelOverlay(_cy, mount);")
	if cardsIdx < 0 || edgeIdx < 0 {
		t.Fatalf("D37k: cannot find both install call sites — cards=%d, edge=%d", cardsIdx, edgeIdx)
	}
	if !(cardsIdx < edgeIdx) {
		t.Errorf("D37k: edge-label overlay install must come AFTER cards overlay install (got cards=%d, edge=%d)", cardsIdx, edgeIdx)
	}
}

// TestExplorer_D37k_DestroyHtmlCardOverlayTearsDownChip pins that
// the cards-overlay teardown also tears down the chip overlay so
// renderer re-renders / lens unmount leave no orphan DOM.
func TestExplorer_D37k_DestroyHtmlCardOverlayTearsDownChip(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	body := readD37kFunctionBody(t, js, "function _destroyHtmlCardOverlay()")

	if !strings.Contains(body, "_destroyEdgeLabelOverlay()") {
		t.Errorf("D37k: _destroyHtmlCardOverlay must call _destroyEdgeLabelOverlay() — body:\n%s", body)
	}
}

// TestExplorer_D37k_PublicDiagnosticSurface pins the diagnostic
// surface added to `cytoscapePoc` for runtime inspection / tests.
func TestExplorer_D37k_PublicDiagnosticSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)

	for _, want := range []string{
		"_installEdgeLabelOverlay:  _installEdgeLabelOverlay",
		"_destroyEdgeLabelOverlay:  _destroyEdgeLabelOverlay",
		"_showEdgeLabel:            _showEdgeLabel",
		"_hideEdgeLabel:            _hideEdgeLabel",
		"_syncEdgeLabelPosition:    _syncEdgeLabelPosition",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37k: diagnostic surface must expose %q", want)
		}
	}
}

// ── CSS contract ──────────────────────────────────────────────────

// TestExplorer_D37k_OverlayCssIsScoped pins that the new overlay
// and chip rules are scoped under the renderer-identity selector
// (the test pattern established by D35f /
// TestExplorer_CytoscapePoc_CSSScopedToActiveBodyClass).
func TestExplorer_D37k_OverlayCssIsScoped(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37kAuthorityCSSPath)

	for _, want := range []string{
		`.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-edge-label-overlay`,
		`.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-edge-label-chip`,
		`.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-edge-label-chip[hidden]`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37k: CSS must scope overlay rules under the renderer-identity selector — missing %q", want)
		}
	}
}

// TestExplorer_D37k_OverlayCssZIndexAboveCards pins that the
// overlay sits above the html-card overlay (cards' z-index 5).
func TestExplorer_D37k_OverlayCssZIndexAboveCards(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37kAuthorityCSSPath)

	overlayIdx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-edge-label-overlay`)
	if overlayIdx < 0 {
		t.Fatal("D37k: overlay rule missing")
	}
	openBrace  := strings.Index(css[overlayIdx:], "{")
	closeBrace := strings.Index(css[overlayIdx+openBrace:], "}")
	if openBrace < 0 || closeBrace < 0 {
		t.Fatal("D37k: cannot bound overlay rule")
	}
	block := css[overlayIdx+openBrace+1 : overlayIdx+openBrace+closeBrace]

	for _, want := range []string{
		"position: absolute",
		"inset: 0",
		"pointer-events: none",
		"transform-origin: top left",
		"z-index: 7",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37k: overlay rule must declare %q — block:\n%s", want, block)
		}
	}
}

// TestExplorer_D37k_ChipCssIsReadable pins the chip styling that
// makes it actually legible: 12px text, weight 500, max-width with
// overflow handling, full-contrast token, pointer-events: none.
func TestExplorer_D37k_ChipCssIsReadable(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37kAuthorityCSSPath)

	chipIdx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-edge-label-chip {`)
	if chipIdx < 0 {
		t.Fatal("D37k: chip rule missing")
	}
	openBrace  := strings.Index(css[chipIdx:], "{")
	closeBrace := strings.Index(css[chipIdx+openBrace:], "}")
	if openBrace < 0 || closeBrace < 0 {
		t.Fatal("D37k: cannot bound chip rule")
	}
	block := css[chipIdx+openBrace+1 : chipIdx+openBrace+closeBrace]

	for _, want := range []string{
		"font-size: 12px",
		"font-weight: 500",
		"max-width:",
		"overflow: hidden",
		"text-overflow: ellipsis",
		"pointer-events: none",
		"color: var(--on-surface,",
		"border-radius:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37k: chip rule must declare %q — block:\n%s", want, block)
		}
	}
}

// ── Foundation preservation ───────────────────────────────────────

// TestExplorer_D37k_PreservesD37fOverlayContracts is the foundation
// preservation check for D37f.
func TestExplorer_D37k_PreservesD37fOverlayContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)

	for _, want := range []string{
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
		"var PROJECTION_MODEL  = 'layer-pan-zoom-card-model-position'",
		"function _installHtmlCardOverlay(cy, mount, elements)",
		"function _syncLayer()",
		"function _syncCards()",
		"function _destroyHtmlCardOverlay()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37k: foundation D37f contract %q must remain", want)
		}
	}
}

// TestExplorer_D37k_PreservesD37jContextContracts pins D37j.
func TestExplorer_D37k_PreservesD37jContextContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)

	for _, want := range []string{
		"viewAuthorityContext:",
		"exitAuthorityContext:",
		"toggleAuthorityContext:",
		"isAuthorityContextActive:",
		"canViewAuthorityContext:",
		"function _computeAuthorityContext(cy, node)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37k: D37j authority-context contract %q must remain", want)
		}
	}
}

// TestExplorer_D37k_NoNewDependencyOrApiChange pins the brief's
// "no new dependency, no API/schema/projection change" boundary.
func TestExplorer_D37k_NoNewDependencyOrApiChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37kAuthorityShellAsset)
	exec := stripJSComments(js)

	for _, banned := range []string{
		"require('qtip",
		"require('popper",
		"require('@floating-ui",
		"import 'qtip",
		"import 'popper",
		"import '@floating-ui",
		// No fetch from the edge-label code path.
		"adapter.fetch({ view: 'decision_surface'",
		"adapter.fetch({ view: 'agent'",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D37k: no new dependency / API change permitted — found %q", banned)
		}
	}

	// Frontend caller still uses `view: 'service'` only.
	if !strings.Contains(js, "adapter.fetch({ view: 'service', id: rootId") {
		t.Error("D37k: frontend caller must still use `view: 'service'` (no API extension)")
	}
}

// TestExplorer_D37k_ToolbarUntouched pins that D37h camera-cluster
// controls were not modified.
func TestExplorer_D37k_ToolbarUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js")

	// All D37h public surface symbols still present.
	for _, want := range []string{
		"wire:",
		"refit:",
		"isWired:",
		"renderZoomPercent:",
		"syncZoomSelectedEnabled:",
		"ensureSubscriptions:",
		// D37j addition still present.
		"syncAuthorityContextButton:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37k: toolbar surface %q must remain (toolbar must not change in D37k)", want)
		}
	}
}
