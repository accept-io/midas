package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37q-viewport-2-impl — Context Connector Router on Shared Stage tests.
//
// Strategic Context's connector painter previously resolved every
// endpoint from a caller-supplied `getCardElement(cardId)` callback
// and `element.getBoundingClientRect()`. D37q-viewport-2-impl
// introduces the shared platform stage's anchor contract as the
// preferred endpoint source while preserving the DOM-measured
// centroid path as a per-endpoint fallback.
//
// Contract:
//
//   1. The painter consults `MIDASExplorerGraph.graphStage.anchorOf
//      (stage, cardKey, 'centre')` first when the caller supplies
//      `opts.stage`.
//   2. When the stage anchor returns null (stage absent, card not in
//      stage, malformed stage, etc.), the painter falls back to the
//      DOM-measured centroid for that specific endpoint.
//   3. Connectors whose source OR target endpoint cannot be resolved
//      by either source are silently skipped — preserving the pre-
//      tranche skip contract.
//   4. The strategic Context renderer passes `_lastStage` into the
//      painter only on the spatial-foundation paint path. The
//      document-flow path passes no stage (and keeps the DOM-
//      centroid behaviour).
//   5. Visual output is unchanged in spatial mode because the
//      stage's `centre` anchor equals the DOM-measured centroid for
//      absolutely-positioned cards.
//   6. Connector visual classes, dash patterns, SVG layering, and
//      idempotency are all unchanged.
//
// Out of scope (deferred to future tranches):
//   - Per-connector `sourceSide` / `targetSide` fields in connector
//     specs (painter currently always uses 'centre').
//   - Collision avoidance, orthogonal routing, edge labels.
//   - A standalone `graph-connector-router.js` peer module.
//   - Authority Cytoscape edge routing (Cytoscape owns coords).
//   - Legacy native Context's `addLiveConnector` path.

const (
	d37qV2PainterAsset           = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37qV2RendererAsset          = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37qV2StageAsset             = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37qV2GraphRendererAsset     = "/explorer/assets/js/graph/graph-renderer.js"
	d37qV2AuthorityPocAsset      = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37qV2CameraBusAsset         = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37qV2SelectionBridgeAsset   = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37qV2PaneShellAsset         = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37qV2DrawerAsset            = "/explorer/assets/js/graph/graph-drawer.js"
	d37qV2AuthorityEdgeAsset     = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37qV2AuthorityWorkbench     = "/explorer/assets/js/graph/authority/authority-graph-workbench.js"
	d37qV2ContextSelectionBridge = "/explorer/assets/js/graph/context/context-selection-bridge.js"
)

// ── 1. graphStage anchor contract preserved ───────────────────────

func TestExplorer_D37qViewport2_GraphStageAnchorContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2StageAsset)

	// Anchor function + side vocabulary intact.
	if !strings.Contains(js, "function anchorOf(stage, cardId, side)") {
		t.Errorf("D37q-viewport-2-impl: graphStage must expose anchorOf(stage, cardId, side)")
	}
	for _, want := range []string{
		"'top'",
		"'right'",
		"'bottom'",
		"'left'",
		"'centre'",
		"ANCHOR_SIDES",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-2-impl: graphStage anchor side vocabulary must include %q", want)
		}
	}
	// anchorOf is exported on the public surface.
	if !strings.Contains(js, "anchorOf:                anchorOf") {
		t.Errorf("D37q-viewport-2-impl: graphStage must export anchorOf on its public surface")
	}
}

// ── 2. Strategic Context builds the stage before connectors ───────

