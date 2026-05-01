package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"

	"github.com/accept-io/midas/internal/store/sqltx"
)

type PostgresRepository struct {
	db sqltx.DBTX
}

func NewPostgresRepository(db sqltx.DBTX) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Append(ctx context.Context, ev *AuditEvent) error {
	var (
		prevHash string
		maxSeq   int
	)

	err := r.db.QueryRowContext(ctx,
		`SELECT sequence_no, event_hash
		 FROM audit_events
		 WHERE envelope_id = $1
		 ORDER BY sequence_no DESC
		 LIMIT 1`,
		ev.EnvelopeID,
	).Scan(&maxSeq, &prevHash)

	if err != nil {
		if err == sql.ErrNoRows {
			maxSeq = 0
			prevHash = ""
		} else {
			return err
		}
	}

	ev.SequenceNo = maxSeq + 1
	ev.PrevHash = prevHash

	hash, err := ComputeEventHash(ev)
	if err != nil {
		return err
	}
	ev.setHash(hash)

	payloadBytes, err := json.Marshal(ev.Payload)
	if err != nil {
		return err
	}

	// ✅ FIXED: Added request_source to the INSERT statement
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO audit_events (
			id, envelope_id, request_source, request_id, sequence_no, event_type,
			performed_by_type, performed_by_id, payload_json,
			prev_hash, event_hash, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		ev.ID,
		ev.EnvelopeID,
		ev.RequestSource, // ✅ FIXED: Added this parameter
		ev.RequestID,
		ev.SequenceNo,
		ev.EventType,
		nullableString(string(ev.PerformedByType)),
		nullableString(ev.PerformedByID),
		payloadBytes,
		nullableString(ev.PrevHash),
		ev.EventHash,
		ev.OccurredAt,
	)
	return err
}

