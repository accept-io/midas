package audit

import (
	"context"
	"sync"
)

type MemoryRepository struct {
	mu sync.RWMutex

	eventsByEnvelope map[string][]*AuditEvent
	eventsByRequest  map[string][]*AuditEvent
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		eventsByEnvelope: make(map[string][]*AuditEvent),
		eventsByRequest:  make(map[string][]*AuditEvent),
	}
}

func (r *MemoryRepository) Append(ctx context.Context, ev *AuditEvent) error {
	_ = ctx

	r.mu.Lock()
	defer r.mu.Unlock()

	envelopeEvents := r.eventsByEnvelope[ev.EnvelopeID]

	ev.SequenceNo = len(envelopeEvents) + 1

	if len(envelopeEvents) > 0 {
		ev.PrevHash = envelopeEvents[len(envelopeEvents)-1].EventHash
	} else {
		ev.PrevHash = ""
	}

	hash, err := ComputeEventHash(ev)
	if err != nil {
		return err
	}

	ev.setHash(hash)

	r.eventsByEnvelope[ev.EnvelopeID] = append(r.eventsByEnvelope[ev.EnvelopeID], ev)
	r.eventsByRequest[ev.RequestID] = append(r.eventsByRequest[ev.RequestID], ev)

	return nil
}

func (r *MemoryRepository) ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*AuditEvent, error) {
	_ = ctx

	r.mu.RLock()
	defer r.mu.RUnlock()

	events := r.eventsByEnvelope[envelopeID]
	result := make([]*AuditEvent, len(events))
	copy(result, events)

	return result, nil
}

func (r *MemoryRepository) ListByRequestID(ctx context.Context, requestID string) ([]*AuditEvent, error) {
	_ = ctx

	r.mu.RLock()
	defer r.mu.RUnlock()

	events := r.eventsByRequest[requestID]
	result := make([]*AuditEvent, len(events))
	copy(result, events)

	return result, nil
}

var _ AuditEventRepository = (*MemoryRepository)(nil)
