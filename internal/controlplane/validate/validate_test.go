package validate

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// ============================================================================
// Test Data Fixtures
// ============================================================================
type unsupportedDocument struct{}

func (unsupportedDocument) GetKind() string { return "Unsupported" }
func (unsupportedDocument) GetID() string   { return "test-id" }

var (
	validSurfaceDoc = types.SurfaceDocument{
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
			ProcessID:   "payment.process",
		},
	}

	validAgentDoc = types.AgentDocument{
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

	validProfileDoc = types.ProfileDocument{
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
	}

	validGrantDoc = types.GrantDocument{
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
		},
	}
)

// ============================================================================
// ID Format Validation Tests
// ============================================================================

func TestValidateIDFormat_Valid(t *testing.T) {
	validIDs := []string{
		"payment.execute",
		"agent-1",
		"profile_tier_1",
		"a",
		"0",
		"a0",
		"my-resource.id_v2",
		"payment-execution-v1.2.3",
	}

	for _, id := range validIDs {
		t.Run(id, func(t *testing.T) {
			if err := validateIDFormat(id); err != nil {
				t.Errorf("expected valid ID %q to pass, got error: %v", id, err)
			}
		})
	}
}

func TestValidateIDFormat_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		expectedErr string
	}{
		{
			name:        "leading space",
			id:          " payment.execute",
			expectedErr: "leading or trailing whitespace",
		},
		{
			name:        "trailing space",
			id:          "payment.execute ",
			expectedErr: "leading or trailing whitespace",
		},
		{
			name:        "internal space",
			id:          "payment execute",
			expectedErr: "contains spaces",
		},
		{
			name:        "uppercase letter",
			id:          "Payment.execute",
			expectedErr: "must start with lowercase",
		},
		{
			name:        "special character at start",
			id:          "-payment",
			expectedErr: "must start with lowercase",
		},
		{
			name:        "invalid character",
			id:          "payment@execute",
			expectedErr: "must start with lowercase",
		},
		{
			name:        "whitespace only",
			id:          "   ",
			expectedErr: "leading or trailing whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIDFormat(tt.id)
			if err == nil {
				t.Fatalf("expected error for ID %q, got nil", tt.id)
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("expected error containing %q, got: %v", tt.expectedErr, err)
			}
		})
	}
}

func TestValidateIDFormat_MaxLength(t *testing.T) {
	validID := strings.Repeat("a", MaxIDLength)
	if err := validateIDFormat(validID); err != nil {
		t.Errorf("expected ID of length %d to pass, got error: %v", MaxIDLength, err)
	}

	tooLongID := strings.Repeat("a", MaxIDLength+1)
	err := validateIDFormat(tooLongID)
	if err == nil {
		t.Fatal("expected error for ID exceeding max length, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Errorf("expected max length error, got: %v", err)
	}
}

// ============================================================================
// Document Identity Validation Tests
// ============================================================================

func TestValidateIdentity_MissingID(t *testing.T) {
	docWithoutID := validSurfaceDoc
	docWithoutID.Metadata.ID = ""

	doc := parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   "",
		Doc:  docWithoutID,
	}

	errs := validateIdentity(doc)
	if len(errs) == 0 {
		t.Fatal("expected validation error for missing ID, got none")
	}

	found := false
	for _, err := range errs {
		if err.Field == "metadata.id" && strings.Contains(err.Message, "required") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about missing metadata.id, got: %v", errs)
	}
}

func TestValidateIdentity_InvalidIDFormat(t *testing.T) {
	docWithBadID := validSurfaceDoc
	docWithBadID.Metadata.ID = " payment.execute"

	doc := parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   " payment.execute",
		Doc:  docWithBadID,
	}

	errs := validateIdentity(doc)
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid ID format, got none")
	}

	found := false
	for _, err := range errs {
		if err.Field == "metadata.id" && strings.Contains(err.Message, "invalid id format") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about invalid ID format, got: %v", errs)
	}
}

// ============================================================================
// Surface Validation Tests
// ============================================================================

func TestValidateSurface_Valid(t *testing.T) {
	errs := validateSurface(validSurfaceDoc)
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid Surface, got: %v", errs)
	}
}

