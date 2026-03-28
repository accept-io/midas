package oidc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"
)

const (
	pkceCookieName = "midas_oidc_pkce"
	// 32 random bytes → 43+ char verifier (meets RFC 7636 minimum of 43).
	pkceBytes = 32
)

// GeneratePKCE returns a PKCE (RFC 7636) verifier and its S256 challenge.
// verifier is stored server-side (in a cookie); challenge is sent to the provider.
func GeneratePKCE() (verifier, challenge string, err error) {
	b := make([]byte, pkceBytes)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("oidc: generate pkce: %w", err)
	}
	// Verifier: base64url-encoded random bytes (no padding).
	verifier = base64.RawURLEncoding.EncodeToString(b)
	// Challenge: BASE64URL(SHA256(verifier)).
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// SetPKCECookie stores the PKCE verifier in an HttpOnly cookie.
func SetPKCECookie(w http.ResponseWriter, verifier string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     pkceCookieName,
		Value:    verifier,
		Path:     "/auth/oidc",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Now().UTC().Add(10 * time.Minute),
	})
}

// ConsumePKCECookie reads the PKCE verifier cookie and immediately clears it.
// Returns ("", false) when the cookie is absent.
func ConsumePKCECookie(w http.ResponseWriter, r *http.Request, secure bool) (string, bool) {
	cookie, err := r.Cookie(pkceCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     pkceCookieName,
		Value:    "",
		Path:     "/auth/oidc",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
	})
	return cookie.Value, true
}
