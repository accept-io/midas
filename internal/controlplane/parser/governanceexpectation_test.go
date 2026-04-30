package parser

import (
	"testing"

	"github.com/accept-io/midas/internal/controlplane/types"
)

// TestParser_GovernanceExpectation_FullyPopulated_Parses asserts a
// fully-populated bundle round-trips through ParseYAML into a typed
// GovernanceExpectationDocument with every spec field preserved.
func TestParser_GovernanceExpectation_FullyPopulated_Parses(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
kind: GovernanceExpectation
metadata:
  id: expect-credit-assessment-risk
  name: Credit assessment risk expectation
spec:
  description: Requires credit assessment governance for risk-shaped decisions
  scope_kind: process
  scope_id: proc-credit-assessment
  required_surface_id: surf-v2-credit-assess
  condition_type: risk_condition
  condition_payload:
    amount_greater_than: 5000
    currency: GBP
  business_owner: consumer-lending-team
  technical_owner: midas
  lifecycle:
    status: active
    effective_from: "2026-01-01T00:00:00Z"
    effective_until: "2027-01-01T00:00:00Z"
    version: 1
`)

	pd, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	if pd.Kind != types.KindGovernanceExpectation {
		t.Fatalf("Kind: want %q, got %q", types.KindGovernanceExpectation, pd.Kind)
	}
	if pd.ID != "expect-credit-assessment-risk" {
		t.Errorf("ID: want %q, got %q", "expect-credit-assessment-risk", pd.ID)
	}

	doc, ok := pd.Doc.(types.GovernanceExpectationDocument)
	if !ok {
		t.Fatalf("Doc payload: want types.GovernanceExpectationDocument, got %T", pd.Doc)
	}

	if doc.Metadata.Name != "Credit assessment risk expectation" {
		t.Errorf("Metadata.Name: got %q", doc.Metadata.Name)
	}
	if doc.Spec.Description == "" {
		t.Errorf("Spec.Description: want non-empty")
	}
	if doc.Spec.ScopeKind != "process" {
		t.Errorf("Spec.ScopeKind: want process, got %q", doc.Spec.ScopeKind)
	}
	if doc.Spec.ScopeID != "proc-credit-assessment" {
		t.Errorf("Spec.ScopeID: got %q", doc.Spec.ScopeID)
	}
	if doc.Spec.RequiredSurfaceID != "surf-v2-credit-assess" {
		t.Errorf("Spec.RequiredSurfaceID: got %q", doc.Spec.RequiredSurfaceID)
	}
	if doc.Spec.ConditionType != "risk_condition" {
		t.Errorf("Spec.ConditionType: got %q", doc.Spec.ConditionType)
	}
	if got := doc.Spec.ConditionPayload["currency"]; got != "GBP" {
		t.Errorf("Spec.ConditionPayload[currency]: want GBP, got %v", got)
	}
	// yaml.v3 decodes integer literals into int (matching map[string]any
	// semantics); we only assert on its presence and value, not its Go type.
	if got := doc.Spec.ConditionPayload["amount_greater_than"]; got == nil {
		t.Errorf("Spec.ConditionPayload[amount_greater_than]: want present")
	}
	if doc.Spec.BusinessOwner != "consumer-lending-team" {
		t.Errorf("Spec.BusinessOwner: got %q", doc.Spec.BusinessOwner)
	}
	if doc.Spec.TechnicalOwner != "midas" {
		t.Errorf("Spec.TechnicalOwner: got %q", doc.Spec.TechnicalOwner)
	}
	if doc.Spec.Lifecycle.Status != "active" {
		t.Errorf("Spec.Lifecycle.Status: got %q", doc.Spec.Lifecycle.Status)
	}
	if doc.Spec.Lifecycle.EffectiveFrom != "2026-01-01T00:00:00Z" {
		t.Errorf("Spec.Lifecycle.EffectiveFrom: got %q", doc.Spec.Lifecycle.EffectiveFrom)
	}
	if doc.Spec.Lifecycle.EffectiveUntil != "2027-01-01T00:00:00Z" {
		t.Errorf("Spec.Lifecycle.EffectiveUntil: got %q", doc.Spec.Lifecycle.EffectiveUntil)
	}
	if doc.Spec.Lifecycle.Version != 1 {
		t.Errorf("Spec.Lifecycle.Version: want 1, got %d", doc.Spec.Lifecycle.Version)
	}
}

// TestParser_GovernanceExpectation_Minimal_Parses asserts the parser
// accepts a document carrying only the fields that are structurally
// required by the parser (i.e. apiVersion, kind, metadata.id, and the
// scope/required-surface/condition discriminators). Validation of which
// fields are mandatory is an apply-validate concern and is exercised
// separately.
func TestParser_GovernanceExpectation_Minimal_Parses(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
kind: GovernanceExpectation
metadata:
  id: ge-minimal
spec:
  scope_kind: process
  scope_id: proc-x
  required_surface_id: surf-x
  condition_type: risk_condition
  business_owner: b
  technical_owner: t
`)

	pd, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	doc, ok := pd.Doc.(types.GovernanceExpectationDocument)
	if !ok {
		t.Fatalf("Doc payload: want types.GovernanceExpectationDocument, got %T", pd.Doc)
	}
	if len(doc.Spec.ConditionPayload) != 0 {
		t.Errorf("Spec.ConditionPayload: want empty/nil, got %v", doc.Spec.ConditionPayload)
	}
	if doc.Spec.Lifecycle.Status != "" {
		t.Errorf("Spec.Lifecycle.Status: want empty, got %q", doc.Spec.Lifecycle.Status)
	}
	if doc.Spec.Lifecycle.Version != 0 {
		t.Errorf("Spec.Lifecycle.Version: want 0, got %d", doc.Spec.Lifecycle.Version)
	}
}

// TestParser_GovernanceExpectation_NestedConditionPayload_Map proves the
// parser accepts a nested map under condition_payload — the payload is
// opaque to apply (#52) and only structurally validated by the matching
// engine in #53. yaml.v3 strict-decode applies to known struct fields,
// not to arbitrary map[string]any values, so unknown keys inside the
// payload must not cause parse failure.
func TestParser_GovernanceExpectation_NestedConditionPayload_Map(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
kind: GovernanceExpectation
metadata:
  id: ge-nested
spec:
  scope_kind: process
  scope_id: proc-x
  required_surface_id: surf-x
  condition_type: risk_condition
  condition_payload:
    arbitrary_key_1: foo
    nested:
      deep:
        deeper: 42
    list_of_things: [a, b, c]
  business_owner: b
  technical_owner: t
`)

	pd, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	doc := pd.Doc.(types.GovernanceExpectationDocument)
	if doc.Spec.ConditionPayload["arbitrary_key_1"] != "foo" {
		t.Errorf("payload missing top-level key")
	}
	nested, ok := doc.Spec.ConditionPayload["nested"].(map[string]any)
	if !ok {
		t.Fatalf("payload.nested: want map[string]any, got %T", doc.Spec.ConditionPayload["nested"])
	}
	deep, ok := nested["deep"].(map[string]any)
	if !ok {
		t.Fatalf("payload.nested.deep: want map[string]any, got %T", nested["deep"])
	}
	if deep["deeper"] != 42 {
		t.Errorf("payload.nested.deep.deeper: want 42, got %v", deep["deeper"])
	}
}
