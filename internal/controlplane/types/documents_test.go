package types_test

import (
	"encoding/json"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/types"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Surface Document Tests
// ---------------------------------------------------------------------------

func TestSurfaceDocument_JSONRoundTrip(t *testing.T) {
	original := types.SurfaceDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindSurface,
		Metadata: types.DocumentMetadata{
			ID:   "payment.execute",
			Name: "Payment Execution",
			Labels: map[string]string{
				"team":       "payments",
				"compliance": "pci",
			},
		},
		Spec: types.SurfaceSpec{
			Description: "Authorization for executing payment transactions",
			Category:    "financial",
			RiskTier:    "high",
			Status:      "active",
		},
	}

	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	var decoded types.SurfaceDocument
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if decoded.APIVersion != original.APIVersion {
		t.Errorf("apiVersion: expected %q, got %q", original.APIVersion, decoded.APIVersion)
	}
	if decoded.Kind != original.Kind {
		t.Errorf("kind: expected %q, got %q", original.Kind, decoded.Kind)
	}
	if decoded.Metadata.ID != original.Metadata.ID {
		t.Errorf("metadata.id: expected %q, got %q", original.Metadata.ID, decoded.Metadata.ID)
	}
	if decoded.Spec.Category != original.Spec.Category {
		t.Errorf("spec.category: expected %q, got %q", original.Spec.Category, decoded.Spec.Category)
	}
	if decoded.Spec.RiskTier != original.Spec.RiskTier {
		t.Errorf("spec.risk_tier: expected %q, got %q", original.Spec.RiskTier, decoded.Spec.RiskTier)
	}
	if decoded.Metadata.Labels["team"] != "payments" {
		t.Errorf("labels: expected team=payments, got %q", decoded.Metadata.Labels["team"])
	}
}

func TestSurfaceDocument_YAMLRoundTrip(t *testing.T) {
	original := types.SurfaceDocument{
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
	}

	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal YAML: %v", err)
	}

	var decoded types.SurfaceDocument
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if decoded.Metadata.ID != original.Metadata.ID {
		t.Errorf("expected ID %q, got %q", original.Metadata.ID, decoded.Metadata.ID)
	}
	if decoded.Spec.Status != original.Spec.Status {
		t.Errorf("expected status %q, got %q", original.Spec.Status, decoded.Spec.Status)
	}
}

func TestSurfaceDocument_GetKindAndID(t *testing.T) {
	doc := types.SurfaceDocument{
		Kind: types.KindSurface,
		Metadata: types.DocumentMetadata{
			ID: "payment.execute",
		},
	}

	if doc.GetKind() != types.KindSurface {
		t.Errorf("expected kind %q, got %q", types.KindSurface, doc.GetKind())
	}
	if doc.GetID() != "payment.execute" {
		t.Errorf("expected id %q, got %q", "payment.execute", doc.GetID())
	}
}

// ---------------------------------------------------------------------------
// Agent Document Tests
// ---------------------------------------------------------------------------

func TestAgentDocument_JSONRoundTrip(t *testing.T) {
	original := types.AgentDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAgent,
		Metadata: types.DocumentMetadata{
			ID:   "agent-credit-scoring-prod",
			Name: "Credit Scoring Agent (Production)",
			Labels: map[string]string{
				"team":       "credit-risk",
				"deployment": "production",
			},
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
	}

	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	var decoded types.AgentDocument
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if decoded.Spec.Type != original.Spec.Type {
		t.Errorf("expected type %q, got %q", original.Spec.Type, decoded.Spec.Type)
	}
	if decoded.Spec.Runtime.Model != original.Spec.Runtime.Model {
		t.Errorf("expected model %q, got %q", original.Spec.Runtime.Model, decoded.Spec.Runtime.Model)
	}
	if decoded.Spec.Status != original.Spec.Status {
		t.Errorf("expected status %q, got %q", original.Spec.Status, decoded.Spec.Status)
	}
}

