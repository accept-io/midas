package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/store/sqltx"
	"github.com/accept-io/midas/internal/surface"
)

type SurfaceRepo struct {
	db sqltx.DBTX
}

func NewSurfaceRepo(db sqltx.DBTX) (*SurfaceRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	return &SurfaceRepo{db: db}, nil
}

// FindLatestByID returns the latest version (renamed from FindByID)
func (r *SurfaceRepo) FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error) {
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
		&s.EffectiveFrom, // DB column: effective_date, struct field: EffectiveFrom
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

// FindByIDVersion returns a specific version
func (r *SurfaceRepo) FindByIDVersion(ctx context.Context, id string, version int) (*surface.DecisionSurface, error) {
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
		WHERE id = $1 AND version = $2
		LIMIT 1
	`

	var s surface.DecisionSurface

	err := r.db.QueryRowContext(ctx, q, id, version).Scan(
		&s.ID,
		&s.Name,
		&s.Domain,
		&s.BusinessOwner,
		&s.TechnicalOwner,
		&s.Status,
		&s.Version,
		&s.EffectiveFrom,
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
		&s.EffectiveFrom,
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

// ListVersions returns all versions of a surface
func (r *SurfaceRepo) ListVersions(ctx context.Context, id string) ([]*surface.DecisionSurface, error) {
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
	`

	rows, err := r.db.QueryContext(ctx, q, id)
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
			&s.EffectiveFrom,
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

// ListAll returns the latest version of each surface (renamed from List)
func (r *SurfaceRepo) ListAll(ctx context.Context) ([]*surface.DecisionSurface, error) {
	const q = `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT
			s.id,
			s.name,
			s.domain,
			s.business_owner,
			s.technical_owner,
			s.status,
			s.version,
			s.effective_date,
			s.created_at,
			s.updated_at
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		ORDER BY s.id
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
			&s.EffectiveFrom,
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

// ListByStatus returns surfaces (latest version) with given status
func (r *SurfaceRepo) ListByStatus(ctx context.Context, status surface.SurfaceStatus) ([]*surface.DecisionSurface, error) {
	const q = `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT
			s.id,
			s.name,
			s.domain,
			s.business_owner,
			s.technical_owner,
			s.status,
			s.version,
			s.effective_date,
			s.created_at,
			s.updated_at
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		WHERE s.status = $1
		ORDER BY s.id
	`

	rows, err := r.db.QueryContext(ctx, q, string(status))
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
			&s.EffectiveFrom,
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

// ListByDomain returns surfaces (latest version) in given domain
func (r *SurfaceRepo) ListByDomain(ctx context.Context, domain string) ([]*surface.DecisionSurface, error) {
	const q = `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT
			s.id,
			s.name,
			s.domain,
			s.business_owner,
			s.technical_owner,
			s.status,
			s.version,
			s.effective_date,
			s.created_at,
			s.updated_at
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		WHERE s.domain = $1
		ORDER BY s.id
	`

	rows, err := r.db.QueryContext(ctx, q, domain)
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
			&s.EffectiveFrom,
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

// Search finds surfaces (latest version) matching criteria
// NOTE: This is a simplified implementation that only uses the existing schema fields.
// Full implementation requires Tags, Taxonomy, Category columns in the schema.
func (r *SurfaceRepo) Search(ctx context.Context, criteria surface.SearchCriteria) ([]*surface.DecisionSurface, error) {
	// For now, just filter by domain and status since those are the only fields we have
	query := `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT
			s.id,
			s.name,
			s.domain,
			s.business_owner,
			s.technical_owner,
			s.status,
			s.version,
			s.effective_date,
			s.created_at,
			s.updated_at
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		WHERE 1=1
	`

	args := []interface{}{}
	argNum := 1

	if criteria.Domain != "" {
		query += fmt.Sprintf(" AND s.domain = $%d", argNum)
		args = append(args, criteria.Domain)
		argNum++
	}

	if len(criteria.Status) > 0 {
		// Convert status slice to strings for IN clause
		statusStrings := make([]interface{}, len(criteria.Status))
		placeholders := ""
		for i, status := range criteria.Status {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += fmt.Sprintf("$%d", argNum)
			statusStrings[i] = string(status)
			argNum++
		}
		query += fmt.Sprintf(" AND s.status IN (%s)", placeholders)
		args = append(args, statusStrings...)
	}

	query += " ORDER BY s.id"

	rows, err := r.db.QueryContext(ctx, query, args...)
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
			&s.EffectiveFrom,
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
		s.EffectiveFrom, // DB column: effective_date, struct field: EffectiveFrom
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
		s.EffectiveFrom,
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

// compile-time check
var _ surface.SurfaceRepository = (*SurfaceRepo)(nil)
