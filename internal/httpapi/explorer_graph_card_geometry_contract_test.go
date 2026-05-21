package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37s-context-geometry-2-impl — Global Card Geometry Contract
//
// Strategic principle: in all standard MIDAS graph-card views, visible
// cards must not overlap each other unless a lens explicitly opts into
// a deliberate clustering / stacking / overlap mode.
//
// Platform contract:
//
//   graphStage           — computes non-overlapping positions for
//                          stage-managed layouts (given accurate
//                          footprints) and exposes `validateNoOverlap`
//                          as a post-condition helper.
//   graphCytoscapeEngine — validates non-overlap for every engine-
//                          mounted graph against MEASURED runtime
//                          card dimensions; emits dedup'd diagnostics
//                          but does NOT silently mutate positions.
//   lenses               — must supply accurate or conservative
//                          footprints; must respect the engine-
//                          validated invariant.
//
// These tests pin the contract surface. Browser validation is the
// user-side runtime gate.

const (
	d37s2StageAsset    = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37s2EngineAsset   = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37s2ContextAsset  = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
)

// ── 1. graphStage exposes validateNoOverlap helper ────────────────

// TestExplorer_GraphStage_ValidateNoOverlapHelperExists pins that
// graphStage exports a `validateNoOverlap(stage) → {overlaps}` helper
// that detects bounding-box overlap between non-sentinel cards. This
// is the stage-layer post-condition surface.
func TestExplorer_GraphStage_ValidateNoOverlapHelperExists(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37s2StageAsset)

	if !strings.Contains(js, "function validateNoOverlap(stage)") {
		t.Errorf("D37s-2: graphStage must declare validateNoOverlap(stage) helper")
	}

	// The helper must compute card bounding boxes from stage.cards
	// entries (x, y, width, height) and check pairwise overlap.
	if !strings.Contains(js, "function _rectOfCardEntry(c)") {
		t.Errorf("D37s-2: graphStage must declare a rect-of-card-entry helper")
	}
	if !strings.Contains(js, "function _rectsOverlap(a, b)") {
		t.Errorf("D37s-2: graphStage must declare a pairwise rect-overlap helper")
	}

	// Sentinels are excluded from overlap detection.
	vIdx := strings.Index(js, "function validateNoOverlap(stage)")
	if vIdx < 0 {
		t.Fatal("D37s-2: validateNoOverlap must exist")
	}
	tail := js[vIdx:]
	endRel := strings.Index(tail[1:], "\n  function ")
	if endRel < 0 {
		t.Fatalf("D37s-2: validateNoOverlap body must be well-formed")
	}
	body := tail[:endRel+1]
	if !strings.Contains(body, "if (!c || c.isSentinel) continue;") {
		t.Errorf("D37s-2: validateNoOverlap must exclude sentinels from overlap detection")
	}

	// The helper must be exported on the public surface.
	if !strings.Contains(js, "validateNoOverlap:       validateNoOverlap,") {
		t.Errorf("D37s-2: graphStage public surface must export validateNoOverlap")
	}
}

// ── 2. graphStage diagnostic codes include overlap codes ──────────

// TestExplorer_GraphStage_DiagnosticCodesIncludeOverlapCodes pins
// the locked diagnostic codes for the non-overlap contract.
func TestExplorer_GraphStage_DiagnosticCodesIncludeOverlapCodes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37s2StageAsset)

	for _, code := range []string{
		`CARD_OVERLAP_DETECTED:         'card_overlap_detected',`,
		`CARD_OVERLAP_UNRESOLVED:       'card_overlap_unresolved',`,
		`CARD_OVERLAP_RESOLVED:         'card_overlap_resolved',`,
	} {
		if !strings.Contains(js, code) {
			t.Errorf("D37s-2: graphStage diagnostic codes must include %q", code)
		}
	}

	// validateNoOverlap emits the CARD_OVERLAP_DETECTED diagnostic.
	if !regexp.MustCompile(`_DIAGNOSTIC_CODES\.CARD_OVERLAP_DETECTED`).MatchString(js) {
		t.Errorf("D37s-2: validateNoOverlap must emit CARD_OVERLAP_DETECTED diagnostics")
	}
}

