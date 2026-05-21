package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37d-authority-cytoscape-mount-visibility-fix
//
// Closes the verified D37c blank-state root cause: the Authority
// Cytoscape mount was `position: relative; width: 100%; height: 100%;`
// and ended up stacking BELOW its `.governance-map-canvas-scroll`
// sibling inside `.midas-graph-renderer-slot`, clipped invisible by
// `.midas-graph-viewport { overflow: hidden }`.
//
// D37d's fix: change the active Authority mount rule to
// `position: absolute; inset: 0;` so the mount overlays the slot
// (the slot is itself `position: absolute; inset: 0;`) and fills
// the visible viewport exactly. `overflow: hidden` is retained on
// the mount as Cytoscape canvas-discipline (D35h contract). The
// strategic clip remains `.midas-graph-viewport { overflow:
// hidden }`.
//
// Optional defensive secondary fix: `_renderUnavailable(mount,
// message)` now guards `if (!mount) return;` so a null mount (when
// `_ensureMount()` fails) no longer throws `TypeError`.
//
// Tests below pin the CSS shape, the negative anti-regression
// (no `position: relative` re-emergence), the retained overflow
// discipline, the defensive null guard, the GraphViewport clip
// contract, and the D37b strategic-state preservation.

const (
	d37dAuthorityCSSPath    = "/explorer/assets/css/authority-cytoscape-poc.css"
	d37dAuthorityShellAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37dHostAssetPath       = "/explorer/assets/js/graph/graph-viewport.js"
	d37dMountSelector       = `.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-mount`
)

// readActiveAuthorityMountRule reads the executable (comment-
// stripped) Authority CSS, locates the `.cytoscape-poc-mount` rule
// scoped under the active-renderer selector, and returns the
// declaration block (the substring between the rule's opening `{`
// and the matching `}`). Fatal if the rule is missing.
func readActiveAuthorityMountRule(t *testing.T, srv *Server) string {
	t.Helper()
	css := getExplorerAsset(t, srv, d37dAuthorityCSSPath)
	exec := stripCSSComments(css)
	idx := strings.Index(exec, d37dMountSelector)
	if idx < 0 {
		t.Fatalf("D37d: active Authority mount selector %q must exist in the executable CSS", d37dMountSelector)
	}
	openBrace := strings.Index(exec[idx:], "{")
	if openBrace < 0 {
		t.Fatalf("D37d: cannot find `{` after %q", d37dMountSelector)
	}
	closeBrace := strings.Index(exec[idx+openBrace:], "}")
	if closeBrace < 0 {
		t.Fatalf("D37d: cannot find matching `}` for %q", d37dMountSelector)
	}
	return exec[idx+openBrace+1 : idx+openBrace+closeBrace]
}

// TestExplorer_D37dAuthorityMount_PositionsAbsoluteInsideSlot pins
// the primary CSS fix: the active Authority mount declares
// `position: absolute;` and `inset: 0;` so it overlays the slot
// rather than stacking below the legacy `.governance-map-canvas-
// scroll` sibling.
func TestExplorer_D37dAuthorityMount_PositionsAbsoluteInsideSlot(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := readActiveAuthorityMountRule(t, srv)

	if !strings.Contains(body, "position: absolute;") {
		t.Errorf("D37d: active Authority mount must declare `position: absolute;` — rule body was:\n%s", body)
	}
	if !strings.Contains(body, "inset: 0;") {
		t.Errorf("D37d: active Authority mount must declare `inset: 0;` so it anchors to the slot's four edges — rule body was:\n%s", body)
	}
}

