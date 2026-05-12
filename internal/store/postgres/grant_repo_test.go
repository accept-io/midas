package postgres

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/value"
)

func TestGrantRepo_Create_GrantReasonRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	agentID := "tst-agent-gran-reason"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, type, owner, operational_state, capabilities, metadata, created_at, updated_at)
		VALUES ($1, 'test', 'ai', 'owner', 'active', '[]', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, agentID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE agent_id = $1`, agentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
	})

	repo, err := NewGrantRepo(db)
	if err != nil {
		t.Fatalf("NewGrantRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	g := &authority.AuthorityGrant{
		ID:            "grant-reason-rt-1",
		AgentID:       agentID,
		ProfileID:     "profile-1",
		GrantedBy:     "admin",
		GrantReason:   "business justification",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now.Add(-time.Hour),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE id = $1`, g.ID)
	})

	got, err := repo.FindByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected grant, got nil")
	}
	if got.GrantReason != "business justification" {
		t.Errorf("GrantReason: want %q, got %q", "business justification", got.GrantReason)
	}
	if got.Status != authority.GrantStatusActive {
		t.Errorf("Status: want active, got %s", got.Status)
	}
}

func TestGrantRepo_Update_SuspensionFieldsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	agentID := "tst-agent-susp"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, type, owner, operational_state, capabilities, metadata, created_at, updated_at)
		VALUES ($1, 'test', 'ai', 'owner', 'active', '[]', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, agentID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE agent_id = $1`, agentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
	})

	repo, err := NewGrantRepo(db)
	if err != nil {
		t.Fatalf("NewGrantRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	g := &authority.AuthorityGrant{
		ID:            "grant-susp-rt-1",
		AgentID:       agentID,
		ProfileID:     "profile-1",
		GrantedBy:     "admin",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now.Add(-time.Hour),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE id = $1`, g.ID)
	})

	suspendedAt := now
	g.Status = authority.GrantStatusSuspended
	g.SuspendedBy = "ops-user"
	g.SuspendedAt = &suspendedAt
	g.SuspendReason = "security investigation"
	g.UpdatedAt = now

	if err := repo.Update(ctx, g); err != nil {
		t.Fatalf("Update (suspend): %v", err)
	}

	got, err := repo.FindByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != authority.GrantStatusSuspended {
		t.Errorf("Status: want suspended, got %s", got.Status)
	}
	if got.SuspendedBy != "ops-user" {
		t.Errorf("SuspendedBy: want ops-user, got %q", got.SuspendedBy)
	}
	if got.SuspendedAt == nil {
		t.Error("SuspendedAt: want non-nil, got nil")
	}
	if got.SuspendReason != "security investigation" {
		t.Errorf("SuspendReason: want %q, got %q", "security investigation", got.SuspendReason)
	}
}

func TestGrantRepo_Update_RevocationFieldsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	agentID := "tst-agent-rev"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, type, owner, operational_state, capabilities, metadata, created_at, updated_at)
		VALUES ($1, 'test', 'ai', 'owner', 'active', '[]', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, agentID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE agent_id = $1`, agentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
	})

	repo, err := NewGrantRepo(db)
	if err != nil {
		t.Fatalf("NewGrantRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	g := &authority.AuthorityGrant{
		ID:            "grant-rev-rt-1",
		AgentID:       agentID,
		ProfileID:     "profile-1",
		GrantedBy:     "admin",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now.Add(-time.Hour),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE id = $1`, g.ID)
	})

	revokedAt := now
	g.Status = authority.GrantStatusRevoked
	g.RevokedBy = "admin-1"
	g.RevokedAt = &revokedAt
	g.RevokeReason = "policy violation"
	g.UpdatedAt = now

	if err := repo.Update(ctx, g); err != nil {
		t.Fatalf("Update (revoke): %v", err)
	}

	got, err := repo.FindByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != authority.GrantStatusRevoked {
		t.Errorf("Status: want revoked, got %s", got.Status)
	}
	if got.RevokedBy != "admin-1" {
		t.Errorf("RevokedBy: want admin-1, got %q", got.RevokedBy)
	}
	if got.RevokedAt == nil {
		t.Error("RevokedAt: want non-nil, got nil")
	}
	if got.RevokeReason != "policy violation" {
		t.Errorf("RevokeReason: want %q, got %q", "policy violation", got.RevokeReason)
	}
}

func TestGrantRepo_Update_ReinstateClears_SuspensionFields(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	agentID := "tst-agent-reinst"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, type, owner, operational_state, capabilities, metadata, created_at, updated_at)
		VALUES ($1, 'test', 'ai', 'owner', 'active', '[]', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, agentID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE agent_id = $1`, agentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
	})

	repo, err := NewGrantRepo(db)
	if err != nil {
		t.Fatalf("NewGrantRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	suspendedAt := now
	g := &authority.AuthorityGrant{
		ID:            "grant-reinst-rt-1",
		AgentID:       agentID,
		ProfileID:     "profile-1",
		GrantedBy:     "admin",
		Status:        authority.GrantStatusSuspended,
		SuspendedBy:   "ops-user",
		SuspendedAt:   &suspendedAt,
		SuspendReason: "investigation",
		EffectiveDate: now.Add(-time.Hour),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE id = $1`, g.ID)
	})

	// Reinstate: clear suspension fields, set status=active
	g.Status = authority.GrantStatusActive
	g.SuspendedBy = ""
	g.SuspendedAt = nil
	g.SuspendReason = ""
	g.UpdatedAt = now

	if err := repo.Update(ctx, g); err != nil {
		t.Fatalf("Update (reinstate): %v", err)
	}

	got, err := repo.FindByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != authority.GrantStatusActive {
		t.Errorf("Status: want active, got %s", got.Status)
	}
	if got.SuspendedBy != "" {
		t.Errorf("SuspendedBy: want empty, got %q", got.SuspendedBy)
	}
	if got.SuspendedAt != nil {
		t.Errorf("SuspendedAt: want nil, got %v", got.SuspendedAt)
	}
	if got.SuspendReason != "" {
		t.Errorf("SuspendReason: want empty, got %q", got.SuspendReason)
	}
}

