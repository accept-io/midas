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

// ---------------------------------------------------------------------------
// D37ak-graph-native-tabs-contract-impl — Authority provider migrated onto
// the formal GraphTabConfig contract.
//
// Pins:
//   • AUTHORITY_TAB_CONFIG declared as a module-level constant
//   • config.enabled = true; defaultTab = 'details'
//   • config.items contains exactly three entries — details, authority,
//     evidence — each with id / label / provider / supports
//   • support lists cover the Authority kind vocabulary; evidence
//     supports ['*']
//   • the provider object exposes `tabs: AUTHORITY_TAB_CONFIG`
//   • openTab() / closePane() forward into shell.setActiveTab so the
//     shell shadow stays aligned with canvas-edge intent
//   • diagnostic export surfaces _AUTHORITY_TAB_CONFIG
//   • the canvas-edge module does not duplicate the shell's tab-config
//     introspection helpers (no parallel getTabs / getDefaultTab here)
//
// All previous D37p-authority-2-impl invariants are preserved
// unchanged; the migration adds a declarative `tabs` field plus the
// shell-shadow sync calls.

// ── M. Declarative tab config ──────────────────────────────────────

func TestExplorer_D37ak_Authority_TabConfigDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	if !strings.Contains(js, "var AUTHORITY_TAB_CONFIG = {") {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: AUTHORITY_TAB_CONFIG must be declared as a module-level constant")
	}
	for _, want := range []string{
		"enabled:    true,",
		"defaultTab: 'details',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37ak-graph-native-tabs-contract-impl: AUTHORITY_TAB_CONFIG must declare %q", want)
		}
	}
}

func TestExplorer_D37ak_Authority_TabItems(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// Extract the AUTHORITY_TAB_CONFIG block to scope the pins.
	startIdx := strings.Index(js, "var AUTHORITY_TAB_CONFIG = {")
	if startIdx < 0 {
		t.Fatal("D37ak-graph-native-tabs-contract-impl: AUTHORITY_TAB_CONFIG must be declared")
	}
	tail := js[startIdx:]
	// Find the closing '};' that ends the config literal. The next
	// `\n  var _paneProvider` marker is a robust delimiter — the
	// config sits between AUTHORITY_TAB_CONFIG and the provider.
	endIdx := strings.Index(tail, "var _paneProvider")
	if endIdx < 0 {
		t.Fatalf("D37ak-graph-native-tabs-contract-impl: AUTHORITY_TAB_CONFIG must precede _paneProvider")
	}
	cfg := tail[:endIdx]

	// Three tab items: details / authority / evidence.
	for _, want := range []string{
		"id:       'details',",
		"id:       'authority',",
		"id:       'evidence',",
		"label:    'Details',",
		"label:    'Authority',",
		"label:    'Evidence',",
		"provider: 'authority.details',",
		"provider: 'authority.authority',",
		"provider: 'authority.evidence',",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("D37ak-graph-native-tabs-contract-impl: AUTHORITY_TAB_CONFIG.items must declare %q", want)
		}
	}
	// supports lists.
	if !strings.Contains(cfg, "supports: [\n          'business_service',\n          'decision_surface',\n          'authority_profile',\n          'authority_grant',\n          'agent',\n          'fail_mode_policy',\n          'escalation_target',\n        ],") {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: details tab supports must cover the Authority eligible-kind vocabulary")
	}
	if !strings.Contains(cfg, "supports: ['*'],") {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: evidence tab supports must be ['*'] (wildcard)")
	}
	// The exact count of supports lists should be three (one per tab).
	if got := strings.Count(cfg, "supports:"); got != 3 {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: AUTHORITY_TAB_CONFIG must have exactly 3 supports lists (one per tab); got %d", got)
	}
}

// ── N. Provider exposes the formal config ──────────────────────────

func TestExplorer_D37ak_Authority_ProviderExposesTabsField(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// The provider literal must include `tabs: AUTHORITY_TAB_CONFIG`.
	startIdx := strings.Index(js, "var _paneProvider")
	if startIdx < 0 {
		t.Fatalf("D37ak-graph-native-tabs-contract-impl: provider object must be declared")
	}
	endIdx := strings.Index(js[startIdx:], "function _registerWithSharedPaneShell")
	if endIdx < 0 {
		t.Fatalf("D37ak-graph-native-tabs-contract-impl: provider block must end before _registerWithSharedPaneShell")
	}
	providerBlock := js[startIdx : startIdx+endIdx]

	if !strings.Contains(providerBlock, "tabs: AUTHORITY_TAB_CONFIG") {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: provider must expose `tabs: AUTHORITY_TAB_CONFIG`")
	}
	// The legacy `sections` field is preserved for backward-compat
	// introspection tools (D37p-authority-2-impl invariant).
	if !strings.Contains(providerBlock, "sections:") {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: provider must retain its sections field (preserved from D37p-authority-2-impl)")
	}
}

// ── O. Shell sync on openTab/closePane ─────────────────────────────

