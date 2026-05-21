package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37o-impl-4 — Context Renderer Visual Parity Foundation tests
//
// Pins the visual-foundation structure for the opt-in strategic
// Context renderer: five-band layout (with cap-proc split), right
// governance column, overflow sentinel rendering, and first-pass
// SVG connector painting via a renderer-local connector painter.
// Asserts preservation of the projection-handoff boundary and the
// model/painter/renderer separation established by D37o-impl-1
// through D37o-impl-3.

const (
	d37oImpl4RendererAsset   = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37oImpl4RendererCSS     = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37oImpl4PainterAsset    = "/explorer/assets/js/graph/context/context-html-card-painter.js"
	d37oImpl4ConnPainter     = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37oImpl4CardModelAsset  = "/explorer/assets/js/graph/context/context-card-model.js"
	d37oImpl4ConnModelAsset  = "/explorer/assets/js/graph/context/context-connector-model.js"
	d37oImpl4LayoutAsset     = "/explorer/assets/js/graph/context/context-layout-model.js"
	d37oImpl4HandoffAsset    = "/explorer/assets/js/graph/context/context-projection-handoff.js"
)

// ── A. Visual foundation asset + scope ───────────────────────────────

func TestExplorer_D37oImpl4_ConnectorPainterServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4ConnPainter)
	if len(js) == 0 {
		t.Fatal("D37o-impl-4: context-connector-painter.js must be served")
	}
}

// TestExplorer_D37oImpl4_FullLoadOrder pins that the connector
// painter slots into the existing load order between the card painter
// and the renderer (and after the connector model).
func TestExplorer_D37oImpl4_FullLoadOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	order := []string{
		"context-card-model.js",
		"context-connector-model.js",
		"context-layout-model.js",
		"context-projection-handoff.js",
		"context-html-card-painter.js",
		"context-connector-painter.js",
		"context-cytoscape-renderer.js",
	}
	last := -1
	for _, asset := range order {
		idx := strings.Index(body, asset)
		if idx < 0 {
			t.Errorf("D37o-impl-4: %q must appear in index.html", asset)
			continue
		}
		if idx <= last {
			t.Errorf("D37o-impl-4: %q must appear AFTER the previous asset in load order", asset)
		}
		last = idx
	}
}

// TestExplorer_D37oImpl4_CssScopedToContext pins that every new rule
// in the renderer stylesheet remains scoped to the canonical
// [data-active-renderer="context"] selector — no leaks under
// alternate identities, Authority, or unscoped global rules.
func TestExplorer_D37oImpl4_CssScopedToContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl4RendererCSS)
	cssExec := stripCSSComments(css)

	prefix := `.midas-graph-viewport[data-active-renderer="context"]`
	// At-rules (e.g. @media) have a different structure; for the
	// purpose of this pin, we strip @media blocks and check their
	// inner rules separately.
	scanned := cssExec
	for {
		idx := strings.Index(scanned, "@media")
		if idx < 0 {
			break
		}
		// Find the opening brace and matching close.
		openIdx := strings.Index(scanned[idx:], "{")
		if openIdx < 0 {
			break
		}
		openIdx += idx
		// Brace counter to find the @media block's close.
		depth := 1
		j := openIdx + 1
		for j < len(scanned) && depth > 0 {
			if scanned[j] == '{' {
				depth++
			} else if scanned[j] == '}' {
				depth--
			}
			j++
		}
		// Walk the inner rules of the @media block: each rule must
		// also be scoped under the renderer prefix.
		inner := scanned[openIdx+1 : j-1]
		_assertCssScoped(t, inner, prefix, "(@media)")
		// Excise the @media block from `scanned` and continue.
		scanned = scanned[:idx] + scanned[j:]
	}
	_assertCssScoped(t, scanned, prefix, "")
}

func _assertCssScoped(t *testing.T, cssExec, prefix, ctx string) {
	t.Helper()
	for i := 0; i < len(cssExec); i++ {
		if cssExec[i] != '{' {
			continue
		}
		start := strings.LastIndexAny(cssExec[:i], "}")
		if start < 0 {
			start = 0
		} else {
			start++
		}
		selector := strings.TrimSpace(cssExec[start:i])
		if selector == "" {
			continue
		}
		if !strings.HasPrefix(selector, prefix) {
			t.Errorf("D37o-impl-4: CSS rule %s must scope under %s — rogue selector %q", ctx, prefix, selector)
		}
	}
}

