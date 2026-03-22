package approval_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Fake repository
// ---------------------------------------------------------------------------

type fakeSurfaceRepo struct {
	items  map[string]*surface.DecisionSurface
	stored []*surface.DecisionSurface // captures Update calls in order
}

func newFakeRepo() *fakeSurfaceRepo {
	return &fakeSurfaceRepo{items: make(map[string]*surface.DecisionSurface)}
}

func (r *fakeSurfaceRepo) seed(s *surface.DecisionSurface) {
	r.items[s.ID] = s
}

func (r *fakeSurfaceRepo) FindLatestByID(_ context.Context, id string) (*surface.DecisionSurface, error) {
	return r.items[id], nil
}

func (r *fakeSurfaceRepo) Update(_ context.Context, s *surface.DecisionSurface) error {
	r.items[s.ID] = s
	r.stored = append(r.stored, s)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func reviewSurface(id string) *surface.DecisionSurface {
	now := time.Now().UTC()
	return &surface.DecisionSurface{
		ID:             id,
		Name:           "Test Surface",
		Status:         surface.SurfaceStatusReview,
		Version:        1,
		EffectiveFrom:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
		BusinessOwner:  "owner-1",
		TechnicalOwner: "tech-1",
		Domain:         "test",
	}
}

func activeSurface(id string) *surface.DecisionSurface {
	s := reviewSurface(id)
	s.Status = surface.SurfaceStatusActive
	return s
}

func adminApprover(id string) identity.Principal {
	return identity.Principal{
		ID:    id,
		Roles: []string{identity.RoleAdmin},
	}
}

// ---------------------------------------------------------------------------
// ApproveSurface tests
// ---------------------------------------------------------------------------

func TestApproveSurface_Success(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("payments.execute"))

	svc := approval.NewService(repo, approval.DefaultPolicy())
	submitter := identity.Principal{ID: "submitter-1"}
	approver := adminApprover("approver-admin")

	got, err := svc.ApproveSurface(context.Background(), "payments.execute", submitter, approver)
	if err != nil {
		t.Fatalf("ApproveSurface: unexpected error: %v", err)
	}

	if got.Status != surface.SurfaceStatusActive {
		t.Errorf("expected status active, got %q", got.Status)
	}
	if got.ApprovedBy != "approver-admin" {
		t.Errorf("expected ApprovedBy %q, got %q", "approver-admin", got.ApprovedBy)
	}
	if got.ApprovedAt == nil {
		t.Error("expected ApprovedAt to be set")
	}

	// Repository must have received the update.
	if len(repo.stored) != 1 {
		t.Fatalf("expected 1 Update call, got %d", len(repo.stored))
	}
}

func TestApproveSurface_RejectsDraft(t *testing.T) {
	repo := newFakeRepo()
	s := reviewSurface("loan.originate")
	s.Status = surface.SurfaceStatusDraft
	repo.seed(s)

	svc := approval.NewService(repo, approval.DefaultPolicy())
	_, err := svc.ApproveSurface(context.Background(), "loan.originate", identity.Principal{}, adminApprover("approver-1"))

	if !errors.Is(err, approval.ErrInvalidStatus) {
		t.Errorf("expected ErrInvalidStatus for draft surface, got: %v", err)
	}
}

func TestApproveSurface_RejectsActive(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(activeSurface("loan.originate"))

	svc := approval.NewService(repo, approval.DefaultPolicy())
	_, err := svc.ApproveSurface(context.Background(), "loan.originate", identity.Principal{}, adminApprover("approver-1"))

	if !errors.Is(err, approval.ErrInvalidStatus) {
		t.Errorf("expected ErrInvalidStatus for active surface, got: %v", err)
	}
}

func TestApproveSurface_RejectsDeprecated(t *testing.T) {
	repo := newFakeRepo()
	s := activeSurface("loan.originate")
	s.Status = surface.SurfaceStatusDeprecated
	repo.seed(s)

	svc := approval.NewService(repo, approval.DefaultPolicy())
	_, err := svc.ApproveSurface(context.Background(), "loan.originate", identity.Principal{}, adminApprover("approver-1"))

	if !errors.Is(err, approval.ErrInvalidStatus) {
		t.Errorf("expected ErrInvalidStatus for deprecated surface, got: %v", err)
	}
}