// ── 3. Context footprints are kind-aware ──────────────────────────

// TestExplorer_ContextFootprints_AreKindAware pins that
// `_buildCardFootprints` no longer returns the uniform 220×64
// footprint for every card. A kind-aware estimator must drive
// per-card heights.
func TestExplorer_ContextFootprints_AreKindAware(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37s2ContextAsset)

	// The kind-aware estimator function must exist.
	if !strings.Contains(js, "function _estimatedContextCardFootprint(card)") {
		t.Errorf("D37s-2: Context must declare _estimatedContextCardFootprint(card) kind-aware estimator")
	}

	// The estimator must branch on card.kind for at least the four
	// canonical kinds with distinct height treatments.
	for _, want := range []string{
		"case 'business_service':",
		"case 'related_business_service':",
		"case 'decision_surface':",
		"case 'ai_system':",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37s-2: _estimatedContextCardFootprint must handle %q", want)
		}
	}

	// _buildCardFootprints must consult the estimator (or measured
	// dims) rather than return uniform constants.
	bIdx := strings.Index(js, "function _buildCardFootprints(cards, stageConsts)")
	if bIdx < 0 {
		t.Fatal("D37s-2: _buildCardFootprints must exist")
	}
	bTail := js[bIdx:]
	bEndRel := strings.Index(bTail[1:], "\n  function ")
	if bEndRel < 0 {
		t.Fatalf("D37s-2: _buildCardFootprints body must be well-formed")
	}
	bBody := bTail[:bEndRel+1]

	if !strings.Contains(bBody, "_estimatedContextCardFootprint(c)") {
		t.Errorf("D37s-2: _buildCardFootprints must call _estimatedContextCardFootprint(c) for kind-aware first-paint sizing")
	}

	// Negative pin: the uniform-defaults shape MUST be gone from
	// _buildCardFootprints' body (it would produce identical 220×64
	// footprints for every card).
	if regexp.MustCompile(`out\[c\.id\]\s*=\s*\{\s*width:\s*defW,\s*height:\s*defH\s*\}`).MatchString(bBody) {
		t.Errorf("D37s-2: _buildCardFootprints must NOT assign uniform {width: defW, height: defH} footprints for every card — kind-aware estimator replaces this")
	}
}

// ── 4. Context footprints prefer measured over estimated ──────────

// TestExplorer_ContextFootprints_PreferMeasuredDimensions pins that
// _buildCardFootprints consults a per-card measurement cache and
// prefers measured dims over kind estimates when available.
func TestExplorer_ContextFootprints_PreferMeasuredDimensions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37s2ContextAsset)

	// The measurement cache must exist as a module-level state.
	if !strings.Contains(js, "var _measuredFootprints = {};") {
		t.Errorf("D37s-2: Context must maintain a per-card-id measurement cache `_measuredFootprints`")
	}

	// _buildCardFootprints consults the cache first.
	bIdx := strings.Index(js, "function _buildCardFootprints(cards, stageConsts)")
	if bIdx < 0 {
		t.Fatal("D37s-2: _buildCardFootprints must exist")
	}
	bTail := js[bIdx:]
	bEndRel := strings.Index(bTail[1:], "\n  function ")
	if bEndRel < 0 {
		t.Fatalf("D37s-2: _buildCardFootprints body must be well-formed")
	}
	bBody := bTail[:bEndRel+1]

	if !strings.Contains(bBody, "var measured = _measuredFootprints[id];") {
		t.Errorf("D37s-2: _buildCardFootprints must look up `_measuredFootprints[id]` for measurement preference")
	}
	if !regexp.MustCompile(`if \(measured && measured\.width > 0 && measured\.height > 0\)`).MatchString(bBody) {
		t.Errorf("D37s-2: _buildCardFootprints must prefer measured dimensions when both width and height are positive")
	}
}

