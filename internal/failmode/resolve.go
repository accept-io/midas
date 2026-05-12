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

// ---------------------------------------------------------------------------
// D29c-1 — Resolution path model
//
// ResolveWithPath returns, alongside the effective policy, a fixed-length-3
// path describing the hierarchy walk:
//
//   path[0] = surface           level
//   path[1] = business_service  level
//   path[2] = deployment_default level
//
// Every level emits exactly one path entry on every resolution call,
// regardless of whether the level was consulted. The path describes:
//
//   - which levels were consulted (status != skipped),
//   - which references existed (Status=resolved/failed),
//   - which references were empty (Status=not_configured),
//   - which policy IDs were attempted (ReferenceID),
//   - which lookups succeeded (Status=resolved + PolicyID + Version),
//   - which lookups failed (Status=failed + ReferenceID),
//   - which level won (Status=resolved, with lower-priority levels skipped),
//   - and why resolution did or did not produce an effective policy
//     (Reason drawn from the fixed vocabulary below).
//
// The path is attached to EvaluationResult as a Go-only test observable;
// it is never serialised on the /v1/evaluate response, never included in
// audit-event payloads, and never consulted to compute outcomes in this
// tranche.
// ---------------------------------------------------------------------------

// ResolutionLevel enumerates the three hierarchy levels in their fixed
// resolution order. The path slice is indexed by this order:
// surface (0), business_service (1), deployment_default (2).
type ResolutionLevel string

const (
	ResolutionLevelSurface           ResolutionLevel = "surface"
	ResolutionLevelBusinessService   ResolutionLevel = "business_service"
	ResolutionLevelDeploymentDefault ResolutionLevel = "deployment_default"
)

// ResolutionPathStatus enumerates the per-level outcome of a resolution
// walk. Every level reports exactly one of these statuses on every call.
//
//   - not_configured: the level had no reference (empty string).
//   - resolved:       the level had a reference and FindActiveAt returned
//                     an active policy.
//   - failed:         the level had a reference and FindActiveAt returned
//                     an error or nil (no active policy at evaluation time).
//   - skipped:        the level was not consulted because an earlier
//                     level resolved or failed. Skipped entries still
//                     appear in the path so its length is always 3.
type ResolutionPathStatus string

const (
	ResolutionPathStatusNotConfigured ResolutionPathStatus = "not_configured"
	ResolutionPathStatusResolved      ResolutionPathStatus = "resolved"
	ResolutionPathStatusFailed        ResolutionPathStatus = "failed"
	ResolutionPathStatusSkipped       ResolutionPathStatus = "skipped"
)

// ResolutionPathEntry describes a single level of the resolution walk.
//
// Fields:
//
//   - Level:       which hierarchy level this entry represents.
//   - Source:      the corresponding ResolutionSource constant (empty
//                  for skipped / not_configured entries that did not
//                  produce a winning policy).
//   - ReferenceID: the FailModePolicy ID that the level held when its
//                  reference was non-empty. Empty otherwise.
//   - Status:      one of the four ResolutionPathStatus values.
//   - PolicyID:    populated only when Status=resolved.
//   - Version:     populated only when Status=resolved.
//   - Reason:      a stable, non-sensitive reason drawn from the
//                  Resolution Reason* constants below. Never carries
//                  raw SQL errors, stack traces, DSNs, or wrapped
//                  internal error text.
type ResolutionPathEntry struct {
	Level       ResolutionLevel
	Source      ResolutionSource
	ReferenceID string
	Status      ResolutionPathStatus
	PolicyID    string
	Version     int
	Reason      string
}

// ResolutionResult is the structured return value of ResolveWithPath. It
// carries the effective policy (when one resolved), the fixed-length-3
// path describing the hierarchy walk, and — when resolution succeeds —
// the full domain entity so callers can inspect rules without a second
// repository lookup.
//
// Effective is the empty EffectiveFailModePolicy{Source: ResolutionSourceNone}
// when no policy was configured at any level. Path is always populated
// with exactly three entries (one per hierarchy level) when ResolveWithPath
// is invoked, regardless of whether an effective policy was found or an
// error occurred.
//
// Policy is the full *FailModePolicy (with its Rules slice) that
// produced Effective. It is set ONLY when Effective.Source is one of
// surface / business_service / deployment_default — i.e. when resolution
// succeeded. It is nil when no policy resolved or when resolution
// errored. Callers must not mutate the pointee; the value is shared
// with the underlying repository's in-memory representation.
//
// Added in D29c-2 so the orchestrator can select the per-correctness-
// class rule for trigger-event payloads without invoking FindActiveAt
// a second time.
type ResolutionResult struct {
	Effective EffectiveFailModePolicy
	Path      []ResolutionPathEntry
	Policy    *FailModePolicy
}

