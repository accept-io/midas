package apply_test

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store"
)

// seedTestProcess seeds the canonical "test.bs" business service, "test.cap"
// capability, and "test.process" process into repos so that surface fixtures
// referencing ProcessID="test.process" pass the memory-store structural
// integrity checks. In the v1 service-led model Process belongs to a
// BusinessService (required); the Capability is seeded for completeness but
// is not referenced by Process.
func seedTestProcess(t *testing.T, repos *store.Repositories) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repos.BusinessServices.Create(ctx, &businessservice.BusinessService{
		ID: "test.bs", Name: "Test BS", ServiceType: businessservice.ServiceTypeInternal,
		Status: "active", Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seedTestProcess: create business service: %v", err)
	}
	if err := repos.Capabilities.Create(ctx, &capability.Capability{
		ID: "test.cap", Name: "Test Cap", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seedTestProcess: create capability: %v", err)
	}
	if err := repos.Processes.Create(ctx, &process.Process{
		ID: "test.process", Name: "Test Process", BusinessServiceID: "test.bs",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seedTestProcess: create process: %v", err)
	}
}