// TestExplorer_D37qViewport2_StrategicContextBuildsStageBeforeConnectors
// pins the spatial-foundation paint order: `graphStage.compose(...)`
// →  store on `_lastStage` →  paint cards →  paint connectors with
// `stage` passed in.
func TestExplorer_D37qViewport2_StrategicContextBuildsStageBeforeConnectors(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2RendererAsset)

	// Stage composed inside the spatial-foundation path.
	if !strings.Contains(js, "graphStage.compose(layout, footprints, safeArea, {})") {
		t.Errorf("D37q-viewport-2-impl: strategic Context spatial path must compose a stage via graphStage.compose")
	}
	// Stage stored before connector painting.
	if !strings.Contains(js, "_lastStage = stage;") {
		t.Errorf("D37q-viewport-2-impl: strategic Context spatial path must store the composed stage on _lastStage")
	}
	// D37q-viewport-8-impl — Connector paint pass receives the stage
	// as the 4th argument AND uses `stageEl` as its container so the
	// connector SVG (now a child of `stageEl`) shares the same
	// transformed coordinate space as the cards.
	if !strings.Contains(js, "_paintConnectorsForCanvas(svg, stageEl, connectors, stage);") {
		t.Errorf("D37q-viewport-8-impl: strategic Context spatial path must call _paintConnectorsForCanvas(svg, stageEl, connectors, stage) (post-D37q-viewport-8 container is stageEl, not the outer canvas)")
	}
	// The document-flow path does NOT pass a stage (callers omit the
	// 4th arg → painter receives `stage: null` → DOM-centroid
	// fallback). The non-spatial container remains the canvas.
	if !regexp.MustCompile(`_paintConnectorsForCanvas\(svg,\s*canvas,\s*connectors\);`).MatchString(js) {
		t.Errorf("D37q-viewport-2-impl: strategic Context document-flow path must keep its existing _paintConnectorsForCanvas(svg, canvas, connectors) call (no stage → DOM-centroid fallback)")
	}
}

// ── 3. Painter prefers stage anchors ──────────────────────────────

func TestExplorer_D37qViewport2_ContextConnectorPainterPrefersStageAnchors(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2PainterAsset)

	// Stage-anchor helper exists and consults graphStage.anchorOf.
	if !strings.Contains(js, "function _anchorFromStage(cardKey, opts)") {
		t.Errorf("D37q-viewport-2-impl: painter must declare a _anchorFromStage helper")
	}
	if !strings.Contains(js, "stageMod.anchorOf(opts.stage, cardKey, DEFAULT_ANCHOR_SIDE)") {
		t.Errorf("D37q-viewport-2-impl: painter must resolve endpoints via graphStage.anchorOf(stage, cardKey, 'centre')")
	}
	// Default side is 'centre'.
	if !strings.Contains(js, "DEFAULT_ANCHOR_SIDE    = 'centre'") {
		t.Errorf("D37q-viewport-2-impl: painter default anchor side must be 'centre' (matches DOM-centroid in spatial mode)")
	}
	// Per-endpoint resolver tries stage first, then falls back.
	if !strings.Contains(js, "function _resolveEndpoint(cardKey, opts, getCardElement, containerRect)") {
		t.Errorf("D37q-viewport-2-impl: painter must declare a _resolveEndpoint helper")
	}
	if !regexp.MustCompile(`var anchored = _anchorFromStage\(cardKey, opts\);[\s\S]{0,100}?if \(anchored\) return anchored;`).MatchString(js) {
		t.Errorf("D37q-viewport-2-impl: _resolveEndpoint must prefer the stage anchor when available")
	}
}

// ── 4. Painter falls back to DOM centroids ────────────────────────

func TestExplorer_D37qViewport2_ContextConnectorPainterFallsBackToDomCentroids(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2PainterAsset)

	// DOM-centroid helper preserved.
	if !strings.Contains(js, "function _centroid(rect, containerRect)") {
		t.Errorf("D37q-viewport-2-impl: painter must preserve the _centroid helper for the DOM-measured fallback")
	}
	// Fallback uses getCardElement + getBoundingClientRect.
	if !strings.Contains(js, "el = getCardElement(cardKey);") {
		t.Errorf("D37q-viewport-2-impl: painter must call getCardElement(cardKey) for the DOM fallback")
	}
	if !strings.Contains(js, "el.getBoundingClientRect") {
		t.Errorf("D37q-viewport-2-impl: painter must measure DOM centroids via getBoundingClientRect")
	}
	// The container rect is still computed from opts.containerEl /
	// svgEl.parentNode at paint time.
	if !strings.Contains(js, "containerEl.getBoundingClientRect()") {
		t.Errorf("D37q-viewport-2-impl: painter must continue to compute the container rect for DOM-centroid math")
	}
}

