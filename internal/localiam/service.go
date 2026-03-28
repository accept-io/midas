package localiam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/accept-io/midas/internal/identity"
)

// ProviderLocalIAM is the identity.Principal.Provider value set when a
// principal is resolved from a local IAM session.
const ProviderLocalIAM = "localiam"

// SessionCookieName is the HTTP cookie name used to carry the session ID.
const SessionCookieName = "midas_session"

const (
	bootstrapUsername    = "admin"
	bootstrapPassword    = "admin"
	defaultSessionTTL    = 8 * time.Hour
	bcryptCost           = bcrypt.DefaultCost
)

// Sentinel errors returned by Service methods.
var (
	ErrInvalidCredentials = errors.New("localiam: invalid username or password")
	ErrUserDisabled       = errors.New("localiam: user account is disabled")
	ErrUserNotFound       = errors.New("localiam: user not found")
	ErrSessionNotFound    = errors.New("localiam: session not found or expired")
	ErrWeakPassword       = errors.New("localiam: password does not meet requirements")
)

// Config holds localiam runtime configuration.
type Config struct {
	// Enabled controls whether local IAM is active. When false, the service
	// should not be constructed.
	Enabled bool
	// SessionTTL is how long a session remains valid. Defaults to 8 hours.
	SessionTTL time.Duration
	// SecureCookies sets the Secure flag on session cookies. Enable in
	// production (HTTPS). Disable only for local HTTP development.
	SecureCookies bool
}

// Service provides local platform IAM operations: bootstrap, login, logout,
// session resolution, and password management. It is the only component that
// touches the user and session repositories.
type Service struct {
	users    UserRepository
	sessions SessionRepository
	cfg      Config
}

// NewService constructs a Service. sessionTTL defaults to 8 hours when zero.
func NewService(users UserRepository, sessions SessionRepository, cfg Config) *Service {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	return &Service{users: users, sessions: sessions, cfg: cfg}
}

// Bootstrap creates the default admin user if no users exist. It is safe to
// call on every startup — it is a no-op when at least one user already exists.
func (s *Service) Bootstrap(ctx context.Context) error {
	n, err := s.users.Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil // already bootstrapped
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(bootstrapPassword), bcryptCost)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	u := &User{
		ID:                 generateID(),
		Username:           bootstrapUsername,
		PasswordHash:       string(hash),
		Roles:              []string{identity.RoleAdmin},
		Enabled:            true,
		MustChangePassword: true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.users.Create(ctx, u); err != nil {
		return err
	}

	slog.Info("localiam_bootstrap_complete",
		"username", bootstrapUsername,
		"must_change_password", true,
		"message", "default admin/admin credentials created — change password on first login",
	)
	return nil
}

// Login validates credentials, creates a session, and returns the session and
// user. Returns ErrInvalidCredentials on bad username or password.
func (s *Service) Login(ctx context.Context, username, password string) (*Session, *User, error) {
	u, err := s.users.FindByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return nil, nil, err
	}
	if u == nil {
		// Constant-time stub to prevent username enumeration via timing.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$invalid"), []byte(password))
		return nil, nil, ErrInvalidCredentials
	}
	if !u.Enabled {
		return nil, nil, ErrUserDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	now := time.Now().UTC()
	sess := &Session{
		ID:        generateSessionID(),
		UserID:    u.ID,
		CreatedAt: now,
		ExpiresAt: now.Add(s.cfg.SessionTTL),
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, nil, err
	}

	// Record last login — best-effort; failure does not abort login.
	u.LastLoginAt = &now
	u.UpdatedAt = now
	_ = s.users.Update(ctx, u)

	return sess, u, nil
}

// ResolveSession looks up a session by ID and returns the associated session,
// the resolved principal, and whether the user must change their password.
// Returns ErrSessionNotFound when the session is absent or expired.
//
// For external-auth sessions (OIDC) where PrincipalJSON is populated, the
// principal is decoded from JSON and no local user lookup is performed.
// mustChangePassword is always false for external sessions.
func (s *Service) ResolveSession(ctx context.Context, sessionID string) (*Session, *identity.Principal, bool, error) {
	sess, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return nil, nil, false, err
	}
	if sess == nil || time.Now().UTC().After(sess.ExpiresAt) {
		if sess != nil {
			_ = s.sessions.Delete(ctx, sess.ID) // clean up expired session
		}
		return nil, nil, false, ErrSessionNotFound
	}

	// External-auth session: principal is stored inline.
	if sess.PrincipalJSON != "" {
		var p identity.Principal
		if err := json.Unmarshal([]byte(sess.PrincipalJSON), &p); err != nil {
			return nil, nil, false, fmt.Errorf("localiam: decode external principal: %w", err)
		}
		return sess, &p, false, nil
	}

	u, err := s.users.FindByID(ctx, sess.UserID)
	if err != nil {
		return nil, nil, false, err
	}
	if u == nil {
		return nil, nil, false, ErrUserNotFound
	}

	return sess, UserToPrincipal(u), u.MustChangePassword, nil
}

// CreateExternalSession creates a session for an externally-authenticated
// principal (e.g. OIDC). No local user record is required. The principal is
// stored as JSON in the session and is decoded on each ResolveSession call.
// Callers must validate and normalize the principal before calling this.
func (s *Service) CreateExternalSession(ctx context.Context, p *identity.Principal) (*Session, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("localiam: marshal external principal: %w", err)
	}
	now := time.Now().UTC()
	sess := &Session{
		ID:            generateSessionID(),
		UserID:        "",
		CreatedAt:     now,
		ExpiresAt:     now.Add(s.cfg.SessionTTL),
		PrincipalJSON: string(data),
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Logout deletes the session identified by sessionID.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

// ChangePassword verifies currentPassword, then replaces the hash with a
// bcrypt hash of newPassword and clears MustChangePassword.
//
// Password policy:
//   - newPassword must not be empty
//   - newPassword must not equal the literal string "admin"
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return ErrWeakPassword
	}
	if newPassword == bootstrapPassword {
		return ErrWeakPassword
	}

	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return err
	}

	u.PasswordHash = string(hash)
	u.MustChangePassword = false
	u.UpdatedAt = time.Now().UTC()
	return s.users.Update(ctx, u)
}

// UserToPrincipal converts a local IAM User to an *identity.Principal.
// The Principal.ID is prefixed with "localiam:" to namespace it from other
// provider namespaces (e.g. "static:", "entra:").
func UserToPrincipal(u *User) *identity.Principal {
	return &identity.Principal{
		ID:       "localiam:" + u.ID,
		Subject:  u.Username,
		Name:     u.Username,
		Roles:    identity.NormalizeRoles(u.Roles),
		Provider: ProviderLocalIAM,
	}
}

// generateID returns a new random hex ID for users.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("localiam: crypto/rand failure: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// generateSessionID returns a cryptographically secure 32-byte random hex string.
func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("localiam: crypto/rand failure: " + err.Error())
	}
	return hex.EncodeToString(b)
}
