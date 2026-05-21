package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37o-fix-1 — Context Projection Provider tests
//
// Pins the contract for the new
// `context-projection-provider.js` module that closes the strategic
// renderer's missing-projection gap diagnosed in D37o-fix-1-assess:
//
//   • asset presence + load order;
//   • public surface (init / destroy / refresh / isAvailable /
//     getLastPublishMeta / _constants);
//   • acquires projection via the existing context-graph-adapter
//     (no raw `/v1/graphs/context` coupling);
//   • publishes through `contextProjection.publishProjection(...)`
//     with the required metadata fields;
//   • is renderer-independent (no viewport.activateById, no
//     renderer-mount DOM, no `data-active-renderer` writes);
//   • does not render or read DOM (no document.createElement /
//     appendChild / innerHTML / querySelector / getElementById);
//   • does not import / call drawer setters, evidence tray, Authority
//     modules, or the dormant Cytoscape spike;
//   • dedupes in-flight requests;
//   • reasserts that the strategic renderer remains fetch-free and
//     the projection handoff remains passive;
//   • leaves the legacy renderer registration, drawer, tray,
//     Authority canvas-edge wrapper, selected-object pane, and
//     default renderer behaviour untouched.
//
// Test-name prefix is `D37oFix1` so it runs under the focused
// `-run 'D37oFix1|D37oImpl6'` validation filter.

const (
	d37oFix1ProviderAsset    = "/explorer/assets/js/graph/context/context-projection-provider.js"
	d37oFix1AdapterAsset     = "/explorer/assets/js/graph/context/context-graph-adapter.js"
	d37oFix1HandoffAsset     = "/explorer/assets/js/graph/context/context-projection-handoff.js"
	d37oFix1RendererAsset    = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37oFix1LegacyViewAsset  = "/explorer/assets/js/graph/context/context-graph-view.js"
	d37oFix1SpikeAsset       = "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js"
	d37oFix1PaneAsset        = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37oFix1EvidenceTrayAst  = "/explorer/assets/js/graph/context/context-evidence-tray.js"
	d37oFix1AuthorityCanvasA = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
)

// ── A. Asset presence and load order ─────────────────────────────────

func TestExplorer_D37oFix1_ProviderJsAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)
	if len(js) == 0 {
		t.Fatal("D37o-fix-1: context-projection-provider.js must be served")
	}
}

func TestExplorer_D37oFix1_ProviderScriptTagPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-projection-provider.js"`) {
		t.Errorf("D37o-fix-1: index.html must include <script> for context-projection-provider.js")
	}
}

// TestExplorer_D37oFix1_ProviderLoadsAfterAdapterAndHandoff pins that
// the provider script is loaded strictly after both the
// context-graph-adapter (whose `contextAdapter.fetch` it calls) and
// the projection handoff (whose `publishProjection` it calls).
func TestExplorer_D37oFix1_ProviderLoadsAfterAdapterAndHandoff(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	adapterIdx  := strings.Index(body, "context-graph-adapter.js")
	handoffIdx  := strings.Index(body, "context-projection-handoff.js")
	providerIdx := strings.Index(body, "context-projection-provider.js")
	if adapterIdx < 0 || handoffIdx < 0 || providerIdx < 0 {
		t.Fatal("D37o-fix-1: adapter, handoff, and provider scripts must all appear in index.html")
	}
	if adapterIdx >= providerIdx {
		t.Errorf("D37o-fix-1: provider must load AFTER context-graph-adapter.js (adapter idx=%d, provider idx=%d)", adapterIdx, providerIdx)
	}
	if handoffIdx >= providerIdx {
		t.Errorf("D37o-fix-1: provider must load AFTER context-projection-handoff.js (handoff idx=%d, provider idx=%d)", handoffIdx, providerIdx)
	}
}

