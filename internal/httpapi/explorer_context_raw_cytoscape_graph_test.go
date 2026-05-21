package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37s-context-raw-cytoscape-1-impl — Raw Context Cytoscape Graph Correctness
//
// This tranche turns the strategic Context Cytoscape renderer from a live
// skeleton scaffold into a production raw Cytoscape graph surface.
//
// The pre-tranche `_paintSkeleton` function unconditionally appended a
// "Context Renderer — skeleton" banner + a "full visual parity arrives in
// a later tranche" note + a model-summary text row to the top of every
// paint. The strategic Context renderer is no longer a skeleton; the
// production paint function is `_paintStrategicContext`, and the mount
// switches to `data-mount-mode="graph"` so the canvas fills the renderer
// slot (Authority mount shape).
//
// These tests pin the source-contract surface of the tranche. Browser
// validation (no skeleton banner visible; canvas bounding rect matches
// mount; raw cy graph navigable) is the user-side runtime gate.

const (
	d37sRawContextAsset      = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37sRawContextCSS        = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37sRawEngineAsset       = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37sRawAuthorityAsset    = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37sRawViewportAsset     = "/explorer/assets/js/graph/graph-viewport.js"
	d37sRawSelBridgeAsset    = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37sRawCameraBusAsset    = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37sRawPaneAsset         = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37sRawEvidenceTrayAsset = "/explorer/assets/js/graph/context/context-evidence-tray.js"
)

// ── 1. No skeleton banner in strategic spatial production path ────

// TestExplorer_D37sRawContext_NoSkeletonBannerInStrategicSpatialPath
// pins that the live paint path no longer appends "Context Renderer —
// skeleton" or "full visual parity arrives in a later tranche".
func TestExplorer_D37sRawContext_NoSkeletonBannerInStrategicSpatialPath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	// Banner text strings must not appear as DOM text content
	// produced by the renderer. Their presence inside an executable
	// `textContent = '...'` assignment is the load-bearing pin.
	// Comments-only references (historical context) are allowed.
	for _, banned := range []string{
		`textContent = 'Context Renderer — skeleton';`,
		`textContent = 'Opt-in via ?contextRenderer=strategic. The skeleton proves GraphViewport lifecycle and model wiring; full visual parity arrives in a later tranche.';`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37s-context-raw-cytoscape-1-impl: skeleton banner text must NOT be appended by the renderer — found %q in source", banned)
		}
	}
}

// ── 2. Production paint function is not skeleton-named ────────────

// TestExplorer_D37sRawContext_ProductionPaintFunctionIsNotSkeletonNamed
// pins that the production paint function is `_paintStrategicContext`,
// not `_paintSkeleton`.
func TestExplorer_D37sRawContext_ProductionPaintFunctionIsNotSkeletonNamed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	if !strings.Contains(js, "function _paintStrategicContext()") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: production paint entry point must be `function _paintStrategicContext()`")
	}
	// `_paintSkeleton` must NOT exist as a function declaration in
	// source — historical references inside comments are allowed.
	if regexp.MustCompile(`(?m)^\s*function _paintSkeleton\s*\(`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: pre-tranche function `_paintSkeleton` must NOT exist in source — it was renamed to `_paintStrategicContext` to reflect the identity reset")
	}
}

// ── 3. Reflow loop uses production paint ──────────────────────────

// TestExplorer_D37sRawContext_ReflowLoopUsesProductionPaint pins that
// the measurement-driven reflow loop dispatches into
// `_paintStrategicContext`, not `_paintSkeleton`.
func TestExplorer_D37sRawContext_ReflowLoopUsesProductionPaint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	rIdx := strings.Index(js, "function _scheduleReflow()")
	if rIdx < 0 {
		t.Fatal("D37s-context-raw-cytoscape-1-impl: _scheduleReflow must exist")
	}
	rTail := js[rIdx:]
	rEndRel := strings.Index(rTail[1:], "\n  function ")
	if rEndRel < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: _scheduleReflow body must be well-formed")
	}
	rBody := rTail[:rEndRel+1]

	if !strings.Contains(rBody, "_paintStrategicContext()") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: _scheduleReflow must dispatch via `_paintStrategicContext()` so reflow paints don't reintroduce skeleton chrome")
	}
	if strings.Contains(rBody, "_paintSkeleton()") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: _scheduleReflow must NOT call `_paintSkeleton()` (the function no longer exists)")
	}
}

// ── 4. Mount lifecycle uses production paint ──────────────────────