func TestAgentDocument_YAMLRoundTrip(t *testing.T) {
	original := types.AgentDocument{
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
	}

	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal YAML: %v", err)
	}

	var decoded types.AgentDocument
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if decoded.Metadata.ID != original.Metadata.ID {
		t.Errorf("expected ID %q, got %q", original.Metadata.ID, decoded.Metadata.ID)
	}
	if decoded.Spec.Runtime.Provider != original.Spec.Runtime.Provider {
		t.Errorf("expected provider %q, got %q", original.Spec.Runtime.Provider, decoded.Spec.Runtime.Provider)
	}
}

// ---------------------------------------------------------------------------
// Profile Document Tests
// ---------------------------------------------------------------------------

func TestProfileDocument_YAMLRoundTrip_WithNestedFields(t *testing.T) {
	original := types.ProfileDocument{
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
			Lifecycle: types.ProfileLifecycle{
				Status:        "active",
				EffectiveFrom: "2025-03-01T00:00:00Z",
				Version:       1,
			},
		},
	}

	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal YAML: %v", err)
	}

	var decoded types.ProfileDocument
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if decoded.Spec.SurfaceID != "payment.execute" {
		t.Errorf("expected surface_id %q, got %q", "payment.execute", decoded.Spec.SurfaceID)
	}
	if decoded.Spec.Authority.DecisionConfidenceThreshold != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", decoded.Spec.Authority.DecisionConfidenceThreshold)
	}
	if decoded.Spec.Authority.ConsequenceThreshold.Type != "monetary" {
		t.Errorf("expected consequence type %q, got %q", "monetary", decoded.Spec.Authority.ConsequenceThreshold.Type)
	}
	if decoded.Spec.Authority.ConsequenceThreshold.Amount != 10000 {
		t.Errorf("expected amount 10000, got %f", decoded.Spec.Authority.ConsequenceThreshold.Amount)
	}
	if decoded.Spec.Authority.ConsequenceThreshold.Currency != "USD" {
		t.Errorf("expected currency USD, got %q", decoded.Spec.Authority.ConsequenceThreshold.Currency)
	}
	if len(decoded.Spec.InputRequirements.RequiredContext) != 2 {
		t.Errorf("expected 2 required context keys, got %d", len(decoded.Spec.InputRequirements.RequiredContext))
	}
	if decoded.Spec.Policy.Reference != "rego://payments/auto_approve_v1" {
		t.Errorf("expected policy reference, got %q", decoded.Spec.Policy.Reference)
	}
	if decoded.Spec.Policy.FailMode != "closed" {
		t.Errorf("expected fail mode %q, got %q", "closed", decoded.Spec.Policy.FailMode)
	}
	if decoded.Spec.Lifecycle.Version != 1 {
		t.Errorf("expected version 1, got %d", decoded.Spec.Lifecycle.Version)
	}
}

func TestProfileDocument_RiskRatingConsequenceThreshold(t *testing.T) {
	profile := types.ProfileDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindProfile,
		Metadata: types.DocumentMetadata{
			ID: "data-access-tier-1",
		},
		Spec: types.ProfileSpec{
			SurfaceID: "data.access",
			Authority: types.ProfileAuthority{
				DecisionConfidenceThreshold: 0.90,
				ConsequenceThreshold: types.ConsequenceThreshold{
					Type:       "risk_rating",
					RiskRating: "medium",
				},
			},
		},
	}

	yamlBytes, err := yaml.Marshal(profile)
	if err != nil {
		t.Fatalf("failed to marshal YAML: %v", err)
	}

	var decoded types.ProfileDocument
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if decoded.Spec.Authority.ConsequenceThreshold.Type != "risk_rating" {
		t.Errorf("expected type risk_rating, got %q", decoded.Spec.Authority.ConsequenceThreshold.Type)
	}
	if decoded.Spec.Authority.ConsequenceThreshold.RiskRating != "medium" {
		t.Errorf("expected risk_rating medium, got %q", decoded.Spec.Authority.ConsequenceThreshold.RiskRating)
	}
}

// ---------------------------------------------------------------------------
// Grant Document Tests
// ---------------------------------------------------------------------------

