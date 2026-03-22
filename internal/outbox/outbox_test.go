package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/outbox"
)

// ---------------------------------------------------------------------------
// New constructor tests
// ---------------------------------------------------------------------------

func TestNew_PopulatesFields(t *testing.T) {
	payload := json.RawMessage(`{"surface_id":"payments.execute"}`)
	ev, err := outbox.New(
		outbox.EventDecisionCompleted,
		"envelope",
		"env-abc",
		"midas.decisions",
		"src::req-1",
		payload,
	)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	if ev.ID == "" {
		t.Error("expected non-empty ID")
	}
	if ev.EventType != outbox.EventDecisionCompleted {
		t.Errorf("expected EventType %q, got %q", outbox.EventDecisionCompleted, ev.EventType)
	}
	if ev.AggregateType != "envelope" {
		t.Errorf("expected AggregateType %q, got %q", "envelope", ev.AggregateType)
	}
	if ev.AggregateID != "env-abc" {
		t.Errorf("expected AggregateID %q, got %q", "env-abc", ev.AggregateID)
	}
	if ev.Topic != "midas.decisions" {
		t.Errorf("expected Topic %q, got %q", "midas.decisions", ev.Topic)
	}
	if ev.EventKey != "src::req-1" {
		t.Errorf("expected EventKey %q, got %q", "src::req-1", ev.EventKey)
	}
	if ev.PublishedAt != nil {
		t.Error("expected PublishedAt to be nil on construction")
	}
	if ev.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestNew_NilPayload_NormalisedToEmptyObject(t *testing.T) {
	ev, err := outbox.New(
		outbox.EventDecisionCompleted,
		"envelope",
		"env-1",
		"midas.decisions",
		"",
		nil, // nil payload
	)
	if err != nil {
		t.Fatalf("New with nil payload: unexpected error: %v", err)
	}
	if ev.Payload == nil {
		t.Fatal("expected non-nil Payload after normalisation")
	}
	if string(ev.Payload) != "{}" {
		t.Errorf("expected Payload %q, got %q", "{}", string(ev.Payload))
	}
}

func TestNew_InvalidPayload_ReturnsError(t *testing.T) {
	_, err := outbox.New(
		outbox.EventDecisionCompleted,
		"envelope",
		"env-1",
		"midas.decisions",
		"",
		json.RawMessage(`not-json`),
	)
	if err == nil {
		t.Fatal("expected error for invalid JSON payload, got nil")
	}
	if !errors.Is(err, outbox.ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload, got %v", err)
	}
}

func TestNew_EmptyEventType_ReturnsError(t *testing.T) {
	_, err := outbox.New(
		"", // empty eventType
		"envelope",
		"env-1",
		"midas.decisions",
		"",
		nil,
	)
	if err == nil {
		t.Fatal("expected error for empty event_type, got nil")
	}
	if !errors.Is(err, outbox.ErrEmptyEventType) {
		t.Errorf("expected ErrEmptyEventType, got %v", err)
	}
}

func TestNew_EmptyAggregateType_ReturnsError(t *testing.T) {
	_, err := outbox.New(
		outbox.EventDecisionCompleted,
		"", // empty aggregateType
		"env-1",
		"midas.decisions",
		"",
		nil,
	)
	if err == nil {
		t.Fatal("expected error for empty aggregate_type, got nil")
	}
	if !errors.Is(err, outbox.ErrEmptyAggregateType) {
		t.Errorf("expected ErrEmptyAggregateType, got %v", err)
	}
}

func TestNew_EmptyAggregateID_ReturnsError(t *testing.T) {
	_, err := outbox.New(
		outbox.EventDecisionCompleted,
		"envelope",
		"", // empty aggregateID
		"midas.decisions",
		"",
		nil,
	)
	if err == nil {
		t.Fatal("expected error for empty aggregate_id, got nil")
	}
	if !errors.Is(err, outbox.ErrEmptyAggregateID) {
		t.Errorf("expected ErrEmptyAggregateID, got %v", err)
	}
}

func TestNew_EmptyTopic_ReturnsError(t *testing.T) {
	_, err := outbox.New(
		outbox.EventDecisionCompleted,
		"envelope",
		"env-1",
		"", // empty topic
		"",
		nil,
	)
	if err == nil {
		t.Fatal("expected error for empty topic, got nil")
	}
	if !errors.Is(err, outbox.ErrEmptyTopic) {
		t.Errorf("expected ErrEmptyTopic, got %v", err)
	}
}

func TestNew_ValidEmptyObjectPayload_Succeeds(t *testing.T) {
	ev, err := outbox.New(
		outbox.EventDecisionCompleted,
		"envelope",
		"env-1",
		"midas.decisions",
		"",
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("New with explicit {} payload: unexpected error: %v", err)
	}
	if string(ev.Payload) != "{}" {
		t.Errorf("expected Payload {}, got %q", string(ev.Payload))
	}
}

// ---------------------------------------------------------------------------
// MemoryRepository tests
// ---------------------------------------------------------------------------

func mustNew(t *testing.T, eventType outbox.EventType, aggregateType, aggregateID, topic, eventKey string, payload json.RawMessage) *outbox.OutboxEvent {
	t.Helper()
	ev, err := outbox.New(eventType, aggregateType, aggregateID, topic, eventKey, payload)
	if err != nil {
		t.Fatalf("outbox.New: %v", err)
	}
	return ev
}

func TestMemoryRepository_AppendAndListUnpublished(t *testing.T) {
	ctx := context.Background()
	repo := outbox.NewMemoryRepository()

	ev := mustNew(t, outbox.EventDecisionCompleted, "envelope", "env-1", "midas.decisions", "k1", json.RawMessage(`{}`))
	if err := repo.Append(ctx, ev); err != nil {
		t.Fatalf("Append: %v", err)
	}

	unpublished, err := repo.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	if len(unpublished) != 1 {
		t.Fatalf("expected 1 unpublished event, got %d", len(unpublished))
	}
	if unpublished[0].ID != ev.ID {
		t.Errorf("expected ID %q, got %q", ev.ID, unpublished[0].ID)
	}
}

func TestMemoryRepository_MarkPublished(t *testing.T) {
	ctx := context.Background()
	repo := outbox.NewMemoryRepository()

	ev := mustNew(t, outbox.EventDecisionCompleted, "envelope", "env-2", "midas.decisions", "k2", json.RawMessage(`{}`))
	if err := repo.Append(ctx, ev); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := repo.MarkPublished(ctx, ev.ID); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	unpublished, err := repo.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	if len(unpublished) != 0 {
		t.Errorf("expected 0 unpublished after marking, got %d", len(unpublished))
	}

	all := repo.All(ctx)
	if all[0].PublishedAt == nil {
		t.Error("expected PublishedAt to be set after MarkPublished")
	}
}

func TestMemoryRepository_MarkPublished_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := outbox.NewMemoryRepository()

	err := repo.MarkPublished(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent event, got nil")
	}
}

