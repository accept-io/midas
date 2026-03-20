package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// GET /v1/surfaces/{id}/recovery
// ---------------------------------------------------------------------------

func TestGetSurfaceRecovery_ActiveVersion_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	activeVer := 1
	activeSt := "active"

	svc := &mockIntrospectionService{
		getSurfaceRecoveryFn: func(ctx context.Context, id string) (*SurfaceRecoveryResult, error) {
			if id != "surf-test" {
				t.Errorf("unexpected id %q", id)
			}
			return &SurfaceRecoveryResult{
				SurfaceID:              "surf-test",
				LatestVersion:          1,
				LatestStatus:           "active",
				ActiveVersion:          &activeVer,
				ActiveStatus:           &activeSt,
				SuccessorSurfaceID:     "",
				DeprecationReason:      "",
				VersionCount:           1,
				Warnings:               []string{},
				RecommendedNextActions: []string{},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/surf-test/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[SurfaceRecoveryResult](t, rec)
	if resp.SurfaceID != "surf-test" {
		t.Errorf("expected surface_id 'surf-test', got %q", resp.SurfaceID)
	}
	if resp.LatestVersion != 1 {
		t.Errorf("expected latest_version 1, got %d", resp.LatestVersion)
	}
	if resp.ActiveVersion == nil {
		t.Fatal("expected non-nil active_version")
	}
	if *resp.ActiveVersion != 1 {
		t.Errorf("expected active_version 1, got %d", *resp.ActiveVersion)
	}
	if resp.ActiveStatus == nil {
		t.Fatal("expected non-nil active_status")
	}
	if *resp.ActiveStatus != "active" {
		t.Errorf("expected active_status 'active', got %q", *resp.ActiveStatus)
	}

	_ = now // referenced to avoid unused import
}

func TestGetSurfaceRecovery_ReviewOnly_NilActiveVersion(t *testing.T) {
	svc := &mockIntrospectionService{
		getSurfaceRecoveryFn: func(ctx context.Context, id string) (*SurfaceRecoveryResult, error) {
			return &SurfaceRecoveryResult{
				SurfaceID:              "surf-pending",
				LatestVersion:          1,
				LatestStatus:           "review",
				ActiveVersion:          nil,
				ActiveStatus:           nil,
				VersionCount:           1,
				Warnings:               []string{"no active version — evaluation requests will be rejected"},
				RecommendedNextActions: []string{"approve latest review version to enable evaluation"},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/surf-pending/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Parse as a map to check null fields explicitly.
	resp := decodeJSON[map[string]any](t, rec)

	if v, ok := resp["active_version"]; !ok || v != nil {
		t.Errorf("expected active_version to be null, got %v", v)
	}
	if v, ok := resp["active_status"]; !ok || v != nil {
		t.Errorf("expected active_status to be null, got %v", v)
	}

	warnings, _ := resp["warnings"].([]any)
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}

	actions, _ := resp["recommended_next_actions"].([]any)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
	if len(actions) > 0 && actions[0] != "approve latest review version to enable evaluation" {
		t.Errorf("unexpected action: %v", actions[0])
	}
}

func TestGetSurfaceRecovery_NotFound_Returns404(t *testing.T) {
	svc := &mockIntrospectionService{
		getSurfaceRecoveryFn: func(ctx context.Context, id string) (*SurfaceRecoveryResult, error) {
			return nil, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/no-such-surface/recovery", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetSurfaceRecovery_NilService_Returns501(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/surf-test/recovery", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
}

func TestGetSurfaceRecovery_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodPost, "/v1/surfaces/surf-test/recovery", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestGetSurfaceRecovery_DeprecatedWithSuccessor(t *testing.T) {
	svc := &mockIntrospectionService{
		getSurfaceRecoveryFn: func(ctx context.Context, id string) (*SurfaceRecoveryResult, error) {
			return &SurfaceRecoveryResult{
				SurfaceID:              "surf-old",
				LatestVersion:          1,
				LatestStatus:           "deprecated",
				ActiveVersion:          nil,
				ActiveStatus:           nil,
				SuccessorSurfaceID:     "surf-new",
				DeprecationReason:      "replaced by new surface",
				VersionCount:           1,
				Warnings:               []string{"surface is deprecated and should not receive new grants"},
				RecommendedNextActions: []string{"inspect successor surface 'surf-new' and plan grant migration"},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/surf-old/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[SurfaceRecoveryResult](t, rec)
	if resp.SuccessorSurfaceID != "surf-new" {
		t.Errorf("expected successor_surface_id 'surf-new', got %q", resp.SuccessorSurfaceID)
	}
	if resp.DeprecationReason != "replaced by new surface" {
		t.Errorf("expected deprecation_reason 'replaced by new surface', got %q", resp.DeprecationReason)
	}
	if len(resp.RecommendedNextActions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(resp.RecommendedNextActions))
	}
	want := "inspect successor surface 'surf-new' and plan grant migration"
	if resp.RecommendedNextActions[0] != want {
		t.Errorf("expected action %q, got %q", want, resp.RecommendedNextActions[0])
	}
}

// ---------------------------------------------------------------------------
// GET /v1/profiles/{id}/recovery
// ---------------------------------------------------------------------------

func TestGetProfileRecovery_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	activeVer := 2
	activeSt := "active"

	svc := &mockIntrospectionService{
		getProfileRecoveryFn: func(ctx context.Context, id string) (*ProfileRecoveryResult, error) {
			if id != "prof-test" {
				t.Errorf("unexpected profile id %q", id)
			}
			return &ProfileRecoveryResult{
				ProfileID:     "prof-test",
				SurfaceID:     "surf-test",
				LatestVersion: 2,
				LatestStatus:  "active",
				ActiveVersion: &activeVer,
				ActiveStatus:  &activeSt,
				VersionCount:  2,
				Versions: []ProfileVersionEntry{
					{Version: 2, Status: "active", EffectiveFrom: now.Add(-time.Hour)},
					{Version: 1, Status: "active", EffectiveFrom: now.Add(-48 * time.Hour)},
				},
				ActiveGrantCount:       3,
				CapabilityNote:         "profiles are persisted with status=active immediately on apply; there is no review/approval checkpoint for profiles in the current implementation",
				Warnings:               []string{},
				RecommendedNextActions: []string{"inspect dependent grants before deprecating this profile version"},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/prof-test/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[ProfileRecoveryResult](t, rec)
	if resp.ProfileID != "prof-test" {
		t.Errorf("expected profile_id 'prof-test', got %q", resp.ProfileID)
	}
	if resp.SurfaceID != "surf-test" {
		t.Errorf("expected surface_id 'surf-test', got %q", resp.SurfaceID)
	}
	if resp.LatestVersion != 2 {
		t.Errorf("expected latest_version 2, got %d", resp.LatestVersion)
	}
	if resp.ActiveVersion == nil {
		t.Fatal("expected non-nil active_version")
	}
	if *resp.ActiveVersion != 2 {
		t.Errorf("expected active_version 2, got %d", *resp.ActiveVersion)
	}
	if resp.ActiveGrantCount != 3 {
		t.Errorf("expected active_grant_count 3, got %d", resp.ActiveGrantCount)
	}
	if len(resp.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(resp.Versions))
	}
	if resp.CapabilityNote == "" {
		t.Error("expected non-empty capability_note")
	}
}

func TestGetProfileRecovery_NotFound_Returns404(t *testing.T) {
	svc := &mockIntrospectionService{
		getProfileRecoveryFn: func(ctx context.Context, id string) (*ProfileRecoveryResult, error) {
			return nil, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/no-such-profile/recovery", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetProfileRecovery_NilService_Returns501(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/prof-test/recovery", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
}

func TestGetProfileRecovery_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodPost, "/v1/profiles/prof-test/recovery", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestGetProfileRecovery_NoActiveVersion(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	svc := &mockIntrospectionService{
		getProfileRecoveryFn: func(ctx context.Context, id string) (*ProfileRecoveryResult, error) {
			return &ProfileRecoveryResult{
				ProfileID:     "prof-future",
				SurfaceID:     "surf-test",
				LatestVersion: 1,
				LatestStatus:  "active",
				ActiveVersion: nil,
				ActiveStatus:  nil,
				VersionCount:  1,
				Versions: []ProfileVersionEntry{
					{Version: 1, Status: "active", EffectiveFrom: now.Add(24 * time.Hour)},
				},
				ActiveGrantCount:       0,
				CapabilityNote:         "profiles are persisted with status=active immediately on apply; there is no review/approval checkpoint for profiles in the current implementation",
				Warnings:               []string{"no version is currently effective; a version exists with a future effective_date"},
				RecommendedNextActions: []string{"re-apply profile with an effective_date in the past to restore evaluation eligibility"},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/prof-future/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[map[string]any](t, rec)
	if v := resp["active_version"]; v != nil {
		t.Errorf("expected active_version null, got %v", v)
	}

	warnings, _ := resp["warnings"].([]any)
	if len(warnings) == 0 {
		t.Error("expected at least one warning")
	}

	actions, _ := resp["recommended_next_actions"].([]any)
	if len(actions) == 0 {
		t.Error("expected at least one recommended action")
	}
}

// ---------------------------------------------------------------------------
// Helpers — verify the surface recovery action strings exactly
// ---------------------------------------------------------------------------

func TestSurfaceRecoveryActions_ReviewOnly_NoActive(t *testing.T) {
	now := time.Now().UTC()
	latest := &surface.DecisionSurface{
		ID:            "surf-1",
		Version:       1,
		Status:        surface.SurfaceStatusReview,
		EffectiveFrom: now.Add(-time.Hour),
	}
	actions := buildSurfaceRecoveryActions(latest, nil, []*surface.DecisionSurface{latest})
	if len(actions) != 1 || actions[0] != "approve latest review version to enable evaluation" {
		t.Errorf("unexpected actions: %v", actions)
	}
}

func TestSurfaceRecoveryActions_ReviewAndActive(t *testing.T) {
	now := time.Now().UTC()
	v1 := &surface.DecisionSurface{ID: "s", Version: 1, Status: surface.SurfaceStatusActive, EffectiveFrom: now.Add(-2 * time.Hour)}
	v2 := &surface.DecisionSurface{ID: "s", Version: 2, Status: surface.SurfaceStatusReview, EffectiveFrom: now.Add(-time.Hour)}
	actions := buildSurfaceRecoveryActions(v2, v1, []*surface.DecisionSurface{v1, v2})
	if len(actions) != 1 || actions[0] != "approve latest review version to activate updated configuration" {
		t.Errorf("unexpected actions: %v", actions)
	}
}

func TestSurfaceRecoveryActions_DeprecatedWithSuccessor(t *testing.T) {
	latest := &surface.DecisionSurface{
		ID:                 "surf-old",
		Version:            1,
		Status:             surface.SurfaceStatusDeprecated,
		SuccessorSurfaceID: "surf-new",
	}
	actions := buildSurfaceRecoveryActions(latest, nil, []*surface.DecisionSurface{latest})
	if len(actions) != 1 || actions[0] != "inspect successor surface 'surf-new' and plan grant migration" {
		t.Errorf("unexpected actions: %v", actions)
	}
}

func TestSurfaceRecoveryActions_DeprecatedNoSuccessor_NoReview(t *testing.T) {
	latest := &surface.DecisionSurface{
		ID:      "surf-old",
		Version: 1,
		Status:  surface.SurfaceStatusDeprecated,
	}
	actions := buildSurfaceRecoveryActions(latest, nil, []*surface.DecisionSurface{latest})
	if len(actions) != 1 || actions[0] != "apply replacement surface" {
		t.Errorf("unexpected actions: %v", actions)
	}
}

func TestSurfaceRecoveryActions_DeprecatedNoSuccessor_WithReview(t *testing.T) {
	now := time.Now().UTC()
	v1 := &surface.DecisionSurface{ID: "s", Version: 1, Status: surface.SurfaceStatusDeprecated}
	v2 := &surface.DecisionSurface{ID: "s", Version: 2, Status: surface.SurfaceStatusReview, EffectiveFrom: now.Add(-time.Hour)}
	actions := buildSurfaceRecoveryActions(v2, nil, []*surface.DecisionSurface{v1, v2})
	if len(actions) != 1 || actions[0] != "approve replacement surface" {
		t.Errorf("unexpected actions: %v", actions)
	}
}