// TestExplorer_D37sRawContext_MountLifecycleUsesProductionPaint pins
// that the renderer's mount/destroy lifecycle wires
// `_paintStrategicContext` for the initial + idempotent re-mount
// paints (not `_paintSkeleton`).
func TestExplorer_D37sRawContext_MountLifecycleUsesProductionPaint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	// All renderer lifecycle paths use the production paint name.
	if !strings.Contains(js, "_paintStrategicContext();") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: renderer must call `_paintStrategicContext();` (initial + idempotent re-mount + projection-publish + reflow paths)")
	}
	// Negative pin: no live caller of the pre-tranche name remains.
	if regexp.MustCompile(`\b_paintSkeleton\(\)`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: no caller of pre-tranche `_paintSkeleton()` must remain — every live caller is now `_paintStrategicContext()`")
	}
}

// ── 5. Mount carries data-mount-mode attribute ────────────────────

// TestExplorer_D37sRawContext_MountCarriesModeAttribute pins that the
// production paint function sets `data-mount-mode` to `'graph'` or
// `'document'` so CSS can scope layout per mode.
func TestExplorer_D37sRawContext_MountCarriesModeAttribute(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	// Both mode values are produced by the renderer.
	if !regexp.MustCompile(`_mountEl\.setAttribute\('data-mount-mode',\s*'graph'\)`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: renderer must set `data-mount-mode='graph'` on the mount in production graph mode")
	}
	if !regexp.MustCompile(`_mountEl\.setAttribute\('data-mount-mode',\s*'document'\)`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: renderer must set `data-mount-mode='document'` on the mount in empty-state / flow mode")
	}
}

// ── 6. Mount CSS — graph mode has no flex column ──────────────────

// TestExplorer_D37sRawContext_MountCssGraphModeFillsCanvas pins that
// the mount's graph-mode CSS rule does NOT carry the flex-column
// chrome (display: flex; flex-direction: column; padding; gap) that
// constrained the canvas in pre-tranche layout.
func TestExplorer_D37sRawContext_MountCssGraphModeFillsCanvas(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37sRawContextCSS)

	// The mount has THREE rules now: base + document + graph. The
	// base rule no longer carries flex/padding — those moved into
	// the document-mode rule.
	baseSpec := `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-mount {`
	baseIdx  := strings.Index(css, baseSpec)
	if baseIdx < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: base mount CSS rule %q must be present", baseSpec)
	}
	baseTail := css[baseIdx:]
	baseCloseRel := strings.Index(baseTail, "\n}\n")
	if baseCloseRel < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: base mount CSS rule body must be well-formed")
	}
	baseBody := baseTail[:baseCloseRel+1]
	if regexp.MustCompile(`display:\s*flex;`).MatchString(baseBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: base mount CSS rule must NOT declare `display: flex;` — that's moved to the document-mode rule")
	}
	if regexp.MustCompile(`flex-direction:\s*column;`).MatchString(baseBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: base mount CSS rule must NOT declare `flex-direction: column;` — that's moved to the document-mode rule")
	}
	if regexp.MustCompile(`padding:\s*18px`).MatchString(baseBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: base mount CSS rule must NOT declare `padding: 18px 24px;` — that's moved to the document-mode rule")
	}

	// Graph-mode rule must be present and must NOT add padding/flex.
	graphSpec := `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-mount[data-mount-mode="graph"] {`
	graphIdx  := strings.Index(css, graphSpec)
	if graphIdx < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: graph-mode mount CSS rule %q must be present", graphSpec)
	}
	graphTail := css[graphIdx:]
	graphCloseRel := strings.Index(graphTail, "\n}\n")
	if graphCloseRel < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: graph-mode mount CSS rule body must be well-formed")
	}
	graphBody := graphTail[:graphCloseRel+1]
	if regexp.MustCompile(`display:\s*flex;`).MatchString(graphBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: graph-mode mount rule must NOT carry `display: flex;`")
	}
	if regexp.MustCompile(`padding:`).MatchString(graphBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: graph-mode mount rule must NOT carry padding (canvas fills the mount edge-to-edge)")
	}

	// Document-mode rule retains the flex-column shape so empty-state
	// text + flow-mode bands layout correctly.
	docSpec := `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-mount[data-mount-mode="document"] {`
	docIdx  := strings.Index(css, docSpec)
	if docIdx < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: document-mode mount CSS rule %q must be present", docSpec)
	}
	docTail := css[docIdx:]
	docCloseRel := strings.Index(docTail, "\n}\n")
	if docCloseRel < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: document-mode mount CSS rule body must be well-formed")
	}
	docBody := docTail[:docCloseRel+1]
	if !regexp.MustCompile(`display:\s*flex;`).MatchString(docBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: document-mode rule must keep `display: flex;` for empty-state / flow-layout content")
	}
}

// ── 7. Spatial canvas CSS fills the mount ─────────────────────────

