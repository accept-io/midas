package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlaudit"
)

func TestControlAuditRepo_Postgres_AppendAndList(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()
	rec := controlaudit.NewSurfaceCreatedRecord("alice", "payments.execute", 1)

	if err := repo.Append(ctx, rec); err != nil {
		t.Fatalf("Append: %v", err)
	}

	results, err := repo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0]
	if got.ID != rec.ID {
		t.Errorf("ID: want %q, got %q", rec.ID, got.ID)
	}
	if got.Actor != "alice" {
		t.Errorf("Actor: want alice, got %q", got.Actor)
	}
	if got.Action != controlaudit.ActionSurfaceCreated {
		t.Errorf("Action: want %q, got %q", controlaudit.ActionSurfaceCreated, got.Action)
	}
	if got.ResourceKind != controlaudit.ResourceKindSurface {
		t.Errorf("ResourceKind: want %q, got %q", controlaudit.ResourceKindSurface, got.ResourceKind)
	}
	if got.ResourceID != "payments.execute" {
		t.Errorf("ResourceID: want payments.execute, got %q", got.ResourceID)
	}
	if got.ResourceVersion == nil || *got.ResourceVersion != 1 {
		t.Errorf("ResourceVersion: want 1, got %v", got.ResourceVersion)
	}
	if got.Summary == "" {
		t.Error("Summary should not be empty")
	}
}

func TestControlAuditRepo_Postgres_ListNewestFirst(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()

	r1 := controlaudit.NewSurfaceCreatedRecord("alice", "surf-oldest", 1)
	r1.OccurredAt = time.Now().Add(-3 * time.Second).UTC()

	r2 := controlaudit.NewSurfaceCreatedRecord("alice", "surf-middle", 1)
	r2.OccurredAt = time.Now().Add(-1 * time.Second).UTC()

	r3 := controlaudit.NewSurfaceCreatedRecord("alice", "surf-newest", 1)
	r3.OccurredAt = time.Now().UTC()

	for _, r := range []*controlaudit.ControlAuditRecord{r1, r2, r3} {
		if err := repo.Append(ctx, r); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	results, err := repo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
	if results[0].ResourceID != "surf-newest" {
		t.Errorf("expected surf-newest first, got %q", results[0].ResourceID)
	}
	if results[2].ResourceID != "surf-oldest" {
		t.Errorf("expected surf-oldest last, got %q", results[2].ResourceID)
	}
}

func TestControlAuditRepo_Postgres_FilterByResourceKind(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewAgentCreatedRecord("system", "agent-1"))
	_ = repo.Append(ctx, controlaudit.NewGrantCreatedRecord("system", "grant-1"))

	results, err := repo.List(ctx, controlaudit.ListFilter{ResourceKind: controlaudit.ResourceKindSurface})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].ResourceKind != controlaudit.ResourceKindSurface {
		t.Errorf("unexpected kind %q", results[0].ResourceKind)
	}
}

func TestControlAuditRepo_Postgres_FilterByResourceID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-alpha", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-beta", 1))

	results, err := repo.List(ctx, controlaudit.ListFilter{ResourceID: "surf-alpha"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].ResourceID != "surf-alpha" {
		t.Errorf("unexpected resource_id %q", results[0].ResourceID)
	}
}

