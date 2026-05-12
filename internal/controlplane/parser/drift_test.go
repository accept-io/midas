package parser

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/types"
)

const validDriftDefinitionYAML = `apiVersion: midas.accept.io/v1
kind: DriftDefinition
metadata:
  id: surface-escalation-drift
  name: Surface Escalation Drift
spec:
  name: Surface Escalation Drift
  description: Tracks escalation-rate drift for a decision surface.
  business_owner: risk-governance
  technical_owner: platform-governance
  target:
    kind: decision_surface
    id: surf-card-dispute-triage
  origin: manual
  managed: true
  metrics:
    - metric_id: escalation-rate
      drift_type: outcome
      baseline_strategy: since_last_governed_change
      window_seconds: 604800
      baseline_window_seconds: 0
      cadence: day
      threshold_direction: ascending
      warning_threshold: 0.10
      breached_threshold: 0.25
      governance_expectation_ref: ""
      governance_expectation_ver: 0
      description: Escalation-rate drift over a seven-day window.
lifecycle:
  effective_from: "2026-01-01T00:00:00Z"
  status: review
`

func TestParser_DriftDefinition_ValidYAML(t *testing.T) {
	parsed, err := ParseYAML([]byte(validDriftDefinitionYAML))
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	if parsed.Kind != types.KindDriftDefinition {
		t.Errorf("Kind: want %q, got %q", types.KindDriftDefinition, parsed.Kind)
	}
	if parsed.ID != "surface-escalation-drift" {
		t.Errorf("ID: want %q, got %q", "surface-escalation-drift", parsed.ID)
	}
	doc, ok := parsed.Doc.(types.DriftDefinitionDocument)
	if !ok {
		t.Fatalf("parsed.Doc: want types.DriftDefinitionDocument, got %T", parsed.Doc)
	}
	if doc.Spec.Name != "Surface Escalation Drift" {
		t.Errorf("Spec.Name: %q", doc.Spec.Name)
	}
	if doc.Spec.Target.Kind != "decision_surface" {
		t.Errorf("Spec.Target.Kind: want %q, got %q", "decision_surface", doc.Spec.Target.Kind)
	}
	if doc.Spec.Target.ID != "surf-card-dispute-triage" {
		t.Errorf("Spec.Target.ID: %q", doc.Spec.Target.ID)
	}
	if len(doc.Spec.Metrics) != 1 {
		t.Fatalf("Spec.Metrics: want 1, got %d", len(doc.Spec.Metrics))
	}
	m := doc.Spec.Metrics[0]
	if m.MetricID != "escalation-rate" {
		t.Errorf("MetricID: %q", m.MetricID)
	}
	if m.DriftType != "outcome" {
		t.Errorf("DriftType: %q", m.DriftType)
	}
	if m.BaselineStrategy != "since_last_governed_change" {
		t.Errorf("BaselineStrategy: %q", m.BaselineStrategy)
	}
	if m.WindowSeconds != 604800 {
		t.Errorf("WindowSeconds: %d", m.WindowSeconds)
	}
	if m.Cadence != "day" {
		t.Errorf("Cadence: %q", m.Cadence)
	}
	if m.ThresholdDirection != "ascending" {
		t.Errorf("ThresholdDirection: %q", m.ThresholdDirection)
	}
	if m.WarningThreshold != 0.10 {
		t.Errorf("WarningThreshold: %v", m.WarningThreshold)
	}
	if m.BreachedThreshold != 0.25 {
		t.Errorf("BreachedThreshold: %v", m.BreachedThreshold)
	}
	if doc.Lifecycle.EffectiveFrom != "2026-01-01T00:00:00Z" {
		t.Errorf("Lifecycle.EffectiveFrom: %q", doc.Lifecycle.EffectiveFrom)
	}
}

func TestParser_DriftDefinition_StrictUnknownFieldsRejected(t *testing.T) {
	const yamlWithUnknownField = `apiVersion: midas.accept.io/v1
kind: DriftDefinition
metadata:
  id: surface-escalation-drift
spec:
  name: Test
  business_owner: owner
  technical_owner: owner
  target:
    kind: decision_surface
    id: surf-x
  unknown_field: should_be_rejected
  metrics:
    - metric_id: escalation-rate
      drift_type: outcome
      baseline_strategy: rolling
      window_seconds: 3600
      cadence: hour
      threshold_direction: ascending
      warning_threshold: 0.10
      breached_threshold: 0.25
`
	_, err := ParseYAML([]byte(yamlWithUnknownField))
	if err == nil {
		t.Fatal("ParseYAML must reject unknown fields under strict unmarshal")
	}
}

func TestParser_UnsupportedKind_MentionsDriftDefinition(t *testing.T) {
	const yamlBadKind = `apiVersion: midas.accept.io/v1
kind: WeirdNewKind
metadata:
  id: weird-x
`
	_, err := ParseYAML([]byte(yamlBadKind))
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
	if !strings.Contains(err.Error(), "DriftDefinition") {
		t.Errorf("unsupported-kind error message must list DriftDefinition; got: %v", err)
	}
}
