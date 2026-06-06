package httpapi

// drift_handler.go — Drift-1d read-only HTTP API. Backs 13 GET routes
// over the Drift first-class structural entity (Drift-1a/1b/1c).
//
// Runtime-inert: every handler reads from drift repositories only; no
// repository write methods are reachable from this file. The
// aggregation worker, threshold detector, and audit-chain integration
// land in later tranches (Drift-3a/b/c, Drift-4).

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/drift"
	driftanalytics "github.com/accept-io/midas/internal/drift/analytics"
)

// ---------------------------------------------------------------------------
// Reader interfaces — narrow apply-side subsets of Drift-1a's repositories.
// Mirror StructuralService's reader-subset convention so this package owns
// only the read methods it actually invokes.
// ---------------------------------------------------------------------------

type DriftDefinitionReader interface {
	FindByID(ctx context.Context, id string) (*drift.DriftDefinition, error)
	FindByIDAndVersion(ctx context.Context, id string, version int) (*drift.DriftDefinition, error)
	ListVersions(ctx context.Context, id string) ([]*drift.DriftDefinition, error)
}

type DriftSeriesReader interface {
	FindByID(ctx context.Context, id string) (*drift.DriftSeries, error)
	ListByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftSeries, error)
}

type DriftSeriesPointReader interface {
	FindByID(ctx context.Context, id string) (*drift.DriftSeriesPoint, error)
	ListBySeries(ctx context.Context, seriesID string, fromWindow time.Time, limit int) ([]*drift.DriftSeriesPoint, error)
}

type DriftObservationReader interface {
	FindByID(ctx context.Context, id string) (*drift.DriftObservation, error)
	ListBySeries(ctx context.Context, seriesID string) ([]*drift.DriftObservation, error)
	ListByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftObservation, error)
	ListByEntity(ctx context.Context, kind drift.TargetEntityKind, entityID string) ([]*drift.DriftObservation, error)
}

type DriftAnnotationReader interface {
	FindByID(ctx context.Context, id string) (*drift.DriftAnnotation, error)
	ListByTarget(ctx context.Context, kind drift.DriftAnnotationTargetKind, targetID string) ([]*drift.DriftAnnotation, error)
}

// driftReadService is the narrow contract the Server consults for the
// /v1/drift/* endpoints. It is owned by this package so handlers can
// be tested with a stub.
type driftReadService interface {
	HasDefinitions() bool
	HasSeries() bool
	HasSeriesPoints() bool
	HasObservations() bool
	HasAnnotations() bool

	GetDefinition(ctx context.Context, id string) (*drift.DriftDefinition, error)
	GetDefinitionVersion(ctx context.Context, id string, version int) (*drift.DriftDefinition, error)
	ListDefinitionVersions(ctx context.Context, id string) ([]*drift.DriftDefinition, error)

	GetSeries(ctx context.Context, id string) (*drift.DriftSeries, error)
	ListSeriesByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftSeries, error)

	GetSeriesPoint(ctx context.Context, id string) (*drift.DriftSeriesPoint, error)
	ListSeriesPoints(ctx context.Context, seriesID string, fromWindow time.Time, limit int) ([]*drift.DriftSeriesPoint, error)

	GetObservation(ctx context.Context, id string) (*drift.DriftObservation, error)
	ListObservationsBySeries(ctx context.Context, seriesID string) ([]*drift.DriftObservation, error)
	ListObservationsByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftObservation, error)
	ListObservationsByEntity(ctx context.Context, kind drift.TargetEntityKind, entityID string) ([]*drift.DriftObservation, error)

	GetAnnotation(ctx context.Context, id string) (*drift.DriftAnnotation, error)
	ListAnnotationsByTarget(ctx context.Context, kind drift.DriftAnnotationTargetKind, targetID string) ([]*drift.DriftAnnotation, error)
}

// DriftReadService satisfies driftReadService by delegating to the
// underlying repository readers.
type DriftReadService struct {
	definitions  DriftDefinitionReader
	series       DriftSeriesReader
	seriesPoints DriftSeriesPointReader
	observations DriftObservationReader
	annotations  DriftAnnotationReader
}

