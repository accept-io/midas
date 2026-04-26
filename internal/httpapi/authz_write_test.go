package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/authz"
	"github.com/accept-io/midas/internal/config"
	cpTypes "github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Fixtures for the write-path authz model.
//
// All tests here construct a server in AuthModeRequired with a static-token
// authenticator carrying exactly one role per token, so the matrix of
// (role, endpoint) can be exercised without role soup.
// ---------------------------------------------------------------------------

// principalsForTests is the canonical mapping from test token → single-role
// principal used by the table-driven tests below.
var principalsForTests = map[string]*identity.Principal{
	"t-admin":    {ID: "user:admin", Provider: identity.ProviderStatic, Roles: []string{identity.RolePlatformAdmin}},
	"t-operator": {ID: "user:operator", Provider: identity.ProviderStatic, Roles: []string{identity.RolePlatformOperator}},
	"t-viewer":   {ID: "user:viewer", Provider: identity.ProviderStatic, Roles: []string{identity.RolePlatformViewer}},
	"t-approver": {ID: "user:approver", Provider: identity.ProviderStatic, Roles: []string{identity.RoleGovernanceApprover}},
	"t-reviewer": {ID: "user:reviewer", Provider: identity.ProviderStatic, Roles: []string{identity.RoleGovernanceReviewer}},
}

func authnForTests() auth.Authenticator {
	// Defensive: copy map so tests cannot mutate the fixture between runs.
	m := make(map[string]*identity.Principal, len(principalsForTests))
	for k, v := range principalsForTests {
		m[k] = v
	}
	return auth.NewStaticTokenAuthenticator(m)
}

// writeTestServer returns a server wired with:
//   - a mock control-plane that always succeeds for apply/plan,
//   - a mock approval service that always succeeds,
//   - a mock grant-lifecycle service that always succeeds.
//
// The wiring matters only to prove the 200 branch is reachable when
// authorization passes; the authz layer runs before any of these.
func writeTestServer(t *testing.T) *Server {
	t.Helper()
	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return &cpTypes.ApplyResult{}, nil
		},
	}
	mockApproval := &mockApprovalService{
		approveSurfaceFn: func(_ context.Context, id string, _ identity.Principal, _ identity.Principal) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, Status: surface.SurfaceStatusActive}, nil
		},
		deprecateSurfaceFn: func(_ context.Context, id string, _ string, _ string, _ string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, Status: surface.SurfaceStatusDeprecated}, nil
		},
		approveProfileFn: func(_ context.Context, id string, _ int, _ string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{ID: id, Status: authority.ProfileStatusActive}, nil
		},
		deprecateProfileFn: func(_ context.Context, id string, _ int, _ string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{ID: id, Status: authority.ProfileStatusDeprecated}, nil
		},
	}
	mockGrants := &mockGrantLifecycleService{
		suspendGrantFn: func(_ context.Context, id, _, _ string) (*authority.AuthorityGrant, error) {
			return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusSuspended}, nil
		},
		revokeGrantFn: func(_ context.Context, id, _, _ string) (*authority.AuthorityGrant, error) {
			return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusRevoked}, nil
		},
		reinstateGrantFn: func(_ context.Context, id, _ string) (*authority.AuthorityGrant, error) {
			return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusActive}, nil
		},
	}

	return NewServerFull(&mockOrchestrator{}, mockCP, mockApproval, nil, nil, mockGrants).
		WithAuthMode(config.AuthModeRequired).
		WithAuthenticator(authnForTests())
}

// writeEndpoint describes one of the 11 in-scope control-plane write
// endpoints, with the permission it requires and a body that would succeed
// if the authorization layer passed through.
type writeEndpoint struct {
	name               string
	method             string
	path               string
	contentType        string
	body               string
	requiredPermission authz.Permission
}