// TestExplorer_D37oFix1_ProviderLoadsBeforeStrategicRenderer pins that
// the provider is registered before the strategic Context renderer
// activates, so the renderer's first paint can already see a
// published projection if state is resolvable.
func TestExplorer_D37oFix1_ProviderLoadsBeforeStrategicRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	providerIdx := strings.Index(body, "context-projection-provider.js")
	rendererIdx := strings.Index(body, "context-cytoscape-renderer.js")
	if providerIdx < 0 || rendererIdx < 0 {
		t.Fatal("D37o-fix-1: provider and strategic renderer scripts must both appear in index.html")
	}
	if providerIdx >= rendererIdx {
		t.Errorf("D37o-fix-1: provider must load BEFORE context-cytoscape-renderer.js (provider idx=%d, renderer idx=%d)", providerIdx, rendererIdx)
	}
}

// ── B. Public surface ────────────────────────────────────────────────

func TestExplorer_D37oFix1_PublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.contextProjectionProvider",
		"init:",
		"destroy:",
		"refresh:",
		"isAvailable:",
		"getLastPublishMeta:",
		"_constants:",
		"PROVIDER_ID:",
		"LENS_ID:",
		"DEFAULT_DEPTH:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-fix-1: provider must export %q on its public surface", want)
		}
	}
}

// TestExplorer_D37oFix1_ConstantsLockedValues pins the canonical
// identity strings. PROVIDER_ID identifies the producer source in
// metadata; LENS_ID is the canonical Context lens identity (no
// rollout-mode words); DEFAULT_DEPTH matches the existing inline
// `refreshGovernanceMap` depth.
func TestExplorer_D37oFix1_ConstantsLockedValues(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, want := range []string{
		"var PROVIDER_ID   = 'context-projection-provider';",
		"var LENS_ID       = 'context';",
		"var DEFAULT_DEPTH = 5;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-fix-1: provider must declare locked constant %q", want)
		}
	}
}

// ── C. Provider publishes through the handoff ────────────────────────

func TestExplorer_D37oFix1_PublishesThroughHandoff(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	if !strings.Contains(js, "h.publishProjection(") {
		t.Errorf("D37o-fix-1: provider must invoke contextProjection.publishProjection(...)")
	}
}

func TestExplorer_D37oFix1_PublishMetaShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, want := range []string{
		"source:    PROVIDER_ID,",
		"view:      params.view,",
		"rootId:    params.id,",
		"depth:     params.depth,",
		"fetchedAt:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-fix-1: provider publish meta must include %q", want)
		}
	}
}

// ── D. Acquisition uses the existing adapter ────────────────────────

func TestExplorer_D37oFix1_AcquiresViaContextAdapter(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	if !strings.Contains(js, "g.contextAdapter") {
		t.Errorf("D37o-fix-1: provider must acquire projection via contextAdapter")
	}
	if !strings.Contains(js, "a.fetch(") && !strings.Contains(js, "a.fetch ") {
		t.Errorf("D37o-fix-1: provider must call the adapter's fetch(...) helper")
	}
}

// TestExplorer_D37oFix1_NoRawApiCoupling pins that the provider does
// not bypass the adapter by talking to the API client directly or
// hitting the /v1/graphs/context endpoint with a raw fetch.
func TestExplorer_D37oFix1_NoRawApiCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, banned := range []string{
		"/v1/graphs/context",
		"MIDASExplorerAPI.graphs.context",
		"XMLHttpRequest",
		"new Image(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: provider must not contain raw backend coupling %q", banned)
		}
	}
	// `fetch(` is a stricter check: the provider should call the
	// adapter's wrapped `fetch` method, never the global `fetch()`.
	if strings.Contains(js, "window.fetch(") {
		t.Errorf("D37o-fix-1: provider must not call window.fetch directly")
	}
}

// ── E. Renderer-independence ─────────────────────────────────────────

func TestExplorer_D37oFix1_ProviderIsRendererIndependent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, banned := range []string{
		"viewport.register",
		"activateById",
		"data-active-renderer",
		"context-renderer-mount",
		"context-renderer-canvas",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: provider must not contain renderer-coupling token %q", banned)
		}
	}
}

// ── F. No DOM ownership ──────────────────────────────────────────────

func TestExplorer_D37oFix1_ProviderDoesNotTouchDOM(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, banned := range []string{
		"document.createElement",
		"appendChild",
		"innerHTML",
		".querySelector",
		"getElementById",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: provider must not perform DOM operation %q", banned)
		}
	}
}

