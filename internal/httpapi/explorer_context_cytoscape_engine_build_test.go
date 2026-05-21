package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37r-context-cytoscape-2-impl — Context Cytoscape Engine Foundation
// (tranche A of the strategic Context Cytoscape convergence roadmap).
//
// Source-contract tests for the strategic spatial Context renderer's
// Cytoscape engine integration. Pins:
//
//   1. The production strategic Context renderer instantiates real
//      Cytoscape inside its spatial paint path.
//   2. The production Cytoscape instantiation is distinct from the
//      dormant overlay-spike module.
//   3. Context cards are mapped into Cytoscape node elements.
//   4. Context connectors are mapped into Cytoscape edge elements.
//   5. Cytoscape's preset layout consumes positions from the existing
//      graphStage / stage model.
//   6. The Cytoscape style array covers all five connector visual
//      classes and dash semantics.
//   7. The camera-bus delegate wraps Cytoscape pan/zoom/fit when the
//      Cytoscape engine is live.
//   8. Cytoscape `tap` on nodes publishes through the existing
//      contextSelectionBridge.selectCard contract.
//   9. The Context selected-object pane provider remains registered.
//  10. The Context evidence/drift tray public API remains present.
//  11. The Context drawer registration remains present.
//  12. Native Context default adoption is unchanged.
//  13. Non-spatial strategic fallback remains safe.
//  14. Authority Cytoscape renderer and canvas-edge modules are
//      untouched by this build.
//  15. Spatial strategic Context no longer uses
//      context-connector-painter.js as the primary edge renderer.
//
// These tests are source-contract assertions on the served asset
// bodies. They do not exercise runtime behaviour; that belongs to
// browser validation per the build prompt.

const (
	d37rContextRendererAsset       = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37rContextSpikeAsset          = "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js"
	d37rContextConnectorPainter    = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37rContextPaneAsset           = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37rContextEvidenceTrayAsset   = "/explorer/assets/js/graph/context/context-evidence-tray.js"
	d37rGraphDrawerAsset           = "/explorer/assets/js/graph/graph-drawer.js"
	d37rGraphViewportAsset         = "/explorer/assets/js/graph/graph-viewport.js"
	d37rAuthorityPocAsset          = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37rAuthorityCanvasEdgeAsset   = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37rIndexHTMLAsset             = "/explorer/index.html"
	d37rContextSelectionBridgeJS   = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37rGraphSelectionBridgeJS     = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
)

// ── 1. Strategic spatial Context instantiates real Cytoscape ──────

// TestExplorer_ContextStrategicSpatialRendererInstantiatesCytoscape pins
// that the strategic Context renderer's source carries a real
// `window.cytoscape({...})` instantiation site inside the spatial
// paint path — proving the engine convergence is structurally
// present, not just named.
// D37r-tranche-B'' flip: the Cytoscape instantiation moved from
// the lens into the shared engine module. Context no longer holds
// a `_cy = window.cytoscape({` site; instead it calls
// `engine.mount(canvas, {...})` and the engine module owns the
// constructor call. The availability-detection helper
// (`_cytoscapeAvailable`) and the rAF retry mechanism are deleted
// — the vendor script tag now precedes the engine module so
// `window.cytoscape` is synchronously available. This test is
// flipped to assert the engine-consumer location on both sides:
// the engine module still instantiates cytoscape, and the Context
// renderer reaches the engine via the canonical namespace.
func TestExplorer_ContextStrategicSpatialRendererInstantiatesCytoscape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	// Context now consumes the engine — no direct cy instantiation.
	if !strings.Contains(js, "engine.mount(canvas, {") {
		t.Errorf("D37r-tranche-B'' (flipped): Context renderer must call engine.mount(canvas, {...})")
	}
	if !strings.Contains(js, "g.graphCytoscapeEngine") {
		t.Errorf("D37r-tranche-B'' (flipped): Context renderer must reach the engine via MIDASExplorerGraph.graphCytoscapeEngine")
	}
	// Spatial paint entry point remains as the routing function.
	if !strings.Contains(js, "function _renderSpatialCytoscape(") {
		t.Errorf("D37r-tranche-B'' (flipped): _renderSpatialCytoscape entry point must remain")
	}
	if !strings.Contains(js, "_renderSpatialCytoscape(layout, cards, connectors, stage, byId, painter);") {
		t.Errorf("D37r-tranche-B'' (flipped): spatial-foundation must call _renderSpatialCytoscape when the engine module is present")
	}

	// The engine module itself owns the cy constructor call.
	engineJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	if !strings.Contains(engineJS, "cy = window.cytoscape({") {
		t.Errorf("D37r-tranche-B'' (flipped): engine module must own the cy = window.cytoscape({ instantiation")
	}
}

// ── 2. Production Cytoscape path is NOT the dormant spike ─────────

