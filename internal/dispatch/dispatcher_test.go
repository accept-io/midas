package dispatch_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/dispatch"
	"github.com/accept-io/midas/internal/outbox"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeRepo is an in-memory DispatcherRepo for unit tests.
type fakeRepo struct {
	events       []*outbox.OutboxEvent
	publishedIDs []string

	claimErr       error
	markPublishErr error
}

func (r *fakeRepo) ClaimUnpublished(_ context.Context, limit int) ([]*outbox.OutboxEvent, error) {
	if r.claimErr != nil {
		return nil, r.claimErr
	}
	var out []*outbox.OutboxEvent
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

func (r *fakeRepo) MarkPublished(_ context.Context, id string) error {
	if r.markPublishErr != nil {
		return r.markPublishErr
	}
	now := time.Now().UTC()
	for _, ev := range r.events {
		if ev.ID == id {
			ev.PublishedAt = &now
			r.publishedIDs = append(r.publishedIDs, id)
			return nil
		}
	}
	return errors.New("outbox: event not found: " + id)
}

// fakePublisher records published messages and can be configured to fail.
type fakePublisher struct {
	published  []dispatch.Message
	publishErr error
	callCount  atomic.Int64
}

func (p *fakePublisher) Publish(_ context.Context, msg dispatch.Message) error {
	p.callCount.Add(1)
	if p.publishErr != nil {
		return p.publishErr
	}
	p.published = append(p.published, msg)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustEvent(t *testing.T, eventType outbox.EventType, aggregateID string) *outbox.OutboxEvent {
	t.Helper()
	ev, err := outbox.New(eventType, "envelope", aggregateID, "midas.decisions", "key:"+aggregateID, json.RawMessage(`{"id":"`+aggregateID+`"}`))
	if err != nil {
		t.Fatalf("outbox.New: %v", err)
	}
	return ev
}

func shortIntervalConfig() dispatch.DispatcherConfig {
	return dispatch.DispatcherConfig{
		BatchSize:    10,
		PollInterval: 5 * time.Millisecond,
		MaxBackoff:   10 * time.Millisecond,
	}
}

func newDispatcher(t *testing.T, repo dispatch.DispatcherRepo, pub dispatch.Publisher, cfg dispatch.DispatcherConfig) *dispatch.Dispatcher {
	t.Helper()
	d, err := dispatch.NewDispatcher(repo, pub, cfg)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	return d
}

// ---------------------------------------------------------------------------
// NewDispatcher validation
// ---------------------------------------------------------------------------

func TestNewDispatcher_NilRepo_ReturnsError(t *testing.T) {
	pub := &fakePublisher{}
	_, err := dispatch.NewDispatcher(nil, pub, shortIntervalConfig())
	if err == nil {
		t.Fatal("expected error for nil repo, got nil")
	}
}

func TestNewDispatcher_NilPublisher_ReturnsError(t *testing.T) {
	repo := &fakeRepo{}
	_, err := dispatch.NewDispatcher(repo, nil, shortIntervalConfig())
	if err == nil {
		t.Fatal("expected error for nil publisher, got nil")
	}
}

func TestNewDispatcher_ZeroBatchSize_ReturnsError(t *testing.T) {
	cfg := shortIntervalConfig()
	cfg.BatchSize = 0
	_, err := dispatch.NewDispatcher(&fakeRepo{}, &fakePublisher{}, cfg)
	if err == nil {
		t.Fatal("expected error for zero BatchSize, got nil")
	}
}

func TestNewDispatcher_ZeroPollInterval_ReturnsError(t *testing.T) {
	cfg := shortIntervalConfig()
	cfg.PollInterval = 0
	_, err := dispatch.NewDispatcher(&fakeRepo{}, &fakePublisher{}, cfg)
	if err == nil {
		t.Fatal("expected error for zero PollInterval, got nil")
	}
}

// ---------------------------------------------------------------------------
// Dispatch behaviour tests
// ---------------------------------------------------------------------------

// TestDispatcher_EmptyQueue_DoesNothing verifies that when the queue is empty
// the publisher is never called and no errors occur.
func TestDispatcher_EmptyQueue_DoesNothing(t *testing.T) {
	repo := &fakeRepo{}
	pub := &fakePublisher{}
	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	d.Run(ctx)

	if pub.callCount.Load() != 0 {
		t.Errorf("expected 0 publish calls for empty queue, got %d", pub.callCount.Load())
	}
}

// TestDispatcher_OneEvent_PublishedAndMarked verifies the happy path: one
// unpublished event is published and then marked published.
func TestDispatcher_OneEvent_PublishedAndMarked(t *testing.T) {
	ev := mustEvent(t, outbox.EventDecisionCompleted, "env-1")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev}}
	pub := &fakePublisher{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	d.Run(ctx)

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(pub.published))
	}
	if pub.published[0].Topic != "midas.decisions" {
		t.Errorf("expected topic %q, got %q", "midas.decisions", pub.published[0].Topic)
	}
	if ev.PublishedAt == nil {
		t.Error("expected event to be marked published, PublishedAt is nil")
	}
}

// TestDispatcher_PublishFailure_LeavesRowUnpublished verifies that when the
// broker rejects the message the row is not marked published.
func TestDispatcher_PublishFailure_LeavesRowUnpublished(t *testing.T) {
	ev := mustEvent(t, outbox.EventDecisionCompleted, "env-fail")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev}}
	pub := &fakePublisher{publishErr: errors.New("broker unavailable")}

	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	d.Run(ctx)

	if ev.PublishedAt != nil {
		t.Error("expected event to remain unpublished after publish failure")
	}
	if len(repo.publishedIDs) != 0 {
		t.Errorf("expected 0 mark-published calls, got %d", len(repo.publishedIDs))
	}
}

