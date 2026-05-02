package oidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	goidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/accept-io/midas/internal/identity"
)

// Sentinel errors returned by Service.
var (
	ErrNoRolesMapped   = errors.New("oidc: no internal roles mapped for this user")
	ErrGroupNotAllowed = errors.New("oidc: user is not a member of any allowed group")
	ErrInvalidState    = errors.New("oidc: invalid or missing state parameter")
)

// Claims holds the extracted identity claims from an ID token.
type Claims struct {
	Subject  string
	Username string
	Groups   []string
	Raw      map[string]interface{}
}

// Service provides OIDC authorization code flow for platform/Explorer login.
// It handles the redirect, code exchange, token validation, and principal
// construction. Session creation is delegated to localiam (see httpapi).
type Service struct {
	cfg      Config
	provider *goidc.Provider
	oauth2   oauth2.Config
}

// NewService initialises an OIDCService by performing OIDC discovery against
// cfg.IssuerURL. Returns an error if discovery fails or the config is invalid.
// The provided context is used only during initialisation.
func NewService(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.IssuerURL == "" {
		return nil, fmt.Errorf("oidc: issuer_url is required")
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("oidc: client_id is required")
	}
	if cfg.RedirectURL == "" {
		return nil, fmt.Errorf("oidc: redirect_url is required")
	}

	provider, err := goidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc: provider discovery failed for %q: %w", cfg.IssuerURL, err)
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{goidc.ScopeOpenID, "profile", "email"}
	}

	oa := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	// Override endpoints if explicitly provided (useful for providers that
	// require tenant-specific or non-standard auth/token URLs, or when
	// operators prefer not to rely on OIDC discovery).
	if cfg.AuthURL != "" || cfg.TokenURL != "" {
		oa.Endpoint = oauth2.Endpoint{
			AuthURL:  cfg.AuthURL,
			TokenURL: cfg.TokenURL,
		}
	}

	slog.Info("oidc_provider_ready",
		"provider", cfg.ProviderName,
		"issuer", cfg.IssuerURL,
		"use_pkce", cfg.UsePKCE,
	)

	return &Service{cfg: cfg, provider: provider, oauth2: oa}, nil
}

// AuthURL returns the authorization URL to redirect the user to.
// state is the CSRF token. pkceChallenge is included when cfg.UsePKCE is true
// (pass an empty string to skip PKCE regardless of config).
func (s *Service) AuthURL(state, pkceChallenge string) string {
	opts := []oauth2.AuthCodeOption{}
	if s.cfg.DomainHint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("domain_hint", s.cfg.DomainHint))
	}
	if pkceChallenge != "" {
		opts = append(opts,
			oauth2.SetAuthURLParam("code_challenge", pkceChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)
	}
	return s.oauth2.AuthCodeURL(state, opts...)
}

// Exchange exchanges the authorization code for tokens, validates the ID token,
// extracts claims, and returns a populated Claims value.
// pkceVerifier is included in the exchange when non-empty.
func (s *Service) Exchange(ctx context.Context, code, pkceVerifier string) (*Claims, error) {
	opts := []oauth2.AuthCodeOption{}
	if pkceVerifier != "" {
		opts = append(opts, oauth2.SetAuthURLParam("code_verifier", pkceVerifier))
	}

	token, err := s.oauth2.Exchange(ctx, code, opts...)
	if err != nil {
		return nil, fmt.Errorf("oidc: code exchange failed: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("oidc: no id_token in response")
	}

	verifier := s.provider.Verifier(&goidc.Config{ClientID: s.cfg.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: id_token verification failed: %w", err)
	}

	var raw map[string]interface{}
	if err := idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("oidc: claims extraction failed: %w", err)
	}

	claims := &Claims{
		Subject: idToken.Subject,
		Raw:     raw,
	}

	// Extract username from the configured claim.
	if uc := s.cfg.UsernameClaim; uc != "" {
		if v, ok := raw[uc].(string); ok {
			claims.Username = v
		}
	}
	if claims.Username == "" {
		claims.Username = idToken.Subject // fallback
	}

	// Extract groups from the configured claim. Treat missing claim as empty.
	if gc := s.cfg.GroupsClaim; gc != "" {
		claims.Groups = extractStringSlice(raw, gc)
	}

	return claims, nil
}

// BuildPrincipal converts OIDC claims to a MIDAS *identity.Principal.
// It enforces AllowedGroups and DenyIfNoRoles per configuration.
// Returns ErrGroupNotAllowed or ErrNoRolesMapped on policy denial.
func (s *Service) BuildPrincipal(claims *Claims) (*identity.Principal, error) {
	// AllowedGroups check: user must belong to at least one listed group.
	if len(s.cfg.AllowedGroups) > 0 {
		allowed := false
		allowedSet := make(map[string]struct{}, len(s.cfg.AllowedGroups))
		for _, g := range s.cfg.AllowedGroups {
			allowedSet[g] = struct{}{}
		}
		for _, g := range claims.Groups {
			if _, ok := allowedSet[g]; ok {
				allowed = true
				break
			}
		}
		if !allowed {
			slog.Warn("oidc_group_not_allowed",
				"subject", claims.Subject,
				"username", claims.Username,
			)
			return nil, ErrGroupNotAllowed
		}
	}

	roles := MapExternalRoles(claims.Groups, s.cfg.RoleMappings)

	if s.cfg.DenyIfNoRoles && len(roles) == 0 {
		slog.Warn("oidc_no_roles_mapped",
			"subject", claims.Subject,
			"username", claims.Username,
			"groups", claims.Groups,
		)
		return nil, ErrNoRolesMapped
	}

	p := &identity.Principal{
		ID:       "oidc:" + claims.Subject,
		Subject:  claims.Subject,
		Name:     claims.Username,
		Roles:    roles,
		Provider: Provider,
	}

	slog.Info("oidc_principal_built",
		"subject", claims.Subject,
		"username", claims.Username,
		"roles", roles,
	)

	return p, nil
}

// UsePKCE reports whether PKCE is configured.
func (s *Service) UsePKCE() bool { return s.cfg.UsePKCE }

// SecureCookies is a convenience that returns whether the TLS flag should be
// set on OIDC helper cookies; it mirrors the localiam secure-cookies setting
// and is passed through from the calling HTTP handler.

// extractStringSlice extracts a []string from a raw claims map.
// Handles both []interface{} (JSON arrays) and plain string values.
func extractStringSlice(raw map[string]interface{}, key string) []string {
	v, ok := raw[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if val != "" {
			return []string{val}
		}
	}
	return nil
}

// newServiceWithHTTPClient is a test helper that overrides the HTTP client
// used during provider discovery. Not exported.
func newServiceWithHTTPClient(ctx context.Context, cfg Config, client *http.Client) (*Service, error) {
	ctx = goidc.ClientContext(ctx, client)
	return NewService(ctx, cfg)
}
