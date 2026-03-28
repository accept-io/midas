package oidc

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/accept-io/midas/internal/identity"
)

// --- BuildPrincipal tests ---

func newTestService(cfg Config) *Service {
	// Construct a Service directly without provider discovery for unit tests.
	return &Service{cfg: cfg}
}

func TestBuildPrincipal_Success(t *testing.T) {
	svc := newTestService(Config{
		RoleMappings: []RoleMapping{
			{External: "midas-admins", Internal: identity.RolePlatformAdmin},
		},
		DenyIfNoRoles: true,
	})
	claims := &Claims{
		Subject:  "user-123",
		Username: "alice",
		Groups:   []string{"midas-admins"},
	}
	p, err := svc.BuildPrincipal(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "oidc:user-123" {
		t.Errorf("ID = %q, want %q", p.ID, "oidc:user-123")
	}
	if p.Subject != "user-123" {
		t.Errorf("Subject = %q, want %q", p.Subject, "user-123")
	}
	if p.Name != "alice" {
		t.Errorf("Name = %q, want %q", p.Name, "alice")
	}
	if p.Provider != Provider {
		t.Errorf("Provider = %q, want %q", p.Provider, Provider)
	}
	if len(p.Roles) != 1 || p.Roles[0] != identity.RolePlatformAdmin {
		t.Errorf("Roles = %v, want [%s]", p.Roles, identity.RolePlatformAdmin)
	}
}

func TestBuildPrincipal_DenyIfNoRoles(t *testing.T) {
	svc := newTestService(Config{
		RoleMappings:  []RoleMapping{},
		DenyIfNoRoles: true,
	})
	claims := &Claims{
		Subject:  "user-456",
		Username: "bob",
		Groups:   []string{"some-unmapped-group"},
	}
	_, err := svc.BuildPrincipal(claims)
	if !errors.Is(err, ErrNoRolesMapped) {
		t.Errorf("expected ErrNoRolesMapped, got %v", err)
	}
}

func TestBuildPrincipal_AllowNoRolesWhenFlagFalse(t *testing.T) {
	svc := newTestService(Config{
		RoleMappings:  []RoleMapping{},
		DenyIfNoRoles: false,
	})
	claims := &Claims{
		Subject:  "user-789",
		Username: "charlie",
		Groups:   []string{"some-unmapped-group"},
	}
	p, err := svc.BuildPrincipal(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Roles) != 0 {
		t.Errorf("expected empty roles, got %v", p.Roles)
	}
}

func TestBuildPrincipal_AllowedGroups_Denied(t *testing.T) {
	svc := newTestService(Config{
		AllowedGroups: []string{"approved-group"},
		RoleMappings: []RoleMapping{
			{External: "other-group", Internal: identity.RolePlatformAdmin},
		},
		DenyIfNoRoles: false,
	})
	claims := &Claims{
		Subject: "user-111",
		Groups:  []string{"other-group"},
	}
	_, err := svc.BuildPrincipal(claims)
	if !errors.Is(err, ErrGroupNotAllowed) {
		t.Errorf("expected ErrGroupNotAllowed, got %v", err)
	}
}

func TestBuildPrincipal_AllowedGroups_Allowed(t *testing.T) {
	svc := newTestService(Config{
		AllowedGroups: []string{"approved-group"},
		RoleMappings: []RoleMapping{
			{External: "approved-group", Internal: identity.RolePlatformOperator},
		},
		DenyIfNoRoles: true,
	})
	claims := &Claims{
		Subject: "user-222",
		Groups:  []string{"approved-group"},
	}
	p, err := svc.BuildPrincipal(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Roles) != 1 || p.Roles[0] != identity.RolePlatformOperator {
		t.Errorf("Roles = %v, want [%s]", p.Roles, identity.RolePlatformOperator)
	}
}

func TestBuildPrincipal_AllowedGroups_Empty_AllowsAll(t *testing.T) {
	// Empty AllowedGroups list means "no group restriction".
	svc := newTestService(Config{
		AllowedGroups: nil,
		RoleMappings: []RoleMapping{
			{External: "any-group", Internal: identity.RolePlatformViewer},
		},
		DenyIfNoRoles: true,
	})
	claims := &Claims{
		Subject: "user-333",
		Groups:  []string{"any-group"},
	}
	p, err := svc.BuildPrincipal(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Roles) == 0 {
		t.Error("expected roles, got empty")
	}
}

// --- GenerateState / ConsumeStateCookie tests ---

func TestGenerateState_IsNonEmpty(t *testing.T) {
	s, err := GenerateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) == 0 {
		t.Error("expected non-empty state")
	}
}

