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
// Map renderer. The Authority Graph lens UI was added in D32b-impl-1;
// the original D32a-impl-1 disabled-Authority pins have been updated
// to assert the Authority lens is now enabled.
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
//     with Context active by default and Authority enabled (D32b-impl-1
//     promoted Authority from disabled-with-"coming-next" to active).
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
			// D37p-clean-1 retired the dead dispatch functions
			// (register / render / clear). The live helper surface
			// retained by graph-renderer.js is asserted via the
			// representative `lensAgnosticConnectorPath` + the
			// production `clearCanvas` + `addNode` primitives.
			"function lensAgnosticConnectorPath(",
			"function clearCanvas(",
			"function addNode(",
		},
	},
	{
		path: "/explorer/assets/js/graph/graph-inspector.js",
		ns:   "window.MIDASExplorerGraph",
		contains: []string{
			// D37p-clean-2 retired the dead dispatch function
			// `renderNode(lens, node, mount)`. The live frame-setter
			// surface is asserted via a representative setter.
			"function setName(",
			"function setFields(",
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

// TestExplorer_D32a_APIClient_GraphsAuthorityURLOnlyInAPIClient confirms
// that the literal /v1/graphs/authority URL string appears only in the
// canonical API client (core/api-client.js). Active UI paths reach the
// endpoint via ExplorerAPI.graphs.authority(...) — never by hard-coding
// the URL — so the routing surface remains a single source of truth.
// D32b-impl-1 introduced the Authority Graph lens; the URL-discipline
// pin is retained, the disabled-Authority assumption is dropped.
func TestExplorer_D32a_APIClient_GraphsAuthorityURLOnlyInAPIClient(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	bodyHTML := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if strings.Contains(bodyHTML, "/v1/graphs/authority") {
		t.Errorf("index.html must not contain the literal /v1/graphs/authority URL — call ExplorerAPI.graphs.authority() instead")
	}
	// Every JS file except api-client.js must be free of the literal
	// URL string. Authority adapter/view/inspector reach the endpoint
	// via the typed ExplorerAPI.graphs.authority(...) method.
	for _, f := range d32aJSFiles {
		if f.path == "/explorer/assets/js/core/api-client.js" {
			continue
		}
		body := performRequest(t, srv, http.MethodGet, f.path, nil).Body.String()
		if strings.Contains(body, "/v1/graphs/authority") {
			t.Errorf("%s: must not reference /v1/graphs/authority — only the API client may declare the URL", f.path)
		}
	}
	// D32b-impl-1 — extend the negative-URL pin to cover the new
	// Authority lens assets so they never accidentally inline the URL.
	for _, path := range []string{
		"/explorer/assets/js/graph/authority/authority-graph-adapter.js",
		"/explorer/assets/js/graph/authority/authority-graph-view.js",
		"/explorer/assets/js/graph/authority/authority-graph-inspector.js",
	} {
		body := performRequest(t, srv, http.MethodGet, path, nil).Body.String()
		if strings.Contains(body, "/v1/graphs/authority") {
			t.Errorf("%s: must not reference /v1/graphs/authority — only the API client may declare the URL", path)
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

// TestExplorer_D32a_HTML_LensSwitcher pins the Service Workbench mode
// toolbar (Form View | Context Graph | Authority Graph). D32b-impl-2a
// moved the toolbar from .services-map-view-header into the always-
// visible .governance-map-toolbar-left (the previous location was
// hidden by the body.gmap-focus-mode CSS rule whenever focus mode
// defaulted ON). The switcher must therefore live inside the
// .governance-map-toolbar block, not before it.
func TestExplorer_D32a_HTML_LensSwitcher(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// D32b-impl-4 — The graph view switcher (Form / Context / Authority,
	// plus the Knowledge placeholder) lives inside the workbench
	// toolbar's right zone (.governance-map-toolbar-right), NOT in the
	// shell header (which is collapsed by `body.gmap-focus-mode
	// .shell-header { display: none; }` whenever focus mode is active
	// — the default on graph entry). D32d-impl-4 had placed the menu
	// in .shell-header-right, but that location was hidden in the
	// operator's primary view.
	toolbarRightIdx := strings.Index(body, `class="governance-map-toolbar-right"`)
	if toolbarRightIdx < 0 {
		t.Fatal("D32b-impl-4: missing .governance-map-toolbar-right (workbench toolbar right group)")
	}
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	if menuIdx < 0 {
		t.Fatal("D32b-impl-4: missing .graph-view-menu (workbench mode menu)")
	}
	if menuIdx < toolbarRightIdx {
		t.Errorf("D32b-impl-4: .graph-view-menu must live inside .governance-map-toolbar-right; menuIdx=%d toolbarRightIdx=%d", menuIdx, toolbarRightIdx)
	}

	// Context button — active, data-lens="context". D32b-impl-2
	// composes the Context button class as
	// `service-workbench-mode graph-lens-button is-active`; the pin
	// asserts the active state via both the substring of the class
	// attribute and the aria-pressed attribute.
	if !strings.Contains(body, `data-lens="context"`) {
		t.Error("missing Context lens button (data-lens=\"context\")")
	}
	contextIdx := strings.Index(body, `data-lens="context"`)
	if contextIdx >= 0 {
		// Walk back to enclosing <button … and forward to the first '>'
		// so the pin sees only the button's opening tag.
		tagStart := strings.LastIndex(body[:contextIdx], "<button")
		tagEnd := strings.Index(body[contextIdx:], ">")
		if tagStart >= 0 && tagEnd > 0 {
			ctxBtn := body[tagStart : contextIdx+tagEnd+1]
			if !strings.Contains(ctxBtn, "is-active") {
				t.Error("Context lens button must be active by default (.is-active)")
			}
			if !strings.Contains(ctxBtn, `aria-pressed="true"`) {
				t.Error("Context lens button must declare aria-pressed=\"true\" by default")
			}
		}
	}

	// Authority button — enabled, data-lens="authority", no aria-disabled
	// and no "coming next" copy (D32b-impl-1).
	if !strings.Contains(body, `data-lens="authority"`) {
		t.Error("missing Authority lens button (data-lens=\"authority\")")
	}
	// Scope the disabled-state negative pins to the Authority button
	// markup specifically so other parts of the page (which may
	// legitimately carry aria-disabled / is-disabled for unrelated
	// controls) are not affected.
	authBtnIdx := strings.Index(body, `data-lens="authority"`)
	if authBtnIdx >= 0 {
		// Walk back to the enclosing <button … tag for the Authority
		// button.
		tagStart := strings.LastIndex(body[:authBtnIdx], "<button")
		tagEnd := strings.Index(body[authBtnIdx:], ">")
		if tagStart >= 0 && tagEnd > 0 {
			authBtnTag := body[tagStart : authBtnIdx+tagEnd+1]
			if strings.Contains(authBtnTag, "is-disabled") {
				t.Error("D32b-impl-1: Authority lens button must not carry .is-disabled")
			}
			if strings.Contains(authBtnTag, `aria-disabled="true"`) {
				t.Error("D32b-impl-1: Authority lens button must not declare aria-disabled=\"true\"")
			}
		}
	}
	if strings.Contains(body, "Authority lens — coming next") {
		t.Error("D32b-impl-1: 'Authority lens — coming next' copy must be removed")
	}
	if strings.Contains(body, `class="graph-lens-coming-next"`) {
		t.Error("D32b-impl-1: .graph-lens-coming-next sibling must be removed")
	}

	// Lens switcher CSS must ship in the cascade (any file is fine —
	// the conceptual stylesheet is what matters). .is-disabled is
	// retained because the shell still toggles it as a runtime affordance
	// for any feature-gated lens added in the future.
	allCSS := getExplorerAllCSS(t, srv)
	for _, want := range []string{
		".graph-lens-switcher",
		".graph-lens-button",
		".graph-lens-button.is-active",
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

	// D32a-impl-8 — Inline IIFE must NOT contain the
	// mapContextGraphToCardLayout shim or its per-kind switch
	// branches. The adapter module owns the mapping outright; the
	// shim was deleted in D32a-impl-8 (no inline callers remained
	// after refreshGovernanceMap was rewritten to call modules
	// directly).
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if strings.Contains(body, "function mapContextGraphToCardLayout(") {
		t.Error("D32a-impl-8: inline mapContextGraphToCardLayout shim must remain removed (call-sites use ExplorerGraph.shell.refresh which dispatches through the adapter)")
	}
	for _, gone := range []string{
		"case 'related_business_service':",
		"case 'ai_system_binding':",
		"case 'authority_summary':",
		"collectedBindings.push({",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32a-impl-2/8: inline body must NOT contain %q (lives in adapter module)", gone)
		}
	}
	// Adapter module retains the production declaration.
	if !strings.Contains(adapterJS, "function mapToCardLayout(") {
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

	// Both call-sites should pass lens, view, id, depth.
	// D32a-impl-8 — refreshGovernanceMap was tightened to a single-
	// line shell.refresh call; loadBusinessServiceRecord in
	// services-view.js uses the same call shape. The conceptual JS
	// surface contains the call.
	allJS := getExplorerAllJS(t, srv)
	if !strings.Contains(allJS, "ExplorerGraph.shell.refresh({") &&
		!strings.Contains(allJS, "shell.refresh({") &&
		!strings.Contains(allJS, "graph.refresh({") {
		t.Error("D32a-impl-2: inline IIFE or services-view.js must invoke ExplorerGraph.shell.refresh")
	}
	if !strings.Contains(allJS, "lens: 'context'") {
		t.Error("D32a-impl-2: production fetch must pass lens: 'context'")
	}

	// Shell module surface must declare refresh + render.
	shellJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-shell.js")
	for _, want := range []string{
		"function refresh(",
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

	// Positive pins: lens-agnostic surface. D37p-clean-1 retired the
	// dead dispatch trio (register / render / clear); the lens-agnostic
	// helper surface remains.
	for _, want := range []string{
		"lensAgnosticConnectorPath",
		"lensAgnosticNodePosition",
		"function clearCanvas(",
		"function addNode(",
		"function addLiveConnector(",
		"function applyVisibilityFilters(",
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("graph-renderer.js missing required lens-agnostic export: %q", want)
		}
	}
}

// TestExplorer_D32aImpl2_Bridge_WiredFromInlineIIFE — historically
// pinned the bridge publication block. D32a-impl-7 removed the bridge
// entirely (no module consumer existed). This test now pins the
// post-removal state: the bridge namespace is absent from active
// rendering paths in the inline IIFE.
func TestExplorer_D32aImpl2_Bridge_WiredFromInlineIIFE(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Negative pins — the bridge publication assignments must NOT
	// reappear. The inline renderer functions are still declared by
	// name; their D32aImpl3 body-content pins are the authoritative
	// production-rendering coverage.
	for _, gone := range []string{
		"window.MIDASExplorerGovernanceMapBridge = window.MIDASExplorerGovernanceMapBridge",
		".renderGovernanceMap        = renderGovernanceMap;",
		".renderGovernanceMapEmpty   = renderGovernanceMapEmpty;",
		".renderGovernanceMapError   = renderGovernanceMapError;",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32a-impl-7: bridge publication must remain removed: %q", gone)
		}
	}
	// The D32a-impl-7 removal note remains in the source as a
	// documented decision for future readers.
	if !strings.Contains(body, "D32a-impl-7 — MIDASExplorerGovernanceMapBridge removed") {
		t.Error("D32a-impl-7: removal note must remain in the inline IIFE so future readers find the decision")
	}
}

// TestExplorer_D32aImpl2_AuthorityEndpointAccessIsScoped pins that
// the Authority Graph endpoint is reached only through the canonical
// path: the typed ExplorerAPI.graphs.authority(...) method declared
// in core/api-client.js, called by the Authority adapter
// (graph/authority/authority-graph-adapter.js).
//
// Originally (D32a-impl-2) the Authority UI was disabled and this
// test enforced "no JS calls Authority at all". D32b-impl-1 enabled
// the Authority lens; the test now permits the adapter to call the
// typed method while keeping every other JS module Authority-fetch-
// free so the routing surface stays single-sourced.
func TestExplorer_D32aImpl2_AuthorityEndpointAccessIsScoped(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Inline HTML body — no literal Authority URL, no direct
	// ExplorerAPI.graphs.authority() call. The HTML may route lens
	// switches through the view module; it does not fetch directly.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if strings.Contains(body, "/v1/graphs/authority") {
		t.Error("D32b-impl-1: index.html must not contain the literal /v1/graphs/authority URL")
	}
	if strings.Contains(body, "ExplorerAPI.graphs.authority(") {
		t.Error("D32b-impl-1: index.html must not call ExplorerAPI.graphs.authority() directly — route through the Authority adapter")
	}

	// Every JS module EXCEPT api-client.js and the Authority adapter
	// must be Authority-fetch-free. The view and inspector reach the
	// endpoint indirectly (the adapter wraps the typed method).
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
		"/explorer/assets/js/graph/authority/authority-graph-view.js",
		"/explorer/assets/js/graph/authority/authority-graph-inspector.js",
	} {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, "/v1/graphs/authority") {
			t.Errorf("D32b-impl-1: %s must not reference /v1/graphs/authority", path)
		}
		if strings.Contains(js, "ExplorerAPI.graphs.authority(") || strings.Contains(js, "api.authority(") {
			t.Errorf("D32b-impl-1: %s must not call the Authority graph endpoint directly — route through window.MIDASExplorerGraph.authorityAdapter", path)
		}
	}

	// The Authority adapter is the single allowed call-site outside
	// the API client. Pin that it really does invoke the typed method
	// (caught a typo in the original implementation).
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	if !strings.Contains(adapterJS, "api.authority(") {
		t.Error("D32b-impl-1: authority-graph-adapter.js must invoke api.authority(...) — the wrapper around the typed API client method")
	}
}

// TestExplorer_D32aImpl2_LegacyEndpointsAbsent pins that the
// pre-D31d endpoint URLs do not return anywhere in the Explorer
// shell or its JS modules. The endpoints were renamed in D31d:
//
//	/v1/authority-graph                 → /v1/graphs/context
//	/v1/businessservices/{id}/governance-map → /v1/graphs/context
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
		// D32h-fix-2f-hotfix-2 — addConnector / addLiveConnector grew
		// optional anchor-side + lens pathFn parameters so Authority
		// connectors can override the shared dominant-axis Bezier.
		// Backward-compatible: existing five-arg calls still work
		// (pathFn defaults to undefined; addConnector falls through to
		// the shared _curvePath).
		"function addConnector(p1, p2, cls, srcAnchor, dstAnchor, pathFn)",
		"function addConnectorHitTarget(",
		"function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls, pathFn)",
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

	// D32a-impl-8 — Inline renderGovernanceMap shim removed. The
	// production renderer is reached through the workbench refresh
	// loop (refreshGovernanceMap) which dispatches via
	// ExplorerGraph.contextView.renderContextGraph directly.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if strings.Contains(body, "function renderGovernanceMap(data) {") {
		t.Error("D32a-impl-8: inline renderGovernanceMap shim must remain removed")
	}
	if !strings.Contains(body, "ExplorerGraph.contextView.renderContextGraph(payloadOrLayout, _gmapRenderCtx)") {
		t.Error("D32a-impl-8: refreshGovernanceMap must dispatch payload directly to ExplorerGraph.contextView.renderContextGraph")
	}
	// Sentinel renderer-body patterns must remain absent from the
	// inline IIFE — they live in context-graph-view.js. (The
	// `connector-ai-binding` class name still appears in inline
	// documentation comments referencing the connector kinds; the
	// renderer-ownership test in D32aImpl3_RendererOwnsProductionPrimitives
	// already pins absence of the class string from the inline call-sites
	// it cares about, so we omit it here.)
	for _, gone := range []string{
		"failModePolicyBadgesForSurface",
		"setGovernanceMapSummary([",
		"isAIRootView ? 'ai:'",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32a-impl-3/8: inline body must NOT contain %q (lives in context-graph-view.js)", gone)
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

	// D32a-impl-8 — Inline shims for the production renderer cluster
	// were removed entirely. Each former shim is now reachable only
	// through its module namespace; the inline IIFE no longer
	// declares any of them.
	gone := []string{
		"function renderGovernanceMap(data) {",
		"function renderGovernanceMapEmpty(message, bsId) {",
		"function renderGovernanceMapError(message) {",
		"function clearGovernanceMapCanvas() {",
		"function addNode(spec, pos) {",
		"function addConnector(p1, p2, cls) {",
		"function addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId, srcLabel, dstLabel) {",
		"function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls) {",
		"function addMoreNode(layerKey, layerLabel, total, rendered, pos) {",
		"function mapContextGraphToCardLayout(",
		"function attachGmapDragHandlers(",
	}
	for _, decl := range gone {
		if strings.Contains(body, decl) {
			t.Errorf("D32a-impl-8: inline shim %q must remain removed", decl)
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
		// Selection clear hook. D32b-debug-3: hook bundle is resolved
		// lazily via _hooks() and bound to a local `hVis` inside
		// applyVisibilityFilters so the clear-selection call sees the
		// post-IIFE hook object, not a stale cached reference.
		"hVis.clearSelection",
	} {
		if !strings.Contains(rendererJS, want) {
			t.Errorf("graph-renderer.js applyVisibilityFilters behavioural pin missing: %q", want)
		}
	}
}

// TestExplorer_D32aImpl3_BridgeIsCompatAlias — D32a-impl-7 removed the
// bridge entirely. This test now pins absence: the inline IIFE must
// not re-declare any production bridge that owns rendering, and no
// graph module may reach into the bridge namespace for production
// dispatch.
func TestExplorer_D32aImpl3_BridgeIsCompatAlias(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// Negative pin: bridge publication assignments must remain absent.
	if strings.Contains(body, "MIDASExplorerGovernanceMapBridge = window.MIDASExplorerGovernanceMapBridge") {
		t.Error("D32a-impl-7: bridge publication block must remain absent")
	}
	if strings.Contains(body, "Production bridge wiring") {
		t.Error("bridge comment must not describe itself as a production bridge")
	}
	// The D32a-impl-7 removal note remains as a documented decision.
	if !strings.Contains(body, "D32a-impl-7 — MIDASExplorerGovernanceMapBridge removed") {
		t.Error("D32a-impl-7: removal note must remain so future readers find the decision")
	}
	// Graph view + shell must not reach into the bridge for production
	// dispatch.
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if strings.Contains(viewJS, "MIDASExplorerGovernanceMapBridge") {
		t.Error("D32a-impl-7: context-graph-view.js must not reference the removed bridge")
	}
	shellJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-shell.js")
	if strings.Contains(shellJS, "function _bridge()") {
		t.Error("D32a-impl-7: graph-shell.js must not declare a _bridge() resolver")
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
	// D32a-impl-8 — the attachGmapDragHandlers shim was removed; the
	// renderer hook bundle invokes the interactions module directly.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if strings.Contains(body, "function attachGmapDragHandlers(") {
		t.Error("D32a-impl-8: inline attachGmapDragHandlers shim must remain removed")
	}
	if !strings.Contains(body, "MIDASExplorerGraph.interactions.attachNodeDragHandlers(node, id)") {
		t.Error("D32a-impl-8: renderer hook bundle must invoke MIDASExplorerGraph.interactions.attachNodeDragHandlers directly")
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
	// D32a-impl-7 — the per-shim delegations (every `function fooShim()
	// { return MIDASExplorerServices.X(...); }` block) were deleted.
	// The remaining inline call-sites now invoke the module directly:
	//   - goBackOrToOwningService → MIDASExplorerServices.showMap(currentSelectedService)
	//   - handleGovernanceMapAction → MIDASExplorerServices.showRecord(action.target_id)
	//   - refreshCoverage hook → MIDASExplorerServices.updateCoverageSummary(...)
	//   - IIFE bootstrap setTimeouts → MIDASExplorerServices.{showCatalogue,loadCatalogue}()
	for _, want := range []string{
		"MIDASExplorerServices.loadCatalogue()",
		"MIDASExplorerServices.showCatalogue()",
		"MIDASExplorerServices.showRecord(action.target_id)",
		"MIDASExplorerServices.showMap(currentSelectedService)",
		"MIDASExplorerServices.updateCoverageSummary",
		"MIDASExplorerServices.init({",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must call module directly via %q", want)
		}
	}
	// Negative pin — shim function declarations must remain absent.
	for _, gone := range []string{
		"function showBusinessServiceRecord(serviceId) {",
		"function showBusinessServiceMap(serviceId) {",
		"function loadBusinessServicesList()",
		"function showServicesCatalogue()",
		"function renderServicesCatalogue(filter)",
		"function renderBusinessServiceRecord(payload)",
		"function setServicesSubView(view)",
		"function showServicesDriftOverview()",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32a-impl-7: inline Services shim must remain removed: %q", gone)
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
	// D32a-impl-7 — Capabilities shims removed. Remaining inline
	// call-sites use the module namespace directly.
	for _, want := range []string{
		"MIDASExplorerCapabilities.loadCatalogue()",
		"MIDASExplorerCapabilities.showRecord(action.target_id)",
		"MIDASExplorerCapabilities.init({}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must call module directly via %q", want)
		}
	}
	// Negative pin — shim function declarations must remain absent.
	for _, gone := range []string{
		"function showCapabilityRecord(capId) {",
		"function loadCapabilitiesList()",
		"function showCapabilitiesCatalogue()",
		"function renderCapabilitiesCatalogue(filter)",
		"function loadCapabilityRecord(capId)",
		"function renderCapabilityRecord(payload)",
		"function setCapabilitiesSubView(view)",
		"function loadCapabilityChildren(capId)",
		"function loadCapabilityBusinessServices(capId)",
		"function loadCapabilityAIBindings(capId)",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32a-impl-7: inline Capabilities shim must remain removed: %q", gone)
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
		"/explorer/assets/js/graph/authority/authority-graph-adapter.js",
		"/explorer/assets/js/graph/authority/authority-graph-view.js",
		"/explorer/assets/js/graph/authority/authority-graph-inspector.js",
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

// ---------------------------------------------------------------------------
// D32a-impl-7 — Compatibility Shim Removal and Index Slimdown
//
// Tests in this section pin the end-state of D32a-impl-7:
//
//   • All 14 Services compatibility shims and all 13 Capabilities
//     compatibility shims have been removed from the inline IIFE.
//   • The MIDASExplorerGovernanceMapBridge publication block has been
//     removed. The bridge namespace is unused by all modules.
//   • The graph-shell.js `render()` method + `_bridge()` resolver
//     have been removed (zero call-sites).
//   • Remaining cross-module call-sites use MIDASExplorerServices.* /
//     MIDASExplorerCapabilities.* directly.
//   • index.html is below 8,200 lines (down from the 8,362 D32a-impl-6
//     baseline).
// ---------------------------------------------------------------------------

// TestExplorer_D32aImpl7_NoServiceShims pins absence of every inline
// Services compatibility shim function. Each declaration listed here
// was removed by D32a-impl-7; cross-module callers now invoke
// MIDASExplorerServices.* directly.
func TestExplorer_D32aImpl7_NoServiceShims(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	gone := []string{
		"function loadBusinessServicesList()",
		"function setServicesSubView(view)",
		"function showServicesDriftOverview()",
		"function showServicesCatalogue()",
		"function showBusinessServiceRecord(serviceId)",
		"function showBusinessServiceMap(serviceId)",
		"function renderServicesView()",
		"function renderServicesCatalogue(filter)",
		"function loadBusinessServiceRecord(serviceId)",
		"function renderRecordFieldGrid(rows)",
		"function renderRecordSection(title, contentHtml)",
		"function renderRelatedList(rows, emptyMessage)",
		"function renderBusinessServiceRecord(payload)",
		"function updateServicesCoverageSummary(records)",
	}
	for _, decl := range gone {
		if strings.Contains(body, decl) {
			t.Errorf("D32a-impl-7: inline Services shim %q must remain removed", decl)
		}
	}
}

// TestExplorer_D32aImpl7_NoCapabilityShims pins absence of every
// inline Capabilities compatibility shim function.
func TestExplorer_D32aImpl7_NoCapabilityShims(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	gone := []string{
		"function setCapabilitiesSubView(view)",
		"function showCapabilitiesCatalogue()",
		"function showCapabilityRecord(capId)",
		"function loadCapabilitiesList()",
		"function renderCapabilitiesCatalogue(filter)",
		"function loadCapabilityRecord(capId)",
		"function loadCapabilityChildren(capId)",
		"function loadCapabilityBusinessServices(capId)",
		"function loadCapabilityAIBindings(capId)",
		"function renderCapabilityRecord(payload)",
		"function renderCapabilityChildrenSection(capId)",
		"function renderCapabilityBusinessServicesSection(capId)",
		"function renderCapabilityAIBindingsSection(capId)",
	}
	for _, decl := range gone {
		if strings.Contains(body, decl) {
			t.Errorf("D32a-impl-7: inline Capabilities shim %q must remain removed", decl)
		}
	}
}

// TestExplorer_D32aImpl7_BridgeRemoved pins that
// MIDASExplorerGovernanceMapBridge is no longer published. The
// publication block, the legacy property assignments, and the
// shell's _bridge() resolver / render() method are all absent.
func TestExplorer_D32aImpl7_BridgeRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// Inline IIFE: the bridge publication assignments must remain gone.
	for _, gone := range []string{
		"window.MIDASExplorerGovernanceMapBridge = window.MIDASExplorerGovernanceMapBridge",
		".renderGovernanceMap        = renderGovernanceMap;",
		".refreshGovernanceMap       = refreshGovernanceMap;",
		".setGovernanceMapStatus     = setGovernanceMapStatus;",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32a-impl-7: bridge publication %q must remain removed", gone)
		}
	}
	// Removal note remains for git-blame readers.
	if !strings.Contains(body, "D32a-impl-7 — MIDASExplorerGovernanceMapBridge removed") {
		t.Error("D32a-impl-7: removal note must remain in the inline IIFE")
	}
	// graph-shell.js must no longer declare the bridge resolver or
	// the render() method that depended on it.
	shellJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-shell.js")
	if strings.Contains(shellJS, "function _bridge()") {
		t.Error("D32a-impl-7: graph-shell.js _bridge() resolver must remain removed")
	}
	if strings.Contains(shellJS, "function render(payload, mount)") {
		t.Error("D32a-impl-7: graph-shell.js render() method must remain removed (no callers)")
	}
	// No module reaches into the bridge for production dispatch.
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
		// Active references (function calls / property access) are
		// what matter; doc-comment mentions are allowed for context.
		// Match `MIDASExplorerGovernanceMapBridge.` (property access)
		// or `window.MIDASExplorerGovernanceMapBridge =` (write).
		if strings.Contains(js, "MIDASExplorerGovernanceMapBridge.") ||
			strings.Contains(js, "window.MIDASExplorerGovernanceMapBridge =") {
			t.Errorf("D32a-impl-7: %s must not actively reference the removed bridge namespace", path)
		}
	}
}

