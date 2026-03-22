package apply

import (
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/value"
)

// mapAgentDocumentToAgent converts a validated AgentDocument into an Agent
// domain model ready for persistence.
//
// Agent type is mapped from the document spec.type field. The operational state
// is derived from spec.status: "active" maps to OperationalStateActive; any
// other value maps to OperationalStateSuspended. Timestamps are normalized to UTC.
func mapAgentDocumentToAgent(doc types.AgentDocument, now time.Time) (*agent.Agent, error) {
	now = now.UTC()

	agentType, err := mapAgentType(doc.Spec.Type)
	if err != nil {
		return nil, err
	}

	state := agent.OperationalStateActive
	if strings.TrimSpace(doc.Spec.Status) != "active" {
		state = agent.OperationalStateSuspended
	}

	return &agent.Agent{
		ID:               strings.TrimSpace(doc.Metadata.ID),
		Name:             strings.TrimSpace(doc.Metadata.Name),
		Type:             agentType,
		ModelVersion:     strings.TrimSpace(doc.Spec.Runtime.Version),
		OperationalState: state,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func mapAgentType(docType string) (agent.AgentType, error) {
	switch strings.TrimSpace(docType) {
	case "llm_agent", "copilot":
		return agent.AgentTypeAI, nil
	case "workflow", "automation", "rpa":
		return agent.AgentTypeService, nil
	default:
		return "", fmt.Errorf("unrecognised agent type %q; cannot map to domain AgentType", docType)
	}
}

// mapProfileDocumentToAuthorityProfile converts a validated ProfileDocument into
// an AuthorityProfile domain model ready for persistence.
//
// version is the version number assigned by the planner: 1 for a first-time
// create, N+1 when appending to an existing profile lineage. Profiles are
// persisted with status active; the document schema does not carry a governance
// review workflow for profiles in the current apply path. Timestamps are
// normalized to UTC.
func mapProfileDocumentToAuthorityProfile(doc types.ProfileDocument, now time.Time, createdBy string, version int) (*authority.AuthorityProfile, error) {
	now = now.UTC()

	consequence, err := mapConsequenceThreshold(doc.Spec.Authority.ConsequenceThreshold)
	if err != nil {
		return nil, err
	}

	failMode := authority.FailModeClosed
	if strings.TrimSpace(doc.Spec.Policy.FailMode) == "open" {
		failMode = authority.FailModeOpen
	}

	effectiveFrom := now
	if strings.TrimSpace(doc.Spec.Lifecycle.EffectiveFrom) != "" {
		parsed, err := time.Parse(time.RFC3339, doc.Spec.Lifecycle.EffectiveFrom)
		if err != nil {
			return nil, fmt.Errorf("invalid lifecycle.effective_from: %w", err)
		}
		effectiveFrom = parsed.UTC()
	}

	return &authority.AuthorityProfile{
		ID:                   strings.TrimSpace(doc.Metadata.ID),
		Version:              version,
		SurfaceID:            strings.TrimSpace(doc.Spec.SurfaceID),
		Name:                 strings.TrimSpace(doc.Metadata.Name),
		Status:               authority.ProfileStatusReview,
		EffectiveDate:        effectiveFrom,
		ConfidenceThreshold:  doc.Spec.Authority.DecisionConfidenceThreshold,
		ConsequenceThreshold: consequence,
		PolicyReference:      strings.TrimSpace(doc.Spec.Policy.Reference),
		FailMode:             failMode,
		RequiredContextKeys:  append([]string(nil), doc.Spec.InputRequirements.RequiredContext...),
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            strings.TrimSpace(createdBy),
	}, nil
}

func mapConsequenceThreshold(ct types.ConsequenceThreshold) (authority.Consequence, error) {
	switch strings.TrimSpace(ct.Type) {
	case "monetary":
		return authority.Consequence{
			Type:     value.ConsequenceTypeMonetary,
			Amount:   ct.Amount,
			Currency: strings.TrimSpace(ct.Currency),
		}, nil
	case "risk_rating":
		rr, err := mapRiskRating(ct.RiskRating)
		if err != nil {
			return authority.Consequence{}, err
		}
		return authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: rr,
		}, nil
	case "":
		// No consequence threshold specified; return zero value.
		return authority.Consequence{}, nil
	default:
		return authority.Consequence{}, fmt.Errorf("unrecognised consequence threshold type %q", ct.Type)
	}
}

func mapRiskRating(s string) (value.RiskRating, error) {
	switch strings.TrimSpace(s) {
	case "low":
		return value.RiskRatingLow, nil
	case "medium":
		return value.RiskRatingMedium, nil
	case "high":
		return value.RiskRatingHigh, nil
	case "critical":
		return value.RiskRatingCritical, nil
	default:
		return "", fmt.Errorf("unrecognised risk rating %q", s)
	}
}

// mapGrantDocumentToAuthorityGrant converts a validated GrantDocument into an
// AuthorityGrant domain model ready for persistence.
//
// The grant status is mapped directly from the document spec.status field.
// Timestamps are normalized to UTC. EffectiveUntil is optional; if absent, the
// grant has no expiration.
func mapGrantDocumentToAuthorityGrant(doc types.GrantDocument, now time.Time) (*authority.AuthorityGrant, error) {
	now = now.UTC()

	grantStatus, err := mapGrantStatus(doc.Spec.Status)
	if err != nil {
		return nil, err
	}

	effectiveFrom, err := time.Parse(time.RFC3339, doc.Spec.EffectiveFrom)
	if err != nil {
		return nil, fmt.Errorf("invalid spec.effective_from: %w", err)
	}

	var expiresAt *time.Time
	if strings.TrimSpace(doc.Spec.EffectiveUntil) != "" {
		t, err := time.Parse(time.RFC3339, doc.Spec.EffectiveUntil)
		if err != nil {
			return nil, fmt.Errorf("invalid spec.effective_until: %w", err)
		}
		utc := t.UTC()
		expiresAt = &utc
	}

	return &authority.AuthorityGrant{
		ID:            strings.TrimSpace(doc.Metadata.ID),
		AgentID:       strings.TrimSpace(doc.Spec.AgentID),
		ProfileID:     strings.TrimSpace(doc.Spec.ProfileID),
		GrantedBy:     strings.TrimSpace(doc.Spec.GrantedBy),
		Status:        grantStatus,
		EffectiveDate: effectiveFrom.UTC(),
		ExpiresAt:     expiresAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func mapGrantStatus(s string) (authority.GrantStatus, error) {
	switch strings.TrimSpace(s) {
	case "active":
		return authority.GrantStatusActive, nil
	case "suspended":
		return authority.GrantStatusSuspended, nil
	case "revoked", "expired":
		return authority.GrantStatusRevoked, nil
	default:
		return "", fmt.Errorf("unrecognised grant status %q", s)
	}
}
