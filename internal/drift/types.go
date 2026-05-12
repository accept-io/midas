// Package drift defines the Drift first-class governance structural
// entities introduced in Drift-1a.
//
// Drift in MIDAS is a governance-native primitive: a measurable change
// over time in the behaviour, usage, confidence, outcomes, evidence,
// policy path, population exposure, or governance posture of a governed
// entity (DecisionSurface, AISystemBinding, AISystem, Process,
// Capability, BusinessService, Agent, AuthorityProfile, AuthorityGrant).
//
// Layering: this package is structural domain, not runtime. Production
// code in internal/drift must not import internal/decision. Repositories
// declared here have no callers outside tests in Drift-1a; the runtime
// path is unchanged. Persistence, control-plane Kind, HTTP, ingestion,
// aggregation, detection, and Explorer integration are scoped to later
// tranches (Drift-1b through Drift-6).
//
// V1 taxonomy. Drift-1a admits exactly nine drift types: invocation,
// outcome, confidence, latency, evidence, authority, policy, coverage,
// process_path. Population, data, prediction, performance, and concept
// drift are deferred to V2 and rejected outright by the validator.
//
// Severity bands. DriftSeriesPoint and DriftObservation expose five
// status bands: healthy, warning, breached, unknown_insufficient_data,
// unknown_detector_error. There is deliberately no single "unknown"
// value — operators triage insufficient_data and detector_error
// differently, so the split is enforced at the type level.
//
// Backfill / computation provenance. Backfill state is encoded on a
// separate DriftPointComputationMode enum, not on any detector-status
// enum. DriftSeriesPoint carries ComputationMode, ComputedAt,
// BackfillRunID, and SourceWindowComplete; DriftObservation carries
// Backfilled, BackfillRunID, ObservedWindowStart/End, DetectedAt, and
// EmittedAt.
//
// Revision semantics. DriftDefinition revisions are atomic: any change
// to any embedded DriftMetricDefinition requires a new (ID, Version+1)
// of the parent definition. DriftMetricDefinition has no independent
// lifecycle, no independent approval, and no independent audit. The
// helper RevisionTransitionPlan models the deprecate-on-activate
// semantics that later persistence and control-plane tranches will
// apply atomically.
//
// Series continuity. DriftSeries carries ContinuityGroupID,
// PreviousSeriesID, SupersededBySeriesID, CutoverAt, and SealedAt
// fields so that compatible definition revisions can be stitched
// visually in later tranches. Drift-1a does not implement stitching
// logic; the fields exist so the schema is right when later tranches
// need them.
package drift

import "time"

// DriftType is the closed enumeration of V1-supported drift kinds.
//
// V1 admits exactly the nine values listed below. Population, data,
// prediction, performance, and concept drift are deferred to V2 and
// MUST be rejected by Validate. The exclusions are explicit (not
// "anything not listed is rejected") because dropped checks elsewhere
// in the system would otherwise let them through silently.
type DriftType string

const (
	DriftTypeInvocation  DriftType = "invocation"
	DriftTypeOutcome     DriftType = "outcome"
	DriftTypeConfidence  DriftType = "confidence"
	DriftTypeLatency     DriftType = "latency"
	DriftTypeEvidence    DriftType = "evidence"
	DriftTypeAuthority   DriftType = "authority"
	DriftTypePolicy      DriftType = "policy"
	DriftTypeCoverage    DriftType = "coverage"
	DriftTypeProcessPath DriftType = "process_path"
)

// DriftDefinitionStatus is the lifecycle state of a DriftDefinition
// revision. Mirrors authority.AuthorityProfile / failmode.FailModePolicy
// (draft → review → active → deprecated → retired).
type DriftDefinitionStatus string

const (
	DriftDefinitionStatusDraft      DriftDefinitionStatus = "draft"
	DriftDefinitionStatusReview     DriftDefinitionStatus = "review"
	DriftDefinitionStatusActive     DriftDefinitionStatus = "active"
	DriftDefinitionStatusDeprecated DriftDefinitionStatus = "deprecated"
	DriftDefinitionStatusRetired    DriftDefinitionStatus = "retired"
)

