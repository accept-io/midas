package httpapi

// failmode_policy_response.go — wire-format DTOs and mappers for the
// D29d FailModePolicy read API. Conventions mirror drift_response.go:
//
//   - List wrappers always include a non-null array (even empty).
//   - Nullable timestamps use *time.Time (zero in domain → null in JSON).
//   - Domain enums are stringified; the OpenAPI enum definitions are
//     the contract for valid wire values.
//   - The wire shape exposes the D29b three-axis rule fields
//     (permitted_mode, enforcement_state, outcome) — the runtime
//     remains evidence-only and does not consult rule contents.

import (
	"time"

	"github.com/accept-io/midas/internal/failmode"
)

// failModePolicyRuleResponse is the wire shape for a single
// FailModePolicyRule. Axis B (enforcement_state) and Axis C (outcome)
// are emitted verbatim; defaults are applied at the repository
// deserialisation layer and inside failmode.Validate (per D29b), so
// callers see effective values.
type failModePolicyRuleResponse struct {
	CorrectnessClass string `json:"correctness_class"`
	PermittedMode    string `json:"permitted_mode"`
	EnforcementState string `json:"enforcement_state,omitempty"`
	Outcome          string `json:"outcome,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// failModePolicyResponse is the wire shape for a single FailModePolicy
// revision returned by the GET endpoints. Nullable timestamps use
// *time.Time without omitempty so the JSON output emits explicit null
// (matching the OpenAPI `nullable: true` declarations) instead of
// dropping the key — same convention as drift_response.go.
type failModePolicyResponse struct {
	ID                string                       `json:"id"`
	Version           int                          `json:"version"`
	Name              string                       `json:"name"`
	Description       string                       `json:"description"`
	Status            string                       `json:"status"`
	EffectiveDate     time.Time                    `json:"effective_date"`
	EffectiveUntil    *time.Time                   `json:"effective_until"`
	RetiredAt         *time.Time                   `json:"retired_at"`
	BusinessOwner     string                       `json:"business_owner"`
	TechnicalOwner    string                       `json:"technical_owner"`
	Rules             []failModePolicyRuleResponse `json:"rules"`
	Origin            string                       `json:"origin"`
	Managed           bool                         `json:"managed"`
	Replaces          string                       `json:"replaces"`
	SuccessorPolicyID string                       `json:"successor_policy_id"`
	SuccessorVersion  int                          `json:"successor_version"`
	CreatedAt         time.Time                    `json:"created_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
	CreatedBy         string                       `json:"created_by"`
	ApprovedBy        string                       `json:"approved_by"`
	ApprovedAt        *time.Time                   `json:"approved_at"`
}

// failModePolicyListResponse wraps a versions array.
type failModePolicyListResponse struct {
	FailModePolicies []failModePolicyResponse `json:"fail_mode_policies"`
}

func toFailModePolicyRuleResponse(r failmode.FailModePolicyRule) failModePolicyRuleResponse {
	return failModePolicyRuleResponse{
		CorrectnessClass: string(r.CorrectnessClass),
		PermittedMode:    string(r.PermittedMode),
		EnforcementState: string(r.EnforcementState),
		Outcome:          string(r.Outcome),
		Reason:           r.Reason,
	}
}

func toFailModePolicyResponse(p *failmode.FailModePolicy) failModePolicyResponse {
	rules := make([]failModePolicyRuleResponse, 0, len(p.Rules))
	for _, r := range p.Rules {
		rules = append(rules, toFailModePolicyRuleResponse(r))
	}
	return failModePolicyResponse{
		ID:                p.ID,
		Version:           p.Version,
		Name:              p.Name,
		Description:       p.Description,
		Status:            string(p.Status),
		EffectiveDate:     p.EffectiveDate,
		EffectiveUntil:    p.EffectiveUntil,
		RetiredAt:         p.RetiredAt,
		BusinessOwner:     p.BusinessOwner,
		TechnicalOwner:    p.TechnicalOwner,
		Rules:             rules,
		Origin:            p.Origin,
		Managed:           p.Managed,
		Replaces:          p.Replaces,
		SuccessorPolicyID: p.SuccessorPolicyID,
		SuccessorVersion:  p.SuccessorVersion,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
		CreatedBy:         p.CreatedBy,
		ApprovedBy:        p.ApprovedBy,
		ApprovedAt:        p.ApprovedAt,
	}
}

func toFailModePolicyListResponse(items []*failmode.FailModePolicy) failModePolicyListResponse {
	out := make([]failModePolicyResponse, 0, len(items))
	for _, p := range items {
		if p == nil {
			continue
		}
		out = append(out, toFailModePolicyResponse(p))
	}
	return failModePolicyListResponse{FailModePolicies: out}
}
