package memory

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
)

// makeRel builds a BSR with sensible defaults that callers override.
func makeRel(id, source, target, relType string, createdAt time.Time) *businessservice.BusinessServiceRelationship {
	return &businessservice.BusinessServiceRelationship{
		ID:                    id,
		SourceBusinessService: source,
		TargetBusinessService: target,
		RelationshipType:      relType,
		Description:           "test relationship",
		CreatedAt:             createdAt,
		CreatedBy:             "test",
	}
}

// newMemRepoNoFK constructs a memory repo without the BS validator, so tests
// that don't care about referenced-BS existence don't have to seed dummy BSes.
func newMemRepoNoFK() *BusinessServiceRelationshipRepo {
	return NewBusinessServiceRelationshipRepo()
}

func TestMemoryRelationshipRepo_Create_Persists(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	rel := makeRel("rel-1", "bs-a", "bs-b", businessservice.RelationshipTypeDependsOn, time.Now())
	if err := repo.Create(ctx, rel); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "rel-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SourceBusinessService != "bs-a" || got.TargetBusinessService != "bs-b" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestMemoryRelationshipRepo_Create_RejectsSelfReference(t *testing.T) {
	repo := newMemRepoNoFK()
	rel := makeRel("rel-self", "bs-a", "bs-a", businessservice.RelationshipTypeDependsOn, time.Now())
	if err := repo.Create(context.Background(), rel); !errors.Is(err, businessservice.ErrRelationshipSelfReference) {
		t.Errorf("want ErrRelationshipSelfReference, got %v", err)
	}
}

func TestMemoryRelationshipRepo_Create_RejectsInvalidRelationshipType(t *testing.T) {
	repo := newMemRepoNoFK()
	rel := makeRel("rel-bad", "bs-a", "bs-b", "invented_type", time.Now())
	if err := repo.Create(context.Background(), rel); !errors.Is(err, businessservice.ErrRelationshipInvalidType) {
		t.Errorf("want ErrRelationshipInvalidType, got %v", err)
	}
}

func TestMemoryRelationshipRepo_Create_RejectsDuplicateID(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	if err := repo.Create(ctx, makeRel("rel-dup", "bs-a", "bs-b", businessservice.RelationshipTypeDependsOn, time.Now())); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// Same ID, different triple — duplicate-id rule still fires.
	err := repo.Create(ctx, makeRel("rel-dup", "bs-a", "bs-c", businessservice.RelationshipTypeSupports, time.Now()))
	if !errors.Is(err, businessservice.ErrRelationshipDuplicateID) {
		t.Errorf("want ErrRelationshipDuplicateID, got %v", err)
	}
}

func TestMemoryRelationshipRepo_Create_RejectsDuplicateTriple(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	if err := repo.Create(ctx, makeRel("rel-1", "bs-a", "bs-b", businessservice.RelationshipTypeDependsOn, time.Now())); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// Same triple, different ID — triple-uniqueness rule fires.
	err := repo.Create(ctx, makeRel("rel-2", "bs-a", "bs-b", businessservice.RelationshipTypeDependsOn, time.Now()))
	if !errors.Is(err, businessservice.ErrRelationshipDuplicateTriple) {
		t.Errorf("want ErrRelationshipDuplicateTriple, got %v", err)
	}
}

func TestMemoryRelationshipRepo_GetByID_Found(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	created := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	rel := makeRel("rel-found", "bs-a", "bs-b", businessservice.RelationshipTypeSupports, created)
	if err := repo.Create(ctx, rel); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "rel-found")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.CreatedAt.Equal(created) || got.RelationshipType != businessservice.RelationshipTypeSupports {
		t.Errorf("unexpected fields: %+v", got)
	}
}

func TestMemoryRelationshipRepo_GetByID_NotFound_ReturnsErr(t *testing.T) {
	repo := newMemRepoNoFK()
	_, err := repo.GetByID(context.Background(), "rel-missing")
	if !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		t.Errorf("want ErrRelationshipNotFound, got %v", err)
	}
}

func TestMemoryRelationshipRepo_List_ReturnsDeterministicOrder(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	t0 := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	// Insert three rows with two distinct created_at values to exercise both
	// the DESC time order and the ID-ASC tiebreak.
	rels := []*businessservice.BusinessServiceRelationship{
		makeRel("rel-c", "bs-a", "bs-b", businessservice.RelationshipTypeDependsOn, t0),                  // older
		makeRel("rel-a", "bs-a", "bs-c", businessservice.RelationshipTypeSupports, t0.Add(time.Minute)),  // newer (tied with rel-b)
		makeRel("rel-b", "bs-a", "bs-d", businessservice.RelationshipTypeDependsOn, t0.Add(time.Minute)), // newer (tied with rel-a)
	}
	for _, r := range rels {
		if err := repo.Create(ctx, r); err != nil {
			t.Fatalf("Create %s: %v", r.ID, err)
		}
	}
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	// Expected: rel-a (newer, ID < rel-b), rel-b (newer), rel-c (older)
	wantOrder := []string{"rel-a", "rel-b", "rel-c"}
	for i, r := range got {
		if r.ID != wantOrder[i] {
			t.Errorf("position %d: want %s, got %s (full order=%v)", i, wantOrder[i], r.ID, idsOf(got))
		}
	}
}

