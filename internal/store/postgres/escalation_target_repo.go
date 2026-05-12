package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// EscalationTargetRepo is the Postgres implementation of
// escalation.Repository. Mirrors FailModePolicyRepo / ProfileRepo
// posture: raw SQL, scan helpers, transaction-compatible via
// sqltx.DBTX. The schema's chk_escalation_targets_* CHECK constraints
// are the SQL-layer integrity backstop; shape validation lives in
// internal/escalation.
type EscalationTargetRepo struct {
	db sqltx.DBTX
}

func NewEscalationTargetRepo(db sqltx.DBTX) (*EscalationTargetRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &EscalationTargetRepo{db: db}, nil
}

const escalationTargetColumns = `
	id,
	version,
	name,
	description,
	kind,
	handle,
	status,
	effective_date,
	effective_until,
	business_owner,
	technical_owner,
	created_at,
	updated_at,
	created_by,
	approved_by,
	approved_at
`

// FindByID returns the latest version (highest Version) of the target
// by its logical ID. Returns nil, nil when no rows exist.
func (r *EscalationTargetRepo) FindByID(ctx context.Context, id string) (*escalation.EscalationTarget, error) {
	q := `
		SELECT ` + escalationTargetColumns + `
		FROM escalation_targets
		WHERE id = $1
		ORDER BY version DESC
		LIMIT 1
	`
	t, err := scanEscalationTargetRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// FindByIDAndVersion retrieves a specific (id, version) pair. Returns
// nil, nil when the pair does not exist.
func (r *EscalationTargetRepo) FindByIDAndVersion(ctx context.Context, id string, version int) (*escalation.EscalationTarget, error) {
	q := `
		SELECT ` + escalationTargetColumns + `
		FROM escalation_targets
		WHERE id = $1 AND version = $2
	`
	t, err := scanEscalationTargetRow(r.db.QueryRowContext(ctx, q, id, version))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// FindActiveAt resolves the active version for the logical id at the
// given instant:
//
//   - status = 'active'
//   - effective_date <= at
//   - (effective_until IS NULL OR effective_until > at)
//
// On multiple matches the highest version wins.
func (r *EscalationTargetRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*escalation.EscalationTarget, error) {
	q := `
		SELECT ` + escalationTargetColumns + `
		FROM escalation_targets
		WHERE id = $1
		  AND status = 'active'
		  AND effective_date <= $2
		  AND (effective_until IS NULL OR effective_until > $2)
		ORDER BY version DESC
		LIMIT 1
	`
	t, err := scanEscalationTargetRow(r.db.QueryRowContext(ctx, q, id, at))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// List returns the latest version of every escalation target, sorted
// by id ascending. Backs the /v1/escalation-targets list endpoint.
// Returns an empty slice (not nil) when the table is empty.
func (r *EscalationTargetRepo) List(ctx context.Context) ([]*escalation.EscalationTarget, error) {
	q := `
		SELECT ` + escalationTargetColumns + `
		FROM escalation_targets e
		WHERE version = (
			SELECT MAX(version) FROM escalation_targets WHERE id = e.id
		)
		ORDER BY id ASC
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*escalation.EscalationTarget{}
	for rows.Next() {
		t, err := scanEscalationTargetRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListVersions returns all versions of the target ordered by version
// DESC. Returns an empty slice when the id has no rows.
func (r *EscalationTargetRepo) ListVersions(ctx context.Context, id string) ([]*escalation.EscalationTarget, error) {
	q := `
		SELECT ` + escalationTargetColumns + `
		FROM escalation_targets
		WHERE id = $1
		ORDER BY version DESC
	`
	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*escalation.EscalationTarget{}
	for rows.Next() {
		t, err := scanEscalationTargetRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Create inserts a new target version. The caller assigns Version.
// The schema's PRIMARY KEY (id, version) rejects duplicates with a
// unique-violation error.
func (r *EscalationTargetRepo) Create(ctx context.Context, t *escalation.EscalationTarget) error {
	const q = `
		INSERT INTO escalation_targets (
			id,
			version,
			name,
			description,
			kind,
			handle,
			status,
			effective_date,
			effective_until,
			business_owner,
			technical_owner,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)
	`
	_, err := r.db.ExecContext(
		ctx, q,
		t.ID,
		t.Version,
		t.Name,
		t.Description,
		string(t.Kind),
		t.Handle,
		string(t.Status),
		t.EffectiveDate,
		nullableTime(t.EffectiveUntil),
		t.BusinessOwner,
		t.TechnicalOwner,
		t.CreatedAt,
		t.UpdatedAt,
		t.CreatedBy,
		t.ApprovedBy,
		nullableTime(t.ApprovedAt),
	)
	return err
}

// Update replaces the matching (id, version) row in place. Returns a
// wrapped error when no row is updated, mirroring ProfileRepo and
// FailModePolicyRepo posture.
func (r *EscalationTargetRepo) Update(ctx context.Context, t *escalation.EscalationTarget) error {
	const q = `
		UPDATE escalation_targets
		SET
			name            = $3,
			description     = $4,
			kind            = $5,
			handle          = $6,
			status          = $7,
			effective_date  = $8,
			effective_until = $9,
			business_owner  = $10,
			technical_owner = $11,
			updated_at      = $12,
			created_by      = $13,
			approved_by     = $14,
			approved_at     = $15
		WHERE id = $1
		  AND version = $2
	`
	res, err := r.db.ExecContext(
		ctx, q,
		t.ID,
		t.Version,
		t.Name,
		t.Description,
		string(t.Kind),
		t.Handle,
		string(t.Status),
		t.EffectiveDate,
		nullableTime(t.EffectiveUntil),
		t.BusinessOwner,
		t.TechnicalOwner,
		t.UpdatedAt,
		t.CreatedBy,
		t.ApprovedBy,
		nullableTime(t.ApprovedAt),
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("escalation_target not found: id=%s version=%d", t.ID, t.Version)
	}
	return nil
}

type escalationTargetScanner interface {
	Scan(dest ...any) error
}

func scanEscalationTargetRow(row escalationTargetScanner) (*escalation.EscalationTarget, error) {
	var (
		t              escalation.EscalationTarget
		kind           string
		status         string
		effectiveUntil sql.NullTime
		approvedAt     sql.NullTime
	)
	err := row.Scan(
		&t.ID,
		&t.Version,
		&t.Name,
		&t.Description,
		&kind,
		&t.Handle,
		&status,
		&t.EffectiveDate,
		&effectiveUntil,
		&t.BusinessOwner,
		&t.TechnicalOwner,
		&t.CreatedAt,
		&t.UpdatedAt,
		&t.CreatedBy,
		&t.ApprovedBy,
		&approvedAt,
	)
	if err != nil {
		return nil, err
	}
	t.Kind = escalation.Kind(kind)
	t.Status = escalation.Status(status)
	if effectiveUntil.Valid {
		v := effectiveUntil.Time
		t.EffectiveUntil = &v
	}
	if approvedAt.Valid {
		v := approvedAt.Time
		t.ApprovedAt = &v
	}
	return &t, nil
}

func scanEscalationTargetRows(rows *sql.Rows) (*escalation.EscalationTarget, error) {
	return scanEscalationTargetRow(rows)
}

var _ escalation.Repository = (*EscalationTargetRepo)(nil)
