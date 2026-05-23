package httpapi

import (
	"os"
	"strings"
	"testing"
)

// explorer_d37at2_followup_t1_kind_label_test.go —
// D37at2-followup-kind-label-consolidation-tranche1-impl.
//
// This tranche removes the two duplicate Authority kind-label tables
// from authority-cytoscape-poc.js — the local _nodeTypeLabel switch
// (Table 2) and the _NODE_SUBTITLES map (Table 3) — and rewires every
// Cytoscape-native Authority kind-label reader to resolve through the
// canonical NODE_KIND_LABELS table in authority-graph-adapter.js
// (Table 1) via the local nodeKindLabel(kind) wrapper.
//
// Tranche scope is explicitly the Cytoscape-native rendering path:
//
//   • poc.js HTML-card icon SVG title (a11y)
//   • poc.js HTML-card kind chip text
//   • poc.js rich-theme Cytoscape node label (uppercase)
//   • poc.js native thin-card subtitle
//   • canvas-edge identity strip
//   • drawer kind row (already adapter-backed pre-tranche)
//   • legend (already adapter-backed pre-tranche)
//   • view payload label (already adapter-backed pre-tranche)
//
// The cytoscape-html-overlay.js local _nodeTypeLabel function (Table
// 4 — independent UPPERCASE variant with Context-overlap cases) was
// migrated by tranche 2 (D37at2-followup-overlay-consolidation-impl).
// As of that tranche the overlay resolves its eyebrow / aria-label
// vocabulary through authorityAdapter.nodeKindLabel(kind) with
// .toUpperCase() applied as an overlay-side presentation transform,
// and the Context-overlap cases (capability / process / ai_system)
// were removed as dead unreachable code. Tests in this file now
// include the overlay in the no-drift convergence contract.
//
// CWD at test time is internal/httpapi.

const (
	d37at2FollowupT1PocPath         = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37at2FollowupT1CanvasEdgePath  = "explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js"
	d37at2FollowupT1AdapterPath     = "explorer/assets/js/graph/authority/authority-graph-adapter.js"
	d37at2FollowupT1InspectorPath   = "explorer/assets/js/graph/authority/authority-graph-inspector.js"
	d37at2FollowupT1OverlaysPath    = "explorer/assets/js/graph/authority/authority-graph-overlays.js"
	d37at2FollowupT1ViewPath        = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d37at2FollowupT1HtmlOverlayPath = "explorer/assets/js/graph/authority/cytoscape-html-overlay.js"
)

func d37at2FollowupT1ReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d37at2FollowupT1PocPath)
	if err != nil {
		t.Fatalf("D37at2-followup-t1: cannot read poc.js at %s: %v", d37at2FollowupT1PocPath, err)
	}
	return string(b)
}

func d37at2FollowupT1ReadAdapter(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d37at2FollowupT1AdapterPath)
	if err != nil {
		t.Fatalf("D37at2-followup-t1: cannot read adapter at %s: %v", d37at2FollowupT1AdapterPath, err)
	}
	return string(b)
}

func d37at2FollowupT1ReadCanvasEdge(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d37at2FollowupT1CanvasEdgePath)
	if err != nil {
		t.Fatalf("D37at2-followup-t1: cannot read canvas-edge at %s: %v", d37at2FollowupT1CanvasEdgePath, err)
	}
	return string(b)
}

func d37at2FollowupT1ReadHtmlOverlay(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d37at2FollowupT1HtmlOverlayPath)
	if err != nil {
		t.Fatalf("D37at2-followup-t1: cannot read html-overlay at %s: %v", d37at2FollowupT1HtmlOverlayPath, err)
	}
	return string(b)
}

// ── 1. Local kind-label table removed from poc.js ────────────────────

