package audit

import (
	"context"
	"errors"
	"time"
)

// DefaultListLimit is applied when ListFilter.Limit is zero.
const DefaultListLimit = 100

// MaxListLimit caps any caller-supplied Limit. Implementations clamp to
// this value silently; HTTP layers in front of the repository may
// reject requests that exceed it (see the /v1/coverage handler in
// internal/httpapi).
const MaxListLimit = 500

// ErrInvalidTimeRange is returned by List when Until is non-zero and
// strictly before Since. The repository deliberately rejects rather
// than silently swap or ignore — a malformed range is a caller error
// the HTTP layer should surface as 400.
var ErrInvalidTimeRange = errors.New("audit: list filter Until is before Since")

// ListFilter constrains a List call. All fields are optional; a
// zero-value filter returns every event up to DefaultListLimit. Field
// semantics:
//
//   - EventType / EventTypes: alternative event-type filters.
//     EventTypes wins when non-empty (the broader filter takes
//     precedence). When both are empty, no event-type filter applies.
//   - PayloadContains: top-level JSON containment only. Keys must be
//     at the top level of the persisted payload. Nested-path filters
//     (e.g. summary.confidence) are deliberately not supported and
//     must not be added.
//   - Since is inclusive (occurred_at >= Since); Until is exclusive
//     (occurred_at < Until). Zero time.Time values mean unbounded.
//   - OrderDesc=true returns newest first (the coverage read service's
//     default); OrderDesc=false returns oldest first.
//   - Limit=0 → DefaultListLimit. Limit > MaxListLimit → MaxListLimit.
type ListFilter struct {
	EventType  AuditEventType
	EventTypes []AuditEventType

	EnvelopeID    string
	RequestSource string
	RequestID     string

	// Top-level payload containment only.
	PayloadContains map[string]any

	Since time.Time
	Until time.Time

	Limit     int
	OrderDesc bool
}

// EffectiveLimit returns the limit that implementations should actually
// apply: DefaultListLimit when zero, MaxListLimit when over the cap,
// the caller's value otherwise.
func (f ListFilter) EffectiveLimit() int {
	if f.Limit <= 0 {
		return DefaultListLimit
	}
	if f.Limit > MaxListLimit {
		return MaxListLimit
	}
	return f.Limit
}

// Validate checks the filter for inconsistencies that warrant a hard
// error rather than a silent fix-up. Callers should invoke this before
// dispatching to a repository implementation; both the memory and
// Postgres impls also call it defensively.
func (f ListFilter) Validate() error {
	if !f.Until.IsZero() && !f.Since.IsZero() && f.Until.Before(f.Since) {
		return ErrInvalidTimeRange
	}
	return nil
}

type AuditEventRepository interface {
	Append(ctx context.Context, ev *AuditEvent) error
	ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*AuditEvent, error)
	ListByRequestID(ctx context.Context, requestID string) ([]*AuditEvent, error)

	// List returns audit events matching the supplied filter. See
	// ListFilter for field semantics. Results are ordered by
	// occurred_at — descending when OrderDesc is true, ascending
	// otherwise. Implementations must apply EffectiveLimit (default
	// when zero, capped at MaxListLimit). Returns ErrInvalidTimeRange
	// when Until is non-zero and strictly before Since.
	//
	// This method is the query primitive consumed by the
	// governancecoverage read service (#56). It does not replace
	// ListByEnvelopeID or ListByRequestID — those are kept for the
	// hash-chain validator and existing callers.
	List(ctx context.Context, filter ListFilter) ([]*AuditEvent, error)
}
