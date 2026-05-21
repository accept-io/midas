package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37o-impl-1 — Context Card, Connector, and Layout Model Foundation
//
// Asset-text and structural tests pinning the three pure-data, renderer-
// independent model modules introduced by D37o-impl-1:
//
//   /explorer/assets/js/graph/context/context-card-model.js
//   /explorer/assets/js/graph/context/context-connector-model.js
//   /explorer/assets/js/graph/context/context-layout-model.js
//
// The contracts are locked in
//   docs/design/D37o-design-1-context-strategic-renderer-architecture.md
//
// Tests cover:
//   - Module presence (asset serving)
//   - Public namespace shape (MIDASExplorerGraph.contextModels.{card,connector,layout})
//   - Per-kind / per-edge-kind coverage of the locked vocabularies
//   - Visual-class taxonomy and base-mapping pins
//   - reports_coverage gap/authority working-decision predicate pin
//   - Layout band order, governance column, split columns, overflow sentinel
//   - Renderer-purity meta-test (no DOM / no graph-engine APIs in module source)
//   - Naming-fossilisation pins (no durable temporary renderer names)
//   - Foundation preservation (legacy context renderer / drawer / tray / spike / authority / viewport untouched)
//   - No <script>/<link> additions for the model modules in index.html
//   - No renderer registration in the model tranche

const (
	d37oImpl1CardAsset      = "/explorer/assets/js/graph/context/context-card-model.js"
	d37oImpl1ConnectorAsset = "/explorer/assets/js/graph/context/context-connector-model.js"
	d37oImpl1LayoutAsset    = "/explorer/assets/js/graph/context/context-layout-model.js"

	d37oImpl1LegacyAdapter   = "/explorer/assets/js/graph/context/context-graph-adapter.js"
	d37oImpl1LegacyView      = "/explorer/assets/js/graph/context/context-graph-view.js"
	d37oImpl1LegacyInspector = "/explorer/assets/js/graph/context/context-graph-inspector.js"
	d37oImpl1LegacyTray      = "/explorer/assets/js/graph/context/context-evidence-tray.js"
	d37oImpl1LegacySpike     = "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js"

	d37oImpl1AuthorityPoc = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37oImpl1ViewportHost = "/explorer/assets/js/graph/graph-viewport.js"
)

// ── Module presence ──────────────────────────────────────────────────

func TestExplorer_D37oImpl1_CardModelAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1CardAsset)
	if len(js) == 0 {
		t.Fatal("D37o-impl-1: context-card-model.js must be served (length 0)")
	}
}

func TestExplorer_D37oImpl1_ConnectorModelAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1ConnectorAsset)
	if len(js) == 0 {
		t.Fatal("D37o-impl-1: context-connector-model.js must be served (length 0)")
	}
}

func TestExplorer_D37oImpl1_LayoutModelAssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)
	if len(js) == 0 {
		t.Fatal("D37o-impl-1: context-layout-model.js must be served (length 0)")
	}
}

// ── Public namespace ─────────────────────────────────────────────────

// TestExplorer_D37oImpl1_GroupedNamespaceAttached pins that each model
// module attaches to the shared MIDASExplorerGraph.contextModels.{card|connector|layout}
// namespace per the D37o-design-1 §16 preferred shape.
func TestExplorer_D37oImpl1_GroupedNamespaceAttached(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	card := getExplorerAsset(t, srv, d37oImpl1CardAsset)
	if !strings.Contains(card, "window.MIDASExplorerGraph.contextModels.card = {") {
		t.Errorf("D37o-impl-1: context-card-model must attach to MIDASExplorerGraph.contextModels.card")
	}

	conn := getExplorerAsset(t, srv, d37oImpl1ConnectorAsset)
	if !strings.Contains(conn, "window.MIDASExplorerGraph.contextModels.connector = {") {
		t.Errorf("D37o-impl-1: context-connector-model must attach to MIDASExplorerGraph.contextModels.connector")
	}

	layout := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)
	if !strings.Contains(layout, "window.MIDASExplorerGraph.contextModels.layout = {") {
		t.Errorf("D37o-impl-1: context-layout-model must attach to MIDASExplorerGraph.contextModels.layout")
	}
}

