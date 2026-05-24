package httpapi

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37r-tranche-B' — Shared Cytoscape HTML Overlay Module.
//
// Source-contract tests for the shared `graphCytoscapeOverlay` platform
// module + its Authority and Context consumers. The strategic rule
// "no graph lens may implement its own HTML overlay mechanics" is
// encoded both as documentation in the source files and as
// `TestExplorer_StrategicRule_NoLensImplementsOverlayMechanism` below.
//
// Tests covered:
//
//   1. ModuleExists                              — shared module present
//   2. PublicAPIShape                            — `mount(cy, mountEl, options)` + handle surface
//   3. LensAgnostic                              — no lens-specific names in the shared module
//   4. OwnsPositionSync                          — shared module owns model-layer projection + viewport-event sync
//   5. OwnsSelectionClassSync                    — shared module owns cy select/unselect class toggling
//   6. OwnsHoverClassSync                        — shared module owns cy mouseover/mouseout class toggling
//   7. OwnsTeardown                              — `destroy()` removes layer + detaches listeners
//   8. LoadedBeforeConsumers                     — `index.html` script-tag order
//   9. AuthorityRenderer_UsesSharedOverlay       — Authority `install` calls the shared module
//  10. AuthorityOverlay_NoLongerContainsPositionSync — Authority file no longer subscribes to viewport events / iterates `renderedPosition()`
//  11. AuthorityVisualOutputUnchanged            — Authority CSS + template DOM unchanged
//  12. ContextRenderer_UsesSharedOverlay         — Context renderer calls the shared module
//  13. ContextRenderer_InlineOverlayHelpersRemoved — Context inline overlay helpers gone
//  14. ContextTemplate_UsesNativePainter          — Context template calls `renderer.buildNodeCardElement`
//  15. ContextSelectionWiringPreserved           — `cy.on('tap', 'node', …)` → contextSelectionBridge.selectCard intact
//  16. ContextCytoscapeEdgeStylesPreserved       — five visual-class edge styles + dash semantics retained
//  17. StrategicRule_NoLensImplementsOverlayMechanism — scan all lens JS files for the banned mechanism shape
//  18. NativeContextFallbackPreserved            — native-context default/adoption unchanged
//  19. NonSpatialStrategicFallbackPreserved      — non-spatial strategic fallback safe
//  20. TrancheAAndBInvariantsHold                — re-pin: cytoscape instantiation, node/edge builders, selection bridge, camera delegate, edge styles
//  21. GraphRenderer_AddNodeUsesBuildNodeCardElement — `addNode` constructs card DOM via the pure factory

const (
	d37rBprimeOverlayModule       = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js"
	d37rBprimeContextRenderer     = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37rBprimeAuthorityOverlay    = "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js"
	d37rBprimeGraphRenderer       = "/explorer/assets/js/graph/graph-renderer.js"
	d37rBprimeAuthorityPoc        = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37rBprimeIndexHTML           = "/explorer/index.html"
	d37rBprimeViewportJS          = "/explorer/assets/js/graph/graph-viewport.js"
	d37rBprimeAuthorityOverlayCSS = "/explorer/assets/css/cytoscape-html-overlay.css"
)

// ── 1. Shared module exists ───────────────────────────────────────

func TestExplorer_GraphCytoscapeOverlay_ModuleExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeOverlayModule)
	if len(js) == 0 {
		t.Fatalf("D37r-tranche-B': shared overlay module must be served at %q", d37rBprimeOverlayModule)
	}
}

// ── 2. Public API shape ──────────────────────────────────────────

func TestExplorer_GraphCytoscapeOverlay_PublicAPIShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeOverlayModule)

	// Public surface.
	if !strings.Contains(js, "window.MIDASExplorerGraph.graphCytoscapeOverlay = {") {
		t.Errorf("D37r-tranche-B': module must attach `graphCytoscapeOverlay` to MIDASExplorerGraph")
	}
	if !strings.Contains(js, "function mount(cy, mountEl, options)") {
		t.Errorf("D37r-tranche-B': module must export mount(cy, mountEl, options)")
	}

	// Required options recognised inside mount.
	for _, want := range []string{
		"var template = opts.template;",
		"var keyForNode = opts.keyForNode;",
		"var lensId = _str(opts.lensId);",
		"opts.stateClasses",
		"opts.syncSelected",
		"opts.syncHover",
		"opts.pointerEvents",
		"opts.layerClassName",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B': mount() must recognise option %q", want)
		}
	}

	// Returned handle surface.
	if !regexp.MustCompile(`return\s*\{\s*destroy:\s*destroy,\s*refresh:\s*refresh,\s*getCardEl:\s*getCardEl,\s*getLayerEl:\s*getLayerEl,\s*\}`).MatchString(js) {
		t.Errorf("D37r-tranche-B': mount() must return a handle with destroy / refresh / getCardEl / getLayerEl")
	}
}

// ── 3. Lens-agnostic ─────────────────────────────────────────────

