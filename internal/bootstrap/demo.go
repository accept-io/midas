package bootstrap

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// demoSurfaceID is the stable identifier for the demo surface.
// It matches the IDs used in the MIDAS Explorer sample scenarios so that
// Explorer works out of the box when demo seeding is enabled.
const demoSurfaceID = "surf-payments-approval"

// SeedDemo inserts a minimal demonstration dataset that makes the MIDAS
// Explorer sample scenarios work immediately.
//
// The seed is idempotent: if the demo surface already exists the function
// returns nil without modifying any data. This makes it safe to call on
// every startup.
//
// Demo dataset:
//
//	Surface  surf-payments-approval  — payments approval decision surface
//	Agent    agent-payments-bot      — AI agent authorised to approve payments
//	Profile  profile-payments-std    — standard authority limits
//	Grant    grant-payments-bot-std  — links agent to profile
//
// Authority thresholds:
//
//	Confidence ≥ 0.85   (Explorer Execute scenario sends 0.95 — passes)
//	Consequence ≤ 1000  (Explorer Execute sends 100 — passes; escalate sends 1,000,000 — escalates)
func SeedDemo(ctx context.Context, repos *store.Repositories) error {
	// Idempotency guard: if the demo surface already exists, skip seeding entirely.
	existing, err := repos.Surfaces.FindLatestByID(ctx, demoSurfaceID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	now := time.Now().UTC()
	effective := now.Add(-time.Hour) // retroactively active

	// Surface
	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:      demoSurfaceID,
		Version: 1,

		Name:        "Payments Approval",
		Description: "Governs autonomous approval of payment transactions",
		Domain:      "payments",

		DecisionType:       surface.DecisionTypeTactical,
		ReversibilityClass: surface.ReversibilityConditionallyReversible,
		FailureMode:        surface.FailureModeClosed,

		// No required context keys so Explorer scenarios work without
		// supplying a context map.
		RequiredContext:  surface.ContextSchema{Fields: []surface.ContextField{}},
		ConsequenceTypes: []surface.ConsequenceType{},

		Status:        surface.SurfaceStatusActive,
		EffectiveFrom: effective,

		BusinessOwner:  "payments-team",
		TechnicalOwner: "midas",

		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return err
	}

	// Agent
	if err := repos.Agents.Create(ctx, &agent.Agent{
		ID:               "agent-payments-bot",
		Name:             "Payments Bot",
		Type:             agent.AgentTypeAI,
		Owner:            "payments-team",
		ModelVersion:     "v1",
		Endpoint:         "local",
		OperationalState: agent.OperationalStateActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		return err
	}

	// Profile — must be ProfileStatusActive for FindActiveAt to resolve it.
	// Thresholds are chosen so Explorer's three demo scenarios hit distinct outcomes:
	//   Execute:              confidence=0.95 (≥0.85 ✓), consequence=100   (≤1000 ✓)  → accept
	//   Escalate-confidence:  confidence=0.30 (<0.85 ✗)                               → escalate
	//   Escalate-consequence: confidence=0.95 (≥0.85 ✓), consequence=1,000,000 (>1000 ✗) → escalate
	if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
		ID:          "profile-payments-std",
		Version:     1,
		SurfaceID:   demoSurfaceID,
		Name:        "Standard Payments Authority",
		Description: "Standard authority limits for automated payment approval",

		Status:        authority.ProfileStatusActive,
		EffectiveDate: effective,

		ConfidenceThreshold: 0.85,
		ConsequenceThreshold: authority.Consequence{
			Type:     value.ConsequenceTypeMonetary,
			Amount:   1000,
			Currency: "GBP",
		},

		EscalationMode:      authority.EscalationModeAuto,
		FailMode:            authority.FailModeClosed,
		RequiredContextKeys: []string{}, // no required context

		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return err
	}

	// Grant — links agent to profile
	if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
		ID:            "grant-payments-bot-std",
		AgentID:       "agent-payments-bot",
		ProfileID:     "profile-payments-std",
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
