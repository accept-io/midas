package policy

import "context"

// PolicyInput is the structured input passed from the orchestrator to the policy layer.
type PolicyInput struct {
	SurfaceID string
	AgentID   string
	Context   map[string]any
}

// PolicyResult is the result returned by a policy evaluation.
type PolicyResult struct {
	Allowed bool
	Reason  string
}

// PolicyEvaluator defines the policy evaluation boundary.
// Implementations may be no-op, embedded OPA, or enterprise extensions.
type PolicyEvaluator interface {
	Evaluate(ctx context.Context, input PolicyInput) (PolicyResult, error)
}
