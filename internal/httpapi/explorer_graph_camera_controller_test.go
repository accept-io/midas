package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-impl-3 — Shared Graph Camera Controller tests
//
// Pins the platform contract for the new
// `graph-platform/graph-camera-controller.js` module and the
// strategic Context renderer's spatial-mode integration:
//
//   • asset present, served, loaded after graph-stage.js, loaded
//     before context-cytoscape-renderer.js;
//   • public surface: create + locked zoom constants;
//   • instance surface: zoom / pan / fit / focus / reset / apply /
//     destroy / getTransform / subscribe;
//   • renderer-agnostic purity (no lens kinds, no engine APIs, no
//     legacy DOM ids, no drawer / tray / pane / Authority / backend
//     coupling);
//   • fit / focus consume StageModel + safe-area + viewport rect;
//   • zoom clamping to MIN_ZOOM / MAX_ZOOM;
//   • Context renderer wires camera ONLY in spatial mode, cleans up
//     on destroy, exposes `getCamera()` as a diagnostic;
//   • foundation preserved (legacy renderer, Authority, pane,
//     projection provider, default renderer, no temporary renderer
//     identities).

const (
	d37pImpl3CameraAsset    = "/explorer/assets/js/graph/graph-platform/graph-camera-controller.js"
	d37pImpl3StageAsset     = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37pImpl3RendererAsset  = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pImpl3CssAsset       = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37pImpl3LegacyCamera   = "/explorer/assets/js/graph/graph-camera.js"
	d37pImpl3LegacyView     = "/explorer/assets/js/graph/context/context-graph-view.js"
	d37pImpl3AuthorityModul = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37pImpl3PaneAsset      = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
)

// ── A. Asset presence + load order ───────────────────────────────────

func TestExplorer_D37pImpl3_GraphCamera_AssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)
	if len(js) == 0 {
		t.Fatal("D37p-impl-3: graph-camera-controller.js must be served")
	}
}

func TestExplorer_D37pImpl3_GraphCamera_ScriptTagWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-controller.js"`) {
		t.Errorf("D37p-impl-3: index.html must include <script> for graph-camera-controller.js")
	}
	if c := strings.Count(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-controller.js"`); c != 1 {
		t.Errorf("D37p-impl-3: graph-camera-controller.js must be included exactly once (found %d)", c)
	}
}

// TestExplorer_D37pImpl3_GraphCamera_LoadOrder pins:
//
//   graph-stage.js → graph-camera-controller.js → context-cytoscape-renderer.js
//
// so the camera can read StageModels at create time and the renderer
// can instantiate one at mount time. The check matches against the
// `src="…"` attribute of each <script> tag (filenames may also
// appear in comments).
func TestExplorer_D37pImpl3_GraphCamera_LoadOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	stageIdx    := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-stage.js"`)
	cameraIdx   := strings.Index(body, `src="/explorer/assets/js/graph/graph-platform/graph-camera-controller.js"`)
	rendererIdx := strings.Index(body, `src="/explorer/assets/js/graph/context/context-cytoscape-renderer.js"`)
	if stageIdx < 0 || cameraIdx < 0 || rendererIdx < 0 {
		t.Fatal("D37p-impl-3: stage, camera, and renderer <script> tags must all be present")
	}
	if stageIdx >= cameraIdx {
		t.Errorf("D37p-impl-3: graph-camera-controller.js must load AFTER graph-stage.js (stage idx=%d, camera idx=%d)", stageIdx, cameraIdx)
	}
	if cameraIdx >= rendererIdx {
		t.Errorf("D37p-impl-3: graph-camera-controller.js must load BEFORE context-cytoscape-renderer.js (camera idx=%d, renderer idx=%d)", cameraIdx, rendererIdx)
	}
}

// ── B. Public surface ────────────────────────────────────────────────

func TestExplorer_D37pImpl3_GraphCamera_PublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.graphCameraController",
		"create:",
		"_constants:",
		"DEFAULT_ZOOM",
		"MIN_ZOOM",
		"MAX_ZOOM",
		"ZOOM_STEP",
		"DEFAULT_FIT_PADDING",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: camera controller public surface must declare %q", want)
		}
	}
}