func TestGrantDocument_JSONRoundTrip(t *testing.T) {
	original := types.GrantDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindGrant,
		Metadata: types.DocumentMetadata{
			ID: "grant-credit-scoring-tier-1",
		},
		Spec: types.GrantSpec{
			AgentID:        "agent-credit-scoring-prod",
			ProfileID:      "payments-tier-1",
			GrantedBy:      "admin@example.com",
			GrantedAt:      "2025-03-17T10:00:00Z",
			EffectiveFrom:  "2025-03-17T10:00:00Z",
			EffectiveUntil: "2025-12-31T23:59:59Z",
			Status:         "active",
			Metadata: map[string]string{
				"approval_ticket": "JIRA-1234",
				"rationale":       "Production deployment",
			},
		},
	}

	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	var decoded types.GrantDocument
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if decoded.Spec.AgentID != original.Spec.AgentID {
		t.Errorf("expected agent_id %q, got %q", original.Spec.AgentID, decoded.Spec.AgentID)
	}
	if decoded.Spec.ProfileID != original.Spec.ProfileID {
		t.Errorf("expected profile_id %q, got %q", original.Spec.ProfileID, decoded.Spec.ProfileID)
	}
	if decoded.Spec.GrantedAt != "2025-03-17T10:00:00Z" {
		t.Errorf("timestamp not preserved: got %q", decoded.Spec.GrantedAt)
	}
	if decoded.Spec.Metadata["approval_ticket"] != "JIRA-1234" {
		t.Errorf("expected approval_ticket JIRA-1234, got %q", decoded.Spec.Metadata["approval_ticket"])
	}
}

func TestGrantDocument_YAMLRoundTrip(t *testing.T) {
	original := types.GrantDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindGrant,
		Metadata: types.DocumentMetadata{
			ID: "grant-test",
		},
		Spec: types.GrantSpec{
			AgentID:   "agent-1",
			ProfileID: "profile-1",
			GrantedBy: "user@example.com",
			Status:    "active",
		},
	}

	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal YAML: %v", err)
	}

	var decoded types.GrantDocument
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if decoded.Spec.AgentID != original.Spec.AgentID {
		t.Errorf("expected agent_id %q, got %q", original.Spec.AgentID, decoded.Spec.AgentID)
	}
}

// ---------------------------------------------------------------------------
// Document Interface Tests
// ---------------------------------------------------------------------------

func TestDocumentInterface(t *testing.T) {
	var docs []types.Document

	docs = append(docs, types.SurfaceDocument{
		Kind:     types.KindSurface,
		Metadata: types.DocumentMetadata{ID: "surf-1"},
	})
	docs = append(docs, types.AgentDocument{
		Kind:     types.KindAgent,
		Metadata: types.DocumentMetadata{ID: "agent-1"},
	})
	docs = append(docs, types.ProfileDocument{
		Kind:     types.KindProfile,
		Metadata: types.DocumentMetadata{ID: "prof-1"},
	})
	docs = append(docs, types.GrantDocument{
		Kind:     types.KindGrant,
		Metadata: types.DocumentMetadata{ID: "grant-1"},
	})

	expectedKinds := []string{types.KindSurface, types.KindAgent, types.KindProfile, types.KindGrant}
	expectedIDs := []string{"surf-1", "agent-1", "prof-1", "grant-1"}

	for i, doc := range docs {
		if doc.GetKind() != expectedKinds[i] {
			t.Errorf("doc %d: expected kind %q, got %q", i, expectedKinds[i], doc.GetKind())
		}
		if doc.GetID() != expectedIDs[i] {
			t.Errorf("doc %d: expected id %q, got %q", i, expectedIDs[i], doc.GetID())
		}
	}
}

// ---------------------------------------------------------------------------
// Kind / Version Constant Tests
// ---------------------------------------------------------------------------

func TestKindConstants(t *testing.T) {
	tests := []struct {
		constant string
		expected string
	}{
		{types.KindSurface, "Surface"},
		{types.KindAgent, "Agent"},
		{types.KindProfile, "Profile"},
		{types.KindGrant, "Grant"},
	}

	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("expected constant %q, got %q", tt.expected, tt.constant)
		}
	}
}

func TestAPIVersionConstant(t *testing.T) {
	if types.APIVersionV1 != "midas.accept.io/v1" {
		t.Errorf("expected APIVersionV1 %q, got %q", "midas.accept.io/v1", types.APIVersionV1)
	}
}
