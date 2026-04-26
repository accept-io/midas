package memory

// envelope_structural_test.go — round-trip coverage for ADR-0001 envelope
// structural snapshot fields against the in-memory envelope repo.
//
// The memory repo stores *Envelope directly, so these tests primarily prove
// the struct fields survive Create/GetByID without a separate column
// mapping layer. They also pin the empty-capability-set behaviour that the
// Postgres marshaller's normalisation enforces.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/envelope"
)

func newStructuralEnvelope(t *testing.T, id string, structure envelope.ResolvedStructure) *envelope.Envelope {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	env, err := envelope.New(id, "src-test", "req-"+id, json.RawMessage(`{}`), now)
	if err != nil {
		t.Fatalf("envelope.New: %v", err)
	}
	env.Resolved.Structure = structure
	return env
}

func TestMemoryEnvelopeRepo_StructuralSnapshotRoundTrip_Populated(t *testing.T) {
	repo := NewEnvelopeRepo()
	ctx := context.Background()

	want := envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-mem-1", Origin: "manual", Managed: true, Replaces: "", Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-mem-1", Origin: "manual", Managed: true, Replaces: "bs-mem-0", Status: "active",
		},
		EnablingCapabilities: []envelope.CapabilitySnapshot{
			{ID: "cap-a", Name: "Cap A", Origin: "manual", Managed: true, Replaces: "", Status: "active"},
			{ID: "cap-b", Name: "Cap B", Origin: "manual", Managed: true, Replaces: "cap-old", Status: "active"},
		},
	}
	env := newStructuralEnvelope(t, "env-mem-rt-1", want)

	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "env-mem-rt-1")
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}

	if got.Resolved.Structure.Process != want.Process {
		t.Errorf("process snapshot: want %+v, got %+v", want.Process, got.Resolved.Structure.Process)
	}
	if got.Resolved.Structure.BusinessService != want.BusinessService {
		t.Errorf("business service snapshot: want %+v, got %+v", want.BusinessService, got.Resolved.Structure.BusinessService)
	}
	if len(got.Resolved.Structure.EnablingCapabilities) != len(want.EnablingCapabilities) {
		t.Fatalf("enabling capabilities length: want %d, got %d", len(want.EnablingCapabilities), len(got.Resolved.Structure.EnablingCapabilities))
	}
	for i, c := range want.EnablingCapabilities {
		if got.Resolved.Structure.EnablingCapabilities[i] != c {
			t.Errorf("capability[%d]: want %+v, got %+v", i, c, got.Resolved.Structure.EnablingCapabilities[i])
		}
	}
}

func TestMemoryEnvelopeRepo_StructuralSnapshotRoundTrip_EmptyCapabilities(t *testing.T) {
	repo := NewEnvelopeRepo()
	ctx := context.Background()

	want := envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-mem-2", Origin: "manual", Managed: true, Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-mem-2", Origin: "manual", Managed: true, Status: "active",
		},
		EnablingCapabilities: []envelope.CapabilitySnapshot{},
	}
	env := newStructuralEnvelope(t, "env-mem-rt-empty", want)

	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "env-mem-rt-empty")
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}

	if got.Resolved.Structure.EnablingCapabilities == nil {
		t.Error("EnablingCapabilities is nil; want non-nil empty slice for deterministic JSON serialisation")
	}
	if len(got.Resolved.Structure.EnablingCapabilities) != 0 {
		t.Errorf("EnablingCapabilities length: want 0, got %d", len(got.Resolved.Structure.EnablingCapabilities))
	}
}
