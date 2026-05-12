package httpapi

// drift_response.go — wire-format DTOs and mappers for the Drift-1d
// read API. Conventions:
//
//   - List wrappers always include a non-null array (even empty) so
//     downstream consumers don't have to defend against null.
//   - Nullable timestamps use *time.Time (zero in domain → null in
//     JSON via omitempty + pointer).
//   - SummaryStats / BaselineStats / *EnvelopeIDs are passed through
//     as-is; nil maps marshal to {} via the helper, nil slices to [].
//   - Domain enums are stringified into the wire shape; the OpenAPI
//     enum definition is the contract for valid values.

import (
	"time"

	"github.com/accept-io/midas/internal/drift"
)

// driftTargetRef is the discriminated-reference shape used in the
// definition and observation responses.
type driftTargetRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// driftMetricResponse is the wire shape for an embedded metric.
type driftMetricResponse struct {
	MetricID                 string  `json:"metric_id"`
	DriftType                string  `json:"drift_type"`
	BaselineStrategy         string  `json:"baseline_strategy"`
	BaselineWindowSeconds    int     `json:"baseline_window_seconds"`
	WindowSeconds            int     `json:"window_seconds"`
	Cadence                  string  `json:"cadence"`
	WarningThreshold         float64 `json:"warning_threshold"`
	BreachedThreshold        float64 `json:"breached_threshold"`
	ThresholdDirection       string  `json:"threshold_direction"`
	GovernanceExpectationRef string  `json:"governance_expectation_ref"`
	GovernanceExpectationVer int     `json:"governance_expectation_ver"`
	Description              string  `json:"description"`
}

// driftDefinitionResponse is the wire shape for a single
// DriftDefinition revision.
type driftDefinitionResponse struct {
	ID                    string                `json:"id"`
	Version               int                   `json:"version"`
	Name                  string                `json:"name"`
	Description           string                `json:"description"`
	Status                string                `json:"status"`
	EffectiveDate         time.Time             `json:"effective_date"`
	EffectiveUntil        *time.Time            `json:"effective_until"`
	RetiredAt             *time.Time            `json:"retired_at"`
	BusinessOwner         string                `json:"business_owner"`
	TechnicalOwner        string                `json:"technical_owner"`
	Target                driftTargetRef        `json:"target"`
	Metrics               []driftMetricResponse `json:"metrics"`
	Origin                string                `json:"origin"`
	Managed               bool                  `json:"managed"`
	Replaces              string                `json:"replaces"`
	SuccessorDefinitionID string                `json:"successor_definition_id"`
	SuccessorVersion      int                   `json:"successor_version"`
	CreatedAt             time.Time             `json:"created_at"`
	UpdatedAt             time.Time             `json:"updated_at"`
	CreatedBy             string                `json:"created_by"`
	ApprovedBy            string                `json:"approved_by"`
	ApprovedAt            *time.Time            `json:"approved_at"`
}

type driftDefinitionListResponse struct {
	DriftDefinitions []driftDefinitionResponse `json:"drift_definitions"`
}

func toDriftMetricResponse(m drift.DriftMetricDefinition) driftMetricResponse {
	return driftMetricResponse{
		MetricID:                 m.MetricID,
		DriftType:                string(m.DriftType),
		BaselineStrategy:         string(m.BaselineStrategy),
		BaselineWindowSeconds:    m.BaselineWindowSeconds,
		WindowSeconds:            m.WindowSeconds,
		Cadence:                  string(m.Cadence),
		WarningThreshold:         m.WarningThreshold,
		BreachedThreshold:        m.BreachedThreshold,
		ThresholdDirection:       string(m.ThresholdDirection),
		GovernanceExpectationRef: m.GovernanceExpectationRef,
		GovernanceExpectationVer: m.GovernanceExpectationVer,
		Description:              m.Description,
	}
}

