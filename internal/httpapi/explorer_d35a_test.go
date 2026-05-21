package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35a-midas-graph-viewport-foundation
//
// Structural foundation for the shared graph-viewport abstraction.
// D35a adds:
//   • `.midas-graph-viewport` — additive wrapper inside
//     `.governance-map-body` that fills the body's single-column grid
//     cell. Future sole graph clip boundary and anchor for graph
//     chrome (mode rail, camera cluster, legend, tooltip).
//   • `.midas-graph-renderer-slot` — additive wrapper inside the
//     viewport that wraps the existing graph DOM. The future single
//     home for whichever renderer is active.
//
// D35a is STRUCTURAL ONLY. No renderer-activation API, no overflow/
// clip changes, no renderer migrations. Existing renderer overflow
// rules (`.governance-map-canvas-scroll { overflow: auto }`,
// `.context-cy-spike-mount { overflow: hidden }`, etc.) are preserved
// unchanged so this tranche is visually invariant.
//
// Tests below pin:
//   1. Both new classes appear in `index.html`.
//   2. The renderer slot is inside the viewport.
//   3. The viewport contains the existing graph DOM (`#gmap-canvas`
//      → `#gmap-scene` → `#gmap-svg`) and the existing chrome
//      (`.gmap-mode-rail`, `.gmap-camera-cluster`,
//      `.gmap-legend-overlay`, `.gmap-connector-tooltip`).
//   4. The new CSS rules exist in `governance-map.css` with
//      `position: relative` on the viewport and `position: absolute;
//      inset: 0` on the slot.
//   5. The viewport has NO `overflow` rule (D35a non-goal:
//      preserving existing renderer-specific overflow behaviour).
//   6. Activation flags / IDs from prior tranches still exist
//      (`body.cytoscape-poc-active`, `body.context-cy-spike-active`,
//      `#gmap-canvas` id, `.governance-map-canvas-scroll` class).

// TestExplorer_D35aViewport_StructureExistsInIndexHtml pins both new
// classes and their containment.
func TestExplorer_D35aViewport_StructureExistsInIndexHtml(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35a: index.html missing %q", want)
		}
	}

	// Renderer slot must be a descendant of viewport.
	vIdx := strings.Index(body, `<div class="midas-graph-viewport">`)
	if vIdx < 0 {
		t.Fatal("D35a: viewport div not found")
	}
	sIdx := strings.Index(body, `<div class="midas-graph-renderer-slot">`)
	if sIdx < 0 {
		t.Fatal("D35a: renderer-slot div not found")
	}
	if sIdx < vIdx {
		t.Error("D35a: renderer-slot must appear AFTER viewport open tag (slot must be inside viewport)")
	}
	// Viewport must close AFTER renderer-slot.
	closeIdx := strings.Index(body[vIdx:], `<!-- /.midas-graph-viewport -->`)
	if closeIdx < 0 {
		t.Fatal("D35a: viewport close marker not found")
	}
	closeIdx += vIdx
	if sIdx > closeIdx {
		t.Error("D35a: renderer-slot must appear before viewport close")
	}
}