// TestExplorer_D32aImpl7_CallSitesUseModulesDirectly pins that the
// inline call-sites that used to dispatch through shims now invoke
// MIDASExplorerServices.* / MIDASExplorerCapabilities.* directly.
func TestExplorer_D32aImpl7_CallSitesUseModulesDirectly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		// goBackOrToOwningService fallback.
		"MIDASExplorerServices.showMap(currentSelectedService)",
		// handleGovernanceMapAction dispatcher.
		"MIDASExplorerServices.showRecord(action.target_id)",
		"MIDASExplorerCapabilities.showRecord(action.target_id)",
		// refreshCoverage hook.
		"MIDASExplorerServices.updateCoverageSummary",
		// IIFE bootstrap setTimeouts.
		"MIDASExplorerServices.showCatalogue()",
		"MIDASExplorerServices.loadCatalogue()",
		"MIDASExplorerCapabilities.loadCatalogue()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32a-impl-7: inline IIFE must invoke module directly via %q", want)
		}
	}
}

// TestExplorer_D32aImpl7_IndexHtmlReducedBelowImpl6 pins index.html
// below 8,200 lines. The D32a-impl-6 baseline was ~8,362; D32a-impl-7
// shim + bridge removal brought it to ~8,112.
func TestExplorer_D32aImpl7_IndexHtmlReducedBelowImpl6(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 8200 {
		t.Errorf("D32a-impl-7: index.html line count %d exceeds 8200 — shim/bridge removal should land lower", lines)
	}
}

// ---------------------------------------------------------------------------
// D32a-impl-8 — Graph Renderer Inline Cluster Removal
//
// Tests in this section pin the end-state of D32a-impl-8:
//
//   • All 11 inline graph-renderer shims removed: renderGovernanceMap,
//     renderGovernanceMapEmpty, renderGovernanceMapError,
//     clearGovernanceMapCanvas, addNode, addConnector,
//     addConnectorHitTarget, addLiveConnector, addMoreNode,
//     mapContextGraphToCardLayout, attachGmapDragHandlers.
//   • refreshGovernanceMap rewritten to dispatch directly to module
//     functions (ExplorerGraph.contextView.renderContextGraph* / shell.refresh).
//   • The renderer-hook bundle's attachDragHandlers entry now calls
//     MIDASExplorerGraph.interactions.attachNodeDragHandlers directly.
//   • Production rendering primitives are reachable only through
//     module namespaces (no inline shim layer).
//   • index.html is below 8,100 lines.
// ---------------------------------------------------------------------------

// TestExplorer_D32aImpl8_NoInlineRendererShims pins absence of the
// 11 deleted shim function declarations.
func TestExplorer_D32aImpl8_NoInlineRendererShims(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	gone := []string{
		"function renderGovernanceMap(data) {",
		"function renderGovernanceMapEmpty(message, bsId) {",
		"function renderGovernanceMapError(message) {",
		"function clearGovernanceMapCanvas() {",
		"function addNode(spec, pos) {",
		"function addConnector(p1, p2, cls) {",
		"function addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId, srcLabel, dstLabel) {",
		"function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls) {",
		"function addMoreNode(layerKey, layerLabel, total, rendered, pos) {",
		"function mapContextGraphToCardLayout(projection, view) {",
		"function attachGmapDragHandlers(node, nodeId) {",
	}
	for _, decl := range gone {
		if strings.Contains(body, decl) {
			t.Errorf("D32a-impl-8: inline shim %q must remain removed", decl)
		}
	}
}

// TestExplorer_D32aImpl8_RefreshDispatchIsModuleDirect pins that
// refreshGovernanceMap dispatches directly to context-graph-view.js
// for empty/error/success and to graph-shell.js for the fetch.
func TestExplorer_D32aImpl8_RefreshDispatchIsModuleDirect(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"function refreshGovernanceMap()",
		"ExplorerGraph.shell.refresh({ lens: 'context', view: fetchView, id: rootId, depth: 5 })",
		"ExplorerGraph.contextView.renderContextGraphEmpty(",
		"ExplorerGraph.contextView.renderContextGraphError(",
		"ExplorerGraph.contextView.renderContextGraph(payloadOrLayout, _gmapRenderCtx)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32a-impl-8: refreshGovernanceMap must dispatch via %q", want)
		}
	}
}

// TestExplorer_D32aImpl8_RendererHookInvokesInteractionsDirectly pins
// that the renderer hook bundle (`_rendererHooks.attachDragHandlers`)
// invokes the interactions module's attachNodeDragHandlers directly
// rather than routing through the deleted inline shim.
func TestExplorer_D32aImpl8_RendererHookInvokesInteractionsDirectly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "MIDASExplorerGraph.interactions.attachNodeDragHandlers(node, id)") {
		t.Error("D32a-impl-8: _rendererHooks.attachDragHandlers must invoke MIDASExplorerGraph.interactions.attachNodeDragHandlers directly")
	}
	if strings.Contains(body, "attachGmapDragHandlers(node, id)") {
		t.Error("D32a-impl-8: renderer hook must not route through the (removed) attachGmapDragHandlers shim")
	}
}

// TestExplorer_D32aImpl8_IndexHtmlReducedBelowImpl7 pins index.html
// below 8,100 lines (D32a-impl-7 left it at 8,111; D32a-impl-8 shim
// + refresh-rewrite brings it to ~8,030).
func TestExplorer_D32aImpl8_IndexHtmlReducedBelowImpl7(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 8100 {
		t.Errorf("D32a-impl-8: index.html line count %d exceeds 8100 — graph-renderer shim removal should land lower", lines)
	}
}

// ---------------------------------------------------------------------------
// D32a-impl-9 — Context Graph Evidence/Drift Tray Extraction
//
// The Context Graph "Runtime Evidence" tray (drift + activity panels,
// expand/collapse wiring, metric/range selectors, demo-signal
// synthesis, and the selection-aware filter) was the largest cohesive
// inline subsystem remaining inside the IIFE in index.html (~990
// lines). D32a-impl-9 extracts it to a new module
// /explorer/assets/js/graph/context/context-evidence-tray.js owned by
// `window.MIDASExplorerGraph.contextEvidenceTray`.
//
// Tests in this section pin:
//
//   • The module file is served and exposes the documented namespace.
//   • The 14 tray functions and 6 tray state variables live in the
//     module body, not the inline IIFE.
//   • The 14 inline declarations are gone from index.html.
//   • The inspector hook (_inspectorHooks.notifyEvidenceTraySelectionChanged)
//     dispatches through the module namespace rather than calling a
//     deleted inline shim.
//   • index.html is below 7,100 lines (D32a-impl-8 left it at ~8,030;
//     D32a-impl-9 brings it to ~7,065).
// ---------------------------------------------------------------------------

// TestExplorer_D32aImpl9_EvidenceTrayModuleServedAndExposesNamespace
// pins that the new context-evidence-tray.js asset is served and
// publishes its public surface on window.MIDASExplorerGraph.
func TestExplorer_D32aImpl9_EvidenceTrayModuleServedAndExposesNamespace(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	modBody := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-evidence-tray.js")
	if !strings.Contains(modBody, "window.MIDASExplorerGraph.contextEvidenceTray = {") {
		t.Fatal("D32a-impl-9: module must publish window.MIDASExplorerGraph.contextEvidenceTray = { ... }")
	}
	for _, want := range []string{
		"init:",
		"notifySelectionChanged:",
		"applyState:",
		"render:",
	} {
		if !strings.Contains(modBody, want) {
			t.Errorf("D32a-impl-9: contextEvidenceTray surface must expose %q", want)
		}
	}
	// Shell script tag is wired so the asset is loaded by the browser
	// in the same cascade the rest of the graph modules use.
	htmlBody := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(htmlBody,
		`<script src="/explorer/assets/js/graph/context/context-evidence-tray.js"></script>`) {
		t.Error("D32a-impl-9: index.html must <script> the context-evidence-tray.js module")
	}
}

// TestExplorer_D32aImpl9_TrayFunctionsLiveInModule pins that the 14
// tray functions and 6 tray state vars are owned by the module body.
func TestExplorer_D32aImpl9_TrayFunctionsLiveInModule(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	modBody := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-evidence-tray.js")
	for _, want := range []string{
		// Tray demo-data synthesis primitives
		"function hashGmapDemoSeed(",
		"function buildDemoDriftSeries(",
		"function buildDemoGovernanceSignal(",
		"function getGmapEvidenceSignalSemantics(",
		// Tray renderers
		"function renderGmapEvidenceTrayChart(",
		"function renderGmapEvidenceTrayTiles(",
		"function renderGmapEvidenceTrayDriftPanel(",
		"function renderGmapEvidenceTrayActivityPanel(",
		// Tray header + state apply + selection notify
		"function updateGmapEvidenceTrayHeader(",
		"function notifyGmapEvidenceTraySelectionChanged(",
		"function filterGmapEvidenceActivityForSelection(",
		"function applyGmapEvidenceTrayState(",
		"function wireGmapEvidenceTraySelectors(",
		"function gmapNodeKindLabel(",
		// Tray module-private state declarations
		"let gmapEvidenceTrayExpanded",
		"let gmapEvidenceTrayActiveTab",
		"let gmapEvidenceActivityItems",
		"let gmapEvidenceActivityLoading",
		"let gmapEvidenceActivityError",
		"let gmapEvidenceActivityLoadedOnce",
	} {
		if !strings.Contains(modBody, want) {
			t.Errorf("D32a-impl-9: tray module must own %q", want)
		}
	}
}

// TestExplorer_D32aImpl9_InlineTrayDeclarationsRemoved pins that the
// 14 tray function declarations and 6 tray state vars are gone from
// the index.html inline body. The module owns them now.
func TestExplorer_D32aImpl9_InlineTrayDeclarationsRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	gone := []string{
		"function hashGmapDemoSeed(",
		"function buildDemoDriftSeries(",
		"function buildDemoGovernanceSignal(",
		"function getGmapEvidenceSignalSemantics(",
		"function renderGmapEvidenceTrayChart(",
		"function renderGmapEvidenceTrayTiles(",
		"function renderGmapEvidenceTrayDriftPanel(",
		"function renderGmapEvidenceTrayActivityPanel(",
		"function updateGmapEvidenceTrayHeader(",
		"function notifyGmapEvidenceTraySelectionChanged(",
		"function filterGmapEvidenceActivityForSelection(",
		"function applyGmapEvidenceTrayState(",
		"function wireGmapEvidenceTraySelectors(",
		"function gmapNodeKindLabel(",
		"let gmapEvidenceTrayExpanded",
		"let gmapEvidenceTrayActiveTab",
		"let gmapEvidenceActivityItems",
		"let gmapEvidenceActivityLoading",
		"let gmapEvidenceActivityError",
		"let gmapEvidenceActivityLoadedOnce",
	}
	for _, decl := range gone {
		if strings.Contains(body, decl) {
			t.Errorf("D32a-impl-9: inline declaration %q must be gone — owned by the module", decl)
		}
	}
}

// TestExplorer_D32aImpl9_InspectorHookDispatchesToModule pins that the
// inspector hook bundle forwards selection-change notifications to the
// module namespace rather than calling a deleted inline shim.
func TestExplorer_D32aImpl9_InspectorHookDispatchesToModule(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body,
		"window.MIDASExplorerGraph.contextEvidenceTray.notifySelectionChanged()") {
		t.Error("D32a-impl-9: _inspectorHooks.notifyEvidenceTraySelectionChanged must dispatch via MIDASExplorerGraph.contextEvidenceTray.notifySelectionChanged()")
	}
	// The boot wiring must call init() with the hook bundle so the
	// tray can resolve gmapData / view / root + camera + selection
	// callbacks without leaking inline globals into the module.
	if !strings.Contains(body, "MIDASExplorerGraph.contextEvidenceTray.init({") {
		t.Error("D32a-impl-9: boot wiring must invoke MIDASExplorerGraph.contextEvidenceTray.init({ hooks: { ... } })")
	}
	for _, want := range []string{
		"getGmapData:",
		"getCurrentGraphView:",
		"getCurrentGraphRootId:",
		"focusGmapOnNode:",
		"selectNode:",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32a-impl-9: contextEvidenceTray.init hook bundle must include %q", want)
		}
	}
}

// TestExplorer_D32aImpl9_IndexHtmlReducedBelowImpl8 pins index.html
// below 7,500 lines. Successive ceilings:
//   - D32a-impl-8 left it at ~8,030
//   - D32a-impl-9 tray extraction brought it to ~7,065
//   - D32b-impl-1 shell wiring added ~30 lines (~7,099)
//   - D32b-impl-2 Service Workbench mode toolbar + Authority panels
//     container + diagnostics/posture wiring added ~210 lines (~7,312)
//   - D32b-impl-3 unified drawer registration + icon-only toolbar
//     SVG markup + Authority drawer provider wiring added ~110 lines
//     (~7,425), partially offset by removing the .authority-graph-
//     panels overlay markup. The drawer LOGIC itself lives in a
//     separate module (graph-drawer.js) — only the registrations
//     are inline.
//   - D32b-debug-1 — lens-race fix added ~14 lines of doc comments
//     plus the lens guard at the top of refreshGovernanceMap and the
//     pre-seed of selectedGraphLens in setWorkbenchMode('authority')
//     (~7,514). The fix prevents Context and Authority modes from
//     rendering the same canvas — see the operator-reported bug pinned
//     by TestExplorer_D32bDebug1_*.
//
// The 8,000-line ceiling from the D32a tranche prompt is enforced
// here as a 7,685 ceiling so a future inline regression is loud
// rather than silent.
//   - D32h-fix-1 — bumped 7,550 → 7,650 (+100 headroom) to absorb the
//     lens-aware bottom-workbench DOM addition (~58 lines: a sibling
//     <section id="gmap-authority-workbench"> next to the existing
//     #gmap-evidence-tray, with five-tab markup). The Drift Analytics
//     tray was NOT removed; both lens trays now coexist and CSS routes
//     visibility from body[data-graph-lens]. The Authority canvas
//     itself, the inspector, and Context behaviour are untouched.
//   - D33x-help-1 — bumped 7,650 → 7,660 (+10 headroom) for the
//     toolbar Help button + two `<script>` tags loading the help
//     modules. Both the button and the script-tag block are followed
//     by single-line comments so the extraction discipline is
//     preserved; the bump only accommodates the legitimate +5 lines.
//   - D33x-list-mode — bumped 7,660 → 7,685 (+25 headroom) for the
//     lens-aware Form / Records branch (`if (pocActive && lens ===
//     'authority') poc.setViewMode('list')`) and the Authority-branch
//     exit hook that returns the graph to spine layout when the
//     operator clicks the Authority button while in List Mode. Both
//     additions are defensive guards (typeof checks, try/catch) so
//     they cannot regress Context Graph or non-PoC sessions.
//   - D34b-context-cytoscape-html-overlay-card-parity-spike — bumped
//     7,685 → 7,692 (+7 headroom) for the gated Context HTML-overlay
//     spike: one `<link>` for its CSS, one `<script>` for its
//     module, and a one-line annotated comment per asset. Module is
//     self-gated on `?cytoscape=1&contextHtmlCards=1`; loading the
//     script with the gate closed early-returns and exposes only
//     `isActive`. Removable in four lines.
//   - D35a-midas-graph-viewport-foundation — bumped 7,692 → 7,720
//     (+28 headroom) for the additive `.midas-graph-viewport` +
//     `.midas-graph-renderer-slot` wrapper around the graph DOM. The
//     wrappers add: viewport open + heredoc comment (12 lines);
//     renderer-slot open + heredoc comment (5 lines); slot close
//   - viewport close-marker (2 lines); +4 lines for the extra
//     indentation produced by wrapping `.governance-map-canvas-scroll`
//     two levels deeper. Removable by inverting the wrap.
//   - D37h-authority-cytoscape-navigation-toolbar — bumped 7,720 →
//     7,730 (+10 headroom) for the three new camera-cluster controls
//     (zoom %, zoom-to-selected, reset-view) added inside
//     `.gmap-camera-cluster`. Each control occupies one line; one
//     annotation comment fronts the cluster. Removable in 4 lines if
//     the camera/navigation tranche is reverted.
//   - D37m-impl-1-authority-canvas-edge-context-tabs — bumped 7,730 →
//     7,750 (+20 headroom) for the static skeleton of the right-side
//     canvas-edge tab strip + pane (three tab buttons, pane with
//     header/body/footer; ~12 lines of markup), one annotation
//     comment, plus one `<link>` and one `<script>` (with their
//     annotation comments). Removable in ~16 lines if the tranche
//     is reverted.
//   - D37o-impl-2-context-strategic-renderer-skeleton — bumped 7,750
//     → 7,770 (+20 headroom) for the strategic Context renderer
//     wiring: one `<link>` for its CSS skeleton, three `<script>`
//     tags for the D37o-impl-1 model modules (card / connector /
//     layout), one `<script>` for the renderer skeleton module, and
//     two annotation comments. Renderer is opt-in via
//     `?contextRenderer=strategic`; legacy Context renderer remains
//     the default. Removable in ~8 lines if reverted.
func TestExplorer_D32aImpl9_IndexHtmlReducedBelowImpl8(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 7820 {
		t.Errorf("D32a-impl-9 / D32b-debug-1 / D32h-fix-1 / D33x-help-1 / D33x-list-mode / D34b / D35a / D37h / D37m / D37o-impl-2 / D37o-toolbar-1: index.html line count %d exceeds 7820 — extraction discipline should hold", lines)
	}
}

// ---------------------------------------------------------------------------
// D32b-impl-1 — Authority Graph Lens Renderer
//
// Tests in this section pin the end-state of D32b-impl-1:
//
//   • Three new Authority lens modules are served and register their
//     namespaces:
//       - graph/authority/authority-graph-adapter.js
//       - graph/authority/authority-graph-view.js
//       - graph/authority/authority-graph-inspector.js
//   • authority-graph.css ships in the cascade.
//   • The Authority lens-switcher button is enabled (not is-disabled,
//     not aria-disabled, no "coming next" copy).
//   • #graph/authority is registered with the router and drives
//     ExplorerGraph.authorityView.refresh.
//   • The Authority view reaches the /v1/graphs/authority endpoint
//     via ExplorerGraph.shell.refresh({lens:'authority',...}) or
//     authorityAdapter.fetch(...) — never via direct fetch().
//   • The adapter supports exactly the seven Authority node kinds
//     and exactly the seven Authority edge kinds; it does NOT carry
//     Context-only kinds.
//   • The inspector ships a formatter for every Authority node kind.
//   • graph-renderer.js carries no Authority-specific node-kind
//     identifiers (the renderer remains lens-agnostic).
//   • index.html does not contain inline Authority renderer logic.
//   • Authority empty state copy exists for the no-selected-service
//     branch.
//   • Diagnostics + surface_posture panels remain unimplemented in
//     this tranche (D32b-impl-2 scope).
// ---------------------------------------------------------------------------

// d32bImpl1AuthorityNodeKinds is the canonical client-side allow-list
// pinned by D32b-impl-1. Backend changes that alter the set are
// expected to land alongside a frontend change.
var d32bImpl1AuthorityNodeKinds = []string{
	"business_service",
	"decision_surface",
	"authority_profile",
	"authority_grant",
	"agent",
	"fail_mode_policy",
	"escalation_target",
}

// d32bImpl1AuthorityEdgeKinds is the canonical client-side edge-kind
// allow-list pinned by D32b-impl-1.
var d32bImpl1AuthorityEdgeKinds = []string{
	"business_service_has_surface",
	"surface_uses_profile",
	"profile_has_grant",
	"grant_authorises_agent",
	"surface_has_fail_mode_policy",
	"business_service_has_fail_mode_policy",
	"profile_escalates_to",
}

// d32bImpl1ContextOnlyNodeKinds — the renderer namespace must never
// claim to support these.
var d32bImpl1ContextOnlyNodeKinds = []string{
	"capability",
	"process",
	"ai_system",
	"ai_system_binding",
	"authority_summary",
	"coverage",
}

// TestExplorer_D32bImpl1_AuthorityModulesServedAndExposeNamespace
// pins that all three new Authority lens modules are served at
// their canonical paths and publish their public namespaces.
func TestExplorer_D32bImpl1_AuthorityModulesServedAndExposeNamespace(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, c := range []struct {
		path string
		ns   string
	}{
		{
			path: "/explorer/assets/js/graph/authority/authority-graph-adapter.js",
			ns:   "window.MIDASExplorerGraph.authorityAdapter",
		},
		{
			path: "/explorer/assets/js/graph/authority/authority-graph-view.js",
			ns:   "window.MIDASExplorerGraph.authorityView",
		},
		{
			path: "/explorer/assets/js/graph/authority/authority-graph-inspector.js",
			ns:   "window.MIDASExplorerGraph.authorityInspector",
		},
	} {
		rec := performRequest(t, srv, http.MethodGet, c.path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("D32b-impl-1: %s want 200, got %d", c.path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), c.ns+" = {") {
			t.Errorf("D32b-impl-1: %s must publish %s = { ... }", c.path, c.ns)
		}
	}
	// The Authority CSS file must also be served.
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/css/authority-graph.css", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("D32b-impl-1: /explorer/assets/css/authority-graph.css want 200, got %d", rec.Code)
	}
	// The lens-CSS cascade must include the authority-graph stylesheet.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `<link rel="stylesheet" href="/explorer/assets/css/authority-graph.css">`) {
		t.Error("D32b-impl-1: index.html must include the authority-graph.css <link> in head")
	}
}

