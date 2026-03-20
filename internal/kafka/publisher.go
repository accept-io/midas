package kafka

import (
	"context"
	"fmt"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/accept-io/midas/internal/dispatch"
)

// KafkaPublisher implements dispatch.Publisher using segmentio/kafka-go.
//
// Each Publish call writes one message to the configured Kafka topic using the
// writer's internal batching. The writer is created per-topic by kafka-go's
// transport layer; callers do not need to manage per-topic writers explicitly.
//
// Publish blocks until the broker acknowledges receipt according to the
// RequiredAcks setting or until ctx is cancelled.
type KafkaPublisher struct {
	writer *kafkago.Writer
}

// NewKafkaPublisher constructs a KafkaPublisher from cfg. cfg.Brokers must be
// non-empty; all other fields are optional.
//
// Config field mapping to kafka-go v0.4.50:
//   - Brokers       → kafka.Writer.Addr (via kafka.TCP)
//   - RequiredAcks  → kafka.Writer.RequiredAcks
//   - WriteTimeout  → kafka.Writer.WriteTimeout (applied when > 0)
//   - ClientID      → kafka.Transport.ClientID (Transport is set on the Writer)
//
// The returned publisher is safe for concurrent use. Call Close when the
// publisher is no longer needed to flush pending writes and release resources.
func NewKafkaPublisher(cfg KafkaConfig) (*KafkaPublisher, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka: at least one broker address is required")
	}

	// ClientID is carried on the Transport, not on Writer directly. A Transport
	// value with ClientID set is created unconditionally so the writer always
	// identifies itself to the broker, even when ClientID is empty string.
	transport := &kafkago.Transport{
		ClientID: cfg.ClientID,
	}

	w := &kafkago.Writer{
		Addr:                   kafkago.TCP(cfg.Brokers...),
		RequiredAcks:           kafkago.RequiredAcks(cfg.RequiredAcks),
		AllowAutoTopicCreation: false,
		Transport:              transport,
	}

	if cfg.WriteTimeout > 0 {
		w.WriteTimeout = cfg.WriteTimeout
	}

	return &KafkaPublisher{writer: w}, nil
}

// Publish sends msg to the Kafka topic named in msg.Topic. It blocks until the
// broker acknowledges receipt or ctx is cancelled.
//
// A non-nil error means the message may or may not have been delivered. The
// dispatcher will leave the outbox row unpublished and retry on the next cycle.
func (p *KafkaPublisher) Publish(ctx context.Context, msg dispatch.Message) error {
	km := kafkago.Message{
		Topic:   msg.Topic,
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: toKafkaHeaders(msg.Headers),
	}

	if err := p.writer.WriteMessages(ctx, km); err != nil {
		return fmt.Errorf("kafka: publish to topic %q: %w", msg.Topic, err)
	}
	return nil
}

// Close flushes any pending writes and closes the underlying Kafka connection.
// It should be called when the publisher is no longer needed.
func (p *KafkaPublisher) Close() error {
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("kafka: close writer: %w", err)
	}
	return nil
}

// toKafkaHeaders converts dispatch.Header slice to kafka-go's Header slice.
func toKafkaHeaders(headers []dispatch.Header) []kafkago.Header {
	if len(headers) == 0 {
		return nil
	}
	out := make([]kafkago.Header, len(headers))
	for i, h := range headers {
		out[i] = kafkago.Header{
			Key:   h.Key,
			Value: h.Value,
		}
	}
	return out
}

// Ensure KafkaPublisher satisfies the Publisher interface at compile time.
var _ dispatch.Publisher = (*KafkaPublisher)(nil)
