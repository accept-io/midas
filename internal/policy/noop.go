package policy

import "context"

// NoOpPolicyEvaluator is the default policy evaluator used when
// no real policy engine is configured yet.
type NoOpPolicyEvaluator struct{}

// Evaluate always allows the request.
// This is a temporary implementation for early development.
func (n NoOpPolicyEvaluator) Evaluate(ctx context.Context, input PolicyInput) (PolicyResult, error) {
	_ = ctx
	_ = input

	return PolicyResult{
		Allowed: true,
		Reason:  "no policy configured",
	}, nil
}
