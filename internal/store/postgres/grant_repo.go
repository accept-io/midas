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

// grantColumns is the canonical SELECT column list for authority_grants.
// All read methods use this list; it must match the column order in scanGrantRow.
const grantColumns = `
	id,
	agent_id,
	profile_id,
	granted_by,
	grant_reason,
	status,
	effective_date,
	expires_at,
	revoked_at,
	revoked_by,
	revocation_reason,
	suspended_at,
	suspended_by,
	suspend_reason,
	created_at,
	updated_at
`

func (r *GrantRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error) {
	q := `SELECT` + grantColumns + `FROM authority_grants WHERE id = $1`

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
	q := `SELECT` + grantColumns + `
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
	q := `SELECT` + grantColumns + `
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
		g, err := scanGrantRow(rows)
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

func (r *GrantRepo) ListByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
	q := `SELECT` + grantColumns + `
		FROM authority_grants
		WHERE profile_id = $1
		ORDER BY effective_date DESC, created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, q, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*authority.AuthorityGrant

	for rows.Next() {
		g, err := scanGrantRow(rows)
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
			grant_reason,
			status,
			effective_date,
			expires_at,
			revoked_at,
			revoked_by,
			revocation_reason,
			suspended_at,
			suspended_by,
			suspend_reason,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		g.ID,
		g.AgentID,
		g.ProfileID,
		g.GrantedBy,
		nullableString(g.GrantReason),
		g.Status,
		g.EffectiveDate,
		nullableTime(g.ExpiresAt),
		nullableTime(g.RevokedAt),
		nullableString(g.RevokedBy),
		nullableString(g.RevokeReason),
		nullableTime(g.SuspendedAt),
		nullableString(g.SuspendedBy),
		nullableString(g.SuspendReason),
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

// Update persists all mutable fields of a grant atomically. Used by grant
// lifecycle governance (suspend, revoke, reinstate) to write actor, reason,
// and timestamp fields in a single operation.
func (r *GrantRepo) Update(ctx context.Context, g *authority.AuthorityGrant) error {
	const q = `
		UPDATE authority_grants
		SET
			status            = $2,
			granted_by        = $3,
			grant_reason      = $4,
			effective_date    = $5,
			expires_at        = $6,
			revoked_at        = $7,
			revoked_by        = $8,
			revocation_reason = $9,
			suspended_at      = $10,
			suspended_by      = $11,
			suspend_reason    = $12,
			updated_at        = $13
		WHERE id = $1
	`

	res, err := r.db.ExecContext(
		ctx, q,
		g.ID,
		g.Status,
		g.GrantedBy,
		nullableString(g.GrantReason),
		g.EffectiveDate,
		nullableTime(g.ExpiresAt),
		nullableTime(g.RevokedAt),
		nullableString(g.RevokedBy),
		nullableString(g.RevokeReason),
		nullableTime(g.SuspendedAt),
		nullableString(g.SuspendedBy),
		nullableString(g.SuspendReason),
		g.UpdatedAt,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("grant not found: id=%s", g.ID)
	}

	return nil
}

type grantScanner interface {
	Scan(dest ...any) error
}

// scanGrantRow scans the canonical column set defined by grantColumns.
// Column order must match grantColumns exactly.
func scanGrantRow(row grantScanner) (*authority.AuthorityGrant, error) {
	var (
		g                authority.AuthorityGrant
		grantReason      sql.NullString
		expiresAt        sql.NullTime
		revokedAt        sql.NullTime
		revokedBy        sql.NullString
		revocationReason sql.NullString
		suspendedAt      sql.NullTime
		suspendedBy      sql.NullString
		suspendReason    sql.NullString
	)

	err := row.Scan(
		&g.ID,
		&g.AgentID,
		&g.ProfileID,
		&g.GrantedBy,
		&grantReason,
		&g.Status,
		&g.EffectiveDate,
		&expiresAt,
		&revokedAt,
		&revokedBy,
		&revocationReason,
		&suspendedAt,
		&suspendedBy,
		&suspendReason,
		&g.CreatedAt,
		&g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if grantReason.Valid {
		g.GrantReason = grantReason.String
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
	if revocationReason.Valid {
		g.RevokeReason = revocationReason.String
	}
	if suspendedAt.Valid {
		t := suspendedAt.Time
		g.SuspendedAt = &t
	}
	if suspendedBy.Valid {
		g.SuspendedBy = suspendedBy.String
	}
	if suspendReason.Valid {
		g.SuspendReason = suspendReason.String
	}

	return &g, nil
}

var _ authority.GrantRepository = (*GrantRepo)(nil)