func TestMemoryRepository_Append_NilEvent(t *testing.T) {
	ctx := context.Background()
	repo := outbox.NewMemoryRepository()

	err := repo.Append(ctx, nil)
	if err == nil {
		t.Error("expected error when appending nil event, got nil")
	}
}

func TestMemoryRepository_ListUnpublished_MixedState(t *testing.T) {
	ctx := context.Background()
	repo := outbox.NewMemoryRepository()

	ev1 := mustNew(t, outbox.EventDecisionCompleted, "envelope", "env-3", "midas.decisions", "k3", json.RawMessage(`{}`))
	ev2 := mustNew(t, outbox.EventDecisionEscalated, "envelope", "env-4", "midas.decisions", "k4", json.RawMessage(`{}`))

	if err := repo.Append(ctx, ev1); err != nil {
		t.Fatalf("Append ev1: %v", err)
	}
	if err := repo.Append(ctx, ev2); err != nil {
		t.Fatalf("Append ev2: %v", err)
	}

	if err := repo.MarkPublished(ctx, ev1.ID); err != nil {
		t.Fatalf("MarkPublished ev1: %v", err)
	}

	unpublished, err := repo.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	if len(unpublished) != 1 {
		t.Fatalf("expected 1 unpublished, got %d", len(unpublished))
	}
	if unpublished[0].ID != ev2.ID {
		t.Errorf("expected unpublished event to be ev2 (%q), got %q", ev2.ID, unpublished[0].ID)
	}
}

func TestMemoryRepository_All(t *testing.T) {
	ctx := context.Background()
	repo := outbox.NewMemoryRepository()

	ev1 := mustNew(t, outbox.EventSurfaceApproved, "surface", "surf-1", "midas.surfaces", "surf-1", json.RawMessage(`{}`))
	ev2 := mustNew(t, outbox.EventSurfaceDeprecated, "surface", "surf-1", "midas.surfaces", "surf-1", json.RawMessage(`{}`))

	_ = repo.Append(ctx, ev1)
	_ = repo.Append(ctx, ev2)

	all := repo.All(ctx)
	if len(all) != 2 {
		t.Fatalf("expected 2 events in All(), got %d", len(all))
	}
}

func TestMemoryRepository_All_Empty(t *testing.T) {
	ctx := context.Background()
	repo := outbox.NewMemoryRepository()

	all := repo.All(ctx)
	if len(all) != 0 {
		t.Fatalf("expected 0 events in empty repo, got %d", len(all))
	}
}
