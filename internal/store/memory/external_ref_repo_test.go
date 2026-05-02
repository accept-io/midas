package memory

// External-ref round-trip tests for the five entities that gained an
// ExternalRef field in Epic 1, PR 3:
//
//	BusinessService, BusinessServiceRelationship,
//	AISystem, AISystemVersion, AISystemBinding
//
// Tests are co-located here so the shared makeExtRef factory and the
// per-entity coverage matrix sit in one file. Each entity gets the same
// five tests:
//
//   - Create_WithExternalRef_Persists
//   - Create_WithoutExternalRef_StoresNil
//   - Create_RejectsInconsistentExternalRef
//   - Get_RoundTripsExternalRef
//   - DefensiveCopy_PreventsExternalRefMutation
//
// The DefensiveCopy test pins the LastSyncedAt deep-copy contract:
// mutating the timestamp on a returned value must not affect stored
// state.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/externalref"
)

// makeExtRef builds a populated ExternalRef pointing at GitHub. Used as
// the canonical "fully populated" fixture across the five entities.
func makeExtRef() *externalref.ExternalRef {
	t := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	return &externalref.ExternalRef{
		SourceSystem:  "github",
		SourceID:      "accept-io/midas",
		SourceURL:     "https://github.com/accept-io/midas",
		SourceVersion: "v1.2.0",
		LastSyncedAt:  &t,
	}
}

// makeInconsistentExtRef builds an ExternalRef that violates the
// consistency rule (system set, id empty). All five entity Create paths
// must reject this with externalref.ErrInconsistent.
func makeInconsistentExtRef() *externalref.ExternalRef {
	return &externalref.ExternalRef{SourceSystem: "github" /* SourceID intentionally empty */}
}

// ---------------------------------------------------------------------------
// BusinessService
// ---------------------------------------------------------------------------

func makeBSWithExt(id string, ref *externalref.ExternalRef) *businessservice.BusinessService {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &businessservice.BusinessService{
		ID:          id,
		Name:        id,
		ServiceType: businessservice.ServiceTypeInternal,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExternalRef: ref,
	}
}

func TestMemoryBSRepo_ExternalRef_Persists(t *testing.T) {
	r := NewBusinessServiceRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeBSWithExt("bs-ext", makeExtRef())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByID(ctx, "bs-ext")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ExternalRef == nil || got.ExternalRef.SourceSystem != "github" {
		t.Errorf("ExternalRef not persisted: %+v", got.ExternalRef)
	}
}

func TestMemoryBSRepo_ExternalRef_NilStaysNil(t *testing.T) {
	r := NewBusinessServiceRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeBSWithExt("bs-no-ext", nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := r.GetByID(ctx, "bs-no-ext")
	if got.ExternalRef != nil {
		t.Errorf("ExternalRef should be nil; got %+v", got.ExternalRef)
	}
}

func TestMemoryBSRepo_ExternalRef_EmptyButNonNil_CanonicalisesToNil(t *testing.T) {
	r := NewBusinessServiceRepo()
	ctx := context.Background()
	bs := makeBSWithExt("bs-canon", &externalref.ExternalRef{})
	if err := r.Create(ctx, bs); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := r.GetByID(ctx, "bs-canon")
	if got.ExternalRef != nil {
		t.Errorf("Empty ExternalRef should canonicalise to nil; got %+v", got.ExternalRef)
	}
}

func TestMemoryBSRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	r := NewBusinessServiceRepo()
	err := r.Create(context.Background(), makeBSWithExt("bs-bad", makeInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("Create with inconsistent ExternalRef: want ErrInconsistent, got %v", err)
	}
}

func TestMemoryBSRepo_ExternalRef_DefensiveCopy(t *testing.T) {
	r := NewBusinessServiceRepo()
	ctx := context.Background()
	bs := makeBSWithExt("bs-mut", makeExtRef())
	if err := r.Create(ctx, bs); err != nil {
		t.Fatal(err)
	}

	// Caller mutates the input after Create.
	bs.ExternalRef.SourceSystem = "MUTATED"
	mutated := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	*bs.ExternalRef.LastSyncedAt = mutated

	got, _ := r.GetByID(ctx, "bs-mut")
	if got.ExternalRef.SourceSystem == "MUTATED" {
		t.Error("caller mutation of ExternalRef.SourceSystem leaked into stored state")
	}
	if got.ExternalRef.LastSyncedAt.Equal(mutated) {
		t.Error("caller mutation of ExternalRef.LastSyncedAt leaked into stored state — pointer not deep-copied")
	}
}

// ---------------------------------------------------------------------------
// BusinessServiceRelationship
// ---------------------------------------------------------------------------

func makeBSRWithExt(id string, ref *externalref.ExternalRef) *businessservice.BusinessServiceRelationship {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &businessservice.BusinessServiceRelationship{
		ID:                    id,
		SourceBusinessService: "bs-a",
		TargetBusinessService: "bs-b",
		RelationshipType:      "depends_on",
		CreatedAt:             now,
		ExternalRef:           ref,
	}
}

func TestMemoryBSRRepo_ExternalRef_Persists(t *testing.T) {
	r := NewBusinessServiceRelationshipRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeBSRWithExt("rel-ext", makeExtRef())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByID(ctx, "rel-ext")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ExternalRef == nil || got.ExternalRef.SourceID != "accept-io/midas" {
		t.Errorf("ExternalRef not persisted: %+v", got.ExternalRef)
	}
}

func TestMemoryBSRRepo_ExternalRef_NilStaysNil(t *testing.T) {
	r := NewBusinessServiceRelationshipRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeBSRWithExt("rel-no-ext", nil)); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, "rel-no-ext")
	if got.ExternalRef != nil {
		t.Errorf("expected nil ExternalRef; got %+v", got.ExternalRef)
	}
}

func TestMemoryBSRRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	r := NewBusinessServiceRelationshipRepo()
	err := r.Create(context.Background(), makeBSRWithExt("rel-bad", makeInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("want ErrInconsistent, got %v", err)
	}
}

func TestMemoryBSRRepo_ExternalRef_DefensiveCopy(t *testing.T) {
	r := NewBusinessServiceRelationshipRepo()
	ctx := context.Background()
	rel := makeBSRWithExt("rel-mut", makeExtRef())
	if err := r.Create(ctx, rel); err != nil {
		t.Fatal(err)
	}
	mutated := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	*rel.ExternalRef.LastSyncedAt = mutated
	got, _ := r.GetByID(ctx, "rel-mut")
	if got.ExternalRef.LastSyncedAt.Equal(mutated) {
		t.Error("LastSyncedAt mutation leaked into stored state")
	}
}

func TestMemoryBSRRepo_ExternalRef_UpdateRoundTrips(t *testing.T) {
	r := NewBusinessServiceRelationshipRepo()
	ctx := context.Background()
	rel := makeBSRWithExt("rel-upd", nil)
	if err := r.Create(ctx, rel); err != nil {
		t.Fatal(err)
	}
	rel.ExternalRef = makeExtRef()
	if err := r.Update(ctx, rel); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.GetByID(ctx, "rel-upd")
	if got.ExternalRef == nil || got.ExternalRef.SourceVersion != "v1.2.0" {
		t.Errorf("Update did not persist ExternalRef: %+v", got.ExternalRef)
	}
}

// ---------------------------------------------------------------------------
// AISystem
// ---------------------------------------------------------------------------

func makeAISystemWithExt(id string, ref *externalref.ExternalRef) *aisystem.AISystem {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &aisystem.AISystem{
		ID:          id,
		Name:        id,
		Status:      aisystem.AISystemStatusActive,
		Origin:      aisystem.AISystemOriginManual,
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExternalRef: ref,
	}
}

func TestMemoryAISystemRepo_ExternalRef_Persists(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeAISystemWithExt("ai-ext", makeExtRef())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := r.GetByID(ctx, "ai-ext")
	if got.ExternalRef == nil || got.ExternalRef.SourceURL != "https://github.com/accept-io/midas" {
		t.Errorf("ExternalRef not persisted: %+v", got.ExternalRef)
	}
}

func TestMemoryAISystemRepo_ExternalRef_NilStaysNil(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeAISystemWithExt("ai-no-ext", nil)); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, "ai-no-ext")
	if got.ExternalRef != nil {
		t.Errorf("expected nil ExternalRef; got %+v", got.ExternalRef)
	}
}

func TestMemoryAISystemRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	r := NewAISystemRepo()
	err := r.Create(context.Background(), makeAISystemWithExt("ai-bad", makeInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("want ErrInconsistent, got %v", err)
	}
}

