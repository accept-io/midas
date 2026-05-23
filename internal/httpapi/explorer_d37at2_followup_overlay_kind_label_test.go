package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d37at2_followup_overlay_kind_label_test.go —
// D37at2-followup-overlay-consolidation-impl.
//
// This tranche consolidates the Authority HTML overlay's kind-label
// source. Pre-tranche, cytoscape-html-overlay.js owned a hard-coded
// local _nodeTypeLabel switch (Table 4 in the D37at2-followup
// inventory) with an UPPERCASE variant and stale Context-overlap
// cases. Post-tranche, the overlay resolves its eyebrow / aria-label
// vocabulary through window.MIDASExplorerGraph.authorityAdapter
// .nodeKindLabel(kind) and applies .toUpperCase() as a presentation
// transform at the call site. The unreachable Context-overlap cases
// (capability, process, ai_system) were removed; Authority's
// FORBIDDEN_CONTEXT_NODE_KINDS contract makes them unreachable
// through this overlay's runtime path.
//
// The overlay remains Authority-only:
//
//   • install / destroy are still called only from
//     authority-cytoscape-poc.js;
//   • the shared overlay mount still hard-codes lensId: 'authority';
//   • Context Graph view still has no reference to the overlay.
//
// CWD at test time is internal/httpapi.

const (
	d37at2FollowupOverlayHtmlOverlayPath = "explorer/assets/js/graph/authority/cytoscape-html-overlay.js"
	d37at2FollowupOverlayPocPath         = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37at2FollowupOverlayContextViewPath = "explorer/assets/js/graph/context/context-graph-view.js"
)

func d37at2FollowupOverlayReadOverlay(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d37at2FollowupOverlayHtmlOverlayPath)
	if err != nil {
		t.Fatalf("D37at2-followup-overlay: cannot read overlay at %s: %v", d37at2FollowupOverlayHtmlOverlayPath, err)
	}
	return string(b)
}

// d37at2FollowupOverlayBoundFn bounds a JS function body in a
// 2-space-indented IIFE source by locating its opening declaration
// and slicing to the next "\n  }" closing brace. Returns the body
// substring including the declaration line and the closing brace.
func d37at2FollowupOverlayBoundFn(t *testing.T, js, decl string) string {
	t.Helper()
	start := strings.Index(js, decl)
	if start < 0 {
		t.Fatalf("D37at2-followup-overlay: function declaration %q missing", decl)
	}
	tail := js[start:]
	endRel := strings.Index(tail, "\n  }")
	if endRel < 0 {
		t.Fatalf("D37at2-followup-overlay: could not bound function body starting at %q", decl)
	}
	return tail[:endRel+4]
}

// ── 1. Overlay label resolver uses the Authority adapter ─────────────

// TestExplorer_D37at2FollowupOverlay_LabelResolverUsesAuthorityAdapter
// asserts that the overlay's _nodeTypeLabel resolves through
// authorityAdapter.nodeKindLabel(kind) rather than a hard-coded local
// switch / table of Authority labels.
func TestExplorer_D37at2FollowupOverlay_LabelResolverUsesAuthorityAdapter(t *testing.T) {
	js := d37at2FollowupOverlayReadOverlay(t)

	body := d37at2FollowupOverlayBoundFn(t, js, "function _nodeTypeLabel(kind)")

	// Positive — body must reach into the canonical adapter.
	for _, want := range []string{
		"authorityAdapter",
		"adapter.nodeKindLabel(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37at2-followup-overlay: _nodeTypeLabel body must resolve through window.MIDASExplorerGraph.authorityAdapter.nodeKindLabel — missing %q", want)
		}
	}

	// Negative — no hard-coded Authority kind-label literals inside
	// the function body. The seven Authority kinds must derive from
	// the adapter, not from a local switch.
	for _, banned := range []string{
		"'BUSINESS SERVICE'",
		"'DECISION SURFACE'",
		"'AUTHORITY PROFILE'",
		"'AUTHORITY GRANT'",
		"'AGENT'",
		"'FAIL-MODE POLICY'",
		"'FAIL MODE POLICY'",
		"'ESCALATION TARGET'",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37at2-followup-overlay: _nodeTypeLabel must not carry %q as a hard-coded literal — the overlay resolves through the canonical adapter", banned)
		}
	}

	// Uppercase is applied as a presentation transform at the call
	// site or inside the resolver, not authored into the canonical
	// vocabulary.
	if !strings.Contains(body, ".toUpperCase()") {
		t.Error("D37at2-followup-overlay: _nodeTypeLabel must apply .toUpperCase() as the overlay's presentation transform")
	}

	// Unknown-kind fallback must remain — a novel kind resolves to
	// underscore-to-space uppercase rather than echoing the raw id.
	if !strings.Contains(body, ".replace(/_/g, ' ').toUpperCase()") {
		t.Error("D37at2-followup-overlay: _nodeTypeLabel must preserve the underscore-to-space uppercase fallback for unknown kinds")
	}
}

