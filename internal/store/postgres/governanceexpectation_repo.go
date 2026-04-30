package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// GovernanceExpectationRepo provides persistence operations for the
// governance_expectations table.
type GovernanceExpectationRepo struct {
	db sqltx.DBTX
}

// NewGovernanceExpectationRepo constructs a repo bound to db. db must be
// non-nil.
func NewGovernanceExpectationRepo(db sqltx.DBTX) (*GovernanceExpectationRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &GovernanceExpectationRepo{db: db}, nil
}

// emptyJSONObject is the canonical empty-object representation used when
// the caller passes a nil or empty ConditionPayload. It matches the
// schema's DEFAULT '{}' so reads round-trip predictably.
var emptyJSONObject = []byte(`{}`)

// payloadForInsert returns the JSONB bytes to persist for the given
// payload, normalising nil and empty inputs to "{}". Non-empty inputs
// are passed through verbatim — Postgres normalises JSONB on storage,
// so byte-for-byte identity is not preserved across the round-trip but
// semantic equivalence is.
func payloadForInsert(payload json.RawMessage) []byte {
	if len(payload) == 0 {
		return emptyJSONObject
	}
	return []byte(payload)
}

// Create inserts a new (id, version) row. Returns an error when the
// (id, version) pair already exists (PRIMARY KEY violation) or when any
// CHECK constraint rejects the row.
func (r *GovernanceExpectationRepo) Create(ctx context.Context, e *governanceexpectation.GovernanceExpectation) error {
	const q = `
		INSERT INTO governance_expectations (
			id,
			version,
			scope_kind,
			scope_id,
			required_surface_id,
			name,
			description,
			status,
			effective_date,
			effective_until,
			retired_at,
			condition_type,
			condition_payload_json,
			business_owner,
			technical_owner,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		e.ID,
		e.Version,
		string(e.ScopeKind),
		e.ScopeID,
		e.RequiredSurfaceID,
		e.Name,
		nullableString(e.Description),
		string(e.Status),
		e.EffectiveDate,
		nullableTime(e.EffectiveUntil),
		nullableTime(e.RetiredAt),
		string(e.ConditionType),
		payloadForInsert(e.ConditionPayload),
		e.BusinessOwner,
		e.TechnicalOwner,
		e.CreatedAt,
		e.UpdatedAt,
		nullableString(e.CreatedBy),
		nullableString(e.ApprovedBy),
		nullableTime(e.ApprovedAt),
	)
	return err
}

// FindByID returns the latest version of an expectation by its logical
// ID. Returns nil, nil when no expectation with that ID exists.
func (r *GovernanceExpectationRepo) FindByID(ctx context.Context, id string) (*governanceexpectation.GovernanceExpectation, error) {
	const q = expectationSelectColumns + `
		FROM governance_expectations
		WHERE id = $1
		ORDER BY version DESC
		LIMIT 1
	`

	e, err := scanExpectationRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

// FindByIDAndVersion retrieves a specific (id, version) pair. Returns
// nil, nil when the pair does not exist.
func (r *GovernanceExpectationRepo) FindByIDAndVersion(ctx context.Context, id string, version int) (*governanceexpectation.GovernanceExpectation, error) {
	const q = expectationSelectColumns + `
		FROM governance_expectations
		WHERE id = $1 AND version = $2
	`

	e, err := scanExpectationRow(r.db.QueryRowContext(ctx, q, id, version))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

// ListVersions returns every version of an expectation in version-DESC
// order (latest first). Returns an empty slice when no versions exist.
func (r *GovernanceExpectationRepo) ListVersions(ctx context.Context, id string) ([]*governanceexpectation.GovernanceExpectation, error) {
	const q = expectationSelectColumns + `
		FROM governance_expectations
		WHERE id = $1
		ORDER BY version DESC
	`

	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*governanceexpectation.GovernanceExpectation
	for rows.Next() {
		e, err := scanExpectationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Update persists the mutable lifecycle/audit fields of an existing
// (id, version) row. The mutable set is:
//
//   - status
//   - effective_until
//   - retired_at
//   - updated_at
//   - approved_by
//   - approved_at
//
// All other fields are immutable per (id, version). Returns an error
// when no row matches (id, version).
func (r *GovernanceExpectationRepo) Update(ctx context.Context, e *governanceexpectation.GovernanceExpectation) error {
	const q = `
		UPDATE governance_expectations
		SET
			status = $3,
			effective_until = $4,
			retired_at = $5,
			updated_at = $6,
			approved_by = $7,
			approved_at = $8
		WHERE id = $1
		  AND version = $2
	`

	res, err := r.db.ExecContext(
		ctx,
		q,
		e.ID,
		e.Version,
		string(e.Status),
		nullableTime(e.EffectiveUntil),
		nullableTime(e.RetiredAt),
		e.UpdatedAt,
		nullableString(e.ApprovedBy),
		nullableTime(e.ApprovedAt),
	)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("governance expectation not found: id=%s version=%d", e.ID, e.Version)
	}
	return nil
}

// expectationSelectColumns is the canonical SELECT column list for
// reads. Kept as a constant so SELECT order matches Scan order in
// scanExpectationRow.
const expectationSelectColumns = `
	SELECT
		id,
		version,
		scope_kind,
		scope_id,
		required_surface_id,
		name,
		description,
		status,
		effective_date,
		effective_until,
		retired_at,
		condition_type,
		condition_payload_json,
		business_owner,
		technical_owner,
		created_at,
		updated_at,
		created_by,
		approved_by,
		approved_at
`

// expectationScanner is satisfied by both *sql.Row and *sql.Rows so the
// scan helper can be reused for single-row and multi-row reads.
type expectationScanner interface {
	Scan(dest ...any) error
}

// scanExpectationRow scans a row from governance_expectations into a
// new GovernanceExpectation. Returns the underlying scan error
// unwrapped — callers handle sql.ErrNoRows as nil, nil per the
// FindBy* contract.
func scanExpectationRow(row expectationScanner) (*governanceexpectation.GovernanceExpectation, error) {
	var (
		e              governanceexpectation.GovernanceExpectation
		scopeKind      string
		description    sql.NullString
		status         string
		effectiveUntil sql.NullTime
		retiredAt      sql.NullTime
		conditionType  string
		payloadBytes   []byte
		createdBy      sql.NullString
		approvedBy     sql.NullString
		approvedAt     sql.NullTime
	)

	err := row.Scan(
		&e.ID,
		&e.Version,
		&scopeKind,
		&e.ScopeID,
		&e.RequiredSurfaceID,
		&e.Name,
		&description,
		&status,
		&e.EffectiveDate,
		&effectiveUntil,
		&retiredAt,
		&conditionType,
		&payloadBytes,
		&e.BusinessOwner,
		&e.TechnicalOwner,
		&e.CreatedAt,
		&e.UpdatedAt,
		&createdBy,
		&approvedBy,
		&approvedAt,
	)
	if err != nil {
		return nil, err
	}

	e.ScopeKind = governanceexpectation.ScopeKind(scopeKind)
	e.Status = governanceexpectation.ExpectationStatus(status)
	e.ConditionType = governanceexpectation.ConditionType(conditionType)

	if description.Valid {
		e.Description = description.String
	}
	if effectiveUntil.Valid {
		t := effectiveUntil.Time
		e.EffectiveUntil = &t
	}
	if retiredAt.Valid {
		t := retiredAt.Time
		e.RetiredAt = &t
	}
	if createdBy.Valid {
		e.CreatedBy = createdBy.String
	}
	if approvedBy.Valid {
		e.ApprovedBy = approvedBy.String
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		e.ApprovedAt = &t
	}

	if len(payloadBytes) > 0 {
		// Defensive copy: pq returns a slice into its own buffer that
		// could be aliased on subsequent reads. Owning the bytes
		// matches the memory repo's defensive-copy posture.
		buf := make([]byte, len(payloadBytes))
		copy(buf, payloadBytes)
		e.ConditionPayload = json.RawMessage(buf)
	}

	return &e, nil
}

// Compile-time check that *GovernanceExpectationRepo satisfies the
// domain Repository interface.
var _ governanceexpectation.Repository = (*GovernanceExpectationRepo)(nil)
