package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/store/sqltx"
	"github.com/accept-io/midas/internal/value"
)

type GrantRepo struct {
	db sqltx.DBTX
}

func NewGrantRepo(db sqltx.DBTX) (*GrantRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	return &GrantRepo{db: db}, nil
}

// grantColumns is the canonical SELECT column list for authority_grants.
// All read methods use this list; it must match the column order in scanGrantRow.
//
// D31i appends capabilities and constraints (JSONB) to the end of the
// list. New columns are emitted after the existing ones so the column
// order remains backwards-compatible with the scanGrantRow scanner.
const grantColumns = `
	id,
	agent_id,
	profile_id,
	granted_by,
	grant_reason,
	status,
	effective_date,
	expires_at,
	revoked_at,
	revoked_by,
	revocation_reason,
	suspended_at,
	suspended_by,
	suspend_reason,
	created_at,
	updated_at,
	capabilities,
	constraints
`

func (r *GrantRepo) FindByID(ctx context.Context, id string) (*authority.AuthorityGrant, error) {
	q := `SELECT` + grantColumns + `FROM authority_grants WHERE id = $1`

	g, err := scanGrantRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return g, nil
}

// FindActiveByAgentAndProfile returns the active grant linking agentID to profileID.
// Schema v2.1: Checks status='active' AND effective_date <= now AND (expires_at IS NULL OR expires_at > now)
func (r *GrantRepo) FindActiveByAgentAndProfile(ctx context.Context, agentID, profileID string) (*authority.AuthorityGrant, error) {
	q := `SELECT` + grantColumns + `
		FROM authority_grants
		WHERE agent_id = $1
		  AND profile_id = $2
		  AND status = 'active'
		  AND effective_date <= NOW()
		  AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY effective_date DESC, created_at DESC
		LIMIT 1
	`

	g, err := scanGrantRow(r.db.QueryRowContext(ctx, q, agentID, profileID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return g, nil
}

func (r *GrantRepo) ListByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
	q := `SELECT` + grantColumns + `
		FROM authority_grants
		WHERE agent_id = $1
		ORDER BY effective_date DESC, created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, q, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*authority.AuthorityGrant

	for rows.Next() {
		g, err := scanGrantRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *GrantRepo) ListByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
	q := `SELECT` + grantColumns + `
		FROM authority_grants
		WHERE profile_id = $1
		ORDER BY effective_date DESC, created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, q, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*authority.AuthorityGrant

	for rows.Next() {
		g, err := scanGrantRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *GrantRepo) Create(ctx context.Context, g *authority.AuthorityGrant) error {
	const q = `
		INSERT INTO authority_grants (
			id,
			agent_id,
			profile_id,
			granted_by,
			grant_reason,
			status,
			effective_date,
			expires_at,
			revoked_at,
			revoked_by,
			revocation_reason,
			suspended_at,
			suspended_by,
			suspend_reason,
			created_at,
			updated_at,
			capabilities,
			constraints
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
	`

	capsJSON, err := marshalCapabilities(g.Capabilities)
	if err != nil {
		return fmt.Errorf("marshal capabilities for grant %q: %w", g.ID, err)
	}
	constraintsJSON, err := marshalConstraints(g.Constraints)
	if err != nil {
		return fmt.Errorf("marshal constraints for grant %q: %w", g.ID, err)
	}

	_, err = r.db.ExecContext(
		ctx,
		q,
		g.ID,
		g.AgentID,
		g.ProfileID,
		g.GrantedBy,
		nullableString(g.GrantReason),
		g.Status,
		g.EffectiveDate,
		nullableTime(g.ExpiresAt),
		nullableTime(g.RevokedAt),
		nullableString(g.RevokedBy),
		nullableString(g.RevokeReason),
		nullableTime(g.SuspendedAt),
		nullableString(g.SuspendedBy),
		nullableString(g.SuspendReason),
		g.CreatedAt,
		g.UpdatedAt,
		capsJSON,
		constraintsJSON,
	)
	return err
}

// Revoke marks a grant as revoked and records revocation metadata.
// Schema v2.1: Sets status='revoked', revoked_at=NOW(), revoked_by=revokedBy
func (r *GrantRepo) Revoke(ctx context.Context, id string, revokedBy string) error {
	const q = `
		UPDATE authority_grants
		SET
			status = 'revoked',
			revoked_at = NOW(),
			revoked_by = $2,
			updated_at = NOW()
		WHERE id = $1
	`

	res, err := r.db.ExecContext(ctx, q, id, revokedBy)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("grant not found: id=%s", id)
	}

	return nil
}

// Suspend temporarily disables a grant without full revocation.
// Schema v2.1: Sets status='suspended'
func (r *GrantRepo) Suspend(ctx context.Context, id string) error {
	const q = `
		UPDATE authority_grants
		SET
			status = 'suspended',
			updated_at = NOW()
		WHERE id = $1
	`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("grant not found: id=%s", id)
	}

	return nil
}

