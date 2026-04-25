package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
		SELECT process_id, name, capability_id, status, origin, managed, COALESCE(replaces, ''),
		       COALESCE(description, ''), COALESCE(owner_id, ''),
		       COALESCE(business_service_id, ''),
		       created_at, updated_at
		FROM processes
		WHERE process_id = $1`
	var p process.Process
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&p.ID,
		&p.Name,
		&p.CapabilityID,
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

// Create inserts a new process record.
func (r *ProcessRepo) Create(ctx context.Context, p *process.Process) error {
	const q = `
		INSERT INTO processes
		  (process_id, capability_id, parent_process_id, name, status, origin, managed, replaces,
		   description, owner_id, business_service_id, created_at, updated_at, level)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NULL)`
	_, err := r.db.ExecContext(ctx, q,
		p.ID,
		p.CapabilityID,
		sql.NullString{Valid: p.ParentProcessID != "", String: p.ParentProcessID},
		p.Name,
		p.Status,
		p.Origin,
		p.Managed,
		sql.NullString{Valid: p.Replaces != "", String: p.Replaces},
		p.Description,
		p.Owner,
		sql.NullString{Valid: p.BusinessServiceID != "", String: p.BusinessServiceID},
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
		sql.NullString{Valid: p.BusinessServiceID != "", String: p.BusinessServiceID},
		p.UpdatedAt,
	)
	return err
}

// EnsureInferred inserts the inferred process if it does not exist, or validates
// that the existing row has compatible inferred semantics.
//
// Returns (true, nil) when a new row was created, (false, nil) when an existing row
// already has origin=inferred, managed=false, and the correct capabilityID, or an
// error if a conflicting row exists (e.g. wrong origin, managed=true, or wrong parent).
//
// db must be a transaction when called from EnsureInferredStructure.
func (r *ProcessRepo) EnsureInferred(ctx context.Context, db sqltx.DBTX, processID, capabilityID string) (bool, error) {
	now := time.Now().UTC()
	const insert = `
		INSERT INTO processes (process_id, capability_id, name, status, origin, managed, created_at, updated_at, level)
		VALUES ($1, $2, $1, 'active', 'inferred', false, $3, $3, NULL)
		ON CONFLICT (process_id) DO NOTHING`
	res, err := db.ExecContext(ctx, insert, processID, capabilityID, now)
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
	const sel = `SELECT origin, managed, capability_id FROM processes WHERE process_id = $1`
	var origin string
	var managed bool
	var existingCapID string
	if err := db.QueryRowContext(ctx, sel, processID).Scan(&origin, &managed, &existingCapID); err != nil {
		return false, err
	}
	if origin != "inferred" || managed || existingCapID != capabilityID {
		return false, fmt.Errorf(
			"process %s already exists with origin=%s managed=%v capability_id=%s, cannot ensure as inferred entity linked to %s",
			processID, origin, managed, existingCapID, capabilityID,
		)
	}
	return false, nil
}

// GetInferredMeta reads origin, managed, and capability_id for promotion validation.
// Returns (false, "", false, "", nil) when the row does not exist.
func (r *ProcessRepo) GetInferredMeta(ctx context.Context, id string) (exists bool, origin string, managed bool, capabilityID string, err error) {
	const q = `SELECT origin, managed, capability_id FROM processes WHERE process_id = $1`
	err = r.db.QueryRowContext(ctx, q, id).Scan(&origin, &managed, &capabilityID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", false, "", nil
	}
	if err != nil {
		return false, "", false, "", err
	}
	return true, origin, managed, capabilityID, nil
}

// GetReplaces reads the replaces column for the given process.
// Returns ("", false, nil) when the row does not exist or replaces is NULL.
func (r *ProcessRepo) GetReplaces(ctx context.Context, id string) (replacesID string, ok bool, err error) {
	const q = `SELECT replaces FROM processes WHERE process_id = $1`
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

// CreateManaged inserts a new managed process with replaces pointing at the given
// inferred process ID. Uses origin='manual' and managed=true.
func (r *ProcessRepo) CreateManaged(ctx context.Context, db sqltx.DBTX, id, capabilityID, replacesID string) error {
	now := time.Now().UTC()
	const q = `
		INSERT INTO processes (process_id, capability_id, name, status, origin, managed, replaces, created_at, updated_at, level)
		VALUES ($1, $2, $1, 'active', 'manual', true, $3, $4, $4, NULL)`
	_, err := db.ExecContext(ctx, q, id, capabilityID, replacesID, now)
	return err
}

// DeprecateInferred sets status='deprecated' on the given inferred process.
// Returns an error if the row is not found or is not inferred.
func (r *ProcessRepo) DeprecateInferred(ctx context.Context, db sqltx.DBTX, id string) error {
	now := time.Now().UTC()
	const q = `UPDATE processes SET status='deprecated', updated_at=$2 WHERE process_id=$1 AND origin='inferred'`
	res, err := db.ExecContext(ctx, q, id, now)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("process %q not found or is not inferred; cannot deprecate", id)
	}
	return nil
}

// List returns all processes ordered by process_id.
func (r *ProcessRepo) List(ctx context.Context) ([]*process.Process, error) {
	const q = `
		SELECT process_id, name, capability_id, status, origin, managed, COALESCE(replaces, ''),
		       COALESCE(description, ''), COALESCE(owner_id, ''),
		       COALESCE(business_service_id, ''),
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
		if err := rows.Scan(&p.ID, &p.Name, &p.CapabilityID, &p.Status, &p.Origin, &p.Managed, &p.Replaces, &p.Description, &p.Owner, &p.BusinessServiceID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// FindEligibleForCleanup returns the IDs of deprecated inferred processes that are
// safe to delete. A zero cutoff matches all ages; a non-zero cutoff restricts to
// processes whose updated_at < cutoff.
//
// Eligibility (all must hold):
//  - origin = 'inferred', managed = false, status = 'deprecated'
//  - updated_at < cutoff (skipped when cutoff is zero)
//  - no decision_surface references this process (provides transitive envelope protection)
//  - no other process has replaces = this process_id
//  - no other process has parent_process_id = this process_id
func (r *ProcessRepo) FindEligibleForCleanup(ctx context.Context, db sqltx.DBTX, cutoff time.Time) ([]string, error) {
	var (
		rows *sql.Rows
		err  error
	)

	const baseQ = `
		SELECT p.process_id
		FROM processes p
		WHERE p.origin    = 'inferred'
		  AND p.managed   = false
		  AND p.status    = 'deprecated'
		  AND NOT EXISTS (SELECT 1 FROM decision_surfaces ds WHERE ds.process_id = p.process_id)
		  AND NOT EXISTS (SELECT 1 FROM processes p2 WHERE p2.replaces         = p.process_id)
		  AND NOT EXISTS (SELECT 1 FROM processes p3 WHERE p3.parent_process_id = p.process_id)`

	if cutoff.IsZero() {
		rows, err = db.QueryContext(ctx, baseQ+" ORDER BY p.process_id")
	} else {
		rows, err = db.QueryContext(ctx, baseQ+" AND p.updated_at < $1 ORDER BY p.process_id", cutoff)
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

// DeleteByIDs deletes processes with the given IDs within the given transaction.
// It is a no-op when ids is empty.
func (r *ProcessRepo) DeleteByIDs(ctx context.Context, db sqltx.DBTX, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	// Build a parameterised IN clause.
	args := make([]any, len(ids))
	placeholders := make([]byte, 0, len(ids)*5)
	for i, id := range ids {
		args[i] = id
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, []byte(fmt.Sprintf("$%d", i+1))...)
	}
	q := "DELETE FROM processes WHERE process_id IN (" + string(placeholders) + ")"
	_, err := db.ExecContext(ctx, q, args...)
	return err
}

// ListByCapabilityID returns processes belonging to the given capability, ordered by process_id.
func (r *ProcessRepo) ListByCapabilityID(ctx context.Context, capabilityID string) ([]*process.Process, error) {
	const q = `
		SELECT process_id, name, capability_id, status, origin, managed, COALESCE(replaces, ''),
		       COALESCE(description, ''), COALESCE(owner_id, ''),
		       COALESCE(business_service_id, ''),
		       created_at, updated_at
		FROM processes
		WHERE capability_id = $1
		ORDER BY process_id`
	rows, err := r.db.QueryContext(ctx, q, capabilityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*process.Process
	for rows.Next() {
		var p process.Process
		if err := rows.Scan(&p.ID, &p.Name, &p.CapabilityID, &p.Status, &p.Origin, &p.Managed, &p.Replaces, &p.Description, &p.Owner, &p.BusinessServiceID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}
