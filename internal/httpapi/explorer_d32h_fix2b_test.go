package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// explorer_d32h_fix2b_test.go — D32h-fix-2b pins the lens-aware
// dispatch in selectGovernanceMapNode (the single inline shim every
// selection-entry path converges on) and the Authority inspector's
// new evidence-tray hook notification. Tests are intentionally
// focused on the *contract* the tranche introduces; no scope-creep
// pins (those belong to follow-up tranches).
//
// All tests are source-string pins against the served HTML / JS
// — Tier-1 coverage. Tier-2 (executed-JS) coverage is scheduled for
// D32h-fix-2d's goja harness.

// TestExplorer_D32hFix2b_SelectGovernanceMapNodeIsLensAware pins
// that the shim reads selectedGraphLens from the store before
// dispatching.
func TestExplorer_D32hFix2b_SelectGovernanceMapNodeIsLensAware(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// The shim must read the lens from the store; default to 'context'
	// when the store / getter is unavailable; guard against throws.
	for _, want := range []string{
		"function selectGovernanceMapNode(nodeId)",
		"window.MIDASExplorerStore.getState().selectedGraphLens",
		"var lens = 'context';",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32h-fix-2b: selectGovernanceMapNode must read the active lens — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2b_SelectGovernanceMapNodeRoutesAuthorityToAuthorityInspector
// pins the authority branch of the lens-aware dispatch.
func TestExplorer_D32hFix2b_SelectGovernanceMapNodeRoutesAuthorityToAuthorityInspector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Authority branch: guarded by lens === 'authority', existence
	// check, and typeof === 'function' check, then dispatches.
	for _, want := range []string{
		"if (lens === 'authority' &&",
		"ExplorerGraph.authorityInspector &&",
		"typeof ExplorerGraph.authorityInspector.selectNode === 'function'",
		"return ExplorerGraph.authorityInspector.selectNode(nodeId);",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32h-fix-2b: Authority lens branch must guard + dispatch — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesContextDefault
// pins the Context fallthrough — when lens is not 'authority' or
// the Authority inspector is unavailable, the shim must call
// ExplorerGraph.contextInspector.selectNode(nodeId) verbatim. The
// literal substring is also pinned by older tests
// (explorer_d32a_test.go:1490 and explorer_d32b_debug2_test.go:322);
// this test makes the preservation explicit for D32h-fix-2b.
func TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesContextDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, "return ExplorerGraph.contextInspector.selectNode(nodeId);") {
		t.Error("D32h-fix-2b: Context fallthrough must preserve the call ExplorerGraph.contextInspector.selectNode(nodeId) byte-for-byte (older tests pin this literal)")
	}
}

// TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesGmapSelectedId
// pins that gmapSelectedId is set BEFORE lens dispatch, so every
// callsite that reads the inline primary-selection binding observes
// the same value regardless of which inspector runs.
func TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesGmapSelectedId(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	fnIdx := strings.Index(body, "function selectGovernanceMapNode(nodeId)")
	if fnIdx < 0 {
		t.Fatal("D32h-fix-2b: selectGovernanceMapNode function not found in served HTML")
	}
	// Look for the assignment within the function body.
	tail := body[fnIdx:]
	assignIdx := strings.Index(tail, "gmapSelectedId = nodeId;")
	if assignIdx < 0 {
		t.Fatal("D32h-fix-2b: gmapSelectedId = nodeId; must remain inside selectGovernanceMapNode")
	}
	lensReadIdx := strings.Index(tail, "selectedGraphLens")
	if lensReadIdx < 0 {
		t.Fatal("D32h-fix-2b: lens read missing inside selectGovernanceMapNode")
	}
	if assignIdx >= lensReadIdx {
		t.Errorf("D32h-fix-2b: gmapSelectedId = nodeId; (offset %d) must come BEFORE the lens read (offset %d); inline callers depend on the primary-selection binding being current before dispatch", assignIdx, lensReadIdx)
	}
}

// TestExplorer_D32hFix2b_SelectGovernanceMapNodeDefaultsToContextOnReadFailure
// pins the defensive try/catch — the shim must not throw if the
// store is missing or getState fails; it must fall through to the
// Context branch.
func TestExplorer_D32hFix2b_SelectGovernanceMapNodeDefaultsToContextOnReadFailure(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	fnIdx := strings.Index(body, "function selectGovernanceMapNode(nodeId)")
	if fnIdx < 0 {
		t.Fatal("D32h-fix-2b: selectGovernanceMapNode function not found in served HTML")
	}
	tail := body[fnIdx:]
	// The lens read must be wrapped in a try/catch within the
	// selectGovernanceMapNode function body. We look for the
	// distinctive try { ... } catch (_) { ... } shape.
	if !strings.Contains(tail, "try {") {
		t.Error("D32h-fix-2b: lens read must be wrapped in try { … } catch (_) { … } so the shim defaults to Context on read failure")
	}
	if !strings.Contains(tail, "catch (_) { /* default to context */ }") {
		t.Error("D32h-fix-2b: the catch clause must default to Context — no throw should escape selectGovernanceMapNode")
	}
}

// TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeNotifiesEvidenceTrayHook
// pins the parity addition — Authority's selectNode must call
// _inspectorHooks.notifyEvidenceTraySelectionChanged so the
// Authority workbench refreshes on click. Mirrors Context's notify
// at context-graph-inspector.js:111-113.
func TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeNotifiesEvidenceTrayHook(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-inspector.js")

	for _, want := range []string{
		"window.MIDASExplorerGraph._inspectorHooks",
		"typeof hooks.notifyEvidenceTraySelectionChanged === 'function'",
		"hooks.notifyEvidenceTraySelectionChanged();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2b: Authority selectNode must notify the evidence-tray hook — missing %q", want)
		}
	}

	// The notify call must come AFTER _renderInto so the workbench
	// reads the freshly-painted selection state.
	renderIdx := strings.Index(js, "_renderInto(selectedNode);")
	notifyIdx := strings.Index(js, "hooks.notifyEvidenceTraySelectionChanged();")
	if renderIdx < 0 {
		t.Fatal("D32h-fix-2b: _renderInto call missing from Authority selectNode")
	}
	if notifyIdx < 0 {
		t.Fatal("D32h-fix-2b: notifyEvidenceTraySelectionChanged call missing from Authority selectNode")
	}
	if notifyIdx <= renderIdx {
		t.Errorf("D32h-fix-2b: notify hook (offset %d) must fire AFTER _renderInto (offset %d) so the workbench reads the post-paint selection state", notifyIdx, renderIdx)
	}
}

// TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeMarksSelectedCard
// pins the existing parity behaviour: Authority's selectNode
// continues to mark `.selected` on the clicked card and clear it
// elsewhere. This behaviour predates D32h-fix-2b but becomes
// load-bearing now that the click path actually reaches Authority's
// selectNode.
func TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeMarksSelectedCard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-inspector.js")

	for _, want := range []string{
		"function selectNode(nodeId)",
		"canvas.querySelectorAll('.gmap-node')",
		"n.classList.toggle('selected', isSel);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2b: Authority selectNode must mark `.selected` on the clicked card — missing %q", want)
		}
	}
}
