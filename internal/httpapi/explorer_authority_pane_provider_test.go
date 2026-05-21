package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-authority-2-impl — Authority Selected-Object Pane Provider tests.
//
// Pins the canvas-edge-tabs module's new integration with the shared
// `graphSelectedObjectPane` shell:
//
//   • the canvas-edge tabs facade registers itself as the 'authority'
//     provider via `graphSelectedObjectPane.registerLensProvider`;
//   • the provider exposes a stable shape (id / label / sections /
//     copy / open / close / toggle / isOpen / setPaneMode /
//     getPaneMode / notifySelectionChanged / render / getSelection);
//   • section ids cover the existing canvas-edge tab vocabulary
//     ('details', 'authority', 'evidence');
//   • the provider delegates to the existing canvas-edge controller —
//     it does not re-implement tab rendering;
//   • the provider does NOT publish into the shared selection bridge
//     (selection publishing remains owned by D37p-authority-1-impl in
//     authority-cytoscape-poc.js);
//   • the provider does NOT subscribe to graphSelectionBridge nor
//     call drawer / workbench / tray side-effect setters from the
//     provider block;
//   • the existing canvas-edge public surface (init / destroy /
//     render / openTab / closePane / syncSelection / isOpen) is
//     preserved verbatim;
//   • the DOM wrapper + asset references in index.html remain intact;
//   • the shared shell module remains generic — no Authority-specific
//     coupling leaks back into it.

const (
	d37pAuth2EdgeAsset           = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37pAuth2EdgeCss             = "/explorer/assets/css/authority-canvas-edge-tabs.css"
	d37pAuth2ShellAsset          = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37pAuth2SharedBridgeAsset   = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37pAuth2AuthorityPoc        = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37pAuth2AuthorityToolbar    = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
	d37pAuth2AuthorityWorkbench  = "/explorer/assets/js/graph/authority/authority-graph-workbench.js"
	d37pAuth2CameraBusAsset      = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37pAuth2ViewportAsset       = "/explorer/assets/js/graph/graph-viewport.js"
	d37pAuth2DrawerAsset         = "/explorer/assets/js/graph/graph-drawer.js"
	d37pAuth2ContextPaneAsset    = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
)

// ── A. Authority registers a provider with the shared shell ─────────

func TestExplorer_D37pAuthority2_RegistersProviderWithSharedShell(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	if !strings.Contains(js, "graphSelectedObjectPane") {
		t.Errorf("D37p-authority-2-impl: authority-canvas-edge-tabs.js must reference graphSelectedObjectPane")
	}
	if !strings.Contains(js, "registerLensProvider('authority'") {
		t.Errorf("D37p-authority-2-impl: Authority canvas-edge tabs must call graphSelectedObjectPane.registerLensProvider('authority', ...)")
	}
}

func TestExplorer_D37pAuthority2_ProviderShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// The provider object literal exposes the locked shape declared
	// by the shared shell's contract.
	for _, want := range []string{
		"id:    'authority'",
		"label: 'Authority'",
		"sections:",
		"copy:",
		"open: function",
		"close: function",
		"toggle: function",
		"isOpen: function",
		"setPaneMode: function",
		"getPaneMode: function",
		"notifySelectionChanged: function",
		"render: function",
		"getSelection: function",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-2-impl: provider must declare %q", want)
		}
	}
}

func TestExplorer_D37pAuthority2_ProviderSectionsCoverCanvasEdgeTabs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// Section ids mirror the existing canvas-edge TAB_IDS vocabulary.
	for _, want := range []string{
		"id: 'details'",
		"id: 'authority'",
		"id: 'evidence'",
		"label: 'Details'",
		"label: 'Authority'",
		"label: 'Evidence'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-2-impl: provider sections must include %q", want)
		}
	}
}

// ── B. Delegation, not duplication ─────────────────────────────────

