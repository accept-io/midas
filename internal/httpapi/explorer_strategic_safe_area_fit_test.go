package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37s-viewport-fit-1-impl — Strategic Safe-Area Fit Envelope.
//
// Platform contract:
//
//   GraphViewport.getUsableGraphRect() returns the actual usable graph
//     area (viewport rect minus all MIDAS chrome that overlays it).
//   graphCytoscapeEngine consumes the usable rect in `_fitToUsableRect`
//     and applies pan/zoom via `cy.viewport({...})`. The engine does
//     NOT compute chrome offsets itself.
//   Lenses pass `_hostCtx.getUsableGraphRect` through to engine.mount
//     as the `getUsableGraphRect` option. Lenses MUST NOT compute
//     chrome offsets.
//   Authority remains bespoke pending B''-Authority migration;
//     whitelisted by source-evidenced commitment.
//
// These tests pin the source-contract surface. Browser validation
// (graph fits inside the actual visible area; no nodes clipped by
// drawer / tray / toolbar / camera cluster) is the user-side gate.

const (
	d37svfViewportAsset  = "/explorer/assets/js/graph/graph-viewport.js"
	d37svfEngineAsset    = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37svfContextAsset   = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37svfAuthorityAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

// ── 1. GraphViewport exposes getUsableGraphRect ───────────────────

// TestExplorer_StrategicFit_GraphViewportExposesUsableGraphRect pins
// that GraphViewport exposes an explicit usable-graph-rect contract.
func TestExplorer_StrategicFit_GraphViewportExposesUsableGraphRect(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37svfViewportAsset)

	if !strings.Contains(js, "function getUsableGraphRect()") {
		t.Errorf("D37s-viewport-fit-1: GraphViewport must declare `getUsableGraphRect()` to expose the platform usable graph rectangle")
	}

	// Return shape: { x, y, width, height, insets: {...} }.
	rIdx := strings.Index(js, "function getUsableGraphRect()")
	if rIdx < 0 {
		t.Fatal("D37s-viewport-fit-1: getUsableGraphRect must exist")
	}
	rTail := js[rIdx:]
	rEndRel := strings.Index(rTail[1:], "\n  function ")
	if rEndRel < 0 {
		t.Fatalf("D37s-viewport-fit-1: getUsableGraphRect body must be well-formed")
	}
	rBody := rTail[:rEndRel+1]

	for _, want := range []string{
		"x:",
		"y:",
		"width:",
		"height:",
		"insets:",
	} {
		if !strings.Contains(rBody, want) {
			t.Errorf("D37s-viewport-fit-1: getUsableGraphRect return shape must include %q", want)
		}
	}

	// Public export retains the new method.
	if !regexp.MustCompile(`getUsableGraphRect:\s+getUsableGraphRect,`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: GraphViewport.viewport public surface must export getUsableGraphRect")
	}

	// Renderer ctx (passed to factory.mount) carries the new key.
	if !regexp.MustCompile(`getUsableGraphRect:\s+getUsableGraphRect,`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: renderer ctx must carry getUsableGraphRect")
	}
}

// ── 2. Chrome inventory includes the right drawer ─────────────────

// TestExplorer_StrategicFit_ChromeInventoryIncludesRightDrawer pins
// that the chrome inventory includes the right drawer (`gmap-right-
// rail`), which overlays the graph from outside the viewport DOM tree.
func TestExplorer_StrategicFit_ChromeInventoryIncludesRightDrawer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37svfViewportAsset)

	// The CHROME_CLASSES array must include the right-drawer class.
	if !regexp.MustCompile(`'gmap-right-rail'`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: CHROME_CLASSES must include 'gmap-right-rail' (right drawer overlays the graph from outside the viewport DOM tree)")
	}

	// Pre-tranche inventory members preserved.
	for _, want := range []string{
		"'gmap-mode-rail'",
		"'gmap-camera-cluster'",
		"'gmap-legend-overlay'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37s-viewport-fit-1: CHROME_CLASSES must retain %q", want)
		}
	}
}

// ── 3. Aspect-based attribution + intersection rect ───────────────

