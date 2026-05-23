package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37ah-readiness-gated-initial-fit — Readiness-gated two-mode lifecycle.
// CLEAN BREAK from D37af. Replaces the time-gated stabilisation loop with
// a six-state machine driven by readiness, not by elapsed time:
//
//   awaiting_container       — container has zero size at mount.
//   stabilising              — rAF readiness loop checks 9 fit-input gates
//                              each frame; only attempts a fit when all
//                              gates pass; reveals after
//                              INITIAL_FIT_STABLE_FRAMES consecutive
//                              stable post-fit snapshots.
//   revealed                 — clean reveal (state transitions to this
//                              from 'stabilising').
//   forced_reveal_cy_zero    — safety cap fired while 'stabilising' with
//                              cy.width/height === 0.
//   forced_reveal_fit_failed — safety cap fired while 'stabilising' with
//                              cy sized but no successful fit.
//   blocked_zero_container   — safety cap fired while still
//                              'awaiting_container' — graph stays hidden.
//
// Readiness gates (9 fields), checked each rAF in 'stabilising':
//   containerRect.width/height, cy.width/height, usableRect.width/height
//   (when provided), elementsBbox.w/h, and finally _runFitPipeline
//   reporting fitApplied = true. Failing input gates emit
//   engine_readiness_gate_failed. A failed fit pipeline emits
//   engine_readiness_fit_failed.
//
// Safety cap (single external setTimeout at INITIAL_FIT_SAFETY_MS):
//   • state === 'awaiting_container'
//       → _transitionTo('blocked_zero_container'); container stays hidden;
//         emit engine_initial_mount_container_blocked.
//   • state === 'stabilising' AND cy.width/height === 0
//       → _revealGraph('forced_cy_zero').
//   • state === 'stabilising' AND cy.width/height > 0
//       → _revealGraph('forced_fit_failed').
//
// _revealGraph(reason) maps reason → newState, data-initial-fit, diag:
//   'clean'             → 'revealed' / 'complete' / engine_initial_reveal.
//   'forced_cy_zero'    → 'forced_reveal_cy_zero' / 'failed-reveal-cy-zero'
//                         / engine_initial_reveal_forced_cy_zero.
//   'forced_fit_failed' → 'forced_reveal_fit_failed' / 'failed-reveal-fit'
//                         / engine_initial_reveal_forced_fit_failed.
//
// Post-reveal correction: eligible from 'revealed' OR
// 'forced_reveal_cy_zero' (NOT 'forced_reveal_fit_failed' or
// 'blocked_zero_container').
//
// State transitions go through _transitionTo, which emits
// engine_initial_lifecycle_state_changed plus a co-located geometry dump
// (trigger='state_transition', source=newState, phase=null).
//
// Invariants pinned by this file:
//   • Container hidden at mount.
//   • All D37ah constants + new diagnostic codes declared.
//   • Legacy 'pending' state + DIAG_ENGINE_INITIAL_REVEAL_FORCED removed.
//   • Six-state vocabulary present; initial state 'awaiting_container'.
//   • _transitionTo helper exists and emits both diagnostics.
//   • _takeFitSnapshot + _snapshotsMatch shape preserved.
//   • Readiness loop has 9 gates + the three readiness diagnostic codes.
//   • _scheduleNextStabilisationFrame uses rAF and state-gates on
//     'stabilising'.
//   • _revealGraph(reason) maps each of the three reasons correctly.
//   • _tryPostRevealCorrection gated on 'revealed' OR
//     'forced_reveal_cy_zero'.
//   • _onContainerResize state-branches; awaiting_container promotes to
//     stabilising on sized rect; passive ticks remain camera-preserving.
//   • Safety cap setTimeout applies the three-way rule.
//   • destroy() cancels rAF + safety timer.
//   • _runFitPipeline / _fitToUsableRect / _fitWithSafeArea /
//     _resolveFitPadding byte-identical (verified via separate D37ag
//     pin tests).
//   • D37u, D37x, D37aa contracts preserved.
//   • Authority untouched.

const (
	d37ahEngineModule       = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37ahNativeLabelsModule = "/explorer/assets/js/graph/graph-platform/graph-native-labels.js"
	d37ahAuthorityPoc       = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

// ── 1. Container hidden at mount ────────────────────────────────────

func TestExplorer_D37ahInitialFit_ContainerHiddenAtMount(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "container.style.visibility = 'hidden';") {
		t.Errorf("D37ah-readiness-gated-initial-fit: engine must set container.style.visibility = 'hidden' at mount")
	}
	if !strings.Contains(js, "container.setAttribute('data-initial-fit', 'pending');") {
		t.Errorf("D37ah-readiness-gated-initial-fit: engine must tag data-initial-fit=\"pending\" at mount")
	}
	if strings.Contains(js, "container.style.display = 'none'") {
		t.Errorf("D37ah-readiness-gated-initial-fit: engine must NOT hide via display:none")
	}
}

// ── 2. Stability constants declared ─────────────────────────────────

func TestExplorer_D37ahInitialFit_StabilityConstantsDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	cases := []string{
		"var INITIAL_FIT_STABLE_FRAMES        = 4;",
		"var INITIAL_FIT_STABILITY_EPSILON_PX = 0.5;",
		"var INITIAL_FIT_SAFETY_MS            = 2000;",
		"var POST_REVEAL_STABILISATION_MS     = 1000;",
		"var POST_REVEAL_MAX_CORRECTIONS      = 1;",
	}
	for _, c := range cases {
		if !strings.Contains(js, c) {
			t.Errorf("D37ah-readiness-gated-initial-fit: engine must declare %q", c)
		}
	}
}

// ── 3. New diagnostic codes declared ────────────────────────────────

