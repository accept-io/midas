package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37x-engine-node-geometry-contract — Platform-level node geometry & label
// policy. The shared graph engine governs node body geometry and native
// Cytoscape label policy on behalf of every lens.
//
// Required invariants:
//
//   • Engine base style sizes nodes from declared `data(width)` and
//     `data(height)`. No `width: "label"` / `height: "label"` for
//     production lenses.
//   • Engine base style declares `label: ''` so cy renders no native
//     label by default. Lenses opt in by overriding the label style.
//   • Engine fit paths read `boundingBox(FIT_BBOX_OPTS)` with
//     `includeLabels: false` and the related label / overlay /
//     outline flags off — fit math uses declared bodies, not
//     rendered label / overlay overflow.
//   • Engine emits `node_geometry_invalid_dimensions` (dedup'd) when
//     any supplied node lacks positive finite width/height.
//   • The native label helper module exists, exposes constants, and
//     publishes `makeNativeNodeLabel(text, width, height, options?)`.
//   • Context renderer derives `data.label` via the helper and binds
//     the cy native label to `data(label)` — NOT `data(id)` (raw
//     `kind:id` technical strings).
//   • D37u passive resize policy is intact.

const (
	d37xEngineModule       = "/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"
	d37xNativeLabelsModule = "/explorer/assets/js/graph/graph-platform/graph-native-labels.js"
	d37xContextRenderer    = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37xIndexHTML          = "/explorer"
	d37xAuthorityPoc       = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
)

// ── 1. Engine base-style geometry contract ────────────────────────────

func TestExplorer_D37xEngineGeometry_BaseStyleDeclaresContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xEngineModule)

	// Locate the engine's _buildBaseStyle() function and inspect its
	// `node` style entry directly.
	bIdx := strings.Index(js, "function _buildBaseStyle()")
	if bIdx < 0 {
		t.Fatal("D37x-engine-node-geometry-contract: _buildBaseStyle must exist")
	}
	bTail := js[bIdx:]
	bEnd := strings.Index(bTail, "\n  }\n")
	if bEnd < 0 {
		t.Fatalf("D37x-engine-node-geometry-contract: _buildBaseStyle body must be well-formed")
	}
	bBody := bTail[:bEnd+1]

	if !strings.Contains(bBody, "'width':              'data(width)'") {
		t.Errorf("D37x-engine-node-geometry-contract: base style must declare width = data(width)")
	}
	if !strings.Contains(bBody, "'height':             'data(height)'") {
		t.Errorf("D37x-engine-node-geometry-contract: base style must declare height = data(height)")
	}
	if !strings.Contains(bBody, "'label':              ''") {
		t.Errorf("D37x-engine-node-geometry-contract: base style must declare label = '' (no native label by default)")
	}
}

// ── 2. Engine base style must NOT use width/height: 'label' ──────────

func TestExplorer_D37xEngineGeometry_NoLabelDrivenNodeSize(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xEngineModule)

	// Scan non-comment lines for the forbidden `width: 'label'` /
	// `height: 'label'` declarations. Comments documenting the rule
	// itself are fine.
	for _, line := range strings.Split(js, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		if regexp.MustCompile(`'width'\s*:\s*'label'`).MatchString(line) {
			t.Errorf("D37x-engine-node-geometry-contract: width: 'label' is forbidden in executable engine code (found %q)", strings.TrimSpace(line))
		}
		if regexp.MustCompile(`'height'\s*:\s*'label'`).MatchString(line) {
			t.Errorf("D37x-engine-node-geometry-contract: height: 'label' is forbidden in executable engine code (found %q)", strings.TrimSpace(line))
		}
	}
}

// ── 3. Engine fit-bbox options exclude labels/overlays/outlines ─────

