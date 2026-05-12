package httpapi

// evidence_handler.go — D30b runtime evidence read API.
//
// Provides a production, OpenAPI-documented, role-gated read surface
// for the audit-event chain of one evaluation envelope:
//
//	GET /v1/evidence/envelopes/{id}/audit-events
//
// The narrow interface pattern matches the existing httpapi style
// (mirrors driftReadService / failModePolicyReadService /
// coverageReadService): a tiny consumer-owned interface, a concrete
// EvidenceReadService struct that wraps domain repositories, and a
// Server.WithEvidenceReadService builder for wiring.
//
// Read-only by construction. The handler does not write to any
// repository, does not call the orchestrator, and does not expose the
// envelope's raw submitted payload — `/v1/envelopes/{id}` remains the
// canonical envelope-detail endpoint and is the only route that
// surfaces Submitted.Raw.
//
// Disjoint from the Explorer route. GET /explorer/envelopes/{id}/audit-events
// remains in place and is backed by the Explorer-isolated in-memory
// store; the production route here uses the production envelopes /
// audit repositories.

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/envelope"
)

// EvidenceEnvelopeReader is the narrow envelope read contract the
// evidence read service requires. Subset of envelope.EnvelopeRepository.
// Defined locally so the service does not depend on the full
// repository interface — production stores satisfy it without
// adapter code.
type EvidenceEnvelopeReader interface {
	GetByID(ctx context.Context, id string) (*envelope.Envelope, error)
}

// EvidenceAuditEventReader is the narrow audit-event read contract.
// Subset of audit.AuditEventRepository. Production stores satisfy
// it without adapter code; both the memory and Postgres impls
// guarantee SequenceNo-ASC ordering on ListByEnvelopeID, but the
// envelope-scoped handler also sorts defensively before serialising.
//
// D30c adds List(ctx, filter) for the cross-envelope search route
// at GET /v1/evidence/audit-events. The same audit.ListFilter
// primitive backs the /v1/coverage projection; both endpoints share
// the same indexes on the Postgres side.
type EvidenceAuditEventReader interface {
	ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*audit.AuditEvent, error)
	List(ctx context.Context, filter audit.ListFilter) ([]*audit.AuditEvent, error)
}

// evidenceReadService is the consumer-owned read surface the handler
// depends on. Keeping the interface here (rather than reusing the
// store-side reader types directly on the handler) lets tests inject
// a stub without referencing the production repository types and
// keeps the route's dependency footprint minimal.
//
// D30c adds ListAuditEvents — the cross-envelope search method that
// powers GET /v1/evidence/audit-events.
//
// D30d adds VerifyEnvelopeIntegrity — the per-envelope hash-chain
// verifier that powers GET /v1/evidence/envelopes/{id}/integrity.
type evidenceReadService interface {
	HasEvidence() bool
	GetEnvelope(ctx context.Context, id string) (*envelope.Envelope, error)
	ListEnvelopeAuditEvents(ctx context.Context, envelopeID string) ([]*audit.AuditEvent, error)
	ListAuditEvents(ctx context.Context, filter audit.ListFilter) ([]*audit.AuditEvent, error)
	VerifyEnvelopeIntegrity(ctx context.Context, env *envelope.Envelope) (audit.IntegrityVerificationResult, error)
}

// EvidenceReadService satisfies evidenceReadService by delegating to
// the supplied envelope + audit readers. Both readers can be nil at
// construction time; HasEvidence() returns false when either is
// missing and the routes respond 501 Not Implemented. This mirrors
// the safe-default posture used by FailModePolicyReadService and
// CoverageReadService.
type EvidenceReadService struct {
	envelopes EvidenceEnvelopeReader
	audit     EvidenceAuditEventReader
}

// NewEvidenceReadService constructs an EvidenceReadService with the
// supplied readers. A nil reader is supported — HasEvidence()
// returns false and the routes return 501. The constructor does not
// validate non-nilness so that test wiring can degrade gracefully
// when only one reader is present.
func NewEvidenceReadService(envelopes EvidenceEnvelopeReader, audit EvidenceAuditEventReader) *EvidenceReadService {
	return &EvidenceReadService{envelopes: envelopes, audit: audit}
}