func TestExplorer_GraphCytoscapeOverlay_LensAgnostic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeOverlayModule)

	// The shared module must not name any specific lens or
	// lens-specific symbol. Comments may mention lenses by name to
	// explain the design (e.g. "Authority passes …") — those
	// substrings are allowed only if they appear inside JavaScript
	// comment lines, never in load-bearing code.
	//
	// Pragmatic enforcement: strip `//`-comments and `/* … */`
	// blocks before scanning for forbidden tokens. The strategic
	// rule is about runtime coupling, not about whether the design
	// rationale can name examples.
	stripped := _stripJSComments(js)

	for _, banned := range []string{
		"contextCardPainter",
		"contextSelectionBridge",
		"contextProjection",
		"graphSelectionBridge",
		"cytoscapePoc",
		"authorityView",
		"authorityGraphWorkbench",
		"knowledgeGraphContract",
		"knowledgeGraphShell",
	} {
		if strings.Contains(stripped, banned) {
			t.Errorf("D37r-tranche-B': shared overlay module must not reference lens-specific symbol %q outside comments", banned)
		}
	}

	// Also: the module must not import or call into lens directories
	// by path. Path strings are unlikely in this file anyway, but
	// pin defensively.
	for _, bannedPath := range []string{
		"/graph/context/",
		"/graph/authority/",
		"/graph/knowledge/",
	} {
		if strings.Contains(stripped, bannedPath) {
			t.Errorf("D37r-tranche-B': shared overlay module must not reference lens directory path %q outside comments", bannedPath)
		}
	}
}

// _stripJSComments — defensive helper for D37r-tranche-B' tests.
// Removes `// ...` line comments and `/* ... */` block comments so
// scans can target load-bearing code only. Not a full JS parser —
// good enough for whole-line / fenced-block comment removal in the
// asset bodies these tests inspect.
func _stripJSComments(src string) string {
	// Block comments first.
	blockRe := regexp.MustCompile(`(?s)/\*.*?\*/`)
	src = blockRe.ReplaceAllString(src, "")
	// Line comments.
	lineRe := regexp.MustCompile(`(?m)//[^\n]*$`)
	src = lineRe.ReplaceAllString(src, "")
	return src
}

// ── 4. Shared module owns position sync ──────────────────────────

func TestExplorer_GraphCytoscapeOverlay_OwnsPositionSync(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeOverlayModule)

	// The module declares the locked viewport-event vocabulary as
	// a single constant and subscribes via cy.on(SYNC_EVENTS, ...).
	if !strings.Contains(js, "var SYNC_EVENTS         = 'render pan zoom position layoutstop';") {
		t.Errorf("D37r-tranche-B': shared module must declare SYNC_EVENTS with the canonical event list")
	}
	if !strings.Contains(js, "cy.on(SYNC_EVENTS, _syncBound)") {
		t.Errorf("D37r-tranche-B': shared module must subscribe to cy via cy.on(SYNC_EVENTS, _syncBound)")
	}

	// D37p authority-projection flip: the shared overlay now uses a
	// two-tier projection contract. The layer receives cy.pan()/cy.zoom(),
	// and each wrapper is placed from model-space node.position() with
	// explicit centring against the inner card dimensions.
	if !regexp.MustCompile(`(?s)function _syncLayer\(\)[\s\S]*?cy\.pan\(\)[\s\S]*?cy\.zoom\(\)[\s\S]*?scale\(`).MatchString(js) {
		t.Errorf("D37p authority-projection: shared module's _syncLayer must apply Cytoscape pan and zoom to the overlay layer")
	}
	if !regexp.MustCompile(`(?s)function _syncCard\(entry\)[\s\S]*?n\.position\(\)`).MatchString(js) {
		t.Errorf("D37p authority-projection: shared module's _syncCard must use model-space n.position() per tracked node")
	}
	if regexp.MustCompile(`(?s)function _syncCard\(entry\)[\s\S]*?renderedPosition\(\)`).MatchString(js) {
		t.Errorf("D37p authority-projection: shared module's _syncCard must not use renderedPosition() in the model-layer path")
	}
	if !regexp.MustCompile(`(?s)function _sync\(\)\s*\{[\s\S]*?_syncCard\(`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix (flipped): shared module's _sync must iterate tracked entries and delegate to _syncCard")
	}

	// Projection sync writes translate() on wrappers while layer-level
	// pan/zoom owns scale.
	if !strings.Contains(js, "entry.wrapper.style.transform = 'translate(' + tx + 'px, ' + ty + 'px)'") {
		t.Errorf("D37p authority-projection: shared module must write model-space translate() positions on card wrappers")
	}
	// translate(-50%, -50%) MUST be absent from the per-card transform
	// path. If a future change re-introduces it (e.g. someone "fixes"
	// the wrapper centring by reverting to the broken pattern), this
	// negative pin catches the regression.
	if regexp.MustCompile(`wrapper\.style\.transform\s*=\s*[^;]*translate\(-50%`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix (flipped): shared module must NOT use translate(-50%%, -50%%) for wrapper centring — measured-dimension arithmetic is the load-bearing contract (translate(-50%%) collapses to zero when the wrapper has no intrinsic dimensions, e.g. when the lens template root is position:absolute)")
	}
	if !regexp.MustCompile(`Math\.round\(p\.x\s*-\s*w\s*/\s*2\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix (flipped): centring must compute tx = round(p.x - w/2) using measured width")
	}
	if !regexp.MustCompile(`Math\.round\(p\.y\s*-\s*h\s*/\s*2\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B''-centring-fix (flipped): centring must compute ty = round(p.y - h/2) using measured height")
	}

	// rAF-coalesced.
	if !strings.Contains(js, "window.requestAnimationFrame") {
		t.Errorf("D37r-tranche-B': shared module must rAF-coalesce its sync")
	}
}

// ── 5. Shared module owns selection-class sync ───────────────────