// TestExplorer_D37dAuthorityMount_DoesNotStackBelowCanvasScroll
// pins the negative anti-regression: the active Authority mount
// rule must NOT re-introduce `position: relative;` or `height:
// 100%;` — those were the pre-D37d declarations that caused the
// sibling-stacking layout collision.
func TestExplorer_D37dAuthorityMount_DoesNotStackBelowCanvasScroll(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := readActiveAuthorityMountRule(t, srv)

	if strings.Contains(body, "position: relative;") {
		t.Errorf("D37d: active Authority mount must NOT declare `position: relative;` — this caused the pre-D37d sibling-stacking blank-state. Rule body was:\n%s", body)
	}
	// height:100% is redundant once inset:0 anchors the mount and
	// would re-introduce the same containing-block height confusion
	// the fix is meant to eliminate.
	if strings.Contains(body, "height: 100%;") {
		t.Errorf("D37d: active Authority mount must NOT re-introduce `height: 100%%;` — `inset: 0;` already anchors the mount. Rule body was:\n%s", body)
	}
	if strings.Contains(body, "width: 100%;") {
		t.Errorf("D37d: active Authority mount must NOT re-introduce `width: 100%%;` — `inset: 0;` already anchors the mount. Rule body was:\n%s", body)
	}
}

// TestExplorer_D37dAuthorityMount_RetainsCytoscapeOverflowDiscipline
// pins that the mount's own `overflow: hidden` is preserved as
// Cytoscape canvas-discipline (D35h contract). The strategic clip
// remains `.midas-graph-viewport { overflow: hidden }`.
func TestExplorer_D37dAuthorityMount_RetainsCytoscapeOverflowDiscipline(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := readActiveAuthorityMountRule(t, srv)

	if !strings.Contains(body, "overflow: hidden;") {
		t.Errorf("D37d: active Authority mount must retain `overflow: hidden;` as Cytoscape canvas-discipline (D35h contract). Rule body was:\n%s", body)
	}
}

// TestExplorer_D37dAuthorityMount_GraphViewportClipContractPreserved
// pins the foundation invariants the fix does not touch:
//   • `.midas-graph-viewport { overflow: hidden }` remains the
//     strategic clip authority.
//   • `.context-cy-spike-overlay` remains non-clipping.
//   • Authority still registers + activates under the renderer id
//     `'authority'`.
//   • CSS still keys off `data-active-renderer="authority"`.
func TestExplorer_D37dAuthorityMount_GraphViewportClipContractPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Strategic clip rule lives in governance-map.css.
	clipCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	clipExec := stripCSSComments(clipCSS)
	if !strings.Contains(clipExec, ".midas-graph-viewport") ||
		!strings.Contains(clipExec, "overflow: hidden") {
		t.Error("D37d: `.midas-graph-viewport { overflow: hidden }` strategic clip must remain (D35f)")
	}

	// Context Cytoscape overlay remains non-clipping (exactly 1
	// `overflow: hidden` in the spike CSS — on the mount, not the
	// overlay).
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if strings.Count(stripCSSComments(spikeCSS), "overflow: hidden") != 1 {
		t.Error("D37d: spike CSS must have exactly 1 `overflow: hidden` (mount; overlay non-clipping)")
	}

	// Authority still registered + activated under the production id.
	shellJS := getExplorerAsset(t, srv, d37dAuthorityShellAsset)
	for _, want := range []string{
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
	} {
		if !strings.Contains(shellJS, want) {
			t.Errorf("D37d: D37b Authority GraphViewport contract %q must remain", want)
		}
	}

	// CSS selectors still key off data-active-renderer="authority".
	if !strings.Contains(stripCSSComments(getExplorerAsset(t, srv, d37dAuthorityCSSPath)),
		`data-active-renderer="authority"`) {
		t.Error("D37d: Authority CSS must continue to key off `data-active-renderer=\"authority\"`")
	}
}

