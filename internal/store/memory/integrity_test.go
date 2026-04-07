package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/processcapability"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
)

// TestG12_Process_InvalidCapability_Rejected verifies that creating a process
// with a non-existent capability_id returns an error.
func TestG12_Process_InvalidCapability_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", CapabilityID: "cap-nonexistent", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error creating process with non-existent capability, got nil")
	}
}

// TestG12_Process_ValidCapability_Succeeds verifies that creating a process
// with an existing capability_id succeeds.
func TestG12_Process_ValidCapability_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", CapabilityID: "cap1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// TestG12_Process_InvalidBusinessService_Rejected verifies that creating a process
// with a non-existent business_service_id returns an error.
func TestG12_Process_InvalidBusinessService_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", CapabilityID: "cap1", BusinessServiceID: "bs-nonexistent",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error creating process with non-existent business service, got nil")
	}
}

// TestG12_Process_ValidBusinessService_Succeeds verifies that creating a process
// with an existing business_service_id succeeds.
func TestG12_Process_ValidBusinessService_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := repos.BusinessServices.Create(context.Background(), &businessservice.BusinessService{
		ID: "bs1", Name: "bs1", Status: "active", Origin: "manual", Managed: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", CapabilityID: "cap1", BusinessServiceID: "bs1",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// TestG12_Process_ParentDifferentCapability_Rejected verifies that creating a
// child process whose parent belongs to a different capability returns an error.
func TestG12_Process_ParentDifferentCapability_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	for _, id := range []string{"cap1", "cap2"} {
		if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
			ID: id, Name: id, Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("seed capability %q: %v", id, err)
		}
	}

	// Parent belongs to cap1.
	if err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "parent", Name: "parent", CapabilityID: "cap1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed parent process: %v", err)
	}

	// Child belongs to cap2 — different capability → reject.
	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "child", Name: "child", CapabilityID: "cap2", ParentProcessID: "parent",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error: parent and child belong to different capabilities, got nil")
	}
}

// TestG12_Process_ParentNonExistent_Rejected verifies that creating a child
// process with a non-existent parent_process_id returns an error.
func TestG12_Process_ParentNonExistent_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "child", Name: "child", CapabilityID: "cap1", ParentProcessID: "nonexistent-parent",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error: parent process does not exist, got nil")
	}
}

// TestG12_Process_SameCapabilityParent_Succeeds verifies that a child process
// with a parent in the same capability is accepted.
func TestG12_Process_SameCapabilityParent_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "parent", Name: "parent", CapabilityID: "cap1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "child", Name: "child", CapabilityID: "cap1", ParentProcessID: "parent",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// TestG12_ProcessCapability_InvalidProcess_Rejected verifies that creating a
// ProcessCapability link with a non-existent process_id returns an error.
func TestG12_ProcessCapability_InvalidProcess_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}

	err := repos.ProcessCapabilities.Create(context.Background(), &processcapability.ProcessCapability{
		ProcessID: "nonexistent-process", CapabilityID: "cap1", CreatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error: process does not exist, got nil")
	}
}

// TestG12_ProcessCapability_InvalidCapability_Rejected verifies that creating a
// ProcessCapability link with a non-existent capability_id returns an error.
func TestG12_ProcessCapability_InvalidCapability_Rejected(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", CapabilityID: "cap1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed process: %v", err)
	}

	err := repos.ProcessCapabilities.Create(context.Background(), &processcapability.ProcessCapability{
		ProcessID: "p1", CapabilityID: "cap-nonexistent", CreatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error: capability does not exist, got nil")
	}
}

// TestG12_ProcessCapability_Valid_Succeeds verifies that a valid link succeeds.
func TestG12_ProcessCapability_Valid_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", CapabilityID: "cap1", Status: "active",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed process: %v", err)
	}

	err := repos.ProcessCapabilities.Create(context.Background(), &processcapability.ProcessCapability{
		ProcessID: "p1", CapabilityID: "cap1", CreatedAt: time.Now(),
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

	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap1", Name: "cap1", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	if err := repos.Processes.Create(context.Background(), &process.Process{
		ID: "p1", Name: "p1", CapabilityID: "cap1", Status: "active",
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
