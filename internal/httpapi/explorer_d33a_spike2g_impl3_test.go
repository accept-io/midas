package httpapi

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl3_test.go — D33a-spike-2g-impl-3.
//
// First runtime consumer of the MIDAS icon registry: the Cytoscape
// Authority Graph PoC. The 7 self-authored data: URI glyphs from the
// previous PoC have been replaced by `_iconForKind(kind)` calls that
// resolve through `window.MIDASExplorerIcons.cytoscapeDataURI`. This
// tranche is scoped strictly to the PoC; production Authority Graph
// rendering, Context Graph rendering, and app chrome are untouched.
//
// Tests are source-string / file-system pins, matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl3PocPath   = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl3ViewPath  = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl3IndexHTML = "explorer/index.html"
	d33aSpike2gImpl3CtxDir    = "explorer/assets/js/graph/context"
)

func d33aSpike2gImpl3ReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl3PocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-3: cannot read PoC at %s: %v", d33aSpike2gImpl3PocPath, err)
	}
	return string(b)
}

// ── 1. PoC consumes the MIDAS icon registry ──────────────────────────

// TestExplorer_D33aSpike2gImpl3_PocUsesMidasIconRegistry pins that the
// Cytoscape PoC references the registry namespace, the data-URI
// helper, and every MIDAS-facing key for the 7 Authority node kinds.
func TestExplorer_D33aSpike2gImpl3_PocUsesMidasIconRegistry(t *testing.T) {
	js := d33aSpike2gImpl3ReadPoc(t)
	for _, want := range []string{
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"authorityBusinessService",
		"authorityDecisionSurface",
		"authorityProfile",
		"authorityGrant",
		"authorityAgent",
		"authorityFailModePolicy",
		"authorityEscalationTarget",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-3: PoC must reference %q for registry-backed icon resolution", want)
		}
	}
}

// ── 2. Old self-authored icon map removed / converted ────────────────

// TestExplorer_D33aSpike2gImpl3_PocNoLongerEmbedsSelfAuthoredObjectCardSvgs
// pins that the self-authored `_OBJECT_CARD_ICONS` data: URI map has
// been removed (or, if retained as a thin compatibility alias, no
// longer carries self-authored inline SVG data: URIs). The pin uses
// distinctive path fragments from the previous map — each was unique
// to that PoC's self-drawn glyph and is not part of any Lucide icon.
func TestExplorer_D33aSpike2gImpl3_PocNoLongerEmbedsSelfAuthoredObjectCardSvgs(t *testing.T) {
	js := d33aSpike2gImpl3ReadPoc(t)

	// No raw data: URI for an SVG can remain — the registry's
	// cytoscapeDataURI builds these at call time.
	if strings.Contains(js, "data:image/svg+xml;utf8,<svg") {
		t.Error("D33a-spike-2g-impl-3: PoC must not embed raw `data:image/svg+xml;utf8,<svg ...>` strings — icons come from the registry now")
	}

	// Distinctive geometry fragments from the previous self-authored
	// icon map. None of these path segments appear in the vendored
	// Lucide subset, so finding any one is hard evidence the old
	// glyph survived.
	for _, fragment := range []string{
		// building windows from the business_service glyph
		"M8 8v12 M12 8v12 M16 8v12",
		// shield outline from the authority_profile glyph
		"M12 3l8 3v6c0 5-3 8-8 9c-5-1-8-4-8-9V6z",
		// "check on document" polyline from authority_grant
		"polyline points='8,12 11,15 16,9'",
		// agent shoulders from agent
		"M5 21c0-3.5 3-6.5 7-6.5s7 3 7 6.5",
		// warning triangle outer path from fail_mode_policy
		"M12 3L21 20H3z",
		// escalation up-arrow from escalation_target
		"M12 4l8 8h-5v8h-6v-8H4z",
	} {
		if strings.Contains(js, fragment) {
			t.Errorf("D33a-spike-2g-impl-3: self-authored SVG path fragment must not remain in the PoC — found %q", fragment)
		}
	}
}

// ── 3. All 7 Authority kinds covered ─────────────────────────────────

// TestExplorer_D33aSpike2gImpl3_PocIconMapCoversAllAuthorityKinds pins
// the kind→key mapping carries every Authority node kind. Bound the
// scan to the `_AUTHORITY_KIND_ICON_KEYS = { ... }` block so the
// substring check cannot be fooled by an incidental occurrence of the
// kind name elsewhere.
func TestExplorer_D33aSpike2gImpl3_PocIconMapCoversAllAuthorityKinds(t *testing.T) {
	js := d33aSpike2gImpl3ReadPoc(t)
	start := strings.Index(js, "var _AUTHORITY_KIND_ICON_KEYS = {")
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-3: _AUTHORITY_KIND_ICON_KEYS declaration missing")
	}
	end := strings.Index(js[start:], "\n  };")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-3: could not bound _AUTHORITY_KIND_ICON_KEYS map")
	}
	body := js[start : start+end]
	for _, kind := range []string{
		"business_service",
		"decision_surface",
		"authority_profile",
		"authority_grant",
		"agent",
		"fail_mode_policy",
		"escalation_target",
	} {
		if !strings.Contains(body, kind+":") {
			t.Errorf("D33a-spike-2g-impl-3: kind %q missing from _AUTHORITY_KIND_ICON_KEYS", kind)
		}
	}
}

