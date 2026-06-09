package bootstrap

// synthetic_drift.go — Drift-2a synthetic drift generator.
//
// Deterministic, dev/demo-only seed that populates the drift repositories
// (DriftDefinitions, DriftSeries, DriftSeriesPoints, DriftObservations,
// DriftAnnotations) with a plausible dataset over existing demo entities.
// Intended consumer is the Drift-2b heatmap UI tranche — runtime ingestion
// does not exist yet, so without this seed the read APIs return empty
// pages.
//
// Properties enforced here:
//
//   - Deterministic: no time.Now(); no global RNG. All timestamps derive
//     from syntheticDriftEpoch; all jitter uses a per-series rand.Rand
//     seeded from a stable hash of the series ID.
//   - Idempotent: every entity is looked up by stable ID before Create;
//     existing rows are left unchanged. No Update / UpdateStatus /
//     UpdateOperatorStatus / Supersede / Seal / DeleteBefore calls.
//   - Runtime-inert: no aggregation, detector, audit-chain, envelope, or
//     decision-path code is invoked. The generator writes directly to
//     repositories with status=active because Drift-2a is demo seed data,
//     not a governance apply path.
//   - V1 only: every drift type is one of the nine V1 values; every
//     status uses the five-band set with no bare 'unknown'.
//   - Gated: callers invoke this only when MIDAS_DEV_SEED_SYNTHETIC_DRIFT
//     is true (off by default).
//
// Coverage: ten DriftDefinitions covering all nine V1 drift types and all
// nine V1 TargetEntityKinds, with metrics whose series exercise all five
// status bands and at least one backfilled point + observation.

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"time"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store"
)

// syntheticDriftCreator is the audit attribution string for every entity
// produced by this seed. Distinct from human user IDs so a future
// operator can scan-and-purge synthetic rows by attribution.
const syntheticDriftCreator = "seed-synthetic-drift"

// syntheticDriftEpoch anchors every timestamp produced by this seed.
// Pinned (rather than time.Now()) so the dataset is byte-identical
// across invocations on different days — the dev UX requirement that
// makes Drift-2b screenshots stable.
var syntheticDriftEpoch = time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

// syntheticDriftExplorerAnalyticsEpoch anchors the compact Explorer Drift
// Analytics demo target inside the June 2026 30-day browser diagnostics
// window without mutating the existing Drift-2a April dataset.
var syntheticDriftExplorerAnalyticsEpoch = time.Date(2026, time.May, 11, 0, 0, 0, 0, time.UTC)

// syntheticDriftEffective is the EffectiveDate stamped on every
// generated DriftDefinition. Before the first point window so the
// definitions are "active at" every synthetic point.
var syntheticDriftEffective = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// syntheticPointCount is the number of daily points produced per series.
// Brief minimum is 30; we emit exactly 30 so windows span [epoch, epoch+30d).
const syntheticPointCount = 30

// syntheticDayWindow is the per-point cadence. All series in this seed
// use daily windows.
const syntheticDayWindow = 24 * time.Hour

// syntheticBackfillRunID is the BackfillRunID stamped on the one
// backfilled point + observation the brief requires. Stable so tests
// can pin it.
const syntheticBackfillRunID = "backfill-2026-04-15"

// syntheticPattern enumerates the per-series trajectories the generator
// supports. Each pattern produces a deterministic value sequence that,
// when scored against the metric thresholds, yields the documented
// status mix and series-roll-up status.
type syntheticPattern int

const (
	patternAllHealthy syntheticPattern = iota
	patternWarningTrail
	patternBreachedTrail
	patternUnknownInsufficientData
	patternUnknownDetectorError
)

// syntheticDef is the in-memory plan for one DriftDefinition revision.
type syntheticDef struct {
	ID               string
	Name             string
	Description      string
	BusinessOwner    string
	TechnicalOwner   string
	TargetEntityKind drift.TargetEntityKind
	TargetEntityID   string
	PointEpoch       time.Time
	Metrics          []syntheticMetric
}

// syntheticMetric is the in-memory plan for one DriftMetricDefinition
// plus its derived DriftSeries, DriftSeriesPoints, DriftObservation
// (optional), and DriftAnnotation (optional).
type syntheticMetric struct {
	MetricID           string
	Description        string
	DriftType          drift.DriftType
	ThresholdDirection drift.ThresholdDirection
	WarningThreshold   float64
	BreachedThreshold  float64
	Pattern            syntheticPattern
	// BackfillPointIndex is the index (0..syntheticPointCount-1) of the
	// single point to mark as backfilled. -1 disables; only one pattern
	// in the dataset uses a non-negative value to satisfy the
	// "≥1 backfilled point" requirement.
	BackfillPointIndex int
	// Observation, when non-nil, requests one DriftObservation pinned to
	// the latest non-healthy point of the series (or, for the
	// unknown_detector_error pattern, the first error point).
	Observation *syntheticObservation
	// Annotation, when non-nil, requests one DriftAnnotation against the
	// emitted DriftSeries.
	Annotation *syntheticAnnotation
}

