package httpapi

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl2_test.go — D33a-spike-2g-impl-2.
//
// Adds a local MIDAS icon registry (assets/js/icons/midas-icons.js)
// that exposes the 30-icon Lucide subset vendored in impl-1. These
// tests pin the registry file, its public API, its coverage of the
// final subset, source-string consistency against the vendored .svg
// files, accessibility/sanitisation guards on inlineSvg, the
// Cytoscape data-URI helper, hygiene (no external URLs / no fetches),
// and that NO graph rendering code consumes the registry yet.
//
// Tests are source-string / file-system based, matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl2RegistryPath = "explorer/assets/js/icons/midas-icons.js"
	d33aSpike2gImpl2IndexHTML    = "explorer/index.html"
	d33aSpike2gImpl2PocPath      = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl2ViewPath     = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl2LucideDir    = "explorer/assets/icons/lucide"
)

// d33aSpike2gImpl2Catalogue is the canonical mapping of MIDAS-facing
// keys to Lucide filenames as required by the tranche brief. It is
// duplicated here so the test is independent of the JS source layout.
type d33aSpike2gImpl2Entry struct {
	Name   string
	Lucide string
}

func d33aSpike2gImpl2ExpectedCatalogue() []d33aSpike2gImpl2Entry {
	return []d33aSpike2gImpl2Entry{
		// Authority Graph + shared
		{"authorityBusinessService", "building-2"},
		{"authorityDecisionSurface", "workflow"},
		{"authorityProfile", "shield-check"},
		{"authorityGrant", "file-check-2"},
		{"authorityAgent", "bot"},
		{"authorityFailModePolicy", "triangle-alert"},
		{"authorityEscalationTarget", "arrow-up-from-line"},
		// Context-only
		{"contextCapability", "puzzle"},
		{"contextProcess", "route"},
		{"contextAiSystem", "cpu"},
		{"contextCoverage", "target"},
		{"contextAiSystemBinding", "link-2"},
		// Workbench chrome
		{"graphRefresh", "refresh-cw"},
		{"chromeSettings", "settings"},
		{"chromeInfo", "info"},
		{"chromeHelp", "circle-help"},
		{"chromeExternal", "external-link"},
		{"chromeDownload", "download"},
		// Lifecycle
		{"lifecycleDraft", "circle-dashed"},
		{"lifecycleReview", "eye"},
		{"lifecycleActive", "circle-check"},
		{"lifecycleDeprecated", "archive"},
		{"lifecycleRetired", "archive-x"},
		// Severity / state
		{"severityCritical", "octagon-alert"},
		{"stateBlocked", "circle-x"},
		{"stateSuspended", "circle-pause"},
		// Audit / integrity
		{"auditIntegrityVerified", "lock"},
		{"auditIntegrityBroken", "lock-open"},
		// Posture (corrected `posture*` naming)
		{"postureDrift", "trending-down"},
		{"postureDiagnostics", "stethoscope"},
	}
}

func d33aSpike2gImpl2ReadRegistry(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl2RegistryPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-2: cannot read registry at %s: %v", d33aSpike2gImpl2RegistryPath, err)
	}
	return string(b)
}

// ── 1. Registry file + script load ─────────────────────────────────────

// TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryFilePresent pins
// the registry source file exists.
func TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryFilePresent(t *testing.T) {
	if _, err := os.Stat(d33aSpike2gImpl2RegistryPath); err != nil {
		t.Fatalf("D33a-spike-2g-impl-2: registry file %s missing: %v", d33aSpike2gImpl2RegistryPath, err)
	}
}