// TestExplorer_D35aViewport_HostsExistingGraphDOM pins that the
// existing graph DOM (canvas-scroll → gmap-canvas → gmap-scene →
// gmap-svg) lives inside the renderer slot, and existing chrome
// (mode-rail, camera-cluster, legend, tooltip) lives inside the
// viewport.
func TestExplorer_D35aViewport_HostsExistingGraphDOM(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	vIdx := strings.Index(body, `<div class="midas-graph-viewport">`)
	if vIdx < 0 {
		t.Fatal("D35a: viewport div not found")
	}
	vEnd := strings.Index(body[vIdx:], `<!-- /.midas-graph-viewport -->`)
	if vEnd < 0 {
		t.Fatal("D35a: viewport close marker not found")
	}
	viewport := body[vIdx : vIdx+vEnd]

	// Slot wraps the existing graph DOM.
	sIdx := strings.Index(viewport, `<div class="midas-graph-renderer-slot">`)
	if sIdx < 0 {
		t.Fatal("D35a: renderer-slot not inside viewport")
	}
	// Inside the slot, the existing scroll wrapper + canvas must
	// remain present. We don't bound the slot end precisely; we just
	// assert the next graph-DOM tokens appear AFTER the slot opener
	// and BEFORE the viewport closer.
	slotInner := viewport[sIdx:]
	for _, want := range []string{
		`<div class="governance-map-canvas-scroll">`,
		`<div id="gmap-canvas" class="governance-map-canvas"`,
		`<div id="gmap-scene" class="governance-map-scene">`,
		`<svg id="gmap-svg" class="governance-map-svg"`,
	} {
		if !strings.Contains(slotInner, want) {
			t.Errorf("D35a: existing graph DOM %q must remain inside the renderer slot", want)
		}
	}

	// Chrome must remain inside the viewport (siblings of the slot).
	for _, want := range []string{
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`class="gmap-legend-overlay"`,
		`class="gmap-connector-tooltip"`,
	} {
		if !strings.Contains(viewport, want) {
			t.Errorf("D35a: chrome element %q must remain inside the viewport", want)
		}
	}
}

// TestExplorer_D35aViewport_CSSAdditive pins the viewport + slot
// CSS rules. D35f-retire-transitional-renderer-debt PROMOTED
// `.midas-graph-viewport` to `overflow: hidden` (the strategic
// clip authority); the pre-D35f "no overflow" negative pin is
// retired here in favour of a positive pin for the new clip rule.
// `TestExplorer_D35fViewport_IsStrategicClipAuthority` carries
// the full clip-authority contract.
func TestExplorer_D35aViewport_CSSAdditive(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	vIdx := strings.Index(css, ".midas-graph-viewport {")
	if vIdx < 0 {
		t.Fatal("D35a: .midas-graph-viewport CSS rule missing")
	}
	vEnd := strings.Index(css[vIdx:], "\n  }")
	if vEnd < 0 {
		t.Fatal("D35a: cannot bound .midas-graph-viewport rule")
	}
	vBlock := css[vIdx : vIdx+vEnd]
	if !strings.Contains(vBlock, "position: relative") {
		t.Errorf("D35a: viewport must declare `position: relative`; body was:\n%s", vBlock)
	}
	// D35f: viewport is the strategic clip authority — overflow MUST
	// now be hidden. (Pre-D35f this was a negative pin asserting NO
	// overflow rule; D35f inverts it.)
	if !strings.Contains(vBlock, "overflow: hidden") {
		t.Errorf("D35f: viewport must declare `overflow: hidden` (strategic clip authority); body was:\n%s", vBlock)
	}

	sIdx := strings.Index(css, ".midas-graph-renderer-slot {")
	if sIdx < 0 {
		t.Fatal("D35a: .midas-graph-renderer-slot CSS rule missing")
	}
	sEnd := strings.Index(css[sIdx:], "\n  }")
	if sEnd < 0 {
		t.Fatal("D35a: cannot bound .midas-graph-renderer-slot rule")
	}
	sBlock := css[sIdx : sIdx+sEnd]
	for _, want := range []string{
		"position: absolute",
		"inset: 0",
	} {
		if !strings.Contains(sBlock, want) {
			t.Errorf("D35a: renderer-slot must declare %q; body was:\n%s", want, sBlock)
		}
	}
}

// TestExplorer_D35aViewport_BodyPositionRelativePreserved pins that
// the existing `.governance-map-body { position: relative }` rule
// survives. The viewport currently inherits its role as positioning
// context for absolutely-positioned chrome; if the body lost
// `position: relative`, the cascade would still work via the
// viewport, but D35a is an additive tranche — the existing rule
// must not regress.
func TestExplorer_D35aViewport_BodyPositionRelativePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	bIdx := strings.Index(css, ".governance-map-body {")
	if bIdx < 0 {
		t.Fatal("D35a: .governance-map-body rule missing")
	}
	bEnd := strings.Index(css[bIdx:], "\n  }")
	if bEnd < 0 {
		t.Fatal("D35a: cannot bound .governance-map-body rule")
	}
	bBlock := css[bIdx : bIdx+bEnd]
	if !strings.Contains(bBlock, "position: relative") {
		t.Errorf("D35a: .governance-map-body must keep `position: relative`; body was:\n%s", bBlock)
	}
}

