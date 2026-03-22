// Package outbox implements the transactional outbox pattern for MIDAS.
//
// The outbox is a durable staging table written in the same database
// transaction as the domain state change that produced the event. A separate
// dispatcher (not part of this package) reads unpublished rows and delivers
// them to downstream consumers, then marks them published. This decouples
// reliable event delivery from the evaluation hot path without introducing
// distributed transactions.
//
// Audit log and outbox are distinct concerns:
//   - Audit events are hash-chained, append-only, and governance records.
//   - Outbox events are routing envelopes for downstream integration; they carry
//     a JSON payload and are marked published once delivered.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors returned by New.
var (
	// ErrEmptyEventType is returned when EventType is blank.
	ErrEmptyEventType = errors.New("outbox: event_type must not be empty")

	// ErrEmptyAggregateType is returned when AggregateType is blank.
	ErrEmptyAggregateType = errors.New("outbox: aggregate_type must not be empty")

	// ErrEmptyAggregateID is returned when AggregateID is blank.
	ErrEmptyAggregateID = errors.New("outbox: aggregate_id must not be empty")

	// ErrEmptyTopic is returned when Topic is blank.
	ErrEmptyTopic = errors.New("outbox: topic must not be empty")

	// ErrInvalidPayload is returned when the payload is not valid JSON.
	ErrInvalidPayload = errors.New("outbox: payload must be valid JSON")
)

// EventType identifies the kind of domain event carried by an outbox row.
// Each value corresponds to a named integration event that downstream systems
// may subscribe to.
type EventType string

const (
	// EventDecisionCompleted is emitted when an evaluation closes with the
	// Execute (accept) outcome. Downstream systems use this to trigger
	// post-decision workflows. This event is emitted only for the Execute
	// outcome; Reject and RequestClarification outcomes do not produce this
	// event because no downstream action is warranted for them.
	EventDecisionCompleted EventType = "decision.completed"

	// EventDecisionEscalated is emitted when an evaluation produces an
	// Escalate outcome and the envelope transitions to AWAITING_REVIEW.
	// This event is not emitted for Execute, Reject, or RequestClarification.
	EventDecisionEscalated EventType = "decision.escalated"

	// EventDecisionReviewResolved is emitted when a reviewer closes an
	// escalated envelope via ResolveEscalation. This event is emitted for
	// both APPROVED and REJECTED review decisions; the payload carries the
	// decision field to distinguish them.
	EventDecisionReviewResolved EventType = "decision.review_resolved"

	// EventSurfaceApproved is emitted when ApproveSurface successfully
	// transitions a surface from review to active.
	EventSurfaceApproved EventType = "surface.approved"

	// EventSurfaceDeprecated is emitted when DeprecateSurface successfully
	// transitions a surface from active to deprecated.
	EventSurfaceDeprecated EventType = "surface.deprecated"

	// EventProfileApproved is emitted when ApproveProfile successfully
	// transitions a profile from review to active.
	EventProfileApproved EventType = "profile.approved"

	// EventProfileDeprecated is emitted when DeprecateProfile successfully
	// transitions a profile from active to deprecated.
	EventProfileDeprecated EventType = "profile.deprecated"

	// EventGrantSuspended is emitted when SuspendGrant successfully
	// transitions a grant from active to suspended.
	EventGrantSuspended EventType = "grant.suspended"

	// EventGrantRevoked is emitted when RevokeGrant permanently revokes a grant.
	EventGrantRevoked EventType = "grant.revoked"

	// EventGrantReinstated is emitted when ReinstateGrant restores a suspended
	// grant to active.
	EventGrantReinstated EventType = "grant.reinstated"
)

// OutboxEvent is a single row in the outbox_events table.
//
// Fields that influence routing (topic, event_key) are separate from the
// payload so that dispatcher implementations can route without deserialising
// the payload.
type OutboxEvent struct {
	// ID is a UUID assigned at construction time.
	ID string

	// EventType identifies the kind of domain event (e.g. "decision.completed").
	EventType EventType

	// AggregateType is the resource kind that produced the event
	// (e.g. "envelope", "surface").
	AggregateType string

	// AggregateID is the identifier of the aggregate instance (e.g. envelope ID,
	// surface ID).
	AggregateID string

	// Topic is the logical destination for the event (e.g. "midas.decisions").
	// Dispatcher implementations map this to a Kafka topic, SNS topic, etc.
	Topic string

	// EventKey is the optional partition/routing key for ordered delivery
	// (e.g. request_source + ":" + request_id, or surface ID).
	EventKey string

	// Payload is the JSON-encoded event body delivered to consumers.
	// Always a valid JSON value; never nil (normalised to {} on construction).
	Payload json.RawMessage

	// CreatedAt is set at construction time.
	CreatedAt time.Time

	// PublishedAt is nil until the dispatcher successfully delivers the event.
	PublishedAt *time.Time
}

// New constructs an OutboxEvent with a new UUID and the current time.
//
// Invariants enforced at construction:
//   - eventType must not be empty.
//   - aggregateType must not be empty.
//   - aggregateID must not be empty.
//   - topic must not be empty.
//   - nil payload is normalised to json.RawMessage(`{}`).
//   - payload must be valid JSON (checked after normalisation).
func New(
	eventType EventType,
	aggregateType string,
	aggregateID string,
	topic string,
	eventKey string,
	payload json.RawMessage,
) (*OutboxEvent, error) {
	if eventType == "" {
		return nil, ErrEmptyEventType
	}
	if aggregateType == "" {
		return nil, ErrEmptyAggregateType
	}
	if aggregateID == "" {
		return nil, ErrEmptyAggregateID
	}
	if topic == "" {
		return nil, ErrEmptyTopic
	}

	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("%w: received %q", ErrInvalidPayload, string(payload))
	}

	return &OutboxEvent{
		ID:            uuid.NewString(),
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Topic:         topic,
		EventKey:      eventKey,
		Payload:       payload,
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// Repository defines the persistence contract for outbox events.
//
// All write methods must be called with a repository instance that is bound
// to the same database transaction as the domain state change. This is the
// invariant that makes the outbox durable: the event row and the domain row
// commit together or roll back together.
type Repository interface {
	// Append writes a single outbox event. The event must not be nil.
	// Returns an error if persistence fails.
	Append(ctx context.Context, ev *OutboxEvent) error

	// ListUnpublished returns all rows where published_at IS NULL, ordered
	// by created_at ascending. Dispatcher implementations call this to find
	// events awaiting delivery.
	ListUnpublished(ctx context.Context) ([]*OutboxEvent, error)

	// ClaimUnpublished returns up to limit unpublished rows using
	// SELECT FOR UPDATE SKIP LOCKED inside a short-lived transaction.
	// Rows are ordered by created_at ASC, id ASC. The locking prevents
	// concurrent dispatchers from processing the same rows simultaneously.
	// Claimed rows remain unpublished in the database until MarkPublished
	// is called for each one.
	ClaimUnpublished(ctx context.Context, limit int) ([]*OutboxEvent, error)

	// MarkPublished sets published_at to now for the given event ID.
	// Returns an error if the event does not exist or the update fails.
	MarkPublished(ctx context.Context, id string) error
}
