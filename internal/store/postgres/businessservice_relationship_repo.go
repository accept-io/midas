package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// BusinessServiceRelationshipRepo provides persistence for the
// business_service_relationships junction table.
//
// Constraint mapping (Postgres → repository sentinel):
//
//   - UNIQUE violation on uniq_bsr_triple   → ErrRelationshipDuplicateTriple
//   - UNIQUE violation on PK (id)            → ErrRelationshipDuplicateID
//   - CHECK chk_bsr_relationship_type        → ErrRelationshipInvalidType
//   - CHECK chk_bsr_no_self_reference        → ErrRelationshipSelfReference
//   - FK violation on source/target          → wrapped error naming the
//     missing referenced business service
//
// All other errors propagate unwrapped via fmt.Errorf so the caller can
// inspect with errors.Is / errors.As.
type BusinessServiceRelationshipRepo struct {
	db sqltx.DBTX
}

// NewBusinessServiceRelationshipRepo constructs a repo bound to db.
// Returns ErrNilDB when db is nil.
func NewBusinessServiceRelationshipRepo(db sqltx.DBTX) (*BusinessServiceRelationshipRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &BusinessServiceRelationshipRepo{db: db}, nil
}

func (r *BusinessServiceRelationshipRepo) Create(ctx context.Context, rel *businessservice.BusinessServiceRelationship) error {
	const q = `
		INSERT INTO business_service_relationships (
			id,
			source_business_service_id,
			target_business_service_id,
			relationship_type,
			description,
			created_at,
			created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, q,
		rel.ID,
		rel.SourceBusinessService,
		rel.TargetBusinessService,
		rel.RelationshipType,
		nullableString(rel.Description),
		rel.CreatedAt,
		nullableString(rel.CreatedBy),
	)
	return mapRelationshipCreateErr(err)
}

func (r *BusinessServiceRelationshipRepo) GetByID(ctx context.Context, id string) (*businessservice.BusinessServiceRelationship, error) {
	const q = `
		SELECT id, source_business_service_id, target_business_service_id,
		       relationship_type, description, created_at, created_by
		FROM business_service_relationships
		WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	rel, err := scanRelationship(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, businessservice.ErrRelationshipNotFound
	}
	return rel, err
}

func (r *BusinessServiceRelationshipRepo) List(ctx context.Context) ([]*businessservice.BusinessServiceRelationship, error) {
	const q = `
		SELECT id, source_business_service_id, target_business_service_id,
		       relationship_type, description, created_at, created_by
		FROM business_service_relationships
		ORDER BY created_at DESC, id ASC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRelationshipRows(rows)
}

func (r *BusinessServiceRelationshipRepo) ListBySourceBusinessService(ctx context.Context, sourceID string) ([]*businessservice.BusinessServiceRelationship, error) {
	const q = `
		SELECT id, source_business_service_id, target_business_service_id,
		       relationship_type, description, created_at, created_by
		FROM business_service_relationships
		WHERE source_business_service_id = $1
		ORDER BY created_at DESC, id ASC`
	rows, err := r.db.QueryContext(ctx, q, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRelationshipRows(rows)
}

func (r *BusinessServiceRelationshipRepo) ListByTargetBusinessService(ctx context.Context, targetID string) ([]*businessservice.BusinessServiceRelationship, error) {
	const q = `
		SELECT id, source_business_service_id, target_business_service_id,
		       relationship_type, description, created_at, created_by
		FROM business_service_relationships
		WHERE target_business_service_id = $1
		ORDER BY created_at DESC, id ASC`
	rows, err := r.db.QueryContext(ctx, q, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRelationshipRows(rows)
}

// Update mutates the description field only — the only mutable column on
// the BSR junction. Returns ErrRelationshipNotFound when no row matches.
func (r *BusinessServiceRelationshipRepo) Update(ctx context.Context, rel *businessservice.BusinessServiceRelationship) error {
	const q = `
		UPDATE business_service_relationships
		   SET description = $2
		 WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, rel.ID, nullableString(rel.Description))
	if err != nil {
		return fmt.Errorf("update business service relationship: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update business service relationship rows affected: %w", err)
	}
	if rows == 0 {
		return businessservice.ErrRelationshipNotFound
	}
	return nil
}

func (r *BusinessServiceRelationshipRepo) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM business_service_relationships WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete business service relationship: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete business service relationship rows affected: %w", err)
	}
	if rows == 0 {
		return businessservice.ErrRelationshipNotFound
	}
	return nil
}

// rowScanner is the narrow interface satisfied by both *sql.Row and *sql.Rows
// for the Scan call. Used by scanRelationship to share scanning logic between
// GetByID and the list paths.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRelationship(row rowScanner) (*businessservice.BusinessServiceRelationship, error) {
	var rel businessservice.BusinessServiceRelationship
	var description sql.NullString
	var createdBy sql.NullString
	if err := row.Scan(
		&rel.ID,
		&rel.SourceBusinessService,
		&rel.TargetBusinessService,
		&rel.RelationshipType,
		&description,
		&rel.CreatedAt,
		&createdBy,
	); err != nil {
		return nil, err
	}
	if description.Valid {
		rel.Description = description.String
	}
	if createdBy.Valid {
		rel.CreatedBy = createdBy.String
	}
	return &rel, nil
}

func scanRelationshipRows(rows *sql.Rows) ([]*businessservice.BusinessServiceRelationship, error) {
	out := make([]*businessservice.BusinessServiceRelationship, 0)
	for rows.Next() {
		rel, err := scanRelationship(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// mapRelationshipCreateErr translates Postgres constraint violations on
// INSERT into BSR repository sentinels. Other errors propagate wrapped.
func mapRelationshipCreateErr(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return fmt.Errorf("create business service relationship: %w", err)
	}
	switch pqErr.Code {
	case "23505": // unique_violation
		switch pqErr.Constraint {
		case "uniq_bsr_triple":
			return businessservice.ErrRelationshipDuplicateTriple
		case "business_service_relationships_pkey":
			return businessservice.ErrRelationshipDuplicateID
		}
	case "23514": // check_violation
		switch pqErr.Constraint {
		case "chk_bsr_relationship_type":
			return businessservice.ErrRelationshipInvalidType
		case "chk_bsr_no_self_reference":
			return businessservice.ErrRelationshipSelfReference
		}
	case "23503": // foreign_key_violation
		// Constraint name is auto-generated by Postgres; the message
		// contains the missing reference. Surface it verbatim — operators
		// can read the column name out of pq's Detail.
		return fmt.Errorf("create business service relationship: referenced business service not found: %w", err)
	}
	return fmt.Errorf("create business service relationship: %w", err)
}

var _ businessservice.RelationshipRepository = (*BusinessServiceRelationshipRepo)(nil)
