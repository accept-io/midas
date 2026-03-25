package httpapi

import (
	"embed"
	"encoding/json"
	"net/http"
)

//go:embed explorer
var explorerFS embed.FS

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}
