package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
)

func newAIVersionRepo(t *testing.T) (*AISystemVersionRepo, *AISystemRepo, context.Context, *sql.DB) {
	t.Helper()
	r, ctx, db := newAISystemRepo(t)
	vRepo, err := NewAISystemVersionRepo(db)
	if err != nil {
		t.Fatalf("NewAISystemVersionRepo: %v", err)
	}
	return vRepo, r, ctx, db
}

func seedSystem(t *testing.T, r *AISystemRepo, ctx context.Context, id string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := r.Create(ctx, &aisystem.AISystem{
		ID: id, Name: id, Status: aisystem.AISystemStatusActive,
		Origin: aisystem.AISystemOriginManual, Managed: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seedSystem(%s): %v", id, err)
	}
}

func newTestPGAIVersion(systemID string, version int, now time.Time) *aisystem.AISystemVersion {
	return &aisystem.AISystemVersion{
		AISystemID:           systemID,
		Version:              version,
		Status:               aisystem.AISystemVersionStatusActive,
		EffectiveFrom:        now,
		ComplianceFrameworks: []string{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func TestPGAIVersion_Create_RoundTrip(t *testing.T) {
	v, sys, ctx, _ := newAIVersionRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-vrt")

	now := time.Now().UTC().Truncate(time.Millisecond)
	in := newTestPGAIVersion("tst-ai-vrt", 1, now)
	in.ComplianceFrameworks = []string{"iso-42001", "soc2"}
	in.ReleaseLabel = "2026.04-r1"
	if err := v.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := v.GetByIDAndVersion(ctx, "tst-ai-vrt", 1)
	if err != nil {
		t.Fatalf("GetByIDAndVersion: %v", err)
	}
	if got.ReleaseLabel != "2026.04-r1" {
		t.Errorf("release_label round-trip failed: got %q", got.ReleaseLabel)
	}
	if len(got.ComplianceFrameworks) != 2 || got.ComplianceFrameworks[0] != "iso-42001" {
		t.Errorf("compliance_frameworks round-trip failed: got %v", got.ComplianceFrameworks)
	}
}

func TestPGAIVersion_Create_DuplicateTuple(t *testing.T) {
	v, sys, ctx, _ := newAIVersionRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-vdup")
	now := time.Now().UTC()
	if err := v.Create(ctx, newTestPGAIVersion("tst-ai-vdup", 1, now)); err != nil {
		t.Fatal(err)
	}
	err := v.Create(ctx, newTestPGAIVersion("tst-ai-vdup", 1, now))
	if !errors.Is(err, aisystem.ErrAISystemVersionAlreadyExists) {
		t.Errorf("want ErrAISystemVersionAlreadyExists, got %v", err)
	}
}

func TestPGAIVersion_Create_InvalidStatus_RejectedByCheck(t *testing.T) {
	v, sys, ctx, _ := newAIVersionRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-vinvst")
	now := time.Now().UTC()
	in := newTestPGAIVersion("tst-ai-vinvst", 1, now)
	in.Status = "frozen"
	err := v.Create(ctx, in)
	if !errors.Is(err, aisystem.ErrInvalidStatus) {
		t.Errorf("want ErrInvalidStatus, got %v", err)
	}
}

func TestPGAIVersion_Create_ZeroVersion_RejectedByCheck(t *testing.T) {
	v, sys, ctx, _ := newAIVersionRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-vinvv")
	now := time.Now().UTC()
	in := newTestPGAIVersion("tst-ai-vinvv", 0, now)
	err := v.Create(ctx, in)
	if !errors.Is(err, aisystem.ErrInvalidVersion) {
		t.Errorf("want ErrInvalidVersion, got %v", err)
	}
}

func TestPGAIVersion_Create_BadEffectiveRange_RejectedByCheck(t *testing.T) {
	v, sys, ctx, _ := newAIVersionRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-vbadr")
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	in := newTestPGAIVersion("tst-ai-vbadr", 1, now)
	in.EffectiveUntil = &earlier
	err := v.Create(ctx, in)
	if !errors.Is(err, aisystem.ErrInvalidEffectiveRange) {
		t.Errorf("want ErrInvalidEffectiveRange, got %v", err)
	}
}

func TestPGAIVersion_Create_MissingSystem_FKViolation(t *testing.T) {
	v, _, ctx, _ := newAIVersionRepo(t)
	now := time.Now().UTC()
	err := v.Create(ctx, newTestPGAIVersion("tst-ai-vghost", 1, now))
	if !errors.Is(err, aisystem.ErrAISystemNotFound) {
		t.Errorf("want ErrAISystemNotFound, got %v", err)
	}
}

func TestPGAIVersion_ListBySystem_OrderedDesc(t *testing.T) {
	v, sys, ctx, _ := newAIVersionRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-vlist")
	now := time.Now().UTC()
	for _, ver := range []int{1, 2, 3} {
		if err := v.Create(ctx, newTestPGAIVersion("tst-ai-vlist", ver, now)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := v.ListBySystem(ctx, "tst-ai-vlist")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Version != 3 || got[2].Version != 1 {
		t.Errorf("ListBySystem version order off; got %v", versionsOfPG(got))
	}
}

func TestPGAIVersion_GetActiveBySystem_PicksHighestActive(t *testing.T) {
	v, sys, ctx, _ := newAIVersionRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-vact")
	now := time.Now().UTC()

	tests := []struct {
		ver    int
		status string
	}{
		{1, aisystem.AISystemVersionStatusActive},
		{2, aisystem.AISystemVersionStatusReview},
		{3, aisystem.AISystemVersionStatusActive},
		{4, aisystem.AISystemVersionStatusDeprecated},
	}
	for _, tc := range tests {
		in := newTestPGAIVersion("tst-ai-vact", tc.ver, now)
		in.Status = tc.status
		if err := v.Create(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	got, err := v.GetActiveBySystem(ctx, "tst-ai-vact")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Version != 3 {
		t.Errorf("GetActiveBySystem: want v3, got %+v", got)
	}
}

func TestPGAIVersion_FK_RestrictsParentDelete(t *testing.T) {
	v, sys, ctx, db := newAIVersionRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-vrestrict")
	now := time.Now().UTC()
	if err := v.Create(ctx, newTestPGAIVersion("tst-ai-vrestrict", 1, now)); err != nil {
		t.Fatal(err)
	}
	// ON DELETE RESTRICT: deleting the parent ai_systems row while a
	// version still references it must fail with FK violation.
	_, err := db.ExecContext(ctx, `DELETE FROM ai_systems WHERE id = $1`, "tst-ai-vrestrict")
	if err == nil {
		t.Fatal("expected FK RESTRICT violation, got nil")
	}
	// Clean up so the t.Cleanup ordering works (versions must go first).
	if _, err := db.ExecContext(ctx, `DELETE FROM ai_system_versions WHERE ai_system_id = $1`, "tst-ai-vrestrict"); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func versionsOfPG(vs []*aisystem.AISystemVersion) []int {
	out := make([]int, len(vs))
	for i, v := range vs {
		out[i] = v.Version
	}
	return out
}
