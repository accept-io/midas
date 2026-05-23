package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37aa-graphstage-model-space-expansion — Platform model-space expansion +
// hard no-overlap invariant.
//
// The shared graph platform (graphStage) now expands its main strip width
// to fit the widest required band, mirroring the mechanism both reference
// legacy graphs use to avoid card overlap (legacy Context: mainStripW =
// max(floor, requiredRowW); legacy Authority: canvasW = max(MIN_CANVAS_W,
// maxX + NODE_W + EDGE_PAD)). Cytoscape `preset` renders the supplied
// positions verbatim and the engine's existing fit pipeline zooms out to
// frame the expanded stage (post-D37x label-independent bbox).
//
// Invariants pinned by this file:
//
//   • _computeRequiredMainStripWidth + _rowRequiredWidth exist as
//     platform helpers.
//   • _composeBanded uses them to compute mainStripW, then sets
//     stageWidth = padding.left + mainStripW + governance + padding.right.
//   • Split-column bands compute leftReqW / rightReqW per band and
//     allocate ASYMMETRIC column ranges so each column fits exactly
//     the required width; cards no longer overflow into the adjacent
//     column or governance space.
//   • compose() calls validateNoOverlap automatically and surfaces a
//     `stage.overlapReport` summary.
//   • DEFAULT_STAGE_MIN_WIDTH remains a floor, not a ceiling.
//   • D37u resize policy and D37x geometry/label contracts unchanged.
//   • Authority source untouched.

const (
	d37aaStageModule        = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37aaEngineModule       = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37aaNativeLabelsModule = "/explorer/assets/js/graph/graph-platform/graph-native-labels.js"
	d37aaAuthorityPoc       = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

// ── 1. Required-width helpers exist ───────────────────────────────────

func TestExplorer_D37aaModelSpace_RequiredWidthHelpersExist(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaStageModule)

	if !strings.Contains(js, "function _rowRequiredWidth(slots, cardFootprints, defaults)") {
		t.Errorf("D37aa-graphstage-model-space-expansion: graph-stage.js must define _rowRequiredWidth(slots, cardFootprints, defaults)")
	}
	if !strings.Contains(js, "function _computeRequiredMainStripWidth(layoutSpec, cardFootprints, defaults)") {
		t.Errorf("D37aa-graphstage-model-space-expansion: graph-stage.js must define _computeRequiredMainStripWidth(layoutSpec, cardFootprints, defaults)")
	}
}

// ── 2. Required-width math for non-split rows ─────────────────────────

func TestExplorer_D37aaModelSpace_NonSplitRowWidthShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaStageModule)

	// _rowRequiredWidth must sum footprint widths and add (n-1)*gapX.
	hIdx := strings.Index(js, "function _rowRequiredWidth(slots, cardFootprints, defaults)")
	if hIdx < 0 {
		t.Fatal("D37aa-graphstage-model-space-expansion: _rowRequiredWidth must exist")
	}
	hTail := js[hIdx:]
	hEnd := strings.Index(hTail[1:], "\n  function ")
	if hEnd < 0 {
		t.Fatalf("D37aa-graphstage-model-space-expansion: _rowRequiredWidth body must be well-formed")
	}
	hBody := hTail[:hEnd+1]

	if !strings.Contains(hBody, "totalW += w") {
		t.Errorf("D37aa-graphstage-model-space-expansion: _rowRequiredWidth must sum footprint widths")
	}
	if !strings.Contains(hBody, "if (n > 1) totalW += (n - 1) * defaults.gapX;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: _rowRequiredWidth must add (n-1) * gapX between cards")
	}
	if !strings.Contains(hBody, "if (seen[id]) continue;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: _rowRequiredWidth must deduplicate repeated cardIds")
	}
}

// ── 3. Required-width math for split-column bands ─────────────────────

