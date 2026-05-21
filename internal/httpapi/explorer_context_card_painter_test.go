package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37o-impl-3 — Context HTML Card Painter and Projection Handoff
//
// Asset-text and structural tests pinning:
//
//   • The projection-handoff boundary
//     (/explorer/assets/js/graph/context/context-projection-handoff.js)
//     with the public surface contextProjection.{publishProjection,
//     getCurrentProjection, getLastMeta, subscribe, clear}.
//
//   • The HTML card painter
//     (/explorer/assets/js/graph/context/context-html-card-painter.js)
//     consuming ContextCard model objects and emitting renderer-owned
//     DOM with the full card-slot vocabulary.
//
//   • The strategic Context renderer reading the projection through
//     the handoff (not legacy private globals), invoking the painter
//     to render cards grouped by layout band, subscribing to
//     projection updates on mount, and unsubscribing on destroy.
//
//   • The legacy Context lens publishing the projection to the
//     handoff at a clean boundary, without altering legacy
//     rendering and without depending on the strategic activation
//     mode.
//
//   • Naming + coupling guardrails preserved (no temporary renderer
//     names, no legacy primitives, no drawer / tray / spike
//     coupling, no Cytoscape inside model / painter / handoff).

const (
	d37oImpl3HandoffAsset   = "/explorer/assets/js/graph/context/context-projection-handoff.js"
	d37oImpl3PainterAsset   = "/explorer/assets/js/graph/context/context-html-card-painter.js"
	d37oImpl3RendererAsset  = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37oImpl3RendererCSS    = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37oImpl3LegacyView     = "/explorer/assets/js/graph/context/context-graph-view.js"
)

// ── A. Asset presence and load order ─────────────────────────────────

func TestExplorer_D37oImpl3_HandoffAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3HandoffAsset)
	if len(js) == 0 {
		t.Fatal("D37o-impl-3: context-projection-handoff.js must be served")
	}
}

func TestExplorer_D37oImpl3_PainterAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3PainterAsset)
	if len(js) == 0 {
		t.Fatal("D37o-impl-3: context-html-card-painter.js must be served")
	}
}

// TestExplorer_D37oImpl3_FullLoadOrder pins the strict load order in
// index.html:
//   1. context-card-model.js
//   2. context-connector-model.js
//   3. context-layout-model.js
//   4. context-projection-handoff.js
//   5. context-html-card-painter.js
//   6. context-cytoscape-renderer.js
func TestExplorer_D37oImpl3_FullLoadOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	order := []string{
		"context-card-model.js",
		"context-connector-model.js",
		"context-layout-model.js",
		"context-projection-handoff.js",
		"context-html-card-painter.js",
		"context-cytoscape-renderer.js",
	}
	last := -1
	for _, asset := range order {
		idx := strings.Index(body, asset)
		if idx < 0 {
			t.Errorf("D37o-impl-3: %q must appear in index.html", asset)
			continue
		}
		if idx <= last {
			t.Errorf("D37o-impl-3: %q must appear AFTER previous asset in load order", asset)
		}
		last = idx
	}
}

// TestExplorer_D37oImpl3_CssRemainsScopedToContext pins that the CSS
// extensions added in this tranche remain scoped under the canonical
// active-renderer selector.
func TestExplorer_D37oImpl3_CssRemainsScopedToContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl3RendererCSS)
	cssExec := stripCSSComments(css)

	prefix := `.midas-graph-viewport[data-active-renderer="context"]`
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
			t.Errorf("D37o-impl-3: CSS must remain scoped under %s — rogue %q", prefix, selector)
		}
	}
}

// ── B. Projection handoff ────────────────────────────────────────────

func TestExplorer_D37oImpl3_HandoffPublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3HandoffAsset)

	declStart := strings.Index(js, "window.MIDASExplorerGraph.contextProjection = {")
	if declStart < 0 {
		t.Fatal("D37o-impl-3: contextProjection public surface registration missing")
	}
	declEnd := strings.Index(js[declStart:], "};")
	if declEnd < 0 {
		t.Fatal("D37o-impl-3: cannot bound contextProjection declaration")
	}
	block := js[declStart : declStart+declEnd]

	for _, want := range []string{
		"publishProjection:",
		"getCurrentProjection:",
		"getLastMeta:",
		"subscribe:",
		"clear:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37o-impl-3: contextProjection must expose %q", want)
		}
	}
}

