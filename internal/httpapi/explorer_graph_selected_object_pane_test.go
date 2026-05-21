package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-pane-1 — Shared Selected-Object Pane Shell tests
//
// Pins the new `graph-platform/graph-selected-object-pane.js`
// shared shell module + matching CSS scaffold + Context provider
// integration:
//
//   • shared shell served + script tag + stylesheet link wired;
//   • shell loads after graph-selection-bridge.js and before
//     context-selected-object-pane.js;
//   • public surface (init / destroy / registerLensProvider /
//     unregisterLensProvider / getRegisteredLensIds / setActiveLens /
//     getActiveLens / getActiveProvider / open / close / toggle /
//     isOpen / setPaneMode / getPaneMode / render /
//     notifySelectionChanged / subscribe / _constants);
//   • locked PANE_MODES, EVENTS, DEFAULT_SECTIONS, STORAGE_KEY;
//   • provider registry semantics + active-lens tracking;
//   • subscription to graphSelectionBridge;
//   • shell never calls into contextSelectionBridge / contextEvidenceTray /
//     drawer setters / GraphViewport lifecycle;
//   • shell carries no lens-specific kind strings;
//   • CSS scoped to generic [data-selected-object-pane] only;
//   • Context provider registration in context-selected-object-pane.js;
//   • all D37o-impl-6 invariants survive (public facade, sections,
//     locked copy, wrapper);
//   • foundation preserved (drawer / evidence tray / Authority
//     canvas-edge / camera bus / selection bridge all intact).

const (
	d37pPane1ShellAsset       = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37pPane1ShellCss         = "/explorer/assets/css/graph-selected-object-pane.css"
	d37pPane1SelBridgeAsset   = "/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"
	d37pPane1CtxPaneAsset     = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
	d37pPane1CtxPaneCss       = "/explorer/assets/css/context-selected-object-pane.css"
	d37pPane1RendererAsset    = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pPane1CtxBridgeAsset   = "/explorer/assets/js/graph/context/context-selection-bridge.js"
	d37pPane1AuthorityPoc     = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37pPane1AuthorityEdge    = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37pPane1Drawer           = "/explorer/assets/js/graph/graph-drawer.js"
	d37pPane1Provider         = "/explorer/assets/js/graph/context/context-projection-provider.js"
	d37pPane1CameraBusAsset   = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
)

// ── A. Asset presence + load order ─────────────────────────────────

func TestExplorer_D37pPane1_Shell_AssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	if len(getExplorerAsset(t, srv, d37pPane1ShellAsset)) == 0 {
		t.Fatal("D37p-pane-1: graph-selected-object-pane.js must be served")
	}
	if len(getExplorerAsset(t, srv, d37pPane1ShellCss)) == 0 {
		t.Fatal("D37p-pane-1: graph-selected-object-pane.css must be served")
	}
}

func TestExplorer_D37pPane1_Shell_ScriptAndLinkTagsWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`src="/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"`,
		`href="/explorer/assets/css/graph-selected-object-pane.css"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37p-pane-1: index.html must include %q", want)
		}
		if c := strings.Count(body, want); c != 1 {
			t.Errorf("D37p-pane-1: %q must appear exactly once (found %d)", want, c)
		}
	}
}

// TestExplorer_D37pPane1_Shell_LoadOrder pins:
//
//   graph-selection-bridge.js → graph-selected-object-pane.js →
//   context-selected-object-pane.js
//
// so the Context pane can register itself as the 'context'
// provider against an already-initialised shell.
func TestExplorer_D37pPane1_Shell_LoadOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	bridgeIdx := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-selection-bridge.js"`)
	shellIdx  := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"`)
	ctxIdx    := strings.Index(body, `src="/explorer/assets/js/graph/context/context-selected-object-pane.js"`)
	if bridgeIdx < 0 || shellIdx < 0 || ctxIdx < 0 {
		t.Fatal("D37p-pane-1: bridge, shell, and Context pane scripts must all be present")
	}
	if bridgeIdx >= shellIdx {
		t.Errorf("D37p-pane-1: shared shell must load AFTER graph-selection-bridge.js (bridge=%d, shell=%d)", bridgeIdx, shellIdx)
	}
	if shellIdx >= ctxIdx {
		t.Errorf("D37p-pane-1: shared shell must load BEFORE context-selected-object-pane.js (shell=%d, ctx=%d)", shellIdx, ctxIdx)
	}
}

