package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37ag-diagnostic-geometry-dump — observational instrumentation. Adds an
// `engine_geometry_dump` diagnostic emitted from six call sites:
//
//   1. _runStabilisationFrame, immediately after _runFitPipeline('initial_mount', 'stabilising').
//   2. _revealGraph, immediately after the existing engine_initial_reveal /
//      engine_initial_reveal_forced emission.
//   3. _tryPostRevealCorrection, immediately after the existing
//      engine_post_reveal_stabilisation_fit emission.
//   4-6. Three handle.fit() branches (caller-supplied-padding,
//        usable-rect-success, fallback), each immediately after the
//        branch's existing _emitLifecycleDiagnostic(DIAG_ENGINE_FIT_INVOKED, …).
//
// Behaviour invariants (this tranche is OBSERVATIONAL ONLY):
//   • No changes to _runFitPipeline, _fitToUsableRect, _fitWithSafeArea,
//     _resolveFitPadding (byte-identical to current main).
//   • No changes to handle.fit() control flow.
//   • No changes to stabilisation loop, _onContainerResize,
//     _tryPostRevealCorrection, _revealGraph behaviour.
//   • All existing diagnostic codes preserved.
//   • All existing D37u / D37af / D37x pins still pass.

const (
	d37agEngineModule = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
)

// ── 1. Diagnostic code declared ─────────────────────────────────────

func TestExplorer_D37agGeometryDump_DiagnosticCodeDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	// D37ah widened the alignment column when new (longer) D37ah
	// constants were added to the same block; the literal-string pin
	// follows the new column width.
	if !strings.Contains(js, "DIAG_ENGINE_GEOMETRY_DUMP                       = 'engine_geometry_dump'") {
		t.Errorf("D37ag-diagnostic-geometry-dump: engine must declare DIAG_ENGINE_GEOMETRY_DUMP = 'engine_geometry_dump'")
	}
}

// ── 2. _emitGeometryDump helper exists with expected signature ──────

func TestExplorer_D37agGeometryDump_EmitHelperExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	if !strings.Contains(js, "function _emitGeometryDump(trigger, source, phase)") {
		t.Errorf("D37ag-diagnostic-geometry-dump: engine must define _emitGeometryDump(trigger, source, phase)")
	}
}

// ── 3. All four trigger string literals present ─────────────────────

func TestExplorer_D37agGeometryDump_TriggerStringsPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	for _, lit := range []string{
		"'stabilisation_tick'",
		"'initial_reveal'",
		"'post_reveal_correction'",
		"'explicit_fit'",
		// D37ah-readiness-gated-initial-fit — adds state_transition,
		// emitted by _transitionTo alongside every lifecycle state
		// change.
		"'state_transition'",
	} {
		if !strings.Contains(js, lit) {
			t.Errorf("D37ag-diagnostic-geometry-dump: trigger literal %s must be present in engine source", lit)
		}
	}
}

// ── 4. Payload shape contains all required fields ───────────────────

func TestExplorer_D37agGeometryDump_PayloadShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	idx := strings.Index(js, "function _emitGeometryDump(trigger, source, phase)")
	if idx < 0 {
		t.Fatal("D37ag-diagnostic-geometry-dump: _emitGeometryDump must exist")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ag-diagnostic-geometry-dump: _emitGeometryDump body must be well-formed")
	}
	body := tail[:end+1]

	required := []string{
		// All payload field keys.
		"trigger:",
		"source:",
		"phase:",
		"cyWidth:",
		"cyHeight:",
		"containerRect:",
		"usableRect:",
		"elementsBbox:",
		"resolvedPadding:",
		"postFitPan:",
		"postFitZoom:",
		"timestamp:",
		// Routes through the existing per-firing emitter.
		"_emitLifecycleDiagnostic(DIAG_ENGINE_GEOMETRY_DUMP",
	}
	for _, r := range required {
		if !strings.Contains(body, r) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _emitGeometryDump payload must declare %q", r)
		}
	}
	// Geometry sources match the prompt's spec.
	for _, source := range []string{
		"opts.getUsableGraphRect()",
		"cy.elements()",
		"eles.boundingBox(FIT_BBOX_OPTS)",
		"_resolveFitPadding(undefined, opts.getSafeArea)",
		"cy.pan()",
		"cy.zoom()",
		"container.getBoundingClientRect()",
	} {
		if !strings.Contains(body, source) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _emitGeometryDump must read %q", source)
		}
	}
}

