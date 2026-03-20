package approval_test

// controlaudit_test.go — verifies that ApproveSurface and DeprecateSurface
// emit the correct control-plane audit records.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/store/memory"
)

func TestApprovalAudit_SurfaceApproved_EmitsRecord(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("audit-surf-1"))

	auditRepo := memory.NewControlAuditRepo()
	svc := approval.NewServiceWithAll(repo, approval.DefaultPolicy(), nil, auditRepo)

	submitter := identity.Principal{ID: "submitter-1"}
	approver := adminApprover("approver-1")

	ctx := context.Background()
	_, err := svc.ApproveSurface(ctx, "audit-surf-1", submitter, approver)
	if err != nil {
		t.Fatalf("ApproveSurface: %v", err)
	}

	records, err := auditRepo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	r := records[0]
	if r.Action != controlaudit.ActionSurfaceApproved {
		t.Errorf("Action: want %q, got %q", controlaudit.ActionSurfaceApproved, r.Action)
	}
	if r.Actor != "approver-1" {
		t.Errorf("Actor: want approver-1, got %q", r.Actor)
	}
	if r.ResourceID != "audit-surf-1" {
		t.Errorf("ResourceID: want audit-surf-1, got %q", r.ResourceID)
	}
	if r.ResourceVersion == nil || *r.ResourceVersion != 1 {
		t.Errorf("ResourceVersion: want 1, got %v", r.ResourceVersion)
	}
}

func TestApprovalAudit_SurfaceApproved_NotEmittedOnFailure(t *testing.T) {
	repo := newFakeRepo()
	// No surface seeded → ApproveSurface will return ErrSurfaceNotFound.

	auditRepo := memory.NewControlAuditRepo()
	svc := approval.NewServiceWithAll(repo, approval.DefaultPolicy(), nil, auditRepo)

	submitter := identity.Principal{ID: "submitter-1"}
	approver := adminApprover("approver-1")

	ctx := context.Background()
	_, err := svc.ApproveSurface(ctx, "does-not-exist", submitter, approver)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	records, _ := auditRepo.List(ctx, controlaudit.ListFilter{})
	if len(records) != 0 {
		t.Errorf("expected 0 audit records on failure, got %d", len(records))
	}
}

func TestApprovalAudit_SurfaceDeprecated_EmitsRecord(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(activeSurface("audit-surf-2"))

	auditRepo := memory.NewControlAuditRepo()
	svc := approval.NewServiceWithAll(repo, approval.DefaultPolicy(), nil, auditRepo)

	ctx := context.Background()
	_, err := svc.DeprecateSurface(ctx, "audit-surf-2", "ops-team", "replaced by v2", "audit-surf-3")
	if err != nil {
		t.Fatalf("DeprecateSurface: %v", err)
	}

	records, err := auditRepo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	r := records[0]
	if r.Action != controlaudit.ActionSurfaceDeprecated {
		t.Errorf("Action: want %q, got %q", controlaudit.ActionSurfaceDeprecated, r.Action)
	}
	if r.Actor != "ops-team" {
		t.Errorf("Actor: want ops-team, got %q", r.Actor)
	}
	if r.ResourceID != "audit-surf-2" {
		t.Errorf("ResourceID: want audit-surf-2, got %q", r.ResourceID)
	}
	if r.Metadata == nil {
		t.Fatal("expected non-nil metadata for surface.deprecated")
	}
	if r.Metadata.DeprecationReason != "replaced by v2" {
		t.Errorf("DeprecationReason: want 'replaced by v2', got %q", r.Metadata.DeprecationReason)
	}
	if r.Metadata.SuccessorSurfaceID != "audit-surf-3" {
		t.Errorf("SuccessorSurfaceID: want audit-surf-3, got %q", r.Metadata.SuccessorSurfaceID)
	}
}

func TestApprovalAudit_SurfaceDeprecated_NotEmittedOnFailure(t *testing.T) {
	repo := newFakeRepo()
	// Surface seeded in review state (cannot deprecate — must be active).
	repo.seed(reviewSurface("audit-surf-review"))

	auditRepo := memory.NewControlAuditRepo()
	svc := approval.NewServiceWithAll(repo, approval.DefaultPolicy(), nil, auditRepo)

	ctx := context.Background()
	_, err := svc.DeprecateSurface(ctx, "audit-surf-review", "ops", "outdated", "")
	if err == nil {
		t.Fatal("expected error for non-active surface, got nil")
	}

	records, _ := auditRepo.List(ctx, controlaudit.ListFilter{})
	if len(records) != 0 {
		t.Errorf("expected 0 audit records on failure, got %d", len(records))
	}
}

func TestApprovalAudit_NilRepo_NoOp(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("audit-surf-nil"))

	// controlAudit is nil — should not panic.
	svc := approval.NewServiceWithAll(repo, approval.DefaultPolicy(), nil, nil)

	submitter := identity.Principal{ID: "submitter-x"}
	approver := adminApprover("approver-x")

	ctx := context.Background()
	_, err := svc.ApproveSurface(ctx, "audit-surf-nil", submitter, approver)
	if err != nil {
		t.Fatalf("ApproveSurface with nil controlAudit: %v", err)
	}
	// No assertion on records — just verifying no panic.
}