func TestValidateSurface_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.SurfaceDocument) types.SurfaceDocument
		expectedField string
	}{
		{
			name: "missing name",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Metadata.Name = ""
				return doc
			},
			expectedField: "metadata.name",
		},
		{
			name: "missing category",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Spec.Category = ""
				return doc
			},
			expectedField: "spec.category",
		},
		{
			name: "missing risk_tier",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Spec.RiskTier = ""
				return doc
			},
			expectedField: "spec.risk_tier",
		},
		{
			name: "missing status",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Spec.Status = ""
				return doc
			},
			expectedField: "spec.status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validSurfaceDoc)
			errs := validateSurface(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "required") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error for field %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateSurface_InvalidEnumValues(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.SurfaceDocument) types.SurfaceDocument
		expectedField string
	}{
		{
			name: "invalid risk_tier",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Spec.RiskTier = "critical"
				return doc
			},
			expectedField: "spec.risk_tier",
		},
		{
			name: "invalid status",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Spec.Status = "enabled"
				return doc
			},
			expectedField: "spec.status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validSurfaceDoc)
			errs := validateSurface(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "invalid value") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected enum error for field %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateSurface_FieldLengthLimits(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.SurfaceDocument) types.SurfaceDocument
		expectedField string
	}{
		{
			name: "name exceeds max length",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Metadata.Name = strings.Repeat("a", MaxNameLength+1)
				return doc
			},
			expectedField: "metadata.name",
		},
		{
			name: "category exceeds max length",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Spec.Category = strings.Repeat("a", MaxFieldLength+1)
				return doc
			},
			expectedField: "spec.category",
		},
		{
			name: "description exceeds max length",
			modify: func(doc types.SurfaceDocument) types.SurfaceDocument {
				doc.Spec.Description = strings.Repeat("a", MaxFieldLength+1)
				return doc
			},
			expectedField: "spec.description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validSurfaceDoc)
			errs := validateSurface(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error for exceeding field length, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "exceeds maximum length") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected length error for %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateSurface_ProcessID(t *testing.T) {
	tests := []struct {
		name      string
		processID string
		wantErr   bool
	}{
		{"absent is invalid", "", true},
		{"valid id", "payments.limits-v1", false},
		{"invalid id with spaces", "invalid id", true},
		{"invalid id uppercase", "InvalidID", true},
		{"invalid id leading hyphen", "-bad", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := validSurfaceDoc
			doc.Spec.ProcessID = tt.processID
			errs := validateSurface(doc)

			hasProcessErr := false
			for _, e := range errs {
				if e.Field == "spec.process_id" {
					hasProcessErr = true
					break
				}
			}
			if tt.wantErr && !hasProcessErr {
				t.Errorf("want process_id error for %q, got none", tt.processID)
			}
			if !tt.wantErr && hasProcessErr {
				t.Errorf("want no process_id error for %q, got one", tt.processID)
			}
		})
	}
}

// ============================================================================
// Agent Validation Tests
// ============================================================================

func TestValidateAgent_Valid(t *testing.T) {
	errs := validateAgent(validAgentDoc)
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid Agent, got: %v", errs)
	}
}

