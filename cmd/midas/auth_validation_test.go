package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/identity"
)

// stubAuthenticator is a minimal auth.Authenticator for testing.
type stubAuthenticator struct{}

func (s *stubAuthenticator) Authenticate(_ *http.Request) (*identity.Principal, error) {
	return nil, nil
}

func TestValidateAuthConfig(t *testing.T) {
	tests := []struct {
		name        string
		backend     string
		auth        auth.Authenticator
		envDisabled string
		wantErr     bool
	}{
		{
			name:    "postgres with auth configured — OK",
			backend: "postgres",
			auth:    &stubAuthenticator{},
			wantErr: false,
		},
		{
			name:        "postgres without auth, MIDAS_AUTH_DISABLED=true — OK",
			backend:     "postgres",
			auth:        nil,
			envDisabled: "true",
			wantErr:     false,
		},
		{
			name:    "postgres without auth, no opt-out — error",
			backend: "postgres",
			auth:    nil,
			wantErr: true,
		},
		{
			name:        "postgres without auth, MIDAS_AUTH_DISABLED=false — error",
			backend:     "postgres",
			auth:        nil,
			envDisabled: "false",
			wantErr:     true,
		},
		{
			name:    "memory without auth — OK (no enforcement)",
			backend: "memory",
			auth:    nil,
			wantErr: false,
		},
		{
			name:    "memory with auth — OK",
			backend: "memory",
			auth:    &stubAuthenticator{},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MIDAS_AUTH_DISABLED", tc.envDisabled)

			err := validateAuthConfig(tc.backend, tc.auth)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateAuthConfig(%q, auth=%v): got err=%v, wantErr=%v",
					tc.backend, tc.auth != nil, err, tc.wantErr)
			}
			if tc.wantErr && err != nil {
				msg := err.Error()
				if !strings.Contains(msg, "Postgres mode requires authentication.") {
					t.Errorf("error missing expected guidance: %s", msg)
				}
				if !strings.Contains(msg, "MIDAS_AUTH_DISABLED=true") {
					t.Errorf("error missing MIDAS_AUTH_DISABLED hint: %s", msg)
				}
			}
		})
	}
}
