package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D32a-impl-1 — Explorer Shell, API Client Layer, Router, Store, and Graph
// Lens Shell.
//
// This tranche promotes assets/js/api.js from a single auth-header
// primitive into a structured API client; adds core/config.js,
// core/router.js, core/store.js; and adds a lens-agnostic Graph shell
// with a Context lens adapter alongside the production Governance
// Map renderer. The Authority lens UI is intentionally absent (the
// switcher button is disabled with a "coming next" affordance) —
// implementing the Authority Graph UI is a separate tranche.
//
// Tests in this file pin:
//   • Each new JS file is served with a JS content-type and 200 OK.
//   • Each new JS file establishes its expected window.MIDAS* namespace.
//   • The API client surface (graphs.context, graphs.authority, and
//     the seven endpoint groups required by the brief) is declared.
//   • The new <script src> tags appear AFTER state.js and BEFORE
//     the existing governance-map and records scripts so namespaces
//     resolve in dependency order.
//   • All new <script src> tags are plain (no type=module / defer /
//     async), matching the D27j-ui-foundation-{3..6} convention.
//   • The two <meta name="midas-*"> tags are present in <head>.
//   • The lens switcher renders inside the services-map-view-header,
//     with Context enabled and Authority disabled.
//   • The inline IIFE binds the new namespaces to ExplorerConfig /
//     ExplorerRouter / ExplorerStore / ExplorerGraph locals.
//   • The inline IIFE no longer issues hand-built fetch() calls to
//     /v1/graphs/context — those two call-sites now route through
//     ExplorerAPI.graphs.context.
//   • A #graph/context hash is honoured by the inline view router
//     (and a corresponding handler is registered with the new
//     router for future migration).
//   • The Authority graph endpoint is NOT directly called from any
//     active UI path — the only reference is the client method.
// ---------------------------------------------------------------------------

// d32aJSFiles enumerates the new JS files this tranche introduces.
// Each entry is { path, namespace-substring, additional-substring }.
// The additional-substring is an optional second pin used when the
// namespace alone is not unique enough to ensure the file shipped.
var d32aJSFiles = []struct {
	path     string
	ns       string
	contains []string
}{
	{
		path: "/explorer/assets/js/core/api-client.js",
		ns:   "window.MIDASExplorerAPI",
		contains: []string{
			"window.MIDASExplorerAPI.configure",
			"window.MIDASExplorerAPI.getConfig",
			"window.MIDASExplorerAPI.request",
			"window.MIDASExplorerAPI.graphs",
			"window.MIDASExplorerAPI.businessServices",
			"window.MIDASExplorerAPI.capabilities",
			"window.MIDASExplorerAPI.drift",
			"window.MIDASExplorerAPI.evidence",
			"window.MIDASExplorerAPI.escalationTargets",
			"window.MIDASExplorerAPI.failModePolicies",
			"/v1/graphs/context",
			"/v1/graphs/authority",
		},
	},
	{
		path: "/explorer/assets/js/core/config.js",
		ns:   "window.MIDASExplorerConfig",
		contains: []string{
			`'midas-api-base'`,
			`'midas-auth-mode'`,
		},
	},
	{
		path: "/explorer/assets/js/core/router.js",
		ns:   "window.MIDASExplorerRouter",
		contains: []string{
			"function register(",
			"function start(",
			"function navigate(",
			"function current(",
		},
	},
	{
		path: "/explorer/assets/js/core/store.js",
		ns:   "window.MIDASExplorerStore",
		contains: []string{
			"function getState(",
			"function setState(",
			"function subscribe(",
			"function reset(",
			"selectedGraphLens",
			"graphDataByLens",
		},
	},
	{
		path: "/explorer/assets/js/graph/graph-types.js",
		ns:   "window.MIDASExplorerGraph",
		contains: []string{
			"AUTHORITY_NODE_KINDS",
			"AUTHORITY_EDGE_KINDS",
			"CONTEXT_NODE_KINDS",
		},
	},
	{
		path: "/explorer/assets/js/graph/graph-layout.js",
		ns:   "window.MIDASExplorerGraph",
		contains: []string{
			"function layoutByRows(",
			"function bbox(",
		},
	},
	{
		path: "/explorer/assets/js/graph/graph-renderer.js",
		ns:   "window.MIDASExplorerGraph",
		contains: []string{
			"function register(",
			"function render(",
			"function clear(",
		},
	},
	{
		path: "/explorer/assets/js/graph/graph-inspector.js",
		ns:   "window.MIDASExplorerGraph",
		contains: []string{
			"function renderNode(",
		},
	},
	{
		path: "/explorer/assets/js/graph/graph-shell.js",
		ns:   "window.MIDASExplorerGraph",
		contains: []string{
			"function init(",
			"function setActiveLens(",
			"function getActiveLens(",
			"data-lens",
		},
	},
	{
		path: "/explorer/assets/js/graph/context/context-graph-adapter.js",
		ns:   "window.MIDASExplorerGraph",
		contains: []string{
			"contextAdapter",
			"function fetchContextGraph(",
		},
	},
	{
		path: "/explorer/assets/js/graph/context/context-graph-view.js",
		ns:   "window.MIDASExplorerGraph",
		contains: []string{
			"renderContextGraph",
			"renderContextGraphEmpty",
			"renderContextGraphError",
			"contextView",
		},
	},
}

// TestExplorer_D32a_NewJSFiles_Served checks every new JS file ships,
// has a JS content-type, and contains its namespace and key tokens.
func TestExplorer_D32a_NewJSFiles_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, f := range d32aJSFiles {
		f := f
		t.Run(f.path, func(t *testing.T) {
			rec := performRequest(t, srv, http.MethodGet, f.path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
				t.Errorf("want JavaScript Content-Type, got %q", ct)
			}
			body := rec.Body.String()
			if !strings.Contains(body, f.ns) {
				t.Errorf("%s: missing namespace reference %q", f.path, f.ns)
			}
			for _, want := range f.contains {
				if !strings.Contains(body, want) {
					t.Errorf("%s: missing required token %q", f.path, want)
				}
			}
		})
	}
}

// TestExplorer_D32a_APIClient_GraphsAuthorityNotCalledByUI confirms
// that no active UI path calls /v1/graphs/authority directly — the
// only reference must be inside the API client method itself. The
// Authority Graph UI ships in a subsequent tranche; today the lens
// switcher's Authority button is disabled.
func TestExplorer_D32a_APIClient_GraphsAuthorityNotCalledByUI(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	// The API client may (and must) declare the URL. Every other
	// surface must NOT contain it. Checking the inline HTML body
	// + every other JS file gives the strongest guarantee with the
	// fewest false positives.
	bodyHTML := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if strings.Contains(bodyHTML, "/v1/graphs/authority") {
		t.Errorf("index.html must not call /v1/graphs/authority — Authority lens UI is not implemented in this tranche")
	}
	// Every JS file except api-client.js must be authority-free.
	for _, f := range d32aJSFiles {
		if f.path == "/explorer/assets/js/core/api-client.js" {
			continue
		}
		body := performRequest(t, srv, http.MethodGet, f.path, nil).Body.String()
		if strings.Contains(body, "/v1/graphs/authority") {
			t.Errorf("%s: must not reference /v1/graphs/authority — only the API client may declare the URL", f.path)
		}
	}
}

// TestExplorer_D32a_HTML_LinksNewScripts pins the new <script src>
// tags load in the right order (after state.js, before the
// governance-map foundation scripts) and that the markup uses plain
// <script src=…> with no type=module / defer / async.
func TestExplorer_D32a_HTML_LinksNewScripts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	wantOrdered := []string{
		`<script src="/explorer/assets/js/api.js"></script>`,
		`<script src="/explorer/assets/js/state.js"></script>`,
		`<script src="/explorer/assets/js/core/api-client.js"></script>`,
		`<script src="/explorer/assets/js/core/config.js"></script>`,
		`<script src="/explorer/assets/js/core/router.js"></script>`,
		`<script src="/explorer/assets/js/core/store.js"></script>`,
		`<script src="/explorer/assets/js/graph/graph-types.js"></script>`,
		`<script src="/explorer/assets/js/graph/graph-layout.js"></script>`,
		`<script src="/explorer/assets/js/graph/graph-renderer.js"></script>`,
		`<script src="/explorer/assets/js/graph/graph-inspector.js"></script>`,
		`<script src="/explorer/assets/js/graph/graph-shell.js"></script>`,
		`<script src="/explorer/assets/js/graph/context/context-graph-adapter.js"></script>`,
		`<script src="/explorer/assets/js/graph/context/context-graph-view.js"></script>`,
		`<script src="/explorer/assets/js/governance-map/constants.js"></script>`,
	}
	prevIdx := -1
	for _, want := range wantOrdered {
		idx := strings.Index(body, want)
		if idx < 0 {
			t.Errorf("missing JS <script> tag: %q", want)
			continue
		}
		if idx <= prevIdx {
			t.Errorf("script tag out of load order: %q at idx=%d prev=%d", want, idx, prevIdx)
		}
		prevIdx = idx
	}
	// Negative pins — new files must use plain <script src=…>.
	negativeFiles := []string{
		"core/api-client.js",
		"core/config.js",
		"core/router.js",
		"core/store.js",
		"graph/graph-types.js",
		"graph/graph-layout.js",
		"graph/graph-renderer.js",
		"graph/graph-inspector.js",
		"graph/graph-shell.js",
		"graph/context/context-graph-adapter.js",
		"graph/context/context-graph-view.js",
	}
	for _, f := range negativeFiles {
		for _, forbidden := range []string{
			`<script type="module" src="/explorer/assets/js/` + f + `"`,
			`<script defer src="/explorer/assets/js/` + f + `"`,
			`<script async src="/explorer/assets/js/` + f + `"`,
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("D32a-impl-1: %s must use plain <script src=…>; found %q", f, forbidden)
			}
		}
	}
}