func TestExplorer_D37ahInitialFit_DiagnosticCodesDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	cases := []string{
		"DIAG_ENGINE_INITIAL_STABILISATION_STARTED       = 'engine_initial_stabilisation_started'",
		"DIAG_ENGINE_INITIAL_STABILISATION_SAMPLE        = 'engine_initial_stabilisation_sample'",
		"DIAG_ENGINE_INITIAL_STABILISATION_STABLE        = 'engine_initial_stabilisation_stable'",
		"DIAG_ENGINE_INITIAL_FIT_APPLIED                 = 'engine_initial_fit_applied'",
		"DIAG_ENGINE_INITIAL_REVEAL                      = 'engine_initial_reveal'",
		"DIAG_ENGINE_INITIAL_REVEAL_FORCED_CY_ZERO       = 'engine_initial_reveal_forced_cy_zero'",
		"DIAG_ENGINE_INITIAL_REVEAL_FORCED_FIT_FAILED    = 'engine_initial_reveal_forced_fit_failed'",
		"DIAG_ENGINE_POST_REVEAL_STABILISATION_FIT       = 'engine_post_reveal_stabilisation_fit'",
		"DIAG_ENGINE_INITIAL_LIFECYCLE_STATE_CHANGED     = 'engine_initial_lifecycle_state_changed'",
		"DIAG_ENGINE_READINESS_GATE_FAILED               = 'engine_readiness_gate_failed'",
		"DIAG_ENGINE_READINESS_FIT_FAILED                = 'engine_readiness_fit_failed'",
		"DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_UNSIZED     = 'engine_initial_mount_container_unsized'",
		"DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_BLOCKED     = 'engine_initial_mount_container_blocked'",
	}
	for _, c := range cases {
		if !strings.Contains(js, c) {
			t.Errorf("D37ah-readiness-gated-initial-fit: engine must declare %q", c)
		}
	}
	// The D37af unified DIAG_ENGINE_INITIAL_REVEAL_FORCED has been split
	// in two — clean break, no backward compat.
	for _, banned := range []string{
		"DIAG_ENGINE_INITIAL_REVEAL_FORCED         = 'engine_initial_reveal_forced'",
		"DIAG_ENGINE_INITIAL_REVEAL_FORCED         =",
		"'engine_initial_reveal_forced'",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37ah-readiness-gated-initial-fit: legacy unified forced-reveal token %q must be removed (replaced by _CY_ZERO + _FIT_FAILED)", banned)
		}
	}
}

// ── 4. Six-state machine declared ───────────────────────────────────

func TestExplorer_D37ahInitialFit_StateMachineDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "var _initialFitState                 = 'awaiting_container';") {
		t.Errorf("D37ah-readiness-gated-initial-fit: engine must declare _initialFitState initialised to 'awaiting_container'")
	}
	for _, state := range []string{
		"'awaiting_container'",
		"'stabilising'",
		"'revealed'",
		"'forced_reveal_cy_zero'",
		"'forced_reveal_fit_failed'",
		"'blocked_zero_container'",
	} {
		if !strings.Contains(js, state) {
			t.Errorf("D37ah-readiness-gated-initial-fit: six-state machine must use %s", state)
		}
	}
	// Legacy D37af / D37ad executable state values must be gone.
	for _, banned := range []string{
		"_initialFitState = 'pending'",
		"_initialFitState === 'pending'",
		"_initialFitState !== 'pending'",
		"_initialFitState = 'forced_reveal'",
		"_initialFitState === 'forced_reveal'",
		"_initialFitState !== 'forced_reveal'",
		"_initialFitState = 'fitted_pending_reveal'",
		"_initialFitState === 'fitted_pending_reveal'",
		"_initialFitState !== 'fitted_pending_reveal'",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37ah-readiness-gated-initial-fit: legacy state token %q must be removed (clean break)", banned)
		}
	}
}

// ── 5. _transitionTo helper emits state_changed + geometry dump ─────

func TestExplorer_D37ahInitialFit_TransitionToHelperShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "function _transitionTo(newState)") {
		t.Fatal("D37ah-readiness-gated-initial-fit: engine must define _transitionTo(newState)")
	}
	idx := strings.Index(js, "function _transitionTo(newState)")
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: _transitionTo body must be well-formed")
	}
	body := tail[:end+1]

	required := []string{
		"if (_initialFitState === newState) return;",
		"var prevState = _initialFitState;",
		"_initialFitState = newState;",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_LIFECYCLE_STATE_CHANGED, {",
		"from: prevState,",
		"to:   newState,",
		"_emitGeometryDump('state_transition', newState, null);",
	}
	for _, r := range required {
		if !strings.Contains(body, r) {
			t.Errorf("D37ah-readiness-gated-initial-fit: _transitionTo must contain %q", r)
		}
	}
}

// ── 6. _takeFitSnapshot reads all required geometry ────────────────

func TestExplorer_D37ahInitialFit_SnapshotHelperReadsFitGeometry(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "function _takeFitSnapshot()") {
		t.Fatal("D37ah-readiness-gated-initial-fit: engine must define _takeFitSnapshot()")
	}
	idx := strings.Index(js, "function _takeFitSnapshot()")
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: _takeFitSnapshot body must be well-formed")
	}
	body := tail[:end+1]

	required := []string{
		"cw = _isFn(cy.width)  ? cy.width()  : 0",
		"ch = _isFn(cy.height) ? cy.height() : 0",
		"opts.getUsableGraphRect()",
		"eles.boundingBox(FIT_BBOX_OPTS)",
	}
	for _, r := range required {
		if !strings.Contains(body, r) {
			t.Errorf("D37ah-readiness-gated-initial-fit: _takeFitSnapshot must read %q for fit-readiness", r)
		}
	}
}

// ── 7. _snapshotsMatch uses epsilon comparison ─────────────────────

func TestExplorer_D37ahInitialFit_SnapshotsMatchUsesEpsilon(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "function _snapshotsMatch(a, b, epsilon)") {
		t.Fatal("D37ah-readiness-gated-initial-fit: _snapshotsMatch must exist")
	}
	idx := strings.Index(js, "function _snapshotsMatch(a, b, epsilon)")
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: _snapshotsMatch body must be well-formed")
	}
	body := tail[:end+1]

	if !strings.Contains(body, "INITIAL_FIT_STABILITY_EPSILON_PX") {
		t.Errorf("D37ah-readiness-gated-initial-fit: _snapshotsMatch must use INITIAL_FIT_STABILITY_EPSILON_PX as the default epsilon")
	}
	if !strings.Contains(body, "if (Math.abs((a[k] || 0) - (b[k] || 0)) > eps) return false;") {
		t.Errorf("D37ah-readiness-gated-initial-fit: _snapshotsMatch must compare each key with abs-diff > eps")
	}
}

// ── 8. Readiness loop body shape ───────────────────────────────────

