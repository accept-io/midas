package approval_test

// drift_approval_test.go — exercises DriftDefinition lifecycle methods
// on approval.Service (Drift-1e). Mirrors failmode_approval_test.go in
// shape: Service is constructed with no surface repo, no outbox, and a
// recording control-audit; the drift-definition repo is wired via the
// WithDriftDefinitionRepository builder.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/drift"
)

// fakeDriftDefinitionRepo is a minimal in-memory DriftDefinitionRepository
// for approval-service tests. Keyed by "id:version".
type fakeDriftDefinitionRepo struct {
	defs    map[string]*drift.DriftDefinition
	updated []*drift.DriftDefinition
}

func newFakeDriftDefinitionRepo(ds ...*drift.DriftDefinition) *fakeDriftDefinitionRepo {
	r := &fakeDriftDefinitionRepo{defs: map[string]*drift.DriftDefinition{}}
	for _, d := range ds {
		r.defs[ddKey(d.ID, d.Version)] = d
	}
	return r
}

func ddKey(id string, version int) string {
	return fmt.Sprintf("%s:%d", id, version)
}

func (f *fakeDriftDefinitionRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*drift.DriftDefinition, error) {
	d, ok := f.defs[ddKey(id, version)]
	if !ok {
		return nil, nil
	}
	cp := *d
	return &cp, nil
}

func (f *fakeDriftDefinitionRepo) Update(_ context.Context, d *drift.DriftDefinition) error {
	f.defs[ddKey(d.ID, d.Version)] = d
	f.updated = append(f.updated, d)
	return nil
}

// makeDDInStatus returns a structurally-valid DriftDefinition at the
// requested status. CreatedBy = "alice" by default; tests override
// before injection when exercising maker-checker.
func makeDDInStatus(id string, version int, status drift.DriftDefinitionStatus) *drift.DriftDefinition {
	now := time.Now().UTC().Add(-time.Hour)
	return &drift.DriftDefinition{
		ID:               id,
		Version:          version,
		Name:             "Drift " + id,
		Status:           status,
		EffectiveDate:    now,
		BusinessOwner:    "owner",
		TechnicalOwner:   "owner",
		TargetEntityKind: drift.TargetEntityKindDecisionSurface,
		TargetEntityID:   "surf-x",
		Origin:           drift.DriftOriginManual,
		Managed:          true,
		Metrics: []drift.DriftMetricDefinition{
			{
				MetricID:           "outcome-psi",
				DriftType:          drift.DriftTypeOutcome,
				BaselineStrategy:   drift.BaselineStrategyFixedGoverned,
				WindowSeconds:      3600,
				Cadence:            drift.CadenceHour,
				WarningThreshold:   0.10,
				BreachedThreshold:  0.20,
				ThresholdDirection: drift.ThresholdDirectionAscending,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "alice",
	}
}

func newDriftApprovalService(t *testing.T, repo approval.DriftDefinitionRepository, audit *recordingControlAudit) *approval.Service {
	t.Helper()
	svc := approval.NewServiceWithAll(nil, approval.DefaultPolicy(), nil, audit).
		WithDriftDefinitionRepository(repo)
	return svc
}

// ---------------------------------------------------------------------
// Submit
// ---------------------------------------------------------------------

func TestSubmitDriftDefinition_HappyPath(t *testing.T) {
	repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusDraft))
	audit := &recordingControlAudit{}
	svc := newDriftApprovalService(t, repo, audit)

	got, err := svc.SubmitDriftDefinition(context.Background(), "approve-rate-drift", 1, "operator", "ready")
	if err != nil {
		t.Fatalf("SubmitDriftDefinition: %v", err)
	}
	if got.Status != drift.DriftDefinitionStatusReview {
		t.Errorf("Status = %q, want review", got.Status)
	}
	if len(audit.records) != 1 {
		t.Errorf("audit records = %d, want 1", len(audit.records))
	}
}

func TestSubmitDriftDefinition_RejectsNonDraft(t *testing.T) {
	repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusActive))
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	_, err := svc.SubmitDriftDefinition(context.Background(), "approve-rate-drift", 1, "operator", "")
	if !errors.Is(err, approval.ErrDriftDefinitionNotInDraft) {
		t.Errorf("err = %v, want ErrDriftDefinitionNotInDraft", err)
	}
}

func TestSubmitDriftDefinition_NotFound(t *testing.T) {
	svc := newDriftApprovalService(t, newFakeDriftDefinitionRepo(), &recordingControlAudit{})
	_, err := svc.SubmitDriftDefinition(context.Background(), "missing", 1, "operator", "")
	if !errors.Is(err, approval.ErrDriftDefinitionNotFound) {
		t.Errorf("err = %v, want ErrDriftDefinitionNotFound", err)
	}
}

// ---------------------------------------------------------------------
// Approve
// ---------------------------------------------------------------------

