package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35d-port-authority-cytoscape-to-graphviewport
//
// Ports Authority Cytoscape onto the MIDASGraphViewport host as the
// first real alternative renderer:
//
//   • Authority activation routes through the GraphViewport host.
//     Pre-D35g this was `viewport.activate('authority',
//     factory)`. D35g introduced the renderer registry, so the
//     module now registers its factory once at module init via
//     `viewport.register('authority', factory)` and
//     activates via `viewport.activateById('authority')`.
//     These D35d assertions have been updated to the registry
//     contract; the underlying invariant (Authority routes through
//     the host, not direct DOM) is unchanged.
//   • The renderer factory's `mount(slotEl, ctx)` creates
//     `.cytoscape-poc-mount` INSIDE `.midas-graph-renderer-slot`
//     supplied by the host (NOT inserted before `#gmap-canvas` inside
//     `.governance-map-canvas-scroll` as it did pre-D35d).
//   • Authority uses `ctx.getSafeArea()` for fit-padding composition.
//   • Authority subscribes to `ctx.onResize(...)` for resize handling
//     (callback triggers cy.resize + refit).
//   • Destroy unsubscribes resize, calls cy.destroy, removes only
//     Authority-owned DOM. Routed through `viewport.deactivate()`.
//
// Transitional debt (D35d does NOT remove):
//   • `body.cytoscape-poc-active` body class is still added by the
//     PoC IIFE on `?cytoscape=1`. The class drives load-bearing CSS
//     (most notably `body.cytoscape-poc-active #gmap-canvas {
//     display:none !important }`) that the renderer-id model is
//     meant to replace. Marked for removal in D35f.
//   • `body.cytoscape-poc-active #gmap-canvas { display:none
//     !important }` CSS rule remains. D35f task.
//
// Explicit non-goals enforced by tests below:
//   • Context Cytoscape spike is NOT migrated.
//   • `.context-cy-spike-overlay { overflow: hidden }` is unchanged.
//   • Native Context's `#gmap-canvas`/`#gmap-scene`/`#gmap-svg` DOM
//     is not deleted or recreated.
//   • No NEW tactical activation flag / hide rule / overflow hack is
//     introduced.

const d35dAuthorityAssetPath = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"

// TestExplorer_D35dAuthority_ActivatesThroughViewportHost pins
// that Authority Cytoscape routes its mount creation through the
// GraphViewport host. As of D35g, activation is via the registry:
// the module registers `_authorityRendererFactory` under the id
// `'authority'` and `_ensureMount` calls
// `viewport.activateById('authority')`.
func TestExplorer_D35dAuthority_ActivatesThroughViewportHost(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35dAuthorityAssetPath)

	// D35g — Activation is now by registered renderer id.
	if !strings.Contains(js, "vp.activateById('authority')") {
		t.Error("D35d/D35g: Authority must call vp.activateById('authority')")
	}
	// D35g — The factory must be registered with the host.
	if !strings.Contains(js, "vp.register('authority', _authorityRendererFactory)") {
		t.Error("D35d/D35g: Authority must register its factory via vp.register('authority', _authorityRendererFactory)")
	}
	// D35g — Direct factory activation is retired.
	if strings.Contains(js, "vp.activate('authority', _authorityRendererFactory)") {
		t.Error("D35g: Authority must NOT pass the factory directly to vp.activate — use register + activateById")
	}
	// The host lookup is defensive: tolerates missing host.
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport",
		"typeof vp.activateById === 'function'",
		"typeof vp.getActiveRendererId === 'function'",
		"typeof vp.getRendererSlotEl === 'function'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35d/D35g: Authority activation must defensively probe host — missing %q", want)
		}
	}
}

