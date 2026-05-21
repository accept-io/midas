package httpapi

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35h-graphviewport-module-contract-documentation
//
// Documents the GraphViewport platform module as a durable internal
// architecture contract. This is a strategic platform-module
// tranche, NOT feature work and NOT tactical patching.
//
// Primary deliverable: docs/design/midas-graph-viewport.md describing
//   purpose, distributed/service-based direction, host & renderer
//   responsibilities, DOM contract, renderer factory contract,
//   renderer context contract, registry, activation lifecycle,
//   baseline adoption, renderer identity, safe area, resize,
//   clipping, teardown, testing discipline, anti-patterns, and a
//   future renderer integration checklist.
//
// Descriptor hardening: deferred. The brief explicitly permits
// deferral when documenting the descriptor model as a future option
// is sufficient. The current `register(rendererId, factory)` API is
// small, well-tested, and adequate for the renderers in flight; a
// descriptor model only earns its weight when richer queries (by
// kind / engine / capabilities) become needed. The architecture
// document captures the deferred descriptor sketch in Appendix A.
//
// Tests below pin:
//   • The architecture document exists.
//   • The document covers each required section (1–18 of the brief).
//   • graph-viewport.js references the architecture document.
//   • D35g registry behaviour is preserved.
//   • D35f renderer identity and clipping contracts are preserved.
//   • No legacy activation / fallback pattern returns.
//   • No new renderer or graph-domain feature is introduced.

const d35hContractDocPath = "../../docs/design/midas-graph-viewport.md"

// readContractDoc reads the GraphViewport architecture document from
// the canonical D35h path. Fatal if the file is missing — the
// document IS the primary D35h deliverable.
func readContractDoc(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(d35hContractDocPath)
	if err != nil {
		t.Fatalf("D35h: cannot read GraphViewport architecture document at %s: %v", d35hContractDocPath, err)
	}
	return string(body)
}

// TestExplorer_D35hDocs_GraphViewportContractDocumentExists pins
// that the architecture document is on disk at the canonical path.
func TestExplorer_D35hDocs_GraphViewportContractDocumentExists(t *testing.T) {
	info, err := os.Stat(d35hContractDocPath)
	if err != nil {
		t.Fatalf("D35h: GraphViewport architecture document must exist at %s: %v", d35hContractDocPath, err)
	}
	if info.IsDir() {
		t.Fatalf("D35h: %s must be a file, not a directory", d35hContractDocPath)
	}
	// Sanity: not empty.
	if info.Size() < 1024 {
		t.Errorf("D35h: GraphViewport architecture document is suspiciously small (%d bytes); expected a comprehensive contract", info.Size())
	}
}

// TestExplorer_D35hDocs_DocumentsStrategicPlatformRole pins that
// the document establishes the GraphViewport as a reusable
// strategic platform module — not a Cytoscape workaround, not an
// Explorer-specific patch.
func TestExplorer_D35hDocs_DocumentsStrategicPlatformRole(t *testing.T) {
	doc := readContractDoc(t)
	for _, want := range []string{
		"strategic platform module",
		"reusable",
		"not a Cytoscape workaround",
		"not an Explorer-specific patch",
		// Purpose section is explicit.
		"Purpose and platform role",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35h: architecture document must describe strategic platform role — missing %q", want)
		}
	}
}

// TestExplorer_D35hDocs_DocumentsServiceBasedDirection pins that
// the document explicitly aligns with MIDAS's modular, distributed,
// service-based architecture direction, and names the future graph
// domains the host is meant to support.
func TestExplorer_D35hDocs_DocumentsServiceBasedDirection(t *testing.T) {
	doc := readContractDoc(t)
	for _, want := range []string{
		// Strategic framing.
		"modular, distributed, service-based architecture",
		// Future graph domains expected to integrate via the host.
		"Knowledge Graph",
		"drift",
		"resilience",
		"evidence",
		"policy",
		"service-topology",
		// Plug-in renderer modules, not bespoke viewports.
		"registered renderer modules",
		"MUST NOT create their own viewport abstractions",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35h: architecture document must document service-based direction — missing %q", want)
		}
	}
}