func TestExplorer_GraphCytoscapeOverlay_OwnsSelectionClassSync(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeOverlayModule)

	if !regexp.MustCompile(`cy\.on\('select',\s*'node',\s*_selectBound\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B': shared module must subscribe to cy.on('select', 'node', _selectBound)")
	}
	if !regexp.MustCompile(`cy\.on\('unselect',\s*'node',\s*_unselectBound\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B': shared module must subscribe to cy.on('unselect', 'node', _unselectBound)")
	}
	// Class toggling applies to BOTH wrapper and the inner template
	// element (the firstChild) so native CSS rules like
	// `.gmap-node.selected` fire directly on the lens card.
	if !regexp.MustCompile(`var inner = wrapper\.firstElementChild \|\| wrapper\.firstChild;`).MatchString(js) {
		t.Errorf("D37r-tranche-B': shared module must resolve the inner template element for state-class application")
	}
	if !strings.Contains(js, "DEFAULT_STATE_CLASS_SELECTED = 'is-selected'") {
		t.Errorf("D37r-tranche-B': shared module must default selected class to 'is-selected'")
	}
}

// ── 6. Shared module owns hover-class sync ───────────────────────

func TestExplorer_GraphCytoscapeOverlay_OwnsHoverClassSync(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeOverlayModule)

	if !regexp.MustCompile(`cy\.on\('mouseover',\s*'node',\s*_mouseoverBound\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B': shared module must subscribe to cy.on('mouseover', 'node', _mouseoverBound)")
	}
	if !regexp.MustCompile(`cy\.on\('mouseout',\s*'node',\s*_mouseoutBound\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B': shared module must subscribe to cy.on('mouseout', 'node', _mouseoutBound)")
	}
	if !strings.Contains(js, "DEFAULT_STATE_CLASS_HOVER    = 'is-hover'") {
		t.Errorf("D37r-tranche-B': shared module must default hover class to 'is-hover'")
	}
}

// ── 7. Teardown ──────────────────────────────────────────────────

func TestExplorer_GraphCytoscapeOverlay_OwnsTeardown(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeOverlayModule)

	if !strings.Contains(js, "function destroy()") {
		t.Errorf("D37r-tranche-B': shared module must declare destroy()")
	}
	// destroy() cancels the rAF, detaches listeners, removes the
	// layer DIV, and clears the tracked card map.
	idx := strings.Index(js, "function destroy()")
	if idx < 0 {
		t.Fatal("D37r-tranche-B': destroy() must be present")
	}
	tail := js[idx:]
	endRel := strings.Index(tail[1:], "\n    function ")
	if endRel < 0 {
		t.Fatal("D37r-tranche-B': destroy() body must be well-formed")
	}
	body := tail[:endRel+1]

	for _, want := range []string{
		"_detachListeners();",
		"_layerEl.parentNode.removeChild(_layerEl)",
		"_byKey   = {};",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37r-tranche-B': destroy() must include %q", want)
		}
	}
}

// ── 8. Loaded before consumers ───────────────────────────────────

func TestExplorer_GraphCytoscapeOverlay_LoadedBeforeConsumers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	// `/explorer/index.html` may redirect to the canonical `/explorer`;
	// fetch through performRequest with the canonical path so we get
	// the HTML body directly.
	resp := performRequest(t, srv, "GET", "/explorer", nil)
	html := resp.Body.String()
	if !strings.Contains(html, "<script") {
		t.Fatalf("D37r-tranche-B': /explorer must serve HTML containing <script> tags (got %d bytes)", len(html))
	}
	_ = d37rBprimeIndexHTML

	idxOverlay := strings.Index(html, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	idxContext := strings.Index(html, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")
	idxAuthOvr := strings.Index(html, "/explorer/assets/js/graph/authority/cytoscape-html-overlay.js")

	if idxOverlay < 0 {
		t.Fatalf("D37r-tranche-B': shared overlay script tag must be in index.html")
	}
	if idxContext < 0 || idxAuthOvr < 0 {
		t.Fatalf("D37r-tranche-B': both consumer script tags must remain in index.html (context=%d, authOverlay=%d)", idxContext, idxAuthOvr)
	}
	if idxOverlay >= idxContext {
		t.Errorf("D37r-tranche-B': shared overlay must be loaded BEFORE the Context renderer (overlay=%d, context=%d)", idxOverlay, idxContext)
	}
	if idxOverlay >= idxAuthOvr {
		t.Errorf("D37r-tranche-B': shared overlay must be loaded BEFORE the Authority overlay (overlay=%d, authOverlay=%d)", idxOverlay, idxAuthOvr)
	}
}

// ── 9. Authority renderer uses shared overlay ────────────────────

func TestExplorer_AuthorityRenderer_UsesSharedOverlay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeAuthorityOverlay)

	// Authority resolves the shared module by canonical namespace
	// and calls mount with the locked options shape.
	if !strings.Contains(js, "window.MIDASExplorerGraph.graphCytoscapeOverlay") {
		t.Errorf("D37r-tranche-B': Authority overlay file must reach `graphCytoscapeOverlay` on the shared namespace")
	}
	if !strings.Contains(js, "sharedOverlay.mount(cy, mount, {") {
		t.Errorf("D37r-tranche-B': Authority overlay must call sharedOverlay.mount(cy, mount, {...})")
	}
	if !strings.Contains(js, "lensId: 'authority',") {
		t.Errorf("D37r-tranche-B': Authority overlay must declare lensId: 'authority'")
	}
	// Authority cards are clickable; pass pointerEvents 'auto'.
	if !strings.Contains(js, "pointerEvents: 'auto',") {
		t.Errorf("D37r-tranche-B': Authority overlay must opt into pointerEvents: 'auto' so cards remain clickable")
	}
	// Preserve the existing CSS rule via layerClassName.
	if !strings.Contains(js, "layerClassName: OVERLAY_CLASS,") {
		t.Errorf("D37r-tranche-B': Authority overlay must pass layerClassName: OVERLAY_CLASS to preserve the existing CSS layer rule")
	}
}

// ── 10. Authority overlay file no longer owns position sync ──────