// ── 2. fail_mode_policy converges to canonical ───────────────────────

// TestExplorer_D37at2FollowupOverlay_FailModePolicyConvergesToCanonical
// asserts the overlay no longer carries the legacy hyphenated
// 'FAIL-MODE POLICY' literal. The runtime value for fail_mode_policy
// derives from the canonical adapter value 'Fail Mode Policy' and is
// uppercased at the call site to 'FAIL MODE POLICY' — matching the
// rich-theme Cytoscape label converged by tranche 1.
func TestExplorer_D37at2FollowupOverlay_FailModePolicyConvergesToCanonical(t *testing.T) {
	js := d37at2FollowupOverlayReadOverlay(t)

	if strings.Contains(js, "'FAIL-MODE POLICY'") {
		t.Error("D37at2-followup-overlay: cytoscape-html-overlay.js must not carry the legacy hyphenated 'FAIL-MODE POLICY' literal — fail_mode_policy resolves through authorityAdapter.nodeKindLabel and uppercases to 'FAIL MODE POLICY'")
	}
	// The hard-coded title-case hyphenated form must not appear
	// either — the canonical adapter value is unhyphenated.
	if strings.Contains(js, "'Fail-Mode Policy'") {
		t.Error("D37at2-followup-overlay: cytoscape-html-overlay.js must not carry the legacy hyphenated 'Fail-Mode Policy' literal — convergence on the canonical adapter value is the operator-visible change in this tranche")
	}
}

// ── 3. Context-like cases removed ────────────────────────────────────

// TestExplorer_D37at2FollowupOverlay_RemovesUnreachableContextCases
// asserts the dead Context-overlap cases were removed from the
// overlay. Authority's adapter FORBIDDEN_CONTEXT_NODE_KINDS contract
// makes capability / process / ai_system unreachable through this
// overlay's runtime path; the local cases were stale copy-paste
// residue from the D34a spike phase.
func TestExplorer_D37at2FollowupOverlay_RemovesUnreachableContextCases(t *testing.T) {
	js := d37at2FollowupOverlayReadOverlay(t)

	// The dead label literals must not return.
	for _, banned := range []string{
		"'CAPABILITY'",
		"'PROCESS'",
		"'AI SYSTEM'",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37at2-followup-overlay: cytoscape-html-overlay.js must not carry the dead Context-overlap label %s — Authority's FORBIDDEN_CONTEXT_NODE_KINDS contract makes the kind unreachable through this overlay", banned)
		}
	}
	// The Context-only switch-case shapes must not return.
	for _, banned := range []string{
		"case 'capability':",
		"case 'process':",
		"case 'ai_system':",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37at2-followup-overlay: cytoscape-html-overlay.js must not carry a switch case for the dead Context-overlap kind %q — the kind is unreachable through this overlay", banned)
		}
	}
}

// ── 4. aria-label + eyebrow both use the canonical resolver ──────────

