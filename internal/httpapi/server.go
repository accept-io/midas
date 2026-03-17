package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

const (
	maxRequestBodyBytes  = 1 << 20 // 1 MiB
	defaultRequestSource = "api"
	maxIdentifierLength  = 255
)

// orchestrator defines the narrow application contract required by the HTTP API.
// It is intentionally owned by the consumer (httpapi) rather than the producer (decision).
type orchestrator interface {
	Evaluate(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error)
	ResolveEscalation(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error)
	GetEnvelopeByID(ctx context.Context, id string) (*envelope.Envelope, error)
	GetEnvelopeByRequestScope(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error)
	ListEnvelopes(ctx context.Context) ([]*envelope.Envelope, error)
}

type Server struct {
	mux          *http.ServeMux
	orchestrator orchestrator
}

func NewServer(orchestrator orchestrator) *Server {
	mux := http.NewServeMux()

	s := &Server{
		mux:          mux,
		orchestrator: orchestrator,
	}
	s.routes()

	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReady)
	s.mux.HandleFunc("/v1/evaluate", s.handleEvaluate)
	s.mux.HandleFunc("/v1/reviews", s.handleCreateReview)
	s.mux.HandleFunc("/v1/envelopes/", s.handleGetEnvelope)
	s.mux.HandleFunc("/v1/envelopes", s.handleListEnvelopes)
	s.mux.HandleFunc("/v1/decisions/request/", s.handleGetDecisionByRequestID)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}

// ---------------------------------------------------------------------------
// Health/Readiness
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "midas",
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ready",
		"service": "midas",
	})
}

// ---------------------------------------------------------------------------
// Evaluate
// ---------------------------------------------------------------------------

type evaluateRequest struct {
	SurfaceID     string               `json:"surface_id"`
	AgentID       string               `json:"agent_id"`
	Confidence    float64              `json:"confidence"`
	Consequence   *evaluateConsequence `json:"consequence,omitempty"`
	Context       map[string]any       `json:"context,omitempty"`
	RequestSource string               `json:"request_source,omitempty"`
	RequestID     string               `json:"request_id,omitempty"`
}

type evaluateConsequence struct {
	Type       value.ConsequenceType `json:"type"`
	Amount     float64               `json:"amount,omitempty"`
	Currency   string                `json:"currency,omitempty"`
	RiskRating value.RiskRating      `json:"risk_rating,omitempty"`
}

type evaluateResponse struct {
	Outcome     string `json:"outcome"`
	Reason      string `json:"reason"`
	EnvelopeID  string `json:"envelope_id,omitempty"`
	Explanation string `json:"explanation,omitempty"`
}

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{
			"error": err.Error(),
		})
		return
	}

	var req evaluateRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	req.SurfaceID = strings.TrimSpace(req.SurfaceID)
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.RequestSource = strings.TrimSpace(req.RequestSource)
	req.RequestID = strings.TrimSpace(req.RequestID)
	if req.Consequence != nil {
		req.Consequence.Currency = strings.TrimSpace(req.Consequence.Currency)
	}

	if req.SurfaceID == "" || req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "surface_id and agent_id are required",
		})
		return
	}

	if req.Confidence < 0 || req.Confidence > 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "confidence must be between 0 and 1",
		})
		return
	}

	// Default request_source to "api" if not provided.
	if req.RequestSource == "" {
		req.RequestSource = defaultRequestSource
	}

	// Generate request_id if absent; validate format if provided.
	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	} else {
		if !isValidIdentifier(req.RequestID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "request_id contains invalid characters or exceeds length limit",
			})
			return
		}
	}

	result, err := s.orchestrator.Evaluate(r.Context(), toEvalRequest(req), json.RawMessage(rawBody))
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEvaluation)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, evaluateResponse{
		Outcome:     string(result.Outcome),
		Reason:      string(result.ReasonCode),
		EnvelopeID:  result.EnvelopeID,
		Explanation: result.Explanation,
	})
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

type reviewRequest struct {
	EnvelopeID string `json:"envelope_id"`
	Decision   string `json:"decision"`
	Reviewer   string `json:"reviewer"`
	Notes      string `json:"notes,omitempty"`
}

