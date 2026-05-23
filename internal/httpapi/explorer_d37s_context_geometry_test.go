package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37s-context-geometry-1-impl — Strategic Context Renderer Geometry Tranche
//
// Three converging contract changes land in this tranche:
//
//   Leak 1 — Cy node dimensions are derived from the overlay's measured
//            card footprint, not from the lens's stage-supplied constants.
//            Connectors clip at the visible card boundary instead of a
//            fixed 220×64 model footprint.
//
//   Leak 2 — Engine fit consumes per-side safe-area insets from a
//            mount-supplied `getSafeArea` callback. `handle.fit(opts)`
//            accepts `opts.padding` as either a scalar (back-compat)
//            or a per-side `{top, right, bottom, left}` object.
//            Internal algorithm mirrors Authority's `_fitToAvailableCanvas`
//            including the `FIT_MIN_VISIBLE_PX` degenerate-viewport
//            guard.
//
//   Leak 3 — Context lens stops hard-sizing the envelope. `canvas.style.
//            width / .height` writes removed; the spatial-canvas CSS
//            rule gains `flex: 1 1 0; min-height: 0` so the canvas
//            fills the remaining vertical slot. Horizontal overflow
//            structurally eliminated.
//
// These tests pin the source contracts. Browser validation (items
// 2/3/4/6 in the approval) is the user-side runtime gate.

const (
	d37sEngineAsset   = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37sOverlayAsset  = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js"
	d37sContextAsset  = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37sContextCSS    = "/explorer/assets/css/context-cytoscape-renderer.css"
)

// ── 1. Engine propagates measured dims to cy node ─────────────────

// TestExplorer_D37s_EnginePropagatesMeasuredDimsToCyNode pins that the
// engine declares a `_propagateDimensions` function that writes the
// overlay's measured card footprint back to the corresponding cy node's
// `data.width` / `data.height`. This is the load-bearing change for
// Leak 1 (connector endpoints clip at visible card boundary).
func TestExplorer_D37s_EnginePropagatesMeasuredDimsToCyNode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sEngineAsset)

	if !strings.Contains(js, "function _propagateDimensions(cy, key, w, h)") {
		t.Errorf("D37s-context-geometry-1-impl: engine must declare _propagateDimensions(cy, key, w, h)")
	}
	// The function writes via cy.getElementById + n.data({width, height}).
	pIdx := strings.Index(js, "function _propagateDimensions(cy, key, w, h)")
	if pIdx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _propagateDimensions must be present")
	}
	tail := js[pIdx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _propagateDimensions body must be well-formed")
	}
	body := tail[:endRel+1]
	if !strings.Contains(body, "cy.getElementById(_str(key))") {
		t.Errorf("D37s-context-geometry-1-impl: _propagateDimensions must look up the cy node via cy.getElementById(_str(key))")
	}
	if !regexp.MustCompile(`n\.data\(\{\s*width:\s*w,\s*height:\s*h\s*\}\)`).MatchString(body) {
		t.Errorf("D37s-context-geometry-1-impl: _propagateDimensions must write `n.data({width: w, height: h})` so the cy 'node' style's data(width)/data(height) bindings re-size the node bounding box")
	}

	// The engine wires the overlay's onMeasure callback to invoke
	// _propagateDimensions(cy, key, w, h). This is the load-bearing
	// flow: overlay measures → onMeasure → engine propagates → cy
	// re-routes edges.
	//
	// D37s-context-geometry-2-impl flip: the onMeasure callback body
	// expanded from a single-line passthrough to a multi-line block
	// that ALSO updates the measurement cache, schedules the lens
	// callback, and schedules overlap validation. The load-bearing
	// `_propagateDimensions(cy, key, w, h)` call must remain inside
	// the onMeasure body — the regex is loosened to match either the
	// pre-tranche single-line shape OR the post-tranche multi-line
	// shape so this test continues to assert the propagation flow
	// without over-constraining the body's other contents.
	if !regexp.MustCompile(`(?s)onMeasure:\s*function\s*\(key,\s*w,\s*h\)\s*\{[\s\S]*?_propagateDimensions\(cy,\s*key,\s*w,\s*h\)`).MatchString(js) {
		t.Errorf("D37s-context-geometry-1-impl (flipped for D37s-2): engine.mount's `onMeasure: function (key, w, h)` body must contain `_propagateDimensions(cy, key, w, h)` so dimension changes flow from overlay measurement to cy.data() write")
	}
}

