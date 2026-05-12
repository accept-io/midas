package httpapi

// escalation_target_response.go — wire-format DTOs and mappers for the
// D31k-impl-1 EscalationTarget read API. Conventions mirror
// failmode_policy_response.go and drift_response.go:
//
//   - List wrappers always include a non-null array (even empty).
//   - Nullable timestamps use *time.Time (zero in domain → null in JSON).
//   - Domain enums are stringified; the OpenAPI enum definitions are
//     the contract for valid wire values.

import (
	"time"

	"github.com/accept-io/midas/internal/escalation"
)

// escalationTargetResponse is the wire shape for a single
// EscalationTarget revision returned by the GET endpoints. Nullable
// timestamps use *time.Time without omitempty so the JSON output
// emits explicit null (matching the OpenAPI `nullable: true`
// declarations) instead of dropping the key.
type escalationTargetResponse struct {
	ID             string     `json:"id"`
	Version        int        `json:"version"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Kind           string     `json:"kind"`
	Handle         string     `json:"handle"`
	Status         string     `json:"status"`
	EffectiveDate  time.Time  `json:"effective_date"`
	EffectiveUntil *time.Time `json:"effective_until"`
	BusinessOwner  string     `json:"business_owner"`
	TechnicalOwner string     `json:"technical_owner"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CreatedBy      string     `json:"created_by"`
	ApprovedBy     string     `json:"approved_by"`
	ApprovedAt     *time.Time `json:"approved_at"`
}

// escalationTargetListResponse wraps a versions/list array.
type escalationTargetListResponse struct {
	EscalationTargets []escalationTargetResponse `json:"escalation_targets"`
}

func toEscalationTargetResponse(t *escalation.EscalationTarget) escalationTargetResponse {
	return escalationTargetResponse{
		ID:             t.ID,
		Version:        t.Version,
		Name:           t.Name,
		Description:    t.Description,
		Kind:           string(t.Kind),
		Handle:         t.Handle,
		Status:         string(t.Status),
		EffectiveDate:  t.EffectiveDate,
		EffectiveUntil: t.EffectiveUntil,
		BusinessOwner:  t.BusinessOwner,
		TechnicalOwner: t.TechnicalOwner,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		CreatedBy:      t.CreatedBy,
		ApprovedBy:     t.ApprovedBy,
		ApprovedAt:     t.ApprovedAt,
	}
}

func toEscalationTargetListResponse(items []*escalation.EscalationTarget) escalationTargetListResponse {
	out := make([]escalationTargetResponse, 0, len(items))
	for _, t := range items {
		if t == nil {
			continue
		}
		out = append(out, toEscalationTargetResponse(t))
	}
	return escalationTargetListResponse{EscalationTargets: out}
}