// HasEvidence reports whether both readers are wired. Returns true
// only when the service was constructed with non-nil envelope and
// audit readers; false otherwise. Used by the handler to distinguish
// 501 (not configured) from 404 (configured but envelope absent).
func (s *EvidenceReadService) HasEvidence() bool {
	return s != nil && s.envelopes != nil && s.audit != nil
}

// GetEnvelope returns the production envelope by id, or (nil, nil)
// when not found. The handler interprets (nil, nil) as 404.
func (s *EvidenceReadService) GetEnvelope(ctx context.Context, id string) (*envelope.Envelope, error) {
	return s.envelopes.GetByID(ctx, id)
}

// ListEnvelopeAuditEvents returns the audit-event chain for the
// supplied envelope id. Ordering is the repository's natural order
// (SequenceNo ASC for both memory and Postgres impls); the handler
// applies a defensive sort before serialising.
func (s *EvidenceReadService) ListEnvelopeAuditEvents(ctx context.Context, envelopeID string) ([]*audit.AuditEvent, error) {
	return s.audit.ListByEnvelopeID(ctx, envelopeID)
}

// ListAuditEvents (D30c) returns audit events matching the supplied
// filter, ordered per filter.OrderDesc and capped per filter.Limit
// (default 100, max 500). Returns audit.ErrInvalidTimeRange when the
// caller supplies an inverted Since/Until pair; the handler surfaces
// that as 400.
func (s *EvidenceReadService) ListAuditEvents(ctx context.Context, filter audit.ListFilter) ([]*audit.AuditEvent, error) {
	return s.audit.List(ctx, filter)
}

// VerifyEnvelopeIntegrity (D30d) checks the audit-event chain for one
// envelope and returns a structured result. (result, nil) indicates
// the chain was inspected — Valid=true on success, Valid=false on an
// integrity finding. (zero result, err) is reserved for repository /
// hash-compute failures that prevent verification; the handler maps
// those to 500.
//
// The audit reader satisfies audit.AuditRepository's ListByEnvelopeID
// requirement via the existing EvidenceAuditEventReader interface, so
// no adapter code is needed.
func (s *EvidenceReadService) VerifyEnvelopeIntegrity(ctx context.Context, env *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
	return audit.VerifyEnvelopeIntegrity(ctx, s.audit, env)
}

var _ evidenceReadService = (*EvidenceReadService)(nil)

// ---------------------------------------------------------------------------
// Response DTOs
// ---------------------------------------------------------------------------

// runtimeAuditEvent is the wire shape for one audit event on the
// production /v1/evidence/envelopes/{id}/audit-events response. The
// JSON tag set is byte-identical to the Explorer route's
// auditEventResponse so SIEM and integration consumers that already
// read the Explorer shape can target the production route with no
// transform. The internal Go type is separate to keep the production
// and Explorer wire shapes evolvable independently.
type runtimeAuditEvent struct {
	ID            string         `json:"id"`
	EnvelopeID    string         `json:"envelope_id"`
	RequestSource string         `json:"request_source"`
	RequestID     string         `json:"request_id"`
	SequenceNo    int            `json:"sequence_no"`
	EventType     string         `json:"event_type"`
	PerformerType string         `json:"performer_type"`
	PerformerID   string         `json:"performer_id"`
	Payload       map[string]any `json:"payload"`
	OccurredAt    time.Time      `json:"occurred_at"`
	Hash          string         `json:"hash"`
	PrevHash      string         `json:"prev_hash,omitempty"`
}

// runtimeAuditEventListResponse is the wire shape for
// GET /v1/evidence/envelopes/{id}/audit-events. items is always
// present (never null) — empty list is an empty array. count equals
// len(items) since this endpoint does not paginate (audit chains
// are bounded by evaluation-step count). Cross-envelope search and
// pagination are deferred to D30c.
type runtimeAuditEventListResponse struct {
	EnvelopeID string              `json:"envelope_id"`
	Items      []runtimeAuditEvent `json:"items"`
	Count      int                 `json:"count"`
}

// ---------------------------------------------------------------------------
// Prefix dispatcher
// ---------------------------------------------------------------------------

