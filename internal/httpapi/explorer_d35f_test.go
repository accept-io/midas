package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35f-retire-transitional-renderer-debt
//
// Retires the transitional legacy activation and clipping debt left
// behind by D35c/D35d/D35e:
//
//   • Host-owned renderer identity. GraphViewport publishes
//     `data-active-renderer="<rendererId>"` on `.midas-graph-
//     viewport`. activate/adoptExisting/deactivate keep it
//     consistent with `getActiveRendererId()`.
//   • Body-class activation flags retired. Authority no longer flips
//     `body.cytoscape-poc-active`; Context no longer flips
//     `body.context-cy-spike-active`.
//   • Strategic clip authority. `.midas-graph-viewport { overflow:
//     hidden }` is now the universal clip; `.context-cy-spike-
//     overlay` remains non-clipping; per-renderer mount overflow
//     stays as Cytoscape canvas-discipline defence-in-depth.
//   • Legacy fallback mount paths removed. Authority `_ensureMount`
//     no longer falls back to `parent.insertBefore(_mountEl, host)`;
//     Context `install()` no longer falls back to
//     `.governance-map-canvas-scroll` direct append.
//   • #gmap-canvas hiding re-keyed onto host renderer identity.
//   • CSS gates re-keyed onto `.midas-graph-viewport[data-active-
//     renderer="…"]` selectors.

const d35fHostAssetPath = "/explorer/assets/js/graph/graph-viewport.js"

// TestExplorer_D35fViewport_SetsDataActiveRenderer pins that the
// host writes `data-active-renderer` on `.midas-graph-viewport`.
func TestExplorer_D35fViewport_SetsDataActiveRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35fHostAssetPath)

	for _, want := range []string{
		// Constant declaration.
		`var ACTIVE_RENDERER_ATTR = 'data-active-renderer';`,
		// Helper definition.
		"function _setActiveRendererAttribute(rendererId)",
		// Helper writes the attribute on the viewport.
		"vp.setAttribute(ACTIVE_RENDERER_ATTR, rendererId)",
		"vp.removeAttribute(ACTIVE_RENDERER_ATTR)",
		// Public surface exposes the constant for tests / DevTools.
		"ACTIVE_RENDERER_ATTR: ACTIVE_RENDERER_ATTR",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35f: host must publish data-active-renderer — missing %q", want)
		}
	}
}

// TestExplorer_D35fViewport_ActiveRendererIdAndDataAttributeStayConsistent
// pins that `activate`, `adoptExisting`, and `deactivate` all
// update the data-active-renderer attribute alongside `_activeId`.
func TestExplorer_D35fViewport_ActiveRendererIdAndDataAttributeStayConsistent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35fHostAssetPath)

	// activate: sets the attribute before mount; rolls back on
	// mount failure.
	actStart := strings.Index(js, "function activate(rendererId, factory)")
	if actStart < 0 {
		t.Fatal("D35f: activate definition missing")
	}
	actEnd := strings.Index(js[actStart:], "\n  }")
	if actEnd < 0 {
		t.Fatal("D35f: cannot bound activate body")
	}
	actBody := js[actStart : actStart+actEnd]
	if !strings.Contains(actBody, "_setActiveRendererAttribute(rendererId)") {
		t.Error("D35f: activate must call _setActiveRendererAttribute(rendererId)")
	}
	if !strings.Contains(actBody, "_setActiveRendererAttribute(_baselineId || null)") {
		t.Error("D35f: activate must roll back the attribute on mount failure")
	}

	// adoptExisting: sets the attribute when adopting.
	adStart := strings.Index(js, "function adoptExisting(rendererId, handle)")
	if adStart < 0 {
		t.Fatal("D35f: adoptExisting definition missing")
	}
	adEnd := strings.Index(js[adStart:], "\n  }")
	if adEnd < 0 {
		t.Fatal("D35f: cannot bound adoptExisting body")
	}
	adBody := js[adStart : adStart+adEnd]
	if !strings.Contains(adBody, "_setActiveRendererAttribute(rendererId)") {
		t.Error("D35f: adoptExisting must call _setActiveRendererAttribute(rendererId)")
	}

	// deactivate: clears the attribute (or restores baseline).
	deStart := strings.Index(js, "function deactivate()")
	if deStart < 0 {
		t.Fatal("D35f: deactivate definition missing")
	}
	deEnd := strings.Index(js[deStart:], "\n  }")
	if deEnd < 0 {
		t.Fatal("D35f: cannot bound deactivate body")
	}
	deBody := js[deStart : deStart+deEnd]
	for _, want := range []string{
		"_setActiveRendererAttribute(_baselineId)",
		"_setActiveRendererAttribute(null)",
	} {
		if !strings.Contains(deBody, want) {
			t.Errorf("D35f: deactivate must include %q (baseline restore / clear)", want)
		}
	}
}