// TestExplorer_D37oImpl3_HandoffIsRendererIndependent pins that the
// handoff carries NO references to DOM APIs, graph engines, drawer
// setters, evidence-tray hooks, legacy renderer DOM ids, or the
// dormant overlay-spike module.
func TestExplorer_D37oImpl3_HandoffIsRendererIndependent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3HandoffAsset)

	for _, banned := range []string{
		"document.",
		"querySelector",
		"getElementById",
		"createElement",
		"cytoscape",
		"Cytoscape",
		"viewport.register",
		"activateById",
		"setName",
		"setFields",
		"setGovernance",
		"setActions",
		"setInlineActions",
		"gmap-canvas",
		"gmap-svg",
		"gmap-scene",
		"addNode",
		"addConnector",
		"lensAgnosticConnectorPath",
		"context-cytoscape-overlay-spike",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-3: handoff must NOT contain forbidden symbol %q (renderer-independence)", banned)
		}
	}
}

// TestExplorer_D37oImpl3_HandoffDoesNotMutateProjection pins that the
// handoff stores references to projection / meta but never mutates
// them. The publish path assigns to module-private variables; it does
// NOT call Array methods like splice/push on the projection itself.
func TestExplorer_D37oImpl3_HandoffDoesNotMutateProjection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3HandoffAsset)

	publishStart := strings.Index(js, "function publishProjection(projection, meta)")
	if publishStart < 0 {
		t.Fatal("D37o-impl-3: publishProjection function missing")
	}
	publishEnd := strings.Index(js[publishStart:], "\n  }\n")
	if publishEnd < 0 {
		t.Fatal("D37o-impl-3: cannot bound publishProjection body")
	}
	body := js[publishStart : publishStart+publishEnd]

	for _, banned := range []string{
		"projection.nodes.push",
		"projection.edges.push",
		"projection.nodes.splice",
		"projection.edges.splice",
		"projection.nodes =",
		"projection.edges =",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37o-impl-3: publishProjection must NOT mutate the projection payload — found %q", banned)
		}
	}
}

// TestExplorer_D37oImpl3_ProjectionProviderPublishesProjection
// retargets the original D37o-impl-3 "legacy view publishes" test
// onto the live producer after D37p-clean-1.
//
// Pre-D37p-clean-1, the legacy `context-graph-view.js` defined
// `_publishToProjectionHandoff(payload, ctx)` and fired it from
// inside `lensImpl.render` (with `source: 'legacy-context-lens'`).
// That code path had zero call-sites at runtime — the dead
// `MIDASExplorerGraph.renderer.render(...)` dispatcher never fired —
// so it never published. D37o-fix-1 (`contextProjectionProvider`)
// is the live producer; it acquires via the context adapter and
// publishes through `contextProjection.publishProjection(payload,
// meta)`. D37p-clean-1 retired the legacy helper and dispatcher
// entirely.
//
// This test now pins:
//   • the dead helper is gone from the legacy view;
//   • the live provider is the producer that publishes through
//     `contextProjection.publishProjection(...)`.
// The legacy view's prior `'legacy-context-lens'` source tag is
// obsolete; the live provider carries its own `PROVIDER_ID` meta.
// handoff at a clean boundary. The publish call:
//   • lives inside a helper guarded by a defensive availability check,
//   • is invoked BEFORE legacy rendering (so consumers can observe
//     projections even when legacy rendering errors later),
//   • carries an explicit `source: 'legacy-context-lens'` meta tag,
//   • does NOT branch on activation mode (the legacy view does not
//     know about strategic vs legacy rollout).
func TestExplorer_D37oImpl3_ProjectionProviderPublishesProjection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Dead helper definition + call must be gone from the legacy view.
	// (Explanatory comments referencing the historical name are OK;
	// the assertion targets the executable substrings.)
	legacy := getExplorerAsset(t, srv, d37oImpl3LegacyView)
	if regexp.MustCompile(`function\s+_publishToProjectionHandoff\s*\(`).MatchString(legacy) {
		t.Errorf("D37p-clean-1: dead _publishToProjectionHandoff function must be removed from context-graph-view.js")
	}
	if strings.Contains(legacy, "_publishToProjectionHandoff(payload, ctx)") {
		t.Errorf("D37p-clean-1: dead _publishToProjectionHandoff call must be removed from context-graph-view.js")
	}
	if strings.Contains(legacy, "source: 'legacy-context-lens'") {
		t.Errorf("D37p-clean-1: dead 'legacy-context-lens' source tag must be removed from context-graph-view.js")
	}

	// Live producer publishes through the handoff.
	provider := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-projection-provider.js")
	if !strings.Contains(provider, "h.publishProjection(") {
		t.Errorf("D37p-clean-1: live contextProjectionProvider must publish through contextProjection.publishProjection(...)")
	}
	if !strings.Contains(provider, "PROVIDER_ID") {
		t.Errorf("D37p-clean-1: live contextProjectionProvider must carry a PROVIDER_ID meta")
	}
}

