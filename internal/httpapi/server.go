package httpapi

import (
	"encoding/json"
	"net/http"
)

type Server struct {
	mux *http.ServeMux
}

func NewServer() *Server {
	mux := http.NewServeMux()

	s := &Server{mux: mux}
	s.routes()

	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReady)
	s.mux.HandleFunc("/v1/evaluate", s.handleEvaluate)
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
	SurfaceID   string      `json:"surface_id"`
	AgentID     string      `json:"agent_id"`
	Confidence  float64     `json:"confidence"`
	Consequence interface{} `json:"consequence,omitempty"`
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

	resp := evaluateResponse{
		Outcome:    "RequestClarification",
		Reason:     "evaluation pipeline not yet implemented",
		EnvelopeID: "env_placeholder",
	}

	writeJSON(w, http.StatusOK, resp)
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
