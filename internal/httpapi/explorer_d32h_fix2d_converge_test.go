package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// explorer_d32h_fix2d_converge_test.go — D32h-fix-2d-converge pins
// the Authority workbench's chrome convergence to Context's
// evidence-tray contract. The rewrite changed only declarations
// (height model, transition target, token names); every existing
// selector and DOM element is preserved.
//
// Reference for invariants:
//   - context-evidence-tray.js:1016-1017 (glyph swap)
//   - context-evidence-tray.js:1039-1043 (200ms re-fit timeout)
//   - governance-map.css:1246-1370 (Context tray chrome)
//   - docs/analysis/D32h-fix-2d-parity-letterbox-divergence.md

// TestExplorer_D32hFix2dConverge_AuthorityWorkbenchUsesHeightCollapseModel
// pins the height-driven collapse model (height: 36px collapsed,
// height: 320px expanded). Bans the pre-fix min-height / max-height
// shape that allowed content to drive height regardless of
// `.is-expanded` state.
func TestExplorer_D32hFix2dConverge_AuthorityWorkbenchUsesHeightCollapseModel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	// Required: height-driven collapse / expansion.
	required := []string{
		".gmap-authority-workbench {",
		"  height: 36px;",
		".gmap-authority-workbench.is-expanded {",
		"  height: 320px;",
	}
	for _, want := range required {
		if !strings.Contains(css, want) {
			t.Errorf("D32h-fix-2d-converge: height-driven collapse model missing in served CSS: %q", want)
		}
	}

	// Banned: the pre-fix height model. Scope the scan to rule
	// bodies only — the explanatory comment block in the new CSS
	// legitimately documents the deleted form, so a raw-text check
	// would false-positive on the comment text.
	commentRe := regexp.MustCompile(`(?s)/\*.*?\*/`)
	stripped := commentRe.ReplaceAllString(css, "")
	for _, banned := range []string{
		"min-height: 36px;\n  max-height: 280px;",
		"max-height: 280px;",
		"max-height: 320px;",
	} {
		if strings.Contains(stripped, banned) {
			t.Errorf("D32h-fix-2d-converge: pre-fix height model must be gone (rule body): %q", banned)
		}
	}
}

// TestExplorer_D32hFix2dConverge_AuthorityWorkbenchTransitionsHeight
// pins that the workbench root animates `height`, not `max-height`.
// Animating the wrong property is what hid the collapse defect for
// D32h-fix-1's lifecycle.
func TestExplorer_D32hFix2dConverge_AuthorityWorkbenchTransitionsHeight(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	if !strings.Contains(css, "transition: height 0.18s ease-out;") {
		t.Error("D32h-fix-2d-converge: workbench root must declare `transition: height 0.18s ease-out;` (matches Context tray at governance-map.css:1254)")
	}

	// Ban any `transition: max-height …` on a rule targeting
	// `.gmap-authority-workbench` (any class variant — root,
	// `.is-expanded`, etc.).
	commentRe := regexp.MustCompile(`(?s)/\*.*?\*/`)
	stripped := commentRe.ReplaceAllString(css, "")
	ruleRe := regexp.MustCompile(`([^{}]*)\{([^}]*)\}`)
	for _, m := range ruleRe.FindAllStringSubmatch(stripped, -1) {
		selector := m[1]
		body := m[2]
		if !strings.Contains(selector, "gmap-authority-workbench") {
			continue
		}
		if regexp.MustCompile(`transition\s*:\s*max-height`).MatchString(body) {
			t.Errorf("D32h-fix-2d-converge: workbench rule must not animate max-height — rule: %q", strings.TrimSpace(selector))
		}
	}
}

// TestExplorer_D32hFix2dConverge_AuthorityWorkbenchHeaderHasFixedHeight
// pins the explicit header height + flex-shrink:0. Without these,
// when the parent collapses to height:36px, the header (without a
// fixed height) could fail to consume the full 36px, leaving a
// truncated chrome bar.
func TestExplorer_D32hFix2dConverge_AuthorityWorkbenchHeaderHasFixedHeight(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	// Find the header rule and confirm height + flex-shrink.
	idx := strings.Index(css, ".gmap-authority-workbench-header {")
	if idx < 0 {
		t.Fatal("D32h-fix-2d-converge: .gmap-authority-workbench-header rule missing")
	}
	end := strings.Index(css[idx:], "}")
	if end < 0 {
		t.Fatal("D32h-fix-2d-converge: malformed header rule")
	}
	body := css[idx : idx+end+1]

	for _, want := range []string{
		"height: 36px;",
		"flex-shrink: 0;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32h-fix-2d-converge: header rule must declare %q to ensure the chrome bar fills the 36px collapsed height. Header rule body: %q", want, strings.TrimSpace(body))
		}
	}
}