func TestMemoryRelationshipRepo_ListBySourceBusinessService_FiltersCorrectly(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	now := time.Now()
	mustCreate(t, repo, ctx, makeRel("rel-1", "bs-source", "bs-target-a", businessservice.RelationshipTypeDependsOn, now))
	mustCreate(t, repo, ctx, makeRel("rel-2", "bs-source", "bs-target-b", businessservice.RelationshipTypeSupports, now))
	mustCreate(t, repo, ctx, makeRel("rel-3", "bs-other", "bs-target-a", businessservice.RelationshipTypeDependsOn, now))

	got, err := repo.ListBySourceBusinessService(ctx, "bs-source")
	if err != nil {
		t.Fatalf("ListBySource: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 source matches, got %d", len(got))
	}
	for _, r := range got {
		if r.SourceBusinessService != "bs-source" {
			t.Errorf("filter leak: %+v", r)
		}
	}
}

func TestMemoryRelationshipRepo_ListByTargetBusinessService_FiltersCorrectly(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	now := time.Now()
	mustCreate(t, repo, ctx, makeRel("rel-1", "bs-source-a", "bs-target", businessservice.RelationshipTypeDependsOn, now))
	mustCreate(t, repo, ctx, makeRel("rel-2", "bs-source-b", "bs-target", businessservice.RelationshipTypeSupports, now))
	mustCreate(t, repo, ctx, makeRel("rel-3", "bs-source-a", "bs-other", businessservice.RelationshipTypeDependsOn, now))

	got, err := repo.ListByTargetBusinessService(ctx, "bs-target")
	if err != nil {
		t.Fatalf("ListByTarget: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 target matches, got %d", len(got))
	}
	for _, r := range got {
		if r.TargetBusinessService != "bs-target" {
			t.Errorf("filter leak: %+v", r)
		}
	}
}

func TestMemoryRelationshipRepo_Delete_RemovesRow(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	mustCreate(t, repo, ctx, makeRel("rel-del", "bs-a", "bs-b", businessservice.RelationshipTypeDependsOn, time.Now()))
	if err := repo.Delete(ctx, "rel-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, "rel-del"); !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		t.Errorf("expected not-found after delete, got %v", err)
	}
}

func TestMemoryRelationshipRepo_Delete_NotFound_ReturnsErr(t *testing.T) {
	repo := newMemRepoNoFK()
	if err := repo.Delete(context.Background(), "rel-missing"); !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		t.Errorf("want ErrRelationshipNotFound on delete-missing, got %v", err)
	}
}

func TestMemoryRelationshipRepo_DefensiveCopy_CallerCannotMutateStoredValue(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	mustCreate(t, repo, ctx, makeRel("rel-defcopy", "bs-a", "bs-b", businessservice.RelationshipTypeDependsOn, time.Now()))
	got, _ := repo.GetByID(ctx, "rel-defcopy")
	got.Description = "tamper"
	got.SourceBusinessService = "tampered-source"

	again, _ := repo.GetByID(ctx, "rel-defcopy")
	if again.Description == "tamper" || again.SourceBusinessService == "tampered-source" {
		t.Error("repo returned a non-defensive copy: caller mutation leaked into store")
	}
}

func TestMemoryRelationshipRepo_Update_MutatesDescriptionOnly(t *testing.T) {
	repo := newMemRepoNoFK()
	ctx := context.Background()
	mustCreate(t, repo, ctx, makeRel("rel-upd", "bs-a", "bs-b", businessservice.RelationshipTypeDependsOn, time.Now()))
	updated := &businessservice.BusinessServiceRelationship{
		ID:                    "rel-upd",
		SourceBusinessService: "ignored",
		TargetBusinessService: "ignored",
		RelationshipType:      "ignored",
		Description:           "updated description",
	}
	if err := repo.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := repo.GetByID(ctx, "rel-upd")
	if got.Description != "updated description" {
		t.Errorf("Description not updated: got %q", got.Description)
	}
	// Source/Target/RelationshipType must NOT have been mutated.
	if got.SourceBusinessService != "bs-a" || got.TargetBusinessService != "bs-b" || got.RelationshipType != businessservice.RelationshipTypeDependsOn {
		t.Errorf("Update mutated immutable fields: %+v", got)
	}
}

func TestMemoryRelationshipRepo_Update_NotFound_ReturnsErr(t *testing.T) {
	repo := newMemRepoNoFK()
	err := repo.Update(context.Background(), &businessservice.BusinessServiceRelationship{ID: "rel-missing"})
	if !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		t.Errorf("want ErrRelationshipNotFound, got %v", err)
	}
}

// mustCreate is a test helper that fails the test on Create error.
func mustCreate(t *testing.T, repo *BusinessServiceRelationshipRepo, ctx context.Context, rel *businessservice.BusinessServiceRelationship) {
	t.Helper()
	if err := repo.Create(ctx, rel); err != nil {
		t.Fatalf("Create %s: %v", rel.ID, err)
	}
}

// idsOf is a small helper for diagnostic output.
func idsOf(rels []*businessservice.BusinessServiceRelationship) []string {
	out := make([]string, len(rels))
	for i, r := range rels {
		out[i] = r.ID
	}
	sort.Strings(out)
	return out
}