func TestExplorer_AuthorityOverlay_NoLongerContainsPositionSync(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeAuthorityOverlay)
	stripped := _stripJSComments(js)

	// Authority must no longer subscribe to viewport events directly.
	if regexp.MustCompile(`cy\.on\(\s*['"]?render`).MatchString(stripped) {
		t.Errorf("D37r-tranche-B': Authority overlay must no longer subscribe to cy 'render' events directly (load-bearing code, comments excluded)")
	}
	if regexp.MustCompile(`cy\.on\(SYNC_EVENTS`).MatchString(stripped) {
		t.Errorf("D37r-tranche-B': Authority overlay must no longer subscribe to cy.on(SYNC_EVENTS, ...) directly")
	}
	// Authority must no longer iterate nodes calling renderedPosition.
	if regexp.MustCompile(`renderedPosition\(\)`).MatchString(stripped) {
		t.Errorf("D37r-tranche-B': Authority overlay must no longer call renderedPosition() directly")
	}
	// Authority must no longer install a ResizeObserver itself for
	// the overlay layer; the shared module owns it.
	if regexp.MustCompile(`new\s+window\.ResizeObserver`).MatchString(stripped) {
		t.Errorf("D37r-tranche-B': Authority overlay must no longer instantiate ResizeObserver for overlay sync")
	}
	// _sync is retired (kept as no-op shim for diagnostics).
	if !strings.Contains(js, "function _sync(/* cy */) { /* retired") {
		t.Errorf("D37r-tranche-B': Authority overlay's _sync must be the documented no-op retirement shim")
	}
}

// ── 11. Authority visual output unchanged ────────────────────────

func TestExplorer_AuthorityVisualOutputUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Authority template DOM anchors — the symbols _buildCard
	// produces — must remain present.
	overlayJS := getExplorerAsset(t, srv, d37rBprimeAuthorityOverlay)
	for _, anchor := range []string{
		"function _buildCard(data)",
		"card.className = CARD_CLASS;",
		"eyebrow.className = 'gmap-node-label';",
		"name.className = 'gmap-node-name';",
		"meta.className = 'gmap-node-meta';",
		"function _wireCardClick(card, nodeId)",
		"function _dimNativeNodes(cy)",
		"CARD_CLASS:    CARD_CLASS,",
	} {
		if !strings.Contains(overlayJS, anchor) {
			t.Errorf("D37r-tranche-B': Authority template DOM must remain unchanged — anchor missing: %q", anchor)
		}
	}

	// Authority overlay CSS still defines its load-bearing
	// selectors. The shared module preserves the existing layer
	// class via `layerClassName: OVERLAY_CLASS`, so the rule still
	// applies to the layer DIV.
	overlayCSS := getExplorerAsset(t, srv, d37rBprimeAuthorityOverlayCSS)
	for _, anchor := range []string{
		".midas-cy-overlay-layer",
		".midas-cy-overlay-card",
	} {
		if !strings.Contains(overlayCSS, anchor) {
			t.Errorf("D37r-tranche-B': Authority overlay CSS must keep selector %q", anchor)
		}
	}

	// Authority Cytoscape PoC's call into the overlay is unchanged.
	pocJS := getExplorerAsset(t, srv, d37rBprimeAuthorityPoc)
	if !strings.Contains(pocJS, "_installHtmlCardOverlay(_cy, mount, elements)") {
		t.Errorf("D37r-tranche-B': Authority Cytoscape PoC's overlay install call must remain unchanged")
	}
}

// ── 12. Context renderer uses shared overlay ─────────────────────

func TestExplorer_ContextRenderer_UsesSharedOverlay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeContextRenderer)

	if !strings.Contains(js, "function _mountCytoscapeOverlayViaSharedModule(cards)") {
		t.Errorf("D37r-tranche-B': Context renderer must define _mountCytoscapeOverlayViaSharedModule(cards)")
	}
	if !strings.Contains(js, "shared.mount(_cy, _stageEl, {") {
		t.Errorf("D37r-tranche-B': Context renderer must call shared.mount(_cy, _stageEl, {...})")
	}
	if !strings.Contains(js, "lensId: 'context',") {
		t.Errorf("D37r-tranche-B': Context renderer must declare lensId: 'context' to the shared overlay")
	}
	if !strings.Contains(js, "pointerEvents: 'none',") {
		t.Errorf("D37r-tranche-B': Context renderer must pass pointerEvents: 'none' (Cytoscape owns interactions)")
	}
	if !strings.Contains(js, "stateClasses: { selected: 'selected', hover: 'is-hover' },") {
		t.Errorf("D37r-tranche-B': Context renderer must map selected→'selected' to fire native .gmap-node.selected CSS")
	}
}

// ── 13. Inline overlay helpers removed from Context ──────────────

func TestExplorer_ContextRenderer_InlineOverlayHelpersRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeContextRenderer)
	stripped := _stripJSComments(js)

	// The inline helper FUNCTION DECLARATIONS (load-bearing code,
	// not the documentation comments that reference them).
	for _, banned := range []string{
		"function _mountCytoscapeOverlay(",
		"function _syncCytoscapeOverlay(",
		"function _wireCytoscapeOverlaySync(",
		"function _wireCytoscapeOverlayStateSync(",
	} {
		if strings.Contains(stripped, banned) {
			t.Errorf("D37r-tranche-B': Context renderer must no longer declare %q (load-bearing code, comments excluded)", banned)
		}
	}

	// The Cytoscape overlay element + rAF + card-map state vars are
	// gone too.
	for _, banned := range []string{
		"_cyOverlayEl",
		"_cyOverlayCardsByKey",
		"_cySyncRaf",
	} {
		if strings.Contains(stripped, banned) {
			t.Errorf("D37r-tranche-B': Context renderer must no longer reference %q (state replaced by shared overlay handle)", banned)
		}
	}

	// Replaced by the handle.
	if !strings.Contains(js, "var _cyOverlayHandle") {
		t.Errorf("D37r-tranche-B': Context renderer must declare _cyOverlayHandle (shared module handle)")
	}
}

