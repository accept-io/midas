package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37r-context-cytoscape-3-impl — Context Cytoscape Styling and Edge
// Semantics (tranche B of the strategic Context Cytoscape convergence
// roadmap).
//
// Source-contract tests for the strategic spatial Context renderer's
// Cytoscape style layer + overlay state synchronisation. Pins parity
// between the Cytoscape edge styles and the strategic Context CSS
// (`/explorer/assets/css/context-cytoscape-renderer.css`) — colour
// tokens, widths, opacities, dash patterns, and per-class semantics.
// Also pins the overlay class-application path that toggles
// `is-selected` / `is-hover` on overlay cards from Cytoscape events.

const (
	d37rB_RendererAsset       = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37rB_StrategicCssAsset   = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37rB_GovernanceCssAsset  = "/explorer/assets/css/governance-map.css"
	d37rB_AuthorityCssAsset   = "/explorer/assets/css/authority-cytoscape-poc.css"
	d37rB_AuthorityGraphCss   = "/explorer/assets/css/authority-graph.css"
	d37rB_AuthorityOverlayCss = "/explorer/assets/css/cytoscape-html-overlay.css"
	d37rB_TokensCss           = "/explorer/assets/css/tokens.css"
	d37rB_PainterAsset        = "/explorer/assets/js/graph/context/context-html-card-painter.js"
	d37rB_ConnectorPainter    = "/explorer/assets/js/graph/context/context-connector-painter.js"
)

// ── helper: slice the Cytoscape style builder body ───────────────

func sliceStyleBuilderBody(t *testing.T, js string) string {
	t.Helper()
	const sig = "function _buildContextCytoscapeStyle()"
	idx := strings.Index(js, sig)
	if idx < 0 {
		t.Fatal("D37r tranche B: _buildContextCytoscapeStyle() must be present")
	}
	tail := js[idx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatal("D37r tranche B: _buildContextCytoscapeStyle body must be well-formed")
	}
	return tail[:endRel+1]
}

// ── 1. Node kind class hooks reach the overlay card ──────────────

// TestExplorer_ContextCytoscapeStyle_NodeKindsCoverAllNativeKinds pins
// that every native ContextCard kind continues to reach the overlay
// card's class list via the existing `contextCardPainter.renderCard`
// (which applies `context-card--<kind>`). The Cytoscape style layer
// itself does NOT style per-kind because the visible card is the
// overlay; the painter applies the kind class unchanged.
func TestExplorer_ContextCytoscapeStyle_NodeKindsCoverAllNativeKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	painter := getExplorerAsset(t, srv, d37rB_PainterAsset)

	// The painter still emits `context-card--<kind>` for every kind.
	if !strings.Contains(painter, "KIND_CLASS_PREFIX  = 'context-card--';") {
		t.Errorf("D37r tranche B: contextCardPainter must keep emitting `context-card--<kind>` so overlay cards inherit kind classes")
	}
	if !strings.Contains(painter, "kindCls = _str(card.kind) ? (KIND_CLASS_PREFIX + _str(card.kind)) : '';") {
		t.Errorf("D37r tranche B: painter must derive kindCls from card.kind")
	}

	// The Cytoscape edge mapper emits node data carrying `kind`, so
	// the overlay card-by-id map sees the same identity as the cy
	// node — preserving kind class application.
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)
	if !strings.Contains(js, "kind:     String(c.kind || ''),") {
		t.Errorf("D37r tranche B: Cytoscape node data must carry `kind` so kind classes can be reconciled with overlay cards")
	}
}

// ── 2. Emphasis / role / selected / hover state parity ──────────