func TestExplorer_D37xEngineGeometry_FitBboxOptsExcludesLabels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xEngineModule)

	if !strings.Contains(js, "var FIT_BBOX_OPTS = {") {
		t.Fatal("D37x-engine-node-geometry-contract: engine must declare FIT_BBOX_OPTS as the shared fit bbox option set")
	}
	// All six label/overlay/outline switches must be false.
	required := []string{
		"includeLabels:       false",
		"includeMainLabels:   false",
		"includeSourceLabels: false",
		"includeTargetLabels: false",
		"includeOverlays:     false",
		"includeUnderlays:    false",
		"includeOutlines:     false",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37x-engine-node-geometry-contract: FIT_BBOX_OPTS must set %s (labels/overlays/outlines excluded from fit math)", r)
		}
	}
}

// ── 4. Engine fit paths use FIT_BBOX_OPTS ────────────────────────────

func TestExplorer_D37xEngineGeometry_FitPathsCallBboxWithOpts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xEngineModule)

	// Both fit paths must read bbox via FIT_BBOX_OPTS.
	if !strings.Contains(js, "eles.boundingBox(FIT_BBOX_OPTS)") {
		t.Errorf("D37x-engine-node-geometry-contract: engine fit paths must call boundingBox(FIT_BBOX_OPTS) so labels/overlays/outlines are excluded")
	}
	// Inside _fitWithSafeArea and _fitToUsableRect specifically.
	for _, fn := range []string{"function _fitWithSafeArea(cy, padding)", "function _fitToUsableRect(cy, usableRect, emitDiag)"} {
		fIdx := strings.Index(js, fn)
		if fIdx < 0 {
			t.Fatalf("D37x-engine-node-geometry-contract: %s must exist", fn)
		}
		fTail := js[fIdx:]
		fEnd := strings.Index(fTail[1:], "\n  function ")
		if fEnd < 0 {
			fEnd = strings.Index(fTail, "\n  // ──")
			if fEnd < 0 {
				fEnd = len(fTail) - 1
			}
		}
		fBody := fTail[:fEnd+1]
		if !strings.Contains(fBody, "eles.boundingBox(FIT_BBOX_OPTS)") {
			t.Errorf("D37x-engine-node-geometry-contract: %s must call eles.boundingBox(FIT_BBOX_OPTS)", fn)
		}
		// Negative: no default-options bbox call inside this function.
		if regexp.MustCompile(`eles\.boundingBox\(\s*\)`).MatchString(fBody) {
			t.Errorf("D37x-engine-node-geometry-contract: %s must NOT call eles.boundingBox() with default options (defaults include labels)", fn)
		}
	}
}

// ── 5. Geometry-invalid diagnostic is declared and emitted ──────────

func TestExplorer_D37xEngineGeometry_DiagnosticDeclaredAndEmitted(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xEngineModule)

	if !strings.Contains(js, "DIAG_NODE_GEOMETRY_INVALID") {
		t.Errorf("D37x-engine-node-geometry-contract: engine must declare DIAG_NODE_GEOMETRY_INVALID")
	}
	if !strings.Contains(js, "'node_geometry_invalid_dimensions'") {
		t.Errorf("D37x-engine-node-geometry-contract: DIAG_NODE_GEOMETRY_INVALID must map to the literal 'node_geometry_invalid_dimensions'")
	}
	if !strings.Contains(js, "_recordFitDiagnostic(DIAG_NODE_GEOMETRY_INVALID)") {
		t.Errorf("D37x-engine-node-geometry-contract: engine must emit DIAG_NODE_GEOMETRY_INVALID via _recordFitDiagnostic when a node lacks valid dimensions")
	}
	// Pre-mount scan flag must be checked.
	if !strings.Contains(js, "if (_nodeGeometryViolated)") {
		t.Errorf("D37x-engine-node-geometry-contract: engine must check _nodeGeometryViolated and emit the diagnostic once per mount")
	}
}

// ── 6. Native label helper module is served and exported ────────────