// ── 5. Stabilisation-tick emission sits right after _runFitPipeline ─

func TestExplorer_D37agGeometryDump_StabilisationTickEmission(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	idx := strings.Index(js, "function _runStabilisationFrame()")
	if idx < 0 {
		t.Fatal("D37ag-diagnostic-geometry-dump: _runStabilisationFrame must exist")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ag-diagnostic-geometry-dump: _runStabilisationFrame body must be well-formed")
	}
	body := tail[:end+1]

	fitCall := "var fitApplied = _runFitPipeline('initial_mount', 'stabilising');"
	dumpCall := "_emitGeometryDump('stabilisation_tick', 'initial_mount', 'stabilising');"

	if !strings.Contains(body, fitCall) {
		t.Errorf("D37ag-diagnostic-geometry-dump: _runStabilisationFrame must still call %s", fitCall)
	}
	if !strings.Contains(body, dumpCall) {
		t.Errorf("D37ag-diagnostic-geometry-dump: _runStabilisationFrame must emit %s", dumpCall)
	}
	// Order: dump AFTER the canonical fit call.
	fitIdx := strings.Index(body, fitCall)
	dumpIdx := strings.Index(body, dumpCall)
	if fitIdx < 0 || dumpIdx < 0 || !(fitIdx < dumpIdx) {
		t.Errorf("D37ag-diagnostic-geometry-dump: stabilisation-tick dump must appear AFTER the _runFitPipeline call so postFitPan/postFitZoom reflect the fit's result")
	}
}

// ── 6. Initial-reveal emission sits right after the reveal diag ─────
//
// D37ah-readiness-gated-initial-fit — Reveal is now reason-based;
// _revealGraph(reason) computes a `revealDiag` variable mapped from
// the reason and emits it via _emitLifecycleDiagnostic. The geometry
// dump still follows the lifecycle emission, but the literal token
// is the resolved variable `revealDiag` rather than the legacy
// boolean ternary.

func TestExplorer_D37agGeometryDump_InitialRevealEmission(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	idx := strings.Index(js, "function _revealGraph(reason)")
	if idx < 0 {
		t.Fatal("D37ag-diagnostic-geometry-dump: _revealGraph(reason) must exist (D37ah API)")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		end = strings.Index(tail, "\n    // ──")
		if end < 0 {
			t.Fatalf("D37ag-diagnostic-geometry-dump: _revealGraph body must be well-formed")
		}
	}
	body := tail[:end+1]

	revealEmit := "_emitLifecycleDiagnostic(revealDiag);"
	dumpCall := "_emitGeometryDump('initial_reveal', null, null);"

	if !strings.Contains(body, revealEmit) {
		t.Errorf("D37ag-diagnostic-geometry-dump: _revealGraph must emit the resolved revealDiag via %q", revealEmit)
	}
	if !strings.Contains(body, dumpCall) {
		t.Errorf("D37ag-diagnostic-geometry-dump: _revealGraph must emit %s", dumpCall)
	}
	revealIdx := strings.Index(body, revealEmit)
	dumpIdx := strings.Index(body, dumpCall)
	if revealIdx < 0 || dumpIdx < 0 || !(revealIdx < dumpIdx) {
		t.Errorf("D37ag-diagnostic-geometry-dump: initial-reveal dump must appear AFTER the reveal-diag emission")
	}
	// The three reason-mapped reveal diagnostic constants must all be
	// referenced in the body (each branch assigns the right one).
	for _, c := range []string{
		"DIAG_ENGINE_INITIAL_REVEAL;",
		"DIAG_ENGINE_INITIAL_REVEAL_FORCED_CY_ZERO;",
		"DIAG_ENGINE_INITIAL_REVEAL_FORCED_FIT_FAILED;",
	} {
		if !strings.Contains(body, c) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _revealGraph must assign %s in the reason → diagnostic mapping", c)
		}
	}
}

// ── 7. Post-reveal correction emission sits right after the existing diag ──

