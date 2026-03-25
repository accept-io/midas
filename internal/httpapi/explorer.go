package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store/memory"
)

//go:embed explorer
var explorerFS embed.FS

// initExplorerRuntime creates the isolated in-memory evaluation runtime used by
// POST /explorer. It always seeds demo data unconditionally, independent
// of cfg.Dev.SeedDemoData. Seeding failures are logged as warnings — Explorer
// continues to work as a request builder even without seeded data.
func (s *Server) initExplorerRuntime() {
	explorerStore := memory.NewStore()
	repos, err := explorerStore.Repositories()
	if err != nil {
		slog.Warn("explorer_store_init_failed", "error", err)
		return
	}
	if err := bootstrap.SeedDemo(context.Background(), repos); err != nil {
		slog.Warn("explorer_seed_failed", "error", err)
		// continue — Explorer still works as a request builder without seed data
	}
	orch, err := decision.NewOrchestrator(explorerStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		slog.Warn("explorer_orchestrator_failed", "error", err)
		return
	}
	s.explorerOrchestrator = orch
}

// handleExplorerIndex serves the Explorer single-page UI at GET /explorer.
func (s *Server) handleExplorerIndex(w http.ResponseWriter, r *http.Request) {
	data, err := explorerFS.ReadFile("explorer/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data) //nolint:errcheck
}

// handleExplorerAssets serves static files embedded under explorer/* (CSS, JS, etc.)
// via the standard FileServer so paths are resolved automatically.
func (s *Server) handleExplorerAssets(w http.ResponseWriter, r *http.Request) {
	http.FileServer(http.FS(explorerFS)).ServeHTTP(w, r)
}

// handleExplorerConfig returns runtime metadata used by the Explorer UI to
// display the current auth mode and policy mode without exposing sensitive state.
func (s *Server) handleExplorerConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	resp := map[string]interface{}{
		"running": true,
	}
	if s.authMode != "" {
		resp["authMode"] = string(s.authMode)
	}
	if s.policyMode != "" {
		resp["policyMode"] = s.policyMode
	}
	if s.storeBackend != "" {
		resp["store"] = s.storeBackend
	}
	if s.explorerDemoSeeded != nil {
		resp["demoSeeded"] = *s.explorerDemoSeeded
	} else {
		resp["demoSeeded"] = "unknown"
	}
	// Explorer always uses an isolated in-memory store regardless of main backend.
	resp["explorerStore"] = "memory"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// handleExplorerGetEnvelope handles GET /explorer/envelopes/{id} using the
// Explorer's isolated in-memory orchestrator so that envelope lookups are
// consistent with evaluations run via POST /explorer.
func (s *Server) handleExplorerGetEnvelope(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.explorerOrchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "explorer runtime not available",
		})
		return
	}

	const prefix = "/explorer/envelopes/"
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing envelope id"})
		return
	}
	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid envelope id"})
		return
	}

	env, err := s.explorerOrchestrator.GetEnvelopeByID(r.Context(), id)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope)
		writeJSON(w, statusCode, errResp)
		return
	}
	if env == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "envelope not found"})
		return
	}
	writeJSON(w, http.StatusOK, env)
}

// handleExplorerEvaluate handles POST /explorer using the Explorer's isolated
// in-memory orchestrator. It reuses handleEvaluateWith so evaluation logic
// stays in one place; only the orchestrator instance differs.
func (s *Server) handleExplorerEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if s.explorerOrchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "explorer runtime not available",
		})
		return
	}
	s.handleEvaluateWith(w, r, s.explorerOrchestrator)
}