// ── Card model coverage ──────────────────────────────────────────────

// TestExplorer_D37oImpl1_CardModelCoversNineKinds pins that the card
// model recognises all 9 Context node kinds.
func TestExplorer_D37oImpl1_CardModelCoversNineKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1CardAsset)

	for _, kind := range []string{
		"business_service",
		"related_business_service",
		"capability",
		"process",
		"decision_surface",
		"ai_system",
		"ai_system_binding",
		"authority_summary",
		"coverage",
	} {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D37o-impl-1: card model must reference node kind %q", kind)
		}
	}
}

// TestExplorer_D37oImpl1_CardModelExposesBuilders pins that the public
// surface includes both the per-node builder and the projection-wide
// builder.
func TestExplorer_D37oImpl1_CardModelExposesBuilders(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1CardAsset)

	for _, sym := range []string{
		"buildCardsFromProjection:",
		"buildCardForNode:",
		"NODE_KINDS:",
		"BADGE_CLASSES:",
		"ACTION_KINDS:",
		"ROLES:",
	} {
		if !strings.Contains(js, sym) {
			t.Errorf("D37o-impl-1: card model must export %q", sym)
		}
	}
}

// TestExplorer_D37oImpl1_CardModelBadgeVocabulary pins the badge
// vocabulary required by D37o-design-1 §4.2.
func TestExplorer_D37oImpl1_CardModelBadgeVocabulary(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1CardAsset)

	for _, badge := range []string{
		"fmp-default",
		"fmp-inherited",
		"fmp-override",
		"ai-bind",
		"ai-warn",
		"coverage-ok",
		"coverage-warn",
	} {
		if !strings.Contains(js, "'"+badge+"'") {
			t.Errorf("D37o-impl-1: card model must include badge class %q", badge)
		}
	}
}

// TestExplorer_D37oImpl1_CardModelActionVocabulary pins the action kinds
// the card model emits (reframe + view-record).
func TestExplorer_D37oImpl1_CardModelActionVocabulary(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1CardAsset)

	for _, act := range []string{
		"reframe-around-this",
		"view-business-service-record",
		"view-capability-record",
	} {
		if !strings.Contains(js, "'"+act+"'") {
			t.Errorf("D37o-impl-1: card model must reference action kind %q", act)
		}
	}
}

// TestExplorer_D37oImpl1_CardModelHasFmpStateBranching pins that the
// FMP three-state hierarchy (default / inherited / override) is
// represented as conditional badging in the card model.
func TestExplorer_D37oImpl1_CardModelHasFmpStateBranching(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1CardAsset)

	for _, want := range []string{
		"fmp-default",
		"fmp-inherited",
		"fmp-override",
		"fail_mode_policy_id",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-1: card model must reference FMP token %q", want)
		}
	}
}

// TestExplorer_D37oImpl1_CardModelHasCoverageGapPredicate pins that
// the card model uses the canonical Coverage gap predicate field
// (`surfaces_with_no_ai_binding`) for the coverage-warn vs coverage-ok
// badge decision. This is the same field the connector model uses
// for the reports_coverage gap/authority working decision.
func TestExplorer_D37oImpl1_CardModelHasCoverageGapPredicate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1CardAsset)

	if !strings.Contains(js, "surfaces_with_no_ai_binding") {
		t.Errorf("D37o-impl-1: card model must consult `surfaces_with_no_ai_binding` for the coverage badge predicate")
	}
}

// ── Connector model coverage ─────────────────────────────────────────