func TestValidateAgent_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.AgentDocument) types.AgentDocument
		expectedField string
	}{
		{
			name: "missing name",
			modify: func(doc types.AgentDocument) types.AgentDocument {
				doc.Metadata.Name = ""
				return doc
			},
			expectedField: "metadata.name",
		},
		{
			name: "missing type",
			modify: func(doc types.AgentDocument) types.AgentDocument {
				doc.Spec.Type = ""
				return doc
			},
			expectedField: "spec.type",
		},
		{
			name: "missing runtime.model for llm_agent",
			modify: func(doc types.AgentDocument) types.AgentDocument {
				doc.Spec.Type = "llm_agent"
				doc.Spec.Runtime.Model = ""
				return doc
			},
			expectedField: "spec.runtime.model",
		},
		{
			name: "missing runtime.provider for llm_agent",
			modify: func(doc types.AgentDocument) types.AgentDocument {
				doc.Spec.Type = "llm_agent"
				doc.Spec.Runtime.Provider = ""
				return doc
			},
			expectedField: "spec.runtime.provider",
		},
		{
			name: "missing status",
			modify: func(doc types.AgentDocument) types.AgentDocument {
				doc.Spec.Status = ""
				return doc
			},
			expectedField: "spec.status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validAgentDoc)
			errs := validateAgent(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "required") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error for field %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateAgent_InvalidEnumValues(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.AgentDocument) types.AgentDocument
		expectedField string
	}{
		{
			name: "invalid type",
			modify: func(doc types.AgentDocument) types.AgentDocument {
				doc.Spec.Type = "quantum_agent"
				return doc
			},
			expectedField: "spec.type",
		},
		{
			name: "invalid status",
			modify: func(doc types.AgentDocument) types.AgentDocument {
				doc.Spec.Status = "running"
				return doc
			},
			expectedField: "spec.status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validAgentDoc)
			errs := validateAgent(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "invalid value") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected enum error for field %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateAgent_NonLLMAgentDoesNotRequireRuntime(t *testing.T) {
	doc := validAgentDoc
	doc.Spec.Type = "workflow"
	doc.Spec.Runtime = types.AgentRuntime{}

	errs := validateAgent(doc)

	for _, err := range errs {
		if strings.Contains(err.Field, "runtime") {
			t.Errorf("workflow agent should not require runtime fields, got error: %v", err)
		}
	}
}

// ============================================================================
// Profile Validation Tests
// ============================================================================

func TestValidateProfile_Valid(t *testing.T) {
	errs := validateProfile(validProfileDoc)
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid Profile, got: %v", errs)
	}
}

func TestValidateProfile_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.ProfileDocument) types.ProfileDocument
		expectedField string
	}{
		{
			name: "missing name",
			modify: func(doc types.ProfileDocument) types.ProfileDocument {
				doc.Metadata.Name = ""
				return doc
			},
			expectedField: "metadata.name",
		},
		{
			name: "missing surface_id",
			modify: func(doc types.ProfileDocument) types.ProfileDocument {
				doc.Spec.SurfaceID = ""
				return doc
			},
			expectedField: "spec.surface_id",
		},
		{
			name: "missing policy.reference",
			modify: func(doc types.ProfileDocument) types.ProfileDocument {
				doc.Spec.Policy.Reference = ""
				return doc
			},
			expectedField: "spec.policy.reference",
		},
		{
			name: "missing policy.fail_mode",
			modify: func(doc types.ProfileDocument) types.ProfileDocument {
				doc.Spec.Policy.FailMode = ""
				return doc
			},
			expectedField: "spec.policy.fail_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validProfileDoc)
			errs := validateProfile(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "required") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error for field %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateProfile_InvalidSurfaceIDFormat(t *testing.T) {
	doc := validProfileDoc
	doc.Spec.SurfaceID = " invalid id "

	errs := validateProfile(doc)
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid surface_id format, got none")
	}

	found := false
	for _, err := range errs {
		if err.Field == "spec.surface_id" && strings.Contains(err.Message, "invalid format") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected format error for spec.surface_id, got: %v", errs)
	}
}

func TestValidateProfile_ConfidenceThresholdRange(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		shouldErr bool
	}{
		{"valid 0.0", 0.0, false},
		{"valid 0.5", 0.5, false},
		{"valid 1.0", 1.0, false},
		{"invalid -0.1", -0.1, true},
		{"invalid 1.1", 1.1, true},
		{"invalid -1.0", -1.0, true},
		{"invalid 2.0", 2.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := validProfileDoc
			doc.Spec.Authority.DecisionConfidenceThreshold = tt.threshold

			errs := validateProfile(doc)
			hasError := false
			for _, err := range errs {
				if err.Field == "spec.authority.decision_confidence_threshold" {
					hasError = true
					break
				}
			}

			if hasError != tt.shouldErr {
				t.Errorf("threshold %.2f: expected error=%v, got error=%v (errors: %v)",
					tt.threshold, tt.shouldErr, hasError, errs)
			}
		})
	}
}

func TestValidateProfile_ConsequenceThreshold_Monetary(t *testing.T) {
	t.Run("valid monetary threshold", func(t *testing.T) {
		doc := validProfileDoc
		doc.Spec.Authority.ConsequenceThreshold = types.ConsequenceThreshold{
			Type:     "monetary",
			Amount:   10000,
			Currency: "USD",
		}

		errs := validateProfile(doc)
		for _, err := range errs {
			if strings.Contains(err.Field, "consequence_threshold") {
				t.Errorf("valid monetary threshold should not error, got: %v", err)
			}
		}
	})

	t.Run("negative monetary amount", func(t *testing.T) {
		doc := validProfileDoc
		doc.Spec.Authority.ConsequenceThreshold = types.ConsequenceThreshold{
			Type:     "monetary",
			Amount:   -100,
			Currency: "USD",
		}

		errs := validateProfile(doc)
		if len(errs) == 0 {
			t.Fatal("expected validation error for negative amount, got none")
		}

		found := false
		for _, err := range errs {
			if err.Field == "spec.authority.consequence_threshold.amount" && strings.Contains(err.Message, "non-negative") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for negative amount, got: %v", errs)
		}
	})

	t.Run("missing currency for monetary type", func(t *testing.T) {
		doc := validProfileDoc
		doc.Spec.Authority.ConsequenceThreshold = types.ConsequenceThreshold{
			Type:     "monetary",
			Amount:   10000,
			Currency: "",
		}

		errs := validateProfile(doc)
		if len(errs) == 0 {
			t.Fatal("expected validation error for missing currency, got none")
		}

		found := false
		for _, err := range errs {
			if err.Field == "spec.authority.consequence_threshold.currency" && strings.Contains(err.Message, "required") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for missing currency, got: %v", errs)
		}
	})
}

