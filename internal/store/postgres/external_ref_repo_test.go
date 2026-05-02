package postgres

// Postgres-backed ExternalRef round-trip tests for the five entities
// that gained the ext_* columns in Epic 1, PR 3.
//
// Gated on DATABASE_URL via openTestDB; the suite skips when not set.
//
// These tests are LOAD-BEARING for the schema CHECK migration block:
// the chk_<table>_ext_consistency CHECKs are added via DROP/ADD inside
// a DO $$ ... $$ block at the bottom of schema.sql. If the migration
// failed to run (or if ALTER TABLE ADD COLUMN IF NOT EXISTS dropped a
// column), the inconsistent-ExternalRef rejection tests below would
// pass through and persist invalid rows. They explicitly verify
// pq.Error 23514 maps to externalref.ErrInconsistent for all five
// tables.

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/externalref"
)

// pgExtRefFixture builds a populated ExternalRef. The timestamp is
// truncated to millisecond precision so round-trip equality compares
// cleanly against the value Postgres returns.
func pgExtRefFixture() *externalref.ExternalRef {
	t := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	return &externalref.ExternalRef{
		SourceSystem:  "github",
		SourceID:      "accept-io/midas-pg-ext",
		SourceURL:     "https://github.com/accept-io/midas-pg-ext",
		SourceVersion: "v1.2.0",
		LastSyncedAt:  &t,
	}
}

// pgInconsistentExtRef returns an ExternalRef that violates the
// consistency rule (system set, id empty). All five Create paths must
// translate the resulting Postgres CHECK violation to
// externalref.ErrInconsistent via mapExtRefError.
func pgInconsistentExtRef() *externalref.ExternalRef {
	return &externalref.ExternalRef{SourceSystem: "github" /* SourceID intentionally empty */}
}

