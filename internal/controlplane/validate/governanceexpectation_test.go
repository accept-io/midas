package validate

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// validGovernanceExpectationDoc is the canonical valid baseline for the
// per-field negative tests. Every spec field a validator could reject is
// populated; tests construct a copy and tweak one field at a time.
var validGovernanceExpectationDoc = types.GovernanceExpectationDocument{
	APIVersion: types.APIVersionV1,
	Kind:       types.KindGovernanceExpectation,
	Metadata: types.DocumentMetadata{
		ID:   "expect-credit-assessment-risk",
		Name: "Credit assessment risk expectation",
	},
	Spec: types.GovernanceExpectationSpec{
		Description:       "Requires credit assessment governance for risk-shaped decisions",
		ScopeKind:         "process",
		ScopeID:           "proc-credit-assessment",
		RequiredSurfaceID: "surf-v2-credit-assess",
		ConditionType:     "risk_condition",
		ConditionPayload: map[string]any{
			"amount_greater_than": 5000,
			"currency":            "GBP",
		},
		BusinessOwner:  "consumer-lending-team",
		TechnicalOwner: "midas",
		Lifecycle: types.GovernanceExpectationLifecycle{
			Status:         "active",
			EffectiveFrom:  "2026-01-01T00:00:00Z",
			EffectiveUntil: "2027-01-01T00:00:00Z",
			Version:        1,
		},
	},
}

func wrapGovernanceExpectation(doc types.GovernanceExpectationDocument) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: doc.GetKind(),
		ID:   doc.GetID(),
		Doc:  doc,
	}
}

// findFieldErr returns the first ValidationError whose Field matches
// field. Used by negative tests to assert the validator names the right
// field path.
func findFieldErr(errs []types.ValidationError, field string) *types.ValidationError {
	for i := range errs {
		if errs[i].Field == field {
			return &errs[i]
		}
	}
	return nil
}

func TestValidateGovernanceExpectation_ValidFull_NoErrors(t *testing.T) {
	errs := ValidateDocument(wrapGovernanceExpectation(validGovernanceExpectationDoc))
	if len(errs) != 0 {
		t.Fatalf("valid full document: want 0 errors, got %d: %+v", len(errs), errs)
	}
}

func TestValidateGovernanceExpectation_ValidMinimal_NoErrors(t *testing.T) {
	doc := types.GovernanceExpectationDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindGovernanceExpectation,
		Metadata: types.DocumentMetadata{
			ID:   "ge-min",
			Name: "Minimal expectation",
		},
		Spec: types.GovernanceExpectationSpec{
			ScopeKind:         "process",
			ScopeID:           "proc-x",
			RequiredSurfaceID: "surf-x",
			ConditionType:     "risk_condition",
			BusinessOwner:     "biz",
			TechnicalOwner:    "tech",
		},
	}
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if len(errs) != 0 {
		t.Fatalf("valid minimal document: want 0 errors, got %d: %+v", len(errs), errs)
	}
}

func TestValidateGovernanceExpectation_RequiredFields(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*types.GovernanceExpectationDocument)
		wantField string
	}{
		{
			name:      "missing_metadata_name",
			mutate:    func(d *types.GovernanceExpectationDocument) { d.Metadata.Name = "" },
			wantField: "metadata.name",
		},
		{
			name:      "missing_scope_kind",
			mutate:    func(d *types.GovernanceExpectationDocument) { d.Spec.ScopeKind = "" },
			wantField: "spec.scope_kind",
		},
		{
			name:      "missing_scope_id",
			mutate:    func(d *types.GovernanceExpectationDocument) { d.Spec.ScopeID = "" },
			wantField: "spec.scope_id",
		},
		{
			name:      "missing_required_surface_id",
			mutate:    func(d *types.GovernanceExpectationDocument) { d.Spec.RequiredSurfaceID = "" },
			wantField: "spec.required_surface_id",
		},
		{
			name:      "missing_condition_type",
			mutate:    func(d *types.GovernanceExpectationDocument) { d.Spec.ConditionType = "" },
			wantField: "spec.condition_type",
		},
		{
			name:      "missing_business_owner",
			mutate:    func(d *types.GovernanceExpectationDocument) { d.Spec.BusinessOwner = "" },
			wantField: "spec.business_owner",
		},
		{
			name:      "missing_technical_owner",
			mutate:    func(d *types.GovernanceExpectationDocument) { d.Spec.TechnicalOwner = "" },
			wantField: "spec.technical_owner",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := validGovernanceExpectationDoc
			tc.mutate(&doc)
			errs := ValidateDocument(wrapGovernanceExpectation(doc))
			if findFieldErr(errs, tc.wantField) == nil {
				t.Fatalf("want a ValidationError on %q; got %+v", tc.wantField, errs)
			}
		})
	}
}