// TestExplorer_StrategicFit_UsesBoundingRectsNotOnlyQuadrantHeuristics
// pins that the safe-area implementation uses actual intersection
// bounding rects + aspect-based axis attribution, NOT the pre-tranche
// quadrant heuristic that over-reserved entire edges based on corner
// position.
func TestExplorer_StrategicFit_UsesBoundingRectsNotOnlyQuadrantHeuristics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37svfViewportAsset)

	// Aspect-based attribution constants must exist.
	if !strings.Contains(js, "var ASPECT_WIDE_THRESHOLD = 1.5;") {
		t.Errorf("D37s-viewport-fit-1: GraphViewport must declare ASPECT_WIDE_THRESHOLD = 1.5")
	}
	if !strings.Contains(js, "var ASPECT_TALL_THRESHOLD = 0.67;") {
		t.Errorf("D37s-viewport-fit-1: GraphViewport must declare ASPECT_TALL_THRESHOLD = 0.67")
	}

	// Intersection of chrome rect and viewport rect must be computed.
	if !regexp.MustCompile(`Math\.max\(r\.left,\s+vpRect\.left\)`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: safe-area implementation must intersect chrome rect with viewport rect (Math.max(r.left, vpRect.left))")
	}
	if !regexp.MustCompile(`Math\.min\(r\.right,\s+vpRect\.right\)`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: safe-area implementation must intersect chrome rect with viewport rect (Math.min(r.right, vpRect.right))")
	}

	// Aspect classification gates contributions per axis.
	if !regexp.MustCompile(`var horizontalOnly = \(aspect >= ASPECT_WIDE_THRESHOLD\);`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: safe-area must classify wide chrome (aspect ≥ ASPECT_WIDE_THRESHOLD) as horizontalOnly")
	}
	if !regexp.MustCompile(`var verticalOnly\s+= \(aspect <= ASPECT_TALL_THRESHOLD\);`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: safe-area must classify tall chrome (aspect ≤ ASPECT_TALL_THRESHOLD) as verticalOnly")
	}

	// Pre-tranche quadrant heuristic must be GONE.
	// The pre-tranche shape used `var chromeCx = r.left + r.width / 2`
	// + `var vpCx = vpRect.left + vpRect.width / 2` to classify by
	// quadrant; the new shape doesn't use these chrome-centre / vp-
	// centre comparisons.
	if regexp.MustCompile(`var chromeCx\s*=\s*r\.left\s*\+\s*r\.width\s*/\s*2;`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: pre-tranche quadrant heuristic (`var chromeCx = r.left + r.width / 2`) must be GONE — replaced with intersection + aspect attribution")
	}
	if regexp.MustCompile(`if \(chromeCx <= vpCx`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: pre-tranche quadrant comparison `if (chromeCx <= vpCx)` must be GONE — replaced with edge-proximity + aspect attribution")
	}
}

// ── 4. Engine fits to usable rect ─────────────────────────────────

// TestExplorer_StrategicFit_EngineFitsToUsableRect pins that the
// engine declares `_fitToUsableRect(cy, usableRect, emitDiag)` and
// that `_settle` / `handle.fit` route through it when
// `opts.getUsableGraphRect` is supplied.
func TestExplorer_StrategicFit_EngineFitsToUsableRect(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37svfEngineAsset)

	if !strings.Contains(js, "function _fitToUsableRect(cy, usableRect, emitDiag)") {
		t.Errorf("D37s-viewport-fit-1: engine must declare _fitToUsableRect(cy, usableRect, emitDiag)")
	}

	// _fitToUsableRect must apply atomically via cy.viewport({zoom, pan}).
	fIdx := strings.Index(js, "function _fitToUsableRect(cy, usableRect, emitDiag)")
	if fIdx < 0 {
		t.Fatal("D37s-viewport-fit-1: _fitToUsableRect must exist")
	}
	fTail := js[fIdx:]
	fEndRel := strings.Index(fTail[1:], "\n  function ")
	if fEndRel < 0 {
		t.Fatalf("D37s-viewport-fit-1: _fitToUsableRect body must be well-formed")
	}
	fBody := fTail[:fEndRel+1]
	if !regexp.MustCompile(`cy\.viewport\(\{\s*zoom:\s*z,\s*pan:\s*\{`).MatchString(fBody) {
		t.Errorf("D37s-viewport-fit-1: _fitToUsableRect must apply zoom+pan atomically via cy.viewport({zoom, pan})")
	}

	// _settle prefers the usable-rect path.
	sIdx := strings.Index(js, "function _settle()")
	if sIdx < 0 {
		t.Fatal("D37s-viewport-fit-1: _settle must exist")
	}
	sTail := js[sIdx:]
	sEnd := strings.Index(sTail, "\n    }\n")
	if sEnd < 0 {
		t.Fatalf("D37s-viewport-fit-1: _settle body must be well-formed")
	}
	sBody := sTail[:sEnd+1]
	if !strings.Contains(sBody, "opts.getUsableGraphRect()") {
		t.Errorf("D37s-viewport-fit-1: _settle must read opts.getUsableGraphRect() before falling back to safe-area padding")
	}
	if !strings.Contains(sBody, "_fitToUsableRect(cy, usable, _recordFitDiagnostic)") {
		t.Errorf("D37s-viewport-fit-1: _settle must call _fitToUsableRect(cy, usable, _recordFitDiagnostic) when a non-zero rect is available")
	}

	// handle.fit also prefers the usable-rect path.
	if !regexp.MustCompile(`fit:\s*function\s*\(fitOpts\)`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: handle.fit must accept fitOpts")
	}
	// The fit body should reference getUsableGraphRect.
	hIdx := strings.Index(js, "fit: function (fitOpts) {")
	if hIdx < 0 {
		t.Fatal("D37s-viewport-fit-1: handle.fit definition must be present")
	}
	hTail := js[hIdx:]
	hEnd := strings.Index(hTail, "\n      },\n")
	if hEnd < 0 {
		t.Fatalf("D37s-viewport-fit-1: handle.fit body must be well-formed")
	}
	hBody := hTail[:hEnd+1]
	if !strings.Contains(hBody, "opts.getUsableGraphRect()") {
		t.Errorf("D37s-viewport-fit-1: handle.fit must consult opts.getUsableGraphRect() when no explicit padding is supplied")
	}
}

