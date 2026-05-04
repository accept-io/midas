package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// SeedDemo inserts the canonical demonstration dataset for the v1 service-led
// structural model: Capabilities enable BusinessServices through the
// business_service_capabilities M:N junction; BusinessServices deliver
// Processes (1:N); Processes contain DecisionSurfaces (1:N).
//
// Idempotency contract:
//
// SeedDemo is per-entity self-healing. It is safe to run on every startup
// against any backend (memory or postgres). For each demo entity and each
// link, the seed:
//
//   - looks up the row by its stable identity (ID, or natural key for
//     junction tables, or (ID, Version) for versioned types);
//   - if the row exists, leaves every field of the existing row unchanged
//     (no overwrite, no delete, no Update);
//   - if the row does not exist, creates it with the seed's canonical
//     values.
//
// This is a deliberate departure from the previous global-anchor guard
// that returned early when bs-consumer-lending already existed. That
// guard meant a deployment which was first seeded before a later demo
// entity was added (e.g. profile-v2-onboarding for Consumer Lending,
// added 2026-04-07) could never pick up the missing rows on a restart —
// the dataset was permanently stuck at its first-seed shape. With
// per-entity idempotency, adding a new demo entity in a future release
// is automatically backfilled on the next restart of any deployment
// running with MIDAS_DEV_SEED_DEMO_DATA=true.
//
// Seeding order mirrors the apply path's kindOrder dependency tiers:
// BusinessService → Capability → BusinessServiceCapability → Process →
// Surface, then auxiliary Agent/Profile/Grant. Reading the seed top-to-
// bottom therefore narrates the model: the service offerings come first,
// then the abilities that enable them, then the links between the two,
// then the processes each service delivers, and finally the decision
// surfaces inside each process.
//
// Dataset overview:
//
//	BusinessServices:
//	  bs-consumer-lending   Consumer Lending
//	  bs-merchant-services  Merchant Services
//
//	Capabilities:
//	  cap-identity-verification  Identity Verification
//	  cap-credit-scoring         Credit Scoring
//	  cap-fraud-detection        Fraud Detection (shared — realised by both services)
//	  cap-payment-authorization  Payment Authorization
//
//	BusinessServiceCapabilities (canonical Capability ↔ BusinessService):
//	  bs-consumer-lending   ↔ cap-identity-verification
//	  bs-consumer-lending   ↔ cap-credit-scoring
//	  bs-consumer-lending   ↔ cap-fraud-detection
//	  bs-merchant-services  ↔ cap-fraud-detection
//	  bs-merchant-services  ↔ cap-payment-authorization
//
//	Processes (→ BusinessService, required N:1):
//	  proc-consumer-onboarding   → bs-consumer-lending
//	  proc-credit-assessment     → bs-consumer-lending
//	  proc-merchant-risk-screen  → bs-merchant-services
//	  proc-merchant-payment-auth → bs-merchant-services
//
//	Surfaces (→ Process):
//	  surf-v2-id-verify        → proc-consumer-onboarding
//	  surf-v2-consumer-fraud   → proc-consumer-onboarding
//	  surf-v2-credit-assess    → proc-credit-assessment
//	  surf-v2-merchant-risk    → proc-merchant-risk-screen
//	  surf-v2-merchant-payment → proc-merchant-payment-auth
//	  surf-v2-merchant-hv-pay  → proc-merchant-payment-auth
//
//	Agent / Profile / Grant:
//	  agent-v2-evaluator
//	  profile-v2-standard    (linked to surf-v2-merchant-payment)
//	  profile-v2-onboarding  (linked to surf-v2-id-verify)
//	  grant-v2-standard      (profile-v2-standard ↔ agent-v2-evaluator)
//	  grant-v2-onboarding    (profile-v2-onboarding ↔ agent-v2-evaluator)
//
// Supports both memory and postgres backends.
func SeedDemo(ctx context.Context, repos *store.Repositories) error {
	now := time.Now().UTC()
	effective := now.Add(-time.Hour)

	// --- Business Services ---
	// The service offerings the organisation delivers and governs. Each
	// BusinessService later owns one or more Processes (1:N) and is enabled
	// by zero or more Capabilities through the BSC junction.

	bsvcs := []*businessservice.BusinessService{
		{
			ID:          "bs-consumer-lending",
			Name:        "Consumer Lending",
			Description: "Retail lending products for individual consumers",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "bs-merchant-services",
			Name:        "Merchant Services",
			Description: "Payment processing and fraud prevention for merchants",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	for _, s := range bsvcs {
		if err := ensureBusinessService(ctx, repos.BusinessServices, s); err != nil {
			return err
		}
	}

	// --- Capabilities ---
	// The enabling abilities that BusinessServices draw on. A Capability
	// can enable any number of BusinessServices via the BSC junction
	// (M:N); cap-fraud-detection demonstrates this by enabling both
	// consumer-lending and merchant-services below.

	caps := []*capability.Capability{
		{
			ID:        "cap-identity-verification",
			Name:      "Identity Verification",
			Status:    "active",
			Origin:    "manual",
			Managed:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "cap-credit-scoring",
			Name:      "Credit Scoring",
			Status:    "active",
			Origin:    "manual",
			Managed:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "cap-fraud-detection",
			Name:      "Fraud Detection",
			Status:    "active",
			Origin:    "manual",
			Managed:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "cap-payment-authorization",
			Name:      "Payment Authorization",
			Status:    "active",
			Origin:    "manual",
			Managed:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	for _, c := range caps {
		if err := ensureCapability(ctx, repos.Capabilities, c); err != nil {
			return err
		}
	}

	// --- BusinessService ↔ Capability links ---
	// The enabling relationships: each row says "this Capability enables
	// this BusinessService". cap-fraud-detection enables both services,
	// demonstrating cross-service capability reuse under M:N.

	bscLinks := []*businessservicecapability.BusinessServiceCapability{
		{BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-identity-verification", CreatedAt: now},
		{BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-credit-scoring", CreatedAt: now},
		{BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-fraud-detection", CreatedAt: now},
		{BusinessServiceID: "bs-merchant-services", CapabilityID: "cap-fraud-detection", CreatedAt: now},
		{BusinessServiceID: "bs-merchant-services", CapabilityID: "cap-payment-authorization", CreatedAt: now},
	}
	for _, bsc := range bscLinks {
		if err := ensureBSC(ctx, repos.BusinessServiceCapabilities, bsc); err != nil {
			return err
		}
	}

	// --- Processes ---
	// Each Process belongs to exactly one BusinessService (N:1, NOT NULL).
	// Each BusinessService delivers one or more Processes; both demo
	// services have two processes here, exercising the multi-Process case.

	procs := []*process.Process{
		{
			ID:                "proc-consumer-onboarding",
			Name:              "Consumer Onboarding",
			BusinessServiceID: "bs-consumer-lending",
			Status:            "active",
			Origin:            "manual",
			Managed:           true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		{
			ID:                "proc-credit-assessment",
			Name:              "Credit Assessment",
			BusinessServiceID: "bs-consumer-lending",
			Status:            "active",
			Origin:            "manual",
			Managed:           true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		{
			ID:                "proc-merchant-risk-screen",
			Name:              "Merchant Risk Screening",
			BusinessServiceID: "bs-merchant-services",
			Status:            "active",
			Origin:            "manual",
			Managed:           true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		{
			ID:                "proc-merchant-payment-auth",
			Name:              "Merchant Payment Authorization",
			BusinessServiceID: "bs-merchant-services",
			Status:            "active",
			Origin:            "manual",
			Managed:           true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}
	for _, p := range procs {
		if err := ensureProcess(ctx, repos.Processes, p); err != nil {
			return err
		}
	}

	// --- Surfaces ---
	// Decision surfaces inside processes (1:N). proc-consumer-onboarding
	// and proc-merchant-payment-auth each carry two surfaces, exercising
	// the multi-Surface-per-Process case.

	surfs := []*surface.DecisionSurface{
		{
			ID:                 "surf-v2-id-verify",
			Version:            1,
			Name:               "Identity Verification",
			Description:        "Governs automated identity verification for consumer onboarding",
			Domain:             "consumer-lending",
			ProcessID:          "proc-consumer-onboarding",
			DecisionType:       surface.DecisionTypeTactical,
			ReversibilityClass: surface.ReversibilityConditionallyReversible,
			FailureMode:        surface.FailureModeClosed,
			RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes:   []surface.ConsequenceType{},
			Status:             surface.SurfaceStatusActive,
			EffectiveFrom:      effective,
			BusinessOwner:      "consumer-lending-team",
			TechnicalOwner:     "midas",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "surf-v2-consumer-fraud",
			Version:            1,
			Name:               "Consumer Fraud Check",
			Description:        "Governs fraud screening during consumer onboarding",
			Domain:             "consumer-lending",
			ProcessID:          "proc-consumer-onboarding",
			DecisionType:       surface.DecisionTypeTactical,
			ReversibilityClass: surface.ReversibilityConditionallyReversible,
			FailureMode:        surface.FailureModeClosed,
			RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes:   []surface.ConsequenceType{},
			Status:             surface.SurfaceStatusActive,
			EffectiveFrom:      effective,
			BusinessOwner:      "consumer-lending-team",
			TechnicalOwner:     "midas",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "surf-v2-credit-assess",
			Version:            1,
			Name:               "Credit Assessment",
			Description:        "Governs automated credit assessment decisions",
			Domain:             "consumer-lending",
			ProcessID:          "proc-credit-assessment",
			DecisionType:       surface.DecisionTypeTactical,
			ReversibilityClass: surface.ReversibilityConditionallyReversible,
			FailureMode:        surface.FailureModeClosed,
			RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes:   []surface.ConsequenceType{},
			Status:             surface.SurfaceStatusActive,
			EffectiveFrom:      effective,
			BusinessOwner:      "consumer-lending-team",
			TechnicalOwner:     "midas",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "surf-v2-merchant-risk",
			Version:            1,
			Name:               "Merchant Risk Screening",
			Description:        "Governs merchant transaction risk screening",
			Domain:             "merchant-services",
			ProcessID:          "proc-merchant-risk-screen",
			DecisionType:       surface.DecisionTypeTactical,
			ReversibilityClass: surface.ReversibilityConditionallyReversible,
			FailureMode:        surface.FailureModeClosed,
			RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes:   []surface.ConsequenceType{},
			Status:             surface.SurfaceStatusActive,
			EffectiveFrom:      effective,
			BusinessOwner:      "merchant-services-team",
			TechnicalOwner:     "midas",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "surf-v2-merchant-payment",
			Version:            1,
			Name:               "Merchant Payment Authorization",
			Description:        "Governs automated payment authorization for merchants",
			Domain:             "merchant-services",
			ProcessID:          "proc-merchant-payment-auth",
			DecisionType:       surface.DecisionTypeTactical,
			ReversibilityClass: surface.ReversibilityConditionallyReversible,
			FailureMode:        surface.FailureModeClosed,
			RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes:   []surface.ConsequenceType{},
			Status:             surface.SurfaceStatusActive,
			EffectiveFrom:      effective,
			BusinessOwner:      "merchant-services-team",
			TechnicalOwner:     "midas",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "surf-v2-merchant-hv-pay",
			Version:            1,
			Name:               "Merchant High-Value Payment Authorization",
			Description:        "Governs high-value payment authorization with enhanced scrutiny",
			Domain:             "merchant-services",
			ProcessID:          "proc-merchant-payment-auth",
			DecisionType:       surface.DecisionTypeStrategic,
			ReversibilityClass: surface.ReversibilityIrreversible,
			FailureMode:        surface.FailureModeClosed,
			RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes:   []surface.ConsequenceType{},
			Status:             surface.SurfaceStatusActive,
			EffectiveFrom:      effective,
			BusinessOwner:      "merchant-services-team",
			TechnicalOwner:     "midas",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	for _, s := range surfs {
		if err := ensureSurface(ctx, repos.Surfaces, s); err != nil {
			return err
		}
	}

	// --- Agent ---

	if err := ensureAgent(ctx, repos.Agents, &agent.Agent{
		ID:               "agent-v2-evaluator",
		Name:             "V2 Demo Evaluator",
		Type:             agent.AgentTypeAI,
		Owner:            "platform-team",
		ModelVersion:     "v1",
		Endpoint:         "local",
		OperationalState: agent.OperationalStateActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		return err
	}

	// --- Profile (standard — merchant payment) ---

	if err := ensureProfile(ctx, repos.Profiles, &authority.AuthorityProfile{
		ID:          "profile-v2-standard",
		Version:     1,
		SurfaceID:   "surf-v2-merchant-payment",
		Name:        "Standard Merchant Payment Authority",
		Description: "Standard authority limits for automated merchant payment authorization",

		Status:        authority.ProfileStatusActive,
		EffectiveDate: effective,

		ConfidenceThreshold: 0.85,
		ConsequenceThreshold: authority.Consequence{
			Type:     value.ConsequenceTypeMonetary,
			Amount:   5000,
			Currency: "GBP",
		},

		EscalationMode:      authority.EscalationModeAuto,
		FailMode:            authority.FailModeClosed,
		RequiredContextKeys: []string{},

		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return err
	}

	// --- Grant (standard — merchant payment) ---

	if err := ensureGrant(ctx, repos.Grants, &authority.AuthorityGrant{
		ID:            "grant-v2-standard",
		AgentID:       "agent-v2-evaluator",
		ProfileID:     "profile-v2-standard",
		GrantedBy:     "system",
		EffectiveDate: effective,
		Status:        authority.GrantStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		return err
	}

	// --- Profile (onboarding — identity verification, requires context) ---
	// Linked to surf-v2-id-verify to enable the Explorer INSUFFICIENT_CONTEXT
	// and context-satisfied scenarios. RequiredContextKeys forces customer_id
	// to be present in the request.

	if err := ensureProfile(ctx, repos.Profiles, &authority.AuthorityProfile{
		ID:          "profile-v2-onboarding",
		Version:     1,
		SurfaceID:   "surf-v2-id-verify",
		Name:        "Onboarding Context Authority",
		Description: "Authority profile for consumer identity verification requiring customer context",

		Status:        authority.ProfileStatusActive,
		EffectiveDate: effective,

		ConfidenceThreshold: 0.80,
		ConsequenceThreshold: authority.Consequence{
			Type:     value.ConsequenceTypeMonetary,
			Amount:   2000,
			Currency: "GBP",
		},

		EscalationMode:      authority.EscalationModeAuto,
		FailMode:            authority.FailModeClosed,
		RequiredContextKeys: []string{"customer_id"},

		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return err
	}

	// --- Grant (onboarding — identity verification) ---

	if err := ensureGrant(ctx, repos.Grants, &authority.AuthorityGrant{
		ID:            "grant-v2-onboarding",
		AgentID:       "agent-v2-evaluator",
		ProfileID:     "profile-v2-onboarding",
		GrantedBy:     "system",
		EffectiveDate: effective,
		Status:        authority.GrantStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		return err
	}

	return nil
}

// ---------------------------------------------------------------------------
// Per-entity ensure helpers
//
// Each helper looks up the row by its stable identity, and only Creates
// when no row exists. Existing rows are returned unchanged — the seed
// never calls Update, never overwrites fields, never deletes. This is
// the load-bearing property that makes SeedDemo safe to re-run on every
// startup and self-healing when a future release adds new demo entities
// to a deployment that was first seeded with an older dataset.
//
// Errors from the lookup are wrapped with the entity kind + identity so
// the failing row is obvious in startup logs. A Create failure is
// reported the same way.
// ---------------------------------------------------------------------------

func ensureBusinessService(ctx context.Context, repo businessservice.BusinessServiceRepository, bs *businessservice.BusinessService) error {
	existing, err := repo.GetByID(ctx, bs.ID)
	if err != nil {
		return fmt.Errorf("lookup business service %s: %w", bs.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, bs); err != nil {
		return fmt.Errorf("create business service %s: %w", bs.ID, err)
	}
	return nil
}

func ensureCapability(ctx context.Context, repo capability.CapabilityRepository, c *capability.Capability) error {
	existing, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("lookup capability %s: %w", c.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, c); err != nil {
		return fmt.Errorf("create capability %s: %w", c.ID, err)
	}
	return nil
}

// ensureBSC uses the natural key (business_service_id, capability_id) via
// the repository's Exists method — the BSC table has no synthetic ID.
func ensureBSC(ctx context.Context, repo businessservicecapability.BusinessServiceCapabilityRepository, bsc *businessservicecapability.BusinessServiceCapability) error {
	exists, err := repo.Exists(ctx, bsc.BusinessServiceID, bsc.CapabilityID)
	if err != nil {
		return fmt.Errorf("lookup business_service_capability %s↔%s: %w", bsc.BusinessServiceID, bsc.CapabilityID, err)
	}
	if exists {
		return nil
	}
	if err := repo.Create(ctx, bsc); err != nil {
		return fmt.Errorf("create business_service_capability %s↔%s: %w", bsc.BusinessServiceID, bsc.CapabilityID, err)
	}
	return nil
}

func ensureProcess(ctx context.Context, repo process.ProcessRepository, p *process.Process) error {
	existing, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("lookup process %s: %w", p.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, p); err != nil {
		return fmt.Errorf("create process %s: %w", p.ID, err)
	}
	return nil
}

// ensureSurface uses FindLatestByID. A surface ID may exist at any version
// (the seed always inserts Version=1, but a deployment may have already
// promoted a v2). Either way, "ID exists at any version" is the correct
// presence check for the seed: if any version is present we do not insert
// the seeded v1, since doing so would conflict with the (id, version)
// uniqueness constraint AND silently downgrade an existing v2.
func ensureSurface(ctx context.Context, repo surface.SurfaceRepository, s *surface.DecisionSurface) error {
	existing, err := repo.FindLatestByID(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("lookup surface %s: %w", s.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, s); err != nil {
		return fmt.Errorf("create surface %s: %w", s.ID, err)
	}
	return nil
}

func ensureAgent(ctx context.Context, repo agent.AgentRepository, a *agent.Agent) error {
	existing, err := repo.GetByID(ctx, a.ID)
	if err != nil {
		return fmt.Errorf("lookup agent %s: %w", a.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, a); err != nil {
		return fmt.Errorf("create agent %s: %w", a.ID, err)
	}
	return nil
}

// ensureProfile uses FindByIDAndVersion: profiles are versioned, the seed
// always inserts Version=1, and the desired idempotency property is
// "create if (id, version) does not yet exist". Using FindByID (latest)
// instead would refuse to insert a missing v1 when only a later version
// happens to exist, which would be wrong for stale-partial-seed repair.
func ensureProfile(ctx context.Context, repo authority.ProfileRepository, p *authority.AuthorityProfile) error {
	existing, err := repo.FindByIDAndVersion(ctx, p.ID, p.Version)
	if err != nil {
		return fmt.Errorf("lookup profile %s v%d: %w", p.ID, p.Version, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, p); err != nil {
		return fmt.Errorf("create profile %s v%d: %w", p.ID, p.Version, err)
	}
	return nil
}

func ensureGrant(ctx context.Context, repo authority.GrantRepository, g *authority.AuthorityGrant) error {
	existing, err := repo.FindByID(ctx, g.ID)
	if err != nil {
		return fmt.Errorf("lookup grant %s: %w", g.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, g); err != nil {
		return fmt.Errorf("create grant %s: %w", g.ID, err)
	}
	return nil
}
