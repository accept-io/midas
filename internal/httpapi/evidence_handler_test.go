package httpapi

// evidence_handler_test.go — D30b pins.
//
// Asserts:
//
//   - GET /v1/evidence/envelopes/{id}/audit-events returns 200 with
//     the ordered audit-event chain when the envelope exists.
//   - Events are returned in SequenceNo ascending order even when the
//     repository hands them back out-of-order (defensive sort).
//   - FailModePolicy audit-event payloads round-trip byte-for-byte;
//     no key is dropped, no value is reshaped.
//   - Error contract: 400 on missing/invalid id; 404 on unknown
//     envelope; 405 on non-GET; 500 on repository error; 501 when
//     the read service is not wired.
//   - Sibling sub-paths under /v1/evidence/envelopes/{id}/... are
//     rejected so D30c/D30d/D30e cannot accidentally ship; the bare
//     /v1/evidence/envelopes/{id} shape (deferred to D30e packet
//     export) is also rejected.
//   - The route does not expose Envelope.Submitted.Raw.
//   - OpenAPI declares the path, the two new schemas, and the
//     payload additionalProperties: true shape; the spec does NOT
//     yet declare the D30c/D30d/D30e routes.
//   - The Explorer audit-events route still exists and is not
//     replaced by /v1/evidence/...

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/envelope"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubEvidenceEnvelopeReader satisfies EvidenceEnvelopeReader with a
// caller-provided lookup function. Used to drive every code path in
// the handler without standing up the real envelope repository.
type stubEvidenceEnvelopeReader struct {
	getFn func(ctx context.Context, id string) (*envelope.Envelope, error)
}

func (s *stubEvidenceEnvelopeReader) GetByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return nil, nil
}

// stubEvidenceAuditEventReader satisfies EvidenceAuditEventReader.
// listFilterFn captures the audit.ListFilter passed to List so D30c
// search tests can pin filter mapping.
type stubEvidenceAuditEventReader struct {
	listFn         func(ctx context.Context, envelopeID string) ([]*audit.AuditEvent, error)
	listFilterFn   func(ctx context.Context, filter audit.ListFilter) ([]*audit.AuditEvent, error)
	lastListFilter audit.ListFilter
}

func (s *stubEvidenceAuditEventReader) ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*audit.AuditEvent, error) {
	if s.listFn != nil {
		return s.listFn(ctx, envelopeID)
	}
	return nil, nil
}

func (s *stubEvidenceAuditEventReader) List(ctx context.Context, filter audit.ListFilter) ([]*audit.AuditEvent, error) {
	s.lastListFilter = filter
	if s.listFilterFn != nil {
		return s.listFilterFn(ctx, filter)
	}
	return nil, nil
}

// newEvidenceTestServer constructs a Server wired with the evidence
// read service backed by the supplied stubs. authMode=open removes
// the auth gate so tests can exercise the handler directly.
func newEvidenceTestServer(t *testing.T, envReader EvidenceEnvelopeReader, auditReader EvidenceAuditEventReader) *Server {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(NewEvidenceReadService(envReader, auditReader))
	return srv
}

// envelopeWithID returns a minimal production envelope with the
// supplied id. The handler only consults env != nil and env.ID(); a
// full envelope is unnecessary.
func envelopeWithID(id string) *envelope.Envelope {
	return &envelope.Envelope{
		Identity: envelope.Identity{ID: id, RequestSource: "test-source", RequestID: "test-req"},
		State:    envelope.EnvelopeStateClosed,
	}
}

// auditEvent returns a fully-formed audit event so test assertions
// can pin every wire field. payload is included verbatim.
func auditEvent(envID string, seq int, evType audit.AuditEventType, payload map[string]any) *audit.AuditEvent {
	ev := audit.NewEvent(envID, "test-source", "test-req", evType, audit.EventPerformerSystem, "midas-orchestrator", payload)
	ev.SequenceNo = seq
	ev.OccurredAt = time.Date(2026, 1, 1, 0, 0, seq, 0, time.UTC)
	ev.Hash = "hash-" + envID + "-" + itoaShort(seq)
	if seq > 1 {
		ev.PrevHash = "hash-" + envID + "-" + itoaShort(seq-1)
	}
	return ev
}

// itoaShort avoids pulling strconv just for fixture identifiers.
func itoaShort(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ---------------------------------------------------------------------------
// Happy path + ordering
// ---------------------------------------------------------------------------

func TestEvidence_AuditEvents_HappyPath_ReturnsOrderedChain(t *testing.T) {
	envID := "env-d30b-happy"
	chain := []*audit.AuditEvent{
		// Deliberately delivered out of order so the defensive sort
		// has work to do.
		auditEvent(envID, 3, audit.AuditEventOutcomeRecorded, map[string]any{"outcome": "accept"}),
		auditEvent(envID, 1, audit.AuditEventEnvelopeCreated, map[string]any{"request_source": "test-source"}),
		auditEvent(envID, 2, audit.AuditEventEvaluationStarted, map[string]any{"from_state": "received", "to_state": "evaluating"}),
		auditEvent(envID, 4, audit.AuditEventEnvelopeClosed, map[string]any{"from_state": "outcome_recorded", "to_state": "closed"}),
	}

	envReader := &stubEvidenceEnvelopeReader{
		getFn: func(_ context.Context, id string) (*envelope.Envelope, error) {
			if id == envID {
				return envelopeWithID(envID), nil
			}
			return nil, nil
		},
	}
	auditReader := &stubEvidenceAuditEventReader{
		listFn: func(_ context.Context, id string) ([]*audit.AuditEvent, error) {
			if id == envID {
				return chain, nil
			}
			return nil, nil
		},
	}
	srv := newEvidenceTestServer(t, envReader, auditReader)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp runtimeAuditEventListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EnvelopeID != envID {
		t.Errorf("envelope_id: want %q, got %q", envID, resp.EnvelopeID)
	}
	if resp.Count != len(chain) {
		t.Errorf("count: want %d, got %d", len(chain), resp.Count)
	}
	if len(resp.Items) != len(chain) {
		t.Fatalf("items len: want %d, got %d", len(chain), len(resp.Items))
	}
	// Pin SequenceNo ASC.
	for i, item := range resp.Items {
		if item.SequenceNo != i+1 {
			t.Errorf("items[%d].sequence_no: want %d, got %d", i, i+1, item.SequenceNo)
		}
		if item.EnvelopeID != envID {
			t.Errorf("items[%d].envelope_id: want %q, got %q", i, envID, item.EnvelopeID)
		}
	}
}

// ---------------------------------------------------------------------------
// FailModePolicy payload round-trip
// ---------------------------------------------------------------------------

func TestEvidence_AuditEvents_FailModePolicyPayloadRoundTrips(t *testing.T) {
	envID := "env-d30b-fmp"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	enforcedPayload := map[string]any{
		"fail_mode_policy_id":      "fmp-demo",
		"fail_mode_policy_version": float64(1),
		"source":                   "surface",
		"trigger_condition":        "policy_evaluator_error",
		"correctness_class":        "resource",
		"permitted_mode":           "closed",
		"enforcement_state":        "enforced",
		"configured_outcome":       "deny",
		"enforced_outcome":         "reject",
		"enforced_reason_code":     "FAIL_MODE_POLICY_DENIED",
		"previous_outcome":         "escalate",
		"previous_reason_code":     "POLICY_ERROR",
		"applied_at":               now,
		"evaluation_time":          now,
		"surface_id":               "surf-test",
		"surface_version":          float64(1),
		"business_service_id":      "bs-test",
		"authority_profile_id":     "prof-test",
		"agent_id":                 "agent-test",
		"policy_reference":         "test-policy",
	}
	chain := []*audit.AuditEvent{
		auditEvent(envID, 1, audit.AuditEventEnvelopeCreated, map[string]any{"request_source": "test-source"}),
		auditEvent(envID, 7, audit.AuditEventFailModePolicyEnforced, enforcedPayload),
		auditEvent(envID, 8, audit.AuditEventOutcomeRecorded, map[string]any{"outcome": "reject", "reason_code": "FAIL_MODE_POLICY_DENIED"}),
	}
	srv := newEvidenceTestServer(t,
		&stubEvidenceEnvelopeReader{getFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return envelopeWithID(envID), nil }},
		&stubEvidenceAuditEventReader{listFn: func(_ context.Context, _ string) ([]*audit.AuditEvent, error) { return chain, nil }},
	)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp runtimeAuditEventListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Find the enforced item.
	var found *runtimeAuditEvent
	for i := range resp.Items {
		if resp.Items[i].EventType == string(audit.AuditEventFailModePolicyEnforced) {
			found = &resp.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("FAIL_MODE_POLICY_ENFORCED missing from response; got %v", resp.Items)
	}
	for k, want := range enforcedPayload {
		got, present := found.Payload[k]
		if !present {
			t.Errorf("payload key %q missing from round-trip", k)
			continue
		}
		if got != want {
			t.Errorf("payload[%q]: want %v (%T), got %v (%T)", k, want, want, got, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestEvidence_AuditEvents_UnknownEnvelope_Returns404(t *testing.T) {
	srv := newEvidenceTestServer(t,
		&stubEvidenceEnvelopeReader{getFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return nil, nil }},
		&stubEvidenceAuditEventReader{},
	)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/env-missing/audit-events", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEvidence_AuditEvents_MissingEnvelopeID_Returns400(t *testing.T) {
	srv := newEvidenceTestServer(t, &stubEvidenceEnvelopeReader{}, &stubEvidenceAuditEventReader{})
	// /v1/evidence/envelopes//audit-events normalises to a tail of
	// "/audit-events" which the dispatcher rejects. The cleanest
	// "missing id" case is /v1/evidence/envelopes/ (no id at all)
	// — that returns 404 from the dispatcher. We assert the 400
	// path via the explicit empty-id branch reached when an id is
	// supplied but is just whitespace.
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/%20/audit-events", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for whitespace-only id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEvidence_AuditEvents_InvalidEnvelopeID_Returns400(t *testing.T) {
	srv := newEvidenceTestServer(t, &stubEvidenceEnvelopeReader{}, &stubEvidenceAuditEventReader{})
	// Control characters and backslashes are rejected by
	// isValidIdentifier. Use a backslash which the URL parser leaves
	// intact in the path. (Slash would mis-segment the URL.)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/bad%5Cid/audit-events", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEvidence_AuditEvents_NonGET_Returns405(t *testing.T) {
	srv := newEvidenceTestServer(t, &stubEvidenceEnvelopeReader{}, &stubEvidenceAuditEventReader{})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := performRequest(t, srv, method, "/v1/evidence/envelopes/env-x/audit-events", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d", method, rec.Code)
		}
	}
}

func TestEvidence_AuditEvents_UnknownSubpath_Returns404(t *testing.T) {
	envID := "env-d30b-paths"
	srv := newEvidenceTestServer(t,
		&stubEvidenceEnvelopeReader{getFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return envelopeWithID(envID), nil }},
		&stubEvidenceAuditEventReader{listFn: func(_ context.Context, _ string) ([]*audit.AuditEvent, error) { return nil, nil }},
	)

	// Each of these sub-paths must NOT silently route to a known
	// handler. The D30 series is complete; the bare-id shape and
	// any unrecognised sub-path still return 404 so future-tranche
	// additions must explicitly wire a new dispatcher arm.
	for _, sub := range []string{
		"",      // bare /v1/evidence/envelopes/{id} — no listing endpoint at the envelope level
		"audit", // close-but-not
		"audit-events/extra",
	} {
		path := "/v1/evidence/envelopes/" + envID
		if sub != "" {
			path += "/" + sub
		}
		rec := performRequest(t, srv, http.MethodGet, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("sub-path %q: want 404, got %d (body=%s)", sub, rec.Code, rec.Body.String())
		}
	}
}

func TestEvidence_AuditEvents_ServiceUnwired_Returns501(t *testing.T) {
	// Server constructed WITHOUT WithEvidenceReadService.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).WithAuthMode(config.AuthModeOpen)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/env-x/audit-events", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when service unwired, got %d", rec.Code)
	}

	// Also assert when the service is wired with nil readers — same
	// 501 contract via HasEvidence()=false.
	srv2 := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(NewEvidenceReadService(nil, nil))
	rec2 := performRequest(t, srv2, http.MethodGet, "/v1/evidence/envelopes/env-x/audit-events", nil)
	if rec2.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when readers nil, got %d", rec2.Code)
	}
}

