package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D36a-knowledge-graph-renderer-shell
//
// First controlled reuse of the GraphViewport platform module
// (D35a–D35i). Adds a thin Knowledge Graph renderer SHELL that
// registers with the GraphViewport host and activates by id —
// nothing else. No graph data, no layout, no projection, no
// Cytoscape, no backend.
//
// The shell proves the strategic question D35i identified:
//   • A third graph renderer plugs into GraphViewport via
//     `register('knowledge-graph', factory)` + `activateById(
//     'knowledge-graph')` with ZERO changes to graph-viewport.js.
//
// What the shell exposes:
//   /explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js
//   /explorer/assets/css/knowledge-graph.css
//   window.MIDASExplorerGraph.knowledgeGraphShell.activate()
//   window.MIDASExplorerGraph.knowledgeGraphShell.deactivate()
//   window.MIDASExplorerGraph.knowledgeGraphShell.getMountEl()
//
// Tests below pin:
//   • Module + CSS are served and wired into index.html.
//   • Shell registers with GraphViewport at module init.
//   • Activation goes through viewport.activateById (not direct
//     `vp.activate(id, factory)`).
//   • Factory contract is `{ mount(slotEl, ctx) → { destroy() } }`.
//   • Mount creates `.knowledge-graph-mount` inside slotEl.
//   • Resize subscription uses `ctx.onResize` and is unsubscribed
//     in destroy.
//   • Destroy removes only renderer-owned DOM (idempotent).
//   • No native, Authority, or Context DOM mutation.
//   • No body-class activation, no legacy scroll fallback.
//   • No new backend / schema / OpenAPI / data feature.
//   • No graph-viewport.js change (no Knowledge-specific branch
//     in the host).
//   • Namespaced activation hook works as documented.
//   • Foundation D35 contracts remain valid.
//   • No unexpected renderer domains added.

const (
	d36aKnowledgeAssetPath    = "/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js"
	d36aKnowledgeCSSPath      = "/explorer/assets/css/knowledge-graph.css"
	d36aKnowledgeRendererID   = "'knowledge-graph'"
	d36aKnowledgeMountClass   = "knowledge-graph-mount"
	d36aKnowledgeShellNS      = "window.MIDASExplorerGraph.knowledgeGraphShell"
)

