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

// ListCursor (D30j) marks the boundary between the previous page and
// the next page in a cursor-paginated List call. The three fields
// are the absolute ordering tuple the repository sorts by — together
// they identify exactly one row globally, so "rows strictly after
// the cursor" is unambiguous regardless of timestamp collisions or
// per-envelope sequence-number reuse.
//
// The repository compares the cursor tuple against (occurred_at,
// sequence_no, id) using the order direction the caller's OrderDesc
// already implies. ID is a UUID and is compared lexically; that is
// sufficient as a stable tertiary tie-breaker even though UUID order
// is not time-ordered.
//
// The HTTP layer encodes / decodes this struct via the audit cursor
// codec (see cursor.go); the codec carries a version field so a
// future encoding break can be detected. Repository callers receive
// the decoded value and never see the encoded form.
type ListCursor struct {
	OccurredAt time.Time
	SequenceNo int
	ID         string
}

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
//   - Limit=0 → DefaultListLimit. Limit > MaxListLimit → MaxListLimit
//     (one-unit headroom for the cursor-pagination probe — see
//     EffectiveLimit).
//   - Cursor (D30j): when non-nil, restricts results to rows strictly
//     past the cursor tuple per the current OrderDesc semantics.
//     Nil cursor means "from the start of the ordered scan".
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

	Cursor *ListCursor
}

// EffectiveLimit returns the limit that implementations should actually
// apply: DefaultListLimit when zero, the caller's value when it fits,
// MaxListLimit+1 when over.
//
// One-unit headroom (MaxListLimit+1) was added in D30j so the
// /v1/evidence/audit-events cursor handler can request "wanted+1"
// rows internally to detect a next page even when the user requested
// the maximum page size. The HTTP layer still rejects user-supplied
// Limit > MaxListLimit with 400 before constructing the filter; the
// only callers that can reach the MaxListLimit+1 branch are internal
// has-next probes.
func (f ListFilter) EffectiveLimit() int {
	if f.Limit <= 0 {
		return DefaultListLimit
	}
	if f.Limit > MaxListLimit+1 {
		return MaxListLimit + 1
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