// TestExplorer_D35dAuthority_RendererFactoryLifecycle pins the
// factory contract shape: `mount(slotEl, ctx)` returning a handle
// with `destroy()`.
func TestExplorer_D35dAuthority_RendererFactoryLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35dAuthorityAssetPath)

	for _, want := range []string{
		// Factory definition.
		"var _authorityRendererFactory = {",
		"mount: function (slotEl, ctx) {",
		// Returns a handle with destroy.
		"destroy: function () {",
		// Factory is exposed on the public surface so tests + future
		// host-driven activation can call it directly.
		"_rendererFactory:          _authorityRendererFactory",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35d: renderer factory contract must include %q", want)
		}
	}
}

// TestExplorer_D35dAuthority_MountsInsideRendererSlot pins that
// the factory creates the `.cytoscape-poc-mount` element as a
// child of the host-supplied `slotEl`.
func TestExplorer_D35dAuthority_MountsInsideRendererSlot(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35dAuthorityAssetPath)

	// Bound to the factory's mount body so we don't accidentally
	// match similar tokens elsewhere in the file.
	start := strings.Index(js, "var _authorityRendererFactory = {")
	if start < 0 {
		t.Fatal("D35d: factory definition not found")
	}
	end := strings.Index(js[start:], "\n  };\n")
	if end < 0 {
		t.Fatal("D35d: cannot bound factory definition")
	}
	body := js[start : start+end]

	for _, want := range []string{
		// Element creation.
		"document.createElement('div')",
		`_mountEl.className = 'cytoscape-poc-mount';`,
		// Appended to the host's slotEl, NOT to #gmap-canvas's parent.
		"slotEl.appendChild(_mountEl)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35d: factory mount must include %q", want)
		}
	}

	// Negative pin — factory MUST NOT use the pre-D35d
	// `parent.insertBefore(_mountEl, host)` shape (which placed the
	// mount as a sibling of #gmap-canvas inside .governance-map-
	// canvas-scroll). The legacy insert lives in the fallback path
	// (NOT in the factory), so this pin guards the factory only.
	if strings.Contains(body, "parent.insertBefore(_mountEl, host)") {
		t.Error("D35d: factory must NOT use the legacy parent.insertBefore mount pattern — slotEl.appendChild only")
	}
}

// TestExplorer_D35dAuthority_DoesNotAppendToLegacyScrollSurface
// pins that the strategic mount target is the renderer slot, not
// `.governance-map-canvas-scroll`.
//
// The legacy fallback (used only when the host is absent — pre-
// D35a/D35b builds, headless harnesses without graph-viewport.js)
// still uses the old parent path. That fallback is marked as
// transitional debt in the source.
func TestExplorer_D35dAuthority_DoesNotAppendToLegacyScrollSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35dAuthorityAssetPath)

	// `_ensureMount` body must contain the host-routed path BEFORE
	// the legacy fallback.
	emStart := strings.Index(js, "function _ensureMount()")
	if emStart < 0 {
		t.Fatal("D35d: _ensureMount definition not found")
	}
	emEnd := strings.Index(js[emStart:], "\n  }")
	if emEnd < 0 {
		t.Fatal("D35d: cannot bound _ensureMount body")
	}
	emBody := js[emStart : emStart+emEnd]

	// D35d host-routed path remains. D35g changed the shape of the
	// call from `vp.activate(id, factory)` to `vp.activateById(id)`.
	if !strings.Contains(emBody, "vp.activateById('authority')") {
		t.Error("D35d/D35g: _ensureMount must include the host-routed activation path (vp.activateById)")
	}
	if strings.Contains(emBody, "vp.activate('authority', _authorityRendererFactory)") {
		t.Error("D35g: _ensureMount must NOT pass the factory directly to vp.activate — use vp.activateById")
	}

	// D35f-retire-transitional-renderer-debt — the legacy fallback
	// that previously inserted `.cytoscape-poc-mount` before
	// `#gmap-canvas` inside `.governance-map-canvas-scroll` has
	// been retired. _ensureMount must NOT contain the legacy
	// insertBefore pattern.
	if strings.Contains(emBody, "parent.insertBefore(_mountEl, host)") {
		t.Error("D35f: legacy `parent.insertBefore(_mountEl, host)` fallback must be retired from _ensureMount")
	}
}

