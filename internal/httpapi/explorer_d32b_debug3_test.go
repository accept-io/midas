package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// explorer_d32b_debug3_test.go — D32b-debug-3 regression coverage for
// the Context Graph reframe failure observed in DevTools:
//
//   user clicks `surf:surf-v2-card-issuance-decision` (a surface node)
//   → selected node remains `bs:bs-cards` (the root business service)
//   → reframe buttons never render
//
// The static D32b-debug-2 source-presence tests all passed against
// the buggy state because the reframe MACHINERY was present — just
// silently disconnected from the renderer's click event. The root
// cause was a module-load-order bug in graph-renderer.js:
//
//   // BUGGY pattern — eager capture
//   var _hooks = window.MIDASExplorerGraph._rendererHooks =
//                window.MIDASExplorerGraph._rendererHooks || {};
//
//   // Later, index.html's inline IIFE:
//   window.MIDASExplorerGraph._rendererHooks = { selectNode: …, … };
//   // This REPLACES the property with a fresh object literal — the
//   // renderer's `_hooks` reference still points at the empty pre-
//   // IIFE object, so `_hooks.selectNode` is undefined.
//
// The fix converts `_hooks` from an eagerly-captured reference to a
// lazy accessor function so every call site sees the current state
// of `window.MIDASExplorerGraph._rendererHooks`. The tests below pin
// that contract explicitly so a future refactor that re-introduces
// eager caching fails loudly with a layer-specific message.

// TestExplorer_D32bDebug3_RendererHooksResolvedLazilyOnEachCall pins
// the core fix: graph-renderer.js MUST NOT eagerly cache the hook
// bundle at module load time. The hook object is reassigned (not
// mutated) by index.html's inline IIFE AFTER this module loads — a
// cached reference would freeze on the empty pre-IIFE object and
// every `_hooks.X` call would silently no-op via the `typeof === 'function'`
// guard.
//
// The canonical fix shape:
//
//   function _hooks() {
//     return (window.MIDASExplorerGraph &&
//             window.MIDASExplorerGraph._rendererHooks) || {};
//   }
//
// And every call site re-resolves: `var h = _hooks(); if (typeof h.X === 'function') h.X(...);`
func TestExplorer_D32bDebug3_RendererHooksResolvedLazilyOnEachCall(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	// Eager-capture form is the regression signature. A `var _hooks =
	// window.MIDASExplorerGraph._rendererHooks` declaration freezes the
	// reference at module load. Match the literal source line we're
	// banning.
	if strings.Contains(js, "var _hooks = window.MIDASExplorerGraph._rendererHooks =") {
		t.Error("D32b-debug-3: graph-renderer.js must NOT eagerly cache _hooks = window.MIDASExplorerGraph._rendererHooks at module load — index.html reassigns this property later, so cached references silently miss every hook call (root cause of the operator-observed Context Graph reframe regression)")
	}
	if strings.Contains(js, "var _hooks =\n               window.MIDASExplorerGraph._rendererHooks") {
		t.Error("D32b-debug-3: graph-renderer.js must NOT eagerly cache _hooks (two-line variant of the eager-capture pattern)")
	}

	// The lazy accessor must exist with the canonical shape.
	if !strings.Contains(js, "function _hooks() {") {
		t.Error("D32b-debug-3: graph-renderer.js must declare a `function _hooks()` accessor so every call site re-resolves window.MIDASExplorerGraph._rendererHooks at use time")
	}
	if !strings.Contains(js, "window.MIDASExplorerGraph._rendererHooks") {
		t.Error("D32b-debug-3: the lazy _hooks() accessor must read window.MIDASExplorerGraph._rendererHooks")
	}
}

