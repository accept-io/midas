package validate_test

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
)

// makeBSRDoc constructs a parsed BSR document for tests. Callers can override
// any field by post-edit before calling ValidateDocument.
func makeBSRDoc(id, source, target, relType string) parser.ParsedDocument {
	doc := types.BusinessServiceRelationshipDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindBusinessServiceRelationship,
		Metadata:   types.DocumentMetadata{ID: id},
		Spec: types.BusinessServiceRelationshipSpec{
			SourceBusinessServiceID: source,
			TargetBusinessServiceID: target,
			RelationshipType:        relType,
			Description:             "test relationship",
		},
	}
	return parser.ParsedDocument{
		Kind: doc.Kind,
		ID:   doc.Metadata.ID,
		Doc:  doc,
	}
}

// makeBSDoc is a small helper for cross-bundle tests that need to seed a BS
// document inside the bundle so the BSR's source/target reference resolves.
func makeBSDoc(id string) parser.ParsedDocument {
	doc := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindBusinessService,
		Metadata:   types.DocumentMetadata{ID: id, Name: id},
		Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
	}
	return parser.ParsedDocument{
		Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc,
	}
}

func TestValidate_BusinessServiceRelationship_HappyPath(t *testing.T) {
	errs := validate.ValidateDocument(makeBSRDoc("rel-1", "bs-a", "bs-b", "depends_on"))
	if len(errs) != 0 {
		t.Errorf("want no errors, got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_RejectsMissingSource(t *testing.T) {
	errs := validate.ValidateDocument(makeBSRDoc("rel-1", "", "bs-b", "depends_on"))
	if !hasFieldErr(errs, "spec.source_business_service_id") {
		t.Errorf("expected spec.source_business_service_id error; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_RejectsMissingTarget(t *testing.T) {
	errs := validate.ValidateDocument(makeBSRDoc("rel-1", "bs-a", "", "depends_on"))
	if !hasFieldErr(errs, "spec.target_business_service_id") {
		t.Errorf("expected spec.target_business_service_id error; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_RejectsMissingRelationshipType(t *testing.T) {
	errs := validate.ValidateDocument(makeBSRDoc("rel-1", "bs-a", "bs-b", ""))
	if !hasFieldErr(errs, "spec.relationship_type") {
		t.Errorf("expected spec.relationship_type error; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_RejectsInvalidRelationshipType(t *testing.T) {
	errs := validate.ValidateDocument(makeBSRDoc("rel-1", "bs-a", "bs-b", "invented_type"))
	if !hasFieldErr(errs, "spec.relationship_type") {
		t.Errorf("expected spec.relationship_type enum error; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_RejectsSelfReference(t *testing.T) {
	errs := validate.ValidateDocument(makeBSRDoc("rel-1", "bs-a", "bs-a", "depends_on"))
	if !hasFieldErr(errs, "spec.target_business_service_id") {
		t.Errorf("expected self-reference error on target field; got %+v", errs)
	}
	// Sanity: error message should reference the "must differ" rule.
	if !errsContain(errs, "must differ") {
		t.Errorf("expected 'must differ' message; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_RejectsBadIDFormat(t *testing.T) {
	errs := validate.ValidateDocument(makeBSRDoc("rel-1", "bs A", "bs-b", "depends_on"))
	if !hasFieldErr(errs, "spec.source_business_service_id") {
		t.Errorf("expected ID-format error on source; got %+v", errs)
	}
}

// ──────────────────────────────────────────────────────────────────────────
// Cross-bundle resolution & uniqueness — exercised through ValidateBundle
// ──────────────────────────────────────────────────────────────────────────

func TestValidate_BusinessServiceRelationship_ResolvesSourceFromBundle(t *testing.T) {
	docs := []parser.ParsedDocument{
		makeBSRDoc("rel-1", "bs-a", "bs-b", "depends_on"), // source declared after the BSR — order independence
		makeBSDoc("bs-a"),
		makeBSDoc("bs-b"),
	}
	errs := validate.ValidateBundle(docs)
	// The BSR validator does field-level only — referential resolution is
	// the planner's concern. Bundle-level validation should accept this
	// shape (no field errors, no triple duplicates).
	if hasKindErrs(errs, types.KindBusinessServiceRelationship) {
		t.Errorf("want no BSR validation errors when source/target appear later in the bundle; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_OrderIndependence_LaterDocumentReferenced(t *testing.T) {
	docs := []parser.ParsedDocument{
		makeBSRDoc("rel-1", "bs-a", "bs-z", "supports"),
		makeBSDoc("bs-a"),
		makeBSDoc("bs-z"),
	}
	errs := validate.ValidateBundle(docs)
	if hasKindErrs(errs, types.KindBusinessServiceRelationship) {
		t.Errorf("BSR should accept references that appear later in the bundle; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_RejectsDuplicateID_InBundle(t *testing.T) {
	docs := []parser.ParsedDocument{
		makeBSRDoc("rel-dup", "bs-a", "bs-b", "depends_on"),
		makeBSRDoc("rel-dup", "bs-a", "bs-c", "supports"),
		makeBSDoc("bs-a"), makeBSDoc("bs-b"), makeBSDoc("bs-c"),
	}
	errs := validate.ValidateBundle(docs)
	if !errsContain(errs, "duplicate resource id") {
		t.Errorf("expected duplicate-id bundle error; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_RejectsDuplicateTriple_InBundle(t *testing.T) {
	docs := []parser.ParsedDocument{
		makeBSRDoc("rel-1", "bs-a", "bs-b", "depends_on"),
		makeBSRDoc("rel-2", "bs-a", "bs-b", "depends_on"), // same triple, different id
		makeBSDoc("bs-a"), makeBSDoc("bs-b"),
	}
	errs := validate.ValidateBundle(docs)
	if !errsContain(errs, "duplicate business-service relationship triple") {
		t.Errorf("expected duplicate-triple bundle error; got %+v", errs)
	}
}

func TestValidate_BusinessServiceRelationship_DifferentTypes_NotDuplicateTriple(t *testing.T) {
	// Same source+target with different relationship_type is allowed (the
	// brief specifies (source, target, type) as the uniqueness key).
	docs := []parser.ParsedDocument{
		makeBSRDoc("rel-1", "bs-a", "bs-b", "depends_on"),
		makeBSRDoc("rel-2", "bs-a", "bs-b", "supports"),
		makeBSDoc("bs-a"), makeBSDoc("bs-b"),
	}
	errs := validate.ValidateBundle(docs)
	for _, e := range errs {
		if strings.Contains(e.Message, "duplicate business-service relationship triple") {
			t.Errorf("different types should not be a duplicate triple: %+v", e)
		}
	}
}

// hasFieldErr returns true if any error references the given field.
func hasFieldErr(errs []types.ValidationError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

// hasKindErrs returns true if any error is for the given kind.
func hasKindErrs(errs []types.ValidationError, kind string) bool {
	for _, e := range errs {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func errsContain(errs []types.ValidationError, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e.Message, substr) {
			return true
		}
	}
	return false
}