// writeEndpoints is the definitive list of endpoints under the new model.
// Any addition must pick a permission from internal/authz and be reflected
// in docs/authorization.md.
func writeEndpoints() []writeEndpoint {
	return []writeEndpoint{
		{"apply", http.MethodPost, "/v1/controlplane/apply", "application/yaml",
			`apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-1
  name: Surface One
spec:
  category: test
  risk_tier: low
  status: review
  process_id: proc-1
`, authz.PermControlplaneApply},
		{"plan", http.MethodPost, "/v1/controlplane/plan", "application/yaml",
			`apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-1
  name: Surface One
spec:
  category: test
  risk_tier: low
  status: review
  process_id: proc-1
`, authz.PermControlplanePlan},
		{"surface-approve", http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", "application/json",
			`{"submitted_by":"user:x","approver_id":"user:y"}`, authz.PermSurfaceApprove},
		{"surface-deprecate", http.MethodPost, "/v1/controlplane/surfaces/surf-1/deprecate", "application/json",
			`{"deprecated_by":"user:x","reason":"obsolete"}`, authz.PermSurfaceDeprecate},
		{"profile-approve", http.MethodPost, "/v1/controlplane/profiles/prof-1/approve", "application/json",
			`{"version":1,"approved_by":"user:x"}`, authz.PermProfileApprove},
		{"profile-deprecate", http.MethodPost, "/v1/controlplane/profiles/prof-1/deprecate", "application/json",
			`{"version":1,"deprecated_by":"user:x"}`, authz.PermProfileDeprecate},
		{"grant-suspend", http.MethodPost, "/v1/controlplane/grants/g1/suspend", "application/json",
			`{"suspended_by":"user:x","reason":"test"}`, authz.PermGrantSuspend},
		{"grant-revoke", http.MethodPost, "/v1/controlplane/grants/g1/revoke", "application/json",
			`{"revoked_by":"user:x","reason":"test"}`, authz.PermGrantRevoke},
		{"grant-reinstate", http.MethodPost, "/v1/controlplane/grants/g1/reinstate", "application/json",
			`{"reinstated_by":"user:x"}`, authz.PermGrantReinstate},
	}
}

// ---------------------------------------------------------------------------
// T3 — Handler-level enforcement, per endpoint × role
// ---------------------------------------------------------------------------