type reviewResponse struct {
	EnvelopeID string `json:"envelope_id"`
	Status     string `json:"status"`
}

func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	rawBody, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{
			"error": err.Error(),
		})
		return
	}

	var req reviewRequest
	if err := decodeStrictJSON(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	req.EnvelopeID = strings.TrimSpace(req.EnvelopeID)
	req.Decision = strings.TrimSpace(req.Decision)
	req.Reviewer = strings.TrimSpace(req.Reviewer)
	req.Notes = strings.TrimSpace(req.Notes)

	if req.EnvelopeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "envelope_id is required",
		})
		return
	}

	if req.Decision == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "decision is required",
		})
		return
	}

	if !isValidIdentifier(req.Reviewer) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "reviewer must be a valid identifier (1-255 characters, no control characters)",
		})
		return
	}

	// Map HTTP decision vocabulary to envelope enum.
	// Accept both canonical (accept/reject) and legacy (approve/deny) for backward compat.
	var reviewDecision envelope.ReviewDecision
	switch strings.ToLower(req.Decision) {
	case "accept", "approve", "approved":
		reviewDecision = envelope.ReviewDecisionApproved
	case "reject", "deny", "denied":
		reviewDecision = envelope.ReviewDecisionRejected
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "decision must be 'accept'/'approve' or 'reject'/'deny'",
		})
		return
	}

	resolvedEnvelope, err := s.orchestrator.ResolveEscalation(r.Context(), decision.EscalationResolution{
		EnvelopeID:   req.EnvelopeID,
		Decision:     reviewDecision,
		ReviewerID:   req.Reviewer,
		ReviewerKind: "human",
		Notes:        req.Notes,
	})
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityReview)
		writeJSON(w, statusCode, errResp)
		return
	}

	// The resolved envelope ID must match the request envelope ID (immutability invariant).
	// If they differ, the orchestrator has violated its contract.
	if resolvedEnvelope != nil && resolvedEnvelope.ID() != "" && resolvedEnvelope.ID() != req.EnvelopeID {
		// Log this as a critical invariant violation in production.
		// For now, preserve the request ID in the response to maintain API contract.
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "envelope identity invariant violated",
		})
		return
	}

	writeJSON(w, http.StatusOK, reviewResponse{
		EnvelopeID: req.EnvelopeID,
		Status:     "resolved",
	})
}

// ---------------------------------------------------------------------------
// Envelope Retrieval
// ---------------------------------------------------------------------------

func (s *Server) handleGetEnvelope(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	const prefix = "/v1/envelopes/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "not found",
		})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, prefix)
	id = strings.TrimSpace(id)

	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing envelope id",
		})
		return
	}

	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid envelope id",
		})
		return
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	env, err := s.orchestrator.GetEnvelopeByID(r.Context(), id)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope)
		writeJSON(w, statusCode, errResp)
		return
	}

	// The orchestrator contract should return ErrEnvelopeNotFound rather than (nil, nil).
	// This check exists for defensive purposes during the error migration.
	if env == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "envelope not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, env)
}

func (s *Server) handleListEnvelopes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	envs, err := s.orchestrator.ListEnvelopes(r.Context())
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope)
		writeJSON(w, statusCode, errResp)
		return
	}

	writeJSON(w, http.StatusOK, envs)
}

func (s *Server) handleGetDecisionByRequestID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	const prefix = "/v1/decisions/request/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "not found",
		})
		return
	}

	requestID := strings.TrimPrefix(r.URL.Path, prefix)
	requestID = strings.TrimSpace(requestID)

	if requestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing request id",
		})
		return
	}

	if !isValidIdentifier(requestID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request id",
		})
		return
	}

	// Request IDs are scoped by source (schema v2.1).
	requestSource := strings.TrimSpace(r.URL.Query().Get("source"))
	if requestSource == "" {
		requestSource = defaultRequestSource
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	env, err := s.orchestrator.GetEnvelopeByRequestScope(r.Context(), requestSource, requestID)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityDecision)
		writeJSON(w, statusCode, errResp)
		return
	}

	// The orchestrator contract should return ErrEnvelopeNotFound rather than (nil, nil).
	// This check exists for defensive purposes during the error migration.
	if env == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "decision not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, env)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const (
	entityEvaluation = "evaluation"
	entityReview     = "review"
	entityEnvelope   = "envelope"
	entityDecision   = "decision"
)

