package dispatch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
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
	events           []*outbox.OutboxEvent
	publishedIDs     []string
	markBatchIDs     [][]string
	affectedOverride *int64

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

func (r *fakeRepo) MarkPublishedBatch(_ context.Context, ids []string) (int64, error) {
	if r.markPublishErr != nil {
		return 0, r.markPublishErr
	}
	copied := append([]string(nil), ids...)
	r.markBatchIDs = append(r.markBatchIDs, copied)

	now := time.Now().UTC()
	var affected int64
	for _, id := range ids {
		for _, ev := range r.events {
			if ev.ID == id {
				ev.PublishedAt = &now
				r.publishedIDs = append(r.publishedIDs, id)
				affected++
				break
			}
		}
	}
	if r.affectedOverride != nil {
		return *r.affectedOverride, nil
	}
	return affected, nil
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

type fakeBatchPublisher struct {
	fakePublisher
	publishedBatches [][]dispatch.Message
	batchErr         error
	batchFailures    map[int]error
	failKeys         map[string]error
	batchCallCount   atomic.Int64
}

func (p *fakeBatchPublisher) PublishBatch(_ context.Context, msgs []dispatch.Message) (dispatch.PublishBatchResult, error) {
	p.batchCallCount.Add(1)
	copied := append([]dispatch.Message(nil), msgs...)
	p.publishedBatches = append(p.publishedBatches, copied)

	if p.batchErr != nil {
		result := dispatch.PublishBatchResult{
			Failed: make([]dispatch.PublishBatchFailure, len(msgs)),
		}
		for i := range msgs {
			result.Failed[i] = dispatch.PublishBatchFailure{Index: i, Err: p.batchErr}
		}
		return result, p.batchErr
	}

	result := dispatch.PublishBatchResult{
		Successful: make([]int, 0, len(msgs)),
	}
	for i, msg := range msgs {
		if err := p.batchFailures[i]; err != nil {
			result.Failed = append(result.Failed, dispatch.PublishBatchFailure{Index: i, Err: err})
			continue
		}
		if err := p.failKeys[string(msg.Key)]; err != nil {
			result.Failed = append(result.Failed, dispatch.PublishBatchFailure{Index: i, Err: err})
			continue
		}
		result.Successful = append(result.Successful, i)
		p.published = append(p.published, msg)
	}
	if len(result.Failed) > 0 {
		return result, result.Failed[0].Err
	}
	return result, nil
}

type fakeRecorder struct {
	claimDurations         int
	publishDurations       int
	markPublishedDurations int
	claimed                int
	published              int
	publishFailures        int
	markPublishedFailures  int
	batchSizes             []int
	topics                 []string
	errorClasses           []string
}

func (r *fakeRecorder) RecordClaimDuration(time.Duration) {
	r.claimDurations++
}

func (r *fakeRecorder) RecordPublishDuration(topic string, _ time.Duration) {
	r.publishDurations++
	r.topics = append(r.topics, topic)
}

func (r *fakeRecorder) RecordMarkPublishedDuration(time.Duration) {
	r.markPublishedDurations++
}

func (r *fakeRecorder) AddClaimed(n int) {
	r.claimed += n
}

func (r *fakeRecorder) AddPublished(n int) {
	r.published += n
}

func (r *fakeRecorder) IncrementPublishFailure(topic string, errorClass string) {
	r.publishFailures++
	r.topics = append(r.topics, topic)
	r.errorClasses = append(r.errorClasses, errorClass)
}

func (r *fakeRecorder) IncrementMarkPublishedFailure() {
	r.markPublishedFailures++
}

func (r *fakeRecorder) ObserveBatchSize(n int) {
	r.batchSizes = append(r.batchSizes, n)
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

func TestDispatcher_BatchPublisherAndBatchMarker(t *testing.T) {
	ev1 := mustEvent(t, outbox.EventDecisionCompleted, "env-batch-a")
	ev2 := mustEvent(t, outbox.EventDecisionEscalated, "env-batch-b")
	ev3 := mustEvent(t, outbox.EventDecisionReviewResolved, "env-batch-c")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev1, ev2, ev3}}
	pub := &fakeBatchPublisher{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	if got := pub.batchCallCount.Load(); got != 1 {
		t.Fatalf("PublishBatch calls: want 1, got %d", got)
	}
	if len(pub.publishedBatches) != 1 || len(pub.publishedBatches[0]) != 3 {
		t.Fatalf("published batches: want one batch of 3, got %#v", pub.publishedBatches)
	}
	if len(repo.markBatchIDs) != 1 || len(repo.markBatchIDs[0]) != 3 {
		t.Fatalf("MarkPublishedBatch calls: want one batch of 3 IDs, got %#v", repo.markBatchIDs)
	}
	for _, ev := range []*outbox.OutboxEvent{ev1, ev2, ev3} {
		if ev.PublishedAt == nil {
			t.Fatalf("expected event %q to be marked published", ev.ID)
		}
	}
}

func TestDispatcher_GroupsBatchPublishesByTopic(t *testing.T) {
	ev1 := mustEvent(t, outbox.EventDecisionCompleted, "env-topic-a")
	ev2 := mustEvent(t, outbox.EventSurfaceApproved, "surface-topic-b")
	ev2.Topic = "midas.surfaces"
	ev3 := mustEvent(t, outbox.EventDecisionEscalated, "env-topic-c")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev1, ev2, ev3}}
	pub := &fakeBatchPublisher{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	if got := pub.batchCallCount.Load(); got != 2 {
		t.Fatalf("PublishBatch calls: want 2 topic groups, got %d", got)
	}
	if len(pub.publishedBatches) != 2 {
		t.Fatalf("published batch count: want 2, got %d", len(pub.publishedBatches))
	}
	if len(pub.publishedBatches[0]) != 2 || pub.publishedBatches[0][0].Topic != "midas.decisions" || pub.publishedBatches[0][1].Topic != "midas.decisions" {
		t.Fatalf("first topic group should contain two decision events, got %#v", pub.publishedBatches[0])
	}
	if len(pub.publishedBatches[1]) != 1 || pub.publishedBatches[1][0].Topic != "midas.surfaces" {
		t.Fatalf("second topic group should contain one surface event, got %#v", pub.publishedBatches[1])
	}
}