func TestEvidence_AuditEvents_RepoError_Returns500(t *testing.T) {
	envID := "env-d30b-err"
	srv := newEvidenceTestServer(t,
		&stubEvidenceEnvelopeReader{getFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return envelopeWithID(envID), nil }},
		&stubEvidenceAuditEventReader{listFn: func(_ context.Context, _ string) ([]*audit.AuditEvent, error) {
			return nil, errors.New("simulated postgres failure")
		}},
	)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/audit-events", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 on repo error, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Submitted-raw exclusion pin
// ---------------------------------------------------------------------------

func TestEvidence_AuditEvents_DoesNotExposeSubmittedRaw(t *testing.T) {
	envID := "env-d30b-no-submitted"
	chain := []*audit.AuditEvent{
		auditEvent(envID, 1, audit.AuditEventEnvelopeCreated, map[string]any{"request_source": "test-source"}),
	}
	srv := newEvidenceTestServer(t,
		&stubEvidenceEnvelopeReader{getFn: func(_ context.Context, _ string) (*envelope.Envelope, error) {
			env := envelopeWithID(envID)
			// Populate Submitted.Raw with a sentinel string so the test
			// fails loud if any future code-path accidentally surfaces it.
			env.Submitted = envelope.Submitted{Raw: json.RawMessage(`{"sentinel":"MUST_NOT_LEAK"}`)}
			return env, nil
		}},
		&stubEvidenceAuditEventReader{listFn: func(_ context.Context, _ string) ([]*audit.AuditEvent, error) { return chain, nil }},
	)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{
		`"submitted"`, `"raw"`, `MUST_NOT_LEAK`, `submitted_hash`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response must not contain %q (Submitted.Raw exposure); body=%s", forbidden, body)
		}
	}
}

// ---------------------------------------------------------------------------
// OpenAPI source pins
// ---------------------------------------------------------------------------

func TestEvidence_OpenAPI_PathAndSchemasDeclared(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	for _, want := range []string{
		`/v1/evidence/envelopes/{id}/audit-events:`,
		`operationId: getEvidenceEnvelopeAuditEvents`,
		`RuntimeAuditEvent:`,
		`RuntimeAuditEventListResponse:`,
		`$ref: "#/components/schemas/RuntimeAuditEventListResponse"`,
		`$ref: "#/components/schemas/RuntimeAuditEvent"`,
		`additionalProperties: true`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("OpenAPI missing %q", want)
		}
	}
}

// TestEvidence_OpenAPI_NoForwardLookingPaths was the
// D30b-introduced source pin that forbade not-yet-shipped D30 paths
// in api/openapi/v1.yaml. The pin's purpose was to catch tranche
// scope creep before each follow-on landed. With D30e (packet
// export) shipping, every D30 evidence path is now part of the
// spec and the forbidden list is empty — the assertion would be
// vacuous. The test has been removed; the D30e source pins below
// assert each component (packet path, RuntimeEvidencePacket schema,
// $refs to Envelope / RuntimeAuditEvent / RuntimeEvidenceIntegrityResponse)
// is now present.

// ---------------------------------------------------------------------------
// Route-registration + Explorer-route-preservation source pins
// ---------------------------------------------------------------------------

func TestEvidence_RouteRegistered(t *testing.T) {
	body, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	src := string(body)
	// The route must be registered under /v1/evidence/envelopes/ with
	// the viewer+ role tuple, behind requireAuth, and routed through
	// the prefix dispatcher.
	if !strings.Contains(src, `"/v1/evidence/envelopes/"`) {
		t.Error(`server.go must register "/v1/evidence/envelopes/" prefix route`)
	}
	if !strings.Contains(src, `handleEvidenceEnvelopesPrefix`) {
		t.Error(`server.go must dispatch the /v1/evidence/envelopes/ prefix through handleEvidenceEnvelopesPrefix`)
	}
	// Role-floor pin: must match /v1/envelopes/{id}.
	mustContainAll := []string{
		`identity.RolePlatformViewer`,
		`identity.RolePlatformOperator`,
		`identity.RolePlatformAdmin`,
	}
	// Locate the registration line and assert the three role
	// constants appear on it. Cheaper than parsing the AST: the
	// route registration form is single-line in this codebase.
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, `"/v1/evidence/envelopes/"`) {
			for _, want := range mustContainAll {
				if !strings.Contains(line, want) {
					t.Errorf("/v1/evidence/envelopes/ registration must reference %q; got %q", want, line)
				}
			}
			if !strings.Contains(line, `s.requireAuth`) {
				t.Errorf("/v1/evidence/envelopes/ must be wrapped in s.requireAuth; got %q", line)
			}
			return
		}
	}
	t.Fatal("did not find a /v1/evidence/envelopes/ HandleFunc line in server.go")
}

func TestEvidence_ExplorerAuditEventsRoute_StillExists(t *testing.T) {
	// Source pin: D30b is parallel to the Explorer route, not a
	// replacement. The Explorer audit-events handler must still be
	// referenced from the explorer dispatcher, and the Explorer
	// route registration must still exist.
	authBody, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatalf("read auth.go: %v", err)
	}
	if !strings.Contains(string(authBody), `"GET /explorer/envelopes/"`) {
		t.Error("auth.go must still register GET /explorer/envelopes/ (Explorer route is parallel to D30b, not replaced)")
	}
	explorerBody, err := os.ReadFile("explorer.go")
	if err != nil {
		t.Fatalf("read explorer.go: %v", err)
	}
	if !strings.Contains(string(explorerBody), `handleExplorerListEnvelopeAuditEvents`) {
		t.Error("explorer.go must still define handleExplorerListEnvelopeAuditEvents")
	}
}

// ---------------------------------------------------------------------------
// D30c — cross-envelope audit-event search
// ---------------------------------------------------------------------------

// newEvidenceSearchTestServer is a thin convenience over
// newEvidenceTestServer for tests that only use the audit reader and
// would otherwise need to construct a no-op envelope stub.
func newEvidenceSearchTestServer(t *testing.T, auditReader *stubEvidenceAuditEventReader) *Server {
	t.Helper()
	return newEvidenceTestServer(t, &stubEvidenceEnvelopeReader{}, auditReader)
}

