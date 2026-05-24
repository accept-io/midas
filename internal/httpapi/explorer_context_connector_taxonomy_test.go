package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

const (
	d37pContextConnectorModelAsset = "/explorer/assets/js/graph/context/context-connector-model.js"
	d37pContextConnectorPainter    = "/explorer/assets/js/graph/context/context-connector-painter.js"
	d37pContextRendererAsset       = "/explorer/assets/js/graph/context/context-cytoscape-renderer.js"
	d37pContextRendererCSS         = "/explorer/assets/css/context-cytoscape-renderer.css"
	d37pGovernanceMapCSS           = "/explorer/assets/css/governance-map.css"
	d37pGovernanceLayersAsset      = "/explorer/assets/js/governance-map/layers.js"
)

func d37pContextConnectorServer() *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
}

func d37pSliceUntil(t *testing.T, src string, marker string, terminator string) string {
	t.Helper()
	idx := strings.Index(src, marker)
	if idx < 0 {
		t.Fatalf("D37p Context connector taxonomy: marker %q not found", marker)
	}
	tail := src[idx:]
	endRel := strings.Index(tail, terminator)
	if endRel < 0 {
		t.Fatalf("D37p Context connector taxonomy: marker %q object terminator not found", marker)
	}
	return tail[:endRel]
}

func TestExplorer_ContextConnectorTaxonomy_ModelDefinesSemanticContract(t *testing.T) {
	srv := d37pContextConnectorServer()
	model := getExplorerAsset(t, srv, d37pContextConnectorModelAsset)

	for _, want := range []string{
		"var CONNECTOR_FAMILIES = Object.freeze([",
		"var CONNECTOR_TYPES = Object.freeze([",
		"connectorType:  taxonomy.connectorType,",
		"family:         taxonomy.family,",
		"direction:      _directionForTemplate(tpl),",
		"arrowPolicy:    _arrowPolicyForTemplate(tpl),",
		"labelPolicy:    LABEL_POLICY_HOVER,",
		"weightPolicy:   WEIGHT_POLICY_NONE,",
		"statePolicy:    taxonomy.family === 'drift_risk_exception' ? 'risk_state' : 'active',",
		"label:          taxonomy.label,",
		"hoverLabel:     taxonomy.label,",
		"hoverSummary:   _hoverSummary(edge, taxonomy),",
		"accessibilityLabel: _accessibilityLabel(edge, taxonomy),",
		"fallback:       taxonomy.fallback === true,",
		"CONNECTOR_TYPES:               CONNECTOR_TYPES,",
		"CONNECTOR_FAMILIES:            CONNECTOR_FAMILIES,",
	} {
		if !strings.Contains(model, want) {
			t.Errorf("D37p Context connector taxonomy: model must expose semantic connector field/constant %q", want)
		}
	}

	for _, family := range []string{
		"structural",
		"dependency",
		"runtime_operational",
		"evidence",
		"drift_risk_exception",
		"semantic_contextual",
		"authority_governance",
	} {
		if !strings.Contains(model, "'"+family+"',") {
			t.Errorf("D37p Context connector taxonomy: connector family %q must be in the controlled family vocabulary", family)
		}
	}
}

