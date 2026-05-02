package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// AISystemRepo provides persistence for the ai_systems table.
//
// Constraint mapping (Postgres → repository sentinel):
//
//   - UNIQUE violation on PK (id)            → ErrAISystemAlreadyExists
//   - CHECK chk_ai_systems_status            → ErrInvalidStatus
//   - CHECK chk_ai_systems_origin            → ErrInvalidOrigin
//   - CHECK chk_ai_systems_no_self_replace   → ErrSelfReplace
//
// Other errors propagate via fmt.Errorf so callers can inspect with
// errors.Is / errors.As.
type AISystemRepo struct {
	db sqltx.DBTX
}

// NewAISystemRepo constructs a repo bound to db. Returns ErrNilDB when db is nil.
func NewAISystemRepo(db sqltx.DBTX) (*AISystemRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &AISystemRepo{db: db}, nil
}

func (r *AISystemRepo) Create(ctx context.Context, sys *aisystem.AISystem) error {
	const q = `
		INSERT INTO ai_systems (
			id, name, description, owner, vendor, system_type,
			status, origin, managed, replaces,
			created_at, updated_at, created_by,
			ext_source_system, ext_source_id, ext_source_url, ext_source_version, ext_last_synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`
	args := append([]any{
		sys.ID,
		sys.Name,
		nullableString(sys.Description),
		nullableString(sys.Owner),
		nullableString(sys.Vendor),
		nullableString(sys.SystemType),
		sys.Status,
		sys.Origin,
		sys.Managed,
		nullableString(sys.Replaces),
		sys.CreatedAt,
		sys.UpdatedAt,
		nullableString(sys.CreatedBy),
	}, extRefInsertValues(sys.ExternalRef)...)
	_, err := r.db.ExecContext(ctx, q, args...)
	return mapAISystemCreateErr(err)
}

func (r *AISystemRepo) GetByID(ctx context.Context, id string) (*aisystem.AISystem, error) {
	const q = `
		SELECT id, name, COALESCE(description, ''),
		       COALESCE(owner, ''), COALESCE(vendor, ''), COALESCE(system_type, ''),
		       status, origin, managed, COALESCE(replaces, ''),
		       created_at, updated_at, COALESCE(created_by, ''),
		       ` + extRefSelectColumns + `
		FROM ai_systems
		WHERE id = $1`
	var sys aisystem.AISystem
	var extScan extRefScan
	dests := append([]any{
		&sys.ID, &sys.Name, &sys.Description,
		&sys.Owner, &sys.Vendor, &sys.SystemType,
		&sys.Status, &sys.Origin, &sys.Managed, &sys.Replaces,
		&sys.CreatedAt, &sys.UpdatedAt, &sys.CreatedBy,
	}, extScan.Dests()...)
	err := r.db.QueryRowContext(ctx, q, id).Scan(dests...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, aisystem.ErrAISystemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get ai system: %w", err)
	}
	sys.ExternalRef = extScan.ToExternalRef()
	return &sys, nil
}

func (r *AISystemRepo) Exists(ctx context.Context, id string) (bool, error) {
	const q = `SELECT 1 FROM ai_systems WHERE id = $1 LIMIT 1`
	var dummy int
	err := r.db.QueryRowContext(ctx, q, id).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *AISystemRepo) List(ctx context.Context) ([]*aisystem.AISystem, error) {
	const q = `
		SELECT id, name, COALESCE(description, ''),
		       COALESCE(owner, ''), COALESCE(vendor, ''), COALESCE(system_type, ''),
		       status, origin, managed, COALESCE(replaces, ''),
		       created_at, updated_at, COALESCE(created_by, ''),
		       ` + extRefSelectColumns + `
		FROM ai_systems
		ORDER BY created_at DESC, id ASC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*aisystem.AISystem, 0)
	for rows.Next() {
		var sys aisystem.AISystem
		var extScan extRefScan
		dests := append([]any{
			&sys.ID, &sys.Name, &sys.Description,
			&sys.Owner, &sys.Vendor, &sys.SystemType,
			&sys.Status, &sys.Origin, &sys.Managed, &sys.Replaces,
			&sys.CreatedAt, &sys.UpdatedAt, &sys.CreatedBy,
		}, extScan.Dests()...)
		if err := rows.Scan(dests...); err != nil {
			return nil, err
		}
		sys.ExternalRef = extScan.ToExternalRef()
		out = append(out, &sys)
	}
	return out, rows.Err()
}

func (r *AISystemRepo) Update(ctx context.Context, sys *aisystem.AISystem) error {
	const q = `
		UPDATE ai_systems
		   SET name = $2,
		       description = $3,
		       owner = $4,
		       vendor = $5,
		       system_type = $6,
		       status = $7,
		       origin = $8,
		       managed = $9,
		       replaces = $10,
		       updated_at = $11,
		       ext_source_system = $12,
		       ext_source_id = $13,
		       ext_source_url = $14,
		       ext_source_version = $15,
		       ext_last_synced_at = $16
		 WHERE id = $1`
	args := append([]any{
		sys.ID,
		sys.Name,
		nullableString(sys.Description),
		nullableString(sys.Owner),
		nullableString(sys.Vendor),
		nullableString(sys.SystemType),
		sys.Status,
		sys.Origin,
		sys.Managed,
		nullableString(sys.Replaces),
		sys.UpdatedAt,
	}, extRefInsertValues(sys.ExternalRef)...)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return mapAISystemUpdateErr(err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update ai system rows affected: %w", err)
	}
	if rows == 0 {
		return aisystem.ErrAISystemNotFound
	}
	return nil
}

func mapAISystemCreateErr(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return fmt.Errorf("create ai system: %w", err)
	}
	switch pqErr.Code {
	case "23505": // unique_violation
		return aisystem.ErrAISystemAlreadyExists
	case "23514": // check_violation
		switch pqErr.Constraint {
		case "chk_ai_systems_status":
			return aisystem.ErrInvalidStatus
		case "chk_ai_systems_origin":
			return aisystem.ErrInvalidOrigin
		case "chk_ai_systems_no_self_replace":
			return aisystem.ErrSelfReplace
		case "chk_ai_systems_ext_consistency":
			return mapExtRefError(err)
		}
	case "23503": // foreign_key_violation
		// replaces FK to ai_systems(id): the referenced predecessor system
		// is missing. Surface verbatim — the operator can read the column
		// out of pq.Detail.
		return fmt.Errorf("create ai system: referenced predecessor not found: %w", err)
	}
	return fmt.Errorf("create ai system: %w", err)
}

func mapAISystemUpdateErr(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return fmt.Errorf("update ai system: %w", err)
	}
	switch pqErr.Code {
	case "23514": // check_violation
		switch pqErr.Constraint {
		case "chk_ai_systems_status":
			return aisystem.ErrInvalidStatus
		case "chk_ai_systems_origin":
			return aisystem.ErrInvalidOrigin
		case "chk_ai_systems_no_self_replace":
			return aisystem.ErrSelfReplace
		case "chk_ai_systems_ext_consistency":
			return mapExtRefError(err)
		}
	}
	return fmt.Errorf("update ai system: %w", err)
}

var _ aisystem.SystemRepository = (*AISystemRepo)(nil)