// NewDriftReadService constructs a service with the supplied readers.
// Any reader may be nil; the corresponding Has<X>() method returns
// false and the routes that depend on it return 501 Not Implemented.
func NewDriftReadService(
	definitions DriftDefinitionReader,
	series DriftSeriesReader,
	seriesPoints DriftSeriesPointReader,
	observations DriftObservationReader,
	annotations DriftAnnotationReader,
) *DriftReadService {
	return &DriftReadService{
		definitions:  definitions,
		series:       series,
		seriesPoints: seriesPoints,
		observations: observations,
		annotations:  annotations,
	}
}

func (s *DriftReadService) HasDefinitions() bool  { return s != nil && s.definitions != nil }
func (s *DriftReadService) HasSeries() bool       { return s != nil && s.series != nil }
func (s *DriftReadService) HasSeriesPoints() bool { return s != nil && s.seriesPoints != nil }
func (s *DriftReadService) HasObservations() bool { return s != nil && s.observations != nil }
func (s *DriftReadService) HasAnnotations() bool  { return s != nil && s.annotations != nil }

func (s *DriftReadService) GetDefinition(ctx context.Context, id string) (*drift.DriftDefinition, error) {
	return s.definitions.FindByID(ctx, id)
}
func (s *DriftReadService) GetDefinitionVersion(ctx context.Context, id string, version int) (*drift.DriftDefinition, error) {
	return s.definitions.FindByIDAndVersion(ctx, id, version)
}
func (s *DriftReadService) ListDefinitionVersions(ctx context.Context, id string) ([]*drift.DriftDefinition, error) {
	return s.definitions.ListVersions(ctx, id)
}
func (s *DriftReadService) GetSeries(ctx context.Context, id string) (*drift.DriftSeries, error) {
	return s.series.FindByID(ctx, id)
}
func (s *DriftReadService) ListSeriesByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftSeries, error) {
	return s.series.ListByDefinition(ctx, definitionID)
}
func (s *DriftReadService) GetSeriesPoint(ctx context.Context, id string) (*drift.DriftSeriesPoint, error) {
	return s.seriesPoints.FindByID(ctx, id)
}
func (s *DriftReadService) ListSeriesPoints(ctx context.Context, seriesID string, fromWindow time.Time, limit int) ([]*drift.DriftSeriesPoint, error) {
	return s.seriesPoints.ListBySeries(ctx, seriesID, fromWindow, limit)
}
func (s *DriftReadService) GetObservation(ctx context.Context, id string) (*drift.DriftObservation, error) {
	return s.observations.FindByID(ctx, id)
}
func (s *DriftReadService) ListObservationsBySeries(ctx context.Context, seriesID string) ([]*drift.DriftObservation, error) {
	return s.observations.ListBySeries(ctx, seriesID)
}
func (s *DriftReadService) ListObservationsByDefinition(ctx context.Context, definitionID string) ([]*drift.DriftObservation, error) {
	return s.observations.ListByDefinition(ctx, definitionID)
}
func (s *DriftReadService) ListObservationsByEntity(ctx context.Context, kind drift.TargetEntityKind, entityID string) ([]*drift.DriftObservation, error) {
	return s.observations.ListByEntity(ctx, kind, entityID)
}
func (s *DriftReadService) GetAnnotation(ctx context.Context, id string) (*drift.DriftAnnotation, error) {
	return s.annotations.FindByID(ctx, id)
}
func (s *DriftReadService) ListAnnotationsByTarget(ctx context.Context, kind drift.DriftAnnotationTargetKind, targetID string) ([]*drift.DriftAnnotation, error) {
	return s.annotations.ListByTarget(ctx, kind, targetID)
}

var _ driftReadService = (*DriftReadService)(nil)

// ---------------------------------------------------------------------------
// Limit helpers shared across drift list endpoints.
// ---------------------------------------------------------------------------

const (
	driftListLimitDefault = 100
	driftListLimitMax     = 500
)