// TestExplorer_D32a_HTML_MetaTagsPresent pins the two runtime-config
// meta tags. core/config.js reads them; without them the API client
// falls back to its hard-coded defaults but the meta tags must be
// present so operators can rewrite content attributes if needed.
func TestExplorer_D32a_HTML_MetaTagsPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`<meta name="midas-api-base" content="">`,
		`<meta name="midas-auth-mode" content="cookie">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing runtime-config meta tag: %q", want)
		}
	}
}

// TestExplorer_D32a_HTML_LensSwitcher pins the lens switcher markup
// inside services-map-view-header, the Context button being active,
// and the Authority button being disabled with a "coming next" hover
// affordance.
func TestExplorer_D32a_HTML_LensSwitcher(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	headerIdx := strings.Index(body, `class="services-map-view-header"`)
	if headerIdx < 0 {
		t.Fatal("missing services-map-view-header (existing pin)")
	}
	// The switcher must appear inside / after the header opener but
	// before the .governance-map-workbench. We grep for the markers
	// in body order.
	switcherIdx := strings.Index(body, `class="graph-lens-switcher"`)
	workbenchIdx := strings.Index(body, `class="governance-map-workbench"`)
	if switcherIdx < 0 {
		t.Fatal("missing .graph-lens-switcher")
	}
	if switcherIdx < headerIdx {
		t.Errorf("lens switcher must appear after the services-map-view-header opener; switcherIdx=%d headerIdx=%d", switcherIdx, headerIdx)
	}
	if workbenchIdx > 0 && switcherIdx > workbenchIdx {
		t.Errorf("lens switcher must appear before the governance-map-workbench; switcherIdx=%d workbenchIdx=%d", switcherIdx, workbenchIdx)
	}

	// Context button — active, data-lens="context".
	if !strings.Contains(body, `data-lens="context"`) {
		t.Error("missing Context lens button (data-lens=\"context\")")
	}
	if !strings.Contains(body, `class="graph-lens-button is-active" data-lens="context"`) {
		t.Error("Context lens button must be active by default")
	}

	// Authority button — disabled, data-lens="authority", aria-disabled="true".
	if !strings.Contains(body, `data-lens="authority"`) {
		t.Error("missing Authority lens button (data-lens=\"authority\")")
	}
	if !strings.Contains(body, `class="graph-lens-button is-disabled" data-lens="authority"`) {
		t.Error("Authority lens button must be visibly disabled")
	}
	if !strings.Contains(body, `aria-disabled="true"`) {
		t.Error("Authority lens button must declare aria-disabled=\"true\"")
	}
	if !strings.Contains(body, "Authority lens — coming next") {
		t.Error("missing 'Authority lens — coming next' affordance")
	}

	// Lens switcher CSS must ship in the cascade (any file is fine —
	// the conceptual stylesheet is what matters).
	allCSS := getExplorerAllCSS(t, srv)
	for _, want := range []string{
		".graph-lens-switcher",
		".graph-lens-button",
		".graph-lens-button.is-active",
		".graph-lens-button.is-disabled",
	} {
		if !strings.Contains(allCSS, want) {
			t.Errorf("missing CSS rule for %q", want)
		}
	}
}

// TestExplorer_D32a_HTML_InlineBindings_NewNamespaces pins the inline
// IIFE binding the four new namespaces to local consts so the lens-
// switcher init + future migrations resolve without a window lookup
// at every call-site.
func TestExplorer_D32a_HTML_InlineBindings_NewNamespaces(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`const ExplorerConfig = window.MIDASExplorerConfig || {};`,
		`const ExplorerRouter = window.MIDASExplorerRouter || {};`,
		`const ExplorerStore  = window.MIDASExplorerStore  || {};`,
		`const ExplorerGraph  = window.MIDASExplorerGraph  || {};`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must contain namespace binding %q", want)
		}
	}
}

// TestExplorer_D32a_HTML_GraphFetchesGoThroughShellAndAPIClient pins
// the indirection chain established by D32a-impl-2: the inline IIFE
// dispatches graph fetches through ExplorerGraph.shell.refresh,
// which delegates to the Context Graph adapter, which calls
// ExplorerAPI.graphs.context. Direct fetch() with a hand-built
// /v1/graphs/context URL must not appear in any active UI surface.
func TestExplorer_D32a_HTML_GraphFetchesGoThroughShellAndAPIClient(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Positive: at least two ExplorerGraph.shell.refresh call-sites
	// in the inline IIFE (loadBusinessServiceRecord +
	// refreshGovernanceMap). D32a-impl-2 makes the shell the
	// production entry point for Context Graph orchestration.
	if n := strings.Count(body, "ExplorerGraph.shell.refresh"); n < 2 {
		t.Errorf("D32a-impl-2: expected ≥2 ExplorerGraph.shell.refresh call-sites in inline IIFE; got %d", n)
	}

	// The shell module must invoke the adapter, and the adapter
	// module must invoke the API client.
	shellJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-shell.js")
	if !strings.Contains(shellJS, "adapter.fetch(") {
		t.Error("D32a-impl-2: graph-shell.js must call adapter.fetch in refresh()")
	}
	if !strings.Contains(shellJS, "adapter.mapToCardLayout(") {
		t.Error("D32a-impl-2: graph-shell.js must call adapter.mapToCardLayout in refresh()")
	}
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-adapter.js")
	if !strings.Contains(adapterJS, "api.context(") {
		t.Error("D32a-impl-2: context-graph-adapter.js fetch() must call api.context (which dispatches to ExplorerAPI.graphs.context)")
	}

	// Negative pins — the legacy URL composition must not return.
	for _, gone := range []string{
		`const url = '/v1/graphs/context?view=' + encodeURIComponent(currentGraphView) +`,
		`const url = '/v1/graphs/context?view=' + encodeURIComponent(fetchView) +`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("inline IIFE must not build /v1/graphs/context URLs by hand: %q", gone)
		}
	}
}

// TestExplorer_D32a_HTML_HashRoute_GraphContext pins that #graph/context
// is recognised by the inline view router (mapped to the services
// view) and that the new router has a registered handler for it.
func TestExplorer_D32a_HTML_HashRoute_GraphContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`'graph/context'`,
		`'graph/authority'`,
		`ExplorerRouter.register('graph/context'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing graph hash-route wiring: %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// D32a-impl-2 — Graph Renderer Migration and Lens Shell Activation
//
// Tests in this section pin the ownership boundaries established by
// D32a-impl-2:
//
//   • Context Graph adapter owns the Context-specific shape mapping
//     (mapToCardLayout was moved from the inline IIFE into the
//     adapter module).
//   • Graph shell owns the production fetch+dispatch entry point
//     (shell.refresh dispatches through the adapter and bridge).
//   • Graph renderer is lens-agnostic — no Context-specific node
//     kind strings hard-coded in graph-renderer.js.
//   • Inline IIFE retains the rendering primitives (renderGovernanceMap,
//     renderGovernanceMapEmpty, renderGovernanceMapError, …)
//     because explorer_test.go pins their inline location AND body
//     content; the inline declarations are now formally compatibility
//     shims, published on window.MIDASExplorerGovernanceMapBridge for
//     the shell to dispatch through.
//   • Legacy endpoint URLs remain absent from active UI code.
//   • Authority lens UI remains unimplemented (no UI path calls
//     /v1/graphs/authority).
// ---------------------------------------------------------------------------

// TestExplorer_D32aImpl2_ContextAdapter_OwnsMapToCardLayout pins
// that the Context Graph adapter module declares mapToCardLayout as
// its public surface and that the inline IIFE retains only a thin
// shim (no business_service / ai_system_binding / coverage /
// authority_summary mapping branches inside the inline body).
func TestExplorer_D32aImpl2_ContextAdapter_OwnsMapToCardLayout(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-adapter.js")
	// Adapter must declare the production implementation.
	for _, want := range []string{
		"function mapToCardLayout(projection, view)",
		// All node-kind branches must live in the adapter.
		"case 'business_service':",
		"case 'related_business_service':",
		"case 'capability':",
		"case 'process':",
		"case 'decision_surface':",
		"case 'ai_system':",
		"case 'ai_system_binding':",
		"case 'coverage':",
		"case 'authority_summary':",
		"mapToCardLayout:      mapToCardLayout,",
	} {
		if !strings.Contains(adapterJS, want) {
			t.Errorf("context-graph-adapter.js missing required production token: %q", want)
		}
	}

	// Inline IIFE must retain only the compatibility shim — none
	// of the per-kind switch branches should still be inline.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	startIdx := strings.Index(body, "function mapContextGraphToCardLayout(projection, view)")
	if startIdx < 0 {
		t.Fatal("inline IIFE must retain mapContextGraphToCardLayout shim declaration")
	}
	// The shim is short; bound the inspected slice to 2.5KB which is
	// generous for the compat wrapper without spilling into siblings.
	endIdx := startIdx + 2500
	if endIdx > len(body) {
		endIdx = len(body)
	}
	shimSlice := body[startIdx:endIdx]
	for _, gone := range []string{
		"case 'related_business_service':",
		"case 'ai_system_binding':",
		"case 'authority_summary':",
		"collectedBindings.push({",
	} {
		if strings.Contains(shimSlice, gone) {
			t.Errorf("D32a-impl-2: inline mapContextGraphToCardLayout shim must NOT contain %q (moved to adapter)", gone)
		}
	}
	// The shim must delegate to the adapter.
	if !strings.Contains(shimSlice, "contextAdapter") || !strings.Contains(shimSlice, ".mapToCardLayout(") {
		t.Error("D32a-impl-2: inline mapContextGraphToCardLayout shim must delegate to contextAdapter.mapToCardLayout")
	}
}

// TestExplorer_D32aImpl2_GraphShell_IsProductionEntryPoint pins
// that ExplorerGraph.shell.refresh is the production entry point
// for Context Graph orchestration. Both inline graph-fetch
// call-sites (loadBusinessServiceRecord + refreshGovernanceMap)
// must dispatch through it.
func TestExplorer_D32aImpl2_GraphShell_IsProductionEntryPoint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Both call-sites should pass lens, view, id, depth.
	for _, want := range []string{
		"ExplorerGraph.shell.refresh({",
		"lens:  'context',",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32a-impl-2: inline IIFE must contain %q (graph shell production entry point)", want)
		}
	}

	// Shell module surface must declare refresh + render.
	shellJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-shell.js")
	for _, want := range []string{
		"function refresh(",
		"function render(",
		"window.MIDASExplorerGraph.shell",
		"shell.refresh",
		// Store integration — shell mirrors loading / error / data
		// state onto MIDASExplorerStore so future state migrations
		// observe a consistent view.
		"loadingByKey",
		"errorByKey",
		"graphDataByLens",
	} {
		if !strings.Contains(shellJS, want) {
			t.Errorf("graph-shell.js missing required token: %q", want)
		}
	}
}