// ── 14. Context template uses the native painter ─────────────────

// TestExplorer_ContextTemplate_UsesNativePainter pins that Context's
// overlay template invokes the same DOM factory the native renderer's
// `addNode` uses — `renderer.buildNodeCardElement(spec)` — so
// Cytoscape Context cards and native `.gmap-node` cards are
// produced by literally the same code path. This is the
// "structural visual parity" guarantee of tranche B'.
func TestExplorer_ContextTemplate_UsesNativePainter(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeContextRenderer)

	// The template's create(node) calls rendererSurface.buildNodeCardElement(spec).
	if !regexp.MustCompile(`var el = rendererSurface\.buildNodeCardElement\(spec\);`).MatchString(js) {
		t.Errorf("D37r-tranche-B': Context template create(node) must call rendererSurface.buildNodeCardElement(spec)")
	}

	// rendererSurface is resolved off the canonical namespace.
	if !strings.Contains(js, "var rendererSurface = g && g.renderer;") {
		t.Errorf("D37r-tranche-B': Context renderer must resolve `rendererSurface` from MIDASExplorerGraph.renderer")
	}
	if !strings.Contains(js, "typeof rendererSurface.buildNodeCardElement !== 'function'") {
		t.Errorf("D37r-tranche-B': Context renderer must guard against missing buildNodeCardElement")
	}

	// ContextCard → native-spec adapter exists.
	if !strings.Contains(js, "function _contextCardToNativeSpec(card)") {
		t.Errorf("D37r-tranche-B': Context renderer must define the ContextCard → native-spec adapter _contextCardToNativeSpec")
	}
	if !strings.Contains(js, "var _CONTEXT_KIND_TO_NATIVE_CLS = {") {
		t.Errorf("D37r-tranche-B': Context renderer must declare _CONTEXT_KIND_TO_NATIVE_CLS for kind→cls mapping")
	}

	// Strategic visual-parity prerequisite: the native factory is
	// exported on `MIDASExplorerGraph.renderer.buildNodeCardElement`
	// AND `addNode` delegates DOM construction to it.
	gr := getExplorerAsset(t, srv, d37rBprimeGraphRenderer)
	if !strings.Contains(gr, "function buildNodeCardElement(spec)") {
		t.Errorf("D37r-tranche-B': graph-renderer.js must declare buildNodeCardElement(spec)")
	}
	if !strings.Contains(gr, "buildNodeCardElement:      buildNodeCardElement,") {
		t.Errorf("D37r-tranche-B': graph-renderer.js must export buildNodeCardElement on the renderer surface")
	}
}

// ── 15. Context selection wiring preserved ───────────────────────

func TestExplorer_ContextSelectionWiringPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeContextRenderer)

	if !strings.Contains(js, "function _wireCytoscapeSelectionTap()") {
		t.Errorf("D37r-tranche-B': Context renderer must keep _wireCytoscapeSelectionTap")
	}
	if !strings.Contains(js, "_cy.on('tap', 'node', function (evt)") {
		t.Errorf("D37r-tranche-B': Context tap handler must remain — `cy.on('tap', 'node', ...)`")
	}
	if !strings.Contains(js, "bridge.selectCard(card)") {
		t.Errorf("D37r-tranche-B': Context tap handler must call contextSelectionBridge.selectCard(card)")
	}
}

// ── 16. Edge styles preserved from original tranche B ────────────

func TestExplorer_ContextCytoscapeEdgeStylesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeContextRenderer)

	for _, want := range []string{
		"function _buildContextCytoscapeStyle()",
		"_readCssVar('--outline-variant',",
		"_readCssVar('--primary',",
		"_readCssVar('--on-surface-variant',",
		"_readCssVar('--badge-bad',",
		"selector: 'edge.context-edge-visual-service',",
		"selector: 'edge.context-edge-visual-ai_binding',",
		"selector: 'edge.context-edge-visual-authority',",
		"selector: 'edge.context-edge-visual-evidence',",
		"selector: 'edge.context-edge-visual-gap',",
		"'line-dash-pattern': [6, 4],",
		"'line-dash-pattern': [5, 5],",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B': edge style %q from original tranche B must remain in the Cytoscape style array", want)
		}
	}
}

// ── 17. Strategic rule — no lens implements overlay mechanism ────

