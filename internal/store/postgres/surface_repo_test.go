package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/surface"
)

func TestSurfaceRepo_ProcessID_Persistence(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Insert prerequisite capability row (processes FK to capabilities).
	if _, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, created_at, updated_at)
		VALUES ('cap-surf-test-001', 'Test Capability', 'active', NOW(), NOW())
		ON CONFLICT (capability_id) DO NOTHING
	`); err != nil {
		t.Fatalf("insert capability: %v", err)
	}

	// Insert prerequisite process row.
	const processID = "loan-origination-surf-test"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO processes (process_id, capability_id, name, status, created_at, updated_at)
		VALUES ($1, 'cap-surf-test-001', 'Loan Origination', 'active', NOW(), NOW())
		ON CONFLICT (process_id) DO NOTHING
	`, processID); err != nil {
		t.Fatalf("insert process: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id IN ('surf-proc-rt-001', 'surf-proc-rt-002')`)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, processID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = 'cap-surf-test-001'`)
	})

	repo, err := NewSurfaceRepo(db)
	if err != nil {
		t.Fatalf("NewSurfaceRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)

	newSurface := func(id, procID string) *surface.DecisionSurface {
		return &surface.DecisionSurface{
			ID:             id,
			Version:        1,
			Name:           "Test Surface",
			Domain:         "default",
			BusinessOwner:  "owner",
			TechnicalOwner: "tech",
			Status:         surface.SurfaceStatusReview,
			EffectiveFrom:  now,
			CreatedAt:      now,
			UpdatedAt:      now,
			ProcessID:      procID,
		}
	}

	t.Run("with process_id round-trips correctly", func(t *testing.T) {
		s := newSurface("surf-proc-rt-001", processID)

		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := repo.FindByIDVersion(ctx, "surf-proc-rt-001", 1)
		if err != nil {
			t.Fatalf("FindByIDVersion: %v", err)
		}
		if got == nil {
			t.Fatal("expected surface, got nil")
		}
		if got.ProcessID != processID {
			t.Errorf("ProcessID: want %q, got %q", processID, got.ProcessID)
		}
	})

	t.Run("without process_id round-trips as empty", func(t *testing.T) {
		s := newSurface("surf-proc-rt-002", "")

		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := repo.FindByIDVersion(ctx, "surf-proc-rt-002", 1)
		if err != nil {
			t.Fatalf("FindByIDVersion: %v", err)
		}
		if got == nil {
			t.Fatal("expected surface, got nil")
		}
		if got.ProcessID != "" {
			t.Errorf("ProcessID: want empty string, got %q", got.ProcessID)
		}
	})
}

func TestSurfaceRepo_ProcessID_UpdateRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Insert prerequisite rows.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, created_at, updated_at)
		VALUES ('cap-surf-upd-001', 'Test Cap Update', 'active', NOW(), NOW())
		ON CONFLICT (capability_id) DO NOTHING
	`); err != nil {
		t.Fatalf("insert capability: %v", err)
	}

	const processID = "loan-update-surf-test"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO processes (process_id, capability_id, name, status, created_at, updated_at)
		VALUES ($1, 'cap-surf-upd-001', 'Loan Update', 'active', NOW(), NOW())
		ON CONFLICT (process_id) DO NOTHING
	`, processID); err != nil {
		t.Fatalf("insert process: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id = 'surf-upd-proc-rt-001'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, processID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = 'cap-surf-upd-001'`)
	})

	repo, err := NewSurfaceRepo(db)
	if err != nil {
		t.Fatalf("NewSurfaceRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	s := &surface.DecisionSurface{
		ID:             "surf-upd-proc-rt-001",
		Version:        1,
		Name:           "Update Test Surface",
		Domain:         "default",
		BusinessOwner:  "owner",
		TechnicalOwner: "tech",
		Status:         surface.SurfaceStatusReview,
		EffectiveFrom:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ProcessID:      "",
	}

	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Update with process_id set.
	s.ProcessID = processID
	s.UpdatedAt = now.Add(time.Second)

	if err := repo.Update(ctx, s); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.FindByIDVersion(ctx, "surf-upd-proc-rt-001", 1)
	if err != nil {
		t.Fatalf("FindByIDVersion after update: %v", err)
	}
	if got == nil {
		t.Fatal("expected surface after update, got nil")
	}
	if got.ProcessID != processID {
		t.Errorf("ProcessID after update: want %q, got %q", processID, got.ProcessID)
	}
}
