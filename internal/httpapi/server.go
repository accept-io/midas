package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

type Server struct {
	mux          *http.ServeMux
	orchestrator *decision.Orchestrator
}

func NewServer(orchestrator *decision.Orchestrator) *Server {
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
	s.mux.HandleFunc("/v1/envelopes/", s.handleGetEnvelope)
	s.mux.HandleFunc("/v1/envelopes", s.handleListEnvelopes)
}

func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

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

type evaluateRequest struct {
	SurfaceID   string               `json:"surface_id"`
	AgentID     string               `json:"agent_id"`
	Confidence  float64              `json:"confidence"`
	Consequence *evaluateConsequence `json:"consequence,omitempty"`
	Context     map[string]any       `json:"context,omitempty"`
	RequestID   string               `json:"request_id,omitempty"`
}

type evaluateConsequence struct {
	Type       value.ConsequenceType `json:"type"`
	Amount     float64               `json:"amount,omitempty"`
	Currency   string                `json:"currency,omitempty"`
	RiskRating value.RiskRating      `json:"risk_rating,omitempty"`
}

type evaluateResponse struct {
	Outcome    string `json:"outcome"`
	Reason     string `json:"reason"`
	EnvelopeID string `json:"envelope_id,omitempty"`
}

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var req evaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON payload",
		})
		return
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

	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	}

	if s.orchestrator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "orchestrator not configured",
		})
		return
	}

	evalReq := toEvalRequest(req)

	result, err := s.orchestrator.Evaluate(r.Context(), evalReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	resp := evaluateResponse{
		Outcome:    string(result.Outcome),
		Reason:     string(result.ReasonCode),
		EnvelopeID: result.EnvelopeID,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetEnvelope(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v1/envelopes/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing envelope id",
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

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
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, envs)
}

// toEvalRequest maps the HTTP request payload into the runtime evaluation contract.
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
		SurfaceID:   req.SurfaceID,
		AgentID:     req.AgentID,
		Confidence:  req.Confidence,
		Consequence: consequence,
		Context:     req.Context,
		RequestID:   req.RequestID,
	}
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
		"error": "method not allowed",
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
