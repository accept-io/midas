package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type hashInput struct {
	EnvelopeID      string             `json:"envelope_id"`
	RequestID       string             `json:"request_id"`
	SequenceNo      int                `json:"sequence_no"`
	EventType       AuditEventType     `json:"event_type"`
	PerformedByType EventPerformerType `json:"performed_by_type"`
	PerformedByID   string             `json:"performed_by_id"`
	Payload         map[string]any     `json:"payload"`
	OccurredAt      string             `json:"occurred_at"`
	PrevHash        string             `json:"prev_hash"`
}

func CanonicalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func ComputeEventHash(e *AuditEvent) (string, error) {
	input := hashInput{
		EnvelopeID:      e.EnvelopeID,
		RequestID:       e.RequestID,
		SequenceNo:      e.SequenceNo,
		EventType:       e.EventType,
		PerformedByType: e.PerformedByType,
		PerformedByID:   e.PerformedByID,
		Payload:         e.Payload,
		OccurredAt:      e.OccurredAt.UTC().Format(time.RFC3339Nano),
		PrevHash:        e.PrevHash,
	}

	b, err := CanonicalJSON(input)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