// TestExplorer_D35aViewport_ExistingActivationFlagsPreserved pins
// that the prior-tranche activation flags / IDs / classes survive.
// D35a is structural and additive — no renderer code changes, no
// activation-API replacement.
func TestExplorer_D35aViewport_ExistingActivationFlagsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// index.html still carries the existing IDs / classes.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`id="gmap-canvas"`,
		`class="governance-map-canvas-scroll"`,
		`class="governance-map-body"`,
		`id="gmap-scene"`,
		`id="gmap-svg"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35a: structural foundation must NOT remove %q from index.html", want)
		}
	}

	// Authority Cytoscape activation flag still in its CSS.
	authPocCSS := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	if !strings.Contains(authPocCSS, "body.cytoscape-poc-active") {
		t.Error("D35a: Authority Cytoscape activation flag `body.cytoscape-poc-active` must survive")
	}

	// Context spike activation flag still in its CSS.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if !strings.Contains(spikeCSS, "body.context-cy-spike-active") {
		t.Error("D35a: Context Cytoscape spike activation flag `body.context-cy-spike-active` must survive")
	}
	// Context spike's overlay overflow:hidden survives — explicit
	// D35a non-goal per the brief.
	if !strings.Contains(spikeCSS, "overflow: hidden") {
		t.Error("D35a: Context spike's existing overflow rules must survive (D35a is visually invariant)")
	}
}

// TestExplorer_D35aViewport_ChromeNotInsideRendererSlot pins that
// the chrome elements (mode rail, camera cluster, legend, tooltip)
// are siblings of the renderer slot inside the viewport — NOT
// children of the slot. If they slipped inside the slot, future
// renderer activation (D35b+) that swaps the slot's contents would
// inadvertently remove the chrome too.
func TestExplorer_D35aViewport_ChromeNotInsideRendererSlot(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Locate the renderer slot. The renderer slot's contents end
	// at the closing `</div>` that matches its opening. We bound
	// it heuristically using the next chrome element opener as
	// the upper bound — if any chrome element appears INSIDE that
	// region, the chrome has been incorrectly nested.
	sIdx := strings.Index(body, `<div class="midas-graph-renderer-slot">`)
	if sIdx < 0 {
		t.Fatal("D35a: renderer-slot not found")
	}
	// The renderer slot in D35a wraps `.governance-map-canvas-scroll`
	// only. Its closing `</div>` lives between the canvas-scroll
	// close and the first chrome element. We assert chrome elements
	// appear AFTER the canvas-scroll structure closes.
	chromeIdx := strings.Index(body[sIdx:], `class="gmap-mode-rail"`)
	if chromeIdx < 0 {
		t.Fatal("D35a: gmap-mode-rail not found after renderer slot")
	}
	chromeIdx += sIdx
	// Between sIdx and chromeIdx, the renderer-slot's content
	// (canvas-scroll → gmap-canvas → gmap-scene → gmap-svg) must
	// exist. Pin a couple of structural tokens to make sure the
	// chrome did NOT slip inside.
	region := body[sIdx:chromeIdx]
	for _, want := range []string{
		`class="governance-map-canvas-scroll"`,
		`id="gmap-canvas"`,
	} {
		if !strings.Contains(region, want) {
			t.Errorf("D35a: renderer-slot region must contain %q before chrome", want)
		}
	}
	for _, banned := range []string{
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`class="gmap-legend-overlay"`,
		`class="gmap-connector-tooltip"`,
	} {
		if strings.Contains(region, banned) {
			t.Errorf("D35a: chrome element %q must NOT be inside the renderer slot (it should be a sibling, anchored to the viewport)", banned)
		}
	}
}
