package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/escalation"
)

func newPGEscTarget(id string, version int, status escalation.Status, eff time.Time) *escalation.EscalationTarget {
	return &escalation.EscalationTarget{
		ID:             id,
		Version:        version,
		Name:           "target " + id,
		Description:    "test",
		Kind:           escalation.KindRole,
		Handle:         "governance.approver",
		Status:         status,
		EffectiveDate:  eff,
		BusinessOwner:  "biz@example.com",
		TechnicalOwner: "tech@example.com",
		CreatedAt:      eff,
		UpdatedAt:      eff,
		CreatedBy:      "admin",
	}
}

func TestPostgresEscalationTargetRepo_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM escalation_targets WHERE id LIKE 'pg-et-%'`)
	})

	repo, err := NewEscalationTargetRepo(db)
	if err != nil {
		t.Fatalf("NewEscalationTargetRepo: %v", err)
	}

	if err := repo.Create(ctx, newPGEscTarget("pg-et-1", 1, escalation.StatusReview, now)); err != nil {
		t.Fatalf("Create v1: %v", err)
	}
	approved := now.Add(time.Hour)
	v2 := newPGEscTarget("pg-et-1", 2, escalation.StatusActive, now)
	v2.ApprovedBy = "approver-1"
	v2.ApprovedAt = &approved
	if err := repo.Create(ctx, v2); err != nil {
		t.Fatalf("Create v2: %v", err)
	}

	got, err := repo.FindByID(ctx, "pg-et-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Fatalf("FindByID must return latest version; got %+v", got)
	}
	if got.Status != escalation.StatusActive {
		t.Errorf("Status: want active, got %s", got.Status)
	}
	if got.ApprovedBy != "approver-1" || got.ApprovedAt == nil || !got.ApprovedAt.Equal(approved) {
		t.Errorf("Approval fields not round-tripped: got %+v / %v", got.ApprovedBy, got.ApprovedAt)
	}
}

func TestPostgresEscalationTargetRepo_FindByIDAndVersion_AndListVersions(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM escalation_targets WHERE id LIKE 'pg-et-vers-%'`)
	})

	repo, _ := NewEscalationTargetRepo(db)
	for v := 1; v <= 3; v++ {
		_ = repo.Create(ctx, newPGEscTarget("pg-et-vers-1", v, escalation.StatusActive, now))
	}

	got, err := repo.FindByIDAndVersion(ctx, "pg-et-vers-1", 2)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("expected v2; got %+v", got)
	}

	missing, _ := repo.FindByIDAndVersion(ctx, "pg-et-vers-1", 99)
	if missing != nil {
		t.Errorf("missing version: want nil, got %+v", missing)
	}

	versions, err := repo.ListVersions(ctx, "pg-et-vers-1")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("ListVersions length: want 3, got %d", len(versions))
	}
	for i, want := range []int{3, 2, 1} {
		if versions[i].Version != want {
			t.Errorf("ListVersions[%d].Version: want %d, got %d", i, want, versions[i].Version)
		}
	}
}

