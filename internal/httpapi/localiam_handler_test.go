package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/localiam"
)

// ---------------------------------------------------------------------------
// In-process stub repos (minimal, no mutex needed in serial tests)
// ---------------------------------------------------------------------------

type stubUserRepo struct {
	byID       map[string]*localiam.User
	byUsername map[string]*localiam.User
}

func newStubUserRepo() *stubUserRepo {
	return &stubUserRepo{
		byID:       make(map[string]*localiam.User),
		byUsername: make(map[string]*localiam.User),
	}
}

func (r *stubUserRepo) FindByID(_ context.Context, id string) (*localiam.User, error) {
	u := r.byID[id]
	if u == nil {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}
func (r *stubUserRepo) FindByUsername(_ context.Context, username string) (*localiam.User, error) {
	u := r.byUsername[username]
	if u == nil {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}
func (r *stubUserRepo) Create(_ context.Context, u *localiam.User) error {
	cp := *u
	r.byID[u.ID] = &cp
	r.byUsername[u.Username] = &cp
	return nil
}
func (r *stubUserRepo) Update(_ context.Context, u *localiam.User) error {
	cp := *u
	r.byID[u.ID] = &cp
	r.byUsername[u.Username] = &cp
	return nil
}
func (r *stubUserRepo) Count(_ context.Context) (int, error) { return len(r.byID), nil }

type stubSessionRepo struct{ items map[string]*localiam.Session }

func newStubSessionRepo() *stubSessionRepo {
	return &stubSessionRepo{items: make(map[string]*localiam.Session)}
}
func (r *stubSessionRepo) Create(_ context.Context, s *localiam.Session) error {
	cp := *s
	r.items[s.ID] = &cp
	return nil
}
func (r *stubSessionRepo) FindByID(_ context.Context, id string) (*localiam.Session, error) {
	s := r.items[id]
	if s == nil {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}
func (r *stubSessionRepo) Delete(_ context.Context, id string) error {
	delete(r.items, id)
	return nil
}
func (r *stubSessionRepo) DeleteExpired(_ context.Context) error { return nil }

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newIAMServer(t *testing.T) (*Server, *localiam.Service) {
	t.Helper()
	users := newStubUserRepo()
	sessions := newStubSessionRepo()
	svc := localiam.NewService(users, sessions, localiam.Config{
		SessionTTL: time.Hour,
	})
	if err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	srv := newTestServer().WithLocalIAM(svc)
	return srv, svc
}

func doLogin(t *testing.T, srv *Server, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := marshalJSON(t, map[string]string{"username": username, "password": password})
	return performRequest(t, srv, http.MethodPost, "/auth/login", body)
}

func sessionCookie(rec *httptest.ResponseRecorder) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == localiam.SessionCookieName {
			return c.Value
		}
	}
	return ""
}

func requestWithCookie(t *testing.T, srv *Server, method, path string, body []byte, cookieVal string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if len(body) > 0 {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if cookieVal != "" {
		req.AddCookie(&http.Cookie{Name: localiam.SessionCookieName, Value: cookieVal})
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

func TestHandlerLogin_ValidCredentials_Returns200WithCookie(t *testing.T) {
	srv, _ := newIAMServer(t)

	rec := doLogin(t, srv, "admin", "admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if sessionCookie(rec) == "" {
		t.Error("want session cookie set on login")
	}
	body := decodeJSON[map[string]any](t, rec)
	if body["must_change_password"] != true {
		t.Errorf("want must_change_password=true, got %v", body["must_change_password"])
	}
}

func TestHandlerLogin_BadPassword_Returns401(t *testing.T) {
	srv, _ := newIAMServer(t)

	rec := doLogin(t, srv, "admin", "wrongpassword")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestHandlerLogin_UnknownUser_Returns401(t *testing.T) {
	srv, _ := newIAMServer(t)

	rec := doLogin(t, srv, "nobody", "admin")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestHandlerLogin_MissingFields_Returns400(t *testing.T) {
	srv, _ := newIAMServer(t)

	body := marshalJSON(t, map[string]string{"username": "admin"}) // no password
	rec := performRequest(t, srv, http.MethodPost, "/auth/login", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Logout tests
// ---------------------------------------------------------------------------

func TestHandlerLogout_ValidSession_ClearsSessionAndCookie(t *testing.T) {
	srv, _ := newIAMServer(t)

	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)

	rec := requestWithCookie(t, srv, http.MethodPost, "/auth/logout", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	// The session should now be invalid.
	meRec := requestWithCookie(t, srv, http.MethodGet, "/auth/me", nil, cookie)
	if meRec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 after logout, got %d", meRec.Code)
	}
}

func TestHandlerLogout_NoSession_Returns200(t *testing.T) {
	srv, _ := newIAMServer(t)

	rec := requestWithCookie(t, srv, http.MethodPost, "/auth/logout", nil, "")
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 even without session, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// /auth/me tests
// ---------------------------------------------------------------------------

func TestHandlerMe_WithSession_ReturnsPrincipal(t *testing.T) {
	srv, _ := newIAMServer(t)

	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)

	rec := requestWithCookie(t, srv, http.MethodGet, "/auth/me", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := decodeJSON[map[string]any](t, rec)
	if body["username"] != "admin" {
		t.Errorf("want username=admin, got %v", body["username"])
	}
	if body["must_change_password"] != true {
		t.Errorf("want must_change_password=true, got %v", body["must_change_password"])
	}
}

func TestHandlerMe_NoSession_Returns401(t *testing.T) {
	srv, _ := newIAMServer(t)

	rec := requestWithCookie(t, srv, http.MethodGet, "/auth/me", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Change-password tests
// ---------------------------------------------------------------------------

func TestHandlerChangePassword_Success_ClearsMustChange(t *testing.T) {
	srv, _ := newIAMServer(t)

	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)

	body := marshalJSON(t, map[string]string{
		"current_password": "admin",
		"new_password":     "NewSecure99!",
	})
	rec := requestWithCookie(t, srv, http.MethodPost, "/auth/change-password", body, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// must_change_password should now be false.
	meRec := requestWithCookie(t, srv, http.MethodGet, "/auth/me", nil, cookie)
	me := decodeJSON[map[string]any](t, meRec)
	if me["must_change_password"] != false {
		t.Errorf("want must_change_password=false after change, got %v", me["must_change_password"])
	}
}

func TestHandlerChangePassword_WrongCurrent_Returns401(t *testing.T) {
	srv, _ := newIAMServer(t)

	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)

	body := marshalJSON(t, map[string]string{
		"current_password": "wrong",
		"new_password":     "NewSecure99!",
	})
	rec := requestWithCookie(t, srv, http.MethodPost, "/auth/change-password", body, cookie)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestHandlerChangePassword_WeakNew_Returns400(t *testing.T) {
	srv, _ := newIAMServer(t)

	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)

	body := marshalJSON(t, map[string]string{
		"current_password": "admin",
		"new_password":     "admin", // rejected
	})
	rec := requestWithCookie(t, srv, http.MethodPost, "/auth/change-password", body, cookie)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestHandlerChangePassword_NoSession_Returns401(t *testing.T) {
	srv, _ := newIAMServer(t)

	body := marshalJSON(t, map[string]string{
		"current_password": "admin",
		"new_password":     "NewSecure99!",
	})
	rec := requestWithCookie(t, srv, http.MethodPost, "/auth/change-password", body, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Forced password change enforcement
// ---------------------------------------------------------------------------

func TestForcedPasswordChange_BlocksExplorerRoutes(t *testing.T) {
	srv, _ := newIAMServer(t)
	// Enable explorer so routes are registered.
	srv.WithExplorerEnabled(true)

	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)

	// POST /explorer requires auth + role; must_change_password should block it.
	req := httptest.NewRequest(http.MethodPost, "/explorer", nil)
	req.AddCookie(&http.Cookie{Name: localiam.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403 when must_change_password=true, got %d", rec.Code)
	}
}

func TestForcedPasswordChange_AllowsMeAndChangePassword(t *testing.T) {
	srv, _ := newIAMServer(t)

	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)

	// /auth/me must be accessible even with must_change_password=true.
	rec := requestWithCookie(t, srv, http.MethodGet, "/auth/me", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for /auth/me with must_change_password=true, got %d", rec.Code)
	}

	// /auth/change-password must be accessible.
	body := marshalJSON(t, map[string]string{
		"current_password": "admin",
		"new_password":     "NewSecure99!",
	})
	rec = requestWithCookie(t, srv, http.MethodPost, "/auth/change-password", body, cookie)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for /auth/change-password with must_change_password=true, got %d", rec.Code)
	}
}

func TestExplorerRoutes_AfterPasswordChange_AllowsAccess(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv.WithExplorerEnabled(true)

	// Login and change password.
	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)

	body := marshalJSON(t, map[string]string{
		"current_password": "admin",
		"new_password":     "NewSecure99!",
	})
	requestWithCookie(t, srv, http.MethodPost, "/auth/change-password", body, cookie)

	// POST /explorer should now reach the handler (not 403 or 401).
	req := httptest.NewRequest(http.MethodPost, "/explorer", nil)
	req.AddCookie(&http.Cookie{Name: localiam.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// The explorer handler may return 422/500 due to missing body — but not 401/403.
	if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Errorf("want request to reach handler after password change, got %d", rec.Code)
	}
}