// ── 2. Dim-propagation threshold guard ────────────────────────────

// TestExplorer_D37s_DimPropagationGuardedBy05pxThreshold pins the
// sub-pixel feedback-loop guard. Without this guard, a fractional
// getBoundingClientRect reading would trigger continuous cy.data
// writes and re-renders.
func TestExplorer_D37s_DimPropagationGuardedBy05pxThreshold(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sEngineAsset)

	// The threshold constant is documented and load-bearing.
	if !strings.Contains(js, "var DIM_PROPAGATION_THRESHOLD = 0.5;") {
		t.Errorf("D37s-context-geometry-1-impl: engine must declare `var DIM_PROPAGATION_THRESHOLD = 0.5;` as the sub-pixel feedback-loop guard")
	}

	pIdx := strings.Index(js, "function _propagateDimensions(cy, key, w, h)")
	if pIdx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _propagateDimensions must exist")
	}
	tail := js[pIdx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _propagateDimensions body must be well-formed")
	}
	body := tail[:endRel+1]

	// Both width and height deltas must be checked against the threshold.
	if !regexp.MustCompile(`wDelta\s*=\s*isFinite\(curW\)\s*\?\s*Math\.abs\(w\s*-\s*curW\)`).MatchString(body) {
		t.Errorf("D37s-context-geometry-1-impl: _propagateDimensions must compute wDelta = isFinite(curW) ? Math.abs(w - curW) : Infinity")
	}
	if !regexp.MustCompile(`hDelta\s*=\s*isFinite\(curH\)\s*\?\s*Math\.abs\(h\s*-\s*curH\)`).MatchString(body) {
		t.Errorf("D37s-context-geometry-1-impl: _propagateDimensions must compute hDelta = isFinite(curH) ? Math.abs(h - curH) : Infinity")
	}
	if !strings.Contains(body, "if (wDelta < DIM_PROPAGATION_THRESHOLD && hDelta < DIM_PROPAGATION_THRESHOLD) return;") {
		t.Errorf("D37s-context-geometry-1-impl: _propagateDimensions must early-return when BOTH wDelta and hDelta are below the 0.5-px threshold (sub-pixel noise filter)")
	}

	// PROPAGATION CONTRACT documentation block is present and names
	// the load-bearing rule.
	if !strings.Contains(js, "PROPAGATION CONTRACT") {
		t.Errorf("D37s-context-geometry-1-impl: engine must document the PROPAGATION CONTRACT at _propagateDimensions")
	}
	for _, phrase := range []string{
		"Loop shape",
		"Guard:",
		"Events that MUST NOT re-trigger propagation",
		"cy 'position'",
		"cy 'pan', 'zoom', 'render'",
	} {
		if !strings.Contains(js, phrase) {
			t.Errorf("D37s-context-geometry-1-impl: PROPAGATION CONTRACT must include %q", phrase)
		}
	}
}

// ── 3. Engine fit consumes safe-area ──────────────────────────────