type syntheticObservation struct {
	OperatorStatus drift.DriftObservationOperatorStatus
	Backfilled     bool
}

type syntheticAnnotation struct {
	Type drift.DriftAnnotationType
	Body string
}

// syntheticDriftPlan returns the deterministic plan for the seed. The
// plan is the single source of truth for IDs, target entities, and
// patterns; everything downstream of this function is a pure projection.
//
// Coverage matrix (definition × drift types × target kinds × statuses):
//
//   - All nine V1 DriftTypes appear at least once.
//   - All nine V1 TargetEntityKinds are exercised.
//   - All five DriftSeriesStatus bands appear across the series set.
//   - Exactly one (metric, point) pair is backfilled; its observation
//     is emitted as Backfilled=true with BackfillRunID set.
//   - Two annotations total: known_business_change on the credit-outcome
//     breach series; remediation_note on the merchant-fraud warning
//     series.
func syntheticDriftPlan() []syntheticDef {
	return []syntheticDef{
		{
			ID:               "drift-demo-merchant-fraud-invocation",
			Name:             "Merchant Fraud Invocation & Latency",
			Description:      "Invocation rate and detector latency on the merchant-risk fraud binding.",
			BusinessOwner:    "merchant-services-team",
			TechnicalOwner:   "ai-platform-team",
			TargetEntityKind: drift.TargetEntityKindAISystemBinding,
			TargetEntityID:   "bind-fraud-on-merchant-risk-surf",
			Metrics: []syntheticMetric{
				{
					MetricID:           "invocation-rate",
					Description:        "Invocations per merchant-fraud binding per day, normalised against baseline.",
					DriftType:          drift.DriftTypeInvocation,
					ThresholdDirection: drift.ThresholdDirectionDescending,
					WarningThreshold:   0.95,
					BreachedThreshold:  0.80,
					Pattern:            patternWarningTrail,
					BackfillPointIndex: -1,
					Observation: &syntheticObservation{
						OperatorStatus: drift.DriftObservationOperatorStatusOpen,
						Backfilled:     false,
					},
					Annotation: &syntheticAnnotation{
						Type: drift.DriftAnnotationTypeRemediationNote,
						Body: "Merchant-acquisition push shifted traffic mix; monitor one cycle before triggering FailModePolicy.",
					},
				},
				{
					MetricID:           "p95-latency-ms",
					Description:        "P95 detector latency in milliseconds.",
					DriftType:          drift.DriftTypeLatency,
					ThresholdDirection: drift.ThresholdDirectionAscending,
					WarningThreshold:   400,
					BreachedThreshold:  800,
					Pattern:            patternAllHealthy,
					BackfillPointIndex: -1,
				},
			},
		},
		{
			ID:               "drift-demo-credit-outcome",
			Name:             "Credit Assessment Outcome & Confidence",
			Description:      "Approve-rate and mean-confidence on the credit-assess surface.",
			BusinessOwner:    "consumer-lending-team",
			TechnicalOwner:   "credit-platform-team",
			TargetEntityKind: drift.TargetEntityKindDecisionSurface,
			TargetEntityID:   "surf-v2-credit-assess",
			Metrics: []syntheticMetric{
				{
					MetricID:           "approve-rate",
					Description:        "Fraction of credit applications approved.",
					DriftType:          drift.DriftTypeOutcome,
					ThresholdDirection: drift.ThresholdDirectionDescending,
					WarningThreshold:   0.85,
					BreachedThreshold:  0.70,
					Pattern:            patternBreachedTrail,
					BackfillPointIndex: -1,
					Observation: &syntheticObservation{
						OperatorStatus: drift.DriftObservationOperatorStatusTriaged,
						Backfilled:     false,
					},
					Annotation: &syntheticAnnotation{
						Type: drift.DriftAnnotationTypeKnownBusinessChange,
						Body: "Risk-appetite refresh tightened approve thresholds 2026-04-15; downward drift is expected.",
					},
				},
				{
					MetricID:           "confidence-mean",
					Description:        "Mean classifier confidence per window.",
					DriftType:          drift.DriftTypeConfidence,
					ThresholdDirection: drift.ThresholdDirectionDescending,
					WarningThreshold:   0.80,
					BreachedThreshold:  0.70,
					Pattern:            patternWarningTrail,
					BackfillPointIndex: -1,
					Observation: &syntheticObservation{
						OperatorStatus: drift.DriftObservationOperatorStatusAccepted,
						Backfilled:     false,
					},
				},
			},
		},
		{
			ID:               "drift-demo-id-verify-evidence",
			Name:             "ID Verification Evidence Completeness",
			Description:      "Evidence-completeness ratio for the id-verify surface.",
			BusinessOwner:    "onboarding-team",
			TechnicalOwner:   "identity-platform-team",
			TargetEntityKind: drift.TargetEntityKindDecisionSurface,
			TargetEntityID:   "surf-v2-id-verify",
			Metrics: []syntheticMetric{
				{
					MetricID:           "evidence-completeness",
					Description:        "Fraction of decisions emitting a complete evidence envelope.",
					DriftType:          drift.DriftTypeEvidence,
					ThresholdDirection: drift.ThresholdDirectionDescending,
					WarningThreshold:   0.95,
					BreachedThreshold:  0.80,
					Pattern:            patternUnknownInsufficientData,
					BackfillPointIndex: -1,
				},
			},
		},
		{
			ID:               "drift-demo-fraud-policy",
			Name:             "Transaction Monitoring Policy & Path",
			Description:      "Policy-match rate and process-path divergence on the transaction-monitoring process.",
			BusinessOwner:    "ffc-team",
			TechnicalOwner:   "policy-platform-team",
			TargetEntityKind: drift.TargetEntityKindProcess,
			TargetEntityID:   "proc-transaction-monitoring",
			Metrics: []syntheticMetric{
				{
					MetricID:           "policy-match-rate",
					Description:        "Fraction of evaluations matching at least one active policy.",
					DriftType:          drift.DriftTypePolicy,
					ThresholdDirection: drift.ThresholdDirectionDescending,
					WarningThreshold:   0.95,
					BreachedThreshold:  0.85,
					Pattern:            patternUnknownDetectorError,
					BackfillPointIndex: -1,
					Observation: &syntheticObservation{
						OperatorStatus: drift.DriftObservationOperatorStatusOpen,
						Backfilled:     true,
					},
				},
				{
					MetricID:           "process-path-divergence",
					Description:        "Path-divergence index versus golden process path.",
					DriftType:          drift.DriftTypeProcessPath,
					ThresholdDirection: drift.ThresholdDirectionAscending,
					WarningThreshold:   0.10,
					BreachedThreshold:  0.20,
					Pattern:            patternAllHealthy,
					BackfillPointIndex: -1,
				},
			},
		},
		{
			ID:               "drift-demo-credit-authority",
			Name:             "Credit Assess Authority Utilisation",
			Description:      "Authority-utilisation ratio for the credit-assess authority profile.",
			BusinessOwner:    "consumer-lending-team",
			TechnicalOwner:   "governance-platform-team",
			TargetEntityKind: drift.TargetEntityKindAuthorityProfile,
			TargetEntityID:   "profile-v2-credit-assess",
			Metrics: []syntheticMetric{
				{
					MetricID:           "authority-utilization",
					Description:        "Ratio of granted authority consumed per window.",
					DriftType:          drift.DriftTypeAuthority,
					ThresholdDirection: drift.ThresholdDirectionAscending,
					WarningThreshold:   0.80,
					BreachedThreshold:  0.95,
					Pattern:            patternAllHealthy,
					BackfillPointIndex: -1,
				},
			},
		},
		{
			ID:               "drift-demo-payments-coverage",
			Name:             "Payments Governance Coverage",
			Description:      "Governance-coverage ratio for the payments business service.",
			BusinessOwner:    "payments-team",
			TechnicalOwner:   "governance-platform-team",
			TargetEntityKind: drift.TargetEntityKindBusinessService,
			TargetEntityID:   "bs-payments",
			Metrics: []syntheticMetric{
				{
					MetricID:           "governance-coverage",
					Description:        "Fraction of payments decisions covered by an active GovernanceExpectation.",
					DriftType:          drift.DriftTypeCoverage,
					ThresholdDirection: drift.ThresholdDirectionDescending,
					WarningThreshold:   0.95,
					BreachedThreshold:  0.80,
					Pattern:            patternBreachedTrail,
					BackfillPointIndex: 12,
					Observation: &syntheticObservation{
						OperatorStatus: drift.DriftObservationOperatorStatusOpen,
						Backfilled:     false,
					},
				},
			},
		},
		{
			ID:               "drift-demo-payment-execution-latency",
			Name:             "Payment Execution Latency",
			Description:      "P95 payment execution latency for the visible payment-execution capability.",
			BusinessOwner:    "payments-team",
			TechnicalOwner:   "payments-platform-team",
			TargetEntityKind: drift.TargetEntityKindCapability,
			TargetEntityID:   "cap-payment-execution",
			PointEpoch:       syntheticDriftExplorerAnalyticsEpoch,
			Metrics: []syntheticMetric{
				{
					MetricID:           "p95-execution-latency-ms",
					Description:        "P95 payment execution latency in milliseconds.",
					DriftType:          drift.DriftTypeLatency,
					ThresholdDirection: drift.ThresholdDirectionAscending,
					WarningThreshold:   120,
					BreachedThreshold:  180,
					Pattern:            patternWarningTrail,
					BackfillPointIndex: -1,
				},
			},
		},
		{
			ID:               "drift-demo-fraud-system-confidence",
			Name:             "Fraud Detection Decision Confidence",
			Description:      "Decision-confidence for the fraud-detection AI system across all bindings.",
			BusinessOwner:    "ffc-team",
			TechnicalOwner:   "ai-platform-team",
			TargetEntityKind: drift.TargetEntityKindAISystem,
			TargetEntityID:   "aisys-fraud-detection",
			Metrics: []syntheticMetric{
				{
					MetricID:           "decision-confidence",
					Description:        "System-wide mean decision confidence.",
					DriftType:          drift.DriftTypeConfidence,
					ThresholdDirection: drift.ThresholdDirectionDescending,
					WarningThreshold:   0.85,
					BreachedThreshold:  0.75,
					Pattern:            patternWarningTrail,
					BackfillPointIndex: -1,
					Observation: &syntheticObservation{
						OperatorStatus: drift.DriftObservationOperatorStatusTriaged,
						Backfilled:     false,
					},
				},
			},
		},
		{
			ID:               "drift-demo-evaluator-agent-invocation",
			Name:             "Evaluator Agent Invocation Volume",
			Description:      "Invocation volume for the v2 evaluator agent.",
			BusinessOwner:    "ai-platform-team",
			TechnicalOwner:   "ai-platform-team",
			TargetEntityKind: drift.TargetEntityKindAgent,
			TargetEntityID:   "agent-v2-evaluator",
			Metrics: []syntheticMetric{
				{
					MetricID:           "invocations-per-day",
					Description:        "Invocations per day; ascending warning when traffic spikes.",
					DriftType:          drift.DriftTypeInvocation,
					ThresholdDirection: drift.ThresholdDirectionAscending,
					WarningThreshold:   2000,
					BreachedThreshold:  5000,
					Pattern:            patternAllHealthy,
					BackfillPointIndex: -1,
				},
			},
		},
		{
			ID:               "drift-demo-fraud-capability-coverage",
			Name:             "Fraud Detection Capability Coverage",
			Description:      "Governance-coverage ratio across the fraud-detection capability.",
			BusinessOwner:    "ffc-team",
			TechnicalOwner:   "governance-platform-team",
			TargetEntityKind: drift.TargetEntityKindCapability,
			TargetEntityID:   "cap-fraud-detection",
			Metrics: []syntheticMetric{
				{
					MetricID:           "capability-coverage",
					Description:        "Fraction of fraud-detection invocations covered by an active GovernanceExpectation.",
					DriftType:          drift.DriftTypeCoverage,
					ThresholdDirection: drift.ThresholdDirectionDescending,
					WarningThreshold:   0.95,
					BreachedThreshold:  0.80,
					Pattern:            patternAllHealthy,
					BackfillPointIndex: -1,
				},
			},
		},
		{
			ID:               "drift-demo-fraud-grant-authority",
			Name:             "Fraud Detection Grant Authority Utilisation",
			Description:      "Authority utilisation for the fraud-detection authority grant.",
			BusinessOwner:    "ffc-team",
			TechnicalOwner:   "governance-platform-team",
			TargetEntityKind: drift.TargetEntityKindAuthorityGrant,
			TargetEntityID:   "grant-v2-fraud-detection",
			Metrics: []syntheticMetric{
				{
					MetricID:           "grant-utilization",
					Description:        "Ratio of grant scope consumed per window.",
					DriftType:          drift.DriftTypeAuthority,
					ThresholdDirection: drift.ThresholdDirectionAscending,
					WarningThreshold:   0.85,
					BreachedThreshold:  0.95,
					Pattern:            patternWarningTrail,
					BackfillPointIndex: -1,
					Observation: &syntheticObservation{
						OperatorStatus: drift.DriftObservationOperatorStatusOpen,
						Backfilled:     false,
					},
				},
			},
		},
	}
}

