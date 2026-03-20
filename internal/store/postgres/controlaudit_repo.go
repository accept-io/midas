package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// ControlAuditRepo implements controlaudit.Repository against Postgres.
// All writes are INSERT-only; UPDATE and DELETE are never issued.
type ControlAuditRepo struct {
	db sqltx.DBTX
}

// NewControlAuditRepo constructs a ControlAuditRepo. db must be non-nil.
func NewControlAuditRepo(db sqltx.DBTX) (*ControlAuditRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &ControlAuditRepo{db: db}, nil
}

// Append inserts one control-plane audit record. The record is immutable after insert.
func (r *ControlAuditRepo) Append(ctx context.Context, rec *controlaudit.ControlAuditRecord) error {
	if rec == nil {
		return nil
	}

	var metaJSON []byte
	if rec.Metadata != nil {
		b, err := json.Marshal(rec.Metadata)
		if err != nil {
			return fmt.Errorf("marshal controlaudit metadata: %w", err)
		}
		metaJSON = b
	}

	const q = `
INSERT INTO controlplane_audit_events
    (id, occurred_at, actor, action, resource_kind, resource_id, resource_version, summary, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := r.db.ExecContext(ctx, q,
		rec.ID,
		rec.OccurredAt,
		rec.Actor,
		string(rec.Action),
		rec.ResourceKind,
		rec.ResourceID,
		nullableIntPtr(rec.ResourceVersion),
		rec.Summary,
		nullableBytes(metaJSON),
	)
	if err != nil {
		return fmt.Errorf("insert controlplane_audit_event: %w", err)
	}
	return nil
}

// List returns control-plane audit records newest-first, applying the filter constraints.
func (r *ControlAuditRepo) List(ctx context.Context, f controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
	limit := effectiveLimitPG(f.Limit)

	q := `
SELECT id, occurred_at, actor, action, resource_kind, resource_id, resource_version, summary, metadata
FROM controlplane_audit_events
WHERE ($1::text IS NULL OR resource_kind = $1)
  AND ($2::text IS NULL OR resource_id   = $2)
  AND ($3::text IS NULL OR actor         = $3)
  AND ($4::text IS NULL OR action        = $4)
ORDER BY occurred_at DESC
LIMIT $5`

	rows, err := r.db.QueryContext(ctx, q,
		nullableString(f.ResourceKind),
		nullableString(f.ResourceID),
		nullableString(f.Actor),
		nullableString(string(f.Action)),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query controlplane_audit_events: %w", err)
	}
	defer rows.Close()

	var out []*controlaudit.ControlAuditRecord
	for rows.Next() {
		rec, err := scanControlAuditRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan controlplane_audit_event: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate controlplane_audit_events: %w", err)
	}
	return out, nil
}

func scanControlAuditRecord(rows *sql.Rows) (*controlaudit.ControlAuditRecord, error) {
	var (
		rec        controlaudit.ControlAuditRecord
		action     string
		version    sql.NullInt64
		metaJSON   []byte
		occurredAt time.Time
	)

	if err := rows.Scan(
		&rec.ID,
		&occurredAt,
		&rec.Actor,
		&action,
		&rec.ResourceKind,
		&rec.ResourceID,
		&version,
		&rec.Summary,
		&metaJSON,
	); err != nil {
		return nil, err
	}

	rec.OccurredAt = occurredAt.UTC()
	rec.Action = controlaudit.Action(action)

	if version.Valid {
		v := int(version.Int64)
		rec.ResourceVersion = &v
	}

	if len(metaJSON) > 0 {
		var meta controlaudit.Metadata
		if err := json.Unmarshal(metaJSON, &meta); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		rec.Metadata = &meta
	}

	return &rec, nil
}

func nullableIntPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func effectiveLimitPG(requested int) int {
	if requested <= 0 {
		return controlaudit.DefaultListLimit
	}
	if requested > controlaudit.MaxListLimit {
		return controlaudit.MaxListLimit
	}
	return requested
}

var _ controlaudit.Repository = (*ControlAuditRepo)(nil)