// ── B. Public surface ──────────────────────────────────────────────

func TestExplorer_D37pPane1_Shell_PublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.graphSelectedObjectPane",
		"init:",
		"destroy:",
		"registerLensProvider:",
		"unregisterLensProvider:",
		"getRegisteredLensIds:",
		"setActiveLens:",
		"getActiveLens:",
		"getActiveProvider:",
		"open:",
		"close:",
		"toggle:",
		"isOpen:",
		"setPaneMode:",
		"getPaneMode:",
		"render:",
		"notifySelectionChanged:",
		"subscribe:",
		"_constants:",
		"PANE_MODES:",
		"EVENTS:",
		"DEFAULT_SECTIONS:",
		"STORAGE_KEY:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-pane-1: shell public surface must declare %q", want)
		}
	}
}

// ── C. Locked constants ────────────────────────────────────────────

func TestExplorer_D37pPane1_Shell_LockedConstants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	if !strings.Contains(js, "['auto', 'pinned', 'hidden']") {
		t.Errorf("D37p-pane-1: PANE_MODES must equal ['auto','pinned','hidden']")
	}
	for _, want := range []string{
		"'provider_registered'",
		"'provider_unregistered'",
		"'active_lens_changed'",
		"'pane_opened'",
		"'pane_closed'",
		"'pane_mode_changed'",
		"'pane_rendered'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-pane-1: EVENTS must include %q", want)
		}
	}
	for _, want := range []string{
		"'summary'",
		"'details'",
		"'actions'",
		"'relationships'",
		"'evidence'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-pane-1: DEFAULT_SECTIONS must include %q", want)
		}
	}
	if !strings.Contains(js, "'midas.graph.selectedObjectPane.mode'") {
		t.Errorf("D37p-pane-1: STORAGE_KEY must equal 'midas.graph.selectedObjectPane.mode'")
	}
}

// ── D. Provider registry semantics ─────────────────────────────────

func TestExplorer_D37pPane1_Shell_ProviderRegistrySemantics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	if !strings.Contains(js, "var _providers") {
		t.Errorf("D37p-pane-1: shell must keep an internal provider registry")
	}
	if !strings.Contains(js, "_providers[lensId] = provider") {
		t.Errorf("D37p-pane-1: registerLensProvider must REPLACE the provider for an existing lens id")
	}
	if !strings.Contains(js, "delete _providers[lensId]") {
		t.Errorf("D37p-pane-1: unregisterLensProvider must remove the registry entry")
	}
	// Provider callbacks must be tolerant of errors — pin the
	// helper that wraps each call.
	if !strings.Contains(js, "_callProvider") {
		t.Errorf("D37p-pane-1: shell must wrap provider callback invocations in a defensive helper")
	}
	if !regexp.MustCompile(`(?s)try \{ return p\[method\]\.apply.*?\}\s*catch \(_\) \{ return null; \}`).MatchString(js) {
		t.Errorf("D37p-pane-1: _callProvider must catch provider errors and return null")
	}
}

// ── E. Active-lens tracking + open/close delegation ────────────────

