package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/approval"
	cpTypes "github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Mock Orchestrator
// ---------------------------------------------------------------------------

type mockOrchestrator struct {
	evaluateFn                  func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error)
	resolveEscalationFn         func(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error)
	getEnvelopeByIDFn           func(ctx context.Context, id string) (*envelope.Envelope, error)
	getEnvelopeByRequestScopeFn func(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error)
	listEnvelopesFn             func(ctx context.Context) ([]*envelope.Envelope, error)
	listEnvelopesByStateFn      func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error)
}

func (m *mockOrchestrator) Evaluate(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
	if m.evaluateFn != nil {
		return m.evaluateFn(ctx, req, raw)
	}
	return decision.EvaluationResult{}, fmt.Errorf("evaluate not implemented")
}

func (m *mockOrchestrator) ResolveEscalation(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error) {
	if m.resolveEscalationFn != nil {
		return m.resolveEscalationFn(ctx, resolution)
	}
	return nil, fmt.Errorf("resolveEscalation not implemented")
}

func (m *mockOrchestrator) GetEnvelopeByID(ctx context.Context, id string) (*envelope.Envelope, error) {
	if m.getEnvelopeByIDFn != nil {
		return m.getEnvelopeByIDFn(ctx, id)
	}
	return nil, fmt.Errorf("getEnvelopeByID not implemented")
}

func (m *mockOrchestrator) GetEnvelopeByRequestScope(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
	if m.getEnvelopeByRequestScopeFn != nil {
		return m.getEnvelopeByRequestScopeFn(ctx, requestSource, requestID)
	}
	return nil, fmt.Errorf("getEnvelopeByRequestScope not implemented")
}

func (m *mockOrchestrator) ListEnvelopes(ctx context.Context) ([]*envelope.Envelope, error) {
	if m.listEnvelopesFn != nil {
		return m.listEnvelopesFn(ctx)
	}
	return nil, fmt.Errorf("listEnvelopes not implemented")
}

func (m *mockOrchestrator) ListEnvelopesByState(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
	if m.listEnvelopesByStateFn != nil {
		return m.listEnvelopesByStateFn(ctx, state)
	}
	return nil, fmt.Errorf("listEnvelopesByState not implemented")
}

func (m *mockOrchestrator) Simulate(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
	return decision.EvaluationResult{}, nil
}

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

func performRequest(t *testing.T, srv *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	return result
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	return decodeJSON[map[string]string](t, rec)
}

func marshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return data
}

// ---------------------------------------------------------------------------
// Health/Readiness Tests
// ---------------------------------------------------------------------------

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/healthz", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
	if resp["service"] != "midas" {
		t.Errorf("expected service 'midas', got %q", resp["service"])
	}
}

func TestHealthEndpoint_MethodNotAllowed(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/healthz", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("expected Allow header 'GET', got %q", allow)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "method not allowed" {
		t.Errorf("expected error 'method not allowed', got %q", errResp["error"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/readyz", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if resp["status"] != "ready" {
		t.Errorf("expected status 'ready', got %q", resp["status"])
	}
}

func TestReadyEndpoint_MethodNotAllowed(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/readyz", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("expected Allow header 'GET', got %q", allow)
	}
}

// ---------------------------------------------------------------------------
// Evaluate Endpoint Tests
// ---------------------------------------------------------------------------

func TestEvaluate_Success(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			if req.SurfaceID != "surf-1" {
				t.Errorf("expected surface_id 'surf-1', got %q", req.SurfaceID)
			}
			if req.AgentID != "agent-1" {
				t.Errorf("expected agent_id 'agent-1', got %q", req.AgentID)
			}
			if req.Confidence != 0.95 {
				t.Errorf("expected confidence 0.95, got %f", req.Confidence)
			}

			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
				EnvelopeID: "env-123",
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.95,
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[evaluateResponse](t, rec)
	if resp.Outcome != "accept" {
		t.Errorf("expected outcome 'accept', got %q", resp.Outcome)
	}
	if resp.Reason != "WITHIN_AUTHORITY" {
		t.Errorf("expected reason 'WITHIN_AUTHORITY', got %q", resp.Reason)
	}
	if resp.EnvelopeID != "env-123" {
		t.Errorf("expected envelope_id 'env-123', got %q", resp.EnvelopeID)
	}
}

func TestEvaluate_RawBodyPassedVerbatim(t *testing.T) {
	expectedBody := `{"surface_id":"surf-1","agent_id":"agent-1","confidence":0.95}`
	var capturedRaw json.RawMessage

	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			capturedRaw = raw
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
			}, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", []byte(expectedBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if string(capturedRaw) != expectedBody {
		t.Errorf("raw body not preserved:\nexpected: %s\ngot: %s", expectedBody, string(capturedRaw))
	}
}

func TestEvaluate_MissingRequiredFields(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})

	tests := []struct {
		name    string
		payload map[string]any
		errMsg  string
	}{
		{
			name:    "missing surface_id",
			payload: map[string]any{"agent_id": "agent-1", "confidence": 0.9},
			errMsg:  "surface_id and agent_id are required",
		},
		{
			name:    "missing agent_id",
			payload: map[string]any{"surface_id": "surf-1", "confidence": 0.9},
			errMsg:  "surface_id and agent_id are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := marshalJSON(t, tt.payload)
			rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != tt.errMsg {
				t.Errorf("expected error %q, got %q", tt.errMsg, errResp["error"])
			}
		})
	}
}

func TestEvaluate_InvalidConfidence(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})

	tests := []struct {
		name       string
		confidence float64
	}{
		{"below zero", -0.1},
		{"above one", 1.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := marshalJSON(t, map[string]any{
				"surface_id": "surf-1",
				"agent_id":   "agent-1",
				"confidence": tt.confidence,
			})

			rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != "confidence must be between 0 and 1" {
				t.Errorf("unexpected error: %q", errResp["error"])
			}
		})
	}
}

func TestEvaluate_ZeroConfidenceIsValid(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			if req.Confidence != 0.0 {
				t.Errorf("expected confidence 0.0, got %f", req.Confidence)
			}
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.0,
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 (zero confidence is valid), got %d", rec.Code)
	}
}

func TestEvaluate_InvalidJSON(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", []byte("not-json"))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "invalid JSON payload" {
		t.Errorf("unexpected error: %q", errResp["error"])
	}
}

func TestEvaluate_UnknownFieldsRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})

	body := []byte(`{
		"surface_id": "surf-1",
		"agent_id": "agent-1",
		"confidence": 0.9,
		"unknown_field": "should-be-rejected"
	}`)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for unknown fields, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "invalid JSON payload" {
		t.Errorf("expected 'invalid JSON payload' error, got %q", errResp["error"])
	}
}

func TestEvaluate_TrailingGarbageRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	body := []byte(`{"surface_id":"surf-1","agent_id":"agent-1","confidence":0.9}garbage`)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for trailing garbage, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "invalid JSON payload" {
		t.Errorf("expected 'invalid JSON payload' error, got %q", errResp["error"])
	}
}

func TestEvaluate_EmptyBodyRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", []byte(""))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body must not be empty" {
		t.Errorf("expected 'request body must not be empty' error, got %q", errResp["error"])
	}
}

func TestEvaluate_WhitespaceOnlyBodyRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", []byte("   \n\t  "))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for whitespace-only body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body must not be empty" {
		t.Errorf("expected 'request body must not be empty' error, got %q", errResp["error"])
	}
}

func TestEvaluate_OversizedBodyRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	largeBody := bytes.Repeat([]byte("x"), (1<<20)+1)

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", largeBody)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 for oversized body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body too large" {
		t.Errorf("expected 'request body too large' error, got %q", errResp["error"])
	}
}

func TestEvaluate_WhitespaceTrimming(t *testing.T) {
	var capturedReq eval.DecisionRequest

	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			capturedReq = req
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id":     "  surf-1  ",
		"agent_id":       "\tagent-1\n",
		"confidence":     0.9,
		"request_id":     "  req-123  ",
		"request_source": "  custom-source  ",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if capturedReq.SurfaceID != "surf-1" {
		t.Errorf("expected trimmed surface_id 'surf-1', got %q", capturedReq.SurfaceID)
	}
	if capturedReq.AgentID != "agent-1" {
		t.Errorf("expected trimmed agent_id 'agent-1', got %q", capturedReq.AgentID)
	}
	if capturedReq.RequestID != "req-123" {
		t.Errorf("expected trimmed request_id 'req-123', got %q", capturedReq.RequestID)
	}
	if capturedReq.RequestSource != "custom-source" {
		t.Errorf("expected trimmed request_source 'custom-source', got %q", capturedReq.RequestSource)
	}
}

func TestEvaluate_DefaultRequestSource(t *testing.T) {
	var capturedReq eval.DecisionRequest

	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			capturedReq = req
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if capturedReq.RequestSource != "api" {
		t.Errorf("expected default request_source 'api', got %q", capturedReq.RequestSource)
	}
}

func TestEvaluate_WhitespaceOnlyRequestSourceDefaultsToAPI(t *testing.T) {
	var capturedReq eval.DecisionRequest

	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			capturedReq = req
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id":     "surf-1",
		"agent_id":       "agent-1",
		"confidence":     0.9,
		"request_source": "   \t  ",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if capturedReq.RequestSource != "api" {
		t.Errorf("expected whitespace-only request_source to default to 'api', got %q", capturedReq.RequestSource)
	}
}

func TestEvaluate_MethodNotAllowed(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/evaluate", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("expected Allow header 'POST', got %q", allow)
	}
}

func TestEvaluate_OrchestratorError(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{}, fmt.Errorf("orchestrator failed")
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "orchestrator failed" {
		t.Errorf("expected error 'orchestrator failed', got %q", errResp["error"])
	}
}

func TestEvaluate_NilOrchestrator(t *testing.T) {
	srv := NewServer(nil)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for nil orchestrator, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "orchestrator not configured" {
		t.Errorf("expected 'orchestrator not configured' error, got %q", errResp["error"])
	}
}

func TestEvaluate_GeneratesRequestIDWhenMissing(t *testing.T) {
	var capturedRequestID string

	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			capturedRequestID = req.RequestID
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
				EnvelopeID: "env-123",
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if capturedRequestID == "" {
		t.Error("expected request_id to be generated, got empty string")
	}
}

func TestEvaluate_PreservesProvidedRequestID(t *testing.T) {
	var capturedRequestID string

	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			capturedRequestID = req.RequestID
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
				EnvelopeID: "env-123",
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
		"request_id": "client-req-123",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if capturedRequestID != "client-req-123" {
		t.Errorf("expected request_id 'client-req-123', got %q", capturedRequestID)
	}
}

func TestEvaluate_InvalidRequestID(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})

	tests := []struct {
		name      string
		requestID string
	}{
		{"backslash", "req\\123"},
		{"null byte", "req\x00123"},
		{"control character", "req/123"},
		{"too long", strings.Repeat("x", 256)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := marshalJSON(t, map[string]any{
				"surface_id": "surf-1",
				"agent_id":   "agent-1",
				"confidence": 0.9,
				"request_id": tt.requestID,
			})

			rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400 for invalid request_id, got %d", rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != "request_id contains invalid characters or exceeds length limit" {
				t.Errorf("unexpected error: %q", errResp["error"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Review Endpoint Tests
// ---------------------------------------------------------------------------

func TestReview_Success(t *testing.T) {
	mock := &mockOrchestrator{
		resolveEscalationFn: func(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error) {
			if resolution.EnvelopeID != "env-123" {
				t.Errorf("expected envelope_id 'env-123', got %q", resolution.EnvelopeID)
			}
			if resolution.Decision != envelope.ReviewDecisionApproved {
				t.Errorf("expected decision 'approved', got %v", resolution.Decision)
			}
			if resolution.ReviewerID != "reviewer-1" {
				t.Errorf("expected reviewer 'reviewer-1', got %q", resolution.ReviewerID)
			}
			if resolution.ReviewerKind != "human" {
				t.Errorf("expected reviewer_kind 'human', got %q", resolution.ReviewerKind)
			}

			return &envelope.Envelope{
				Identity: envelope.Identity{
					ID: "env-123",
				},
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"envelope_id": "env-123",
		"decision":    "accept",
		"reviewer":    "reviewer-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[reviewResponse](t, rec)
	if resp.EnvelopeID != "env-123" {
		t.Errorf("expected envelope_id 'env-123', got %q", resp.EnvelopeID)
	}
	if resp.Status != "resolved" {
		t.Errorf("expected status 'resolved', got %q", resp.Status)
	}
}

func TestReview_NilEnvelopeReturned(t *testing.T) {
	mock := &mockOrchestrator{
		resolveEscalationFn: func(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error) {
			return nil, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"envelope_id": "env-123",
		"decision":    "accept",
		"reviewer":    "reviewer-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", payload)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 even with nil envelope, got %d", rec.Code)
	}

	resp := decodeJSON[reviewResponse](t, rec)
	if resp.EnvelopeID != "env-123" {
		t.Errorf("expected envelope_id 'env-123', got %q", resp.EnvelopeID)
	}
	if resp.Status != "resolved" {
		t.Errorf("expected status 'resolved', got %q", resp.Status)
	}
}

func TestReview_LegacyDecisionVocabulary(t *testing.T) {
	tests := []struct {
		input    string
		expected envelope.ReviewDecision
	}{
		{"accept", envelope.ReviewDecisionApproved},
		{"approve", envelope.ReviewDecisionApproved},
		{"approved", envelope.ReviewDecisionApproved},
		{"reject", envelope.ReviewDecisionRejected},
		{"deny", envelope.ReviewDecisionRejected},
		{"denied", envelope.ReviewDecisionRejected},
		{"ACCEPT", envelope.ReviewDecisionApproved},
		{"REJECT", envelope.ReviewDecisionRejected},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var capturedDecision envelope.ReviewDecision

			mock := &mockOrchestrator{
				resolveEscalationFn: func(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error) {
					capturedDecision = resolution.Decision
					return &envelope.Envelope{
						Identity: envelope.Identity{
							ID: "env-123",
						},
					}, nil
				},
			}

			srv := NewServer(mock)
			payload := marshalJSON(t, map[string]any{
				"envelope_id": "env-123",
				"decision":    tt.input,
				"reviewer":    "reviewer-1",
			})

			rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", payload)

			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rec.Code)
			}

			if capturedDecision != tt.expected {
				t.Errorf("expected decision %v, got %v", tt.expected, capturedDecision)
			}
		})
	}
}

func TestReview_InvalidDecision(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	payload := marshalJSON(t, map[string]any{
		"envelope_id": "env-123",
		"decision":    "maybe",
		"reviewer":    "reviewer-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", payload)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "decision must be 'accept'/'approve' or 'reject'/'deny'" {
		t.Errorf("unexpected error: %q", errResp["error"])
	}
}

func TestReview_MissingRequiredFields(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})

	tests := []struct {
		name    string
		payload map[string]any
		errMsg  string
	}{
		{
			name:    "missing envelope_id",
			payload: map[string]any{"decision": "accept", "reviewer": "reviewer-1"},
			errMsg:  "envelope_id is required",
		},
		{
			name:    "missing decision",
			payload: map[string]any{"envelope_id": "env-123", "reviewer": "reviewer-1"},
			errMsg:  "decision is required",
		},
		{
			name:    "missing reviewer",
			payload: map[string]any{"envelope_id": "env-123", "decision": "accept"},
			errMsg:  "reviewer must be a valid identifier (1-255 characters, no control characters)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := marshalJSON(t, tt.payload)
			rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != tt.errMsg {
				t.Errorf("expected error %q, got %q", tt.errMsg, errResp["error"])
			}
		})
	}
}

func TestReview_InvalidJSON(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", []byte("not-json"))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "invalid JSON payload" {
		t.Errorf("unexpected error: %q", errResp["error"])
	}
}