// ── 5. Context reflow guard with threshold + coalescing ───────────

// TestExplorer_ContextFootprints_ReflowGuardPresent pins the
// measurement-driven reflow guard: a 4-px threshold + coalescing
// flag prevent measurement-layout thrash.
func TestExplorer_ContextFootprints_ReflowGuardPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37s2ContextAsset)

	// Threshold constant is declared.
	if !strings.Contains(js, "var FOOTPRINT_REFLOW_THRESHOLD = 4;") {
		t.Errorf("D37s-2: Context must declare `var FOOTPRINT_REFLOW_THRESHOLD = 4;` as the reflow-guard pixel delta")
	}

	// Coalescing flag is declared.
	if !strings.Contains(js, "var _reflowScheduled = false;") {
		t.Errorf("D37s-2: Context must declare `var _reflowScheduled = false;` as the reflow-coalescing flag")
	}

	// _storeMeasuredFootprint enforces the threshold check.
	if !strings.Contains(js, "function _storeMeasuredFootprint(id, w, h)") {
		t.Errorf("D37s-2: Context must declare _storeMeasuredFootprint(id, w, h)")
	}
	sIdx := strings.Index(js, "function _storeMeasuredFootprint(id, w, h)")
	if sIdx < 0 {
		t.Fatal("D37s-2: _storeMeasuredFootprint must exist")
	}
	sTail := js[sIdx:]
	sEndRel := strings.Index(sTail[1:], "\n  function ")
	if sEndRel < 0 {
		t.Fatalf("D37s-2: _storeMeasuredFootprint body must be well-formed")
	}
	sBody := sTail[:sEndRel+1]
	if !regexp.MustCompile(`deltaW >= FOOTPRINT_REFLOW_THRESHOLD \|\| deltaH >= FOOTPRINT_REFLOW_THRESHOLD`).MatchString(sBody) {
		t.Errorf("D37s-2: _storeMeasuredFootprint must threshold-check both width and height deltas against FOOTPRINT_REFLOW_THRESHOLD")
	}

	// _scheduleReflow exists and is the single scheduler entry point.
	if !strings.Contains(js, "function _scheduleReflow()") {
		t.Errorf("D37s-2: Context must declare _scheduleReflow() as the reflow scheduler")
	}
	rIdx := strings.Index(js, "function _scheduleReflow()")
	if rIdx < 0 {
		t.Fatal("D37s-2: _scheduleReflow must exist")
	}
	rTail := js[rIdx:]
	rEndRel := strings.Index(rTail[1:], "\n  function ")
	if rEndRel < 0 {
		t.Fatalf("D37s-2: _scheduleReflow body must be well-formed")
	}
	rBody := rTail[:rEndRel+1]
	if !strings.Contains(rBody, "if (_reflowScheduled) return;") {
		t.Errorf("D37s-2: _scheduleReflow must coalesce via `if (_reflowScheduled) return;` so multiple measurement events collapse to one reflow")
	}

	// REFLOW GUARD CONTRACT block is documented.
	if !strings.Contains(js, "REFLOW GUARD CONTRACT") {
		t.Errorf("D37s-2: Context must document the REFLOW GUARD CONTRACT at _storeMeasuredFootprint")
	}
}

// ── 6. Engine exposes onMeasurementsChange mount option ───────────

