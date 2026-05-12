package httpapi

// explorer_drift_workbench_test.go — Drift-2c Level 2 Drift Workbench pins.
//
// All Drift-2c assertions live here; the Drift-2b file
// (explorer_drift_test.go) was touched once to update the URL fragment
// in TestExplorer_DriftJS_UsesDrift1dReadEndpointsOnly so the existing
// pin survives the new shared DRIFT_POINT_FETCH_LIMIT constant.
//
// Negative pins enforce:
//   - no Level 3 inspector markup;
//   - no observation-triage action labels (Accept / Suppress / Mark as
//     known business change / Resolve / Triage / Acknowledge);
//   - no threshold band/line rendering;
//   - no combined status/backfill class such as
//     status-warning-backfilled or drift-status-backfilled;
//   - no generic .status-unknown / .drift-status-unknown class;
//   - no V2 drift type names;
//   - no mutating HTTP verbs against /v1/drift;
//   - no /v1/controlplane/drift_definitions calls;
//   - no external chart library reference;
//   - the literal 100 is not scattered across multiple Drift JS
//     call-sites (occurrences must come from the single shared
//     constant declaration).

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func driftWorkbenchHTML(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	return rec.Body.String()
}

func driftWorkbenchJS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	return getExplorerAsset(t, srv, "/explorer/assets/js/drift-workbench.js")
}

func driftWorkbenchCSS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	// drift.css holds both Drift-2b heatmap and Drift-2c workbench
	// styles; the Drift-2c test reaches into the same file.
	return getExplorerAsset(t, srv, "/explorer/assets/css/drift.css")
}

// ---------------------------------------------------------------------------
// Markup pins — workbench container, header, panels, synthetic-data copy,
// no Level 3 inspector, no triage action labels.
// ---------------------------------------------------------------------------

func TestExplorer_HTML_DriftWorkbench_ContainerExists(t *testing.T) {
	body := driftWorkbenchHTML(t)
	for _, want := range []string{
		`id="services-drift-workbench"`,
		`class="drift-workbench"`,
		`data-drift-workbench-header`,
		`data-drift-workbench-series-rail`,
		`data-drift-workbench-chart`,
		`data-drift-workbench-detail`,
		`data-drift-workbench-observations`,
		`data-drift-workbench-annotations`,
		`data-drift-workbench-state`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("explorer HTML missing Drift-2c marker %q", want)
		}
	}
}

func TestExplorer_HTML_DriftWorkbench_ScriptLinked(t *testing.T) {
	body := driftWorkbenchHTML(t)
	if !strings.Contains(body, `<script src="/explorer/assets/js/drift-workbench.js"></script>`) {
		t.Error("explorer HTML must link drift-workbench.js")
	}
	heatmapIdx := strings.Index(body, `<script src="/explorer/assets/js/drift-heatmap.js"></script>`)
	workbenchIdx := strings.Index(body, `<script src="/explorer/assets/js/drift-workbench.js"></script>`)
	if heatmapIdx < 0 || workbenchIdx < 0 {
		t.Fatal("expected both heatmap and workbench script tags")
	}
	if workbenchIdx <= heatmapIdx {
		t.Errorf("drift-workbench.js must load AFTER drift-heatmap.js (heatmap at %d, workbench at %d)",
			heatmapIdx, workbenchIdx)
	}
}

// Drift-2d note: TestExplorer_HTML_DriftWorkbench_NoLevel3InspectorMarkup
// (which previously asserted absence of `drift-observation-inspector`
// markers in this file) was deleted when Drift-2d added the inspector.
// Replacement assertions live in
// internal/httpapi/explorer_drift_observation_inspector_test.go:
//   - TestExplorer_HTML_DriftObservationInspector_MarkupExists
//     (positive markup-exists pin — selector set from the inspector
//     CSS list)
//   - TestExplorer_HTML_DriftObservationInspector_ReadOnly_NoForbiddenLabels
//     (positive read-only pin — every forbidden triage / annotation
//     label asserted absent inside the inspector container's source
//     range only, via extractDriftInspectorSource)