// TestExplorer_D32bDebug3_ClickHandlerInvokesLazyHookSelectNode pins
// the click handler's use of the lazy hook accessor. The DevTools
// repro shows the failure mode: clicking a surface node leaves the
// selection on the root BS because the click handler's
// `_hooks.selectNode(spec.id)` call was reaching the stale empty
// pre-IIFE object, so the typeof guard returned false and the
// inspector was never invoked.
//
// Post-fix, the click handler resolves the bundle freshly and
// dispatches through it:
//
//   var h = _hooks();
//   ...
//   if (typeof h.selectNode === 'function') h.selectNode(spec.id);
//
// This pin protects against a future refactor that re-introduces
// the eager-capture form (which would re-introduce the runtime bug).
func TestExplorer_D32bDebug3_ClickHandlerInvokesLazyHookSelectNode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	// Find the click handler. It contains a `var h = _hooks();` resolve
	// followed by `h.selectNode(spec.id)`.
	clickIdx := strings.Index(js, "node.addEventListener('click', function (e) {")
	if clickIdx < 0 {
		t.Fatal("D32b-debug-3: addNode click handler missing")
	}
	endRel := strings.Index(js[clickIdx:], "node.addEventListener('keydown'")
	if endRel <= 0 {
		t.Fatal("D32b-debug-3: cannot bound the click handler slice")
	}
	clickBody := js[clickIdx : clickIdx+endRel]

	if !strings.Contains(clickBody, "var h = _hooks();") {
		t.Error("D32b-debug-3: click handler must call `var h = _hooks();` so each click sees the live (post-IIFE) hook bundle, not a stale reference")
	}
	if !strings.Contains(clickBody, "h.selectNode(spec.id)") {
		t.Error("D32b-debug-3: click handler must invoke h.selectNode(spec.id) — drives the inspector's selectGovernanceMapNode → contextInspector.selectNode → setActions chain that renders the reframe button")
	}
	// Pin the ordering: applyMultiSelection BEFORE selectNode so the
	// .gmap-multi-selected class and the .selected class are applied
	// in lockstep. (Pre-existing contract; pinned here because the
	// rewrite touched the click handler.)
	amIdx := strings.Index(clickBody, "h.applyMultiSelection()")
	snIdx := strings.LastIndex(clickBody, "h.selectNode(spec.id)")
	if amIdx < 0 || snIdx < 0 || !(amIdx < snIdx) {
		t.Errorf("D32b-debug-3: click handler must call h.applyMultiSelection() BEFORE h.selectNode(spec.id) (am=%d sn=%d)", amIdx, snIdx)
	}
}

// TestExplorer_D32bDebug3_KeydownHandlerInvokesLazyHookSelectNode
// extends the click pin to the keyboard activation path. The keydown
// handler (Enter / Space) hits the same selection chain, so the same
// lazy-resolution discipline is required. A regression in either path
// would leave the inspector silent on keyboard activation.
func TestExplorer_D32bDebug3_KeydownHandlerInvokesLazyHookSelectNode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	kdIdx := strings.Index(js, "node.addEventListener('keydown', function (e) {")
	if kdIdx < 0 {
		t.Fatal("D32b-debug-3: addNode keydown handler missing")
	}
	endRel := strings.Index(js[kdIdx:], "}");
	if endRel <= 0 {
		t.Fatal("D32b-debug-3: cannot bound the keydown handler slice")
	}
	// Bound widely so the slice captures the body.
	end := kdIdx + 400
	if end > len(js) {
		end = len(js)
	}
	kdBody := js[kdIdx:end]

	if !strings.Contains(kdBody, "var h = _hooks();") {
		t.Error("D32b-debug-3: keydown handler must call `var h = _hooks();` (Enter/Space activation must reach the same selection chain as click)")
	}
	if !strings.Contains(kdBody, "h.selectNode(spec.id)") {
		t.Error("D32b-debug-3: keydown handler must invoke h.selectNode(spec.id) on Enter/Space activation")
	}
}

