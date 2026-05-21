package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37f-authority-html-card-overlay-implementation
//
// Migrates the Authority HTML-card overlay from the dormant pre-D37f
// theme-gated one-tier renderedPosition model to the proven D34i
// two-tier transform model, and makes HTML cards the default
// Authority visual.
//
// Three coordinated changes:
//   • CSS:
//       - `.cytoscape-poc-html-overlay` rule drops `overflow: hidden`
//         and adds `transform-origin: top left` (D34i requirement).
//       - All HTML-card CSS rules drop the `[data-cy-theme="html-card"]`
//         qualifier so they apply under default Authority activation.
//       - New `.cytoscape-poc-html-card.selected` rule for the
//         selected-state mirror.
//   • JS:
//       - `_installHtmlCardOverlay(cy, mount, elements)` (themeName
//         param retired); `_renderPayload` calls it unconditionally.
//       - New `LAYER_SYNC_EVENTS = 'pan zoom render resize'`,
//         `CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'`,
//         `PROJECTION_MODEL = 'layer-pan-zoom-card-model-position'`
//         constants.
//       - New `_syncLayer()` writes ONE transform on the overlay:
//         `translate(pan.x, pan.y) scale(zoom)` with
//         `transformOrigin = 'top left'`. MUST NOT iterate cards.
//       - New `_syncCards()` writes per-card transforms in MODEL
//         coords: `translate(p.x, p.y) translate(-50%, -50%)`.
//         Mirrors `n.selected()` onto `.selected` class. MUST NOT
//         use `renderedPosition` or `scale(`.
//       - Per-tier rAF coalescing (`_syncLayerRaf`, `_syncCardsRaf`).
//       - `_destroyHtmlCardOverlay` unbinds both `LAYER_SYNC_EVENTS`
//         and `CARDS_SYNC_EVENTS`; cancels both rAFs.
//   • DEFAULT_THEME promoted from `'classic'` to `'html-card'` so the
//     cy node footprint (240x96) matches the HTML-card overlay
//     footprint — necessary for `cy.fit()` correctness now that the
//     HTML overlay is the default visual.
//
// All D37b/D37d contracts are preserved unchanged. The legacy native
// Authority view (`authority-graph-view.js`) is unaffected.

const (
	d37fAuthorityShellAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37fAuthorityCSSPath    = "/explorer/assets/css/authority-cytoscape-poc.css"
	d37fHostAssetPath       = "/explorer/assets/js/graph/graph-viewport.js"
)

// readD37fInstallBody bounds the assertions to the
// `_installHtmlCardOverlay` body so unrelated code elsewhere does
// not false-match.
func readD37fInstallBody(t *testing.T, js string) string {
	t.Helper()
	start := strings.Index(js, "function _installHtmlCardOverlay(cy, mount, elements)")
	if start < 0 {
		t.Fatal("D37f: _installHtmlCardOverlay definition (cy, mount, elements) not found — themeName param should be retired in D37f")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37f: cannot bound _installHtmlCardOverlay body")
	}
	return js[start : start+end]
}

func readD37fSyncLayerBody(t *testing.T, js string) string {
	t.Helper()
	start := strings.Index(js, "function _syncLayer()")
	if start < 0 {
		t.Fatal("D37f: _syncLayer definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37f: cannot bound _syncLayer body")
	}
	return js[start : start+end]
}

func readD37fSyncCardsBody(t *testing.T, js string) string {
	t.Helper()
	start := strings.Index(js, "function _syncCards()")
	if start < 0 {
		t.Fatal("D37f: _syncCards definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37f: cannot bound _syncCards body")
	}
	return js[start : start+end]
}

func readD37fDestroyBody(t *testing.T, js string) string {
	t.Helper()
	start := strings.Index(js, "function _destroyHtmlCardOverlay()")
	if start < 0 {
		t.Fatal("D37f: _destroyHtmlCardOverlay definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37f: cannot bound _destroyHtmlCardOverlay body")
	}
	return js[start : start+end]
}

