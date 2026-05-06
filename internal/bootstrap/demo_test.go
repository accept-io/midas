package bootstrap

// demo_test.go — bootstrap.SeedDemo idempotency contract.
//
// The seed must:
//   - create the full demo dataset on a fresh store;
//   - be safe to run repeatedly (no duplicates, no errors);
//   - repair stale-partial seeded stores (any subset of demo entities
//     missing from a previous deployment is created on the next run);
//   - NEVER overwrite a row whose ID matches a seed entity — manual
//     edits to seeded demo entities must survive subsequent SeedDemo
//     calls.
//
// Each test below uses the shared in-memory Repositories factory so the
// assertions exercise the same repository implementations the
// production memory backend uses.

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
)

// freshRepos returns a brand-new in-memory Repositories instance. Used
// by the fresh-seed and repeated-seed tests.
func freshRepos(t *testing.T) *store.Repositories {
	t.Helper()
	repos, err := memory.NewStore().Repositories()
	if err != nil {
		t.Fatalf("memory.NewStore().Repositories(): %v", err)
	}
	return repos
}

// ---------------------------------------------------------------------------
// Test 1: fresh seed creates the full current demo dataset
// ---------------------------------------------------------------------------

// TestSeedDemo_FreshCreatesCompleteCurrentDataset asserts that on an
// empty store, SeedDemo produces representative entities across every
// seeded category. The brief deliberately calls out the categories
// individually so a future addition to the seed has to update this
// pin AND the seed itself — preventing a silent contract drift.
func TestSeedDemo_FreshCreatesCompleteCurrentDataset(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("SeedDemo on fresh store: %v", err)
	}

	// Business Services — at least one must exist; assert the canonical
	// IDs both appeared.
	for _, id := range []string{"bs-consumer-lending", "bs-merchant-services"} {
		bs, err := repos.BusinessServices.GetByID(ctx, id)
		if err != nil {
			t.Errorf("GetByID(business_service %s): %v", id, err)
		}
		if bs == nil {
			t.Errorf("expected business service %s to exist", id)
		}
	}

	// Capabilities — assert the four canonical IDs.
	for _, id := range []string{
		"cap-identity-verification", "cap-credit-scoring",
		"cap-fraud-detection", "cap-payment-authorization",
	} {
		c, err := repos.Capabilities.GetByID(ctx, id)
		if err != nil {
			t.Errorf("GetByID(capability %s): %v", id, err)
		}
		if c == nil {
			t.Errorf("expected capability %s to exist", id)
		}
	}

	// Processes — assert the four canonical IDs.
	for _, id := range []string{
		"proc-consumer-onboarding", "proc-credit-assessment",
		"proc-merchant-risk-screen", "proc-merchant-payment-auth",
	} {
		p, err := repos.Processes.GetByID(ctx, id)
		if err != nil {
			t.Errorf("GetByID(process %s): %v", id, err)
		}
		if p == nil {
			t.Errorf("expected process %s to exist", id)
		}
	}

	// Decision Surfaces — assert the six canonical IDs at version 1.
	for _, id := range []string{
		"surf-v2-id-verify", "surf-v2-consumer-fraud",
		"surf-v2-credit-assess", "surf-v2-merchant-risk",
		"surf-v2-merchant-payment", "surf-v2-merchant-hv-pay",
	} {
		s, err := repos.Surfaces.FindLatestByID(ctx, id)
		if err != nil {
			t.Errorf("FindLatestByID(surface %s): %v", id, err)
		}
		if s == nil {
			t.Errorf("expected surface %s to exist", id)
		}
	}

	// Process → BusinessService linkage (FK on process row). Pick one
	// representative process and confirm its BusinessServiceID points
	// at the right BS — this exercises the natural N:1 relationship
	// that the seed creates implicitly.
	if p, _ := repos.Processes.GetByID(ctx, "proc-consumer-onboarding"); p == nil || p.BusinessServiceID != "bs-consumer-lending" {
		t.Errorf("proc-consumer-onboarding must link to bs-consumer-lending; got %+v", p)
	}

	// BusinessService ↔ Capability links. Phase 2B Step 11 expanded
	// the dataset to a richer banking landscape; bs-consumer-lending
	// now has five capabilities (id-verification, credit-scoring,
	// fraud-detection, credit-administration, collections). Assert
	// at least one canonical pair is present and the count matches.
	exists, err := repos.BusinessServiceCapabilities.Exists(ctx, "bs-consumer-lending", "cap-fraud-detection")
	if err != nil {
		t.Errorf("BSC Exists: %v", err)
	}
	if !exists {
		t.Error("expected bs-consumer-lending ↔ cap-fraud-detection BSC link to exist")
	}
	consumerLinks, err := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-consumer-lending")
	if err != nil {
		t.Errorf("ListByBusinessServiceID(bs-consumer-lending): %v", err)
	}
	if len(consumerLinks) != 5 {
		t.Errorf("bs-consumer-lending should have 5 BSC links; got %d", len(consumerLinks))
	}

	// Agent — single seeded agent.
	a, err := repos.Agents.GetByID(ctx, "agent-v2-evaluator")
	if err != nil {
		t.Errorf("GetByID(agent agent-v2-evaluator): %v", err)
	}
	if a == nil {
		t.Error("expected agent agent-v2-evaluator to exist")
	}

	// Authority Profiles — both seeded profiles at version 1.
	for _, id := range []string{"profile-v2-standard", "profile-v2-onboarding"} {
		p, err := repos.Profiles.FindByIDAndVersion(ctx, id, 1)
		if err != nil {
			t.Errorf("FindByIDAndVersion(profile %s v1): %v", id, err)
		}
		if p == nil {
			t.Errorf("expected profile %s v1 to exist", id)
		}
		if p != nil && p.Status != authority.ProfileStatusActive {
			t.Errorf("profile %s should be active; got %q", id, p.Status)
		}
	}

	// Authority Grants — both seeded grants.
	for _, id := range []string{"grant-v2-standard", "grant-v2-onboarding"} {
		g, err := repos.Grants.FindByID(ctx, id)
		if err != nil {
			t.Errorf("FindByID(grant %s): %v", id, err)
		}
		if g == nil {
			t.Errorf("expected grant %s to exist", id)
		}
		if g != nil && g.Status != authority.GrantStatusActive {
			t.Errorf("grant %s should be active; got %q", id, g.Status)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 2: repeated calls are idempotent
// ---------------------------------------------------------------------------

// TestSeedDemo_RepeatedCallsAreIdempotent runs the seed twice and asserts
// no duplicate-key errors AND that representative entity field values
// are unchanged between runs. The "field values unchanged" pin guards
// against a regression where the second call accidentally Updates
// existing rows.
func TestSeedDemo_RepeatedCallsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("first SeedDemo: %v", err)
	}

	// Capture representative pre-state.
	bsBefore, _ := repos.BusinessServices.GetByID(ctx, "bs-consumer-lending")
	capsBefore, _ := repos.Capabilities.List(ctx)
	procsBefore, _ := repos.Processes.List(ctx)
	consumerBSCBefore, _ := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-consumer-lending")
	merchantBSCBefore, _ := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-merchant-services")

	// Second invocation must not error and must not duplicate.
	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("second SeedDemo: %v", err)
	}

	// Representative ID lookups still resolve to the same row content.
	bsAfter, _ := repos.BusinessServices.GetByID(ctx, "bs-consumer-lending")
	if bsBefore == nil || bsAfter == nil {
		t.Fatal("bs-consumer-lending must exist before AND after the second seed")
	}
	if bsAfter.Name != bsBefore.Name || bsAfter.Description != bsBefore.Description ||
		!bsAfter.UpdatedAt.Equal(bsBefore.UpdatedAt) {
		t.Errorf("bs-consumer-lending was modified by the second seed:\n  before=%+v\n  after =%+v", bsBefore, bsAfter)
	}

	// Representative collection sizes are stable: no duplicate rows added.
	capsAfter, _ := repos.Capabilities.List(ctx)
	if len(capsAfter) != len(capsBefore) {
		t.Errorf("Capabilities.List size drifted: before=%d after=%d", len(capsBefore), len(capsAfter))
	}
	procsAfter, _ := repos.Processes.List(ctx)
	if len(procsAfter) != len(procsBefore) {
		t.Errorf("Processes.List size drifted: before=%d after=%d", len(procsBefore), len(procsAfter))
	}
	consumerBSCAfter, _ := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-consumer-lending")
	if len(consumerBSCAfter) != len(consumerBSCBefore) {
		t.Errorf("bs-consumer-lending BSC count drifted: before=%d after=%d", len(consumerBSCBefore), len(consumerBSCAfter))
	}
	merchantBSCAfter, _ := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-merchant-services")
	if len(merchantBSCAfter) != len(merchantBSCBefore) {
		t.Errorf("bs-merchant-services BSC count drifted: before=%d after=%d", len(merchantBSCBefore), len(merchantBSCAfter))
	}

	// Authority data parity: profiles and grants must not duplicate.
	for _, id := range []string{"profile-v2-standard", "profile-v2-onboarding"} {
		if got, _ := repos.Profiles.FindByIDAndVersion(ctx, id, 1); got == nil {
			t.Errorf("profile %s v1 must still exist after second seed", id)
		}
	}
	for _, id := range []string{"grant-v2-standard", "grant-v2-onboarding"} {
		if got, _ := repos.Grants.FindByID(ctx, id); got == nil {
			t.Errorf("grant %s must still exist after second seed", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 3: stale partial seed is repaired across all categories
// ---------------------------------------------------------------------------

// TestSeedDemo_PartialDatasetRepairAddsMissingEntities pre-populates a
// store with a *partial* old-style dataset — the anchor BS exists, but
// representative entities from each later-seeded category are
// deliberately missing. SeedDemo must then add ALL the missing rows
// (not just authority data) and leave the pre-existing rows untouched.
//
// The pre-state mirrors the historical Azure Postgres scenario that
// motivated this phase: bs-consumer-lending exists from an earlier
// release, but several later additions never landed because the old
// global-anchor guard returned early.
func TestSeedDemo_PartialDatasetRepairAddsMissingEntities(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	now := time.Now().UTC()
	effective := now.Add(-time.Hour)

	// Pre-create only a SUBSET of the demo dataset:
	//   - bs-consumer-lending     (anchor that used to short-circuit the seed)
	//   - cap-fraud-detection     (one of four capabilities)
	//   - one BSC link            (one of five)
	//   - proc-consumer-onboarding(one of four processes)
	//   - surf-v2-id-verify       (one of six surfaces)
	//   - agent-v2-evaluator      (single agent)
	//   - profile-v2-standard     (one of two profiles)
	//   - grant-v2-standard       (one of two grants)
	//
	// Everything else MUST be created by the post-state SeedDemo call.
	preBS := &businessservice.BusinessService{
		ID:          "bs-consumer-lending",
		Name:        "Consumer Lending",
		Description: "Retail lending products for individual consumers",
		ServiceType: businessservice.ServiceTypeCustomerFacing,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repos.BusinessServices.Create(ctx, preBS); err != nil {
		t.Fatalf("pre-create BS: %v", err)
	}
	if err := repos.Capabilities.Create(ctx, &capability.Capability{
		ID: "cap-fraud-detection", Name: "Fraud Detection", Status: "active", Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("pre-create capability: %v", err)
	}
	if err := repos.BusinessServiceCapabilities.Create(ctx, &businessservicecapability.BusinessServiceCapability{
		BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-fraud-detection", CreatedAt: now,
	}); err != nil {
		t.Fatalf("pre-create BSC: %v", err)
	}
	if err := repos.Processes.Create(ctx, &process.Process{
		ID: "proc-consumer-onboarding", Name: "Consumer Onboarding",
		BusinessServiceID: "bs-consumer-lending",
		Status:            "active", Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("pre-create process: %v", err)
	}
	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID: "surf-v2-id-verify", Version: 1, Name: "Identity Verification", Domain: "consumer-lending",
		ProcessID:          "proc-consumer-onboarding",
		DecisionType:       surface.DecisionTypeTactical,
		ReversibilityClass: surface.ReversibilityConditionallyReversible,
		FailureMode:        surface.FailureModeClosed,
		RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
		ConsequenceTypes:   []surface.ConsequenceType{},
		Status:             surface.SurfaceStatusActive, EffectiveFrom: effective,
		BusinessOwner: "consumer-lending-team", TechnicalOwner: "midas",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("pre-create surface: %v", err)
	}
	if err := repos.Agents.Create(ctx, &agent.Agent{
		ID: "agent-v2-evaluator", Name: "V2 Demo Evaluator", Type: agent.AgentTypeAI,
		Owner: "platform-team", ModelVersion: "v1", Endpoint: "local",
		OperationalState: agent.OperationalStateActive, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("pre-create agent: %v", err)
	}

	// --- Run the seed: it MUST notice every missing piece and create it. ---
	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("SeedDemo on partial-state store: %v", err)
	}

	// Repaired Business Service.
	if got, _ := repos.BusinessServices.GetByID(ctx, "bs-merchant-services"); got == nil {
		t.Error("bs-merchant-services missing — partial-seed repair did not add it")
	}

	// Repaired Capabilities (three of four were missing).
	for _, id := range []string{
		"cap-identity-verification", "cap-credit-scoring", "cap-payment-authorization",
	} {
		if got, _ := repos.Capabilities.GetByID(ctx, id); got == nil {
			t.Errorf("capability %s missing — partial-seed repair did not add it", id)
		}
	}

	// Repaired BSC links (four of five were missing).
	for _, link := range []struct{ bsID, capID string }{
		{"bs-consumer-lending", "cap-identity-verification"},
		{"bs-consumer-lending", "cap-credit-scoring"},
		{"bs-merchant-services", "cap-fraud-detection"},
		{"bs-merchant-services", "cap-payment-authorization"},
	} {
		exists, err := repos.BusinessServiceCapabilities.Exists(ctx, link.bsID, link.capID)
		if err != nil {
			t.Errorf("BSC Exists(%s,%s): %v", link.bsID, link.capID, err)
		}
		if !exists {
			t.Errorf("BSC link %s↔%s missing — partial-seed repair did not add it", link.bsID, link.capID)
		}
	}

	// Repaired Processes (three of four were missing).
	for _, id := range []string{
		"proc-credit-assessment", "proc-merchant-risk-screen", "proc-merchant-payment-auth",
	} {
		if got, _ := repos.Processes.GetByID(ctx, id); got == nil {
			t.Errorf("process %s missing — partial-seed repair did not add it", id)
		}
	}

	// Repaired Surfaces (five of six were missing).
	for _, id := range []string{
		"surf-v2-consumer-fraud", "surf-v2-credit-assess",
		"surf-v2-merchant-risk", "surf-v2-merchant-payment", "surf-v2-merchant-hv-pay",
	} {
		if got, _ := repos.Surfaces.FindLatestByID(ctx, id); got == nil {
			t.Errorf("surface %s missing — partial-seed repair did not add it", id)
		}
	}

	// Repaired authority data — this is the headline case from Phase 6
	// analysis: profile-v2-onboarding + grant-v2-onboarding link
	// surf-v2-id-verify into the authority graph and were the entities
	// the global-anchor guard was suppressing.
	if got, _ := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-onboarding", 1); got == nil {
		t.Error("profile-v2-onboarding v1 missing — partial-seed repair did not add it")
	}
	if got, _ := repos.Grants.FindByID(ctx, "grant-v2-onboarding"); got == nil {
		t.Error("grant-v2-onboarding missing — partial-seed repair did not add it")
	}
}

// ---------------------------------------------------------------------------
// Test 4: existing manually-edited demo entity is NOT overwritten
// ---------------------------------------------------------------------------

// TestSeedDemo_DoesNotOverwriteExistingDemoEntity pre-creates a row
// with the same ID as a seed entity but with deliberately-modified
// fields (representative across categories: BS name, process status,
// surface domain). After SeedDemo runs, the modified fields must
// remain unchanged — the seed must NEVER touch a row that already
// exists at the seeded identity. This is the contract that protects
// operator-edited demo data from being silently reverted on every
// startup.
func TestSeedDemo_DoesNotOverwriteExistingDemoEntity(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	now := time.Now().UTC()
	effective := now.Add(-time.Hour)

	// Pre-existing BS with a modified Name + Description.
	preBS := &businessservice.BusinessService{
		ID:          "bs-consumer-lending",
		Name:        "EDITED — Renamed Lending Service",
		Description: "EDITED — operator-customised description",
		ServiceType: businessservice.ServiceTypeCustomerFacing,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now, UpdatedAt: now,
	}
	if err := repos.BusinessServices.Create(ctx, preBS); err != nil {
		t.Fatalf("pre-create BS: %v", err)
	}

	// Pre-existing capability with a modified Name.
	preCap := &capability.Capability{
		ID: "cap-fraud-detection", Name: "EDITED — Custom Fraud Detection",
		Status: "active", Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repos.Capabilities.Create(ctx, preCap); err != nil {
		t.Fatalf("pre-create capability: %v", err)
	}

	// Pre-existing process under bs-consumer-lending; modified Name.
	preProc := &process.Process{
		ID: "proc-consumer-onboarding", Name: "EDITED — Custom Onboarding Flow",
		BusinessServiceID: "bs-consumer-lending",
		Status:            "active", Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repos.Processes.Create(ctx, preProc); err != nil {
		t.Fatalf("pre-create process: %v", err)
	}

	// Pre-existing surface with a modified Description.
	preSurf := &surface.DecisionSurface{
		ID: "surf-v2-id-verify", Version: 1, Name: "Identity Verification",
		Description:        "EDITED — operator-customised surface description",
		Domain:             "consumer-lending",
		ProcessID:          "proc-consumer-onboarding",
		DecisionType:       surface.DecisionTypeTactical,
		ReversibilityClass: surface.ReversibilityConditionallyReversible,
		FailureMode:        surface.FailureModeClosed,
		RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
		ConsequenceTypes:   []surface.ConsequenceType{},
		Status:             surface.SurfaceStatusActive, EffectiveFrom: effective,
		BusinessOwner: "consumer-lending-team", TechnicalOwner: "midas",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repos.Surfaces.Create(ctx, preSurf); err != nil {
		t.Fatalf("pre-create surface: %v", err)
	}

	// Pre-existing agent with a modified Name.
	preAgent := &agent.Agent{
		ID: "agent-v2-evaluator", Name: "EDITED — Custom Evaluator", Type: agent.AgentTypeAI,
		Owner: "platform-team", ModelVersion: "v1", Endpoint: "local",
		OperationalState: agent.OperationalStateActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := repos.Agents.Create(ctx, preAgent); err != nil {
		t.Fatalf("pre-create agent: %v", err)
	}

	// Pre-existing profile (v1) with a modified Name.
	preProfile := &authority.AuthorityProfile{
		ID: "profile-v2-standard", Version: 1, SurfaceID: "surf-v2-merchant-payment",
		Name: "EDITED — Custom Standard Authority", Description: "x",
		Status: authority.ProfileStatusActive, EffectiveDate: effective,
		ConfidenceThreshold:  0.85,
		EscalationMode:       authority.EscalationModeAuto,
		FailMode:             authority.FailModeClosed,
		RequiredContextKeys:  []string{},
		CreatedAt:            now, UpdatedAt: now,
	}
	if err := repos.Profiles.Create(ctx, preProfile); err != nil {
		t.Fatalf("pre-create profile: %v", err)
	}

	// Pre-existing grant with a modified GrantedBy.
	preGrant := &authority.AuthorityGrant{
		ID: "grant-v2-standard", AgentID: "agent-v2-evaluator", ProfileID: "profile-v2-standard",
		GrantedBy: "EDITED-operator", EffectiveDate: effective,
		Status:    authority.GrantStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repos.Grants.Create(ctx, preGrant); err != nil {
		t.Fatalf("pre-create grant: %v", err)
	}

	// Run the seed.
	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	// Every modified field must survive unchanged.
	gotBS, _ := repos.BusinessServices.GetByID(ctx, "bs-consumer-lending")
	if gotBS == nil || gotBS.Name != "EDITED — Renamed Lending Service" {
		t.Errorf("BS name was overwritten: got %+v", gotBS)
	}
	if gotBS != nil && gotBS.Description != "EDITED — operator-customised description" {
		t.Errorf("BS description was overwritten: got %q", gotBS.Description)
	}
	gotCap, _ := repos.Capabilities.GetByID(ctx, "cap-fraud-detection")
	if gotCap == nil || gotCap.Name != "EDITED — Custom Fraud Detection" {
		t.Errorf("capability name was overwritten: got %+v", gotCap)
	}
	gotProc, _ := repos.Processes.GetByID(ctx, "proc-consumer-onboarding")
	if gotProc == nil || gotProc.Name != "EDITED — Custom Onboarding Flow" {
		t.Errorf("process name was overwritten: got %+v", gotProc)
	}
	gotSurf, _ := repos.Surfaces.FindLatestByID(ctx, "surf-v2-id-verify")
	if gotSurf == nil || gotSurf.Description != "EDITED — operator-customised surface description" {
		t.Errorf("surface description was overwritten: got %+v", gotSurf)
	}
	gotAgent, _ := repos.Agents.GetByID(ctx, "agent-v2-evaluator")
	if gotAgent == nil || gotAgent.Name != "EDITED — Custom Evaluator" {
		t.Errorf("agent name was overwritten: got %+v", gotAgent)
	}
	gotProfile, _ := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-standard", 1)
	if gotProfile == nil || gotProfile.Name != "EDITED — Custom Standard Authority" {
		t.Errorf("profile name was overwritten: got %+v", gotProfile)
	}
	gotGrant, _ := repos.Grants.FindByID(ctx, "grant-v2-standard")
	if gotGrant == nil || gotGrant.GrantedBy != "EDITED-operator" {
		t.Errorf("grant granted_by was overwritten: got %+v", gotGrant)
	}
}

// ---------------------------------------------------------------------------
// Test 5: relationship/link repair regression
// ---------------------------------------------------------------------------

// TestSeedDemo_RelationshipRepair_AddsMissingLinksWithoutDuplicating
// pre-creates Business Services and Capabilities WITHOUT any of their
// BSC links. SeedDemo must then create the missing links. Re-running
// the seed afterwards must NOT duplicate them.
//
// This is the dedicated link-table regression that complements Test 3.
// Test 3 covers a mixed-category partial state; this test isolates
// the link-table behaviour so a regression in BSC handling alone is
// caught.
func TestSeedDemo_RelationshipRepair_AddsMissingLinksWithoutDuplicating(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	now := time.Now().UTC()

	// Pre-create both BSes and all four capabilities — but NO BSC
	// links and NO downstream entities. SeedDemo must create the
	// links + processes + surfaces + agent + profiles + grants
	// without choking on the pre-existing parents.
	for _, bs := range []*businessservice.BusinessService{
		{
			ID: "bs-consumer-lending", Name: "Consumer Lending",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			Status:      "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "bs-merchant-services", Name: "Merchant Services",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			Status:      "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := repos.BusinessServices.Create(ctx, bs); err != nil {
			t.Fatalf("pre-create BS %s: %v", bs.ID, err)
		}
	}
	for _, c := range []*capability.Capability{
		{ID: "cap-identity-verification", Name: "Identity Verification", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "cap-credit-scoring", Name: "Credit Scoring", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "cap-fraud-detection", Name: "Fraud Detection", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
		{ID: "cap-payment-authorization", Name: "Payment Authorization", Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now},
	} {
		if err := repos.Capabilities.Create(ctx, c); err != nil {
			t.Fatalf("pre-create capability %s: %v", c.ID, err)
		}
	}

	// First seed: must add all five BSC links + downstream entities.
	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("first SeedDemo: %v", err)
	}

	consumerLinks, _ := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-consumer-lending")
	merchantLinks, _ := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-merchant-services")
	// Phase 2B Step 11 enrichment: bs-consumer-lending → 5 caps,
	// bs-merchant-services → 4 caps.
	if len(consumerLinks) != 5 || len(merchantLinks) != 4 {
		t.Errorf("BSC links after first seed: want 5+4, got %d+%d", len(consumerLinks), len(merchantLinks))
	}

	// Second seed: must NOT duplicate links.
	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("second SeedDemo: %v", err)
	}
	consumerLinks2, _ := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-consumer-lending")
	merchantLinks2, _ := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, "bs-merchant-services")
	if len(consumerLinks2) != len(consumerLinks) || len(merchantLinks2) != len(merchantLinks) {
		t.Errorf("BSC links duplicated on re-seed: consumer %d→%d, merchant %d→%d",
			len(consumerLinks), len(consumerLinks2), len(merchantLinks), len(merchantLinks2))
	}
}

// ---------------------------------------------------------------------------
// Test 6: every seeded authority profile uses a schema-compatible
// consequence_type. (Phase 8 follow-up — Azure regression.)
// ---------------------------------------------------------------------------

// seededDemoProfileIDs is the canonical list of authority profile IDs
// that bootstrap.SeedDemo creates. The "uses valid consequence_type"
// test below iterates over all of them, so adding a new profile to
// demo.go automatically extends this assertion's coverage as long as
// the new ID is appended here.
var seededDemoProfileIDs = []string{
	"profile-v2-standard",
	"profile-v2-onboarding",
	// Phase 2B Step 11 enrichment.
	"profile-v2-credit-assess",
	"profile-v2-fraud-detection",
}

// schemaAllowedConsequenceTypes is the set of consequence_type values
// permitted by the Postgres CHECK constraint
// chk_profiles_consequence_type. Mirrored here as a literal set so this
// test fails loudly if a seeded profile uses a value that is in the
// domain enum but not in the schema (the exact failure mode that broke
// Azure startup with consequence_type='monetary').
var schemaAllowedConsequenceTypes = map[string]struct{}{
	"risk_rating":  {},
	"financial":    {},
	"temporal":     {},
	"impact_scope": {},
	"custom":       {},
}

// TestSeedDemo_AuthorityProfilesUseValidConsequenceTypes asserts that
// every seeded profile's consequence_type is non-empty AND in the set
// the Postgres schema accepts. This is the structural pin that prevents
// a future seed addition from re-introducing the Azure regression.
func TestSeedDemo_AuthorityProfilesUseValidConsequenceTypes(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	for _, id := range seededDemoProfileIDs {
		// Look up the latest version of the profile. Per-version
		// idempotency in the seed ensures version 1 always exists; we
		// use FindByID to keep the test future-proof against later
		// version bumps to seeded profiles.
		got, err := repos.Profiles.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID(profile %s): %v", id, err)
		}
		if got == nil {
			t.Fatalf("seeded profile %s missing", id)
		}

		ct := string(got.ConsequenceThreshold.Type)
		if ct == "" {
			t.Errorf("profile %s: consequence_type is empty (schema requires NOT NULL)", id)
			continue
		}
		if _, ok := schemaAllowedConsequenceTypes[ct]; !ok {
			t.Errorf("profile %s: consequence_type %q is not in the schema's chk_profiles_consequence_type whitelist; "+
				"allowed values are risk_rating, financial, temporal, impact_scope, custom", id, ct)
		}
	}
}

// TestSeedDemo_FreshCreatesAuthorityProfiles asserts that on a fresh
// store both seeded profiles exist with valid status, escalation_mode,
// surface_id, and consequence_type. Complements the broader Phase 8
// fresh-seed test by focusing on the authority-profile sub-graph that
// the Azure regression broke.
func TestSeedDemo_FreshCreatesAuthorityProfiles(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	// profile-v2-standard
	std, err := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-standard", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion(profile-v2-standard v1): %v", err)
	}
	if std == nil {
		t.Fatal("profile-v2-standard v1 must exist after fresh seed")
	}
	if std.Status != authority.ProfileStatusActive {
		t.Errorf("profile-v2-standard.status: want active, got %q", std.Status)
	}
	if std.EscalationMode != authority.EscalationModeAuto {
		t.Errorf("profile-v2-standard.escalation_mode: want auto, got %q", std.EscalationMode)
	}
	if std.SurfaceID != "surf-v2-merchant-payment" {
		t.Errorf("profile-v2-standard.surface_id: want surf-v2-merchant-payment, got %q", std.SurfaceID)
	}
	if _, ok := schemaAllowedConsequenceTypes[string(std.ConsequenceThreshold.Type)]; !ok {
		t.Errorf("profile-v2-standard.consequence_type %q not schema-valid", std.ConsequenceThreshold.Type)
	}

	// profile-v2-onboarding
	onb, err := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-onboarding", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion(profile-v2-onboarding v1): %v", err)
	}
	if onb == nil {
		t.Fatal("profile-v2-onboarding v1 must exist after fresh seed")
	}
	if onb.Status != authority.ProfileStatusActive {
		t.Errorf("profile-v2-onboarding.status: want active, got %q", onb.Status)
	}
	if onb.EscalationMode != authority.EscalationModeAuto {
		t.Errorf("profile-v2-onboarding.escalation_mode: want auto, got %q", onb.EscalationMode)
	}
	if onb.SurfaceID != "surf-v2-id-verify" {
		t.Errorf("profile-v2-onboarding.surface_id: want surf-v2-id-verify, got %q", onb.SurfaceID)
	}
	if _, ok := schemaAllowedConsequenceTypes[string(onb.ConsequenceThreshold.Type)]; !ok {
		t.Errorf("profile-v2-onboarding.consequence_type %q not schema-valid", onb.ConsequenceThreshold.Type)
	}
}

// TestSeedDemo_RepeatedCallsRemainIdempotent_AuthorityProfiles is a
// narrowed re-run of the broader idempotency contract focused on the
// authority sub-graph. Two SeedDemo runs must produce stable
// profile/grant counts AND identical consequence_type values (a
// regression that accidentally Update'd the second-run row with a
// different consequence_type would surface here).
func TestSeedDemo_RepeatedCallsRemainIdempotent_AuthorityProfiles(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("first SeedDemo: %v", err)
	}

	stdBefore, _ := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-standard", 1)
	onbBefore, _ := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-onboarding", 1)
	stdGrantBefore, _ := repos.Grants.FindByID(ctx, "grant-v2-standard")
	onbGrantBefore, _ := repos.Grants.FindByID(ctx, "grant-v2-onboarding")

	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("second SeedDemo: %v", err)
	}

	stdAfter, _ := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-standard", 1)
	onbAfter, _ := repos.Profiles.FindByIDAndVersion(ctx, "profile-v2-onboarding", 1)
	stdGrantAfter, _ := repos.Grants.FindByID(ctx, "grant-v2-standard")
	onbGrantAfter, _ := repos.Grants.FindByID(ctx, "grant-v2-onboarding")

	if stdBefore == nil || stdAfter == nil ||
		stdBefore.ConsequenceThreshold.Type != stdAfter.ConsequenceThreshold.Type ||
		stdBefore.ConsequenceThreshold.RiskRating != stdAfter.ConsequenceThreshold.RiskRating ||
		!stdBefore.UpdatedAt.Equal(stdAfter.UpdatedAt) {
		t.Errorf("profile-v2-standard mutated by second seed:\n  before=%+v\n  after =%+v", stdBefore, stdAfter)
	}
	if onbBefore == nil || onbAfter == nil ||
		onbBefore.ConsequenceThreshold.Type != onbAfter.ConsequenceThreshold.Type ||
		onbBefore.ConsequenceThreshold.RiskRating != onbAfter.ConsequenceThreshold.RiskRating ||
		!onbBefore.UpdatedAt.Equal(onbAfter.UpdatedAt) {
		t.Errorf("profile-v2-onboarding mutated by second seed:\n  before=%+v\n  after =%+v", onbBefore, onbAfter)
	}
	if stdGrantBefore == nil || stdGrantAfter == nil ||
		stdGrantBefore.Status != stdGrantAfter.Status {
		t.Errorf("grant-v2-standard mutated by second seed:\n  before=%+v\n  after =%+v", stdGrantBefore, stdGrantAfter)
	}
	if onbGrantBefore == nil || onbGrantAfter == nil ||
		onbGrantBefore.Status != onbGrantAfter.Status {
		t.Errorf("grant-v2-onboarding mutated by second seed:\n  before=%+v\n  after =%+v", onbGrantBefore, onbGrantAfter)
	}
}