// ── 5. Engine handles asymmetric insets ───────────────────────────

// TestExplorer_StrategicFit_EngineHandlesAsymmetricInsets pins that
// the engine's fit pipeline uses per-side asymmetric data, NOT scalar
// padding that collapses asymmetric chrome.
func TestExplorer_StrategicFit_EngineHandlesAsymmetricInsets(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37svfEngineAsset)

	// _fitToUsableRect computes pan against the usable rect's x/y,
	// not a scalar.
	fIdx := strings.Index(js, "function _fitToUsableRect(cy, usableRect, emitDiag)")
	if fIdx < 0 {
		t.Fatal("D37s-viewport-fit-1: _fitToUsableRect must exist")
	}
	fTail := js[fIdx:]
	fEndRel := strings.Index(fTail[1:], "\n  function ")
	if fEndRel < 0 {
		t.Fatalf("D37s-viewport-fit-1: _fitToUsableRect body must be well-formed")
	}
	fBody := fTail[:fEndRel+1]

	if !regexp.MustCompile(`var rcx = ux \+ uw / 2;`).MatchString(fBody) {
		t.Errorf("D37s-viewport-fit-1: _fitToUsableRect must compute viewport-centre X as usable.x + usable.width/2 (asymmetric)")
	}
	if !regexp.MustCompile(`var rcy = uy \+ uh / 2;`).MatchString(fBody) {
		t.Errorf("D37s-viewport-fit-1: _fitToUsableRect must compute viewport-centre Y as usable.y + usable.height/2 (asymmetric)")
	}

	// Diagnostic codes for fit-envelope conditions must exist.
	// Alignment-tolerant — the constants are declared with varied
	// whitespace after the `=` for readability.
	for _, code := range []string{
		"DIAG_USABLE_RECT_EMPTY",
		"DIAG_FIT_BOUNDS_EMPTY",
		"DIAG_FIT_ZOOM_CLAMPED_MIN",
		"DIAG_FIT_ZOOM_CLAMPED_MAX",
		"DIAG_USABLE_RECT_SMALLER_THAN_MIN",
	} {
		if !regexp.MustCompile(`var\s+` + code + `\s+=`).MatchString(js) {
			t.Errorf("D37s-viewport-fit-1: engine must declare fit-diagnostic code constant %q", code)
		}
	}
}

// ── 6. Context routes fit/reset through engine handle ─────────────

// TestExplorer_StrategicFit_ContextUsesEngineFit pins that Context's
// camera adapter routes fit/reset/focus through the engine handle and
// passes `getUsableGraphRect` to engine.mount.
func TestExplorer_StrategicFit_ContextUsesEngineFit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37svfContextAsset)

	// Context's engine.mount passes a getUsableGraphRect callback.
	if !regexp.MustCompile(`(?s)engine\.mount\(canvas,\s*\{[\s\S]*?getUsableGraphRect:\s*function\s*\(\)\s*\{`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: Context must pass `getUsableGraphRect: function () { ... }` to engine.mount")
	}
	// The callback reads from _hostCtx.getUsableGraphRect.
	if !strings.Contains(js, "_hostCtx.getUsableGraphRect()") {
		t.Errorf("D37s-viewport-fit-1: Context's getUsableGraphRect callback must read from _hostCtx.getUsableGraphRect()")
	}

	// Context's camera adapter routes fit/reset through the engine.
	if !strings.Contains(js, "function _buildContextEngineCameraDelegate(handle)") {
		t.Errorf("D37s-viewport-fit-1: Context's engine-camera-delegate factory must remain")
	}

	// Context does NOT compute chrome offsets directly. Negative pin
	// against ad-hoc bottom-tray / right-drawer arithmetic.
	for _, banned := range []string{
		"gmap-evidence-tray",
		"gmap-details", // right drawer id
		"governance-map-toolbar",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37s-viewport-fit-1: Context must NOT reference chrome selector %q — chrome compensation is platform-owned via getUsableGraphRect", banned)
		}
	}
}

