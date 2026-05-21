package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35e-port-context-cytoscape-to-graphviewport
//
// Ports the Context Cytoscape HTML-card renderer onto the MIDAS
// GraphViewport host as the second real alternative renderer:
//
//   • Context activation routes through the GraphViewport host in
//     the public `install()` boundary. Pre-D35g this was
//     `viewport.activate('context-cytoscape', factory)`. D35g
//     introduced the renderer registry, so the module now
//     registers its factory once at module init via
//     `viewport.register('context-cytoscape', factory)` and
//     `install()` activates via
//     `viewport.activateById('context-cytoscape')`. The underlying
//     invariant (Context routes through the host) is unchanged.
//   • The renderer factory's `mount(slotEl, ctx)` creates
//     `.context-cy-spike-mount` INSIDE `.midas-graph-renderer-slot`
//     (NOT directly inside `.governance-map-canvas-scroll` as it
//     did pre-D35e).
//   • Context uses `ctx.getSafeArea()` for fit-padding composition
//     in the settle's `cy.fit(...)` call.
//   • Context subscribes to `ctx.onResize(...)` for resize handling
//     (callback triggers layer-tier resync via `_syncLayerBound`).
//   • Destroy unsubscribes resize, calls cy.destroy, removes only
//     Context-owned DOM. Routed through `viewport.deactivate()`.
//
// STRATEGIC CLIP CONTRACT (D35e):
//   `.context-cy-spike-overlay { overflow: hidden }` is REMOVED.
//   The overlay is a projection layer, not a clip authority. The
//   `.context-cy-spike-mount` retains `overflow: hidden` for
//   Cytoscape canvas-discipline, and the `.midas-graph-viewport`
//   is documented (D35e) and will be enforced (D35f) as the
//   strategic clip authority.
//
//   This is the architectural fix for the user-reported
//   disappearing-card symptom: pre-D35e the overlay's own
//   `overflow: hidden` clipped cards in pre-transform space
//   (overlay's untransformed cy-mount-sized box), so cards at
//   model-x > mount.width vanished before the `scale(cy.zoom)`
//   projected them into the visible viewport. Removing that
//   overlay-level clip lets the overlay's `scale(zoom)` project
//   cards correctly; the mount (and eventually the viewport)
//   clips at the actual visible boundary.
//
// Transitional debt (D35e does NOT remove):
//   • `body.context-cy-spike-active` body class is still added by
//     `install()` (transitional debt — drives the
//     `#gmap-canvas { display:none !important }` CSS hide).
//     Scheduled for D35f.
//   • `body.context-cy-spike-active #gmap-canvas { display:none
//     !important }` CSS rule remains. D35f task.
//   • Transitional legacy fallback in `install()` (no host →
//     append to `.governance-map-canvas-scroll` directly).
//     Scheduled for D35f removal.

const d35eContextAssetPath = "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js"

// TestExplorer_D35eContext_ActivatesThroughViewportHost pins that
// the spike's public `install()` routes through the GraphViewport
// host. As of D35g, activation is via the registry: the module
// registers `_contextCytoscapeRendererFactory` under the id
// `'context-cytoscape'` and `install()` calls
// `viewport.activateById('context-cytoscape')`.
func TestExplorer_D35eContext_ActivatesThroughViewportHost(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	// D35g — Activation is now by registered renderer id.
	if !strings.Contains(js, "vp.activateById('context-cytoscape')") {
		t.Error("D35e/D35g: install() must call vp.activateById('context-cytoscape')")
	}
	// D35g — The factory must be registered with the host.
	if !strings.Contains(js, "vp.register('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35e/D35g: install() must register its factory via vp.register('context-cytoscape', _contextCytoscapeRendererFactory)")
	}
	// D35g — Direct factory activation is retired.
	if strings.Contains(js, "vp.activate('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35g: install() must NOT pass the factory directly to vp.activate — use register + activateById")
	}
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport",
		"typeof vp.activateById === 'function'",
		"typeof vp.getActiveRendererId === 'function'",
		"typeof vp.getRendererSlotEl === 'function'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35e/D35g: host probe must include %q (defensive)", want)
		}
	}
}

