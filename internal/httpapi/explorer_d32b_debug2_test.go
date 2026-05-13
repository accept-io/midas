package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// explorer_d32b_debug2_test.go — D32b-debug-2 regression coverage
// for the Context Graph "reframe-around-this" feature. The operator-
// reported regression was that clicking an eligible Context Graph
// node (decision surface, AI system, related business service) no
// longer reframes the graph rooted at that node.
//
// Static investigation found the reframe machinery intact at every
// layer (adapter emission → renderer dataset write → inspector
// dataset read → frame-setter button render → action-dispatcher
// hook → handleGovernanceMapAction dispatch → refreshGovernanceMap
// re-fetch). The tests below harden that machinery: a regression at
// any layer surfaces here loudly with a layer-specific message so
// the operator-visible bug never re-appears silently.
//
// All tests in this file consume the production extracted modules
// (graph-renderer.js, graph-inspector.js, context-graph-view.js,
// context-graph-inspector.js) via getExplorerAsset / getExplorerAllJS
// — pinning the actual surface the browser executes, not stale shim
// declarations.

// TestExplorer_D32bDebug2_ContextViewEmitsReframeActionForEligibleNodes
// pins reframe-around-this action emission on the three Context Graph
// node types that historically support reframe: decision surface,
// AI system, and related business service. A regression that drops
// the actions: [...] payload from any of these addNode calls means
// the inspector receives an empty actions array, the reframe button
// never renders, and the operator-reported "disappeared" symptom
// reappears.
//
// Capabilities and processes are intentionally NOT pinned for reframe —
// they have never been valid subgraph roots in the Context Graph
// backend projection (no /v1/graphs/context?view=capability route).
// The user-reported list ("decision surface, AI system, capability,
// process, related business service") is more permissive than the
// backend contract; the static pins here match what the backend
// actually supports.
func TestExplorer_D32bDebug2_ContextViewEmitsReframeActionForEligibleNodes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-view.js")

	// Decision surface reframe — surface card renders with target_view:'decision_surface'.
	if !strings.Contains(js, "target_view: 'decision_surface'") {
		t.Error("D32b-debug-2: context-graph-view.js must emit reframe-around-this with target_view:'decision_surface' on surface nodes")
	}
	// AI system reframe — ai card renders with target_view:'ai_system'.
	if !strings.Contains(js, "target_view: 'ai_system'") {
		t.Error("D32b-debug-2: context-graph-view.js must emit reframe-around-this with target_view:'ai_system' on AI system nodes")
	}
	// Related-business-service reframe — related-service card renders with target_view:'service'.
	if !strings.Contains(js, "target_view: 'service'") {
		t.Error("D32b-debug-2: context-graph-view.js must emit reframe-around-this with target_view:'service' on related business service nodes")
	}
	// The action kind itself must appear at least three times — one per
	// emission site above. The kind is the dispatcher's switch key, so
	// renaming or dropping it breaks all three node types at once.
	if got := strings.Count(js, "'reframe-around-this'"); got < 3 {
		t.Errorf("D32b-debug-2: context-graph-view.js must emit 'reframe-around-this' on three node types (surface, ai_system, related_service); got %d occurrences", got)
	}
	// Buttons whose label varies per node type carry the user-facing
	// string. Pin both: surface/ai use 'Reframe around this'; related
	// service uses 'Open service graph' (semantically the same action,
	// historically distinct label).
	if !strings.Contains(js, "'Reframe around this'") {
		t.Error("D32b-debug-2: context-graph-view.js must declare the 'Reframe around this' button label for surface/ai_system reframes")
	}
	if !strings.Contains(js, "'Open service graph'") {
		t.Error("D32b-debug-2: context-graph-view.js must declare the 'Open service graph' button label for related-service reframes")
	}
}