// ── 4. Safe fallback when registry is missing ────────────────────────

// TestExplorer_D33aSpike2gImpl3_PocIconLookupGuardsMissingRegistry
// pins that the icon helper degrades safely if `MIDASExplorerIcons`
// is absent (script load failure, premature execution, etc.). The
// helper must check the registry, check the key, return '' instead
// of throwing, and must never call into the registry without a
// guard.
func TestExplorer_D33aSpike2gImpl3_PocIconLookupGuardsMissingRegistry(t *testing.T) {
	js := d33aSpike2gImpl3ReadPoc(t)

	helperStart := strings.Index(js, "function _iconForKind(")
	if helperStart < 0 {
		t.Fatal("D33a-spike-2g-impl-3: _iconForKind helper missing")
	}
	// Bound the helper body to the closing `}` of the function. Use
	// the next blank line as a conservative bound — the helper is
	// only a few lines long.
	helperEnd := strings.Index(js[helperStart:], "\n  }")
	if helperEnd < 0 {
		t.Fatal("D33a-spike-2g-impl-3: could not bound _iconForKind helper body")
	}
	body := js[helperStart : helperStart+helperEnd]

	// Guards must be visible in source. We require:
	//   • a reference to window.MIDASExplorerIcons (the registry
	//     namespace under test);
	//   • a return path that yields '' (empty data URI) when the
	//     registry / key is unavailable;
	//   • a `has(` membership check (the registry's API).
	for _, want := range []string{
		"window.MIDASExplorerIcons",
		"return ''",
		".has(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-3: _iconForKind safe-fallback must contain %q", want)
		}
	}
	// Helper must NOT throw — assert no `throw` keyword inside the
	// helper body.
	if strings.Contains(body, "throw") {
		t.Error("D33a-spike-2g-impl-3: _iconForKind must not throw when registry/key is missing")
	}
}

// ── 5. Theme branches consume the helper ─────────────────────────────

// TestExplorer_D33aSpike2gImpl3_IconThemesUseRegistryBackedIcons pins
// that icon-using themes route their `background-image` lookup
// through the registry-backed helper, not through any directly
// declared data: URI. We require evidence in three themes:
//
//   • object-card (and object-card-v2 — they share a single
//     `else if` branch);
//   • object-tile-v3 (kind-specific style rules).
func TestExplorer_D33aSpike2gImpl3_IconThemesUseRegistryBackedIcons(t *testing.T) {
	js := d33aSpike2gImpl3ReadPoc(t)

	// object-card / object-card-v2 branch — the shared `else if`
	// guards on `_AUTHORITY_KIND_ICON_KEYS[kind]` and dereferences
	// the helper for the image.
	if !strings.Contains(js, "themeName === 'object-card' || themeName === 'object-card-v2'") {
		t.Error("D33a-spike-2g-impl-3: object-card / object-card-v2 shared branch missing")
	}
	if !strings.Contains(js, "nodeStyle['background-image']     = _iconForKind(kind);") {
		t.Error("D33a-spike-2g-impl-3: object-card / object-card-v2 must assign background-image via _iconForKind(kind)")
	}

	// object-tile-v3 branch — bound to the `if (themeName === ...)`
	// so we know each helper call sits inside the v3 theme.
	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2g-impl-3: object-tile-v3 theme branch missing")
	}
	body := js[start:]
	for _, want := range []string{
		"_iconForKind('business_service')",
		"_iconForKind('decision_surface')",
		"_iconForKind('authority_profile')",
		"_iconForKind('authority_grant')",
		"_iconForKind('agent')",
		"_iconForKind('fail_mode_policy')",
		"_iconForKind('escalation_target')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-3: object-tile-v3 must call %q for background-image", want)
		}
	}
}

// ── 6. Existing theme surface preserved ──────────────────────────────

