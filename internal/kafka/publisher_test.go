package kafka_test

import (
	"testing"
	"time"

	"github.com/accept-io/midas/internal/kafka"
)

// TestNewKafkaPublisher_NoBrokers_ReturnsError verifies that construction
// fails when no broker addresses are provided.
func TestNewKafkaPublisher_NoBrokers_ReturnsError(t *testing.T) {
	cfg := kafka.KafkaConfig{
		Brokers:      nil,
		RequiredAcks: kafka.RequiredAcksAll,
	}
	_, err := kafka.NewKafkaPublisher(cfg)
	if err == nil {
		t.Fatal("expected error for empty broker list, got nil")
	}
}

// TestNewKafkaPublisher_WithBrokers_Succeeds verifies that a KafkaPublisher
// can be constructed without a live broker. No network connection is attempted
// at construction time; connections are deferred to the first Publish call.
func TestNewKafkaPublisher_WithBrokers_Succeeds(t *testing.T) {
	cfg := kafka.KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		RequiredAcks: kafka.RequiredAcksAll,
	}
	pub, err := kafka.NewKafkaPublisher(cfg)
	if err != nil {
		t.Fatalf("NewKafkaPublisher: unexpected error: %v", err)
	}
	if pub == nil {
		t.Fatal("expected non-nil publisher")
	}
	// Close to release the internal writer's resources.
	if err := pub.Close(); err != nil {
		// Close on an unconnected writer may or may not produce an error
		// depending on the kafka-go version; log rather than fail.
		t.Logf("Close: %v (non-fatal; no live broker was used)", err)
	}
}

// TestNewKafkaPublisher_RequiredAcksLeader verifies that RequiredAcksLeader
// is accepted without error. Internal wiring is validated via construction
// success; the kafka-go writer does not expose its fields for direct inspection
// from an external test package.
func TestNewKafkaPublisher_RequiredAcksLeader_Succeeds(t *testing.T) {
	cfg := kafka.KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		RequiredAcks: kafka.RequiredAcksLeader,
	}
	pub, err := kafka.NewKafkaPublisher(cfg)
	if err != nil {
		t.Fatalf("NewKafkaPublisher(RequiredAcksLeader): unexpected error: %v", err)
	}
	pub.Close() //nolint:errcheck
}

// TestNewKafkaPublisher_RequiredAcksNone_Succeeds verifies that
// RequiredAcksNone (fire-and-forget) is accepted at construction time. The
// caller bears responsibility for the reduced durability guarantee.
func TestNewKafkaPublisher_RequiredAcksNone_Succeeds(t *testing.T) {
	cfg := kafka.KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		RequiredAcks: kafka.RequiredAcksNone,
	}
	pub, err := kafka.NewKafkaPublisher(cfg)
	if err != nil {
		t.Fatalf("NewKafkaPublisher(RequiredAcksNone): unexpected error: %v", err)
	}
	pub.Close() //nolint:errcheck
}

// TestNewKafkaPublisher_WriteTimeout_Accepted verifies that a non-zero
// WriteTimeout is accepted at construction time and does not cause an error.
// The timeout is applied to kafka.Writer.WriteTimeout; verification of the
// field value would require accessing the unexported writer field, so this
// test confirms only that construction succeeds.
func TestNewKafkaPublisher_WriteTimeout_Accepted(t *testing.T) {
	cfg := kafka.KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		RequiredAcks: kafka.RequiredAcksAll,
		WriteTimeout: 10 * time.Second,
	}
	pub, err := kafka.NewKafkaPublisher(cfg)
	if err != nil {
		t.Fatalf("NewKafkaPublisher(WriteTimeout=10s): unexpected error: %v", err)
	}
	pub.Close() //nolint:errcheck
}

// TestNewKafkaPublisher_ZeroWriteTimeout_Accepted verifies that zero
// WriteTimeout (no per-write deadline) is accepted. This is the default and
// means context cancellation is the only deadline.
func TestNewKafkaPublisher_ZeroWriteTimeout_Accepted(t *testing.T) {
	cfg := kafka.KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		RequiredAcks: kafka.RequiredAcksAll,
		WriteTimeout: 0,
	}
	pub, err := kafka.NewKafkaPublisher(cfg)
	if err != nil {
		t.Fatalf("NewKafkaPublisher(WriteTimeout=0): unexpected error: %v", err)
	}
	pub.Close() //nolint:errcheck
}

// TestNewKafkaPublisher_ClientID_Accepted verifies that a non-empty ClientID
// is accepted at construction time. ClientID is applied to the kafka-go
// Transport.ClientID field for broker-side observability. The value is not
// readable from the external test package without exposing the internal writer.
func TestNewKafkaPublisher_ClientID_Accepted(t *testing.T) {
	cfg := kafka.KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		RequiredAcks: kafka.RequiredAcksAll,
		ClientID:     "midas-dispatcher",
	}
	pub, err := kafka.NewKafkaPublisher(cfg)
	if err != nil {
		t.Fatalf("NewKafkaPublisher(ClientID=midas-dispatcher): unexpected error: %v", err)
	}
	pub.Close() //nolint:errcheck
}

// TestNewKafkaPublisher_EmptyClientID_Accepted verifies that an empty ClientID
// does not cause a construction error. An empty string results in no client
// identifier being sent to the broker.
func TestNewKafkaPublisher_EmptyClientID_Accepted(t *testing.T) {
	cfg := kafka.KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		RequiredAcks: kafka.RequiredAcksAll,
		ClientID:     "",
	}
	pub, err := kafka.NewKafkaPublisher(cfg)
	if err != nil {
		t.Fatalf("NewKafkaPublisher(ClientID=''): unexpected error: %v", err)
	}
	pub.Close() //nolint:errcheck
}

// TestDefaultKafkaConfig_RequiredAcksAll verifies that the default configuration
// uses the strongest acknowledgement level.
func TestDefaultKafkaConfig_RequiredAcksAll(t *testing.T) {
	cfg := kafka.DefaultKafkaConfig()
	if cfg.RequiredAcks != kafka.RequiredAcksAll {
		t.Errorf("expected RequiredAcksAll (-1), got %d", cfg.RequiredAcks)
	}
}

// TestRequiredAcksConstants verifies the numeric values of the acknowledgement
// level constants match the Kafka protocol specification.
func TestRequiredAcksConstants(t *testing.T) {
	tests := []struct {
		name string
		got  kafka.RequiredAcks
		want kafka.RequiredAcks
	}{
		{"None", kafka.RequiredAcksNone, 0},
		{"Leader", kafka.RequiredAcksLeader, 1},
		{"All", kafka.RequiredAcksAll, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("expected %d, got %d", tc.want, tc.got)
			}
		})
	}
}
