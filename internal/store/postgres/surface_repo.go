package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/surface"
)

type SurfaceRepo struct {
	db *sql.DB
}

func NewSurfaceRepo(db *sql.DB) (*SurfaceRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	return &SurfaceRepo{db: db}, nil
}

func (r *SurfaceRepo) FindByID(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	const q = `
		SELECT
			id,
			name,
			domain,
			business_owner,
			technical_owner,
			status,
			version,
			effective_date,
			created_at,
			updated_at
		FROM decision_surfaces
		WHERE id = $1
		ORDER BY version DESC
		LIMIT 1
	`

	var s surface.DecisionSurface

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&s.ID,
		&s.Name,
		&s.Domain,
		&s.BusinessOwner,
		&s.TechnicalOwner,
		&s.Status,
		&s.Version,
		&s.EffectiveDate,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &s, nil
}

func (r *SurfaceRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*surface.DecisionSurface, error) {
	const q = `
		SELECT
			id,
			name,
			domain,
			business_owner,
			technical_owner,
			status,
			version,
			effective_date,
			created_at,
			updated_at
		FROM decision_surfaces
		WHERE id = $1
		  AND status = 'active'
		  AND effective_date <= $2
		ORDER BY effective_date DESC, version DESC
		LIMIT 1
	`

	var s surface.DecisionSurface

	err := r.db.QueryRowContext(ctx, q, id, at).Scan(
		&s.ID,
		&s.Name,
		&s.Domain,
		&s.BusinessOwner,
		&s.TechnicalOwner,
		&s.Status,
		&s.Version,
		&s.EffectiveDate,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &s, nil
}

func (r *SurfaceRepo) Create(ctx context.Context, s *surface.DecisionSurface) error {
	const q = `
		INSERT INTO decision_surfaces (
			id,
			version,
			name,
			domain,
			business_owner,
			technical_owner,
			status,
			effective_date,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		s.ID,
		s.Version,
		s.Name,
		s.Domain,
		s.BusinessOwner,
		s.TechnicalOwner,
		s.Status,
		s.EffectiveDate,
		s.CreatedAt,
		s.UpdatedAt,
	)
	return err
}

func (r *SurfaceRepo) Update(ctx context.Context, s *surface.DecisionSurface) error {
	const q = `
		UPDATE decision_surfaces
		SET
			name = $3,
			domain = $4,
			business_owner = $5,
			technical_owner = $6,
			status = $7,
			effective_date = $8,
			updated_at = $9
		WHERE id = $1
		  AND version = $2
	`

	res, err := r.db.ExecContext(
		ctx,
		q,
		s.ID,
		s.Version,
		s.Name,
		s.Domain,
		s.BusinessOwner,
		s.TechnicalOwner,
		s.Status,
		s.EffectiveDate,
		s.UpdatedAt,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("surface not found: id=%s version=%d", s.ID, s.Version)
	}

	return nil
}

func (r *SurfaceRepo) List(ctx context.Context) ([]*surface.DecisionSurface, error) {
	const q = `
		SELECT
			id,
			name,
			domain,
			business_owner,
			technical_owner,
			status,
			version,
			effective_date,
			created_at,
			updated_at
		FROM decision_surfaces
		ORDER BY id, version DESC
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*surface.DecisionSurface

	for rows.Next() {
		var s surface.DecisionSurface

		if err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.Domain,
			&s.BusinessOwner,
			&s.TechnicalOwner,
			&s.Status,
			&s.Version,
			&s.EffectiveDate,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, err
		}

		out = append(out, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// compile-time check
var _ surface.SurfaceRepository = (*SurfaceRepo)(nil)
