package drift

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// kebabCase matches a non-empty kebab-case identifier: lowercase
// letters or digits, separated by single hyphens. Mirrors the shape
// used by failmode and other governed-resource ID conventions.
var kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// validDriftTypes is the closed V1 enumeration. Population, data,
// prediction, performance, and concept drift are deferred to V2 and
// MUST be rejected by the validator.
var validDriftTypes = map[DriftType]struct{}{
	DriftTypeInvocation:  {},
	DriftTypeOutcome:     {},
	DriftTypeConfidence:  {},
	DriftTypeLatency:     {},
	DriftTypeEvidence:    {},
	DriftTypeAuthority:   {},
	DriftTypePolicy:      {},
	DriftTypeCoverage:    {},
	DriftTypeProcessPath: {},
}

// excludedDriftTypes lists the V2 / out-of-scope drift kinds. Each is
// rejected by Validate with an explicit message so a dropped check
// elsewhere does not let one through silently.
var excludedDriftTypes = map[DriftType]struct{}{
	"population":  {},
	"data":        {},
	"prediction":  {},
	"performance": {},
	"concept":     {},
}

// validDefinitionStatuses is the closed enumeration accepted by
// Validate for DriftDefinition.Status.
var validDefinitionStatuses = map[DriftDefinitionStatus]struct{}{
	DriftDefinitionStatusDraft:      {},
	DriftDefinitionStatusReview:     {},
	DriftDefinitionStatusActive:     {},
	DriftDefinitionStatusDeprecated: {},
	DriftDefinitionStatusRetired:    {},
}

// validBandStatuses is the shared five-band enumeration. The three
// public types DriftSeriesStatus, DriftSeriesPointStatus, and
// DriftObservationDetectorStatus all carry these underlying string
// values; the validators index this map by string.
var validBandStatuses = map[string]struct{}{
	"healthy":                   {},
	"warning":                   {},
	"breached":                  {},
	"unknown_insufficient_data": {},
	"unknown_detector_error":    {},
}

var validComputationModes = map[DriftPointComputationMode]struct{}{
	DriftPointComputationModeRealtime:   {},
	DriftPointComputationModeBackfilled: {},
	DriftPointComputationModeCorrected:  {},
	DriftPointComputationModeImported:   {},
}

var validOperatorStatuses = map[DriftObservationOperatorStatus]struct{}{
	DriftObservationOperatorStatusOpen:       {},
	DriftObservationOperatorStatusTriaged:    {},
	DriftObservationOperatorStatusResolved:   {},
	DriftObservationOperatorStatusAccepted:   {},
	DriftObservationOperatorStatusSuperseded: {},
}

var validAnnotationStatuses = map[DriftAnnotationStatus]struct{}{
	DriftAnnotationStatusActive:     {},
	DriftAnnotationStatusSuperseded: {},
}

var validAnnotationTargetKinds = map[DriftAnnotationTargetKind]struct{}{
	DriftAnnotationTargetKindSeries:      {},
	DriftAnnotationTargetKindObservation: {},
}

var validAnnotationTypes = map[DriftAnnotationType]struct{}{
	DriftAnnotationTypeKnownBusinessChange: {},
	DriftAnnotationTypeSuppression:         {},
	DriftAnnotationTypeRemediationNote:     {},
	DriftAnnotationTypeAcknowledgement:     {},
}

// validBaselineStrategies is the V1 enumeration. Champion-challenger
// and seasonality-aware are deferred to V2 and rejected here.
var validBaselineStrategies = map[BaselineStrategy]struct{}{
	BaselineStrategyRolling:                 {},
	BaselineStrategyFixedGoverned:           {},
	BaselineStrategyPreviousEquivalent:      {},
	BaselineStrategySinceLastGovernedChange: {},
	BaselineStrategyManuallyPinned:          {},
}

var validCadences = map[Cadence]struct{}{
	CadenceMinute: {},
	CadenceHour:   {},
	CadenceDay:    {},
	CadenceWeek:   {},
}

var validOrigins = map[DriftOrigin]struct{}{
	DriftOriginManual:           {},
	DriftOriginInferred:         {},
	DriftOriginAutoInstrumented: {},
}

