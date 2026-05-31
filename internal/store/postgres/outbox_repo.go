package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/runtimeattr"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// txStarter is satisfied by *sql.DB. It is used by ClaimUnpublished to open
// an internal short-lived transaction for SELECT FOR UPDATE SKIP LOCKED.
type txStarter interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// OutboxRepo is the Postgres-backed implementation of outbox.Repository.
//
// Every write method must be called with a db instance that is bound to the
// current database transaction. The outbox row and the domain row must commit
// together; rolling back the transaction removes both.
type OutboxRepo struct {
	db   sqltx.DBTX
	attr runtimeattr.Recorder
}

// NewOutboxRepo constructs an OutboxRepo using the supplied DBTX, which may
// be a *sql.DB for out-of-transaction reads or a *sql.Tx for transactional writes.
func NewOutboxRepo(db sqltx.DBTX) (*OutboxRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &OutboxRepo{db: db, attr: runtimeattr.NoOpRecorder{}}, nil
}

// WithAttribution wires optional benchmark/runtime attribution. Passing nil
// restores the no-op recorder.
func (r *OutboxRepo) WithAttribution(rec runtimeattr.Recorder) *OutboxRepo {
	if r == nil {
		return r
	}
	r.attr = runtimeattr.RecorderOrNoOp(rec)
	return r
}

// Append inserts a single outbox event row. The row inherits the surrounding
// transaction: if the transaction is rolled back, the row is removed.
func (r *OutboxRepo) Append(ctx context.Context, ev *outbox.OutboxEvent) error {
	return r.AppendBatch(ctx, []*outbox.OutboxEvent{ev})
}

// AppendBatch inserts multiple outbox event rows with one SQL statement. The
// rows inherit the surrounding transaction exactly like Append: if the
// transaction rolls back, all inserted outbox rows are removed.
func (r *OutboxRepo) AppendBatch(ctx context.Context, events []*outbox.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}

	const colsPerRow = 9
	var (
		values = make([]string, 0, len(events))
		args   = make([]any, 0, len(events)*colsPerRow)
	)

	for i, ev := range events {
		if ev == nil {
			return fmt.Errorf("outbox: AppendBatch called with nil event at index %d", i)
		}
		payloadBytes, err := json.Marshal(ev.Payload)
		if err != nil {
			return fmt.Errorf("outbox: marshal payload: %w", err)
		}

		base := i*colsPerRow + 1
		values = append(values, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base, base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8,
		))
		args = append(args,
			ev.ID,
			string(ev.EventType),
			ev.AggregateType,
			ev.AggregateID,
			ev.Topic,
			nullableString(ev.EventKey),
			payloadBytes,
			ev.CreatedAt,
			nullableTime(ev.PublishedAt),
		)
	}

	q := `INSERT INTO outbox_events (
			id, event_type, aggregate_type, aggregate_id,
			topic, event_key, payload, created_at, published_at
		) VALUES ` + strings.Join(values, ",")

	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("outbox: insert batch: %w", err)
	}
	attr := runtimeattr.RecorderOrNoOp(r.attr)
	attr.AddCount(runtimeattr.CountOutboxInsertStatement, 1)
	attr.AddCount(runtimeattr.CountSQLOperation, 1)
	return nil
}