// TestExplorer_D35fViewport_IsStrategicClipAuthority pins the
// promotion of `.midas-graph-viewport` to `overflow: hidden`.
func TestExplorer_D35fViewport_IsStrategicClipAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	idx := strings.Index(css, ".midas-graph-viewport {")
	if idx < 0 {
		t.Fatal("D35f: .midas-graph-viewport CSS rule missing")
	}
	end := strings.Index(css[idx:], "\n  }")
	if end < 0 {
		t.Fatal("D35f: cannot bound .midas-graph-viewport rule")
	}
	block := css[idx : idx+end]
	for _, want := range []string{
		"position: relative",
		"min-height: 0",
		// D35f-promoted strategic clip authority.
		"overflow: hidden",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D35f: viewport rule must include %q (strategic clip authority); body was:\n%s", want, block)
		}
	}
}

// TestExplorer_D35fAuthority_NoBodyClassActivation pins that the
// Authority module no longer flips `body.cytoscape-poc-active`.
func TestExplorer_D35fAuthority_NoBodyClassActivation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	exec := stripJSComments(js)

	for _, banned := range []string{
		"document.body.classList.add('cytoscape-poc-active')",
		"document.body.classList.remove('cytoscape-poc-active')",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D35f: Authority must NOT flip body class — found %q (host owns renderer identity)", banned)
		}
	}

	// Authority CSS gate has migrated to the host renderer-identity
	// selector; pre-D35f body-class gate is gone from executable
	// CSS.
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	execCSS := stripCSSComments(css)
	if strings.Contains(execCSS, "body.cytoscape-poc-active") {
		t.Error("D35f: Authority CSS must NOT contain `body.cytoscape-poc-active` (re-keyed onto host renderer-identity selector)")
	}
	if !strings.Contains(execCSS, `.midas-graph-viewport[data-active-renderer="authority"]`) {
		t.Error("D35f: Authority CSS must use the host renderer-identity selector")
	}
}

// TestExplorer_D35fAuthority_NoLegacyScrollFallback pins that the
// Authority module no longer falls back to inserting the mount
// before `#gmap-canvas` inside `.governance-map-canvas-scroll`.
func TestExplorer_D35fAuthority_NoLegacyScrollFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	exec := stripJSComments(js)

	if strings.Contains(exec, "parent.insertBefore(_mountEl, host)") {
		t.Error("D35f: Authority must NOT have the `parent.insertBefore(_mountEl, host)` legacy fallback (host is always available)")
	}
}

// TestExplorer_D35fAuthority_StillActivatesThroughHost pins that
// Authority still routes activation through the host.
func TestExplorer_D35fAuthority_StillActivatesThroughHost(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// D35g — register + activateById replaces direct vp.activate.
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
		"var _authorityRendererFactory = {",
		"slotEl.appendChild(_mountEl)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35f/D35g: Authority must still activate via host — missing %q", want)
		}
	}
}

// TestExplorer_D35fAuthority_StillUsesSafeAreaResizeAndDestroyContracts
// pins that Authority's D35d contracts still hold.
func TestExplorer_D35fAuthority_StillUsesSafeAreaResizeAndDestroyContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"_rendererCtx.getSafeArea",
		"ctx.onResize(_refitWithSafeArea)",
		"vp.deactivate",
		"_teardownPocResources",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35f: Authority D35d contract %q must remain", want)
		}
	}
}