func TestValidateGovernanceExpectation_ScopeKind_BusinessService_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.ScopeKind = "business_service"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	got := findFieldErr(errs, "spec.scope_kind")
	if got == nil {
		t.Fatalf("want a ValidationError on spec.scope_kind; got %+v", errs)
	}
	if !strings.Contains(got.Message, "not supported by control-plane apply yet") {
		t.Errorf("error message must explain the apply-side scoping limitation; got %q", got.Message)
	}
	if !strings.Contains(got.Message, `"process"`) {
		t.Errorf("error message must name the supported scope (\"process\"); got %q", got.Message)
	}
}

func TestValidateGovernanceExpectation_ScopeKind_Capability_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.ScopeKind = "capability"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	got := findFieldErr(errs, "spec.scope_kind")
	if got == nil {
		t.Fatalf("want a ValidationError on spec.scope_kind; got %+v", errs)
	}
	if !strings.Contains(got.Message, "not supported by control-plane apply yet") {
		t.Errorf("error message must explain the apply-side scoping limitation; got %q", got.Message)
	}
}

func TestValidateGovernanceExpectation_ScopeKind_InvalidValue_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.ScopeKind = "not-a-valid-kind"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.scope_kind") == nil {
		t.Fatalf("want a ValidationError on spec.scope_kind; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_ConditionType_InvalidValue_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.ConditionType = "not-a-real-condition"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.condition_type") == nil {
		t.Fatalf("want a ValidationError on spec.condition_type; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_LifecycleStatus_InvalidValue_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.Lifecycle.Status = "not-a-real-status"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.lifecycle.status") == nil {
		t.Fatalf("want a ValidationError on spec.lifecycle.status; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_LifecycleStatus_Empty_Accepted(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.Lifecycle.Status = ""
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.lifecycle.status") != nil {
		t.Fatalf("empty status must be accepted; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_EffectiveFrom_InvalidFormat_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.Lifecycle.EffectiveFrom = "not-an-rfc3339-date"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.lifecycle.effective_from") == nil {
		t.Fatalf("want a ValidationError on spec.lifecycle.effective_from; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_EffectiveUntil_InvalidFormat_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.Lifecycle.EffectiveUntil = "not-an-rfc3339-date"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.lifecycle.effective_until") == nil {
		t.Fatalf("want a ValidationError on spec.lifecycle.effective_until; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_EffectiveUntil_BeforeFrom_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.Lifecycle.EffectiveFrom = "2026-06-01T00:00:00Z"
	doc.Spec.Lifecycle.EffectiveUntil = "2026-01-01T00:00:00Z"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	got := findFieldErr(errs, "spec.lifecycle.effective_until")
	if got == nil {
		t.Fatalf("want a ValidationError on spec.lifecycle.effective_until; got %+v", errs)
	}
	if !strings.Contains(got.Message, "must be after") {
		t.Errorf("message must explain the ordering violation; got %q", got.Message)
	}
}

func TestValidateGovernanceExpectation_EffectiveUntil_EqualToFrom_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.Lifecycle.EffectiveFrom = "2026-01-01T00:00:00Z"
	doc.Spec.Lifecycle.EffectiveUntil = "2026-01-01T00:00:00Z"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.lifecycle.effective_until") == nil {
		t.Fatalf("equal effective_until/effective_from must be rejected; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_LifecycleVersion_Negative_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.Lifecycle.Version = -3
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.lifecycle.version") == nil {
		t.Fatalf("want a ValidationError on spec.lifecycle.version; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_IDFormat_BadScopeID_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.ScopeID = "BAD ID with spaces"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.scope_id") == nil {
		t.Fatalf("want a ValidationError on spec.scope_id; got %+v", errs)
	}
}

func TestValidateGovernanceExpectation_IDFormat_BadRequiredSurfaceID_Rejected(t *testing.T) {
	doc := validGovernanceExpectationDoc
	doc.Spec.RequiredSurfaceID = "BAD ID"
	errs := ValidateDocument(wrapGovernanceExpectation(doc))
	if findFieldErr(errs, "spec.required_surface_id") == nil {
		t.Fatalf("want a ValidationError on spec.required_surface_id; got %+v", errs)
	}
}