// handleEvidenceEnvelopesPrefix dispatches sub-paths under
// /v1/evidence/envelopes/. Today the only wired sub-path is
// {id}/audit-events; every other shape — including the bare
// /v1/evidence/envelopes/{id} (deferred to D30e packet export) —
// returns 404 so the surface stays explicit.
//
// Non-GET methods receive 405. The prefix does not collide with any
// other registered route; the longest-match ServeMux rule keeps
// /v1/envelopes/ and /v1/evidence/envelopes/ separate.
func (s *Server) handleEvidenceEnvelopesPrefix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	const prefix = "/v1/evidence/envelopes/"
	tail := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if tail == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	parts := strings.Split(tail, "/")
	id := parts[0]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing envelope id"})
		return
	}
	if !isValidIdentifier(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid envelope id"})
		return
	}

	switch {
	case len(parts) == 2 && parts[1] == "audit-events":
		s.handleGetEvidenceEnvelopeAuditEvents(w, r, id)
	case len(parts) == 2 && parts[1] == "integrity":
		s.handleGetEvidenceEnvelopeIntegrity(w, r, id)
	case len(parts) == 2 && parts[1] == "packet":
		s.handleGetEvidencePacket(w, r, id)
	default:
		// Including len(parts)==1 (the bare /v1/evidence/envelopes/{id}
		// shape) and any other sub-path. The D30 series is complete;
		// any future sub-path is a new tranche and gets its own arm.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleGetEvidenceEnvelopeAuditEvents serves
// GET /v1/evidence/envelopes/{id}/audit-events.
//
// Contract:
//
//   - 501 when the evidence read service is not wired (nil service
//     or nil envelope/audit readers). Matches the project's
//     not-configured posture used by /v1/coverage and
//     /v1/fail_mode_policies/*.
//   - 400 when the envelope id is empty or fails isValidIdentifier.
//     (The prefix dispatcher already enforces these checks; the
//     handler accepts the validated id and does not re-check.)
//   - 404 when the envelope does not exist. The existence gate is
//     deliberate: returning an empty audit-event list for an unknown
//     envelope would be a directory-scan oracle.
//   - 500 when either repository returns an error.
//   - 200 with the (defensively sorted) audit-event chain otherwise.
//
// The handler does not return the envelope's Submitted.Raw payload;
// callers that need envelope detail use GET /v1/envelopes/{id}.
func (s *Server) handleGetEvidenceEnvelopeAuditEvents(w http.ResponseWriter, r *http.Request, envelopeID string) {
	if s.evidenceRead == nil || !s.evidenceRead.HasEvidence() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "evidence read service not configured",
		})
		return
	}

	env, err := s.evidenceRead.GetEnvelope(r.Context(), envelopeID)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope, false)
		writeJSON(w, statusCode, errResp)
		return
	}
	if env == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "envelope not found"})
		return
	}

	events, err := s.evidenceRead.ListEnvelopeAuditEvents(r.Context(), envelopeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Defensive sort: both production repositories already return
	// SequenceNo-ASC (Postgres via ORDER BY, memory via append-order
	// which matches the monotonic per-envelope sequence numbers),
	// but a stable sort here protects the wire contract against any
	// future repository contract change.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].SequenceNo < events[j].SequenceNo
	})

	items := make([]runtimeAuditEvent, 0, len(events))
	for _, ev := range events {
		if ev == nil {
			continue
		}
		items = append(items, runtimeAuditEvent{
			ID:            ev.ID,
			EnvelopeID:    ev.EnvelopeID,
			RequestSource: ev.RequestSource,
			RequestID:     ev.RequestID,
			SequenceNo:    ev.SequenceNo,
			EventType:     string(ev.EventType),
			PerformerType: string(ev.PerformedByType),
			PerformerID:   ev.PerformedByID,
			Payload:       ev.Payload,
			OccurredAt:    ev.OccurredAt,
			Hash:          ev.Hash,
			PrevHash:      ev.PrevHash,
		})
	}

	writeJSON(w, http.StatusOK, runtimeAuditEventListResponse{
		EnvelopeID: envelopeID,
		Items:      items,
		Count:      len(items),
	})
}

// ---------------------------------------------------------------------------
// D30c — cross-envelope audit-event search
// ---------------------------------------------------------------------------

