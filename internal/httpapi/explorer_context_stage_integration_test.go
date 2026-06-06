package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-impl-2 — Context Layout Policy on Shared Stage tests
//
// Pins the wiring of the strategic Context renderer to the shared
// graph-stage platform module (D37p-impl-1):
//
//   • graph-stage.js is loaded once, before context-cytoscape-renderer.js;
//   • the renderer reads `?contextLayout=spatial` and gates on the
//     presence of `MIDASExplorerGraph.graphStage`;
//   • the renderer calls `graphStage.compose(...)` and consumes
//     `stage.cards` + `stage.dimensions`;
//   • the existing non-spatial flow path is preserved as fallback;
//   • the layout model emits `layoutKind: 'banded'` but stays
//     coordinate-free;
//   • CSS adds a `[data-spatial="true"]` variant scoped under the
//     active-renderer context attribute;
//   • renderer remains fetch-free, free of legacy DOM scraping, and
//     decoupled from drawer / tray / Authority surfaces;
//   • the surrounding foundation (legacy renderer, Selected-Object
//     Pane, projection provider script, GraphViewport, default
//     renderer behaviour) is preserved.
//
// Tests are source-contract pins (asset-text). Runtime browser
// validation is required separately and is recorded in the final
// report's checklist.

const (
	d37pImpl2RendererAsset    = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pImpl2LayoutAsset      = "/explorer/assets/js/graph/context/context-layout-model.js"
	d37pImpl2CssAsset         = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37pImpl2StageAsset       = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37pImpl2LegacyViewAsset  = "/explorer/assets/js/graph/context/context-graph-view.js"
	d37pImpl2AuthorityCEdge   = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37pImpl2PaneAsset        = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
)

// ── A. Asset load order ──────────────────────────────────────────────

func TestExplorer_D37pImpl2_ContextStage_GraphStageScriptPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, `src="/explorer/assets/js/graph/graph-platform/graph-stage.js"`) {
		t.Errorf("D37p-impl-2: index.html must include <script> for graph-platform/graph-stage.js")
	}
	// Exactly one script tag for the platform stage.
	count := strings.Count(body, `src="/explorer/assets/js/graph/graph-platform/graph-stage.js"`)
	if count != 1 {
		t.Errorf("D37p-impl-2: graph-stage.js must be included exactly once (found %d)", count)
	}
}

// TestExplorer_D37pImpl2_ContextStage_GraphStageLoadsBeforeRenderer pins
// the load order — the platform stage must be parsed before the
// strategic Context renderer's IIFE runs so the renderer's spatial
// gate can find `MIDASExplorerGraph.graphStage`.
func TestExplorer_D37pImpl2_ContextStage_GraphStageLoadsBeforeRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	stageIdx    := strings.Index(body, "graph-platform/graph-stage.js")
	rendererIdx := strings.Index(body, "context-cytoscape-renderer.js")
	if stageIdx < 0 || rendererIdx < 0 {
		t.Fatal("D37p-impl-2: both graph-stage.js and context-cytoscape-renderer.js scripts must appear in index.html")
	}
	if stageIdx >= rendererIdx {
		t.Errorf("D37p-impl-2: graph-stage.js must load BEFORE context-cytoscape-renderer.js (stage idx=%d, renderer idx=%d)", stageIdx, rendererIdx)
	}
}

// ── B. Spatial flag detection ────────────────────────────────────────

func TestExplorer_D37pImpl2_ContextStage_RendererReadsSpatialFlag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	for _, want := range []string{
		"contextLayout",
		"'spatial'",
		"LAYOUT_QUERY_PARAM",
		"_isSpatialMode",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-2: strategic renderer must declare the spatial flag (%q)", want)
		}
	}
}

// ── C. Renderer consumes graphStage ──────────────────────────────────

func TestExplorer_D37pImpl2_ContextStage_RendererConsumesGraphStage(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph",
		"graphStage",
		"graphStage.compose(",
		"stage.cards",
		"stage.dimensions",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-2: strategic renderer must consume the shared graph stage (%q)", want)
		}
	}
}

