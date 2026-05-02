package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
)

func newTestAIVersion(systemID string, version int, now time.Time) *aisystem.AISystemVersion {
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

func TestAIVersionRepo_Create_RoundTrip(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if err := r.Create(ctx, newTestAIVersion("ai-1", 1, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByIDAndVersion(ctx, "ai-1", 1)
	if err != nil {
		t.Fatalf("GetByIDAndVersion: %v", err)
	}
	if got.AISystemID != "ai-1" || got.Version != 1 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestAIVersionRepo_Create_DuplicateTuple(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestAIVersion("ai-1", 1, now)); err != nil {
		t.Fatal(err)
	}
	err := r.Create(ctx, newTestAIVersion("ai-1", 1, now))
	if !errors.Is(err, aisystem.ErrAISystemVersionAlreadyExists) {
		t.Errorf("want ErrAISystemVersionAlreadyExists, got %v", err)
	}
}

func TestAIVersionRepo_Create_RejectsInvalidStatus(t *testing.T) {
	r := NewAISystemVersionRepo()
	now := time.Now().UTC()
	v := newTestAIVersion("ai-1", 1, now)
	v.Status = "frozen"
	err := r.Create(context.Background(), v)
	if !errors.Is(err, aisystem.ErrInvalidStatus) {
		t.Errorf("want ErrInvalidStatus, got %v", err)
	}
}

func TestAIVersionRepo_Create_RejectsZeroVersion(t *testing.T) {
	r := NewAISystemVersionRepo()
	now := time.Now().UTC()
	v := newTestAIVersion("ai-1", 0, now)
	err := r.Create(context.Background(), v)
	if !errors.Is(err, aisystem.ErrInvalidVersion) {
		t.Errorf("want ErrInvalidVersion, got %v", err)
	}
}

func TestAIVersionRepo_Create_RejectsBadEffectiveRange(t *testing.T) {
	r := NewAISystemVersionRepo()
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	v := newTestAIVersion("ai-1", 1, now)
	v.EffectiveUntil = &earlier
	err := r.Create(context.Background(), v)
	if !errors.Is(err, aisystem.ErrInvalidEffectiveRange) {
		t.Errorf("want ErrInvalidEffectiveRange, got %v", err)
	}
}

func TestAIVersionRepo_Create_RejectsMissingSystemFK(t *testing.T) {
	sysRepo := NewAISystemRepo()
	r := NewAISystemVersionRepo()
	r.aiSystems = sysRepo

	now := time.Now().UTC()
	err := r.Create(context.Background(), newTestAIVersion("ghost", 1, now))
	if !errors.Is(err, aisystem.ErrAISystemNotFound) {
		t.Errorf("want ErrAISystemNotFound, got %v", err)
	}
}

func TestAIVersionRepo_GetByIDAndVersion_NotFound(t *testing.T) {
	r := NewAISystemVersionRepo()
	_, err := r.GetByIDAndVersion(context.Background(), "absent", 1)
	if !errors.Is(err, aisystem.ErrAISystemVersionNotFound) {
		t.Errorf("want ErrAISystemVersionNotFound, got %v", err)
	}
}

func TestAIVersionRepo_ListBySystem_OrderedDesc(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, v := range []int{1, 2, 3} {
		if err := r.Create(ctx, newTestAIVersion("ai-1", v, now)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.ListBySystem(ctx, "ai-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Version != 3 || got[2].Version != 1 {
		t.Errorf("ListBySystem ordering off; got %v", versionsOf(got))
	}
}

func versionsOf(vs []*aisystem.AISystemVersion) []int {
	out := make([]int, len(vs))
	for i, v := range vs {
		out[i] = v.Version
	}
	return out
}

func TestAIVersionRepo_GetActiveBySystem_PicksHighestActive(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	v1 := newTestAIVersion("ai-1", 1, now)
	v1.Status = aisystem.AISystemVersionStatusActive
	v2 := newTestAIVersion("ai-1", 2, now)
	v2.Status = aisystem.AISystemVersionStatusReview
	v3 := newTestAIVersion("ai-1", 3, now)
	v3.Status = aisystem.AISystemVersionStatusActive
	v4 := newTestAIVersion("ai-1", 4, now)
	v4.Status = aisystem.AISystemVersionStatusDeprecated

	for _, v := range []*aisystem.AISystemVersion{v1, v2, v3, v4} {
		if err := r.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	got, err := r.GetActiveBySystem(ctx, "ai-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Version != 3 {
		t.Errorf("GetActiveBySystem: want v3, got %+v", got)
	}
}

func TestAIVersionRepo_GetActiveBySystem_NoneActive(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	v := newTestAIVersion("ai-1", 1, now)
	v.Status = aisystem.AISystemVersionStatusReview
	if err := r.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetActiveBySystem(ctx, "ai-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("want nil, got %+v", got)
	}
}

func TestAIVersionRepo_Update_RoundTrip(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestAIVersion("ai-1", 1, now)); err != nil {
		t.Fatal(err)
	}
	upd := newTestAIVersion("ai-1", 1, now)
	upd.Status = aisystem.AISystemVersionStatusDeprecated
	upd.ReleaseLabel = "2026.04-r1"
	if err := r.Update(ctx, upd); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.GetByIDAndVersion(ctx, "ai-1", 1)
	if got.Status != aisystem.AISystemVersionStatusDeprecated || got.ReleaseLabel != "2026.04-r1" {
		t.Errorf("Update did not apply: %+v", got)
	}
}

func TestAIVersionRepo_DefensiveCopy(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	v := newTestAIVersion("ai-1", 1, now)
	v.ComplianceFrameworks = []string{"iso-42001"}
	if err := r.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	v.ComplianceFrameworks[0] = "MUTATED"

	got, _ := r.GetByIDAndVersion(ctx, "ai-1", 1)
	if got.ComplianceFrameworks[0] == "MUTATED" {
		t.Error("memory repo did not defensively copy ComplianceFrameworks slice")
	}
}