// TestExplorer_D32bDebug2_RendererAddNodeWritesActionsToDataset
// pins the renderer's contract that every addNode call serialises
// spec.actions onto the node card's `data-node-actions` attribute.
// The inspector (context-graph-inspector.js selectNode) reads this
// attribute to know which actions to render — a regression that
// stops the renderer writing it would silently strip every reframe
// button without changing the action-emission code at all.
func TestExplorer_D32bDebug2_RendererAddNodeWritesActionsToDataset(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	if !strings.Contains(js, "node.dataset.nodeActions = JSON.stringify(spec.actions || []);") {
		t.Error("D32b-debug-2: graph-renderer.js addNode must serialise spec.actions to node.dataset.nodeActions — the inspector reads this attribute to render reframe buttons")
	}
	// The inline-actions slot is the DOM home for inline reframe
	// buttons (the ones rendered on the selected node card itself).
	if !strings.Contains(js, `'<div class="gmap-node-inline-actions" hidden></div>'`) {
		t.Error("D32b-debug-2: graph-renderer.js addNode must include the inline-actions slot in the node card template — graph-inspector.setInlineActions targets this slot")
	}
}

// TestExplorer_D32bDebug2_ContextInspectorForwardsActionsToFrameSetters
// pins the production Context inspector's contract: on every
// selectNode call it reads the renderer-written
// `data-node-actions` attribute and forwards the parsed array to
// BOTH setActions (bottom panel) and setInlineActions (node card).
// A regression that drops one of these calls leaves the operator
// with reframe affordance in only one place — or none — depending
// on which call was removed.
func TestExplorer_D32bDebug2_ContextInspectorForwardsActionsToFrameSetters(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-inspector.js")

	if !strings.Contains(js, "selectedNode.dataset.nodeActions") {
		t.Error("D32b-debug-2: context-graph-inspector.js selectNode must read selectedNode.dataset.nodeActions")
	}
	if !strings.Contains(js, "insp.setActions(") {
		t.Error("D32b-debug-2: context-graph-inspector.js selectNode must forward parsed actions to inspector.setActions — drives the bottom-panel reframe button")
	}
	if !strings.Contains(js, "insp.setInlineActions(selectedNode,") {
		t.Error("D32b-debug-2: context-graph-inspector.js selectNode must forward parsed actions to inspector.setInlineActions(node, …) — drives the inline reframe button on the node card")
	}
}

// TestExplorer_D32bDebug2_GraphInspectorRendersReframeButtons
// pins that the lens-agnostic inspector frame's setActions +
// setInlineActions know how to render reframe-around-this buttons.
// A regression that drops the reframe branch from either function
// leaves the actions array unrendered. Pin both the bottom-panel
// class (gmap-action-reframe) and the inline class
// (gmap-action-reframe-inline) so the CSS rules that style them
// continue to match.
func TestExplorer_D32bDebug2_GraphInspectorRendersReframeButtons(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-inspector.js")

	// Bottom-panel reframe button branch — gmap-action-reframe class.
	if !strings.Contains(js, "'gmap-action-reframe'") &&
		!strings.Contains(js, `"gmap-action-reframe"`) &&
		!strings.Contains(js, "btn btn-secondary gmap-action-reframe") {
		t.Error("D32b-debug-2: graph-inspector.js setActions must render the bottom-panel reframe button with class 'gmap-action-reframe'")
	}
	// Inline reframe button branch — gmap-action-reframe-inline class.
	if !strings.Contains(js, "'gmap-action-reframe-inline'") &&
		!strings.Contains(js, `"gmap-action-reframe-inline"`) &&
		!strings.Contains(js, "gmap-action-reframe-inline") {
		t.Error("D32b-debug-2: graph-inspector.js setInlineActions must render the inline reframe button with class 'gmap-action-reframe-inline'")
	}
	// Both branches must guard on action.kind === 'reframe-around-this'.
	if got := strings.Count(js, "'reframe-around-this'"); got < 2 {
		t.Errorf("D32b-debug-2: graph-inspector.js must check action.kind === 'reframe-around-this' in BOTH setActions and setInlineActions; got %d occurrences", got)
	}
	// Click handler must invoke the dispatcher hook. The dispatcher
	// is wired in index.html (window.MIDASExplorerGraph._actionDispatcher);
	// the inspector receives it lazily via the _dispatch() helper.
	if !strings.Contains(js, "if (dispatch) dispatch(action);") {
		t.Error("D32b-debug-2: graph-inspector.js reframe button click handler must invoke dispatch(action) — without this the button is a visual decoration only")
	}
}