// TestExplorer_D37pImpl2_ContextStage_RendererPositionsCardsAbsolutely
// pins that the spatial path writes inline absolute coordinates onto
// card elements from the StageModel.
func TestExplorer_D37pImpl2_ContextStage_RendererPositionsCardsAbsolutely(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	if !strings.Contains(js, "_applyStagePosition") {
		t.Errorf("D37p-impl-2: spatial path must define an explicit position-application helper")
	}
	for _, want := range []string{
		"el.style.position = 'absolute'",
		"el.style.left",
		"el.style.top",
		"el.style.width",
		"el.style.height",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-2: spatial path must write absolute coordinates inline (%q)", want)
		}
	}
}

// TestExplorer_D37pImpl2_ContextStage_StageRootEmittedInSpatialMode pins
// the DOM envelope: a stage element with the `context-renderer-stage`
// class carrying its dimensions, hosted inside a canvas element
// flagged with `data-spatial="true"`.
func TestExplorer_D37pImpl2_ContextStage_StageRootEmittedInSpatialMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	for _, want := range []string{
		"context-renderer-stage",
		"setAttribute('data-spatial', 'true')",
		"data-stage-width",
		"data-stage-height",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-2: spatial render must emit a stage element with locked attributes (%q)", want)
		}
	}
}

// ── D. Non-spatial fallback preserved ────────────────────────────────

func TestExplorer_D37pImpl2_ContextStage_FallbackPathPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	// The existing flow-layout helpers must remain in source so the
	// renderer continues to render the banded flow foundation when
	// spatial mode is not active.
	for _, want := range []string{
		"_renderMain",
		"_renderBandSection",
		"_renderGovernance",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-2: non-spatial flow-layout helper %q must remain in source", want)
		}
	}

	// The spatial dispatcher must be conditional, not unconditional.
	if !strings.Contains(js, "if (_isSpatialMode() && _hasGraphStage())") {
		t.Errorf("D37p-impl-2: spatial path must be a conditional branch in _renderVisualFoundation")
	}
}

// ── E. Renderer remains fetch-free ───────────────────────────────────

func TestExplorer_D37pImpl2_ContextStage_RendererRemainsFetchFree(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	for _, banned := range []string{
		"window.fetch(",
		"XMLHttpRequest",
		"/v1/graphs/context",
		"MIDASExplorerAPI.graphs.context",
		"contextAdapter.fetch(",
		"shell.refresh(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-2: strategic renderer must remain fetch-free; found %q", banned)
		}
	}
}

// ── F. Renderer does not scrape legacy DOM ───────────────────────────

func TestExplorer_D37pImpl2_ContextStage_RendererNoLegacyDomScraping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	for _, banned := range []string{
		"#gmap-canvas",
		"#gmap-svg",
		"#gmap-scene",
		".gmap-node",
		"getElementById('gmap-canvas')",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-2: strategic renderer must not scrape legacy renderer DOM; found %q", banned)
		}
	}
}

// ── G. Renderer does not call drawer / tray / Authority surfaces ─────

func TestExplorer_D37pImpl2_ContextStage_RendererNoDrawerOrTrayOrAuthorityCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	for _, banned := range []string{
		".setName(",
		".setFields(",
		".setGovernance(",
		".setActions(",
		".setInlineActions(",
		"contextEvidenceTray",
		"authorityWorkbench",
		"authority-canvas-edge",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-2: strategic renderer must not couple to %q", banned)
		}
	}
}

// ── H. Layout model remains generic ──────────────────────────────────

func TestExplorer_D37pImpl2_ContextStage_LayoutModelEmitsLayoutKind(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2LayoutAsset)

	if !strings.Contains(js, "layoutKind:       'banded'") {
		t.Errorf("D37p-impl-2: layout model must publish layoutKind:'banded' so graphStage can consume it")
	}
}