// D37r-tranche-B' flip: hover-class and selected-class state sync
// moved out of this file (the inline `_wireCytoscapeOverlayStateSync`
// helper plus the per-event `_cy.on('mouseover'/'mouseout', …)`
// subscriptions) into the shared platform module at
// /explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js.
// The Context renderer now passes `stateClasses: { selected: 'selected',
// hover: 'is-hover' }` + `syncSelected: true` + `syncHover: true` to
// `graphCytoscapeOverlay.mount(...)` and the shared module wires the
// cy events. The test below is flipped to assert the new shape.
func TestExplorer_ContextCytoscapeStyle_NodeEmphasisCoverAllNativeStates(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)

	// Context passes the locked state-class vocabulary to the shared
	// overlay module. Selected → 'selected' fires the native
	// `gmap-node.selected` CSS rule directly on the inner card;
	// hover → 'is-hover' fires the existing scoped Cytoscape-engine
	// rule from context-cytoscape-renderer.css.
	if !strings.Contains(js, "stateClasses: { selected: 'selected', hover: 'is-hover' },") {
		t.Errorf("D37r-tranche-B' (flipped): Context must declare stateClasses: { selected: 'selected', hover: 'is-hover' }")
	}
	if !strings.Contains(js, "syncSelected: true,") {
		t.Errorf("D37r-tranche-B' (flipped): Context must enable syncSelected: true so cy-driven selection toggles the selected class")
	}
	if !strings.Contains(js, "syncHover:    true,") {
		t.Errorf("D37r-tranche-B' (flipped): Context must enable syncHover: true so cy-driven hover toggles the hover class")
	}

	// The shared module owns the actual cy event subscriptions.
	sharedJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	for _, want := range []string{
		"cy.on('select',   'node', _selectBound)",
		"cy.on('unselect', 'node', _unselectBound)",
		"cy.on('mouseover', 'node', _mouseoverBound)",
		"cy.on('mouseout',  'node', _mouseoutBound)",
	} {
		if !strings.Contains(sharedJS, want) {
			t.Errorf("D37r-tranche-B' (flipped): shared overlay module must own the state-class event subscription %q", want)
		}
	}

	// Selected-state ALSO continues to flow through the canonical
	// selection bridge — the D37q-viewport-5 contract intact.
	if regexp.MustCompile(`_cy\.on\(\s*'select'`).MatchString(_stripJSComments(js)) {
		t.Errorf("D37r-tranche-B' (flipped): Context module must not subscribe to raw Cytoscape 'select' events directly (canonical path is graphSelectionBridge / contextSelectionBridge per D37q-viewport-5)")
	}
	if !strings.Contains(js, "function _subscribeToSelectionBridge()") {
		t.Errorf("D37r-tranche-B' (flipped): bridge-subscription helper must remain — _subscribeToSelectionBridge is the canonical selected-state read path")
	}
	if !strings.Contains(js, "function _applySelectionVisual(selectedId)") {
		t.Errorf("D37r-tranche-B' (flipped): _applySelectionVisual must remain")
	}
}

// ── 3. Five visual classes match native colour tokens ────────────

func TestExplorer_ContextCytoscapeStyle_FiveVisualClassesMatchNativeColour(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)
	css := getExplorerAsset(t, srv, d37rB_StrategicCssAsset)
	body := sliceStyleBuilderBody(t, js)

	// All five visual-class selectors present in the style array.
	for _, want := range []string{
		"selector: 'edge.context-edge-visual-service',",
		"selector: 'edge.context-edge-visual-ai_binding',",
		"selector: 'edge.context-edge-visual-authority',",
		"selector: 'edge.context-edge-visual-evidence',",
		"selector: 'edge.context-edge-visual-gap',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37r tranche B: style array must include %q", want)
		}
	}

	// Per-class colour parity. Each Cytoscape rule must consume the
	// SAME token the native CSS rule uses (read at runtime via
	// `_readCssVar`). The mapping is:
	//   service   → --outline-variant
	//   ai_binding → --primary
	//   authority → --primary
	//   evidence  → --on-surface-variant
	//   gap       → --badge-bad
	for _, want := range []string{
		"serviceColor   = _readCssVar('--outline-variant',",
		"aiBindingColor = _readCssVar('--primary',",
		"authorityColor = _readCssVar('--primary',",
		"evidenceColor  = _readCssVar('--on-surface-variant',",
		"gapColor       = _readCssVar('--badge-bad',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37r tranche B: visual-class colour must derive from a runtime token (%q)", want)
		}
	}

	// The same tokens are demonstrably referenced by the native CSS.
	for _, native := range []string{
		"stroke: var(--outline-variant",    // service / base
		"stroke: var(--primary",            // ai_binding + authority
		"stroke: var(--on-surface-variant", // evidence
		"stroke: var(--badge-bad",          // gap
	} {
		if !strings.Contains(css, native) {
			t.Errorf("D37r tranche B: parity prerequisite — native CSS must still reference %q", native)
		}
	}

	// Per-class width parity (matches native CSS literally).
	for _, w := range []string{
		"'width':      1.4,",                  // service / evidence
		"'width':      1.6,",                  // ai_binding / gap
		"'width':             1.5,",           // authority (longer key padding)
	} {
		if !strings.Contains(body, w) {
			t.Errorf("D37r tranche B: per-visual-class width parity must include %q", w)
		}
	}

	// Per-class opacity parity.
	for _, o := range []string{
		"'opacity':    0.72,",  // service
		"'opacity':    0.88,",  // ai_binding
		"'opacity':           0.82,",  // authority
		"'opacity':    0.78,",  // evidence
		"'opacity':           0.92,",  // gap
	} {
		if !strings.Contains(body, o) {
			t.Errorf("D37r tranche B: per-visual-class opacity parity must include %q", o)
		}
	}
}