func TestValidateProfile_ConsequenceThreshold_RiskRating(t *testing.T) {
	t.Run("valid risk_rating threshold", func(t *testing.T) {
		doc := validProfileDoc
		doc.Spec.Authority.ConsequenceThreshold = types.ConsequenceThreshold{
			Type:       "risk_rating",
			RiskRating: "high",
		}

		errs := validateProfile(doc)
		for _, err := range errs {
			if strings.Contains(err.Field, "consequence_threshold") {
				t.Errorf("valid risk_rating threshold should not error, got: %v", err)
			}
		}
	})

	t.Run("missing risk_rating for risk_rating type", func(t *testing.T) {
		doc := validProfileDoc
		doc.Spec.Authority.ConsequenceThreshold = types.ConsequenceThreshold{
			Type:       "risk_rating",
			RiskRating: "",
		}

		errs := validateProfile(doc)
		if len(errs) == 0 {
			t.Fatal("expected validation error for missing risk_rating, got none")
		}

		found := false
		for _, err := range errs {
			if err.Field == "spec.authority.consequence_threshold.risk_rating" && strings.Contains(err.Message, "required") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for missing risk_rating, got: %v", errs)
		}
	})
}

func TestValidateProfile_InvalidConsequenceThresholdType(t *testing.T) {
	doc := validProfileDoc
	doc.Spec.Authority.ConsequenceThreshold = types.ConsequenceThreshold{
		Type: "invalid_type",
	}

	errs := validateProfile(doc)
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid consequence threshold type, got none")
	}

	found := false
	for _, err := range errs {
		if err.Field == "spec.authority.consequence_threshold.type" && strings.Contains(err.Message, "invalid value") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected enum error for consequence_threshold.type, got: %v", errs)
	}
}

func TestValidateProfile_EmptyRequiredContext(t *testing.T) {
	doc := validProfileDoc
	doc.Spec.InputRequirements.RequiredContext = []string{"customer_id", "", "transaction_id"}

	errs := validateProfile(doc)
	if len(errs) == 0 {
		t.Fatal("expected validation error for empty context key, got none")
	}

	found := false
	for _, err := range errs {
		if strings.Contains(err.Field, "required_context") && strings.Contains(err.Message, "cannot be empty") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error for empty context key, got: %v", errs)
	}
}

// ============================================================================
// Grant Validation Tests
// ============================================================================

func TestValidateGrant_Valid(t *testing.T) {
	errs := validateGrant(validGrantDoc)
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid Grant, got: %v", errs)
	}
}

