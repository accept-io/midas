package apply

import (
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/failmode"
)

// computeFailModePolicyDiff compares the persisted FailModePolicy (existing)
// with the proposed new version expressed as a FailModePolicyDocument. It
// returns a PlanDiff describing only stable scalar fields whose values
// differ after normalisation, and nil when there are no changes.
//
// The diff is conservative and advisory only — it never blocks apply.
// Slice fields (Rules) are emitted as a single "spec.rules" entry when
// the rule set differs, mirroring how computeProfileDiff handles
// ConsequenceThreshold. Mapping errors yield nil because an unreliable
// diff is worse than none.
func computeFailModePolicyDiff(existing *failmode.FailModePolicy, doc types.FailModePolicyDocument) *PlanDiff {
	if existing == nil {
		return nil
	}
	proposed, err := mapFailModePolicyDocumentToDomain(doc, existing.UpdatedAt, existing.CreatedBy, existing.Version+1)
	if err != nil {
		return nil
	}

	var fields []FieldDiff
	addScalar := func(name string, before, after any) {
		if before != after {
			fields = append(fields, FieldDiff{Field: name, Before: before, After: after})
		}
	}

	addScalar("metadata.name", existing.Name, proposed.Name)
	addScalar("spec.name", existing.Name, proposed.Name)
	addScalar("spec.description", existing.Description, proposed.Description)
	addScalar("spec.business_owner", existing.BusinessOwner, proposed.BusinessOwner)
	addScalar("spec.technical_owner", existing.TechnicalOwner, proposed.TechnicalOwner)
	addScalar("spec.origin", existing.Origin, proposed.Origin)
	addScalar("spec.managed", existing.Managed, proposed.Managed)
	addScalar("spec.replaces", existing.Replaces, proposed.Replaces)

	if !rulesEqual(existing.Rules, proposed.Rules) {
		fields = append(fields, FieldDiff{
			Field:  "spec.rules",
			Before: existing.Rules,
			After:  proposed.Rules,
		})
	}

	if len(fields) == 0 {
		return nil
	}
	return &PlanDiff{Fields: fields}
}

// rulesEqual reports whether two rule slices are equal as sets, keyed by
// CorrectnessClass. Order is not significant because the domain validator
// enforces exhaustiveness without ordering. Reason text is compared
// verbatim — operators see reason changes in the diff output.
func rulesEqual(a, b []failmode.FailModePolicyRule) bool {
	if len(a) != len(b) {
		return false
	}
	byClass := make(map[failmode.CorrectnessClass]failmode.FailModePolicyRule, len(a))
	for _, r := range a {
		byClass[r.CorrectnessClass] = r
	}
	for _, r := range b {
		other, ok := byClass[r.CorrectnessClass]
		if !ok {
			return false
		}
		if other.PermittedMode != r.PermittedMode || other.Reason != r.Reason {
			return false
		}
	}
	return true
}
