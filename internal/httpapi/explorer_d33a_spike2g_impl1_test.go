package httpapi

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl1_test.go — D33a-spike-2g-impl-1.
// Vendoring foundation for the curated Lucide icon subset. No runtime
// wiring yet — assets / metadata / NOTICE only.
//
// Tests follow the existing Explorer Tier-1 style but read directly
// from the filesystem (the icons live under the embedded explorer
// tree but the test asserts the disk source, which is what gets
// embedded at build time).

const (
	d33aSpike2gLucideDir = "explorer/assets/icons/lucide"
	d33aSpike2gExpectedIconCount = 30
)

func d33aSpike2gLucidePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(d33aSpike2gLucideDir, name)
}

func d33aSpike2gReadLucideFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gLucidePath(t, name))
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-1: cannot read %s/%s: %v", d33aSpike2gLucideDir, name, err)
	}
	return string(b)
}

// ── Licence files ────────────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl1_LucideLicenseFilePresent pins that
// the Lucide LICENSE file exists in the vendor directory.
func TestExplorer_D33aSpike2gImpl1_LucideLicenseFilePresent(t *testing.T) {
	if _, err := os.Stat(d33aSpike2gLucidePath(t, "LICENSE")); err != nil {
		t.Fatalf("D33a-spike-2g-impl-1: LICENSE missing under %s: %v", d33aSpike2gLucideDir, err)
	}
}

// TestExplorer_D33aSpike2gImpl1_LucideLicenseIncludesIscAndFeatherMitSections
// pins that the LICENSE file carries BOTH the ISC section (covering
// Lucide Icons and Contributors) AND the Feather-derived MIT section
// (covering Cole Bemis / Feather). Both must be preserved verbatim.
func TestExplorer_D33aSpike2gImpl1_LucideLicenseIncludesIscAndFeatherMitSections(t *testing.T) {
	body := d33aSpike2gReadLucideFile(t, "LICENSE")
	for _, want := range []string{
		"ISC License",
		"Lucide Icons and Contributors",
		"The MIT License (MIT)",
		"Cole Bemis",
		"Feather",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-1: LICENSE must contain %q (preserve both ISC and Feather/MIT sections verbatim)", want)
		}
	}
}

// ── Vendor metadata ──────────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl1_LucideReadmeAndVendorMetadataPresent
// pins that README.md and VENDOR.md exist and that VENDOR.md declares
// the source, licence, and verification gate.
func TestExplorer_D33aSpike2gImpl1_LucideReadmeAndVendorMetadataPresent(t *testing.T) {
	if _, err := os.Stat(d33aSpike2gLucidePath(t, "README.md")); err != nil {
		t.Fatalf("D33a-spike-2g-impl-1: README.md missing under %s: %v", d33aSpike2gLucideDir, err)
	}
	vendor := d33aSpike2gReadLucideFile(t, "VENDOR.md")
	for _, want := range []string{
		"Lucide",
		"https://lucide.dev/",
		"https://github.com/lucide-icons/lucide",
		// Verification gate — must remain explicit until a real
		// upstream tag/commit is recorded.
		"TO BE VERIFIED BEFORE MERGE",
		"Feather",
		"MIT",
	} {
		if !strings.Contains(vendor, want) {
			t.Errorf("D33a-spike-2g-impl-1: VENDOR.md must contain %q", want)
		}
	}
}

// ── Icon count + filenames ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl1_LucideIconsCountMatchesFinalSubset
// pins exactly 30 .svg files in the vendor directory.
func TestExplorer_D33aSpike2gImpl1_LucideIconsCountMatchesFinalSubset(t *testing.T) {
	entries, err := os.ReadDir(d33aSpike2gLucideDir)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-1: cannot read dir %s: %v", d33aSpike2gLucideDir, err)
	}
	var svgCount int
	var svgNames []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".svg") {
			svgCount++
			svgNames = append(svgNames, e.Name())
		}
	}
	if svgCount != d33aSpike2gExpectedIconCount {
		sort.Strings(svgNames)
		t.Errorf("D33a-spike-2g-impl-1: want exactly %d .svg files under %s, found %d: %v",
			d33aSpike2gExpectedIconCount, d33aSpike2gLucideDir, svgCount, svgNames)
	}
}

