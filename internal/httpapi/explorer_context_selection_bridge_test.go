package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37o-impl-5 — Context Selection, Action, Reframe, and Drawer Bridge tests
//
// Pins the bridge module + renderer wiring that turns strategic-renderer
// card clicks into the existing Context surfaces (right drawer
// inspector frame, bottom evidence tray notification, action
// dispatcher). The bridge is the ONLY new module allowed to call
// drawer setters directly; the renderer + painters remain free of
// drawer coupling.

const (
	d37oImpl5BridgeAsset       = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37oImpl5RendererAsset     = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37oImpl5PainterAsset      = "/explorer/assets/js/graph/context/context-html-card-painter.js"
	d37oImpl5ConnPainterAsset  = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37oImpl5RendererCSSAsset  = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37oImpl5HandoffAsset      = "/explorer/assets/js/graph/context/context-projection-handoff.js"
	d37oImpl5LegacyView        = "/explorer/assets/js/graph/context/context-graph-view.js"
)

// ── A. Asset presence + load order ───────────────────────────────────

func TestExplorer_D37oImpl5_BridgeAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)
	if len(js) == 0 {
		t.Fatal("D37o-impl-5: context-selection-bridge.js must be served")
	}
}

// TestExplorer_D37oImpl5_FullLoadOrder pins the bridge's place in the
// load order: after every other Context module (handoff / painter /
// connector painter), before the renderer.
func TestExplorer_D37oImpl5_FullLoadOrder(t *testing.T) {
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
		"context-selection-bridge.js",
		"context-cytoscape-renderer.js",
	}
	last := -1
	for _, asset := range order {
		idx := strings.Index(body, asset)
		if idx < 0 {
			t.Errorf("D37o-impl-5: %q must appear in index.html", asset)
			continue
		}
		if idx <= last {
			t.Errorf("D37o-impl-5: %q must appear AFTER the previous asset in load order", asset)
		}
		last = idx
	}
}

// ── B. Bridge public surface ─────────────────────────────────────────

func TestExplorer_D37oImpl5_BridgePublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	declStart := strings.Index(js, "window.MIDASExplorerGraph.contextSelectionBridge = {")
	if declStart < 0 {
		t.Fatal("D37o-impl-5: contextSelectionBridge registration missing")
	}
	declEnd := strings.Index(js[declStart:], "};")
	if declEnd < 0 {
		t.Fatal("D37o-impl-5: cannot bound contextSelectionBridge declaration")
	}
	block := js[declStart : declStart+declEnd]

	for _, want := range []string{
		"selectCard:",
		"clearSelection:",
		"getSelected:",
		"subscribe:",
		"handleAction:",
		"legacyActionsFromCard:",
		"toLegacyActionShape:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37o-impl-5: bridge must expose %q", want)
		}
	}
}

// TestExplorer_D37oImpl5_BridgeNamingDurable pins that the bridge
// uses the canonical durable name `contextSelectionBridge` and
// carries no rollout-mode words.
func TestExplorer_D37oImpl5_BridgeNamingDurable(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
		"strategicSelectionBridge",
		"contextStrategicBridge",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-5: bridge must NOT contain temporary identity %q", banned)
		}
	}
}

// ── C. Drawer bridge ─────────────────────────────────────────────────

// TestExplorer_D37oImpl5_BridgeUsesInspectorFrame pins that the
// bridge consumes ContextCard data and calls the existing
// MIDASExplorerGraph.inspector frame setters. This is the
// architecturally-isolated fallback that the brief explicitly allows
// when no card-shaped inspector adapter exists yet. The bridge file
// is the ONLY allowed location for these setter calls.
func TestExplorer_D37oImpl5_BridgeUsesInspectorFrame(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.inspector",
		"insp.setName(name)",
		"insp.setFields(rows)",
		"insp.setGovernance(html)",
		"insp.setActions(",
		"card.details",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: bridge must populate the drawer via %q", want)
		}
	}
}

// TestExplorer_D37oImpl5_BridgeUsesContextGovernanceHelper pins that
// the bridge re-uses the legacy `contextInspector.buildFailModePolicySection`
// helper rather than re-implementing FMP three-state HTML.
func TestExplorer_D37oImpl5_BridgeUsesContextGovernanceHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	if !strings.Contains(js, "ctxIns.buildFailModePolicySection") {
		t.Errorf("D37o-impl-5: bridge must reuse contextInspector.buildFailModePolicySection (no duplicate Governance markup)")
	}
}

