package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37j-authority-cytoscape-client-side-authority-context-view
//
// Implements "View authority context" — a client-side projection-style
// filter over the currently loaded Authority Cytoscape graph. Operator
// selects a supported governance object and clicks the new toolbar
// control; the renderer hides every cy element outside the directed-
// traversal authority context of the focal node (predecessors ∪
// successors ∪ self + the BS-default fail-mode-policy edge for any
// business_service ancestor). The HTML card overlay mirrors visibility
// via the D37j `n.visible()` check in `_syncCards`. Exit restores all
// elements.
//
// D37j is **client-side only**. No backend fetch. No element removal.
// No projection re-rooting. No schema / OpenAPI / handler changes.
//
// Eligible focal kinds in D37j:
//   • decision_surface
//   • authority_profile
//   • authority_grant
//
// Disabled in D37j (deferred to D38a/b/c backend re-rooting):
//   • business_service (already the default root)
//   • agent (cross-BS)
//   • fail_mode_policy (cross-BS)
//   • escalation_target (cross-BS)

const (
	d37jAuthorityShellAsset   = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37jAuthorityToolbarAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js"
	d37jAuthorityCSSPath      = "/explorer/assets/css/authority-cytoscape-poc.css"
)

// readD37jFunctionBody bounds assertions to a single function body so
// negative pins elsewhere in the file don't false-match.
func readD37jFunctionBody(t *testing.T, js, signature string) string {
	t.Helper()
	start := strings.Index(js, signature)
	if start < 0 {
		t.Fatalf("D37j: function %q not found", signature)
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatalf("D37j: cannot bound function %q", signature)
	}
	return js[start : start+end]
}

// TestExplorer_D37j_AuthorityContextButtonPresent pins the new
// toolbar control exists with the documented IDs/labels/state.
func TestExplorer_D37j_AuthorityContextButtonPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-authority-context-button"`,
		`aria-label="View authority context"`,
		`title="View authority context"`,
		`aria-pressed="false"`,
		`aria-disabled="true"`,
		`disabled`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37j: authority-context button must carry %q", want)
		}
	}
}

// TestExplorer_D37j_AuthorityContextButtonInsideCluster pins that
// the new button lives inside `.gmap-camera-cluster` (no parallel
// toolbar).
func TestExplorer_D37j_AuthorityContextButtonInsideCluster(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	clusterStart := strings.Index(body, `class="gmap-camera-cluster"`)
	if clusterStart < 0 {
		t.Fatal("D37j: `.gmap-camera-cluster` wrapper missing")
	}
	clusterEnd := strings.Index(body[clusterStart:], `</div>`)
	if clusterEnd < 0 {
		t.Fatal("D37j: cannot bound `.gmap-camera-cluster`")
	}
	cluster := body[clusterStart : clusterStart+clusterEnd]

	if !strings.Contains(cluster, `id="gmap-authority-context-button"`) {
		t.Errorf("D37j: authority-context button must live inside `.gmap-camera-cluster` — cluster was:\n%s", cluster)
	}
}

// TestExplorer_D37j_NoParallelToolbar pins that no new toolbar
// cluster was introduced.
func TestExplorer_D37j_NoParallelToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	// Exactly one `.gmap-camera-cluster` exists.
	count := strings.Count(body, `class="gmap-camera-cluster"`)
	if count != 1 {
		t.Errorf("D37j: expected exactly one `.gmap-camera-cluster` cluster in markup, found %d", count)
	}
}

// TestExplorer_D37j_RendererExposesContextApi pins the new public
// surface on `cytoscapePoc`.
func TestExplorer_D37j_RendererExposesContextApi(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)

	for _, want := range []string{
		// Public-surface keys.
		"viewAuthorityContext:",
		"exitAuthorityContext:",
		"toggleAuthorityContext:",
		"isAuthorityContextActive:",
		"canViewAuthorityContext:",
		"onAuthorityContextChanged:",
		// Implementation helpers.
		"function _viewAuthorityContext()",
		"function _exitAuthorityContext()",
		"function _toggleAuthorityContext()",
		"function _isAuthorityContextActive()",
		"function _canViewAuthorityContext()",
		"function _computeAuthorityContext(cy, node)",
		"function _onAuthorityContextChanged(handler)",
		"function _checkAutoExitContext()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37j: renderer must expose %q", want)
		}
	}
}