func TestExplorer_ContextConnectorTaxonomy_TypesMapToFamilies(t *testing.T) {
	srv := d37pContextConnectorServer()
	model := getExplorerAsset(t, srv, d37pContextConnectorModelAsset)
	taxonomy := d37pSliceUntil(t, model, "var TAXONOMY_BY_EDGE_KIND = Object.freeze({", "\n  });")

	cases := []struct {
		edgeKind      string
		connectorType string
		family        string
		label         string
	}{
		{"has_capability", "service_contains_capability", "structural", "contains"},
		{"has_process", "service_supports_process", "dependency", "supports"},
		{"has_surface", "process_uses_surface", "dependency", "uses surface"},
		{"relates_to", "service_depends_on_service", "dependency", "depends on"},
		{"bound_to", "ai_binding_applies_to_scope", "dependency", "applies to"},
		{"system_of", "ai_binding_uses_system", "dependency", "uses system"},
		{"summarises", "authority_summary_informs_service", "authority_governance", "informs"},
	}
	for _, tc := range cases {
		re := regexp.MustCompile(regexp.QuoteMeta(tc.edgeKind+": {") +
			`[\s\S]*?connectorType:\s*'` + regexp.QuoteMeta(tc.connectorType) + `'` +
			`[\s\S]*?family:\s*'` + regexp.QuoteMeta(tc.family) + `'` +
			`[\s\S]*?label:\s*'` + regexp.QuoteMeta(tc.label) + `'`)
		if !re.MatchString(taxonomy) {
			t.Errorf("D37p Context connector taxonomy: %s must map to %s/%s/%q", tc.edgeKind, tc.connectorType, tc.family, tc.label)
		}
	}

	for _, want := range []string{
		"connectorType: isGap ? 'coverage_reports_gap_for_service' : 'coverage_reports_on_service'",
		"family: isGap ? 'drift_risk_exception' : 'evidence'",
		"label: isGap ? 'coverage gap' : 'reports coverage'",
	} {
		if !strings.Contains(model, want) {
			t.Errorf("D37p Context connector taxonomy: reports_coverage resolver must include %q", want)
		}
	}
}

func TestExplorer_ContextConnectorTaxonomy_ControlledPoliciesAndNoGenericTypes(t *testing.T) {
	srv := d37pContextConnectorServer()
	model := getExplorerAsset(t, srv, d37pContextConnectorModelAsset)

	for _, want := range []string{
		"var DIRECTION_SOURCE_TO_TARGET = 'source_to_target';",
		"var DIRECTION_ASSOCIATIVE      = 'associative';",
		"var ARROW_DIRECTED             = 'directed';",
		"var ARROW_NONE                 = 'none';",
		"var LABEL_POLICY_HOVER         = 'hover';",
		"var WEIGHT_POLICY_NONE         = 'none';",
		"return tpl && tpl.dir === 'undirected' ? DIRECTION_ASSOCIATIVE : DIRECTION_SOURCE_TO_TARGET;",
		"return tpl && tpl.dir === 'undirected' ? ARROW_NONE : ARROW_DIRECTED;",
	} {
		if !strings.Contains(model, want) {
			t.Errorf("D37p Context connector taxonomy: controlled policy contract must include %q", want)
		}
	}

	typesBlock := d37pSliceUntil(t, model, "var CONNECTOR_TYPES = Object.freeze([", "\n  ]);")
	for _, banned := range []string{
		"'node_",
		"'context_",
		"node_relates_to_activity",
		"node_has_drift_signal",
		"context_depends_on_context",
	} {
		if strings.Contains(typesBlock, banned) {
			t.Errorf("D37p Context connector taxonomy: generic fallback connector type %q must not appear in the controlled first-pass type list", banned)
		}
	}
}

func TestExplorer_ContextConnectorTaxonomy_CytoscapeCarriesMetadata(t *testing.T) {
	srv := d37pContextConnectorServer()
	renderer := getExplorerAsset(t, srv, d37pContextRendererAsset)

	for _, want := range []string{
		"connectorType:      ce.data.connectorType,",
		"family:             ce.data.family,",
		"label:              ce.data.label,",
		"direction:          ce.data.direction,",
		"arrowPolicy:        ce.data.arrowPolicy,",
		"labelPolicy:        ce.data.labelPolicy,",
		"weightPolicy:       ce.data.weightPolicy,",
		"accessibilityLabel: ce.data.accessibilityLabel,",
		"hoverSummary:       ce.data.hoverSummary,",
		"if (c.connectorType) classes.push('context-edge-type-' + String(c.connectorType));",
		"if (c.family) classes.push('context-edge-family-' + String(c.family));",
		"if (c.arrowPolicy) classes.push('context-edge-arrow-' + String(c.arrowPolicy));",
		"if (c.labelPolicy) classes.push('context-edge-label-' + String(c.labelPolicy));",
		"if (c.weightPolicy) classes.push('context-edge-weight-' + String(c.weightPolicy));",
		"accessibilityLabel: String(c.accessibilityLabel || c.ariaText || ''),",
	} {
		if !strings.Contains(renderer, want) {
			t.Errorf("D37p Context connector taxonomy: Cytoscape edge mapper must carry metadata %q", want)
		}
	}
}