// TestExplorer_D35hDocs_DocumentsHostAndRendererResponsibilities
// pins the host vs renderer ownership split.
func TestExplorer_D35hDocs_DocumentsHostAndRendererResponsibilities(t *testing.T) {
	doc := readContractDoc(t)
	for _, want := range []string{
		// Host responsibilities section + items.
		"Host responsibilities",
		".midas-graph-viewport",
		".midas-graph-renderer-slot",
		"data-active-renderer",
		"renderer registry",
		"activation lifecycle",
		"Baseline restoration",
		"Safe-area calculation",
		"Resize broadcast",
		"Clipping boundary",
		"Chrome anchoring",
		// Renderer responsibilities section + items.
		"Renderer responsibilities",
		"only what it creates",
		"graph engine",
		"destroy()",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35h: architecture document must document host/renderer responsibilities — missing %q", want)
		}
	}
}

// TestExplorer_D35hDocs_DocumentsDomFactoryAndContextContracts
// pins the DOM contract, factory contract, and ctx contract.
func TestExplorer_D35hDocs_DocumentsDomFactoryAndContextContracts(t *testing.T) {
	doc := readContractDoc(t)
	for _, want := range []string{
		// DOM contract.
		"DOM contract",
		".governance-map-body",
		// Factory contract.
		"Renderer factory contract",
		"mount(slotEl, ctx)",
		"destroy()",
		"return", // factory returns a handle
		// Context contract fields.
		"Renderer context contract",
		"viewportEl",
		"slotEl",
		"getViewportRect",
		"getSafeArea",
		"onResize",
		"hooks",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35h: architecture document must document DOM/factory/ctx contracts — missing %q", want)
		}
	}
}

// TestExplorer_D35hDocs_DocumentsRegistryAndActivationLifecycle
// pins the registry API and the activation/deactivation lifecycle,
// including the D35g REPLACE policy and the native baseline.
func TestExplorer_D35hDocs_DocumentsRegistryAndActivationLifecycle(t *testing.T) {
	doc := readContractDoc(t)
	for _, want := range []string{
		// Registry contract section + every public API entry.
		"Registry contract",
		"register(rendererId, factory)",
		"unregister(rendererId)",
		"hasRenderer(rendererId)",
		"listRegistered()",
		"activateById(rendererId)",
		"activate(rendererId, factory)",
		"adoptExisting",
		"deactivate()",
		"getActiveRendererId()",
		// D35g REPLACE policy.
		"REPLACE",
		"idempotent",
		// Activation lifecycle section.
		"Activation lifecycle",
		"factory.mount",
		"factory.destroy",
		// Baseline adoption.
		"native-context",
		"baseline",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35h: architecture document must document registry + activation lifecycle — missing %q", want)
		}
	}
}

// TestExplorer_D35hDocs_DocumentsRendererIdentitySafeAreaResizeAndClipping
// pins the identity / safe-area / resize / clipping contracts.
func TestExplorer_D35hDocs_DocumentsRendererIdentitySafeAreaResizeAndClipping(t *testing.T) {
	doc := readContractDoc(t)
	for _, want := range []string{
		// Renderer identity contract.
		"Renderer identity contract",
		`data-active-renderer="`,
		// Safe area.
		"Safe-area contract",
		"chrome-aware",
		"fit padding",
		// Resize.
		"Resize contract",
		"ctx.onResize",
		"unsubscribe",
		// Clipping.
		"Clipping contract",
		"strategic clip authority",
		".context-cy-spike-overlay",
		"projection layer",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35h: architecture document must document identity/safe-area/resize/clipping — missing %q", want)
		}
	}
}

// TestExplorer_D35hDocs_DocumentsAntiPatternsAndFutureRendererChecklist
// pins the anti-patterns list and the integration checklist.
func TestExplorer_D35hDocs_DocumentsAntiPatternsAndFutureRendererChecklist(t *testing.T) {
	doc := readContractDoc(t)
	for _, want := range []string{
		// Anti-patterns section.
		"Anti-patterns",
		"body.cytoscape-poc-active",
		"body.context-cy-spike-active",
		".governance-map-canvas-scroll",
		"Renderer-specific viewport abstractions",
		"Overlay-level clipping",
		"Fallback append paths",
		"Tactical CSS patches",
		// Future renderer checklist.
		"Future renderer integration checklist",
		// Concrete checklist items.
		"Choose a `rendererId`",
		"mount(slotEl, ctx)",
		"viewport.register(rendererId, factory)",
		"viewport.activateById(rendererId)",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35h: architecture document must document anti-patterns + checklist — missing %q", want)
		}
	}
}

