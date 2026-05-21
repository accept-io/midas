package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37q-viewport-8-impl — Context Spatial Connector Transform Alignment.
//
// D37q-viewport-8-diagnose classified the strategic Context spatial
// connector layer as a Class C (real) transform-alignment defect:
// connectors and cards lived in different coordinate spaces because
// the connector SVG was a SIBLING of `_stageEl` while only `_stageEl`
// received the camera's `transform: translate(...) scale(...)` style.
// Under non-identity camera transforms, cards visually moved/scaled
// while connector `<line>` endpoints stayed at their pre-transform
// stage-local coordinates → visible desync during zoom / pan / fit /
// reset.
//
// D37q-viewport-8-impl moves the connector SVG INSIDE `_stageEl` so
// the camera transform propagates to both layers atomically. No new
// camera subscription, no per-transform repaint, no DOM duplication.
// The fix is a one-line DOM-ordering change plus a one-line
// container-pass change at the painter call site.
//
// Contract invariants pinned here:
//
//   1. Spatial path appends the connector SVG to `stageEl`, not to
//      the outer canvas.
//   2. Cards and connector SVG are both descendants of `stageEl` —
//      the element receiving the camera transform.
//   3. Camera transform remains applied to `_stageEl` only; no
//      duplicate transform on the SVG itself.
//   4. No connector subscription to the camera (the painter does not
//      become camera-aware; layers share transform via DOM nesting).
//   5. Spatial painter call uses `stageEl` as `containerEl` so the
//      DOM-fallback measurement coordinate system is consistent
//      with the SVG's local coordinate system.
//   6. Non-spatial path's `containerEl` remains `canvas` — unchanged.
//   7. Stage-anchor path (`DEFAULT_ANCHOR_SIDE = 'centre'`) remains
//      the preferred endpoint resolver.
//   8. Connector visual classes, dash patterns, idempotency, and the
//      "below cards" stacking remain intact.
//   9. Legacy native Context, Authority, camera bus, selection
//      bridge, selected-object pane, drawer, and workbench are
//      untouched.

const (
	d37qV8RendererAsset           = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37qV8PainterAsset            = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37qV8RendererCssAsset        = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37qV8StageAsset              = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37qV8CameraControllerAsset   = "/explorer/assets/js/graph/graph-platform/graph-camera-controller.js"
	d37qV8CameraBusAsset          = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37qV8SelectionBridgeAsset    = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37qV8PaneShellAsset          = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37qV8DrawerAsset             = "/explorer/assets/js/graph/graph-drawer.js"
	d37qV8GraphRendererAsset      = "/explorer/assets/js/graph/graph-renderer.js"
	d37qV8AuthorityPocAsset       = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37qV8AuthorityEdgeAsset      = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
)

// ── 1. Connector SVG is appended to the transformed stage ─────────

// TestExplorer_D37qViewport8_ConnectorSvgInsideTransformedStage pins
// the load-bearing DOM-ordering change in the spatial render path:
// the SVG is now appended to `stageEl` (the camera-transformed
// element), not to the outer `canvas` sibling. The pre-D37q-viewport-8
// `canvas.appendChild(svg)` call in the spatial-foundation block is
// gone.
func TestExplorer_D37qViewport8_ConnectorSvgInsideTransformedStage(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV8RendererAsset)

	// Slice the spatial-foundation function body so we don't pick up
	// the non-spatial `canvas.appendChild(svg)` call that legitimately
	// remains in `_renderVisualFoundation`.
	spatialStart := strings.Index(js, "function _renderSpatialFoundation(layout, cards, connectors) {")
	if spatialStart < 0 {
		t.Fatal("D37q-viewport-8-impl: _renderSpatialFoundation must be present")
	}
	// _renderSpatialFoundation ends at the next top-level `function `
	// declaration. Slice until the next one.
	tail := js[spatialStart:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatal("D37q-viewport-8-impl: _renderSpatialFoundation block must be well-formed")
	}
	spatial := tail[:endRel+1]

	// Inside the spatial block, the SVG is appended to stageEl, not canvas.
	if !strings.Contains(spatial, "stageEl.appendChild(svg);") {
		t.Errorf("D37q-viewport-8-impl: spatial-foundation must append the connector SVG INSIDE stageEl via stageEl.appendChild(svg)")
	}
	if strings.Contains(spatial, "canvas.appendChild(svg)") {
		t.Errorf("D37q-viewport-8-impl: spatial-foundation must NOT append the connector SVG to the outer canvas anymore (pre-D37q-viewport-8 structure)")
	}
	// The stage is still appended to the canvas (the camera-
	// transformed element remains stageEl).
	if !strings.Contains(spatial, "canvas.appendChild(stageEl);") {
		t.Errorf("D37q-viewport-8-impl: spatial-foundation must continue to append stageEl to canvas")
	}
}

