package approval_test

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/outbox"
)

// fakeProfileRepo is a minimal in-memory ProfileRepository for approval tests.
type fakeProfileRepo struct {
	profiles map[string]*authority.AuthorityProfile // key: "id:version"
	updated  []*authority.AuthorityProfile
}

func newFakeProfileRepo(profiles ...*authority.AuthorityProfile) *fakeProfileRepo {
	r := &fakeProfileRepo{profiles: make(map[string]*authority.AuthorityProfile)}
	for _, p := range profiles {
		key := profileKey(p.ID, p.Version)
		r.profiles[key] = p
	}
	return r
}

func profileKey(id string, version int) string {
	return id + ":" + string(rune('0'+version))
}

func (f *fakeProfileRepo) FindByIDAndVersion(_ context.Context, id string, version int) (*authority.AuthorityProfile, error) {
	p, ok := f.profiles[profileKey(id, version)]
	if !ok {
		return nil, nil
	}
	copy := *p
	return &copy, nil
}

func (f *fakeProfileRepo) Update(_ context.Context, p *authority.AuthorityProfile) error {
	f.profiles[profileKey(p.ID, p.Version)] = p
	f.updated = append(f.updated, p)
	return nil
}

func makeReviewProfile(id string, version int) *authority.AuthorityProfile {
	return &authority.AuthorityProfile{
		ID:            id,
		Version:       version,
		SurfaceID:     "surf-1",
		Name:          id + "-profile",
		Status:        authority.ProfileStatusReview,
		EffectiveDate: time.Now().UTC().Add(-time.Hour),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

func makeActiveProfile(id string, version int) *authority.AuthorityProfile {
	p := makeReviewProfile(id, version)
	p.Status = authority.ProfileStatusActive
	return p
}

func TestApproveProfile_ReviewToActive_Success(t *testing.T) {
	profile := makeReviewProfile("prof-1", 1)
	repo := newFakeProfileRepo(profile)
	svc := approval.NewServiceWithProfile(nil, repo, approval.Policy{})

	updated, err := svc.ApproveProfile(context.Background(), "prof-1", 1, "operator@example.com")
	if err != nil {
		t.Fatalf("ApproveProfile: unexpected error: %v", err)
	}
	if updated.Status != authority.ProfileStatusActive {
		t.Errorf("expected status active, got %s", updated.Status)
	}
	if updated.ApprovedBy != "operator@example.com" {
		t.Errorf("expected approvedBy operator@example.com, got %s", updated.ApprovedBy)
	}
	if updated.ApprovedAt == nil {
		t.Error("expected ApprovedAt to be set")
	}
}

func TestApproveProfile_CapturesApproverAndTimestamp(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	profile := makeReviewProfile("prof-2", 1)
	repo := newFakeProfileRepo(profile)
	svc := approval.NewServiceWithProfile(nil, repo, approval.Policy{})

	updated, err := svc.ApproveProfile(context.Background(), "prof-2", 1, "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated.ApprovedBy != "alice" {
		t.Errorf("approvedBy: want alice, got %s", updated.ApprovedBy)
	}
	if updated.ApprovedAt == nil || updated.ApprovedAt.Before(before) {
		t.Errorf("approvedAt not set correctly: %v", updated.ApprovedAt)
	}
}

func TestApproveProfile_NotFound_ReturnsError(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := approval.NewServiceWithProfile(nil, repo, approval.Policy{})

	_, err := svc.ApproveProfile(context.Background(), "nonexistent", 1, "operator")
	if err == nil {
		t.Fatal("expected error for not found, got nil")
	}
}

func TestApproveProfile_WrongStatus_NotInReview(t *testing.T) {
	active := makeActiveProfile("prof-3", 1)
	repo := newFakeProfileRepo(active)
	svc := approval.NewServiceWithProfile(nil, repo, approval.Policy{})

	_, err := svc.ApproveProfile(context.Background(), "prof-3", 1, "operator")
	if err == nil {
		t.Fatal("expected error for non-review profile, got nil")
	}
	if err != approval.ErrProfileNotInReview {
		t.Errorf("expected ErrProfileNotInReview, got %v", err)
	}
}

func TestApproveProfile_NilProfileRepo_ReturnsError(t *testing.T) {
	svc := approval.NewServiceWithProfile(nil, nil, approval.Policy{})

	_, err := svc.ApproveProfile(context.Background(), "prof-1", 1, "operator")
	if err == nil {
		t.Fatal("expected error when profileRepo is nil")
	}
}

func TestDeprecateProfile_ActiveToDeprecated_Success(t *testing.T) {
	profile := makeActiveProfile("prof-4", 1)
	repo := newFakeProfileRepo(profile)
	svc := approval.NewServiceWithProfile(nil, repo, approval.Policy{})

	updated, err := svc.DeprecateProfile(context.Background(), "prof-4", 1, "operator@example.com")
	if err != nil {
		t.Fatalf("DeprecateProfile: unexpected error: %v", err)
	}
	if updated.Status != authority.ProfileStatusDeprecated {
		t.Errorf("expected status deprecated, got %s", updated.Status)
	}
}

func TestDeprecateProfile_WrongStatus_NotActive(t *testing.T) {
	review := makeReviewProfile("prof-5", 1)
	repo := newFakeProfileRepo(review)
	svc := approval.NewServiceWithProfile(nil, repo, approval.Policy{})

	_, err := svc.DeprecateProfile(context.Background(), "prof-5", 1, "operator")
	if err == nil {
		t.Fatal("expected error for non-active profile, got nil")
	}
	if err != approval.ErrProfileNotActive {
		t.Errorf("expected ErrProfileNotActive, got %v", err)
	}
}

func TestDeprecateProfile_NotFound_ReturnsError(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := approval.NewServiceWithProfile(nil, repo, approval.Policy{})

	_, err := svc.DeprecateProfile(context.Background(), "nonexistent", 1, "operator")
	if err == nil {
		t.Fatal("expected error for not found, got nil")
	}
}

// ---------------------------------------------------------------------------
// Outbox emission for profile lifecycle
// ---------------------------------------------------------------------------

// fakeOutboxRepo captures outbox events for assertions.
type fakeOutboxRepo struct {
	appended []*outbox.OutboxEvent
}

func (f *fakeOutboxRepo) Append(_ context.Context, ev *outbox.OutboxEvent) error {
	f.appended = append(f.appended, ev)
	return nil
}

func (f *fakeOutboxRepo) ListUnpublished(_ context.Context) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}

func (f *fakeOutboxRepo) ClaimUnpublished(_ context.Context, _ int) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}

