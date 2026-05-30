package kafka

import (
	"crypto/tls"
	"strings"
	"testing"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
)

func TestNewKafkaPublisher_LocalKafkaNoTLSOrSASL(t *testing.T) {
	pub, err := NewKafkaPublisher(KafkaConfig{
		Brokers:      []string{"localhost:9092"},
		RequiredAcks: RequiredAcksAll,
	})
	if err != nil {
		t.Fatalf("NewKafkaPublisher: %v", err)
	}
	defer pub.Close() //nolint:errcheck

	transport, ok := pub.writer.Transport.(*kafkago.Transport)
	if ok {
		if transport.TLS != nil {
			t.Fatal("expected TLS to be unset")
		}
		if transport.SASL != nil {
			t.Fatal("expected SASL to be unset")
		}
	}
}

func TestNewKafkaPublisher_EventHubsStyleTLSAndPlainSASL(t *testing.T) {
	pub, err := NewKafkaPublisher(KafkaConfig{
		Brokers:       []string{"evh-midas-test.servicebus.windows.net:9093"},
		ClientID:      "midas-test",
		RequiredAcks:  RequiredAcksAll,
		WriteTimeout:  0,
		TLSEnabled:    true,
		SASLMechanism: "plain",
		SASLUsername:  "$ConnectionString",
		SASLPassword:  "secret-value",
	})
	if err != nil {
		t.Fatalf("NewKafkaPublisher: %v", err)
	}
	defer pub.Close() //nolint:errcheck

	transport, ok := pub.writer.Transport.(*kafkago.Transport)
	if !ok {
		t.Fatalf("writer transport type: got %T", pub.writer.Transport)
	}
	if transport.TLS == nil {
		t.Fatal("expected TLS config")
	}
	if transport.TLS.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS MinVersion: got %d, want %d", transport.TLS.MinVersion, tls.VersionTLS12)
	}
	if transport.SASL == nil {
		t.Fatal("expected SASL mechanism")
	}
	mechanism, ok := transport.SASL.(plain.Mechanism)
	if !ok {
		t.Fatalf("SASL mechanism type: got %T, want plain.Mechanism", transport.SASL)
	}
	if mechanism.Name() != "PLAIN" {
		t.Fatalf("SASL mechanism name: got %q, want PLAIN", mechanism.Name())
	}
	if mechanism.Username != "$ConnectionString" {
		t.Fatalf("SASL username: got %q, want $ConnectionString", mechanism.Username)
	}
	if mechanism.Password != "secret-value" {
		t.Fatal("SASL password was not preserved on the mechanism")
	}
}

func TestNewKafkaPublisher_InvalidSASLMechanismFailsClearly(t *testing.T) {
	_, err := NewKafkaPublisher(KafkaConfig{
		Brokers:       []string{"localhost:9092"},
		RequiredAcks:  RequiredAcksAll,
		SASLMechanism: "scram-sha-256",
		SASLUsername:  "user",
		SASLPassword:  "secret-value",
	})
	if err == nil {
		t.Fatal("expected unsupported SASL mechanism error")
	}
	if !strings.Contains(err.Error(), "unsupported SASL mechanism") {
		t.Fatalf("wrong error: %v", err)
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("error leaked SASL password: %v", err)
	}
}

func TestNewKafkaPublisher_PartialSASLConfigFailsClearly(t *testing.T) {
	tests := []struct {
		name string
		cfg  KafkaConfig
	}{
		{
			name: "username without password",
			cfg: KafkaConfig{
				Brokers:      []string{"localhost:9092"},
				RequiredAcks: RequiredAcksAll,
				SASLUsername: "user",
			},
		},
		{
			name: "password without username",
			cfg: KafkaConfig{
				Brokers:      []string{"localhost:9092"},
				RequiredAcks: RequiredAcksAll,
				SASLPassword: "secret-value",
			},
		},
		{
			name: "mechanism without credentials",
			cfg: KafkaConfig{
				Brokers:       []string{"localhost:9092"},
				RequiredAcks:  RequiredAcksAll,
				SASLMechanism: "plain",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewKafkaPublisher(tc.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("error leaked SASL password: %v", err)
			}
		})
	}
}