func TestExplorer_D37pPane1_Shell_ActiveLensAndDelegation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	if !strings.Contains(js, "function setActiveLens") {
		t.Errorf("D37p-pane-1: shell must expose setActiveLens")
	}
	if !strings.Contains(js, "function getActiveLens") {
		t.Errorf("D37p-pane-1: shell must expose getActiveLens")
	}
	if !strings.Contains(js, "function getActiveProvider") {
		t.Errorf("D37p-pane-1: shell must expose getActiveProvider")
	}
	for _, want := range []string{
		`_callProvider('open'`,
		`_callProvider('close'`,
		`_callProvider('setPaneMode'`,
		`_callProvider('notifySelectionChanged'`,
		`_callProvider('render'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-pane-1: shell must delegate via the provider helper (%q)", want)
		}
	}
}

// ── F. Selection-bridge integration ────────────────────────────────

func TestExplorer_D37pPane1_Shell_BindsToSelectionBridge(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	if !strings.Contains(js, "graphSelectionBridge") {
		t.Errorf("D37p-pane-1: shell must reference graphSelectionBridge")
	}
	if !strings.Contains(js, "bridge.subscribe(") {
		t.Errorf("D37p-pane-1: shell must subscribe to graphSelectionBridge")
	}
	// Shell must NOT reach into the Context bridge directly.
	if strings.Contains(js, "contextSelectionBridge") {
		t.Errorf("D37p-pane-1: shell must not reference contextSelectionBridge directly")
	}
}

// ── G. Eventing ────────────────────────────────────────────────────

func TestExplorer_D37pPane1_Shell_EventingPattern(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	if !strings.Contains(js, "var _subscribers") {
		t.Errorf("D37p-pane-1: shell must keep a subscriber list")
	}
	if !strings.Contains(js, "function subscribe") {
		t.Errorf("D37p-pane-1: shell must expose subscribe")
	}
	if !regexp.MustCompile(`try \{ entry\.handler\(event\); \}\s*catch \(_\) \{`).MatchString(js) {
		t.Errorf("D37p-pane-1: subscriber dispatch must be wrapped in try/catch")
	}
}

// ── H. Purity ──────────────────────────────────────────────────────

func TestExplorer_D37pPane1_Shell_NoBackendOrEngineCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/",
		"MIDASExplorerAPI",
		"publishProjection",
		"viewport.activate",
		"activateById",
		"cytoscape",
		"Cytoscape",
		".setName(",
		".setFields(",
		".setGovernance(",
		".setActions(",
		".setInlineActions(",
		"contextEvidenceTray",
		"authority-canvas-edge",
		"authorityWorkbench",
		"#gmap-canvas",
		"#gmap-svg",
		"#gmap-scene",
		".gmap-node",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-pane-1: shell must not couple to %q", banned)
		}
	}
	if regexp.MustCompile(`\bcy\.`).MatchString(js) {
		t.Errorf("D37p-pane-1: shell must not access a graph-engine instance via cy.<member>")
	}
}

func TestExplorer_D37pPane1_Shell_NoLensSpecificKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	for _, banned := range []string{
		"business_service",
		"decision_surface",
		"ai_system",
		"authority_profile",
		"authority_grant",
		"fail_mode_policy",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-pane-1: shell must remain lens-agnostic; found kind %q", banned)
		}
	}
}

func TestExplorer_D37pPane1_Shell_NoTemporaryRendererNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1ShellAsset)

	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-pane-1: shell must not introduce temporary renderer name %q", banned)
		}
	}
}

// ── I. CSS scoping ─────────────────────────────────────────────────

func TestExplorer_D37pPane1_SharedCss_GenericScoping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pPane1ShellCss)

	// Generic attribute selectors only — no Context-specific class
	// names, no Authority canvas-edge selectors, no drawer selectors.
	for _, want := range []string{
		"[data-selected-object-pane]",
		"[data-selected-object-pane-header]",
		"[data-selected-object-pane-body]",
		"[data-selected-object-pane-footer]",
		"[data-selected-object-pane-section]",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37p-pane-1: shared CSS must declare generic selector %q", want)
		}
	}
	for _, banned := range []string{
		".gmap-context-selected-object-pane",
		"data-context-selected-object-pane",
		"gmap-canvas-edge-tabs",
		"gmap-right-rail",
		"gmap-details",
		"gmap-evidence-tray",
		"context-renderer-",
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D37p-pane-1: shared CSS must not reference lens-specific / drawer / tray selector %q", banned)
		}
	}
}

// TestExplorer_D37pPane1_SharedCss_ScopedUnderViewport pins that
// every shared rule starts with `.midas-graph-viewport ` so the
// shell styles don't leak outside the graph workspace.
func TestExplorer_D37pPane1_SharedCss_ScopedUnderViewport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pPane1ShellCss)

	commentRE := regexp.MustCompile(`(?s)/\*.*?\*/`)
	clean := commentRE.ReplaceAllString(css, "")
	rules := strings.Split(clean, "}")
	found := 0
	for _, rule := range rules {
		brace := strings.LastIndex(rule, "{")
		if brace < 0 {
			continue
		}
		selector := strings.TrimSpace(rule[:brace])
		if selector == "" {
			continue
		}
		found++
		if !strings.HasPrefix(selector, ".midas-graph-viewport") {
			t.Errorf("D37p-pane-1: shared CSS selector %q must be scoped under .midas-graph-viewport", selector)
		}
	}
	if found == 0 {
		t.Fatal("D37p-pane-1: shared CSS file declared no rules")
	}
}

// ── J. Context provider integration ────────────────────────────────

func TestExplorer_D37pPane1_ContextProvider_Registers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1CtxPaneAsset)

	if !strings.Contains(js, "graphSelectedObjectPane") {
		t.Errorf("D37p-pane-1: Context pane must reference graphSelectedObjectPane")
	}
	if !strings.Contains(js, "shell.registerLensProvider('context'") {
		t.Errorf("D37p-pane-1: Context pane must register itself as the 'context' provider on the shared shell")
	}
	if !strings.Contains(js, "_registerWithSharedShell") {
		t.Errorf("D37p-pane-1: Context pane must declare a shared-shell registration helper")
	}
	if !strings.Contains(js, "shell.setActiveLens('context')") {
		t.Errorf("D37p-pane-1: Context pane must set the shared shell's active lens to 'context'")
	}
	// Provider callback shape — pin every callback name the shell
	// can route through. (Substrings are whitespace-tolerant: the
	// exact field-alignment padding inside the provider literal is
	// not load-bearing.)
	for _, want := range []string{
		"open:",
		"close:",
		"toggle:",
		"isOpen:",
		"setPaneMode:",
		"getPaneMode:",
		"notifySelectionChanged",
		"sections:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-pane-1: Context provider must declare %q", want)
		}
	}
	if !regexp.MustCompile(`copy:\s*COPY`).MatchString(js) {
		t.Errorf("D37p-pane-1: Context provider must expose its locked COPY object")
	}
}

func TestExplorer_D37pPane1_ContextProvider_PreservesPublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1CtxPaneAsset)

	// The Context pane facade keys are unchanged from D37o-impl-6.
	for _, want := range []string{
		"window.MIDASExplorerGraph.contextSelectedObjectPane",
		"init:        init",
		"destroy:     destroy",
		"open:        open",
		"close:       close",
		"toggle:      toggle",
		"isOpen:      isOpen",
		"setPaneMode: setPaneMode",
		"getPaneMode: getPaneMode",
		"_constants:",
		"PANE_MODES:  PANE_MODES.slice()",
		"SECTION_IDS: SECTION_IDS.slice()",
		"COPY:        COPY",
		"STORAGE_KEY: STORAGE_KEY",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-pane-1: Context facade must still expose %q", want)
		}
	}
}

func TestExplorer_D37pPane1_ContextProvider_RetainsSectionsAndCopy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pPane1CtxPaneAsset)

	// Existing D37o-impl-6 section renderers must remain.
	for _, want := range []string{
		"function _renderSummary",
		"function _renderDetails",
		"function _renderActions",
		"function _renderRelationships",
		"function _renderEvidence",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-pane-1: Context provider must retain D37o-impl-6 section renderer %q", want)
		}
	}
	// Locked copy strings remain (D37o-impl-6-copy-fix).
	for _, want := range []string{
		"noSelection:        'Select an object to inspect it.',",
		"noRelationships:    'No relationships for the selected object.',",
		"noDetails:          'No primary details available for this object.',",
		"evidenceDeferral:   'Detailed evidence remains available in the bottom Evidence tab.',",
		"closeButtonLabel:   'Close selected-object pane',",
		"paneAriaLabel:      'Selected object',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-pane-1: Context locked copy must remain (%q)", want)
		}
	}
}

// ── K. Foundation preservation ─────────────────────────────────────

func TestExplorer_D37pPane1_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Existing platform / lens / drawer / tray assets still served.
	for _, asset := range []string{
		d37pPane1SelBridgeAsset,
		d37pPane1CtxBridgeAsset,
		d37pPane1RendererAsset,
		d37pPane1AuthorityPoc,
		d37pPane1AuthorityEdge,
		d37pPane1Drawer,
		d37pPane1Provider,
		d37pPane1CameraBusAsset,
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37p-pane-1: foundation asset %q must remain served", asset)
		}
	}

	// Strategic Context renderer identity unchanged.
	rendererJS := getExplorerAsset(t, srv, d37pPane1RendererAsset)
	if !strings.Contains(rendererJS, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37p-pane-1: strategic Context renderer canonical id must remain 'context'")
	}

	// Wrappers + markup intact.
	for _, want := range []string{
		"gmap-context-selected-object-pane",  // Context pane wrapper
		"data-context-selected-object-pane",
		"gmap-evidence-tray",                  // bottom tray
		"gmap-details",                        // right drawer
		"gmap-canvas-edge-tabs",               // Authority canvas-edge
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37p-pane-1: existing wrapper / markup %q must remain present", want)
		}
	}

	// No temporary renderer identities introduced.
	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37p-pane-1: must not introduce temporary renderer name %q", banned)
		}
	}
}

// TestExplorer_D37pPane1_AuthorityCanvasEdgeMigrated converts the
// previous temporal pin (D37p-pane-1 era: "Authority must not
// reference graphSelectedObjectPane yet") into its now-correct
// inverse following D37p-authority-2-impl. The Authority canvas-edge
// tabs module is the second lens (after Context) to register a
// provider with the shared shell. The Authority Cytoscape renderer
// module itself remains uncoupled from the shell — only the
// canvas-edge tabs facade owns the provider seam, which keeps the
// shell ↔ renderer boundary clean.
func TestExplorer_D37pPane1_AuthorityCanvasEdgeMigrated(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	edgeJs := getExplorerAsset(t, srv, d37pPane1AuthorityEdge)
	if !strings.Contains(edgeJs, "graphSelectedObjectPane") {
		t.Errorf("D37p-pane-1: authority-canvas-edge-tabs.js must reference graphSelectedObjectPane (migrated by D37p-authority-2-impl)")
	}
	if !strings.Contains(edgeJs, "registerLensProvider('authority'") {
		t.Errorf("D37p-pane-1: authority-canvas-edge-tabs.js must register itself as the 'authority' provider on graphSelectedObjectPane (migrated by D37p-authority-2-impl)")
	}

	// The Authority Cytoscape renderer module itself remains
	// uncoupled from the shared pane shell — only the canvas-edge
	// tabs facade owns the provider seam.
	pocJs := getExplorerAsset(t, srv, d37pPane1AuthorityPoc)
	if strings.Contains(pocJs, "graphSelectedObjectPane") {
		t.Errorf("D37p-pane-1: authority-cytoscape-poc.js must NOT reference graphSelectedObjectPane (provider seam belongs to the canvas-edge tabs facade)")
	}
}

// ── L. Index.html line count ───────────────────────────────────────

func TestExplorer_D37pPane1_IndexHtmlWithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 7820 {
		t.Errorf("D37p-pane-1: index.html line count %d exceeds the existing 7820 ceiling", lines)
	}
}
