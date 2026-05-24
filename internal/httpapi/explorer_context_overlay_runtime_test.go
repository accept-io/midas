package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readD37o16(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("explorer", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func d37o16ContextRenderer(t *testing.T) string {
	t.Helper()
	return readD37o16(t, "assets/js/graph/context/context-cytoscape-renderer.js")
}

func TestExplorer_ContextOverlayRuntime_OverlayEnabledOnlyForExplicitMode(t *testing.T) {
	js := d37o16ContextRenderer(t)
	for _, want := range []string{
		"var contextHtmlOverlayMode = _isContextHtmlOverlayMode();",
		"overlayEnabled: contextHtmlOverlayMode ? true : false",
		"Protected raw route contract: overlayEnabled: false",
		"`contextOverlay=html-cards`",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-16: conditional overlayEnabled contract missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayRuntime_MeasurementCallbackOnlyExplicitMode(t *testing.T) {
	js := d37o16ContextRenderer(t)
	for _, want := range []string{
		"onMeasurementsChange: contextHtmlOverlayMode ? function (measurements)",
		"_recordContextOverlayMeasurement(k, m.width, m.height)",
		"_contextOverlayAdapter.recordOverlayMeasurement(key, w, h, 'overlay-measure')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-16: explicit overlay measurement callback missing %q", want)
		}
	}
	if strings.Contains(js, "_recordContextOverlayMeasurement(k, m.width, m.height);\n          }") &&
		!strings.Contains(js, "} : null,") {
		t.Fatalf("D37o-overlap-16: measurement callback must be disabled for raw mode")
	}
}

func TestExplorer_ContextOverlayRuntime_DiagnosticsBufferGatedAndShaped(t *testing.T) {
	js := d37o16ContextRenderer(t)
	for _, want := range []string{
		"function _contextOverlayDiagnosticsEnabled()",
		"return _isContextHtmlOverlayMode() || _isUnsupportedContextOverlayMode();",
		"function _appendContextOverlayDiagnostic(source, message, payload, overrides)",
		"if (!_contextOverlayDiagnosticsEnabled()) return false;",
		"window.__midasOverlayDiagnostics",
		"timestamp:",
		"source:",
		"route:",
		"rendererId:",
		"graphSurfaceId: 'context'",
		"rendererMode:",
		"renderGeneration:",
		"cardId:",
		"cardKind:",
		"policyId:",
		"reservedWidth:",
		"reservedHeight:",
		"measuredWidth:",
		"measuredHeight:",
		"classification:",
		"action:",
		"recomposeAttempt:",
		"message:",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-16: diagnostics buffer contract missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayRuntime_DiagnosticEventsAreForwarded(t *testing.T) {
	js := d37o16ContextRenderer(t)
	for _, want := range []string{
		"Context overlay mode activated.",
		"Context overlay adapter session created.",
		"Context overlay adapter session destroyed.",
		"Context overlay adapter diagnostic.",
		"Context overlay adapter requested recompose.",
		"Context overlay recompose handler entered.",
		"Context overlay measurement recorded.",
		"Unsupported contextOverlay mode.",
		"classification: 'unsupported_context_overlay_mode'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37o-overlap-16: diagnostics forwarding missing %q", want)
		}
	}
}

func TestExplorer_ContextOverlayRuntime_InvalidOverlayDoesNotFallbackSilently(t *testing.T) {
	js := d37o16ContextRenderer(t)
	unsupportedIdx := strings.Index(js, "if (_isUnsupportedContextOverlayMode())")
	emptyIdx := strings.Index(js, "_paintEmptyState('Unsupported contextOverlay mode: ' + _readContextOverlayMode())")
	diagIdx := strings.Index(js, "classification: 'unsupported_context_overlay_mode'")
	if unsupportedIdx < 0 {
		t.Fatalf("D37o-overlap-16: unsupported contextOverlay guard missing")
	}
	returnIdx := strings.Index(js[unsupportedIdx:], "return;")
	if unsupportedIdx < 0 || emptyIdx < unsupportedIdx || diagIdx < unsupportedIdx || returnIdx < 0 {
		t.Fatalf("D37o-overlap-16: unsupported contextOverlay must diagnose and return before raw rendering")
	}
}

func TestExplorer_ContextOverlayRuntime_NoSentinelOrBrowserAutomationIntroduced(t *testing.T) {
	js := d37o16ContextRenderer(t)
	for _, forbidden := range []string{
		"geometry.checkCurrent",
		"geometry.checkKnownSurfaces",
		"graphGeometryDiag",
		"playwright",
		"puppeteer",
		"cypress",
		"selenium",
		"npm ",
		"npx ",
	} {
		if strings.Contains(strings.ToLower(js), strings.ToLower(forbidden)) {
			t.Fatalf("D37o-overlap-16: Context runtime activation introduced forbidden marker %q", forbidden)
		}
	}
}

func TestExplorer_ContextOverlayRuntime_PlatformModulesRemainUnwiredToDiagnosticsBuffer(t *testing.T) {
	for _, rel := range []string{
		"assets/js/graph/graph-platform/graph-footprint-policy.js",
		"assets/js/graph/graph-platform/graph-footprint-measurement-sink.js",
		"assets/js/graph/graph-platform/graph-footprint-measurement-adapter.js",
		"assets/js/graph/graph-platform/graph-cytoscape-engine.js",
		"assets/js/graph/graph-platform/graph-cytoscape-overlay.js",
		"assets/js/graph/graph-platform/graph-stage.js",
		"assets/js/graph/graph-platform/graph-geometry-sentinel.js",
	} {
		src := readD37o16(t, rel)
		if strings.Contains(src, "__midasOverlayDiagnostics") {
			t.Fatalf("D37o-overlap-16: diagnostics buffer must remain Context-owned, found in %s", rel)
		}
	}
}

func TestExplorer_ContextOverlayRuntime_AuthorityAndCssRemainUnreferenced(t *testing.T) {
	js := d37o16ContextRenderer(t)
	for _, forbidden := range []string{
		"authority-cytoscape",
		"authority-graph",
	} {
		if strings.Contains(strings.ToLower(js), forbidden) {
			t.Fatalf("D37o-overlap-16: Context runtime activation should not introduce %q", forbidden)
		}
	}
}

func TestExplorer_ContextOverlayRuntime_ManualBrowserValidationStillRequired(t *testing.T) {
	// Source tests prove only route gating, callback wiring, and diagnostics shape.
	// Operator browser validation is still required for visible HTML cards, actual
	// DOM measurements, overlap/no-overlap, recompose convergence, and sentinel
	// pass/fail.
	js := d37o16ContextRenderer(t)
	for _, forbidden := range []string{
		"overlap acceptance",
		"browser acceptance complete",
		"sentinel pass",
	} {
		if strings.Contains(strings.ToLower(js), forbidden) {
			t.Fatalf("D37o-overlap-16: source must not claim browser/runtime acceptance via %q", forbidden)
		}
	}
}
