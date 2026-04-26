package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"

	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// ErrEnvelopeNotFound is returned by Update when no row matches the given ID.
var ErrEnvelopeNotFound = errors.New("envelope not found")

// nullableBool returns SQL NULL when the snapshot is absent (signalled by
// presenceID == ""), otherwise the bool value. The presence indicator is
// needed because `false` is a meaningful value and cannot be distinguished
// from "absent" by the bool alone — we use the snapshot's own ID field as
// the presence sentinel. Used for envelope structural-snapshot columns
// where an unresolved Process or BusinessService leaves all five columns
// (id, origin, managed, replaces, status) NULL.
func nullableBool(presenceID string, value bool) any {
	if presenceID == "" {
		return nil
	}
	return value
}

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
//
// ADR-0001 service-led structural denormalization:
//
//	resolved_process_id, resolved_process_origin, resolved_process_managed,
//	resolved_process_replaces, resolved_process_status
//	resolved_business_service_id, resolved_business_service_origin,
//	resolved_business_service_managed, resolved_business_service_replaces,
//	resolved_business_service_status
//	resolved_enabling_capabilities_json (JSONB, sorted by id ascending)
//
// The Go envelope holds these only as Resolved.Structure; this repo is the
// sole place where the structural fields are decomposed into columns and
// reassembled on read.
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
	resolved_process_id,
	resolved_process_origin,
	resolved_process_managed,
	resolved_process_replaces,
	resolved_process_status,
	resolved_business_service_id,
	resolved_business_service_origin,
	resolved_business_service_managed,
	resolved_business_service_replaces,
	resolved_business_service_status,
	resolved_enabling_capabilities_json,
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

