package httpapi

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D36b-knowledge-graph-frontend-projection-contract
//
// Contract definition tranche. Adds:
//   • docs/design/D36b-knowledge-graph-frontend-projection-contract.md
//     — the frontend-facing projection contract (envelope, node
//     kinds, edge kinds, payloads, facets, summary/diagnostics,
//     renderer expectations, GraphViewport integration, deferred
//     backend expectations, non-goals, future implementation
//     sequence).
//   • /explorer/assets/js/graph/knowledge/knowledge-graph-contract.js
//     — small declarative constants module so the renderer id and
//     canonical node/edge kind taxonomies live in exactly one
//     place.
//   • Tiny harmless touch on the D36a shell: it now sources its
//     `RENDERER_ID` from the contract constant defensively, with
//     a fallback to the literal `'knowledge-graph'`.
//
// D36b does NOT add: a real renderer, real layout, Cytoscape,
// backend, OpenAPI, schema, seed, fetch, mock graph data, or any
// graph-viewport.js modification. Tests below pin all of these.

const (
	d36bContractDocPath   = "../../docs/design/D36b-knowledge-graph-frontend-projection-contract.md"
	d36bConstantsAssetURL = "/explorer/assets/js/graph/knowledge/knowledge-graph-contract.js"
	d36bShellAssetURL     = "/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js"
	d36bHostAssetURL      = "/explorer/assets/js/graph/graph-viewport.js"
)

// d36bCanonicalNodeKinds — the 10 kinds the contract document and
// constants module both pin as canonical for D36b.
var d36bCanonicalNodeKinds = []string{
	"ai_system",
	"business_service",
	"capability",
	"concept",
	"control",
	"decision_surface",
	"evidence",
	"obligation",
	"policy",
	"risk_theme",
}

// d36bDeferredNodeKinds — recognised in the namespace, not
// canonical in D36b. Listed in the doc and constants so the
// namespace stays coherent.
var d36bDeferredNodeKinds = []string{
	"concept_cluster",
	"metric",
}

// d36bCanonicalEdgeKinds — the 10 kinds the contract document and
// constants module both pin as canonical for D36b.
var d36bCanonicalEdgeKinds = []string{
	"applies_to",
	"constrains",
	"depends_on",
	"derived_from",
	"evidences",
	"governs",
	"implements",
	"mitigates",
	"relates_to",
	"supports",
}

// d36bDeferredEdgeKinds — recognised, not canonical.
var d36bDeferredEdgeKinds = []string{
	"aggregates",
	"measures",
}

// readContractDocD36b reads the contract document from disk.
// Fatal if missing — the document IS the primary deliverable.
func readContractDocD36b(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(d36bContractDocPath)
	if err != nil {
		t.Fatalf("D36b: cannot read contract document at %s: %v", d36bContractDocPath, err)
	}
	return string(body)
}

// ── Contract document tests ─────────────────────────────────────────

// TestExplorer_D36bKnowledgeContract_DocumentExists pins the
// document is on disk at the canonical path and is non-trivial.
func TestExplorer_D36bKnowledgeContract_DocumentExists(t *testing.T) {
	info, err := os.Stat(d36bContractDocPath)
	if err != nil {
		t.Fatalf("D36b: contract document must exist at %s: %v", d36bContractDocPath, err)
	}
	if info.IsDir() {
		t.Fatalf("D36b: %s must be a file, not a directory", d36bContractDocPath)
	}
	if info.Size() < 2048 {
		t.Errorf("D36b: contract document is suspiciously small (%d bytes); expected a comprehensive contract", info.Size())
	}
}