// ── B. Five-band layout ──────────────────────────────────────────────

// TestExplorer_D37oImpl4_RendersAllFiveBands pins that the renderer
// builds explicit sections for the five locked bands.
func TestExplorer_D37oImpl4_RendersAllFiveBands(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	// The renderer iterates layout.bands and emits one section per
	// band, with the data-band-id attribute set from the band's id.
	if !strings.Contains(js, "section.setAttribute('data-band-id', band.id)") {
		t.Errorf("D37o-impl-4: renderer must emit data-band-id per band section from layout model")
	}
	// The layout model's BAND_IDS list is the source of truth; the
	// renderer must consult layout.bands (not raw projection arrays).
	if !strings.Contains(js, "layout.bands[b]") || !strings.Contains(js, "for (var b = 0; b < layout.bands.length; b++)") {
		t.Errorf("D37o-impl-4: renderer must iterate layout.bands directly")
	}
}

// TestExplorer_D37oImpl4_CapProcSplit pins that the cap-proc band is
// rendered with two visually distinct columns driven by the layout
// model's splitColumns spec.
func TestExplorer_D37oImpl4_CapProcSplit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	for _, want := range []string{
		"band.id === 'cap-proc'",
		"band.splitColumns",
		"'context-renderer-cap-proc-split'",
		"_renderSplitColumn('left',  band.splitColumns.left",
		"_renderSplitColumn('right', band.splitColumns.right",
		"'context-renderer-cap-proc-' + side",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-4: cap-proc split rendering must include %q", want)
		}
	}

	// CSS must define the split + left + right classes.
	css := getExplorerAsset(t, srv, d37oImpl4RendererCSS)
	for _, want := range []string{
		".context-renderer-cap-proc-split",
		".context-renderer-cap-proc-left",
		".context-renderer-cap-proc-right",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37o-impl-4: cap-proc split CSS must include %q", want)
		}
	}
}

// TestExplorer_D37oImpl4_RendererDoesNotReassignKinds pins that the
// renderer does not reassign node kinds to bands itself — that is
// the layout model's job. The renderer simply consumes
// layout.bands[].cards[].cardId.
func TestExplorer_D37oImpl4_RendererDoesNotReassignKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	// If the renderer reassigned kinds it would have to branch on
	// kind strings. Banning those literal branches keeps the layout
	// model authoritative.
	for _, banned := range []string{
		`card.kind === 'capability'`,
		`card.kind === 'process'`,
		`card.kind === 'authority_summary'`,
		`card.kind === 'coverage'`,
		`'related_business_service':  return 'related'`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-4: renderer must NOT reassign node kinds to bands — found %q (layout model owns assignment)", banned)
		}
	}
}

// ── C. Governance column ─────────────────────────────────────────────

// TestExplorer_D37oImpl4_RendersGovernanceColumn pins that the
// governance column is rendered from layout.governanceColumn with
// distinct top + bottom slots.
func TestExplorer_D37oImpl4_RendersGovernanceColumn(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	for _, want := range []string{
		"function _renderGovernance(layout, byId, painter)",
		"layout.governanceColumn.cards",
		"_newGovernanceSlot('top',    'Authority')",
		"_newGovernanceSlot('bottom', 'Coverage')",
		"slot.governancePosition",
		"'context-renderer-governance'",
		"data-governance-position",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-4: governance rendering must include %q", want)
		}
	}
}

// TestExplorer_D37oImpl4_GovernanceCssDistinct pins that the
// governance CSS exists and is distinct from the main band styles.
func TestExplorer_D37oImpl4_GovernanceCssDistinct(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl4RendererCSS)

	for _, want := range []string{
		".context-renderer-governance ",
		".context-renderer-governance-slot",
		`.context-renderer-governance-slot[data-governance-position="top"]`,
		`.context-renderer-governance-slot[data-governance-position="bottom"]`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37o-impl-4: governance CSS must include %q", want)
		}
	}
}

// ── D. Overflow sentinel ─────────────────────────────────────────────

