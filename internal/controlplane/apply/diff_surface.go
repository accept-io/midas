package apply

import (
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/surface"
)

// computeSurfaceDiff compares the persisted active-state Surface (latest)
// with the proposed new version expressed as a SurfaceDocument. It returns
// a PlanDiff describing only stable scalar fields whose values differ after
// normalisation. It returns nil when there are no changes.
//
// The diff is conservative: complex structural fields (required_context,
// consequence_types, mandatory_evidence, tags, taxonomy, stakeholders,
// compliance_frameworks, external_references) are deliberately excluded
// because they require deep structural comparison beyond the scope of this
// release. Fields the planner does not yet normalise reliably are also
// excluded.
//
// The function relies on mapSurfaceDocumentToDecisionSurface to perform the
// same normalisation the executor will apply, so the "after" values reflect
// what would actually be persisted. Mapping errors are treated as an
// inability to produce a trustworthy diff and yield nil.
func computeSurfaceDiff(latest *surface.DecisionSurface, doc types.SurfaceDocument) *PlanDiff {
	if latest == nil {
		return nil
	}
	// Use the same mapper the executor uses, so normalised values match what
	// will be written. now/createdBy/version do not influence the fields we
	// compare below, so placeholder zero values are safe.
	proposed, err := mapSurfaceDocumentToDecisionSurface(doc, latest.UpdatedAt, latest.CreatedBy, latest.Version+1)
	if err != nil {
		return nil
	}

	var fields []FieldDiff
	addScalar := func(name string, before, after any) {
		if before != after {
			fields = append(fields, FieldDiff{Field: name, Before: before, After: after})
		}
	}

	addScalar("spec.name", latest.Name, proposed.Name)
	addScalar("spec.description", latest.Description, proposed.Description)
	addScalar("spec.domain", latest.Domain, proposed.Domain)
	addScalar("spec.category", latest.Category, proposed.Category)
	addScalar("spec.minimum_confidence", latest.MinimumConfidence, proposed.MinimumConfidence)
	addScalar("spec.decision_type", string(latest.DecisionType), string(proposed.DecisionType))
	addScalar("spec.reversibility_class", string(latest.ReversibilityClass), string(proposed.ReversibilityClass))
	addScalar("spec.failure_mode", string(latest.FailureMode), string(proposed.FailureMode))
	addScalar("spec.policy_package", latest.PolicyPackage, proposed.PolicyPackage)
	addScalar("spec.policy_version", latest.PolicyVersion, proposed.PolicyVersion)
	addScalar("spec.business_owner", latest.BusinessOwner, proposed.BusinessOwner)
	addScalar("spec.technical_owner", latest.TechnicalOwner, proposed.TechnicalOwner)
	addScalar("spec.process_id", latest.ProcessID, proposed.ProcessID)

	if len(fields) == 0 {
		return nil
	}
	return &PlanDiff{Fields: fields}
}