func TestEvidence_Search_EmptyResultReturnsArrayAndZeroCount(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return nil, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp runtimeAuditEventSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("count: want 0, got %d", resp.Count)
	}
	if resp.Items == nil {
		t.Error("items must be a non-nil empty array, not null")
	}
	if len(resp.Items) != 0 {
		t.Errorf("items: want empty, got %d", len(resp.Items))
	}
	// Top-level wrapper must not contain envelope_id since search
	// results may span envelopes.
	body := rec.Body.String()
	if strings.Contains(body, `"envelope_id"`) {
		// Re-decode as map to confirm the field is absent at top
		// level — events themselves carry envelope_id, but those
		// keys only appear inside items[].
		// An empty items[] makes any "envelope_id" in body a wrapper-level leak.
		t.Errorf("search response body must NOT contain top-level envelope_id; got body=%s", body)
	}
}

func TestEvidence_Search_EventTypeFilter_PassedToRepo(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?event_type=FAIL_MODE_POLICY_ENFORCED", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got := string(auditReader.lastListFilter.EventType); got != "FAIL_MODE_POLICY_ENFORCED" {
		t.Errorf("filter.EventType: want %q, got %q", "FAIL_MODE_POLICY_ENFORCED", got)
	}
	if len(auditReader.lastListFilter.EventTypes) != 0 {
		t.Errorf("filter.EventTypes: want empty when only event_type set; got %v", auditReader.lastListFilter.EventTypes)
	}
}

func TestEvidence_Search_EventTypesFilter_WinsOverEventType(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?event_type=A&event_types=B,C", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	// Both fields are forwarded as-is; the audit.ListFilter
	// precedence rule (EventTypes wins when non-empty) is enforced
	// by the repository, not by the handler. Pin that both were
	// captured so the precedence behaves correctly end-to-end.
	if got := string(auditReader.lastListFilter.EventType); got != "A" {
		t.Errorf("filter.EventType: want %q, got %q", "A", got)
	}
	want := []audit.AuditEventType{"B", "C"}
	got := auditReader.lastListFilter.EventTypes
	if len(got) != len(want) {
		t.Fatalf("filter.EventTypes: want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("filter.EventTypes[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestEvidence_Search_EnvelopeIDFilter_PassedToRepo(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?envelope_id=env-d30c-foo", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got := auditReader.lastListFilter.EnvelopeID; got != "env-d30c-foo" {
		t.Errorf("filter.EnvelopeID: want %q, got %q", "env-d30c-foo", got)
	}
}

func TestEvidence_Search_RequestSourceAndRequestID_PassedToRepo(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?request_source=svc-A&request_id=req-42", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got := auditReader.lastListFilter.RequestSource; got != "svc-A" {
		t.Errorf("filter.RequestSource: want %q, got %q", "svc-A", got)
	}
	if got := auditReader.lastListFilter.RequestID; got != "req-42" {
		t.Errorf("filter.RequestID: want %q, got %q", "req-42", got)
	}
}

func TestEvidence_Search_SinceUntil_PassedToRepo(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	since := "2026-01-01T00:00:00Z"
	until := "2026-01-02T00:00:00Z"
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?since="+since+"&until="+until, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	wantSince, _ := time.Parse(time.RFC3339, since)
	wantUntil, _ := time.Parse(time.RFC3339, until)
	if !auditReader.lastListFilter.Since.Equal(wantSince) {
		t.Errorf("filter.Since: want %v, got %v", wantSince, auditReader.lastListFilter.Since)
	}
	if !auditReader.lastListFilter.Until.Equal(wantUntil) {
		t.Errorf("filter.Until: want %v, got %v", wantUntil, auditReader.lastListFilter.Until)
	}
}

func TestEvidence_Search_LimitDefault_PassesDefaultPlusOneProbe(t *testing.T) {
	// D30j: when the caller omits ?limit, the handler treats the
	// "wanted" page size as audit.DefaultListLimit and asks the
	// repository for one extra row so a next-page probe is possible
	// without a second round-trip. Public callers cannot reach the
	// MaxListLimit+1 branch — the handler caps user-supplied limit
	// at MaxListLimit before computing wanted+1.
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if auditReader.lastListFilter.Limit != audit.DefaultListLimit+1 {
		t.Errorf("filter.Limit: want DefaultListLimit+1 (%d), got %d",
			audit.DefaultListLimit+1, auditReader.lastListFilter.Limit)
	}
}

func TestEvidence_Search_LimitInvalid_Returns400(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"non_numeric", "/v1/evidence/audit-events?limit=abc"},
		{"negative", "/v1/evidence/audit-events?limit=-1"},
		{"zero", "/v1/evidence/audit-events?limit=0"},
		{"oversize", "/v1/evidence/audit-events?limit=501"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			auditReader := &stubEvidenceAuditEventReader{
				listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
			}
			srv := newEvidenceSearchTestServer(t, auditReader)
			rec := performRequest(t, srv, http.MethodGet, c.query, nil)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("want 400 for %s, got %d: %s", c.name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestEvidence_Search_LimitMax_Accepted(t *testing.T) {
	// D30j: limit=500 (MaxListLimit) is the largest user-facing value
	// the handler accepts. Internally it asks the repository for 501
	// rows so the has-next probe still fires at the maximum page
	// size. EffectiveLimit's MaxListLimit+1 branch was added in D30j
	// to permit this internal value while keeping user-supplied
	// limit > MaxListLimit a hard 400.
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?limit=500", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for max limit, got %d", rec.Code)
	}
	if auditReader.lastListFilter.Limit != audit.MaxListLimit+1 {
		t.Errorf("filter.Limit: want MaxListLimit+1 (%d), got %d",
			audit.MaxListLimit+1, auditReader.lastListFilter.Limit)
	}
}

func TestEvidence_Search_OrderDescDefault(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !auditReader.lastListFilter.OrderDesc {
		t.Error("default order must produce OrderDesc=true (newest first)")
	}
}

func TestEvidence_Search_OrderAsc_PassedToRepo(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events?order=asc", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if auditReader.lastListFilter.OrderDesc {
		t.Error("order=asc must produce OrderDesc=false")
	}
}

func TestEvidence_Search_OrderInvalid_Returns400(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	for _, q := range []string{
		"/v1/evidence/audit-events?order=ascending",
		"/v1/evidence/audit-events?order=newest",
		"/v1/evidence/audit-events?order=desc1",
	} {
		rec := performRequest(t, srv, http.MethodGet, q, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("want 400 for %q, got %d", q, rec.Code)
		}
	}
}

func TestEvidence_Search_InvalidSince_Returns400(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?since=not-a-time", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid since, got %d", rec.Code)
	}
}

func TestEvidence_Search_InvalidUntil_Returns400(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?until=2026%2F01%2F01", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid until, got %d", rec.Code)
	}
}

func TestEvidence_Search_UntilBeforeSince_Returns400(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			t.Fatal("repository must NOT be called when the time range is invalid")
			return nil, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?since=2026-01-02T00:00:00Z&until=2026-01-01T00:00:00Z", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for until<since, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEvidence_Search_EmptyEventTypesToken_Returns400(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			t.Fatal("repository must NOT be called when event_types has empty token")
			return nil, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	for _, q := range []string{
		"/v1/evidence/audit-events?event_types=A,,B",
		"/v1/evidence/audit-events?event_types=,B",
		"/v1/evidence/audit-events?event_types=A,",
		"/v1/evidence/audit-events?event_types=A,%20%20,B",
	} {
		rec := performRequest(t, srv, http.MethodGet, q, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("want 400 for %q, got %d", q, rec.Code)
		}
	}
}

func TestEvidence_Search_EventTypesWhitespaceTrimmed(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?event_types=%20A%20,%20B%20", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	want := []audit.AuditEventType{"A", "B"}
	got := auditReader.lastListFilter.EventTypes
	if len(got) != len(want) {
		t.Fatalf("filter.EventTypes: want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("filter.EventTypes[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestEvidence_Search_FailModePolicyPayloadRoundTrips(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	enforcedPayload := map[string]any{
		"fail_mode_policy_id":      "fmp-demo",
		"fail_mode_policy_version": float64(1),
		"source":                   "surface",
		"trigger_condition":        "policy_evaluator_error",
		"correctness_class":        "resource",
		"permitted_mode":           "closed",
		"enforcement_state":        "enforced",
		"configured_outcome":       "deny",
		"enforced_outcome":         "reject",
		"enforced_reason_code":     "FAIL_MODE_POLICY_DENIED",
		"previous_outcome":         "escalate",
		"previous_reason_code":     "POLICY_ERROR",
		"applied_at":               now,
		"evaluation_time":          now,
	}
	ev := auditEvent("env-d30c-fmp", 7, audit.AuditEventFailModePolicyEnforced, enforcedPayload)

	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return []*audit.AuditEvent{ev}, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp runtimeAuditEventSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("count: want 1, got %d", resp.Count)
	}
	if resp.Items[0].EventType != string(audit.AuditEventFailModePolicyEnforced) {
		t.Errorf("event_type: want %q, got %q",
			audit.AuditEventFailModePolicyEnforced, resp.Items[0].EventType)
	}
	for k, want := range enforcedPayload {
		got, present := resp.Items[0].Payload[k]
		if !present {
			t.Errorf("payload key %q missing from search round-trip", k)
			continue
		}
		if got != want {
			t.Errorf("payload[%q]: want %v, got %v", k, want, got)
		}
	}
}

func TestEvidence_Search_ServiceUnwired_Returns501(t *testing.T) {
	// Server constructed WITHOUT WithEvidenceReadService.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).WithAuthMode(config.AuthModeOpen)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when service unwired, got %d", rec.Code)
	}
	// Wired with nil readers — same contract.
	srv2 := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(NewEvidenceReadService(nil, nil))
	rec2 := performRequest(t, srv2, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec2.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when readers nil, got %d", rec2.Code)
	}
}

func TestEvidence_Search_RepoError_Returns500(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return nil, errors.New("simulated postgres failure")
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 on repo error, got %d", rec.Code)
	}
}

// TestEvidence_Search_RepoErrInvalidTimeRange_Returns400 pins the
// defensive 400-mapping in the search handler: if the audit
// repository surfaces ErrInvalidTimeRange (e.g. because the
// handler's pre-validation contract changed), the handler still
// returns 400 — not 500.
func TestEvidence_Search_RepoErrInvalidTimeRange_Returns400(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return nil, audit.ErrInvalidTimeRange
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 on ErrInvalidTimeRange from repo, got %d", rec.Code)
	}
}