func TestDispatcher_RecordsClaimPublishAndMarkMetrics(t *testing.T) {
	ev1 := mustEvent(t, outbox.EventDecisionCompleted, "env-metrics-a")
	ev2 := mustEvent(t, outbox.EventDecisionEscalated, "env-metrics-b")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev1, ev2}}
	pub := &fakeBatchPublisher{}
	rec := &fakeRecorder{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig()).WithRecorder(rec)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	d.Run(ctx)

	if rec.claimDurations == 0 {
		t.Fatal("expected claim duration to be recorded")
	}
	if rec.claimed != 2 {
		t.Fatalf("claimed total: want 2, got %d", rec.claimed)
	}
	if rec.publishDurations != 1 {
		t.Fatalf("publish durations: want 1 topic batch, got %d", rec.publishDurations)
	}
	if rec.markPublishedDurations != 1 {
		t.Fatalf("mark-published durations: want 1 batch, got %d", rec.markPublishedDurations)
	}
	if rec.published != 2 {
		t.Fatalf("published total: want 2, got %d", rec.published)
	}
	if len(rec.batchSizes) == 0 || rec.batchSizes[0] != 2 {
		t.Fatalf("first observed batch size: want 2, got %v", rec.batchSizes)
	}
	if rec.publishFailures != 0 || rec.markPublishedFailures != 0 {
		t.Fatalf("unexpected failures recorded: publish=%d mark=%d", rec.publishFailures, rec.markPublishedFailures)
	}
}

func TestDispatcher_RecordsPublishFailureClass(t *testing.T) {
	ev := mustEvent(t, outbox.EventDecisionCompleted, "env-publish-failure")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev}}
	pub := &fakeBatchPublisher{batchErr: context.DeadlineExceeded}
	rec := &fakeRecorder{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig()).WithRecorder(rec)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	d.Run(ctx)

	if rec.publishFailures == 0 {
		t.Fatal("expected publish failure metric to be recorded")
	}
	if len(rec.errorClasses) == 0 || rec.errorClasses[0] != "context_deadline" {
		t.Fatalf("error class: want context_deadline, got %v", rec.errorClasses)
	}
	if rec.published != 0 {
		t.Fatalf("published total: want 0 after publish failure, got %d", rec.published)
	}
}