// assertExtRefRoundTrip checks every field on a round-tripped
// ExternalRef against the fixture, with millisecond truncation on the
// timestamp comparison to absorb Postgres's storage precision.
func assertExtRefRoundTrip(t *testing.T, got *externalref.ExternalRef) {
	t.Helper()
	if got == nil {
		t.Fatalf("ExternalRef nil after round-trip; want populated")
	}
	want := pgExtRefFixture()
	if got.SourceSystem != want.SourceSystem || got.SourceID != want.SourceID ||
		got.SourceURL != want.SourceURL || got.SourceVersion != want.SourceVersion {
		t.Errorf("ExternalRef text fields mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if got.LastSyncedAt == nil {
		t.Errorf("LastSyncedAt nil after round-trip; want %v", *want.LastSyncedAt)
		return
	}
	if !got.LastSyncedAt.Equal(*want.LastSyncedAt) {
		t.Errorf("LastSyncedAt mismatch: got %v, want %v", *got.LastSyncedAt, *want.LastSyncedAt)
	}
}

// ---------------------------------------------------------------------------
// BusinessService
// ---------------------------------------------------------------------------

func newPGBSRepoForExt(t *testing.T) (*BusinessServiceRepo, context.Context, *sql.DB) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM business_services WHERE business_service_id LIKE 'tst-ext-%'`)
		db.Close()
	})
	r, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}
	return r, context.Background(), db
}

func makePGBSWithExt(id string, ref *externalref.ExternalRef) *businessservice.BusinessService {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &businessservice.BusinessService{
		ID: id, Name: id, ServiceType: businessservice.ServiceTypeInternal,
		Status: "active", Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
		ExternalRef: ref,
	}
}

func TestPGBSRepo_ExternalRef_RoundTrip(t *testing.T) {
	r, ctx, _ := newPGBSRepoForExt(t)
	if err := r.Create(ctx, makePGBSWithExt("tst-ext-bs-rt", pgExtRefFixture())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByID(ctx, "tst-ext-bs-rt")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	assertExtRefRoundTrip(t, got.ExternalRef)
}

func TestPGBSRepo_ExternalRef_NilColumns_ReturnNil(t *testing.T) {
	r, ctx, _ := newPGBSRepoForExt(t)
	if err := r.Create(ctx, makePGBSWithExt("tst-ext-bs-nil", nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := r.GetByID(ctx, "tst-ext-bs-nil")
	if got.ExternalRef != nil {
		t.Errorf("expected nil ExternalRef from NULL columns; got %+v", got.ExternalRef)
	}
}

func TestPGBSRepo_ExternalRef_RejectsInconsistent_ViaCheckConstraint(t *testing.T) {
	r, ctx, _ := newPGBSRepoForExt(t)
	err := r.Create(ctx, makePGBSWithExt("tst-ext-bs-bad", pgInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("Create with inconsistent ExternalRef: want ErrInconsistent, got %v", err)
	}
}

func TestPGBSRepo_ExternalRef_UpdateRoundTrips(t *testing.T) {
	r, ctx, _ := newPGBSRepoForExt(t)
	if err := r.Create(ctx, makePGBSWithExt("tst-ext-bs-upd", nil)); err != nil {
		t.Fatal(err)
	}
	upd := makePGBSWithExt("tst-ext-bs-upd", pgExtRefFixture())
	if err := r.Update(ctx, upd); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.GetByID(ctx, "tst-ext-bs-upd")
	assertExtRefRoundTrip(t, got.ExternalRef)
}

// ---------------------------------------------------------------------------
// BusinessServiceRelationship
// ---------------------------------------------------------------------------

func newPGBSRRepoForExt(t *testing.T) (*BusinessServiceRelationshipRepo, *BusinessServiceRepo, context.Context) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM business_service_relationships WHERE id LIKE 'tst-ext-bsr-%'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM business_services WHERE business_service_id LIKE 'tst-ext-bsr-%'`)
		db.Close()
	})
	bsRepo, _ := NewBusinessServiceRepo(db)
	bsrRepo, _ := NewBusinessServiceRelationshipRepo(db)
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, id := range []string{"tst-ext-bsr-a", "tst-ext-bsr-b"} {
		_ = bsRepo.Create(context.Background(), &businessservice.BusinessService{
			ID: id, Name: id, ServiceType: businessservice.ServiceTypeInternal,
			Status: "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	return bsrRepo, bsRepo, context.Background()
}

func TestPGBSRRepo_ExternalRef_RoundTrip(t *testing.T) {
	bsr, _, ctx := newPGBSRRepoForExt(t)
	rel := &businessservice.BusinessServiceRelationship{
		ID: "tst-ext-bsr-rt", SourceBusinessService: "tst-ext-bsr-a", TargetBusinessService: "tst-ext-bsr-b",
		RelationshipType: "depends_on", CreatedAt: time.Now().UTC(),
		ExternalRef: pgExtRefFixture(),
	}
	if err := bsr.Create(ctx, rel); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := bsr.GetByID(ctx, "tst-ext-bsr-rt")
	assertExtRefRoundTrip(t, got.ExternalRef)
}

func TestPGBSRRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	bsr, _, ctx := newPGBSRRepoForExt(t)
	rel := &businessservice.BusinessServiceRelationship{
		ID: "tst-ext-bsr-bad", SourceBusinessService: "tst-ext-bsr-a", TargetBusinessService: "tst-ext-bsr-b",
		RelationshipType: "depends_on", CreatedAt: time.Now().UTC(),
		ExternalRef: pgInconsistentExtRef(),
	}
	err := bsr.Create(ctx, rel)
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("want ErrInconsistent, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AISystem
// ---------------------------------------------------------------------------

func newPGAISystemRepoForExt(t *testing.T) (*AISystemRepo, context.Context) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_system_bindings WHERE id LIKE 'tst-ext-ai-%'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_system_versions WHERE ai_system_id LIKE 'tst-ext-ai-%'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_systems WHERE id LIKE 'tst-ext-ai-%'`)
		db.Close()
	})
	r, err := NewAISystemRepo(db)
	if err != nil {
		t.Fatalf("NewAISystemRepo: %v", err)
	}
	return r, context.Background()
}

func makePGAISystemWithExt(id string, ref *externalref.ExternalRef) *aisystem.AISystem {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &aisystem.AISystem{
		ID: id, Name: id,
		Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual,
		Managed: true, CreatedAt: now, UpdatedAt: now,
		ExternalRef: ref,
	}
}

func TestPGAISystemRepo_ExternalRef_RoundTrip(t *testing.T) {
	r, ctx := newPGAISystemRepoForExt(t)
	if err := r.Create(ctx, makePGAISystemWithExt("tst-ext-ai-rt", pgExtRefFixture())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := r.GetByID(ctx, "tst-ext-ai-rt")
	assertExtRefRoundTrip(t, got.ExternalRef)
}

func TestPGAISystemRepo_ExternalRef_NilColumnsReturnNil(t *testing.T) {
	r, ctx := newPGAISystemRepoForExt(t)
	if err := r.Create(ctx, makePGAISystemWithExt("tst-ext-ai-nil", nil)); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, "tst-ext-ai-nil")
	if got.ExternalRef != nil {
		t.Errorf("expected nil ExternalRef; got %+v", got.ExternalRef)
	}
}

func TestPGAISystemRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	r, ctx := newPGAISystemRepoForExt(t)
	err := r.Create(ctx, makePGAISystemWithExt("tst-ext-ai-bad", pgInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("want ErrInconsistent, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AISystemVersion
// ---------------------------------------------------------------------------

func newPGAIVersionRepoForExt(t *testing.T) (*AISystemVersionRepo, *AISystemRepo, context.Context) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_system_versions WHERE ai_system_id LIKE 'tst-ext-aiv-%'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_systems WHERE id LIKE 'tst-ext-aiv-%'`)
		db.Close()
	})
	sysRepo, _ := NewAISystemRepo(db)
	verRepo, _ := NewAISystemVersionRepo(db)
	if err := sysRepo.Create(context.Background(), makePGAISystemWithExt("tst-ext-aiv-parent", nil)); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	return verRepo, sysRepo, context.Background()
}

