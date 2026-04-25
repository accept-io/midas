package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
)

func TestProcessRepo_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	procRepo := NewProcessRepo()

	now := time.Now().UTC()
	p := &process.Process{
		ID:           "proc-create-001",
		Name:         "Loan Approval",
		CapabilityID: "cap-create-001",
		Status:       "active",
		Origin:       "manual",
		Managed:      true,
		Owner:        "team-lending",
		CreatedAt:    now,
		UpdatedAt:    now,
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
		{"CapabilityID", p.CapabilityID, got.CapabilityID},
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

// TestProcessRepo_Create_CapabilityValidation guards the existing
// capability-existence check in the memory store: when a capabilities
// repo is wired in, Create must fail for unknown capability_id.
func TestProcessRepo_Create_CapabilityValidation(t *testing.T) {
	ctx := context.Background()
	capRepo := NewCapabilityRepo()
	procRepo := NewProcessRepo()
	procRepo.capabilities = capRepo

	now := time.Now().UTC()
	if err := capRepo.Create(ctx, &capability.Capability{
		ID:        "cap-proc-val",
		Name:      "Cap",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}

	// Process with known capability succeeds.
	if err := procRepo.Create(ctx, &process.Process{
		ID:           "proc-val-001",
		Name:         "OK",
		CapabilityID: "cap-proc-val",
		Status:       "active",
		Origin:       "manual",
		Managed:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("Create with known capability: %v", err)
	}

	// Process with unknown capability fails.
	if err := procRepo.Create(ctx, &process.Process{
		ID:           "proc-val-002",
		Name:         "Bad",
		CapabilityID: "cap-missing",
		Status:       "active",
		Origin:       "manual",
		Managed:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err == nil {
		t.Fatal("expected error for unknown capability_id, got nil")
	}
}

func TestProcessRepo_ListByCapabilityID(t *testing.T) {
	ctx := context.Background()
	procRepo := NewProcessRepo()

	now := time.Now().UTC()
	procs := []*process.Process{
		{ID: "proc-list-001", Name: "P1", CapabilityID: "cap-A", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "proc-list-002", Name: "P2", CapabilityID: "cap-A", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "proc-list-003", Name: "P3", CapabilityID: "cap-B", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
	}
	for _, p := range procs {
		if err := procRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create %s: %v", p.ID, err)
		}
	}

	rows, err := procRepo.ListByCapabilityID(ctx, "cap-A")
	if err != nil {
		t.Fatalf("ListByCapabilityID: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("ListByCapabilityID(cap-A): want 2 rows, got %d", len(rows))
	}
}
