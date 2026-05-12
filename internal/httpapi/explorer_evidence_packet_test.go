package httpapi

// explorer_evidence_packet_test.go — D30h pins.
//
// Asserts the Runtime Evidence Integrity + Packet UI inside the
// Records detail rail:
//
//   - evidence-packet.js is served and registers
//     window.MIDASExplorerRecords.evidencePacket.
//   - The module calls the production
//     /v1/evidence/envelopes/{id}/integrity and /packet endpoints,
//     never an Explorer-isolated route, never the D30c search route.
//   - All eight documented state-copy strings appear verbatim.
//   - The module renders the packet JSON via JSON.stringify(..., null, 2).
//   - It uses the existing MIDASExplorerUtils.copyToClipboard helper.
//   - The detail rail emits Verify integrity / View evidence packet
//     buttons + the two empty panel slots in renderRecordsDetail()'s
//     output.
//   - CSS: D30h selectors are present, sit outside the D29g/D30g
//     scoped slices, and use tokens only.
//   - Read-only: forbidden mutating labels + HTTP verbs are absent.
//   - Backend stays unchanged: server.go registers no new
//     /v1/evidence/* prefixes beyond D30b/c/d/e.

import (
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
)

// d30hEvidencePacketJS fetches the new module's body.
func d30hEvidencePacketJS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/assets/js/records/evidence-packet.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("evidence-packet.js must be served; got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("evidence-packet.js must be non-empty")
	}
	return body
}

func d30hExplorerIndex(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	return rec.Body.String()
}

func d30hRecordsCSS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	return getExplorerAsset(t, srv, "/explorer/assets/css/records.css")
}

// ---------------------------------------------------------------------------
// JS source pins
// ---------------------------------------------------------------------------

func TestExplorer_AssetsJS_RecordsEvidencePacket_Served(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	for _, want := range []string{
		`window.MIDASExplorerRecords`,
		`window.MIDASExplorerRecords.evidencePacket`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-packet.js must register %q", want)
		}
	}
}

func TestExplorer_EvidencePacketJS_NamespaceAndPublicFunctions(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	for _, want := range []string{
		`function init()`,
		`function loadIntegrity(`,
		`function loadPacket(`,
		`function clear()`,
		`function renderIntegrity(`,
		`function renderPacket(`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-packet.js missing required function %q", want)
		}
	}
}

func TestExplorer_EvidencePacketJS_CallsProductionEndpoints(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	for _, want := range []string{
		`'/v1/evidence/envelopes/'`,
		`'/integrity'`,
		`'/packet'`,
		`fetch(url`,
		`credentials: 'same-origin'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-packet.js missing %q (must call production /v1/evidence/envelopes/{id}/integrity and /packet via same-origin fetch)", want)
		}
	}
}

func TestExplorer_EvidencePacketJS_DoesNotCallExplorerOrSearchRoutes(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	for _, forbidden := range []string{
		// Explorer-isolated workbench route.
		`/explorer/envelopes/`,
		// D30c search route.
		`/v1/evidence/audit-events`,
		// D30b envelope-chain route — packet already contains
		// audit_events, so the packet UI must not fetch the chain
		// separately.
		`/audit-events'`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-packet.js must NOT reference %q (use /v1/evidence/envelopes/{id}/integrity and /packet only)", forbidden)
		}
	}
}

func TestExplorer_EvidencePacketJS_UsesGETOnly(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	// Mutating HTTP method literals must not appear.
	for _, forbidden := range []string{
		`"POST"`, `'POST'`, `"PUT"`, `'PUT'`,
		`"PATCH"`, `'PATCH'`, `"DELETE"`, `'DELETE'`,
		`method: 'POST'`, `method: "POST"`,
		`method: 'PUT'`, `method: "PUT"`,
		`method: 'PATCH'`, `method: "PATCH"`,
		`method: 'DELETE'`, `method: "DELETE"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-packet.js must NOT use mutating HTTP verb literal %q", forbidden)
		}
	}
}

func TestExplorer_EvidencePacketJS_StateCopy(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	for _, want := range []string{
		`'No envelope selected.'`,
		`'Verifying evidence integrity…'`,
		`'Audit chain verified.'`,
		`'Audit chain integrity issue detected.'`,
		`'Integrity status could not be loaded.'`,
		`'Loading evidence packet…'`,
		`'Evidence packet loaded.'`,
		`'Evidence packet could not be loaded.'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-packet.js missing state copy %q", want)
		}
	}
}