// TestExplorer_D32bDebug2_ActionDispatcherInvokesHandleGovernanceMapAction
// pins the inline dispatcher hook in index.html that the inspector's
// `_dispatch()` resolves to. The dispatcher forwards reframe action
// objects to handleGovernanceMapAction (the central switch); a
// regression that disconnects this hook leaves every reframe button
// click as a no-op.
func TestExplorer_D32bDebug2_ActionDispatcherInvokesHandleGovernanceMapAction(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, "window.MIDASExplorerGraph._actionDispatcher = function (action) {") {
		t.Error("D32b-debug-2: index.html must register window.MIDASExplorerGraph._actionDispatcher — graph-inspector.js resolves the dispatcher through this hook")
	}
	if !strings.Contains(body, "handleGovernanceMapAction(action)") {
		t.Error("D32b-debug-2: _actionDispatcher must forward to handleGovernanceMapAction(action) so the inspector button reaches the inline switch")
	}
}

// TestExplorer_D32bDebug2_ReframeDispatchDoesNotConsultLensGuard pins
// the boundary of the D32b-debug-1 lens guard: it lives ONLY in
// refreshGovernanceMap (and renderAuthorityGraph), NOT in the
// reframe-around-this case of handleGovernanceMapAction. The
// reframe-case sets currentGraphView / currentGraphRootId and
// invokes refreshGovernanceMap; the lens guard inside that callee
// is the only point of contention. A regression that added a
// duplicate lens-guard early-return inside the reframe case would
// silently block reframe even when the operator is in Context
// (because selectedGraphLens may briefly be 'authority' between a
// pre-seed and a setActiveLens call).
func TestExplorer_D32bDebug2_ReframeDispatchDoesNotConsultLensGuard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	caseIdx := strings.Index(body, "case 'reframe-around-this':")
	if caseIdx < 0 {
		t.Fatal("D32b-debug-2: handleGovernanceMapAction must declare a `case 'reframe-around-this':` branch")
	}
	// Bound by the next `case ` keyword in the switch (the default).
	endRel := strings.Index(body[caseIdx+1:], "      case ")
	if endRel <= 0 {
		endRel = strings.Index(body[caseIdx:], "default:")
		if endRel <= 0 {
			t.Fatal("D32b-debug-2: cannot bound the reframe case slice (default branch missing)")
		}
	}
	caseBody := body[caseIdx : caseIdx+1+endRel]

	// The reframe case must NOT read selectedGraphLens directly — the
	// downstream refreshGovernanceMap is the single source of lens
	// gating, so a duplicate here would over-gate.
	if strings.Contains(caseBody, "selectedGraphLens") {
		t.Error("D32b-debug-2: reframe-around-this case must NOT read selectedGraphLens — only refreshGovernanceMap performs the lens guard; duplicating it here would silently block Context reframe")
	}
	// And it must still push history + set currentGraphView /
	// currentGraphRootId + invoke refreshGovernanceMap so the new
	// root actually paints.
	for _, want := range []string{
		"pushGmapHistory(action.target_view, action.target_id)",
		"currentGraphView = action.target_view",
		"currentGraphRootId = action.target_id",
		"gmapLastBSId = null",
		"refreshGovernanceMap()",
	} {
		if !strings.Contains(caseBody, want) {
			t.Errorf("D32b-debug-2: reframe-around-this case body must contain %q", want)
		}
	}
}