func TestExplorer_D37xNativeLabels_ModuleServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xNativeLabelsModule)

	if len(js) == 0 {
		t.Fatalf("D37x-engine-node-geometry-contract: native labels module must be served at %q", d37xNativeLabelsModule)
	}
	required := []string{
		"window.MIDASExplorerGraph.graphNativeLabels",
		"makeNativeNodeLabel: makeNativeNodeLabel",
		"function makeNativeNodeLabel(text, width, height, options)",
		"NATIVE_LABEL_FONT_SIZE_PX",
		"NATIVE_LABEL_LINE_HEIGHT_PX",
		"NATIVE_LABEL_PADDING_X_PX",
		"NATIVE_LABEL_PADDING_Y_PX",
		"NATIVE_LABEL_DEFAULT_MAX_LINES",
		"NATIVE_LABEL_GLYPH_WIDTH_RATIO",
		"NATIVE_LABEL_TRUNCATION_GLYPH",
		"_constants:",
	}
	for _, r := range required {
		if !strings.Contains(js, r) {
			t.Errorf("D37x-engine-node-geometry-contract: native labels module must contain %q", r)
		}
	}
}

// ── 7. Native label helper module is wired in index.html ────────────

func TestExplorer_D37xNativeLabels_ScriptTagOrdered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	html := getExplorerAsset(t, srv, d37xIndexHTML)

	helperTag := `<script src="/explorer/assets/js/graph/graph-platform/graph-native-labels.js"></script>`
	engineTag := `<script src="/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js"></script>`
	contextTag := `<script src="/explorer/assets/js/graph/context/context-cytoscape-renderer.js"></script>`

	helperIdx := strings.Index(html, helperTag)
	engineIdx := strings.Index(html, engineTag)
	contextIdx := strings.Index(html, contextTag)

	if helperIdx < 0 {
		t.Fatal("D37x-engine-node-geometry-contract: native labels script tag must be present in index.html")
	}
	if engineIdx < 0 || contextIdx < 0 {
		t.Fatal("D37x-engine-node-geometry-contract: engine + context script tags must be present in index.html")
	}
	if !(helperIdx < engineIdx) {
		t.Errorf("D37x-engine-node-geometry-contract: native labels script must load BEFORE the engine module")
	}
	if !(helperIdx < contextIdx) {
		t.Errorf("D37x-engine-node-geometry-contract: native labels script must load BEFORE the Context renderer")
	}
}

// ── 8. Helper truncation semantics are testable (deterministic) ─────

func TestExplorer_D37xNativeLabels_DeterministicTruncationShape(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xNativeLabelsModule)

	// Empty / null input branch.
	if !strings.Contains(js, "if (!raw) return ''") {
		t.Errorf("D37x-engine-node-geometry-contract: helper must return '' for empty/null input")
	}
	// Invalid dimensions branch.
	if !strings.Contains(js, "if (!_isFiniteNumber(width) || !_isFiniteNumber(height)) return ''") {
		t.Errorf("D37x-engine-node-geometry-contract: helper must return '' for non-finite width/height")
	}
	if !strings.Contains(js, "if (width <= 0 || height <= 0) return ''") {
		t.Errorf("D37x-engine-node-geometry-contract: helper must return '' for non-positive width/height")
	}
	// Available-box guard.
	if !strings.Contains(js, "if (availW <= 0 || availH <= 0) return ''") {
		t.Errorf("D37x-engine-node-geometry-contract: helper must return '' when padding eats the available label box")
	}
	// Capacity math is deterministic — uses floor over availW/glyphWidthPx.
	if !strings.Contains(js, "var approxCharsPerLine = Math.max(1, Math.floor(availW / glyphWidthPx))") {
		t.Errorf("D37x-engine-node-geometry-contract: helper must use deterministic capacity math")
	}
	// Truncation path uses the ellipsis glyph.
	if !strings.Contains(js, "return truncated + ellipsis;") {
		t.Errorf("D37x-engine-node-geometry-contract: helper must append ellipsis on truncation")
	}
	// Short-input fast path.
	if !strings.Contains(js, "if (raw.length <= capacity) return raw;") {
		t.Errorf("D37x-engine-node-geometry-contract: helper must return the raw label verbatim when it fits the capacity")
	}
}

// ── 9. Context no longer renders raw data(id) as the visible label ──

