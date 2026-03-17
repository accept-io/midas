package apply

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

func TestNewService(t *testing.T) {
	svc := NewService()
	if svc == nil {
		t.Fatal("expected NewService to return a non-nil service")
	}
}

// TestServiceApply_EmptyBundle verifies that a nil bundle is treated as vacuously
// successful: no validation errors, no created entries, Success() == true.
// In the current apply service, an empty bundle produces no validation errors
// and no result statuses, so Success() should be true.
func TestServiceApply_EmptyBundle(t *testing.T) {
	svc := NewService()

	result := svc.Apply(context.Background(), nil)

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d", result.ValidationErrorCount())
	}
	if result.TotalCount() != 0 {
		t.Fatalf("expected zero results, got %d", result.TotalCount())
	}
	if !result.Success() {
		t.Fatal("expected empty bundle apply to be successful")
	}
}

func TestServiceApply_ValidBundle_ReturnsCreatedResults(t *testing.T) {
	svc := NewService()

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   "payment.execute",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata: types.DocumentMetadata{
					ID:   "payment.execute",
					Name: "Payment Execution",
				},
				Spec: types.SurfaceSpec{
					Description: "Authorization for executing payment transactions",
					Category:    "financial",
					RiskTier:    "high",
					Status:      "active",
				},
			},
		},
		{
			Kind: types.KindAgent,
			ID:   "agent-credit-scoring-prod",
			Doc: types.AgentDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindAgent,
				Metadata: types.DocumentMetadata{
					ID:   "agent-credit-scoring-prod",
					Name: "Credit Scoring Agent",
				},
				Spec: types.AgentSpec{
					Type: "llm_agent",
					Runtime: types.AgentRuntime{
						Model:    "gpt-4",
						Version:  "2024-11-20",
						Provider: "openai",
					},
					Status: "active",
				},
			},
		},
		{
			Kind: types.KindProfile,
			ID:   "payments-tier-1",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata: types.DocumentMetadata{
					ID:   "payments-tier-1",
					Name: "Payments Auto-Approve Tier 1",
				},
				Spec: types.ProfileSpec{
					SurfaceID: "payment.execute",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.85,
						ConsequenceThreshold: types.ConsequenceThreshold{
							Type:     "monetary",
							Amount:   10000,
							Currency: "USD",
						},
					},
					InputRequirements: types.ProfileInputRequirements{
						RequiredContext: []string{"customer_id", "transaction_id"},
					},
					Policy: types.ProfilePolicy{
						Reference: "rego://payments/auto_approve_v1",
						FailMode:  "closed",
					},
				},
			},
		},
		{
			Kind: types.KindGrant,
			ID:   "grant-credit-scoring-tier-1",
			Doc: types.GrantDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindGrant,
				Metadata: types.DocumentMetadata{
					ID:   "grant-credit-scoring-tier-1",
					Name: "Credit Scoring Tier 1 Grant",
				},
				Spec: types.GrantSpec{
					AgentID:        "agent-credit-scoring-prod",
					ProfileID:      "payments-tier-1",
					GrantedBy:      "admin@example.com",
					GrantedAt:      "2025-03-17T10:00:00Z",
					EffectiveFrom:  "2025-03-17T10:00:00Z",
					EffectiveUntil: "2025-12-31T23:59:59Z",
					Status:         "active",
				},
			},
		},
	}

	result := svc.Apply(context.Background(), docs)

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got %d: %+v", result.ValidationErrorCount(), result.ValidationErrors)
	}
	if result.CreatedCount() != 4 {
		t.Fatalf("expected 4 created results, got %d", result.CreatedCount())
	}
	if result.TotalCount() != 4 {
		t.Fatalf("expected 4 total results, got %d", result.TotalCount())
	}
	if !result.Success() {
		t.Fatal("expected valid bundle apply to be successful")
	}
}

func TestServiceApply_InvalidBundle_ReturnsValidationErrorsOnly(t *testing.T) {
	svc := NewService()

	// Name and Category are intentionally blank to trigger validation failures
	// on metadata.name and spec.category respectively.
	docs := []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   "payment.execute",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata: types.DocumentMetadata{
					ID:   "payment.execute",
					Name: "",
				},
				Spec: types.SurfaceSpec{
					Description: "Authorization for executing payment transactions",
					Category:    "",
					RiskTier:    "high",
					Status:      "active",
				},
			},
		},
	}

	result := svc.Apply(context.Background(), docs)

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors, got none")
	}

	errFields := make(map[string]bool)
	for _, e := range result.ValidationErrors {
		errFields[e.Field] = true
	}
	if !errFields["metadata.name"] {
		t.Error("expected validation error for metadata.name, not found")
	}
	if !errFields["spec.category"] {
		t.Error("expected validation error for spec.category, not found")
	}

	if result.TotalCount() != 0 {
		t.Fatalf("expected zero created results for invalid bundle, got %d", result.TotalCount())
	}
	if result.Success() {
		t.Fatal("expected invalid bundle apply to be unsuccessful")
	}
}

func TestServiceApply_MixedInvalidBundle_ReturnsNoCreatedResults(t *testing.T) {
	svc := NewService()

	// One valid Surface and one invalid Grant (AgentID blank).
	// Bundle-level validation must poison the entire apply: zero creates expected.
	docs := []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   "payment.execute",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata: types.DocumentMetadata{
					ID:   "payment.execute",
					Name: "Payment Execution",
				},
				Spec: types.SurfaceSpec{
					Description: "Authorization for executing payment transactions",
					Category:    "financial",
					RiskTier:    "high",
					Status:      "active",
				},
			},
		},
		{
			Kind: types.KindGrant,
			ID:   "grant-credit-scoring-tier-1",
			Doc: types.GrantDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindGrant,
				Metadata: types.DocumentMetadata{
					ID:   "grant-credit-scoring-tier-1",
					Name: "Credit Scoring Tier 1 Grant",
				},
				Spec: types.GrantSpec{
					AgentID:        "",
					ProfileID:      "payments-tier-1",
					GrantedBy:      "admin@example.com",
					GrantedAt:      "2025-03-17T10:00:00Z",
					EffectiveFrom:  "2025-03-17T10:00:00Z",
					EffectiveUntil: "2025-12-31T23:59:59Z",
					Status:         "active",
				},
			},
		},
	}

	result := svc.Apply(context.Background(), docs)

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation errors, got none")
	}
	if result.CreatedCount() != 0 {
		t.Fatalf("expected zero created results for mixed invalid bundle, got %d", result.CreatedCount())
	}
	if result.TotalCount() != 0 {
		t.Fatalf("expected zero total results for mixed invalid bundle, got %d", result.TotalCount())
	}
	if result.Success() {
		t.Fatal("expected mixed invalid bundle apply to be unsuccessful")
	}
}

type stubRepo struct{}

func (stubRepo) CreateSurface(types.SurfaceDocument) error { return nil }
func (stubRepo) CreateAgent(types.AgentDocument) error     { return nil }
func (stubRepo) CreateProfile(types.ProfileDocument) error { return nil }
func (stubRepo) CreateGrant(types.GrantDocument) error     { return nil }

func TestNewServiceWithRepo(t *testing.T) {
	svc := NewServiceWithRepo(stubRepo{})
	if svc == nil {
		t.Fatal("expected NewServiceWithRepo to return a non-nil service")
	}
}
