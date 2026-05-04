package postgres

// capability_apply_integration_test.go — focused Postgres-backed
// regression tests for control-plane → Capability persistence
// invariants. Skipped automatically when DATABASE_URL is unset
// (handled by openTestDB).
//
// Companion to bsr_apply_integration_test.go and
// hierarchy_apply_integration_test.go. This file pins per-Capability
// apply-time round-trips that need the full apply service + Postgres
// store wired together.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// TestApplyCapability_PostgresPersistsCreatedBy proves that the apply
// actor passed into svc.Apply propagates through the control-plane
// mapper into the Capability domain struct and lands on the
// capabilities row's created_by column. Pre-Phase-0B-1, the field
// was set on the in-flight struct but silently dropped by the
// Postgres repo Create — so this test would have failed with a
// CreatedBy of "" against the old code. After the fix it pins the
// actor end-to-end at the storage boundary.
func TestApplyCapability_PostgresPersistsCreatedBy(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM capabilities WHERE capability_id = 'cap-int-cb-actor'`)
		db.Close()
	})
	// Pre-clean.
	_, _ = db.ExecContext(ctx,
		`DELETE FROM capabilities WHERE capability_id = 'cap-int-cb-actor'`)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		ControlAudit: repos.ControlAudit,
	})

	const actor = "operator:cap-created-by"
	bundle := []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-int-cb-actor",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-int-cb-actor", Name: "CB actor capability"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
	}

	result := svc.Apply(ctx, bundle, actor)
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("validation errors: %+v", result.ValidationErrors)
	}
	if result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: %+v", result.Results)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("CreatedCount: want 1, got %d (result=%+v)", result.CreatedCount(), result)
	}

	capRepo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}
	got, err := capRepo.GetByID(ctx, "cap-int-cb-actor")
	if err != nil {
		t.Fatalf("GetByID after apply: %v", err)
	}
	if got == nil {
		t.Fatal("expected capability, got nil")
	}
	if got.CreatedBy != actor {
		t.Errorf("Capability persisted with wrong created_by: want %q, got %q",
			actor, got.CreatedBy)
	}
}