func TestPostgresEscalationTargetRepo_FindActiveAt(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Millisecond).Add(-2 * time.Hour)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM escalation_targets WHERE id LIKE 'pg-et-active-%'`)
	})

	repo, _ := NewEscalationTargetRepo(db)

	// v1 active baseline
	_ = repo.Create(ctx, newPGEscTarget("pg-et-active-1", 1, escalation.StatusActive, t0))
	// v2 active, expires before lookup
	v2 := newPGEscTarget("pg-et-active-1", 2, escalation.StatusActive, t0.Add(10*time.Minute))
	expiresAt := t0.Add(20 * time.Minute)
	v2.EffectiveUntil = &expiresAt
	_ = repo.Create(ctx, v2)
	// v3 deprecated, must be ignored
	v3 := newPGEscTarget("pg-et-active-1", 3, escalation.StatusDeprecated, t0.Add(30*time.Minute))
	_ = repo.Create(ctx, v3)

	got, err := repo.FindActiveAt(ctx, "pg-et-active-1", t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Fatalf("expected v1 (expired v2 + deprecated v3 ignored); got %+v", got)
	}

	if g, _ := repo.FindActiveAt(ctx, "pg-et-active-1", t0.Add(-time.Hour)); g != nil {
		t.Errorf("future-dated lookup should not resolve; got %+v", g)
	}
}

func TestPostgresEscalationTargetRepo_List(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM escalation_targets WHERE id LIKE 'pg-et-list-%'`)
	})

	repo, _ := NewEscalationTargetRepo(db)
	for _, id := range []string{"pg-et-list-c", "pg-et-list-a", "pg-et-list-b"} {
		_ = repo.Create(ctx, newPGEscTarget(id, 1, escalation.StatusActive, now))
		_ = repo.Create(ctx, newPGEscTarget(id, 2, escalation.StatusActive, now.Add(time.Hour)))
	}

	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Filter to only ids we created (in case the test database has
	// other rows from prior test runs or fixtures).
	var ours []*escalation.EscalationTarget
	for _, t := range got {
		if len(t.ID) >= len("pg-et-list-") && t.ID[:len("pg-et-list-")] == "pg-et-list-" {
			ours = append(ours, t)
		}
	}
	if len(ours) != 3 {
		t.Fatalf("List of our ids: want 3, got %d (%+v)", len(ours), ours)
	}
	for i, want := range []string{"pg-et-list-a", "pg-et-list-b", "pg-et-list-c"} {
		if ours[i].ID != want {
			t.Errorf("List[%d].ID: want %q, got %q", i, want, ours[i].ID)
		}
		if ours[i].Version != 2 {
			t.Errorf("List[%d].Version: want latest (2), got %d", i, ours[i].Version)
		}
	}
}

func TestPostgresEscalationTargetRepo_Update_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM escalation_targets WHERE id LIKE 'pg-et-upd-%'`)
	})

	repo, _ := NewEscalationTargetRepo(db)
	tgt := newPGEscTarget("pg-et-upd-1", 1, escalation.StatusReview, now)
	if err := repo.Create(ctx, tgt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	approved := now.Add(2 * time.Hour)
	tgt.Status = escalation.StatusActive
	tgt.Name = "promoted"
	tgt.ApprovedBy = "approver-1"
	tgt.ApprovedAt = &approved
	tgt.UpdatedAt = approved
	if err := repo.Update(ctx, tgt); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.FindByIDAndVersion(ctx, "pg-et-upd-1", 1)
	if got == nil {
		t.Fatal("FindByIDAndVersion returned nil")
	}
	if got.Status != escalation.StatusActive || got.Name != "promoted" {
		t.Errorf("Update fields: %+v", got)
	}
	if got.ApprovedBy != "approver-1" || got.ApprovedAt == nil || !got.ApprovedAt.Equal(approved) {
		t.Errorf("Approval round-trip: %+v / %v", got.ApprovedBy, got.ApprovedAt)
	}
}

func TestPostgresEscalationTargetRepo_Update_MissingRowReturnsError(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, _ := NewEscalationTargetRepo(db)
	tgt := newPGEscTarget("pg-et-not-exist", 1, escalation.StatusActive,
		time.Now().UTC().Truncate(time.Millisecond))
	err := repo.Update(ctx, tgt)
	if err == nil {
		t.Fatal("Update on missing row should return error")
	}
}

func TestPostgresEscalationTargetRepo_RejectsInvalidKind(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM escalation_targets WHERE id LIKE 'pg-et-bad-%'`)
	})
	repo, _ := NewEscalationTargetRepo(db)
	tgt := newPGEscTarget("pg-et-bad-kind", 1, escalation.StatusActive, now)
	tgt.Kind = "team" // not canonical
	if err := repo.Create(ctx, tgt); err == nil {
		t.Errorf("schema CHECK should reject kind=%q", tgt.Kind)
	}
}

func TestPostgresEscalationTargetRepo_RejectsInvalidStatus(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM escalation_targets WHERE id LIKE 'pg-et-bad-%'`)
	})
	repo, _ := NewEscalationTargetRepo(db)
	tgt := newPGEscTarget("pg-et-bad-status", 1, "approved", now)
	if err := repo.Create(ctx, tgt); err == nil {
		t.Errorf("schema CHECK should reject status=%q", tgt.Status)
	}
}
