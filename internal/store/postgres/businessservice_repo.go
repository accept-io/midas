package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// BusinessServiceRepo provides persistence operations for the business_services table.
type BusinessServiceRepo struct {
	db sqltx.DBTX
}

func NewBusinessServiceRepo(db sqltx.DBTX) (*BusinessServiceRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &BusinessServiceRepo{db: db}, nil
}

// Exists reports whether a business service with the given ID exists.
func (r *BusinessServiceRepo) Exists(ctx context.Context, id string) (bool, error) {
	const q = `SELECT 1 FROM business_services WHERE business_service_id = $1 LIMIT 1`
	var dummy int
	err := r.db.QueryRowContext(ctx, q, id).Scan(&dummy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetByID returns the business service with the given ID, or nil if not found.
func (r *BusinessServiceRepo) GetByID(ctx context.Context, id string) (*businessservice.BusinessService, error) {
	const q = `
		SELECT business_service_id, name, COALESCE(description, ''),
		       service_type, COALESCE(regulatory_scope, ''),
		       status, origin, managed, COALESCE(replaces, ''), COALESCE(owner_id, ''),
		       created_at, updated_at,
		       ` + extRefSelectColumns + `
		FROM business_services
		WHERE business_service_id = $1`
	var s businessservice.BusinessService
	var extScan extRefScan
	dests := append([]any{
		&s.ID, &s.Name, &s.Description,
		&s.ServiceType, &s.RegulatoryScope,
		&s.Status, &s.Origin, &s.Managed, &s.Replaces, &s.OwnerID,
		&s.CreatedAt, &s.UpdatedAt,
	}, extScan.Dests()...)
	err := r.db.QueryRowContext(ctx, q, id).Scan(dests...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.ExternalRef = extScan.ToExternalRef()
	return &s, nil
}

// List returns all business services ordered by business_service_id.
func (r *BusinessServiceRepo) List(ctx context.Context) ([]*businessservice.BusinessService, error) {
	const q = `
		SELECT business_service_id, name, COALESCE(description, ''),
		       service_type, COALESCE(regulatory_scope, ''),
		       status, origin, managed, COALESCE(replaces, ''), COALESCE(owner_id, ''),
		       created_at, updated_at,
		       ` + extRefSelectColumns + `
		FROM business_services
		ORDER BY business_service_id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*businessservice.BusinessService
	for rows.Next() {
		var s businessservice.BusinessService
		var extScan extRefScan
		dests := append([]any{
			&s.ID, &s.Name, &s.Description,
			&s.ServiceType, &s.RegulatoryScope,
			&s.Status, &s.Origin, &s.Managed, &s.Replaces, &s.OwnerID,
			&s.CreatedAt, &s.UpdatedAt,
		}, extScan.Dests()...)
		if err := rows.Scan(dests...); err != nil {
			return nil, err
		}
		s.ExternalRef = extScan.ToExternalRef()
		out = append(out, &s)
	}
	return out, rows.Err()
}

// Create inserts a new business service record.
func (r *BusinessServiceRepo) Create(ctx context.Context, s *businessservice.BusinessService) error {
	const q = `
		INSERT INTO business_services
		  (business_service_id, name, description, service_type, regulatory_scope,
		   status, origin, managed, replaces, owner_id, created_at, updated_at,
		   ext_source_system, ext_source_id, ext_source_url, ext_source_version, ext_last_synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`
	args := append([]any{
		s.ID, s.Name, s.Description, s.ServiceType, s.RegulatoryScope,
		s.Status, s.Origin, s.Managed,
		sql.NullString{Valid: s.Replaces != "", String: s.Replaces},
		s.OwnerID, s.CreatedAt, s.UpdatedAt,
	}, extRefInsertValues(s.ExternalRef)...)
	_, err := r.db.ExecContext(ctx, q, args...)
	return mapExtRefError(err)
}

// Update modifies the mutable fields of an existing business service record.
func (r *BusinessServiceRepo) Update(ctx context.Context, s *businessservice.BusinessService) error {
	const q = `
		UPDATE business_services
		SET name=$2, description=$3, service_type=$4, regulatory_scope=$5,
		    status=$6, owner_id=$7, updated_at=$8,
		    ext_source_system=$9, ext_source_id=$10, ext_source_url=$11,
		    ext_source_version=$12, ext_last_synced_at=$13
		WHERE business_service_id=$1`
	args := append([]any{
		s.ID, s.Name, s.Description, s.ServiceType, s.RegulatoryScope,
		s.Status, s.OwnerID, s.UpdatedAt,
	}, extRefInsertValues(s.ExternalRef)...)
	_, err := r.db.ExecContext(ctx, q, args...)
	return mapExtRefError(err)
}
