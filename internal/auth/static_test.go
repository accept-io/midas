package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/accept-io/midas/internal/identity"
)

func TestStaticTokenAuthenticator_ValidToken(t *testing.T) {
	tokens := map[string]*identity.Principal{
		"tok-alice": {ID: "user:alice", Provider: identity.ProviderStatic, Roles: []string{identity.RoleAdmin}},
	}
	a := NewStaticTokenAuthenticator(tokens)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer tok-alice")

	p, err := a.Authenticate(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "user:alice" {
		t.Errorf("want ID user:alice, got %q", p.ID)
	}
	if p.Provider != identity.ProviderStatic {
		t.Errorf("want provider static, got %q", p.Provider)
	}
}

func TestStaticTokenAuthenticator_UnknownToken(t *testing.T) {
	a := NewStaticTokenAuthenticator(map[string]*identity.Principal{})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer unknown-tok")

	_, err := a.Authenticate(r)
	if err == nil {
		t.Fatal("expected error for unknown token")
	}
}

func TestStaticTokenAuthenticator_NoHeader(t *testing.T) {
	a := NewStaticTokenAuthenticator(map[string]*identity.Principal{})

	r := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := a.Authenticate(r)
	if err == nil {
		t.Fatal("expected error for missing header")
	}
	if err != ErrNoCredentials {
		t.Errorf("want ErrNoCredentials, got %v", err)
	}
}

func TestStaticTokenAuthenticator_NonBearerScheme(t *testing.T) {
	a := NewStaticTokenAuthenticator(map[string]*identity.Principal{})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	_, err := a.Authenticate(r)
	if err != ErrNoCredentials {
		t.Errorf("want ErrNoCredentials for non-Bearer scheme, got %v", err)
	}
}

func TestParseBearerToken(t *testing.T) {
	tests := []struct {
		header  string
		want    string
		wantOK  bool
	}{
		{"Bearer tok123", "tok123", true},
		{"Bearer  tok123  ", "tok123", true},
		{"", "", false},
		{"Basic dXNlcg==", "", false},
		{"Bearer ", "", false},
		{"bearer tok123", "", false}, // case-sensitive
	}

	for _, tc := range tests {
		got, ok := parseBearerToken(tc.header)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("parseBearerToken(%q) = (%q, %v), want (%q, %v)",
				tc.header, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestLoadStaticTokensFromEnv_Empty(t *testing.T) {
	t.Setenv("MIDAS_AUTH_TOKENS", "")
	a, err := LoadStaticTokensFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != nil {
		t.Error("want nil authenticator when env is empty")
	}
}

func TestLoadStaticTokensFromEnv_Valid(t *testing.T) {
	t.Setenv("MIDAS_AUTH_TOKENS", "tok-a|user:alice|admin,approver;tok-b|svc:deploy|operator")

	a, err := LoadStaticTokensFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("want non-nil authenticator")
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer tok-a")
	p, err := a.Authenticate(r)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.ID != "user:alice" {
		t.Errorf("want ID user:alice, got %q", p.ID)
	}
	if !p.HasRole(identity.RoleAdmin) {
		t.Error("want admin role")
	}
	if !p.HasRole(identity.RoleApprover) {
		t.Error("want approver role")
	}
}

func TestLoadStaticTokensFromEnv_Malformed(t *testing.T) {
	t.Setenv("MIDAS_AUTH_TOKENS", "notokenvalue-no-pipe-separator")

	_, err := LoadStaticTokensFromEnv()
	if err == nil {
		t.Error("want error for malformed entry")
	}
}
