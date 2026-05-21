package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35b-midas-graph-viewport-js-host
//
// JS host abstraction that consumes the D35a structural foundation.
// Adds `window.MIDASExplorerGraph.viewport` with:
//   • DOM lookup:  getViewportEl, getRendererSlotEl, getViewportRect
//   • Chrome-aware: getSafeArea
//   • Subscription: onResize (ResizeObserver with window-resize fallback)
//   • Lifecycle:   activate(id, factory), deactivate(), getActiveRendererId
//
// D35b is structural API only. No renderer is migrated onto the host
// in this tranche. Native Context, Authority Cytoscape, and the
// Context Cytoscape spike continue to activate via their existing
// mechanics.
//
// Tests below pin:
//   1. The host module is served and loaded by index.html.
//   2. The module exports the full required API surface.
//   3. The host queries the D35a class names (viewport + slot).
//   4. The safe-area logic references the three chrome classes.
//   5. The host installs a ResizeObserver with a window-resize fallback.
//   6. activate/deactivate implement a mount → destroy lifecycle.
//   7. NO production renderer calls `viewport.activate` in this tranche.
//   8. Existing renderer activation flags + IDs remain unchanged.

const d35bHostAssetPath = "/explorer/assets/js/graph/graph-viewport.js"

// TestExplorer_D35bHost_ModuleServedAndLinked pins that the module
// is served by the explorer asset handler AND linked from
// index.html. Load order matters: the host must load BEFORE the
// renderer modules so they can rely on it.
func TestExplorer_D35bHost_ModuleServedAndLinked(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Module asset is served.
	js := getExplorerAsset(t, srv, d35bHostAssetPath)
	if len(js) == 0 {
		t.Fatal("D35b: host module asset is empty")
	}

	// index.html has a <script> tag for the host AND it comes BEFORE
	// the production graph-renderer script.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	hostIdx := strings.Index(body, `src="`+d35bHostAssetPath+`"`)
	rendererIdx := strings.Index(body, `src="/explorer/assets/js/graph/graph-renderer.js"`)
	if hostIdx < 0 {
		t.Fatal("D35b: graph-viewport.js <script> tag missing from index.html")
	}
	if rendererIdx < 0 {
		t.Fatal("D35b: graph-renderer.js <script> tag missing from index.html")
	}
	if hostIdx > rendererIdx {
		t.Error("D35b: graph-viewport.js must be linked BEFORE graph-renderer.js so future renderers can rely on the host at init")
	}
}

// TestExplorer_D35bHost_NamespaceAndApiSurface pins the public
// namespace and every required API method name.
func TestExplorer_D35bHost_NamespaceAndApiSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35bHostAssetPath)

	// Standard MIDASExplorerGraph namespace bootstrap.
	if !strings.Contains(js, "window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};") {
		t.Error("D35b: host must use the standard `window.MIDASExplorerGraph = ... || {}` namespace bootstrap")
	}
	if !strings.Contains(js, "window.MIDASExplorerGraph.viewport = {") {
		t.Error("D35b: host must export under `window.MIDASExplorerGraph.viewport`")
	}

	for _, want := range []string{
		// Public API methods exported on the namespace.
		"getViewportEl:        getViewportEl",
		"getRendererSlotEl:    getRendererSlotEl",
		"getViewportRect:      getViewportRect",
		"getSafeArea:          getSafeArea",
		"onResize:             onResize",
		"activate:             activate",
		"deactivate:           deactivate",
		"getActiveRendererId:  getActiveRendererId",
		// Required function definitions.
		"function getViewportEl(",
		"function getRendererSlotEl(",
		"function getViewportRect(",
		"function getSafeArea(",
		"function onResize(",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35b: host public surface must include %q", want)
		}
	}
}

// TestExplorer_D35bHost_QueriesD35aClassNames pins that the host
// queries the EXACT class names introduced in D35a. If D35a renames
// the wrapper, the host must follow.
func TestExplorer_D35bHost_QueriesD35aClassNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35bHostAssetPath)

	for _, want := range []string{
		`var VIEWPORT_CLASS      = 'midas-graph-viewport';`,
		`var RENDERER_SLOT_CLASS = 'midas-graph-renderer-slot';`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35b: host must query D35a class %q verbatim", want)
		}
	}
}

