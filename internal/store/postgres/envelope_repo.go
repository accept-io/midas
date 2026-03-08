package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
)

type EnvelopeRepo struct {
	db *sql.DB
}

func NewEnvelopeRepo(db *sql.DB) (*EnvelopeRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	return &EnvelopeRepo{db: db}, nil
}

func (r *EnvelopeRepo) GetByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	const q = `
		SELECT
			id,
			request_id,
			surface_id,
			surface_version,
			profile_id,
			profile_version,
			agent_id,
			state,
			outcome,
			reason_code,
			created_at,
			updated_at,
			closed_at
		FROM operational_envelopes
		WHERE id = $1
	`

	e, err := scanEnvelopeRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return e, nil
}

func (r *EnvelopeRepo) GetByRequestID(ctx context.Context, requestID string) (*envelope.Envelope, error) {
	const q = `
		SELECT
			id,
			request_id,
			surface_id,
			surface_version,
			profile_id,
			profile_version,
			agent_id,
			state,
			outcome,
			reason_code,
			created_at,
			updated_at,
			closed_at
		FROM operational_envelopes
		WHERE request_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`

	e, err := scanEnvelopeRow(r.db.QueryRowContext(ctx, q, requestID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return e, nil
}

func (r *EnvelopeRepo) List(ctx context.Context) ([]*envelope.Envelope, error) {
	const q = `
		SELECT
			id,
			request_id,
			surface_id,
			surface_version,
			profile_id,
			profile_version,
			agent_id,
			state,
			outcome,
			reason_code,
			created_at,
			updated_at,
			closed_at
		FROM operational_envelopes
		ORDER BY created_at DESC, id DESC
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*envelope.Envelope

	for rows.Next() {
		e, err := scanEnvelopeRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *EnvelopeRepo) Create(ctx context.Context, e *envelope.Envelope) error {
	const q = `
		INSERT INTO operational_envelopes (
			id,
			request_id,
			surface_id,
			surface_version,
			profile_id,
			profile_version,
			agent_id,
			state,
			outcome,
			reason_code,
			created_at,
			updated_at,
			closed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		e.ID,
		e.RequestID,
		nullableString(e.Evidence.SurfaceID),
		nullableInt(e.Evidence.SurfaceVersion),
		nullableString(e.Evidence.ProfileID),
		nullableInt(e.Evidence.ProfileVersion),
		nullableString(e.Evidence.AgentID),
		e.State,
		nullableOutcome(e.Outcome),
		nullableReasonCode(e.ReasonCode),
		e.CreatedAt,
		e.UpdatedAt,
		nullableTime(e.ClosedAt),
	)
	return err
}

func (r *EnvelopeRepo) Update(ctx context.Context, e *envelope.Envelope) error {
	const q = `
		UPDATE operational_envelopes
		SET
			request_id = $2,
			surface_id = $3,
			surface_version = $4,
			profile_id = $5,
			profile_version = $6,
			agent_id = $7,
			state = $8,
			outcome = $9,
			reason_code = $10,
			updated_at = $11,
			closed_at = $12
		WHERE id = $1
	`

	res, err := r.db.ExecContext(
		ctx,
		q,
		e.ID,
		e.RequestID,
		nullableString(e.Evidence.SurfaceID),
		nullableInt(e.Evidence.SurfaceVersion),
		nullableString(e.Evidence.ProfileID),
		nullableInt(e.Evidence.ProfileVersion),
		nullableString(e.Evidence.AgentID),
		e.State,
		nullableOutcome(e.Outcome),
		nullableReasonCode(e.ReasonCode),
		e.UpdatedAt,
		nullableTime(e.ClosedAt),
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("envelope not found: id=%s", e.ID)
	}

	return nil
}

type envelopeScanner interface {
	Scan(dest ...any) error
}

func scanEnvelopeRow(row envelopeScanner) (*envelope.Envelope, error) {
	var (
		e              envelope.Envelope
		surfaceID      sql.NullString
		surfaceVersion sql.NullInt64
		profileID      sql.NullString
		profileVersion sql.NullInt64
		agentID        sql.NullString
		outcome        sql.NullString
		reasonCode     sql.NullString
		closedAt       sql.NullTime
	)

	err := row.Scan(
		&e.ID,
		&e.RequestID,
		&surfaceID,
		&surfaceVersion,
		&profileID,
		&profileVersion,
		&agentID,
		&e.State,
		&outcome,
		&reasonCode,
		&e.CreatedAt,
		&e.UpdatedAt,
		&closedAt,
	)
	if err != nil {
		return nil, err
	}

	if surfaceID.Valid {
		e.Evidence.SurfaceID = surfaceID.String
	}
	if surfaceVersion.Valid {
		e.Evidence.SurfaceVersion = int(surfaceVersion.Int64)
	}
	if profileID.Valid {
		e.Evidence.ProfileID = profileID.String
	}
	if profileVersion.Valid {
		e.Evidence.ProfileVersion = int(profileVersion.Int64)
	}
	if agentID.Valid {
		e.Evidence.AgentID = agentID.String
	}
	if outcome.Valid {
		e.Outcome = eval.Outcome(outcome.String)
	}
	if reasonCode.Valid {
		e.ReasonCode = eval.ReasonCode(reasonCode.String)
	}
	if closedAt.Valid {
		t := closedAt.Time
		e.ClosedAt = &t
	}

	return &e, nil
}

func nullableInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableOutcome(o eval.Outcome) any {
	if o == "" {
		return nil
	}
	return o
}

func nullableReasonCode(r eval.ReasonCode) any {
	if r == "" {
		return nil
	}
	return r
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

var _ envelope.EnvelopeRepository = (*EnvelopeRepo)(nil)
