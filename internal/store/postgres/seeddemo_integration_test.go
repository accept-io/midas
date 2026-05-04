package postgres

// seeddemo_integration_test.go — Postgres regression for the Phase 8
// follow-up: bootstrap.SeedDemo must be able to insert ALL its seeded
// rows into a real Postgres schema, including the authority profiles
// that previously triggered chk_profiles_consequence_type with
// consequence_type='monetary'.
//
// The Phase 8 demo_test.go in internal/bootstrap proves the seed is
// per-entity self-healing against the in-memory backend, but the memory
// repos do not enforce the Postgres CHECK constraints. This test closes
// that gap by running the actual SeedDemo against a real schema.
//
// Gated on DATABASE_URL via openTestDB; the suite skips when not set.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/bootstrap"
)

// cleanupSeedDemoRows wipes every table SeedDemo writes to, in FK-safe
// order. The seed is idempotent so a follow-up call is a no-op, but a
// previous run's rows could fail later assertions (e.g. a surviving
// row from an old monetary-typed Postgres deployment) — wipe up front.
func cleanupSeedDemoRows(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()

	stmts := []string{
		// Children before parents.
		`DELETE FROM authority_grants WHERE id IN ('grant-v2-standard', 'grant-v2-onboarding')`,
		`DELETE FROM authority_profiles WHERE id IN ('profile-v2-standard', 'profile-v2-onboarding')`,
		`DELETE FROM agents WHERE id = 'agent-v2-evaluator'`,
		`DELETE FROM decision_surfaces WHERE id IN (
			'surf-v2-id-verify','surf-v2-consumer-fraud','surf-v2-credit-assess',
			'surf-v2-merchant-risk','surf-v2-merchant-payment','surf-v2-merchant-hv-pay'
		)`,
		`DELETE FROM business_service_capabilities WHERE business_service_id IN ('bs-consumer-lending','bs-merchant-services')`,
		`DELETE FROM processes WHERE process_id IN (
			'proc-consumer-onboarding','proc-credit-assessment',
			'proc-merchant-risk-screen','proc-merchant-payment-auth'
		)`,
		`DELETE FROM capabilities WHERE capability_id IN (
			'cap-identity-verification','cap-credit-scoring',
			'cap-fraud-detection','cap-payment-authorization'
		)`,
		`DELETE FROM business_services WHERE business_service_id IN ('bs-consumer-lending','bs-merchant-services')`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("cleanup %q: %v", s, err)
		}
	}
}

// TestSeedDemo_PostgresEndToEnd runs bootstrap.SeedDemo against a real
// Postgres schema and asserts:
//
//  1. The seed completes without error — in particular, the authority
//     profile inserts no longer violate chk_profiles_consequence_type.
//  2. Both seeded profiles round-trip with consequence_type='risk_rating'
//     plus a non-empty consequence_risk_rating, satisfying both
//     chk_profiles_consequence_type AND chk_profiles_consequence_union.
//  3. Re-running SeedDemo on the same database is a no-op (no
//     duplicate-key errors, no row mutation).
//
// This is the integration test that would have caught the original
// Azure regression — the failure surfaced as
// "pq: new row for relation \"authority_profiles\" violates check
//  constraint \"chk_profiles_consequence_type\" (23514)" precisely at
// the seeded profile's first Create call against Postgres.
func TestSeedDemo_PostgresEndToEnd(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() {
		cleanupSeedDemoRows(t, db)
		db.Close()
	})

	// Pre-clean in case a prior failed run left rows behind. SeedDemo
	// is idempotent, but it is ID-keyed: any existing row blocks the
	// fresh-create assertions below.
	cleanupSeedDemoRows(t, db)

	store, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := store.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	ctx := context.Background()

	// --- First seed: must succeed end-to-end. ---
	if err := bootstrap.SeedDemo(ctx, repos); err != nil {
		t.Fatalf("first SeedDemo against Postgres: %v", err)
	}

	// --- Profiles round-trip with schema-compatible consequence_type. ---
	std, err := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-standard", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion(profile-v2-standard v1): %v", err)
	}
	if std == nil {
		t.Fatal("profile-v2-standard v1 must exist after SeedDemo against Postgres")
	}
	if std.ConsequenceThreshold.Type != "risk_rating" {
		t.Errorf("profile-v2-standard.consequence_type: want risk_rating, got %q", std.ConsequenceThreshold.Type)
	}
	if std.ConsequenceThreshold.RiskRating == "" {
		t.Errorf("profile-v2-standard.risk_rating must be non-empty when consequence_type=risk_rating; got %+v", std.ConsequenceThreshold)
	}
	if std.Status != authority.ProfileStatusActive {
		t.Errorf("profile-v2-standard.status: want active, got %q", std.Status)
	}
	if std.EscalationMode != authority.EscalationModeAuto {
		t.Errorf("profile-v2-standard.escalation_mode: want auto, got %q", std.EscalationMode)
	}

	onb, err := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-onboarding", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion(profile-v2-onboarding v1): %v", err)
	}
	if onb == nil {
		t.Fatal("profile-v2-onboarding v1 must exist after SeedDemo against Postgres")
	}
	if onb.ConsequenceThreshold.Type != "risk_rating" {
		t.Errorf("profile-v2-onboarding.consequence_type: want risk_rating, got %q", onb.ConsequenceThreshold.Type)
	}
	if onb.ConsequenceThreshold.RiskRating == "" {
		t.Errorf("profile-v2-onboarding.risk_rating must be non-empty when consequence_type=risk_rating; got %+v", onb.ConsequenceThreshold)
	}

	// --- Grants round-trip linked to profiles + agent. ---
	stdGrant, err := repos.Grants.FindByID(ctx, "grant-v2-standard")
	if err != nil {
		t.Fatalf("FindByID(grant-v2-standard): %v", err)
	}
	if stdGrant == nil || stdGrant.Status != authority.GrantStatusActive {
		t.Errorf("grant-v2-standard must exist and be active; got %+v", stdGrant)
	}
	onbGrant, err := repos.Grants.FindByID(ctx, "grant-v2-onboarding")
	if err != nil {
		t.Fatalf("FindByID(grant-v2-onboarding): %v", err)
	}
	if onbGrant == nil || onbGrant.Status != authority.GrantStatusActive {
		t.Errorf("grant-v2-onboarding must exist and be active; got %+v", onbGrant)
	}

	// --- Second seed: idempotent against Postgres too. ---
	if err := bootstrap.SeedDemo(ctx, repos); err != nil {
		t.Fatalf("second SeedDemo against Postgres (must be idempotent): %v", err)
	}

	// Profile + grant counts unchanged by the second run.
	stdAfter, _ := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-standard", 1)
	if stdAfter == nil || !stdAfter.UpdatedAt.Equal(std.UpdatedAt) {
		t.Errorf("profile-v2-standard mutated by second seed:\n  before=%+v\n  after =%+v", std, stdAfter)
	}
	onbAfter, _ := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-onboarding", 1)
	if onbAfter == nil || !onbAfter.UpdatedAt.Equal(onb.UpdatedAt) {
		t.Errorf("profile-v2-onboarding mutated by second seed:\n  before=%+v\n  after =%+v", onb, onbAfter)
	}
}
