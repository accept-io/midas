package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37o-toolbar-1 — Lens-Switch GraphViewport Restore tests
//
// Pins the narrow fix that restores Context-lens switching after the
// Authority renderer has been activated. The inline
// `setWorkbenchMode('context')` branch in index.html must now ask the
// GraphViewport host to deactivate when the active renderer is
// 'authority'. Under ?contextRenderer=strategic the strategic
// renderer's body-lens observer (context-cytoscape-renderer.js) is
// expected to re-activate 'context' on its own; the Context branch
// therefore does not hard-code `activateById('context')`.
//
// Asset-text + structural pin style matches the existing D37o tests.

// extractContextBranch returns the body of the `if (mode === 'context')
// { … }` block from index.html. We slice between the literal
// `if (mode === 'context') {` line and the next `if (mode === 'authority') {`
// line (the next sibling branch in `setWorkbenchMode`) rather than
// regex-matching the closing brace, because the branch contains
// nested if-blocks whose closing braces are easy to over-match.
func extractContextBranch(t *testing.T, body string) string {
	t.Helper()
	const ctxMarker  = "if (mode === 'context') {"
	const authMarker = "if (mode === 'authority') {"
	ctxIdx := strings.Index(body, ctxMarker)
	if ctxIdx < 0 {
		t.Fatalf("D37o-toolbar-1: could not locate %q in index.html", ctxMarker)
	}
	authIdx := strings.Index(body[ctxIdx:], authMarker)
	if authIdx < 0 {
		t.Fatalf("D37o-toolbar-1: could not locate %q after the Context branch", authMarker)
	}
	return body[ctxIdx : ctxIdx+authIdx]
}

func extractAuthorityBranch(t *testing.T, body string) string {
	t.Helper()
	const authMarker = "if (mode === 'authority') {"
	authIdx := strings.Index(body, authMarker)
	if authIdx < 0 {
		t.Fatalf("D37o-toolbar-1: could not locate %q in index.html", authMarker)
	}
	// The Authority branch is the LAST branch in `setWorkbenchMode`;
	// the function closes with `\n  }\n` (the function body's close)
	// at two-space indent. Slice up to the close of the enclosing
	// function so we never bleed into adjacent inline state.
	rest := body[authIdx:]
	end := strings.Index(rest, "\n  }\n")
	if end < 0 {
		t.Fatalf("D37o-toolbar-1: could not locate the end of setWorkbenchMode after the Authority branch")
	}
	return rest[:end]
}

// ── A. Context branch restores the viewport baseline ─────────────────

func TestExplorer_D37oToolbar1_ContextBranchRestoresViewportBaseline(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	branch := extractContextBranch(t, body)

	if !strings.Contains(branch, "MIDASExplorerGraph.viewport.deactivate()") {
		t.Errorf("D37o-toolbar-1: Context branch must call MIDASExplorerGraph.viewport.deactivate() to restore the baseline from 'authority'")
	}
	// The deactivate call must be wrapped in try/catch so a viewport
	// host failure does not break lens switching.
	if !regexp.MustCompile(`try\s*\{\s*window\.MIDASExplorerGraph\.viewport\.deactivate\(\)`).MatchString(branch) {
		t.Errorf("D37o-toolbar-1: Context branch's deactivate() must be wrapped in a try/catch")
	}
	if !strings.Contains(branch, "swallow") {
		t.Errorf("D37o-toolbar-1: Context branch's deactivate() catch must explain the swallow")
	}
	// A short why-comment must accompany the fix so future tranches
	// don't accidentally rip it out. The comment marker "D37o-toolbar-1"
	// is enough to keep it discoverable.
	if !strings.Contains(branch, "D37o-toolbar-1") {
		t.Errorf("D37o-toolbar-1: Context branch must carry a `D37o-toolbar-1` comment marker explaining the restore")
	}
}

// ── B. Restore call is guarded to data-active-renderer="authority" ──

func TestExplorer_D37oToolbar1_ContextBranchGuardedToAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	branch := extractContextBranch(t, body)

	// The guard must consult getActiveRendererId() and compare to
	// 'authority'. This prevents accidental tear-down of the strategic
	// 'context' renderer when the operator clicks Context while
	// strategic is already active.
	if !strings.Contains(branch, "getActiveRendererId()") {
		t.Errorf("D37o-toolbar-1: Context branch must guard via getActiveRendererId()")
	}
	if !strings.Contains(branch, "=== 'authority'") {
		t.Errorf("D37o-toolbar-1: Context branch's guard must compare the active renderer id to 'authority'")
	}
	if !strings.Contains(branch, "typeof window.MIDASExplorerGraph.viewport.deactivate === 'function'") {
		t.Errorf("D37o-toolbar-1: Context branch must defensively check that viewport.deactivate is a function before calling it")
	}
}

// ── C. Context branch does not hard-code strategic activation ───────

