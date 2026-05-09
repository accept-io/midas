package approval_test

// failmode_approval_test.go — exercises FailModePolicy lifecycle methods
// on approval.Service (D27j-impl-1c). Mirrors profile_approval_test.go's
// fake-repo pattern — Service is constructed with no surface repo, no
// outbox, no control audit, and the fail-mode-policy repo wired via the
// WithFailModePolicyRepository builder.
//
// The control-audit assertion uses a tiny stub that records appended
// records so happy-path tests can verify emission without depending on
// the controlaudit package's storage layer.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/failmode"
)

// fakeFailModePolicyRepo is a minimal in-memory FailModePolicyRepository
// for approval-service tests. Keyed by "id:version" string, mirroring
// fakeProfileRepo.
type fakeFailModePolicyRepo struct {
	policies map[string]*failmode.FailModePolicy
	updated  []*failmode.FailModePolicy
}

func newFakeFailModePolicyRepo(ps ...*failmode.FailModePolicy) *fakeFailModePolicyRepo {
	r := &fakeFailModePolicyRepo{policies: map[string]*failmode.FailModePolicy{}}
	for _, p := range ps {
		r.policies[fmpKey(p.ID, p.Version)] = p
	}
	return r
}

func fmpKey(id string, version int) string {
	return id + ":" + string(rune('0'+version))
}

func (f *fakeFailModePolicyRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*failmode.FailModePolicy, error) {
	p, ok := f.policies[fmpKey(id, version)]
	if !ok {
		return nil, nil
	}
	cp := *p
	return &cp, nil
}

func (f *fakeFailModePolicyRepo) Update(_ context.Context, p *failmode.FailModePolicy) error {
	f.policies[fmpKey(p.ID, p.Version)] = p
	f.updated = append(f.updated, p)
	return nil
}

// recordingControlAudit captures appended records so tests can assert on
// emission without coupling to controlaudit's persistent storage layer.
type recordingControlAudit struct {
	records []*controlaudit.ControlAuditRecord
}

func (r *recordingControlAudit) Append(_ context.Context, rec *controlaudit.ControlAuditRecord) error {
	r.records = append(r.records, rec)
	return nil
}

func (r *recordingControlAudit) List(_ context.Context, _ controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
	return r.records, nil
}

// makeFMPInStatus returns a structurally-valid FailModePolicy at the
// requested status. Only the fields the lifecycle code reads are populated.
func makeFMPInStatus(id string, version int, status failmode.FailModePolicyStatus) *failmode.FailModePolicy {
	return &failmode.FailModePolicy{
		ID:             id,
		Version:        version,
		Name:           id + " policy",
		Status:         status,
		EffectiveDate:  time.Now().UTC().Add(-time.Hour),
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Origin:         "manual",
		Managed:        true,
		CreatedAt:      time.Now().UTC().Add(-time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-time.Hour),
		CreatedBy:      "applier",
	}
}

func TestApproveFailModePolicy_ReviewToActive_Success(t *testing.T) {
	policy := makeFMPInStatus("default-fmp", 1, failmode.FailModePolicyStatusReview)
	repo := newFakeFailModePolicyRepo(policy)
	audit := &recordingControlAudit{}

	svc := approval.NewServiceWithAll(nil, approval.Policy{}, nil, audit).WithFailModePolicyRepository(repo)

	got, err := svc.ApproveFailModePolicy(context.Background(), "default-fmp", 1, "approver@example.com")
	if err != nil {
		t.Fatalf("ApproveFailModePolicy: unexpected error: %v", err)
	}
	if got.Status != failmode.FailModePolicyStatusActive {
		t.Errorf("Status: want active, got %q", got.Status)
	}
	if got.ApprovedBy != "approver@example.com" {
		t.Errorf("ApprovedBy: want approver@example.com, got %q", got.ApprovedBy)
	}
	if got.ApprovedAt == nil {
		t.Error("ApprovedAt: want non-nil")
	}
	if len(repo.updated) != 1 {
		t.Errorf("repo Update calls: want 1, got %d", len(repo.updated))
	}
	if len(audit.records) != 1 {
		t.Fatalf("audit records: want 1, got %d", len(audit.records))
	}
	rec := audit.records[0]
	if rec.Action != controlaudit.ActionFailModePolicyApproved {
		t.Errorf("audit action: want %q, got %q", controlaudit.ActionFailModePolicyApproved, rec.Action)
	}
	if rec.ResourceID != "default-fmp" {
		t.Errorf("audit resource_id: want %q, got %q", "default-fmp", rec.ResourceID)
	}
	if rec.Actor != "approver@example.com" {
		t.Errorf("audit actor: want %q, got %q", "approver@example.com", rec.Actor)
	}
}

func TestApproveFailModePolicy_NotFound(t *testing.T) {
	repo := newFakeFailModePolicyRepo()
	svc := approval.NewService(nil, approval.Policy{}).WithFailModePolicyRepository(repo)

	_, err := svc.ApproveFailModePolicy(context.Background(), "missing", 1, "approver")
	if !errors.Is(err, approval.ErrFailModePolicyNotFound) {
		t.Errorf("want ErrFailModePolicyNotFound, got %v", err)
	}
}