// TestExplorer_Engine_ExposesMeasurementsChangeCallback pins that
// engine.mount() accepts an optional `onMeasurementsChange` callback.
func TestExplorer_Engine_ExposesMeasurementsChangeCallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37s2EngineAsset)

	// The mount-options docblock documents `onMeasurementsChange`.
	if !strings.Contains(js, "onMeasurementsChange —") {
		t.Errorf("D37s-2: engine mount-options docblock must document `onMeasurementsChange` callback")
	}

	// _scheduleMeasurementChange wires the lens-supplied callback via
	// opts.onMeasurementsChange after the engine's measurement cache
	// updates.
	if !strings.Contains(js, "function _scheduleMeasurementChange()") {
		t.Errorf("D37s-2: engine must declare _scheduleMeasurementChange() to coalesce lens callbacks")
	}
	if !strings.Contains(js, "_isFn(opts.onMeasurementsChange)") {
		t.Errorf("D37s-2: engine must read opts.onMeasurementsChange before invoking it")
	}
	if !regexp.MustCompile(`opts\.onMeasurementsChange\(snapshot\)`).MatchString(js) {
		t.Errorf("D37s-2: engine must invoke opts.onMeasurementsChange(snapshot) with the coalesced measurement cache")
	}

	// The measurement cache feeds the lens callback.
	if !strings.Contains(js, "var _measurementCache       = {};") {
		t.Errorf("D37s-2: engine must maintain `_measurementCache` for the lens callback")
	}

	// Context lens passes the callback through engine.mount.
	contextJS := getExplorerAsset(t, srv, d37s2ContextAsset)
	if !regexp.MustCompile(`onMeasurementsChange:\s*function\s*\(measurements\)`).MatchString(contextJS) {
		t.Errorf("D37s-2: Context lens must pass `onMeasurementsChange: function (measurements)` to engine.mount")
	}
	if !strings.Contains(contextJS, "_storeMeasuredFootprint(k, m.width, m.height)") {
		t.Errorf("D37s-2: Context's onMeasurementsChange callback must call _storeMeasuredFootprint(k, m.width, m.height) per measurement entry")
	}
}

// ── 7. Engine validates non-overlap (no position mutation) ────────

// TestExplorer_Engine_ValidatesNonOverlapButDoesNotMutatePositions
// pins that the engine has a non-overlap validator that emits
// diagnostics but does NOT rewrite supplied node positions.
func TestExplorer_Engine_ValidatesNonOverlapButDoesNotMutatePositions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37s2EngineAsset)

	// _validateNoOverlap exists.
	if !strings.Contains(js, "function _validateNoOverlap(cy)") {
		t.Errorf("D37s-2: engine must declare _validateNoOverlap(cy) as the global non-overlap validator")
	}

	// The validator reads positions and data dimensions but does NOT
	// call n.position(...) with arguments (which would mutate
	// positions in cytoscape).
	vIdx := strings.Index(js, "function _validateNoOverlap(cy)")
	if vIdx < 0 {
		t.Fatal("D37s-2: _validateNoOverlap must exist")
	}
	vTail := js[vIdx:]
	vEndRel := strings.Index(vTail[1:], "\n  function ")
	if vEndRel < 0 {
		t.Fatalf("D37s-2: _validateNoOverlap body must be well-formed")
	}
	vBody := vTail[:vEndRel+1]
	if !strings.Contains(vBody, "n.position()") {
		t.Errorf("D37s-2: _validateNoOverlap must READ positions via n.position() — no-argument call (the mutating form is n.position({x, y}))")
	}
	// Negative pin: no setter-form position call inside the validator.
	if regexp.MustCompile(`n\.position\(\s*\{`).MatchString(vBody) {
		t.Errorf("D37s-2: _validateNoOverlap must NOT mutate positions — the engine validates, lenses comply; no n.position({x, y}) write allowed in the validator body")
	}

	// The PLATFORM CONTRACT comment block names the rule.
	if !strings.Contains(js, "PLATFORM CONTRACT") {
		t.Errorf("D37s-2: engine must document the PLATFORM CONTRACT for the non-overlap invariant")
	}
	for _, phrase := range []string{
		"graphStage computes",
		"graphCytoscapeEngine VALIDATES",
		"does NOT silently mutate supplied positions",
	} {
		if !strings.Contains(js, phrase) {
			t.Errorf("D37s-2: PLATFORM CONTRACT block must include %q", phrase)
		}
	}

	// _runOverlapValidation populates the diagnostics buffer.
	if !regexp.MustCompile(`_engineDiagnostics\.push\(\{[\s\S]*?code:\s*'card_overlap_detected'`).MatchString(js) {
		t.Errorf("D37s-2: _runOverlapValidation must push entries with code 'card_overlap_detected' onto _engineDiagnostics")
	}

	// handle.getDiagnostics() exposes the buffer.
	if !regexp.MustCompile(`getDiagnostics:\s*function\s*\(\)`).MatchString(js) {
		t.Errorf("D37s-2: engine handle must expose getDiagnostics()")
	}
	if !strings.Contains(js, "return _engineDiagnostics.slice();") {
		t.Errorf("D37s-2: handle.getDiagnostics() must return a shallow-cloned copy of _engineDiagnostics")
	}
}