// runtimeAuditEventSearchResponse is the wire shape for
// GET /v1/evidence/audit-events. Deliberately omits the
// envelope_id field that the envelope-scoped chain response
// carries — search results span envelopes and the wrapper should
// not pretend to scope to one.
//
// items is always present (never null) — empty result is an empty
// array. count equals len(items). The endpoint applies a hard
// upper bound on returned events via the underlying audit
// repository's MaxListLimit (500).
//
// D30j adds next_cursor: an opaque, URL-safe token that the client
// passes verbatim back to the server as the `cursor` query parameter
// to retrieve the next page under the same query. The field is
// omitted from the response (omitempty) when no further pages exist;
// callers therefore stop paginating when next_cursor is absent rather
// than relying on a count comparison.
type runtimeAuditEventSearchResponse struct {
	Items      []runtimeAuditEvent `json:"items"`
	Count      int                 `json:"count"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

// handleSearchEvidenceAuditEvents serves
// GET /v1/evidence/audit-events.
//
// Query parameters (all optional):
//   - event_type      single event-type tag
//   - event_types     CSV; wins over event_type when non-empty
//   - envelope_id     exact envelope id
//   - request_source  exact request source
//   - request_id      exact request id
//   - since           RFC3339; inclusive lower bound on occurred_at
//   - until           RFC3339; exclusive upper bound on occurred_at
//   - limit           1..audit.MaxListLimit; empty → repo default
//   - order           "asc" or "desc"; default "desc"
//   - cursor          opaque token from a prior response's
//                     next_cursor; bound to the order direction of
//                     the original request (replaying a desc cursor
//                     against an asc query is rejected)
//
// Contract:
//   - 501 when the evidence read service is not wired.
//   - 400 on invalid query parameter shape (non-numeric / negative /
//     zero / oversize limit, unparseable since/until, inverted time
//     range, empty event_types token, unknown order value,
//     malformed/order-mismatched cursor).
//   - 405 on non-GET method.
//   - 500 on repository error.
//   - 200 with the (possibly empty) search response otherwise.
//
// Pagination model (D30j): cursor-based. The handler internally
// requests "wanted+1" rows from the repository to detect whether a
// further page exists; the extra probe row is trimmed before
// serialising and converted into next_cursor. Callers stop
// paginating when next_cursor is absent from the response.
//
// Deliberately not exposed: payload_contains (filter primitive
// supports it; URL-query surface intentionally does not), offset
// or page-number pagination (the cursor-based model is the only
// supported pagination surface), submitted-raw envelope payload
// (only exposed by GET /v1/envelopes/{id}).
func (s *Server) handleSearchEvidenceAuditEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.evidenceRead == nil || !s.evidenceRead.HasEvidence() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "evidence read service not configured",
		})
		return
	}

	q := r.URL.Query()

	// limit — empty → 0 (repo applies DefaultListLimit). Reject
	// non-numeric, negative, zero, and oversize.
	limit := 0
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		parsed, perr := parsePositiveInt(v)
		if perr != nil || parsed < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit must be a positive integer",
			})
			return
		}
		if parsed > audit.MaxListLimit {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit exceeds maximum allowed value",
			})
			return
		}
		limit = parsed
	}

	since, err := parseRFC3339Param(q.Get("since"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "since must be an RFC3339 timestamp",
		})
		return
	}
	until, err := parseRFC3339Param(q.Get("until"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "until must be an RFC3339 timestamp",
		})
		return
	}

	// order — default desc; "asc" flips OrderDesc to false; any
	// other value is a hard 400 so typos do not silently change the
	// result ordering.
	orderDesc := true
	if v := strings.TrimSpace(q.Get("order")); v != "" {
		switch strings.ToLower(v) {
		case "desc":
			orderDesc = true
		case "asc":
			orderDesc = false
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "order must be 'asc' or 'desc'",
			})
			return
		}
	}

	// event_types — CSV with whitespace trimmed per token. Empty
	// tokens are a 400 (rather than silently dropping them) so the
	// caller learns about malformed input. When non-empty,
	// EventTypes wins over EventType in audit.ListFilter semantics.
	var eventTypes []audit.AuditEventType
	if v := strings.TrimSpace(q.Get("event_types")); v != "" {
		raw := strings.Split(v, ",")
		eventTypes = make([]audit.AuditEventType, 0, len(raw))
		for _, tok := range raw {
			t := strings.TrimSpace(tok)
			if t == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "event_types contains an empty token",
				})
				return
			}
			eventTypes = append(eventTypes, audit.AuditEventType(t))
		}
	}

	// cursor (D30j) — optional opaque pagination token from a prior
	// response. Decoding is bound to the current order direction so
	// a desc cursor replayed against an asc query is rejected with
	// 400 (rather than silently producing a gap/overlap). All
	// shape/parse errors collapse to "invalid cursor"; the
	// order-mismatch case gets its own message so operators can
	// diagnose the typo quickly.
	var cursor *audit.ListCursor
	if v := strings.TrimSpace(q.Get("cursor")); v != "" {
		parsed, cerr := audit.DecodeListCursor(v, orderDesc)
		if cerr != nil {
			switch {
			case errors.Is(cerr, audit.ErrCursorOrderMismatch):
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "cursor order does not match request order",
				})
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "cursor is malformed",
				})
			}
			return
		}
		cursor = parsed
	}

	// wanted is the page size the client asked for (or the repo
	// default when unspecified). The repository is asked for one
	// extra row so the handler can detect a next page without a
	// second round trip; the probe row is trimmed before serialising
	// and converted into next_cursor below.
	wanted := audit.DefaultListLimit
	if limit > 0 {
		wanted = limit
	}

	filter := audit.ListFilter{
		EventType:     audit.AuditEventType(strings.TrimSpace(q.Get("event_type"))),
		EventTypes:    eventTypes,
		EnvelopeID:    strings.TrimSpace(q.Get("envelope_id")),
		RequestSource: strings.TrimSpace(q.Get("request_source")),
		RequestID:     strings.TrimSpace(q.Get("request_id")),
		Since:         since,
		Until:         until,
		Limit:         wanted + 1,
		OrderDesc:     orderDesc,
		Cursor:        cursor,
	}

	// Pre-validate the time range so the handler can surface a
	// clear 400 message rather than the repository's wrapped error.
	if vErr := filter.Validate(); vErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": vErr.Error(),
		})
		return
	}

	events, err := s.evidenceRead.ListAuditEvents(r.Context(), filter)
	if err != nil {
		// Defensive: a repo that surfaces ErrInvalidTimeRange (e.g.
		// because of a future contract change) should still be a
		// 400 to the caller — not a 500.
		if errors.Is(err, audit.ErrInvalidTimeRange) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// D30j — detect a next page from the probe row, trim to the
	// caller's requested size, and synthesise next_cursor from the
	// last row that survives the trim. When the repo returned ≤
	// wanted rows there is no next page; next_cursor is left empty
	// and the response omits the field via omitempty.
	hasMore := len(events) > wanted
	if hasMore {
		events = events[:wanted]
	}

	items := make([]runtimeAuditEvent, 0, len(events))
	for _, ev := range events {
		if ev == nil {
			continue
		}
		items = append(items, runtimeAuditEvent{
			ID:            ev.ID,
			EnvelopeID:    ev.EnvelopeID,
			RequestSource: ev.RequestSource,
			RequestID:     ev.RequestID,
			SequenceNo:    ev.SequenceNo,
			EventType:     string(ev.EventType),
			PerformerType: string(ev.PerformedByType),
			PerformerID:   ev.PerformedByID,
			Payload:       ev.Payload,
			OccurredAt:    ev.OccurredAt,
			Hash:          ev.Hash,
			PrevHash:      ev.PrevHash,
		})
	}

	var nextCursor string
	if hasMore && len(events) > 0 {
		last := events[len(events)-1]
		nextCursor = audit.EncodeListCursor(&audit.ListCursor{
			OccurredAt: last.OccurredAt,
			SequenceNo: last.SequenceNo,
			ID:         last.ID,
		}, orderDesc)
	}

	writeJSON(w, http.StatusOK, runtimeAuditEventSearchResponse{
		Items:      items,
		Count:      len(items),
		NextCursor: nextCursor,
	})
}

// ---------------------------------------------------------------------------
// D30d — per-envelope audit-chain integrity verification
// ---------------------------------------------------------------------------

// runtimeEvidenceIntegrityResponse is the wire shape for
// GET /v1/evidence/envelopes/{id}/integrity. Valid=true indicates the
// audit-event chain for the envelope passes every check; Valid=false
// indicates an integrity finding (the response is still HTTP 200 —
// the endpoint reports status, not transport failure). ErrorKind and
// ErrorMessage are populated only when Valid=false.
//
// checked_at is the HTTP-layer timestamp at which the verification
// ran. It is deliberately not stored on the audit substrate — the
// verifier is stateless and can be re-run idempotently.
//
// The response intentionally omits audit-event payloads, audit-event
// IDs, the envelope's Submitted.Raw, the envelope's SubmittedHash,
// and any other content beyond integrity status; callers that need
// envelope or chain detail use /v1/envelopes/{id} or
// /v1/evidence/envelopes/{id}/audit-events.
type runtimeEvidenceIntegrityResponse struct {
	EnvelopeID     string    `json:"envelope_id"`
	Valid          bool      `json:"valid"`
	ChainLength    int       `json:"chain_length"`
	FirstEventHash string    `json:"first_event_hash"`
	FinalEventHash string    `json:"final_event_hash"`
	CheckedAt      time.Time `json:"checked_at"`
	ErrorKind      string    `json:"error_kind,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
}

