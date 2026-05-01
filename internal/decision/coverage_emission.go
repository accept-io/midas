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
		// Always queue the detected event for the match.
		if err := acc.recordObservation(
			env.RequestSource(),
			env.RequestID(),
			audit.AuditEventGovernanceConditionDetected,
			buildGovernanceConditionDetectedPayload(m, req, processID),
		); err != nil {
			return err
		}

		// Per #55: when the matched expectation's required surface
		// differs from the surface this evaluation was actually for,
		// queue a GOVERNANCE_COVERAGE_GAP event immediately after the
		// detected event. The interleaved per-match emission keeps
		// each match's facts adjacent in the audit chain, which makes
		// replay analysis simpler than a grouped (all detected, then
		// all gap) shape would.
		//
		// MVP correlation is same-evaluation only: the comparison
		// happens with both observations in scope of the current
		// orchestrator pass. There is no time-window correlation, no
		// background reconciliation, and no bypass detection (a
		// condition appearing on a path that never invokes
		// /v1/evaluate produces no matcher invocation and therefore
		// no gap event — see AuditEventGovernanceCoverageGap doc).
		if m.RequiredSurfaceID != req.SurfaceID {
			if err := acc.recordObservation(
				env.RequestSource(),
				env.RequestID(),
				audit.AuditEventGovernanceCoverageGap,
				buildGovernanceCoverageGapPayload(m, req, processID, env.ID()),
			); err != nil {
				return err
			}
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
// The "summary" sub-object is built by buildCoverageContextSummary so
// that the GOVERNANCE_CONDITION_DETECTED and GOVERNANCE_COVERAGE_GAP
// events for the same match carry byte-identical summary fields. Drift
// between the two events' summary shapes would be a maintenance hazard
// for #56's read model.
func buildGovernanceConditionDetectedPayload(
	m governancecoverage.Match,
	req eval.DecisionRequest,
	processID string,
) map[string]any {
	return map[string]any{
		"expectation_id":      m.ExpectationID,
		"expectation_version": m.Version,
		"process_id":          processID,
		"required_surface_id": m.RequiredSurfaceID,
		"condition_type":      string(m.ConditionType),
		"summary":             buildCoverageContextSummary(req),
	}
}

// buildGovernanceCoverageGapPayload constructs the audit-event payload
// for a single coverage gap (#55). A gap is recorded when an active
// GovernanceExpectation matches but the evaluation actually ran for a
// different Surface — the expected Surface was not invoked.
//
// Two surface IDs disambiguate "what was expected" from "what actually
// happened":
//
//   - missing_surface_id is the Surface the expectation required
//     (m.RequiredSurfaceID); it carries the issue's "missing surface
//     ID" label.
//   - actual_surface_id is the Surface this evaluation was actually
//     for (req.SurfaceID).
//
// The correlation_basis sub-object discriminates the MVP correlation
// model. Today the only value is "same_evaluation" — both observations
// (the matched condition and the surface mismatch) are produced inside
// the same orchestrator pass, so elapsed time is meaningfully zero and
// is deliberately omitted. The discriminator's `type` field gives
// future correlation models (time_window, external_evidence) a stable
// extension shape without restructuring the payload.
//
// summary is built by the shared buildCoverageContextSummary helper so
// the gap and detected events for the same match carry byte-identical
// summary fields.
func buildGovernanceCoverageGapPayload(
	m governancecoverage.Match,
	req eval.DecisionRequest,
	processID string,
	envelopeID string,
) map[string]any {
	return map[string]any{
		"expectation_id":      m.ExpectationID,
		"expectation_version": m.Version,
		"missing_surface_id":  m.RequiredSurfaceID,
		"actual_surface_id":   req.SurfaceID,
		"process_id":          processID,
		"condition_type":      string(m.ConditionType),
		"correlation_basis": map[string]any{
			"type":           "same_evaluation",
			"request_source": req.RequestSource,
			"request_id":     req.RequestID,
			"envelope_id":    envelopeID,
		},
		"summary": buildCoverageContextSummary(req),
	}
}

// buildCoverageContextSummary returns the typed risk-shape summary
// shared by the GOVERNANCE_CONDITION_DETECTED and
// GOVERNANCE_COVERAGE_GAP events. Both events describe the same matched
// condition; identical summary shape is required so #56's read model
// can join them on a single canonical sub-object.
//
// Follows the existing CONSEQUENCE_CHECKED pattern (see
// appendConsequenceCheckedEvent in orchestrator.go): when
// req.Consequence is nil the consequence key is omitted entirely so a
// JSONB-path query like `payload_json -> 'summary' ? 'consequence'` is
// true only when consequence facts were actually present at evaluation
// time. Including `consequence: {}` would create false positives for
// that query shape and is the wrong shape for the audit consumer.
func buildCoverageContextSummary(req eval.DecisionRequest) map[string]any {
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
	return summary
}