func TestExplorer_D37aaModelSpace_SplitColumnBandRequiredWidth(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaStageModule)

	hIdx := strings.Index(js, "function _computeRequiredMainStripWidth(layoutSpec, cardFootprints, defaults)")
	if hIdx < 0 {
		t.Fatal("D37aa-graphstage-model-space-expansion: _computeRequiredMainStripWidth must exist")
	}
	hTail := js[hIdx:]
	hEnd := strings.Index(hTail[1:], "\n  function ")
	if hEnd < 0 {
		t.Fatalf("D37aa-graphstage-model-space-expansion: _computeRequiredMainStripWidth body must be well-formed")
	}
	hBody := hTail[:hEnd+1]

	// Split-column branch must compute both leftReq and rightReq, then
	// combine as leftReq + gapX + rightReq when both are non-empty.
	if !strings.Contains(hBody, "var leftReq  = _rowRequiredWidth(leftSlots,  cardFootprints, defaults);") {
		t.Errorf("D37aa-graphstage-model-space-expansion: split-column band must compute leftReq via _rowRequiredWidth")
	}
	if !strings.Contains(hBody, "var rightReq = _rowRequiredWidth(rightSlots, cardFootprints, defaults);") {
		t.Errorf("D37aa-graphstage-model-space-expansion: split-column band must compute rightReq via _rowRequiredWidth")
	}
	if !strings.Contains(hBody, "bandReq = leftReq + defaults.gapX + rightReq;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: split-column band with both sides non-empty must combine widths as leftReq + gapX + rightReq")
	}
	// Non-split branch path.
	if !strings.Contains(hBody, "bandReq = _rowRequiredWidth(flat, cardFootprints, defaults);") {
		t.Errorf("D37aa-graphstage-model-space-expansion: non-split band must compute bandReq from flat slot widths")
	}
	// Aggregation: take MAX across bands.
	if !strings.Contains(hBody, "if (bandReq > max) max = bandReq;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: aggregation must take max across bands")
	}
}

// ── 4. _composeBanded uses the expanded mainStripW ────────────────────

func TestExplorer_D37aaModelSpace_ComposeBandedUsesExpandedWidth(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaStageModule)

	cIdx := strings.Index(js, "function _composeBanded(stage, layoutSpec, cardFootprints, defaults, diagnostics)")
	if cIdx < 0 {
		t.Fatal("D37aa-graphstage-model-space-expansion: _composeBanded must exist")
	}
	cTail := js[cIdx:]
	cEnd := strings.Index(cTail[1:], "\n  function ")
	if cEnd < 0 {
		cEnd = len(cTail) - 1
	}
	cBody := cTail[:cEnd+1]

	// Required-width call.
	if !strings.Contains(cBody, "_computeRequiredMainStripWidth(") {
		t.Errorf("D37aa-graphstage-model-space-expansion: _composeBanded must call _computeRequiredMainStripWidth")
	}
	// Floor / expand pattern: paddedFloor + grow if requiredMainStripW exceeds it.
	if !strings.Contains(cBody, "var paddedFloor   = defaults.minWidth - padding.left - padding.right - govGap - govWidth;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: _composeBanded must compute paddedFloor from minWidth + chrome")
	}
	if !strings.Contains(cBody, "var mainStripW    = (requiredMainStripW > paddedFloor) ? requiredMainStripW : paddedFloor;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: mainStripW must be max(paddedFloor, requiredMainStripW)")
	}
	if !strings.Contains(cBody, "var stageWidth    = padding.left + mainStripW + govGap + govWidth + padding.right;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: stageWidth must be padding.left + mainStripW + govGap + govWidth + padding.right")
	}
	// Right edge of main strip uses the expanded width.
	if !strings.Contains(cBody, "var mainStripRight = mainStripLeft + mainStripW;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: mainStripRight must be mainStripLeft + mainStripW (expanded)")
	}
}

// ── 5. Split-column placement is asymmetric per band ──────────────────

