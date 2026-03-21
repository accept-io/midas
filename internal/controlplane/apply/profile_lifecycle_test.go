package apply

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// stubProfileRepo is a minimal in-memory implementation for profile apply tests.
type stubProfileRepo struct {
	profiles map[string]*authority.AuthorityProfile
}

func newStubProfileRepo() *stubProfileRepo {
	return &stubProfileRepo{profiles: make(map[string]*authority.AuthorityProfile)}
}

func (r *stubProfileRepo) FindByID(_ context.Context, id string) (*authority.AuthorityProfile, error) {
	return r.profiles[id], nil
}

func (r *stubProfileRepo) Create(_ context.Context, p *authority.AuthorityProfile) error {
	r.profiles[p.ID] = p
	return nil
}

func (r *stubProfileRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*authority.AuthorityProfile, error) {
	p := r.profiles[id]
	if p != nil && p.Version == version {
		return p, nil
	}
	return nil, nil
}

func (r *stubProfileRepo) FindActiveAt(_ context.Context, id string, at time.Time) (*authority.AuthorityProfile, error) {
	p := r.profiles[id]
	if p == nil || p.Status != authority.ProfileStatusActive {
		return nil, nil
	}
	if p.EffectiveDate.After(at) {
		return nil, nil
	}
	return p, nil
}

func (r *stubProfileRepo) ListBySurface(_ context.Context, _ string) ([]*authority.AuthorityProfile, error) {
	return nil, nil
}

func (r *stubProfileRepo) ListVersions(_ context.Context, _ string) ([]*authority.AuthorityProfile, error) {
	return nil, nil
}

func (r *stubProfileRepo) Update(_ context.Context, p *authority.AuthorityProfile) error {
	r.profiles[p.ID] = p
	return nil
}

func TestApplyProfile_SetsStatusToReview(t *testing.T) {
	profileRepo := newStubProfileRepo()
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindProfile,
			ID:   "payments-tier-1",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata: types.DocumentMetadata{
					ID:   "payments-tier-1",
					Name: "Payments Tier 1",
				},
				Spec: types.ProfileSpec{
					SurfaceID: "payment.execute",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.85,
						ConsequenceThreshold: types.ConsequenceThreshold{
							Type:       "monetary",
							Amount:     10000,
							Currency:   "USD",
						},
					},
					Policy: types.ProfilePolicy{
						Reference: "rego://payments/auto_approve_v1",
						FailMode:  "closed",
					},
				},
			},
		},
	}

	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d", result.CreatedCount())
	}

	// Verify the profile was persisted with status=review
	p := profileRepo.profiles["payments-tier-1"]
	if p == nil {
		t.Fatal("profile was not persisted")
	}
	if p.Status != authority.ProfileStatusReview {
		t.Errorf("status: got %q, want %q", p.Status, authority.ProfileStatusReview)
	}
	if p.Version != 1 {
		t.Errorf("version: got %d, want 1", p.Version)
	}
}

func TestApplyProfile_IncrementsVersion(t *testing.T) {
	profileRepo := newStubProfileRepo()

	// Seed an existing profile
	profileRepo.profiles["payments-tier-1"] = &authority.AuthorityProfile{
		ID:      "payments-tier-1",
		Version: 1,
		Status:  authority.ProfileStatusActive,
	}

	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindProfile,
			ID:   "payments-tier-1",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata: types.DocumentMetadata{
					ID:   "payments-tier-1",
					Name: "Payments Tier 1 Updated",
				},
				Spec: types.ProfileSpec{
					SurfaceID: "payment.execute",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.90,
						ConsequenceThreshold: types.ConsequenceThreshold{
							Type:       "monetary",
							Amount:     20000,
							Currency:   "USD",
						},
					},
					Policy: types.ProfilePolicy{
						Reference: "rego://payments/auto_approve_v2",
						FailMode:  "closed",
					},
				},
			},
		},
	}

	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}

	p := profileRepo.profiles["payments-tier-1"]
	if p == nil {
		t.Fatal("profile was not persisted")
	}
	if p.Version != 2 {
		t.Errorf("version: got %d, want 2", p.Version)
	}
	if p.Status != authority.ProfileStatusReview {
		t.Errorf("status: got %q, want %q", p.Status, authority.ProfileStatusReview)
	}
}

func TestApplyProfile_WithoutRepo_FallsBackToCreated(t *testing.T) {
	// When no profile repo is configured, Apply should still mark the resource as created
	// (validation-only mode for profiles)
	svc := NewService()

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindProfile,
			ID:   "payments-tier-1",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata: types.DocumentMetadata{
					ID:   "payments-tier-1",
					Name: "Payments Tier 1",
				},
				Spec: types.ProfileSpec{
					SurfaceID: "payment.execute",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.85,
						ConsequenceThreshold: types.ConsequenceThreshold{
							Type:       "monetary",
							Amount:     10000,
							Currency:   "USD",
						},
					},
					Policy: types.ProfilePolicy{
						Reference: "rego://payments/auto_approve_v1",
						FailMode:  "closed",
					},
				},
			},
		},
	}

	result := svc.Apply(context.Background(), docs, "")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d", result.CreatedCount())
	}
}
