package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// DriftDefinitionRepo is the Postgres implementation of
// drift.DriftDefinitionRepository. Mirrors FailModePolicyRepo's posture: raw
// SQL via sqltx.DBTX, scan helpers, transactional Create that writes child
// metric rows, Update that mutates only the parent's lifecycle / approval /
// successor fields (Drift-1b decision: child metric rows are immutable on
// Update; metric changes require a new revision via Create).
type DriftDefinitionRepo struct {
	db sqltx.DBTX
}

func NewDriftDefinitionRepo(db sqltx.DBTX) (*DriftDefinitionRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &DriftDefinitionRepo{db: db}, nil
}

const driftDefinitionColumns = `
	id,
	version,
	name,
	description,
	status,
	effective_date,
	effective_until,
	retired_at,
	business_owner,
	technical_owner,
	target_entity_kind,
	target_entity_id,
	origin,
	managed,
	replaces,
	successor_definition_id,
	successor_version,
	created_at,
	updated_at,
	created_by,
	approved_by,
	approved_at
`

const driftMetricDefinitionColumns = `
	metric_id,
	drift_type,
	baseline_strategy,
	baseline_window_seconds,
	window_seconds,
	cadence,
	warning_threshold,
	breached_threshold,
	threshold_direction,
	governance_expectation_ref,
	governance_expectation_ver,
	description
`

