package validate_test

import (
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
)

// ---------------------------------------------------------------------------
// AISystem (Epic 1, PR 2)
// ---------------------------------------------------------------------------

func makeAISystemDoc(id string) types.AISystemDocument {
	return types.AISystemDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAISystem,
		Metadata:   types.DocumentMetadata{ID: id, Name: id + " name"},
		Spec: types.AISystemSpec{
			Status: "active",
			Origin: "manual",
		},
	}
}

func wrapAISystemDoc(d types.AISystemDocument) parser.ParsedDocument {
	return parser.ParsedDocument{Kind: d.Kind, ID: d.Metadata.ID, Doc: d}
}

func TestValidate_AISystem_HappyPath(t *testing.T) {
	errs := validate.ValidateDocument(wrapAISystemDoc(makeAISystemDoc("ai-1")))
	if len(errs) != 0 {
		t.Errorf("want no errors, got %+v", errs)
	}
}

func TestValidate_AISystem_RejectsMissingName(t *testing.T) {
	d := makeAISystemDoc("ai-1")
	d.Metadata.Name = ""
	errs := validate.ValidateDocument(wrapAISystemDoc(d))
	if !hasFieldErr(errs, "metadata.name") {
		t.Errorf("expected metadata.name error; got %+v", errs)
	}
}

func TestValidate_AISystem_RejectsInvalidStatus(t *testing.T) {
	d := makeAISystemDoc("ai-1")
	d.Spec.Status = "frozen"
	errs := validate.ValidateDocument(wrapAISystemDoc(d))
	if !hasFieldErr(errs, "spec.status") {
		t.Errorf("expected spec.status enum error; got %+v", errs)
	}
}

func TestValidate_AISystem_AcceptsEmptyStatusForDefault(t *testing.T) {
	d := makeAISystemDoc("ai-1")
	d.Spec.Status = "" // empty → mapper defaults to "active"
	errs := validate.ValidateDocument(wrapAISystemDoc(d))
	if len(errs) != 0 {
		t.Errorf("empty status should be acceptable (mapper supplies default); got %+v", errs)
	}
}

func TestValidate_AISystem_RejectsInvalidOrigin(t *testing.T) {
	d := makeAISystemDoc("ai-1")
	d.Spec.Origin = "imported"
	errs := validate.ValidateDocument(wrapAISystemDoc(d))
	if !hasFieldErr(errs, "spec.origin") {
		t.Errorf("expected spec.origin enum error; got %+v", errs)
	}
}

func TestValidate_AISystem_RejectsSelfReplace(t *testing.T) {
	d := makeAISystemDoc("ai-1")
	d.Spec.Replaces = "ai-1"
	errs := validate.ValidateDocument(wrapAISystemDoc(d))
	if !errsContain(errs, "cannot replace itself") {
		t.Errorf("expected self-replace error; got %+v", errs)
	}
}

func TestValidate_AISystem_RejectsBadReplacesIDFormat(t *testing.T) {
	d := makeAISystemDoc("ai-1")
	d.Spec.Replaces = "AI With Spaces"
	errs := validate.ValidateDocument(wrapAISystemDoc(d))
	if !hasFieldErr(errs, "spec.replaces") {
		t.Errorf("expected spec.replaces format error; got %+v", errs)
	}
}

// ---------------------------------------------------------------------------
// AISystemVersion
// ---------------------------------------------------------------------------

func makeAIVersionDoc(id string) types.AISystemVersionDocument {
	return types.AISystemVersionDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAISystemVersion,
		Metadata:   types.DocumentMetadata{ID: id},
		Spec: types.AISystemVersionSpec{
			AISystemID:    "ai-1",
			Version:       1,
			Status:        "active",
			EffectiveFrom: "2026-04-15T00:00:00Z",
		},
	}
}

func wrapAIVersionDoc(d types.AISystemVersionDocument) parser.ParsedDocument {
	return parser.ParsedDocument{Kind: d.Kind, ID: d.Metadata.ID, Doc: d}
}