// TestExplorer_D37j_EligibleKindsAreExactlyThree pins the single
// source of truth for which focal kinds are enabled in D37j. The
// brief is explicit that Agent, Fail-Mode Policy, and Escalation
// Target must NOT be client-side enabled (they require backend
// re-rooting).
func TestExplorer_D37j_EligibleKindsAreExactlyThree(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)

	// Bound to the _AUTHORITY_CONTEXT_ELIGIBLE_KINDS declaration.
	start := strings.Index(js, "var _AUTHORITY_CONTEXT_ELIGIBLE_KINDS = {")
	if start < 0 {
		t.Fatal("D37j: _AUTHORITY_CONTEXT_ELIGIBLE_KINDS not found")
	}
	end := strings.Index(js[start:], "};")
	if end < 0 {
		t.Fatal("D37j: cannot bound _AUTHORITY_CONTEXT_ELIGIBLE_KINDS")
	}
	block := js[start : start+end]

	// Required entries (the three supported focal kinds).
	for _, want := range []string{
		"decision_surface:",
		"authority_profile:",
		"authority_grant:",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("D37j: eligible-kinds map must include %q", want)
		}
	}

	// Forbidden entries (deferred to backend re-rooting).
	for _, banned := range []string{
		"business_service:",
		"agent:",
		"fail_mode_policy:",
		"escalation_target:",
	} {
		if strings.Contains(block, banned) {
			t.Errorf("D37j: eligible-kinds map must NOT enable %q (deferred to D38 backend re-rooting)", banned)
		}
	}
}

// TestExplorer_D37j_ComputeContextUsesPredecessorsAndSuccessors pins
// the Cytoscape traversal pattern.
func TestExplorer_D37j_ComputeContextUsesPredecessorsAndSuccessors(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	body := readD37jFunctionBody(t, js, "function _computeAuthorityContext(cy, node)")

	for _, want := range []string{
		"node.predecessors()",
		"node.successors()",
		".union(node)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37j: _computeAuthorityContext must use %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37j_ContextDoesNotCallBackendFetch is the load-bearing
// negative pin: D37j is purely client-side.
func TestExplorer_D37j_ContextDoesNotCallBackendFetch(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	exec := stripJSComments(js)

	// Inspect the three context-view functions.
	for _, fnSig := range []string{
		"function _viewAuthorityContext()",
		"function _exitAuthorityContext()",
		"function _toggleAuthorityContext()",
	} {
		body := readD37jFunctionBody(t, exec, fnSig)
		for _, banned := range []string{
			"adapter.fetch",
			"/v1/graphs/authority",
			"view: 'decision_surface'",
			"view: 'agent'",
			"view: 'authority_profile'",
			"view: 'fail_mode_policy'",
			"fetch(",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D37j: %s must NOT call %q — body:\n%s", fnSig, banned, body)
			}
		}
	}
}

// TestExplorer_D37j_ContextDoesNotRemoveElements pins that exit /
// view use `.show()` / `.hide()` only, never `.remove()`. Removing
// elements is destructive and would force a full re-render.
func TestExplorer_D37j_ContextDoesNotRemoveElements(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	exec := stripJSComments(js)

	for _, fnSig := range []string{
		"function _viewAuthorityContext()",
		"function _exitAuthorityContext()",
	} {
		body := readD37jFunctionBody(t, exec, fnSig)
		if strings.Contains(body, ".remove()") {
			t.Errorf("D37j: %s must NOT call `.remove()` — body:\n%s", fnSig, body)
		}
	}
}

// TestExplorer_D37j_ViewHidesNonFocus pins the view-entry contract:
// hide non-focus elements + sync cards + fit the visible collection.
func TestExplorer_D37j_ViewHidesNonFocus(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	body := readD37jFunctionBody(t, js, "function _viewAuthorityContext()")

	for _, want := range []string{
		"_computeAuthorityContext(_cy, node)",
		"_cy.elements().difference(focus)",
		"nonFocus.hide()",
		"_syncCards()",
		"_fitToAvailableCanvas(_cy)",
		"_authorityContextActive      = true",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37j: _viewAuthorityContext must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37j_ExitRestoresAllElements pins the exit contract.
func TestExplorer_D37j_ExitRestoresAllElements(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	body := readD37jFunctionBody(t, js, "function _exitAuthorityContext()")

	for _, want := range []string{
		"cy.elements().show()",
		"_authorityContextActive      = false",
		"_syncCards()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37j: _exitAuthorityContext must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37j_CardSyncMirrorsCytoscapeVisibility pins the
// D37j extension to `_syncCards`: hidden cy nodes get a hidden card.
func TestExplorer_D37j_CardSyncMirrorsCytoscapeVisibility(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	body := readD37jFunctionBody(t, js, "function _syncCards()")

	for _, want := range []string{
		"n.visible",
		"!n.visible()",
		"card.style.display = 'none'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37j: _syncCards must mirror cy visibility onto cards — missing %q\nbody:\n%s", want, body)
		}
	}
}

// TestExplorer_D37j_ResetViewExitsContext pins that the reset-view
// camera button exits any active context view first.
func TestExplorer_D37j_ResetViewExitsContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	body := readD37jFunctionBody(t, js, "function _resetView(cy)")

	if !strings.Contains(body, "_exitAuthorityContext()") {
		t.Errorf("D37j: _resetView must call _exitAuthorityContext() — body:\n%s", body)
	}
}

// TestExplorer_D37j_CentreOnRootExitsContext pins that the legacy
// centre-button camera operation exits context first.
func TestExplorer_D37j_CentreOnRootExitsContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	body := readD37jFunctionBody(t, js, "function _centerOnRoot(cy)")

	if !strings.Contains(body, "_exitAuthorityContext()") {
		t.Errorf("D37j: _centerOnRoot must call _exitAuthorityContext() — body:\n%s", body)
	}
}

// TestExplorer_D37j_AutoExitOnSelectionChange pins the `select` event
// auto-exit handler (Option A from the brief).
func TestExplorer_D37j_AutoExitOnSelectionChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)

	// _wireInteractions must bind `cy.on('select', 'node', ...)` that
	// calls _checkAutoExitContext.
	wireBody := readD37jFunctionBody(t, js, "function _wireInteractions()")
	if !strings.Contains(wireBody, "_cy.on('select', 'node'") {
		t.Errorf("D37j: _wireInteractions must bind `_cy.on('select', 'node', ...)` for auto-exit — body:\n%s", wireBody)
	}
	if !strings.Contains(wireBody, "_checkAutoExitContext()") {
		t.Errorf("D37j: _wireInteractions must call _checkAutoExitContext() in the select handler — body:\n%s", wireBody)
	}

	// _checkAutoExitContext only exits on a different node — not on
	// empty selection or re-select of the focal node.
	checkBody := readD37jFunctionBody(t, js, "function _checkAutoExitContext()")
	for _, want := range []string{
		"_authorityContextActive",
		"cy.elements(':selected')",
		"_authorityContextFocalNodeId",
		"_exitAuthorityContext()",
	} {
		if !strings.Contains(checkBody, want) {
			t.Errorf("D37j: _checkAutoExitContext must include %q — body:\n%s", want, checkBody)
		}
	}
}

