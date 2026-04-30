package localiam

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/accept-io/midas/internal/platformauth"
)

// allowedWhenMustChange is the set of paths permitted when must_change_password
// is true. All other routes receive HTTP 403.
var allowedWhenMustChange = map[string]bool{
	"/auth/me":              true,
	"/auth/change-password": true,
	"/auth/logout":          true,
}

// AuthMiddleware returns middleware that reads the session cookie, resolves the
// session, and stores both the principal (via platformauth.WithPrincipal) and
// the must_change_password flag in the request context.
//
// It is extraction-only: it never writes an error response when the session is
// absent or invalid. Enforcement is the responsibility of
// platformauth.RequireAuthenticated, platformauth.RequireRole, and
// EnforceMustChangePassword.
func (s *Service) AuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			_, principal, mustChange, err := s.ResolveSession(r.Context(), cookie.Value)
			if err != nil {
				// Invalid or expired session: clear the stale cookie and continue.
				s.clearCookie(w)
				next.ServeHTTP(w, r)
				return
			}

			ctx := platformauth.WithPrincipal(r.Context(), principal)
			ctx = WithMustChangePassword(ctx, mustChange)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// EnforceMustChangePassword is middleware that blocks requests from principals
// whose must_change_password flag is true, returning HTTP 403, unless the
// request path is one of the explicitly allowed paths:
//
//   - GET  /auth/me
//   - POST /auth/change-password
//   - POST /auth/logout
func EnforceMustChangePassword(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if MustChangePasswordFromContext(r.Context()) && !allowedWhenMustChange[r.URL.Path] {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error":  "password_change_required",
				"detail": "you must change your password before accessing this resource",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SetSessionCookie writes the session cookie to the response.
//
// The Secure attribute is sourced from config. Production-profile deployments
// require it to be true via config.ValidateSemantic when local_iam.enabled is
// true; local HTTP development keeps it false so browsers round-trip the
// cookie over plain HTTP.
func (s *Service) SetSessionCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Production-profile deployments require this to be true via config validation.
		Secure: s.cfg.SecureCookies,
		Path:   "/",
	})
}

// clearCookie clears the session cookie by setting MaxAge = -1.
//
// The Secure attribute mirrors SetSessionCookie so set and clear pairs stay
// coherent across deployment modes.
func (s *Service) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Production-profile deployments require this to be true via config validation.
		Secure: s.cfg.SecureCookies,
		Path:   "/",
	})
}

// ClearSessionCookie is the exported variant used by the logout handler.
func (s *Service) ClearSessionCookie(w http.ResponseWriter) {
	s.clearCookie(w)
}

// writeJSON is the local error-response helper matching the pattern used across
// internal/httpapi: Content-Type application/json, status code, body encoded.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