func TestExplorer_HTML_DriftWorkbench_NoTriageActionLabels(t *testing.T) {
	body := driftWorkbenchHTML(t)
	js := driftWorkbenchJS(t)
	// Drift-2d — narrow the body search to exclude the inspector
	// container's source. The inspector itself is independently
	// pinned to be free of these labels by
	// TestExplorer_HTML_DriftObservationInspector_ReadOnly_NoForbiddenLabels;
	// scoping here ensures the workbench surface outside the
	// inspector remains free of triage labels even while the
	// inspector adds its own read-only markup.
	bodyOutsideInspector := stripInspectorSource(body)

	// Forbidden UI / action labels — these belong to operator
	// workflow surfaces that do not exist in this Explorer. Use
	// case-sensitive matches with surrounding context to avoid false
	// positives on incidental occurrences.
	for _, forbidden := range []string{
		`>Accept<`,
		`>Suppress<`,
		`>Mark as known business change<`,
		`>Resolve<`,
		`>Triage<`,
		`>Acknowledge<`,
		`aria-label="Accept`,
		`aria-label="Suppress`,
		`aria-label="Mark as known business change`,
		`aria-label="Resolve`,
		`aria-label="Triage`,
		`aria-label="Acknowledge`,
	} {
		if strings.Contains(bodyOutsideInspector, forbidden) {
			t.Errorf("workbench surface (outside inspector) must not include triage action label %q in HTML", forbidden)
		}
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must not include triage action label %q", forbidden)
		}
	}
}

// stripInspectorSource removes the Drift-2d inspector container's
// source range from the explorer body so the workbench-scope no-
// triage pin can ignore the inspector's contents. The inspector is a
// pre-rendered <section id="drift-observation-inspector"> with a
// matching </section> close; this helper computes the substring by
// finding the open tag and walking forward to the matching close.
func stripInspectorSource(body string) string {
	const openMarker = `<section id="drift-observation-inspector"`
	const closeMarker = `</section>`
	openIdx := strings.Index(body, openMarker)
	if openIdx < 0 {
		return body
	}
	// Find the matching </section> by counting opens from the marker
	// onward. The inspector contains no nested <section> in HTML
	// markup (it has <header> children and JS-rendered content); the
	// first </section> after openIdx is the close.
	closeIdx := strings.Index(body[openIdx:], closeMarker)
	if closeIdx < 0 {
		return body
	}
	end := openIdx + closeIdx + len(closeMarker)
	return body[:openIdx] + body[end:]
}

func TestExplorer_DriftWorkbench_SyntheticBadgeRenderedFromJS(t *testing.T) {
	js := driftWorkbenchJS(t)
	if !strings.Contains(js, "Synthetic demo signal. Not calculated from runtime aggregation.") {
		t.Error("drift-workbench.js must render the synthetic-data badge copy in the workbench header")
	}
}

// ---------------------------------------------------------------------------
// Interaction / source pins — heatmap-cell activation opens the workbench;
// keyboard activation uses Enter/Space; selected-cell class is applied.
// ---------------------------------------------------------------------------

func TestExplorer_DriftJS_HeatmapCellActivationOpensWorkbench(t *testing.T) {
	js := driftHeatmapJS(t)
	for _, want := range []string{
		`MIDASExplorerDriftWorkbench.openDriftWorkbench`,
		`is-selected`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-heatmap.js must wire workbench-open / selected-cell %q", want)
		}
	}
}

func TestExplorer_DriftJS_HeatmapCellKeyboardActivation(t *testing.T) {
	js := driftHeatmapJS(t)
	// Keyboard activation must accept Enter and Space (browsers
	// produce both ' ' and 'Spacebar' for Space depending on key
	// event source — we accept either).
	for _, want := range []string{
		`'keydown'`,
		`e.key === 'Enter'`,
		`e.key === ' '`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-heatmap.js must handle keyboard activation token %q", want)
		}
	}
	if !strings.Contains(js, `e.preventDefault()`) {
		t.Error("drift-heatmap.js must preventDefault on Enter/Space activation to suppress page scroll")
	}
}

func TestExplorer_DriftCSS_SelectedCellClassExists(t *testing.T) {
	css := driftWorkbenchCSS(t)
	if !strings.Contains(css, `.drift-heatmap-cell.is-selected`) {
		t.Error("drift.css must define .drift-heatmap-cell.is-selected for the persistent selected state")
	}
}

