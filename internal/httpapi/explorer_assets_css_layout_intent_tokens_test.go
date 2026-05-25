package httpapi

// explorer_assets_css_layout_intent_tokens_test.go — D38d pin set
// for the six layout-intent tokens added to tokens.css.
//
// The pins cover:
//   1. Each of the six tokens is present and resolves to the
//      expected --space-N primitive.
//   2. The layout-intent block sits within 30 lines of the
//      --space-* primitive block (adjacency invariant).
//   3. No --layout-* token appears outside tokens.css today
//      (foundation-preservation pin against accidental
//      consumer adoption before the D38e migration tranche).
//   4. No --layout-* token appears in the light-mode override
//      block (theme-neutral invariant).
//
// The pins are deliberately strict so D38e (consumer migration)
// has stable ground to build on, and so future drift from the
// layout-intent token system fails loudly at `go test` time.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// layoutIntentToken pairs each declared token with the
// primitive it must resolve to. Order matches the declaration
// order in tokens.css.
type layoutIntentToken struct {
	name      string
	primitive string
}

var layoutIntentTokens = []layoutIntentToken{
	{name: "--layout-page-margin", primitive: "var(--space-5)"},
	{name: "--layout-section-gap", primitive: "var(--space-5)"},
	{name: "--layout-card-padding", primitive: "var(--space-4)"},
	{name: "--layout-element-gap", primitive: "var(--space-3)"},
	{name: "--layout-inline-gap", primitive: "var(--space-2)"},
	{name: "--layout-tight-gap", primitive: "var(--space-1)"},
}

// TestLayoutIntent_TokensPresentAndMapToPrimitives pins that
// each of the six layout-intent tokens is declared in tokens.css
// and that its value is the documented var(--space-N) reference.
// A raw pixel value or a different primitive fails the test.
func TestLayoutIntent_TokensPresentAndMapToPrimitives(t *testing.T) {
	tokensCSS := readLayoutIntentTokensCSS(t)
	for _, tok := range layoutIntentTokens {
		// Match `<name>:\s*<primitive>` allowing arbitrary whitespace
		// between the colon and the value. The trailing `;` is required
		// so we don't accidentally match a declaration that consumes
		// the token (e.g. inside a comment or another property).
		re := regexp.MustCompile(
			regexp.QuoteMeta(tok.name) +
				`\s*:\s*` +
				regexp.QuoteMeta(tok.primitive) +
				`\s*;`,
		)
		if !re.MatchString(tokensCSS) {
			t.Errorf("D38d: token %q must be declared as %q in tokens.css", tok.name, tok.primitive)
		}
	}
}

// TestLayoutIntent_BlockAdjacentToPrimitives pins that the
// layout-intent block sits within 30 lines of the --space-*
// primitive block. The 30-line ceiling is small enough to catch
// fragmentation (someone moving the layout block to a different
// section of the file) but lenient enough to allow comment
// reflow.
func TestLayoutIntent_BlockAdjacentToPrimitives(t *testing.T) {
	tokensCSS := readLayoutIntentTokensCSS(t)
	lines := strings.Split(tokensCSS, "\n")

	space6Line := -1
	firstLayoutLine := -1
	for i, line := range lines {
		if space6Line < 0 && strings.Contains(line, "--space-6:") {
			space6Line = i
		}
		if firstLayoutLine < 0 && strings.Contains(line, "--layout-page-margin") {
			firstLayoutLine = i
		}
	}
	if space6Line < 0 {
		t.Fatal("D38d: --space-6 primitive line not found in tokens.css")
	}
	if firstLayoutLine < 0 {
		t.Fatal("D38d: --layout-page-margin line not found in tokens.css")
	}
	if firstLayoutLine <= space6Line {
		t.Errorf("D38d: layout-intent block must appear AFTER --space-6 (space6=%d, layout=%d)", space6Line, firstLayoutLine)
	}
	const maxGap = 30
	if gap := firstLayoutLine - space6Line; gap > maxGap {
		t.Errorf("D38d: layout-intent block must sit within %d lines of --space-6 (gap=%d)", maxGap, gap)
	}
}

