package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PublisherType identifies which message broker implementation the dispatcher
// will use. When DispatcherConfig.Enabled is true, a real publisher must be
// configured; "none" is only valid when the dispatcher is disabled.
type PublisherType string

const (
	// PublisherTypeNone is the zero value. Valid only when
	// DispatcherConfig.Enabled is false; it signals that no outbound transport
	// is configured. Validate rejects this value when Enabled is true.
	PublisherTypeNone PublisherType = "none"

	// PublisherTypeKafka wires the Kafka publisher. Requires non-empty
	// KafkaConfig.Brokers.
	PublisherTypeKafka PublisherType = "kafka"
)

// DispatcherConfig controls whether the outbox dispatcher runs and how it
// behaves. All durations are wall-clock values; zero means use the default.
type DispatcherConfig struct {
	// Enabled controls whether the dispatcher goroutine is started.
	// When false the outbox is written but never polled.
	Enabled bool

	// Publisher selects the broker implementation. Ignored when Enabled is false.
	Publisher PublisherType

	// BatchSize is the maximum number of outbox rows claimed per poll cycle.
	// Defaults to 100.
	BatchSize int

	// PollInterval is the sleep duration between poll cycles when the queue
	// is empty. Defaults to 2 seconds.
	PollInterval time.Duration

	// MaxBackoff is the upper bound for exponential sleep on consecutive poll
	// errors. Defaults to 30 seconds.
	MaxBackoff time.Duration
}

// KafkaConfig holds broker-level settings for the Kafka publisher. Only
// meaningful when DispatcherConfig.Publisher == PublisherTypeKafka.
type KafkaConfig struct {
	// Brokers is the list of seed broker addresses in "host:port" form.
	// Required when PublisherType is "kafka".
	Brokers []string

	// ClientID is an optional identifier sent to the broker for observability.
	ClientID string

	// RequiredAcks controls acknowledgement level: -1=all ISRs, 0=none, 1=leader.
	// Defaults to -1 (strongest).
	RequiredAcks int

	// WriteTimeout bounds the per-message publish call. Zero means no timeout.
	WriteTimeout time.Duration
}

// AppConfig is the top-level runtime configuration for a MIDAS process.
type AppConfig struct {
	Dispatcher DispatcherConfig
	Kafka      KafkaConfig
}

// ErrInvalidConfig is returned when AppConfig.Validate finds an inconsistent
// or missing required field.
var ErrInvalidConfig = errors.New("config: invalid configuration")

// Validate returns ErrInvalidConfig (wrapped) for any configuration that
// cannot lead to a valid runtime state. Validation is purely structural and
// does not open network connections.
//
// Dispatcher semantics:
//   - DispatcherEnabled=false: always passes regardless of publisher fields.
//   - DispatcherEnabled=true: a real publisher must be configured; "none" and
//     empty publisher values are rejected because there is no valid runtime
//     state where the dispatcher is enabled but has no outbound transport.
func (c AppConfig) Validate() error {
	if !c.Dispatcher.Enabled {
		// Dispatcher off — publisher fields are irrelevant.
		return nil
	}

	switch c.Dispatcher.Publisher {
	case PublisherTypeNone, "":
		return fmt.Errorf(
			"%w: dispatcher enabled but no publisher configured",
			ErrInvalidConfig,
		)

	case PublisherTypeKafka:
		if len(c.Kafka.Brokers) == 0 {
			return fmt.Errorf(
				"%w: dispatcher enabled with kafka publisher but no brokers provided",
				ErrInvalidConfig,
			)
		}
		return nil

	default:
		return fmt.Errorf(
			"%w: unsupported dispatcher publisher: %s",
			ErrInvalidConfig,
			c.Dispatcher.Publisher,
		)
	}
}

