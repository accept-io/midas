package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
)

func newAIBindingRepo(t *testing.T) (*AISystemBindingRepo, *AISystemRepo, *AISystemVersionRepo, context.Context) {
	t.Helper()
	r, ctx, db := newAISystemRepo(t)
	vRepo, err := NewAISystemVersionRepo(db)
	if err != nil {
		t.Fatalf("NewAISystemVersionRepo: %v", err)
	}
	bRepo, err := NewAISystemBindingRepo(db)
	if err != nil {
		t.Fatalf("NewAISystemBindingRepo: %v", err)
	}
	return bRepo, r, vRepo, ctx
}

func newTestPGBinding(id, sysID string, now time.Time) *aisystem.AISystemBinding {
	return &aisystem.AISystemBinding{
		ID:                id,
		AISystemID:        sysID,
		BusinessServiceID: "bs-x", // not seeded — overridden in tests that exercise FKs
		CreatedAt:         now,
	}
}

func TestPGAIBinding_Create_RoundTrip(t *testing.T) {
	b, sys, _, ctx := newAIBindingRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-bind-rt")

	now := time.Now().UTC().Truncate(time.Millisecond)
	in := &aisystem.AISystemBinding{
		ID:         "tst-ai-bind-rt-1",
		AISystemID: "tst-ai-bind-rt",
		ProcessID:  "", // tested elsewhere; here only surface_id is set (no FK)
		SurfaceID:  "surf-x",
		Role:       "evaluator",
		CreatedAt:  now,
	}
	if err := b.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := b.GetByID(ctx, "tst-ai-bind-rt-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SurfaceID != "surf-x" || got.Role != "evaluator" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestPGAIBinding_Create_DuplicateID(t *testing.T) {
	b, sys, _, ctx := newAIBindingRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-bind-dup")
	now := time.Now().UTC()
	in := newTestPGBinding("tst-ai-bind-dup-1", "tst-ai-bind-dup", now)
	in.BusinessServiceID = ""
	in.SurfaceID = "surf-y"
	if err := b.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	err := b.Create(ctx, in)
	if !errors.Is(err, aisystem.ErrAISystemBindingAlreadyExists) {
		t.Errorf("want ErrAISystemBindingAlreadyExists, got %v", err)
	}
}

func TestPGAIBinding_Create_NoContextReference_RejectedByCheck(t *testing.T) {
	b, sys, _, ctx := newAIBindingRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-bind-noctx")
	now := time.Now().UTC()
	in := &aisystem.AISystemBinding{
		ID:         "tst-ai-bind-noctx-1",
		AISystemID: "tst-ai-bind-noctx",
		CreatedAt:  now,
	}
	err := b.Create(ctx, in)
	if !errors.Is(err, aisystem.ErrBindingMissingContext) {
		t.Errorf("want ErrBindingMissingContext, got %v", err)
	}
}

func TestPGAIBinding_Create_MissingAISystem_FKViolation(t *testing.T) {
	b, _, _, ctx := newAIBindingRepo(t)
	now := time.Now().UTC()
	in := &aisystem.AISystemBinding{
		ID:         "tst-ai-bind-fk-1",
		AISystemID: "tst-ai-bind-ghost",
		SurfaceID:  "surf-x",
		CreatedAt:  now,
	}
	err := b.Create(ctx, in)
	if err == nil {
		t.Fatal("expected FK violation, got nil")
	}
	if !strings.Contains(err.Error(), "foreign key") {
		t.Errorf("error should mention foreign key violation; got %q", err.Error())
	}
}

func TestPGAIBinding_Create_PinnedVersionFK(t *testing.T) {
	b, sys, v, ctx := newAIBindingRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-bind-ver")
	now := time.Now().UTC()
	if err := v.Create(ctx, newTestPGAIVersion("tst-ai-bind-ver", 1, now)); err != nil {
		t.Fatal(err)
	}

	pin := 1
	in := &aisystem.AISystemBinding{
		ID:              "tst-ai-bind-ver-ok",
		AISystemID:      "tst-ai-bind-ver",
		AISystemVersion: &pin,
		SurfaceID:       "surf-x",
		CreatedAt:       now,
	}
	if err := b.Create(ctx, in); err != nil {
		t.Fatalf("Create with valid version: %v", err)
	}

	bad := 99
	in2 := &aisystem.AISystemBinding{
		ID:              "tst-ai-bind-ver-bad",
		AISystemID:      "tst-ai-bind-ver",
		AISystemVersion: &bad,
		SurfaceID:       "surf-x",
		CreatedAt:       now,
	}
	err := b.Create(ctx, in2)
	if err == nil {
		t.Fatal("expected FK violation for missing version, got nil")
	}
}

func TestPGAIBinding_GetByID_NotFound(t *testing.T) {
	b, _, _, ctx := newAIBindingRepo(t)
	_, err := b.GetByID(ctx, "tst-ai-bind-absent")
	if !errors.Is(err, aisystem.ErrAISystemBindingNotFound) {
		t.Errorf("want ErrAISystemBindingNotFound, got %v", err)
	}
}

func TestPGAIBinding_ListByAISystem_OrderedDesc(t *testing.T) {
	b, sys, _, ctx := newAIBindingRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-bind-list")
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i, id := range []string{"tst-ai-bind-list-old", "tst-ai-bind-list-mid", "tst-ai-bind-list-new"} {
		in := &aisystem.AISystemBinding{
			ID:         id,
			AISystemID: "tst-ai-bind-list",
			SurfaceID:  "surf-x",
			CreatedAt:  base.Add(time.Duration(i) * time.Hour),
		}
		if err := b.Create(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	got, err := b.ListByAISystem(ctx, "tst-ai-bind-list")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "tst-ai-bind-list-new" || got[2].ID != "tst-ai-bind-list-old" {
		t.Errorf("ListByAISystem ordering off; got %v", pgBindingIDs(got))
	}
}

func TestPGAIBinding_Delete(t *testing.T) {
	b, sys, _, ctx := newAIBindingRepo(t)
	seedSystem(t, sys, ctx, "tst-ai-bind-del")
	now := time.Now().UTC()
	in := &aisystem.AISystemBinding{
		ID:         "tst-ai-bind-del-1",
		AISystemID: "tst-ai-bind-del",
		SurfaceID:  "surf-x",
		CreatedAt:  now,
	}
	if err := b.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(ctx, "tst-ai-bind-del-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := b.Delete(ctx, "tst-ai-bind-del-1"); !errors.Is(err, aisystem.ErrAISystemBindingNotFound) {
		t.Errorf("Delete (idempotent): want ErrAISystemBindingNotFound, got %v", err)
	}
}

func pgBindingIDs(bs []*aisystem.AISystemBinding) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.ID
	}
	return out
}
