package audit

import (
	"time"

	"github.com/google/uuid"
)

type AuditEvent struct {
	ID            string
	EnvelopeID    string
	RequestSource string // Schema v2.1: source system identifier
	RequestID     string

	SequenceNo int
	EventType  AuditEventType

	PerformedByType EventPerformerType
	PerformedByID   string

	Payload map[string]any

	OccurredAt time.Time

	PrevHash  string
	EventHash string

	// Hash is an alias for EventHash used by the orchestrator integrity tracking.
	// Both fields refer to the same value; repositories populate EventHash and
	// the orchestrator reads Hash. They are kept in sync by the Append methods.
	Hash string
}

// NewEvent creates a new audit event with schema v2.1 request scoping.
func NewEvent(
	envelopeID string,
	requestSource string,
	requestID string,
	eventType AuditEventType,
	performerType EventPerformerType,
	performerID string,
	payload map[string]any,
) *AuditEvent {
	if payload == nil {
		payload = map[string]any{}
	}

	return &AuditEvent{
		ID:              uuid.NewString(),
		EnvelopeID:      envelopeID,
		RequestSource:   requestSource,
		RequestID:       requestID,
		EventType:       eventType,
		PerformedByType: performerType,
		PerformedByID:   performerID,
		Payload:         payload,
		OccurredAt:      time.Now().UTC(),
	}
}

// setHash sets both Hash and EventHash to the same value, keeping
// the two fields consistent. Call this after computing the event hash.
func (e *AuditEvent) setHash(h string) {
	e.Hash = h
	e.EventHash = h
}