func (f *fakeOutboxRepo) MarkPublished(_ context.Context, _ string) error {
	return nil
}

func TestApproveProfile_EmitsOutboxEvent(t *testing.T) {
	profile := makeReviewProfile("prof-outbox-1", 1)
	repo := newFakeProfileRepo(profile)
	ob := &fakeOutboxRepo{}
	svc := approval.NewServiceWithProfileAndOutbox(nil, repo, approval.Policy{}, ob, nil)

	if _, err := svc.ApproveProfile(context.Background(), "prof-outbox-1", 1, "operator@example.com"); err != nil {
		t.Fatalf("ApproveProfile: unexpected error: %v", err)
	}

	if len(ob.appended) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(ob.appended))
	}
	if ob.appended[0].EventType != outbox.EventProfileApproved {
		t.Errorf("expected event type profile.approved, got %s", ob.appended[0].EventType)
	}
	if ob.appended[0].AggregateType != "profile" {
		t.Errorf("expected aggregate_type profile, got %s", ob.appended[0].AggregateType)
	}
	if ob.appended[0].AggregateID != "prof-outbox-1" {
		t.Errorf("expected aggregate_id prof-outbox-1, got %s", ob.appended[0].AggregateID)
	}
}

func TestDeprecateProfile_EmitsOutboxEvent(t *testing.T) {
	profile := makeActiveProfile("prof-outbox-2", 1)
	repo := newFakeProfileRepo(profile)
	ob := &fakeOutboxRepo{}
	svc := approval.NewServiceWithProfileAndOutbox(nil, repo, approval.Policy{}, ob, nil)

	if _, err := svc.DeprecateProfile(context.Background(), "prof-outbox-2", 1, "operator@example.com"); err != nil {
		t.Fatalf("DeprecateProfile: unexpected error: %v", err)
	}

	if len(ob.appended) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(ob.appended))
	}
	if ob.appended[0].EventType != outbox.EventProfileDeprecated {
		t.Errorf("expected event type profile.deprecated, got %s", ob.appended[0].EventType)
	}
	if ob.appended[0].AggregateType != "profile" {
		t.Errorf("expected aggregate_type profile, got %s", ob.appended[0].AggregateType)
	}
}

func TestApproveProfile_NilOutbox_DoesNotEmitEvent(t *testing.T) {
	profile := makeReviewProfile("prof-nilob-1", 1)
	repo := newFakeProfileRepo(profile)
	// NewServiceWithProfile wires no outbox — must not panic
	svc := approval.NewServiceWithProfile(nil, repo, approval.Policy{})

	if _, err := svc.ApproveProfile(context.Background(), "prof-nilob-1", 1, "operator"); err != nil {
		t.Fatalf("ApproveProfile: unexpected error: %v", err)
	}
}
