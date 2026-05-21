package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37s-context-geometry-diagnostic — Surgical overlay-disable change.
//
// This file pins the minimal contract for the overlay-disable switch
// used to inspect the raw Cytoscape graph in strategic spatial Context.
//
// Scope (verbatim from the build prompt):
//   • The HTML overlay must be hidden/disabled for strategic spatial
//     Context.
//   • Authority must not be affected.
//   • Native Context must not be affected.
//   • graphStage / graphCytoscapeEngine API must remain (the only
//     engine change is an optional `overlayEnabled` flag, default
//     true, back-compat).
//   • selection bridge / camera bus contracts must remain.
//
// Tests here verify the source-contract surface only. Browser
// validation (raw nodes visible, no overlay cards) is the user-side
// runtime gate.

const (
	d37sodEngineAsset   = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37sodContextAsset  = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37sodAuthAsset     = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37sodViewportAsset = "/explorer/assets/js/graph/graph-viewport.js"
	d37sodStageAsset    = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37sodGraphSel      = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37sodCameraBus     = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
)

// ── 1. Engine exposes overlayEnabled option (default true) ────────

// TestExplorer_D37sOverlayDisable_EngineSupportsOverlayEnabledFlag pins
// that the engine's mount path supports an optional `overlayEnabled`
// boolean. Default is `true` (back-compat with every existing consumer);
// when explicitly `false`, the engine skips the shared overlay mount.
func TestExplorer_D37sOverlayDisable_EngineSupportsOverlayEnabledFlag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sodEngineAsset)

	// The engine captures the flag from opts with the default-true
	// shape `(opts.overlayEnabled !== false)`.
	if !regexp.MustCompile(`var overlayEnabled = \(opts\.overlayEnabled !== false\);`).MatchString(js) {
		t.Errorf("D37s-overlay-disable: engine must capture `var overlayEnabled = (opts.overlayEnabled !== false);` so the default is true and only an explicit `false` opts out")
	}

	// The overlay-mount gate consults the flag.
	if !regexp.MustCompile(`if \(overlayEnabled && g && g\.graphCytoscapeOverlay && _isFn\(g\.graphCytoscapeOverlay\.mount\)\)`).MatchString(js) {
		t.Errorf("D37s-overlay-disable: engine's overlay-mount gate must check `overlayEnabled` before calling graphCytoscapeOverlay.mount(...)")
	}

	// Public mount-options docblock documents the new option.
	if !strings.Contains(js, "overlayEnabled    — optional boolean, default `true`.") {
		t.Errorf("D37s-overlay-disable: engine mount-options docblock must document the `overlayEnabled` option")
	}
}

// ── 2. Context passes overlayEnabled: false ───────────────────────

// TestExplorer_D37sOverlayDisable_ContextOptsOut pins that strategic
// spatial Context's `engine.mount(...)` call explicitly opts out of
// the overlay.
func TestExplorer_D37sOverlayDisable_ContextOptsOut(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sodContextAsset)

	// The engine.mount(...) call includes `overlayEnabled: false`.
	if !regexp.MustCompile(`(?s)engine\.mount\(canvas,\s*\{[\s\S]*?overlayEnabled:\s*false`).MatchString(js) {
		t.Errorf("D37s-overlay-disable: Context's engine.mount(...) call must include `overlayEnabled: false` so the engine skips the HTML overlay mount")
	}

	// Context appends a raw-node visibility override to its existing
	// edge-style override so the cy nodes are visible without the
	// overlay (the engine's base style is transparent, expecting the
	// overlay to paint the visible card).
	if !strings.Contains(js, "function _buildContextRawNodeVisibilityOverride()") {
		t.Errorf("D37s-overlay-disable: Context must declare `_buildContextRawNodeVisibilityOverride()` so raw cy nodes are visible when the overlay is disabled")
	}
	if !regexp.MustCompile(`nodeStyleOverride:\s*_buildContextEdgeStyleOverride\(\)\.concat\(_buildContextRawNodeVisibilityOverride\(\)\)`).MatchString(js) {
		t.Errorf("D37s-overlay-disable: nodeStyleOverride must be `_buildContextEdgeStyleOverride().concat(_buildContextRawNodeVisibilityOverride())` so the visible-node override is appended to the existing edge styles")
	}

	// The visible-node override targets the cy `node` selector with a
	// non-transparent fill + a label so the graph is inspectable.
	rIdx := strings.Index(js, "function _buildContextRawNodeVisibilityOverride()")
	if rIdx < 0 {
		t.Fatal("D37s-overlay-disable: _buildContextRawNodeVisibilityOverride must exist")
	}
	rTail := js[rIdx:]
	rEndRel := strings.Index(rTail[1:], "\n  function ")
	if rEndRel < 0 {
		t.Fatalf("D37s-overlay-disable: _buildContextRawNodeVisibilityOverride body must be well-formed")
	}
	rBody := rTail[:rEndRel+1]
	if !strings.Contains(rBody, "selector: 'node',") {
		t.Errorf("D37s-overlay-disable: visible-node override must target the `node` selector")
	}
	if !strings.Contains(rBody, "'label':              'data(id)',") {
		t.Errorf("D37s-overlay-disable: visible-node override must set a label so each cy node is identifiable")
	}
	if regexp.MustCompile(`'background-opacity':\s*0[,\s\}]`).MatchString(rBody) {
		t.Errorf("D37s-overlay-disable: visible-node override must NOT keep background-opacity at 0 — that would defeat the purpose of disabling the overlay")
	}
}