var validTargetEntityKinds = map[TargetEntityKind]struct{}{
	TargetEntityKindBusinessService:  {},
	TargetEntityKindCapability:       {},
	TargetEntityKindProcess:          {},
	TargetEntityKindDecisionSurface:  {},
	TargetEntityKindAISystem:         {},
	TargetEntityKindAISystemBinding:  {},
	TargetEntityKindAgent:            {},
	TargetEntityKindAuthorityProfile: {},
	TargetEntityKindAuthorityGrant:   {},
}

var validThresholdDirections = map[ThresholdDirection]struct{}{
	ThresholdDirectionAscending:  {},
	ThresholdDirectionDescending: {},
}

// Validate returns the set of validation errors for a DriftDefinition.
// A nil error slice (len == 0) indicates the definition is well-formed
// under the V1 invariants.
//
// Callers (control-plane mappers, tests) invoke Validate explicitly.
// Repositories do not call Validate; they trust the caller to validate
// before persisting and rely on the database CHECK constraints (added
// in Drift-1b) as the integrity backstop.
func Validate(d *DriftDefinition) []error {
	if d == nil {
		return []error{errors.New("definition must not be nil")}
	}

	var errs []error

	if d.ID == "" {
		errs = append(errs, errors.New("ID must not be empty"))
	} else if !kebabCase.MatchString(d.ID) {
		errs = append(errs, fmt.Errorf("ID %q must be kebab-case", d.ID))
	}

	if d.Version <= 0 {
		errs = append(errs, fmt.Errorf("Version must be > 0, got %d", d.Version))
	}

	if d.Name == "" {
		errs = append(errs, errors.New("Name must not be empty"))
	}

	if d.BusinessOwner == "" {
		errs = append(errs, errors.New("BusinessOwner must not be empty"))
	}
	if d.TechnicalOwner == "" {
		errs = append(errs, errors.New("TechnicalOwner must not be empty"))
	}

	if _, ok := validDefinitionStatuses[d.Status]; !ok {
		errs = append(errs, fmt.Errorf("Status %q is not one of draft|review|active|deprecated|retired", d.Status))
	}

	if d.EffectiveDate.IsZero() {
		errs = append(errs, errors.New("EffectiveDate must not be zero"))
	}
	if d.EffectiveUntil != nil && !d.EffectiveDate.IsZero() && !d.EffectiveUntil.After(d.EffectiveDate) {
		errs = append(errs, errors.New("EffectiveUntil must be after EffectiveDate"))
	}
	if d.RetiredAt != nil && !d.EffectiveDate.IsZero() && d.RetiredAt.Before(d.EffectiveDate) {
		errs = append(errs, errors.New("RetiredAt must not be before EffectiveDate"))
	}

	if _, ok := validOrigins[d.Origin]; !ok {
		errs = append(errs, fmt.Errorf("Origin %q is not one of manual|inferred|auto_instrumented", d.Origin))
	}

	if d.Replaces != "" && d.Replaces == d.ID {
		errs = append(errs, errors.New("Replaces must not equal ID (no self-reference)"))
	}

	if _, ok := validTargetEntityKinds[d.TargetEntityKind]; !ok {
		errs = append(errs, fmt.Errorf("TargetEntityKind %q is not a valid kind", d.TargetEntityKind))
	}
	if d.TargetEntityID == "" {
		errs = append(errs, errors.New("TargetEntityID must not be empty"))
	}

	if d.CreatedAt.IsZero() {
		errs = append(errs, errors.New("CreatedAt must not be zero"))
	}
	if d.UpdatedAt.IsZero() {
		errs = append(errs, errors.New("UpdatedAt must not be zero"))
	}
	if d.CreatedBy == "" {
		errs = append(errs, errors.New("CreatedBy must not be empty"))
	}

	errs = append(errs, validateMetrics(d.Metrics)...)

	return errs
}

// validateMetrics enforces the metric-set-level invariants on a
// DriftDefinition: at-least-one entry, no duplicate MetricID, every
// metric individually well-formed.
func validateMetrics(metrics []DriftMetricDefinition) []error {
	var errs []error

	if len(metrics) == 0 {
		errs = append(errs, errors.New("Metrics must contain at least one DriftMetricDefinition"))
	}

	seen := make(map[string]struct{}, len(metrics))
	for i, m := range metrics {
		if m.MetricID != "" {
			if _, dup := seen[m.MetricID]; dup {
				errs = append(errs, fmt.Errorf("Metrics[%d]: duplicate MetricID %q", i, m.MetricID))
			} else {
				seen[m.MetricID] = struct{}{}
			}
		}
		for _, e := range ValidateMetric(m) {
			errs = append(errs, fmt.Errorf("Metrics[%d]: %w", i, e))
		}
	}

	return errs
}