// TestExplorer_D37oImpl3_LegacyContextEntryPointUnchanged pins the
// live legacy Context lens entry points after D37p-clean-1 retired
// the dead `renderer.register('context', lensImpl)` dispatcher path.
// The inspector dispatcher namespace is separate and still alive.
func TestExplorer_D37oImpl3_LegacyContextEntryPointUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3LegacyView)

	// D37p-clean-2 retired the dead inspector dispatcher; the live
	// Context inspector content is still driven via the lens-agnostic
	// frame setters on `MIDASExplorerGraph.inspector.set*`.
	for _, want := range []string{
		"window.MIDASExplorerGraph.contextView",
		"function renderContextGraph(data, ctx)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-3: legacy view must retain %q", want)
		}
	}
}

// ── C. Card painter ──────────────────────────────────────────────────

func TestExplorer_D37oImpl3_PainterPublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3PainterAsset)

	declStart := strings.Index(js, "window.MIDASExplorerGraph.contextCardPainter = {")
	if declStart < 0 {
		t.Fatal("D37o-impl-3: contextCardPainter registration missing")
	}
	declEnd := strings.Index(js[declStart:], "};")
	if declEnd < 0 {
		t.Fatal("D37o-impl-3: cannot bound contextCardPainter declaration")
	}
	block := js[declStart : declStart+declEnd]

	for _, want := range []string{
		"renderCard:",
		"renderCardBody:",
		"renderBadge:",
		"renderMetric:",
		"_constants:",
		"CARD_CLASS:",
		"KIND_CLASS_PREFIX:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37o-impl-3: painter must expose %q", want)
		}
	}
}

// TestExplorer_D37oImpl3_PainterEmitsCardSlotVocabulary pins that the
// painter source contains every locked DOM class hook for the card
// slot vocabulary (label / name / subtitle / meta / badges / metrics /
// actions).
func TestExplorer_D37oImpl3_PainterEmitsCardSlotVocabulary(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3PainterAsset)

	for _, want := range []string{
		"'context-card-header'",
		"'context-card-eyebrow'",
		"'context-card-name'",
		"'context-card-subtitle'",
		"'context-card-meta'",
		"'context-card-meta-row'",
		"'context-card-badges'",
		"'context-card-badge'",
		"'context-card-metrics'",
		"'context-card-metric'",
		"'context-card-actions'",
		"'context-card-action'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-3: painter must emit DOM class %s", want)
		}
	}
}

// TestExplorer_D37oImpl3_PainterPerKindClassHook pins the per-kind
// class hook prefix and shows that the painter applies it from the
// card's kind field (driven by data, not hard-coded per kind).
func TestExplorer_D37oImpl3_PainterPerKindClassHook(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3PainterAsset)

	if !strings.Contains(js, `var KIND_CLASS_PREFIX  = 'context-card--';`) {
		t.Errorf("D37o-impl-3: painter must define KIND_CLASS_PREFIX = 'context-card--'")
	}
	if !strings.Contains(js, "KIND_CLASS_PREFIX + _str(card.kind)") {
		t.Errorf("D37o-impl-3: painter must derive per-kind class from card.kind, not hard-coded per-kind branches")
	}
}

// TestExplorer_D37oImpl3_PainterAriaSupport pins accessibility:
// every card gets data-kind / data-card-id and an aria-label.
func TestExplorer_D37oImpl3_PainterAriaSupport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3PainterAsset)

	for _, want := range []string{
		`el.setAttribute('data-card-id', _str(card.id))`,
		`el.setAttribute('data-kind', _str(card.kind))`,
		`el.setAttribute('aria-label', _str(card.ariaLabel))`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-3: painter must include %q for accessibility / instrumentation", want)
		}
	}
}