// ── 7. Authority bespoke fit whitelisted pending migration ────────

// TestExplorer_StrategicFit_AuthorityBespokeFitWhitelistedPendingMigration
// pins that Authority remains bespoke (its own _fitToAvailableCanvas
// + _safeAreaPadding) and is whitelisted by source-documented
// commitment pending B''-Authority migration.
func TestExplorer_StrategicFit_AuthorityBespokeFitWhitelistedPendingMigration(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	poc := getExplorerAsset(t, srv, d37svfAuthorityAsset)

	// Authority retains its bespoke fit path.
	if !strings.Contains(poc, "function _fitToAvailableCanvas(cy, opts)") {
		t.Errorf("D37s-viewport-fit-1: Authority must still expose _fitToAvailableCanvas (bespoke fit, pending B''-Authority migration)")
	}
	if !strings.Contains(poc, "function _safeAreaPadding(dims)") {
		t.Errorf("D37s-viewport-fit-1: Authority must still expose _safeAreaPadding (bespoke fit input)")
	}

	// Authority does NOT consume the engine's new fit-envelope keys —
	// the engine path is pending B''-Authority.
	for _, banned := range []string{
		"graphCytoscapeEngine.mount",
		"_fitToUsableRect",
		"getUsableGraphRect: function",
	} {
		if strings.Contains(poc, banned) {
			t.Errorf("D37s-viewport-fit-1: Authority must NOT reference %q — Authority engine migration is the separate B''-Authority tranche", banned)
		}
	}

	// Authority STILL consumes GraphViewport's getSafeArea (which the
	// new aspect-based attribution algorithm improves). This is the
	// "Authority inherits platform safe-area" path until full
	// migration.
	if !strings.Contains(poc, "_rendererCtx.getSafeArea") {
		t.Errorf("D37s-viewport-fit-1: Authority must still consume _rendererCtx.getSafeArea (so the new aspect-attribution algorithm benefits Authority transparently)")
	}
}

// ── 8. Context overlay remains disabled ───────────────────────────

// TestExplorer_StrategicFit_RawContextOverlayRemainsDisabled pins
// that this tranche does NOT re-enable the Context HTML overlay.
func TestExplorer_StrategicFit_RawContextOverlayRemainsDisabled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37svfContextAsset)

	if !regexp.MustCompile(`overlayEnabled:\s*false`).MatchString(js) {
		t.Errorf("D37s-viewport-fit-1: Context must still pass `overlayEnabled: false` — overlay re-enable is gated until after the strategic fit envelope is browser-validated")
	}
}

// ── 9. Native Context fallback preserved ──────────────────────────

// TestExplorer_StrategicFit_NativeContextFallbackPreserved pins that
// native Context remains default + adoption-targeted.
func TestExplorer_StrategicFit_NativeContextFallbackPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37svfViewportAsset)

	if !strings.Contains(js, "_baselineId = 'native-context';") {
		t.Errorf("D37s-viewport-fit-1: GraphViewport must still adopt native-context as baseline")
	}
	if !strings.Contains(js, "adoptExisting('native-context')") {
		t.Errorf("D37s-viewport-fit-1: GraphViewport must still adoptExisting('native-context')")
	}
}

// ── 10. Authority files unchanged by this tranche ─────────────────

// TestExplorer_StrategicFit_AuthorityUnchanged pins that Authority's
// load-bearing markers remain intact + Authority does not reference
// the new fit-envelope tokens.
func TestExplorer_StrategicFit_AuthorityUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	poc := getExplorerAsset(t, srv, d37svfAuthorityAsset)

	for _, want := range []string{
		"_cy = window.cytoscape({",
		"viewport.register('authority',",
		"_installHtmlCardOverlay",
		"_fitToAvailableCanvas",
		"bus.registerLens('authority',",
	} {
		if !strings.Contains(poc, want) {
			t.Errorf("D37s-viewport-fit-1: Authority marker %q must remain", want)
		}
	}

	for _, banned := range []string{
		"_fitToUsableRect",
		"getUsableGraphRect: function",
		"DIAG_USABLE_RECT_EMPTY",
	} {
		if strings.Contains(poc, banned) {
			t.Errorf("D37s-viewport-fit-1: Authority must NOT reference engine-side fit-envelope token %q", banned)
		}
	}
}