func TestValidateGrant_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.GrantDocument) types.GrantDocument
		expectedField string
	}{
		{
			name: "missing agent_id",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.AgentID = ""
				return doc
			},
			expectedField: "spec.agent_id",
		},
		{
			name: "missing profile_id",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.ProfileID = ""
				return doc
			},
			expectedField: "spec.profile_id",
		},
		{
			name: "missing granted_by",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.GrantedBy = ""
				return doc
			},
			expectedField: "spec.granted_by",
		},
		{
			name: "missing granted_at",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.GrantedAt = ""
				return doc
			},
			expectedField: "spec.granted_at",
		},
		{
			name: "missing effective_from",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.EffectiveFrom = ""
				return doc
			},
			expectedField: "spec.effective_from",
		},
		{
			name: "missing status",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.Status = ""
				return doc
			},
			expectedField: "spec.status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validGrantDoc)
			errs := validateGrant(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "required") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error for field %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateGrant_InvalidIDFormats(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.GrantDocument) types.GrantDocument
		expectedField string
	}{
		{
			name: "invalid agent_id format",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.AgentID = "Agent 1"
				return doc
			},
			expectedField: "spec.agent_id",
		},
		{
			name: "invalid profile_id format",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.ProfileID = "profile/tier-1"
				return doc
			},
			expectedField: "spec.profile_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validGrantDoc)
			errs := validateGrant(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "invalid format") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected format error for field %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateGrant_InvalidTimestampFormat(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.GrantDocument) types.GrantDocument
		expectedField string
	}{
		{
			name: "invalid granted_at format",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.GrantedAt = "2025-03-17"
				return doc
			},
			expectedField: "spec.granted_at",
		},
		{
			name: "invalid effective_from format",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.EffectiveFrom = "not-a-timestamp"
				return doc
			},
			expectedField: "spec.effective_from",
		},
		{
			name: "invalid effective_until format",
			modify: func(doc types.GrantDocument) types.GrantDocument {
				doc.Spec.EffectiveUntil = "2025/12/31 23:59:59"
				return doc
			},
			expectedField: "spec.effective_until",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validGrantDoc)
			errs := validateGrant(doc)

			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}

			found := false
			for _, err := range errs {
				if err.Field == tt.expectedField && strings.Contains(err.Message, "RFC3339") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected RFC3339 error for field %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateGrant_TemporalConsistency(t *testing.T) {
	t.Run("effective_from after effective_until", func(t *testing.T) {
		doc := validGrantDoc
		doc.Spec.EffectiveFrom = "2025-12-31T23:59:59Z"
		doc.Spec.EffectiveUntil = "2025-03-17T10:00:00Z"

		errs := validateGrant(doc)
		if len(errs) == 0 {
			t.Fatal("expected validation error for effective_from > effective_until, got none")
		}

		found := false
		for _, err := range errs {
			if err.Field == "spec.effective_from" && strings.Contains(err.Message, "before or equal to") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected temporal error, got: %v", errs)
		}
	})

	t.Run("granted_at after effective_from", func(t *testing.T) {
		doc := validGrantDoc
		doc.Spec.GrantedAt = "2025-12-01T00:00:00Z"
		doc.Spec.EffectiveFrom = "2025-03-17T10:00:00Z"

		errs := validateGrant(doc)
		if len(errs) == 0 {
			t.Fatal("expected validation error for granted_at > effective_from, got none")
		}

		found := false
		for _, err := range errs {
			if err.Field == "spec.granted_at" && strings.Contains(err.Message, "before or equal to") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected temporal error, got: %v", errs)
		}
	})

	t.Run("equal timestamps are valid", func(t *testing.T) {
		doc := validGrantDoc
		timestamp := "2025-03-17T10:00:00Z"
		doc.Spec.GrantedAt = timestamp
		doc.Spec.EffectiveFrom = timestamp
		doc.Spec.EffectiveUntil = timestamp

		errs := validateGrant(doc)
		for _, err := range errs {
			if strings.Contains(err.Field, "granted_at") || strings.Contains(err.Field, "effective") {
				if strings.Contains(err.Message, "before or equal to") {
					t.Errorf("equal timestamps should be valid, got error: %v", err)
				}
			}
		}
	})
}

func TestValidateGrant_InvalidStatus(t *testing.T) {
	doc := validGrantDoc
	doc.Spec.Status = "pending"

	errs := validateGrant(doc)
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid status, got none")
	}

	found := false
	for _, err := range errs {
		if err.Field == "spec.status" && strings.Contains(err.Message, "invalid value") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected enum error for status, got: %v", errs)
	}
}

// ============================================================================
// ValidateDocument Integration Tests
// ============================================================================