// TestExplorer_D32bImpl1_AuthorityScriptsLoadedInOrder pins that the
// Authority module <script> tags load in dependency order so namespace
// lookups resolve at boot without a poll guard. D32b-impl-2 added the
// diagnostics + surface posture panel modules; the view loads LAST
// because it dispatches to those panels after a successful render.
//
// Required order:
//
//	adapter → inspector → diagnostics-panel → surface-posture-panel → view
func TestExplorer_D32bImpl1_AuthorityScriptsLoadedInOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	adapterTag := `<script src="/explorer/assets/js/graph/authority/authority-graph-adapter.js"></script>`
	inspectorTag := `<script src="/explorer/assets/js/graph/authority/authority-graph-inspector.js"></script>`
	diagnosticsTag := `<script src="/explorer/assets/js/graph/authority/authority-diagnostics-panel.js"></script>`
	postureTag := `<script src="/explorer/assets/js/graph/authority/authority-surface-posture-panel.js"></script>`
	viewTag := `<script src="/explorer/assets/js/graph/authority/authority-graph-view.js"></script>`
	for _, want := range []string{adapterTag, inspectorTag, diagnosticsTag, postureTag, viewTag} {
		if !strings.Contains(body, want) {
			t.Fatalf("D32b-impl-2: missing <script> tag: %s", want)
		}
	}
	idx := func(s string) int { return strings.Index(body, s) }
	a, i, d, p, v := idx(adapterTag), idx(inspectorTag), idx(diagnosticsTag), idx(postureTag), idx(viewTag)
	if !(a < i && i < d && d < p && p < v) {
		t.Errorf("D32b-impl-2: Authority scripts must load in order adapter→inspector→diagnostics-panel→surface-posture-panel→view (got adapter=%d, inspector=%d, diagnostics=%d, posture=%d, view=%d)", a, i, d, p, v)
	}
}

// TestExplorer_D32bImpl1_AuthorityAdapterSurface pins the documented
// adapter surface: fetch / normalise / nodeKindLabel / nodeKindCategory
// / edgeKindLabel / connectorClassForEdge / nodeTypedData / nodeBadges
// / NODE_KINDS / EDGE_KINDS. The adapter is the lens's only place
// for Authority-specific decisions.
func TestExplorer_D32bImpl1_AuthorityAdapterSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	for _, want := range []string{
		"fetch:",
		"normalise:",
		"nodeKindLabel:",
		"nodeKindCategory:",
		"edgeKindLabel:",
		"connectorClassForEdge:",
		"nodeTypedData:",
		"nodeBadges:",
		"NODE_KINDS:",
		"EDGE_KINDS:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32b-impl-1: authority-graph-adapter.js surface must expose %q", want)
		}
	}
}

// TestExplorer_D32bImpl1_AuthorityAdapterCarriesExactlySevenNodeKinds
// pins that the adapter's NODE_KINDS array contains exactly the seven
// canonical strings — no more, no fewer, no typos.
func TestExplorer_D32bImpl1_AuthorityAdapterCarriesExactlySevenNodeKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	// Each canonical kind must appear in a quoted string literal.
	for _, kind := range d32bImpl1AuthorityNodeKinds {
		needle := "'" + kind + "'"
		if !strings.Contains(js, needle) {
			t.Errorf("D32b-impl-1: authority adapter must declare node kind %q", kind)
		}
	}
	// The adapter exports an explicit FORBIDDEN list for tests — pin
	// its presence + that each Context-only kind appears inside it.
	// We scope the negative pin to the NODE_KIND_LABELS / NODE_KINDS /
	// CATEGORY tables: a Context-only kind quoted there would be a
	// semantic claim, whereas quoting it inside _FORBIDDEN_*
	// (where it must appear) is intentional.
	if !strings.Contains(js, "_FORBIDDEN_CONTEXT_NODE_KINDS") {
		t.Error("D32b-impl-1: authority adapter must expose _FORBIDDEN_CONTEXT_NODE_KINDS for test introspection")
	}
	// Pin that the canonical claim-tables don't host Context kinds.
	// Each table is delimited by its top-of-block declaration name;
	// we scan the slice strictly between the table name and the next
	// `var ` (the adapter declares each table at module scope).
	for _, table := range []string{"NODE_KINDS = Object.freeze([", "NODE_KIND_LABELS = Object.freeze({", "NODE_KIND_CATEGORY = Object.freeze({"} {
		idx := strings.Index(js, table)
		if idx < 0 {
			t.Errorf("D32b-impl-1: authority adapter missing claim-table %q", table)
			continue
		}
		// Find the matching closing line. Tables are short; bounding
		// at the next `);` for the array or `});` for the object is
		// sufficient because the freeze-call is closed on its own line.
		var endTok string
		if strings.HasSuffix(table, "[") {
			endTok = "]);"
		} else {
			endTok = "});"
		}
		endRel := strings.Index(js[idx:], endTok)
		if endRel < 0 {
			t.Errorf("D32b-impl-1: authority adapter claim-table %q has no closing delimiter", table)
			continue
		}
		region := js[idx : idx+endRel]
		for _, banned := range d32bImpl1ContextOnlyNodeKinds {
			if strings.Contains(region, "'"+banned+"'") {
				t.Errorf("D32b-impl-1: authority adapter claim-table %q must not include Context-only kind %q", table, banned)
			}
		}
	}
}

// TestExplorer_D32bImpl1_AuthorityAdapterCarriesExactlySevenEdgeKinds
// pins the seven Authority edge kinds.
func TestExplorer_D32bImpl1_AuthorityAdapterCarriesExactlySevenEdgeKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	for _, kind := range d32bImpl1AuthorityEdgeKinds {
		needle := "'" + kind + "'"
		if !strings.Contains(js, needle) {
			t.Errorf("D32b-impl-1: authority adapter must declare edge kind %q", kind)
		}
	}
}

// TestExplorer_D32bImpl1_AuthorityViewFetchesViaShellOrAdapter
// confirms the Authority lens reaches the endpoint exclusively
// through the shell/adapter — never via direct fetch().
func TestExplorer_D32bImpl1_AuthorityViewFetchesViaShellOrAdapter(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	// The view orchestrates fetches through the shell.
	if !strings.Contains(viewJS, "shell.refresh(") {
		t.Error("D32b-impl-1: authority-graph-view.js must call shell.refresh({lens:'authority',...})")
	}
	// The view must NOT direct-fetch the Authority endpoint.
	if strings.Contains(viewJS, "fetch('/v1/graphs/authority") || strings.Contains(viewJS, `fetch("/v1/graphs/authority`) {
		t.Error("D32b-impl-1: authority-graph-view.js must not call fetch('/v1/graphs/authority...') directly")
	}
	// The view passes 'service' view + a depth default.
	if !strings.Contains(viewJS, "'service'") && !strings.Contains(viewJS, `"service"`) {
		t.Error("D32b-impl-1: authority-graph-view.js must specify view='service' on the refresh call")
	}
	// The adapter is the single typed-method caller.
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-adapter.js")
	if !strings.Contains(adapterJS, "api.authority(") {
		t.Error("D32b-impl-1: authority-graph-adapter.js must call api.authority(...) (the canonical typed method)")
	}
}

// TestExplorer_D32bImpl1_AuthorityInspectorSupportsAllSevenKinds pins
// that the inspector ships a formatter for every Authority node kind.
// The formatter dispatch table is keyed by kind; one entry must exist
// per kind. We do not pin the inner field-list in this test because
// the inspector test for required-field coverage runs alongside.
func TestExplorer_D32bImpl1_AuthorityInspectorSupportsAllSevenKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-inspector.js")
	for _, kind := range d32bImpl1AuthorityNodeKinds {
		// Each formatter table entry is `<kind>:  _format…` —
		// pin the kind key followed by a colon.
		needle := kind + ":"
		if !strings.Contains(js, needle) {
			t.Errorf("D32b-impl-1: authority-graph-inspector.js must register a formatter for %q", kind)
		}
	}
}

// TestExplorer_D32bImpl1_AuthorityInspectorCoversRequiredFields pins
// the per-kind required-fields contract from the D32b-impl-1 prompt.
// Each field must appear as a quoted token in the inspector source.
//
// D33a-spike-2g-impl-5e — the three-block content model promoted the
// `name` field out of the per-kind field rows and into the
// selected-node title (rendered via `insp.setName(projLabel || projId)`
// from the projection's `_label`, which the adapter sources from
// `node.label`, which is the entity's name). The per-kind required-
// fields lists below were narrowed accordingly: `name` is no longer
// required to appear as a `['name', …]` row. The title-source
// contract is verified by a sibling assertion below.
func TestExplorer_D32bImpl1_AuthorityInspectorCoversRequiredFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-inspector.js")
	type fieldSet struct {
		kind   string
		fields []string
	}
	required := []fieldSet{
		{"business_service", []string{"status", "owner", "service_type", "fail_mode_policy_id"}},
		{"decision_surface", []string{"version", "status", "process_id", "business_service_id", "effective_policy_source", "effective_policy_id", "inherits_bs_policy"}},
		{"authority_profile", []string{"version", "surface_id", "status", "validity_status", "confidence_threshold", "consequence_threshold", "escalation_mode", "escalation_target_id", "fail_mode"}},
		{"authority_grant", []string{"profile_id", "agent_id", "status", "validity_status", "capabilities", "constraints"}},
		{"agent", []string{"type", "owner", "model_version", "operational_state"}},
		{"fail_mode_policy", []string{"version", "status", "effective_date", "effective_until", "business_owner", "technical_owner", "origin", "managed", "rule_count_by_class"}},
		{"escalation_target", []string{"version", "kind", "handle", "status", "effective_date", "effective_until", "business_owner", "technical_owner"}},
	}
	for _, fs := range required {
		for _, f := range fs.fields {
			// Each formatter writes `['<field>', d.<field> …]`. The
			// quoted-token pin is the cheapest signal; if a field is
			// renamed, both occurrences fail together.
			if !strings.Contains(js, "'"+f+"'") {
				t.Errorf("D32b-impl-1: inspector must surface field %q for kind %q", f, fs.kind)
			}
		}
	}
	// post-impl-5e contract: the entity name flows to the selected-
	// node title via setName, not via a per-kind field row.
	if !strings.Contains(js, "insp.setName(") {
		t.Error("D32b-impl-1 (post-impl-5e): inspector must call insp.setName(...) — entity name flows to the selected-node title, not to a `['name', …]` field row")
	}
}

// TestExplorer_D32bImpl1_LensSwitcherAuthorityEnabled — sibling assertion
// to TestExplorer_D32a_HTML_LensSwitcher, scoped specifically to the
// D32b-impl-1 enabling of the Authority button. Kept separate so
// future readers find the D32b decision when grepping.
func TestExplorer_D32bImpl1_LensSwitcherAuthorityEnabled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// The Authority button must still carry data-lens="authority".
	authBtnIdx := strings.Index(body, `data-lens="authority"`)
	if authBtnIdx < 0 {
		t.Fatal("D32b-impl-1: Authority lens button missing data-lens=\"authority\"")
	}
	// Scope: re-anchor on the enclosing <button … > tag.
	tagStart := strings.LastIndex(body[:authBtnIdx], "<button")
	tagEnd := strings.Index(body[authBtnIdx:], ">")
	if tagStart < 0 || tagEnd <= 0 {
		t.Fatal("D32b-impl-1: could not isolate Authority lens button markup")
	}
	authBtn := body[tagStart : authBtnIdx+tagEnd+1]
	if strings.Contains(authBtn, "is-disabled") {
		t.Error("D32b-impl-1: Authority button must not have .is-disabled")
	}
	if strings.Contains(authBtn, "aria-disabled=") {
		t.Error("D32b-impl-1: Authority button must not declare aria-disabled")
	}
	if strings.Contains(authBtn, "coming next") {
		t.Error("D32b-impl-1: Authority button must not carry 'coming next' copy")
	}
}

// TestExplorer_D32bImpl1_GraphAuthorityRouteDrivesView pins that the
// #graph/authority route is registered with the new router AND that
// the handler activates the Authority lens + drives a refresh.
func TestExplorer_D32bImpl1_GraphAuthorityRouteDrivesView(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "ExplorerRouter.register('graph/authority'") {
		t.Error("D32b-impl-1: index.html must register a router handler for graph/authority")
	}
	if !strings.Contains(body, "ExplorerGraph.shell.setActiveLens('authority')") {
		t.Error("D32b-impl-1: graph/authority route must call shell.setActiveLens('authority')")
	}
	if !strings.Contains(body, "ExplorerGraph.authorityView.refresh(") {
		t.Error("D32b-impl-1: graph/authority route must call authorityView.refresh(...)")
	}
	// The Context route handler must remain present so the back-and-
	// forth lens switch keeps Context rendering correct.
	if !strings.Contains(body, "ExplorerRouter.register('graph/context'") {
		t.Error("D32b-impl-1: graph/context route handler must remain registered")
	}
}

// TestExplorer_D32bImpl1_RendererRemainsLensAgnostic pins that
// graph-renderer.js does not carry any Authority-specific node-kind
// or edge-kind string literals. Lens-specific decisions belong to
// the adapter and view modules.
func TestExplorer_D32bImpl1_RendererRemainsLensAgnostic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")
	// Authority node kinds with the leading-dot scope ("'authority_…'")
	// are pin-safe — they are unique enough to not collide with
	// substrings of unrelated identifiers.
	for _, kind := range []string{
		"'authority_profile'",
		"'authority_grant'",
		"'escalation_target'",
	} {
		if strings.Contains(js, kind) {
			t.Errorf("D32b-impl-1: graph-renderer.js must remain lens-agnostic (found %q)", kind)
		}
	}
	for _, edge := range []string{
		"'business_service_has_surface'",
		"'surface_uses_profile'",
		"'profile_has_grant'",
		"'grant_authorises_agent'",
		"'profile_escalates_to'",
	} {
		if strings.Contains(js, edge) {
			t.Errorf("D32b-impl-1: graph-renderer.js must remain lens-agnostic (found edge %q)", edge)
		}
	}
}

// TestExplorer_D32bImpl1_IndexHtmlHasNoInlineAuthorityRenderer pins
// that the Authority lens UI is module-owned: no inline render*Authority*
// function, no inline addAuthorityNode, no inline mapAuthorityGraphTo*,
// no inline ExplorerAPI.graphs.authority(...) call.
func TestExplorer_D32bImpl1_IndexHtmlHasNoInlineAuthorityRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	gone := []string{
		"function renderAuthorityGraph(",
		"function renderAuthorityGraphEmpty(",
		"function renderAuthorityGraphError(",
		"function mapAuthorityGraphToCardLayout(",
		"function addAuthorityNode(",
	}
	for _, decl := range gone {
		if strings.Contains(body, decl) {
			t.Errorf("D32b-impl-1: Authority renderer must remain module-owned — %q must not appear inline", decl)
		}
	}
	// Inline IIFE must not invoke the typed API method directly — the
	// adapter owns that call-site.
	if strings.Contains(body, "ExplorerAPI.graphs.authority(") {
		t.Error("D32b-impl-1: index.html must not call ExplorerAPI.graphs.authority() directly — route through window.MIDASExplorerGraph.authorityAdapter")
	}
}

// TestExplorer_D32bImpl1_AuthorityViewHasEmptyState pins that the
// "no business service selected" empty state copy exists in the view.
// The exact wording is the copy used by the renderer; D32b-impl-2 may
// refine it, but for D32b-impl-1 it must be present.
func TestExplorer_D32bImpl1_AuthorityViewHasEmptyState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, "Select a business service") {
		t.Error("D32b-impl-1: authority-graph-view.js must render an empty-state when no business service is selected")
	}
	// The error state must also be present, with distinguishable copy.
	if !strings.Contains(js, "Authority Graph fetch failed") {
		t.Error("D32b-impl-1: authority-graph-view.js must surface a fetch-failure error state")
	}
	// The empty/error overlays are namespaced with .authority-graph-*
	// so the styling test can pin them.
	if !strings.Contains(js, "authority-graph-overlay") {
		t.Error("D32b-impl-1: authority-graph-view.js must use the .authority-graph-overlay namespace for overlays")
	}
}

// TestExplorer_D32bImpl1_GraphTypesAlignedWithBackend pins that the
// graph-types.js client-side allow-lists match the backend Authority
// Graph projection (NodeKind*/EdgeKind* constants in
// internal/graph/authority/projection.go). The previous frontend
// allow-list was stale; D32b-impl-1 corrected it.
func TestExplorer_D32bImpl1_GraphTypesAlignedWithBackend(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-types.js")
	for _, kind := range d32bImpl1AuthorityNodeKinds {
		needle := "'" + kind + "'"
		if !strings.Contains(js, needle) {
			t.Errorf("D32b-impl-1: graph-types.js AUTHORITY_NODE_KINDS must list %q", kind)
		}
	}
	for _, edge := range d32bImpl1AuthorityEdgeKinds {
		needle := "'" + edge + "'"
		if !strings.Contains(js, needle) {
			t.Errorf("D32b-impl-1: graph-types.js AUTHORITY_EDGE_KINDS must list %q", edge)
		}
	}
	// Stale legacy edge names must be gone.
	for _, stale := range []string{
		"'owns'",
		"'evaluates_at'",
		"'governs'",
		"'binds'",
		"'escalates_via'",
		"'rolls_up_to'",
		"'governs_with'",
	} {
		if strings.Contains(js, stale) {
			t.Errorf("D32b-impl-1: graph-types.js must drop the stale Authority edge kind %q", stale)
		}
	}
}

// TestExplorer_D32bImpl1_GraphShellLensAware pins that the shell can
// host both lenses. The disabled-lenses map must no longer hard-code
// authority; the adapter lookup must be lens-keyed.
func TestExplorer_D32bImpl1_GraphShellLensAware(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-shell.js")
	if strings.Contains(js, "_disabledLenses = { authority: true }") ||
		strings.Contains(js, "_disabledLenses = {authority: true}") {
		t.Error("D32b-impl-1: graph-shell.js must not hard-code authority as a disabled lens")
	}
	// Lens-keyed adapter lookup: the shell resolves the adapter by
	// lens name, not by the literal `contextAdapter` field.
	if !strings.Contains(js, "function _adapter(lens)") && !strings.Contains(js, "function _adapter(lens) {") {
		t.Error("D32b-impl-1: graph-shell.js must define _adapter(lens) for lens-aware adapter lookup")
	}
	if !strings.Contains(js, "'Adapter'") && !strings.Contains(js, `"Adapter"`) {
		t.Error("D32b-impl-1: graph-shell.js must look up adapters via lens + 'Adapter' namespace key")
	}
}

// TestExplorer_D32bImpl1_DiagnosticsAndPostureDeferred (renamed in
// D32b-impl-2: TestExplorer_D32bImpl2_DiagnosticFiltersDeferred) was
// the original D32b-impl-1 negative pin for the diagnostics and
// surface_posture panels. D32b-impl-2 implements the read-only panels
// but explicitly defers per-severity / per-kind / per-surface FILTER
// UI; this test now pins only the filter-affordance absence.
//
// The panel modules (authority-diagnostics-panel.js,
// authority-surface-posture-panel.js) and their <script> tags ARE
// expected to appear in the served HTML; positive existence pins for
// them live in TestExplorer_D32bImpl2_* tests below.
func TestExplorer_D32bImpl1_DiagnosticsAndPostureDeferred(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// Filter UI affordances remain D32b-impl-3+ scope.
	for _, banned := range []string{
		"authority-diagnostics-filter",
		"authority-posture-filter",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D32b-impl-2: %q is post-D32b-impl-2 scope and must not yet appear", banned)
		}
	}
	// The view source must not yet contain filter-apply helpers.
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, banned := range []string{
		"applyDiagnosticFilter",
		"applyPostureFilter",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32b-impl-2: %q is post-D32b-impl-2 scope and must not yet appear in the view", banned)
		}
	}
}

// TestExplorer_D32bImpl1_NoLegacyGraphRoutes carries forward the
// negative pin from prior tranches: no /v1/authority-graph,
// /v1/businessservices/{id}/governance-map, or knowledge-graph
// route is ever referenced. Authority lens UI must use the canonical
// /v1/graphs/authority path through the API client.
func TestExplorer_D32bImpl1_NoLegacyGraphRoutes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// Each pattern below is scoped enough that a legitimate CSS class
	// name (e.g. `.governance-map-workbench`) does not trip the pin.
	// The `/v1/businessservices/{id}/governance-map` legacy URL is
	// intentionally NOT pinned — it appears only in a removal-note
	// comment and is otherwise gone from active code (D32a-impl-6
	// already pins the absence of any `fetch('/v1/businessservices'`).
	for _, gone := range []string{
		"/v1/authority-graph",
		"/v1/graphs/knowledge",
		"/v1/knowledge-graph",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32b-impl-1: index.html must not reference legacy/unsupported route %q", gone)
		}
	}
}

// TestExplorer_D32bImpl1_ContextLensRemainsWired pins that the Context
// Graph lens is still wired through context-graph-view.js and that
// the Context Graph URL is still declared by the API client. This is
// a regression guard so a future Authority change does not silently
// break the Context path.
func TestExplorer_D32bImpl1_ContextLensRemainsWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	apiJS := getExplorerAsset(t, srv, "/explorer/assets/js/core/api-client.js")
	if !strings.Contains(apiJS, "/v1/graphs/context") {
		t.Error("D32b-impl-1: API client must still declare /v1/graphs/context for the Context lens")
	}
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	// D37p-clean-1 retired the dead `renderer.register('context', lensImpl)`
	// dispatcher path. The live legacy Context lens entry point is the
	// `contextView` export; the shell calls it via
	// `ExplorerGraph.shell.refresh({lens:'context', …})` →
	// `refreshGovernanceMap` → `renderContextGraph`.
	if !strings.Contains(viewJS, "MIDASExplorerGraph.contextView") {
		t.Error("D32b-impl-1: context-graph-view.js must still expose window.MIDASExplorerGraph.contextView")
	}
	// The Context Graph refresh inside the inline IIFE still dispatches
	// through ExplorerGraph.shell.refresh({lens:'context',...}).
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "ExplorerGraph.shell.refresh({ lens: 'context'") {
		t.Error("D32b-impl-1: Context refresh must continue to dispatch through ExplorerGraph.shell.refresh({lens:'context',...})")
	}
}

// ---------------------------------------------------------------------------
// D32b-impl-2 — Authority Workbench Navigation, Entry Point,
// Diagnostics, and Surface Posture.
//
// Tests in this section pin the end-state of D32b-impl-2:
//
//   • The Service Workbench mode toolbar (Form View | Context Graph |
//     Authority Graph) is the primary operator entry point in
//     .services-map-view-header. The original canvas-level icon-only
//     Form/Graph toggle is gone.
//   • Each mode button is wired:
//       - Form View      → MIDASExplorerServices.showRecord(...)
//       - Context Graph  → shell.setActiveLens('context') + showMap
//       - Authority Graph → shell.setActiveLens('authority') +
//                            authorityView.refresh + showMap
//   • ensureFocusModeEnabledOnGraphEntry() defaults Focus Mode to ON
//     when the operator enters a graph mode from outside.
//   • Clicking the left-nav Services item returns to the Services
//     catalogue from any Workbench sub-view.
//   • Two new panel modules exist:
//       authority-diagnostics-panel.js  (diagnostic summary +
//                                        diagnostics list)
//       authority-surface-posture-panel.js (surface_posture rows)
//   • Both panel modules read backend-supplied rollups verbatim and
//     contain no fetch / no governance recomputation.
//   • A namespaced container holds the three panel slots; visibility
//     is driven by the store subscription and the mode-toolbar
//     handlers.
//   • Authority panels are visible only in Authority Graph mode.
//   • Authority Graph URL is still reached only via the API client.
// ---------------------------------------------------------------------------