// TestExplorer_D37oImpl1_ConnectorModelCoversEightKinds pins all 8
// Context edge kinds.
func TestExplorer_D37oImpl1_ConnectorModelCoversEightKinds(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1ConnectorAsset)

	for _, kind := range []string{
		"relates_to",
		"has_capability",
		"has_process",
		"has_surface",
		"bound_to",
		"system_of",
		"summarises",
		"reports_coverage",
	} {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D37o-impl-1: connector model must reference edge kind %q", kind)
		}
	}
}

// TestExplorer_D37oImpl1_ConnectorModelFiveVisualClasses pins all 5
// visual connector classes (including the reserved `evidence` class).
func TestExplorer_D37oImpl1_ConnectorModelFiveVisualClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1ConnectorAsset)

	for _, cls := range []string{
		"'service'",
		"'ai_binding'",
		"'authority'",
		"'evidence'",
		"'gap'",
	} {
		if !strings.Contains(js, cls) {
			t.Errorf("D37o-impl-1: connector model must include visual class %s", cls)
		}
	}
}

// TestExplorer_D37oImpl1_ConnectorModelBaseMapping pins the locked
// edge-kind → (semantic, visual, stroke, dash, directionality) mapping
// for the seven non-conditional edge kinds, per D37o-design-1 §5.3.
func TestExplorer_D37oImpl1_ConnectorModelBaseMapping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1ConnectorAsset)

	// Bound to the BASE_MAPPING block so we don't false-match on prose.
	start := strings.Index(js, "var BASE_MAPPING = Object.freeze({")
	if start < 0 {
		t.Fatal("D37o-impl-1: BASE_MAPPING declaration missing")
	}
	end := strings.Index(js[start:], "});")
	if end < 0 {
		t.Fatal("D37o-impl-1: cannot bound BASE_MAPPING block")
	}
	block := js[start : start+end]

	pairs := []struct {
		kind string
		want []string
	}{
		{"has_capability", []string{"'structural'", "'service'", "'neutral'", "DASH_SOLID", "'directed'"}},
		{"has_process", []string{"'structural'", "'service'", "'neutral'", "DASH_SOLID", "'directed'"}},
		{"has_surface", []string{"'structural'", "'service'", "'neutral'", "DASH_SOLID", "'directed'"}},
		{"relates_to", []string{"'structural'", "'service'", "'neutral'", "DASH_SOLID", "'undirected'"}},
		{"bound_to", []string{"'functional'", "'ai_binding'", "'ai'", "DASH_SOLID", "'directed'"}},
		{"system_of", []string{"'functional'", "'ai_binding'", "'ai'", "DASH_SOLID", "'directed'"}},
		{"summarises", []string{"'synthesis'", "'authority'", "'authority'", "DASH_6_4", "'directed'"}},
	}
	for _, p := range pairs {
		rowStart := strings.Index(block, p.kind+":")
		if rowStart < 0 {
			t.Errorf("D37o-impl-1: BASE_MAPPING row missing for %q", p.kind)
			continue
		}
		rowEnd := strings.Index(block[rowStart:], "},")
		if rowEnd < 0 {
			t.Errorf("D37o-impl-1: cannot bound BASE_MAPPING row for %q", p.kind)
			continue
		}
		row := block[rowStart : rowStart+rowEnd]
		for _, want := range p.want {
			if !strings.Contains(row, want) {
				t.Errorf("D37o-impl-1: BASE_MAPPING[%q] must contain %q; row:\n%s", p.kind, want, row)
			}
		}
	}
}

