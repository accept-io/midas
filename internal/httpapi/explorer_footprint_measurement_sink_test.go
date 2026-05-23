package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

const (
	d37o10IndexAsset       = "/explorer"
	d37o10SinkAsset        = "/explorer/assets/js/graph/graph-platform/graph-footprint-measurement-sink.js"
	d37o10PolicyAsset      = "/explorer/assets/js/graph/graph-platform/graph-footprint-policy.js"
	d37o10ContextAsset     = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37o10AuthorityAsset   = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37o10AuthorityToolbar = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
)

func d37o10Asset(t *testing.T, path string) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	return getExplorerAsset(t, srv, path)
}

func TestExplorer_FootprintMeasurement_LoadsAfterFootprintPolicy(t *testing.T) {
	html := d37o10Asset(t, d37o10IndexAsset)
	policyIdx := strings.Index(html, "/explorer/assets/js/graph/graph-platform/graph-footprint-policy.js")
	sinkIdx := strings.Index(html, "/explorer/assets/js/graph/graph-platform/graph-footprint-measurement-sink.js")
	contextIdx := strings.Index(html, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")
	if policyIdx < 0 || sinkIdx < 0 || contextIdx < 0 {
		t.Fatalf("D37o-overlap-10: index.html must load policy, sink, and Context renderer scripts")
	}
	if !(policyIdx < sinkIdx && sinkIdx < contextIdx) {
		t.Fatalf("D37o-overlap-10: measurement sink must load after footprint policy and before Context renderer")
	}
}

func TestExplorer_FootprintMeasurement_PublicSurfaceAndMethods(t *testing.T) {
	js := d37o10Asset(t, d37o10SinkAsset)
	for _, want := range []string{
		"window.MIDASExplorerGraph.footprintMeasurement",
		"createSession: createSession",
		"function createSession(options)",
		"registerResolvedFootprint: registerResolvedFootprint",
		"recordMeasurement: recordMeasurement",
		"getCardResult: getCardResult",
		"getGraphResult: getGraphResult",
		"resetForGeneration: resetForGeneration",
		"destroy: destroy",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-10: measurement sink public surface missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurement_ClassificationStringsAndActions(t *testing.T) {
	js := d37o10Asset(t, d37o10SinkAsset)
	for _, want := range []string{
		"within_tolerance",
		"footprint_width_exceeded",
		"footprint_height_exceeded",
		"footprint_below_minimum",
		"fixed_footprint_css_violation",
		"measurement_unstable",
		"recompose_limit_exceeded",
		"missing_resolved_footprint",
		"invalid_measurement_payload",
		"accept",
		"warn",
		"request_recompose",
		"fail",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-10: sink must expose classification/action %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurement_RecordMeasurementDecisionRules(t *testing.T) {
	js := d37o10Asset(t, d37o10SinkAsset)
	for _, want := range []string{
		"function registerResolvedFootprint(cardId, resolvedFootprint)",
		"session.footprints[id] = _copy(resolvedFootprint)",
		"function recordMeasurement(measurementPayload)",
		"CLASSIFICATIONS.MISSING_FOOTPRINT",
		"CLASSIFICATIONS.INVALID_PAYLOAD",
		"CLASSIFICATIONS.BELOW_MINIMUM",
		"CLASSIFICATIONS.MEASUREMENT_UNSTABLE",
		"CLASSIFICATIONS.WIDTH_EXCEEDED",
		"CLASSIFICATIONS.HEIGHT_EXCEEDED",
		"CLASSIFICATIONS.FIXED_POLICY_VIOLATION",
		"CLASSIFICATIONS.WITHIN_TOLERANCE",
		"result.mismatchDimension = 'none'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-10: recordMeasurement decision rule missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurement_BoundedRecomposeStateMachine(t *testing.T) {
	js := d37o10Asset(t, d37o10SinkAsset)
	for _, want := range []string{
		"var DEFAULT_MAX_RECOMPOSE_ATTEMPTS = 2;",
		"session.recomposeAttempt >= session.maxRecomposeAttempts",
		"session.recomposeAttempt += 1;",
		"CLASSIFICATIONS.RECOMPOSE_LIMIT_EXCEEDED",
		"updatedFootprintCandidate",
		"Math.ceil(Math.max(result.reservedWidth, result.measuredWidth",
		"Math.ceil(Math.max(result.reservedHeight, result.measuredHeight",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-10: bounded recompose marker missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurement_ResetDestroyAndAggregateResults(t *testing.T) {
	js := d37o10Asset(t, d37o10SinkAsset)
	for _, want := range []string{
		"function resetForGeneration(renderGeneration)",
		"session.recomposeAttempt = 0",
		"session.destroyed = false",
		"function destroy()",
		"session.destroyed = true",
		"function getGraphResult()",
		"classification = 'not_measured'",
		"classification = 'fail'",
		"classification = ACTIONS.REQUEST_RECOMPOSE",
		"classification = 'warn'",
		"classification = 'pass'",
		"resultsByCardId: results",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-10: reset/destroy/aggregate marker missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurement_DiagnosticsAndCallbacks(t *testing.T) {
	js := d37o10Asset(t, d37o10SinkAsset)
	for _, want := range []string{
		"onDiagnostic",
		"onRecomposeRequested",
		"severity: severity",
		"sessionId: session.sessionId",
		"result: _copy(result)",
		"updatedFootprintCandidates",
		"source: 'measured-dom'",
		"session.onDiagnostic({",
		"try { session.onRecomposeRequested(payload); } catch",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-10: diagnostic/callback marker missing %q", want)
		}
	}
}

func TestExplorer_FootprintMeasurement_NoForbiddenRuntimeAccess(t *testing.T) {
	js := d37o10Asset(t, d37o10SinkAsset)
	for _, banned := range []string{
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
		"graphStage",
		"setTimeout",
		"setInterval",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-overlap-10: measurement sink must not contain forbidden runtime access %q", banned)
		}
	}
	windowUses := regexp.MustCompile(`window\.`).FindAllStringIndex(js, -1)
	if len(windowUses) != 3 {
		t.Fatalf("D37o-overlap-10: only expected window uses are namespace attachment guards, got %d", len(windowUses))
	}
	if !strings.Contains(js, "window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};") ||
		!strings.Contains(js, "window.MIDASExplorerGraph.footprintMeasurement = {") {
		t.Errorf("D37o-overlap-10: permitted window use must be MIDASExplorerGraph namespace attachment only")
	}
}

func TestExplorer_FootprintMeasurement_NotWiredToActiveConsumers(t *testing.T) {
	for path, label := range map[string]string{
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js": "overlay",
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js":  "engine",
		"/explorer/assets/js/graph/graph-platform/graph-stage.js":             "stage",
		"/explorer/assets/js/graph/graph-platform/graph-geometry-sentinel.js": "sentinel",
		"/explorer/assets/js/graph/context/context-cytoscape-renderer.js":     "context",
	} {
		src := d37o10Asset(t, path)
		if strings.Contains(src, "footprintMeasurement") ||
			strings.Contains(src, "graph-footprint-measurement-sink") {
			t.Errorf("D37o-overlap-10: sink must not be wired to %s in this tranche", label)
		}
	}
}

func TestExplorer_FootprintMeasurement_PreservationSourcesUntouched(t *testing.T) {
	policy := d37o10Asset(t, d37o10PolicyAsset)
	context := d37o10Asset(t, d37o10ContextAsset)
	authority := d37o10Asset(t, d37o10AuthorityAsset)
	toolbar := d37o10Asset(t, d37o10AuthorityToolbar)
	css := getExplorerAllCSS(t, NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).WithExplorerEnabled(true))

	for _, src := range []struct {
		name string
		body string
	}{
		{"graph-footprint-policy", policy},
		{"context renderer", context},
		{"authority renderer", authority},
		{"authority toolbar", toolbar},
		{"css", css},
	} {
		if strings.Contains(src.body, "footprintMeasurement") ||
			strings.Contains(src.body, "graph-footprint-measurement-sink") {
			t.Errorf("D37o-overlap-10: %s must remain unwired to measurement sink", src.name)
		}
	}
	if !strings.Contains(context, "overlayEnabled: false") {
		t.Errorf("D37o-overlap-10: Context raw strategic path must keep overlayEnabled false")
	}
}