// ValidateMetric returns validation errors for a single
// DriftMetricDefinition. Exported so test fixtures and later mappers
// can validate metrics in isolation.
func ValidateMetric(m DriftMetricDefinition) []error {
	var errs []error

	if m.MetricID == "" {
		errs = append(errs, errors.New("MetricID must not be empty"))
	} else if !kebabCase.MatchString(m.MetricID) {
		errs = append(errs, fmt.Errorf("MetricID %q must be kebab-case", m.MetricID))
	}

	if _, excluded := excludedDriftTypes[m.DriftType]; excluded {
		errs = append(errs, fmt.Errorf(
			"DriftType %q is deferred to V2 and not admitted by Drift-1a "+
				"(invocation|outcome|confidence|latency|evidence|authority|policy|coverage|process_path)",
			m.DriftType,
		))
	} else if _, ok := validDriftTypes[m.DriftType]; !ok {
		errs = append(errs, fmt.Errorf(
			"DriftType %q is not one of "+
				"invocation|outcome|confidence|latency|evidence|authority|policy|coverage|process_path",
			m.DriftType,
		))
	}

	if _, ok := validBaselineStrategies[m.BaselineStrategy]; !ok {
		errs = append(errs, fmt.Errorf(
			"BaselineStrategy %q is not one of V1 strategies "+
				"(rolling|fixed_governed|previous_equivalent|since_last_governed_change|manually_pinned)",
			m.BaselineStrategy,
		))
	}

	if m.WindowSeconds <= 0 {
		errs = append(errs, fmt.Errorf("WindowSeconds must be > 0, got %d", m.WindowSeconds))
	}
	if m.BaselineWindowSeconds < 0 {
		errs = append(errs, fmt.Errorf("BaselineWindowSeconds must be >= 0, got %d", m.BaselineWindowSeconds))
	}

	if _, ok := validCadences[m.Cadence]; !ok {
		errs = append(errs, fmt.Errorf("Cadence %q is not one of minute|hour|day|week", m.Cadence))
	}

	if _, ok := validThresholdDirections[m.ThresholdDirection]; !ok {
		errs = append(errs, fmt.Errorf("ThresholdDirection %q must be ascending or descending", m.ThresholdDirection))
	} else {
		switch m.ThresholdDirection {
		case ThresholdDirectionAscending:
			if !(m.WarningThreshold < m.BreachedThreshold) {
				errs = append(errs, fmt.Errorf(
					"ThresholdDirection ascending requires WarningThreshold < BreachedThreshold; got warning=%v breached=%v",
					m.WarningThreshold, m.BreachedThreshold,
				))
			}
		case ThresholdDirectionDescending:
			if !(m.WarningThreshold > m.BreachedThreshold) {
				errs = append(errs, fmt.Errorf(
					"ThresholdDirection descending requires WarningThreshold > BreachedThreshold; got warning=%v breached=%v",
					m.WarningThreshold, m.BreachedThreshold,
				))
			}
		}
	}

	if m.GovernanceExpectationVer < 0 {
		errs = append(errs, fmt.Errorf("GovernanceExpectationVer must be >= 0, got %d", m.GovernanceExpectationVer))
	}

	return errs
}

