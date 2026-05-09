package failmode

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/surface"
)

// EffectiveFailModePolicy is the test-observable result of effective-policy
// resolution at evaluation time (D27j-impl-2). It carries enough to
// identify the resolved policy and to attribute the resolution decision,
// but does NOT carry rules — the closed-only invariant prohibits rule
// inspection at runtime in this tranche.
type EffectiveFailModePolicy struct {
	PolicyID string
	Version  int
	Source   ResolutionSource
}

// ResolutionSource enumerates where the effective FailModePolicy was
// resolved from. The empty value indicates "no policy configured at any
// level" — a valid state in -2 because deployment-default config wiring is
// deferred to a later tranche.
type ResolutionSource string

const (
	ResolutionSourceNone              ResolutionSource = ""
	ResolutionSourceSurface           ResolutionSource = "surface"
	ResolutionSourceBusinessService   ResolutionSource = "business_service"
	ResolutionSourceDeploymentDefault ResolutionSource = "deployment_default"
)

// activeFailModePolicyFinder is the bounded read interface the resolver
// requires. Mirrors the failmode.PolicyRepository.FindActiveAt signature
// so the production memory and Postgres repos satisfy it without
// adapter code.
type activeFailModePolicyFinder interface {
	FindActiveAt(ctx context.Context, id string, at time.Time) (*FailModePolicy, error)
}

// ErrFailModePolicyResolutionFailed is returned by Resolve when a
// configured reference (Surface override / BusinessService default /
// deployment default) is non-empty but no active policy at the
// evaluation time covers the reference. The orchestrator currently logs
// this and continues; the apply-time validator is the authoritative gate
// preventing this state from being reachable in normal approved
// configuration.
var ErrFailModePolicyResolutionFailed = errors.New("fail-mode policy resolution failed")

// Resolve determines the effective FailModePolicy for an evaluation
// context using the precedence:
//
//  1. Surface override (surface.FailModePolicyID)
//  2. BusinessService default (businessService.FailModePolicyID)
//  3. Deployment default (deploymentDefaultPolicyID)
//
// Resolution is bounded: at most three FindActiveAt calls. ListVersions is
// never used. FailModePolicy.Rules is never inspected — this resolver
// records observability metadata only and never participates in
// evaluation outcome computation in the closed-only D27j-impl-2 tranche.
//
// Resolver fallback semantics: each level is consulted only if the
// previous level had an empty reference. A non-empty reference that
// cannot be resolved as active does NOT silently fall back to the next
// level — it returns ErrFailModePolicyResolutionFailed wrapped with
// context. This preserves operator intent: a surface that explicitly
// names a policy must use that policy, not a fallback.
//
// The empty result `EffectiveFailModePolicy{Source: ResolutionSourceNone}`
// is valid and is returned when no level configures a policy. The caller
// (orchestrator) treats the empty result as "no fail-mode posture
// recorded for this evaluation."
//
// Inputs:
//   - repo: the active-version finder. Required.
//   - sur: the resolved DecisionSurface. May be nil if surface lookup
//     failed earlier (the caller should not invoke Resolve in that case;
//     defensive handling returns the empty result).
//   - bs: the resolved BusinessService. May be nil similarly.
//   - evaluationTime: point-in-time for FindActiveAt lookups. Must be
//     non-zero.
//   - deploymentDefaultPolicyID: optional. Empty string means "no
//     deployment default configured." D27j-impl-2 always passes empty
//     from the orchestrator; the parameter exists so resolver tests can
//     exercise the path before config wiring lands.
func Resolve(
	ctx context.Context,
	repo activeFailModePolicyFinder,
	sur *surface.DecisionSurface,
	bs *businessservice.BusinessService,
	evaluationTime time.Time,
	deploymentDefaultPolicyID string,
) (EffectiveFailModePolicy, error) {
	if repo == nil {
		return EffectiveFailModePolicy{}, fmt.Errorf("%w: repository is nil", ErrFailModePolicyResolutionFailed)
	}
	if evaluationTime.IsZero() {
		return EffectiveFailModePolicy{}, fmt.Errorf("%w: evaluation time is zero", ErrFailModePolicyResolutionFailed)
	}

	// Surface override.
	if sur != nil && sur.FailModePolicyID != "" {
		policy, err := repo.FindActiveAt(ctx, sur.FailModePolicyID, evaluationTime)
		if err != nil {
			return EffectiveFailModePolicy{}, fmt.Errorf("%w: surface override %q: %v", ErrFailModePolicyResolutionFailed, sur.FailModePolicyID, err)
		}
		if policy == nil {
			return EffectiveFailModePolicy{}, fmt.Errorf("%w: surface override %q is not active at %v", ErrFailModePolicyResolutionFailed, sur.FailModePolicyID, evaluationTime)
		}
		return EffectiveFailModePolicy{
			PolicyID: policy.ID,
			Version:  policy.Version,
			Source:   ResolutionSourceSurface,
		}, nil
	}

	// BusinessService default.
	if bs != nil && bs.FailModePolicyID != "" {
		policy, err := repo.FindActiveAt(ctx, bs.FailModePolicyID, evaluationTime)
		if err != nil {
			return EffectiveFailModePolicy{}, fmt.Errorf("%w: business service default %q: %v", ErrFailModePolicyResolutionFailed, bs.FailModePolicyID, err)
		}
		if policy == nil {
			return EffectiveFailModePolicy{}, fmt.Errorf("%w: business service default %q is not active at %v", ErrFailModePolicyResolutionFailed, bs.FailModePolicyID, evaluationTime)
		}
		return EffectiveFailModePolicy{
			PolicyID: policy.ID,
			Version:  policy.Version,
			Source:   ResolutionSourceBusinessService,
		}, nil
	}

	// Deployment default.
	if deploymentDefaultPolicyID != "" {
		policy, err := repo.FindActiveAt(ctx, deploymentDefaultPolicyID, evaluationTime)
		if err != nil {
			return EffectiveFailModePolicy{}, fmt.Errorf("%w: deployment default %q: %v", ErrFailModePolicyResolutionFailed, deploymentDefaultPolicyID, err)
		}
		if policy == nil {
			return EffectiveFailModePolicy{}, fmt.Errorf("%w: deployment default %q is not active at %v", ErrFailModePolicyResolutionFailed, deploymentDefaultPolicyID, evaluationTime)
		}
		return EffectiveFailModePolicy{
			PolicyID: policy.ID,
			Version:  policy.Version,
			Source:   ResolutionSourceDeploymentDefault,
		}, nil
	}

	// No configuration at any level — empty result, no error.
	return EffectiveFailModePolicy{Source: ResolutionSourceNone}, nil
}