// Reactivate restores a suspended grant.
// Schema v2.1: Sets status='active' (only valid from suspended state)
func (r *GrantRepo) Reactivate(ctx context.Context, id string) error {
	const q = `
		UPDATE authority_grants
		SET
			status = 'active',
			updated_at = NOW()
		WHERE id = $1
		  AND status = 'suspended'
	`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("grant not found or not suspended: id=%s", id)
	}

	return nil
}

// Update persists all mutable fields of a grant atomically. Used by grant
// lifecycle governance (suspend, revoke, reinstate) to write actor, reason,
// and timestamp fields in a single operation.
//
// D31i: capabilities and constraints are also rewritten on every
// Update so the full authority shape round-trips through the lifecycle
// service when callers carry a freshly-loaded grant through a status
// transition.
func (r *GrantRepo) Update(ctx context.Context, g *authority.AuthorityGrant) error {
	const q = `
		UPDATE authority_grants
		SET
			status            = $2,
			granted_by        = $3,
			grant_reason      = $4,
			effective_date    = $5,
			expires_at        = $6,
			revoked_at        = $7,
			revoked_by        = $8,
			revocation_reason = $9,
			suspended_at      = $10,
			suspended_by      = $11,
			suspend_reason    = $12,
			updated_at        = $13,
			capabilities     = $14,
			constraints      = $15
		WHERE id = $1
	`

	capsJSON, err := marshalCapabilities(g.Capabilities)
	if err != nil {
		return fmt.Errorf("marshal capabilities for grant %q: %w", g.ID, err)
	}
	constraintsJSON, err := marshalConstraints(g.Constraints)
	if err != nil {
		return fmt.Errorf("marshal constraints for grant %q: %w", g.ID, err)
	}

	res, err := r.db.ExecContext(
		ctx, q,
		g.ID,
		g.Status,
		g.GrantedBy,
		nullableString(g.GrantReason),
		g.EffectiveDate,
		nullableTime(g.ExpiresAt),
		nullableTime(g.RevokedAt),
		nullableString(g.RevokedBy),
		nullableString(g.RevokeReason),
		nullableTime(g.SuspendedAt),
		nullableString(g.SuspendedBy),
		nullableString(g.SuspendReason),
		g.UpdatedAt,
		capsJSON,
		constraintsJSON,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("grant not found: id=%s", g.ID)
	}

	return nil
}

type grantScanner interface {
	Scan(dest ...any) error
}

// scanGrantRow scans the canonical column set defined by grantColumns.
// Column order must match grantColumns exactly.
func scanGrantRow(row grantScanner) (*authority.AuthorityGrant, error) {
	var (
		g                authority.AuthorityGrant
		grantReason      sql.NullString
		expiresAt        sql.NullTime
		revokedAt        sql.NullTime
		revokedBy        sql.NullString
		revocationReason sql.NullString
		suspendedAt      sql.NullTime
		suspendedBy      sql.NullString
		suspendReason    sql.NullString
		capsRaw          []byte
		constraintsRaw   []byte
	)

	err := row.Scan(
		&g.ID,
		&g.AgentID,
		&g.ProfileID,
		&g.GrantedBy,
		&grantReason,
		&g.Status,
		&g.EffectiveDate,
		&expiresAt,
		&revokedAt,
		&revokedBy,
		&revocationReason,
		&suspendedAt,
		&suspendedBy,
		&suspendReason,
		&g.CreatedAt,
		&g.UpdatedAt,
		&capsRaw,
		&constraintsRaw,
	)
	if err != nil {
		return nil, err
	}

	if grantReason.Valid {
		g.GrantReason = grantReason.String
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		g.ExpiresAt = &t
	}
	if revokedAt.Valid {
		t := revokedAt.Time
		g.RevokedAt = &t
	}
	if revokedBy.Valid {
		g.RevokedBy = revokedBy.String
	}
	if revocationReason.Valid {
		g.RevokeReason = revocationReason.String
	}
	if suspendedAt.Valid {
		t := suspendedAt.Time
		g.SuspendedAt = &t
	}
	if suspendedBy.Valid {
		g.SuspendedBy = suspendedBy.String
	}
	if suspendReason.Valid {
		g.SuspendReason = suspendReason.String
	}

	g.Capabilities, err = unmarshalCapabilities(capsRaw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal capabilities for grant %q: %w", g.ID, err)
	}
	g.Constraints, err = unmarshalConstraints(constraintsRaw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal constraints for grant %q: %w", g.ID, err)
	}

	return &g, nil
}