// TestExplorer_D36bKnowledgeContract_DocumentsStrategicPositioning
// pins the strategic platform-module framing: Knowledge Graph is
// hosted by GraphViewport, not a parallel viewport, and is
// compatible with MIDAS's modular, distributed, service-based
// direction.
func TestExplorer_D36bKnowledgeContract_DocumentsStrategicPositioning(t *testing.T) {
	doc := readContractDocD36b(t)
	for _, want := range []string{
		// Anchored to the D35h architecture contract.
		"midas-graph-viewport.md",
		// Knowledge renderer id from D36a.
		"'knowledge-graph'",
		// Strategic framing. Substrings chosen to be wrap-tolerant.
		"modular, distributed, service-based",
		"graph-domain view hosted by",
		"create its own viewport",
		// Not a replacement for the existing lenses. The doc's
		// phrasing uses bold around "**not**", so we check for the
		// substring that survives the markdown emphasis.
		"replacement for Context Graph",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: contract must document strategic positioning — missing %q", want)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_DistinguishesContextAuthorityKnowledge
// pins that the document explicitly separates the three lenses by
// scope, primary question, and renderer-id namespace.
func TestExplorer_D36bKnowledgeContract_DistinguishesContextAuthorityKnowledge(t *testing.T) {
	doc := readContractDocD36b(t)
	for _, want := range []string{
		// Section heading.
		"Relationship to Context Graph and Authority Graph",
		// Each lens named and contrasted.
		"Context Graph",
		"Authority Graph",
		"Knowledge Graph",
		// Renderer ids.
		"context-cytoscape",
		"authority-cytoscape",
		"knowledge-graph",
		// Knowledge's primary question.
		"What knowledge relationships explain or constrain",
		// Rules of separation.
		"references but does not own",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: contract must distinguish lenses — missing %q", want)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_DocumentsProjectionEnvelope
// pins the top-level envelope fields the renderer will consume.
func TestExplorer_D36bKnowledgeContract_DocumentsProjectionEnvelope(t *testing.T) {
	doc := readContractDocD36b(t)
	for _, want := range []string{
		"Frontend projection shape",
		`"view": "knowledge"`,
		`"projection_id"`,
		`"root"`,
		`"nodes"`,
		`"edges"`,
		`"facets"`,
		`"summary"`,
		`"diagnostics"`,
		`"generated_at"`,
		`"snapshot_id"`,
		`"warnings"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: projection envelope must document %q", want)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_DocumentsNodeKinds pins every
// canonical and deferred node kind in the document, AND that the
// document carries the per-kind discipline columns.
func TestExplorer_D36bKnowledgeContract_DocumentsNodeKinds(t *testing.T) {
	doc := readContractDocD36b(t)
	if !strings.Contains(doc, "Node kinds") {
		t.Error("D36b: contract must contain a 'Node kinds' section")
	}
	// Every canonical kind appears in the document.
	for _, kind := range d36bCanonicalNodeKinds {
		if !strings.Contains(doc, "`"+kind+"`") {
			t.Errorf("D36b: canonical node kind %q must be documented (with backtick code style)", kind)
		}
	}
	// Every deferred kind appears in the document.
	for _, kind := range d36bDeferredNodeKinds {
		if !strings.Contains(doc, "`"+kind+"`") {
			t.Errorf("D36b: deferred node kind %q must be documented (with backtick code style)", kind)
		}
	}
	// Per-kind discipline: each canonical kind is annotated as
	// canonical for D36b.
	if !strings.Contains(doc, "canonical (D36b)") {
		t.Error("D36b: contract must annotate canonical node kinds with 'canonical (D36b)'")
	}
	// Deferred kinds are explicitly flagged.
	if !strings.Contains(doc, "**deferred**") {
		t.Error("D36b: contract must explicitly flag deferred kinds with '**deferred**'")
	}
}

// TestExplorer_D36bKnowledgeContract_DocumentsEdgeKinds pins
// every canonical and deferred edge kind.
func TestExplorer_D36bKnowledgeContract_DocumentsEdgeKinds(t *testing.T) {
	doc := readContractDocD36b(t)
	if !strings.Contains(doc, "Edge kinds") {
		t.Error("D36b: contract must contain an 'Edge kinds' section")
	}
	for _, kind := range d36bCanonicalEdgeKinds {
		if !strings.Contains(doc, "`"+kind+"`") {
			t.Errorf("D36b: canonical edge kind %q must be documented (with backtick code style)", kind)
		}
	}
	for _, kind := range d36bDeferredEdgeKinds {
		if !strings.Contains(doc, "`"+kind+"`") {
			t.Errorf("D36b: deferred edge kind %q must be documented (with backtick code style)", kind)
		}
	}
	// Directionality contract: at least bidirectional vs directional
	// must appear so the renderer knows when to draw arrowheads.
	for _, want := range []string{"bidirectional", "directional"} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: edge-kind table must document directionality — missing %q", want)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_DocumentsNodeAndEdgePayloads
// pins the node + edge payload contracts.
func TestExplorer_D36bKnowledgeContract_DocumentsNodeAndEdgePayloads(t *testing.T) {
	doc := readContractDocD36b(t)
	for _, want := range []string{
		// Node payload section + key fields.
		"Node payload contract",
		`"label"`,
		`"subtitle"`,
		`"status"`,
		`"description"`,
		`"metadata"`,
		`"links"`,
		// Edge payload section + key fields.
		"Edge payload contract",
		`"source"`,
		`"target"`,
		`"confidence"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: payload contracts must document %q", want)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_DocumentsFacetsSummaryDiagnostics
// pins the facets / summary / diagnostics sections.
func TestExplorer_D36bKnowledgeContract_DocumentsFacetsSummaryDiagnostics(t *testing.T) {
	doc := readContractDocD36b(t)
	for _, want := range []string{
		// Section names.
		"Facets / filters / grouping model",
		"Summary and diagnostics model",
		// Facet-side concepts.
		"node kind",
		"edge kind",
		"confidence",
		// Summary-side concepts.
		"node_count_by_kind",
		"edge_count_by_kind",
		// Diagnostic levels — documented as a set in the contract.
		"{info, warning, error}",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: facets/summary/diagnostics must document %q", want)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_DocumentsGraphViewportIntegration
// pins the renderer + GraphViewport integration expectations.
func TestExplorer_D36bKnowledgeContract_DocumentsGraphViewportIntegration(t *testing.T) {
	doc := readContractDocD36b(t)
	for _, want := range []string{
		// Renderer expectations heading.
		"Renderer expectations",
		// Required renderer behaviours that the host enables.
		"viewport.register('knowledge-graph'",
		"viewport.activateById('knowledge-graph')",
		"ctx.getSafeArea()",
		"ctx.onResize",
		".midas-graph-renderer-slot",
		// GraphViewport integration heading.
		"GraphViewport integration expectations",
		// Zero-host-change invariant (wrap-tolerant substring).
		"zero changes to",
		// data-active-renderer identity flip.
		`data-active-renderer`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: GraphViewport integration must document %q", want)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_DefersBackendProjection pins
// that the backend endpoint is explicitly deferred (not
// implemented in D36b).
func TestExplorer_D36bKnowledgeContract_DefersBackendProjection(t *testing.T) {
	doc := readContractDocD36b(t)
	for _, want := range []string{
		"Future backend projection expectations",
		"GET /v1/graphs/knowledge",
		"NOT IMPLEMENTED IN D36b",
		// At least one explicit "deferred" / "out of scope" framing.
		"out of scope for",
		// Future client / service boundary note.
		"Future client / service boundary",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: backend must be explicitly deferred — missing %q", want)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_DocumentsFutureImplementationSequence
// pins the recommended sequence from D36c onward.
func TestExplorer_D36bKnowledgeContract_DocumentsFutureImplementationSequence(t *testing.T) {
	doc := readContractDocD36b(t)
	for _, want := range []string{
		"Future implementation sequence",
		"D36c",
		"D36d",
		"D36e",
		"D36f",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: future implementation sequence must document %q", want)
		}
	}
	// Non-goals + open questions sections are present.
	for _, want := range []string{
		"Non-goals",
		"Open questions and deferred decisions",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D36b: contract must include %q section", want)
		}
	}
}

// ── No-feature / no-backend tests ───────────────────────────────────

// TestExplorer_D36b_NoBackendSchemaOpenAPIOrSeedChanges pins that
// no Knowledge-Graph backend route, OpenAPI entry, schema change,
// or seed data was introduced.
func TestExplorer_D36b_NoBackendSchemaOpenAPIOrSeedChanges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Probe candidate Knowledge endpoints.
	for _, p := range []string{
		"/v1/graphs/knowledge",
		"/v1/graphs/knowledge/",
		"/v1/knowledge",
		"/v1/knowledge/",
		"/v1/knowledge-graph",
	} {
		resp := performRequest(t, srv, http.MethodGet, p, nil)
		if resp.Code == 200 {
			t.Errorf("D36b: endpoint %s must NOT be served — D36b is contract definition only", p)
		}
	}

	// OpenAPI sweep — fetch the OpenAPI document if any, and assert
	// no /v1/graphs/knowledge path appears.
	for _, openAPIPath := range []string{
		"/v1/openapi.json",
		"/openapi.json",
	} {
		resp := performRequest(t, srv, http.MethodGet, openAPIPath, nil)
		if resp.Code == 200 {
			body := resp.Body.String()
			if strings.Contains(body, "/v1/graphs/knowledge") ||
				strings.Contains(body, "/v1/knowledge") {
				t.Errorf("D36b: OpenAPI document at %s must NOT contain a Knowledge Graph endpoint path", openAPIPath)
			}
		}
	}

	// Schema sweep — schema files must NOT contain Knowledge-Graph
	// tables.
	for _, schemaPath := range []string{
		"../../internal/store/postgres/schema.sql",
	} {
		body, err := os.ReadFile(schemaPath)
		if err != nil {
			continue // schema file may not exist in every build
		}
		lower := strings.ToLower(string(body))
		for _, banned := range []string{
			"knowledge_graph",
			"knowledge_node",
			"knowledge_edge",
			"knowledge_projection",
		} {
			if strings.Contains(lower, banned) {
				t.Errorf("D36b: schema at %s must NOT contain Knowledge Graph table %q — D36b is contract only", schemaPath, banned)
			}
		}
	}
}

// TestExplorer_D36b_NoRuntimeKnowledgeFeatureCreep pins that the
// D36a Knowledge shell still does NOT contain any runtime feature
// patterns — no fetch, no Cytoscape, no mock graph data — and
// that graph-viewport.js still contains no Knowledge-specific
// branch or id literal.
func TestExplorer_D36b_NoRuntimeKnowledgeFeatureCreep(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Knowledge shell remains shell-only.
	shellJS := getExplorerAsset(t, srv, d36bShellAssetURL)
	shellExec := stripJSComments(shellJS)
	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/graphs/knowledge",
		"/v1/knowledge",
		"cytoscape(",
		"window.cytoscape",
		"require('cytoscape')",
		// Mock graph-data structures that would imply a fake graph.
		"nodes: [",
		"edges: [",
		"connectors:",
	} {
		if strings.Contains(shellExec, banned) {
			t.Errorf("D36b: Knowledge shell must NOT contain %q — shell is contract-only", banned)
		}
	}

	// graph-viewport.js still contains no Knowledge literal.
	hostJS := getExplorerAsset(t, srv, d36bHostAssetURL)
	if strings.Contains(hostJS, "'knowledge-graph'") ||
		strings.Contains(hostJS, `"knowledge-graph"`) {
		t.Error("D36b: graph-viewport.js must NOT contain the literal 'knowledge-graph' id — host stays renderer-neutral")
	}

	// Authority + Context modules unchanged (registry calls still
	// present, no new vp.register call introduced by D36b).
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	if !strings.Contains(authJS, "vp.register('authority', _authorityRendererFactory)") {
		t.Error("D36b: Authority registration must remain intact")
	}
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	if !strings.Contains(ctxJS, "vp.register('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D36b: Context registration must remain intact")
	}
}

// ── Constants module tests ──────────────────────────────────────────

// TestExplorer_D36bKnowledgeContract_ConstantsPinnedIfAdded pins
// the small declarative constants module: it exists, it exposes
// the documented constants on the documented namespace, the
// canonical-kind lists match the contract document exactly, and
// the module is purely declarative (no fetch, no DOM mutation
// outside the namespace export, no Cytoscape).
func TestExplorer_D36bKnowledgeContract_ConstantsPinnedIfAdded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d36bConstantsAssetURL)
	if len(js) < 256 {
		t.Fatalf("D36b: constants module at %s is suspiciously small (%d bytes)", d36bConstantsAssetURL, len(js))
	}

	// Public namespace + every documented constant.
	for _, want := range []string{
		"window.MIDASExplorerGraph.knowledgeGraphContract",
		"KNOWLEDGE_GRAPH_RENDERER_ID:",
		"KNOWLEDGE_PROJECTION_VIEW:",
		"KNOWLEDGE_NODE_KINDS:",
		"KNOWLEDGE_DEFERRED_NODE_KINDS:",
		"KNOWLEDGE_EDGE_KINDS:",
		"KNOWLEDGE_DEFERRED_EDGE_KINDS:",
		// Renderer id literal must equal D36a's.
		"var KNOWLEDGE_GRAPH_RENDERER_ID = 'knowledge-graph';",
		// View literal matches the projection envelope.
		"var KNOWLEDGE_PROJECTION_VIEW = 'knowledge';",
		// Frozen arrays so downstream consumers cannot mutate.
		"Object.freeze([",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D36b: constants module must include %q", want)
		}
	}

	// Every canonical / deferred kind from the contract appears as
	// a string literal in the constants module.
	for _, kind := range d36bCanonicalNodeKinds {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D36b: constants module must declare canonical node kind %q", kind)
		}
	}
	for _, kind := range d36bDeferredNodeKinds {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D36b: constants module must declare deferred node kind %q", kind)
		}
	}
	for _, kind := range d36bCanonicalEdgeKinds {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D36b: constants module must declare canonical edge kind %q", kind)
		}
	}
	for _, kind := range d36bDeferredEdgeKinds {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D36b: constants module must declare deferred edge kind %q", kind)
		}
	}

	// Purely declarative — no runtime side effects beyond the
	// namespace export.
	exec := stripJSComments(js)
	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"cytoscape(",
		"window.cytoscape",
		"document.createElement",
		"document.body",
		"document.getElementById",
		"document.getElementsByClassName",
		"document.querySelector",
		"setTimeout(",
		"setInterval(",
		"addEventListener(",
		"appendChild(",
		"removeChild(",
		"vp.register(",
		"vp.activate(",
		"vp.activateById(",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D36b: constants module must be purely declarative — must NOT contain %q", banned)
		}
	}
}

