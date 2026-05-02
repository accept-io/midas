package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// AISystemVersionRepo provides persistence for the ai_system_versions table.
//
// Constraint mapping (Postgres → repository sentinel):
//
//   - UNIQUE violation on (ai_system_id, version)         → ErrAISystemVersionAlreadyExists
//   - CHECK chk_ai_versions_status                        → ErrInvalidStatus
//   - CHECK chk_ai_versions_version_positive              → ErrInvalidVersion
//   - CHECK chk_ai_versions_effective_range               → ErrInvalidEffectiveRange
//   - FK violation on ai_system_id                        → ErrAISystemNotFound (wrapped)
type AISystemVersionRepo struct {
	db sqltx.DBTX
}

// NewAISystemVersionRepo constructs a repo bound to db. Returns ErrNilDB when db is nil.
func NewAISystemVersionRepo(db sqltx.DBTX) (*AISystemVersionRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &AISystemVersionRepo{db: db}, nil
}

func (r *AISystemVersionRepo) Create(ctx context.Context, ver *aisystem.AISystemVersion) error {
	frameworks := ver.ComplianceFrameworks
	if frameworks == nil {
		frameworks = []string{}
	}
	frameworksJSON, err := json.Marshal(frameworks)
	if err != nil {
		return fmt.Errorf("marshal compliance_frameworks: %w", err)
	}

	const q = `
		INSERT INTO ai_system_versions (
			ai_system_id, version, release_label, model_artifact, model_hash, endpoint,
			status, effective_from, effective_until, retired_at,
			compliance_frameworks, documentation_url,
			created_at, updated_at, created_by,
			ext_source_system, ext_source_id, ext_source_url, ext_source_version, ext_last_synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`
	args := append([]any{
		ver.AISystemID,
		ver.Version,
		nullableString(ver.ReleaseLabel),
		nullableString(ver.ModelArtifact),
		nullableString(ver.ModelHash),
		nullableString(ver.Endpoint),
		ver.Status,
		ver.EffectiveFrom,
		nullableTime(ver.EffectiveUntil),
		nullableTime(ver.RetiredAt),
		frameworksJSON,
		nullableString(ver.DocumentationURL),
		ver.CreatedAt,
		ver.UpdatedAt,
		nullableString(ver.CreatedBy),
	}, extRefInsertValues(ver.ExternalRef)...)
	_, err = r.db.ExecContext(ctx, q, args...)
	return mapAIVersionCreateErr(err)
}

func (r *AISystemVersionRepo) GetByIDAndVersion(ctx context.Context, aiSystemID string, version int) (*aisystem.AISystemVersion, error) {
	const q = `
		SELECT ai_system_id, version,
		       COALESCE(release_label, ''), COALESCE(model_artifact, ''),
		       COALESCE(model_hash, ''), COALESCE(endpoint, ''),
		       status, effective_from, effective_until, retired_at,
		       compliance_frameworks, COALESCE(documentation_url, ''),
		       created_at, updated_at, COALESCE(created_by, ''),
		       ` + extRefSelectColumns + `
		FROM ai_system_versions
		WHERE ai_system_id = $1 AND version = $2`
	row := r.db.QueryRowContext(ctx, q, aiSystemID, version)
	ver, err := scanAIVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, aisystem.ErrAISystemVersionNotFound
	}
	return ver, err
}

