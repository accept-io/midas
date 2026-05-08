package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/governancecoverage"
	"github.com/accept-io/midas/internal/platformauth"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store/memory"
)

// Explorer envelope list pagination bounds.
//
// These mirror the convention used by /v1/coverage and /v1/controlplane/audit:
// values above max return 400 rather than silently clamping. Default 50 keeps
// the list compact for the Records view and the Bottom Evidence Tray.
const (
	explorerEnvelopeListDefaultLimit = 50
	explorerEnvelopeListMaxLimit     = 500
)

//go:embed explorer
var explorerFS embed.FS

// initExplorerRuntime creates the isolated in-memory evaluation runtime used by
// POST /explorer. It always seeds the demo dataset unconditionally, independent
// of cfg.Dev.SeedDemoData. Seeding failures are logged as warnings — Explorer
// continues to work as a request builder even without seeded data.
//
// Side-effects on Server:
//   - explorerOrchestrator   — used by POST /explorer and /explorer/simulate
//   - explorerCoverageRead   — used by GET /explorer/coverage; backed by the
//     same isolated audit repository the orchestrator writes to, so Explorer
//     coverage reads see only Explorer-emitted events. The production
//     coverage read service (s.coverageRead) is bound to the production
//     audit repository in cmd/midas/main.go and is unaffected — that
//     isolation is the load-bearing property pinned by the isolation tests.
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
	if repos.Audit != nil {
		s.explorerAudit = repos.Audit
		s.explorerCoverageRead = governancecoverage.NewReadService(repos.Audit)
	}
}

// handleExplorerIndex serves the Explorer single-page UI at GET /explorer.
//
// When local IAM is active this handler is reached via explorerShellHandler,
// which applies AuthMiddleware (session extraction) before calling here. The
// handler then branches intentionally on session presence:
//   - authenticated: serve shell normally (Cache-Control: no-store)
//   - unauthenticated: serve shell with X-Auth-Required: true to signal that
//     the server has actively checked and found no session; the shell itself
//     contains the login overlay which is the primary login UX
//
// Both branches serve the HTML shell so the client-side login overlay flow
// remains intact for both local IAM and OIDC modes.
func (s *Server) handleExplorerIndex(w http.ResponseWriter, r *http.Request) {
	data, err := explorerFS.ReadFile("explorer/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if s.localIAM != nil {
		w.Header().Set("Cache-Control", "no-store")
		if _, ok := platformauth.PrincipalFromContext(r.Context()); !ok {
			// Server has actively checked: no valid session present.
			// The login overlay in the shell will handle the login flow.
			w.Header().Set("X-Auth-Required", "true")
		}
	}
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
	// Signal to the UI that local IAM (session-cookie auth) is active.
	if s.localIAM != nil {
		resp["localiam"] = true
	}
	// Signal to the UI that the demo Local IAM user (demo/demo) is seeded.
	// Used by the login panel to display a contextual hint.
	if s.seedDemoUser {
		resp["demoUser"] = true
	}
	// Signal to the UI that OIDC SSO login is active (replaces local login form).
	if s.oidcService != nil {
		resp["oidc"] = true
	}

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
		statusCode, errResp := mapDomainError(err, entityEnvelope, false)
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
	s.handleEvaluateWith(w, r, s.explorerOrchestrator, false, auditStatusExplorer)
}

// handleExplorerSimulate handles POST /explorer/simulate using the Explorer's
// isolated in-memory orchestrator. It reuses handleSimulateWith so that the
// simulate path shares the same request parsing and validation logic as the
// evaluate path; only the orchestrator method called differs.
//
// No envelope is created, no audit events are written, and no outbox messages
// are queued. The response includes simulated:true and omits envelope_id.
func (s *Server) handleExplorerSimulate(w http.ResponseWriter, r *http.Request) {
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
	s.handleSimulateWith(w, r, s.explorerOrchestrator)
}