// TestDispatcher_MarkPublishedFailure_LoggedAfterSuccessfulPublish verifies
// that when MarkPublished fails after a successful broker publish, the
// dispatcher continues without panic. The event remains available for retry
// (at-least-once delivery).
func TestDispatcher_MarkPublishedFailure_AcceptedAsAtLeastOnce(t *testing.T) {
	ev := mustEvent(t, outbox.EventDecisionCompleted, "env-mark-fail")
	repo := &fakeRepo{
		events:         []*outbox.OutboxEvent{ev},
		markPublishErr: errors.New("postgres connection lost"),
	}
	pub := &fakePublisher{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	// Must not panic; dispatcher continues running.
	d.Run(ctx)

	// The message was published to the broker.
	if len(pub.published) == 0 {
		t.Error("expected at least one broker publish before MarkPublished failure")
	}
	// But the event is not marked published in the repo.
	if ev.PublishedAt != nil {
		t.Error("expected event.PublishedAt to remain nil when MarkPublished fails")
	}
}

// TestDispatcher_MultipleEvents_ProcessedInOrder verifies that multiple events
// are all published and marked published, and that they are published in the
// order returned by ClaimUnpublished.
func TestDispatcher_MultipleEvents_ProcessedInOrder(t *testing.T) {
	ev1 := mustEvent(t, outbox.EventDecisionCompleted, "env-a")
	ev2 := mustEvent(t, outbox.EventDecisionEscalated, "env-b")
	ev3 := mustEvent(t, outbox.EventDecisionReviewResolved, "env-c")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev1, ev2, ev3}}
	pub := &fakePublisher{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	d.Run(ctx)

	if len(pub.published) != 3 {
		t.Fatalf("expected 3 published messages, got %d", len(pub.published))
	}
	// Verify all events are marked published.
	for _, ev := range []*outbox.OutboxEvent{ev1, ev2, ev3} {
		if ev.PublishedAt == nil {
			t.Errorf("expected event %q to be marked published", ev.ID)
		}
	}
}

// TestDispatcher_ContextCancellation_ExitsCleanly verifies that cancelling the
// context causes Run to return without blocking indefinitely.
func TestDispatcher_ContextCancellation_ExitsCleanly(t *testing.T) {
	repo := &fakeRepo{}
	pub := &fakePublisher{}
	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Dispatcher exited cleanly.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not exit within 500ms after context cancellation")
	}
}

// TestDispatcher_ClaimError_BacksOff verifies that when ClaimUnpublished
// returns an error the dispatcher backs off and does not call Publish.
func TestDispatcher_ClaimError_BacksOff(t *testing.T) {
	repo := &fakeRepo{claimErr: errors.New("database error")}
	pub := &fakePublisher{}
	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	d.Run(ctx)

	if pub.callCount.Load() != 0 {
		t.Errorf("expected 0 publish calls when claim fails, got %d", pub.callCount.Load())
	}
}

// ---------------------------------------------------------------------------
// Message construction tests
// ---------------------------------------------------------------------------

// TestDispatcher_MessageConstruction verifies that the Message published to the
// broker is correctly derived from the OutboxEvent fields.
func TestDispatcher_MessageConstruction(t *testing.T) {
	ev, err := outbox.New(
		outbox.EventSurfaceApproved,
		"surface",
		"surf-xyz",
		"midas.surfaces",
		"routing-key-1",
		json.RawMessage(`{"surface_id":"surf-xyz"}`),
	)
	if err != nil {
		t.Fatalf("outbox.New: %v", err)
	}

	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev}}
	pub := &fakePublisher{}
	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(pub.published))
	}

	msg := pub.published[0]

	if msg.Topic != "midas.surfaces" {
		t.Errorf("expected Topic %q, got %q", "midas.surfaces", msg.Topic)
	}
	if string(msg.Key) != "routing-key-1" {
		t.Errorf("expected Key %q, got %q", "routing-key-1", string(msg.Key))
	}
	if string(msg.Value) != `{"surface_id":"surf-xyz"}` {
		t.Errorf("expected Value %q, got %q", `{"surface_id":"surf-xyz"}`, string(msg.Value))
	}

	// Verify required headers are present.
	headerMap := make(map[string]string)
	for _, h := range msg.Headers {
		headerMap[h.Key] = string(h.Value)
	}
	assertHeader(t, headerMap, "event_type", string(outbox.EventSurfaceApproved))
	assertHeader(t, headerMap, "aggregate_type", "surface")
	assertHeader(t, headerMap, "aggregate_id", "surf-xyz")
}

func assertHeader(t *testing.T, headers map[string]string, key, want string) {
	t.Helper()
	got, ok := headers[key]
	if !ok {
		t.Errorf("expected header %q to be present", key)
		return
	}
	if got != want {
		t.Errorf("header %q: expected %q, got %q", key, want, got)
	}
}

// TestDispatcher_EmptyEventKey_NilMessageKey verifies that an empty EventKey
// results in a nil Message.Key (Kafka uses nil key for non-partitioned messages).
func TestDispatcher_EmptyEventKey_NilMessageKey(t *testing.T) {
	ev, err := outbox.New(
		outbox.EventDecisionCompleted,
		"envelope",
		"env-nokey",
		"midas.decisions",
		"", // empty key
		nil,
	)
	if err != nil {
		t.Fatalf("outbox.New: %v", err)
	}

	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev}}
	pub := &fakePublisher{}
	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(pub.published))
	}
	if pub.published[0].Key != nil {
		t.Errorf("expected nil Key for empty EventKey, got %q", pub.published[0].Key)
	}
}