// TestExplorer_D37at2FollowupT1_KindLabel_PocJsHasNoLocalTable asserts
// the clean-break removal of the two duplicate Authority kind-label
// tables from authority-cytoscape-poc.js:
//
//   • function _nodeTypeLabel(kind) — Table 2 in the inventory
//   • var/let/const _NODE_SUBTITLES — Table 3 in the inventory
//
// Both are replaced by reads through the canonical adapter table. A
// transitional shim is explicitly NOT permitted.
func TestExplorer_D37at2FollowupT1_KindLabel_PocJsHasNoLocalTable(t *testing.T) {
	js := d37at2FollowupT1ReadPoc(t)

	// Negative pin — the legacy local switch must be gone.
	if strings.Contains(js, "function _nodeTypeLabel(") {
		t.Error("D37at2-followup-t1: function _nodeTypeLabel(...) must not return in authority-cytoscape-poc.js — Cytoscape-native call sites resolve via nodeKindLabel(kind) → adapter.nodeKindLabel(kind)")
	}
	// Negative pin — the legacy subtitle table must be gone (Option A).
	for _, banned := range []string{
		"var _NODE_SUBTITLES",
		"let _NODE_SUBTITLES",
		"const _NODE_SUBTITLES",
	} {
		if strings.Contains(js, banned+" = {") || strings.Contains(js, banned+" =") {
			t.Errorf("D37at2-followup-t1: %q declaration must not return in authority-cytoscape-poc.js — _nodeSubtitle resolves through the canonical adapter", banned)
		}
	}
	// Negative pin — the public exports for the removed symbols must
	// not return.
	if strings.Contains(js, "_nodeTypeLabel:            _nodeTypeLabel,") ||
		strings.Contains(js, "_nodeTypeLabel: _nodeTypeLabel,") {
		t.Error("D37at2-followup-t1: poc public surface must not export _nodeTypeLabel — the function is removed")
	}
	if strings.Contains(js, "_nodeSubtitles:            _NODE_SUBTITLES,") ||
		strings.Contains(js, "_nodeSubtitles: _NODE_SUBTITLES,") {
		t.Error("D37at2-followup-t1: poc public surface must not export _nodeSubtitles — the underlying table is removed")
	}
}

// ── 2. Canonical NODE_KIND_LABELS contract ───────────────────────────

// TestExplorer_D37at2FollowupT1_KindLabel_CanonicalLabelsPerKind pins
// the canonical NODE_KIND_LABELS table in authority-graph-adapter.js
// and asserts the canonical fail_mode_policy label is the unhyphenated
// 'Fail Mode Policy' string used by the drawer, legend, and view
// payload pre-tranche, which the in-scope Cytoscape-native surfaces
// must now also resolve to.
func TestExplorer_D37at2FollowupT1_KindLabel_CanonicalLabelsPerKind(t *testing.T) {
	js := d37at2FollowupT1ReadAdapter(t)

	tableStart := strings.Index(js, "var NODE_KIND_LABELS = Object.freeze({")
	if tableStart < 0 {
		t.Fatal("D37at2-followup-t1: canonical NODE_KIND_LABELS table missing from authority-graph-adapter.js")
	}
	tableEnd := strings.Index(js[tableStart:], "});")
	if tableEnd < 0 {
		t.Fatal("D37at2-followup-t1: could not bound NODE_KIND_LABELS table")
	}
	tableBody := js[tableStart : tableStart+tableEnd]

	pairs := map[string]string{
		"business_service":  "'Business Service'",
		"decision_surface":  "'Decision Surface'",
		"authority_profile": "'Authority Profile'",
		"authority_grant":   "'Authority Grant'",
		"agent":             "'Agent'",
		"fail_mode_policy":  "'Fail Mode Policy'",
		"escalation_target": "'Escalation Target'",
	}
	for kind, label := range pairs {
		if !strings.Contains(tableBody, kind+":") {
			t.Errorf("D37at2-followup-t1: NODE_KIND_LABELS missing kind %q", kind)
			continue
		}
		if !strings.Contains(tableBody, label) {
			t.Errorf("D37at2-followup-t1: NODE_KIND_LABELS missing canonical label %q (for %q)", label, kind)
		}
	}

	// Negative pin — the canonical table must not carry the legacy
	// hyphenated form anywhere inside its body.
	if strings.Contains(tableBody, "'Fail-Mode Policy'") {
		t.Error("D37at2-followup-t1: canonical NODE_KIND_LABELS must not carry the legacy hyphenated 'Fail-Mode Policy'")
	}
}

// ── 3. Cytoscape-native surfaces read the adapter ────────────────────

