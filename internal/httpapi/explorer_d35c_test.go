package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35c-adopt-native-context-as-baseline
//
// Adopts the existing native Context Graph as the baseline active
// renderer occupying `.midas-graph-renderer-slot`. This is an
// OWNERSHIP BRIDGE, not a renderer migration:
//   • Native DOM is not recreated, cleared, moved, or destroyed.
//   • Authority Cytoscape stays on its existing activation path.
//   • Context Cytoscape spike stays on its existing activation path.
//   • Existing body activation flags and `#gmap-canvas` hide rules
//     remain in place.
//
// D35c introduces ONE new host API method:
//   adoptExisting(rendererId, handle?)
//   - Validates rendererId.
//   - Records active renderer id + handle.
//   - Defaults handle to `{ destroy: no-op }` if not supplied.
//   - Idempotent for the same id.
//   - Preserves the single-active-renderer discipline of activate().
//   - Does NOT call factory.mount.
//   - Does NOT mutate the slot's contents.
//
// At the end of the host module's IIFE, the host auto-adopts
// 'native-context' as the baseline. After page load:
//   window.MIDASExplorerGraph.viewport.getActiveRendererId() === 'native-context'

const d35cHostAssetPath = "/explorer/assets/js/graph/graph-viewport.js"

// TestExplorer_D35cHost_AdoptExistingApiSurface pins the new
// public API method + the export shape.
func TestExplorer_D35cHost_AdoptExistingApiSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35cHostAssetPath)

	for _, want := range []string{
		// Function definition.
		"function adoptExisting(rendererId, handle)",
		// Export line.
		"adoptExisting:        adoptExisting",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35c: host must expose adoptExisting — missing %q", want)
		}
	}
}

// TestExplorer_D35cHost_AdoptExistingLifecycle pins the function's
// internal contract: validation, idempotency, single-active
// discipline, default no-op destroy, no mount call.
func TestExplorer_D35cHost_AdoptExistingLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35cHostAssetPath)

	start := strings.Index(js, "function adoptExisting(rendererId, handle)")
	if start < 0 {
		t.Fatal("D35c: adoptExisting definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D35c: cannot bound adoptExisting body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		// Validation.
		"typeof rendererId !== 'string' || rendererId.length === 0",
		// Idempotent: same id → no-op success.
		"if (_activeId === rendererId && _activeHandle)",
		// Single-active discipline: deactivate previous before adopting.
		"if (_activeHandle) {",
		"deactivate();",
		// Default no-op destroy when no handle (or no destroy fn).
		"|| typeof handle.destroy !== 'function'",
		"handle = { destroy: function () { /* no-op */ } };",
		// Records new active state.
		"_activeId     = rendererId;",
		"_activeHandle = handle;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35c: adoptExisting must include %q", want)
		}
	}

	// Negative — adoptExisting must NOT call factory.mount.
	if strings.Contains(body, "factory.mount(") {
		t.Error("D35c: adoptExisting must NOT call factory.mount (no mounting; ownership bridge only)")
	}
}

// TestExplorer_D35cHost_AdoptExistingDoesNotMutateSlot pins that
// adoptExisting performs no DOM mutation on the slot. Any token
// that would modify the slot's children is banned in the function
// body.
func TestExplorer_D35cHost_AdoptExistingDoesNotMutateSlot(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35cHostAssetPath)

	start := strings.Index(js, "function adoptExisting(rendererId, handle)")
	if start < 0 {
		t.Fatal("D35c: adoptExisting definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D35c: cannot bound adoptExisting body")
	}
	body := js[start : start+end]

	for _, banned := range []string{
		"slotEl.appendChild",
		"slotEl.removeChild",
		"slotEl.replaceChild",
		"slotEl.innerHTML",
		"slotEl.textContent",
		// Generic DOM-mutation tokens that could only mean "touch
		// the slot's contents."
		".appendChild(",
		".removeChild(",
		".replaceChild(",
		".innerHTML",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D35c: adoptExisting body must NOT contain %q (ownership bridge — no DOM mutation)", banned)
		}
	}
}