func TestExplorer_D37pAuthority2_ProviderDelegatesToExistingControllers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// The provider's methods invoke the canvas-edge controller's
	// existing functions; no parallel tab-rendering implementation.
	startIdx := strings.Index(js, "var _paneProvider")
	if startIdx < 0 {
		t.Fatalf("D37p-authority-2-impl: provider object _paneProvider must be declared")
	}
	endIdx := strings.Index(js[startIdx:], "function _registerWithSharedPaneShell")
	if endIdx < 0 {
		t.Fatalf("D37p-authority-2-impl: _paneProvider object must end before _registerWithSharedPaneShell")
	}
	providerBlock := js[startIdx : startIdx+endIdx]

	for _, want := range []string{
		"openTab(",
		"closePane()",
		"isOpen()",
		"render()",
	} {
		if !strings.Contains(providerBlock, want) {
			t.Errorf("D37p-authority-2-impl: provider must delegate to existing canvas-edge function %q", want)
		}
	}
	// The provider must NOT contain rendering logic of its own —
	// no DOM creation, no innerHTML, no tab-specific renderers.
	for _, banned := range []string{
		"createElement",
		"appendChild",
		"innerHTML",
		"renderDetailsTab(",
		"renderAuthorityTab(",
		"renderEvidenceTab(",
		"_buildDetailsModel(",
		"_buildAuthorityModel(",
		"_buildEvidenceModel(",
	} {
		if strings.Contains(providerBlock, banned) {
			t.Errorf("D37p-authority-2-impl: provider must not duplicate tab rendering (%q)", banned)
		}
	}
}

// ── C. Registration site + idempotency ─────────────────────────────

func TestExplorer_D37pAuthority2_RegisterHelperCalledFromInit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	if !strings.Contains(js, "function _registerWithSharedPaneShell") {
		t.Errorf("D37p-authority-2-impl: a _registerWithSharedPaneShell helper must be declared")
	}
	if !strings.Contains(js, "_registerWithSharedPaneShell();") {
		t.Errorf("D37p-authority-2-impl: _registerWithSharedPaneShell must be called from init()")
	}
	// The register helper must defensively bail when the shell is
	// unavailable so the canvas-edge tabs continue to work without
	// the shared platform.
	if !regexp.MustCompile(`function _registerWithSharedPaneShell\(\)[\s\S]*?if \(!shell \|\| typeof shell\.registerLensProvider !== 'function'\) return false;`).MatchString(js) {
		t.Errorf("D37p-authority-2-impl: register helper must defensively short-circuit when the shell is unavailable")
	}
}

func TestExplorer_D37pAuthority2_AssertsActiveLensOnAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	if !strings.Contains(js, "shell.setActiveLens('authority')") {
		t.Errorf("D37p-authority-2-impl: Authority provider must call graphSelectedObjectPane.setActiveLens('authority') when Authority is active")
	}
	if !strings.Contains(js, "_isAuthorityLensActive()") {
		t.Errorf("D37p-authority-2-impl: active-lens assertion must be gated on Authority being the active renderer")
	}
}

// ── D. Recursion + side-effect discipline ──────────────────────────

func TestExplorer_D37pAuthority2_ProviderDoesNotPublishSelection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// The provider consumes selection events; it must not publish
	// them. The existing D37p-authority-1-impl IIFE in
	// authority-cytoscape-poc.js owns the publish contract.
	startIdx := strings.Index(js, "var _paneProvider")
	if startIdx < 0 {
		t.Fatalf("D37p-authority-2-impl: provider object must be declared")
	}
	endIdx := strings.Index(js[startIdx:], "function _registerWithSharedPaneShell")
	if endIdx < 0 {
		t.Fatalf("D37p-authority-2-impl: provider block must end before _registerWithSharedPaneShell")
	}
	providerBlock := js[startIdx : startIdx+endIdx]

	for _, banned := range []string{
		"graphSelectionBridge.selectCard",
		"graphSelectionBridge.clearSelection",
		"bridge.selectCard(",
		"bridge.clearSelection(",
	} {
		if strings.Contains(providerBlock, banned) {
			t.Errorf("D37p-authority-2-impl: provider must NOT publish into graphSelectionBridge (%q)", banned)
		}
	}
}

func TestExplorer_D37pAuthority2_ProviderNoDrawerOrWorkbenchCalls(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	startIdx := strings.Index(js, "var _paneProvider")
	if startIdx < 0 {
		t.Fatalf("D37p-authority-2-impl: provider object must be declared")
	}
	endIdx := strings.Index(js[startIdx:], "function _registerWithSharedPaneShell")
	if endIdx < 0 {
		t.Fatalf("D37p-authority-2-impl: provider block must end before _registerWithSharedPaneShell")
	}
	providerBlock := js[startIdx : startIdx+endIdx]

	// The provider must not call drawer / workbench / tray setters.
	for _, banned := range []string{
		"drawer.",
		"authorityWorkbench.render",
		"authorityWorkbench.setActiveTab",
		"contextEvidenceTray",
		".setName(",
		".setFields(",
		".setGovernance(",
		".setActions(",
		".setInlineActions(",
	} {
		if strings.Contains(providerBlock, banned) {
			t.Errorf("D37p-authority-2-impl: provider must not call lens-side-effect setter %q (those stay where they live)", banned)
		}
	}
}

