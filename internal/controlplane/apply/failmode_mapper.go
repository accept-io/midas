package apply

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/failmode"
)

// mapFailModePolicyDocumentToDomain converts a validated FailModePolicy
// control-plane document into a domain failmode.FailModePolicy ready for
// persistence. Mirrors the patterns established by the Profile and
// GovernanceExpectation mappers (governanceexpectation_mapper.go).
//
// Key behaviours:
//
//   - Status is ALWAYS set to FailModePolicyStatusReview, regardless of any
//     value in doc.Lifecycle.Status. The document's status is
//     accepted-but-forced; the validator has already rejected typo'd values,
//     so the field here is informational only. Promotion to active is the
//     responsibility of a future approval-endpoint tranche.
//
//   - Version is supplied by the planner. 1 for first-time creates,
//     latest+1 for re-applies. The document's Lifecycle.Version is
//     informational only and does not feed into the persisted version.
//
//   - EffectiveDate falls back to now.UTC() when lifecycle.effective_from
//     is empty (matches Profile and GovernanceExpectation mappers).
//
//   - Origin defaults to "manual"; Managed defaults to true. The
//     pointer-bool convention on Spec.Managed lets callers distinguish
//     "unset" from "explicit false".
//
//   - failmode.Validate is called defensively after construction. The
//     control-plane validator runs earlier in the apply pipeline; this
//     second call guards against a bypass and surfaces a clear mapper-
//     side error if invariants drift.
//
//   - All string fields are TrimSpace'd. CreatedAt/UpdatedAt are now.UTC().
//     Approval audit fields (ApprovedBy, ApprovedAt) are zero — they are
//     populated only by the future approval flow.
func mapFailModePolicyDocumentToDomain(
	doc types.FailModePolicyDocument,
	now time.Time,
	createdBy string,
	version int,
) (*failmode.FailModePolicy, error) {
	now = now.UTC()

	effectiveFrom := now
	if s := strings.TrimSpace(doc.Lifecycle.EffectiveFrom); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("invalid lifecycle.effective_from: %w", err)
		}
		effectiveFrom = parsed.UTC()
	}

	var effectiveUntil *time.Time
	if s := strings.TrimSpace(doc.Lifecycle.EffectiveUntil); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("invalid lifecycle.effective_until: %w", err)
		}
		utc := parsed.UTC()
		effectiveUntil = &utc
	}

	origin := strings.TrimSpace(doc.Spec.Origin)
	if origin == "" {
		origin = "manual"
	}

	managed := true
	if doc.Spec.Managed != nil {
		managed = *doc.Spec.Managed
	}

	rules := make([]failmode.FailModePolicyRule, 0, len(doc.Spec.Rules))
	for _, r := range doc.Spec.Rules {
		rules = append(rules, failmode.FailModePolicyRule{
			CorrectnessClass: failmode.CorrectnessClass(strings.TrimSpace(r.CorrectnessClass)),
			PermittedMode:    failmode.PermittedMode(strings.TrimSpace(r.PermittedMode)),
			Reason:           strings.TrimSpace(r.Reason),
		})
	}

	policy := &failmode.FailModePolicy{
		ID:             strings.TrimSpace(doc.Metadata.ID),
		Version:        version,
		Name:           strings.TrimSpace(doc.Spec.Name),
		Description:    strings.TrimSpace(doc.Spec.Description),
		Status:         failmode.FailModePolicyStatusReview,
		EffectiveDate:  effectiveFrom,
		EffectiveUntil: effectiveUntil,
		BusinessOwner:  strings.TrimSpace(doc.Spec.BusinessOwner),
		TechnicalOwner: strings.TrimSpace(doc.Spec.TechnicalOwner),
		Rules:          rules,
		Origin:         origin,
		Managed:        managed,
		Replaces:       strings.TrimSpace(doc.Spec.Replaces),
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      strings.TrimSpace(createdBy),
	}

	if errs := failmode.Validate(policy); len(errs) > 0 {
		return nil, fmt.Errorf("fail-mode policy domain validation failed: %w", errors.Join(errs...))
	}

	return policy, nil
}
