package apply

// Mapper coverage for the shared mapExternalRefSpec helper and the
// per-entity wiring on the five mappers extended in Epic 1, PR 3.
//
// The shared helper's contract is:
//
//   - nil spec → nil ref
//   - all-empty spec → nil ref (canonicalisation)
//   - populated spec → ref with whitespace trimmed and timestamp
//     normalised to UTC
//   - non-RFC3339 timestamp → error returned to caller (the validator
//     should have caught this upstream; the mapper guards against
//     programmer error)

import (
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/types"
)

func TestMapExternalRefSpec_NilReturnsNil(t *testing.T) {
	got, err := mapExternalRefSpec(nil)
	if err != nil {
		t.Fatalf("nil spec must not error: %v", err)
	}
	if got != nil {
		t.Errorf("nil spec must return nil ref; got %+v", got)
	}
}

func TestMapExternalRefSpec_AllEmptyCanonicaliseToNil(t *testing.T) {
	got, err := mapExternalRefSpec(&types.ExternalRefSpec{})
	if err != nil {
		t.Fatalf("empty spec must not error: %v", err)
	}
	if got != nil {
		t.Errorf("empty spec must canonicalise to nil; got %+v", got)
	}
}

func TestMapExternalRefSpec_WhitespaceOnlyCanonicaliseToNil(t *testing.T) {
	spec := &types.ExternalRefSpec{
		SourceSystem: "  ", SourceID: "  ", SourceURL: "  ",
		SourceVersion: "  ", LastSyncedAt: "  ",
	}
	got, err := mapExternalRefSpec(spec)
	if err != nil {
		t.Fatalf("whitespace spec must not error: %v", err)
	}
	if got != nil {
		t.Errorf("whitespace spec must canonicalise to nil; got %+v", got)
	}
}

func TestMapExternalRefSpec_TrimsWhitespace(t *testing.T) {
	spec := &types.ExternalRefSpec{
		SourceSystem:  "  github  ",
		SourceID:      "  accept-io/midas  ",
		SourceURL:     "  https://github.com/accept-io/midas  ",
		SourceVersion: "  v1.2.0  ",
		LastSyncedAt:  "2026-04-30T09:00:00Z",
	}
	got, err := mapExternalRefSpec(spec)
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if got.SourceSystem != "github" || got.SourceID != "accept-io/midas" ||
		got.SourceURL != "https://github.com/accept-io/midas" || got.SourceVersion != "v1.2.0" {
		t.Errorf("trim failed: %+v", got)
	}
}

func TestMapExternalRefSpec_LastSyncedAtNormalisedToUTC(t *testing.T) {
	// Provide a timestamp with a non-zero offset; mapper must normalise to UTC.
	spec := &types.ExternalRefSpec{
		SourceSystem: "github", SourceID: "x",
		LastSyncedAt: "2026-04-30T11:00:00+02:00",
	}
	got, err := mapExternalRefSpec(spec)
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if got.LastSyncedAt == nil {
		t.Fatal("LastSyncedAt nil after parse")
	}
	want := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	if !got.LastSyncedAt.Equal(want) {
		t.Errorf("LastSyncedAt UTC normalisation: got %v, want %v", *got.LastSyncedAt, want)
	}
	if got.LastSyncedAt.Location() != time.UTC {
		t.Errorf("LastSyncedAt location: got %v, want UTC", got.LastSyncedAt.Location())
	}
}

func TestMapExternalRefSpec_BadTimestampReturnsError(t *testing.T) {
	spec := &types.ExternalRefSpec{
		SourceSystem: "github", SourceID: "x",
		LastSyncedAt: "not-a-timestamp",
	}
	if _, err := mapExternalRefSpec(spec); err == nil {
		t.Error("bad timestamp must error")
	}
}

func TestMapExternalRefSpec_OnlySystemAndID(t *testing.T) {
	got, err := mapExternalRefSpec(&types.ExternalRefSpec{SourceSystem: "github", SourceID: "x"})
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if got == nil || got.SourceSystem != "github" || got.SourceID != "x" {
		t.Errorf("system+id only: got %+v", got)
	}
	if got.LastSyncedAt != nil {
		t.Errorf("LastSyncedAt should be nil; got %v", *got.LastSyncedAt)
	}
}

// ---------------------------------------------------------------------------
// Per-entity wiring — confirm each entity mapper invokes mapExternalRefSpec
// and assigns the result to the entity's ExternalRef field.
// ---------------------------------------------------------------------------

func validExtSpec() *types.ExternalRefSpec {
	return &types.ExternalRefSpec{
		SourceSystem: "github", SourceID: "accept-io/midas",
		SourceURL: "https://github.com/accept-io/midas", SourceVersion: "v1.2.0",
		LastSyncedAt: "2026-04-30T09:00:00Z",
	}
}

