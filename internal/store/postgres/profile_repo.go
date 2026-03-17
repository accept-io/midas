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

// FindByID returns the latest version of a profile by its logical ID.
func (r *ProfileRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
	const q = `
		SELECT
			id,
			version,
			surface_id,
			name,
			description,
			status,
			effective_date,
			effective_until,
			retired_at,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
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

// FindByIDAndVersion retrieves a specific profile version.
func (r *ProfileRepo) FindByIDAndVersion(ctx context.Context, id string, version int) (*authority.AuthorityProfile, error) {
	const q = `
		SELECT
			id,
			version,
			surface_id,
			name,
			description,
			status,
			effective_date,
			effective_until,
			retired_at,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
		FROM authority_profiles
		WHERE id = $1 AND version = $2
	`

	p, err := scanProfileRow(r.db.QueryRowContext(ctx, q, id, version))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return p, nil
}

// FindActiveAt resolves the active version where:
//   - status = 'active'
//   - effective_date <= at
//   - (effective_until IS NULL OR effective_until > at)
//
// Schema v2.1: Now checks status field in addition to date range.
func (r *ProfileRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*authority.AuthorityProfile, error) {
	const q = `
		SELECT
			id,
			version,
			surface_id,
			name,
			description,
			status,
			effective_date,
			effective_until,
			retired_at,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
		FROM authority_profiles
		WHERE id = $1
		  AND status = 'active'
		  AND effective_date <= $2
		  AND (effective_until IS NULL OR effective_until > $2)
		ORDER BY version DESC
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
			version,
			surface_id,
			name,
			description,
			status,
			effective_date,
			effective_until,
			retired_at,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
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

// ListVersions returns all versions of a profile ordered by version DESC.
func (r *ProfileRepo) ListVersions(ctx context.Context, id string) ([]*authority.AuthorityProfile, error) {
	const q = `
		SELECT
			id,
			version,
			surface_id,
			name,
			description,
			status,
			effective_date,
			effective_until,
			retired_at,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
		FROM authority_profiles
		WHERE id = $1
		ORDER BY version DESC
	`

	rows, err := r.db.QueryContext(ctx, q, id)
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
			version,
			surface_id,
			name,
			description,
			status,
			effective_date,
			effective_until,
			retired_at,
			confidence_threshold,
			consequence_type,
			consequence_amount,
			consequence_currency,
			consequence_risk_rating,
			policy_reference,
			escalation_mode,
			fail_mode,
			required_context_keys,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
		)
	`

	_, err = r.db.ExecContext(
		ctx,
		q,
		p.ID,
		p.Version,
		p.SurfaceID,
		p.Name,
		nullableString(p.Description),
		p.Status,
		p.EffectiveDate,
		nullableTime(p.EffectiveUntil),
		nullableTime(p.RetiredAt),
		p.ConfidenceThreshold,
		p.ConsequenceThreshold.Type,
		nullableAmount(p.ConsequenceThreshold),
		nullableCurrency(p.ConsequenceThreshold),
		nullableRiskRating(p.ConsequenceThreshold),
		nullableString(p.PolicyReference),
		p.EscalationMode,
		p.FailMode,
		requiredContextKeys,
		p.CreatedAt,
		p.UpdatedAt,
		nullableString(p.CreatedBy),
		nullableString(p.ApprovedBy),
		nullableTime(p.ApprovedAt),
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
			description = $5,
			status = $6,
			effective_date = $7,
			effective_until = $8,
			retired_at = $9,
			confidence_threshold = $10,
			consequence_type = $11,
			consequence_amount = $12,
			consequence_currency = $13,
			consequence_risk_rating = $14,
			policy_reference = $15,
			escalation_mode = $16,
			fail_mode = $17,
			required_context_keys = $18,
			updated_at = $19,
			approved_by = $20,
			approved_at = $21
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
		nullableString(p.Description),
		p.Status,
		p.EffectiveDate,
		nullableTime(p.EffectiveUntil),
		nullableTime(p.RetiredAt),
		p.ConfidenceThreshold,
		p.ConsequenceThreshold.Type,
		nullableAmount(p.ConsequenceThreshold),
		nullableCurrency(p.ConsequenceThreshold),
		nullableRiskRating(p.ConsequenceThreshold),
		nullableString(p.PolicyReference),
		p.EscalationMode,
		p.FailMode,
		requiredContextKeys,
		p.UpdatedAt,
		nullableString(p.ApprovedBy),
		nullableTime(p.ApprovedAt),
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
		description          sql.NullString
		effectiveUntil       sql.NullTime
		retiredAt            sql.NullTime
		consequenceType      value.ConsequenceType
		consequenceAmount    sql.NullFloat64
		consequenceCurrency  sql.NullString
		consequenceRisk      sql.NullString
		policyReference      sql.NullString
		requiredContextBytes []byte
		createdBy            sql.NullString
		approvedBy           sql.NullString
		approvedAt           sql.NullTime
	)

	err := row.Scan(
		&p.ID,
		&p.Version,
		&p.SurfaceID,
		&p.Name,
		&description,
		&p.Status,
		&p.EffectiveDate,
		&effectiveUntil,
		&retiredAt,
		&p.ConfidenceThreshold,
		&consequenceType,
		&consequenceAmount,
		&consequenceCurrency,
		&consequenceRisk,
		&policyReference,
		&p.EscalationMode,
		&p.FailMode,
		&requiredContextBytes,
		&p.CreatedAt,
		&p.UpdatedAt,
		&createdBy,
		&approvedBy,
		&approvedAt,
	)
	if err != nil {
		return nil, err
	}

	if description.Valid {
		p.Description = description.String
	}
	if effectiveUntil.Valid {
		t := effectiveUntil.Time
		p.EffectiveUntil = &t
	}
	if retiredAt.Valid {
		t := retiredAt.Time
		p.RetiredAt = &t
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

	if createdBy.Valid {
		p.CreatedBy = createdBy.String
	}
	if approvedBy.Valid {
		p.ApprovedBy = approvedBy.String
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		p.ApprovedAt = &t
	}

	return &p, nil
}

func scanProfileRows(rows *sql.Rows) (*authority.AuthorityProfile, error) {
	return scanProfileRow(rows)
}

var _ authority.ProfileRepository = (*ProfileRepo)(nil)
