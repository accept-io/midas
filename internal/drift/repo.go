package drift

import (
	"context"
	"time"
)

// DriftDefinitionRepository persists DriftDefinition revisions. Semantics
// mirror failmode.PolicyRepository / authority.ProfileRepository:
//
//   - Create appends a (id, version) row; the caller assigns Version.
//   - Update replaces the matching (id, version) row in place. Memory
//     posture: silent no-op when the row is missing. Postgres posture
//     (Drift-1b) will return a wrapped error on no-op update.
//   - FindByID returns the latest version (highest Version) regardless
//     of status. Returns (nil, nil) when no rows exist for the id.
//   - FindByIDAndVersion returns (nil, nil) when the (id, version)
//     pair does not exist.
//   - FindActiveAt resolves Status==active AND EffectiveDate <= at AND
//     (EffectiveUntil IS NULL OR EffectiveUntil > at). When multiple
//     rows satisfy the predicate (an invariant violation), the highest
//     Version wins.
//   - ListVersions returns versions in descending Version order; an
//     empty slice (not nil) for an unknown id.
//
// Repositories do not call Validate; callers and tests do.
type DriftDefinitionRepository interface {
	Create(ctx context.Context, d *DriftDefinition) error
	Update(ctx context.Context, d *DriftDefinition) error
	FindByID(ctx context.Context, id string) (*DriftDefinition, error)
	FindByIDAndVersion(ctx context.Context, id string, version int) (*DriftDefinition, error)
	FindActiveAt(ctx context.Context, id string, at time.Time) (*DriftDefinition, error)
	ListVersions(ctx context.Context, id string) ([]*DriftDefinition, error)
}

// DriftSeriesRepository persists DriftSeries headers. Series are
// system-managed; populated by the aggregation worker added in
// Drift-3b. Drift-1a constructs and tests the repository in isolation.
//
//   - FindByDefinitionAndMetric resolves the (definition_id,
//     definition_version, metric_id) tuple to its single matching
//     series. Returns (nil, nil) when no row matches.
//   - ListByContinuityGroup returns every series sharing the same
//     ContinuityGroupID across (typically distinct) DriftDefinition
//     revisions. Used by later UI tranches to stitch the visual
//     timeline.
//   - UpdateStatus mutates only the rolled-up Status; other fields
//     are unchanged.
//   - Seal sets SealedAt; called when the parent DriftDefinition is
//     retired.
type DriftSeriesRepository interface {
	Create(ctx context.Context, s *DriftSeries) error
	FindByID(ctx context.Context, id string) (*DriftSeries, error)
	FindByDefinitionAndMetric(
		ctx context.Context,
		definitionID string,
		definitionVersion int,
		metricID string,
	) (*DriftSeries, error)
	ListByDefinition(ctx context.Context, definitionID string) ([]*DriftSeries, error)
	ListByContinuityGroup(ctx context.Context, groupID string) ([]*DriftSeries, error)
	UpdateStatus(ctx context.Context, seriesID string, status DriftSeriesStatus) error
	Seal(ctx context.Context, seriesID string, at time.Time) error
}

// DriftSeriesPointRepository persists DriftSeriesPoint append-only
// records.
//
//   - ListBySeries paginates by WindowStart ascending, starting at
//     fromWindow (inclusive); limit <= 0 is treated as no limit.
//   - DeleteBefore is declared for later retention pruning. Drift-1a
//     does not exercise it in detection / aggregation paths; the
//     interface is fixed now so Drift-1b's schema partitioning does
//     not require interface churn later.
type DriftSeriesPointRepository interface {
	Create(ctx context.Context, p *DriftSeriesPoint) error
	FindByID(ctx context.Context, id string) (*DriftSeriesPoint, error)
	ListBySeries(
		ctx context.Context,
		seriesID string,
		fromWindow time.Time,
		limit int,
	) ([]*DriftSeriesPoint, error)
	DeleteBefore(ctx context.Context, seriesID string, before time.Time) (int, error)
}

// DriftObservationRepository persists DriftObservation envelopes
// (audit-chain integration is Drift-4; the repository layer simply
// stores the records).
//
//   - UpdateOperatorStatus mutates only OperatorStatus. The detector-
//     side DetectorStatus is immutable on an existing observation.
type DriftObservationRepository interface {
	Create(ctx context.Context, o *DriftObservation) error
	FindByID(ctx context.Context, id string) (*DriftObservation, error)
	ListBySeries(ctx context.Context, seriesID string) ([]*DriftObservation, error)
	ListByDefinition(ctx context.Context, definitionID string) ([]*DriftObservation, error)
	ListByEntity(
		ctx context.Context,
		kind TargetEntityKind,
		entityID string,
	) ([]*DriftObservation, error)
	UpdateOperatorStatus(
		ctx context.Context,
		observationID string,
		status DriftObservationOperatorStatus,
	) error
}

// DriftAnnotationRepository persists DriftAnnotation records.
// Annotations target either a series or an observation; the
// (TargetKind, TargetID) tuple is the natural lookup key.
//
//   - Supersede sets Status=superseded and SupersededByID on the named
//     annotation, recording UpdatedAt at the supplied timestamp.
type DriftAnnotationRepository interface {
	Create(ctx context.Context, a *DriftAnnotation) error
	FindByID(ctx context.Context, id string) (*DriftAnnotation, error)
	ListByTarget(
		ctx context.Context,
		kind DriftAnnotationTargetKind,
		targetID string,
	) ([]*DriftAnnotation, error)
	Supersede(
		ctx context.Context,
		annotationID string,
		supersededByID string,
		at time.Time,
	) error
}