// DriftSeriesStatus is the rolled-up status of a DriftSeries derived
// from its latest points. Five-band; identical value set to
// DriftSeriesPointStatus and DriftObservationDetectorStatus. The three
// types are distinct so call sites read clearly; their value sets are
// validated against a shared internal map.
type DriftSeriesStatus string

const (
	DriftSeriesStatusHealthy                 DriftSeriesStatus = "healthy"
	DriftSeriesStatusWarning                 DriftSeriesStatus = "warning"
	DriftSeriesStatusBreached                DriftSeriesStatus = "breached"
	DriftSeriesStatusUnknownInsufficientData DriftSeriesStatus = "unknown_insufficient_data"
	DriftSeriesStatusUnknownDetectorError    DriftSeriesStatus = "unknown_detector_error"
)

// DriftSeriesPointStatus is the per-window detector status. Five bands;
// the unknown state is deliberately split into insufficient_data (a
// "wait" condition) and detector_error (a "fix the detector"
// condition) — there is NO single "unknown" value.
type DriftSeriesPointStatus string

const (
	DriftSeriesPointStatusHealthy                 DriftSeriesPointStatus = "healthy"
	DriftSeriesPointStatusWarning                 DriftSeriesPointStatus = "warning"
	DriftSeriesPointStatusBreached                DriftSeriesPointStatus = "breached"
	DriftSeriesPointStatusUnknownInsufficientData DriftSeriesPointStatus = "unknown_insufficient_data"
	DriftSeriesPointStatusUnknownDetectorError    DriftSeriesPointStatus = "unknown_detector_error"
)

// DriftPointComputationMode separates backfill / correction provenance
// from detector status. Backfill state MUST NOT be encoded on any
// detector-status enum (status describes the detector result; this
// enum describes how the point was computed).
type DriftPointComputationMode string

const (
	DriftPointComputationModeRealtime   DriftPointComputationMode = "realtime"
	DriftPointComputationModeBackfilled DriftPointComputationMode = "backfilled"
	DriftPointComputationModeCorrected  DriftPointComputationMode = "corrected"
	DriftPointComputationModeImported   DriftPointComputationMode = "imported"
)

// DriftObservationDetectorStatus is the detector-side status carried on
// every DriftObservation. Same five bands as DriftSeriesPointStatus.
type DriftObservationDetectorStatus string

const (
	DriftObservationDetectorStatusHealthy                 DriftObservationDetectorStatus = "healthy"
	DriftObservationDetectorStatusWarning                 DriftObservationDetectorStatus = "warning"
	DriftObservationDetectorStatusBreached                DriftObservationDetectorStatus = "breached"
	DriftObservationDetectorStatusUnknownInsufficientData DriftObservationDetectorStatus = "unknown_insufficient_data"
	DriftObservationDetectorStatusUnknownDetectorError    DriftObservationDetectorStatus = "unknown_detector_error"
)

// DriftObservationOperatorStatus is the operator-triage status carried
// on every DriftObservation. Decoupled from the detector-side status —
// the detector's view does not change when the operator acknowledges.
type DriftObservationOperatorStatus string

const (
	DriftObservationOperatorStatusOpen       DriftObservationOperatorStatus = "open"
	DriftObservationOperatorStatusTriaged    DriftObservationOperatorStatus = "triaged"
	DriftObservationOperatorStatusResolved   DriftObservationOperatorStatus = "resolved"
	DriftObservationOperatorStatusAccepted   DriftObservationOperatorStatus = "accepted"
	DriftObservationOperatorStatusSuperseded DriftObservationOperatorStatus = "superseded"
)

// DriftAnnotationStatus is the lifecycle state of a single annotation.
// Annotations are append-only; supersession happens by adding a newer
// annotation that references the older one.
type DriftAnnotationStatus string

const (
	DriftAnnotationStatusActive     DriftAnnotationStatus = "active"
	DriftAnnotationStatusSuperseded DriftAnnotationStatus = "superseded"
)

// DriftAnnotationTargetKind is the kind of object a DriftAnnotation
// attaches to. An annotation targets either a series (long-lived
// commentary) or a single observation (event-specific commentary).
type DriftAnnotationTargetKind string

const (
	DriftAnnotationTargetKindSeries      DriftAnnotationTargetKind = "series"
	DriftAnnotationTargetKindObservation DriftAnnotationTargetKind = "observation"
)