// TestExplorer_D35dAuthority_UsesSafeAreaForFit pins that the
// fit-padding helper composes with `ctx.getSafeArea()` from the
// host when the renderer ctx is available.
func TestExplorer_D35dAuthority_UsesSafeAreaForFit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35dAuthorityAssetPath)

	// `_safeAreaPadding` must reference the ctx-supplied getSafeArea.
	saStart := strings.Index(js, "function _safeAreaPadding(dims)")
	if saStart < 0 {
		t.Fatal("D35d: _safeAreaPadding definition not found")
	}
	saEnd := strings.Index(js[saStart:], "\n  }")
	if saEnd < 0 {
		t.Fatal("D35d: cannot bound _safeAreaPadding body")
	}
	saBody := js[saStart : saStart+saEnd]

	for _, want := range []string{
		// Consults the host context.
		"_rendererCtx",
		"_rendererCtx.getSafeArea",
		// Compose with legacy CSS-token padding using Math.max.
		"Math.max(sa.top",
		"hostMax > computed",
	} {
		if !strings.Contains(saBody, want) {
			t.Errorf("D35d: _safeAreaPadding must compose host safe-area — missing %q", want)
		}
	}
}

// TestExplorer_D35dAuthority_UsesViewportResizeSubscription pins
// that the factory subscribes to `ctx.onResize(...)` and stores
// the unsubscribe for destroy.
func TestExplorer_D35dAuthority_UsesViewportResizeSubscription(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35dAuthorityAssetPath)

	start := strings.Index(js, "var _authorityRendererFactory = {")
	if start < 0 {
		t.Fatal("D35d: factory definition not found")
	}
	end := strings.Index(js[start:], "\n  };\n")
	if end < 0 {
		t.Fatal("D35d: cannot bound factory definition")
	}
	body := js[start : start+end]

	for _, want := range []string{
		"typeof ctx.onResize === 'function'",
		"_rendererResizeUnsub = ctx.onResize(_refitWithSafeArea)",
		// Destroy unsubscribes.
		"if (_rendererResizeUnsub) _rendererResizeUnsub()",
		"_rendererResizeUnsub = null;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35d: factory resize wiring must include %q", want)
		}
	}

	// Negative — Authority must NOT install a NEW independent global
	// resize listener when ctx.onResize is available. The PoC didn't
	// have one pre-D35d, so we just guard the regression.
	if strings.Contains(js, "window.addEventListener('resize'") {
		t.Error("D35d: Authority must use ctx.onResize, not window.addEventListener('resize')")
	}
}

