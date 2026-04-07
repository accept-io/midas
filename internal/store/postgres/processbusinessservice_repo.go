package postgres

import (
	"context"

	"github.com/accept-io/midas/internal/processbusinessservice"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// ProcessBusinessServiceRepo provides persistence operations for the process_business_services table.
type ProcessBusinessServiceRepo struct {
	db sqltx.DBTX
}

func NewProcessBusinessServiceRepo(db sqltx.DBTX) (*ProcessBusinessServiceRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &ProcessBusinessServiceRepo{db: db}, nil
}

// Create inserts a new process–business-service link.
func (r *ProcessBusinessServiceRepo) Create(ctx context.Context, pbs *processbusinessservice.ProcessBusinessService) error {
	const q = `
		INSERT INTO process_business_services (process_id, business_service_id, created_at)
		VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, q, pbs.ProcessID, pbs.BusinessServiceID, pbs.CreatedAt)
	return err
}

// ListByProcessID returns all business-service links for a given process, ordered by business_service_id.
func (r *ProcessBusinessServiceRepo) ListByProcessID(ctx context.Context, processID string) ([]*processbusinessservice.ProcessBusinessService, error) {
	const q = `
		SELECT process_id, business_service_id, created_at
		FROM process_business_services
		WHERE process_id = $1
		ORDER BY business_service_id`
	rows, err := r.db.QueryContext(ctx, q, processID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*processbusinessservice.ProcessBusinessService
	for rows.Next() {
		var pbs processbusinessservice.ProcessBusinessService
		if err := rows.Scan(&pbs.ProcessID, &pbs.BusinessServiceID, &pbs.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &pbs)
	}
	return out, rows.Err()
}

// ListByBusinessServiceID returns all process links for a given business service, ordered by process_id.
func (r *ProcessBusinessServiceRepo) ListByBusinessServiceID(ctx context.Context, businessServiceID string) ([]*processbusinessservice.ProcessBusinessService, error) {
	const q = `
		SELECT process_id, business_service_id, created_at
		FROM process_business_services
		WHERE business_service_id = $1
		ORDER BY process_id`
	rows, err := r.db.QueryContext(ctx, q, businessServiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*processbusinessservice.ProcessBusinessService
	for rows.Next() {
		var pbs processbusinessservice.ProcessBusinessService
		if err := rows.Scan(&pbs.ProcessID, &pbs.BusinessServiceID, &pbs.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &pbs)
	}
	return out, rows.Err()
}

// Delete removes the link between a process and a business service.
func (r *ProcessBusinessServiceRepo) Delete(ctx context.Context, processID, businessServiceID string) error {
	const q = `DELETE FROM process_business_services WHERE process_id = $1 AND business_service_id = $2`
	_, err := r.db.ExecContext(ctx, q, processID, businessServiceID)
	return err
}