func TestValidateDocument_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		doc  parser.ParsedDocument
	}{
		{
			name: "Surface",
			doc: parser.ParsedDocument{
				Kind: types.KindSurface,
				ID:   validSurfaceDoc.Metadata.ID,
				Doc:  validSurfaceDoc,
			},
		},
		{
			name: "Agent",
			doc: parser.ParsedDocument{
				Kind: types.KindAgent,
				ID:   validAgentDoc.Metadata.ID,
				Doc:  validAgentDoc,
			},
		},
		{
			name: "Profile",
			doc: parser.ParsedDocument{
				Kind: types.KindProfile,
				ID:   validProfileDoc.Metadata.ID,
				Doc:  validProfileDoc,
			},
		},
		{
			name: "Grant",
			doc: parser.ParsedDocument{
				Kind: types.KindGrant,
				ID:   validGrantDoc.Metadata.ID,
				Doc:  validGrantDoc,
			},
		},
		{
			name: "BusinessService",
			doc: parser.ParsedDocument{
				Kind: types.KindBusinessService,
				ID:   "bs-consumer-lending",
				Doc: types.BusinessServiceDocument{
					APIVersion: types.APIVersionV1,
					Kind:       types.KindBusinessService,
					Metadata:   types.DocumentMetadata{ID: "bs-consumer-lending", Name: "Consumer Lending"},
					Spec:       types.BusinessServiceSpec{ServiceType: "customer_facing", Status: "active"},
				},
			},
		},
		{
			name: "BusinessServiceCapability",
			doc: parser.ParsedDocument{
				Kind: types.KindBusinessServiceCapability,
				ID:   "bsc-lending-fraud",
				Doc: types.BusinessServiceCapabilityDocument{
					APIVersion: types.APIVersionV1,
					Kind:       types.KindBusinessServiceCapability,
					Metadata:   types.DocumentMetadata{ID: "bsc-lending-fraud"},
					Spec: types.BusinessServiceCapabilitySpec{
						BusinessServiceID: "bs-consumer-lending",
						CapabilityID:      "cap-fraud-detection",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateDocument(tt.doc)
			if len(errs) > 0 {
				t.Errorf("expected no errors for valid %s document, got: %v", tt.name, errs)
			}
		})
	}
}

// ============================================================================
// BusinessService Validation Tests
// ============================================================================

func TestValidateBusinessService_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.BusinessServiceDocument) types.BusinessServiceDocument
		expectedField string
	}{
		{
			name: "missing name",
			modify: func(d types.BusinessServiceDocument) types.BusinessServiceDocument {
				d.Metadata.Name = ""
				return d
			},
			expectedField: "metadata.name",
		},
		{
			name: "missing service_type",
			modify: func(d types.BusinessServiceDocument) types.BusinessServiceDocument {
				d.Spec.ServiceType = ""
				return d
			},
			expectedField: "spec.service_type",
		},
		{
			name: "missing status",
			modify: func(d types.BusinessServiceDocument) types.BusinessServiceDocument {
				d.Spec.Status = ""
				return d
			},
			expectedField: "spec.status",
		},
	}

	base := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindBusinessService,
		Metadata:   types.DocumentMetadata{ID: "bs-test", Name: "Test Service"},
		Spec:       types.BusinessServiceSpec{ServiceType: "customer_facing", Status: "active"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(base)
			errs := validateBusinessService(doc)
			if len(errs) == 0 {
				t.Fatalf("expected validation error for %s, got none", tt.name)
			}
			found := false
			for _, e := range errs {
				if e.Field == tt.expectedField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error on %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateBusinessService_InvalidEnumValues(t *testing.T) {
	base := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindBusinessService,
		Metadata:   types.DocumentMetadata{ID: "bs-test", Name: "Test Service"},
		Spec:       types.BusinessServiceSpec{ServiceType: "customer_facing", Status: "active"},
	}

	tests := []struct {
		name          string
		modify        func(types.BusinessServiceDocument) types.BusinessServiceDocument
		expectedField string
	}{
		{
			name: "invalid service_type",
			modify: func(d types.BusinessServiceDocument) types.BusinessServiceDocument {
				d.Spec.ServiceType = "unknown_type"
				return d
			},
			expectedField: "spec.service_type",
		},
		{
			name: "invalid status",
			modify: func(d types.BusinessServiceDocument) types.BusinessServiceDocument {
				d.Spec.Status = "unknown_status"
				return d
			},
			expectedField: "spec.status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(base)
			errs := validateBusinessService(doc)
			if len(errs) == 0 {
				t.Fatalf("expected validation error for %s, got none", tt.name)
			}
			found := false
			for _, e := range errs {
				if e.Field == tt.expectedField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected enum error on %q, got: %v", tt.expectedField, errs)
			}
		})
	}
}

// TestValidateBusinessService_InactiveStatusRejected confirms that 'inactive' is not
// a valid BusinessService status. The business_services schema CHECK constraint allows
// only ('active', 'deprecated') — 'inactive' is valid for Capability and Process but
// not BusinessService, and must be caught here before any DB write occurs.
func TestValidateBusinessService_InactiveStatusRejected(t *testing.T) {
	doc := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindBusinessService,
		Metadata:   types.DocumentMetadata{ID: "bs-test", Name: "Test Service"},
		Spec:       types.BusinessServiceSpec{ServiceType: "customer_facing", Status: "inactive"},
	}
	errs := validateBusinessService(doc)
	if len(errs) == 0 {
		t.Fatal("expected validation error for status=inactive on BusinessService, got none")
	}
	found := false
	for _, e := range errs {
		if e.Field == "spec.status" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error on spec.status; got: %v", errs)
	}
}

// TestValidateBusinessService_ValidStatuses confirms 'active' and 'deprecated' pass.
func TestValidateBusinessService_ValidStatuses(t *testing.T) {
	base := types.BusinessServiceDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindBusinessService,
		Metadata:   types.DocumentMetadata{ID: "bs-test", Name: "Test Service"},
		Spec:       types.BusinessServiceSpec{ServiceType: "customer_facing", Status: "active"},
	}
	for _, status := range []string{"active", "deprecated"} {
		doc := base
		doc.Spec.Status = status
		errs := validateBusinessService(doc)
		for _, e := range errs {
			if e.Field == "spec.status" {
				t.Errorf("status=%q should be valid for BusinessService, got error: %s", status, e.Message)
			}
		}
	}
}