// TestExplorer_D32aImpl2_Renderer_IsLensAgnostic pins that
// graph-renderer.js contains no Context-specific node kinds — those
// belong in the adapter. The renderer may know generic concepts
// (node, edge, connector, label) but not governance semantics.
func TestExplorer_D32aImpl2_Renderer_IsLensAgnostic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rendererJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	// Negative pins: Context Graph node-kind strings must not be
	// hard-coded here. (The 'context' string identifies the lens —
	// it's checked by name in case statements; we exclude that.)
	forbidden := []string{
		`'business_service'`,
		`'related_business_service'`,
		`'capability'`,
		`'process'`,
		`'decision_surface'`,
		`'ai_system'`,
		`'ai_system_binding'`,
		`'coverage'`,
		`'authority_summary'`,
	}
	for _, f := range forbidden {
		if strings.Contains(rendererJS, f) {
			t.Errorf("graph-renderer.js must not hard-code Context node kind %q (belongs in adapter)", f)
		}
	}

	// Positive pins: lens-agnostic surface.
	for _, want := range []string{
		"function register(",
		"function render(",
		"function clear(",
		"lensAgnosticConnectorPath",
		"lensAgnosticNodePosition",
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("graph-renderer.js missing required lens-agnostic export: %q", want)
		}
	}
}

// TestExplorer_D32aImpl2_Bridge_WiredFromInlineIIFE pins that the
// inline IIFE registers its rendering primitives on
// MIDASExplorerGovernanceMapBridge — the documented compatibility
// seam that the shell + Context view module dispatch through. The
// bridge is the load-bearing production-path mechanism while the
// rendering primitives themselves remain inline.
func TestExplorer_D32aImpl2_Bridge_WiredFromInlineIIFE(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// D32a-impl-3 — the bridge is now a compatibility alias only.
	// The inline shim functions (renderGovernanceMap,
	// renderGovernanceMapEmpty, renderGovernanceMapError) delegate to
	// the module renderer; the bridge points at those shims so any
	// legacy listener still resolves. The production primitives are
	// not assigned to the bridge directly — they live in the
	// renderer / context-view modules.
	for _, want := range []string{
		"window.MIDASExplorerGovernanceMapBridge",
		".renderGovernanceMap        = renderGovernanceMap;",
		".renderGovernanceMapEmpty   = renderGovernanceMapEmpty;",
		".renderGovernanceMapError   = renderGovernanceMapError;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32a-impl-3: bridge compat-alias must expose %q (compat surface only)", want)
		}
	}
	// The bridge wiring comment must declare the compatibility-alias
	// status so future readers know it no longer owns rendering.
	if !strings.Contains(body, "compatibility alias") {
		t.Error("D32a-impl-3: bridge wiring must be annotated as a compatibility alias (production rendering moved to modules)")
	}
}

// TestExplorer_D32aImpl2_NoActiveUI_Authority pins that the
// Authority Graph endpoint is referenced ONLY in the API client
// declaration. Every other JS module and the inline HTML body must
// be Authority-fetch-free in this tranche.
func TestExplorer_D32aImpl2_NoActiveUI_Authority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Inline HTML body — no Authority URL.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if strings.Contains(body, "/v1/graphs/authority") {
		t.Error("D32a-impl-2: index.html must not reference /v1/graphs/authority (Authority lens UI remains disabled)")
	}
	if strings.Contains(body, "ExplorerAPI.graphs.authority(") {
		t.Error("D32a-impl-2: no active UI path must call ExplorerAPI.graphs.authority()")
	}

	// Every JS module except api-client.js must be Authority-free.
	for _, path := range []string{
		"/explorer/assets/js/core/config.js",
		"/explorer/assets/js/core/router.js",
		"/explorer/assets/js/core/store.js",
		"/explorer/assets/js/graph/graph-types.js",
		"/explorer/assets/js/graph/graph-layout.js",
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-inspector.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
	} {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, "/v1/graphs/authority") {
			t.Errorf("D32a-impl-2: %s must not reference /v1/graphs/authority", path)
		}
		if strings.Contains(js, "ExplorerAPI.graphs.authority(") || strings.Contains(js, "api.authority(") {
			t.Errorf("D32a-impl-2: %s must not call the Authority graph endpoint", path)
		}
	}
}

// TestExplorer_D32aImpl2_LegacyEndpointsAbsent pins that the
// pre-D31d endpoint URLs do not return anywhere in the Explorer
// shell or its JS modules. The endpoints were renamed in D31d:
//   /v1/authority-graph                 → /v1/graphs/context
//   /v1/businessservices/{id}/governance-map → /v1/graphs/context
func TestExplorer_D32aImpl2_LegacyEndpointsAbsent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// /v1/authority-graph — D31d renamed to /v1/graphs/context.
	// /v1/businessservices/{id}/governance-map — older legacy URL.
	// The literal "governance-map" alone is also used for the CSS
	// filename (/explorer/assets/css/governance-map.css) and class
	// names; the legacy-URL pins are the precise check.
	for _, gone := range []string{
		"/v1/authority-graph",
		"/governance-map?",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32a-impl-2: legacy endpoint reference %q must remain absent from index.html", gone)
		}
	}
	for _, path := range []string{
		"/explorer/assets/js/core/api-client.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
		"/explorer/assets/js/graph/graph-shell.js",
	} {
		js := getExplorerAsset(t, srv, path)
		for _, gone := range []string{
			"/v1/authority-graph",
			"/governance-map?",
		} {
			if strings.Contains(js, gone) {
				t.Errorf("D32a-impl-2: legacy endpoint reference %q must remain absent from %s", gone, path)
			}
		}
	}
}

// TestExplorer_D32aImpl2_LegacyRoutesPreserved confirms the
// pre-existing top-level Explorer routes still parse through the
// inline view router. The lens-switcher and #graph/* additions
// must not displace #services / #capabilities / #evaluate /
// #records / #drift / #settings.
func TestExplorer_D32aImpl2_LegacyRoutesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// VALID_VIEWS array (inline router source of truth).
	if !strings.Contains(body, "const VALID_VIEWS = ['services', 'capabilities', 'evaluate', 'records', 'settings']") {
		t.Error("D32a-impl-2: inline VALID_VIEWS array must remain intact (legacy top-level routes)")
	}
	// nav items for each route must remain present (data-nav-view
	// drives the sidebar click handlers).
	for _, view := range []string{"services", "capabilities", "evaluate", "records", "settings"} {
		if !strings.Contains(body, `data-nav-view="`+view+`"`) {
			t.Errorf("D32a-impl-2: sidebar nav for %q must remain (data-nav-view=\"%s\")", view, view)
		}
	}
}

