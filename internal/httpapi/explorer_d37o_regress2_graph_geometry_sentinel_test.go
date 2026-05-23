package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const d37oRegress2SentinelAsset = "/explorer/assets/js/graph/graph-platform/graph-geometry-sentinel.js"

func TestExplorer_D37oRegress2_GraphGeometrySentinelAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oRegress2SentinelAsset)
	if len(js) == 0 {
		t.Fatal("D37o-regress-2: graph geometry sentinel asset must be served")
	}
}

func TestExplorer_D37oRegress2_GraphGeometrySentinelLoadedByExplorer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	tag := `src="/explorer/assets/js/graph/graph-platform/graph-geometry-sentinel.js"`
	if !strings.Contains(body, tag) {
		t.Fatalf("D37o-regress-2: Explorer must load graph-geometry-sentinel.js with a plain script tag")
	}

	toolbarIdx := strings.Index(body, "graph-camera-toolbar-adapter.js")
	sentinelIdx := strings.Index(body, "graph-geometry-sentinel.js")
	engineIdx := strings.Index(body, "graph-cytoscape-engine.js")
	if toolbarIdx < 0 || sentinelIdx < 0 || engineIdx < 0 {
		t.Fatalf("D37o-regress-2: expected toolbar, sentinel, and engine scripts in Explorer")
	}
	if !(toolbarIdx < sentinelIdx && sentinelIdx < engineIdx) {
		t.Fatalf("D37o-regress-2: sentinel must load after graph camera toolbar adapter and before graph-cytoscape-engine.js (toolbar=%d sentinel=%d engine=%d)", toolbarIdx, sentinelIdx, engineIdx)
	}
}

func TestExplorer_D37oRegress2_GraphGeometryPublicApiPinned(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oRegress2SentinelAsset)
	for _, want := range []string{
		"window.MIDASExplorerGraph.geometry = {",
		"check: check",
		"checkCurrent: checkCurrent",
		"checkKnownSurfaces: checkKnownSurfaces",
		"knownSurfaces: KNOWN_SURFACES.slice()",
		"runFlaggedOnce: _scheduleFlaggedRun",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-regress-2: sentinel public API missing %q", want)
		}
	}
}

func TestExplorer_D37oRegress2_GraphGeometryUsesOnlyBrowserNativeMeasurement(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oRegress2SentinelAsset)
	for _, want := range []string{
		"getBoundingClientRect()",
		"requestAnimationFrame",
		"querySelectorAll(selector)",
		"new URLSearchParams(search)",
		"getActiveRendererId",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-regress-2: zero-dependency sentinel must use browser/local API %q", want)
		}
	}
	for _, forbidden := range []string{
		"import ",
		"require(",
		"playwright",
		"puppeteer",
		"cypress",
		"jest",
		"node_modules",
		"package.json",
		"package-lock.json",
	} {
		if strings.Contains(strings.ToLower(js), strings.ToLower(forbidden)) {
			t.Errorf("D37o-regress-2: sentinel must not introduce external tooling/dependency marker %q", forbidden)
		}
	}
}

func TestExplorer_D37oRegress2_GraphGeometryClassificationVocabulary(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oRegress2SentinelAsset)
	for _, want := range []string{
		"pass",
		"overlap",
		"not_applicable",
		"no_cards",
		"active_renderer_mismatch",
		"measurement_error",
	} {
		if !strings.Contains(js, "'"+want+"'") {
			t.Errorf("D37o-regress-2: classification %q must be present", want)
		}
	}
}

func TestExplorer_D37oRegress2_KnownSurfaceRegistryPinned(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oRegress2SentinelAsset)
	for _, want := range []string{
		"id: 'strategic-context'",
		"selector: '.midas-graph-viewport[data-active-renderer=\"context\"] .context-card'",
		"expectedActiveRendererId: 'context'",
		"id: 'legacy-context'",
		"selector: '#gmap-canvas .gmap-node:not(.authority-poc-inspector-carrier)'",
		"expectedActiveRendererId: 'native-context'",
		"id: 'authority'",
		"selector: '.midas-graph-viewport[data-active-renderer=\"authority\"] .cytoscape-poc-html-card'",
		"expectedActiveRendererId: 'authority'",
		"id: 'context-cytoscape-spike'",
		"selector: '.midas-graph-viewport[data-active-renderer=\"context-cytoscape\"] .context-cy-spike-overlay .gmap-node'",
		"id: 'drift-heatmap'",
		"selector: '.drift-heatmap .drift-heatmap-cell'",
		"id: 'knowledge-shell'",
		"selector: '.midas-graph-viewport[data-active-renderer=\"knowledge-graph\"] .knowledge-graph-mount-card'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-regress-2: known surface registry missing %q", want)
		}
	}
}

func TestExplorer_D37oRegress2_GraphGeometryDiagnosticFlagPinned(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oRegress2SentinelAsset)
	for _, want := range []string{
		"graphGeometryDiag",
		"console.info('[graph-geometry-sentinel]'",
		"document.addEventListener('DOMContentLoaded', _scheduleFlaggedRun, { once: true })",
		"raf(function () { raf(run); });",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-regress-2: diagnostic flag/settle path missing %q", want)
		}
	}
}

func TestExplorer_D37oRegress2_ResultShapePinned(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oRegress2SentinelAsset)
	for _, want := range []string{
		"surfaceId:",
		"surfaceLabel:",
		"url:",
		"activeRendererId:",
		"selectorUsed:",
		"candidateElementCount:",
		"measuredElementCount:",
		"overlapCount:",
		"overlapPairs:",
		"outOfViewportCount:",
		"outOfViewportElements:",
		"classification:",
		"reason:",
		"overlapWidth:",
		"overlapHeight:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-regress-2: result shape must include %q", want)
		}
	}
}

func TestExplorer_D37oRegress2_NoPackageManagerContamination(t *testing.T) {
	for _, name := range []string{"package.json", "package-lock.json", "node_modules"} {
		path := filepath.Join("..", "..", name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("D37o-regress-2: %s must not be created for the zero-dependency sentinel", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("D37o-regress-2: could not inspect %s: %v", name, err)
		}
	}
}
