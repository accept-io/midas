package postgres

// Postgres-backed tests for BusinessServiceRelationshipRepo. Gated on
// DATABASE_URL via openTestDB; the suite skips when not set, mirroring
// the convention used by every other Postgres repo test in this package.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
)

// seedBSR_TestServices inserts two BSes that BSR tests reference. Returns
// the seeded ids and a cleanup hook. Tests that need additional services
// can extend before invoking. Cleanup deletes both seeded BSes; row-level
// cleanup of the BSR table itself is the responsibility of each test
// (mirrors the per-test cleanup pattern used elsewhere in this package).
func seedBSR_TestServices(t *testing.T, ctx context.Context, repo *BusinessServiceRepo, ids ...string) func() {
	t.Helper()
	if len(ids) == 0 {
		ids = []string{"tst-bsr-src", "tst-bsr-tgt"}
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, id := range ids {
		svc := &businessservice.BusinessService{
			ID:          id,
			Name:        id,
			ServiceType: businessservice.ServiceTypeInternal,
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := repo.Create(ctx, svc); err != nil {
			t.Fatalf("seed BS %q: %v", id, err)
		}
	}
	return func() {
		// Best-effort cleanup. Tests that delete BS rows themselves should
		// no-op here; the DELETE WHERE id = $1 pattern below tolerates that.
	}
}

// newBSRRepo opens the test DB and returns the BSR repo + the BS repo
// (callers seed referenced BSes with the latter) + cleanup. Tests should
// call db.Close() inside t.Cleanup so the cleanup query runs against an
// open connection.
func newBSRRepo(t *testing.T) (*BusinessServiceRelationshipRepo, *BusinessServiceRepo, context.Context) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() {
		// Per-test row cleanup. Each test inserts ids prefixed with
		// "tst-bsr-" or "tst-bsr-rel-"; this best-effort cleanup catches
		// stragglers without coupling to a specific ID set.
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM business_service_relationships WHERE id LIKE 'tst-bsr-%'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM business_services WHERE business_service_id LIKE 'tst-bsr-%'`)
		db.Close()
	})

	bsRepo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}
	bsrRepo, err := NewBusinessServiceRelationshipRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRelationshipRepo: %v", err)
	}
	return bsrRepo, bsRepo, context.Background()
}

func TestPostgresRelationshipRepo_Create_Persists(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-a", "tst-bsr-b")

	now := time.Now().UTC().Truncate(time.Millisecond)
	rel := &businessservice.BusinessServiceRelationship{
		ID:                    "tst-bsr-rel-1",
		SourceBusinessService: "tst-bsr-a",
		TargetBusinessService: "tst-bsr-b",
		RelationshipType:      businessservice.RelationshipTypeDependsOn,
		Description:           "round-trip test",
		CreatedAt:             now,
		CreatedBy:             "test",
	}
	if err := bsr.Create(ctx, rel); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := bsr.GetByID(ctx, "tst-bsr-rel-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SourceBusinessService != "tst-bsr-a" || got.TargetBusinessService != "tst-bsr-b" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Description != "round-trip test" {
		t.Errorf("description round-trip mismatch: got %q", got.Description)
	}
}

func TestPostgresRelationshipRepo_Create_RejectsSelfReference_ViaCheckConstraint(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-self")

	rel := &businessservice.BusinessServiceRelationship{
		ID:                    "tst-bsr-rel-self",
		SourceBusinessService: "tst-bsr-self",
		TargetBusinessService: "tst-bsr-self",
		RelationshipType:      businessservice.RelationshipTypeDependsOn,
		CreatedAt:             time.Now().UTC(),
	}
	err := bsr.Create(ctx, rel)
	if !errors.Is(err, businessservice.ErrRelationshipSelfReference) {
		t.Errorf("want ErrRelationshipSelfReference, got %v", err)
	}
}

func TestPostgresRelationshipRepo_Create_RejectsInvalidRelationshipType_ViaCheckConstraint(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-x", "tst-bsr-y")

	rel := &businessservice.BusinessServiceRelationship{
		ID:                    "tst-bsr-rel-bad",
		SourceBusinessService: "tst-bsr-x",
		TargetBusinessService: "tst-bsr-y",
		RelationshipType:      "invented_type",
		CreatedAt:             time.Now().UTC(),
	}
	err := bsr.Create(ctx, rel)
	if !errors.Is(err, businessservice.ErrRelationshipInvalidType) {
		t.Errorf("want ErrRelationshipInvalidType, got %v", err)
	}
}

func TestPostgresRelationshipRepo_Create_RejectsDuplicateTriple_ViaUniqueConstraint(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-dt-a", "tst-bsr-dt-b")

	now := time.Now().UTC()
	if err := bsr.Create(ctx, &businessservice.BusinessServiceRelationship{
		ID: "tst-bsr-rel-dt-1", SourceBusinessService: "tst-bsr-dt-a", TargetBusinessService: "tst-bsr-dt-b",
		RelationshipType: businessservice.RelationshipTypeDependsOn, CreatedAt: now,
	}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	err := bsr.Create(ctx, &businessservice.BusinessServiceRelationship{
		ID: "tst-bsr-rel-dt-2", SourceBusinessService: "tst-bsr-dt-a", TargetBusinessService: "tst-bsr-dt-b",
		RelationshipType: businessservice.RelationshipTypeDependsOn, CreatedAt: now,
	})
	if !errors.Is(err, businessservice.ErrRelationshipDuplicateTriple) {
		t.Errorf("want ErrRelationshipDuplicateTriple, got %v", err)
	}
}

func TestPostgresRelationshipRepo_Create_RejectsUnknownSourceBusinessService_ViaForeignKey(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-fk-tgt")

	rel := &businessservice.BusinessServiceRelationship{
		ID:                    "tst-bsr-rel-fksrc",
		SourceBusinessService: "tst-bsr-does-not-exist",
		TargetBusinessService: "tst-bsr-fk-tgt",
		RelationshipType:      businessservice.RelationshipTypeDependsOn,
		CreatedAt:             time.Now().UTC(),
	}
	err := bsr.Create(ctx, rel)
	if err == nil {
		t.Fatal("want FK violation, got nil")
	}
	if !strings.Contains(err.Error(), "referenced business service not found") {
		t.Errorf("expected FK message, got %v", err)
	}
}

func TestPostgresRelationshipRepo_Create_RejectsUnknownTargetBusinessService_ViaForeignKey(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-fk-src")

	rel := &businessservice.BusinessServiceRelationship{
		ID:                    "tst-bsr-rel-fktgt",
		SourceBusinessService: "tst-bsr-fk-src",
		TargetBusinessService: "tst-bsr-does-not-exist",
		RelationshipType:      businessservice.RelationshipTypeDependsOn,
		CreatedAt:             time.Now().UTC(),
	}
	err := bsr.Create(ctx, rel)
	if err == nil {
		t.Fatal("want FK violation, got nil")
	}
	if !strings.Contains(err.Error(), "referenced business service not found") {
		t.Errorf("expected FK message, got %v", err)
	}
}

func TestPostgresRelationshipRepo_GetByID_Found(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-g-a", "tst-bsr-g-b")

	created := time.Now().UTC().Truncate(time.Millisecond)
	rel := &businessservice.BusinessServiceRelationship{
		ID:                    "tst-bsr-rel-get",
		SourceBusinessService: "tst-bsr-g-a",
		TargetBusinessService: "tst-bsr-g-b",
		RelationshipType:      businessservice.RelationshipTypeSupports,
		Description:           "supports relationship",
		CreatedAt:             created,
		CreatedBy:             "operator:test",
	}
	if err := bsr.Create(ctx, rel); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := bsr.GetByID(ctx, "tst-bsr-rel-get")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.RelationshipType != businessservice.RelationshipTypeSupports || got.CreatedBy != "operator:test" {
		t.Errorf("unexpected fields: %+v", got)
	}
}

func TestPostgresRelationshipRepo_GetByID_NotFound_ReturnsErr(t *testing.T) {
	bsr, _, ctx := newBSRRepo(t)
	_, err := bsr.GetByID(ctx, "tst-bsr-rel-missing")
	if !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		t.Errorf("want ErrRelationshipNotFound, got %v", err)
	}
}

func TestPostgresRelationshipRepo_ListBySourceBusinessService_FiltersCorrectly(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-ls-src", "tst-bsr-ls-a", "tst-bsr-ls-b", "tst-bsr-ls-c")
	now := time.Now().UTC().Truncate(time.Millisecond)
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-ls-1", "tst-bsr-ls-src", "tst-bsr-ls-a", businessservice.RelationshipTypeDependsOn, now)
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-ls-2", "tst-bsr-ls-src", "tst-bsr-ls-b", businessservice.RelationshipTypeSupports, now)
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-ls-3", "tst-bsr-ls-c", "tst-bsr-ls-a", businessservice.RelationshipTypeDependsOn, now)

	got, err := bsr.ListBySourceBusinessService(ctx, "tst-bsr-ls-src")
	if err != nil {
		t.Fatalf("ListBySourceBusinessService: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 source matches, got %d", len(got))
	}
}

func TestPostgresRelationshipRepo_ListByTargetBusinessService_FiltersCorrectly(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-lt-tgt", "tst-bsr-lt-x", "tst-bsr-lt-y", "tst-bsr-lt-z")
	now := time.Now().UTC().Truncate(time.Millisecond)
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-lt-1", "tst-bsr-lt-x", "tst-bsr-lt-tgt", businessservice.RelationshipTypeDependsOn, now)
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-lt-2", "tst-bsr-lt-y", "tst-bsr-lt-tgt", businessservice.RelationshipTypeSupports, now)
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-lt-3", "tst-bsr-lt-z", "tst-bsr-lt-x", businessservice.RelationshipTypeDependsOn, now)

	got, err := bsr.ListByTargetBusinessService(ctx, "tst-bsr-lt-tgt")
	if err != nil {
		t.Fatalf("ListByTargetBusinessService: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 target matches, got %d", len(got))
	}
}

func TestPostgresRelationshipRepo_Delete_RemovesRow(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-d-a", "tst-bsr-d-b")
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-del", "tst-bsr-d-a", "tst-bsr-d-b", businessservice.RelationshipTypeDependsOn, time.Now().UTC())

	if err := bsr.Delete(ctx, "tst-bsr-rel-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := bsr.GetByID(ctx, "tst-bsr-rel-del"); !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		t.Errorf("want ErrRelationshipNotFound after delete, got %v", err)
	}
}

func TestPostgresRelationshipRepo_OnDeleteCascade_RemovesRowWhenSourceDeleted(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-cd-src", "tst-bsr-cd-tgt")
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-cd", "tst-bsr-cd-src", "tst-bsr-cd-tgt", businessservice.RelationshipTypeDependsOn, time.Now().UTC())

	// Delete the source business service. ON DELETE CASCADE should remove
	// the BSR row even though we delete via raw SQL.
	db := bsr.db
	if _, err := db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, "tst-bsr-cd-src"); err != nil {
		t.Fatalf("delete source BS: %v", err)
	}
	if _, err := bsr.GetByID(ctx, "tst-bsr-rel-cd"); !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		t.Errorf("expected BSR row to be cascaded; GetByID returned %v", err)
	}
}

func TestPostgresRelationshipRepo_OnDeleteRestrict_PreventsTargetDeletion(t *testing.T) {
	bsr, bs, ctx := newBSRRepo(t)
	seedBSR_TestServices(t, ctx, bs, "tst-bsr-rs-src", "tst-bsr-rs-tgt")
	mustCreatePG(t, bsr, ctx, "tst-bsr-rel-rs", "tst-bsr-rs-src", "tst-bsr-rs-tgt", businessservice.RelationshipTypeDependsOn, time.Now().UTC())

	// Attempting to delete the target BS should fail because the BSR's
	// target FK is ON DELETE RESTRICT.
	db := bsr.db
	_, err := db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, "tst-bsr-rs-tgt")
	if err == nil {
		t.Fatal("expected FK RESTRICT to prevent target deletion, got nil")
	}
	// Be lenient about the exact message — different PG versions phrase it
	// differently. The presence of either "foreign key" or "violates" is enough.
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "foreign key") && !strings.Contains(low, "violates") {
		t.Errorf("expected FK violation, got %v", err)
	}
}

func mustCreatePG(t *testing.T, bsr *BusinessServiceRelationshipRepo, ctx context.Context, id, source, target, relType string, createdAt time.Time) {
	t.Helper()
	if err := bsr.Create(ctx, &businessservice.BusinessServiceRelationship{
		ID:                    id,
		SourceBusinessService: source,
		TargetBusinessService: target,
		RelationshipType:      relType,
		CreatedAt:             createdAt,
	}); err != nil {
		t.Fatalf("Create %s: %v", id, err)
	}
}
