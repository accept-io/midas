package apply

import (
	"context"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/drift"
)

// planDriftDefinitionEntry plans a single DriftDefinition document. The
// Kind is versioned in the same way as Profile, GovernanceExpectation,
// and FailModePolicy: re-applying a document whose logical ID already
// exists creates a NEW version (CreateKindNewVersion, NewVersion =
// existing.Version + 1). First-time applies plan a Create with
// NewVersion = 1.
//
// Metric-immutability defense (Drift-1c): if the document explicitly
// sets lifecycle.version to a value that already exists AND that
// existing revision's metrics differ from the document's metrics, the
// entry is marked Invalid with the canonical message. This is a
// defensive check — the FMP-style happy path always advances to a new
// version, so this branch only fires when an operator has explicitly
// pinned a version. The brief mandates the exact rejection wording.
//
// DriftDefinition has cross-document references (target entity,
// optional GovernanceExpectation reference). Apply-time referential
// resolution is performed in this planner via the helpers in
// drift_ref_check.go using whichever apply-side repos are available;
// when a relevant repo is nil the planner degrades to structural-only
// validation for that target kind, matching the convention used by
// other Kinds.
func (s *Service) planDriftDefinitionEntry(ctx context.Context, doc parser.ParsedDocument, entry *ApplyPlanEntry) {
	dDoc, ok := doc.Doc.(types.DriftDefinitionDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a DriftDefinitionDocument",
		})
		return
	}

	existing, err := s.driftDefinitionRepo.FindByID(ctx, dDoc.Metadata.ID)
	if err != nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "repository error during planning: " + err.Error(),
		})
		return
	}

	// Metric-immutability defense.
	if dDoc.Lifecycle.Version > 0 {
		pinned, err := s.driftDefinitionRepo.FindByIDAndVersion(ctx, dDoc.Metadata.ID, dDoc.Lifecycle.Version)
		if err != nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Message: "repository error during planning: " + err.Error(),
			})
			return
		}
		if pinned != nil {
			if changedID, mutated := metricSetMutation(pinned.Metrics, dDoc.Spec.Metrics); mutated {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourceValidation
				msg := "DriftDefinition metrics are immutable within a revision; create a new version."
				if changedID != "" {
					msg = fmt.Sprintf("%s Changed metric_id: %q.", msg, changedID)
				}
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.metrics",
					Message: msg,
				})
				return
			}
		}
	}

	// Apply-time referential resolution. Errors here mark the entry
	// invalid; otherwise the entry continues as Create / CreateKindNewVersion.
	if refErrs := s.checkDriftReferences(ctx, dDoc); len(refErrs) > 0 {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, refErrs...)
		return
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState

	if existing != nil {
		entry.NewVersion = existing.Version + 1
		entry.CreateKind = CreateKindNewVersion
		entry.Message = fmt.Sprintf(
			"drift definition %q exists at version %d; will create version %d",
			dDoc.Metadata.ID, existing.Version, existing.Version+1,
		)
		if diff := computeDriftDefinitionDiff(existing, dDoc); diff != nil {
			entry.Diff = diff
		}
	} else {
		entry.NewVersion = 1
		entry.CreateKind = CreateKindNew
	}
}

// applyDriftDefinition maps a DriftDefinitionDocument to a domain
// drift.DriftDefinition and persists it via Create. Mirrors
// applyFailModePolicy. Audit emission is deferred to Drift-1e (the
// approval-endpoint tranche) — Drift-1c emits no DriftDefinition
// audit record.
func (s *Service) applyDriftDefinition(
	ctx context.Context,
	repos *RepositorySet,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	version int,
	result *types.ApplyResult,
) error {
	dDoc, ok := doc.Doc.(types.DriftDefinitionDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindDriftDefinition)
	}

	d, err := mapDriftDefinitionDocumentToDomain(dDoc, now, actor, version)
	if err != nil {
		return fmt.Errorf("map drift definition document: %w", err)
	}

	if err := repos.DriftDefinitions.Create(ctx, d); err != nil {
		return fmt.Errorf("create drift definition: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// metricSetMutation reports whether the metric set has been changed
// (added, removed, or any field altered) between two revisions. It
// returns the metric_id of the first detected change so the validation
// error can name it. The comparison is order-insensitive (metrics are
// keyed by metric_id within a revision).
func metricSetMutation(persisted []drift.DriftMetricDefinition, incoming []types.DriftMetricSpec) (string, bool) {
	if len(persisted) != len(incoming) {
		// Structural cardinality mismatch. Find an ID that appears
		// in one set but not the other to surface in the error.
		seen := make(map[string]struct{}, len(persisted))
		for _, m := range persisted {
			seen[m.MetricID] = struct{}{}
		}
		for _, m := range incoming {
			if _, ok := seen[m.MetricID]; !ok {
				return m.MetricID, true
			}
		}
		// Fall back to the first persisted ID.
		if len(persisted) > 0 {
			return persisted[0].MetricID, true
		}
		return "", true
	}

	persistedByID := make(map[string]drift.DriftMetricDefinition, len(persisted))
	for _, m := range persisted {
		persistedByID[m.MetricID] = m
	}
	for _, m := range incoming {
		p, ok := persistedByID[m.MetricID]
		if !ok {
			return m.MetricID, true
		}
		if metricFieldsDiffer(p, m) {
			return m.MetricID, true
		}
	}
	return "", false
}

// metricFieldsDiffer reports whether two metric specs disagree on any
// substantive field. Description is included because the brief treats
// any metric change as a revision-bumping event.
func metricFieldsDiffer(p drift.DriftMetricDefinition, d types.DriftMetricSpec) bool {
	if string(p.DriftType) != d.DriftType {
		return true
	}
	if string(p.BaselineStrategy) != d.BaselineStrategy {
		return true
	}
	if p.BaselineWindowSeconds != d.BaselineWindowSeconds {
		return true
	}
	if p.WindowSeconds != d.WindowSeconds {
		return true
	}
	if string(p.Cadence) != d.Cadence {
		return true
	}
	if p.WarningThreshold != d.WarningThreshold {
		return true
	}
	if p.BreachedThreshold != d.BreachedThreshold {
		return true
	}
	if string(p.ThresholdDirection) != d.ThresholdDirection {
		return true
	}
	if p.GovernanceExpectationRef != d.GovernanceExpectationRef {
		return true
	}
	if p.GovernanceExpectationVer != d.GovernanceExpectationVer {
		return true
	}
	if p.Description != d.Description {
		return true
	}
	return false
}
