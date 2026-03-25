package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/identity"
)

// contextKey is an unexported type for context keys owned by this package.
// Using a package-local type prevents collisions with keys from other packages.
type contextKey int

const principalContextKey contextKey = iota

// WithPolicyMeta attaches policy mode metadata to the server for use in health
// and evaluate responses. Call this at boot after detecting the active evaluator.
// mode is a short string like "noop"; evaluatorName is a human-readable label.
func (s *Server) WithPolicyMeta(mode, evaluatorName string) *Server {
	s.policyMode = mode
	s.policyEvaluatorName = evaluatorName
	return s
}

// WithHealthCheck sets a function that handleReady calls to verify the
// backing store is reachable. Return nil means ready; any error causes /readyz
// to respond 503. Pass nil to treat the server as always ready (memory mode).
func (s *Server) WithHealthCheck(fn func(context.Context) error) *Server {
	s.readyFn = fn
	return s
}

// WithAuthenticator configures the server to authenticate governance requests.
// It is safe to call after NewServerFull because requireAuth reads s.authenticator
// at request time rather than at route-registration time.
// Returns the server to allow builder-style chaining.
func (s *Server) WithAuthenticator(a auth.Authenticator) *Server {
	s.authenticator = a
	return s
}

// WithAuthMode sets the authentication mode for the server.
// Must always be called at startup with the value from config (cfg.Auth.Mode).
// config.AuthModeOpen — requests pass through without authentication.
// config.AuthModeRequired — all governed routes require a valid bearer token.
func (s *Server) WithAuthMode(mode config.AuthMode) *Server {
	s.authMode = mode
	return s
}

// WithExplorerEnabled registers the /explorer routes when enabled is true.
// Call this at startup with cfg.Server.ExplorerEnabled after NewServerFull.
// When false (or never called) the explorer routes are not registered and
// requests to /explorer return 404.
func (s *Server) WithExplorerEnabled(enabled bool) *Server {
	s.explorerEnabled = enabled
	if enabled {
		// Build the Explorer's isolated in-memory runtime (seeded unconditionally).
		s.initExplorerRuntime()
		s.mux.HandleFunc("GET /explorer", s.handleExplorerIndex)
		s.mux.HandleFunc("POST /explorer", s.requireAuth(s.requireRole(identity.RoleOperator, identity.RoleAdmin)(s.handleExplorerEvaluate)))
		s.mux.HandleFunc("GET /explorer/config", s.handleExplorerConfig)
		s.mux.HandleFunc("GET /explorer/", s.handleExplorerAssets)
	}
	return s
}

// WithStoreBackend records the active store backend (e.g. "memory", "postgres")
// so the Explorer config endpoint can surface it to the UI.
func (s *Server) WithStoreBackend(backend string) *Server {
	s.storeBackend = backend
	return s
}

// WithDemoSeeded records whether demo data was successfully seeded at startup,
// so the Explorer config endpoint can tell the UI which scenarios are ready.
func (s *Server) WithDemoSeeded(seeded bool) *Server {
	s.explorerDemoSeeded = &seeded
	return s
}

// PrincipalFromContext retrieves the verified Principal that requireAuth stored
// in the request context. Returns nil when no principal is present (i.e. the
// authenticator was not configured or the middleware was not applied).
func PrincipalFromContext(ctx context.Context) *identity.Principal {
	p, _ := ctx.Value(principalContextKey).(*identity.Principal)
	return p
}

// actorFromContext returns the authenticated principal's ID when auth is active,
// or falls back to the caller-supplied value when no principal is in the context.
// This provides backward compatibility: unauthenticated deployments continue to
// accept actor identifiers from the request body.
func actorFromContext(ctx context.Context, fallback string) string {
	if p := PrincipalFromContext(ctx); p != nil {
		return p.ID
	}
	return fallback
}

// requireRole returns middleware that enforces role-based access control.
// It must be composed inside requireAuth so that the principal is already in context.
//
// In AuthModeOpen the middleware is a no-op: requests are forwarded without a
// principal so that dev/memory deployments can call all routes without tokens.
// In all other modes a principal must be present and hold at least one of the
// required roles; missing principal → 401, wrong role → 403.
func (s *Server) requireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if s.authMode == config.AuthModeOpen {
				next(w, r)
				return
			}

			p := PrincipalFromContext(r.Context())
			if p == nil {
				slog.Warn("authz_no_principal",
					"method", r.Method,
					"path", r.URL.Path,
					"required_roles", roles,
				)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}

			if !p.HasAnyRole(roles...) {
				slog.Warn("authz_forbidden",
					"method", r.Method,
					"path", r.URL.Path,
					"principal_id", p.ID,
					"required_roles", roles,
				)
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}

			next(w, r)
		}
	}
}

// requireAuth returns a handler that enforces authentication before forwarding
// to next.
//
// When authMode is AuthModeOpen the middleware is a no-op: requests are
// forwarded without a principal in context. In all other cases a valid bearer
// token is required. If no authenticator is configured the request is rejected
// with 401 — a server without an authenticator must not silently pass requests.
//
// On authentication failure the handler writes 401 and logs the event; on
// success it stores the verified principal in the request context so that
// handlers can retrieve it via PrincipalFromContext.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authMode == config.AuthModeOpen {
			next(w, r)
			return
		}
		if s.authenticator == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		principal, err := s.authenticator.Authenticate(r)
		if err != nil {
			if errors.Is(err, auth.ErrNoCredentials) {
				slog.Warn("auth_no_credentials",
					"method", r.Method,
					"path", r.URL.Path,
				)
			} else {
				slog.Warn("auth_failed",
					"method", r.Method,
					"path", r.URL.Path,
					"error", err,
				)
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		slog.Debug("auth_ok",
			"principal_id", principal.ID,
			"provider", principal.Provider,
			"method", r.Method,
			"path", r.URL.Path,
		)

		ctx := context.WithValue(r.Context(), principalContextKey, principal)
		next(w, r.WithContext(ctx))
	}
}