// ── 4. Dashed-edge configuration is non-default and per-class ───

func TestExplorer_ContextCytoscapeStyle_DashSemanticsRenderDashed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)
	body := sliceStyleBuilderBody(t, js)

	// Authority — dashed 6,4 (matches CSS `stroke-dasharray="6 4"`
	// emitted by the connector painter for DASH_6_4).
	if !regexp.MustCompile(`'edge\.context-edge-visual-authority'[\s\S]*?'line-style':\s*'dashed'`).MatchString(body) {
		t.Errorf("D37r tranche B: authority visual class must be dashed")
	}
	if !regexp.MustCompile(`'edge\.context-edge-visual-authority'[\s\S]*?'line-dash-pattern':\s*\[6,\s*4\]`).MatchString(body) {
		t.Errorf("D37r tranche B: authority visual class must declare line-dash-pattern [6, 4]")
	}

	// Gap — dashed 5,5 (matches CSS for DASH_5_5).
	if !regexp.MustCompile(`'edge\.context-edge-visual-gap'[\s\S]*?'line-style':\s*'dashed'`).MatchString(body) {
		t.Errorf("D37r tranche B: gap visual class must be dashed")
	}
	if !regexp.MustCompile(`'edge\.context-edge-visual-gap'[\s\S]*?'line-dash-pattern':\s*\[5,\s*5\]`).MatchString(body) {
		t.Errorf("D37r tranche B: gap visual class must declare line-dash-pattern [5, 5]")
	}

	// Generic dashed marker (kept for forward-compatibility with
	// future visual classes that share dash semantics).
	if !strings.Contains(body, "selector: 'edge.context-edge-dash-dashed',") {
		t.Errorf("D37r tranche B: context-edge-dash-dashed marker selector must remain present")
	}
}

// ── 5. Solid edge configuration ──────────────────────────────────

func TestExplorer_ContextCytoscapeStyle_DashSemanticsRenderSolid(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)
	body := sliceStyleBuilderBody(t, js)

	// Service / ai_binding / evidence are solid. The style array is
	// a flat list of `{ selector, style: { ... } }` objects; for
	// each solid visual class we slice out only that entry's block
	// (selector → closing `}` of its style object) and assert the
	// per-block contents. Cross-entry regex matches are rejected.
	for _, klass := range []string{"service", "ai_binding", "evidence"} {
		anchor := "selector: 'edge.context-edge-visual-" + klass + "',"
		idx := strings.Index(body, anchor)
		if idx < 0 {
			t.Errorf("D37r tranche B: style array must include selector for visual class %q", klass)
			continue
		}
		// Slice from the anchor up to the next selector entry (or
		// the end of the array). Each entry's style object is flat,
		// so a single-block slice suffices.
		afterAnchor := body[idx+len(anchor):]
		nextSelector := strings.Index(afterAnchor, "selector: ")
		var block string
		if nextSelector >= 0 {
			block = afterAnchor[:nextSelector]
		} else {
			block = afterAnchor
		}
		if !strings.Contains(block, "'line-style': 'solid'") {
			t.Errorf("D37r tranche B: visual class %q block must declare 'line-style': 'solid'", klass)
		}
		if strings.Contains(block, "line-dash-pattern") {
			t.Errorf("D37r tranche B: visual class %q block must not carry a line-dash-pattern", klass)
		}
	}

	// Generic solid marker entry remains present.
	if !strings.Contains(body, "selector: 'edge.context-edge-dash-solid',") {
		t.Errorf("D37r tranche B: context-edge-dash-solid marker selector must remain present")
	}
}

// ── 6. No edge selected / hover styling (parity: native has none) ─