// TestExplorer_D37oImpl1_ReportsCoverageWorkingDecision pins the
// D-CONN-1 working decision: reports_coverage defaults to the
// `authority` visual class; it is promoted to `gap` when the source
// Coverage node carries surfaces_with_no_ai_binding > 0.
//
// The exact source field used by the predicate is locked here in the
// test pin so any future refinement is intentional and visible.
func TestExplorer_D37oImpl1_ReportsCoverageWorkingDecision(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1ConnectorAsset)

	resolveStart := strings.Index(js, "function _resolveReportsCoverage(edge, projection)")
	if resolveStart < 0 {
		t.Fatal("D37o-impl-1: _resolveReportsCoverage helper missing")
	}
	resolveEnd := strings.Index(js[resolveStart:], "\n  }\n")
	if resolveEnd < 0 {
		t.Fatal("D37o-impl-1: cannot bound _resolveReportsCoverage body")
	}
	body := js[resolveStart : resolveStart+resolveEnd]

	for _, want := range []string{
		"surfaces_with_no_ai_binding",
		"'gap'",
		"'authority'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-1: _resolveReportsCoverage must reference %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37oImpl1_ConnectorModelExposesBuilders pins the
// connector model public surface.
func TestExplorer_D37oImpl1_ConnectorModelExposesBuilders(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1ConnectorAsset)

	for _, sym := range []string{
		"buildConnectorsFromProjection:",
		"buildConnectorForEdge:",
		"connectorVisualClassForEdge:",
		"EDGE_KINDS:",
		"VISUAL_CLASSES:",
	} {
		if !strings.Contains(js, sym) {
			t.Errorf("D37o-impl-1: connector model must export %q", sym)
		}
	}
}

// ── Layout model coverage ────────────────────────────────────────────

// TestExplorer_D37oImpl1_LayoutBandOrder pins the strict top→bottom
// layer ordering: related → root → cap-proc → surface → ai.
func TestExplorer_D37oImpl1_LayoutBandOrder(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)

	idx := strings.Index(js, "var LAYER_ORDER = Object.freeze(")
	if idx < 0 {
		t.Fatal("D37o-impl-1: LAYER_ORDER declaration missing")
	}
	end := strings.Index(js[idx:], ");")
	if end < 0 {
		t.Fatal("D37o-impl-1: cannot bound LAYER_ORDER")
	}
	block := js[idx : idx+end]

	// Strict order — find each token's offset and assert monotonic.
	tokens := []string{"'related'", "'root'", "'cap-proc'", "'surface'", "'ai'"}
	last := -1
	for _, tok := range tokens {
		o := strings.Index(block, tok)
		if o < 0 {
			t.Errorf("D37o-impl-1: LAYER_ORDER missing %s", tok)
			continue
		}
		if o < last {
			t.Errorf("D37o-impl-1: LAYER_ORDER order violated at %s", tok)
		}
		last = o
	}
}

// TestExplorer_D37oImpl1_LayoutGovernanceColumn pins that the layout
// model emits a governance column with authority_summary at the top
// and coverage at the bottom.
func TestExplorer_D37oImpl1_LayoutGovernanceColumn(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)

	idx := strings.Index(js, "var GOVERNANCE_SLOTS = Object.freeze({")
	if idx < 0 {
		t.Fatal("D37o-impl-1: GOVERNANCE_SLOTS declaration missing")
	}
	end := strings.Index(js[idx:], "});")
	if end < 0 {
		t.Fatal("D37o-impl-1: cannot bound GOVERNANCE_SLOTS block")
	}
	block := js[idx : idx+end]

	for _, want := range []string{
		"top:    'authority_summary'",
		"bottom: 'coverage'",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37o-impl-1: GOVERNANCE_SLOTS must contain %q; block:\n%s", want, block)
		}
	}
}

// TestExplorer_D37oImpl1_LayoutSplitColumns pins that the layout model
// distinguishes capability (left) from process (right) for the cap-proc
// band's split-column structure.
func TestExplorer_D37oImpl1_LayoutSplitColumns(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)

	if !strings.Contains(js, "function _isLeftColumnKind") {
		t.Errorf("D37o-impl-1: layout model must define _isLeftColumnKind helper")
	}
	if !strings.Contains(js, "function _isRightColumnKind") {
		t.Errorf("D37o-impl-1: layout model must define _isRightColumnKind helper")
	}
	if !strings.Contains(js, "splitColumns") {
		t.Errorf("D37o-impl-1: layout model must populate splitColumns for the cap-proc band")
	}
}