// TestExplorer_D32bImpl2_WorkbenchModeToolbar pins the Service Workbench
// mode toolbar (D32b-impl-2 placement + D32b-impl-3 icon-only style).
// The toolbar is now icon-only: SVG icons replace the visible text
// labels, and accessibility is preserved via aria-label + title.
func TestExplorer_D32bImpl2_WorkbenchModeToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Class attribute is now composite — service-workbench-modes +
	// graph-lens-switcher + service-workbench-icon-modes — so the pin
	// D32d-impl-4 — The toolbar was migrated to the header's
	// .graph-view-menu (with a new Knowledge Graph placeholder). The
	// menu reuses the existing .service-workbench-mode + data-workbench
	// -mode attributes so the setWorkbenchMode dispatcher continues to
	// pick the buttons up.
	for _, want := range []string{
		`graph-view-menu`,
		`service-workbench-mode`,
		`service-workbench-icon-mode`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32d-impl-4: header graph view menu must include class component %q", want)
		}
	}
	if !strings.Contains(body, `aria-label="Graph view"`) {
		t.Error("D32d-impl-4: header .graph-view-menu must declare aria-label=\"Graph view\"")
	}
	// Each mode carries data-workbench-mode + an accessible label
	// (aria-label + title). Visible English text labels are NOT
	// required — D32b-impl-3 dropped them in favour of inline SVG
	// icons; D32d-impl-4 keeps the icon-only contract in the header
	// and adds a Knowledge Graph placeholder.
	for _, want := range []string{
		`data-workbench-mode="form"`,
		`data-workbench-mode="context"`,
		`data-workbench-mode="authority"`,
		`data-workbench-mode="knowledge"`,
		`aria-label="Form / Records view"`,
		`aria-label="Context Graph"`,
		`aria-label="Authority Graph"`,
		`aria-label="Knowledge Graph (coming soon)"`,
		`title="Form / Records view"`,
		`title="Context Graph"`,
		`title="Authority Graph"`,
		`title="Knowledge Graph — coming soon"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32d-impl-4: header graph view menu must include %q", want)
		}
	}
	// Context Graph is active by default.
	if !strings.Contains(body, `data-workbench-mode="context" data-lens="context"`) {
		t.Error("D32b-impl-2: Context Graph button must carry data-workbench-mode=\"context\" data-lens=\"context\"")
	}
	if !strings.Contains(body, `data-workbench-mode="authority" data-lens="authority"`) {
		t.Error("D32b-impl-2: Authority Graph button must carry data-workbench-mode=\"authority\" data-lens=\"authority\"")
	}
	// "Open Workbench" replaces the previous "Open Context Graph"
	// primary CTA on the record header.
	if !strings.Contains(body, `>Open Workbench<`) {
		t.Error("D32b-impl-2: record-header CTA must read \"Open Workbench\"")
	}
	if strings.Contains(body, `>Open Context Graph<`) {
		t.Error("D32b-impl-2: legacy \"Open Context Graph\" CTA must be removed")
	}
}

// TestExplorer_D32bImpl2_WorkbenchModeHandlers pins that each mode
// button's click handler reaches the right target.
func TestExplorer_D32bImpl2_WorkbenchModeHandlers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, "function setWorkbenchMode(") {
		t.Fatal("D32b-impl-2: setWorkbenchMode(mode) dispatch must exist")
	}
	// Each branch invokes the right downstream API.
	if !strings.Contains(body, "MIDASExplorerServices.showRecord(serviceId)") {
		t.Error("D32b-impl-2: Form View branch must call MIDASExplorerServices.showRecord(serviceId)")
	}
	if !strings.Contains(body, "ExplorerGraph.shell.setActiveLens('context')") {
		t.Error("D32b-impl-2: Context Graph branch must call shell.setActiveLens('context')")
	}
	if !strings.Contains(body, "ExplorerGraph.shell.setActiveLens('authority')") {
		t.Error("D32b-impl-2: Authority Graph branch must call shell.setActiveLens('authority')")
	}
	if !strings.Contains(body, "ExplorerGraph.authorityView.refresh({ rootId: serviceId })") {
		t.Error("D32b-impl-2: Authority Graph branch must call authorityView.refresh({rootId: serviceId})")
	}
	if !strings.Contains(body, "MIDASExplorerServices.showMap(serviceId)") {
		t.Error("D32b-impl-2: graph branches must call MIDASExplorerServices.showMap(serviceId) to enter map sub-view")
	}
}

// TestExplorer_D32bImpl2_FocusModeDefaultsOnEntry pins the focus-mode
// default-ON behaviour when entering a graph workbench.
func TestExplorer_D32bImpl2_FocusModeDefaultsOnEntry(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, "function ensureFocusModeEnabledOnGraphEntry()") {
		t.Fatal("D32b-impl-2: ensureFocusModeEnabledOnGraphEntry() helper must exist in the inline IIFE")
	}
	// The helper must NOT write localStorage so a subsequent entry
	// re-defaults to ON.
	helperStart := strings.Index(body, "function ensureFocusModeEnabledOnGraphEntry()")
	helperEnd := strings.Index(body[helperStart:], "\n  }\n")
	if helperEnd < 0 {
		t.Fatal("D32b-impl-2: ensureFocusModeEnabledOnGraphEntry body has no closing brace")
	}
	helperBody := body[helperStart : helperStart+helperEnd]
	if strings.Contains(helperBody, "localStorage.setItem") {
		t.Error("D32b-impl-2: ensureFocusModeEnabledOnGraphEntry must not write localStorage (user toggle remains authoritative)")
	}
	if !strings.Contains(helperBody, "gmapFocusMode = true") {
		t.Error("D32b-impl-2: ensureFocusModeEnabledOnGraphEntry must set gmapFocusMode = true")
	}
	if !strings.Contains(helperBody, "applyGmapFocusMode()") {
		t.Error("D32b-impl-2: ensureFocusModeEnabledOnGraphEntry must call applyGmapFocusMode() to sync DOM")
	}
	// Called from setGmapMode('map') so any entry into the graph
	// workbench (record-CTA, deep-link, lens switch, mode toolbar)
	// passes through this helper.
	if !strings.Contains(body, "if (mode === 'map' && typeof ensureFocusModeEnabledOnGraphEntry === 'function')") {
		t.Error("D32b-impl-2: setGmapMode hook must invoke ensureFocusModeEnabledOnGraphEntry() when mode === 'map'")
	}
}

// TestExplorer_D32bImpl2_ServicesNavReturnsToCatalogue pins the
// left-nav Services click → catalogue return semantics.
func TestExplorer_D32bImpl2_ServicesNavReturnsToCatalogue(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// The nav-item click handler must call MIDASExplorerServices
	// .showCatalogue() when target === 'services'.
	if !strings.Contains(body, "if (target === 'services' &&") {
		t.Fatal("D32b-impl-2: Services nav handler must dispatch to MIDASExplorerServices.showCatalogue() when target === 'services'")
	}
	if !strings.Contains(body, "MIDASExplorerServices.showCatalogue();") {
		t.Error("D32b-impl-2: Services nav handler must invoke MIDASExplorerServices.showCatalogue()")
	}
}

// TestExplorer_D32bImpl2_AuthorityPanelContainer was a D32b-impl-2
// contract test pinning the floating .authority-graph-panels overlay
// markup. D32b-impl-3 removed the overlay entirely — Authority
// diagnostics + surface posture now render inside the unified
// right-side graph drawer (#gmap-details) via the Authority drawer
// provider. The test body now pins the inverse: the overlay must
// remain removed, and the Authority drawer provider must inject the
// expected data-* containers into the drawer panel mount on tab
// activation.
func TestExplorer_D32bImpl2_AuthorityPanelContainer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Negative pin — the old overlay markup must be gone.
	for _, gone := range []string{
		`class="authority-graph-panels"`,
		`data-authority-graph-panels`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32b-impl-3: .authority-graph-panels overlay markup must remain removed: %q", gone)
		}
	}
	// D37av2-prereq-authority-rail-decommission-content-impl flipped
	// these from positive to negative pins. The Authority drawer's
	// Diagnostics + Posture & Help tabs were decommissioned; their
	// render functions were removed from authority-graph-view.js. The
	// strategic homes are:
	//   • Workbench Overview (diagnostic summary counts)
	//   • Workbench Evidence (projection-wide diagnostics)
	//   • Workbench Posture (Surface Posture clickable list — moved by
	//     this tranche)
	//   • OSS Help / authority-graph (legend / glyph reference)
	// data-authority-surface-posture now lives in the Workbench Posture
	// tab markup (authority-graph-workbench.js), not view.js. The
	// Workbench Posture tab is the SOLE authoritative Surface Posture
	// home.
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, banned := range []string{
		`data-authority-diagnostic-summary`,
		`data-authority-diagnostics`,
		`data-authority-surface-posture`,
		`data-authority-summary-mount`,
		`data-authority-layer-chips`,
		`data-authority-legend`,
		`_authorityRenderDiagnosticsIntoDrawer`,
		`_authorityRenderPostureAndHelpIntoDrawer`,
	} {
		if strings.Contains(viewJS, banned) {
			t.Errorf("D37av2-prereq: authority-graph-view.js must not contain decommissioned right-rail mount/render token %q (Surface Posture moved to Workbench Posture tab; Diagnostics duplicated by Workbench Overview + Evidence; summary/layers/legend retired as live runtime UI)", banned)
		}
	}
}

// TestExplorer_D32bImpl2_AuthorityPanelVisibility pins that the
// inline IIFE flips panel container visibility on lens transitions
// (mode-toolbar click + store subscription).
func TestExplorer_D32bImpl2_AuthorityPanelVisibility(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// D32b-impl-3 — _showAuthorityPanels is preserved as a thin
	// drawer.setActiveLens adapter, retaining the same name so the
	// small number of remaining call-sites continue to work.
	if !strings.Contains(body, "function _showAuthorityPanels(show)") {
		t.Fatal("D32b-impl-3: _showAuthorityPanels(show) helper must remain (now a drawer.setActiveLens adapter)")
	}
	helperIdx := strings.Index(body, "function _showAuthorityPanels(show)")
	helperEnd := strings.Index(body[helperIdx:], "\n  }\n")
	if helperEnd < 0 {
		t.Fatal("D32b-impl-3: _showAuthorityPanels body has no closing brace")
	}
	helperBody := body[helperIdx : helperIdx+helperEnd]
	if !strings.Contains(helperBody, "drawer.setActiveLens") {
		t.Error("D32b-impl-3: _showAuthorityPanels must dispatch through drawer.setActiveLens (overlay removed)")
	}
	// Mode-toolbar branches set the active lens via the helper.
	if !strings.Contains(body, "_showAuthorityPanels(true);") {
		t.Error("D32b-impl-3: Authority Graph mode branch must call _showAuthorityPanels(true)")
	}
	if !strings.Contains(body, "_showAuthorityPanels(false);") {
		t.Error("D32b-impl-3: Form / Context branches must call _showAuthorityPanels(false)")
	}
	// Store subscription keeps the drawer in sync with selectedGraphLens.
	if !strings.Contains(body, "MIDASExplorerStore.subscribe") {
		t.Error("D32b-impl-3: index.html must subscribe to MIDASExplorerStore to keep the drawer in sync with selectedGraphLens")
	}
	if !strings.Contains(body, "drawer.setActiveLens(state.selectedGraphLens)") {
		t.Error("D32b-impl-3: store subscription must dispatch through drawer.setActiveLens(state.selectedGraphLens)")
	}
}

// TestExplorer_D32bImpl2_DiagnosticsPanelModule pins the diagnostics-
// panel module's contract: a window-attached namespace with render()
// + clear(); reads diagnostic_summary + diagnostics from the
// projection; no direct fetch; no governance recomputation.
func TestExplorer_D32bImpl2_DiagnosticsPanelModule(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/assets/js/graph/authority/authority-diagnostics-panel.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("D32b-impl-2: authority-diagnostics-panel.js want 200, got %d", rec.Code)
	}
	js := rec.Body.String()
	if !strings.Contains(js, "window.MIDASExplorerGraph.authorityDiagnosticsPanel = {") {
		t.Error("D32b-impl-2: module must publish window.MIDASExplorerGraph.authorityDiagnosticsPanel = { ... }")
	}
	for _, want := range []string{"render:", "clear:"} {
		if !strings.Contains(js, want) {
			t.Errorf("D32b-impl-2: authorityDiagnosticsPanel must expose %q", want)
		}
	}
	// Reads the backend rollup; does not recompute.
	for _, want := range []string{
		"data-authority-diagnostic-summary",
		"data-authority-diagnostics",
		"projection.diagnostic_summary",
		"projection.diagnostics",
		"highest_severity",
		"node_refs",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32b-impl-2: authority-diagnostics-panel.js must reference backend rollup token %q", want)
		}
	}
	// Pure renderer — no fetch, no client-side recomputation.
	if strings.Contains(js, "fetch(") {
		t.Error("D32b-impl-2: authority-diagnostics-panel.js must not call fetch(...)")
	}
	if strings.Contains(js, "ExplorerAPI") {
		t.Error("D32b-impl-2: authority-diagnostics-panel.js must not invoke ExplorerAPI directly")
	}
}

// TestExplorer_D32bImpl2_SurfacePosturePanelModule pins the surface-
// posture-panel module's contract.
func TestExplorer_D32bImpl2_SurfacePosturePanelModule(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/assets/js/graph/authority/authority-surface-posture-panel.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("D32b-impl-2: authority-surface-posture-panel.js want 200, got %d", rec.Code)
	}
	js := rec.Body.String()
	if !strings.Contains(js, "window.MIDASExplorerGraph.authoritySurfacePosturePanel = {") {
		t.Error("D32b-impl-2: module must publish window.MIDASExplorerGraph.authoritySurfacePosturePanel = { ... }")
	}
	for _, want := range []string{"render:", "clear:"} {
		if !strings.Contains(js, want) {
			t.Errorf("D32b-impl-2: authoritySurfacePosturePanel must expose %q", want)
		}
	}
	// Reads backend-supplied surface_posture rows verbatim.
	for _, want := range []string{
		"data-authority-surface-posture",
		"surface_posture",
		"authority_status",
		"profile_status",
		"grant_status",
		"agent_status",
		"fail_mode_policy_status",
		"escalation_status",
		"complete_paths",
		"incomplete_paths",
		"highest_severity",
		"diagnostic_kinds",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32b-impl-2: authority-surface-posture-panel.js must reference backend posture token %q", want)
		}
	}
	// Pure renderer — no fetch, no governance derivation.
	if strings.Contains(js, "fetch(") {
		t.Error("D32b-impl-2: authority-surface-posture-panel.js must not call fetch(...)")
	}
	if strings.Contains(js, "ExplorerAPI") {
		t.Error("D32b-impl-2: authority-surface-posture-panel.js must not invoke ExplorerAPI directly")
	}
	// Row click attempts to select the corresponding surface node.
	if !strings.Contains(js, "decision_surface:") {
		t.Error("D32b-impl-2: surface posture row click must reference the decision_surface:<id> node key")
	}
}

// TestExplorer_D32bImpl2_AuthorityViewRendersPanels pins that the
// view dispatches a render call to both panel modules after a
// successful Authority Graph paint, and clears them on empty/error.
func TestExplorer_D32bImpl2_AuthorityViewRendersPanels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, "function _renderAuthorityPanels(payload)") {
		t.Error("D32b-impl-2: authority-graph-view.js must define _renderAuthorityPanels(payload)")
	}
	if !strings.Contains(js, "authorityDiagnosticsPanel") {
		t.Error("D32b-impl-2: authority-graph-view.js must dispatch to authorityDiagnosticsPanel")
	}
	if !strings.Contains(js, "authoritySurfacePosturePanel") {
		t.Error("D32b-impl-2: authority-graph-view.js must dispatch to authoritySurfacePosturePanel")
	}
	if !strings.Contains(js, "function _clearAuthorityPanels()") {
		t.Error("D32b-impl-2: authority-graph-view.js must define _clearAuthorityPanels() for empty/error states")
	}
}

// TestExplorer_D32bImpl2_AuthorityCSSPanels pins that the per-panel
// CSS classes are declared in authority-graph.css. D32b-impl-3
// removed the .authority-graph-panels overlay rules (the overlay
// itself is gone) and added .authority-drawer-section + the icon-
// mode toolbar rules; the per-panel content classes that the panel
// modules emit remain unchanged.
func TestExplorer_D32bImpl2_AuthorityCSSPanels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, want := range []string{
		".authority-diagnostics-summary",
		".authority-diagnostics-list",
		".authority-posture-rows",
		".authority-posture-row",
		".service-workbench-modes",
		".authority-diagnostics-severity-critical",
		// D32b-impl-3 additions.
		".authority-drawer-section",
		".service-workbench-icon-modes",
		".service-workbench-icon-mode",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32b-impl-3: authority-graph.css must declare %q", want)
		}
	}
	// Negative pin — the removed overlay rule must remain gone.
	if strings.Contains(css, ".authority-graph-panels[hidden]") {
		t.Error("D32b-impl-3: the .authority-graph-panels overlay rule must remain removed (overlay deleted)")
	}
}

// ---------------------------------------------------------------------------
// D32b-impl-2a — Restore Service Workbench Mode Toolbar.
//
// D32b-impl-2 declared the toolbar inside .services-map-view-header,
// but that header is hidden by `body.gmap-focus-mode .services-map-
// view-header { display: none; }` in shell.css. Because D32b-impl-2
// also defaulted focus mode to ON when entering the graph workbench,
// the toolbar was invisible to operators in practice.
//
// D32b-impl-2a moves the toolbar into the always-visible
// .governance-map-toolbar-left so it remains reachable in focus mode.
// The tests below pin:
//
//   • The toolbar lives inside .governance-map-toolbar (visible).
//   • The toolbar reads BEFORE .governance-map-toolbar-centre so the
//     operator-facing reading order is
//        Service · <name> → Form View | Context Graph | Authority
//        Graph → Search → Layers.
//   • Focus-mode CSS does not hide .governance-map-toolbar.
//   • Authority Graph rendering ends with a scheduleFitToView call so
//     the operator opens to a centred / framed graph rather than a
//     top-aligned canvas (the Context lens already does this; D32b-
//     impl-2a adds it to the Authority lens).
//   • The pre-existing scheduleGmapFitToView / fitGmapToBounds /
//     camera.scheduleFitToView helpers are the only centring path —
//     no parallel centring implementation is introduced.
// ---------------------------------------------------------------------------

// TestExplorer_D32bImpl2a_ToolbarLivesInsideVisibleWorkbenchToolbar
// pins that the graph view toolbar sits in a visible, focus-mode-
// resilient location. D32b-impl-2a placed it inside .governance-map-
// toolbar-left; D32d-impl-4 migrated it again to the Explorer header
// (.shell-header-right > .graph-view-menu) — but the shell header is
// hidden by `body.gmap-focus-mode .shell-header { display: none; }`
// whenever focus mode is active (the default on graph entry), so the
// menu was unreachable in the active workbench. D32b-impl-4 relocates
// the menu back inside the workbench toolbar's right zone
// (.governance-map-toolbar-right) which is focus-mode-resilient.
func TestExplorer_D32bImpl2a_ToolbarLivesInsideVisibleWorkbenchToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	toolbarRightIdx := strings.Index(body, `class="governance-map-toolbar-right"`)
	if toolbarRightIdx < 0 {
		t.Fatal("D32b-impl-4: .governance-map-toolbar-right missing")
	}
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	if menuIdx < 0 {
		t.Fatal("D32b-impl-4: .graph-view-menu missing")
	}
	if menuIdx < toolbarRightIdx {
		t.Errorf("D32b-impl-4: .graph-view-menu must live inside .governance-map-toolbar-right; menuIdx=%d toolbarRightIdx=%d", menuIdx, toolbarRightIdx)
	}

	// All four modes must be present (Knowledge Graph is a
	// placeholder, but its data-workbench-mode + aria-label still
	// ship in markup). D32b-impl-3 made the toolbar icon-only;
	// accessibility relies on aria-label + title.
	for _, want := range []string{
		`data-workbench-mode="form"`,
		`data-workbench-mode="context"`,
		`data-workbench-mode="authority"`,
		`data-workbench-mode="knowledge"`,
		`aria-label="Form / Records view"`,
		`aria-label="Context Graph"`,
		`aria-label="Authority Graph"`,
		`aria-label="Knowledge Graph (coming soon)"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32d-impl-4: header graph view menu must include %q", want)
		}
	}

	// Negative pin — the buttons must NOT live inside .governance-map-
	// toolbar-left any more (D32d-impl-4 migration). The in-workbench
	// toolbar's left group now hosts only the Back button + current-
	// root span.
	// Negative pin — the menu must NOT live inside
	// .governance-map-toolbar-left any more (D32b-impl-3 migration).
	leftIdx := strings.Index(body, `class="governance-map-toolbar-left"`)
	if leftIdx >= 0 {
		// Bound by the next opener of .governance-map-toolbar-centre.
		centreIdx := strings.Index(body, `class="governance-map-toolbar-centre"`)
		if centreIdx > leftIdx {
			leftSlice := body[leftIdx:centreIdx]
			if strings.Contains(leftSlice, `data-workbench-mode=`) {
				t.Error("D32b-impl-3: mode buttons must not live inside .governance-map-toolbar-left (migrated to the right group)")
			}
		}
	}
	// Negative pin — the menu must NOT live inside .services-map-view-
	// header (focus-mode CSS hides that header).
	headerIdx := strings.Index(body, `class="services-map-view-header"`)
	headerEnd := strings.Index(body[headerIdx:], `</header>`)
	if headerIdx >= 0 && headerEnd > 0 {
		headerSlice := body[headerIdx : headerIdx+headerEnd]
		if strings.Contains(headerSlice, `data-workbench-mode=`) {
			t.Error("D32b-impl-2a: mode buttons must not live inside .services-map-view-header (focus-mode CSS hides that header)")
		}
	}
	// D32b-impl-4 — Negative pin: the menu must NOT live inside the
	// shell header either, because `body.gmap-focus-mode .shell-header
	// { display: none; }` hides the entire shell header whenever focus
	// mode is on (the default on graph entry).
	shellHeaderIdx := strings.Index(body, `<header class="shell-header"`)
	shellHeaderEnd := strings.Index(body[shellHeaderIdx:], `</header>`)
	if shellHeaderIdx >= 0 && shellHeaderEnd > 0 {
		shellHeaderSlice := body[shellHeaderIdx : shellHeaderIdx+shellHeaderEnd]
		if strings.Contains(shellHeaderSlice, `data-workbench-mode=`) {
			t.Error("D32b-impl-4: mode buttons must not live inside <header class=\"shell-header\"> (focus-mode CSS hides the shell header)")
		}
	}
}

// TestExplorer_D32bImpl2a_ToolbarReadingOrder pins reading order
// across two surfaces:
//
// In-workbench toolbar reading order (D32b-impl-4):
//
//	Back → Service · <name> → Search graph → Layers → Graph View menu
//
// Graph View menu reading order:
//
//	.governance-map-toolbar-right opener → .graph-view-menu opener →
//	Form / Records → Context Graph → Authority Graph →
//	Knowledge Graph placeholder
//
// The menu is the LAST landmark in the workbench toolbar (right zone)
// — Search and Layers sit in the centre group, Back + current-root in
// the left group, and the mode menu rounds out the right edge.
func TestExplorer_D32bImpl2a_ToolbarReadingOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// In-workbench reading order: back → current-root → search →
	// layers → mode menu.
	backIdx := strings.Index(body, `id="gmap-back-button"`)
	currentRootIdx := strings.Index(body, `id="gmap-current-root"`)
	searchIdx := strings.Index(body, `id="gmap-search-input"`)
	layersIdx := strings.Index(body, `id="gmap-layers-button"`)
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	for _, p := range []struct {
		name string
		idx  int
	}{
		{"gmap-back-button", backIdx},
		{"gmap-current-root", currentRootIdx},
		{"gmap-search-input", searchIdx},
		{"gmap-layers-button", layersIdx},
		{"graph-view-menu", menuIdx},
	} {
		if p.idx < 0 {
			t.Fatalf("D32b-impl-4: in-workbench toolbar landmark %q missing", p.name)
		}
	}
	if !(backIdx < currentRootIdx &&
		currentRootIdx < searchIdx &&
		searchIdx < layersIdx &&
		layersIdx < menuIdx) {
		t.Errorf("D32b-impl-4: in-workbench reading-order broken (back=%d root=%d search=%d layers=%d menu=%d)",
			backIdx, currentRootIdx, searchIdx, layersIdx, menuIdx)
	}

	// Graph View menu reading order: container → form → context →
	// authority → knowledge.
	toolbarRightIdx := strings.Index(body, `class="governance-map-toolbar-right"`)
	formIdx := strings.Index(body, `data-workbench-mode="form"`)
	contextIdx := strings.Index(body, `data-workbench-mode="context"`)
	authorityIdx := strings.Index(body, `data-workbench-mode="authority"`)
	knowledgeIdx := strings.Index(body, `data-workbench-mode="knowledge"`)
	for _, p := range []struct {
		name string
		idx  int
	}{
		{"governance-map-toolbar-right", toolbarRightIdx},
		{"graph-view-menu", menuIdx},
		{"data-workbench-mode=form", formIdx},
		{"data-workbench-mode=context", contextIdx},
		{"data-workbench-mode=authority", authorityIdx},
		{"data-workbench-mode=knowledge", knowledgeIdx},
	} {
		if p.idx < 0 {
			t.Fatalf("D32b-impl-4: graph view menu landmark %q missing", p.name)
		}
	}
	if !(toolbarRightIdx < menuIdx &&
		menuIdx < formIdx &&
		formIdx < contextIdx &&
		contextIdx < authorityIdx &&
		authorityIdx < knowledgeIdx) {
		t.Errorf("D32b-impl-4: graph view menu reading order broken (toolbarRight=%d menu=%d form=%d context=%d authority=%d knowledge=%d)",
			toolbarRightIdx, menuIdx, formIdx, contextIdx, authorityIdx, knowledgeIdx)
	}
}