// TestExplorer_D37dAuthority_RenderUnavailableNullSafe pins the
// optional defensive secondary fix: `_renderUnavailable(mount,
// message)` guards `if (!mount) return;` so a null mount cannot
// throw `TypeError`.
func TestExplorer_D37dAuthority_RenderUnavailableNullSafe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37dAuthorityShellAsset)

	// Bound the assertions to the _renderUnavailable body so an
	// unrelated `if (!mount) return;` elsewhere doesn't false-match.
	defIdx := strings.Index(js, "function _renderUnavailable(mount, message)")
	if defIdx < 0 {
		t.Fatal("D37d: _renderUnavailable definition not found")
	}
	endIdx := strings.Index(js[defIdx:], "\n  }")
	if endIdx < 0 {
		t.Fatal("D37d: cannot bound _renderUnavailable body")
	}
	body := js[defIdx : defIdx+endIdx]

	if !strings.Contains(body, "if (!mount) return;") {
		t.Errorf("D37d: _renderUnavailable must guard `if (!mount) return;` to avoid TypeError when _ensureMount() returns null. Body was:\n%s", body)
	}
	// The null guard must appear BEFORE the first mount.appendChild
	// (or any other mount.<member> access). Strip comments first so
	// the explanatory docblock (which legitimately mentions
	// `mount.appendChild(...)` when describing the pre-D37d bug)
	// does not trip the order check.
	execBody := stripJSComments(body)
	guardIdx := strings.Index(execBody, "if (!mount) return;")
	appendIdx := strings.Index(execBody, "mount.appendChild(")
	if guardIdx < 0 || appendIdx < 0 || guardIdx > appendIdx {
		t.Errorf("D37d: `if (!mount) return;` guard must appear BEFORE `mount.appendChild(...)` in executable code (guardIdx=%d, appendIdx=%d)", guardIdx, appendIdx)
	}
}

// TestExplorer_D37d_D37bContractsPreserved is the foundation-wide
// regression check. Every D37b invariant must remain intact after
// the D37d mount-visibility fix.
func TestExplorer_D37d_D37bContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	hostJS := getExplorerAsset(t, srv, d37dHostAssetPath)
	hostExec := stripJSComments(hostJS)

	// graph-viewport.js remains renderer-neutral in executable code.
	for _, banned := range []string{
		"'authority'",
		`"authority"`,
		"'authority-cytoscape'",
		`"authority-cytoscape"`,
	} {
		if strings.Contains(hostExec, banned) {
			t.Errorf("D37d: graph-viewport.js executable code must remain renderer-neutral — found %q", banned)
		}
	}
	// Host generic surface intact.
	for _, want := range []string{
		"function register(rendererId, factory)",
		"function activateById(rendererId)",
		"function _setActiveRendererAttribute(rendererId)",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D37d: generic GraphViewport host surface %q must remain", want)
		}
	}

	// D37b Authority production renderer id + activation + aria-label + bridge.
	shellJS := getExplorerAsset(t, srv, d37dAuthorityShellAsset)
	for _, want := range []string{
		// D37b — production renderer id.
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
		// D37b — production aria-label (no PoC suffix).
		`_mountEl.setAttribute('aria-label', 'Authority Graph')`,
		// D37b — Diagnostics-panel / Surface-posture-panel / Workbench bridge.
		"window.MIDASExplorerGraph.authorityDiagnosticsPanel",
		"diagPanel.render(payload)",
		"window.MIDASExplorerGraph.authoritySurfacePosturePanel",
		"posturePanel.render(payload)",
		"window.MIDASExplorerGraph.authorityWorkbench",
		"workbenchMod.render()",
		// D37b — Cached projection so the workbench (cache-driven) sees
		// the same payload the Cytoscape renderer drew from.
		"window.MIDASExplorerGraph._lastAuthorityProjection = payload",
	} {
		if !strings.Contains(shellJS, want) {
			t.Errorf("D37d: D37b Authority contract %q must remain", want)
		}
	}

	// Negative: pre-D37b user-facing PoC aria-label must remain
	// retired.
	if strings.Contains(shellJS, `'Authority Graph (Cytoscape PoC)'`) ||
		strings.Contains(shellJS, `"Authority Graph (Cytoscape PoC)"`) {
		t.Error("D37d: pre-D37b aria-label `Authority Graph (Cytoscape PoC)` must remain retired")
	}

	// D36a — Knowledge shell unaffected.
	knJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js")
	for _, want := range []string{
		"vp.register('knowledge-graph', _knowledgeGraphRendererFactory)",
		"vp.activateById(RENDERER_ID)",
	} {
		if !strings.Contains(knJS, want) {
			t.Errorf("D37d: D36a Knowledge shell contract %q must remain", want)
		}
	}

	// D35e — Context Cytoscape spike unaffected.
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	for _, want := range []string{
		"vp.register('context-cytoscape', _contextCytoscapeRendererFactory)",
		"vp.activateById('context-cytoscape')",
	} {
		if !strings.Contains(ctxJS, want) {
			t.Errorf("D37d: D35e Context spike contract %q must remain", want)
		}
	}
}
