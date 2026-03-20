package outbox

import (
	"encoding/json"
	"fmt"
	"time"
)

// builders.go centralises construction of all outbox event payloads. Every
// builder function returns a json.RawMessage produced by marshalling a typed
// contract struct, guaranteeing schema consistency across all call sites.
//
// CONTRACT MODEL — PERMISSIVE BUILDERS
//
// Builders are schema constructors, not domain validators. Their responsibility
// is to produce well-formed JSON that conforms to the declared contract struct.
// Empty string arguments are accepted; builders do not reject them. Semantic
// completeness (e.g. non-empty envelope IDs, valid outcome codes) is enforced
// by callers — the orchestrator, approval service, and review handler — not
// here. This separation keeps builders predictable and independently testable.
//
// All builders set:
//   - EventVersion = "v1"
//   - Timestamp    = time.Now().UTC().Format(time.RFC3339)

const eventVersion = "v1"

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// BuildDecisionCompletedEvent constructs the payload for EventDecisionCompleted.
// All arguments are required; empty strings are accepted for fields that may be
// unavailable at construction time (e.g. surfaceID before surface resolution).
func BuildDecisionCompletedEvent(
	envelopeID, requestSource, requestID,
	surfaceID, agentID,
	outcome, reasonCode string,
) (json.RawMessage, error) {
	ev := DecisionCompletedEvent{
		EventVersion:  eventVersion,
		EnvelopeID:    envelopeID,
		RequestSource: requestSource,
		RequestID:     requestID,
		SurfaceID:     surfaceID,
		AgentID:       agentID,
		Outcome:       outcome,
		ReasonCode:    reasonCode,
		Timestamp:     nowTimestamp(),
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal DecisionCompletedEvent: %w", err)
	}
	return json.RawMessage(b), nil
}

// BuildDecisionEscalatedEvent constructs the payload for EventDecisionEscalated.
func BuildDecisionEscalatedEvent(
	envelopeID, requestSource, requestID,
	surfaceID, agentID,
	reasonCode string,
) (json.RawMessage, error) {
	ev := DecisionEscalatedEvent{
		EventVersion:  eventVersion,
		EnvelopeID:    envelopeID,
		RequestSource: requestSource,
		RequestID:     requestID,
		SurfaceID:     surfaceID,
		AgentID:       agentID,
		ReasonCode:    reasonCode,
		Timestamp:     nowTimestamp(),
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal DecisionEscalatedEvent: %w", err)
	}
	return json.RawMessage(b), nil
}

// BuildDecisionReviewResolvedEvent constructs the payload for EventDecisionReviewResolved.
func BuildDecisionReviewResolvedEvent(
	envelopeID, requestSource, requestID,
	decision, reviewerID string,
) (json.RawMessage, error) {
	ev := DecisionReviewResolvedEvent{
		EventVersion:  eventVersion,
		EnvelopeID:    envelopeID,
		RequestSource: requestSource,
		RequestID:     requestID,
		Decision:      decision,
		ReviewerID:    reviewerID,
		Timestamp:     nowTimestamp(),
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal DecisionReviewResolvedEvent: %w", err)
	}
	return json.RawMessage(b), nil
}

// BuildSurfaceApprovedEvent constructs the payload for EventSurfaceApproved.
func BuildSurfaceApprovedEvent(surfaceID, approvedBy string) (json.RawMessage, error) {
	ev := SurfaceApprovedEvent{
		EventVersion: eventVersion,
		SurfaceID:    surfaceID,
		ApprovedBy:   approvedBy,
		Timestamp:    nowTimestamp(),
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal SurfaceApprovedEvent: %w", err)
	}
	return json.RawMessage(b), nil
}

// BuildSurfaceDeprecatedEvent constructs the payload for EventSurfaceDeprecated.
func BuildSurfaceDeprecatedEvent(surfaceID, deprecatedBy string) (json.RawMessage, error) {
	ev := SurfaceDeprecatedEvent{
		EventVersion: eventVersion,
		SurfaceID:    surfaceID,
		DeprecatedBy: deprecatedBy,
		Timestamp:    nowTimestamp(),
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal SurfaceDeprecatedEvent: %w", err)
	}
	return json.RawMessage(b), nil
}
