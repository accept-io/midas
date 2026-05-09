package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// FailModePolicyRepo is the Postgres implementation of
// failmode.PolicyRepository. It mirrors ProfileRepo's posture: raw SQL,
// scan helpers, JSONB rules round-tripped via encoding/json, and
// transaction-compatibility via sqltx.DBTX.
//
// D27j-impl-1a invariant: this repository is constructed and held in the
// store aggregator, but no runtime path consults it. The schema CHECK
// constraints are the integrity backstop; deeper rule validation lives in
// internal/failmode.
type FailModePolicyRepo struct {
	db sqltx.DBTX
}

func NewFailModePolicyRepo(db sqltx.DBTX) (*FailModePolicyRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &FailModePolicyRepo{db: db}, nil
}

const failModePolicyColumns = `
	id,
	version,
	name,
	description,
	status,
	effective_date,
	effective_until,
	retired_at,
	rules,
	business_owner,
	technical_owner,
	successor_policy_id,
	successor_version,
	origin,
	managed,
	replaces,
	created_at,
	updated_at,
	created_by,
	approved_by,
	approved_at
`

// FindByID returns the latest version (highest Version) of the policy by
// its logical ID. Returns nil, nil when no rows exist.
func (r *FailModePolicyRepo) FindByID(ctx context.Context, id string) (*failmode.FailModePolicy, error) {
	q := `
		SELECT ` + failModePolicyColumns + `
		FROM fail_mode_policies
		WHERE id = $1
		ORDER BY version DESC
		LIMIT 1
	`

	p, err := scanFailModePolicyRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// FindByIDAndVersion retrieves a specific (id, version) pair. Returns
// nil, nil when the pair does not exist.
func (r *FailModePolicyRepo) FindByIDAndVersion(ctx context.Context, id string, version int) (*failmode.FailModePolicy, error) {
	q := `
		SELECT ` + failModePolicyColumns + `
		FROM fail_mode_policies
		WHERE id = $1 AND version = $2
	`

	p, err := scanFailModePolicyRow(r.db.QueryRowContext(ctx, q, id, version))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// FindActiveAt resolves the active version for the logical ID at the given
// instant:
//
//   - status = 'active'
//   - effective_date <= at
//   - (effective_until IS NULL OR effective_until > at)
//
// On multiple matches (an invariant violation) the highest Version is
// returned via ORDER BY version DESC LIMIT 1.
func (r *FailModePolicyRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*failmode.FailModePolicy, error) {
	q := `
		SELECT ` + failModePolicyColumns + `
		FROM fail_mode_policies
		WHERE id = $1
		  AND status = 'active'
		  AND effective_date <= $2
		  AND (effective_until IS NULL OR effective_until > $2)
		ORDER BY version DESC
		LIMIT 1
	`

	p, err := scanFailModePolicyRow(r.db.QueryRowContext(ctx, q, id, at))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// ListVersions returns all versions of the policy ordered by version DESC.
// Returns an empty slice (not nil) when no rows exist for the id.
func (r *FailModePolicyRepo) ListVersions(ctx context.Context, id string) ([]*failmode.FailModePolicy, error) {
	q := `
		SELECT ` + failModePolicyColumns + `
		FROM fail_mode_policies
		WHERE id = $1
		ORDER BY version DESC
	`

	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*failmode.FailModePolicy{}
	for rows.Next() {
		p, err := scanFailModePolicyRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Create inserts a new policy version. The caller assigns Version. The
// schema's PRIMARY KEY (id, version) rejects duplicates with a unique-
// violation error.
func (r *FailModePolicyRepo) Create(ctx context.Context, p *failmode.FailModePolicy) error {
	rulesJSON, err := json.Marshal(serialiseRules(p.Rules))
	if err != nil {
		return err
	}

	const q = `
		INSERT INTO fail_mode_policies (
			id,
			version,
			name,
			description,
			status,
			effective_date,
			effective_until,
			retired_at,
			rules,
			business_owner,
			technical_owner,
			successor_policy_id,
			successor_version,
			origin,
			managed,
			replaces,
			created_at,
			updated_at,
			created_by,
			approved_by,
			approved_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
		)
	`

	_, err = r.db.ExecContext(
		ctx,
		q,
		p.ID,
		p.Version,
		p.Name,
		nullableString(p.Description),
		string(p.Status),
		p.EffectiveDate,
		nullableTime(p.EffectiveUntil),
		nullableTime(p.RetiredAt),
		rulesJSON,
		p.BusinessOwner,
		p.TechnicalOwner,
		nullableString(p.SuccessorPolicyID),
		nullableInt(p.SuccessorVersion),
		p.Origin,
		p.Managed,
		nullableString(p.Replaces),
		p.CreatedAt,
		p.UpdatedAt,
		nullableString(p.CreatedBy),
		nullableString(p.ApprovedBy),
		nullableTime(p.ApprovedAt),
	)
	return err
}

// Update replaces the matching (id, version) row in place. Returns a
// wrapped error when no row is updated, mirroring ProfileRepo posture.
func (r *FailModePolicyRepo) Update(ctx context.Context, p *failmode.FailModePolicy) error {
	rulesJSON, err := json.Marshal(serialiseRules(p.Rules))
	if err != nil {
		return err
	}

	const q = `
		UPDATE fail_mode_policies
		SET
			name = $3,
			description = $4,
			status = $5,
			effective_date = $6,
			effective_until = $7,
			retired_at = $8,
			rules = $9,
			business_owner = $10,
			technical_owner = $11,
			successor_policy_id = $12,
			successor_version = $13,
			origin = $14,
			managed = $15,
			replaces = $16,
			updated_at = $17,
			approved_by = $18,
			approved_at = $19
		WHERE id = $1
		  AND version = $2
	`

	res, err := r.db.ExecContext(
		ctx,
		q,
		p.ID,
		p.Version,
		p.Name,
		nullableString(p.Description),
		string(p.Status),
		p.EffectiveDate,
		nullableTime(p.EffectiveUntil),
		nullableTime(p.RetiredAt),
		rulesJSON,
		p.BusinessOwner,
		p.TechnicalOwner,
		nullableString(p.SuccessorPolicyID),
		nullableInt(p.SuccessorVersion),
		p.Origin,
		p.Managed,
		nullableString(p.Replaces),
		p.UpdatedAt,
		nullableString(p.ApprovedBy),
		nullableTime(p.ApprovedAt),
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("fail_mode_policy not found: id=%s version=%d", p.ID, p.Version)
	}
	return nil
}

// failModeRuleRow is the JSONB wire shape for a single rule. The keys are
// snake_case to match the typical apply-document convention; the inner-
// structure validation lives in internal/failmode.
type failModeRuleRow struct {
	CorrectnessClass string `json:"correctness_class"`
	PermittedMode    string `json:"permitted_mode"`
	Reason           string `json:"reason,omitempty"`
}

func serialiseRules(rs []failmode.FailModePolicyRule) []failModeRuleRow {
	out := make([]failModeRuleRow, len(rs))
	for i, r := range rs {
		out[i] = failModeRuleRow{
			CorrectnessClass: string(r.CorrectnessClass),
			PermittedMode:    string(r.PermittedMode),
			Reason:           r.Reason,
		}
	}
	return out
}

func deserialiseRules(b []byte) ([]failmode.FailModePolicyRule, error) {
	if len(b) == 0 {
		return []failmode.FailModePolicyRule{}, nil
	}
	var rows []failModeRuleRow
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	out := make([]failmode.FailModePolicyRule, len(rows))
	for i, r := range rows {
		out[i] = failmode.FailModePolicyRule{
			CorrectnessClass: failmode.CorrectnessClass(r.CorrectnessClass),
			PermittedMode:    failmode.PermittedMode(r.PermittedMode),
			Reason:           r.Reason,
		}
	}
	return out, nil
}

type failModePolicyScanner interface {
	Scan(dest ...any) error
}

func scanFailModePolicyRow(row failModePolicyScanner) (*failmode.FailModePolicy, error) {
	var (
		p                 failmode.FailModePolicy
		status            string
		description       sql.NullString
		effectiveUntil    sql.NullTime
		retiredAt         sql.NullTime
		rulesBytes        []byte
		successorPolicyID sql.NullString
		successorVersion  sql.NullInt64
		replaces          sql.NullString
		createdBy         sql.NullString
		approvedBy        sql.NullString
		approvedAt        sql.NullTime
	)

	err := row.Scan(
		&p.ID,
		&p.Version,
		&p.Name,
		&description,
		&status,
		&p.EffectiveDate,
		&effectiveUntil,
		&retiredAt,
		&rulesBytes,
		&p.BusinessOwner,
		&p.TechnicalOwner,
		&successorPolicyID,
		&successorVersion,
		&p.Origin,
		&p.Managed,
		&replaces,
		&p.CreatedAt,
		&p.UpdatedAt,
		&createdBy,
		&approvedBy,
		&approvedAt,
	)
	if err != nil {
		return nil, err
	}

	p.Status = failmode.FailModePolicyStatus(status)

	if description.Valid {
		p.Description = description.String
	}
	if effectiveUntil.Valid {
		t := effectiveUntil.Time
		p.EffectiveUntil = &t
	}
	if retiredAt.Valid {
		t := retiredAt.Time
		p.RetiredAt = &t
	}

	rules, err := deserialiseRules(rulesBytes)
	if err != nil {
		return nil, err
	}
	p.Rules = rules

	if successorPolicyID.Valid {
		p.SuccessorPolicyID = successorPolicyID.String
	}
	if successorVersion.Valid {
		p.SuccessorVersion = int(successorVersion.Int64)
	}
	if replaces.Valid {
		p.Replaces = replaces.String
	}
	if createdBy.Valid {
		p.CreatedBy = createdBy.String
	}
	if approvedBy.Valid {
		p.ApprovedBy = approvedBy.String
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		p.ApprovedAt = &t
	}

	return &p, nil
}

func scanFailModePolicyRows(rows *sql.Rows) (*failmode.FailModePolicy, error) {
	return scanFailModePolicyRow(rows)
}

var _ failmode.PolicyRepository = (*FailModePolicyRepo)(nil)
