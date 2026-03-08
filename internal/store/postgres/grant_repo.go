package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/accept-io/midas/internal/authority"
)

type GrantRepo struct {
	db *sql.DB
}

func NewGrantRepo(db *sql.DB) (*GrantRepo, error) {
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
			effective_date,
			status,
			created_at,
			updated_at
		FROM agent_authorizations
		WHERE id = $1
	`

	var g authority.AuthorityGrant

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&g.ID,
		&g.AgentID,
		&g.ProfileID,
		&g.GrantedBy,
		&g.EffectiveDate,
		&g.Status,
		&g.CreatedAt,
		&g.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &g, nil
}

func (r *GrantRepo) FindActiveByAgentAndProfile(ctx context.Context, agentID, profileID string) (*authority.AuthorityGrant, error) {
	const q = `
		SELECT
			id,
			agent_id,
			profile_id,
			granted_by,
			effective_date,
			status,
			created_at,
			updated_at
		FROM agent_authorizations
		WHERE agent_id = $1
		  AND profile_id = $2
		  AND status = 'active'
		ORDER BY effective_date DESC, created_at DESC
		LIMIT 1
	`

	var g authority.AuthorityGrant

	err := r.db.QueryRowContext(ctx, q, agentID, profileID).Scan(
		&g.ID,
		&g.AgentID,
		&g.ProfileID,
		&g.GrantedBy,
		&g.EffectiveDate,
		&g.Status,
		&g.CreatedAt,
		&g.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &g, nil
}

func (r *GrantRepo) ListByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
	const q = `
		SELECT
			id,
			agent_id,
			profile_id,
			granted_by,
			effective_date,
			status,
			created_at,
			updated_at
		FROM agent_authorizations
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
		var g authority.AuthorityGrant

		if err := rows.Scan(
			&g.ID,
			&g.AgentID,
			&g.ProfileID,
			&g.GrantedBy,
			&g.EffectiveDate,
			&g.Status,
			&g.CreatedAt,
			&g.UpdatedAt,
		); err != nil {
			return nil, err
		}

		out = append(out, &g)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *GrantRepo) Create(ctx context.Context, g *authority.AuthorityGrant) error {
	const q = `
		INSERT INTO agent_authorizations (
			id,
			agent_id,
			profile_id,
			granted_by,
			effective_date,
			status,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		g.ID,
		g.AgentID,
		g.ProfileID,
		g.GrantedBy,
		g.EffectiveDate,
		g.Status,
		g.CreatedAt,
		g.UpdatedAt,
	)
	return err
}

func (r *GrantRepo) Revoke(ctx context.Context, id string) error {
	const q = `
		UPDATE agent_authorizations
		SET
			status = 'revoked',
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

var _ authority.GrantRepository = (*GrantRepo)(nil)