// ---------------------------------------------------------------------------
// D32a-impl-3 — Governance Map Renderer Extraction and Bridge Retirement
//
// Tests in this section pin the architectural end-state established
// by D32a-impl-3:
//
//   • The production Context Graph renderer cluster has been extracted
//     from the inline IIFE into module files. The inline IIFE retains
//     thin compatibility shims; the implementations are module-owned.
//   • The graph renderer module owns lens-agnostic node + connector +
//     filter primitives (addNode, addLiveConnector, addConnector,
//     addConnectorHitTarget, effectiveGmapPosition, addMoreNode,
//     applyVisibilityFilters, clearCanvas).
//   • The Context Graph view module owns the Context-lens renderer
//     and empty/error states (renderContextGraph,
//     renderContextGraphEmpty, renderContextGraphError).
//   • The bridge namespace (MIDASExplorerGovernanceMapBridge) is a
//     compatibility alias; it no longer owns rendering.
//   • Renderer hooks (selectNode / applyMultiSelection /
//     attachDragHandlers / connectorEndpointLabel / hideConnectorTooltip
//     / getSelectedId / clearSelection / clearInspector) are published
//     on window.MIDASExplorerGraph._rendererHooks so the moved
//     renderer can call back into the still-inline orchestration.
//   • Shared state objects (positions / dragOverrides / connectors /
//     selectedNodeIds / visibilityFilters) live on
//     window.MIDASExplorerGraph.state.
//   • Behavioural contracts that previously pinned inline body
//     content are preserved through assertions on the conceptual JS
//     surface (getExplorerAllJS).
// ---------------------------------------------------------------------------

// TestExplorer_D32aImpl3_RendererOwnsProductionPrimitives pins that
// the lens-agnostic renderer module exports the production
// implementations of every primitive that previously lived inline.
func TestExplorer_D32aImpl3_RendererOwnsProductionPrimitives(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rendererJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	for _, want := range []string{
		"function clearCanvas(",
		"function addNode(spec, pos)",
		"function addConnector(p1, p2, cls)",
		"function addConnectorHitTarget(",
		"function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls)",
		"function addMoreNode(",
		"function effectiveGmapPosition(id)",
		"function applyVisibilityFilters(",
		// Surface mapping — these names must appear on the namespace
		// so the inline shims and lens views can call into them.
		"clearCanvas:               clearCanvas,",
		"addNode:                   addNode,",
		"addConnector:              addConnector,",
		"addConnectorHitTarget:     addConnectorHitTarget,",
		"addLiveConnector:          addLiveConnector,",
		"addMoreNode:               addMoreNode,",
		"effectiveGmapPosition:     effectiveGmapPosition,",
		"applyVisibilityFilters:    applyVisibilityFilters,",
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("graph-renderer.js missing required production token: %q", want)
		}
	}

	// Lens-agnostic guarantee: Context-specific node-kind strings
	// must not appear here. Those live in the Context view.
	forbidden := []string{
		`'business_service'`,
		`'related_business_service'`,
		`'capability'`,
		`'process'`,
		`'decision_surface'`,
		`'ai_system'`,
		`'ai_system_binding'`,
		`'coverage'`,
		`'authority_summary'`,
		"business-service-node",
		"related-service-node",
		"capability-node",
		"process-node",
		"decision-surface-node",
		"ai-system-node",
		"authority-node",
		"coverage-node",
	}
	for _, f := range forbidden {
		if strings.Contains(rendererJS, f) {
			t.Errorf("graph-renderer.js must not hard-code Context-specific token %q (belongs in Context view/adapter)", f)
		}
	}
}

// TestExplorer_D32aImpl3_ContextViewOwnsRenderContextGraph pins the
// Context Graph renderer implementation in the Context view module
// (graph/context/context-graph-view.js) and asserts the inline IIFE
// retains only a thin compatibility shim.
func TestExplorer_D32aImpl3_ContextViewOwnsRenderContextGraph(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")

	// Production implementations live in the view module.
	for _, want := range []string{
		"function renderContextGraph(data, ctx)",
		"function renderContextGraphEmpty(message, bsId, ctx)",
		"function renderContextGraphError(message, ctx)",
		// Per-kind node-card emission: business / related / cap /
		// proc / surface / ai / authority / coverage. The view must
		// own each branch.
		"business-service-node selected gmap-root-node",
		"ai-system-node selected gmap-root-node",
		"decision-surface-node selected gmap-root-node",
		"label: 'CAPABILITY'",
		"label: 'PROCESS'",
		"label: 'DECISION SURFACE'",
		"label: 'AUTHORITY'",
		"label: 'COVERAGE'",
		// Root-kind discriminator branches.
		"isAIRootView",
		"isSurfRootView",
		// FMP badge logic — Context-specific, view-owned.
		"_failModePolicyBadgesForSurface",
		"FMP override",
		"FMP inherited",
		"FMP default",
		// Connector emission via renderer primitive.
		"renderer.addLiveConnector(",
		"connector-service",
		"connector-ai-binding",
		"connector-authority",
		"connector-evidence",
		"connector-gap",
		// Summary panel rows.
		"Outgoing relationships",
		"Coverage gaps",
		"Root AI system",
		"Root decision surface",
		// Camera/focus orchestration through ctx hooks.
		"ctx.focusOnRoot(rootCardId)",
		"ctx.applyFitMode(true)",
		"ctx.scheduleFitToView()",
		"ctx.applyMultiSelection()",
	} {
		if !strings.Contains(viewJS, want) {
			t.Errorf("context-graph-view.js missing required production token: %q", want)
		}
	}

	// The inline IIFE must retain ONLY the thin shim. Its body
	// references `_gmapRenderCtx` and delegates to the view module.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	startIdx := strings.Index(body, "function renderGovernanceMap(data) {")
	if startIdx < 0 {
		t.Fatal("inline IIFE must retain renderGovernanceMap shim declaration")
	}
	// Bound a generous slice (1500 bytes) for the shim wrapper and
	// the closing brace. The production renderer (820 lines, ~40KB)
	// no longer fits here.
	endIdx := startIdx + 1500
	if endIdx > len(body) {
		endIdx = len(body)
	}
	shimSlice := body[startIdx:endIdx]
	if !strings.Contains(shimSlice, "ExplorerGraph.contextView.renderContextGraph(data, _gmapRenderCtx)") {
		t.Error("D32a-impl-3: inline renderGovernanceMap shim must delegate to ExplorerGraph.contextView.renderContextGraph")
	}
	// The renderer body content moved to the view module — sentinel
	// patterns must NOT appear inline anymore.
	for _, gone := range []string{
		"failModePolicyBadgesForSurface",      // view-owned helper
		"setGovernanceMapSummary([",           // summary panel emission moved
		"isAIRootView ? 'ai:'",                // root id discriminator moved
		"'connector-ai-binding'",              // connector class emission moved
	} {
		if strings.Contains(shimSlice, gone) {
			t.Errorf("D32a-impl-3: inline renderGovernanceMap shim must NOT contain %q (moved to context-graph-view.js)", gone)
		}
	}
}

// TestExplorer_D32aImpl3_InlineShimsOnly pins that each previously-
// inline production function declaration is now a thin compatibility
// shim, not the original ~800-line implementation. The shim delegates
// to the renderer/view module.
func TestExplorer_D32aImpl3_InlineShimsOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	type shim struct {
		decl     string
		delegate string
	}
	shims := []shim{
		{"function renderGovernanceMap(data) {", "ExplorerGraph.contextView.renderContextGraph(data, _gmapRenderCtx)"},
		{"function renderGovernanceMapEmpty(message, bsId) {", "ExplorerGraph.contextView.renderContextGraphEmpty(message, bsId, _gmapRenderCtx)"},
		{"function renderGovernanceMapError(message) {", "ExplorerGraph.contextView.renderContextGraphError(message, _gmapRenderCtx)"},
		{"function clearGovernanceMapCanvas() {", "ExplorerGraph.renderer.clearCanvas()"},
		{"function addNode(spec, pos) {", "ExplorerGraph.renderer.addNode(spec, pos)"},
		{"function addConnector(p1, p2, cls) {", "ExplorerGraph.renderer.addConnector(p1, p2, cls)"},
		{"function addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId, srcLabel, dstLabel) {", "ExplorerGraph.renderer.addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId, srcLabel, dstLabel)"},
		{"function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls) {", "ExplorerGraph.renderer.addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls)"},
		{"function addMoreNode(layerKey, layerLabel, total, rendered, pos) {", "ExplorerGraph.renderer.addMoreNode(layerKey, layerLabel, total, rendered, pos)"},
		{"function effectiveGmapPosition(id) {", "ExplorerGraph.renderer.effectiveGmapPosition(id)"},
		{"function applyGmapVisibilityFilters() {", "ExplorerGraph.renderer.applyVisibilityFilters()"},
	}
	for _, s := range shims {
		idx := strings.Index(body, s.decl)
		if idx < 0 {
			t.Errorf("D32a-impl-3: inline shim declaration missing: %q", s.decl)
			continue
		}
		// Slice generous — shims should be ≤ 400 bytes each.
		end := idx + 800
		if end > len(body) {
			end = len(body)
		}
		slice := body[idx:end]
		if !strings.Contains(slice, s.delegate) {
			t.Errorf("D32a-impl-3: inline shim for %q must delegate via %q", s.decl, s.delegate)
		}
	}
}

