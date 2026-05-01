package approval_test

// expectation_approval_test.go — service-level tests for #57's
// ApproveGovernanceExpectation method. Mirrors profile_approval_test.go
// in shape: in-memory fakeExpectationRepo + a fakeControlAuditRepo, with
// the service constructed via NewService(...).WithExpectationRepository(...).
//
// Lifecycle gating, immutability of non-lifecycle fields, and the
// control-audit emission contract are pinned here. The HTTP boundary is
// covered by expectation_approval_handler_test.go; the Postgres-backed
// audit-record persistence is pinned by
// expectation_approval_postgres_test.go.

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/governanceexpectation"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeExpectationRepo struct {
	rows    map[string]*governanceexpectation.GovernanceExpectation // key: "id:version"
	updated []*governanceexpectation.GovernanceExpectation
}

func newFakeExpectationRepo(rows ...*governanceexpectation.GovernanceExpectation) *fakeExpectationRepo {
	r := &fakeExpectationRepo{rows: make(map[string]*governanceexpectation.GovernanceExpectation)}
	for _, e := range rows {
		r.rows[expKey(e.ID, e.Version)] = e
	}
	return r
}

func expKey(id string, version int) string {
	return id + ":" + strconv.Itoa(version)
}

func (f *fakeExpectationRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*governanceexpectation.GovernanceExpectation, error) {
	row, ok := f.rows[expKey(id, version)]
	if !ok {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}

func (f *fakeExpectationRepo) Update(_ context.Context, e *governanceexpectation.GovernanceExpectation) error {
	cp := *e
	f.rows[expKey(e.ID, e.Version)] = &cp
	f.updated = append(f.updated, &cp)
	return nil
}

// fakeControlAuditRepo records every appended record for assertion.
// Errors are not exercised here; the service's appendControlAudit
// swallows them by design (ADR-041b). List is a no-op — it is part of
// the Repository interface but not exercised by approval flow.
type fakeControlAuditRepo struct {
	appended []*controlaudit.ControlAuditRecord
}

func (f *fakeControlAuditRepo) Append(_ context.Context, rec *controlaudit.ControlAuditRecord) error {
	f.appended = append(f.appended, rec)
	return nil
}

func (f *fakeControlAuditRepo) List(_ context.Context, _ controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

func makeReviewExpectation(id string, version int) *governanceexpectation.GovernanceExpectation {
	now := time.Now().UTC().Add(-time.Hour)
	return &governanceexpectation.GovernanceExpectation{
		ID:                id,
		Version:           version,
		ScopeKind:         governanceexpectation.ScopeKindProcess,
		ScopeID:           "proc-1",
		RequiredSurfaceID: "surf-required",
		Name:              id + "-expectation",
		Description:       "test expectation",
		Status:            governanceexpectation.ExpectationStatusReview,
		EffectiveDate:     now,
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  json.RawMessage(`{"min_confidence": 0.5}`),
		BusinessOwner:     "biz-owner",
		TechnicalOwner:    "tech-owner",
		CreatedAt:         now,
		UpdatedAt:         now,
		CreatedBy:         "creator-actor",
	}
}

func makeExpectationWithStatus(id string, version int, status governanceexpectation.ExpectationStatus) *governanceexpectation.GovernanceExpectation {
	e := makeReviewExpectation(id, version)
	e.Status = status
	return e
}

// buildService wires a Service with the supplied expectation rows and
// a recording control-audit repository. Returns both so tests can
// assert on the audit appended slice.
func buildService(t *testing.T, rows ...*governanceexpectation.GovernanceExpectation) (*approval.Service, *fakeExpectationRepo, *fakeControlAuditRepo) {
	t.Helper()
	repo := newFakeExpectationRepo(rows...)
	audit := &fakeControlAuditRepo{}
	svc := approval.NewServiceWithAll(nil, approval.Policy{}, nil, audit).
		WithExpectationRepository(repo)
	return svc, repo, audit
}

// ---------------------------------------------------------------------------
// S1 — happy path: review → active.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_HappyPath_TransitionsToActive(t *testing.T) {
	svc, _, _ := buildService(t, makeReviewExpectation("ge-1", 1))

	updated, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-1", 1, "approver@example.com")
	if err != nil {
		t.Fatalf("ApproveGovernanceExpectation: %v", err)
	}
	if updated.Status != governanceexpectation.ExpectationStatusActive {
		t.Errorf("Status: want active, got %s", updated.Status)
	}
}

// ---------------------------------------------------------------------------
// S2 — approved_by persisted from the actor argument.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_PersistsApprovedBy(t *testing.T) {
	svc, repo, _ := buildService(t, makeReviewExpectation("ge-2", 1))

	if _, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-2", 1, "operator@example.com"); err != nil {
		t.Fatalf("ApproveGovernanceExpectation: %v", err)
	}
	stored := repo.rows[expKey("ge-2", 1)]
	if stored.ApprovedBy != "operator@example.com" {
		t.Errorf("ApprovedBy: got %q, want operator@example.com", stored.ApprovedBy)
	}
}