func TestExplorer_D37ahInitialFit_ReadinessLoopShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "function _runStabilisationFrame()") {
		t.Fatal("D37ah-readiness-gated-initial-fit: engine must define _runStabilisationFrame()")
	}
	idx := strings.Index(js, "function _runStabilisationFrame()")
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: _runStabilisationFrame body must be well-formed")
	}
	body := tail[:end+1]

	// State gate is 'stabilising' (not 'pending').
	if !strings.Contains(body, "if (_initialFitState !== 'stabilising') return;") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must state-gate on 'stabilising'")
	}
	// The rAF-internal time-cap short-circuit MUST be removed (single
	// external setTimeout owns the safety cap now).
	if strings.Contains(body, "INITIAL_FIT_SAFETY_MS") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop body must NOT reference INITIAL_FIT_SAFETY_MS (the external safety-cap setTimeout owns the cap)")
	}
	if strings.Contains(body, "_revealGraph(true)") || strings.Contains(body, "_revealGraph(false)") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must NOT call boolean _revealGraph(...) (reason-based API only)")
	}
	// cy.resize() per frame.
	if !strings.Contains(body, "cy.resize()") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must call cy.resize() per frame")
	}
	// 9 readiness gates — each emits engine_readiness_gate_failed when
	// the corresponding field is missing / zero. Gate names match the
	// fields pinned in the D37ah design.
	for _, gate := range []string{
		"gate: 'containerRect.width'",
		"gate: 'containerRect.height'",
		"gate: 'cy.width'",
		"gate: 'cy.height'",
		"gate: 'usableRect.width'",
		"gate: 'usableRect.height'",
		"gate: 'elementsBbox.w'",
		"gate: 'elementsBbox.h'",
	} {
		if !strings.Contains(body, gate) {
			t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must label its failing-gate diagnostic with %q", gate)
		}
	}
	if !strings.Contains(body, "_emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_GATE_FAILED, {") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must emit DIAG_ENGINE_READINESS_GATE_FAILED with a gate payload on input-gate failure")
	}
	// Canonical fit call preserved (D37u + D37s + D37ad pin).
	if !strings.Contains(body, "var fitApplied = _runFitPipeline('initial_mount', 'stabilising');") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must invoke _runFitPipeline('initial_mount', 'stabilising')")
	}
	// Geometry dump after the canonical fit call.
	if !strings.Contains(body, "_emitGeometryDump('stabilisation_tick', 'initial_mount', 'stabilising');") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must emit stabilisation_tick geometry dump after the fit call")
	}
	// Gate 9: fit-pipeline failure emits engine_readiness_fit_failed.
	if !strings.Contains(body, "_emitLifecycleDiagnostic(DIAG_ENGINE_READINESS_FIT_FAILED);") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must emit DIAG_ENGINE_READINESS_FIT_FAILED when _runFitPipeline returns false")
	}
	// Stability counter logic.
	if !strings.Contains(body, "_stableFrameCount++") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must increment _stableFrameCount on matching post-fit snapshots")
	}
	if !strings.Contains(body, "_stableFrameCount = 1;") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must reset _stableFrameCount on non-matching snapshots")
	}
	if !strings.Contains(body, "if (_stableFrameCount >= INITIAL_FIT_STABLE_FRAMES)") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must reveal once _stableFrameCount reaches INITIAL_FIT_STABLE_FRAMES")
	}
	if !strings.Contains(body, "_revealGraph('clean')") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must call _revealGraph('clean') on stable reveal")
	}
	// Reschedule on each non-terminal exit.
	if !strings.Contains(body, "_scheduleNextStabilisationFrame()") {
		t.Errorf("D37ah-readiness-gated-initial-fit: readiness loop must reschedule the next animation frame on non-terminal exits")
	}
}

// ── 9. Scheduler uses rAF and state-gates on 'stabilising' ─────────

func TestExplorer_D37ahInitialFit_LoopSchedulerUsesRAF(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "function _scheduleNextStabilisationFrame()") {
		t.Fatal("D37ah-readiness-gated-initial-fit: engine must define _scheduleNextStabilisationFrame()")
	}
	idx := strings.Index(js, "function _scheduleNextStabilisationFrame()")
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: _scheduleNextStabilisationFrame body must be well-formed")
	}
	body := tail[:end+1]

	if !strings.Contains(body, "if (_initialFitState !== 'stabilising') return;") {
		t.Errorf("D37ah-readiness-gated-initial-fit: scheduler must state-gate on 'stabilising'")
	}
	if !strings.Contains(body, "_stabilisationRaf = window.requestAnimationFrame(_runStabilisationFrame);") {
		t.Errorf("D37ah-readiness-gated-initial-fit: scheduler must use requestAnimationFrame for the rAF loop")
	}
	// Legacy D37ad cadence + D37af 'pending' must not appear.
	for _, banned := range []string{
		"function _settleEarly(phase)",
		"function _settleFinal()",
		"function _attemptInitialFit(phase)",
		"function _scheduleInitialReveal()",
		"setTimeout(_settleFinal, 120)",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37ah-readiness-gated-initial-fit: legacy cadence symbol %q must remain removed (readiness loop replaces it)", banned)
		}
	}
}

// ── 10. _revealGraph(reason) — clean / cy_zero / fit_failed mapping ─

func TestExplorer_D37ahInitialFit_RevealGraphReasonMapping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "function _revealGraph(reason)") {
		t.Fatal("D37ah-readiness-gated-initial-fit: _revealGraph(reason) must exist (reason-based API)")
	}
	idx := strings.Index(js, "function _revealGraph(reason)")
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		end = strings.Index(tail, "\n    // ──")
		if end < 0 {
			t.Fatalf("D37ah-readiness-gated-initial-fit: _revealGraph body must be well-formed")
		}
	}
	body := tail[:end+1]

	// Re-entry guard covers all four terminal-reveal states.
	for _, terminal := range []string{
		"_initialFitState === 'revealed'",
		"_initialFitState === 'forced_reveal_cy_zero'",
		"_initialFitState === 'forced_reveal_fit_failed'",
		"_initialFitState === 'blocked_zero_container'",
	} {
		if !strings.Contains(body, terminal) {
			t.Errorf("D37ah-readiness-gated-initial-fit: _revealGraph re-entry guard must check %q", terminal)
		}
	}
	// Reason → state / attr / diag mapping.
	required := []string{
		// 'clean'
		"newState   = 'revealed';",
		"attrValue  = 'complete';",
		"revealDiag = DIAG_ENGINE_INITIAL_REVEAL;",
		// 'forced_cy_zero'
		"newState   = 'forced_reveal_cy_zero';",
		"attrValue  = 'failed-reveal-cy-zero';",
		"revealDiag = DIAG_ENGINE_INITIAL_REVEAL_FORCED_CY_ZERO;",
		// 'forced_fit_failed'
		"newState   = 'forced_reveal_fit_failed';",
		"attrValue  = 'failed-reveal-fit';",
		"revealDiag = DIAG_ENGINE_INITIAL_REVEAL_FORCED_FIT_FAILED;",
		// Goes through _transitionTo (canonical state change path).
		"_transitionTo(newState);",
		"container.style.visibility = '';",
		"container.setAttribute('data-initial-fit', attrValue);",
		"_postRevealGraceEndTs = now + POST_REVEAL_STABILISATION_MS;",
		"_bindUserInteractionGuards();",
		"_emitLifecycleDiagnostic(revealDiag);",
		"_emitGeometryDump('initial_reveal', null, null);",
	}
	for _, r := range required {
		if !strings.Contains(body, r) {
			t.Errorf("D37ah-readiness-gated-initial-fit: _revealGraph must contain %q", r)
		}
	}
	// The legacy ternary `forced ? ... : ...` emission must be gone.
	if strings.Contains(body, "forced ? DIAG_ENGINE_INITIAL_REVEAL_FORCED") {
		t.Errorf("D37ah-readiness-gated-initial-fit: legacy boolean ternary forced ? ... must be removed")
	}
	if strings.Contains(body, "forced ? 'failed-reveal' : 'complete'") {
		t.Errorf("D37ah-readiness-gated-initial-fit: legacy boolean ternary for data-initial-fit must be removed")
	}
}

