package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37q-viewport-3-impl — Context Renderer Activation Contract Alignment.
//
// Before D37q-viewport-3-impl, the strategic Context renderer registered
// a `context` camera-bus delegate only when spatial mode was active
// (the delegate registration lived inside `_ensureCamera()`, which is
// only called from the spatial-foundation render path). Non-spatial
// strategic Context therefore left the camera bus with no `context`
// delegate at all; toolbar dispatches against the `context` lens
// silently no-opped at the bus's dispatch layer.
//
// This tranche installs a fallback delegate at `_mount()` so the
// strategic Context renderer always satisfies the camera-bus contract
// when active. Spatial-mode behaviour is preserved verbatim: the
// existing `_ensureCamera()` → `_registerCameraBusDelegate()` path
// upgrades the fallback to a live-camera delegate via the bus's
// REPLACE policy. Cleanup is unchanged: `_destroyCamera()` already
// calls `_unregisterCameraBusDelegate()`.
//
// These tests pin the contract.

const (
	d37qV3ContextRendererAsset    = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37qV3ContextConnectorPainter = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37qV3CameraBusAsset          = "/explorer/assets/js/graph/graph-platform/graph-camera-bus.js"
	d37qV3CameraToolbarAdapter    = "/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js"
	d37qV3AuthorityPocAsset       = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37qV3ViewportAsset           = "/explorer/assets/js/graph/graph-viewport.js"
	d37qV3StageAsset              = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
)

// ── 1. Strategic Context still registers with GraphViewport ────────

func TestExplorer_D37qViewport3_ContextRendererRegistersViewport(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3ContextRendererAsset)

	for _, want := range []string{
		"var RENDERER_ID    = 'context';",
		"g.viewport.register(RENDERER_ID, _factoryFor())",
		"function _factoryFor()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-3-impl: strategic Context renderer must still register with GraphViewport (%q)", want)
		}
	}
}

// ── 2. Delegate registration no longer spatial-gated ──────────────

// TestExplorer_D37qViewport3_ContextCameraDelegateRegistrationNotSpatialGated
// pins that the strategic Context renderer's camera-bus delegate
// registration fires at `_mount()` time, not solely from the
// spatial-mode `_ensureCamera()` path.
func TestExplorer_D37qViewport3_ContextCameraDelegateRegistrationNotSpatialGated(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3ContextRendererAsset)

	// `_registerCameraBusDelegate()` must be called from a non-spatial
	// site, i.e. inside `_mount(slotEl, ctx)`. The bus's REPLACE
	// policy means the spatial path's call is still allowed.
	mountIdx := strings.Index(js, "function _mount(slotEl, ctx)")
	if mountIdx < 0 {
		t.Fatalf("D37q-viewport-3-impl: strategic Context _mount(slotEl, ctx) must be present")
	}
	mountEnd := strings.Index(js[mountIdx:], "function _destroy()")
	if mountEnd < 0 {
		t.Fatalf("D37q-viewport-3-impl: strategic Context _mount/_destroy block must be well-formed")
	}
	mountBody := js[mountIdx : mountIdx+mountEnd]
	if !strings.Contains(mountBody, "_registerCameraBusDelegate()") {
		t.Errorf("D37q-viewport-3-impl: _registerCameraBusDelegate() must be called from _mount() (not gated on spatial mode)")
	}

	// The spatial path's registration must remain (REPLACE policy
	// upgrades the fallback when the live camera is created).
	if !strings.Contains(js, "if (_camera) _registerCameraBusDelegate();") {
		t.Errorf("D37q-viewport-3-impl: spatial _ensureCamera path must still call _registerCameraBusDelegate() after creating a live camera")
	}
}

// ── 3. Delegate exposes the locked command set ────────────────────

