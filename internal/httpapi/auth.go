package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/accept-io/midas/internal/auth"
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

// WithAuthenticator configures the server to authenticate governance requests.
// It is safe to call after NewServerFull because requireAuth reads s.authenticator
// at request time rather than at route-registration time.
// Returns the server to allow builder-style chaining.
func (s *Server) WithAuthenticator(a auth.Authenticator) *Server {
	s.authenticator = a
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
// When s.authenticator is nil the check is a no-op, preserving backward
// compatibility with unauthenticated deployments (matching requireAuth behaviour).
// When a principal is present but holds none of the required roles, 403 is returned.
// When no principal is present on a protected route, 401 is returned.
func (s *Server) requireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if s.authenticator == nil {
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
// to next. When s.authenticator is nil the middleware is a no-op, preserving
// backward compatibility for deployments that have not configured auth.
//
// On authentication failure the handler writes 401 and logs the event; on
// success it stores the verified principal in the request context so that
// handlers can retrieve it via PrincipalFromContext.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authenticator == nil {
			next(w, r)
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
