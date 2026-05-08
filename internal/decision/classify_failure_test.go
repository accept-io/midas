package decision

// Tests for classifyFailure, wrapFailure, and categorizePersistErr.
//
// This file is in package decision (not decision_test) because classifyFailure
// and the related helpers are unexported.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store/memory"
)

// ---------------------------------------------------------------------------
// classifyFailure — explicit wrapper (first priority)
// ---------------------------------------------------------------------------

func TestClassifyFailure_ExplicitWrapper(t *testing.T) {
	cases := []struct {
		name     string
		category FailureCategory
	}{
		{"envelope_persistence", FailureCategoryEnvelopePersistence},
		{"audit_append", FailureCategoryAuditAppend},
		{"invalid_transition", FailureCategoryInvalidTransition},
		{"policy_evaluation", FailureCategoryPolicyEvaluation},
		{"authority_resolution", FailureCategoryAuthorityResolution},
		{"resolve_review", FailureCategoryResolveReview},
		{"unknown", FailureCategoryUnknown},
	}

	for _, tc := range cases {
		t.Run(string(tc.category), func(t *testing.T) {
			err := wrapFailure(tc.category, errors.New("some inner error"))
			got := classifyFailure(err)
			if got != string(tc.category) {
				t.Errorf("classifyFailure(%q) = %q, want %q", tc.category, got, tc.category)
			}
		})
	}
}

// Wrapping preserves the error chain: errors.Is must still find the inner sentinel.
func TestClassifyFailure_ExplicitWrapper_PreservesChain(t *testing.T) {
	inner := errors.New("inner sentinel")
	wrapped := wrapFailure(FailureCategoryInvalidTransition, fmt.Errorf("outer: %w", inner))
	if !errors.Is(wrapped, inner) {
		t.Error("errors.Is should find inner sentinel through wrapFailure chain")
	}
	if classifyFailure(wrapped) != string(FailureCategoryInvalidTransition) {
		t.Errorf("classifyFailure should return FailureCategoryInvalidTransition")
	}
}

// A wrapFailure error nested inside another fmt.Errorf wrapper is still found
// via errors.As.
func TestClassifyFailure_ExplicitWrapper_NestedInFmtErrorf(t *testing.T) {
	inner := wrapFailure(FailureCategoryEnvelopePersistence, errors.New("db error"))
	outer := fmt.Errorf("persist evaluation: %w", inner)
	got := classifyFailure(outer)
	if got != string(FailureCategoryEnvelopePersistence) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryEnvelopePersistence)
	}
}

// ---------------------------------------------------------------------------
// classifyFailure — sentinel errors (second priority)
// ---------------------------------------------------------------------------

func TestClassifyFailure_Sentinels(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ErrEnvelopeNotFound", ErrEnvelopeNotFound},
		{"ErrEnvelopeNotAwaitingReview", ErrEnvelopeNotAwaitingReview},
		{"ErrEnvelopeAlreadyClosed", ErrEnvelopeAlreadyClosed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyFailure(tc.err)
			if got != string(FailureCategoryResolveReview) {
				t.Errorf("classifyFailure(%v) = %q, want %q", tc.err, got, FailureCategoryResolveReview)
			}
		})
	}
}

// Sentinels wrapped in fmt.Errorf are still classified correctly.
func TestClassifyFailure_Sentinels_Wrapped(t *testing.T) {
	err := fmt.Errorf("envelope %s is in state %s: %w", "env-1", "EVALUATING", ErrEnvelopeNotAwaitingReview)
	got := classifyFailure(err)
	if got != string(FailureCategoryResolveReview) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryResolveReview)
	}
}

// ---------------------------------------------------------------------------
// classifyFailure — heuristic fallback (third priority)
// ---------------------------------------------------------------------------

func TestClassifyFailure_Heuristic_Policy(t *testing.T) {
	err := errors.New("policy evaluation returned unexpected error")
	got := classifyFailure(err)
	if got != string(FailureCategoryPolicyEvaluation) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryPolicyEvaluation)
	}
}

