package analytics

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

type DriftAnalyticsReadService interface {
	GetNodeAnalytics(ctx context.Context, req DriftAnalyticsRequest) (DriftAnalyticsResponse, error)
}

type DefinitionReader interface {
	ListByTarget(ctx context.Context, kind drift.TargetEntityKind, entityID string) ([]*drift.DriftDefinition, error)
}

type SeriesReader interface {
	ListByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftSeries, error)
}

type SeriesPointReader interface {
	ListBySeries(ctx context.Context, seriesID string, fromWindow time.Time, limit int) ([]*drift.DriftSeriesPoint, error)
}

type ObservationReader interface {
	ListBySeries(ctx context.Context, seriesID string) ([]*drift.DriftObservation, error)
}

type AnnotationReader interface {
	ListByTarget(ctx context.Context, kind drift.DriftAnnotationTargetKind, targetID string) ([]*drift.DriftAnnotation, error)
}

type Service struct {
	definitions  DefinitionReader
	series       SeriesReader
	seriesPoints SeriesPointReader
	observations ObservationReader
	annotations  AnnotationReader
	now          func() time.Time
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func NewService(
	definitions DefinitionReader,
	series SeriesReader,
	seriesPoints SeriesPointReader,
	observations ObservationReader,
	annotations AnnotationReader,
	opts ...Option,
) *Service {
	s := &Service{
		definitions:  definitions,
		series:       series,
		seriesPoints: seriesPoints,
		observations: observations,
		annotations:  annotations,
		now:          func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type DriftAnalyticsRequest struct {
	NodeKind string
	NodeID   string
	RangeKey string
}

type DriftAnalyticsResponse struct {
	Node                 NodeRef              `json:"node"`
	Range                RangeRef             `json:"range"`
	Chart                Chart                `json:"chart"`
	Provenance           Provenance           `json:"provenance"`
	SourceClassification SourceClassification `json:"sourceClassification"`
	ProjectionAsOf       *time.Time           `json:"projectionAsOf,omitempty"`
	DataAvailable        bool                 `json:"dataAvailable"`
}

type NodeRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type RangeRef struct {
	Key   string     `json:"key"`
	Label string     `json:"label"`
	From  *time.Time `json:"from,omitempty"`
	To    *time.Time `json:"to,omitempty"`
}

type Chart struct {
	MetricID      string        `json:"metricId,omitempty"`
	DriftType     string        `json:"driftType,omitempty"`
	SeriesID      string        `json:"seriesId,omitempty"`
	Observed      []ObservedPt  `json:"observed"`
	Expected      []ExpectedPt  `json:"expected"`
	Watch         []ExpectedPt  `json:"watch"`
	Breach        []ExpectedPt  `json:"breach"`
	CurrentValue  *float64      `json:"currentValue,omitempty"`
	CurrentStatus string        `json:"currentStatus,omitempty"`
	YDomain       NumericDomain `json:"yDomain"`
}

type ObservedPt struct {
	T         time.Time `json:"t"`
	Value     float64   `json:"value"`
	Status    string    `json:"status,omitempty"`
	Magnitude *float64  `json:"magnitude,omitempty"`
}

type ExpectedPt struct {
	T     time.Time `json:"t"`
	Value float64   `json:"value"`
}

type NumericDomain struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type Provenance struct {
	EnvelopeIDs        []string `json:"envelopeIds"`
	PointIDs           []string `json:"pointIds"`
	ObservationIDs     []string `json:"observationIds"`
	AnnotationIDs      []string `json:"annotationIds"`
	ProfileRefs        []string `json:"profileRefs"`
	PolicyRefs         []string `json:"policyRefs"`
	VerificationStatus string   `json:"verificationStatus"`
}

type SourceClassification struct {
	SelectedNode        string `json:"selectedNode"`
	ObservedSeries      string `json:"observedSeries"`
	ExpectedBaseline    string `json:"expectedBaseline"`
	Thresholds          string `json:"thresholds"`
	Status              string `json:"status"`
	Provenance          string `json:"provenance"`
	CompositeScore      string `json:"compositeScore"`
	ContributionValues  string `json:"contributionValues"`
	ContributionWeights string `json:"contributionWeights"`
	GraphOverlay        string `json:"graphOverlay"`
}

var (
	ErrInvalidRequest = errors.New("invalid drift analytics request")
	ErrNotConfigured  = errors.New("drift analytics read service not configured")
)

func (s *Service) GetNodeAnalytics(ctx context.Context, req DriftAnalyticsRequest) (DriftAnalyticsResponse, error) {
	resp := emptyResponse(req)
	if strings.TrimSpace(req.NodeKind) == "" || strings.TrimSpace(req.NodeID) == "" {
		return resp, ErrInvalidRequest
	}
	kind := drift.TargetEntityKind(strings.TrimSpace(req.NodeKind))
	if !isValidTargetKind(kind) {
		return resp, ErrInvalidRequest
	}
	if s == nil || s.definitions == nil || s.series == nil || s.seriesPoints == nil {
		return resp, ErrNotConfigured
	}
	resp.Node = NodeRef{Kind: string(kind), ID: strings.TrimSpace(req.NodeID)}
	resp.Range = s.resolveRange(req.RangeKey)

	defs, err := s.definitions.ListByTarget(ctx, kind, resp.Node.ID)
	if err != nil {
		return resp, err
	}
	if len(defs) == 0 {
		return resp, nil
	}

	selected, err := s.selectScalarSeries(ctx, defs, resp.Range.From)
	if err != nil {
		return resp, err
	}
	if selected == nil {
		return resp, nil
	}

	resp.Chart.MetricID = selected.metric.MetricID
	resp.Chart.DriftType = string(selected.metric.DriftType)
	resp.Chart.SeriesID = selected.series.ID
	resp.Chart.YDomain = yDomain(selected.points, selected.metric)
	resp.SourceClassification.ObservedSeries = "backend"
	resp.SourceClassification.ExpectedBaseline = "backend"
	resp.SourceClassification.Status = "backend"
	resp.SourceClassification.CompositeScore = "demo_provisional"
	resp.SourceClassification.ContributionValues = "demo_provisional"
	resp.SourceClassification.ContributionWeights = "demo_provisional"

	if selected.metric.ThresholdDirection == drift.ThresholdDirectionAscending {
		resp.SourceClassification.Thresholds = "backend"
	}

	for _, p := range selected.points {
		observed, _ := numericValue(p.SummaryStats["value"])
		expected, _ := numericValue(p.BaselineStats["baseline"])
		mag := p.Magnitude
		resp.Chart.Observed = append(resp.Chart.Observed, ObservedPt{
			T:         p.WindowStart,
			Value:     observed,
			Status:    string(p.Status),
			Magnitude: &mag,
		})
		resp.Chart.Expected = append(resp.Chart.Expected, ExpectedPt{T: p.WindowStart, Value: expected})
		if selected.metric.ThresholdDirection == drift.ThresholdDirectionAscending {
			resp.Chart.Watch = append(resp.Chart.Watch, ExpectedPt{T: p.WindowStart, Value: selected.metric.WarningThreshold})
			resp.Chart.Breach = append(resp.Chart.Breach, ExpectedPt{T: p.WindowStart, Value: selected.metric.BreachedThreshold})
		}
		if resp.ProjectionAsOf == nil || p.WindowStart.After(*resp.ProjectionAsOf) {
			t := p.WindowStart
			resp.ProjectionAsOf = &t
		}
		resp.Provenance.PointIDs = append(resp.Provenance.PointIDs, p.ID)
		resp.Provenance.EnvelopeIDs = append(resp.Provenance.EnvelopeIDs, p.ProvenanceEnvelopeIDs...)
	}
	last := selected.points[len(selected.points)-1]
	current, _ := numericValue(last.SummaryStats["value"])
	resp.Chart.CurrentValue = &current
	resp.Chart.CurrentStatus = string(last.Status)
	resp.DataAvailable = true

	if err := s.attachProvenance(ctx, &resp, selected); err != nil {
		return resp, err
	}
	uniqProvenance(&resp.Provenance)
	return resp, nil
}

func emptyResponse(req DriftAnalyticsRequest) DriftAnalyticsResponse {
	return DriftAnalyticsResponse{
		Node:  NodeRef{Kind: strings.TrimSpace(req.NodeKind), ID: strings.TrimSpace(req.NodeID)},
		Range: RangeRef{Key: cleanRangeKey(req.RangeKey), Label: rangeLabel(cleanRangeKey(req.RangeKey))},
		Chart: Chart{
			Observed: []ObservedPt{},
			Expected: []ExpectedPt{},
			Watch:    []ExpectedPt{},
			Breach:   []ExpectedPt{},
			YDomain:  NumericDomain{Min: 0, Max: 1},
		},
		Provenance: Provenance{
			EnvelopeIDs:        []string{},
			PointIDs:           []string{},
			ObservationIDs:     []string{},
			AnnotationIDs:      []string{},
			ProfileRefs:        []string{},
			PolicyRefs:         []string{},
			VerificationStatus: "not_requested",
		},
		SourceClassification: SourceClassification{
			SelectedNode:        "backend",
			ObservedSeries:      "unavailable",
			ExpectedBaseline:    "unavailable",
			Thresholds:          "unavailable",
			Status:              "unavailable",
			Provenance:          "not_available",
			CompositeScore:      "not_available",
			ContributionValues:  "not_available",
			ContributionWeights: "not_available",
			GraphOverlay:        "not_implemented",
		},
		DataAvailable: false,
	}
}

type candidate struct {
	def    *drift.DriftDefinition
	metric drift.DriftMetricDefinition
	series *drift.DriftSeries
	points []*drift.DriftSeriesPoint
}

func (s *Service) selectScalarSeries(
	ctx context.Context,
	defs []*drift.DriftDefinition,
	from *time.Time,
) (*candidate, error) {
	var candidates []*candidate
	fromWindow := time.Time{}
	if from != nil {
		fromWindow = *from
	}
	for _, d := range defs {
		if d == nil {
			continue
		}
		series, err := s.series.ListByDefinition(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		metricsByID := make(map[string]drift.DriftMetricDefinition, len(d.Metrics))
		for _, m := range d.Metrics {
			metricsByID[m.MetricID] = m
		}
		for _, sr := range series {
			if sr == nil || sr.DefinitionVersion != d.Version {
				continue
			}
			m, ok := metricsByID[sr.MetricID]
			if !ok {
				continue
			}
			points, err := s.seriesPoints.ListBySeries(ctx, sr.ID, fromWindow, 0)
			if err != nil {
				return nil, err
			}
			if len(points) == 0 {
				continue
			}
			sort.Slice(points, func(i, j int) bool {
				return points[i].WindowStart.Before(points[j].WindowStart)
			})
			if !latestChartable(points) {
				continue
			}
			chartable := chartablePoints(points)
			if len(chartable) == 0 {
				continue
			}
			candidates = append(candidates, &candidate{def: d, metric: m, series: sr, points: chartable})
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		a := candidates[i]
		b := candidates[j]
		al := a.points[len(a.points)-1]
		bl := b.points[len(b.points)-1]
		if sa, sb := severity(al.Status), severity(bl.Status); sa != sb {
			return sa > sb
		}
		if !al.WindowStart.Equal(bl.WindowStart) {
			return al.WindowStart.After(bl.WindowStart)
		}
		if a.metric.MetricID != b.metric.MetricID {
			return a.metric.MetricID < b.metric.MetricID
		}
		return a.series.ID < b.series.ID
	})
	return candidates[0], nil
}

func latestChartable(points []*drift.DriftSeriesPoint) bool {
	if len(points) == 0 {
		return false
	}
	return pointChartable(points[len(points)-1])
}

func chartablePoints(points []*drift.DriftSeriesPoint) []*drift.DriftSeriesPoint {
	out := []*drift.DriftSeriesPoint{}
	for _, p := range points {
		if pointChartable(p) {
			out = append(out, p)
		}
	}
	return out
}

func pointChartable(p *drift.DriftSeriesPoint) bool {
	if p == nil {
		return false
	}
	if _, ok := numericValue(p.SummaryStats["value"]); !ok {
		return false
	}
	if _, ok := numericValue(p.BaselineStats["baseline"]); !ok {
		return false
	}
	return true
}

func numericValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return finite(n)
	case float32:
		return finite(float64(n))
	case int:
		return finite(float64(n))
	case int64:
		return finite(float64(n))
	case int32:
		return finite(float64(n))
	case uint:
		return finite(float64(n))
	case uint64:
		if n > math.MaxInt64 {
			return 0, false
		}
		return finite(float64(n))
	default:
		return 0, false
	}
}

func finite(v float64) (float64, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func severity(s drift.DriftSeriesPointStatus) int {
	switch s {
	case drift.DriftSeriesPointStatusBreached:
		return 4
	case drift.DriftSeriesPointStatusWarning:
		return 3
	case drift.DriftSeriesPointStatusUnknownDetectorError:
		return 2
	case drift.DriftSeriesPointStatusUnknownInsufficientData:
		return 1
	case drift.DriftSeriesPointStatusHealthy:
		return 0
	default:
		return -1
	}
}

func yDomain(points []*drift.DriftSeriesPoint, metric drift.DriftMetricDefinition) NumericDomain {
	maxVal := 0.2
	for _, p := range points {
		if v, ok := numericValue(p.SummaryStats["value"]); ok && v > maxVal {
			maxVal = v
		}
		if v, ok := numericValue(p.BaselineStats["baseline"]); ok && v > maxVal {
			maxVal = v
		}
	}
	if metric.WarningThreshold > maxVal {
		maxVal = metric.WarningThreshold
	}
	if metric.BreachedThreshold > maxVal {
		maxVal = metric.BreachedThreshold
	}
	return NumericDomain{Min: 0, Max: maxVal}
}

func (s *Service) attachProvenance(ctx context.Context, resp *DriftAnalyticsResponse, selected *candidate) error {
	if selected.metric.GovernanceExpectationRef != "" {
		resp.Provenance.PolicyRefs = append(resp.Provenance.PolicyRefs, selected.metric.GovernanceExpectationRef)
	}
	if s.observations != nil {
		obs, err := s.observations.ListBySeries(ctx, selected.series.ID)
		if err != nil {
			return err
		}
		for _, o := range obs {
			if o == nil {
				continue
			}
			resp.Provenance.ObservationIDs = append(resp.Provenance.ObservationIDs, o.ID)
			resp.Provenance.EnvelopeIDs = append(resp.Provenance.EnvelopeIDs, o.EvidenceEnvelopeIDs...)
			if o.RelatedFailModePolicyID != "" {
				resp.Provenance.PolicyRefs = append(resp.Provenance.PolicyRefs, o.RelatedFailModePolicyID)
			}
			if o.RelatedGovernanceExpRef != "" {
				resp.Provenance.PolicyRefs = append(resp.Provenance.PolicyRefs, o.RelatedGovernanceExpRef)
			}
			if resp.ProjectionAsOf == nil || o.EmittedAt.After(*resp.ProjectionAsOf) {
				t := o.EmittedAt
				resp.ProjectionAsOf = &t
			}
			if s.annotations != nil {
				anns, err := s.annotations.ListByTarget(ctx, drift.DriftAnnotationTargetKindObservation, o.ID)
				if err != nil {
					return err
				}
				for _, a := range anns {
					if a != nil {
						resp.Provenance.AnnotationIDs = append(resp.Provenance.AnnotationIDs, a.ID)
						resp.Provenance.EnvelopeIDs = append(resp.Provenance.EnvelopeIDs, a.ReferenceEnvelopeIDs...)
					}
				}
			}
		}
	}
	if s.annotations != nil {
		anns, err := s.annotations.ListByTarget(ctx, drift.DriftAnnotationTargetKindSeries, selected.series.ID)
		if err != nil {
			return err
		}
		for _, a := range anns {
			if a != nil {
				resp.Provenance.AnnotationIDs = append(resp.Provenance.AnnotationIDs, a.ID)
				resp.Provenance.EnvelopeIDs = append(resp.Provenance.EnvelopeIDs, a.ReferenceEnvelopeIDs...)
			}
		}
	}
	if len(resp.Provenance.PointIDs) > 0 || len(resp.Provenance.ObservationIDs) > 0 || len(resp.Provenance.AnnotationIDs) > 0 {
		resp.SourceClassification.Provenance = "backend_refs"
	}
	return nil
}

func uniqProvenance(p *Provenance) {
	p.EnvelopeIDs = uniqStrings(p.EnvelopeIDs)
	p.PointIDs = uniqStrings(p.PointIDs)
	p.ObservationIDs = uniqStrings(p.ObservationIDs)
	p.AnnotationIDs = uniqStrings(p.AnnotationIDs)
	p.ProfileRefs = uniqStrings(p.ProfileRefs)
	p.PolicyRefs = uniqStrings(p.PolicyRefs)
}

func uniqStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (s *Service) resolveRange(key string) RangeRef {
	clean := cleanRangeKey(key)
	r := RangeRef{Key: clean, Label: rangeLabel(clean)}
	if clean == "30d" {
		to := s.now()
		from := to.Add(-30 * 24 * time.Hour)
		r.From = &from
		r.To = &to
	}
	return r
}

func cleanRangeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || key == "last_30_days" {
		return "30d"
	}
	return key
}

func rangeLabel(key string) string {
	switch key {
	case "30d":
		return "Last 30 days"
	default:
		return key
	}
}

func isValidTargetKind(kind drift.TargetEntityKind) bool {
	switch kind {
	case drift.TargetEntityKindBusinessService,
		drift.TargetEntityKindCapability,
		drift.TargetEntityKindProcess,
		drift.TargetEntityKindDecisionSurface,
		drift.TargetEntityKindAISystem,
		drift.TargetEntityKindAISystemBinding,
		drift.TargetEntityKindAgent,
		drift.TargetEntityKindAuthorityProfile,
		drift.TargetEntityKindAuthorityGrant:
		return true
	default:
		return false
	}
}