// TestExplorer_D36bKnowledgeContract_ConstantsAlignWithDocument
// pins that the constants module and the contract document remain
// in lockstep: every canonical / deferred kind in constants
// appears in the document, and the renderer id literal matches.
func TestExplorer_D36bKnowledgeContract_ConstantsAlignWithDocument(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	doc := readContractDocD36b(t)
	js := getExplorerAsset(t, srv, d36bConstantsAssetURL)

	// Renderer id literal matches between document and constants.
	const literalID = "'knowledge-graph'"
	if !strings.Contains(doc, literalID) {
		t.Errorf("D36b: contract document must reference renderer id %s", literalID)
	}
	if !strings.Contains(js, literalID) {
		t.Errorf("D36b: constants module must reference renderer id %s", literalID)
	}

	// Every canonical node kind in constants appears in doc.
	for _, kind := range d36bCanonicalNodeKinds {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D36b alignment: canonical node kind %q missing from constants", kind)
		}
		if !strings.Contains(doc, "`"+kind+"`") {
			t.Errorf("D36b alignment: canonical node kind %q missing from contract document", kind)
		}
	}
	// Every canonical edge kind in constants appears in doc.
	for _, kind := range d36bCanonicalEdgeKinds {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D36b alignment: canonical edge kind %q missing from constants", kind)
		}
		if !strings.Contains(doc, "`"+kind+"`") {
			t.Errorf("D36b alignment: canonical edge kind %q missing from contract document", kind)
		}
	}
	// Deferred kinds also appear in both.
	for _, kind := range append(append([]string{}, d36bDeferredNodeKinds...), d36bDeferredEdgeKinds...) {
		if !strings.Contains(js, "'"+kind+"'") {
			t.Errorf("D36b alignment: deferred kind %q missing from constants", kind)
		}
		if !strings.Contains(doc, "`"+kind+"`") {
			t.Errorf("D36b alignment: deferred kind %q missing from contract document", kind)
		}
	}

	// Constants module exists; document explicitly references it.
	if !strings.Contains(doc, "knowledge-graph-contract.js") {
		t.Error("D36b alignment: contract document must reference the constants module path")
	}
}