func TestEvidence_Search_NonGET_Returns405(t *testing.T) {
	srv := newEvidenceSearchTestServer(t, &stubEvidenceAuditEventReader{})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := performRequest(t, srv, method, "/v1/evidence/audit-events", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d", method, rec.Code)
		}
	}
}

func TestEvidence_Search_DoesNotExposeSubmittedRaw(t *testing.T) {
	// Search route never loads envelopes, so Submitted.Raw cannot
	// be on its code path — pin the invariant explicitly so a future
	// refactor cannot quietly add an envelope-loading branch.
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return []*audit.AuditEvent{auditEvent("env-d30c", 1, audit.AuditEventEnvelopeCreated, map[string]any{
				// Sentinel value sneaked into an audit-event payload to
				// pin that the route writes payloads as recorded but
				// without naming Submitted.Raw fields.
				"request_source": "test-source",
			})}, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{
		`"submitted"`, `"raw"`, `submitted_hash`, `Submitted.Raw`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("search response must NOT contain %q; body=%s", forbidden, body)
		}
	}
}

func TestEvidence_Search_OpenAPI_PathAndSchemaDeclared(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	for _, want := range []string{
		`/v1/evidence/audit-events:`,
		`operationId: searchEvidenceAuditEvents`,
		// All eight query parameters.
		`- name: event_type`,
		`- name: event_types`,
		`- name: envelope_id`,
		`- name: request_source`,
		`- name: request_id`,
		`- name: since`,
		`- name: until`,
		`- name: limit`,
		`- name: order`,
		// New schema + reused RuntimeAuditEvent.
		`RuntimeAuditEventSearchResponse:`,
		`$ref: "#/components/schemas/RuntimeAuditEventSearchResponse"`,
		`$ref: "#/components/schemas/RuntimeAuditEvent"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("OpenAPI missing %q", want)
		}
	}
}

func TestEvidence_Search_OpenAPI_NoEventTypeEnum(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	// The new event_type query parameter must be free-text. Look for
	// the line block and assert no `enum:` follows under it before the
	// next `- name:` token. Cheap-and-explicit: extract the slice and
	// scan.
	idx := strings.Index(src, `- name: event_type`)
	if idx < 0 {
		t.Fatalf("event_type parameter block missing")
	}
	// Look at the next ~250 bytes after the parameter name.
	end := idx + 400
	if end > len(src) {
		end = len(src)
	}
	slice := src[idx:end]
	if strings.Contains(slice, `enum:`) {
		t.Errorf("event_type / event_types parameters must NOT enumerate audit event types; slice=%s", slice)
	}
}

func TestEvidence_Search_RouteRegistered(t *testing.T) {
	body, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	src := string(body)
	if !strings.Contains(src, `"/v1/evidence/audit-events"`) {
		t.Error(`server.go must register "/v1/evidence/audit-events" route`)
	}
	if !strings.Contains(src, `handleSearchEvidenceAuditEvents`) {
		t.Error(`server.go must dispatch /v1/evidence/audit-events through handleSearchEvidenceAuditEvents`)
	}
	for _, line := range strings.Split(src, "\n") {
		if !strings.Contains(line, `"/v1/evidence/audit-events"`) {
			continue
		}
		for _, want := range []string{
			`identity.RolePlatformViewer`,
			`identity.RolePlatformOperator`,
			`identity.RolePlatformAdmin`,
			`s.requireAuth`,
		} {
			if !strings.Contains(line, want) {
				t.Errorf("/v1/evidence/audit-events registration must reference %q; got %q", want, line)
			}
		}
		return
	}
	t.Fatal("did not find /v1/evidence/audit-events HandleFunc line in server.go")
}

// TestEvidence_Search_EnvelopeScopedRouteStillExists confirms D30c
// did not replace or alter the D30b envelope-scoped chain route.
func TestEvidence_Search_EnvelopeScopedRouteStillExists(t *testing.T) {
	body, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	if !strings.Contains(string(body), `"/v1/evidence/envelopes/"`) {
		t.Error("server.go must still register /v1/evidence/envelopes/ (D30b route is parallel to D30c, not replaced)")
	}
}

// ---------------------------------------------------------------------------
// D30j — opaque cursor pagination on the cross-envelope search route
// ---------------------------------------------------------------------------

// makeCursorChain builds n audit events ordered so the newest (highest
// SequenceNo) is the natural first row under desc order. All events
// share the same envelope/request scope.
func makeCursorChain(envID string, n int) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, 0, n)
	for i := 1; i <= n; i++ {
		ev := auditEvent(envID, i, audit.AuditEventEnvelopeCreated, map[string]any{
			"seq": i,
		})
		out = append(out, ev)
	}
	return out
}

// TestEvidence_Search_Cursor_FirstPageReturnsNextCursor pins the
// has-next probe: the handler asks the repo for wanted+1 rows, and
// when the repo returns exactly that many it trims the probe row and
// emits a non-empty next_cursor that lexically encodes the last
// surviving row.
func TestEvidence_Search_Cursor_FirstPageReturnsNextCursor(t *testing.T) {
	chain := makeCursorChain("env-cursor-page1", 3) // wanted=2 → repo returns 3
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return chain, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?limit=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Probe row must have been requested even though the user asked
	// for 2 (limit + 1 = 3 internal).
	if auditReader.lastListFilter.Limit != 3 {
		t.Errorf("filter.Limit: want 3 (wanted+1), got %d", auditReader.lastListFilter.Limit)
	}

	var resp runtimeAuditEventSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items: want 2 (probe trimmed), got %d", len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Fatal("next_cursor: want non-empty (probe row indicates a next page)")
	}
	// The cursor must decode back to the last surviving row.
	decoded, err := audit.DecodeListCursor(resp.NextCursor, true /* desc default */)
	if err != nil {
		t.Fatalf("decode next_cursor: %v", err)
	}
	if decoded.ID != resp.Items[1].ID {
		t.Errorf("next_cursor must point at last item; want id=%s, got id=%s",
			resp.Items[1].ID, decoded.ID)
	}
}

// TestEvidence_Search_Cursor_SecondPageReceivesDecodedCursor pins
// the cursor-input path: the supplied cursor token must be decoded
// and forwarded to the repo on filter.Cursor (the repo, not the
// handler, performs the predicate restriction).
func TestEvidence_Search_Cursor_SecondPageReceivesDecodedCursor(t *testing.T) {
	cursor := audit.EncodeListCursor(&audit.ListCursor{
		OccurredAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		SequenceNo: 7,
		ID:         "evt-abc",
	}, true)

	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?cursor="+cursor, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := auditReader.lastListFilter.Cursor
	if got == nil {
		t.Fatal("filter.Cursor: want decoded *audit.ListCursor; got nil")
	}
	if got.ID != "evt-abc" || got.SequenceNo != 7 {
		t.Errorf("filter.Cursor: want {ID=evt-abc, SequenceNo=7}, got %+v", got)
	}
}

// TestEvidence_Search_Cursor_NoNextCursorOnLastPage pins the
// "no next page" signal: when the repo returns ≤ wanted rows, no
// probe overflow was detected and next_cursor must be empty / absent.
func TestEvidence_Search_Cursor_NoNextCursorOnLastPage(t *testing.T) {
	chain := makeCursorChain("env-cursor-end", 2) // wanted=5 → repo returns 2
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return chain, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?limit=5", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp runtimeAuditEventSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("items: want 2, got %d", len(resp.Items))
	}
	if resp.NextCursor != "" {
		t.Errorf("next_cursor: want empty on last page, got %q", resp.NextCursor)
	}
}

// TestEvidence_Search_Cursor_OmitsNextCursorFromBody pins the JSON
// omitempty contract: when there is no next page, the next_cursor
// field must be absent from the response body (not just empty
// string) so clients can use field presence as the stop signal.
func TestEvidence_Search_Cursor_OmitsNextCursorFromBody(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return nil, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"next_cursor"`) {
		t.Errorf("body must omit next_cursor when no next page; got %s", rec.Body.String())
	}
}

