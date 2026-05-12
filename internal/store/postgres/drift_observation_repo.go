package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// DriftObservationRepo is the Postgres implementation of
// drift.DriftObservationRepository.
type DriftObservationRepo struct {
	db sqltx.DBTX
}

func NewDriftObservationRepo(db sqltx.DBTX) (*DriftObservationRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &DriftObservationRepo{db: db}, nil
}

const driftObservationColumns = `
	id,
	definition_id,
	definition_version,
	series_id,
	point_id,
	target_entity_kind,
	target_entity_id,
	drift_type,
	magnitude,
	detector_status,
	operator_status,
	baseline_window_id,
	observed_window_start,
	observed_window_end,
	detected_at,
	emitted_at,
	backfilled,
	backfill_run_id,
	evidence_envelope_ids,
	related_fail_mode_policy_id,
	related_governance_exp_ref,
	correction_of,
	created_at,
	updated_at
`

func (r *DriftObservationRepo) Create(ctx context.Context, o *drift.DriftObservation) error {
	evidenceJSON, err := marshalJSONStringArray(o.EvidenceEnvelopeIDs)
	if err != nil {
		return err
	}
	const q = `
		INSERT INTO drift_observations (
			id,
			definition_id, definition_version,
			series_id, point_id,
			target_entity_kind, target_entity_id,
			drift_type,
			magnitude,
			detector_status, operator_status,
			baseline_window_id,
			observed_window_start, observed_window_end,
			detected_at, emitted_at,
			backfilled, backfill_run_id,
			evidence_envelope_ids,
			related_fail_mode_policy_id, related_governance_exp_ref,
			correction_of,
			created_at, updated_at
		) VALUES (
			$1,
			$2, $3,
			$4, $5,
			$6, $7,
			$8,
			$9,
			$10, $11,
			$12,
			$13, $14,
			$15, $16,
			$17, $18,
			$19,
			$20, $21,
			$22,
			$23, $24
		)
	`
	_, err = r.db.ExecContext(
		ctx, q,
		o.ID,
		o.DefinitionID, o.DefinitionVersion,
		o.SeriesID, o.PointID,
		string(o.TargetEntityKind), o.TargetEntityID,
		string(o.DriftType),
		o.Magnitude,
		string(o.DetectorStatus), string(o.OperatorStatus),
		nullableString(o.BaselineWindowID),
		o.ObservedWindowStart, o.ObservedWindowEnd,
		o.DetectedAt, o.EmittedAt,
		o.Backfilled, nullableString(o.BackfillRunID),
		evidenceJSON,
		nullableString(o.RelatedFailModePolicyID), nullableString(o.RelatedGovernanceExpRef),
		nullableString(o.CorrectionOf),
		o.CreatedAt, o.UpdatedAt,
	)
	return err
}

func (r *DriftObservationRepo) FindByID(ctx context.Context, id string) (*drift.DriftObservation, error) {
	q := `SELECT ` + driftObservationColumns + ` FROM drift_observations WHERE id = $1`
	o, err := scanDriftObservationRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return o, nil
}

func (r *DriftObservationRepo) ListBySeries(ctx context.Context, seriesID string) ([]*drift.DriftObservation, error) {
	q := `
		SELECT ` + driftObservationColumns + `
		FROM drift_observations
		WHERE series_id = $1
		ORDER BY observed_window_start ASC, created_at ASC
	`
	return r.queryObservations(ctx, q, seriesID)
}

func (r *DriftObservationRepo) ListByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftObservation, error) {
	q := `
		SELECT ` + driftObservationColumns + `
		FROM drift_observations
		WHERE definition_id = $1
		ORDER BY observed_window_start ASC, created_at ASC
	`
	return r.queryObservations(ctx, q, definitionID)
}

func (r *DriftObservationRepo) ListByEntity(
	ctx context.Context,
	kind drift.TargetEntityKind,
	entityID string,
) ([]*drift.DriftObservation, error) {
	q := `
		SELECT ` + driftObservationColumns + `
		FROM drift_observations
		WHERE target_entity_kind = $1 AND target_entity_id = $2
		ORDER BY observed_window_start ASC, created_at ASC
	`
	return r.queryObservations(ctx, q, string(kind), entityID)
}

// UpdateOperatorStatus mutates only OperatorStatus. The detector-side
// DetectorStatus is left untouched. Silent no-op when the observation
// does not exist (mirrors memory; brief specifies no-op).
func (r *DriftObservationRepo) UpdateOperatorStatus(
	ctx context.Context,
	observationID string,
	status drift.DriftObservationOperatorStatus,
) error {
	const q = `
		UPDATE drift_observations
		SET operator_status = $2,
		    updated_at = COALESCE(updated_at, NOW())
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, q, observationID, string(status))
	return err
}

func (r *DriftObservationRepo) queryObservations(ctx context.Context, q string, args ...any) ([]*drift.DriftObservation, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*drift.DriftObservation{}
	for rows.Next() {
		o, err := scanDriftObservationRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanDriftObservationRow(row driftScanner) (*drift.DriftObservation, error) {
	var (
		o                       drift.DriftObservation
		targetEntityKind        string
		driftType               string
		detectorStatus          string
		operatorStatus          string
		baselineWindowID        sql.NullString
		backfillRunID           sql.NullString
		evidenceBytes           []byte
		relatedFailModePolicyID sql.NullString
		relatedGovernanceExpRef sql.NullString
		correctionOf            sql.NullString
	)
	err := row.Scan(
		&o.ID,
		&o.DefinitionID, &o.DefinitionVersion,
		&o.SeriesID, &o.PointID,
		&targetEntityKind, &o.TargetEntityID,
		&driftType,
		&o.Magnitude,
		&detectorStatus, &operatorStatus,
		&baselineWindowID,
		&o.ObservedWindowStart, &o.ObservedWindowEnd,
		&o.DetectedAt, &o.EmittedAt,
		&o.Backfilled, &backfillRunID,
		&evidenceBytes,
		&relatedFailModePolicyID, &relatedGovernanceExpRef,
		&correctionOf,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	o.TargetEntityKind = drift.TargetEntityKind(targetEntityKind)
	o.DriftType = drift.DriftType(driftType)
	o.DetectorStatus = drift.DriftObservationDetectorStatus(detectorStatus)
	o.OperatorStatus = drift.DriftObservationOperatorStatus(operatorStatus)
	if baselineWindowID.Valid {
		o.BaselineWindowID = baselineWindowID.String
	}
	if backfillRunID.Valid {
		o.BackfillRunID = backfillRunID.String
	}
	if relatedFailModePolicyID.Valid {
		o.RelatedFailModePolicyID = relatedFailModePolicyID.String
	}
	if relatedGovernanceExpRef.Valid {
		o.RelatedGovernanceExpRef = relatedGovernanceExpRef.String
	}
	if correctionOf.Valid {
		o.CorrectionOf = correctionOf.String
	}
	evidence, err := unmarshalJSONStringArray(evidenceBytes)
	if err != nil {
		return nil, err
	}
	o.EvidenceEnvelopeIDs = evidence
	return &o, nil
}

func scanDriftObservationRows(rows *sql.Rows) (*drift.DriftObservation, error) {
	return scanDriftObservationRow(rows)
}

var _ drift.DriftObservationRepository = (*DriftObservationRepo)(nil)
