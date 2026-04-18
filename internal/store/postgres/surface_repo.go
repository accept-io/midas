package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/store/sqltx"
	"github.com/accept-io/midas/internal/surface"
)

type SurfaceRepo struct {
	db sqltx.DBTX
}

func NewSurfaceRepo(db sqltx.DBTX) (*SurfaceRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	return &SurfaceRepo{db: db}, nil
}

// surfaceColumns is the canonical SELECT column list for decision_surfaces
// when querying a single table (no alias).
const surfaceColumns = `
	id,
	name,
	domain,
	business_owner,
	technical_owner,
	status,
	version,
	effective_date,
	created_at,
	updated_at,
	approved_by,
	approved_at,
	process_id
`

// surfaceColumnsAliased is used in CTE queries where the surfaces table is
// aliased as "s" and joined to avoid column ambiguity on "id".
const surfaceColumnsAliased = `
	s.id,
	s.name,
	s.domain,
	s.business_owner,
	s.technical_owner,
	s.status,
	s.version,
	s.effective_date,
	s.created_at,
	s.updated_at,
	s.approved_by,
	s.approved_at,
	s.process_id
`

// scanSurface scans the canonical column set into a DecisionSurface.
func scanSurface(scan func(dest ...any) error) (*surface.DecisionSurface, error) {
	var (
		s          surface.DecisionSurface
		approvedBy sql.NullString
		approvedAt sql.NullTime
		processID  sql.NullString
	)
	if err := scan(
		&s.ID,
		&s.Name,
		&s.Domain,
		&s.BusinessOwner,
		&s.TechnicalOwner,
		&s.Status,
		&s.Version,
		&s.EffectiveFrom,
		&s.CreatedAt,
		&s.UpdatedAt,
		&approvedBy,
		&approvedAt,
		&processID,
	); err != nil {
		return nil, err
	}
	if approvedBy.Valid {
		s.ApprovedBy = approvedBy.String
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		s.ApprovedAt = &t
	}
	if processID.Valid {
		s.ProcessID = processID.String
	}
	return &s, nil
}