// TestExplorer_D37oImpl3_PainterActionsAreDisplayOnly pins that
// actions are rendered as inert DOM (no event listeners, no
// selection/reframe routing) in this tranche.
func TestExplorer_D37oImpl3_PainterActionsAreDisplayOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3PainterAsset)

	for _, banned := range []string{
		"addEventListener('click'",
		"addEventListener(\"click\"",
		"onclick =",
		"onclick=",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-3: painter must NOT attach event listeners (actions are display-only) — found %q", banned)
		}
	}
}

// TestExplorer_D37oImpl3_PainterRendererIndependent pins that the
// painter carries no graph-engine, drawer, tray, or legacy DOM
// coupling, and no model-rebuilding.
func TestExplorer_D37oImpl3_PainterRendererIndependent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3PainterAsset)

	for _, banned := range []string{
		"cytoscape",
		"Cytoscape",
		"viewport.register",
		"activateById",
		"setName(",
		"setFields(",
		"setGovernance(",
		"setActions(",
		"setInlineActions(",
		"gmap-canvas",
		"gmap-svg",
		"gmap-scene",
		"addNode",
		"addConnector",
		"lensAgnosticConnectorPath",
		"context-cytoscape-overlay-spike",
		"buildCardsFromProjection",
		"buildConnectorsFromProjection",
		"buildLayout",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-3: painter must NOT contain %q", banned)
		}
	}
}

// ── D. Renderer integration ──────────────────────────────────────────

// TestExplorer_D37oImpl3_RendererReadsProjectionViaHandoff pins that
// the renderer reads the current projection EXCLUSIVELY through the
// projection handoff — never through any private legacy global such
// as `_lastContextProjection`.
func TestExplorer_D37oImpl3_RendererReadsProjectionViaHandoff(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3RendererAsset)

	// Positive: the renderer reads through getCurrentProjection.
	if !strings.Contains(js, "g.contextProjection.getCurrentProjection()") {
		t.Errorf("D37o-impl-3: renderer must call contextProjection.getCurrentProjection() to source the projection")
	}
	// Negative: the renderer must NOT consult any private legacy
	// projection global.
	for _, banned := range []string{
		"_lastContextProjection",
		"_lastProjection",
		"_currentProjection",
		"g._lastContextProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-3: renderer must NOT read private legacy projection state %q", banned)
		}
	}
}

// TestExplorer_D37oImpl3_RendererConsumesCardPainter pins that the
// renderer paints cards through the card painter rather than
// building DOM inline.
func TestExplorer_D37oImpl3_RendererConsumesCardPainter(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3RendererAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.contextCardPainter",
		"painter.renderCard(card, null)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-3: renderer must consume painter via %q", want)
		}
	}
}

// TestExplorer_D37oImpl3_RendererSubscribesAndUnsubscribes pins the
// mount/destroy symmetry around the projection subscription.
func TestExplorer_D37oImpl3_RendererSubscribesAndUnsubscribes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3RendererAsset)

	// Mount path subscribes via _subscribeToProjection.
	if !strings.Contains(js, "function _subscribeToProjection()") {
		t.Errorf("D37o-impl-3: renderer must define _subscribeToProjection")
	}
	if !strings.Contains(js, "g.contextProjection.subscribe(") {
		t.Errorf("D37o-impl-3: renderer must call contextProjection.subscribe(...)")
	}
	// Destroy path unsubscribes.
	if !strings.Contains(js, "function _unsubscribeFromProjection()") {
		t.Errorf("D37o-impl-3: renderer must define _unsubscribeFromProjection")
	}
	// Bound the destroy function and assert it calls the unsubscribe
	// helper.
	destroyIdx := strings.Index(js, "function _destroy()")
	if destroyIdx < 0 {
		t.Fatal("D37o-impl-3: _destroy function missing")
	}
	destroyEnd := strings.Index(js[destroyIdx:], "\n  }\n")
	if destroyEnd < 0 {
		t.Fatal("D37o-impl-3: cannot bound _destroy body")
	}
	destroyBody := js[destroyIdx : destroyIdx+destroyEnd]
	if !strings.Contains(destroyBody, "_unsubscribeFromProjection()") {
		t.Errorf("D37o-impl-3: _destroy must call _unsubscribeFromProjection() — body:\n%s", destroyBody)
	}
}

