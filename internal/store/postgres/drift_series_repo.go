package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// DriftSeriesRepo is the Postgres implementation of
// drift.DriftSeriesRepository.
type DriftSeriesRepo struct {
	db sqltx.DBTX
}

func NewDriftSeriesRepo(db sqltx.DBTX) (*DriftSeriesRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &DriftSeriesRepo{db: db}, nil
}

const driftSeriesColumns = `
	id,
	definition_id,
	definition_version,
	metric_id,
	cadence,
	status,
	continuity_group_id,
	previous_series_id,
	superseded_by_series_id,
	cutover_at,
	sealed_at,
	created_at,
	updated_at
`

func (r *DriftSeriesRepo) Create(ctx context.Context, s *drift.DriftSeries) error {
	const q = `
		INSERT INTO drift_series (
			id,
			definition_id, definition_version, metric_id,
			cadence, status,
			continuity_group_id, previous_series_id, superseded_by_series_id,
			cutover_at, sealed_at,
			created_at, updated_at
		) VALUES (
			$1,
			$2, $3, $4,
			$5, $6,
			$7, $8, $9,
			$10, $11,
			$12, $13
		)
	`
	_, err := r.db.ExecContext(
		ctx, q,
		s.ID,
		s.DefinitionID, s.DefinitionVersion, s.MetricID,
		string(s.Cadence), string(s.Status),
		s.ContinuityGroupID, nullableString(s.PreviousSeriesID), nullableString(s.SupersededBySeriesID),
		nullableTime(s.CutoverAt), nullableTime(s.SealedAt),
		s.CreatedAt, s.UpdatedAt,
	)
	return err
}

func (r *DriftSeriesRepo) FindByID(ctx context.Context, id string) (*drift.DriftSeries, error) {
	q := `SELECT ` + driftSeriesColumns + ` FROM drift_series WHERE id = $1`
	s, err := scanDriftSeriesRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *DriftSeriesRepo) FindByDefinitionAndMetric(
	ctx context.Context,
	definitionID string,
	definitionVersion int,
	metricID string,
) (*drift.DriftSeries, error) {
	q := `
		SELECT ` + driftSeriesColumns + `
		FROM drift_series
		WHERE definition_id = $1
		  AND definition_version = $2
		  AND metric_id = $3
		LIMIT 1
	`
	s, err := scanDriftSeriesRow(r.db.QueryRowContext(ctx, q, definitionID, definitionVersion, metricID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *DriftSeriesRepo) ListByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftSeries, error) {
	q := `
		SELECT ` + driftSeriesColumns + `
		FROM drift_series
		WHERE definition_id = $1
		ORDER BY definition_version DESC, metric_id ASC
	`
	rows, err := r.db.QueryContext(ctx, q, definitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*drift.DriftSeries{}
	for rows.Next() {
		s, err := scanDriftSeriesRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *DriftSeriesRepo) ListByContinuityGroup(ctx context.Context, groupID string) ([]*drift.DriftSeries, error) {
	q := `
		SELECT ` + driftSeriesColumns + `
		FROM drift_series
		WHERE continuity_group_id = $1
		ORDER BY definition_version ASC, metric_id ASC
	`
	rows, err := r.db.QueryContext(ctx, q, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*drift.DriftSeries{}
	for rows.Next() {
		s, err := scanDriftSeriesRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateStatus mutates only the rolled-up Status. Silent no-op when the
// series ID does not exist (mirrors memory repo posture; the brief
// explicitly calls for no-op).
func (r *DriftSeriesRepo) UpdateStatus(ctx context.Context, seriesID string, status drift.DriftSeriesStatus) error {
	const q = `
		UPDATE drift_series
		SET status = $2,
		    updated_at = COALESCE(updated_at, NOW())
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, q, seriesID, string(status))
	return err
}

// Seal sets SealedAt and updated_at when the series exists. Silent no-op
// otherwise.
func (r *DriftSeriesRepo) Seal(ctx context.Context, seriesID string, at time.Time) error {
	const q = `
		UPDATE drift_series
		SET sealed_at = $2,
		    updated_at = $2
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, q, seriesID, at)
	return err
}

func scanDriftSeriesRow(row driftScanner) (*drift.DriftSeries, error) {
	var (
		s                drift.DriftSeries
		cadence          string
		status           string
		previousSeriesID sql.NullString
		supersededByID   sql.NullString
		cutoverAt        sql.NullTime
		sealedAt         sql.NullTime
	)
	err := row.Scan(
		&s.ID,
		&s.DefinitionID, &s.DefinitionVersion, &s.MetricID,
		&cadence, &status,
		&s.ContinuityGroupID, &previousSeriesID, &supersededByID,
		&cutoverAt, &sealedAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.Cadence = drift.Cadence(cadence)
	s.Status = drift.DriftSeriesStatus(status)
	if previousSeriesID.Valid {
		s.PreviousSeriesID = previousSeriesID.String
	}
	if supersededByID.Valid {
		s.SupersededBySeriesID = supersededByID.String
	}
	if cutoverAt.Valid {
		t := cutoverAt.Time
		s.CutoverAt = &t
	}
	if sealedAt.Valid {
		t := sealedAt.Time
		s.SealedAt = &t
	}
	return &s, nil
}

func scanDriftSeriesRows(rows *sql.Rows) (*drift.DriftSeries, error) {
	return scanDriftSeriesRow(rows)
}

var _ drift.DriftSeriesRepository = (*DriftSeriesRepo)(nil)