func TestExplorer_D37xContextLabels_NoRawIdLabel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xContextRenderer)

	// Locate the visibility override and assert label binds to data(label).
	oIdx := strings.Index(js, "function _buildContextRawNodeVisibilityOverride()")
	if oIdx < 0 {
		t.Fatal("D37x-engine-node-geometry-contract: _buildContextRawNodeVisibilityOverride must exist")
	}
	oTail := js[oIdx:]
	oEnd := strings.Index(oTail[1:], "\n  function ")
	if oEnd < 0 {
		oEnd = len(oTail) - 1
	}
	oBody := oTail[:oEnd+1]

	if strings.Contains(oBody, "'label':              'data(id)'") {
		t.Errorf("D37x-engine-node-geometry-contract: Context override must NOT bind cy label to data(id) (raw technical identifier)")
	}
	if !strings.Contains(oBody, "'label':              'data(label)'") {
		t.Errorf("D37x-engine-node-geometry-contract: Context override must bind cy label to data(label) (display-safe pre-truncated value)")
	}
}

// ── 10. Context node builder calls the native-labels helper ─────────

func TestExplorer_D37xContextLabels_NodeBuilderUsesHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xContextRenderer)

	if !strings.Contains(js, "MIDASExplorerGraph.graphNativeLabels") {
		t.Errorf("D37x-engine-node-geometry-contract: Context renderer must reference MIDASExplorerGraph.graphNativeLabels")
	}
	if !strings.Contains(js, "graphNativeLabels.makeNativeNodeLabel") {
		t.Errorf("D37x-engine-node-geometry-contract: Context renderer must reference the makeNativeNodeLabel helper")
	}
	// data.label is set per node from the helper output.
	bIdx := strings.Index(js, "function _buildContextCytoscapeNodes(cards, stage)")
	if bIdx < 0 {
		t.Fatal("D37x-engine-node-geometry-contract: _buildContextCytoscapeNodes must exist")
	}
	bTail := js[bIdx:]
	bEnd := strings.Index(bTail[1:], "\n  function ")
	if bEnd < 0 {
		bEnd = len(bTail) - 1
	}
	bBody := bTail[:bEnd+1]

	if !strings.Contains(bBody, "label:       displayLabel") {
		t.Errorf("D37x-engine-node-geometry-contract: _buildContextCytoscapeNodes must assign data.label from the helper-produced displayLabel")
	}
	// Stable technical id remains.
	if !strings.Contains(bBody, "id:          String(c.id)") {
		t.Errorf("D37x-engine-node-geometry-contract: _buildContextCytoscapeNodes must keep data.id as the stable technical String(c.id)")
	}
	if !strings.Contains(bBody, "technicalId: String(c.id)") {
		t.Errorf("D37x-engine-node-geometry-contract: _buildContextCytoscapeNodes must expose data.technicalId alongside data.id")
	}
	// Width/height still required.
	if !strings.Contains(bBody, "width:       w,") || !strings.Contains(bBody, "height:      h,") {
		t.Errorf("D37x-engine-node-geometry-contract: _buildContextCytoscapeNodes must keep data.width / data.height")
	}
}

// ── 11. Engine data propagation includes label/technicalId/fullLabel ─

func TestExplorer_D37xContextLabels_EngineDataPropagation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xContextRenderer)

	bIdx := strings.Index(js, "function _buildContextEngineData(cards, connectors, stage)")
	if bIdx < 0 {
		t.Fatal("D37x-engine-node-geometry-contract: _buildContextEngineData must exist")
	}
	bTail := js[bIdx:]
	bEnd := strings.Index(bTail[1:], "\n  function ")
	if bEnd < 0 {
		bEnd = len(bTail) - 1
	}
	bBody := bTail[:bEnd+1]

	required := []string{
		"label:       cn.data.label,",
		"technicalId: cn.data.technicalId,",
		"fullLabel:   cn.data.fullLabel,",
		"width:       cn.data.width,",
		"height:      cn.data.height,",
	}
	for _, r := range required {
		if !strings.Contains(bBody, r) {
			t.Errorf("D37x-engine-node-geometry-contract: _buildContextEngineData must propagate %q", r)
		}
	}
}