func TestApproveDriftDefinition_HappyPath(t *testing.T) {
	d := makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusReview)
	d.CreatedBy = "alice"
	repo := newFakeDriftDefinitionRepo(d)
	audit := &recordingControlAudit{}
	svc := newDriftApprovalService(t, repo, audit)

	got, err := svc.ApproveDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob")
	if err != nil {
		t.Fatalf("ApproveDriftDefinition: %v", err)
	}
	if got.Status != drift.DriftDefinitionStatusActive {
		t.Errorf("Status = %q, want active", got.Status)
	}
	if got.ApprovedBy != "bob" {
		t.Errorf("ApprovedBy = %q, want bob", got.ApprovedBy)
	}
	if got.ApprovedAt == nil {
		t.Error("ApprovedAt nil; want non-nil")
	}
	if len(audit.records) != 1 {
		t.Errorf("audit records = %d, want 1", len(audit.records))
	}
}

func TestApproveDriftDefinition_MakerCannotApproveOwnRevision(t *testing.T) {
	d := makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusReview)
	d.CreatedBy = "alice"
	repo := newFakeDriftDefinitionRepo(d)
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	_, err := svc.ApproveDriftDefinition(context.Background(), "approve-rate-drift", 1, "alice")
	if !errors.Is(err, approval.ErrDriftDefinitionMakerChecker) {
		t.Errorf("err = %v, want ErrDriftDefinitionMakerChecker", err)
	}
}

func TestApproveDriftDefinition_MakerCheckerCaseInsensitive(t *testing.T) {
	d := makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusReview)
	d.CreatedBy = "Alice"
	repo := newFakeDriftDefinitionRepo(d)
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	_, err := svc.ApproveDriftDefinition(context.Background(), "approve-rate-drift", 1, "  ALICE ")
	if !errors.Is(err, approval.ErrDriftDefinitionMakerChecker) {
		t.Errorf("err = %v, want ErrDriftDefinitionMakerChecker (case-insensitive trim)", err)
	}
}

func TestApproveDriftDefinition_RejectsNonReview(t *testing.T) {
	repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusDraft))
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	_, err := svc.ApproveDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob")
	if !errors.Is(err, approval.ErrDriftDefinitionNotInReview) {
		t.Errorf("err = %v, want ErrDriftDefinitionNotInReview", err)
	}
}

func TestApproveDriftDefinition_PriorActiveNotAutoDeprecated(t *testing.T) {
	// Drift-1e mirrors FailModePolicy / Profile / GovernanceExpectation:
	// approving v2 does NOT auto-deprecate v1. This test pins that
	// deferred behaviour so a future tranche that adds atomic
	// auto-deprecation will surface as a deliberate test update.
	v1 := makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusActive)
	v2 := makeDDInStatus("approve-rate-drift", 2, drift.DriftDefinitionStatusReview)
	v2.CreatedBy = "alice"
	repo := newFakeDriftDefinitionRepo(v1, v2)
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	_, err := svc.ApproveDriftDefinition(context.Background(), "approve-rate-drift", 2, "bob")
	if err != nil {
		t.Fatalf("ApproveDriftDefinition v2: %v", err)
	}
	stillActive, _ := repo.FindByIDAndVersion(context.Background(), "approve-rate-drift", 1)
	if stillActive.Status != drift.DriftDefinitionStatusActive {
		t.Errorf("v1 status = %q, want active (Drift-1e does not auto-deprecate prior actives)", stillActive.Status)
	}
}

// ---------------------------------------------------------------------
// Reject
// ---------------------------------------------------------------------

func TestRejectDriftDefinition_HappyPath(t *testing.T) {
	repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusReview))
	audit := &recordingControlAudit{}
	svc := newDriftApprovalService(t, repo, audit)

	got, err := svc.RejectDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob", "thresholds need review")
	if err != nil {
		t.Fatalf("RejectDriftDefinition: %v", err)
	}
	if got.Status != drift.DriftDefinitionStatusDraft {
		t.Errorf("Status = %q, want draft", got.Status)
	}
	if got.ApprovedBy != "" {
		t.Errorf("ApprovedBy = %q, want empty (review→draft must not touch approval fields)", got.ApprovedBy)
	}
	if got.ApprovedAt != nil {
		t.Error("ApprovedAt non-nil; want nil")
	}
	if len(audit.records) != 1 {
		t.Errorf("audit records = %d, want 1", len(audit.records))
	}
}

func TestRejectDriftDefinition_RejectsNonReview(t *testing.T) {
	repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusActive))
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	_, err := svc.RejectDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob", "")
	if !errors.Is(err, approval.ErrDriftDefinitionNotInReview) {
		t.Errorf("err = %v, want ErrDriftDefinitionNotInReview", err)
	}
}

// ---------------------------------------------------------------------
// Deprecate
// ---------------------------------------------------------------------

