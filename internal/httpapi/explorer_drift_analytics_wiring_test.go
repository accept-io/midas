package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplorer_DriftAnalytics_APIClientExposesBackendGET(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/core/api-client.js")
	for _, want := range []string{
		"analytics: function (nodeKind, nodeId, range, opts)",
		"/v1/drift/analytics",
		"node_kind: nodeKind",
		"node_id: nodeId",
		"range: range || '30d'",
		"return request('/v1/drift/analytics' + q, { signal: opts.signal })",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift analytics API client must contain %q", want)
		}
	}
	driftBlock := js[strings.Index(js, "var drift = {"):]
	driftBlock = driftBlock[:strings.Index(driftBlock, "  var evidence = {")]
	for _, forbidden := range []string{"method: 'POST'", "method: 'PUT'", "method: 'PATCH'", "method: 'DELETE'"} {
		if strings.Contains(driftBlock, forbidden) {
			t.Errorf("drift API client must not add mutating drift analytics call %q", forbidden)
		}
	}
}

func TestExplorer_DriftAnalytics_PanelMapsBackendChartAndSources(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	for _, want := range []string{
		"function _backendViewModel(resp, nodeRef)",
		"resp.dataAvailable !== true",
		"chart.observed",
		"chart.expected",
		"chart.watch",
		"chart.breach",
		"chart.yDomain",
		"chart.currentStatus",
		"projectionAsOf: resp.projectionAsOf || null",
		"provenance: resp.provenance || null",
		"sourceStateLabel: 'Chart from backend'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel backend mapping must contain %q", want)
		}
	}
	for _, want := range []string{
		"src.observedSeries = src.observedSeries || 'unavailable'",
		"src.expectedBaseline = src.expectedBaseline || 'unavailable'",
		"src.thresholds = src.thresholds || 'unavailable'",
		"src.status = src.status || 'unavailable'",
		"src.provenance = src.provenance || 'not_available'",
		"src.compositeScore = 'demo_provisional'",
		"src.contributionValues = 'demo_provisional'",
		"src.contributionWeights = 'demo_provisional'",
		"src.graphOverlay = 'not_implemented'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel source classification must contain %q", want)
		}
	}
}

func TestExplorer_DriftAnalytics_FallbackAndStaleResponseGuards(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	for _, want := range []string{
		"_state.requestSeq += 1",
		"if (seq !== _state.requestSeq) return",
		"AbortController",
		"_state.pendingAbort.abort()",
		"resp && resp.__status",
		"_renderViewModel(vm || _fallbackForNode(nodeRef, 'Demo evidence'))",
		"err.name === 'AbortError'",
		"sourceStateLabel: 'Demo evidence'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel fallback/stale guard must contain %q", want)
		}
	}
}

func TestExplorer_DriftAnalytics_SelectedNodeMappingUsesExistingRefs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		"getSelectedNodeRef:",
		"graphSelectionBridge",
		"shared.getCurrentNodeRef()",
		"shared.getCurrentCard()",
		"contextSelectionBridge",
		"context.getCurrentCard()",
		"card.sourceNodeRef",
		"state.selectedNodeRef",
		"gmapSelectedId",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("boot wiring must expose selected node ref token %q", want)
		}
	}
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	for _, want := range []string{
		"function _resolveNodeRef(input)",
		"bs: 'business_service'",
		"businessservice: 'business_service'",
		"capability: 'capability'",
		"process: 'process'",
		"decisionsurface: 'decision_surface'",
		"agent: 'agent'",
		"authorityprofile: 'authority_profile'",
		"surface: 'decision_surface'",
		"ai: 'ai_system'",
		"raw.replace(/[\\s_-]+/g, '').toLowerCase()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("selected-node resolver must contain %q", want)
		}
	}
	for _, forbidden := range []string{
		"related_business_service: 'business_service'",
		"relatedbusinessservice: 'business_service'",
		"coverage: 'business_service'",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("unsupported visual/relationship node must not map to backend drift target: %q", forbidden)
		}
	}
}

func TestExplorer_DriftAnalytics_CompactPanelCallsBackendWithNormalisedBusinessService(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/drift/drift-analytics-panel.js")
	for _, want := range []string{
		"api.analytics(nodeRef.kind, nodeRef.id, '30d'",
		"kind = _backendNodeKind(kind)",
		"businessservice: 'business_service'",
		"business_service",
		"raw.slice(0, sep)",
		"raw.slice(sep + 1)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("compact panel must normalise selected refs before backend call; missing %q", want)
		}
	}
	if strings.Contains(js, "api.analytics('BusinessService'") ||
		strings.Contains(js, "node_kind=BusinessService") {
		t.Error("compact panel must not call drift analytics with BusinessService")
	}
}

func TestExplorer_DriftAnalytics_NegativeScopePins(t *testing.T) {
	root := repoRootForDriftAnalyticsTest(t)
	for _, path := range []string{
		"internal/httpapi/explorer/assets/js/drift/drift-analytics-panel.js",
		"internal/httpapi/explorer/assets/js/core/api-client.js",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("expected repo file %s: %v", path, err)
		}
	}
	panel := readRepoFileForDriftAnalyticsTest(t, root, "internal/httpapi/explorer/assets/js/drift/drift-analytics-panel.js")
	for _, forbidden := range []string{
		"hash verified",
		"reconstructible",
		"graph overlay",
		"GraphOverlay",
		"maximised Drift Analysis",
		"Maximized Drift Analysis",
	} {
		if strings.Contains(panel, forbidden) {
			t.Errorf("compact drift panel must not introduce out-of-scope claim/feature %q", forbidden)
		}
	}
	for _, forbiddenPath := range []string{
		"schema.sql",
		"migrations",
		"deploy/new-service",
	} {
		if strings.Contains(panel, forbiddenPath) {
			t.Errorf("compact drift panel must not reference out-of-scope path %q", forbiddenPath)
		}
	}
}

func repoRootForDriftAnalyticsTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func readRepoFileForDriftAnalyticsTest(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}