// ── Foundation regression tests ─────────────────────────────────────

// TestExplorer_D36b_D36aKnowledgeShellPreserved pins the D36a
// shell behaviour is intact and that the only D36a touch
// permitted in D36b — the defensive consumption of
// KNOWLEDGE_GRAPH_RENDERER_ID — landed correctly.
func TestExplorer_D36b_D36aKnowledgeShellPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	shellJS := getExplorerAsset(t, srv, d36bShellAssetURL)

	// D36a invariants still in place.
	for _, want := range []string{
		// Registration call.
		"vp.register('knowledge-graph', _knowledgeGraphRendererFactory)",
		// Factory + lifecycle.
		"var _knowledgeGraphRendererFactory = {",
		"mount: function (slotEl, ctx) {",
		"destroy: function () {",
		// Mount creates the renderer-owned root in the slot.
		"slotEl.appendChild(_mountEl)",
		// Resize subscription.
		"_rendererCtx.onResize(_onHostResize)",
		// Teardown helper.
		"_teardownResources",
		// Namespaced activation helper.
		"window.MIDASExplorerGraph.knowledgeGraphShell = {",
		"activate: function ()",
		"vp.activateById(RENDERER_ID)",
	} {
		if !strings.Contains(shellJS, want) {
			t.Errorf("D36b: D36a shell invariant %q must remain", want)
		}
	}

	// D36b — Shell now sources RENDERER_ID from the contract
	// module defensively. The fallback literal is preserved so the
	// shell still works if the contract module fails to load.
	for _, want := range []string{
		"window.MIDASExplorerGraph.knowledgeGraphContract",
		"_contract.KNOWLEDGE_GRAPH_RENDERER_ID",
		"'knowledge-graph'", // fallback literal
		"var RENDERER_ID",
	} {
		if !strings.Contains(shellJS, want) {
			t.Errorf("D36b: shell must consume contract constant with fallback — missing %q", want)
		}
	}

	// Index.html load order: contract.js must appear BEFORE the
	// shell so the shell's IIFE can consume the constant.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	cIdx := strings.Index(body, "knowledge-graph-contract.js")
	sIdx := strings.Index(body, "knowledge-graph-renderer.js")
	if cIdx < 0 || sIdx < 0 {
		t.Fatal("D36b: index.html must include both knowledge-graph-contract.js and knowledge-graph-renderer.js")
	}
	if cIdx >= sIdx {
		t.Error("D36b: knowledge-graph-contract.js must load BEFORE knowledge-graph-renderer.js so the shell can consume the renderer-id constant")
	}
}