func TestExplorer_DriftWorkbenchJS_NoMutatingHTTPVerbs(t *testing.T) {
	js := driftWorkbenchJS(t)
	for _, forbidden := range []string{
		`method: 'POST'`, `method: "POST"`,
		`method: 'PUT'`, `method: "PUT"`,
		`method: 'PATCH'`, `method: "PATCH"`,
		`method: 'DELETE'`, `method: "DELETE"`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must not call mutating HTTP verb %q", forbidden)
		}
	}
}

func TestExplorer_DriftWorkbenchJS_NoControlPlaneLifecycleCalls(t *testing.T) {
	js := driftWorkbenchJS(t)
	if strings.Contains(js, "/v1/controlplane/drift_definitions") {
		t.Error("drift-workbench.js must not call /v1/controlplane/drift_definitions (lifecycle endpoints are out of scope)")
	}
}

// ---------------------------------------------------------------------------
// Data-loading pins — endpoints used, shared point limit constant, no
// scattered literal 100, latest-point selection by window_end, brief TODOs.
// ---------------------------------------------------------------------------

func TestExplorer_DriftJS_SharedPointLimitConstantDefined(t *testing.T) {
	js := driftHeatmapJS(t)
	for _, want := range []string{
		`const DRIFT_POINT_FETCH_LIMIT = 100`,
		// Brief-required TODO next to the constant.
		`TODO: replace client-side point loading with backend latest/range/sort`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-heatmap.js must define shared point-limit constant token %q", want)
		}
	}
	// The constant must be exposed on the shared namespace so
	// drift-workbench.js can read it.
	if !strings.Contains(js, `DRIFT_POINT_FETCH_LIMIT: DRIFT_POINT_FETCH_LIMIT`) {
		t.Error("drift-heatmap.js must export DRIFT_POINT_FETCH_LIMIT on window.MIDASExplorerDrift")
	}
}

func TestExplorer_DriftJS_NoScatteredLiteral100(t *testing.T) {
	// The literal 100 must appear at most twice in drift-heatmap.js
	// — exactly once in the constant declaration and once in any
	// adjacent comment that documents the value. Anything more than
	// two occurrences indicates the literal has been scattered.
	js := driftHeatmapJS(t)
	count := strings.Count(js, "100")
	if count > 2 {
		t.Errorf("drift-heatmap.js: literal '100' must appear at most twice (constant + adjacent comment); got %d occurrences", count)
	}
	// The workbench module must read the value from the shared
	// namespace, not redeclare it. No literal 100 should appear in
	// drift-workbench.js at all (any future change adding a magic
	// number would surface here).
	wj := driftWorkbenchJS(t)
	if strings.Count(wj, "100") > 0 {
		t.Error("drift-workbench.js must not contain the literal '100'; read DRIFT_POINT_FETCH_LIMIT from MIDASExplorerDrift")
	}
}