func TestExplorer_D37pImpl3_GraphCamera_LockedZoomBounds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, want := range []string{
		"var DEFAULT_ZOOM                = 1.0;",
		"var MIN_ZOOM                    = 0.5;",
		"var MAX_ZOOM                    = 2.0;",
		"var ZOOM_STEP                   = 1.25;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: locked zoom bound %q must appear in source", want)
		}
	}
}

// ── C. Instance surface ──────────────────────────────────────────────

func TestExplorer_D37pImpl3_GraphCamera_InstanceSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, want := range []string{
		"zoomIn:",
		"zoomOut:",
		"setZoom:",
		"getZoom:",
		"panBy:",
		"setPan:",
		"getPan:",
		"fit:",
		"reset:",
		"focusBounds:",
		"focusCard:",
		"apply:",
		"destroy:",
		"getTransform:",
		"subscribe:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: camera instance must expose %q", want)
		}
	}
}

// ── D. Transform model ───────────────────────────────────────────────

func TestExplorer_D37pImpl3_GraphCamera_TransformModelPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, want := range []string{
		"zoom:",
		"panX:",
		"panY:",
		"_clampZoom",
		"applyTransform",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: transform model token %q must appear", want)
		}
	}
}

func TestExplorer_D37pImpl3_GraphCamera_FitConsumesStageModelAndSafeArea(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, want := range []string{
		"getStageModel",
		"getSafeArea",
		"fitBounds",
		"getBoundingClientRect",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: fit() must consume %q", want)
		}
	}
}

// ── E. Renderer-agnostic purity ──────────────────────────────────────

func TestExplorer_D37pImpl3_GraphCamera_NoLensSpecificSelectors(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, banned := range []string{
		"#gmap-canvas",
		"#gmap-scene",
		"#gmap-svg",
		".gmap-node",
		".context-card",
		".context-renderer-stage",
		".midas-graph-viewport",
		"contextProjection",
		"contextSelectionBridge",
		"contextEvidenceTray",
		"contextSelectedObjectPane",
		"authority",
		"authorityWorkbench",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-3: camera controller must remain renderer-agnostic; found %q", banned)
		}
	}
}

func TestExplorer_D37pImpl3_GraphCamera_NoGraphEngineCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, banned := range []string{
		"cytoscape",
		"Cytoscape",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-3: camera controller must not couple to a graph engine; found %q", banned)
		}
	}
	if regexp.MustCompile(`\bcy\.`).MatchString(js) {
		t.Errorf("D37p-impl-3: camera controller must not reference a Cytoscape instance via `cy.<member>`")
	}
}

func TestExplorer_D37pImpl3_GraphCamera_NoDrawerOrTrayOrPaneCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, banned := range []string{
		".setName(",
		".setFields(",
		".setGovernance(",
		".setActions(",
		".setInlineActions(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-3: camera controller must not call drawer / pane setters; found %q", banned)
		}
	}
}

func TestExplorer_D37pImpl3_GraphCamera_NoBackendCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/",
		"MIDASExplorerAPI",
		"publishProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-3: camera controller must not couple to backend / projection; found %q", banned)
		}
	}
}

func TestExplorer_D37pImpl3_GraphCamera_NoGraphViewportLifecycle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, banned := range []string{
		"viewport.activate",
		"activateById",
		"viewport.register",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-3: camera controller must not enter GraphViewport lifecycle; found %q", banned)
		}
	}
}

func TestExplorer_D37pImpl3_GraphCamera_NoTemporaryRendererNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-3: camera controller must not introduce temporary renderer name %q", banned)
		}
	}
}

// TestExplorer_D37pImpl3_GraphCamera_NoAutoInit pins that the
// controller is pure — it does not register any DOMContentLoaded
// hook, timer, or animation-frame callback at module evaluation. The
// renderer creates camera instances; the controller does not bootstrap
// itself.
func TestExplorer_D37pImpl3_GraphCamera_NoAutoInit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3CameraAsset)

	for _, banned := range []string{
		"DOMContentLoaded",
		"window.addEventListener",
		"setTimeout(",
		"setInterval(",
		"requestAnimationFrame(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-3: camera controller must not register lifecycle hooks (%q)", banned)
		}
	}
}