// TestExplorer_D32aImpl3_StateNamespacePopulated pins that the shared
// graph state namespace is established with the canonical state
// containers and the inline IIFE binds local consts to those same
// objects (preserving identity through every existing reader/writer).
func TestExplorer_D32aImpl3_StateNamespacePopulated(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rendererJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")
	for _, want := range []string{
		"window.MIDASExplorerGraph.state",
		"_state.positions",
		"_state.dragOverrides",
		"_state.connectors",
		"_state.selectedNodeIds",
		"_state.visibilityFilters",
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("graph-renderer.js must declare state container %q", want)
		}
	}
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"const _ExplorerGraphState = window.MIDASExplorerGraph.state",
		"const gmapPositions = _ExplorerGraphState.positions;",
		"const gmapVisibilityFilters = _ExplorerGraphState.visibilityFilters;",
		"const gmapDragOverrides = _ExplorerGraphState.dragOverrides;",
		"const gmapConnectors = _ExplorerGraphState.connectors;",
		"const gmapSelectedNodeIds = _ExplorerGraphState.selectedNodeIds;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must bind local const to namespace state: %q", want)
		}
	}
}

// TestExplorer_D32aImpl3_RendererHooksPublished pins that the inline
// IIFE publishes the renderer hooks the moved primitives rely on
// (drag attach, multi-select, inspector clear, tooltip cleanup, etc.).
func TestExplorer_D32aImpl3_RendererHooksPublished(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"window.MIDASExplorerGraph._rendererHooks",
		"attachDragHandlers:",
		"selectNode:",
		"applyMultiSelection:",
		"connectorEndpointLabel:",
		"hideConnectorTooltip:",
		"getSelectedId:",
		"clearSelection:",
		"clearInspector:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must publish renderer hook %q", want)
		}
	}
	// Render context bundle for the view module.
	for _, want := range []string{
		"const _gmapRenderCtx = {",
		"setStatus:",
		"setCurrentRoot:",
		"setSummary:",
		"setDetailsName:",
		"setDetailsFields:",
		"selectNode:",
		"applyZoom:",
		"focusOnRoot:",
		"applyFitMode:",
		"scheduleFitToView:",
		"applyMultiSelection:",
		"window.MIDASExplorerGraph._renderCtx = _gmapRenderCtx;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must wire render-ctx hook %q", want)
		}
	}
}

// TestExplorer_D32aImpl3_BehaviouralCoverage_RenderTail pins the
// classic render-tail ordering — focus root before scheduling fit-to-
// view, multi-selection re-applied at the end — but against the
// module body where the implementation now lives.
func TestExplorer_D32aImpl3_BehaviouralCoverage_RenderTail(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")

	// Anchor on the actual call-site forms (`if (typeof ctx.X === …`)
	// to avoid matching the docstring listings of the same names that
	// appear at the top of the file.
	focusIdx := strings.Index(viewJS, "ctx.focusOnRoot === 'function'")
	scheduleIdx := strings.Index(viewJS, "ctx.scheduleFitToView === 'function'")
	multiIdx := strings.Index(viewJS, "ctx.applyMultiSelection === 'function'")
	if focusIdx < 0 {
		t.Error("Render tail: focusOnRoot call missing from context-graph-view.js")
	}
	if scheduleIdx < 0 {
		t.Error("Render tail: scheduleFitToView call missing from context-graph-view.js")
	}
	if multiIdx < 0 {
		t.Error("Render tail: applyMultiSelection call missing from context-graph-view.js")
	}
	if focusIdx > 0 && scheduleIdx > 0 && focusIdx > scheduleIdx {
		t.Errorf("focusOnRoot must precede scheduleFitToView in render tail (focus=%d, schedule=%d)", focusIdx, scheduleIdx)
	}
	if scheduleIdx > 0 && multiIdx > 0 && scheduleIdx > multiIdx {
		t.Errorf("scheduleFitToView must precede applyMultiSelection (schedule=%d, multi=%d)", scheduleIdx, multiIdx)
	}
}

// TestExplorer_D32aImpl3_BehaviouralCoverage_LiveConnectorMetadata
// pins addLiveConnector's hover metadata wiring (data attributes +
// aria-label + hit-target twin) in its new module home.
func TestExplorer_D32aImpl3_BehaviouralCoverage_LiveConnectorMetadata(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rendererJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")
	for _, want := range []string{
		"pathEl.classList.add('gmap-connector')",
		"pathEl.setAttribute('data-connector-kind'",
		"pathEl.setAttribute('data-source-node-id', srcId)",
		"pathEl.setAttribute('data-target-node-id', dstId)",
		"pathEl.setAttribute('role', 'img')",
		"pathEl.setAttribute(",
		"'aria-label'",
		"hitEl.gmapVisibleConnector = pathEl",
		"pathEl.gmapHitTarget = hitEl",
		"_state.connectors.push({",
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("graph-renderer.js addLiveConnector behavioural pin missing: %q", want)
		}
	}
}

// TestExplorer_D32aImpl3_BehaviouralCoverage_VisibilityFilters pins
// the chip-driven visibility walk semantics (endpoint-hidden derivation,
// bindings class override, selection clear) in the renderer module.
func TestExplorer_D32aImpl3_BehaviouralCoverage_VisibilityFilters(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rendererJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")
	for _, want := range []string{
		"function applyVisibilityFilters(",
		// Endpoint-derived hide.
		"hiddenIds.has(c.srcId) || hiddenIds.has(c.dstId)",
		// Bindings class override.
		"connector-ai-binding",
		// Connector hidden class toggling.
		"gmap-connector-hidden",
		// Selection clear hook.
		"_hooks.clearSelection",
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("graph-renderer.js applyVisibilityFilters behavioural pin missing: %q", want)
		}
	}
}

// TestExplorer_D32aImpl3_BridgeIsCompatAlias pins that the bridge no
// longer owns rendering: it is documented as a compatibility alias and
// its forwarded properties point at the inline shims (which themselves
// delegate to the modules).
func TestExplorer_D32aImpl3_BridgeIsCompatAlias(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Positive: the bridge wiring carries the documented compat-alias
	// annotation so future readers know it no longer owns rendering.
	if !strings.Contains(body, "compatibility alias") {
		t.Error("bridge wiring must declare compatibility-alias status")
	}
	// Positive: the bridge wiring is the LAST occurrence — placed at
	// the end of the IIFE after every renderer/shim declaration.
	wireIdx := strings.Index(body, "MIDASExplorerGovernanceMapBridge = window.MIDASExplorerGovernanceMapBridge")
	if wireIdx < 0 {
		t.Fatal("bridge wiring not found")
	}
	if !strings.Contains(body[wireIdx:], ".renderGovernanceMap        = renderGovernanceMap;") {
		t.Error("bridge wiring must forward renderGovernanceMap (inline shim)")
	}
	// Negative: the bridge declarations must not be wrapped in a
	// comment that implies it OWNS the renderer.
	if strings.Contains(body, "Production bridge wiring") {
		t.Error("bridge comment must not describe itself as a production bridge (D32a-impl-2 wording retired in D32a-impl-3)")
	}
}

// TestExplorer_D32aImpl3_IndexHtmlMaterialReduction confirms a
// material reduction in index.html relative to D32a-impl-2's end
// state. The renderer + node + connector + filter + empty + error
// implementations moved out; the inline shims are short. Pinning a
// loose upper bound guards against the renderer slipping back inline.
func TestExplorer_D32aImpl3_IndexHtmlMaterialReduction(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// Count newlines for a stable cross-platform line proxy.
	lines := strings.Count(body, "\n") + 1
	// D32a-impl-2 end state: 10,658 lines. After extraction the
	// inline file should be materially smaller. The threshold here
	// (10,000) is a conservative ceiling — current measurement is
	// ~9,700 lines.
	if lines > 10000 {
		t.Errorf("D32a-impl-3: index.html line count %d exceeds 10000 — renderer may have slipped back inline", lines)
	}
}

