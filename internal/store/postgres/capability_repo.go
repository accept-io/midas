package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// CapabilityRepo provides persistence operations for the capabilities table.
type CapabilityRepo struct {
	db sqltx.DBTX
}

func NewCapabilityRepo(db sqltx.DBTX) (*CapabilityRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &CapabilityRepo{db: db}, nil
}

// Exists reports whether a capability with the given ID exists in the capabilities table.
func (r *CapabilityRepo) Exists(ctx context.Context, id string) (bool, error) {
	const q = `SELECT 1 FROM capabilities WHERE capability_id = $1 LIMIT 1`
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

// GetByID returns the capability with the given ID, or nil if not found.
func (r *CapabilityRepo) GetByID(ctx context.Context, id string) (*capability.Capability, error) {
	const q = `
		SELECT capability_id, name, status, origin, managed, COALESCE(replaces, ''),
		       COALESCE(description, ''), COALESCE(owner_id, ''),
		       COALESCE(parent_capability_id, ''),
		       created_at, updated_at
		FROM capabilities
		WHERE capability_id = $1`
	var c capability.Capability
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&c.ID,
		&c.Name,
		&c.Status,
		&c.Origin,
		&c.Managed,
		&c.Replaces,
		&c.Description,
		&c.Owner,
		&c.ParentCapabilityID,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// Create inserts a new capability record.
func (r *CapabilityRepo) Create(ctx context.Context, c *capability.Capability) error {
	const q = `
		INSERT INTO capabilities
		  (capability_id, name, status, origin, managed, replaces, description, owner_id, parent_capability_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := r.db.ExecContext(ctx, q,
		c.ID,
		c.Name,
		c.Status,
		c.Origin,
		c.Managed,
		sql.NullString{Valid: c.Replaces != "", String: c.Replaces},
		c.Description,
		c.Owner,
		sql.NullString{Valid: c.ParentCapabilityID != "", String: c.ParentCapabilityID},
		c.CreatedAt,
		c.UpdatedAt,
	)
	return err
}

// Update modifies an existing capability record.
func (r *CapabilityRepo) Update(ctx context.Context, c *capability.Capability) error {
	const q = `
		UPDATE capabilities
		SET name=$2, status=$3, description=$4, owner_id=$5, updated_at=$6
		WHERE capability_id=$1`
	_, err := r.db.ExecContext(ctx, q,
		c.ID,
		c.Name,
		c.Status,
		c.Description,
		c.Owner,
		c.UpdatedAt,
	)
	return err
}

// List returns all capabilities ordered by capability_id.
func (r *CapabilityRepo) List(ctx context.Context) ([]*capability.Capability, error) {
	const q = `
		SELECT capability_id, name, status, origin, managed, COALESCE(replaces, ''),
		       COALESCE(description, ''), COALESCE(owner_id, ''),
		       COALESCE(parent_capability_id, ''),
		       created_at, updated_at
		FROM capabilities
		ORDER BY capability_id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*capability.Capability
	for rows.Next() {
		var c capability.Capability
		if err := rows.Scan(&c.ID, &c.Name, &c.Status, &c.Origin, &c.Managed, &c.Replaces, &c.Description, &c.Owner, &c.ParentCapabilityID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}
