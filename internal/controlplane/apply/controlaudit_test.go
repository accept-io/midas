package apply_test

// controlaudit_test.go — verifies that applying resources emits the correct
// control-plane audit records into the controlaudit.Repository.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
)

// surfaceDoc builds a minimal valid ParsedDocument for a surface with the given id.
func surfaceDoc(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   id,
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Test Surface " + id},
			Spec: types.SurfaceSpec{
				Description: "test",
				Category:    "financial",
				RiskTier:    "high",
				Status:      "active",
			},
		},
	}
}

// agentDoc builds a minimal valid ParsedDocument for an agent with the given id.
func agentDoc(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindAgent,
		ID:   id,
		Doc: types.AgentDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindAgent,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Test Agent " + id},
			Spec: types.AgentSpec{
				Type: "llm_agent",
				Runtime: types.AgentRuntime{
					Model:    "gpt-4",
					Version:  "2024-11-20",
					Provider: "openai",
				},
				Status: "active",
			},
		},
	}
}

// profileDoc builds a minimal valid ParsedDocument for a profile referencing surfaceID.
func profileDoc(id, surfaceID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProfile,
		ID:   id,
		Doc: types.ProfileDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProfile,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Test Profile " + id},
			Spec: types.ProfileSpec{
				SurfaceID: surfaceID,
				Authority: types.ProfileAuthority{
					DecisionConfidenceThreshold: 0.80,
					ConsequenceThreshold: types.ConsequenceThreshold{
						Type:     "monetary",
						Amount:   5000,
						Currency: "USD",
					},
				},
				InputRequirements: types.ProfileInputRequirements{
					RequiredContext: []string{"customer_id"},
				},
				Policy: types.ProfilePolicy{
					Reference: "rego://test/v1",
					FailMode:  "closed",
				},
			},
		},
	}
}

// grantDoc builds a minimal valid ParsedDocument for a grant.
func grantDoc(id, agentID, profileID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindGrant,
		ID:   id,
		Doc: types.GrantDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindGrant,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Test Grant " + id},
			Spec: types.GrantSpec{
				AgentID:       agentID,
				ProfileID:     profileID,
				GrantedBy:     "system",
				GrantedAt:     "2025-01-01T00:00:00Z",
				EffectiveFrom: "2025-01-01T00:00:00Z",
				Status:        "active",
			},
		},
	}
}

func newAuditServiceWithRepos(repos *store.Repositories) *apply.Service {
	return apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:     repos.Surfaces,
		Agents:       repos.Agents,
		Profiles:     repos.Profiles,
		Grants:       repos.Grants,
		ControlAudit: repos.ControlAudit,
	})
}

func TestApplyAudit_SurfaceCreated(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newAuditServiceWithRepos(repos)
	ctx := context.Background()

	result := svc.Apply(ctx, []parser.ParsedDocument{surfaceDoc("audit-surf-1")}, "alice")
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d: %v", result.CreatedCount(), result.ValidationErrors)
	}

	audit, err := repos.ControlAudit.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(audit))
	}
	r := audit[0]
	if r.Action != controlaudit.ActionSurfaceCreated {
		t.Errorf("Action: want %q, got %q", controlaudit.ActionSurfaceCreated, r.Action)
	}
	if r.Actor != "alice" {
		t.Errorf("Actor: want alice, got %q", r.Actor)
	}
	if r.ResourceID != "audit-surf-1" {
		t.Errorf("ResourceID: want audit-surf-1, got %q", r.ResourceID)
	}
	if r.ResourceVersion == nil || *r.ResourceVersion != 1 {
		t.Errorf("ResourceVersion: want 1, got %v", r.ResourceVersion)
	}
}

func TestApplyAudit_AgentCreated(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newAuditServiceWithRepos(repos)
	ctx := context.Background()

	result := svc.Apply(ctx, []parser.ParsedDocument{agentDoc("audit-agent-1")}, "system")
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d", result.CreatedCount())
	}

	audit, err := repos.ControlAudit.List(ctx, controlaudit.ListFilter{Action: controlaudit.ActionAgentCreated})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(audit))
	}
	r := audit[0]
	if r.ResourceID != "audit-agent-1" {
		t.Errorf("ResourceID: want audit-agent-1, got %q", r.ResourceID)
	}
	if r.ResourceVersion != nil {
		t.Errorf("ResourceVersion: want nil for agents, got %v", r.ResourceVersion)
	}
}