// TestExplorer_D33aSpike2gImpl1_LucideExpectedIconFilesPresent
// pins every named icon file from the catalogue.
func TestExplorer_D33aSpike2gImpl1_LucideExpectedIconFilesPresent(t *testing.T) {
	expected := []string{
		// Authority + shared graph nodes (7)
		"building-2.svg",
		"workflow.svg",
		"shield-check.svg",
		"file-check-2.svg",
		"bot.svg",
		"triangle-alert.svg",
		"arrow-up-from-line.svg",
		// Context-only graph nodes (5)
		"puzzle.svg",
		"route.svg",
		"cpu.svg",
		"target.svg",
		"link-2.svg",
		// Workbench / chrome (6)
		"refresh-cw.svg",
		"settings.svg",
		"info.svg",
		"circle-help.svg",
		"external-link.svg",
		"download.svg",
		// Lifecycle (5)
		"circle-dashed.svg",
		"eye.svg",
		"circle-check.svg",
		"archive.svg",
		"archive-x.svg",
		// Severity / state (3)
		"octagon-alert.svg",
		"circle-x.svg",
		"circle-pause.svg",
		// Audit / integrity (2)
		"lock.svg",
		"lock-open.svg",
		// Posture (2)
		"trending-down.svg",
		"stethoscope.svg",
	}
	if got := len(expected); got != d33aSpike2gExpectedIconCount {
		t.Fatalf("D33a-spike-2g-impl-1: test fixture lists %d expected icons but catalogue says %d", got, d33aSpike2gExpectedIconCount)
	}
	for _, name := range expected {
		if _, err := os.Stat(d33aSpike2gLucidePath(t, name)); err != nil {
			t.Errorf("D33a-spike-2g-impl-1: expected icon file %q missing under %s: %v", name, d33aSpike2gLucideDir, err)
		}
	}
}

// ── SVG hygiene ──────────────────────────────────────────────────────

// d33aSpike2gAllSvgs returns the list of .svg filenames in the vendor
// directory, used by the hygiene tests below.
func d33aSpike2gAllSvgs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(d33aSpike2gLucideDir)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-1: cannot read dir %s: %v", d33aSpike2gLucideDir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".svg") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// TestExplorer_D33aSpike2gImpl1_LucideIconsUseCurrentColorStroke
// pins that every SVG uses stroke="currentColor" so it picks up the
// consumer's CSS color token, and rejects any literal stroke hex.
func TestExplorer_D33aSpike2gImpl1_LucideIconsUseCurrentColorStroke(t *testing.T) {
	for _, name := range d33aSpike2gAllSvgs(t) {
		body := d33aSpike2gReadLucideFile(t, name)
		if !strings.Contains(body, `stroke="currentColor"`) {
			t.Errorf("D33a-spike-2g-impl-1: %s must contain stroke=\"currentColor\"", name)
		}
		// Reject any per-path / per-attr hex stroke colours.
		// (Allow `stroke="currentColor"` and `stroke-` prefixed
		// attribute names like stroke-width / stroke-linecap.)
		lowered := strings.ToLower(body)
		for _, badPrefix := range []string{
			`stroke="#`,
			`stroke='#`,
			`stroke="rgb`,
			`stroke="hsl`,
		} {
			if strings.Contains(lowered, badPrefix) {
				t.Errorf("D33a-spike-2g-impl-1: %s must not use hard-coded stroke colour (found %q)", name, badPrefix)
			}
		}
	}
}