func TestExplorer_ContextCytoscapeStyle_EdgeSelectedHoverParity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)
	css := getExplorerAsset(t, srv, d37rB_StrategicCssAsset)
	body := sliceStyleBuilderBody(t, js)

	// Parity baseline: native CSS has no `.context-connector:hover`,
	// `.context-connector:selected`, or `.context-connector.is-*`
	// rule. Cytoscape must therefore not introduce one either.
	for _, banned := range []string{
		".context-connector:hover",
		".context-connector:selected",
		".context-connector.is-selected",
		".context-connector.is-hover",
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D37r tranche B: parity prerequisite — native strategic CSS unexpectedly contains %q (test must be updated)", banned)
		}
	}

	// Cytoscape edge `:selected` and `:hover` styling must be absent
	// (we don't add what native doesn't express).
	if strings.Contains(body, "selector: 'edge:selected'") {
		t.Errorf("D37r tranche B: edge:selected styling must not appear (native has no edge :selected)")
	}
	if strings.Contains(body, "selector: 'edge:active'") || strings.Contains(body, "selector: 'edge:hover'") {
		t.Errorf("D37r tranche B: edge hover styling must not appear (native has no edge :hover)")
	}
}

// ── 7. Adjacent-edge emphasis on node selection (native: none) ──

// D37r-tranche-B' flip: the inline `_wireCytoscapeOverlayStateSync`
// helper is retired (mechanism moved to the shared module). The
// adjacent-edge-emphasis ban is re-pinned by scanning the shared
// overlay module instead — it must not touch edges either.
func TestExplorer_ContextCytoscapeStyle_AdjacentEdgeEmphasisOnNodeSelection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	sharedJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	stripped := _stripJSComments(sharedJS)

	// The shared overlay module's state-class application path must
	// not touch edges — its event subscriptions are scoped to 'node'
	// only.
	for _, banned := range []string{
		"cy.edges()",
		".connectedEdges(",
		"cy.on('select',   'edge'",
		"cy.on('mouseover', 'edge'",
	} {
		if strings.Contains(stripped, banned) {
			t.Errorf("D37r-tranche-B' (flipped): shared overlay module must not touch edges (found %q) — adjacent-edge emphasis is not native behaviour", banned)
		}
	}

	// The Context renderer also must not subscribe to cy edge events
	// for overlay-state purposes.
	contextJS := getExplorerAsset(t, srv, d37rB_RendererAsset)
	contextStripped := _stripJSComments(contextJS)
	for _, banned := range []string{
		"_cy.on('select',   'edge'",
		"_cy.on('mouseover', 'edge'",
		"_cy.on('mouseout',  'edge'",
	} {
		if strings.Contains(contextStripped, banned) {
			t.Errorf("D37r-tranche-B' (flipped): Context renderer must not subscribe to cy edge events (found %q)", banned)
		}
	}
}

// ── 8. Arrow parity (native: none) ────────────────────────────────

func TestExplorer_ContextCytoscapeStyle_ArrowParity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)
	body := sliceStyleBuilderBody(t, js)

	// Native CSS uses `stroke` + `stroke-width` only — no marker-
	// end / arrow shape. Cytoscape must echo `target-arrow-shape:
	// 'none'` and `source-arrow-shape: 'none'` on the base edge.
	if !strings.Contains(body, "'target-arrow-shape':   'none',") {
		t.Errorf("D37r tranche B: base edge must declare target-arrow-shape: 'none' (native has no arrows)")
	}
	if !strings.Contains(body, "'source-arrow-shape':   'none',") {
		t.Errorf("D37r tranche B: base edge must declare source-arrow-shape: 'none' (native has no arrows)")
	}
	// No per-class arrow shape overrides.
	if regexp.MustCompile(`'edge\.context-edge-visual-[^']+'[\s\S]*?'target-arrow-shape'\s*:\s*'(triangle|circle|square|chevron|tee|diamond|vee)'`).MatchString(body) {
		t.Errorf("D37r tranche B: no visual-class may introduce a non-none arrow shape (native has no arrows)")
	}
}

// ── 9. Cytoscape style consumes design tokens ────────────────────

func TestExplorer_ContextCytoscapeStyle_ConsumesDesignTokens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)

	// Helper exists and reads CSS custom properties at runtime.
	if !strings.Contains(js, "function _readCssVar(name, fallback)") {
		t.Errorf("D37r tranche B: _readCssVar(name, fallback) helper must be present")
	}
	if !strings.Contains(js, "window.getComputedStyle(document.documentElement)") {
		t.Errorf("D37r tranche B: _readCssVar must read tokens off document.documentElement at runtime")
	}
	if !strings.Contains(js, ".getPropertyValue(name)") {
		t.Errorf("D37r tranche B: _readCssVar must call getPropertyValue(name)")
	}

	// Each Cytoscape visual class consumes its token.
	body := sliceStyleBuilderBody(t, js)
	for _, want := range []string{
		"_readCssVar('--outline-variant',",
		"_readCssVar('--primary',",
		"_readCssVar('--on-surface-variant',",
		"_readCssVar('--badge-bad',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37r tranche B: style builder must call %q", want)
		}
	}
}