func TestExplorer_DriftWorkbenchJS_UsesDrift1dReadEndpoints(t *testing.T) {
	js := driftWorkbenchJS(t)
	for _, want := range []string{
		`'/v1/drift/series/'`,
		`'/points?limit=' + limit`,
		`'/observations'`,
		`'/annotations'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-workbench.js must use Drift-1d read endpoint %q", want)
		}
	}
}

func TestExplorer_DriftWorkbenchJS_ReadsLatestPointHelperFromHeatmap(t *testing.T) {
	js := driftWorkbenchJS(t)
	if !strings.Contains(js, "MIDASExplorerDrift.selectLatestPointByWindowEnd") {
		t.Error("drift-workbench.js must reuse the shared selectLatestPointByWindowEnd helper from MIDASExplorerDrift")
	}
	// The workbench must not redeclare a duplicate latest-point
	// selector function — the helper lives only in drift-heatmap.js.
	if strings.Contains(js, `function selectLatestPointByWindowEnd`) {
		t.Error("drift-workbench.js must not redeclare selectLatestPointByWindowEnd; reuse from window.MIDASExplorerDrift")
	}
}

func TestExplorer_DriftWorkbenchJS_SortsOrSelectsByWindowEnd(t *testing.T) {
	js := driftWorkbenchJS(t)
	for _, want := range []string{
		`Date.parse(a.window_end`,
		`Date.parse(b.window_end`,
		`window_end`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-workbench.js must sort/select by window_end token %q", want)
		}
	}
}

func TestExplorer_DriftWorkbenchJS_BackendPointRangeTODO(t *testing.T) {
	heatmap := driftHeatmapJS(t)
	if !strings.Contains(heatmap, "TODO: replace client-side point loading with backend latest/range/sort") {
		t.Error("drift-heatmap.js must contain the brief-required TODO for backend point range/latest support")
	}
	// drift-workbench.js must contain the brief-required TODO for
	// detector-aware threshold overlays near the chart code.
	js := driftWorkbenchJS(t)
	if !strings.Contains(js, "TODO: add detector-aware threshold overlays when chart value\n  // semantics are metric-specific.") {
		t.Error("drift-workbench.js must contain the brief-required TODO for detector-aware threshold overlays")
	}
}

// ---------------------------------------------------------------------------
// Series-selection ordering pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftJS_HeatmapWorstStatusOrderingUnchanged(t *testing.T) {
	// Heatmap worst-status precedence must remain breached-first
	// (Drift-2b contract). This test re-pins the order against the
	// heatmap source so a Drift-2c regression here surfaces under
	// the right name.
	js := driftHeatmapJS(t)
	wantOrdered := []string{
		`'breached'`,
		`'unknown_detector_error'`,
		`'warning'`,
		`'unknown_insufficient_data'`,
		`'healthy'`,
		`'empty'`,
	}
	idx := strings.Index(js, "WORST_STATUS_ORDER")
	if idx < 0 {
		t.Fatal("WORST_STATUS_ORDER constant missing from drift-heatmap.js")
	}
	tail := js[idx:]
	prev := -1
	for _, w := range wantOrdered {
		i := strings.Index(tail, w)
		if i < 0 {
			t.Errorf("WORST_STATUS_ORDER missing entry %q", w)
			continue
		}
		if i <= prev {
			t.Errorf("WORST_STATUS_ORDER entry %q out of order", w)
		}
		prev = i
	}
}

func TestExplorer_DriftWorkbenchJS_DefaultSeriesOrderingInvestigationFirst(t *testing.T) {
	js := driftWorkbenchJS(t)
	// Investigation-first: detector_error wins over breached because a
	// broken detector must be fixed before interpreting breached
	// values.
	wantOrdered := []string{
		`'unknown_detector_error'`,
		`'breached'`,
		`'warning'`,
		`'unknown_insufficient_data'`,
		`'healthy'`,
		`'empty'`,
	}
	idx := strings.Index(js, "WORKBENCH_SELECTION_ORDER")
	if idx < 0 {
		t.Fatal("WORKBENCH_SELECTION_ORDER constant missing from drift-workbench.js")
	}
	tail := js[idx:]
	prev := -1
	for _, w := range wantOrdered {
		i := strings.Index(tail, w)
		if i < 0 {
			t.Errorf("WORKBENCH_SELECTION_ORDER missing entry %q", w)
			continue
		}
		if i <= prev {
			t.Errorf("WORKBENCH_SELECTION_ORDER entry %q out of brief-required order", w)
		}
		prev = i
	}
}

// ---------------------------------------------------------------------------
// Chart pins — vanilla SVG, magnitude only, no thresholds, backfill marker
// ---------------------------------------------------------------------------

func TestExplorer_DriftWorkbenchJS_ChartUsesVanillaSVG(t *testing.T) {
	js := driftWorkbenchJS(t)
	if !strings.Contains(js, `<svg class="drift-workbench-chart"`) {
		t.Error("drift-workbench.js must render the chart as inline SVG")
	}
	// Negative: no chart-library / bundler imports.
	for _, forbidden := range []string{
		`import * from`,
		`require('chart`,
		`require("chart`,
		`require('plotly`,
		`require("plotly`,
		`require('d3`,
		`require("d3`,
		`from 'chart`,
		`from "chart`,
		`from 'd3`,
		`from "d3`,
		`Chart.register`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must not reference external charting library: %q", forbidden)
		}
	}
}

func TestExplorer_DriftWorkbenchJS_ChartYValueIsMagnitude(t *testing.T) {
	js := driftWorkbenchJS(t)
	// y-axis label must be the literal "Magnitude" so a future units
	// pass can grep for it cleanly.
	if !strings.Contains(js, `>Magnitude<`) {
		t.Error("drift-workbench.js must render the y-axis title text 'Magnitude'")
	}
	if !strings.Contains(js, `Number(p.magnitude)`) {
		t.Error("drift-workbench.js must compute y-position from p.magnitude")
	}
	// Forbidden y-axis labels — the brief explicitly says do not
	// mislabel magnitude as confidence/latency/rate/percentage/PSI.
	// These checks are word-boundary substring; PSI is uppercase.
	for _, forbidden := range []string{
		`>Confidence<`, `>Latency<`, `>Rate<`, `>Percentage<`, `>PSI<`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must not mislabel magnitude axis as %q", forbidden)
		}
	}
}

func TestExplorer_DriftWorkbenchJS_NoThresholdRendering(t *testing.T) {
	js := driftWorkbenchJS(t)
	css := driftWorkbenchCSS(t)
	for _, forbidden := range []string{
		`drift-workbench-threshold-band`,
		`drift-workbench-threshold-line`,
		`thresholdBand`,
		`thresholdLine`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must not render threshold token %q in Drift-2c", forbidden)
		}
		if strings.Contains(css, forbidden) {
			t.Errorf("drift.css must not declare threshold selector %q in Drift-2c", forbidden)
		}
	}
}

