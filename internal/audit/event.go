package audit

import (
	"time"

	"github.com/google/uuid"
)

type AuditEvent struct {
	ID         string
	EnvelopeID string
	RequestID  string

	SequenceNo int
	EventType  AuditEventType

	PerformedByType EventPerformerType
	PerformedByID   string

	Payload map[string]any

	OccurredAt time.Time

	PrevHash  string
	EventHash string
}

func NewEvent(
	envelopeID string,
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
		RequestID:       requestID,
		EventType:       eventType,
		PerformedByType: performerType,
		PerformedByID:   performerID,
		Payload:         payload,
		OccurredAt:      time.Now().UTC(),
	}
}
