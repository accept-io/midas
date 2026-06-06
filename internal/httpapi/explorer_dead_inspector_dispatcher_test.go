package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-clean-2 — Dead Inspector Dispatcher Retirement tests.
//
// Symmetric to D37p-clean-1. The legacy
// `MIDASExplorerGraph.inspector.register / renderNode / clear`
// dispatcher was a per-lens dispatch table on a separate namespace
// from `MIDASExplorerGraph.renderer`. The same diagnostic findings
// apply: zero runtime call-sites for the dispatch entry points; every
// live consumer reaches the inspector frame via:
//
//   • `MIDASExplorerGraph.inspector.set*` frame setters (lens-agnostic);
//   • `MIDASExplorerGraph.authorityInspector.selectNode(nodeId)` for
//     Authority per-node renders (called by the inline graph-shell
//     selectNode hook and the surface-posture panel).
//
// D37p-clean-2 retires:
//   • the three dispatch functions on `graph-inspector.js`
//     (`register / renderNode / clear`);
//   • the internal `_impls` registry;
//   • the dispatcher-adapter `inspectorImpl` in `context-graph-view.js`
//     and the matching `inspector.register('context', ...)` call;
//   • the dispatcher-adapter `inspectorImpl` in
//     `authority-graph-inspector.js` and the matching
//     `inspector.register('authority', ...)` call.
//
// The lens-agnostic frame-setter surface stays. The Authority
// `selectNode` entry point stays. Visible UI is unchanged.

const (
	d37pClean2InspectorAsset      = "/explorer/assets/js/graph/graph-inspector.js"
	d37pClean2ContextViewAsset    = "/explorer/assets/js/graph/context/context-graph-view.js"
	d37pClean2AuthorityInspector  = "/explorer/assets/js/graph/authority/authority-graph-inspector.js"
	d37pClean2AuthorityEdgeAsset  = "/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37pClean2AuthorityWorkbench  = "/explorer/assets/js/graph/authority/authority-graph-workbench.js"
	d37pClean2DrawerAsset         = "/explorer/assets/js/graph/graph-drawer.js"
	d37pClean2PaneShellAsset      = "/explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js"
	d37pClean2ContextPaneAsset    = "/explorer/assets/js/graph/context/context-selected-object-pane.js"
)

// ── A. Dispatcher functions removed from graph-inspector.js ─────────

func TestExplorer_D37pClean2_DispatcherFunctionsRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2InspectorAsset)

	if regexp.MustCompile(`function\s+register\s*\(\s*lens\s*,\s*impl\s*\)`).MatchString(js) {
		t.Errorf("D37p-clean-2: graph-inspector.js must not define a register(lens, impl) function anymore")
	}
	if regexp.MustCompile(`function\s+renderNode\s*\(\s*lens\s*,\s*node\s*,\s*mount\s*\)`).MatchString(js) {
		t.Errorf("D37p-clean-2: graph-inspector.js must not define a renderNode(lens, node, mount) dispatch function anymore")
	}
	if regexp.MustCompile(`function\s+clear\s*\(\s*lens\s*,\s*mount\s*\)`).MatchString(js) {
		t.Errorf("D37p-clean-2: graph-inspector.js must not define a clear(lens, mount) dispatch function anymore")
	}
	if regexp.MustCompile(`var\s+_impls\s*=\s*\{\}`).MatchString(js) {
		t.Errorf("D37p-clean-2: graph-inspector.js must not keep an internal _impls dispatch registry anymore")
	}
}

// TestExplorer_D37pClean2_InspectorPublicSurfaceSlimmed pins that the
// dead dispatch methods are no longer exported on
// `MIDASExplorerGraph.inspector` while the live frame setters + show
// / hide remain.
func TestExplorer_D37pClean2_InspectorPublicSurfaceSlimmed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2InspectorAsset)

	startIdx := strings.Index(js, "window.MIDASExplorerGraph.inspector = {")
	if startIdx < 0 {
		t.Fatalf("D37p-clean-2: graph-inspector.js must still expose window.MIDASExplorerGraph.inspector")
	}
	tail := js[startIdx:]
	endRel := strings.Index(tail, "};")
	if endRel < 0 {
		t.Fatalf("D37p-clean-2: graph-inspector.js public surface literal must be terminated")
	}
	surface := tail[:endRel]

	// Dead dispatch methods must be gone from the literal.
	for _, banned := range []string{
		"register:",
		"renderNode:",
		"clear:",
	} {
		if strings.Contains(surface, banned) {
			t.Errorf("D37p-clean-2: dead dispatch method %q must be removed from MIDASExplorerGraph.inspector public surface", banned)
		}
	}

	// Live frame setters + show / hide must remain.
	for _, want := range []string{
		"show:",
		"hide:",
		"setName:",
		"setFields:",
		"setSummary:",
		"setGovernance:",
		"setActions:",
		"setInlineActions:",
	} {
		if !strings.Contains(surface, want) {
			t.Errorf("D37p-clean-2: live frame setter %q must remain on MIDASExplorerGraph.inspector", want)
		}
	}
}