func makePGAIVersionWithExt(systemID string, version int, ref *externalref.ExternalRef) *aisystem.AISystemVersion {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &aisystem.AISystemVersion{
		AISystemID: systemID, Version: version,
		Status: aisystem.AISystemVersionStatusActive, EffectiveFrom: now,
		ComplianceFrameworks: []string{},
		CreatedAt:            now, UpdatedAt: now,
		ExternalRef: ref,
	}
}

func TestPGAIVersionRepo_ExternalRef_RoundTrip(t *testing.T) {
	v, _, ctx := newPGAIVersionRepoForExt(t)
	if err := v.Create(ctx, makePGAIVersionWithExt("tst-ext-aiv-parent", 1, pgExtRefFixture())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := v.GetByIDAndVersion(ctx, "tst-ext-aiv-parent", 1)
	assertExtRefRoundTrip(t, got.ExternalRef)
}

func TestPGAIVersionRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	v, _, ctx := newPGAIVersionRepoForExt(t)
	err := v.Create(ctx, makePGAIVersionWithExt("tst-ext-aiv-parent", 2, pgInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("want ErrInconsistent, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AISystemBinding
// ---------------------------------------------------------------------------

func newPGAIBindingRepoForExt(t *testing.T) (*AISystemBindingRepo, *AISystemRepo, context.Context) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_system_bindings WHERE id LIKE 'tst-ext-bind-%'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_systems WHERE id LIKE 'tst-ext-bind-%'`)
		db.Close()
	})
	sysRepo, _ := NewAISystemRepo(db)
	bRepo, _ := NewAISystemBindingRepo(db)
	if err := sysRepo.Create(context.Background(), makePGAISystemWithExt("tst-ext-bind-parent", nil)); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	return bRepo, sysRepo, context.Background()
}

func makePGBindingWithExt(id string, ref *externalref.ExternalRef) *aisystem.AISystemBinding {
	return &aisystem.AISystemBinding{
		ID: id, AISystemID: "tst-ext-bind-parent", SurfaceID: "surf-x",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
		ExternalRef: ref,
	}
}

func TestPGAIBindingRepo_ExternalRef_RoundTrip(t *testing.T) {
	b, _, ctx := newPGAIBindingRepoForExt(t)
	if err := b.Create(ctx, makePGBindingWithExt("tst-ext-bind-rt", pgExtRefFixture())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := b.GetByID(ctx, "tst-ext-bind-rt")
	assertExtRefRoundTrip(t, got.ExternalRef)
}

func TestPGAIBindingRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	b, _, ctx := newPGAIBindingRepoForExt(t)
	err := b.Create(ctx, makePGBindingWithExt("tst-ext-bind-bad", pgInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("want ErrInconsistent, got %v", err)
	}
}