// TestExplorer_D37j_ToolbarBindsContextButton pins that the toolbar
// bridge's binding list includes the new button id with the new
// handler.
func TestExplorer_D37j_ToolbarBindsContextButton(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityToolbarAsset)

	for _, want := range []string{
		"'gmap-authority-context-button'",
		"_onAuthorityContext",
		"poc.toggleAuthorityContext()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37j: toolbar bridge must include %q", want)
		}
	}
}

// TestExplorer_D37j_ToolbarSyncReadsRendererState pins that the
// toolbar button state-sync reads from the renderer's public API
// (`canViewAuthorityContext` + `isAuthorityContextActive`), not from
// DOM heuristics.
func TestExplorer_D37j_ToolbarSyncReadsRendererState(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityToolbarAsset)
	body := readD37jFunctionBody(t, js, "function _syncAuthorityContextButton()")

	for _, want := range []string{
		"poc.isAuthorityContextActive",
		"poc.canViewAuthorityContext",
		"View authority context",
		"Exit authority context",
		"aria-pressed",
		"setAttribute('disabled'",
		"removeAttribute('disabled')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37j: _syncAuthorityContextButton must include %q — body:\n%s", want, body)
		}
	}
}

// TestExplorer_D37j_ToolbarSubscribesToContextChange pins that the
// toolbar installs a subscription against the renderer's context-
// change publisher, so the button state updates without polling.
func TestExplorer_D37j_ToolbarSubscribesToContextChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityToolbarAsset)

	for _, want := range []string{
		"function _ensureAuthorityContextSubscription()",
		"poc.onAuthorityContextChanged",
		"_syncAuthorityContextButton()",
		"_ensureAuthorityContextSubscription()",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37j: toolbar context-change subscription must include %q", want)
		}
	}

	// The selection-change subscription must also update the
	// context button (a selection change can flip eligibility).
	selStart := strings.Index(js, "function _ensureSelectionSubscription()")
	if selStart < 0 {
		t.Fatal("D37j: _ensureSelectionSubscription not found")
	}
	selEnd := strings.Index(js[selStart:], "\n  }\n")
	if selEnd < 0 {
		t.Fatal("D37j: cannot bound _ensureSelectionSubscription")
	}
	selBody := js[selStart : selStart+selEnd]
	if !strings.Contains(selBody, "_syncAuthorityContextButton()") {
		t.Errorf("D37j: _ensureSelectionSubscription must also re-sync the authority-context button — body:\n%s", selBody)
	}
}

