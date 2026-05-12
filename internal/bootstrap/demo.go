package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/failmode"
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
			OwnerID:     "consumer-lending-team",
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			// D29d demo seed: attach the canonical closed-only
			// FailModePolicy as this BusinessService's default so the
			// runtime resolver exercises the BS-default level of the
			// fail-mode hierarchy walk. The runtime remains
			// evidence-only; this attachment only changes the
			// frequency of FAIL_MODE_POLICY_RESOLVED emission for
			// evaluations against any Surface under this service.
			FailModePolicyID: "fmp-demo-default",
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			ID:          "bs-merchant-services",
			Name:        "Merchant Services",
			Description: "Payment processing and fraud prevention for merchants",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			OwnerID:     "merchant-services-team",
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "bs-retail-banking",
			Name:        "Retail Banking",
			Description: "Personal accounts, deposits, statements, and everyday banking for individual customers",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			OwnerID:     "retail-banking-team",
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "bs-payments",
			Name:        "Payments",
			Description: "Domestic and cross-border payment execution, including faster payments and wire transfers",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			OwnerID:     "payments-team",
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "bs-cards",
			Name:        "Cards",
			Description: "Card issuance, lifecycle management, and dispute resolution for debit and credit cards",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			OwnerID:     "cards-team",
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "bs-customer-onboarding",
			Name:        "Customer Onboarding",
			Description: "End-to-end customer acquisition: KYC, party authentication, vulnerability assessment, account opening",
			ServiceType: businessservice.ServiceTypeCustomerFacing,
			OwnerID:     "onboarding-team",
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "bs-fraud-financial-crime",
			Name:        "Fraud & Financial Crime",
			Description: "Cross-domain fraud detection, AML screening, and financial-crime case management",
			ServiceType: businessservice.ServiceTypeInternal,
			OwnerID:     "ffc-team",
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

	// BIAN-aligned capability hierarchy. Top-level "domain"
	// capabilities (no parent) group leaf capabilities into recognised
	// banking service domains. Existing pre-Phase-2B capabilities
	// (cap-identity-verification, cap-credit-scoring, cap-fraud-
	// detection, cap-payment-authorization) remain at the top level
	// for idempotency — the ensure helper does not Update existing
	// rows, so retro-fitting parents onto them would silently no-op
	// on already-seeded deployments. New leaf capabilities use
	// ParentCapabilityID to nest under domain parents.
	caps := []*capability.Capability{
		// --- Existing leaf capabilities (pre-Phase-2B, no parent) ---
		{
			ID:        "cap-identity-verification",
			Name:      "Identity Verification",
			Status:    "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:        "cap-credit-scoring",
			Name:      "Credit Scoring",
			Status:    "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:        "cap-fraud-detection",
			Name:      "Fraud Detection",
			Status:    "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:        "cap-payment-authorization",
			Name:      "Payment Authorization",
			Status:    "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		// --- BIAN domain parents ---
		{
			ID:          "cap-customer",
			Name:        "Customer",
			Description: "BIAN service domain: customer-centric capabilities (offer, onboarding, authentication, vulnerability)",
			Status:      "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:          "cap-product",
			Name:        "Product Management",
			Description: "BIAN service domain: product specification, fulfillment, and lifecycle",
			Status:      "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:          "cap-credit",
			Name:        "Credit",
			Description: "BIAN service domain: credit risk, scoring, administration, and collections",
			Status:      "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:          "cap-payment",
			Name:        "Payments",
			Description: "BIAN service domain: payment authorization, execution, clearing, settlement",
			Status:      "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:          "cap-financial-crime",
			Name:        "Financial Crime",
			Description: "BIAN service domain: fraud, AML, sanctions, and case management",
			Status:      "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:          "cap-card-ops",
			Name:        "Card Operations",
			Description: "BIAN service domain: card issuance, lifecycle, dispute resolution",
			Status:      "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		// --- New leaf capabilities under domains ---
		{
			ID:                 "cap-customer-offer",
			Name:               "Customer Offer",
			Description:        "Tailored product offer presentation",
			ParentCapabilityID: "cap-customer",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-customer-onboarding",
			Name:               "Customer Onboarding",
			Description:        "Customer acquisition and account opening lifecycle",
			ParentCapabilityID: "cap-customer",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-party-authentication",
			Name:               "Party Authentication",
			Description:        "Authentication of customer identity claims",
			ParentCapabilityID: "cap-customer",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-vulnerability-assessment",
			Name:               "Customer Vulnerability Assessment",
			Description:        "Detection and management of vulnerable customer indicators",
			ParentCapabilityID: "cap-customer",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-product-fulfillment",
			Name:               "Product Fulfillment",
			Description:        "Provisioning of products into customer accounts",
			ParentCapabilityID: "cap-product",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-credit-administration",
			Name:               "Credit Administration",
			Description:        "Servicing of credit facilities post-origination",
			ParentCapabilityID: "cap-credit",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-collections",
			Name:               "Collections",
			Description:        "Recovery of overdue credit balances",
			ParentCapabilityID: "cap-credit",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-payment-execution",
			Name:               "Payment Execution",
			Description:        "Outbound payment instruction execution and settlement",
			ParentCapabilityID: "cap-payment",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-fraud-evaluation",
			Name:               "Fraud Evaluation",
			Description:        "Real-time fraud risk evaluation per transaction or event",
			ParentCapabilityID: "cap-financial-crime",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-aml-screening",
			Name:               "AML Screening",
			Description:        "Anti-money-laundering screening of parties and transactions",
			ParentCapabilityID: "cap-financial-crime",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-card-issuance",
			Name:               "Card Issuance",
			Description:        "Production and dispatch of physical and virtual cards",
			ParentCapabilityID: "cap-card-ops",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID:                 "cap-dispute-resolution",
			Name:               "Dispute Resolution",
			Description:        "Card transaction dispute and chargeback handling",
			ParentCapabilityID: "cap-card-ops",
			Status:             "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
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
		// Consumer Lending — pre-Phase-2B + new credit-domain caps.
		{BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-identity-verification", CreatedAt: now},
		{BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-credit-scoring", CreatedAt: now},
		{BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-fraud-detection", CreatedAt: now},
		{BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-credit-administration", CreatedAt: now},
		{BusinessServiceID: "bs-consumer-lending", CapabilityID: "cap-collections", CreatedAt: now},
		// Merchant Services — pre-Phase-2B + payment+ffc reuse.
		{BusinessServiceID: "bs-merchant-services", CapabilityID: "cap-fraud-detection", CreatedAt: now},
		{BusinessServiceID: "bs-merchant-services", CapabilityID: "cap-payment-authorization", CreatedAt: now},
		{BusinessServiceID: "bs-merchant-services", CapabilityID: "cap-payment-execution", CreatedAt: now},
		{BusinessServiceID: "bs-merchant-services", CapabilityID: "cap-fraud-evaluation", CreatedAt: now},
		// Retail Banking — customer-facing essentials.
		{BusinessServiceID: "bs-retail-banking", CapabilityID: "cap-customer-offer", CreatedAt: now},
		{BusinessServiceID: "bs-retail-banking", CapabilityID: "cap-product-fulfillment", CreatedAt: now},
		{BusinessServiceID: "bs-retail-banking", CapabilityID: "cap-party-authentication", CreatedAt: now},
		// Payments — payment + fraud domains.
		{BusinessServiceID: "bs-payments", CapabilityID: "cap-payment-authorization", CreatedAt: now},
		{BusinessServiceID: "bs-payments", CapabilityID: "cap-payment-execution", CreatedAt: now},
		{BusinessServiceID: "bs-payments", CapabilityID: "cap-fraud-evaluation", CreatedAt: now},
		{BusinessServiceID: "bs-payments", CapabilityID: "cap-aml-screening", CreatedAt: now},
		// Cards — card-ops + authentication.
		{BusinessServiceID: "bs-cards", CapabilityID: "cap-card-issuance", CreatedAt: now},
		{BusinessServiceID: "bs-cards", CapabilityID: "cap-dispute-resolution", CreatedAt: now},
		{BusinessServiceID: "bs-cards", CapabilityID: "cap-fraud-detection", CreatedAt: now},
		{BusinessServiceID: "bs-cards", CapabilityID: "cap-payment-authorization", CreatedAt: now},
		// Customer Onboarding — full customer-domain stack.
		{BusinessServiceID: "bs-customer-onboarding", CapabilityID: "cap-customer-onboarding", CreatedAt: now},
		{BusinessServiceID: "bs-customer-onboarding", CapabilityID: "cap-party-authentication", CreatedAt: now},
		{BusinessServiceID: "bs-customer-onboarding", CapabilityID: "cap-identity-verification", CreatedAt: now},
		{BusinessServiceID: "bs-customer-onboarding", CapabilityID: "cap-vulnerability-assessment", CreatedAt: now},
		// Fraud & Financial Crime — full FFC stack.
		{BusinessServiceID: "bs-fraud-financial-crime", CapabilityID: "cap-fraud-detection", CreatedAt: now},
		{BusinessServiceID: "bs-fraud-financial-crime", CapabilityID: "cap-fraud-evaluation", CreatedAt: now},
		{BusinessServiceID: "bs-fraud-financial-crime", CapabilityID: "cap-aml-screening", CreatedAt: now},
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
		// --- Existing pre-Phase-2B processes ---
		{
			ID: "proc-consumer-onboarding", Name: "Consumer Onboarding",
			BusinessServiceID: "bs-consumer-lending",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-credit-assessment", Name: "Credit Assessment",
			BusinessServiceID: "bs-consumer-lending",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-merchant-risk-screen", Name: "Merchant Risk Screening",
			BusinessServiceID: "bs-merchant-services",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-merchant-payment-auth", Name: "Merchant Payment Authorization",
			BusinessServiceID: "bs-merchant-services",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		// --- Consumer Lending: collections process ---
		{
			ID: "proc-loan-collections", Name: "Loan Collections",
			BusinessServiceID: "bs-consumer-lending",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		// --- Retail Banking ---
		{
			ID: "proc-account-opening", Name: "Account Opening",
			BusinessServiceID: "bs-retail-banking",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-statement-generation", Name: "Statement Generation",
			BusinessServiceID: "bs-retail-banking",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		// --- Payments ---
		{
			ID: "proc-payment-initiation", Name: "Payment Initiation",
			BusinessServiceID: "bs-payments",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-payment-clearing", Name: "Payment Clearing & Settlement",
			BusinessServiceID: "bs-payments",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-cross-border-transfer", Name: "Cross-Border Transfer",
			BusinessServiceID: "bs-payments",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		// --- Cards ---
		{
			ID: "proc-card-issuance-flow", Name: "Card Issuance",
			BusinessServiceID: "bs-cards",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-card-dispute", Name: "Card Dispute",
			BusinessServiceID: "bs-cards",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		// --- Customer Onboarding ---
		{
			ID: "proc-kyc-collection", Name: "KYC Collection",
			BusinessServiceID: "bs-customer-onboarding",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-vulnerability-screen", Name: "Vulnerability Screening",
			BusinessServiceID: "bs-customer-onboarding",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		// --- Fraud & Financial Crime ---
		{
			ID: "proc-transaction-monitoring", Name: "Transaction Monitoring",
			BusinessServiceID: "bs-fraud-financial-crime",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-aml-investigation", Name: "AML Investigation",
			BusinessServiceID: "bs-fraud-financial-crime",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "proc-financial-crime-case", Name: "Financial Crime Case Management",
			BusinessServiceID: "bs-fraud-financial-crime",
			Status:            "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
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
			RequiredContext:    surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes:   []surface.ConsequenceType{},
			Status:             surface.SurfaceStatusActive,
			EffectiveFrom:      effective,
			BusinessOwner:      "merchant-services-team",
			TechnicalOwner:     "midas",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		// --- Phase 2B Step 11 enrichment: surfaces under new processes ---
		// Density principle: not every process owns a surface (e.g.
		// proc-payment-clearing, proc-financial-crime-case are
		// deliberately surfaceless to demonstrate sparse rows in the
		// graph). Some surfaces are governed (have profile/grant);
		// others are intentionally ungoverned to demonstrate coverage
		// gaps in the authority overlay.
		{
			ID: "surf-v2-collections-priority", Version: 1,
			Name: "Collections Call Priority", Description: "Prioritises overdue accounts for collections outreach",
			Domain: "consumer-lending", ProcessID: "proc-loan-collections",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "consumer-lending-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-account-eligibility", Version: 1,
			Name: "Account Opening Eligibility", Description: "Determines eligibility to open a retail account",
			Domain: "retail-banking", ProcessID: "proc-account-opening",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityConditionallyReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "retail-banking-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-statement-suppression", Version: 1,
			Name: "Statement Suppression", Description: "Suppresses statement generation under specified conditions",
			Domain: "retail-banking", ProcessID: "proc-statement-generation",
			DecisionType: surface.DecisionTypeOperational, ReversibilityClass: surface.ReversibilityReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "retail-banking-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-payment-initiation-check", Version: 1,
			Name: "Payment Initiation Check", Description: "Pre-authorisation checks for outbound payments",
			Domain: "payments", ProcessID: "proc-payment-initiation",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityConditionallyReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "payments-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-sanctions-screen", Version: 1,
			Name: "Cross-Border Sanctions Screen", Description: "Sanctions screening for cross-border transfers",
			Domain: "payments", ProcessID: "proc-cross-border-transfer",
			DecisionType: surface.DecisionTypeStrategic, ReversibilityClass: surface.ReversibilityIrreversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "payments-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-card-issuance-decision", Version: 1,
			Name: "Card Issuance Decision", Description: "Approves or declines card issuance requests",
			Domain: "cards", ProcessID: "proc-card-issuance-flow",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityConditionallyReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "cards-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-dispute-triage", Version: 1,
			Name: "Card Dispute Triage", Description: "Triage of card disputes into chargeback / decline / investigation paths",
			Domain: "cards", ProcessID: "proc-card-dispute",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityConditionallyReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "cards-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-kyc-evaluation", Version: 1,
			Name: "KYC Evaluation", Description: "Evaluates KYC completeness and risk for new customers",
			Domain: "customer-onboarding", ProcessID: "proc-kyc-collection",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityConditionallyReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "onboarding-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-vulnerability-flag", Version: 1,
			Name: "Vulnerability Flag", Description: "Flags potentially vulnerable customers for human review",
			Domain: "customer-onboarding", ProcessID: "proc-vulnerability-screen",
			DecisionType: surface.DecisionTypeStrategic, ReversibilityClass: surface.ReversibilityReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "onboarding-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-tx-anomaly", Version: 1,
			Name: "Transaction Anomaly Detection", Description: "Real-time anomaly detection on transaction streams",
			Domain: "fraud-financial-crime", ProcessID: "proc-transaction-monitoring",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityConditionallyReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "ffc-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-tx-velocity", Version: 1,
			Name: "Transaction Velocity Check", Description: "Velocity-based screening for unusual transaction frequency",
			Domain: "fraud-financial-crime", ProcessID: "proc-transaction-monitoring",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "ffc-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "surf-v2-aml-alert-triage", Version: 1,
			Name: "AML Alert Triage", Description: "Triages AML alerts into investigation, dismissal, or escalation",
			Domain: "fraud-financial-crime", ProcessID: "proc-aml-investigation",
			DecisionType: surface.DecisionTypeTactical, ReversibilityClass: surface.ReversibilityConditionallyReversible,
			RequiredContext: surface.ContextSchema{Fields: []surface.ContextField{}},
			ConsequenceTypes: []surface.ConsequenceType{}, Status: surface.SurfaceStatusActive,
			EffectiveFrom: effective, BusinessOwner: "ffc-team", TechnicalOwner: "midas",
			CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, s := range surfs {
		if err := ensureSurface(ctx, repos.Surfaces, s); err != nil {
			return err
		}
	}

	// --- Business-service relationships (Phase 2B Step 11) ---
	// Realistic banking dependencies. Includes a bidirectional pair
	// (bs-payments ↔ bs-fraud-financial-crime) using two distinct
	// relationship types — payments depends_on fraud (for screening)
	// and fraud supports payments (as the screening provider). The
	// schema's uniq_bsr_triple constraint is on (source, target,
	// type), so the two rows coexist legally.
	bsRels := []*businessservice.BusinessServiceRelationship{
		{
			ID:                    "rel-payments-deps-fraud",
			SourceBusinessService: "bs-payments", TargetBusinessService: "bs-fraud-financial-crime",
			RelationshipType: "depends_on",
			Description:      "Payments must invoke fraud screening before authorising outbound transfers",
			CreatedAt:        now, CreatedBy: "system",
		},
		{
			ID:                    "rel-fraud-supports-payments",
			SourceBusinessService: "bs-fraud-financial-crime", TargetBusinessService: "bs-payments",
			RelationshipType: "supports",
			Description:      "Fraud & Financial Crime supports Payments by providing real-time screening",
			CreatedAt:        now, CreatedBy: "system",
		},
		{
			ID:                    "rel-cards-supports-retail",
			SourceBusinessService: "bs-cards", TargetBusinessService: "bs-retail-banking",
			RelationshipType: "supports",
			Description:      "Cards extends Retail Banking with debit/credit card products",
			CreatedAt:        now, CreatedBy: "system",
		},
		{
			ID:                    "rel-consumer-lending-deps-onboarding",
			SourceBusinessService: "bs-consumer-lending", TargetBusinessService: "bs-customer-onboarding",
			RelationshipType: "depends_on",
			Description:      "Consumer Lending requires customer onboarding to have completed before origination",
			CreatedAt:        now, CreatedBy: "system",
		},
		{
			ID:                    "rel-merchant-services-deps-fraud",
			SourceBusinessService: "bs-merchant-services", TargetBusinessService: "bs-fraud-financial-crime",
			RelationshipType: "depends_on",
			Description:      "Merchant Services routes transactions through Fraud & Financial Crime screening",
			CreatedAt:        now, CreatedBy: "system",
		},
		{
			ID:                    "rel-payments-supports-cards",
			SourceBusinessService: "bs-payments", TargetBusinessService: "bs-cards",
			RelationshipType: "supports",
			Description:      "Payments executes the payment leg of card transactions",
			CreatedAt:        now, CreatedBy: "system",
		},
		{
			ID:                    "rel-retail-banking-part-of-onboarding",
			SourceBusinessService: "bs-retail-banking", TargetBusinessService: "bs-customer-onboarding",
			RelationshipType: "depends_on",
			Description:      "Retail Banking onboarding flows are a customer-onboarding consumer",
			CreatedAt:        now, CreatedBy: "system",
		},
		{
			ID:                    "rel-customer-onboarding-deps-fraud",
			SourceBusinessService: "bs-customer-onboarding", TargetBusinessService: "bs-fraud-financial-crime",
			RelationshipType: "depends_on",
			Description:      "Onboarding requires sanctions / AML screening before account opening",
			CreatedAt:        now, CreatedBy: "system",
		},
	}
	for _, r := range bsRels {
		if err := ensureBSR(ctx, repos.BusinessServiceRelationships, r); err != nil {
			return err
		}
	}

	// --- Agents ---

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
	if err := ensureAgent(ctx, repos.Agents, &agent.Agent{
		ID:               "agent-v2-fraud-bot",
		Name:             "Fraud Detection Bot",
		Type:             agent.AgentTypeAI,
		Owner:            "ffc-team",
		ModelVersion:     "v1",
		Endpoint:         "local",
		OperationalState: agent.OperationalStateActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		return err
	}

	// --- Profile (standard — merchant payment) ---
	//
	// Consequence type: risk_rating. The domain enum and the Postgres
	// schema constraint chk_profiles_consequence_type only intersect on
	// 'risk_rating' — the domain's other constant (monetary) is not
	// accepted by the schema's CHECK, which uses 'financial'. Reconciling
	// that naming mismatch is a separate cross-cutting refactor; the demo
	// seed uses the value that's valid in BOTH the domain code and the
	// schema so it can be re-applied against Postgres on every startup
	// without violating chk_profiles_consequence_type. RiskRatingHigh
	// fits the merchant-payment-authorisation narrative (high-impact
	// transactional decision).

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
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},

		EscalationMode:      authority.EscalationModeAuto,
		EscalationTargetID:  "et-governance-approver",
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
		// D31i: standard baseline — the demo evaluator may
		// recommend or approve merchant-payment authorisations.
		// No stop authority and no constraints; the Authority
		// Graph shows this grant as a healthy posture baseline.
		Capabilities: []authority.Capability{
			authority.CapabilityRecommend,
			authority.CapabilityApprove,
		},
	}); err != nil {
		return err
	}

	// --- Profile (onboarding — identity verification, requires context) ---
	// Linked to surf-v2-id-verify to enable the Explorer INSUFFICIENT_CONTEXT
	// and context-satisfied scenarios. RequiredContextKeys forces customer_id
	// to be present in the request. RiskRatingMedium reflects the
	// onboarding-identity-verification narrative (lower-stakes than the
	// merchant-payment-authorisation profile above). See the consequence-
	// type rationale on profile-v2-standard.

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
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingMedium,
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
		Capabilities: []authority.Capability{
			authority.CapabilityRecommend,
			authority.CapabilityApprove,
		},
	}); err != nil {
		return err
	}

	// --- Additional profiles + grants (Phase 2B Step 11) ---
	// Spreads governance across more surfaces so the Authority Graph
	// shows mixed coverage: surf-v2-credit-assess and
	// surf-v2-consumer-fraud are now governed; many other surfaces
	// remain ungoverned to illustrate coverage gaps.

	if err := ensureProfile(ctx, repos.Profiles, &authority.AuthorityProfile{
		ID:          "profile-v2-credit-assess",
		Version:     1,
		SurfaceID:   "surf-v2-credit-assess",
		Name:        "Credit Assessment Authority",
		Description: "Authority profile for automated credit assessment",
		Status:      authority.ProfileStatusActive,
		EffectiveDate: effective, ConfidenceThreshold: 0.82,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingMedium,
		},
		EscalationMode: authority.EscalationModeAuto, FailMode: authority.FailModeClosed,
		RequiredContextKeys: []string{"customer_id"},
		CreatedAt:           now, UpdatedAt: now,
	}); err != nil {
		return err
	}

	if err := ensureGrant(ctx, repos.Grants, &authority.AuthorityGrant{
		ID:            "grant-v2-credit-assess",
		AgentID:       "agent-v2-evaluator",
		ProfileID:     "profile-v2-credit-assess",
		GrantedBy:     "system",
		EffectiveDate: effective,
		Status:        authority.GrantStatusActive,
		CreatedAt:     now, UpdatedAt: now,
		Capabilities: []authority.Capability{
			authority.CapabilityRecommend,
			authority.CapabilityApprove,
		},
		// D31i: a confidence_threshold_min constraint demonstrates the
		// runtime-narrowing semantics in the demo — credit assessment
		// approvals require ≥0.90 confidence on top of the profile's
		// own 0.82 threshold.
		Constraints: []authority.Constraint{
			{Kind: authority.ConstraintKindConfidenceThresholdMin, MinConfidence: 0.90},
		},
	}); err != nil {
		return err
	}

	if err := ensureProfile(ctx, repos.Profiles, &authority.AuthorityProfile{
		ID:          "profile-v2-fraud-detection",
		Version:     1,
		SurfaceID:   "surf-v2-consumer-fraud",
		Name:        "Consumer Fraud Detection Authority",
		Description: "Authority profile for automated consumer fraud screening",
		Status:      authority.ProfileStatusActive,
		EffectiveDate: effective, ConfidenceThreshold: 0.90,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		EscalationMode: authority.EscalationModeAuto, FailMode: authority.FailModeClosed,
		RequiredContextKeys: []string{},
		CreatedAt:           now, UpdatedAt: now,
	}); err != nil {
		return err
	}

	if err := ensureGrant(ctx, repos.Grants, &authority.AuthorityGrant{
		ID:            "grant-v2-fraud-detection",
		AgentID:       "agent-v2-fraud-bot",
		ProfileID:     "profile-v2-fraud-detection",
		GrantedBy:     "system",
		EffectiveDate: effective,
		Status:        authority.GrantStatusActive,
		CreatedAt:     now, UpdatedAt: now,
		// D31i: fraud detection exercises the full authority spine
		// including stop. The bot may recommend, escalate, reject,
		// or invoke a kill-switch via the stop capability.
		Capabilities: []authority.Capability{
			authority.CapabilityRecommend,
			authority.CapabilityEscalate,
			authority.CapabilityReject,
			authority.CapabilityStop,
		},
	}); err != nil {
		return err
	}

	// --- AI systems (Phase 2B Step 11) ---
	// Six AI systems modelling realistic banking AI usage. Each has
	// one active version. Bindings cover all four scopes; one system
	// (aisys-fraud-detection) is intentionally bound at every scope
	// kind to demonstrate multi-scope reframe traversal.
	aiSystems := []*aisystem.AISystem{
		{
			ID: "aisys-identity-verification", Name: "Identity Verification AI",
			Description: "Document and biometric identity verification model",
			Owner:       "ffc-team", Vendor: "Acme ID", SystemType: "ml-model",
			Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual, Managed: true,
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			ID: "aisys-credit-risk-scoring", Name: "Credit Risk Scoring AI",
			Description: "Probability-of-default model for consumer credit",
			Owner:       "consumer-lending-team", Vendor: "RiskMetrics", SystemType: "ml-model",
			Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual, Managed: true,
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			ID: "aisys-fraud-detection", Name: "Transaction Fraud Detection AI",
			Description: "Multi-channel real-time transaction fraud detection ensemble",
			Owner:       "ffc-team", Vendor: "FraudShield", SystemType: "ensemble",
			Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual, Managed: true,
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			ID: "aisys-card-dispute-triage", Name: "Card Dispute Triage AI",
			Description: "Auto-classifies card disputes into chargeback, decline, or investigation",
			Owner:       "cards-team", Vendor: "DisputeIQ", SystemType: "classifier",
			Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual, Managed: true,
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			ID: "aisys-collections-priority", Name: "Collections Prioritisation AI",
			Description: "Ranks delinquent accounts by recovery probability",
			Owner:       "consumer-lending-team", Vendor: "Recovery.ai", SystemType: "ranker",
			Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual, Managed: true,
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			ID: "aisys-vulnerability-detection", Name: "Customer Vulnerability Detection AI",
			Description: "Detects indicators of customer vulnerability from interaction signals",
			Owner:       "onboarding-team", Vendor: "Vista", SystemType: "ml-model",
			Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual, Managed: true,
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
	}
	for _, s := range aiSystems {
		if err := ensureAISystem(ctx, repos.AISystems, s); err != nil {
			return err
		}
	}

	aiVersions := []*aisystem.AISystemVersion{
		{
			AISystemID: "aisys-identity-verification", Version: 1, ReleaseLabel: "v1.0",
			ModelArtifact: "model://identity-verify/v1", Status: aisystem.AISystemVersionStatusActive,
			EffectiveFrom: effective, ComplianceFrameworks: []string{"iso-42001"},
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			AISystemID: "aisys-credit-risk-scoring", Version: 1, ReleaseLabel: "v1.0",
			ModelArtifact: "model://credit-risk/v1", Status: aisystem.AISystemVersionStatusActive,
			EffectiveFrom: effective, ComplianceFrameworks: []string{"iso-42001"},
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			AISystemID: "aisys-fraud-detection", Version: 1, ReleaseLabel: "v1.0",
			ModelArtifact: "model://fraud-detect/v1", Status: aisystem.AISystemVersionStatusActive,
			EffectiveFrom: effective, ComplianceFrameworks: []string{"iso-42001"},
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			AISystemID: "aisys-card-dispute-triage", Version: 1, ReleaseLabel: "v1.0",
			ModelArtifact: "model://card-dispute/v1", Status: aisystem.AISystemVersionStatusActive,
			EffectiveFrom: effective, ComplianceFrameworks: []string{},
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			AISystemID: "aisys-collections-priority", Version: 1, ReleaseLabel: "v1.0",
			ModelArtifact: "model://collections-rank/v1", Status: aisystem.AISystemVersionStatusActive,
			EffectiveFrom: effective, ComplianceFrameworks: []string{},
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
		{
			AISystemID: "aisys-vulnerability-detection", Version: 1, ReleaseLabel: "v1.0",
			ModelArtifact: "model://vulnerability/v1", Status: aisystem.AISystemVersionStatusActive,
			EffectiveFrom: effective, ComplianceFrameworks: []string{"iso-42001"},
			CreatedAt: now, UpdatedAt: now, CreatedBy: "system",
		},
	}
	for _, v := range aiVersions {
		if err := ensureAISystemVersion(ctx, repos.AISystemVersions, v); err != nil {
			return err
		}
	}

	// AI bindings — exercise all four scope kinds. aisys-fraud-detection
	// is deliberately bound at every scope so that AI-system reframe
	// produces a graph with all four target shapes simultaneously.
	one := 1
	aiBindings := []*aisystem.AISystemBinding{
		// === Surface-scoped ===
		{
			ID: "bind-id-verify-on-surf", AISystemID: "aisys-identity-verification",
			AISystemVersion: &one, SurfaceID: "surf-v2-id-verify",
			Role: "primary", Description: "Primary ID verification on consumer onboarding",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-fraud-on-consumer-fraud-surf", AISystemID: "aisys-fraud-detection",
			AISystemVersion: &one, SurfaceID: "surf-v2-consumer-fraud",
			Role: "primary", Description: "Primary fraud screening on consumer onboarding fraud check",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-fraud-on-merchant-risk-surf", AISystemID: "aisys-fraud-detection",
			AISystemVersion: &one, SurfaceID: "surf-v2-merchant-risk",
			Role: "primary", Description: "Primary fraud screening on merchant risk surface",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-credit-risk-on-credit-surf", AISystemID: "aisys-credit-risk-scoring",
			AISystemVersion: &one, SurfaceID: "surf-v2-credit-assess",
			Role: "primary", Description: "Credit risk model scoring credit assessment surface",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-card-dispute-on-dispute-surf", AISystemID: "aisys-card-dispute-triage",
			AISystemVersion: &one, SurfaceID: "surf-v2-dispute-triage",
			Role: "primary", Description: "Auto-triage of card disputes",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-fraud-on-tx-anomaly-surf", AISystemID: "aisys-fraud-detection",
			AISystemVersion: &one, SurfaceID: "surf-v2-tx-anomaly",
			Role: "shadow", Description: "Shadow ensemble on transaction anomaly detection",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-vuln-on-vuln-flag-surf", AISystemID: "aisys-vulnerability-detection",
			AISystemVersion: &one, SurfaceID: "surf-v2-vulnerability-flag",
			Role: "primary", Description: "Vulnerability detection on flag surface",
			CreatedAt: now, CreatedBy: "system",
		},
		// === Process-scoped ===
		{
			ID: "bind-fraud-on-merchant-risk-proc", AISystemID: "aisys-fraud-detection",
			AISystemVersion: &one, ProcessID: "proc-merchant-risk-screen",
			Role: "ambient", Description: "Process-level fraud screening across merchant risk processes",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-fraud-on-tx-monitoring-proc", AISystemID: "aisys-fraud-detection",
			AISystemVersion: &one, ProcessID: "proc-transaction-monitoring",
			Role: "ambient", Description: "Process-level fraud monitoring",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-credit-risk-on-credit-proc", AISystemID: "aisys-credit-risk-scoring",
			AISystemVersion: &one, ProcessID: "proc-credit-assessment",
			Role: "ambient", Description: "Process-level credit scoring",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-collections-on-collections-proc", AISystemID: "aisys-collections-priority",
			AISystemVersion: &one, ProcessID: "proc-loan-collections",
			Role: "ambient", Description: "Process-level collections prioritisation",
			CreatedAt: now, CreatedBy: "system",
		},
		// === Capability-scoped ===
		{
			ID: "bind-id-verify-on-party-auth-cap", AISystemID: "aisys-identity-verification",
			AISystemVersion: &one, CapabilityID: "cap-party-authentication",
			BusinessServiceID: "bs-customer-onboarding",
			Role: "ambient", Description: "Capability-level ID verification across party authentication",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-fraud-on-fraud-eval-cap", AISystemID: "aisys-fraud-detection",
			AISystemVersion: &one, CapabilityID: "cap-fraud-evaluation",
			BusinessServiceID: "bs-fraud-financial-crime",
			Role: "ambient", Description: "Capability-level fraud evaluation",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-vuln-on-vuln-cap", AISystemID: "aisys-vulnerability-detection",
			AISystemVersion: &one, CapabilityID: "cap-vulnerability-assessment",
			BusinessServiceID: "bs-customer-onboarding",
			Role: "ambient", Description: "Capability-level vulnerability detection",
			CreatedAt: now, CreatedBy: "system",
		},
		// === Business-service-scoped ===
		{
			ID: "bind-fraud-on-ffc-bs", AISystemID: "aisys-fraud-detection",
			AISystemVersion: &one, BusinessServiceID: "bs-fraud-financial-crime",
			Role: "ambient", Description: "Service-wide fraud detection coverage",
			CreatedAt: now, CreatedBy: "system",
		},
		{
			ID: "bind-vuln-on-onboarding-bs", AISystemID: "aisys-vulnerability-detection",
			AISystemVersion: &one, BusinessServiceID: "bs-customer-onboarding",
			Role: "ambient", Description: "Service-wide vulnerability monitoring",
			CreatedAt: now, CreatedBy: "system",
		},
	}
	for _, b := range aiBindings {
		if err := ensureAISystemBinding(ctx, repos.AISystemBindings, b); err != nil {
			return err
		}
	}

	// --- FailModePolicies (D29d demo seed) ---
	// One canonical active, closed-only policy attached to
	// bs-consumer-lending above as its BusinessService default. The
	// runtime resolver remains evidence-only — this seed only
	// exercises the BS-default level of failmode.ResolveWithPath; it
	// does not influence outcomes, audit hash chains, or
	// POLICY_EVALUATED payloads. The (id, version) is FindByIDAndVersion-
	// keyed for per-entity idempotency, mirroring ensureProfile.
	if repos.FailModePolicies != nil {
		failModePolicies := []*failmode.FailModePolicy{
			{
				ID:             "fmp-demo-default",
				Version:        1,
				Name:           "Demo Closed-Only FailModePolicy",
				Description:    "Canonical demo fail-mode policy: closed posture across all permitted classes, evidence-only enforcement.",
				Status:         failmode.FailModePolicyStatusActive,
				EffectiveDate:  effective,
				BusinessOwner:  "consumer-lending-team",
				TechnicalOwner: "platform-runtime-team",
				Rules: []failmode.FailModePolicyRule{
					{
						CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity,
						PermittedMode:    failmode.PermittedModeClosed,
						EnforcementState: failmode.EnforcementStateEvidenceOnly,
						Outcome:          failmode.OutcomeEscalate,
					},
					{
						CorrectnessClass: failmode.CorrectnessClassPersistence,
						PermittedMode:    failmode.PermittedModeClosed,
						EnforcementState: failmode.EnforcementStateEvidenceOnly,
						Outcome:          failmode.OutcomeEscalate,
					},
					{
						// Input must carry not_applicable per the
						// closed-only invariant (D27j-impl-1a) — the
						// validator rejects any other PermittedMode
						// for this class. EnforcementState is forced
						// to evidence_only for not_applicable.
						CorrectnessClass: failmode.CorrectnessClassInput,
						PermittedMode:    failmode.PermittedModeNotApplicable,
						EnforcementState: failmode.EnforcementStateEvidenceOnly,
						Outcome:          failmode.OutcomeEscalate,
					},
					{
						CorrectnessClass: failmode.CorrectnessClassResource,
						PermittedMode:    failmode.PermittedModeClosed,
						EnforcementState: failmode.EnforcementStateEvidenceOnly,
						Outcome:          failmode.OutcomeEscalate,
					},
					{
						CorrectnessClass: failmode.CorrectnessClassConsistency,
						PermittedMode:    failmode.PermittedModeClosed,
						EnforcementState: failmode.EnforcementStateEvidenceOnly,
						Outcome:          failmode.OutcomeEscalate,
					},
				},
				Origin:    "manual",
				Managed:   true,
				CreatedAt: now,
				UpdatedAt: now,
				CreatedBy: "system",
			},
		}
		for _, p := range failModePolicies {
			if err := ensureFailModePolicy(ctx, repos.FailModePolicies, p); err != nil {
				return err
			}
		}
	}

	// --- Escalation targets (D31k-impl-1) -------------------------------
	// Seeds one active role target (et-governance-approver) that
	// profile-v2-standard references via EscalationTargetID. The seed
	// remains a no-op when the repository is unwired (memory-mode
	// tests that construct a partial Repositories may leave it nil).
	if repos.EscalationTargets != nil {
		approvedAt := now
		targets := []*escalation.EscalationTarget{
			{
				ID:             "et-governance-approver",
				Version:        1,
				Name:           "Governance Approver",
				Description:    "Default human reviewer for escalated decisions in the demo dataset",
				Kind:           escalation.KindRole,
				Handle:         "governance.approver",
				Status:         escalation.StatusActive,
				EffectiveDate:  effective,
				BusinessOwner:  "platform",
				TechnicalOwner: "platform",
				CreatedAt:      now,
				UpdatedAt:      now,
				CreatedBy:      "system",
				ApprovedBy:     "system",
				ApprovedAt:     &approvedAt,
			},
		}
		for _, t := range targets {
			if err := ensureEscalationTarget(ctx, repos.EscalationTargets, t); err != nil {
				return err
			}
		}
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

// ensureFailModePolicy uses FindByIDAndVersion for the same reason as
// ensureProfile: FailModePolicy is versioned and the seed inserts a
// specific (id, version) row. FindByID would skip creation when a
// later version exists, which is wrong for partial-seed repair on
// upgrade. The repository's Create assigns the persisted Version from
// the input row's Version field, so the seed remains in control of
// the demo dataset's versioning.
func ensureFailModePolicy(ctx context.Context, repo failmode.PolicyRepository, p *failmode.FailModePolicy) error {
	existing, err := repo.FindByIDAndVersion(ctx, p.ID, p.Version)
	if err != nil {
		return fmt.Errorf("lookup fail mode policy %s v%d: %w", p.ID, p.Version, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, p); err != nil {
		return fmt.Errorf("create fail mode policy %s v%d: %w", p.ID, p.Version, err)
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

// ensureBSR is the per-relationship idempotency helper for the
// directed business_service_relationships table. The repo's GetByID
// returns a sentinel ErrRelationshipNotFound on missing rather than
// (nil, nil) — we treat that error as "not present" and proceed
// to Create.
func ensureBSR(ctx context.Context, repo businessservice.RelationshipRepository, rel *businessservice.BusinessServiceRelationship) error {
	existing, err := repo.GetByID(ctx, rel.ID)
	if err != nil && !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		return fmt.Errorf("lookup business_service_relationship %s: %w", rel.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, rel); err != nil {
		return fmt.Errorf("create business_service_relationship %s: %w", rel.ID, err)
	}
	return nil
}

// ensureAISystem is the per-system idempotency helper. The repo's
// GetByID returns ErrAISystemNotFound on missing.
func ensureAISystem(ctx context.Context, repo aisystem.SystemRepository, sys *aisystem.AISystem) error {
	existing, err := repo.GetByID(ctx, sys.ID)
	if err != nil && !errors.Is(err, aisystem.ErrAISystemNotFound) {
		return fmt.Errorf("lookup ai_system %s: %w", sys.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, sys); err != nil {
		return fmt.Errorf("create ai_system %s: %w", sys.ID, err)
	}
	return nil
}

// ensureAISystemVersion uses (ai_system_id, version) — versions are
// composite-keyed at the schema level and the seed always inserts
// version=1 today. Repo returns ErrAISystemVersionNotFound on missing.
func ensureAISystemVersion(ctx context.Context, repo aisystem.VersionRepository, ver *aisystem.AISystemVersion) error {
	existing, err := repo.GetByIDAndVersion(ctx, ver.AISystemID, ver.Version)
	if err != nil && !errors.Is(err, aisystem.ErrAISystemVersionNotFound) {
		return fmt.Errorf("lookup ai_system_version %s v%d: %w", ver.AISystemID, ver.Version, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, ver); err != nil {
		return fmt.Errorf("create ai_system_version %s v%d: %w", ver.AISystemID, ver.Version, err)
	}
	return nil
}

// ensureAISystemBinding is the per-binding idempotency helper.
// Bindings have synthetic IDs and no triple-uniqueness rule (the
// domain explicitly permits multiple bindings of the same AI system
// to the same scope target with different roles), so GetByID is the
// presence check. Repo returns ErrAISystemBindingNotFound on missing.
func ensureAISystemBinding(ctx context.Context, repo aisystem.BindingRepository, b *aisystem.AISystemBinding) error {
	existing, err := repo.GetByID(ctx, b.ID)
	if err != nil && !errors.Is(err, aisystem.ErrAISystemBindingNotFound) {
		return fmt.Errorf("lookup ai_system_binding %s: %w", b.ID, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, b); err != nil {
		return fmt.Errorf("create ai_system_binding %s: %w", b.ID, err)
	}
	return nil
}

// ensureEscalationTarget mirrors ensureFailModePolicy / ensureProfile:
// look up by (id, version) and only Create when the row is absent.
// EscalationTarget is versioned; the demo seed always inserts
// Version=1 and the desired idempotency is "create if (id, version)
// does not yet exist" — same posture as the other versioned-entity
// seed helpers.
func ensureEscalationTarget(ctx context.Context, repo escalation.Repository, t *escalation.EscalationTarget) error {
	existing, err := repo.FindByIDAndVersion(ctx, t.ID, t.Version)
	if err != nil {
		return fmt.Errorf("lookup escalation target %s v%d: %w", t.ID, t.Version, err)
	}
	if existing != nil {
		return nil
	}
	if err := repo.Create(ctx, t); err != nil {
		return fmt.Errorf("create escalation target %s v%d: %w", t.ID, t.Version, err)
	}
	return nil
}