// DriftAnnotationType discriminates the operator intent behind an
// annotation. Closed enumeration; new types require an explicit code
// change.
type DriftAnnotationType string

const (
	DriftAnnotationTypeKnownBusinessChange DriftAnnotationType = "known_business_change"
	DriftAnnotationTypeSuppression         DriftAnnotationType = "suppression"
	DriftAnnotationTypeRemediationNote     DriftAnnotationType = "remediation_note"
	DriftAnnotationTypeAcknowledgement     DriftAnnotationType = "acknowledgement"
)

// BaselineStrategy is the V1 enumeration of supported baseline
// strategies. Champion-challenger and seasonality-aware are deferred
// to V2 and are NOT admitted by the validator.
type BaselineStrategy string

const (
	BaselineStrategyRolling                 BaselineStrategy = "rolling"
	BaselineStrategyFixedGoverned           BaselineStrategy = "fixed_governed"
	BaselineStrategyPreviousEquivalent      BaselineStrategy = "previous_equivalent"
	BaselineStrategySinceLastGovernedChange BaselineStrategy = "since_last_governed_change"
	BaselineStrategyManuallyPinned          BaselineStrategy = "manually_pinned"
)

// Cadence is the aggregation cadence of a DriftSeries.
type Cadence string

const (
	CadenceMinute Cadence = "minute"
	CadenceHour   Cadence = "hour"
	CadenceDay    Cadence = "day"
	CadenceWeek   Cadence = "week"
)

// DriftOrigin records how a DriftDefinition came to exist. Mirrors the
// origin discriminator on FailModePolicy.
type DriftOrigin string

const (
	DriftOriginManual           DriftOrigin = "manual"
	DriftOriginInferred         DriftOrigin = "inferred"
	DriftOriginAutoInstrumented DriftOrigin = "auto_instrumented"
)

// TargetEntityKind is the kind of governed entity a DriftDefinition
// targets. The closed enumeration mirrors the existing structural
// catalogue.
type TargetEntityKind string

const (
	TargetEntityKindBusinessService  TargetEntityKind = "business_service"
	TargetEntityKindCapability       TargetEntityKind = "capability"
	TargetEntityKindProcess          TargetEntityKind = "process"
	TargetEntityKindDecisionSurface  TargetEntityKind = "decision_surface"
	TargetEntityKindAISystem         TargetEntityKind = "ai_system"
	TargetEntityKindAISystemBinding  TargetEntityKind = "ai_system_binding"
	TargetEntityKindAgent            TargetEntityKind = "agent"
	TargetEntityKindAuthorityProfile TargetEntityKind = "authority_profile"
	TargetEntityKindAuthorityGrant   TargetEntityKind = "authority_grant"
)

// ThresholdDirection determines how WarningThreshold and
// BreachedThreshold relate. Ascending means breached > warning (e.g.
// "latency p95 ms"); descending means breached < warning (e.g.
// "approve rate").
type ThresholdDirection string

const (
	ThresholdDirectionAscending  ThresholdDirection = "ascending"
	ThresholdDirectionDescending ThresholdDirection = "descending"
)

// DriftDefinition is a governed, versioned spec for what to measure
// on a target entity, against what baseline, by what detector, against
// what thresholds. Composite (ID, Version) primary key; lifecycle
// shape mirrors authority.AuthorityProfile and failmode.FailModePolicy.
//
// Atomic-revision semantics: any change to any embedded
// DriftMetricDefinition is a revision bump. DriftMetricDefinition has
// no independent lifecycle.
type DriftDefinition struct {
	ID          string
	Version     int
	Name        string
	Description string

	Status         DriftDefinitionStatus
	EffectiveDate  time.Time
	EffectiveUntil *time.Time
	RetiredAt      *time.Time

	BusinessOwner  string
	TechnicalOwner string

	TargetEntityKind TargetEntityKind
	TargetEntityID   string

	// Metrics is the embedded metric set. Atomic-revision invariant:
	// any add/remove/modify here requires a new DriftDefinition
	// revision. The validator enforces no-duplicate-MetricID and at
	// least one entry; resolvability of optional GovernanceExpectation
	// references is a later-tranche concern.
	Metrics []DriftMetricDefinition

	Origin   DriftOrigin
	Managed  bool
	Replaces string // optional logical predecessor; must not equal ID

	SuccessorDefinitionID string
	SuccessorVersion      int

	CreatedAt  time.Time
	UpdatedAt  time.Time
	CreatedBy  string
	ApprovedBy string
	ApprovedAt *time.Time
}