func TestExplorer_D37agGeometryDump_PostRevealCorrectionEmission(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	idx := strings.Index(js, "function _tryPostRevealCorrection()")
	if idx < 0 {
		t.Fatal("D37ag-diagnostic-geometry-dump: _tryPostRevealCorrection must exist")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		end = strings.Index(tail, "\n    // ──")
		if end < 0 {
			t.Fatalf("D37ag-diagnostic-geometry-dump: _tryPostRevealCorrection body must be well-formed")
		}
	}
	body := tail[:end+1]

	existing := "_emitLifecycleDiagnostic(DIAG_ENGINE_POST_REVEAL_STABILISATION_FIT);"
	dumpCall := "_emitGeometryDump('post_reveal_correction', 'post_reveal_stabilisation', null);"

	if !strings.Contains(body, existing) {
		t.Errorf("D37ag-diagnostic-geometry-dump: _tryPostRevealCorrection must still emit DIAG_ENGINE_POST_REVEAL_STABILISATION_FIT")
	}
	if !strings.Contains(body, dumpCall) {
		t.Errorf("D37ag-diagnostic-geometry-dump: _tryPostRevealCorrection must emit %s", dumpCall)
	}
	existingIdx := strings.Index(body, existing)
	dumpIdx := strings.Index(body, dumpCall)
	if existingIdx < 0 || dumpIdx < 0 || !(existingIdx < dumpIdx) {
		t.Errorf("D37ag-diagnostic-geometry-dump: post-reveal correction dump must appear AFTER engine_post_reveal_stabilisation_fit emission")
	}
}

// ── 8. All three handle.fit() branches have BOTH emissions, in order ──

func TestExplorer_D37agGeometryDump_HandleFitThreeBranchEmissions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	hIdx := strings.Index(js, "fit: function (fitOpts) {")
	if hIdx < 0 {
		t.Fatal("D37ag-diagnostic-geometry-dump: handle.fit must exist")
	}
	hTail := js[hIdx:]
	hEnd := strings.Index(hTail, "\n      },\n")
	if hEnd < 0 {
		t.Fatalf("D37ag-diagnostic-geometry-dump: handle.fit body must be well-formed")
	}
	hBody := hTail[:hEnd+1]

	// Inside handle.fit body, count occurrences of each emission. We
	// expect exactly THREE pairs of (fit_invoked, geometry_dump), one
	// per branch, each pair adjacent with the fit_invoked first.
	fitInvokedLine := "_emitLifecycleDiagnostic(DIAG_ENGINE_FIT_INVOKED, { source: source });"
	dumpLine := "_emitGeometryDump('explicit_fit', source, null);"

	if cnt := strings.Count(hBody, fitInvokedLine); cnt != 3 {
		t.Errorf("D37ag-diagnostic-geometry-dump: handle.fit body must contain exactly 3 fit_invoked emissions (one per branch); got %d", cnt)
	}
	if cnt := strings.Count(hBody, dumpLine); cnt != 3 {
		t.Errorf("D37ag-diagnostic-geometry-dump: handle.fit body must contain exactly 3 geometry_dump emissions (one per branch); got %d", cnt)
	}

	// Each fit_invoked emission must be immediately followed (within a
	// small window) by a geometry_dump emission. Walk the body.
	remaining := hBody
	branchN := 0
	for {
		fIdx := strings.Index(remaining, fitInvokedLine)
		if fIdx < 0 {
			break
		}
		branchN++
		// Advance to just past the fit_invoked line.
		afterFit := remaining[fIdx+len(fitInvokedLine):]
		// The geometry_dump emission for THIS branch must appear before
		// the NEXT fit_invoked emission (or end-of-body).
		nextFitIdx := strings.Index(afterFit, fitInvokedLine)
		dumpIdx := strings.Index(afterFit, dumpLine)
		if dumpIdx < 0 {
			t.Errorf("D37ag-diagnostic-geometry-dump: handle.fit branch %d has no following %s", branchN, dumpLine)
			break
		}
		if nextFitIdx >= 0 && dumpIdx > nextFitIdx {
			t.Errorf("D37ag-diagnostic-geometry-dump: handle.fit branch %d emits a fit_invoked BEFORE its own geometry_dump appears (out of order)", branchN)
		}
		// Move past this branch's geometry_dump for the next iteration.
		remaining = afterFit[dumpIdx+len(dumpLine):]
	}
	if branchN != 3 {
		t.Errorf("D37ag-diagnostic-geometry-dump: expected to walk 3 handle.fit branches, walked %d", branchN)
	}
}

// ── 9. No existing diagnostic codes were removed ────────────────────

