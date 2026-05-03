package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/process"
)

// TestProcessRepo_CreateAndGetByID exercises the v1 service-led shape:
// Process belongs to BusinessService (required); no capability_id column.
func TestProcessRepo_CreateAndGetByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	svcID := "tst-svc-proc-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO business_services (business_service_id, name, service_type, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'internal', 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (business_service_id) DO NOTHING`,
		svcID,
	); err != nil {
		t.Fatalf("insert business_service: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = 'tst-proc-001'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, svcID)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	p := &process.Process{
		ID:                "tst-proc-001",
		Name:              "Loan Approval",
		BusinessServiceID: svcID,
		Description:       "Approves loan applications",
		Status:            "active",
		Origin:            "manual",
		Managed:           true,
		Replaces:          "",
		Owner:             "team-lending",
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected process, got nil")
	}

	checks := []struct{ field, want, got string }{
		{"ID", p.ID, got.ID},
		{"Name", p.Name, got.Name},
		{"BusinessServiceID", p.BusinessServiceID, got.BusinessServiceID},
		{"Description", p.Description, got.Description},
		{"Status", p.Status, got.Status},
		{"Origin", p.Origin, got.Origin},
		{"Replaces", p.Replaces, got.Replaces},
		{"Owner", p.Owner, got.Owner},
	}
	for _, c := range checks {
		if c.want != c.got {
			t.Errorf("%s: want %q, got %q", c.field, c.want, c.got)
		}
	}
	if got.Managed != p.Managed {
		t.Errorf("Managed: want %v, got %v", p.Managed, got.Managed)
	}
}

// TestProcessRepo_Create_RejectsEmptyBusinessServiceID asserts the v1
// service-led requirement: business_service_id is mandatory at the repo
// boundary; the empty string is refused without a database round-trip.
func TestProcessRepo_Create_RejectsEmptyBusinessServiceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := repo.Create(context.Background(), &process.Process{
		ID:        "tst-proc-empty-bs",
		Name:      "Bad",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err == nil {
		t.Fatal("expected error for empty business_service_id, got nil")
	}
}

// TestProcessRepo_Update_BusinessServiceID exercises updating the
// business-service link on an existing process.
func TestProcessRepo_Update_BusinessServiceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	svcA := "tst-svc-proc-upd-a"
	svcB := "tst-svc-proc-upd-b"

	for _, id := range []string{svcA, svcB} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO business_services (business_service_id, name, service_type, status, origin, managed, created_at, updated_at)
			 VALUES ($1, $1, 'internal', 'active', 'manual', true, NOW(), NOW())
			 ON CONFLICT (business_service_id) DO NOTHING`,
			id,
		); err != nil {
			t.Fatalf("insert business_service %s: %v", id, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = 'tst-proc-upd-001'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id IN ($1, $2)`, svcA, svcB)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	p := &process.Process{
		ID:                "tst-proc-upd-001",
		Name:              "Original Name",
		BusinessServiceID: svcA,
		Status:            "active",
		Origin:            "manual",
		Managed:           true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := now.Add(time.Second)
	p.Name = "Updated Name"
	p.Status = "deprecated"
	p.Description = "now has a description"
	p.Owner = "team-new"
	p.BusinessServiceID = svcB
	p.UpdatedAt = updated

	if err := repo.Update(ctx, p); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("Name: want %q, got %q", "Updated Name", got.Name)
	}
	if got.BusinessServiceID != svcB {
		t.Errorf("BusinessServiceID: want %q, got %q", svcB, got.BusinessServiceID)
	}
	if got.Status != "deprecated" {
		t.Errorf("Status: want deprecated, got %s", got.Status)
	}
	if !got.UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt: want %v, got %v", updated, got.UpdatedAt)
	}
	// origin and managed are immutable via Update
	if got.Origin != "manual" {
		t.Errorf("Origin: want manual, got %s", got.Origin)
	}
	if !got.Managed {
		t.Error("Managed: want true, got false")
	}
}

// TestProcessRepo_List_ReturnsAllRows verifies List returns every process
// row in the table, including the business_service_id field.
func TestProcessRepo_List_ReturnsAllRows(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	svcID := "tst-svc-proc-lst-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO business_services (business_service_id, name, service_type, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'internal', 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (business_service_id) DO NOTHING`,
		svcID,
	); err != nil {
		t.Fatalf("insert business_service: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id IN ('tst-proc-lst-001', 'tst-proc-lst-002')`)
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, svcID)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	procs := []*process.Process{
		{ID: "tst-proc-lst-001", Name: "P1", BusinessServiceID: svcID, Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "tst-proc-lst-002", Name: "P2", BusinessServiceID: svcID, Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
	}
	for _, p := range procs {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("Create %s: %v", p.ID, err)
		}
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := map[string]string{}
	for _, p := range all {
		if p.ID == "tst-proc-lst-001" || p.ID == "tst-proc-lst-002" {
			found[p.ID] = p.BusinessServiceID
		}
	}
	if len(found) != 2 {
		t.Fatalf("List: want 2 test rows, got %d", len(found))
	}
	for id, bs := range found {
		if bs != svcID {
			t.Errorf("%s BusinessServiceID: want %q, got %q", id, svcID, bs)
		}
	}
}

// TestProcessRepo_ListByBusinessService exercises the new method added
// in Epic 1 PR 4 to support the governance map read service. Pins the
// filter behaviour, the deterministic ordering by process_id, and the
// empty-slice (not nil) contract for unknown business services.
func TestProcessRepo_ListByBusinessService(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	svcA := "tst-svc-proc-lbs-a"
	svcB := "tst-svc-proc-lbs-b"

	for _, sv := range []string{svcA, svcB} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO business_services (business_service_id, name, service_type, status, origin, managed, created_at, updated_at)
			 VALUES ($1, $1, 'internal', 'active', 'manual', true, NOW(), NOW())
			 ON CONFLICT (business_service_id) DO NOTHING`,
			sv,
		); err != nil {
			t.Fatalf("seed BS %s: %v", sv, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id LIKE 'tst-proc-lbs-%'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id IN ($1, $2)`, svcA, svcB)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	// Insert in non-alphabetical order to verify ORDER BY in the query.
	for _, p := range []*process.Process{
		{ID: "tst-proc-lbs-c", Name: "C", BusinessServiceID: svcA, Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "tst-proc-lbs-a", Name: "A", BusinessServiceID: svcA, Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "tst-proc-lbs-other", Name: "Other", BusinessServiceID: svcB, Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
	} {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("Create %s: %v", p.ID, err)
		}
	}

	got, err := repo.ListByBusinessService(ctx, svcA)
	if err != nil {
		t.Fatalf("ListByBusinessService: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 processes for %q, got %d", svcA, len(got))
	}
	if got[0].ID != "tst-proc-lbs-a" || got[1].ID != "tst-proc-lbs-c" {
		t.Errorf("ORDER BY process_id violated; got [%s, %s]", got[0].ID, got[1].ID)
	}
	for _, p := range got {
		if p.BusinessServiceID != svcA {
			t.Errorf("filter leaked: %s has business_service_id=%q", p.ID, p.BusinessServiceID)
		}
	}

	// Unknown BS — non-nil empty slice per contract.
	none, err := repo.ListByBusinessService(ctx, "tst-svc-proc-lbs-nope")
	if err != nil {
		t.Fatalf("ListByBusinessService(unknown): %v", err)
	}
	if none == nil {
		t.Error("empty result should be empty slice, not nil")
	}
	if len(none) != 0 {
		t.Errorf("want 0 processes, got %d", len(none))
	}
}