// TestExplorer_D37oImpl5_RendererDoesNotCallDrawerSetters pins that
// the renderer continues to contain NO direct calls to drawer
// setters. The bridge owns those calls; the renderer just produces
// model-level selection events.
func TestExplorer_D37oImpl5_RendererDoesNotCallDrawerSetters(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5RendererAsset)

	for _, banned := range []string{
		"setName(",
		"setFields(",
		"setGovernance(",
		"setActions(",
		"setInlineActions(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-5: renderer must NOT call drawer setter %q (bridge owns drawer coupling)", banned)
		}
	}
}

// TestExplorer_D37oImpl5_PainterDoesNotCallDrawerSetters pins the
// same guarantee for the card painter and the connector painter.
func TestExplorer_D37oImpl5_PainterDoesNotCallDrawerSetters(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, asset := range []string{
		d37oImpl5PainterAsset,
		d37oImpl5ConnPainterAsset,
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"setName(",
			"setFields(",
			"setGovernance(",
			"setActions(",
			"setInlineActions(",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37o-impl-5: %s must NOT call drawer setter %q", asset, banned)
			}
		}
	}
}

// TestExplorer_D37oImpl5_BridgeFallbackDocumented pins that the
// bridge file carries a comment block explaining WHY direct drawer
// setter calls live here, so a future tranche knows where to retire
// the fallback when a clean inspector adapter exists.
func TestExplorer_D37oImpl5_BridgeFallbackDocumented(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	for _, want := range []string{
		"Why a bridge",
		"silently no-op",
		"collapses into a one-line delegation",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: bridge must document fallback rationale (missing phrase %q)", want)
		}
	}
}

// ── D. Evidence tray bridge ──────────────────────────────────────────

func TestExplorer_D37oImpl5_BridgeNotifiesEvidenceTray(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.contextEvidenceTray",
		"tray.notifySelectionChanged()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: bridge must notify evidence tray via %q", want)
		}
	}

	// Renderer + painters must NOT touch the tray directly.
	for _, asset := range []string{
		d37oImpl5RendererAsset,
		d37oImpl5PainterAsset,
		d37oImpl5ConnPainterAsset,
	} {
		body := getExplorerAsset(t, srv, asset)
		if strings.Contains(body, "contextEvidenceTray") {
			t.Errorf("D37o-impl-5: %s must NOT reference contextEvidenceTray (bridge owns the notification)", asset)
		}
	}
}

// ── E. Existing selection state integration ──────────────────────────

func TestExplorer_D37oImpl5_BridgeUsesGlobalSelectionState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	if !strings.Contains(js, "window.MIDASExplorerGraph.selection") {
		t.Errorf("D37o-impl-5: bridge must update shared selection state via MIDASExplorerGraph.selection")
	}
	if !strings.Contains(js, "sel.setSelected(card.id)") {
		t.Errorf("D37o-impl-5: bridge must call selection.setSelected(card.id)")
	}
}

// ── F. Action handling ───────────────────────────────────────────────

// TestExplorer_D37oImpl5_BridgeTranslatesActionShape pins that the
// bridge converts the camelCase ContextCard ActionDescriptor into
// the legacy snake_case wire shape expected by
// `MIDASExplorerGraph._actionDispatcher` /
// `handleGovernanceMapAction`.
func TestExplorer_D37oImpl5_BridgeTranslatesActionShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	for _, want := range []string{
		"function toLegacyActionShape(action)",
		"target_id:   action.targetId",
		"target_view: action.targetView",
		"window.MIDASExplorerGraph._actionDispatcher",
		"dispatch(toLegacyActionShape(action))",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: bridge must translate ActionDescriptor via %q", want)
		}
	}
}

// TestExplorer_D37oImpl5_BridgeHandlesAllThreeActionKinds pins that
// the bridge's translation preserves all three action kinds named
// in the brief (no whitelist filtering at the bridge; the legacy
// dispatcher already enforces the whitelist via
// handleGovernanceMapAction). Source-level pin: the bridge's
// translation logic is kind-agnostic.
func TestExplorer_D37oImpl5_BridgeHandlesAllThreeActionKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	for _, want := range []string{
		"reframe-around-this",
		"view-business-service-record",
		"view-capability-record",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: bridge documentation / behaviour must reference action kind %q", want)
		}
	}
}

// TestExplorer_D37oImpl5_BridgeDoesNotFetchBackend pins that the
// bridge introduces no new backend fetch. Reframe routes through
// the existing legacy `_actionDispatcher` → legacy lens fetch →
// projection-handoff republish path.
func TestExplorer_D37oImpl5_BridgeDoesNotFetchBackend(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/graphs/context",
		"graphs.context(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-5: bridge must NOT perform a backend fetch — found %q", banned)
		}
	}
}

// ── G. Renderer click delegation ─────────────────────────────────────

