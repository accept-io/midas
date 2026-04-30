package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/governanceexpectation"
)

// makeExpectation builds a minimal valid GovernanceExpectation for tests.
// All fields the schema marks NOT NULL are populated; lifecycle pointers
// are left nil so individual tests can set them as needed.
func makeExpectation(id string, version int, status governanceexpectation.ExpectationStatus) *governanceexpectation.GovernanceExpectation {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &governanceexpectation.GovernanceExpectation{
		ID:                id,
		Version:           version,
		ScopeKind:         governanceexpectation.ScopeKindProcess,
		ScopeID:           "proc-1",
		RequiredSurfaceID: "surf-1",
		Name:              id,
		Status:            status,
		EffectiveDate:     now.Add(-time.Hour),
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  json.RawMessage(`{"k":"v"}`),
		BusinessOwner:     "biz-owner",
		TechnicalOwner:    "tech-owner",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func TestGovernanceExpectationMemoryRepo_Create_AppendsAndFindsLatest(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	v1 := makeExpectation("exp-a", 1, governanceexpectation.ExpectationStatusReview)
	v2 := makeExpectation("exp-a", 2, governanceexpectation.ExpectationStatusReview)

	if err := r.Create(ctx, v1); err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if err := r.Create(ctx, v2); err != nil {
		t.Fatalf("create v2: %v", err)
	}

	got, err := r.FindByID(ctx, "exp-a")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("FindByID: expected expectation, got nil")
	}
	if got.Version != 2 {
		t.Errorf("FindByID: want version 2, got %d", got.Version)
	}
}

func TestGovernanceExpectationMemoryRepo_Create_RejectsDuplicate(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	v1 := makeExpectation("exp-dup", 1, governanceexpectation.ExpectationStatusReview)
	if err := r.Create(ctx, v1); err != nil {
		t.Fatalf("first create: %v", err)
	}

	dup := makeExpectation("exp-dup", 1, governanceexpectation.ExpectationStatusReview)
	if err := r.Create(ctx, dup); err == nil {
		t.Fatal("expected error on duplicate (id, version), got nil")
	}
}

func TestGovernanceExpectationMemoryRepo_FindByIDAndVersion_ReturnsExact(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	for _, v := range []int{1, 2, 3} {
		if err := r.Create(ctx, makeExpectation("exp-b", v, governanceexpectation.ExpectationStatusReview)); err != nil {
			t.Fatalf("create v%d: %v", v, err)
		}
	}

	for _, want := range []int{1, 2, 3} {
		got, err := r.FindByIDAndVersion(ctx, "exp-b", want)
		if err != nil {
			t.Fatalf("FindByIDAndVersion(%d): %v", want, err)
		}
		if got == nil || got.Version != want {
			t.Errorf("want version %d, got %v", want, got)
		}
	}

	none, err := r.FindByIDAndVersion(ctx, "exp-b", 99)
	if err != nil || none != nil {
		t.Errorf("missing version: want nil/nil, got %v / %v", none, err)
	}

	none, err = r.FindByIDAndVersion(ctx, "no-such-id", 1)
	if err != nil || none != nil {
		t.Errorf("missing id: want nil/nil, got %v / %v", none, err)
	}
}

func TestGovernanceExpectationMemoryRepo_ListVersions_DescendingOrder(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	for _, v := range []int{1, 2, 3} {
		_ = r.Create(ctx, makeExpectation("exp-c", v, governanceexpectation.ExpectationStatusReview))
	}

	versions, err := r.ListVersions(ctx, "exp-c")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("want 3 versions, got %d", len(versions))
	}
	if versions[0].Version != 3 || versions[1].Version != 2 || versions[2].Version != 1 {
		t.Errorf("want [3,2,1], got [%d,%d,%d]",
			versions[0].Version, versions[1].Version, versions[2].Version)
	}

	empty, err := r.ListVersions(ctx, "no-such-id")
	if err != nil {
		t.Fatalf("ListVersions empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("missing id: want empty slice, got %d entries", len(empty))
	}
}

func TestGovernanceExpectationMemoryRepo_Update_MutatesLifecycleFieldsOnly(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	original := makeExpectation("exp-d", 1, governanceexpectation.ExpectationStatusReview)
	if err := r.Create(ctx, original); err != nil {
		t.Fatalf("create: %v", err)
	}

	approvedAt := time.Now().UTC().Add(time.Minute).Truncate(time.Millisecond)
	mutated := *original
	// Mutable fields:
	mutated.Status = governanceexpectation.ExpectationStatusActive
	mutated.UpdatedAt = approvedAt
	mutated.ApprovedBy = "approver"
	mutated.ApprovedAt = &approvedAt
	// Fields the Update contract MUST ignore:
	mutated.Name = "should-be-ignored"
	mutated.Description = "should-be-ignored"
	mutated.ScopeID = "should-be-ignored"
	mutated.RequiredSurfaceID = "should-be-ignored"
	mutated.BusinessOwner = "should-be-ignored"
	mutated.TechnicalOwner = "should-be-ignored"
	mutated.ConditionPayload = json.RawMessage(`{"ignored":true}`)

	if err := r.Update(ctx, &mutated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "exp-d", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil {
		t.Fatal("FindByIDAndVersion: nil after update")
	}

	// Mutable fields applied:
	if got.Status != governanceexpectation.ExpectationStatusActive {
		t.Errorf("Status: want active, got %q", got.Status)
	}
	if !got.UpdatedAt.Equal(approvedAt) {
		t.Errorf("UpdatedAt: want %v, got %v", approvedAt, got.UpdatedAt)
	}
	if got.ApprovedBy != "approver" {
		t.Errorf("ApprovedBy: want approver, got %q", got.ApprovedBy)
	}
	if got.ApprovedAt == nil || !got.ApprovedAt.Equal(approvedAt) {
		t.Errorf("ApprovedAt: want %v, got %v", approvedAt, got.ApprovedAt)
	}

	// Immutable fields preserved from original:
	if got.Name != original.Name {
		t.Errorf("Name should be immutable: want %q, got %q", original.Name, got.Name)
	}
	if got.ScopeID != original.ScopeID {
		t.Errorf("ScopeID should be immutable: want %q, got %q", original.ScopeID, got.ScopeID)
	}
	if got.RequiredSurfaceID != original.RequiredSurfaceID {
		t.Errorf("RequiredSurfaceID should be immutable: want %q, got %q", original.RequiredSurfaceID, got.RequiredSurfaceID)
	}
	if got.BusinessOwner != original.BusinessOwner {
		t.Errorf("BusinessOwner should be immutable: want %q, got %q", original.BusinessOwner, got.BusinessOwner)
	}
	if got.TechnicalOwner != original.TechnicalOwner {
		t.Errorf("TechnicalOwner should be immutable: want %q, got %q", original.TechnicalOwner, got.TechnicalOwner)
	}
	if string(got.ConditionPayload) != string(original.ConditionPayload) {
		t.Errorf("ConditionPayload should be immutable: want %s, got %s",
			string(original.ConditionPayload), string(got.ConditionPayload))
	}
}

func TestGovernanceExpectationMemoryRepo_Update_NotFound_ReturnsError(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	missing := makeExpectation("exp-missing", 1, governanceexpectation.ExpectationStatusActive)
	if err := r.Update(ctx, missing); err == nil {
		t.Fatal("expected error updating non-existent (id, version), got nil")
	}
}

func TestGovernanceExpectationMemoryRepo_NilPayload_NormalisedOnCreate(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	e := makeExpectation("exp-nil", 1, governanceexpectation.ExpectationStatusReview)
	e.ConditionPayload = nil
	if err := r.Create(ctx, e); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.FindByID(ctx, "exp-nil")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("FindByID: nil expectation")
	}
	if string(got.ConditionPayload) != "{}" {
		t.Errorf("ConditionPayload should normalise to '{}'; got %q", string(got.ConditionPayload))
	}

	// Same normalisation for an empty (non-nil) payload.
	e2 := makeExpectation("exp-empty", 1, governanceexpectation.ExpectationStatusReview)
	e2.ConditionPayload = json.RawMessage("")
	if err := r.Create(ctx, e2); err != nil {
		t.Fatalf("create empty: %v", err)
	}
	got2, _ := r.FindByID(ctx, "exp-empty")
	if string(got2.ConditionPayload) != "{}" {
		t.Errorf("empty payload should normalise to '{}'; got %q", string(got2.ConditionPayload))
	}
}

func TestGovernanceExpectationMemoryRepo_ReturnsClones(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	original := makeExpectation("exp-clone", 1, governanceexpectation.ExpectationStatusReview)
	originalPayload := append(json.RawMessage(nil), original.ConditionPayload...)
	originalName := original.Name
	if err := r.Create(ctx, original); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Mutating the original (post-Create) must not mutate stored state.
	original.Name = "mutated-after-create"
	original.ConditionPayload[0] = '!'

	got, err := r.FindByID(ctx, "exp-clone")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Name != originalName {
		t.Errorf("post-Create caller mutation leaked: stored Name = %q", got.Name)
	}
	if string(got.ConditionPayload) != string(originalPayload) {
		t.Errorf("post-Create caller mutation leaked into ConditionPayload: stored = %q", string(got.ConditionPayload))
	}

	// Mutating a returned clone must not mutate stored state either.
	got.Name = "mutated-after-read"
	got.ConditionPayload[0] = '@'
	again, _ := r.FindByID(ctx, "exp-clone")
	if again.Name != originalName {
		t.Errorf("post-read clone mutation leaked: stored Name = %q", again.Name)
	}
	if string(again.ConditionPayload) != string(originalPayload) {
		t.Errorf("post-read clone mutation leaked into ConditionPayload: stored = %q", string(again.ConditionPayload))
	}

	// Pointer fields (EffectiveUntil, RetiredAt, ApprovedAt) must be
	// independently allocated so mutating one does not affect the other.
	approvedAt := time.Now().UTC().Truncate(time.Millisecond)
	withApproved := makeExpectation("exp-clone-ptr", 1, governanceexpectation.ExpectationStatusReview)
	withApproved.ApprovedAt = &approvedAt
	if err := r.Create(ctx, withApproved); err != nil {
		t.Fatalf("create with approved_at: %v", err)
	}
	got2, _ := r.FindByID(ctx, "exp-clone-ptr")
	if got2.ApprovedAt == &approvedAt {
		t.Error("ApprovedAt pointer was not copied; reads alias caller-supplied state")
	}
}

// TestGovernanceExpectationMemoryRepo_ListActiveByScope_FiltersOnEveryPredicate
// covers all four legs of the active-at-time predicate (status, scope,
// effective-date window, retired_at) plus the "in scope but mismatched
// scope_kind" path. Each candidate is built around a pinned `now` so
// the test is deterministic.
func TestGovernanceExpectationMemoryRepo_ListActiveByScope_FiltersOnEveryPredicate(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	mk := func(id string, mutate func(*governanceexpectation.GovernanceExpectation)) {
		e := makeExpectation(id, 1, governanceexpectation.ExpectationStatusActive)
		e.EffectiveDate = now.Add(-time.Hour)
		mutate(e)
		if err := r.Create(ctx, e); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	// Match candidate.
	mk("ge-match", func(e *governanceexpectation.GovernanceExpectation) {})

	// Status filtered out.
	mk("ge-review", func(e *governanceexpectation.GovernanceExpectation) {
		e.Status = governanceexpectation.ExpectationStatusReview
	})
	mk("ge-deprecated", func(e *governanceexpectation.GovernanceExpectation) {
		e.Status = governanceexpectation.ExpectationStatusDeprecated
	})

	// Scope filtered out.
	mk("ge-other-scope", func(e *governanceexpectation.GovernanceExpectation) {
		e.ScopeID = "proc-2"
	})
	mk("ge-other-kind", func(e *governanceexpectation.GovernanceExpectation) {
		e.ScopeKind = governanceexpectation.ScopeKindBusinessService
	})

	// Future-dated.
	mk("ge-future", func(e *governanceexpectation.GovernanceExpectation) {
		e.EffectiveDate = now.Add(time.Hour)
	})

	// Expired.
	mk("ge-expired", func(e *governanceexpectation.GovernanceExpectation) {
		past := now.Add(-time.Minute)
		e.EffectiveUntil = &past
	})

	// EffectiveUntil exactly == now: must NOT match (predicate is strict >).
	mk("ge-until-equals-now", func(e *governanceexpectation.GovernanceExpectation) {
		until := now
		e.EffectiveUntil = &until
	})

	// Retired_at non-nil.
	mk("ge-retired-at", func(e *governanceexpectation.GovernanceExpectation) {
		retired := now.Add(-time.Minute)
		e.RetiredAt = &retired
	})

	// EffectiveDate exactly == now: must match (predicate is <= for from).
	mk("ge-from-equals-now", func(e *governanceexpectation.GovernanceExpectation) {
		e.EffectiveDate = now
	})

	got, err := r.ListActiveByScope(ctx, governanceexpectation.ScopeKindProcess, "proc-1", now)
	if err != nil {
		t.Fatalf("ListActiveByScope: %v", err)
	}

	gotIDs := map[string]bool{}
	for _, e := range got {
		gotIDs[e.ID] = true
	}
	want := []string{"ge-match", "ge-from-equals-now"}
	for _, id := range want {
		if !gotIDs[id] {
			t.Errorf("expected %q in result, got %v", id, gotIDs)
		}
	}
	for _, id := range []string{"ge-review", "ge-deprecated", "ge-other-scope", "ge-other-kind", "ge-future", "ge-expired", "ge-until-equals-now", "ge-retired-at"} {
		if gotIDs[id] {
			t.Errorf("did not expect %q in result", id)
		}
	}
}

// TestGovernanceExpectationMemoryRepo_ListActiveByScope_ReturnsClones
// asserts the read path's defensive-copy posture extends to
// ListActiveByScope: callers must not be able to mutate stored state
// through a returned pointer.
func TestGovernanceExpectationMemoryRepo_ListActiveByScope_ReturnsClones(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	e := makeExpectation("ge-clone", 1, governanceexpectation.ExpectationStatusActive)
	e.EffectiveDate = now.Add(-time.Hour)
	originalName := e.Name
	if err := r.Create(ctx, e); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.ListActiveByScope(ctx, governanceexpectation.ScopeKindProcess, "proc-1", now)
	if err != nil {
		t.Fatalf("ListActiveByScope: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 result, got %d", len(got))
	}

	got[0].Name = "mutated-by-caller"
	again, _ := r.ListActiveByScope(ctx, governanceexpectation.ScopeKindProcess, "proc-1", now)
	if again[0].Name != originalName {
		t.Errorf("post-read mutation leaked into stored state: stored Name = %q", again[0].Name)
	}
}

// TestGovernanceExpectationMemoryRepo_ListActiveByScope_MultipleVersions
// asserts the impl returns every active version when more than one
// satisfies the predicate. Picking a single version is the caller's
// concern, not the repo's.
func TestGovernanceExpectationMemoryRepo_ListActiveByScope_MultipleVersions(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	for _, v := range []int{1, 2} {
		e := makeExpectation("ge-multi", v, governanceexpectation.ExpectationStatusActive)
		e.EffectiveDate = now.Add(-time.Hour)
		if err := r.Create(ctx, e); err != nil {
			t.Fatalf("create v%d: %v", v, err)
		}
	}

	got, err := r.ListActiveByScope(ctx, governanceexpectation.ScopeKindProcess, "proc-1", now)
	if err != nil {
		t.Fatalf("ListActiveByScope: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 versions, got %d", len(got))
	}
}

// TestGovernanceExpectationMemoryRepo_ListActiveByScope_EmptyResult
// asserts the empty-scope path returns a nil/empty slice with no error.
func TestGovernanceExpectationMemoryRepo_ListActiveByScope_EmptyResult(t *testing.T) {
	r := NewGovernanceExpectationRepo()
	ctx := context.Background()

	got, err := r.ListActiveByScope(ctx, governanceexpectation.ScopeKindProcess, "no-such-process", time.Now().UTC())
	if err != nil {
		t.Fatalf("ListActiveByScope: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 results, got %d", len(got))
	}
}

// TestGovernanceExpectationMemoryRepo_RepositoryInterface is a redundant
// compile-time-style check — if the Repository assignment fails the
// package would not build. Kept as a self-documenting assertion that the
// memory repo satisfies the same interface postgres uses.
func TestGovernanceExpectationMemoryRepo_RepositoryInterface(t *testing.T) {
	var r governanceexpectation.Repository = NewGovernanceExpectationRepo()
	if r == nil {
		t.Fatal("memory repo must satisfy governanceexpectation.Repository")
	}
	// Touch the repo so the assignment is observable to vet.
	if _, err := r.FindByID(context.Background(), "no-such-id"); err != nil {
		t.Fatalf("FindByID on empty repo: want nil error, got %v", err)
	}
}