// ---------------------------------------------------------------------------
// D32a-impl-4 — Explorer Inline IIFE Decomposition and Index Slimdown
//
// Tests in this section pin the architectural end-state of D32a-impl-4:
//
//   • graph-camera.js owns zoom / pan / fit / focus camera state +
//     lifecycle (clampZoom, applyZoom, setZoom, focusRoot, fitToBounds,
//     scheduleFitToView, applyFitMode, computeRenderedExtent).
//   • graph-interactions.js owns the node drag handler attachment.
//   • graph-selection.js owns single + multi-selection state and the
//     applyMultiSelection / clearMulti helpers.
//   • graph-inspector.js owns the lens-agnostic inspector frame
//     setters (setName / setFields / setSummary / setGovernance /
//     setActions / setInlineActions) plus show / hide.
//   • graph/context/context-graph-inspector.js owns the Context-lens
//     inspector content (selectNode, buildFailModePolicySection,
//     GOVERNANCE_KEYS).
//   • The inline IIFE retains thin compatibility shims for the
//     legacy names (selectGovernanceMapNode, setGovernanceMapDetails*,
//     applyGmapZoom, focusGmapOnRoot, etc.) — every shim is a single
//     delegating call into the module.
//   • Shared state lives at window.MIDASExplorerGraph.state and
//     covers zoom / panX / panY / positions / dragOverrides /
//     connectors / dragState / selectedId / selectedNodeIds /
//     visibilityFilters.
//   • Inline IIFE wires _actionDispatcher / _interactionsHooks /
//     _inspectorHooks so modules can call back into the still-inline
//     orchestration without coupling.
// ---------------------------------------------------------------------------

// TestExplorer_D32aImpl4_CameraModuleOwnsLifecycle pins that the
// graph camera module owns the production state + lifecycle, and the
// inline IIFE retains only delegating shims.
func TestExplorer_D32aImpl4_CameraModuleOwnsLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	cameraJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-camera.js")
	for _, want := range []string{
		"function clampZoom(",
		"function computeRenderedExtent(",
		"function applyZoom(",
		"function setZoom(",
		"function focusRoot(",
		"function fitToBounds(",
		"function scheduleFitToView(",
		"function applyFitMode(",
		"window.MIDASExplorerGraph.camera",
		// Behavioural pins migrated from the inline body tests.
		"var safe = _safeArea(scrollEl)",
		"scene.style.transform = 'translate(' + _state.panX + 'px, ' + _state.panY + 'px) scale(' + _state.zoom + ')'",
		"GMAP_ZOOM",
		"FIT_MIN",
	} {
		if !strings.Contains(cameraJS, want) {
			t.Errorf("graph-camera.js missing required production token: %q", want)
		}
	}

	// Inline IIFE — each public camera function must be a thin shim
	// that delegates into the camera module.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	type shim struct{ decl, delegate string }
	shims := []shim{
		{"function clampGmapZoom(z) {", "ExplorerGraph.camera.clampZoom(z)"},
		{"function computeGmapRenderedExtent(canvas) {", "ExplorerGraph.camera.computeRenderedExtent(canvas)"},
		{"function applyGmapZoom() {", "ExplorerGraph.camera.applyZoom()"},
		{"function setGmapZoom(z) {", "ExplorerGraph.camera.setZoom(z)"},
		{"function focusGmapOnRoot(rootCardId) {", "ExplorerGraph.camera.focusRoot(rootCardId)"},
		{"function fitGmapToBounds() {", "ExplorerGraph.camera.fitToBounds()"},
		{"function scheduleGmapFitToView() {", "ExplorerGraph.camera.scheduleFitToView()"},
		{"function applyGmapFitMode(active) {", "ExplorerGraph.camera.applyFitMode(active)"},
	}
	for _, s := range shims {
		idx := strings.Index(body, s.decl)
		if idx < 0 {
			t.Errorf("D32a-impl-4: inline shim missing for %q", s.decl)
			continue
		}
		end := idx + 600
		if end > len(body) {
			end = len(body)
		}
		if !strings.Contains(body[idx:end], s.delegate) {
			t.Errorf("D32a-impl-4: shim %q must delegate via %q", s.decl, s.delegate)
		}
	}
}

// TestExplorer_D32aImpl4_InteractionsModuleOwnsDrag pins that the
// graph interactions module owns the node drag handler.
func TestExplorer_D32aImpl4_InteractionsModuleOwnsDrag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	intJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-interactions.js")
	for _, want := range []string{
		"function attachNodeDragHandlers(",
		"window.MIDASExplorerGraph.interactions",
		// Behavioural pins migrated from the inline body tests:
		// group drag start positions, pointer threshold, drag-then-
		// click suppression.
		"isGroupDrag = _state.selectedNodeIds.has(nodeId) && _state.selectedNodeIds.size > 1",
		"_state.dragState = {",
		"groupStartPositions",
		"setPointerCapture",
		"releasePointerCapture",
		"hideConnectorTooltip",
	} {
		if !strings.Contains(intJS, want) {
			t.Errorf("graph-interactions.js missing required production token: %q", want)
		}
	}
	// Inline IIFE shim must delegate.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "ExplorerGraph.interactions.attachNodeDragHandlers(node, nodeId)") {
		t.Error("D32a-impl-4: inline attachGmapDragHandlers must delegate to ExplorerGraph.interactions.attachNodeDragHandlers")
	}
	// _interactionsHooks must be published so the module can call
	// back into still-inline repaintGmapConnectors.
	if !strings.Contains(body, "window.MIDASExplorerGraph._interactionsHooks") {
		t.Error("D32a-impl-4: inline IIFE must publish _interactionsHooks for the interactions module")
	}
	if !strings.Contains(body, "repaintConnectors:") {
		t.Error("D32a-impl-4: _interactionsHooks must expose repaintConnectors")
	}
}

// TestExplorer_D32aImpl4_SelectionModuleOwnsState pins that the
// graph selection module owns selection state + the multi-selection
// helpers.
func TestExplorer_D32aImpl4_SelectionModuleOwnsState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	selJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-selection.js")
	for _, want := range []string{
		"function getSelected(",
		"function setSelected(",
		"function getSelectedSet(",
		"function addToMulti(",
		"function removeFromMulti(",
		"function toggleMulti(",
		"function clearMulti(",
		"function applyMultiSelection(",
		"window.MIDASExplorerGraph.selection",
		"_state.selectedNodeIds",
		"gmap-multi-selected",
	} {
		if !strings.Contains(selJS, want) {
			t.Errorf("graph-selection.js missing required production token: %q", want)
		}
	}
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"ExplorerGraph.selection.applyMultiSelection()",
		"ExplorerGraph.selection.clearMulti()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32a-impl-4: inline shim must delegate via %q", want)
		}
	}
}

// TestExplorer_D32aImpl4_InspectorModuleOwnsFrame pins that the
// inspector module owns the lens-agnostic frame setters.
func TestExplorer_D32aImpl4_InspectorModuleOwnsFrame(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	insJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")
	for _, want := range []string{
		"function setName(",
		"function setFields(",
		"function setSummary(",
		"function setGovernance(",
		"function setActions(",
		"function setInlineActions(",
		"function show(",
		"function hide(",
		"window.MIDASExplorerGraph.inspector",
		// Action whitelist + dispatch through _actionDispatcher.
		"view-business-service-record",
		"view-capability-record",
		"reframe-around-this",
		"_actionDispatcher",
	} {
		if !strings.Contains(insJS, want) {
			t.Errorf("graph-inspector.js missing required production token: %q", want)
		}
	}
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"ExplorerGraph.inspector.setName(name)",
		"ExplorerGraph.inspector.setFields(rows)",
		"ExplorerGraph.inspector.setSummary(rows)",
		"ExplorerGraph.inspector.setGovernance(html)",
		"ExplorerGraph.inspector.setActions(actions)",
		"ExplorerGraph.inspector.setInlineActions(node, actions)",
		// _actionDispatcher wiring.
		"window.MIDASExplorerGraph._actionDispatcher",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32a-impl-4: inline IIFE must contain %q", want)
		}
	}
}

// TestExplorer_D32aImpl4_ContextInspectorOwnsContent pins that the
// Context inspector module owns the Context-lens inspector content.
func TestExplorer_D32aImpl4_ContextInspectorOwnsContent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-inspector.js")
	for _, want := range []string{
		"function selectNode(nodeId)",
		"function buildFailModePolicySection(",
		"window.MIDASExplorerGraph.contextInspector",
		// Fail-mode policy reference wording (Context-specific).
		"Default policy",
		"Surface override",
		"Inherited default",
		"Effective source",
		"Business service default",
		"None configured",
		"Evidence only",
		"Soft/open",
		"Not enabled",
		// Markup the inline section uses.
		"gmap-details-section",
		"gmap-details-title",
		"gmap-fmp-reference",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("context-graph-inspector.js missing required production token: %q", want)
		}
	}
	// Inline shim must delegate.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"ExplorerGraph.contextInspector.selectNode(nodeId)",
		"ExplorerGraph.contextInspector.buildFailModePolicySection(nodeKind, details, data)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32a-impl-4: inline IIFE must contain %q", want)
		}
	}
}

// TestExplorer_D32aImpl4_StateNamespaceCovered pins that the state
// namespace now covers camera state too (zoom / panX / panY) in
// addition to the D32a-impl-3 fields.
func TestExplorer_D32aImpl4_StateNamespaceCovered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	cameraJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-camera.js")
	for _, want := range []string{
		"_state.zoom",
		"_state.panX",
		"_state.panY",
	} {
		if !strings.Contains(cameraJS, want) {
			t.Errorf("graph-camera.js must operate on namespace state: %q", want)
		}
	}
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"let gmapZoom = _ExplorerGraphState.zoom",
		"let gmapPanX = _ExplorerGraphState.panX",
		"let gmapPanY = _ExplorerGraphState.panY",
		"function _syncCameraToState()",
		"function _syncCameraFromState()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must bind camera locals to namespace state: %q", want)
		}
	}
}

