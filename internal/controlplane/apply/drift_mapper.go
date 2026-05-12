package apply

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/drift"
)

// mapDriftDefinitionDocumentToDomain converts a validated DriftDefinition
// control-plane document into a domain drift.DriftDefinition ready for
// persistence. Mirrors the patterns established by the FailModePolicy
// and GovernanceExpectation mappers.
//
// Key behaviours:
//
//   - Status is ALWAYS set to drift.DriftDefinitionStatusReview, regardless
//     of any value the document carries in lifecycle.status. The
//     document's status is accepted-but-forced; the validator has already
//     rejected typo'd values, so the field here is informational only.
//     Promotion to active is the responsibility of a future approval
//     endpoint (Drift-1e).
//
//   - Version is supplied by the planner (1 for first-time creates,
//     existing.Version+1 for re-applies). The document's lifecycle.version
//     is informational only.
//
//   - EffectiveDate falls back to now.UTC() when lifecycle.effective_from
//     is empty (matches FailModePolicy / GovernanceExpectation mappers).
//
//   - Origin defaults to "manual"; Managed defaults to true via the
//     pointer-bool convention on Spec.Managed (matches FailModePolicy).
//
//   - drift.Validate is called defensively after construction. The
//     control-plane validator runs earlier in the apply pipeline; this
//     second call guards against a bypass and surfaces a clear mapper-
//     side error if invariants drift.
//
//   - All string fields are TrimSpace'd. CreatedAt/UpdatedAt are now.UTC().
//     Approval audit fields (ApprovedBy, ApprovedAt) are zero — they are
//     populated only by the future approval flow (Drift-1e).
func mapDriftDefinitionDocumentToDomain(
	doc types.DriftDefinitionDocument,
	now time.Time,
	createdBy string,
	version int,
) (*drift.DriftDefinition, error) {
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

	origin := drift.DriftOrigin(strings.TrimSpace(doc.Spec.Origin))
	if origin == "" {
		origin = drift.DriftOriginManual
	}

	managed := true
	if doc.Spec.Managed != nil {
		managed = *doc.Spec.Managed
	}

	metrics := make([]drift.DriftMetricDefinition, 0, len(doc.Spec.Metrics))
	for _, m := range doc.Spec.Metrics {
		metrics = append(metrics, drift.DriftMetricDefinition{
			MetricID:                 strings.TrimSpace(m.MetricID),
			DriftType:                drift.DriftType(strings.TrimSpace(m.DriftType)),
			BaselineStrategy:         drift.BaselineStrategy(strings.TrimSpace(m.BaselineStrategy)),
			BaselineWindowSeconds:    m.BaselineWindowSeconds,
			WindowSeconds:            m.WindowSeconds,
			Cadence:                  drift.Cadence(strings.TrimSpace(m.Cadence)),
			WarningThreshold:         m.WarningThreshold,
			BreachedThreshold:        m.BreachedThreshold,
			ThresholdDirection:       drift.ThresholdDirection(strings.TrimSpace(m.ThresholdDirection)),
			GovernanceExpectationRef: strings.TrimSpace(m.GovernanceExpectationRef),
			GovernanceExpectationVer: m.GovernanceExpectationVer,
			Description:              strings.TrimSpace(m.Description),
		})
	}

	d := &drift.DriftDefinition{
		ID:               strings.TrimSpace(doc.Metadata.ID),
		Version:          version,
		Name:             strings.TrimSpace(doc.Spec.Name),
		Description:      strings.TrimSpace(doc.Spec.Description),
		Status:           drift.DriftDefinitionStatusReview,
		EffectiveDate:    effectiveFrom,
		EffectiveUntil:   effectiveUntil,
		BusinessOwner:    strings.TrimSpace(doc.Spec.BusinessOwner),
		TechnicalOwner:   strings.TrimSpace(doc.Spec.TechnicalOwner),
		TargetEntityKind: drift.TargetEntityKind(strings.TrimSpace(doc.Spec.Target.Kind)),
		TargetEntityID:   strings.TrimSpace(doc.Spec.Target.ID),
		Metrics:          metrics,
		Origin:           origin,
		Managed:          managed,
		Replaces:         strings.TrimSpace(doc.Spec.Replaces),
		CreatedAt:        now,
		UpdatedAt:        now,
		CreatedBy:        strings.TrimSpace(createdBy),
	}

	if errs := drift.Validate(d); len(errs) > 0 {
		return nil, fmt.Errorf("drift definition domain validation failed: %w", errors.Join(errs...))
	}

	return d, nil
}