// TestExplorer_D35bHost_SafeAreaReferencesChromeClasses pins that
// the safe-area calculation references the three chrome classes.
// Other class names (e.g. tooltip) are intentionally NOT in this
// list — the tooltip is a tiny element that should not influence
// fit padding.
func TestExplorer_D35bHost_SafeAreaReferencesChromeClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35bHostAssetPath)

	// Pin the constant declaration AND the chrome class names.
	if !strings.Contains(js, "var CHROME_CLASSES = [") {
		t.Error("D35b: host must declare CHROME_CLASSES array for safe-area calculation")
	}
	for _, want := range []string{
		`'gmap-mode-rail'`,
		`'gmap-camera-cluster'`,
		`'gmap-legend-overlay'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35b: safe-area calculation must reference %q", want)
		}
	}
	// Negative pin — the host should NOT reference the tooltip in
	// safe-area calculation (it's a transient hover element).
	if strings.Contains(js, `'gmap-connector-tooltip'`) {
		t.Error("D35b: safe-area must not include `gmap-connector-tooltip` (transient hover element)")
	}
}

// TestExplorer_D35bHost_ResizeObserverWithFallback pins the resize
// subscription strategy: ResizeObserver when available, window
// resize event as fallback.
func TestExplorer_D35bHost_ResizeObserverWithFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35bHostAssetPath)

	for _, want := range []string{
		// Capability check before constructing the observer.
		"typeof window.ResizeObserver === 'function'",
		"new window.ResizeObserver(",
		// Fallback to window resize when ResizeObserver is missing
		// OR when the viewport element isn't in the DOM yet.
		"window.addEventListener('resize'",
		"window.removeEventListener('resize'",
		// Returns an unsubscribe function.
		"return function unsubscribe()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35b: onResize must include %q", want)
		}
	}
}

// TestExplorer_D35bHost_ActivateLifecycle pins the activate/destroy
// contract: activate calls factory.mount(slotEl, ctx), stores the
// returned handle, and deactivate calls handle.destroy().
func TestExplorer_D35bHost_ActivateLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35bHostAssetPath)

	// activate(): expected internal terms.
	actStart := strings.Index(js, "function activate(rendererId, factory)")
	if actStart < 0 {
		t.Fatal("D35b: activate function definition missing")
	}
	actEnd := strings.Index(js[actStart:], "\n  }")
	if actEnd < 0 {
		t.Fatal("D35b: cannot bound activate body")
	}
	actBody := js[actStart : actStart+actEnd]
	for _, want := range []string{
		// Deactivate any prior renderer before mounting the next.
		"deactivate();",
		// Build the ctx object handed to factory.mount.
		// D37s-viewport-fit-1-impl flip: ctx alignment widened to
		// accommodate the new `getUsableGraphRect` key. The keys
		// pinned here used to use 1-space alignment after the colon
		// (e.g. `getViewportRect: getViewportRect`); the alignment
		// is now wider (e.g. `getViewportRect:    getViewportRect`)
		// so the new strategic key sits in the same column as the
		// existing ones. The keys + values still exist; only the
		// whitespace after the colon changed.
		"viewportEl:",
		"slotEl:",
		"getViewportRect:    getViewportRect",
		"getSafeArea:        getSafeArea",
		"getUsableGraphRect: getUsableGraphRect",
		"onResize:           onResize",
		"hooks:",
		// Mount + store handle.
		"factory.mount(slotEl, ctx)",
		"_activeHandle =",
		"_activeId     = rendererId",
	} {
		if !strings.Contains(actBody, want) {
			t.Errorf("D35b: activate must include %q", want)
		}
	}

	// deactivate(): expected internal terms.
	deStart := strings.Index(js, "function deactivate()")
	if deStart < 0 {
		t.Fatal("D35b: deactivate function definition missing")
	}
	deEnd := strings.Index(js[deStart:], "\n  }")
	if deEnd < 0 {
		t.Fatal("D35b: cannot bound deactivate body")
	}
	deBody := js[deStart : deStart+deEnd]
	for _, want := range []string{
		// Idempotent: tolerate repeated calls.
		"if (!_activeHandle)",
		"_activeHandle = null;",
		"_activeId     = null;",
		// Exactly-once destroy.
		"h.destroy()",
	} {
		if !strings.Contains(deBody, want) {
			t.Errorf("D35b: deactivate must include %q", want)
		}
	}
}

// TestExplorer_D35bHost_DefensiveBehaviour pins the defensive coding
// style: getViewportEl returns null when absent; getViewportRect
// returns a zero-filled rect when viewport is absent; getSafeArea
// returns zeros when viewport is absent.
func TestExplorer_D35bHost_DefensiveBehaviour(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35bHostAssetPath)

	for _, want := range []string{
		// _byClass returns null when no element matches.
		"return (nodes && nodes.length > 0) ? nodes[0] : null;",
		// _rectOf returns a zero-filled rect for absent elements.
		`return { x: 0, y: 0, width: 0, height: 0, left: 0, top: 0, right: 0, bottom: 0 };`,
		// getSafeArea returns zeros when viewport is absent.
		"return { top: 0, right: 0, bottom: 0, left: 0 };",
		// Try/catch wraps DOM access throughout.
		"try {",
		"} catch (_) {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35b: defensive coding requires %q", want)
		}
	}
}

// TestExplorer_D35bHost_NoProductionRendererMigration pins the
// non-migration boundary at the time of each tranche. D35b
// originally asserted "NO renderer calls viewport.activate yet."
//
// D35d-port-authority-cytoscape-to-graphviewport explicitly
// migrated Authority Cytoscape onto the host (a deliberate
// supersede). This test now scopes the ban to the renderers that
// have NOT been migrated yet, so it continues to catch unintended
// migrations of Context Cytoscape spike, native Context modules,
// etc. — without flagging the intentional D35d Authority work.
//
// Removed from the ban list as of D35d:
//   • authority-cytoscape-poc.js (migrated — see D35d tests)
//   • authority-cytoscape-toolbar.js (still doesn't call activate,
//     but kept off the ban list since it's part of the Authority
//     module group)
//   • authority-graph-view.js (same)
//   • cytoscape-html-overlay.js (D34a Authority HTML overlay; not
//     yet migrated but in the Authority module group)
//
// Files still banned: native Context + Context Cytoscape spike
// (scheduled for D35d/D35e/D35f).
func TestExplorer_D35bHost_NoProductionRendererMigration(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rendererAssets := []string{
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
		// Authority modules: removed in D35d (Authority migration).
		// Context Cytoscape spike: removed in D35e (Context Cytoscape
		// migration). As of D35g, both modules legitimately call
		// `viewport.register('<id>', factory)` + `viewport.activateById(
		// '<id>')` instead of `viewport.activate(id, factory)`.
	}
	for _, path := range rendererAssets {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, ".viewport.activate(") ||
			strings.Contains(js, "viewport.activate(") {
			t.Errorf("non-goal violation: %s must NOT call viewport.activate (renderer not yet scheduled for host migration)", path)
		}
		// D35g — also forbid the registry-based activation API on
		// non-migrated production renderers.
		if strings.Contains(js, ".viewport.activateById(") ||
			strings.Contains(js, "viewport.activateById(") ||
			strings.Contains(js, ".viewport.register(") ||
			strings.Contains(js, "viewport.register(") {
			t.Errorf("non-goal violation: %s must NOT call viewport.activateById / viewport.register (renderer not yet scheduled for host migration)", path)
		}
	}

	// Existing activation flags / IDs remain in place.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`id="gmap-canvas"`,
		`class="governance-map-canvas-scroll"`,
		`class="midas-graph-viewport"`,
		`class="midas-graph-renderer-slot"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35b: structural identifier %q must remain in index.html", want)
		}
	}

	authPocCSS := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	if !strings.Contains(authPocCSS, "body.cytoscape-poc-active") {
		// D35d preserves this class as transitional debt (CSS rules
		// keyed on it — most notably `#gmap-canvas { display:none
		// !important }` — are not yet replaced by renderer-id
		// attributes). Scheduled for D35f removal.
		t.Error("Authority body class `body.cytoscape-poc-active` must survive (transitional debt; D35f will retire it)")
	}
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if !strings.Contains(spikeCSS, "body.context-cy-spike-active") {
		t.Error("Context spike body class `body.context-cy-spike-active` must survive (no migration in D35b/c/d; scheduled for D35e)")
	}
}

// TestExplorer_D35bHost_NoNewDependency pins that no third-party JS
// dependency was added by D35b. The host module must be a self-
// contained IIFE with no require/import.
func TestExplorer_D35bHost_NoNewDependency(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35bHostAssetPath)

	for _, banned := range []string{
		"require(",
		"import ",
		"from '",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D35b: host module must remain a self-contained IIFE — found %q", banned)
		}
	}
}
