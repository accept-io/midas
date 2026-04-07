package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
		SELECT capability_id, name, status,
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
		  (capability_id, name, status, description, owner_id, parent_capability_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.db.ExecContext(ctx, q,
		c.ID,
		c.Name,
		c.Status,
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

// EnsureInferred inserts the inferred capability if it does not exist, or validates
// that the existing row has compatible inferred semantics.
//
// Returns (true, nil) when a new row was created, (false, nil) when an existing row
// already has origin=inferred and managed=false, or an error if a conflicting row
// exists (e.g. origin=manual or managed=true).
//
// db must be a transaction when called from EnsureInferredStructure so the insert
// participates in the caller's transactional chain.
func (r *CapabilityRepo) EnsureInferred(ctx context.Context, db sqltx.DBTX, id string) (bool, error) {
	now := time.Now().UTC()
	const insert = `
		INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		VALUES ($1, $1, 'active', 'inferred', false, $2, $2)
		ON CONFLICT (capability_id) DO NOTHING`
	res, err := db.ExecContext(ctx, insert, id, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	// Row already exists; validate that it has compatible inferred semantics.
	const sel = `SELECT origin, managed FROM capabilities WHERE capability_id = $1`
	var origin string
	var managed bool
	if err := db.QueryRowContext(ctx, sel, id).Scan(&origin, &managed); err != nil {
		return false, err
	}
	if origin != "inferred" || managed {
		return false, fmt.Errorf("capability %s already exists with origin=%s managed=%v, cannot ensure as inferred entity", id, origin, managed)
	}
	return false, nil
}

// GetInferredMeta reads origin and managed for promotion validation.
// Returns (false, "", false, nil) when the row does not exist.
func (r *CapabilityRepo) GetInferredMeta(ctx context.Context, id string) (exists bool, origin string, managed bool, err error) {
	const q = `SELECT origin, managed FROM capabilities WHERE capability_id = $1`
	err = r.db.QueryRowContext(ctx, q, id).Scan(&origin, &managed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", false, nil
	}
	if err != nil {
		return false, "", false, err
	}
	return true, origin, managed, nil
}

// GetReplaces reads the replaces column for the given capability.
// Returns ("", false, nil) when the row does not exist or replaces is NULL.
func (r *CapabilityRepo) GetReplaces(ctx context.Context, id string) (replacesID string, ok bool, err error) {
	const q = `SELECT replaces FROM capabilities WHERE capability_id = $1`
	var rep sql.NullString
	err = r.db.QueryRowContext(ctx, q, id).Scan(&rep)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !rep.Valid {
		return "", false, nil
	}
	return rep.String, true, nil
}

// CreateManaged inserts a new managed capability with replaces pointing at the given
// inferred capability ID. Uses origin='manual' and managed=true.
func (r *CapabilityRepo) CreateManaged(ctx context.Context, db sqltx.DBTX, id, replacesID string) error {
	now := time.Now().UTC()
	const q = `
		INSERT INTO capabilities (capability_id, name, status, origin, managed, replaces, created_at, updated_at)
		VALUES ($1, $1, 'active', 'manual', true, $2, $3, $3)`
	_, err := db.ExecContext(ctx, q, id, replacesID, now)
	return err
}

// DeprecateInferred sets status='deprecated' on the given inferred capability.
// Returns an error if the row is not found or is not inferred.
func (r *CapabilityRepo) DeprecateInferred(ctx context.Context, db sqltx.DBTX, id string) error {
	now := time.Now().UTC()
	const q = `UPDATE capabilities SET status='deprecated', updated_at=$2 WHERE capability_id=$1 AND origin='inferred'`
	res, err := db.ExecContext(ctx, q, id, now)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("capability %q not found or is not inferred; cannot deprecate", id)
	}
	return nil
}

// FindEligibleForCleanup returns the IDs of deprecated inferred capabilities that
// are safe to delete. A zero cutoff matches all ages; a non-zero cutoff restricts
// to capabilities whose updated_at < cutoff.
//
// This must be called AFTER eligible processes have been deleted within the same
// transaction, so that capabilities only referenced by now-deleted processes become
// visible as eligible in this query.
//
// Eligibility (all must hold):
//  - origin = 'inferred', managed = false, status = 'deprecated'
//  - updated_at < cutoff (skipped when cutoff is zero)
//  - no process references this capability via capability_id
//  - no other capability has replaces = this capability_id
//  - no other capability has parent_capability_id = this capability_id
func (r *CapabilityRepo) FindEligibleForCleanup(ctx context.Context, db sqltx.DBTX, cutoff time.Time) ([]string, error) {
	var (
		rows *sql.Rows
		err  error
	)

	const baseQ = `
		SELECT c.capability_id
		FROM capabilities c
		WHERE c.origin  = 'inferred'
		  AND c.managed = false
		  AND c.status  = 'deprecated'
		  AND NOT EXISTS (SELECT 1 FROM processes    p  WHERE p.capability_id           = c.capability_id)
		  AND NOT EXISTS (SELECT 1 FROM capabilities c2 WHERE c2.replaces               = c.capability_id)
		  AND NOT EXISTS (SELECT 1 FROM capabilities c3 WHERE c3.parent_capability_id   = c.capability_id)`

	if cutoff.IsZero() {
		rows, err = db.QueryContext(ctx, baseQ+" ORDER BY c.capability_id")
	} else {
		rows, err = db.QueryContext(ctx, baseQ+" AND c.updated_at < $1 ORDER BY c.capability_id", cutoff)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteByIDs deletes capabilities with the given IDs within the given transaction.
// It is a no-op when ids is empty.
func (r *CapabilityRepo) DeleteByIDs(ctx context.Context, db sqltx.DBTX, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, len(ids))
	placeholders := make([]byte, 0, len(ids)*5)
	for i, id := range ids {
		args[i] = id
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, []byte(fmt.Sprintf("$%d", i+1))...)
	}
	q := "DELETE FROM capabilities WHERE capability_id IN (" + string(placeholders) + ")"
	_, err := db.ExecContext(ctx, q, args...)
	return err
}

// List returns all capabilities ordered by capability_id.
func (r *CapabilityRepo) List(ctx context.Context) ([]*capability.Capability, error) {
	const q = `
		SELECT capability_id, name, status,
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
		if err := rows.Scan(&c.ID, &c.Name, &c.Status, &c.Description, &c.Owner, &c.ParentCapabilityID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}
