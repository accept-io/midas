package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// ProcessRepo provides persistence operations for the processes table.
type ProcessRepo struct {
	db sqltx.DBTX
}

func NewProcessRepo(db sqltx.DBTX) (*ProcessRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &ProcessRepo{db: db}, nil
}

// Exists reports whether a process with the given ID exists in the processes table.
func (r *ProcessRepo) Exists(ctx context.Context, id string) (bool, error) {
	const q = `SELECT 1 FROM processes WHERE process_id = $1 LIMIT 1`
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

// GetByID returns the process with the given ID, or nil if not found.
func (r *ProcessRepo) GetByID(ctx context.Context, id string) (*process.Process, error) {
	const q = `
		SELECT process_id, name, status, origin, managed, COALESCE(replaces, ''),
		       COALESCE(description, ''), COALESCE(owner_id, ''),
		       business_service_id,
		       created_at, updated_at
		FROM processes
		WHERE process_id = $1`
	var p process.Process
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&p.ID,
		&p.Name,
		&p.Status,
		&p.Origin,
		&p.Managed,
		&p.Replaces,
		&p.Description,
		&p.Owner,
		&p.BusinessServiceID,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// Create inserts a new process record. business_service_id is required by the
// database (NOT NULL); empty string is rejected before the round-trip.
func (r *ProcessRepo) Create(ctx context.Context, p *process.Process) error {
	if p.BusinessServiceID == "" {
		return fmt.Errorf("process %q: business_service_id is required", p.ID)
	}
	const q = `
		INSERT INTO processes
		  (process_id, parent_process_id, name, status, origin, managed, replaces,
		   description, owner_id, business_service_id, created_at, updated_at, level)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULL)`
	_, err := r.db.ExecContext(ctx, q,
		p.ID,
		sql.NullString{Valid: p.ParentProcessID != "", String: p.ParentProcessID},
		p.Name,
		p.Status,
		p.Origin,
		p.Managed,
		sql.NullString{Valid: p.Replaces != "", String: p.Replaces},
		p.Description,
		p.Owner,
		p.BusinessServiceID,
		p.CreatedAt,
		p.UpdatedAt,
	)
	return err
}

// Update modifies an existing process record.
func (r *ProcessRepo) Update(ctx context.Context, p *process.Process) error {
	const q = `
		UPDATE processes
		SET name=$2, status=$3, description=$4, owner_id=$5,
		    business_service_id=$6, updated_at=$7
		WHERE process_id=$1`
	_, err := r.db.ExecContext(ctx, q,
		p.ID,
		p.Name,
		p.Status,
		p.Description,
		p.Owner,
		p.BusinessServiceID,
		p.UpdatedAt,
	)
	return err
}

// List returns all processes ordered by process_id.
func (r *ProcessRepo) List(ctx context.Context) ([]*process.Process, error) {
	const q = `
		SELECT process_id, name, status, origin, managed, COALESCE(replaces, ''),
		       COALESCE(description, ''), COALESCE(owner_id, ''),
		       business_service_id,
		       created_at, updated_at
		FROM processes
		ORDER BY process_id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*process.Process
	for rows.Next() {
		var p process.Process
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.Origin, &p.Managed, &p.Replaces, &p.Description, &p.Owner, &p.BusinessServiceID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