// TestExplorer_D32aImpl4_IndexHtmlReducedBelowImpl3 pins a tighter
// upper bound than D32a-impl-3's. D32a-impl-3 left index.html at
// ~9,698 lines; D32a-impl-4 should pull this materially lower.
func TestExplorer_D32aImpl4_IndexHtmlReducedBelowImpl3(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 9300 {
		t.Errorf("D32a-impl-4: index.html line count %d exceeds 9300 — camera/inspector/selection extraction should land lower", lines)
	}
}

// TestExplorer_D32aImpl4_NoActiveAuthorityUI confirms the Authority
// Graph endpoint remains absent from active UI code (carry-forward
// from D32a-impl-2/3).
func TestExplorer_D32aImpl4_NoActiveAuthorityUI(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, path := range []string{
		"/explorer/assets/js/graph/graph-camera.js",
		"/explorer/assets/js/graph/graph-interactions.js",
		"/explorer/assets/js/graph/graph-selection.js",
		"/explorer/assets/js/graph/graph-inspector.js",
		"/explorer/assets/js/graph/context/context-graph-inspector.js",
	} {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, "/v1/graphs/authority") {
			t.Errorf("D32a-impl-4: %s must not reference /v1/graphs/authority", path)
		}
		if strings.Contains(js, "ExplorerAPI.graphs.authority(") || strings.Contains(js, "api.authority(") {
			t.Errorf("D32a-impl-4: %s must not invoke Authority Graph API", path)
		}
	}
}

// ---------------------------------------------------------------------------
// D32a-impl-5 — Explorer Services and Capabilities View Extraction
//
// Tests in this section pin the architectural end-state of D32a-impl-5:
//
//   • services-view.js owns the Services view orchestration.
//   • capabilities-view.js owns the Capabilities view orchestration.
//   • ExplorerAPI.businessServices routes point at /v1/businessservices
//     (corrected from a stale /v1/business-services declaration).
//   • Service/capability fetches go through ExplorerAPI.
//   • selectedBusinessServiceId / selectedCapabilityId are written
//     to the ExplorerStore on selection.
//   • Inline IIFE retains thin compatibility shims.
//   • Authority Graph remains absent from active UI paths.
// ---------------------------------------------------------------------------

func TestExplorer_D32aImpl5_ServicesModuleOwnsOrchestration(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/services/services-view.js")
	for _, want := range []string{
		"window.MIDASExplorerServices",
		"function loadCatalogue(",
		"function showCatalogue(",
		"function showRecord(",
		"function showMap(",
		"function showDriftOverview(",
		"function renderCatalogue(",
		"function renderRecord(",
		"function loadRecord(",
		"function setSubView(",
		"function getSelectedServiceId(",
		"function setSelectedServiceId(",
		"_api()",
		"api.list()",
		"graph.refresh({ lens: 'context', view: 'service'",
		"selectedBusinessServiceId",
		"services-bs-loading",
		"services-bs-empty",
		"services-bs-error",
		"Loading business services",
		"Could not load business services",
		"No business services found",
		"services-record-section-title",
		"Governance summary",
		"Decision surfaces",
		"AI systems",
		"renderRecordFieldGrid:",
		"renderRecordSection:",
		"renderRelatedList:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("services-view.js missing required token: %q", want)
		}
	}
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"MIDASExplorerServices.loadCatalogue()",
		"MIDASExplorerServices.showCatalogue()",
		"MIDASExplorerServices.showRecord(serviceId)",
		"MIDASExplorerServices.showMap(serviceId)",
		"MIDASExplorerServices.showDriftOverview()",
		"MIDASExplorerServices.renderCatalogue(filter)",
		"MIDASExplorerServices.renderRecord(payload)",
		"MIDASExplorerServices.loadRecord(serviceId)",
		"MIDASExplorerServices.setSubView(view)",
		"MIDASExplorerServices.init({",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must delegate via %q", want)
		}
	}
}

func TestExplorer_D32aImpl5_CapabilitiesModuleOwnsOrchestration(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/capabilities/capabilities-view.js")
	for _, want := range []string{
		"window.MIDASExplorerCapabilities",
		"function loadCatalogue(",
		"function showCatalogue(",
		"function showRecord(",
		"function renderCatalogue(",
		"function renderRecord(",
		"function loadRecord(",
		"function loadChildren(",
		"function loadBusinessServices(",
		"function loadAIBindings(",
		"function renderChildrenSection(",
		"function renderBusinessServicesSection(",
		"function renderAIBindingsSection(",
		"function setSubView(",
		"function getSelectedCapabilityId(",
		"function setSelectedCapabilityId(",
		"api.list()",
		"api.get(capId)",
		"api.children(capId)",
		"api.businessServices(capId)",
		"api.aiBindings(capId)",
		"selectedCapabilityId",
		"capabilities-loading",
		"capabilities-error",
		"capabilities-empty",
		"capabilities-record-children",
		"capabilities-record-business-services",
		"capabilities-record-ai-bindings",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("capabilities-view.js missing required token: %q", want)
		}
	}
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"MIDASExplorerCapabilities.loadCatalogue()",
		"MIDASExplorerCapabilities.showCatalogue()",
		"MIDASExplorerCapabilities.showRecord(capId)",
		"MIDASExplorerCapabilities.renderCatalogue(filter)",
		"MIDASExplorerCapabilities.renderRecord(payload)",
		"MIDASExplorerCapabilities.loadRecord(capId)",
		"MIDASExplorerCapabilities.renderChildrenSection(capId)",
		"MIDASExplorerCapabilities.renderBusinessServicesSection(capId)",
		"MIDASExplorerCapabilities.renderAIBindingsSection(capId)",
		"MIDASExplorerCapabilities.init({}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must delegate via %q", want)
		}
	}
}

func TestExplorer_D32aImpl5_APIClientRoutesMatchBackend(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/core/api-client.js")
	for _, want := range []string{
		"'/v1/businessservices'",
		"'/v1/businessservices/'",
		"'/v1/capabilities'",
		"'/v1/capabilities/'",
		"/businessservices",
		"/ai-bindings",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("api-client.js must reference backend path %q", want)
		}
	}
	for _, gone := range []string{
		"'/v1/business-services'",
		"'/v1/business-services/'",
		"/v1/capabilities/' + encodeURIComponent(id) + '/business-services'",
	} {
		if strings.Contains(js, gone) {
			t.Errorf("api-client.js must not reference stale hyphenated path %q", gone)
		}
	}
}

func TestExplorer_D32aImpl5_NoDirectBSCapabilitiesFetch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, gone := range []string{
		"fetch('/v1/businessservices'",
		"fetch('/v1/capabilities'",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32a-impl-5: inline IIFE must not contain direct fetch %q (use ExplorerAPI)", gone)
		}
	}
	for _, path := range []string{
		"/explorer/assets/js/services/services-view.js",
		"/explorer/assets/js/capabilities/capabilities-view.js",
	} {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, "fetch('/v1/") {
			t.Errorf("D32a-impl-5: %s must not call fetch() directly — use ExplorerAPI", path)
		}
	}
}

func TestExplorer_D32aImpl5_StoreSelectedIdsWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	svcJS := getExplorerAsset(t, srv, "/explorer/assets/js/services/services-view.js")
	capJS := getExplorerAsset(t, srv, "/explorer/assets/js/capabilities/capabilities-view.js")
	if !strings.Contains(svcJS, "setState({ selectedBusinessServiceId: id || '' })") {
		t.Error("services-view.js must write selectedBusinessServiceId to ExplorerStore on selection")
	}
	if !strings.Contains(capJS, "setState({ selectedCapabilityId: id || '' })") {
		t.Error("capabilities-view.js must write selectedCapabilityId to ExplorerStore on selection")
	}
}

func TestExplorer_D32aImpl5_GraphRefreshOnServiceSelection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	svcJS := getExplorerAsset(t, srv, "/explorer/assets/js/services/services-view.js")
	if !strings.Contains(svcJS, "graph.refresh({ lens: 'context', view: 'service'") {
		t.Error("services-view.js loadRecord must call graph.refresh({lens:'context', view:'service'})")
	}
	if !strings.Contains(svcJS, "depth: 5") {
		t.Error("services-view.js graph.refresh must request depth: 5")
	}
}