// TestExplorer_D35cHost_DeactivateAfterAdoptIsIdempotent pins that
// deactivate handles the post-adoptExisting state correctly: the
// no-op destroy is invoked exactly once, state is cleared,
// repeated calls are safe.
//
// This is a static-analysis assertion — the deactivate body was
// already shaped to be idempotent in D35b; D35c just relies on
// that property. We re-pin the relevant deactivate guards here so
// any future regression is caught.
func TestExplorer_D35cHost_DeactivateAfterAdoptIsIdempotent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35cHostAssetPath)

	start := strings.Index(js, "function deactivate()")
	if start < 0 {
		t.Fatal("D35c: deactivate definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D35c: cannot bound deactivate body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		// Tolerate the no-active-renderer case (e.g. deactivate
		// called twice).
		"if (!_activeHandle)",
		"_activeId = null;",
		// Clears active state BEFORE invoking destroy, so a
		// re-entrant destroy() can't see stale active state.
		"_activeHandle = null;",
		// Exactly-once destroy invocation.
		"h.destroy()",
		// Defensive: tolerate destroy() that throws. The catch
		// keyword sits on its own line in the source (the `}` from
		// the inline try is on the previous line), so we pin the
		// catch keyword + opener rather than `} catch`.
		"catch (_) {",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35c: deactivate must include %q (idempotency contract)", want)
		}
	}
}

// TestExplorer_D35cNative_RegistersBaselineRenderer pins the
// auto-registration that runs at the end of the host's IIFE. The
// host calls `adoptExisting('native-context')` so any subsequent
// activation sees a clean baseline.
func TestExplorer_D35cNative_RegistersBaselineRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35cHostAssetPath)

	// Baseline-adoption helper exists, runs at IIFE end, and uses
	// the canonical 'native-context' id.
	for _, want := range []string{
		"function _adoptNativeContextBaseline()",
		"adoptExisting('native-context')",
		// The helper is invoked synchronously inside the IIFE.
		"_adoptNativeContextBaseline();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35c: baseline registration must include %q", want)
		}
	}
}

// TestExplorer_D35cNative_RegistrationIsDefensive pins that the
// baseline registration cannot break page load: the helper is
// wrapped in try/catch, checks the slot exists, and respects any
// prior `activate(...)` call.
func TestExplorer_D35cNative_RegistrationIsDefensive(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35cHostAssetPath)

	start := strings.Index(js, "function _adoptNativeContextBaseline()")
	if start < 0 {
		t.Fatal("D35c: _adoptNativeContextBaseline definition missing")
	}
	end := strings.Index(js[start:], "\n  }")
	if end < 0 {
		t.Fatal("D35c: cannot bound _adoptNativeContextBaseline body")
	}
	body := js[start : start+end]

	for _, want := range []string{
		// Defensive try/catch wraps the whole body.
		"try {",
		"} catch (_) {",
		// Respect an earlier activate(): if `_activeId` is set,
		// don't override.
		"if (_activeId) return;",
		// Slot must be present.
		"if (!getRendererSlotEl()) return;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35c: baseline registration must be defensive — missing %q", want)
		}
	}
}