func TestDispatcher_PartialBatchPublishMarksOnlySuccessfulRows(t *testing.T) {
	ev1 := mustEvent(t, outbox.EventDecisionCompleted, "env-partial-a")
	ev2 := mustEvent(t, outbox.EventDecisionEscalated, "env-partial-b")
	ev3 := mustEvent(t, outbox.EventDecisionReviewResolved, "env-partial-c")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev1, ev2, ev3}}
	pub := &fakeBatchPublisher{
		failKeys: map[string]error{string(ev2.EventKey): errors.New("broker rejected message")},
	}
	rec := &fakeRecorder{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig()).WithRecorder(rec)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	if ev1.PublishedAt == nil || ev3.PublishedAt == nil {
		t.Fatal("expected successful batch messages to be marked published")
	}
	if ev2.PublishedAt != nil {
		t.Fatal("expected failed batch message to remain unpublished")
	}
	if rec.published != 2 {
		t.Fatalf("published total: want 2 successful events, got %d", rec.published)
	}
	if rec.publishFailures == 0 {
		t.Fatal("expected publish failure metric for failed message")
	}
}

func TestDispatcher_BatchMarkMismatchWarnsAndRecordsFailure(t *testing.T) {
	ev1 := mustEvent(t, outbox.EventDecisionCompleted, "env-mismatch-a")
	ev2 := mustEvent(t, outbox.EventDecisionEscalated, "env-mismatch-b")
	affected := int64(1)
	repo := &fakeRepo{
		events:           []*outbox.OutboxEvent{ev1, ev2},
		affectedOverride: &affected,
	}
	pub := &fakeBatchPublisher{}
	rec := &fakeRecorder{}
	d := newDispatcher(t, repo, pub, shortIntervalConfig()).WithRecorder(rec)

	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(oldLogger)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	logs := buf.String()
	if !strings.Contains(logs, "outbox_mark_published_mismatch") {
		t.Fatalf("expected mark-published mismatch warning, got:\n%s", logs)
	}
	if rec.markPublishedFailures == 0 {
		t.Fatal("expected mark-published failure metric on affected-count mismatch")
	}
}

func TestDispatcher_SerialPublishFallbackStillWorks(t *testing.T) {
	ev1 := mustEvent(t, outbox.EventDecisionCompleted, "env-fallback-a")
	ev2 := mustEvent(t, outbox.EventDecisionEscalated, "env-fallback-b")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev1, ev2}}
	pub := &fakePublisher{}

	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	if got := pub.callCount.Load(); got != 2 {
		t.Fatalf("serial Publish calls: want 2, got %d", got)
	}
	if len(repo.markBatchIDs) != 1 || len(repo.markBatchIDs[0]) != 2 {
		t.Fatalf("batch marker should still be used after serial publish fallback, got %#v", repo.markBatchIDs)
	}
}

func TestDispatcher_DoesNotInfoLogPerEventSuccess(t *testing.T) {
	ev1 := mustEvent(t, outbox.EventDecisionCompleted, "env-log-a")
	ev2 := mustEvent(t, outbox.EventDecisionEscalated, "env-log-b")
	repo := &fakeRepo{events: []*outbox.OutboxEvent{ev1, ev2}}
	pub := &fakePublisher{}
	d := newDispatcher(t, repo, pub, shortIntervalConfig())

	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(oldLogger)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	logs := buf.String()
	if !strings.Contains(logs, "outbox_batch_dispatched") {
		t.Fatalf("expected batch-level info log, got:\n%s", logs)
	}
	if strings.Contains(logs, "outbox_event_dispatched") {
		t.Fatalf("per-event success log must not be emitted at info level:\n%s", logs)
	}
	if !strings.Contains(logs, "dispatcher_instance_id=") || !strings.Contains(logs, "batch_id=") {
		t.Fatalf("expected dispatcher and batch correlation fields, got:\n%s", logs)
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