// TestExplorer_D35eContext_RendererFactoryLifecycle pins the
// factory contract: `mount(slotEl, ctx)` returning a handle with
// `destroy()`, exposed on the public surface.
func TestExplorer_D35eContext_RendererFactoryLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	for _, want := range []string{
		"var _contextCytoscapeRendererFactory = {",
		"mount: function (slotEl, ctx) {",
		"destroy: function () {",
		// Exposed for tests + future host-driven paths.
		"_rendererFactory:    _contextCytoscapeRendererFactory",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35e: factory contract must include %q", want)
		}
	}
}

// TestExplorer_D35eContext_MountsInsideRendererSlot pins that the
// factory creates `.context-cy-spike-mount` inside `slotEl` (the
// host-supplied `.midas-graph-renderer-slot`).
//
// The strategic install path delegates to `_installResources(slotEl)`
// where `parentEl.appendChild(_mountEl)` lands inside the slot.
func TestExplorer_D35eContext_MountsInsideRendererSlot(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	// Factory delegates to _installResources(slotEl).
	factStart := strings.Index(js, "var _contextCytoscapeRendererFactory = {")
	if factStart < 0 {
		t.Fatal("D35e: factory definition not found")
	}
	factEnd := strings.Index(js[factStart:], "\n  };\n")
	if factEnd < 0 {
		t.Fatal("D35e: cannot bound factory definition")
	}
	factBody := js[factStart : factStart+factEnd]
	if !strings.Contains(factBody, "_installResources(slotEl)") {
		t.Error("D35e: factory.mount must delegate to _installResources(slotEl)")
	}

	// _installResources uses parentEl.appendChild for the mount.
	irStart := strings.Index(js, "function _installResources(parentEl)")
	if irStart < 0 {
		t.Fatal("D35e: _installResources definition not found")
	}
	irEnd := strings.Index(js[irStart:], "\n  }\n")
	if irEnd < 0 {
		t.Fatal("D35e: cannot bound _installResources body")
	}
	irBody := js[irStart : irStart+irEnd]
	for _, want := range []string{
		`_mountEl.className = 'context-cy-spike-mount';`,
		"parentEl.appendChild(_mountEl)",
	} {
		if !strings.Contains(irBody, want) {
			t.Errorf("D35e: _installResources must include %q", want)
		}
	}
	// Negative — the factory mount itself must NOT manually pick
	// `.governance-map-canvas-scroll` as parent. The strategic
	// parent is `slotEl` supplied by the host.
	if strings.Contains(factBody, "governance-map-canvas-scroll") {
		t.Error("D35e: factory body must NOT reference `.governance-map-canvas-scroll` (host supplies slotEl)")
	}
}

// TestExplorer_D35eContext_DoesNotAppendToLegacyScrollSurface
// pins that the strategic mount path goes through the host.
//
// D35f-retire-transitional-renderer-debt — the legacy fallback
// (which used `.governance-map-canvas-scroll` as a mount parent
// when the host was unavailable) is RETIRED. install() now ONLY
// goes through the host; if the host fails, install fails safely.
func TestExplorer_D35eContext_DoesNotAppendToLegacyScrollSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	// Public install body — host-routed path required.
	installStart := strings.Index(js, "function install(options) {")
	if installStart < 0 {
		t.Fatal("D35f: install definition not found")
	}
	installEnd := strings.Index(js[installStart:], "\n  }\n")
	if installEnd < 0 {
		t.Fatal("D35f: cannot bound install body")
	}
	installBody := js[installStart : installStart+installEnd]

	// D35g — install() activates via the registry id, not by passing
	// the factory inline.
	if !strings.Contains(installBody, "vp.activateById('context-cytoscape')") {
		t.Error("D35f/D35g: install must include the host-routed activation path (vp.activateById)")
	}
	if strings.Contains(installBody, "vp.activate('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35g: install must NOT pass the factory directly to vp.activate — use vp.activateById")
	}
	// D35f: legacy fallback RETIRED.
	if strings.Contains(installBody, "getElementsByClassName('governance-map-canvas-scroll')") {
		t.Error("D35f: install must NOT contain legacy `.governance-map-canvas-scroll` fallback (retired in D35f)")
	}
	// Fail-safe on host absence.
	if !strings.Contains(installBody, "'host-unavailable'") {
		t.Error("D35f: install must fail with 'host-unavailable' reason when host activation fails")
	}
}

