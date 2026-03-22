package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/outbox"
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
	db sqltx.DBTX
}

// NewOutboxRepo constructs an OutboxRepo using the supplied DBTX, which may
// be a *sql.DB for out-of-transaction reads or a *sql.Tx for transactional writes.
func NewOutboxRepo(db sqltx.DBTX) (*OutboxRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &OutboxRepo{db: db}, nil
}

// Append inserts a single outbox event row. The row inherits the surrounding
// transaction: if the transaction is rolled back, the row is removed.
func (r *OutboxRepo) Append(ctx context.Context, ev *outbox.OutboxEvent) error {
	if ev == nil {
		return fmt.Errorf("outbox: Append called with nil event")
	}

	payloadBytes, err := json.Marshal(ev.Payload)
	if err != nil {
		return fmt.Errorf("outbox: marshal payload: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO outbox_events (
			id, event_type, aggregate_type, aggregate_id,
			topic, event_key, payload, created_at, published_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
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
	if err != nil {
		return fmt.Errorf("outbox: insert: %w", err)
	}
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