func TestExplorer_DriftWorkbenchJS_BackfillMarkerExists(t *testing.T) {
	js := driftWorkbenchJS(t)
	if !strings.Contains(js, `drift-workbench-backfill-marker`) {
		t.Error("drift-workbench.js must render a distinct backfill marker for backfilled points")
	}
	if !strings.Contains(js, `is-backfilled`) {
		t.Error("drift-workbench.js must use the .is-backfilled modifier on backfilled points")
	}
	css := driftWorkbenchCSS(t)
	hasBackfilledPoint := strings.Contains(css, `.drift-workbench-point.is-backfilled`)
	hasBackfillMarker := strings.Contains(css, `.drift-workbench-backfill-marker`)
	if !hasBackfilledPoint && !hasBackfillMarker {
		t.Error("drift.css must style .drift-workbench-point.is-backfilled and/or .drift-workbench-backfill-marker")
	}
}

// ---------------------------------------------------------------------------
// Status / backfill separation pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftWorkbenchCSS_StatusClassesPresent(t *testing.T) {
	css := driftWorkbenchCSS(t)
	for _, want := range []string{
		`.drift-workbench-point.status-healthy`,
		`.drift-workbench-point.status-warning`,
		`.drift-workbench-point.status-breached`,
		`.drift-workbench-point.status-unknown-insufficient-data`,
		`.drift-workbench-point.status-unknown-detector-error`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("drift.css missing workbench status class %q", want)
		}
	}
}

func TestExplorer_DriftWorkbench_NoCombinedStatusBackfillClass(t *testing.T) {
	css := driftWorkbenchCSS(t)
	js := driftWorkbenchJS(t)
	// Pin the actual selector form (leading `.`) and the JS class-
	// emission form so explanatory comments documenting the
	// forbidden patterns can still mention them by name. A bare
	// substring scan tripped on its own documentation; this tighter
	// pin only catches real selectors and class-list assignments.
	cssSelectors := []string{
		`.status-healthy-backfilled`,
		`.status-warning-backfilled`,
		`.status-breached-backfilled`,
		`.status-unknown-insufficient-data-backfilled`,
		`.status-unknown-detector-error-backfilled`,
		`.drift-status-backfilled`,
		`.drift-workbench-status-backfilled`,
	}
	for _, c := range cssSelectors {
		if strings.Contains(css, c) {
			t.Errorf("drift.css must not introduce combined status/backfill selector %q (backfill is orthogonal)", c)
		}
	}
	// In JS we forbid the class names being pushed onto a class list
	// (with surrounding quotes) — that's the actual emission form.
	jsEmissions := []string{
		`'status-warning-backfilled'`, `"status-warning-backfilled"`,
		`'status-breached-backfilled'`, `"status-breached-backfilled"`,
		`'status-healthy-backfilled'`, `"status-healthy-backfilled"`,
		`'status-unknown-insufficient-data-backfilled'`, `"status-unknown-insufficient-data-backfilled"`,
		`'status-unknown-detector-error-backfilled'`, `"status-unknown-detector-error-backfilled"`,
		`'drift-status-backfilled'`, `"drift-status-backfilled"`,
	}
	for _, c := range jsEmissions {
		if strings.Contains(js, c) {
			t.Errorf("drift-workbench.js must not emit combined status/backfill class %q", c)
		}
	}
	// 'backfilled' must not be promoted to a detector status value.
	for _, forbidden := range []string{
		`'backfilled': 'drift-status`,
		`detector_status: 'backfilled'`,
		`DetectorStatus: 'backfilled'`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must not treat backfilled as a detector status: %q", forbidden)
		}
	}
}

