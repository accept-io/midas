package main

import (
	"context"
	"log"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/httpapi"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
)

func main() {

	surfaces := memory.NewSurfaceRepo()
	profiles := memory.NewProfileRepo()
	grants := memory.NewGrantRepo()
	agents := memory.NewAgentRepo()
	envelopes := memory.NewEnvelopeRepo()

	now := time.Now().UTC()

	err := surfaces.Create(context.Background(), &surface.DecisionSurface{
		ID:             "loan_auto_approval",
		Name:           "Loan Auto Approval",
		Domain:         "lending",
		BusinessOwner:  "credit-risk",
		TechnicalOwner: "midas",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		EffectiveDate:  now.Add(-time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		log.Fatal(err)
	}

	err = agents.Create(context.Background(), &agent.Agent{
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
		log.Fatal(err)
	}

	err = profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  "profile-loan-low",
		SurfaceID:           "loan_auto_approval",
		Name:                "Low Risk Loan Authority",
		ConfidenceThreshold: 0.80,
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
		log.Fatal(err)
	}

	err = grants.Create(context.Background(), &authority.AuthorityGrant{
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
		log.Fatal(err)
	}

	policies := policy.NoOpPolicyEvaluator{}

	orchestrator, err := decision.NewOrchestrator(
		surfaces,
		profiles,
		grants,
		agents,
		envelopes,
		policies,
	)
	if err != nil {
		log.Fatal(err)
	}

	srv := httpapi.NewServer(orchestrator)

	log.Println("MIDAS listening on :8080")
	log.Fatal(srv.ListenAndServe(":8080"))
}