// ── 10. No new design tokens introduced ──────────────────────────

func TestExplorer_ContextCytoscapeStyle_NoNewDesignTokensIntroduced(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokens := getExplorerAsset(t, srv, d37rB_TokensCss)

	// The strategic Cytoscape build must not introduce any new
	// `--midas-*` token. The repo's existing token surface uses
	// MDC-style names (`--primary`, `--outline-variant`, etc.) and
	// does not currently have `--midas-*` names; ensure that
	// remains the case.
	if strings.Contains(tokens, "--midas-") {
		t.Errorf("D37r tranche B: tokens.css must not contain new `--midas-*` tokens (none should be introduced)")
	}

	// Tokens this build consumes from tokens.css must already exist
	// (parity prerequisite). `--badge-bad` is consumed only as a
	// var() fallback target — it is NOT a defined token in
	// tokens.css today, and this build does NOT introduce one. The
	// strategic Context CSS at context-cytoscape-renderer.css:322
	// already uses `var(--badge-bad, #ff6b6b)` with the literal
	// `#ff6b6b` fallback, and the Cytoscape style layer consumes
	// `_readCssVar('--badge-bad', '#ff6b6b')` with the same fallback —
	// neither side defines the token.
	for _, existing := range []string{
		"--outline-variant:",
		"--primary:",
		"--on-surface-variant:",
	} {
		if !strings.Contains(tokens, existing) {
			t.Errorf("D37r tranche B: existing token %q must remain defined in tokens.css", existing)
		}
	}

	// `--badge-bad` deliberately is NOT defined as a token in
	// tokens.css. Pin the fallback-only contract: both the strategic
	// Context CSS and the Cytoscape style layer reference the same
	// undefined-with-fallback shape.
	if strings.Contains(tokens, "--badge-bad:") {
		t.Errorf("D37r tranche B: tokens.css must not introduce `--badge-bad:` (was not defined in baseline; build does not introduce new tokens)")
	}
}

// ── 11. Overlay selected class is applied via the canonical bridge ─

// TestExplorer_ContextCytoscapeOverlay_SelectedClassAppliedOnCySelect
// pins that overlay cards receive the canonical `.is-selected`
// class on a selection change. The D37q-viewport-5 canonicalisation
// pins Context modules MUST NOT subscribe directly to raw Cytoscape
// `select` events; the canonical write surface is
// `contextSelectionBridge.selectCard(card)` (called from the cy tap
// handler) and the canonical read surface is the bridge's
// `subscribe(...)` callback in `_subscribeToSelectionBridge`. That
// callback feeds `_applySelectionVisual(selectedId)` which iterates
// `_cardEls` — the same map `_mountCytoscapeOverlay` populates via
// `_trackCardEl`. So overlay cards receive `.is-selected` /
// `aria-current` from the bridge for free.
// D37r-tranche-B' flip: selected-class application moved out of the
// Context renderer's inline `_mountCytoscapeOverlay` + bridge-driven
// `_applySelectionVisual` path into the shared overlay module's
// `syncSelected: true` subscription, which toggles the configured
// selected class (`'selected'` per Context's stateClasses option) on
// the wrapper AND inner card. The Context bridge-subscription path
// remains present for cross-cutting selection state, but the
// overlay's selected-class is now driven by cy.select events through
// the shared module — not by the bridge iterating `_cardEls`. The
// D37q-viewport-5 ban on Context modules subscribing to raw cy
// selection events is preserved: the shared module is a PLATFORM
// module, not a Context module, so its `cy.on('select', 'node', …)`
// subscription is allowed.
func TestExplorer_ContextCytoscapeOverlay_SelectedClassAppliedOnCySelect(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)

	// Context opts the shared module into selected-class application.
	if !strings.Contains(js, "syncSelected: true,") {
		t.Errorf("D37r-tranche-B' (flipped): Context must opt into syncSelected: true")
	}
	if !strings.Contains(js, "selected: 'selected'") {
		t.Errorf("D37r-tranche-B' (flipped): Context must declare selected class 'selected' (fires native gmap-node.selected CSS)")
	}

	// Shared module subscribes to cy select/unselect.
	sharedJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	if !regexp.MustCompile(`cy\.on\('select',\s*'node',\s*_selectBound\)`).MatchString(sharedJS) {
		t.Errorf("D37r-tranche-B' (flipped): shared overlay module must subscribe to cy 'select' on node")
	}
	if !regexp.MustCompile(`cy\.on\('unselect',\s*'node',\s*_unselectBound\)`).MatchString(sharedJS) {
		t.Errorf("D37r-tranche-B' (flipped): shared overlay module must subscribe to cy 'unselect' on node")
	}

	// Bridge subscription path stays — still the canonical READ
	// seam for cross-cutting selection state.
	if !strings.Contains(js, "function _subscribeToSelectionBridge()") {
		t.Errorf("D37r-tranche-B' (flipped): bridge-subscription helper must remain (canonical bridge-driven READ path)")
	}
	if !strings.Contains(js, "g.contextSelectionBridge.subscribe(function (card)") {
		t.Errorf("D37r-tranche-B' (flipped): renderer must continue to subscribe to contextSelectionBridge")
	}

	// `_applySelectionVisual` remains for non-overlay selection
	// visuals (legacy DOM path, non-spatial fallback, etc.).
	if !strings.Contains(js, "function _applySelectionVisual(selectedId)") {
		t.Errorf("D37r-tranche-B' (flipped): _applySelectionVisual must remain")
	}

	// D37q-viewport-5 ban: Context module itself must not subscribe
	// to raw cy 'select' / 'unselect'. (The shared platform module
	// is allow-listed under the D37q-viewport-5 contract — overlay
	// state-class application is a presentation concern, not a
	// governance concern.)
	if regexp.MustCompile(`_cy\.on\(\s*'select'`).MatchString(_stripJSComments(js)) {
		t.Errorf("D37r-tranche-B' (flipped): Context module must not subscribe to raw Cytoscape 'select' events in load-bearing code")
	}
	if regexp.MustCompile(`_cy\.on\(\s*'unselect'`).MatchString(_stripJSComments(js)) {
		t.Errorf("D37r-tranche-B' (flipped): Context module must not subscribe to raw Cytoscape 'unselect' events in load-bearing code")
	}
}