// SeedSyntheticDrift populates the drift repositories with the
// deterministic Drift-2a synthetic dataset. Callers must check
// cfg.Dev.SeedSyntheticDrift before invoking. Safe to re-run; existing
// rows (matched by stable ID) are left unchanged.
//
// All five drift repositories on the supplied *store.Repositories must
// be non-nil. Returns an error if any is missing — the seed has no
// useful behaviour on a partially-configured store, and silently
// degrading would mask configuration mistakes.
func SeedSyntheticDrift(ctx context.Context, repos *store.Repositories) error {
	if repos == nil {
		return errors.New("bootstrap: SeedSyntheticDrift requires non-nil repos")
	}
	if repos.DriftDefinitions == nil ||
		repos.DriftSeries == nil ||
		repos.DriftSeriesPoints == nil ||
		repos.DriftObservations == nil ||
		repos.DriftAnnotations == nil {
		return errors.New(
			"bootstrap: SeedSyntheticDrift requires all five drift repositories " +
				"(DriftDefinitions, DriftSeries, DriftSeriesPoints, DriftObservations, DriftAnnotations) " +
				"to be configured",
		)
	}

	plan := syntheticDriftPlan()
	for _, defPlan := range plan {
		if err := ensureSyntheticDriftDefinition(ctx, repos, defPlan); err != nil {
			return err
		}
		for _, m := range defPlan.Metrics {
			seriesID := syntheticSeriesID(defPlan.ID, m.MetricID)
			seriesStatus := rollupStatus(m.Pattern)
			if err := ensureSyntheticDriftSeries(ctx, repos, defPlan, m, seriesID, seriesStatus); err != nil {
				return err
			}
			if err := ensureSyntheticDriftPoints(ctx, repos, defPlan, m, seriesID); err != nil {
				return err
			}
			if err := ensureSyntheticDriftObservation(ctx, repos, defPlan, m, seriesID); err != nil {
				return err
			}
			if err := ensureSyntheticDriftAnnotation(ctx, repos, defPlan, m, seriesID); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// ID helpers
//
// All IDs derive deterministically from the definition/metric/point index
// pair; nothing references time.Now() or a global RNG.
// ---------------------------------------------------------------------------

func syntheticSeriesID(defID, metricID string) string {
	return defID + "-series-" + metricID
}

func syntheticPointID(seriesID string, index int) string {
	return fmt.Sprintf("%s-pt-%03d", seriesID, index)
}

func syntheticObservationID(seriesID string, index int) string {
	return fmt.Sprintf("%s-obs-%03d", seriesID, index)
}

func syntheticAnnotationID(seriesID string) string {
	return seriesID + "-anno-001"
}

func syntheticBaselineWindowID(seriesID string, index int) string {
	return fmt.Sprintf("%s-bw-%03d", seriesID, index)
}

func syntheticPointEpoch(p syntheticDef) time.Time {
	if p.PointEpoch.IsZero() {
		return syntheticDriftEpoch
	}
	return p.PointEpoch
}

// ---------------------------------------------------------------------------
// Ensure helpers
//
// Each helper looks up the row by its stable ID and only calls Create
// when no row exists. Mirrors the bootstrap.SeedDemo ensure* convention
// so adding the synthetic seed alongside the structural seed is a
// uniform mental model.
// ---------------------------------------------------------------------------

func ensureSyntheticDriftDefinition(ctx context.Context, repos *store.Repositories, p syntheticDef) error {
	existing, err := repos.DriftDefinitions.FindByIDAndVersion(ctx, p.ID, 1)
	if err != nil {
		return fmt.Errorf("lookup drift definition %s v1: %w", p.ID, err)
	}
	if existing != nil {
		return nil
	}

	approvedAt := syntheticDriftEffective
	def := &drift.DriftDefinition{
		ID:               p.ID,
		Version:          1,
		Name:             p.Name,
		Description:      p.Description,
		Status:           drift.DriftDefinitionStatusActive,
		EffectiveDate:    syntheticDriftEffective,
		BusinessOwner:    p.BusinessOwner,
		TechnicalOwner:   p.TechnicalOwner,
		TargetEntityKind: p.TargetEntityKind,
		TargetEntityID:   p.TargetEntityID,
		Metrics:          make([]drift.DriftMetricDefinition, 0, len(p.Metrics)),
		Origin:           drift.DriftOriginManual,
		Managed:          true,
		CreatedAt:        syntheticDriftEffective,
		UpdatedAt:        syntheticDriftEffective,
		CreatedBy:        syntheticDriftCreator,
		ApprovedBy:       syntheticDriftCreator,
		ApprovedAt:       &approvedAt,
	}
	for _, m := range p.Metrics {
		def.Metrics = append(def.Metrics, drift.DriftMetricDefinition{
			MetricID:              m.MetricID,
			DriftType:             m.DriftType,
			BaselineStrategy:      drift.BaselineStrategyRolling,
			BaselineWindowSeconds: int(7 * syntheticDayWindow / time.Second),
			WindowSeconds:         int(syntheticDayWindow / time.Second),
			Cadence:               drift.CadenceDay,
			WarningThreshold:      m.WarningThreshold,
			BreachedThreshold:     m.BreachedThreshold,
			ThresholdDirection:    m.ThresholdDirection,
			Description:           m.Description,
		})
	}

	if err := repos.DriftDefinitions.Create(ctx, def); err != nil {
		return fmt.Errorf("create drift definition %s v1: %w", p.ID, err)
	}
	return nil
}

func ensureSyntheticDriftSeries(
	ctx context.Context,
	repos *store.Repositories,
	defPlan syntheticDef,
	m syntheticMetric,
	seriesID string,
	rollup drift.DriftSeriesStatus,
) error {
	existing, err := repos.DriftSeries.FindByID(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("lookup drift series %s: %w", seriesID, err)
	}
	if existing != nil {
		return nil
	}

	s := &drift.DriftSeries{
		ID:                seriesID,
		DefinitionID:      defPlan.ID,
		DefinitionVersion: 1,
		MetricID:          m.MetricID,
		Cadence:           drift.CadenceDay,
		Status:            rollup,
		ContinuityGroupID: defPlan.ID + "-cg",
		CreatedAt:         syntheticDriftEffective,
		UpdatedAt:         syntheticPointEpoch(defPlan).Add(syntheticPointCount * syntheticDayWindow),
	}
	if err := repos.DriftSeries.Create(ctx, s); err != nil {
		return fmt.Errorf("create drift series %s: %w", seriesID, err)
	}
	return nil
}

func ensureSyntheticDriftPoints(
	ctx context.Context,
	repos *store.Repositories,
	defPlan syntheticDef,
	m syntheticMetric,
	seriesID string,
) error {
	rng := rand.New(rand.NewSource(stableSeed(seriesID)))
	for i := 0; i < syntheticPointCount; i++ {
		pointID := syntheticPointID(seriesID, i)
		existing, err := repos.DriftSeriesPoints.FindByID(ctx, pointID)
		if err != nil {
			return fmt.Errorf("lookup drift point %s: %w", pointID, err)
		}
		if existing != nil {
			continue
		}

		windowStart := syntheticPointEpoch(defPlan).Add(time.Duration(i) * syntheticDayWindow)
		windowEnd := windowStart.Add(syntheticDayWindow)

		status, value, baseline, magnitude := scorePoint(m, i, rng)

		mode := drift.DriftPointComputationModeRealtime
		backfillRunID := ""
		computedAt := windowEnd.Add(time.Hour)
		if i == m.BackfillPointIndex {
			mode = drift.DriftPointComputationModeBackfilled
			backfillRunID = syntheticBackfillRunID
			computedAt = windowEnd.Add(7 * syntheticDayWindow)
		}

		baselineWindowID := syntheticBaselineWindowID(seriesID, i)
		if status == drift.DriftSeriesPointStatusUnknownInsufficientData {
			baselineWindowID = ""
		}

		p := &drift.DriftSeriesPoint{
			ID:          pointID,
			SeriesID:    seriesID,
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
			SampleCount: 10000 + int64(rng.Intn(5000)),
			SummaryStats: map[string]any{
				"value": value,
				"unit":  unitForMetric(m),
			},
			BaselineStats: map[string]any{
				"baseline": baseline,
				"strategy": string(drift.BaselineStrategyRolling),
			},
			BaselineWindowID:      baselineWindowID,
			Magnitude:             magnitude,
			Status:                status,
			ComputationMode:       mode,
			ComputedAt:            computedAt,
			BackfillRunID:         backfillRunID,
			SourceWindowComplete:  true,
			ProvenanceEnvelopeIDs: []string{},
			CreatedAt:             computedAt,
		}
		if err := repos.DriftSeriesPoints.Create(ctx, p); err != nil {
			return fmt.Errorf("create drift point %s: %w", pointID, err)
		}
	}
	return nil
}

func ensureSyntheticDriftObservation(
	ctx context.Context,
	repos *store.Repositories,
	defPlan syntheticDef,
	m syntheticMetric,
	seriesID string,
) error {
	if m.Observation == nil {
		return nil
	}

	pointIndex := pickObservationPointIndex(m.Pattern)
	if pointIndex < 0 {
		// Pattern does not produce any non-healthy point. Skip silently
		// rather than fabricate an artificial signal.
		return nil
	}

	obsID := syntheticObservationID(seriesID, pointIndex)
	existing, err := repos.DriftObservations.FindByID(ctx, obsID)
	if err != nil {
		return fmt.Errorf("lookup drift observation %s: %w", obsID, err)
	}
	if existing != nil {
		return nil
	}

	pointID := syntheticPointID(seriesID, pointIndex)
	windowStart := syntheticPointEpoch(defPlan).Add(time.Duration(pointIndex) * syntheticDayWindow)
	windowEnd := windowStart.Add(syntheticDayWindow)
	detectedAt := windowEnd.Add(time.Hour)
	emittedAt := detectedAt.Add(time.Minute)

	rng := rand.New(rand.NewSource(stableSeed(seriesID)))
	for i := 0; i < pointIndex; i++ {
		_, _, _, _ = scorePoint(m, i, rng)
	}
	status, _, _, magnitude := scorePoint(m, pointIndex, rng)

	backfillRunID := ""
	if m.Observation.Backfilled {
		backfillRunID = syntheticBackfillRunID
	}

	o := &drift.DriftObservation{
		ID:                  obsID,
		DefinitionID:        defPlan.ID,
		DefinitionVersion:   1,
		SeriesID:            seriesID,
		PointID:             pointID,
		TargetEntityKind:    defPlan.TargetEntityKind,
		TargetEntityID:      defPlan.TargetEntityID,
		DriftType:           m.DriftType,
		Magnitude:           magnitude,
		DetectorStatus:      drift.DriftObservationDetectorStatus(status),
		OperatorStatus:      m.Observation.OperatorStatus,
		BaselineWindowID:    syntheticBaselineWindowID(seriesID, pointIndex),
		ObservedWindowStart: windowStart,
		ObservedWindowEnd:   windowEnd,
		DetectedAt:          detectedAt,
		EmittedAt:           emittedAt,
		Backfilled:          m.Observation.Backfilled,
		BackfillRunID:       backfillRunID,
		EvidenceEnvelopeIDs: []string{},
		CreatedAt:           detectedAt,
		UpdatedAt:           emittedAt,
	}
	// For unknown_insufficient_data points the schema permits an empty
	// baseline_window_id on the observation; clear it for coherence.
	if status == drift.DriftSeriesPointStatusUnknownInsufficientData {
		o.BaselineWindowID = ""
	}
	if err := repos.DriftObservations.Create(ctx, o); err != nil {
		return fmt.Errorf("create drift observation %s: %w", obsID, err)
	}
	return nil
}

func ensureSyntheticDriftAnnotation(
	ctx context.Context,
	repos *store.Repositories,
	defPlan syntheticDef,
	m syntheticMetric,
	seriesID string,
) error {
	if m.Annotation == nil {
		return nil
	}
	annoID := syntheticAnnotationID(seriesID)
	existing, err := repos.DriftAnnotations.FindByID(ctx, annoID)
	if err != nil {
		return fmt.Errorf("lookup drift annotation %s: %w", annoID, err)
	}
	if existing != nil {
		return nil
	}
	a := &drift.DriftAnnotation{
		ID:                   annoID,
		TargetKind:           drift.DriftAnnotationTargetKindSeries,
		TargetID:             seriesID,
		AnnotationType:       m.Annotation.Type,
		Body:                 m.Annotation.Body,
		ReferenceEnvelopeIDs: []string{},
		Status:               drift.DriftAnnotationStatusActive,
		AuthorID:             syntheticDriftCreator,
		CreatedAt:            syntheticPointEpoch(defPlan).Add(syntheticPointCount * syntheticDayWindow),
		UpdatedAt:            syntheticPointEpoch(defPlan).Add(syntheticPointCount * syntheticDayWindow),
	}
	if err := repos.DriftAnnotations.Create(ctx, a); err != nil {
		return fmt.Errorf("create drift annotation %s: %w", annoID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Scoring / pattern helpers
//
// Pure functions: same (metric, index, rng-state) always yields the same
// (status, value, baseline, magnitude). RNG is per-series so jitter is
// deterministic across runs while different series get distinct sequences.
// ---------------------------------------------------------------------------

// scorePoint returns the status band, current value, baseline value, and
// magnitude for point index i under the given metric's pattern. The
// magnitude is the absolute deviation from the baseline in the metric's
// native units — runtime detector logic would compute z-scores; this
// seed exposes a simpler shape so the heatmap UI has something to bin.
func scorePoint(m syntheticMetric, i int, rng *rand.Rand) (
	drift.DriftSeriesPointStatus, float64, float64, float64,
) {
	switch m.Pattern {
	case patternUnknownInsufficientData:
		return drift.DriftSeriesPointStatusUnknownInsufficientData, 0, 0, 0
	case patternUnknownDetectorError:
		// First 25 points healthy at the healthy-anchor value; last 5
		// flip to unknown_detector_error (baseline is still observed
		// because the detector failed *after* baseline establishment).
		baseline := patternBaseline(m)
		healthyValue := patternHealthyValue(m) + jitter(rng, 0.005)
		if i < syntheticPointCount-5 {
			return drift.DriftSeriesPointStatusHealthy, healthyValue, baseline,
				abs(healthyValue - baseline)
		}
		return drift.DriftSeriesPointStatusUnknownDetectorError, healthyValue, baseline,
			abs(healthyValue - baseline)
	case patternAllHealthy:
		baseline := patternBaseline(m)
		value := patternHealthyValue(m) + jitter(rng, 0.005)
		return drift.DriftSeriesPointStatusHealthy, value, baseline, abs(value - baseline)
	case patternWarningTrail:
		baseline := patternBaseline(m)
		var value float64
		// First 20 healthy, last 10 cross the warning threshold but stay
		// short of breached.
		if i < 20 {
			value = patternHealthyValue(m) + jitter(rng, 0.005)
		} else {
			// Linear walk into the warning band.
			steps := float64(i-19) / 10.0
			value = lerp(patternHealthyValue(m), patternWarningBandMidpoint(m), steps) + jitter(rng, 0.005)
		}
		return classifyPoint(m, value), value, baseline, abs(value - baseline)
	case patternBreachedTrail:
		baseline := patternBaseline(m)
		var value float64
		switch {
		case i < 15:
			value = patternHealthyValue(m) + jitter(rng, 0.005)
		case i < 25:
			steps := float64(i-14) / 10.0
			value = lerp(patternHealthyValue(m), patternWarningBandMidpoint(m), steps) + jitter(rng, 0.005)
		default:
			steps := float64(i-24) / 5.0
			value = lerp(patternWarningBandMidpoint(m), patternBreachedAnchor(m), steps) + jitter(rng, 0.005)
		}
		return classifyPoint(m, value), value, baseline, abs(value - baseline)
	default:
		return drift.DriftSeriesPointStatusHealthy, 0, 0, 0
	}
}

// classifyPoint maps a value to one of the three classifiable bands
// (healthy / warning / breached). The two unknown bands are produced
// only by their dedicated patterns.
func classifyPoint(m syntheticMetric, value float64) drift.DriftSeriesPointStatus {
	if m.ThresholdDirection == drift.ThresholdDirectionDescending {
		switch {
		case value < m.BreachedThreshold:
			return drift.DriftSeriesPointStatusBreached
		case value < m.WarningThreshold:
			return drift.DriftSeriesPointStatusWarning
		default:
			return drift.DriftSeriesPointStatusHealthy
		}
	}
	// Ascending: higher is worse.
	switch {
	case value > m.BreachedThreshold:
		return drift.DriftSeriesPointStatusBreached
	case value > m.WarningThreshold:
		return drift.DriftSeriesPointStatusWarning
	default:
		return drift.DriftSeriesPointStatusHealthy
	}
}

// patternHealthyValue returns a value comfortably in the healthy band
// for the metric's threshold direction.
func patternHealthyValue(m syntheticMetric) float64 {
	if m.ThresholdDirection == drift.ThresholdDirectionDescending {
		// Healthy is above warning; pick 1.05× warning as a safe anchor.
		return m.WarningThreshold * 1.05
	}
	// Ascending healthy is below warning; pick 0.5× warning.
	return m.WarningThreshold * 0.5
}

// patternWarningBandMidpoint returns a value between the warning and
// breached thresholds — the centre of the warning band.
func patternWarningBandMidpoint(m syntheticMetric) float64 {
	return (m.WarningThreshold + m.BreachedThreshold) / 2.0
}

// patternBreachedAnchor returns a value past the breached threshold.
func patternBreachedAnchor(m syntheticMetric) float64 {
	if m.ThresholdDirection == drift.ThresholdDirectionDescending {
		return m.BreachedThreshold * 0.90
	}
	return m.BreachedThreshold * 1.10
}

// patternBaseline returns the steady-state baseline value used in
// baseline_stats. For demo purposes baseline == healthy anchor; the
// magnitude column is always (value - baseline) in absolute terms.
func patternBaseline(m syntheticMetric) float64 {
	return patternHealthyValue(m)
}

// rollupStatus returns the DriftSeriesStatus implied by a pattern —
// always the latest band the pattern produces. The aggregation worker
// (Drift-3b) will compute this from points at runtime; here we hard-code
// the answer so the read APIs are consistent with the point data.
func rollupStatus(p syntheticPattern) drift.DriftSeriesStatus {
	switch p {
	case patternAllHealthy:
		return drift.DriftSeriesStatusHealthy
	case patternWarningTrail:
		return drift.DriftSeriesStatusWarning
	case patternBreachedTrail:
		return drift.DriftSeriesStatusBreached
	case patternUnknownInsufficientData:
		return drift.DriftSeriesStatusUnknownInsufficientData
	case patternUnknownDetectorError:
		return drift.DriftSeriesStatusUnknownDetectorError
	default:
		return drift.DriftSeriesStatusHealthy
	}
}

// pickObservationPointIndex chooses the point a single per-series
// observation is pinned to: for warning/breached/unknown_detector_error
// patterns it is the latest "interesting" point; for healthy and
// unknown_insufficient_data it returns -1 (no observation emitted).
func pickObservationPointIndex(p syntheticPattern) int {
	switch p {
	case patternWarningTrail, patternBreachedTrail:
		return syntheticPointCount - 1
	case patternUnknownDetectorError:
		return syntheticPointCount - 5
	default:
		return -1
	}
}

// unitForMetric returns a human-readable unit label for the metric's
// summary_stats payload. Heatmap UI tooltips can render this without
// case-analysing the drift type.
func unitForMetric(m syntheticMetric) string {
	switch m.DriftType {
	case drift.DriftTypeLatency:
		return "ms"
	case drift.DriftTypeInvocation:
		if m.ThresholdDirection == drift.ThresholdDirectionAscending {
			return "invocations_per_day"
		}
		return "ratio"
	default:
		return "ratio"
	}
}

// jitter returns a small deterministic perturbation in [-amount, +amount].
// Per-series RNG provides distinct sequences across series.
func jitter(rng *rand.Rand, amount float64) float64 {
	return (rng.Float64()*2 - 1) * amount
}

// lerp linearly interpolates from a to b at t in [0,1].
func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// abs returns the absolute value of x.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// stableSeed hashes a string into an int64 seed suitable for
// rand.NewSource. FNV-1a 64-bit is stdlib and produces a stable hash
// across architectures and Go versions.
func stableSeed(s string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return int64(h.Sum64()) //nolint:gosec // not a security boundary
}