func TestMemoryAISystemRepo_ExternalRef_DefensiveCopy(t *testing.T) {
	r := NewAISystemRepo()
	ctx := context.Background()
	sys := makeAISystemWithExt("ai-mut", makeExtRef())
	if err := r.Create(ctx, sys); err != nil {
		t.Fatal(err)
	}
	sys.ExternalRef.SourceSystem = "MUTATED"
	got, _ := r.GetByID(ctx, "ai-mut")
	if got.ExternalRef.SourceSystem == "MUTATED" {
		t.Error("caller mutation leaked into stored state")
	}
}

// ---------------------------------------------------------------------------
// AISystemVersion
// ---------------------------------------------------------------------------

func makeAIVersionWithExt(systemID string, version int, ref *externalref.ExternalRef) *aisystem.AISystemVersion {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &aisystem.AISystemVersion{
		AISystemID:           systemID,
		Version:              version,
		Status:               aisystem.AISystemVersionStatusActive,
		EffectiveFrom:        now,
		ComplianceFrameworks: []string{},
		CreatedAt:            now,
		UpdatedAt:            now,
		ExternalRef:          ref,
	}
}

func TestMemoryAIVersionRepo_ExternalRef_Persists(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeAIVersionWithExt("ai-1", 1, makeExtRef())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := r.GetByIDAndVersion(ctx, "ai-1", 1)
	if got.ExternalRef == nil || got.ExternalRef.LastSyncedAt == nil {
		t.Errorf("ExternalRef LastSyncedAt not persisted: %+v", got.ExternalRef)
	}
}

func TestMemoryAIVersionRepo_ExternalRef_NilStaysNil(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeAIVersionWithExt("ai-2", 1, nil)); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByIDAndVersion(ctx, "ai-2", 1)
	if got.ExternalRef != nil {
		t.Errorf("expected nil ExternalRef; got %+v", got.ExternalRef)
	}
}

func TestMemoryAIVersionRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	r := NewAISystemVersionRepo()
	err := r.Create(context.Background(), makeAIVersionWithExt("ai-3", 1, makeInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("want ErrInconsistent, got %v", err)
	}
}

func TestMemoryAIVersionRepo_ExternalRef_DefensiveCopy(t *testing.T) {
	r := NewAISystemVersionRepo()
	ctx := context.Background()
	ver := makeAIVersionWithExt("ai-4", 1, makeExtRef())
	if err := r.Create(ctx, ver); err != nil {
		t.Fatal(err)
	}
	mutated := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	*ver.ExternalRef.LastSyncedAt = mutated
	got, _ := r.GetByIDAndVersion(ctx, "ai-4", 1)
	if got.ExternalRef.LastSyncedAt.Equal(mutated) {
		t.Error("LastSyncedAt mutation leaked into stored state")
	}
}

// ---------------------------------------------------------------------------
// AISystemBinding
// ---------------------------------------------------------------------------

func makeBindingWithExt(id string, ref *externalref.ExternalRef) *aisystem.AISystemBinding {
	return &aisystem.AISystemBinding{
		ID:                id,
		AISystemID:        "ai-1",
		BusinessServiceID: "bs-x",
		CreatedAt:         time.Now().UTC().Truncate(time.Millisecond),
		ExternalRef:       ref,
	}
}

func TestMemoryAIBindingRepo_ExternalRef_Persists(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeBindingWithExt("bind-ext", makeExtRef())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := r.GetByID(ctx, "bind-ext")
	if got.ExternalRef == nil || got.ExternalRef.SourceVersion != "v1.2.0" {
		t.Errorf("ExternalRef not persisted: %+v", got.ExternalRef)
	}
}

func TestMemoryAIBindingRepo_ExternalRef_NilStaysNil(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	if err := r.Create(ctx, makeBindingWithExt("bind-no-ext", nil)); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, "bind-no-ext")
	if got.ExternalRef != nil {
		t.Errorf("expected nil ExternalRef; got %+v", got.ExternalRef)
	}
}

func TestMemoryAIBindingRepo_ExternalRef_RejectsInconsistent(t *testing.T) {
	r := NewAISystemBindingRepo()
	err := r.Create(context.Background(), makeBindingWithExt("bind-bad", makeInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("want ErrInconsistent, got %v", err)
	}
}

func TestMemoryAIBindingRepo_ExternalRef_DefensiveCopy(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	b := makeBindingWithExt("bind-mut", makeExtRef())
	if err := r.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	b.ExternalRef.SourceID = "MUTATED"
	got, _ := r.GetByID(ctx, "bind-mut")
	if got.ExternalRef.SourceID == "MUTATED" {
		t.Error("caller mutation leaked into stored state")
	}
}