func toDriftDefinitionResponse(d *drift.DriftDefinition) driftDefinitionResponse {
	metrics := make([]driftMetricResponse, 0, len(d.Metrics))
	for _, m := range d.Metrics {
		metrics = append(metrics, toDriftMetricResponse(m))
	}
	return driftDefinitionResponse{
		ID:                    d.ID,
		Version:               d.Version,
		Name:                  d.Name,
		Description:           d.Description,
		Status:                string(d.Status),
		EffectiveDate:         d.EffectiveDate,
		EffectiveUntil:        d.EffectiveUntil,
		RetiredAt:             d.RetiredAt,
		BusinessOwner:         d.BusinessOwner,
		TechnicalOwner:        d.TechnicalOwner,
		Target:                driftTargetRef{Kind: string(d.TargetEntityKind), ID: d.TargetEntityID},
		Metrics:               metrics,
		Origin:                string(d.Origin),
		Managed:               d.Managed,
		Replaces:              d.Replaces,
		SuccessorDefinitionID: d.SuccessorDefinitionID,
		SuccessorVersion:      d.SuccessorVersion,
		CreatedAt:             d.CreatedAt,
		UpdatedAt:             d.UpdatedAt,
		CreatedBy:             d.CreatedBy,
		ApprovedBy:            d.ApprovedBy,
		ApprovedAt:            d.ApprovedAt,
	}
}

func toDriftDefinitionListResponse(items []*drift.DriftDefinition) driftDefinitionListResponse {
	out := make([]driftDefinitionResponse, 0, len(items))
	for _, d := range items {
		if d == nil {
			continue
		}
		out = append(out, toDriftDefinitionResponse(d))
	}
	return driftDefinitionListResponse{DriftDefinitions: out}
}

