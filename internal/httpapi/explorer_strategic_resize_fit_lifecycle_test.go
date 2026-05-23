package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37u-cytoscape-resize-policy-impl — Resize-only passive lifecycle.
//
// The shared graph engine owns resize lifecycle, camera policy, and
// fit policy. The upstream Cytoscape assessment (D37t) established:
//
//   • cy.resize() invalidates Cytoscape's cached container size and
//     schedules a redraw; it does NOT mutate _private.pan or
//     _private.zoom.
//   • cy.fit() and cy.viewport({zoom, pan}) DO mutate camera. They
//     are CAMERA TRIGGERS and must NOT fire on passive container
//     resize (drawer toggle, tray expand, focus mode, window resize).
//
// Therefore the passive resize lifecycle is reduced to:
//
//   1. emit engine_resize_detected
//   2. call cy.resize()
//   3. emit engine_cy_resize_called
//   4. emit engine_camera_preserved_on_resize
//
// No timer. No fit. No viewport. Operator pan/zoom preserved.
//
// Explicit fit (initial mount, handle.fit, future operator actions)
// remains supported and emits engine_fit_invoked with a `source`
// field naming the trigger.
//
// Scope guards encoded as tests:
//   • Obsolete D37s-engine-lifecycle-2-impl deferred-refit machinery
//     (RESIZE_SETTLE_WINDOW_MS, RECT_STABILITY_THRESHOLD_PX,
//     _resizeRefitTimer, _lastFittedRect, _rectMateriallyDiffers,
//     _runResizeRefit, resize_refit_skipped_rect_unchanged) is fully
//     removed.
//   • Authority bespoke `_refitWithSafeArea` path is untouched.
//   • GraphViewport `onResize` lifecycle is untouched.

const (
	d37uResizePolicyEngineModule   = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37uResizePolicyAuthorityPoc   = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37uResizePolicyViewportModule = "/explorer/assets/js/graph/graph-viewport.js"
)

// ── 1. New diagnostic codes are declared ───────────────────────────────

func TestExplorer_D37uResizePolicy_DiagnosticCodesDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	cases := []struct {
		name   string
		needle string
	}{
		{"engine_resize_detected",
			"DIAG_ENGINE_RESIZE_DETECTED            = 'engine_resize_detected'"},
		{"engine_cy_resize_called",
			"DIAG_ENGINE_CY_RESIZE_CALLED           = 'engine_cy_resize_called'"},
		{"engine_camera_preserved_on_resize",
			"DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE = 'engine_camera_preserved_on_resize'"},
		{"engine_fit_invoked",
			"DIAG_ENGINE_FIT_INVOKED                = 'engine_fit_invoked'"},
	}
	for _, c := range cases {
		if !strings.Contains(js, c.needle) {
			t.Errorf("D37u-cytoscape-resize-policy-impl: engine must declare %s (looking for %q)", c.name, c.needle)
		}
	}
}

// ── 2. Obsolete deferred-refit machinery removed ───────────────────────

func TestExplorer_D37uResizePolicy_DeferredRefitMachineryRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	// Each of these names belonged to the now-retired D37s-engine-
	// lifecycle-2-impl trailing-edge debounce + previous-rect guard.
	// They are dead code under the resize-only passive policy and
	// must not survive in executable form.
	bannedSymbols := []string{
		"function _runResizeRefit",
		"function _scheduleResizeRefit",
		"function _rectMateriallyDiffers",
		"var _resizeRefitTimer",
		"var _lastFittedRect",
		"var RESIZE_SETTLE_WINDOW_MS",
		"var RECT_STABILITY_THRESHOLD_PX",
		"DIAG_RESIZE_REFIT_COALESCED",
		"DIAG_RESIZE_REFIT_SKIPPED_RECT_UNCHANGED",
		"DIAG_ENGINE_REFIT_AFTER_RESIZE",
		"'resize_refit_coalesced'",
		"'resize_refit_skipped_rect_unchanged'",
		"'engine_refit_after_resize'",
	}
	for _, s := range bannedSymbols {
		if strings.Contains(js, s) {
			t.Errorf("D37u-cytoscape-resize-policy-impl: %q is obsolete and must be removed from the engine module (resize-only passive policy)", s)
		}
	}
}

// ── 3. ResizeObserver routes through the resize-only handler ───────────

func TestExplorer_D37uResizePolicy_ROUsesResizeOnlyHandler(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	if !strings.Contains(js, "function _onContainerResize()") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: engine must define _onContainerResize() as the resize-only handler")
	}
	if !strings.Contains(js, "resizeObs = new window.ResizeObserver(_onContainerResize)") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: ResizeObserver must be constructed with _onContainerResize as its callback")
	}
}

