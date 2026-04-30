package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/governanceexpectation"
)

// makeExpectation builds a fully-populated GovernanceExpectation suitable
// for INSERT under all schema CHECK constraints. Tests override fields as
// needed.
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

// cleanupExpectation deletes test rows. Errors are intentionally
// ignored: t.Cleanup callbacks run AFTER the test function's `defer
// db.Close()`, so the connection is already closed by the time this
// runs. This matches the pattern in capability_repo_test.go and friends.
func cleanupExpectation(_ *testing.T, db *sql.DB, id string) {
	_, _ = db.Exec(`DELETE FROM governance_expectations WHERE id = $1`, id)
}

func TestGovernanceExpectationRepo_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	v1 := makeExpectation("tst-exp-find-001", 1, governanceexpectation.ExpectationStatusReview)
	v2 := makeExpectation("tst-exp-find-001", 2, governanceexpectation.ExpectationStatusReview)
	if err := repo.Create(ctx, v1); err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if err := repo.Create(ctx, v2); err != nil {
		t.Fatalf("create v2: %v", err)
	}
	t.Cleanup(func() { cleanupExpectation(t, db, "tst-exp-find-001") })

	got, err := repo.FindByID(ctx, "tst-exp-find-001")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected expectation, got nil")
	}
	if got.Version != 2 {
		t.Errorf("FindByID: want latest version 2, got %d", got.Version)
	}
	if got.ScopeKind != governanceexpectation.ScopeKindProcess {
		t.Errorf("ScopeKind round-trip: want process, got %q", got.ScopeKind)
	}
	if got.ConditionType != governanceexpectation.ConditionTypeRiskCondition {
		t.Errorf("ConditionType round-trip: want risk_condition, got %q", got.ConditionType)
	}

	// Postgres normalises JSONB; assert semantic equality, not byte
	// identity.
	var stored, expected map[string]any
	if err := json.Unmarshal(got.ConditionPayload, &stored); err != nil {
		t.Fatalf("unmarshal stored payload: %v", err)
	}
	if err := json.Unmarshal(v2.ConditionPayload, &expected); err != nil {
		t.Fatalf("unmarshal expected payload: %v", err)
	}
	if stored["k"] != expected["k"] {
		t.Errorf("ConditionPayload round-trip: want k=%v, got k=%v", expected["k"], stored["k"])
	}
}

func TestGovernanceExpectationRepo_FindByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	got, err := repo.FindByID(ctx, "tst-exp-no-such")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestGovernanceExpectationRepo_Create_DuplicateRejected(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	v1 := makeExpectation("tst-exp-dup-001", 1, governanceexpectation.ExpectationStatusReview)
	if err := repo.Create(ctx, v1); err != nil {
		t.Fatalf("first create: %v", err)
	}
	t.Cleanup(func() { cleanupExpectation(t, db, "tst-exp-dup-001") })

	dup := makeExpectation("tst-exp-dup-001", 1, governanceexpectation.ExpectationStatusReview)
	if err := repo.Create(ctx, dup); err == nil {
		t.Fatal("expected PK violation on duplicate (id, version), got nil")
	}
}

func TestGovernanceExpectationRepo_FindByIDAndVersion_AndListVersions(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	for _, v := range []int{1, 2, 3} {
		e := makeExpectation("tst-exp-list-001", v, governanceexpectation.ExpectationStatusReview)
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("create v%d: %v", v, err)
		}
	}
	t.Cleanup(func() { cleanupExpectation(t, db, "tst-exp-list-001") })

	for _, want := range []int{1, 2, 3} {
		got, err := repo.FindByIDAndVersion(ctx, "tst-exp-list-001", want)
		if err != nil {
			t.Fatalf("FindByIDAndVersion(%d): %v", want, err)
		}
		if got == nil || got.Version != want {
			t.Errorf("want version %d, got %v", want, got)
		}
	}

	none, err := repo.FindByIDAndVersion(ctx, "tst-exp-list-001", 99)
	if err != nil || none != nil {
		t.Errorf("missing version: want nil/nil, got %v / %v", none, err)
	}

	versions, err := repo.ListVersions(ctx, "tst-exp-list-001")
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

	empty, err := repo.ListVersions(ctx, "tst-exp-no-such")
	if err != nil {
		t.Fatalf("ListVersions empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("missing id: want empty slice, got %d entries", len(empty))
	}
}

