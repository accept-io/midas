package outbox

import "encoding/json"

// contracts.go defines versioned event payload types for all outbox events
// emitted by MIDAS. Each struct corresponds to a named integration event
// that downstream consumers may subscribe to.
//
// Versioning: all structs carry an EventVersion field. The current version
// for every event type is "v1". A version bump introduces a new struct
// (e.g. DecisionCompletedEventV2) rather than modifying the existing one,
// so that producers and consumers can migrate independently.
//
// JSON field names are the authoritative schema for downstream consumers.
// Do not rename json tags without a version bump.

// DecisionCompletedEvent is the payload for EventDecisionCompleted.
// Emitted when an evaluation closes with the Execute (accept) outcome.
type DecisionCompletedEvent struct {
	EventVersion  string `json:"event_version"`
	EnvelopeID    string `json:"envelope_id"`
	RequestSource string `json:"request_source"`
	RequestID     string `json:"request_id"`
	SurfaceID     string `json:"surface_id"`
	AgentID       string `json:"agent_id"`
	Outcome       string `json:"outcome"`
	ReasonCode    string `json:"reason_code"`
	Timestamp     string `json:"timestamp"`
}

// DecisionEscalatedEvent is the payload for EventDecisionEscalated.
// Emitted when an evaluation produces an Escalate outcome and the envelope
// transitions to AWAITING_REVIEW.
type DecisionEscalatedEvent struct {
	EventVersion  string `json:"event_version"`
	EnvelopeID    string `json:"envelope_id"`
	RequestSource string `json:"request_source"`
	RequestID     string `json:"request_id"`
	SurfaceID     string `json:"surface_id"`
	AgentID       string `json:"agent_id"`
	ReasonCode    string `json:"reason_code"`
	Timestamp     string `json:"timestamp"`
}

// DecisionReviewResolvedEvent is the payload for EventDecisionReviewResolved.
// Emitted when a reviewer closes an escalated envelope. The Decision field
// distinguishes APPROVED from REJECTED resolutions.
type DecisionReviewResolvedEvent struct {
	EventVersion  string `json:"event_version"`
	EnvelopeID    string `json:"envelope_id"`
	RequestSource string `json:"request_source"`
	RequestID     string `json:"request_id"`
	Decision      string `json:"decision"`
	ReviewerID    string `json:"reviewer_id"`
	Timestamp     string `json:"timestamp"`
}

// SurfaceApprovedEvent is the payload for EventSurfaceApproved.
// Emitted when ApproveSurface successfully transitions a surface from review
// to active.
type SurfaceApprovedEvent struct {
	EventVersion string `json:"event_version"`
	SurfaceID    string `json:"surface_id"`
	ApprovedBy   string `json:"approved_by"`
	Timestamp    string `json:"timestamp"`
}

// SurfaceDeprecatedEvent is the payload for EventSurfaceDeprecated.
// Emitted when DeprecateSurface successfully transitions a surface from active
// to deprecated.
type SurfaceDeprecatedEvent struct {
	EventVersion string `json:"event_version"`
	SurfaceID    string `json:"surface_id"`
	DeprecatedBy string `json:"deprecated_by"`
	Timestamp    string `json:"timestamp"`
}

// ProfileApprovedEvent is the payload for EventProfileApproved.
// Emitted when ApproveProfile successfully transitions a profile from review
// to active.
type ProfileApprovedEvent struct {
	EventVersion string `json:"event_version"`
	ProfileID    string `json:"profile_id"`
	SurfaceID    string `json:"surface_id"`
	ApprovedBy   string `json:"approved_by"`
	Timestamp    string `json:"timestamp"`
}

// ProfileDeprecatedEvent is the payload for EventProfileDeprecated.
// Emitted when DeprecateProfile successfully transitions a profile from active
// to deprecated.
type ProfileDeprecatedEvent struct {
	EventVersion string `json:"event_version"`
	ProfileID    string `json:"profile_id"`
	SurfaceID    string `json:"surface_id"`
	DeprecatedBy string `json:"deprecated_by"`
	Timestamp    string `json:"timestamp"`
}