// ============================================================================
// BusinessServiceCapability Validation Tests
// ============================================================================

// validBSCDoc is the canonical valid BusinessServiceCapability used as the
// base for negative-case mutations. Both ID fields conform to validateIDFormat.
var validBSCDoc = types.BusinessServiceCapabilityDocument{
	APIVersion: types.APIVersionV1,
	Kind:       types.KindBusinessServiceCapability,
	Metadata:   types.DocumentMetadata{ID: "bsc-test"},
	Spec: types.BusinessServiceCapabilitySpec{
		BusinessServiceID: "bs-test",
		CapabilityID:      "cap-test",
	},
}

func TestValidateBusinessServiceCapability_Valid(t *testing.T) {
	errs := validateBusinessServiceCapability(validBSCDoc)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid BSC, got: %+v", errs)
	}
}

func TestValidateBusinessServiceCapability_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.BusinessServiceCapabilityDocument) types.BusinessServiceCapabilityDocument
		expectedField string
	}{
		{
			name: "missing business_service_id",
			modify: func(d types.BusinessServiceCapabilityDocument) types.BusinessServiceCapabilityDocument {
				d.Spec.BusinessServiceID = ""
				return d
			},
			expectedField: "spec.business_service_id",
		},
		{
			name: "whitespace-only business_service_id",
			modify: func(d types.BusinessServiceCapabilityDocument) types.BusinessServiceCapabilityDocument {
				d.Spec.BusinessServiceID = "   "
				return d
			},
			expectedField: "spec.business_service_id",
		},
		{
			name: "missing capability_id",
			modify: func(d types.BusinessServiceCapabilityDocument) types.BusinessServiceCapabilityDocument {
				d.Spec.CapabilityID = ""
				return d
			},
			expectedField: "spec.capability_id",
		},
		{
			name: "whitespace-only capability_id",
			modify: func(d types.BusinessServiceCapabilityDocument) types.BusinessServiceCapabilityDocument {
				d.Spec.CapabilityID = "   "
				return d
			},
			expectedField: "spec.capability_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validBSCDoc)
			errs := validateBusinessServiceCapability(doc)
			if len(errs) == 0 {
				t.Fatalf("expected validation error for %s, got none", tt.name)
			}
			found := false
			for _, e := range errs {
				if e.Field == tt.expectedField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error on %q, got: %+v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateBusinessServiceCapability_InvalidIDFormat(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(types.BusinessServiceCapabilityDocument) types.BusinessServiceCapabilityDocument
		expectedField string
	}{
		{
			name: "business_service_id with uppercase",
			modify: func(d types.BusinessServiceCapabilityDocument) types.BusinessServiceCapabilityDocument {
				d.Spec.BusinessServiceID = "BS-Bad"
				return d
			},
			expectedField: "spec.business_service_id",
		},
		{
			name: "capability_id with space",
			modify: func(d types.BusinessServiceCapabilityDocument) types.BusinessServiceCapabilityDocument {
				d.Spec.CapabilityID = "cap bad"
				return d
			},
			expectedField: "spec.capability_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.modify(validBSCDoc)
			errs := validateBusinessServiceCapability(doc)
			if len(errs) == 0 {
				t.Fatalf("expected validation error for %s, got none", tt.name)
			}
			found := false
			for _, e := range errs {
				if e.Field == tt.expectedField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected ID-format error on %q, got: %+v", tt.expectedField, errs)
			}
		})
	}
}

func TestValidateDocument_UnsupportedType(t *testing.T) {
	doc := parser.ParsedDocument{
		Kind: "Unsupported",
		ID:   "test-id",
		Doc:  unsupportedDocument{},
	}

	errs := ValidateDocument(doc)
	if len(errs) == 0 {
		t.Fatal("expected error for unsupported document type, got none")
	}

	found := false
	for _, err := range errs {
		if strings.Contains(err.Message, "unsupported document type") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unsupported type error, got: %v", errs)
	}
}

// ============================================================================
// ValidateBundle Tests
// ============================================================================

func TestValidateBundle_AllValid(t *testing.T) {
	docs := []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   validSurfaceDoc.Metadata.ID,
			Doc:  validSurfaceDoc,
		},
		{
			Kind: types.KindAgent,
			ID:   validAgentDoc.Metadata.ID,
			Doc:  validAgentDoc,
		},
		{
			Kind: types.KindProfile,
			ID:   validProfileDoc.Metadata.ID,
			Doc:  validProfileDoc,
		},
		{
			Kind: types.KindGrant,
			ID:   validGrantDoc.Metadata.ID,
			Doc:  validGrantDoc,
		},
	}

	errs := ValidateBundle(docs)
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid bundle, got: %v", errs)
	}
}

