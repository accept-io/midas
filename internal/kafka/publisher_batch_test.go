package kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/dispatch"
	kafkago "github.com/segmentio/kafka-go"
)

type recordingWriter struct {
	messages []kafkago.Message
	err      error
}

func (w *recordingWriter) WriteMessages(_ context.Context, msgs ...kafkago.Message) error {
	w.messages = append([]kafkago.Message(nil), msgs...)
	return w.err
}

func (w *recordingWriter) Close() error { return nil }

func TestKafkaPublisher_PublishBatchWritesAllMessages(t *testing.T) {
	writer := &recordingWriter{}
	pub := &KafkaPublisher{writeMessages: writer.WriteMessages, closeWriter: writer.Close}

	msgs := []dispatch.Message{
		{
			Topic: "midas.decisions",
			Key:   []byte("k1"),
			Value: []byte(`{"n":1}`),
			Headers: []dispatch.Header{
				{Key: "event_type", Value: []byte("decision.completed")},
			},
		},
		{
			Topic: "midas.decisions",
			Key:   []byte("k2"),
			Value: []byte(`{"n":2}`),
			Headers: []dispatch.Header{
				{Key: "event_type", Value: []byte("decision.escalated")},
			},
		},
	}

	result, err := pub.PublishBatch(context.Background(), msgs)
	if err != nil {
		t.Fatalf("PublishBatch: %v", err)
	}
	if len(result.Successful) != 2 {
		t.Fatalf("successful indexes: want 2, got %v", result.Successful)
	}
	if len(writer.messages) != 2 {
		t.Fatalf("writer messages: want 2, got %d", len(writer.messages))
	}
	if writer.messages[0].Topic != "midas.decisions" || string(writer.messages[0].Key) != "k1" || string(writer.messages[0].Value) != `{"n":1}` {
		t.Fatalf("first kafka message not preserved: %#v", writer.messages[0])
	}
	if len(writer.messages[0].Headers) != 1 || writer.messages[0].Headers[0].Key != "event_type" || string(writer.messages[0].Headers[0].Value) != "decision.completed" {
		t.Fatalf("headers not preserved: %#v", writer.messages[0].Headers)
	}
}

func TestKafkaPublisher_PublishBatchEmptyIsNoOp(t *testing.T) {
	writer := &recordingWriter{}
	pub := &KafkaPublisher{writeMessages: writer.WriteMessages, closeWriter: writer.Close}

	result, err := pub.PublishBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("PublishBatch(nil): %v", err)
	}
	if len(result.Successful) != 0 || len(result.Failed) != 0 {
		t.Fatalf("empty result should have no indexes, got %#v", result)
	}
	if len(writer.messages) != 0 {
		t.Fatalf("writer should not be called for empty batch, got %d messages", len(writer.messages))
	}
}

func TestKafkaPublisher_PublishBatchMapsPartialWriteErrors(t *testing.T) {
	writeErrs := kafkago.WriteErrors{nil, errors.New("partition unavailable"), nil}
	writer := &recordingWriter{err: writeErrs}
	pub := &KafkaPublisher{writeMessages: writer.WriteMessages, closeWriter: writer.Close}

	msgs := []dispatch.Message{
		{Topic: "midas.decisions", Key: []byte("k1")},
		{Topic: "midas.decisions", Key: []byte("k2")},
		{Topic: "midas.decisions", Key: []byte("k3")},
	}

	result, err := pub.PublishBatch(context.Background(), msgs)
	if err == nil {
		t.Fatal("expected partial write error")
	}
	if got := result.Successful; len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("successful indexes: want [0 2], got %v", got)
	}
	if len(result.Failed) != 1 || result.Failed[0].Index != 1 {
		t.Fatalf("failed indexes: want index 1, got %#v", result.Failed)
	}
}

func TestKafkaPublisher_PublishBatchUnknownErrorConfirmsNothing(t *testing.T) {
	writer := &recordingWriter{err: errors.New("broker unavailable")}
	pub := &KafkaPublisher{writeMessages: writer.WriteMessages, closeWriter: writer.Close}

	msgs := []dispatch.Message{
		{Topic: "midas.decisions", Key: []byte("k1")},
		{Topic: "midas.decisions", Key: []byte("k2")},
	}

	result, err := pub.PublishBatch(context.Background(), msgs)
	if err == nil {
		t.Fatal("expected batch write error")
	}
	if len(result.Successful) != 0 {
		t.Fatalf("unknown error must not confirm successes, got %v", result.Successful)
	}
	if len(result.Failed) != 2 {
		t.Fatalf("unknown error should mark all messages failed, got %#v", result.Failed)
	}
}