var validDriftTargetEntityKindStrings = map[string]drift.TargetEntityKind{
	string(drift.TargetEntityKindBusinessService):  drift.TargetEntityKindBusinessService,
	string(drift.TargetEntityKindCapability):       drift.TargetEntityKindCapability,
	string(drift.TargetEntityKindProcess):          drift.TargetEntityKindProcess,
	string(drift.TargetEntityKindDecisionSurface):  drift.TargetEntityKindDecisionSurface,
	string(drift.TargetEntityKindAISystem):         drift.TargetEntityKindAISystem,
	string(drift.TargetEntityKindAISystemBinding):  drift.TargetEntityKindAISystemBinding,
	string(drift.TargetEntityKindAgent):            drift.TargetEntityKindAgent,
	string(drift.TargetEntityKindAuthorityProfile): drift.TargetEntityKindAuthorityProfile,
	string(drift.TargetEntityKindAuthorityGrant):   drift.TargetEntityKindAuthorityGrant,
}

// errInvalidDriftQuery is returned when a query parameter cannot be parsed.
// Surfaced to the caller as 400 Bad Request.
var errInvalidDriftQuery = errors.New("invalid drift query parameter")

// parseDriftListQuery validates limit + from_window for the points
// endpoint. Returns (fromWindow, limit, error). Empty from_window
// resolves to time.Time{} (zero — treated as "no lower bound" by the
// repo's ListBySeries). Empty limit resolves to driftListLimitDefault.
func parseDriftListQuery(q map[string][]string) (time.Time, int, error) {
	limit := driftListLimitDefault
	if vs, ok := q["limit"]; ok && len(vs) > 0 && strings.TrimSpace(vs[0]) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(vs[0]))
		if err != nil {
			return time.Time{}, 0, errors.New("limit must be an integer")
		}
		if n <= 0 {
			return time.Time{}, 0, errors.New("limit must be > 0")
		}
		if n > driftListLimitMax {
			return time.Time{}, 0, errors.New("limit must be <= 500")
		}
		limit = n
	}

	var fromWindow time.Time
	if vs, ok := q["from_window"]; ok && len(vs) > 0 && strings.TrimSpace(vs[0]) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(vs[0]))
		if err != nil {
			return time.Time{}, 0, errors.New("from_window must be an RFC3339 timestamp")
		}
		fromWindow = t
	}

	return fromWindow, limit, nil
}

// ---------------------------------------------------------------------------
// Prefix dispatchers. Go's net/http ServeMux uses prefix matching for
// patterns ending in "/", so we register a single dispatcher per top-
// level prefix and route inside.
// ---------------------------------------------------------------------------

