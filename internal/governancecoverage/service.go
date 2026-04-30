package governancecoverage

import (
	"context"

	"github.com/accept-io/midas/internal/governanceexpectation"
)

// Service is the thin wiring between the active-at-time scope query
// (governanceexpectation.Repository.ListActiveByScope) and the pure
// Matcher. It exists so the orchestrator wiring added by #54 has a
// single dependency to inject; today only tests exercise this type.
//
// The Service has no state beyond the repo handle and a Matcher, and
// performs no event emission, no caching, and no decision-altering
// logic. Errors come exclusively from the repository — the matcher
// itself never errors.
type Service struct {
	repo    governanceexpectation.Repository
	matcher *Matcher
}

// NewService constructs a Service backed by repo. repo must be non-nil
// in production wiring; tests may pass any compatible implementation.
func NewService(repo governanceexpectation.Repository) *Service {
	return &Service{
		repo:    repo,
		matcher: NewMatcher(),
	}
}

// MatchesFor returns the GovernanceExpectations that match the runtime
// context represented by in. The flow is:
//
//  1. Query the repository for active expectations under
//     (ScopeKindProcess, in.ProcessID) at in.ObservedAt.
//  2. Hand the candidate list to the pure Matcher.
//
// The repository call is the only source of error. The matcher
// guarantees non-empty input always produces a (possibly empty)
// match slice, never an error.
//
// Empty in.ProcessID short-circuits to an empty result with no error —
// there is no scope to query.
func (s *Service) MatchesFor(ctx context.Context, in Input) ([]Match, error) {
	if in.ProcessID == "" {
		return nil, nil
	}
	candidates, err := s.repo.ListActiveByScope(
		ctx,
		governanceexpectation.ScopeKindProcess,
		in.ProcessID,
		in.ObservedAt,
	)
	if err != nil {
		return nil, err
	}
	return s.matcher.Match(in, candidates), nil
}