// TestExplorer_D37oImpl4_RendersOverflowSentinel pins that the
// renderer renders overflow sentinel specs from the layout model,
// not from raw projection arrays.
func TestExplorer_D37oImpl4_RendersOverflowSentinel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	for _, want := range []string{
		"function _renderSentinelTile(sentinel)",
		"function _indexSentinelsByBand(layout)",
		"function _appendSentinelsForBand(parentEl, sentinelsByBand, bandId, column)",
		"layout.overflowPolicy.sentinelCards",
		"'context-renderer-sentinel'",
		"sentinel.bandId",
		"sentinel.column",
		"sentinel.layerLabel",
		"'+' + more + ' more'",
		"'_overflow_sentinel'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-4: overflow-sentinel rendering must include %q", want)
		}
	}
}

// TestExplorer_D37oImpl4_SentinelNotRealCardKind pins that the
// sentinel kind is the renderer-only token '_overflow_sentinel' and
// is NEVER added to the card model's NODE_KINDS list.
func TestExplorer_D37oImpl4_SentinelNotRealCardKind(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	cardModel := getExplorerAsset(t, srv, d37oImpl4CardModelAsset)

	if strings.Contains(cardModel, "_overflow_sentinel") {
		t.Errorf("D37o-impl-4: card model must NOT contain '_overflow_sentinel' (renderer-only token)")
	}
	if !strings.Contains(cardModel, "var NODE_KINDS = Object.freeze([") {
		t.Fatal("D37o-impl-4: card model NODE_KINDS list missing")
	}
	// The card model exposes 9 kinds; sentinel is not among them.
	nodeKindsStart := strings.Index(cardModel, "var NODE_KINDS = Object.freeze([")
	nodeKindsEnd := strings.Index(cardModel[nodeKindsStart:], "]);")
	if nodeKindsEnd < 0 {
		t.Fatal("D37o-impl-4: cannot bound NODE_KINDS block")
	}
	block := cardModel[nodeKindsStart : nodeKindsStart+nodeKindsEnd]
	if strings.Contains(block, "overflow") || strings.Contains(block, "sentinel") {
		t.Errorf("D37o-impl-4: NODE_KINDS must NOT mention overflow/sentinel — block:\n%s", block)
	}
}

// TestExplorer_D37oImpl4_SentinelCssPresent pins the sentinel CSS.
func TestExplorer_D37oImpl4_SentinelCssPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl4RendererCSS)

	for _, want := range []string{
		".context-renderer-sentinel ",
		".context-renderer-sentinel-more",
		".context-renderer-sentinel-label",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37o-impl-4: sentinel CSS must include %q", want)
		}
	}
}

// ── E. Connector rendering ───────────────────────────────────────────

// TestExplorer_D37oImpl4_ConnectorPainterPublicSurface pins the
// painter's public namespace and shape.
func TestExplorer_D37oImpl4_ConnectorPainterPublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4ConnPainter)

	declStart := strings.Index(js, "window.MIDASExplorerGraph.contextConnectorPainter = {")
	if declStart < 0 {
		t.Fatal("D37o-impl-4: contextConnectorPainter registration missing")
	}
	declEnd := strings.Index(js[declStart:], "};")
	if declEnd < 0 {
		t.Fatal("D37o-impl-4: cannot bound contextConnectorPainter declaration")
	}
	block := js[declStart : declStart+declEnd]

	for _, want := range []string{
		"paintConnectors: paintConnectors",
		"_constants:",
		"SVG_NS:",
		"CONNECTOR_CLASS:",
		"CONNECTOR_CLASS_PREFIX:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37o-impl-4: connector painter must export %q", want)
		}
	}
}

// TestExplorer_D37oImpl4_ConnectorPainterFiveClasses pins the five
// visual classes via the CSS class hooks emitted by the painter and
// styled by the renderer CSS.
func TestExplorer_D37oImpl4_ConnectorPainterFiveClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// The painter emits class hooks derived from visualClass; the CSS
	// styles all five class-hook variants.
	css := getExplorerAsset(t, srv, d37oImpl4RendererCSS)
	for _, want := range []string{
		".context-connector ",
		".context-connector--service",
		".context-connector--ai_binding",
		".context-connector--authority",
		".context-connector--evidence",
		".context-connector--gap",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37o-impl-4: connector CSS must include %q", want)
		}
	}

	js := getExplorerAsset(t, srv, d37oImpl4ConnPainter)
	if !strings.Contains(js, "CONNECTOR_CLASS_PREFIX + visualClass") {
		t.Errorf("D37o-impl-4: connector painter must derive class hook from visualClass (data-driven)")
	}
}