// TestExplorer_D33aSpike2gImpl1_LucideIconsHaveStandardSvgShape pins
// the Lucide-standard SVG header on every icon. The exact viewBox,
// fill=none, and stroke-linecap/linejoin=round are required by the
// vendoring policy (see VENDOR.md "modification policy").
func TestExplorer_D33aSpike2gImpl1_LucideIconsHaveStandardSvgShape(t *testing.T) {
	for _, name := range d33aSpike2gAllSvgs(t) {
		body := d33aSpike2gReadLucideFile(t, name)
		for _, want := range []string{
			"<svg",
			`viewBox="0 0 24 24"`,
			`fill="none"`,
			`stroke-linecap="round"`,
			`stroke-linejoin="round"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("D33a-spike-2g-impl-1: %s missing standard Lucide attribute %q", name, want)
			}
		}
	}
}

// TestExplorer_D33aSpike2gImpl1_LucideIconsNoExternalReferences pins
// asset hygiene: no <image>, no xlink:href, no http(s) URLs except
// the canonical SVG XML namespace.
func TestExplorer_D33aSpike2gImpl1_LucideIconsNoExternalReferences(t *testing.T) {
	const svgNamespace = "http://www.w3.org/2000/svg"
	for _, name := range d33aSpike2gAllSvgs(t) {
		body := d33aSpike2gReadLucideFile(t, name)
		// No external image embedding.
		if strings.Contains(body, "<image") {
			t.Errorf("D33a-spike-2g-impl-1: %s must not include <image> element", name)
		}
		if strings.Contains(body, "xlink:href") {
			t.Errorf("D33a-spike-2g-impl-1: %s must not include xlink:href", name)
		}
		// Strip the SVG namespace string; anything else http/https is
		// an external reference and forbidden.
		stripped := strings.ReplaceAll(body, svgNamespace, "")
		for _, banned := range []string{"http://", "https://"} {
			if strings.Contains(stripped, banned) {
				t.Errorf("D33a-spike-2g-impl-1: %s must not contain external URL %q (only the SVG xmlns is allowed)", name, banned)
			}
		}
	}
}

// ── NOTICE attribution ──────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl1_RootNoticeIncludesLucideAttribution
// pins the root NOTICE file carries the Lucide attribution paragraph.
// Project root is two directories above the test working directory
// (internal/httpapi → internal → repo-root).
func TestExplorer_D33aSpike2gImpl1_RootNoticeIncludesLucideAttribution(t *testing.T) {
	// Resolve repo root from CWD = internal/httpapi.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-1: cannot read CWD: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	noticePath := filepath.Join(repoRoot, "NOTICE")
	b, err := os.ReadFile(noticePath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-1: cannot read root NOTICE at %s: %v", noticePath, err)
	}
	body := string(b)
	for _, want := range []string{
		"Lucide",
		"ISC",
		"Feather",
		"MIT",
		"internal/httpapi/explorer/assets/icons/lucide/LICENSE",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-1: root NOTICE must contain %q (Lucide attribution paragraph)", want)
		}
	}
	// Existing notice content must be preserved.
	if !strings.Contains(body, "Accept MIDAS") || !strings.Contains(body, "Accept.io") {
		t.Error("D33a-spike-2g-impl-1: root NOTICE must preserve the existing Accept MIDAS / Accept.io copyright header")
	}
}

// ── No runtime wiring yet ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl1_NoRuntimeRegistryOrGraphRenderingChanges
// pins the production-isolation invariants delivered by impl-1's
// vendoring: no graph-visuals classification module exists, and the
// production Authority Graph view does not reference Lucide or the
// MIDAS icon registry namespace.
//
// History:
//   • D33a-spike-2g-impl-2 introduced
//     assets/js/icons/midas-icons.js as the local MIDAS icon registry
//     (that file is allowed and pinned by the impl-2 tests).
//   • D33a-spike-2g-impl-3 made the Cytoscape PoC consume the
//     registry (pinned positively by the impl-3 tests). The original
//     impl-1 PoC checks (PoC must still carry _OBJECT_CARD_ICONS;
//     PoC must NOT yet reference MIDASExplorerIcons) were removed
//     here in impl-3 because they no longer reflect the current
//     downstream contract.
//
// What is still pinned here is the strict production-isolation
// invariant: midas-graph-visuals.js does not yet exist, and the
// production Authority Graph view does not consume the registry.
func TestExplorer_D33aSpike2gImpl1_NoRuntimeRegistryOrGraphRenderingChanges(t *testing.T) {
	// 1. Future graph-visuals module must NOT exist yet.
	for _, registryPath := range []string{
		"explorer/assets/js/icons/midas-graph-visuals.js",
	} {
		if _, err := os.Stat(registryPath); err == nil {
			t.Errorf("D33a-spike-2g-impl-1: module %s must NOT exist in this tranche (deferred to a later D33a-spike-2g tranche)", registryPath)
		}
	}

	// 2. Production Authority view contains no Lucide / registry
	// references. The Cytoscape PoC may reference the registry
	// (impl-3 wired that intentionally) — production must not.
	viewPath := "explorer/assets/js/graph/authority/authority-graph-view.js"
	viewBody, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-1: cannot read Authority view at %s: %v", viewPath, err)
	}
	for _, banned := range []string{"lucide", "MIDASExplorerIcons", "MIDASExplorerGraphVisuals"} {
		if strings.Contains(string(viewBody), banned) {
			t.Errorf("D33a-spike-2g-impl-1: production Authority view must not reference %q", banned)
		}
	}
}