// TriggerCondition enumerates the failure conditions that may produce
// a FAIL_MODE_POLICY_TRIGGER_FIRED audit event while a FailModePolicy
// has resolved for the evaluation.
//
// The type lives in this package so the FailModePolicy runtime
// vocabulary stays centralised. The authoritative trigger-to-
// correctness-class mapping lives alongside the constants in
// triggers.go; runtime code resolves the class through
// CorrectnessClassForTrigger rather than reaching for individual
// CorrectnessClass* constants at each call site.
type TriggerCondition string

const (
	// FailModePolicyTriggerPolicyEvaluatorError records that the policy
	// evaluator returned a non-nil error during evaluation. Mapped to
	// CorrectnessClassResource via the trigger taxonomy in triggers.go.
	FailModePolicyTriggerPolicyEvaluatorError TriggerCondition = "policy_evaluator_error"

	// FailModePolicyTriggerAuthorityResolutionFailure (D29j) records that
	// authority chain resolution produced a deterministic Reject outcome —
	// NO_ACTIVE_GRANT, PROFILE_NOT_FOUND, or GRANT_PROFILE_SURFACE_MISMATCH —
	// while a FailModePolicy had resolved for the evaluation. Mapped to
	// CorrectnessClassResource via the trigger taxonomy in triggers.go;
	// the deterministic-outcome authority path is a distinct
	// reason-code surface from the orchestrator's failure_class table
	// (which only fires on the `err != nil` authority path).
	FailModePolicyTriggerAuthorityResolutionFailure TriggerCondition = "authority_resolution_failure"
)

// SelectRuleForClass returns the FailModePolicyRule for the given
// correctness class on the supplied policy, with D29b axis defaults
// applied. The second return value is true when a rule was found,
// false otherwise.
//
// FailModePolicy validation requires exhaustive correctness-class
// coverage, so under healthy data a not-found result is unreachable.
// The helper is defensive — it returns (zero rule, false) rather than
// panicking — so the orchestrator can record a stable `rule_status =
// "not_found"` evidence marker and continue.
//
// nil policy yields (zero rule, false). The helper does not mutate
// the supplied policy.
func SelectRuleForClass(p *FailModePolicy, class CorrectnessClass) (FailModePolicyRule, bool) {
	if p == nil {
		return FailModePolicyRule{}, false
	}
	for _, r := range p.Rules {
		if r.CorrectnessClass == class {
			return ApplyRuleAxisDefaults(r), true
		}
	}
	return FailModePolicyRule{}, false
}

// Resolution reason strings — the fixed vocabulary that path-entry
// Reason fields draw from. Detailed error context is still returned in
// the error return value and logged via the orchestrator's slog.Warn;
// the path's Reason field is bounded to this vocabulary so it never
// carries sensitive content (DSNs, stack traces, wrapped error text).
const (
	ResolutionReasonSurfaceNotConfigured      = "surface has no fail_mode_policy_id"
	ResolutionReasonSurfaceResolved           = "surface override resolved"
	ResolutionReasonSurfaceLookupFailed       = "surface fail_mode_policy_id could not be resolved"
	ResolutionReasonBSNotConfigured           = "business service has no fail_mode_policy_id"
	ResolutionReasonBSResolved                = "business service default resolved"
	ResolutionReasonBSLookupFailed            = "business service fail_mode_policy_id could not be resolved"
	ResolutionReasonDeploymentDefaultEmpty    = "deployment default policy id is empty"
	ResolutionReasonDeploymentDefaultResolved = "deployment default resolved"
	ResolutionReasonDeploymentDefaultFailed   = "deployment default policy id could not be resolved"
	ResolutionReasonEarlierLevelFailed        = "earlier level failed"
	ResolutionReasonHigherPriorityResolved    = "higher-priority level resolved"
)

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
	// D29c-1 compatibility wrapper. The implementation is in
	// ResolveWithPath; this wrapper discards the path so existing
	// callers keep their two-return signature.
	result, err := ResolveWithPath(ctx, repo, sur, bs, evaluationTime, deploymentDefaultPolicyID)
	return result.Effective, err
}

