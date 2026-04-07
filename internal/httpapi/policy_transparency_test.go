package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
)

// ---------------------------------------------------------------------------
// evaluate response — policy transparency fields
// ---------------------------------------------------------------------------

func TestEvaluate_Response_IncludesPolicyModeNoop(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
				EnvelopeID: "env-1",
				PolicyMode: "noop",
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
		"request_id": "req-policy-noop-001",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[evaluateResponse](t, rec)
	if resp.PolicyMode != "noop" {
		t.Errorf("policy_mode: want %q, got %q", "noop", resp.PolicyMode)
	}
}

func TestEvaluate_Response_PolicySkipped_True_WhenNoopAndPolicyRef(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{
				Outcome:         eval.OutcomeAccept,
				ReasonCode:      eval.ReasonWithinAuthority,
				EnvelopeID:      "env-1",
				PolicyMode:      "noop",
				PolicyReference: "rego://payments/limits",
				PolicySkipped:   true,
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
		"request_id": "req-policy-skip-001",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[evaluateResponse](t, rec)
	if !resp.PolicySkipped {
		t.Error("policy_skipped: want true when noop active and profile has policy_ref")
	}
	if resp.PolicyReference != "rego://payments/limits" {
		t.Errorf("policy_reference: want %q, got %q", "rego://payments/limits", resp.PolicyReference)
	}
}

func TestEvaluate_Response_PolicySkipped_Absent_WhenNoPolicyRef(t *testing.T) {
	mock := &mockOrchestrator{
		evaluateFn: func(_ context.Context, _ eval.DecisionRequest, _ json.RawMessage) (decision.EvaluationResult, error) {
			return decision.EvaluationResult{
				Outcome:    eval.OutcomeAccept,
				ReasonCode: eval.ReasonWithinAuthority,
				EnvelopeID: "env-1",
				PolicyMode: "noop",
				// PolicyReference empty, PolicySkipped false (zero values)
			}, nil
		},
	}

	srv := NewServer(mock)
	payload := marshalJSON(t, map[string]any{
		"surface_id": "surf-1",
		"agent_id":   "agent-1",
		"confidence": 0.9,
		"request_id": "req-policy-noref-001",
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[evaluateResponse](t, rec)
	if resp.PolicySkipped {
		t.Error("policy_skipped: want false when no policy_ref configured")
	}
	if resp.PolicyReference != "" {
		t.Errorf("policy_reference: want empty, got %q", resp.PolicyReference)
	}
}

// ---------------------------------------------------------------------------
// /healthz — policy metadata
// ---------------------------------------------------------------------------

func TestHealth_IncludesPolicyMeta_WhenSet(t *testing.T) {
	srv := newTestServer().WithPolicyMeta("noop", "NoOpPolicyEvaluator")

	rec := performRequest(t, srv, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if resp["policy_mode"] != "noop" {
		t.Errorf("policy_mode: want %q, got %q", "noop", resp["policy_mode"])
	}
	if resp["policy_evaluator"] != "NoOpPolicyEvaluator" {
		t.Errorf("policy_evaluator: want %q, got %q", "NoOpPolicyEvaluator", resp["policy_evaluator"])
	}
	// Existing fields must still be present.
	if resp["status"] != "ok" {
		t.Errorf("status: want %q, got %q", "ok", resp["status"])
	}
}

func TestHealth_NoPolicyMeta_WhenNotSet(t *testing.T) {
	srv := newTestServer() // no WithPolicyMeta call

	rec := performRequest(t, srv, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if _, ok := resp["policy_mode"]; ok {
		t.Errorf("policy_mode: want absent when not configured, got %q", resp["policy_mode"])
	}
}

// ---------------------------------------------------------------------------
// /readyz — policy metadata
// ---------------------------------------------------------------------------

func TestReady_IncludesPolicyMeta_WhenSet(t *testing.T) {
	srv := newTestServer().WithPolicyMeta("noop", "NoOpPolicyEvaluator")

	rec := performRequest(t, srv, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if resp["policy_mode"] != "noop" {
		t.Errorf("policy_mode: want %q, got %q", "noop", resp["policy_mode"])
	}
	if resp["policy_evaluator"] != "NoOpPolicyEvaluator" {
		t.Errorf("policy_evaluator: want %q, got %q", "NoOpPolicyEvaluator", resp["policy_evaluator"])
	}
	if resp["status"] != "ready" {
		t.Errorf("status: want %q, got %q", "ready", resp["status"])
	}
}