// ── 8. Engine overlap diagnostics deduplicated ────────────────────

// TestExplorer_Engine_OverlapDiagnosticsDeduplicated pins that
// repeated overlap-pair diagnostics are deduplicated and that the
// validation is debounced.
func TestExplorer_Engine_OverlapDiagnosticsDeduplicated(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37s2EngineAsset)

	// Debounce constant declared.
	if !strings.Contains(js, "var OVERLAP_VALIDATION_DEBOUNCE_MS = 250;") {
		t.Errorf("D37s-2: engine must declare OVERLAP_VALIDATION_DEBOUNCE_MS = 250 as the validation debounce window")
	}

	// _scheduleOverlapValidation arms a timer; not re-armed while pending.
	if !strings.Contains(js, "function _scheduleOverlapValidation()") {
		t.Errorf("D37s-2: engine must declare _scheduleOverlapValidation()")
	}
	sIdx := strings.Index(js, "function _scheduleOverlapValidation()")
	if sIdx < 0 {
		t.Fatal("D37s-2: _scheduleOverlapValidation must exist")
	}
	sTail := js[sIdx:]
	sEndRel := strings.Index(sTail[1:], "\n    function ")
	if sEndRel < 0 {
		t.Fatalf("D37s-2: _scheduleOverlapValidation body must be well-formed")
	}
	sBody := sTail[:sEndRel+1]
	if !strings.Contains(sBody, "if (_overlapValidationTimer) return;") {
		t.Errorf("D37s-2: _scheduleOverlapValidation must early-return when a timer is already pending (debounce coalescing)")
	}

	// Per-pair seen-set declared.
	if !strings.Contains(js, "var _overlapSeen            = {};") {
		t.Errorf("D37s-2: engine must declare `_overlapSeen` per-pair seen-set for diagnostic dedup")
	}

	// _runOverlapValidation skips already-seen pairs.
	rIdx := strings.Index(js, "function _runOverlapValidation()")
	if rIdx < 0 {
		t.Fatal("D37s-2: _runOverlapValidation must exist")
	}
	rTail := js[rIdx:]
	rEndRel := strings.Index(rTail[1:], "\n    function ")
	if rEndRel < 0 {
		t.Fatalf("D37s-2: _runOverlapValidation body must be well-formed")
	}
	rBody := rTail[:rEndRel+1]
	if !strings.Contains(rBody, "if (_overlapSeen[pairKey]) continue;") {
		t.Errorf("D37s-2: _runOverlapValidation must skip already-seen overlap pairs to dedup diagnostics")
	}
	// Seen-set resets when validation finds zero overlaps (re-emergence re-fires).
	if !regexp.MustCompile(`if \(!result\.overlaps\.length\)[\s\S]*?_overlapSeen = \{\};`).MatchString(rBody) {
		t.Errorf("D37s-2: _runOverlapValidation must reset _overlapSeen when no overlaps remain so re-emergence after resolution re-fires the warning")
	}
}

// ── 9. Authority unchanged by this build ──────────────────────────