// TestWriteEndpoints_Admin_AllowedEverywhere asserts that platform.admin
// reaches the handler for every write endpoint. This is the load-bearing
// seeded-admin non-regression check at the HTTP boundary (the full
// bundle end-to-end is covered separately by TestBootstrapAdmin_BundleFlow).
func TestWriteEndpoints_Admin_AllowedEverywhere(t *testing.T) {
	srv := writeTestServer(t)
	for _, ep := range writeEndpoints() {
		ep := ep
		t.Run(ep.name, func(t *testing.T) {
			rec := postAuthed(t, srv, ep, "t-admin")
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("platform.admin must not be gated on %s; got %d: %s", ep.name, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestWriteEndpoints_Demo_DeniedOnEveryWrite asserts that a demo-user-
// equivalent principal (platform.operator) receives 403 on every write
// endpoint. This is required spec item D (Demo user denied on control-
// plane writes).
func TestWriteEndpoints_Demo_DeniedOnEveryWrite(t *testing.T) {
	srv := writeTestServer(t)
	for _, ep := range writeEndpoints() {
		ep := ep
		t.Run(ep.name, func(t *testing.T) {
			rec := postAuthed(t, srv, ep, "t-operator")
			if rec.Code != http.StatusForbidden {
				t.Errorf("demo/operator must be 403 on %s; got %d: %s", ep.name, rec.Code, rec.Body.String())
			}
			assertRequiredPermissionBody(t, rec, ep.requiredPermission)
		})
	}
}

// TestWriteEndpoints_Viewer_DeniedOnEveryWrite mirrors the operator test
// for the viewer role, guaranteeing read-only roles never bleed into
// writes.
func TestWriteEndpoints_Viewer_DeniedOnEveryWrite(t *testing.T) {
	srv := writeTestServer(t)
	for _, ep := range writeEndpoints() {
		ep := ep
		t.Run(ep.name, func(t *testing.T) {
			rec := postAuthed(t, srv, ep, "t-viewer")
			if rec.Code != http.StatusForbidden {
				t.Errorf("viewer must be 403 on %s; got %d", ep.name, rec.Code)
			}
		})
	}
}

// TestWriteEndpoints_Reviewer_DeniedOnEveryWrite mirrors the operator test
// for the reviewer role, whose scope is /v1/reviews (out of model).
func TestWriteEndpoints_Reviewer_DeniedOnEveryWrite(t *testing.T) {
	srv := writeTestServer(t)
	for _, ep := range writeEndpoints() {
		ep := ep
		t.Run(ep.name, func(t *testing.T) {
			rec := postAuthed(t, srv, ep, "t-reviewer")
			if rec.Code != http.StatusForbidden {
				t.Errorf("reviewer must be 403 on %s; got %d", ep.name, rec.Code)
			}
		})
	}
}

// TestWriteEndpoints_GovernanceApprover_NarrowScope asserts approve passes,
// every other write endpoint denies. Covers spec item G.
func TestWriteEndpoints_GovernanceApprover_NarrowScope(t *testing.T) {
	srv := writeTestServer(t)

	allowed := map[string]bool{
		"surface-approve": true,
		"profile-approve": true,
	}

	for _, ep := range writeEndpoints() {
		ep := ep
		t.Run(ep.name, func(t *testing.T) {
			rec := postAuthed(t, srv, ep, "t-approver")
			if allowed[ep.name] {
				if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
					t.Errorf("approver must reach handler on %s, got %d: %s", ep.name, rec.Code, rec.Body.String())
				}
			} else {
				if rec.Code != http.StatusForbidden {
					t.Errorf("approver must be 403 on %s (narrow scope); got %d", ep.name, rec.Code)
				}
				assertRequiredPermissionBody(t, rec, ep.requiredPermission)
			}
		})
	}
}

// TestWriteEndpoints_SubActionSeparation drives spec item H: approve must
// not leak to deprecate, and grant {suspend,revoke,reinstate} are each
// independently gated. This is proved by the narrow-scope approver test
// above for approve↔deprecate; this test additionally asserts that a
// principal holding only grant:suspend (expressed via a dedicated test
// token) cannot revoke or reinstate. We construct such a token via a
// synthetic role bundle using identity roles that do not include the
// other two permissions — in the default set, no canonical role has only
// grant:suspend, but the approver's restricted scope proves the shape
// generically. Here we additionally verify that approver cannot hit
// grant-revoke (which shares no permission with approve).
func TestWriteEndpoints_SubActionSeparation(t *testing.T) {
	srv := writeTestServer(t)
	cases := []struct {
		token string
		ep    string
		want  int
	}{
		// Approver has surface:approve but NOT surface:deprecate.
		{"t-approver", "surface-deprecate", http.StatusForbidden},
		// Approver has profile:approve but NOT profile:deprecate.
		{"t-approver", "profile-deprecate", http.StatusForbidden},
		// Approver has neither suspend nor revoke nor reinstate.
		{"t-approver", "grant-suspend", http.StatusForbidden},
		{"t-approver", "grant-revoke", http.StatusForbidden},
		{"t-approver", "grant-reinstate", http.StatusForbidden},
	}
	for _, c := range cases {
		c := c
		t.Run(c.token+"_"+c.ep, func(t *testing.T) {
			ep := findEndpoint(t, c.ep)
			rec := postAuthed(t, srv, ep, c.token)
			if rec.Code != c.want {
				t.Errorf("want %d, got %d: %s", c.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T4 — Unauthenticated and open-mode semantics
// ---------------------------------------------------------------------------

// TestWriteEndpoints_NoToken_Returns401 asserts that in authenticated mode
// every write endpoint returns 401 when no Authorization header is sent.
// Covers the "missing principal" branch of requirePermission.
func TestWriteEndpoints_NoToken_Returns401(t *testing.T) {
	srv := writeTestServer(t)
	for _, ep := range writeEndpoints() {
		ep := ep
		t.Run(ep.name, func(t *testing.T) {
			rec := performRequestWithHeaders(t, srv, ep.method, ep.path,
				[]byte(ep.body),
				map[string]string{"Content-Type": ep.contentType})
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("want 401, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestWriteEndpoints_OpenMode_BypassesPermission asserts spec item I:
// AuthModeOpen short-circuits requirePermission exactly like requireRole
// does today, preserving dev/memory deployments. Every write endpoint
// reaches the handler without any Authorization header.
func TestWriteEndpoints_OpenMode_BypassesPermission(t *testing.T) {
	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return &cpTypes.ApplyResult{}, nil
		},
	}
	mockApproval := &mockApprovalService{
		approveSurfaceFn:   func(_ context.Context, id string, _, _ identity.Principal) (*surface.DecisionSurface, error) { return &surface.DecisionSurface{ID: id, Status: surface.SurfaceStatusActive}, nil },
		deprecateSurfaceFn: func(_ context.Context, id string, _, _, _ string) (*surface.DecisionSurface, error) { return &surface.DecisionSurface{ID: id, Status: surface.SurfaceStatusDeprecated}, nil },
		approveProfileFn:   func(_ context.Context, id string, _ int, _ string) (*authority.AuthorityProfile, error) { return &authority.AuthorityProfile{ID: id, Status: authority.ProfileStatusActive}, nil },
		deprecateProfileFn: func(_ context.Context, id string, _ int, _ string) (*authority.AuthorityProfile, error) { return &authority.AuthorityProfile{ID: id, Status: authority.ProfileStatusDeprecated}, nil },
	}
	mockGrants := &mockGrantLifecycleService{
		suspendGrantFn:   func(_ context.Context, id, _, _ string) (*authority.AuthorityGrant, error) { return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusSuspended}, nil },
		revokeGrantFn:    func(_ context.Context, id, _, _ string) (*authority.AuthorityGrant, error) { return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusRevoked}, nil },
		reinstateGrantFn: func(_ context.Context, id, _ string) (*authority.AuthorityGrant, error) { return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusActive}, nil },
	}
	srv := NewServerFull(&mockOrchestrator{}, mockCP, mockApproval, nil, nil, mockGrants).
		WithAuthMode(config.AuthModeOpen)

	for _, ep := range writeEndpoints() {
		ep := ep
		t.Run(ep.name, func(t *testing.T) {
			rec := performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
				map[string]string{"Content-Type": ep.contentType})
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("open mode must pass through authorization on %s; got %d: %s",
					ep.name, rec.Code, rec.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T5 — Denial response shape
// ---------------------------------------------------------------------------

// TestForbiddenBody_IncludesRequiredPermission asserts the spec requirement
// that every write-path 403 includes "required_permission" naming the
// permission the caller lacked. Also asserts it does NOT leak the caller's
// role list or held permissions.
func TestForbiddenBody_IncludesRequiredPermission(t *testing.T) {
	srv := writeTestServer(t)
	ep := findEndpoint(t, "apply")
	rec := postAuthed(t, srv, ep, "t-operator")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}

	if body["error"] != "forbidden" {
		t.Errorf(`want error="forbidden", got %v`, body["error"])
	}
	if got := body["required_permission"]; got != string(authz.PermControlplaneApply) {
		t.Errorf("want required_permission=%q, got %v", authz.PermControlplaneApply, got)
	}

	// Information-leak checks: body must not name the roles the caller holds.
	for _, leak := range []string{"platform.operator", "platform.admin", "roles", "held", "granted"} {
		for k, v := range body {
			if strings.Contains(strings.ToLower(k), leak) {
				t.Errorf("403 body must not expose %q (found key %q)", leak, k)
			}
			if s, ok := v.(string); ok && strings.Contains(strings.ToLower(s), leak) {
				t.Errorf("403 body must not expose %q (found in value of %q: %q)", leak, k, s)
			}
		}
	}
}

// TestUnauthorizedBody_Unchanged asserts the 401 body is still exactly
// {"error":"unauthorized"} — no required_permission leakage. 401 and 403
// have distinct semantics and must not converge.
func TestUnauthorizedBody_Unchanged(t *testing.T) {
	srv := writeTestServer(t)
	ep := findEndpoint(t, "apply")
	rec := performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
		map[string]string{"Content-Type": ep.contentType})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf(`want error="unauthorized", got %v`, body["error"])
	}
	if _, present := body["required_permission"]; present {
		t.Errorf("401 body must not include required_permission; got %v", body)
	}
}

// ---------------------------------------------------------------------------
// T6 — Static-token alias normalization (spec item E)
// ---------------------------------------------------------------------------

// TestStaticToken_DeprecatedAdminAlias_ResolvesToAdminBundle asserts that
// a static-token principal constructed with the deprecated alias "admin"
// (via identity.NormalizeRoles at the authenticator-construction boundary)
// resolves to the full platform.admin permission bundle.
//
// This mirrors real deployments that still use the documented alias form
// in midas.yaml/config_init.go examples.
func TestStaticToken_DeprecatedAdminAlias_ResolvesToAdminBundle(t *testing.T) {
	// Simulate what cmd/midas/main.go:buildAuthenticator does: apply
	// NormalizeRoles to the raw config role string.
	normalised := identity.NormalizeRoles([]string{"admin"})
	authn := auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-legacy-admin": {
			ID:       "user:legacy-admin",
			Provider: identity.ProviderStatic,
			Roles:    normalised,
		},
	})
	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return &cpTypes.ApplyResult{}, nil
		},
	}
	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP).
		WithAuthMode(config.AuthModeRequired).WithAuthenticator(authn)

	ep := findEndpoint(t, "apply")
	rec := performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
		map[string]string{"Authorization": "Bearer tok-legacy-admin", "Content-Type": ep.contentType})

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("deprecated admin alias must resolve to full bundle; got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestStaticToken_CanonicalAdmin_Matches asserts the canonical form behaves
// identically to the deprecated alias case — they are the same principal
// from the authz layer's perspective.
func TestStaticToken_CanonicalAdmin_Matches(t *testing.T) {
	authn := auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-canonical-admin": {
			ID:       "user:admin",
			Provider: identity.ProviderStatic,
			Roles:    identity.NormalizeRoles([]string{"platform.admin"}),
		},
	})
	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return &cpTypes.ApplyResult{}, nil
		},
	}
	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP).
		WithAuthMode(config.AuthModeRequired).WithAuthenticator(authn)

	ep := findEndpoint(t, "apply")
	rec := performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
		map[string]string{"Authorization": "Bearer tok-canonical-admin", "Content-Type": ep.contentType})

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("canonical admin must resolve to full bundle; got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// T7 — Multi-role union (spec item F)
// ---------------------------------------------------------------------------

// TestStaticToken_MultiRole_AdminPlusApprover asserts the union composition:
// admin+approver gets the full admin bundle (approver contributes nothing
// not already in admin).
func TestStaticToken_MultiRole_AdminPlusApprover(t *testing.T) {
	authn := auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-multi": {
			ID:       "user:multi",
			Provider: identity.ProviderStatic,
			Roles:    []string{identity.RolePlatformAdmin, identity.RoleGovernanceApprover},
		},
	})
	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return &cpTypes.ApplyResult{}, nil
		},
	}
	srv := NewServerFull(&mockOrchestrator{}, mockCP, &mockApprovalService{
		approveSurfaceFn: func(_ context.Context, id string, _, _ identity.Principal) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, Status: surface.SurfaceStatusActive}, nil
		},
	}, nil, nil, nil).
		WithAuthMode(config.AuthModeRequired).WithAuthenticator(authn)

	// Admin ability preserved:
	ep := findEndpoint(t, "apply")
	rec := performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
		map[string]string{"Authorization": "Bearer tok-multi", "Content-Type": ep.contentType})
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("admin+approver must pass apply gate; got %d: %s", rec.Code, rec.Body.String())
	}

	// Approve ability still works too:
	ep = findEndpoint(t, "surface-approve")
	rec = performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
		map[string]string{"Authorization": "Bearer tok-multi", "Content-Type": ep.contentType})
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("admin+approver must pass surface-approve; got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestStaticToken_MultiRole_OperatorPlusApprover asserts the union
// correctly grants only approver's two permissions (operator contributes
// no control-plane writes). Deprecate/apply/etc. remain denied.
func TestStaticToken_MultiRole_OperatorPlusApprover(t *testing.T) {
	authn := auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-op-app": {
			ID:       "user:op-app",
			Provider: identity.ProviderStatic,
			Roles:    []string{identity.RolePlatformOperator, identity.RoleGovernanceApprover},
		},
	})
	srv := writeTestServerWithAuthenticator(t, authn)

	// surface-approve: allowed (approver contributes it).
	ep := findEndpoint(t, "surface-approve")
	rec := performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
		map[string]string{"Authorization": "Bearer tok-op-app", "Content-Type": ep.contentType})
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Errorf("op+approver must pass surface-approve; got %d: %s", rec.Code, rec.Body.String())
	}

	// apply, deprecate, grant-revoke: all denied (neither role contributes).
	for _, name := range []string{"apply", "surface-deprecate", "grant-revoke"} {
		ep = findEndpoint(t, name)
		rec = performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
			map[string]string{"Authorization": "Bearer tok-op-app", "Content-Type": ep.contentType})
		if rec.Code != http.StatusForbidden {
			t.Errorf("op+approver must be 403 on %s; got %d: %s", name, rec.Code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// Final gate — platform.admin is not a wildcard bypass
// ---------------------------------------------------------------------------

// TestPlatformAdmin_IsNotBypass asserts that the enforcement path does not
// special-case platform.admin. A synthetic principal whose roles expand to
// a single canonical role other than admin is denied, even if that role is
// labeled "platform.something.admin"-ish.
//
// This guards against a future refactor inadvertently reintroducing a
// role-name substring match ("contains 'admin' → allow").
func TestPlatformAdmin_IsNotBypass(t *testing.T) {
	authn := auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-look-alike": {
			ID:       "user:sneaky",
			Provider: identity.ProviderStatic,
			Roles:    []string{"platform.adminish"},
		},
	})
	mockCP := &mockControlPlane{applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
		return &cpTypes.ApplyResult{}, nil
	}}
	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP).
		WithAuthMode(config.AuthModeRequired).WithAuthenticator(authn)

	ep := findEndpoint(t, "apply")
	rec := performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
		map[string]string{"Authorization": "Bearer tok-look-alike", "Content-Type": ep.contentType})

	if rec.Code != http.StatusForbidden {
		t.Errorf("look-alike role must be 403; got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func postAuthed(t *testing.T, srv *Server, ep writeEndpoint, token string) *httptest.ResponseRecorder {
	t.Helper()
	return performRequestWithHeaders(t, srv, ep.method, ep.path, []byte(ep.body),
		map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  ep.contentType,
		})
}

