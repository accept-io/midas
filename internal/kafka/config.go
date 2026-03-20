// Package kafka provides a Kafka-backed Publisher for the outbox dispatcher.
//
// This package wraps github.com/segmentio/kafka-go and is the only place in
// the codebase that imports Kafka client code. All other packages depend only
// on the dispatch.Publisher interface.
package kafka

import "time"

// KafkaConfig holds the broker-level configuration for KafkaPublisher.
type KafkaConfig struct {
	// Brokers is the list of seed broker addresses in "host:port" form.
	// At least one address is required.
	Brokers []string

	// ClientID is an optional string sent to the broker on every connection
	// for observability (visible in broker logs and metrics). Applied to the
	// kafka-go Transport.ClientID field.
	ClientID string

	// RequiredAcks controls the acknowledgement level:
	//   -1 = all in-sync replicas (strongest, default)
	//    0 = no acknowledgement (fire and forget; not recommended)
	//    1 = leader only
	//
	// For outbox delivery RequiredAcksAll (-1) is recommended because it
	// provides the strongest durability guarantee before MarkPublished is called.
	RequiredAcks RequiredAcks

	// WriteTimeout bounds each WriteMessages call. Zero means no per-write
	// timeout; context cancellation remains the only deadline in that case.
	WriteTimeout time.Duration
}

// RequiredAcks mirrors the kafka-go RequiredAcks type with named constants.
type RequiredAcks int

const (
	// RequiredAcksNone instructs the broker to not acknowledge the write.
	// Messages may be lost on leader failure. Not recommended for outbox use.
	RequiredAcksNone RequiredAcks = 0

	// RequiredAcksLeader instructs the broker to acknowledge after the leader
	// has written to its log. Provides reasonable durability without full ISR sync.
	RequiredAcksLeader RequiredAcks = 1

	// RequiredAcksAll instructs the broker to acknowledge only after all
	// in-sync replicas have written the message. Strongest durability guarantee.
	RequiredAcksAll RequiredAcks = -1
)

// DefaultKafkaConfig returns production-safe defaults.
func DefaultKafkaConfig() KafkaConfig {
	return KafkaConfig{
		RequiredAcks: RequiredAcksAll,
	}
}
