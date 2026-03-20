// Package dispatch implements the transactional outbox dispatcher.
//
// The dispatcher polls unpublished outbox rows, publishes them to a message
// broker via the Publisher interface, and marks each row published only after
// the broker acknowledges receipt. Delivery is at-least-once: if the process
// crashes between a successful broker publish and the MarkPublished write, the
// row will be claimed again on the next poll and re-published.
//
// Consumer-side idempotency is assumed. The OutboxEvent.ID field is stable
// across retries and can serve as a deduplication key.
package dispatch

import (
	"context"

	"github.com/accept-io/midas/internal/outbox"
)

// Publisher delivers a single message to a message broker and returns only
// after the broker has acknowledged receipt.
//
// Implementations must be safe for concurrent use. Publish must be idempotent
// or the caller must accept that duplicate messages may be produced.
type Publisher interface {
	// Publish sends msg to the broker. Returns nil only when the broker has
	// acknowledged receipt. Any returned error means the message may or may
	// not have been delivered; the caller should treat the event as
	// unpublished and retry on the next poll cycle.
	Publish(ctx context.Context, msg Message) error
}

// DispatcherRepo is the persistence interface required by the Dispatcher.
// It is a strict subset of outbox.Repository extended with claiming semantics.
type DispatcherRepo interface {
	// ClaimUnpublished returns up to limit unpublished outbox rows ordered by
	// created_at ASC, id ASC. The implementation uses SELECT FOR UPDATE SKIP
	// LOCKED inside a short-lived transaction to prevent concurrent dispatchers
	// from claiming the same rows simultaneously.
	//
	// Rows returned are not mutated by the claim; they remain unpublished in
	// the database until MarkPublished is called for each one.
	ClaimUnpublished(ctx context.Context, limit int) ([]*outbox.OutboxEvent, error)

	// MarkPublished sets published_at to now for the given event ID.
	// Returns an error if the event does not exist or the update fails.
	MarkPublished(ctx context.Context, id string) error
}

// Message is the broker-agnostic envelope that Publisher implementations
// receive. All fields are populated from the outbox row before publishing.
type Message struct {
	// Topic is the logical destination (maps to a Kafka topic, SNS topic, etc.)
	Topic string

	// Key is the optional partition / ordering key. May be empty.
	Key []byte

	// Value is the JSON-encoded event payload. Always valid JSON.
	Value []byte

	// Headers carry event metadata without requiring payload deserialization.
	Headers []Header
}

// Header is a single key/value pair attached to a Message.
type Header struct {
	Key   string
	Value []byte
}