// ---------------------------------------------------------------------------
// D31i: capabilities + constraints JSON helpers
// ---------------------------------------------------------------------------
//
// The persistence format pins each Capability as its canonical string
// and each Constraint as a tagged-union object discriminated by "kind".
// Per-kind payloads:
//
//	confidence_threshold_min:   { "min_confidence": 0.85 }
//	consequence_threshold_max:  { "max_consequence": { "type": "monetary", "amount": 100, "currency": "USD" } }
//	                            { "max_consequence": { "type": "risk_rating", "risk_rating": "medium" } }
//	human_only:                 {}
//	ai_only:                    {}
//	time_window:                { "start_time": "2026-01-15T09:00:00Z", "end_time": "2026-01-15T17:00:00Z" }
//
// Marshal helpers always emit the empty array (`[]`) for nil/empty
// slices so the JSONB column never goes through JSON-null; the
// schema's chk_grants_*_array CHECK constraint is satisfied
// regardless of how a caller built the grant struct.

type constraintJSON struct {
	Kind           string             `json:"kind"`
	MinConfidence  *float64           `json:"min_confidence,omitempty"`
	MaxConsequence *consequenceJSON   `json:"max_consequence,omitempty"`
	StartTime      *time.Time         `json:"start_time,omitempty"`
	EndTime        *time.Time         `json:"end_time,omitempty"`
}

type consequenceJSON struct {
	Type       string  `json:"type"`
	Amount     float64 `json:"amount,omitempty"`
	Currency   string  `json:"currency,omitempty"`
	RiskRating string  `json:"risk_rating,omitempty"`
}

func marshalCapabilities(caps []authority.Capability) ([]byte, error) {
	if len(caps) == 0 {
		return []byte("[]"), nil
	}
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, string(c))
	}
	return json.Marshal(out)
}

func unmarshalCapabilities(raw []byte) ([]authority.Capability, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err != nil {
		return nil, err
	}
	if len(strs) == 0 {
		return nil, nil
	}
	out := make([]authority.Capability, 0, len(strs))
	for _, s := range strs {
		out = append(out, authority.Capability(s))
	}
	return out, nil
}

func marshalConstraints(cs []authority.Constraint) ([]byte, error) {
	if len(cs) == 0 {
		return []byte("[]"), nil
	}
	out := make([]constraintJSON, 0, len(cs))
	for _, c := range cs {
		cj := constraintJSON{Kind: string(c.Kind)}
		switch c.Kind {
		case authority.ConstraintKindConfidenceThresholdMin:
			m := c.MinConfidence
			cj.MinConfidence = &m
		case authority.ConstraintKindConsequenceThresholdMax:
			cj.MaxConsequence = &consequenceJSON{
				Type:       string(c.MaxConsequence.Type),
				Amount:     c.MaxConsequence.Amount,
				Currency:   c.MaxConsequence.Currency,
				RiskRating: string(c.MaxConsequence.RiskRating),
			}
		case authority.ConstraintKindTimeWindow:
			st := c.StartTime
			et := c.EndTime
			cj.StartTime = &st
			cj.EndTime = &et
		case authority.ConstraintKindHumanOnly, authority.ConstraintKindAIOnly:
			// no payload
		}
		out = append(out, cj)
	}
	return json.Marshal(out)
}

func unmarshalConstraints(raw []byte) ([]authority.Constraint, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var rows []constraintJSON
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]authority.Constraint, 0, len(rows))
	for _, r := range rows {
		c := authority.Constraint{Kind: authority.ConstraintKind(r.Kind)}
		if r.MinConfidence != nil {
			c.MinConfidence = *r.MinConfidence
		}
		if r.MaxConsequence != nil {
			c.MaxConsequence = authority.Consequence{
				Type:       value.ConsequenceType(r.MaxConsequence.Type),
				Amount:     r.MaxConsequence.Amount,
				Currency:   r.MaxConsequence.Currency,
				RiskRating: value.RiskRating(r.MaxConsequence.RiskRating),
			}
		}
		if r.StartTime != nil {
			c.StartTime = *r.StartTime
		}
		if r.EndTime != nil {
			c.EndTime = *r.EndTime
		}
		out = append(out, c)
	}
	return out, nil
}

var _ authority.GrantRepository = (*GrantRepo)(nil)