// ── 12. data.label remains optional for overlay renderers ──────────

func TestExplorer_D37xEngineGeometry_LabelOptionalForOverlayRenderers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xEngineModule)

	// Engine's pre-mount scan must validate width/height but MUST NOT
	// require data.label. A negative regression check: no scan rule
	// equivalent to "data.label === '' → emit".
	if !strings.Contains(js, "var _nodeGeometryViolated = false;") {
		t.Fatal("D37x-engine-node-geometry-contract: engine must scan supplied data for geometry violations")
	}
	scanIdx := strings.Index(js, "var _nodeGeometryViolated = false;")
	scanTail := js[scanIdx:]
	scanEnd := strings.Index(scanTail, "\n    }\n")
	if scanEnd < 0 {
		t.Fatalf("D37x-engine-node-geometry-contract: pre-mount scan block must be well-formed")
	}
	scanBody := scanTail[:scanEnd+1]

	if !strings.Contains(scanBody, "_snData.width") || !strings.Contains(scanBody, "_snData.height") {
		t.Errorf("D37x-engine-node-geometry-contract: pre-mount scan must check width and height")
	}
	if strings.Contains(scanBody, "_snData.label") {
		t.Errorf("D37x-engine-node-geometry-contract: pre-mount scan must NOT require data.label (labels are optional; overlay renderers leave them absent)")
	}
}

// ── 13. D37u passive resize policy regression ──────────────────────

func TestExplorer_D37xEngineGeometry_PassiveResizeStillPreservesCamera(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37xEngineModule)

	// _onContainerResize must still call cy.resize() and emit the
	// camera-preserved diagnostic; must NOT have regrown a refit path.
	rIdx := strings.Index(js, "function _onContainerResize()")
	if rIdx < 0 {
		t.Fatal("D37x-engine-node-geometry-contract: _onContainerResize must still exist (D37u)")
	}
	rTail := js[rIdx:]
	rEnd := strings.Index(rTail, "\n    }\n")
	if rEnd < 0 {
		t.Fatalf("D37x-engine-node-geometry-contract: _onContainerResize body must be well-formed")
	}
	rBody := rTail[:rEnd+1]

	if !strings.Contains(rBody, "cy.resize()") {
		t.Errorf("D37x-engine-node-geometry-contract: passive resize must still call cy.resize()")
	}
	if !strings.Contains(rBody, "_emitLifecycleDiagnostic(DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE)") {
		t.Errorf("D37x-engine-node-geometry-contract: passive resize must still emit DIAG_ENGINE_CAMERA_PRESERVED_ON_RESIZE")
	}
	forbidden := []string{
		"_runFitPipeline",
		"_fitToUsableRect",
		"_fitWithSafeArea",
		"cy.viewport(",
		"cy.fit(",
		"setTimeout",
	}
	for _, f := range forbidden {
		if strings.Contains(rBody, f) {
			t.Errorf("D37x-engine-node-geometry-contract: passive resize path must NOT contain %q (D37u policy regression)", f)
		}
	}
}

// ── 14. Authority is unaffected (no data.label requirement) ─────────

func TestExplorer_D37xEngineGeometry_AuthorityUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	authJS := getExplorerAsset(t, srv, d37xAuthorityPoc)

	// Authority must still set its cy node `label: ''` for the
	// html-card theme (its visible cards are HTML overlay; cy is
	// visually inert).
	if !strings.Contains(authJS, "'label': ''") {
		t.Errorf("D37x-engine-node-geometry-contract: Authority's cy style must still declare label: '' (overlay renderers leave native label blank)")
	}
	// Authority's _refitWithSafeArea path is untouched.
	if !strings.Contains(authJS, "function _refitWithSafeArea()") {
		t.Errorf("D37x-engine-node-geometry-contract: Authority _refitWithSafeArea must remain")
	}
}