// TestExplorer_D37oImpl4_ConnectorDashPatterns pins that the painter
// translates the model's dashPattern into stroke-dasharray. The
// authority (6 4) and gap (5 5) patterns come from the connector
// model spec; the painter must apply them faithfully.
func TestExplorer_D37oImpl4_ConnectorDashPatterns(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4ConnPainter)

	// The painter reads dashPattern.{on, off} and emits a
	// stroke-dasharray attribute. We assert the helper logic exists.
	for _, want := range []string{
		"function _dashAttr(dashPattern)",
		"dashPattern.on  === 'number'",
		"dashPattern.off === 'number'",
		"return dashPattern.on + ' ' + dashPattern.off;",
		"line.setAttribute('stroke-dasharray', dash);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-4: connector painter must include dash-pattern handling %q", want)
		}
	}

	// The connector model emits the locked dash patterns. Cross-check
	// here so a future model change makes the dash-pattern contract
	// visible.
	connModel := getExplorerAsset(t, srv, d37oImpl4ConnModelAsset)
	for _, want := range []string{
		"DASH_6_4    = Object.freeze({ on: 6, off: 4 });",
		"DASH_5_5    = Object.freeze({ on: 5, off: 5 });",
		"DASH_6_4,   dir: 'directed'",
		"DASH_5_5, dir: 'directed'",
	} {
		if !strings.Contains(connModel, want) {
			t.Errorf("D37o-impl-4: connector model must keep dash-pattern constants %q (cross-check)", want)
		}
	}
}

// TestExplorer_D37oImpl4_ConnectorsRenderBehindCards pins that the
// SVG connector layer paints behind cards. The renderer creates the
// SVG BEFORE the main + governance children; the CSS gives the
// layer z-index 0 while main + governance get z-index 1.
func TestExplorer_D37oImpl4_ConnectorsRenderBehindCards(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	// DOM-order: SVG appended first, then main, then governance.
	canvasIdx := strings.Index(js, "canvas.className = 'context-renderer-canvas';")
	if canvasIdx < 0 {
		t.Fatal("D37o-impl-4: canvas element creation missing")
	}
	svgAppend  := strings.Index(js[canvasIdx:], "canvas.appendChild(svg)")
	mainAppend := strings.Index(js[canvasIdx:], "canvas.appendChild(main)")
	govAppend  := strings.Index(js[canvasIdx:], "canvas.appendChild(governance)")
	if svgAppend < 0 || mainAppend < 0 || govAppend < 0 {
		t.Fatal("D37o-impl-4: canvas must append svg, main, governance in order")
	}
	if !(svgAppend < mainAppend && mainAppend < govAppend) {
		t.Errorf("D37o-impl-4: canvas children must be appended in order svg → main → governance (got positions %d, %d, %d)", svgAppend, mainAppend, govAppend)
	}

	css := getExplorerAsset(t, srv, d37oImpl4RendererCSS)
	if !strings.Contains(css, ".context-renderer-connectors {") {
		t.Errorf("D37o-impl-4: CSS must define .context-renderer-connectors rule")
	}
	// Connector layer must be z-index 0 (or unset, but explicitly 0
	// is the contract).
	connBlock := _extractCssRuleBlock(t, css, `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-connectors {`)
	if !strings.Contains(connBlock, "z-index: 0") {
		t.Errorf("D37o-impl-4: connector layer must declare z-index: 0 — block:\n%s", connBlock)
	}
	if !strings.Contains(connBlock, "pointer-events: none") {
		t.Errorf("D37o-impl-4: connector layer must be pointer-events: none")
	}

	// Main and governance must declare z-index: 1 to paint above the
	// connector layer.
	mainBlock := _extractCssRuleBlock(t, css, `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-main {`)
	if !strings.Contains(mainBlock, "z-index: 1") {
		t.Errorf("D37o-impl-4: .context-renderer-main must declare z-index: 1 to paint above connector layer")
	}
	govBlock := _extractCssRuleBlock(t, css, `.midas-graph-viewport[data-active-renderer="context"] .context-renderer-governance {`)
	if !strings.Contains(govBlock, "z-index: 1") {
		t.Errorf("D37o-impl-4: .context-renderer-governance must declare z-index: 1 to paint above connector layer")
	}
}