// TestExplorer_D37fAuthorityHtmlOverlay_RunsByDefault pins that the
// theme-gate is retired: `_renderPayload` calls
// `_installHtmlCardOverlay(_cy, mount, elements)` unconditionally
// (no `if (_activeTheme === 'html-card')` wrapper). Also pins
// DEFAULT_THEME is `'html-card'` so cy nodes are 240x96.
func TestExplorer_D37fAuthorityHtmlOverlay_RunsByDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	exec := stripJSComments(js)

	// D37f — unconditional install call site.
	if !strings.Contains(js, "_installHtmlCardOverlay(_cy, mount, elements);") {
		t.Error("D37f: _renderPayload must call _installHtmlCardOverlay(_cy, mount, elements); without theme-gate")
	}
	// D37f — pre-D37f theme-gate retired.
	if strings.Contains(exec, "if (_activeTheme === 'html-card') {") ||
		strings.Contains(exec, "_installHtmlCardOverlay(_cy, mount, elements, _activeTheme);") {
		t.Error("D37f: pre-D37f theme-gate `if (_activeTheme === 'html-card')` + 4-arg install call must be retired from executable code")
	}
	// D37f — DEFAULT_THEME promoted so cy node footprint matches.
	if !strings.Contains(js, "var DEFAULT_THEME  = 'html-card';") {
		t.Error("D37f: DEFAULT_THEME must be 'html-card' so cy node footprint (240x96) matches the HTML-card overlay footprint")
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_CreatesOverlayInsideCyMount
// pins that the overlay is appended to the Authority Cytoscape mount.
func TestExplorer_D37fAuthorityHtmlOverlay_CreatesOverlayInsideCyMount(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	body := readD37fInstallBody(t, js)

	for _, want := range []string{
		"document.createElement('div')",
		"_htmlOverlayEl.className = 'cytoscape-poc-html-overlay';",
		"mount.appendChild(_htmlOverlayEl)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f: install must create overlay inside cy mount — missing %q", want)
		}
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_OverlayIsPointerPassiveAndNonClipping
// pins the CSS contract: pointer-events:none, NO overflow:hidden,
// transform-origin: top left.
func TestExplorer_D37fAuthorityHtmlOverlay_OverlayIsPointerPassiveAndNonClipping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37fAuthorityCSSPath)
	exec := stripCSSComments(css)

	// Locate the active Authority overlay rule.
	idx := strings.Index(exec, `.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-html-overlay`)
	if idx < 0 {
		t.Fatal("D37f: active Authority overlay CSS rule missing")
	}
	openBrace := strings.Index(exec[idx:], "{")
	if openBrace < 0 {
		t.Fatal("D37f: cannot find `{` after overlay selector")
	}
	closeBrace := strings.Index(exec[idx+openBrace:], "}")
	if closeBrace < 0 {
		t.Fatal("D37f: cannot find matching `}` for overlay rule")
	}
	block := exec[idx+openBrace+1 : idx+openBrace+closeBrace]

	// Positive pins.
	if !strings.Contains(block, "pointer-events: none") {
		t.Errorf("D37f: overlay rule must declare `pointer-events: none` — body was:\n%s", block)
	}
	if !strings.Contains(block, "transform-origin: top left") {
		t.Errorf("D37f: overlay rule must declare `transform-origin: top left` (D34i requirement) — body was:\n%s", block)
	}

	// Negative pin — D35e/D37f disappearing-card bug must remain
	// retired.
	if strings.Contains(block, "overflow: hidden") {
		t.Errorf("D37f: overlay rule must NOT declare `overflow: hidden` (D35e/D37f fix — overlay is a projection layer, not a clip authority) — body was:\n%s", block)
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_PreservesAuthorityCardDesign
// pins that every per-kind selector + the card root + typography
// rules remain in place. The CSS selectors no longer require the
// `[data-cy-theme="html-card"]` qualifier — they apply under
// default Authority activation.
func TestExplorer_D37fAuthorityHtmlOverlay_PreservesAuthorityCardDesign(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37fAuthorityCSSPath)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)

	// CSS: per-kind selectors + card root + status row.
	for _, want := range []string{
		`.cytoscape-poc-html-card[data-kind="business_service"]`,
		`.cytoscape-poc-html-card[data-kind="decision_surface"]`,
		`.cytoscape-poc-html-card[data-kind="authority_profile"]`,
		`.cytoscape-poc-html-card[data-kind="authority_grant"]`,
		`.cytoscape-poc-html-card[data-kind="agent"]`,
		`.cytoscape-poc-html-card[data-kind="fail_mode_policy"]`,
		`.cytoscape-poc-html-card[data-kind="escalation_target"]`,
		`.cytoscape-poc-html-card[data-root="true"]`,
		".cytoscape-poc-html-card-kind",
		".cytoscape-poc-html-card-title",
		".cytoscape-poc-html-card-status",
		"width: 240px",
		"height: 96px",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37f: Authority HTML-card CSS must preserve %q", want)
		}
	}

	// D37f — `[data-cy-theme="html-card"]` qualifier dropped from CSS.
	cssExec := stripCSSComments(css)
	if strings.Contains(cssExec, `[data-cy-theme="html-card"]`) {
		t.Error("D37f: CSS rules must NOT scope under `[data-cy-theme=\"html-card\"]` — HTML cards are the default Authority visual")
	}

	// JS: _buildHtmlCard preserves the card DOM shape.
	for _, want := range []string{
		"var _knowledgeGraphRendererFactory", // unrelated sanity
	}[:0] {
		_ = want
	}
	for _, want := range []string{
		"function _buildHtmlCard(d)",
		"card.className = 'cytoscape-poc-html-card';",
		"card.setAttribute('data-node-id', d.id);",
		"card.setAttribute('data-kind', d.kind || '');",
		"if (d.isRoot) card.setAttribute('data-root', 'true');",
		"kindEl.className = 'cytoscape-poc-html-card-kind';",
		"titleEl.className = 'cytoscape-poc-html-card-title';",
		"statusEl.className = 'cytoscape-poc-html-card-status';",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37f: _buildHtmlCard must preserve card DOM shape — missing %q", want)
		}
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_UsesTwoTierTransform pins
// the two-tier transform shapes verbatim from the D34i model.
func TestExplorer_D37fAuthorityHtmlOverlay_UsesTwoTierTransform(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	layerBody := readD37fSyncLayerBody(t, js)
	cardsBody := readD37fSyncCardsBody(t, js)

	// LAYER tier — pan/zoom transform with transform-origin: top left.
	for _, want := range []string{
		"_cy.pan()",
		"_cy.zoom()",
		"'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')'",
		"transformOrigin = 'top left'",
		"_htmlOverlayEl.style",
	} {
		if !strings.Contains(layerBody, want) {
			t.Errorf("D37f: _syncLayer must include %q — body was:\n%s", want, layerBody)
		}
	}

	// CARDS tier — model coords + translate(-50%, -50%); mirrors selected.
	for _, want := range []string{
		"n.position()",
		"translate(-50%, -50%)",
		"'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)'",
		"card.style.transform",
		"n.selected()",
	} {
		if !strings.Contains(cardsBody, want) {
			t.Errorf("D37f: _syncCards must include %q — body was:\n%s", want, cardsBody)
		}
	}

	// D37f — Projection model identifier surfaced for diagnostics.
	if !strings.Contains(js, "var PROJECTION_MODEL  = 'layer-pan-zoom-card-model-position';") {
		t.Error("D37f: PROJECTION_MODEL diagnostic constant must be defined")
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_BindsLayerSyncEvents pins
// LAYER_SYNC_EVENTS = 'pan zoom render resize' constant + binding.
func TestExplorer_D37fAuthorityHtmlOverlay_BindsLayerSyncEvents(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)

	for _, want := range []string{
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"cy.on(LAYER_SYNC_EVENTS, _syncLayerBound)",
		"_syncLayerRaf",
		"_syncLayerBound",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37f: layer-tier wiring must include %q", want)
		}
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_BindsCardSyncEvents pins
// CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'
// constant + binding.
func TestExplorer_D37fAuthorityHtmlOverlay_BindsCardSyncEvents(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)

	for _, want := range []string{
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
		"cy.on(CARDS_SYNC_EVENTS, _syncCardsBound)",
		"_syncCardsRaf",
		"_syncCardsBound",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37f: cards-tier wiring must include %q", want)
		}
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_LayerSyncIsO1 pins that
// `_syncLayer` does NOT iterate `_htmlCardsByKey` — pan/zoom must
// cost O(1) per event regardless of node count.
func TestExplorer_D37fAuthorityHtmlOverlay_LayerSyncIsO1(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	body := readD37fSyncLayerBody(t, js)

	for _, banned := range []string{
		"_htmlCardsByKey",
		"Object.keys(",
		"for (var i = 0",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37f: _syncLayer must remain O(1) — must NOT contain %q", banned)
		}
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_CardSyncUsesModelPositionOnly
// pins that `_syncCards` uses MODEL coords (n.position()) only —
// no `renderedPosition` and no per-card `scale(`. The layer owns
// projection.
func TestExplorer_D37fAuthorityHtmlOverlay_CardSyncUsesModelPositionOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	body := readD37fSyncCardsBody(t, js)

	for _, banned := range []string{
		"renderedPosition",
		"renderedWidth",
		"renderedHeight",
		"scale(",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37f: _syncCards must NOT contain %q (layer owns projection)", banned)
		}
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_MirrorsSelectedState pins
// that `_syncCards` toggles `.selected` on the card when the
// corresponding cy node is selected.
func TestExplorer_D37fAuthorityHtmlOverlay_MirrorsSelectedState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	body := readD37fSyncCardsBody(t, js)

	for _, want := range []string{
		"n.selected()",
		"card.classList.add('selected')",
		"card.classList.remove('selected')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f: _syncCards must mirror selected state — missing %q", want)
		}
	}

	// CSS: .selected rule exists.
	css := getExplorerAsset(t, srv, d37fAuthorityCSSPath)
	if !strings.Contains(css, ".cytoscape-poc-html-card.selected") {
		t.Error("D37f: CSS must define `.cytoscape-poc-html-card.selected` rule for the selected-state visual")
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_DestroyRemovesOwnedDomAndListeners
// pins that `_destroyHtmlCardOverlay` cancels both rAFs, unbinds
// both event groups, and removes the overlay DOM.
func TestExplorer_D37fAuthorityHtmlOverlay_DestroyRemovesOwnedDomAndListeners(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	body := readD37fDestroyBody(t, js)

	for _, want := range []string{
		// Cancels both rAFs.
		"cancelAnimationFrame(_syncLayerRaf)",
		"cancelAnimationFrame(_syncCardsRaf)",
		// Unbinds both event groups.
		"_cy.off(LAYER_SYNC_EVENTS, _syncLayerBound)",
		"_cy.off(CARDS_SYNC_EVENTS, _syncCardsBound)",
		// Nulls bound handlers.
		"_syncLayerBound = null",
		"_syncCardsBound = null",
		// Removes overlay element + clears card map.
		"_htmlOverlayEl.parentNode.removeChild(_htmlOverlayEl)",
		"_htmlOverlayEl  = null;",
		"_htmlCardsByKey = {};",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f: _destroyHtmlCardOverlay must include %q — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_DoesNotBreakCytoscapeInteraction
// pins the pointer-passive contract: overlay + cards have
// `pointer-events: none` in CSS so Cytoscape continues to own all
// hit-testing.
func TestExplorer_D37fAuthorityHtmlOverlay_DoesNotBreakCytoscapeInteraction(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37fAuthorityCSSPath)

	// Overlay pointer-events:none — see _OverlayIsPointerPassiveAndNonClipping.
	// Card pointer-events:none — separate selector.
	if !strings.Contains(css, "pointer-events: none") {
		t.Error("D37f: card CSS must declare `pointer-events: none` so Cytoscape owns hit-testing")
	}
	// Count: both overlay and card rules should declare pointer-events:none.
	count := strings.Count(stripCSSComments(css), "pointer-events: none")
	if count < 2 {
		t.Errorf("D37f: pointer-events:none must appear on BOTH overlay and card rules (found %d in executable CSS)", count)
	}
}

// TestExplorer_D37fAuthorityHtmlOverlay_HtmlCardFootprintMatchesCyNodeFootprint
// pins that the html-card theme descriptor + the CSS card width/height
// agree on the 240x96 footprint (necessary for cy.fit() correctness
// now that the HTML overlay is the default).
func TestExplorer_D37fAuthorityHtmlOverlay_HtmlCardFootprintMatchesCyNodeFootprint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	css := getExplorerAsset(t, srv, d37fAuthorityCSSPath)

	// DEFAULT_THEME is 'html-card' so the html-card descriptor drives
	// cy node sizing.
	if !strings.Contains(js, "var DEFAULT_THEME  = 'html-card';") {
		t.Error("D37f: DEFAULT_THEME must be 'html-card' so cy node footprint matches HTML-card overlay")
	}
	// html-card theme descriptor pins 240x96 cy node footprint.
	htStart := strings.Index(js, "case 'html-card':")
	if htStart < 0 {
		t.Fatal("D37f: 'html-card' theme descriptor branch not found")
	}
	htEnd := strings.Index(js[htStart:], "case 'object-tile-v3':")
	if htEnd < 0 {
		t.Fatal("D37f: cannot bound 'html-card' descriptor")
	}
	htBody := js[htStart : htStart+htEnd]
	if !strings.Contains(htBody, "nodeW: 240") || !strings.Contains(htBody, "nodeH: 96") {
		t.Errorf("D37f: html-card descriptor must set nodeW: 240, nodeH: 96 — body was:\n%s", htBody)
	}

	// CSS card rule sets matching footprint.
	if !strings.Contains(css, "width: 240px") || !strings.Contains(css, "height: 96px") {
		t.Error("D37f: card CSS must declare 240x96 footprint matching the cy node footprint")
	}
}

// TestExplorer_D37f_D37bD37dContractsPreserved is the foundation-wide
// regression check for D37b + D37d invariants.
func TestExplorer_D37f_D37bD37dContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	css := getExplorerAsset(t, srv, d37fAuthorityCSSPath)

	// D37b — production renderer id + activation + aria-label + panel bridge.
	for _, want := range []string{
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
		`_mountEl.setAttribute('aria-label', 'Authority Graph')`,
		"window.MIDASExplorerGraph.authorityDiagnosticsPanel",
		"diagPanel.render(payload)",
		"window.MIDASExplorerGraph.authoritySurfacePosturePanel",
		"posturePanel.render(payload)",
		"window.MIDASExplorerGraph.authorityWorkbench",
		"workbenchMod.render()",
		"window.MIDASExplorerGraph._lastAuthorityProjection = payload",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37f: D37b Authority contract %q must remain", want)
		}
	}

	// D37d — mount positioning.
	cssExec := stripCSSComments(css)
	if !strings.Contains(cssExec, "position: absolute;") || !strings.Contains(cssExec, "inset: 0;") {
		t.Error("D37f: D37d mount positioning (position: absolute; inset: 0;) must remain")
	}

	// Pre-D37b user-facing PoC aria-label still retired.
	if strings.Contains(js, `'Authority Graph (Cytoscape PoC)'`) {
		t.Error("D37f: pre-D37b aria-label `Authority Graph (Cytoscape PoC)` must remain retired")
	}
}