// ── G. No drawer / tray / Authority coupling ─────────────────────────

func TestExplorer_D37oFix1_NoDrawerOrTrayOrAuthorityCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	// Drawer setters: substring matches like `setName(` would be too
	// loose because Date.now() is benign — use explicit "setName(" /
	// "setFields(" / etc. signatures.
	for _, banned := range []string{
		".setName(",
		".setFields(",
		".setGovernance(",
		".setActions(",
		".setInlineActions(",
		"contextEvidenceTray",
		"contextInspector",
		"authorityWorkbench",
		"authority-canvas-edge",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: provider must not couple to %q", banned)
		}
	}
}

// ── H. No spike import ───────────────────────────────────────────────

func TestExplorer_D37oFix1_NoSpikeImport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, banned := range []string{
		"context-cytoscape-overlay-spike",
		"contextHtmlCards",
		"cytoscape=1",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: provider must not depend on the dormant spike (%q)", banned)
		}
	}
}

// ── I. Naming discipline (no durable temporary identities) ───────────

func TestExplorer_D37oFix1_NoTemporaryRendererIdentities(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: provider must not introduce temporary identity %q", banned)
		}
	}
}

// ── J. Dedup + lifecycle ─────────────────────────────────────────────

func TestExplorer_D37oFix1_InFlightDedupPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, want := range []string{
		"_inflightKey",
		"_inflightToken",
		"_lastKey",
		"_keyOf",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-fix-1: provider must track %q for dedup discipline", want)
		}
	}
}

func TestExplorer_D37oFix1_StoreSubscriptionAndDestroy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	if !strings.Contains(js, "MIDASExplorerStore") {
		t.Errorf("D37o-fix-1: provider must observe state via MIDASExplorerStore")
	}
	if !strings.Contains(js, "s.subscribe(") {
		t.Errorf("D37o-fix-1: provider must subscribe to store changes")
	}
	if !strings.Contains(js, "_storeUnsubscribe") {
		t.Errorf("D37o-fix-1: provider must keep an unsubscribe handle for destroy()")
	}
}

func TestExplorer_D37oFix1_LensGatingPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	if !strings.Contains(js, "_isContextLensActive") {
		t.Errorf("D37o-fix-1: provider must gate fetches on the Context lens being active")
	}
	if !strings.Contains(js, "selectedGraphLens") {
		t.Errorf("D37o-fix-1: provider must consult the store's selectedGraphLens slot")
	}
}

func TestExplorer_D37oFix1_BootstrapOnDomReady(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	if !strings.Contains(js, "DOMContentLoaded") {
		t.Errorf("D37o-fix-1: provider must bootstrap on DOMContentLoaded")
	}
	if !strings.Contains(js, "document.readyState === 'loading'") {
		t.Errorf("D37o-fix-1: provider must check document.readyState before adding the bootstrap listener")
	}
}

// TestExplorer_D37oFix1_FailedFetchDoesNotClear pins the design rule
// that a transient fetch failure must not blank consumers' last-known
// projection — i.e. the provider must NOT call
// `contextProjection.clear()` on the error path.
func TestExplorer_D37oFix1_FailedFetchDoesNotClear(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	if strings.Contains(js, "contextProjection.clear(") {
		t.Errorf("D37o-fix-1: provider must not call contextProjection.clear() — failed fetch must leave the previous projection intact")
	}
	if strings.Contains(js, "h.clear(") {
		t.Errorf("D37o-fix-1: provider must not call the handoff's clear() helper")
	}
}

// ── K. Strategic renderer remains fetch-free ─────────────────────────

func TestExplorer_D37oFix1_StrategicRendererRemainsFetchFree(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1RendererAsset)

	for _, banned := range []string{
		"window.fetch(",
		"XMLHttpRequest",
		"/v1/graphs/context",
		"MIDASExplorerAPI.graphs.context",
		"contextAdapter.fetch(",
		"shell.refresh(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: strategic renderer must remain fetch-free; found %q", banned)
		}
	}
}

// ── L. Handoff remains passive ───────────────────────────────────────

func TestExplorer_D37oFix1_HandoffRemainsPassive(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1HandoffAsset)

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"document.createElement",
		"appendChild",
		"querySelector",
		"getElementById",
		"contextAdapter",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: projection handoff must remain passive; found %q", banned)
		}
	}
}