// ── 2. Cards and SVG share the transformed parent ─────────────────

// TestExplorer_D37qViewport8_ConnectorSvgAndCardsShareTransform pins
// that the spatial-foundation block:
//   - appends the SVG to stageEl,
//   - appends cards to stageEl,
//   - stores stageEl as _stageEl,
// so cards and connectors share the same parent element — which is
// the element the camera writes its transform on.
func TestExplorer_D37qViewport8_ConnectorSvgAndCardsShareTransform(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV8RendererAsset)

	// SVG appended into stageEl.
	if !strings.Contains(js, "stageEl.appendChild(svg);") {
		t.Errorf("D37q-viewport-8-impl: SVG must be a child of stageEl")
	}
	// Cards appended into stageEl (preserved from earlier tranches).
	if !strings.Contains(js, "stageEl.appendChild(el);") {
		t.Errorf("D37q-viewport-8-impl: cards must remain children of stageEl")
	}
	// Sentinel tiles also share stageEl.
	if !strings.Contains(js, "stageEl.appendChild(senEl);") {
		t.Errorf("D37q-viewport-8-impl: sentinel tiles must remain children of stageEl")
	}
	// _stageEl tracks the element receiving the camera transform.
	if !strings.Contains(js, "_stageEl   = stageEl;") {
		t.Errorf("D37q-viewport-8-impl: _stageEl must reference the same stageEl element the SVG and cards are appended to")
	}
}

// ── 3. Camera transform stays on _stageEl only ────────────────────

// TestExplorer_D37qViewport8_CameraTransformStillAppliesToStage pins
// that the camera's `applyTransform` callback continues to write the
// inline transform on `_stageEl` only — NOT on the SVG separately.
// The fix relies on DOM nesting, not on duplicating the transform.
func TestExplorer_D37qViewport8_CameraTransformStillAppliesToStage(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV8RendererAsset)

	if !strings.Contains(js, "_stageEl.style.transformOrigin = 'top left';") {
		t.Errorf("D37q-viewport-8-impl: camera applyTransform must still write transformOrigin on _stageEl")
	}
	if !regexp.MustCompile(`_stageEl\.style\.transform\s*=\s*\n?\s*'translate\(' \+ px \+ 'px, ' \+ py \+ 'px\) scale\(' \+ z \+ '\)';`).MatchString(js) {
		t.Errorf("D37q-viewport-8-impl: camera applyTransform must still write the translate/scale transform on _stageEl")
	}
	// No second `style.transform =` write on the SVG. Pin the
	// absence: no occurrence of `svg.style.transform` anywhere in the
	// renderer.
	if regexp.MustCompile(`svg\.style\.transform\s*=`).MatchString(js) {
		t.Errorf("D37q-viewport-8-impl: SVG must NOT receive its own transform — the stage's transform must propagate via DOM nesting")
	}
}

// ── 4. No new camera subscription / per-transform repaint ─────────