// LoadAppConfig reads AppConfig from environment variables. Unset variables
// fall back to safe defaults. Validation is separate (call Validate).
//
// Environment variables:
//
//	MIDAS_DISPATCHER_ENABLED       bool   (default: false)
//	MIDAS_DISPATCHER_PUBLISHER     string (default: "none"; valid: "none", "kafka")
//	MIDAS_DISPATCHER_BATCH_SIZE    int    (default: 100)
//	MIDAS_DISPATCHER_POLL_INTERVAL string (default: "2s"; Go duration)
//	MIDAS_DISPATCHER_MAX_BACKOFF   string (default: "30s"; Go duration)
//
//	MIDAS_KAFKA_BROKERS            string (comma-separated host:port; required when publisher=kafka)
//	MIDAS_KAFKA_CLIENT_ID          string (default: "midas")
//	MIDAS_KAFKA_REQUIRED_ACKS      int    (default: -1)
//	MIDAS_KAFKA_WRITE_TIMEOUT      string (default: ""; zero means no timeout)
func LoadAppConfig() (AppConfig, error) {
	cfg := AppConfig{
		Dispatcher: DispatcherConfig{
			Enabled:      false,
			Publisher:    PublisherTypeNone,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: KafkaConfig{
			ClientID:     "midas",
			RequiredAcks: -1,
		},
	}

	if v := os.Getenv("MIDAS_DISPATCHER_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return AppConfig{}, fmt.Errorf(
				"%w: MIDAS_DISPATCHER_ENABLED must be a boolean (true/false): %v",
				ErrInvalidConfig, err,
			)
		}
		cfg.Dispatcher.Enabled = b
	}

	if v := os.Getenv("MIDAS_DISPATCHER_PUBLISHER"); v != "" {
		cfg.Dispatcher.Publisher = PublisherType(strings.ToLower(strings.TrimSpace(v)))
	}

	if v := os.Getenv("MIDAS_DISPATCHER_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return AppConfig{}, fmt.Errorf(
				"%w: MIDAS_DISPATCHER_BATCH_SIZE must be a positive integer: %q",
				ErrInvalidConfig, v,
			)
		}
		cfg.Dispatcher.BatchSize = n
	}

	if v := os.Getenv("MIDAS_DISPATCHER_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return AppConfig{}, fmt.Errorf(
				"%w: MIDAS_DISPATCHER_POLL_INTERVAL must be a positive Go duration (e.g. 2s): %q",
				ErrInvalidConfig, v,
			)
		}
		cfg.Dispatcher.PollInterval = d
	}

	if v := os.Getenv("MIDAS_DISPATCHER_MAX_BACKOFF"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return AppConfig{}, fmt.Errorf(
				"%w: MIDAS_DISPATCHER_MAX_BACKOFF must be a positive Go duration (e.g. 30s): %q",
				ErrInvalidConfig, v,
			)
		}
		cfg.Dispatcher.MaxBackoff = d
	}

	if v := os.Getenv("MIDAS_KAFKA_BROKERS"); v != "" {
		for _, b := range strings.Split(v, ",") {
			b = strings.TrimSpace(b)
			if b != "" {
				cfg.Kafka.Brokers = append(cfg.Kafka.Brokers, b)
			}
		}
	}

	if v := os.Getenv("MIDAS_KAFKA_CLIENT_ID"); v != "" {
		cfg.Kafka.ClientID = strings.TrimSpace(v)
	}

	if v := os.Getenv("MIDAS_KAFKA_REQUIRED_ACKS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n != -1 && n != 0 && n != 1) {
			return AppConfig{}, fmt.Errorf(
				"%w: MIDAS_KAFKA_REQUIRED_ACKS must be -1, 0, or 1: %q",
				ErrInvalidConfig, v,
			)
		}
		cfg.Kafka.RequiredAcks = n
	}

	if v := os.Getenv("MIDAS_KAFKA_WRITE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return AppConfig{}, fmt.Errorf(
				"%w: MIDAS_KAFKA_WRITE_TIMEOUT must be a non-negative Go duration (e.g. 10s): %q",
				ErrInvalidConfig, v,
			)
		}
		cfg.Kafka.WriteTimeout = d
	}

	return cfg, nil
}
