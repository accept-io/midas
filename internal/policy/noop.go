package policy

import "context"

// NoOpPolicyEvaluator is the default policy evaluator used when
// no real policy engine is configured yet.
type NoOpPolicyEvaluator struct{}

// Evaluate always allows the request. No policy logic is applied.
// Callers can detect noop mode via PolicyMode() rather than by checking the result reason.
func (n NoOpPolicyEvaluator) Evaluate(ctx context.Context, input PolicyInput) (PolicyResult, error) {
	_ = ctx
	_ = input

	return PolicyResult{
		Allowed: true,
		Reason:  "no policy configured",
	}, nil
}

// PolicyMode implements PolicyModer. It returns PolicyModeNoop so callers
// can detect and surface the active policy mode without importing this package.
func (NoOpPolicyEvaluator) PolicyMode() string { return PolicyModeNoop }
