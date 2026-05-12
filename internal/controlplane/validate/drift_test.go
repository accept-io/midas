package validate

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// validDriftDefinitionDoc returns a structurally-valid DriftDefinition
// document satisfying every Drift-1c validator constraint. Tests mutate
// one field at a time and assert the resulting validator output.
func validDriftDefinitionDoc() types.DriftDefinitionDocument {
	return types.DriftDefinitionDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindDriftDefinition,
		Metadata:   types.DocumentMetadata{ID: "surface-escalation-drift", Name: "Surface Escalation Drift"},
		Spec: types.DriftDefinitionSpec{
			Name:           "Surface Escalation Drift",
			Description:    "Tracks escalation-rate drift for a decision surface.",
			BusinessOwner:  "risk-governance",
			TechnicalOwner: "platform-governance",
			Target: types.DriftTargetSpec{
				Kind: "decision_surface",
				ID:   "surf-card-dispute-triage",
			},
			Origin: "manual",
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
		Lifecycle: types.DriftDefinitionLifecycle{
			EffectiveFrom: "2026-01-01T00:00:00Z",
		},
	}
}

func wrapDrift(t *testing.T, doc types.DriftDefinitionDocument) parser.ParsedDocument {
	t.Helper()
	return parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}
}

func validateDrift(t *testing.T, doc types.DriftDefinitionDocument) []types.ValidationError {
	t.Helper()
	return ValidateDocument(wrapDrift(t, doc))
}

func TestValidateDriftDefinition_Valid(t *testing.T) {
	doc := validDriftDefinitionDoc()
	errs := validateDrift(t, doc)
	if len(errs) != 0 {
		t.Fatalf("valid DriftDefinition should produce no errors; got %d: %+v", len(errs), errs)
	}
}

