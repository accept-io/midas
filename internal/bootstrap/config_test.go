package bootstrap_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/outbox"
)

// ---------------------------------------------------------------------------
// AppConfig.Validate
// ---------------------------------------------------------------------------

func TestValidate_DispatcherDisabled_NoKafka_Valid(t *testing.T) {
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
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidate_DispatcherDisabled_KafkaPresent_Valid(t *testing.T) {
	// Kafka config may be present; when dispatcher is disabled it is ignored.
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      false,
			Publisher:    bootstrap.PublisherTypeKafka,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{
			Brokers: []string{"broker:9092"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config when dispatcher disabled, got error: %v", err)
	}
}

func TestValidate_DispatcherEnabled_PublisherNone_Invalid(t *testing.T) {
	// DispatcherEnabled=true requires a real publisher; "none" is rejected.
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherTypeNone,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for enabled+none, got nil")
	}
	if !errors.Is(err, bootstrap.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestValidate_DispatcherEnabled_EmptyPublisher_Invalid(t *testing.T) {
	// DispatcherEnabled=true requires a real publisher; empty publisher is rejected.
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherType(""),
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for enabled+empty publisher, got nil")
	}
	if !errors.Is(err, bootstrap.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestValidate_DispatcherDisabled_PublisherNone_Valid(t *testing.T) {
	// DispatcherEnabled=false: all publisher fields are ignored.
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
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config for disabled+none, got error: %v", err)
	}
}

func TestValidate_DispatcherEnabled_KafkaWithBrokers_Valid(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherTypeKafka,
			BatchSize:    50,
			PollInterval: 1 * time.Second,
			MaxBackoff:   10 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{
			Brokers:      []string{"kafka1:9092", "kafka2:9092"},
			RequiredAcks: -1,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config for enabled+kafka+brokers, got error: %v", err)
	}
}

func TestValidate_DispatcherEnabled_KafkaNoBrokers_Invalid(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherTypeKafka,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{
			Brokers: nil,
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for enabled+kafka+no brokers, got nil")
	}
	if !errors.Is(err, bootstrap.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestValidate_DispatcherEnabled_UnknownPublisher_Invalid(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherType("rabbitmq"),
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown publisher type, got nil")
	}
	if !errors.Is(err, bootstrap.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// LoadAppConfig defaults
// ---------------------------------------------------------------------------

func TestLoadAppConfig_Defaults(t *testing.T) {
	// Ensure none of the MIDAS_DISPATCHER_* or MIDAS_KAFKA_* vars are set.
	unsetEnv(t,
		"MIDAS_DISPATCHER_ENABLED",
		"MIDAS_DISPATCHER_PUBLISHER",
		"MIDAS_DISPATCHER_BATCH_SIZE",
		"MIDAS_DISPATCHER_POLL_INTERVAL",
		"MIDAS_DISPATCHER_MAX_BACKOFF",
		"MIDAS_KAFKA_BROKERS",
		"MIDAS_KAFKA_CLIENT_ID",
		"MIDAS_KAFKA_REQUIRED_ACKS",
		"MIDAS_KAFKA_WRITE_TIMEOUT",
	)

	cfg, err := bootstrap.LoadAppConfig()
	if err != nil {
		t.Fatalf("LoadAppConfig: unexpected error: %v", err)
	}
	if cfg.Dispatcher.Enabled {
		t.Error("expected dispatcher disabled by default")
	}
	if cfg.Dispatcher.Publisher != bootstrap.PublisherTypeNone {
		t.Errorf("expected publisher=none by default, got %q", cfg.Dispatcher.Publisher)
	}
	if cfg.Dispatcher.BatchSize != 100 {
		t.Errorf("expected default BatchSize=100, got %d", cfg.Dispatcher.BatchSize)
	}
	if cfg.Dispatcher.PollInterval != 2*time.Second {
		t.Errorf("expected default PollInterval=2s, got %v", cfg.Dispatcher.PollInterval)
	}
	if cfg.Dispatcher.MaxBackoff != 30*time.Second {
		t.Errorf("expected default MaxBackoff=30s, got %v", cfg.Dispatcher.MaxBackoff)
	}
	if cfg.Kafka.RequiredAcks != -1 {
		t.Errorf("expected default RequiredAcks=-1, got %d", cfg.Kafka.RequiredAcks)
	}
	if cfg.Kafka.ClientID != "midas" {
		t.Errorf("expected default ClientID=midas, got %q", cfg.Kafka.ClientID)
	}
}

func TestLoadAppConfig_DispatcherEnabled_KafkaBrokers(t *testing.T) {
	t.Setenv("MIDAS_DISPATCHER_ENABLED", "true")
	t.Setenv("MIDAS_DISPATCHER_PUBLISHER", "kafka")
	t.Setenv("MIDAS_KAFKA_BROKERS", "b1:9092,b2:9092")

	cfg, err := bootstrap.LoadAppConfig()
	if err != nil {
		t.Fatalf("LoadAppConfig: unexpected error: %v", err)
	}
	if !cfg.Dispatcher.Enabled {
		t.Error("expected dispatcher enabled")
	}
	if cfg.Dispatcher.Publisher != bootstrap.PublisherTypeKafka {
		t.Errorf("expected publisher=kafka, got %q", cfg.Dispatcher.Publisher)
	}
	if len(cfg.Kafka.Brokers) != 2 {
		t.Errorf("expected 2 brokers, got %d: %v", len(cfg.Kafka.Brokers), cfg.Kafka.Brokers)
	}
}

func TestLoadAppConfig_InvalidBoolEnabled_ReturnsError(t *testing.T) {
	t.Setenv("MIDAS_DISPATCHER_ENABLED", "yes_please")

	_, err := bootstrap.LoadAppConfig()
	if err == nil {
		t.Fatal("expected error for non-boolean MIDAS_DISPATCHER_ENABLED, got nil")
	}
	if !errors.Is(err, bootstrap.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestLoadAppConfig_InvalidBatchSize_ReturnsError(t *testing.T) {
	t.Setenv("MIDAS_DISPATCHER_BATCH_SIZE", "0")

	_, err := bootstrap.LoadAppConfig()
	if err == nil {
		t.Fatal("expected error for non-positive MIDAS_DISPATCHER_BATCH_SIZE, got nil")
	}
	if !errors.Is(err, bootstrap.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestLoadAppConfig_InvalidPollInterval_ReturnsError(t *testing.T) {
	t.Setenv("MIDAS_DISPATCHER_POLL_INTERVAL", "not-a-duration")

	_, err := bootstrap.LoadAppConfig()
	if err == nil {
		t.Fatal("expected error for invalid MIDAS_DISPATCHER_POLL_INTERVAL, got nil")
	}
	if !errors.Is(err, bootstrap.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}
}

func TestLoadAppConfig_InvalidRequiredAcks_ReturnsError(t *testing.T) {
	t.Setenv("MIDAS_KAFKA_REQUIRED_ACKS", "2")

	_, err := bootstrap.LoadAppConfig()
	if err == nil {
		t.Fatal("expected error for invalid MIDAS_KAFKA_REQUIRED_ACKS, got nil")
	}
	if !errors.Is(err, bootstrap.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// BuildDispatcher wiring
// ---------------------------------------------------------------------------

func TestBuildDispatcher_DispatcherDisabled_ReturnsNilDispatcher(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      false,
			Publisher:    bootstrap.PublisherTypeKafka,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{
			Brokers: []string{"broker:9092"},
		},
	}

	wiring, err := bootstrap.BuildDispatcher(cfg, &noopOutboxRepo{})
	if err != nil {
		t.Fatalf("BuildDispatcher: unexpected error: %v", err)
	}
	if wiring.Dispatcher != nil {
		t.Error("expected nil Dispatcher when dispatcher is disabled")
	}
	if wiring.KafkaPublisher != nil {
		t.Error("expected nil KafkaPublisher when dispatcher is disabled")
	}
}

func TestBuildDispatcher_EnabledPublisherNone_ReturnsError(t *testing.T) {
	// When the dispatcher is enabled, publisher=none is invalid at startup.
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherTypeNone,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{},
	}

	_, err := bootstrap.BuildDispatcher(cfg, &noopOutboxRepo{})
	if err == nil {
		t.Fatal("expected error for enabled+publisher=none, got nil")
	}
}

func TestBuildDispatcher_EnabledNilRepo_ReturnsError(t *testing.T) {
	// When the dispatcher is enabled, a nil outbox repository is invalid at
	// startup. The in-memory store returns nil; enabling the dispatcher against
	// it must be caught here so the process fails fast rather than running
	// silently without dispatching.
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

func TestBuildDispatcher_KafkaWithBrokers_ReturnsDispatcher(t *testing.T) {
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
			RequiredAcks: -1,
		},
	}

	wiring, err := bootstrap.BuildDispatcher(cfg, &noopOutboxRepo{})
	if err != nil {
		t.Fatalf("BuildDispatcher: unexpected error: %v", err)
	}
	if wiring.Dispatcher == nil {
		t.Error("expected non-nil Dispatcher for kafka publisher with brokers")
	}
	if wiring.KafkaPublisher == nil {
		t.Error("expected non-nil KafkaPublisher")
	}
	// Release the internal kafka-go writer.
	wiring.Close()
}

func TestBuildDispatcher_KafkaNoBrokers_ReturnsError(t *testing.T) {
	cfg := bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      true,
			Publisher:    bootstrap.PublisherTypeKafka,
			BatchSize:    100,
			PollInterval: 2 * time.Second,
			MaxBackoff:   30 * time.Second,
		},
		Kafka: bootstrap.KafkaConfig{
			Brokers: nil,
		},
	}

	_, err := bootstrap.BuildDispatcher(cfg, &noopOutboxRepo{})
	if err == nil {
		t.Fatal("expected error for kafka publisher with no brokers, got nil")
	}
}

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// noopOutboxRepo satisfies outbox.Repository with no-op implementations for
// use in unit tests that do not need real outbox persistence.
type noopOutboxRepo struct{}

func (r *noopOutboxRepo) Append(_ context.Context, _ *outbox.OutboxEvent) error {
	return nil
}

func (r *noopOutboxRepo) ListUnpublished(_ context.Context) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}

func (r *noopOutboxRepo) ClaimUnpublished(_ context.Context, _ int) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}

func (r *noopOutboxRepo) MarkPublished(_ context.Context, _ string) error {
	return nil
}

// unsetEnv clears environment variables for the duration of the test.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		t.Setenv(k, "")
	}
}