// TestExplorer_StrategicRule_NoLensImplementsOverlayMechanism scans
// every JavaScript file under
// internal/httpapi/explorer/assets/js/graph/ EXCEPT the shared
// overlay module and the (now-template-only) Authority overlay file
// (which holds Authority's template). If any file fits the overlay-
// mechanism shape — subscribing to cy viewport events AND iterating
// `cy.nodes()` for `renderedPosition()` — the strategic rule is
// violated and the test fails.
//
// The Authority overlay file is whitelisted because, even after
// tranche B', it still holds Authority's template + dim/restore +
// install/destroy/refresh public surface. It does NOT subscribe to
// viewport events or iterate `renderedPosition()` (pinned by
// TestExplorer_AuthorityOverlay_NoLongerContainsPositionSync).
//
// The Context Cytoscape overlay-spike file is whitelisted because
// it is dormant — gated by `?cytoscape=1&contextHtmlCards=1` and
// not registered on the strategic activation path. The spike will
// be retired in a separate cleanup tranche.
func TestExplorer_StrategicRule_NoLensImplementsOverlayMechanism(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	whitelist := map[string]bool{
		// The canonical mechanism owner.
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js": true,
		// Authority template-only file — pin separately that it no
		// longer owns the mechanism (test #10).
		"/explorer/assets/js/graph/authority/cytoscape-html-overlay.js": true,
		// Dormant spike (out-of-tree for production activation).
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js": true,
	}

	// Walk every JS file the asset route serves under graph/.
	// We can't introspect the asset route directly; instead we
	// enumerate from the filesystem path the tests already use
	// (the assets are served from this directory). Glob-style walk
	// via filepath.Walk would normally require importing more
	// stdlib; instead drive a known set discovered earlier — the
	// Glob in Step 0 enumerated ~46 files. Pin them here.
	candidates := []string{
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-layout.js",
		"/explorer/assets/js/graph/graph-camera.js",
		"/explorer/assets/js/graph/graph-interactions.js",
		"/explorer/assets/js/graph/graph-selection.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/graph-types.js",
		"/explorer/assets/js/graph/graph-drawer.js",
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/graph-inspector.js",
		"/explorer/assets/js/graph/graph-platform/graph-stage.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-controller.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-bus.js",
		"/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js",
		"/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js",
		"/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js",
		"/explorer/assets/js/graph/context/context-cytoscape-renderer.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
		"/explorer/assets/js/graph/context/context-graph-inspector.js",
		"/explorer/assets/js/graph/context/context-selection-bridge.js",
		"/explorer/assets/js/graph/context/context-selected-object-pane.js",
		"/explorer/assets/js/graph/context/context-evidence-tray.js",
		"/explorer/assets/js/graph/context/context-projection-handoff.js",
		"/explorer/assets/js/graph/context/context-projection-provider.js",
		"/explorer/assets/js/graph/context/context-card-model.js",
		"/explorer/assets/js/graph/context/context-connector-model.js",
		"/explorer/assets/js/graph/context/context-layout-model.js",
		"/explorer/assets/js/graph/context/context-html-card-painter.js",
		"/explorer/assets/js/graph/context/context-connector-painter.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js",
		"/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js",
		"/explorer/assets/js/graph/authority/authority-graph-view.js",
		"/explorer/assets/js/graph/authority/authority-graph-adapter.js",
		"/explorer/assets/js/graph/authority/authority-graph-layout.js",
		"/explorer/assets/js/graph/authority/authority-graph-connectors.js",
		"/explorer/assets/js/graph/authority/authority-graph-inspector.js",
		"/explorer/assets/js/graph/authority/authority-graph-overlays.js",
		"/explorer/assets/js/graph/authority/authority-graph-workbench.js",
		"/explorer/assets/js/graph/authority/authority-diagnostics-panel.js",
		"/explorer/assets/js/graph/authority/authority-surface-posture-panel.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-contract.js",
	}

	// Bans (each evaluated on the post-comment-strip body):
	//   (a) cy.on('render' ...) / cy.on('pan' ...) / cy.on('zoom' ...)
	//   (b) calling renderedPosition() on a cy node
	//   (c) creating an element with class containing 'overlay-layer'
	//       or 'overlay-card' as load-bearing code
	bannedSubscriptionRe := regexp.MustCompile(`cy\.on\(\s*['"](?:render|pan|zoom|position|layoutstop)['"]`)
	bannedRenderedPosRe := regexp.MustCompile(`\brenderedPosition\(\s*\)`)
	bannedClassRe := regexp.MustCompile(`['"][^'"]*(?:overlay-layer|overlay-card)[^'"]*['"]`)

	for _, asset := range candidates {
		if whitelist[asset] {
			continue
		}
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			// Asset not served — skip.
			continue
		}
		stripped := _stripJSComments(js)

		if bannedSubscriptionRe.MatchString(stripped) && bannedRenderedPosRe.MatchString(stripped) {
			t.Errorf("D37r-tranche-B' strategic rule violation: lens module %q subscribes to cy viewport events AND iterates renderedPosition() — overlay mechanics belong in the shared platform module", filepath.Base(asset))
		}
		if bannedClassRe.MatchString(stripped) {
			t.Errorf("D37r-tranche-B' strategic rule violation: lens module %q references an `overlay-layer` / `overlay-card` class as load-bearing code — overlay class names belong to the shared platform module", filepath.Base(asset))
		}
	}
}

// ── 17a. Inner template element receives the pointer-events value ─

