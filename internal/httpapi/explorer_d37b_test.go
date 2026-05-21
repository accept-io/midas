package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37b-authority-graph-cytoscape-html-card-renderer-on-graphviewport
//
// Promotes the Cytoscape HTML-card Authority renderer (formerly the
// `?cytoscape=1`-gated PoC under renderer id `'authority-cytoscape'`)
// to the PRODUCTION Authority renderer hosted by GraphViewport
// under the renderer id `'authority'`. Tests below pin:
//
//   • The pre-D37b `?cytoscape=1` URL gate is retired.
//   • The pre-D37b renderer id `'authority-cytoscape'` is retired
//     from the JS executable surface in favour of `'authority'`.
//   • The pre-D37b user-facing aria-label "(Cytoscape PoC)" is
//     retired.
//   • Activation flows through `viewport.activateById('authority')`.
//   • Registration uses `viewport.register('authority',
//     _authorityRendererFactory)` at module init.
//   • CSS rules are scoped to
//     `.midas-graph-viewport[data-active-renderer="authority"]`.
//   • Mount remains inside `.midas-graph-renderer-slot`.
//   • Cytoscape engine + HTML card design + theme system are
//     preserved (no degradation to plain Cytoscape labels).
//   • Diagnostics / Surface posture / Workbench panels are bridged
//     from the Cytoscape render path.
//   • Existing `/v1/graphs/authority` backend projection is still
//     the data source.
//   • `graph-viewport.js` carries no Authority-specific branch.
//   • Context renderer + Knowledge shell + foundation D35/D36
//     contracts remain valid.
//
// Internal naming debt deliberately NOT addressed in D37b (scoped
// to a future D37c+ cleanup):
//   • Module filename `authority-cytoscape-poc.js`
//   • CSS filename `authority-cytoscape-poc.css`
//   • Mount element class `.cytoscape-poc-mount`
//   • Public-surface namespace `window.MIDASExplorerGraph.cytoscapePoc`
//
// These naming artifacts ship today as internal naming debt; the
// strategic rename is confined to renderer id + user-facing
// surface in D37b.

const (
	d37bAuthorityShellAssetPath = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37bAuthorityCSSPath        = "/explorer/assets/css/authority-cytoscape-poc.css"
	d37bHostAssetPath           = "/explorer/assets/js/graph/graph-viewport.js"
)

// TestExplorer_D37bAuthority_ActivationUsesGraphViewportCytoscapeRenderer
// pins that the Authority renderer module is unconditionally active
// (no URL gate) and routes Authority activation through GraphViewport
// + Cytoscape.
func TestExplorer_D37bAuthority_ActivationUsesGraphViewportCytoscapeRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)
	exec := stripJSComments(js)

	// Pre-D37b URL-flag gate must be retired from executable code.
	if strings.Contains(exec, "sp.get('cytoscape') === '1'") {
		t.Error("D37b: pre-D37b `?cytoscape=1` gate must be retired from executable code")
	}
	if strings.Contains(exec, "if (!_isPocActive()) {") {
		t.Error("D37b: pre-D37b `if (!_isPocActive()) { return; }` IIFE guard must be retired from executable code")
	}

	// _isPocActive is preserved as a public symbol (back-compat).
	if !strings.Contains(js, "function _isPocActive()") {
		t.Error("D37b: _isPocActive symbol must remain defined (back-compat after gate retirement)")
	}

	// Cytoscape is the engine the module instantiates.
	if !strings.Contains(exec, "window.cytoscape") {
		t.Error("D37b: Authority renderer must drive Cytoscape (window.cytoscape reference)")
	}

	// The module wires Authority activation through GraphViewport.
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport",
		"vp.activateById('authority')",
		"vp.register('authority', _authorityRendererFactory)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: Authority renderer must wire through GraphViewport — missing %q", want)
		}
	}
}