func TestValidate_AISystemVersion_HappyPath(t *testing.T) {
	errs := validate.ValidateDocument(wrapAIVersionDoc(makeAIVersionDoc("aiv-1")))
	if len(errs) != 0 {
		t.Errorf("want no errors, got %+v", errs)
	}
}

func TestValidate_AISystemVersion_RejectsMissingAISystemID(t *testing.T) {
	d := makeAIVersionDoc("aiv-1")
	d.Spec.AISystemID = ""
	errs := validate.ValidateDocument(wrapAIVersionDoc(d))
	if !hasFieldErr(errs, "spec.ai_system_id") {
		t.Errorf("expected spec.ai_system_id error; got %+v", errs)
	}
}

func TestValidate_AISystemVersion_RejectsZeroVersion(t *testing.T) {
	d := makeAIVersionDoc("aiv-1")
	d.Spec.Version = 0
	errs := validate.ValidateDocument(wrapAIVersionDoc(d))
	if !hasFieldErr(errs, "spec.version") {
		t.Errorf("expected spec.version error; got %+v", errs)
	}
}

func TestValidate_AISystemVersion_RejectsInvalidStatus(t *testing.T) {
	d := makeAIVersionDoc("aiv-1")
	d.Spec.Status = "frozen"
	errs := validate.ValidateDocument(wrapAIVersionDoc(d))
	if !hasFieldErr(errs, "spec.status") {
		t.Errorf("expected spec.status enum error; got %+v", errs)
	}
}

func TestValidate_AISystemVersion_AcceptsReviewStatus(t *testing.T) {
	d := makeAIVersionDoc("aiv-1")
	d.Spec.Status = "review"
	errs := validate.ValidateDocument(wrapAIVersionDoc(d))
	if len(errs) != 0 {
		t.Errorf("review status is honoured by apply; want no errors, got %+v", errs)
	}
}

func TestValidate_AISystemVersion_RejectsMissingEffectiveFrom(t *testing.T) {
	d := makeAIVersionDoc("aiv-1")
	d.Spec.EffectiveFrom = ""
	errs := validate.ValidateDocument(wrapAIVersionDoc(d))
	if !hasFieldErr(errs, "spec.effective_from") {
		t.Errorf("expected spec.effective_from required error; got %+v", errs)
	}
}

func TestValidate_AISystemVersion_RejectsInvalidEffectiveRange(t *testing.T) {
	d := makeAIVersionDoc("aiv-1")
	d.Spec.EffectiveFrom = "2026-04-15T00:00:00Z"
	d.Spec.EffectiveUntil = "2026-04-15T00:00:00Z" // same — must be strictly after
	errs := validate.ValidateDocument(wrapAIVersionDoc(d))
	if !hasFieldErr(errs, "spec.effective_until") {
		t.Errorf("expected spec.effective_until > effective_from error; got %+v", errs)
	}
}

func TestValidate_AISystemVersion_RejectsBadTimestamp(t *testing.T) {
	d := makeAIVersionDoc("aiv-1")
	d.Spec.EffectiveFrom = "yesterday"
	errs := validate.ValidateDocument(wrapAIVersionDoc(d))
	if !hasFieldErr(errs, "spec.effective_from") {
		t.Errorf("expected RFC3339 parse error; got %+v", errs)
	}
}

// ---------------------------------------------------------------------------
// AISystemBinding (per-document — rule 1: at-least-one-context)
// ---------------------------------------------------------------------------

func makeAIBindingDoc(id string) types.AISystemBindingDocument {
	return types.AISystemBindingDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAISystemBinding,
		Metadata:   types.DocumentMetadata{ID: id},
		Spec: types.AISystemBindingSpec{
			AISystemID:        "ai-1",
			BusinessServiceID: "bs-x",
		},
	}
}

func wrapAIBindingDoc(d types.AISystemBindingDocument) parser.ParsedDocument {
	return parser.ParsedDocument{Kind: d.Kind, ID: d.Metadata.ID, Doc: d}
}

func TestValidate_AISystemBinding_HappyPath(t *testing.T) {
	errs := validate.ValidateDocument(wrapAIBindingDoc(makeAIBindingDoc("bind-1")))
	if len(errs) != 0 {
		t.Errorf("want no errors, got %+v", errs)
	}
}

