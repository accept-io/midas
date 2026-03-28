package localiam

import (
	"context"
	"time"
)

// Session is a server-side platform IAM session. Sessions are stored
// server-side and identified by a cryptographically secure random ID
// transmitted via an HTTP-only cookie. No JWT or refresh tokens.
type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	// PrincipalJSON holds a JSON-encoded *identity.Principal for sessions
	// created by external auth providers (e.g. OIDC). When set, UserID is
	// empty and no local user record is required. Ignored for local IAM sessions.
	PrincipalJSON string
}

// SessionRepository defines persistence for platform IAM sessions.
type SessionRepository interface {
	// Create stores a new session.
	Create(ctx context.Context, s *Session) error
	// FindByID returns nil, nil when not found.
	FindByID(ctx context.Context, id string) (*Session, error)
	// Delete removes a single session (logout).
	Delete(ctx context.Context, id string) error
	// DeleteExpired removes all sessions whose ExpiresAt is before now.
	// Called opportunistically; failures are non-fatal.
	DeleteExpired(ctx context.Context) error
}