func TestExplorer_D37ak_Authority_OpenTabSyncsShellShadow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// openTab must end with a _syncShellActiveTab(tabId) call.
	if !regexp.MustCompile(`(?s)function openTab\(tabId\)[\s\S]*?_syncShellActiveTab\(tabId\);\s*\n  \}`).MatchString(js) {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: openTab must end by calling _syncShellActiveTab(tabId) so the shell shadow stays aligned")
	}
	// closePane must clear the shadow via _syncShellActiveTab(null).
	if !regexp.MustCompile(`(?s)function closePane\(\)[\s\S]*?_syncShellActiveTab\(null\);\s*\n  \}`).MatchString(js) {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: closePane must end by calling _syncShellActiveTab(null) so the shell shadow clears")
	}
	// The sync helper itself must defend against the shell being
	// unavailable.
	if !regexp.MustCompile(`function _syncShellActiveTab\(tabId\)\s*\{[\s\S]*?if \(!shell \|\| typeof shell\.setActiveTab !== 'function'\) return;[\s\S]*?shell\.setActiveTab\(tabId, 'authority'\);`).MatchString(js) {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: _syncShellActiveTab must defensively short-circuit when the shell is unavailable")
	}
}

// ── P. Diagnostic export ───────────────────────────────────────────

func TestExplorer_D37ak_Authority_DiagnosticExportsAuthorityTabConfig(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	if !strings.Contains(js, "_AUTHORITY_TAB_CONFIG: AUTHORITY_TAB_CONFIG,") {
		t.Errorf("D37ak-graph-native-tabs-contract-impl: canvas-edge diagnostic surface must export _AUTHORITY_TAB_CONFIG")
	}
}

// ── Q. Authority module does not duplicate shell helpers ───────────

func TestExplorer_D37ak_Authority_DoesNotDuplicateShellHelpers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuth2EdgeAsset)

	// The Authority module must not declare any parallel introspection
	// helpers — those live on the shell. (Function declarations only;
	// referencing the shell's methods is allowed.)
	for _, banned := range []string{
		"function getTabConfig(",
		"function getTabs(",
		"function getDefaultTab(",
		"function tabSupportsKind(",
		"function buildSelectionContext(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37ak-graph-native-tabs-contract-impl: Authority module must not duplicate shell helper %q (use graphSelectedObjectPane instead)", banned)
		}
	}
}

// ── R. Legacy letterbox guard (D37ak does not touch the drawer) ────

func TestExplorer_D37ak_LegacyLetterboxUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Right-side letterbox markup remains intact — D37ak does NOT
	// remove or disable it.
	for _, want := range []string{
		`id="gmap-details"`,
		`class="governance-map-details gmap-right-rail"`,
		`data-rail-tab="inspector"`,
		`data-rail-tab="evidence"`,
		`data-rail-tab="config"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37ak-graph-native-tabs-contract-impl: legacy right-rail markup %q must remain (this tranche does not retire the letterbox)", want)
		}
	}
	// Drawer + inspector modules remain served.
	for _, asset := range []string{
		"/explorer/assets/js/graph/graph-drawer.js",
		"/explorer/assets/js/graph/graph-inspector.js",
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37ak-graph-native-tabs-contract-impl: %q must remain served", asset)
		}
	}
}

// ── S. Engine isolation guard ──────────────────────────────────────

func TestExplorer_D37ak_EngineSurfaceUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Pin a few load-bearing engine markers; if D37ak silently
	// touched any of these, the markers would shift.
	engineJs := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js")
	for _, want := range []string{
		"function _runFitPipeline(source, phase)",
		"function _onContainerResize()",
		"function _armInitialSafetyCap()",
		"DIAG_ENGINE_INITIAL_REVEAL",
		"INITIAL_FIT_SAFETY_MS",
	} {
		if !strings.Contains(engineJs, want) {
			t.Errorf("D37ak-graph-native-tabs-contract-impl: engine marker %q must remain present (this tranche does not modify the engine)", want)
		}
	}
	// And the engine itself must not now reference the new tab
	// contract helpers — those live on the shell only.
	for _, banned := range []string{
		"graphSelectedObjectPane.getTabs",
		"graphSelectedObjectPane.setActiveTab",
		"AUTHORITY_TAB_CONFIG",
	} {
		if strings.Contains(engineJs, banned) {
			t.Errorf("D37ak-graph-native-tabs-contract-impl: engine must not couple to the tab contract (%q)", banned)
		}
	}
}

// ── T. Context provider untouched in this tranche ──────────────────

// TestExplorer_D37am_ContextProviderMigratedToTabsConfig is the
// lockstep successor to the D37ak-era `ContextProviderNotMigrated`
// pin. After D37am-context-tabs-config-impl, Context becomes the
// second formal consumer of the global tab contract (Authority was
// first). This test asserts the migration happened: Context still
// registers as the 'context' provider AND declares a `tabs:
// CONTEXT_TAB_CONFIG` field on the provider literal.
func TestExplorer_D37am_ContextProviderMigratedToTabsConfig(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	ctxJs := getExplorerAsset(t, srv, d37pAuth2ContextPaneAsset)

	// Context still registers as the 'context' provider.
	if !strings.Contains(ctxJs, "registerLensProvider('context'") {
		t.Errorf("D37am-context-tabs-config-impl: Context must remain registered")
	}
	// Context now declares the formal tabs config.
	if !strings.Contains(ctxJs, "tabs: CONTEXT_TAB_CONFIG") {
		t.Errorf("D37am-context-tabs-config-impl: Context provider must declare `tabs: CONTEXT_TAB_CONFIG` (migration completed)")
	}
}