// TestEvidence_Search_Cursor_InvalidCursor_Returns400 pins the
// malformed-input contract: the handler rejects a non-decodable
// cursor with 400 before touching the repository.
func TestEvidence_Search_Cursor_InvalidCursor_Returns400(t *testing.T) {
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			t.Fatal("repository must NOT be called when cursor is invalid")
			return nil, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	for _, q := range []string{
		"/v1/evidence/audit-events?cursor=!!!not-base64!!!",
		"/v1/evidence/audit-events?cursor=garbage",
	} {
		rec := performRequest(t, srv, http.MethodGet, q, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400 for malformed cursor, got %d", q, rec.Code)
		}
	}
}

// TestEvidence_Search_Cursor_OrderMismatch_Returns400 pins the
// order-binding contract: a desc-encoded cursor replayed against an
// asc query must be rejected with 400 — never silently re-interpreted.
func TestEvidence_Search_Cursor_OrderMismatch_Returns400(t *testing.T) {
	descCursor := audit.EncodeListCursor(&audit.ListCursor{
		OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SequenceNo: 1,
		ID:         "evt-mismatch",
	}, true)

	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			t.Fatal("repository must NOT be called on order mismatch")
			return nil, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?order=asc&cursor="+descCursor, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 on desc cursor vs asc query, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEvidence_Search_Cursor_AppliesAlongsideEventTypeFilter pins
// that cursor composes with the other ListFilter dimensions — the
// handler must forward both the decoded cursor AND the event_type
// filter to the repo so the cursor walk respects the same predicate.
func TestEvidence_Search_Cursor_AppliesAlongsideEventTypeFilter(t *testing.T) {
	cursor := audit.EncodeListCursor(&audit.ListCursor{
		OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SequenceNo: 3,
		ID:         "evt-mix",
	}, true)
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) { return nil, nil },
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?event_type=GOVERNANCE_CONDITION_DETECTED&cursor="+cursor, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if auditReader.lastListFilter.Cursor == nil {
		t.Error("filter.Cursor must be forwarded alongside event_type filter")
	}
	if string(auditReader.lastListFilter.EventType) != "GOVERNANCE_CONDITION_DETECTED" {
		t.Errorf("filter.EventType: want %q, got %q",
			"GOVERNANCE_CONDITION_DETECTED", auditReader.lastListFilter.EventType)
	}
}

// TestEvidence_Search_Cursor_LimitMax_DetectsNextPage pins the
// max-page-size has-next path: limit=500 with the repo returning 501
// rows must produce a 500-row page + non-empty next_cursor.
// EffectiveLimit's MaxListLimit+1 branch is the load-bearing piece —
// without it the repo would clamp 501 to 500 and the probe would not
// fire.
func TestEvidence_Search_Cursor_LimitMax_DetectsNextPage(t *testing.T) {
	chain := makeCursorChain("env-cursor-max", audit.MaxListLimit+1)
	auditReader := &stubEvidenceAuditEventReader{
		listFilterFn: func(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
			return chain, nil
		},
	}
	srv := newEvidenceSearchTestServer(t, auditReader)
	rec := performRequest(t, srv, http.MethodGet,
		"/v1/evidence/audit-events?limit=500", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp runtimeAuditEventSearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != audit.MaxListLimit {
		t.Errorf("items: want MaxListLimit (%d), got %d", audit.MaxListLimit, len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Error("next_cursor: must be non-empty when repo returned MaxListLimit+1 rows")
	}
}

func TestEvidence_Search_OpenAPI_CursorParameterAndNextCursor(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	for _, want := range []string{
		`- name: cursor`,
		`next_cursor`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("OpenAPI must declare %q for D30j cursor pagination", want)
		}
	}
}

func TestEvidence_Search_OpenAPI_NoOffsetParameters(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	// The /v1/evidence/audit-events path block is delimited by the
	// next path key under `paths:`. Slice that block out and check
	// no offset-style pagination params are declared.
	const startKey = "/v1/evidence/audit-events:"
	startIdx := strings.Index(src, startKey)
	if startIdx < 0 {
		t.Fatalf("missing %q in OpenAPI", startKey)
	}
	// End of block = next path key starting with `  /v1/`.
	tail := src[startIdx+len(startKey):]
	endIdx := strings.Index(tail, "\n  /v1/")
	var block string
	if endIdx < 0 {
		block = tail
	} else {
		block = tail[:endIdx]
	}
	for _, forbidden := range []string{
		`- name: offset`,
		`- name: page`,
		`- name: page_size`,
	} {
		if strings.Contains(block, forbidden) {
			t.Errorf("/v1/evidence/audit-events must NOT declare %q; cursor is the only pagination mechanism", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// D30d — per-envelope audit-chain integrity verification
// ---------------------------------------------------------------------------

// stubIntegrityService is a minimal evidenceReadService stub used by
// D30d tests that need to control the verify result independently of
// the envelope and audit readers. The other-method delegations return
// non-functional defaults; tests that exercise those methods continue
// to use the D30b/D30c stubs.
type stubIntegrityService struct {
	hasEvidenceFn func() bool
	getEnvFn      func(ctx context.Context, id string) (*envelope.Envelope, error)
	verifyFn      func(ctx context.Context, env *envelope.Envelope) (audit.IntegrityVerificationResult, error)
}

func (s *stubIntegrityService) HasEvidence() bool {
	if s.hasEvidenceFn != nil {
		return s.hasEvidenceFn()
	}
	return true
}
func (s *stubIntegrityService) GetEnvelope(ctx context.Context, id string) (*envelope.Envelope, error) {
	if s.getEnvFn != nil {
		return s.getEnvFn(ctx, id)
	}
	return nil, nil
}
func (s *stubIntegrityService) ListEnvelopeAuditEvents(_ context.Context, _ string) ([]*audit.AuditEvent, error) {
	return nil, nil
}
func (s *stubIntegrityService) ListAuditEvents(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
	return nil, nil
}
func (s *stubIntegrityService) VerifyEnvelopeIntegrity(ctx context.Context, env *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
	if s.verifyFn != nil {
		return s.verifyFn(ctx, env)
	}
	return audit.IntegrityVerificationResult{}, nil
}

// newIntegrityTestServer wires the supplied stub service onto a
// fresh Server with auth-mode=open so handler tests can drive every
// code path without touching the production auth chain.
func newIntegrityTestServer(t *testing.T, svc *stubIntegrityService) *Server {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)
	return srv
}

func TestEvidence_Integrity_ValidChain_Returns200ValidTrue(t *testing.T) {
	envID := "env-d30d-valid"
	svc := &stubIntegrityService{
		getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) {
			return envelopeWithID(envID), nil
		},
		verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
			return audit.IntegrityVerificationResult{
				EnvelopeID:     envID,
				Valid:          true,
				ChainLength:    7,
				FirstEventHash: "hash-first",
				FinalEventHash: "hash-final",
			}, nil
		},
	}
	srv := newIntegrityTestServer(t, svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/integrity", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp runtimeEvidenceIntegrityResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Valid {
		t.Errorf("Valid: want true, got false (%+v)", resp)
	}
	if resp.EnvelopeID != envID {
		t.Errorf("envelope_id: want %q, got %q", envID, resp.EnvelopeID)
	}
	if resp.ChainLength != 7 {
		t.Errorf("chain_length: want 7, got %d", resp.ChainLength)
	}
	if resp.FirstEventHash != "hash-first" {
		t.Errorf("first_event_hash: want %q, got %q", "hash-first", resp.FirstEventHash)
	}
	if resp.FinalEventHash != "hash-final" {
		t.Errorf("final_event_hash: want %q, got %q", "hash-final", resp.FinalEventHash)
	}
	if resp.CheckedAt.IsZero() {
		t.Error("checked_at must be populated")
	}
	if resp.ErrorKind != "" || resp.ErrorMessage != "" {
		t.Errorf("error_kind/error_message must be omitted on valid chain; got %q / %q",
			resp.ErrorKind, resp.ErrorMessage)
	}
	// Wire-level pin: error_kind / error_message omitempty must
	// actually omit on valid responses.
	body := rec.Body.String()
	if strings.Contains(body, `"error_kind"`) || strings.Contains(body, `"error_message"`) {
		t.Errorf("valid response must not include error_kind / error_message keys; body=%s", body)
	}
}

func TestEvidence_Integrity_InvalidChain_Returns200ValidFalse(t *testing.T) {
	envID := "env-d30d-invalid"
	cases := []struct {
		name     string
		kind     audit.IntegrityErrorKind
		message  string
	}{
		{"missing_events", audit.IntegrityErrorKindMissingEvents, "no audit trail"},
		{"sequence_gap", audit.IntegrityErrorKindSequenceGap, "sequence gap at sequence 3 (previous=1)"},
		{"prev_hash_mismatch", audit.IntegrityErrorKindPrevHashMismatch, "chain break at sequence 2 (prev_hash=wrong, previous_event_hash=correct)"},
		{"event_hash_mismatch", audit.IntegrityErrorKindEventHashMismatch, "hash mismatch at sequence 2 (stored=bad, computed=good)"},
		{"terminal_state_mismatch", audit.IntegrityErrorKindTerminalStateMismatch, "state mismatch (envelope=escalated, audit=closed)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &stubIntegrityService{
				getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) {
					return envelopeWithID(envID), nil
				},
				verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
					return audit.IntegrityVerificationResult{
						EnvelopeID:     envID,
						Valid:          false,
						ChainLength:    3,
						FirstEventHash: "hash-first",
						FinalEventHash: "hash-final",
						ErrorKind:      c.kind,
						ErrorMessage:   c.message,
					}, nil
				},
			}
			srv := newIntegrityTestServer(t, svc)

			rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/integrity", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("want 200 (integrity failure reports as 200), got %d: %s", rec.Code, rec.Body.String())
			}
			var resp runtimeEvidenceIntegrityResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Valid {
				t.Errorf("Valid: want false, got true")
			}
			if resp.ErrorKind != string(c.kind) {
				t.Errorf("error_kind: want %q, got %q", c.kind, resp.ErrorKind)
			}
			if resp.ErrorMessage != c.message {
				t.Errorf("error_message: want %q, got %q", c.message, resp.ErrorMessage)
			}
			if resp.ChainLength != 3 {
				t.Errorf("chain_length: want 3, got %d", resp.ChainLength)
			}
		})
	}
}

