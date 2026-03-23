package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/identity"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestServer() *Server {
	return NewServerFull(nil, nil, nil, nil, nil, nil)
}

func aliceAuthenticator() auth.Authenticator {
	return auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		"tok-alice": {
			ID:       "user:alice",
			Provider: identity.ProviderStatic,
			Roles:    []string{identity.RoleAdmin},
		},
	})
}

// ---------------------------------------------------------------------------
// PrincipalFromContext
// ---------------------------------------------------------------------------

func TestPrincipalFromContext_NoPrincipal(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := PrincipalFromContext(r.Context()); got != nil {
		t.Errorf("want nil, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// actorFromContext
// ---------------------------------------------------------------------------

func TestActorFromContext_NoPrincipal_ReturnsFallback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got := actorFromContext(r.Context(), "body-actor")
	if got != "body-actor" {
		t.Errorf("want body-actor, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// requireAuth — no authenticator configured (no-op)
// ---------------------------------------------------------------------------

func TestRequireAuth_NoAuthenticator_PassesThrough(t *testing.T) {
	srv := newTestServer()

	called := false
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if !called {
		t.Error("inner handler should be called when no authenticator is set")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// requireAuth — authenticator configured
// ---------------------------------------------------------------------------

func TestRequireAuth_ValidToken_StoresPrincipalInContext(t *testing.T) {
	srv := newTestServer().WithAuthenticator(aliceAuthenticator())

	var capturedPrincipal *identity.Principal
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		capturedPrincipal = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.Header.Set("Authorization", "Bearer tok-alice")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if capturedPrincipal == nil {
		t.Fatal("principal not stored in context")
	}
	if capturedPrincipal.ID != "user:alice" {
		t.Errorf("want user:alice, got %q", capturedPrincipal.ID)
	}
}

func TestRequireAuth_MissingToken_Returns401(t *testing.T) {
	srv := newTestServer().WithAuthenticator(aliceAuthenticator())

	called := false
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Error("inner handler must not be called on auth failure")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestRequireAuth_UnknownToken_Returns401(t *testing.T) {
	srv := newTestServer().WithAuthenticator(aliceAuthenticator())

	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// WithAuthenticator — post-construction wiring
// ---------------------------------------------------------------------------

func TestWithAuthenticator_AfterConstruction(t *testing.T) {
	srv := newTestServer()
	if srv.authenticator != nil {
		t.Fatal("authenticator should be nil before WithAuthenticator")
	}

	returned := srv.WithAuthenticator(aliceAuthenticator())
	if returned != srv {
		t.Error("WithAuthenticator should return the same server instance")
	}
	if srv.authenticator == nil {
		t.Error("authenticator should be set after WithAuthenticator")
	}
}

// ---------------------------------------------------------------------------
// actorFromContext — principal present
// ---------------------------------------------------------------------------

func TestActorFromContext_WithPrincipal_ReturnsID(t *testing.T) {
	srv := newTestServer().WithAuthenticator(aliceAuthenticator())

	var capturedActor string
	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		capturedActor = actorFromContext(r.Context(), "body-actor")
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.Header.Set("Authorization", "Bearer tok-alice")
	w := httptest.NewRecorder()
	handler(w, r)

	if capturedActor != "user:alice" {
		t.Errorf("want user:alice, got %q", capturedActor)
	}
}

// ---------------------------------------------------------------------------
// Handler-level integration tests: auth enforcement and principal propagation
// ---------------------------------------------------------------------------

// TestHandlerAuth_Reviews_NoToken_Returns401 verifies that POST /v1/reviews
// rejects unauthenticated requests when an authenticator is configured.
func TestHandlerAuth_Reviews_NoToken_Returns401(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthenticator(aliceAuthenticator())

	body := marshalJSON(t, map[string]any{
		"envelope_id": "env-abc",
		"decision":    "approve",
		"reviewer":    "imposter",
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", body)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandlerAuth_Reviews_TokenOverridesBodyReviewer verifies that when a
// valid bearer token is present, the authenticated principal's ID is used as
// ReviewerID — the body-supplied "reviewer" field is ignored.
func TestHandlerAuth_Reviews_TokenOverridesBodyReviewer(t *testing.T) {
	var capturedReviewerID string

	mock := &mockOrchestrator{
		resolveEscalationFn: func(_ context.Context, res decision.EscalationResolution) (*envelope.Envelope, error) {
			capturedReviewerID = res.ReviewerID
			return nil, nil // nil envelope handled gracefully by the handler
		},
	}

	srv := NewServerFull(mock, nil, nil, nil, nil, nil).
		WithAuthenticator(aliceAuthenticator())

	body := marshalJSON(t, map[string]any{
		"envelope_id": "env-abc",
		"decision":    "approve",
		"reviewer":    "imposter", // should be overridden by the token principal
	})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/reviews", body,
		map[string]string{"Authorization": "Bearer tok-alice"})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if capturedReviewerID != "user:alice" {
		t.Errorf("want ReviewerID=user:alice (from token), got %q", capturedReviewerID)
	}
}

// TestHandlerAuth_ApproveProfile_TokenOverridesBodyActor verifies that when a
// valid bearer token is present on a governance handler that already uses
// actorFromContext, the authenticated principal's ID is used as the actor —
// the body-supplied actor field is ignored.
func TestHandlerAuth_ApproveProfile_TokenOverridesBodyActor(t *testing.T) {
	var capturedApprovedBy string

	mockApproval := &mockApprovalService{
		approveProfileFn: func(_ context.Context, _ string, _ int, approvedBy string) (*authority.AuthorityProfile, error) {
			capturedApprovedBy = approvedBy
			return &authority.AuthorityProfile{
				ID:         "prof-1",
				Version:    1,
				Status:     authority.ProfileStatusActive,
				ApprovedBy: approvedBy,
			}, nil
		},
	}

	srv := NewServerFull(&mockOrchestrator{}, nil, mockApproval, nil, nil, nil).
		WithAuthenticator(aliceAuthenticator())

	body := marshalJSON(t, map[string]any{
		"version":     1,
		"approved_by": "imposter", // should be overridden by the token principal
	})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/profiles/prof-1/approve", body,
		map[string]string{"Authorization": "Bearer tok-alice"})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if capturedApprovedBy != "user:alice" {
		t.Errorf("want approvedBy=user:alice (from token), got %q", capturedApprovedBy)
	}
}