func TestExplorer_D37agGeometryDump_NoExistingDiagnosticCodesRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	for _, code := range []string{
		"DIAG_ENGINE_RESIZE_DETECTED",
		"DIAG_ENGINE_CY_RESIZE_CALLED",
		"DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE",
		"DIAG_ENGINE_FIT_INVOKED",
		"DIAG_ENGINE_INITIAL_STABILISATION_STARTED",
		"DIAG_ENGINE_INITIAL_STABILISATION_SAMPLE",
		"DIAG_ENGINE_INITIAL_STABILISATION_STABLE",
		"DIAG_ENGINE_INITIAL_FIT_APPLIED",
		"DIAG_ENGINE_INITIAL_REVEAL",
		// D37ah-readiness-gated-initial-fit — replaced the unified
		// DIAG_ENGINE_INITIAL_REVEAL_FORCED with two reason-specific
		// codes (clean break).
		"DIAG_ENGINE_INITIAL_REVEAL_FORCED_CY_ZERO",
		"DIAG_ENGINE_INITIAL_REVEAL_FORCED_FIT_FAILED",
		"DIAG_ENGINE_POST_REVEAL_STABILISATION_FIT",
		"DIAG_ENGINE_GEOMETRY_DUMP",
	} {
		if !strings.Contains(js, code) {
			t.Errorf("D37ag-diagnostic-geometry-dump: previously-declared diagnostic constant %q must still be present", code)
		}
	}

	// Same check for the literal string values.
	for _, code := range []string{
		"'engine_resize_detected'",
		"'engine_cy_resize_called'",
		"'engine_camera_preserved_on_resize'",
		"'engine_fit_invoked'",
		"'engine_initial_stabilisation_started'",
		"'engine_initial_stabilisation_sample'",
		"'engine_initial_stabilisation_stable'",
		"'engine_initial_fit_applied'",
		"'engine_initial_reveal'",
		"'engine_initial_reveal_forced_cy_zero'",
		"'engine_initial_reveal_forced_fit_failed'",
		"'engine_post_reveal_stabilisation_fit'",
		"'engine_geometry_dump'",
	} {
		if !strings.Contains(js, code) {
			t.Errorf("D37ag-diagnostic-geometry-dump: previously-declared diagnostic code literal %s must still be present", code)
		}
	}
}

// ── 10. _runStabilisationFrame canonical call unchanged ────────────

func TestExplorer_D37agGeometryDump_StabilisationCanonicalCallUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	// The canonical fit call from the stabilisation loop must still
	// be _runFitPipeline('initial_mount', 'stabilising') — adding the
	// geometry dump must not have altered its arguments.
	if !strings.Contains(js, "var fitApplied = _runFitPipeline('initial_mount', 'stabilising');") {
		t.Errorf("D37ag-diagnostic-geometry-dump: _runStabilisationFrame must still invoke _runFitPipeline('initial_mount', 'stabilising')")
	}
}

// ── 11. _onContainerResize body unchanged (D37u re-pin) ────────────

func TestExplorer_D37agGeometryDump_OnContainerResizeUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ag-diagnostic-geometry-dump: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ag-diagnostic-geometry-dump: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	required := []string{
		"_emitLifecycleDiagnostic(DIAG_ENGINE_RESIZE_DETECTED)",
		"cy.resize()",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_CY_RESIZE_CALLED)",
		"if (_tryPostRevealCorrection()) return;",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)",
	}
	for _, r := range required {
		if !strings.Contains(rBody, r) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _onContainerResize body must still contain %q (D37u preservation)", r)
		}
	}
	forbidden := []string{
		"_emitGeometryDump",
		"_runFitPipeline",
		"_fitToUsableRect",
		"_fitWithSafeArea",
		"cy.viewport(",
		"cy.fit(",
		"setTimeout",
	}
	for _, f := range forbidden {
		if strings.Contains(rBody, f) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _onContainerResize body must not contain %q (D37u preservation; geometry dump must NOT be emitted from passive RO ticks directly)", f)
		}
	}
}

// ── 12. Canonical fit functions unchanged (no math mutations) ──────
//
// The four protected functions — _runFitPipeline, _fitToUsableRect,
// _fitWithSafeArea, _resolveFitPadding — must remain structurally
// intact. We pin a representative set of essential statements per
// function. Any material change to the math will remove at least one
// pinned statement. The pin set covers each function's defining
// behaviour (signature + camera operations + invariant guards) so a
// silent edit cannot pass without tripping a pin.