func TestGenerateState_IsUnique(t *testing.T) {
	a, _ := GenerateState()
	b, _ := GenerateState()
	if a == b {
		t.Error("two calls returned identical state — not random")
	}
}

func TestSetAndConsumeStateCookie(t *testing.T) {
	w := httptest.NewRecorder()
	SetStateCookie(w, "test-state-value", false)

	// Build a request carrying the cookie set above.
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}

	w2 := httptest.NewRecorder()
	got, ok := ConsumeStateCookie(w2, req, false)
	if !ok {
		t.Fatal("ConsumeStateCookie returned false")
	}
	if got != "test-state-value" {
		t.Errorf("got %q, want %q", got, "test-state-value")
	}
	// Cookie should be cleared on response.
	cleared := false
	for _, c := range w2.Result().Cookies() {
		if c.Name == stateCookieName && c.MaxAge == -1 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("state cookie was not cleared after consumption")
	}
}

func TestConsumeStateCookie_MissingCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback", nil)
	w := httptest.NewRecorder()
	_, ok := ConsumeStateCookie(w, req, false)
	if ok {
		t.Error("expected false for missing cookie")
	}
}

// --- GeneratePKCE / ConsumePKCECookie tests ---

func TestGeneratePKCE_VerifierAndChallengeFormat(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verifier) < 43 {
		t.Errorf("verifier too short: %d chars (RFC 7636 min is 43)", len(verifier))
	}
	// Recompute challenge from verifier and check equality.
	sum := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != wantChallenge {
		t.Errorf("challenge mismatch: got %q, want %q", challenge, wantChallenge)
	}
}

func TestGeneratePKCE_IsUnique(t *testing.T) {
	v1, _, _ := GeneratePKCE()
	v2, _, _ := GeneratePKCE()
	if v1 == v2 {
		t.Error("two PKCE calls returned identical verifier")
	}
}

func TestSetAndConsumePKCECookie(t *testing.T) {
	w := httptest.NewRecorder()
	SetPKCECookie(w, "test-pkce-verifier", false)

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}

	w2 := httptest.NewRecorder()
	got, ok := ConsumePKCECookie(w2, req, false)
	if !ok {
		t.Fatal("ConsumePKCECookie returned false")
	}
	if got != "test-pkce-verifier" {
		t.Errorf("got %q, want %q", got, "test-pkce-verifier")
	}
	cleared := false
	for _, c := range w2.Result().Cookies() {
		if c.Name == pkceCookieName && c.MaxAge == -1 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("pkce cookie was not cleared after consumption")
	}
}

// --- extractStringSlice tests ---

func TestExtractStringSlice_ArrayValue(t *testing.T) {
	raw := map[string]interface{}{
		"groups": []interface{}{"g1", "g2", "g3"},
	}
	got := extractStringSlice(raw, "groups")
	if len(got) != 3 || got[0] != "g1" || got[2] != "g3" {
		t.Errorf("got %v", got)
	}
}

func TestExtractStringSlice_StringValue(t *testing.T) {
	raw := map[string]interface{}{
		"group": "single-group",
	}
	got := extractStringSlice(raw, "group")
	if len(got) != 1 || got[0] != "single-group" {
		t.Errorf("got %v", got)
	}
}

func TestExtractStringSlice_MissingKey(t *testing.T) {
	raw := map[string]interface{}{}
	got := extractStringSlice(raw, "groups")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestExtractStringSlice_EmptyStringsFiltered(t *testing.T) {
	raw := map[string]interface{}{
		"groups": []interface{}{"g1", "", "g2"},
	}
	got := extractStringSlice(raw, "groups")
	if len(got) != 2 {
		t.Errorf("expected 2 elements, got %v", got)
	}
}

// --- UsePKCE ---

func TestUsePKCE(t *testing.T) {
	svcOn := newTestService(Config{UsePKCE: true})
	if !svcOn.UsePKCE() {
		t.Error("expected UsePKCE=true")
	}
	svcOff := newTestService(Config{UsePKCE: false})
	if svcOff.UsePKCE() {
		t.Error("expected UsePKCE=false")
	}
}