// ── 4. Passive resize path cannot mutate camera ────────────────────────

func TestExplorer_D37uResizePolicy_PassiveResizeDoesNotFit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	// Extract the _onContainerResize body and assert it does NOT call
	// any camera-mutating function.
	idx := strings.Index(js, "function _onContainerResize()")
	if idx < 0 {
		t.Fatal("D37u-cytoscape-resize-policy-impl: _onContainerResize must exist")
	}
	tail := js[idx:]
	endRel := strings.Index(tail, "\n    }\n")
	if endRel < 0 {
		t.Fatalf("D37u-cytoscape-resize-policy-impl: _onContainerResize body must be well-formed")
	}
	body := tail[:endRel+1]

	// Must call cy.resize().
	if !strings.Contains(body, "cy.resize()") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: passive resize must call cy.resize()")
	}
	// Must emit the resize-only lifecycle diagnostics.
	required := []string{
		"_emitLifecycleDiagnostic(DIAG_ENGINE_RESIZE_DETECTED)",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_CY_RESIZE_CALLED)",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)",
	}
	for _, r := range required {
		if !strings.Contains(body, r) {
			t.Errorf("D37u-cytoscape-resize-policy-impl: passive resize must emit %s", r)
		}
	}
	// Must NOT call any camera-mutating function.
	forbidden := []string{
		"_runFitPipeline",
		"_fitToUsableRect",
		"_fitWithSafeArea",
		"cy.viewport(",
		"cy.fit(",
		"cy.pan(",
		"cy.zoom(",
		"setTimeout",
	}
	for _, f := range forbidden {
		if strings.Contains(body, f) {
			t.Errorf("D37u-cytoscape-resize-policy-impl: passive resize path must NOT call %q (camera-mutation / deferred-refit machinery)", f)
		}
	}
}

// ── 5. Explicit fit path still works and is source-tagged ──────────────

func TestExplorer_D37uResizePolicy_ExplicitFitEmitsSourceTaggedDiagnostic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	// _runFitPipeline accepts (source, phase) — phase added in
	// D37ad-initial-fit-stable-cadence for the multi-tick cadence.
	if !strings.Contains(js, "function _runFitPipeline(source, phase)") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: _runFitPipeline must take (source, phase)")
	}
	// _runFitPipeline emits engine_fit_invoked with the source.
	if !strings.Contains(js, "_emitLifecycleDiagnostic(DIAG_ENGINE_FIT_INVOKED, ") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: explicit fit path must emit DIAG_ENGINE_FIT_INVOKED with a payload (source)")
	}
	// Initial settle uses the 'initial_mount' source. After
	// D37af-initial-fit-stability-gate the call site is
	// `_runFitPipeline('initial_mount', 'stabilising')` (the
	// rAF-driven stabilisation loop tags its phase as 'stabilising').
	if !strings.Contains(js, "_runFitPipeline('initial_mount', 'stabilising')") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: initial settle must invoke _runFitPipeline('initial_mount', 'stabilising')")
	}
	// handle.fit accepts optional fitOpts.source; defaults to 'explicit_fit'.
	if !strings.Contains(js, "source: source") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: handle.fit's engine_fit_invoked emit must include source")
	}
	if !strings.Contains(js, "'explicit_fit'") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: handle.fit must default the source tag to 'explicit_fit'")
	}
}

// ── 6. Explicit fit still uses the safe-area-aware path ────────────────

func TestExplorer_D37uResizePolicy_ExplicitFitUsesSafeAreaPath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	// handle.fit body must still consult getUsableGraphRect and fall
	// back to _fitWithSafeArea — the existing safe-area-aware contract
	// is preserved.
	hIdx := strings.Index(js, "fit: function (fitOpts) {")
	if hIdx < 0 {
		t.Fatal("D37u-cytoscape-resize-policy-impl: handle.fit must exist")
	}
	hTail := js[hIdx:]
	hEnd := strings.Index(hTail, "\n      },\n")
	if hEnd < 0 {
		t.Fatalf("D37u-cytoscape-resize-policy-impl: handle.fit body must be well-formed")
	}
	hBody := hTail[:hEnd+1]

	if !strings.Contains(hBody, "opts.getUsableGraphRect()") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: handle.fit must still consult getUsableGraphRect() (safe-area-aware path)")
	}
	if !strings.Contains(hBody, "_fitToUsableRect(cy, usable, _recordFitDiagnostic)") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: handle.fit must still call _fitToUsableRect on the strategic path")
	}
	if !strings.Contains(hBody, "_fitWithSafeArea(cy, _resolveFitPadding") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: handle.fit must still fall back to _fitWithSafeArea")
	}
}