// explorerEnvelopeSummary is the compact wire format for one envelope in the
// /explorer/envelopes list response. Full detail remains available via
// GET /explorer/envelopes/{id}.
type explorerEnvelopeSummary struct {
	ID             string     `json:"id"`
	State          string     `json:"state"`
	RequestID      string     `json:"request_id"`
	RequestSource  string     `json:"request_source"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	EvaluatedAt    *time.Time `json:"evaluated_at,omitempty"`
	Outcome        string     `json:"outcome,omitempty"`
	ReasonCode     string     `json:"reason_code,omitempty"`
	SurfaceID      string     `json:"surface_id,omitempty"`
	SurfaceVersion int        `json:"surface_version,omitempty"`
	ProfileID      string     `json:"profile_id,omitempty"`
	ProfileVersion int        `json:"profile_version,omitempty"`
	GrantID        string     `json:"grant_id,omitempty"`
	AgentID        string     `json:"agent_id,omitempty"`
	SubjectID      string     `json:"subject_id,omitempty"`
	// BusinessServiceID and ProcessID are read from Resolved.Structure
	// snapshots, which are point-in-time evidence frozen at evaluation
	// time. They may be empty when the resolved chain did not include
	// service-led structural context.
	BusinessServiceID string `json:"business_service_id,omitempty"`
	ProcessID         string `json:"process_id,omitempty"`
}

// explorerEnvelopeListResponse is the wire format for GET /explorer/envelopes.
//
// items is always present (never null) — empty list is an empty array.
// count reflects the number of envelopes returned after filtering and limit.
// limit echoes the effective limit applied (default or caller-supplied,
// after validation).
type explorerEnvelopeListResponse struct {
	Items []explorerEnvelopeSummary `json:"items"`
	Count int                       `json:"count"`
	Limit int                       `json:"limit"`
}

// handleExplorerListEnvelopes serves GET /explorer/envelopes against the
// Explorer's isolated in-memory orchestrator. It returns a compact list
// of envelope summaries — full detail remains available via
// GET /explorer/envelopes/{id}.
//
// Isolation: this endpoint reads only from s.explorerOrchestrator (the
// isolated in-memory runtime built in initExplorerRuntime). Production
// envelope state (s.orchestrator) is never consulted here. The same
// disjointness property pinned by /explorer/coverage applies here.
//
// Query parameters (all optional):
//   - state           — exact-match envelope lifecycle state filter; one of
//     received, evaluating, outcome_recorded, escalated,
//     awaiting_review, closed. Invalid value → 400.
//   - since, until    — RFC3339 timestamps; filter by envelope created_at.
//     since is inclusive, until is exclusive. Invalid → 400.
//   - limit           — positive integer ≤ explorerEnvelopeListMaxLimit
//     (500). Default explorerEnvelopeListDefaultLimit (50).
//     Malformed or above-max → 400.
//
// Sorting: created_at descending. Items are sorted in the handler since the
// underlying repository (memory.EnvelopeRepo) iterates a map.
//
// Empty result returns 200 with items: [], count: 0, limit: <effective>.
func (s *Server) handleExplorerListEnvelopes(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()

	stateParam := strings.TrimSpace(q.Get("state"))
	if stateParam != "" && !isValidEnvelopeState(envelope.EnvelopeState(stateParam)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid state filter: must be one of received, evaluating, outcome_recorded, escalated, awaiting_review, closed",
		})
		return
	}

	since, err := parseRFC3339Param(q.Get("since"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "since must be an RFC3339 timestamp",
		})
		return
	}
	until, err := parseRFC3339Param(q.Get("until"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "until must be an RFC3339 timestamp",
		})
		return
	}

	limit := explorerEnvelopeListDefaultLimit
	if limitStr := strings.TrimSpace(q.Get("limit")); limitStr != "" {
		parsed, perr := parsePositiveInt(limitStr)
		if perr != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit must be a positive integer",
			})
			return
		}
		if parsed > explorerEnvelopeListMaxLimit {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit exceeds maximum allowed value",
			})
			return
		}
		limit = parsed
	}

	envs, err := s.explorerOrchestrator.ListEnvelopesByState(r.Context(), envelope.EnvelopeState(stateParam))
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope, false)
		writeJSON(w, statusCode, errResp)
		return
	}

	// Filter by created_at against since (inclusive) and until (exclusive),
	// matching the coverage endpoint's documented semantics.
	filtered := make([]*envelope.Envelope, 0, len(envs))
	for _, env := range envs {
		if env == nil {
			continue
		}
		if !since.IsZero() && env.CreatedAt.Before(since) {
			continue
		}
		if !until.IsZero() && !env.CreatedAt.Before(until) {
			continue
		}
		filtered = append(filtered, env)
	}

	// Newest first by created_at. SliceStable so envelopes with identical
	// timestamps keep repository iteration order rather than being shuffled.
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	resp := explorerEnvelopeListResponse{
		Items: make([]explorerEnvelopeSummary, 0, len(filtered)),
		Count: len(filtered),
		Limit: limit,
	}
	for _, env := range filtered {
		resp.Items = append(resp.Items, toExplorerEnvelopeSummary(env))
	}
	writeJSON(w, http.StatusOK, resp)
}

// toExplorerEnvelopeSummary projects an envelope into the compact list DTO.
// Optional fields read from Resolved.Structure / Resolved.Subject are emitted
// only when present; the underlying snapshots are point-in-time evidence and
// may legitimately be empty.
func toExplorerEnvelopeSummary(env *envelope.Envelope) explorerEnvelopeSummary {
	sum := explorerEnvelopeSummary{
		ID:             env.ID(),
		State:          string(env.State),
		RequestID:      env.RequestID(),
		RequestSource:  env.RequestSource(),
		CreatedAt:      env.CreatedAt,
		UpdatedAt:      env.UpdatedAt,
		EvaluatedAt:    env.Evaluation.EvaluatedAt,
		Outcome:        string(env.Evaluation.Outcome),
		ReasonCode:     string(env.Evaluation.ReasonCode),
		SurfaceID:      env.Resolved.Authority.SurfaceID,
		SurfaceVersion: env.Resolved.Authority.SurfaceVersion,
		ProfileID:      env.Resolved.Authority.ProfileID,
		ProfileVersion: env.Resolved.Authority.ProfileVersion,
		GrantID:        env.Resolved.Authority.GrantID,
		AgentID:        env.Resolved.Authority.AgentID,
	}
	if env.Resolved.Subject != nil {
		sum.SubjectID = env.Resolved.Subject.ID
	}
	sum.BusinessServiceID = env.Resolved.Structure.BusinessService.ID
	sum.ProcessID = env.Resolved.Structure.Process.ID
	return sum
}