// TestExplorer_D32bDebug2_SetWorkbenchModeContextRestoresLensBeforeShowMap
// pins the post-Authority recovery path: when the operator switches
// from Authority back to Context, setWorkbenchMode('context') must
// call setActiveLens('context') BEFORE showMap. setActiveLens
// updates store.selectedGraphLens to 'context'; if showMap fired
// first while selectedGraphLens still carried the 'authority' pre-
// seed from the previous Authority switch, refreshGovernanceMap's
// lens guard would block the Context render and the operator would
// see an empty / stale canvas.
//
// The symmetric pre-seed in the 'authority' branch is already
// pinned by TestExplorer_D32bDebug1_SetWorkbenchModeAuthorityPreSeedsLens;
// this test is the Context-side complement that ensures the
// recovery direction works too.
func TestExplorer_D32bDebug2_SetWorkbenchModeContextRestoresLensBeforeShowMap(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	branchIdx := strings.Index(body, "if (mode === 'context') {")
	if branchIdx < 0 {
		t.Fatal("D32b-debug-2: Context branch of setWorkbenchMode missing")
	}
	endRel := strings.Index(body[branchIdx:], "\n    }\n")
	if endRel <= 0 {
		t.Fatal("D32b-debug-2: Context branch body has no closing brace")
	}
	branchBody := body[branchIdx : branchIdx+endRel]

	setLensIdx := strings.Index(branchBody, "ExplorerGraph.shell.setActiveLens('context')")
	showMapIdx := strings.Index(branchBody, "MIDASExplorerServices.showMap(serviceId)")
	if setLensIdx < 0 {
		t.Error("D32b-debug-2: Context branch must invoke ExplorerGraph.shell.setActiveLens('context') so selectedGraphLens is restored to 'context' before any Context fetch fires")
	}
	if showMapIdx < 0 {
		t.Fatal("D32b-debug-2: Context branch must continue to call MIDASExplorerServices.showMap(serviceId)")
	}
	if setLensIdx >= 0 && showMapIdx >= 0 && !(setLensIdx < showMapIdx) {
		t.Errorf("D32b-debug-2: setActiveLens('context') at offset %d must precede showMap at offset %d — otherwise refreshGovernanceMap reads a stale 'authority' selectedGraphLens (from a prior Authority pre-seed) and blocks the Context render",
			setLensIdx, showMapIdx)
	}
}

// TestExplorer_D32bDebug2_RendererClickInvokesSelectNodeHook pins the
// renderer's click handler routing: a node click must call the
// _rendererHooks.selectNode hook so the inline orchestration
// (selectGovernanceMapNode → contextInspector.selectNode → setActions)
// receives the selection event. Without this hook fire the inspector
// is never told about the click; setActions never runs; the reframe
// button never appears.
//
// D32b-debug-3 updated: the click handler resolves the hook bundle
// via the lazy `_hooks()` accessor (because index.html reassigns
// the window-level hook object AFTER this module loads — see the
// TestExplorer_D32bDebug3_RendererHooksResolvedLazilyOnEachCall pin).
// The call form is therefore `h.selectNode(spec.id)` where `h = _hooks()`.
func TestExplorer_D32bDebug2_RendererClickInvokesSelectNodeHook(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	if !strings.Contains(js, "h.selectNode(spec.id)") {
		t.Error("D32b-debug-2: graph-renderer.js addNode click handler must call h.selectNode(spec.id) (where h = _hooks()) — drives selectGovernanceMapNode → contextInspector.selectNode → setActions/setInlineActions")
	}
	// The hook bundle itself must be readable from the renderer module
	// — it consults window.MIDASExplorerGraph._rendererHooks lazily
	// (see TestExplorer_D32bDebug3_RendererHooksResolvedLazilyOnEachCall).
	if !strings.Contains(js, "_rendererHooks") {
		t.Error("D32b-debug-2: graph-renderer.js must consult window.MIDASExplorerGraph._rendererHooks for the click → select-node bridge")
	}
}

// TestExplorer_D32bDebug2_RendererHookBridgesSelectNodeToContextInspector
// pins the inline hook bundle that bridges the renderer's click event
// to the production Context inspector. Index.html wires _rendererHooks
// .selectNode → selectGovernanceMapNode (the thin shim that delegates
// to ExplorerGraph.contextInspector.selectNode). A regression that
// breaks any link in this chain leaves clicks with no visible effect.
func TestExplorer_D32bDebug2_RendererHookBridgesSelectNodeToContextInspector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// 1. The _rendererHooks bundle wires selectNode to selectGovernanceMapNode.
	if !strings.Contains(body, "selectNode:              function (id)       { if (typeof selectGovernanceMapNode === 'function') selectGovernanceMapNode(id); }") {
		t.Error("D32b-debug-2: index.html _rendererHooks must wire selectNode → selectGovernanceMapNode so the renderer's click event reaches the inline inspector orchestration")
	}
	// 2. The shim itself delegates to ExplorerGraph.contextInspector.selectNode.
	if !strings.Contains(body, "ExplorerGraph.contextInspector.selectNode(nodeId)") {
		t.Error("D32b-debug-2: selectGovernanceMapNode shim must delegate to ExplorerGraph.contextInspector.selectNode(nodeId) — the production Context inspector lives in graph/context/context-graph-inspector.js")
	}
}

