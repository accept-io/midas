package bootstrap_test

import (
	"testing"
	"time"

	"github.com/accept-io/midas/internal/bootstrap"
)

// ---------------------------------------------------------------------------
// BuildDispatcher — wiring contract
//
// These tests cover the strict wiring contract enforced by BuildDispatcher:
//   - disabled → nil wiring, no error
//   - enabled + kafka + repo present → non-nil Dispatcher
//   - enabled + nil repo → hard error (no silent skip)
//   - enabled + no publisher → hard error
//   - enabled + unknown publisher → hard error
//
// For test cases that require a live kafka.Writer (kafka publisher with
// brokers), the writer is constructed but not connected; kafka-go defers
// connection to the first WriteMessages call.
// ---------------------------------------------------------------------------

func TestBuildDispatcher_Disabled_ReturnsNilWiringNoError(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      false,
			Publisher:    bootstrap.PublisherTypeNone,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{},
	}

	wiring, err := bootstrap.BuildDispatcher(cfg, nil)
	if err != nil {
		t.Fatalf("BuildDispatcher(disabled): unexpected error: %v", err)
	}
	if wiring == nil {
		t.Fatal("expected non-nil DispatcherWiring struct even when disabled")
	}
	if wiring.Dispatcher != nil {
		t.Error("expected nil Dispatcher when disabled")
	}
	if wiring.KafkaPublisher != nil {
		t.Error("expected nil KafkaPublisher when disabled")
	}
}

func TestBuildDispatcher_Disabled_CloseIsSafe(t *testing.T) {
	// Close on a wiring built from a disabled config must not panic,
	// even though all fields are nil.
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled: false,
		},
	}
	wiring, err := bootstrap.BuildDispatcher(cfg, nil)
	if err != nil {
		t.Fatalf("BuildDispatcher: unexpected error: %v", err)
	}
	// Must not panic.
	wiring.Close()
}

func TestBuildDispatcher_EnabledKafkaWithRepo_ReturnsNonNilDispatcher(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherTypeKafka,
			BatchSize:    10,
			PollInterval: 100 * time.Millisecond,
			MaxBackoff:   1 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{
			Brokers:      []string{"localhost:9092"},
			ClientID:     "midas-test",
			RequiredAcks: -1,
		},
	}

	wiring, err := bootstrap.BuildDispatcher(cfg, &noopOutboxRepo{})
	if err != nil {
		t.Fatalf("BuildDispatcher: unexpected error: %v", err)
	}
	if wiring.Dispatcher == nil {
		t.Error("expected non-nil Dispatcher")
	}
	if wiring.KafkaPublisher == nil {
		t.Error("expected non-nil KafkaPublisher")
	}
	wiring.Close()
}

func TestBuildDispatcher_EnabledNilOutboxRepo_ReturnsError(t *testing.T) {
	// Passing nil repo when the dispatcher is enabled is a startup error.
	// Silently skipping would leave events undelivered with no indication.
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherTypeKafka,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{
			Brokers: []string{"broker:9092"},
		},
	}

	_, err := bootstrap.BuildDispatcher(cfg, nil)
	if err == nil {
		t.Fatal("expected error for enabled+nil outbox repo, got nil")
	}
}

func TestBuildDispatcher_EnabledPublisherNone_ReturnsError_Wiring(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherTypeNone,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
	}

	_, err := bootstrap.BuildDispatcher(cfg, &noopOutboxRepo{})
	if err == nil {
		t.Fatal("expected error for enabled+publisher=none, got nil")
	}
}

func TestBuildDispatcher_EnabledEmptyPublisher_ReturnsError(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherType(""),
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
	}

	_, err := bootstrap.BuildDispatcher(cfg, &noopOutboxRepo{})
	if err == nil {
		t.Fatal("expected error for enabled+empty publisher, got nil")
	}
}

func TestBuildDispatcher_EnabledUnknownPublisher_ReturnsError(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherType("rabbitmq"),
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
	}

	_, err := bootstrap.BuildDispatcher(cfg, &noopOutboxRepo{})
	if err == nil {
		t.Fatal("expected error for enabled+unknown publisher, got nil")
	}
}
