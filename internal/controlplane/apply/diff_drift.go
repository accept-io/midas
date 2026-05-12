package apply

import (
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/drift"
)

// computeDriftDefinitionDiff compares the persisted DriftDefinition
// (existing) with the proposed new revision expressed as a
// DriftDefinitionDocument. Returns a PlanDiff describing fields whose
// values differ after normalisation, or nil when there are no changes.
//
// Diff scope (Drift-1c, per brief decision):
//   - Parent mutable fields (lifecycle / approval / successor / target /
//     ownership / origin / managed / replaces) are diffed normally as
//     scalar entries.
//   - The metric set is diffed as a whole. Any difference (add, remove,
//     or any field altered) emits a single "spec.metrics" entry that
//     flags the change as requiring a new revision. The diff itself
//     never blocks apply — the planner's metric-immutability defense
//     handles rejection separately.
//
// Mapping errors yield nil because an unreliable diff is worse than
// none.
func computeDriftDefinitionDiff(existing *drift.DriftDefinition, doc types.DriftDefinitionDocument) *PlanDiff {
	if existing == nil {
		return nil
	}
	proposed, err := mapDriftDefinitionDocumentToDomain(doc, existing.UpdatedAt, existing.CreatedBy, existing.Version+1)
	if err != nil {
		return nil
	}

	var fields []FieldDiff
	addScalar := func(name string, before, after any) {
		if before != after {
			fields = append(fields, FieldDiff{Field: name, Before: before, After: after})
		}
	}

	addScalar("spec.name", existing.Name, proposed.Name)
	addScalar("spec.description", existing.Description, proposed.Description)
	addScalar("spec.business_owner", existing.BusinessOwner, proposed.BusinessOwner)
	addScalar("spec.technical_owner", existing.TechnicalOwner, proposed.TechnicalOwner)
	addScalar("spec.target.kind", string(existing.TargetEntityKind), string(proposed.TargetEntityKind))
	addScalar("spec.target.id", existing.TargetEntityID, proposed.TargetEntityID)
	addScalar("spec.origin", string(existing.Origin), string(proposed.Origin))
	addScalar("spec.managed", existing.Managed, proposed.Managed)
	addScalar("spec.replaces", existing.Replaces, proposed.Replaces)

	if !driftMetricsEqual(existing.Metrics, proposed.Metrics) {
		// Metric changes require a new revision; the planner emits
		// the immutability error separately when an operator pins
		// lifecycle.version. The diff entry is informational only.
		fields = append(fields, FieldDiff{
			Field:  "spec.metrics",
			Before: existing.Metrics,
			After:  proposed.Metrics,
		})
	}

	if len(fields) == 0 {
		return nil
	}
	return &PlanDiff{Fields: fields}
}

// driftMetricsEqual reports whether two metric slices are equal as sets,
// keyed by metric_id. Order is not significant — the validator already
// rejects duplicate metric IDs within a revision.
func driftMetricsEqual(a, b []drift.DriftMetricDefinition) bool {
	if len(a) != len(b) {
		return false
	}
	byID := make(map[string]drift.DriftMetricDefinition, len(a))
	for _, m := range a {
		byID[m.MetricID] = m
	}
	for _, m := range b {
		other, ok := byID[m.MetricID]
		if !ok {
			return false
		}
		if !singleDriftMetricEqual(other, m) {
			return false
		}
	}
	return true
}

func singleDriftMetricEqual(x, y drift.DriftMetricDefinition) bool {
	if x.DriftType != y.DriftType {
		return false
	}
	if x.BaselineStrategy != y.BaselineStrategy {
		return false
	}
	if x.BaselineWindowSeconds != y.BaselineWindowSeconds {
		return false
	}
	if x.WindowSeconds != y.WindowSeconds {
		return false
	}
	if x.Cadence != y.Cadence {
		return false
	}
	if x.WarningThreshold != y.WarningThreshold {
		return false
	}
	if x.BreachedThreshold != y.BreachedThreshold {
		return false
	}
	if x.ThresholdDirection != y.ThresholdDirection {
		return false
	}
	if x.GovernanceExpectationRef != y.GovernanceExpectationRef {
		return false
	}
	if x.GovernanceExpectationVer != y.GovernanceExpectationVer {
		return false
	}
	if x.Description != y.Description {
		return false
	}
	return true
}