// TestExplorer_D35hViewport_ReferencesContractDoc pins that
// graph-viewport.js carries a top-of-file reference to the
// architecture document, so a developer reading the source can
// find the contract.
func TestExplorer_D35hViewport_ReferencesContractDoc(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	for _, want := range []string{
		// Header must point at the architecture document.
		"docs/design/midas-graph-viewport.md",
		// And explain WHY it matters (one short sentence about the
		// canonical reference / integration pattern).
		"Architecture contract",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35h: graph-viewport.js must reference the architecture document — missing %q", want)
		}
	}
}

// TestExplorer_D35h_D35gRegistryContractPreserved pins that the
// D35g registry surface and renderer-module registrations remain
// intact. D35h is documentation; it must not regress D35g.
func TestExplorer_D35h_D35gRegistryContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")

	// Registry API exported on the host.
	for _, want := range []string{
		"function register(rendererId, factory)",
		"function unregister(rendererId)",
		"function hasRenderer(rendererId)",
		"function listRegistered()",
		"function activateById(rendererId)",
		"register:             register,",
		"unregister:           unregister,",
		"hasRenderer:          hasRenderer,",
		"listRegistered:       listRegistered,",
		"activateById:         activateById,",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35h: D35g registry API %q must remain on the host", want)
		}
	}

	// Authority registers + activates by id (D35g contract).
	if !strings.Contains(authJS, "vp.register('authority', _authorityRendererFactory)") {
		t.Error("D35h: D35g — Authority must remain registered with the host")
	}
	if !strings.Contains(authJS, "vp.activateById('authority')") {
		t.Error("D35h: D35g — Authority must activate via vp.activateById")
	}

	// Context registers + activates by id (D35g contract).
	if !strings.Contains(ctxJS, "vp.register('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35h: D35g — Context must remain registered with the host")
	}
	if !strings.Contains(ctxJS, "vp.activateById('context-cytoscape')") {
		t.Error("D35h: D35g — Context must activate via vp.activateById")
	}

	// Native context remains ADOPTED (not registered).
	for _, path := range []string{
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
	} {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, "register('native-context'") ||
			strings.Contains(js, `register("native-context"`) {
			t.Errorf("D35h: native-context must remain ADOPTED, not registered; found register('native-context') in %s", path)
		}
	}
	// And adoption itself is still in place.
	if !strings.Contains(hostJS, "adoptExisting('native-context')") {
		t.Error("D35h: native-context baseline adoption must remain")
	}
}

// TestExplorer_D35h_D35fRendererIdentityAndClipContractsPreserved
// pins the D35f host-owned identity attribute and the strategic
// clipping authority remain intact.
func TestExplorer_D35h_D35fRendererIdentityAndClipContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	// D35f — host-owned data-active-renderer.
	for _, want := range []string{
		`var ACTIVE_RENDERER_ATTR = 'data-active-renderer';`,
		"function _setActiveRendererAttribute(rendererId)",
		"ACTIVE_RENDERER_ATTR: ACTIVE_RENDERER_ATTR",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35h: D35f data-active-renderer contract %q must remain", want)
		}
	}

	// D35f — viewport is the strategic clip authority (rule lives
	// in governance-map.css).
	clipCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	clipExec := stripCSSComments(clipCSS)
	if !strings.Contains(clipExec, ".midas-graph-viewport") ||
		!strings.Contains(clipExec, "overflow: hidden") {
		t.Error("D35h: `.midas-graph-viewport { overflow: hidden }` strategic clip must remain (D35f)")
	}

	// D35e — overlay must remain non-clipping (the spike CSS still
	// has exactly one `overflow: hidden`, on the mount; not on the
	// overlay).
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if strings.Count(stripCSSComments(spikeCSS), "overflow: hidden") != 1 {
		t.Error("D35h: spike CSS must have exactly 1 `overflow: hidden` (mount; overlay is non-clipping)")
	}
}