// TestExplorer_D37s_EngineFitConsumesSafeArea pins that `handle.fit()`
// pulls per-side insets from `opts.getSafeArea` (mount option) and
// applies them via `cy.viewport({zoom, pan})` — NOT via the pre-tranche
// scalar `cy.fit(undefined, 24)`.
func TestExplorer_D37s_EngineFitConsumesSafeArea(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sEngineAsset)

	// _resolveFitPadding helper exists and pulls from getSafeArea
	// when no explicit padding is supplied.
	if !strings.Contains(js, "function _resolveFitPadding(input, getSafeArea)") {
		t.Errorf("D37s-context-geometry-1-impl: engine must declare _resolveFitPadding(input, getSafeArea) normaliser")
	}

	// _fitWithSafeArea helper exists and applies via cy.viewport({zoom, pan}).
	if !strings.Contains(js, "function _fitWithSafeArea(cy, padding)") {
		t.Errorf("D37s-context-geometry-1-impl: engine must declare _fitWithSafeArea(cy, padding) per-side fit algorithm")
	}
	fIdx := strings.Index(js, "function _fitWithSafeArea(cy, padding)")
	if fIdx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _fitWithSafeArea must exist")
	}
	fTail := js[fIdx:]
	fEndRel := strings.Index(fTail[1:], "\n  function ")
	if fEndRel < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _fitWithSafeArea body must be well-formed")
	}
	fBody := fTail[:fEndRel+1]
	if !regexp.MustCompile(`cy\.viewport\(\{\s*zoom:\s*z,\s*pan:\s*\{\s*x:\s*rcx\s*-\s*gx\s*\*\s*z,\s*y:\s*rcy\s*-\s*gy\s*\*\s*z\s*\}\s*\}\)`).MatchString(fBody) {
		t.Errorf("D37s-context-geometry-1-impl: _fitWithSafeArea must apply the computed transform atomically via cy.viewport({zoom, pan})")
	}

	// handle.fit accepts opts (object with optional padding) and
	// pulls from opts.getSafeArea when no padding is supplied.
	if !regexp.MustCompile(`fit:\s*function\s*\(fitOpts\)`).MatchString(js) {
		t.Errorf("D37s-context-geometry-1-impl: handle.fit must accept an opts parameter (fitOpts) for per-call padding")
	}
	if !regexp.MustCompile(`_resolveFitPadding\(padInput,\s*opts\.getSafeArea\)`).MatchString(js) {
		t.Errorf("D37s-context-geometry-1-impl: handle.fit must call _resolveFitPadding(padInput, opts.getSafeArea) so mount-supplied safe-area drives the fit when no explicit padding is passed")
	}

	// _runFitPipeline (the shared helper called by both _settle and
	// _runResizeRefit per D37s-strategic-fit-resize-lifecycle-impl)
	// composes initial-fit padding from the lens-supplied safe area
	// and applies it via _fitWithSafeArea. _settle is a one-liner
	// wrapper over _runFitPipeline; asserting on the helper covers
	// both the initial-fit path and the resize-driven refit path.
	sIdx := strings.Index(js, "function _runFitPipeline(source, phase)")
	if sIdx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _runFitPipeline(source, phase) must exist (post-D37ad-initial-fit-stable-cadence source-tagged shared helper)")
	}
	sTail := js[sIdx:]
	endBrace := strings.Index(sTail, "\n    }\n")
	if endBrace < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _runFitPipeline body must be well-formed")
	}
	sBody := sTail[:endBrace+1]
	if !strings.Contains(sBody, "_resolveFitPadding(undefined, opts.getSafeArea)") {
		t.Errorf("D37s-context-geometry-1-impl: _runFitPipeline must call _resolveFitPadding(undefined, opts.getSafeArea) to compose initial-fit padding from the lens-supplied safe-area")
	}
	if !strings.Contains(sBody, "_fitWithSafeArea(cy, padding)") {
		t.Errorf("D37s-context-geometry-1-impl: _runFitPipeline must call _fitWithSafeArea(cy, padding) instead of the pre-tranche cy.fit(undefined, 24)")
	}

	// Negative pin: the pre-tranche `cy.fit(undefined, 24)` shape MUST
	// be gone from the engine's EXECUTABLE code (comments documenting
	// the pre-tranche shape are intentionally retained for context).
	// Iterate non-comment lines and check none contain the call.
	for _, line := range strings.Split(js, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		if regexp.MustCompile(`cy\.fit\(undefined,\s*24\)`).MatchString(line) {
			t.Errorf("D37s-context-geometry-1-impl: the pre-tranche `cy.fit(undefined, 24)` hardcoded scalar fit MUST be removed from executable code (found on non-comment line: %q) — replaced with _fitWithSafeArea", strings.TrimSpace(line))
		}
	}
}

// ── 4. Engine fit degenerate-viewport guard ───────────────────────

