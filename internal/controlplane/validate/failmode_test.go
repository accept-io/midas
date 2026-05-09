package validate

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// validFailModePolicyDoc returns a structurally-valid FailModePolicy
// document satisfying every closed-only validator constraint. Tests mutate
// one field at a time and assert the resulting validator output.
func validFailModePolicyDoc() types.FailModePolicyDocument {
	return types.FailModePolicyDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindFailModePolicy,
		Metadata:   types.DocumentMetadata{ID: "default-fail-mode", Name: "Default Fail-Mode Policy"},
		Spec: types.FailModePolicySpec{
			Name:           "Default Fail-Mode Policy",
			Description:    "Closed-only baseline.",
			BusinessOwner:  "owner@example.com",
			TechnicalOwner: "platform-team",
			Rules: []types.FailModePolicyRuleSpec{
				{CorrectnessClass: "governance_integrity", PermittedMode: "closed"},
				{CorrectnessClass: "persistence", PermittedMode: "closed"},
				{CorrectnessClass: "input", PermittedMode: "not_applicable"},
				{CorrectnessClass: "resource", PermittedMode: "closed", Reason: "policy evaluator unreachable"},
				{CorrectnessClass: "consistency", PermittedMode: "closed"},
			},
			Origin: "manual",
		},
		Lifecycle: types.FailModePolicyLifecycle{
			EffectiveFrom: "2026-01-01T00:00:00Z",
		},
	}
}

func wrapFMP(t *testing.T, doc types.FailModePolicyDocument) parser.ParsedDocument {
	t.Helper()
	return parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}
}

func validateFMP(t *testing.T, doc types.FailModePolicyDocument) []types.ValidationError {
	t.Helper()
	return ValidateDocument(wrapFMP(t, doc))
}

func TestValidateFailModePolicy_Valid(t *testing.T) {
	doc := validFailModePolicyDoc()
	errs := validateFMP(t, doc)
	if len(errs) != 0 {
		t.Fatalf("valid policy should produce no errors; got %d: %+v", len(errs), errs)
	}
}

// containsFieldErr reports whether errs contains a ValidationError whose
// Field equals field and whose Message contains messageSubstring.
func containsFieldErr(errs []types.ValidationError, field, messageSubstring string) bool {
	for _, e := range errs {
		if e.Field == field && strings.Contains(e.Message, messageSubstring) {
			return true
		}
	}
	return false
}

func containsFieldErrJustField(errs []types.ValidationError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

func TestValidateFailModePolicy_RequiresMetadataID(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Metadata.ID = ""
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "metadata.id") {
		t.Errorf("expected metadata.id error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsNonKebabCaseID(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Metadata.ID = "Default Policy" // spaces and uppercase
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "metadata.id") {
		t.Errorf("expected metadata.id format error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RequiresSpecName(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.Name = ""
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "spec.name", "is required") {
		t.Errorf("expected spec.name required error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RequiresBusinessOwner(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.BusinessOwner = ""
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "spec.business_owner", "is required") {
		t.Errorf("expected business_owner required error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RequiresTechnicalOwner(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.TechnicalOwner = ""
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "spec.technical_owner", "is required") {
		t.Errorf("expected technical_owner required error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsInvalidOrigin(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.Origin = "auto"
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "spec.origin", "invalid value") {
		t.Errorf("expected spec.origin enum error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsReplacesEqualsID(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.Replaces = doc.Metadata.ID
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "spec.replaces", "no self-reference") {
		t.Errorf("expected spec.replaces no-self-reference error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsMissingRules(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.Rules = nil
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "spec.rules") {
		t.Errorf("expected spec.rules error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsNonExhaustiveRules(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.Rules = doc.Spec.Rules[:4] // drop one
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "spec.rules") {
		t.Errorf("expected spec.rules exhaustiveness error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsDuplicateCorrectnessClass(t *testing.T) {
	doc := validFailModePolicyDoc()
	// Replace input with a second resource entry — count stays 5 but input
	// disappears and resource is duplicated.
	doc.Spec.Rules[2] = types.FailModePolicyRuleSpec{CorrectnessClass: "resource", PermittedMode: "closed"}
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "spec.rules") {
		t.Errorf("expected spec.rules duplicate error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsUnknownCorrectnessClass(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.Rules[0] = types.FailModePolicyRuleSpec{CorrectnessClass: "garbage", PermittedMode: "closed"}
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "spec.rules") {
		t.Errorf("expected spec.rules unknown-class error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsInputWithClosed(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "input" {
			doc.Spec.Rules[i].PermittedMode = "closed"
		}
	}
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "spec.rules") {
		t.Errorf("expected spec.rules input-must-be-not_applicable error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsInputWithSoft(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "input" {
			doc.Spec.Rules[i].PermittedMode = "soft"
		}
	}
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "spec.rules") {
		t.Errorf("expected spec.rules soft-rejected error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsInputWithOpen(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "input" {
			doc.Spec.Rules[i].PermittedMode = "open"
		}
	}
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "spec.rules") {
		t.Errorf("expected spec.rules open-rejected error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsNonInputWithNotApplicable(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "consistency" {
			doc.Spec.Rules[i].PermittedMode = "not_applicable"
		}
	}
	errs := validateFMP(t, doc)
	if !containsFieldErrJustField(errs, "spec.rules") {
		t.Errorf("expected spec.rules non-input-must-be-closed error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsNonInputWithSoft(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "resource" {
			doc.Spec.Rules[i].PermittedMode = "soft"
		}
	}
	errs := validateFMP(t, doc)
	found := false
	for _, e := range errs {
		if e.Field == "spec.rules" && strings.Contains(e.Message, "not admitted") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected spec.rules 'not admitted' (soft-rejected) error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsNonInputWithOpen(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "resource" {
			doc.Spec.Rules[i].PermittedMode = "open"
		}
	}
	errs := validateFMP(t, doc)
	found := false
	for _, e := range errs {
		if e.Field == "spec.rules" && strings.Contains(e.Message, "not admitted") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected spec.rules 'not admitted' (open-rejected) error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_AcceptsEmptyReasonOnClosed(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		doc.Spec.Rules[i].Reason = ""
	}
	errs := validateFMP(t, doc)
	if len(errs) != 0 {
		t.Errorf("empty reason on closed-only rules should validate; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsInvalidLifecycleStatus(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Lifecycle.Status = "garbage"
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "lifecycle.status", "invalid value") {
		t.Errorf("expected lifecycle.status enum error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_AcceptsExplicitReviewStatus(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Lifecycle.Status = "review"
	errs := validateFMP(t, doc)
	if len(errs) != 0 {
		t.Errorf("explicit review status should validate; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsMalformedEffectiveFrom(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Lifecycle.EffectiveFrom = "not-a-date"
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "lifecycle.effective_from", "RFC3339") {
		t.Errorf("expected RFC3339 error on lifecycle.effective_from; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsEffectiveUntilEqualsEffectiveFrom(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Lifecycle.EffectiveFrom = "2026-01-01T00:00:00Z"
	doc.Lifecycle.EffectiveUntil = "2026-01-01T00:00:00Z"
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "lifecycle.effective_until", "must be after") {
		t.Errorf("expected lifecycle.effective_until window error; got %+v", errs)
	}
}

func TestValidateFailModePolicy_RejectsNegativeLifecycleVersion(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Lifecycle.Version = -1
	errs := validateFMP(t, doc)
	if !containsFieldErr(errs, "lifecycle.version", "≥ 0") {
		t.Errorf("expected lifecycle.version negative-rejected error; got %+v", errs)
	}
}
