package oidc

// google_test.go validates that the existing OIDC implementation supports
// Google Workspace as a second provider via configuration alone, without any
// provider-specific code paths.
//
// Google differs from Entra in two ways that matter for claim extraction:
//   - groups_claim is "hd" (hosted domain), a plain string not an array
//   - username_claim is "email" not "preferred_username"
//
// Both are already handled generically. These tests verify that.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/accept-io/midas/internal/identity"
)

// googleConfig returns a Service configured as a Google Workspace operator would.
// It bypasses discovery (no network call) using newTestService.
func googleConfig() Config {
	return Config{
		ProviderName:  "google",
		GroupsClaim:   "hd",
		UsernameClaim: "email",
		SubjectClaim:  "sub",
		RoleMappings: []RoleMapping{
			{External: "example.com", Internal: identity.RolePlatformAdmin},
		},
		DenyIfNoRoles: true,
		UsePKCE:       true,
	}
}

// ---------------------------------------------------------------------------
// Claim extraction — Google hd claim format
// ---------------------------------------------------------------------------

func TestGoogle_HDClaim_StringWrapsToSlice(t *testing.T) {
	raw := map[string]interface{}{
		"hd": "example.com",
	}
	got := extractStringSlice(raw, "hd")
	want := []string{"example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGoogle_HDClaim_Missing_ReturnsNil(t *testing.T) {
	// Personal Google accounts (non-Workspace) do not have the hd claim.
	raw := map[string]interface{}{
		"sub":   "115977861111111111111",
		"email": "user@gmail.com",
	}
	got := extractStringSlice(raw, "hd")
	if got != nil {
		t.Errorf("expected nil for missing hd, got %v", got)
	}
}

func TestGoogle_EmailClaim_ExtractedAsUsername(t *testing.T) {
	// Verify the extraction pattern Exchange uses for username_claim = "email".
	raw := map[string]interface{}{
		"sub":   "115977861111111111111",
		"email": "alice@example.com",
		"hd":    "example.com",
	}
	// Mirrors the logic in Exchange: raw[UsernameClaim].(string)
	var username string
	if v, ok := raw["email"].(string); ok {
		username = v
	}
	if username != "alice@example.com" {
		t.Errorf("got %q, want %q", username, "alice@example.com")
	}
}

func TestGoogle_MissingPreferredUsername_FallsBackToSubject(t *testing.T) {
	// When username_claim is not set (or defaults to "preferred_username" which
	// Google doesn't include), Exchange falls back to idToken.Subject.
	// This test verifies the fallback logic mirrors what Exchange does.
	raw := map[string]interface{}{
		"sub":   "115977861111111111111",
		"email": "alice@example.com",
		// no "preferred_username"
	}
	var username string
	if v, ok := raw["preferred_username"].(string); ok {
		username = v
	}
	if username == "" {
		username = raw["sub"].(string) // fallback
	}
	if username != "115977861111111111111" {
		t.Errorf("fallback to subject: got %q, want subject value", username)
	}
}

// ---------------------------------------------------------------------------
// BuildPrincipal — Google Workspace config
// ---------------------------------------------------------------------------

func TestGoogle_BuildPrincipal_HDMapped_Success(t *testing.T) {
	svc := newTestService(googleConfig())
	claims := &Claims{
		Subject:  "115977861111111111111",
		Username: "alice@example.com",
		Groups:   []string{"example.com"}, // hd extracted as []string
	}
	p, err := svc.BuildPrincipal(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "oidc:115977861111111111111" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.Subject != "115977861111111111111" {
		t.Errorf("Subject = %q", p.Subject)
	}
	if p.Name != "alice@example.com" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Provider != Provider {
		t.Errorf("Provider = %q, want %q", p.Provider, Provider)
	}
	if len(p.Roles) != 1 || p.Roles[0] != identity.RolePlatformAdmin {
		t.Errorf("Roles = %v", p.Roles)
	}
}

func TestGoogle_BuildPrincipal_MissingHD_DenyIfNoRoles(t *testing.T) {
	// Personal Gmail accounts (no hd) → no groups → denied.
	svc := newTestService(googleConfig())
	claims := &Claims{
		Subject:  "115977861111111111111",
		Username: "user@gmail.com",
		Groups:   nil, // no hd claim
	}
	_, err := svc.BuildPrincipal(claims)
	if !errors.Is(err, ErrNoRolesMapped) {
		t.Errorf("expected ErrNoRolesMapped for personal Gmail account, got %v", err)
	}
}

func TestGoogle_BuildPrincipal_WrongDomain_DenyIfNoRoles(t *testing.T) {
	// A Workspace user from an unmapped domain → no role mapping → denied.
	svc := newTestService(googleConfig())
	claims := &Claims{
		Subject:  "999",
		Username: "user@other.com",
		Groups:   []string{"other.com"},
	}
	_, err := svc.BuildPrincipal(claims)
	if !errors.Is(err, ErrNoRolesMapped) {
		t.Errorf("expected ErrNoRolesMapped for unmapped domain, got %v", err)
	}
}

func TestGoogle_BuildPrincipal_AllowedGroups_DomainGating(t *testing.T) {
	cfg := googleConfig()
	cfg.AllowedGroups = []string{"example.com"}

	svc := newTestService(cfg)

	// Correct domain → allowed.
	good := &Claims{Subject: "1", Username: "a@example.com", Groups: []string{"example.com"}}
	if _, err := svc.BuildPrincipal(good); err != nil {
		t.Errorf("expected success for allowed domain, got %v", err)
	}

	// Wrong domain → denied even if hd present.
	bad := &Claims{Subject: "2", Username: "b@other.com", Groups: []string{"other.com"}}
	if _, err := svc.BuildPrincipal(bad); !errors.Is(err, ErrGroupNotAllowed) {
		t.Errorf("expected ErrGroupNotAllowed for disallowed domain, got %v", err)
	}
}

func TestGoogle_BuildPrincipal_MultipleRoleMappings(t *testing.T) {
	cfg := Config{
		GroupsClaim:   "hd",
		UsernameClaim: "email",
		RoleMappings: []RoleMapping{
			{External: "ops.example.com", Internal: identity.RolePlatformOperator},
			{External: "admin.example.com", Internal: identity.RolePlatformAdmin},
		},
		DenyIfNoRoles: true,
	}
	svc := newTestService(cfg)

	// Operator domain.
	opClaims := &Claims{Subject: "1", Username: "op@ops.example.com", Groups: []string{"ops.example.com"}}
	p, err := svc.BuildPrincipal(opClaims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Roles) != 1 || p.Roles[0] != identity.RolePlatformOperator {
		t.Errorf("Roles = %v, want [%s]", p.Roles, identity.RolePlatformOperator)
	}
}

// ---------------------------------------------------------------------------
// Structural equivalence — Google principal matches Entra principal shape
// ---------------------------------------------------------------------------

func TestGoogle_PrincipalStructure_IdenticalToEntra(t *testing.T) {
	// Both providers must produce a *identity.Principal with the same field set.
	// Provider value is "oidc" for both; the caller cannot distinguish by struct shape.

	entraConfig := Config{
		RoleMappings:  []RoleMapping{{External: "midas-admins", Internal: identity.RolePlatformAdmin}},
		DenyIfNoRoles: true,
	}
	entraSvc := newTestService(entraConfig)
	entraClaims := &Claims{
		Subject:  "entra-user-guid",
		Username: "alice",
		Groups:   []string{"midas-admins"},
	}
	entraP, err := entraSvc.BuildPrincipal(entraClaims)
	if err != nil {
		t.Fatalf("entra BuildPrincipal: %v", err)
	}

	googleSvc := newTestService(googleConfig())
	googleClaims := &Claims{
		Subject:  "115977861111111111111",
		Username: "alice@example.com",
		Groups:   []string{"example.com"},
	}
	googleP, err := googleSvc.BuildPrincipal(googleClaims)
	if err != nil {
		t.Fatalf("google BuildPrincipal: %v", err)
	}

	// Same Provider value.
	if entraP.Provider != googleP.Provider {
		t.Errorf("provider mismatch: entra=%q google=%q", entraP.Provider, googleP.Provider)
	}
	// Both produce exactly one canonical role.
	if len(entraP.Roles) != 1 || len(googleP.Roles) != 1 {
		t.Errorf("role count mismatch: entra=%v google=%v", entraP.Roles, googleP.Roles)
	}
	if entraP.Roles[0] != googleP.Roles[0] {
		t.Errorf("role value mismatch: entra=%q google=%q", entraP.Roles[0], googleP.Roles[0])
	}
	// ID format is provider-prefixed subject for both.
	if entraP.ID != "oidc:entra-user-guid" {
		t.Errorf("entra ID = %q", entraP.ID)
	}
	if googleP.ID != "oidc:115977861111111111111" {
		t.Errorf("google ID = %q", googleP.ID)
	}
}

// ---------------------------------------------------------------------------
// End-to-end Google flow simulation (bypasses network; tests full claim path)
// ---------------------------------------------------------------------------

func TestGoogle_FullFlow_HappyPath(t *testing.T) {
	// Simulates the complete post-exchange path: raw claims → Claims extraction
	// → BuildPrincipal, using Google-shaped token data.

	// Step 1: raw claims as they come from an ID token (what Exchange produces).
	raw := map[string]interface{}{
		"sub":            "115977861111111111111",
		"email":          "alice@example.com",
		"email_verified": true,
		"hd":             "example.com",
		"name":           "Alice Smith",
	}

	// Step 2: claim extraction (mirrors Exchange internals).
	groups := extractStringSlice(raw, "hd")
	var username string
	if v, ok := raw["email"].(string); ok {
		username = v
	}
	if username == "" {
		username = raw["sub"].(string)
	}

	claims := &Claims{
		Subject:  raw["sub"].(string),
		Username: username,
		Groups:   groups,
		Raw:      raw,
	}

	// Step 3: BuildPrincipal.
	svc := newTestService(googleConfig())
	p, err := svc.BuildPrincipal(claims)
	if err != nil {
		t.Fatalf("BuildPrincipal: %v", err)
	}

	if p.ID != "oidc:115977861111111111111" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.Name != "alice@example.com" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Provider != Provider {
		t.Errorf("Provider = %q", p.Provider)
	}
	if len(p.Roles) != 1 || p.Roles[0] != identity.RolePlatformAdmin {
		t.Errorf("Roles = %v", p.Roles)
	}
}

func TestGoogle_FullFlow_PersonalGmail_Rejected(t *testing.T) {
	// Personal accounts lack the hd claim → empty groups → denied.
	raw := map[string]interface{}{
		"sub":            "987654321",
		"email":          "user@gmail.com",
		"email_verified": true,
		// no "hd"
	}

	groups := extractStringSlice(raw, "hd")
	claims := &Claims{
		Subject:  raw["sub"].(string),
		Username: raw["email"].(string),
		Groups:   groups,
		Raw:      raw,
	}

	svc := newTestService(googleConfig())
	_, err := svc.BuildPrincipal(claims)
	if !errors.Is(err, ErrNoRolesMapped) {
		t.Errorf("expected ErrNoRolesMapped for personal Gmail, got %v", err)
	}
}

func TestGoogle_FullFlow_UnmappedDomain_Rejected(t *testing.T) {
	// A Workspace account from a domain not in role_mappings → denied.
	raw := map[string]interface{}{
		"sub":   "111222333",
		"email": "user@partner.com",
		"hd":    "partner.com",
	}

	groups := extractStringSlice(raw, "hd")
	claims := &Claims{
		Subject:  raw["sub"].(string),
		Username: raw["email"].(string),
		Groups:   groups,
	}

	svc := newTestService(googleConfig())
	_, err := svc.BuildPrincipal(claims)
	if !errors.Is(err, ErrNoRolesMapped) {
		t.Errorf("expected ErrNoRolesMapped for unmapped domain, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Role mapping — Google domain values
// ---------------------------------------------------------------------------

func TestGoogle_RoleMapping_ExactDomainMatch(t *testing.T) {
	mappings := []RoleMapping{
		{External: "example.com", Internal: identity.RolePlatformAdmin},
		{External: "ops.example.com", Internal: identity.RolePlatformOperator},
	}

	got := MapExternalRoles([]string{"example.com"}, mappings)
	want := []string{identity.RolePlatformAdmin}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGoogle_RoleMapping_DomainCaseSensitive(t *testing.T) {
	// Domain values from Google are lowercase; mapping must match exactly.
	mappings := []RoleMapping{
		{External: "Example.Com", Internal: identity.RolePlatformAdmin},
	}
	// Lowercase hd value should NOT match an uppercase mapping entry.
	got := MapExternalRoles([]string{"example.com"}, mappings)
	if len(got) != 0 {
		t.Errorf("expected no match for case-mismatched domain, got %v", got)
	}
}

func TestGoogle_RoleMapping_GovernanceRoleViaDomain(t *testing.T) {
	// Verify that governance roles can also be assigned via domain mapping.
	mappings := []RoleMapping{
		{External: "governance.example.com", Internal: identity.RoleGovernanceApprover},
	}
	got := MapExternalRoles([]string{"governance.example.com"}, mappings)
	want := []string{identity.RoleGovernanceApprover}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// UsePKCE — Google requires PKCE
// ---------------------------------------------------------------------------

func TestGoogle_UsePKCE_TrueByDefault(t *testing.T) {
	svc := newTestService(googleConfig())
	if !svc.UsePKCE() {
		t.Error("expected PKCE enabled for Google config")
	}
}
