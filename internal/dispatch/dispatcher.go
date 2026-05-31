package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/accept-io/midas/internal/outbox"
)

// Dispatcher polls the outbox for unpublished rows, publishes them to the
// configured broker, and marks each row published only after the broker
// acknowledges receipt.
//
// Delivery semantics are at-least-once: if the process crashes between a
// successful broker publish and the MarkPublished write, the row will be
// re-claimed on the next poll and re-published. Consumer-side idempotency
// is assumed.
type Dispatcher struct {
	repo       DispatcherRepo
	publisher  Publisher
	cfg        DispatcherConfig
	recorder   Recorder
	instanceID string
}

var dispatcherInstanceSeq atomic.Uint64

// NewDispatcher constructs a Dispatcher. All arguments must be non-nil and
// DispatcherConfig.BatchSize and DispatcherConfig.PollInterval must be
// positive.
func NewDispatcher(repo DispatcherRepo, publisher Publisher, cfg DispatcherConfig) (*Dispatcher, error) {
	if repo == nil {
		return nil, errNilArg("repo")
	}
	if publisher == nil {
		return nil, errNilArg("publisher")
	}
	if cfg.BatchSize <= 0 {
		return nil, errInvalidCfg("BatchSize must be > 0")
	}
	if cfg.PollInterval <= 0 {
		return nil, errInvalidCfg("PollInterval must be > 0")
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = cfg.PollInterval
	}
	return &Dispatcher{
		repo:       repo,
		publisher:  publisher,
		cfg:        cfg,
		recorder:   noopRecorder{},
		instanceID: newDispatcherInstanceID(),
	}, nil
}

// WithRecorder wires an optional metrics recorder into the dispatcher.
func (d *Dispatcher) WithRecorder(rec Recorder) *Dispatcher {
	if rec == nil {
		d.recorder = noopRecorder{}
		return d
	}
	d.recorder = rec
	return d
}

// Run starts the dispatch loop and blocks until ctx is cancelled. It returns
// when the context is done; any cleanup (e.g. broker connection close) is the
// caller's responsibility.
//
// Run is safe to call from a goroutine. The typical pattern:
//
//	go dispatcher.Run(ctx)
func (d *Dispatcher) Run(ctx context.Context) {
	slog.Info("outbox_dispatcher_started",
		"dispatcher_instance_id", d.instanceID,
		"batch_size", d.cfg.BatchSize,
		"poll_interval", d.cfg.PollInterval,
		"max_backoff", d.cfg.MaxBackoff,
	)

	backoff := d.cfg.PollInterval

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox_dispatcher_stopped")
			return
		default:
		}

		processed, err := d.poll(ctx)
		if err != nil {
			slog.Error("outbox_dispatcher_poll_error",
				"dispatcher_instance_id", d.instanceID,
				"error", err,
				"backoff", backoff,
			)
			backoff = min(backoff*2, d.cfg.MaxBackoff)
			select {
			case <-ctx.Done():
				slog.Info("outbox_dispatcher_stopped")
				return
			case <-time.After(backoff):
			}
			continue
		}

		// Reset backoff after any successful poll (including empty).
		backoff = d.cfg.PollInterval

		if processed == 0 {
			// Queue is empty or all publishes failed; sleep before the next poll.
			// Sleeping when publishes fail prevents tight-looping against a
			// degraded broker.
			select {
			case <-ctx.Done():
				slog.Info("outbox_dispatcher_stopped")
				return
			case <-time.After(d.cfg.PollInterval):
			}
			continue
		}

		// At least one event was dispatched successfully. Loop immediately in
		// case the batch was full and more events await.
	}
}

// poll claims one batch of unpublished events, publishes them grouped by topic,
// and marks successfully acknowledged rows. Returns the number of events
// successfully published to the broker (not merely claimed). A return value of
// zero means the queue was empty or all publishes failed; the caller should
// sleep before the next poll.
func (d *Dispatcher) poll(ctx context.Context) (int, error) {
	batchID := newBatchID()
	claimStart := time.Now()
	events, err := d.repo.ClaimUnpublished(ctx, d.cfg.BatchSize)
	d.recorder.RecordClaimDuration(time.Since(claimStart))
	if err != nil {
		return 0, err
	}
	d.recorder.AddClaimed(len(events))
	d.recorder.ObserveBatchSize(len(events))
	if len(events) == 0 {
		return 0, nil
	}

	published := 0
	for _, group := range groupByTopic(events) {
		published += d.dispatchGroup(ctx, batchID, group)
	}

	if published > 0 {
		slog.Info("outbox_batch_dispatched",
			"dispatcher_instance_id", d.instanceID,
			"batch_id", batchID,
			"count", published,
			"claimed", len(events),
		)
	}

	return published, nil
}

