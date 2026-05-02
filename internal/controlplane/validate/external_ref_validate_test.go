package validate_test

// Validator coverage for the optional ExternalRef field that five
// document specs gained in Epic 1, PR 3.
//
// Test posture: each per-entity entry validator delegates to the
// shared validateExternalRefSpec helper. Tests therefore cover the
// shared rules once per code path (RejectsInconsistent,
// RejectsBadTimestamp, AcceptsNil, AcceptsValid) and confirm the
// shared helper is wired into all five entities.

import (
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
)

// validExtRefSpec returns a fully-populated, well-formed ExternalRefSpec.
func validExtRefSpec() *types.ExternalRefSpec {
	return &types.ExternalRefSpec{
		SourceSystem:  "github",
		SourceID:      "accept-io/midas",
		SourceURL:     "https://github.com/accept-io/midas",
		SourceVersion: "v1.2.0",
		LastSyncedAt:  "2026-04-30T09:00:00Z",
	}
}

// inconsistentExtRefSpec violates the consistency rule (system set, id empty).
func inconsistentExtRefSpec() *types.ExternalRefSpec {
	return &types.ExternalRefSpec{SourceSystem: "github" /* SourceID intentionally empty */}
}

// onlySystemAndID is the minimum-valid populated ref (the two
// required-together fields, nothing else).
func onlySystemAndID() *types.ExternalRefSpec {
	return &types.ExternalRefSpec{SourceSystem: "github", SourceID: "x"}
}

// extRefBadTimestamp has consistent system+id but a non-RFC3339 timestamp.
func extRefBadTimestamp() *types.ExternalRefSpec {
	return &types.ExternalRefSpec{SourceSystem: "github", SourceID: "x", LastSyncedAt: "yesterday"}
}

// ---------------------------------------------------------------------------
// Per-entity wiring — one valid + one invalid per entity, to confirm the
// shared helper is invoked from every per-entity validator.
// ---------------------------------------------------------------------------

func TestValidate_ExternalRef_BusinessService(t *testing.T) {
	doc := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
		Metadata: types.DocumentMetadata{ID: "bs-1", Name: "BS"},
		Spec: types.BusinessServiceSpec{
			ServiceType: "internal", Status: "active",
			ExternalRef: validExtRefSpec(),
		},
	}
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); len(errs) != 0 {
		t.Errorf("valid ext_ref: want no errors, got %+v", errs)
	}
	doc.Spec.ExternalRef = inconsistentExtRefSpec()
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); !hasFieldErr(errs, "spec.external_ref") {
		t.Errorf("inconsistent ext_ref: want spec.external_ref error, got %+v", errs)
	}
}

func TestValidate_ExternalRef_BusinessServiceRelationship(t *testing.T) {
	doc := types.BusinessServiceRelationshipDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindBusinessServiceRelationship,
		Metadata: types.DocumentMetadata{ID: "rel-1"},
		Spec: types.BusinessServiceRelationshipSpec{
			SourceBusinessServiceID: "bs-a", TargetBusinessServiceID: "bs-b", RelationshipType: "depends_on",
			ExternalRef: validExtRefSpec(),
		},
	}
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); len(errs) != 0 {
		t.Errorf("valid: %+v", errs)
	}
	doc.Spec.ExternalRef = inconsistentExtRefSpec()
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); !hasFieldErr(errs, "spec.external_ref") {
		t.Errorf("inconsistent: want spec.external_ref error, got %+v", errs)
	}
}

func TestValidate_ExternalRef_AISystem(t *testing.T) {
	doc := types.AISystemDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindAISystem,
		Metadata: types.DocumentMetadata{ID: "ai-1", Name: "AI"},
		Spec: types.AISystemSpec{
			Status: "active", Origin: "manual",
			ExternalRef: validExtRefSpec(),
		},
	}
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); len(errs) != 0 {
		t.Errorf("valid: %+v", errs)
	}
	doc.Spec.ExternalRef = inconsistentExtRefSpec()
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); !hasFieldErr(errs, "spec.external_ref") {
		t.Errorf("inconsistent: %+v", errs)
	}
}

func TestValidate_ExternalRef_AISystemVersion(t *testing.T) {
	doc := types.AISystemVersionDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindAISystemVersion,
		Metadata: types.DocumentMetadata{ID: "aiv-1"},
		Spec: types.AISystemVersionSpec{
			AISystemID: "ai-1", Version: 1, Status: "active",
			EffectiveFrom: "2026-04-15T00:00:00Z",
			ExternalRef:   validExtRefSpec(),
		},
	}
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); len(errs) != 0 {
		t.Errorf("valid: %+v", errs)
	}
	doc.Spec.ExternalRef = inconsistentExtRefSpec()
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); !hasFieldErr(errs, "spec.external_ref") {
		t.Errorf("inconsistent: %+v", errs)
	}
}

