package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37o-impl-2 — Context Strategic Renderer Skeleton tests
//
// Pins the asset presence, identity, activation gate, lifecycle,
// model consumption, coexistence, and naming-fossilisation contracts
// locked in D37o-design-1.
//
// The strategic renderer is OPT-IN: it activates only when the URL
// carries `?contextRenderer=strategic`. The legacy native Context
// renderer remains the default Context renderer until the rollout
// gate (T10), which is NOT this tranche.

const (
	d37oImpl2RendererAsset    = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37oImpl2RendererCSSPath  = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37oImpl2LegacyView       = "/explorer/assets/js/graph/context/context-graph-view.js"
	d37oImpl2LegacyAdapter    = "/explorer/assets/js/graph/context/context-graph-adapter.js"
	d37oImpl2LegacyInspector  = "/explorer/assets/js/graph/context/context-graph-inspector.js"
	d37oImpl2LegacyTray       = "/explorer/assets/js/graph/context/context-evidence-tray.js"
	d37oImpl2DormantSpike     = "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js"
	d37oImpl2AuthorityPoc     = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37oImpl2ViewportHost     = "/explorer/assets/js/graph/graph-viewport.js"
	d37oImpl2CardModelAsset   = "/explorer/assets/js/graph/context/context-card-model.js"
	d37oImpl2ConnModelAsset   = "/explorer/assets/js/graph/context/context-connector-model.js"
	d37oImpl2LayoutModelAsset = "/explorer/assets/js/graph/context/context-layout-model.js"
)

// ── A. Asset presence and loading ────────────────────────────────────

func TestExplorer_D37oImpl2_RendererJsAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)
	if len(js) == 0 {
		t.Fatal("D37o-impl-2: context-cytoscape-renderer.js must be served")
	}
}

func TestExplorer_D37oImpl2_RendererCssAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl2RendererCSSPath)
	if len(css) == 0 {
		t.Fatal("D37o-impl-2: context-cytoscape-renderer.css must be served")
	}
}

