package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
)

// In the v1 service-led model, Process belongs to exactly one BusinessService
// (processes.business_service_id NOT NULL). The memory ProcessRepo enforces:
//   - business_service_id is required
//   - the referenced BusinessService must exist (when wired)
//   - parent_process_id, when set, must reference an existing process
//
// Surface still requires a non-empty process_id pointing at an existing process.

// TestG12_Process_InvalidBusinessService_Rejected verifies that creating a process
// with a non-existent business_service_id returns an error.
func TestG12_Process_InvalidBusinessService_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", BusinessServiceID: "bs-nonexistent", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error creating process with non-existent business service, got nil")
	}
}

// TestG12_Process_MissingBusinessService_Rejected verifies that creating a process
// with an empty business_service_id returns an error.
func TestG12_Process_MissingBusinessService_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error creating process with empty business_service_id, got nil")
	}
}

// TestG12_Process_ValidBusinessService_Succeeds verifies that creating a process
// with an existing business_service_id succeeds.
func TestG12_Process_ValidBusinessService_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.BusinessServices.Create(context.Background(), &businessservice.BusinessService{
		ID: "bs1", Name: "bs1", ServiceType: businessservice.ServiceTypeInternal, Status: "active",
		Origin: "manual", Managed: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", BusinessServiceID: "bs1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// TestG12_Process_ParentNonExistent_Rejected verifies that creating a process
// with a parent_process_id that does not exist returns an error.
func TestG12_Process_ParentNonExistent_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.BusinessServices.Create(context.Background(), &businessservice.BusinessService{
		ID: "bs1", Name: "bs1", ServiceType: businessservice.ServiceTypeInternal, Status: "active",
		Origin: "manual", Managed: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "child", Name: "child", BusinessServiceID: "bs1", ParentProcessID: "nonexistent-parent",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error: parent process does not exist, got nil")
	}
}

// TestG12_Process_ValidParent_Succeeds verifies that creating a child process with an
// existing parent succeeds in the v1 service-led model. The previous parent-shares-
// capability invariant is gone; only parent existence is checked.
func TestG12_Process_ValidParent_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.BusinessServices.Create(context.Background(), &businessservice.BusinessService{
		ID: "bs1", Name: "bs1", ServiceType: businessservice.ServiceTypeInternal, Status: "active",
		Origin: "manual", Managed: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}
	if err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "parent", Name: "parent", BusinessServiceID: "bs1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "child", Name: "child", BusinessServiceID: "bs1", ParentProcessID: "parent",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// TestG12_Surface_InvalidProcess_Rejected verifies that creating a surface
// with a non-existent process_id returns an error.
func TestG12_Surface_InvalidProcess_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	now := time.Now()
	err := repos.Surfaces.Create(context.Background(), &surface.DecisionSurface{
		ID:            "surf1",
		Version:       1,
		ProcessID:     "nonexistent-process",
		Status:        surface.SurfaceStatusActive,
		EffectiveFrom: now,
	})
	if err == nil {
		t.Fatal("expected error: process does not exist, got nil")
	}
}

// TestG12_Surface_ValidProcess_Succeeds verifies that creating a surface
// with an existing process_id succeeds.
func TestG12_Surface_ValidProcess_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.BusinessServices.Create(context.Background(), &businessservice.BusinessService{
		ID: "bs1", Name: "bs1", ServiceType: businessservice.ServiceTypeInternal, Status: "active",
		Origin: "manual", Managed: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}
	if err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", BusinessServiceID: "bs1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed process: %v", err)
	}

	now := time.Now()
	err := repos.Surfaces.Create(context.Background(), &surface.DecisionSurface{
		ID:            "surf1",
		Version:       1,
		ProcessID:     "p1",
		Status:        surface.SurfaceStatusActive,
		EffectiveFrom: now,
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}