// TestExplorer_D37at2FollowupT1_KindLabel_CytoscapeNativeSurfacesReadAdapter
// asserts that the in-scope Cytoscape-native Authority surfaces now
// resolve their kind label through the canonical adapter rather than
// through the removed _nodeTypeLabel / _NODE_SUBTITLES tables.
//
// The test is source-level and intentionally robust to line shifts —
// it pins call forms, not line numbers.
func TestExplorer_D37at2FollowupT1_KindLabel_CytoscapeNativeSurfacesReadAdapter(t *testing.T) {
	poc := d37at2FollowupT1ReadPoc(t)
	ce := d37at2FollowupT1ReadCanvasEdge(t)

	// 3.1 — poc.js exposes a thin nodeKindLabel(kind) wrapper that
	// delegates to window.MIDASExplorerGraph.authorityAdapter.
	helperStart := strings.Index(poc, "function nodeKindLabel(kind)")
	if helperStart < 0 {
		t.Fatal("D37at2-followup-t1: poc.js must declare function nodeKindLabel(kind) as the adapter-backed wrapper")
	}
	helperEnd := strings.Index(poc[helperStart:], "\n  }")
	if helperEnd < 0 {
		t.Fatal("D37at2-followup-t1: could not bound nodeKindLabel body")
	}
	helperBody := poc[helperStart : helperStart+helperEnd]
	for _, want := range []string{
		"authorityAdapter",
		"adapter.nodeKindLabel",
	} {
		if !strings.Contains(helperBody, want) {
			t.Errorf("D37at2-followup-t1: nodeKindLabel must delegate to window.MIDASExplorerGraph.authorityAdapter.nodeKindLabel — missing %q", want)
		}
	}

	// 3.2 — HTML-card icon SVG title resolves via nodeKindLabel.
	if !strings.Contains(poc, "title:  nodeKindLabel(d.kind") &&
		!strings.Contains(poc, "title: nodeKindLabel(d.kind") {
		t.Error("D37at2-followup-t1: HTML-card icon SVG title must resolve via nodeKindLabel(d.kind …)")
	}

	// 3.3 — HTML-card kind chip text resolves via nodeKindLabel.
	if !strings.Contains(poc, "kindEl.textContent = nodeKindLabel(d.kind") {
		t.Error("D37at2-followup-t1: HTML-card kind chip textContent must resolve via nodeKindLabel(d.kind …)")
	}

	// 3.4 — Rich-theme Cytoscape label resolves via nodeKindLabel
	// (uppercase is a presentation transform at the call site).
	if !strings.Contains(poc, "nodeKindLabel(ele.data('kind')).toUpperCase()") {
		t.Error("D37at2-followup-t1: rich-theme Cytoscape label must compose `_displayLabel(ele) + '\\n' + nodeKindLabel(ele.data('kind')).toUpperCase()`")
	}

	// 3.5 — _nodeSubtitle reads the canonical adapter, not a local
	// table.
	subStart := strings.Index(poc, "function _nodeSubtitle(")
	if subStart < 0 {
		t.Fatal("D37at2-followup-t1: _nodeSubtitle helper missing from poc.js")
	}
	subEnd := strings.Index(poc[subStart:], "\n  }")
	if subEnd < 0 {
		t.Fatal("D37at2-followup-t1: could not bound _nodeSubtitle body")
	}
	subBody := poc[subStart : subStart+subEnd]
	if !strings.Contains(subBody, "authorityAdapter") || !strings.Contains(subBody, "nodeKindLabel") {
		t.Error("D37at2-followup-t1: _nodeSubtitle must resolve its vocabulary through window.MIDASExplorerGraph.authorityAdapter.nodeKindLabel")
	}
	if strings.Contains(subBody, "_NODE_SUBTITLES") {
		t.Error("D37at2-followup-t1: _nodeSubtitle must not reference the removed _NODE_SUBTITLES table")
	}

	// 3.6 — canvas-edge _kindLabel no longer delegates to
	// poc._nodeTypeLabel; it resolves via the canonical adapter.
	kindStart := strings.Index(ce, "function _kindLabel(kind)")
	if kindStart < 0 {
		t.Fatal("D37at2-followup-t1: canvas-edge _kindLabel wrapper missing")
	}
	kindEnd := strings.Index(ce[kindStart:], "\n  }")
	if kindEnd < 0 {
		t.Fatal("D37at2-followup-t1: could not bound canvas-edge _kindLabel body")
	}
	kindBody := ce[kindStart : kindStart+kindEnd]
	if strings.Contains(kindBody, "_nodeTypeLabel") {
		t.Error("D37at2-followup-t1: canvas-edge _kindLabel must not reference poc._nodeTypeLabel — the function is removed")
	}
	if !strings.Contains(kindBody, "authorityAdapter") || !strings.Contains(kindBody, "nodeKindLabel") {
		t.Error("D37at2-followup-t1: canvas-edge _kindLabel must resolve via window.MIDASExplorerGraph.authorityAdapter.nodeKindLabel")
	}

	// 3.7 — no hard-coded 'Fail-Mode Policy' (hyphenated) string
	// remains in poc.js. The canonical convergence is the only
	// operator-visible label change in this tranche, and it must
	// emanate from the adapter, not be re-introduced locally.
	if strings.Contains(poc, "'Fail-Mode Policy'") {
		t.Error("D37at2-followup-t1: hyphenated 'Fail-Mode Policy' must not appear as a hard-coded display label string in authority-cytoscape-poc.js")
	}
}