func TestValidate_ExternalRef_AISystemBinding(t *testing.T) {
	doc := types.AISystemBindingDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindAISystemBinding,
		Metadata: types.DocumentMetadata{ID: "bind-1"},
		Spec: types.AISystemBindingSpec{
			AISystemID: "ai-1", BusinessServiceID: "bs-x",
			ExternalRef: validExtRefSpec(),
		},
	}
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); len(errs) != 0 {
		t.Errorf("valid: %+v", errs)
	}
	doc.Spec.ExternalRef = inconsistentExtRefSpec()
	if errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}); !hasFieldErr(errs, "spec.external_ref") {
		t.Errorf("inconsistent: %+v", errs)
	}
}

// ---------------------------------------------------------------------------
// Shared rules — exercised once via BusinessService since the helper is
// the same code path for all five.
// ---------------------------------------------------------------------------

func TestValidate_ExternalRef_AcceptsNilOnAllKinds(t *testing.T) {
	cases := []parser.ParsedDocument{
		{Kind: types.KindBusinessService, ID: "bs-nil", Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
			Metadata: types.DocumentMetadata{ID: "bs-nil", Name: "BS"},
			Spec:     types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
		}},
		{Kind: types.KindBusinessServiceRelationship, ID: "rel-nil", Doc: types.BusinessServiceRelationshipDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindBusinessServiceRelationship,
			Metadata: types.DocumentMetadata{ID: "rel-nil"},
			Spec: types.BusinessServiceRelationshipSpec{
				SourceBusinessServiceID: "bs-a", TargetBusinessServiceID: "bs-b", RelationshipType: "depends_on",
			},
		}},
		{Kind: types.KindAISystem, ID: "ai-nil", Doc: types.AISystemDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystem,
			Metadata: types.DocumentMetadata{ID: "ai-nil", Name: "AI"},
			Spec:     types.AISystemSpec{Status: "active", Origin: "manual"},
		}},
		{Kind: types.KindAISystemVersion, ID: "aiv-nil", Doc: types.AISystemVersionDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystemVersion,
			Metadata: types.DocumentMetadata{ID: "aiv-nil"},
			Spec: types.AISystemVersionSpec{
				AISystemID: "ai-1", Version: 1, Status: "active",
				EffectiveFrom: "2026-04-15T00:00:00Z",
			},
		}},
		{Kind: types.KindAISystemBinding, ID: "bind-nil", Doc: types.AISystemBindingDocument{
			APIVersion: types.APIVersionV1, Kind: types.KindAISystemBinding,
			Metadata: types.DocumentMetadata{ID: "bind-nil"},
			Spec: types.AISystemBindingSpec{
				AISystemID: "ai-1", BusinessServiceID: "bs-x",
			},
		}},
	}
	for _, c := range cases {
		t.Run(c.Kind, func(t *testing.T) {
			if errs := validate.ValidateDocument(c); len(errs) != 0 {
				t.Errorf("nil ExternalRef must be accepted; got %+v", errs)
			}
		})
	}
}

func TestValidate_ExternalRef_AcceptsMinimumValid(t *testing.T) {
	doc := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
		Metadata: types.DocumentMetadata{ID: "bs-min", Name: "BS"},
		Spec: types.BusinessServiceSpec{
			ServiceType: "internal", Status: "active",
			ExternalRef: onlySystemAndID(),
		},
	}
	errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc})
	if len(errs) != 0 {
		t.Errorf("system+id only must be accepted; got %+v", errs)
	}
}

func TestValidate_ExternalRef_RejectsSystemWithoutID(t *testing.T) {
	doc := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
		Metadata: types.DocumentMetadata{ID: "bs-half", Name: "BS"},
		Spec: types.BusinessServiceSpec{
			ServiceType: "internal", Status: "active",
			ExternalRef: &types.ExternalRefSpec{SourceSystem: "github"}, // SourceID missing
		},
	}
	errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc})
	if !hasFieldErr(errs, "spec.external_ref") {
		t.Errorf("want spec.external_ref consistency error; got %+v", errs)
	}
}

func TestValidate_ExternalRef_RejectsIDWithoutSystem(t *testing.T) {
	doc := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
		Metadata: types.DocumentMetadata{ID: "bs-half2", Name: "BS"},
		Spec: types.BusinessServiceSpec{
			ServiceType: "internal", Status: "active",
			ExternalRef: &types.ExternalRefSpec{SourceID: "x"}, // SourceSystem missing
		},
	}
	errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc})
	if !hasFieldErr(errs, "spec.external_ref") {
		t.Errorf("want spec.external_ref consistency error; got %+v", errs)
	}
}

func TestValidate_ExternalRef_RejectsBadLastSyncedAtTimestamp(t *testing.T) {
	doc := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1, Kind: types.KindBusinessService,
		Metadata: types.DocumentMetadata{ID: "bs-bad-ts", Name: "BS"},
		Spec: types.BusinessServiceSpec{
			ServiceType: "internal", Status: "active",
			ExternalRef: extRefBadTimestamp(),
		},
	}
	errs := validate.ValidateDocument(parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc})
	if !hasFieldErr(errs, "spec.external_ref.last_synced_at") {
		t.Errorf("want spec.external_ref.last_synced_at error; got %+v", errs)
	}
}