func _extractCssRuleBlock(t *testing.T, css, openingSignature string) string {
	t.Helper()
	idx := strings.Index(css, openingSignature)
	if idx < 0 {
		t.Fatalf("D37o-impl-4: CSS rule with opening %q missing", openingSignature)
	}
	end := strings.Index(css[idx:], "}")
	if end < 0 {
		t.Fatalf("D37o-impl-4: cannot bound CSS rule starting at %q", openingSignature)
	}
	return css[idx : idx+end]
}

// TestExplorer_D37oImpl4_ConnectorPainterRendererIndependent pins
// that the painter contains no projection / model / drawer / tray /
// graph-engine coupling. It receives connector specs + a card-lookup
// callback; it knows nothing else.
func TestExplorer_D37oImpl4_ConnectorPainterRendererIndependent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4ConnPainter)

	for _, banned := range []string{
		"contextProjection",
		"getCurrentProjection",
		"buildConnectorsFromProjection",
		"buildCardsFromProjection",
		"buildLayout",
		"cytoscape",
		"Cytoscape",
		"setName(",
		"setFields(",
		"setGovernance(",
		"setActions(",
		"setInlineActions(",
		"gmap-canvas",
		"gmap-svg",
		"gmap-scene",
		"addNode",
		"context-cytoscape-overlay-spike",
		"viewport.register",
		"activateById",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-4: connector painter must NOT contain %q", banned)
		}
	}
}

// TestExplorer_D37oImpl4_RendererPlumbsConnectorPainter pins that
// the renderer invokes the connector painter with a card-element
// lookup callback (driven by the renderer's own card index, NOT by
// projection inspection).
func TestExplorer_D37oImpl4_RendererPlumbsConnectorPainter(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	for _, want := range []string{
		"g.contextConnectorPainter",
		"painter.paintConnectors(svgEl, connectors, {",
		"containerEl: canvasEl",
		"getCardElement: function (cardId) { return _cardEls[cardId] || null; }",
		"function _trackCardEl(el, card)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-4: renderer must plumb the connector painter via %q", want)
		}
	}
}

// ── F. Model/painter/renderer boundary preserved ─────────────────────

// TestExplorer_D37oImpl4_PainterDoesNotPerformLayout pins that the
// card painter contains no layout / band / sentinel / governance
// logic. Layout is the renderer's responsibility.
func TestExplorer_D37oImpl4_PainterDoesNotPerformLayout(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4PainterAsset)

	for _, banned := range []string{
		"layout.bands",
		"layout.governanceColumn",
		"overflowPolicy",
		"sentinelCards",
		"context-renderer-band",
		"context-renderer-main",
		"context-renderer-governance",
		"context-renderer-canvas",
		"context-renderer-connectors",
		"context-renderer-sentinel",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-4: card painter must NOT contain layout/band hook %q", banned)
		}
	}
}

// TestExplorer_D37oImpl4_RendererDoesNotRebuildModels pins that the
// renderer does not duplicate model constants (badge classes,
// connector visual classes, band ids).
func TestExplorer_D37oImpl4_RendererDoesNotRebuildModels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	for _, banned := range []string{
		"var NODE_KINDS",
		"var EDGE_KINDS",
		"var VISUAL_CLASSES",
		"var BAND_IDS",
		"var BADGE_CLASSES",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-4: renderer must NOT redefine model constant %q", banned)
		}
	}
}

// TestExplorer_D37oImpl4_LayoutModelStillRendererIndependent pins
// that the layout model still does NOT reference any DOM / renderer
// / connector-painter symbol.
func TestExplorer_D37oImpl4_LayoutModelStillRendererIndependent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4LayoutAsset)

	for _, banned := range []string{
		"document.",
		"querySelector",
		"createElement",
		"cytoscape",
		"contextRenderer",
		"contextCardPainter",
		"contextConnectorPainter",
		"contextProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-4: layout model must remain renderer-independent — found %q", banned)
		}
	}
}

// ── G. Projection handoff preservation ───────────────────────────────

