package kafka

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"

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
	writer        *kafkago.Writer
	writeMessages func(ctx context.Context, msgs ...kafkago.Message) error
	closeWriter   func() error
}

// NewKafkaPublisher constructs a KafkaPublisher from cfg. cfg.Brokers must be
// non-empty; all other fields are optional.
//
// Config field mapping to kafka-go v0.4.50:
//   - Brokers       → kafka.Writer.Addr (via kafka.TCP)
//   - RequiredAcks  → kafka.Writer.RequiredAcks
//   - WriteTimeout  → kafka.Writer.WriteTimeout (applied when > 0)
//   - ClientID      → kafka.Transport.ClientID (Transport is set on the Writer)
//   - TLSEnabled    → kafka.Transport.TLS
//   - SASL plain    → kafka.Transport.SASL
//
// The returned publisher is safe for concurrent use. Call Close when the
// publisher is no longer needed to flush pending writes and release resources.
func NewKafkaPublisher(cfg KafkaConfig) (*KafkaPublisher, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka: at least one broker address is required")
	}
	if cfg.SASLUsername != "" && cfg.SASLPassword == "" {
		return nil, fmt.Errorf("kafka: SASL password is required when username is set")
	}
	if cfg.SASLPassword != "" && cfg.SASLUsername == "" {
		return nil, fmt.Errorf("kafka: SASL username is required when password is set")
	}
	if cfg.SASLMechanism != "" && (cfg.SASLUsername == "" || cfg.SASLPassword == "") {
		return nil, fmt.Errorf("kafka: SASL username and password are required when mechanism is set")
	}

	// ClientID is carried on the Transport, not on Writer directly. A Transport
	// value with ClientID set is created unconditionally so the writer always
	// identifies itself to the broker, even when ClientID is empty string.
	transport := &kafkago.Transport{
		ClientID: cfg.ClientID,
	}
	if cfg.TLSEnabled {
		transport.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	if cfg.SASLUsername != "" || cfg.SASLPassword != "" || cfg.SASLMechanism != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.SASLMechanism)) {
		case "plain":
			transport.SASL = plain.Mechanism{
				Username: cfg.SASLUsername,
				Password: cfg.SASLPassword,
			}
		default:
			return nil, fmt.Errorf("kafka: unsupported SASL mechanism %q (supported: plain)", cfg.SASLMechanism)
		}
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

	return &KafkaPublisher{
		writer:        w,
		writeMessages: w.WriteMessages,
		closeWriter:   w.Close,
	}, nil
}

// Publish sends msg to the Kafka topic named in msg.Topic. It blocks until the
// broker acknowledges receipt or ctx is cancelled.
//
// A non-nil error means the message may or may not have been delivered. The
// dispatcher will leave the outbox row unpublished and retry on the next cycle.
func (p *KafkaPublisher) Publish(ctx context.Context, msg dispatch.Message) error {
	if err := p.write(ctx, toKafkaMessage(msg)); err != nil {
		return fmt.Errorf("kafka: publish to topic %q: %w", msg.Topic, err)
	}
	return nil
}

// PublishBatch sends msgs to Kafka with one WriteMessages call. kafka-go
// exposes partial failures as kafka.WriteErrors; those are mapped back to input
// indexes so the dispatcher can mark only acknowledged messages as published.
// For non-partial errors, success cannot be determined, so every message is
// left unconfirmed for retry.
func (p *KafkaPublisher) PublishBatch(ctx context.Context, msgs []dispatch.Message) (dispatch.PublishBatchResult, error) {
	result := dispatch.PublishBatchResult{}
	if len(msgs) == 0 {
		return result, nil
	}

	kmsgs := make([]kafkago.Message, len(msgs))
	for i, msg := range msgs {
		kmsgs[i] = toKafkaMessage(msg)
	}

	err := p.write(ctx, kmsgs...)
	if err == nil {
		result.Successful = make([]int, len(msgs))
		for i := range msgs {
			result.Successful[i] = i
		}
		return result, nil
	}

	var writeErrs kafkago.WriteErrors
	if errors.As(err, &writeErrs) && len(writeErrs) == len(msgs) {
		for i, writeErr := range writeErrs {
			if writeErr == nil {
				result.Successful = append(result.Successful, i)
				continue
			}
			result.Failed = append(result.Failed, dispatch.PublishBatchFailure{Index: i, Err: writeErr})
		}
		return result, fmt.Errorf("kafka: publish batch: %w", err)
	}

	result.Failed = make([]dispatch.PublishBatchFailure, len(msgs))
	for i := range msgs {
		result.Failed[i] = dispatch.PublishBatchFailure{Index: i, Err: err}
	}
	return result, fmt.Errorf("kafka: publish batch: %w", err)
}

// Close flushes any pending writes and closes the underlying Kafka connection.
// It should be called when the publisher is no longer needed.
func (p *KafkaPublisher) Close() error {
	closeFn := p.closeWriter
	if closeFn == nil && p.writer != nil {
		closeFn = p.writer.Close
	}
	if closeFn == nil {
		return nil
	}
	if err := closeFn(); err != nil {
		return fmt.Errorf("kafka: close writer: %w", err)
	}
	return nil
}

func (p *KafkaPublisher) write(ctx context.Context, msgs ...kafkago.Message) error {
	writeFn := p.writeMessages
	if writeFn == nil && p.writer != nil {
		writeFn = p.writer.WriteMessages
	}
	if writeFn == nil {
		return fmt.Errorf("kafka: writer is not configured")
	}
	return writeFn(ctx, msgs...)
}

func toKafkaMessage(msg dispatch.Message) kafkago.Message {
	return kafkago.Message{
		Topic:   msg.Topic,
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: toKafkaHeaders(msg.Headers),
	}
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
var _ dispatch.BatchPublisher = (*KafkaPublisher)(nil)