func TestApproveFailModePolicy_NotInReview_ReturnsConflictSentinel(t *testing.T) {
	for _, st := range []failmode.FailModePolicyStatus{
		failmode.FailModePolicyStatusActive,
		failmode.FailModePolicyStatusDeprecated,
		failmode.FailModePolicyStatusRetired,
	} {
		t.Run(string(st), func(t *testing.T) {
			policy := makeFMPInStatus("policy-x", 1, st)
			repo := newFakeFailModePolicyRepo(policy)
			svc := approval.NewService(nil, approval.Policy{}).WithFailModePolicyRepository(repo)

			_, err := svc.ApproveFailModePolicy(context.Background(), "policy-x", 1, "approver")
			if !errors.Is(err, approval.ErrFailModePolicyNotInReview) {
				t.Errorf("status=%q: want ErrFailModePolicyNotInReview, got %v", st, err)
			}
		})
	}
}

func TestApproveFailModePolicy_RepositoryNotConfigured(t *testing.T) {
	svc := approval.NewService(nil, approval.Policy{})
	_, err := svc.ApproveFailModePolicy(context.Background(), "policy-x", 1, "approver")
	if err == nil {
		t.Fatal("want error when repo not configured")
	}
}

func TestDeprecateFailModePolicy_ActiveToDeprecated_Success(t *testing.T) {
	policy := makeFMPInStatus("policy-x", 1, failmode.FailModePolicyStatusActive)
	repo := newFakeFailModePolicyRepo(policy)
	audit := &recordingControlAudit{}

	svc := approval.NewServiceWithAll(nil, approval.Policy{}, nil, audit).WithFailModePolicyRepository(repo)

	before := time.Now().UTC().Add(-time.Second)
	got, err := svc.DeprecateFailModePolicy(context.Background(), "policy-x", 1, "operator@example.com", "Superseded")
	if err != nil {
		t.Fatalf("DeprecateFailModePolicy: %v", err)
	}
	if got.Status != failmode.FailModePolicyStatusDeprecated {
		t.Errorf("Status: want deprecated, got %q", got.Status)
	}
	if !got.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt: want > %v, got %v", before, got.UpdatedAt)
	}
	if len(audit.records) != 1 {
		t.Fatalf("audit records: want 1, got %d", len(audit.records))
	}
	rec := audit.records[0]
	if rec.Action != controlaudit.ActionFailModePolicyDeprecated {
		t.Errorf("audit action: want %q, got %q", controlaudit.ActionFailModePolicyDeprecated, rec.Action)
	}
	if rec.Actor != "operator@example.com" {
		t.Errorf("audit actor: want operator@example.com, got %q", rec.Actor)
	}
	if rec.Metadata == nil || rec.Metadata.DeprecationReason != "Superseded" {
		t.Errorf("audit metadata.deprecation_reason: want Superseded, got %+v", rec.Metadata)
	}
}

func TestDeprecateFailModePolicy_EmptyReason_OmitsMetadata(t *testing.T) {
	policy := makeFMPInStatus("policy-x", 1, failmode.FailModePolicyStatusActive)
	repo := newFakeFailModePolicyRepo(policy)
	audit := &recordingControlAudit{}

	svc := approval.NewServiceWithAll(nil, approval.Policy{}, nil, audit).WithFailModePolicyRepository(repo)

	if _, err := svc.DeprecateFailModePolicy(context.Background(), "policy-x", 1, "operator", ""); err != nil {
		t.Fatalf("DeprecateFailModePolicy: %v", err)
	}
	rec := audit.records[0]
	if rec.Metadata != nil {
		t.Errorf("metadata: want nil for empty reason, got %+v", rec.Metadata)
	}
}

func TestDeprecateFailModePolicy_NotFound(t *testing.T) {
	repo := newFakeFailModePolicyRepo()
	svc := approval.NewService(nil, approval.Policy{}).WithFailModePolicyRepository(repo)

	_, err := svc.DeprecateFailModePolicy(context.Background(), "missing", 1, "operator", "reason")
	if !errors.Is(err, approval.ErrFailModePolicyNotFound) {
		t.Errorf("want ErrFailModePolicyNotFound, got %v", err)
	}
}

func TestDeprecateFailModePolicy_NotActive_ReturnsConflictSentinel(t *testing.T) {
	for _, st := range []failmode.FailModePolicyStatus{
		failmode.FailModePolicyStatusReview,
		failmode.FailModePolicyStatusDraft,
		failmode.FailModePolicyStatusDeprecated,
		failmode.FailModePolicyStatusRetired,
	} {
		t.Run(string(st), func(t *testing.T) {
			policy := makeFMPInStatus("policy-x", 1, st)
			repo := newFakeFailModePolicyRepo(policy)
			svc := approval.NewService(nil, approval.Policy{}).WithFailModePolicyRepository(repo)

			_, err := svc.DeprecateFailModePolicy(context.Background(), "policy-x", 1, "operator", "reason")
			if !errors.Is(err, approval.ErrFailModePolicyNotActive) {
				t.Errorf("status=%q: want ErrFailModePolicyNotActive, got %v", st, err)
			}
		})
	}
}

func TestDeprecateFailModePolicy_RepositoryNotConfigured(t *testing.T) {
	svc := approval.NewService(nil, approval.Policy{})
	_, err := svc.DeprecateFailModePolicy(context.Background(), "policy-x", 1, "operator", "reason")
	if err == nil {
		t.Fatal("want error when repo not configured")
	}
}