// TestExplorer_D32bImpl2a_ToolbarNotHiddenInFocusMode pins that the
// focus-mode hide rule covers only chrome that legitimately collapses
// when the operator wants maximum canvas. The .governance-map-toolbar
// is NOT in that list — it carries the controls (Search / Layers /
// the new Workbench mode toolbar) that operators need at all times.
func TestExplorer_D32bImpl2a_ToolbarNotHiddenInFocusMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	hideRuleIdx := strings.Index(css, "body.gmap-focus-mode .shell-header,")
	if hideRuleIdx < 0 {
		t.Fatal("D32b-impl-2a: focus-mode hide rule must remain present (it hides shell chrome on graph focus)")
	}
	hideEnd := strings.Index(css[hideRuleIdx:], "}")
	if hideEnd <= 0 {
		t.Fatal("D32b-impl-2a: focus-mode hide rule has no closing brace")
	}
	hideRule := css[hideRuleIdx : hideRuleIdx+hideEnd]
	// The governance-map-toolbar must not be a selector inside the
	// hide rule.
	for _, banned := range []string{
		".governance-map-toolbar",
		".governance-map-toolbar-left",
		".governance-map-toolbar-centre",
		".service-workbench-modes",
		".graph-lens-switcher",
	} {
		if strings.Contains(hideRule, banned) {
			t.Errorf("D32b-impl-2a: focus-mode hide rule must not collapse %q (toolbar must remain visible)", banned)
		}
	}
}

// TestExplorer_D32bImpl2a_AuthorityViewSchedulesFitToView pins that
// the Authority Graph view schedules a fit-to-view via the shared
// graph-camera helper after a successful render. The Context view
// already does this through ctx.scheduleFitToView; the Authority
// view dispatches to MIDASExplorerGraph.camera.scheduleFitToView
// directly because its render path does not receive a ctx bundle.
func TestExplorer_D32bImpl2a_AuthorityViewSchedulesFitToView(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(js, "MIDASExplorerGraph.camera") {
		t.Error("D32b-impl-2a: authority-graph-view.js must reach the shared graph-camera module for fit-to-view")
	}
	if !strings.Contains(js, "scheduleFitToView()") {
		t.Error("D32b-impl-2a: authority-graph-view.js must call camera.scheduleFitToView() after render")
	}
	// Negative pin — no parallel centring implementation.
	for _, banned := range []string{
		"function authorityScheduleFit",
		"function _authorityFitToBounds",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32b-impl-2a: must not introduce a parallel centring function: %q", banned)
		}
	}
}

// TestExplorer_D32bImpl2a_ContextViewSchedulesFitToView is a
// regression guard: the Context lens has scheduled fit-to-view since
// pre-D32 tranches (via ctx.scheduleFitToView). D32b-impl-2a pins
// this so a future refactor does not silently remove it.
func TestExplorer_D32bImpl2a_ContextViewSchedulesFitToView(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")
	if !strings.Contains(js, "ctx.scheduleFitToView") {
		t.Error("D32b-impl-2a: context-graph-view.js must continue to call ctx.scheduleFitToView() after render so the operator opens to a framed graph")
	}
	// The shell exposes a scheduleFitToView pass-through that maps to
	// the camera module's scheduleFitToView. The Context render ctx
	// bundle (built in index.html) wires this; pin both ends.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "scheduleGmapFitToView") {
		t.Error("D32b-impl-2a: scheduleGmapFitToView() shim must remain in the inline IIFE for the Context render context")
	}
}

// TestExplorer_D32bImpl2a_ServicesMapViewHeaderRemainsRetainedAsShell
// pins that .services-map-view-header still exists as a structural
// landmark (the Back button + record-name span) but does NOT contain
// the Workbench mode toolbar. Existing tests that anchor on the
// header marker continue to work.
func TestExplorer_D32bImpl2a_ServicesMapViewHeaderRemainsRetainedAsShell(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `class="services-map-view-header"`) {
		t.Fatal("D32b-impl-2a: .services-map-view-header must remain as a structural landmark")
	}
	// The header now hosts only the Back button + record-name span;
	// the mode toolbar is gone from this region.
	headerIdx := strings.Index(body, `class="services-map-view-header"`)
	headerEnd := strings.Index(body[headerIdx:], `</header>`)
	headerSlice := body[headerIdx : headerIdx+headerEnd]
	if !strings.Contains(headerSlice, `id="services-map-back-btn"`) {
		t.Error("D32b-impl-2a: .services-map-view-header must retain the Back button (#services-map-back-btn)")
	}
	if strings.Contains(headerSlice, `data-workbench-mode=`) {
		t.Error("D32b-impl-2a: mode toolbar must not live inside .services-map-view-header")
	}
}

// ---------------------------------------------------------------------------
// D32b-impl-3 — Unified Graph Drawer Module and Icon Mode Toolbar.
//
// Tests in this section pin the end-state of D32b-impl-3:
//
//   • A new module assets/js/graph/graph-drawer.js is served and
//     publishes window.MIDASExplorerGraph.drawer with init,
//     registerLens, setActiveLens, setActiveTab, render, open,
//     close, isOpen, clear.
//   • The pre-existing right-side drawer DOM is preserved
//     (#gmap-details, .gmap-right-rail*, three data-rail-tab buttons,
//     three data-rail-panel sections, the close button).
//   • No second drawer / no Authority-specific drawer shell exists.
//   • The previous .authority-graph-panels overlay markup is gone.
//   • Context lens and Authority lens register drawer providers
//     through MIDASExplorerGraph.drawer.registerLens(...).
//   • Authority diagnostics and surface posture render inside the
//     unified drawer (no overlay).
//   • The Workbench mode toolbar is icon-only — SVG icons + aria-
//     label + title — and visually matches the bottom-right canvas
//     control buttons.
//   • setGmapRightRailTab remains as a thin compatibility shim that
//     delegates to drawer.setActiveTab.
//   • API discipline preserved.
// ---------------------------------------------------------------------------

// TestExplorer_D32bImpl3_GraphDrawerModuleServedAndExposesNamespace
// pins that the new drawer module is served and publishes its
// documented public surface.
func TestExplorer_D32bImpl3_GraphDrawerModuleServedAndExposesNamespace(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/assets/js/graph/graph-drawer.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("D32b-impl-3: graph-drawer.js want 200, got %d", rec.Code)
	}
	js := rec.Body.String()
	if !strings.Contains(js, "window.MIDASExplorerGraph.drawer = {") {
		t.Fatal("D32b-impl-3: module must publish window.MIDASExplorerGraph.drawer = { ... }")
	}
	for _, want := range []string{
		"init:",
		"registerLens:",
		"setActiveLens:",
		"getActiveLens:",
		"setActiveTab:",
		"getActiveTab:",
		"render:",
		"open:",
		"close:",
		"isOpen:",
		"clear:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32b-impl-3: graph-drawer.js must expose %q", want)
		}
	}
	// The module must NOT call fetch / ExplorerAPI directly — it is
	// pure DOM coordination.
	if strings.Contains(js, "fetch(") {
		t.Error("D32b-impl-3: graph-drawer.js must not call fetch(...)")
	}
	if strings.Contains(js, "ExplorerAPI") {
		t.Error("D32b-impl-3: graph-drawer.js must not invoke ExplorerAPI directly")
	}
	// Script tag must load before graph-shell.js so the shell + lens
	// views can register against the drawer at load time.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	drawerTag := `<script src="/explorer/assets/js/graph/graph-drawer.js"></script>`
	shellTag := `<script src="/explorer/assets/js/graph/graph-shell.js"></script>`
	drawerIdx := strings.Index(body, drawerTag)
	shellIdx := strings.Index(body, shellTag)
	if drawerIdx < 0 || shellIdx < 0 {
		t.Fatalf("D32b-impl-3: graph-drawer / graph-shell script tags missing (drawer=%d, shell=%d)", drawerIdx, shellIdx)
	}
	if !(drawerIdx < shellIdx) {
		t.Errorf("D32b-impl-3: graph-drawer.js must load BEFORE graph-shell.js (drawer=%d, shell=%d)", drawerIdx, shellIdx)
	}
}

// TestExplorer_D32bImpl3_DrawerDOMPreserved pins that the pre-D32b-
// impl-3 drawer DOM survives. The drawer styling took significant
// effort to tune; the drawer module reuses the markup verbatim.
func TestExplorer_D32bImpl3_DrawerDOMPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`id="gmap-details"`,
		`class="governance-map-details gmap-right-rail"`,
		`class="gmap-right-rail-tabs"`,
		`class="gmap-right-rail-tab is-active"`,
		`data-rail-tab="inspector"`,
		`data-rail-tab="evidence"`,
		`data-rail-tab="config"`,
		`class="gmap-right-rail-header"`,
		`id="gmap-right-rail-title"`,
		`id="gmap-right-rail-close"`,
		`class="gmap-right-rail-close"`,
		`id="gmap-rail-panel-inspector"`,
		`id="gmap-rail-panel-evidence"`,
		`id="gmap-rail-panel-config"`,
		`data-rail-panel="inspector"`,
		`data-rail-panel="evidence"`,
		`data-rail-panel="config"`,
		`id="gmap-inspector-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32b-impl-3: drawer landmark %q must remain (drawer style preservation)", want)
		}
	}
	// Negative pin — no second drawer shell.
	if strings.Count(body, `class="governance-map-details`) != 1 {
		t.Error("D32b-impl-3: there must be exactly one .governance-map-details drawer shell")
	}
	if strings.Count(body, `id="gmap-details"`) != 1 {
		t.Error("D32b-impl-3: there must be exactly one #gmap-details drawer")
	}
}

// TestExplorer_D32bImpl3_AuthorityOverlayRemoved pins the absence of
// the previous .authority-graph-panels overlay markup. Authority
// diagnostics + surface posture now render inside the unified
// drawer; the overlay is gone, not hidden.
func TestExplorer_D32bImpl3_AuthorityOverlayRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, gone := range []string{
		`class="authority-graph-panels"`,
		`data-authority-graph-panels`,
		`class="authority-graph-panel authority-diagnostics-summary"`,
		`class="authority-graph-panel authority-diagnostics-list"`,
		`class="authority-graph-panel authority-surface-posture"`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32b-impl-3: .authority-graph-panels overlay markup must remain removed: %q", gone)
		}
	}
}

// TestExplorer_D32bImpl3_LensProvidersRegistered pins that both
// Context and Authority lenses register drawer providers through the
// unified drawer module. The Context provider lives in the inline
// IIFE (preserves the existing #gmap-details-* scaffold); the
// Authority provider lives in authority-graph-view.js.
func TestExplorer_D32bImpl3_LensProvidersRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Context registration is in the inline IIFE.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "MIDASExplorerGraph.drawer.registerLens('context'") {
		t.Error("D32b-impl-3: index.html must register the Context lens drawer provider (MIDASExplorerGraph.drawer.registerLens('context', …))")
	}
	// Authority registration is in authority-graph-view.js.
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(viewJS, "window.MIDASExplorerGraph.drawer.registerLens('authority'") {
		t.Error("D32b-impl-3: authority-graph-view.js must register the Authority lens drawer provider")
	}
	// Authority labels — only the inspector slot remains registered as
	// of D37av2-prereq-authority-rail-decommission-content-impl. The
	// Diagnostics + Posture & Help slot registrations were dropped
	// after the corrective right-rail retirement assessment confirmed:
	//   • Surface Posture moved to the Workbench Posture tab;
	//   • Diagnostics is duplicated by Workbench Overview + Evidence;
	//   • Summary pills are duplicated by Workbench Overview;
	//   • Layer chips and Graph legend are no longer mounted as live
	//     runtime UI;
	//   • Help framing is owned by the OSS Help module.
	// D33a-spike-2g-impl-5d renamed the inspector tab user-facing label
	// `Inspector` → `Showcase` (slot id stays 'inspector'); accept
	// either label here so the historical pin survives the additive
	// change while the impl-5d test
	// (D33aSpike2gImpl5d_InspectorTabRenamedToShowcase) pins the new
	// value exactly.
	if !strings.Contains(viewJS, "label: 'Inspector'") &&
		!strings.Contains(viewJS, "label: 'Showcase'") {
		t.Error("D32b-impl-3 (post-D37av2-prereq): Authority drawer registration must still declare label 'Inspector' or 'Showcase' for the inspector slot (the only slot Authority registers post-tranche)")
	}
	// D37av2-prereq — negative pins on the decommissioned slot labels
	// and render-function dispatches. The previous positive pins on
	// `label: 'Diagnostics'` / `label: 'Posture & Help'` /
	// `_authorityRenderDiagnosticsIntoDrawer` /
	// `_authorityRenderPostureAndHelpIntoDrawer` were flipped to
	// negative pins after this tranche dropped those entries.
	for _, banned := range []string{
		"label: 'Diagnostics'",
		"label: 'Posture & Help'",
		"_authorityRenderDiagnosticsIntoDrawer",
		"_authorityRenderPostureAndHelpIntoDrawer",
	} {
		if strings.Contains(viewJS, banned) {
			t.Errorf("D37av2-prereq: Authority drawer registration must not declare/dispatch %q — slot decommissioned, content moved to Workbench/OSS Help", banned)
		}
	}
	// Authority Inspector tab renderer remains a no-op (the existing
	// #gmap-details-* DOM is populated by authorityInspector on
	// node selection).
	if !strings.Contains(viewJS, "void ctx; void mount;") {
		t.Error("D32b-impl-3: Authority Inspector tab renderer is intentionally a no-op (existing DOM scaffold)")
	}
}

// TestExplorer_D32bImpl3_DrawerLensSync pins that the store
// subscription drives drawer.setActiveLens whenever selectedGraphLens
// changes, so the drawer's labels + content always reflect the
// active lens.
func TestExplorer_D32bImpl3_DrawerLensSync(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "MIDASExplorerStore.subscribe") {
		t.Error("D32b-impl-3: index.html must subscribe to MIDASExplorerStore for drawer/lens sync")
	}
	if !strings.Contains(body, "drawer.setActiveLens(state.selectedGraphLens)") {
		t.Error("D32b-impl-3: store subscription must dispatch through drawer.setActiveLens(state.selectedGraphLens)")
	}
	// setWorkbenchMode also explicitly toggles the lens via the
	// _showAuthorityPanels adapter (now a drawer.setActiveLens
	// wrapper).
	if !strings.Contains(body, "_showAuthorityPanels(true);") || !strings.Contains(body, "_showAuthorityPanels(false);") {
		t.Error("D32b-impl-3: setWorkbenchMode must continue to call _showAuthorityPanels(true|false) for drawer lens sync")
	}
}

// TestExplorer_D32bImpl3_TabActivationThroughDrawer pins that
// setGmapRightRailTab is now a thin compatibility shim delegating to
// drawer.setActiveTab.
func TestExplorer_D32bImpl3_TabActivationThroughDrawer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// Shim function must still exist (other call-sites use it by name).
	if !strings.Contains(body, "function setGmapRightRailTab(tab)") {
		t.Fatal("D32b-impl-3: setGmapRightRailTab shim must remain for back-compat with existing call-sites")
	}
	// And it must delegate to the drawer module.
	if !strings.Contains(body, "window.MIDASExplorerGraph.drawer.setActiveTab(tab)") {
		t.Error("D32b-impl-3: setGmapRightRailTab must delegate to MIDASExplorerGraph.drawer.setActiveTab")
	}
	// drawer.init is invoked once at boot to bind click handlers on
	// the drawer tab buttons.
	if !strings.Contains(body, "MIDASExplorerGraph.drawer.init") {
		t.Error("D32b-impl-3: index.html must invoke MIDASExplorerGraph.drawer.init() at boot")
	}
}

// TestExplorer_D32bImpl3_IconOnlyModeToolbar pins the icon-only
// Workbench mode toolbar. SVG icons replace the previous English
// text labels; accessibility comes from aria-label + title; the
// visual treatment matches the bottom-right canvas controls.
func TestExplorer_D32bImpl3_IconOnlyModeToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// D32d-impl-4 — the mode toolbar lives inside the header's
	// .graph-view-menu container. The container must declare the
	// graph-view-menu class and host four icon buttons (Form, Context,
	// Authority, Knowledge placeholder). The .service-workbench-icon-
	// mode visual class is preserved on each button so the existing
	// hover/focus/.is-active rules in authority-graph.css carry over.
	if !strings.Contains(body, `class="graph-view-menu"`) {
		t.Error("D32d-impl-4: header must contain .graph-view-menu container")
	}
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	if menuIdx < 0 {
		t.Fatal("D32d-impl-4: .graph-view-menu markup missing")
	}
	// Bound the slice by the next sibling element on the header
	// (#iam-user-pill) which sits immediately after the menu.
	end := strings.Index(body[menuIdx:], `id="iam-user-pill"`)
	if end <= 0 {
		end = 4096
	}
	modesBlock := body[menuIdx : menuIdx+end]
	if strings.Count(modesBlock, "service-workbench-icon-mode") < 4 {
		t.Error("D32d-impl-4: there must be four .service-workbench-icon-mode buttons in the header menu (Form / Context / Authority / Knowledge)")
	}
	// Four SVG icons — one per button.
	if strings.Count(modesBlock, "<svg ") < 4 {
		t.Error("D32d-impl-4: each header menu button must contain an inline SVG icon")
	}
	// Each mode button declares aria-label + title for accessibility.
	for _, mode := range []struct{ ariaLabel, title string }{
		{"Form / Records view", "Form / Records view"},
		{"Context Graph", "Context Graph"},
		{"Authority Graph", "Authority Graph"},
		{"Knowledge Graph (coming soon)", "Knowledge Graph — coming soon"},
	} {
		if !strings.Contains(modesBlock, `aria-label="`+mode.ariaLabel+`"`) {
			t.Errorf("D32d-impl-4: header menu button must declare aria-label=%q", mode.ariaLabel)
		}
		if !strings.Contains(modesBlock, `title="`+mode.title+`"`) {
			t.Errorf("D32d-impl-4: header menu button must declare title=%q", mode.title)
		}
	}
	// Negative pin — no plain English label content inside any of the
	// three mode buttons. The labels live exclusively on aria-label
	// and title.
	for _, banned := range []string{
		`>Form View</button>`,
		`>Form / Records view</button>`,
		`>Context Graph</button>`,
		`>Authority Graph</button>`,
		`>Knowledge Graph</button>`,
	} {
		if strings.Contains(modesBlock, banned) {
			t.Errorf("D32d-impl-4: header menu must be icon-only — visible text label %q must be removed", banned)
		}
	}
}

// TestExplorer_D32bImpl3_IconToolbarCSS pins the supporting CSS
// rules. The icon-mode buttons inherit the dark drawer/control
// visual style; .service-workbench-icon-mode declares the fixed
// compact bounding box matching the bottom-right canvas controls.
func TestExplorer_D32bImpl3_IconToolbarCSS(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, want := range []string{
		".service-workbench-icon-modes",
		".service-workbench-icon-mode",
		".service-workbench-icon-mode.is-active",
		".service-workbench-icon-mode:hover",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32b-impl-3: authority-graph.css must declare %q (icon-mode toolbar style)", want)
		}
	}
}

// TestExplorer_D32bImpl3_DrawerNotHiddenInFocusMode is a regression
// guard. The drawer remains an operational surface in focus mode —
// the .gmap-right-rail container must not appear in the focus-mode
// hide selector chain.
func TestExplorer_D32bImpl3_DrawerNotHiddenInFocusMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	hideRuleIdx := strings.Index(css, "body.gmap-focus-mode .shell-header,")
	if hideRuleIdx < 0 {
		t.Fatal("D32b-impl-3: focus-mode hide rule must remain present")
	}
	hideEnd := strings.Index(css[hideRuleIdx:], "}")
	hideRule := css[hideRuleIdx : hideRuleIdx+hideEnd]
	if strings.Contains(hideRule, ".governance-map-details") || strings.Contains(hideRule, ".gmap-right-rail") {
		t.Error("D32b-impl-3: focus-mode hide rule must not collapse the right-side drawer")
	}
}

// ---------------------------------------------------------------------------
// D32e-impl-1 — Drift Analytics Letterbox Graph View.
//
// Tests in this section pin the end-state of D32e-impl-1:
//
//   • Five new frontend modules under assets/js/drift/ are served:
//       drift-chart-formatters.js
//       drift-chart-demo-adapter.js
//       drift-series-chart.js
//       drift-series-list.js
//       drift-analytics-panel.js
//   • Each module publishes its documented namespace.
//   • A new stylesheet assets/css/drift-analytics.css is served.
//   • The existing Runtime Evidence letterbox shell is preserved —
//     header, tabs, collapse toggle — but the tray title is now
//     "Drift Analytics" and the Drift tab body hosts the new panel
//     mount points.
//   • The Drift tab body markup contains the data-* mount points
//     the analytics panel module addresses.
//   • The Drift tab renderer in context-evidence-tray.js delegates
//     to MIDASExplorerDriftAnalytics.render() when present.
//   • Boot wiring invokes MIDASExplorerDriftAnalytics.init({hooks}).
//   • New modules do not call fetch directly or reference ExplorerAPI
//     except through the canonical typed methods.
//   • ExplorerAPI.drift gained seriesPointsByID for /v1/drift/series/{id}/points.
//   • Demo / synthetic data is clearly labelled as "DEMO DATA".
// ---------------------------------------------------------------------------

// d32eImpl1DriftModules is the canonical list of new frontend modules
// pinned by D32e-impl-1: { path, namespace, additional contains list }.
var d32eImpl1DriftModules = []struct {
	path     string
	ns       string
	contains []string
}{
	{
		path: "/explorer/assets/js/drift/drift-chart-formatters.js",
		ns:   "window.MIDASExplorerDriftChartFormatters",
		contains: []string{
			"formatTimestamp",
			"formatTimestampLong",
			"formatValue",
			"severityRank",
			"classifyPointSeverity",
		},
	},
	{
		path: "/explorer/assets/js/drift/drift-analytics-view-model.js",
		ns:   "window.MIDASExplorerDriftAnalyticsViewModel",
		contains: []string{
			"normalise:",
			"buildPoints:",
			"demoValues64:",
			"sourceClassification",
			"authority-path-divergence",
		},
	},
	{
		path: "/explorer/assets/js/drift/drift-chart-demo-adapter.js",
		ns:   "window.MIDASExplorerDriftChartAdapter",
		contains: []string{
			"fromServiceContext",
			"fromGraphNode",
			"isDemoData",
			"demo_derived",
		},
	},
	{
		path: "/explorer/assets/js/drift/drift-series-chart.js",
		ns:   "window.MIDASExplorerDriftSeriesChart",
		contains: []string{
			"render:",
			"clear:",
			"<svg",
			"role=\"img\"",
			"drift-series-chart-actual",
			"drift-series-chart-baseline",
			"drift-series-chart-anomaly",
		},
	},
	{
		path: "/explorer/assets/js/drift/drift-series-list.js",
		ns:   "window.MIDASExplorerDriftContributionRail",
		contains: []string{
			"render:",
			"clear:",
			"aria-pressed",
			"drift-contribution-row",
			"data-drift-contribution-id",
			"is-selected",
		},
	},
	{
		path: "/explorer/assets/js/drift/drift-analytics-panel.js",
		ns:   "window.MIDASExplorerDriftAnalytics",
		contains: []string{
			"init:",
			"render:",
			"clear:",
			"setExpanded:",
			"setSelectedSeries:",
			"data-drift-compact-summary",
			"Observed vs expected",
			"Top contributor",
		},
	},
}