func TestGovernanceExpectationRepo_Update_MutatesLifecycleFieldsOnly(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	original := makeExpectation("tst-exp-upd-001", 1, governanceexpectation.ExpectationStatusReview)
	if err := repo.Create(ctx, original); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { cleanupExpectation(t, db, "tst-exp-upd-001") })

	approvedAt := time.Now().UTC().Add(time.Minute).Truncate(time.Millisecond)
	mutated := *original
	mutated.Status = governanceexpectation.ExpectationStatusActive
	mutated.UpdatedAt = approvedAt
	mutated.ApprovedBy = "approver"
	mutated.ApprovedAt = &approvedAt
	// Fields the Update SQL does not include — should be ignored:
	mutated.Name = "should-be-ignored"
	mutated.ScopeID = "should-be-ignored"
	mutated.RequiredSurfaceID = "should-be-ignored"
	mutated.BusinessOwner = "should-be-ignored"
	mutated.TechnicalOwner = "should-be-ignored"
	mutated.ConditionPayload = json.RawMessage(`{"ignored":true}`)

	if err := repo.Update(ctx, &mutated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.FindByIDAndVersion(ctx, "tst-exp-upd-001", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil {
		t.Fatal("nil after Update")
	}

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

	// JSONB equality after a full round trip via Postgres normalisation.
	var stored, expected map[string]any
	if err := json.Unmarshal(got.ConditionPayload, &stored); err != nil {
		t.Fatalf("unmarshal stored payload: %v", err)
	}
	if err := json.Unmarshal(original.ConditionPayload, &expected); err != nil {
		t.Fatalf("unmarshal original payload: %v", err)
	}
	if stored["k"] != expected["k"] {
		t.Errorf("ConditionPayload should be immutable; want k=%v, got k=%v",
			expected["k"], stored["k"])
	}
}

func TestGovernanceExpectationRepo_Update_NotFound_ReturnsError(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	missing := makeExpectation("tst-exp-missing-001", 1, governanceexpectation.ExpectationStatusActive)
	err = repo.Update(ctx, missing)
	if err == nil {
		t.Fatal("expected error updating non-existent (id, version), got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found'; got: %v", err)
	}
}

func TestGovernanceExpectationRepo_NilPayload_NormalisedToEmptyObject(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	e := makeExpectation("tst-exp-nil-payload", 1, governanceexpectation.ExpectationStatusReview)
	e.ConditionPayload = nil
	if err := repo.Create(ctx, e); err != nil {
		t.Fatalf("create with nil payload: %v", err)
	}
	t.Cleanup(func() { cleanupExpectation(t, db, "tst-exp-nil-payload") })

	got, err := repo.FindByID(ctx, "tst-exp-nil-payload")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("nil after Create")
	}

	var stored map[string]any
	if err := json.Unmarshal(got.ConditionPayload, &stored); err != nil {
		t.Fatalf("unmarshal stored payload: %v", err)
	}
	if len(stored) != 0 {
		t.Errorf("nil ConditionPayload should normalise to '{}' object; got %s",
			string(got.ConditionPayload))
	}
}

