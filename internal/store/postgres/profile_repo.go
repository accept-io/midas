package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/store/sqltx"
	"github.com/accept-io/midas/internal/value"
)

type ProfileRepo struct {
	db sqltx.DBTX
}

func NewProfileRepo(db sqltx.DBTX) (*ProfileRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	return &ProfileRepo{db: db}, nil
}

func (r *ProfileRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
	const q = `
		SELECT
			id,
			surface_id,
			name,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			version,
			effective_date,
			created_at,
			updated_at
		FROM authority_profiles
		WHERE id = $1
		ORDER BY version DESC
		LIMIT 1
	`

	p, err := scanProfileRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return p, nil
}

func (r *ProfileRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*authority.AuthorityProfile, error) {
	const q = `
		SELECT
			id,
			surface_id,
			name,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			version,
			effective_date,
			created_at,
			updated_at
		FROM authority_profiles
		WHERE id = $1
		  AND effective_date <= $2
		ORDER BY effective_date DESC, version DESC
		LIMIT 1
	`

	p, err := scanProfileRow(r.db.QueryRowContext(ctx, q, id, at))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return p, nil
}

func (r *ProfileRepo) ListBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	const q = `
		SELECT
			id,
			surface_id,
			name,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			version,
			effective_date,
			created_at,
			updated_at
		FROM authority_profiles
		WHERE surface_id = $1
		ORDER BY id, version DESC
	`

	rows, err := r.db.QueryContext(ctx, q, surfaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*authority.AuthorityProfile

	for rows.Next() {
		p, err := scanProfileRows(rows)
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

func (r *ProfileRepo) Create(ctx context.Context, p *authority.AuthorityProfile) error {
	requiredContextKeys, err := json.Marshal(p.RequiredContextKeys)
	if err != nil {
		return err
	}

	const q = `
		INSERT INTO authority_profiles (
			id,
			surface_id,
			name,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			version,
			effective_date,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)
	`

	_, err = r.db.ExecContext(
		ctx,
		q,
		p.ID,
		p.SurfaceID,
		p.Name,
		p.ConfidenceThreshold,
		p.ConsequenceThreshold.Type,
		nullableAmount(p.ConsequenceThreshold),
		nullableCurrency(p.ConsequenceThreshold),
		nullableRiskRating(p.ConsequenceThreshold),
		nullableString(p.PolicyReference),
		p.EscalationMode,
		p.FailMode,
		requiredContextKeys,
		p.Version,
		p.EffectiveDate,
		p.CreatedAt,
		p.UpdatedAt,
	)
	return err
}

func (r *ProfileRepo) Update(ctx context.Context, p *authority.AuthorityProfile) error {
	requiredContextKeys, err := json.Marshal(p.RequiredContextKeys)
	if err != nil {
		return err
	}

	const q = `
		UPDATE authority_profiles
		SET
			surface_id = $3,
			name = $4,
			confidence_threshold = $5,
			consequence_type = $6,
			consequence_amount = $7,
			consequence_currency = $8,
			consequence_risk_rating = $9,
			policy_reference = $10,
			escalation_mode = $11,
			fail_mode = $12,
			required_context_keys = $13,
			effective_date = $14,
			updated_at = $15
		WHERE id = $1
		  AND version = $2
	`

	res, err := r.db.ExecContext(
		ctx,
		q,
		p.ID,
		p.Version,
		p.SurfaceID,
		p.Name,
		p.ConfidenceThreshold,
		p.ConsequenceThreshold.Type,
		nullableAmount(p.ConsequenceThreshold),
		nullableCurrency(p.ConsequenceThreshold),
		nullableRiskRating(p.ConsequenceThreshold),
		nullableString(p.PolicyReference),
		p.EscalationMode,
		p.FailMode,
		requiredContextKeys,
		p.EffectiveDate,
		p.UpdatedAt,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("profile not found: id=%s version=%d", p.ID, p.Version)
	}

	return nil
}

type profileScanner interface {
	Scan(dest ...any) error
}

func scanProfileRow(row profileScanner) (*authority.AuthorityProfile, error) {
	var (
		p                    authority.AuthorityProfile
		consequenceType      value.ConsequenceType
		consequenceAmount    sql.NullFloat64
		consequenceCurrency  sql.NullString
		consequenceRisk      sql.NullString
		policyReference      sql.NullString
		requiredContextBytes []byte
	)

	err := row.Scan(
		&p.ID,
		&p.SurfaceID,
		&p.Name,
		&p.ConfidenceThreshold,
		&consequenceType,
		&consequenceAmount,
		&consequenceCurrency,
		&consequenceRisk,
		&policyReference,
		&p.EscalationMode,
		&p.FailMode,
		&requiredContextBytes,
		&p.Version,
		&p.EffectiveDate,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	p.ConsequenceThreshold = authority.Consequence{
		Type: consequenceType,
	}

	if consequenceAmount.Valid {
		p.ConsequenceThreshold.Amount = consequenceAmount.Float64
	}
	if consequenceCurrency.Valid {
		p.ConsequenceThreshold.Currency = consequenceCurrency.String
	}
	if consequenceRisk.Valid {
		p.ConsequenceThreshold.RiskRating = value.RiskRating(consequenceRisk.String)
	}
	if policyReference.Valid {
		p.PolicyReference = policyReference.String
	}

	if len(requiredContextBytes) > 0 {
		if err := json.Unmarshal(requiredContextBytes, &p.RequiredContextKeys); err != nil {
			return nil, err
		}
	} else {
		p.RequiredContextKeys = []string{}
	}

	return &p, nil
}

func scanProfileRows(rows *sql.Rows) (*authority.AuthorityProfile, error) {
	return scanProfileRow(rows)
}

func nullableAmount(c authority.Consequence) any {
	if c.Type == value.ConsequenceTypeMonetary {
		return c.Amount
	}
	return nil
}

func nullableCurrency(c authority.Consequence) any {
	if c.Type == value.ConsequenceTypeMonetary {
		return c.Currency
	}
	return nil
}

func nullableRiskRating(c authority.Consequence) any {
	if c.Type == value.ConsequenceTypeRiskRating {
		return string(c.RiskRating)
	}
	return nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

var _ authority.ProfileRepository = (*ProfileRepo)(nil)