// TestExplorer_D32eImpl1_DriftModulesServedAndExposeNamespace pins
// that each new module is served and publishes its namespace.
func TestExplorer_D32eImpl1_DriftModulesServedAndExposeNamespace(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, m := range d32eImpl1DriftModules {
		rec := performRequest(t, srv, http.MethodGet, m.path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("D32e-impl-1: %s want 200, got %d", m.path, rec.Code)
			continue
		}
		body := rec.Body.String()
		if !strings.Contains(body, m.ns+" = {") {
			t.Errorf("D32e-impl-1: %s must publish %s = { ... }", m.path, m.ns)
		}
		for _, want := range m.contains {
			if !strings.Contains(body, want) {
				t.Errorf("D32e-impl-1: %s must contain %q", m.path, want)
			}
		}
	}
}

// TestExplorer_D32eImpl1_DriftAnalyticsCSSServed pins that the new
// stylesheet is served and contains the key class names the modules
// emit.
func TestExplorer_D32eImpl1_DriftAnalyticsCSSServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/css/drift-analytics.css", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("D32e-impl-1: drift-analytics.css want 200, got %d", rec.Code)
	}
	css := rec.Body.String()
	for _, want := range []string{
		".drift-analytics-panel",
		".drift-analytics-demo-badge",
		".drift-compact-layout",
		".drift-compact-score",
		".drift-compact-status",
		".drift-compact-chart",
		".drift-compact-contributor",
		".drift-compact-observed",
		".drift-compact-expected",
		".drift-compact-deviation",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32e-impl-1: drift-analytics.css must declare %q", want)
		}
	}
	// The stylesheet must be linked in <head>.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `<link rel="stylesheet" href="/explorer/assets/css/drift-analytics.css">`) {
		t.Error("D32e-impl-1: index.html must <link> the drift-analytics.css stylesheet")
	}
}

// TestExplorer_D32eImpl1_DriftAnalyticsScriptOrder pins that the
// drift script tags load in dependency order so namespaces resolve
// at attach time:
//
//	formatters → demo-adapter → series-chart → series-list → panel
func TestExplorer_D32eImpl1_DriftAnalyticsScriptOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	tagFor := func(name string) string {
		return `<script src="/explorer/assets/js/drift/` + name + `.js"></script>`
	}
	tags := []string{
		tagFor("drift-chart-formatters"),
		tagFor("drift-analytics-view-model"),
		tagFor("drift-chart-demo-adapter"),
		tagFor("drift-series-chart"),
		tagFor("drift-series-list"),
		tagFor("drift-analytics-panel"),
	}
	idxs := make([]int, len(tags))
	for i, tag := range tags {
		idxs[i] = strings.Index(body, tag)
		if idxs[i] < 0 {
			t.Fatalf("D32e-impl-1: missing <script> tag: %s", tag)
		}
	}
	for i := 1; i < len(idxs); i++ {
		if idxs[i-1] >= idxs[i] {
			t.Errorf("D32e-impl-1: drift modules must load in order formatters→demo-adapter→series-chart→series-list→panel (got %v)", idxs)
		}
	}
}

// TestExplorer_D32eImpl1_DriftAnalyticsMountInTray pins the panel
// mount points inside the existing letterbox tray.
func TestExplorer_D32eImpl1_DriftAnalyticsMountInTray(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Tray shell preserved — header / tabs / collapse toggle survive.
	for _, want := range []string{
		`id="gmap-evidence-tray"`,
		`class="gmap-evidence-tray-header"`,
		`class="gmap-evidence-tray-tabs"`,
		`id="gmap-evidence-tray-toggle"`,
		`data-tab="drift"`,
		`data-tab="evidence"`,
		`data-tab="activity"`,
		`id="gmap-evidence-tray-panel"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32e-impl-1: tray landmark %q must remain (shell preservation)", want)
		}
	}
	// The tray title is now "Drift Analytics" (the panel name) — the
	// previous "Runtime Evidence" title was renamed to reflect the
	// upgraded purpose.
	titleIdx := strings.Index(body, `class="gmap-evidence-tray-title"`)
	if titleIdx < 0 {
		t.Fatal("D32e-impl-1: tray title element missing")
	}
	titleSlice := body[titleIdx:]
	closeIdx := strings.Index(titleSlice, `</span>`)
	if closeIdx < 0 {
		t.Fatal("D32e-impl-1: tray title closing tag missing")
	}
	titleBlock := titleSlice[:closeIdx]
	if !strings.Contains(titleBlock, "DRIFT ANALYTICS") {
		t.Errorf("D32e-impl-1: tray title must read \"DRIFT ANALYTICS\", got %q", titleBlock)
	}
	// The collapse/expand control aria-label + title still reference
	// the tray-as-control idiom but with the new analytics name.
	if !strings.Contains(body, `aria-label="Expand letterbox"`) {
		t.Error("D32e-impl-1: collapse/expand button must aria-label the compact letterbox")
	}
	// Required data-* mount points must exist inside the compact
	// Drift tab body/header.
	for _, want := range []string{
		`data-drift-compact-summary`,
		`data-drift-analytics-demo-badge`,
		`data-drift-analytics-title`,
		`data-drift-analytics-subtitle`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32e-impl-1: Drift tab body must include mount point %q", want)
		}
	}
	// The Drift tab body still hosts the drift-analytics-panel
	// container element so the rich layout is visible on first paint.
	if !strings.Contains(body, `class="drift-analytics-panel"`) {
		t.Error("D32e-impl-1: Drift tab body must host .drift-analytics-panel container")
	}
	if !strings.Contains(body, `data-drift-analysis-open`) {
		t.Error("D32e-impl-1: compact header must expose the disabled Open Drift Analysis affordance")
	}
	if !strings.Contains(body, `aria-label="Open Drift Analysis"`) {
		t.Error("D32e-impl-1: compact header open affordance must expose an accessible label")
	}
	if strings.Contains(body, `<span>Open Drift Analysis</span>`) {
		t.Error("D32e-impl-1: compact header open affordance must be icon-only")
	}
}

func TestExplorer_D32eImpl1_LetterboxToggleUsesSafeSvgIcon(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	toggleIdx := strings.Index(body, `id="gmap-evidence-tray-toggle"`)
	if toggleIdx < 0 {
		t.Fatal("D32e-impl-1: letterbox toggle missing")
	}
	toggleEnd := strings.Index(body[toggleIdx:], `</button>`)
	if toggleEnd < 0 {
		t.Fatal("D32e-impl-1: letterbox toggle closing tag missing")
	}
	toggleBlock := body[toggleIdx : toggleIdx+toggleEnd]
	for _, want := range []string{
		`id="gmap-evidence-tray-toggle"`,
		`aria-label="Expand letterbox"`,
		`title="Expand letterbox"`,
		`class="gmap-evidence-tray-toggle-icon"`,
		`<svg`,
		`viewBox="0 0 16 16"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32e-impl-1: letterbox toggle must contain %q", want)
		}
	}
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-evidence-tray.js")
	for _, want := range []string{
		"Collapse letterbox",
		"Expand letterbox",
		"gmapEvidenceTrayExpanded = !gmapEvidenceTrayExpanded",
		"toggle.setAttribute('aria-expanded'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32e-impl-1: letterbox toggle behaviour must contain %q", want)
		}
	}
	for _, bad := range []string{
		"â–¼",
		"â–²",
		"▲",
		"▼",
		"glyph.textContent",
	} {
		if strings.Contains(toggleBlock, bad) || strings.Contains(js, bad) {
			t.Errorf("D32e-impl-1: letterbox toggle must not contain fragile/corrupt glyph token %q", bad)
		}
	}
}

func TestExplorer_D32eImpl1_LetterboxHeaderConsolidatesTabs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	headerIdx := strings.Index(body, `class="gmap-evidence-tray-header"`)
	if headerIdx < 0 {
		t.Fatal("D32e-impl-1: consolidated tray header missing")
	}
	bodyIdx := strings.Index(body, `class="gmap-evidence-tray-body"`)
	if bodyIdx < 0 {
		t.Fatal("D32e-impl-1: tray body missing")
	}
	if headerIdx > bodyIdx {
		t.Fatal("D32e-impl-1: tray header must precede body")
	}
	headerBlock := body[headerIdx:bodyIdx]
	for _, want := range []string{
		`class="gmap-evidence-tray-header-left"`,
		`data-drift-analytics-title>DRIFT ANALYTICS`,
		`id="gmap-evidence-tray-node"`,
		`class="gmap-evidence-tray-tabs" role="tablist"`,
		`class="gmap-evidence-tray-header-right"`,
		`data-drift-analytics-demo-badge`,
		`class="gmap-evidence-tray-open-analysis-icon"`,
		`aria-label="Open Drift Analysis"`,
		`title="Open Drift Analysis"`,
	} {
		if !strings.Contains(headerBlock, want) {
			t.Errorf("D32e-impl-1: consolidated tray header must contain %q", want)
		}
	}
	for _, bad := range []string{
		`data-drift-analytics-severity-badge`,
		`>WATCH<`,
	} {
		if strings.Contains(headerBlock, bad) {
			t.Errorf("D32e-impl-1: consolidated tray header must not contain status chip token %q", bad)
		}
	}
	for _, tab := range []string{"overview", "drift", "evidence", "activity"} {
		want := `data-tab="` + tab + `"`
		if strings.Count(headerBlock, want) != 1 {
			t.Errorf("D32e-impl-1: consolidated tray header must contain exactly one %s tab", tab)
		}
	}
	if strings.Count(headerBlock, `role="tab"`) != 4 {
		t.Fatalf("D32e-impl-1: consolidated tray header must contain exactly four tab roles")
	}
	if !strings.Contains(headerBlock, `data-tab="drift" role="tab" aria-selected="true" aria-current="page"`) {
		t.Error("D32e-impl-1: Drift tab must remain active/current on first paint")
	}
	trayJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-evidence-tray.js")
	for _, want := range []string{
		"setAttribute('aria-current', 'page')",
		"removeAttribute('aria-current')",
	} {
		if !strings.Contains(trayJS, want) {
			t.Errorf("D32e-impl-1: tray tab state must contain %q", want)
		}
	}
	bodyOpenEnd := strings.Index(body[bodyIdx:], `id="gmap-evidence-tray-panel"`)
	if bodyOpenEnd < 0 {
		t.Fatal("D32e-impl-1: tray panel missing")
	}
	bodyBeforePanel := body[bodyIdx : bodyIdx+bodyOpenEnd]
	if strings.Contains(bodyBeforePanel, `gmap-evidence-tray-tabs`) {
		t.Error("D32e-impl-1: tabs must not render as a separate body row before the panel")
	}
}

func TestExplorer_D32eImpl1_LetterboxTabsUseUnderlineActiveState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	headerIdx := strings.Index(body, `class="gmap-evidence-tray-header"`)
	bodyIdx := strings.Index(body, `class="gmap-evidence-tray-body"`)
	if headerIdx < 0 || bodyIdx < 0 || headerIdx > bodyIdx {
		t.Fatal("D32e-impl-1: consolidated tray header/body missing or out of order")
	}
	headerBlock := body[headerIdx:bodyIdx]
	for _, tab := range []string{"overview", "drift", "evidence", "activity"} {
		if strings.Count(headerBlock, `data-tab="`+tab+`"`) != 1 {
			t.Fatalf("D32e-impl-1: expected exactly one %s tab", tab)
		}
	}
	if !strings.Contains(headerBlock, `data-tab="drift" role="tab" aria-selected="true" aria-current="page"`) {
		t.Fatal("D32e-impl-1: Drift tab must remain active/current on first paint")
	}

	css := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	ruleBody := func(selector string) string {
		needle := selector + " {"
		idx := strings.Index(css, needle)
		if idx < 0 {
			t.Fatalf("D32e-impl-1: CSS selector %q missing", selector)
		}
		start := idx + len(needle)
		end := strings.Index(css[start:], "}")
		if end < 0 {
			t.Fatalf("D32e-impl-1: CSS selector %q has no closing brace", selector)
		}
		return css[start : start+end]
	}

	baseTab := ruleBody(".gmap-evidence-tray-tab")
	tabUnderline := ruleBody(".gmap-evidence-tray-tab::after")
	activeTab := ruleBody(".gmap-evidence-tray-tab.is-active")
	activeUnderline := ruleBody(".gmap-evidence-tray-tab.is-active::after")
	hoverTab := ruleBody(".gmap-evidence-tray-tab:hover:not(.is-active)")
	lightActiveTab := ruleBody(`:root[data-theme="light"] .gmap-evidence-tray-tab.is-active`)
	lightHoverTab := ruleBody(`:root[data-theme="light"] .gmap-evidence-tray-tab:hover:not(.is-active)`)

	for _, want := range []string{
		"border: 0;",
		"border-radius: 0;",
		"position: relative;",
	} {
		if !strings.Contains(baseTab, want) {
			t.Errorf("D32e-impl-1: base tray tab style must contain %q", want)
		}
	}
	for _, want := range []string{
		"background: transparent;",
		"border-color: transparent;",
		"box-shadow: none;",
		"font-weight: 650;",
	} {
		if !strings.Contains(activeTab, want) {
			t.Errorf("D32e-impl-1: active tray tab style must contain %q", want)
		}
	}
	if !strings.Contains(tabUnderline, "height: 2px;") {
		t.Error("D32e-impl-1: tray tab underline must be 2px high")
	}
	if !strings.Contains(activeUnderline, "background: var(--primary);") {
		t.Error("D32e-impl-1: active tray tab underline must use the primary token")
	}
	for _, block := range []struct {
		name string
		body string
	}{
		{"base tab", baseTab},
		{"active tab", activeTab},
		{"hover tab", hoverTab},
		{"light active tab", lightActiveTab},
		{"light hover tab", lightHoverTab},
	} {
		for _, bad := range []string{
			"border-radius: 999px;",
			"background: var(--surface-container",
			"box-shadow: inset 0 -2px 0 var(--primary);",
		} {
			if strings.Contains(block.body, bad) {
				t.Errorf("D32e-impl-1: %s must not retain old pill/button treatment %q", block.name, bad)
			}
		}
	}
}

// TestExplorer_D32eImpl1_TrayDriftTabDelegatesToAnalytics pins that
// the Context evidence-tray module routes its Drift tab through the
// new analytics panel module rather than emitting the legacy controls
// + tiles layout. The fallback to the legacy renderer is preserved
// for test isolation, but production builds always have the module
// loaded.
func TestExplorer_D32eImpl1_TrayDriftTabDelegatesToAnalytics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-evidence-tray.js")
	if !strings.Contains(js, "function renderGmapEvidenceTrayDriftPanel()") {
		t.Fatal("D32e-impl-1: context-evidence-tray.js must still expose renderGmapEvidenceTrayDriftPanel")
	}
	// The renderer delegates to MIDASExplorerDriftAnalytics.render()
	// when the module is present.
	if !strings.Contains(js, "window.MIDASExplorerDriftAnalytics.render()") {
		t.Error("D32e-impl-1: Drift tab renderer must dispatch to MIDASExplorerDriftAnalytics.render() when the module is present")
	}
	// The analytics layout must be restored if a previous tab switch
	// overwrote the panel innerHTML.
	if !strings.Contains(js, "data-drift-compact-summary") {
		t.Error("D32e-impl-1: Drift tab renderer must restore the compact summary mount when missing")
	}
	if strings.Contains(js, "data-drift-analytics-series-list") || strings.Contains(js, "data-drift-analytics-chart") {
		t.Error("D32e-impl-1: Drift tab renderer must not restore rich rail/chart mounts in the compact letterbox")
	}
}

// TestExplorer_D32eImpl1_BootWiring pins that the inline IIFE invokes
// MIDASExplorerDriftAnalytics.init with hooks that resolve the
// currently-selected business service + graph node.
func TestExplorer_D32eImpl1_BootWiring(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, "MIDASExplorerDriftAnalytics.init({") {
		t.Fatal("D32e-impl-1: index.html must invoke MIDASExplorerDriftAnalytics.init({ ... }) at boot")
	}
	for _, want := range []string{
		"getServiceId:",
		"getSelectedNodeId:",
		"currentSelectedService",
		"gmapSelectedId",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32e-impl-1: boot wiring must include hook token %q", want)
		}
	}
}

// TestExplorer_D32eImpl1_NoDirectFetchInDriftModules pins API
// discipline: new drift modules must not call fetch(...) and must
// not invoke ExplorerAPI directly (they use the demo adapter today;
// when wired to backend data they will go through ExplorerAPI.drift
// in the panel module, not in the chart/list/formatter modules).
func TestExplorer_D32eImpl1_NoDirectFetchInDriftModules(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, m := range d32eImpl1DriftModules {
		body := performRequest(t, srv, http.MethodGet, m.path, nil).Body.String()
		if strings.Contains(body, "fetch(") {
			t.Errorf("D32e-impl-1: %s must not call fetch(...)", m.path)
		}
		// The chart / list / formatters / demo adapter must not even
		// reference ExplorerAPI. The panel module is allowed to read
		// ExplorerAPI.drift in a future tranche when real data is
		// wired in, but today it must not.
		if strings.Contains(body, "ExplorerAPI") {
			t.Errorf("D32e-impl-1: %s must not invoke ExplorerAPI directly in this tranche", m.path)
		}
	}
}

// TestExplorer_D32eImpl1_APIClientDriftSeriesPointsByID pins the new
// typed accessor added to ExplorerAPI.drift for the existing
// per-series points endpoint. This is the canonical path the
// analytics panel will use once real series ids are mapped per
// service; today the demo adapter covers the gap.
func TestExplorer_D32eImpl1_APIClientDriftSeriesPointsByID(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/core/api-client.js")
	for _, want := range []string{
		"seriesPointsByID:",
		"/v1/drift/series/",
		"seriesObservations:",
		"seriesAnnotations:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32e-impl-1: api-client.js must declare %q", want)
		}
	}
}

// TestExplorer_D32eImpl1_ChartHasAccessibleLabel pins that the
// chart renderer emits an <svg role="img" aria-label="..."> root so
// assistive tech announces the chart with a meaningful name.
func TestExplorer_D32eImpl1_ChartHasAccessibleLabel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-series-chart.js")
	if !strings.Contains(js, `role="img"`) {
		t.Error("D32e-impl-1: drift-series-chart.js must emit role=\"img\" on the chart SVG")
	}
	if !strings.Contains(js, `aria-label="`) {
		t.Error("D32e-impl-1: drift-series-chart.js must emit aria-label on the chart SVG")
	}
	// Empty state is announced as a status region.
	if !strings.Contains(js, `role="status"`) {
		t.Error("D32e-impl-1: chart empty state must use role=\"status\" so assistive tech announces it")
	}
}

// TestExplorer_D32eImpl1_ContributionRailIsKeyboardAccessible pins
// that contribution rows are <button> elements with aria-pressed
// reflecting selection.
func TestExplorer_D32eImpl1_ContributionRailIsKeyboardAccessible(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-series-list.js")
	for _, want := range []string{
		`<button type="button"`,
		`aria-pressed`,
		`aria-label`,
		`data-drift-contribution-id`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32e-impl-1: drift-series-list.js must emit %q for accessibility", want)
		}
	}
}

// TestExplorer_D32eImpl1_DemoDataHonestlyLabelled pins the demo-data
// labelling contract. The demo adapter must:
//   - return isDemo: true on every series object it produces, and
//   - surface a DEMO DATA badge that the panel module conditionally
//     reveals via the data-drift-analytics-demo-badge slot.
func TestExplorer_D32eImpl1_DemoDataHonestlyLabelled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-chart-demo-adapter.js")
	if !strings.Contains(adapterJS, "sourceClassification: 'demo_derived'") {
		t.Error("D32e-impl-1: demo adapter must tag the view model with sourceClassification demo_derived")
	}
	panelJS := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	if !strings.Contains(panelJS, "isDemoData") {
		t.Error("D32e-impl-1: analytics panel must call adapter.isDemoData(...) to decide demo-badge visibility")
	}
	// The visible badge text is "DEMO DATA"; the panel writes it
	// dynamically. Pin the literal string in the panel source.
	if !strings.Contains(panelJS, "DEMO DATA") {
		t.Error("D32e-impl-1: analytics panel must surface a 'DEMO DATA' badge when adapter says the series is synthetic")
	}
	// The static HTML badge mount must exist with the hidden
	// attribute so it does not flash on first paint before the panel
	// has classified its data.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	badgeIdx := strings.Index(body, `data-drift-analytics-demo-badge`)
	if badgeIdx < 0 {
		t.Fatal("D32e-impl-1: data-drift-analytics-demo-badge mount missing")
	}
	tagEnd := strings.Index(body[badgeIdx:], `>`)
	if tagEnd <= 0 {
		t.Fatal("D32e-impl-1: demo badge open tag has no closing >")
	}
	openTag := body[badgeIdx : badgeIdx+tagEnd+1]
	if !strings.Contains(openTag, "hidden") {
		t.Errorf("D32e-impl-1: data-drift-analytics-demo-badge must default hidden, got %q", openTag)
	}
}

// TestExplorer_D32eImpl1_ChartSupportsBaselineAndAnomaly pins that
// the chart renders three visual layers when the series supplies
// them: actual line, baseline reference, and anomaly markers. This
// is the operator-grade analytical layout the prompt asks for.
func TestExplorer_D32eImpl1_ChartSupportsBaselineAndAnomaly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-series-chart.js")
	for _, want := range []string{
		"drift-series-chart-actual",
		"drift-series-chart-baseline",
		"drift-series-chart-anomaly",
		// Y + X axis labels rendered.
		"drift-series-chart-axis-label",
		// Grid lines for readability.
		"drift-series-chart-grid",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32e-impl-1: drift-series-chart.js must emit %q", want)
		}
	}
}

// TestExplorer_D32eImpl1_PanelSeverityAndDemoFlow pins the analytics
// panel's decision logic: top-severity classification + demo-badge
// visibility are both driven by the adapter result. No client-side
// recomputation of authoritative severity beyond ordering.
func TestExplorer_D32eImpl1_PanelSeverityAndDemoFlow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	for _, want := range []string{
		"_renderHeader",
		"_renderCompact",
		"_compactChart",
		"_topContribution",
		"Observed vs expected",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32e-impl-1: drift-analytics-panel.js must include %q", want)
		}
	}
	for _, bad := range []string{
		"MIDASExplorerDriftContributionRail",
		"MIDASExplorerDriftSeriesChart",
		"_renderContributionRail",
	} {
		if strings.Contains(js, bad) {
			t.Errorf("D32e-impl-1: compact letterbox panel must not mount rich renderer token %q", bad)
		}
	}
}

func TestExplorer_D32eImpl1_CompactLetterboxSummaryContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`data-drift-compact-summary`,
		`data-drift-analysis-open`,
		`aria-label="Open Drift Analysis"`,
		`DRIFT ANALYTICS`,
		`id="gmap-evidence-tray-node"`,
		`data-drift-analytics-demo-badge`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32e-impl-1: compact letterbox markup must contain %q", want)
		}
	}
	panel := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	for _, want := range []string{
		"Drift score",
		"0.146",
		"WATCH",
		"0.006 below breach",
		"Top contributor",
		"Authority-path divergence",
		"49%",
		"of current drift",
		"Last 30 days",
		"Observed vs expected",
		"Observed",
		"Expected (declared baseline)",
		"&middot;",
		"Next:",
		"Escalation-rate deviation",
		"26%",
		"Select a node to view drift analysis",
		"Loading drift analysis",
		"drift-compact-deviation",
		"May 30",
		"Jun 07",
		"Jun 15",
		"Jun 23",
		"Jun 29",
		"Demo evidence",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("D32e-impl-1: compact letterbox panel must contain %q", want)
		}
	}
}