func TestExplorer_D37oToolbar1_ContextBranchDoesNotForceStrategicActivation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	branch := extractContextBranch(t, body)

	// Under ?contextRenderer=strategic the strategic renderer's
	// MutationObserver on body[data-graph-lens] re-activates 'context'
	// itself. The toolbar branch must not duplicate that activation
	// here.
	if strings.Contains(branch, "activateById('context')") {
		t.Errorf("D37o-toolbar-1: Context branch must not hard-code activateById('context') — defer to the strategic renderer's body-lens observer")
	}
	if strings.Contains(branch, `activateById("context")`) {
		t.Errorf("D37o-toolbar-1: Context branch must not hard-code activateById(\"context\")")
	}
	// Authority refresh must not be invoked from the Context branch.
	if strings.Contains(branch, "authorityView.refresh") {
		t.Errorf("D37o-toolbar-1: Context branch must not call authorityView.refresh")
	}
	// Projection provider has its own lifecycle; the toolbar must not
	// poke it from the Context branch.
	if strings.Contains(branch, "contextProjectionProvider") {
		t.Errorf("D37o-toolbar-1: Context branch must not couple to contextProjectionProvider")
	}
	// Strategic activation gate (the query parameter handling) lives
	// only in context-cytoscape-renderer.js; the toolbar must not read
	// or write it.
	if strings.Contains(branch, "contextRenderer=strategic") {
		t.Errorf("D37o-toolbar-1: Context branch must not consult the strategic activation query parameter")
	}
}

// ── D. Authority branch still activates Authority ───────────────────

func TestExplorer_D37oToolbar1_AuthorityBranchStillActivatesAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	branch := extractAuthorityBranch(t, body)

	for _, want := range []string{
		"_showAuthorityPanels(true)",
		"_setWorkbenchModeActiveButton('authority')",
		"selectedGraphLens: 'authority'",
		"ExplorerGraph.shell.setActiveLens('authority')",
		"ExplorerGraph.authorityView.refresh",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D37o-toolbar-1: Authority branch must still contain %q (no removal of Authority activation)", want)
		}
	}
	// The fix must NOT have leaked a viewport.deactivate() call into
	// the Authority branch — Authority's own activate() handles the
	// previous-renderer teardown.
	if strings.Contains(branch, "MIDASExplorerGraph.viewport.deactivate()") {
		t.Errorf("D37o-toolbar-1: Authority branch must not contain viewport.deactivate() — Authority's own activate() handles teardown of the previous renderer")
	}
}

// ── E. Strategic Context renderer observer remains intact ───────────

func TestExplorer_D37oToolbar1_StrategicContextObserverStillIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	if !strings.Contains(js, "MutationObserver") {
		t.Errorf("D37o-toolbar-1: strategic renderer must still install a MutationObserver on body[data-graph-lens]")
	}
	if !strings.Contains(js, "data-graph-lens") {
		t.Errorf("D37o-toolbar-1: strategic renderer must still observe body[data-graph-lens]")
	}
	if !strings.Contains(js, "g.viewport.activateById(RENDERER_ID)") {
		t.Errorf("D37o-toolbar-1: strategic renderer must still call activateById(RENDERER_ID) from its observer")
	}
	// The strategic renderer's activation gate must still be the query
	// parameter — not flipped by this tranche.
	if !strings.Contains(js, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37o-toolbar-1: strategic activation query param must remain 'contextRenderer'")
	}
	if !strings.Contains(js, "var MODE_STRATEGIC = 'strategic';") {
		t.Errorf("D37o-toolbar-1: strategic activation mode token must remain 'strategic'")
	}
}

// ── F. Foundation preservation ──────────────────────────────────────

func TestExplorer_D37oToolbar1_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Drawer markup still present.
	if !strings.Contains(body, "gmap-details") {
		t.Errorf("D37o-toolbar-1: drawer markup (gmap-details) must remain present")
	}
	// Evidence tray markup still present.
	if !strings.Contains(body, "gmap-evidence-tray") {
		t.Errorf("D37o-toolbar-1: evidence tray markup (gmap-evidence-tray) must remain present")
	}
	// Authority canvas-edge wrapper still present.
	if !strings.Contains(body, "gmap-canvas-edge-tabs") {
		t.Errorf("D37o-toolbar-1: Authority canvas-edge wrapper (gmap-canvas-edge-tabs) must remain present")
	}
	// Selected-object pane wrapper still present.
	if !strings.Contains(body, "gmap-context-selected-object-pane") {
		t.Errorf("D37o-toolbar-1: Context Selected-Object Pane wrapper must remain present")
	}
	// Projection provider script still present.
	if !strings.Contains(body, "context-projection-provider.js") {
		t.Errorf("D37o-toolbar-1: contextProjectionProvider <script> must remain in load order")
	}
	// Default renderer behaviour unchanged: the strategic activation
	// gate is still opt-in only.
	if strings.Contains(body, "contextRenderer=legacy") && strings.Contains(body, "default-renderer-flip") {
		t.Errorf("D37o-toolbar-1: must not flip the default renderer in this tranche")
	}
	// No temporary renderer names introduced anywhere in index.html.
	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37o-toolbar-1: must not introduce temporary renderer name %q", banned)
		}
	}
}

// ── G. Authority's existing activation chain still intact ───────────

func TestExplorer_D37oToolbar1_AuthorityActivationChainIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	if !strings.Contains(js, "vp.activateById('authority')") {
		t.Errorf("D37o-toolbar-1: Authority Cytoscape module must still call viewport.activateById('authority')")
	}
}