// TestExplorer_D35eContext_UsesSafeAreaForFit pins that the
// settle's `cy.fit(...)` composes the host's `ctx.getSafeArea()`
// with the legacy MIDAS fit padding.
func TestExplorer_D35eContext_UsesSafeAreaForFit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	for _, want := range []string{
		// Composition logic exists.
		"_rendererCtx.getSafeArea()",
		"hostMax > fitPadding",
		// Floor is the existing MIDAS layout constant.
		"_midasFitPadding()",
		// Composition uses Math.max.
		"Math.max(sa.top",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35e: safe-area composition must include %q", want)
		}
	}
}

// TestExplorer_D35eContext_UsesViewportResizeSubscription pins
// that the factory subscribes to `ctx.onResize(...)` and that the
// local ResizeObserver + window-resize path is skipped when the
// host owns activation.
func TestExplorer_D35eContext_UsesViewportResizeSubscription(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	// Factory subscribes via ctx.onResize and stores unsubscribe.
	factStart := strings.Index(js, "var _contextCytoscapeRendererFactory = {")
	if factStart < 0 {
		t.Fatal("D35e: factory definition not found")
	}
	factEnd := strings.Index(js[factStart:], "\n  };\n")
	if factEnd < 0 {
		t.Fatal("D35e: cannot bound factory definition")
	}
	factBody := js[factStart : factStart+factEnd]
	for _, want := range []string{
		"typeof ctx.onResize === 'function'",
		"_rendererResizeUnsub = ctx.onResize(_onHostResize)",
		"if (_rendererResizeUnsub) _rendererResizeUnsub()",
		"_rendererResizeUnsub = null;",
	} {
		if !strings.Contains(factBody, want) {
			t.Errorf("D35e: factory resize wiring must include %q", want)
		}
	}

	// `_onHostResize` triggers the layer-tier sync (D34i two-tier
	// model preserved — pan/zoom updates layer transform only).
	hrStart := strings.Index(js, "function _onHostResize()")
	if hrStart < 0 {
		t.Fatal("D35e: _onHostResize handler not found")
	}
	hrEnd := strings.Index(js[hrStart:], "\n  }")
	if hrEnd < 0 {
		t.Fatal("D35e: cannot bound _onHostResize body")
	}
	hrBody := js[hrStart : hrStart+hrEnd]
	if !strings.Contains(hrBody, "_syncLayerBound()") {
		t.Error("D35e: _onHostResize must call _syncLayerBound() (D34i layer-tier resync)")
	}

	// Local ResizeObserver + window.resize MUST be skipped when
	// _rendererCtx is set (host owns resize).
	if !strings.Contains(js, "if (!_rendererCtx) {") {
		t.Error("D35e: local ResizeObserver path must be guarded by `if (!_rendererCtx)` so the host owns resize")
	}
}