func TestExplorer_D37aaModelSpace_SplitColumnPlacementIsAsymmetric(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaStageModule)

	// The split-column placement block in _composeBanded must compute
	// per-band leftReqW / rightReqW and allocate matching column
	// ranges leftX0..leftX1 and rightX0..rightX1 with a gapX gap.
	required := []string{
		"var leftReqW  = _rowRequiredWidth(leftSlots,  cardFootprints, defaults);",
		"var rightReqW = _rowRequiredWidth(rightSlots, cardFootprints, defaults);",
		"splitTotalW = leftReqW + splitGap + rightReqW;",
		"var leftX0  = mainStripLeft + splitSlack;",
		"var leftX1  = leftX0 + leftReqW;",
		"var rightX0 = (leftReqW > 0 && rightReqW > 0) ? (leftX1 + splitGap) : leftX0;",
		"var rightX1 = rightX0 + rightReqW;",
		"x0: leftX0, x1: leftX1,",
		"x0: rightX0, x1: rightX1,",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37aa-graphstage-model-space-expansion: split-column placement must include %q", r)
		}
	}

	// Legacy 50/50 split using mainStripMidX must be GONE from the
	// _placeRow ctx for split columns.
	bad := []string{
		"x0: mainStripLeft, x1: mainStripMidX - defaults.gapX / 2,",
		"x0: mainStripMidX + defaults.gapX / 2, x1: mainStripRight,",
	}
	for _, b := range bad {
		if strings.Contains(js, b) {
			t.Errorf("D37aa-graphstage-model-space-expansion: legacy 50/50 split-column allocation %q must be removed", b)
		}
	}
}

// ── 6. compose() runs validateNoOverlap automatically ─────────────────

func TestExplorer_D37aaModelSpace_ComposeRunsValidateNoOverlap(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaStageModule)

	// compose() body must call validateNoOverlap and write
	// stage.overlapReport.
	cIdx := strings.Index(js, "function compose(layoutSpec, cardFootprints, safeArea, opts)")
	if cIdx < 0 {
		t.Fatal("D37aa-graphstage-model-space-expansion: compose must exist")
	}
	cTail := js[cIdx:]
	cEnd := strings.Index(cTail[1:], "\n  function ")
	if cEnd < 0 {
		cEnd = strings.Index(cTail, "\n  // ──")
		if cEnd < 0 {
			cEnd = len(cTail) - 1
		}
	}
	cBody := cTail[:cEnd+1]

	if !regexp.MustCompile(`var report = validateNoOverlap\(stage\);`).MatchString(cBody) {
		t.Errorf("D37aa-graphstage-model-space-expansion: compose() must call validateNoOverlap(stage) as a hard post-condition")
	}
	if !strings.Contains(cBody, "stage.overlapReport = {") {
		t.Errorf("D37aa-graphstage-model-space-expansion: compose() must write stage.overlapReport for downstream consumers")
	}
	if !strings.Contains(cBody, "overlapCount: report.overlaps.length,") {
		t.Errorf("D37aa-graphstage-model-space-expansion: stage.overlapReport must include overlapCount")
	}
}

// ── 7. DEFAULT_STAGE_MIN_WIDTH remains a floor ────────────────────────

func TestExplorer_D37aaModelSpace_MinWidthIsAFloor(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaStageModule)

	// The pre-tranche assignment `var stageWidth = defaults.minWidth`
	// (used as both floor AND ceiling) MUST be gone from _composeBanded.
	cIdx := strings.Index(js, "function _composeBanded(stage, layoutSpec, cardFootprints, defaults, diagnostics)")
	if cIdx < 0 {
		t.Fatal("D37aa-graphstage-model-space-expansion: _composeBanded must exist")
	}
	cTail := js[cIdx:]
	cEnd := strings.Index(cTail[1:], "\n  function ")
	if cEnd < 0 {
		cEnd = len(cTail) - 1
	}
	cBody := cTail[:cEnd+1]

	if regexp.MustCompile(`var stageWidth = defaults\.minWidth;\s*$`).MatchString(cBody) {
		t.Errorf("D37aa-graphstage-model-space-expansion: _composeBanded must not seed stageWidth = defaults.minWidth as the only value (minWidth is a floor; stageWidth grows for content)")
	}
	// Constants block still declares DEFAULT_STAGE_MIN_WIDTH = 1180.
	if !strings.Contains(js, "var DEFAULT_STAGE_MIN_WIDTH  = 1180;") {
		t.Errorf("D37aa-graphstage-model-space-expansion: DEFAULT_STAGE_MIN_WIDTH must remain declared (used as the floor)")
	}
}