// TestExplorer_D37qViewport3_ContextCameraDelegateHasRequiredCommands
// pins that both the spatial-mode delegate and the non-spatial
// fallback expose the complete locked command vocabulary documented
// by the camera bus.
func TestExplorer_D37qViewport3_ContextCameraDelegateHasRequiredCommands(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3ContextRendererAsset)

	// Locate the two delegate factory functions.
	for _, builder := range []struct {
		name    string
		startEx string
	}{
		{name: "spatial delegate builder", startEx: "function _buildContextCameraDelegate(camera)"},
		{name: "fallback delegate builder", startEx: "function _buildFallbackContextCameraDelegate()"},
	} {
		idx := strings.Index(js, builder.startEx)
		if idx < 0 {
			t.Fatalf("D37q-viewport-3-impl: %s must be present (%q)", builder.name, builder.startEx)
		}
		// Slice forward to the next top-level `function ` declaration.
		tail := js[idx+len(builder.startEx):]
		endRel := strings.Index(tail, "\n  function ")
		if endRel < 0 {
			endRel = len(tail)
		}
		body := tail[:endRel]

		for _, cmd := range []string{
			"zoomIn:",
			"zoomOut:",
			"fit:",
			"reset:",
			"setZoom:",
			"getZoom:",
			"focusRoot:",
			"focusSelected:",
		} {
			if !strings.Contains(body, cmd) {
				t.Errorf("D37q-viewport-3-impl: %s must expose %q", builder.name, cmd)
			}
		}
	}
}

// ── 4. Spatial camera path preserved ──────────────────────────────

func TestExplorer_D37qViewport3_SpatialCameraPathPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3ContextRendererAsset)

	for _, want := range []string{
		"function _ensureCamera()",
		"g.graphCameraController.create(_cameraTarget(), {})",
		"function _renderSpatialFoundation(layout, cards, connectors)",
		// Spatial mode still gates the spatial render path.
		"if (_isSpatialMode() && _hasGraphStage()) {",
		"_renderSpatialFoundation(layout, cards, connectors);",
		// The spatial delegate wraps the live camera.
		"_buildContextCameraDelegate(_camera)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-3-impl: spatial camera path must remain intact (%q)", want)
		}
	}
}

// ── 5. Non-spatial fallback delegate is present + safe ────────────

func TestExplorer_D37qViewport3_NonSpatialStrategicContextHasSafeFallbackDelegate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3ContextRendererAsset)

	// The fallback builder exists.
	if !strings.Contains(js, "function _buildFallbackContextCameraDelegate()") {
		t.Errorf("D37q-viewport-3-impl: non-spatial strategic Context must declare a fallback camera-bus delegate builder")
	}

	// Slice the fallback body and assert each method is a safe no-op /
	// stable-value path: no `_camera.`, no DOM access, no throws.
	idx := strings.Index(js, "function _buildFallbackContextCameraDelegate()")
	if idx < 0 {
		t.Fatal("D37q-viewport-3-impl: fallback builder must be present")
	}
	tail := js[idx:]
	endRel := strings.Index(tail, "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37q-viewport-3-impl: fallback builder body must be well-formed")
	}
	body := tail[:endRel]

	for _, banned := range []string{
		"_camera.",
		"document.",
		"throw ",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37q-viewport-3-impl: fallback delegate must remain safe — found banned reference %q", banned)
		}
	}

	// `getZoom` must return a stable identity ratio (1).
	if !regexp.MustCompile(`getZoom:\s*function\s*\(\s*\)\s*\{\s*return 1;\s*\}`).MatchString(body) {
		t.Errorf("D37q-viewport-3-impl: fallback getZoom must return a stable identity ratio (1)")
	}

	// D37r-context-cytoscape-2-impl — the delegate dispatcher now
	// selects three-way: live Cytoscape delegate when `_cy` is set
	// (strategic spatial Cytoscape branch), live controller delegate
	// when `_camera` is set (DOM/SVG load-order-safety fallback or
	// non-spatial spatial-controller path), otherwise the safe
	// no-op fallback. The pre-D37r two-way ternary pin is flipped
	// here to assert the new three-way selection — all three branches
	// must be present and the fallback branch must still appear as
	// the terminal `else`.
	if !strings.Contains(js, "if (_cy) {") {
		t.Errorf("D37r-context-cytoscape-2-impl: _registerCameraBusDelegate must prefer the Cytoscape delegate when _cy is live")
	}
	if !strings.Contains(js, "_buildContextCytoscapeCameraDelegate(_cy)") {
		t.Errorf("D37r-context-cytoscape-2-impl: _registerCameraBusDelegate must wrap the live Cytoscape instance via _buildContextCytoscapeCameraDelegate(_cy)")
	}
	if !strings.Contains(js, "} else if (_camera) {") {
		t.Errorf("D37r-context-cytoscape-2-impl: _registerCameraBusDelegate must keep the controller-delegate branch when Cytoscape is not live but a controller camera exists")
	}
	if !strings.Contains(js, "_buildContextCameraDelegate(_camera)") {
		t.Errorf("D37r-context-cytoscape-2-impl: _registerCameraBusDelegate must keep the controller delegate path via _buildContextCameraDelegate(_camera)")
	}
	if !strings.Contains(js, "_buildFallbackContextCameraDelegate()") {
		t.Errorf("D37r-context-cytoscape-2-impl: _registerCameraBusDelegate must keep the safe fallback delegate path as the terminal branch")
	}
}

