package governancecoverage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/value"
)

// helperActiveExpectation is the service-test analogue of
// makeActiveExpectation in matcher_test.go. Kept locally so the two
// test files do not implicitly couple.
func helperActiveExpectation(id string, version int, scopeID string, payload string) *governanceexpectation.GovernanceExpectation {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &governanceexpectation.GovernanceExpectation{
		ID:                id,
		Version:           version,
		ScopeKind:         governanceexpectation.ScopeKindProcess,
		ScopeID:           scopeID,
		RequiredSurfaceID: "surf-" + id,
		Name:              id,
		Status:            governanceexpectation.ExpectationStatusActive,
		EffectiveDate:     now.Add(-time.Hour),
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  json.RawMessage(payload),
		BusinessOwner:     "biz",
		TechnicalOwner:    "tech",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func TestService_MatchesFor_QueriesByProcessScope(t *testing.T) {
	repo := memory.NewGovernanceExpectationRepo()
	ctx := context.Background()

	// Three actives under proc-1, two of which match the runtime input.
	if err := repo.Create(ctx, helperActiveExpectation("ge-match-1", 1, "proc-1",
		`{"min_confidence": 0.5}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := repo.Create(ctx, helperActiveExpectation("ge-match-2", 1, "proc-1",
		`{"consequence_amount_at_least": 1000}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := repo.Create(ctx, helperActiveExpectation("ge-no-match", 1, "proc-1",
		`{"min_confidence": 0.99}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// One under a different process, must not be queried.
	if err := repo.Create(ctx, helperActiveExpectation("ge-other-proc", 1, "proc-OTHER",
		`{}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := NewService(repo)
	got, err := svc.MatchesFor(ctx, Input{
		ProcessID:  "proc-1",
		Confidence: 0.9,
		Consequence: &eval.Consequence{
			Type:   value.ConsequenceTypeMonetary,
			Amount: 5000,
		},
		ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("MatchesFor: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 matches, got %d (%+v)", len(got), got)
	}
	// Lex order on ID.
	if got[0].ExpectationID != "ge-match-1" || got[1].ExpectationID != "ge-match-2" {
		t.Errorf("want [ge-match-1, ge-match-2], got %v", []string{got[0].ExpectationID, got[1].ExpectationID})
	}
}

func TestService_MatchesFor_ReturnsLatestActiveVersion(t *testing.T) {
	repo := memory.NewGovernanceExpectationRepo()
	ctx := context.Background()

	for _, v := range []int{1, 2, 3} {
		if err := repo.Create(ctx, helperActiveExpectation("ge-multi", v, "proc-1", `{}`)); err != nil {
			t.Fatalf("seed v%d: %v", v, err)
		}
	}

	svc := NewService(repo)
	got, err := svc.MatchesFor(ctx, Input{
		ProcessID:  "proc-1",
		ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("MatchesFor: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 match (latest only), got %d", len(got))
	}
	if got[0].Version != 3 {
		t.Errorf("want Version 3, got %d", got[0].Version)
	}
}

func TestService_MatchesFor_EmptyProcessID_NoQuery(t *testing.T) {
	// Empty ProcessID must short-circuit before the repo is touched.
	repo := failingRepo{}
	svc := NewService(repo)
	got, err := svc.MatchesFor(context.Background(), Input{ProcessID: ""})
	if err != nil {
		t.Fatalf("want no error for empty ProcessID, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 matches, got %d", len(got))
	}
}

func TestService_MatchesFor_RepoError_PropagatesUnchanged(t *testing.T) {
	want := errors.New("simulated repo failure")
	repo := failingRepo{err: want}
	svc := NewService(repo)
	_, err := svc.MatchesFor(context.Background(), Input{
		ProcessID:  "proc-1",
		ObservedAt: time.Now().UTC(),
	})
	if !errors.Is(err, want) {
		t.Errorf("want repo error to propagate; got %v", err)
	}
}

// failingRepo is a no-op governanceexpectation.Repository that returns
// the configured err from ListActiveByScope. All other methods are
// no-ops; the service does not call them.
type failingRepo struct{ err error }

func (failingRepo) Create(_ context.Context, _ *governanceexpectation.GovernanceExpectation) error {
	return nil
}
func (failingRepo) FindByID(_ context.Context, _ string) (*governanceexpectation.GovernanceExpectation, error) {
	return nil, nil
}
func (failingRepo) FindByIDAndVersion(_ context.Context, _ string, _ int) (*governanceexpectation.GovernanceExpectation, error) {
	return nil, nil
}
func (failingRepo) ListVersions(_ context.Context, _ string) ([]*governanceexpectation.GovernanceExpectation, error) {
	return nil, nil
}
func (failingRepo) Update(_ context.Context, _ *governanceexpectation.GovernanceExpectation) error {
	return nil
}
func (r failingRepo) ListActiveByScope(_ context.Context, _ governanceexpectation.ScopeKind, _ string, _ time.Time) ([]*governanceexpectation.GovernanceExpectation, error) {
	return nil, r.err
}

// Compile-time check that failingRepo satisfies governanceexpectation.Repository.
var _ governanceexpectation.Repository = failingRepo{}