func toEvalRequest(req evaluateRequest) eval.DecisionRequest {
	var consequence *eval.Consequence
	if req.Consequence != nil {
		consequence = &eval.Consequence{
			Type:       req.Consequence.Type,
			Amount:     req.Consequence.Amount,
			Currency:   req.Consequence.Currency,
			RiskRating: req.Consequence.RiskRating,
		}
	}

	return eval.DecisionRequest{
		SurfaceID:     req.SurfaceID,
		AgentID:       req.AgentID,
		Confidence:    req.Confidence,
		Consequence:   consequence,
		Context:       req.Context,
		RequestSource: req.RequestSource,
		RequestID:     req.RequestID,
	}
}

// mapDomainError translates domain errors to HTTP status codes and response bodies.
// TODO: Remove TRANSITIONAL string-matching fallback once all orchestrator
// errors are migrated to typed sentinels.
func mapDomainError(err error, entityType string) (int, map[string]string) {
	if err == nil {
		return http.StatusOK, nil
	}

	// Typed sentinel error checks (preferred).
	switch {
	case errors.Is(err, decision.ErrEnvelopeNotFound):
		msg := entityType + " not found"
		if entityType == entityReview {
			msg = "envelope not found"
		}
		return http.StatusNotFound, map[string]string{"error": msg}

	case errors.Is(err, decision.ErrEnvelopeNotAwaitingReview),
		errors.Is(err, decision.ErrEnvelopeAlreadyClosed):
		return http.StatusConflict, map[string]string{"error": err.Error()}

	case errors.Is(err, decision.ErrEmptyIdentifier),
		errors.Is(err, decision.ErrInvalidReviewDecision):
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}

	// TRANSITIONAL: String-matching fallback for errors not yet migrated to typed sentinels.
	// This is brittle and will break silently if error messages change.
	// Remove this block once all domain errors use typed sentinels.
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "not found"):
		return http.StatusNotFound, map[string]string{"error": errMsg}
	case strings.Contains(errMsg, "invalid state"),
		strings.Contains(errMsg, "already resolved"),
		strings.Contains(errMsg, "already closed"):
		return http.StatusConflict, map[string]string{"error": errMsg}
	case strings.Contains(errMsg, "self-review"),
		strings.Contains(errMsg, "insufficient authority"):
		return http.StatusForbidden, map[string]string{"error": errMsg}
	case strings.Contains(errMsg, "duplicate"):
		return http.StatusConflict, map[string]string{"error": errMsg}
	default:
		// Unmapped error: log in production to identify gaps in error migration.
		return http.StatusInternalServerError, map[string]string{"error": errMsg}
	}
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
		"error": "method not allowed",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	// Encoding failure is ignored here because once headers/status are written
	// there is no useful recovery path for the handler.
	_ = json.NewEncoder(w).Encode(v)
}

var errRequestBodyTooLarge = errors.New("request body too large")

func readRequestBody(w http.ResponseWriter, r *http.Request, maxBytes int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, errRequestBodyTooLarge
		}
		return nil, errors.New("failed to read request body")
	}

	if len(bytes.TrimSpace(rawBody)) == 0 {
		return nil, errors.New("request body must not be empty")
	}

	return rawBody, nil
}

func decodeStrictJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON payload")
	}

	// Reject trailing tokens (e.g., multiple JSON objects or trailing garbage).
	// If anything other than EOF is found, the input is malformed.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("invalid JSON payload")
	}

	return nil
}

// isValidIdentifier validates that an identifier is safe for use in URLs and storage.
// Rejects empty strings, excessive length, path traversal characters, and control characters.
func isValidIdentifier(id string) bool {
	if id == "" || len(id) > maxIdentifierLength {
		return false
	}

	for _, r := range id {
		// Reject path separators, null bytes, and control characters.
		if r == '/' || r == '\\' || r == 0 || r < 32 || r == 127 {
			return false
		}
	}

	return true
}