// FindByID returns the latest version (highest Version) of the definition
// by its logical ID. Returns nil, nil when no rows exist.
func (r *DriftDefinitionRepo) FindByID(ctx context.Context, id string) (*drift.DriftDefinition, error) {
	q := `
		SELECT ` + driftDefinitionColumns + `
		FROM drift_definitions
		WHERE id = $1
		ORDER BY version DESC
		LIMIT 1
	`
	d, err := scanDriftDefinitionRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := r.attachMetrics(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// FindByIDAndVersion retrieves a specific (id, version) pair. Returns
// nil, nil when the pair does not exist.
func (r *DriftDefinitionRepo) FindByIDAndVersion(ctx context.Context, id string, version int) (*drift.DriftDefinition, error) {
	q := `
		SELECT ` + driftDefinitionColumns + `
		FROM drift_definitions
		WHERE id = $1 AND version = $2
	`
	d, err := scanDriftDefinitionRow(r.db.QueryRowContext(ctx, q, id, version))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := r.attachMetrics(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// FindActiveAt resolves the active version for the logical ID at the given
// instant. Status='active', effective_date <= at, (effective_until IS NULL
// OR effective_until > at). Highest Version wins on multiple matches.
func (r *DriftDefinitionRepo) FindActiveAt(ctx context.Context, id string, at time.Time) (*drift.DriftDefinition, error) {
	q := `
		SELECT ` + driftDefinitionColumns + `
		FROM drift_definitions
		WHERE id = $1
		  AND status = 'active'
		  AND effective_date <= $2
		  AND (effective_until IS NULL OR effective_until > $2)
		ORDER BY version DESC
		LIMIT 1
	`
	d, err := scanDriftDefinitionRow(r.db.QueryRowContext(ctx, q, id, at))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := r.attachMetrics(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ListVersions returns every version of the definition in version DESC
// order. Returns an empty slice (not nil) when the id has no rows.
func (r *DriftDefinitionRepo) ListVersions(ctx context.Context, id string) ([]*drift.DriftDefinition, error) {
	q := `
		SELECT ` + driftDefinitionColumns + `
		FROM drift_definitions
		WHERE id = $1
		ORDER BY version DESC
	`
	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*drift.DriftDefinition{}
	for rows.Next() {
		d, err := scanDriftDefinitionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, d := range out {
		if err := r.attachMetrics(ctx, d); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ListByTarget returns every definition revision for the target entity.
// The lookup is read-only and uses the existing target_entity index.
func (r *DriftDefinitionRepo) ListByTarget(
	ctx context.Context,
	kind drift.TargetEntityKind,
	entityID string,
) ([]*drift.DriftDefinition, error) {
	q := `
		SELECT ` + driftDefinitionColumns + `
		FROM drift_definitions
		WHERE target_entity_kind = $1 AND target_entity_id = $2
		ORDER BY id ASC, version DESC
	`
	rows, err := r.db.QueryContext(ctx, q, string(kind), entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*drift.DriftDefinition{}
	for rows.Next() {
		d, err := scanDriftDefinitionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, d := range out {
		if err := r.attachMetrics(ctx, d); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Create inserts a new definition revision and all of its child metric
// rows. Operates inside the caller's transaction when DBTX is *sql.Tx;
// otherwise wraps a local transaction so the parent and children are
// written atomically.
func (r *DriftDefinitionRepo) Create(ctx context.Context, d *drift.DriftDefinition) error {
	tx, commit, rollback, err := beginIfNeeded(ctx, r.db)
	if err != nil {
		return err
	}
	defer rollback()

	const q = `
		INSERT INTO drift_definitions (
			id, version,
			name, description,
			status, effective_date, effective_until, retired_at,
			business_owner, technical_owner,
			target_entity_kind, target_entity_id,
			origin, managed, replaces,
			successor_definition_id, successor_version,
			created_at, updated_at, created_by,
			approved_by, approved_at
		) VALUES (
			$1, $2,
			$3, $4,
			$5, $6, $7, $8,
			$9, $10,
			$11, $12,
			$13, $14, $15,
			$16, $17,
			$18, $19, $20,
			$21, $22
		)
	`
	if _, err := tx.ExecContext(
		ctx, q,
		d.ID, d.Version,
		d.Name, nullableString(d.Description),
		string(d.Status), d.EffectiveDate, nullableTime(d.EffectiveUntil), nullableTime(d.RetiredAt),
		d.BusinessOwner, d.TechnicalOwner,
		string(d.TargetEntityKind), d.TargetEntityID,
		string(d.Origin), d.Managed, nullableString(d.Replaces),
		nullableString(d.SuccessorDefinitionID), nullableInt(d.SuccessorVersion),
		d.CreatedAt, d.UpdatedAt, nullableString(d.CreatedBy),
		nullableString(d.ApprovedBy), nullableTime(d.ApprovedAt),
	); err != nil {
		return err
	}

	for _, m := range d.Metrics {
		if err := insertDriftMetric(ctx, tx, d.ID, d.Version, m); err != nil {
			return err
		}
	}

	return commit()
}

// Update replaces the matching (id, version) parent row's mutable
// lifecycle / approval / successor fields. Per Drift-1b, child metric
// rows are immutable on Update; metric changes require a new revision
// via Create. Returns a wrapped not-found error when the parent row
// does not exist (mirrors FailModePolicyRepo posture).
func (r *DriftDefinitionRepo) Update(ctx context.Context, d *drift.DriftDefinition) error {
	const q = `
		UPDATE drift_definitions
		SET
			name = $3,
			description = $4,
			status = $5,
			effective_date = $6,
			effective_until = $7,
			retired_at = $8,
			business_owner = $9,
			technical_owner = $10,
			origin = $11,
			managed = $12,
			replaces = $13,
			successor_definition_id = $14,
			successor_version = $15,
			updated_at = $16,
			approved_by = $17,
			approved_at = $18
		WHERE id = $1
		  AND version = $2
	`
	res, err := r.db.ExecContext(
		ctx, q,
		d.ID, d.Version,
		d.Name, nullableString(d.Description),
		string(d.Status), d.EffectiveDate, nullableTime(d.EffectiveUntil), nullableTime(d.RetiredAt),
		d.BusinessOwner, d.TechnicalOwner,
		string(d.Origin), d.Managed, nullableString(d.Replaces),
		nullableString(d.SuccessorDefinitionID), nullableInt(d.SuccessorVersion),
		d.UpdatedAt, nullableString(d.ApprovedBy), nullableTime(d.ApprovedAt),
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("drift_definition not found: id=%s version=%d", d.ID, d.Version)
	}
	return nil
}

// attachMetrics loads child metric rows for d in a stable order and
// assigns them to d.Metrics.
func (r *DriftDefinitionRepo) attachMetrics(ctx context.Context, d *drift.DriftDefinition) error {
	q := `
		SELECT ` + driftMetricDefinitionColumns + `
		FROM drift_metric_definitions
		WHERE definition_id = $1 AND definition_version = $2
		ORDER BY metric_id ASC
	`
	rows, err := r.db.QueryContext(ctx, q, d.ID, d.Version)
	if err != nil {
		return err
	}
	defer rows.Close()

	metrics := []drift.DriftMetricDefinition{}
	for rows.Next() {
		m, err := scanDriftMetricDefinitionRow(rows)
		if err != nil {
			return err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// Defensive: stable order regardless of scan order.
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].MetricID < metrics[j].MetricID })
	d.Metrics = metrics
	return nil
}

func insertDriftMetric(ctx context.Context, tx sqltx.DBTX, defID string, defVer int, m drift.DriftMetricDefinition) error {
	const q = `
		INSERT INTO drift_metric_definitions (
			definition_id, definition_version, metric_id,
			drift_type, baseline_strategy,
			baseline_window_seconds, window_seconds,
			cadence,
			warning_threshold, breached_threshold, threshold_direction,
			governance_expectation_ref, governance_expectation_ver,
			description
		) VALUES (
			$1, $2, $3,
			$4, $5,
			$6, $7,
			$8,
			$9, $10, $11,
			$12, $13,
			$14
		)
	`
	_, err := tx.ExecContext(
		ctx, q,
		defID, defVer, m.MetricID,
		string(m.DriftType), string(m.BaselineStrategy),
		m.BaselineWindowSeconds, m.WindowSeconds,
		string(m.Cadence),
		m.WarningThreshold, m.BreachedThreshold, string(m.ThresholdDirection),
		nullableString(m.GovernanceExpectationRef), m.GovernanceExpectationVer,
		nullableString(m.Description),
	)
	return err
}

type driftScanner interface {
	Scan(dest ...any) error
}

func scanDriftDefinitionRow(row driftScanner) (*drift.DriftDefinition, error) {
	var (
		d                     drift.DriftDefinition
		description           sql.NullString
		effectiveUntil        sql.NullTime
		retiredAt             sql.NullTime
		replaces              sql.NullString
		successorDefinitionID sql.NullString
		successorVersion      sql.NullInt64
		createdBy             sql.NullString
		approvedBy            sql.NullString
		approvedAt            sql.NullTime
		statusStr             string
		targetEntityKind      string
		origin                string
	)
	err := row.Scan(
		&d.ID, &d.Version,
		&d.Name, &description,
		&statusStr, &d.EffectiveDate, &effectiveUntil, &retiredAt,
		&d.BusinessOwner, &d.TechnicalOwner,
		&targetEntityKind, &d.TargetEntityID,
		&origin, &d.Managed, &replaces,
		&successorDefinitionID, &successorVersion,
		&d.CreatedAt, &d.UpdatedAt, &createdBy,
		&approvedBy, &approvedAt,
	)
	if err != nil {
		return nil, err
	}
	d.Status = drift.DriftDefinitionStatus(statusStr)
	d.TargetEntityKind = drift.TargetEntityKind(targetEntityKind)
	d.Origin = drift.DriftOrigin(origin)
	if description.Valid {
		d.Description = description.String
	}
	if effectiveUntil.Valid {
		t := effectiveUntil.Time
		d.EffectiveUntil = &t
	}
	if retiredAt.Valid {
		t := retiredAt.Time
		d.RetiredAt = &t
	}
	if replaces.Valid {
		d.Replaces = replaces.String
	}
	if successorDefinitionID.Valid {
		d.SuccessorDefinitionID = successorDefinitionID.String
	}
	if successorVersion.Valid {
		d.SuccessorVersion = int(successorVersion.Int64)
	}
	if createdBy.Valid {
		d.CreatedBy = createdBy.String
	}
	if approvedBy.Valid {
		d.ApprovedBy = approvedBy.String
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		d.ApprovedAt = &t
	}
	return &d, nil
}

func scanDriftDefinitionRows(rows *sql.Rows) (*drift.DriftDefinition, error) {
	return scanDriftDefinitionRow(rows)
}

func scanDriftMetricDefinitionRow(row driftScanner) (drift.DriftMetricDefinition, error) {
	var (
		m                drift.DriftMetricDefinition
		driftType        string
		baselineStrategy string
		cadence          string
		thresholdDir     string
		govExpRef        sql.NullString
		description      sql.NullString
	)
	err := row.Scan(
		&m.MetricID,
		&driftType, &baselineStrategy,
		&m.BaselineWindowSeconds, &m.WindowSeconds,
		&cadence,
		&m.WarningThreshold, &m.BreachedThreshold, &thresholdDir,
		&govExpRef, &m.GovernanceExpectationVer,
		&description,
	)
	if err != nil {
		return drift.DriftMetricDefinition{}, err
	}
	m.DriftType = drift.DriftType(driftType)
	m.BaselineStrategy = drift.BaselineStrategy(baselineStrategy)
	m.Cadence = drift.Cadence(cadence)
	m.ThresholdDirection = drift.ThresholdDirection(thresholdDir)
	if govExpRef.Valid {
		m.GovernanceExpectationRef = govExpRef.String
	}
	if description.Valid {
		m.Description = description.String
	}
	return m, nil
}

// beginIfNeeded yields a transactional DBTX. When the supplied DBTX is
// already a *sql.Tx (caller is inside Store.WithTx), it returns the same
// tx and a no-op commit/rollback. When the caller is operating directly
// against *sql.DB, it opens a fresh transaction so multi-row writes are
// atomic.
func beginIfNeeded(ctx context.Context, db sqltx.DBTX) (sqltx.DBTX, func() error, func(), error) {
	switch v := db.(type) {
	case *sql.Tx:
		return v, func() error { return nil }, func() {}, nil
	case *sql.DB:
		tx, err := v.BeginTx(ctx, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		commit := func() error { return tx.Commit() }
		rollback := func() {
			// Best-effort; ignore "tx done" after commit.
			_ = tx.Rollback()
		}
		return tx, commit, rollback, nil
	default:
		// Unknown DBTX flavour — treat as caller-managed; commit/rollback no-ops.
		return v, func() error { return nil }, func() {}, nil
	}
}

var _ drift.DriftDefinitionRepository = (*DriftDefinitionRepo)(nil)
