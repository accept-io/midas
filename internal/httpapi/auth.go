package httpapi

import (
	"context"
	"errors"
	"html"
	"log/slog"
	"net/http"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/authz"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/oidc"
	"github.com/accept-io/midas/internal/platformauth"
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

// WithLocalIAM enables local platform IAM (username/password login for the
// Explorer/console). It registers the /auth/* endpoints and wires the session
// authenticator. Call before WithExplorerEnabled to ensure session auth is
// applied to Explorer routes; calling order is otherwise flexible because
// Explorer route handlers check s.localIAM at request time.
//
// The /v1/* routes and StaticTokenAuthenticator are not affected.
func (s *Server) WithLocalIAM(svc *localiam.Service) *Server {
	s.localIAM = svc
	// Auth endpoints are always open for login; me/change-password/logout
	// rely on the session middleware applied inside their handlers.
	s.mux.HandleFunc("/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("/auth/me", s.localIAM.AuthMiddleware()(
		http.HandlerFunc(s.handleAuthMe),
	).ServeHTTP)
	s.mux.HandleFunc("/auth/change-password", s.localIAM.AuthMiddleware()(
		http.HandlerFunc(s.handleAuthChangePassword),
	).ServeHTTP)
	return s
}

// WithOIDC enables OIDC-based platform login. It registers /auth/oidc/login
// and /auth/oidc/callback. WithLocalIAM must be called first because session
// creation is delegated to the local IAM service.
//
// secureCookies should match cfg.LocalIAM.SecureCookies so that the OIDC
// helper cookies (state, PKCE) use the same Secure flag as the session cookie.
//
// The /v1/* routes and StaticTokenAuthenticator are not affected.
func (s *Server) WithOIDC(svc oidcProvider, secureCookies bool) *Server {
	s.oidcService = svc
	s.secureCookiesFlag = secureCookies
	s.mux.HandleFunc("/auth/oidc/login", s.handleOIDCLogin)
	s.mux.HandleFunc("/auth/oidc/callback", s.handleOIDCCallback)
	return s
}

// handleOIDCLogin initiates the OIDC authorization code flow.
// It generates a CSRF state, optionally a PKCE pair, and redirects to the
// identity provider.
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.oidcService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "oidc not configured"})
		return
	}

	secure := s.localIAM != nil && s.secureCookies()

	state, err := oidc.GenerateState()
	if err != nil {
		slog.Error("oidc_state_gen_failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	oidc.SetStateCookie(w, state, secure)

	var pkceChallenge string
	if s.oidcService.UsePKCE() {
		verifier, challenge, err := oidc.GeneratePKCE()
		if err != nil {
			slog.Error("oidc_pkce_gen_failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		oidc.SetPKCECookie(w, verifier, secure)
		pkceChallenge = challenge
	}

	authURL := s.oidcService.AuthURL(state, pkceChallenge)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCCallback handles the authorization code callback from the provider.
// It validates state, exchanges the code, builds a principal, and creates a
// session using the local IAM service before redirecting to the Explorer.
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.oidcService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "oidc not configured"})
		return
	}
	if s.localIAM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session service not configured"})
		return
	}

	secure := s.secureCookies()

	// 1. Validate CSRF state.
	receivedState := r.URL.Query().Get("state")
	expectedState, ok := oidc.ConsumeStateCookie(w, r, secure)
	if !ok || receivedState == "" || receivedState != expectedState {
		slog.Warn("oidc_invalid_state",
			"received", receivedState,
			"cookie_present", ok,
		)
		oidcError(w, "invalid_state", "Login state mismatch — please try again.", http.StatusBadRequest)
		return
	}

	// 2. Read and discard PKCE verifier cookie (empty string = PKCE disabled).
	pkceVerifier, _ := oidc.ConsumePKCECookie(w, r, secure)

	// 3. Check for provider-side errors.
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		slog.Warn("oidc_provider_error", "error", errParam, "description", desc)
		oidcError(w, "provider_error", "Login could not be completed. Please try again.", http.StatusUnauthorized)
		return
	}

	// 4. Exchange code for tokens and validate ID token.
	code := r.URL.Query().Get("code")
	if code == "" {
		oidcError(w, "missing_code", "Authorization code not received.", http.StatusBadRequest)
		return
	}

	claims, err := s.oidcService.Exchange(r.Context(), code, pkceVerifier)
	if err != nil {
		slog.Error("oidc_exchange_failed", "error", err)
		oidcError(w, "exchange_failed", "Token exchange failed. Please try again.", http.StatusUnauthorized)
		return
	}

	// 5. Build principal (enforces allowed_groups + deny_if_no_roles).
	principal, err := s.oidcService.BuildPrincipal(claims)
	if err != nil {
		slog.Warn("oidc_principal_denied",
			"subject", claims.Subject,
			"username", claims.Username,
			"error", err,
		)
		oidcError(w, "access_denied", "Your account is not authorised to access this system.", http.StatusForbidden)
		return
	}

	// 6. Create MIDAS session (same session model as local IAM).
	sess, err := s.localIAM.CreateExternalSession(r.Context(), principal)
	if err != nil {
		slog.Error("oidc_session_create_failed", "error", err)
		oidcError(w, "session_error", "Failed to create session. Please try again.", http.StatusInternalServerError)
		return
	}

	// 7. Set session cookie and redirect to Explorer.
	s.localIAM.SetSessionCookie(w, sess.ID, sess.ExpiresAt)
	slog.Info("oidc_login_success",
		"subject", principal.Subject,
		"username", principal.Name,
		"roles", principal.Roles,
	)
	http.Redirect(w, r, "/explorer", http.StatusFound)
}