// ── 5. Visual classes preserved ───────────────────────────────────

func TestExplorer_D37qViewport2_ConnectorVisualClassesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2PainterAsset)

	for _, want := range []string{
		"CONNECTOR_CLASS        = 'context-connector'",
		"CONNECTOR_CLASS_PREFIX = 'context-connector--'",
		"data-visual-class",
		"data-edge-kind",
		"'service'",
		"line.setAttribute('class', classes.join(' '))",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-2-impl: connector visual-class handling must remain intact (%q)", want)
		}
	}
}

// ── 6. Dash patterns preserved ────────────────────────────────────

func TestExplorer_D37qViewport2_ConnectorDashPatternsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2PainterAsset)

	for _, want := range []string{
		"function _dashAttr(dashPattern)",
		"'solid'",
		"stroke-dasharray",
		"_dashAttr(c.dashPattern)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-2-impl: connector dash-pattern handling must remain intact (%q)", want)
		}
	}
}

// ── 7. SVG layer z-order preserved ────────────────────────────────

func TestExplorer_D37qViewport2_ConnectorSvgLayeringPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rendererJs := getExplorerAsset(t, srv, d37qV2RendererAsset)

	// SVG class name + the spatial-foundation order: SVG appended
	// BEFORE the stage so cards (z-index: 1) sit above connectors
	// (z-index: 0). Pin the load-bearing order.
	if !strings.Contains(rendererJs, `svg.setAttribute('class', 'context-renderer-connectors')`) {
		t.Errorf("D37q-viewport-2-impl: connector SVG layer class name must remain 'context-renderer-connectors'")
	}
	// CSS z-order is asserted by inspecting the renderer CSS at a
	// load-bearing rule. The CSS rule places .context-renderer-stage
	// at z-index: 1 (above the connectors layer at z-index: 0). The
	// distance budget tolerates the rule's explanatory comment block
	// between the selector and the z-index declaration.
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-renderer.css")
	if !regexp.MustCompile(`(?s)\.context-renderer-stage\s*\{[^}]*z-index:\s*1`).MatchString(css) {
		t.Errorf("D37q-viewport-2-impl: stage element must remain at z-index 1 (above connectors)")
	}
	if !regexp.MustCompile(`(?s)\.context-renderer-connectors\s*\{[^}]*z-index:\s*0`).MatchString(css) {
		t.Errorf("D37q-viewport-2-impl: connectors layer must remain at z-index 0 (below cards)")
	}
}

// ── 8. Paint idempotency preserved ────────────────────────────────

func TestExplorer_D37qViewport2_ConnectorPaintIdempotencyPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2PainterAsset)

	// _emptySvg clears children before each paint pass, preventing
	// duplicate-connector accumulation across repaints.
	if !strings.Contains(js, "function _emptySvg(svgEl)") {
		t.Errorf("D37q-viewport-2-impl: painter must preserve _emptySvg() to keep paint passes idempotent")
	}
	if !strings.Contains(js, "while (svgEl.firstChild) svgEl.removeChild(svgEl.firstChild)") {
		t.Errorf("D37q-viewport-2-impl: _emptySvg must walk-and-remove children to clear the layer")
	}
	if !strings.Contains(js, "_emptySvg(svgEl);") {
		t.Errorf("D37q-viewport-2-impl: paintConnectors must call _emptySvg before painting")
	}
}

// ── 9. Missing stage / card / anchor is safe ──────────────────────