// TestLayoutIntent_NotInLightThemeBlock pins that no
// --layout-* token appears inside the :root[data-theme="light"]
// override block. The layout tokens are theme-neutral.
func TestLayoutIntent_NotInLightThemeBlock(t *testing.T) {
	tokensCSS := readLayoutIntentTokensCSS(t)
	lightStart := strings.Index(tokensCSS, `:root[data-theme="light"]`)
	if lightStart < 0 {
		t.Fatal("D38d: :root[data-theme=\"light\"] block not found in tokens.css")
	}
	// The light block runs from its declaration to the closing
	// brace of the rule. Find the matching close brace by counting.
	lightSection := tokensCSS[lightStart:]
	braceOpen := strings.Index(lightSection, "{")
	if braceOpen < 0 {
		t.Fatal("D38d: light-mode block has no opening brace")
	}
	depth := 0
	closeIdx := -1
	for i := braceOpen; i < len(lightSection); i++ {
		switch lightSection[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				closeIdx = i
			}
		}
		if closeIdx >= 0 {
			break
		}
	}
	if closeIdx < 0 {
		t.Fatal("D38d: light-mode block has no closing brace")
	}
	body := lightSection[braceOpen : closeIdx+1]
	if strings.Contains(body, "--layout-") {
		t.Errorf("D38d: --layout-* token leaked into :root[data-theme=\"light\"] block; layout tokens are theme-neutral")
	}
}

// layoutConsumerCase pairs a CSS consumer file with a substring
// that must appear in it. The substring binds the layout-intent
// token to its expected consumer selector so a future tranche
// that detaches the token from the selector fails this pin.
type layoutConsumerCase struct {
	name      string // subtest name (also surfaces in failure output)
	file      string // CSS filename under explorer/assets/css/
	mustHave  string // substring required somewhere in the file
}

// layoutConsumerCases enumerates the consumer-side pins added by
// D38e. Each entry corresponds to a documented migration in the
// D38e tranche report. Adding a new consumer in a future tranche
// (D38f) appends a row; removing one is a deliberate regression
// signal.
var layoutConsumerCases = []layoutConsumerCase{
	// Page-margin pins — four top-level views plus the two
	// previously-flush outliers (capabilities, drift).
	{name: "Services_UsesPageMargin", file: "services.css",
		mustHave: ".services-view {\n    padding: var(--layout-page-margin)"},
	{name: "Records_UsesPageMargin", file: "records.css",
		mustHave: ".records-view { padding: var(--layout-page-margin)"},
	{name: "Capabilities_UsesPageMargin", file: "capabilities.css",
		mustHave: ".capabilities-catalogue { padding: var(--layout-page-margin)"},
	{name: "Drift_UsesPageMargin", file: "drift.css",
		mustHave: "padding: var(--layout-page-margin)"},

	// Card-padding pins — the three catalogue cards converged on
	// 14px var(--layout-card-padding). The vertical 14px is the
	// documented SKIP per the D38e decision table.
	{name: "RecordsMetric_UsesCardPadding", file: "records.css",
		mustHave: "padding: 14px var(--layout-card-padding)"},
	{name: "ServicesBsCard_UsesCardPadding", file: "services.css",
		mustHave: "padding: 14px var(--layout-card-padding)"},
	{name: "CapabilitiesCard_UsesCardPadding", file: "capabilities.css",
		mustHave: "padding: 14px var(--layout-card-padding)"},

	// Section-gap pins — grid-level gaps and major section margins
	// migrated to var(--layout-section-gap).
	{name: "RecordsGrid_UsesSectionGap", file: "records.css",
		mustHave: "gap: var(--layout-section-gap)"},
	{name: "ServicesGrid_UsesSectionGap", file: "services.css",
		mustHave: "gap: var(--layout-section-gap)"},
}

// TestLayoutIntent_ConsumersUseTokens replaces the prior
// "NoConsumerUsageYet" pin from D38d. Each subtest asserts that
// the named consumer file contains the expected --layout-* token
// reference. This pins the D38e migration in both directions:
// removing the consumer reference fails the test, and the test
// itself documents the canonical consumer selector for each
// layout-intent token.
func TestLayoutIntent_ConsumersUseTokens(t *testing.T) {
	cssDir := filepath.Join(repoRoot(t), "internal", "httpapi", "explorer", "assets", "css")
	for _, c := range layoutConsumerCases {
		t.Run(c.name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(cssDir, c.file))
			if err != nil {
				t.Fatalf("read %s: %v", c.file, err)
			}
			if !strings.Contains(string(body), c.mustHave) {
				t.Errorf("D38e: %s must contain %q", c.file, c.mustHave)
			}
		})
	}
}

// readLayoutIntentTokensCSS reads tokens.css from the filesystem
// (not via the HTTP test server) so the adjacency test can
// reason about line numbers in the source file rather than the
// served body, which may differ if the server applies any
// transformation.
func readLayoutIntentTokensCSS(t *testing.T) string {
	t.Helper()
	p := filepath.Join(repoRoot(t), "internal", "httpapi", "explorer", "assets", "css", "tokens.css")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}