// TestExplorer_D35dAuthority_DestroyTearsDownOwnedDomOnly pins the
// factory's destroy contract: unsubscribe resize, tear down only
// Authority-owned resources via `_teardownPocResources`, clear ctx.
func TestExplorer_D35dAuthority_DestroyTearsDownOwnedDomOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35dAuthorityAssetPath)

	// Factory.destroy contract.
	factStart := strings.Index(js, "var _authorityRendererFactory = {")
	if factStart < 0 {
		t.Fatal("D35d: factory definition not found")
	}
	factEnd := strings.Index(js[factStart:], "\n  };\n")
	if factEnd < 0 {
		t.Fatal("D35d: cannot bound factory definition")
	}
	factBody := js[factStart : factStart+factEnd]
	for _, want := range []string{
		"destroy: function () {",
		"if (_rendererResizeUnsub) _rendererResizeUnsub()",
		"_teardownPocResources();",
		"_rendererCtx = null;",
	} {
		if !strings.Contains(factBody, want) {
			t.Errorf("D35d: factory destroy must include %q", want)
		}
	}

	// `_teardownPocResources` is the new extracted helper — it
	// destroys cy + removes ONLY the Authority-owned mount DOM.
	resStart := strings.Index(js, "function _teardownPocResources()")
	if resStart < 0 {
		t.Fatal("D35d: _teardownPocResources definition not found")
	}
	resEnd := strings.Index(js[resStart:], "\n  }")
	if resEnd < 0 {
		t.Fatal("D35d: cannot bound _teardownPocResources body")
	}
	resBody := js[resStart : resStart+resEnd]
	for _, want := range []string{
		"_destroyCy();",
		"_mountEl.parentNode.removeChild(_mountEl)",
		"_mountEl = null;",
	} {
		if !strings.Contains(resBody, want) {
			t.Errorf("D35d: _teardownPocResources must include %q", want)
		}
	}
	// Negative — teardown must NOT touch native graph DOM.
	for _, banned := range []string{
		"getElementById('gmap-canvas').remove",
		"getElementById('gmap-scene').remove",
		"getElementById('gmap-svg').remove",
		`removeChild(document.getElementById("gmap-canvas")`,
	} {
		if strings.Contains(resBody, banned) {
			t.Errorf("D35d: _teardownPocResources must NOT remove native DOM — found %q", banned)
		}
	}

	// `_uninstallPoc` now routes through viewport.deactivate when the
	// host owns our activation.
	unStart := strings.Index(js, "function _uninstallPoc()")
	if unStart < 0 {
		t.Fatal("D35d: _uninstallPoc definition not found")
	}
	unEnd := strings.Index(js[unStart:], "\n  }")
	if unEnd < 0 {
		t.Fatal("D35d: cannot bound _uninstallPoc body")
	}
	unBody := js[unStart : unStart+unEnd]
	for _, want := range []string{
		"vp.deactivate",
		`vp.getActiveRendererId() === 'authority'`,
		"vp.deactivate();",
		// Fallback when host not active for us.
		"_teardownPocResources();",
	} {
		if !strings.Contains(unBody, want) {
			t.Errorf("D35d: _uninstallPoc must include %q", want)
		}
	}
	// D35f — body-class removal RETIRED. Renderer identity is now
	// host-owned via data-active-renderer; the host's
	// `viewport.deactivate()` clears it.
	if strings.Contains(unBody, "classList.remove('cytoscape-poc-active')") {
		t.Error("D35f: _uninstallPoc must NOT flip body class (host owns renderer identity)")
	}
}

// TestExplorer_D35dAuthority_DoesNotDeleteNativeGraphDom pins that
// the migration does NOT introduce any code that removes the
// native graph DOM tokens. Across the whole Authority module.
func TestExplorer_D35dAuthority_DoesNotDeleteNativeGraphDom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35dAuthorityAssetPath)

	for _, banned := range []string{
		// Direct removal patterns by id.
		`document.getElementById('gmap-canvas').remove`,
		`document.getElementById("gmap-canvas").remove`,
		`document.getElementById('gmap-scene').remove`,
		`document.getElementById('gmap-svg').remove`,
		// innerHTML wipes that would clear the slot's existing
		// native canvas-scroll wrapper.
		"slotEl.innerHTML",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D35d: Authority must NOT touch native graph DOM — found %q", banned)
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
			t.Errorf("D35d: native graph DOM token %q must remain in index.html", want)
		}
	}
}