func (r *AISystemVersionRepo) ListBySystem(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemVersion, error) {
	const q = `
		SELECT ai_system_id, version,
		       COALESCE(release_label, ''), COALESCE(model_artifact, ''),
		       COALESCE(model_hash, ''), COALESCE(endpoint, ''),
		       status, effective_from, effective_until, retired_at,
		       compliance_frameworks, COALESCE(documentation_url, ''),
		       created_at, updated_at, COALESCE(created_by, ''),
		       ` + extRefSelectColumns + `
		FROM ai_system_versions
		WHERE ai_system_id = $1
		ORDER BY version DESC`
	rows, err := r.db.QueryContext(ctx, q, aiSystemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAIVersionRows(rows)
}

func (r *AISystemVersionRepo) GetActiveBySystem(ctx context.Context, aiSystemID string) (*aisystem.AISystemVersion, error) {
	const q = `
		SELECT ai_system_id, version,
		       COALESCE(release_label, ''), COALESCE(model_artifact, ''),
		       COALESCE(model_hash, ''), COALESCE(endpoint, ''),
		       status, effective_from, effective_until, retired_at,
		       compliance_frameworks, COALESCE(documentation_url, ''),
		       created_at, updated_at, COALESCE(created_by, ''),
		       ` + extRefSelectColumns + `
		FROM ai_system_versions
		WHERE ai_system_id = $1
		  AND status = 'active'
		ORDER BY version DESC
		LIMIT 1`
	row := r.db.QueryRowContext(ctx, q, aiSystemID)
	ver, err := scanAIVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ver, err
}

func (r *AISystemVersionRepo) Update(ctx context.Context, ver *aisystem.AISystemVersion) error {
	frameworks := ver.ComplianceFrameworks
	if frameworks == nil {
		frameworks = []string{}
	}
	frameworksJSON, err := json.Marshal(frameworks)
	if err != nil {
		return fmt.Errorf("marshal compliance_frameworks: %w", err)
	}

	const q = `
		UPDATE ai_system_versions
		   SET release_label = $3,
		       model_artifact = $4,
		       model_hash = $5,
		       endpoint = $6,
		       status = $7,
		       effective_from = $8,
		       effective_until = $9,
		       retired_at = $10,
		       compliance_frameworks = $11,
		       documentation_url = $12,
		       updated_at = $13,
		       ext_source_system = $14,
		       ext_source_id = $15,
		       ext_source_url = $16,
		       ext_source_version = $17,
		       ext_last_synced_at = $18
		 WHERE ai_system_id = $1 AND version = $2`
	args := append([]any{
		ver.AISystemID,
		ver.Version,
		nullableString(ver.ReleaseLabel),
		nullableString(ver.ModelArtifact),
		nullableString(ver.ModelHash),
		nullableString(ver.Endpoint),
		ver.Status,
		ver.EffectiveFrom,
		nullableTime(ver.EffectiveUntil),
		nullableTime(ver.RetiredAt),
		frameworksJSON,
		nullableString(ver.DocumentationURL),
		ver.UpdatedAt,
	}, extRefInsertValues(ver.ExternalRef)...)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return mapAIVersionUpdateErr(err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update ai system version rows affected: %w", err)
	}
	if rows == 0 {
		return aisystem.ErrAISystemVersionNotFound
	}
	return nil
}

func scanAIVersion(row rowScanner) (*aisystem.AISystemVersion, error) {
	var ver aisystem.AISystemVersion
	var effectiveUntil sql.NullTime
	var retiredAt sql.NullTime
	var frameworksRaw []byte
	var extScan extRefScan
	dests := append([]any{
		&ver.AISystemID, &ver.Version,
		&ver.ReleaseLabel, &ver.ModelArtifact,
		&ver.ModelHash, &ver.Endpoint,
		&ver.Status, &ver.EffectiveFrom, &effectiveUntil, &retiredAt,
		&frameworksRaw, &ver.DocumentationURL,
		&ver.CreatedAt, &ver.UpdatedAt, &ver.CreatedBy,
	}, extScan.Dests()...)
	if err := row.Scan(dests...); err != nil {
		return nil, err
	}
	ver.ExternalRef = extScan.ToExternalRef()
	if effectiveUntil.Valid {
		t := effectiveUntil.Time
		ver.EffectiveUntil = &t
	}
	if retiredAt.Valid {
		t := retiredAt.Time
		ver.RetiredAt = &t
	}
	if len(frameworksRaw) > 0 {
		if err := json.Unmarshal(frameworksRaw, &ver.ComplianceFrameworks); err != nil {
			return nil, fmt.Errorf("unmarshal compliance_frameworks: %w", err)
		}
	}
	if ver.ComplianceFrameworks == nil {
		ver.ComplianceFrameworks = []string{}
	}
	return &ver, nil
}

func scanAIVersionRows(rows *sql.Rows) ([]*aisystem.AISystemVersion, error) {
	out := make([]*aisystem.AISystemVersion, 0)
	for rows.Next() {
		ver, err := scanAIVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ver)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func mapAIVersionCreateErr(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return fmt.Errorf("create ai system version: %w", err)
	}
	switch pqErr.Code {
	case "23505": // unique_violation — composite PK (ai_system_id, version)
		return aisystem.ErrAISystemVersionAlreadyExists
	case "23514": // check_violation
		switch pqErr.Constraint {
		case "chk_ai_versions_status":
			return aisystem.ErrInvalidStatus
		case "chk_ai_versions_version_positive":
			return aisystem.ErrInvalidVersion
		case "chk_ai_versions_effective_range":
			return aisystem.ErrInvalidEffectiveRange
		case "chk_ai_system_versions_ext_consistency":
			return mapExtRefError(err)
		}
	case "23503": // foreign_key_violation — ai_system_id missing
		return fmt.Errorf("create ai system version: %w: %s", aisystem.ErrAISystemNotFound, pqErr.Detail)
	}
	return fmt.Errorf("create ai system version: %w", err)
}

func mapAIVersionUpdateErr(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return fmt.Errorf("update ai system version: %w", err)
	}
	switch pqErr.Code {
	case "23514": // check_violation
		switch pqErr.Constraint {
		case "chk_ai_versions_status":
			return aisystem.ErrInvalidStatus
		case "chk_ai_versions_effective_range":
			return aisystem.ErrInvalidEffectiveRange
		case "chk_ai_system_versions_ext_consistency":
			return mapExtRefError(err)
		}
	}
	return fmt.Errorf("update ai system version: %w", err)
}

var _ aisystem.VersionRepository = (*AISystemVersionRepo)(nil)