// TestExplorer_D35eContext_PreservesD34iTwoTierTransform pins
// that D34i's two-tier transform math is preserved verbatim by
// D35e. The factory mount + install must NOT alter `_syncLayer`,
// `_syncCards`, transform string shapes, or the projection model.
func TestExplorer_D35eContext_PreservesD34iTwoTierTransform(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	for _, want := range []string{
		// Layer-tier transform shape.
		"function _syncLayer()",
		"_cy.pan()",
		"_cy.zoom()",
		"'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')'",
		"transformOrigin = 'top left'",
		// Card-tier transform shape.
		"function _syncCards()",
		"n.position()",
		"'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)'",
		// Projection model identifier.
		"'layer-pan-zoom-card-model-position'",
		// Per-tier event constants survive.
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35e: D34i two-tier transform regressed — missing %q", want)
		}
	}
}

// TestExplorer_D35eContext_OverlayDoesNotOwnClipping pins the
// strategic CSS change: `.context-cy-spike-overlay` no longer has
// `overflow: hidden`. The overlay is a projection layer; the
// viewport (eventually) and the cy mount (today) own clipping.
func TestExplorer_D35eContext_OverlayDoesNotOwnClipping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	// Find the overlay rule and assert overflow:hidden is GONE.
	// D35f re-keyed from body-class to host-owned renderer identity.
	idx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] .context-cy-spike-overlay {`)
	if idx < 0 {
		t.Fatal("D35e/D35f: overlay CSS rule missing")
	}
	end := strings.Index(css[idx:], "\n}")
	if end < 0 {
		t.Fatal("D35e: cannot bound overlay CSS rule")
	}
	block := css[idx : idx+end]
	if strings.Contains(block, "overflow: hidden") {
		t.Errorf("D35e: overlay rule must NOT declare `overflow: hidden` (strategic fix for disappearing-card symptom); body was:\n%s", block)
	}
	// Positive — D34i transform-origin still required for the
	// projection model.
	if !strings.Contains(block, "transform-origin: top left") {
		t.Error("D35e: overlay rule must keep `transform-origin: top left` (D34i projection model)")
	}
}

// TestExplorer_D35eContext_NoNewRendererLevelClippingHack pins
// that D35e did NOT replace overlay-level clipping with another
// tactical patch. The mount's pre-existing `overflow: hidden` is
// preserved (Cytoscape container discipline), but no NEW renderer-
// level overflow / clip-path rule appears in the spike CSS.
func TestExplorer_D35eContext_NoNewRendererLevelClippingHack(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	exec := stripCSSComments(css)

	// Pre-D35e count of `overflow: hidden` in executable CSS was
	// 2 (mount + overlay). D35e removes 1 (overlay), leaving 1
	// (mount). Any value other than 1 means a new clip rule has
	// been added (>1) or the mount's discipline was accidentally
	// removed (<1).
	count := strings.Count(exec, "overflow: hidden")
	if count != 1 {
		t.Errorf("D35e: spike CSS must declare `overflow: hidden` exactly once (on `.context-cy-spike-mount` for cy canvas discipline); got %d occurrences", count)
	}

	// No `clip-path` or `clip` rules introduced as substitutes.
	for _, banned := range []string{
		"clip-path:",
		"clip:",
		"contain: layout",
		"contain: paint",
		"contain: layout paint",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D35e: spike CSS must NOT introduce %q as a substitute clip authority", banned)
		}
	}
}

// TestExplorer_D35eContext_DestroyTearsDownOwnedDomOnly pins the
// destroy contract: factory.destroy unsubscribes + delegates to
// `_teardownResources` (which clears cy + removes only the
// Context-owned mount + clears refs). Public `destroy()` routes
// through `viewport.deactivate()`.
func TestExplorer_D35eContext_DestroyTearsDownOwnedDomOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	// Factory.destroy unsubscribes + delegates.
	factStart := strings.Index(js, "var _contextCytoscapeRendererFactory = {")
	if factStart < 0 {
		t.Fatal("D35e: factory definition not found")
	}
	factEnd := strings.Index(js[factStart:], "\n  };\n")
	if factEnd < 0 {
		t.Fatal("D35e: cannot bound factory definition")
	}
	factBody := js[factStart : factStart+factEnd]
	for _, want := range []string{
		"destroy: function () {",
		"if (_rendererResizeUnsub) _rendererResizeUnsub()",
		"_teardownResources();",
		"_rendererCtx = null;",
	} {
		if !strings.Contains(factBody, want) {
			t.Errorf("D35e: factory destroy must include %q", want)
		}
	}

	// _teardownResources removes only Context-owned DOM.
	trStart := strings.Index(js, "function _teardownResources()")
	if trStart < 0 {
		t.Fatal("D35e: _teardownResources definition not found")
	}
	trEnd := strings.Index(js[trStart:], "\n  }\n")
	if trEnd < 0 {
		t.Fatal("D35e: cannot bound _teardownResources body")
	}
	trBody := js[trStart : trStart+trEnd]
	for _, want := range []string{
		"_cy.destroy()",
		"_mountEl.parentNode.removeChild(_mountEl)",
		"_mountEl     = null;",
		"_overlayEl   = null;",
		"_cardsByKey  = null;",
	} {
		if !strings.Contains(trBody, want) {
			t.Errorf("D35e: _teardownResources must include %q", want)
		}
	}
	// Negative — teardown must NOT remove native DOM.
	for _, banned := range []string{
		"getElementById('gmap-canvas').remove",
		"getElementById('gmap-scene').remove",
		"getElementById('gmap-svg').remove",
	} {
		if strings.Contains(trBody, banned) {
			t.Errorf("D35e: _teardownResources must NOT remove native DOM — found %q", banned)
		}
	}

	// Public destroy routes through viewport.deactivate when host
	// owns activation.
	deStart := strings.Index(js, "function destroy() {")
	if deStart < 0 {
		t.Fatal("D35e: destroy definition not found")
	}
	deEnd := strings.Index(js[deStart:], "\n  }\n")
	if deEnd < 0 {
		t.Fatal("D35e: cannot bound destroy body")
	}
	deBody := js[deStart : deStart+deEnd]
	for _, want := range []string{
		`vp.getActiveRendererId() === 'context-cytoscape'`,
		"vp.deactivate();",
		"_teardownResources();",
	} {
		if !strings.Contains(deBody, want) {
			t.Errorf("D35e: destroy() must include %q", want)
		}
	}
	// D35f — body-class removal RETIRED. Renderer identity is
	// host-owned via data-active-renderer.
	if strings.Contains(deBody, "classList.remove(BODY_FLAG_CLASS)") {
		t.Error("D35f: destroy() must NOT flip body class (host owns renderer identity)")
	}
}

// TestExplorer_D35eContext_DoesNotDeleteNativeGraphDom pins that
// the migration does NOT introduce any code that removes the
// native graph DOM tokens.
func TestExplorer_D35eContext_DoesNotDeleteNativeGraphDom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35eContextAssetPath)

	for _, banned := range []string{
		`document.getElementById('gmap-canvas').remove`,
		`document.getElementById("gmap-canvas").remove`,
		`document.getElementById('gmap-scene').remove`,
		`document.getElementById('gmap-svg').remove`,
		// innerHTML wipes that would clear the slot's existing
		// native canvas-scroll wrapper.
		"slotEl.innerHTML",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D35e: Context Cytoscape must NOT touch native graph DOM — found %q", banned)
		}
	}

	// Native DOM tokens still in index.html.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`id="gmap-canvas"`,
		`id="gmap-scene"`,
		`id="gmap-svg"`,
		`class="governance-map-canvas-scroll"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35e: native graph DOM token %q must remain in index.html", want)
		}
	}
}