// ── 11. User-interaction guards wire cy events ──────────────────────

func TestExplorer_D37ahInitialFit_UserInteractionGuardsWiredToCyEvents(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "function _bindUserInteractionGuards()") {
		t.Errorf("D37ah-readiness-gated-initial-fit: engine must define _bindUserInteractionGuards()")
	}
	if !strings.Contains(js, "cy.on('pan zoom drag tap', function () { _userInteracted = true; })") {
		t.Errorf("D37ah-readiness-gated-initial-fit: user-interaction guard must subscribe to cy 'pan zoom drag tap' and set _userInteracted = true")
	}
}

// ── 12. Post-reveal correction allowed from 'revealed' or cy_zero ──

func TestExplorer_D37ahInitialFit_PostRevealCorrectionGuards(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	idx := strings.Index(js, "function _tryPostRevealCorrection()")
	if idx < 0 {
		t.Fatal("D37ah-readiness-gated-initial-fit: _tryPostRevealCorrection must exist")
	}
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		end = strings.Index(tail, "\n    // ──")
		if end < 0 {
			t.Fatalf("D37ah-readiness-gated-initial-fit: _tryPostRevealCorrection body must be well-formed")
		}
	}
	body := tail[:end+1]

	required := []string{
		"if (_initialFitState !== 'revealed' &&",
		"_initialFitState !== 'forced_reveal_cy_zero') return false;",
		"if (_postRevealCorrectionsRemaining <= 0) return false;",
		"if (_userInteracted) return false;",
		"if (now > _postRevealGraceEndTs) return false;",
		"var sample = _takeFitSnapshot();",
		"_snapshotsMatch(sample.snapshot, _revealSnapshot)",
		"_postRevealCorrectionsRemaining--;",
		"_runFitPipeline('post_reveal_stabilisation', null);",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_POST_REVEAL_STABILISATION_FIT);",
	}
	for _, r := range required {
		if !strings.Contains(body, r) {
			t.Errorf("D37ah-readiness-gated-initial-fit: _tryPostRevealCorrection must contain %q", r)
		}
	}
	// MUST NOT allow correction from the other two terminal states.
	if !strings.Contains(body, "_initialFitState !== 'revealed' &&") ||
		!strings.Contains(body, "_initialFitState !== 'forced_reveal_cy_zero'") {
		t.Errorf("D37ah-readiness-gated-initial-fit: _tryPostRevealCorrection state guard must use the explicit two-state predicate")
	}
}

// ── 13. _onContainerResize state-branching dispatch ─────────────────

func TestExplorer_D37ahInitialFit_OnResizeStateBranches(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ah-readiness-gated-initial-fit: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	required := []string{
		// Always-on lifecycle.
		"_emitLifecycleDiagnostic(DIAG_ENGINE_RESIZE_DETECTED)",
		"cy.resize()",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_CY_RESIZE_CALLED)",
		// awaiting_container promotion branch.
		"if (_initialFitState === 'awaiting_container')",
		"cr = container.getBoundingClientRect();",
		"if (cr && cr.width > 0 && cr.height > 0)",
		"_transitionTo('stabilising');",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_STARTED)",
		"_scheduleNextStabilisationFrame();",
		// stabilising no-op branch.
		"if (_initialFitState === 'stabilising')",
		// post-reveal correction branch.
		"if (_tryPostRevealCorrection()) return;",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)",
	}
	for _, r := range required {
		if !strings.Contains(rBody, r) {
			t.Errorf("D37ah-readiness-gated-initial-fit: _onContainerResize must contain %q", r)
		}
	}
}

// ── 14. D37u passive resize policy preserved (no camera mutation) ──

func TestExplorer_D37ahInitialFit_OnResizeForbidsCameraMutationTokens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ah-readiness-gated-initial-fit: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

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
		if strings.Contains(rBody, f) {
			t.Errorf("D37ah-readiness-gated-initial-fit: D37u regression — %q must not literally appear in _onContainerResize body", f)
		}
	}
}

// ── 15. Mount kick-off — initial state decision ─────────────────────

func TestExplorer_D37ahInitialFit_MountKickOffByContainerReadiness(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	required := []string{
		"var initialContainerRect = null;",
		"initialContainerRect = container.getBoundingClientRect();",
		"if (initialContainerRect &&",
		"initialContainerRect.width > 0 &&",
		"initialContainerRect.height > 0)",
		"_transitionTo('stabilising');",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_STARTED);",
		"_scheduleNextStabilisationFrame();",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_UNSIZED);",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37ah-readiness-gated-initial-fit: mount kick-off must contain %q", r)
		}
	}
}

// ── 16. Safety cap — single external setTimeout + three-way rule ───