// TestExplorer_D35dAuthority_NoNewTacticalActivationOrOverflowHack
// pins the strategic-discipline non-goal: D35d MUST NOT introduce
// a new body class, a new #gmap-canvas hide rule, a new overflow
// hack, or any other tactical "make it look right" patch.
func TestExplorer_D35dAuthority_NoNewTacticalActivationOrOverflowHack(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Authority JS — no NEW body-class additions beyond the
	// D35f-retire-transitional-renderer-debt — the pre-D35d
	// `body.cytoscape-poc-active` activation flag is RETIRED.
	// Authority no longer flips ANY body class; renderer identity
	// is host-owned via `data-active-renderer`.
	authJS := getExplorerAsset(t, srv, d35dAuthorityAssetPath)
	exec := stripJSComments(authJS)
	if strings.Contains(exec, "document.body.classList.add('cytoscape-poc-active')") {
		t.Error("D35f: Authority must NOT flip `body.cytoscape-poc-active` (host owns renderer identity via data-active-renderer)")
	}
	// No NEW body-class identifiers introduced by D35d/D35f.
	for _, banned := range []string{
		"d35d-active",
		"d35f-active",
		"authority-cytoscape-active",
		"graph-renderer-active",
	} {
		if strings.Contains(authJS, banned) {
			t.Errorf("D35d/D35f: must NOT introduce new body-class activation flag — found %q", banned)
		}
	}

	// Authority CSS — D35d adds no NEW overflow/clip/hide rules.
	authCSS := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	// Pre-D35d count of `display: none !important;` occurrences = 1
	// (the #gmap-canvas hide). Any increase means we added another
	// hide hack.
	hideCount := strings.Count(authCSS, "display: none !important;")
	if hideCount != 1 {
		t.Errorf("D35d: expected exactly 1 pre-existing `display: none !important` in authority CSS; got %d (regression?)", hideCount)
	}
}

// TestExplorer_D35d_NoContextCytoscapeMigration — SUPERSEDED.
//
// D35e migrated Context Cytoscape onto the GraphViewport host.
// The non-goal this test enforced ("no Context migration in D35d")
// is now superseded.
//
// Coverage moved to the D35e tests
// `TestExplorer_D35eContext_ActivatesThroughViewportHost` and
// `TestExplorer_D35eContext_DoesNotAppendToLegacyScrollSurface`.
func TestExplorer_D35d_NoContextCytoscapeMigration(t *testing.T) {
	t.Skip("D35e: superseded — Context Cytoscape migrated onto GraphViewport host. See TestExplorer_D35eContext_*.")
}

// TestExplorer_D35d_ContextSpikeOverflowUnchanged — SUPERSEDED.
//
// D35d-era this asserted that the spike's overlay `overflow: hidden`
// MUST survive (disappearing-card symptom deferred to D35e). D35e
// landed the strategic fix: the overlay no longer clips. The mount
// (`.context-cy-spike-mount`) still has `overflow: hidden` for
// Cytoscape canvas-discipline, so the substring still appears in
// spike CSS — but the semantic "overlay must own clipping" is
// reversed.
//
// Coverage moved to:
//   • TestExplorer_D35eContext_OverlayDoesNotOwnClipping
//   • TestExplorer_D35eContext_NoNewRendererLevelClippingHack
//
// Body-class preservation continues to be pinned in the D35d
// `_NoNewTacticalActivationOrHidingHack` family and in the new
// D35e `_NoNewTacticalActivationOrHidingHack` test.
func TestExplorer_D35d_ContextSpikeOverflowUnchanged(t *testing.T) {
	t.Skip("D35e: superseded — overlay-level clipping intentionally removed. See TestExplorer_D35eContext_OverlayDoesNotOwnClipping.")
}

// TestExplorer_D35d_D35aD35bD35cContractsPreserved pins the
// foundation contracts from prior tranches remain valid.
func TestExplorer_D35d_D35aD35bD35cContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a structural DOM.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35d: D35a structural class %q must remain in index.html", want)
		}
	}

	// D35b host API + module link.
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport = {",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
		"function getSafeArea(",
		"function onResize(",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35d: D35b host API %q must remain", want)
		}
	}

	// D35c adoptExisting + baseline.
	for _, want := range []string{
		"function adoptExisting(",
		"adoptExisting:        adoptExisting",
		"function _adoptNativeContextBaseline()",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35d: D35c API %q must remain", want)
		}
	}
}