// secureCookies returns true when the localIAM service is configured with
// secure cookies. Used for OIDC helper cookies (state, PKCE) to match the
// same Secure flag as the session cookie.
func (s *Server) secureCookies() bool {
	// Delegate to localIAM config. If localIAM is nil, default to false (dev mode).
	// We access the flag indirectly by checking if a secure-cookie test cookie
	// can be created; instead we track it on Server directly from WithLocalIAMSecure.
	// For now, use the localIAM reference — its SetSessionCookie already uses the
	// right value; we mirror that via a dedicated field populated by WithOIDC callers.
	return s.secureCookiesFlag
}

// oidcError writes a human-readable OIDC error response. Since the callback is
// a browser redirect, we render a minimal HTML page rather than JSON.
func oidcError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`<!doctype html><html><head><title>Login error</title></head><body>` +
		`<h2>Login failed</h2><p>` + html.EscapeString(message) + `</p>` +
		`<p><a href="/explorer">Return to Explorer</a></p>` +
		`</body></html>`))
	slog.Debug("oidc_error_response", "code", code, "status", status)
}

// WithExplorerEnabled registers the /explorer routes when enabled is true.
// Call this at startup with cfg.Server.ExplorerEnabled after NewServerFull.
// When false (or never called) the explorer routes are not registered and
// requests to /explorer return 404.
//
// When WithLocalIAM has been called, Explorer POST routes use session-cookie
// auth (via the localiam AuthMiddleware). Otherwise they fall back to the
// existing bearer-token requireAuth/requireRole path unchanged.
func (s *Server) WithExplorerEnabled(enabled bool) *Server {
	s.explorerEnabled = enabled
	if enabled {
		// Build the Explorer's isolated in-memory runtime (seeded unconditionally).
		s.initExplorerRuntime()
		s.mux.HandleFunc("GET /explorer", s.explorerShellHandler(s.handleExplorerIndex))
		s.mux.HandleFunc("POST /explorer", s.explorerAuthHandler(s.handleExplorerEvaluate))
		s.mux.HandleFunc("POST /explorer/simulate", s.explorerAuthHandler(s.handleExplorerSimulate))
		s.mux.HandleFunc("GET /explorer/config", s.handleExplorerConfig)
		s.mux.HandleFunc("GET /explorer/envelopes/", s.explorerReadAuthHandler(s.handleExplorerGetEnvelope))
		s.mux.HandleFunc("GET /explorer/", s.handleExplorerAssets)
	}
	return s
}