func TestMapper_BusinessService_PropagatesExternalRef(t *testing.T) {
	doc := types.BusinessServiceDocument{
		Metadata: types.DocumentMetadata{ID: "bs-1", Name: "BS"},
		Spec: types.BusinessServiceSpec{
			ServiceType: "internal", Status: "active",
			ExternalRef: validExtSpec(),
		},
	}
	bs, err := mapBusinessServiceDocumentToBusinessService(doc, time.Now())
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if bs.ExternalRef == nil || bs.ExternalRef.SourceID != "accept-io/midas" {
		t.Errorf("ExternalRef not propagated: %+v", bs.ExternalRef)
	}
}

func TestMapper_BSR_PropagatesExternalRef(t *testing.T) {
	doc := types.BusinessServiceRelationshipDocument{
		Metadata: types.DocumentMetadata{ID: "rel-1"},
		Spec: types.BusinessServiceRelationshipSpec{
			SourceBusinessServiceID: "bs-a", TargetBusinessServiceID: "bs-b", RelationshipType: "depends_on",
			ExternalRef: validExtSpec(),
		},
	}
	rel, err := mapBusinessServiceRelationshipDocumentToBusinessServiceRelationship(doc, time.Now(), "u")
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if rel.ExternalRef == nil || rel.ExternalRef.SourceVersion != "v1.2.0" {
		t.Errorf("ExternalRef not propagated: %+v", rel.ExternalRef)
	}
}

func TestMapper_AISystem_PropagatesExternalRef(t *testing.T) {
	doc := types.AISystemDocument{
		Metadata: types.DocumentMetadata{ID: "ai-1", Name: "AI"},
		Spec: types.AISystemSpec{
			Status: "active", Origin: "manual",
			ExternalRef: validExtSpec(),
		},
	}
	sys, err := mapAISystemDocumentToAISystem(doc, time.Now(), "u")
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if sys.ExternalRef == nil || sys.ExternalRef.LastSyncedAt == nil {
		t.Errorf("ExternalRef not propagated: %+v", sys.ExternalRef)
	}
}

func TestMapper_AIVersion_PropagatesExternalRef(t *testing.T) {
	doc := types.AISystemVersionDocument{
		Metadata: types.DocumentMetadata{ID: "aiv-1"},
		Spec: types.AISystemVersionSpec{
			AISystemID: "ai-1", Version: 1, Status: "active",
			EffectiveFrom: "2026-04-15T00:00:00Z",
			ExternalRef:   validExtSpec(),
		},
	}
	ver, err := mapAISystemVersionDocumentToAISystemVersion(doc, time.Now(), "u")
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if ver.ExternalRef == nil || ver.ExternalRef.SourceURL == "" {
		t.Errorf("ExternalRef not propagated: %+v", ver.ExternalRef)
	}
}

func TestMapper_AIBinding_PropagatesExternalRef(t *testing.T) {
	doc := types.AISystemBindingDocument{
		Metadata: types.DocumentMetadata{ID: "bind-1"},
		Spec: types.AISystemBindingSpec{
			AISystemID: "ai-1", BusinessServiceID: "bs-x",
			ExternalRef: validExtSpec(),
		},
	}
	b, err := mapAISystemBindingDocumentToAISystemBinding(doc, time.Now(), "u")
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if b.ExternalRef == nil || b.ExternalRef.SourceSystem != "github" {
		t.Errorf("ExternalRef not propagated: %+v", b.ExternalRef)
	}
}

func TestMapper_AllFiveEntities_PassThroughNilExternalRef(t *testing.T) {
	now := time.Now()

	bs, _ := mapBusinessServiceDocumentToBusinessService(types.BusinessServiceDocument{
		Spec: types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
	}, now)
	if bs.ExternalRef != nil {
		t.Errorf("BS: nil spec must produce nil ExternalRef; got %+v", bs.ExternalRef)
	}

	rel, _ := mapBusinessServiceRelationshipDocumentToBusinessServiceRelationship(types.BusinessServiceRelationshipDocument{}, now, "u")
	if rel.ExternalRef != nil {
		t.Errorf("BSR: nil spec must produce nil ExternalRef; got %+v", rel.ExternalRef)
	}

	sys, _ := mapAISystemDocumentToAISystem(types.AISystemDocument{}, now, "u")
	if sys.ExternalRef != nil {
		t.Errorf("AISystem: nil spec must produce nil ExternalRef; got %+v", sys.ExternalRef)
	}

	ver, _ := mapAISystemVersionDocumentToAISystemVersion(types.AISystemVersionDocument{
		Spec: types.AISystemVersionSpec{
			AISystemID: "ai-1", Version: 1, Status: "active",
			EffectiveFrom: "2026-04-15T00:00:00Z",
		},
	}, now, "u")
	if ver.ExternalRef != nil {
		t.Errorf("AIVersion: nil spec must produce nil ExternalRef; got %+v", ver.ExternalRef)
	}

	b, _ := mapAISystemBindingDocumentToAISystemBinding(types.AISystemBindingDocument{}, now, "u")
	if b.ExternalRef != nil {
		t.Errorf("AIBinding: nil spec must produce nil ExternalRef; got %+v", b.ExternalRef)
	}
}
