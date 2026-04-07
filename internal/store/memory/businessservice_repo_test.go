package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
)

func TestBusinessServiceRepo_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	repo := NewBusinessServiceRepo()

	now := time.Now().UTC()
	svc := &businessservice.BusinessService{
		ID:              "bs-create-001",
		Name:            "Loan Origination",
		Description:     "Core lending service",
		ServiceType:     businessservice.ServiceTypeCustomerFacing,
		RegulatoryScope: "APRA CPS 230",
		Status:          "active",
		Origin:          "manual",
		Managed:         true,
		OwnerID:         "team-lending",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := repo.Create(ctx, svc); err != nil {
		t.Fatalf("Create: %v", err)
	}

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
}

func TestBusinessServiceRepo_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewBusinessServiceRepo()

	got, err := repo.GetByID(ctx, "bs-nonexistent")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestBusinessServiceRepo_Exists(t *testing.T) {
	ctx := context.Background()
	repo := NewBusinessServiceRepo()

	ok, err := repo.Exists(ctx, "bs-nonexistent")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Error("expected false for non-existent service")
	}

	now := time.Now().UTC()
	svc := &businessservice.BusinessService{
		ID:          "bs-exists-001",
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

	ok, err = repo.Exists(ctx, svc.ID)
	if err != nil {
		t.Fatalf("Exists after create: %v", err)
	}
	if !ok {
		t.Error("expected true after create")
	}
}

func TestBusinessServiceRepo_Update(t *testing.T) {
	ctx := context.Background()
	repo := NewBusinessServiceRepo()

	now := time.Now().UTC()
	svc := &businessservice.BusinessService{
		ID:          "bs-update-001",
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

	svc.Name = "Updated Name"
	svc.ServiceType = businessservice.ServiceTypeCustomerFacing
	svc.Status = "deprecated"
	svc.Description = "now has a description"
	svc.OwnerID = "team-new"
	svc.UpdatedAt = now.Add(1)

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
}

func TestBusinessServiceRepo_List(t *testing.T) {
	ctx := context.Background()
	repo := NewBusinessServiceRepo()

	now := time.Now().UTC()
	ids := []string{"bs-list-001", "bs-list-002"}
	for i, id := range ids {
		svc := &businessservice.BusinessService{
			ID:          id,
			Name:        "Service " + id,
			ServiceType: businessservice.ServiceTypeInternal,
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now.Add(time.Duration(i)),
		}
		if err := repo.Create(ctx, svc); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

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
		t.Errorf("List: want %d services, got %d total with %d matching", len(ids), len(all), found)
	}
}
