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

func SeedDemo(ctx context.Context, repos *store.Repositories) error {

	now := time.Now().UTC()

	err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		// Identity
		ID:      "loan_auto_approval",
		Version: 1,

		// Core Evaluation Fields
		RequiredContext:   surface.ContextSchema{Fields: []surface.ContextField{}}, // Empty for now
		ConsequenceTypes:  []surface.ConsequenceType{},                             // Empty for now
		MinimumConfidence: 0.80,
		FailureMode:       surface.FailureModeClosed,

		// Registry & Metadata
		Name:               "Loan Auto Approval",
		Description:        "Auto loan approval decision surface",
		Domain:             "lending",
		DecisionType:       surface.DecisionTypeTactical,
		ReversibilityClass: surface.ReversibilityConditionallyReversible,

		// Lifecycle
		Status:        surface.SurfaceStatusActive,
		EffectiveFrom: now.Add(-time.Hour),

		// Ownership
		BusinessOwner:  "credit-risk",
		TechnicalOwner: "midas",

		// Audit
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return err
	}

	err = repos.Agents.Create(ctx, &agent.Agent{
		ID:               "agent-credit-1",
		Name:             "Credit Decision Agent",
		Type:             agent.AgentTypeAI,
		Owner:            "accept.io",
		ModelVersion:     "v1",
		Endpoint:         "local",
		OperationalState: agent.OperationalStateActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return err
	}

	err = repos.Profiles.Create(ctx, &authority.AuthorityProfile{
		ID:                  "profile-loan-low",
		SurfaceID:           "loan_auto_approval",
		Name:                "Low Risk Loan Authority",
		ConfidenceThreshold: 0.80,
		ConsequenceThreshold: authority.Consequence{
			Type:     value.ConsequenceTypeMonetary,
			Amount:   5000,
			Currency: "GBP",
		},
		PolicyReference:     "",
		EscalationMode:      authority.EscalationModeAuto,
		FailMode:            authority.FailModeClosed,
		RequiredContextKeys: []string{"customer_id", "loan_amount"},
		Version:             1,
		EffectiveDate:       now.Add(-time.Hour),
		CreatedAt:           now,
		UpdatedAt:           now,
	})
	if err != nil {
		return err
	}

	err = repos.Grants.Create(ctx, &authority.AuthorityGrant{
		ID:            "grant-credit-1",
		AgentID:       "agent-credit-1",
		ProfileID:     "profile-loan-low",
		GrantedBy:     "system",
		EffectiveDate: now.Add(-time.Hour),
		Status:        authority.GrantStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return err
	}

	return nil
}