func TestReview_UnknownFieldsRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})

	body := []byte(`{
		"envelope_id": "env-123",
		"decision": "accept",
		"reviewer": "reviewer-1",
		"unknown_field": "should-be-rejected"
	}`)

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for unknown fields, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "invalid JSON payload" {
		t.Errorf("expected 'invalid JSON payload' error, got %q", errResp["error"])
	}
}

func TestReview_TrailingGarbageRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	body := []byte(`{"envelope_id":"env-123","decision":"accept","reviewer":"reviewer-1"}garbage`)

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for trailing garbage, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "invalid JSON payload" {
		t.Errorf("expected 'invalid JSON payload' error, got %q", errResp["error"])
	}
}

func TestReview_EmptyBodyRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", []byte(""))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body must not be empty" {
		t.Errorf("expected 'request body must not be empty' error, got %q", errResp["error"])
	}
}

func TestReview_WhitespaceOnlyBodyRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", []byte("   \n\t  "))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for whitespace-only body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body must not be empty" {
		t.Errorf("expected 'request body must not be empty' error, got %q", errResp["error"])
	}
}

func TestReview_OversizedBodyRejected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	largeBody := bytes.Repeat([]byte("x"), (1<<20)+1)

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", largeBody)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 for oversized body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body too large" {
		t.Errorf("expected 'request body too large' error, got %q", errResp["error"])
	}
}

func TestReview_WhitespaceTrimming(t *testing.T) {
	var capturedResolution decision.EscalationResolution

	mock := &mockOrchestrator{
		resolveEscalationFn: func(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error) {
			capturedResolution = resolution
			return &envelope.Envelope{
				Identity: envelope.Identity{
					ID: "env-123",
				},
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"envelope_id": "  env-123  ",
		"decision":    "  accept  ",
		"reviewer":    "\treviewer-1\n",
		"notes":       "  some notes  ",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if capturedResolution.EnvelopeID != "env-123" {
		t.Errorf("expected trimmed envelope_id 'env-123', got %q", capturedResolution.EnvelopeID)
	}
	if capturedResolution.ReviewerID != "reviewer-1" {
		t.Errorf("expected trimmed reviewer 'reviewer-1', got %q", capturedResolution.ReviewerID)
	}
	if capturedResolution.Notes != "some notes" {
		t.Errorf("expected trimmed notes 'some notes', got %q", capturedResolution.Notes)
	}
}

func TestReview_InvalidReviewer(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})

	tests := []struct {
		name     string
		reviewer string
	}{
		{"backslash", "reviewer\\123"},
		{"null byte", "reviewer\x00123"},
		{"control character", "reviewer\x01123"},
		{"too long", strings.Repeat("x", 256)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := marshalJSON(t, map[string]any{
				"envelope_id": "env-123",
				"decision":    "accept",
				"reviewer":    tt.reviewer,
			})

			rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", payload)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400 for invalid reviewer, got %d", rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != "reviewer must be a valid identifier (1-255 characters, no control characters)" {
				t.Errorf("unexpected error: %q", errResp["error"])
			}
		})
	}
}

func TestReview_EnvelopeIdentityInvariantViolation(t *testing.T) {
	mock := &mockOrchestrator{
		resolveEscalationFn: func(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error) {
			return &envelope.Envelope{
				Identity: envelope.Identity{
					ID: "env-DIFFERENT",
				},
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"envelope_id": "env-123",
		"decision":    "accept",
		"reviewer":    "reviewer-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", payload)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for invariant violation, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "envelope identity invariant violated" {
		t.Errorf("expected 'envelope identity invariant violated' error, got %q", errResp["error"])
	}
}

func TestReview_MethodNotAllowed(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/reviews", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("expected Allow header 'POST', got %q", allow)
	}
}

func TestReview_NilOrchestrator(t *testing.T) {
	srv := NewServer(nil)
	payload := marshalJSON(t, map[string]any{
		"envelope_id": "env-123",
		"decision":    "accept",
		"reviewer":    "reviewer-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/reviews", payload)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for nil orchestrator, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "orchestrator not configured" {
		t.Errorf("expected 'orchestrator not configured' error, got %q", errResp["error"])
	}
}

// ---------------------------------------------------------------------------
// Domain Error Mapping Tests
// ---------------------------------------------------------------------------

func TestDomainErrorMapping_TypedSentinels(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "ErrEnvelopeNotFound",
			err:            decision.ErrEnvelopeNotFound,
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "evaluation not found",
		},
		{
			name:           "ErrEnvelopeNotAwaitingReview",
			err:            decision.ErrEnvelopeNotAwaitingReview,
			expectedStatus: http.StatusConflict,
			expectedMsg:    decision.ErrEnvelopeNotAwaitingReview.Error(),
		},
		{
			name:           "ErrEnvelopeAlreadyClosed",
			err:            decision.ErrEnvelopeAlreadyClosed,
			expectedStatus: http.StatusConflict,
			expectedMsg:    decision.ErrEnvelopeAlreadyClosed.Error(),
		},
		{
			name:           "ErrEmptyIdentifier",
			err:            decision.ErrEmptyIdentifier,
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    decision.ErrEmptyIdentifier.Error(),
		},
		{
			name:           "ErrInvalidReviewDecision",
			err:            decision.ErrInvalidReviewDecision,
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    decision.ErrInvalidReviewDecision.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockOrchestrator{
				evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
					return decision.EvaluationResult{}, tt.err
				},
			}

			srv := NewServer(mock)
			payload := marshalJSON(t, map[string]any{
				"surface_id": "surf-1",
				"agent_id":   "agent-1",
				"confidence": 0.9,
			})

			rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != tt.expectedMsg {
				t.Errorf("expected error %q, got %q", tt.expectedMsg, errResp["error"])
			}
		})
	}
}

func TestDomainErrorMapping_StringMatchFallback(t *testing.T) {
	tests := []struct {
		name           string
		errMsg         string
		expectedStatus int
	}{
		{
			name:           "self-review",
			errMsg:         "cannot self-review decision",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "duplicate",
			errMsg:         "duplicate request detected",
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "not found fallback",
			errMsg:         "resource not found",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid state",
			errMsg:         "invalid state transition",
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockOrchestrator{
				evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
					return decision.EvaluationResult{}, errors.New(tt.errMsg)
				},
			}

			srv := NewServer(mock)
			payload := marshalJSON(t, map[string]any{
				"surface_id": "surf-1",
				"agent_id":   "agent-1",
				"confidence": 0.9,
			})

			rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != tt.errMsg {
				t.Errorf("expected error %q, got %q", tt.errMsg, errResp["error"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetEnvelope Tests
// ---------------------------------------------------------------------------

func TestGetEnvelope_Success(t *testing.T) {
	mockEnv := &envelope.Envelope{
		Identity: envelope.Identity{
			ID: "env-123",
		},
	}

	mock := &mockOrchestrator{
		getEnvelopeByIDFn: func(ctx context.Context, id string) (*envelope.Envelope, error) {
			if id != "env-123" {
				t.Errorf("expected id 'env-123', got %q", id)
			}
			return mockEnv, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes/env-123", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	resp := decodeJSON[envelope.Envelope](t, rec)
	if resp.Identity.ID != "env-123" {
		t.Errorf("expected envelope ID 'env-123', got %q", resp.Identity.ID)
	}
}

func TestGetEnvelope_NotFound(t *testing.T) {
	mock := &mockOrchestrator{
		getEnvelopeByIDFn: func(ctx context.Context, id string) (*envelope.Envelope, error) {
			return nil, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes/env-missing", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "envelope not found" {
		t.Errorf("expected error 'envelope not found', got %q", errResp["error"])
	}
}

func TestGetEnvelope_MissingID(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes/", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "missing envelope id" {
		t.Errorf("expected 'missing envelope id' error, got %q", errResp["error"])
	}
}

func TestGetEnvelope_InvalidID(t *testing.T) {
	mock := &mockOrchestrator{
		getEnvelopeByIDFn: func(ctx context.Context, id string) (*envelope.Envelope, error) {
			t.Error("orchestrator should not be called for invalid envelope ID")
			return nil, nil
		},
	}

	srv := NewServer(mock)

	tests := []struct {
		name string
		id   string
	}{
		{"backslash", "env\\123"},
		{"slash", "env/123"},
		{"too long", strings.Repeat("x", 256)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes/"+tt.id, nil)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400 for invalid ID, got %d", rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != "invalid envelope id" {
				t.Errorf("expected 'invalid envelope id' error, got %q", errResp["error"])
			}
		})
	}
}

func TestGetEnvelope_OrchestratorError(t *testing.T) {
	mock := &mockOrchestrator{
		getEnvelopeByIDFn: func(ctx context.Context, id string) (*envelope.Envelope, error) {
			return nil, fmt.Errorf("database error")
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes/env-123", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "database error" {
		t.Errorf("expected error 'database error', got %q", errResp["error"])
	}
}

func TestGetEnvelope_MethodNotAllowed(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/envelopes/env-123", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("expected Allow header 'GET', got %q", allow)
	}
}

func TestGetEnvelope_NilOrchestrator(t *testing.T) {
	srv := NewServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes/env-123", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for nil orchestrator, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "orchestrator not configured" {
		t.Errorf("expected 'orchestrator not configured' error, got %q", errResp["error"])
	}
}

// ---------------------------------------------------------------------------
// ListEnvelopes Tests
// ---------------------------------------------------------------------------

func TestListEnvelopes_Success(t *testing.T) {
	mockEnvs := []*envelope.Envelope{
		{
			Identity: envelope.Identity{
				ID: "env-1",
			},
		},
		{
			Identity: envelope.Identity{
				ID: "env-2",
			},
		},
	}

	mock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			// No state filter — return all.
			if state != "" {
				t.Errorf("expected empty state filter, got %q", state)
			}
			return mockEnvs, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	resp := decodeJSON[[]*envelope.Envelope](t, rec)
	if len(resp) != 2 {
		t.Errorf("expected 2 envelopes, got %d", len(resp))
	}
}

func TestListEnvelopes_EmptyList(t *testing.T) {
	mock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			return []*envelope.Envelope{}, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	resp := decodeJSON[[]*envelope.Envelope](t, rec)
	if len(resp) != 0 {
		t.Errorf("expected 0 envelopes, got %d", len(resp))
	}
}

func TestListEnvelopes_MethodNotAllowed(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/envelopes", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("expected Allow header 'GET', got %q", allow)
	}
}

func TestListEnvelopes_NilOrchestrator(t *testing.T) {
	srv := NewServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for nil orchestrator, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "orchestrator not configured" {
		t.Errorf("expected 'orchestrator not configured' error, got %q", errResp["error"])
	}
}

// ---------------------------------------------------------------------------
// GetDecisionByRequestID Tests
// ---------------------------------------------------------------------------

func TestGetDecisionByRequestID_Success(t *testing.T) {
	mockEnv := &envelope.Envelope{
		Identity: envelope.Identity{
			ID: "env-123",
		},
	}

	mock := &mockOrchestrator{
		getEnvelopeByRequestScopeFn: func(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
			if requestID != "req-123" {
				t.Errorf("expected request_id 'req-123', got %q", requestID)
			}
			if requestSource != "api" {
				t.Errorf("expected default request_source 'api', got %q", requestSource)
			}
			return mockEnv, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/decisions/request/req-123", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	resp := decodeJSON[envelope.Envelope](t, rec)
	if resp.Identity.ID != "env-123" {
		t.Errorf("expected envelope ID 'env-123', got %q", resp.Identity.ID)
	}
}

func TestGetDecisionByRequestID_CustomSource(t *testing.T) {
	mockEnv := &envelope.Envelope{
		Identity: envelope.Identity{
			ID: "env-123",
		},
	}

	mock := &mockOrchestrator{
		getEnvelopeByRequestScopeFn: func(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
			if requestSource != "custom-source" {
				t.Errorf("expected request_source 'custom-source', got %q", requestSource)
			}
			return mockEnv, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/decisions/request/req-123?source=custom-source", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestGetDecisionByRequestID_WhitespaceTrimmedSource(t *testing.T) {
	mockEnv := &envelope.Envelope{
		Identity: envelope.Identity{
			ID: "env-123",
		},
	}

	mock := &mockOrchestrator{
		getEnvelopeByRequestScopeFn: func(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
			if requestSource != "custom-source" {
				t.Errorf("expected trimmed request_source 'custom-source', got %q", requestSource)
			}
			return mockEnv, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/decisions/request/req-123?source=%20%20custom-source%20%20", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestGetDecisionByRequestID_NotFound(t *testing.T) {
	mock := &mockOrchestrator{
		getEnvelopeByRequestScopeFn: func(ctx context.Context, requestSource, requestID string) (*envelope.Envelope, error) {
			return nil, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/decisions/request/req-missing", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "decision not found" {
		t.Errorf("expected error 'decision not found', got %q", errResp["error"])
	}
}

func TestGetDecisionByRequestID_MissingID(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/decisions/request/", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "missing request id" {
		t.Errorf("expected 'missing request id' error, got %q", errResp["error"])
	}
}

func TestGetDecisionByRequestID_InvalidID(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})

	tests := []struct {
		name string
		id   string
	}{
		{"backslash", "req\\123"},
		{"too long", strings.Repeat("x", 256)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performRequest(t, srv, http.MethodGet, "/v1/decisions/request/"+tt.id, nil)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400 for invalid ID, got %d", rec.Code)
			}

			errResp := decodeError(t, rec)
			if errResp["error"] != "invalid request id" {
				t.Errorf("expected 'invalid request id' error, got %q", errResp["error"])
			}
		})
	}
}

func TestGetDecisionByRequestID_MethodNotAllowed(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/decisions/request/req-123", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("expected Allow header 'GET', got %q", allow)
	}
}

func TestGetDecisionByRequestID_NilOrchestrator(t *testing.T) {
	srv := NewServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/decisions/request/req-123", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for nil orchestrator, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "orchestrator not configured" {
		t.Errorf("expected 'orchestrator not configured' error, got %q", errResp["error"])
	}
}

// ---------------------------------------------------------------------------
// Content-Type Tests
// ---------------------------------------------------------------------------

func TestResponsesHaveJSONContentType(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
			}, nil
		},
		getEnvelopeByIDFn: func(ctx context.Context, id string) (*envelope.Envelope, error) {
			return nil, nil
		},
	}

	srv := NewServer(mock)

	tests := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"health", http.MethodGet, "/healthz", nil},
		{"ready", http.MethodGet, "/readyz", nil},
		{"evaluate success", http.MethodPost, "/v1/evaluate", marshalJSON(t, map[string]any{
			"surface_id": "s1",
			"agent_id":   "a1",
			"confidence": 0.9,
		})},
		{"evaluate bad request", http.MethodPost, "/v1/evaluate", []byte("invalid")},
		{"envelope not found", http.MethodGet, "/v1/envelopes/missing", nil},
		{"method not allowed", http.MethodPost, "/healthz", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performRequest(t, srv, tt.method, tt.path, tt.body)

			ct := rec.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type to contain 'application/json', got %q", ct)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mock Control Plane Service
// ---------------------------------------------------------------------------

type mockControlPlane struct {
	applyBundleFn func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error)
	planBundleFn  func(ctx context.Context, bundle []byte) (*apply.ApplyPlan, error)
}

func (m *mockControlPlane) ApplyBundle(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
	if m.applyBundleFn != nil {
		return m.applyBundleFn(ctx, bundle, actor)
	}
	return nil, fmt.Errorf("applyBundle not implemented")
}

func (m *mockControlPlane) PlanBundle(ctx context.Context, bundle []byte) (*apply.ApplyPlan, error) {
	if m.planBundleFn != nil {
		return m.planBundleFn(ctx, bundle)
	}
	return nil, fmt.Errorf("planBundle not implemented")
}

// ---------------------------------------------------------------------------
// Test Helper for Headers
// ---------------------------------------------------------------------------

func performRequestWithHeaders(t *testing.T, srv *Server, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Control Plane - Apply Bundle Tests
// ---------------------------------------------------------------------------

func TestApplyBundle_Success(t *testing.T) {
	yamlBundle := `---
apiVersion: midas/v1
kind: Surface
metadata:
  id: test-surface
spec:
  name: Test Surface
  category: test
  risk_tier: tier-1
  status: active
`

	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			if !bytes.Contains(bundle, []byte("test-surface")) {
				t.Errorf("expected bundle to contain 'test-surface'")
			}

			result := &cpTypes.ApplyResult{}
			result.AddCreated("Surface", "test-surface")
			return result, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte(yamlBundle), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[cpTypes.ApplyResult](t, rec)
	if resp.CreatedCount() != 1 {
		t.Errorf("expected 1 created resource, got %d", resp.CreatedCount())
	}
	if len(resp.Results) != 1 || resp.Results[0].ID != "test-surface" {
		t.Errorf("expected created resource 'test-surface'")
	}
}

func TestApplyBundle_AllowedContentTypes(t *testing.T) {
	yamlBundle := `---
apiVersion: midas/v1
kind: Surface
metadata:
  id: test-surface
spec:
  name: Test Surface
  category: test
  risk_tier: tier-1
  status: active
`

	tests := []struct {
		name        string
		contentType string
	}{
		{"application/yaml", "application/yaml"},
		{"application/x-yaml", "application/x-yaml"},
		{"text/yaml", "text/yaml"},
		{"no content-type", ""},
		{"with charset", "application/yaml; charset=utf-8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCP := &mockControlPlane{
				applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
					result := &cpTypes.ApplyResult{}
					result.AddCreated("Surface", "test-surface")
					return result, nil
				},
			}

			srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
			rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
				[]byte(yamlBundle), map[string]string{"Content-Type": tt.contentType})

			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200 for %q, got %d", tt.contentType, rec.Code)
			}
		})
	}
}

