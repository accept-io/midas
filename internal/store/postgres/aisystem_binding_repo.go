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

// AISystemBindingRepo provides persistence for the ai_system_bindings table.
//
// Constraint mapping (Postgres → repository sentinel):
//
//   - UNIQUE violation on PK (id)                              → ErrAISystemBindingAlreadyExists
//   - CHECK chk_ai_bindings_at_least_one_target                → ErrBindingMissingContext
//   - FK violation on ai_system_id / fk_ai_bindings_version /
//     business_service_id / capability_id / process_id          → wrapped FK error
type AISystemBindingRepo struct {
	db sqltx.DBTX
}

// NewAISystemBindingRepo constructs a repo bound to db. Returns ErrNilDB when db is nil.
func NewAISystemBindingRepo(db sqltx.DBTX) (*AISystemBindingRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &AISystemBindingRepo{db: db}, nil
}

func (r *AISystemBindingRepo) Create(ctx context.Context, b *aisystem.AISystemBinding) error {
	const q = `
		INSERT INTO ai_system_bindings (
			id, ai_system_id, ai_system_version,
			business_service_id, capability_id, process_id, surface_id,
			role, description,
			created_at, created_by,
			ext_source_system, ext_source_id, ext_source_url, ext_source_version, ext_last_synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`
	var version any
	if b.AISystemVersion != nil {
		version = *b.AISystemVersion
	}
	args := append([]any{
		b.ID,
		b.AISystemID,
		version,
		nullableString(b.BusinessServiceID),
		nullableString(b.CapabilityID),
		nullableString(b.ProcessID),
		nullableString(b.SurfaceID),
		nullableString(b.Role),
		nullableString(b.Description),
		b.CreatedAt,
		nullableString(b.CreatedBy),
	}, extRefInsertValues(b.ExternalRef)...)
	_, err := r.db.ExecContext(ctx, q, args...)
	return mapAIBindingCreateErr(err)
}

func (r *AISystemBindingRepo) GetByID(ctx context.Context, id string) (*aisystem.AISystemBinding, error) {
	const q = bindingSelectColumns + ` FROM ai_system_bindings WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	b, err := scanAIBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, aisystem.ErrAISystemBindingNotFound
	}
	return b, err
}

func (r *AISystemBindingRepo) List(ctx context.Context) ([]*aisystem.AISystemBinding, error) {
	return r.queryBindings(ctx, bindingSelectColumns+` FROM ai_system_bindings ORDER BY created_at DESC, id ASC`)
}

func (r *AISystemBindingRepo) ListByAISystem(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemBinding, error) {
	return r.queryBindings(ctx,
		bindingSelectColumns+` FROM ai_system_bindings WHERE ai_system_id = $1 ORDER BY created_at DESC, id ASC`,
		aiSystemID)
}

func (r *AISystemBindingRepo) ListByBusinessService(ctx context.Context, bsID string) ([]*aisystem.AISystemBinding, error) {
	return r.queryBindings(ctx,
		bindingSelectColumns+` FROM ai_system_bindings WHERE business_service_id = $1 ORDER BY created_at DESC, id ASC`,
		bsID)
}

func (r *AISystemBindingRepo) ListByCapability(ctx context.Context, capID string) ([]*aisystem.AISystemBinding, error) {
	return r.queryBindings(ctx,
		bindingSelectColumns+` FROM ai_system_bindings WHERE capability_id = $1 ORDER BY created_at DESC, id ASC`,
		capID)
}

func (r *AISystemBindingRepo) ListByProcess(ctx context.Context, procID string) ([]*aisystem.AISystemBinding, error) {
	return r.queryBindings(ctx,
		bindingSelectColumns+` FROM ai_system_bindings WHERE process_id = $1 ORDER BY created_at DESC, id ASC`,
		procID)
}

func (r *AISystemBindingRepo) ListBySurface(ctx context.Context, surfID string) ([]*aisystem.AISystemBinding, error) {
	return r.queryBindings(ctx,
		bindingSelectColumns+` FROM ai_system_bindings WHERE surface_id = $1 ORDER BY created_at DESC, id ASC`,
		surfID)
}

func (r *AISystemBindingRepo) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM ai_system_bindings WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete ai system binding: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete ai system binding rows affected: %w", err)
	}
	if rows == 0 {
		return aisystem.ErrAISystemBindingNotFound
	}
	return nil
}

func (r *AISystemBindingRepo) queryBindings(ctx context.Context, query string, args ...any) ([]*aisystem.AISystemBinding, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*aisystem.AISystemBinding, 0)
	for rows.Next() {
		b, err := scanAIBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

const bindingSelectColumns = `
	SELECT id, ai_system_id, ai_system_version,
	       COALESCE(business_service_id, ''),
	       COALESCE(capability_id, ''),
	       COALESCE(process_id, ''),
	       COALESCE(surface_id, ''),
	       COALESCE(role, ''),
	       COALESCE(description, ''),
	       created_at,
	       COALESCE(created_by, ''),
	       ` + extRefSelectColumns

func scanAIBinding(row rowScanner) (*aisystem.AISystemBinding, error) {
	var b aisystem.AISystemBinding
	var version sql.NullInt64
	var extScan extRefScan
	dests := append([]any{
		&b.ID, &b.AISystemID, &version,
		&b.BusinessServiceID, &b.CapabilityID, &b.ProcessID, &b.SurfaceID,
		&b.Role, &b.Description,
		&b.CreatedAt, &b.CreatedBy,
	}, extScan.Dests()...)
	if err := row.Scan(dests...); err != nil {
		return nil, err
	}
	if version.Valid {
		v := int(version.Int64)
		b.AISystemVersion = &v
	}
	b.ExternalRef = extScan.ToExternalRef()
	return &b, nil
}

func mapAIBindingCreateErr(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return fmt.Errorf("create ai system binding: %w", err)
	}
	switch pqErr.Code {
	case "23505": // unique_violation
		return aisystem.ErrAISystemBindingAlreadyExists
	case "23514": // check_violation
		switch pqErr.Constraint {
		case "chk_ai_bindings_at_least_one_target":
			return aisystem.ErrBindingMissingContext
		case "chk_ai_system_bindings_ext_consistency":
			return mapExtRefError(err)
		}
	case "23503": // foreign_key_violation
		// Surface verbatim — the constraint name in pqErr.Constraint and the
		// referenced row in pqErr.Detail tell the operator which FK failed.
		return fmt.Errorf("create ai system binding: foreign key violation: %s: %w", pqErr.Constraint, err)
	}
	return fmt.Errorf("create ai system binding: %w", err)
}

var _ aisystem.BindingRepository = (*AISystemBindingRepo)(nil)