// TestExplorer_D35h_NoLegacyActivationOrFallbackRegression checks
// that the documentation tranche has not accidentally reintroduced
// any retired contract. Scans renderer-relevant JS for legacy
// patterns.
func TestExplorer_D35h_NoLegacyActivationOrFallbackRegression(t *testing.T) {
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

	for _, path := range rendererJSAssets {
		js := getExplorerAsset(t, srv, path)
		exec := stripJSComments(js)

		// Pre-D35g direct factory activation must not return for
		// Authority/Context/Knowledge.
		if strings.Contains(exec, "vp.activate('authority',") ||
			strings.Contains(exec, "vp.activate('context-cytoscape',") ||
			strings.Contains(exec, "vp.activate('knowledge-graph',") {
			t.Errorf("D35h/D36a: %s contains a pre-D35g direct factory activation; use register + activateById", path)
		}

		// Pre-D35f body-class activation flips must not return.
		for _, banned := range []string{
			"document.body.classList.add('cytoscape-poc-active')",
			"document.body.classList.add('context-cy-spike-active')",
			"document.body.classList.add(BODY_FLAG_CLASS)",
			"document.body.classList.remove(BODY_FLAG_CLASS)",
		} {
			if strings.Contains(exec, banned) {
				t.Errorf("D35h: %s reintroduces retired body-class activation %q", path, banned)
			}
		}
	}

	// Spike CSS — overlay must not have its retired overflow:hidden
	// reintroduced.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	spikeExec := stripCSSComments(spikeCSS)
	// Look for the specific overlay rule that pre-D35e had.
	if strings.Contains(spikeExec, ".context-cy-spike-overlay") {
		// The overlay selector must NOT carry an overflow:hidden
		// declaration. We bound the search to the next `}` so
		// downstream rules don't false-match.
		idx := strings.Index(spikeExec, ".context-cy-spike-overlay")
		end := strings.Index(spikeExec[idx:], "}")
		if end < 0 {
			t.Fatal("D35h: .context-cy-spike-overlay rule has no closing brace")
		}
		overlayBody := spikeExec[idx : idx+end]
		if strings.Contains(overlayBody, "overflow: hidden") {
			t.Error("D35h: .context-cy-spike-overlay must remain non-clipping (D35e contract); reintroducing overflow:hidden is forbidden")
		}
	}
}

// TestExplorer_D35h_NoNewRendererOrFeature pins that D35h has not
// introduced a new graph renderer or graph-domain feature. The
// only registered renderers are still 'authority' and
// 'context-cytoscape'.
func TestExplorer_D35h_NoNewRendererOrFeature(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Sweep every renderer asset for `vp.register(...)` calls. The
	// set must equal exactly the known renderer ids.
	//
	// D36a — Allow-list relaxed to include the Knowledge Graph
	// renderer shell ('knowledge-graph'). The shell is the first
	// controlled reuse of the GraphViewport platform module and
	// registers in /explorer/assets/js/graph/knowledge/knowledge-
	// graph-renderer.js. Drift / Resilience / Evidence / Policy /
	// service-topology ids remain forbidden — when a future tranche
	// introduces one of those, relax this allow-list in that
	// tranche just as D36a did for 'knowledge-graph'.
	rendererJSAssets := []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js",
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
	}

	allowedRegistrations := map[string]bool{
		"'authority'": true,
		"'context-cytoscape'":   true,
		"'knowledge-graph'":     true, // D36a — first controlled reuse.
	}

	for _, path := range rendererJSAssets {
		js := getExplorerAsset(t, srv, path)
		exec := stripJSComments(js)

		// Scan all `vp.register(` occurrences.
		i := 0
		for {
			idx := strings.Index(exec[i:], "vp.register(")
			if idx < 0 {
				break
			}
			start := i + idx + len("vp.register(")
			// Pull the first quoted argument out (delimited by `,`).
			commaIdx := strings.Index(exec[start:], ",")
			if commaIdx < 0 {
				break
			}
			rawId := strings.TrimSpace(exec[start : start+commaIdx])
			if !allowedRegistrations[rawId] {
				t.Errorf("D35h/D36a: %s registers an unexpected renderer id %s — not in the {Authority, Context, Knowledge} allow-list. New renderer domains must be introduced in their own tranche and added to the allow-list there.", path, rawId)
			}
			i = start + commaIdx
		}
	}

	// Sweep new-renderer-id markers that would indicate an
	// out-of-scope graph domain was added. Knowledge is intentionally
	// NOT in this banned list anymore (D36a relaxation). Drift /
	// Resilience / Evidence / Policy / service-topology remain
	// forbidden until their own dedicated tranches.
	for _, path := range rendererJSAssets {
		js := getExplorerAsset(t, srv, path)
		exec := stripJSComments(js)
		for _, bannedId := range []string{
			"'drift-graph'",
			"'drift-topology'",
			"'resilience-graph'",
			"'resilience-topology'",
			"'service-topology'",
			"'evidence-graph'",
			"'policy-graph'",
		} {
			if strings.Contains(exec, bannedId) {
				t.Errorf("D35h/D36a: %s references an out-of-scope graph-domain id %s — that domain is not yet in scope", path, bannedId)
			}
		}
	}
}

