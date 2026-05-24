package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

const d37pAuthorityProjectionOverlayAsset = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js"

func TestExplorer_GraphCytoscapeOverlay_AuthorityProjectionContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuthorityProjectionOverlayAsset)

	for _, want := range []string{
		"PROJECTION_MODEL_LAYER_TRANSFORM = 'model-layer-transform'",
		"var projectionMode = _str(opts.projectionMode) || PROJECTION_MODEL_LAYER_TRANSFORM;",
		"_layerEl.setAttribute('data-projection-mode', projectionMode);",
		"_layerEl.style.transformOrigin = 'top left';",
		"pan = (typeof cy.pan === 'function') ? cy.pan() : { x: 0, y: 0 };",
		"zoom = (typeof cy.zoom === 'function') ? cy.zoom() : 1;",
		"scale(' + zoom + ')'",
		"try { p = n.position(); } catch (_) { p = null; }",
		"entry.wrapper.style.transform = 'translate(' + tx + 'px, ' + ty + 'px)'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37p authority-projection: shared overlay must contain %q", want)
		}
	}

	if strings.Contains(js, "renderedPosition()") {
		t.Fatalf("D37p authority-projection: shared overlay must not use renderedPosition() in the production projection contract")
	}
	if regexp.MustCompile(`wrapper\.style\.transform\s*=\s*[^;]*translate\(-50%`).MatchString(js) {
		t.Fatalf("D37p authority-projection: zero-size wrappers must not be centred with translate(-50%%, -50%%)")
	}
	if !regexp.MustCompile(`Math\.round\(p\.x\s*-\s*w\s*/\s*2\)`).MatchString(js) {
		t.Fatalf("D37p authority-projection: card x placement must subtract half the inner card width")
	}
	if !regexp.MustCompile(`Math\.round\(p\.y\s*-\s*h\s*/\s*2\)`).MatchString(js) {
		t.Fatalf("D37p authority-projection: card y placement must subtract half the inner card height")
	}
}

func TestExplorer_GraphCytoscapeOverlay_AuthorityProjectionUsesInnerCardDimensions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuthorityProjectionOverlayAsset)

	for _, want := range []string{
		"var w = Number(entry.inner.offsetWidth || 0);",
		"var h = Number(entry.inner.offsetHeight || 0);",
		"fallbackWidth: fp ? fp.width : 0",
		"fallbackHeight: fp ? fp.height : 0",
		"width: entry.measuredWidth / zoom",
		"height: entry.measuredHeight / zoom",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37p authority-projection: shared overlay inner-dimension centring must contain %q", want)
		}
	}

	if regexp.MustCompile(`(?s)function _syncCard\(entry\)[\s\S]*?measuredWidth[\s\S]*?renderedPosition`).MatchString(js) {
		t.Fatalf("D37p authority-projection: card centring must not subtract measured dimensions from renderedPosition")
	}
}

func TestExplorer_GraphCytoscapeOverlay_NativeNodeVisibilityOption(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuthorityProjectionOverlayAsset)

	for _, want := range []string{
		"NATIVE_NODE_VISIBILITY_HIDDEN    = 'hidden-while-mounted'",
		"NATIVE_NODE_VISIBILITY_PRESERVE  = 'preserve'",
		"var nativeNodeVisibility = _str(opts.nativeNodeVisibility) || NATIVE_NODE_VISIBILITY_HIDDEN;",
		"var hideNativeNodes = nativeNodeVisibility === NATIVE_NODE_VISIBILITY_HIDDEN;",
		"if (!hideNativeNodes) return;",
		"cy.nodes().style({",
		"'text-opacity': 0",
		"'border-opacity': 0",
		"'background-opacity': 0",
		"cy.nodes().removeStyle('opacity text-opacity border-opacity background-opacity');",
		"_restoreNativeNodes();",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37p authority-projection: native node visibility contract must contain %q", want)
		}
	}
}

func TestExplorer_GraphCytoscapeOverlay_AuthorityProjectionKeepsMeasurementSecondary(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pAuthorityProjectionOverlayAsset)

	for _, want := range []string{
		"var onMeasure = _isFn(opts.onMeasure) ? opts.onMeasure : null;",
		"try { onMeasure(key, entry.measuredWidth, entry.measuredHeight); }",
		"Projection correctness",
		"does not depend on measurement-driven recompose",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("D37p authority-projection: measurement must remain available but secondary; missing %q", want)
		}
	}

	if strings.Contains(js, "request_recompose") || strings.Contains(js, "updatedFootprintCandidates") {
		t.Fatalf("D37p authority-projection: shared overlay must not own Context measurement/recompose semantics")
	}
}

func TestExplorer_GraphCytoscapeOverlay_NoContextSpecificProjectionMath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	contextJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	for _, forbidden := range []string{
		"model-layer-transform",
		"projectionMode:",
		"nativeNodeVisibility:",
	} {
		if strings.Contains(contextJS, forbidden) {
			t.Fatalf("D37p authority-projection: Context must not own shared overlay projection wiring %q", forbidden)
		}
	}
}
