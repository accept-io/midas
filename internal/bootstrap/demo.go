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
// The seed is idempotent: if the bs-consumer-lending business service already
// exists the function returns nil without modifying any data.
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
//	  profile-v2-standard  (linked to surf-v2-merchant-payment)
//	  grant-v2-standard
//
// Supports both memory and postgres backends.
func SeedDemo(ctx context.Context, repos *store.Repositories) error {
	// Idempotency guard: if the anchor business service already exists, skip.
	existing, err := repos.BusinessServices.GetByID(ctx, "bs-consumer-lending")
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

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
		if err := repos.BusinessServices.Create(ctx, s); err != nil {
			return fmt.Errorf("create business service %s: %w", s.ID, err)
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
		if err := repos.Capabilities.Create(ctx, c); err != nil {
			return fmt.Errorf("create capability %s: %w", c.ID, err)
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
		if err := repos.BusinessServiceCapabilities.Create(ctx, bsc); err != nil {
			return fmt.Errorf("create business_service_capability %s↔%s: %w", bsc.BusinessServiceID, bsc.CapabilityID, err)
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
		if err := repos.Processes.Create(ctx, p); err != nil {
			return fmt.Errorf("create process %s: %w", p.ID, err)
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
		if err := repos.Surfaces.Create(ctx, s); err != nil {
			return fmt.Errorf("create surface %s: %w", s.ID, err)
		}
	}

	// --- Agent ---

	if err := repos.Agents.Create(ctx, &agent.Agent{
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
		return fmt.Errorf("create agent: %w", err)
	}

	// --- Profile ---

	if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
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
		return fmt.Errorf("create profile: %w", err)
	}

	// --- Grant (standard — merchant payment) ---

	if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
		ID:            "grant-v2-standard",
		AgentID:       "agent-v2-evaluator",
		ProfileID:     "profile-v2-standard",
		GrantedBy:     "system",
		EffectiveDate: effective,
		Status:        authority.GrantStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		return fmt.Errorf("create grant: %w", err)
	}

	// --- Profile (onboarding — identity verification, requires context) ---
	// Linked to surf-v2-id-verify to enable the Explorer INSUFFICIENT_CONTEXT
	// and context-satisfied scenarios. RequiredContextKeys forces customer_id
	// to be present in the request.

	if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
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
		return fmt.Errorf("create profile-v2-onboarding: %w", err)
	}

	// --- Grant (onboarding — identity verification) ---

	if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
		ID:            "grant-v2-onboarding",
		AgentID:       "agent-v2-evaluator",
		ProfileID:     "profile-v2-onboarding",
		GrantedBy:     "system",
		EffectiveDate: effective,
		Status:        authority.GrantStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		return fmt.Errorf("create grant-v2-onboarding: %w", err)
	}

	return nil
}
