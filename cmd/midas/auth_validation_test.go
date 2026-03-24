package main

import (
	"testing"

	"github.com/accept-io/midas/internal/config"
)

// TestBuildAuthenticator_NilWhenNoTokens verifies that buildAuthenticator
// returns nil (no-op auth) when the token list is empty, regardless of mode.
func TestBuildAuthenticator_NilWhenNoTokens(t *testing.T) {
	a, err := buildAuthenticator(config.AuthConfig{
		Mode:   config.AuthModeOpen,
		Tokens: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != nil {
		t.Error("want nil authenticator for empty token list")
	}
}

// TestBuildAuthenticator_BuildsFromTokenEntries verifies that a non-empty token
// list produces a working StaticTokenAuthenticator.
func TestBuildAuthenticator_BuildsFromTokenEntries(t *testing.T) {
	a, err := buildAuthenticator(config.AuthConfig{
		Mode: config.AuthModeRequired,
		Tokens: []config.TokenEntry{
			{Token: "tok-admin", Principal: "user:admin", Roles: "admin"},
			{Token: "tok-op", Principal: "svc:worker", Roles: "operator"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("want non-nil authenticator for non-empty token list")
	}
}

// TestBuildAuthenticator_MultipleRoles verifies that comma-separated roles are
// split correctly.
func TestBuildAuthenticator_MultipleRoles(t *testing.T) {
	a, err := buildAuthenticator(config.AuthConfig{
		Mode: config.AuthModeRequired,
		Tokens: []config.TokenEntry{
			{Token: "tok", Principal: "user:alice", Roles: "admin,operator,reviewer"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("want non-nil authenticator")
	}
}

// TestBuildAuthenticator_NoRoles verifies that an entry with an empty Roles
// field is accepted (the principal just has no roles).
func TestBuildAuthenticator_NoRoles(t *testing.T) {
	a, err := buildAuthenticator(config.AuthConfig{
		Mode: config.AuthModeRequired,
		Tokens: []config.TokenEntry{
			{Token: "tok", Principal: "svc:monitor"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("want non-nil authenticator")
	}
}
