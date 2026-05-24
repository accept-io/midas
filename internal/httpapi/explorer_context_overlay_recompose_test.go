package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readD37o18(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("explorer", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func d37o18ContextRenderer(t *testing.T) string {
	t.Helper()
	return readD37o18(t, "assets/js/graph/context/context-cytoscape-renderer.js")
}

func TestExplorer_ContextOverlayRecompose_RawRouteRemainsIsolated(t *testing.T) {
	js := d37o18ContextRenderer(t)
	for _, want := range []string{
		"Protected raw route contract: overlayEnabled: false",
		"overlayEnabled: contextHtmlOverlayMode ? true : false",
		"onMeasurementsChange: contextHtmlOverlayMode ? function (measurements)",
		"if (!_isContextHtmlOverlayMode()) return;",
		"var overlayCandidate = _isContextHtmlOverlayMode() ? _contextOverlayFootprintCandidates[id] : null;",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-18: raw/overlay isolation marker missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayRecompose_StoresCandidatesByCardId(t *testing.T) {
	js := d37o18ContextRenderer(t)
	for _, want := range []string{
		"var candidates = request && request.updatedFootprintCandidates;",
		"for (var id in candidates)",
		"_contextOverlayFootprintCandidates[String(id)] = {",
		"reservedWidth: c.reservedWidth",
		"reservedHeight: c.reservedHeight",
		"source: c.source || 'measured-dom'",
		"policyId: c.policyId || ''",
		"cardKind: c.cardKind || null",
		"cardVariant: c.cardVariant == null ? null : c.cardVariant",
		"Context overlay footprint candidate stored.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-18: candidate storage marker missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayRecompose_IncrementsGenerationResetsAdapterAndSchedulesNormalPaint(t *testing.T) {
	js := d37o18ContextRenderer(t)
	for _, want := range []string{
		"_contextOverlayRenderGeneration += 1;",
		"_contextOverlayRecomposeRequested = true;",
		"Context overlay render generation incremented.",
		"_contextOverlayAdapter.resetForGeneration(_contextOverlayRenderGeneration)",
		"Context overlay adapter generation reset.",
		"Context overlay recompose render scheduled.",
		"_scheduleReflow();",
		"normal `_paintStrategicContext` path",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-18: recompose scheduling marker missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"graphStage.compose(layout, _contextOverlayFootprintCandidates",
		"footprintMeasurement.recordMeasurement",
		"graphGeometryDiag",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("D37o-overlap-18: recompose handler must not bypass platform ownership with %q", forbidden)
		}
	}
}

func TestExplorer_ContextOverlayRecompose_CandidatesAppliedBeforeGraphStageCompose(t *testing.T) {
	js := d37o18ContextRenderer(t)
	buildIdx := strings.Index(js, "function _buildCardFootprints(cards, stageConsts)")
	candidateIdx := strings.Index(js, "var overlayCandidate = _isContextHtmlOverlayMode() ? _contextOverlayFootprintCandidates[id] : null;")
	applyIdx := strings.Index(js, "Context overlay candidate applied to stage footprint.")
	composeIdx := strings.Index(js, "graphStage.compose(layout, footprints, safeArea, {})")
	if buildIdx < 0 || candidateIdx < buildIdx || applyIdx < candidateIdx || composeIdx < applyIdx {
		t.Fatalf("D37o-overlap-18: candidate footprint must be applied before graphStage.compose")
	}
	for _, want := range []string{
		"width: overlayCandidate.reservedWidth",
		"height: overlayCandidate.reservedHeight",
		"var footprints  = _buildCardFootprints(cards, stageConsts);",
		"try { stage = graphStage.compose(layout, footprints, safeArea, {}); }",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-18: candidate-to-stage marker missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayRecompose_StageOutputStillFeedsEngineNodeGeometry(t *testing.T) {
	js := d37o18ContextRenderer(t)
	for _, want := range []string{
		"var entry = stage.cards[c.id];",
		"var w = (typeof entry.width  === 'number' && entry.width  > 0) ? entry.width  : 220;",
		"var h = (typeof entry.height === 'number' && entry.height > 0) ? entry.height : 64;",
		"_engineHandle.refresh(_buildContextEngineData(cards, connectors, stage))",
		"data:   _buildContextEngineData(cards, connectors, stage)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-18: stage-to-engine geometry marker missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayRecompose_AdapterReceivesCandidateAdjustedFootprints(t *testing.T) {
	js := d37o18ContextRenderer(t)
	registerIdx := strings.Index(js, "_contextOverlayAdapter.registerResolvedFootprint(")
	resolveIdx := -1
	if registerIdx >= 0 {
		localResolveIdx := strings.Index(js[registerIdx:], "_contextOverlayResolvedFootprint(card)")
		if localResolveIdx >= 0 {
			resolveIdx = registerIdx + localResolveIdx
		}
	}
	candidateIdx := strings.Index(js, "var candidate = id ? _contextOverlayFootprintCandidates[id] : null;")
	if registerIdx < 0 || resolveIdx < registerIdx || candidateIdx < 0 {
		t.Fatalf("D37o-overlap-18: adapter footprint registration must use candidate-aware resolver")
	}
	for _, want := range []string{
		"reservedWidth: candidate.reservedWidth",
		"reservedHeight: candidate.reservedHeight",
		"source: 'context-overlay-candidate'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-18: candidate-aware resolved footprint marker missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayRecompose_PlatformModulesRemainUnchangedByContract(t *testing.T) {
	for _, rel := range []string{
		"assets/js/graph/graph-platform/graph-footprint-policy.js",
		"assets/js/graph/graph-platform/graph-footprint-measurement-sink.js",
		"assets/js/graph/graph-platform/graph-footprint-measurement-adapter.js",
		"assets/js/graph/graph-platform/graph-stage.js",
		"assets/js/graph/graph-platform/graph-cytoscape-engine.js",
		"assets/js/graph/graph-platform/graph-cytoscape-overlay.js",
	} {
		src := readD37o18(t, rel)
		for _, forbidden := range []string{
			"context-overlay-candidate",
			"Context overlay recompose render scheduled.",
			"__midasOverlayDiagnostics",
		} {
			if strings.Contains(src, forbidden) {
				t.Fatalf("D37o-overlap-18: %s must not consume Context overlay recompose wiring marker %q", rel, forbidden)
			}
		}
	}
}

func TestExplorer_ContextOverlayRecompose_AuthorityCssSentinelAndBrowserAutomationRemainUntouched(t *testing.T) {
	context := d37o18ContextRenderer(t)
	for _, forbidden := range []string{
		"authority-cytoscape",
		"authority/",
		"graphGeometryDiag.checkCurrent",
		"graphGeometryDiag.checkKnownSurfaces",
		"playwright",
		"puppeteer",
		"cypress",
		"selenium",
	} {
		if strings.Contains(strings.ToLower(context), strings.ToLower(forbidden)) {
			t.Fatalf("D37o-overlap-18: Context recompose wiring introduced forbidden marker %q", forbidden)
		}
	}
	// Source tests prove candidate-to-stage wiring only. Operator browser
	// validation is still required for actual DOM measurement, visible
	// overlap/no-overlap, recompose convergence, and sentinel pass/fail.
}
