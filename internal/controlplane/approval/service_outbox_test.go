package approval_test

// Outbox emission tests for the approval service.
//
// These unit tests verify that:
// - ApproveSurface emits a surface.approved outbox event on success.
// - DeprecateSurface emits a surface.deprecated outbox event on success.
// - Failed operations (ErrInvalidStatus, ErrApprovalForbidden, etc.) do not
//   produce outbox events.
// - When outbox is nil (NewService, not NewServiceWithOutbox), no panic occurs
//   and existing behaviour is preserved.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/outbox"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newOutboxRepo() *outbox.MemoryRepository {
	return outbox.NewMemoryRepository()
}

// ---------------------------------------------------------------------------
// ApproveSurface outbox tests
// ---------------------------------------------------------------------------

func TestApproveSurface_EmitsOutboxEvent_OnSuccess(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("payments.execute"))
	ob := newOutboxRepo()

	svc := approval.NewServiceWithOutbox(repo, approval.DefaultPolicy(), ob)
	submitter := identity.Principal{ID: "submitter-1"}
	approver := adminApprover("approver-admin")

	_, err := svc.ApproveSurface(context.Background(), "payments.execute", submitter, approver)
	if err != nil {
		t.Fatalf("ApproveSurface: unexpected error: %v", err)
	}

	events := ob.All(context.Background())
	if len(events) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != outbox.EventSurfaceApproved {
		t.Errorf("expected event_type %q, got %q", outbox.EventSurfaceApproved, ev.EventType)
	}
	if ev.AggregateType != "surface" {
		t.Errorf("expected aggregate_type %q, got %q", "surface", ev.AggregateType)
	}
	if ev.AggregateID != "payments.execute" {
		t.Errorf("expected aggregate_id %q, got %q", "payments.execute", ev.AggregateID)
	}
	if ev.Topic != "midas.surfaces" {
		t.Errorf("expected topic %q, got %q", "midas.surfaces", ev.Topic)
	}
	if ev.PublishedAt != nil {
		t.Error("expected published_at to be nil on new outbox event")
	}
}

func TestApproveSurface_NoOutboxEvent_OnFailure(t *testing.T) {
	repo := newFakeRepo()
	// Seed with draft — ApproveSurface will return ErrInvalidStatus.
	s := reviewSurface("payments.execute")
	s.Status = "draft"
	repo.seed(s)
	ob := newOutboxRepo()

	svc := approval.NewServiceWithOutbox(repo, approval.DefaultPolicy(), ob)
	_, err := svc.ApproveSurface(context.Background(), "payments.execute",
		identity.Principal{ID: "submitter-1"}, adminApprover("approver-1"))
	if err == nil {
		t.Fatal("expected error for draft surface, got nil")
	}

	events := ob.All(context.Background())
	if len(events) != 0 {
		t.Errorf("expected 0 outbox events on failure, got %d", len(events))
	}
}

func TestApproveSurface_NoOutboxEvent_WhenOutboxNil(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("payments.execute"))

	// NewService, not NewServiceWithOutbox — outbox is nil.
	svc := approval.NewService(repo, approval.DefaultPolicy())

	_, err := svc.ApproveSurface(context.Background(), "payments.execute",
		identity.Principal{ID: "submitter-1"}, adminApprover("approver-admin"))
	if err != nil {
		t.Fatalf("ApproveSurface with nil outbox: unexpected error: %v", err)
	}
	// No panic = existing behaviour preserved.
}