// ── 6. Native-context delegate unchanged ──────────────────────────

func TestExplorer_D37qViewport3_NativeContextDelegateUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3CameraToolbarAdapter)

	for _, want := range []string{
		"bus.registerLens('native-context', _legacyDelegate())",
		"function _legacyDelegate()",
		"_legacyCamera()",
		// All eight commands present in the legacy delegate.
		"zoomIn: function ",
		"zoomOut: function ",
		"fit: function ",
		"reset: function ",
		"focusRoot: function ",
		"focusSelected: function ",
		"setZoom: function ",
		"getZoom: function ",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-3-impl: native-context delegate must remain unchanged (%q)", want)
		}
	}
}

// ── 7. Authority camera delegate unchanged ────────────────────────

func TestExplorer_D37qViewport3_AuthorityCameraDelegateUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3AuthorityPocAsset)

	for _, want := range []string{
		"_registerAuthorityCameraBusDelegate",
		"bus.registerLens('authority',",
		"poc.zoomBy(step)",
		"poc.zoomBy(1 / step)",
		"poc.fit()",
		"poc.resetView()",
		"poc.centerOnRoot()",
		"poc.zoomToSelected()",
		"poc.getZoomPercent()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-3-impl: Authority camera delegate must remain unchanged (%q)", want)
		}
	}
}

// ── 8. No default renderer flip ───────────────────────────────────

func TestExplorer_D37qViewport3_NoDefaultRendererFlip(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Native Context remains the GraphViewport baseline.
	vp := getExplorerAsset(t, srv, d37qV3ViewportAsset)
	if !strings.Contains(vp, "_baselineId = 'native-context';") {
		t.Errorf("D37q-viewport-3-impl: GraphViewport host must still adopt native-context as the baseline renderer")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37q-viewport-3-impl: GraphViewport host must still adopt native-context at module init")
	}

	// Strategic Context remains opt-in via the contextRenderer URL flag;
	// spatial mode remains a separate opt-in via the contextLayout flag.
	ctx := getExplorerAsset(t, srv, d37qV3ContextRendererAsset)
	if !strings.Contains(ctx, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37q-viewport-3-impl: strategic Context activation must remain opt-in via 'contextRenderer'")
	}
	if !strings.Contains(ctx, "var MODE_STRATEGIC = 'strategic';") {
		t.Errorf("D37q-viewport-3-impl: strategic activation mode must remain 'strategic'")
	}
	if !strings.Contains(ctx, "var LAYOUT_QUERY_PARAM = 'contextLayout';") {
		t.Errorf("D37q-viewport-3-impl: layout activation must remain opt-in via 'contextLayout'")
	}
	if !strings.Contains(ctx, "var LAYOUT_MODE_SPATIAL = 'spatial';") {
		t.Errorf("D37q-viewport-3-impl: spatial layout mode must remain 'spatial'")
	}
	// The spatial-mode gate is still consulted only for the spatial
	// render path — not for delegate registration.
	if !strings.Contains(ctx, "if (_isSpatialMode() && _hasGraphStage()) {") {
		t.Errorf("D37q-viewport-3-impl: spatial render path must remain gated on _isSpatialMode() && _hasGraphStage()")
	}
}