// TestExplorer_D32bDebug3_NoEagerHooksReferencesRemain audits the
// entire renderer file for any remaining `_hooks.X` reference (no
// parentheses). After D32b-debug-3 every call site must be either
// `_hooks().X` (lazy resolve at use) or `h.X` where `var h = _hooks()`
// just above. A `_hooks.X` form would re-introduce the eager-capture
// regression for that specific call.
//
// Comments containing `_hooks.X` are tolerated by stripping line
// content after the first `//` before the scan. The accessor
// declaration `function _hooks() { ... }` is matched separately
// and excluded.
func TestExplorer_D32bDebug3_NoEagerHooksReferencesRemain(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	lines := strings.Split(js, "\n")
	for i, raw := range lines {
		// Strip comments. The renderer's source has only `// ...` style
		// comments — there are no `/* */` blocks that span multiple
		// lines on the relevant lines, so per-line stripping is safe.
		code := raw
		if idx := strings.Index(code, "//"); idx >= 0 {
			code = code[:idx]
		}
		if !strings.Contains(code, "_hooks") {
			continue
		}
		// The accessor declaration is allowed.
		if strings.Contains(code, "function _hooks()") {
			continue
		}
		// Lazy form `_hooks()` is allowed. Any remaining `_hooks.X`
		// without the parens is the regression form.
		stripped := strings.ReplaceAll(code, "_hooks()", "")
		if strings.Contains(stripped, "_hooks.") {
			t.Errorf("D32b-debug-3: graph-renderer.js line %d contains eager `_hooks.X` reference (must be `_hooks().X` or `var h = _hooks(); h.X`): %q", i+1, strings.TrimSpace(raw))
		}
	}
}

// TestExplorer_D32bDebug3_RendererHooksAssignmentInIndexHTML pins the
// index.html side of the contract: the inline IIFE assigns a fresh
// object literal to window.MIDASExplorerGraph._rendererHooks. This
// is the assignment shape that breaks an eager-capture cache, so
// flipping graph-renderer.js back to eager caching would re-introduce
// the bug. We pin BOTH sides so the contract is symmetric:
//   • renderer reads lazily (D32bDebug3_RendererHooksResolvedLazilyOnEachCall)
//   • inline IIFE reassigns rather than mutates (this test)
func TestExplorer_D32bDebug3_RendererHooksAssignmentInIndexHTML(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// The IIFE must set _rendererHooks. We don't pin the assignment
	// form (= vs Object.assign) — what matters is that the property
	// ends up with selectNode wired. The lazy-resolution test guards
	// the renderer regardless of the form.
	if !strings.Contains(body, "window.MIDASExplorerGraph._rendererHooks = {") {
		t.Error("D32b-debug-3: index.html must register window.MIDASExplorerGraph._rendererHooks (the bundle the renderer's lazy _hooks() accessor reads)")
	}
	if !strings.Contains(body, "selectNode:              function (id)") &&
		!strings.Contains(body, "selectNode: function (id)") {
		t.Error("D32b-debug-3: window.MIDASExplorerGraph._rendererHooks must declare selectNode — bridges the renderer's click event to selectGovernanceMapNode")
	}
}

// TestExplorer_D32bDebug3_ContextInspectorSelectNodeUsesDataNodeId
// pins the inspector's node-resolution strategy. The DevTools repro
// suggested a id-vs-dataset mismatch hypothesis (document.getElementById
// would fail because nodes don't carry DOM ids); the actual root cause
// was elsewhere, but the resolution strategy still needs explicit
// protection. Inspector resolution uses `n.dataset.nodeId === nodeId`
// over `canvas.querySelectorAll('.gmap-node')` — node lookup by data-
// attribute, not DOM id.
func TestExplorer_D32bDebug3_ContextInspectorSelectNodeUsesDataNodeId(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-inspector.js")

	if !strings.Contains(js, "n.dataset.nodeId === nodeId") {
		t.Error("D32b-debug-3: context-graph-inspector.js selectNode must resolve nodes by `n.dataset.nodeId === nodeId` (not document.getElementById, which would fail because nodes carry data attributes only)")
	}
	if !strings.Contains(js, "canvas.querySelectorAll('.gmap-node')") {
		t.Error("D32b-debug-3: context-graph-inspector.js selectNode must enumerate every `.gmap-node` via querySelectorAll — the only loop that toggles the `.selected` class on the canvas")
	}
	if strings.Contains(js, "document.getElementById(nodeId)") {
		t.Error("D32b-debug-3: context-graph-inspector.js must NOT resolve the clicked node via document.getElementById — DOM ids are not set on .gmap-node elements; the lookup would fail and selection would stay on the root")
	}
}