// TestGrantRepo_Create_CapabilitiesAndConstraintsRoundTrip pins that the
// D31i JSONB columns survive an INSERT → SELECT round trip with full
// fidelity. Exercises every constraint kind so a future shape change
// (e.g. a new payload field) shows up here.
func TestGrantRepo_Create_CapabilitiesAndConstraintsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	agentID := "tst-agent-caps-constraints"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, type, owner, operational_state, capabilities, metadata, created_at, updated_at)
		VALUES ($1, 'test', 'ai', 'owner', 'active', '[]', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, agentID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE agent_id = $1`, agentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
	})

	repo, err := NewGrantRepo(db)
	if err != nil {
		t.Fatalf("NewGrantRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	winStart := now.Add(-time.Hour).Truncate(time.Millisecond)
	winEnd := now.Add(time.Hour).Truncate(time.Millisecond)

	want := &authority.AuthorityGrant{
		ID:            "grant-caps-rt-1",
		AgentID:       agentID,
		ProfileID:     "profile-1",
		GrantedBy:     "admin",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now.Add(-time.Hour),
		CreatedAt:     now,
		UpdatedAt:     now,
		Capabilities: []authority.Capability{
			authority.CapabilityRecommend,
			authority.CapabilityApprove,
			authority.CapabilityStop,
		},
		Constraints: []authority.Constraint{
			{Kind: authority.ConstraintKindConfidenceThresholdMin, MinConfidence: 0.85},
			{Kind: authority.ConstraintKindConsequenceThresholdMax, MaxConsequence: authority.Consequence{
				Type: value.ConsequenceTypeMonetary, Amount: 1000, Currency: "USD",
			}},
			{Kind: authority.ConstraintKindHumanOnly},
			{Kind: authority.ConstraintKindTimeWindow, StartTime: winStart, EndTime: winEnd},
		},
	}

	if err := repo.Create(ctx, want); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE id = $1`, want.ID)
	})

	got, err := repo.FindByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected grant, got nil")
	}

	if !reflect.DeepEqual(got.Capabilities, want.Capabilities) {
		t.Errorf("Capabilities mismatch:\nwant %#v\ngot  %#v", want.Capabilities, got.Capabilities)
	}
	if len(got.Constraints) != len(want.Constraints) {
		t.Fatalf("Constraints len: want %d, got %d", len(want.Constraints), len(got.Constraints))
	}
	for i, gc := range got.Constraints {
		wc := want.Constraints[i]
		if gc.Kind != wc.Kind {
			t.Errorf("Constraints[%d].Kind: want %q, got %q", i, wc.Kind, gc.Kind)
		}
		switch wc.Kind {
		case authority.ConstraintKindConfidenceThresholdMin:
			if gc.MinConfidence != wc.MinConfidence {
				t.Errorf("Constraints[%d].MinConfidence: want %v, got %v", i, wc.MinConfidence, gc.MinConfidence)
			}
		case authority.ConstraintKindConsequenceThresholdMax:
			if gc.MaxConsequence != wc.MaxConsequence {
				t.Errorf("Constraints[%d].MaxConsequence: want %+v, got %+v", i, wc.MaxConsequence, gc.MaxConsequence)
			}
		case authority.ConstraintKindTimeWindow:
			if !gc.StartTime.Equal(wc.StartTime) {
				t.Errorf("Constraints[%d].StartTime: want %v, got %v", i, wc.StartTime, gc.StartTime)
			}
			if !gc.EndTime.Equal(wc.EndTime) {
				t.Errorf("Constraints[%d].EndTime: want %v, got %v", i, wc.EndTime, gc.EndTime)
			}
		}
	}
}

// TestGrantRepo_Create_EmptyCapabilitiesAndConstraints pins the
// DEFAULT '[]' columns produce nil slices on read, matching the
// memory repo's zero-value shape so parity tests across the two
// implementations remain consistent.
func TestGrantRepo_Create_EmptyCapabilitiesAndConstraints(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	agentID := "tst-agent-caps-empty"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, type, owner, operational_state, capabilities, metadata, created_at, updated_at)
		VALUES ($1, 'test', 'ai', 'owner', 'active', '[]', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, agentID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE agent_id = $1`, agentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
	})

	repo, err := NewGrantRepo(db)
	if err != nil {
		t.Fatalf("NewGrantRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	g := &authority.AuthorityGrant{
		ID:            "grant-caps-empty-1",
		AgentID:       agentID,
		ProfileID:     "profile-1",
		GrantedBy:     "admin",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now.Add(-time.Hour),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM authority_grants WHERE id = $1`, g.ID)
	})

	got, err := repo.FindByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Capabilities != nil {
		t.Errorf("Capabilities: want nil, got %#v", got.Capabilities)
	}
	if got.Constraints != nil {
		t.Errorf("Constraints: want nil, got %#v", got.Constraints)
	}
}