func TestExplorer_D37ahInitialFit_SafetyCapThreeWayRule(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "_initialFitRevealTimer = setTimeout(function () {") {
		t.Errorf("D37ah-readiness-gated-initial-fit: engine must arm a single external safety-cap setTimeout")
	}
	if !strings.Contains(js, "}, INITIAL_FIT_SAFETY_MS);") {
		t.Errorf("D37ah-readiness-gated-initial-fit: safety-cap setTimeout must use INITIAL_FIT_SAFETY_MS (2000 ms)")
	}
	// Three-way fan-out body.
	required := []string{
		// Rule 1: awaiting_container → blocked_zero_container.
		"if (_initialFitState === 'awaiting_container')",
		"_transitionTo('blocked_zero_container');",
		"container.setAttribute('data-initial-fit', 'blocked-zero-container');",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_BLOCKED);",
		// Rule 2: stabilising + cy zero → forced_cy_zero.
		"if (_initialFitState !== 'stabilising') return;",
		"capCw = _isFn(cy.width)",
		"capCh = _isFn(cy.height)",
		"if (!(capCw > 0) || !(capCh > 0))",
		"_revealGraph('forced_cy_zero');",
		// Rule 3: stabilising + cy positive → forced_fit_failed.
		"_revealGraph('forced_fit_failed');",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37ah-readiness-gated-initial-fit: safety-cap body must contain %q", r)
		}
	}
	// The legacy unified _revealGraph(true) MUST be gone.
	if strings.Contains(js, "_revealGraph(true);") {
		t.Errorf("D37ah-readiness-gated-initial-fit: legacy unified _revealGraph(true) must be removed (reason-based API only)")
	}
}

// ── 17. destroy() cancels rAF + safety timer ────────────────────────

func TestExplorer_D37ahInitialFit_DestroyCancelsLoopAndSafety(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	dIdx := strings.Index(js, "destroy: function ()")
	if dIdx < 0 {
		t.Fatal("D37ah-readiness-gated-initial-fit: handle.destroy must exist")
	}
	dTail := js[dIdx:]
	dEnd := strings.Index(dTail, "\n      },\n")
	if dEnd < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: handle.destroy body must be well-formed")
	}
	dBody := dTail[:dEnd+1]

	if !strings.Contains(dBody, "clearTimeout(_initialFitRevealTimer)") {
		t.Errorf("D37ah-readiness-gated-initial-fit: destroy() must clearTimeout the safety-cap timer")
	}
	if !strings.Contains(dBody, "window.cancelAnimationFrame(_stabilisationRaf)") {
		t.Errorf("D37ah-readiness-gated-initial-fit: destroy() must cancelAnimationFrame the readiness loop")
	}
}

// ── 18. handle.fit canonical path preserved ─────────────────────────

func TestExplorer_D37ahInitialFit_HandleFitCanonicalPathUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	hIdx := strings.Index(js, "fit: function (fitOpts) {")
	if hIdx < 0 {
		t.Fatal("D37ah-readiness-gated-initial-fit: handle.fit must exist")
	}
	hTail := js[hIdx:]
	hEnd := strings.Index(hTail, "\n      },\n")
	if hEnd < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: handle.fit body must be well-formed")
	}
	hBody := hTail[:hEnd+1]

	if !strings.Contains(hBody, "opts.getUsableGraphRect()") {
		t.Errorf("D37ah-readiness-gated-initial-fit: handle.fit must still consult getUsableGraphRect()")
	}
	if !strings.Contains(hBody, "_fitToUsableRect(cy, usable, _recordFitDiagnostic)") {
		t.Errorf("D37ah-readiness-gated-initial-fit: handle.fit must still call _fitToUsableRect")
	}
	if !strings.Contains(hBody, "_fitWithSafeArea(cy, _resolveFitPadding") {
		t.Errorf("D37ah-readiness-gated-initial-fit: handle.fit must still fall back to _fitWithSafeArea")
	}
}

// ── 19. D37u passive resize lifecycle preserved (full pin) ──────────

func TestExplorer_D37ahInitialFit_D37uResizeContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ah-readiness-gated-initial-fit: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ah-readiness-gated-initial-fit: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	if !strings.Contains(rBody, "cy.resize()") {
		t.Errorf("D37ah-readiness-gated-initial-fit: D37u passive resize must still call cy.resize()")
	}
	if !strings.Contains(rBody, "_emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)") {
		t.Errorf("D37ah-readiness-gated-initial-fit: D37u passive resize must still emit DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE on non-corrective ticks")
	}
}

// ── 20. D37x geometry/label contracts preserved ─────────────────────

func TestExplorer_D37ahInitialFit_D37xContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	engineJS := getExplorerAsset(t, srv, d37ahEngineModule)
	labelsJS := getExplorerAsset(t, srv, d37ahNativeLabelsModule)

	required := []string{
		"var FIT_BBOX_OPTS = {",
		"includeLabels:       false",
		"includeOverlays:     false",
		"includeOutlines:     false",
		"eles.boundingBox(FIT_BBOX_OPTS)",
		"'width':              'data(width)'",
		"'height':             'data(height)'",
		"'label':              ''",
	}
	for _, r := range required {
		if !strings.Contains(engineJS, r) {
			t.Errorf("D37ah-readiness-gated-initial-fit: D37x contract regression — engine missing %q", r)
		}
	}
	if !strings.Contains(labelsJS, "function makeNativeNodeLabel(text, width, height, options)") {
		t.Errorf("D37ah-readiness-gated-initial-fit: D37x native-labels helper regression")
	}
}

// ── 21. Authority source untouched ─────────────────────────────────

func TestExplorer_D37ahInitialFit_AuthorityUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJS := getExplorerAsset(t, srv, d37ahAuthorityPoc)

	if !strings.Contains(authJS, "function _refitWithSafeArea()") {
		t.Errorf("D37ah-readiness-gated-initial-fit: Authority _refitWithSafeArea must remain")
	}
	if !strings.Contains(authJS, "ctx.onResize(_refitWithSafeArea)") {
		t.Errorf("D37ah-readiness-gated-initial-fit: Authority ctx.onResize subscription must remain")
	}
	if strings.Contains(authJS, "_initialFitState") {
		t.Errorf("D37ah-readiness-gated-initial-fit: Authority must not have absorbed engine state")
	}
	if strings.Contains(authJS, "_takeFitSnapshot") {
		t.Errorf("D37ah-readiness-gated-initial-fit: Authority must not have absorbed engine snapshot helpers")
	}
	if strings.Contains(authJS, "_transitionTo") {
		t.Errorf("D37ah-readiness-gated-initial-fit: Authority must not have absorbed engine state-transition helper")
	}
}