// TestExplorer_D37qViewport8_NoConnectorRepaintOnCameraTransform
// pins that the fix did NOT introduce a camera-subscription path
// (Option C from the diagnosis). Connectors are NOT repainted on
// every transform; they ride along via the shared transformed
// parent element.
func TestExplorer_D37qViewport8_NoConnectorRepaintOnCameraTransform(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV8RendererAsset)

	// The strategic Context renderer must NOT subscribe to the
	// camera controller's transform events.
	if regexp.MustCompile(`_camera\.subscribe\s*\(`).MatchString(js) {
		t.Errorf("D37q-viewport-8-impl: renderer must NOT subscribe to camera transform events (the fix is DOM-nesting-based)")
	}
	if regexp.MustCompile(`camera\.subscribe\s*\(`).MatchString(js) {
		t.Errorf("D37q-viewport-8-impl: renderer must NOT subscribe to any camera transform event")
	}
	// The painter helper does not have a per-camera-transform repaint
	// loop.
	if strings.Contains(js, "function _repaintConnectorsOnTransform") {
		t.Errorf("D37q-viewport-8-impl: renderer must NOT introduce a per-transform connector-repaint helper")
	}
}

// ── 5. Spatial painter call uses stageEl as the container ─────────

func TestExplorer_D37qViewport8_SpatialPainterUsesStageLocalContainer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV8RendererAsset)

	if !strings.Contains(js, "_paintConnectorsForCanvas(svg, stageEl, connectors, stage);") {
		t.Errorf("D37q-viewport-8-impl: spatial-foundation paint call must pass stageEl as the container so DOM-fallback measurements share the SVG's local coordinate system")
	}
}

// ── 6. Non-spatial painter call still uses canvas ─────────────────

func TestExplorer_D37qViewport8_NonSpatialPainterStillUsesCanvasContainer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV8RendererAsset)

	// The document-flow path has no stage and continues to use the
	// canvas as the painter container. The literal form is the
	// pre-tranche signature (no 4th arg).
	if !regexp.MustCompile(`_paintConnectorsForCanvas\(svg,\s*canvas,\s*connectors\);`).MatchString(js) {
		t.Errorf("D37q-viewport-8-impl: non-spatial document-flow paint call must keep its existing _paintConnectorsForCanvas(svg, canvas, connectors) signature (no stage, canvas container)")
	}
}

// ── 7. Connector layering preserved inside the stage ──────────────

// TestExplorer_D37qViewport8_ConnectorLayeringPreservedInsideStage
// pins that the CSS still places connectors below cards inside the
// stage's stacking context. The stage itself remains the
// camera-transformed stacking context.
func TestExplorer_D37qViewport8_ConnectorLayeringPreservedInsideStage(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37qV8RendererCssAsset)

	// Stage rule still at z-index: 1, transformOrigin top left, will-change transform.
	if !regexp.MustCompile(`(?s)\.context-renderer-stage\s*\{[^}]*z-index:\s*1[^}]*transform-origin:\s*top left[^}]*will-change:\s*transform`).MatchString(css) {
		t.Errorf("D37q-viewport-8-impl: stage element must remain z-index: 1 + transform-origin: top left + will-change: transform")
	}
	// Connectors rule still at z-index: 0 with full-stage absolute positioning.
	if !regexp.MustCompile(`(?s)\.context-renderer-connectors\s*\{[^}]*position:\s*absolute[^}]*top:\s*0[^}]*left:\s*0[^}]*z-index:\s*0`).MatchString(css) {
		t.Errorf("D37q-viewport-8-impl: connectors layer must remain position: absolute; top:0; left:0; z-index:0")
	}
	// Cards retain z-index: 1 in spatial mode to stay above the
	// connectors. Scope the regex to the spatial-canvas rule so we
	// don't match the non-spatial .context-card rule which lacks
	// position: absolute / z-index by design.
	if !regexp.MustCompile(`(?s)\.context-renderer-canvas\[data-spatial="true"\]\s+\.context-card\s*\{[^}]*position:\s*absolute[^}]*z-index:\s*1`).MatchString(css) {
		t.Errorf("D37q-viewport-8-impl: spatial cards must remain position: absolute + z-index: 1 above the connector layer")
	}
}

