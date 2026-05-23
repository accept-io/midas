package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/runtimeattr"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/sqltx"
)

const (
	txStageBegin    = "begin"
	txStageCallback = "callback"
	txStageCommit   = "commit"
	txStageRollback = "rollback"
	txStageTotal    = "total"

	txStageResultSuccess  = "success"
	txStageResultError    = "error"
	txStageResultCommit   = "commit"
	txStageResultRollback = "rollback"
	txStageResultPanic    = "panic"
)

type Store struct {
	db      *sql.DB
	metrics store.TransactionRecorder
	attr    runtimeattr.Recorder
}

func NewStore(db *sql.DB, metrics store.TransactionRecorder) (*Store, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	if metrics == nil {
		metrics = store.NoOpTransactionRecorder{}
	}
	return &Store{
		db:      db,
		metrics: metrics,
		attr:    runtimeattr.NoOpRecorder{},
	}, nil
}

// WithAttribution wires optional low-cardinality runtime attribution.
// Passing nil restores the no-op recorder.
func (s *Store) WithAttribution(rec runtimeattr.Recorder) *Store {
	if s == nil {
		return s
	}
	s.attr = runtimeattr.RecorderOrNoOp(rec)
	return s
}

// DBStats returns the current database/sql pool statistics for production
// metrics. It deliberately exposes only aggregate pool counters, never DSN or
// host details.
func (s *Store) DBStats() sql.DBStats {
	if s == nil || s.db == nil {
		return sql.DBStats{}
	}
	return s.db.Stats()
}

// OutboxBacklogStats returns aggregate unpublished-outbox metrics using the
// base database handle. It is read-only and safe for scrape-time metrics.
func (s *Store) OutboxBacklogStats(ctx context.Context) (outbox.BacklogStats, error) {
	if s == nil || s.db == nil {
		return outbox.BacklogStats{}, ErrNilDB
	}
	repo, err := NewOutboxRepo(s.db)
	if err != nil {
		return outbox.BacklogStats{}, err
	}
	return repo.BacklogStats(ctx)
}

// Repositories returns repositories bound to the base DB connection.
// Use this for read operations that do not require a transaction.
func (s *Store) Repositories() (*store.Repositories, error) {
	return newRepositories(s.db, s.attr)
}