// driftSeriesResponse is the wire shape for a single DriftSeries.
type driftSeriesResponse struct {
	ID                   string     `json:"id"`
	DefinitionID         string     `json:"definition_id"`
	DefinitionVersion    int        `json:"definition_version"`
	MetricID             string     `json:"metric_id"`
	Cadence              string     `json:"cadence"`
	Status               string     `json:"status"`
	ContinuityGroupID    string     `json:"continuity_group_id"`
	PreviousSeriesID     string     `json:"previous_series_id"`
	SupersededBySeriesID string     `json:"superseded_by_series_id"`
	CutoverAt            *time.Time `json:"cutover_at"`
	SealedAt             *time.Time `json:"sealed_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type driftSeriesListResponse struct {
	DriftSeries []driftSeriesResponse `json:"drift_series"`
}

func toDriftSeriesResponse(s *drift.DriftSeries) driftSeriesResponse {
	return driftSeriesResponse{
		ID:                   s.ID,
		DefinitionID:         s.DefinitionID,
		DefinitionVersion:    s.DefinitionVersion,
		MetricID:             s.MetricID,
		Cadence:              string(s.Cadence),
		Status:               string(s.Status),
		ContinuityGroupID:    s.ContinuityGroupID,
		PreviousSeriesID:     s.PreviousSeriesID,
		SupersededBySeriesID: s.SupersededBySeriesID,
		CutoverAt:            s.CutoverAt,
		SealedAt:             s.SealedAt,
		CreatedAt:            s.CreatedAt,
		UpdatedAt:            s.UpdatedAt,
	}
}

func toDriftSeriesListResponse(items []*drift.DriftSeries) driftSeriesListResponse {
	out := make([]driftSeriesResponse, 0, len(items))
	for _, s := range items {
		if s == nil {
			continue
		}
		out = append(out, toDriftSeriesResponse(s))
	}
	return driftSeriesListResponse{DriftSeries: out}
}

// driftSeriesPointResponse is the wire shape for a single
// DriftSeriesPoint. SummaryStats / BaselineStats pass through as
// opaque maps; ProvenanceEnvelopeIDs is normalised to a non-nil slice.
type driftSeriesPointResponse struct {
	ID                    string         `json:"id"`
	SeriesID              string         `json:"series_id"`
	WindowStart           time.Time      `json:"window_start"`
	WindowEnd             time.Time      `json:"window_end"`
	SampleCount           int64          `json:"sample_count"`
	SummaryStats          map[string]any `json:"summary_stats"`
	BaselineStats         map[string]any `json:"baseline_stats"`
	BaselineWindowID      string         `json:"baseline_window_id"`
	Magnitude             float64        `json:"magnitude"`
	Status                string         `json:"status"`
	ComputationMode       string         `json:"computation_mode"`
	ComputedAt            time.Time      `json:"computed_at"`
	BackfillRunID         string         `json:"backfill_run_id"`
	SourceWindowComplete  bool           `json:"source_window_complete"`
	ProvenanceEnvelopeIDs []string       `json:"provenance_envelope_ids"`
	CreatedAt             time.Time      `json:"created_at"`
}

type driftSeriesPointListResponse struct {
	DriftSeriesPoints []driftSeriesPointResponse `json:"drift_series_points"`
}

func toDriftSeriesPointResponse(p *drift.DriftSeriesPoint) driftSeriesPointResponse {
	summary := p.SummaryStats
	if summary == nil {
		summary = map[string]any{}
	}
	baseline := p.BaselineStats
	if baseline == nil {
		baseline = map[string]any{}
	}
	provenance := p.ProvenanceEnvelopeIDs
	if provenance == nil {
		provenance = []string{}
	}
	return driftSeriesPointResponse{
		ID:                    p.ID,
		SeriesID:              p.SeriesID,
		WindowStart:           p.WindowStart,
		WindowEnd:             p.WindowEnd,
		SampleCount:           p.SampleCount,
		SummaryStats:          summary,
		BaselineStats:         baseline,
		BaselineWindowID:      p.BaselineWindowID,
		Magnitude:             p.Magnitude,
		Status:                string(p.Status),
		ComputationMode:       string(p.ComputationMode),
		ComputedAt:            p.ComputedAt,
		BackfillRunID:         p.BackfillRunID,
		SourceWindowComplete:  p.SourceWindowComplete,
		ProvenanceEnvelopeIDs: provenance,
		CreatedAt:             p.CreatedAt,
	}
}

func toDriftSeriesPointListResponse(items []*drift.DriftSeriesPoint) driftSeriesPointListResponse {
	out := make([]driftSeriesPointResponse, 0, len(items))
	for _, p := range items {
		if p == nil {
			continue
		}
		out = append(out, toDriftSeriesPointResponse(p))
	}
	return driftSeriesPointListResponse{DriftSeriesPoints: out}
}

// driftObservationResponse is the wire shape for a single
// DriftObservation.
type driftObservationResponse struct {
	ID                      string         `json:"id"`
	DefinitionID            string         `json:"definition_id"`
	DefinitionVersion       int            `json:"definition_version"`
	SeriesID                string         `json:"series_id"`
	PointID                 string         `json:"point_id"`
	Target                  driftTargetRef `json:"target"`
	DriftType               string         `json:"drift_type"`
	Magnitude               float64        `json:"magnitude"`
	DetectorStatus          string         `json:"detector_status"`
	OperatorStatus          string         `json:"operator_status"`
	BaselineWindowID        string         `json:"baseline_window_id"`
	ObservedWindowStart     time.Time      `json:"observed_window_start"`
	ObservedWindowEnd       time.Time      `json:"observed_window_end"`
	DetectedAt              time.Time      `json:"detected_at"`
	EmittedAt               time.Time      `json:"emitted_at"`
	Backfilled              bool           `json:"backfilled"`
	BackfillRunID           string         `json:"backfill_run_id"`
	EvidenceEnvelopeIDs     []string       `json:"evidence_envelope_ids"`
	RelatedFailModePolicyID string         `json:"related_fail_mode_policy_id"`
	RelatedGovernanceExpRef string         `json:"related_governance_exp_ref"`
	CorrectionOf            string         `json:"correction_of"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
}