// GrantSuspendedEvent is the payload for EventGrantSuspended.
// Emitted when SuspendGrant successfully transitions a grant from active to
// suspended.
type GrantSuspendedEvent struct {
	EventVersion string `json:"event_version"`
	GrantID      string `json:"grant_id"`
	AgentID      string `json:"agent_id"`
	ProfileID    string `json:"profile_id"`
	SuspendedBy  string `json:"suspended_by"`
	Reason       string `json:"reason,omitempty"`
	Timestamp    string `json:"timestamp"`
}

// GrantRevokedEvent is the payload for EventGrantRevoked.
// Emitted when RevokeGrant permanently revokes a grant.
type GrantRevokedEvent struct {
	EventVersion string `json:"event_version"`
	GrantID      string `json:"grant_id"`
	AgentID      string `json:"agent_id"`
	ProfileID    string `json:"profile_id"`
	RevokedBy    string `json:"revoked_by"`
	Reason       string `json:"reason,omitempty"`
	Timestamp    string `json:"timestamp"`
}

// GrantReinstatedEvent is the payload for EventGrantReinstated.
// Emitted when ReinstateGrant restores a suspended grant to active.
type GrantReinstatedEvent struct {
	EventVersion string `json:"event_version"`
	GrantID      string `json:"grant_id"`
	AgentID      string `json:"agent_id"`
	ProfileID    string `json:"profile_id"`
	ReinstatedBy string `json:"reinstated_by"`
	Timestamp    string `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// External event contract types (docs/events.md)
//
// These types implement the versioned external event contract. The outer
// ExternalEventEnvelope wraps a typed inner payload and carries the fields
// that all external events share: schema_version, event_id, type,
// occurred_at, envelope_id. Field names are authoritative — do not rename
// json tags without incrementing schema_version.
// ---------------------------------------------------------------------------

// ExternalEventEnvelope is the outer wrapper for all external MIDAS events.
// Payload holds the typed inner payload marshalled as raw JSON.
type ExternalEventEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	EventID       string          `json:"event_id"`
	Type          string          `json:"type"`
	OccurredAt    string          `json:"occurred_at"`
	EnvelopeID    string          `json:"envelope_id"`
	Payload       json.RawMessage `json:"payload"`
}

// DecisionOutcomeRecordedPayload is the payload for EventDecisionOutcomeRecorded.
// Emitted for all evaluation outcomes (accept, escalate, reject, request_clarification).
type DecisionOutcomeRecordedPayload struct {
	RequestSource string `json:"request_source"`
	RequestID     string `json:"request_id"`
	SurfaceID     string `json:"surface_id"`
	AgentID       string `json:"agent_id"`
	Outcome       string `json:"outcome"`
	ReasonCode    string `json:"reason_code"`
}

// DecisionEnvelopeClosedPayload is the payload for EventDecisionEnvelopeClosed.
// Emitted when a governance envelope reaches the closed terminal state.
// Review is present only for envelopes closed via escalation review.
type DecisionEnvelopeClosedPayload struct {
	RequestSource string                        `json:"request_source"`
	RequestID     string                        `json:"request_id"`
	FinalOutcome  string                        `json:"final_outcome"`
	ClosedAt      string                        `json:"closed_at"`
	Review        *DecisionEnvelopeClosedReview `json:"review,omitempty"`
}

// DecisionEnvelopeClosedReview carries the reviewer's decision for envelopes
// closed after escalation review. ReviewerKind has no omitempty because it is
// always known when a review object is present; Notes is explicitly optional.
type DecisionEnvelopeClosedReview struct {
	Decision     string `json:"decision"`
	ReviewerID   string `json:"reviewer_id"`
	ReviewerKind string `json:"reviewer_kind"`
	Notes        string `json:"notes,omitempty"`
}