// TestExplorer_D37pClean2_LiveFrameSettersBodiesIntact pins that the
// live frame-setter implementations are preserved verbatim (each
// function still resolves its target DOM id and writes to it).
func TestExplorer_D37pClean2_LiveFrameSettersBodiesIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2InspectorAsset)

	for _, want := range []string{
		"function setName(name)",
		"function setFields(rows)",
		"function setSummary(rows)",
		"function setGovernance(html)",
		"function setActions(actions)",
		"function setInlineActions(node, actions)",
		"function show(mount)",
		"function hide(mount)",
		"document.getElementById('gmap-details-name')",
		"document.getElementById('gmap-details-fields')",
		"document.getElementById('gmap-details-summary')",
		"document.getElementById('gmap-details-governance')",
		"document.getElementById('gmap-details-actions')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-2: live frame-setter implementation %q must be preserved", want)
		}
	}
}

// ── B. Context view: dispatcher adapter retired ─────────────────────

func TestExplorer_D37pClean2_ContextInspectorImplRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2ContextViewAsset)

	if strings.Contains(js, "window.MIDASExplorerGraph.inspector.register('context', inspectorImpl)") {
		t.Errorf("D37p-clean-2: dead inspector.register('context', inspectorImpl) call must be removed from context-graph-view.js")
	}
	// The dispatcher-adapter `inspectorImpl` object literal must be
	// gone. The pattern is distinct enough to assert without
	// false-positives.
	if regexp.MustCompile(`var\s+inspectorImpl\s*=\s*\{\s*[\s\S]*?renderNode:\s*function\s*\(node,\s*mount\)`).MatchString(js) {
		t.Errorf("D37p-clean-2: dead Context inspectorImpl object literal must be removed from context-graph-view.js")
	}
}

// TestExplorer_D37pClean2_ContextViewLivePathPreserved pins the live
// Context entry points after this retirement.
func TestExplorer_D37pClean2_ContextViewLivePathPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2ContextViewAsset)

	for _, want := range []string{
		"function renderContextGraph(data, ctx)",
		"function renderContextGraphEmpty(message, bsId, ctx)",
		"function renderContextGraphError(message, ctx)",
		"window.MIDASExplorerGraph.contextView",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-2: live Context entry point %q must remain in context-graph-view.js", want)
		}
	}
}

// ── C. Authority inspector: dispatcher adapter retired ──────────────

func TestExplorer_D37pClean2_AuthorityInspectorImplRetired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2AuthorityInspector)

	if strings.Contains(js, "window.MIDASExplorerGraph.inspector.register('authority', inspectorImpl)") {
		t.Errorf("D37p-clean-2: dead inspector.register('authority', inspectorImpl) call must be removed from authority-graph-inspector.js")
	}
	if regexp.MustCompile(`var\s+inspectorImpl\s*=\s*\{\s*[\s\S]*?renderNode:\s*renderNode`).MatchString(js) {
		t.Errorf("D37p-clean-2: dead Authority inspectorImpl object literal must be removed from authority-graph-inspector.js")
	}
}

// TestExplorer_D37pClean2_AuthorityInspectorLivePathPreserved pins
// that the live Authority inspector entry point survives.
func TestExplorer_D37pClean2_AuthorityInspectorLivePathPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2AuthorityInspector)

	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityInspector",
		"selectNode:",
		// renderNode + clear remain on the public surface for
		// forward-compatibility even though the dispatcher adapter is
		// gone — see the D37p-clean-2 comment in authority-graph-inspector.js.
		"function renderNode(node, mount)",
		"function clear(mount)",
		// The live frame-setter call sites that drive the Authority
		// inspector content must be intact.
		"insp.setName(",
		"insp.setFields(",
		"insp.setSummary(",
		"insp.setGovernance(",
		"insp.setActions(",
		"insp.setInlineActions(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-2: live Authority inspector path %q must remain in authority-graph-inspector.js", want)
		}
	}
}