// explorerShellHandler applies session extraction to GET /explorer when local
// IAM is active. It does not enforce authentication — the shell contains the
// login overlay which is the primary login UX. The middleware is applied so
// the server makes an active, intentional auth decision on every shell request
// rather than serving blindly as anonymous public content.
func (s *Server) explorerShellHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.localIAM != nil {
			s.localIAM.AuthMiddleware()(http.HandlerFunc(h)).ServeHTTP(w, r)
		} else {
			h(w, r)
		}
	}
}

// explorerAuthHandler wraps an Explorer handler with authentication and
// must_change_password enforcement. When localIAM is configured, session-cookie
// auth is used with operator-or-admin role enforcement. Otherwise the existing
// bearer-token requireAuth + requireRole path is used unchanged.
func (s *Server) explorerAuthHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.localIAM != nil {
			s.localIAM.AuthMiddleware()(
				localiam.EnforceMustChangePassword(
					platformauth.RequireRole(identity.RolePlatformOperator, identity.RolePlatformAdmin)(
						http.HandlerFunc(h),
					),
				),
			).ServeHTTP(w, r)
		} else {
			s.requireAuth(s.requireRole(identity.RolePlatformOperator, identity.RolePlatformAdmin)(h))(w, r)
		}
	}
}

// explorerReadAuthHandler wraps an Explorer read handler with authentication
// and must_change_password enforcement, without role restriction.
func (s *Server) explorerReadAuthHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.localIAM != nil {
			s.localIAM.AuthMiddleware()(
				localiam.EnforceMustChangePassword(
					platformauth.RequireAuthenticated(
						http.HandlerFunc(h),
					),
				),
			).ServeHTTP(w, r)
		} else {
			s.requireAuth(h)(w, r)
		}
	}
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

// WithSeedDemoUser records whether the demo Local IAM user (demo/demo) is
// seeded at startup, so the Explorer login panel can display a contextual hint.
func (s *Server) WithSeedDemoUser(v bool) *Server {
	s.seedDemoUser = v
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

// requirePermission returns middleware that enforces a single scoped
// permission (see internal/authz) on a control-plane write endpoint. It is
// the fine-grained replacement for requireRole on write paths.
//
// Composition: requirePermission must be wrapped inside requireAuth so that
// the principal is already in context. In AuthModeOpen the middleware is a
// no-op: requests are forwarded without inspecting the principal, preserving
// the dev/memory-store experience in open deployments. This mirrors the
// short-circuit in requireRole at this file.
//
// Denial semantics:
//   - no principal present (authenticated mode) → 401 with {"error":"unauthorized"}
//   - principal present but lacking the required permission →
//     403 with {"error":"forbidden","required_permission":"<perm>"}
//
// The 403 body additively carries the required permission string. It does
// not leak which permissions the caller already holds.
//
// Principal → permission resolution goes through authz.HasPermission, which
// reads the principal's normalised Roles slice. The bootstrap admin user
// stored with the deprecated alias "admin" (identity.RoleAdmin) is already
// normalised to "platform.admin" at principal-construction time, so its
// resolution here is unaffected.
func (s *Server) requirePermission(perm authz.Permission) func(http.HandlerFunc) http.HandlerFunc {
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
					"required_permission", string(perm),
				)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}

			if !authz.HasPermission(p, perm) {
				slog.Warn("authz_forbidden",
					"method", r.Method,
					"path", r.URL.Path,
					"principal_id", p.ID,
					"required_permission", string(perm),
				)
				writeJSON(w, http.StatusForbidden, map[string]any{
					"error":               "forbidden",
					"required_permission": string(perm),
				})
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