// TestExplorer_D37pAuthority2_NotifySelectionChangedIsNoOp pins that
// the provider's notifySelectionChanged hook is a deliberate no-op.
// The existing `cytoscapePoc.onSelectionChanged(syncSelection)`
// subscription already drives canvas-edge repaints; forwarding the
// shell's notifySelectionChanged here would double-render on every
// selection. The hook signature is preserved so the shell can call
// it safely.
func TestExplorer_D37pAuthority2_NotifySelectionChangedIsNoOp(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// The hook exists and references its parameters defensively
	// (the `void selection; void event;` pattern documents intent).
	if !regexp.MustCompile(`notifySelectionChanged: function\s*\(\s*selection,\s*event\s*\)\s*\{\s*void selection; void event;\s*\}`).MatchString(js) {
		t.Errorf("D37p-authority-2-impl: notifySelectionChanged must be a deliberate no-op (existing onSelectionChanged subscription already drives canvas-edge repaints)")
	}
}

// ── E. Pane-mode handling ──────────────────────────────────────────

func TestExplorer_D37pAuthority2_PaneModeLocked(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// PANE_MODES locked to the shared shell's vocabulary.
	for _, want := range []string{
		"'auto'",
		"'pinned'",
		"'hidden'",
		"var _paneMode",
		"var PANE_MODES",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-2-impl: PANE_MODES vocabulary must declare %q", want)
		}
	}
	// Mode setter ignores unknown values and never throws.
	if !regexp.MustCompile(`if \(PANE_MODES\.indexOf\(mode\) < 0\) return;`).MatchString(js) {
		t.Errorf("D37p-authority-2-impl: setPaneMode must guard the input against unknown values")
	}
}

// ── F. Existing canvas-edge public surface preserved ───────────────

func TestExplorer_D37pAuthority2_CanvasEdgePublicSurfacePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	if !strings.Contains(js, "window.MIDASExplorerGraph.authorityCanvasEdgeTabs") {
		t.Errorf("D37p-authority-2-impl: authorityCanvasEdgeTabs public surface must remain present")
	}
	for _, want := range []string{
		"init:",
		"destroy:",
		"render:",
		"openTab:",
		"closePane:",
		"syncSelection:",
		"isOpen:",
		"_TAB_IDS:",
		"_ELIGIBLE_KINDS:",
		"_FIELD_DEFS:",
		"_WORKBENCH_MAPPING:",
		"_COPY:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-2-impl: canvas-edge public surface must still export %q", want)
		}
	}
	// Diagnostic surface gains the new provider reference.
	for _, want := range []string{
		"_paneProvider:",
		"_PANE_MODES:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-2-impl: canvas-edge diagnostic surface must export %q for tests", want)
		}
	}
}

// ── G. No canvas-edge retirement ───────────────────────────────────