func TestApproveSurface_SurfaceNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := approval.NewService(repo, approval.DefaultPolicy())

	_, err := svc.ApproveSurface(context.Background(), "nonexistent", identity.Principal{}, adminApprover("approver-1"))

	if !errors.Is(err, approval.ErrSurfaceNotFound) {
		t.Errorf("expected ErrSurfaceNotFound, got: %v", err)
	}
}

func TestApproveSurface_ForbidsSelfReview(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("payments.execute"))

	svc := approval.NewService(repo, approval.DefaultPolicy())
	submitter := identity.Principal{ID: "user-1"}
	approver := identity.Principal{
		ID:    "user-1",
		Roles: []string{identity.RoleAdmin},
	}

	_, err := svc.ApproveSurface(context.Background(), "payments.execute", submitter, approver)

	if !errors.Is(err, approval.ErrApprovalForbidden) {
		t.Errorf("expected ErrApprovalForbidden for self-review, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeprecateSurface tests
// ---------------------------------------------------------------------------

func TestDeprecateSurface_Success(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(activeSurface("payments.execute"))

	svc := approval.NewService(repo, approval.DefaultPolicy())
	got, err := svc.DeprecateSurface(context.Background(), "payments.execute", "ops-admin", "superseded by payments.execute.v2", "payments.execute.v2")
	if err != nil {
		t.Fatalf("DeprecateSurface: unexpected error: %v", err)
	}

	if got.Status != surface.SurfaceStatusDeprecated {
		t.Errorf("expected status deprecated, got %q", got.Status)
	}
	if got.DeprecationReason != "superseded by payments.execute.v2" {
		t.Errorf("expected DeprecationReason set, got %q", got.DeprecationReason)
	}
	if got.SuccessorSurfaceID != "payments.execute.v2" {
		t.Errorf("expected SuccessorSurfaceID set, got %q", got.SuccessorSurfaceID)
	}

	if len(repo.stored) != 1 {
		t.Fatalf("expected 1 Update call, got %d", len(repo.stored))
	}
}

func TestDeprecateSurface_RejectsReview(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("payments.execute"))

	svc := approval.NewService(repo, approval.DefaultPolicy())
	_, err := svc.DeprecateSurface(context.Background(), "payments.execute", "ops-admin", "reason", "")

	if !errors.Is(err, approval.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition for review surface, got: %v", err)
	}
}

func TestDeprecateSurface_RejectsDraft(t *testing.T) {
	repo := newFakeRepo()
	s := reviewSurface("payments.execute")
	s.Status = surface.SurfaceStatusDraft
	repo.seed(s)

	svc := approval.NewService(repo, approval.DefaultPolicy())
	_, err := svc.DeprecateSurface(context.Background(), "payments.execute", "ops-admin", "reason", "")

	if !errors.Is(err, approval.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition for draft surface, got: %v", err)
	}
}

func TestDeprecateSurface_SurfaceNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := approval.NewService(repo, approval.DefaultPolicy())

	_, err := svc.DeprecateSurface(context.Background(), "nonexistent", "ops-admin", "reason", "")

	if !errors.Is(err, approval.ErrSurfaceNotFound) {
		t.Errorf("expected ErrSurfaceNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Full promotion lifecycle test
// ---------------------------------------------------------------------------

// TestLifecyclePromotion_ReviewToActiveToDeprecated verifies the complete
// governed promotion path: review → active → deprecated.
func TestLifecyclePromotion_ReviewToActiveToDeprecated(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("lending.originate"))

	svc := approval.NewService(repo, approval.DefaultPolicy())
	submitter := identity.Principal{ID: "submitter-1"}
	approver := adminApprover("approver-1")

	// Step 1: review → active
	active, err := svc.ApproveSurface(context.Background(), "lending.originate", submitter, approver)
	if err != nil {
		t.Fatalf("ApproveSurface: %v", err)
	}
	if active.Status != surface.SurfaceStatusActive {
		t.Fatalf("expected active, got %q", active.Status)
	}

	// Step 2: active → deprecated
	deprecated, err := svc.DeprecateSurface(context.Background(), "lending.originate", "approver-1", "replaced by v2", "lending.originate.v2")
	if err != nil {
		t.Fatalf("DeprecateSurface: %v", err)
	}
	if deprecated.Status != surface.SurfaceStatusDeprecated {
		t.Fatalf("expected deprecated, got %q", deprecated.Status)
	}
}