// TestExplorer_D37qViewport2_MissingStageOrAnchorIsSafe pins that
// every defensive guard in the new endpoint-resolution path is
// present. The painter must:
//   - early-return on missing svg / connectors array;
//   - tolerate missing opts;
//   - tolerate missing opts.stage (no stage-anchor attempt);
//   - tolerate missing graphStage module;
//   - tolerate missing card in stage (anchorOf returns null →
//     fall back to DOM);
//   - silently skip connectors whose endpoints cannot be resolved.
func TestExplorer_D37qViewport2_MissingStageOrAnchorIsSafe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2PainterAsset)

	for _, want := range []string{
		// Top-level guards.
		"if (!svgEl || !Array.isArray(connectors)) return 0;",
		// Stage-anchor guard chain.
		"if (!_isPlainObject(opts) || !_isPlainObject(opts.stage)) return null;",
		"if (!stageMod || typeof stageMod.anchorOf !== 'function') return null;",
		// Anchor result is validated (numeric x / y).
		"typeof anchor.x === 'number'",
		"typeof anchor.y === 'number'",
		// DOM fallback null guards.
		"if (typeof getCardElement !== 'function') return null;",
		"if (!el || typeof el.getBoundingClientRect !== 'function') return null;",
		// Per-connector skip when an endpoint is unresolved.
		"if (!srcC || !dstC) continue;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-2-impl: defensive guard %q must be present", want)
		}
	}
	// Try / catch around the anchor lookup so a malformed stage
	// cannot throw out of the painter.
	if !regexp.MustCompile(`(?s)try \{[\s\S]*?stageMod\.anchorOf\(opts\.stage, cardKey, DEFAULT_ANCHOR_SIDE\);[\s\S]*?\} catch \(_\) \{`).MatchString(js) {
		t.Errorf("D37q-viewport-2-impl: stageMod.anchorOf call must be wrapped in try/catch")
	}
}

// ── 10. Legacy native Context connector path unchanged ────────────

func TestExplorer_D37qViewport2_LegacyNativeContextConnectorPathUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2GraphRendererAsset)

	for _, want := range []string{
		// Legacy SVG connector helpers preserved.
		"function addConnector(",
		"function addConnectorHitTarget(",
		"function addLiveConnector(",
		// Exposed on MIDASExplorerGraph.renderer for legacy Context.
		"addConnector:              addConnector",
		"addConnectorHitTarget:     addConnectorHitTarget",
		"addLiveConnector:          addLiveConnector",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-2-impl: legacy native Context connector path must remain untouched (%q)", want)
		}
	}
	// Legacy renderer must NOT have suddenly grown a graphStage
	// dependency.
	if strings.Contains(js, "graphStage.anchorOf") {
		t.Errorf("D37q-viewport-2-impl: legacy graph-renderer.js must not adopt graphStage.anchorOf in this tranche")
	}
}

// ── 11. Authority does not consume graphStage ─────────────────────

func TestExplorer_D37qViewport2_AuthorityDoesNotConsumeGraphStage(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2AuthorityPocAsset)

	// Authority is Cytoscape-engine-owned. graphStage is for
	// DOM/SVG renderers.
	for _, banned := range []string{
		"graphStage.compose",
		"graphStage.anchorOf",
		"graphStage.fitBoundsOf",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37q-viewport-2-impl: Authority must NOT consume %q (Cytoscape owns coordinates)", banned)
		}
	}
}

// ── 12. Camera / selection / pane / drawer contracts unchanged ────