// TestExplorer_D36aKnowledgeShell_ModuleServedAndLinked pins that
// both the JS and CSS assets are served, and that index.html
// includes them in the right place (graph-viewport.js BEFORE the
// shell so vp.register is available at module init).
func TestExplorer_D36aKnowledgeShell_ModuleServedAndLinked(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Assets are served.
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)
	if len(js) < 256 {
		t.Errorf("D36a: Knowledge shell JS is suspiciously small (%d bytes)", len(js))
	}
	css := getExplorerAsset(t, srv, d36aKnowledgeCSSPath)
	if len(css) < 64 {
		t.Errorf("D36a: Knowledge shell CSS is suspiciously small (%d bytes)", len(css))
	}

	// Index.html wires both.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`<script src="/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js">`,
		`<link rel="stylesheet" href="/explorer/assets/css/knowledge-graph.css">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D36a: index.html must include %q", want)
		}
	}

	// Load-order invariant: graph-viewport.js MUST appear before
	// knowledge-graph-renderer.js so vp.register is available when
	// the shell's IIFE runs.
	vpIdx := strings.Index(body, "/explorer/assets/js/graph/graph-viewport.js")
	kgIdx := strings.Index(body, "/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js")
	if vpIdx < 0 || kgIdx < 0 {
		t.Fatal("D36a: both graph-viewport.js and knowledge-graph-renderer.js must appear in index.html")
	}
	if vpIdx >= kgIdx {
		t.Error("D36a: graph-viewport.js must load BEFORE knowledge-graph-renderer.js")
	}
}

// TestExplorer_D36aKnowledgeShell_RegistersWithGraphViewport pins
// that the shell registers its factory under 'knowledge-graph' at
// module init via vp.register.
func TestExplorer_D36aKnowledgeShell_RegistersWithGraphViewport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)

	for _, want := range []string{
		// Registration call.
		"vp.register('knowledge-graph', _knowledgeGraphRendererFactory)",
		// Defensive probe on register before calling.
		"typeof vp.register === 'function'",
		// IIFE-end registration helper (mirrors Authority/Context shape).
		"_registerWithGraphViewport",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D36a: shell must register with GraphViewport — missing %q", want)
		}
	}

	// D36b — Renderer id is sourced defensively from the shared
	// constants module (knowledge-graph-contract.js) and falls
	// back to the literal `'knowledge-graph'` if the contract
	// module did not load. This pins both halves of the migration:
	// the shell consumes the constant, and the fallback literal
	// still appears so the contract id is detectable even if the
	// contract module is missing.
	for _, want := range []string{
		// Defensive consumption of the contract constant.
		"window.MIDASExplorerGraph.knowledgeGraphContract",
		"_contract.KNOWLEDGE_GRAPH_RENDERER_ID",
		// Fallback literal preserved so the renderer id survives a
		// perturbed load order.
		"'knowledge-graph'",
		// RENDERER_ID symbol still declared so the rest of the
		// module (activation helper, registration) can reference it.
		"var RENDERER_ID",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D36a/D36b: shell must consume KNOWLEDGE_GRAPH_RENDERER_ID with a defensive fallback — missing %q", want)
		}
	}
}

// TestExplorer_D36aKnowledgeShell_ActivatesByRegisteredId pins
// that the shell's activation path uses viewport.activateById, NOT
// the pre-D35g direct factory activation pattern.
func TestExplorer_D36aKnowledgeShell_ActivatesByRegisteredId(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)

	// Activation helper calls activateById.
	if !strings.Contains(js, "vp.activateById(RENDERER_ID)") {
		t.Error("D36a: shell activation helper must call vp.activateById(RENDERER_ID)")
	}
	if !strings.Contains(js, "typeof vp.activateById === 'function'") {
		t.Error("D36a: shell must defensively probe vp.activateById availability")
	}

	// Pre-D35g direct factory activation must NOT appear.
	if strings.Contains(js, "vp.activate('knowledge-graph',") {
		t.Error("D36a: shell must NOT pass the factory directly to vp.activate — use register + activateById")
	}
}

// TestExplorer_D36aKnowledgeShell_RendererFactoryLifecycle pins
// the factory contract: `{ mount(slotEl, ctx) → { destroy() } }`.
func TestExplorer_D36aKnowledgeShell_RendererFactoryLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)

	for _, want := range []string{
		"var _knowledgeGraphRendererFactory = {",
		"mount: function (slotEl, ctx) {",
		"destroy: function () {",
		// Exposed on the public surface for tests / diagnostics.
		"_rendererFactory:  _knowledgeGraphRendererFactory",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D36a: factory contract must include %q", want)
		}
	}
}

// TestExplorer_D36aKnowledgeShell_MountsInsideRendererSlot pins
// that mount creates `.knowledge-graph-mount` as a direct child of
// the host-supplied slotEl, and never appends to any legacy
// surface.
func TestExplorer_D36aKnowledgeShell_MountsInsideRendererSlot(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)

	// Bound the assertions to the mount function body so unrelated
	// references elsewhere don't false-match.
	mountStart := strings.Index(js, "mount: function (slotEl, ctx) {")
	if mountStart < 0 {
		t.Fatal("D36a: factory.mount definition not found")
	}
	mountEnd := strings.Index(js[mountStart:], "\n    },")
	if mountEnd < 0 {
		t.Fatal("D36a: cannot bound factory.mount body")
	}
	mountBody := js[mountStart : mountStart+mountEnd]

	for _, want := range []string{
		// Creates the mount element.
		"document.createElement('div')",
		"_mountEl.className = MOUNT_CLASS;",
		// Appends to slotEl (the host slot), NOT to legacy DOM.
		"slotEl.appendChild(_mountEl)",
	} {
		if !strings.Contains(mountBody, want) {
			t.Errorf("D36a: mount must include %q", want)
		}
	}

	// Negative pins — mount must not append to any non-slot surface.
	for _, banned := range []string{
		".governance-map-canvas-scroll",
		"getElementById('gmap-canvas')",
		"getElementById('gmap-scene')",
		"getElementById('gmap-svg')",
	} {
		if strings.Contains(mountBody, banned) {
			t.Errorf("D36a: mount must NOT touch %q (use slotEl only)", banned)
		}
	}

	// The MOUNT_CLASS constant must equal the documented class name
	// so CSS keyed on `.knowledge-graph-mount` actually applies.
	if !strings.Contains(js, "var MOUNT_CLASS = 'knowledge-graph-mount';") {
		t.Error("D36a: MOUNT_CLASS must be declared as 'knowledge-graph-mount'")
	}
}

// TestExplorer_D36aKnowledgeShell_UsesViewportResizeSubscription
// pins that mount subscribes to ctx.onResize and the destroy path
// unsubscribes.
func TestExplorer_D36aKnowledgeShell_UsesViewportResizeSubscription(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)

	for _, want := range []string{
		// Defensive probe on ctx.onResize.
		"typeof _rendererCtx.onResize === 'function'",
		// Stores the unsubscribe for teardown.
		"_rendererResizeUnsub = _rendererCtx.onResize(_onHostResize)",
		// Teardown unsubscribes.
		"if (_rendererResizeUnsub)",
		"_rendererResizeUnsub();",
		"_rendererResizeUnsub = null;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D36a: resize wiring must include %q", want)
		}
	}

	// Negative — must NOT install an independent global resize
	// listener when ctx.onResize is the documented entry point.
	if strings.Contains(js, "window.addEventListener('resize'") {
		t.Error("D36a: shell must use ctx.onResize, not window.addEventListener('resize')")
	}
}

// TestExplorer_D36aKnowledgeShell_DestroyTearsDownOwnedDomOnly
// pins that destroy is idempotent and removes only the shell's own
// `.knowledge-graph-mount`.
func TestExplorer_D36aKnowledgeShell_DestroyTearsDownOwnedDomOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)

	// _teardownResources is the helper destroy delegates to. Bound
	// positive pins on the raw body (the literal lines we want to
	// see), and bound negative pins on the comment-stripped body
	// (so safety comments that NAME the forbidden tokens — to
	// explain what teardown must NOT do — don't false-match).
	tStart := strings.Index(js, "function _teardownResources()")
	if tStart < 0 {
		t.Fatal("D36a: _teardownResources definition not found")
	}
	tEnd := strings.Index(js[tStart:], "\n  }\n")
	if tEnd < 0 {
		t.Fatal("D36a: cannot bound _teardownResources body")
	}
	tBody := js[tStart : tStart+tEnd]
	tExec := stripJSComments(tBody)

	for _, want := range []string{
		// Removes the renderer-owned mount.
		"_mountEl.parentNode.removeChild(_mountEl)",
		// Nulls internal state so a repeated call is a no-op.
		"_mountEl     = null;",
		"_rendererCtx = null;",
		// Unsubscribes resize before removing DOM.
		"_rendererResizeUnsub()",
	} {
		if !strings.Contains(tBody, want) {
			t.Errorf("D36a: _teardownResources must include %q", want)
		}
	}

	// Negative pins — teardown EXECUTABLE code must NOT touch
	// native, Authority, or Context DOM.
	for _, banned := range []string{
		"getElementById('gmap-canvas').remove",
		"getElementById('gmap-scene').remove",
		"getElementById('gmap-svg').remove",
		".cytoscape-poc-mount",
		".context-cy-spike-mount",
		".context-cy-spike-overlay",
		".governance-map-canvas-scroll",
	} {
		if strings.Contains(tExec, banned) {
			t.Errorf("D36a: _teardownResources executable code must NOT touch %q", banned)
		}
	}

	// Destroy handle returned by mount delegates to
	// _teardownResources.
	if !strings.Contains(js, "_teardownResources();") {
		t.Error("D36a: factory.destroy must delegate to _teardownResources()")
	}
}

// TestExplorer_D36aKnowledgeShell_NoNativeAuthorityContextDomMutation
// pins that NOWHERE in the shell module are native, Authority, or
// Context DOM tokens touched.
func TestExplorer_D36aKnowledgeShell_NoNativeAuthorityContextDomMutation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)
	exec := stripJSComments(js)

	// Native DOM tokens — must not be touched anywhere in
	// executable code.
	for _, banned := range []string{
		"#gmap-canvas",
		"#gmap-scene",
		"#gmap-svg",
		"getElementById('gmap-canvas'",
		"getElementById('gmap-scene'",
		"getElementById('gmap-svg'",
		"getElementsByClassName('governance-map-canvas-scroll'",
		// Authority renderer-owned DOM.
		".cytoscape-poc-mount",
		"_authorityRendererFactory",
		// Context renderer-owned DOM.
		".context-cy-spike-mount",
		".context-cy-spike-overlay",
		"_contextCytoscapeRendererFactory",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D36a: shell must NOT reference %q (foreign DOM/state)", banned)
		}
	}
}

// TestExplorer_D36aKnowledgeShell_NoBodyClassOrLegacyFallback pins
// that the shell does not introduce a body class or any of the
// retired activation/fallback patterns.
func TestExplorer_D36aKnowledgeShell_NoBodyClassOrLegacyFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)
	exec := stripJSComments(js)

	// No body classes added/removed by the shell.
	for _, banned := range []string{
		"document.body.classList.add(",
		"document.body.classList.remove(",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D36a: shell must NOT flip body classes — found %q", banned)
		}
	}

	// No body-class identifier markers that would indicate a new
	// activation flag.
	for _, banned := range []string{
		"knowledge-graph-active",
		"BODY_FLAG_CLASS",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D36a: shell must NOT introduce a new body-class activation marker — found %q", banned)
		}
	}

	// No legacy scroll fallback.
	if strings.Contains(exec, "governance-map-canvas-scroll") {
		t.Error("D36a: shell must NOT use legacy `.governance-map-canvas-scroll` fallback")
	}

	// No direct vp.activate(id, factory).
	if strings.Contains(exec, "vp.activate('knowledge-graph',") ||
		strings.Contains(exec, "viewport.activate('knowledge-graph',") {
		t.Error("D36a: shell must NOT call vp.activate(id, factory) directly")
	}
}

// TestExplorer_D36aKnowledgeShell_NoFeatureBackendOrSchemaChanges
// pins that D36a is a SHELL only — no backend endpoint, no
// OpenAPI entry, no schema change, no mock graph data.
func TestExplorer_D36aKnowledgeShell_NoFeatureBackendOrSchemaChanges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// No /v1/graphs/knowledge endpoint.
	for _, p := range []string{
		"/v1/graphs/knowledge",
		"/v1/graphs/knowledge/",
		"/v1/knowledge",
	} {
		resp := performRequest(t, srv, http.MethodGet, p, nil)
		if resp.Code == 200 {
			t.Errorf("D36a: endpoint %s must NOT be served (D36a is a renderer SHELL — no backend)", p)
		}
	}

	// Shell module must not contain a fetch / data path.
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)
	exec := stripJSComments(js)
	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/graphs/knowledge",
		"/v1/knowledge",
		// Mock graph-data structures that would imply a fake graph.
		"nodes: [",
		"edges: [",
		"connectors:",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D36a: shell must NOT contain %q (D36a is a SHELL — no data, no fetch, no mock nodes)", banned)
		}
	}

	// No Cytoscape inside the shell module.
	for _, banned := range []string{
		"cytoscape(",
		"window.cytoscape",
		"require('cytoscape')",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D36a: shell must NOT instantiate Cytoscape — found %q", banned)
		}
	}
}

// TestExplorer_D36aKnowledgeShell_NoGraphViewportHostChange pins
// that graph-viewport.js carries NO Knowledge-specific branch or
// id reference, and that the host's generic register / activateById
// path is unchanged.
func TestExplorer_D36aKnowledgeShell_NoGraphViewportHostChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	// No literal Knowledge id anywhere in the host (in executable
	// code OR comments — knowledge is mentioned ONLY in the
	// general "future graph domains" framing, which uses the
	// human form, not the renderer id literal).
	if strings.Contains(hostJS, "'knowledge-graph'") ||
		strings.Contains(hostJS, `"knowledge-graph"`) {
		t.Error("D36a: graph-viewport.js must NOT contain the literal 'knowledge-graph' id — the host stays renderer-neutral")
	}

	// The host's register / activateById bodies must remain
	// id-agnostic (no hardcoded renderer ids). This is the D35i
	// extensibility contract; D36a re-verifies it after first reuse.
	regStart := strings.Index(hostJS, "function register(rendererId, factory)")
	if regStart < 0 {
		t.Fatal("D36a: register definition not found")
	}
	regEnd := strings.Index(hostJS[regStart:], "\n  }\n")
	if regEnd < 0 {
		t.Fatal("D36a: cannot bound register body")
	}
	regBody := hostJS[regStart : regStart+regEnd]
	if strings.Contains(regBody, "'knowledge-graph'") ||
		strings.Contains(regBody, "'authority'") ||
		strings.Contains(regBody, "'context-cytoscape'") {
		t.Error("D36a: register body must remain id-agnostic — found a hardcoded renderer id")
	}

	aStart := strings.Index(hostJS, "function activateById(rendererId)")
	if aStart < 0 {
		t.Fatal("D36a: activateById definition not found")
	}
	aEnd := strings.Index(hostJS[aStart:], "\n  }\n")
	if aEnd < 0 {
		t.Fatal("D36a: cannot bound activateById body")
	}
	aBody := hostJS[aStart : aStart+aEnd]
	if strings.Contains(aBody, "'knowledge-graph'") ||
		strings.Contains(aBody, "'authority'") ||
		strings.Contains(aBody, "'context-cytoscape'") {
		t.Error("D36a: activateById body must remain id-agnostic — found a hardcoded renderer id")
	}
}

// TestExplorer_D36aKnowledgeShell_KnowledgeActivationHook pins
// the namespaced activation helper. This is the documented entry
// point for browser smoke tests and future renderer dev iteration
// — the disabled header placeholder button is intentionally NOT
// wired (the Knowledge Graph FEATURE isn't implemented; the
// disabled button correctly communicates that).
func TestExplorer_D36aKnowledgeShell_KnowledgeActivationHook(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36aKnowledgeAssetPath)

	// Helper namespaced under window.MIDASExplorerGraph (the
	// established graph-module namespace).
	for _, want := range []string{
		"window.MIDASExplorerGraph.knowledgeGraphShell = {",
		"activate: function ()",
		"deactivate: function ()",
		"getMountEl:",
		"rendererId:        RENDERER_ID,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D36a: namespaced activation helper must include %q", want)
		}
	}

	// Helper's activate() delegates to vp.activateById.
	hStart := strings.Index(js, "activate: function ()")
	if hStart < 0 {
		t.Fatal("D36a: activate helper definition not found")
	}
	hEnd := strings.Index(js[hStart:], "\n    },")
	if hEnd < 0 {
		t.Fatal("D36a: cannot bound activate helper body")
	}
	hBody := js[hStart : hStart+hEnd]
	if !strings.Contains(hBody, "vp.activateById(RENDERER_ID)") {
		t.Error("D36a: activate helper must call vp.activateById(RENDERER_ID)")
	}

	// The header Knowledge placeholder button must remain disabled
	// (D36a does NOT wire it — that would imply the feature exists).
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`data-workbench-mode="knowledge"`,
		`aria-disabled="true"`,
		`disabled`,
		`Knowledge Graph`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D36a: header Knowledge placeholder button must remain disabled — missing %q", want)
		}
	}
}

// TestExplorer_D36a_D35GraphViewportContractsPreserved is the
// foundation-wide regression check. Every D35a–D35i invariant
// must remain intact.
func TestExplorer_D36a_D35GraphViewportContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a — structural DOM tokens.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D36a: D35a structural class %q must remain", want)
		}
	}

	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	// D35b/c — host API + adoption.
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport = {",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
		"function getSafeArea(",
		"function onResize(",
		"function adoptExisting(",
		"function _adoptNativeContextBaseline()",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D36a: D35b/c host API %q must remain", want)
		}
	}

	// D35f — host-owned identity attribute.
	for _, want := range []string{
		`var ACTIVE_RENDERER_ATTR = 'data-active-renderer';`,
		"function _setActiveRendererAttribute(rendererId)",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D36a: D35f identity contract %q must remain", want)
		}
	}

	// D35g — registry surface.
	for _, want := range []string{
		"function register(rendererId, factory)",
		"function unregister(rendererId)",
		"function hasRenderer(rendererId)",
		"function listRegistered()",
		"function activateById(rendererId)",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D36a: D35g registry %q must remain", want)
		}
	}

	// D35d — Authority registers + activates by id.
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	for _, want := range []string{
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
	} {
		if !strings.Contains(authJS, want) {
			t.Errorf("D36a: D35d Authority contract %q must remain", want)
		}
	}

	// D35e — Context registers + activates by id.
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	for _, want := range []string{
		"vp.register('context-cytoscape', _contextCytoscapeRendererFactory)",
		"vp.activateById('context-cytoscape')",
	} {
		if !strings.Contains(ctxJS, want) {
			t.Errorf("D36a: D35e Context contract %q must remain", want)
		}
	}

	// D35f — strategic clip.
	clipCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	if !strings.Contains(stripCSSComments(clipCSS), ".midas-graph-viewport") ||
		!strings.Contains(stripCSSComments(clipCSS), "overflow: hidden") {
		t.Error("D36a: `.midas-graph-viewport { overflow: hidden }` strategic clip must remain (D35f)")
	}

	// D35e — overlay non-clipping.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if strings.Count(stripCSSComments(spikeCSS), "overflow: hidden") != 1 {
		t.Error("D36a: spike CSS must have exactly 1 `overflow: hidden` (mount; overlay non-clipping)")
	}
}

// TestExplorer_D36a_NoUnexpectedRendererDomains sweeps every
// renderer module for vp.register calls and asserts the registered
// id set is exactly {Authority, Context, Knowledge}. Mirrors the
// D35h/D35i discipline but rooted in D36a so a future tranche
// introducing Drift / Resilience / etc. has to update this test
// just like D36a updated the D35h/D35i allow-lists.
func TestExplorer_D36a_NoUnexpectedRendererDomains(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rendererJSAssets := []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js",
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
	}

	allowed := map[string]bool{
		"'authority'": true,
		"'context-cytoscape'":   true,
		"'knowledge-graph'":     true,
	}

	for _, path := range rendererJSAssets {
		js := getExplorerAsset(t, srv, path)
		exec := stripJSComments(js)

		i := 0
		for {
			idx := strings.Index(exec[i:], "vp.register(")
			if idx < 0 {
				break
			}
			start := i + idx + len("vp.register(")
			commaIdx := strings.Index(exec[start:], ",")
			if commaIdx < 0 {
				break
			}
			rawID := strings.TrimSpace(exec[start : start+commaIdx])
			if !allowed[rawID] {
				t.Errorf("D36a: %s registers an unexpected renderer id %s", path, rawID)
			}
			i = start + commaIdx
		}

		// Sweep for out-of-scope domain ids that should not appear
		// until their own tranches introduce them.
		for _, bannedID := range []string{
			"'drift-graph'",
			"'drift-topology'",
			"'resilience-graph'",
			"'resilience-topology'",
			"'service-topology'",
			"'evidence-graph'",
			"'policy-graph'",
		} {
			if strings.Contains(exec, bannedID) {
				t.Errorf("D36a: %s references out-of-scope graph-domain id %s", path, bannedID)
			}
		}
	}
}
