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

	cpTypes "github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
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
		listEnvelopesFn: func(ctx context.Context) ([]*envelope.Envelope, error) {
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
		listEnvelopesFn: func(ctx context.Context) ([]*envelope.Envelope, error) {
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
	applyBundleFn func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error)
}

func (m *mockControlPlane) ApplyBundle(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
	if m.applyBundleFn != nil {
		return m.applyBundleFn(ctx, bundle)
	}
	return nil, fmt.Errorf("applyBundle not implemented")
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
		applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
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
				applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
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
		applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
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
		applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
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
		applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
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
		applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
			return nil, errors.New("parse error: invalid yaml syntax")
		},
	}

	srv := NewServerWithControlPlane(&mockOrchestrator{}, mockCP)
	rec := performRequestWithHeaders(t, srv, http.MethodPost, "/v1/controlplane/apply",
		[]byte("invalid: [yaml}"), map[string]string{"Content-Type": "application/yaml"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for parse error, got %d", rec.Code)
	}

	errResp := decodeError(t, rec)
	if !strings.Contains(errResp["error"], "parse") {
		t.Errorf("expected error to mention 'parse', got %q", errResp["error"])
	}
}

func TestApplyBundle_InfrastructureError(t *testing.T) {
	mockCP := &mockControlPlane{
		applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
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
		applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
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
		applyBundleFn: func(ctx context.Context, bundle []byte) (*cpTypes.ApplyResult, error) {
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
