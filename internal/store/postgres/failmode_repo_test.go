package postgres

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/failmode"
)

func cleanupFailModePolicies(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM fail_mode_policies`); err != nil {
		t.Fatalf("cleanup fail_mode_policies: %v", err)
	}
}

func makePostgresTestPolicy(id string, version int, status failmode.FailModePolicyStatus, effective time.Time) *failmode.FailModePolicy {
	return &failmode.FailModePolicy{
		ID:             id,
		Version:        version,
		Name:           "Test " + id,
		Description:    "Postgres repo test fixture.",
		Status:         status,
		EffectiveDate:  effective,
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, Reason: "audit substrate cannot record"},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed, Reason: "policy evaluator unreachable"},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed},
		},
		Origin:    "manual",
		Managed:   true,
		CreatedAt: effective,
		UpdatedAt: effective,
		CreatedBy: "test",
	}
}

func TestPostgresFailModePolicyRepo_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, err := NewFailModePolicyRepo(db)
	if err != nil {
		t.Fatalf("NewFailModePolicyRepo: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := r.Create(ctx, makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)); err != nil {
		t.Fatalf("Create v1: %v", err)
	}
	if err := r.Create(ctx, makePostgresTestPolicy("policy-a", 2, failmode.FailModePolicyStatusReview, now)); err != nil {
		t.Fatalf("Create v2: %v", err)
	}

	got, err := r.FindByID(ctx, "policy-a")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("FindByID: want v2, got %+v", got)
	}
}

func TestPostgresFailModePolicyRepo_FindByID_UnknownReturnsNil(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	got, err := r.FindByID(context.Background(), "missing")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got != nil {
		t.Errorf("FindByID: want nil for missing id, got %+v", got)
	}
}

func TestPostgresFailModePolicyRepo_FindByIDAndVersion(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := r.Create(ctx, makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "policy-a", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion v1: %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Errorf("FindByIDAndVersion v1: want v1, got %+v", got)
	}

	got, err = r.FindByIDAndVersion(ctx, "policy-a", 99)
	if err != nil {
		t.Fatalf("FindByIDAndVersion v99: %v", err)
	}
	if got != nil {
		t.Errorf("FindByIDAndVersion v99: want nil for missing version, got %+v", got)
	}
}

func TestPostgresFailModePolicyRepo_FindActiveAt(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tEnd := t0.Add(24 * time.Hour)

	active := makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, t0)
	active.EffectiveUntil = &tEnd
	if err := r.Create(ctx, active); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Inside window
	got, err := r.FindActiveAt(ctx, "policy-a", t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt inside: %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Errorf("FindActiveAt inside: want v1, got %+v", got)
	}

	// Before EffectiveDate
	got, err = r.FindActiveAt(ctx, "policy-a", t0.Add(-time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt before: %v", err)
	}
	if got != nil {
		t.Errorf("FindActiveAt before EffectiveDate: want nil, got %+v", got)
	}

	// At EffectiveUntil — strict > means excluded.
	got, err = r.FindActiveAt(ctx, "policy-a", tEnd)
	if err != nil {
		t.Fatalf("FindActiveAt at-until: %v", err)
	}
	if got != nil {
		t.Errorf("FindActiveAt at EffectiveUntil: want nil, got %+v", got)
	}
}

func TestPostgresFailModePolicyRepo_FindActiveAt_ExcludesNonActive(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := r.Create(ctx, makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusReview, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindActiveAt(ctx, "policy-a", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got != nil {
		t.Errorf("FindActiveAt: want nil for review status, got %+v", got)
	}
}

func TestPostgresFailModePolicyRepo_ListVersionsDescending(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	for v := 1; v <= 3; v++ {
		if err := r.Create(ctx, makePostgresTestPolicy("policy-a", v, failmode.FailModePolicyStatusReview, now)); err != nil {
			t.Fatalf("Create v%d: %v", v, err)
		}
	}

	got, err := r.ListVersions(ctx, "policy-a")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListVersions: want 3, got %d", len(got))
	}
	for i, want := range []int{3, 2, 1} {
		if got[i].Version != want {
			t.Errorf("ListVersions[%d]: want v%d, got v%d", i, want, got[i].Version)
		}
	}

	got, err = r.ListVersions(ctx, "missing")
	if err != nil {
		t.Fatalf("ListVersions missing: %v", err)
	}
	if got == nil {
		t.Errorf("ListVersions missing: want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("ListVersions missing: want empty, got %d entries", len(got))
	}
}

func TestPostgresFailModePolicyRepo_RulesJSONBRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	original := makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)
	if err := r.Create(ctx, original); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "policy-a", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil {
		t.Fatal("FindByIDAndVersion: nil result")
	}
	if len(got.Rules) != len(original.Rules) {
		t.Fatalf("Rules length: want %d, got %d", len(original.Rules), len(got.Rules))
	}

	// Build a class→rule map to compare independent of slice order.
	got_by_class := map[failmode.CorrectnessClass]failmode.FailModePolicyRule{}
	for _, r := range got.Rules {
		got_by_class[r.CorrectnessClass] = r
	}
	for _, want := range original.Rules {
		gotRule, ok := got_by_class[want.CorrectnessClass]
		if !ok {
			t.Errorf("missing class %q in round-trip", want.CorrectnessClass)
			continue
		}
		if gotRule.PermittedMode != want.PermittedMode {
			t.Errorf("class %q: PermittedMode want %q, got %q", want.CorrectnessClass, want.PermittedMode, gotRule.PermittedMode)
		}
		if gotRule.Reason != want.Reason {
			t.Errorf("class %q: Reason want %q, got %q", want.CorrectnessClass, want.Reason, gotRule.Reason)
		}
	}
}

func TestPostgresFailModePolicyRepo_DuplicatePrimaryKeyRejected(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := r.Create(ctx, makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)); err != nil {
		t.Fatalf("Create v1: %v", err)
	}
	err := r.Create(ctx, makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now))
	if err == nil {
		t.Fatal("Create with duplicate (id, version) should fail; got nil error")
	}
}

func TestPostgresFailModePolicyRepo_InvalidStatusRejectedByCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	bad := makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatus("garbage"), now)
	err := r.Create(ctx, bad)
	if err == nil {
		t.Fatal("Create with invalid status should fail CHECK constraint; got nil error")
	}
	// An unknown status simultaneously violates chk_fmp_status (status not in
	// the enum) and chk_fmp_approval_fields (status matches neither arm of
	// the approval-fields predicate). Postgres surfaces only one constraint
	// per failed insert; either is acceptable evidence that the schema
	// rejects unknown status values.
	if !strings.Contains(err.Error(), "chk_fmp_status") && !strings.Contains(err.Error(), "chk_fmp_approval_fields") {
		t.Errorf("expected chk_fmp_status or chk_fmp_approval_fields violation; got %v", err)
	}
}

func TestPostgresFailModePolicyRepo_InvalidOriginRejectedByCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	bad := makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)
	bad.Origin = "auto"
	err := r.Create(ctx, bad)
	if err == nil {
		t.Fatal("Create with invalid origin should fail CHECK constraint; got nil error")
	}
	if !strings.Contains(err.Error(), "chk_fmp_origin") {
		t.Errorf("expected chk_fmp_origin violation; got %v", err)
	}
}

func TestPostgresFailModePolicyRepo_InvalidEffectiveDatesRejectedByCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	bad := makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)
	until := now // == effective_date, fails strict >
	bad.EffectiveUntil = &until
	err := r.Create(ctx, bad)
	if err == nil {
		t.Fatal("Create with EffectiveUntil == EffectiveDate should fail CHECK constraint; got nil error")
	}
	if !strings.Contains(err.Error(), "chk_fmp_effective_dates") {
		t.Errorf("expected chk_fmp_effective_dates violation; got %v", err)
	}
}

func TestPostgresFailModePolicyRepo_InvalidApprovalFieldsRejectedByCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// status=draft with approved_at set must fail the approval-fields CHECK.
	bad := makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusDraft, now)
	approvedAt := now
	bad.ApprovedAt = &approvedAt
	err := r.Create(ctx, bad)
	if err == nil {
		t.Fatal("Create with draft + approved_at should fail CHECK constraint; got nil error")
	}
	if !strings.Contains(err.Error(), "chk_fmp_approval_fields") {
		t.Errorf("expected chk_fmp_approval_fields violation; got %v", err)
	}
}

func TestPostgresFailModePolicyRepo_SelfReplaceRejectedByCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	bad := makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)
	bad.Replaces = bad.ID
	err := r.Create(ctx, bad)
	if err == nil {
		t.Fatal("Create with replaces == id should fail CHECK constraint; got nil error")
	}
	if !strings.Contains(err.Error(), "chk_fmp_no_self_replace") {
		t.Errorf("expected chk_fmp_no_self_replace violation; got %v", err)
	}
}

func TestPostgresFailModePolicyRepo_NonArrayRulesJSONBRejectedByCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Bypass the repo to inject an object instead of an array, exercising
	// chk_fmp_rules_array directly.
	const q = `
		INSERT INTO fail_mode_policies (
			id, version, name, status, effective_date, rules,
			business_owner, technical_owner, origin, managed,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6::jsonb,
			$7, $8, $9, $10,
			$11, $12
		)
	`
	_, err := db.ExecContext(ctx, q,
		"policy-a", 1, "Test", "active", now,
		`{"not":"an array"}`,
		"owner@example.com", "platform-team", "manual", true,
		now, now,
	)
	if err == nil {
		t.Fatal("Insert with non-array rules JSONB should fail CHECK constraint; got nil error")
	}
	if !strings.Contains(err.Error(), "chk_fmp_rules_array") {
		t.Errorf("expected chk_fmp_rules_array violation; got %v", err)
	}
}

func TestPostgresFailModePolicyRepo_UpdateReplacesInPlace(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := r.Create(ctx, makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusReview, now)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := makePostgresTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)
	approvedAt := now.Add(time.Hour)
	updated.ApprovedBy = "approver-1"
	updated.ApprovedAt = &approvedAt
	updated.UpdatedAt = approvedAt

	if err := r.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "policy-a", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got.Status != failmode.FailModePolicyStatusActive {
		t.Errorf("Status: want active, got %q", got.Status)
	}
	if got.ApprovedBy != "approver-1" {
		t.Errorf("ApprovedBy: want approver-1, got %q", got.ApprovedBy)
	}
}

func TestPostgresFailModePolicyRepo_UpdateOnMissingRowReturnsError(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupFailModePolicies(t, db)

	r, _ := NewFailModePolicyRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	err := r.Update(ctx, makePostgresTestPolicy("missing", 1, failmode.FailModePolicyStatusActive, now))
	if err == nil {
		t.Fatal("Update on missing row should return an error (Postgres posture); got nil")
	}
	if !strings.Contains(err.Error(), "fail_mode_policy not found") {
		t.Errorf("expected 'fail_mode_policy not found' error, got %v", err)
	}
}

func TestPostgresStore_Repositories_FailModePolicies_NonNil(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Store.Repositories: %v", err)
	}
	if repos.FailModePolicies == nil {
		t.Error("Repositories.FailModePolicies must be non-nil after Postgres aggregator")
	}
}