// TestExplorer_AuthorityUnchanged_NonOverlapBuild pins that Authority
// is structurally untouched by this build. Authority remains outside
// the engine (pending B''-Authority migration) and is whitelisted in
// the strategic-rule test. No source references to the new geometry
// callbacks appear in Authority modules.
func TestExplorer_AuthorityUnchanged_NonOverlapBuild(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	poc := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	edge := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js")

	// Authority PoC retains its load-bearing markers (D37r-tranche-B'' invariants).
	for _, want := range []string{
		"_cy = window.cytoscape({",
		"viewport.register('authority',",
		"_registerAuthorityCameraBusDelegate",
		"bus.registerLens('authority',",
		"_installHtmlCardOverlay",
		"_fitToAvailableCanvas",
	} {
		if !strings.Contains(poc, want) {
			t.Errorf("D37s-2: Authority PoC must remain unchanged — load-bearing marker %q missing", want)
		}
	}

	// Authority does NOT consume the new engine callbacks.
	for _, banned := range []string{
		"onMeasurementsChange",
		"_storeMeasuredFootprint",
		"_estimatedContextCardFootprint",
		"graphCytoscapeEngine.mount",
	} {
		if strings.Contains(poc, banned) {
			t.Errorf("D37s-2: Authority PoC must NOT reference the new geometry callback %q — Authority remains outside the engine in this build", banned)
		}
	}

	// Authority canvas-edge tabs module retains its provider hook.
	if !strings.Contains(edge, "registerLensProvider") {
		t.Errorf("D37s-2: Authority canvas-edge tabs module must remain unchanged (registerLensProvider)")
	}
}

// ── 10. Context surfaces preserved ────────────────────────────────

// TestExplorer_ContextSurfacesPreserved_NonOverlapBuild pins that
// Context's selected-object pane, evidence/drift tray, drawer, and
// selection-bridge contracts remain intact. None of this build's
// changes should rewire the lens's selection surfaces.
func TestExplorer_ContextSurfacesPreserved_NonOverlapBuild(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	contextJS := getExplorerAsset(t, srv, d37s2ContextAsset)

	// Selection bridge integration.
	if !strings.Contains(contextJS, "bridge.selectCard(card)") {
		t.Errorf("D37s-2: Context selection-bridge call site `bridge.selectCard(card)` must remain")
	}

	// Camera-bus delegate factory remains.
	if !strings.Contains(contextJS, "function _buildContextEngineCameraDelegate(handle)") {
		t.Errorf("D37s-2: Context's engine-camera-delegate factory must remain")
	}

	// Spatial mode gate retained.
	if !strings.Contains(contextJS, "if (_isSpatialMode() && _hasGraphStage()) {") {
		t.Errorf("D37s-2: Context spatial-mode gate must remain")
	}

	// Pane / tray / drawer providers remain present in source.
	for _, asset := range []string{
		"/explorer/assets/js/graph/context/context-selected-object-pane.js",
		"/explorer/assets/js/graph/context/context-evidence-tray.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			t.Errorf("D37s-2: Context surface asset %q must remain served", asset)
		}
	}
}

// ── 11. Native Context + non-spatial fallback preserved ───────────

// TestExplorer_NativeContextFallbackPreserved_NonOverlapBuild pins
// that the native Context default and the non-spatial strategic
// fallback are unchanged.
func TestExplorer_NativeContextFallbackPreserved_NonOverlapBuild(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Native Context default — GraphViewport adopts native-context.
	vp := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")
	if !strings.Contains(vp, "_baselineId = 'native-context';") {
		t.Errorf("D37s-2: GraphViewport must still adopt native-context as the baseline renderer")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37s-2: GraphViewport must still adoptExisting('native-context')")
	}

	// Non-spatial strategic Context fallback retained.
	contextJS := getExplorerAsset(t, srv, d37s2ContextAsset)
	if !strings.Contains(contextJS, "function _buildFallbackContextCameraDelegate()") {
		t.Errorf("D37s-2: non-spatial strategic Context fallback camera-bus delegate must remain")
	}
}