func TestExplorer_D37agGeometryDump_RunFitPipelineUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	required := []string{
		"function _runFitPipeline(source, phase) {",
		"if (_destroyed) return false;",
		"var fitApplied = false;",
		"cy.resize();",
		"var fittedViaUsable = false;",
		"if (_isFn(opts.getUsableGraphRect)) {",
		"usable = opts.getUsableGraphRect();",
		"if (_isPlainObject(usable) && usable.width > 0 && usable.height > 0) {",
		"fittedViaUsable = _fitToUsableRect(cy, usable, _recordFitDiagnostic);",
		"_recordFitDiagnostic(DIAG_USABLE_RECT_EMPTY);",
		"if (!fittedViaUsable) {",
		"var padding = _resolveFitPadding(undefined, opts.getSafeArea);",
		"_fitWithSafeArea(cy, padding);",
		"fitApplied = true;",
		"source: (typeof source === 'string' && source) ? source : 'explicit_fit',",
		"if (typeof phase === 'string' && phase) diagPayload.phase = phase;",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_FIT_INVOKED, diagPayload);",
		"return fitApplied;",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _runFitPipeline body regression — missing %q", r)
		}
	}
}

func TestExplorer_D37agGeometryDump_FitToUsableRectUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	required := []string{
		"function _fitToUsableRect(cy, usableRect, emitDiag) {",
		"if (!cy) return false;",
		"try { eles = cy.elements(':visible'); }",
		"if (_isFn(emitDiag)) emitDiag(DIAG_FIT_BOUNDS_EMPTY);",
		"try { bb = eles.boundingBox(FIT_BBOX_OPTS); } catch (_) { return false; }",
		"if (!(uw > 0) || !(uh > 0)) {",
		"if (_isFn(emitDiag)) emitDiag(DIAG_USABLE_RECT_EMPTY);",
		"if (uw < FIT_MIN_VISIBLE_PX || uh < FIT_MIN_VISIBLE_PX) {",
		"if (_isFn(emitDiag)) emitDiag(DIAG_USABLE_RECT_SMALLER_THAN_MIN);",
		"var zRaw = Math.min(uw / bb.w, uh / bb.h);",
		"var zMaxClamp = Math.min(isFinite(cyMax) ? cyMax : Infinity, FIT_ZOOM_MAX);",
		"var zMinClamp = Math.max(cyMin || 0, FIT_ZOOM_MIN);",
		"var rcx = ux + uw / 2;",
		"var rcy = uy + uh / 2;",
		"var gx  = bb.x1 + bb.w / 2;",
		"var gy  = bb.y1 + bb.h / 2;",
		"cy.viewport({ zoom: z, pan: { x: rcx - gx * z, y: rcy - gy * z } });",
		"return true;",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _fitToUsableRect body regression — missing %q", r)
		}
	}
}

func TestExplorer_D37agGeometryDump_FitWithSafeAreaUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	required := []string{
		"function _fitWithSafeArea(cy, padding) {",
		"if (!cy) return;",
		"try { eles = cy.elements(':visible'); }",
		"try { bb = eles.boundingBox(FIT_BBOX_OPTS); } catch (_) { return; }",
		"if (!bb || !(bb.w > 0) || !(bb.h > 0)) return;",
		"var L = padding.left, R = padding.right, T = padding.top, B = padding.bottom;",
		"if (cw - L - R < FIT_MIN_VISIBLE_PX) {",
		"if (ch - T - B < FIT_MIN_VISIBLE_PX) {",
		"var z = Math.min(vw / bb.w, vh / bb.h);",
		"cy.viewport({",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _fitWithSafeArea body regression — missing %q", r)
		}
	}
}