// ---------------------------------------------------------------------------
// S3 — approved_at populated to a recent UTC time.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_PersistsApprovedAt(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	svc, repo, _ := buildService(t, makeReviewExpectation("ge-3", 1))

	if _, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-3", 1, "approver"); err != nil {
		t.Fatalf("ApproveGovernanceExpectation: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)
	stored := repo.rows[expKey("ge-3", 1)]
	if stored.ApprovedAt == nil {
		t.Fatal("ApprovedAt: want non-nil")
	}
	if stored.ApprovedAt.Before(before) || stored.ApprovedAt.After(after) {
		t.Errorf("ApprovedAt: %v outside expected window [%v, %v]", *stored.ApprovedAt, before, after)
	}
}

// ---------------------------------------------------------------------------
// S4 — updated_at advances to the approval timestamp.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_PersistsUpdatedAt(t *testing.T) {
	row := makeReviewExpectation("ge-4", 1)
	originalUpdatedAt := row.UpdatedAt
	svc, repo, _ := buildService(t, row)

	time.Sleep(10 * time.Millisecond)
	if _, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-4", 1, "approver"); err != nil {
		t.Fatalf("ApproveGovernanceExpectation: %v", err)
	}
	stored := repo.rows[expKey("ge-4", 1)]
	if !stored.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("UpdatedAt: %v not after original %v", stored.UpdatedAt, originalUpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// S5 — immutable fields unchanged after approval (mirrors the
// repository.Update lifecycle/audit-only contract).
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_ImmutableFieldsUnchanged(t *testing.T) {
	row := makeReviewExpectation("ge-5", 2)
	svc, repo, _ := buildService(t, row)

	if _, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-5", 2, "approver"); err != nil {
		t.Fatalf("ApproveGovernanceExpectation: %v", err)
	}
	stored := repo.rows[expKey("ge-5", 2)]

	checks := []struct {
		field, want, got string
	}{
		{"ScopeID", row.ScopeID, stored.ScopeID},
		{"RequiredSurfaceID", row.RequiredSurfaceID, stored.RequiredSurfaceID},
		{"Name", row.Name, stored.Name},
		{"Description", row.Description, stored.Description},
		{"BusinessOwner", row.BusinessOwner, stored.BusinessOwner},
		{"TechnicalOwner", row.TechnicalOwner, stored.TechnicalOwner},
		{"CreatedBy", row.CreatedBy, stored.CreatedBy},
		{"ConditionType", string(row.ConditionType), string(stored.ConditionType)},
		{"ScopeKind", string(row.ScopeKind), string(stored.ScopeKind)},
	}
	for _, c := range checks {
		if c.want != c.got {
			t.Errorf("%s: want %q, got %q", c.field, c.want, c.got)
		}
	}
	if string(row.ConditionPayload) != string(stored.ConditionPayload) {
		t.Errorf("ConditionPayload: want %s, got %s", row.ConditionPayload, stored.ConditionPayload)
	}
	if !stored.EffectiveDate.Equal(row.EffectiveDate) {
		t.Errorf("EffectiveDate: want %v, got %v", row.EffectiveDate, stored.EffectiveDate)
	}
}