// TestExplorer_D35eAuthority_D35dContractPreserved pins that
// D35d's Authority migration remains intact. Authority still
// activates through the host (now via the D35g registry rather
// than direct factory activation) and its mount-in-slot contract
// is unchanged.
func TestExplorer_D35eAuthority_D35dContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// D35g — register + activateById replaces direct vp.activate.
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
		"var _authorityRendererFactory = {",
		"slotEl.appendChild(_mountEl)",
		"_rendererCtx.getSafeArea",
		"ctx.onResize(_refitWithSafeArea)",
		"vp.deactivate",
	} {
		if !strings.Contains(authJS, want) {
			t.Errorf("D35e/D35g: D35d Authority contract %q must remain", want)
		}
	}
}

// TestExplorer_D35eNative_D35cBaselinePreserved pins that D35c's
// native-context baseline registration in the host remains.
func TestExplorer_D35eNative_D35cBaselinePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	for _, want := range []string{
		"function adoptExisting(",
		"function _adoptNativeContextBaseline()",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35e: D35c baseline registration %q must remain", want)
		}
	}
}

// TestExplorer_D35e_NoNewTacticalActivationOrHidingHack pins that
// D35e introduces no new body class, no new #gmap-canvas hide
// rule, no new overflow hack, no new clip-path rule. Strategic
// module discipline.
func TestExplorer_D35e_NoNewTacticalActivationOrHidingHack(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Spike JS — D35f retired ALL body-class flips. No
	// `document.body.classList.add(BODY_FLAG_CLASS)` call should
	// remain in executable code.
	js := getExplorerAsset(t, srv, d35eContextAssetPath)
	exec := stripJSComments(js)
	if strings.Contains(exec, "document.body.classList.add(BODY_FLAG_CLASS)") {
		t.Error("D35f: spike must NOT add body class (host owns renderer identity via data-active-renderer)")
	}
	for _, banned := range []string{
		"d35e-active",
		"d35f-active",
		"context-cytoscape-active",
		"graph-renderer-active",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D35e/D35f: must NOT introduce new body-class activation flag — found %q", banned)
		}
	}

	// Spike CSS — no NEW hide / overflow rules. Count only the
	// EXECUTABLE CSS; documentation comments legitimately reference
	// `display: none` when explaining what other rules do.
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	execCSS := stripCSSComments(css)
	hideCount := strings.Count(execCSS, "display: none !important")
	if hideCount != 1 {
		t.Errorf("D35e: expected exactly 1 pre-existing `display: none !important` in executable spike CSS (the #gmap-canvas hide); got %d", hideCount)
	}

	// And no clip-path / new contain rules introduced. Reuses
	// `execCSS` declared above.
	for _, banned := range []string{
		"clip-path:",
		"clip:",
	} {
		if strings.Contains(execCSS, banned) {
			t.Errorf("D35e: spike CSS must NOT introduce %q (no new clipping hack)", banned)
		}
	}
}