// TestExplorer_D33aSpike2gImpl3_ExistingThemesPreserved pins that the
// theme catalogue is unchanged by impl-3. All eight names must
// remain selectable, and `classic` must remain the documented
// default.
func TestExplorer_D33aSpike2gImpl3_ExistingThemesPreserved(t *testing.T) {
	js := d33aSpike2gImpl3ReadPoc(t)
	// D33a-spike-2g-impl-4 appended `'authority-thin-card-v1'` to the
	// tail of the array. The impl-3 contract (8 prior themes remain
	// in their established positions; classic is the default) is
	// preserved by the additive change.
	const wantThemes = "var _THEMES        = ['classic', 'midas-card', 'object-card', 'object-card-v2', 'glass-card', 'holo-card', 'html-card', 'object-tile-v3', 'authority-thin-card-v1'];"
	if !strings.Contains(js, wantThemes) {
		t.Errorf("D33a-spike-2g-impl-3: theme catalogue must remain unchanged — expected %q", wantThemes)
	}
	if !strings.Contains(js, "var DEFAULT_THEME  = 'classic';") {
		t.Error("D33a-spike-2g-impl-3: DEFAULT_THEME must remain 'classic'")
	}
	for _, theme := range []string{
		"classic",
		"midas-card",
		"object-card",
		"object-card-v2",
		"glass-card",
		"holo-card",
		"html-card",
		"object-tile-v3",
	} {
		needle := "case '" + theme + "':"
		if !strings.Contains(js, needle) {
			t.Errorf("D33a-spike-2g-impl-3: theme dispatch missing case for %q", theme)
		}
	}
}

// ── 7. Production isolation ──────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl3_ProductionAuthorityViewDoesNotConsumeMidasIcons
// pins that the production Authority Graph view does NOT consume the
// MIDAS icon registry. The registry remains a PoC-only consumer in
// this tranche.
func TestExplorer_D33aSpike2gImpl3_ProductionAuthorityViewDoesNotConsumeMidasIcons(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl3ViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-3: cannot read Authority view at %s: %v", d33aSpike2gImpl3ViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		"MIDASExplorerIcons",
		"Lucide",
		"lucide",
		"cytoscapeDataURI",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-3: production Authority view must not reference %q", banned)
		}
	}
}

// TestExplorer_D33aSpike2gImpl3_ContextGraphDoesNotConsumeMidasIcons
// pins that no Context Graph module references the icon registry.
func TestExplorer_D33aSpike2gImpl3_ContextGraphDoesNotConsumeMidasIcons(t *testing.T) {
	entries, err := os.ReadDir(d33aSpike2gImpl3CtxDir)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-3: cannot read %s: %v", d33aSpike2gImpl3CtxDir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	for _, name := range files {
		path := filepath.Join(d33aSpike2gImpl3CtxDir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("D33a-spike-2g-impl-3: cannot read %s: %v", path, err)
			continue
		}
		if strings.Contains(string(b), "MIDASExplorerIcons") {
			t.Errorf("D33a-spike-2g-impl-3: Context Graph module %s must NOT reference MIDASExplorerIcons", name)
		}
	}
}

// ── 8. Load order remains valid ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl3_RegistryLoadsBeforePoc pins that
// index.html loads midas-icons.js BEFORE authority-cytoscape-poc.js so
// `window.MIDASExplorerIcons` is defined when the PoC IIFE executes.
func TestExplorer_D33aSpike2gImpl3_RegistryLoadsBeforePoc(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl3IndexHTML)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-3: cannot read index.html at %s: %v", d33aSpike2gImpl3IndexHTML, err)
	}
	body := string(b)
	const registryTag = `assets/js/icons/midas-icons.js`
	const pocTag = `assets/js/graph/authority/authority-cytoscape-poc.js`
	regIdx := strings.Index(body, registryTag)
	if regIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-3: index.html must reference %q", registryTag)
	}
	pocIdx := strings.Index(body, pocTag)
	if pocIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-3: index.html must reference %q", pocTag)
	}
	if !(regIdx < pocIdx) {
		t.Errorf("D33a-spike-2g-impl-3: midas-icons.js (%d) must load BEFORE authority-cytoscape-poc.js (%d)", regIdx, pocIdx)
	}
}

// ── 9. No external icon assets introduced ────────────────────────────

// TestExplorer_D33aSpike2gImpl3_NoExternalIconAssetsIntroduced pins
// asset hygiene: the PoC introduces no remote image / font / script
// fetches. The Cytoscape vendor reference and the SVG xmlns namespace
// embedded in `cytoscape.min.js` are unrelated to the icon swap.
func TestExplorer_D33aSpike2gImpl3_NoExternalIconAssetsIntroduced(t *testing.T) {
	js := d33aSpike2gImpl3ReadPoc(t)
	for _, banned := range []string{
		"new Image(",
		".src = 'http",
		`.src = "http`,
		"fetch('http",
		`fetch("http`,
		"XMLHttpRequest",
		"@font-face",
		"import('",
		`import("`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2g-impl-3: PoC must not introduce external asset fetch %q", banned)
		}
	}
}