func TestExplorer_D32eImpl1_Tranche1Spec14ViewModelContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vm := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-view-model.js")
	for _, want := range []string{
		"DRIFT SCORE (composite)",
		"Authority-path divergence",
		"Escalation-rate deviation",
		"Evidence-completeness gap",
		"Outcome-mix shift",
		"authority-path-divergence",
		"escalation-rate-deviation",
		"evidence-completeness-gap",
		"outcome-mix-shift",
		"event-policy-update-jun-01",
		"event-profile-change-jun-07",
		"event-incident-jun-13",
		"event-policy-update-jun-21",
		"event-profile-change-jun-27",
		"sourceClassification",
		"demo_derived",
		"May 30",
		"Jun 03",
		"Jun 07",
		"Jun 11",
		"Jun 15",
		"Jun 19",
		"Jun 23",
		"Jun 27",
		"Jun 29",
		"0.000",
		"0.050",
		"0.100",
		"0.150",
		"0.200",
	} {
		if !strings.Contains(vm, want) {
			t.Errorf("D32e-tranche-1: view-model contract must contain %q", want)
		}
	}
	valuesStart := strings.Index(vm, "var VALUE_SERIES = [")
	if valuesStart < 0 {
		t.Fatal("D32e-tranche-1: view model must declare the fixed VALUE_SERIES array")
	}
	valuesEnd := strings.Index(vm[valuesStart:], "];")
	if valuesEnd < 0 {
		t.Fatal("D32e-tranche-1: fixed VALUE_SERIES array must close")
	}
	valuesBlock := vm[valuesStart : valuesStart+valuesEnd]
	if got := strings.Count(valuesBlock, "."); got != 64 {
		t.Fatalf("D32e-tranche-1: fixed VALUE_SERIES must contain 64 decimal values, got %d", got)
	}
}

func TestExplorer_D32eImpl1_Tranche1ChartPinsTicksAndEvents(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-series-chart.js")
	for _, want := range []string{
		"viewModel.yTicks",
		"viewModel.xTicks",
		"data-drift-event-id",
		"drift-series-zone-label-breach",
		"drift-series-zone-label-watch",
		"drift-series-zone-label-normal",
		"drift-series-area-gradient",
		"Observed vs. expected - drift is the deviation, not the level",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32e-tranche-1: chart must contain %q", want)
		}
	}
}

func TestExplorer_D32eImpl1_Tranche1ThemeTokensComplete(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/drift-analytics.css")
	for _, want := range []string{
		":root {",
		`:root[data-theme="light"]`,
		"--drift-chart-gradient-start",
		"--drift-chart-gradient-end",
		"--drift-deviation-fill",
		"--drift-selected-surface",
		"--drift-toolbar-surface",
		"--drift-grey",
		"--drift-contribution-track",
		"--drift-red-bg",
		"--drift-amber-bg",
		"--drift-green-bg",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32e-tranche-1: drift CSS must define token %q", want)
		}
	}
	if !strings.Contains(css, "--drift-deviation-fill: rgba(96, 165, 250, 0.10);") {
		t.Error("D32e-tranche-1: dark theme must define a visible compact deviation fill")
	}
	if !strings.Contains(css, "--drift-deviation-fill: rgba(47, 109, 246, 0.16);") {
		t.Error("D32e-tranche-1: light theme must define a visible compact deviation fill")
	}
	if !strings.Contains(css, ".drift-compact-deviation {\n  fill: var(--drift-deviation-fill);") {
		t.Error("D32e-tranche-1: compact deviation path must use the theme-specific deviation fill token")
	}
}

func TestExplorer_D32eImpl1_CompactLetterboxFits230WithoutShrinkingChart(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	driftCSS := getExplorerAsset(t, srv, "/explorer/assets/css/drift-analytics.css")
	for _, want := range []string{
		".gmap-evidence-tray.is-expanded",
		"height: 230px;",
		"padding: 8px 10px;",
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D32e-tranche-1: governance-map.css must contain %q", want)
		}
	}
	for _, want := range []string{
		"gap: 0;",
		"padding: 8px 11px;",
		"padding: 3px 6px 3px 2px;",
		"height: 152px;",
		"stroke-width: 2.5;",
		"opacity: 0.34;",
		"font-variant-numeric: tabular-nums;",
	} {
		if !strings.Contains(driftCSS, want) {
			t.Errorf("D32e-tranche-1: compact drift CSS must contain %q", want)
		}
	}
}

func TestExplorer_D32eImpl1_CompactScoreCardAndChartHeaderStructure(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	panel := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/drift-analytics.css")
	for _, want := range []string{
		"drift-compact-card",
		"drift-compact-divider",
		"drift-compact-score-row",
		"drift-compact-status",
		"drift-compact-chart-header",
		"drift-compact-series-title",
		"Observed vs expected",
		"Expected (declared baseline)",
		"drift-compact-runner-up",
	} {
		if !strings.Contains(panel, want) && !strings.Contains(css, want) {
			t.Errorf("D32e-tranche-1: compact Drift assets must contain left-rail token %q", want)
		}
	}
	if strings.Contains(panel, "drift-compact-chart-title-row") {
		t.Error("D32e-tranche-1: compact chart must not render a title/legend chrome row above the plot")
	}
	if strings.Contains(panel, "Drift is the shaded deviation.") {
		t.Error("D32e-tranche-1: compact left rail must not reintroduce the old chart subtitle")
	}
	if strings.Contains(panel, "DRIFT SCORE") || strings.Contains(panel, "TOP CONTRIBUTOR") ||
		strings.Contains(panel, "drift-compact-score--") {
		t.Error("D32e-tranche-1: compact labels must be sentence case and must not render a vertical status accent class")
	}
	if strings.Contains(css, "border-left-width") || strings.Contains(css, "border-left-color") ||
		strings.Contains(css, "drift-compact-score--") {
		t.Error("D32e-tranche-1: compact CSS must remove the score-card vertical status accent")
	}
	if !strings.Contains(panel, `'<div class="drift-compact-chart">' +`) ||
		!strings.Contains(panel, `'<div class="drift-compact-chart-header">'`) {
		t.Error("D32e-tranche-1: middle compact chart container must mount the compact chart header and plot")
	}
	heroIdx := strings.Index(panel, "drift-compact-score-value")
	keyIdx := strings.Index(panel, "drift-compact-series-title")
	chartIdx := strings.Index(panel, "drift-compact-chart-header")
	if heroIdx < 0 || keyIdx < 0 || chartIdx < 0 || keyIdx < chartIdx || keyIdx < heroIdx {
		t.Error("D32e-tranche-1: chart key must render in the chart header after the Drift score hero block")
	}
	scoreBlockEnd := strings.Index(panel, `'<div class="drift-compact-chart">' +`)
	if scoreBlockEnd < 0 {
		t.Fatal("D32e-tranche-1: compact chart block missing")
	}
	scoreBlock := panel[:scoreBlockEnd]
	if strings.Contains(scoreBlock, "drift-compact-legend") {
		t.Error("D32e-tranche-1: left score card must not contain the chart legend")
	}
	if !strings.Contains(scoreBlock, "drift-compact-status") || !strings.Contains(scoreBlock, "WATCH") {
		t.Error("D32e-tranche-1: WATCH status chip must live in the score card")
	}
}

func TestExplorer_D32eImpl1_CompactChartUsesMeasuredWidth(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	panel := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/drift-analytics.css")
	for _, want := range []string{
		"chartWidth",
		"ResizeObserver",
		"_measureChartWidth",
		"_chartContentWidth",
		"chart.clientWidth",
		"var W = Math.max(360, Math.floor(width || 720));",
		"var pad = { top: 16, right: 32, bottom: 28, left: 46 };",
		"(pad.left - 8)",
		"0.000",
		"0.100",
		"0.200",
		"0.146",
		`preserveAspectRatio="xMinYMin meet"`,
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("D32e-tranche-1: compact chart measured-width renderer must contain %q", want)
		}
	}
	for _, bad := range []string{
		"var W = 560;",
		"preserveAspectRatio=\"none\"",
		"pad = { top: 16, right: 28, bottom: 28, left: 48 }",
		"var pad = { top: 16, right: 40, bottom: 28, left: 34 };",
	} {
		if strings.Contains(panel, bad) {
			t.Errorf("D32e-tranche-1: compact chart must not retain fixed/oversized width token %q", bad)
		}
	}
	if !strings.Contains(css, "padding: 0 2px 1px 44px;") {
		t.Error("D32e-tranche-1: compact chart header should align with the reduced y-axis gutter")
	}
}

func TestExplorer_D32eImpl1_Tranche1CleanBreakNegatives(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	panel := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	rail := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-series-list.js")
	tray := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-evidence-tray.js")
	driftCSS := getExplorerAsset(t, srv, "/explorer/assets/css/drift-analytics.css")
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, bad := range []string{
		"window.MIDASExplorerDriftSeriesList",
		"drift-series-row-sev-",
		"gmap-evidence-tray-metric",
		"gmap-evidence-tray-range",
	} {
		if strings.Contains(panel, bad) || strings.Contains(rail, bad) {
			t.Errorf("D32e-tranche-1: new Drift modules must not contain legacy token %q", bad)
		}
	}
	for _, bad := range []string{
		"wireGmapEvidenceTraySelectors();",
		`aria-label="Drift series list"`,
	} {
		if strings.Contains(tray, bad) {
			t.Errorf("D32e-tranche-1: tray Drift path must not use legacy fallback token %q", bad)
		}
	}
	for _, bad := range []string{
		"fonts.googleapis",
		"fonts.gstatic",
		"@import url",
		"@font-face",
		"IBM Plex",
		"new cytoscape(",
	} {
		if strings.Contains(panel, bad) || strings.Contains(rail, bad) || strings.Contains(tray, bad) ||
			strings.Contains(driftCSS, bad) || strings.Contains(gmapCSS, bad) || strings.Contains(body, bad) {
			t.Errorf("D32e-tranche-1: compact Drift assets must not introduce external font/network/Cytoscape token %q", bad)
		}
	}
}

// TestExplorer_D32eImpl1_LegacyEvidenceShellPreserved pins that the
// surrounding letterbox shell (focus-mode-resilient placement, tray
// header, evidence/activity tab routing through the legacy
// context-evidence-tray module) is unchanged.
func TestExplorer_D32eImpl1_LegacyEvidenceShellPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`id="gmap-evidence-tray"`,
		`class="gmap-evidence-tray-tabs"`,
		`data-tab="overview"`,
		`data-tab="drift"`,
		`data-tab="evidence"`,
		`data-tab="activity"`,
		`id="gmap-evidence-tray-toggle"`,
		`id="gmap-evidence-tray-panel"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32e-impl-1: tray shell landmark %q must remain (the analytics panel replaces only Drift tab content)", want)
		}
	}
}

// ---------------------------------------------------------------------------
// D32d-impl-4 — Workbench Mode Menu (location updated by D32b-impl-4).
//
// Tests in this section pin the end-state of the workbench mode menu:
//
//   • The Service Workbench mode toolbar was migrated out of
//     .governance-map-toolbar-left (D32b-impl-3) into the Explorer
//     header's .shell-header-right (D32d-impl-4), and then BACK into
//     the workbench toolbar's right zone .governance-map-toolbar-right
//     (D32b-impl-4) because the shell header is hidden in focus mode.
//   • Four buttons: Form / Records (data-workbench-mode="form"),
//     Context Graph (data-workbench-mode="context" data-lens=
//     "context"), Authority Graph (data-workbench-mode="authority"
//     data-lens="authority"), Knowledge Graph placeholder
//     (data-workbench-mode="knowledge" + disabled + aria-disabled).
//   • setWorkbenchMode('knowledge') early-returns so a deep-link or
//     programmatic call does not crash.
//   • The existing dispatcher (setWorkbenchMode) drives all four
//     surfaces — toolbar click → setWorkbenchMode → MIDASExplorer
//     Services / shell.setActiveLens — so the menu is a drop-in
//     replacement without state-machine changes.
//   • assets/css/graph-view-menu.css ships and is linked from index.html.
//   • Existing graph behaviour (Context Graph, Authority Graph, drawer,
//     focus-mode) is preserved (covered by other test groups).
//   • D32b-impl-4 — the MIDAS logo now sources the SAME canonical SVG
//     for both the sidebar mark and the favicon, so the geometry is
//     byte-identical across the two render contexts.
// ---------------------------------------------------------------------------

// TestExplorer_D32dImpl4_HeaderGraphViewMenuExists pins that the
// graph view menu container lives inside the visible workbench
// toolbar (NOT in the focus-mode-hidden shell header).
func TestExplorer_D32dImpl4_HeaderGraphViewMenuExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `class="graph-view-menu"`) {
		t.Fatal("D32b-impl-4: .graph-view-menu container must exist")
	}
	if !strings.Contains(body, `role="group"`) || !strings.Contains(body, `aria-label="Graph view"`) {
		t.Error("D32b-impl-4: .graph-view-menu must declare role=\"group\" + aria-label=\"Graph view\"")
	}
	toolbarRightIdx := strings.Index(body, `class="governance-map-toolbar-right"`)
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	if toolbarRightIdx < 0 {
		t.Fatal("D32b-impl-4: .governance-map-toolbar-right missing")
	}
	if menuIdx < toolbarRightIdx {
		t.Errorf("D32b-impl-4: .graph-view-menu must sit inside .governance-map-toolbar-right; menu=%d toolbarRight=%d", menuIdx, toolbarRightIdx)
	}
}

// TestExplorer_D32dImpl4_FourMenuButtons pins the four icon buttons
// in the header menu: Form / Records, Context Graph, Authority Graph,
// Knowledge Graph (placeholder).
func TestExplorer_D32dImpl4_FourMenuButtons(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	if menuIdx < 0 {
		t.Fatal("D32d-impl-4: .graph-view-menu missing")
	}
	// D32b-impl-4 — the menu was relocated from .shell-header-right
	// into the workbench toolbar's right group. The new end-marker
	// is the polite-live announce span that sits immediately after
	// the menu inside .governance-map-toolbar-right.
	end := strings.Index(body[menuIdx:], `class="gmap-view-mode-feedback"`)
	if end <= 0 {
		t.Fatal("D32d-impl-4: cannot locate end of header menu block")
	}
	block := body[menuIdx : menuIdx+end]

	// Each of the four modes must be present with the right
	// data-workbench-mode and accessible-name attributes. Visible
	// English labels are NOT required — accessibility comes from
	// aria-label + title only.
	for _, mode := range []struct {
		dataMode  string
		ariaLabel string
		title     string
	}{
		{"form", "Form / Records view", "Form / Records view"},
		{"context", "Context Graph", "Context Graph"},
		{"authority", "Authority Graph", "Authority Graph"},
		{"knowledge", "Knowledge Graph (coming soon)", "Knowledge Graph — coming soon"},
	} {
		if !strings.Contains(block, `data-workbench-mode="`+mode.dataMode+`"`) {
			t.Errorf("D32d-impl-4: header menu missing data-workbench-mode=%q button", mode.dataMode)
		}
		if !strings.Contains(block, `aria-label="`+mode.ariaLabel+`"`) {
			t.Errorf("D32d-impl-4: header menu missing aria-label=%q", mode.ariaLabel)
		}
		if !strings.Contains(block, `title="`+mode.title+`"`) {
			t.Errorf("D32d-impl-4: header menu missing title=%q", mode.title)
		}
	}
	// Reading order: form → context → authority → knowledge.
	idx := func(s string) int { return strings.Index(block, s) }
	f, c, a, k := idx(`data-workbench-mode="form"`), idx(`data-workbench-mode="context"`),
		idx(`data-workbench-mode="authority"`), idx(`data-workbench-mode="knowledge"`)
	if !(f < c && c < a && a < k) {
		t.Errorf("D32d-impl-4: header menu reading order broken (form=%d context=%d authority=%d knowledge=%d)", f, c, a, k)
	}
}

// TestExplorer_D32dImpl4_ActiveStateAndAriaPressed pins that the
// active state is exposed through both .is-active class and
// aria-pressed attribute on the Context Graph default mode.
func TestExplorer_D32dImpl4_ActiveStateAndAriaPressed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	if menuIdx < 0 {
		t.Fatal("D32b-impl-4: graph view menu missing")
	}
	// D32b-impl-4 — the menu sits inside .governance-map-toolbar-right;
	// the polite-live announce span (.gmap-view-mode-feedback) is the
	// next sibling and provides a stable end-marker.
	end := strings.Index(body[menuIdx:], `class="gmap-view-mode-feedback"`)
	if end <= 0 {
		t.Fatal("D32b-impl-4: cannot locate end of graph view menu block")
	}
	block := body[menuIdx : menuIdx+end]
	// Context Graph default-active in the graph view menu.
	if !strings.Contains(block, `data-workbench-mode="context" data-lens="context"`) {
		t.Error("D32d-impl-4: Context Graph button must carry data-workbench-mode + data-lens")
	}
	contextBtnIdx := strings.Index(block, `data-workbench-mode="context"`)
	tagStart := strings.LastIndex(block[:contextBtnIdx], "<button")
	tagEnd := strings.Index(block[contextBtnIdx:], ">")
	if tagStart < 0 || tagEnd <= 0 {
		t.Fatal("D32d-impl-4: cannot isolate Context Graph button opening tag")
	}
	ctxBtn := block[tagStart : contextBtnIdx+tagEnd+1]
	if !strings.Contains(ctxBtn, "is-active") {
		t.Error("D32d-impl-4: Context Graph button must default .is-active")
	}
	if !strings.Contains(ctxBtn, `aria-pressed="true"`) {
		t.Error("D32d-impl-4: Context Graph button must default aria-pressed=\"true\"")
	}
	// The other three modes default not-active.
	for _, dataMode := range []string{"form", "authority", "knowledge"} {
		bIdx := strings.Index(block, `data-workbench-mode="`+dataMode+`"`)
		tStart := strings.LastIndex(block[:bIdx], "<button")
		tEnd := strings.Index(block[bIdx:], ">")
		if tStart < 0 || tEnd <= 0 {
			continue
		}
		btn := block[tStart : bIdx+tEnd+1]
		if !strings.Contains(btn, `aria-pressed="false"`) {
			t.Errorf("D32d-impl-4: %q button must default aria-pressed=\"false\"", dataMode)
		}
	}
}

// TestExplorer_D32dImpl4_KnowledgePlaceholderDisabled pins the
// Knowledge Graph placeholder contract: visible, but disabled +
// aria-disabled so the dispatcher early-returns.
func TestExplorer_D32dImpl4_KnowledgePlaceholderDisabled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	kIdx := strings.Index(body, `data-workbench-mode="knowledge"`)
	if kIdx < 0 {
		t.Fatal("D32d-impl-4: Knowledge Graph placeholder missing")
	}
	tagStart := strings.LastIndex(body[:kIdx], "<button")
	tagEnd := strings.Index(body[kIdx:], ">")
	if tagStart < 0 || tagEnd <= 0 {
		t.Fatal("D32d-impl-4: cannot isolate Knowledge button opening tag")
	}
	kBtn := body[tagStart : kIdx+tagEnd+1]
	if !strings.Contains(kBtn, `aria-disabled="true"`) {
		t.Error("D32d-impl-4: Knowledge Graph placeholder must declare aria-disabled=\"true\"")
	}
	if !strings.Contains(kBtn, ` disabled`) {
		t.Error("D32d-impl-4: Knowledge Graph placeholder must declare native `disabled` so click events are suppressed")
	}
	if !strings.Contains(kBtn, "is-placeholder") {
		t.Error("D32d-impl-4: Knowledge Graph placeholder must declare .is-placeholder for CSS dimming")
	}
	// Negative pin — Knowledge button must not have data-lens (no
	// underlying lens to register).
	if strings.Contains(kBtn, `data-lens=`) {
		t.Error("D32d-impl-4: Knowledge Graph placeholder must not declare data-lens (no lens registered)")
	}
}

// TestExplorer_D32dImpl4_DispatcherHandlesKnowledgeAsNoOp pins that
// setWorkbenchMode early-returns when called with 'knowledge'. This
// covers the case of a programmatic call or future deep-link without
// crashing.
func TestExplorer_D32dImpl4_DispatcherHandlesKnowledgeAsNoOp(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	dispatchIdx := strings.Index(body, "function setWorkbenchMode(mode) {")
	if dispatchIdx < 0 {
		t.Fatal("D32d-impl-4: setWorkbenchMode dispatcher missing")
	}
	// Bound the slice by the next `\n  }\n` close-brace.
	end := strings.Index(body[dispatchIdx:], "\n  }\n")
	if end <= 0 {
		end = 4096
	}
	dispatch := body[dispatchIdx : dispatchIdx+end]
	if !strings.Contains(dispatch, "if (mode === 'knowledge') return;") {
		t.Error("D32d-impl-4: setWorkbenchMode must early-return when mode === 'knowledge'")
	}
}

// TestExplorer_D32dImpl4_GraphViewMenuCSSServed pins that the new
// stylesheet ships, is linked from index.html, and declares the
// menu container + placeholder rules.
func TestExplorer_D32dImpl4_GraphViewMenuCSSServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/css/graph-view-menu.css", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("D32d-impl-4: graph-view-menu.css want 200, got %d", rec.Code)
	}
	css := rec.Body.String()
	for _, want := range []string{
		".graph-view-menu",
		".graph-view-menu-button",
		".graph-view-menu-button.is-placeholder",
		".graph-view-menu-button[disabled]",
		`[aria-disabled="true"]`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32d-impl-4: graph-view-menu.css must declare %q", want)
		}
	}
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `<link rel="stylesheet" href="/explorer/assets/css/graph-view-menu.css">`) {
		t.Error("D32d-impl-4: index.html must <link> graph-view-menu.css")
	}
}

// TestExplorer_D32dImpl4_HeaderMenuButtonsDispatchThroughSetWorkbenchMode
// pins that the header menu buttons reuse the existing setWorkbench
// Mode dispatcher (no new state machine introduced).
func TestExplorer_D32dImpl4_HeaderMenuButtonsDispatchThroughSetWorkbenchMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// The dispatcher must still pick up buttons by the .service-
	// workbench-mode[data-workbench-mode] selector. The selector lives
	// in the inline IIFE.
	if !strings.Contains(body, `.service-workbench-mode[data-workbench-mode]`) {
		t.Error("D32d-impl-4: setWorkbenchMode dispatcher must continue to select .service-workbench-mode[data-workbench-mode]")
	}
	// The menu buttons carry the .service-workbench-mode class so the
	// dispatcher picks them up. Pin the composition. D32b-impl-4
	// relocated the menu into .governance-map-toolbar-right, so the
	// stable end-marker is now the polite-live announce span.
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	end := strings.Index(body[menuIdx:], `class="gmap-view-mode-feedback"`)
	if end <= 0 {
		t.Fatal("D32b-impl-4: cannot locate end of graph view menu block")
	}
	block := body[menuIdx : menuIdx+end]
	if strings.Count(block, "service-workbench-mode service-workbench-icon-mode") < 4 {
		t.Error("D32b-impl-4: each menu button must carry both .service-workbench-mode and .service-workbench-icon-mode for dispatcher pickup + visual treatment")
	}
}

// TestExplorer_D32dImpl4_MIDASLogoGeometryConsistent pins the
// geometric concept shared between the sidebar logo mark and the
// favicon SVG. Both render four vertical white bars where bars 2 & 3
// are down-shifted by 2 px. The bar widths and gaps differ by 1 px
// per axis between the two assets (sidebar uses 5px bars / 4px gaps;
// favicon previously used 4px bars / 3px gaps inside a letterboxed
// 32×32 square — D32b-impl-4 unified both render contexts on the
// same canonical SVG asset (assets/img/midas-logo.svg), so the
// geometry is now byte-identical. This test pins the asset path,
// the favicon link, and the sidebar <img> source — a redesign that
// diverges the two renders fails loudly.
func TestExplorer_D32dImpl4_MIDASLogoGeometryConsistent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	html := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// D32b-impl-4 — both favicon and sidebar mark source the same
	// canonical SVG. The asset itself is served by the embedded FS.
	logoAssetPath := "/explorer/assets/img/midas-logo.svg"
	logoRec := performRequest(t, srv, http.MethodGet, logoAssetPath, nil)
	if logoRec.Code != http.StatusOK {
		t.Fatalf("D32b-impl-4: %s want 200, got %d", logoAssetPath, logoRec.Code)
	}
	logoSVG := logoRec.Body.String()
	for _, want := range []string{
		`viewBox="0 0 32 32"`,
		// Rounded 32×32 tile with dark fill.
		`rx="6"`,
		`fill="#05070D"`,
		// Four white bars.
		`width="4" height="20"`,
		// x positions 4 / 11 / 18 / 25.
		`x="4"`,
		`x="11"`,
		`x="18"`,
		`x="25"`,
		// y='6' for bars 1 & 4; y='8' for bars 2 & 3.
		`y="6"`,
		`y="8"`,
	} {
		if !strings.Contains(logoSVG, want) {
			t.Errorf("D32b-impl-4: canonical logo SVG must include %q", want)
		}
	}

	// Favicon now references the canonical asset (no data URI).
	if !strings.Contains(html, `<link rel="icon" type="image/svg+xml" href="`+logoAssetPath+`"`) {
		t.Error("D32b-impl-4: favicon <link> must reference the canonical SVG asset path")
	}
	if strings.Contains(html, `href="data:image/svg+xml,`) {
		t.Error("D32b-impl-4: legacy favicon data URI must be removed (canonical SVG is the single source of truth)")
	}

	// Sidebar mark is now an <img> sourcing the canonical SVG.
	if !strings.Contains(html, `class="midas-logo-mark"`) {
		t.Fatal("D32b-impl-4: .midas-logo-mark missing from sidebar")
	}
	if !strings.Contains(html, `<img class="midas-logo-mark" src="`+logoAssetPath+`"`) {
		t.Error("D32b-impl-4: sidebar .midas-logo-mark must be an <img> sourcing the canonical SVG asset")
	}
	// Negative pin — the legacy CSS-bar geometry (4 <span> bars +
	// translateY(2px) per-span rule) is gone now that the canonical
	// SVG bakes the geometry into a single asset.
	if strings.Contains(html, `class="midas-logo-mark" aria-hidden="true">
        <span>`) {
		t.Error("D32b-impl-4: legacy 4-span sidebar logo geometry must be removed (replaced by canonical SVG asset)")
	}
	// Accessibility — the <img> is decorative (sibling .shell-brand-
	// title carries the brand name), so it must declare empty alt or
	// aria-hidden.
	imgIdx := strings.Index(html, `<img class="midas-logo-mark"`)
	if imgIdx >= 0 {
		tagEnd := strings.Index(html[imgIdx:], ">")
		if tagEnd > 0 {
			imgTag := html[imgIdx : imgIdx+tagEnd+1]
			if !(strings.Contains(imgTag, `alt=""`) || strings.Contains(imgTag, `aria-hidden="true"`)) {
				t.Error("D32b-impl-4: sidebar logo <img> must declare alt=\"\" or aria-hidden=\"true\" (decorative; brand name is in .shell-brand-title)")
			}
		}
	}

	// Sidebar logo CSS — the .midas-logo-mark element is now an <img>,
	// so the rules size the asset and handle collapsed-sidebar state.
	// The old per-span / translateY rules are gone.
	css := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	if !strings.Contains(css, `.midas-logo-mark {`) {
		t.Error("D32b-impl-4: shell.css must declare .midas-logo-mark sizing")
	}
	if !strings.Contains(css, `body.sidebar-collapsed .midas-logo-mark`) {
		t.Error("D32b-impl-4: shell.css must declare collapsed-sidebar sizing for .midas-logo-mark")
	}
}

