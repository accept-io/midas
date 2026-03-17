package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// ErrEnvelopeNotFound is returned by Update when no row matches the given ID.
var ErrEnvelopeNotFound = errors.New("envelope not found")

// EnvelopeRepo implements envelope.EnvelopeRepository against Postgres.
//
// Schema v2.1 Column layout (operational_envelopes):
//
//	Section 1 — Identity:   id, request_source, request_id, schema_version
//	Section 2 — Submitted:  submitted_raw (JSONB), submitted_hash (TEXT), received_at
//	Section 3 — Resolved:   resolved_json (JSONB) + denormalized authority chain columns
//	Section 4 — Evaluation: state, outcome, reason_code, explanation_json (JSONB), evaluated_at
//	Section 5 — Integrity:  integrity_json (JSONB)
//	Review:                 review_json (JSONB)
//	Lifecycle:              created_at, updated_at, closed_at
//
// Schema v2.1 denormalized authority chain:
//
//	resolved_surface_id, resolved_surface_version
//	resolved_profile_id, resolved_profile_version
//	resolved_grant_id, resolved_agent_id, resolved_subject_id
type EnvelopeRepo struct {
	db sqltx.DBTX
}

func NewEnvelopeRepo(db sqltx.DBTX) (*EnvelopeRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &EnvelopeRepo{db: db}, nil
}

// ---------------------------------------------------------------------------
// Select column list — shared by GetByID, GetByRequestID, GetByRequestScope, List
// ---------------------------------------------------------------------------

const selectCols = `
	id,
	request_source,
	request_id,
	schema_version,
	submitted_raw,
	submitted_hash,
	received_at,
	resolved_json,
	resolved_surface_id,
	resolved_surface_version,
	resolved_profile_id,
	resolved_profile_version,
	resolved_grant_id,
	resolved_agent_id,
	resolved_subject_id,
	state,
	outcome,
	reason_code,
	explanation_json,
	evaluated_at,
	integrity_json,
	review_json,
	created_at,
	updated_at,
	closed_at
`

func (r *EnvelopeRepo) GetByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	q := `SELECT ` + selectCols + ` FROM operational_envelopes WHERE id = $1`

	e, err := scanEnvelopeRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get envelope by id: %w", err)
	}
	return e, nil
}

