package governancemap

// read_service_seeded_test.go — full-stack regression tests that
// exercise the ReadService against the actual memory repositories
// after bootstrap.SeedDemo has run. This complements the stub-reader
// tests in read_service_test.go: those isolate aggregation rules,
// these isolate the seed → read pipeline so a regression in either
// surfaces here.
//
// The headline regression covered here is Phase 6's Authority node
// isolation finding: the Consumer Lending governance map must report
// non-zero authority counts because the seed links profile-v2-onboarding
// (active) to surf-v2-id-verify (under proc-consumer-onboarding under
// bs-consumer-lending), and grant-v2-onboarding (active) ties that
// profile to agent-v2-evaluator. After Phase 8's per-entity idempotency
// refactor, this property must hold on a fresh seed AND after a
// repeated seed AND after a partial-state repair seed.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/store/memory"
)

// newSeededReadService creates a real ReadService backed by memory
// repositories that have been seeded via bootstrap.SeedDemo. Returns
// the ReadService for read assertions.
func newSeededReadService(t *testing.T) *ReadService {
	t.Helper()
	store := memory.NewStore()
	repos, err := store.Repositories()
	if err != nil {
		t.Fatalf("memory.NewStore().Repositories(): %v", err)
	}
	if err := bootstrap.SeedDemo(context.Background(), repos); err != nil {
		t.Fatalf("bootstrap.SeedDemo: %v", err)
	}
	return NewReadService(
		repos.BusinessServices,
		repos.BusinessServiceRelationships,
		repos.BusinessServiceCapabilities,
		repos.Capabilities,
		repos.Processes,
		repos.Surfaces,
		repos.Profiles,
		repos.Grants,
		repos.AISystems,
		repos.AISystemVersions,
		repos.AISystemBindings,
	)
}

// TestGetGovernanceMap_SeededConsumerLending_AuthorityNonZero is the
// Phase 8 headline regression: after a fresh seed the Consumer Lending
// governance map must surface the onboarding profile/grant/agent.
//
// This is the test that would have failed against the pre-Phase-8
// global-anchor-guard seed if it had ever been run on a deployment
// where bs-consumer-lending was created before profile-v2-onboarding
// was added (the actual Azure scenario).
func TestGetGovernanceMap_SeededConsumerLending_AuthorityNonZero(t *testing.T) {
	svc := newSeededReadService(t)

	got, err := svc.GetGovernanceMap(context.Background(), "bs-consumer-lending")
	if err != nil {
		t.Fatalf("GetGovernanceMap(bs-consumer-lending): %v", err)
	}
	if got == nil {
		t.Fatal("GetGovernanceMap returned nil")
	}
	if got.AuthoritySummary == nil {
		t.Fatal("AuthoritySummary must be non-nil")
	}

	if got.AuthoritySummary.ActiveProfileCount != 1 {
		t.Errorf("ActiveProfileCount: want 1, got %d", got.AuthoritySummary.ActiveProfileCount)
	}
	if got.AuthoritySummary.ActiveGrantCount != 1 {
		t.Errorf("ActiveGrantCount: want 1, got %d", got.AuthoritySummary.ActiveGrantCount)
	}
	if got.AuthoritySummary.ActiveAgentCount != 1 {
		t.Errorf("ActiveAgentCount: want 1, got %d", got.AuthoritySummary.ActiveAgentCount)
	}

	// surf-v2-id-verify must carry profile_count == 1 so the frontend
	// can draw its surface→authority connector. This is the per-surface
	// pin that complements the AuthoritySummary aggregate.
	var idVerify *SurfaceNode
	for _, s := range got.Surfaces {
		if s.Surface != nil && s.Surface.ID == "surf-v2-id-verify" {
			idVerify = s
			break
		}
	}
	if idVerify == nil {
		t.Fatal("surf-v2-id-verify must appear in Consumer Lending governance map")
	}
	if idVerify.ProfileCount != 1 {
		t.Errorf("surf-v2-id-verify.profile_count: want 1, got %d", idVerify.ProfileCount)
	}
}

// TestGetGovernanceMap_SeededConsumerLending_RepeatedSeedStillNonZero
// pins the property under the per-entity idempotency contract: running
// SeedDemo twice (or any number of times) must NOT degrade the
// AuthoritySummary. A regression that accidentally Update'd existing
// rows (e.g. zeroed out a ProfileCount) would surface here.
func TestGetGovernanceMap_SeededConsumerLending_RepeatedSeedStillNonZero(t *testing.T) {
	store := memory.NewStore()
	repos, err := store.Repositories()
	if err != nil {
		t.Fatalf("memory.NewStore().Repositories(): %v", err)
	}
	if err := bootstrap.SeedDemo(context.Background(), repos); err != nil {
		t.Fatalf("first SeedDemo: %v", err)
	}
	if err := bootstrap.SeedDemo(context.Background(), repos); err != nil {
		t.Fatalf("second SeedDemo: %v", err)
	}

	svc := NewReadService(
		repos.BusinessServices,
		repos.BusinessServiceRelationships,
		repos.BusinessServiceCapabilities,
		repos.Capabilities,
		repos.Processes,
		repos.Surfaces,
		repos.Profiles,
		repos.Grants,
		repos.AISystems,
		repos.AISystemVersions,
		repos.AISystemBindings,
	)
	got, err := svc.GetGovernanceMap(context.Background(), "bs-consumer-lending")
	if err != nil {
		t.Fatalf("GetGovernanceMap: %v", err)
	}
	if got.AuthoritySummary.ActiveProfileCount != 1 ||
		got.AuthoritySummary.ActiveGrantCount != 1 ||
		got.AuthoritySummary.ActiveAgentCount != 1 {
		t.Errorf("AuthoritySummary after repeated seed: want all=1, got profiles=%d grants=%d agents=%d",
			got.AuthoritySummary.ActiveProfileCount,
			got.AuthoritySummary.ActiveGrantCount,
			got.AuthoritySummary.ActiveAgentCount)
	}
}