// TestExplorer_D32bDebug3_RendererAddsDataNodeIdAttribute pins the
// other half of the dataset-lookup contract: the renderer writes
// `data-node-id="<spec.id>"` on every node card. Inspector
// resolution depends on this attribute matching the renderer's spec.id.
func TestExplorer_D32bDebug3_RendererAddsDataNodeIdAttribute(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-renderer.js")

	if !strings.Contains(js, "node.dataset.nodeId = spec.id;") {
		t.Error("D32b-debug-3: graph-renderer.js addNode must set `node.dataset.nodeId = spec.id` so context-graph-inspector.selectNode can resolve the clicked node by data-attribute")
	}
}

// TestExplorer_D32bDebug3_ReframeRefreshUsesCurrentGraphViewAndRoot
// pins the refresh dispatch: after handleGovernanceMapAction sets
// currentGraphView/currentGraphRootId to the reframe target, the
// downstream refreshGovernanceMap must fetch THOSE values via
// shell.refresh, not a stale 'service' / rootBS. A regression that
// hard-coded view:'service' here would render the SAME BS subgraph
// no matter which reframe target the operator picked.
func TestExplorer_D32bDebug3_ReframeRefreshUsesCurrentGraphViewAndRoot(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	fnIdx := strings.Index(body, "function refreshGovernanceMap()")
	if fnIdx < 0 {
		t.Fatal("D32b-debug-3: refreshGovernanceMap function missing")
	}
	endRel := strings.Index(body[fnIdx:], "\n  }\n")
	if endRel <= 0 {
		t.Fatal("D32b-debug-3: refreshGovernanceMap body unbounded")
	}
	fnBody := body[fnIdx : fnIdx+endRel]

	// The fetch view comes from currentGraphView, NOT a literal.
	if !strings.Contains(fnBody, "const fetchView = currentGraphView") &&
		!strings.Contains(fnBody, "var fetchView = currentGraphView") {
		t.Error("D32b-debug-3: refreshGovernanceMap must capture fetchView from currentGraphView (not a hard-coded view) — otherwise reframe targets render against the wrong projection")
	}
	if !strings.Contains(fnBody, "view: fetchView") {
		t.Error("D32b-debug-3: shell.refresh dispatch must pass view: fetchView (the reframed currentGraphView)")
	}
	// The id comes from currentGraphRootId.
	if !strings.Contains(fnBody, "const rootId = currentGraphRootId") &&
		!strings.Contains(fnBody, "var rootId = currentGraphRootId") {
		t.Error("D32b-debug-3: refreshGovernanceMap must capture rootId from currentGraphRootId so reframe targets the new root")
	}
}

// TestExplorer_D32bDebug3_ReframeDedupResetsBeforeRefresh pins that
// the reframe dispatcher resets `gmapLastBSId = null` BEFORE
// invoking refreshGovernanceMap. Without this reset, refreshGovernance
// Map's existing dedup (`rootId === gmapLastBSId`) would return early
// when the operator reframes back to a previously-rendered root.
// Pre-existing contract; restated here so a regression that moves
// the dedup reset to AFTER the refresh call (no-op) is caught loudly.
func TestExplorer_D32bDebug3_ReframeDedupResetsBeforeRefresh(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	caseIdx := strings.Index(body, "case 'reframe-around-this':")
	if caseIdx < 0 {
		t.Fatal("D32b-debug-3: reframe case missing")
	}
	endRel := strings.Index(body[caseIdx+1:], "      case ")
	if endRel <= 0 {
		endRel = strings.Index(body[caseIdx:], "default:")
	}
	caseBody := body[caseIdx : caseIdx+1+endRel]

	resetIdx := strings.Index(caseBody, "gmapLastBSId = null")
	refreshIdx := strings.Index(caseBody, "refreshGovernanceMap()")
	if resetIdx < 0 {
		t.Error("D32b-debug-3: reframe case must reset gmapLastBSId = null (otherwise the dedup in refreshGovernanceMap blocks the new fetch when reframing back to a previously-rendered root)")
	}
	if refreshIdx < 0 {
		t.Error("D32b-debug-3: reframe case must invoke refreshGovernanceMap()")
	}
	if resetIdx >= 0 && refreshIdx >= 0 && !(resetIdx < refreshIdx) {
		t.Errorf("D32b-debug-3: gmapLastBSId reset (offset %d) must precede refreshGovernanceMap (offset %d)", resetIdx, refreshIdx)
	}
}

