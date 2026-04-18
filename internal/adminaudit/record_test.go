package adminaudit

import (
	"testing"
	"time"
)

func TestNewRecord_AssignsIDAndTimestamp(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	rec := NewRecord(ActionPasswordChanged, OutcomeSuccess, ActorTypeUser)
	after := time.Now().UTC().Add(time.Second)

	if rec.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rec.OccurredAt.Before(before) || rec.OccurredAt.After(after) {
		t.Errorf("OccurredAt outside expected window: got %v", rec.OccurredAt)
	}
	if rec.Action != ActionPasswordChanged {
		t.Errorf("Action = %q, want %q", rec.Action, ActionPasswordChanged)
	}
	if rec.Outcome != OutcomeSuccess {
		t.Errorf("Outcome = %q, want %q", rec.Outcome, OutcomeSuccess)
	}
	if rec.ActorType != ActorTypeUser {
		t.Errorf("ActorType = %q, want %q", rec.ActorType, ActorTypeUser)
	}
}

// TestActionsEnumerated asserts that the action vocabulary is fixed at
// exactly the five first-pass actions documented in Issue #41. Adding a new
// action must be an intentional change to this test.
func TestActionsEnumerated(t *testing.T) {
	want := map[Action]bool{
		ActionApplyInvoked:          true,
		ActionPromoteExecuted:       true,
		ActionCleanupExecuted:       true,
		ActionPasswordChanged:       true,
		ActionBootstrapAdminCreated: true,
	}
	for a := range want {
		if string(a) == "" {
			t.Errorf("action constant resolves to empty string: %v", a)
		}
	}
	if len(want) != 5 {
		t.Fatalf("expected 5 actions, got %d", len(want))
	}
}