func TestExplorer_ContextConnectorTaxonomy_VisualGrammar(t *testing.T) {
	srv := d37pContextConnectorServer()
	renderer := getExplorerAsset(t, srv, d37pContextRendererAsset)
	css := getExplorerAsset(t, srv, d37pContextRendererCSS)
	governanceCSS := getExplorerAsset(t, srv, d37pGovernanceMapCSS)
	style := sliceStyleBuilderBody(t, renderer)

	for _, want := range []string{
		"selector: 'edge.context-edge-arrow-directed'",
		"'target-arrow-shape':        'triangle',",
		"'target-arrow-fill':         'filled',",
		"'arrow-scale':               1.3,",
		"'target-distance-from-node': 16,",
		"selector: 'edge.context-edge-arrow-none'",
		"'label':                '',",
		"selector: 'edge.context-edge-family-structural.context-edge-arrow-directed'",
		"selector: 'edge.context-edge-family-dependency.context-edge-arrow-directed'",
		"selector: 'edge.context-edge-family-runtime_operational.context-edge-arrow-directed'",
		"selector: 'edge.context-edge-family-evidence.context-edge-arrow-directed'",
		"selector: 'edge.context-edge-family-drift_risk_exception.context-edge-arrow-directed'",
		"selector: 'edge.context-edge-family-authority_governance.context-edge-arrow-directed'",
	} {
		if !strings.Contains(style, want) {
			t.Errorf("D37p Context connector taxonomy: visual grammar must include %q", want)
		}
	}

	if strings.Contains(style, "'label':                'data(label)'") ||
		strings.Contains(style, "'label': 'data(label)'") {
		t.Errorf("D37p Context connector taxonomy: edge labels must remain hover/config metadata, not persistent Cytoscape labels")
	}
	for _, selector := range []string{
		"edge.context-edge-visual-service",
		"edge.context-edge-visual-ai_binding",
		"edge.context-edge-visual-authority",
		"edge.context-edge-visual-evidence",
		"edge.context-edge-visual-gap",
	} {
		re := regexp.MustCompile(regexp.QuoteMeta(selector) + `[\s\S]*?'width':\s+1\.4,`)
		if !re.MatchString(style) {
			t.Errorf("D37p Context connector taxonomy: %s must use uniform non-decorative width 1.4", selector)
		}
	}
	if !regexp.MustCompile(`edge\.context-edge-visual-authority[\s\S]*?'line-dash-pattern':\s*\[6,\s*4\]`).MatchString(style) {
		t.Errorf("D37p Context connector taxonomy: authority/governance connector style must keep documented dashed [6,4] semantics")
	}
	if !regexp.MustCompile(`edge\.context-edge-visual-gap[\s\S]*?'line-dash-pattern':\s*\[5,\s*5\]`).MatchString(style) {
		t.Errorf("D37p Context connector taxonomy: drift/risk connector style must keep documented dashed [5,5] semantics")
	}

	if !regexp.MustCompile(`\.context-connector\s*\{[\s\S]*?stroke-width:\s*1\.4`).MatchString(css) {
		t.Errorf("D37p Context connector taxonomy: base SVG connector CSS must set uniform stroke-width 1.4")
	}
	for _, selector := range []string{
		"context-connector--ai_binding",
		"context-connector--authority",
		"context-connector--gap",
	} {
		re := regexp.MustCompile(regexp.QuoteMeta(selector) + `[\s\S]*?stroke-width:\s*1\.[568]`)
		if re.MatchString(css) {
			t.Errorf("D37p Context connector taxonomy: %s must not override uniform CSS stroke-width decoratively", selector)
		}
	}
	for _, selector := range []string{
		"connector-service",
		"connector-ai-binding",
		"connector-authority",
		"connector-evidence",
		"connector-gap",
	} {
		re := regexp.MustCompile(regexp.QuoteMeta(selector) + `[\s\S]*?stroke-width:\s*1\.4`)
		if !re.MatchString(governanceCSS) {
			t.Errorf("D37p Context connector taxonomy: %s legacy SVG class must use uniform stroke-width 1.4", selector)
		}
	}
}

