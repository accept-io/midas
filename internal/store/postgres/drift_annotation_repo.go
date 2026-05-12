package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// DriftAnnotationRepo is the Postgres implementation of
// drift.DriftAnnotationRepository.
type DriftAnnotationRepo struct {
	db sqltx.DBTX
}

func NewDriftAnnotationRepo(db sqltx.DBTX) (*DriftAnnotationRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &DriftAnnotationRepo{db: db}, nil
}

const driftAnnotationColumns = `
	id,
	target_kind,
	target_id,
	annotation_type,
	body,
	suppression_until,
	reference_envelope_ids,
	status,
	superseded_by_id,
	author_id,
	created_at,
	updated_at
`

func (r *DriftAnnotationRepo) Create(ctx context.Context, a *drift.DriftAnnotation) error {
	referencesJSON, err := marshalJSONStringArray(a.ReferenceEnvelopeIDs)
	if err != nil {
		return err
	}
	const q = `
		INSERT INTO drift_annotations (
			id,
			target_kind, target_id,
			annotation_type, body,
			suppression_until,
			reference_envelope_ids,
			status, superseded_by_id,
			author_id,
			created_at, updated_at
		) VALUES (
			$1,
			$2, $3,
			$4, $5,
			$6,
			$7,
			$8, $9,
			$10,
			$11, $12
		)
	`
	_, err = r.db.ExecContext(
		ctx, q,
		a.ID,
		string(a.TargetKind), a.TargetID,
		string(a.AnnotationType), a.Body,
		nullableTime(a.SuppressionUntil),
		referencesJSON,
		string(a.Status), nullableString(a.SupersededByID),
		a.AuthorID,
		a.CreatedAt, a.UpdatedAt,
	)
	return err
}

func (r *DriftAnnotationRepo) FindByID(ctx context.Context, id string) (*drift.DriftAnnotation, error) {
	q := `SELECT ` + driftAnnotationColumns + ` FROM drift_annotations WHERE id = $1`
	a, err := scanDriftAnnotationRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

func (r *DriftAnnotationRepo) ListByTarget(
	ctx context.Context,
	kind drift.DriftAnnotationTargetKind,
	targetID string,
) ([]*drift.DriftAnnotation, error) {
	q := `
		SELECT ` + driftAnnotationColumns + `
		FROM drift_annotations
		WHERE target_kind = $1 AND target_id = $2
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, q, string(kind), targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*drift.DriftAnnotation{}
	for rows.Next() {
		a, err := scanDriftAnnotationRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Supersede sets status='superseded', superseded_by_id, and updated_at.
// Silent no-op when the annotation ID does not exist.
func (r *DriftAnnotationRepo) Supersede(
	ctx context.Context,
	annotationID string,
	supersededByID string,
	at time.Time,
) error {
	const q = `
		UPDATE drift_annotations
		SET status = 'superseded',
		    superseded_by_id = $2,
		    updated_at = $3
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, q, annotationID, supersededByID, at)
	return err
}

func scanDriftAnnotationRow(row driftScanner) (*drift.DriftAnnotation, error) {
	var (
		a                drift.DriftAnnotation
		targetKind       string
		annotationType   string
		suppressionUntil sql.NullTime
		referencesBytes  []byte
		statusStr        string
		supersededByID   sql.NullString
	)
	err := row.Scan(
		&a.ID,
		&targetKind, &a.TargetID,
		&annotationType, &a.Body,
		&suppressionUntil,
		&referencesBytes,
		&statusStr, &supersededByID,
		&a.AuthorID,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.TargetKind = drift.DriftAnnotationTargetKind(targetKind)
	a.AnnotationType = drift.DriftAnnotationType(annotationType)
	a.Status = drift.DriftAnnotationStatus(statusStr)
	if suppressionUntil.Valid {
		t := suppressionUntil.Time
		a.SuppressionUntil = &t
	}
	if supersededByID.Valid {
		a.SupersededByID = supersededByID.String
	}
	references, err := unmarshalJSONStringArray(referencesBytes)
	if err != nil {
		return nil, err
	}
	a.ReferenceEnvelopeIDs = references
	return &a, nil
}

func scanDriftAnnotationRows(rows *sql.Rows) (*drift.DriftAnnotation, error) {
	return scanDriftAnnotationRow(rows)
}

var _ drift.DriftAnnotationRepository = (*DriftAnnotationRepo)(nil)