// ── 12. Cytoscape mouseover/mouseout applies overlay hover class ─
//
// D37r-tranche-B' flip: the hover-class application moved out of the
// Context renderer's inline `_wireCytoscapeOverlayStateSync` helper
// into the shared overlay module's `syncHover: true` subscription.
// The Context renderer opts in via `stateClasses: { hover: 'is-hover' }`.
// This test is flipped to assert the new shape: the wiring lives in
// the shared module; Context declares the class name. The
// Cytoscape-scoped `.context-card.is-hover` CSS rule still remains
// in the strategic stylesheet (it pre-existed tranche B') so the
// hover visual is still expressed.
func TestExplorer_ContextCytoscapeOverlay_HoverClassAppliedOnCyHover(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)

	// Context opts the shared module into hover-class application.
	if !strings.Contains(js, "syncHover:    true,") {
		t.Errorf("D37r-tranche-B' (flipped): Context renderer must opt into syncHover: true")
	}
	if !strings.Contains(js, "hover: 'is-hover'") {
		t.Errorf("D37r-tranche-B' (flipped): Context renderer must declare hover class 'is-hover'")
	}

	// The shared module owns the cy mouseover/mouseout wiring.
	sharedJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js")
	if !regexp.MustCompile(`cy\.on\('mouseover',\s*'node',\s*_mouseoverBound\)`).MatchString(sharedJS) {
		t.Errorf("D37r-tranche-B' (flipped): shared overlay module must subscribe cy 'mouseover' on node")
	}
	if !regexp.MustCompile(`cy\.on\('mouseout',\s*'node',\s*_mouseoutBound\)`).MatchString(sharedJS) {
		t.Errorf("D37r-tranche-B' (flipped): shared overlay module must subscribe cy 'mouseout' on node")
	}

	// Scoped `.is-hover` rule remains on the strategic Cytoscape CSS
	// path (carried over from tranche B; the shared module reuses
	// the same class name).
	css := getExplorerAsset(t, srv, d37rB_StrategicCssAsset)
	if !regexp.MustCompile(`(?s)\.context-renderer-canvas\[data-spatial="true"\]\[data-engine="cytoscape"\][\s\S]*?\.context-card\.is-hover\s*\{[\s\S]*?border-color:\s*var\(--primary`).MatchString(css) {
		t.Errorf("D37r-tranche-B' (flipped): strategic CSS must keep the Cytoscape-scoped .context-card.is-hover rule")
	}
}

