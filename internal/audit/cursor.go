package audit

// cursor.go — D30j opaque pagination cursor codec.
//
// EncodeListCursor and DecodeListCursor are the only public surface;
// the JSON payload structure and base64 encoding are implementation
// detail. Clients of /v1/evidence/audit-events treat the cursor as an
// opaque string, copy it verbatim into the next request, and rely on
// the server to reject malformed or order-mismatched cursors.
//
// The encoded payload carries a version (`v`) to support future
// migrations: a future tranche that changes the cursor shape can
// detect the version mismatch and reject the cursor with a clear
// error rather than silently misbehaving.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// listCursorVersion is the wire-format version. Incrementing this
// constant intentionally invalidates every existing cursor —
// clients re-issue from the first page.
const listCursorVersion = 1

// ErrInvalidCursor is returned by DecodeListCursor when the supplied
// cursor cannot be parsed, fails schema validation, or carries an
// order direction that does not match the caller's request. The HTTP
// layer surfaces this as 400 with a deliberately-opaque error
// message so cursor internals never leak through error text.
var ErrInvalidCursor = errors.New("audit: cursor is malformed")

// ErrCursorOrderMismatch is returned by DecodeListCursor when the
// cursor encodes a different OrderDesc value than the caller
// supplied. Cursors are bound to an order direction to prevent
// silent gaps / duplicates when clients change `order` mid-pagination.
var ErrCursorOrderMismatch = errors.New("audit: cursor order does not match request order")

// listCursorPayload is the on-wire JSON shape of the cursor. JSON
// tags are short to keep the encoded string compact; the encoded
// form is opaque so terseness has no cost to clients.
type listCursorPayload struct {
	Version    int       `json:"v"`
	OccurredAt time.Time `json:"occurred_at"`
	SequenceNo int       `json:"sequence_no"`
	ID         string    `json:"id"`
	Order      string    `json:"order"`
}

// EncodeListCursor serialises a cursor pointing to (cursor.OccurredAt,
// cursor.SequenceNo, cursor.ID) in the supplied order direction.
// Returns the URL-safe base64 encoding of the canonical JSON payload.
//
// orderDesc=true → "desc" (newest first); false → "asc". The order
// is stored in the cursor so DecodeListCursor can reject a desc
// cursor replayed against an asc query.
//
// Returns an empty string when cursor is nil — callers building
// next_cursor wires omit the field entirely in that case.
func EncodeListCursor(cursor *ListCursor, orderDesc bool) string {
	if cursor == nil {
		return ""
	}
	order := "asc"
	if orderDesc {
		order = "desc"
	}
	payload := listCursorPayload{
		Version:    listCursorVersion,
		OccurredAt: cursor.OccurredAt.UTC(),
		SequenceNo: cursor.SequenceNo,
		ID:         cursor.ID,
		Order:      order,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		// json.Marshal of this fixed shape cannot fail under any
		// reachable input; fall back to an empty cursor rather than
		// surfacing the error to the caller. The handler renders an
		// empty cursor as "no next page" which is safe.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(body)
}

// DecodeListCursor parses an opaque cursor and validates that it
// matches the caller's order direction. Returns ErrInvalidCursor on
// any decoding / schema failure; ErrCursorOrderMismatch when the
// cursor's order does not match wantOrderDesc.
//
// All decoding errors collapse to ErrInvalidCursor by design: the
// wire surface stays opaque even on failure, so a client cannot
// distinguish (and try to repair) a base64 error from a JSON error
// from a missing-field error.
func DecodeListCursor(encoded string, wantOrderDesc bool) (*ListCursor, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		// Empty cursor is treated as "no cursor" by callers, but
		// DecodeListCursor is only invoked when the caller has a
		// non-empty string; surface that as malformed so the
		// handler can produce a 400 rather than silently fall back
		// to first-page semantics.
		return nil, ErrInvalidCursor
	}
	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var payload listCursorPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, ErrInvalidCursor
	}
	if payload.Version != listCursorVersion {
		return nil, ErrInvalidCursor
	}
	if payload.ID == "" {
		return nil, ErrInvalidCursor
	}
	if payload.OccurredAt.IsZero() {
		return nil, ErrInvalidCursor
	}
	wantOrder := "asc"
	if wantOrderDesc {
		wantOrder = "desc"
	}
	switch payload.Order {
	case "asc", "desc":
		// recognised
	default:
		return nil, ErrInvalidCursor
	}
	if payload.Order != wantOrder {
		return nil, ErrCursorOrderMismatch
	}
	return &ListCursor{
		OccurredAt: payload.OccurredAt.UTC(),
		SequenceNo: payload.SequenceNo,
		ID:         payload.ID,
	}, nil
}

// cursorRetainsRow reports whether the given event lies strictly
// past the cursor under the requested order direction. Used by the
// memory repository to enforce the cursor predicate after the other
// filters have run.
//
// For asc order (occurred_at ASC, sequence_no ASC, id ASC), "after"
// means strictly larger in the lexicographic 3-tuple. For desc
// order, "after" means strictly smaller in occurred_at (the primary)
// but strictly larger in sequence_no / id when occurred_at is equal
// — because the secondary / tertiary sort is ASC regardless of the
// primary direction (see postgres_repository.go ORDER BY).
func cursorRetainsRow(ev *AuditEvent, cursor *ListCursor, orderDesc bool) bool {
	if ev == nil || cursor == nil {
		return ev != nil
	}
	if !ev.OccurredAt.Equal(cursor.OccurredAt) {
		if orderDesc {
			return ev.OccurredAt.Before(cursor.OccurredAt)
		}
		return ev.OccurredAt.After(cursor.OccurredAt)
	}
	// occurred_at equal — both secondary and tertiary keys are
	// ASC regardless of primary direction.
	if ev.SequenceNo != cursor.SequenceNo {
		return ev.SequenceNo > cursor.SequenceNo
	}
	return ev.ID > cursor.ID
}

// _ ensures fmt is referenced for future debug helpers without a
// no-op import that the linter would strip.
var _ = fmt.Sprintf