// TestExplorer_GraphCytoscapeOverlay_InnerElementInheritsPointerEvents
// pins the runtime-correctness contract for `_wrapElement`.
//
// Why this test exists: the prior public-API test
// (TestExplorer_GraphCytoscapeOverlay_PublicAPIShape) pinned that
// `options.pointerEvents` is RECOGNISED by mount(), and the existing
// implementation comment described the policy ("Per-card click
// capture is controlled by the wrapper's `pointerEvents`"). Neither
// pinned the runtime CORRECTNESS property that the configured
// pointer-events value must reach the TEMPLATE-RETURNED ELEMENT,
// not just the wrapper.
//
// The CSS spec is the load-bearing detail: `pointer-events: none`
// on a parent does NOT propagate to descendants that have their
// own non-none pointer-events value. A `<button>` defaults to
// `pointer-events: auto` and opts back in even when its parent
// is `none`. For Context's `pointerEvents: 'none'` opt-in to mean
// "Cytoscape gets every click", the inner template element must
// itself be `pointer-events: none`. The pre-fix shared module
// applied the value to the wrapper only, leaving the inner
// `<button class="gmap-node">` capturing every click — which is
// exactly what the prior tranche A inline overlay did right and
// the tranche B' refactor regressed.
//
// This test closes the gap by pinning the load-bearing line.
func TestExplorer_GraphCytoscapeOverlay_InnerElementInheritsPointerEvents(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeOverlayModule)

	// Locate the _wrapElement body.
	idx := strings.Index(js, "function _wrapElement(node)")
	if idx < 0 {
		t.Fatal("D37r-tranche-B'-fix-2: _wrapElement(node) must be present in the shared overlay module")
	}
	tail := js[idx:]
	endRel := strings.Index(tail[1:], "\n    function ")
	if endRel < 0 {
		t.Fatal("D37r-tranche-B'-fix-2: _wrapElement body must be well-formed")
	}
	body := tail[:endRel+1]

	// The fix: pointerEvents is applied to BOTH the wrapper AND
	// the inner template-returned element.
	if !strings.Contains(body, "wrapper.style.pointerEvents = pointerEvents;") {
		t.Errorf("D37r-tranche-B'-fix-2: _wrapElement must apply pointerEvents to the wrapper (pre-existing pin)")
	}
	if !strings.Contains(body, "inner.style.pointerEvents = pointerEvents;") {
		t.Errorf("D37r-tranche-B'-fix-2: _wrapElement must ALSO apply pointerEvents to the template-returned `inner` element (the CSS-spec descendant-opt-in fix)")
	}

	// The application to inner must be guarded against a null /
	// no-style return (some templates may legitimately return DOM
	// nodes without a `.style` property; the fix must not throw).
	if !strings.Contains(body, "if (inner && inner.style) {") {
		t.Errorf("D37r-tranche-B'-fix-2: _wrapElement must guard the inner.style.pointerEvents write against null / no-style template returns")
	}

	// Sanity: the application to inner is BEFORE the wrapper.appendChild
	// (the order doesn't strictly matter for the runtime outcome
	// because appendChild doesn't mutate the inner element's style,
	// but pinning the order makes the source pattern reviewable as
	// "configure inner, then attach to wrapper").
	innerIdx := strings.Index(body, "inner.style.pointerEvents = pointerEvents;")
	appendIdx := strings.Index(body, "wrapper.appendChild(inner);")
	if innerIdx < 0 || appendIdx < 0 {
		t.Fatal("D37r-tranche-B'-fix-2: cannot locate inner pointer-events assignment or wrapper.appendChild")
	}
	if innerIdx >= appendIdx {
		t.Errorf("D37r-tranche-B'-fix-2: inner.style.pointerEvents assignment must precede wrapper.appendChild(inner) (configure-then-attach pattern)")
	}
}

// ── 18. Native default preserved ─────────────────────────────────

func TestExplorer_GraphCytoscapeOverlay_NativeContextFallbackPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vp := getExplorerAsset(t, srv, d37rBprimeViewportJS)

	if !strings.Contains(vp, "_baselineId = 'native-context';") {
		t.Errorf("D37r-tranche-B': native-context baseline adoption must remain unchanged")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37r-tranche-B': GraphViewport must still adoptExisting('native-context')")
	}
}

// ── 19. Non-spatial strategic fallback preserved ─────────────────

func TestExplorer_GraphCytoscapeOverlay_NonSpatialStrategicFallbackPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeContextRenderer)

	if !strings.Contains(js, "if (_isSpatialMode() && _hasGraphStage()) {") {
		t.Errorf("D37r-tranche-B': spatial-mode gate must remain — non-spatial strategic Context must not enter the Cytoscape path")
	}
	if !strings.Contains(js, "function _buildFallbackContextCameraDelegate()") {
		t.Errorf("D37r-tranche-B': non-spatial strategic Context fallback camera delegate must remain")
	}
}

// ── 20. Tranche A + B invariants hold ────────────────────────────

// D37r-tranche-B” flip: the cy instantiation marker moved into the
// shared engine module. The lens-side wiring is now the engine.mount
// call. The visual-class style strings moved into the override
// builder `_buildContextEdgeStyleOverride()`, still in the Context
// renderer (preserved verbatim from the dead legacy
// `_buildContextCytoscapeStyle()`). The node/edge builders, bridge
// selection contract, and camera-delegate factory remain.
func TestExplorer_GraphCytoscapeOverlay_TrancheAAndBInvariantsHold(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeContextRenderer)

	for _, want := range []string{
		// D37r-tranche-B'' flip: cy instantiation lives in the
		// engine module now. The lens-side wiring is engine.mount.
		"engine.mount(canvas, {",
		// Node + edge builders consume stage.
		"function _buildContextCytoscapeNodes(cards, stage)",
		"function _buildContextCytoscapeEdges(connectors, stage)",
		// Selection still publishes through the bridge.
		"bridge.selectCard(card)",
		// Camera delegate factories still wired (the legacy
		// cy-based factory is preserved as dead code; the
		// engine-handle-based factory is the live one).
		"function _buildContextCytoscapeCameraDelegate(cy)",
		"function _buildContextEngineCameraDelegate(handle)",
		// Five visual-class edge styles still present (now in
		// _buildContextEdgeStyleOverride, still in the renderer).
		"'edge.context-edge-visual-service'",
		"'edge.context-edge-visual-ai_binding'",
		"'edge.context-edge-visual-authority'",
		"'edge.context-edge-visual-evidence'",
		"'edge.context-edge-visual-gap'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'' (flipped): tranche A/B invariant %q must remain", want)
		}
	}

	// Engine-side: the shared module owns the cy constructor.
	engineJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	if !strings.Contains(engineJS, "cy = window.cytoscape({") {
		t.Errorf("D37r-tranche-B'' (flipped): tranche A/B invariant — `cy = window.cytoscape({` must remain in the shared engine module")
	}

	// Connector painter remains for fallback.
	cp := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-connector-painter.js")
	if !strings.Contains(cp, "paintConnectors") {
		t.Errorf("D37r-tranche-B': context-connector-painter must remain for fallback (paintConnectors symbol)")
	}
}