// TestExplorer_D37s_EngineFitDegenerateViewportGuard pins the
// FIT_MIN_VISIBLE_PX clamp that prevents per-side insets from
// collapsing the visible region.
func TestExplorer_D37s_EngineFitDegenerateViewportGuard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sEngineAsset)

	// FIT_MIN_VISIBLE_PX constant matches Authority's value (96).
	if !strings.Contains(js, "var FIT_MIN_VISIBLE_PX  = 96;") {
		t.Errorf("D37s-context-geometry-1-impl: engine must declare `var FIT_MIN_VISIBLE_PX  = 96;` (mirrors Authority's degenerate-viewport floor)")
	}

	fIdx := strings.Index(js, "function _fitWithSafeArea(cy, padding)")
	if fIdx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _fitWithSafeArea must exist")
	}
	fTail := js[fIdx:]
	fEndRel := strings.Index(fTail[1:], "\n  function ")
	if fEndRel < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _fitWithSafeArea body must be well-formed")
	}
	fBody := fTail[:fEndRel+1]

	// The guard scales L+R back proportionally when they would collapse cw below FIT_MIN_VISIBLE_PX.
	if !regexp.MustCompile(`if \(cw - L - R < FIT_MIN_VISIBLE_PX\)`).MatchString(fBody) {
		t.Errorf("D37s-context-geometry-1-impl: _fitWithSafeArea must guard against horizontal viewport collapse via `if (cw - L - R < FIT_MIN_VISIBLE_PX)`")
	}
	if !regexp.MustCompile(`if \(ch - T - B < FIT_MIN_VISIBLE_PX\)`).MatchString(fBody) {
		t.Errorf("D37s-context-geometry-1-impl: _fitWithSafeArea must guard against vertical viewport collapse via `if (ch - T - B < FIT_MIN_VISIBLE_PX)`")
	}
	// The visible-width/height computations use the FIT_MIN_VISIBLE_PX floor.
	if !regexp.MustCompile(`var vw = Math\.max\(FIT_MIN_VISIBLE_PX,\s*cw - L - R\);`).MatchString(fBody) {
		t.Errorf("D37s-context-geometry-1-impl: _fitWithSafeArea must compute vw = Math.max(FIT_MIN_VISIBLE_PX, cw - L - R)")
	}
	if !regexp.MustCompile(`var vh = Math\.max\(FIT_MIN_VISIBLE_PX,\s*ch - T - B\);`).MatchString(fBody) {
		t.Errorf("D37s-context-geometry-1-impl: _fitWithSafeArea must compute vh = Math.max(FIT_MIN_VISIBLE_PX, ch - T - B)")
	}
}

// ── 5. Context lens stops hard-sizing the envelope ────────────────