// DriftMetricDefinition is one named metric inside a DriftDefinition.
// Has no independent lifecycle, status, or approval — modifying any
// field here is a revision bump on the parent DriftDefinition.
//
// GovernanceExpectationRef and GovernanceExpectationVer are optional
// references to a GovernanceExpectation whose threshold the metric
// adopts. Drift-1a validates structural shape only; resolvability is
// a later-tranche concern.
type DriftMetricDefinition struct {
	MetricID string

	DriftType DriftType

	BaselineStrategy      BaselineStrategy
	BaselineWindowSeconds int
	WindowSeconds         int
	Cadence               Cadence

	WarningThreshold   float64
	BreachedThreshold  float64
	ThresholdDirection ThresholdDirection

	GovernanceExpectationRef string
	GovernanceExpectationVer int

	Description string
}

// DriftSeries is a single time-ordered stream of summary stats for one
// (DriftMetricDefinition × cadence) combination. System-managed.
//
// Continuity-group fields support visual stitching across compatible
// definition revisions in later tranches. Drift-1a does not implement
// stitching logic — the fields exist so the schema is right when later
// tranches need them.
type DriftSeries struct {
	ID string

	DefinitionID      string
	DefinitionVersion int
	MetricID          string

	Cadence Cadence
	Status  DriftSeriesStatus

	ContinuityGroupID    string
	PreviousSeriesID     string // optional; must not equal ID
	SupersededBySeriesID string // optional; must not equal ID
	CutoverAt            *time.Time
	SealedAt             *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DriftSeriesPoint is a single (window, summary, baseline, magnitude,
// status) tuple. Append-only; corrections produce a new point with
// ComputationMode == corrected.
type DriftSeriesPoint struct {
	ID       string
	SeriesID string

	WindowStart time.Time
	WindowEnd   time.Time

	SampleCount int64

	// SummaryStats / BaselineStats are opaque maps at the domain
	// layer. Ingestion and detection tranches will define typed
	// sub-shapes. JSONB-shaped at the (later) schema layer.
	SummaryStats  map[string]any
	BaselineStats map[string]any

	BaselineWindowID string

	Magnitude float64
	Status    DriftSeriesPointStatus

	ComputationMode      DriftPointComputationMode
	ComputedAt           time.Time
	BackfillRunID        string // required when ComputationMode is backfilled
	SourceWindowComplete bool

	ProvenanceEnvelopeIDs []string

	CreatedAt time.Time
}

// DriftObservation is the audit-chain-bearing record of a materially-
// significant detector result (typically a threshold crossing). The
// observation itself does not mutate runtime state. A future
// FailModePolicyResolver may consume observations as evidence inputs.
type DriftObservation struct {
	ID string

	DefinitionID      string
	DefinitionVersion int
	SeriesID          string
	PointID           string

	TargetEntityKind TargetEntityKind
	TargetEntityID   string

	DriftType DriftType

	Magnitude float64

	DetectorStatus DriftObservationDetectorStatus
	OperatorStatus DriftObservationOperatorStatus

	BaselineWindowID    string
	ObservedWindowStart time.Time
	ObservedWindowEnd   time.Time

	DetectedAt time.Time
	EmittedAt  time.Time

	Backfilled    bool
	BackfillRunID string // required when Backfilled is true

	EvidenceEnvelopeIDs []string

	RelatedFailModePolicyID  string // optional
	RelatedGovernanceExpRef  string // optional
	CorrectionOf             string // optional self-reference; must not equal ID

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DriftAnnotation is operator commentary attached to a series or
// observation. Append-only; supersession happens by adding a newer
// annotation that references the older one.
type DriftAnnotation struct {
	ID string

	TargetKind DriftAnnotationTargetKind
	TargetID   string

	AnnotationType DriftAnnotationType

	Body string

	SuppressionUntil *time.Time

	ReferenceEnvelopeIDs []string

	Status         DriftAnnotationStatus
	SupersededByID string // optional; must not equal own ID

	AuthorID string

	CreatedAt time.Time
	UpdatedAt time.Time
}