func TestValidateBundle_DuplicateIDs(t *testing.T) {
	surfaceDoc1 := validSurfaceDoc
	surfaceDoc1.Metadata.ID = "payment.execute"

	surfaceDoc2 := validSurfaceDoc
	surfaceDoc2.Metadata.ID = "payment.execute"

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   "payment.execute",
			Doc:  surfaceDoc1,
		},
		{
			Kind: types.KindSurface,
			ID:   "payment.execute",
			Doc:  surfaceDoc2,
		},
	}

	errs := ValidateBundle(docs)

	duplicateErrors := 0
	for _, err := range errs {
		if err.Field == "metadata.id" && strings.Contains(err.Message, "duplicate") {
			duplicateErrors++
		}
	}

	if duplicateErrors < 2 {
		t.Errorf("expected at least 2 duplicate errors (one for each occurrence), got %d (errors: %v)", duplicateErrors, errs)
	}
}

func TestValidateBundle_DuplicateIDsAcrossDifferentKinds(t *testing.T) {
	surfaceDoc := validSurfaceDoc
	surfaceDoc.Metadata.ID = "shared-id"

	agentDoc := validAgentDoc
	agentDoc.Metadata.ID = "shared-id"

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   "shared-id",
			Doc:  surfaceDoc,
		},
		{
			Kind: types.KindAgent,
			ID:   "shared-id",
			Doc:  agentDoc,
		},
	}

	errs := ValidateBundle(docs)

	for _, err := range errs {
		if err.Field == "metadata.id" && strings.Contains(err.Message, "duplicate") {
			t.Errorf("same ID across different kinds should be allowed, got error: %v", err)
		}
	}
}

func TestValidateBundle_DocumentIndexAnnotation(t *testing.T) {
	invalidSurface := validSurfaceDoc
	invalidSurface.Metadata.ID = "surface-1"
	invalidSurface.Spec.Category = ""

	agentDoc := validAgentDoc
	agentDoc.Metadata.ID = "agent-1"

	profileDoc := validProfileDoc
	profileDoc.Metadata.ID = "profile-1"

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindAgent,
			ID:   "agent-1",
			Doc:  agentDoc,
		},
		{
			Kind: types.KindSurface,
			ID:   "surface-1",
			Doc:  invalidSurface,
		},
		{
			Kind: types.KindProfile,
			ID:   "profile-1",
			Doc:  profileDoc,
		},
	}

	errs := ValidateBundle(docs)

	found := false
	for _, err := range errs {
		if err.Field == "spec.category" && err.DocumentIndex == 2 {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected error annotated with DocumentIndex=2 (1-based), got: %v", errs)
	}
}

func TestValidateBundle_EmptyBundle(t *testing.T) {
	errs := ValidateBundle([]parser.ParsedDocument{})
	if len(errs) > 0 {
		t.Errorf("expected no errors for empty bundle, got: %v", errs)
	}
}

func TestValidateBundle_SkipsEmptyIDsInDuplicateDetection(t *testing.T) {
	invalidDoc1 := validSurfaceDoc
	invalidDoc1.Metadata.ID = ""

	invalidDoc2 := validSurfaceDoc
	invalidDoc2.Metadata.ID = ""

	docs := []parser.ParsedDocument{
		{
			Kind: types.KindSurface,
			ID:   "",
			Doc:  invalidDoc1,
		},
		{
			Kind: types.KindSurface,
			ID:   "",
			Doc:  invalidDoc2,
		},
	}

	errs := ValidateBundle(docs)

	hasRequiredError := false
	hasDuplicateError := false

	for _, err := range errs {
		if err.Field == "metadata.id" && strings.Contains(err.Message, "required") {
			hasRequiredError = true
		}
		if err.Field == "metadata.id" && strings.Contains(err.Message, "duplicate") {
			hasDuplicateError = true
		}
	}

	if !hasRequiredError {
		t.Error("expected 'required' error for empty IDs")
	}
	if hasDuplicateError {
		t.Error("should not report duplicates for empty IDs")
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	if !contains(slice, "banana") {
		t.Error("expected contains to find 'banana'")
	}
	if contains(slice, "orange") {
		t.Error("expected contains to not find 'orange'")
	}
	if contains([]string{}, "anything") {
		t.Error("expected contains to return false for empty slice")
	}
}