// ---------------------------------------------------------------------------
// D32b-impl-4 — Restore Workbench Mode Menu + Canonical MIDAS Logo.
//
// Tests in this section pin:
//
//   • The Workbench mode menu lives inside the workbench toolbar's
//     right group (.governance-map-toolbar-right) — focus-mode-
//     resilient, unlike the D32d-impl-4 shell-header location which
//     was hidden by `body.gmap-focus-mode .shell-header { display:
//     none; }`.
//   • The shell-header location is empty of mode buttons (regression
//     guard against another header migration).
//   • The MIDAS canonical SVG asset lives at the expected path, is
//     served from the embedded FS, and carries the four-bar geometry.
//   • Both the favicon and the sidebar mark source the same asset
//     (no duplicate inline SVGs).
//   • Knowledge Graph placeholder remains disabled and never reaches
//     a backend endpoint.
// ---------------------------------------------------------------------------

// TestExplorer_D32bImpl4_WorkbenchModeMenuVisibleInToolbar pins the
// menu's location, content, and accessibility in the workbench-
// toolbar right zone.
func TestExplorer_D32bImpl4_WorkbenchModeMenuVisibleInToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Anchor on the workbench toolbar's right group.
	toolbarRightIdx := strings.Index(body, `class="governance-map-toolbar-right"`)
	if toolbarRightIdx < 0 {
		t.Fatal("D32b-impl-4: .governance-map-toolbar-right missing (workbench toolbar right group)")
	}
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	if menuIdx < 0 {
		t.Fatal("D32b-impl-4: .graph-view-menu missing")
	}
	if menuIdx < toolbarRightIdx {
		t.Errorf("D32b-impl-4: .graph-view-menu must sit inside .governance-map-toolbar-right; menu=%d toolbarRight=%d", menuIdx, toolbarRightIdx)
	}
	// The polite-live announce span (.gmap-view-mode-feedback) is the
	// next sibling — pin its presence so the menu's natural end-marker
	// stays stable.
	feedbackIdx := strings.Index(body, `class="gmap-view-mode-feedback"`)
	if feedbackIdx < 0 {
		t.Fatal("D32b-impl-4: .gmap-view-mode-feedback announce span missing")
	}
	if !(menuIdx < feedbackIdx) {
		t.Errorf("D32b-impl-4: .graph-view-menu must precede .gmap-view-mode-feedback in reading order; menu=%d feedback=%d", menuIdx, feedbackIdx)
	}
}

// TestExplorer_D32bImpl4_MenuRemainsVisibleInFocusMode pins that
// `body.gmap-focus-mode` does NOT collapse the workbench toolbar
// (where the menu now lives). This is a CSS-level regression guard.
func TestExplorer_D32bImpl4_MenuRemainsVisibleInFocusMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	hideRuleIdx := strings.Index(css, "body.gmap-focus-mode .shell-header,")
	if hideRuleIdx < 0 {
		t.Fatal("D32b-impl-4: focus-mode hide rule on .shell-header must remain (D32b-impl-2 / D32b-impl-3 baseline)")
	}
	hideEnd := strings.Index(css[hideRuleIdx:], "}")
	hideRule := css[hideRuleIdx : hideRuleIdx+hideEnd]
	// The .governance-map-toolbar must NOT appear in the hide rule —
	// it's the menu host and must remain visible in focus mode.
	for _, banned := range []string{
		".governance-map-toolbar",
		".governance-map-toolbar-right",
		".graph-view-menu",
	} {
		if strings.Contains(hideRule, banned) {
			t.Errorf("D32b-impl-4: focus-mode hide rule must not collapse %q (workbench mode menu must remain visible)", banned)
		}
	}
}

// TestExplorer_D32bImpl4_ShellHeaderHasNoModeButtons pins that the
// menu does NOT live in .shell-header-right any more (a regression
// guard against re-introducing the D32d-impl-4 location).
func TestExplorer_D32bImpl4_ShellHeaderHasNoModeButtons(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	shellHeaderIdx := strings.Index(body, `<header class="shell-header"`)
	if shellHeaderIdx < 0 {
		t.Fatal("D32b-impl-4: <header class=\"shell-header\"> missing")
	}
	shellHeaderEnd := strings.Index(body[shellHeaderIdx:], `</header>`)
	if shellHeaderEnd <= 0 {
		t.Fatal("D32b-impl-4: shell-header </header> close tag missing")
	}
	headerSlice := body[shellHeaderIdx : shellHeaderIdx+shellHeaderEnd]
	for _, banned := range []string{
		`class="graph-view-menu"`,
		`data-workbench-mode=`,
	} {
		if strings.Contains(headerSlice, banned) {
			t.Errorf("D32b-impl-4: <header class=\"shell-header\"> must not contain %q (focus-mode CSS hides the shell header)", banned)
		}
	}
}

// TestExplorer_D32bImpl4_CanonicalLogoAssetServed pins that the new
// SVG asset is served from the embedded FS with the documented
// 4-bar geometry on a rounded-tile background.
func TestExplorer_D32bImpl4_CanonicalLogoAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/img/midas-logo.svg", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("D32b-impl-4: canonical logo SVG want 200, got %d", rec.Code)
	}
	svg := rec.Body.String()
	for _, want := range []string{
		`<svg`,
		`viewBox="0 0 32 32"`,
		// Rounded-tile background (#05070D, rx=6).
		`rx="6"`,
		`fill="#05070D"`,
		// 4 white bars, 4×20, with bars 2 & 3 down-shifted by 2 px.
		`width="4" height="20"`,
		`x="4"`,
		`x="11"`,
		`x="18"`,
		`x="25"`,
		`y="6"`,
		`y="8"`,
		`fill="#FFFFFF"`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("D32b-impl-4: canonical logo SVG must include %q", want)
		}
	}
	// Exactly 4 white bar rects.
	whiteBars := strings.Count(svg, `fill="#FFFFFF"`)
	if whiteBars != 4 {
		t.Errorf("D32b-impl-4: canonical logo SVG must contain exactly 4 white bars; got %d", whiteBars)
	}
}

// TestExplorer_D32bImpl4_LogoSingleSourceOfTruth pins that the
// sidebar mark and the favicon both reference the canonical asset
// rather than duplicating the SVG inline.
func TestExplorer_D32bImpl4_LogoSingleSourceOfTruth(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// Sidebar logo is an <img> sourcing the canonical asset.
	if !strings.Contains(body, `<img class="midas-logo-mark" src="/explorer/assets/img/midas-logo.svg"`) {
		t.Error("D32b-impl-4: sidebar .midas-logo-mark must be an <img src> pointing at the canonical SVG asset")
	}
	// Favicon references the canonical asset (not a data URI).
	if !strings.Contains(body, `<link rel="icon" type="image/svg+xml" href="/explorer/assets/img/midas-logo.svg"`) {
		t.Error("D32b-impl-4: favicon must reference the canonical SVG asset path")
	}
	if strings.Contains(body, `href="data:image/svg+xml,`) {
		t.Error("D32b-impl-4: legacy favicon data URI must be removed (canonical SVG is the single source of truth)")
	}
	// Negative pin — the legacy 4-span sidebar geometry is gone.
	if strings.Contains(body, `<span></span><span></span><span></span><span></span>`) {
		t.Error("D32b-impl-4: legacy 4-span sidebar logo geometry must be removed")
	}
	// Negative pin — no second inline SVG copy of the logo geometry.
	// The asset itself contains the geometry; index.html must not
	// re-inline it.
	if strings.Contains(body, `<rect width="32" height="32" rx="6" fill="#05070D"/>`) {
		t.Error("D32b-impl-4: index.html must not inline the canonical SVG (single source of truth = the asset file)")
	}
}

// TestExplorer_D32bImpl4_LogoAccessibility pins the decorative-image
// contract: the sidebar <img> declares alt="" (or aria-hidden="true")
// so assistive tech does not double-announce the brand (the sibling
// .shell-brand-title carries the brand name).
func TestExplorer_D32bImpl4_LogoAccessibility(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	imgIdx := strings.Index(body, `<img class="midas-logo-mark"`)
	if imgIdx < 0 {
		t.Fatal("D32b-impl-4: sidebar logo <img> missing")
	}
	tagEnd := strings.Index(body[imgIdx:], ">")
	if tagEnd <= 0 {
		t.Fatal("D32b-impl-4: sidebar logo <img> tag has no closing >")
	}
	imgTag := body[imgIdx : imgIdx+tagEnd+1]
	hasEmptyAlt := strings.Contains(imgTag, `alt=""`)
	hasAriaHidden := strings.Contains(imgTag, `aria-hidden="true"`)
	if !(hasEmptyAlt && hasAriaHidden) {
		t.Errorf("D32b-impl-4: sidebar logo <img> must declare BOTH alt=\"\" and aria-hidden=\"true\" (decorative; brand name in .shell-brand-title); got tag=%q", imgTag)
	}
	// Brand name still rendered by the sibling text element.
	if !strings.Contains(body, `class="shell-brand-title">MIDAS Explorer<`) {
		t.Error("D32b-impl-4: .shell-brand-title must continue to carry the visible brand text \"MIDAS Explorer\"")
	}
}

// TestExplorer_D32bImpl4_LogoCollapsedSidebarSizing pins that
// shell.css declares a smaller-bounding-box sizing rule for the
// collapsed sidebar so the mark continues to look correct when the
// brand row narrows.
func TestExplorer_D32bImpl4_LogoCollapsedSidebarSizing(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	// Default size (expanded sidebar).
	if !strings.Contains(css, `.midas-logo-mark {`) {
		t.Error("D32b-impl-4: shell.css must declare .midas-logo-mark default sizing")
	}
	if !strings.Contains(css, `width: 28px;`) {
		t.Error("D32b-impl-4: .midas-logo-mark must declare width: 28px (default sidebar sizing)")
	}
	// Collapsed-sidebar override.
	if !strings.Contains(css, `body.sidebar-collapsed .midas-logo-mark {`) {
		t.Error("D32b-impl-4: shell.css must declare collapsed-sidebar override for .midas-logo-mark")
	}
}

// TestExplorer_D32bImpl4_KnowledgePlaceholderNoBackendRoute confirms
// that the Knowledge Graph placeholder does not introduce any
// /v1/graphs/knowledge URL string anywhere in the served HTML or any
// of the JS modules — i.e. no backend route is exercised even
// accidentally.
func TestExplorer_D32bImpl4_KnowledgePlaceholderNoBackendRoute(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, banned := range []string{
		"/v1/graphs/knowledge",
		"/v1/knowledge-graph",
		"#graph/knowledge",
		"ExplorerAPI.graphs.knowledge",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D32b-impl-4: Knowledge Graph placeholder must not reference %q", banned)
		}
	}
	// The same negative pin extended across every module the
	// conceptual JS surface includes. (The surface is the same one
	// already concatenated by other contract tests via
	// getExplorerAllJS — but this test is intentionally narrower:
	// the placeholder is in inline HTML, not in any module.)
	js := getExplorerAllJS(t, srv)
	for _, banned := range []string{
		"/v1/graphs/knowledge",
		"ExplorerAPI.graphs.knowledge",
		"api.knowledge(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32b-impl-4: no JS module may reference Knowledge Graph endpoints (%q)", banned)
		}
	}
}

// ---------------------------------------------------------------------------
// D32b-debug-1 — Context Graph and Authority Graph rendering same view.
//
// Operator-reported regression on Azure revision midas-api--0000036:
// clicking Authority Graph and Context Graph showed the same view.
// Root cause is a race in the inline workbench mode dispatcher: the
// Authority branch of setWorkbenchMode invokes MIDASExplorerServices
// .showMap(serviceId), which through the _hooks.setGmapMode('map') +
// _hooks.refreshGovernanceMap chain unconditionally triggers the
// inline refreshGovernanceMap() — hard-coded to lens:'context' +
// contextView.renderContextGraph. The Authority view's parallel
// fetch+render then races for the shared #gmap-canvas; whichever
// response arrives LAST paints. So the operator sees Context content
// even when they clicked Authority.
//
// The fix has three parts:
//
//   1. refreshGovernanceMap reads selectedGraphLens from the store
//      and early-returns when the active lens is not 'context'.
//   2. setWorkbenchMode('authority') pre-seeds selectedGraphLens =
//      'authority' in the store BEFORE calling showMap, so the lens
//      guard in (1) sees the right lens during showMap-triggered
//      refreshGovernanceMap.
//   3. authority-graph-view.js's renderAuthorityGraph applies the
//      same active-lens guard before painting so a late-arriving
//      Authority response cannot clobber a Context canvas (reverse
//      race when the operator switches Authority → Context).
// ---------------------------------------------------------------------------

// TestExplorer_D32bDebug1_RefreshGovernanceMapHasLensGuard pins
// the inline Context refresh is gated by the store's
// selectedGraphLens. Without this, refreshGovernanceMap fires a
// Context fetch+render anywhere showMap is called, regardless of
// the operator's mode.
func TestExplorer_D32bDebug1_RefreshGovernanceMapHasLensGuard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	startIdx := strings.Index(body, "function refreshGovernanceMap()")
	if startIdx < 0 {
		t.Fatal("D32b-debug-1: refreshGovernanceMap function missing")
	}
	endRel := strings.Index(body[startIdx:], "\n  }\n")
	if endRel <= 0 {
		t.Fatal("D32b-debug-1: refreshGovernanceMap body has no closing brace")
	}
	fnBody := body[startIdx : startIdx+endRel]
	if !strings.Contains(fnBody, "selectedGraphLens") {
		t.Error("D32b-debug-1: refreshGovernanceMap must read selectedGraphLens (Context-only refresh must skip when active lens is not 'context')")
	}
	if !strings.Contains(fnBody, "MIDASExplorerStore.getState") {
		t.Error("D32b-debug-1: refreshGovernanceMap must read MIDASExplorerStore.getState() for the lens-guard check")
	}
	if !strings.Contains(fnBody, "activeLens !== 'context'") {
		t.Error("D32b-debug-1: refreshGovernanceMap must early-return when activeLens !== 'context'")
	}
}

// TestExplorer_D32bDebug1_SetWorkbenchModeAuthorityPreSeedsLens pins
// the second half of the fix: setWorkbenchMode('authority') writes
// selectedGraphLens='authority' to the store BEFORE showMap.
func TestExplorer_D32bDebug1_SetWorkbenchModeAuthorityPreSeedsLens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	branchIdx := strings.Index(body, "if (mode === 'authority') {")
	if branchIdx < 0 {
		t.Fatal("D32b-debug-1: Authority branch of setWorkbenchMode missing")
	}
	endRel := strings.Index(body[branchIdx:], "\n    }\n")
	if endRel <= 0 {
		t.Fatal("D32b-debug-1: Authority branch body has no closing brace")
	}
	branchBody := body[branchIdx : branchIdx+endRel]
	seedIdx := strings.Index(branchBody, "MIDASExplorerStore.setState({ selectedGraphLens: 'authority' })")
	showMapIdx := strings.Index(branchBody, "MIDASExplorerServices.showMap(serviceId)")
	if seedIdx < 0 {
		t.Error("D32b-debug-1: Authority branch must pre-seed MIDASExplorerStore.setState({ selectedGraphLens: 'authority' })")
	}
	if showMapIdx < 0 {
		t.Fatal("D32b-debug-1: Authority branch must continue to call MIDASExplorerServices.showMap(serviceId)")
	}
	if !(seedIdx < showMapIdx) {
		t.Errorf("D32b-debug-1: store pre-seed (offset %d) must precede showMap (offset %d) in setWorkbenchMode('authority') — otherwise refreshGovernanceMap's lens guard reads a stale 'context' lens and the Context fetch fires anyway",
			seedIdx, showMapIdx)
	}
}

// TestExplorer_D32bDebug1_AuthorityRenderHasLensGuard pins the
// reverse-race guard: a late-arriving Authority fetch must not
// clobber a freshly-rendered Context canvas.
func TestExplorer_D32bDebug1_AuthorityRenderHasLensGuard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	startIdx := strings.Index(js, "function renderAuthorityGraph(payload, ctx) {")
	if startIdx < 0 {
		t.Fatal("D32b-debug-1: renderAuthorityGraph function missing")
	}
	// Restrict the slice to the first ~1.5 KB so the guard is pinned
	// at the TOP of the function, not buried below an early return
	// path that would miss a fast-path render.
	end := len(js)
	if startIdx+1500 < end {
		end = startIdx + 1500
	}
	fnBody := js[startIdx:end]
	if !strings.Contains(fnBody, "selectedGraphLens") {
		t.Error("D32b-debug-1: renderAuthorityGraph must read selectedGraphLens (active-lens guard against reverse-direction race)")
	}
	if !strings.Contains(fnBody, "MIDASExplorerStore") {
		t.Error("D32b-debug-1: renderAuthorityGraph must consult MIDASExplorerStore for the active lens")
	}
	if !strings.Contains(fnBody, "activeLens !== 'authority'") {
		t.Error("D32b-debug-1: renderAuthorityGraph must early-return when activeLens !== 'authority'")
	}
}

// TestExplorer_D32bDebug1_LensDispatchEndpointsRemainDistinct
// pins the canonical pre-condition: API client paths must remain
// distinct (regression guard against a future copy-paste collapse).
func TestExplorer_D32bDebug1_LensDispatchEndpointsRemainDistinct(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/core/api-client.js")
	if !strings.Contains(js, "request('/v1/graphs/context' + q)") {
		t.Error("D32b-debug-1: ExplorerAPI.graphs.context must call /v1/graphs/context")
	}
	if !strings.Contains(js, "request('/v1/graphs/authority' + q)") {
		t.Error("D32b-debug-1: ExplorerAPI.graphs.authority must call /v1/graphs/authority")
	}
	contextFnIdx := strings.Index(js, "context: function (params)")
	authorityFnIdx := strings.Index(js, "authority: function (params)")
	if contextFnIdx < 0 || authorityFnIdx < 0 {
		t.Fatal("D32b-debug-1: ExplorerAPI.graphs.{context,authority} declarations missing")
	}
	contextBody := js[contextFnIdx:]
	if ctxEnd := strings.Index(contextBody, "},"); ctxEnd > 0 {
		contextBody = contextBody[:ctxEnd]
	}
	if strings.Contains(contextBody, "/v1/graphs/authority") {
		t.Error("D32b-debug-1: graphs.context must NOT reference /v1/graphs/authority")
	}
	authorityBody := js[authorityFnIdx:]
	if authEnd := strings.Index(authorityBody, "},"); authEnd > 0 {
		authorityBody = authorityBody[:authEnd]
	}
	if strings.Contains(authorityBody, "/v1/graphs/context") {
		t.Error("D32b-debug-1: graphs.authority must NOT reference /v1/graphs/context")
	}
}

// TestExplorer_D32bDebug1_AuthorityViewDoesNotCallContextRenderer
// pins that the Authority view paints via its own renderer. A
// regression that pointed Authority at the Context renderer would
// surface the same "same view" symptom even after the race fix.
func TestExplorer_D32bDebug1_AuthorityViewDoesNotCallContextRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, banned := range []string{
		"contextView.renderContextGraph",
		"contextAdapter.mapToCardLayout",
		"contextAdapter.fetch",
		"ExplorerAPI.graphs.context(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32b-debug-1: authority-graph-view.js must NOT call %q (would reduce Authority to Context content)", banned)
		}
	}
}

// TestExplorer_D32bDebug1_ContextRefreshIsContextOnly pins the
// complementary guarantee: the inline Context refresh dispatches to
// the Context lens only. A regression that pointed it at lens:
// 'authority' would surface the same operator-reported bug from the
// opposite direction.
func TestExplorer_D32bDebug1_ContextRefreshIsContextOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	startIdx := strings.Index(body, "function refreshGovernanceMap()")
	if startIdx < 0 {
		t.Fatal("D32b-debug-1: refreshGovernanceMap missing")
	}
	endRel := strings.Index(body[startIdx:], "\n  }\n")
	fnBody := body[startIdx : startIdx+endRel]
	if !strings.Contains(fnBody, "ExplorerGraph.shell.refresh({ lens: 'context'") {
		t.Error("D32b-debug-1: refreshGovernanceMap must dispatch lens:'context' on shell.refresh")
	}
	if !strings.Contains(fnBody, "ExplorerGraph.contextView.renderContextGraph") {
		t.Error("D32b-debug-1: refreshGovernanceMap must render via ExplorerGraph.contextView.renderContextGraph")
	}
	for _, banned := range []string{
		"ExplorerGraph.authorityView.renderAuthorityGraph",
		"authorityAdapter.fetch",
		"ExplorerAPI.graphs.authority(",
		"lens: 'authority'",
	} {
		if strings.Contains(fnBody, banned) {
			t.Errorf("D32b-debug-1: refreshGovernanceMap must NOT reference %q (Context-only refresh)", banned)
		}
	}
}