func TestExplorer_D37agGeometryDump_ResolveFitPaddingUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	required := []string{
		"function _resolveFitPadding(input, getSafeArea) {",
		"if (typeof input === 'number' && isFinite(input) && input >= 0) {",
		"return { top: input, right: input, bottom: input, left: input };",
		"if (_isPlainObject(input)) {",
		"top:    (typeof input.top    === 'number' && isFinite(input.top))    ? input.top    : DEFAULT_FIT_PADDING,",
		"right:  (typeof input.right  === 'number' && isFinite(input.right))  ? input.right  : DEFAULT_FIT_PADDING,",
		"bottom: (typeof input.bottom === 'number' && isFinite(input.bottom)) ? input.bottom : DEFAULT_FIT_PADDING,",
		"left:   (typeof input.left   === 'number' && isFinite(input.left))   ? input.left   : DEFAULT_FIT_PADDING,",
		"if (_isFn(getSafeArea)) {",
		"try { sa = getSafeArea(); } catch (_) { sa = null; }",
		"if (_isPlainObject(sa)) {",
		"top:    Math.max(DEFAULT_FIT_PADDING, (typeof sa.top    === 'number') ? sa.top    : 0),",
		"right:  Math.max(DEFAULT_FIT_PADDING, (typeof sa.right  === 'number') ? sa.right  : 0),",
		"bottom: Math.max(DEFAULT_FIT_PADDING, (typeof sa.bottom === 'number') ? sa.bottom : 0),",
		"left:   Math.max(DEFAULT_FIT_PADDING, (typeof sa.left   === 'number') ? sa.left   : 0),",
		"return { top: DEFAULT_FIT_PADDING, right: DEFAULT_FIT_PADDING, bottom: DEFAULT_FIT_PADDING, left: DEFAULT_FIT_PADDING };",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _resolveFitPadding body regression — missing %q", r)
		}
	}
}

// ── 13. _emitGeometryDump never mutates engine state ────────────────
//
// Belt-and-braces: the body must NOT call any function known to
// change pan/zoom/elements/style. Read-only operations only.

func TestExplorer_D37agGeometryDump_HelperIsReadOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	idx := strings.Index(js, "function _emitGeometryDump(trigger, source, phase)")
	if idx < 0 {
		t.Fatal("D37ag-diagnostic-geometry-dump: _emitGeometryDump must exist")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ag-diagnostic-geometry-dump: _emitGeometryDump body must be well-formed")
	}
	body := tail[:end+1]

	forbidden := []string{
		"cy.viewport(",
		"cy.fit(",
		"cy.zoom({",
		"cy.zoom( {",
		"cy.pan({",
		"cy.pan( {",
		"cy.resize(",
		"cy.add(",
		"cy.remove(",
		"cy.elements().remove(",
		"_fitToUsableRect(",
		"_fitWithSafeArea(",
		"_runFitPipeline(",
		"_revealGraph(",
		"_tryPostRevealCorrection(",
	}
	for _, f := range forbidden {
		if strings.Contains(body, f) {
			t.Errorf("D37ag-diagnostic-geometry-dump: _emitGeometryDump must be read-only; mutation-token %q must not appear in body", f)
		}
	}
}

// ── 14. State-transition emission inside _transitionTo ──────────────
//
// D37ah-readiness-gated-initial-fit — Adds a fifth call site that
// fires the geometry dump on every lifecycle state change. The dump
// uses trigger='state_transition', source=<newState>, phase=null and
// MUST be emitted immediately after the state_changed lifecycle
// diagnostic.

func TestExplorer_D37agGeometryDump_StateTransitionEmission(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37agEngineModule)

	idx := strings.Index(js, "function _transitionTo(newState)")
	if idx < 0 {
		t.Fatal("D37ag-diagnostic-geometry-dump: _transitionTo(newState) must exist (D37ah)")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ag-diagnostic-geometry-dump: _transitionTo body must be well-formed")
	}
	body := tail[:end+1]

	stateChanged := "_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_LIFECYCLE_STATE_CHANGED,"
	dumpCall := "_emitGeometryDump('state_transition', newState, null);"

	if !strings.Contains(body, stateChanged) {
		t.Errorf("D37ag-diagnostic-geometry-dump: _transitionTo must emit DIAG_ENGINE_INITIAL_LIFECYCLE_STATE_CHANGED")
	}
	if !strings.Contains(body, dumpCall) {
		t.Errorf("D37ag-diagnostic-geometry-dump: _transitionTo must emit %s", dumpCall)
	}
	scIdx := strings.Index(body, stateChanged)
	dumpIdx := strings.Index(body, dumpCall)
	if scIdx < 0 || dumpIdx < 0 || !(scIdx < dumpIdx) {
		t.Errorf("D37ag-diagnostic-geometry-dump: state-transition dump must appear AFTER the state_changed emission so the dump carries the new state")
	}
}