func TestApplyBundle_UnsupportedMediaType(t *testing.T) {
	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			t.Error("applyBundle should not be called for unsupported content type")
			return nil, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte("some content"), map[string]string{"Content-Type": "application/json"})

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status 415, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], "application/yaml") {
		t.Errorf("expected error to mention 'application/yaml', got %q", errResp["error"])
	}
}

func TestApplyBundle_ValidationErrors(t *testing.T) {
	yamlBundle := `---
apiVersion: midas/v1
kind: Surface
metadata:
  id: surf-1
spec:
  category: test
  risk_tier: tier-1
  status: active
`

	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			// Validation errors are returned in ApplyResult.ValidationErrors
			result := &cpTypes.ApplyResult{}
			result.AddFieldError("Surface", "surf-1", "name", "required")
			return result, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte(yamlBundle), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (validation errors in result), got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[cpTypes.ApplyResult](t, rec)
	if resp.ValidationErrorCount() != 1 {
		t.Errorf("expected 1 validation error, got %d", resp.ValidationErrorCount())
	}
	if len(resp.ValidationErrors) != 1 {
		t.Fatalf("expected 1 validation error in array, got %d", len(resp.ValidationErrors))
	}
	if resp.ValidationErrors[0].Field != "name" {
		t.Errorf("expected field 'name', got %q", resp.ValidationErrors[0].Field)
	}
}

func TestApplyBundle_EmptyBody(t *testing.T) {
	srv := NewServerWithControlPlane(&mockOrchestrator{}, &mockControlPlane{})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte(""), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body must not be empty" {
		t.Errorf("expected 'request body must not be empty' error, got %q", errResp["error"])
	}
}

func TestApplyBundle_WhitespaceOnlyBody(t *testing.T) {
	srv := NewServerWithControlPlane(&mockOrchestrator{}, &mockControlPlane{})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte("   \n\t  "), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for whitespace-only body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body must not be empty" {
		t.Errorf("expected 'request body must not be empty' error, got %q", errResp["error"])
	}
}

func TestApplyBundle_OversizedBody(t *testing.T) {
	srv := NewServerWithControlPlane(&mockOrchestrator{}, &mockControlPlane{})
	largeBody := bytes.Repeat([]byte("x"), (10<<20)+1) // 10 MiB + 1 byte

	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		largeBody, map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 for oversized body, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "request body too large" {
		t.Errorf("expected 'request body too large' error, got %q", errResp["error"])
	}
}

func TestApplyBundle_MethodNotAllowed(t *testing.T) {
	srv := NewServerWithControlPlane(&mockOrchestrator{}, &mockControlPlane{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/controlplane/apply", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("expected Allow header 'POST', got %q", allow)
	}
}

func TestApplyBundle_NilControlPlane(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte("some yaml"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501 for nil control plane, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "control plane not configured" {
		t.Errorf("expected 'control plane not configured' error, got %q", errResp["error"])
	}
}

func TestApplyBundle_MultipleResources(t *testing.T) {
	yamlBundle := `---
apiVersion: midas/v1
kind: Surface
metadata:
  id: surf-1
spec:
  name: Surface One
  category: test
  risk_tier: tier-1
  status: active
---
apiVersion: midas/v1
kind: Agent
metadata:
  id: agent-1
spec:
  name: Agent One
  type: llm
  runtime:
    provider: anthropic
    model: claude-sonnet-4
  status: active
`

	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			result := &cpTypes.ApplyResult{}
			result.AddCreated("Surface", "surf-1")
			result.AddCreated("Agent", "agent-1")
			return result, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte(yamlBundle), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[cpTypes.ApplyResult](t, rec)
	if resp.CreatedCount() != 2 {
		t.Errorf("expected 2 created resources, got %d", resp.CreatedCount())
	}
}

func TestApplyBundle_ParseErrors(t *testing.T) {
	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			return nil, fmt.Errorf("%w: invalid yaml syntax", apply.ErrInvalidBundle)
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte("invalid: [yaml}"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for parse error, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], apply.ErrInvalidBundle.Error()) {
		t.Errorf("expected error to mention %q, got %q", apply.ErrInvalidBundle.Error(), errResp["error"])
	}
}

func TestApplyBundle_InfrastructureError(t *testing.T) {
	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			return nil, errors.New("database connection failed")
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte("valid yaml"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for infrastructure error, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], "database") {
		t.Errorf("expected error to mention 'database', got %q", errResp["error"])
	}
}

func TestApplyBundle_MixedResults(t *testing.T) {
	yamlBundle := `---
apiVersion: midas/v1
kind: Surface
metadata:
  id: surf-new
spec:
  name: New Surface
  category: test
  risk_tier: tier-1
  status: active
---
apiVersion: midas/v1
kind: Surface
metadata:
  id: surf-existing
spec:
  name: Existing Surface
  category: test
  risk_tier: tier-1
  status: active
`

	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			result := &cpTypes.ApplyResult{}
			result.AddCreated("Surface", "surf-new")
			result.AddConflict("Surface", "surf-existing", "already exists")
			return result, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte(yamlBundle), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[cpTypes.ApplyResult](t, rec)
	if resp.CreatedCount() != 1 {
		t.Errorf("expected 1 created resource, got %d", resp.CreatedCount())
	}
	if resp.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict, got %d", resp.ConflictCount())
	}
}

func TestApplyBundle_ResponseStructure(t *testing.T) {
	yamlBundle := `---
apiVersion: midas/v1
kind: Surface
metadata:
  id: test-surface
spec:
  name: Test Surface
  category: test
  risk_tier: tier-1
  status: active
`

	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			result := &cpTypes.ApplyResult{}
			result.AddCreated("Surface", "test-surface")
			return result, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte(yamlBundle), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify response has JSON Content-Type
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type to contain 'application/json', got %q", ct)
	}

	// Verify response structure via counts and content (not nil-slice checks)
	resp := decodeJSON[cpTypes.ApplyResult](t, rec)

	if resp.CreatedCount() != 1 {
		t.Errorf("expected 1 created resource, got %d", resp.CreatedCount())
	}
	if resp.ConflictCount() != 0 {
		t.Errorf("expected 0 conflicts, got %d", resp.ConflictCount())
	}
	if resp.ApplyErrorCount() != 0 {
		t.Errorf("expected 0 errors, got %d", resp.ApplyErrorCount())
	}
	if resp.UnchangedCount() != 0 {
		t.Errorf("expected 0 unchanged, got %d", resp.UnchangedCount())
	}
	if resp.ValidationErrorCount() != 0 {
		t.Errorf("expected 0 validation errors, got %d", resp.ValidationErrorCount())
	}

	// Verify actual content in Results array
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Kind != "Surface" {
		t.Errorf("expected kind 'Surface', got %q", resp.Results[0].Kind)
	}
	if resp.Results[0].ID != "test-surface" {
		t.Errorf("expected id 'test-surface', got %q", resp.Results[0].ID)
	}
	if resp.Results[0].Status != cpTypes.ResourceStatusCreated {
		t.Errorf("expected status 'created', got %q", resp.Results[0].Status)
	}
}

// ---------------------------------------------------------------------------
// Idempotency / scoped-replay HTTP tests
// ---------------------------------------------------------------------------

// TestEvaluate_ScopedConflictReturns409 verifies that the HTTP layer maps
// ErrScopedRequestConflict → 409 Conflict.
func TestEvaluate_ScopedConflictReturns409(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{}, decision.ErrScopedRequestConflict
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id":     "surf-1",
		"agent_id":       "agent-1",
		"confidence":     0.9,
		"request_source": "svc-a",
		"request_id":     "req-001",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict, got %d", rec.Code)
	}
	errResp := decodeError(t, rec)
	want := decision.ErrScopedRequestConflict.Error()
	if errResp["error"] != want {
		t.Errorf("error message: got %q, want %q", errResp["error"], want)
	}
}

// TestEvaluate_ScopedConflictWrappedReturns409 verifies that wrapping
// ErrScopedRequestConflict in another error still maps to 409
// (errors.Is chain is preserved).
func TestEvaluate_ScopedConflictWrappedReturns409(t *testing.T) {
	wrappedErr := fmt.Errorf("inner: %w", decision.ErrScopedRequestConflict)
	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{}, wrappedErr
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict for wrapped ErrScopedRequestConflict, got %d", rec.Code)
	}
}

// TestEvaluate_ScopedConflictInDomainErrorMappingTable verifies the sentinel
// error is covered by the typed sentinel table (regression guard against
// future refactors that could silently break the mapping).
func TestDomainErrorMapping_ScopedRequestConflict(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{}, decision.ErrScopedRequestConflict
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusConflict {
		t.Errorf("ErrScopedRequestConflict must map to 409, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], "scoped request conflict") {
		t.Errorf("error body should mention 'scoped request conflict', got %q", errResp["error"])
	}
}

// TestEvaluate_SuccessfulReplayPassesThroughResult verifies that when the
// orchestrator returns a successful result (idempotent replay), the HTTP layer
// responds 200 with the correct fields.
func TestEvaluate_SuccessfulReplayPassesThroughResult(t *testing.T) {
	replayResult := decision.EvaluationResult{
		Outcome:    eval.OutcomeAccept,
		ReasonCode: eval.ReasonWithinAuthority,
		EnvelopeID: "env-original-001",
	}

	mock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			return replayResult, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id":     "surf-1",
		"agent_id":       "agent-1",
		"confidence":     0.9,
		"request_source": "svc-a",
		"request_id":     "req-replay-01",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)

	if rec.Code != http.StatusOK {
		t.Errorf("replay response must be 200, got %d", rec.Code)
	}

	resp := decodeJSON[evaluateResponse](t, rec)
	if resp.EnvelopeID != "env-original-001" {
		t.Errorf("EnvelopeID: got %q, want %q", resp.EnvelopeID, "env-original-001")
	}
	if resp.Outcome != string(eval.OutcomeAccept) {
		t.Errorf("Outcome: got %q, want %q", resp.Outcome, eval.OutcomeAccept)
	}
}

// ---------------------------------------------------------------------------
// POST /v1/controlplane/plan — dry-run plan endpoint
// ---------------------------------------------------------------------------

// TestPlanBundle_NotConfigured_Returns501 verifies that the plan endpoint
// returns 501 when no control plane service is configured.
func TestPlanBundle_NotConfigured_Returns501(t *testing.T) {
	srv := NewServerWithControlPlane(&mockOrchestrator{}, nil)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/plan",
		[]byte("some: yaml"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501, got %d", rec.Code)
	}
}

// TestPlanBundle_MethodNotAllowed_Returns405 verifies that the plan endpoint
// rejects non-POST methods.
func TestPlanBundle_MethodNotAllowed_Returns405(t *testing.T) {
	srv := NewServerWithControlPlane(&mockOrchestrator{}, &mockControlPlane{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/controlplane/plan", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

// TestPlanBundle_UnsupportedMediaType_Returns415 verifies that the plan
// endpoint rejects non-YAML content types.
func TestPlanBundle_UnsupportedMediaType_Returns415(t *testing.T) {
	srv := NewServerWithControlPlane(&mockOrchestrator{}, &mockControlPlane{})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/plan",
		[]byte("{}"), map[string]string{"Content-Type": "application/json"})

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status 415, got %d", rec.Code)
	}
}

// TestPlanBundle_EmptyBody_Returns400 verifies that an empty request body
// returns 400.
func TestPlanBundle_EmptyBody_Returns400(t *testing.T) {
	srv := NewServerWithControlPlane(&mockOrchestrator{}, &mockControlPlane{})
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/plan",
		[]byte(""), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty body, got %d", rec.Code)
	}
}

