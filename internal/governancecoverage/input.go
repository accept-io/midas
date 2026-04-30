package governancecoverage

import (
	"time"

	"github.com/accept-io/midas/internal/eval"
)

// Input is the runtime context the matcher evaluates expectations
// against. It is caller-populated; the pure matcher does no resolution.
//
// The orchestrator wiring added in #54 will populate Input from the
// evaluation request plus the resolved Surface (for ProcessID); tests
// populate it directly.
//
// Field-by-field semantics:
//
//   - ProcessID is the load-bearing scope key today. Apply (#52) supports
//     only process-scoped expectations and the matcher treats
//     ProcessID as the scope discriminator. An empty ProcessID will
//     match no expectations.
//
//   - SurfaceID, AgentID, RequestSource, RequestID are recorded for
//     future grammar extensions; the current risk_condition grammar
//     does not read them. They are kept on Input so #54+ can extend
//     the grammar without changing the matcher's public surface.
//
//   - Confidence and Consequence are the typed risk-shaped fields the
//     risk_condition grammar reads. Consequence is a pointer because
//     the originating eval.DecisionRequest models it as a pointer; a
//     nil Consequence means "no consequence-shaped facts to match
//     against" and consequence-based constraints will not be satisfied.
//
//   - ObservedAt is the wall-clock time used for the active-at-time
//     window check on every candidate expectation. Caller-authoritative
//     so the orchestrator's injected clock keeps tests deterministic.
type Input struct {
	ProcessID     string
	SurfaceID     string
	AgentID       string
	Confidence    float64
	Consequence   *eval.Consequence
	RequestSource string
	RequestID     string
	ObservedAt    time.Time
}