// ---------------------------------------------------------------------------
// D37ai-blocked-container-recovery — Recovery from zero-container initial
// mount.
//
// D37ah introduced the six-state lifecycle but left two of the
// initial-stabilisation states ('awaiting_container', 'blocked_zero_container')
// flowing through the steady-state camera-preserved branch on ResizeObserver
// ticks. 'blocked_zero_container' was not handled explicitly at all, so a
// later resize tick delivering a measurable container was misinterpreted as
// "passive steady-state resize, preserve camera" rather than "the container
// is finally measurable, restart initial stabilisation". The graph stayed
// blank.
//
// D37ai fixes this:
//   • _onContainerResize now branches explicitly on 'blocked_zero_container'
//     and 'awaiting_container'. When the container is measurable, it
//     transitions to 'stabilising', kicks off the readiness loop, and (for
//     the blocked branch) re-arms the safety-cap timer from the recovery
//     moment.
//   • Inside the readiness loop's awaiting / blocked / stabilising branches,
//     engine_camera_preserved_on_resize is NOT emitted — that diagnostic is
//     reserved for steady-state ticks after a real reveal.
//   • The mount-time inline setTimeout was extracted into a shared helper
//     _armInitialSafetyCap() that the blocked-recovery branch also calls.
//
// Invariants pinned by this section:
//   • _armInitialSafetyCap helper exists, applies the three-way rule, and
//     clears any pending timer before re-arming.
//   • _onContainerResize branches: awaiting / blocked / stabilising /
//     steady-state.
//   • Blocked branch resets _lastFitSnapshot / _stableFrameCount /
//     _revealSnapshot, transitions to 'stabilising', resets
//     data-initial-fit to 'pending', re-arms the cap, schedules the loop.
//   • Awaiting branch in _onContainerResize does NOT emit
//     camera_preserved.
//   • Blocked branch in _onContainerResize does NOT emit
//     camera_preserved.
//   • Stabilising branch in _onContainerResize does NOT emit
//     camera_preserved.
//   • _onContainerResize body emits camera_preserved exactly ONCE (in the
//     steady-state branch).
//   • _onContainerResize body emits cy_resize_called exactly ONCE
//     (steady-state branch); cy.resize() and the cy_resize_called emit live
//     ONLY in the steady-state branch.
//   • The safety-cap setTimeout no longer lives inline at mount-time; it
//     lives inside the helper.
//   • Hard refresh path: a blocked-then-recovered mount reaches
//     'stabilising' on the same ResizeObserver tick that delivers a sized
//     rect; no force-reveal from zero-container.

// ── 22. _armInitialSafetyCap helper exists with three-way rule ──────

func TestExplorer_D37aiBlockedRecovery_ArmSafetyCapHelperShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	if !strings.Contains(js, "function _armInitialSafetyCap()") {
		t.Fatal("D37ai-blocked-container-recovery: engine must define _armInitialSafetyCap()")
	}
	idx := strings.Index(js, "function _armInitialSafetyCap()")
	tail := js[idx:]
	end := strings.Index(tail[1:], "\n    function ")
	if end < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _armInitialSafetyCap body must be well-formed")
	}
	body := tail[:end+1]

	required := []string{
		// Idempotence: clears any pending timer before re-arming.
		"if (_initialFitRevealTimer) {",
		"clearTimeout(_initialFitRevealTimer)",
		"_initialFitRevealTimer = 0;",
		// New external setTimeout (the single safety-cap mechanism).
		"_initialFitRevealTimer = setTimeout(function () {",
		"}, INITIAL_FIT_SAFETY_MS);",
		// Three-way rule:
		"if (_initialFitState === 'awaiting_container')",
		"_transitionTo('blocked_zero_container');",
		"container.setAttribute('data-initial-fit', 'blocked-zero-container');",
		"_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_BLOCKED);",
		"if (_initialFitState !== 'stabilising') return;",
		"capCw = _isFn(cy.width)",
		"capCh = _isFn(cy.height)",
		"if (!(capCw > 0) || !(capCh > 0))",
		"_revealGraph('forced_cy_zero');",
		"_revealGraph('forced_fit_failed');",
	}
	for _, r := range required {
		if !strings.Contains(body, r) {
			t.Errorf("D37ai-blocked-container-recovery: _armInitialSafetyCap must contain %q", r)
		}
	}
}

// ── 23. Mount-time uses the helper (no inline setTimeout) ───────────

func TestExplorer_D37aiBlockedRecovery_MountUsesArmSafetyCapHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	// The mount-time block must invoke the helper. Locate the
	// MountKickOff section and assert the call appears after the
	// stabilising kick-off / unsized fallback.
	mountMarker := "_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_MOUNT_CONTAINER_UNSIZED);"
	mIdx := strings.Index(js, mountMarker)
	if mIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: mount-time unsized-container diagnostic emission must remain")
	}
	// After the mount-time decision block, the engine must arm the
	// safety cap via the helper (rather than re-introducing the inline
	// setTimeout that previously lived here).
	mountTail := js[mIdx:]
	if !strings.Contains(mountTail, "_armInitialSafetyCap();") {
		t.Errorf("D37ai-blocked-container-recovery: mount-time must call _armInitialSafetyCap(); to arm the cap (no inline setTimeout)")
	}
	// The mount-kickoff section (after MOUNT_CONTAINER_UNSIZED, before
	// `return handle;`) must NOT contain a literal `setTimeout` — that
	// would mean the inline cap survived.
	retIdx := strings.Index(mountTail, "return handle;")
	if retIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: mount() must end with `return handle;`")
	}
	mountKickOffTail := mountTail[:retIdx]
	if strings.Contains(mountKickOffTail, "setTimeout(") {
		t.Errorf("D37ai-blocked-container-recovery: mount-kickoff section must not contain an inline setTimeout (extracted into _armInitialSafetyCap)")
	}
}

// ── 24. _onContainerResize has a blocked_zero_container recovery branch ──

