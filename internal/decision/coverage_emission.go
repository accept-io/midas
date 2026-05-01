package decision

import (
	"context"
	"log/slog"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/governancecoverage"
)

// emitCoverageEvents queries the wired GovernanceExpectation matching
// service for active expectations under processID and queues one
// GOVERNANCE_CONDITION_DETECTED observational audit event per match.
//
// Behaviour summary, in order:
//
//  1. Coverage service not wired → no events, no error. The orchestrator
//     stays compatible with deployments and tests that have not opted
//     in (the production wiring is `WithCoverageService` from main.go).
//
//  2. processID empty → defensive skip. The matching service already
//     short-circuits empty ProcessID, but checking here avoids a wasted
//     repo call and makes the contract explicit.
//
//  3. Coverage service repository error → log at warn level and return
//     nil. Coverage evidence is observational; a single transient repo
//     failure must not block evaluation. Sustained patterns are an
//     alerting concern (warning-rate threshold), not a per-event error.
//
//  4. Each Match → one acc.recordObservation call carrying the typed
//     payload from buildGovernanceConditionDetectedPayload. The matcher
//     returns matches sorted lexicographically by ExpectationID;
//     preserving that order makes audit sequence numbers deterministic
//     across runs.
//
// The only error this returns is a queue-invariant violation from
// recordObservation (envelope-id mismatch, accumulator already
// persisted, pre-hashed event). That is a programmer error and must
// propagate so it surfaces as a system failure rather than getting
// silently dropped.
func (o *Orchestrator) emitCoverageEvents(
	ctx context.Context,
	acc *evaluationAccumulator,
	env *envelope.Envelope,
	req eval.DecisionRequest,
	processID string,
	now time.Time,
) error {
	if o.coverage == nil {
		return nil
	}
	if processID == "" {
		// Defensive: matcher service already short-circuits this case,
		// but guarding here makes the no-process-no-events contract
		// explicit and avoids a wasted dependency call.
		return nil
	}

	input := governancecoverage.Input{
		ProcessID:     processID,
		SurfaceID:     req.SurfaceID,
		AgentID:       req.AgentID,
		Confidence:    req.Confidence,
		Consequence:   req.Consequence,
		RequestSource: req.RequestSource,
		RequestID:     req.RequestID,
		ObservedAt:    now,
	}

	matches, err := o.coverage.MatchesFor(ctx, input)
	if err != nil {
		// Observational, not decision-blocking. Warn-level so a sustained
		// pattern can be picked up by alerting on warning rates without
		// every transient repo blip paging an operator.
		slog.Warn("coverage_match_failed",
			"request_source", req.RequestSource,
			"request_id", req.RequestID,
			"process_id", processID,
			"error", err,
		)
		return nil
	}

	for _, m := range matches {
		payload := buildGovernanceConditionDetectedPayload(m, req, processID)
		if err := acc.recordObservation(
			env.RequestSource(),
			env.RequestID(),
			audit.AuditEventGovernanceConditionDetected,
			payload,
		); err != nil {
			return err
		}
	}
	return nil
}

// buildGovernanceConditionDetectedPayload constructs the observational
// audit-event payload for a single match. The payload deliberately
// excludes:
//
//   - The expectation's threshold values. They are identifiable from
//     (expectation_id, expectation_version) and embedding them here
//     would duplicate state that #56 may want to render from a single
//     source of truth.
//   - The full request.Context map. It is untyped, unbounded, and
//     would risk leaking sensitive data into the hash-chained audit
//     trail. Defer to a future typed-context contract.
//
// The "summary" sub-object follows the existing CONSEQUENCE_CHECKED
// pattern (see appendConsequenceCheckedEvent): when req.Consequence is
// nil we omit the consequence key entirely so a JSONB-path query like
// `payload_json -> 'summary' ? 'consequence'` is true only when
// consequence facts were actually present at evaluation time.
// Including `consequence: {}` would create false positives for that
// query shape and is the wrong shape for the audit consumer.
func buildGovernanceConditionDetectedPayload(
	m governancecoverage.Match,
	req eval.DecisionRequest,
	processID string,
) map[string]any {
	summary := map[string]any{
		"confidence": req.Confidence,
	}
	if req.Consequence != nil {
		summary["consequence"] = map[string]any{
			"type":        string(req.Consequence.Type),
			"amount":      req.Consequence.Amount,
			"currency":    req.Consequence.Currency,
			"risk_rating": string(req.Consequence.RiskRating),
		}
	}
	return map[string]any{
		"expectation_id":      m.ExpectationID,
		"expectation_version": m.Version,
		"process_id":          processID,
		"required_surface_id": m.RequiredSurfaceID,
		"condition_type":      string(m.ConditionType),
		"summary":             summary,
	}
}