// ── 4. No drift between Cytoscape-native surfaces ────────────────────

// TestExplorer_D37at2FollowupT1_KindLabel_NoDriftBetweenCytoscapeNativeSurfaces
// asserts that the in-scope Cytoscape-native Authority surfaces are
// all adapter-backed via source-level dependency, and that no
// competing local Authority kind-label table remains in poc.js.
//
// D37at2-followup-overlay-consolidation-impl extended this contract
// to include cytoscape-html-overlay.js, which now resolves its
// eyebrow / aria-label vocabulary through the Authority adapter and
// applies .toUpperCase() as a presentation transform at the call
// site. The HTML overlay is no longer an excluded surface.
func TestExplorer_D37at2FollowupT1_KindLabel_NoDriftBetweenCytoscapeNativeSurfaces(t *testing.T) {
	poc := d37at2FollowupT1ReadPoc(t)
	ce := d37at2FollowupT1ReadCanvasEdge(t)

	// Already-canonical surfaces (drawer, legend, view payload, and as
	// of D37at2-followup-overlay the HTML overlay) all read through
	// adapter.nodeKindLabel.
	type adapterReader struct {
		path string
		want string
	}
	for _, r := range []adapterReader{
		{d37at2FollowupT1InspectorPath, "adapter.nodeKindLabel("},
		{d37at2FollowupT1OverlaysPath, "adapter.nodeKindLabel("},
		{d37at2FollowupT1ViewPath, "adapter.nodeKindLabel("},
		{d37at2FollowupT1HtmlOverlayPath, "adapter.nodeKindLabel("},
	} {
		b, err := os.ReadFile(r.path)
		if err != nil {
			t.Fatalf("D37at2-followup-t1: cannot read %s: %v", r.path, err)
		}
		if !strings.Contains(string(b), r.want) {
			t.Errorf("D37at2-followup-t1: %s must continue to read kind labels via %s", r.path, r.want)
		}
	}

	// In-scope Cytoscape-native readers in poc.js — must resolve
	// through nodeKindLabel(kind).
	for _, want := range []string{
		"function nodeKindLabel(kind)",
		"title:  nodeKindLabel(d.kind",
		"kindEl.textContent = nodeKindLabel(d.kind",
		"nodeKindLabel(ele.data('kind')).toUpperCase()",
	} {
		if !strings.Contains(poc, want) {
			// title check supports a single-space variant; only
			// fail the two-space form if both alternates are absent.
			if want == "title:  nodeKindLabel(d.kind" && strings.Contains(poc, "title: nodeKindLabel(d.kind") {
				continue
			}
			t.Errorf("D37at2-followup-t1: poc.js Cytoscape-native reader missing %q", want)
		}
	}

	// In-scope canvas-edge reader — wrapper preserved but body must
	// trace to adapter.nodeKindLabel.
	if !strings.Contains(ce, "function _kindLabel(kind)") {
		t.Error("D37at2-followup-t1: canvas-edge must retain its _kindLabel wrapper for the six in-module callers")
	}
	if !strings.Contains(ce, "adapter.nodeKindLabel(kind)") {
		t.Error("D37at2-followup-t1: canvas-edge _kindLabel must trace to adapter.nodeKindLabel(kind)")
	}

	// Negative pins — no competing local table in poc.js, no
	// in-scope reader of a removed table.
	for _, banned := range []string{
		"function _nodeTypeLabel(",
		"var _NODE_SUBTITLES",
		"_NODE_SUBTITLES[",
	} {
		if strings.Contains(poc, banned) {
			t.Errorf("D37at2-followup-t1: poc.js must not contain %q — competing local kind-label vocabulary", banned)
		}
	}
}

