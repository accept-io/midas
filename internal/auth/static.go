package auth

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/accept-io/midas/internal/identity"
)

// StaticTokenAuthenticator authenticates requests by looking up a bearer token
// in a pre-configured token-to-principal map. This is the community-edition
// authentication implementation. Enterprise deployments will supply Entra or
// Ping implementations that satisfy the same Authenticator interface.
type StaticTokenAuthenticator struct {
	// tokens maps bearer token value → verified Principal.
	// The map is read-only after construction; no locking is needed.
	tokens map[string]*identity.Principal
}

// Compile-time assertion that *StaticTokenAuthenticator implements Authenticator.
var _ Authenticator = (*StaticTokenAuthenticator)(nil)

// NewStaticTokenAuthenticator constructs an authenticator from the provided
// token map. tokens must not be nil; use an empty map to accept no requests.
func NewStaticTokenAuthenticator(tokens map[string]*identity.Principal) *StaticTokenAuthenticator {
	if tokens == nil {
		tokens = make(map[string]*identity.Principal)
	}
	return &StaticTokenAuthenticator{tokens: tokens}
}

// Authenticate extracts the bearer token from the Authorization header and
// returns the associated Principal. Returns ErrNoCredentials when no header is
// present; returns a descriptive error when the token is present but unknown.
func (a *StaticTokenAuthenticator) Authenticate(r *http.Request) (*identity.Principal, error) {
	token, ok := parseBearerToken(r.Header.Get("Authorization"))
	if !ok {
		return nil, ErrNoCredentials
	}

	p, found := a.tokens[token]
	if !found {
		return nil, fmt.Errorf("auth: unknown token")
	}

	return p, nil
}

// parseBearerToken extracts the token value from a "Bearer <token>" header.
// Returns ("", false) when the header is absent or not a Bearer scheme.
func parseBearerToken(authHeader string) (string, bool) {
	authHeader = strings.TrimSpace(authHeader)
	if authHeader == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", false
	}
	token := strings.TrimSpace(authHeader[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// LoadStaticTokensFromEnv reads the MIDAS_AUTH_TOKENS environment variable and
// returns a configured StaticTokenAuthenticator. Returns nil when the variable
// is unset or empty — callers should skip auth wiring in that case.
//
// Format: semicolon-separated entries, each with the form:
//
//	token|principal-id|role1,role2
//
// The pipe character (|) separates the three fields so that principal IDs may
// contain colons (e.g. "user:alice", "svc:payments-engine").
//
// Example:
//
//	MIDAS_AUTH_TOKENS="secret-1|user:alice|admin,approver;secret-2|svc:deploy|operator"
func LoadStaticTokensFromEnv() (*StaticTokenAuthenticator, error) {
	raw := strings.TrimSpace(os.Getenv("MIDAS_AUTH_TOKENS"))
	if raw == "" {
		return nil, nil
	}

	tokens := make(map[string]*identity.Principal)
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Format: token|principal-id|role1,role2
		parts := strings.SplitN(entry, "|", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("auth: malformed token entry %q (want token|principal-id[|roles])", entry)
		}

		token := strings.TrimSpace(parts[0])
		principalID := strings.TrimSpace(parts[1])

		if token == "" {
			return nil, fmt.Errorf("auth: empty token in entry %q", entry)
		}
		if principalID == "" {
			return nil, fmt.Errorf("auth: empty principal id in entry %q", entry)
		}

		var roles []string
		if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
			for _, r := range strings.Split(parts[2], ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					roles = append(roles, r)
				}
			}
		}

		tokens[token] = &identity.Principal{
			ID:       principalID,
			Subject:  principalID,
			Roles:    identity.NormalizeRoles(roles),
			Provider: identity.ProviderStatic,
		}
	}

	if len(tokens) == 0 {
		return nil, nil
	}

	return NewStaticTokenAuthenticator(tokens), nil
}
