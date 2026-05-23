package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readD37o14(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("explorer", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func d37o14ContextRenderer(t *testing.T) string {
	t.Helper()
	return readD37o14(t, "assets/js/graph/context/context-cytoscape-renderer.js")
}

func TestExplorer_ContextOverlayMode_ParserExistsAndRecognisesHtmlCards(t *testing.T) {
	js := d37o14ContextRenderer(t)
	for _, want := range []string{
		"var OVERLAY_QUERY_PARAM = 'contextOverlay';",
		"var OVERLAY_MODE_HTML_CARDS = 'html-cards';",
		"function _readContextOverlayMode()",
		"function _isContextHtmlOverlayMode()",
		"return _readContextOverlayMode() === OVERLAY_MODE_HTML_CARDS;",
		"function _isUnsupportedContextOverlayMode()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-14: overlay parser contract missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayMode_RawSpatialRoutePreserved(t *testing.T) {
	js := d37o14ContextRenderer(t)
	for _, want := range []string{
		"var LAYOUT_QUERY_PARAM = 'contextLayout';",
		"var LAYOUT_MODE_SPATIAL = 'spatial';",
		"function _isSpatialMode()",
		"overlayEnabled: false",
		"raw Cytoscape mode",
		"Absence of `contextOverlay`",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-14: protected raw route contract missing %q", want)
		}
	}
	if strings.Index(js, "overlayEnabled: false") < 0 ||
		strings.Index(js, "_ensureContextOverlayAdapter(cards)") < 0 {
		t.Fatalf("D37o-overlap-14: expected raw overlay flag and adapter scaffold markers")
	}
}

func TestExplorer_ContextOverlayMode_ExplicitModeSeparateFromSpatialLayout(t *testing.T) {
	js := d37o14ContextRenderer(t)
	if !strings.Contains(js, "contextOverlay") || !strings.Contains(js, "contextLayout") {
		t.Fatalf("D37o-overlap-14: overlay mode and spatial layout query params must both remain present")
	}
	if strings.Contains(js, "var LAYOUT_MODE_SPATIAL = 'html-overlay'") ||
		strings.Contains(js, "var MODE_STRATEGIC = 'strategic-overlay'") {
		t.Fatalf("D37o-overlap-14: overlay scaffold must not create new layout or renderer identities")
	}
}

func TestExplorer_ContextOverlayMode_InvalidModeHasDiagnosticNotSilentFallback(t *testing.T) {
	js := d37o14ContextRenderer(t)
	for _, want := range []string{
		"if (_isUnsupportedContextOverlayMode())",
		"Unsupported contextOverlay mode:",
		"unsupported_context_overlay_mode",
		"_destroyContextOverlayAdapter();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-14: unsupported overlay mode handling missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayMode_AdapterSessionOnlyExplicitOverlayPath(t *testing.T) {
	js := d37o14ContextRenderer(t)
	for _, want := range []string{
		"function _ensureContextOverlayAdapter(cards)",
		"if (!_isContextHtmlOverlayMode())",
		"_destroyContextOverlayAdapter();",
		"factory.createAdapter({",
		"rendererMode: 'html-overlay-scaffold'",
		"onDiagnostic: _handleContextOverlayDiagnostic",
		"onRecomposeRequested: _handleContextOverlayRecomposeRequested",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-14: adapter lifecycle scaffold missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayMode_RegistersFootprintsBeforeEngineMount(t *testing.T) {
	js := d37o14ContextRenderer(t)
	registerIdx := strings.Index(js, "_ensureContextOverlayAdapter(cards);")
	renderIdx := strings.Index(js, "_renderSpatialCytoscape(layout, cards, connectors, stage, byId, painter);")
	mountIdx := strings.Index(js, "_engineHandle = engine.mount(canvas, {")
	if registerIdx < 0 || renderIdx < 0 || mountIdx < 0 {
		t.Fatalf("D37o-overlap-14: expected adapter registration, render call, and engine mount markers")
	}
	if !(registerIdx < renderIdx && renderIdx < mountIdx) {
		t.Fatalf("D37o-overlap-14: adapter footprint registration scaffold must appear before engine mount")
	}
	for _, want := range []string{
		"_contextOverlayAdapter.registerResolvedFootprint(",
		"String(card.id)",
		"_contextOverlayResolvedFootprint(card)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-14: footprint registration plumbing missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayMode_MeasurementCallbackShapeDormant(t *testing.T) {
	js := d37o14ContextRenderer(t)
	for _, want := range []string{
		"function _recordContextOverlayMeasurement(key, w, h)",
		"_contextOverlayAdapter.recordOverlayMeasurement(key, w, h, 'overlay-measure')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-14: measurement callback scaffold missing %q", want)
		}
	}
	if strings.Contains(js, "onMeasure: _recordContextOverlayMeasurement") {
		t.Fatalf("D37o-overlap-14: measurement callback must remain dormant, not wired to overlay")
	}
}

func TestExplorer_ContextOverlayMode_RecomposeHandlerOwnedByContext(t *testing.T) {
	js := d37o14ContextRenderer(t)
	for _, want := range []string{
		"function _handleContextOverlayRecomposeRequested(request)",
		"_contextOverlayFootprintCandidates[String(id)]",
		"_contextOverlayRenderGeneration += 1;",
		"_contextOverlayRecomposeRequested = true;",
		"_contextOverlayAdapter.resetForGeneration(_contextOverlayRenderGeneration)",
		"intentionally not wired in this tranche",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-14: Context-owned recompose scaffold missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"footprintMeasurement.recordMeasurement",
		"graphStage.compose(layout, _contextOverlayFootprintCandidates",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D37o-overlap-14: recompose scaffold must not bypass owner boundaries via %q", forbidden)
		}
	}
}

func TestExplorer_ContextOverlayMode_ResolverPureAndCandidatesLocal(t *testing.T) {
	context := d37o14ContextRenderer(t)
	policy := readD37o14(t, "assets/js/graph/graph-platform/graph-footprint-policy.js")
	if strings.Contains(policy, "_contextOverlayFootprintCandidates") ||
		strings.Contains(policy, "html-overlay-scaffold") {
		t.Fatalf("D37o-overlap-14: resolver must remain pure and unaware of Context overlay candidates")
	}
	for _, want := range []string{
		"var _contextOverlayFootprintCandidates = {};",
		"_contextOverlayFootprintCandidates[id]",
		"source: 'context-overlay-candidate'",
	} {
		if !strings.Contains(context, want) {
			t.Errorf("D37o-overlap-14: local candidate flow missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayMode_NoAuthorityCssBackendOrBrowserTooling(t *testing.T) {
	context := d37o14ContextRenderer(t)
	for _, forbidden := range []string{
		"authority-cytoscape",
		"authority/",
		"Playwright",
		"Cypress",
		"Puppeteer",
		"Selenium",
		"graphGeometryDiag pass",
		"sentinel pass",
	} {
		if strings.Contains(context, forbidden) {
			t.Errorf("D37o-overlap-14: Context overlay scaffold must not introduce forbidden marker %q", forbidden)
		}
	}
	for _, rel := range []string{
		"assets/js/graph/authority/authority-cytoscape-poc.js",
		"assets/js/graph/authority/authority-cytoscape-toolbar.js",
		"assets/css/authority-cytoscape-poc.css",
		"assets/css/authority-graph.css",
		"assets/css/authority-canvas-edge-tabs.css",
	} {
		body := readD37o14(t, rel)
		if strings.Contains(body, "contextOverlay") || strings.Contains(body, "html-overlay-scaffold") {
			t.Fatalf("D37o-overlap-14: %s must not reference Context overlay scaffold", rel)
		}
	}
}

func TestExplorer_ContextOverlayMode_SourceTestsDoNotClaimBrowserAcceptance(t *testing.T) {
	body := readD37o14(t, "../explorer_context_overlay_mode_test.go")
	for _, want := range []string{
		"measurement callback must remain dormant",
		"SourceTestsDoNotClaimBrowserAcceptance",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-overlap-14: test file must document source-only scaffold boundary %q", want)
		}
	}
	for _, forbidden := range []string{
		"browser acceptance " + "passed",
		"overlap is " + "fixed",
		"sentinel acceptance " + "passed",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D37o-overlap-14: source tests must not claim browser/runtime acceptance via %q", forbidden)
		}
	}
}