// TestExplorer_D35cNative_DoesNotRecreateGraphDom pins that the
// native graph DOM in index.html survives untouched by D35c. The
// host adopts ownership of the existing slot contents; it does
// not regenerate them.
func TestExplorer_D35cNative_DoesNotRecreateGraphDom(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Existing native graph DOM tokens — D35c must not remove or
	// regenerate them.
	for _, want := range []string{
		`<div class="governance-map-canvas-scroll">`,
		`<div id="gmap-canvas" class="governance-map-canvas"`,
		`<div id="gmap-scene" class="governance-map-scene">`,
		`<svg id="gmap-svg" class="governance-map-svg"`,
		`viewBox="0 0 1180 720"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35c: native graph DOM %q must remain in index.html", want)
		}
	}

	// The host module body must NOT contain DOM-creation helpers
	// targeting the graph elements (it adopts, never builds).
	js := getExplorerAsset(t, srv, d35cHostAssetPath)
	for _, banned := range []string{
		"document.createElement('div')",
		"document.createElement(\"div\")",
		"createElement('canvas')",
		"createElement('svg')",
		// Any innerHTML write would mutate the slot.
		".innerHTML =",
		".innerHTML=",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D35c: host must NOT contain %q (ownership bridge — no DOM creation)", banned)
		}
	}
}

// TestExplorer_D35c_NoAuthorityMigration — RETIRED.
//
// D35d-port-authority-cytoscape-to-graphviewport intentionally
// migrated Authority Cytoscape onto the GraphViewport host. The
// non-goal this test enforced ("no Authority migration in D35c")
// was correct AT THE TIME OF D35c, but is now superseded.
//
// Coverage moved to:
//   • TestExplorer_D35dAuthority_ActivatesThroughViewportHost
//   • TestExplorer_D35dAuthority_RendererFactoryLifecycle
//   • TestExplorer_D35dAuthority_MountsInsideRendererSlot
//   • TestExplorer_D35dAuthority_DoesNotAppendToLegacyScrollSurface
//
// Authority body-class survival is now pinned in
// `TestExplorer_D35bHost_NoProductionRendererMigration` and the
// transitional-debt note in D35d's report.
func TestExplorer_D35c_NoAuthorityMigration(t *testing.T) {
	t.Skip("D35d: superseded — Authority migrated onto GraphViewport host. See TestExplorer_D35dAuthority_*.")
}

// TestExplorer_D35c_NoContextCytoscapeMigration — SUPERSEDED.
//
// D35e-port-context-cytoscape-to-graphviewport intentionally
// migrated the Context Cytoscape spike onto the GraphViewport
// host. The non-goal this test enforced ("no Context migration in
// D35c") was correct AT THE TIME OF D35c, but is now superseded.
//
// Coverage moved to:
//   • TestExplorer_D35eContext_ActivatesThroughViewportHost
//   • TestExplorer_D35eContext_RendererFactoryLifecycle
//   • TestExplorer_D35eContext_MountsInsideRendererSlot
//   • TestExplorer_D35eContext_DoesNotAppendToLegacyScrollSurface
func TestExplorer_D35c_NoContextCytoscapeMigration(t *testing.T) {
	t.Skip("D35e: superseded — Context Cytoscape migrated onto GraphViewport host. See TestExplorer_D35eContext_*.")
}

// TestExplorer_D35c_ExistingActivationFlagsPreserved pins that
// every load-bearing structural identifier the prior tranches
// relied on is still in place.
//
// D35f-retire-transitional-renderer-debt — body-class activation
// flags (`body.cytoscape-poc-active`, `body.context-cy-spike-
// active`) were retired in favour of host-owned renderer identity
// on `.midas-graph-viewport[data-active-renderer="…"]`. The
// `#gmap-canvas` hide rule survives, re-keyed onto that attribute.
func TestExplorer_D35c_ExistingActivationFlagsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// index.html — structural IDs/classes survive.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`id="gmap-canvas"`,
		`class="governance-map-canvas-scroll"`,
		`class="governance-map-body"`,
		`class="midas-graph-viewport"`,
		`class="midas-graph-renderer-slot"`,
		// Production graph-renderer.js still loaded (the host does
		// not replace it).
		`src="/explorer/assets/js/graph/graph-renderer.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35c: structural identifier %q must remain in index.html", want)
		}
	}

	// Authority CSS — #gmap-canvas hide re-keyed onto host renderer
	// identity (D35f).
	authPocCSS := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	for _, want := range []string{
		`.midas-graph-viewport[data-active-renderer="authority"]`,
		`.midas-graph-viewport[data-active-renderer="authority"] #gmap-canvas`,
		"display: none !important;",
	} {
		if !strings.Contains(authPocCSS, want) {
			t.Errorf("D35f: Authority Cytoscape CSS must keep %q (host-keyed)", want)
		}
	}

	// Context spike CSS — canvas hide re-keyed onto host renderer
	// identity; overlay overflow:hidden REMOVED in D35e (the
	// architectural fix); mount-level overflow:hidden survives as
	// Cytoscape canvas-discipline.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	for _, want := range []string{
		`.midas-graph-viewport[data-active-renderer="context-cytoscape"]`,
		`.midas-graph-viewport[data-active-renderer="context-cytoscape"] #gmap-canvas`,
		"display: none !important",
		// Mount-level overflow:hidden remains.
		"overflow: hidden",
	} {
		if !strings.Contains(spikeCSS, want) {
			t.Errorf("D35f: Context spike CSS must keep %q (host-keyed)", want)
		}
	}
}