// TestExplorer_D37sRawContext_CanvasCssFillsMount pins that the
// spatial-canvas CSS rule uses `position: absolute; inset: 0;` so the
// canvas fills the mount directly (Authority mount shape), not as a
// flex residual slot.
func TestExplorer_D37sRawContext_CanvasCssFillsMount(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37sRawContextCSS)

	spec := `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-canvas[data-spatial="true"] {`
	idx  := strings.Index(css, spec)
	if idx < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: spatial-canvas CSS rule %q must be present", spec)
	}
	tail := css[idx:]
	closeRel := strings.Index(tail, "\n}\n")
	if closeRel < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: spatial-canvas CSS rule body must be well-formed")
	}
	body := tail[:closeRel+1]

	if !regexp.MustCompile(`position:\s*absolute;`).MatchString(body) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: spatial-canvas rule must include `position: absolute;` so it fills the mount directly")
	}
	if !regexp.MustCompile(`inset:\s*0;`).MatchString(body) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: spatial-canvas rule must include `inset: 0;` to fill all four sides of the mount")
	}
	// Negative pin: pre-tranche flex-residual-slot shape must be gone.
	if regexp.MustCompile(`flex:\s*1\s+1\s+0;`).MatchString(body) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: spatial-canvas rule must NOT include `flex: 1 1 0;` — the canvas is no longer a flex residual slot in graph mode")
	}
}

// ── 8. Engine refresh used on repaint ─────────────────────────────

// TestExplorer_D37sRawContext_EngineRefreshUsedOnRepaint pins that
// the strategic spatial path calls `_engineHandle.refresh(...)` on
// subsequent paints in the same mode (not full destroy + remount).
func TestExplorer_D37sRawContext_EngineRefreshUsedOnRepaint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	// The same-mode branch calls engine.handle.refresh.
	if !regexp.MustCompile(`_engineHandle\.refresh\(\s*_buildContextEngineData\(cards,\s*connectors,\s*stage\)\s*\)`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: spatial repaint must call `_engineHandle.refresh(_buildContextEngineData(cards, connectors, stage))` on subsequent paints in the same mode (not full destroy+remount)")
	}

	// The mode-tracking variable exists; the same-mode gate consults it.
	if !strings.Contains(js, "var _currentRenderedMode = null;") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: renderer must declare `_currentRenderedMode` to track paint mode for refresh-vs-mount decisions")
	}
	if !regexp.MustCompile(`var sameMode = \(_currentRenderedMode === 'graph' && _engineHandle && _engineHandle\.refresh\)`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: _renderSpatialCytoscape must gate the refresh path on `sameMode = (_currentRenderedMode === 'graph' && _engineHandle && _engineHandle.refresh)`")
	}

	// engine.handle.refresh exists on the engine side (sanity).
	engineJS := getExplorerAsset(t, srv, d37sRawEngineAsset)
	if !strings.Contains(engineJS, "refresh: function (newData) {") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: engine handle must expose `refresh: function (newData)` for the lens to call")
	}
}

// ── 9. Overlay remains disabled ───────────────────────────────────

// TestExplorer_D37sRawContext_OverlayRemainsDisabled pins that the
// strategic spatial Context path still passes `overlayEnabled: false`
// (this tranche does NOT re-enable the overlay).
func TestExplorer_D37sRawContext_OverlayRemainsDisabled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	if !regexp.MustCompile(`overlayEnabled:\s*false`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: strategic spatial Context must still pass `overlayEnabled: false` — overlay re-enable is deferred to a follow-up tranche")
	}
}

// ── 10. Raw node visibility override retained ─────────────────────

// TestExplorer_D37sRawContext_RawNodeVisibilityOverrideRetained pins
// that the raw-node visibility override is still concat'd into the
// nodeStyleOverride.
func TestExplorer_D37sRawContext_RawNodeVisibilityOverrideRetained(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	if !strings.Contains(js, "function _buildContextRawNodeVisibilityOverride()") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: raw-node visibility override helper must remain")
	}
	if !regexp.MustCompile(`nodeStyleOverride:\s*_buildContextEdgeStyleOverride\(\)\.concat\(_buildContextRawNodeVisibilityOverride\(\)\)`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: nodeStyleOverride must remain `_buildContextEdgeStyleOverride().concat(_buildContextRawNodeVisibilityOverride())`")
	}
}

// ── 11. Engine.mount still used ───────────────────────────────────

// TestExplorer_D37sRawContext_EngineMountStillUsed pins that
// `engine.mount(canvas, {...})` remains the first-paint code path.
func TestExplorer_D37sRawContext_EngineMountStillUsed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	if !regexp.MustCompile(`engine\.mount\(canvas,\s*\{`).MatchString(js) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: first-paint path must call `engine.mount(canvas, {...})`")
	}
}

