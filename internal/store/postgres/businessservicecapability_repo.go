package postgres

import (
	"context"

	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// BusinessServiceCapabilityRepo provides persistence operations for the
// business_service_capabilities junction table.
type BusinessServiceCapabilityRepo struct {
	db sqltx.DBTX
}

func NewBusinessServiceCapabilityRepo(db sqltx.DBTX) (*BusinessServiceCapabilityRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &BusinessServiceCapabilityRepo{db: db}, nil
}

// Create inserts a new business-service ↔ capability link.
func (r *BusinessServiceCapabilityRepo) Create(ctx context.Context, bsc *businessservicecapability.BusinessServiceCapability) error {
	const q = `
		INSERT INTO business_service_capabilities (business_service_id, capability_id, created_at)
		VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, q, bsc.BusinessServiceID, bsc.CapabilityID, bsc.CreatedAt)
	return err
}

// Exists reports whether a junction row already exists for the given
// business-service ↔ capability pair.
func (r *BusinessServiceCapabilityRepo) Exists(ctx context.Context, businessServiceID, capabilityID string) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM business_service_capabilities
			WHERE business_service_id = $1 AND capability_id = $2
		)`
	var ok bool
	if err := r.db.QueryRowContext(ctx, q, businessServiceID, capabilityID).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

// ListByBusinessServiceID returns all capability links for a given business service, ordered by capability_id.
func (r *BusinessServiceCapabilityRepo) ListByBusinessServiceID(ctx context.Context, businessServiceID string) ([]*businessservicecapability.BusinessServiceCapability, error) {
	const q = `
		SELECT business_service_id, capability_id, created_at
		FROM business_service_capabilities
		WHERE business_service_id = $1
		ORDER BY capability_id`
	rows, err := r.db.QueryContext(ctx, q, businessServiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*businessservicecapability.BusinessServiceCapability
	for rows.Next() {
		var bsc businessservicecapability.BusinessServiceCapability
		if err := rows.Scan(&bsc.BusinessServiceID, &bsc.CapabilityID, &bsc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &bsc)
	}
	return out, rows.Err()
}

// ListByCapabilityID returns all business-service links for a given capability, ordered by business_service_id.
func (r *BusinessServiceCapabilityRepo) ListByCapabilityID(ctx context.Context, capabilityID string) ([]*businessservicecapability.BusinessServiceCapability, error) {
	const q = `
		SELECT business_service_id, capability_id, created_at
		FROM business_service_capabilities
		WHERE capability_id = $1
		ORDER BY business_service_id`
	rows, err := r.db.QueryContext(ctx, q, capabilityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*businessservicecapability.BusinessServiceCapability
	for rows.Next() {
		var bsc businessservicecapability.BusinessServiceCapability
		if err := rows.Scan(&bsc.BusinessServiceID, &bsc.CapabilityID, &bsc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &bsc)
	}
	return out, rows.Err()
}

// Delete removes the link between a business service and a capability.
func (r *BusinessServiceCapabilityRepo) Delete(ctx context.Context, businessServiceID, capabilityID string) error {
	const q = `DELETE FROM business_service_capabilities WHERE business_service_id = $1 AND capability_id = $2`
	_, err := r.db.ExecContext(ctx, q, businessServiceID, capabilityID)
	return err
}