// ListByState returns all envelopes in the given lifecycle state, ordered by
// creation time descending. An empty state returns all envelopes (same as List).
func (r *EnvelopeRepo) ListByState(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
	if state == "" {
		return r.List(ctx)
	}

	q := `SELECT ` + selectCols + ` FROM operational_envelopes WHERE state = $1 ORDER BY created_at DESC, id DESC`

	rows, err := r.db.QueryContext(ctx, q, string(state))
	if err != nil {
		return nil, fmt.Errorf("list envelopes by state: %w", err)
	}
	defer rows.Close()

	var out []*envelope.Envelope
	for rows.Next() {
		e, err := scanEnvelopeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("list envelopes by state: scan row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list envelopes by state: rows error: %w", err)
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
			resolved_process_id,
			resolved_process_origin,
			resolved_process_managed,
			resolved_process_replaces,
			resolved_process_status,
			resolved_business_service_id,
			resolved_business_service_origin,
			resolved_business_service_managed,
			resolved_business_service_replaces,
			resolved_business_service_status,
			resolved_enabling_capabilities_json,
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
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25,
			$26,
			$27, $28, $29, $30, $31, $32, $33, $34, $35, $36
		)
	`

	cols, err := marshalEnvelopeCols(e)
	if err != nil {
		return err
	}

	proc := e.Resolved.Structure.Process
	bs := e.Resolved.Structure.BusinessService

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
		nullableString(proc.ID),
		nullableString(proc.Origin),
		nullableBool(proc.ID, proc.Managed),
		nullableString(proc.Replaces),
		nullableString(proc.Status),
		nullableString(bs.ID),
		nullableString(bs.Origin),
		nullableBool(bs.ID, bs.Managed),
		nullableString(bs.Replaces),
		nullableString(bs.Status),
		cols.enablingCapsJSON,
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
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return envelope.ErrEnvelopeScopeConflict
		}
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
			request_source                   = $2,
			schema_version                   = $3,
			submitted_raw                    = $4,
			submitted_hash                   = $5,
			received_at                      = $6,
			resolved_json                    = $7,
			resolved_surface_id              = $8,
			resolved_surface_version         = $9,
			resolved_profile_id              = $10,
			resolved_profile_version         = $11,
			resolved_grant_id                = $12,
			resolved_agent_id                = $13,
			resolved_subject_id              = $14,
			resolved_process_id              = $15,
			resolved_process_origin          = $16,
			resolved_process_managed         = $17,
			resolved_process_replaces        = $18,
			resolved_process_status          = $19,
			resolved_business_service_id     = $20,
			resolved_business_service_origin = $21,
			resolved_business_service_managed = $22,
			resolved_business_service_replaces = $23,
			resolved_business_service_status = $24,
			resolved_enabling_capabilities_json = $25,
			state                            = $26,
			outcome                          = $27,
			reason_code                      = $28,
			explanation_json                 = $29,
			evaluated_at                     = $30,
			integrity_json                   = $31,
			review_json                      = $32,
			updated_at                       = $33,
			closed_at                        = $34
		WHERE id = $1
	`

	cols, err := marshalEnvelopeCols(e)
	if err != nil {
		return err
	}

	proc := e.Resolved.Structure.Process
	bs := e.Resolved.Structure.BusinessService

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
		nullableString(proc.ID),
		nullableString(proc.Origin),
		nullableBool(proc.ID, proc.Managed),
		nullableString(proc.Replaces),
		nullableString(proc.Status),
		nullableString(bs.ID),
		nullableString(bs.Origin),
		nullableBool(bs.ID, bs.Managed),
		nullableString(bs.Replaces),
		nullableString(bs.Status),
		cols.enablingCapsJSON,
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
	submittedRaw          []byte
	resolvedJSON          []byte
	enablingCapsJSON      []byte
	explanationJSON       []byte
	evaluatedAt           *time.Time
	integrityJSON         []byte
	reviewJSON            []byte
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

	// Section 3 (structural): normalise and sort the capability snapshot in
	// place before marshalling either the full Resolved blob or the
	// dedicated JSONB column. Sorting in place ensures the embedded copy
	// inside resolved_json and the standalone resolved_enabling_capabilities_json
	// are byte-identical with respect to ordering. A nil slice is replaced
	// with an empty slice so JSON serialisation produces `[]` rather than
	// `null` — the empty-set must be a meaningful audit fact, not a missing
	// value (ADR-0001).
	if e.Resolved.Structure.EnablingCapabilities == nil {
		e.Resolved.Structure.EnablingCapabilities = []envelope.CapabilitySnapshot{}
	}
	sort.Slice(e.Resolved.Structure.EnablingCapabilities, func(i, j int) bool {
		return e.Resolved.Structure.EnablingCapabilities[i].ID < e.Resolved.Structure.EnablingCapabilities[j].ID
	})

	// Section 3: serialise full Resolved struct.
	cols.resolvedJSON, err = json.Marshal(e.Resolved)
	if err != nil {
		return envelopeCols{}, fmt.Errorf("marshal resolved: %w", err)
	}

	// Section 3 (structural JSONB): the enabling capability set is also
	// stored on a dedicated JSONB column for column-level visibility. The
	// schema declares NOT NULL DEFAULT '[]'; we always send a concrete
	// JSON array, never nil bytes.
	cols.enablingCapsJSON, err = json.Marshal(e.Resolved.Structure.EnablingCapabilities)
	if err != nil {
		return envelopeCols{}, fmt.Errorf("marshal enabling capabilities: %w", err)
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
		e                              envelope.Envelope
		schemaVersion                  int
		submittedRaw                   []byte
		submittedHash                  sql.NullString
		resolvedJSON                   []byte
		resolvedSurfaceID              sql.NullString
		resolvedSurfaceVersion         sql.NullInt64
		resolvedProfileID              sql.NullString
		resolvedProfileVersion         sql.NullInt64
		resolvedGrantID                sql.NullString
		resolvedAgentID                sql.NullString
		resolvedSubjectID              sql.NullString
		resolvedProcessID              sql.NullString
		resolvedProcessOrigin          sql.NullString
		resolvedProcessManaged         sql.NullBool
		resolvedProcessReplaces        sql.NullString
		resolvedProcessStatus          sql.NullString
		resolvedBusinessServiceID      sql.NullString
		resolvedBusinessServiceOrigin  sql.NullString
		resolvedBusinessServiceManaged sql.NullBool
		resolvedBusinessServiceReplaces sql.NullString
		resolvedBusinessServiceStatus  sql.NullString
		enablingCapsJSON               []byte
		outcome                        sql.NullString
		reasonCode                     sql.NullString
		explanationJSON                []byte
		evaluatedAt                    sql.NullTime
		integrityJSON                  []byte
		reviewJSON                     []byte
		closedAt                       sql.NullTime
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
		&resolvedProcessID,
		&resolvedProcessOrigin,
		&resolvedProcessManaged,
		&resolvedProcessReplaces,
		&resolvedProcessStatus,
		&resolvedBusinessServiceID,
		&resolvedBusinessServiceOrigin,
		&resolvedBusinessServiceManaged,
		&resolvedBusinessServiceReplaces,
		&resolvedBusinessServiceStatus,
		&enablingCapsJSON,
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

	// Section 3 (structural): reassemble Process and BusinessService snapshots
	// from dedicated columns. The dedicated columns are the authoritative
	// source — they are written explicitly and atomically with the envelope
	// row. Any value present in resolved_json (which was unmarshalled above)
	// is overwritten here so a partial-write or schema-evolution scenario
	// cannot produce divergent values.
	if resolvedProcessID.Valid {
		e.Resolved.Structure.Process.ID = resolvedProcessID.String
	}
	if resolvedProcessOrigin.Valid {
		e.Resolved.Structure.Process.Origin = resolvedProcessOrigin.String
	}
	if resolvedProcessManaged.Valid {
		e.Resolved.Structure.Process.Managed = resolvedProcessManaged.Bool
	}
	if resolvedProcessReplaces.Valid {
		e.Resolved.Structure.Process.Replaces = resolvedProcessReplaces.String
	}
	if resolvedProcessStatus.Valid {
		e.Resolved.Structure.Process.Status = resolvedProcessStatus.String
	}
	if resolvedBusinessServiceID.Valid {
		e.Resolved.Structure.BusinessService.ID = resolvedBusinessServiceID.String
	}
	if resolvedBusinessServiceOrigin.Valid {
		e.Resolved.Structure.BusinessService.Origin = resolvedBusinessServiceOrigin.String
	}
	if resolvedBusinessServiceManaged.Valid {
		e.Resolved.Structure.BusinessService.Managed = resolvedBusinessServiceManaged.Bool
	}
	if resolvedBusinessServiceReplaces.Valid {
		e.Resolved.Structure.BusinessService.Replaces = resolvedBusinessServiceReplaces.String
	}
	if resolvedBusinessServiceStatus.Valid {
		e.Resolved.Structure.BusinessService.Status = resolvedBusinessServiceStatus.String
	}

	// Section 3 (structural JSONB): reassemble enabling capabilities from
	// the dedicated JSONB column. The schema NOT NULL DEFAULT '[]' means
	// this column is always non-empty; a defensive guard keeps the path
	// safe if a hand-written test fixture or future migration violates that.
	if len(enablingCapsJSON) > 0 {
		var caps []envelope.CapabilitySnapshot
		if err := json.Unmarshal(enablingCapsJSON, &caps); err != nil {
			return nil, fmt.Errorf("unmarshal enabling capabilities: %w", err)
		}
		e.Resolved.Structure.EnablingCapabilities = caps
	}
	// Empty-set normalisation: ensure the in-memory representation is
	// always a non-nil slice so subsequent JSON marshalling produces `[]`,
	// never `null`.
	if e.Resolved.Structure.EnablingCapabilities == nil {
		e.Resolved.Structure.EnablingCapabilities = []envelope.CapabilitySnapshot{}
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