// TestExplorer_D37bAuthority_RegistersProductionAuthorityRenderer
// pins the IIFE-end GraphViewport registration uses the production
// renderer id `'authority'`.
func TestExplorer_D37bAuthority_RegistersProductionAuthorityRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)

	if !strings.Contains(js, "vp.register('authority', _authorityRendererFactory)") {
		t.Error("D37b: Authority must register under production renderer id `'authority'`")
	}
	// Defensive registration helper still present.
	if !strings.Contains(js, "_registerWithGraphViewport") {
		t.Error("D37b: defensive IIFE-end _registerWithGraphViewport helper must remain")
	}
	// Pre-D37b id literal must be retired from registration code.
	exec := stripJSComments(js)
	if strings.Contains(exec, "vp.register('authority-cytoscape'") {
		t.Error("D37b: pre-D37b renderer id `'authority-cytoscape'` must NOT be used in vp.register")
	}
}

// TestExplorer_D37bAuthority_ActivatesAuthorityById pins the in-module
// activation call (`_ensureMount` activates by id) uses `'authority'`.
func TestExplorer_D37bAuthority_ActivatesAuthorityById(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)
	exec := stripJSComments(js)

	if !strings.Contains(js, "vp.activateById('authority')") {
		t.Error("D37b: _ensureMount must call vp.activateById('authority')")
	}
	if !strings.Contains(js, "vp.getActiveRendererId() !== 'authority'") {
		t.Error("D37b: _ensureMount must guard on vp.getActiveRendererId() !== 'authority'")
	}
	if !strings.Contains(js, "vp.getActiveRendererId() === 'authority'") {
		t.Error("D37b: _uninstallPoc must guard on vp.getActiveRendererId() === 'authority' for host-routed deactivate")
	}
	if strings.Contains(exec, "vp.activateById('authority-cytoscape'") {
		t.Error("D37b: pre-D37b vp.activateById('authority-cytoscape') must be retired from executable code")
	}
}

// TestExplorer_D37bAuthority_DataActiveRendererIsAuthority pins that
// CSS keys off the production renderer id `'authority'`, and that
// the pre-D37b `'authority-cytoscape'` selector is retired.
func TestExplorer_D37bAuthority_DataActiveRendererIsAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37bAuthorityCSSPath)
	exec := stripCSSComments(css)

	if !strings.Contains(exec, `.midas-graph-viewport[data-active-renderer="authority"]`) {
		t.Error("D37b: Authority CSS must use the production renderer-identity selector")
	}
	if strings.Contains(exec, `.midas-graph-viewport[data-active-renderer="authority-cytoscape"]`) {
		t.Error("D37b: Authority CSS must NOT use the pre-D37b renderer id 'authority-cytoscape'")
	}
}

// TestExplorer_D37bAuthority_MountsInsideRendererSlot pins that the
// renderer mount is created inside `.midas-graph-renderer-slot`
// (host-supplied via factory.mount(slotEl, ctx)).
func TestExplorer_D37bAuthority_MountsInsideRendererSlot(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)

	// Bound assertions to the factory's mount body.
	factStart := strings.Index(js, "var _authorityRendererFactory = {")
	if factStart < 0 {
		t.Fatal("D37b: _authorityRendererFactory definition not found")
	}
	factEnd := strings.Index(js[factStart:], "\n  };\n")
	if factEnd < 0 {
		t.Fatal("D37b: cannot bound factory definition")
	}
	body := js[factStart : factStart+factEnd]

	if !strings.Contains(body, "slotEl.appendChild(_mountEl)") {
		t.Error("D37b: factory.mount must append _mountEl to the host slotEl")
	}
	if !strings.Contains(body, "_mountEl.className = 'cytoscape-poc-mount';") {
		t.Error("D37b: factory.mount must create the .cytoscape-poc-mount element (filename naming debt; class name retained as internal naming debt)")
	}
	// The factory.mount must NOT mount into the legacy scroll surface.
	if strings.Contains(body, ".governance-map-canvas-scroll") {
		t.Error("D37b: factory.mount must NOT reference .governance-map-canvas-scroll (legacy fallback retired in D35f)")
	}
}