// TestExplorer_D37pImpl2_ContextStage_LayoutModelStaysCoordinateFree
// pins that the layout model does not start computing pixel
// coordinates as part of the D37p-impl-2 change. Coordinates remain
// the platform stage's responsibility.
func TestExplorer_D37pImpl2_ContextStage_LayoutModelStaysCoordinateFree(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2LayoutAsset)

	// The layout model must not introduce pixel-coordinate math or
	// DOM-aware identifiers. (The model already declared
	// `cardMinSpacingPx` for density hints in D37o-impl-1; that
	// existing token is allowed.)
	for _, banned := range []string{
		"document.",
		"querySelector",
		"getElementById",
		"createElement",
		"appendChild",
		"innerHTML",
		"graphStage",
		"cytoscape",
		"Cytoscape",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-2: layout model must remain coordinate-free / DOM-free; found %q", banned)
		}
	}

	// Forbid the most-likely Context-specific coordinate-leak tokens.
	for _, banned := range []string{
		"NODE_W",
		"NODE_H",
		"node.x",
		"node.y",
		".x =",
		".y =",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-2: layout model must not compute Context-specific coordinates (%q)", banned)
		}
	}
}

// ── I. CSS scoping for spatial mode ──────────────────────────────────

func TestExplorer_D37pImpl2_ContextStage_CssSpatialSelectorPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pImpl2CssAsset)

	for _, want := range []string{
		`.context-renderer-canvas[data-spatial="true"]`,
		`.context-renderer-stage`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37p-impl-2: spatial CSS must declare %q", want)
		}
	}
}

// TestExplorer_D37pImpl2_ContextStage_AllNewRulesScopedToContext pins
// that every CSS rule declaring `data-spatial="true"` is scoped under
// the active-renderer context attribute, so the spatial styles apply
// only when the strategic Context renderer is mounted by the host.
//
// Strategy: only the SELECTOR portion of a CSS rule (the text on the
// line that ends in `{`) is considered. We split by `}` to get
// candidate rules, then check the selector portion of each rule that
// mentions `data-spatial="true"`.
func TestExplorer_D37pImpl2_ContextStage_AllNewRulesScopedToContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pImpl2CssAsset)

	// Strip block comments so they don't pollute selector lines.
	commentRE := regexp.MustCompile(`(?s)/\*.*?\*/`)
	clean := commentRE.ReplaceAllString(css, "")

	rules := strings.Split(clean, "}")
	found := 0
	for _, raw := range rules {
		brace := strings.LastIndex(raw, "{")
		if brace < 0 {
			continue
		}
		selector := strings.TrimSpace(raw[:brace])
		if !strings.Contains(selector, `[data-spatial="true"]`) {
			continue
		}
		found++
		if !strings.HasPrefix(selector, `.midas-graph-viewport[data-active-renderer="context"]`) {
			t.Errorf("D37p-impl-2: spatial CSS selector %q must be scoped under .midas-graph-viewport[data-active-renderer=\"context\"]", selector)
		}
	}
	if found == 0 {
		t.Fatal("D37p-impl-2: no [data-spatial=\"true\"] CSS rules found")
	}
}

// ── J. Existing non-spatial CSS still present ────────────────────────

func TestExplorer_D37pImpl2_ContextStage_CssFallbackRulesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pImpl2CssAsset)

	for _, want := range []string{
		`.context-renderer-mount`,
		`.context-renderer-canvas`,
		`.context-renderer-main`,
		`.context-renderer-band`,
		`.context-renderer-governance`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37p-impl-2: existing non-spatial CSS rule %q must remain", want)
		}
	}
}

// ── K. Foundation preservation ───────────────────────────────────────