// ValidateSeries returns validation errors for a DriftSeries. Series
// are system-managed; validation guards self-consistency of the
// continuity-group fields and structural coherence.
func ValidateSeries(s *DriftSeries) []error {
	if s == nil {
		return []error{errors.New("series must not be nil")}
	}

	var errs []error

	if s.ID == "" {
		errs = append(errs, errors.New("ID must not be empty"))
	}
	if s.DefinitionID == "" {
		errs = append(errs, errors.New("DefinitionID must not be empty"))
	}
	if s.DefinitionVersion <= 0 {
		errs = append(errs, fmt.Errorf("DefinitionVersion must be > 0, got %d", s.DefinitionVersion))
	}
	if s.MetricID == "" {
		errs = append(errs, errors.New("MetricID must not be empty"))
	}
	if _, ok := validCadences[s.Cadence]; !ok {
		errs = append(errs, fmt.Errorf("Cadence %q is not one of minute|hour|day|week", s.Cadence))
	}
	if _, ok := validBandStatuses[string(s.Status)]; !ok {
		errs = append(errs, fmt.Errorf(
			"Status %q is not one of "+
				"healthy|warning|breached|unknown_insufficient_data|unknown_detector_error",
			s.Status,
		))
	}

	if s.ContinuityGroupID == "" {
		errs = append(errs, errors.New("ContinuityGroupID must not be empty"))
	}
	if s.PreviousSeriesID != "" && s.PreviousSeriesID == s.ID {
		errs = append(errs, errors.New("PreviousSeriesID must not equal own ID"))
	}
	if s.SupersededBySeriesID != "" && s.SupersededBySeriesID == s.ID {
		errs = append(errs, errors.New("SupersededBySeriesID must not equal own ID"))
	}
	if s.PreviousSeriesID != "" && s.SupersededBySeriesID != "" && s.PreviousSeriesID == s.SupersededBySeriesID {
		errs = append(errs, errors.New("PreviousSeriesID and SupersededBySeriesID must not be equal"))
	}
	if s.SealedAt != nil && !s.CreatedAt.IsZero() && s.SealedAt.Before(s.CreatedAt) {
		errs = append(errs, errors.New("SealedAt must not be before CreatedAt"))
	}

	return errs
}

// ValidatePoint returns validation errors for a DriftSeriesPoint.
// Computation-provenance coherence: BackfillRunID is required when
// ComputationMode is backfilled. Backfill state MUST NOT be encoded
// on Status.
func ValidatePoint(p *DriftSeriesPoint) []error {
	if p == nil {
		return []error{errors.New("point must not be nil")}
	}

	var errs []error

	if p.ID == "" {
		errs = append(errs, errors.New("ID must not be empty"))
	}
	if p.SeriesID == "" {
		errs = append(errs, errors.New("SeriesID must not be empty"))
	}

	if p.WindowStart.IsZero() || p.WindowEnd.IsZero() {
		errs = append(errs, errors.New("WindowStart and WindowEnd must not be zero"))
	} else if !p.WindowStart.Before(p.WindowEnd) {
		errs = append(errs, errors.New("WindowStart must be before WindowEnd"))
	}

	if p.SampleCount < 0 {
		errs = append(errs, fmt.Errorf("SampleCount must be >= 0, got %d", p.SampleCount))
	}

	if _, ok := validBandStatuses[string(p.Status)]; !ok {
		errs = append(errs, fmt.Errorf(
			"Status %q is not one of "+
				"healthy|warning|breached|unknown_insufficient_data|unknown_detector_error",
			p.Status,
		))
	}

	// BaselineWindowID is required UNLESS Status is
	// unknown_insufficient_data (in which case the detector legitimately
	// has no baseline to compare against).
	if p.Status != DriftSeriesPointStatusUnknownInsufficientData && p.BaselineWindowID == "" {
		errs = append(errs, errors.New(
			"BaselineWindowID must not be empty unless Status is unknown_insufficient_data",
		))
	}

	if _, ok := validComputationModes[p.ComputationMode]; !ok {
		errs = append(errs, fmt.Errorf(
			"ComputationMode %q is not one of realtime|backfilled|corrected|imported",
			p.ComputationMode,
		))
	}
	if p.ComputedAt.IsZero() {
		errs = append(errs, errors.New("ComputedAt must not be zero"))
	}
	if p.ComputationMode == DriftPointComputationModeBackfilled && p.BackfillRunID == "" {
		errs = append(errs, errors.New(
			"BackfillRunID must not be empty when ComputationMode is backfilled",
		))
	}

	return errs
}