func TestExplorer_EvidencePacketJS_RendersJSON(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	// Pretty-printed JSON (indent=2) and textContent assignment.
	for _, want := range []string{
		`JSON.stringify(packet, null, 2)`,
		`pre.textContent`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-packet.js must render the packet via %q", want)
		}
	}
}

func TestExplorer_EvidencePacketJS_UsesCopyToClipboardHelper(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	for _, want := range []string{
		`MIDASExplorerUtils`,
		`copyToClipboard`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-packet.js must reuse the existing clipboard helper; missing %q", want)
		}
	}
}

func TestExplorer_EvidencePacketJS_ReadOnly(t *testing.T) {
	body := d30hEvidencePacketJS(t)
	for _, forbidden := range []string{
		// Mutating action labels.
		`Delete`,
		`Approve`,
		`Deprecate`,
		`Change enforcement`,
		`Disable enforcement`,
		`Re-run`,
		`Replay`,
		`Suppress`,
		`Resolve`,
		`Annotate`,
		`Sign packet`,
		`Submit packet`,
		`Upload packet`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-packet.js must NOT contain forbidden label %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Markup pins
// ---------------------------------------------------------------------------

// TestExplorer_HTML_RecordsDetail_IntegrityAndPacketButtonsPresent pins
// that renderRecordsDetail() emits both new buttons. The buttons live
// inside the inline IIFE's HTML-string concatenation, so the literal
// strings appear in the served index.html body.
func TestExplorer_HTML_RecordsDetail_IntegrityAndPacketButtonsPresent(t *testing.T) {
	body := d30hExplorerIndex(t)
	for _, want := range []string{
		`id="records-verify-integrity-btn"`,
		`id="records-view-packet-btn"`,
		`>Verify integrity<`,
		`>View evidence packet<`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html missing detail-rail button %q", want)
		}
	}
}

// TestExplorer_HTML_RecordsDetail_PanelSlotsPresent pins the two
// empty panel slots emitted by renderRecordsDetail() so the
// evidence-packet.js module has stable targets to populate.
func TestExplorer_HTML_RecordsDetail_PanelSlotsPresent(t *testing.T) {
	body := d30hExplorerIndex(t)
	for _, want := range []string{
		`id="records-integrity-panel"`,
		`id="records-packet-panel"`,
		`data-runtime-evidence-integrity`,
		`data-runtime-evidence-packet`,
		`class="runtime-evidence-integrity-panel"`,
		`class="runtime-evidence-packet-panel"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html missing detail-rail panel slot %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// CSS pins
// ---------------------------------------------------------------------------

func TestExplorer_RecordsCSS_RuntimeEvidencePacket_SelectorsPresent(t *testing.T) {
	css := d30hRecordsCSS(t)
	for _, want := range []string{
		`.runtime-evidence-actions`,
		`.runtime-evidence-action-button`,
		`.runtime-evidence-integrity-panel`,
		`.runtime-evidence-integrity-status`,
		`.runtime-evidence-integrity-status.is-valid`,
		`.runtime-evidence-integrity-status.is-invalid`,
		`.runtime-evidence-integrity-kv`,
		`.runtime-evidence-integrity-kv-key`,
		`.runtime-evidence-integrity-kv-value`,
		`.runtime-evidence-packet-mono`,
		`.runtime-evidence-packet-panel`,
		`.runtime-evidence-packet-summary`,
		`.runtime-evidence-packet-json`,
		`.runtime-evidence-packet-state`,
		`.runtime-evidence-packet-error`,
		`.runtime-evidence-packet-actions`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("records.css missing D30h selector %q", want)
		}
	}
	// Light-mode overrides for the new selectors must also exist.
	for _, want := range []string{
		`:root[data-theme="light"] .runtime-evidence-integrity-status`,
		`:root[data-theme="light"] .runtime-evidence-packet-json`,
		`:root[data-theme="light"] .runtime-evidence-packet-error`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("records.css missing D30h light-mode override %q", want)
		}
	}
}

func TestExplorer_RecordsCSS_RuntimeEvidencePacket_UsesTokens(t *testing.T) {
	css := d30hRecordsCSS(t)
	// Slice the D30h dark-mode block from its banner comment up to
	// the existing records-metrics responsive @media rule that
	// terminates the dark-mode region. This scope mirrors the D30g
	// slice pattern and stays outside the D29g/D30g scoped slices.
	start := strings.Index(css, `D30h — Runtime Evidence Integrity + Packet panels.`)
	if start < 0 {
		t.Fatal("D30h CSS banner comment missing")
	}
	terminator := strings.Index(css[start:], "@media (max-width: 1280px) {\n    .records-metrics")
	if terminator < 0 {
		t.Fatal("expected D30h block to terminate before the records-metrics responsive @media rule")
	}
	slice := css[start : start+terminator]

	hexRe := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	if m := hexRe.FindString(slice); m != "" {
		t.Errorf("D30h CSS block must not contain raw hex value %q (use tokens)", m)
	}
	rgbaRe := regexp.MustCompile(`rgba?\(`)
	if m := rgbaRe.FindString(slice); m != "" {
		t.Errorf("D30h CSS block must not contain raw %q value (use tokens)", m)
	}
	for _, want := range []string{
		`var(--surface-container)`,
		`var(--on-surface)`,
		`var(--on-surface-variant)`,
		`var(--outline-variant)`,
		`var(--outline)`,
		`var(--primary)`,
		`var(--radius-tight)`,
		`var(--font-mono)`,
		`var(--border-hairline)`,
	} {
		if !strings.Contains(slice, want) {
			t.Errorf("D30h CSS block must use token %q", want)
		}
	}
}

// TestExplorer_RecordsCSS_D30hBlock_StartsAfterD30g pins the
// boundary between the D30g and D30h CSS blocks so the D30g
// token-only slice (which terminates at the records-metrics
// responsive @media) stays scoped to D30g only.
func TestExplorer_RecordsCSS_D30hBlock_StartsAfterD30g(t *testing.T) {
	css := d30hRecordsCSS(t)
	d30gStart := strings.Index(css, `D30g — Runtime Evidence Search panel.`)
	if d30gStart < 0 {
		t.Fatal("D30g banner missing")
	}
	d30hStart := strings.Index(css, `D30h — Runtime Evidence Integrity + Packet panels.`)
	if d30hStart < 0 {
		t.Fatal("D30h banner missing")
	}
	if d30hStart <= d30gStart {
		t.Errorf("D30h block must start AFTER the D30g block; d30g=%d d30h=%d",
			d30gStart, d30hStart)
	}
}

// ---------------------------------------------------------------------------
// Script-link order
// ---------------------------------------------------------------------------

func TestExplorer_HTML_LinksRecordsScripts_EvidencePacketOrdered(t *testing.T) {
	body := d30hExplorerIndex(t)
	searchTag := `<script src="/explorer/assets/js/records/evidence-search.js"></script>`
	packetTag := `<script src="/explorer/assets/js/records/evidence-packet.js"></script>`
	sIdx := strings.Index(body, searchTag)
	pIdx := strings.Index(body, packetTag)
	if sIdx < 0 {
		t.Fatalf("index.html missing %q", searchTag)
	}
	if pIdx < 0 {
		t.Fatalf("index.html missing %q", packetTag)
	}
	if !(sIdx < pIdx) {
		t.Errorf("evidence-packet.js must load AFTER evidence-search.js; got search=%d packet=%d", sIdx, pIdx)
	}
}

// ---------------------------------------------------------------------------
// Backend / boundary pins
// ---------------------------------------------------------------------------

func TestExplorer_EvidencePacketJS_NoNewBackendRoutes(t *testing.T) {
	// D30h is UI-only. server.go must still register exactly two
	// /v1/evidence/* HandleFunc literals (D30b prefix + D30c fixed).
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	body := string(src)
	count := strings.Count(body, `"/v1/evidence/`)
	if count != 2 {
		t.Errorf("server.go must register exactly two /v1/evidence/ HandleFunc literals (D30b prefix + D30c fixed); got %d", count)
	}
	for _, want := range []string{
		`"/v1/evidence/envelopes/"`,
		`"/v1/evidence/audit-events"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("server.go must still register %q (D30h must not remove existing routes)", want)
		}
	}
}

// TestExplorer_EvidencePacketJS_PriorScriptsStillRegistered pins
// that D30h is additive: every prior Records script tag is still in
// the served index.html.
func TestExplorer_EvidencePacketJS_PriorScriptsStillRegistered(t *testing.T) {
	body := d30hExplorerIndex(t)
	for _, want := range []string{
		`<script src="/explorer/assets/js/records/envelope-summary.js"></script>`,
		`<script src="/explorer/assets/js/records/evidence-helpers.js"></script>`,
		`<script src="/explorer/assets/js/records/audit-event-renderers.js"></script>`,
		`<script src="/explorer/assets/js/records/evidence-search.js"></script>`,
		`<script src="/explorer/assets/js/records/evidence-packet.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html must register %q", want)
		}
	}
}
