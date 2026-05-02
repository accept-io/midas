package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
)

func newTestAISystem(id string, now time.Time) *aisystem.AISystem {
	return &aisystem.AISystem{
		ID:        id,
		Name:      id + "-name",
		Status:    aisystem.AISystemStatusActive,
		Origin:    aisystem.AISystemOriginManual,
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestAISystemRepo_Create_RoundTrip(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	sys := newTestAISystem("ai-1", now)
	if err := r.Create(ctx, sys); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByID(ctx, "ai-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != "ai-1" || got.Name != "ai-1-name" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestAISystemRepo_Create_DuplicateID(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	if err := r.Create(ctx, newTestAISystem("dup", now)); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	err := r.Create(ctx, newTestAISystem("dup", now))
	if !errors.Is(err, aisystem.ErrAISystemAlreadyExists) {
		t.Errorf("want ErrAISystemAlreadyExists, got %v", err)
	}
}

func TestAISystemRepo_Create_RejectsInvalidStatus(t *testing.T) {
	r := NewAISystemRepo()
	now := time.Now().UTC()
	sys := newTestAISystem("bad-status", now)
	sys.Status = "frozen"
	err := r.Create(context.Background(), sys)
	if !errors.Is(err, aisystem.ErrInvalidStatus) {
		t.Errorf("want ErrInvalidStatus, got %v", err)
	}
}

func TestAISystemRepo_Create_RejectsInvalidOrigin(t *testing.T) {
	r := NewAISystemRepo()
	now := time.Now().UTC()
	sys := newTestAISystem("bad-origin", now)
	sys.Origin = "imported"
	err := r.Create(context.Background(), sys)
	if !errors.Is(err, aisystem.ErrInvalidOrigin) {
		t.Errorf("want ErrInvalidOrigin, got %v", err)
	}
}

func TestAISystemRepo_Create_RejectsSelfReplace(t *testing.T) {
	r := NewAISystemRepo()
	now := time.Now().UTC()
	sys := newTestAISystem("loop", now)
	sys.Replaces = "loop"
	err := r.Create(context.Background(), sys)
	if !errors.Is(err, aisystem.ErrSelfReplace) {
		t.Errorf("want ErrSelfReplace, got %v", err)
	}
}

func TestAISystemRepo_GetByID_NotFound(t *testing.T) {
	r := NewAISystemRepo()
	_, err := r.GetByID(context.Background(), "nope")
	if !errors.Is(err, aisystem.ErrAISystemNotFound) {
		t.Errorf("want ErrAISystemNotFound, got %v", err)
	}
}

func TestAISystemRepo_Exists(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestAISystem("yes", now)); err != nil {
		t.Fatal(err)
	}
	ok, err := r.Exists(ctx, "yes")
	if err != nil || !ok {
		t.Errorf("Exists(yes) = (%v, %v); want (true, nil)", ok, err)
	}
	ok, err = r.Exists(ctx, "no")
	if err != nil || ok {
		t.Errorf("Exists(no) = (%v, %v); want (false, nil)", ok, err)
	}
}

func TestAISystemRepo_List_OrderedByCreatedAtDesc(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i, id := range []string{"old", "mid", "new"} {
		sys := newTestAISystem(id, base.Add(time.Duration(i)*time.Hour))
		if err := r.Create(ctx, sys); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	got, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 || got[0].ID != "new" || got[2].ID != "old" {
		t.Errorf("List ordering off; got %v", aiSystemIDsOf(got))
	}
}

func aiSystemIDsOf(systems []*aisystem.AISystem) []string {
	out := make([]string, len(systems))
	for i, s := range systems {
		out[i] = s.ID
	}
	return out
}

func TestAISystemRepo_Update_RoundTrip(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestAISystem("upd", now)); err != nil {
		t.Fatal(err)
	}
	updated := newTestAISystem("upd", now)
	updated.Description = "changed"
	updated.Status = aisystem.AISystemStatusDeprecated
	if err := r.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := r.GetByID(ctx, "upd")
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "changed" || got.Status != aisystem.AISystemStatusDeprecated {
		t.Errorf("Update did not apply: %+v", got)
	}
}

func TestAISystemRepo_Update_NotFound(t *testing.T) {
	r := NewAISystemRepo()
	err := r.Update(context.Background(), newTestAISystem("ghost", time.Now()))
	if !errors.Is(err, aisystem.ErrAISystemNotFound) {
		t.Errorf("want ErrAISystemNotFound, got %v", err)
	}
}

func TestAISystemRepo_DefensiveCopy(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	sys := newTestAISystem("defensive", now)
	if err := r.Create(ctx, sys); err != nil {
		t.Fatal(err)
	}
	sys.Name = "mutated-after-create"

	got, err := r.GetByID(ctx, "defensive")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name == "mutated-after-create" {
		t.Error("memory repo did not defensively copy on Create — caller mutation leaked into stored state")
	}

	got.Name = "mutated-after-get"
	again, _ := r.GetByID(ctx, "defensive")
	if again.Name == "mutated-after-get" {
		t.Error("memory repo did not defensively copy on GetByID — caller mutation leaked into stored state")
	}
}