// TestExplorer_D37oImpl5_RendererWiresMountClick pins the renderer's
// click + keyboard delegation on the renderer-owned mount.
func TestExplorer_D37oImpl5_RendererWiresMountClick(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5RendererAsset)

	for _, want := range []string{
		"function _wireMountInteraction()",
		"function _unwireMountInteraction()",
		"function _onMountClick(ev)",
		"function _onMountKeydown(ev)",
		"function _closestWithAttribute(el, attr)",
		"_mountEl.addEventListener('click',   _mountClickHandler)",
		"_mountEl.addEventListener('keydown', _mountKeydownHandler)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: renderer must wire mount interaction via %q", want)
		}
	}
}

// TestExplorer_D37oImpl5_RendererStopsActionPropagation pins that
// the renderer's click handler stops propagation on action clicks
// so they don't accidentally trigger card selection.
func TestExplorer_D37oImpl5_RendererStopsActionPropagation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5RendererAsset)

	// The action-click branch must call stopPropagation.
	bodyStart := strings.Index(js, "function _onMountClick(ev)")
	if bodyStart < 0 {
		t.Fatal("D37o-impl-5: _onMountClick missing")
	}
	bodyEnd := strings.Index(js[bodyStart:], "\n  }\n")
	if bodyEnd < 0 {
		t.Fatal("D37o-impl-5: cannot bound _onMountClick body")
	}
	body := js[bodyStart : bodyStart+bodyEnd]
	if !strings.Contains(body, "ev.stopPropagation()") {
		t.Errorf("D37o-impl-5: action click branch must call stopPropagation — body:\n%s", body)
	}
}

// TestExplorer_D37oImpl5_RendererDelegatesToBridge pins that the
// renderer routes both card clicks and action clicks through the
// bridge — no inline selection logic in the renderer.
func TestExplorer_D37oImpl5_RendererDelegatesToBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5RendererAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.contextSelectionBridge",
		"bridge.selectCard(card)",
		"bridge.handleAction(action)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: renderer must delegate to bridge via %q", want)
		}
	}
}

// TestExplorer_D37oImpl5_RendererHandlesMissingBridgeGracefully pins
// the defensive guards: every bridge call site checks the bridge
// surface before invoking.
func TestExplorer_D37oImpl5_RendererHandlesMissingBridgeGracefully(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5RendererAsset)

	// At every call site the renderer guards on typeof X === 'function'.
	for _, want := range []string{
		"typeof bridge.handleAction !== 'function'",
		"typeof bridge.selectCard !== 'function'",
		"typeof g.contextSelectionBridge.subscribe !== 'function'",
		"typeof g.contextSelectionBridge.getSelected !== 'function'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: renderer must defensively guard bridge calls via %q", want)
		}
	}
}

// ── H. Visual selected state ─────────────────────────────────────────

func TestExplorer_D37oImpl5_RendererAppliesSelectedClass(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5RendererAsset)

	for _, want := range []string{
		"function _applySelectionVisual(selectedId)",
		"el.classList.add('is-selected')",
		"el.classList.remove('is-selected')",
		`el.setAttribute('aria-current', 'true')`,
		`el.removeAttribute('aria-current')`,
		"function _reapplySelectionAfterPaint()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: renderer must manage selected visual state via %q", want)
		}
	}

	// And subscribe to the bridge so it can mirror state on every
	// selectCard call.
	if !strings.Contains(js, "g.contextSelectionBridge.subscribe(function (card)") {
		t.Errorf("D37o-impl-5: renderer must subscribe to the bridge to apply selection visuals")
	}
}

// TestExplorer_D37oImpl5_SelectedCssRulePresent pins the selected /
// hover / focus CSS so the visual state has a styled presence.
func TestExplorer_D37oImpl5_SelectedCssRulePresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl5RendererCSSAsset)

	for _, want := range []string{
		".context-card.is-selected",
		".context-card:hover",
		".context-card:focus-visible",
		".context-card-action:focus-visible",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37o-impl-5: renderer CSS must define selected/focus rule %q", want)
		}
	}
}

// ── I. Painter exposes click target attributes ───────────────────────

func TestExplorer_D37oImpl5_PainterEmitsInteractiveAttributes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5PainterAsset)

	for _, want := range []string{
		`el.setAttribute('role', 'button')`,
		`el.setAttribute('tabindex', '0')`,
		`li.setAttribute('data-action-kind',        _str(a.kind))`,
		`li.setAttribute('data-action-target-id',   _str(a.targetId))`,
		`li.setAttribute('data-action-target-view', _str(a.targetView))`,
		`li.setAttribute('data-action-label',       _str(a.label))`,
		`li.setAttribute('role', 'button')`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-5: painter must emit interactive attribute %q", want)
		}
	}
}

// ── J. Projection handoff preservation ───────────────────────────────

