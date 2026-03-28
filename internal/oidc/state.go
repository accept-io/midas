package oidc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

const (
	stateCookieName = "midas_oidc_state"
	// 16 random bytes → 32-char hex state value; sufficient for CSRF protection.
	stateBytes = 16
)

// GenerateState returns a cryptographically secure random state string for
// inclusion in the OIDC authorization request.
func GenerateState() (string, error) {
	b := make([]byte, stateBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oidc: generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SetStateCookie writes the CSRF state to an HttpOnly cookie.
// The cookie expires after 10 minutes — sufficient for any interactive login.
func SetStateCookie(w http.ResponseWriter, state string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/auth/oidc",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Now().UTC().Add(10 * time.Minute),
	})
}

// ConsumeStateCookie reads the state cookie and immediately clears it by
// setting MaxAge = -1. Returns ("", false) when the cookie is absent.
func ConsumeStateCookie(w http.ResponseWriter, r *http.Request, secure bool) (string, bool) {
	cookie, err := r.Cookie(stateCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	// Clear the cookie immediately after reading (one-time use).
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/auth/oidc",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
	})
	return cookie.Value, true
}