// TestExplorer_D37s_ContextLensEnvelopeNotHardSized pins that
// `_renderSpatialCytoscape` no longer writes
// `canvas.style.width = stage.dimensions.width + 'px'` / .height.
// This is the load-bearing change for Leak 3 (horizontal overflow).
func TestExplorer_D37s_ContextLensEnvelopeNotHardSized(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sContextAsset)

	// Locate the _renderSpatialCytoscape body.
	idx := strings.Index(js, "function _renderSpatialCytoscape(layout, cards, connectors, stage, byId, painter)")
	if idx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _renderSpatialCytoscape must be present")
	}
	tail := js[idx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _renderSpatialCytoscape body must be well-formed")
	}
	body := tail[:endRel+1]

	// Negative pins: the inline pixel-size writes MUST be absent from
	// the engine path's canvas creation. If a future change re-adds
	// them, this test catches the regression.
	if regexp.MustCompile(`canvas\.style\.width\s*=\s*width\s*\+\s*'px'`).MatchString(body) {
		t.Errorf("D37s-context-geometry-1-impl: _renderSpatialCytoscape must NOT set canvas.style.width = width + 'px' — the slot-sized envelope replaces hard-sizing")
	}
	if regexp.MustCompile(`canvas\.style\.height\s*=\s*height\s*\+\s*'px'`).MatchString(body) {
		t.Errorf("D37s-context-geometry-1-impl: _renderSpatialCytoscape must NOT set canvas.style.height = height + 'px' — the slot-sized envelope replaces hard-sizing")
	}

	// Diagnostic attributes (data-stage-width / data-stage-height) are
	// retained as informational markers — but they no longer drive
	// layout. Just confirm they're present so stage-aware tooling can
	// still introspect.
	if !strings.Contains(body, `canvas.setAttribute('data-stage-width',  String(width))`) {
		t.Errorf("D37s-context-geometry-1-impl: _renderSpatialCytoscape should retain `data-stage-width` as a diagnostic attribute (no longer a layout driver)")
	}
	if !strings.Contains(body, `canvas.setAttribute('data-stage-height', String(height))`) {
		t.Errorf("D37s-context-geometry-1-impl: _renderSpatialCytoscape should retain `data-stage-height` as a diagnostic attribute")
	}

	// Spatial-canvas CSS rule MUST gain `flex: 1 1 0; min-height: 0`
	// so the (now un-pixel-sized) canvas fills the remaining flex
	// space in _mountEl. This is the load-bearing CSS extension.
	css := getExplorerAsset(t, srv, d37sContextCSS)
	specificity := `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-canvas[data-spatial="true"]`
	ruleIdx := strings.Index(css, specificity+" {")
	if ruleIdx < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: spatial-canvas CSS rule %q must be present", specificity)
	}
	// Slice the rule body until the closing brace at column 0 (the rule's `}` on its own line).
	ruleTail := css[ruleIdx:]
	closeIdx := strings.Index(ruleTail, "\n}\n")
	if closeIdx < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: spatial-canvas CSS rule body must be well-formed")
	}
	ruleBody := ruleTail[:closeIdx+1]
	// D37s-context-raw-cytoscape-1-impl flip: the spatial-canvas
	// rule no longer carries `flex: 1 1 0; min-height: 0;`. The
	// pre-tranche shape assumed a flex-column mount with the
	// skeleton banner consuming top vertical space and the canvas
	// growing into the residual flex slot. D37s-context-raw-cytoscape-
	// 1-impl retires the skeleton banner from the live paint path
	// and switches the mount to `data-mount-mode="graph"` (positioned
	// container, no flex column). The canvas becomes the sole child
	// of the mount and `position: absolute; inset: 0` fills it
	// directly — Authority's mount shape. The new pin asserts the
	// absolute-positioned canvas fills the mount.
	if !regexp.MustCompile(`position:\s*absolute;`).MatchString(ruleBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl (flipped): spatial-canvas CSS rule must include `position: absolute;` so the canvas fills the mount slot directly (mount is no longer flex column)")
	}
	if !regexp.MustCompile(`inset:\s*0;`).MatchString(ruleBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl (flipped): spatial-canvas CSS rule must include `inset: 0;` so the canvas fills all four sides of the mount")
	}
	// Negative pin: pre-tranche flex-residual-slot shape MUST be gone.
	if regexp.MustCompile(`flex:\s*1\s+1\s+0;`).MatchString(ruleBody) {
		t.Errorf("D37s-context-raw-cytoscape-1-impl (flipped): spatial-canvas CSS rule must NOT include `flex: 1 1 0;` — the mount is no longer flex-column in graph mode")
	}
}

// ── 6. Context lens passes getSafeArea to engine.mount ────────────

// TestExplorer_D37s_ContextLensPassesGetSafeAreaToEngine pins that
// `_renderSpatialCytoscape`'s call to `engine.mount(canvas, {…})`
// includes a `getSafeArea` option that pulls from `_hostCtx.getSafeArea`.
func TestExplorer_D37s_ContextLensPassesGetSafeAreaToEngine(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sContextAsset)

	// The engine.mount call must include a `getSafeArea` option that
	// is a function reading from _hostCtx.getSafeArea (the GraphViewport
	// host's live chrome-measured safe-area source).
	if !regexp.MustCompile(`(?s)engine\.mount\(canvas,\s*\{[\s\S]*?getSafeArea:\s*function\s*\(\)\s*\{[\s\S]*?_hostCtx\.getSafeArea\(\)`).MatchString(js) {
		t.Errorf("D37s-context-geometry-1-impl: Context lens must pass `getSafeArea: function () { …_hostCtx.getSafeArea()… }` to engine.mount so the engine consumes the GraphViewport host's live chrome-measured insets")
	}

	// The callback must guard against a missing _hostCtx (defensive at
	// paint time — e.g. mid-mount teardown).
	if !regexp.MustCompile(`if \(!_hostCtx \|\| typeof _hostCtx\.getSafeArea !== 'function'\) return null;`).MatchString(js) {
		t.Errorf("D37s-context-geometry-1-impl: Context lens's getSafeArea callback must guard against _hostCtx being null or missing getSafeArea before calling")
	}
}