func TestExplorer_D37oImpl5_HandoffStillTheOnlyProjectionSource(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5RendererAsset)

	if !strings.Contains(js, "g.contextProjection.getCurrentProjection()") {
		t.Errorf("D37o-impl-5: renderer must continue to read through contextProjection.getCurrentProjection()")
	}
	if !strings.Contains(js, "g.contextProjection.subscribe(") {
		t.Errorf("D37o-impl-5: renderer must continue to subscribe through contextProjection.subscribe(...)")
	}

	for _, banned := range []string{
		"_lastContextProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-5: renderer must NOT reintroduce private legacy state %q", banned)
		}
	}
}

// ── K. Bridge isolation from legacy renderer DOM ────────────────────

func TestExplorer_D37oImpl5_BridgeDoesNotScrapeLegacyDom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl5BridgeAsset)

	for _, banned := range []string{
		"#gmap-canvas",
		"#gmap-svg",
		"#gmap-scene",
		"gmap-canvas",
		"gmap-svg",
		"gmap-scene",
		"querySelector",
		"getElementById",
		"createElement",
		"addNode",
		"addConnector",
		"lensAgnosticConnectorPath",
		"context-cytoscape-overlay-spike",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-5: bridge must NOT contain legacy-DOM token %q", banned)
		}
	}
}

// ── L. Card model action descriptors carry targetView ───────────────

// TestExplorer_D37oImpl5_CardModelActionsCarryTargetView pins that
// every card kind that emits a `reframe-around-this` action also
// emits the `targetView` field — the bridge's translation depends
// on it for the legacy `target_view` wire field.
func TestExplorer_D37oImpl5_CardModelActionsCarryTargetView(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-card-model.js")

	// Each reframe-action emission site should include a targetView.
	reframeIdx := 0
	for {
		idx := strings.Index(js[reframeIdx:], "'reframe-around-this'")
		if idx < 0 {
			break
		}
		// Inspect a 320-char window around the occurrence and require
		// `targetView` to appear in it.
		start := reframeIdx + idx
		end := start + 320
		if end > len(js) {
			end = len(js)
		}
		windowStr := js[start:end]
		if !strings.Contains(windowStr, "targetView") {
			t.Errorf("D37o-impl-5: every reframe-around-this action emission must carry targetView — window:\n%s", windowStr)
		}
		reframeIdx = start + len("'reframe-around-this'")
	}
}

// ── M. Foundation preservation ──────────────────────────────────────

func TestExplorer_D37oImpl5_FoundationPreserved(t *testing.T) {
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
			t.Errorf("D37o-impl-5: foundation markup %q must remain", want)
		}
	}

	// Legacy renderer / inspector / tray / spike modules still served.
	for _, asset := range []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-evidence-tray.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			t.Errorf("D37o-impl-5: %s must remain served", asset)
		}
	}

	// Legacy Context entry point unchanged. D37p-clean-1 retired the
	// dead `renderer.register('context', lensImpl)` dispatcher call
	// and the unreachable `_publishToProjectionHandoff` helper; the
	// live legacy entry point is the `contextView` export. The
	// inspector dispatcher namespace remains alive and unchanged.
	view := getExplorerAsset(t, srv, d37oImpl5LegacyView)
	if !strings.Contains(view, "window.MIDASExplorerGraph.contextView") {
		t.Errorf("D37o-impl-5: legacy context-graph-view.js must still expose contextView entry points")
	}
	// D37p-clean-2 retired the dead inspector dispatcher.
	if strings.Contains(view, "MIDASExplorerGraph.inspector.register('context', inspectorImpl)") {
		t.Errorf("D37p-clean-2: dead inspector.register('context', inspectorImpl) call must be removed from context-graph-view.js")
	}

	// Renderer identity unchanged.
	rendererJS := getExplorerAsset(t, srv, d37oImpl5RendererAsset)
	for _, want := range []string{
		`var RENDERER_ID    = 'context';`,
		`var QUERY_PARAM    = 'contextRenderer';`,
		`var MODE_STRATEGIC = 'strategic';`,
		`g.viewport.register(RENDERER_ID, _factoryFor())`,
		`g.viewport.activateById(RENDERER_ID)`,
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("D37o-impl-5: renderer identity contract regressed — missing %q", want)
		}
	}
}

// TestExplorer_D37oImpl5_NoDurableTemporaryNamesAcrossArtefacts pins
// the no-temporary-names rule across every artefact this tranche
// touches.
func TestExplorer_D37oImpl5_NoDurableTemporaryNamesAcrossArtefacts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37oImpl5BridgeAsset,
		d37oImpl5RendererAsset,
		d37oImpl5PainterAsset,
		d37oImpl5ConnPainterAsset,
		d37oImpl5RendererCSSAsset,
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
				t.Errorf("D37o-impl-5: %s must NOT contain temporary renderer name %q", asset, banned)
			}
		}
	}
}