// ── 5. Non-fail_mode_policy kinds keep their canonical labels ────────

// TestExplorer_D37at2FollowupT1_AuthorityNoBehaviouralChange_NonFailModePolicyKinds
// pins that for the six non-fail_mode_policy Authority kinds, the
// canonical label remains unchanged from its pre-tranche value. The
// only intentional operator-visible label change in this tranche is
// the fail_mode_policy convergence.
func TestExplorer_D37at2FollowupT1_AuthorityNoBehaviouralChange_NonFailModePolicyKinds(t *testing.T) {
	js := d37at2FollowupT1ReadAdapter(t)

	tableStart := strings.Index(js, "var NODE_KIND_LABELS = Object.freeze({")
	if tableStart < 0 {
		t.Fatal("D37at2-followup-t1: canonical NODE_KIND_LABELS table missing")
	}
	tableEnd := strings.Index(js[tableStart:], "});")
	if tableEnd < 0 {
		t.Fatal("D37at2-followup-t1: could not bound NODE_KIND_LABELS table")
	}
	tableBody := js[tableStart : tableStart+tableEnd]

	stable := map[string]string{
		"business_service":  "'Business Service'",
		"decision_surface":  "'Decision Surface'",
		"authority_profile": "'Authority Profile'",
		"authority_grant":   "'Authority Grant'",
		"agent":             "'Agent'",
		"escalation_target": "'Escalation Target'",
	}
	for kind, label := range stable {
		if !strings.Contains(tableBody, kind+":") {
			t.Errorf("D37at2-followup-t1: NODE_KIND_LABELS missing kind %q", kind)
			continue
		}
		if !strings.Contains(tableBody, label) {
			t.Errorf("D37at2-followup-t1: NODE_KIND_LABELS canonical label for %q must remain %q (no behavioural change for non-fail_mode_policy kinds)", kind, label)
		}
	}
}

// ── 6. fail_mode_policy converges to canonical ───────────────────────