// FindLatestByID returns the latest version (renamed from FindByID)
func (r *SurfaceRepo) FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	q := `SELECT` + surfaceColumns + `FROM decision_surfaces WHERE id = $1 ORDER BY version DESC LIMIT 1`

	s, err := scanSurface(func(dest ...any) error {
		return r.db.QueryRowContext(ctx, q, id).Scan(dest...)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

// FindByIDVersion returns a specific version
func (r *SurfaceRepo) FindByIDVersion(ctx context.Context, id string, version int) (*surface.DecisionSurface, error) {
	q := `SELECT` + surfaceColumns + `FROM decision_surfaces WHERE id = $1 AND version = $2 LIMIT 1`

	s, err := scanSurface(func(dest ...any) error {
		return r.db.QueryRowContext(ctx, q, id, version).Scan(dest...)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *SurfaceRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*surface.DecisionSurface, error) {
	q := `SELECT` + surfaceColumns + `
		FROM decision_surfaces
		WHERE id = $1
		  AND status = 'active'
		  AND effective_date <= $2
		ORDER BY effective_date DESC, version DESC
		LIMIT 1`

	s, err := scanSurface(func(dest ...any) error {
		return r.db.QueryRowContext(ctx, q, id, at).Scan(dest...)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

// ListVersions returns all versions of a surface ordered by version ascending.
func (r *SurfaceRepo) ListVersions(ctx context.Context, id string) ([]*surface.DecisionSurface, error) {
	q := `SELECT` + surfaceColumns + `FROM decision_surfaces WHERE id = $1 ORDER BY version ASC`

	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*surface.DecisionSurface
	for rows.Next() {
		s, err := scanSurface(func(dest ...any) error { return rows.Scan(dest...) })
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAll returns the latest version of each surface (renamed from List)
func (r *SurfaceRepo) ListAll(ctx context.Context) ([]*surface.DecisionSurface, error) {
	q := `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT` + surfaceColumnsAliased + `
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		ORDER BY s.id
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*surface.DecisionSurface
	for rows.Next() {
		s, err := scanSurface(func(dest ...any) error { return rows.Scan(dest...) })
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByStatus returns surfaces (latest version) with given status
func (r *SurfaceRepo) ListByStatus(ctx context.Context, status surface.SurfaceStatus) ([]*surface.DecisionSurface, error) {
	q := `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT` + surfaceColumnsAliased + `
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		WHERE s.status = $1
		ORDER BY s.id
	`

	rows, err := r.db.QueryContext(ctx, q, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*surface.DecisionSurface
	for rows.Next() {
		s, err := scanSurface(func(dest ...any) error { return rows.Scan(dest...) })
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByDomain returns surfaces (latest version) in given domain
func (r *SurfaceRepo) ListByDomain(ctx context.Context, domain string) ([]*surface.DecisionSurface, error) {
	q := `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT` + surfaceColumnsAliased + `
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		WHERE s.domain = $1
		ORDER BY s.id
	`

	rows, err := r.db.QueryContext(ctx, q, domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*surface.DecisionSurface
	for rows.Next() {
		s, err := scanSurface(func(dest ...any) error { return rows.Scan(dest...) })
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByProcessID returns surfaces (latest version) linked to the given process_id.
func (r *SurfaceRepo) ListByProcessID(ctx context.Context, processID string) ([]*surface.DecisionSurface, error) {
	q := `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT` + surfaceColumnsAliased + `
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		WHERE s.process_id = $1
		ORDER BY s.id
	`

	rows, err := r.db.QueryContext(ctx, q, processID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*surface.DecisionSurface
	for rows.Next() {
		s, err := scanSurface(func(dest ...any) error { return rows.Scan(dest...) })
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Search finds surfaces (latest version) matching criteria.
// NOTE: This is a simplified implementation that filters by domain and status.
// Full tag/taxonomy/category filtering requires additional schema columns.
func (r *SurfaceRepo) Search(ctx context.Context, criteria surface.SearchCriteria) ([]*surface.DecisionSurface, error) {
	query := `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM decision_surfaces
			GROUP BY id
		)
		SELECT` + surfaceColumnsAliased + `
		FROM decision_surfaces s
		INNER JOIN latest_versions lv ON s.id = lv.id AND s.version = lv.max_version
		WHERE 1=1
	`

	args := []interface{}{}
	argNum := 1

	if criteria.Domain != "" {
		query += fmt.Sprintf(" AND s.domain = $%d", argNum)
		args = append(args, criteria.Domain)
		argNum++
	}

	if len(criteria.Status) > 0 {
		statusStrings := make([]interface{}, len(criteria.Status))
		placeholders := ""
		for i, status := range criteria.Status {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += fmt.Sprintf("$%d", argNum)
			statusStrings[i] = string(status)
			argNum++
		}
		query += fmt.Sprintf(" AND s.status IN (%s)", placeholders)
		args = append(args, statusStrings...)
	}

	query += " ORDER BY s.id"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*surface.DecisionSurface
	for rows.Next() {
		s, err := scanSurface(func(dest ...any) error { return rows.Scan(dest...) })
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// EnsureInferred inserts the inferred decision surface (always version 1) if it does
// not exist, or validates that the existing version-1 row has compatible inferred
// semantics.
//
// Returns (true, nil) when a new row was created, (false, nil) when an existing row
// already has origin=inferred, managed=false, and the correct processID, or an error
// if a conflicting row exists (e.g. wrong origin, managed=true, or wrong process link).
//
// db must be a transaction when called from EnsureInferredStructure. The processID must
// already exist in the processes table within the same transaction before this is called.
func (r *SurfaceRepo) EnsureInferred(ctx context.Context, db sqltx.DBTX, surfaceID, processID string) (bool, error) {
	now := time.Now().UTC()
	const insert = `
		INSERT INTO decision_surfaces
			(id, version, name, domain, business_owner, technical_owner, status,
			 effective_date, created_at, updated_at, origin, managed, process_id)
		VALUES ($1, 1, $1, 'inferred', '', '', 'active', $2, $2, $2, 'inferred', false, $3)
		ON CONFLICT (id, version) DO NOTHING`
	res, err := db.ExecContext(ctx, insert, surfaceID, now, processID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	// Row already exists; validate that it has compatible inferred semantics.
	const sel = `
		SELECT origin, managed, COALESCE(process_id, '')
		FROM decision_surfaces
		WHERE id = $1 AND version = 1`
	var origin string
	var managed bool
	var existingProcID string
	if err := db.QueryRowContext(ctx, sel, surfaceID).Scan(&origin, &managed, &existingProcID); err != nil {
		return false, err
	}
	if origin != "inferred" || managed || existingProcID != processID {
		return false, fmt.Errorf(
			"decision surface %s (version 1) already exists with origin=%s managed=%v process_id=%s, cannot ensure as inferred entity linked to %s",
			surfaceID, origin, managed, existingProcID, processID,
		)
	}
	return false, nil
}

// Create inserts a new surface version.
//
// Enforces the Surface → Process invariant (I-1) at the application layer:
// process_id must be non-empty. This check fires before the DB insert so
// callers get a clear error message rather than a constraint-violation code
// from the underlying NOT NULL column.
func (r *SurfaceRepo) Create(ctx context.Context, s *surface.DecisionSurface) error {
	if s.ProcessID == "" {
		return fmt.Errorf("surface process_id must not be empty")
	}

	const q = `
		INSERT INTO decision_surfaces (
			id,
			version,
			name,
			domain,
			business_owner,
			technical_owner,
			status,
			effective_date,
			created_at,
			updated_at,
			approved_by,
			approved_at,
			process_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		s.ID,
		s.Version,
		s.Name,
		s.Domain,
		s.BusinessOwner,
		s.TechnicalOwner,
		s.Status,
		s.EffectiveFrom,
		s.CreatedAt,
		s.UpdatedAt,
		nullableString(s.ApprovedBy),
		nullableTime(s.ApprovedAt),
		s.ProcessID,
	)
	return err
}

// Update modifies mutable fields on an existing surface version.
//
// Enforces the Surface → Process invariant (I-1): process_id must be
// non-empty. Clearing process_id on an existing surface is not permitted.
func (r *SurfaceRepo) Update(ctx context.Context, s *surface.DecisionSurface) error {
	if s.ProcessID == "" {
		return fmt.Errorf("surface process_id must not be empty")
	}
	const q = `
		UPDATE decision_surfaces
		SET
			name = $3,
			domain = $4,
			business_owner = $5,
			technical_owner = $6,
			status = $7,
			effective_date = $8,
			updated_at = $9,
			approved_by = $10,
			approved_at = $11,
			process_id = $12
		WHERE id = $1
		  AND version = $2
	`

	res, err := r.db.ExecContext(
		ctx,
		q,
		s.ID,
		s.Version,
		s.Name,
		s.Domain,
		s.BusinessOwner,
		s.TechnicalOwner,
		s.Status,
		s.EffectiveFrom,
		s.UpdatedAt,
		nullableString(s.ApprovedBy),
		nullableTime(s.ApprovedAt),
		s.ProcessID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("surface not found: id=%s version=%d", s.ID, s.Version)
	}

	return nil
}

// CountByProcessID returns the number of decision surfaces attached to the given process.
func (r *SurfaceRepo) CountByProcessID(ctx context.Context, processID string) (int, error) {
	const q = `SELECT COUNT(*) FROM decision_surfaces WHERE process_id = $1`
	var n int
	err := r.db.QueryRowContext(ctx, q, processID).Scan(&n)
	return n, err
}

// MigrateProcess updates all surfaces from fromProcessID to toProcessID within the
// given transaction. Returns the number of rows updated.
func (r *SurfaceRepo) MigrateProcess(ctx context.Context, db sqltx.DBTX, fromProcessID, toProcessID string) (int64, error) {
	now := time.Now().UTC()
	const q = `UPDATE decision_surfaces SET process_id = $2, updated_at = $3 WHERE process_id = $1`
	res, err := db.ExecContext(ctx, q, fromProcessID, toProcessID, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// compile-time check
var _ surface.SurfaceRepository = (*SurfaceRepo)(nil)