func (r *PostgresRepository) ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*AuditEvent, error) {
	// ✅ FIXED: Added request_source to the SELECT statement
	const q = `
		SELECT
			id, envelope_id, request_source, request_id, sequence_no, event_type,
			performed_by_type, performed_by_id, payload_json,
			prev_hash, event_hash, occurred_at
		FROM audit_events
		WHERE envelope_id = $1
		ORDER BY sequence_no ASC
	`

	rows, err := r.db.QueryContext(ctx, q, envelopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEventRows(rows)
}

func (r *PostgresRepository) ListByRequestID(ctx context.Context, requestID string) ([]*AuditEvent, error) {
	// ✅ FIXED: Added request_source to the SELECT statement
	const q = `
		SELECT
			id, envelope_id, request_source, request_id, sequence_no, event_type,
			performed_by_type, performed_by_id, payload_json,
			prev_hash, event_hash, occurred_at
		FROM audit_events
		WHERE request_id = $1
		ORDER BY envelope_id ASC, sequence_no ASC
	`

	rows, err := r.db.QueryContext(ctx, q, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEventRows(rows)
}

func scanEventRows(rows *sql.Rows) ([]*AuditEvent, error) {
	var out []*AuditEvent

	for rows.Next() {
		var (
			ev              AuditEvent
			performedByType sql.NullString
			performedByID   sql.NullString
			payloadBytes    []byte
			prevHash        sql.NullString
		)

		// ✅ FIXED: Added &ev.RequestSource to the Scan
		if err := rows.Scan(
			&ev.ID,
			&ev.EnvelopeID,
			&ev.RequestSource, // ✅ FIXED: Added this field
			&ev.RequestID,
			&ev.SequenceNo,
			&ev.EventType,
			&performedByType,
			&performedByID,
			&payloadBytes,
			&prevHash,
			&ev.EventHash,
			&ev.OccurredAt,
		); err != nil {
			return nil, err
		}

		if performedByType.Valid {
			ev.PerformedByType = EventPerformerType(performedByType.String)
		}
		if performedByID.Valid {
			ev.PerformedByID = performedByID.String
		}
		if prevHash.Valid {
			ev.PrevHash = prevHash.String
		}
		// Keep Hash in sync with EventHash after scanning from the database.
		ev.Hash = ev.EventHash

		if len(payloadBytes) > 0 {
			if err := json.Unmarshal(payloadBytes, &ev.Payload); err != nil {
				return nil, err
			}
			if ev.Payload == nil {
				ev.Payload = map[string]any{}
			}
		} else {
			ev.Payload = map[string]any{}
		}

		out = append(out, &ev)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// List implements AuditEventRepository.List for the Postgres backend.
// Builds a parameterised SELECT against audit_events. The query
// honours the existing indexes:
//
//   - event_type / event_type = ANY(...) → idx_audit_events_event_type
//   - envelope_id                        → idx_audit_events_envelope_id_seq
//   - request_source + request_id        → idx_audit_events_request_scope
//   - occurred_at range                  → idx_audit_events_occurred_at
//   - PayloadContains @>                 → idx_audit_events_payload_gin
//
// All values are passed as parameters; no SQL is built by string
// concatenation of caller-supplied values. ORDER BY occurred_at
// {DESC|ASC}, sequence_no ASC stabilises ties so detected/gap pairs
// from the same evaluation surface in chain order.
func (r *PostgresRepository) List(ctx context.Context, filter ListFilter) ([]*AuditEvent, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	const selectClause = `
		SELECT
			id, envelope_id, request_source, request_id, sequence_no, event_type,
			performed_by_type, performed_by_id, payload_json,
			prev_hash, event_hash, occurred_at
		FROM audit_events
	`

	var (
		predicates []string
		args       []any
	)
	addPredicate := func(clause string, val any) {
		args = append(args, val)
		predicates = append(predicates, fmt.Sprintf(clause, len(args)))
	}

	wantTypes := effectiveEventTypes(filter)
	switch len(wantTypes) {
	case 0:
		// No event-type filter applied.
	case 1:
		addPredicate("event_type = $%d", string(wantTypes[0]))
	default:
		strs := make([]string, len(wantTypes))
		for i, t := range wantTypes {
			strs[i] = string(t)
		}
		addPredicate("event_type = ANY($%d)", pq.Array(strs))
	}
	if filter.EnvelopeID != "" {
		addPredicate("envelope_id = $%d", filter.EnvelopeID)
	}
	if filter.RequestSource != "" {
		addPredicate("request_source = $%d", filter.RequestSource)
	}
	if filter.RequestID != "" {
		addPredicate("request_id = $%d", filter.RequestID)
	}
	if !filter.Since.IsZero() {
		addPredicate("occurred_at >= $%d", filter.Since.UTC())
	}
	if !filter.Until.IsZero() {
		addPredicate("occurred_at < $%d", filter.Until.UTC())
	}
	if len(filter.PayloadContains) > 0 {
		// JSONB containment via @>; the GIN index on
		// payload_json jsonb_path_ops handles top-level keys.
		bytes, err := json.Marshal(filter.PayloadContains)
		if err != nil {
			return nil, fmt.Errorf("audit list: marshal PayloadContains: %w", err)
		}
		addPredicate("payload_json @> $%d", string(bytes))
	}

	q := selectClause
	if len(predicates) > 0 {
		q += " WHERE " + joinAnd(predicates)
	}
	if filter.OrderDesc {
		q += " ORDER BY occurred_at DESC, sequence_no ASC"
	} else {
		q += " ORDER BY occurred_at ASC, sequence_no ASC"
	}

	args = append(args, filter.EffectiveLimit())
	q += fmt.Sprintf(" LIMIT $%d", len(args))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEventRows(rows)
}

// joinAnd joins predicate fragments with " AND " — a tiny helper that
// keeps the List query construction free of strings.Join's import-only
// pull, since the rest of this package doesn't use it.
func joinAnd(predicates []string) string {
	if len(predicates) == 0 {
		return ""
	}
	out := predicates[0]
	for i := 1; i < len(predicates); i++ {
		out += " AND " + predicates[i]
	}
	return out
}

var _ AuditEventRepository = (*PostgresRepository)(nil)
