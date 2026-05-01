package audit

import (
	"context"
	"reflect"
	"sort"
	"sync"
)

type MemoryRepository struct {
	mu sync.RWMutex

	eventsByEnvelope map[string][]*AuditEvent
	eventsByRequest  map[string][]*AuditEvent

	// all preserves insertion order across envelopes for the generic
	// List query. Append to this slice in addition to the per-envelope
	// and per-request maps. The slice is the single source of truth
	// for time-range queries.
	all []*AuditEvent
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
	r.all = append(r.all, ev)

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

// List implements AuditEventRepository.List for the in-memory backend.
// Linear scan over r.all; defensive copies returned so callers cannot
// mutate stored state. Predicates mirror the Postgres impl exactly.
//
// Ordering: by occurred_at (descending when filter.OrderDesc is true,
// ascending otherwise). Tie-break by SequenceNo so events within a
// single envelope retain their insertion order — important for tests
// that rely on stable ordering across runs.
func (r *MemoryRepository) List(ctx context.Context, filter ListFilter) ([]*AuditEvent, error) {
	_ = ctx

	if err := filter.Validate(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	wantTypes := effectiveEventTypes(filter)
	limit := filter.EffectiveLimit()

	var matched []*AuditEvent
	for _, ev := range r.all {
		if !matchesEventTypes(ev, wantTypes) {
			continue
		}
		if filter.EnvelopeID != "" && ev.EnvelopeID != filter.EnvelopeID {
			continue
		}
		if filter.RequestSource != "" && ev.RequestSource != filter.RequestSource {
			continue
		}
		if filter.RequestID != "" && ev.RequestID != filter.RequestID {
			continue
		}
		if !filter.Since.IsZero() && ev.OccurredAt.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && !ev.OccurredAt.Before(filter.Until) {
			continue
		}
		if !payloadContainsAll(ev.Payload, filter.PayloadContains) {
			continue
		}
		matched = append(matched, ev)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].OccurredAt.Equal(matched[j].OccurredAt) {
			// Tie-break by sequence number within the envelope so
			// detected/gap pairs from the same evaluation always
			// surface in the chain order the accumulator wrote them.
			return matched[i].SequenceNo < matched[j].SequenceNo
		}
		if filter.OrderDesc {
			return matched[i].OccurredAt.After(matched[j].OccurredAt)
		}
		return matched[i].OccurredAt.Before(matched[j].OccurredAt)
	})

	if len(matched) > limit {
		matched = matched[:limit]
	}

	// Defensive copy: callers must not be able to mutate stored state.
	out := make([]*AuditEvent, len(matched))
	for i, ev := range matched {
		cp := *ev
		out[i] = &cp
	}
	return out, nil
}

// effectiveEventTypes returns the event-type allowlist to apply.
// EventTypes wins when non-empty (broader filter); otherwise the
// single EventType is used. Returns nil when neither is set, signalling
// "any event type".
func effectiveEventTypes(f ListFilter) []AuditEventType {
	if len(f.EventTypes) > 0 {
		return f.EventTypes
	}
	if f.EventType != "" {
		return []AuditEventType{f.EventType}
	}
	return nil
}

// matchesEventTypes reports whether ev's type is in want, or
// unconditionally true when want is nil.
func matchesEventTypes(ev *AuditEvent, want []AuditEventType) bool {
	if want == nil {
		return true
	}
	for _, t := range want {
		if ev.EventType == t {
			return true
		}
	}
	return false
}

// payloadContainsAll reports whether every (key, value) in want is
// present at the top level of payload with deep-equal value. Returns
// true when want is empty. Mirrors PostgreSQL's @> JSONB containment
// for top-level keys (no nested-path semantics).
func payloadContainsAll(payload map[string]any, want map[string]any) bool {
	if len(want) == 0 {
		return true
	}
	for k, wv := range want {
		gv, ok := payload[k]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(gv, wv) {
			return false
		}
	}
	return true
}

var _ AuditEventRepository = (*MemoryRepository)(nil)