func TestClassifyFailure_Heuristic_AuthorityResolution(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"authority", "authority chain lookup failed"},
		{"grant", "grant not found for agent"},
		{"profile", "profile version mismatch"},
		{"surface", "surface is inactive"},
		{"agent", "agent suspended"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyFailure(errors.New(tc.msg))
			if got != string(FailureCategoryAuthorityResolution) {
				t.Errorf("classifyFailure(%q) = %q, want %q", tc.msg, got, FailureCategoryAuthorityResolution)
			}
		})
	}
}

func TestClassifyFailure_Heuristic_Unknown(t *testing.T) {
	err := errors.New("something completely unexpected happened")
	got := classifyFailure(err)
	if got != string(FailureCategoryUnknown) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryUnknown)
	}
}

func TestClassifyFailure_Nil(t *testing.T) {
	got := classifyFailure(nil)
	if got != "" {
		t.Errorf("classifyFailure(nil) = %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// categorizePersistErr
// ---------------------------------------------------------------------------

func TestCategorizePersistErr_Nil(t *testing.T) {
	if categorizePersistErr(nil) != nil {
		t.Error("categorizePersistErr(nil) should return nil")
	}
}

func TestCategorizePersistErr_AuditAppend(t *testing.T) {
	// Simulate the error produced by flushEventsAndUpdate in the accumulator.
	inner := errors.New("db connection refused")
	err := categorizePersistErr(fmt.Errorf("persist evaluation: %w",
		fmt.Errorf("audit append envelope_created [envelope env-1]: %w", inner)))

	got := classifyFailure(err)
	if got != string(FailureCategoryAuditAppend) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryAuditAppend)
	}
	// Error chain must still reach the original error.
	if !errors.Is(err, inner) {
		t.Error("errors.Is should find original inner error through categorizePersistErr chain")
	}
}

func TestCategorizePersistErr_EnvelopeCreate(t *testing.T) {
	// Simulate a Create failure from persistNew.
	inner := errors.New("unique constraint violation")
	err := categorizePersistErr(fmt.Errorf("persist evaluation: %w",
		fmt.Errorf("create envelope [env-1]: %w", inner)))

	got := classifyFailure(err)
	if got != string(FailureCategoryEnvelopePersistence) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryEnvelopePersistence)
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should find original inner error")
	}
}

func TestCategorizePersistErr_ScopeConflict_IsIdempotencyConflict(t *testing.T) {
	// Simulate a concurrent duplicate insert: the postgres repo returns
	// ErrEnvelopeScopeConflict directly (no wrapping) on pq code 23505.
	// categorizePersistErr must recognise it via errors.Is and map it to
	// FailureCategoryIdempotencyConflict so the HTTP layer returns 409.
	err := categorizePersistErr(fmt.Errorf("persist evaluation: %w", envelope.ErrEnvelopeScopeConflict))

	got := classifyFailure(err)
	if got != string(FailureCategoryIdempotencyConflict) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryIdempotencyConflict)
	}
	// The categorised error must also satisfy errors.Is(err, ErrScopedRequestConflict)
	// so that mapDomainError in the HTTP layer can route it to 409.
	if !errors.Is(err, ErrScopedRequestConflict) {
		t.Error("errors.Is(err, ErrScopedRequestConflict) should be true")
	}
}

func TestCategorizePersistErr_EnvelopeUpdate(t *testing.T) {
	// Simulate an Update failure from flushEventsAndUpdate.
	inner := errors.New("serialization failure")
	err := categorizePersistErr(fmt.Errorf("persist evaluation: %w",
		fmt.Errorf("persist final envelope state [env-1]: %w", inner)))

	got := classifyFailure(err)
	if got != string(FailureCategoryEnvelopePersistence) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryEnvelopePersistence)
	}
}

