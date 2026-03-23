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

// PolicyModer is an optional interface that policy evaluators may implement
// to expose their operating mode. Callers use this for transparency and
// observability without importing concrete evaluator types.
type PolicyModer interface {
	PolicyMode() string
}

// Policy mode constants identify the active evaluation strategy.
// Use these rather than raw strings when branching on policy mode.
const (
	// PolicyModeNoop indicates no real policy engine is configured.
	// All policy checks pass silently. Profiles with a policy_ref will
	// not have that policy enforced.
	PolicyModeNoop = "noop"

	// PolicyModeUnknown is used when the evaluator does not implement
	// PolicyModer and its mode cannot be determined.
	PolicyModeUnknown = "unknown"
)
