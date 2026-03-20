package bootstrap

import (
	"fmt"

	"github.com/accept-io/midas/internal/dispatch"
	"github.com/accept-io/midas/internal/kafka"
	"github.com/accept-io/midas/internal/outbox"
)

// DispatcherWiring holds everything the caller needs to run and shut down the
// outbox dispatcher. Fields are nil only when the dispatcher is disabled
// (cfg.Dispatcher.Enabled == false).
type DispatcherWiring struct {
	// Dispatcher is the configured Dispatcher, or nil when the dispatcher is
	// disabled.
	Dispatcher *dispatch.Dispatcher

	// KafkaPublisher is the Kafka-backed publisher, or nil when the publisher
	// type is not "kafka". Caller must call Close() during shutdown.
	KafkaPublisher *kafka.KafkaPublisher
}

// BuildDispatcher constructs a Dispatcher and, when required, a KafkaPublisher
// from the supplied AppConfig. repo is the outbox.Repository the dispatcher
// will poll.
//
// Returns a DispatcherWiring with nil Dispatcher only when
// cfg.Dispatcher.Enabled is false. Every other code path either returns a
// fully wired Dispatcher or a non-nil error.
//
// Callers must call cfg.Validate() before BuildDispatcher to catch invalid
// config combinations early. BuildDispatcher does not duplicate that
// validation, but it does enforce the repo requirement which Validate cannot
// check (repo availability depends on the selected store backend).
//
// Error cases when Enabled is true:
//   - publisher is "none" or empty: no valid outbound transport
//   - publisher is an unrecognised value
//   - repo is nil: no durable outbox repository is available
//   - publisher=kafka and broker construction fails
//   - dispatcher construction fails (invalid batch size or poll interval)
func BuildDispatcher(cfg AppConfig, repo outbox.Repository) (*DispatcherWiring, error) {
	wiring := &DispatcherWiring{}

	if !cfg.Dispatcher.Enabled {
		// Dispatcher is off. No wiring is needed; this is the normal operating
		// mode when outbox delivery is not required.
		return wiring, nil
	}

	// From this point Enabled is true. Every failure is a hard startup error.

	if cfg.Dispatcher.Publisher == PublisherTypeNone || cfg.Dispatcher.Publisher == "" {
		return nil, fmt.Errorf(
			"bootstrap: dispatcher enabled but publisher is %q; set a real publisher (e.g. \"kafka\")",
			cfg.Dispatcher.Publisher,
		)
	}

	if repo == nil {
		return nil, fmt.Errorf(
			"bootstrap: dispatcher enabled but no durable outbox repository is available; " +
				"ensure MIDAS_STORE is set to a backend that supports the outbox (e.g. \"postgres\")",
		)
	}

	switch cfg.Dispatcher.Publisher {
	case PublisherTypeKafka:
		kCfg := kafka.KafkaConfig{
			Brokers:      cfg.Kafka.Brokers,
			ClientID:     cfg.Kafka.ClientID,
			RequiredAcks: kafka.RequiredAcks(cfg.Kafka.RequiredAcks),
			WriteTimeout: cfg.Kafka.WriteTimeout,
		}

		pub, err := kafka.NewKafkaPublisher(kCfg)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: build kafka publisher: %w", err)
		}
		wiring.KafkaPublisher = pub

		dispCfg := dispatch.DispatcherConfig{
			BatchSize:    cfg.Dispatcher.BatchSize,
			PollInterval: cfg.Dispatcher.PollInterval,
			MaxBackoff:   cfg.Dispatcher.MaxBackoff,
		}

		d, err := dispatch.NewDispatcher(repo, pub, dispCfg)
		if err != nil {
			// Close the publisher to avoid a resource leak when dispatcher
			// construction fails after a publisher has been opened.
			_ = pub.Close()
			return nil, fmt.Errorf("bootstrap: build dispatcher: %w", err)
		}
		wiring.Dispatcher = d

	default:
		return nil, fmt.Errorf(
			"bootstrap: unsupported dispatcher publisher: %q",
			cfg.Dispatcher.Publisher,
		)
	}

	return wiring, nil
}

// Close releases resources held by the DispatcherWiring. It must be called
// after the dispatcher goroutine has exited (i.e. after the context has been
// cancelled and the goroutine's done channel has been closed).
func (w *DispatcherWiring) Close() {
	if w.KafkaPublisher != nil {
		_ = w.KafkaPublisher.Close()
	}
}
