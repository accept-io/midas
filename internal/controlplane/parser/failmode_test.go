package parser

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/types"
)

const validFailModePolicyYAML = `apiVersion: midas.accept.io/v1
kind: FailModePolicy
metadata:
  id: default-fail-mode
  name: Default Fail-Mode Policy
spec:
  name: Default Fail-Mode Policy
  description: Closed-only baseline.
  business_owner: owner@example.com
  technical_owner: platform-team
  rules:
    - correctness_class: governance_integrity
      permitted_mode: closed
    - correctness_class: persistence
      permitted_mode: closed
    - correctness_class: input
      permitted_mode: not_applicable
    - correctness_class: resource
      permitted_mode: closed
      reason: policy evaluator unreachable
    - correctness_class: consistency
      permitted_mode: closed
  origin: manual
lifecycle:
  effective_from: "2026-01-01T00:00:00Z"
`

func TestParser_FailModePolicy_ValidYAML(t *testing.T) {
	parsed, err := ParseYAML([]byte(validFailModePolicyYAML))
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	if parsed.Kind != types.KindFailModePolicy {
		t.Errorf("Kind: want %q, got %q", types.KindFailModePolicy, parsed.Kind)
	}
	if parsed.ID != "default-fail-mode" {
		t.Errorf("ID: want %q, got %q", "default-fail-mode", parsed.ID)
	}
	doc, ok := parsed.Doc.(types.FailModePolicyDocument)
	if !ok {
		t.Fatalf("parsed.Doc: want types.FailModePolicyDocument, got %T", parsed.Doc)
	}
	if doc.Spec.Name != "Default Fail-Mode Policy" {
		t.Errorf("Spec.Name: want %q, got %q", "Default Fail-Mode Policy", doc.Spec.Name)
	}
	if doc.Spec.BusinessOwner != "owner@example.com" {
		t.Errorf("Spec.BusinessOwner: want %q, got %q", "owner@example.com", doc.Spec.BusinessOwner)
	}
	if len(doc.Spec.Rules) != 5 {
		t.Fatalf("Spec.Rules: want 5, got %d", len(doc.Spec.Rules))
	}
	// Verify the resource rule round-trips reason text.
	var sawResourceReason bool
	for _, r := range doc.Spec.Rules {
		if r.CorrectnessClass == "resource" && r.Reason == "policy evaluator unreachable" {
			sawResourceReason = true
			break
		}
	}
	if !sawResourceReason {
		t.Errorf("rule reason did not round-trip; got rules: %+v", doc.Spec.Rules)
	}
	if doc.Spec.Origin != "manual" {
		t.Errorf("Spec.Origin: want %q, got %q", "manual", doc.Spec.Origin)
	}
	if doc.Lifecycle.EffectiveFrom != "2026-01-01T00:00:00Z" {
		t.Errorf("Lifecycle.EffectiveFrom: want RFC3339, got %q", doc.Lifecycle.EffectiveFrom)
	}
}

func TestParser_FailModePolicy_StrictUnknownFieldsRejected(t *testing.T) {
	const yamlWithUnknownField = `apiVersion: midas.accept.io/v1
kind: FailModePolicy
metadata:
  id: default-fail-mode
spec:
  name: Default
  business_owner: owner@example.com
  technical_owner: platform-team
  unknown_field: should_be_rejected
  rules:
    - correctness_class: input
      permitted_mode: not_applicable
`
	_, err := ParseYAML([]byte(yamlWithUnknownField))
	if err == nil {
		t.Fatal("expected strict-unknown-field error, got nil")
	}
	if !strings.Contains(err.Error(), "FailModePolicy") {
		t.Errorf("error should reference FailModePolicy; got %v", err)
	}
}

func TestParser_FailModePolicy_DoesNotAffectOtherKinds(t *testing.T) {
	// Sanity: a Profile document still parses after the FailModePolicy
	// case was added.
	const profileYAML = `apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: prof-test
spec:
  surface_id: surf-test
  policy:
    fail_mode: closed
`
	parsed, err := ParseYAML([]byte(profileYAML))
	if err != nil {
		t.Fatalf("ParseYAML(Profile): %v", err)
	}
	if parsed.Kind != types.KindProfile {
		t.Errorf("Kind: want %q, got %q", types.KindProfile, parsed.Kind)
	}
}