// TestExplorer_D35e_D35aD35bD35cD35dContractsPreserved pins all
// foundation contracts from D35a–D35d remain valid.
func TestExplorer_D35e_D35aD35bD35cD35dContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a — structural DOM.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35e: D35a structural class %q must remain in index.html", want)
		}
	}

	// D35b — host API.
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport = {",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
		"function getSafeArea(",
		"function onResize(",
		"function adoptExisting(",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35e: D35b/D35c API %q must remain", want)
		}
	}

	// D35d — Authority migration (D35g register + activateById).
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	if !strings.Contains(authJS, "vp.activateById('authority')") {
		t.Error("D35e/D35g: D35d Authority host-routed activation must remain (now via vp.activateById)")
	}
	if !strings.Contains(authJS, "vp.register('authority', _authorityRendererFactory)") {
		t.Error("D35e/D35g: D35d Authority renderer must be registered with the host")
	}

	// Authority body-class survives as transitional debt.
	authCSS := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	if !strings.Contains(authCSS, "body.cytoscape-poc-active") {
		t.Error("D35e: Authority body class must survive (transitional debt, scheduled for D35f)")
	}

	// Context body-class also survives as transitional debt.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if !strings.Contains(spikeCSS, "body.context-cy-spike-active") {
		t.Error("D35e: Context body class must survive (transitional debt, scheduled for D35f)")
	}
}