// ── F. Context renderer integration ──────────────────────────────────

func TestExplorer_D37pImpl3_ContextRenderer_UsesCameraInSpatialPath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3RendererAsset)

	for _, want := range []string{
		"graphCameraController",
		"graphCameraController.create(",
		"_ensureCamera",
		"_destroyCamera",
		"_cameraTarget",
		"getCamera:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: strategic Context renderer must wire the shared camera (%q)", want)
		}
	}
}

func TestExplorer_D37pImpl3_ContextRenderer_CameraIsSpatialOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3RendererAsset)

	// The camera is created from the spatial render path only — the
	// _ensureCamera() call must live inside _renderSpatialFoundation,
	// not in the flow-layout helpers or _mount itself.
	spatialStart := strings.Index(js, "function _renderSpatialFoundation(")
	if spatialStart < 0 {
		t.Fatal("D37p-impl-3: spatial render function not found in renderer")
	}
	// Find next function declaration after the spatial path.
	spatialEnd := strings.Index(js[spatialStart+len("function _renderSpatialFoundation("):], "\n  function ")
	if spatialEnd < 0 {
		t.Fatal("D37p-impl-3: could not delimit the spatial render function body")
	}
	body := js[spatialStart : spatialStart+len("function _renderSpatialFoundation(")+spatialEnd]
	if !strings.Contains(body, "_ensureCamera()") {
		t.Errorf("D37p-impl-3: _ensureCamera() must be invoked from inside _renderSpatialFoundation")
	}

	// The non-spatial flow helpers must NOT carry camera calls.
	for _, helper := range []string{
		"function _renderMain(",
		"function _renderBandSection(",
		"function _renderGovernance(",
	} {
		h := strings.Index(js, helper)
		if h < 0 {
			continue
		}
		end := strings.Index(js[h+len(helper):], "\n  function ")
		if end < 0 {
			continue
		}
		region := js[h : h+len(helper)+end]
		for _, banned := range []string{
			"_ensureCamera()",
			"_camera.fit(",
			"_camera.zoomIn(",
			"_camera.zoomOut(",
		} {
			if strings.Contains(region, banned) {
				t.Errorf("D37p-impl-3: flow-layout helper near %q must not call %q", helper, banned)
			}
		}
	}
}

func TestExplorer_D37pImpl3_ContextRenderer_CameraTargetSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3RendererAsset)

	for _, want := range []string{
		"viewportEl:",
		"stageEl:",
		"getStageModel:",
		"getSafeArea:",
		"getSelectedCardId:",
		"getRootCardId:",
		"applyTransform:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: camera target surface must declare %q", want)
		}
	}
}

func TestExplorer_D37pImpl3_ContextRenderer_DestroyTearsDownCamera(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3RendererAsset)

	// _destroy() must invoke _destroyCamera() so a deactivation does
	// not leave a dangling camera instance.
	destroyStart := strings.Index(js, "function _destroy()")
	if destroyStart < 0 {
		t.Fatal("D37p-impl-3: renderer _destroy() not found")
	}
	destroyEnd := strings.Index(js[destroyStart:], "\n  }")
	if destroyEnd < 0 {
		t.Fatal("D37p-impl-3: renderer _destroy() body not delimited")
	}
	body := js[destroyStart : destroyStart+destroyEnd]
	if !strings.Contains(body, "_destroyCamera()") {
		t.Errorf("D37p-impl-3: _destroy() must call _destroyCamera()")
	}
}

func TestExplorer_D37pImpl3_ContextRenderer_AppliesTransformInline(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3RendererAsset)

	for _, want := range []string{
		"_stageEl.style.transformOrigin = 'top left'",
		"_stageEl.style.transform",
		"'translate(' +",
		"'px) scale(' +",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: applyTransform callback must write the inline CSS transform (%q)", want)
		}
	}
}

// ── G. CSS scoping ──────────────────────────────────────────────────