func TestEvidence_Integrity_UnknownEnvelope_Returns404(t *testing.T) {
	verifyCalled := false
	svc := &stubIntegrityService{
		getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return nil, nil },
		verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
			verifyCalled = true
			return audit.IntegrityVerificationResult{}, nil
		},
	}
	srv := newIntegrityTestServer(t, svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/env-missing/integrity", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
	if verifyCalled {
		t.Error("VerifyEnvelopeIntegrity must NOT be called when envelope is missing")
	}
}

func TestEvidence_Integrity_InvalidEnvelopeID_Returns400(t *testing.T) {
	srv := newIntegrityTestServer(t, &stubIntegrityService{})
	// Backslash is rejected by isValidIdentifier; the prefix
	// dispatcher enforces this before the integrity arm runs.
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/bad%5Cid/integrity", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestEvidence_Integrity_NonGET_Returns405(t *testing.T) {
	srv := newIntegrityTestServer(t, &stubIntegrityService{})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := performRequest(t, srv, method, "/v1/evidence/envelopes/env-x/integrity", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d", method, rec.Code)
		}
	}
}

func TestEvidence_Integrity_ServiceUnwired_Returns501(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).WithAuthMode(config.AuthModeOpen)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/env-x/integrity", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when service unwired, got %d", rec.Code)
	}

	srv2 := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(NewEvidenceReadService(nil, nil))
	rec2 := performRequest(t, srv2, http.MethodGet, "/v1/evidence/envelopes/env-x/integrity", nil)
	if rec2.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when readers nil, got %d", rec2.Code)
	}
}

func TestEvidence_Integrity_RepoError_Returns500(t *testing.T) {
	envID := "env-d30d-repoerr"
	svc := &stubIntegrityService{
		getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) {
			return envelopeWithID(envID), nil
		},
		verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
			return audit.IntegrityVerificationResult{}, errors.New("simulated postgres failure")
		},
	}
	srv := newIntegrityTestServer(t, svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/integrity", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 on repo error (verifier could not complete), got %d", rec.Code)
	}
}

// TestEvidence_Integrity_GetEnvelopeError_Returns500 pins that an
// error from the envelope reader (separate from the verifier path)
// surfaces as a non-2xx — the route never claims integrity status it
// could not compute.
func TestEvidence_Integrity_GetEnvelopeError_Returns500(t *testing.T) {
	svc := &stubIntegrityService{
		getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) {
			return nil, errors.New("simulated envelope read failure")
		},
	}
	srv := newIntegrityTestServer(t, svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/env-x/integrity", nil)
	if rec.Code < 500 {
		t.Errorf("want >=500 on envelope reader error, got %d", rec.Code)
	}
}

