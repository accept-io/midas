package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/process"
)

func TestProcessRepo_ListByBusinessService(t *testing.T) {
	ctx := context.Background()
	procRepo := NewProcessRepo()
	now := time.Now().UTC()

	// Seed three processes across two business services.
	for _, p := range []*process.Process{
		{ID: "proc-c", Name: "C", BusinessServiceID: "bs-1", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "proc-a", Name: "A", BusinessServiceID: "bs-1", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "proc-other", Name: "Other", BusinessServiceID: "bs-2", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
	} {
		if err := procRepo.Create(ctx, p); err != nil {
			t.Fatalf("seed %s: %v", p.ID, err)
		}
	}

	got, err := procRepo.ListByBusinessService(ctx, "bs-1")
	if err != nil {
		t.Fatalf("ListByBusinessService: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 processes for bs-1, got %d", len(got))
	}
	// Deterministic ordering by process ID.
	if got[0].ID != "proc-a" || got[1].ID != "proc-c" {
		t.Errorf("ordering mismatch; got [%s, %s]", got[0].ID, got[1].ID)
	}
	for _, p := range got {
		if p.BusinessServiceID != "bs-1" {
			t.Errorf("filter leaked: process %s has business_service_id=%q", p.ID, p.BusinessServiceID)
		}
	}

	// Empty result for an unknown BS must be a non-nil empty slice
	// per the interface contract.
	none, err := procRepo.ListByBusinessService(ctx, "bs-nonexistent")
	if err != nil {
		t.Fatalf("ListByBusinessService(unknown): %v", err)
	}
	if none == nil {
		t.Error("empty result should be empty slice, not nil")
	}
	if len(none) != 0 {
		t.Errorf("want 0 processes for unknown BS, got %d", len(none))
	}
}

func TestProcessRepo_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	procRepo := NewProcessRepo()

	now := time.Now().UTC()
	p := &process.Process{
		ID:                "proc-create-001",
		Name:              "Loan Approval",
		BusinessServiceID: "bs-create-001",
		Status:            "active",
		Origin:            "manual",
		Managed:           true,
		Owner:             "team-lending",
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := procRepo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := procRepo.GetByID(ctx, p.ID)
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
		{"Status", p.Status, got.Status},
		{"Origin", p.Origin, got.Origin},
		{"Owner", p.Owner, got.Owner},
	}
	for _, ck := range checks {
		if ck.want != ck.got {
			t.Errorf("%s: want %q, got %q", ck.field, ck.want, ck.got)
		}
	}
	if got.Managed != p.Managed {
		t.Errorf("Managed: want %v, got %v", p.Managed, got.Managed)
	}
}

// TestProcessRepo_Create_BusinessServiceValidation guards the v1 service-led
// invariant: when a business-service repo is wired in, Create must fail for
// an unknown business_service_id and for a missing one.
func TestProcessRepo_Create_BusinessServiceValidation(t *testing.T) {
	ctx := context.Background()
	bsRepo := NewBusinessServiceRepo()
	procRepo := NewProcessRepo()
	procRepo.businessSvcs = bsRepo

	now := time.Now().UTC()
	if err := bsRepo.Create(ctx, &businessservice.BusinessService{
		ID:          "bs-proc-val",
		Name:        "BS",
		ServiceType: businessservice.ServiceTypeInternal,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}

	// Process with known business service succeeds.
	if err := procRepo.Create(ctx, &process.Process{
		ID:                "proc-val-001",
		Name:              "OK",
		BusinessServiceID: "bs-proc-val",
		Status:            "active",
		Origin:            "manual",
		Managed:           true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("Create with known business service: %v", err)
	}

	// Process with unknown business service fails.
	if err := procRepo.Create(ctx, &process.Process{
		ID:                "proc-val-002",
		Name:              "Bad",
		BusinessServiceID: "bs-missing",
		Status:            "active",
		Origin:            "manual",
		Managed:           true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err == nil {
		t.Fatal("expected error for unknown business_service_id, got nil")
	}

	// Process with empty business_service_id fails.
	if err := procRepo.Create(ctx, &process.Process{
		ID:        "proc-val-003",
		Name:      "Empty",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err == nil {
		t.Fatal("expected error for empty business_service_id, got nil")
	}
}