func TestValidate_AISystemBinding_RejectsMissingAISystemID(t *testing.T) {
	d := makeAIBindingDoc("bind-1")
	d.Spec.AISystemID = ""
	errs := validate.ValidateDocument(wrapAIBindingDoc(d))
	if !hasFieldErr(errs, "spec.ai_system_id") {
		t.Errorf("expected spec.ai_system_id error; got %+v", errs)
	}
}

func TestValidate_AISystemBinding_RejectsNoContextReference(t *testing.T) {
	d := makeAIBindingDoc("bind-1")
	d.Spec.BusinessServiceID = ""
	errs := validate.ValidateDocument(wrapAIBindingDoc(d))
	if !errsContain(errs, "at least one of") {
		t.Errorf("expected at-least-one-context error; got %+v", errs)
	}
}

func TestValidate_AISystemBinding_AcceptsAnyContextSubset(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*types.AISystemBindingDocument)
	}{
		{"only-bs", func(d *types.AISystemBindingDocument) { d.Spec.BusinessServiceID = "bs-x" }},
		{"only-cap", func(d *types.AISystemBindingDocument) { d.Spec.CapabilityID = "cap-x" }},
		{"only-proc", func(d *types.AISystemBindingDocument) { d.Spec.ProcessID = "proc-x" }},
		{"only-surf", func(d *types.AISystemBindingDocument) { d.Spec.SurfaceID = "surf-x" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := makeAIBindingDoc("bind-1")
			d.Spec.BusinessServiceID = ""
			tc.mut(&d)
			errs := validate.ValidateDocument(wrapAIBindingDoc(d))
			if len(errs) != 0 {
				t.Errorf("want no errors with %s, got %+v", tc.name, errs)
			}
		})
	}
}

func TestValidate_AISystemBinding_RejectsZeroPinnedVersion(t *testing.T) {
	d := makeAIBindingDoc("bind-1")
	zero := 0
	d.Spec.AISystemVersion = &zero
	errs := validate.ValidateDocument(wrapAIBindingDoc(d))
	if !hasFieldErr(errs, "spec.ai_system_version") {
		t.Errorf("expected ai_system_version >= 1 error; got %+v", errs)
	}
}

func TestValidate_AISystemBinding_RejectsBadContextIDFormat(t *testing.T) {
	d := makeAIBindingDoc("bind-1")
	d.Spec.SurfaceID = "Surface With Spaces"
	errs := validate.ValidateDocument(wrapAIBindingDoc(d))
	if !hasFieldErr(errs, "spec.surface_id") {
		t.Errorf("expected spec.surface_id format error; got %+v", errs)
	}
}

// ---------------------------------------------------------------------------
// Bundle-level uniqueness
// ---------------------------------------------------------------------------

func TestValidateBundle_AISystemVersion_RejectsDuplicateTuple(t *testing.T) {
	d1 := makeAIVersionDoc("aiv-1")
	d2 := makeAIVersionDoc("aiv-2") // different metadata.id
	d2.Spec.AISystemID = d1.Spec.AISystemID
	d2.Spec.Version = d1.Spec.Version

	errs := validate.ValidateBundle([]parser.ParsedDocument{
		wrapAIVersionDoc(d1),
		wrapAIVersionDoc(d2),
	})
	if !errsContain(errs, "duplicate ai system version tuple") {
		t.Errorf("expected duplicate-tuple bundle error; got %+v", errs)
	}
}

func TestValidateBundle_AISystemVersion_DifferentVersionsNotDuplicate(t *testing.T) {
	d1 := makeAIVersionDoc("aiv-1")
	d2 := makeAIVersionDoc("aiv-2")
	d2.Spec.Version = 2

	errs := validate.ValidateBundle([]parser.ParsedDocument{
		wrapAIVersionDoc(d1),
		wrapAIVersionDoc(d2),
	})
	if errsContain(errs, "duplicate ai system version tuple") {
		t.Errorf("(ai_system_id, v1) and (ai_system_id, v2) must not collide; got %+v", errs)
	}
}