// TestExplorer_D32bDebug3_BackStackPushesCurrentBeforeOverwrite pins
// the back-stack contract: pushGmapHistory captures CURRENT
// currentGraphView + currentGraphRootId, and the reframe case
// invokes the push BEFORE mutating those locals. A regression that
// reordered overwrite-then-push would lose the previous root on Back.
func TestExplorer_D32bDebug3_BackStackPushesCurrentBeforeOverwrite(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	caseIdx := strings.Index(body, "case 'reframe-around-this':")
	if caseIdx < 0 {
		t.Fatal("D32b-debug-3: reframe case missing")
	}
	endRel := strings.Index(body[caseIdx+1:], "      case ")
	if endRel <= 0 {
		endRel = strings.Index(body[caseIdx:], "default:")
	}
	caseBody := body[caseIdx : caseIdx+1+endRel]

	pushIdx := strings.Index(caseBody, "pushGmapHistory(action.target_view, action.target_id)")
	setViewIdx := strings.Index(caseBody, "currentGraphView = action.target_view")
	setRootIdx := strings.Index(caseBody, "currentGraphRootId = action.target_id")
	if pushIdx < 0 || setViewIdx < 0 || setRootIdx < 0 {
		t.Fatal("D32b-debug-3: reframe case must push history then overwrite currentGraphView/currentGraphRootId")
	}
	if !(pushIdx < setViewIdx && pushIdx < setRootIdx) {
		t.Errorf("D32b-debug-3: pushGmapHistory (offset %d) must precede currentGraphView/RootId overwrites (view=%d root=%d) — otherwise Back loses the previous root", pushIdx, setViewIdx, setRootIdx)
	}
}

// TestExplorer_D32bDebug3_AuthorityIsolationPreserved pins that the
// D32b-debug-3 fix does not bleed into Authority lens behaviour:
//   • Authority view still reads selectedGraphLens for its render guard
//   • Authority does NOT register reframe-around-this affordances
//   • The Context refreshGovernanceMap lens guard is intact
//
// This is the cross-lens regression boundary the user explicitly
// called out: Authority Graph behaviour must remain unchanged.
func TestExplorer_D32bDebug3_AuthorityIsolationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Authority render guard intact (D32b-debug-1).
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if !strings.Contains(authJS, "selectedGraphLens") {
		t.Error("D32b-debug-3: authority-graph-view.js must still read selectedGraphLens (D32b-debug-1 lens guard preserved)")
	}
	if strings.Contains(authJS, "'reframe-around-this'") {
		t.Error("D32b-debug-3: authority-graph-view.js must NOT register reframe-around-this — Authority lens does not support reframe (Context-only contract)")
	}

	// Context refresh lens guard intact (D32b-debug-1).
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	fnIdx := strings.Index(body, "function refreshGovernanceMap()")
	endRel := strings.Index(body[fnIdx:], "\n  }\n")
	fnBody := body[fnIdx : fnIdx+endRel]
	if !strings.Contains(fnBody, "activeLens !== 'context'") {
		t.Error("D32b-debug-3: refreshGovernanceMap lens guard `activeLens !== 'context'` must still be in place (D32b-debug-1 preserved)")
	}
}
