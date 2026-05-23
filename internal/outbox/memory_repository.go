package outbox

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryRepository is an in-process outbox repository backed by a mutex-guarded
// slice. It is suitable for unit tests and in-memory integration tests that do
// not require durable storage.
//
// Because the memory store has no real transaction support, Append merely
// appends to the slice. Tests that need to verify rollback behaviour must use
// the Postgres repository with a real transaction.
type MemoryRepository struct {
	mu     sync.RWMutex
	events []*OutboxEvent
}

// NewMemoryRepository returns an empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{}
}

// Append adds ev to the in-memory event list. ev must not be nil.
func (r *MemoryRepository) Append(ctx context.Context, ev *OutboxEvent) error {
	return r.AppendBatch(ctx, []*OutboxEvent{ev})
}

// AppendBatch adds events to the in-memory event list in declaration order.
func (r *MemoryRepository) AppendBatch(_ context.Context, events []*OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}
	for _, ev := range events {
		if ev == nil {
			return fmt.Errorf("outbox: AppendBatch called with nil event")
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
	return nil
}

// ClaimUnpublished returns up to limit events where PublishedAt is nil, in
// insertion order. The in-memory implementation does not use actual locking;
// it is suitable for unit tests only.
func (r *MemoryRepository) ClaimUnpublished(_ context.Context, limit int) ([]*OutboxEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []*OutboxEvent
	for _, ev := range r.events {
		if ev.PublishedAt == nil {
			out = append(out, ev)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// ListUnpublished returns all events where PublishedAt is nil, in insertion order.
func (r *MemoryRepository) ListUnpublished(_ context.Context) ([]*OutboxEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []*OutboxEvent
	for _, ev := range r.events {
		if ev.PublishedAt == nil {
			out = append(out, ev)
		}
	}
	return out, nil
}

// BacklogStats returns aggregate unpublished-row metrics for the in-memory
// repository. It is used by tests and memory-mode safety checks.
func (r *MemoryRepository) BacklogStats(_ context.Context) (BacklogStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var stats BacklogStats
	var oldest time.Time
	for _, ev := range r.events {
		if ev.PublishedAt != nil {
			continue
		}
		stats.UnpublishedCount++
		if oldest.IsZero() || ev.CreatedAt.Before(oldest) {
			oldest = ev.CreatedAt
		}
	}
	if !oldest.IsZero() {
		stats.OldestUnpublishedAge = time.Since(oldest)
		if stats.OldestUnpublishedAge < 0 {
			stats.OldestUnpublishedAge = 0
		}
	}
	return stats, nil
}

// MarkPublished sets PublishedAt on the event with the given ID.
// Returns an error if no event with that ID exists.
func (r *MemoryRepository) MarkPublished(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	for _, ev := range r.events {
		if ev.ID == id {
			ev.PublishedAt = &now
			return nil
		}
	}
	return fmt.Errorf("outbox: event %q not found", id)
}

// All returns a snapshot of every event in the repository, published or not.
// Used in tests to assert the full set of appended events.
func (r *MemoryRepository) All(_ context.Context) []*OutboxEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*OutboxEvent, len(r.events))
	copy(out, r.events)
	return out
}

var _ Repository = (*MemoryRepository)(nil)