// ── 8. Stage-anchor contract preserved (D37q-viewport-2 carry-over)

func TestExplorer_D37qViewport8_StageAnchorContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	painter := getExplorerAsset(t, srv, d37qV8PainterAsset)
	stage := getExplorerAsset(t, srv, d37qV8StageAsset)

	// Painter still resolves endpoints via graphStage.anchorOf with
	// DEFAULT_ANCHOR_SIDE = 'centre'.
	for _, want := range []string{
		"function _anchorFromStage(cardKey, opts)",
		"stageMod.anchorOf(opts.stage, cardKey, DEFAULT_ANCHOR_SIDE)",
		"DEFAULT_ANCHOR_SIDE    = 'centre'",
	} {
		if !strings.Contains(painter, want) {
			t.Errorf("D37q-viewport-8-impl: stage-anchor contract from D37q-viewport-2 must remain — missing %q", want)
		}
	}
	// graphStage still exposes anchorOf.
	if !strings.Contains(stage, "function anchorOf(stage, cardId, side)") {
		t.Errorf("D37q-viewport-8-impl: graphStage anchorOf must remain")
	}
}

// ── 9. DOM fallback safety preserved ──────────────────────────────

func TestExplorer_D37qViewport8_DomFallbackStillSafe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	painter := getExplorerAsset(t, srv, d37qV8PainterAsset)

	// Per-endpoint resolver still tries stage first, then falls back
	// to DOM centroids.
	for _, want := range []string{
		"function _resolveEndpoint(cardKey, opts, getCardElement, containerRect)",
		"var anchored = _anchorFromStage(cardKey, opts);",
		"if (anchored) return anchored;",
		"el = getCardElement(cardKey);",
		"el.getBoundingClientRect",
		"_centroid(rect, containerRect)",
	} {
		if !strings.Contains(painter, want) {
			t.Errorf("D37q-viewport-8-impl: per-endpoint resolver must preserve the DOM-centroid fallback (%q)", want)
		}
	}
	// Defensive guards still present.
	for _, want := range []string{
		"if (!_isPlainObject(opts) || !_isPlainObject(opts.stage)) return null;",
		"if (!stageMod || typeof stageMod.anchorOf !== 'function') return null;",
		"if (typeof getCardElement !== 'function') return null;",
		"if (!srcC || !dstC) continue;",
	} {
		if !strings.Contains(painter, want) {
			t.Errorf("D37q-viewport-8-impl: defensive guard %q must be present in the painter", want)
		}
	}
}

// ── 10. Legacy native Context unchanged ───────────────────────────

func TestExplorer_D37qViewport8_LegacyNativeContextUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV8GraphRendererAsset)

	for _, want := range []string{
		"function addConnector(",
		"function addConnectorHitTarget(",
		"function addLiveConnector(",
		"addConnector:              addConnector",
		"addLiveConnector:          addLiveConnector",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-8-impl: legacy native Context connector path must remain (%q)", want)
		}
	}
	if strings.Contains(js, "graphStage.anchorOf") {
		t.Errorf("D37q-viewport-8-impl: legacy graph-renderer.js must not adopt graphStage.anchorOf in this tranche")
	}
}

// ── 11. Authority unchanged ───────────────────────────────────────