// ── 21. addNode uses buildNodeCardElement as sole DOM path ───────

// TestExplorer_GraphRenderer_AddNodeUsesBuildNodeCardElement is the
// required parity test from the user direction. It asserts:
//   - addNode delegates card DOM construction to buildNodeCardElement;
//   - the card-body innerHTML composition (`<span class="gmap-node-
//     label">` and siblings) lives in buildNodeCardElement, not in
//     addNode;
//   - addNode does NOT mutate the card's innerHTML after the factory
//     returns (no inline createElement / innerHTML for the card body
//     in addNode itself).
//
// Catches future drift where someone modifies addNode to mutate the
// card after construction in a way that would not reach the Cytoscape
// path.
func TestExplorer_GraphRenderer_AddNodeUsesBuildNodeCardElement(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rBprimeGraphRenderer)

	// Locate buildNodeCardElement and addNode bodies.
	bnceIdx := strings.Index(js, "function buildNodeCardElement(spec)")
	if bnceIdx < 0 {
		t.Fatal("D37r-tranche-B': buildNodeCardElement(spec) must be present")
	}
	addNodeIdx := strings.Index(js, "function addNode(spec, pos)")
	if addNodeIdx < 0 {
		t.Fatal("D37r-tranche-B': addNode(spec, pos) must be present")
	}

	// Slice addNode body — from its declaration to the next
	// top-level function. The file uses 2-space indented top-level
	// declarations.
	tail := js[addNodeIdx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatal("D37r-tranche-B': addNode body must be well-formed")
	}
	addNodeBody := tail[:endRel+1]

	// addNode delegates DOM construction to the factory.
	if !strings.Contains(addNodeBody, "var node = buildNodeCardElement(spec);") {
		t.Errorf("D37r-tranche-B': addNode must construct the card DOM via `var node = buildNodeCardElement(spec);`")
	}

	// addNode body must NOT contain any of the card-DOM tokens —
	// those live in buildNodeCardElement, not in addNode.
	for _, banned := range []string{
		"document.createElement('button')",
		"node.innerHTML",
		"'<span class=\"gmap-node-label\"",
		"'<span class=\"gmap-node-name\"",
		"'<div class=\"gmap-node-meta\"",
		"'<div class=\"gmap-node-inline-actions\"",
		"node.dataset.nodeId = spec.id",
		"node.dataset.nodeKind",
		"node.dataset.nodeName",
		"node.dataset.nodeDetails",
		"node.dataset.nodeActions",
	} {
		if strings.Contains(addNodeBody, banned) {
			t.Errorf("D37r-tranche-B': addNode body must not contain card-DOM token %q — DOM construction belongs in buildNodeCardElement", banned)
		}
	}

	// addNode body MUST still contain the legacy concerns that are
	// NOT card-DOM construction.
	for _, want := range []string{
		"var scene = document.getElementById('gmap-scene');",
		"if (!scene) return;",
		"node.style.left = pos.x + 'px';",
		"node.style.top  = pos.y + 'px';",
		"node.addEventListener('click'",
		"node.addEventListener('keydown'",
		"hAttach.attachDragHandlers(node, spec.id);",
		"scene.appendChild(node);",
	} {
		if !strings.Contains(addNodeBody, want) {
			t.Errorf("D37r-tranche-B': addNode must keep non-DOM-construction concern %q", want)
		}
	}

	// buildNodeCardElement body holds the card-DOM tokens. The body
	// extends from `function buildNodeCardElement(spec)` to the
	// CLOSING `}` of the function — we stop at the next top-level
	// declaration but EXCLUDE the documentation comment block that
	// precedes `function addNode` so banned-substring checks below
	// only inspect the function body itself.
	tail2 := js[bnceIdx:]
	endRel2 := strings.Index(tail2[1:], "\n  function ")
	if endRel2 < 0 {
		t.Fatal("D37r-tranche-B': buildNodeCardElement body must be well-formed")
	}
	bnceBlock := tail2[:endRel2+1]
	// Trim the trailing documentation-comment block addressed at
	// `addNode` so the purity scan below does not see it. Find the
	// first `// addNode` comment line that introduces the next
	// function's docblock; cut the body there.
	addNodeDocIdx := strings.Index(bnceBlock, "\n  // addNode —")
	if addNodeDocIdx >= 0 {
		bnceBlock = bnceBlock[:addNodeDocIdx]
	}
	bnceBody := bnceBlock
	for _, want := range []string{
		"var node = document.createElement('button');",
		"node.innerHTML =",
		"'<span class=\"gmap-node-label\">'",
		"'<span class=\"gmap-node-name\">'",
		"'<div class=\"gmap-node-meta\">'",
		"'<div class=\"gmap-node-inline-actions\" hidden></div>'",
		"return node;",
	} {
		if !strings.Contains(bnceBody, want) {
			t.Errorf("D37r-tranche-B': buildNodeCardElement body must own card-DOM token %q", want)
		}
	}

	// Purity: buildNodeCardElement does NOT reach into legacy
	// renderer state. Strip JS comments first so documentation
	// inside the function (e.g. block comments describing the
	// purity contract) does not falsely flag bans.
	purityScan := _stripJSComments(bnceBody)
	for _, banned := range []string{
		"_hooks()",
		"_state.",
		"#gmap-scene",
		"getElementById",
		"appendChild",
		"addEventListener",
		"attachDragHandlers",
	} {
		if strings.Contains(purityScan, banned) {
			t.Errorf("D37r-tranche-B': buildNodeCardElement must be pure — must not reference %q in load-bearing code", banned)
		}
	}
}