func TestExplorer_DriftWorkbenchCSS_NoGenericUnknownClass(t *testing.T) {
	css := driftWorkbenchCSS(t)
	// Any drift-workbench-point.status-… class containing the word
	// "unknown" must be exactly one of the two allowed forms.
	re := regexp.MustCompile(`\.drift-workbench-point\.status-[A-Za-z0-9_-]*unknown[A-Za-z0-9_-]*`)
	for _, m := range re.FindAllString(css, -1) {
		if m != ".drift-workbench-point.status-unknown-insufficient-data" &&
			m != ".drift-workbench-point.status-unknown-detector-error" {
			t.Errorf("drift.css introduces non-allowed unknown class %q", m)
		}
	}
	bare := regexp.MustCompile(`\.status-unknown([^-A-Za-z0-9_]|$)`)
	if loc := bare.FindStringIndex(css); loc != nil {
		t.Errorf("drift.css must not introduce a generic .status-unknown class at byte %d", loc[0])
	}
}

// ---------------------------------------------------------------------------
// Runtime-inertness source pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftWorkbenchJS_RuntimeInert(t *testing.T) {
	js := driftWorkbenchJS(t)
	for _, forbidden := range []string{
		"DriftSeriesCreate",
		"DriftSeriesPointCreate",
		"DriftObservationCreate",
		"DriftAnnotationCreate",
		"UpdateOperatorStatus",
		"Supersede",
		"runDriftAggregation",
		"runDriftDetector",
		"auditEnvelopeAppend",
		"appendAuditEnvelope",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must remain runtime-inert; forbidden token %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Empty / error-state copy pins (workbench)
// ---------------------------------------------------------------------------

func TestExplorer_DriftWorkbenchJS_EmptyAndErrorStateCopy(t *testing.T) {
	js := driftWorkbenchJS(t)
	for _, want := range []string{
		"Select a drift cell to inspect the time series.",
		"No drift definition exists for this entity and drift type.",
		"This drift series has no points.",
		"Observations could not be loaded.",
		"Annotations could not be loaded.",
		"Drift repository is not configured.",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-workbench.js missing required state copy %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// V2 drift type / no-NPM / no-bundler pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftWorkbench_NoV2DriftTypes(t *testing.T) {
	js := driftWorkbenchJS(t)
	css := driftWorkbenchCSS(t)
	for _, forbidden := range []string{
		"'population'", "'data drift'", "'prediction'", "'concept'",
		"Population", "Prediction", "Concept",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must not reference V2 drift type %q", forbidden)
		}
		if strings.Contains(css, forbidden) {
			t.Errorf("drift.css must not reference V2 drift type %q", forbidden)
		}
	}
}

func TestExplorer_DriftWorkbench_NoNPMOrBundlerOrFramework(t *testing.T) {
	js := driftWorkbenchJS(t)
	for _, forbidden := range []string{
		`require(`,
		`module.exports`,
		`from 'react'`, `from "react"`,
		`from 'vue'`, `from "vue"`,
		`webpack`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-workbench.js must not use npm/bundler/framework token %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Backend / OpenAPI no-change pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftWorkbench_NoNewBackendRoutesRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, probe := range []string{
		"/v1/drift/workbench",
		"/v1/drift/level2",
		"/v1/drift/series-detail",
	} {
		rec := performRequest(t, srv, http.MethodGet, probe, nil)
		if rec.Code == http.StatusOK || rec.Code == http.StatusNotImplemented {
			t.Errorf("Drift-2c must not introduce backend route %s; GET returned %d", probe, rec.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// Workbench wiring pins on the heatmap side
// ---------------------------------------------------------------------------

func TestExplorer_DriftHeatmapJS_ExposesCacheAccessorsForWorkbench(t *testing.T) {
	js := driftHeatmapJS(t)
	for _, want := range []string{
		`getDriftLoaded`,
		`getDriftModel`,
		`driftFetchJSON: driftFetchJSON`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-heatmap.js must export %q on MIDASExplorerDrift for the workbench", want)
		}
	}
}