// TestExplorer_D35fContext_NoBodyClassActivation pins that the
// Context spike no longer flips `body.context-cy-spike-active`.
func TestExplorer_D35fContext_NoBodyClassActivation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	exec := stripJSComments(js)

	for _, banned := range []string{
		"document.body.classList.add(BODY_FLAG_CLASS)",
		"document.body.classList.remove(BODY_FLAG_CLASS)",
		"document.body.classList.add('context-cy-spike-active')",
		"document.body.classList.remove('context-cy-spike-active')",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D35f: Context spike must NOT flip body class — found %q (host owns renderer identity)", banned)
		}
	}

	// Spike CSS gate migrated to host renderer-identity selector.
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	execCSS := stripCSSComments(css)
	if strings.Contains(execCSS, "body.context-cy-spike-active") {
		t.Error("D35f: spike CSS must NOT contain `body.context-cy-spike-active` (re-keyed onto host renderer-identity selector)")
	}
	if !strings.Contains(execCSS, `.midas-graph-viewport[data-active-renderer="context-cytoscape"]`) {
		t.Error("D35f: spike CSS must use the host renderer-identity selector")
	}
}

// TestExplorer_D35fContext_NoLegacyScrollFallback pins that the
// Context spike no longer falls back to `.governance-map-canvas-
// scroll` in install().
func TestExplorer_D35fContext_NoLegacyScrollFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	exec := stripJSComments(js)

	// Negative pin in install() body specifically — diagnostic
	// lookups elsewhere (debugState) legitimately read the scroll
	// wrapper's rect.
	installStart := strings.Index(exec, "function install(options) {")
	if installStart < 0 {
		t.Fatal("D35f: install definition missing")
	}
	installEnd := strings.Index(exec[installStart:], "\n  }\n")
	if installEnd < 0 {
		t.Fatal("D35f: cannot bound install body")
	}
	installBody := exec[installStart : installStart+installEnd]
	if strings.Contains(installBody, "getElementsByClassName('governance-map-canvas-scroll')") {
		t.Error("D35f: Context install() must NOT have legacy `.governance-map-canvas-scroll` fallback")
	}
}

// TestExplorer_D35fContext_StillActivatesThroughHost pins that
// Context still routes activation through the host.
func TestExplorer_D35fContext_StillActivatesThroughHost(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		// D35g — register + activateById replaces direct vp.activate.
		"vp.register('context-cytoscape', _contextCytoscapeRendererFactory)",
		"vp.activateById('context-cytoscape')",
		"var _contextCytoscapeRendererFactory = {",
		"_installResources(slotEl)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35f/D35g: Context must still activate via host — missing %q", want)
		}
	}
}

// TestExplorer_D35fContext_StillUsesSafeAreaResizeAndDestroyContracts
// pins that Context's D35e contracts still hold.
func TestExplorer_D35fContext_StillUsesSafeAreaResizeAndDestroyContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	for _, want := range []string{
		"_rendererCtx.getSafeArea()",
		"ctx.onResize(_onHostResize)",
		"vp.deactivate",
		"_teardownResources",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35f: Context D35e contract %q must remain", want)
		}
	}
}

// TestExplorer_D35fContext_OverlayStillDoesNotOwnClipping pins
// that D35e's strategic fix (overlay has no overflow:hidden)
// survives D35f's CSS re-keying.
func TestExplorer_D35fContext_OverlayStillDoesNotOwnClipping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")

	idx := strings.Index(css, `.midas-graph-viewport[data-active-renderer="context-cytoscape"] .context-cy-spike-overlay {`)
	if idx < 0 {
		t.Fatal("D35f: overlay CSS rule missing")
	}
	end := strings.Index(css[idx:], "\n}")
	if end < 0 {
		t.Fatal("D35f: cannot bound overlay CSS rule")
	}
	block := css[idx : idx+end]
	if strings.Contains(block, "overflow: hidden") {
		t.Errorf("D35f: overlay rule must remain non-clipping; body was:\n%s", block)
	}
}