// handleGetEvidenceEnvelopeIntegrity serves
// GET /v1/evidence/envelopes/{id}/integrity. Dispatched by
// handleEvidenceEnvelopesPrefix when parts[1] == "integrity".
//
// Contract:
//
//   - 501 when the evidence read service is not wired.
//   - 400 is enforced by the dispatcher before this handler runs.
//   - 404 when the envelope does not exist.
//   - 500 on repository / hash-compute error (verifier could not
//     complete; integrity status is undecided).
//   - 200 with Valid=true when every check passes.
//   - 200 with Valid=false when an integrity finding is reported.
//
// The handler does not expose audit-event payloads, audit-event IDs,
// or Envelope.Submitted.Raw.
func (s *Server) handleGetEvidenceEnvelopeIntegrity(w http.ResponseWriter, r *http.Request, envelopeID string) {
	if s.evidenceRead == nil || !s.evidenceRead.HasEvidence() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "evidence read service not configured",
		})
		return
	}

	env, err := s.evidenceRead.GetEnvelope(r.Context(), envelopeID)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope, false)
		writeJSON(w, statusCode, errResp)
		return
	}
	if env == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "envelope not found"})
		return
	}

	result, err := s.evidenceRead.VerifyEnvelopeIntegrity(r.Context(), env)
	if err != nil {
		// Repository / hash-compute failure prevents verification.
		// Raw error message is bounded — the audit helper wraps with
		// "listing events: …" or "failed to compute hash at sequence N: …".
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := runtimeEvidenceIntegrityResponse{
		EnvelopeID:     result.EnvelopeID,
		Valid:          result.Valid,
		ChainLength:    result.ChainLength,
		FirstEventHash: result.FirstEventHash,
		FinalEventHash: result.FinalEventHash,
		CheckedAt:      time.Now().UTC(),
	}
	if !result.Valid {
		resp.ErrorKind = string(result.ErrorKind)
		resp.ErrorMessage = result.ErrorMessage
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// D30e — runtime evidence packet export
// ---------------------------------------------------------------------------

// runtimeEvidencePacketResponse is the wire shape for
// GET /v1/evidence/envelopes/{id}/packet. The packet is an
// envelope-scoped composition of three existing reads — the
// envelope (same shape as /v1/envelopes/{id}), the audit-event chain
// (same items as /v1/evidence/envelopes/{id}/audit-events, without
// the wrapper count), and the integrity verification result (same
// shape as /v1/evidence/envelopes/{id}/integrity). No new shapes
// are introduced.
//
// envelope_id equals the path id and Envelope.Identity.ID;
// generated_at is the UTC timestamp at which the handler composed
// the packet. Both are convenience fields for export consumers; the
// underlying evidence is what the three composed routes already
// expose.
//
// Submitted.Raw appears only inside the embedded Envelope, mirroring
// the existing /v1/envelopes/{id} contract. The packet wrapper has
// no top-level submitted field.
type runtimeEvidencePacketResponse struct {
	EnvelopeID  string                           `json:"envelope_id"`
	GeneratedAt time.Time                        `json:"generated_at"`
	Envelope    *envelope.Envelope               `json:"envelope"`
	AuditEvents []runtimeAuditEvent              `json:"audit_events"`
	Integrity   runtimeEvidenceIntegrityResponse `json:"integrity"`
}

// handleGetEvidencePacket serves
// GET /v1/evidence/envelopes/{id}/packet. Dispatched by
// handleEvidenceEnvelopesPrefix when parts[1] == "packet".
//
// Composition: GetEnvelope → ListEnvelopeAuditEvents →
// VerifyEnvelopeIntegrity. The verifier internally re-fetches the
// audit chain; the small duplicate read on a bounded chain is the
// accepted trade-off documented in the D30e brief — reusing the
// existing verifier helper avoids duplicating its logic at this
// surface.
//
// Contract:
//
//   - 501 when the evidence read service is not wired.
//   - 400 is enforced by the prefix dispatcher before this handler.
//   - 404 when the envelope does not exist.
//   - 500 on any underlying read failure (envelope reader, audit
//     reader, or verifier). The packet is never returned partially;
//     either every section completes or the route returns an error.
//   - 200 with integrity.valid=false when the chain is invalid —
//     the packet export succeeded; the packet reports that the
//     evidence is broken.
//
// The handler does not introduce redaction. Submitted.Raw appears
// inside the envelope sub-object exactly as /v1/envelopes/{id}
// returns it; no query parameter changes that.
func (s *Server) handleGetEvidencePacket(w http.ResponseWriter, r *http.Request, envelopeID string) {
	if s.evidenceRead == nil || !s.evidenceRead.HasEvidence() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "evidence read service not configured",
		})
		return
	}

	env, err := s.evidenceRead.GetEnvelope(r.Context(), envelopeID)
	if err != nil {
		statusCode, errResp := mapDomainError(err, entityEnvelope, false)
		writeJSON(w, statusCode, errResp)
		return
	}
	if env == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "envelope not found"})
		return
	}

	events, err := s.evidenceRead.ListEnvelopeAuditEvents(r.Context(), envelopeID)
	if err != nil {
		// Audit-read failure is a partial-packet condition; per the
		// D30e brief the packet must not be returned partially.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	result, err := s.evidenceRead.VerifyEnvelopeIntegrity(r.Context(), env)
	if err != nil {
		// Verifier could not complete — packet export cannot report
		// integrity status, so it cannot be assembled.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Defensive sort by SequenceNo ASC. Both production repository
	// impls already return ascending order; the sort here mirrors
	// the D30b handler and protects against any future repo contract
	// change.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].SequenceNo < events[j].SequenceNo
	})

	items := make([]runtimeAuditEvent, 0, len(events))
	for _, ev := range events {
		if ev == nil {
			continue
		}
		items = append(items, runtimeAuditEvent{
			ID:            ev.ID,
			EnvelopeID:    ev.EnvelopeID,
			RequestSource: ev.RequestSource,
			RequestID:     ev.RequestID,
			SequenceNo:    ev.SequenceNo,
			EventType:     string(ev.EventType),
			PerformerType: string(ev.PerformedByType),
			PerformerID:   ev.PerformedByID,
			Payload:       ev.Payload,
			OccurredAt:    ev.OccurredAt,
			Hash:          ev.Hash,
			PrevHash:      ev.PrevHash,
		})
	}

	integrity := runtimeEvidenceIntegrityResponse{
		EnvelopeID:     result.EnvelopeID,
		Valid:          result.Valid,
		ChainLength:    result.ChainLength,
		FirstEventHash: result.FirstEventHash,
		FinalEventHash: result.FinalEventHash,
		CheckedAt:      time.Now().UTC(),
	}
	if !result.Valid {
		integrity.ErrorKind = string(result.ErrorKind)
		integrity.ErrorMessage = result.ErrorMessage
	}

	writeJSON(w, http.StatusOK, runtimeEvidencePacketResponse{
		EnvelopeID:  envelopeID,
		GeneratedAt: time.Now().UTC(),
		Envelope:    env,
		AuditEvents: items,
		Integrity:   integrity,
	})
}