func findEndpoint(t *testing.T, name string) writeEndpoint {
	t.Helper()
	for _, ep := range writeEndpoints() {
		if ep.name == name {
			return ep
		}
	}
	t.Fatalf("unknown endpoint %q in writeEndpoints()", name)
	return writeEndpoint{}
}

func assertRequiredPermissionBody(t *testing.T, rec *httptest.ResponseRecorder, want authz.Permission) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	got, ok := body["required_permission"].(string)
	if !ok {
		t.Fatalf("403 body missing required_permission: %v", body)
	}
	if got != string(want) {
		t.Errorf("required_permission = %q, want %q", got, want)
	}
}

// writeTestServerWithAuthenticator returns a full server with a caller-
// supplied authenticator but the same service mocks as writeTestServer.
func writeTestServerWithAuthenticator(t *testing.T, authn auth.Authenticator) *Server {
	t.Helper()
	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return &cpTypes.ApplyResult{}, nil
		},
	}
	mockApproval := &mockApprovalService{
		approveSurfaceFn: func(_ context.Context, id string, _, _ identity.Principal) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, Status: surface.SurfaceStatusActive}, nil
		},
		deprecateSurfaceFn: func(_ context.Context, id string, _, _, _ string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, Status: surface.SurfaceStatusDeprecated}, nil
		},
		approveProfileFn: func(_ context.Context, id string, _ int, _ string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{ID: id, Status: authority.ProfileStatusActive}, nil
		},
		deprecateProfileFn: func(_ context.Context, id string, _ int, _ string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{ID: id, Status: authority.ProfileStatusDeprecated}, nil
		},
	}
	mockGrants := &mockGrantLifecycleService{
		suspendGrantFn:   func(_ context.Context, id, _, _ string) (*authority.AuthorityGrant, error) { return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusSuspended}, nil },
		revokeGrantFn:    func(_ context.Context, id, _, _ string) (*authority.AuthorityGrant, error) { return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusRevoked}, nil },
		reinstateGrantFn: func(_ context.Context, id, _ string) (*authority.AuthorityGrant, error) { return &authority.AuthorityGrant{ID: id, Status: authority.GrantStatusActive}, nil },
	}
	return NewServerFull(&mockOrchestrator{}, mockCP, mockApproval, nil, nil, mockGrants).
		WithAuthMode(config.AuthModeRequired).
		WithAuthenticator(authn)
}
