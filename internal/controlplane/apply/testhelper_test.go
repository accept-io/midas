package apply_test

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store"
)

// seedTestProcess seeds the canonical "test.cap" capability and "test.process"
// process into repos so that surface fixtures referencing ProcessID="test.process"
// pass the memory-store structural integrity checks introduced in G-12.
func seedTestProcess(t *testing.T, repos *store.Repositories) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repos.Capabilities.Create(ctx, &capability.Capability{
		ID: "test.cap", Name: "Test Cap", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seedTestProcess: create capability: %v", err)
	}
	if err := repos.Processes.Create(ctx, &process.Process{
		ID: "test.process", Name: "Test Process", CapabilityID: "test.cap",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seedTestProcess: create process: %v", err)
	}
}
