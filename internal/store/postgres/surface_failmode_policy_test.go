package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/surface"
)

// surface_failmode_policy_test.go — round-trip regression for the
// optional Surface.FailModePolicyID column added in D27j-impl-2.
// Confirms the Postgres repo persists, reads, updates, and clears
// the value, and that legacy rows (NULL fail_mode_policy_id) read
// back as the empty string. No FK exists because fail_mode_policies
// is keyed by (id, version) — apply-time validation is the gate.

func TestSurfaceRepo_FailModePolicyID_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, created_at, updated_at)
		VALUES ('cap-fmp-rt-001', 'FMP test', 'active', NOW(), NOW())
		ON CONFLICT (capability_id) DO NOTHING
	`); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO business_services (business_service_id, name, service_type, status, created_at, updated_at)
		VALUES ('bs-fmp-rt-001', 'FMP BS', 'internal', 'active', NOW(), NOW())
		ON CONFLICT (business_service_id) DO NOTHING
	`); err != nil {
		t.Fatalf("insert business service: %v", err)
	}
	const processID = "proc-surf-fmp-rt"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO processes (process_id, business_service_id, name, status, created_at, updated_at)
		VALUES ($1, 'bs-fmp-rt-001', 'FMP Process', 'active', NOW(), NOW())
		ON CONFLICT (process_id) DO NOTHING
	`, processID); err != nil {
		t.Fatalf("insert process: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id LIKE 'surf-fmp-rt-%'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, processID)
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = 'bs-fmp-rt-001'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = 'cap-fmp-rt-001'`)
	})

	repo, err := NewSurfaceRepo(db)
	if err != nil {
		t.Fatalf("NewSurfaceRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	newSurface := func(id, fmpID string) *surface.DecisionSurface {
		return &surface.DecisionSurface{
			ID:               id,
			Version:          1,
			Name:             "FMP Surface",
			Domain:           "default",
			BusinessOwner:    "owner",
			TechnicalOwner:   "tech",
			Status:           surface.SurfaceStatusReview,
			EffectiveFrom:    now,
			CreatedAt:        now,
			UpdatedAt:        now,
			ProcessID:        processID,
			FailModePolicyID: fmpID,
		}
	}

	t.Run("with fail_mode_policy_id round-trips correctly", func(t *testing.T) {
		s := newSurface("surf-fmp-rt-001", "fmp-default")
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.FindByIDVersion(ctx, "surf-fmp-rt-001", 1)
		if err != nil {
			t.Fatalf("FindByIDVersion: %v", err)
		}
		if got == nil {
			t.Fatal("expected surface, got nil")
		}
		if got.FailModePolicyID != "fmp-default" {
			t.Errorf("FailModePolicyID: want %q, got %q", "fmp-default", got.FailModePolicyID)
		}
	})

	t.Run("empty fail_mode_policy_id round-trips as empty string", func(t *testing.T) {
		s := newSurface("surf-fmp-rt-002", "")
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.FindByIDVersion(ctx, "surf-fmp-rt-002", 1)
		if err != nil {
			t.Fatalf("FindByIDVersion: %v", err)
		}
		if got == nil {
			t.Fatal("expected surface, got nil")
		}
		if got.FailModePolicyID != "" {
			t.Errorf("FailModePolicyID: want empty, got %q", got.FailModePolicyID)
		}
	})

	t.Run("Update can set and clear fail_mode_policy_id", func(t *testing.T) {
		s := newSurface("surf-fmp-rt-003", "")
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("Create: %v", err)
		}

		s.FailModePolicyID = "fmp-set-on-update"
		if err := repo.Update(ctx, s); err != nil {
			t.Fatalf("Update set: %v", err)
		}
		got, _ := repo.FindByIDVersion(ctx, "surf-fmp-rt-003", 1)
		if got == nil || got.FailModePolicyID != "fmp-set-on-update" {
			t.Errorf("Update should persist new FailModePolicyID; got %+v", got)
		}

		s.FailModePolicyID = ""
		if err := repo.Update(ctx, s); err != nil {
			t.Fatalf("Update clear: %v", err)
		}
		got, _ = repo.FindByIDVersion(ctx, "surf-fmp-rt-003", 1)
		if got == nil || got.FailModePolicyID != "" {
			t.Errorf("Update should clear FailModePolicyID; got %+v", got)
		}
	})
}