// TestExplorer_D37oImpl2_IndexHtmlWiresRendererAssets pins the
// <link> + <script> wiring in index.html.
func TestExplorer_D37oImpl2_IndexHtmlWiresRendererAssets(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`href="/explorer/assets/css/context-cytoscape-renderer.css"`,
		`src="/explorer/assets/js/graph/context/context-cytoscape-renderer.js"`,
		`src="/explorer/assets/js/graph/context/context-card-model.js"`,
		`src="/explorer/assets/js/graph/context/context-connector-model.js"`,
		`src="/explorer/assets/js/graph/context/context-layout-model.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-2: index.html must include %q", want)
		}
	}
}

// TestExplorer_D37oImpl2_ModelsLoadBeforeRenderer pins the load
// order: every model module's <script> tag precedes the renderer's
// <script> tag in the served HTML.
func TestExplorer_D37oImpl2_ModelsLoadBeforeRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	rendererIdx := strings.Index(body, "context-cytoscape-renderer.js")
	if rendererIdx < 0 {
		t.Fatal("D37o-impl-2: renderer script tag missing")
	}

	for _, modelAsset := range []string{
		"context-card-model.js",
		"context-connector-model.js",
		"context-layout-model.js",
	} {
		modelIdx := strings.Index(body, modelAsset)
		if modelIdx < 0 {
			t.Errorf("D37o-impl-2: model module %q must appear in index.html", modelAsset)
			continue
		}
		if modelIdx >= rendererIdx {
			t.Errorf("D37o-impl-2: %q must load BEFORE context-cytoscape-renderer.js", modelAsset)
		}
	}
}

// TestExplorer_D37oImpl2_RendererLoadsAfterViewportHost pins that
// graph-viewport.js loads before the strategic renderer so the host
// API is available at factory-registration time.
func TestExplorer_D37oImpl2_RendererLoadsAfterViewportHost(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	viewportIdx := strings.Index(body, "graph-viewport.js")
	rendererIdx := strings.Index(body, "context-cytoscape-renderer.js")
	if viewportIdx < 0 || rendererIdx < 0 {
		t.Fatal("D37o-impl-2: both graph-viewport.js and context-cytoscape-renderer.js must be wired")
	}
	if viewportIdx >= rendererIdx {
		t.Errorf("D37o-impl-2: graph-viewport.js must load BEFORE context-cytoscape-renderer.js")
	}
}

// ── B. Renderer registration and identity ────────────────────────────

// TestExplorer_D37oImpl2_RegistersAsCanonicalContext pins that the
// renderer registers with the host under the canonical id 'context'.
func TestExplorer_D37oImpl2_RegistersAsCanonicalContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	for _, want := range []string{
		`var RENDERER_ID    = 'context';`,
		`g.viewport.register(RENDERER_ID, _factoryFor())`,
		`g.viewport.activateById(RENDERER_ID)`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-2: renderer must include %q", want)
		}
	}
}

// TestExplorer_D37oImpl2_NoAlternateRendererIds pins that the
// renderer file does NOT register any alternate id (every viewport
// register/activate call uses the constant `RENDERER_ID`, never a
// hard-coded alternative).
func TestExplorer_D37oImpl2_NoAlternateRendererIds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	for _, banned := range []string{
		`viewport.register('context-v2'`,
		`viewport.register("context-v2"`,
		`viewport.register('new-context'`,
		`viewport.register("new-context"`,
		`viewport.register('context-new'`,
		`viewport.register('context-next'`,
		`activateById('context-v2'`,
		`activateById('new-context'`,
		`activateById('context-new'`,
		`activateById('context-next'`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-2: renderer must NOT use alternate renderer id %q", banned)
		}
	}
}

// TestExplorer_D37oImpl2_CssScopedToCanonicalContext pins that every
// CSS rule in the renderer stylesheet is scoped under the canonical
// `[data-active-renderer="context"]` selector — no rules for
// alternate identities.
func TestExplorer_D37oImpl2_CssScopedToCanonicalContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37oImpl2RendererCSSPath)
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
			t.Errorf("D37o-impl-2: every renderer CSS rule must scope under %s — rogue selector %q", prefix, selector)
		}
	}
}

// ── C. Activation gate ───────────────────────────────────────────────

// TestExplorer_D37oImpl2_ActivationGateConstants pins the activation-
// mode constants. The query param + mode words live in their own
// namespace — they must not be the renderer id.
func TestExplorer_D37oImpl2_ActivationGateConstants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	for _, want := range []string{
		`var QUERY_PARAM    = 'contextRenderer';`,
		`var MODE_STRATEGIC = 'strategic';`,
		`var MODE_LEGACY    = 'legacy';`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-2: renderer must define activation constant %q", want)
		}
	}
}

// TestExplorer_D37oImpl2_StrategicActivationGate pins the activation-
// gate behaviour: the renderer only calls `viewport.activateById`
// when the mode equals MODE_STRATEGIC. This test pins the gate
// expression literally.
func TestExplorer_D37oImpl2_StrategicActivationGate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	maybeIdx := strings.Index(js, "function _maybeActivate()")
	if maybeIdx < 0 {
		t.Fatal("D37o-impl-2: _maybeActivate helper missing")
	}
	maybeEnd := strings.Index(js[maybeIdx:], "\n  }\n")
	if maybeEnd < 0 {
		t.Fatal("D37o-impl-2: cannot bound _maybeActivate body")
	}
	body := js[maybeIdx : maybeIdx+maybeEnd]

	for _, want := range []string{
		"_isStrategicMode()",
		"activateById(RENDERER_ID)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-2: _maybeActivate must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37oImpl2_LegacyModeRecognised pins that the activation
// helper distinguishes the legacy mode word explicitly (so consumers
// reading the source can see both branches).
func TestExplorer_D37oImpl2_LegacyModeRecognised(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	if !strings.Contains(js, "function _isLegacyMode()") {
		t.Errorf("D37o-impl-2: renderer must expose _isLegacyMode helper for explicit legacy-mode handling")
	}
	if !strings.Contains(js, "MODE_LEGACY") {
		t.Errorf("D37o-impl-2: renderer must reference MODE_LEGACY constant")
	}
}

// TestExplorer_D37oImpl2_ActivationModeSeparateFromRendererId pins
// that the activation-mode words appear ONLY in the four allowed
// scopes (query-param read, activation-mode helpers, the public
// _constants surface for diagnostics, and comments). They must NOT
// appear in the renderer-id constant, registration calls, mount
// class name, or DOM attribute name.
func TestExplorer_D37oImpl2_ActivationModeSeparateFromRendererId(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	// Renderer id must be literal 'context' (not 'strategic-context' etc.).
	if !strings.Contains(js, `var RENDERER_ID    = 'context';`) {
		t.Errorf("D37o-impl-2: RENDERER_ID must be the canonical 'context' literal")
	}
	// Mount class name must not contain rollout-mode words.
	if !strings.Contains(js, `var MOUNT_CLASS = 'context-renderer-mount';`) {
		t.Errorf("D37o-impl-2: MOUNT_CLASS must use canonical 'context-renderer-mount'")
	}
	for _, banned := range []string{
		`'strategic-context'`,
		`"strategic-context"`,
		`'context-strategic'`,
		`"context-strategic"`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-2: renderer must NOT embed rollout-mode in identity %q", banned)
		}
	}
}

// ── D. Legacy preservation ───────────────────────────────────────────

// TestExplorer_D37oImpl2_LegacyContextRendererUntouched asserts that
// the legacy native Context renderer is still served and still
// exposes its production entry points. D37p-clean-1 retired the dead
// `MIDASExplorerGraph.renderer.register('context', lensImpl)` call;
// the live Context lens entry point is the `contextView` export the
// shell calls via `refreshGovernanceMap` →
// `ExplorerGraph.contextView.renderContextGraph(...)`. The separate
// inspector dispatcher namespace remains alive and unchanged.
func TestExplorer_D37oImpl2_LegacyContextRendererUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	view := getExplorerAsset(t, srv, d37oImpl2LegacyView)
	if !strings.Contains(view, "window.MIDASExplorerGraph.contextView") {
		t.Errorf("D37o-impl-2: legacy context-graph-view.js must still expose its production contextView entry points")
	}
	if !strings.Contains(view, "renderContextGraph") {
		t.Errorf("D37o-impl-2: legacy context-graph-view.js must still expose renderContextGraph")
	}
	// D37p-clean-2 retired the dead inspector dispatcher too.
	if strings.Contains(view, "MIDASExplorerGraph.inspector.register('context', inspectorImpl)") {
		t.Errorf("D37p-clean-2: dead inspector.register('context', inspectorImpl) call must be removed from context-graph-view.js")
	}

	for _, asset := range []string{
		d37oImpl2LegacyAdapter,
		d37oImpl2LegacyInspector,
		d37oImpl2LegacyTray,
	} {
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			t.Errorf("D37o-impl-2: legacy asset %s must still be served", asset)
		}
	}
}

// TestExplorer_D37oImpl2_LegacyFileNotRenamed pins that
// context-graph-view.js was NOT renamed during this tranche.
func TestExplorer_D37oImpl2_LegacyFileNotRenamed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, "context-graph-view.js") {
		t.Errorf("D37o-impl-2: index.html must continue to load context-graph-view.js (no early rename)")
	}
	for _, banned := range []string{
		"context-graph-view-legacy.js",
		"context-graph-view-v2.js",
		"context-graph-view-strategic.js",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37o-impl-2: index.html must NOT load renamed legacy %q", banned)
		}
	}
}

// TestExplorer_D37oImpl2_DrawerAndTrayUntouched is a no-regression
// pin for the right drawer and the bottom evidence tray.
func TestExplorer_D37oImpl2_DrawerAndTrayUntouched(t *testing.T) {
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
			t.Errorf("D37o-impl-2: %q must remain (drawer + tray untouched)", want)
		}
	}
}

// ── E. Dormant spike preservation ────────────────────────────────────

// TestExplorer_D37oImpl2_DormantSpikeUntouched asserts that the
// dormant Context Cytoscape spike is still served and the strategic
// renderer does NOT import or depend on it.
func TestExplorer_D37oImpl2_DormantSpikeUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	spike := getExplorerAsset(t, srv, d37oImpl2DormantSpike)
	if len(spike) == 0 {
		t.Fatal("D37o-impl-2: dormant context-cytoscape-overlay-spike.js must still be served")
	}
	// The spike retains its own renderer identity (context-cytoscape).
	if !strings.Contains(spike, "context-cytoscape") {
		t.Errorf("D37o-impl-2: dormant spike must keep its own identity (context-cytoscape)")
	}

	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)
	if strings.Contains(js, "context-cytoscape-overlay-spike") {
		t.Errorf("D37o-impl-2: strategic renderer must NOT import or reference the dormant spike")
	}
}

// TestExplorer_D37oImpl2_StrategicWinsWhenBothGatesPresent pins the
// source-level guarantee that the strategic renderer's activation
// path runs independently of the spike's URL gates. The strategic
// renderer reads ONLY the `contextRenderer` query parameter; it does
// not check the spike's `cytoscape=1&contextHtmlCards=1` gate. So a
// URL containing both gates flips data-active-renderer to `context`
// before the spike's own install path even runs.
func TestExplorer_D37oImpl2_StrategicWinsWhenBothGatesPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	if strings.Contains(js, "cytoscape=1") || strings.Contains(js, "contextHtmlCards=1") {
		t.Errorf("D37o-impl-2: strategic renderer must NOT consult the dormant spike's URL gates")
	}
	if !strings.Contains(js, "decodeURIComponent(p[0]) === QUERY_PARAM") {
		t.Errorf("D37o-impl-2: strategic renderer must read its own activation-mode query parameter")
	}
}

// ── F. Renderer lifecycle ────────────────────────────────────────────

// TestExplorer_D37oImpl2_FactoryShape pins the factory contract.
func TestExplorer_D37oImpl2_FactoryShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	for _, want := range []string{
		"function _factoryFor()",
		"function _mount(slotEl, ctx)",
		"function _destroy()",
		"mount: function (slotEl, ctx)",
		"destroy: _destroy",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-2: renderer must include factory part %q", want)
		}
	}
}

// TestExplorer_D37oImpl2_DestroyRemovesMountElement pins that destroy
// removes the renderer-owned root from its parent.
func TestExplorer_D37oImpl2_DestroyRemovesMountElement(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	destroyStart := strings.Index(js, "function _destroy()")
	if destroyStart < 0 {
		t.Fatal("D37o-impl-2: _destroy missing")
	}
	destroyEnd := strings.Index(js[destroyStart:], "\n  }\n")
	if destroyEnd < 0 {
		t.Fatal("D37o-impl-2: cannot bound _destroy body")
	}
	body := js[destroyStart : destroyStart+destroyEnd]

	for _, want := range []string{
		"_mountEl.parentNode.removeChild(_mountEl)",
		"_mountEl          = null",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-2: _destroy must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37oImpl2_MountIsScoped pins that mount takes
// ownership only of its own renderer-owned root; it does not query
// or mutate legacy renderer DOM (no #gmap-canvas / #gmap-svg /
// #gmap-scene references).
func TestExplorer_D37oImpl2_MountIsScoped(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	for _, banned := range []string{
		"#gmap-canvas",
		"#gmap-svg",
		"#gmap-scene",
		"gmap-canvas",
		"gmap-svg",
		"gmap-scene",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-2: renderer must NOT reference legacy DOM id %q", banned)
		}
	}
}

// ── G. Model consumption ─────────────────────────────────────────────

// TestExplorer_D37oImpl2_ConsumesModelBuilders pins that the renderer
// reads the D37o-impl-1 model surface and calls each builder.
func TestExplorer_D37oImpl2_ConsumesModelBuilders(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	for _, want := range []string{
		"MIDASExplorerGraph.contextModels",
		"models.card.buildCardsFromProjection",
		"models.connector.buildConnectorsFromProjection",
		"models.layout.buildLayout",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-2: renderer must consume model builder %q", want)
		}
	}
}

// TestExplorer_D37oImpl2_DoesNotDuplicateModelConstants pins that the
// renderer file does NOT redefine model vocabularies (node kinds /
// edge kinds / visual classes / band ids). Those are owned by the
// model modules; the renderer references them via the model surface.
func TestExplorer_D37oImpl2_DoesNotDuplicateModelConstants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	for _, banned := range []string{
		"var NODE_KINDS",
		"var EDGE_KINDS",
		"var VISUAL_CLASSES",
		"var BAND_IDS",
		"var BADGE_CLASSES",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-2: renderer must NOT redefine model constant %q (owned by model modules)", banned)
		}
	}
}

// TestExplorer_D37oImpl2_PublicDiagnosticsSurface pins the compact
// diagnostic surface (window.MIDASExplorerGraph.contextRenderer).
func TestExplorer_D37oImpl2_PublicDiagnosticsSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	declStart := strings.Index(js, "window.MIDASExplorerGraph.contextRenderer = {")
	if declStart < 0 {
		t.Fatal("D37o-impl-2: public surface registration missing")
	}
	declEnd := strings.Index(js[declStart:], "};")
	if declEnd < 0 {
		t.Fatal("D37o-impl-2: cannot bound public surface declaration")
	}
	block := js[declStart : declStart+declEnd]

	for _, want := range []string{
		"isAvailable:",
		"isActive:",
		"getLastModelSummary:",
		"_constants:",
		"RENDERER_ID:",
		"QUERY_PARAM:",
		"MODE_STRATEGIC:",
		"MODE_LEGACY:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37o-impl-2: diagnostic surface must expose %q", want)
		}
	}
}

// ── H. Naming-fossilisation guardrails ───────────────────────────────

// TestExplorer_D37oImpl2_NoDurableTemporaryNames pins that no
// durable temporary renderer-identity names appear in the new JS or
// CSS files.
func TestExplorer_D37oImpl2_NoDurableTemporaryNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{d37oImpl2RendererAsset, d37oImpl2RendererCSSPath} {
		body := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"context-v2",
			"context-strategic",
			"new-context",
			"context-new",
			"context-next",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D37o-impl-2: %s must NOT contain temporary renderer name %q", asset, banned)
			}
		}
	}
}

// TestExplorer_D37oImpl2_NoLegacyPrimitivesOrDrawerSetters pins that
// the renderer does not call legacy graph-renderer primitives or
// drawer setters as render input.
func TestExplorer_D37oImpl2_NoLegacyPrimitivesOrDrawerSetters(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl2RendererAsset)

	for _, banned := range []string{
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
			t.Errorf("D37o-impl-2: renderer must NOT use forbidden symbol %q", banned)
		}
	}
}

// ── I. Authority + viewport host untouched ───────────────────────────

func TestExplorer_D37oImpl2_AuthorityAndViewportUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	authority := getExplorerAsset(t, srv, d37oImpl2AuthorityPoc)
	if !strings.Contains(authority, `vp.register('authority', _authorityRendererFactory)`) {
		t.Errorf("D37o-impl-2: Authority renderer factory registration must remain")
	}

	vp := getExplorerAsset(t, srv, d37oImpl2ViewportHost)
	if !strings.Contains(vp, "ACTIVE_RENDERER_ATTR") {
		t.Errorf("D37o-impl-2: GraphViewport active-renderer-attribute constant must remain")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37o-impl-2: GraphViewport native-context baseline adoption must remain")
	}
}