type driftObservationListResponse struct {
	DriftObservations []driftObservationResponse `json:"drift_observations"`
}

func toDriftObservationResponse(o *drift.DriftObservation) driftObservationResponse {
	evidence := o.EvidenceEnvelopeIDs
	if evidence == nil {
		evidence = []string{}
	}
	return driftObservationResponse{
		ID:                      o.ID,
		DefinitionID:            o.DefinitionID,
		DefinitionVersion:       o.DefinitionVersion,
		SeriesID:                o.SeriesID,
		PointID:                 o.PointID,
		Target:                  driftTargetRef{Kind: string(o.TargetEntityKind), ID: o.TargetEntityID},
		DriftType:               string(o.DriftType),
		Magnitude:               o.Magnitude,
		DetectorStatus:          string(o.DetectorStatus),
		OperatorStatus:          string(o.OperatorStatus),
		BaselineWindowID:        o.BaselineWindowID,
		ObservedWindowStart:     o.ObservedWindowStart,
		ObservedWindowEnd:       o.ObservedWindowEnd,
		DetectedAt:              o.DetectedAt,
		EmittedAt:               o.EmittedAt,
		Backfilled:              o.Backfilled,
		BackfillRunID:           o.BackfillRunID,
		EvidenceEnvelopeIDs:     evidence,
		RelatedFailModePolicyID: o.RelatedFailModePolicyID,
		RelatedGovernanceExpRef: o.RelatedGovernanceExpRef,
		CorrectionOf:            o.CorrectionOf,
		CreatedAt:               o.CreatedAt,
		UpdatedAt:               o.UpdatedAt,
	}
}

func toDriftObservationListResponse(items []*drift.DriftObservation) driftObservationListResponse {
	out := make([]driftObservationResponse, 0, len(items))
	for _, o := range items {
		if o == nil {
			continue
		}
		out = append(out, toDriftObservationResponse(o))
	}
	return driftObservationListResponse{DriftObservations: out}
}

// driftAnnotationTargetRef discriminates whether the annotation
// targets a series or an observation.
type driftAnnotationTargetRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// driftAnnotationResponse is the wire shape for a single
// DriftAnnotation.
type driftAnnotationResponse struct {
	ID                   string                   `json:"id"`
	Target               driftAnnotationTargetRef `json:"target"`
	AnnotationType       string                   `json:"annotation_type"`
	Body                 string                   `json:"body"`
	SuppressionUntil     *time.Time               `json:"suppression_until"`
	ReferenceEnvelopeIDs []string                 `json:"reference_envelope_ids"`
	Status               string                   `json:"status"`
	SupersededByID       string                   `json:"superseded_by_id"`
	AuthorID             string                   `json:"author_id"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
}

type driftAnnotationListResponse struct {
	DriftAnnotations []driftAnnotationResponse `json:"drift_annotations"`
}

func toDriftAnnotationResponse(a *drift.DriftAnnotation) driftAnnotationResponse {
	references := a.ReferenceEnvelopeIDs
	if references == nil {
		references = []string{}
	}
	return driftAnnotationResponse{
		ID:                   a.ID,
		Target:               driftAnnotationTargetRef{Kind: string(a.TargetKind), ID: a.TargetID},
		AnnotationType:       string(a.AnnotationType),
		Body:                 a.Body,
		SuppressionUntil:     a.SuppressionUntil,
		ReferenceEnvelopeIDs: references,
		Status:               string(a.Status),
		SupersededByID:       a.SupersededByID,
		AuthorID:             a.AuthorID,
		CreatedAt:            a.CreatedAt,
		UpdatedAt:            a.UpdatedAt,
	}
}

func toDriftAnnotationListResponse(items []*drift.DriftAnnotation) driftAnnotationListResponse {
	out := make([]driftAnnotationResponse, 0, len(items))
	for _, a := range items {
		if a == nil {
			continue
		}
		out = append(out, toDriftAnnotationResponse(a))
	}
	return driftAnnotationListResponse{DriftAnnotations: out}
}