// TestExplorer_D37oImpl3_RendererEmptyState pins that the renderer
// preserves an explicit empty state when no projection has been
// published.
func TestExplorer_D37oImpl3_RendererEmptyState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3RendererAsset)

	if !strings.Contains(js, "_appendEmptyState(") {
		t.Errorf("D37o-impl-3: renderer must keep an explicit empty-state branch")
	}
	if !strings.Contains(js, "'Awaiting Context projection.") {
		t.Errorf("D37o-impl-3: renderer must preserve the awaiting-projection empty-state copy")
	}
}

// TestExplorer_D37oImpl3_RendererIdentityUnchanged pins that the
// renderer id remains the canonical 'context' and the activation
// mode remains 'strategic'.
func TestExplorer_D37oImpl3_RendererIdentityUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3RendererAsset)

	for _, want := range []string{
		`var RENDERER_ID    = 'context';`,
		`var QUERY_PARAM    = 'contextRenderer';`,
		`var MODE_STRATEGIC = 'strategic';`,
		`g.viewport.register(RENDERER_ID, _factoryFor())`,
		`g.viewport.activateById(RENDERER_ID)`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-3: renderer identity contract regressed — missing %q", want)
		}
	}
}

// ── E. Services-based boundary ───────────────────────────────────────

// TestExplorer_D37oImpl3_NoBackendFetchInThisTranche pins that this
// tranche does NOT introduce a service-backed projection client. The
// renderer remains a consumer of the handoff; the handoff is
// populated externally (today: legacy lens; tomorrow: a service
// provider replaceable without touching any of these files).
func TestExplorer_D37oImpl3_NoBackendFetchInThisTranche(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37oImpl3HandoffAsset,
		d37oImpl3PainterAsset,
		d37oImpl3RendererAsset,
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"fetch(",
			"XMLHttpRequest",
			"/v1/graphs/context",
			"graphs.context(",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37o-impl-3: %s must NOT perform a backend fetch in this tranche — found %q", asset, banned)
			}
		}
	}
}

// TestExplorer_D37oImpl3_PainterDoesNotKnowProjectionSource pins that
// the painter receives only the card spec — it does NOT see meta /
// source / view fields from the projection handoff.
func TestExplorer_D37oImpl3_PainterDoesNotKnowProjectionSource(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl3PainterAsset)

	for _, banned := range []string{
		"contextProjection",
		"getCurrentProjection",
		"getLastMeta",
		"subscribe",
		"meta.source",
		"projection.nodes",
		"projection.edges",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-3: painter must NOT inspect projection source — found %q", banned)
		}
	}
}

// TestExplorer_D37oImpl3_ModelsIndependentOfProjectionSource pins
// that the model modules do NOT import the projection handoff.
// Models receive a projection by parameter; they are agnostic to
// where it came from.
func TestExplorer_D37oImpl3_ModelsIndependentOfProjectionSource(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		"/explorer/assets/js/graph/context/context-card-model.js",
		"/explorer/assets/js/graph/context/context-connector-model.js",
		"/explorer/assets/js/graph/context/context-layout-model.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"contextProjection",
			"getCurrentProjection",
			"contextCardPainter",
			"contextRenderer",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37o-impl-3: model %s must remain independent of projection source / painter / renderer — found %q", asset, banned)
			}
		}
	}
}

// ── F. Naming + coupling guardrails ──────────────────────────────────

func TestExplorer_D37oImpl3_NoDurableTemporaryNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37oImpl3HandoffAsset,
		d37oImpl3PainterAsset,
		d37oImpl3RendererAsset,
		d37oImpl3RendererCSS,
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
				t.Errorf("D37o-impl-3: %s must NOT contain temporary renderer name %q", asset, banned)
			}
		}
	}
}

// ── G. Foundation preservation ───────────────────────────────────────

func TestExplorer_D37oImpl3_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Right drawer + bottom evidence tray markup intact.
	for _, want := range []string{
		`id="gmap-details"`,
		`gmap-right-rail`,
		`id="gmap-rail-panel-inspector"`,
		`id="gmap-evidence-tray"`,
		`id="gmap-evidence-tray-panel"`,
		`data-tab="drift"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-3: foundation markup %q must remain", want)
		}
	}

	// Authority + viewport host modules still served.
	for _, asset := range []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			t.Errorf("D37o-impl-3: %s must remain served", asset)
		}
	}

	// Dormant spike identity preserved (`context-cytoscape` is its
	// renderer id; orthogonal to the canonical `context`).
	spike := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	if !strings.Contains(spike, "context-cytoscape") {
		t.Errorf("D37o-impl-3: dormant spike identity context-cytoscape must remain")
	}
}