// ── 8. Cytoscape stays on `preset` (no library layout switch) ────────

func TestExplorer_D37aaModelSpace_CytoscapeStaysOnPreset(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaEngineModule)

	if !strings.Contains(js, "layout:               { name: 'preset', fit: false }") {
		t.Errorf("D37aa-graphstage-model-space-expansion: engine must keep layout: { name: 'preset', fit: false } (no Cytoscape layout switch)")
	}
}

// ── 9. D37x geometry / label contracts preserved ─────────────────────

func TestExplorer_D37aaModelSpace_D37xContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	engineJS := getExplorerAsset(t, srv, d37aaEngineModule)
	labelsJS := getExplorerAsset(t, srv, d37aaNativeLabelsModule)

	// D37x fit-bbox options block.
	required := []string{
		"var FIT_BBOX_OPTS = {",
		"includeLabels:       false",
		"includeOverlays:     false",
		"includeOutlines:     false",
		"eles.boundingBox(FIT_BBOX_OPTS)",
	}
	for _, r := range required {
		if !strings.Contains(engineJS, r) {
			t.Errorf("D37aa-graphstage-model-space-expansion: D37x fit-bbox contract regression — engine missing %q", r)
		}
	}
	// D37x base-style geometry contract — width/height bind to data.
	if !strings.Contains(engineJS, "'width':              'data(width)'") ||
		!strings.Contains(engineJS, "'height':             'data(height)'") {
		t.Errorf("D37aa-graphstage-model-space-expansion: D37x base-style geometry regression")
	}
	// D37x base-style label is blank.
	if !strings.Contains(engineJS, "'label':              ''") {
		t.Errorf("D37aa-graphstage-model-space-expansion: D37x base-style label regression")
	}
	// D37x native labels module still exports the helper.
	if !strings.Contains(labelsJS, "function makeNativeNodeLabel(text, width, height, options)") {
		t.Errorf("D37aa-graphstage-model-space-expansion: D37x native-labels helper regression")
	}
}

// ── 10. D37u passive resize policy preserved ─────────────────────────

func TestExplorer_D37aaModelSpace_D37uResizeContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37aaEngineModule)

	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37aa-graphstage-model-space-expansion: _onContainerResize must exist")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37aa-graphstage-model-space-expansion: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	if !strings.Contains(rBody, "cy.resize()") {
		t.Errorf("D37aa-graphstage-model-space-expansion: D37u passive resize must still call cy.resize()")
	}
	if !strings.Contains(rBody, "_emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)") {
		t.Errorf("D37aa-graphstage-model-space-expansion: D37u passive resize must still emit DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE")
	}
	forbidden := []string{
		"_runFitPipeline",
		"_fitToUsableRect",
		"_fitWithSafeArea",
		"cy.viewport(",
		"cy.fit(",
	}
	for _, f := range forbidden {
		if strings.Contains(rBody, f) {
			t.Errorf("D37aa-graphstage-model-space-expansion: D37u passive resize regression — %q must not appear", f)
		}
	}
}

// ── 11. Authority source untouched ───────────────────────────────────

func TestExplorer_D37aaModelSpace_AuthorityUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJS := getExplorerAsset(t, srv, d37aaAuthorityPoc)

	if !strings.Contains(authJS, "function _refitWithSafeArea()") {
		t.Errorf("D37aa-graphstage-model-space-expansion: Authority _refitWithSafeArea must remain")
	}
	if !strings.Contains(authJS, "ctx.onResize(_refitWithSafeArea)") {
		t.Errorf("D37aa-graphstage-model-space-expansion: Authority ctx.onResize subscription must remain")
	}
	if !strings.Contains(authJS, "function _computePresetPositions(projection, elements)") {
		t.Errorf("D37aa-graphstage-model-space-expansion: Authority _computePresetPositions must remain")
	}
}