// TestExplorer_ContextCytoscapeInstantiationIsNotDormantSpike pins
// that the production Cytoscape integration does not depend on the
// dormant overlay-spike module (`context-cytoscape-overlay-spike.js`)
// — its IIFE is gated by `?cytoscape=1&contextHtmlCards=1` and is
// explicitly out of scope for strategic Context activation.
func TestExplorer_ContextCytoscapeInstantiationIsNotDormantSpike(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	renderer := getExplorerAsset(t, srv, d37rContextRendererAsset)
	// D37r-tranche-B'' flip: the constructor call moved to the
	// engine module. The Context-renderer-side prerequisite is the
	// engine consumption call.
	if !strings.Contains(renderer, "engine.mount(canvas, {") {
		t.Fatal("D37r-tranche-B'' (flipped): Context renderer must consume the engine before this assertion is meaningful")
	}

	// The strategic Context renderer must not reference the
	// overlay-spike module's public surface or DOM markers.
	for _, banned := range []string{
		"contextCytoscapeOverlaySpike", // hypothetical public surface
		"context-cy-spike-active",
		"context-cy-spike-mount",
		"context-cy-spike-overlay",
		"contextHtmlCards=1",
	} {
		if strings.Contains(renderer, banned) {
			t.Errorf("D37r: strategic Context renderer must not reference the dormant overlay-spike (found %q)", banned)
		}
	}

	// The dormant spike file is still served (untouched by this
	// build) but gated by its own activation flags.
	spike := getExplorerAsset(t, srv, d37rContextSpikeAsset)
	if !strings.Contains(spike, "BODY_FLAG_CLASS") || !strings.Contains(spike, "context-cy-spike-active") {
		t.Errorf("D37r: overlay-spike module must remain present and gated by its own body-class")
	}
	if !regexp.MustCompile(`cytoscape\s*=\s*1`).MatchString(spike) {
		t.Errorf("D37r: overlay-spike module must remain self-gated on its existing query-param contract")
	}
}

// ── 3. Cards map to Cytoscape nodes ───────────────────────────────