func TestExplorer_D37pImpl2_ContextStage_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Strategic renderer identity unchanged.
	rendererJS := getExplorerAsset(t, srv, d37pImpl2RendererAsset)
	if !strings.Contains(rendererJS, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37p-impl-2: strategic Context renderer canonical id must remain 'context'")
	}
	if !strings.Contains(rendererJS, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37p-impl-2: strategic activation query param must remain 'contextRenderer'")
	}

	// Legacy renderer still present.
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-graph-view.js"`) {
		t.Errorf("D37p-impl-2: legacy context-graph-view.js script tag must remain")
	}
	legacyJS := getExplorerAsset(t, srv, d37pImpl2LegacyViewAsset)
	// D37p-clean-1 retired the dead `renderer.register('context', lensImpl)`
	// dispatcher path; the live legacy Context entry point is the
	// `contextView` export.
	if !strings.Contains(legacyJS, "window.MIDASExplorerGraph.contextView") {
		t.Errorf("D37p-impl-2: legacy context-graph-view.js must still expose contextView entry points")
	}

	// Authority canvas-edge wrapper still served.
	auth := getExplorerAsset(t, srv, d37pImpl2AuthorityCEdge)
	if len(auth) == 0 {
		t.Errorf("D37p-impl-2: Authority canvas-edge wrapper must remain served")
	}

	// Selected-Object Pane wrapper still present in markup.
	if !strings.Contains(body, "gmap-context-selected-object-pane") {
		t.Errorf("D37p-impl-2: Context Selected-Object Pane wrapper must remain present")
	}

	// Projection provider script still wired.
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-projection-provider.js"`) {
		t.Errorf("D37p-impl-2: context-projection-provider.js script tag must remain in index.html")
	}

	// Default renderer behaviour unchanged: no flip to strategic by
	// default, no temporary renderer identities.
	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37p-impl-2: must not introduce temporary renderer name %q", banned)
		}
	}
}

// ── L. Index.html line count ─────────────────────────────────────────

// TestExplorer_D37pImpl2_ContextStage_IndexHtmlWithinCeiling pins that
// the added <script> tag did not push index.html past the existing
// line-count ceiling. The current ceiling (post-D37o-toolbar-1) is
// 8000; this tranche should land comfortably under it.
func TestExplorer_D37pImpl2_ContextStage_IndexHtmlWithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 8000 {
		t.Errorf("D37p-impl-2: index.html line count %d exceeds the existing 8000 ceiling — pin bump required", lines)
	}
}

// ── M. Spatial path graph-engine status ─────────────────────────────
//
// D37p-impl-2 pinned that the strategic spatial path must NOT
// instantiate a graph engine — that was the correct contract for the
// pre-D37r DOM/SVG strategic path. D37r-context-cytoscape-2-impl
// flipped that to the cy-construction contract. D37r-tranche-B''
// flips it again: the cy instantiation moved into the shared engine
// module. Context's spatial paint path now consumes the engine via
// `engine.mount(canvas, {…})`; the engine module owns the cy
// constructor call. The lens-side wiring (engine.mount + cameraAdapter
// returning a cy-backed delegate that calls cy.zoom / cy.fit / etc.)
// is what this test pins.

func TestExplorer_D37pImpl2_ContextStage_SpatialPathNoGraphEngineInstance(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2RendererAsset)

	// D37r-tranche-B'' flip: the strategic spatial path now consumes
	// the shared engine module instead of instantiating cy directly.
	if !strings.Contains(js, "engine.mount(canvas, {") {
		t.Errorf("D37r-tranche-B'' (flips D37r-context-cytoscape-2-impl which flipped D37p-impl-2): strategic spatial path must consume the engine via engine.mount(canvas, {…}) — the engine module now owns the cy constructor")
	}
	// And the lens still calls into cy.<api> via its cameraAdapter
	// delegate — `_buildContextEngineCameraDelegate(handle)` wraps the
	// handle and calls handle.cy methods (zoom / fit / center / etc.).
	if !regexp.MustCompile(`\bcy\.(zoom|fit|center|getElementById|on)\b`).MatchString(js) {
		t.Errorf("D37r-tranche-B'' (flipped): strategic spatial path must call into the Cytoscape instance via cy.<api> through the engine's cameraAdapter")
	}

	// Engine-side: the shared module owns the cy constructor.
	engineJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	if !strings.Contains(engineJS, "cy = window.cytoscape({") {
		t.Errorf("D37r-tranche-B'' (flipped): the shared engine module must own the cy = window.cytoscape({…}) constructor call")
	}
}

// ── N. Sanity — graphStage module still served ──────────────────────

func TestExplorer_D37pImpl2_ContextStage_StageAssetStillServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl2StageAsset)
	if len(js) == 0 {
		t.Fatal("D37p-impl-2: graph-stage.js must still be served by the asset route")
	}
	if !strings.Contains(js, "window.MIDASExplorerGraph.graphStage") {
		t.Errorf("D37p-impl-2: graph-stage.js must still export window.MIDASExplorerGraph.graphStage")
	}
}
