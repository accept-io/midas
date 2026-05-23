package httpapi

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	d37o8IndexAsset     = "/explorer"
	d37o8PolicyAsset    = "/explorer/assets/js/graph/graph-platform/graph-footprint-policy.js"
	d37o8ContextAsset   = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37o8StageAsset     = "/explorer/assets/js/graph/graph-platform/graph-stage.js"
	d37o8EngineAsset    = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37o8AuthorityAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

func TestExplorer_FootprintPolicy_LoadsBeforeContextRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	html := getExplorerAsset(t, srv, d37o8IndexAsset)

	policyIdx := strings.Index(html, "/explorer/assets/js/graph/graph-platform/graph-footprint-policy.js")
	contextIdx := strings.Index(html, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")
	if policyIdx < 0 {
		t.Fatal("D37o-overlap-8: index.html must load graph-footprint-policy.js")
	}
	if contextIdx < 0 {
		t.Fatal("D37o-overlap-8: index.html must load context-cytoscape-renderer.js")
	}
	if policyIdx > contextIdx {
		t.Fatalf("D37o-overlap-8: graph-footprint-policy.js must load before context-cytoscape-renderer.js")
	}
}

func TestExplorer_FootprintPolicy_ExposesResolverNamespace(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37o8PolicyAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.footprintPolicy",
		"resolve: resolve",
		"function resolve(input)",
		"GRAPH_SURFACE_CONTEXT = 'context'",
		"RENDERER_MODE_RAW_CY  = 'raw-cytoscape'",
		"SIZING_MODE_RAW_FIXED = 'raw-cytoscape-node-body-fixed'",
		"rawCytoscapeCompatible: true",
		"htmlOverlayCompatible: false",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-8: footprint policy resolver missing marker %q", want)
		}
	}
}

func TestExplorer_FootprintPolicy_ContextRawDimensionsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37o8PolicyAsset)

	expected := map[string][2]int{
		"business_service":         {220, 132},
		"related_business_service": {220, 76},
		"capability":               {220, 84},
		"process":                  {220, 84},
		"decision_surface":         {220, 104},
		"ai_system":                {220, 96},
		"ai_system_binding":        {220, 84},
		"coverage":                 {220, 96},
		"authority_summary":        {220, 96},
	}
	for kind, dims := range expected {
		want := regexp.MustCompile(regexp.QuoteMeta(kind) + `:\s*Object\.freeze\(\{\s*width:\s*` +
			intRe(dims[0]) + `,\s*height:\s*` + intRe(dims[1]) + `\s*\}\)`)
		if !want.MatchString(js) {
			t.Errorf("D37o-overlap-8: raw Context policy for %s must preserve %dx%d", kind, dims[0], dims[1])
		}
	}

	for _, want := range []string{
		"var RAW_CONTEXT_GAP_X = 32;",
		"var RAW_CONTEXT_GAP_Y = 72;",
		"gapX: RAW_CONTEXT_GAP_X",
		"gapY: RAW_CONTEXT_GAP_Y",
		"tolerance: RAW_CONTEXT_TOLERANCE",
		"source: SOURCE_CONTEXT_ESTIMATE",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-8: raw Context policy must preserve gap/source marker %q", want)
		}
	}
}

func intRe(v int) string {
	return regexp.QuoteMeta(strconv.Itoa(v))
}

func TestExplorer_FootprintPolicy_ContextRawRendererConsumesResolver(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37o8ContextAsset)

	for _, want := range []string{
		"function _resolveContextRawFootprint(card)",
		"window.MIDASExplorerGraph.footprintPolicy",
		"policy.resolve({",
		"graphSurfaceId: 'context'",
		"rendererMode: 'raw-cytoscape'",
		"width: resolved.reservedWidth",
		"height: resolved.reservedHeight",
		"gapX: resolved.gapX",
		"gapY: resolved.gapY",
		"out[id] = _resolveContextRawFootprint(c);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-overlap-8: Context raw renderer must consume footprint policy resolver marker %q", want)
		}
	}

	buildIdx := strings.Index(js, "function _buildCardFootprints(cards, stageConsts)")
	if buildIdx < 0 {
		t.Fatal("D37o-overlap-8: _buildCardFootprints must exist")
	}
	body := js[buildIdx:]
	if next := strings.Index(body[1:], "\n  function "); next > 0 {
		body = body[:next+1]
	}
	if strings.Contains(body, "_estimatedContextCardFootprint(c)") {
		t.Errorf("D37o-overlap-8: active Context footprint builder must not call the old estimator")
	}
}

func TestExplorer_FootprintPolicy_RawContextStageAndEngineGeometryPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	stage := getExplorerAsset(t, srv, d37o8StageAsset)
	ctx := getExplorerAsset(t, srv, d37o8ContextAsset)
	engine := getExplorerAsset(t, srv, d37o8EngineAsset)

	for _, want := range []string{
		"normaliseCardFootprints: normaliseCardFootprints",
		"function _resolveFootprint(footprints, cardId, defaults, diagnostics)",
		"x += fp.width + defaults.gapX",
		"DEFAULT_GAP_X:            DEFAULT_GAP_X",
		"DEFAULT_GAP_Y:            DEFAULT_GAP_Y",
	} {
		if !strings.Contains(stage, want) {
			t.Errorf("D37o-overlap-8: graph-stage geometry contract missing %q", want)
		}
	}

	for _, want := range []string{
		"graphStage.compose(layout, footprints, safeArea, {})",
		"width:       w",
		"height:      h",
		"position: {",
		"+ w / 2",
		"+ h / 2",
		"overlayEnabled: false",
	} {
		if !strings.Contains(ctx, want) {
			t.Errorf("D37o-overlap-8: Context stage-to-engine geometry marker missing %q", want)
		}
	}

	for _, want := range []string{
		"'width':              'data(width)'",
		"'height':             'data(height)'",
		"var overlayEnabled = (opts.overlayEnabled !== false);",
	} {
		if !strings.Contains(engine, want) {
			t.Errorf("D37o-overlap-8: engine node geometry/overlay contract missing %q", want)
		}
	}
}

func TestExplorer_FootprintPolicy_AuthorityIsolation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	auth := getExplorerAsset(t, srv, d37o8AuthorityAsset)

	for _, banned := range []string{
		"footprintPolicy",
		"graph-footprint-policy",
		"_resolveContextRawFootprint",
		"_estimatedContextCardFootprint",
		"graphStage.compose",
	} {
		if strings.Contains(auth, banned) {
			t.Errorf("D37o-overlap-8: Authority must remain isolated from Context footprint policy marker %q", banned)
		}
	}
}
