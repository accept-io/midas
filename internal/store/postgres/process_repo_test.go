package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/process"
)

func TestProcessRepo_CreateAndGetByID_WithBusinessServiceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-proc-001"
	svcID := "tst-svc-proc-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
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
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	p := &process.Process{
		ID:                "tst-proc-001",
		Name:              "Loan Approval",
		CapabilityID:      capID,
		BusinessServiceID: svcID,
		Description:       "Approves loan applications",
		Status:            "active",
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
		{"CapabilityID", p.CapabilityID, got.CapabilityID},
		{"BusinessServiceID", p.BusinessServiceID, got.BusinessServiceID},
		{"Description", p.Description, got.Description},
		{"Status", p.Status, got.Status},
		{"Owner", p.Owner, got.Owner},
	}
	for _, c := range checks {
		if c.want != c.got {
			t.Errorf("%s: want %q, got %q", c.field, c.want, c.got)
		}
	}
	if !got.CreatedAt.Equal(p.CreatedAt) {
		t.Errorf("CreatedAt: want %v, got %v", p.CreatedAt, got.CreatedAt)
	}
	if !got.UpdatedAt.Equal(p.UpdatedAt) {
		t.Errorf("UpdatedAt: want %v, got %v", p.UpdatedAt, got.UpdatedAt)
	}
}

func TestProcessRepo_CreateAndGetByID_NullBusinessServiceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-proc-002"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = 'tst-proc-002'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	p := &process.Process{
		ID:                "tst-proc-002",
		Name:              "Credit Check",
		CapabilityID:      capID,
		BusinessServiceID: "",
		Status:            "active",
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
	if got.BusinessServiceID != "" {
		t.Errorf("BusinessServiceID: want empty, got %q", got.BusinessServiceID)
	}
}

func TestProcessRepo_Update_BusinessServiceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-proc-upd-001"
	svcID := "tst-svc-proc-upd-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO business_services (business_service_id, name, service_type, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'internal', 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (business_service_id) DO NOTHING`,
		svcID,
	); err != nil {
		t.Fatalf("insert business_service: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = 'tst-proc-upd-001'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, svcID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	p := &process.Process{
		ID:           "tst-proc-upd-001",
		Name:         "Original Name",
		CapabilityID: capID,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := now.Add(time.Second)
	p.Name = "Updated Name"
	p.Status = "deprecated"
	p.Description = "now has a description"
	p.Owner = "team-new"
	p.BusinessServiceID = svcID
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
	if got.BusinessServiceID != svcID {
		t.Errorf("BusinessServiceID: want %q, got %q", svcID, got.BusinessServiceID)
	}
	if got.Status != "deprecated" {
		t.Errorf("Status: want deprecated, got %s", got.Status)
	}
	if !got.UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt: want %v, got %v", updated, got.UpdatedAt)
	}
	// capability_id is immutable — confirmed unchanged
	if got.CapabilityID != capID {
		t.Errorf("CapabilityID: want %q, got %q", capID, got.CapabilityID)
	}
}

func TestProcessRepo_Update_ClearBusinessServiceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-proc-clr-001"
	svcID := "tst-svc-proc-clr-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO business_services (business_service_id, name, service_type, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'internal', 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (business_service_id) DO NOTHING`,
		svcID,
	); err != nil {
		t.Fatalf("insert business_service: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = 'tst-proc-clr-001'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, svcID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	p := &process.Process{
		ID:                "tst-proc-clr-001",
		Name:              "Clearance Test",
		CapabilityID:      capID,
		BusinessServiceID: svcID,
		Status:            "active",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Clear the business_service_id via Update
	p.BusinessServiceID = ""
	p.UpdatedAt = now.Add(time.Second)
	if err := repo.Update(ctx, p); err != nil {
		t.Fatalf("Update (clear): %v", err)
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID after clear: %v", err)
	}
	if got.BusinessServiceID != "" {
		t.Errorf("BusinessServiceID after clear: want empty, got %q", got.BusinessServiceID)
	}
}

func TestProcessRepo_List_IncludesBusinessServiceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-proc-lst-001"
	svcID := "tst-svc-proc-lst-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
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
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)

	// One process with BusinessServiceID, one without
	procs := []*process.Process{
		{ID: "tst-proc-lst-001", Name: "P1", CapabilityID: capID, BusinessServiceID: svcID, Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "tst-proc-lst-002", Name: "P2", CapabilityID: capID, BusinessServiceID: "", Status: "active", CreatedAt: now, UpdatedAt: now},
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
	if found["tst-proc-lst-001"] != svcID {
		t.Errorf("tst-proc-lst-001 BusinessServiceID: want %q, got %q", svcID, found["tst-proc-lst-001"])
	}
	if found["tst-proc-lst-002"] != "" {
		t.Errorf("tst-proc-lst-002 BusinessServiceID: want empty, got %q", found["tst-proc-lst-002"])
	}
}

func TestProcessRepo_ListByCapabilityID_IncludesBusinessServiceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	capID := "tst-cap-proc-lcap-001"
	svcID := "tst-svc-proc-lcap-001"

	if _, err := db.ExecContext(ctx,
		`INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (capability_id) DO NOTHING`,
		capID,
	); err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO business_services (business_service_id, name, service_type, status, origin, managed, created_at, updated_at)
		 VALUES ($1, $1, 'internal', 'active', 'manual', true, NOW(), NOW())
		 ON CONFLICT (business_service_id) DO NOTHING`,
		svcID,
	); err != nil {
		t.Fatalf("insert business_service: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = 'tst-proc-lcap-001'`)
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, svcID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, capID)
	})

	repo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	p := &process.Process{
		ID:                "tst-proc-lcap-001",
		Name:              "Cap List Test",
		CapabilityID:      capID,
		BusinessServiceID: svcID,
		Status:            "active",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rows, err := repo.ListByCapabilityID(ctx, capID)
	if err != nil {
		t.Fatalf("ListByCapabilityID: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("ListByCapabilityID: want at least 1 row, got 0")
	}

	var got *process.Process
	for _, r := range rows {
		if r.ID == p.ID {
			got = r
			break
		}
	}
	if got == nil {
		t.Fatalf("ListByCapabilityID: process %q not found in result", p.ID)
	}
	if got.BusinessServiceID != svcID {
		t.Errorf("BusinessServiceID: want %q, got %q", svcID, got.BusinessServiceID)
	}
}