// TestExplorer_D33aSpike2gImpl2_IndexLoadsMidasIconsBeforeGraphConsumers
// pins that index.html loads midas-icons.js, AND that it is loaded
// before authority-cytoscape-poc.js so any future PoC swap can resolve
// the registry namespace at module-load time.
func TestExplorer_D33aSpike2gImpl2_IndexLoadsMidasIconsBeforeGraphConsumers(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl2IndexHTML)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-2: cannot read %s: %v", d33aSpike2gImpl2IndexHTML, err)
	}
	body := string(b)
	const registryTag = `assets/js/icons/midas-icons.js`
	const pocTag = `assets/js/graph/authority/authority-cytoscape-poc.js`
	regIdx := strings.Index(body, registryTag)
	if regIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-2: index.html must reference %q via a <script src=...> tag", registryTag)
	}
	pocIdx := strings.Index(body, pocTag)
	if pocIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-2: index.html must still reference the Cytoscape PoC (%q)", pocTag)
	}
	if !(regIdx < pocIdx) {
		t.Errorf("D33a-spike-2g-impl-2: midas-icons.js (%d) must load BEFORE authority-cytoscape-poc.js (%d) so future consumers resolve the registry at module-load time", regIdx, pocIdx)
	}
}

// ── 2. Public API surface ──────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryExposesPublicSurface
// pins the documented public surface attached to window.
func TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryExposesPublicSurface(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)
	if !strings.Contains(body, "window.MIDASExplorerIcons") {
		t.Fatal("D33a-spike-2g-impl-2: registry must attach to window.MIDASExplorerIcons")
	}
	for _, member := range []string{"names", "has", "inlineSvg", "cytoscapeDataURI", "_sources"} {
		if !strings.Contains(body, member) {
			t.Errorf("D33a-spike-2g-impl-2: public surface missing %q", member)
		}
	}
}

// ── 3. Coverage ────────────────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryCoversFinalSubset
// pins that all 30 MIDAS-facing keys appear in the registry, that the
// `posture*` naming is correct, and that no `postur*` typo leaked in.
func TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryCoversFinalSubset(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)
	cat := d33aSpike2gImpl2ExpectedCatalogue()
	if len(cat) != 30 {
		t.Fatalf("D33a-spike-2g-impl-2: test fixture catalogue is %d entries; brief mandates 30", len(cat))
	}
	for _, entry := range cat {
		if !strings.Contains(body, entry.Name) {
			t.Errorf("D33a-spike-2g-impl-2: registry must declare MIDAS-facing key %q", entry.Name)
		}
	}

	// Corrected posture* naming MUST appear.
	for _, want := range []string{"postureDrift", "postureDiagnostics"} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-2: registry must use corrected name %q", want)
		}
	}

	// The `postur*` typo must NOT appear anywhere. We allow only the
	// substring "postur" when followed by 'e' (i.e. inside "posture").
	const needle = "postur"
	for i := 0; i+len(needle) < len(body); i++ {
		if body[i:i+len(needle)] != needle {
			continue
		}
		next := body[i+len(needle)]
		if next != 'e' {
			t.Errorf("D33a-spike-2g-impl-2: registry must not contain `postur*` typo; found near offset %d: %q", i, body[i:i+len(needle)+1])
		}
	}
}

// TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryMapsToVendoredLucideFiles
// pins that each catalogue entry's Lucide filename exists on disk
// under the vendored icons directory.
func TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryMapsToVendoredLucideFiles(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)
	for _, entry := range d33aSpike2gImpl2ExpectedCatalogue() {
		// Registry must record the lucide filename for this MIDAS key.
		// The literal `'<lucide>'` mapping appears in the catalogue
		// table inside the JS source.
		needle := "'" + entry.Lucide + "'"
		if !strings.Contains(body, needle) {
			t.Errorf("D33a-spike-2g-impl-2: registry must map %q to lucide filename %q (expected literal %q in source)", entry.Name, entry.Lucide, needle)
		}
		// And the underlying file must exist.
		filePath := filepath.Join(d33aSpike2gImpl2LucideDir, entry.Lucide+".svg")
		if _, err := os.Stat(filePath); err != nil {
			t.Errorf("D33a-spike-2g-impl-2: registry references %s but vendored file missing: %v", filePath, err)
		}
	}
}