// TestExplorer_D37at2FollowupOverlay_AriaAndEyebrowUseCanonicalResolver
// asserts both overlay label call sites — aria-label composition and
// visible eyebrow text — invoke the adapter-backed _nodeTypeLabel
// wrapper. The aria-label structure is preserved (the only operator-
// visible change is the label value).
func TestExplorer_D37at2FollowupOverlay_AriaAndEyebrowUseCanonicalResolver(t *testing.T) {
	js := d37at2FollowupOverlayReadOverlay(t)

	body := d37at2FollowupOverlayBoundFn(t, js, "function _buildCard(data) {")

	// aria-label call site — preserved structure.
	if !strings.Contains(body, "'aria-label'") {
		t.Error("D37at2-followup-overlay: _buildCard must keep the aria-label setAttribute path")
	}
	if !strings.Contains(body, "_nodeTypeLabel(data && data.kind)") {
		t.Error("D37at2-followup-overlay: aria-label composition must resolve the kind label via _nodeTypeLabel(data && data.kind)")
	}

	// Eyebrow call site — preserved structure.
	if !strings.Contains(body, "eyebrow.textContent = _nodeTypeLabel(data && data.kind);") {
		t.Error("D37at2-followup-overlay: visible eyebrow text must resolve via eyebrow.textContent = _nodeTypeLabel(data && data.kind);")
	}

	// Belt-and-braces — no aria-label or eyebrow path bypasses the
	// resolver with a hard-coded label literal.
	for _, banned := range []string{
		"'aria-label'\n      , 'BUSINESS",
		"eyebrow.textContent = 'BUSINESS",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D37at2-followup-overlay: _buildCard must not hard-code an aria-label/eyebrow string — %q", banned)
		}
	}
}

// ── 5. Authority-only ownership retained ─────────────────────────────

// TestExplorer_D37at2FollowupOverlay_AuthorityOnlyOwnershipRetained
// re-pins the overlay's Authority-only ownership contract after the
// tranche-2 consolidation. The shared overlay mount still hard-codes
// lensId: 'authority'; install / destroy are still called only by
// authority-cytoscape-poc.js; Context Graph view still does not
// reference the overlay.
func TestExplorer_D37at2FollowupOverlay_AuthorityOnlyOwnershipRetained(t *testing.T) {
	overlay := d37at2FollowupOverlayReadOverlay(t)

	// Hard-coded lensId — the overlay does not route across lenses.
	if !strings.Contains(overlay, "lensId: 'authority'") {
		t.Error("D37at2-followup-overlay: cytoscape-html-overlay.js must still pass lensId: 'authority' to the shared overlay mount — this tranche does not introduce per-lens routing")
	}

	// install / destroy still called only from the Authority PoC.
	pocBytes, err := os.ReadFile(d37at2FollowupOverlayPocPath)
	if err != nil {
		t.Fatalf("D37at2-followup-overlay: cannot read poc at %s: %v", d37at2FollowupOverlayPocPath, err)
	}
	poc := string(pocBytes)
	if !strings.Contains(poc, "_htmlOverlay.install(_cy, { mount: mount, elements: elements })") {
		t.Error("D37at2-followup-overlay: Authority PoC must still mount the overlay via _htmlOverlay.install(_cy, { mount: mount, elements: elements })")
	}
	if !strings.Contains(poc, "_htmlOverlayMod.destroy()") {
		t.Error("D37at2-followup-overlay: Authority PoC must still tear down the overlay via _htmlOverlayMod.destroy()")
	}

	// Context Graph view still has no reference to the overlay.
	ctxBytes, err := os.ReadFile(d37at2FollowupOverlayContextViewPath)
	if err != nil {
		t.Fatalf("D37at2-followup-overlay: cannot read context view at %s: %v", d37at2FollowupOverlayContextViewPath, err)
	}
	if strings.Contains(string(ctxBytes), "cytoscapeHtmlOverlay") {
		t.Error("D37at2-followup-overlay: Context Graph view must NOT reference cytoscapeHtmlOverlay — the overlay is Authority-only")
	}
}