// TestExplorer_D35hDescriptor_ModelPinnedIfImplemented pins the
// descriptor model's documentation state. D35h DEFERRED descriptor
// hardening (the brief permits this). This test pins that:
//   • the architecture document documents the descriptor sketch as
//     a future option (Appendix A);
//   • the descriptor model is NOT implemented as code in
//     graph-viewport.js (no `function registerDescriptor`,
//     no `kind:` / `engine:` / `capabilities:` literal in the
//     viewport surface);
//   • the D35g `register(rendererId, factory)` API remains the
//     single registration entry.
//
// If a future tranche implements descriptors, update this test to
// pin the implemented shape rather than the deferred state.
func TestExplorer_D35hDescriptor_ModelPinnedIfImplemented(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")
	doc := readContractDoc(t)

	// Document mentions the descriptor model as a deferred / future
	// option (Appendix A in the architecture note).
	for _, want := range []string{
		"renderer descriptor",
		"deferred",
		// Sketched fields per the brief.
		"htmlOverlay",
		"safeAreaAware",
		"resizeAware",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35h: architecture document must document the descriptor sketch (deferred) — missing %q", want)
		}
	}

	// Host JS must NOT carry a descriptor implementation. A second
	// registration entry-point would split the D35g registry into
	// parallel surfaces.
	for _, banned := range []string{
		"function registerDescriptor",
		"function _registerDescriptor",
		"registerDescriptor:",
	} {
		if strings.Contains(hostJS, banned) {
			t.Errorf("D35h: descriptor hardening was deferred — host JS must not contain %q", banned)
		}
	}
}

// TestExplorer_D35h_D35aThroughD35gContractsPreserved is the
// foundation-wide regression check. Every prior D35 invariant must
// remain intact through the documentation tranche.
func TestExplorer_D35h_D35aThroughD35gContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a — structural DOM.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35h: D35a structural class %q must remain", want)
		}
	}

	// D35b/c — host API + adoption + baseline.
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")
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
			t.Errorf("D35h: D35b/c host API %q must remain", want)
		}
	}

	// D35d — Authority migrated.
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	for _, want := range []string{
		"var _authorityRendererFactory = {",
		"slotEl.appendChild(_mountEl)",
		"_rendererCtx.getSafeArea",
	} {
		if !strings.Contains(authJS, want) {
			t.Errorf("D35h: D35d Authority contract %q must remain", want)
		}
	}

	// D35e — Context migrated.
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	for _, want := range []string{
		"var _contextCytoscapeRendererFactory = {",
		"_installResources(slotEl)",
		"ctx.onResize(_onHostResize)",
	} {
		if !strings.Contains(ctxJS, want) {
			t.Errorf("D35h: D35e Context contract %q must remain", want)
		}
	}
}