func TestEvidence_Integrity_DoesNotExposeAuditPayloadOrSubmittedRaw(t *testing.T) {
	envID := "env-d30d-noleaks"
	svc := &stubIntegrityService{
		getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) {
			env := envelopeWithID(envID)
			env.Submitted = envelope.Submitted{Raw: json.RawMessage(`{"sentinel":"MUST_NOT_LEAK"}`)}
			return env, nil
		},
		verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
			return audit.IntegrityVerificationResult{
				EnvelopeID:     envID,
				Valid:          true,
				ChainLength:    2,
				FirstEventHash: "abc",
				FinalEventHash: "def",
			}, nil
		},
	}
	srv := newIntegrityTestServer(t, svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/integrity", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// The integrity endpoint must report status only — never audit
	// payloads, audit-event IDs, the envelope's submitted JSON, or
	// the envelope's submitted hash.
	for _, forbidden := range []string{
		`"submitted"`, `"raw"`, `submitted_hash`, `MUST_NOT_LEAK`,
		`"payload"`, `"items"`, `"audit_events"`, `"audit_event_ids"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("integrity response must NOT contain %q; body=%s", forbidden, body)
		}
	}
}

func TestEvidence_Integrity_OpenAPI_PathAndSchemaDeclared(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	for _, want := range []string{
		`/v1/evidence/envelopes/{id}/integrity:`,
		`operationId: getEvidenceEnvelopeIntegrity`,
		`RuntimeEvidenceIntegrityResponse:`,
		`$ref: "#/components/schemas/RuntimeEvidenceIntegrityResponse"`,
		// All required wire fields must be documented.
		`envelope_id`,
		`valid:`,
		`chain_length`,
		`first_event_hash`,
		`final_event_hash`,
		`checked_at`,
		// Optional error fields must be documented (but not required).
		`error_kind`,
		`error_message`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("OpenAPI missing %q", want)
		}
	}
}

// TestEvidence_Integrity_OpenAPI_NoPacketPath was the D30d source
// pin that forbade /v1/evidence/envelopes/{id}/packet in the spec.
// D30e shipped that route, so the forbidden list is now empty and
// the test was removed. The packet's own OpenAPI source pin
// (TestEvidence_Packet_OpenAPI_PathAndSchemaDeclared below)
// asserts the path / schema / $refs are present.

func TestEvidence_Integrity_RouteDispatched(t *testing.T) {
	// The integrity sub-path is handled by the existing
	// /v1/evidence/envelopes/ prefix dispatcher. Pin both the
	// dispatcher arm and the handler symbol so a future edit cannot
	// silently drop or rename the route.
	body, err := os.ReadFile("evidence_handler.go")
	if err != nil {
		t.Fatalf("read evidence_handler.go: %v", err)
	}
	src := string(body)
	for _, want := range []string{
		`parts[1] == "integrity"`,
		`s.handleGetEvidenceEnvelopeIntegrity(`,
		`func (s *Server) handleGetEvidenceEnvelopeIntegrity(`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("evidence_handler.go must contain %q", want)
		}
	}
}

// TestEvidence_Integrity_PriorRoutesStillExist pins that D30d did
// not replace D30b/D30c or the Explorer audit-events route.
func TestEvidence_Integrity_PriorRoutesStillExist(t *testing.T) {
	server, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	for _, want := range []string{
		`"/v1/evidence/envelopes/"`, // D30b prefix
		`"/v1/evidence/audit-events"`, // D30c search route
	} {
		if !strings.Contains(string(server), want) {
			t.Errorf("server.go must still register %q (D30d is additive)", want)
		}
	}
	auth, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatalf("read auth.go: %v", err)
	}
	if !strings.Contains(string(auth), `"GET /explorer/envelopes/"`) {
		t.Error("auth.go must still register GET /explorer/envelopes/ (Explorer route unchanged)")
	}
}

// ---------------------------------------------------------------------------
// D30e — evidence packet export
// ---------------------------------------------------------------------------

// newPacketTestServer wires the supplied stub service onto a Server
// with auth-mode=open so packet tests can exercise every code path
// without touching the production auth chain.
func newPacketTestServer(t *testing.T, svc *stubIntegrityService) *Server {
	t.Helper()
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)
}

// packetEvent is a minimal audit event the packet handler will sort
// and serialise. Reuses the test-package auditEvent helper to keep
// fixtures consistent with D30b/D30c/D30d.
func packetEvent(envID string, seq int, t audit.AuditEventType, payload map[string]any) *audit.AuditEvent {
	return auditEvent(envID, seq, t, payload)
}

func TestEvidence_Packet_HappyPath_ReturnsEnvelopeChainAndIntegrity(t *testing.T) {
	envID := "env-d30e-happy"
	env := envelopeWithID(envID)
	chain := []*audit.AuditEvent{
		packetEvent(envID, 1, audit.AuditEventEnvelopeCreated, map[string]any{"request_source": "test-source"}),
		packetEvent(envID, 2, audit.AuditEventEnvelopeClosed, map[string]any{
			"from_state": "outcome_recorded",
			"to_state":   "closed",
		}),
	}
	svc := &stubIntegrityService{
		getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return env, nil },
		verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
			return audit.IntegrityVerificationResult{
				EnvelopeID:     envID,
				Valid:          true,
				ChainLength:    2,
				FirstEventHash: "hash-first",
				FinalEventHash: "hash-final",
			}, nil
		},
	}
	// stubIntegrityService.ListEnvelopeAuditEvents returns (nil, nil) by
	// default; override with a one-shot stub via WithEvidenceReadService
	// indirection. Simpler: embed an audit-list capture in a thin wrapper.
	svc2 := &packetStubService{base: svc, events: chain}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc2)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp runtimeEvidencePacketResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EnvelopeID != envID {
		t.Errorf("envelope_id: want %q, got %q", envID, resp.EnvelopeID)
	}
	if resp.GeneratedAt.IsZero() {
		t.Error("generated_at must be populated")
	}
	if resp.Envelope == nil || resp.Envelope.ID() != envID {
		t.Errorf("envelope must be present with matching id; got %+v", resp.Envelope)
	}
	if len(resp.AuditEvents) != 2 {
		t.Errorf("audit_events: want 2, got %d", len(resp.AuditEvents))
	}
	if !resp.Integrity.Valid {
		t.Errorf("integrity.valid: want true, got false (%+v)", resp.Integrity)
	}
	if resp.Integrity.EnvelopeID != envID {
		t.Errorf("integrity.envelope_id: want %q, got %q", envID, resp.Integrity.EnvelopeID)
	}
}

// packetStubService extends stubIntegrityService with a fixed
// audit-event slice returned by both ListEnvelopeAuditEvents (handler
// path) and used by the verifier stub indirectly. Keeps tests terse.
type packetStubService struct {
	base   *stubIntegrityService
	events []*audit.AuditEvent
	auditErr error
}

func (s *packetStubService) HasEvidence() bool { return s.base.HasEvidence() }
func (s *packetStubService) GetEnvelope(ctx context.Context, id string) (*envelope.Envelope, error) {
	return s.base.GetEnvelope(ctx, id)
}
func (s *packetStubService) ListEnvelopeAuditEvents(_ context.Context, _ string) ([]*audit.AuditEvent, error) {
	if s.auditErr != nil {
		return nil, s.auditErr
	}
	return s.events, nil
}
func (s *packetStubService) ListAuditEvents(_ context.Context, _ audit.ListFilter) ([]*audit.AuditEvent, error) {
	return nil, nil
}
func (s *packetStubService) VerifyEnvelopeIntegrity(ctx context.Context, env *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
	return s.base.VerifyEnvelopeIntegrity(ctx, env)
}

func TestEvidence_Packet_AuditEventsOrdered_BySequenceNoAsc(t *testing.T) {
	envID := "env-d30e-order"
	env := envelopeWithID(envID)
	// Deliberately delivered out of order.
	chain := []*audit.AuditEvent{
		packetEvent(envID, 3, audit.AuditEventEnvelopeClosed, map[string]any{}),
		packetEvent(envID, 1, audit.AuditEventEnvelopeCreated, map[string]any{}),
		packetEvent(envID, 2, audit.AuditEventOutcomeRecorded, map[string]any{}),
	}
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return env, nil },
			verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
				return audit.IntegrityVerificationResult{EnvelopeID: envID, Valid: true, ChainLength: 3}, nil
			},
		},
		events: chain,
	}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp runtimeEvidencePacketResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.AuditEvents) != 3 {
		t.Fatalf("audit_events: want 3, got %d", len(resp.AuditEvents))
	}
	for i, ev := range resp.AuditEvents {
		if ev.SequenceNo != i+1 {
			t.Errorf("audit_events[%d].sequence_no: want %d, got %d", i, i+1, ev.SequenceNo)
		}
	}
}

func TestEvidence_Packet_GeneratedAt_IsRFC3339UTC(t *testing.T) {
	envID := "env-d30e-time"
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return envelopeWithID(envID), nil },
			verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
				return audit.IntegrityVerificationResult{EnvelopeID: envID, Valid: true}, nil
			},
		},
	}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)

	before := time.Now().UTC()
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	after := time.Now().UTC()
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	// Decode as a generic map to assert the wire-level string shape.
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	gen, ok := raw["generated_at"].(string)
	if !ok || gen == "" {
		t.Fatalf("generated_at must be a non-empty string; got %v", raw["generated_at"])
	}
	parsed, err := time.Parse(time.RFC3339Nano, gen)
	if err != nil {
		t.Fatalf("generated_at must be RFC3339Nano; got %q (%v)", gen, err)
	}
	if parsed.Before(before.Add(-time.Second)) || parsed.After(after.Add(time.Second)) {
		t.Errorf("generated_at must be within the request window; got %v (window %v..%v)",
			parsed, before, after)
	}
	if parsed.Location() != time.UTC {
		t.Errorf("generated_at must be UTC; got location %v", parsed.Location())
	}
}

func TestEvidence_Packet_EnvelopeShape_MatchesV1EnvelopesRoute(t *testing.T) {
	envID := "env-d30e-shape"
	env := envelopeWithID(envID)
	// Populate Submitted.Raw so the comparison covers the full
	// envelope serialisation, not just identity fields.
	env.Submitted = envelope.Submitted{Raw: json.RawMessage(`{"surface_id":"surf-x","agent_id":"agent-x"}`)}

	// Stand up a Server with a stub orchestrator that returns this
	// exact envelope from GetEnvelopeByID, so /v1/envelopes/{id}
	// emits the byte-identical serialisation.
	orch := &mockOrchestrator{
		getEnvelopeByIDFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return env, nil },
	}
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return env, nil },
			verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
				return audit.IntegrityVerificationResult{EnvelopeID: envID, Valid: true}, nil
			},
		},
	}
	srv := NewServerFull(orch, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)

	// /v1/envelopes/{id} body.
	v1Rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes/"+envID, nil)
	if v1Rec.Code != http.StatusOK {
		t.Fatalf("/v1/envelopes/%s: want 200, got %d", envID, v1Rec.Code)
	}
	var v1Body map[string]any
	if err := json.Unmarshal(v1Rec.Body.Bytes(), &v1Body); err != nil {
		t.Fatalf("decode v1 envelope: %v", err)
	}

	// Packet response.
	pktRec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	if pktRec.Code != http.StatusOK {
		t.Fatalf("packet: want 200, got %d", pktRec.Code)
	}
	var pktBody map[string]any
	if err := json.Unmarshal(pktRec.Body.Bytes(), &pktBody); err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	pktEnvelope, ok := pktBody["envelope"].(map[string]any)
	if !ok {
		t.Fatalf("packet.envelope must be an object; got %v", pktBody["envelope"])
	}

	// Compare shape via re-marshalled JSON. Bytes-equal means the
	// packet's envelope sub-object is byte-identical to the
	// /v1/envelopes/{id} body.
	v1Bytes, _ := json.Marshal(v1Body)
	pktBytes, _ := json.Marshal(pktEnvelope)
	if string(v1Bytes) != string(pktBytes) {
		t.Errorf("packet.envelope must match /v1/envelopes/{id} shape\nv1=%s\npacket.envelope=%s",
			v1Bytes, pktBytes)
	}
}

func TestEvidence_Packet_InvalidIntegrity_ReturnsHTTP200WithIntegrityValidFalse(t *testing.T) {
	envID := "env-d30e-invalid"
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return envelopeWithID(envID), nil },
			verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
				return audit.IntegrityVerificationResult{
					EnvelopeID:   envID,
					Valid:        false,
					ChainLength:  2,
					ErrorKind:    audit.IntegrityErrorKindEventHashMismatch,
					ErrorMessage: "hash mismatch at sequence 2 (stored=bad, computed=good)",
				}, nil
			},
		},
	}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (invalid integrity reports as 200), got %d", rec.Code)
	}
	var resp runtimeEvidencePacketResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Integrity.Valid {
		t.Errorf("integrity.valid: want false, got true")
	}
	if resp.Integrity.ErrorKind != string(audit.IntegrityErrorKindEventHashMismatch) {
		t.Errorf("integrity.error_kind: want %q, got %q",
			audit.IntegrityErrorKindEventHashMismatch, resp.Integrity.ErrorKind)
	}
	if !strings.Contains(resp.Integrity.ErrorMessage, "hash mismatch") {
		t.Errorf("integrity.error_message: want substring 'hash mismatch', got %q",
			resp.Integrity.ErrorMessage)
	}
}

func TestEvidence_Packet_UnknownEnvelope_Returns404(t *testing.T) {
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return nil, nil },
		},
	}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/env-missing/packet", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestEvidence_Packet_InvalidEnvelopeID_Returns400(t *testing.T) {
	srv := newPacketTestServer(t, &stubIntegrityService{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/bad%5Cid/packet", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestEvidence_Packet_NonGET_Returns405(t *testing.T) {
	srv := newPacketTestServer(t, &stubIntegrityService{})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := performRequest(t, srv, method, "/v1/evidence/envelopes/env-x/packet", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d", method, rec.Code)
		}
	}
}

func TestEvidence_Packet_ServiceUnwired_Returns501(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).WithAuthMode(config.AuthModeOpen)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/env-x/packet", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when service unwired, got %d", rec.Code)
	}

	srv2 := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(NewEvidenceReadService(nil, nil))
	rec2 := performRequest(t, srv2, http.MethodGet, "/v1/evidence/envelopes/env-x/packet", nil)
	if rec2.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when readers nil, got %d", rec2.Code)
	}
}

func TestEvidence_Packet_AuditRepoError_Returns500(t *testing.T) {
	envID := "env-d30e-auditerr"
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return envelopeWithID(envID), nil },
			verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
				t.Fatal("verifier must NOT be called once the audit lookup has failed")
				return audit.IntegrityVerificationResult{}, nil
			},
		},
		auditErr: errors.New("simulated postgres failure"),
	}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 on audit-list failure, got %d", rec.Code)
	}
	// Partial packet must not be returned.
	if strings.Contains(rec.Body.String(), `"envelope"`) || strings.Contains(rec.Body.String(), `"audit_events"`) {
		t.Errorf("partial packet returned on failure path; body=%s", rec.Body.String())
	}
}

func TestEvidence_Packet_IntegrityRepoError_Returns500(t *testing.T) {
	envID := "env-d30e-intgerr"
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return envelopeWithID(envID), nil },
			verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
				return audit.IntegrityVerificationResult{}, errors.New("simulated verifier failure")
			},
		},
	}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 on verifier failure, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"envelope"`) || strings.Contains(rec.Body.String(), `"audit_events"`) {
		t.Errorf("partial packet returned on failure path; body=%s", rec.Body.String())
	}
}

func TestEvidence_Packet_FailModePolicyPayloadRoundTrips(t *testing.T) {
	envID := "env-d30e-fmp"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	enforcedPayload := map[string]any{
		"fail_mode_policy_id":      "fmp-demo",
		"fail_mode_policy_version": float64(1),
		"source":                   "surface",
		"trigger_condition":        "policy_evaluator_error",
		"correctness_class":        "resource",
		"permitted_mode":           "closed",
		"enforcement_state":        "enforced",
		"configured_outcome":       "deny",
		"enforced_outcome":         "reject",
		"enforced_reason_code":     "FAIL_MODE_POLICY_DENIED",
		"previous_outcome":         "escalate",
		"previous_reason_code":     "POLICY_ERROR",
		"applied_at":               now,
		"evaluation_time":          now,
	}
	chain := []*audit.AuditEvent{
		packetEvent(envID, 1, audit.AuditEventEnvelopeCreated, map[string]any{"request_source": "test-source"}),
		packetEvent(envID, 7, audit.AuditEventFailModePolicyEnforced, enforcedPayload),
		packetEvent(envID, 8, audit.AuditEventOutcomeRecorded, map[string]any{"outcome": "reject"}),
	}
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return envelopeWithID(envID), nil },
			verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
				return audit.IntegrityVerificationResult{EnvelopeID: envID, Valid: true, ChainLength: 3}, nil
			},
		},
		events: chain,
	}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp runtimeEvidencePacketResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found *runtimeAuditEvent
	for i := range resp.AuditEvents {
		if resp.AuditEvents[i].EventType == string(audit.AuditEventFailModePolicyEnforced) {
			found = &resp.AuditEvents[i]
			break
		}
	}
	if found == nil {
		t.Fatal("FAIL_MODE_POLICY_ENFORCED missing from packet audit_events")
	}
	for k, want := range enforcedPayload {
		got, present := found.Payload[k]
		if !present {
			t.Errorf("packet audit_events[*].payload missing key %q", k)
			continue
		}
		if got != want {
			t.Errorf("payload[%q]: want %v, got %v", k, want, got)
		}
	}
}