// TestGovernanceExpectationRepo_ListActiveByScope_FiltersOnEveryPredicate
// exercises every leg of the active-at-time predicate against the live
// Postgres index. Each candidate uses unique IDs so the test is isolated
// from any other rows in the table.
func TestGovernanceExpectationRepo_ListActiveByScope_FiltersOnEveryPredicate(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	scopeID := "tst-list-active-proc"

	type candidate struct {
		id     string
		mutate func(*governanceexpectation.GovernanceExpectation)
	}
	cases := []candidate{
		{id: "tst-list-active-match", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			e.ScopeID = scopeID
			e.Status = governanceexpectation.ExpectationStatusActive
			e.EffectiveDate = now.Add(-time.Hour)
		}},
		{id: "tst-list-active-from-equals-now", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			e.ScopeID = scopeID
			e.Status = governanceexpectation.ExpectationStatusActive
			e.EffectiveDate = now
		}},
		{id: "tst-list-active-review", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			e.ScopeID = scopeID
			e.Status = governanceexpectation.ExpectationStatusReview
			e.EffectiveDate = now.Add(-time.Hour)
		}},
		{id: "tst-list-active-deprecated", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			e.ScopeID = scopeID
			e.Status = governanceexpectation.ExpectationStatusDeprecated
			e.EffectiveDate = now.Add(-time.Hour)
		}},
		{id: "tst-list-active-other-scope", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			e.ScopeID = "tst-list-active-other-proc"
			e.Status = governanceexpectation.ExpectationStatusActive
			e.EffectiveDate = now.Add(-time.Hour)
		}},
		{id: "tst-list-active-future", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			e.ScopeID = scopeID
			e.Status = governanceexpectation.ExpectationStatusActive
			e.EffectiveDate = now.Add(time.Hour)
		}},
		{id: "tst-list-active-expired", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			past := now.Add(-time.Minute)
			e.ScopeID = scopeID
			e.Status = governanceexpectation.ExpectationStatusActive
			e.EffectiveDate = now.Add(-time.Hour)
			e.EffectiveUntil = &past
		}},
		{id: "tst-list-active-until-equals-now", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			until := now
			e.ScopeID = scopeID
			e.Status = governanceexpectation.ExpectationStatusActive
			e.EffectiveDate = now.Add(-time.Hour)
			e.EffectiveUntil = &until
		}},
		// retired_at is constrained by the schema's chk_governance_expectations_retired_at
		// to be >= effective_date, so a non-nil retired_at requires a matching
		// past effective_date. The predicate filters on retired_at IS NULL
		// independently of status.
		{id: "tst-list-active-retired-at", mutate: func(e *governanceexpectation.GovernanceExpectation) {
			retired := now.Add(-time.Minute)
			e.ScopeID = scopeID
			e.Status = governanceexpectation.ExpectationStatusActive
			e.EffectiveDate = now.Add(-time.Hour)
			e.RetiredAt = &retired
		}},
	}

	for _, c := range cases {
		c := c
		e := makeExpectation(c.id, 1, governanceexpectation.ExpectationStatusActive)
		c.mutate(e)
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("create %s: %v", c.id, err)
		}
		t.Cleanup(func() { cleanupExpectation(t, db, c.id) })
	}

	got, err := repo.ListActiveByScope(ctx, governanceexpectation.ScopeKindProcess, scopeID, now)
	if err != nil {
		t.Fatalf("ListActiveByScope: %v", err)
	}

	gotIDs := map[string]bool{}
	for _, e := range got {
		gotIDs[e.ID] = true
	}

	wantPresent := []string{"tst-list-active-match", "tst-list-active-from-equals-now"}
	wantAbsent := []string{
		"tst-list-active-review",
		"tst-list-active-deprecated",
		"tst-list-active-other-scope",
		"tst-list-active-future",
		"tst-list-active-expired",
		"tst-list-active-until-equals-now",
		"tst-list-active-retired-at",
	}
	for _, id := range wantPresent {
		if !gotIDs[id] {
			t.Errorf("expected %q in ListActiveByScope result; got %v", id, gotIDs)
		}
	}
	for _, id := range wantAbsent {
		if gotIDs[id] {
			t.Errorf("did not expect %q in ListActiveByScope result; got %v", id, gotIDs)
		}
	}
}

// TestGovernanceExpectationRepo_ListActiveByScope_MultipleVersions
// asserts that when more than one version of the same logical id is
// active and within the effective-date window, all matching versions
// are returned. Selection of a single version is the caller's concern.
func TestGovernanceExpectationRepo_ListActiveByScope_MultipleVersions(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	scopeID := "tst-list-multi-proc"
	id := "tst-list-multi-id"

	for _, v := range []int{1, 2} {
		e := makeExpectation(id, v, governanceexpectation.ExpectationStatusActive)
		e.ScopeID = scopeID
		e.EffectiveDate = now.Add(-time.Hour)
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("create v%d: %v", v, err)
		}
	}
	t.Cleanup(func() { cleanupExpectation(t, db, id) })

	got, err := repo.ListActiveByScope(ctx, governanceexpectation.ScopeKindProcess, scopeID, now)
	if err != nil {
		t.Fatalf("ListActiveByScope: %v", err)
	}

	count := 0
	for _, e := range got {
		if e.ID == id {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected both versions of %q, got %d", id, count)
	}
}

// TestGovernanceExpectationRepo_ListActiveByScope_EmptyResult covers the
// no-matching-rows branch: query returns an empty (or nil) slice with
// no error.
func TestGovernanceExpectationRepo_ListActiveByScope_EmptyResult(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		t.Fatalf("NewGovernanceExpectationRepo: %v", err)
	}

	got, err := repo.ListActiveByScope(
		ctx,
		governanceexpectation.ScopeKindProcess,
		"tst-list-active-no-such-process-id",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("ListActiveByScope: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 results, got %d", len(got))
	}
}