// ── 13. Overlay card kind / role classes come from painter ───────

func TestExplorer_ContextCytoscapeOverlay_KindClassesAppliedFromNodeData(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	painter := getExplorerAsset(t, srv, d37rB_PainterAsset)

	// The painter — shared with the native strategic non-spatial
	// path — applies kind + role classes from the card model.
	if !strings.Contains(painter, "if (kindCls) classes += ' ' + kindCls;") {
		t.Errorf("D37r tranche B: painter must apply context-card--<kind> class to the rendered card")
	}
	if !strings.Contains(painter, "if (roleCls) classes += ' ' + roleCls;") {
		t.Errorf("D37r tranche B: painter must apply context-card--role-<role> class to the rendered card")
	}

	// The renderer continues to pass the same card model into the
	// painter for the Cytoscape overlay path (no shape difference).
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)
	if !strings.Contains(js, "var el = painter.renderCard(card, null);") {
		t.Errorf("D37r tranche B: Cytoscape overlay must continue to invoke painter.renderCard(card, null) so kind/role classes are applied")
	}
}

// ── 14. Painter source unchanged ─────────────────────────────────

func TestExplorer_ContextCytoscapeOverlay_PainterNotModified(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	painter := getExplorerAsset(t, srv, d37rB_PainterAsset)

	// Constants the existing tests pin — if these drift, the painter
	// has been modified.
	for _, anchor := range []string{
		"var CARD_CLASS         = 'context-card';",
		"var KIND_CLASS_PREFIX  = 'context-card--';",
		"var ROLE_CLASS_PREFIX  = 'context-card--role-';",
		"function renderCard(card, options)",
		"window.MIDASExplorerGraph.contextCardPainter",
	} {
		if !strings.Contains(painter, anchor) {
			t.Errorf("D37r tranche B: contextCardPainter source must remain unmodified — anchor missing: %q", anchor)
		}
	}
}

// ── 15. Cytoscape node base remains visually inert ───────────────

func TestExplorer_ContextCytoscapeStyle_NodeShapeDoesNotCompeteWithOverlay(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)
	body := sliceStyleBuilderBody(t, js)

	// Base node is transparent / borderless / no label so the
	// canvas never paints a competing shape over the overlay card.
	if !strings.Contains(body, "'background-color': 'rgba(0,0,0,0)',") {
		t.Errorf("D37r tranche B: base node background-color must be transparent")
	}
	if !strings.Contains(body, "'background-opacity': 0,") {
		t.Errorf("D37r tranche B: base node background-opacity must be 0")
	}
	if !strings.Contains(body, "'border-width':     0,") {
		t.Errorf("D37r tranche B: base node border-width must be 0")
	}
	if !strings.Contains(body, "'label':            '',") {
		t.Errorf("D37r tranche B: base node label must be empty")
	}

	// Selected and root nodes do not introduce a Cytoscape visual.
	if !regexp.MustCompile(`'node:selected'[\s\S]*?'overlay-opacity':\s*0`).MatchString(body) {
		t.Errorf("D37r tranche B: node:selected must keep overlay-opacity: 0 (overlay owns the visible state)")
	}
	if !regexp.MustCompile(`'node\.context-node-emphasis-root'[\s\S]*?'overlay-opacity':\s*0`).MatchString(body) {
		t.Errorf("D37r tranche B: root-emphasis node must keep overlay-opacity: 0")
	}
}

// ── 16. Native Context CSS unchanged (governance-map.css) ────────

// TestExplorer_ContextCytoscapeStyle_NativeContextStylingUnchanged pins
// the load-bearing anchors in the LEGACY native Context CSS
// (`governance-map.css`). Drift in any of these constants indicates
// the legacy native renderer's visual contract has been touched —
// which this tranche must not do (strategic Cytoscape work only).
func TestExplorer_ContextCytoscapeStyle_NativeContextStylingUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37rB_GovernanceCssAsset)

	for _, anchor := range []string{
		".gmap-node {",
		".gmap-node.selected {",
		".gmap-node:hover { background: var(--surface-container-high); }",
		".gmap-node.gmap-root-node {",
		".gmap-connector {",
		".gmap-connector.is-hovered {",
		".business-service-node .gmap-node-label::before",
		".capability-node       .gmap-node-label::before",
		".decision-surface-node .gmap-node-label::before",
		".ai-system-node        .gmap-node-label::before",
		".authority-node        .gmap-node-label::before",
		".coverage-node         .gmap-node-label::before",
	} {
		if !strings.Contains(css, anchor) {
			t.Errorf("D37r tranche B: legacy native Context CSS must remain unchanged — anchor missing: %q", anchor)
		}
	}
}