// ── 4. SVG source consistency ──────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl2_MidasIconsRegistrySvgMatchesVendoredFiles
// pins that each vendored .svg file's content appears verbatim (after
// trailing-whitespace trim) as a string literal in midas-icons.js.
// The duplication is intentional — there is no build pipeline — but
// the JS strings MUST stay in lockstep with the disk files.
func TestExplorer_D33aSpike2gImpl2_MidasIconsRegistrySvgMatchesVendoredFiles(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)
	for _, entry := range d33aSpike2gImpl2ExpectedCatalogue() {
		filePath := filepath.Join(d33aSpike2gImpl2LucideDir, entry.Lucide+".svg")
		raw, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("D33a-spike-2g-impl-2: cannot read %s: %v", filePath, err)
			continue
		}
		trimmed := strings.TrimRight(string(raw), " \t\r\n")
		if !strings.Contains(body, trimmed) {
			t.Errorf("D33a-spike-2g-impl-2: registry must embed the verbatim contents of %s — the file body does not appear in midas-icons.js (possible drift between disk SVGs and the JS registry strings)", filePath)
		}
	}
}

// ── 5. DOM inline SVG behaviour ────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl2_InlineSvgSupportsAccessibilityOptions
// pins that the registry's inlineSvg implementation supports a
// standard accessibility option set. Source-string check.
func TestExplorer_D33aSpike2gImpl2_InlineSvgSupportsAccessibilityOptions(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)
	for _, want := range []string{
		"aria-hidden",
		`role="img"`,
		"<title>",
		"class",
		"stroke",
		"width",
		"height",
		"stroke-width",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-2: inlineSvg must support %q", want)
		}
	}
}

// TestExplorer_D33aSpike2gImpl2_InlineSvgEscapesTitleAndRejectsUnsafeOptions
// pins that sanitisation helpers exist, and that the implementation
// names the specific unsafe substrings it must guard against. This is
// a source-string pin — the live JS guard is exercised in the browser.
func TestExplorer_D33aSpike2gImpl2_InlineSvgEscapesTitleAndRejectsUnsafeOptions(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)

	// Sanitisation/escape helpers must be visibly named in source.
	for _, helper := range []string{
		"escapeText",
		"sanitiseAttribute",
		"sanitiseClassName",
		"sanitiseStroke",
		"sanitiseLength",
	} {
		if !strings.Contains(body, helper) {
			t.Errorf("D33a-spike-2g-impl-2: expected sanitisation helper %q to be defined in midas-icons.js", helper)
		}
	}

	// The unsafe-attribute pattern must explicitly enumerate the guard
	// substrings called out in the brief. These tokens must appear in
	// the rejection pattern (literal) so a reviewer can read the
	// guarded set without running the JS.
	for _, guard := range []string{
		"<",
		">",
		"javascript:",
		"url(",
		"onload",
		"onclick",
	} {
		if !strings.Contains(body, guard) {
			t.Errorf("D33a-spike-2g-impl-2: sanitisation must visibly guard against %q (token absent from source)", guard)
		}
	}
}

// ── 6. Cytoscape data URI behaviour ────────────────────────────────────

// TestExplorer_D33aSpike2gImpl2_CytoscapeDataUriApiPresent pins that
// the cytoscapeDataURI helper is defined and produces a
// data:image/svg+xml;utf8,... URI via encodeURIComponent.
func TestExplorer_D33aSpike2gImpl2_CytoscapeDataUriApiPresent(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)
	if !strings.Contains(body, "function cytoscapeDataURI") {
		t.Error("D33a-spike-2g-impl-2: cytoscapeDataURI function must be defined in midas-icons.js")
	}
	if !strings.Contains(body, "data:image/svg+xml;utf8,") {
		t.Error(`D33a-spike-2g-impl-2: cytoscapeDataURI output must use the literal prefix "data:image/svg+xml;utf8,"`)
	}
	if !strings.Contains(body, "encodeURIComponent") {
		t.Error("D33a-spike-2g-impl-2: cytoscapeDataURI must encode the SVG payload via encodeURIComponent (no raw <svg or whitespace-heavy XML in the data URI)")
	}
}