func TestApproveSurface_SelfReview_NoOutboxEvent(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("payments.execute"))
	ob := newOutboxRepo()

	svc := approval.NewServiceWithOutbox(repo, approval.DefaultPolicy(), ob)
	submitter := identity.Principal{ID: "user-1"}
	approver := identity.Principal{ID: "user-1", Roles: []string{identity.RoleAdmin}}

	_, err := svc.ApproveSurface(context.Background(), "payments.execute", submitter, approver)
	if err == nil {
		t.Fatal("expected ErrApprovalForbidden for self-review, got nil")
	}

	events := ob.All(context.Background())
	if len(events) != 0 {
		t.Errorf("expected 0 outbox events on forbidden approval, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// DeprecateSurface outbox tests
// ---------------------------------------------------------------------------

func TestDeprecateSurface_EmitsOutboxEvent_OnSuccess(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(activeSurface("payments.execute"))
	ob := newOutboxRepo()

	svc := approval.NewServiceWithOutbox(repo, approval.DefaultPolicy(), ob)
	_, err := svc.DeprecateSurface(context.Background(), "payments.execute",
		"ops-governance", "superseded by v2", "payments.execute.v2")
	if err != nil {
		t.Fatalf("DeprecateSurface: unexpected error: %v", err)
	}

	events := ob.All(context.Background())
	if len(events) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != outbox.EventSurfaceDeprecated {
		t.Errorf("expected event_type %q, got %q", outbox.EventSurfaceDeprecated, ev.EventType)
	}
	if ev.AggregateID != "payments.execute" {
		t.Errorf("expected aggregate_id %q, got %q", "payments.execute", ev.AggregateID)
	}
}

func TestDeprecateSurface_DeprecatedByPopulatedInEvent(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(activeSurface("payments.execute"))
	ob := newOutboxRepo()

	svc := approval.NewServiceWithOutbox(repo, approval.DefaultPolicy(), ob)
	_, err := svc.DeprecateSurface(context.Background(), "payments.execute",
		"ops-governance", "superseded by v2", "payments.execute.v2")
	if err != nil {
		t.Fatalf("DeprecateSurface: unexpected error: %v", err)
	}

	events := ob.All(context.Background())
	if len(events) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(events))
	}

	var payload outbox.SurfaceDeprecatedEvent
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.DeprecatedBy != "ops-governance" {
		t.Errorf("expected deprecated_by %q, got %q", "ops-governance", payload.DeprecatedBy)
	}
	if payload.SurfaceID != "payments.execute" {
		t.Errorf("expected surface_id %q, got %q", "payments.execute", payload.SurfaceID)
	}
}

func TestDeprecateSurface_NoOutboxEvent_OnFailure(t *testing.T) {
	repo := newFakeRepo()
	// Seed with review — DeprecateSurface will return ErrInvalidTransition.
	repo.seed(reviewSurface("payments.execute"))
	ob := newOutboxRepo()

	svc := approval.NewServiceWithOutbox(repo, approval.DefaultPolicy(), ob)
	_, err := svc.DeprecateSurface(context.Background(), "payments.execute", "ops-admin", "reason", "")
	if err == nil {
		t.Fatal("expected ErrInvalidTransition for review surface, got nil")
	}

	events := ob.All(context.Background())
	if len(events) != 0 {
		t.Errorf("expected 0 outbox events on failure, got %d", len(events))
	}
}

func TestDeprecateSurface_NoOutboxEvent_WhenOutboxNil(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(activeSurface("payments.execute"))

	svc := approval.NewService(repo, approval.DefaultPolicy())
	_, err := svc.DeprecateSurface(context.Background(), "payments.execute", "ops-admin", "reason", "")
	if err != nil {
		t.Fatalf("DeprecateSurface with nil outbox: unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Full lifecycle with outbox
// ---------------------------------------------------------------------------

func TestLifecycle_ReviewToDeprecated_OutboxEvents(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(reviewSurface("lending.originate"))
	ob := newOutboxRepo()

	svc := approval.NewServiceWithOutbox(repo, approval.DefaultPolicy(), ob)
	submitter := identity.Principal{ID: "submitter-1"}
	approver := adminApprover("approver-1")

	// Step 1: review → active
	if _, err := svc.ApproveSurface(context.Background(), "lending.originate", submitter, approver); err != nil {
		t.Fatalf("ApproveSurface: %v", err)
	}

	// Step 2: active → deprecated
	if _, err := svc.DeprecateSurface(context.Background(), "lending.originate", "approver-1", "replaced", ""); err != nil {
		t.Fatalf("DeprecateSurface: %v", err)
	}

	events := ob.All(context.Background())
	if len(events) != 2 {
		t.Fatalf("expected 2 outbox events (approved + deprecated), got %d", len(events))
	}
	if events[0].EventType != outbox.EventSurfaceApproved {
		t.Errorf("expected first event %q, got %q", outbox.EventSurfaceApproved, events[0].EventType)
	}
	if events[1].EventType != outbox.EventSurfaceDeprecated {
		t.Errorf("expected second event %q, got %q", outbox.EventSurfaceDeprecated, events[1].EventType)
	}
}
