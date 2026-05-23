package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/accept-io/midas/internal/runtimeattr"
	"github.com/accept-io/midas/internal/store/sqltx"
)

type PostgresRepository struct {
	db   sqltx.DBTX
	attr runtimeattr.Recorder
}

func NewPostgresRepository(db sqltx.DBTX) *PostgresRepository {
	return &PostgresRepository{db: db, attr: runtimeattr.NoOpRecorder{}}
}

// WithAttribution wires optional benchmark/runtime attribution. Passing nil
// restores the no-op recorder.
func (r *PostgresRepository) WithAttribution(rec runtimeattr.Recorder) *PostgresRepository {
	if r == nil {
		return r
	}
	r.attr = runtimeattr.RecorderOrNoOp(rec)
	return r
}

func (r *PostgresRepository) Append(ctx context.Context, ev *AuditEvent) error {
	return r.AppendBatch(ctx, []*AuditEvent{ev})
}

func (r *PostgresRepository) AppendBatch(ctx context.Context, events []*AuditEvent) error {
	if len(events) == 0 {
		return nil
	}
	attr := runtimeattr.RecorderOrNoOp(r.attr)

	envelopeID := ""
	for i, ev := range events {
		if ev == nil {
			return fmt.Errorf("audit: AppendBatch called with nil event at index %d", i)
		}
		if i == 0 {
			envelopeID = ev.EnvelopeID
			continue
		}
		if ev.EnvelopeID != envelopeID {
			return fmt.Errorf("audit: AppendBatch mixed envelope IDs %q and %q", envelopeID, ev.EnvelopeID)
		}
	}

	var (
		prevHash string
		maxSeq   int
	)

	tailStart := time.Now()
	err := r.db.QueryRowContext(ctx,
		`SELECT sequence_no, event_hash
		 FROM audit_events
		 WHERE envelope_id = $1
		 ORDER BY sequence_no DESC
		 LIMIT 1`,
		envelopeID,
	).Scan(&maxSeq, &prevHash)
	runtimeattr.Observe(attr, runtimeattr.StageAuditTailLookup, tailStart)
	attr.AddCount(runtimeattr.CountAuditSelect, 1)
	attr.AddCount(runtimeattr.CountSQLOperation, 1)

	if err != nil {
		if err == sql.ErrNoRows {
			maxSeq = 0
			prevHash = ""
		} else {
			return err
		}
	}

	type preparedAuditEvent struct {
		event        *AuditEvent
		payloadBytes []byte
	}
	prepared := make([]preparedAuditEvent, 0, len(events))
	nextSequence := maxSeq + 1

	for _, ev := range events {
		ev.SequenceNo = nextSequence
		ev.PrevHash = prevHash
		ev.OccurredAt = normalizeOccurredAt(ev.OccurredAt)

		hashStart := time.Now()
		hash, err := ComputeEventHash(ev)
		runtimeattr.Observe(attr, runtimeattr.StageAuditHashCompute, hashStart)
		if err != nil {
			return err
		}
		ev.setHash(hash)

		marshalStart := time.Now()
		payloadBytes, err := json.Marshal(ev.Payload)
		runtimeattr.Observe(attr, runtimeattr.StageAuditPayloadMarshal, marshalStart)
		if err != nil {
			return err
		}
		runtimeattr.ObserveValue(attr, runtimeattr.ValueAuditPayloadBytes, int64(len(payloadBytes)))
		runtimeattr.ObserveValue(attr, runtimeattr.AuditPayloadBytesByTypeValue(string(ev.EventType)), int64(len(payloadBytes)))

		prepared = append(prepared, preparedAuditEvent{event: ev, payloadBytes: payloadBytes})
		nextSequence++
		prevHash = ev.EventHash
	}

	const colsPerRow = 12
	values := make([]string, 0, len(prepared))
	args := make([]any, 0, len(prepared)*colsPerRow)
	for i, item := range prepared {
		base := i*colsPerRow + 1
		values = append(values, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base, base+1, base+2, base+3, base+4, base+5,
			base+6, base+7, base+8, base+9, base+10, base+11,
		))
		ev := item.event
		args = append(args,
			ev.ID,
			ev.EnvelopeID,
			ev.RequestSource,
			ev.RequestID,
			ev.SequenceNo,
			ev.EventType,
			nullableString(string(ev.PerformedByType)),
			nullableString(ev.PerformedByID),
			item.payloadBytes,
			nullableString(ev.PrevHash),
			ev.EventHash,
			ev.OccurredAt,
		)
	}

	insertStart := time.Now()
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO audit_events (
			id, envelope_id, request_source, request_id, sequence_no, event_type,
			performed_by_type, performed_by_id, payload_json,
			prev_hash, event_hash, occurred_at
		) VALUES `+strings.Join(values, ","),
		args...,
	)
	runtimeattr.Observe(attr, runtimeattr.StageAuditInsert, insertStart)
	attr.AddCount(runtimeattr.CountAuditInsert, 1)
	attr.AddCount(runtimeattr.CountSQLOperation, 1)
	return err
}

func (r *PostgresRepository) appendOne(ctx context.Context, ev *AuditEvent) error {
	attr := runtimeattr.RecorderOrNoOp(r.attr)
	var (
		prevHash string
		maxSeq   int
	)

	tailStart := time.Now()
	err := r.db.QueryRowContext(ctx,
		`SELECT sequence_no, event_hash
		 FROM audit_events
		 WHERE envelope_id = $1
		 ORDER BY sequence_no DESC
		 LIMIT 1`,
		ev.EnvelopeID,
	).Scan(&maxSeq, &prevHash)
	runtimeattr.Observe(attr, runtimeattr.StageAuditTailLookup, tailStart)
	attr.AddCount(runtimeattr.CountAuditSelect, 1)
	attr.AddCount(runtimeattr.CountSQLOperation, 1)

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

	hashStart := time.Now()
	hash, err := ComputeEventHash(ev)
	runtimeattr.Observe(attr, runtimeattr.StageAuditHashCompute, hashStart)
	if err != nil {
		return err
	}
	ev.setHash(hash)

	marshalStart := time.Now()
	payloadBytes, err := json.Marshal(ev.Payload)
	runtimeattr.Observe(attr, runtimeattr.StageAuditPayloadMarshal, marshalStart)
	if err != nil {
		return err
	}
	runtimeattr.ObserveValue(attr, runtimeattr.ValueAuditPayloadBytes, int64(len(payloadBytes)))
	runtimeattr.ObserveValue(attr, runtimeattr.AuditPayloadBytesByTypeValue(string(ev.EventType)), int64(len(payloadBytes)))

	// ✅ FIXED: Added request_source to the INSERT statement
	insertStart := time.Now()
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
	runtimeattr.Observe(attr, runtimeattr.StageAuditInsert, insertStart)
	attr.AddCount(runtimeattr.CountAuditInsert, 1)
	attr.AddCount(runtimeattr.CountSQLOperation, 1)
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
	// D30j cursor predicate — restrict to rows strictly past the
	// cursor tuple under the requested order direction. Both
	// secondary (sequence_no) and tertiary (id) sort keys are ASC
	// regardless of OrderDesc, so the comparison flips only on the
	// occurred_at primary key.
	if filter.Cursor != nil {
		c := filter.Cursor
		cmpPrimary := "<"
		if !filter.OrderDesc {
			cmpPrimary = ">"
		}
		// Allocate three parameter slots in order so the placeholder
		// numbering stays consistent with addPredicate's bookkeeping.
		args = append(args, c.OccurredAt.UTC())
		occurredIdx := len(args)
		args = append(args, c.SequenceNo)
		seqIdx := len(args)
		args = append(args, c.ID)
		idIdx := len(args)
		predicates = append(predicates, fmt.Sprintf(
			"(occurred_at %s $%d "+
				"OR (occurred_at = $%d AND sequence_no > $%d) "+
				"OR (occurred_at = $%d AND sequence_no = $%d AND id > $%d))",
			cmpPrimary, occurredIdx,
			occurredIdx, seqIdx,
			occurredIdx, seqIdx, idIdx,
		))
	}

	q := selectClause
	if len(predicates) > 0 {
		q += " WHERE " + joinAnd(predicates)
	}
	// ORDER BY adds `id ASC` as the deterministic tertiary tie-breaker
	// (D30j). The primary + secondary keys are preserved verbatim so
	// the existing governancecoverage detected/gap pair ordering
	// continues to behave; the tertiary only ever fires when both
	// primary and secondary collide, which is rare-to-impossible.
	if filter.OrderDesc {
		q += " ORDER BY occurred_at DESC, sequence_no ASC, id ASC"
	} else {
		q += " ORDER BY occurred_at ASC, sequence_no ASC, id ASC"
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
