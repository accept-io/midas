package apply

import (
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// computeProfileDiff compares the persisted AuthorityProfile (existing) with
// the proposed new version expressed as a ProfileDocument. It returns a
// PlanDiff describing only stable scalar fields whose values differ after
// normalisation, and nil when there are no changes.
//
// The diff is conservative: slice and map fields (required_context_keys and
// other complex structures) are excluded because they require ordered deep
// comparison beyond the scope of this release. The Consequence struct is
// comparable as a value and is emitted as a single field when different so
// callers see the full structured change.
//
// mapProfileDocumentToAuthorityProfile is reused so the "after" values
// reflect the normalisation the executor will apply. Mapping errors yield
// nil because an unreliable diff is worse than none.
func computeProfileDiff(existing *authority.AuthorityProfile, doc types.ProfileDocument) *PlanDiff {
	if existing == nil {
		return nil
	}
	proposed, err := mapProfileDocumentToAuthorityProfile(doc, existing.UpdatedAt, existing.CreatedBy, existing.Version+1)
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
	addScalar("spec.surface_id", existing.SurfaceID, proposed.SurfaceID)
	addScalar("spec.authority.decision_confidence_threshold", existing.ConfidenceThreshold, proposed.ConfidenceThreshold)
	addScalar("spec.policy.reference", existing.PolicyReference, proposed.PolicyReference)
	addScalar("spec.policy.fail_mode", string(existing.FailMode), string(proposed.FailMode))

	if existing.ConsequenceThreshold != proposed.ConsequenceThreshold {
		fields = append(fields, FieldDiff{
			Field:  "spec.authority.consequence_threshold",
			Before: existing.ConsequenceThreshold,
			After:  proposed.ConsequenceThreshold,
		})
	}

	if len(fields) == 0 {
		return nil
	}
	return &PlanDiff{Fields: fields}
}