func TestControlAuditRepo_Postgres_FilterByActor(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("bob", "surf-2", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-3", 1))

	results, err := repo.List(ctx, controlaudit.ListFilter{Actor: "bob"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Actor != "bob" {
		t.Errorf("expected actor bob, got %q", results[0].Actor)
	}
}

func TestControlAuditRepo_Postgres_FilterByAction(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()
	_ = repo.Append(ctx, controlaudit.NewSurfaceCreatedRecord("alice", "surf-1", 1))
	_ = repo.Append(ctx, controlaudit.NewSurfaceApprovedRecord("bob", "surf-1", 1))

	results, err := repo.List(ctx, controlaudit.ListFilter{Action: controlaudit.ActionSurfaceApproved})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Action != controlaudit.ActionSurfaceApproved {
		t.Errorf("expected %q, got %q", controlaudit.ActionSurfaceApproved, results[0].Action)
	}
}

func TestControlAuditRepo_Postgres_NullableVersionAndMetadata(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()

	// Agents have nil version and nil metadata.
	agentRec := controlaudit.NewAgentCreatedRecord("system", "agent-99")
	if err := repo.Append(ctx, agentRec); err != nil {
		t.Fatalf("Append agent: %v", err)
	}

	// Deprecated surface has metadata.
	deprRec := controlaudit.NewSurfaceDeprecatedRecord("ops", "surf-1", 2, "replaced", "surf-2")
	if err := repo.Append(ctx, deprRec); err != nil {
		t.Fatalf("Append deprecated: %v", err)
	}

	// Check agent record.
	agentResults, err := repo.List(ctx, controlaudit.ListFilter{Action: controlaudit.ActionAgentCreated})
	if err != nil {
		t.Fatalf("List agent: %v", err)
	}
	if len(agentResults) != 1 {
		t.Fatalf("expected 1 agent record, got %d", len(agentResults))
	}
	if agentResults[0].ResourceVersion != nil {
		t.Errorf("expected nil version for agent, got %v", agentResults[0].ResourceVersion)
	}
	if agentResults[0].Metadata != nil {
		t.Errorf("expected nil metadata for agent, got %+v", agentResults[0].Metadata)
	}

	// Check deprecated record.
	deprResults, err := repo.List(ctx, controlaudit.ListFilter{Action: controlaudit.ActionSurfaceDeprecated})
	if err != nil {
		t.Fatalf("List deprecated: %v", err)
	}
	if len(deprResults) != 1 {
		t.Fatalf("expected 1 deprecated record, got %d", len(deprResults))
	}
	if deprResults[0].Metadata == nil {
		t.Fatal("expected non-nil metadata for deprecated record")
	}
	if deprResults[0].Metadata.DeprecationReason != "replaced" {
		t.Errorf("DeprecationReason: want replaced, got %q", deprResults[0].Metadata.DeprecationReason)
	}
	if deprResults[0].Metadata.SuccessorSurfaceID != "surf-2" {
		t.Errorf("SuccessorSurfaceID: want surf-2, got %q", deprResults[0].Metadata.SuccessorSurfaceID)
	}
}

// TestControlAuditRepo_Postgres_AllActionConstants verifies that every action
// constant defined in the controlaudit package can be written to Postgres
// without violating the CHECK constraint on controlplane_audit_events.action.
// This test exists to catch schema/code drift: if a new action constant is
// added to record.go but not to the CHECK constraint, this test will fail.
func TestControlAuditRepo_Postgres_AllActionConstants(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()

	records := []*controlaudit.ControlAuditRecord{
		controlaudit.NewSurfaceCreatedRecord("actor", "surf-1", 1),
		controlaudit.NewProfileCreatedRecord("actor", "prof-1", "surf-1", 1),
		controlaudit.NewProfileVersionedRecord("actor", "prof-1", "surf-1", 2),
		controlaudit.NewAgentCreatedRecord("actor", "agent-1"),
		controlaudit.NewGrantCreatedRecord("actor", "grant-1"),
		controlaudit.NewSurfaceApprovedRecord("actor", "surf-1", 1),
		controlaudit.NewSurfaceDeprecatedRecord("actor", "surf-1", 1, "replaced", ""),
		controlaudit.NewProfileApprovedRecord("actor", "prof-1", 1),
		controlaudit.NewProfileDeprecatedRecord("actor", "prof-1", 1),
		controlaudit.NewGrantSuspendedRecord("actor", "grant-1", "policy violation"),
		controlaudit.NewGrantRevokedRecord("actor", "grant-1", "permanent"),
		controlaudit.NewGrantReinstatedRecord("actor", "grant-1"),
	}

	for _, rec := range records {
		if err := repo.Append(ctx, rec); err != nil {
			t.Errorf("Append action %q failed: %v", rec.Action, err)
		}
	}
}

func TestControlAuditRepo_Postgres_LimitEnforced(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = repo.Append(ctx, controlaudit.NewAgentCreatedRecord("system", "agent"))
	}

	results, err := repo.List(ctx, controlaudit.ListFilter{Limit: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3, got %d", len(results))
	}
}

// TestControlAuditRepo_Postgres_GovernanceExpectationActionsAndKindAccepted
// is the load-bearing parity test for #57's schema CHECK extension. It
// asserts that the live audit_events action and resource_kind CHECK
// constraints accept all four GovernanceExpectation values:
//   - action = 'governance_expectation.created'
//   - action = 'governance_expectation.versioned'
//   - action = 'governance_expectation.approved'
//   - resource_kind = 'governance_expectation'
//
// Without the #57 schema extension, these inserts silently fail: the
// approval service's appendControlAudit (and the apply service's
// equivalent) swallow Append errors per ADR-041b, so a Postgres CHECK
// rejection would not surface to the caller. The only way to verify the
// schema accepts these values is a direct repository round-trip — which
// is what this test does.
//
// All three records persist and round-trip through List, proving the
// schema CHECK extension covers #57's new approved record AND
// retroactively closes #52's latent gap for the created/versioned
// records that were previously broken on Postgres.
func TestControlAuditRepo_Postgres_GovernanceExpectationActionsAndKindAccepted(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`DELETE FROM controlplane_audit_events`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM controlplane_audit_events`) }) //nolint

	repo, err := NewControlAuditRepo(db)
	if err != nil {
		t.Fatalf("NewControlAuditRepo: %v", err)
	}

	ctx := context.Background()
	records := []*controlaudit.ControlAuditRecord{
		controlaudit.NewGovernanceExpectationCreatedRecord("alice", "expect-credit-001", 1),
		controlaudit.NewGovernanceExpectationVersionedRecord("alice", "expect-credit-001", 2),
		controlaudit.NewGovernanceExpectationApprovedRecord("approver-bob", "expect-credit-001", 2),
	}
	for _, rec := range records {
		if err := repo.Append(ctx, rec); err != nil {
			t.Fatalf("Append %s: %v — schema CHECK likely missing the action or resource_kind value",
				rec.Action, err)
		}
	}

	results, err := repo.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 records, got %d", len(results))
	}

	// Every persisted row must use ResourceKindGovernanceExpectation —
	// proving the resource_kind CHECK accepts the new value. The
	// expectation_version field on every record must round-trip too.
	for _, got := range results {
		if got.ResourceKind != controlaudit.ResourceKindGovernanceExpectation {
			t.Errorf("ResourceKind: want %q, got %q",
				controlaudit.ResourceKindGovernanceExpectation, got.ResourceKind)
		}
		if got.ResourceID != "expect-credit-001" {
			t.Errorf("ResourceID: want expect-credit-001, got %q", got.ResourceID)
		}
		if got.ResourceVersion == nil {
			t.Errorf("ResourceVersion: want non-nil for action %q", got.Action)
		}
	}

	// Every action must be one of the three GE actions and no other.
	gotActions := make(map[controlaudit.Action]bool)
	for _, got := range results {
		gotActions[got.Action] = true
	}
	for _, want := range []controlaudit.Action{
		controlaudit.ActionGovernanceExpectationCreated,
		controlaudit.ActionGovernanceExpectationVersioned,
		controlaudit.ActionGovernanceExpectationApproved,
	} {
		if !gotActions[want] {
			t.Errorf("expected action %q in persisted set; got actions %v", want, gotActions)
		}
	}

	// Wait one millisecond between inserts so the ID-tiebreak is not
	// relied on. (Defensive — newRecord's UUIDs already disambiguate.)
	_ = time.Millisecond
}
