package approval_test

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/approval"
)

// mockGrantRepo is a minimal in-memory grant repository for unit tests.
type mockGrantRepo struct {
	items map[string]*authority.AuthorityGrant
}

func newMockGrantRepo() *mockGrantRepo {
	return &mockGrantRepo{items: make(map[string]*authority.AuthorityGrant)}
}

func (r *mockGrantRepo) FindByID(_ context.Context, id string) (*authority.AuthorityGrant, error) {
	g, ok := r.items[id]
	if !ok {
		return nil, nil
	}
	return g, nil
}

func (r *mockGrantRepo) Update(_ context.Context, g *authority.AuthorityGrant) error {
	r.items[g.ID] = g
	return nil
}

func activeGrant(id, agentID, profileID string) *authority.AuthorityGrant {
	return &authority.AuthorityGrant{
		ID:            id,
		AgentID:       agentID,
		ProfileID:     profileID,
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func suspendedGrant(id, agentID, profileID string) *authority.AuthorityGrant {
	now := time.Now()
	return &authority.AuthorityGrant{
		ID:            id,
		AgentID:       agentID,
		ProfileID:     profileID,
		Status:        authority.GrantStatusSuspended,
		EffectiveDate: now.Add(-time.Hour),
		SuspendedBy:   "admin-1",
		SuspendedAt:   &now,
		SuspendReason: "investigation",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func revokedGrant(id, agentID, profileID string) *authority.AuthorityGrant {
	now := time.Now()
	return &authority.AuthorityGrant{
		ID:            id,
		AgentID:       agentID,
		ProfileID:     profileID,
		Status:        authority.GrantStatusRevoked,
		EffectiveDate: now.Add(-time.Hour),
		RevokedBy:     "admin-1",
		RevokedAt:     &now,
		RevokeReason:  "policy violation",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// ---------------------------------------------------------------------------
// Suspend
// ---------------------------------------------------------------------------

func TestSuspendGrant_ActiveToSuspended_Success(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = activeGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	got, err := svc.SuspendGrant(context.Background(), "g1", "ops-user", "suspicious activity")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != authority.GrantStatusSuspended {
		t.Errorf("expected status suspended, got %s", got.Status)
	}
}

func TestSuspendGrant_CapturesActorAndReason(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = activeGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	got, err := svc.SuspendGrant(context.Background(), "g1", "ops-user", "suspicious activity")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SuspendedBy != "ops-user" {
		t.Errorf("expected suspended_by=ops-user, got %s", got.SuspendedBy)
	}
	if got.SuspendReason != "suspicious activity" {
		t.Errorf("expected reason, got %s", got.SuspendReason)
	}
	if got.SuspendedAt == nil {
		t.Error("expected suspended_at to be set")
	}
}

func TestSuspendGrant_NotActive_ReturnsError(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = suspendedGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	_, err := svc.SuspendGrant(context.Background(), "g1", "ops-user", "reason")
	if err == nil {
		t.Fatal("expected error for already suspended grant")
	}
}

func TestSuspendGrant_Revoked_ReturnsError(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = revokedGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	_, err := svc.SuspendGrant(context.Background(), "g1", "ops-user", "reason")
	if err == nil {
		t.Fatal("expected error for revoked grant")
	}
}

func TestSuspendGrant_NotFound_ReturnsError(t *testing.T) {
	repo := newMockGrantRepo()
	svc := approval.NewGrantService(repo)

	_, err := svc.SuspendGrant(context.Background(), "missing", "ops-user", "reason")
	if err == nil {
		t.Fatal("expected error for missing grant")
	}
}

// ---------------------------------------------------------------------------
// Revoke
// ---------------------------------------------------------------------------

func TestRevokeGrant_ActiveToRevoked_Success(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = activeGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	got, err := svc.RevokeGrant(context.Background(), "g1", "admin-1", "policy violation")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != authority.GrantStatusRevoked {
		t.Errorf("expected status revoked, got %s", got.Status)
	}
}

func TestRevokeGrant_SuspendedToRevoked_Success(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = suspendedGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	got, err := svc.RevokeGrant(context.Background(), "g1", "admin-1", "confirmed violation")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != authority.GrantStatusRevoked {
		t.Errorf("expected status revoked, got %s", got.Status)
	}
}

func TestRevokeGrant_CapturesActorAndReason(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = activeGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	got, err := svc.RevokeGrant(context.Background(), "g1", "admin-1", "policy violation")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RevokedBy != "admin-1" {
		t.Errorf("expected revoked_by=admin-1, got %s", got.RevokedBy)
	}
	if got.RevokeReason != "policy violation" {
		t.Errorf("expected reason, got %s", got.RevokeReason)
	}
	if got.RevokedAt == nil {
		t.Error("expected revoked_at to be set")
	}
}

func TestRevokeGrant_AlreadyRevoked_ReturnsError(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = revokedGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	_, err := svc.RevokeGrant(context.Background(), "g1", "admin-1", "reason")
	if err == nil {
		t.Fatal("expected error for already revoked grant")
	}
}

func TestRevokeGrant_NotFound_ReturnsError(t *testing.T) {
	repo := newMockGrantRepo()
	svc := approval.NewGrantService(repo)

	_, err := svc.RevokeGrant(context.Background(), "missing", "admin-1", "reason")
	if err == nil {
		t.Fatal("expected error for missing grant")
	}
}

// ---------------------------------------------------------------------------
// Reinstate
// ---------------------------------------------------------------------------

func TestReinstateGrant_SuspendedToActive_Success(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = suspendedGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	got, err := svc.ReinstateGrant(context.Background(), "g1", "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != authority.GrantStatusActive {
		t.Errorf("expected status active, got %s", got.Status)
	}
	// Suspension fields should be cleared
	if got.SuspendedBy != "" {
		t.Errorf("expected suspended_by cleared, got %s", got.SuspendedBy)
	}
	if got.SuspendedAt != nil {
		t.Error("expected suspended_at cleared")
	}
}

func TestReinstateGrant_NotSuspended_ReturnsError(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = activeGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	_, err := svc.ReinstateGrant(context.Background(), "g1", "admin-1")
	if err == nil {
		t.Fatal("expected error for active grant")
	}
}

func TestReinstateGrant_Revoked_ReturnsError(t *testing.T) {
	repo := newMockGrantRepo()
	repo.items["g1"] = revokedGrant("g1", "agent-1", "profile-1")
	svc := approval.NewGrantService(repo)

	_, err := svc.ReinstateGrant(context.Background(), "g1", "admin-1")
	if err == nil {
		t.Fatal("expected error for revoked grant")
	}
}