// TestExplorer_D37bAuthority_PreservesHtmlCardDesign pins that the
// theme system + HTML card overlay + per-kind visual descriptors
// remain available — i.e. D37b did not degrade the Authority
// renderer to plain Cytoscape labels.
func TestExplorer_D37bAuthority_PreservesHtmlCardDesign(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)

	// Theme system intact (declarative theme array + resolver).
	for _, want := range []string{
		"var _THEMES",
		"function _resolveTheme()",
		"var _activeTheme",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: theme system must remain — missing %q", want)
		}
	}

	// The Authority HTML card overlay implementation is preserved.
	for _, want := range []string{
		"function _installHtmlCardOverlay(",
		"function _updateHtmlCardOverlay(",
		"function _destroyHtmlCardOverlay(",
		"'cytoscape-poc-html-card'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: HTML card overlay must remain — missing %q", want)
		}
	}

	// Per-kind visual descriptors (PoC palette + size + shape per
	// Authority node kind) intact.
	for _, want := range []string{
		"function _kindStyle(palette)",
		"business_service:",
		"decision_surface:",
		"authority_profile:",
		"authority_grant:",
		"agent:",
		"fail_mode_policy:",
		"escalation_target:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: per-kind visual descriptors must remain — missing %q", want)
		}
	}

	// CSS file carries the html-card rule set + per-kind selectors.
	css := getExplorerAsset(t, srv, d37bAuthorityCSSPath)
	for _, want := range []string{
		".cytoscape-poc-html-card",
		`data-kind="business_service"`,
		`data-kind="decision_surface"`,
		`data-kind="authority_profile"`,
		`data-kind="authority_grant"`,
		`data-kind="agent"`,
		`data-kind="fail_mode_policy"`,
		`data-kind="escalation_target"`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37b: Authority HTML-card CSS must keep %q", want)
		}
	}
}

// TestExplorer_D37bAuthority_DoesNotUsePlainCyLabelsAsPrimaryVisual
// pins that the Authority renderer does NOT degrade to plain
// Cytoscape labels (i.e. theme system + per-kind style descriptors
// remain in place + per-kind icon registry remains in place).
func TestExplorer_D37bAuthority_DoesNotUsePlainCyLabelsAsPrimaryVisual(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)

	// Icon registry remains wired so node kinds carry Lucide icons,
	// not bare text.
	for _, want := range []string{
		"_AUTHORITY_KIND_ICON_KEYS",
		"function _iconForKind(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: icon registry must remain (no bare Cytoscape labels) — missing %q", want)
		}
	}

	// Visual chrome (kind labels, hover state, focused border weight)
	// remains intact via the per-theme style descriptors.
	if !strings.Contains(js, "borderWidthFocused") {
		t.Error("D37b: focused-border style must remain (visual selected/hover state preserved)")
	}
}

// TestExplorer_D37bAuthority_LegacyNativePathNotDefault pins that
// the pre-D37b production native renderer is NOT the default
// Authority path. The legacy `renderAuthorityGraph` function still
// exists in `authority-graph-view.js` (removal would be too broad
// for D37b), but the Cytoscape PoC patches `authorityView.refresh`
// at module init so normal Authority activation routes through
// Cytoscape. The original is preserved on
// `authorityView._pocOriginalRefresh` for diagnostics.
func TestExplorer_D37bAuthority_LegacyNativePathNotDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	pocJS := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// The PoC patches authorityView.refresh (and preserves the
	// original for diagnostics).
	for _, want := range []string{
		"av._pocOriginalRefresh = av._pocOriginalRefresh || av.refresh",
		"av.refresh = _pocRefresh",
		"_pocRefresh",
		"_patchAuthorityViewRefresh",
	} {
		if !strings.Contains(pocJS, want) {
			t.Errorf("D37b: PoC must intercept authorityView.refresh — missing %q", want)
		}
	}

	// The native view's renderAuthorityGraph still EXISTS (legacy
	// preserved as internal fallback) but is no longer the default
	// user-facing path.
	if !strings.Contains(viewJS, "function renderAuthorityGraph(payload, ctx)") {
		t.Error("D37b: legacy renderAuthorityGraph must still be defined as internal fallback")
	}
}

// TestExplorer_D37bAuthority_UsesExistingAuthorityProjectionFetch
// pins that the Cytoscape Authority renderer still consumes the
// existing /v1/graphs/authority projection via the shared adapter.
func TestExplorer_D37bAuthority_UsesExistingAuthorityProjectionFetch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)

	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityAdapter",
		"adapter.fetch({",
		"view: 'service'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: Authority renderer must consume existing adapter.fetch — missing %q", want)
		}
	}

	// The API-client method still targets /v1/graphs/authority.
	clientJS := getExplorerAsset(t, srv, "/explorer/assets/js/core/api-client.js")
	if !strings.Contains(clientJS, "'/v1/graphs/authority'") {
		t.Error("D37b: API client must still target /v1/graphs/authority")
	}
}

