package postgres

// profile_apply_integration_test.go — end-to-end Profile apply against
// real Postgres. Skipped automatically when DATABASE_URL is not set.
//
// This test pins the contract that motivated profile_repo_test.go: a
// ProfileDocument applied through the control-plane apply path lands
// in authority_profiles with EscalationMode populated to a value that
// satisfies chk_profiles_escalation_mode. The in-memory apply tests
// in internal/controlplane/apply/profile_lifecycle_test.go use a Go
// map-backed stub repo and never exercise the schema CHECK
// constraints; this test does.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// profileApplyIntegrationBundle returns a small bundle: a
// BusinessService → Process → Surface chain so the Profile's
// referential-integrity check (spec.surface_id must resolve) passes,
// followed by the ProfileDocument under test. The Profile carries no
// escalation_mode (the YAML schema does not expose it today). The
// mapper must default the missing field; without that default the
// resulting AuthorityProfile.EscalationMode would be "" and Postgres
// would reject the row on chk_profiles_escalation_mode.
//
// Bundle shape mirrors atomicityIntegrationBundle so the chain is
// validated against the same code path used by the other Postgres
// integration tests.
func profileApplyIntegrationBundle() []parser.ParsedDocument {
	return []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-profile-apply-int",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-profile-apply-int", Name: "Profile Apply Integration Service"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-profile-apply-int",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-profile-apply-int", Name: "Profile Apply Integration Process"},
				Spec:       types.ProcessSpec{BusinessServiceID: "bs-profile-apply-int", Status: "active"},
			},
		},
		{
			Kind: types.KindSurface,
			ID:   "surf-profile-apply-int",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata:   types.DocumentMetadata{ID: "surf-profile-apply-int", Name: "Profile Apply Integration Surface"},
				Spec: types.SurfaceSpec{
					Category:  "integration",
					RiskTier:  "low",
					Status:    "active",
					ProcessID: "proc-profile-apply-int",
				},
			},
		},
		{
			Kind: types.KindProfile,
			ID:   "profile-apply-int-no-escalation",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata: types.DocumentMetadata{
					ID:   "profile-apply-int-no-escalation",
					Name: "Apply integration test (no escalation_mode in YAML)",
				},
				Spec: types.ProfileSpec{
					SurfaceID: "surf-profile-apply-int",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.85,
						ConsequenceThreshold: types.ConsequenceThreshold{
							Type:       "risk_rating",
							RiskRating: "high",
						},
					},
					Policy: types.ProfilePolicy{
						Reference: "rego://test/apply-int",
						FailMode:  "closed",
					},
					Lifecycle: types.ProfileLifecycle{
						Status:        "active",
						EffectiveFrom: "2026-01-01T00:00:00Z",
						Version:       1,
					},
				},
			},
		},
	}
}

func cleanupProfileApplyIntegrationRows(t *testing.T, db *sql.DB) {
	t.Helper()
	// Children before parents to keep FK constraints happy.
	for _, stmt := range []string{
		`DELETE FROM authority_profiles WHERE id = 'profile-apply-int-no-escalation'`,
		`DELETE FROM decision_surfaces WHERE id = 'surf-profile-apply-int'`,
		`DELETE FROM processes WHERE process_id = 'proc-profile-apply-int'`,
		`DELETE FROM business_services WHERE business_service_id = 'bs-profile-apply-int'`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("cleanup %q: %v", stmt, err)
		}
	}
}

// TestApplyProfile_PostgresPersistsWithEscalationModeAuto applies a
// ProfileDocument bundle against real Postgres and asserts that the
// row was persisted with status=review and escalation_mode=auto. The
// YAML carries no escalation_mode field — the assertion proves the
// mapper-side default reaches the storage layer.
func TestApplyProfile_PostgresPersistsWithEscalationModeAuto(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() {
		cleanupProfileApplyIntegrationRows(t, db)
		db.Close()
	})
	cleanupProfileApplyIntegrationRows(t, db)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:         repos.Surfaces,
		Processes:        repos.Processes,
		Capabilities:     repos.Capabilities,
		BusinessServices: repos.BusinessServices,
		Agents:           repos.Agents,
		Profiles:         repos.Profiles,
		Grants:           repos.Grants,
		ControlAudit:     repos.ControlAudit,
		// No Tx runner — apply runs without postgres.NewApplyTxRunner.
		// The Profile path is single-document so atomicity is not the
		// concern under test; correctness of CHECK-constraint
		// satisfaction is.
	})

	ctx := context.Background()
	result := svc.Apply(ctx, profileApplyIntegrationBundle(), "profile-apply-integration")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("validation errors: %+v", result.ValidationErrors)
	}
	if result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: %+v", result.Results)
	}
	if result.CreatedCount() != 4 {
		// BusinessService + Process + Surface + Profile.
		t.Fatalf("CreatedCount: want 4, got %d (result=%+v)", result.CreatedCount(), result)
	}

	repo, err := NewProfileRepo(db)
	if err != nil {
		t.Fatalf("NewProfileRepo: %v", err)
	}
	got, err := repo.FindByIDAndVersion(ctx, "profile-apply-int-no-escalation", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil {
		t.Fatal("profile not persisted")
	}

	if got.EscalationMode != authority.EscalationModeAuto {
		t.Errorf("EscalationMode: want %q, got %q (mapper default did not reach Postgres)",
			authority.EscalationModeAuto, got.EscalationMode)
	}
	if got.Status != authority.ProfileStatusReview {
		t.Errorf("Status: want %q (apply lands in review), got %q",
			authority.ProfileStatusReview, got.Status)
	}
	if got.FailMode != authority.FailModeClosed {
		t.Errorf("FailMode: want %q, got %q",
			authority.FailModeClosed, got.FailMode)
	}
}