// D37r-tranche-B'' flip note: this test still verifies the per-card
// builder produces the cy-element-shape interim form (which the
// engine then translates to canonical via _toCyElements). The
// builder's output shape is preserved deliberately so future
// migration to direct-canonical-shape can land as a separate
// tranche. The pins below are unchanged.
func TestExplorer_ContextCardsMapToCytoscapeNodes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	// Node-builder helper exists.
	if !strings.Contains(js, "function _buildContextCytoscapeNodes(cards, stage)") {
		t.Errorf("D37r: strategic Context renderer must declare _buildContextCytoscapeNodes(cards, stage)")
	}

	// Node group + data shape.
	for _, want := range []string{
		"group: 'nodes',",
		"id:       String(c.id),",
		"kind:     String(c.kind || ''),",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r: Cytoscape node mapper must emit %q", want)
		}
	}

	// Node classes encode kind / emphasis so the style array can
	// select them.
	if !strings.Contains(js, "function _contextCytoscapeNodeClasses(card)") {
		t.Errorf("D37r: node-class helper _contextCytoscapeNodeClasses must be present")
	}
	if !strings.Contains(js, "'context-node-kind-'") {
		t.Errorf("D37r: node mapper must derive a `context-node-kind-<kind>` class")
	}
	if !strings.Contains(js, "'context-node-emphasis-'") {
		t.Errorf("D37r: node mapper must derive a `context-node-emphasis-<emphasis>` class")
	}

	// D37r-tranche-B'' flip: the cy instantiation moved into the
	// engine module. The lens-side wiring is the engine.mount() call
	// whose `data` field is built from _buildContextEngineData, which
	// itself consumes _buildContextCytoscapeNodes(cards, stage). This
	// indirection preserves the existing builder shape while routing
	// it through the canonical engine data contract.
	if !regexp.MustCompile(`(?s)engine\.mount\(canvas,\s*\{.*?data:\s*_buildContextEngineData\(cards,\s*connectors,\s*stage\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B'' (flipped): engine.mount(canvas, {...}).data must be _buildContextEngineData(cards, connectors, stage) — the canonical translator that wraps _buildContextCytoscapeNodes(cards, stage)")
	}
	if !regexp.MustCompile(`(?s)function _buildContextEngineData\(cards, connectors, stage\)[^{]*\{[^}]*_buildContextCytoscapeNodes\(cards,\s*stage\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B'' (flipped): _buildContextEngineData body must call _buildContextCytoscapeNodes(cards, stage) to produce the per-node interim shape that the canonical translator wraps")
	}
}

// ── 4. Connectors map to Cytoscape edges ──────────────────────────

func TestExplorer_ContextConnectorsMapToCytoscapeEdges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	if !strings.Contains(js, "function _buildContextCytoscapeEdges(connectors, stage)") {
		t.Errorf("D37r: strategic Context renderer must declare _buildContextCytoscapeEdges(connectors, stage)")
	}

	for _, want := range []string{
		"group: 'edges',",
		"source:      srcId,",
		"target:      dstId,",
		"edgeKind:    String(c.edgeKind || ''),",
		"visualClass: visualClass,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r: Cytoscape edge mapper must emit %q", want)
		}
	}

	// Dash semantic is preserved (solid vs `{on,off}` object).
	if !strings.Contains(js, "var dashSemantic = 'solid';") {
		t.Errorf("D37r: edge mapper must default dashSemantic to 'solid'")
	}
	if !strings.Contains(js, "dashSemantic = 'dashed';") {
		t.Errorf("D37r: edge mapper must promote to 'dashed' when c.dashPattern is an object")
	}

	// D37r-tranche-B'' flip: the cy instantiation moved into the
	// engine module. The edge builder is consumed via the canonical
	// _buildContextEngineData translator that the engine receives as
	// its `data` field.
	if !regexp.MustCompile(`(?s)function _buildContextEngineData\(cards, connectors, stage\)[^{]*\{[\s\S]*?_buildContextCytoscapeEdges\(connectors,\s*stage\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B'' (flipped): _buildContextEngineData body must call _buildContextCytoscapeEdges(connectors, stage) — the engine receives this canonical data via engine.mount(canvas, {data: …})")
	}
}

// ── 5. Cytoscape layout uses preset positions from graphStage ─────

func TestExplorer_ContextCytoscapeUsesPresetStagePositions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	// Spatial-foundation still composes through graphStage.
	if !strings.Contains(js, "graphStage.compose(layout, footprints, safeArea, {})") {
		t.Errorf("D37r: strategic Context renderer must continue to compose preset positions via graphStage.compose")
	}

	// D37r-tranche-B'' flip: the cy instantiation moved into the
	// engine module. The 'preset' layout (no engine-side layout
	// algorithm — positions come from the stage model) is now a
	// fixed contract of the engine, not a per-lens parameter.
	engineJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	if !regexp.MustCompile(`(?s)cy\s*=\s*window\.cytoscape\(\{[\s\S]*?layout:\s*\{\s*name:\s*'preset'`).MatchString(engineJS) {
		t.Errorf("D37r-tranche-B'' (flipped): engine module's cy instantiation must set layout: { name: 'preset', ... } so stage-driven positions remain authoritative")
	}

	// Node mapper consumes stage.cards[id] for positions.
	if !regexp.MustCompile(`var entry = stage\.cards\[c\.id\];`).MatchString(js) {
		t.Errorf("D37r: node mapper must derive positions from stage.cards[c.id]")
	}
	if !regexp.MustCompile(`position:\s*\{`).MatchString(js) {
		t.Errorf("D37r: node element must carry a `position: { x, y }` block")
	}
}

// ── 6. Style covers visual classes + dash semantics ──────────────

func TestExplorer_ContextCytoscapeStylesCoverConnectorClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	if !strings.Contains(js, "function _buildContextCytoscapeStyle()") {
		t.Errorf("D37r: style builder _buildContextCytoscapeStyle must be present")
	}

	// All five visual class selectors.
	for _, klass := range []string{
		"edge.context-edge-visual-service",
		"edge.context-edge-visual-ai_binding",
		"edge.context-edge-visual-authority",
		"edge.context-edge-visual-evidence",
		"edge.context-edge-visual-gap",
	} {
		if !strings.Contains(js, klass) {
			t.Errorf("D37r: Cytoscape style must cover visual class selector %q", klass)
		}
	}

	// Dash semantics (solid + dashed).
	if !strings.Contains(js, "edge.context-edge-dash-dashed") {
		t.Errorf("D37r: Cytoscape style must cover the dashed-edge selector")
	}
	if !strings.Contains(js, "edge.context-edge-dash-solid") {
		t.Errorf("D37r: Cytoscape style must cover the solid-edge selector")
	}

	// Base node + selected.
	if !strings.Contains(js, "selector: 'node',") {
		t.Errorf("D37r: Cytoscape style must include a base node selector")
	}
	if !strings.Contains(js, "selector: 'node:selected',") {
		t.Errorf("D37r: Cytoscape style must include a selected-node selector")
	}

	// D37r-tranche-B'' flip: the cy instantiation moved into the
	// engine module. Context now contributes its visual-class style
	// entries via the engine's `nodeStyleOverride` option, populated
	// by `_buildContextEdgeStyleOverride()`. The engine concats this
	// override onto its base style array before passing it to the
	// cy constructor. The legacy `_buildContextCytoscapeStyle()` is
	// preserved as dead code (existing assertions above still pin
	// its presence) to keep prior structural pins stable.
	if !regexp.MustCompile(`(?s)engine\.mount\(canvas,\s*\{.*?nodeStyleOverride:\s*_buildContextEdgeStyleOverride\(\)`).MatchString(js) {
		t.Errorf("D37r-tranche-B'' (flipped): engine.mount(canvas, {...}).nodeStyleOverride must call _buildContextEdgeStyleOverride() — the lens-side contribution that the engine concats onto its base style array")
	}
	// And the override builder must cover the five visual classes
	// (parity with the dead _buildContextCytoscapeStyle body so the
	// override is a complete drop-in, not a partial subset).
	overrideIdx := strings.Index(js, "function _buildContextEdgeStyleOverride()")
	if overrideIdx < 0 {
		t.Fatal("D37r-tranche-B'' (flipped): _buildContextEdgeStyleOverride() must be present in source")
	}
	overrideTail := js[overrideIdx:]
	overrideEndRel := strings.Index(overrideTail[1:], "\n  function ")
	if overrideEndRel < 0 {
		t.Fatalf("D37r-tranche-B'' (flipped): _buildContextEdgeStyleOverride() body must be well-formed")
	}
	overrideBody := overrideTail[:overrideEndRel+1]
	for _, klass := range []string{
		"edge.context-edge-visual-service",
		"edge.context-edge-visual-ai_binding",
		"edge.context-edge-visual-authority",
		"edge.context-edge-visual-evidence",
		"edge.context-edge-visual-gap",
	} {
		if !strings.Contains(overrideBody, klass) {
			t.Errorf("D37r-tranche-B'' (flipped): _buildContextEdgeStyleOverride body must cover visual class selector %q (lens-side contribution to engine style)", klass)
		}
	}
}

// ── 7. Camera-bus delegate wraps Cytoscape ────────────────────────

func TestExplorer_ContextCytoscapeCameraDelegateUsesCyViewport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	if !strings.Contains(js, "function _buildContextCytoscapeCameraDelegate(cy)") {
		t.Errorf("D37r: Cytoscape camera-bus delegate builder must be present")
	}

	// Slice the cy delegate body.
	idx := strings.Index(js, "function _buildContextCytoscapeCameraDelegate(cy)")
	if idx < 0 {
		t.Fatal("D37r: cy-camera-delegate builder must be present")
	}
	tail := js[idx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37r: cy-camera-delegate builder body must be well-formed")
	}
	body := tail[:endRel+1]

	// Locked command vocabulary.
	for _, cmd := range []string{
		"zoomIn:",
		"zoomOut:",
		"fit:",
		"reset:",
		"setZoom:",
		"getZoom:",
		"focusRoot:",
		"focusSelected:",
	} {
		if !strings.Contains(body, cmd) {
			t.Errorf("D37r: Cytoscape camera delegate must expose %q", cmd)
		}
	}

	// The delegate calls Cytoscape's pan/zoom/fit APIs.
	for _, api := range []string{
		"cy.zoom(",
		"cy.fit(",
		"cy.center(",
		"cy.getElementById(",
		"cy.width()",
		"cy.height()",
	} {
		if !strings.Contains(body, api) {
			t.Errorf("D37r: Cytoscape camera delegate body must call %q", api)
		}
	}

	// getZoom returns the Cytoscape ratio directly (cy.zoom()) —
	// matching the bus's canonical getZoom() → ratio contract.
	// Use a multiline-tolerant `(?s)` pattern + an explicit cap on the
	// distance between the getZoom signature and the cy.zoom() call so
	// the regex traverses the inner try-block braces without locking
	// onto a distant unrelated cy.zoom() callsite.
	if !regexp.MustCompile(`(?s)getZoom:\s*function\s*\(\).{0,200}var\s+z\s*=\s*cy\.zoom\(\)`).MatchString(body) {
		t.Errorf("D37r: Cytoscape camera delegate getZoom must read cy.zoom() as the canonical ratio")
	}

	// Delegate dispatch prefers Cytoscape when _cy is set.
	if !strings.Contains(js, "delegate = _buildContextCytoscapeCameraDelegate(_cy);") {
		t.Errorf("D37r: _registerCameraBusDelegate must select the Cytoscape delegate when _cy is live")
	}
}

// ── 8. Cytoscape tap publishes through selection bridge ──────────

func TestExplorer_ContextCytoscapeSelectionPublishesThroughBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	if !strings.Contains(js, "function _wireCytoscapeSelectionTap()") {
		t.Errorf("D37r: Cytoscape tap-selection helper _wireCytoscapeSelectionTap must be present")
	}

	// Locate the helper body and assert the locked event wiring.
	idx := strings.Index(js, "function _wireCytoscapeSelectionTap()")
	if idx < 0 {
		t.Fatal("D37r: _wireCytoscapeSelectionTap must be present")
	}
	tail := js[idx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37r: _wireCytoscapeSelectionTap body must be well-formed")
	}
	body := tail[:endRel+1]

	if !strings.Contains(body, "_cy.on('tap', 'node', function (evt)") {
		t.Errorf("D37r: Cytoscape tap-selection wiring must use _cy.on('tap', 'node', ...)")
	}
	if !strings.Contains(body, "bridge.selectCard(card)") {
		t.Errorf("D37r: Cytoscape tap-selection must call contextSelectionBridge.selectCard(card)")
	}
	if !strings.Contains(body, "window.MIDASExplorerGraph.contextSelectionBridge") {
		t.Errorf("D37r: Cytoscape tap-selection must resolve contextSelectionBridge from the shared namespace")
	}

	// The cross-lens graphSelectionBridge contract is unchanged.
	gb := getExplorerAsset(t, srv, d37rGraphSelectionBridgeJS)
	for _, want := range []string{
		"registerLens",
		"selectCard",
	} {
		if !strings.Contains(gb, want) {
			t.Errorf("D37r: graphSelectionBridge public contract %q must remain present", want)
		}
	}
}

// ── 9. Context selected-object pane preserved ─────────────────────

func TestExplorer_ContextSelectedObjectPanePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	pane := getExplorerAsset(t, srv, d37rContextPaneAsset)

	for _, want := range []string{
		"contextSelectedObjectPane",
		"registerLensProvider",
	} {
		if !strings.Contains(pane, want) {
			t.Errorf("D37r: Context selected-object pane module must keep %q", want)
		}
	}
}

// ── 10. Context evidence/drift tray preserved ─────────────────────

func TestExplorer_ContextEvidenceTrayPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tray := getExplorerAsset(t, srv, d37rContextEvidenceTrayAsset)

	// The tray exposes its public surface under the canonical
	// MIDASExplorerGraph namespace. The exact public name is the
	// existing convention (e.g. contextEvidenceTray); pin the
	// presence of the namespace assignment without over-constraining
	// the surface shape.
	if !regexp.MustCompile(`MIDASExplorerGraph\.contextEvidenceTray\s*=`).MatchString(tray) {
		t.Errorf("D37r: Context evidence/drift tray must keep its public surface attached to MIDASExplorerGraph.contextEvidenceTray")
	}
}

// ── 11. Context drawer integration preserved ──────────────────────

func TestExplorer_ContextDrawerPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	drawer := getExplorerAsset(t, srv, d37rGraphDrawerAsset)

	// Drawer module's public surface is `window.MIDASExplorerGraph.drawer`
	// (per the file header at graph-drawer.js:38). Pin the public
	// assignment + the three-slot vocabulary the Context lens uses.
	if !regexp.MustCompile(`MIDASExplorerGraph\.drawer\s*=`).MatchString(drawer) {
		t.Errorf("D37r: graph-drawer module must keep its public surface attached to MIDASExplorerGraph.drawer")
	}
	for _, want := range []string{
		"inspector",
		"evidence",
		"config",
	} {
		if !strings.Contains(drawer, want) {
			t.Errorf("D37r: drawer module must keep the %q slot vocabulary", want)
		}
	}
}

// ── 12. Native Context default preserved ──────────────────────────

func TestExplorer_NativeContextFallbackPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vp := getExplorerAsset(t, srv, d37rGraphViewportAsset)

	if !strings.Contains(vp, "_baselineId = 'native-context';") {
		t.Errorf("D37r: GraphViewport host must still adopt native-context as the baseline renderer")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37r: GraphViewport host must still adopt native-context via adoptExisting")
	}
}

// ── 13. Non-spatial strategic fallback preserved ──────────────────

func TestExplorer_NonSpatialStrategicFallbackPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	// The non-spatial path remains: when ?contextLayout=spatial is
	// absent the renderer renders the document-flow / banded
	// foundation, not the Cytoscape spatial path.
	if !strings.Contains(js, "if (_isSpatialMode() && _hasGraphStage()) {") {
		t.Errorf("D37r: spatial-mode gate must remain — non-spatial strategic mode must not fall into _renderSpatialFoundation")
	}
	if !strings.Contains(js, "function _buildFallbackContextCameraDelegate()") {
		t.Errorf("D37r: non-spatial strategic Context fallback camera-bus delegate must remain present")
	}
}

// ── 14. Authority untouched ───────────────────────────────────────

func TestExplorer_AuthorityUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	poc := getExplorerAsset(t, srv, d37rAuthorityPocAsset)
	edge := getExplorerAsset(t, srv, d37rAuthorityCanvasEdgeAsset)

	// Authority Cytoscape PoC retains its load-bearing markers.
	for _, want := range []string{
		"_cy = window.cytoscape({",
		"viewport.register('authority',",
		"_registerAuthorityCameraBusDelegate",
		"bus.registerLens('authority',",
	} {
		if !strings.Contains(poc, want) {
			t.Errorf("D37r: Authority Cytoscape PoC must remain unchanged (%q)", want)
		}
	}

	// Authority canvas-edge tabs module retains its provider hook.
	if !strings.Contains(edge, "registerLensProvider") {
		t.Errorf("D37r: Authority canvas-edge tabs module must remain unchanged (registerLensProvider)")
	}
}

// ── 15. Strategic spatial no longer uses SVG painter as primary ──

// TestExplorer_StrategicSpatialNoLongerUsesSvgConnectorPainterAsPrimaryEngine
// pins that the Cytoscape branch of `_renderSpatialFoundation` does
// not depend on `context-connector-painter.js` to draw edges. The
// painter file remains for the load-order-safety fallback below the
// branch, but the strategic spatial path's primary edge renderer is
// now Cytoscape.
func TestExplorer_StrategicSpatialNoLongerUsesSvgConnectorPainterAsPrimaryEngine(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	// Locate the Cytoscape entry function body.
	idx := strings.Index(js, "function _renderSpatialCytoscape(layout, cards, connectors, stage, byId, painter)")
	if idx < 0 {
		t.Fatal("D37r: _renderSpatialCytoscape must be present before this assertion is meaningful")
	}
	tail := js[idx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37r: _renderSpatialCytoscape body must be well-formed")
	}
	body := tail[:endRel+1]

	// The Cytoscape branch must NOT delegate edge painting to the
	// SVG painter (which is what _paintConnectorsForCanvas calls).
	if strings.Contains(body, "_paintConnectorsForCanvas(") {
		t.Errorf("D37r: strategic Cytoscape spatial path must not call _paintConnectorsForCanvas (painter is fallback-only)")
	}
	if strings.Contains(body, "contextConnectorPainter") {
		t.Errorf("D37r: strategic Cytoscape spatial path must not reference contextConnectorPainter (painter is fallback-only)")
	}
	if strings.Contains(body, "createElementNS(SVG_NS, 'line')") {
		t.Errorf("D37r: strategic Cytoscape spatial path must not draw SVG <line> connectors itself")
	}

	// D37r-tranche-B'' flip: the Cytoscape edge mapper is now the
	// primary edge source via the engine's canonical data contract.
	// The lens-side wiring inside _renderSpatialCytoscape is the
	// engine.mount(canvas, {…, data: _buildContextEngineData(cards,
	// connectors, stage), …}) call. _buildContextEngineData itself
	// consumes _buildContextCytoscapeEdges; pin the indirection here.
	if !strings.Contains(body, "_buildContextEngineData(cards, connectors, stage)") {
		t.Errorf("D37r-tranche-B'' (flipped): strategic Cytoscape spatial path must consume _buildContextEngineData(cards, connectors, stage) as the engine's canonical data — that translator wraps _buildContextCytoscapeEdges as the primary edge source")
	}
	if !strings.Contains(js, "_buildContextCytoscapeEdges(connectors, stage)") {
		t.Errorf("D37r-tranche-B'' (flipped): _buildContextCytoscapeEdges must remain present (called by _buildContextEngineData)")
	}

	// The painter module file remains present (fallback / legacy).
	painter := getExplorerAsset(t, srv, d37rContextConnectorPainter)
	if !strings.Contains(painter, "paintConnectors") {
		t.Errorf("D37r: context-connector-painter must remain present for the load-order-safety fallback path")
	}
}

// ── 16. Cytoscape branch installs HTML overlay for rich cards ────

// TestExplorer_ContextCytoscapeBranchMountsHtmlOverlay pins that the
// strategic Cytoscape branch mounts an HTML overlay layer that
// follows Cytoscape rendered positions and is pointer-events:none so
// Cytoscape gets every interaction.
//
// D37r-tranche-B' flip: the overlay mechanism moved out of this file
// (the inline `_mountCytoscapeOverlay` / `_syncCytoscapeOverlay` /
// `_wireCytoscapeOverlaySync` helpers) into the shared platform
// module at /explorer/assets/js/graph/graph-platform/graph-cytoscape-
// overlay.js. The Context renderer now calls
// `graphCytoscapeOverlay.mount(...)` and supplies a per-node template
// + key extractor + `pointerEvents: 'none'` + `stateClasses`. The
// mechanism pins below are flipped to assert the new shape on both
// sides of the boundary: Context calls the shared module; the shared
// module owns the actual sync mechanics.
func TestExplorer_ContextCytoscapeBranchMountsHtmlOverlay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	// Context-side: lens supplies template + options to the shared
	// module.
	if !strings.Contains(js, "function _mountCytoscapeOverlayViaSharedModule(cards)") {
		t.Errorf("D37r-tranche-B' (flipped): Context renderer must declare _mountCytoscapeOverlayViaSharedModule(cards)")
	}
	if !strings.Contains(js, "shared.mount(_cy, _stageEl, {") {
		t.Errorf("D37r-tranche-B' (flipped): Context renderer must call shared.mount(_cy, _stageEl, {...})")
	}
	if !strings.Contains(js, "pointerEvents: 'none',") {
		t.Errorf("D37r-tranche-B' (flipped): Context must pass pointerEvents: 'none' so cy receives every interaction")
	}

	// Shared-module side: owns the live `renderedPosition()` reads,
	// the viewport-event subscription, and the layer DIV creation.
	sharedJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	if !strings.Contains(sharedJS, "n.renderedPosition()") {
		t.Errorf("D37r-tranche-B' (flipped): shared overlay module must read Cytoscape rendered positions via n.renderedPosition()")
	}
	if !strings.Contains(sharedJS, "cy.on(SYNC_EVENTS, _syncBound)") {
		t.Errorf("D37r-tranche-B' (flipped): shared overlay module must subscribe to cy viewport events via SYNC_EVENTS")
	}
	if !strings.Contains(sharedJS, "_layerEl.style.pointerEvents = 'none';") {
		t.Errorf("D37r-tranche-B' (flipped): shared overlay module's layer must be pointer-events:none")
	}
}

// ── 17a. RELATED SERVICE adapter spec matches native ────────────

// TestExplorer_ContextCytoscapeAdapter_RelatedServiceSpecMatchesNative
// is the D37r-tranche-B'-fix-1 parity pin. The Context card model's
// `_buildRelatedBusinessService` (context-card-model.js:220-251)
// emits two meta rows for related-service cards: the relationship
// verb AND the relationship description sentence. The native render
// path at context-graph-view.js:200-217 passes only the verb to
// `renderer.addNode(...)` — the description is intentionally NOT
// surfaced in the card body. Pre-fix the Cytoscape adapter
// (`_contextCardToNativeSpec`) passed BOTH meta rows through
// verbatim, making Cytoscape RELATED SERVICE cards render as tall
// panels with the description wrapping inside `.gmap-node-meta`
// while every other kind rendered compact.
//
// This test pins:
//
//   1. The trim is present in the adapter source — specifically the
//      kind-specific `related_business_service` branch that drops
//      everything past the first meta entry.
//   2. The trim is positioned BEFORE the adapter's return value so
//      the final spec carries the trimmed `meta`.
//   3. The trim is NOT applied to other kinds (no general one-entry
//      cap) — multi-entry meta on `business_service`,
//      `decision_surface`, etc. is intentional and must be
//      preserved.
//   4. Native's single-meta-entry contract for related-service is
//      still expressed at the source-of-truth call site in
//      context-graph-view.js (parity prerequisite).
//   5. The card-model's two-entry shape that necessitates the trim
//      is still present (otherwise the trim is dead code).
func TestExplorer_ContextCytoscapeAdapter_RelatedServiceSpecMatchesNative(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	// 1 + 2. The adapter trims meta for related_business_service
	// before returning the spec.
	if !strings.Contains(js, "if (card.kind === 'related_business_service' && meta.length > 1) {") {
		t.Errorf("D37r-tranche-B'-fix-1: adapter must trim meta for `related_business_service` when more than one entry is present")
	}
	if !strings.Contains(js, "meta = meta.slice(0, 1);") {
		t.Errorf("D37r-tranche-B'-fix-1: adapter must keep only the first meta entry (the relationship verb) for related_business_service")
	}

	// The trim sits inside `_contextCardToNativeSpec`, BEFORE the
	// `return {` block. Locate the function body and verify the trim
	// precedes the return statement.
	adapterIdx := strings.Index(js, "function _contextCardToNativeSpec(card)")
	if adapterIdx < 0 {
		t.Fatal("D37r-tranche-B'-fix-1: _contextCardToNativeSpec must be present")
	}
	tail := js[adapterIdx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatal("D37r-tranche-B'-fix-1: _contextCardToNativeSpec body must be well-formed")
	}
	body := tail[:endRel+1]

	trimIdx := strings.Index(body, "if (card.kind === 'related_business_service' && meta.length > 1) {")
	if trimIdx < 0 {
		t.Fatal("D37r-tranche-B'-fix-1: meta trim must live inside _contextCardToNativeSpec")
	}
	returnIdx := strings.Index(body, "return {")
	if returnIdx < 0 {
		t.Fatal("D37r-tranche-B'-fix-1: _contextCardToNativeSpec must have a `return {` statement")
	}
	if trimIdx >= returnIdx {
		t.Errorf("D37r-tranche-B'-fix-1: meta trim must run BEFORE the return statement (trimIdx=%d, returnIdx=%d)", trimIdx, returnIdx)
	}

	// 3. The trim is NOT a general one-entry cap. The adapter must
	// not contain a kind-agnostic slice or length cap on meta beyond
	// the related_business_service branch.
	for _, banned := range []string{
		"meta = meta.slice(0, 1);\n    return",  // trim immediately followed by return = unconditional cap
		"if (meta.length > 1) {\n      meta = meta.slice(0, 1);\n    }\n    return",
		"meta.length = 1;",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37r-tranche-B'-fix-1: adapter must not cap meta uniformly to one entry — multi-entry meta for other kinds (business_service, decision_surface, ai_system) is intentional. Found banned pattern %q", banned)
		}
	}

	// 4. Native's source-of-truth: the inline addNode call for the
	// related-service row passes meta as a single-element array of
	// the relationship verb. If this changes, the parity contract
	// itself shifts and this test should be revisited.
	nativeView := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(nativeView, "meta: rel.relationship_type ? [rel.relationship_type] : [],") {
		t.Errorf("D37r-tranche-B'-fix-1: native related-service call site must keep its single-meta-entry contract (meta: rel.relationship_type ? [rel.relationship_type] : [])")
	}

	// 5. The card-model's two-entry shape is the reason the trim
	// exists. If the model stops emitting a description row, the
	// trim becomes dead code; pin the two-push pattern that
	// motivates this fix.
	cardModel := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-card-model.js")
	if !strings.Contains(cardModel, "if (relType) meta.push(_emptyMetaRow(_str(relType)));") {
		t.Errorf("D37r-tranche-B'-fix-1: card-model parity prerequisite — _buildRelatedBusinessService must still push relationship_type onto meta")
	}
	if !strings.Contains(cardModel, "if (relRow && relRow.description) {\n      meta.push(_emptyMetaRow(_str(relRow.description)));\n    }") {
		t.Errorf("D37r-tranche-B'-fix-1: card-model parity prerequisite — _buildRelatedBusinessService must still push the relationship description as a second meta row (otherwise the adapter trim is dead code)")
	}
}

// ── 17b. Edge mapper uses prefixed `<kind>:<id>` shape ───────────

// TestExplorer_ContextCytoscapeEdges_UsePrefixedIdShape pins the
// runtime-correctness contract for `_buildContextCytoscapeEdges`.
//
// Why this test exists: the prior source-contract test
// (TestExplorer_ContextConnectorsMapToCytoscapeEdges) pinned the
// edge-builder's STRUCTURE — that the function exists, emits a
// `group: 'edges'` element shape, references the right token names,
// etc. It did NOT pin the runtime CORRECTNESS property that endpoint
// keys must match the cy node id shape. As a result, a regression
// where the builder derived endpoint keys from BARE `c.source.id`
// (instead of the prefixed `kind + ':' + id` form used by cy node
// ids and stage.cards keys) shipped invisibly: every connector hit
// the stage.cards guard's `continue` and was dropped, leaving
// `cy.edges().length === 0` despite the structural test passing.
//
// This test closes the gap by pinning the kind-aware key composition
// in the load-bearing source path. If a future change reverts the
// derivation to bare `c.source.id`, this test fails.
func TestExplorer_ContextCytoscapeEdges_UsePrefixedIdShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	// The canonical key composer must be present in source.
	if !strings.Contains(js, "function _connectorEndpointKey(ref)") {
		t.Errorf("D37r-tranche-B'-fix-2: _connectorEndpointKey(ref) helper must be present in context-cytoscape-renderer.js (canonical kind+':'+id composer for Cytoscape edge endpoints)")
	}

	// The composer must build the prefixed `<kind>:<id>` shape.
	if !strings.Contains(js, "return kind + ':' + id;") {
		t.Errorf("D37r-tranche-B'-fix-2: _connectorEndpointKey must compose its return as `kind + ':' + id` to match the stage.cards key shape and cy node id shape")
	}

	// The edge mapper must derive srcId / dstId via the composer
	// (not the bare-id path that previously regressed).
	if !strings.Contains(js, "var srcId = _connectorEndpointKey(c.source);") {
		t.Errorf("D37r-tranche-B'-fix-2: _buildContextCytoscapeEdges must derive srcId via _connectorEndpointKey(c.source) — the prefixed shape — not bare c.source.id")
	}
	if !strings.Contains(js, "var dstId = _connectorEndpointKey(c.target);") {
		t.Errorf("D37r-tranche-B'-fix-2: _buildContextCytoscapeEdges must derive dstId via _connectorEndpointKey(c.target) — the prefixed shape — not bare c.target.id")
	}

	// Negative pin: the pre-fix bare-id derivation must be GONE.
	// If a future change re-introduces it, this test catches the
	// regression immediately.
	bareSrcPattern := `var srcId = String(c\.source\.id != null \? c\.source\.id : '');`
	bareDstPattern := `var dstId = String(c\.target\.id != null \? c\.target\.id : '');`
	if regexp.MustCompile(bareSrcPattern).MatchString(js) {
		t.Errorf("D37r-tranche-B'-fix-2: pre-fix bare srcId derivation (without kind prefix) must NOT be present in source")
	}
	if regexp.MustCompile(bareDstPattern).MatchString(js) {
		t.Errorf("D37r-tranche-B'-fix-2: pre-fix bare dstId derivation (without kind prefix) must NOT be present in source")
	}
}

// ── 17. Destroy path tears down Cytoscape cleanly ────────────────

// D37r-tranche-B'' flip: cy ownership moved into the engine module.
// Teardown is now expressed via the engine handle's destroy() method
// (the engine itself fans out to cy.destroy(), overlay teardown,
// cameraBus deregistration, ResizeObserver disconnect). The
// availability-retry mechanism has been deleted — the vendor script
// tag now precedes the engine module so `window.cytoscape` is
// synchronously available; _cancelCytoscapeRetry no longer exists.
func TestExplorer_ContextCytoscapeDestroyTearsDownCleanly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rContextRendererAsset)

	if !strings.Contains(js, "function _destroyCytoscape()") {
		t.Errorf("D37r-tranche-B'' (flipped): Cytoscape branch must still declare _destroyCytoscape() teardown helper (now routes to the engine handle)")
	}
	if !strings.Contains(js, "_engineHandle.destroy()") {
		t.Errorf("D37r-tranche-B'' (flipped): Cytoscape teardown must call _engineHandle.destroy() — the engine's destroy() handles cy.destroy(), overlay teardown, and cameraBus deregistration")
	}
	// The renderer's _destroy lifecycle must call _destroyCytoscape
	// BEFORE the camera-controller teardown so bus deregistration
	// does not race with in-flight cy events.
	dIdx := strings.Index(js, "function _destroy()")
	if dIdx < 0 {
		t.Fatal("D37r-tranche-B'' (flipped): _destroy() must be present")
	}
	dTail := js[dIdx:]
	dEnd := strings.Index(dTail[1:], "\n  function ")
	if dEnd < 0 {
		t.Fatalf("D37r-tranche-B'' (flipped): _destroy body must be well-formed")
	}
	dBody := dTail[:dEnd+1]
	if !strings.Contains(dBody, "_destroyCytoscape();") {
		t.Errorf("D37r-tranche-B'' (flipped): _destroy() must call _destroyCytoscape() to release the live engine handle")
	}
	// Negative pin: _cancelCytoscapeRetry must be GONE — the
	// availability-retry mechanism was deleted in this tranche.
	if strings.Contains(dBody, "_cancelCytoscapeRetry") {
		t.Errorf("D37r-tranche-B'' (flipped): _destroy() must NOT reference _cancelCytoscapeRetry — the retry mechanism was deleted (vendor script tag now precedes engine module, so cy is synchronously available)")
	}
	// Negative pin (function declarations only — references in
	// comments explaining the deletion are intentional historical
	// signposts).
	for _, decl := range []string{
		"function _cytoscapeAvailable(",
		"function _scheduleCytoscapeRetry(",
		"function _cancelCytoscapeRetry(",
	} {
		if strings.Contains(js, decl) {
			t.Errorf("D37r-tranche-B'' (flipped): availability-retry helper declaration %q must NOT remain in source — it was deleted with the vendor-tag move", decl)
		}
	}
}
