package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/store/sqltx"
)

type GrantRepo struct {
	db sqltx.DBTX
}

func NewGrantRepo(db sqltx.DBTX) (*GrantRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	return &GrantRepo{db: db}, nil
}

func (r *GrantRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error) {
	const q = `
		SELECT
			id,
			agent_id,
			profile_id,
			granted_by,
			status,
			effective_date,
			expires_at,
			revoked_at,
			revoked_by,
			created_at,
			updated_at
		FROM authority_grants
		WHERE id = $1
	`

	g, err := scanGrantRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return g, nil
}

// FindActiveByAgentAndProfile returns the active grant linking agentID to profileID.
// Schema v2.1: Checks status='active' AND effective_date <= now AND (expires_at IS NULL OR expires_at > now)
func (r *GrantRepo) FindActiveByAgentAndProfile(ctx context.Context, agentID, profileID string) (*authority.AuthorityGrant, error) {
	const q = `
		SELECT
			id,
			agent_id,
			profile_id,
			granted_by,
			status,
			effective_date,
			expires_at,
			revoked_at,
			revoked_by,
			created_at,
			updated_at
		FROM authority_grants
		WHERE agent_id = $1
		  AND profile_id = $2
		  AND status = 'active'
		  AND effective_date <= NOW()
		  AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY effective_date DESC, created_at DESC
		LIMIT 1
	`

	g, err := scanGrantRow(r.db.QueryRowContext(ctx, q, agentID, profileID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return g, nil
}

func (r *GrantRepo) ListByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
	const q = `
		SELECT
			id,
			agent_id,
			profile_id,
			granted_by,
			status,
			effective_date,
			expires_at,
			revoked_at,
			revoked_by,
			created_at,
			updated_at
		FROM authority_grants
		WHERE agent_id = $1
		ORDER BY effective_date DESC, created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, q, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*authority.AuthorityGrant

	for rows.Next() {
		g, err := scanGrantRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *GrantRepo) Create(ctx context.Context, g *authority.AuthorityGrant) error {
	const q = `
		INSERT INTO authority_grants (
			id,
			agent_id,
			profile_id,
			granted_by,
			status,
			effective_date,
			expires_at,
			revoked_at,
			revoked_by,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		g.ID,
		g.AgentID,
		g.ProfileID,
		g.GrantedBy,
		g.Status,
		g.EffectiveDate,
		nullableTime(g.ExpiresAt),
		nullableTime(g.RevokedAt),
		nullableString(g.RevokedBy),
		g.CreatedAt,
		g.UpdatedAt,
	)
	return err
}

// Revoke marks a grant as revoked and records revocation metadata.
// Schema v2.1: Sets status='revoked', revoked_at=NOW(), revoked_by=revokedBy
func (r *GrantRepo) Revoke(ctx context.Context, id string, revokedBy string) error {
	const q = `
		UPDATE authority_grants
		SET
			status = 'revoked',
			revoked_at = NOW(),
			revoked_by = $2,
			updated_at = NOW()
		WHERE id = $1
	`

	res, err := r.db.ExecContext(ctx, q, id, revokedBy)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("grant not found: id=%s", id)
	}

	return nil
}

// Suspend temporarily disables a grant without full revocation.
// Schema v2.1: Sets status='suspended'
func (r *GrantRepo) Suspend(ctx context.Context, id string) error {
	const q = `
		UPDATE authority_grants
		SET
			status = 'suspended',
			updated_at = NOW()
		WHERE id = $1
	`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("grant not found: id=%s", id)
	}

	return nil
}

// Reactivate restores a suspended grant.
// Schema v2.1: Sets status='active' (only valid from suspended state)
func (r *GrantRepo) Reactivate(ctx context.Context, id string) error {
	const q = `
		UPDATE authority_grants
		SET
			status = 'active',
			updated_at = NOW()
		WHERE id = $1
		  AND status = 'suspended'
	`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("grant not found or not suspended: id=%s", id)
	}

	return nil
}

type grantScanner interface {
	Scan(dest ...any) error
}

func scanGrantRow(row grantScanner) (*authority.AuthorityGrant, error) {
	var (
		g         authority.AuthorityGrant
		expiresAt sql.NullTime
		revokedAt sql.NullTime
		revokedBy sql.NullString
	)

	err := row.Scan(
		&g.ID,
		&g.AgentID,
		&g.ProfileID,
		&g.GrantedBy,
		&g.Status,
		&g.EffectiveDate,
		&expiresAt,
		&revokedAt,
		&revokedBy,
		&g.CreatedAt,
		&g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		t := expiresAt.Time
		g.ExpiresAt = &t
	}
	if revokedAt.Valid {
		t := revokedAt.Time
		g.RevokedAt = &t
	}
	if revokedBy.Valid {
		g.RevokedBy = revokedBy.String
	}

	return &g, nil
}

func scanGrantRows(rows *sql.Rows) (*authority.AuthorityGrant, error) {
	return scanGrantRow(rows)
}

var _ authority.GrantRepository = (*GrantRepo)(nil)