// WithTx executes fn with repositories bound to a transaction.
// operation should describe the business workflow (e.g., "evaluation", "review", "admin_update").
func (s *Store) WithTx(ctx context.Context, operation string, fn func(*store.Repositories) error) (err error) {
	start := time.Now()

	beginStart := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	runtimeattr.Observe(s.attr, runtimeattr.StageTransactionBegin, beginStart)
	beginDuration := time.Since(beginStart)
	if err != nil {
		s.metrics.RecordTransactionStageDuration(operation, txStageBegin, txStageResultError, beginDuration)
		slog.Error("tx_begin_failed",
			"operation", operation,
			"error", err,
		)
		s.metrics.IncrementTransactionError(operation, "begin")
		return err
	}
	s.metrics.RecordTransactionStageDuration(operation, txStageBegin, txStageResultSuccess, beginDuration)

	defer func() {
		if p := recover(); p != nil {
			duration := time.Since(start)
			slog.Error("tx_panic_recovered",
				"operation", operation,
				"duration_ms", duration.Milliseconds(),
				"panic_value", fmt.Sprintf("%v", p),
			)
			s.metrics.IncrementTransactionError(operation, "panic")
			s.metrics.IncrementTransactionRollback(operation)
			s.metrics.RecordTransactionDuration(operation, "panic", duration)
			s.metrics.RecordTransactionStageDuration(operation, txStageTotal, txStageResultPanic, duration)

			// Attempt rollback and log if it fails
			rollbackStart := time.Now()
			if rbErr := tx.Rollback(); rbErr != nil {
				s.metrics.RecordTransactionStageDuration(operation, txStageRollback, txStageResultError, time.Since(rollbackStart))
				slog.Error("tx_rollback_failed_after_panic",
					"operation", operation,
					"rollback_error", rbErr,
				)
			} else {
				s.metrics.RecordTransactionStageDuration(operation, txStageRollback, txStageResultSuccess, time.Since(rollbackStart))
			}
			panic(p)
		}
	}()

	repos, err := newRepositories(tx, s.attr)
	if err != nil {
		slog.Error("tx_repository_factory_failed",
			"operation", operation,
			"error", err,
		)
		s.metrics.IncrementTransactionError(operation, "repository_factory")

		rollbackStart := time.Now()
		if rbErr := tx.Rollback(); rbErr != nil {
			s.metrics.RecordTransactionStageDuration(operation, txStageRollback, txStageResultError, time.Since(rollbackStart))
			duration := time.Since(start)
			slog.Error("tx_rollback_failed_after_factory_error",
				"operation", operation,
				"factory_error", err,
				"rollback_error", rbErr,
			)
			s.metrics.IncrementTransactionError(operation, "rollback_after_factory_error")
			s.metrics.RecordTransactionDuration(operation, "rollback_error", duration)
			s.metrics.RecordTransactionStageDuration(operation, txStageTotal, "rollback_error", duration)
			return errors.Join(err, rbErr)
		}
		s.metrics.RecordTransactionStageDuration(operation, txStageRollback, txStageResultSuccess, time.Since(rollbackStart))

		duration := time.Since(start)
		s.metrics.IncrementTransactionRollback(operation)
		s.metrics.RecordTransactionDuration(operation, "rollback", duration)
		s.metrics.RecordTransactionStageDuration(operation, txStageTotal, txStageResultRollback, duration)
		return err
	}

	callbackStart := time.Now()
	if err := fn(repos); err != nil {
		runtimeattr.Observe(s.attr, runtimeattr.StageTransactionCallback, callbackStart)
		s.metrics.RecordTransactionStageDuration(operation, txStageCallback, txStageResultError, time.Since(callbackStart))
		// Callback returned error - may be business logic or repository failure
		s.metrics.IncrementTransactionError(operation, "callback_returned_error")

		rollbackStart := time.Now()
		if rbErr := tx.Rollback(); rbErr != nil {
			s.metrics.RecordTransactionStageDuration(operation, txStageRollback, txStageResultError, time.Since(rollbackStart))
			duration := time.Since(start)
			slog.Error("tx_rollback_failed_after_callback_error",
				"operation", operation,
				"callback_error", err,
				"rollback_error", rbErr,
			)
			s.metrics.IncrementTransactionError(operation, "rollback_after_callback_error")
			s.metrics.RecordTransactionDuration(operation, "rollback_error", duration)
			s.metrics.RecordTransactionStageDuration(operation, txStageTotal, "rollback_error", duration)
			return errors.Join(err, rbErr)
		}
		s.metrics.RecordTransactionStageDuration(operation, txStageRollback, txStageResultSuccess, time.Since(rollbackStart))

		duration := time.Since(start)
		s.metrics.IncrementTransactionRollback(operation)
		s.metrics.RecordTransactionDuration(operation, "rollback", duration)
		s.metrics.RecordTransactionStageDuration(operation, txStageTotal, txStageResultRollback, duration)
		return err
	}
	runtimeattr.Observe(s.attr, runtimeattr.StageTransactionCallback, callbackStart)
	s.metrics.RecordTransactionStageDuration(operation, txStageCallback, txStageResultSuccess, time.Since(callbackStart))

	commitStart := time.Now()
	if err := tx.Commit(); err != nil {
		runtimeattr.Observe(s.attr, runtimeattr.StageTransactionCommit, commitStart)
		s.metrics.RecordTransactionStageDuration(operation, txStageCommit, txStageResultError, time.Since(commitStart))
		duration := time.Since(start)
		slog.Error("tx_commit_failed",
			"operation", operation,
			"duration_ms", duration.Milliseconds(),
			"error", err,
		)
		s.metrics.IncrementTransactionError(operation, "commit")
		s.metrics.RecordTransactionDuration(operation, "commit_error", duration)
		s.metrics.RecordTransactionStageDuration(operation, txStageTotal, "commit_error", duration)
		return err
	}
	runtimeattr.Observe(s.attr, runtimeattr.StageTransactionCommit, commitStart)
	s.metrics.RecordTransactionStageDuration(operation, txStageCommit, txStageResultSuccess, time.Since(commitStart))

	duration := time.Since(start)
	s.attr.RecordDuration(runtimeattr.StageTransactionTotal, duration)
	s.metrics.IncrementTransactionCommit(operation)
	s.metrics.RecordTransactionDuration(operation, "commit", duration)
	s.metrics.RecordTransactionStageDuration(operation, txStageTotal, txStageResultCommit, duration)
	return nil
}