// TestExplorer_D37j_IconIsActionMetaphorNotNodeKindIcon pins that
// the new toolbar icon does not reuse Authority node-kind icon hooks
// or `data-kind`. The D37h icon policy: toolbar/action icons and
// graph/node identity icons stay strictly separate.
func TestExplorer_D37j_IconIsActionMetaphorNotNodeKindIcon(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	clusterStart := strings.Index(body, `class="gmap-camera-cluster"`)
	if clusterStart < 0 {
		t.Fatal("D37j: `.gmap-camera-cluster` wrapper missing")
	}
	clusterEnd := strings.Index(body[clusterStart:], `</div>`)
	if clusterEnd < 0 {
		t.Fatal("D37j: cannot bound `.gmap-camera-cluster`")
	}
	cluster := body[clusterStart : clusterStart+clusterEnd]

	for _, banned := range []string{
		"authority-html-card-icon",
		"data-kind=",
		"authorityBusinessService",
		"authorityProfile",
		"authorityGrant",
		"authorityAgent",
		"authorityFailModePolicy",
		"authorityDecisionSurface",
		"authorityEscalationTarget",
	} {
		if strings.Contains(cluster, banned) {
			t.Errorf("D37j: authority-context button must not reuse Authority node-kind hook %q in the camera cluster", banned)
		}
	}
}

// TestExplorer_D37j_NoD37iScopeCreep is the scope-creep guard.
func TestExplorer_D37j_NoD37iScopeCreep(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	exec := stripJSComments(js)

	// Bound the scope check to the D37j context-view functions and
	// the binding list — banned features must not appear here.
	// Whole-file negative pins would over-trigger because the
	// renderer naturally references some of these in unrelated paths
	// (the `_wireInteractions` body sets `boxSelectionEnabled: false`
	// at cy init for instance — that's fine).
	for _, fnSig := range []string{
		"function _viewAuthorityContext()",
		"function _exitAuthorityContext()",
		"function _toggleAuthorityContext()",
		"function _computeAuthorityContext(cy, node)",
		"function _checkAutoExitContext()",
	} {
		body := readD37jFunctionBody(t, exec, fnSig)
		for _, banned := range []string{
			"boxSelectionEnabled(true",
			"selectionType('additive')",
			"on('cxttap'",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D37j: scope creep — D37i/D37k feature %q must not appear in %s", banned, fnSig)
			}
		}
	}
}

// TestExplorer_D37j_NoApiOrSchemaChange pins the brief's
// "no backend changes" boundary.
func TestExplorer_D37j_NoApiOrSchemaChange(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// The handler at /v1/graphs/authority still requires `view=service`.
	// (Verifying the handler didn't acquire new views.)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)
	if !strings.Contains(js, "adapter.fetch({ view: 'service', id: rootId") {
		t.Error("D37j: frontend caller must still use `view: 'service'` (no client-side `view=decision_surface` etc.)")
	}
}

// TestExplorer_D37j_PreservesD37hContracts is the foundation
// preservation check.
func TestExplorer_D37j_PreservesD37hContracts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37jAuthorityShellAsset)

	for _, want := range []string{
		// D37h API.
		"zoomToSelected:",
		"resetView:",
		"getZoomPercent:",
		"onViewportChanged:",
		"onSelectionChanged:",
		// D37h dbltap binding.
		"_cy.on('dbltap', 'node'",
		// D37f two-tier transform constants.
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
		// D37f rich-card icon plumbing.
		"_AUTHORITY_KIND_ICON_KEYS",
		// D37b registration.
		"vp.register('authority', _authorityRendererFactory)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37j: foundation contract %q must remain", want)
		}
	}
}

// TestExplorer_D37j_PreservesExistingToolbarControls pins that none
// of the prior D37h controls were renamed or removed.
func TestExplorer_D37j_PreservesExistingToolbarControls(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	for _, want := range []string{
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
		`id="gmap-zoom-percent"`,
		`id="gmap-zoom-selected-button"`,
		`id="gmap-reset-view-button"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37j: pre-existing D37h control %q must remain", want)
		}
	}
}