// ── 12. Native Context fallback preserved ─────────────────────────

// TestExplorer_D37sRawContext_NativeContextFallbackPreserved pins
// that the native Context default + GraphViewport adoption are
// unchanged.
func TestExplorer_D37sRawContext_NativeContextFallbackPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vp := getExplorerAsset(t, srv, d37sRawViewportAsset)

	if !strings.Contains(vp, "_baselineId = 'native-context';") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: GraphViewport must still adopt `native-context` as baseline")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: GraphViewport must still `adoptExisting('native-context')`")
	}
}

// ── 13. Authority unchanged ───────────────────────────────────────

// TestExplorer_D37sRawContext_AuthorityUnchanged pins that this
// tranche made no changes to Authority's load-bearing markers.
func TestExplorer_D37sRawContext_AuthorityUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	poc := getExplorerAsset(t, srv, d37sRawAuthorityAsset)

	for _, want := range []string{
		"_cy = window.cytoscape({",
		"viewport.register('authority',",
		"_installHtmlCardOverlay",
		"_fitToAvailableCanvas",
		"bus.registerLens('authority',",
	} {
		if !strings.Contains(poc, want) {
			t.Errorf("D37s-context-raw-cytoscape-1-impl: Authority marker %q must remain", want)
		}
	}

	// Authority must not contain Context-specific paint-mode tokens.
	for _, banned := range []string{
		"data-mount-mode",
		"_paintStrategicContext",
		"_engineByCardId",
		"_currentRenderedMode",
	} {
		if strings.Contains(poc, banned) {
			t.Errorf("D37s-context-raw-cytoscape-1-impl: Authority must NOT reference Context-specific token %q", banned)
		}
	}
}

// ── 14. Context surfaces preserved ────────────────────────────────

// TestExplorer_D37sRawContext_ContextSurfacesPreserved pins that
// selected-object pane, evidence tray, selection bridge, and camera
// bus contracts remain valid.
func TestExplorer_D37sRawContext_ContextSurfacesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	contextJS := getExplorerAsset(t, srv, d37sRawContextAsset)
	if !strings.Contains(contextJS, "bridge.selectCard(card)") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: Context must still call `bridge.selectCard(card)` (selection bridge contract)")
	}
	if !strings.Contains(contextJS, "function _buildContextEngineCameraDelegate(handle)") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: Context's engine-camera-delegate factory must remain (camera bus contract)")
	}

	paneJS := getExplorerAsset(t, srv, d37sRawPaneAsset)
	if len(paneJS) == 0 {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: Context selected-object-pane module must remain served")
	}

	trayJS := getExplorerAsset(t, srv, d37sRawEvidenceTrayAsset)
	if len(trayJS) == 0 {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: Context evidence-tray module must remain served")
	}

	selJS := getExplorerAsset(t, srv, d37sRawSelBridgeAsset)
	if len(selJS) == 0 {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: graph-selection-bridge module must remain served")
	}
	busJS := getExplorerAsset(t, srv, d37sRawCameraBusAsset)
	if len(busJS) == 0 {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: graph-camera-bus module must remain served")
	}
}

// ── 15. Empty-state path retained for missing projection ──────────

// TestExplorer_D37sRawContext_EmptyStatePathRetained pins that
// `_paintEmptyState` exists and is used when the projection or
// models are unavailable — but it does NOT carry skeleton identity.
func TestExplorer_D37sRawContext_EmptyStatePathRetained(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sRawContextAsset)

	if !strings.Contains(js, "function _paintEmptyState(message)") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: renderer must declare `_paintEmptyState(message)` for the empty-state branch")
	}
	// The empty-state path is reachable from the production paint.
	pIdx := strings.Index(js, "function _paintStrategicContext()")
	if pIdx < 0 {
		t.Fatal("D37s-context-raw-cytoscape-1-impl: _paintStrategicContext must exist")
	}
	pTail := js[pIdx:]
	pEndRel := strings.Index(pTail[1:], "\n  function ")
	if pEndRel < 0 {
		t.Fatalf("D37s-context-raw-cytoscape-1-impl: _paintStrategicContext body must be well-formed")
	}
	pBody := pTail[:pEndRel+1]
	if !strings.Contains(pBody, "_paintEmptyState(") {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: _paintStrategicContext must dispatch to _paintEmptyState when projection/models are unavailable")
	}

	// The empty-state must NOT contain skeleton identity text.
	if strings.Contains(js, `'Awaiting Context projection. The skeleton will populate model summary when projection data is available.'`) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl: empty-state text must NOT use skeleton identity language — replaced by neutral wording")
	}
}