// TestPlanBundle_ParseError_Returns400 verifies that a bundle that fails to
// parse returns 400 (ErrInvalidBundle).
func TestPlanBundle_ParseError_Returns400(t *testing.T) {
	mockCP := &mockControlPlane{
		planBundleFn: func(ctx context.Context, bundle []byte) (*apply.ApplyPlan, error) {
			return nil, fmt.Errorf("%w: invalid yaml syntax", apply.ErrInvalidBundle)
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/plan",
		[]byte("some: yaml"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for parse error, got %d", rec.Code)
	}
}

// TestPlanBundle_ValidBundle_ReturnsPlanResult verifies that a successful
// plan request returns a structured PlanResult with WouldApply and counts.
func TestPlanBundle_ValidBundle_ReturnsPlanResult(t *testing.T) {
	mockCP := &mockControlPlane{
		planBundleFn: func(ctx context.Context, bundle []byte) (*apply.ApplyPlan, error) {
			plan := &apply.ApplyPlan{
				Entries: []apply.ApplyPlanEntry{
					{
						Kind:           "Surface",
						ID:             "surf-1",
						Action:         apply.ApplyActionCreate,
						DocumentIndex:  1,
						DecisionSource: apply.DecisionSourcePersistedState,
					},
				},
			}
			return plan, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/plan",
		[]byte("some: yaml"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[cpTypes.PlanResult](t, rec)
	if !resp.WouldApply {
		t.Error("expected WouldApply == true for create-only plan")
	}
	if resp.CreateCount != 1 {
		t.Errorf("expected CreateCount == 1, got %d", resp.CreateCount)
	}
	if resp.InvalidCount != 0 {
		t.Errorf("expected InvalidCount == 0, got %d", resp.InvalidCount)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry in plan result, got %d", len(resp.Entries))
	}
	if resp.Entries[0].Kind != "Surface" {
		t.Errorf("expected entry kind 'Surface', got %q", resp.Entries[0].Kind)
	}
	if resp.Entries[0].Action != cpTypes.PlanEntryActionCreate {
		t.Errorf("expected entry action %q, got %q", cpTypes.PlanEntryActionCreate, resp.Entries[0].Action)
	}
	if resp.Entries[0].DecisionSource != cpTypes.PlanEntryDecisionSourcePersistedState {
		t.Errorf("expected DecisionSource %q, got %q",
			cpTypes.PlanEntryDecisionSourcePersistedState, resp.Entries[0].DecisionSource)
	}
}

// TestPlanBundle_PerformsNoWrites verifies that the plan endpoint never
// results in repository writes. This is enforced at the service layer; the
// HTTP test validates that calling /plan does not call apply's write path.
func TestPlanBundle_HTTPNoWrites_ReturnsPlanResult(t *testing.T) {
	writeCalled := false
	mockCP := &mockControlPlane{
		planBundleFn: func(ctx context.Context, bundle []byte) (*apply.ApplyPlan, error) {
			// Simulate a pure-read plan with no writes.
			return &apply.ApplyPlan{
				Entries: []apply.ApplyPlanEntry{
					{
						Kind:           "Surface",
						ID:             "surf-1",
						Action:         apply.ApplyActionCreate,
						DocumentIndex:  1,
						DecisionSource: apply.DecisionSourcePersistedState,
					},
				},
			}, nil
		},
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			writeCalled = true
			return nil, fmt.Errorf("apply must not be called by plan endpoint")
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/plan",
		[]byte("some: yaml"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if writeCalled {
		t.Error("plan endpoint must not trigger any write (ApplyBundle was called)")
	}
}

// TestPlanBundle_InvalidBundle_WouldApplyFalse verifies that a plan response
// for an invalid bundle has WouldApply == false and InvalidCount > 0.
func TestPlanBundle_InvalidBundle_WouldApplyFalse(t *testing.T) {
	mockCP := &mockControlPlane{
		planBundleFn: func(ctx context.Context, bundle []byte) (*apply.ApplyPlan, error) {
			return &apply.ApplyPlan{
				Entries: []apply.ApplyPlanEntry{
					{
						Kind:           "Surface",
						ID:             "surf-bad",
						Action:         apply.ApplyActionInvalid,
						DocumentIndex:  1,
						DecisionSource: apply.DecisionSourceValidation,
						ValidationErrors: []cpTypes.ValidationError{
							{Kind: "Surface", ID: "surf-bad", Field: "metadata.name", Message: "required"},
						},
					},
				},
			}, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/plan",
		[]byte("some: yaml"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 even for invalid plan (it's a dry-run), got %d", rec.Code)
	}

	resp := decodeJSON[cpTypes.PlanResult](t, rec)
	if resp.WouldApply {
		t.Error("expected WouldApply == false for invalid plan")
	}
	if resp.InvalidCount != 1 {
		t.Errorf("expected InvalidCount == 1, got %d", resp.InvalidCount)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
	if len(resp.Entries[0].ValidationErrors) == 0 {
		t.Error("expected ValidationErrors to be present in plan entry")
	}
}

// TestPlanBundle_BundleDependency_DecisionSourceVisible verifies that the HTTP
// response exposes DecisionSource == bundle_dependency for entries whose
// references are satisfied within the same bundle.
func TestPlanBundle_BundleDependency_DecisionSourceVisible(t *testing.T) {
	mockCP := &mockControlPlane{
		planBundleFn: func(ctx context.Context, bundle []byte) (*apply.ApplyPlan, error) {
			return &apply.ApplyPlan{
				Entries: []apply.ApplyPlanEntry{
					{
						Kind:           "Surface",
						ID:             "surf-1",
						Action:         apply.ApplyActionCreate,
						DocumentIndex:  1,
						DecisionSource: apply.DecisionSourcePersistedState,
					},
					{
						Kind:           "Profile",
						ID:             "profile-1",
						Action:         apply.ApplyActionCreate,
						DocumentIndex:  2,
						DecisionSource: apply.DecisionSourceBundleDependency,
					},
				},
			}, nil
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/plan",
		[]byte("some: yaml"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[cpTypes.PlanResult](t, rec)
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(resp.Entries))
	}

	profileEntry := resp.Entries[1]
	if profileEntry.DecisionSource != cpTypes.PlanEntryDecisionSourceBundleDependency {
		t.Errorf("expected profile entry DecisionSource %q, got %q",
			cpTypes.PlanEntryDecisionSourceBundleDependency, profileEntry.DecisionSource)
	}
}

// ---------------------------------------------------------------------------
// Mock Approval Service
// ---------------------------------------------------------------------------

type mockApprovalService struct {
	approveSurfaceFn   func(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error)
	deprecateSurfaceFn func(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error)
	approveProfileFn   func(ctx context.Context, profileID string, version int, approvedBy string) (*authority.AuthorityProfile, error)
	deprecateProfileFn func(ctx context.Context, profileID string, version int, deprecatedBy string) (*authority.AuthorityProfile, error)
}

func (m *mockApprovalService) ApproveSurface(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
	if m.approveSurfaceFn != nil {
		return m.approveSurfaceFn(ctx, surfaceID, submitter, approver)
	}
	return nil, fmt.Errorf("approveSurface not implemented")
}

func (m *mockApprovalService) DeprecateSurface(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error) {
	if m.deprecateSurfaceFn != nil {
		return m.deprecateSurfaceFn(ctx, surfaceID, deprecatedBy, reason, successorID)
	}
	return nil, fmt.Errorf("deprecateSurface not implemented")
}

func (m *mockApprovalService) ApproveProfile(ctx context.Context, profileID string, version int, approvedBy string) (*authority.AuthorityProfile, error) {
	if m.approveProfileFn != nil {
		return m.approveProfileFn(ctx, profileID, version, approvedBy)
	}
	return nil, fmt.Errorf("approveProfile not implemented")
}

func (m *mockApprovalService) DeprecateProfile(ctx context.Context, profileID string, version int, deprecatedBy string) (*authority.AuthorityProfile, error) {
	if m.deprecateProfileFn != nil {
		return m.deprecateProfileFn(ctx, profileID, version, deprecatedBy)
	}
	return nil, fmt.Errorf("deprecateProfile not implemented")
}

// ---------------------------------------------------------------------------
// Approve Surface Endpoint Tests
// ---------------------------------------------------------------------------

func TestApproveSurface_Success(t *testing.T) {
	mock := &mockApprovalService{
		approveSurfaceFn: func(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
			if surfaceID != "payments.execute" {
				t.Errorf("expected surfaceID 'payments.execute', got %q", surfaceID)
			}
			if approver.ID != "approver-1" {
				t.Errorf("expected approver ID 'approver-1', got %q", approver.ID)
			}
			if submitter.ID != "submitter-1" {
				t.Errorf("expected submitter ID 'submitter-1', got %q", submitter.ID)
			}
			return &surface.DecisionSurface{
				ID:         "payments.execute",
				Status:     surface.SurfaceStatusActive,
				ApprovedBy: "approver-1",
			}, nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"submitted_by": "submitter-1",
		"approver_id":  "approver-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/payments.execute/approve", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[approveSurfaceResponse](t, rec)
	if resp.SurfaceID != "payments.execute" {
		t.Errorf("expected surface_id 'payments.execute', got %q", resp.SurfaceID)
	}
	if resp.Status != "active" {
		t.Errorf("expected status 'active', got %q", resp.Status)
	}
	if resp.ApprovedBy != "approver-1" {
		t.Errorf("expected approved_by 'approver-1', got %q", resp.ApprovedBy)
	}
}

func TestApproveSurface_SurfaceNotFound(t *testing.T) {
	mock := &mockApprovalService{
		approveSurfaceFn: func(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
			return nil, approval.ErrSurfaceNotFound
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"approver_id": "approver-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/nonexistent/approve", payload)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "surface not found" {
		t.Errorf("expected error 'surface not found', got %q", errResp["error"])
	}
}

func TestApproveSurface_Forbidden(t *testing.T) {
	mock := &mockApprovalService{
		approveSurfaceFn: func(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
			return nil, approval.ErrApprovalForbidden
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"submitted_by": "user-1",
		"approver_id":  "user-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", payload)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "approver is not authorized to approve this surface" {
		t.Errorf("unexpected error: %q", errResp["error"])
	}
}

func TestApproveSurface_InvalidStatus(t *testing.T) {
	mock := &mockApprovalService{
		approveSurfaceFn: func(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
			return nil, approval.ErrInvalidStatus
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"approver_id": "approver-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-active/approve", payload)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", rec.Code)
	}
}

func TestApproveSurface_NilApprovalService(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, nil)
	payload := marshalJSON(t, map[string]any{
		"approver_id": "approver-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", payload)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501 for nil approval service, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "approval service not configured" {
		t.Errorf("expected 'approval service not configured' error, got %q", errResp["error"])
	}
}

func TestApproveSurface_MissingApproverID(t *testing.T) {
	mock := &mockApprovalService{}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"submitted_by": "submitter-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", payload)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing approver_id, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "approver_id must be a valid identifier" {
		t.Errorf("unexpected error: %q", errResp["error"])
	}
}

func TestApproveSurface_InvalidSurfaceID(t *testing.T) {
	mock := &mockApprovalService{}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"approver_id": "approver-1",
	})

	// Path traversal in surface ID should be rejected.
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf/invalid/approve", payload)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for path with extra segment, got %d", rec.Code)
	}
}

func TestApproveSurface_MethodNotAllowed(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})

	rec := performRequest(t, srv, http.MethodGet, "/v1/controlplane/surfaces/surf-1/approve", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("expected Allow header 'POST', got %q", allow)
	}
}

func TestApproveSurface_UnknownFieldsRejected(t *testing.T) {
	mock := &mockApprovalService{}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body := []byte(`{"approver_id":"approver-1","unknown_field":"bad"}`)

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/approve", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for unknown fields, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "invalid JSON payload" {
		t.Errorf("expected 'invalid JSON payload', got %q", errResp["error"])
	}
}

// ---------------------------------------------------------------------------
// Deprecate Surface Endpoint Tests
// ---------------------------------------------------------------------------

func TestDeprecateSurface_Success(t *testing.T) {
	mock := &mockApprovalService{
		deprecateSurfaceFn: func(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error) {
			if surfaceID != "payments.execute" {
				t.Errorf("expected surfaceID 'payments.execute', got %q", surfaceID)
			}
			if deprecatedBy != "ops-admin" {
				t.Errorf("expected deprecatedBy 'ops-admin', got %q", deprecatedBy)
			}
			if reason != "superseded by v2" {
				t.Errorf("expected reason 'superseded by v2', got %q", reason)
			}
			if successorID != "payments.execute.v2" {
				t.Errorf("expected successorID 'payments.execute.v2', got %q", successorID)
			}
			return &surface.DecisionSurface{
				ID:                 "payments.execute",
				Status:             surface.SurfaceStatusDeprecated,
				DeprecationReason:  "superseded by v2",
				SuccessorSurfaceID: "payments.execute.v2",
			}, nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"deprecated_by": "ops-admin",
		"reason":        "superseded by v2",
		"successor_id":  "payments.execute.v2",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/payments.execute/deprecate", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[deprecateSurfaceResponse](t, rec)
	if resp.SurfaceID != "payments.execute" {
		t.Errorf("expected surface_id 'payments.execute', got %q", resp.SurfaceID)
	}
	if resp.Status != "deprecated" {
		t.Errorf("expected status 'deprecated', got %q", resp.Status)
	}
	if resp.DeprecationReason != "superseded by v2" {
		t.Errorf("expected deprecation_reason 'superseded by v2', got %q", resp.DeprecationReason)
	}
	if resp.SuccessorSurfaceID != "payments.execute.v2" {
		t.Errorf("expected successor_surface_id 'payments.execute.v2', got %q", resp.SuccessorSurfaceID)
	}
}

func TestDeprecateSurface_NoSuccessor(t *testing.T) {
	mock := &mockApprovalService{
		deprecateSurfaceFn: func(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error) {
			if successorID != "" {
				t.Errorf("expected empty successorID, got %q", successorID)
			}
			return &surface.DecisionSurface{
				ID:                "payments.execute",
				Status:            surface.SurfaceStatusDeprecated,
				DeprecationReason: "end of life",
			}, nil
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"deprecated_by": "ops-admin",
		"reason":        "end of life",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/payments.execute/deprecate", payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[deprecateSurfaceResponse](t, rec)
	if resp.Status != "deprecated" {
		t.Errorf("expected status 'deprecated', got %q", resp.Status)
	}
}

func TestDeprecateSurface_SurfaceNotFound(t *testing.T) {
	mock := &mockApprovalService{
		deprecateSurfaceFn: func(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error) {
			return nil, approval.ErrSurfaceNotFound
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"deprecated_by": "ops-admin",
		"reason":        "no longer needed",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/nonexistent/deprecate", payload)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "surface not found" {
		t.Errorf("expected error 'surface not found', got %q", errResp["error"])
	}
}

func TestDeprecateSurface_InvalidTransition(t *testing.T) {
	mock := &mockApprovalService{
		deprecateSurfaceFn: func(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error) {
			return nil, fmt.Errorf("%w: surface must be active to deprecate (current status: review)", approval.ErrInvalidTransition)
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"deprecated_by": "ops-admin",
		"reason":        "not needed",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-in-review/deprecate", payload)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409 for invalid transition, got %d", rec.Code)
	}
}

func TestDeprecateSurface_MissingReason(t *testing.T) {
	mock := &mockApprovalService{}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"deprecated_by": "ops-admin",
		"successor_id":  "surf-v2",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/deprecate", payload)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing reason, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "reason is required" {
		t.Errorf("expected error 'reason is required', got %q", errResp["error"])
	}
}

func TestDeprecateSurface_MissingDeprecatedBy(t *testing.T) {
	mock := &mockApprovalService{}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"reason": "no longer needed",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/deprecate", payload)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing deprecated_by, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "deprecated_by must be a valid identifier" {
		t.Errorf("expected error 'deprecated_by must be a valid identifier', got %q", errResp["error"])
	}
}

func TestDeprecateSurface_NilApprovalService(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, nil)
	payload := marshalJSON(t, map[string]any{
		"reason": "no longer needed",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/deprecate", payload)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501 for nil approval service, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "approval service not configured" {
		t.Errorf("expected 'approval service not configured' error, got %q", errResp["error"])
	}
}

func TestDeprecateSurface_InvalidSuccessorID(t *testing.T) {
	mock := &mockApprovalService{}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"deprecated_by": "ops-admin",
		"reason":        "replaced",
		"successor_id":  strings.Repeat("x", 256),
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/deprecate", payload)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid successor_id, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "successor_id must be a valid identifier" {
		t.Errorf("unexpected error: %q", errResp["error"])
	}
}

func TestDeprecateSurface_UnknownFieldsRejected(t *testing.T) {
	mock := &mockApprovalService{}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	body := []byte(`{"reason":"replaced","unknown_field":"bad"}`)

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/deprecate", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for unknown fields, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "invalid JSON payload" {
		t.Errorf("expected 'invalid JSON payload', got %q", errResp["error"])
	}
}

func TestDeprecateSurface_MethodNotAllowed(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})

	rec := performRequest(t, srv, http.MethodGet, "/v1/controlplane/surfaces/surf-1/deprecate", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Surface Action Unknown Route Tests
// ---------------------------------------------------------------------------

func TestSurfaceActions_UnknownAction(t *testing.T) {
	srv := NewServerWithServices(&mockOrchestrator{}, nil, &mockApprovalService{})
	payload := marshalJSON(t, map[string]any{"reason": "test"})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-1/retire", payload)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for unknown action, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// End-to-End Lifecycle Tests (HTTP layer)
//
// These tests verify the complete governance flow using mock services:
//   apply → review
//   approve → active
//   evaluate succeeds on active surface
//   deprecate → deprecated
//   evaluate rejects on deprecated surface (SURFACE_INACTIVE)
// ---------------------------------------------------------------------------

// TestLifecycle_ApplyApproveEvaluateDeprecate proves the end-to-end lifecycle
// path through the HTTP API: apply puts a surface into review, approve promotes
// it to active, evaluation succeeds, then deprecate moves it to deprecated and
// evaluation is rejected.
func TestLifecycle_ApplyApproveEvaluateDeprecate(t *testing.T) {
	// Track surface state across service calls to simulate a real repository.
	surfaceStatus := surface.SurfaceStatusReview

	approvalMock := &mockApprovalService{
		approveSurfaceFn: func(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
			if surfaceStatus != surface.SurfaceStatusReview {
				return nil, approval.ErrInvalidStatus
			}
			surfaceStatus = surface.SurfaceStatusActive
			return &surface.DecisionSurface{
				ID:         surfaceID,
				Status:     surface.SurfaceStatusActive,
				ApprovedBy: approver.ID,
			}, nil
		},
		deprecateSurfaceFn: func(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error) {
			if surfaceStatus != surface.SurfaceStatusActive {
				return nil, fmt.Errorf("%w: surface must be active to deprecate (current status: %s)", approval.ErrInvalidTransition, surfaceStatus)
			}
			surfaceStatus = surface.SurfaceStatusDeprecated
			return &surface.DecisionSurface{
				ID:                 surfaceID,
				Status:             surface.SurfaceStatusDeprecated,
				DeprecationReason:  reason,
				SuccessorSurfaceID: successorID,
			}, nil
		},
	}

	orchestratorMock := &mockOrchestrator{
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			// Surface is active: evaluation succeeds.
			if surfaceStatus == surface.SurfaceStatusActive {
				return decision.EvaluationResult{
					Outcome:    eval.OutcomeAccept,
					ReasonCode: eval.ReasonWithinAuthority,
					EnvelopeID: "env-001",
				}, nil
			}
			// Surface is not active: orchestrator returns SURFACE_INACTIVE.
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeReject,
				ReasonCode: eval.ReasonSurfaceInactive,
			}, nil
		},
	}

	srv := NewServerWithServices(orchestratorMock, nil, approvalMock)

	// Step 1: surface is in review (simulating post-apply state).
	// Verify that approve succeeds and transitions to active.
	approvePayload := marshalJSON(t, map[string]any{
		"submitted_by": "team-a",
		"approver_id":  "governance-board",
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/payments.execute/approve", approvePayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve step: expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	approveResp := decodeJSON[approveSurfaceResponse](t, rec)
	if approveResp.Status != "active" {
		t.Errorf("approve step: expected status 'active', got %q", approveResp.Status)
	}

	// Step 2: surface is active; evaluate should succeed.
	evalPayload := marshalJSON(t, map[string]any{
		"surface_id": "payments.execute",
		"agent_id":   "agent-001",
		"confidence": 0.9,
	})
	rec = performRequest(t, srv, http.MethodPost, "/v1/evaluate", evalPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate (active) step: expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	evalResp := decodeJSON[evaluateResponse](t, rec)
	if evalResp.Outcome != string(eval.OutcomeAccept) {
		t.Errorf("evaluate (active) step: expected outcome 'accept', got %q", evalResp.Outcome)
	}

	// Step 3: deprecate the surface.
	deprecatePayload := marshalJSON(t, map[string]any{
		"deprecated_by": "governance-board",
		"reason":        "superseded by payments.execute.v2",
		"successor_id":  "payments.execute.v2",
	})
	rec = performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/payments.execute/deprecate", deprecatePayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("deprecate step: expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	deprecateResp := decodeJSON[deprecateSurfaceResponse](t, rec)
	if deprecateResp.Status != "deprecated" {
		t.Errorf("deprecate step: expected status 'deprecated', got %q", deprecateResp.Status)
	}

	// Step 4: surface is deprecated; evaluate should reject with SURFACE_INACTIVE.
	rec = performRequest(t, srv, http.MethodPost, "/v1/evaluate", evalPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate (deprecated) step: expected status 200 (domain reject), got %d: %s", rec.Code, rec.Body.String())
	}
	evalResp = decodeJSON[evaluateResponse](t, rec)
	if evalResp.Outcome != string(eval.OutcomeReject) {
		t.Errorf("evaluate (deprecated) step: expected outcome 'reject', got %q", evalResp.Outcome)
	}
	if evalResp.Reason != string(eval.ReasonSurfaceInactive) {
		t.Errorf("evaluate (deprecated) step: expected reason %q, got %q", eval.ReasonSurfaceInactive, evalResp.Reason)
	}
}

// TestLifecycle_ApproveRejectsAlreadyActive verifies that attempting to approve
// a surface that is already active returns 409 Conflict.
func TestLifecycle_ApproveRejectsAlreadyActive(t *testing.T) {
	mock := &mockApprovalService{
		approveSurfaceFn: func(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
			return nil, approval.ErrInvalidStatus
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"approver_id": "approver-1",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-active/approve", payload)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409 for already-active surface, got %d", rec.Code)
	}
}

// TestLifecycle_DeprecateRejectsNonActive verifies that attempting to deprecate
// a surface that is not active returns 409 Conflict.
func TestLifecycle_DeprecateRejectsNonActive(t *testing.T) {
	mock := &mockApprovalService{
		deprecateSurfaceFn: func(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error) {
			return nil, fmt.Errorf("%w: surface must be active to deprecate (current status: review)", approval.ErrInvalidTransition)
		},
	}

	srv := NewServerWithServices(&mockOrchestrator{}, nil, mock)
	payload := marshalJSON(t, map[string]any{
		"deprecated_by": "ops-admin",
		"reason":        "should not work",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/surf-in-review/deprecate", payload)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409 for non-active surface deprecation, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// ListEnvelopes — state filter tests
// ---------------------------------------------------------------------------

func TestListEnvelopes_StateFilter_Closed(t *testing.T) {
	closedEnv := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-closed"},
		State:    envelope.EnvelopeStateClosed,
	}

	mock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			if state != envelope.EnvelopeStateClosed {
				t.Errorf("expected state filter %q, got %q", envelope.EnvelopeStateClosed, state)
			}
			return []*envelope.Envelope{closedEnv}, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes?state=closed", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[[]*envelope.Envelope](t, rec)
	if len(resp) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(resp))
	}
	if resp[0].Identity.ID != "env-closed" {
		t.Errorf("expected envelope id 'env-closed', got %q", resp[0].Identity.ID)
	}
}

func TestListEnvelopes_StateFilter_AwaitingReview(t *testing.T) {
	escalatedEnv := &envelope.Envelope{
		Identity: envelope.Identity{ID: "env-pending"},
		State:    envelope.EnvelopeStateAwaitingReview,
	}

	mock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			if state != envelope.EnvelopeStateAwaitingReview {
				t.Errorf("expected state filter %q, got %q", envelope.EnvelopeStateAwaitingReview, state)
			}
			return []*envelope.Envelope{escalatedEnv}, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes?state=awaiting_review", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[[]*envelope.Envelope](t, rec)
	if len(resp) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(resp))
	}
	if resp[0].State != envelope.EnvelopeStateAwaitingReview {
		t.Errorf("expected state %q, got %q", envelope.EnvelopeStateAwaitingReview, resp[0].State)
	}
}

func TestListEnvelopes_StateFilter_Invalid(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodGet, "/v1/envelopes?state=invalid_state", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid state, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], "invalid state filter") {
		t.Errorf("expected 'invalid state filter' in error, got %q", errResp["error"])
	}
}

// ---------------------------------------------------------------------------
// ListEscalations endpoint tests
// ---------------------------------------------------------------------------

func TestListEscalations_Success(t *testing.T) {
	escalatedEnv := &envelope.Envelope{
		Identity:   envelope.Identity{ID: "env-escalated"},
		State:      envelope.EnvelopeStateAwaitingReview,
		Evaluation: envelope.Evaluation{Outcome: "escalate"},
	}

	mock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			if state != envelope.EnvelopeStateAwaitingReview {
				t.Errorf("escalations endpoint must filter by awaiting_review, got %q", state)
			}
			return []*envelope.Envelope{escalatedEnv}, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalations", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[[]*envelope.Envelope](t, rec)
	if len(resp) != 1 {
		t.Fatalf("expected 1 escalation, got %d", len(resp))
	}
	if resp[0].Identity.ID != "env-escalated" {
		t.Errorf("expected envelope id 'env-escalated', got %q", resp[0].Identity.ID)
	}
	if resp[0].State != envelope.EnvelopeStateAwaitingReview {
		t.Errorf("expected state awaiting_review, got %q", resp[0].State)
	}
}

func TestListEscalations_EmptyQueue(t *testing.T) {
	mock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			return []*envelope.Envelope{}, nil
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalations", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for empty queue, got %d", rec.Code)
	}

	resp := decodeJSON[[]*envelope.Envelope](t, rec)
	if len(resp) != 0 {
		t.Errorf("expected 0 escalations, got %d", len(resp))
	}
}

func TestListEscalations_MethodNotAllowed(t *testing.T) {
	srv := NewServer(&mockOrchestrator{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/escalations", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("expected Allow header 'GET', got %q", allow)
	}
}

func TestListEscalations_NilOrchestrator(t *testing.T) {
	srv := NewServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalations", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for nil orchestrator, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "orchestrator not configured" {
		t.Errorf("expected 'orchestrator not configured' error, got %q", errResp["error"])
	}
}

func TestListEscalations_OrchestratorError(t *testing.T) {
	mock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			return nil, fmt.Errorf("database unavailable")
		},
	}

	srv := NewServer(mock)
	rec := performRequest(t, srv, http.MethodGet, "/v1/escalations", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// End-to-end escalation workflow test
//
// Exercises the full operator workflow against the mock orchestrator:
//   1. evaluate produces escalation (Escalate outcome, AWAITING_REVIEW state)
//   2. pending queue shows the envelope (GET /v1/escalations)
//   3. envelope can be retrieved by ID (GET /v1/envelopes/{id})
//   4. review resolves the escalation (POST /v1/reviews)
//   5. envelope leaves pending state (GET /v1/escalations is now empty)
// ---------------------------------------------------------------------------

func TestEscalationWorkflow_EndToEnd(t *testing.T) {
	const envelopeID = "env-workflow-test"

	// Simulate state transitions across the workflow steps.
	// The envelope starts in awaiting_review and moves to closed after review.
	currentState := envelope.EnvelopeStateAwaitingReview

	escalatedEnv := &envelope.Envelope{
		Identity: envelope.Identity{ID: envelopeID},
		State:    envelope.EnvelopeStateAwaitingReview,
		Evaluation: envelope.Evaluation{
			Outcome:    "escalate",
			ReasonCode: "CONFIDENCE_BELOW_THRESHOLD",
		},
	}
	closedEnv := &envelope.Envelope{
		Identity: envelope.Identity{ID: envelopeID},
		State:    envelope.EnvelopeStateClosed,
		Review: &envelope.EscalationReview{
			Decision:   envelope.ReviewDecisionApproved,
			ReviewerID: "reviewer-ops",
		},
		Evaluation: envelope.Evaluation{
			Outcome:    "escalate",
			ReasonCode: "CONFIDENCE_BELOW_THRESHOLD",
		},
	}

	mock := &mockOrchestrator{
		// Step 1: evaluate returns escalation.
		evaluateFn: func(ctx context.Context, req eval.DecisionRequest, raw json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{
				Outcome:     "escalate",
				ReasonCode:  "CONFIDENCE_BELOW_THRESHOLD",
				EnvelopeID:  envelopeID,
				State:       envelope.EnvelopeStateAwaitingReview,
				Explanation: "Confidence 0.50 is below required threshold 0.80.",
			}, nil
		},
		// Steps 2 & 5: list escalations reflects current state.
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			if state != envelope.EnvelopeStateAwaitingReview {
				t.Errorf("escalations endpoint must filter awaiting_review, got %q", state)
			}
			if currentState == envelope.EnvelopeStateAwaitingReview {
				return []*envelope.Envelope{escalatedEnv}, nil
			}
			return []*envelope.Envelope{}, nil
		},
		// Step 3: retrieve envelope by ID.
		getEnvelopeByIDFn: func(ctx context.Context, id string) (*envelope.Envelope, error) {
			if id != envelopeID {
				t.Errorf("expected envelope id %q, got %q", envelopeID, id)
			}
			return escalatedEnv, nil
		},
		// Step 4: resolve escalation closes the envelope.
		resolveEscalationFn: func(ctx context.Context, resolution decision.EscalationResolution) (*envelope.Envelope, error) {
			if resolution.EnvelopeID != envelopeID {
				t.Errorf("expected envelope id %q, got %q", envelopeID, resolution.EnvelopeID)
			}
			if resolution.Decision != envelope.ReviewDecisionApproved {
				t.Errorf("expected APPROVED decision, got %q", resolution.Decision)
			}
			if resolution.ReviewerID != "reviewer-ops" {
				t.Errorf("expected reviewer 'reviewer-ops', got %q", resolution.ReviewerID)
			}
			currentState = envelope.EnvelopeStateClosed
			return closedEnv, nil
		},
	}

	srv := NewServer(mock)

	// Step 1: evaluate — expect escalation outcome.
	evalPayload := marshalJSON(t, map[string]any{
		"surface_id": "surf-payments",
		"agent_id":   "agent-proc",
		"confidence": 0.50,
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", evalPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 1 evaluate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	evalResp := decodeJSON[evaluateResponse](t, rec)
	if evalResp.Outcome != "escalate" {
		t.Errorf("step 1: expected outcome 'escalate', got %q", evalResp.Outcome)
	}
	if evalResp.EnvelopeID != envelopeID {
		t.Errorf("step 1: expected envelope_id %q, got %q", envelopeID, evalResp.EnvelopeID)
	}

	// Step 2: pending queue shows the escalated envelope.
	rec = performRequest(t, srv, http.MethodGet, "/v1/escalations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 2 list escalations: expected 200, got %d", rec.Code)
	}
	pending := decodeJSON[[]*envelope.Envelope](t, rec)
	if len(pending) != 1 {
		t.Fatalf("step 2: expected 1 pending escalation, got %d", len(pending))
	}
	if pending[0].Identity.ID != envelopeID {
		t.Errorf("step 2: expected envelope id %q in pending queue, got %q", envelopeID, pending[0].Identity.ID)
	}

	// Step 3: retrieve the envelope by ID.
	rec = performRequest(t, srv, http.MethodGet, "/v1/envelopes/"+envelopeID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 3 get envelope: expected 200, got %d", rec.Code)
	}
	fetched := decodeJSON[envelope.Envelope](t, rec)
	if fetched.State != envelope.EnvelopeStateAwaitingReview {
		t.Errorf("step 3: expected state awaiting_review, got %q", fetched.State)
	}

	// Step 4: submit review to resolve the escalation.
	reviewPayload := marshalJSON(t, map[string]any{
		"envelope_id": envelopeID,
		"decision":    "approve",
		"reviewer":    "reviewer-ops",
		"notes":       "reviewed and approved by ops team",
	})
	rec = performRequest(t, srv, http.MethodPost, "/v1/reviews", reviewPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 4 review: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	reviewResp := decodeJSON[reviewResponse](t, rec)
	if reviewResp.Status != "resolved" {
		t.Errorf("step 4: expected status 'resolved', got %q", reviewResp.Status)
	}
	if reviewResp.EnvelopeID != envelopeID {
		t.Errorf("step 4: expected envelope_id %q, got %q", envelopeID, reviewResp.EnvelopeID)
	}

	// Step 5: pending queue is now empty.
	rec = performRequest(t, srv, http.MethodGet, "/v1/escalations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 5 list escalations: expected 200, got %d", rec.Code)
	}
	remaining := decodeJSON[[]*envelope.Envelope](t, rec)
	if len(remaining) != 0 {
		t.Errorf("step 5: expected empty escalation queue after resolution, got %d items", len(remaining))
	}
}

// ---------------------------------------------------------------------------
// Mock Introspection Service
// ---------------------------------------------------------------------------

type mockIntrospectionService struct {
	getSurfaceFn            func(ctx context.Context, id string) (*surface.DecisionSurface, error)
	listSurfaceVersionsFn   func(ctx context.Context, id string) ([]*surface.DecisionSurface, error)
	getSurfaceImpactFn      func(ctx context.Context, id string) (*SurfaceImpactResult, error)
	listProfilesBySurfaceFn func(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error)
	getProfileFn            func(ctx context.Context, id string) (*authority.AuthorityProfile, error)
	listProfileVersionsFn   func(ctx context.Context, id string) ([]*authority.AuthorityProfile, error)
	getAgentFn              func(ctx context.Context, id string) (*agent.Agent, error)
	getGrantFn              func(ctx context.Context, id string) (*authority.AuthorityGrant, error)
	listGrantsByAgentFn     func(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error)
	listGrantsByProfileFn   func(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error)
	getSurfaceRecoveryFn    func(ctx context.Context, id string) (*SurfaceRecoveryResult, error)
	getProfileRecoveryFn    func(ctx context.Context, id string) (*ProfileRecoveryResult, error)
}

func (m *mockIntrospectionService) GetSurface(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	if m.getSurfaceFn != nil {
		return m.getSurfaceFn(ctx, id)
	}
	return nil, fmt.Errorf("getSurface not implemented")
}

func (m *mockIntrospectionService) ListSurfaceVersions(ctx context.Context, id string) ([]*surface.DecisionSurface, error) {
	if m.listSurfaceVersionsFn != nil {
		return m.listSurfaceVersionsFn(ctx, id)
	}
	return nil, fmt.Errorf("listSurfaceVersions not implemented")
}

func (m *mockIntrospectionService) GetSurfaceImpact(ctx context.Context, id string) (*SurfaceImpactResult, error) {
	if m.getSurfaceImpactFn != nil {
		return m.getSurfaceImpactFn(ctx, id)
	}
	return nil, fmt.Errorf("getSurfaceImpact not implemented")
}

func (m *mockIntrospectionService) ListProfilesBySurface(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	if m.listProfilesBySurfaceFn != nil {
		return m.listProfilesBySurfaceFn(ctx, surfaceID)
	}
	return nil, fmt.Errorf("listProfilesBySurface not implemented")
}

func (m *mockIntrospectionService) GetProfile(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
	if m.getProfileFn != nil {
		return m.getProfileFn(ctx, id)
	}
	return nil, fmt.Errorf("getProfile not implemented")
}

func (m *mockIntrospectionService) ListProfileVersions(ctx context.Context, id string) ([]*authority.AuthorityProfile, error) {
	if m.listProfileVersionsFn != nil {
		return m.listProfileVersionsFn(ctx, id)
	}
	return nil, fmt.Errorf("listProfileVersions not implemented")
}

func (m *mockIntrospectionService) GetAgent(ctx context.Context, id string) (*agent.Agent, error) {
	if m.getAgentFn != nil {
		return m.getAgentFn(ctx, id)
	}
	return nil, fmt.Errorf("getAgent not implemented")
}

func (m *mockIntrospectionService) GetGrant(ctx context.Context, id string) (*authority.AuthorityGrant, error) {
	if m.getGrantFn != nil {
		return m.getGrantFn(ctx, id)
	}
	return nil, fmt.Errorf("getGrant not implemented")
}

func (m *mockIntrospectionService) ListGrantsByAgent(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
	if m.listGrantsByAgentFn != nil {
		return m.listGrantsByAgentFn(ctx, agentID)
	}
	return nil, fmt.Errorf("listGrantsByAgent not implemented")
}

func (m *mockIntrospectionService) ListGrantsByProfile(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
	if m.listGrantsByProfileFn != nil {
		return m.listGrantsByProfileFn(ctx, profileID)
	}
	return nil, fmt.Errorf("listGrantsByProfile not implemented")
}

func (m *mockIntrospectionService) GetSurfaceRecovery(ctx context.Context, id string) (*SurfaceRecoveryResult, error) {
	if m.getSurfaceRecoveryFn != nil {
		return m.getSurfaceRecoveryFn(ctx, id)
	}
	return nil, fmt.Errorf("getSurfaceRecovery not implemented")
}

func (m *mockIntrospectionService) GetProfileRecovery(ctx context.Context, id string) (*ProfileRecoveryResult, error) {
	if m.getProfileRecoveryFn != nil {
		return m.getProfileRecoveryFn(ctx, id)
	}
	return nil, fmt.Errorf("getProfileRecovery not implemented")
}

// newIntrospectionServer constructs a Server with only the introspection service wired.
func newIntrospectionServer(svc introspectionService) *Server {
	return NewServerWithAllServices(&mockOrchestrator{}, nil, nil, svc)
}

// ---------------------------------------------------------------------------
// GET /v1/surfaces/{id}
// ---------------------------------------------------------------------------

func TestGetSurface_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockIntrospectionService{
		getSurfaceFn: func(ctx context.Context, id string) (*surface.DecisionSurface, error) {
			if id != "payments.execute" {
				t.Errorf("expected id 'payments.execute', got %q", id)
			}
			return &surface.DecisionSurface{
				ID:             "payments.execute",
				Name:           "Payment Execution",
				Status:         surface.SurfaceStatusActive,
				Version:        2,
				EffectiveFrom:  now,
				ApprovedBy:     "approver-1",
				ApprovedAt:     &now,
				Domain:         "payments",
				BusinessOwner:  "finance-team",
				TechnicalOwner: "platform-team",
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/payments.execute", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[surfaceResponse](t, rec)
	if resp.ID != "payments.execute" {
		t.Errorf("expected id 'payments.execute', got %q", resp.ID)
	}
	if resp.Status != "active" {
		t.Errorf("expected status 'active', got %q", resp.Status)
	}
	if resp.Version != 2 {
		t.Errorf("expected version 2, got %d", resp.Version)
	}
	if resp.ApprovedBy != "approver-1" {
		t.Errorf("expected approved_by 'approver-1', got %q", resp.ApprovedBy)
	}
	if resp.Domain != "payments" {
		t.Errorf("expected domain 'payments', got %q", resp.Domain)
	}
}

func TestGetSurface_NotFound(t *testing.T) {
	svc := &mockIntrospectionService{
		getSurfaceFn: func(ctx context.Context, id string) (*surface.DecisionSurface, error) {
			return nil, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/unknown-surface", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], "not found") {
		t.Errorf("expected 'not found' error, got %q", errResp["error"])
	}
}

func TestGetSurface_InvalidID(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/bad/id/with/slashes", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetSurface_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodPost, "/v1/surfaces/payments.execute", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestGetSurface_NilIntrospection(t *testing.T) {
	srv := newIntrospectionServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/payments.execute", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /v1/surfaces/{id}/versions
// ---------------------------------------------------------------------------

func TestGetSurfaceVersions_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockIntrospectionService{
		listSurfaceVersionsFn: func(ctx context.Context, id string) ([]*surface.DecisionSurface, error) {
			if id != "payments.execute" {
				t.Errorf("expected id 'payments.execute', got %q", id)
			}
			return []*surface.DecisionSurface{
				{
					ID:            "payments.execute",
					Version:       1,
					Status:        surface.SurfaceStatusActive,
					EffectiveFrom: now,
					CreatedAt:     now,
					UpdatedAt:     now,
				},
				{
					ID:            "payments.execute",
					Version:       2,
					Status:        surface.SurfaceStatusDeprecated,
					EffectiveFrom: now,
					ApprovedBy:    "approver-1",
					ApprovedAt:    &now,
					CreatedAt:     now,
					UpdatedAt:     now,
				},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/payments.execute/versions", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	versions := decodeJSON[[]surfaceVersionResponse](t, rec)
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != 1 {
		t.Errorf("expected first version 1, got %d", versions[0].Version)
	}
	if versions[1].Version != 2 {
		t.Errorf("expected second version 2, got %d", versions[1].Version)
	}
	if versions[1].ApprovedBy != "approver-1" {
		t.Errorf("expected approved_by 'approver-1', got %q", versions[1].ApprovedBy)
	}
}

func TestGetSurfaceVersions_NotFound(t *testing.T) {
	svc := &mockIntrospectionService{
		listSurfaceVersionsFn: func(ctx context.Context, id string) ([]*surface.DecisionSurface, error) {
			return []*surface.DecisionSurface{}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/unknown-surface/versions", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetSurfaceVersions_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodDelete, "/v1/surfaces/payments.execute/versions", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestGetSurfaceVersions_NilIntrospection(t *testing.T) {
	srv := newIntrospectionServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/payments.execute/versions", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /v1/profiles?surface_id={id}
// ---------------------------------------------------------------------------

func TestListProfiles_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockIntrospectionService{
		listProfilesBySurfaceFn: func(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
			if surfaceID != "payments.execute" {
				t.Errorf("expected surfaceID 'payments.execute', got %q", surfaceID)
			}
			return []*authority.AuthorityProfile{
				{
					ID:                  "profile-1",
					Version:             1,
					SurfaceID:           "payments.execute",
					Name:                "Standard Payments",
					Status:              authority.ProfileStatusActive,
					EffectiveDate:       now,
					ConfidenceThreshold: 0.85,
					EscalationMode:      authority.EscalationModeAuto,
					FailMode:            authority.FailModeClosed,
					CreatedAt:           now,
					UpdatedAt:           now,
				},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles?surface_id=payments.execute", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	profiles := decodeJSON[[]profileResponse](t, rec)
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].ID != "profile-1" {
		t.Errorf("expected id 'profile-1', got %q", profiles[0].ID)
	}
	if profiles[0].SurfaceID != "payments.execute" {
		t.Errorf("expected surface_id 'payments.execute', got %q", profiles[0].SurfaceID)
	}
	if profiles[0].ConfidenceThreshold != 0.85 {
		t.Errorf("expected confidence_threshold 0.85, got %f", profiles[0].ConfidenceThreshold)
	}
	if profiles[0].Status != "active" {
		t.Errorf("expected status 'active', got %q", profiles[0].Status)
	}
}

func TestListProfiles_EmptyResult(t *testing.T) {
	svc := &mockIntrospectionService{
		listProfilesBySurfaceFn: func(ctx context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
			return []*authority.AuthorityProfile{}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles?surface_id=payments.execute", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	profiles := decodeJSON[[]profileResponse](t, rec)
	if len(profiles) != 0 {
		t.Errorf("expected empty array, got %d items", len(profiles))
	}
}

func TestListProfiles_MissingSurfaceID(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], "surface_id") {
		t.Errorf("expected error mentioning 'surface_id', got %q", errResp["error"])
	}
}

func TestListProfiles_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodPost, "/v1/profiles?surface_id=payments.execute", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestListProfiles_NilIntrospection(t *testing.T) {
	srv := newIntrospectionServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles?surface_id=payments.execute", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// E2E: Surface Lifecycle Visibility
// Apply bundle → approve surface → introspect surface, versions, profiles
// ---------------------------------------------------------------------------

func TestE2E_SurfaceLifecycleVisibility(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	surfaceID := "payments.execute"

	// Surface starts in review after apply, transitions to active after approval.
	activeSurface := &surface.DecisionSurface{
		ID:             surfaceID,
		Name:           "Payment Execution",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		EffectiveFrom:  now,
		ApprovedBy:     "ops-lead",
		ApprovedAt:     &now,
		Domain:         "payments",
		BusinessOwner:  "finance-team",
		TechnicalOwner: "platform-team",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	introspectionSvc := &mockIntrospectionService{
		getSurfaceFn: func(ctx context.Context, id string) (*surface.DecisionSurface, error) {
			if id != surfaceID {
				return nil, nil
			}
			return activeSurface, nil
		},
		listSurfaceVersionsFn: func(ctx context.Context, id string) ([]*surface.DecisionSurface, error) {
			if id != surfaceID {
				return []*surface.DecisionSurface{}, nil
			}
			return []*surface.DecisionSurface{activeSurface}, nil
		},
		listProfilesBySurfaceFn: func(ctx context.Context, surfaceIDParam string) ([]*authority.AuthorityProfile, error) {
			if surfaceIDParam != surfaceID {
				return []*authority.AuthorityProfile{}, nil
			}
			return []*authority.AuthorityProfile{
				{
					ID:                  "profile-payments-standard",
					Version:             1,
					SurfaceID:           surfaceID,
					Name:                "Standard Payments Authority",
					Status:              authority.ProfileStatusActive,
					EffectiveDate:       now,
					ConfidenceThreshold: 0.80,
					EscalationMode:      authority.EscalationModeAuto,
					FailMode:            authority.FailModeClosed,
					CreatedAt:           now,
					UpdatedAt:           now,
				},
			}, nil
		},
	}

	approvalSvc := &mockApprovalService{
		approveSurfaceFn: func(ctx context.Context, sid string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
			return activeSurface, nil
		},
	}

	cpSvc := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte, actor string) (*cpTypes.ApplyResult, error) {
			result := &cpTypes.ApplyResult{}
			result.AddCreated("surface", surfaceID)
			return result, nil
		},
	}

	srv := NewServerWithAllServices(&mockOrchestrator{}, cpSvc, approvalSvc, introspectionSvc)

	// Step 1: apply the bundle — surface enters review.
	applyPayload := []byte(`
apiVersion: midas/v1
kind: DecisionSurface
metadata:
  id: payments.execute
  name: Payment Execution
spec:
  domain: payments
`)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/apply", applyPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 1 apply: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 2: approve the surface.
	approvePayload := marshalJSON(t, map[string]any{
		"submitted_by": "submitter-1",
		"approver_id":  "ops-lead",
	})
	rec = performRequest(t, srv, http.MethodPost, "/v1/controlplane/surfaces/"+surfaceID+"/approve", approvePayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 2 approve: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 3: GET /v1/surfaces/{id} — latest version is active.
	rec = performRequest(t, srv, http.MethodGet, "/v1/surfaces/"+surfaceID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 3 get surface: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	surf := decodeJSON[surfaceResponse](t, rec)
	if surf.ID != surfaceID {
		t.Errorf("step 3: expected id %q, got %q", surfaceID, surf.ID)
	}
	if surf.Status != "active" {
		t.Errorf("step 3: expected status 'active', got %q", surf.Status)
	}
	if surf.ApprovedBy != "ops-lead" {
		t.Errorf("step 3: expected approved_by 'ops-lead', got %q", surf.ApprovedBy)
	}
	if surf.Domain != "payments" {
		t.Errorf("step 3: expected domain 'payments', got %q", surf.Domain)
	}

	// Step 4: GET /v1/surfaces/{id}/versions — version history.
	rec = performRequest(t, srv, http.MethodGet, "/v1/surfaces/"+surfaceID+"/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 4 list versions: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	versions := decodeJSON[[]surfaceVersionResponse](t, rec)
	if len(versions) != 1 {
		t.Fatalf("step 4: expected 1 version, got %d", len(versions))
	}
	if versions[0].Version != 1 {
		t.Errorf("step 4: expected version 1, got %d", versions[0].Version)
	}

	// Step 5: GET /v1/profiles?surface_id={id} — attached profiles.
	rec = performRequest(t, srv, http.MethodGet, "/v1/profiles?surface_id="+surfaceID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 5 list profiles: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	profiles := decodeJSON[[]profileResponse](t, rec)
	if len(profiles) != 1 {
		t.Fatalf("step 5: expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].SurfaceID != surfaceID {
		t.Errorf("step 5: expected surface_id %q, got %q", surfaceID, profiles[0].SurfaceID)
	}
	if profiles[0].ConfidenceThreshold != 0.80 {
		t.Errorf("step 5: expected confidence_threshold 0.80, got %f", profiles[0].ConfidenceThreshold)
	}
}

// ---------------------------------------------------------------------------
// GET /v1/profiles/{id}
// ---------------------------------------------------------------------------

func TestGetProfile_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockIntrospectionService{
		getProfileFn: func(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
			if id != "profile-payments-standard" {
				t.Errorf("expected id 'profile-payments-standard', got %q", id)
			}
			return &authority.AuthorityProfile{
				ID:                  "profile-payments-standard",
				Version:             1,
				SurfaceID:           "payments.execute",
				Name:                "Standard Payments Authority",
				Status:              authority.ProfileStatusActive,
				EffectiveDate:       now,
				ConfidenceThreshold: 0.80,
				EscalationMode:      authority.EscalationModeAuto,
				FailMode:            authority.FailModeClosed,
				CreatedAt:           now,
				UpdatedAt:           now,
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/profile-payments-standard", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[profileResponse](t, rec)
	if resp.ID != "profile-payments-standard" {
		t.Errorf("expected id 'profile-payments-standard', got %q", resp.ID)
	}
	if resp.SurfaceID != "payments.execute" {
		t.Errorf("expected surface_id 'payments.execute', got %q", resp.SurfaceID)
	}
	if resp.Status != "active" {
		t.Errorf("expected status 'active', got %q", resp.Status)
	}
	if resp.ConfidenceThreshold != 0.80 {
		t.Errorf("expected confidence_threshold 0.80, got %f", resp.ConfidenceThreshold)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	svc := &mockIntrospectionService{
		getProfileFn: func(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/no-such-profile", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "profile not found" {
		t.Errorf("expected 'profile not found', got %q", errResp["error"])
	}
}

func TestGetProfile_MissingID(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	// /v1/profiles/ with trailing slash and empty ID segment hits handleGetProfile.
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetProfile_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodPost, "/v1/profiles/profile-payments-standard", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestGetProfile_NilIntrospection(t *testing.T) {
	srv := newIntrospectionServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/profile-payments-standard", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /v1/profiles/{id}/versions
// ---------------------------------------------------------------------------

func TestGetProfileVersions_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockIntrospectionService{
		listProfileVersionsFn: func(ctx context.Context, id string) ([]*authority.AuthorityProfile, error) {
			if id != "profile-payments-standard" {
				t.Errorf("expected id 'profile-payments-standard', got %q", id)
			}
			return []*authority.AuthorityProfile{
				{
					ID:            "profile-payments-standard",
					Version:       2,
					SurfaceID:     "payments.execute",
					Name:          "Standard Payments Authority",
					Status:        authority.ProfileStatusActive,
					EffectiveDate: now,
					CreatedAt:     now,
					UpdatedAt:     now,
				},
				{
					ID:            "profile-payments-standard",
					Version:       1,
					SurfaceID:     "payments.execute",
					Name:          "Standard Payments Authority",
					Status:        authority.ProfileStatusActive,
					EffectiveDate: now,
					CreatedAt:     now,
					UpdatedAt:     now,
				},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/profile-payments-standard/versions", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	versions := decodeJSON[[]profileResponse](t, rec)
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	// Service returns descending order (latest first); handler preserves it.
	if versions[0].Version != 2 {
		t.Errorf("expected first version 2 (latest), got %d", versions[0].Version)
	}
	if versions[1].Version != 1 {
		t.Errorf("expected second version 1, got %d", versions[1].Version)
	}
	if versions[0].SurfaceID != "payments.execute" {
		t.Errorf("expected surface_id 'payments.execute', got %q", versions[0].SurfaceID)
	}
}

func TestGetProfileVersions_NotFound(t *testing.T) {
	svc := &mockIntrospectionService{
		listProfileVersionsFn: func(ctx context.Context, id string) ([]*authority.AuthorityProfile, error) {
			return []*authority.AuthorityProfile{}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/no-such-profile/versions", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetProfileVersions_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodDelete, "/v1/profiles/profile-payments-standard/versions", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestGetProfileVersions_NilIntrospection(t *testing.T) {
	srv := newIntrospectionServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/profile-payments-standard/versions", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetProfileVersions_InvalidID(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	longID := strings.Repeat("x", 256) // exceeds maxIdentifierLength
	rec := performRequest(t, srv, http.MethodGet, "/v1/profiles/"+longID+"/versions", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /v1/agents/{id}
// ---------------------------------------------------------------------------

func TestGetAgent_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockIntrospectionService{
		getAgentFn: func(ctx context.Context, id string) (*agent.Agent, error) {
			if id != "agent-payments-bot" {
				t.Errorf("expected id 'agent-payments-bot', got %q", id)
			}
			return &agent.Agent{
				ID:               "agent-payments-bot",
				Name:             "Payments Bot",
				Type:             agent.AgentTypeAI,
				Owner:            "payments-team",
				ModelVersion:     "gpt-4o",
				OperationalState: agent.OperationalStateActive,
				CreatedAt:        now,
				UpdatedAt:        now,
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/agents/agent-payments-bot", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[agentResponse](t, rec)
	if resp.ID != "agent-payments-bot" {
		t.Errorf("expected id 'agent-payments-bot', got %q", resp.ID)
	}
	if resp.Name != "Payments Bot" {
		t.Errorf("expected name 'Payments Bot', got %q", resp.Name)
	}
	if resp.Type != "ai" {
		t.Errorf("expected type 'ai', got %q", resp.Type)
	}
	if resp.Owner != "payments-team" {
		t.Errorf("expected owner 'payments-team', got %q", resp.Owner)
	}
	if resp.OperationalState != "active" {
		t.Errorf("expected operational_state 'active', got %q", resp.OperationalState)
	}
	if resp.ModelVersion != "gpt-4o" {
		t.Errorf("expected model_version 'gpt-4o', got %q", resp.ModelVersion)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	svc := &mockIntrospectionService{
		getAgentFn: func(ctx context.Context, id string) (*agent.Agent, error) {
			return nil, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/agents/no-such-agent", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	errResp := decodeError(t, rec)
	if errResp["error"] != "agent not found" {
		t.Errorf("expected 'agent not found', got %q", errResp["error"])
	}
}

func TestGetAgent_MissingID(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/agents/", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAgent_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodPost, "/v1/agents/agent-payments-bot", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestGetAgent_NilIntrospection(t *testing.T) {
	srv := newIntrospectionServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/agents/agent-payments-bot", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /v1/grants?agent_id={id} and GET /v1/grants?profile_id={id}
// ---------------------------------------------------------------------------

func TestListGrants_ByAgent_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockIntrospectionService{
		listGrantsByAgentFn: func(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
			if agentID != "agent-payments-bot" {
				t.Errorf("expected agentID 'agent-payments-bot', got %q", agentID)
			}
			return []*authority.AuthorityGrant{
				{
					ID:            "grant-001",
					AgentID:       "agent-payments-bot",
					ProfileID:     "profile-payments-standard",
					Status:        authority.GrantStatusActive,
					GrantedBy:     "ops-lead",
					EffectiveDate: now,
					CreatedAt:     now,
					UpdatedAt:     now,
				},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/grants?agent_id=agent-payments-bot", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	grants := decodeJSON[[]grantResponse](t, rec)
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(grants))
	}
	if grants[0].ID != "grant-001" {
		t.Errorf("expected id 'grant-001', got %q", grants[0].ID)
	}
	if grants[0].AgentID != "agent-payments-bot" {
		t.Errorf("expected agent_id 'agent-payments-bot', got %q", grants[0].AgentID)
	}
	if grants[0].ProfileID != "profile-payments-standard" {
		t.Errorf("expected profile_id 'profile-payments-standard', got %q", grants[0].ProfileID)
	}
	if grants[0].Status != "active" {
		t.Errorf("expected status 'active', got %q", grants[0].Status)
	}
	if grants[0].GrantedBy != "ops-lead" {
		t.Errorf("expected granted_by 'ops-lead', got %q", grants[0].GrantedBy)
	}
}

func TestListGrants_ByProfile_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &mockIntrospectionService{
		listGrantsByProfileFn: func(ctx context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
			if profileID != "profile-payments-standard" {
				t.Errorf("expected profileID 'profile-payments-standard', got %q", profileID)
			}
			return []*authority.AuthorityGrant{
				{
					ID:            "grant-001",
					AgentID:       "agent-payments-bot",
					ProfileID:     "profile-payments-standard",
					Status:        authority.GrantStatusActive,
					GrantedBy:     "ops-lead",
					EffectiveDate: now,
					CreatedAt:     now,
					UpdatedAt:     now,
				},
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/grants?profile_id=profile-payments-standard", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	grants := decodeJSON[[]grantResponse](t, rec)
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(grants))
	}
	if grants[0].ProfileID != "profile-payments-standard" {
		t.Errorf("expected profile_id 'profile-payments-standard', got %q", grants[0].ProfileID)
	}
}

func TestListGrants_EmptyResult(t *testing.T) {
	svc := &mockIntrospectionService{
		listGrantsByAgentFn: func(ctx context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
			return []*authority.AuthorityGrant{}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/grants?agent_id=agent-unknown", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	grants := decodeJSON[[]grantResponse](t, rec)
	if len(grants) != 0 {
		t.Errorf("expected 0 grants, got %d", len(grants))
	}
}

func TestListGrants_MissingParam(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/grants", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], "agent_id or profile_id") {
		t.Errorf("unexpected error: %q", errResp["error"])
	}
}

func TestListGrants_BothParams(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/grants?agent_id=a1&profile_id=p1", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListGrants_MethodNotAllowed(t *testing.T) {
	svc := &mockIntrospectionService{}
	srv := newIntrospectionServer(svc)

	rec := performRequest(t, srv, http.MethodPost, "/v1/grants?agent_id=agent-1", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestListGrants_NilIntrospection(t *testing.T) {
	srv := newIntrospectionServer(nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/grants?agent_id=agent-1", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// E2E: Agent, Profile, and Grant Visibility
// Wire mocks for all new endpoints and confirm agent, profile, grants readable.
// ---------------------------------------------------------------------------

func TestE2E_AgentProfileGrantVisibility(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	surfaceID := "payments.execute"
	profileID := "profile-payments-standard"
	agentID := "agent-payments-bot"
	grantID := "grant-001"

	activeProfile := &authority.AuthorityProfile{
		ID:                  profileID,
		Version:             1,
		SurfaceID:           surfaceID,
		Name:                "Standard Payments Authority",
		Status:              authority.ProfileStatusActive,
		EffectiveDate:       now,
		ConfidenceThreshold: 0.80,
		EscalationMode:      authority.EscalationModeAuto,
		FailMode:            authority.FailModeClosed,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	activeAgent := &agent.Agent{
		ID:               agentID,
		Name:             "Payments Bot",
		Type:             agent.AgentTypeAI,
		Owner:            "payments-team",
		ModelVersion:     "gpt-4o",
		OperationalState: agent.OperationalStateActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	activeGrant := &authority.AuthorityGrant{
		ID:            grantID,
		AgentID:       agentID,
		ProfileID:     profileID,
		Status:        authority.GrantStatusActive,
		GrantedBy:     "ops-lead",
		EffectiveDate: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	introspectionSvc := &mockIntrospectionService{
		getProfileFn: func(ctx context.Context, id string) (*authority.AuthorityProfile, error) {
			if id == profileID {
				return activeProfile, nil
			}
			return nil, nil
		},
		getAgentFn: func(ctx context.Context, id string) (*agent.Agent, error) {
			if id == agentID {
				return activeAgent, nil
			}
			return nil, nil
		},
		listGrantsByAgentFn: func(ctx context.Context, aid string) ([]*authority.AuthorityGrant, error) {
			if aid == agentID {
				return []*authority.AuthorityGrant{activeGrant}, nil
			}
			return []*authority.AuthorityGrant{}, nil
		},
		listGrantsByProfileFn: func(ctx context.Context, pid string) ([]*authority.AuthorityGrant, error) {
			if pid == profileID {
				return []*authority.AuthorityGrant{activeGrant}, nil
			}
			return []*authority.AuthorityGrant{}, nil
		},
	}

	srv := NewServerWithAllServices(&mockOrchestrator{}, nil, nil, introspectionSvc)

	// Step 1: GET /v1/agents/{id} — confirms agent readable.
	rec := performRequest(t, srv, http.MethodGet, "/v1/agents/"+agentID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 1 get agent: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ag := decodeJSON[agentResponse](t, rec)
	if ag.ID != agentID {
		t.Errorf("step 1: expected id %q, got %q", agentID, ag.ID)
	}
	if ag.OperationalState != "active" {
		t.Errorf("step 1: expected operational_state 'active', got %q", ag.OperationalState)
	}

	// Step 2: GET /v1/profiles/{id} — confirms profile readable.
	rec = performRequest(t, srv, http.MethodGet, "/v1/profiles/"+profileID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 2 get profile: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	prof := decodeJSON[profileResponse](t, rec)
	if prof.ID != profileID {
		t.Errorf("step 2: expected id %q, got %q", profileID, prof.ID)
	}
	if prof.SurfaceID != surfaceID {
		t.Errorf("step 2: expected surface_id %q, got %q", surfaceID, prof.SurfaceID)
	}
	if prof.ConfidenceThreshold != 0.80 {
		t.Errorf("step 2: expected confidence_threshold 0.80, got %f", prof.ConfidenceThreshold)
	}

	// Step 3: GET /v1/grants?agent_id={id} — confirms grants readable by agent.
	rec = performRequest(t, srv, http.MethodGet, "/v1/grants?agent_id="+agentID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 3 list grants by agent: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	grantsByAgent := decodeJSON[[]grantResponse](t, rec)
	if len(grantsByAgent) != 1 {
		t.Fatalf("step 3: expected 1 grant, got %d", len(grantsByAgent))
	}
	if grantsByAgent[0].AgentID != agentID {
		t.Errorf("step 3: expected agent_id %q, got %q", agentID, grantsByAgent[0].AgentID)
	}
	if grantsByAgent[0].ProfileID != profileID {
		t.Errorf("step 3: expected profile_id %q, got %q", profileID, grantsByAgent[0].ProfileID)
	}

	// Step 4: GET /v1/grants?profile_id={id} — confirms grants readable by profile.
	rec = performRequest(t, srv, http.MethodGet, "/v1/grants?profile_id="+profileID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("step 4 list grants by profile: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	grantsByProfile := decodeJSON[[]grantResponse](t, rec)
	if len(grantsByProfile) != 1 {
		t.Fatalf("step 4: expected 1 grant, got %d", len(grantsByProfile))
	}
	if grantsByProfile[0].AgentID != agentID {
		t.Errorf("step 4: expected agent_id %q, got %q", agentID, grantsByProfile[0].AgentID)
	}
}

// ---------------------------------------------------------------------------
// GET /v1/surfaces/{id}/impact
// ---------------------------------------------------------------------------

func TestGetSurfaceImpact_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	result := &SurfaceImpactResult{
		Surface: &surface.DecisionSurface{
			ID:             "payments.execute",
			Name:           "Payment Execution",
			Status:         surface.SurfaceStatusActive,
			Version:        1,
			Domain:         "payments",
			BusinessOwner:  "finance-team",
			TechnicalOwner: "platform-team",
			ApprovedBy:     "approver-1",
			ApprovedAt:     &now,
		},
		Profiles: []*authority.AuthorityProfile{
			{ID: "prof-1", Name: "Standard", Status: authority.ProfileStatusActive, Version: 1,
				EscalationMode: authority.EscalationModeAuto, FailMode: authority.FailModeClosed},
		},
		Grants: []*authority.AuthorityGrant{
			{ID: "grant-1", AgentID: "agent-1", ProfileID: "prof-1",
				Status: authority.GrantStatusActive, GrantedBy: "admin", EffectiveDate: now},
		},
		Agents: []*agent.Agent{
			{ID: "agent-1", Name: "Payment Bot", Type: agent.AgentTypeAI,
				Owner: "platform-team", OperationalState: agent.OperationalStateActive},
		},
		Summary: ImpactSummary{
			ProfileCount: 1, GrantCount: 1, AgentCount: 1,
			ActiveProfileCount: 1, ActiveGrantCount: 1, ActiveAgentCount: 1,
		},
		Warnings: []string{
			"surface has active profiles",
			"surface has active grants",
			"deprecating may affect active agent authority",
		},
	}

	svc := &mockIntrospectionService{
		getSurfaceImpactFn: func(ctx context.Context, id string) (*SurfaceImpactResult, error) {
			if id != "payments.execute" {
				t.Errorf("expected id 'payments.execute', got %q", id)
			}
			return result, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/payments.execute/impact", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[surfaceImpactResponse](t, rec)

	// Surface section
	if resp.Surface.ID != "payments.execute" {
		t.Errorf("surface.id: expected 'payments.execute', got %q", resp.Surface.ID)
	}
	if resp.Surface.Status != "active" {
		t.Errorf("surface.status: expected 'active', got %q", resp.Surface.Status)
	}
	if resp.Surface.ApprovedBy != "approver-1" {
		t.Errorf("surface.approved_by: expected 'approver-1', got %q", resp.Surface.ApprovedBy)
	}

	// Profiles section
	if len(resp.Profiles) != 1 {
		t.Fatalf("profiles: expected 1, got %d", len(resp.Profiles))
	}
	if resp.Profiles[0].ID != "prof-1" {
		t.Errorf("profiles[0].id: expected 'prof-1', got %q", resp.Profiles[0].ID)
	}
	if resp.Profiles[0].Status != "active" {
		t.Errorf("profiles[0].status: expected 'active', got %q", resp.Profiles[0].Status)
	}

	// Grants section
	if len(resp.Grants) != 1 {
		t.Fatalf("grants: expected 1, got %d", len(resp.Grants))
	}
	if resp.Grants[0].ID != "grant-1" {
		t.Errorf("grants[0].id: expected 'grant-1', got %q", resp.Grants[0].ID)
	}
	if resp.Grants[0].AgentID != "agent-1" {
		t.Errorf("grants[0].agent_id: expected 'agent-1', got %q", resp.Grants[0].AgentID)
	}

	// Agents section
	if len(resp.Agents) != 1 {
		t.Fatalf("agents: expected 1, got %d", len(resp.Agents))
	}
	if resp.Agents[0].ID != "agent-1" {
		t.Errorf("agents[0].id: expected 'agent-1', got %q", resp.Agents[0].ID)
	}
	if resp.Agents[0].OperationalState != "active" {
		t.Errorf("agents[0].operational_state: expected 'active', got %q", resp.Agents[0].OperationalState)
	}

	// Summary section
	if resp.Summary.ProfileCount != 1 {
		t.Errorf("summary.profile_count: expected 1, got %d", resp.Summary.ProfileCount)
	}
	if resp.Summary.ActiveGrantCount != 1 {
		t.Errorf("summary.active_grant_count: expected 1, got %d", resp.Summary.ActiveGrantCount)
	}
	if resp.Summary.ActiveAgentCount != 1 {
		t.Errorf("summary.active_agent_count: expected 1, got %d", resp.Summary.ActiveAgentCount)
	}

	// Warnings section
	if len(resp.Warnings) != 3 {
		t.Fatalf("warnings: expected 3, got %d: %v", len(resp.Warnings), resp.Warnings)
	}
	if resp.Warnings[0] != "surface has active profiles" {
		t.Errorf("warnings[0]: expected 'surface has active profiles', got %q", resp.Warnings[0])
	}
}

func TestGetSurfaceImpact_NotFound(t *testing.T) {
	svc := &mockIntrospectionService{
		getSurfaceImpactFn: func(ctx context.Context, id string) (*SurfaceImpactResult, error) {
			return nil, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/no-such-surface/impact", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	errResp := decodeError(t, rec)
	if errResp["error"] != "surface not found" {
		t.Errorf("expected 'surface not found', got %q", errResp["error"])
	}
}

func TestGetSurfaceImpact_InvalidID(t *testing.T) {
	srv := newIntrospectionServer(&mockIntrospectionService{})
	// An ID exceeding maxIdentifierLength (255) is rejected with 400.
	longID := strings.Repeat("x", 256)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/"+longID+"/impact", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetSurfaceImpact_MethodNotAllowed(t *testing.T) {
	srv := newIntrospectionServer(&mockIntrospectionService{})
	rec := performRequest(t, srv, http.MethodPost, "/v1/surfaces/surf-1/impact", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("expected Allow 'GET', got %q", allow)
	}
}

func TestGetSurfaceImpact_NilService(t *testing.T) {
	// No introspection service wired — all introspection endpoints return 501.
	srv := NewServerWithAllServices(&mockOrchestrator{}, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/surf-1/impact", nil)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
}

// TestGetSurfaceImpact_EmptyDependencies verifies that surfaces with no profiles,
// grants, or agents produce empty arrays (not null) and an empty warnings list.
func TestGetSurfaceImpact_EmptyDependencies(t *testing.T) {
	surf := &surface.DecisionSurface{
		ID:     "isolated-surface",
		Name:   "Isolated Surface",
		Status: surface.SurfaceStatusReview,
	}
	result := &SurfaceImpactResult{
		Surface:  surf,
		Profiles: []*authority.AuthorityProfile{},
		Grants:   []*authority.AuthorityGrant{},
		Agents:   []*agent.Agent{},
		Summary:  ImpactSummary{},
		Warnings: []string{},
	}

	svc := &mockIntrospectionService{
		getSurfaceImpactFn: func(ctx context.Context, id string) (*SurfaceImpactResult, error) {
			return result, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/isolated-surface/impact", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[surfaceImpactResponse](t, rec)

	if resp.Profiles == nil || len(resp.Profiles) != 0 {
		t.Errorf("profiles: expected empty slice, got %v", resp.Profiles)
	}
	if resp.Grants == nil || len(resp.Grants) != 0 {
		t.Errorf("grants: expected empty slice, got %v", resp.Grants)
	}
	if resp.Agents == nil || len(resp.Agents) != 0 {
		t.Errorf("agents: expected empty slice, got %v", resp.Agents)
	}
	if resp.Warnings == nil || len(resp.Warnings) != 0 {
		t.Errorf("warnings: expected empty slice, got %v", resp.Warnings)
	}
	if resp.Summary.ProfileCount != 0 {
		t.Errorf("summary.profile_count: expected 0, got %d", resp.Summary.ProfileCount)
	}
}

// TestGetSurfaceImpact_Integration exercises the full dependency graph through
// the HTTP layer using the real IntrospectionService backed by mock repositories.
// It creates a surface, two profiles, two agents, two grants (one active, one
// revoked), then verifies that the impact response is correct: counts,
// deduplication, ordering, and warnings all match expected state.
func TestGetSurfaceImpact_Integration(t *testing.T) {
	surfaceID := "lending.approve"
	profileAID := "prof-a"
	profileBID := "prof-b"
	agentAID := "agent-a"
	agentBID := "agent-b"
	grantAID := "grant-a"
	grantBID := "grant-b"

	now := time.Now().UTC().Truncate(time.Second)

	surf := &surface.DecisionSurface{
		ID:             surfaceID,
		Name:           "Lending Approval",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		Domain:         "lending",
		BusinessOwner:  "credit-team",
		TechnicalOwner: "platform-team",
		ApprovedBy:     "admin",
		ApprovedAt:     &now,
	}
	profileA := &authority.AuthorityProfile{
		ID:             profileAID,
		SurfaceID:      surfaceID,
		Name:           "Active Profile",
		Status:         authority.ProfileStatusActive,
		Version:        1,
		EscalationMode: authority.EscalationModeAuto,
		FailMode:       authority.FailModeClosed,
	}
	profileB := &authority.AuthorityProfile{
		ID:             profileBID,
		SurfaceID:      surfaceID,
		Name:           "Draft Profile",
		Status:         authority.ProfileStatusDraft,
		Version:        1,
		EscalationMode: authority.EscalationModeManual,
		FailMode:       authority.FailModeOpen,
	}
	agentA := &agent.Agent{
		ID:               agentAID,
		Name:             "Lending Bot",
		Type:             agent.AgentTypeAI,
		Owner:            "credit-team",
		OperationalState: agent.OperationalStateActive,
	}
	agentB := &agent.Agent{
		ID:               agentBID,
		Name:             "Analyst Service",
		Type:             agent.AgentTypeService,
		Owner:            "risk-team",
		OperationalState: agent.OperationalStateSuspended,
	}
	grantA := &authority.AuthorityGrant{
		ID:            grantAID,
		AgentID:       agentAID,
		ProfileID:     profileAID,
		Status:        authority.GrantStatusActive,
		GrantedBy:     "admin",
		EffectiveDate: now,
	}
	grantB := &authority.AuthorityGrant{
		ID:            grantBID,
		AgentID:       agentBID,
		ProfileID:     profileBID,
		Status:        authority.GrantStatusRevoked,
		GrantedBy:     "admin",
		EffectiveDate: now,
	}

	// Wire mock repositories that serve the prepared fixtures.
	svc := &mockIntrospectionService{
		getSurfaceImpactFn: func(ctx context.Context, id string) (*SurfaceImpactResult, error) {
			if id != surfaceID {
				return nil, nil
			}
			profiles := []*authority.AuthorityProfile{profileA, profileB}
			grants := []*authority.AuthorityGrant{grantA, grantB}
			agents := []*agent.Agent{agentA, agentB}
			summary := ImpactSummary{
				ProfileCount:       2,
				GrantCount:         2,
				AgentCount:         2,
				ActiveProfileCount: 1, // only profileA
				ActiveGrantCount:   1, // only grantA
				ActiveAgentCount:   1, // only agentA (agentB is suspended)
			}
			return &SurfaceImpactResult{
				Surface:  surf,
				Profiles: profiles,
				Grants:   grants,
				Agents:   agents,
				Summary:  summary,
				Warnings: buildImpactWarnings(summary),
			}, nil
		},
	}

	srv := newIntrospectionServer(svc)
	rec := performRequest(t, srv, http.MethodGet, "/v1/surfaces/"+surfaceID+"/impact", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[surfaceImpactResponse](t, rec)

	// Surface metadata
	if resp.Surface.ID != surfaceID {
		t.Errorf("surface.id: expected %q, got %q", surfaceID, resp.Surface.ID)
	}
	if resp.Surface.Domain != "lending" {
		t.Errorf("surface.domain: expected 'lending', got %q", resp.Surface.Domain)
	}

	// Both profiles present
	if len(resp.Profiles) != 2 {
		t.Fatalf("profiles: expected 2, got %d", len(resp.Profiles))
	}
	if resp.Profiles[0].ID != profileAID || resp.Profiles[1].ID != profileBID {
		t.Errorf("profiles order: got [%q, %q]", resp.Profiles[0].ID, resp.Profiles[1].ID)
	}

	// Both grants present
	if len(resp.Grants) != 2 {
		t.Fatalf("grants: expected 2, got %d", len(resp.Grants))
	}
	if resp.Grants[0].ID != grantAID || resp.Grants[1].ID != grantBID {
		t.Errorf("grants order: got [%q, %q]", resp.Grants[0].ID, resp.Grants[1].ID)
	}

	// Both distinct agents present
	if len(resp.Agents) != 2 {
		t.Fatalf("agents: expected 2, got %d", len(resp.Agents))
	}
	if resp.Agents[0].ID != agentAID || resp.Agents[1].ID != agentBID {
		t.Errorf("agents order: got [%q, %q]", resp.Agents[0].ID, resp.Agents[1].ID)
	}

	// Summary counts reflect active-only filtering
	if resp.Summary.ProfileCount != 2 {
		t.Errorf("summary.profile_count: expected 2, got %d", resp.Summary.ProfileCount)
	}
	if resp.Summary.ActiveProfileCount != 1 {
		t.Errorf("summary.active_profile_count: expected 1, got %d", resp.Summary.ActiveProfileCount)
	}
	if resp.Summary.GrantCount != 2 {
		t.Errorf("summary.grant_count: expected 2, got %d", resp.Summary.GrantCount)
	}
	if resp.Summary.ActiveGrantCount != 1 {
		t.Errorf("summary.active_grant_count: expected 1, got %d", resp.Summary.ActiveGrantCount)
	}
	if resp.Summary.AgentCount != 2 {
		t.Errorf("summary.agent_count: expected 2, got %d", resp.Summary.AgentCount)
	}
	if resp.Summary.ActiveAgentCount != 1 {
		t.Errorf("summary.active_agent_count: expected 1, got %d", resp.Summary.ActiveAgentCount)
	}

	// Warnings: active profile + active grant + active agent → all three
	if len(resp.Warnings) != 3 {
		t.Fatalf("warnings: expected 3, got %d: %v", len(resp.Warnings), resp.Warnings)
	}
	wantWarnings := []string{
		"surface has active profiles",
		"surface has active grants",
		"deprecating may affect active agent authority",
	}
	for i, w := range wantWarnings {
		if resp.Warnings[i] != w {
			t.Errorf("warnings[%d]: expected %q, got %q", i, w, resp.Warnings[i])
		}
	}
}
