package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/surface"
)

// TestSurfaceRepo_ProcessID_Persistence exercises the Surface → Process
// invariant at the Postgres repository boundary (Issue #33):
//
//   - Create with a valid ProcessID round-trips the value.
//   - Create with an empty ProcessID is rejected at the application layer
//     before the INSERT is issued.
//   - The DB-level NOT NULL constraint on decision_surfaces.process_id
//     is the backstop if the application check were bypassed.
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

	t.Run("empty process_id is rejected at the application layer", func(t *testing.T) {
		s := newSurface("surf-proc-rt-002", "")

		err := repo.Create(ctx, s)
		if err == nil {
			t.Fatal("expected error for empty ProcessID, got nil")
		}
		if !strings.Contains(err.Error(), "process_id") {
			t.Errorf("error must name process_id, got: %v", err)
		}

		// Nothing was persisted.
		got, err := repo.FindByIDVersion(ctx, "surf-proc-rt-002", 1)
		if err != nil {
			t.Fatalf("FindByIDVersion after rejected Create: %v", err)
		}
		if got != nil {
			t.Errorf("surface must not be persisted when Create rejects, got %+v", got)
		}
	})
}

// TestSurfaceRepo_ProcessID_UpdateRoundTrip verifies that Update round-trips
// a change of ProcessID between two valid values (the expected re-link
// workflow) and rejects clearing ProcessID to empty.
func TestSurfaceRepo_ProcessID_UpdateRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, created_at, updated_at)
		VALUES ('cap-surf-upd-001', 'Test Cap Update', 'active', NOW(), NOW())
		ON CONFLICT (capability_id) DO NOTHING
	`); err != nil {
		t.Fatalf("insert capability: %v", err)
	}

	const (
		processID1 = "loan-update-surf-test-1"
		processID2 = "loan-update-surf-test-2"
	)
	for _, pid := range []string{processID1, processID2} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO processes (process_id, capability_id, name, status, created_at, updated_at)
			VALUES ($1, 'cap-surf-upd-001', 'Loan Update', 'active', NOW(), NOW())
			ON CONFLICT (process_id) DO NOTHING
		`, pid); err != nil {
			t.Fatalf("insert process %q: %v", pid, err)
		}
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id = 'surf-upd-proc-rt-001'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, processID1)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, processID2)
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
		ProcessID:      processID1,
	}

	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("update re-links to a different valid ProcessID", func(t *testing.T) {
		s.ProcessID = processID2
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
		if got.ProcessID != processID2 {
			t.Errorf("ProcessID after update: want %q, got %q", processID2, got.ProcessID)
		}
	})

	t.Run("update rejects clearing ProcessID to empty", func(t *testing.T) {
		s.ProcessID = ""
		err := repo.Update(ctx, s)
		if err == nil {
			t.Fatal("expected error when clearing ProcessID to empty")
		}
		if !strings.Contains(err.Error(), "process_id") {
			t.Errorf("error must name process_id, got: %v", err)
		}
		// The stored value is unchanged.
		got, _ := repo.FindByIDVersion(ctx, "surf-upd-proc-rt-001", 1)
		if got == nil || got.ProcessID != processID2 {
			t.Errorf("ProcessID must be unchanged after rejected Update; got %+v", got)
		}
	})
}

// TestSurfaceRepo_ProcessID_SchemaRejectsNullAtDBLevel proves the database
// NOT NULL constraint on decision_surfaces.process_id is in place. It
// attempts a raw INSERT with NULL process_id and expects the DB to reject
// it with a not-null constraint error. This is the data-integrity backstop
// for callers that bypass the application-layer repository check.
func TestSurfaceRepo_ProcessID_SchemaRejectsNullAtDBLevel(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id = 'surf-null-proc-test'`)
	})

	// Raw INSERT bypassing the Go layer, using NULL process_id.
	_, err := db.ExecContext(ctx, `
		INSERT INTO decision_surfaces
			(id, version, name, domain, business_owner, technical_owner, status,
			 effective_date, created_at, updated_at, process_id)
		VALUES ($1, 1, 'null-test', 'default', '', '', 'draft',
			NOW(), NOW(), NOW(), NULL)
	`, "surf-null-proc-test")

	if err == nil {
		t.Fatal("expected DB-level rejection of NULL process_id, got nil")
	}
	// Postgres not-null violation. Don't assert the exact wording — drivers
	// differ — but require that the message names the column.
	if !strings.Contains(strings.ToLower(err.Error()), "process_id") {
		t.Errorf("expected error to mention process_id, got: %v", err)
	}
}
