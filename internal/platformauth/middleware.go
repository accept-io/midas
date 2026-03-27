package platformauth

import (
	"encoding/json"
	"net/http"

	"github.com/accept-io/midas/internal/auth"
)

// Authenticate returns middleware that extracts a principal from the request
// using the provided authenticator and stores it in the request context via
// WithPrincipal. It is extraction-only: it never writes an error response,
// never logs, and never blocks a request — enforcement is the responsibility of
// RequireAuthenticated and RequireRole.
//
// Special cases:
//   - authenticator is nil: passes through unchanged.
//   - principal already present in context: does not overwrite; passes through.
//   - authenticator returns (nil, nil): passes through without storing.
//   - authenticator returns an error: passes through without storing.
func Authenticate(authenticator auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Do not overwrite a principal already placed by an earlier middleware.
			if _, ok := PrincipalFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}

			p, err := authenticator.Authenticate(r)
			if err != nil {
				// Extraction only — no enforcement, no response, no log.
				next.ServeHTTP(w, r)
				return
			}
			if p != nil {
				r = r.WithContext(WithPrincipal(r.Context(), p))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuthenticated is middleware that returns HTTP 401 when no principal is
// present in the request context. It must be composed after Authenticate.
func RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := PrincipalFromContext(r.Context()); !ok {
			writeErrorJSON(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole returns middleware that enforces role-based access control.
// It must be composed after Authenticate (and optionally RequireAuthenticated).
//
// Panics at construction time (not request time) when called with zero roles —
// this is a programmer error and should be caught during startup.
//
// At request time:
//   - no principal in context → 401
//   - principal present but lacks all required roles → 403
//   - principal has at least one required role → calls next
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	if len(roles) == 0 {
		panic("platformauth: RequireRole called with zero roles; at least one role must be specified")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFromContext(r.Context())
			if !ok {
				writeErrorJSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !p.HasAnyRole(roles...) {
				writeErrorJSON(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeErrorJSON writes a JSON error response using the same pattern as
// internal/httpapi: Content-Type application/json, numeric status, {"error": msg}.
func writeErrorJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