func TestExplorer_D37qViewport8_AuthorityUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJs := getExplorerAsset(t, srv, d37qV8AuthorityPocAsset)
	edge := getExplorerAsset(t, srv, d37qV8AuthorityEdgeAsset)

	// Authority does not consume graphStage.
	for _, banned := range []string{
		"graphStage.compose",
		"graphStage.anchorOf",
		"graphStage.fitBoundsOf",
	} {
		if strings.Contains(authJs, banned) {
			t.Errorf("D37q-viewport-8-impl: Authority must not consume %q (Cytoscape owns coordinates)", banned)
		}
	}
	// Authority module public surface intact.
	if !strings.Contains(authJs, "window.MIDASExplorerGraph.cytoscapePoc") {
		t.Errorf("D37q-viewport-8-impl: Authority cytoscapePoc public surface must remain present")
	}
	// Authority canvas-edge module intact.
	if !strings.Contains(edge, "window.MIDASExplorerGraph.authorityCanvasEdgeTabs") {
		t.Errorf("D37q-viewport-8-impl: Authority canvas-edge module must remain intact")
	}
}

// ── 12. Camera / selection / pane / drawer contracts unchanged ────

func TestExplorer_D37qViewport8_CameraSelectionPaneDrawerUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Camera bus locked vocabulary intact.
	bus := getExplorerAsset(t, srv, d37qV8CameraBusAsset)
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
			t.Errorf("D37q-viewport-8-impl: graphCameraBus locked vocabulary must remain intact (%q)", want)
		}
	}

	// Camera controller subscribe API still exists (we just do not
	// consume it in the renderer) — it must remain available for
	// future / other consumers.
	ctrl := getExplorerAsset(t, srv, d37qV8CameraControllerAsset)
	if !strings.Contains(ctrl, "function subscribe(handler)") {
		t.Errorf("D37q-viewport-8-impl: graphCameraController subscribe(handler) must remain available")
	}

	// Selection bridge locked event vocabulary intact.
	bridge := getExplorerAsset(t, srv, d37qV8SelectionBridgeAsset)
	for _, want := range []string{
		"'selection_changed'",
		"'selection_cleared'",
	} {
		if !strings.Contains(bridge, want) {
			t.Errorf("D37q-viewport-8-impl: graphSelectionBridge events must remain intact (%q)", want)
		}
	}

	// Selected-object pane shell intact.
	pane := getExplorerAsset(t, srv, d37qV8PaneShellAsset)
	for _, want := range []string{
		"registerLensProvider:",
		"setActiveLens:",
	} {
		if !strings.Contains(pane, want) {
			t.Errorf("D37q-viewport-8-impl: graphSelectedObjectPane public surface must remain intact (%q)", want)
		}
	}

	// Drawer intact.
	drawer := getExplorerAsset(t, srv, d37qV8DrawerAsset)
	if !strings.Contains(drawer, "registerLens:") {
		t.Errorf("D37q-viewport-8-impl: graph-drawer registerLens must remain")
	}
}

// ── 13. Painter still exports DEFAULT_ANCHOR_SIDE ─────────────────

func TestExplorer_D37qViewport8_PainterDefaultAnchorSideExported(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV8PainterAsset)

	if !strings.Contains(js, "DEFAULT_ANCHOR_SIDE:    DEFAULT_ANCHOR_SIDE") {
		t.Errorf("D37q-viewport-8-impl: painter must continue to export DEFAULT_ANCHOR_SIDE on its _constants surface")
	}
}

// ── 14. No CSS regressions ────────────────────────────────────────

// TestExplorer_D37qViewport8_NoCssRegressions pins that the
// connector CSS rule still scopes under the spatial canvas attribute
// even though the SVG is now a descendant via `.context-renderer-stage`.
// The selector `.context-renderer-canvas[data-spatial="true"] .context-renderer-connectors`
// continues to match because the SVG remains a descendant of the
// canvas (just one level deeper).
func TestExplorer_D37qViewport8_NoCssRegressions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37qV8RendererCssAsset)

	if !strings.Contains(css, `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-canvas[data-spatial="true"] .context-renderer-connectors {`) {
		t.Errorf("D37q-viewport-8-impl: connectors CSS rule must continue to scope under the spatial canvas attribute selector")
	}
}