func newRepositories(db sqltx.DBTX, recorders ...runtimeattr.Recorder) (*store.Repositories, error) {
	var attr runtimeattr.Recorder = runtimeattr.NoOpRecorder{}
	if len(recorders) > 0 {
		attr = runtimeattr.RecorderOrNoOp(recorders[0])
	}
	caps, err := NewCapabilityRepo(db)
	if err != nil {
		return nil, err
	}

	procs, err := NewProcessRepo(db)
	if err != nil {
		return nil, err
	}

	surfaces, err := NewSurfaceRepo(db)
	if err != nil {
		return nil, err
	}

	agents, err := NewAgentRepo(db)
	if err != nil {
		return nil, err
	}

	profiles, err := NewProfileRepo(db)
	if err != nil {
		return nil, err
	}

	grants, err := NewGrantRepo(db)
	if err != nil {
		return nil, err
	}

	envelopes, err := NewEnvelopeRepo(db)
	if err != nil {
		return nil, err
	}
	envelopes.WithAttribution(attr)

	outboxRepo, err := NewOutboxRepo(db)
	if err != nil {
		return nil, err
	}
	outboxRepo.WithAttribution(attr)

	auditRepo := audit.NewPostgresRepository(db).WithAttribution(attr)

	controlAuditRepo, err := NewControlAuditRepo(db)
	if err != nil {
		return nil, err
	}

	adminAuditRepo, err := NewAdminAuditRepo(db)
	if err != nil {
		return nil, err
	}

	localUsers, err := NewLocalUserRepo(db)
	if err != nil {
		return nil, err
	}

	localSessions, err := NewLocalSessionRepo(db)
	if err != nil {
		return nil, err
	}

	businessServices, err := NewBusinessServiceRepo(db)
	if err != nil {
		return nil, err
	}

	bsCaps, err := NewBusinessServiceCapabilityRepo(db)
	if err != nil {
		return nil, err
	}

	bsRelationships, err := NewBusinessServiceRelationshipRepo(db)
	if err != nil {
		return nil, err
	}

	governanceExpectations, err := NewGovernanceExpectationRepo(db)
	if err != nil {
		return nil, err
	}

	aiSystems, err := NewAISystemRepo(db)
	if err != nil {
		return nil, err
	}

	aiSystemVersions, err := NewAISystemVersionRepo(db)
	if err != nil {
		return nil, err
	}

	aiSystemBindings, err := NewAISystemBindingRepo(db)
	if err != nil {
		return nil, err
	}

	failModePolicies, err := NewFailModePolicyRepo(db)
	if err != nil {
		return nil, err
	}

	escalationTargets, err := NewEscalationTargetRepo(db)
	if err != nil {
		return nil, err
	}

	driftDefinitions, err := NewDriftDefinitionRepo(db)
	if err != nil {
		return nil, err
	}

	driftSeries, err := NewDriftSeriesRepo(db)
	if err != nil {
		return nil, err
	}

	driftSeriesPoints, err := NewDriftSeriesPointRepo(db)
	if err != nil {
		return nil, err
	}

	driftObservations, err := NewDriftObservationRepo(db)
	if err != nil {
		return nil, err
	}

	driftAnnotations, err := NewDriftAnnotationRepo(db)
	if err != nil {
		return nil, err
	}

	return &store.Repositories{
		Capabilities:                 caps,
		Processes:                    procs,
		Surfaces:                     surfaces,
		Agents:                       agents,
		Profiles:                     profiles,
		Grants:                       grants,
		Envelopes:                    envelopes,
		Audit:                        auditRepo,
		ControlAudit:                 controlAuditRepo,
		AdminAudit:                   adminAuditRepo,
		Outbox:                       outboxRepo,
		LocalUsers:                   localUsers,
		LocalSessions:                localSessions,
		BusinessServices:             businessServices,
		BusinessServiceCapabilities:  bsCaps,
		BusinessServiceRelationships: bsRelationships,
		GovernanceExpectations:       governanceExpectations,
		AISystems:                    aiSystems,
		AISystemVersions:             aiSystemVersions,
		AISystemBindings:             aiSystemBindings,
		FailModePolicies:             failModePolicies,
		EscalationTargets:            escalationTargets,
		DriftDefinitions:             driftDefinitions,
		DriftSeries:                  driftSeries,
		DriftSeriesPoints:            driftSeriesPoints,
		DriftObservations:            driftObservations,
		DriftAnnotations:             driftAnnotations,
	}, nil
}
