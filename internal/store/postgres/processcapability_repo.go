package postgres

import (
	"context"

	"github.com/accept-io/midas/internal/processcapability"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// ProcessCapabilityRepo provides persistence operations for the process_capabilities table.
type ProcessCapabilityRepo struct {
	db sqltx.DBTX
}

func NewProcessCapabilityRepo(db sqltx.DBTX) (*ProcessCapabilityRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &ProcessCapabilityRepo{db: db}, nil
}

// Create inserts a new process-capability link.
func (r *ProcessCapabilityRepo) Create(ctx context.Context, pc *processcapability.ProcessCapability) error {
	const q = `
		INSERT INTO process_capabilities (process_id, capability_id, created_at)
		VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, q, pc.ProcessID, pc.CapabilityID, pc.CreatedAt)
	return err
}

// ListByProcessID returns all capability links for a given process, ordered by capability_id.
func (r *ProcessCapabilityRepo) ListByProcessID(ctx context.Context, processID string) ([]*processcapability.ProcessCapability, error) {
	const q = `
		SELECT process_id, capability_id, created_at
		FROM process_capabilities
		WHERE process_id = $1
		ORDER BY capability_id`
	rows, err := r.db.QueryContext(ctx, q, processID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*processcapability.ProcessCapability
	for rows.Next() {
		var pc processcapability.ProcessCapability
		if err := rows.Scan(&pc.ProcessID, &pc.CapabilityID, &pc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &pc)
	}
	return out, rows.Err()
}

// ListByCapabilityID returns all process links for a given capability, ordered by process_id.
func (r *ProcessCapabilityRepo) ListByCapabilityID(ctx context.Context, capabilityID string) ([]*processcapability.ProcessCapability, error) {
	const q = `
		SELECT process_id, capability_id, created_at
		FROM process_capabilities
		WHERE capability_id = $1
		ORDER BY process_id`
	rows, err := r.db.QueryContext(ctx, q, capabilityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*processcapability.ProcessCapability
	for rows.Next() {
		var pc processcapability.ProcessCapability
		if err := rows.Scan(&pc.ProcessID, &pc.CapabilityID, &pc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &pc)
	}
	return out, rows.Err()
}

// Delete removes the link between a process and a capability.
func (r *ProcessCapabilityRepo) Delete(ctx context.Context, processID, capabilityID string) error {
	const q = `DELETE FROM process_capabilities WHERE process_id = $1 AND capability_id = $2`
	_, err := r.db.ExecContext(ctx, q, processID, capabilityID)
	return err
}