// ---------------------------------------------------------------------------
// S6 — unknown logical ID returns ErrGovernanceExpectationNotFound.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_UnknownID_ReturnsNotFound(t *testing.T) {
	svc, _, _ := buildService(t)

	_, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-unknown", 1, "approver")
	if !errors.Is(err, approval.ErrGovernanceExpectationNotFound) {
		t.Errorf("want ErrGovernanceExpectationNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// S7 — known ID with unknown version returns ErrGovernanceExpectationNotFound.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_UnknownVersion_ReturnsNotFound(t *testing.T) {
	svc, _, _ := buildService(t, makeReviewExpectation("ge-7", 1))

	_, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-7", 99, "approver")
	if !errors.Is(err, approval.ErrGovernanceExpectationNotFound) {
		t.Errorf("want ErrGovernanceExpectationNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// S8–S11 — non-review states return ErrGovernanceExpectationNotInReview.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_AlreadyActive_ReturnsNotInReview(t *testing.T) {
	svc, _, _ := buildService(t, makeExpectationWithStatus("ge-8", 1, governanceexpectation.ExpectationStatusActive))
	_, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-8", 1, "approver")
	if !errors.Is(err, approval.ErrGovernanceExpectationNotInReview) {
		t.Errorf("want ErrGovernanceExpectationNotInReview, got %v", err)
	}
}

func TestApproveGovernanceExpectation_Deprecated_ReturnsNotInReview(t *testing.T) {
	svc, _, _ := buildService(t, makeExpectationWithStatus("ge-9", 1, governanceexpectation.ExpectationStatusDeprecated))
	_, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-9", 1, "approver")
	if !errors.Is(err, approval.ErrGovernanceExpectationNotInReview) {
		t.Errorf("want ErrGovernanceExpectationNotInReview, got %v", err)
	}
}

func TestApproveGovernanceExpectation_Retired_ReturnsNotInReview(t *testing.T) {
	svc, _, _ := buildService(t, makeExpectationWithStatus("ge-10", 1, governanceexpectation.ExpectationStatusRetired))
	_, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-10", 1, "approver")
	if !errors.Is(err, approval.ErrGovernanceExpectationNotInReview) {
		t.Errorf("want ErrGovernanceExpectationNotInReview, got %v", err)
	}
}

func TestApproveGovernanceExpectation_Draft_ReturnsNotInReview(t *testing.T) {
	// Draft is unreachable through normal flow but the lifecycle
	// constraint must catch it as a defence-in-depth case.
	svc, _, _ := buildService(t, makeExpectationWithStatus("ge-11", 1, governanceexpectation.ExpectationStatusDraft))
	_, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-11", 1, "approver")
	if !errors.Is(err, approval.ErrGovernanceExpectationNotInReview) {
		t.Errorf("want ErrGovernanceExpectationNotInReview, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// S12 — control-audit record emitted with correct shape.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_EmitsControlAuditRecord(t *testing.T) {
	svc, _, audit := buildService(t, makeReviewExpectation("ge-12", 3))

	if _, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-12", 3, "approver-x"); err != nil {
		t.Fatalf("ApproveGovernanceExpectation: %v", err)
	}
	if len(audit.appended) != 1 {
		t.Fatalf("want 1 control-audit record, got %d", len(audit.appended))
	}
	rec := audit.appended[0]
	if rec.Action != controlaudit.ActionGovernanceExpectationApproved {
		t.Errorf("Action: got %q, want governance_expectation.approved", rec.Action)
	}
	if rec.ResourceKind != controlaudit.ResourceKindGovernanceExpectation {
		t.Errorf("ResourceKind: got %q, want %q", rec.ResourceKind, controlaudit.ResourceKindGovernanceExpectation)
	}
	if rec.ResourceID != "ge-12" {
		t.Errorf("ResourceID: got %q, want ge-12", rec.ResourceID)
	}
	if rec.Actor != "approver-x" {
		t.Errorf("Actor: got %q, want approver-x", rec.Actor)
	}
	if rec.ResourceVersion == nil || *rec.ResourceVersion != 3 {
		t.Errorf("ResourceVersion: got %v, want 3", rec.ResourceVersion)
	}
}

// ---------------------------------------------------------------------------
// S13 — repository not configured returns a configuration error.
// ---------------------------------------------------------------------------

func TestApproveGovernanceExpectation_RepositoryNotConfigured_ReturnsError(t *testing.T) {
	// No WithExpectationRepository called → expectationRepo is nil.
	svc := approval.NewService(nil, approval.Policy{})

	_, err := svc.ApproveGovernanceExpectation(context.Background(), "ge-13", 1, "approver")
	if err == nil {
		t.Fatal("want error when expectation repository is not configured")
	}
}