// ResolveWithPath determines the effective FailModePolicy for an
// evaluation context using the same precedence as Resolve, and
// additionally returns a fixed-length-3 path describing the hierarchy
// walk (surface, business_service, deployment_default in order).
//
// The path is populated on every code path:
//
//   - On successful resolution at any level: the path records the
//     resolved level with Status=resolved and lower-priority levels as
//     Status=skipped with ResolutionReasonHigherPriorityResolved.
//   - On explicit-reference resolution failure: the path records the
//     failing level with Status=failed and lower-priority levels as
//     Status=skipped with ResolutionReasonEarlierLevelFailed. The error
//     is also returned and resolution does NOT fall through (matches
//     pre-D29c-1 Resolve semantics).
//   - On no-policy-at-any-level: the path records each level as
//     Status=not_configured; Effective is the empty value and error is
//     nil.
//
// On invalid inputs (nil repo, zero evaluation time) the function
// returns a zero ResolutionResult (nil Path) and the wrapped error,
// matching pre-D29c-1 Resolve semantics. The orchestrator treats those
// inputs as programmer errors; no path is recorded.
func ResolveWithPath(
	ctx context.Context,
	repo activeFailModePolicyFinder,
	sur *surface.DecisionSurface,
	bs *businessservice.BusinessService,
	evaluationTime time.Time,
	deploymentDefaultPolicyID string,
) (ResolutionResult, error) {
	if repo == nil {
		return ResolutionResult{}, fmt.Errorf("%w: repository is nil", ErrFailModePolicyResolutionFailed)
	}
	if evaluationTime.IsZero() {
		return ResolutionResult{}, fmt.Errorf("%w: evaluation time is zero", ErrFailModePolicyResolutionFailed)
	}

	path := make([]ResolutionPathEntry, 3)
	path[0] = ResolutionPathEntry{Level: ResolutionLevelSurface}
	path[1] = ResolutionPathEntry{Level: ResolutionLevelBusinessService}
	path[2] = ResolutionPathEntry{Level: ResolutionLevelDeploymentDefault}

	// -----------------------------------------------------------------
	// Surface level
	// -----------------------------------------------------------------
	if sur == nil || sur.FailModePolicyID == "" {
		path[0].Status = ResolutionPathStatusNotConfigured
		path[0].Reason = ResolutionReasonSurfaceNotConfigured
	} else {
		path[0].ReferenceID = sur.FailModePolicyID
		policy, err := repo.FindActiveAt(ctx, sur.FailModePolicyID, evaluationTime)
		if err != nil {
			path[0].Status = ResolutionPathStatusFailed
			path[0].Reason = ResolutionReasonSurfaceLookupFailed
			markRemainingAsEarlierLevelFailed(path[1:])
			return ResolutionResult{Path: path}, fmt.Errorf("%w: surface override %q: %v",
				ErrFailModePolicyResolutionFailed, sur.FailModePolicyID, err)
		}
		if policy == nil {
			path[0].Status = ResolutionPathStatusFailed
			path[0].Reason = ResolutionReasonSurfaceLookupFailed
			markRemainingAsEarlierLevelFailed(path[1:])
			return ResolutionResult{Path: path}, fmt.Errorf("%w: surface override %q is not active at %v",
				ErrFailModePolicyResolutionFailed, sur.FailModePolicyID, evaluationTime)
		}
		path[0].Status = ResolutionPathStatusResolved
		path[0].Source = ResolutionSourceSurface
		path[0].PolicyID = policy.ID
		path[0].Version = policy.Version
		path[0].Reason = ResolutionReasonSurfaceResolved
		markRemainingAsHigherPriorityResolved(path[1:])
		return ResolutionResult{
			Effective: EffectiveFailModePolicy{
				PolicyID: policy.ID,
				Version:  policy.Version,
				Source:   ResolutionSourceSurface,
			},
			Path:   path,
			Policy: policy,
		}, nil
	}

	// -----------------------------------------------------------------
	// BusinessService level
	// -----------------------------------------------------------------
	if bs == nil || bs.FailModePolicyID == "" {
		path[1].Status = ResolutionPathStatusNotConfigured
		path[1].Reason = ResolutionReasonBSNotConfigured
	} else {
		path[1].ReferenceID = bs.FailModePolicyID
		policy, err := repo.FindActiveAt(ctx, bs.FailModePolicyID, evaluationTime)
		if err != nil {
			path[1].Status = ResolutionPathStatusFailed
			path[1].Reason = ResolutionReasonBSLookupFailed
			markRemainingAsEarlierLevelFailed(path[2:])
			return ResolutionResult{Path: path}, fmt.Errorf("%w: business service default %q: %v",
				ErrFailModePolicyResolutionFailed, bs.FailModePolicyID, err)
		}
		if policy == nil {
			path[1].Status = ResolutionPathStatusFailed
			path[1].Reason = ResolutionReasonBSLookupFailed
			markRemainingAsEarlierLevelFailed(path[2:])
			return ResolutionResult{Path: path}, fmt.Errorf("%w: business service default %q is not active at %v",
				ErrFailModePolicyResolutionFailed, bs.FailModePolicyID, evaluationTime)
		}
		path[1].Status = ResolutionPathStatusResolved
		path[1].Source = ResolutionSourceBusinessService
		path[1].PolicyID = policy.ID
		path[1].Version = policy.Version
		path[1].Reason = ResolutionReasonBSResolved
		markRemainingAsHigherPriorityResolved(path[2:])
		return ResolutionResult{
			Effective: EffectiveFailModePolicy{
				PolicyID: policy.ID,
				Version:  policy.Version,
				Source:   ResolutionSourceBusinessService,
			},
			Path:   path,
			Policy: policy,
		}, nil
	}

	// -----------------------------------------------------------------
	// Deployment default level
	// -----------------------------------------------------------------
	if deploymentDefaultPolicyID == "" {
		path[2].Status = ResolutionPathStatusNotConfigured
		path[2].Reason = ResolutionReasonDeploymentDefaultEmpty
		return ResolutionResult{
			Effective: EffectiveFailModePolicy{Source: ResolutionSourceNone},
			Path:      path,
		}, nil
	}
	path[2].ReferenceID = deploymentDefaultPolicyID
	policy, err := repo.FindActiveAt(ctx, deploymentDefaultPolicyID, evaluationTime)
	if err != nil {
		path[2].Status = ResolutionPathStatusFailed
		path[2].Reason = ResolutionReasonDeploymentDefaultFailed
		return ResolutionResult{Path: path}, fmt.Errorf("%w: deployment default %q: %v",
			ErrFailModePolicyResolutionFailed, deploymentDefaultPolicyID, err)
	}
	if policy == nil {
		path[2].Status = ResolutionPathStatusFailed
		path[2].Reason = ResolutionReasonDeploymentDefaultFailed
		return ResolutionResult{Path: path}, fmt.Errorf("%w: deployment default %q is not active at %v",
			ErrFailModePolicyResolutionFailed, deploymentDefaultPolicyID, evaluationTime)
	}
	path[2].Status = ResolutionPathStatusResolved
	path[2].Source = ResolutionSourceDeploymentDefault
	path[2].PolicyID = policy.ID
	path[2].Version = policy.Version
	path[2].Reason = ResolutionReasonDeploymentDefaultResolved
	return ResolutionResult{
		Effective: EffectiveFailModePolicy{
			PolicyID: policy.ID,
			Version:  policy.Version,
			Source:   ResolutionSourceDeploymentDefault,
		},
		Path:   path,
		Policy: policy,
	}, nil
}

// markRemainingAsEarlierLevelFailed sets every entry in the supplied
// slice to Status=skipped with Reason=ResolutionReasonEarlierLevelFailed.
// Used when a higher-priority level returned an explicit-reference
// failure so the lower-priority levels are not consulted.
func markRemainingAsEarlierLevelFailed(entries []ResolutionPathEntry) {
	for i := range entries {
		entries[i].Status = ResolutionPathStatusSkipped
		entries[i].Reason = ResolutionReasonEarlierLevelFailed
	}
}

// markRemainingAsHigherPriorityResolved sets every entry in the supplied
// slice to Status=skipped with Reason=ResolutionReasonHigherPriorityResolved.
// Used when a higher-priority level resolved successfully so the
// lower-priority levels are not consulted.
func markRemainingAsHigherPriorityResolved(entries []ResolutionPathEntry) {
	for i := range entries {
		entries[i].Status = ResolutionPathStatusSkipped
		entries[i].Reason = ResolutionReasonHigherPriorityResolved
	}
}