// ── 3. Authority does NOT pass overlayEnabled: false ──────────────

// TestExplorer_D37sOverlayDisable_AuthorityUnaffected pins that
// Authority is structurally untouched by this change. Authority does
// not consume the shared engine; it MUST NOT reference the new flag.
func TestExplorer_D37sOverlayDisable_AuthorityUnaffected(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37sodAuthAsset)

	// Authority must not reference overlayEnabled (Authority bypasses
	// the engine; the flag has no meaning there).
	if strings.Contains(js, "overlayEnabled") {
		t.Errorf("D37s-overlay-disable: Authority must NOT reference `overlayEnabled` — Authority does not consume the engine; the flag is Context-specific")
	}

	// Authority's own overlay path is intact.
	if !strings.Contains(js, "_installHtmlCardOverlay") {
		t.Errorf("D37s-overlay-disable: Authority must still install its own HTML overlay (_installHtmlCardOverlay)")
	}
	// Authority still instantiates cy directly + registers with the bus + viewport.
	for _, want := range []string{
		"_cy = window.cytoscape({",
		"viewport.register('authority',",
		"bus.registerLens('authority',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37s-overlay-disable: Authority's load-bearing marker %q must remain", want)
		}
	}
}

// ── 4. Native Context unchanged ───────────────────────────────────

// TestExplorer_D37sOverlayDisable_NativeContextUnchanged pins that
// native Context is still the default and its renderer registration
// is unchanged.
func TestExplorer_D37sOverlayDisable_NativeContextUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vp := getExplorerAsset(t, srv, d37sodViewportAsset)

	if !strings.Contains(vp, "_baselineId = 'native-context';") {
		t.Errorf("D37s-overlay-disable: GraphViewport must still adopt native-context as the baseline renderer")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37s-overlay-disable: GraphViewport must still adoptExisting('native-context')")
	}
}

// ── 5. graphStage and contract surfaces unchanged ─────────────────

// TestExplorer_D37sOverlayDisable_PlatformContractsUnchanged pins
// that this surgical change touches NEITHER graphStage NOR the
// selection-bridge NOR the camera-bus public surface.
func TestExplorer_D37sOverlayDisable_PlatformContractsUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// graphStage source must NOT reference overlayEnabled (no stage
	// changes in this build).
	stageJS := getExplorerAsset(t, srv, d37sodStageAsset)
	if strings.Contains(stageJS, "overlayEnabled") {
		t.Errorf("D37s-overlay-disable: graphStage must NOT reference `overlayEnabled` — this change is scoped to the engine + Context lens; graphStage is untouched")
	}

	// graphStage retains its public surface.
	for _, want := range []string{
		"compose:                 compose,",
		"anchorOf:                anchorOf,",
		"fitBoundsOf:             fitBoundsOf,",
		"normaliseCardFootprints: normaliseCardFootprints,",
		"validateNoOverlap:       validateNoOverlap,",
	} {
		if !strings.Contains(stageJS, want) {
			t.Errorf("D37s-overlay-disable: graphStage public surface marker %q must remain", want)
		}
	}

	// Selection bridge module still served.
	selJS := getExplorerAsset(t, srv, d37sodGraphSel)
	if len(selJS) == 0 {
		t.Errorf("D37s-overlay-disable: graph-selection-bridge module must remain served")
	}

	// Camera bus module still served.
	busJS := getExplorerAsset(t, srv, d37sodCameraBus)
	if len(busJS) == 0 {
		t.Errorf("D37s-overlay-disable: graph-camera-bus module must remain served")
	}

	// Context still wires through the selection bridge.
	contextJS := getExplorerAsset(t, srv, d37sodContextAsset)
	if !strings.Contains(contextJS, "bridge.selectCard(card)") {
		t.Errorf("D37s-overlay-disable: Context must still call bridge.selectCard(card) (selection bridge contract unchanged)")
	}
	if !strings.Contains(contextJS, "function _buildContextEngineCameraDelegate(handle)") {
		t.Errorf("D37s-overlay-disable: Context's engine-camera-delegate factory must remain (camera bus contract unchanged)")
	}
}
