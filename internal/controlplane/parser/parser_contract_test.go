package parser

// parser_contract_test.go — bundle-parser contract tests.
//
// These tests define, in executable form, the behaviour the control-plane
// YAML parser is committed to today. They are deliberately read-only of
// production behaviour: nothing here changes how ParseYAML or
// ParseYAMLStream actually work. The purpose is to pin the contract so
// accidental regressions fail a test rather than ship silently.
//
// Each section's prose states the contract, and the tests that follow
// enforce it. When a future change intentionally alters one of these
// contracts, updating the test is the required way to signal that.

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Contract: structurally malformed YAML returns an error from ParseYAML and
// from ParseYAMLStream (it does not panic, does not silently produce an
// empty document, and does not hang).
// ---------------------------------------------------------------------------

func TestParserContract_MalformedYAML_ReturnsError(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{
			name: "tab_indentation_in_mapping",
			yaml: "apiVersion: midas.accept.io/v1\nkind: Surface\nmetadata:\n\tid: payment.execute\nspec:\n\tcategory: financial\n",
		},
		{
			name: "unclosed_flow_sequence",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: [financial, high`,
		},
		{
			name: "unclosed_flow_mapping",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata: {id: payment.execute,
spec:
  category: financial`,
		},
		{
			name: "unterminated_double_quoted_string",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: "payment.execute
spec:
  category: financial`,
		},
		{
			name: "mixed_sequence_and_mapping_at_same_level",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  - financial
  category: high`,
		},
		{
			name: "binary_garbage_header",
			yaml: "\x00\x01\x02\xffapiVersion: midas.accept.io/v1\nkind: Surface\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/ParseYAML", func(t *testing.T) {
			_, err := ParseYAML([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error for malformed YAML %q, got nil", tc.name)
			}
		})
		t.Run(tc.name+"/ParseYAMLStream", func(t *testing.T) {
			_, err := ParseYAMLStream([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error for malformed YAML %q, got nil", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Contract: a document missing apiVersion is rejected by ParseYAML, and is
// also rejected when the stream contains at least one real document. This
// makes the "apiVersion is required" contract explicit at both entry points.
// ---------------------------------------------------------------------------

func TestParserContract_MissingAPIVersion_StreamFails(t *testing.T) {
	// Stream with one document that's missing apiVersion.
	data := []byte(`kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial
`)

	if _, err := ParseYAML(data); err == nil || !strings.Contains(err.Error(), "missing apiVersion") {
		t.Errorf("ParseYAML: expected 'missing apiVersion' error, got %v", err)
	}
	if _, err := ParseYAMLStream(data); err == nil {
		t.Fatal("ParseYAMLStream: expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Contract: a document missing kind is rejected by ParseYAML and
// ParseYAMLStream.
// ---------------------------------------------------------------------------

func TestParserContract_MissingKind_StreamFails(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
metadata:
  id: payment.execute
spec:
  category: financial
`)

	if _, err := ParseYAML(data); err == nil || !strings.Contains(err.Error(), "missing kind") {
		t.Errorf("ParseYAML: expected 'missing kind' error, got %v", err)
	}
	if _, err := ParseYAMLStream(data); err == nil {
		t.Fatal("ParseYAMLStream: expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Contract: missing metadata.id is NOT caught at parse time.
//
// The parser accepts the document and returns an empty ParsedDocument.ID.
// Downstream validation (see internal/controlplane/validate.ValidateDocument)
// is responsible for rejecting the document on the basis of id-format rules,
// and the bundle-level planner rejects the overall apply. This test pins
// the split of responsibility: if a future change decides to fail at parse
// time instead, this test must be updated.
// ---------------------------------------------------------------------------

func TestParserContract_MissingMetadataID_ParsesEmptyID(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{
			name: "metadata_block_present_id_absent",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  name: Payment Execution
spec:
  category: financial
`,
		},
		{
			name: "metadata_block_absent_entirely",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
spec:
  category: financial
`,
		},
		{
			name: "metadata_id_explicitly_empty_string",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: ""
spec:
  category: financial
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ParseYAML([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("parser must not reject on missing metadata.id; got err=%v", err)
			}
			if doc.ID != "" {
				t.Errorf("expected empty ID, got %q — the missing-id contract is enforced downstream, not here", doc.ID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Contract: unknown YAML fields are REJECTED at parse time.
//
// The apply-path parser uses strictUnmarshal (yaml.NewDecoder with
// KnownFields(true)) for every per-kind typed decode. Any field not
// present on the typed struct at any nesting level — top-level, metadata,
// spec, or deeper — produces a parse error that propagates through
// ApplyBundle / PlanBundle as an invalid-bundle error.
//
// This aligns the bundle parser with the startup config parser
// (internal/config/loader.go), which also uses strict decoding. A future
// change that relaxes this posture would be a contract change and must
// update this test.
//
// Scope of the contract:
//   - rejection fires on the typed decode path (per-kind structs)
//   - rejection is recursive: metadata and spec fields are checked too
//   - typos of known fields surface as unknown-field errors with the
//     exact offending key name in the error message, aiding diagnosis
// ---------------------------------------------------------------------------

func TestParserContract_UnknownFields_Rejected(t *testing.T) {
	cases := []struct {
		name           string
		yaml           string
		wantFieldToken string // substring expected in the error (usually the offending key)
	}{
		{
			name: "unknown_top_level_field",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial
unknown_top: value-should-be-rejected
`,
			wantFieldToken: "unknown_top",
		},
		{
			name: "unknown_metadata_field",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
  name: Payment Execution
  unknown_meta: value-should-be-rejected
spec:
  category: financial
`,
			wantFieldToken: "unknown_meta",
		},
		{
			name: "unknown_spec_field",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial
  unknown_spec: value-should-be-rejected
`,
			wantFieldToken: "unknown_spec",
		},
		{
			name: "typo_of_known_field_rejected",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  catgory: financial
`,
			wantFieldToken: "catgory",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseYAML([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("strict-parse contract violated: parser accepted unknown field, expected rejection")
			}
			if !strings.Contains(err.Error(), tc.wantFieldToken) {
				t.Errorf("strict-parse contract: expected error to name the offending field %q; got %q",
					tc.wantFieldToken, err.Error())
			}
		})
	}
}

// TestParserContract_UnknownFields_RejectedAcrossKinds proves the strict
// posture applies to every supported document kind, not just Surface.
func TestParserContract_UnknownFields_RejectedAcrossKinds(t *testing.T) {
	// Each case is a minimal-ish valid document for a given kind with one
	// unknown field injected under the kind's spec.
	cases := []struct {
		name string
		yaml string
	}{
		{
			name: "Agent",
			yaml: `apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-x
spec:
  type: llm_agent
  runtime:
    model: gpt-4
    version: "1"
    provider: openai
  status: active
  unknown_agent_spec: reject-me
`,
		},
		{
			name: "Profile",
			yaml: `apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: profile-x
spec:
  surface_id: s
  authority:
    decision_confidence_threshold: 0.5
    consequence_threshold:
      type: monetary
      amount: 1
      currency: USD
  policy:
    reference: rego://x
    fail_mode: closed
  unknown_profile_spec: reject-me
`,
		},
		{
			name: "Grant",
			yaml: `apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-x
spec:
  agent_id: a
  profile_id: p
  granted_by: x
  granted_at: 2025-01-01T00:00:00Z
  effective_from: 2025-01-01T00:00:00Z
  status: active
  unknown_grant_spec: reject-me
`,
		},
		{
			name: "Capability",
			yaml: `apiVersion: midas.accept.io/v1
kind: Capability
metadata:
  id: cap-x
spec:
  status: active
  unknown_capability_spec: reject-me
`,
		},
		{
			name: "Process",
			yaml: `apiVersion: midas.accept.io/v1
kind: Process
metadata:
  id: proc-x
spec:
  capability_id: cap-x
  status: active
  unknown_process_spec: reject-me
`,
		},
		{
			name: "BusinessService",
			yaml: `apiVersion: midas.accept.io/v1
kind: BusinessService
metadata:
  id: bs-x
spec:
  service_type: customer_facing
  status: active
  unknown_bs_spec: reject-me
`,
		},
		{
			name: "ProcessCapability",
			yaml: `apiVersion: midas.accept.io/v1
kind: ProcessCapability
metadata:
  id: pc-x
spec:
  process_id: p
  capability_id: c
  unknown_pc_spec: reject-me
`,
		},
		{
			name: "ProcessBusinessService",
			yaml: `apiVersion: midas.accept.io/v1
kind: ProcessBusinessService
metadata:
  id: pbs-x
spec:
  process_id: p
  business_service_id: b
  unknown_pbs_spec: reject-me
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseYAML([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("strict-parse contract violated for %s: unknown spec field was accepted", tc.name)
			}
		})
	}
}

// TestParserContract_UnknownFields_StreamPropagates proves the strict
// posture carries through ParseYAMLStream — which is the path actually
// reached by ApplyBundle and PlanBundle.
func TestParserContract_UnknownFields_StreamPropagates(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial
  mystery: reject-me
`)
	_, err := ParseYAMLStream(data)
	if err == nil {
		t.Fatal("strict-parse contract violated: ParseYAMLStream accepted unknown field")
	}
	if !strings.Contains(err.Error(), "mystery") {
		t.Errorf("expected error to name the offending field %q; got %q", "mystery", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Contract: parsing an adversarial alias structure within the 10 MiB
// HTTP-side budget completes in bounded time. It may succeed or return an
// error — both are acceptable. What is NOT acceptable is for the parser to
// hang, which would manifest in production as a stalled apply request.
//
// This test is the regression guard for a class of failure analogous to
// YAML billion-laughs / quadratic-blowup / unbounded-alias expansion. The
// runtime cap is deliberately generous to avoid flakiness on slow CI hosts
// while still being orders of magnitude below "hangs indefinitely".
//
// The input is constructed to fit well under 10 MiB serialized so that it
// mirrors what a real apply request could deliver through the HTTP body
// cap.
// ---------------------------------------------------------------------------

// buildAdversarialAliasBundle returns a YAML document that uses nested
// aliases to amplify reference count without growing the serialized size.
// The structure uses a standard quadratic-blowup shape:
//
//	level0: &a [x,x,x,x,...,x]             # width N
//	level1: &b [*a,*a,*a,...,*a]           # width N, each *a refers to level0
//	level2: &c [*b,*b,*b,...,*b]           # width N
//	level3: &d [*c,*c,*c,...,*c]           # width N
//	spec:
//	  field: *d
//
// Expansion size is O(N^4) references, while serialized size is ~O(4N).
// N=40 gives ~2.56 million references, serialized in a few kilobytes.
func buildAdversarialAliasBundle(width int) []byte {
	repeat := func(token string, n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = token
		}
		return strings.Join(parts, ", ")
	}

	var sb strings.Builder
	sb.WriteString("apiVersion: midas.accept.io/v1\n")
	sb.WriteString("kind: Surface\n")
	sb.WriteString("metadata:\n")
	sb.WriteString("  id: adversarial.alias\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  category: financial\n")
	// The adversarial payload lives on unknown fields. Under the strict
	// decoder (see TestParserContract_UnknownFields_Rejected) these keys
	// will cause the decode to return an unknown-field error; under any
	// future non-strict decode path the alias graph would be walked
	// before the fields are dropped. In either case the bounded-time
	// assertion below is what the test actually guards: the parser must
	// return (success or error) within the deadline rather than hang on
	// pathological input.
	fmt.Fprintf(&sb, "  a: &a [%s]\n", repeat(`"x"`, width))
	fmt.Fprintf(&sb, "  b: &b [%s]\n", repeat("*a", width))
	fmt.Fprintf(&sb, "  c: &c [%s]\n", repeat("*b", width))
	fmt.Fprintf(&sb, "  d: &d [%s]\n", repeat("*c", width))
	sb.WriteString("  payload: *d\n")
	return []byte(sb.String())
}

// parseWithTimeout runs parse in a goroutine and fails the test if parse
// does not return within the deadline. The goroutine is allowed to outlive
// the test function on timeout — the test has already failed by then and
// leaking a single goroutine per CI run is preferable to blocking on a
// runaway parser.
func parseWithTimeout(t *testing.T, data []byte, deadline time.Duration) (ParsedDocument, error, bool) {
	t.Helper()
	type result struct {
		doc ParsedDocument
		err error
	}
	done := make(chan result, 1)
	go func() {
		doc, err := ParseYAML(data)
		done <- result{doc: doc, err: err}
	}()
	select {
	case r := <-done:
		return r.doc, r.err, true
	case <-time.After(deadline):
		return ParsedDocument{}, nil, false
	}
}

func TestParserContract_AdversarialAliases_BoundedTime(t *testing.T) {
	// Budget constants chosen so that:
	//   - the serialized payload is well under the HTTP-side 10 MiB cap
	//     (it is in the low kilobytes at worst)
	//   - healthy parser runs complete in a handful of milliseconds on
	//     modest hardware
	//   - a runaway parser (expansion of O(width^4) refs) fails this test
	//     in bounded time rather than hanging CI
	const (
		width    = 40
		maxBytes = 10 << 20 // the apply-side HTTP body cap
		deadline = 5 * time.Second
	)

	bundle := buildAdversarialAliasBundle(width)
	if len(bundle) >= maxBytes {
		t.Fatalf("adversarial bundle is %d bytes, exceeds HTTP budget of %d", len(bundle), maxBytes)
	}

	_, err, finished := parseWithTimeout(t, bundle, deadline)
	if !finished {
		t.Fatalf("parser did not return within %s on adversarial alias input (bundle=%d bytes); possible regression in alias handling", deadline, len(bundle))
	}
	// We do not assert on err: the yaml.v3 library is free to reject the
	// input (for example via its internal alias-depth guard) or to accept
	// it. Either is acceptable; only an unbounded parse is a regression.
	_ = err
}

func TestParserContract_AdversarialAliases_StreamBoundedTime(t *testing.T) {
	// Same contract for the stream entry point, which is the one actually
	// reached by ApplyBundle / PlanBundle.
	const (
		width    = 40
		maxBytes = 10 << 20
		deadline = 5 * time.Second
	)

	bundle := buildAdversarialAliasBundle(width)
	if len(bundle) >= maxBytes {
		t.Fatalf("adversarial bundle is %d bytes, exceeds HTTP budget of %d", len(bundle), maxBytes)
	}

	done := make(chan error, 1)
	go func() {
		_, err := ParseYAMLStream(bundle)
		done <- err
	}()
	select {
	case <-done:
		// success or error — either is acceptable; only a hang fails.
	case <-time.After(deadline):
		t.Fatalf("ParseYAMLStream did not return within %s on adversarial alias input (bundle=%d bytes)", deadline, len(bundle))
	}
}