// dispatchGroup publishes one topic-homogeneous group and marks successfully
// published rows in one batch where the repository supports it. Returns the
// number of events successfully acknowledged by the broker, regardless of
// whether marking them published succeeds.
//
// Errors at each stage are logged but never propagated: a publish failure leaves
// the row unpublished for the next poll cycle; a mark-published failure after a
// successful publish is logged and accepted as a potential duplicate (at-least-once).
func (d *Dispatcher) dispatchGroup(ctx context.Context, batchID string, events []*outbox.OutboxEvent) int {
	if len(events) == 0 {
		return 0
	}

	topic := events[0].Topic
	msgs := make([]Message, len(events))
	for i, ev := range events {
		msgs[i] = eventToMessage(ev)
	}

	publishStart := time.Now()
	result, err := d.publishBatch(ctx, msgs)
	d.recorder.RecordPublishDuration(topic, time.Since(publishStart))
	if err != nil {
		d.logPublishFailures(batchID, events, result, err)
	}
	if len(result.Successful) == 0 {
		return 0
	}
	d.recorder.AddPublished(len(result.Successful))

	publishedIDs := make([]string, 0, len(result.Successful))
	for _, idx := range result.Successful {
		if idx < 0 || idx >= len(events) {
			continue
		}
		publishedIDs = append(publishedIDs, events[idx].ID)
	}

	markStart := time.Now()
	affected, err := d.markPublishedBatch(ctx, publishedIDs)
	d.recorder.RecordMarkPublishedDuration(time.Since(markStart))
	if err != nil {
		d.recorder.IncrementMarkPublishedFailure()
		// The message was delivered to the broker but the database write failed.
		// The row will be re-claimed and re-published (at-least-once delivery).
		// Consumers must tolerate duplicates. WARN because this is recoverable
		// and expected under at-least-once semantics.
		slog.Warn("outbox_mark_published_failed",
			"dispatcher_instance_id", d.instanceID,
			"batch_id", batchID,
			"topic", topic,
			"count", len(publishedIDs),
			"error", err,
		)
		return len(result.Successful)
	}
	if affected != int64(len(publishedIDs)) {
		d.recorder.IncrementMarkPublishedFailure()
		slog.Warn("outbox_mark_published_mismatch",
			"dispatcher_instance_id", d.instanceID,
			"batch_id", batchID,
			"topic", topic,
			"expected_count", len(publishedIDs),
			"affected_count", affected,
		)
	}

	slog.Debug("outbox_batch_marked_published",
		"dispatcher_instance_id", d.instanceID,
		"batch_id", batchID,
		"topic", topic,
		"count", len(publishedIDs),
	)
	return len(result.Successful)
}

func (d *Dispatcher) publishBatch(ctx context.Context, msgs []Message) (PublishBatchResult, error) {
	if len(msgs) == 0 {
		return PublishBatchResult{}, nil
	}
	if p, ok := d.publisher.(BatchPublisher); ok {
		return p.PublishBatch(ctx, msgs)
	}

	result := PublishBatchResult{
		Successful: make([]int, 0, len(msgs)),
	}
	var firstErr error
	for i, msg := range msgs {
		if err := d.publisher.Publish(ctx, msg); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			result.Failed = append(result.Failed, PublishBatchFailure{Index: i, Err: err})
			continue
		}
		result.Successful = append(result.Successful, i)
	}
	return result, firstErr
}

func (d *Dispatcher) markPublishedBatch(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if repo, ok := d.repo.(BatchMarker); ok {
		return repo.MarkPublishedBatch(ctx, ids)
	}

	var affected int64
	for _, id := range ids {
		if err := d.repo.MarkPublished(ctx, id); err != nil {
			return affected, err
		}
		affected++
	}
	return affected, nil
}

func (d *Dispatcher) logPublishFailures(batchID string, events []*outbox.OutboxEvent, result PublishBatchResult, batchErr error) {
	failures := result.Failed
	if len(failures) == 0 && batchErr != nil {
		failures = make([]PublishBatchFailure, len(events))
		for i := range events {
			failures[i] = PublishBatchFailure{Index: i, Err: batchErr}
		}
	}
	for _, failure := range failures {
		if failure.Index < 0 || failure.Index >= len(events) {
			continue
		}
		ev := events[failure.Index]
		err := failure.Err
		if err == nil {
			err = batchErr
		}
		d.recorder.IncrementPublishFailure(ev.Topic, classifyError(err))
		// Publish failures are transient: the row remains unpublished and will
		// be re-claimed on the next poll cycle. WARN rather than ERROR because
		// a degraded broker is expected to recover.
		slog.Warn("outbox_publish_failed",
			"dispatcher_instance_id", d.instanceID,
			"batch_id", batchID,
			"event_id", ev.ID,
			"event_type", ev.EventType,
			"aggregate_type", ev.AggregateType,
			"aggregate_id", ev.AggregateID,
			"topic", ev.Topic,
			"error", err,
		)
	}
}

func groupByTopic(events []*outbox.OutboxEvent) [][]*outbox.OutboxEvent {
	if len(events) == 0 {
		return nil
	}
	order := make([]string, 0, len(events))
	groups := make(map[string][]*outbox.OutboxEvent, len(events))
	for _, ev := range events {
		if _, ok := groups[ev.Topic]; !ok {
			order = append(order, ev.Topic)
		}
		groups[ev.Topic] = append(groups[ev.Topic], ev)
	}

	out := make([][]*outbox.OutboxEvent, 0, len(order))
	for _, topic := range order {
		out = append(out, groups[topic])
	}
	return out
}

func newDispatcherInstanceID() string {
	return fmt.Sprintf("dispatcher-%d-%d", time.Now().UTC().UnixNano(), dispatcherInstanceSeq.Add(1))
}

func newBatchID() string {
	return fmt.Sprintf("batch-%d", time.Now().UTC().UnixNano())
}

func classifyError(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline"
	default:
		return "publish_error"
	}
}

// eventToMessage converts an outbox row into the broker-agnostic Message type.
// Routing metadata is carried in headers so that consumers can inspect it
// without deserialising the payload.
func eventToMessage(ev *outbox.OutboxEvent) Message {
	headers := []Header{
		{Key: "event_type", Value: []byte(ev.EventType)},
		{Key: "aggregate_type", Value: []byte(ev.AggregateType)},
		{Key: "aggregate_id", Value: []byte(ev.AggregateID)},
	}

	var key []byte
	if ev.EventKey != "" {
		key = []byte(ev.EventKey)
	}

	return Message{
		Topic:   ev.Topic,
		Key:     key,
		Value:   []byte(ev.Payload),
		Headers: headers,
	}
}