// ---------------------------------------------------------------------------
// wrapFailure applied to transition errors — as done in finish() / evaluate()
// ---------------------------------------------------------------------------

func TestClassifyFailure_TransitionViaWrapFailure(t *testing.T) {
	// Simulate what acc.transition() returns (from evaluation_accumulator.go).
	transErr := fmt.Errorf("transition CLOSED→EVALUATING: %w", errors.New("invalid transition"))
	wrapped := wrapFailure(FailureCategoryInvalidTransition, transErr)

	got := classifyFailure(wrapped)
	if got != string(FailureCategoryInvalidTransition) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryInvalidTransition)
	}
}

// A wrapFailure-wrapped transition error nested in a fmt.Errorf from WithTx
// is still classified correctly (simulates the ResolveEscalation path where
// the WithTx closure returns the error).
func TestClassifyFailure_TransitionViaWrapFailure_ThroughWithTx(t *testing.T) {
	transErr := fmt.Errorf("transition AWAITING_REVIEW→EVALUATING: %w", errors.New("invalid transition"))
	innerWrapped := wrapFailure(FailureCategoryInvalidTransition, transErr)
	// WithTx typically re-wraps or passes through; simulate a fmt.Errorf wrap.
	outerErr := fmt.Errorf("resolve_escalation tx: %w", innerWrapped)

	got := classifyFailure(outerErr)
	if got != string(FailureCategoryInvalidTransition) {
		t.Errorf("classifyFailure = %q, want %q", got, FailureCategoryInvalidTransition)
	}
}

// ---------------------------------------------------------------------------
// FailureClass mapping (D27i-c chunk 1 of F1)
// ---------------------------------------------------------------------------

// TestClassifyFailure_FailureClassMapping pins the (FailureCategory →
// FailureClass) mapping table from D27i-a-rev1 §2.2. Each of the eight
// FailureCategory* constants must map to exactly the class the ADR
// names. Drift in this table is a governance change, not a code change —
// hence the exhaustive table-driven assertion.
func TestClassifyFailure_FailureClassMapping(t *testing.T) {
	cases := []struct {
		category FailureCategory
		want     FailureClass
	}{
		{FailureCategoryEnvelopePersistence, FailureClassGovernanceIntegrity},
		{FailureCategoryAuditAppend, FailureClassGovernanceIntegrity},
		{FailureCategoryInvalidTransition, FailureClassConsistency},
		{FailureCategoryAuthorityResolution, FailureClassConsistency},
		{FailureCategoryIdempotencyConflict, FailureClassInput},
		{FailureCategoryPolicyEvaluation, FailureClassResource},
		{FailureCategoryResolveReview, FailureClassConsistency},
		{FailureCategoryUnknown, FailureClassConsistency},
	}
	for _, tc := range cases {
		t.Run(string(tc.category), func(t *testing.T) {
			err := wrapFailure(tc.category, errors.New("inner"))
			gotCat, gotCls := ClassifyFailure(err)
			if gotCat != tc.category {
				t.Errorf("category: got %q, want %q", gotCat, tc.category)
			}
			if gotCls != tc.want {
				t.Errorf("class: got %q, want %q", gotCls, tc.want)
			}
		})
	}
}