// ── 9. Connector routing untouched ────────────────────────────────

func TestExplorer_D37qViewport3_NoConnectorRoutingChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	painter := getExplorerAsset(t, srv, d37qV3ContextConnectorPainter)
	stage := getExplorerAsset(t, srv, d37qV3StageAsset)

	// Painter public surface intact.
	for _, want := range []string{
		"window.MIDASExplorerGraph.contextConnectorPainter",
		"paintConnectors",
	} {
		if !strings.Contains(painter, want) {
			t.Errorf("D37q-viewport-3-impl: Context connector painter must remain intact (%q)", want)
		}
	}

	// Stage's anchor contract intact.
	for _, want := range []string{
		"window.MIDASExplorerGraph.graphStage",
		"compose",
		"anchorOf",
		"fitBoundsOf",
	} {
		if !strings.Contains(stage, want) {
			t.Errorf("D37q-viewport-3-impl: graphStage contract must remain intact (%q)", want)
		}
	}
}

// ── 10. Camera bus contract intact ────────────────────────────────

// TestExplorer_D37qViewport3_CameraBusContractIntact reasserts that the
// camera-bus module itself was not touched by this tranche.
func TestExplorer_D37qViewport3_CameraBusContractIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3CameraBusAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.graphCameraBus",
		"registerLens:",
		"unregisterLens:",
		"'zoomIn'",
		"'zoomOut'",
		"'fit'",
		"'reset'",
		"'focusRoot'",
		"'focusSelected'",
		"'setZoom'",
		"'getZoom'",
		// REPLACE policy in the registry.
		"_registry[lensId] = delegate",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37q-viewport-3-impl: graphCameraBus contract must remain intact (%q)", want)
		}
	}
}

// ── 11. Clean teardown unchanged ──────────────────────────────────

// TestExplorer_D37qViewport3_DelegateCleanupOnDestroy pins that
// `_destroyCamera()` continues to unregister the camera-bus delegate
// before tearing down the camera instance. With the new fallback
// path, this same cleanup covers both the fallback registration and
// the spatial-mode registration because the bus stores both under
// the same `context` lens id (REPLACE policy).
func TestExplorer_D37qViewport3_DelegateCleanupOnDestroy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qV3ContextRendererAsset)

	if !strings.Contains(js, "function _destroyCamera()") {
		t.Errorf("D37q-viewport-3-impl: _destroyCamera() must remain present")
	}
	if !strings.Contains(js, "function _unregisterCameraBusDelegate()") {
		t.Errorf("D37q-viewport-3-impl: _unregisterCameraBusDelegate() helper must remain present")
	}
	// `_destroyCamera` calls `_unregisterCameraBusDelegate` first.
	idx := strings.Index(js, "function _destroyCamera()")
	if idx < 0 {
		t.Fatalf("D37q-viewport-3-impl: _destroyCamera() must be present")
	}
	tail := js[idx:]
	endRel := strings.Index(tail, "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37q-viewport-3-impl: _destroyCamera() body must be well-formed")
	}
	body := tail[:endRel]
	if !strings.Contains(body, "_unregisterCameraBusDelegate();") {
		t.Errorf("D37q-viewport-3-impl: _destroyCamera() must call _unregisterCameraBusDelegate() (covers both spatial and fallback cleanup)")
	}
}