// ValidateObservation returns validation errors for a DriftObservation.
// Backfill-flag coherence: BackfillRunID is required when Backfilled is
// true.
func ValidateObservation(o *DriftObservation) []error {
	if o == nil {
		return []error{errors.New("observation must not be nil")}
	}

	var errs []error

	if o.ID == "" {
		errs = append(errs, errors.New("ID must not be empty"))
	}
	if o.DefinitionID == "" {
		errs = append(errs, errors.New("DefinitionID must not be empty"))
	}
	if o.DefinitionVersion <= 0 {
		errs = append(errs, fmt.Errorf("DefinitionVersion must be > 0, got %d", o.DefinitionVersion))
	}
	if o.SeriesID == "" {
		errs = append(errs, errors.New("SeriesID must not be empty"))
	}
	if o.PointID == "" {
		errs = append(errs, errors.New("PointID must not be empty"))
	}

	if _, ok := validTargetEntityKinds[o.TargetEntityKind]; !ok {
		errs = append(errs, fmt.Errorf("TargetEntityKind %q is not a valid kind", o.TargetEntityKind))
	}
	if o.TargetEntityID == "" {
		errs = append(errs, errors.New("TargetEntityID must not be empty"))
	}

	if _, excluded := excludedDriftTypes[o.DriftType]; excluded {
		errs = append(errs, fmt.Errorf(
			"DriftType %q is deferred to V2 and not admitted by Drift-1a", o.DriftType,
		))
	} else if _, ok := validDriftTypes[o.DriftType]; !ok {
		errs = append(errs, fmt.Errorf("DriftType %q is not one of the nine V1 values", o.DriftType))
	}

	if _, ok := validBandStatuses[string(o.DetectorStatus)]; !ok {
		errs = append(errs, fmt.Errorf(
			"DetectorStatus %q is not one of "+
				"healthy|warning|breached|unknown_insufficient_data|unknown_detector_error",
			o.DetectorStatus,
		))
	}
	if _, ok := validOperatorStatuses[o.OperatorStatus]; !ok {
		errs = append(errs, fmt.Errorf(
			"OperatorStatus %q is not one of open|triaged|resolved|accepted|superseded",
			o.OperatorStatus,
		))
	}

	if o.ObservedWindowStart.IsZero() || o.ObservedWindowEnd.IsZero() {
		errs = append(errs, errors.New("ObservedWindowStart and ObservedWindowEnd must not be zero"))
	} else if !o.ObservedWindowStart.Before(o.ObservedWindowEnd) {
		errs = append(errs, errors.New("ObservedWindowStart must be before ObservedWindowEnd"))
	}

	if o.DetectedAt.IsZero() {
		errs = append(errs, errors.New("DetectedAt must not be zero"))
	}
	if o.EmittedAt.IsZero() {
		errs = append(errs, errors.New("EmittedAt must not be zero"))
	}

	if o.Backfilled && o.BackfillRunID == "" {
		errs = append(errs, errors.New("BackfillRunID must not be empty when Backfilled is true"))
	}

	if o.CorrectionOf != "" && o.CorrectionOf == o.ID {
		errs = append(errs, errors.New("CorrectionOf must not equal own ID (no self-correction)"))
	}

	return errs
}

// ValidateAnnotation returns validation errors for a DriftAnnotation.
// SuppressionUntil, when set, must be in the future as observed by
// the supplied now timestamp. Tests inject a fixed now to make the
// check deterministic; callers in production pass time.Now().
func ValidateAnnotation(a *DriftAnnotation, now time.Time) []error {
	if a == nil {
		return []error{errors.New("annotation must not be nil")}
	}

	var errs []error

	if a.ID == "" {
		errs = append(errs, errors.New("ID must not be empty"))
	}

	if _, ok := validAnnotationTargetKinds[a.TargetKind]; !ok {
		errs = append(errs, fmt.Errorf("TargetKind %q must be series or observation", a.TargetKind))
	}
	if a.TargetID == "" {
		errs = append(errs, errors.New("TargetID must not be empty"))
	}

	if _, ok := validAnnotationTypes[a.AnnotationType]; !ok {
		errs = append(errs, fmt.Errorf(
			"AnnotationType %q is not one of "+
				"known_business_change|suppression|remediation_note|acknowledgement",
			a.AnnotationType,
		))
	}

	if _, ok := validAnnotationStatuses[a.Status]; !ok {
		errs = append(errs, fmt.Errorf("Status %q must be active or superseded", a.Status))
	}

	if a.Body == "" {
		errs = append(errs, errors.New("Body must not be empty"))
	}

	if a.SuppressionUntil != nil && !a.SuppressionUntil.After(now) {
		errs = append(errs, errors.New("SuppressionUntil must be in the future"))
	}

	if a.SupersededByID != "" && a.SupersededByID == a.ID {
		errs = append(errs, errors.New("SupersededByID must not equal own ID"))
	}

	if a.AuthorID == "" {
		errs = append(errs, errors.New("AuthorID must not be empty"))
	}

	return errs
}