// TestExplorer_D36b_D35GraphViewportContractsPreserved is the
// foundation-wide regression check. Every D35a–D36a invariant
// must remain intact.
func TestExplorer_D36b_D35GraphViewportContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a — structural DOM.
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D36b: D35a structural class %q must remain", want)
		}
	}

	hostJS := getExplorerAsset(t, srv, d36bHostAssetURL)
	// D35b/c/f/g host API surface still intact.
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport = {",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
		"function getSafeArea(",
		"function onResize(",
		"function adoptExisting(",
		"adoptExisting('native-context')",
		`var ACTIVE_RENDERER_ATTR = 'data-active-renderer';`,
		"function register(rendererId, factory)",
		"function activateById(rendererId)",
		"function listRegistered()",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D36b: host contract %q must remain", want)
		}
	}

	// D35d Authority + D35e Context still registered via the registry.
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	for _, want := range []string{
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
	} {
		if !strings.Contains(authJS, want) {
			t.Errorf("D36b: D35d Authority contract %q must remain", want)
		}
	}
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	for _, want := range []string{
		"vp.register('context-cytoscape', _contextCytoscapeRendererFactory)",
		"vp.activateById('context-cytoscape')",
	} {
		if !strings.Contains(ctxJS, want) {
			t.Errorf("D36b: D35e Context contract %q must remain", want)
		}
	}

	// D35f strategic clip + D35e overlay non-clipping.
	clipCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	if !strings.Contains(stripCSSComments(clipCSS), ".midas-graph-viewport") ||
		!strings.Contains(stripCSSComments(clipCSS), "overflow: hidden") {
		t.Error("D36b: `.midas-graph-viewport { overflow: hidden }` strategic clip must remain")
	}
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if strings.Count(stripCSSComments(spikeCSS), "overflow: hidden") != 1 {
		t.Error("D36b: spike CSS must have exactly 1 `overflow: hidden` (mount; overlay non-clipping)")
	}

	// D35h/D35i architecture documents still on disk.
	for _, p := range []string{
		"../../docs/design/midas-graph-viewport.md",
		"../../docs/design/D35i-graph-viewport-reuse-readiness-audit.md",
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("D36b: foundation document %s must remain: %v", p, err)
		}
	}
}