// ── 7. Overlay invokes onMeasure callback ─────────────────────────

// TestExplorer_D37s_OverlayInvokesOnMeasureCallback pins that the
// shared overlay accepts an `onMeasure(key, w, h)` mount option and
// invokes it after every successful measurement (both the synchronous
// mount-time measure and the per-card ResizeObserver tick).
func TestExplorer_D37s_OverlayInvokesOnMeasureCallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sOverlayAsset)

	// The overlay captures onMeasure from opts.
	if !strings.Contains(js, "var onMeasure = _isFn(opts.onMeasure) ? opts.onMeasure : null;") {
		t.Errorf("D37s-context-geometry-1-impl: shared overlay must capture `onMeasure` from opts")
	}

	// _notifyMeasure helper exists and invokes onMeasure with key + dims.
	if !strings.Contains(js, "function _notifyMeasure(entry)") {
		t.Errorf("D37s-context-geometry-1-impl: shared overlay must declare _notifyMeasure(entry)")
	}
	nIdx := strings.Index(js, "function _notifyMeasure(entry)")
	if nIdx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _notifyMeasure must exist")
	}
	nTail := js[nIdx:]
	nEndRel := strings.Index(nTail[1:], "\n    function ")
	if nEndRel < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _notifyMeasure body must be well-formed")
	}
	nBody := nTail[:nEndRel+1]
	if !strings.Contains(nBody, "onMeasure(key, entry.measuredWidth, entry.measuredHeight)") {
		t.Errorf("D37s-context-geometry-1-impl: _notifyMeasure must invoke `onMeasure(key, entry.measuredWidth, entry.measuredHeight)` so the engine can propagate dimensions to cy")
	}
	if !strings.Contains(nBody, "entry.wrapper.getAttribute('data-overlay-key')") {
		t.Errorf("D37s-context-geometry-1-impl: _notifyMeasure must read the cy node key from the wrapper's `data-overlay-key` attribute")
	}

	// _build invokes _notifyMeasure after a successful initial measure.
	bIdx := strings.Index(js, "function _build()")
	if bIdx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _build must exist")
	}
	bTail := js[bIdx:]
	bEndRel := strings.Index(bTail[1:], "\n    function ")
	if bEndRel < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _build body must be well-formed")
	}
	bBody := bTail[:bEndRel+1]
	if !regexp.MustCompile(`if \(_measureCard\(entry\)\)\s*\{[\s\S]*?_notifyMeasure\(entry\)`).MatchString(bBody) {
		t.Errorf("D37s-context-geometry-1-impl: _build must call _notifyMeasure(entry) after a successful initial _measureCard so the engine sees the first measurement at mount time")
	}

	// The per-card ResizeObserver callback in _observeCard also
	// invokes _notifyMeasure on every actual size change.
	oIdx := strings.Index(js, "function _observeCard(entry)")
	if oIdx < 0 {
		t.Fatal("D37s-context-geometry-1-impl: _observeCard must exist")
	}
	oTail := js[oIdx:]
	oEndRel := strings.Index(oTail[1:], "\n    function ")
	if oEndRel < 0 {
		t.Fatalf("D37s-context-geometry-1-impl: _observeCard body must be well-formed")
	}
	oBody := oTail[:oEndRel+1]
	if !regexp.MustCompile(`_notifyMeasure\(entry\);\s*\n\s*_syncCard\(entry\);`).MatchString(oBody) {
		t.Errorf("D37s-context-geometry-1-impl: _observeCard's ResizeObserver callback must call _notifyMeasure(entry) BEFORE _syncCard(entry) so the engine propagates dimensions to cy before the overlay re-centres the wrapper")
	}
}