// TestExplorer_D37oImpl1_LayoutOverflowSentinel pins the overflow
// sentinel internal kind name and the cap-with-sentinel policy.
func TestExplorer_D37oImpl1_LayoutOverflowSentinel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)

	for _, want := range []string{
		"_overflow_sentinel",
		"'cap-with-sentinel'",
		"sentinelCards",
		"function _newSentinel",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37o-impl-1: layout model must include overflow-sentinel construct %q", want)
		}
	}
}

// TestExplorer_D37oImpl1_LayoutBandAssignmentRules pins assignBandForCard
// behaviour for the locked kind → band mapping.
func TestExplorer_D37oImpl1_LayoutBandAssignmentRules(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)

	bodyStart := strings.Index(js, "function assignBandForCard(card)")
	if bodyStart < 0 {
		t.Fatal("D37o-impl-1: assignBandForCard helper missing")
	}
	bodyEnd := strings.Index(js[bodyStart:], "\n  }\n")
	if bodyEnd < 0 {
		t.Fatal("D37o-impl-1: cannot bound assignBandForCard body")
	}
	body := js[bodyStart : bodyStart+bodyEnd]

	// Locked rules.
	for _, want := range []string{
		`'root'`,
		`'governance'`,
		`'related_business_service'`,
		`'capability'`,
		`'process'`,
		`'decision_surface'`,
		`'ai_system'`,
		`'ai_system_binding'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-1: assignBandForCard must branch on %s — body:\n%s", want, body)
		}
	}

	// D-LAY-1 working decision: ai_system_binding returns null (edges
	// only in MVP).
	if !strings.Contains(body, "return null; // D-LAY-1") {
		t.Errorf("D37o-impl-1: assignBandForCard must comment D-LAY-1 working decision for ai_system_binding — body:\n%s", body)
	}
}

// TestExplorer_D37oImpl1_LayoutNoPixelCoordinates pins that the layout
// model does NOT emit pixel coordinates as part of the spec contract.
// Renderers compute coordinates downstream from the structural spec.
func TestExplorer_D37oImpl1_LayoutNoPixelCoordinates(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)

	// The spec must not include a pixel-coordinate field; bands /
	// cards must be slot-based. The presence of any `pixelX:` /
	// `pixelY:` / `xPx:` / `yPx:` would indicate a leak.
	for _, banned := range []string{
		"pixelX:",
		"pixelY:",
		"xPx:",
		"yPx:",
		"xCoord:",
		"yCoord:",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37o-impl-1: layout model must NOT emit pixel coordinates — found %q", banned)
		}
	}
}

// TestExplorer_D37oImpl1_LayoutFitHintsSafeAreaIntent pins that the
// layout model communicates safe-area-aware fit intent without itself
// calling the host safe-area API.
func TestExplorer_D37oImpl1_LayoutFitHintsSafeAreaIntent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37oImpl1LayoutAsset)

	if !strings.Contains(js, "'safe-area-aware'") {
		t.Errorf("D37o-impl-1: layout model must emit `paddingMode: 'safe-area-aware'` intent")
	}
	if strings.Contains(js, "getSafeArea") {
		t.Errorf("D37o-impl-1: layout model must NOT call getSafeArea (renderer responsibility)")
	}
}

// ── Renderer-purity meta-tests ───────────────────────────────────────

// TestExplorer_D37oImpl1_RendererPurity pins that none of the three
// model modules reference DOM APIs, graph-engine APIs, drawer setters,
// renderer-owned identifiers, or the dormant overlay-spike module.
//
// This is the load-bearing guard that the model layer is truly pure
// data. The list mirrors the forbidden-token catalogue in
// D37o-design-1 §4.1 / §5.1 / §6.1.
func TestExplorer_D37oImpl1_RendererPurity(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	assets := []string{
		d37oImpl1CardAsset,
		d37oImpl1ConnectorAsset,
		d37oImpl1LayoutAsset,
	}

	forbidden := []string{
		"document.",
		"querySelector",
		"getElementById",
		"createElement",
		"cytoscape",
		"Cytoscape",
		"cy.",
		"viewport.register",
		"activateById",
		"graph-renderer",
		"addNode",
		"addConnector",
		"lensAgnosticConnectorPath",
		"gmap-canvas",
		"gmap-svg",
		"gmap-scene",
		"setName",
		"setFields",
		"setGovernance",
		"setActions",
		"setInlineActions",
		"context-cytoscape-overlay-spike",
	}

	for _, asset := range assets {
		js := getExplorerAsset(t, srv, asset)
		for _, tok := range forbidden {
			if strings.Contains(js, tok) {
				t.Errorf("D37o-impl-1: %s must NOT contain forbidden symbol %q (renderer purity)", asset, tok)
			}
		}
	}
}

// ── Naming fossilisation ─────────────────────────────────────────────

// TestExplorer_D37oImpl1_NoDurableTemporaryNames pins that no durable
// temporary renderer-identity names appear in any of the three model
// modules. The canonical identity is `context`; temporary names like
// `context-v2`, `context-strategic`, `new-context` etc. must never
// appear in CSS, DOM ids, module file names, public API keys, or
// renderer ids (D37o-design-1 §7.7).
func TestExplorer_D37oImpl1_NoDurableTemporaryNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37oImpl1CardAsset,
		d37oImpl1ConnectorAsset,
		d37oImpl1LayoutAsset,
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"context-v2",
			"context-strategic",
			"new-context",
			"context-new",
			"context-next",
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37o-impl-1: %s must NOT contain temporary renderer name %q", asset, banned)
			}
		}
	}
}

// TestExplorer_D37oImpl1_NoRendererRegistration pins that the model
// modules do NOT register a renderer with the viewport host. The
// canonical `context` identity is locked by D37o-design-1 but first
// used operationally in D37o-impl-2 (the renderer skeleton tranche),
// not in this model tranche.
func TestExplorer_D37oImpl1_NoRendererRegistration(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	for _, asset := range []string{
		d37oImpl1CardAsset,
		d37oImpl1ConnectorAsset,
		d37oImpl1LayoutAsset,
	} {
		js := getExplorerAsset(t, srv, asset)
		for _, banned := range []string{
			"viewport.register(",
			"activateById(",
			"renderer.register(",
			"adoptExisting(",
			"register('context'",
			`register("context"`,
		} {
			if strings.Contains(js, banned) {
				t.Errorf("D37o-impl-1: %s must NOT register a renderer (model tranche only) — found %q", asset, banned)
			}
		}
	}
}

// ── Foundation preservation ──────────────────────────────────────────

// TestExplorer_D37oImpl1_LegacyContextRendererUntouched asserts that
// the existing legacy Context renderer continues to register itself
// and is served unchanged.
func TestExplorer_D37oImpl1_LegacyContextRendererUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	view := getExplorerAsset(t, srv, d37oImpl1LegacyView)
	// D37p-clean-1 retired the dead `renderer.register('context', lensImpl)`
	// call; the live Context lens entry point is the `contextView`
	// export consumed via `refreshGovernanceMap`.
	if !strings.Contains(view, "window.MIDASExplorerGraph.contextView") {
		t.Errorf("D37o-impl-1: legacy context-graph-view.js must still expose its production contextView entry points")
	}
	if !strings.Contains(view, "renderContextGraph") {
		t.Errorf("D37o-impl-1: legacy context-graph-view.js must still expose renderContextGraph")
	}
	// D37p-clean-2 retired the dead inspector dispatcher too. The
	// live Context inspector content is driven via the lens-agnostic
	// frame setters on `MIDASExplorerGraph.inspector.set*`; no
	// dispatcher registration remains.
	if strings.Contains(view, "MIDASExplorerGraph.inspector.register('context', inspectorImpl)") {
		t.Errorf("D37p-clean-2: dead inspector.register('context', inspectorImpl) call must be removed from context-graph-view.js")
	}

	// Adapter, inspector, tray, spike all continue to be served.
	for _, asset := range []string{
		d37oImpl1LegacyAdapter,
		d37oImpl1LegacyInspector,
		d37oImpl1LegacyTray,
		d37oImpl1LegacySpike,
	} {
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			t.Errorf("D37o-impl-1: legacy asset %s must still be served", asset)
		}
	}
}

// TestExplorer_D37oImpl1_AuthorityAndViewportUntouched asserts that
// the Authority production renderer and the GraphViewport host both
// continue to be served and register their canonical identities.
func TestExplorer_D37oImpl1_AuthorityAndViewportUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	authority := getExplorerAsset(t, srv, d37oImpl1AuthorityPoc)
	if !strings.Contains(authority, `vp.register('authority', _authorityRendererFactory)`) {
		t.Errorf("D37o-impl-1: Authority renderer factory registration must remain untouched")
	}

	vp := getExplorerAsset(t, srv, d37oImpl1ViewportHost)
	if !strings.Contains(vp, "ACTIVE_RENDERER_ATTR") {
		t.Errorf("D37o-impl-1: GraphViewport active-renderer-attribute constant must remain")
	}
	if !strings.Contains(vp, "adoptExisting('native-context')") {
		t.Errorf("D37o-impl-1: GraphViewport native-context baseline adoption must remain")
	}
}

// TestExplorer_D37oImpl1_ModelsLoadedBeforeStrategicRenderer pins that
// when index.html wires the model modules (done in D37o-impl-2 along
// with the strategic renderer skeleton), each model module's <script>
// tag appears BEFORE the strategic renderer's <script> tag. The
// renderer depends on the model surface at registration time.
func TestExplorer_D37oImpl1_ModelsLoadedBeforeStrategicRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	rendererIdx := strings.Index(body, "context-cytoscape-renderer.js")
	if rendererIdx < 0 {
		// Renderer wiring lives in D37o-impl-2; if it has not yet
		// landed, this test is a no-op. The model modules must still
		// be present in the served HTML on their own merits when the
		// renderer ships.
		t.Skip("strategic renderer wiring not yet present in index.html (pre-D37o-impl-2)")
	}

	for _, modelAsset := range []string{
		"context-card-model.js",
		"context-connector-model.js",
		"context-layout-model.js",
	} {
		modelIdx := strings.Index(body, modelAsset)
		if modelIdx < 0 {
			t.Errorf("D37o-impl-2: model module %q must be wired in index.html alongside the renderer", modelAsset)
			continue
		}
		if modelIdx >= rendererIdx {
			t.Errorf("D37o-impl-2: %q must load BEFORE context-cytoscape-renderer.js (model surface required at registration)", modelAsset)
		}
	}
}

// TestExplorer_D37oImpl1_RightDrawerUntouched is a no-regression pin
// for the right drawer markup.
func TestExplorer_D37oImpl1_RightDrawerUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-details"`,
		`gmap-right-rail`,
		`id="gmap-rail-panel-inspector"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-1: right drawer markup %q must remain", want)
		}
	}
}

// TestExplorer_D37oImpl1_EvidenceTrayUntouched is a no-regression pin
// for the bottom evidence tray markup.
func TestExplorer_D37oImpl1_EvidenceTrayUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-evidence-tray"`,
		`id="gmap-evidence-tray-panel"`,
		`data-tab="drift"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37o-impl-1: evidence tray markup %q must remain", want)
		}
	}
}