func TestExplorer_D37qViewport2_CameraSelectionPaneDrawerUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Camera bus locked vocabulary intact.
	bus := getExplorerAsset(t, srv, d37qV2CameraBusAsset)
	for _, want := range []string{
		"'zoomIn'",
		"'zoomOut'",
		"'fit'",
		"'reset'",
		"'focusRoot'",
		"'focusSelected'",
		"'setZoom'",
		"'getZoom'",
	} {
		if !strings.Contains(bus, want) {
			t.Errorf("D37q-viewport-2-impl: graphCameraBus locked vocabulary must remain intact (%q)", want)
		}
	}

	// Selection bridge locked event vocabulary intact.
	bridge := getExplorerAsset(t, srv, d37qV2SelectionBridgeAsset)
	for _, want := range []string{
		"'selection_changed'",
		"'selection_cleared'",
		"'action_dispatched'",
		"'action_error'",
		"'lens_registered'",
		"'lens_unregistered'",
		"'active_lens_changed'",
	} {
		if !strings.Contains(bridge, want) {
			t.Errorf("D37q-viewport-2-impl: graphSelectionBridge locked vocabulary must remain intact (%q)", want)
		}
	}

	// Selected-object pane shell public surface intact.
	pane := getExplorerAsset(t, srv, d37qV2PaneShellAsset)
	for _, want := range []string{
		"registerLensProvider:",
		"setActiveLens:",
		"getActiveProvider:",
	} {
		if !strings.Contains(pane, want) {
			t.Errorf("D37q-viewport-2-impl: graphSelectedObjectPane shell must remain intact (%q)", want)
		}
	}

	// Drawer public surface intact.
	drawer := getExplorerAsset(t, srv, d37qV2DrawerAsset)
	for _, want := range []string{
		"registerLens:",
		"setActiveLens:",
		"setActiveTab:",
	} {
		if !strings.Contains(drawer, want) {
			t.Errorf("D37q-viewport-2-impl: graph-drawer must remain intact (%q)", want)
		}
	}

	// Authority canvas-edge tabs + workbench untouched.
	edge := getExplorerAsset(t, srv, d37qV2AuthorityEdgeAsset)
	if !strings.Contains(edge, "window.MIDASExplorerGraph.authorityCanvasEdgeTabs") {
		t.Errorf("D37q-viewport-2-impl: Authority canvas-edge module must remain intact")
	}
	wb := getExplorerAsset(t, srv, d37qV2AuthorityWorkbench)
	if !strings.Contains(wb, "window.MIDASExplorerGraph.authorityWorkbench") {
		t.Errorf("D37q-viewport-2-impl: Authority workbench module must remain intact")
	}

	// Context selection bridge still publishes through the platform
	// bridge (D37p-selection-1 / D37q-viewport-5 invariants).
	ctxBridge := getExplorerAsset(t, srv, d37qV2ContextSelectionBridge)
	if !strings.Contains(ctxBridge, "bridge.registerLens('context'") {
		t.Errorf("D37q-viewport-2-impl: Context selection bridge must still register the 'context' delegate")
	}
}

// ── 13. Renderer contract still passes ────────────────────────────

// TestExplorer_D37qViewport2_RendererContractStillPasses reasserts
// the load-bearing invariants from D37q-viewport-7's renderer-
// contract suite that intersect with this tranche.
func TestExplorer_D37qViewport2_RendererContractStillPasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Strategic Context still registers with GraphViewport.
	ctxJs := getExplorerAsset(t, srv, d37qV2RendererAsset)
	if !strings.Contains(ctxJs, "g.viewport.register(RENDERER_ID, _factoryFor())") {
		t.Errorf("D37q-viewport-2-impl: strategic Context must still register with GraphViewport")
	}
	// Strategic Context spatial path still consumes graphStage.compose.
	if !strings.Contains(ctxJs, "graphStage.compose(layout, footprints, safeArea, {})") {
		t.Errorf("D37q-viewport-2-impl: strategic Context spatial path must still consume graphStage.compose(...)")
	}
	// graphStage public surface intact.
	stage := getExplorerAsset(t, srv, d37qV2StageAsset)
	for _, want := range []string{
		"compose",
		"anchorOf",
		"fitBoundsOf",
		"normaliseCardFootprints",
	} {
		if !strings.Contains(stage, want) {
			t.Errorf("D37q-viewport-2-impl: graphStage public surface must remain intact (%q)", want)
		}
	}
}

// ── 14. Painter exports the default anchor side constant ──────────

// TestExplorer_D37qViewport2_PainterExportsDefaultAnchorSide pins
// the new diagnostic constant on the painter's public surface so
// tests and DevTools probes can confirm the default side without
// reaching into module internals.
func TestExplorer_D37qViewport2_PainterExportsDefaultAnchorSide(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV2PainterAsset)

	if !strings.Contains(js, "DEFAULT_ANCHOR_SIDE:    DEFAULT_ANCHOR_SIDE") {
		t.Errorf("D37q-viewport-2-impl: painter must export DEFAULT_ANCHOR_SIDE on its _constants surface")
	}
}
