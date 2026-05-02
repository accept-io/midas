package dispatch

import (
	"context"
	"log/slog"
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
	repo      DispatcherRepo
	publisher Publisher
	cfg       DispatcherConfig
}

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
		repo:      repo,
		publisher: publisher,
		cfg:       cfg,
	}, nil
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

// poll claims one batch of unpublished events, publishes each, and marks
// published ones. Returns the number of events successfully published (not
// merely claimed). A return value of zero means the queue was empty or all
// publishes failed; the caller should sleep before the next poll.
func (d *Dispatcher) poll(ctx context.Context) (int, error) {
	events, err := d.repo.ClaimUnpublished(ctx, d.cfg.BatchSize)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}

	published := 0
	for _, ev := range events {
		if d.dispatch(ctx, ev) {
			published++
		}
	}

	if published > 0 {
		slog.Info("outbox_batch_dispatched", "count", published, "claimed", len(events))
	}

	return published, nil
}

// dispatch publishes a single outbox event and, on success, marks it published.
// Returns true if the event was successfully published to the broker (regardless
// of whether MarkPublished succeeded). Returns false if the publish failed.
//
// Errors at each stage are logged but never propagated: a publish failure leaves
// the row unpublished for the next poll cycle; a mark-published failure after a
// successful publish is logged and accepted as a potential duplicate (at-least-once).
func (d *Dispatcher) dispatch(ctx context.Context, ev *outbox.OutboxEvent) bool {
	msg := eventToMessage(ev)

	if err := d.publisher.Publish(ctx, msg); err != nil {
		// Publish failures are transient: the row remains unpublished and will
		// be re-claimed on the next poll cycle. WARN rather than ERROR because
		// a degraded broker is expected to recover.
		slog.Warn("outbox_publish_failed",
			"event_id", ev.ID,
			"event_type", ev.EventType,
			"aggregate_type", ev.AggregateType,
			"aggregate_id", ev.AggregateID,
			"topic", ev.Topic,
			"error", err,
		)
		return false
	}

	if err := d.repo.MarkPublished(ctx, ev.ID); err != nil {
		// The message was delivered to the broker but the database write failed.
		// The row will be re-claimed and re-published (at-least-once delivery).
		// Consumers must tolerate duplicates. WARN because this is recoverable
		// and expected under at-least-once semantics.
		slog.Warn("outbox_mark_published_failed",
			"event_id", ev.ID,
			"event_type", ev.EventType,
			"aggregate_type", ev.AggregateType,
			"aggregate_id", ev.AggregateID,
			"topic", ev.Topic,
			"error", err,
		)
		// Return true: the broker received the message. The dispatcher should
		// not sleep as if the queue was empty; there may be more events to send.
		return true
	}

	slog.Info("outbox_event_dispatched",
		"event_id", ev.ID,
		"event_type", ev.EventType,
		"aggregate_type", ev.AggregateType,
		"aggregate_id", ev.AggregateID,
		"topic", ev.Topic,
	)
	return true
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