func TestExplorer_D32aImpl5_NewModuleScriptsLoaded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`<script src="/explorer/assets/js/services/services-view.js"></script>`,
		`<script src="/explorer/assets/js/capabilities/capabilities-view.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html must reference module script %q", want)
		}
	}
}

func TestExplorer_D32aImpl5_IndexHtmlReducedBelowImpl4(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 8500 {
		t.Errorf("D32a-impl-5: index.html line count %d exceeds 8500 — services/capabilities extraction should land lower", lines)
	}
}

func TestExplorer_D32aImpl5_NoActiveAuthorityUI(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, path := range []string{
		"/explorer/assets/js/services/services-view.js",
		"/explorer/assets/js/capabilities/capabilities-view.js",
	} {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, "/v1/graphs/authority") {
			t.Errorf("D32a-impl-5: %s must not reference /v1/graphs/authority", path)
		}
		if strings.Contains(js, "ExplorerAPI.graphs.authority(") || strings.Contains(js, "api.authority(") {
			t.Errorf("D32a-impl-5: %s must not invoke Authority Graph API", path)
		}
	}
}

// ---------------------------------------------------------------------------
// D32a-impl-6 — Test Debt Burn-down and Inline Shim Cleanup
//
// Tests in this section pin the end-state of D32a-impl-6:
//
//   • Skipped Explorer test count is bounded by a documented threshold.
//     Baseline before D32a-impl-6: 37 skipped tests across explorer_test.go
//     + explorer_d32a_test.go + explorer_drift_test.go +
//     explorer_failmode_enforcement_test.go.
//     End state after D32a-impl-6: 1 skipped test
//     (TestExplorer_HTML_Polish_ServicesColumnHeadersPresent — a
//     documented pre-D32a retirement stub kept so git-blame finds the
//     retirement note rather than a missing-test mystery).
//   • Every Explorer subsystem (graph renderer / camera / interactions /
//     selection / inspector / context view / context adapter / services /
//     capabilities) has at least one D32aImpl{3,4,5} module-ownership
//     test asserting where the production implementation lives.
//   • No direct fetch() to /v1/businessservices, /v1/capabilities,
//     /v1/graphs/context, or /v1/graphs/authority remains outside the
//     API client module.
// ---------------------------------------------------------------------------

// explorerTestFiles is the canonical list of Explorer-related test
// files whose t.Skip count is bounded by D32a-impl-6.
var explorerTestFiles = []string{
	"explorer_test.go",
	"explorer_d32a_test.go",
	"explorer_drift_test.go",
	"explorer_failmode_enforcement_test.go",
}

// TestExplorer_D32aImpl6_SkippedTestCountThreshold caps the total
// number of t.Skip(...) calls across the Explorer test surface.
// D32a-impl-6 burned the skipped-test debt from 37 → 1; the threshold
// of 5 leaves headroom for new strictly-documented retirements but
// catches any regression that would silently re-introduce inline-pin
// debt.
//
// If the threshold needs to grow, the test must be edited along with
// a documented reason in the diff message — silent growth is the
// failure mode this guards against.
func TestExplorer_D32aImpl6_SkippedTestCountThreshold(t *testing.T) {
	const threshold = 5
	total := 0
	perFile := map[string]int{}
	for _, name := range explorerTestFiles {
		path := filepath.Join(".", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		count := strings.Count(string(data), "t.Skip(")
		perFile[name] = count
		total += count
	}
	if total > threshold {
		t.Errorf("D32a-impl-6: skipped Explorer test count %d exceeds threshold %d. Per-file: %v. Either replace the new skip with a module-ownership / behavioural assertion, delete the test if the assertion is obsolete, or raise the threshold with a documented reason.", total, threshold, perFile)
	}
}

// TestExplorer_D32aImpl6_ModuleOwnershipCoverage asserts that the
// D32aImpl{3,4,5} contract tests cover every Explorer subsystem
// extracted in the prior tranches. This is a meta-test: if a
// successor module-ownership test is removed (e.g., by a future
// refactor) without a replacement, this guard surfaces the gap.
func TestExplorer_D32aImpl6_ModuleOwnershipCoverage(t *testing.T) {
	data, err := os.ReadFile("./explorer_d32a_test.go")
	if err != nil {
		t.Fatalf("read explorer_d32a_test.go: %v", err)
	}
	body := string(data)
	required := []string{
		// API client + core foundations (D32a-impl-1/2).
		"TestExplorer_D32a_NewJSFiles_Served",
		"TestExplorer_D32a_HTML_LinksNewScripts",
		"TestExplorer_D32a_HTML_LensSwitcher",
		"TestExplorer_D32a_HTML_GraphFetchesGoThroughShellAndAPIClient",
		"TestExplorer_D32a_HTML_HashRoute_GraphContext",
		// Renderer + view (D32a-impl-3).
		"TestExplorer_D32aImpl3_RendererOwnsProductionPrimitives",
		"TestExplorer_D32aImpl3_ContextViewOwnsRenderContextGraph",
		"TestExplorer_D32aImpl3_InlineShimsOnly",
		"TestExplorer_D32aImpl3_StateNamespacePopulated",
		"TestExplorer_D32aImpl3_BehaviouralCoverage_RenderTail",
		"TestExplorer_D32aImpl3_BehaviouralCoverage_LiveConnectorMetadata",
		"TestExplorer_D32aImpl3_BehaviouralCoverage_VisibilityFilters",
		"TestExplorer_D32aImpl3_BridgeIsCompatAlias",
		// Camera + interactions + selection + inspector (D32a-impl-4).
		"TestExplorer_D32aImpl4_CameraModuleOwnsLifecycle",
		"TestExplorer_D32aImpl4_InteractionsModuleOwnsDrag",
		"TestExplorer_D32aImpl4_SelectionModuleOwnsState",
		"TestExplorer_D32aImpl4_InspectorModuleOwnsFrame",
		"TestExplorer_D32aImpl4_ContextInspectorOwnsContent",
		"TestExplorer_D32aImpl4_StateNamespaceCovered",
		// Services + capabilities (D32a-impl-5).
		"TestExplorer_D32aImpl5_ServicesModuleOwnsOrchestration",
		"TestExplorer_D32aImpl5_CapabilitiesModuleOwnsOrchestration",
		"TestExplorer_D32aImpl5_APIClientRoutesMatchBackend",
		"TestExplorer_D32aImpl5_NoDirectBSCapabilitiesFetch",
		"TestExplorer_D32aImpl5_StoreSelectedIdsWired",
		"TestExplorer_D32aImpl5_GraphRefreshOnServiceSelection",
	}
	for _, name := range required {
		if !strings.Contains(body, "func "+name+"(t *testing.T)") {
			t.Errorf("D32a-impl-6: required module-ownership test missing: %s", name)
		}
	}
}

// TestExplorer_D32aImpl6_NoDirectFetchOutsideAPIClient asserts that
// every active UI code path uses ExplorerAPI for /v1/* fetches —
// the inline IIFE, the graph modules, services-view, and
// capabilities-view must contain no direct fetch() to those
// endpoints.
func TestExplorer_D32aImpl6_NoDirectFetchOutsideAPIClient(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	indexBody := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, gone := range []string{
		"fetch('/v1/businessservices'",
		"fetch('/v1/capabilities'",
		"fetch('/v1/graphs/context'",
		"fetch('/v1/graphs/authority'",
	} {
		if strings.Contains(indexBody, gone) {
			t.Errorf("D32a-impl-6: index.html inline IIFE must not contain direct fetch %q", gone)
		}
	}
	// Every active module (excluding api-client.js, which IS the
	// API client) must be fetch-free for these endpoints. Records /
	// drift / evaluate modules retain their own fetches for non-
	// graph/non-bs/non-cap endpoints — that's expected.
	for _, path := range []string{
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-camera.js",
		"/explorer/assets/js/graph/graph-interactions.js",
		"/explorer/assets/js/graph/graph-selection.js",
		"/explorer/assets/js/graph/graph-inspector.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-inspector.js",
		"/explorer/assets/js/services/services-view.js",
		"/explorer/assets/js/capabilities/capabilities-view.js",
	} {
		js := getExplorerAsset(t, srv, path)
		for _, gone := range []string{
			"fetch('/v1/businessservices",
			"fetch('/v1/capabilities",
			"fetch('/v1/graphs/context",
			"fetch('/v1/graphs/authority",
		} {
			if strings.Contains(js, gone) {
				t.Errorf("D32a-impl-6: %s must not contain direct fetch %q (use ExplorerAPI)", path, gone)
			}
		}
	}
}

// TestExplorer_D32aImpl6_BridgeRemainsCompatAlias confirms the
// MIDASExplorerGovernanceMapBridge is still annotated as a
// compatibility alias and no module module-owns rendering through
// it. The bridge wiring lives at the tail of the inline IIFE; the
// guard pins both the alias comment and that no Context Graph
// renderer dispatches through bridge.renderGovernanceMap as the
// production path (the shell + adapter + view module own it).
func TestExplorer_D32aImpl6_BridgeRemainsCompatAlias(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "compatibility alias") {
		t.Error("D32a-impl-6: bridge wiring must keep the compatibility-alias annotation introduced in D32a-impl-3")
	}
	// Graph view module must not reach into the bridge for its
	// own production dispatch — it owns the lens implementation
	// directly.
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if strings.Contains(viewJS, "MIDASExplorerGovernanceMapBridge") {
		t.Error("D32a-impl-6: context-graph-view.js must not depend on the compatibility-alias bridge for production rendering")
	}
}
