package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
)

// newFullIntrospectionServer builds a real IntrospectionService backed by
// in-memory repositories and wires it into a Server.
func newFullIntrospectionServer(surfaces *memory.SurfaceRepo, profiles *memory.ProfileRepo, grants *memory.GrantRepo) *Server {
	svc := NewIntrospectionServiceFull(surfaces, profiles, nil, grants)
	return NewServerWithAllServices(&mockOrchestrator{}, nil, nil, svc)
}

// ---------------------------------------------------------------------------
// Surface recovery integration tests
// ---------------------------------------------------------------------------

func TestSurfaceRecovery_ActiveV1_ReviewV2_DistinctVersions(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	surfaces := memory.NewSurfaceRepo()

	v1 := &surface.DecisionSurface{
		ID:             "surf-multi",
		Version:        1,
		Status:         surface.SurfaceStatusActive,
		Name:           "Multi Version Surface",
		Domain:         "test",
		ProcessID:      "proc-test",
		EffectiveFrom:  now.Add(-48 * time.Hour),
		BusinessOwner:  "team-a",
		TechnicalOwner: "team-b",
		CreatedAt:      now.Add(-48 * time.Hour),
		UpdatedAt:      now.Add(-48 * time.Hour),
	}
	v2 := &surface.DecisionSurface{
		ID:             "surf-multi",
		Version:        2,
		Status:         surface.SurfaceStatusReview,
		Name:           "Multi Version Surface v2",
		Domain:         "test",
		ProcessID:      "proc-test",
		EffectiveFrom:  now.Add(-time.Hour),
		BusinessOwner:  "team-a",
		TechnicalOwner: "team-b",
		CreatedAt:      now.Add(-time.Hour),
		UpdatedAt:      now.Add(-time.Hour),
	}
	if err := surfaces.Create(ctx, v1); err != nil {
		t.Fatal(err)
	}
	if err := surfaces.Create(ctx, v2); err != nil {
		t.Fatal(err)
	}

	srv := newFullIntrospectionServer(surfaces, memory.NewProfileRepo(), memory.NewGrantRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/surf-multi/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[SurfaceRecoveryResult](t, rec)

	if resp.VersionCount != 2 {
		t.Errorf("expected version_count 2, got %d", resp.VersionCount)
	}
	if resp.LatestVersion != 2 {
		t.Errorf("expected latest_version 2, got %d", resp.LatestVersion)
	}
	if resp.LatestStatus != "review" {
		t.Errorf("expected latest_status 'review', got %q", resp.LatestStatus)
	}
	if resp.ActiveVersion == nil {
		t.Fatal("expected non-nil active_version")
	}
	if *resp.ActiveVersion != 1 {
		t.Errorf("expected active_version 1, got %d", *resp.ActiveVersion)
	}
	if resp.ActiveStatus == nil || *resp.ActiveStatus != "active" {
		t.Errorf("expected active_status 'active'")
	}

	// With v2 in review and v1 active, the recommended action is to approve the review version.
	if len(resp.RecommendedNextActions) != 1 {
		t.Fatalf("expected 1 action, got %d: %v", len(resp.RecommendedNextActions), resp.RecommendedNextActions)
	}
	if resp.RecommendedNextActions[0] != "approve latest review version to activate updated configuration" {
		t.Errorf("unexpected action: %q", resp.RecommendedNextActions[0])
	}
}

func TestSurfaceRecovery_DeprecatedWithSuccessor_SuccessorVisible(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	surfaces := memory.NewSurfaceRepo()
	deprecatedSurf := &surface.DecisionSurface{
		ID:                 "surf-old",
		Version:            1,
		Status:             surface.SurfaceStatusDeprecated,
		Name:               "Old Surface",
		Domain:             "payments",
		ProcessID:          "proc-test",
		EffectiveFrom:      now.Add(-7 * 24 * time.Hour),
		SuccessorSurfaceID: "surf-replacement",
		DeprecationReason:  "migrated to new surface",
		BusinessOwner:      "ops",
		TechnicalOwner:     "platform",
		CreatedAt:          now.Add(-7 * 24 * time.Hour),
		UpdatedAt:          now.Add(-24 * time.Hour),
	}
	if err := surfaces.Create(ctx, deprecatedSurf); err != nil {
		t.Fatal(err)
	}

	srv := newFullIntrospectionServer(surfaces, memory.NewProfileRepo(), memory.NewGrantRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/surf-old/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[SurfaceRecoveryResult](t, rec)
	if resp.SuccessorSurfaceID != "surf-replacement" {
		t.Errorf("expected successor 'surf-replacement', got %q", resp.SuccessorSurfaceID)
	}
	if resp.DeprecationReason != "migrated to new surface" {
		t.Errorf("unexpected deprecation_reason: %q", resp.DeprecationReason)
	}
	if resp.ActiveVersion != nil {
		t.Errorf("expected nil active_version for deprecated surface")
	}

	want := "inspect successor surface 'surf-replacement' and plan grant migration"
	if len(resp.RecommendedNextActions) == 0 || resp.RecommendedNextActions[0] != want {
		t.Errorf("expected action %q, got %v", want, resp.RecommendedNextActions)
	}
}

func TestSurfaceRecovery_NotFound(t *testing.T) {
	srv := newFullIntrospectionServer(memory.NewSurfaceRepo(), memory.NewProfileRepo(), memory.NewGrantRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/does-not-exist/recovery", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Profile recovery integration tests
// ---------------------------------------------------------------------------

func TestProfileRecovery_ActiveVersion_NoActions(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	profiles := memory.NewProfileRepo()
	grants := memory.NewGrantRepo()

	p := &authority.AuthorityProfile{
		ID:            "prof-active",
		Version:       1,
		SurfaceID:     "surf-test",
		Name:          "Active Profile",
		Status:        authority.ProfileStatusActive,
		EffectiveDate: now.Add(-24 * time.Hour),
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now.Add(-24 * time.Hour),
	}
	if err := profiles.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	srv := newFullIntrospectionServer(memory.NewSurfaceRepo(), profiles, grants)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/prof-active/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[ProfileRecoveryResult](t, rec)
	if resp.ProfileID != "prof-active" {
		t.Errorf("expected profile_id 'prof-active', got %q", resp.ProfileID)
	}
	if resp.SurfaceID != "surf-test" {
		t.Errorf("expected surface_id 'surf-test', got %q", resp.SurfaceID)
	}
	if resp.ActiveVersion == nil {
		t.Fatal("expected non-nil active_version")
	}
	if *resp.ActiveVersion != 1 {
		t.Errorf("expected active_version 1, got %d", *resp.ActiveVersion)
	}
	if resp.ActiveGrantCount != 0 {
		t.Errorf("expected active_grant_count 0, got %d", resp.ActiveGrantCount)
	}
	// No actions when there are no active grants and the profile is healthy.
	if len(resp.RecommendedNextActions) != 0 {
		t.Errorf("expected no actions, got %v", resp.RecommendedNextActions)
	}
	if resp.CapabilityNote == "" {
		t.Error("expected non-empty capability_note")
	}
}

func TestProfileRecovery_FutureEffectiveDate_NoActiveVersion(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	profiles := memory.NewProfileRepo()

	// Create a profile with a future effective_date — it exists but is not yet effective.
	p := &authority.AuthorityProfile{
		ID:            "prof-future",
		Version:       1,
		SurfaceID:     "surf-test",
		Name:          "Future Profile",
		Status:        authority.ProfileStatusActive, // status active but effective_date in future
		EffectiveDate: now.Add(24 * time.Hour),       // not yet effective
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := profiles.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	srv := newFullIntrospectionServer(memory.NewSurfaceRepo(), profiles, memory.NewGrantRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/prof-future/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[ProfileRecoveryResult](t, rec)
	if resp.ActiveVersion != nil {
		t.Errorf("expected nil active_version, got %d", *resp.ActiveVersion)
	}

	// Should warn about no current effective version.
	if len(resp.Warnings) == 0 {
		t.Error("expected at least one warning about no effective version")
	}

	// Should recommend re-applying with past effective_date.
	wantAction := "re-apply profile with an effective_date in the past to restore evaluation eligibility"
	found := false
	for _, a := range resp.RecommendedNextActions {
		if a == wantAction {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected action %q in %v", wantAction, resp.RecommendedNextActions)
	}
}

func TestProfileRecovery_MultiVersion_LatestActive(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	profiles := memory.NewProfileRepo()

	v1 := &authority.AuthorityProfile{
		ID:            "prof-multi",
		Version:       1,
		SurfaceID:     "surf-test",
		Name:          "Multi Profile v1",
		Status:        authority.ProfileStatusActive,
		EffectiveDate: now.Add(-48 * time.Hour),
		CreatedAt:     now.Add(-48 * time.Hour),
		UpdatedAt:     now.Add(-48 * time.Hour),
	}
	v2 := &authority.AuthorityProfile{
		ID:            "prof-multi",
		Version:       2,
		SurfaceID:     "surf-test",
		Name:          "Multi Profile v2",
		Status:        authority.ProfileStatusActive,
		EffectiveDate: now.Add(-24 * time.Hour),
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now.Add(-24 * time.Hour),
	}
	if err := profiles.Create(ctx, v1); err != nil {
		t.Fatal(err)
	}
	if err := profiles.Create(ctx, v2); err != nil {
		t.Fatal(err)
	}

	srv := newFullIntrospectionServer(memory.NewSurfaceRepo(), profiles, memory.NewGrantRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/prof-multi/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[ProfileRecoveryResult](t, rec)
	if resp.VersionCount != 2 {
		t.Errorf("expected version_count 2, got %d", resp.VersionCount)
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
	if len(resp.Versions) != 2 {
		t.Errorf("expected 2 version entries, got %d", len(resp.Versions))
	}
}

// ---------------------------------------------------------------------------
// Full HTTP stack integration test
// ---------------------------------------------------------------------------

func TestSurfaceRecoveryHTTP_ActiveV1_ReviewV2(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	surfaces := memory.NewSurfaceRepo()

	v1 := &surface.DecisionSurface{
		ID:             "surf-http-test",
		Version:        1,
		Status:         surface.SurfaceStatusActive,
		Name:           "HTTP Test Surface",
		Domain:         "test",
		ProcessID:      "proc-test",
		EffectiveFrom:  now.Add(-72 * time.Hour),
		BusinessOwner:  "ops",
		TechnicalOwner: "platform",
		CreatedAt:      now.Add(-72 * time.Hour),
		UpdatedAt:      now.Add(-72 * time.Hour),
	}
	v2 := &surface.DecisionSurface{
		ID:             "surf-http-test",
		Version:        2,
		Status:         surface.SurfaceStatusReview,
		Name:           "HTTP Test Surface v2",
		Domain:         "test",
		ProcessID:      "proc-test",
		EffectiveFrom:  now.Add(-time.Hour),
		BusinessOwner:  "ops",
		TechnicalOwner: "platform",
		CreatedAt:      now.Add(-time.Hour),
		UpdatedAt:      now.Add(-time.Hour),
	}
	if err := surfaces.Create(ctx, v1); err != nil {
		t.Fatal(err)
	}
	if err := surfaces.Create(ctx, v2); err != nil {
		t.Fatal(err)
	}

	svc := NewIntrospectionService(surfaces, memory.NewProfileRepo())
	srv := NewServerWithAllServices(&mockOrchestrator{}, nil, nil, svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/surf-http-test/recovery", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[SurfaceRecoveryResult](t, rec)

	if resp.SurfaceID != "surf-http-test" {
		t.Errorf("expected surface_id 'surf-http-test', got %q", resp.SurfaceID)
	}
	if resp.VersionCount != 2 {
		t.Errorf("expected version_count 2, got %d", resp.VersionCount)
	}
	if resp.LatestVersion != 2 {
		t.Errorf("expected latest_version 2, got %d", resp.LatestVersion)
	}
	if resp.LatestStatus != "review" {
		t.Errorf("expected latest_status 'review', got %q", resp.LatestStatus)
	}
	if resp.ActiveVersion == nil {
		t.Fatal("expected non-nil active_version")
	}
	if *resp.ActiveVersion != 1 {
		t.Errorf("expected active_version 1, got %d", *resp.ActiveVersion)
	}
}
