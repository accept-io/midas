package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// DriftSeriesPointRepo is the Postgres implementation of
// drift.DriftSeriesPointRepository. JSONB columns: summary_stats,
// baseline_stats (objects), provenance_envelope_ids (array). Empty
// containers round-trip as '{}' / '[]' (schema defaults), and unmarshal
// to non-nil empty Go containers.
type DriftSeriesPointRepo struct {
	db sqltx.DBTX
}

func NewDriftSeriesPointRepo(db sqltx.DBTX) (*DriftSeriesPointRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &DriftSeriesPointRepo{db: db}, nil
}

const driftSeriesPointColumns = `
	id,
	series_id,
	window_start,
	window_end,
	sample_count,
	summary_stats,
	baseline_stats,
	baseline_window_id,
	magnitude,
	status,
	computation_mode,
	computed_at,
	backfill_run_id,
	source_window_complete,
	provenance_envelope_ids,
	created_at
`

func (r *DriftSeriesPointRepo) Create(ctx context.Context, p *drift.DriftSeriesPoint) error {
	summaryJSON, err := marshalJSONObject(p.SummaryStats)
	if err != nil {
		return err
	}
	baselineJSON, err := marshalJSONObject(p.BaselineStats)
	if err != nil {
		return err
	}
	provenanceJSON, err := marshalJSONStringArray(p.ProvenanceEnvelopeIDs)
	if err != nil {
		return err
	}

	const q = `
		INSERT INTO drift_series_points (
			id, series_id,
			window_start, window_end,
			sample_count,
			summary_stats, baseline_stats,
			baseline_window_id,
			magnitude, status,
			computation_mode, computed_at, backfill_run_id, source_window_complete,
			provenance_envelope_ids,
			created_at
		) VALUES (
			$1, $2,
			$3, $4,
			$5,
			$6, $7,
			$8,
			$9, $10,
			$11, $12, $13, $14,
			$15,
			$16
		)
	`
	_, err = r.db.ExecContext(
		ctx, q,
		p.ID, p.SeriesID,
		p.WindowStart, p.WindowEnd,
		p.SampleCount,
		summaryJSON, baselineJSON,
		nullableString(p.BaselineWindowID),
		p.Magnitude, string(p.Status),
		string(p.ComputationMode), p.ComputedAt, nullableString(p.BackfillRunID), p.SourceWindowComplete,
		provenanceJSON,
		p.CreatedAt,
	)
	return err
}

func (r *DriftSeriesPointRepo) FindByID(ctx context.Context, id string) (*drift.DriftSeriesPoint, error) {
	q := `SELECT ` + driftSeriesPointColumns + ` FROM drift_series_points WHERE id = $1`
	p, err := scanDriftSeriesPointRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// ListBySeries returns the series's points whose WindowStart >=
// fromWindow, ascending by WindowStart. limit <= 0 returns all matching
// rows.
func (r *DriftSeriesPointRepo) ListBySeries(
	ctx context.Context,
	seriesID string,
	fromWindow time.Time,
	limit int,
) ([]*drift.DriftSeriesPoint, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if limit > 0 {
		const q = `
			SELECT ` + driftSeriesPointColumns + `
			FROM drift_series_points
			WHERE series_id = $1 AND window_start >= $2
			ORDER BY window_start ASC
			LIMIT $3
		`
		rows, err = r.db.QueryContext(ctx, q, seriesID, fromWindow, limit)
	} else {
		const q = `
			SELECT ` + driftSeriesPointColumns + `
			FROM drift_series_points
			WHERE series_id = $1 AND window_start >= $2
			ORDER BY window_start ASC
		`
		rows, err = r.db.QueryContext(ctx, q, seriesID, fromWindow)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*drift.DriftSeriesPoint{}
	for rows.Next() {
		p, err := scanDriftSeriesPointRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteBefore removes points whose WindowEnd is strictly before the
// supplied threshold. Returns the count deleted.
func (r *DriftSeriesPointRepo) DeleteBefore(ctx context.Context, seriesID string, before time.Time) (int, error) {
	const q = `DELETE FROM drift_series_points WHERE series_id = $1 AND window_end < $2`
	res, err := r.db.ExecContext(ctx, q, seriesID, before)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func scanDriftSeriesPointRow(row driftScanner) (*drift.DriftSeriesPoint, error) {
	var (
		p                drift.DriftSeriesPoint
		summaryBytes     []byte
		baselineBytes    []byte
		provenanceBytes  []byte
		baselineWindowID sql.NullString
		statusStr        string
		computationMode  string
		backfillRunID    sql.NullString
	)
	err := row.Scan(
		&p.ID, &p.SeriesID,
		&p.WindowStart, &p.WindowEnd,
		&p.SampleCount,
		&summaryBytes, &baselineBytes,
		&baselineWindowID,
		&p.Magnitude, &statusStr,
		&computationMode, &p.ComputedAt, &backfillRunID, &p.SourceWindowComplete,
		&provenanceBytes,
		&p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Status = drift.DriftSeriesPointStatus(statusStr)
	p.ComputationMode = drift.DriftPointComputationMode(computationMode)
	if baselineWindowID.Valid {
		p.BaselineWindowID = baselineWindowID.String
	}
	if backfillRunID.Valid {
		p.BackfillRunID = backfillRunID.String
	}
	summary, err := unmarshalJSONObject(summaryBytes)
	if err != nil {
		return nil, err
	}
	p.SummaryStats = summary
	baseline, err := unmarshalJSONObject(baselineBytes)
	if err != nil {
		return nil, err
	}
	p.BaselineStats = baseline
	provenance, err := unmarshalJSONStringArray(provenanceBytes)
	if err != nil {
		return nil, err
	}
	p.ProvenanceEnvelopeIDs = provenance
	return &p, nil
}

func scanDriftSeriesPointRows(rows *sql.Rows) (*drift.DriftSeriesPoint, error) {
	return scanDriftSeriesPointRow(rows)
}

// marshalJSONObject marshals a map to JSONB-compatible bytes. nil maps
// marshal to '{}' so the schema's NOT NULL DEFAULT '{}' is satisfied.
func marshalJSONObject(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// unmarshalJSONObject unmarshals JSONB bytes into a map. Empty/null
// payloads return a non-nil empty map, mirroring the schema default.
func unmarshalJSONObject(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// marshalJSONStringArray marshals a string slice to a JSONB array. nil
// slices marshal to '[]'.
func marshalJSONStringArray(s []string) ([]byte, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(s)
}

// unmarshalJSONStringArray unmarshals a JSONB array of strings. Empty
// payloads return a non-nil empty slice.
func unmarshalJSONStringArray(b []byte) ([]string, error) {
	if len(b) == 0 {
		return []string{}, nil
	}
	out := []string{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

var _ drift.DriftSeriesPointRepository = (*DriftSeriesPointRepo)(nil)