// GetByRequestID retrieves by request_id only (legacy compatibility).
// For schema v2.1, prefer GetByRequestScope which uses (request_source, request_id).
func (r *EnvelopeRepo) GetByRequestID(ctx context.Context, requestID string) (*envelope.Envelope, error) {
	q := `SELECT ` + selectCols + ` FROM operational_envelopes
		WHERE request_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1`

	e, err := scanEnvelopeRow(r.db.QueryRowContext(ctx, q, requestID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get envelope by request id: %w", err)
	}
	return e, nil
}

// GetByRequestScope retrieves by (request_source, request_id) composite key.
// This is the preferred lookup method for schema v2.1 scoped idempotency.
func (r *EnvelopeRepo) GetByRequestScope(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
	q := `SELECT ` + selectCols + ` FROM operational_envelopes
		WHERE request_source = $1 AND request_id = $2
		LIMIT 1`

	e, err := scanEnvelopeRow(r.db.QueryRowContext(ctx, q, requestSource, requestID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get envelope by request scope: %w", err)
	}
	return e, nil
}

func (r *EnvelopeRepo) List(ctx context.Context) ([]*envelope.Envelope, error) {
	q := `SELECT ` + selectCols + ` FROM operational_envelopes ORDER BY created_at DESC, id DESC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list envelopes: %w", err)
	}
	defer rows.Close()

	var out []*envelope.Envelope
	for rows.Next() {
		e, err := scanEnvelopeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("list envelopes: scan row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list envelopes: rows error: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (r *EnvelopeRepo) Create(ctx context.Context, e *envelope.Envelope) error {
	if e == nil {
		return fmt.Errorf("create envelope: envelope is nil")
	}
	const q = `
		INSERT INTO operational_envelopes (
			id,
			request_source,
			request_id,
			schema_version,
			submitted_raw,
			submitted_hash,
			received_at,
			resolved_json,
			resolved_surface_id,
			resolved_surface_version,
			resolved_profile_id,
			resolved_profile_version,
			resolved_grant_id,
			resolved_agent_id,
			resolved_subject_id,
			state,
			outcome,
			reason_code,
			explanation_json,
			evaluated_at,
			integrity_json,
			review_json,
			created_at,
			updated_at,
			closed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
		)
	`

	cols, err := marshalEnvelopeCols(e)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, q,
		e.ID(),
		e.RequestSource(),
		e.RequestID(),
		e.Identity.SchemaVersion,
		cols.submittedRaw,
		nullableString(e.Integrity.SubmittedHash),
		e.Submitted.ReceivedAt,
		cols.resolvedJSON,
		nullableString(e.ResolvedSurfaceID),
		nullableInt(e.ResolvedSurfaceVersion),
		nullableString(e.ResolvedProfileID),
		nullableInt(e.ResolvedProfileVersion),
		nullableString(e.ResolvedGrantID),
		nullableString(e.ResolvedAgentID),
		nullableString(e.ResolvedSubjectID),
		string(e.State),
		nullableOutcome(e.Evaluation.Outcome),
		nullableReasonCode(e.Evaluation.ReasonCode),
		cols.explanationJSON,
		cols.evaluatedAt,
		cols.integrityJSON,
		cols.reviewJSON,
		e.CreatedAt,
		e.UpdatedAt,
		nullableTime(e.ClosedAt),
	)
	if err != nil {
		return fmt.Errorf("create envelope: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (r *EnvelopeRepo) Update(ctx context.Context, e *envelope.Envelope) error {
	if e == nil {
		return fmt.Errorf("update envelope: envelope is nil")
	}
	const q = `
		UPDATE operational_envelopes
		SET
			request_source       = $2,
			schema_version       = $3,
			submitted_raw        = $4,
			submitted_hash       = $5,
			received_at          = $6,
			resolved_json        = $7,
			resolved_surface_id  = $8,
			resolved_surface_version = $9,
			resolved_profile_id  = $10,
			resolved_profile_version = $11,
			resolved_grant_id    = $12,
			resolved_agent_id    = $13,
			resolved_subject_id  = $14,
			state                = $15,
			outcome              = $16,
			reason_code          = $17,
			explanation_json     = $18,
			evaluated_at         = $19,
			integrity_json       = $20,
			review_json          = $21,
			updated_at           = $22,
			closed_at            = $23
		WHERE id = $1
	`

	cols, err := marshalEnvelopeCols(e)
	if err != nil {
		return err
	}

	res, err := r.db.ExecContext(ctx, q,
		e.ID(),
		e.RequestSource(),
		e.Identity.SchemaVersion,
		cols.submittedRaw,
		nullableString(e.Integrity.SubmittedHash),
		e.Submitted.ReceivedAt,
		cols.resolvedJSON,
		nullableString(e.ResolvedSurfaceID),
		nullableInt(e.ResolvedSurfaceVersion),
		nullableString(e.ResolvedProfileID),
		nullableInt(e.ResolvedProfileVersion),
		nullableString(e.ResolvedGrantID),
		nullableString(e.ResolvedAgentID),
		nullableString(e.ResolvedSubjectID),
		string(e.State),
		nullableOutcome(e.Evaluation.Outcome),
		nullableReasonCode(e.Evaluation.ReasonCode),
		cols.explanationJSON,
		cols.evaluatedAt,
		cols.integrityJSON,
		cols.reviewJSON,
		e.UpdatedAt,
		nullableTime(e.ClosedAt),
	)
	if err != nil {
		return fmt.Errorf("update envelope: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update envelope: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: id=%s", ErrEnvelopeNotFound, e.ID())
	}
	return nil
}

// ---------------------------------------------------------------------------
// marshalEnvelopeCols — shared serialisation for Create and Update
// ---------------------------------------------------------------------------

type envelopeCols struct {
	submittedRaw    []byte
	resolvedJSON    []byte
	explanationJSON []byte
	evaluatedAt     *time.Time
	integrityJSON   []byte
	reviewJSON      []byte
}

func marshalEnvelopeCols(e *envelope.Envelope) (envelopeCols, error) {
	if e == nil {
		return envelopeCols{}, fmt.Errorf("marshal envelope cols: envelope is nil")
	}
	var cols envelopeCols
	var err error

	// Section 2: copy raw bytes to prevent caller mutation after the call.
	if len(e.Submitted.Raw) > 0 {
		cols.submittedRaw = make([]byte, len(e.Submitted.Raw))
		copy(cols.submittedRaw, e.Submitted.Raw)
	}

	// Section 3: serialise full Resolved struct.
	cols.resolvedJSON, err = json.Marshal(e.Resolved)
	if err != nil {
		return envelopeCols{}, fmt.Errorf("marshal resolved: %w", err)
	}

	// Section 4: Explanation is nil until evaluation begins.
	if e.Evaluation.Explanation != nil {
		cols.explanationJSON, err = json.Marshal(e.Evaluation.Explanation)
		if err != nil {
			return envelopeCols{}, fmt.Errorf("marshal explanation: %w", err)
		}
	}
	cols.evaluatedAt = e.Evaluation.EvaluatedAt

	// Section 5: Integrity.
	cols.integrityJSON, err = json.Marshal(e.Integrity)
	if err != nil {
		return envelopeCols{}, fmt.Errorf("marshal integrity: %w", err)
	}

	// Review: only set on escalated envelopes post-resolution.
	if e.Review != nil {
		cols.reviewJSON, err = json.Marshal(e.Review)
		if err != nil {
			return envelopeCols{}, fmt.Errorf("marshal review: %w", err)
		}
	}

	return cols, nil
}

// ---------------------------------------------------------------------------
// Scan
// ---------------------------------------------------------------------------

type envelopeScanner interface {
	Scan(dest ...any) error
}

func scanEnvelopeRow(row envelopeScanner) (*envelope.Envelope, error) {
	var (
		e                      envelope.Envelope
		schemaVersion          int
		submittedRaw           []byte
		submittedHash          sql.NullString
		resolvedJSON           []byte
		resolvedSurfaceID      sql.NullString
		resolvedSurfaceVersion sql.NullInt64
		resolvedProfileID      sql.NullString
		resolvedProfileVersion sql.NullInt64
		resolvedGrantID        sql.NullString
		resolvedAgentID        sql.NullString
		resolvedSubjectID      sql.NullString
		outcome                sql.NullString
		reasonCode             sql.NullString
		explanationJSON        []byte
		evaluatedAt            sql.NullTime
		integrityJSON          []byte
		reviewJSON             []byte
		closedAt               sql.NullTime
	)

	err := row.Scan(
		&e.Identity.ID,
		&e.Identity.RequestSource,
		&e.Identity.RequestID,
		&schemaVersion,
		&submittedRaw,
		&submittedHash,
		&e.Submitted.ReceivedAt,
		&resolvedJSON,
		&resolvedSurfaceID,
		&resolvedSurfaceVersion,
		&resolvedProfileID,
		&resolvedProfileVersion,
		&resolvedGrantID,
		&resolvedAgentID,
		&resolvedSubjectID,
		&e.State,
		&outcome,
		&reasonCode,
		&explanationJSON,
		&evaluatedAt,
		&integrityJSON,
		&reviewJSON,
		&e.CreatedAt,
		&e.UpdatedAt,
		&closedAt,
	)
	if err != nil {
		return nil, err
	}

	// Section 1
	e.Identity.SchemaVersion = schemaVersion

	// Section 2
	if len(submittedRaw) > 0 {
		e.Submitted.Raw = make([]byte, len(submittedRaw))
		copy(e.Submitted.Raw, submittedRaw)
	}

	// Section 3: Resolved JSON + denormalized authority chain
	if len(resolvedJSON) > 0 {
		if err := json.Unmarshal(resolvedJSON, &e.Resolved); err != nil {
			return nil, fmt.Errorf("unmarshal resolved: %w", err)
		}
	}
	if resolvedSurfaceID.Valid {
		e.ResolvedSurfaceID = resolvedSurfaceID.String
	}
	if resolvedSurfaceVersion.Valid {
		e.ResolvedSurfaceVersion = int(resolvedSurfaceVersion.Int64)
	}
	if resolvedProfileID.Valid {
		e.ResolvedProfileID = resolvedProfileID.String
	}
	if resolvedProfileVersion.Valid {
		e.ResolvedProfileVersion = int(resolvedProfileVersion.Int64)
	}
	if resolvedGrantID.Valid {
		e.ResolvedGrantID = resolvedGrantID.String
	}
	if resolvedAgentID.Valid {
		e.ResolvedAgentID = resolvedAgentID.String
	}
	if resolvedSubjectID.Valid {
		e.ResolvedSubjectID = resolvedSubjectID.String
	}

	// Section 4
	if outcome.Valid {
		e.Evaluation.Outcome = eval.Outcome(outcome.String)
	}
	if reasonCode.Valid {
		e.Evaluation.ReasonCode = eval.ReasonCode(reasonCode.String)
	}
	if len(explanationJSON) > 0 {
		var expl envelope.DecisionExplanation
		if err := json.Unmarshal(explanationJSON, &expl); err != nil {
			return nil, fmt.Errorf("unmarshal explanation: %w", err)
		}
		e.Evaluation.Explanation = &expl
	}
	if evaluatedAt.Valid {
		t := evaluatedAt.Time
		e.Evaluation.EvaluatedAt = &t
	}

	// Section 5: unmarshal integrity blob first, then apply the dedicated
	// submitted_hash column last so it is always the authoritative value.
	// This prevents integrity_json from overwriting a separately stored hash.
	if len(integrityJSON) > 0 {
		if err := json.Unmarshal(integrityJSON, &e.Integrity); err != nil {
			return nil, fmt.Errorf("unmarshal integrity: %w", err)
		}
	}
	if submittedHash.Valid {
		e.Integrity.SubmittedHash = submittedHash.String
	}

	// Review
	if len(reviewJSON) > 0 {
		var review envelope.EscalationReview
		if err := json.Unmarshal(reviewJSON, &review); err != nil {
			return nil, fmt.Errorf("unmarshal review: %w", err)
		}
		e.Review = &review
	}

	// Lifecycle
	if closedAt.Valid {
		t := closedAt.Time
		e.ClosedAt = &t
	}

	return &e, nil
}

// Compile-time interface check.
var _ envelope.EnvelopeRepository = (*EnvelopeRepo)(nil)