// handleDriftDefinitionsPrefix dispatches:
//
//	GET /v1/drift/definitions/{id}
//	GET /v1/drift/definitions/{id}/versions
//	GET /v1/drift/definitions/{id}/versions/{version}
//	GET /v1/drift/definitions/{id}/series
//	GET /v1/drift/definitions/{id}/observations
func (s *Server) handleDriftDefinitionsPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/drift/definitions/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	parts := strings.Split(tail, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	switch {
	case len(parts) == 1:
		s.handleGetDriftDefinition(w, r, id)
	case len(parts) == 2 && parts[1] == "versions":
		s.handleListDriftDefinitionVersions(w, r, id)
	case len(parts) == 3 && parts[1] == "versions":
		s.handleGetDriftDefinitionVersion(w, r, id, parts[2])
	case len(parts) == 2 && parts[1] == "series":
		s.handleListDriftSeriesByDefinition(w, r, id)
	case len(parts) == 2 && parts[1] == "observations":
		s.handleListDriftObservationsByDefinition(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleDriftSeriesPrefix dispatches:
//
//	GET /v1/drift/series/{id}
//	GET /v1/drift/series/{id}/points
//	GET /v1/drift/series/{id}/observations
//	GET /v1/drift/series/{id}/annotations
func (s *Server) handleDriftSeriesPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/drift/series/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	parts := strings.Split(tail, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	switch {
	case len(parts) == 1:
		s.handleGetDriftSeries(w, r, id)
	case len(parts) == 2 && parts[1] == "points":
		s.handleListDriftSeriesPoints(w, r, id)
	case len(parts) == 2 && parts[1] == "observations":
		s.handleListDriftObservationsBySeries(w, r, id)
	case len(parts) == 2 && parts[1] == "annotations":
		s.handleListDriftAnnotationsBySeries(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleDriftSeriesPointsPrefix dispatches: GET /v1/drift/series-points/{id}
func (s *Server) handleDriftSeriesPointsPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/drift/series-points/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" || strings.Contains(tail, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	s.handleGetDriftSeriesPoint(w, r, tail)
}

// handleDriftObservationsPrefix dispatches:
//
//	GET /v1/drift/observations/{id}
//	GET /v1/drift/observations/{id}/annotations
func (s *Server) handleDriftObservationsPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/drift/observations/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	parts := strings.Split(tail, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	switch {
	case len(parts) == 1:
		s.handleGetDriftObservation(w, r, id)
	case len(parts) == 2 && parts[1] == "annotations":
		s.handleListDriftAnnotationsByObservation(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleDriftAnnotationsPrefix dispatches: GET /v1/drift/annotations/{id}
func (s *Server) handleDriftAnnotationsPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/drift/annotations/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" || strings.Contains(tail, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	s.handleGetDriftAnnotation(w, r, tail)
}

// handleDriftEntitiesPrefix dispatches: GET /v1/drift/entities/{kind}/{entity_id}/observations
func (s *Server) handleDriftEntitiesPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/drift/entities/"
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	tail = strings.Trim(tail, "/")
	parts := strings.Split(tail, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] != "observations" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	s.handleListDriftObservationsByEntity(w, r, parts[0], parts[1])
}

// ---------------------------------------------------------------------------
// Per-route handlers.
// ---------------------------------------------------------------------------

func (s *Server) handleGetDriftAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.driftAnalytics == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift analytics reader not configured"})
		return
	}
	q := r.URL.Query()
	resp, err := s.driftAnalytics.GetNodeAnalytics(r.Context(), driftanalytics.DriftAnalyticsRequest{
		NodeKind: q.Get("node_kind"),
		NodeID:   q.Get("node_id"),
		RangeKey: q.Get("range"),
	})
	if err != nil {
		switch {
		case errors.Is(err, driftanalytics.ErrInvalidRequest):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_kind and node_id are required and node_kind must be valid"})
		case errors.Is(err, driftanalytics.ErrNotConfigured):
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift analytics reader not configured"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetDriftDefinition(w http.ResponseWriter, r *http.Request, id string) {
	if s.driftRead == nil || !s.driftRead.HasDefinitions() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift definition reader not configured"})
		return
	}
	d, err := s.driftRead.GetDefinition(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if d == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift definition not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDriftDefinitionResponse(d))
}

func (s *Server) handleListDriftDefinitionVersions(w http.ResponseWriter, r *http.Request, id string) {
	if s.driftRead == nil || !s.driftRead.HasDefinitions() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift definition reader not configured"})
		return
	}
	versions, err := s.driftRead.ListDefinitionVersions(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(versions) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift definition not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDriftDefinitionListResponse(versions))
}

func (s *Server) handleGetDriftDefinitionVersion(w http.ResponseWriter, r *http.Request, id, versionStr string) {
	if s.driftRead == nil || !s.driftRead.HasDefinitions() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift definition reader not configured"})
		return
	}
	v, err := strconv.Atoi(versionStr)
	if err != nil || v < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version path segment must be a positive integer"})
		return
	}
	d, err := s.driftRead.GetDefinitionVersion(r.Context(), id, v)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if d == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift definition version not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDriftDefinitionResponse(d))
}

func (s *Server) handleGetDriftSeries(w http.ResponseWriter, r *http.Request, id string) {
	if s.driftRead == nil || !s.driftRead.HasSeries() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift series reader not configured"})
		return
	}
	srs, err := s.driftRead.GetSeries(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if srs == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift series not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDriftSeriesResponse(srs))
}

