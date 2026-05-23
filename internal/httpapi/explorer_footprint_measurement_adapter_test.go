package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	d37o12AdapterAsset = "/explorer/assets/js/graph/graph-platform/graph-footprint-measurement-adapter.js"
)

func readD37o12(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("explorer", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func d37o12AdapterSource(t *testing.T) string {
	t.Helper()
	return readD37o12(t, "assets/js/graph/graph-platform/graph-footprint-measurement-adapter.js")
}

func TestExplorer_FootprintMeasurementAdapter_LoadsAfterPolicyAndSink(t *testing.T) {
	html := readD37o12(t, "index.html")
	policyIdx := strings.Index(html, "/explorer/assets/js/graph/graph-platform/graph-footprint-policy.js")
	sinkIdx := strings.Index(html, "/explorer/assets/js/graph/graph-platform/graph-footprint-measurement-sink.js")
	adapterIdx := strings.Index(html, d37o12AdapterAsset)
	if policyIdx < 0 || sinkIdx < 0 || adapterIdx < 0 {
		t.Fatalf("D37o-overlap-12: policy, sink, and adapter scripts must all be loaded")
	}
	if !(policyIdx < sinkIdx && sinkIdx < adapterIdx) {
		t.Fatalf("D37o-overlap-12: adapter must load after policy and sink")
	}
}

func TestExplorer_FootprintMeasurementAdapter_PublicSurfaceAndMethods(t *testing.T) {
	js := d37o12AdapterSource(t)
	for _, want := range []string{
		"window.MIDASExplorerGraph.footprintMeasurementAdapter",
		"createAdapter: createAdapter",
		"function createAdapter(options)",
		"registerResolvedFootprint: registerResolvedFootprint",
		"recordOverlayMeasurement: recordOverlayMeasurement",
		"resetForGeneration: resetForGeneration",
		"destroy: destroy",
		"getSessionId: getSessionId",
		"getCardResult: getCardResult",
		"getGraphResult: getGraphResult",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-12: adapter public surface missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurementAdapter_DormantAndCreatesOneSinkSession(t *testing.T) {
	js := d37o12AdapterSource(t)
	if strings.Contains(js, "createAdapter({") {
		t.Fatalf("D37o-overlap-12: adapter must not create an adapter at module load")
	}
	if count := strings.Count(js, "factory({"); count != 1 {
		t.Fatalf("D37o-overlap-12: createAdapter should create exactly one sink session, found %d", count)
	}
	if !strings.Contains(js, "sinkFactory") || !strings.Contains(js, "footprintMeasurement") ||
		!strings.Contains(js, "createSession") {
		t.Fatalf("D37o-overlap-12: adapter must support injected sinkFactory and default sink session creation")
	}
}

func TestExplorer_FootprintMeasurementAdapter_RegisterResolvedFootprintForwardsAndCaches(t *testing.T) {
	js := d37o12AdapterSource(t)
	for _, want := range []string{
		"function registerResolvedFootprint(cardId, resolvedFootprint)",
		"adapter.footprints[id] = copy;",
		"_sinkMethod('registerResolvedFootprint')",
		"return fn ? !!fn(id, copy) : false;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-12: footprint registration contract missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurementAdapter_NormalisesOverlayFacts(t *testing.T) {
	js := d37o12AdapterSource(t)
	for _, want := range []string{
		"function recordOverlayMeasurement(key, measuredWidth, measuredHeight, measurementSource)",
		"function _normaliseMeasurement(cardId, w, h, source)",
		"cardId: cardId",
		"policyId: fp.policyId",
		"graphSurfaceId: adapter.graphSurfaceId",
		"rendererId: adapter.rendererId",
		"rendererMode: adapter.rendererMode",
		"cardKind: fp.cardKind",
		"cardVariant: fp.cardVariant == null ? null : fp.cardVariant",
		"reservedWidth: fp.reservedWidth",
		"reservedHeight: fp.reservedHeight",
		"measuredWidth: w",
		"measuredHeight: h",
		"measurementSource: _str(source) || DEFAULT_SOURCE",
		"sequenceNumber: adapter.sequenceNumber",
		"measurementAttempt: adapter.measurementAttempts[cardId]",
		"_sinkMethod('recordMeasurement')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-12: normalisation contract missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurementAdapter_UnregisteredCardDoesNotCallSinkRecord(t *testing.T) {
	js := d37o12AdapterSource(t)
	missingIdx := strings.Index(js, "if (!adapter.footprints[cardId]) return _makeFailure(adapter, cardId, CLASS_MISSING);")
	recordIdx := strings.Index(js, "var fn = _sinkMethod('recordMeasurement');")
	if missingIdx < 0 {
		t.Fatalf("D37o-overlap-12: unregistered card must return missing footprint")
	}
	if recordIdx < 0 || !(missingIdx < recordIdx) {
		t.Fatalf("D37o-overlap-12: unregistered card must not call sink recordMeasurement")
	}
	if !strings.Contains(js, "missing_resolved_footprint") {
		t.Fatalf("D37o-overlap-12: adapter must expose missing footprint compatible classification")
	}
}

func TestExplorer_FootprintMeasurementAdapter_SequenceAndAttemptCounters(t *testing.T) {
	js := d37o12AdapterSource(t)
	for _, want := range []string{
		"adapter.sequenceNumber += 1;",
		"adapter.measurementAttempts[cardId] = (adapter.measurementAttempts[cardId] || 0) + 1;",
		"adapter.sequenceNumber = 0;",
		"adapter.measurementAttempts = {};",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-12: sequence/attempt/reset contract missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurementAdapter_ResetDestroyAndDelegation(t *testing.T) {
	js := d37o12AdapterSource(t)
	for _, want := range []string{
		"function resetForGeneration(renderGeneration)",
		"adapter.renderGeneration = _num(renderGeneration, adapter.renderGeneration);",
		"_sinkMethod('resetForGeneration')",
		"function destroy()",
		"adapter.footprints = {};",
		"_sinkMethod('destroy')",
		"function getCardResult(cardId)",
		"_sinkMethod('getCardResult')",
		"function getGraphResult()",
		"_sinkMethod('getGraphResult')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-12: lifecycle/delegation contract missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurementAdapter_CallbackForwarding(t *testing.T) {
	js := d37o12AdapterSource(t)
	for _, want := range []string{
		"function _forwardDiagnostic(payload)",
		"adapter.onDiagnostic(_decoratePayload(adapter, payload));",
		"function _forwardRecompose(payload)",
		"adapter.onRecomposeRequested(_decoratePayload(adapter, payload));",
		"adapterSessionId: adapter.sessionId",
		"renderGeneration: adapter.renderGeneration",
		"onDiagnostic: _forwardDiagnostic",
		"onRecomposeRequested: _forwardRecompose",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-12: callback forwarding contract missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurementAdapter_NoForbiddenRuntimeAccess(t *testing.T) {
	js := d37o12AdapterSource(t)
	forbidden := []string{
		"document",
		"getBoundingClientRect",
		"ResizeObserver",
		"querySelector",
		"addEventListener",
		"requestAnimationFrame",
		"cytoscape",
		"renderedPosition",
		"boundingBox",
		"renderedBoundingBox",
		".cy",
		"cy.",
	}
	for _, bad := range forbidden {
		if strings.Contains(js, bad) {
			t.Errorf("D37o-overlap-12: adapter must not contain forbidden runtime access %q", bad)
		}
	}
	if strings.Count(js, "window.") != 4 {
		t.Fatalf("D37o-overlap-12: adapter may use window only for namespace/sink access; got %d uses", strings.Count(js, "window."))
	}
}

func TestExplorer_FootprintMeasurementAdapter_NotWiredToActiveConsumers(t *testing.T) {
	js := d37o12AdapterSource(t)
	for _, bad := range []string{
		"graph-cytoscape-overlay",
		"graph-cytoscape-engine",
		"graph-stage",
		"graph-geometry-sentinel",
		"context-cytoscape-renderer",
		"authority-cytoscape",
		"onMeasure",
	} {
		if strings.Contains(js, bad) {
			t.Errorf("D37o-overlap-12: adapter must not wire to active consumer marker %q", bad)
		}
	}

	consumers := map[string]string{
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js": "overlay",
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js":  "engine",
		"/explorer/assets/js/graph/graph-platform/graph-stage.js":             "stage",
		"/explorer/assets/js/graph/graph-platform/graph-geometry-sentinel.js": "sentinel",
	}
	for asset, label := range consumers {
		body := readD37o12(t, strings.TrimPrefix(asset, "/explorer/"))
		if strings.Contains(body, "footprintMeasurementAdapter") ||
			strings.Contains(body, "graph-footprint-measurement-adapter") {
			t.Errorf("D37o-overlap-12: %s must not consume dormant adapter", label)
		}
	}

	context := readD37o12(t, "assets/js/graph/context/context-cytoscape-renderer.js")
	if strings.Contains(context, "footprintMeasurementAdapter") {
		for _, want := range []string{
			"contextOverlay",
			"html-cards",
			"_isContextHtmlOverlayMode()",
			"_ensureContextOverlayAdapter(cards)",
		} {
			if !strings.Contains(context, want) {
				t.Errorf("D37o-overlap-14: Context adapter references must be gated by explicit overlay scaffold marker %q", want)
			}
		}
	}
}

func TestExplorer_FootprintMeasurementAdapter_PreservationSourcesUntouched(t *testing.T) {
	protected := []struct {
		name string
		body string
	}{
		{"graph-footprint-policy", readD37o12(t, "assets/js/graph/graph-platform/graph-footprint-policy.js")},
		{"graph-footprint-measurement-sink", readD37o12(t, "assets/js/graph/graph-platform/graph-footprint-measurement-sink.js")},
		{"authority-cytoscape-poc", readD37o12(t, "assets/js/graph/authority/authority-cytoscape-poc.js")},
		{"authority-cytoscape-toolbar", readD37o12(t, "assets/js/graph/authority/authority-cytoscape-toolbar.js")},
		{"authority-cytoscape-poc-css", readD37o12(t, "assets/css/authority-cytoscape-poc.css")},
		{"authority-graph-css", readD37o12(t, "assets/css/authority-graph.css")},
		{"authority-canvas-edge-tabs-css", readD37o12(t, "assets/css/authority-canvas-edge-tabs.css")},
	}
	for _, src := range protected {
		if strings.Contains(src.body, "footprintMeasurementAdapter") ||
			strings.Contains(src.body, "graph-footprint-measurement-adapter") {
			t.Errorf("D37o-overlap-12: %s must remain independent of dormant adapter", src.name)
		}
	}
	context := readD37o12(t, "assets/js/graph/context/context-cytoscape-renderer.js")
	if !strings.Contains(context, "overlayEnabled: false") {
		t.Fatalf("D37o-overlap-12: Context raw strategic path must keep overlayEnabled:false")
	}
	if strings.Contains(context, "footprintMeasurementAdapter") &&
		!(strings.Contains(context, "contextOverlay") && strings.Contains(context, "html-cards")) {
		t.Fatalf("D37o-overlap-14: Context adapter references must remain explicit-overlay scaffold only")
	}
}