func TestExplorer_D37pImpl3_Css_TransformOriginScoped(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37pImpl3CssAsset)

	for _, want := range []string{
		"transform-origin: top left;",
		"will-change: transform;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37p-impl-3: spatial stage CSS must declare %q", want)
		}
	}

	// Every rule referencing transform-origin or will-change must
	// remain scoped under the strategic Context active-renderer
	// attribute (no global selectors).
	commentRE := regexp.MustCompile(`(?s)/\*.*?\*/`)
	clean := commentRE.ReplaceAllString(css, "")
	for _, rule := range strings.Split(clean, "}") {
		brace := strings.LastIndex(rule, "{")
		if brace < 0 {
			continue
		}
		body := rule[brace+1:]
		if !strings.Contains(body, "transform-origin") && !strings.Contains(body, "will-change") {
			continue
		}
		selector := strings.TrimSpace(rule[:brace])
		if !strings.HasPrefix(selector, `.midas-graph-viewport[data-active-renderer="context"]`) {
			t.Errorf("D37p-impl-3: transform-origin / will-change rule %q must be scoped under .midas-graph-viewport[data-active-renderer=\"context\"]", selector)
		}
	}
}

// ── H. Boundary preservation ─────────────────────────────────────────

func TestExplorer_D37pImpl3_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Legacy camera still served.
	if len(getExplorerAsset(t, srv, d37pImpl3LegacyCamera)) == 0 {
		t.Errorf("D37p-impl-3: legacy graph-camera.js must remain served")
	}
	// Legacy Context renderer still wired in markup.
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-graph-view.js"`) {
		t.Errorf("D37p-impl-3: legacy context-graph-view.js script tag must remain")
	}
	// Authority module still served + no Authority changes required.
	if len(getExplorerAsset(t, srv, d37pImpl3AuthorityModul)) == 0 {
		t.Errorf("D37p-impl-3: Authority Cytoscape module must remain served")
	}
	// Selected-Object Pane still wired + wrapper present.
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-selected-object-pane.js"`) {
		t.Errorf("D37p-impl-3: Selected-Object Pane script tag must remain")
	}
	if !strings.Contains(body, "gmap-context-selected-object-pane") {
		t.Errorf("D37p-impl-3: Selected-Object Pane wrapper must remain present")
	}
	// Projection provider still wired.
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-projection-provider.js"`) {
		t.Errorf("D37p-impl-3: context-projection-provider.js script tag must remain")
	}
	// Default renderer behaviour unchanged.
	rendererJS := getExplorerAsset(t, srv, d37pImpl3RendererAsset)
	if !strings.Contains(rendererJS, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37p-impl-3: strategic renderer canonical id must remain 'context'")
	}
	if !strings.Contains(rendererJS, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37p-impl-3: strategic activation query param must remain 'contextRenderer'")
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
			t.Errorf("D37p-impl-3: must not introduce temporary renderer name %q", banned)
		}
	}
}

func TestExplorer_D37pImpl3_LegacyCameraIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl3LegacyCamera)

	// Legacy camera continues to target the legacy DOM ids via
	// getElementById (raw '#'-prefixed selectors are not used by the
	// legacy camera; it goes through getElementById directly).
	for _, want := range []string{
		`getElementById('gmap-canvas')`,
		`getElementById('gmap-scene')`,
		"state.positions",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-3: legacy camera must still reference %q", want)
		}
	}
}

// ── I. Index.html line count ─────────────────────────────────────────

func TestExplorer_D37pImpl3_IndexHtmlWithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 7820 {
		t.Errorf("D37p-impl-3: index.html line count %d exceeds the existing 7820 ceiling — pin bump required", lines)
	}
}

// ── J. Sibling assets still served ───────────────────────────────────

func TestExplorer_D37pImpl3_PlatformAssetsServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, asset := range []string{
		d37pImpl3CameraAsset,
		d37pImpl3StageAsset,
	} {
		if len(getExplorerAsset(t, srv, asset)) == 0 {
			t.Errorf("D37p-impl-3: platform asset %q must be served", asset)
		}
	}
}
