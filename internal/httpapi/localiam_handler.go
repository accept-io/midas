package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/accept-io/midas/internal/adminaudit"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/platformauth"
)

// handleAuthLogin processes POST /auth/login.
// Validates username + password, creates a session, sets the session cookie.
// Returns must_change_password so the client can redirect appropriately.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	body, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeStrictJSON(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	sess, user, err := s.localIAM.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, localiam.ErrInvalidCredentials) || errors.Is(err, localiam.ErrUserDisabled) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login failed"})
		return
	}

	s.localIAM.SetSessionCookie(w, sess.ID, sess.ExpiresAt)

	writeJSON(w, http.StatusOK, map[string]any{
		"username":            user.Username,
		"roles":               user.Roles,
		"must_change_password": user.MustChangePassword,
	})
}

// handleAuthLogout processes POST /auth/logout.
// Invalidates the current session and clears the cookie.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	cookie, err := r.Cookie(localiam.SessionCookieName)
	if err == nil && cookie.Value != "" {
		_ = s.localIAM.Logout(r.Context(), cookie.Value)
	}
	s.localIAM.ClearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// handleAuthMe processes GET /auth/me.
// Returns the authenticated principal and must_change_password flag.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	p, ok := platformauth.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                  p.ID,
		"username":            p.Name,
		"roles":               p.Roles,
		"provider":            p.Provider,
		"must_change_password": localiam.MustChangePasswordFromContext(r.Context()),
	})
}

// handleAuthChangePassword processes POST /auth/change-password.
// Requires the current session principal. Verifies current password, sets new
// password, and clears must_change_password.
func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	p, ok := platformauth.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	body, err := readRequestBody(w, r, maxRequestBodyBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeStrictJSON(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Extract the raw user ID from the prefixed principal ID ("localiam:<uuid>").
	userID := strings.TrimPrefix(p.ID, "localiam:")

	if err := s.localIAM.ChangePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, localiam.ErrInvalidCredentials) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
			return
		}
		if errors.Is(err, localiam.ErrWeakPassword) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new password does not meet requirements"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password change failed"})
		return
	}

	// Issue #41: record the fact of the change. Never record password
	// material — only identity, request context, and outcome.
	rec := adminaudit.NewRecord(
		adminaudit.ActionPasswordChanged,
		adminaudit.OutcomeSuccess,
		adminaudit.ActorTypeUser,
	)
	rec.ActorID = p.ID
	rec.TargetType = adminaudit.TargetTypeUser
	rec.TargetID = p.ID
	rec.RequestID = strings.TrimSpace(r.Header.Get("X-Request-Id"))
	rec.ClientIP = clientIPFromRequest(r)
	s.appendAdminAudit(r.Context(), rec)

	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}