// ── M. Foundation preservation ───────────────────────────────────────
//
// Reassert that this tranche leaves the legacy renderer registration,
// the drawer, the evidence tray, the Authority canvas-edge wrapper,
// the selected-object pane, and the strategic renderer identity
// unchanged.

// TestExplorer_D37oFix1_LegacyContextEntryPointPreserved confirms
// that the live legacy Context lens entry points remain intact after
// D37p-clean-1 retired the dead `renderer.register('context', lensImpl)`
// dispatcher path and the unreachable `_publishToProjectionHandoff`
// helper. The live producer for Context projections is the
// `contextProjectionProvider` from D37o-fix-1; the legacy view's role
// is now exclusively to host `renderContextGraph` and its empty / error
// variants for the production refresh path.
func TestExplorer_D37oFix1_LegacyContextEntryPointPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1LegacyViewAsset)

	if !strings.Contains(js, "window.MIDASExplorerGraph.contextView") {
		t.Errorf("D37o-fix-1: legacy context-graph-view.js must still expose contextView entry points")
	}
	if !strings.Contains(js, "renderContextGraph") {
		t.Errorf("D37o-fix-1: legacy context-graph-view.js must still expose renderContextGraph")
	}
	// D37p-clean-1 — the dead publish hook + lensImpl are gone. The
	// live producer is contextProjectionProvider (separate file).
	if strings.Contains(js, `window.MIDASExplorerGraph.renderer.register('context', lensImpl)`) {
		t.Errorf("D37p-clean-1: dead renderer.register('context', lensImpl) call must be removed from context-graph-view.js")
	}
	// Dead helper definition + call must be gone; explanatory
	// comments referencing the historical name are allowed.
	if regexp.MustCompile(`function\s+_publishToProjectionHandoff\s*\(`).MatchString(js) {
		t.Errorf("D37p-clean-1: dead _publishToProjectionHandoff function must be removed from context-graph-view.js")
	}
	if strings.Contains(js, "_publishToProjectionHandoff(payload, ctx)") {
		t.Errorf("D37p-clean-1: dead _publishToProjectionHandoff call must be removed from context-graph-view.js")
	}
}

func TestExplorer_D37oFix1_PaneAndEvidenceTrayAndAuthorityUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Selected-object pane still served.
	pane := getExplorerAsset(t, srv, d37oFix1PaneAsset)
	if len(pane) == 0 {
		t.Errorf("D37o-fix-1: context-selected-object-pane.js must remain served")
	}

	// Evidence tray still served.
	tray := getExplorerAsset(t, srv, d37oFix1EvidenceTrayAst)
	if len(tray) == 0 {
		t.Errorf("D37o-fix-1: context-evidence-tray.js must remain served")
	}

	// Authority canvas-edge wrapper still served.
	auth := getExplorerAsset(t, srv, d37oFix1AuthorityCanvasA)
	if len(auth) == 0 {
		t.Errorf("D37o-fix-1: authority-canvas-edge-tabs.js must remain served")
	}

	// Dormant spike still served (preservation discipline).
	spike := getExplorerAsset(t, srv, d37oFix1SpikeAsset)
	if len(spike) == 0 {
		t.Errorf("D37o-fix-1: dormant spike module must remain served")
	}
}

func TestExplorer_D37oFix1_StrategicRendererIdentityPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1RendererAsset)

	if !strings.Contains(js, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37o-fix-1: strategic renderer must continue to use canonical id 'context'")
	}
	if !strings.Contains(js, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37o-fix-1: strategic renderer activation query param must remain 'contextRenderer'")
	}
}

// TestExplorer_D37oFix1_NoBackendOrSchemaChange is a coarse
// belt-and-braces pin: ensure no OpenAPI / schema / generated file
// references appear in the provider source.
func TestExplorer_D37oFix1_NoBackendOrSchemaChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oFix1ProviderAsset)

	for _, banned := range []string{
		"openapi.yaml",
		"schema.sql",
		"generated/",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-fix-1: provider must not reference backend artefact %q", banned)
		}
	}
}