func TestApplyAudit_ProfileCreatedV1(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newAuditServiceWithRepos(repos)
	ctx := context.Background()

	// Apply surface first so profile ref is satisfied.
	docs := []parser.ParsedDocument{
		surfaceDoc("audit-surf-for-profile"),
		profileDoc("audit-prof-1", "audit-surf-for-profile"),
	}

	result := svc.Apply(ctx, docs, "bob")
	if result.CreatedCount() != 2 {
		t.Fatalf("expected 2 created, got %d: %v", result.CreatedCount(), result.ValidationErrors)
	}

	audit, err := repos.ControlAudit.List(ctx, controlaudit.ListFilter{Action: controlaudit.ActionProfileCreated})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 profile.created record, got %d", len(audit))
	}
	r := audit[0]
	if r.Actor != "bob" {
		t.Errorf("Actor: want bob, got %q", r.Actor)
	}
	if r.ResourceVersion == nil || *r.ResourceVersion != 1 {
		t.Errorf("ResourceVersion: want 1, got %v", r.ResourceVersion)
	}
	if r.Metadata == nil || r.Metadata.SurfaceID != "audit-surf-for-profile" {
		t.Errorf("Metadata.SurfaceID: want audit-surf-for-profile, got %v", r.Metadata)
	}
}

func TestApplyAudit_ProfileVersioned(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newAuditServiceWithRepos(repos)
	ctx := context.Background()

	// First apply: create surface and profile v1.
	docs := []parser.ParsedDocument{
		surfaceDoc("audit-surf-versioned"),
		profileDoc("audit-prof-versioned", "audit-surf-versioned"),
	}
	r1 := svc.Apply(ctx, docs, "alice")
	if r1.CreatedCount() != 2 {
		t.Fatalf("first apply: expected 2 created, got %d: %v", r1.CreatedCount(), r1.ValidationErrors)
	}

	// Second apply: apply profile again → should create version 2.
	r2 := svc.Apply(ctx, []parser.ParsedDocument{profileDoc("audit-prof-versioned", "audit-surf-versioned")}, "alice")
	if r2.CreatedCount() != 1 {
		t.Fatalf("second apply: expected 1 created, got %d: %v", r2.CreatedCount(), r2.ValidationErrors)
	}

	// Only the second emit should be profile.versioned; the first is profile.created.
	versionedAudit, err := repos.ControlAudit.List(ctx, controlaudit.ListFilter{Action: controlaudit.ActionProfileVersioned})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(versionedAudit) != 1 {
		t.Fatalf("expected 1 profile.versioned record, got %d", len(versionedAudit))
	}
	if versionedAudit[0].ResourceVersion == nil || *versionedAudit[0].ResourceVersion != 2 {
		t.Errorf("ResourceVersion: want 2, got %v", versionedAudit[0].ResourceVersion)
	}
}

func TestApplyAudit_GrantCreated(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newAuditServiceWithRepos(repos)
	ctx := context.Background()

	docs := []parser.ParsedDocument{
		surfaceDoc("audit-surf-grant"),
		agentDoc("audit-agent-grant"),
		profileDoc("audit-prof-grant", "audit-surf-grant"),
		grantDoc("audit-grant-1", "audit-agent-grant", "audit-prof-grant"),
	}

	result := svc.Apply(ctx, docs, "ops")
	if result.CreatedCount() != 4 {
		t.Fatalf("expected 4 created, got %d: %v", result.CreatedCount(), result.ValidationErrors)
	}

	audit, err := repos.ControlAudit.List(ctx, controlaudit.ListFilter{Action: controlaudit.ActionGrantCreated})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 grant.created record, got %d", len(audit))
	}
	r := audit[0]
	if r.ResourceID != "audit-grant-1" {
		t.Errorf("ResourceID: want audit-grant-1, got %q", r.ResourceID)
	}
	if r.ResourceVersion != nil {
		t.Errorf("ResourceVersion: want nil for grants, got %v", r.ResourceVersion)
	}
}

func TestApplyAudit_ActorDefaultsToSystem(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newAuditServiceWithRepos(repos)
	ctx := context.Background()

	// Pass empty actor — should default to "system".
	result := svc.Apply(ctx, []parser.ParsedDocument{agentDoc("audit-agent-default-actor")}, "")
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d", result.CreatedCount())
	}

	audit, err := repos.ControlAudit.List(ctx, controlaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("expected 1 record, got %d", len(audit))
	}
	if audit[0].Actor != "system" {
		t.Errorf("Actor: want system, got %q", audit[0].Actor)
	}
}

func TestApplyAudit_NoRepoNoAudit(t *testing.T) {
	// Service without ControlAudit should not panic and no records should appear.
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces: repos.Surfaces,
		Agents:   repos.Agents,
		Profiles: repos.Profiles,
		Grants:   repos.Grants,
		// ControlAudit intentionally omitted.
	})
	ctx := context.Background()

	result := svc.Apply(ctx, []parser.ParsedDocument{agentDoc("agent-no-audit")}, "alice")
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d", result.CreatedCount())
	}
	// repos.ControlAudit still exists but was not wired into the service.
	audit, _ := repos.ControlAudit.List(ctx, controlaudit.ListFilter{})
	if len(audit) != 0 {
		t.Errorf("expected 0 audit records when ControlAudit repo not wired, got %d", len(audit))
	}
}