// TestExplorer_D37at2FollowupT1_AuthorityFailModePolicy_ConvergesToCanonical
// asserts that for the in-scope adapter-backed Authority surfaces,
// fail_mode_policy converges on the canonical 'Fail Mode Policy'
// string (unhyphenated). The uppercase rich-theme derivation produces
// 'FAIL MODE POLICY' at the call site, since uppercase is a
// presentation transform — the value still comes from the adapter.
//
// D37at2-followup-overlay-consolidation-impl extended this test to
// include cytoscape-html-overlay.js — the overlay now resolves its
// fail_mode_policy label through authorityAdapter.nodeKindLabel and
// uppercases at the call site, producing 'FAIL MODE POLICY'
// (canonical). The HTML overlay is no longer an excluded surface.
func TestExplorer_D37at2FollowupT1_AuthorityFailModePolicy_ConvergesToCanonical(t *testing.T) {
	adapter := d37at2FollowupT1ReadAdapter(t)

	// Canonical value source.
	if !strings.Contains(adapter, "fail_mode_policy:  'Fail Mode Policy',") {
		t.Error("D37at2-followup-t1: NODE_KIND_LABELS.fail_mode_policy must be the canonical 'Fail Mode Policy' (unhyphenated)")
	}

	// All in-scope surfaces resolve through the adapter, so the
	// runtime value for fail_mode_policy is 'Fail Mode Policy' (and
	// 'FAIL MODE POLICY' after the overlay / rich-theme presentation
	// transform). We assert the source-level contract: neither the
	// hyphenated title-case form nor its uppercase counterpart may
	// appear as a hard-coded literal in any adapter-backed surface.
	for _, asset := range []string{
		d37at2FollowupT1PocPath,
		d37at2FollowupT1CanvasEdgePath,
		d37at2FollowupT1HtmlOverlayPath,
	} {
		b, err := os.ReadFile(asset)
		if err != nil {
			t.Fatalf("D37at2-followup-t1: cannot read %s: %v", asset, err)
		}
		s := string(b)
		if strings.Contains(s, "'Fail-Mode Policy'") {
			t.Errorf("D37at2-followup-t1: %s must not carry the legacy hyphenated 'Fail-Mode Policy' literal — convergence to canonical 'Fail Mode Policy' emanates from the adapter", asset)
		}
		if strings.Contains(s, "'FAIL-MODE POLICY'") {
			t.Errorf("D37at2-followup-t1: %s must not carry the legacy hyphenated uppercase 'FAIL-MODE POLICY' literal — uppercase derives from .toUpperCase() applied to the canonical adapter value", asset)
		}
	}

	// Rich-theme uppercase derivation form — the .toUpperCase() of
	// the canonical 'Fail Mode Policy' is 'FAIL MODE POLICY'. We
	// pin the derivation form, not the runtime string, by asserting
	// the source-level composition.
	poc := d37at2FollowupT1ReadPoc(t)
	if !strings.Contains(poc, "nodeKindLabel(ele.data('kind')).toUpperCase()") {
		t.Error("D37at2-followup-t1: rich-theme Cytoscape label must derive its uppercase form from nodeKindLabel(...) — runtime value for fail_mode_policy is 'FAIL MODE POLICY' (derived from the canonical adapter value)")
	}
}

// ── 7. HTML overlay tranche-2-pending marker retired ─────────────────

// TestExplorer_D37at2FollowupT1_HtmlOverlayOutOfScope was originally a
// scope-defence test that pinned the temporary intermediate state in
// which the HTML overlay still owned its own hyphenated 'FAIL-MODE
// POLICY' label. D37at2-followup-overlay-consolidation-impl
// discharged that scope: the overlay's _nodeTypeLabel now resolves
// through authorityAdapter.nodeKindLabel(kind).toUpperCase() and no
// longer carries the legacy hyphenated literal.
//
// This test now pins the inverse contract — the overlay must NOT
// re-introduce a local hard-coded Authority kind-label switch and
// must NOT re-introduce the hyphenated 'FAIL-MODE POLICY' literal.
// Stronger positive coverage of the adapter-backed resolution lives
// in the D37at2-followup-overlay test file.
func TestExplorer_D37at2FollowupT1_HtmlOverlayOutOfScope(t *testing.T) {
	overlay := d37at2FollowupT1ReadHtmlOverlay(t)

	// The legacy hyphenated literal must not return.
	if strings.Contains(overlay, "'FAIL-MODE POLICY'") {
		t.Error("D37at2-followup-overlay-impl: cytoscape-html-overlay.js must not re-introduce the legacy hyphenated 'FAIL-MODE POLICY' literal — fail_mode_policy resolves through authorityAdapter.nodeKindLabel and uppercases to 'FAIL MODE POLICY'")
	}
	// The dead Context-overlap cases must not return.
	for _, banned := range []string{"'CAPABILITY'", "'PROCESS'", "'AI SYSTEM'"} {
		if strings.Contains(overlay, banned) {
			t.Errorf("D37at2-followup-overlay-impl: cytoscape-html-overlay.js must not re-introduce the dead Context-overlap label %s — Authority adapter FORBIDDEN_CONTEXT_NODE_KINDS makes that kind unreachable through this overlay", banned)
		}
	}
	// The overlay's _nodeTypeLabel function itself must remain (kept
	// for diagnostic / public-surface continuity) but resolve through
	// the adapter, not a local switch.
	if !strings.Contains(overlay, "function _nodeTypeLabel(kind)") {
		t.Error("D37at2-followup-overlay-impl: cytoscape-html-overlay.js must keep function _nodeTypeLabel(kind) as the adapter-backed call-site wrapper")
	}
}