func TestExplorer_D37pAuthority2_CanvasEdgeDomWrapperIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`data-authority-canvas-edge-tabs`,
		`id="gmap-canvas-edge-pane"`,
		`data-canvas-edge-tab="details"`,
		`data-canvas-edge-tab="authority"`,
		`data-canvas-edge-tab="evidence"`,
		`src="/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"`,
		`href="/explorer/assets/css/authority-canvas-edge-tabs.css"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37p-authority-2-impl: index.html must still contain %q (no canvas-edge retirement in this tranche)", want)
		}
	}
}

func TestExplorer_D37pAuthority2_CanvasEdgeCssStillServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	if len(getExplorerAsset(t, srv, d37pAuth2EdgeCss)) == 0 {
		t.Errorf("D37p-authority-2-impl: authority-canvas-edge-tabs.css must still be served")
	}
}

// ── H. Shared shell stays generic ──────────────────────────────────

func TestExplorer_D37pAuthority2_SharedShellStaysGeneric(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2ShellAsset)

	// The shell module itself must not learn Authority semantics.
	// The shell's header comment is allowed to mention Authority as
	// an example lens; the bans below target executable references
	// that would betray lens-specific coupling.
	for _, banned := range []string{
		"'authority'",
		"\"authority\"",
		"registerLensProvider('authority'",
		"authorityCanvasEdgeTabs",
		"authorityWorkbench",
		"cytoscapePoc",
		"cytoscape",
		"Cytoscape",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-authority-2-impl: shared shell must remain renderer-agnostic; found %q", banned)
		}
	}
}

// ── I. Selection-bridge integration preserved ──────────────────────

func TestExplorer_D37pAuthority2_SelectionBridgeDelegateStillRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2AuthorityPoc)

	// The D37p-authority-1-impl bridge delegate IIFE must remain
	// present, including the `graphSelectionBridge` reference, the
	// `'authority'` registration call, and the selection-publish
	// helpers. The registration goes through a local `bridge`
	// variable wrapped around `MIDASExplorerGraph.graphSelectionBridge`,
	// so we pin the underlying substring as well as the local form.
	for _, want := range []string{
		"_registerAuthoritySelectionBridgeDelegate",
		"graphSelectionBridge",
		"bridge.registerLens('authority'",
		"bridge.selectCard(",
		"bridge.clearSelection(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-authority-2-impl: D37p-authority-1 selection bridge wiring must remain (%q)", want)
		}
	}
}

// ── J. Foundation preservation ─────────────────────────────────────

func TestExplorer_D37pAuthority2_FoundationModulesIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37pAuth2ShellAsset,
		d37pAuth2SharedBridgeAsset,
		d37pAuth2CameraBusAsset,
		d37pAuth2ViewportAsset,
		d37pAuth2DrawerAsset,
		d37pAuth2AuthorityPoc,
		d37pAuth2AuthorityToolbar,
		d37pAuth2AuthorityWorkbench,
		d37pAuth2ContextPaneAsset,
	} {
		if got := getExplorerAsset(t, srv, asset); len(got) == 0 {
			t.Errorf("D37p-authority-2-impl: foundation asset %q must still be served", asset)
		}
	}
}

func TestExplorer_D37pAuthority2_CameraBusDelegateIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2AuthorityPoc)

	if !strings.Contains(js, "_registerAuthorityCameraBusDelegate") {
		t.Errorf("D37p-authority-2-impl: D37p-impl-4 camera-bus delegate IIFE must remain present")
	}
	if !strings.Contains(js, "bus.registerLens('authority'") {
		t.Errorf("D37p-authority-2-impl: camera-bus delegate registration with 'authority' lens id must remain present")
	}
}

func TestExplorer_D37pAuthority2_ContextProviderUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2ContextPaneAsset)

	// Context's provider registration from D37p-pane-1 stays intact.
	if !strings.Contains(js, "registerLensProvider('context'") {
		t.Errorf("D37p-authority-2-impl: Context provider registration must remain present")
	}
}

// ── K. Viewport observer keeps active lens aligned ─────────────────

// TestExplorer_D37pAuthority2_ViewportObserverUpdatesActiveLens pins
// that the canvas-edge tabs' existing viewport-attribute observer
// asserts `graphSelectedObjectPane.setActiveLens('authority')` when
// Authority becomes the active renderer. This avoids a parallel
// observer and keeps the shell's active provider aligned with
// renderer-identity flips.
func TestExplorer_D37pAuthority2_ViewportObserverUpdatesActiveLens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	if !regexp.MustCompile(`(?s)_bindViewportObserver[\s\S]*?_isAuthorityLensActive\(\)[\s\S]*?shell\.setActiveLens\('authority'\)`).MatchString(js) {
		t.Errorf("D37p-authority-2-impl: the viewport observer must call shell.setActiveLens('authority') when Authority becomes active")
	}
}

// ── L. Index.html unchanged + line-count ceiling ───────────────────

func TestExplorer_D37pAuthority2_IndexHtmlWithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 7820 {
		t.Errorf("D37p-authority-2-impl: index.html line count %d exceeds the existing 7820 ceiling", lines)
	}
}

func TestExplorer_D37pAuthority2_CanvasEdgeScriptLoadedOnce(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if c := strings.Count(body, `src="/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"`); c != 1 {
		t.Errorf("D37p-authority-2-impl: authority-canvas-edge-tabs.js must be loaded exactly once (found %d)", c)
	}
}
