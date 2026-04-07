package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
)

func TestBusinessServiceRepo_CreateAndGetByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	svc := &businessservice.BusinessService{
		ID:              "tst-bs-001",
		Name:            "Loan Origination",
		Description:     "Core lending service",
		ServiceType:     businessservice.ServiceTypeCustomerFacing,
		RegulatoryScope: "APRA CPS 230",
		Status:          "active",
		Origin:          "manual",
		Managed:         true,
		Replaces:        "",
		OwnerID:         "team-lending",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := repo.Create(ctx, svc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, svc.ID)
	})

	got, err := repo.GetByID(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected service, got nil")
	}

	checks := []struct{ field, want, got string }{
		{"ID", svc.ID, got.ID},
		{"Name", svc.Name, got.Name},
		{"Description", svc.Description, got.Description},
		{"ServiceType", string(svc.ServiceType), string(got.ServiceType)},
		{"RegulatoryScope", svc.RegulatoryScope, got.RegulatoryScope},
		{"Status", svc.Status, got.Status},
		{"Origin", svc.Origin, got.Origin},
		{"Replaces", svc.Replaces, got.Replaces},
		{"OwnerID", svc.OwnerID, got.OwnerID},
	}
	for _, c := range checks {
		if c.want != c.got {
			t.Errorf("%s: want %q, got %q", c.field, c.want, c.got)
		}
	}
	if got.Managed != svc.Managed {
		t.Errorf("Managed: want %v, got %v", svc.Managed, got.Managed)
	}
	if !got.CreatedAt.Equal(svc.CreatedAt) {
		t.Errorf("CreatedAt: want %v, got %v", svc.CreatedAt, got.CreatedAt)
	}
	if !got.UpdatedAt.Equal(svc.UpdatedAt) {
		t.Errorf("UpdatedAt: want %v, got %v", svc.UpdatedAt, got.UpdatedAt)
	}
}

func TestBusinessServiceRepo_GetByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}

	got, err := repo.GetByID(ctx, "tst-bs-nonexistent")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestBusinessServiceRepo_Exists(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}

	ok, err := repo.Exists(ctx, "tst-bs-nonexistent")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Error("expected false for non-existent service")
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	svc := &businessservice.BusinessService{
		ID:          "tst-bs-exists-001",
		Name:        "Payments",
		ServiceType: businessservice.ServiceTypeInternal,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.Create(ctx, svc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, svc.ID)
	})

	ok, err = repo.Exists(ctx, svc.ID)
	if err != nil {
		t.Fatalf("Exists after create: %v", err)
	}
	if !ok {
		t.Error("expected true after create")
	}
}

func TestBusinessServiceRepo_Update(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	svc := &businessservice.BusinessService{
		ID:          "tst-bs-upd-001",
		Name:        "Original Name",
		ServiceType: businessservice.ServiceTypeInternal,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.Create(ctx, svc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, svc.ID)
	})

	updated := now.Add(time.Second)
	svc.Name = "Updated Name"
	svc.ServiceType = businessservice.ServiceTypeCustomerFacing
	svc.Status = "deprecated"
	svc.Description = "now has a description"
	svc.OwnerID = "team-new"
	svc.UpdatedAt = updated

	if err := repo.Update(ctx, svc); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("Name: want %q, got %q", "Updated Name", got.Name)
	}
	if got.ServiceType != businessservice.ServiceTypeCustomerFacing {
		t.Errorf("ServiceType: want customer_facing, got %s", got.ServiceType)
	}
	if got.Status != "deprecated" {
		t.Errorf("Status: want deprecated, got %s", got.Status)
	}
	if got.Description != "now has a description" {
		t.Errorf("Description: want %q, got %q", "now has a description", got.Description)
	}
	if got.OwnerID != "team-new" {
		t.Errorf("OwnerID: want %q, got %q", "team-new", got.OwnerID)
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

func TestBusinessServiceRepo_List(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	ids := []string{"tst-bs-list-001", "tst-bs-list-002"}
	for i, id := range ids {
		svc := &businessservice.BusinessService{
			ID:          id,
			Name:        fmt.Sprintf("Service %d", i+1),
			ServiceType: businessservice.ServiceTypeInternal,
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := repo.Create(ctx, svc); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = $1`, id)
		}
	})

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := 0
	for _, s := range all {
		for _, id := range ids {
			if s.ID == id {
				found++
			}
		}
	}
	if found != len(ids) {
		t.Errorf("List: want %d test services in result, got %d", len(ids), found)
	}
}