// TestExplorer_D37f_D35D36ContractsPreserved pins the broader
// D35/D36 foundation contracts.
func TestExplorer_D37f_D35D36ContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a structural DOM.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f: D35a structural class %q must remain", want)
		}
	}

	hostJS := getExplorerAsset(t, srv, d37fHostAssetPath)
	hostExec := stripJSComments(hostJS)

	// D35b/c/f/g host API + registry-neutrality.
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport = {",
		"function register(rendererId, factory)",
		"function activateById(rendererId)",
		"function _setActiveRendererAttribute(rendererId)",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D37f: D35b/c/f/g host contract %q must remain", want)
		}
	}
	// graph-viewport.js remains renderer-neutral in executable code.
	for _, banned := range []string{
		"'authority'",
		`"authority"`,
		"'authority-cytoscape'",
		`"authority-cytoscape"`,
	} {
		if strings.Contains(hostExec, banned) {
			t.Errorf("D37f: graph-viewport.js executable code must remain renderer-neutral — found %q", banned)
		}
	}

	// D35f strategic clip rule + D35e Context overlay non-clipping.
	clipCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	if !strings.Contains(stripCSSComments(clipCSS), ".midas-graph-viewport") ||
		!strings.Contains(stripCSSComments(clipCSS), "overflow: hidden") {
		t.Error("D37f: `.midas-graph-viewport { overflow: hidden }` strategic clip must remain (D35f)")
	}
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if strings.Count(stripCSSComments(spikeCSS), "overflow: hidden") != 1 {
		t.Error("D37f: Context spike CSS must have exactly 1 `overflow: hidden` (mount; overlay non-clipping)")
	}

	// D36a Knowledge shell unaffected.
	knJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js")
	for _, want := range []string{
		"vp.register('knowledge-graph', _knowledgeGraphRendererFactory)",
		"vp.activateById(RENDERER_ID)",
	} {
		if !strings.Contains(knJS, want) {
			t.Errorf("D37f: D36a Knowledge shell contract %q must remain", want)
		}
	}
}

// TestExplorer_D37fAuthority_NoLegacyOneTierFallback pins the
// negative regression: the pre-D37f one-tier renderedPosition
// projection MUST NOT remain in the executable code of
// `_installHtmlCardOverlay` or `_updateHtmlCardOverlay` /
// `_syncCards`.
func TestExplorer_D37fAuthority_NoLegacyOneTierFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fAuthorityShellAsset)
	exec := stripJSComments(js)

	// Pre-D37f single-tier binding gone from executable code.
	if strings.Contains(exec, "cy.on('render pan zoom position', _htmlSyncBound)") {
		t.Error("D37f: pre-D37f single-tier event binding `cy.on('render pan zoom position', _htmlSyncBound)` must be retired")
	}
	if strings.Contains(exec, "_htmlSyncRaf") {
		t.Error("D37f: pre-D37f single rAF state `_htmlSyncRaf` must be retired (replaced by per-tier _syncLayerRaf + _syncCardsRaf)")
	}
	if strings.Contains(exec, "_htmlSyncBound") {
		t.Error("D37f: pre-D37f single bound handler `_htmlSyncBound` must be retired (replaced by per-tier _syncLayerBound + _syncCardsBound)")
	}
}