// ── 17. Authority CSS unchanged ──────────────────────────────────

func TestExplorer_ContextCytoscapeStyle_AuthorityStylingUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Authority Cytoscape PoC CSS — confirm presence (this asset
	// must continue to be served and must continue to scope its
	// chrome to the Cytoscape PoC body class). A literal anchor in
	// each is sufficient as a tamper detector.
	pocCss := getExplorerAsset(t, srv, d37rB_AuthorityCssAsset)
	if len(pocCss) == 0 {
		t.Errorf("D37r tranche B: authority-cytoscape-poc.css must remain served")
	}

	graphCss := getExplorerAsset(t, srv, d37rB_AuthorityGraphCss)
	if len(graphCss) == 0 {
		t.Errorf("D37r tranche B: authority-graph.css must remain served")
	}

	overlayCss := getExplorerAsset(t, srv, d37rB_AuthorityOverlayCss)
	if len(overlayCss) == 0 {
		t.Errorf("D37r tranche B: cytoscape-html-overlay.css must remain served")
	}

	// The Authority overlay's defining class must continue to exist
	// (the cytoscape-html-overlay layer + card class names tranche A
	// referenced as the reusable pattern).
	if !strings.Contains(overlayCss, "midas-cy-overlay-card") {
		t.Errorf("D37r tranche B: Authority overlay's `midas-cy-overlay-card` class must remain present")
	}
}

// ── 18. Connector painter remains for fallback ───────────────────

func TestExplorer_ContextCytoscapeStyle_ConnectorPainterStillExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	painter := getExplorerAsset(t, srv, d37rB_ConnectorPainter)

	if !strings.Contains(painter, "paintConnectors") {
		t.Errorf("D37r tranche B: context-connector-painter.js must remain present for fallback (`paintConnectors` symbol)")
	}
	if !strings.Contains(painter, "createElementNS(SVG_NS, 'line')") {
		t.Errorf("D37r tranche B: context-connector-painter.js must remain the SVG-line fallback painter")
	}
}

// ── 19. Tranche A invariants still hold (post-tranche-B'' flips) ─

// D37r-tranche-B'' flip: tranche A's cy instantiation marker
// (`_cy = window.cytoscape({`) and the cy-layout-config marker
// (`layout: { name: 'preset', fit: false },`) both moved out of the
// Context renderer when the shared graph engine module took over cy
// ownership. The invariants below are flipped to assert the new
// shape: Context still owns its lens-specific concerns (node/edge
// builders, camera delegate factory, spatial paint entry, destroy
// hook, stage compose), but the cy-construction markers now live in
// the engine module — the Context renderer's lens-side wiring is the
// `engine.mount(canvas, {…})` call.
func TestExplorer_ContextCytoscapeStyle_TrancheAInvariantsHold(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37rB_RendererAsset)

	// Lens-side markers (preserved in the Context renderer).
	for _, want := range []string{
		"engine.mount(canvas, {",
		"function _renderSpatialCytoscape(",
		"function _buildContextCytoscapeNodes(cards, stage)",
		"function _buildContextCytoscapeEdges(connectors, stage)",
		"function _buildContextCytoscapeCameraDelegate(cy)",
		"function _wireCytoscapeSelectionTap()",
		"function _destroyCytoscape()",
		"bridge.selectCard(card)",
		"graphStage.compose(layout, footprints, safeArea, {})",
		// D37r-tranche-B' replacement for the inline mount: the
		// shared-module-backed entry point (preserved as dead
		// helper in B''; the engine module now hosts the live
		// overlay-mount call).
		"function _mountCytoscapeOverlayViaSharedModule(cards)",
		// Pointer-events policy now declared via the engine's
		// `pointerEvents` option (Context passes it through the
		// engine.mount call site).
		"pointerEvents: 'none',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37r-tranche-B'' (flipped): tranche A invariant must hold (post-flip shape) — %q must remain in source", want)
		}
	}

	// Engine-side markers (moved to the engine module in B'').
	engineJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	for _, want := range []string{
		"cy = window.cytoscape({",
		"layout:               { name: 'preset', fit: false },",
	} {
		if !strings.Contains(engineJS, want) {
			t.Errorf("D37r-tranche-B'' (flipped): tranche A invariant must hold in engine module — %q must remain in graph-cytoscape-engine.js", want)
		}
	}
}