// TestExplorer_D35fCSS_NoBodyClassRendererState pins that NO
// executable CSS keys renderer state off body classes.
func TestExplorer_D35fCSS_NoBodyClassRendererState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, path := range []string{
		"/explorer/assets/css/authority-cytoscape-poc.css",
		"/explorer/assets/css/context-cytoscape-overlay-spike.css",
	} {
		css := getExplorerAsset(t, srv, path)
		exec := stripCSSComments(css)
		for _, banned := range []string{
			"body.cytoscape-poc-active",
			"body.context-cy-spike-active",
		} {
			if strings.Contains(exec, banned) {
				t.Errorf("D35f: %s must NOT contain renderer-state selector %q (host owns renderer identity)", path, banned)
			}
		}
	}
}

// TestExplorer_D35fCSS_NoTacticalCanvasHidingHack pins that
// `#gmap-canvas` hiding rules are scoped to host renderer identity
// only — no body-class or other tactical hide.
func TestExplorer_D35fCSS_NoTacticalCanvasHidingHack(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, path := range []string{
		"/explorer/assets/css/authority-cytoscape-poc.css",
		"/explorer/assets/css/context-cytoscape-overlay-spike.css",
	} {
		css := getExplorerAsset(t, srv, path)
		exec := stripCSSComments(css)

		// Every `#gmap-canvas` selector must be scoped to a
		// `.midas-graph-viewport[data-active-renderer="…"]` ancestor.
		segs := strings.Split(exec, "#gmap-canvas")
		for i := 0; i < len(segs)-1; i++ {
			pre := segs[i]
			// The selector chain ends at the most recent `}` (or
			// start of file). Anything between that and the
			// `#gmap-canvas` token is the selector.
			lastClose := strings.LastIndex(pre, "}")
			selector := pre
			if lastClose >= 0 {
				selector = pre[lastClose+1:]
			}
			if !strings.Contains(selector, `.midas-graph-viewport[data-active-renderer=`) {
				t.Errorf("D35f: %s — #gmap-canvas selector not host-keyed: %q", path, strings.TrimSpace(selector))
			}
		}
	}
}

// TestExplorer_D35f_D35aThroughD35eContractsPreserved pins all
// foundation contracts from D35a–D35e remain valid.
func TestExplorer_D35f_D35aThroughD35eContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a — structural DOM.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35f: D35a structural class %q must remain", want)
		}
	}

	// D35b/c — host API + adoptExisting + baseline.
	hostJS := getExplorerAsset(t, srv, d35fHostAssetPath)
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport = {",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
		"function getSafeArea(",
		"function onResize(",
		"function adoptExisting(",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35f: D35b/c API %q must remain", want)
		}
	}

	// D35d — Authority migration (D35g register + activateById).
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	if !strings.Contains(authJS, "vp.activateById('authority')") {
		t.Error("D35f/D35g: D35d Authority host-routed activation must remain (now via vp.activateById)")
	}
	if !strings.Contains(authJS, "vp.register('authority', _authorityRendererFactory)") {
		t.Error("D35f/D35g: D35d Authority renderer must be registered with the host")
	}

	// D35e — Context migration (D35g register + activateById).
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	if !strings.Contains(ctxJS, "vp.activateById('context-cytoscape')") {
		t.Error("D35f/D35g: D35e Context host-routed activation must remain (now via vp.activateById)")
	}
	if !strings.Contains(ctxJS, "vp.register('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35f/D35g: D35e Context renderer must be registered with the host")
	}

	// D35e — overlay non-clipping fix preserved.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if strings.Count(stripCSSComments(spikeCSS), "overflow: hidden") != 1 {
		t.Error("D35f: spike CSS must have exactly 1 `overflow: hidden` (the mount; overlay is non-clipping)")
	}
}