func TestExplorer_D37oImpl4_HandoffStillTheOnlyProjectionSource(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl4RendererAsset)

	if !strings.Contains(js, "g.contextProjection.getCurrentProjection()") {
		t.Errorf("D37o-impl-4: renderer must continue to read through contextProjection.getCurrentProjection()")
	}
	if !strings.Contains(js, "g.contextProjection.subscribe(") {
		t.Errorf("D37o-impl-4: renderer must continue to subscribe through contextProjection.subscribe(...)")
	}

	for _, banned := range []string{
		"_lastContextProjection",
		"_lastProjection",
		"g._lastContextProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-4: renderer must NOT reintroduce private legacy projection state %q", banned)
		}
	}

	// No backend fetch in renderer / painter / connector painter / handoff.
	for _, asset := range []string{
		d37oImpl4RendererAsset,
		d37oImpl4PainterAsset,
		d37oImpl4ConnPainter,
		d37oImpl4HandoffAsset,
	} {
		body := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"fetch(",
			"XMLHttpRequest",
			"/v1/graphs/context",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D37o-impl-4: %s must NOT perform a backend fetch — found %q", asset, banned)
			}
		}
	}
}

// ── H. Guardrails ────────────────────────────────────────────────────

func TestExplorer_D37oImpl4_NoDurableTemporaryNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37oImpl4RendererAsset,
		d37oImpl4RendererCSS,
		d37oImpl4ConnPainter,
		d37oImpl4PainterAsset,
	} {
		body := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"context-v2",
			"context-strategic",
			"new-context",
			"context-new",
			"context-next",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D37o-impl-4: %s must NOT contain temporary renderer name %q", asset, banned)
			}
		}
	}
}

// TestExplorer_D37oImpl4_NoForbiddenCoupling pins that the renderer
// and connector painter do not call legacy graph-renderer primitives
// or drawer setters or reference the dormant spike.
func TestExplorer_D37oImpl4_NoForbiddenCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37oImpl4RendererAsset,
		d37oImpl4ConnPainter,
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"context-cytoscape-overlay-spike",
			"#gmap-canvas",
			"#gmap-svg",
			"#gmap-scene",
			"gmap-canvas",
			"gmap-svg",
			"gmap-scene",
			"addNode",
			"addConnector",
			"lensAgnosticConnectorPath",
			"setName(",
			"setFields(",
			"setGovernance(",
			"setActions(",
			"setInlineActions(",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37o-impl-4: %s must NOT contain forbidden coupling %q", asset, banned)
			}
		}
	}
}

// ── I. Foundation preservation ───────────────────────────────────────

func TestExplorer_D37oImpl4_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-details"`,
		`gmap-right-rail`,
		`id="gmap-rail-panel-inspector"`,
		`id="gmap-evidence-tray"`,
		`id="gmap-evidence-tray-panel"`,
		`data-tab="drift"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-4: foundation markup %q must remain", want)
		}
	}

	for _, asset := range []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			t.Errorf("D37o-impl-4: %s must remain served", asset)
		}
	}

	// Legacy Context entry point remains. D37p-clean-1 retired the
	// dead `renderer.register('context', lensImpl)` call; the live
	// entry point is the `contextView` export. The inspector
	// dispatcher namespace is separate and out of scope for
	// D37p-clean-1.
	view := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(view, "window.MIDASExplorerGraph.contextView") {
		t.Errorf("D37o-impl-4: legacy context-graph-view.js must still expose contextView entry points")
	}
	// D37p-clean-2 retired the dead inspector dispatcher.
	if strings.Contains(view, "MIDASExplorerGraph.inspector.register('context', inspectorImpl)") {
		t.Errorf("D37p-clean-2: dead inspector.register('context', inspectorImpl) call must be removed from context-graph-view.js")
	}

	// Renderer identity unchanged.
	rendererJS := getExplorerAsset(t, srv, d37oImpl4RendererAsset)
	for _, want := range []string{
		`var RENDERER_ID    = 'context';`,
		`var QUERY_PARAM    = 'contextRenderer';`,
		`var MODE_STRATEGIC = 'strategic';`,
		`g.viewport.register(RENDERER_ID, _factoryFor())`,
		`g.viewport.activateById(RENDERER_ID)`,
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("D37o-impl-4: renderer identity contract regressed — missing %q", want)
		}
	}
}
