package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/adminaudit"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// AdminAuditRepo implements adminaudit.Repository against Postgres. Writes
// are INSERT-only; no UPDATE or DELETE is issued. Retrieval is an ORDER-BY-
// desc with simple equality filters.
type AdminAuditRepo struct {
	db sqltx.DBTX
}

func NewAdminAuditRepo(db sqltx.DBTX) (*AdminAuditRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &AdminAuditRepo{db: db}, nil
}

func (r *AdminAuditRepo) Append(ctx context.Context, rec *adminaudit.AdminAuditRecord) error {
	if rec == nil {
		return nil
	}

	var detailsJSON []byte
	if rec.Details != nil {
		b, err := json.Marshal(rec.Details)
		if err != nil {
			return fmt.Errorf("marshal adminaudit details: %w", err)
		}
		detailsJSON = b
	}

	const q = `
INSERT INTO platform_admin_audit_events
    (id, occurred_at, action, outcome, actor_type, actor_id,
     target_type, target_id, request_id, client_ip, required_permission, details)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err := r.db.ExecContext(ctx, q,
		rec.ID,
		rec.OccurredAt,
		string(rec.Action),
		string(rec.Outcome),
		string(rec.ActorType),
		rec.ActorID,
		rec.TargetType,
		rec.TargetID,
		rec.RequestID,
		rec.ClientIP,
		rec.RequiredPermission,
		nullableBytes(detailsJSON),
	)
	if err != nil {
		return fmt.Errorf("insert platform_admin_audit_event: %w", err)
	}
	return nil
}

func (r *AdminAuditRepo) List(ctx context.Context, f adminaudit.ListFilter) ([]*adminaudit.AdminAuditRecord, error) {
	limit := effectiveAdminAuditLimitPG(f.Limit)

	q := `
SELECT id, occurred_at, action, outcome, actor_type, actor_id,
       target_type, target_id, request_id, client_ip, required_permission, details
FROM platform_admin_audit_events
WHERE ($1::text IS NULL OR action      = $1)
  AND ($2::text IS NULL OR outcome     = $2)
  AND ($3::text IS NULL OR actor_id    = $3)
  AND ($4::text IS NULL OR target_type = $4)
  AND ($5::text IS NULL OR target_id   = $5)
ORDER BY occurred_at DESC
LIMIT $6`

	rows, err := r.db.QueryContext(ctx, q,
		nullableString(string(f.Action)),
		nullableString(string(f.Outcome)),
		nullableString(f.ActorID),
		nullableString(f.TargetType),
		nullableString(f.TargetID),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query platform_admin_audit_events: %w", err)
	}
	defer rows.Close()

	var out []*adminaudit.AdminAuditRecord
	for rows.Next() {
		rec, err := scanAdminAuditRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan platform_admin_audit_event: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate platform_admin_audit_events: %w", err)
	}
	return out, nil
}

func scanAdminAuditRecord(rows *sql.Rows) (*adminaudit.AdminAuditRecord, error) {
	var (
		rec         adminaudit.AdminAuditRecord
		action      string
		outcome     string
		actorType   string
		occurredAt  time.Time
		detailsJSON []byte
	)

	if err := rows.Scan(
		&rec.ID,
		&occurredAt,
		&action,
		&outcome,
		&actorType,
		&rec.ActorID,
		&rec.TargetType,
		&rec.TargetID,
		&rec.RequestID,
		&rec.ClientIP,
		&rec.RequiredPermission,
		&detailsJSON,
	); err != nil {
		return nil, err
	}

	rec.OccurredAt = occurredAt.UTC()
	rec.Action = adminaudit.Action(action)
	rec.Outcome = adminaudit.Outcome(outcome)
	rec.ActorType = adminaudit.ActorType(actorType)

	if len(detailsJSON) > 0 {
		var d adminaudit.Details
		if err := json.Unmarshal(detailsJSON, &d); err != nil {
			return nil, fmt.Errorf("unmarshal admin-audit details: %w", err)
		}
		rec.Details = &d
	}

	return &rec, nil
}

func effectiveAdminAuditLimitPG(requested int) int {
	if requested <= 0 {
		return adminaudit.DefaultListLimit
	}
	if requested > adminaudit.MaxListLimit {
		return adminaudit.MaxListLimit
	}
	return requested
}

var _ adminaudit.Repository = (*AdminAuditRepo)(nil)
