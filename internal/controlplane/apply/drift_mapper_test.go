package apply

import (
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/drift"
)

// validDriftDefinitionDoc returns a structurally-valid Drift-1c
// document used as the baseline for mapper tests.
func validDriftDefinitionDoc() types.DriftDefinitionDocument {
	return types.DriftDefinitionDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindDriftDefinition,
		Metadata:   types.DocumentMetadata{ID: "surface-escalation-drift"},
		Spec: types.DriftDefinitionSpec{
			Name:           "Surface Escalation Drift",
			Description:    "Tracks escalation-rate drift for a decision surface.",
			BusinessOwner:  "risk-governance",
			TechnicalOwner: "platform-governance",
			Target: types.DriftTargetSpec{
				Kind: "decision_surface",
				ID:   "surf-card-dispute-triage",
			},
			Metrics: []types.DriftMetricSpec{
				{
					MetricID:           "escalation-rate",
					DriftType:          "outcome",
					BaselineStrategy:   "since_last_governed_change",
					WindowSeconds:      604800,
					Cadence:            "day",
					ThresholdDirection: "ascending",
					WarningThreshold:   0.10,
					BreachedThreshold:  0.25,
				},
			},
		},
	}
}

func TestMapDriftDefinition_SetsReviewStatus(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Lifecycle.Status = "active" // any value tolerated by validator
	now := time.Now().UTC()

	d, err := mapDriftDefinitionDocumentToDomain(doc, now, "alice", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if d.Status != drift.DriftDefinitionStatusReview {
		t.Errorf("Status: want review (forced), got %q", d.Status)
	}
}

func TestMapDriftDefinition_DefaultsOriginManaged(t *testing.T) {
	doc := validDriftDefinitionDoc()
	// Origin and Managed unset
	now := time.Now().UTC()

	d, err := mapDriftDefinitionDocumentToDomain(doc, now, "alice", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if d.Origin != drift.DriftOriginManual {
		t.Errorf("Origin: want manual default, got %q", d.Origin)
	}
	if !d.Managed {
		t.Errorf("Managed: want true default, got false")
	}
}

func TestMapDriftDefinition_PassesExplicitOriginManagedReplaces(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Origin = "auto_instrumented"
	mngd := false
	doc.Spec.Managed = &mngd
	doc.Spec.Replaces = "older-drift"
	now := time.Now().UTC()

	d, err := mapDriftDefinitionDocumentToDomain(doc, now, "alice", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if d.Origin != drift.DriftOrigin("auto_instrumented") {
		t.Errorf("Origin: want auto_instrumented, got %q", d.Origin)
	}
	if d.Managed {
		t.Errorf("Managed: want explicit false, got true")
	}
	if d.Replaces != "older-drift" {
		t.Errorf("Replaces: %q", d.Replaces)
	}
}

func TestMapDriftDefinition_ParsesLifecycleDates(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Lifecycle.EffectiveFrom = "2026-03-01T00:00:00Z"
	doc.Lifecycle.EffectiveUntil = "2026-12-31T23:59:59Z"
	now := time.Now().UTC()

	d, err := mapDriftDefinitionDocumentToDomain(doc, now, "alice", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	wantFrom, _ := time.Parse(time.RFC3339, "2026-03-01T00:00:00Z")
	if !d.EffectiveDate.Equal(wantFrom) {
		t.Errorf("EffectiveDate: want %v, got %v", wantFrom, d.EffectiveDate)
	}
	wantUntil, _ := time.Parse(time.RFC3339, "2026-12-31T23:59:59Z")
	if d.EffectiveUntil == nil || !d.EffectiveUntil.Equal(wantUntil) {
		t.Errorf("EffectiveUntil: want %v, got %v", wantUntil, d.EffectiveUntil)
	}
}

func TestMapDriftDefinition_VersionFromPlanner(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Lifecycle.Version = 99 // informational; mapper must ignore
	now := time.Now().UTC()

	d, err := mapDriftDefinitionDocumentToDomain(doc, now, "alice", 7)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if d.Version != 7 {
		t.Errorf("Version: want 7 from planner, got %d", d.Version)
	}
}

func TestMapDriftDefinition_MetricsRoundTrip(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics[0].BaselineWindowSeconds = 86400
	doc.Spec.Metrics[0].GovernanceExpectationRef = "exp-x"
	doc.Spec.Metrics[0].GovernanceExpectationVer = 0
	doc.Spec.Metrics[0].Description = "Long-window outcome PSI."

	d, err := mapDriftDefinitionDocumentToDomain(doc, time.Now().UTC(), "alice", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if len(d.Metrics) != 1 {
		t.Fatalf("Metrics len = %d, want 1", len(d.Metrics))
	}
	m := d.Metrics[0]
	if m.MetricID != "escalation-rate" {
		t.Errorf("MetricID: %q", m.MetricID)
	}
	if m.DriftType != drift.DriftTypeOutcome {
		t.Errorf("DriftType: %q", m.DriftType)
	}
	if m.BaselineStrategy != drift.BaselineStrategySinceLastGovernedChange {
		t.Errorf("BaselineStrategy: %q", m.BaselineStrategy)
	}
	if m.BaselineWindowSeconds != 86400 {
		t.Errorf("BaselineWindowSeconds: %d", m.BaselineWindowSeconds)
	}
	if m.GovernanceExpectationRef != "exp-x" {
		t.Errorf("GovernanceExpectationRef: %q", m.GovernanceExpectationRef)
	}
	if m.Description != "Long-window outcome PSI." {
		t.Errorf("Description: %q", m.Description)
	}
}

func TestMapDriftDefinition_TargetEntityRoundTrip(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Target.Kind = "ai_system_binding"
	doc.Spec.Target.ID = "binding-x"

	d, err := mapDriftDefinitionDocumentToDomain(doc, time.Now().UTC(), "alice", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if d.TargetEntityKind != drift.TargetEntityKindAISystemBinding {
		t.Errorf("TargetEntityKind: %q", d.TargetEntityKind)
	}
	if d.TargetEntityID != "binding-x" {
		t.Errorf("TargetEntityID: %q", d.TargetEntityID)
	}
}

func TestMapDriftDefinition_DomainValidationFailureSurfacesError(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics[0].DriftType = "population" // V2-deferred; rejected by drift.Validate
	_, err := mapDriftDefinitionDocumentToDomain(doc, time.Now().UTC(), "alice", 1)
	if err == nil {
		t.Fatal("mapper must reject V2-deferred drift_type via drift.Validate")
	}
}