func TestExplorer_D37aiBlockedRecovery_OnResizeHasBlockedRecoveryBranch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	required := []string{
		// Explicit branch.
		"if (_initialFitState === 'blocked_zero_container')",
		// Measurability check.
		"crBlocked = container.getBoundingClientRect();",
		"if (crBlocked && crBlocked.width > 0 && crBlocked.height > 0)",
		// Recovery reset of stability state.
		"_lastFitSnapshot = null;",
		"_stableFrameCount = 0;",
		"_revealSnapshot  = null;",
		// Promote to stabilising via _transitionTo (the only canonical
		// path that fires state_changed + state_transition geometry dump).
		"_transitionTo('stabilising');",
		// DOM-attr reset to 'pending' so the data-initial-fit reflects
		// the new active phase.
		"container.setAttribute('data-initial-fit', 'pending');",
		// Re-arm the safety cap from the recovery moment.
		"_armInitialSafetyCap();",
		// Kick off the readiness loop.
		"_emitLifecycleDiagnostic(DIAG_ENGINE_INITIAL_STABILISATION_STARTED)",
		"_scheduleNextStabilisationFrame();",
	}
	for _, r := range required {
		if !strings.Contains(rBody, r) {
			t.Errorf("D37ai-blocked-container-recovery: blocked branch must contain %q", r)
		}
	}
	// The branch must NOT call _revealGraph — recovery means
	// re-attempt stabilisation, not force-reveal-from-zero. (Search
	// only the blocked-branch text range so calls elsewhere are
	// irrelevant.)
	blockedStart := strings.Index(rBody, "if (_initialFitState === 'blocked_zero_container')")
	if blockedStart < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: locate blocked branch")
	}
	// Find the close brace of the blocked branch — it ends at the next
	// "return;\n      }" that closes the branch.
	blockedTail := rBody[blockedStart:]
	blockedEnd := strings.Index(blockedTail, "return;\n      }")
	if blockedEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: blocked branch must be well-formed")
	}
	blockedBody := blockedTail[:blockedEnd]
	if strings.Contains(blockedBody, "_revealGraph(") {
		t.Errorf("D37ai-blocked-container-recovery: blocked-recovery branch must NOT call _revealGraph (no force-reveal from zero-container; recover via stabilising instead)")
	}
}

// ── 25. _onContainerResize body emits camera_preserved exactly ONCE ──
//
// The diagnostic represents steady-state intent. The initial-
// stabilisation branches (awaiting / blocked / stabilising) must NOT
// emit it. After D37ai it appears exactly once, inside the steady-
// state fall-through branch.

func TestExplorer_D37aiBlockedRecovery_CameraPreservedEmittedOnlyInSteadyState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	camPreserved := "_emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)"
	if got := strings.Count(rBody, camPreserved); got != 1 {
		t.Errorf("D37ai-blocked-container-recovery: _onContainerResize must emit %s exactly once (only in the steady-state branch); got %d occurrences", camPreserved, got)
	}
	// Same for cy_resize_called: only emitted on the steady-state path.
	cyResizeCalled := "_emitLifecycleDiagnostic(DIAG_ENGINE_CY_RESIZE_CALLED)"
	if got := strings.Count(rBody, cyResizeCalled); got != 1 {
		t.Errorf("D37ai-blocked-container-recovery: _onContainerResize must emit %s exactly once (only in the steady-state branch); got %d occurrences", cyResizeCalled, got)
	}
	// Actual cy.resize() statement (not comment-text mentions): only
	// the steady-state branch calls it. (Initial-stabilisation states
	// defer to the readiness rAF loop, which calls cy.resize() at the
	// start of each frame.) Pin the wrapping `try { cy.resize(); }`
	// form to ignore any comment-text mentions of "cy.resize()" in
	// the function docblock.
	cyResizeStmt := "try { cy.resize(); }"
	if got := strings.Count(rBody, cyResizeStmt); got != 1 {
		t.Errorf("D37ai-blocked-container-recovery: _onContainerResize body must contain the cy.resize() statement %q exactly once (steady-state branch only); got %d occurrences", cyResizeStmt, got)
	}
}

// ── 26. Awaiting branch in _onContainerResize: no camera_preserved ──

func TestExplorer_D37aiBlockedRecovery_AwaitingBranchNoSteadyStateDiag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	startIdx := strings.Index(rBody, "if (_initialFitState === 'awaiting_container')")
	if startIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: awaiting branch must exist")
	}
	branchTail := rBody[startIdx:]
	endIdx := strings.Index(branchTail, "return;\n      }")
	if endIdx < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: awaiting branch must be well-formed")
	}
	branchBody := branchTail[:endIdx]

	forbidden := []string{
		"DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE",
		"DIAG_ENGINE_CY_RESIZE_CALLED",
	}
	for _, f := range forbidden {
		if strings.Contains(branchBody, f) {
			t.Errorf("D37ai-blocked-container-recovery: awaiting branch must NOT emit %q (steady-state diagnostic only)", f)
		}
	}
}

// ── 27. Blocked branch in _onContainerResize: no camera_preserved ───

func TestExplorer_D37aiBlockedRecovery_BlockedBranchNoSteadyStateDiag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	startIdx := strings.Index(rBody, "if (_initialFitState === 'blocked_zero_container')")
	if startIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: blocked branch must exist")
	}
	branchTail := rBody[startIdx:]
	endIdx := strings.Index(branchTail, "return;\n      }")
	if endIdx < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: blocked branch must be well-formed")
	}
	branchBody := branchTail[:endIdx]

	forbidden := []string{
		"DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE",
		"DIAG_ENGINE_CY_RESIZE_CALLED",
	}
	for _, f := range forbidden {
		if strings.Contains(branchBody, f) {
			t.Errorf("D37ai-blocked-container-recovery: blocked branch must NOT emit %q (steady-state diagnostic only)", f)
		}
	}
}

// ── 28. Stabilising branch: no camera_preserved emission ────────────

func TestExplorer_D37aiBlockedRecovery_StabilisingBranchNoSteadyStateDiag(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	startIdx := strings.Index(rBody, "if (_initialFitState === 'stabilising')")
	if startIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: stabilising branch must exist")
	}
	branchTail := rBody[startIdx:]
	endIdx := strings.Index(branchTail, "return;\n      }")
	if endIdx < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: stabilising branch must be well-formed")
	}
	branchBody := branchTail[:endIdx]

	forbidden := []string{
		"DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE",
		"DIAG_ENGINE_CY_RESIZE_CALLED",
		"cy.resize()",
	}
	for _, f := range forbidden {
		if strings.Contains(branchBody, f) {
			t.Errorf("D37ai-blocked-container-recovery: stabilising branch must NOT contain %q (the readiness rAF loop owns its own cy.resize() / pacing)", f)
		}
	}
}

// ── 29. Blocked-still-zero short-circuit returns without recovery ───
//
// When the blocked branch executes but the container rect is still
// zero, the branch must exit via `return;` without calling
// _transitionTo, _armInitialSafetyCap, or _scheduleNextStabilisation
// Frame. We assert structurally: the `_transitionTo('stabilising')`
// call inside the blocked branch is guarded by the
// `if (crBlocked && ...)` measurability check.

