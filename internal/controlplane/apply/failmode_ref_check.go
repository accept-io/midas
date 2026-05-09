package apply

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/failmode"
)

// surfaceEffectiveAt returns the effective-from time for a SurfaceDocument.
// When the document declares a non-zero effective_from, that value is
// returned. Otherwise, the mapper falls back to "now" — but at planning
// time the mapper has not yet run, so we return zero (no effective date).
// The ref-check helper falls back to FindByID + active-status check when
// the effective time is zero, matching the resolver's apply-time intent.
func surfaceEffectiveAt(doc types.SurfaceDocument) time.Time {
	if !doc.Spec.EffectiveFrom.IsZero() {
		return doc.Spec.EffectiveFrom.UTC()
	}
	return time.Time{}
}

// checkFailModePolicyReference validates that an optional FailModePolicy
// reference is well-formed and resolves to an active policy at the given
// effective time (D27j-impl-2). Mirrors checkProcessExists in shape.
//
// Behaviour:
//   - empty policyID → no-op success (no reference is valid).
//   - non-empty policyID with no failModePolicyRepo configured → mark
//     entry invalid; the apply pipeline cannot validate the reference.
//   - non-empty policyID and effectiveAt is zero (caller couldn't determine
//     the artefact's effective_from) → fall back to FindByID + active-status
//     check. Reported in the final report; happens for entities that have
//     no effective_from at the document layer (BusinessService is the only
//     such caller in -2).
//   - non-empty policyID and effectiveAt is set → FindActiveAt(policyID,
//     effectiveAt). If nil (no active version covering effectiveAt) →
//     mark invalid. The fail_mode_policies row may exist as review,
//     deprecated, or retired but FindActiveAt only returns rows with
//     status='active' AND effective_date <= effectiveAt AND
//     (effective_until IS NULL OR effective_until > effectiveAt).
//   - any repository error → mark invalid with the wrapped error.
//
// Returns true when validation passes (including the empty-ID no-op).
// Returns false and marks the entry invalid otherwise.
func (s *Service) checkFailModePolicyReference(
	ctx context.Context,
	doc parser.ParsedDocument,
	policyID string,
	effectiveAt time.Time,
	field string,
	entry *ApplyPlanEntry,
) bool {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return true
	}

	if s.failModePolicyRepo == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   field,
			Message: "fail_mode_policy_id validation unavailable: FailModePolicyRepository not configured",
		})
		return false
	}

	if effectiveAt.IsZero() {
		// Fallback path for callers without an effective date (today only
		// BusinessService). Use FindByID to load the latest version, then
		// require Status = active. This is bounded and matches the
		// resolver's intent: "active at apply time".
		latest, err := s.failModePolicyRepo.FindByID(ctx, policyID)
		if err != nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   field,
				Message: "repository error checking fail_mode_policy reference: " + err.Error(),
			})
			return false
		}
		if latest == nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   field,
				Message: fmt.Sprintf("fail_mode_policy_id %q does not exist", policyID),
			})
			return false
		}
		if latest.Status != failmode.FailModePolicyStatusActive {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   field,
				Message: fmt.Sprintf("fail_mode_policy_id %q is not active (current status: %s)", policyID, latest.Status),
			})
			return false
		}
		return true
	}

	// Default path: bounded active-version resolution at the artefact's
	// effective_from. Catches both "policy not yet effective" and "policy
	// already retired" relative to the referencing artefact's window.
	policy, err := s.failModePolicyRepo.FindActiveAt(ctx, policyID, effectiveAt)
	if err != nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   field,
			Message: "repository error checking fail_mode_policy reference: " + err.Error(),
		})
		return false
	}
	if policy == nil {
		// Could be: not exists, not active, or active outside the window.
		// We disambiguate via a follow-up FindByID lookup so the operator-
		// facing error names the actual cause.
		latest, fbErr := s.failModePolicyRepo.FindByID(ctx, policyID)
		if fbErr != nil {
			// Defensive: surface the first error.
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   field,
				Message: "repository error checking fail_mode_policy reference: " + fbErr.Error(),
			})
			return false
		}
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		switch {
		case latest == nil:
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   field,
				Message: fmt.Sprintf("fail_mode_policy_id %q does not exist", policyID),
			})
		case latest.Status != failmode.FailModePolicyStatusActive:
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   field,
				Message: fmt.Sprintf("fail_mode_policy_id %q is not active (current status: %s)", policyID, latest.Status),
			})
		default:
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   field,
				Message: fmt.Sprintf("fail_mode_policy_id %q is not effective at the referencing artefact's effective_from", policyID),
			})
		}
		return false
	}
	return true
}