// TestExplorer_D32bDebug2_ReframeButtonCSSShowsOnSelectedNode pins the
// CSS that makes the inline reframe button visible. The slot is
// `display: none` by default; the rule
//
//   .gmap-node.selected .gmap-node-inline-actions:not([hidden]) {
//     display: flex;
//   }
//
// is what reveals the button when (a) the node carries `.selected`
// (driven by context-graph-inspector.selectNode) AND (b) the slot
// is not [hidden] (driven by setInlineActions removing the hidden
// attribute when at least one inline action renders).
//
// A regression that drops this rule — or restructures the selector
// — leaves the button in the DOM but invisible, producing the exact
// operator-reported "disappeared" symptom.
func TestExplorer_D32bDebug2_ReframeButtonCSSShowsOnSelectedNode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	if !strings.Contains(css, ".gmap-node-inline-actions {") {
		t.Error("D32b-debug-2: governance-map.css must declare the .gmap-node-inline-actions base rule (display:none default)")
	}
	if !strings.Contains(css, ".gmap-node.selected .gmap-node-inline-actions:not([hidden])") {
		t.Error("D32b-debug-2: governance-map.css must declare the reveal rule `.gmap-node.selected .gmap-node-inline-actions:not([hidden])` — otherwise the inline reframe button never becomes visible even when rendered correctly")
	}
	if !strings.Contains(css, ".gmap-action-reframe-inline") {
		t.Error("D32b-debug-2: governance-map.css must style the .gmap-action-reframe-inline button — without sizing/padding the button collapses to zero")
	}
}

// TestExplorer_D32bDebug2_ReframeFlowEndToEndPinsAllLayers consolidates
// the layered pins above into a single end-to-end audit. The test
// runs through the conceptual JS surface (index.html + every module)
// and asserts the named string artefacts at each layer exist together
// in one place. A future refactor that moves any one of these to a
// new file but accidentally drops it during the move will fail this
// single audit even if the per-layer pins drift; the per-layer pins
// fail with layer-specific messages.
func TestExplorer_D32bDebug2_ReframeFlowEndToEndPinsAllLayers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAllJS(t, srv)

	for _, want := range []string{
		// Layer 1 — adapter/view emission
		"'reframe-around-this'",
		"target_view: 'decision_surface'",
		"target_view: 'ai_system'",
		"target_view: 'service'",
		// Layer 2 — renderer dataset write
		"node.dataset.nodeActions = JSON.stringify(spec.actions || []);",
		"gmap-node-inline-actions",
		// Layer 3 — context inspector forwards
		"selectedNode.dataset.nodeActions",
		"insp.setActions(",
		"insp.setInlineActions(selectedNode,",
		// Layer 4 — frame-setter button render
		"gmap-action-reframe",
		"gmap-action-reframe-inline",
		// Layer 5 — dispatcher hook
		"window.MIDASExplorerGraph._actionDispatcher = function (action)",
		"handleGovernanceMapAction(action)",
		// Layer 6 — handleGovernanceMapAction dispatch
		"case 'reframe-around-this':",
		"pushGmapHistory(action.target_view, action.target_id)",
		"currentGraphView = action.target_view",
		"currentGraphRootId = action.target_id",
		// Layer 7 — refresh fires
		"refreshGovernanceMap()",
		// Layer 8 — shell.refresh dispatches lens:'context'
		"ExplorerGraph.shell.refresh({ lens: 'context'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32b-debug-2: reframe end-to-end audit missing artefact %q — the layered pins above should also flag this but the end-to-end audit catches cross-cutting drops", want)
		}
	}
}