func TestExplorer_D37aiBlockedRecovery_BlockedStillZeroDoesNotRecover(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	startIdx := strings.Index(rBody, "if (_initialFitState === 'blocked_zero_container')")
	if startIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: blocked branch must exist")
	}
	branchTail := rBody[startIdx:]
	endIdx := strings.Index(branchTail, "return;\n      }")
	if endIdx < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: blocked branch must be well-formed")
	}
	branchBody := branchTail[:endIdx]

	// The measurability check must appear BEFORE _transitionTo so that
	// a still-zero container path skips recovery.
	gateIdx := strings.Index(branchBody, "if (crBlocked && crBlocked.width > 0 && crBlocked.height > 0)")
	transitionIdx := strings.Index(branchBody, "_transitionTo('stabilising');")
	if gateIdx < 0 || transitionIdx < 0 || !(gateIdx < transitionIdx) {
		t.Errorf("D37ai-blocked-container-recovery: blocked branch's measurability gate must precede the _transitionTo('stabilising') call (still-zero containers must short-circuit without recovering)")
	}
	// The branch must end with `return;` so the post-branch
	// steady-state path is NEVER taken from blocked state regardless
	// of measurability outcome.
	trimmed := strings.TrimRight(branchBody, " \t\n")
	if !strings.HasSuffix(trimmed, "return;\n      }") &&
		!strings.HasSuffix(trimmed, "return;") {
		// Note: branchBody is sliced before the literal "return;\n      }"
		// marker, so the closing 'return;' itself lives just past the
		// slice. Check the rBody continuation instead.
		afterIdx := startIdx + endIdx
		after := rBody[afterIdx:]
		if !strings.HasPrefix(after, "return;\n      }") {
			t.Errorf("D37ai-blocked-container-recovery: blocked branch must end with `return;` before fall-through (sanity check; got prefix %q)", after[:30])
		}
	}
}

// ── 30. Steady-state branch reachable only from non-initial states ──
//
// Structural check: after the awaiting / blocked / stabilising
// branches, the remaining fall-through path is the steady-state D37u
// path. We pin that the steady-state markers (cy.resize() + emits +
// post-reveal correction) appear AFTER the three initial-
// stabilisation branches in the body.

func TestExplorer_D37aiBlockedRecovery_SteadyStatePathFollowsInitialBranches(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	awaitIdx     := strings.Index(rBody, "if (_initialFitState === 'awaiting_container')")
	blockedIdx   := strings.Index(rBody, "if (_initialFitState === 'blocked_zero_container')")
	stabIdx      := strings.Index(rBody, "if (_initialFitState === 'stabilising')")
	// Pin the actual cy.resize() statement form (not comment-text
	// mentions in the function docblock).
	cyResizeIdx  := strings.Index(rBody, "try { cy.resize(); }")
	cyCalledIdx  := strings.Index(rBody, "_emitLifecycleDiagnostic(DIAG_ENGINE_CY_RESIZE_CALLED)")
	postRevealIdx := strings.Index(rBody, "if (_tryPostRevealCorrection()) return;")
	camPresIdx   := strings.Index(rBody, "_emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)")

	for name, v := range map[string]int{
		"awaiting":         awaitIdx,
		"blocked":          blockedIdx,
		"stabilising":      stabIdx,
		"cy.resize()":      cyResizeIdx,
		"cy_resize_called": cyCalledIdx,
		"post_reveal":      postRevealIdx,
		"camera_preserved": camPresIdx,
	} {
		if v < 0 {
			t.Fatalf("D37ai-blocked-container-recovery: marker %q must exist in _onContainerResize body", name)
		}
	}
	// Ordering: awaiting < blocked < stabilising < cy.resize() <
	// cy_resize_called < post_reveal < camera_preserved.
	if !(awaitIdx < blockedIdx &&
		blockedIdx < stabIdx &&
		stabIdx < cyResizeIdx &&
		cyResizeIdx < cyCalledIdx &&
		cyCalledIdx < postRevealIdx &&
		postRevealIdx < camPresIdx) {
		t.Errorf("D37ai-blocked-container-recovery: _onContainerResize body ordering must be awaiting → blocked → stabilising → steady-state (cy.resize, cy_resize_called, post_reveal, camera_preserved); got awaiting=%d blocked=%d stab=%d cyResize=%d cyCalled=%d postReveal=%d camPres=%d", awaitIdx, blockedIdx, stabIdx, cyResizeIdx, cyCalledIdx, postRevealIdx, camPresIdx)
	}
}

// ── 31. Fit math + canonical helpers unchanged (D37ag pin re-check) ─
//
// D37ai must not have touched fit math. The D37ag tests already pin
// _runFitPipeline / _fitToUsableRect / _fitWithSafeArea /
// _resolveFitPadding byte-by-byte; this test cross-checks that
// _onContainerResize's body still does not contain any of the
// forbidden mutation tokens (D37u + D37ah preservation).

func TestExplorer_D37aiBlockedRecovery_OnResizeStillHasNoCameraMutationTokens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	forbidden := []string{
		"_runFitPipeline",
		"_fitToUsableRect",
		"_fitWithSafeArea",
		"cy.viewport(",
		"cy.fit(",
		"cy.pan(",
		"cy.zoom(",
		"setTimeout", // helper is called by name, not via inline setTimeout
	}
	for _, f := range forbidden {
		if strings.Contains(rBody, f) {
			t.Errorf("D37ai-blocked-container-recovery: D37u/D37ah regression — %q must remain absent from _onContainerResize body", f)
		}
	}
}

// ── 32. destroy() still cancels the safety-cap timer ────────────────

func TestExplorer_D37aiBlockedRecovery_DestroyStillCancelsSafetyCap(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37ahEngineModule)

	dIdx := strings.Index(js, "destroy: function ()")
	if dIdx < 0 {
		t.Fatal("D37ai-blocked-container-recovery: handle.destroy must exist")
	}
	dTail := js[dIdx:]
	dEnd := strings.Index(dTail, "\n      },\n")
	if dEnd < 0 {
		t.Fatalf("D37ai-blocked-container-recovery: handle.destroy body must be well-formed")
	}
	dBody := dTail[:dEnd+1]

	// The same timer slot is reused by _armInitialSafetyCap; destroy
	// must still clear it.
	if !strings.Contains(dBody, "clearTimeout(_initialFitRevealTimer)") {
		t.Errorf("D37ai-blocked-container-recovery: destroy() must still clearTimeout the safety-cap timer (now armed via _armInitialSafetyCap helper)")
	}
}