// TestExplorer_D32hFix2dConverge_AuthorityWorkbenchToggleUpdatesGlyph
// pins the JS glyph-update branch added to _setExpanded. Mirrors
// Context's pattern at context-evidence-tray.js:1016-1017.
func TestExplorer_D32hFix2dConverge_AuthorityWorkbenchToggleUpdatesGlyph(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-workbench.js")

	for _, want := range []string{
		"btn.querySelector('span[aria-hidden]')",
		"glyph.textContent = _expanded ? '▼' : '▲';",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2d-converge: _setExpanded must update the toggle glyph — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2dConverge_AuthorityWorkbenchSchedulesRefitOnToggle
// pins the 200ms setTimeout in _setExpanded calling
// scheduleFitToView. Without the re-fit, expanding the workbench
// would clip nodes and leave dead canvas space because the
// canvas-scroll viewport shrinks.
func TestExplorer_D32hFix2dConverge_AuthorityWorkbenchSchedulesRefitOnToggle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-workbench.js")

	for _, want := range []string{
		"window.setTimeout(function ()",
		"window.MIDASExplorerGraph.camera",
		"camera.scheduleFitToView()",
		"200",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2d-converge: _setExpanded must schedule a 200ms scheduleFitToView — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2dConverge_AuthorityWorkbenchUsesSharedTokens
// pins that the Authority workbench CSS references the Material
// Design tokens defined in tokens.css. The migration table in the
// Step 0.1 sign-off lists the canonical pairs.
func TestExplorer_D32hFix2dConverge_AuthorityWorkbenchUsesSharedTokens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	for _, want := range []string{
		"var(--surface-container-low)",
		"var(--surface-container-lowest)",
		"var(--surface-container)",
		"var(--outline-variant)",
		"var(--on-surface)",
		"var(--slate-300)",
		"var(--slate-400)",
		"var(--font-display)",
		"var(--primary)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32h-fix-2d-converge: Authority workbench CSS must reference shared Material token %q", want)
		}
	}

	// Severity / status colours use convention-with-fallback tokens
	// matching drift-analytics.css and the rest of the explorer.
	for _, want := range []string{
		"var(--badge-bad, #f87171)",
		"var(--badge-bad, #ef4444)",
		"var(--badge-warn, #fbbf24)",
		"var(--badge-warn, #f59e0b)",
		"var(--badge-info, #60a5fa)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D32h-fix-2d-converge: Authority workbench severity colour must use convention token %q", want)
		}
	}
}

// TestExplorer_D32hFix2dConverge_AuthorityWorkbenchHardcodedFallbacksRemoved
// pins that no Authority workbench rule references the pre-fix
// undefined-token names (--panel-bg, --border-color, --text-primary,
// --text-muted, --accent-emphasis, --accent-gap, --severity-*).
// These tokens were never defined in tokens.css; only their
// hard-coded fallbacks fired, so theme switching could not affect
// the workbench.
func TestExplorer_D32hFix2dConverge_AuthorityWorkbenchHardcodedFallbacksRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	// Find the workbench CSS block (from .gmap-authority-workbench
	// through the last .authority-workbench-* rule). We bound the
	// scan to this region so the pre-fix banned-token check does not
	// accidentally hit unrelated authority-graph.css content (e.g.
	// .authority-badge-* rules which legitimately use --badge-*
	// tokens but may also use --text-2 etc.).
	startIdx := strings.Index(css, ".gmap-authority-workbench {")
	if startIdx < 0 {
		t.Fatal("D32h-fix-2d-converge: .gmap-authority-workbench root rule missing")
	}
	endIdx := strings.LastIndex(css, ".authority-workbench-diagnostic-message {")
	if endIdx < 0 {
		t.Fatal("D32h-fix-2d-converge: .authority-workbench-diagnostic-message rule missing (expected last content rule)")
	}
	// Extend to the closing brace of that final rule.
	endIdx = endIdx + strings.Index(css[endIdx:], "}") + 1
	block := css[startIdx:endIdx]

	for _, banned := range []string{
		"var(--panel-bg",
		"var(--panel-bg-strong",
		"var(--border-color",
		"var(--text-primary",
		"var(--text-muted",
		"var(--accent-emphasis",
		"var(--accent-gap",
		"var(--severity-critical",
		"var(--severity-warning",
		"var(--severity-info",
	} {
		if strings.Contains(block, banned) {
			t.Errorf("D32h-fix-2d-converge: pre-fix undefined-token reference must be migrated: %q", banned)
		}
	}
}

// TestExplorer_D32hFix2dConverge_ContextLetterboxUntouched pins that
// Context's evidence-tray contract is unchanged. The CSS rules at
// governance-map.css:1246-1265 (collapsed height, transition,
// expanded height) and the Context-tray DOM in index.html remain
// byte-identical.
func TestExplorer_D32hFix2dConverge_ContextLetterboxUntouched(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	contextCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	for _, want := range []string{
		".gmap-evidence-tray {",
		"height: 36px;",
		"transition: height 0.18s ease-out;",
		".gmap-evidence-tray.is-expanded {",
		"height: 320px;",
	} {
		if !strings.Contains(contextCSS, want) {
			t.Errorf("D32h-fix-2d-converge: Context tray CSS contract must remain — missing %q in governance-map.css", want)
		}
	}

	contextJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-evidence-tray.js")
	for _, want := range []string{
		"gmapEvidenceTrayExpanded = !gmapEvidenceTrayExpanded;",
		"applyGmapEvidenceTrayState();",
		"notifySelectionChanged:  notifySelectionChanged,",
	} {
		if !strings.Contains(contextJS, want) {
			t.Errorf("D32h-fix-2d-converge: Context tray module contract must remain — missing %q", want)
		}
	}
}