// ── 7. Per-firing diagnostic emitter accepts an optional payload ──────

func TestExplorer_D37uResizePolicy_LifecycleEmitterAcceptsPayload(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	if !strings.Contains(js, "function _emitLifecycleDiagnostic(code, payload)") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: _emitLifecycleDiagnostic must accept (code, payload) so engine_fit_invoked can carry a source tag")
	}
	// Lifecycle diagnostics must NOT go through the dedup'd
	// _recordFitDiagnostic emitter — per-firing observability is the
	// point.
	dedup := []string{
		"_recordFitDiagnostic(DIAG_ENGINE_RESIZE_DETECTED)",
		"_recordFitDiagnostic(DIAG_ENGINE_CY_RESIZE_CALLED)",
		"_recordFitDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)",
		"_recordFitDiagnostic(DIAG_ENGINE_FIT_INVOKED",
	}
	for _, d := range dedup {
		if strings.Contains(js, d) {
			t.Errorf("D37u-cytoscape-resize-policy-impl: lifecycle codes must use the per-firing _emitLifecycleDiagnostic, NOT the dedup'd _recordFitDiagnostic (found %q)", d)
		}
	}
}

// ── 8. Layout policy unchanged ────────────────────────────────────────

func TestExplorer_D37uResizePolicy_LayoutPolicyUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	// Preset layout still has fit:false so the layout extension does
	// NOT silently auto-fit on every run.
	if !strings.Contains(js, "name: 'preset', fit: false") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: preset layout must keep `fit: false` so the layout extension does not auto-fit (layout policy must stay separate from camera policy)")
	}
}

// ── 9. handle.destroy() no longer references obsolete state ───────────

func TestExplorer_D37uResizePolicy_DestroyHasNoDeferredRefitCleanup(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37uResizePolicyEngineModule)

	dIdx := strings.Index(js, "destroy: function ()")
	if dIdx < 0 {
		t.Fatal("D37u-cytoscape-resize-policy-impl: handle.destroy must exist")
	}
	dTail := js[dIdx:]
	dEnd := strings.Index(dTail, "\n      },\n")
	if dEnd < 0 {
		t.Fatalf("D37u-cytoscape-resize-policy-impl: handle.destroy body must be well-formed")
	}
	dBody := dTail[:dEnd+1]

	bannedInDestroy := []string{
		"_resizeRefitTimer",
		"_lastFittedRect",
		"_resizeRefitRaf",
	}
	for _, b := range bannedInDestroy {
		if strings.Contains(dBody, b) {
			t.Errorf("D37u-cytoscape-resize-policy-impl: destroy() must not reference removed deferred-refit state (%q)", b)
		}
	}
}

// ── 10. Scope guard: Authority bespoke refit path untouched ──────────

func TestExplorer_D37uResizePolicy_AuthorityRefitPathUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJS := getExplorerAsset(t, srv, d37uResizePolicyAuthorityPoc)

	if !strings.Contains(authJS, "function _refitWithSafeArea()") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: Authority must retain _refitWithSafeArea")
	}
	if !strings.Contains(authJS, "_cy.resize();") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: Authority _refitWithSafeArea must still call _cy.resize()")
	}
	if !strings.Contains(authJS, "_fitToAvailableCanvas(_cy)") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: Authority _refitWithSafeArea must still delegate to _fitToAvailableCanvas")
	}
	if !strings.Contains(authJS, "ctx.onResize(_refitWithSafeArea)") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: Authority factory must still subscribe to ctx.onResize(_refitWithSafeArea)")
	}
}

// ── 11. Scope guard: GraphViewport lifecycle untouched ───────────────

func TestExplorer_D37uResizePolicy_GraphViewportLifecycleUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	vpJS := getExplorerAsset(t, srv, d37uResizePolicyViewportModule)

	if !strings.Contains(vpJS, "function onResize(handler)") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: GraphViewport.onResize must remain the subscription surface")
	}
	if strings.Contains(vpJS, "new MutationObserver") || strings.Contains(vpJS, "new window.MutationObserver") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: this tranche must NOT add a MutationObserver to GraphViewport")
	}
	if strings.Contains(vpJS, "onUsableRectChange") {
		t.Errorf("D37u-cytoscape-resize-policy-impl: onUsableRectChange must not be introduced in this tranche")
	}
}