// ── D. No live consumer accidentally removed ────────────────────────

// TestExplorer_D37pClean2_FrameSetterCallSitesIntact pins that the
// existing call sites against the live frame setters in
// context-graph-inspector.js and context-selection-bridge.js continue
// to compile cleanly against the post-retirement surface.
func TestExplorer_D37pClean2_FrameSetterCallSitesIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	ctxInspector := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-graph-inspector.js")
	for _, want := range []string{
		"insp.setName(",
		"insp.setFields(",
		"insp.setGovernance(",
		"insp.setActions(",
		"insp.setInlineActions(",
	} {
		if !strings.Contains(ctxInspector, want) {
			t.Errorf("D37p-clean-2: context-graph-inspector.js must still call live frame setter %q", want)
		}
	}

	ctxSelBridge := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-selection-bridge.js")
	for _, want := range []string{
		"insp.setName(",
		"insp.setFields(",
		"insp.setGovernance(",
		"insp.setActions(",
	} {
		if !strings.Contains(ctxSelBridge, want) {
			t.Errorf("D37p-clean-2: context-selection-bridge.js must still call live frame setter %q", want)
		}
	}
}

// ── E. Authority canvas-edge, drawer, workbench, pane shell intact ─

func TestExplorer_D37pClean2_AuthorityCanvasEdgeIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2AuthorityEdgeAsset)

	// The canvas-edge module must still expose its full public surface
	// + the D37p-authority-2-impl provider registration. None of this
	// tranche's work should touch the canvas-edge tabs.
	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityCanvasEdgeTabs",
		"registerLensProvider('authority'",
		"init:",
		"destroy:",
		"render:",
		"openTab:",
		"closePane:",
		"syncSelection:",
		"isOpen:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-2: Authority canvas-edge module must remain intact (missing %q)", want)
		}
	}
}

func TestExplorer_D37pClean2_AuthorityWorkbenchIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2AuthorityWorkbench)

	for _, want := range []string{
		"window.MIDASExplorerGraph.authorityWorkbench",
		"setActiveTab:",
		"render:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-2: Authority workbench module must remain intact (missing %q)", want)
		}
	}
}

func TestExplorer_D37pClean2_DrawerIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2DrawerAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.drawer",
		"registerLens:",
		"setActiveLens:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-2: graph-drawer.js must remain intact (missing %q)", want)
		}
	}
}

func TestExplorer_D37pClean2_PaneShellIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2PaneShellAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.graphSelectedObjectPane",
		"registerLensProvider:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-clean-2: graphSelectedObjectPane shell must remain intact (missing %q)", want)
		}
	}
}

func TestExplorer_D37pClean2_ContextPaneIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pClean2ContextPaneAsset)

	if !strings.Contains(js, "registerLensProvider('context'") {
		t.Errorf("D37p-clean-2: Context selected-object pane provider registration must remain intact")
	}
}

// ── F. DOM markup preserved ────────────────────────────────────────

func TestExplorer_D37pClean2_DomMarkupPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		// Right drawer wrapper
		`id="gmap-details"`,
		// Frame setters write into these ids
		`id="gmap-details-name"`,
		`id="gmap-details-fields"`,
		`id="gmap-details-summary"`,
		`id="gmap-details-governance"`,
		`id="gmap-details-actions"`,
		// Authority canvas-edge wrapper
		`data-authority-canvas-edge-tabs`,
		// Authority workbench
		`id="gmap-authority-workbench"`,
		// Context selected-object pane wrapper
		`data-context-selected-object-pane`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37p-clean-2: index.html must still contain %q (no DOM retirement in this tranche)", want)
		}
	}
}

// ── G. Index.html ceiling ──────────────────────────────────────────

func TestExplorer_D37pClean2_IndexHtmlWithinCeiling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	lines := strings.Count(body, "\n") + 1
	if lines > 8000 {
		t.Errorf("D37p-clean-2: index.html line count %d exceeds the existing 8000 ceiling", lines)
	}
}