func (s *Server) handleListDriftSeriesByDefinition(w http.ResponseWriter, r *http.Request, definitionID string) {
	if s.driftRead == nil || !s.driftRead.HasSeries() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift series reader not configured"})
		return
	}
	srs, err := s.driftRead.ListSeriesByDefinition(r.Context(), definitionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toDriftSeriesListResponse(srs))
}

func (s *Server) handleListDriftSeriesPoints(w http.ResponseWriter, r *http.Request, seriesID string) {
	if s.driftRead == nil || !s.driftRead.HasSeriesPoints() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift series point reader not configured"})
		return
	}
	fromWindow, limit, err := parseDriftListQuery(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	pts, err := s.driftRead.ListSeriesPoints(r.Context(), seriesID, fromWindow, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toDriftSeriesPointListResponse(pts))
}

func (s *Server) handleGetDriftSeriesPoint(w http.ResponseWriter, r *http.Request, id string) {
	if s.driftRead == nil || !s.driftRead.HasSeriesPoints() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift series point reader not configured"})
		return
	}
	p, err := s.driftRead.GetSeriesPoint(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift series point not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDriftSeriesPointResponse(p))
}

func (s *Server) handleGetDriftObservation(w http.ResponseWriter, r *http.Request, id string) {
	if s.driftRead == nil || !s.driftRead.HasObservations() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift observation reader not configured"})
		return
	}
	o, err := s.driftRead.GetObservation(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if o == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift observation not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDriftObservationResponse(o))
}

func (s *Server) handleListDriftObservationsBySeries(w http.ResponseWriter, r *http.Request, seriesID string) {
	if s.driftRead == nil || !s.driftRead.HasObservations() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift observation reader not configured"})
		return
	}
	obs, err := s.driftRead.ListObservationsBySeries(r.Context(), seriesID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toDriftObservationListResponse(obs))
}

func (s *Server) handleListDriftObservationsByDefinition(w http.ResponseWriter, r *http.Request, definitionID string) {
	if s.driftRead == nil || !s.driftRead.HasObservations() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift observation reader not configured"})
		return
	}
	obs, err := s.driftRead.ListObservationsByDefinition(r.Context(), definitionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toDriftObservationListResponse(obs))
}

func (s *Server) handleListDriftObservationsByEntity(w http.ResponseWriter, r *http.Request, kindStr, entityID string) {
	if s.driftRead == nil || !s.driftRead.HasObservations() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift observation reader not configured"})
		return
	}
	kind, ok := validDriftTargetEntityKindStrings[kindStr]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "target entity kind must be one of business_service|capability|process|decision_surface|ai_system|ai_system_binding|agent|authority_profile|authority_grant",
		})
		return
	}
	obs, err := s.driftRead.ListObservationsByEntity(r.Context(), kind, entityID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toDriftObservationListResponse(obs))
}

func (s *Server) handleGetDriftAnnotation(w http.ResponseWriter, r *http.Request, id string) {
	if s.driftRead == nil || !s.driftRead.HasAnnotations() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift annotation reader not configured"})
		return
	}
	a, err := s.driftRead.GetAnnotation(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if a == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "drift annotation not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDriftAnnotationResponse(a))
}

func (s *Server) handleListDriftAnnotationsBySeries(w http.ResponseWriter, r *http.Request, seriesID string) {
	if s.driftRead == nil || !s.driftRead.HasAnnotations() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift annotation reader not configured"})
		return
	}
	anns, err := s.driftRead.ListAnnotationsByTarget(r.Context(), drift.DriftAnnotationTargetKindSeries, seriesID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toDriftAnnotationListResponse(anns))
}

func (s *Server) handleListDriftAnnotationsByObservation(w http.ResponseWriter, r *http.Request, observationID string) {
	if s.driftRead == nil || !s.driftRead.HasAnnotations() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "drift annotation reader not configured"})
		return
	}
	anns, err := s.driftRead.ListAnnotationsByTarget(r.Context(), drift.DriftAnnotationTargetKindObservation, observationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toDriftAnnotationListResponse(anns))
}