// TestEvidence_Packet_SubmittedRawAppearsOnlyInsideEnvelope pins the
// brief's strongest privacy constraint for D30e: if the envelope
// representation includes Submitted.Raw, the packet includes it ONLY
// inside the embedded envelope object (preserving the
// /v1/envelopes/{id} contract). The packet wrapper has no top-level
// submitted / raw / submitted_hash field, and no audit-event payload
// re-exposes the raw.
func TestEvidence_Packet_SubmittedRawAppearsOnlyInsideEnvelope(t *testing.T) {
	envID := "env-d30e-rawscope"
	const sentinel = "MUST_NOT_LEAK_AT_TOP_LEVEL"
	env := envelopeWithID(envID)
	env.Submitted = envelope.Submitted{Raw: json.RawMessage(`{"surface_id":"surf-x","note":"` + sentinel + `"}`)}

	chain := []*audit.AuditEvent{
		packetEvent(envID, 1, audit.AuditEventEnvelopeCreated, map[string]any{
			// Crucially the audit-event payload must NOT contain the
			// sentinel — only the envelope's Submitted.Raw does.
			"request_source": "test-source",
		}),
	}
	svc := &packetStubService{
		base: &stubIntegrityService{
			getEnvFn: func(_ context.Context, _ string) (*envelope.Envelope, error) { return env, nil },
			verifyFn: func(_ context.Context, _ *envelope.Envelope) (audit.IntegrityVerificationResult, error) {
				return audit.IntegrityVerificationResult{EnvelopeID: envID, Valid: true, ChainLength: 1}, nil
			},
		},
		events: chain,
	}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithEvidenceReadService(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/evidence/envelopes/"+envID+"/packet", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, sentinel) {
		t.Fatal("expected envelope.submitted.raw to contain the sentinel; body did not include it")
	}
	if strings.Count(body, sentinel) != 1 {
		t.Errorf("sentinel must appear exactly once (inside envelope.submitted.raw); count=%d body=%s",
			strings.Count(body, sentinel), body)
	}
	// Wire-level shape: the packet wrapper itself has no submitted
	// / raw / submitted_hash keys at the top level.
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, forbidden := range []string{"submitted", "raw", "submitted_hash"} {
		if _, present := raw[forbidden]; present {
			t.Errorf("packet wrapper must not have top-level %q; got %v", forbidden, raw[forbidden])
		}
	}
	// Sentinel must live under packet.envelope.submitted.raw — walk
	// to confirm.
	pktEnv, ok := raw["envelope"].(map[string]any)
	if !ok {
		t.Fatalf("packet.envelope must be an object")
	}
	submitted, ok := pktEnv["submitted"].(map[string]any)
	if !ok {
		t.Fatalf("packet.envelope.submitted must be an object")
	}
	rawObj, ok := submitted["raw"].(map[string]any)
	if !ok {
		t.Fatalf("packet.envelope.submitted.raw must be a JSON object (RawMessage round-trip); got %T", submitted["raw"])
	}
	if note, _ := rawObj["note"].(string); note != sentinel {
		t.Errorf("envelope.submitted.raw.note: want sentinel %q, got %v", sentinel, rawObj["note"])
	}
}

func TestEvidence_Packet_OpenAPI_PathAndSchemaDeclared(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	for _, want := range []string{
		`/v1/evidence/envelopes/{id}/packet:`,
		`operationId: getEvidenceEnvelopePacket`,
		`RuntimeEvidencePacket:`,
		`$ref: "#/components/schemas/RuntimeEvidencePacket"`,
		// Schema must reference each composed shape rather than
		// inlining.
		`$ref: "#/components/schemas/Envelope"`,
		// RuntimeAuditEvent and RuntimeEvidenceIntegrityResponse are
		// already referenced elsewhere; the assertion here is that
		// the packet schema references each.
	} {
		if !strings.Contains(src, want) {
			t.Errorf("OpenAPI missing %q", want)
		}
	}
	// Locate the RuntimeEvidencePacket schema block and assert it
	// references RuntimeAuditEvent and RuntimeEvidenceIntegrityResponse.
	idx := strings.Index(src, "RuntimeEvidencePacket:")
	if idx < 0 {
		t.Fatal("RuntimeEvidencePacket schema not found")
	}
	// Slice until the next top-level component (any "    XYZ:" two
	// spaces deeper, indicating another schema). Conservative window.
	end := idx + 3000
	if end > len(src) {
		end = len(src)
	}
	slice := src[idx:end]
	for _, want := range []string{
		`$ref: "#/components/schemas/RuntimeAuditEvent"`,
		`$ref: "#/components/schemas/RuntimeEvidenceIntegrityResponse"`,
	} {
		if !strings.Contains(slice, want) {
			t.Errorf("RuntimeEvidencePacket schema must reference %q (slice=%s)", want, slice)
		}
	}
}

// TestEvidence_Packet_PriorRoutesStillExist confirms D30e is additive
// and did not alter any prior evidence or Explorer route.
func TestEvidence_Packet_PriorRoutesStillExist(t *testing.T) {
	server, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	for _, want := range []string{
		`"/v1/evidence/envelopes/"`,   // D30b prefix; backs D30b/D30d/D30e sub-paths
		`"/v1/evidence/audit-events"`, // D30c search route
	} {
		if !strings.Contains(string(server), want) {
			t.Errorf("server.go must still register %q (D30e is additive)", want)
		}
	}
	handler, err := os.ReadFile("evidence_handler.go")
	if err != nil {
		t.Fatalf("read evidence_handler.go: %v", err)
	}
	for _, want := range []string{
		`handleGetEvidenceEnvelopeAuditEvents`, // D30b
		`handleSearchEvidenceAuditEvents`,      // D30c
		`handleGetEvidenceEnvelopeIntegrity`,   // D30d
		`handleGetEvidencePacket`,              // D30e (new)
	} {
		if !strings.Contains(string(handler), want) {
			t.Errorf("evidence_handler.go must define %q", want)
		}
	}
	auth, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatalf("read auth.go: %v", err)
	}
	if !strings.Contains(string(auth), `"GET /explorer/envelopes/"`) {
		t.Error("auth.go must still register GET /explorer/envelopes/ (Explorer route unchanged)")
	}
}