func TestExplorer_ContextConnectorTaxonomy_VisibleLegendAndSvgPainterUseTaxonomy(t *testing.T) {
	srv := d37pContextConnectorServer()
	painter := getExplorerAsset(t, srv, d37pContextConnectorPainter)
	layers := getExplorerAsset(t, srv, d37pGovernanceLayersAsset)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	for _, want := range []string{
		"function _ensureArrowMarker(svgEl)",
		"marker.setAttribute('id', 'context-connector-arrow');",
		"if (c.connectorType) classes.push(CONNECTOR_CLASS + '-type-' + _str(c.connectorType));",
		"if (c.family) classes.push(CONNECTOR_CLASS + '-family-' + _str(c.family));",
		"if (c.arrowPolicy === 'directed') {",
		"line.setAttribute('marker-end', 'url(#context-connector-arrow)');",
		"line.setAttribute('data-connector-label', _str(c.label));",
		"line.setAttribute('data-hover-summary', _str(c.hoverSummary));",
		"line.setAttribute('aria-label', _str(c.accessibilityLabel || c.ariaText));",
	} {
		if !strings.Contains(painter, want) {
			t.Errorf("D37p Context connector taxonomy: SVG painter must expose visible taxonomy affordance %q", want)
		}
	}

	for _, want := range []string{
		"Contains",
		"Depends / supports",
		"Governance",
		"Evidence signal",
		"Drift / gap",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("D37p Context connector taxonomy: canvas legend must expose taxonomy label %q", want)
		}
	}
	for _, old := range []string{
		">Service relationship<",
		">AI binding<",
		">Coverage gap<",
	} {
		if strings.Contains(html, old) {
			t.Errorf("D37p Context connector taxonomy: old visible legend label must not remain: %q", old)
		}
	}
	for _, want := range []string{
		"label: 'Depends / supports'",
		"label: 'Evidence signal'",
		"label: 'Drift / gap'",
		"label: 'Contains'",
	} {
		if !strings.Contains(layers, want) {
			t.Errorf("D37p Context connector taxonomy: layer connector labels must use taxonomy label %q", want)
		}
	}
}

func TestExplorer_ContextConnectorTaxonomy_DisallowedSurfacesUntouchedByTaxonomy(t *testing.T) {
	srv := d37pContextConnectorServer()
	disallowedAssets := []string{
		"/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js",
		"/explorer/assets/js/graph/graph-platform/graph-inspector-platform.js",
		"/explorer/assets/js/graph/graph-platform/graph-stage.js",
		"/explorer/assets/js/graph/graph-platform/graph-footprint-policy.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/authority/authority-graph-connectors.js",
	}
	for _, asset := range disallowedAssets {
		body := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"service_contains_capability",
			"context-edge-family-",
			"context-edge-arrow-",
			"coverage_reports_gap_for_service",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D37p Context connector taxonomy: disallowed/reference asset %s must not carry Context taxonomy implementation marker %q", asset, banned)
			}
		}
	}

	renderer := getExplorerAsset(t, srv, d37pContextRendererAsset)
	for _, banned := range []string{
		"relationship browser",
		"openRelationshipBrowser",
		"zoomControl",
		"fitControl",
		"reframeControl",
		"cameraControl",
		"layoutControl",
	} {
		if strings.Contains(renderer, banned) {
			t.Errorf("D37p Context connector taxonomy: connector implementation must not introduce graph controls or relationship-browser surface %q", banned)
		}
	}
}