// TestClassifyFailure_HelperReturnsCategoryAndClass exercises the three
// classifyFailure code paths (explicit wrapper, sentinel, heuristic
// fallback) plus the nil case, and asserts ClassifyFailure returns both
// axes consistently with the existing classifyFailure string output.
func TestClassifyFailure_HelperReturnsCategoryAndClass(t *testing.T) {
	t.Run("wrapped_category", func(t *testing.T) {
		err := wrapFailure(FailureCategoryEnvelopePersistence, errors.New("boom"))
		cat, cls := ClassifyFailure(err)
		if cat != FailureCategoryEnvelopePersistence {
			t.Errorf("category: got %q, want %q", cat, FailureCategoryEnvelopePersistence)
		}
		if cls != FailureClassGovernanceIntegrity {
			t.Errorf("class: got %q, want %q", cls, FailureClassGovernanceIntegrity)
		}
	})

	t.Run("sentinel_review", func(t *testing.T) {
		cat, cls := ClassifyFailure(ErrEnvelopeNotFound)
		if cat != FailureCategoryResolveReview {
			t.Errorf("category: got %q, want %q", cat, FailureCategoryResolveReview)
		}
		if cls != FailureClassConsistency {
			t.Errorf("class: got %q, want %q", cls, FailureClassConsistency)
		}
	})

	t.Run("heuristic_authority_resolution", func(t *testing.T) {
		cat, cls := ClassifyFailure(errors.New("authority chain lookup failed"))
		if cat != FailureCategoryAuthorityResolution {
			t.Errorf("category: got %q, want %q", cat, FailureCategoryAuthorityResolution)
		}
		if cls != FailureClassConsistency {
			t.Errorf("class: got %q, want %q", cls, FailureClassConsistency)
		}
	})

	t.Run("heuristic_unknown", func(t *testing.T) {
		cat, cls := ClassifyFailure(errors.New("something completely unexpected"))
		if cat != FailureCategoryUnknown {
			t.Errorf("category: got %q, want %q", cat, FailureCategoryUnknown)
		}
		if cls != FailureClassConsistency {
			t.Errorf("class: got %q, want %q", cls, FailureClassConsistency)
		}
	})

	t.Run("nil_returns_empty_pair", func(t *testing.T) {
		cat, cls := ClassifyFailure(nil)
		if cat != "" {
			t.Errorf("nil category: got %q, want empty", cat)
		}
		if cls != "" {
			t.Errorf("nil class: got %q, want empty", cls)
		}
	})
}

// TestEvaluationFailed_SlogIncludesCorrectnessClass drives the
// orchestrator's failure path end-to-end and asserts that the resulting
// `evaluation_failed` slog event carries the new `correctness_class`
// field. The simplest deterministic trigger is a request with empty
// RequestSource: o.evaluate() returns errors.New("request_source is
// required") at the validation guard, which propagates through WithTx
// into the failure-path slog at orchestrator.go:274. The raw error has
// no FailureCategory* wrap and no matching heuristic, so classifyFailure
// produces FailureCategoryUnknown → FailureClassConsistency.
func TestEvaluationFailed_SlogIncludesCorrectnessClass(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	prev := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(prev)

	memStore := memory.NewStoreWithRepositories(memory.NewRepositories())
	orch, err := NewOrchestrator(memStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	req := eval.DecisionRequest{
		// RequestSource intentionally empty — triggers
		// "request_source is required" inside o.evaluate().
		RequestID: "req-classify-slog-test",
		SurfaceID: "surf-x",
		AgentID:   "agent-x",
	}
	if _, evalErr := orch.Evaluate(context.Background(), req, json.RawMessage(`{}`)); evalErr == nil {
		t.Fatal("expected evaluate to fail with empty request_source")
	}

	found := false
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["msg"] != "evaluation_failed" {
			continue
		}
		cls, ok := entry["correctness_class"].(string)
		if !ok {
			t.Errorf("evaluation_failed slog line is missing correctness_class field, got: %v", entry)
			return
		}
		if cls != string(FailureClassConsistency) {
			t.Errorf("correctness_class: got %q, want %q (FailureCategoryUnknown maps to consistency)",
				cls, FailureClassConsistency)
		}
		// Existing failure_kind must remain present and unchanged.
		if kind, _ := entry["failure_kind"].(string); kind != string(FailureCategoryUnknown) {
			t.Errorf("failure_kind: got %q, want %q", kind, FailureCategoryUnknown)
		}
		found = true
		break
	}
	if !found {
		t.Errorf("evaluation_failed slog event not captured; buffer:\n%s", buf.String())
	}
}
