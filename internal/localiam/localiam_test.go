package localiam

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/identity"
)

// ---------------------------------------------------------------------------
// In-process memory repos for testing (no external dependencies)
// ---------------------------------------------------------------------------

func newTestService(t *testing.T) *Service {
	t.Helper()
	users := &memUserRepo{byID: map[string]*User{}, byUsername: map[string]*User{}}
	sessions := &memSessionRepo{items: map[string]*Session{}}
	return NewService(users, sessions, Config{SessionTTL: time.Hour})
}

// minimal in-process repos — intentionally simple
type memUserRepo struct {
	byID       map[string]*User
	byUsername map[string]*User
}

func (r *memUserRepo) FindByID(_ context.Context, id string) (*User, error) {
	u := r.byID[id]
	if u == nil {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}
func (r *memUserRepo) FindByUsername(_ context.Context, username string) (*User, error) {
	u := r.byUsername[username]
	if u == nil {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}
func (r *memUserRepo) Create(_ context.Context, u *User) error {
	if _, ok := r.byUsername[u.Username]; ok {
		return ErrUserNotFound // reuse sentinel for unique violation test
	}
	cp := *u
	r.byID[u.ID] = &cp
	r.byUsername[u.Username] = &cp
	return nil
}
func (r *memUserRepo) Update(_ context.Context, u *User) error {
	cp := *u
	r.byID[u.ID] = &cp
	r.byUsername[u.Username] = &cp
	return nil
}
func (r *memUserRepo) Count(_ context.Context) (int, error) { return len(r.byID), nil }

type memSessionRepo struct{ items map[string]*Session }

func (r *memSessionRepo) Create(_ context.Context, s *Session) error {
	cp := *s
	r.items[s.ID] = &cp
	return nil
}
func (r *memSessionRepo) FindByID(_ context.Context, id string) (*Session, error) {
	s := r.items[id]
	if s == nil {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}
func (r *memSessionRepo) Delete(_ context.Context, id string) error {
	delete(r.items, id)
	return nil
}
func (r *memSessionRepo) DeleteExpired(_ context.Context) error {
	now := time.Now().UTC()
	for id, s := range r.items {
		if now.After(s.ExpiresAt) {
			delete(r.items, id)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------

func TestBootstrap_CreatesAdminUser(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if err := svc.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	n, _ := svc.users.Count(ctx)
	if n != 1 {
		t.Errorf("want 1 user after bootstrap, got %d", n)
	}

	u, err := svc.users.FindByUsername(ctx, bootstrapUsername)
	if err != nil || u == nil {
		t.Fatalf("want admin user, got nil (err=%v)", err)
	}
	if !u.MustChangePassword {
		t.Error("want must_change_password=true for bootstrap user")
	}
	if !u.Enabled {
		t.Error("want enabled=true for bootstrap user")
	}
}

func TestBootstrap_Idempotent_NoDuplicateUser(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if err := svc.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.Bootstrap(ctx); err != nil {
		t.Fatalf("second Bootstrap should be no-op, got: %v", err)
	}

	n, _ := svc.users.Count(ctx)
	if n != 1 {
		t.Errorf("want exactly 1 user after 2 Bootstrap calls, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

func TestLogin_ValidCredentials_ReturnsSession(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	sess, user, err := svc.Login(ctx, bootstrapUsername, bootstrapPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if sess == nil || sess.ID == "" {
		t.Error("want non-empty session ID")
	}
	if user == nil || user.Username != bootstrapUsername {
		t.Error("want admin user returned")
	}
	if sess.UserID != user.ID {
		t.Error("session UserID must match user ID")
	}
	if sess.ExpiresAt.IsZero() {
		t.Error("want non-zero ExpiresAt")
	}
}

func TestLogin_BadPassword_ReturnsInvalidCredentials(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	_, _, err := svc.Login(ctx, bootstrapUsername, "wrong-password")
	if err != ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UnknownUser_ReturnsInvalidCredentials(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, _, err := svc.Login(ctx, "nobody", "password")
	if err != ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials for unknown user, got %v", err)
	}
}

func TestLogin_DisabledUser_ReturnsUserDisabled(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	// Disable the user.
	u, _ := svc.users.FindByUsername(ctx, bootstrapUsername)
	u.Enabled = false
	_ = svc.users.Update(ctx, u)

	_, _, err := svc.Login(ctx, bootstrapUsername, bootstrapPassword)
	if err != ErrUserDisabled {
		t.Errorf("want ErrUserDisabled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Session resolution
// ---------------------------------------------------------------------------

func TestResolveSession_ValidSession_ReturnsUser(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	sess, _, _ := svc.Login(ctx, bootstrapUsername, bootstrapPassword)

	gotSess, principal, _, err := svc.ResolveSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ResolveSession: %v", err)
	}
	if gotSess.ID != sess.ID {
		t.Error("session ID mismatch")
	}
	if principal.Name != bootstrapUsername {
		t.Error("principal name mismatch")
	}
}

func TestResolveSession_UnknownID_ReturnsNotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, _, _, err := svc.ResolveSession(ctx, "nonexistent-session-id")
	if err != ErrSessionNotFound {
		t.Errorf("want ErrSessionNotFound, got %v", err)
	}
}

func TestResolveSession_ExpiredSession_ReturnsNotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	sess, _, _ := svc.Login(ctx, bootstrapUsername, bootstrapPassword)

	// Manually expire the session.
	stored, _ := svc.sessions.FindByID(ctx, sess.ID)
	stored.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	_ = svc.sessions.Create(ctx, stored) // overwrite (memRepo allows re-create)

	_, _, _, err := svc.ResolveSession(ctx, sess.ID)
	if err != ErrSessionNotFound {
		t.Errorf("want ErrSessionNotFound for expired session, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Logout
// ---------------------------------------------------------------------------

func TestLogout_InvalidatesSession(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	sess, _, _ := svc.Login(ctx, bootstrapUsername, bootstrapPassword)
	if err := svc.Logout(ctx, sess.ID); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	_, _, _, err := svc.ResolveSession(ctx, sess.ID)
	if err != ErrSessionNotFound {
		t.Errorf("want ErrSessionNotFound after logout, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Password validation
// ---------------------------------------------------------------------------

func TestChangePassword_Success_ClearsMustChange(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	u, _ := svc.users.FindByUsername(ctx, bootstrapUsername)
	if err := svc.ChangePassword(ctx, u.ID, bootstrapPassword, "newSecurePass99!"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	updated, _ := svc.users.FindByID(ctx, u.ID)
	if updated.MustChangePassword {
		t.Error("want must_change_password=false after change")
	}
}

func TestChangePassword_WrongCurrent_ReturnsInvalidCredentials(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	u, _ := svc.users.FindByUsername(ctx, bootstrapUsername)
	err := svc.ChangePassword(ctx, u.ID, "wrongcurrent", "newpass")
	if err != ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestChangePassword_EmptyNew_ReturnsWeakPassword(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	u, _ := svc.users.FindByUsername(ctx, bootstrapUsername)
	err := svc.ChangePassword(ctx, u.ID, bootstrapPassword, "")
	if err != ErrWeakPassword {
		t.Errorf("want ErrWeakPassword, got %v", err)
	}
}

func TestChangePassword_AdminLiteral_ReturnsWeakPassword(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_ = svc.Bootstrap(ctx)

	u, _ := svc.users.FindByUsername(ctx, bootstrapUsername)
	err := svc.ChangePassword(ctx, u.ID, bootstrapPassword, "admin")
	if err != ErrWeakPassword {
		t.Errorf("want ErrWeakPassword for 'admin' password, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// UserToPrincipal
// ---------------------------------------------------------------------------

func TestUserToPrincipal_SetsProviderAndFields(t *testing.T) {
	u := &User{
		ID:       "abc123",
		Username: "alice",
		Roles:    []string{"admin"},
	}
	p := UserToPrincipal(u)

	if p.Provider != ProviderLocalIAM {
		t.Errorf("want provider=%q, got %q", ProviderLocalIAM, p.Provider)
	}
	if p.ID != "localiam:abc123" {
		t.Errorf("want ID=localiam:abc123, got %q", p.ID)
	}
	if p.Name != "alice" {
		t.Errorf("want Name=alice, got %q", p.Name)
	}
	if len(p.Roles) != 1 || p.Roles[0] != identity.RolePlatformAdmin {
		t.Errorf("want roles=[platform.admin] (normalized from admin), got %v", p.Roles)
	}
}

func TestUserToPrincipal_RolesAreDefensiveCopy(t *testing.T) {
	u := &User{ID: "x", Roles: []string{"admin"}}
	p := UserToPrincipal(u)
	p.Roles[0] = "mutated"
	if u.Roles[0] == "mutated" {
		t.Error("UserToPrincipal must return a defensive copy of Roles")
	}
}

// ---------------------------------------------------------------------------
// SeedDemoUser
// ---------------------------------------------------------------------------

func TestSeedDemoUser_CreatesUser_WhenAbsent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Bootstrap first so there is an admin user (realistic startup order).
	if err := svc.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}

	if err := svc.SeedDemoUser(ctx); err != nil {
		t.Fatalf("SeedDemoUser: %v", err)
	}

	u, err := svc.users.FindByUsername(ctx, "demo")
	if err != nil || u == nil {
		t.Fatalf("want demo user, got nil (err=%v)", err)
	}
	if u.MustChangePassword {
		t.Error("want must_change_password=false for demo user")
	}
	if !u.Enabled {
		t.Error("want enabled=true for demo user")
	}
	if len(u.Roles) != 1 || u.Roles[0] != identity.RolePlatformOperator {
		t.Errorf("want role %q, got %v", identity.RolePlatformOperator, u.Roles)
	}
	if u.ID == "" {
		t.Error("want non-empty ID")
	}
}

func TestSeedDemoUser_NoOp_WhenUserExists(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if err := svc.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.SeedDemoUser(ctx); err != nil {
		t.Fatal(err)
	}

	// Second call must not error and must not duplicate the user.
	if err := svc.SeedDemoUser(ctx); err != nil {
		t.Fatalf("second SeedDemoUser should be no-op, got: %v", err)
	}

	n, _ := svc.users.Count(ctx)
	if n != 2 { // admin + demo
		t.Errorf("want exactly 2 users after 2 SeedDemoUser calls, got %d", n)
	}
}