func TestDeprecateDriftDefinition_HappyPath(t *testing.T) {
	repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusActive))
	audit := &recordingControlAudit{}
	svc := newDriftApprovalService(t, repo, audit)

	got, err := svc.DeprecateDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob", "replaced", "approve-rate-drift", 2)
	if err != nil {
		t.Fatalf("DeprecateDriftDefinition: %v", err)
	}
	if got.Status != drift.DriftDefinitionStatusDeprecated {
		t.Errorf("Status = %q, want deprecated", got.Status)
	}
	if got.SuccessorDefinitionID != "approve-rate-drift" {
		t.Errorf("SuccessorDefinitionID = %q", got.SuccessorDefinitionID)
	}
	if got.SuccessorVersion != 2 {
		t.Errorf("SuccessorVersion = %d, want 2", got.SuccessorVersion)
	}
}

func TestDeprecateDriftDefinition_RejectsNonActive(t *testing.T) {
	repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusReview))
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	_, err := svc.DeprecateDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob", "", "", 0)
	if !errors.Is(err, approval.ErrDriftDefinitionNotActive) {
		t.Errorf("err = %v, want ErrDriftDefinitionNotActive", err)
	}
}

func TestDeprecateDriftDefinition_PreservesMetrics(t *testing.T) {
	d := makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusActive)
	repo := newFakeDriftDefinitionRepo(d)
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	got, err := svc.DeprecateDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob", "", "", 0)
	if err != nil {
		t.Fatalf("DeprecateDriftDefinition: %v", err)
	}
	if len(got.Metrics) != 1 {
		t.Errorf("Metrics len = %d, want 1 (lifecycle endpoints must not mutate metrics)", len(got.Metrics))
	}
	if got.Metrics[0].WarningThreshold != 0.10 {
		t.Errorf("metric mutated: WarningThreshold = %v", got.Metrics[0].WarningThreshold)
	}
}

// ---------------------------------------------------------------------
// Retire
// ---------------------------------------------------------------------

func TestRetireDriftDefinition_FromEachAllowedSourceState(t *testing.T) {
	for _, src := range []drift.DriftDefinitionStatus{
		drift.DriftDefinitionStatusDraft,
		drift.DriftDefinitionStatusReview,
		drift.DriftDefinitionStatusActive,
		drift.DriftDefinitionStatusDeprecated,
	} {
		t.Run(string(src), func(t *testing.T) {
			repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, src))
			svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

			got, err := svc.RetireDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob", "")
			if err != nil {
				t.Fatalf("RetireDriftDefinition from %q: %v", src, err)
			}
			if got.Status != drift.DriftDefinitionStatusRetired {
				t.Errorf("Status = %q, want retired", got.Status)
			}
			if got.RetiredAt == nil {
				t.Error("RetiredAt nil; want non-nil")
			}
		})
	}
}

func TestRetireDriftDefinition_RejectsAlreadyRetired(t *testing.T) {
	repo := newFakeDriftDefinitionRepo(makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusRetired))
	svc := newDriftApprovalService(t, repo, &recordingControlAudit{})

	_, err := svc.RetireDriftDefinition(context.Background(), "approve-rate-drift", 1, "bob", "")
	if !errors.Is(err, approval.ErrDriftDefinitionAlreadyRetired) {
		t.Errorf("err = %v, want ErrDriftDefinitionAlreadyRetired", err)
	}
}

// ---------------------------------------------------------------------
// Audit-record content shape pin
// ---------------------------------------------------------------------

func TestDriftLifecycle_AuditRecordsContainExpectedActions(t *testing.T) {
	d := makeDDInStatus("approve-rate-drift", 1, drift.DriftDefinitionStatusDraft)
	d.CreatedBy = "alice"
	repo := newFakeDriftDefinitionRepo(d)
	audit := &recordingControlAudit{}
	svc := newDriftApprovalService(t, repo, audit)

	ctx := context.Background()
	if _, err := svc.SubmitDriftDefinition(ctx, "approve-rate-drift", 1, "operator", "ready"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := svc.ApproveDriftDefinition(ctx, "approve-rate-drift", 1, "bob"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := svc.DeprecateDriftDefinition(ctx, "approve-rate-drift", 1, "carol", "replaced", "", 0); err != nil {
		t.Fatalf("Deprecate: %v", err)
	}

	wantActions := []string{
		"drift_definition.submitted",
		"drift_definition.approved",
		"drift_definition.deprecated",
	}
	if len(audit.records) != len(wantActions) {
		t.Fatalf("audit records = %d, want %d", len(audit.records), len(wantActions))
	}
	for i, want := range wantActions {
		if !strings.HasPrefix(string(audit.records[i].Action), strings.TrimSuffix(want, ".submitted")) ||
			string(audit.records[i].Action) != want {
			t.Errorf("audit[%d].Action = %q, want %q", i, audit.records[i].Action, want)
		}
	}
}