func TestValidateDriftDefinition_RequiresSpecName(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Name = ""
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.name", "is required") {
		t.Errorf("missing spec.name error; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RequiresBusinessOwner(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.BusinessOwner = ""
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.business_owner", "is required") {
		t.Errorf("missing spec.business_owner error; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RequiresTechnicalOwner(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.TechnicalOwner = ""
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.technical_owner", "is required") {
		t.Errorf("missing spec.technical_owner error; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RequiresTargetKind(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Target.Kind = ""
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.target.kind", "is required") {
		t.Errorf("missing spec.target.kind error; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RequiresTargetID(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Target.ID = ""
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.target.id", "is required") {
		t.Errorf("missing spec.target.id error; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsInvalidTargetKind(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Target.Kind = "weird_kind"
	errs := validateDrift(t, doc)
	if !containsFieldErrJustField(errs, "spec.target.kind") {
		t.Errorf("invalid target.kind not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsInvalidOrigin(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Origin = "weird"
	errs := validateDrift(t, doc)
	if !containsFieldErrJustField(errs, "spec.origin") {
		t.Errorf("invalid origin not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsReplacesEqualsID(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Replaces = doc.Metadata.ID
	errs := validateDrift(t, doc)
	if !containsFieldErrJustField(errs, "spec.replaces") {
		t.Errorf("self-replaces not rejected; got: %+v", errs)
	}
}

// V1 drift type rejection — the five V2-deferred drift types must each
// be rejected. The brief mandates explicit rejection of each value.
func TestValidateDriftDefinition_RejectsExcludedDriftTypes(t *testing.T) {
	for _, banned := range []string{"population", "data", "prediction", "performance", "concept"} {
		t.Run(banned, func(t *testing.T) {
			doc := validDriftDefinitionDoc()
			doc.Spec.Metrics[0].DriftType = banned
			errs := validateDrift(t, doc)
			if !containsFieldErrJustField(errs, "spec.metrics") {
				t.Fatalf("DriftType %q must be rejected; got: %+v", banned, errs)
			}
		})
	}
}

// V2-deferred baseline strategies must be rejected.
func TestValidateDriftDefinition_RejectsExcludedBaselineStrategies(t *testing.T) {
	for _, banned := range []string{"champion_challenger", "seasonality_aware"} {
		t.Run(banned, func(t *testing.T) {
			doc := validDriftDefinitionDoc()
			doc.Spec.Metrics[0].BaselineStrategy = banned
			errs := validateDrift(t, doc)
			if !containsFieldErrJustField(errs, "spec.metrics") {
				t.Fatalf("BaselineStrategy %q must be rejected; got: %+v", banned, errs)
			}
		})
	}
}

func TestValidateDriftDefinition_RejectsDuplicateMetricIDs(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics = append(doc.Spec.Metrics, doc.Spec.Metrics[0])
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.metrics", "duplicate MetricID") {
		t.Errorf("duplicate metric id not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsBadThresholdOrdering_Ascending(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics[0].ThresholdDirection = "ascending"
	doc.Spec.Metrics[0].WarningThreshold = 0.30
	doc.Spec.Metrics[0].BreachedThreshold = 0.10
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.metrics", "ThresholdDirection") {
		t.Errorf("inverted ascending threshold not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsBadThresholdOrdering_Descending(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics[0].ThresholdDirection = "descending"
	doc.Spec.Metrics[0].WarningThreshold = 0.10
	doc.Spec.Metrics[0].BreachedThreshold = 0.30
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.metrics", "ThresholdDirection") {
		t.Errorf("inverted descending threshold not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsZeroWindowSeconds(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics[0].WindowSeconds = 0
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.metrics", "WindowSeconds") {
		t.Errorf("zero WindowSeconds not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsInvalidCadence(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics[0].Cadence = "fortnight"
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.metrics", "Cadence") {
		t.Errorf("invalid cadence not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsInvalidThresholdDirection(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics[0].ThresholdDirection = "weird"
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.metrics", "ThresholdDirection") {
		t.Errorf("invalid threshold direction not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RequiresAtLeastOneMetric(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics = nil
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "spec.metrics", "at least one") {
		t.Errorf("empty metrics not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsInvalidLifecycleStatus(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Lifecycle.Status = "weird"
	errs := validateDrift(t, doc)
	if !containsFieldErrJustField(errs, "lifecycle.status") {
		t.Errorf("invalid lifecycle.status not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_AcceptsExplicitReviewStatus(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Lifecycle.Status = "review"
	errs := validateDrift(t, doc)
	if len(errs) != 0 {
		t.Errorf("explicit review status should be accepted; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsMalformedEffectiveFrom(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Lifecycle.EffectiveFrom = "not-a-date"
	errs := validateDrift(t, doc)
	if !containsFieldErrJustField(errs, "lifecycle.effective_from") {
		t.Errorf("malformed effective_from not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsEffectiveUntilNotAfterFrom(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Lifecycle.EffectiveFrom = "2026-01-01T00:00:00Z"
	doc.Lifecycle.EffectiveUntil = "2026-01-01T00:00:00Z"
	errs := validateDrift(t, doc)
	if !containsFieldErr(errs, "lifecycle.effective_until", "must be after") {
		t.Errorf("effective_until == from not rejected; got: %+v", errs)
	}
}

func TestValidateDriftDefinition_RejectsNegativeLifecycleVersion(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Lifecycle.Version = -1
	errs := validateDrift(t, doc)
	if !containsFieldErrJustField(errs, "lifecycle.version") {
		t.Errorf("negative lifecycle.version not rejected; got: %+v", errs)
	}
}

// Confirm the validator's V1-rejection error message names the rejected
// drift type explicitly and mentions the V2 / Drift-1a deferral so
// operator-facing apply output is clear.
func TestValidateDriftDefinition_V2RejectionMessageMentionsType(t *testing.T) {
	doc := validDriftDefinitionDoc()
	doc.Spec.Metrics[0].DriftType = "data"
	errs := validateDrift(t, doc)
	var found bool
	for _, e := range errs {
		if e.Field == "spec.metrics" && strings.Contains(e.Message, "data") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("V2-rejection error must name the rejected type; got: %+v", errs)
	}
}