// ListUnpublished returns all rows where published_at IS NULL, ordered by
// created_at ascending. Dispatcher implementations call this to find events
// awaiting delivery.
func (r *OutboxRepo) ListUnpublished(ctx context.Context) ([]*outbox.OutboxEvent, error) {
	const q = `
		SELECT id, event_type, aggregate_type, aggregate_id,
		       topic, event_key, payload, created_at, published_at
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("outbox: list unpublished: %w", err)
	}
	defer rows.Close()

	return scanOutboxRows(rows)
}

// BacklogStats returns aggregate unpublished-row metrics without mutating the
// outbox. It uses the partial unpublished index on published_at IS NULL.
func (r *OutboxRepo) BacklogStats(ctx context.Context) (outbox.BacklogStats, error) {
	const q = `
		SELECT COUNT(*), MIN(created_at)
		FROM outbox_events
		WHERE published_at IS NULL`

	var (
		count     int64
		oldestSQL sql.NullTime
	)
	if err := r.db.QueryRowContext(ctx, q).Scan(&count, &oldestSQL); err != nil {
		return outbox.BacklogStats{}, fmt.Errorf("outbox: backlog stats: %w", err)
	}

	stats := outbox.BacklogStats{UnpublishedCount: count}
	if oldestSQL.Valid {
		stats.OldestUnpublishedAge = time.Since(oldestSQL.Time)
		if stats.OldestUnpublishedAge < 0 {
			stats.OldestUnpublishedAge = 0
		}
	}
	return stats, nil
}

// ClaimUnpublished returns up to limit unpublished rows using
// SELECT FOR UPDATE SKIP LOCKED, ordered by created_at ASC, id ASC.
//
// When the underlying db is a *sql.DB, ClaimUnpublished opens an internal
// short-lived transaction: it acquires row-level locks, reads the rows into
// memory, and immediately commits (releasing the locks). This prevents a
// concurrent dispatcher instance from claiming the same rows during the same
// poll window.
//
// When the underlying db is already a *sql.Tx, ClaimUnpublished runs the
// locking SELECT directly on that transaction; lock lifetime is controlled by
// the caller.
func (r *OutboxRepo) ClaimUnpublished(ctx context.Context, limit int) ([]*outbox.OutboxEvent, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("outbox: ClaimUnpublished limit must be > 0")
	}

	const q = `
		SELECT id, event_type, aggregate_type, aggregate_id,
		       topic, event_key, payload, created_at, published_at
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at ASC, id ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	// r.db satisfies txStarter, meaning it is a *sql.DB (not a *sql.Tx).
	// Open a short-lived transaction: acquire row locks, read rows, commit.
	if starter, ok := r.db.(txStarter); ok {
		tx, err := starter.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("outbox: claim begin tx: %w", err)
		}

		rows, err := tx.QueryContext(ctx, q, limit)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("outbox: claim query: %w", err)
		}

		events, err := scanOutboxRows(rows)
		rows.Close()
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("outbox: claim commit: %w", err)
		}

		return events, nil
	}

	// r.db is a *sql.Tx; run the locking SELECT on the existing transaction.
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: claim query: %w", err)
	}
	defer rows.Close()
	return scanOutboxRows(rows)
}

// MarkPublished sets published_at to the current UTC time for the event with
// the given ID. Returns an error if the row does not exist.
func (r *OutboxRepo) MarkPublished(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`UPDATE outbox_events SET published_at = $1 WHERE id = $2`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("outbox: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("outbox: event %q not found", id)
	}
	return nil
}

// MarkPublishedBatch sets published_at to the current UTC time for the given
// event IDs using a single UPDATE. It returns the number of rows affected. An
// empty ID slice is a no-op.
func (r *OutboxRepo) MarkPublishedBatch(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	now := time.Now().UTC()
	args := make([]any, 0, len(ids)+1)
	args = append(args, now)
	placeholders := make([]string, 0, len(ids))
	for i, id := range ids {
		args = append(args, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+2))
	}

	q := `UPDATE outbox_events SET published_at = $1 WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("outbox: mark published batch: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("outbox: batch rows affected: %w", err)
	}
	return n, nil
}

func scanOutboxRows(rows *sql.Rows) ([]*outbox.OutboxEvent, error) {
	var out []*outbox.OutboxEvent
	for rows.Next() {
		var (
			ev          outbox.OutboxEvent
			eventKey    sql.NullString
			payloadJSON []byte
			publishedAt sql.NullTime
		)
		if err := rows.Scan(
			&ev.ID,
			&ev.EventType,
			&ev.AggregateType,
			&ev.AggregateID,
			&ev.Topic,
			&eventKey,
			&payloadJSON,
			&ev.CreatedAt,
			&publishedAt,
		); err != nil {
			return nil, fmt.Errorf("outbox: scan row: %w", err)
		}
		if eventKey.Valid {
			ev.EventKey = eventKey.String
		}
		if publishedAt.Valid {
			t := publishedAt.Time
			ev.PublishedAt = &t
		}
		if len(payloadJSON) > 0 {
			ev.Payload = json.RawMessage(payloadJSON)
		} else {
			ev.Payload = json.RawMessage(`{}`)
		}
		out = append(out, &ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: rows error: %w", err)
	}
	return out, nil
}

var _ outbox.Repository = (*OutboxRepo)(nil)