// TestExplorer_D33aSpike2gImpl2_CytoscapeDataUriUsesConcreteDefaultStroke
// pins that the data-URI default stroke is a concrete colour rather
// than `currentColor` (Cytoscape's background-image renderer does not
// propagate currentColor through to the rasterised image).
func TestExplorer_D33aSpike2gImpl2_CytoscapeDataUriUsesConcreteDefaultStroke(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)
	if !strings.Contains(body, "'#e2e8f0'") && !strings.Contains(body, `"#e2e8f0"`) {
		t.Error(`D33a-spike-2g-impl-2: cytoscapeDataURI default stroke must be a concrete colour such as "#e2e8f0", not currentColor — Cytoscape background-image renderers do not propagate currentColor`)
	}
}

// ── 7. No external assets ──────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryNoExternalUrls pins
// asset hygiene for the registry source file: no http(s) URLs other
// than the SVG XML namespace, no runtime fetches, no dynamic imports,
// no <image>/xlink:href references, no font-face declarations.
func TestExplorer_D33aSpike2gImpl2_MidasIconsRegistryNoExternalUrls(t *testing.T) {
	body := d33aSpike2gImpl2ReadRegistry(t)

	// The xmlns string appears inside every embedded SVG. Strip every
	// occurrence before scanning for forbidden URLs.
	const svgNamespace = "http://www.w3.org/2000/svg"
	stripped := strings.ReplaceAll(body, svgNamespace, "")
	for _, banned := range []string{"http://", "https://"} {
		if strings.Contains(stripped, banned) {
			t.Errorf("D33a-spike-2g-impl-2: registry must not contain external URL %q (only the SVG xmlns is allowed)", banned)
		}
	}

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"import(",
		"@font-face",
		"new Image",
		"<image",
		"xlink:href",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-2: registry must not contain %q (no runtime fetches, no dynamic imports, no external images)", banned)
		}
	}
}

// ── 8. No graph behaviour changes (production isolation) ─────────────

// TestExplorer_D33aSpike2gImpl2_NoGraphRenderingConsumesIconRegistryYet
// pins the production-isolation invariant for the icon registry:
// the production Authority view and every Context Graph module must
// NOT reference window.MIDASExplorerIcons.
//
// History: impl-2 originally required the Cytoscape PoC to also
// abstain. D33a-spike-2g-impl-3 deliberately wired the PoC to consume
// the registry, so that part of the contract moved to the impl-3
// tests. The production / Context invariants below remain in force
// and are still pinned here.
func TestExplorer_D33aSpike2gImpl2_NoGraphRenderingConsumesIconRegistryYet(t *testing.T) {
	// Production Authority view must not reference the registry.
	viewBody, err := os.ReadFile(d33aSpike2gImpl2ViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-2: cannot read Authority view at %s: %v", d33aSpike2gImpl2ViewPath, err)
	}
	if strings.Contains(string(viewBody), "MIDASExplorerIcons") {
		t.Error("D33a-spike-2g-impl-2: production Authority view must NOT reference MIDASExplorerIcons in this tranche")
	}

	// No Context Graph module may reference the registry.
	contextDir := "explorer/assets/js/graph/context"
	entries, err := os.ReadDir(contextDir)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-2: cannot read %s: %v", contextDir, err)
	}
	var contextFiles []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		contextFiles = append(contextFiles, e.Name())
	}
	sort.Strings(contextFiles)
	for _, name := range contextFiles {
		path := filepath.Join(contextDir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("D33a-spike-2g-impl-2: cannot read %s: %v", path, err)
			continue
		}
		if strings.Contains(string(b), "MIDASExplorerIcons") {
			t.Errorf("D33a-spike-2g-impl-2: Context Graph module %s must NOT reference MIDASExplorerIcons in this tranche", name)
		}
	}
}