// TestExplorer_D37bAuthority_NoUserFacingPocLabel pins that the
// user-facing aria-label is the production "Authority Graph"
// without the pre-D37b "(Cytoscape PoC)" suffix.
func TestExplorer_D37bAuthority_NoUserFacingPocLabel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)

	// Negative — pre-D37b aria-label retired.
	if strings.Contains(js, `'Authority Graph (Cytoscape PoC)'`) {
		t.Error("D37b: pre-D37b aria-label `'Authority Graph (Cytoscape PoC)'` must be retired")
	}
	if strings.Contains(js, `"Authority Graph (Cytoscape PoC)"`) {
		t.Error("D37b: pre-D37b aria-label `\"Authority Graph (Cytoscape PoC)\"` must be retired")
	}
	// Positive — production aria-label set.
	if !strings.Contains(js, `_mountEl.setAttribute('aria-label', 'Authority Graph')`) {
		t.Error("D37b: production aria-label `'Authority Graph'` must be set on the mount")
	}
}

// TestExplorer_D37bAuthority_SelectionRoutesToInspectorBridge pins
// that node selection in the Cytoscape Authority renderer routes
// through the existing inspector-carrier bridge (which the
// production right-drawer inspector consumes).
func TestExplorer_D37bAuthority_SelectionRoutesToInspectorBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)

	for _, want := range []string{
		"_renderInspectorCarriers",
		"_clearInspectorCarriers",
		"_detailsForCarrier",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: selection/inspector bridge must remain — missing %q", want)
		}
	}
}

// TestExplorer_D37bAuthority_DiagnosticsPostureWorkbenchStatePinned
// pins that the Cytoscape render path calls the existing
// diagnostics-panel, surface-posture-panel, and workbench modules
// so all four panel surfaces still render after Authority activation
// flips to Cytoscape.
func TestExplorer_D37bAuthority_DiagnosticsPostureWorkbenchStatePinned(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)

	// Cached projection so the workbench (cache-driven) sees the
	// same payload the Cytoscape renderer drew from.
	if !strings.Contains(js, "window.MIDASExplorerGraph._lastAuthorityProjection = payload") {
		t.Error("D37b: Cytoscape render path must cache _lastAuthorityProjection so workbench reads from it")
	}

	// Bridge calls into the existing panel modules.
	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityDiagnosticsPanel",
		"diagPanel.render(payload)",
		"window.MIDASExplorerGraph.authoritySurfacePosturePanel",
		"posturePanel.render(payload)",
		"window.MIDASExplorerGraph.authorityWorkbench",
		"workbenchMod.render()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37b: diagnostics/posture/workbench bridge must call the existing panel modules — missing %q", want)
		}
	}
}

// TestExplorer_D37bAuthority_NoBodyClassOrLegacyScrollFallback pins
// that D37b did not reintroduce any retired pattern from D35f.
func TestExplorer_D37bAuthority_NoBodyClassOrLegacyScrollFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37bAuthorityShellAssetPath)
	exec := stripJSComments(js)

	// No body-class activation flips.
	for _, banned := range []string{
		"document.body.classList.add('cytoscape-poc-active')",
		"document.body.classList.remove('cytoscape-poc-active')",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D37b: retired body-class activation %q must remain retired", banned)
		}
	}

	// _ensureMount executable code must not introduce a legacy
	// scroll-surface fallback path. (The factory body is the place
	// this would have lived.) Bound the negative pin to the
	// `_ensureMount` body.
	emStart := strings.Index(exec, "function _ensureMount()")
	if emStart < 0 {
		t.Fatal("D37b: _ensureMount definition not found")
	}
	emEnd := strings.Index(exec[emStart:], "\n  }\n")
	if emEnd < 0 {
		t.Fatal("D37b: cannot bound _ensureMount body")
	}
	emBody := exec[emStart : emStart+emEnd]
	if strings.Contains(emBody, "parent.insertBefore(_mountEl, host)") {
		t.Error("D37b: legacy `parent.insertBefore(_mountEl, host)` fallback must remain retired (D35f)")
	}
	if strings.Contains(emBody, "getElementsByClassName('governance-map-canvas-scroll')") {
		t.Error("D37b: legacy `.governance-map-canvas-scroll` fallback must remain retired (D35f)")
	}
}

