package postgres

// Postgres-backed tests for the AI System Registration substrate.
// Gated on DATABASE_URL via openTestDB; the suite skips when not set.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
)

func newAISystemRepo(t *testing.T) (*AISystemRepo, context.Context, *sql.DB) {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() {
		// Clean rows in FK order: bindings → versions → systems.
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_system_bindings WHERE id LIKE 'tst-ai-%'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_system_versions WHERE ai_system_id LIKE 'tst-ai-%'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_systems WHERE id LIKE 'tst-ai-%'`)
		db.Close()
	})
	r, err := NewAISystemRepo(db)
	if err != nil {
		t.Fatalf("NewAISystemRepo: %v", err)
	}
	return r, context.Background(), db
}

func newTestPGAISystem(id string, now time.Time) *aisystem.AISystem {
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

func TestPGAISystem_Create_RoundTrip(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	if err := r.Create(ctx, newTestPGAISystem("tst-ai-rt", now)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByID(ctx, "tst-ai-rt")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "tst-ai-rt-name" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestPGAISystem_Create_DuplicateID(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestPGAISystem("tst-ai-dup", now)); err != nil {
		t.Fatal(err)
	}
	err := r.Create(ctx, newTestPGAISystem("tst-ai-dup", now))
	if !errors.Is(err, aisystem.ErrAISystemAlreadyExists) {
		t.Errorf("want ErrAISystemAlreadyExists, got %v", err)
	}
}

func TestPGAISystem_Create_InvalidStatus_RejectedByCheck(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	now := time.Now().UTC()
	sys := newTestPGAISystem("tst-ai-bad-status", now)
	sys.Status = "frozen"
	err := r.Create(ctx, sys)
	if !errors.Is(err, aisystem.ErrInvalidStatus) {
		t.Errorf("want ErrInvalidStatus, got %v", err)
	}
}

func TestPGAISystem_Create_InvalidOrigin_RejectedByCheck(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	now := time.Now().UTC()
	sys := newTestPGAISystem("tst-ai-bad-origin", now)
	sys.Origin = "imported"
	err := r.Create(ctx, sys)
	if !errors.Is(err, aisystem.ErrInvalidOrigin) {
		t.Errorf("want ErrInvalidOrigin, got %v", err)
	}
}

func TestPGAISystem_Create_SelfReplace_RejectedByCheck(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	now := time.Now().UTC()
	sys := newTestPGAISystem("tst-ai-self", now)
	sys.Replaces = "tst-ai-self"
	err := r.Create(ctx, sys)
	if !errors.Is(err, aisystem.ErrSelfReplace) {
		t.Errorf("want ErrSelfReplace, got %v", err)
	}
}

func TestPGAISystem_GetByID_NotFound(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	_, err := r.GetByID(ctx, "tst-ai-absent")
	if !errors.Is(err, aisystem.ErrAISystemNotFound) {
		t.Errorf("want ErrAISystemNotFound, got %v", err)
	}
}

func TestPGAISystem_Update_RoundTrip(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestPGAISystem("tst-ai-upd", now)); err != nil {
		t.Fatal(err)
	}
	upd := newTestPGAISystem("tst-ai-upd", now)
	upd.Description = "changed"
	upd.Status = aisystem.AISystemStatusDeprecated
	if err := r.Update(ctx, upd); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.GetByID(ctx, "tst-ai-upd")
	if got.Description != "changed" || got.Status != aisystem.AISystemStatusDeprecated {
		t.Errorf("Update did not apply: %+v", got)
	}
}

func TestPGAISystem_Update_NotFound(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	err := r.Update(ctx, newTestPGAISystem("tst-ai-ghost", time.Now()))
	if !errors.Is(err, aisystem.ErrAISystemNotFound) {
		t.Errorf("want ErrAISystemNotFound, got %v", err)
	}
}

func TestPGAISystem_List_OrderedByCreatedAtDesc(t *testing.T) {
	r, ctx, _ := newAISystemRepo(t)
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i, id := range []string{"tst-ai-old", "tst-ai-mid", "tst-ai-new"} {
		sys := newTestPGAISystem(id, base.Add(time.Duration(i)*time.Hour))
		if err := r.Create(ctx, sys); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Filter to the test-prefixed rows so leftover state from other tests
	// in the same DB doesn't break this assertion.
	var ours []string
	for _, s := range got {
		if strings.HasPrefix(s.ID, "tst-ai-") {
			ours = append(ours, s.ID)
		}
	}
	if len(ours) < 3 || ours[0] != "tst-ai-new" {
		t.Errorf("unexpected order: got %v", ours)
	}
}

func TestPGAISystem_FKReplaces_SetNullOnDelete(t *testing.T) {
	r, ctx, db := newAISystemRepo(t)
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestPGAISystem("tst-ai-old-x", now)); err != nil {
		t.Fatal(err)
	}
	successor := newTestPGAISystem("tst-ai-new-x", now)
	successor.Replaces = "tst-ai-old-x"
	if err := r.Create(ctx, successor); err != nil {
		t.Fatal(err)
	}

	// Delete the predecessor; replaces should set NULL on the successor.
	if _, err := db.ExecContext(ctx, `DELETE FROM ai_systems WHERE id = $1`, "tst-ai-old-x"); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(ctx, "tst-ai-new-x")
	if err != nil {
		t.Fatal(err)
	}
	if got.Replaces != "" {
		t.Errorf("replaces should be NULL after predecessor delete; got %q", got.Replaces)
	}
}