// TestExplorer_D37bAuthority_NoGraphViewportHostSpecialCase pins
// that `graph-viewport.js` EXECUTABLE CODE remains renderer-neutral
// — no Authority-specific id literal in the host's executable code.
// (Comments inside `graph-viewport.js` may still mention `'authority'`
// or `'authority-cytoscape'` as historical illustrative examples;
// the host's BEHAVIOUR is the strategic invariant, not its comment
// hygiene. Comment refresh is acknowledged debt for D37c.)
func TestExplorer_D37bAuthority_NoGraphViewportHostSpecialCase(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d37bHostAssetPath)
	hostExec := stripJSComments(hostJS)

	// No literal renderer id in graph-viewport.js EXECUTABLE code.
	for _, banned := range []string{
		"'authority'",
		`"authority"`,
		"'authority-cytoscape'",
		`"authority-cytoscape"`,
	} {
		if strings.Contains(hostExec, banned) {
			t.Errorf("D37b: graph-viewport.js executable code must remain renderer-neutral — found %q", banned)
		}
	}

	// The generic registry surface remains intact.
	for _, want := range []string{
		"function register(rendererId, factory)",
		"function activateById(rendererId)",
		"function _setActiveRendererAttribute(rendererId)",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D37b: generic GraphViewport registry surface %q must remain", want)
		}
	}
}

// TestExplorer_D37b_ContextAndKnowledgeContractsPreserved pins that
// Context renderers (native + Cytoscape spike) and the Knowledge
// shell remain unaffected by the Authority promotion.
func TestExplorer_D37b_ContextAndKnowledgeContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Context Cytoscape spike still registers + activates under
	// 'context-cytoscape'.
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	for _, want := range []string{
		"vp.register('context-cytoscape', _contextCytoscapeRendererFactory)",
		"vp.activateById('context-cytoscape')",
	} {
		if !strings.Contains(ctxJS, want) {
			t.Errorf("D37b: Context Cytoscape contract %q must remain", want)
		}
	}

	// Knowledge shell still registers + activates under
	// 'knowledge-graph'.
	knJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js")
	for _, want := range []string{
		"vp.register('knowledge-graph', _knowledgeGraphRendererFactory)",
		"vp.activateById(RENDERER_ID)",
	} {
		if !strings.Contains(knJS, want) {
			t.Errorf("D37b: Knowledge shell contract %q must remain", want)
		}
	}
}

// TestExplorer_D37b_D35D36ContractsPreserved is the foundation-wide
// regression check: every D35a–D36b invariant remains intact.
func TestExplorer_D37b_D35D36ContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a — structural DOM tokens.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37b: D35a structural class %q must remain", want)
		}
	}

	hostJS := getExplorerAsset(t, srv, d37bHostAssetPath)
	for _, want := range []string{
		// D35b/c host API.
		"window.MIDASExplorerGraph.viewport = {",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
		"function getSafeArea(",
		"function onResize(",
		"function adoptExisting(",
		"adoptExisting('native-context')",
		// D35f identity attribute.
		`var ACTIVE_RENDERER_ATTR = 'data-active-renderer';`,
		// D35g registry surface.
		"function register(rendererId, factory)",
		"function unregister(rendererId)",
		"function hasRenderer(rendererId)",
		"function listRegistered()",
		"function activateById(rendererId)",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D37b: host contract %q must remain", want)
		}
	}

	// D35f strategic clip rule.
	clipCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	clipExec := stripCSSComments(clipCSS)
	if !strings.Contains(clipExec, ".midas-graph-viewport") ||
		!strings.Contains(clipExec, "overflow: hidden") {
		t.Error("D37b: `.midas-graph-viewport { overflow: hidden }` strategic clip must remain (D35f)")
	}

	// D35e — Context overlay non-clipping (exactly 1 overflow:hidden
	// in spike CSS; on the mount, not the overlay).
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if strings.Count(stripCSSComments(spikeCSS), "overflow: hidden") != 1 {
		t.Error("D37b: spike CSS must have exactly 1 `overflow: hidden` (mount; overlay non-clipping)")
	}
}
