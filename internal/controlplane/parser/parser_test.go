package parser

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/types"
)

// ============================================================================
// Test Data Fixtures
// ============================================================================

const (
	validBusinessServiceYAML = `apiVersion: midas.accept.io/v1
kind: BusinessService
metadata:
  id: bs-consumer-lending
  name: Consumer Lending
spec:
  description: Retail lending products for consumers
  service_type: customer_facing
  status: active`

	validBusinessServiceCapabilityYAML = `apiVersion: midas.accept.io/v1
kind: BusinessServiceCapability
metadata:
  id: bsc-lending-fraud
spec:
  business_service_id: bs-consumer-lending
  capability_id: cap-fraud-detection`

	validBusinessServiceRelationshipYAML = `apiVersion: midas.accept.io/v1
kind: BusinessServiceRelationship
metadata:
  id: rel-mortgage-depends-on-onboarding
spec:
  source_business_service_id: bs-mortgage-origination
  target_business_service_id: bs-customer-onboarding
  relationship_type: depends_on
  description: Mortgage origination depends on customer onboarding.`

	validSurfaceYAML = `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
  name: Payment Execution
spec:
  description: Authorization for executing payment transactions
  category: financial
  risk_tier: high
  status: active`

	validAgentYAML = `apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-credit-scoring-prod
  name: Credit Scoring Agent
spec:
  type: llm_agent
  runtime:
    model: gpt-4
    version: 2024-11-20
    provider: openai
  status: active`

	validProfileYAML = `apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: payments-tier-1
  name: Payments Auto-Approve Tier 1
spec:
  surface_id: payment.execute
  authority:
    decision_confidence_threshold: 0.85
    consequence_threshold:
      type: monetary
      amount: 10000
      currency: USD
  input_requirements:
    required_context:
      - customer_id
      - transaction_id
  policy:
    reference: rego://payments/auto_approve_v1
    fail_mode: closed
  lifecycle:
    status: active
    effective_from: 2025-03-01T00:00:00Z
    version: 1`

	validGrantYAML = `apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-credit-scoring-tier-1
spec:
  agent_id: agent-credit-scoring-prod
  profile_id: payments-tier-1
  granted_by: admin@example.com
  granted_at: 2025-03-17T10:00:00Z
  effective_from: 2025-03-17T10:00:00Z
  effective_until: 2025-12-31T23:59:59Z
  status: active
  metadata:
    approval_ticket: JIRA-1234`
)

// ============================================================================
// Happy Path Tests - Table-Driven
// ============================================================================

func TestParseYAML_AllKinds(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		expectedKind string
		expectedID   string
		validateDoc  func(t *testing.T, doc ParsedDocument)
	}{
		{
			name:         "Surface",
			yaml:         validSurfaceYAML,
			expectedKind: types.KindSurface,
			expectedID:   "payment.execute",
			validateDoc: func(t *testing.T, doc ParsedDocument) {
				surfaceDoc, ok := doc.Doc.(types.SurfaceDocument)
				if !ok {
					t.Fatalf("expected doc type SurfaceDocument, got %T", doc.Doc)
				}
				if surfaceDoc.Spec.Category != "financial" {
					t.Errorf("expected category %q, got %q", "financial", surfaceDoc.Spec.Category)
				}
				if surfaceDoc.Spec.RiskTier != "high" {
					t.Errorf("expected risk_tier %q, got %q", "high", surfaceDoc.Spec.RiskTier)
				}
				if surfaceDoc.Spec.Status != "active" {
					t.Errorf("expected status %q, got %q", "active", surfaceDoc.Spec.Status)
				}
			},
		},
		{
			name:         "Agent",
			yaml:         validAgentYAML,
			expectedKind: types.KindAgent,
			expectedID:   "agent-credit-scoring-prod",
			validateDoc: func(t *testing.T, doc ParsedDocument) {
				agentDoc, ok := doc.Doc.(types.AgentDocument)
				if !ok {
					t.Fatalf("expected doc type AgentDocument, got %T", doc.Doc)
				}
				if agentDoc.Spec.Type != "llm_agent" {
					t.Errorf("expected type %q, got %q", "llm_agent", agentDoc.Spec.Type)
				}
				if agentDoc.Spec.Runtime.Provider != "openai" {
					t.Errorf("expected provider %q, got %q", "openai", agentDoc.Spec.Runtime.Provider)
				}
				if agentDoc.Spec.Runtime.Model != "gpt-4" {
					t.Errorf("expected model %q, got %q", "gpt-4", agentDoc.Spec.Runtime.Model)
				}
			},
		},
		{
			name:         "Profile",
			yaml:         validProfileYAML,
			expectedKind: types.KindProfile,
			expectedID:   "payments-tier-1",
			validateDoc: func(t *testing.T, doc ParsedDocument) {
				profileDoc, ok := doc.Doc.(types.ProfileDocument)
				if !ok {
					t.Fatalf("expected doc type ProfileDocument, got %T", doc.Doc)
				}
				if profileDoc.Spec.SurfaceID != "payment.execute" {
					t.Errorf("expected surface_id %q, got %q", "payment.execute", profileDoc.Spec.SurfaceID)
				}
				if profileDoc.Spec.Authority.DecisionConfidenceThreshold != 0.85 {
					t.Errorf("expected decision_confidence_threshold %v, got %v", 0.85, profileDoc.Spec.Authority.DecisionConfidenceThreshold)
				}
				if profileDoc.Spec.Policy.FailMode != "closed" {
					t.Errorf("expected fail_mode %q, got %q", "closed", profileDoc.Spec.Policy.FailMode)
				}
			},
		},
		{
			name:         "Grant",
			yaml:         validGrantYAML,
			expectedKind: types.KindGrant,
			expectedID:   "grant-credit-scoring-tier-1",
			validateDoc: func(t *testing.T, doc ParsedDocument) {
				grantDoc, ok := doc.Doc.(types.GrantDocument)
				if !ok {
					t.Fatalf("expected doc type GrantDocument, got %T", doc.Doc)
				}
				if grantDoc.Spec.AgentID != "agent-credit-scoring-prod" {
					t.Errorf("expected agent_id %q, got %q", "agent-credit-scoring-prod", grantDoc.Spec.AgentID)
				}
				if grantDoc.Spec.ProfileID != "payments-tier-1" {
					t.Errorf("expected profile_id %q, got %q", "payments-tier-1", grantDoc.Spec.ProfileID)
				}
				if grantDoc.Spec.Status != "active" {
					t.Errorf("expected status %q, got %q", "active", grantDoc.Spec.Status)
				}
			},
		},
		{
			name:         "BusinessService",
			yaml:         validBusinessServiceYAML,
			expectedKind: types.KindBusinessService,
			expectedID:   "bs-consumer-lending",
			validateDoc: func(t *testing.T, doc ParsedDocument) {
				bsDoc, ok := doc.Doc.(types.BusinessServiceDocument)
				if !ok {
					t.Fatalf("expected doc type BusinessServiceDocument, got %T", doc.Doc)
				}
				if bsDoc.Spec.ServiceType != "customer_facing" {
					t.Errorf("expected service_type %q, got %q", "customer_facing", bsDoc.Spec.ServiceType)
				}
				if bsDoc.Spec.Status != "active" {
					t.Errorf("expected status %q, got %q", "active", bsDoc.Spec.Status)
				}
				if bsDoc.Spec.Description != "Retail lending products for consumers" {
					t.Errorf("unexpected description: %q", bsDoc.Spec.Description)
				}
			},
		},
		{
			name:         "BusinessServiceCapability",
			yaml:         validBusinessServiceCapabilityYAML,
			expectedKind: types.KindBusinessServiceCapability,
			expectedID:   "bsc-lending-fraud",
			validateDoc: func(t *testing.T, doc ParsedDocument) {
				bscDoc, ok := doc.Doc.(types.BusinessServiceCapabilityDocument)
				if !ok {
					t.Fatalf("expected doc type BusinessServiceCapabilityDocument, got %T", doc.Doc)
				}
				if bscDoc.Spec.BusinessServiceID != "bs-consumer-lending" {
					t.Errorf("expected business_service_id %q, got %q", "bs-consumer-lending", bscDoc.Spec.BusinessServiceID)
				}
				if bscDoc.Spec.CapabilityID != "cap-fraud-detection" {
					t.Errorf("expected capability_id %q, got %q", "cap-fraud-detection", bscDoc.Spec.CapabilityID)
				}
			},
		},
		{
			name:         "BusinessServiceRelationship",
			yaml:         validBusinessServiceRelationshipYAML,
			expectedKind: types.KindBusinessServiceRelationship,
			expectedID:   "rel-mortgage-depends-on-onboarding",
			validateDoc: func(t *testing.T, doc ParsedDocument) {
				bsrDoc, ok := doc.Doc.(types.BusinessServiceRelationshipDocument)
				if !ok {
					t.Fatalf("expected doc type BusinessServiceRelationshipDocument, got %T", doc.Doc)
				}
				if bsrDoc.Spec.SourceBusinessServiceID != "bs-mortgage-origination" {
					t.Errorf("source: got %q", bsrDoc.Spec.SourceBusinessServiceID)
				}
				if bsrDoc.Spec.TargetBusinessServiceID != "bs-customer-onboarding" {
					t.Errorf("target: got %q", bsrDoc.Spec.TargetBusinessServiceID)
				}
				if bsrDoc.Spec.RelationshipType != "depends_on" {
					t.Errorf("relationship_type: got %q", bsrDoc.Spec.RelationshipType)
				}
				if bsrDoc.Spec.Description == "" {
					t.Error("description should round-trip non-empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if doc.Kind != tt.expectedKind {
				t.Errorf("Kind = %q, want %q", doc.Kind, tt.expectedKind)
			}
			if doc.ID != tt.expectedID {
				t.Errorf("ID = %q, want %q", doc.ID, tt.expectedID)
			}

			if tt.validateDoc != nil {
				tt.validateDoc(t, doc)
			}
		})
	}
}

// ============================================================================
// Document Interface Contract Tests
// ============================================================================

func TestParsedDocument_InterfaceContracts(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		expectedKind string
		expectedID   string
	}{
		{"Surface", validSurfaceYAML, types.KindSurface, "payment.execute"},
		{"Agent", validAgentYAML, types.KindAgent, "agent-credit-scoring-prod"},
		{"Profile", validProfileYAML, types.KindProfile, "payments-tier-1"},
		{"Grant", validGrantYAML, types.KindGrant, "grant-credit-scoring-tier-1"},
		{"BusinessService", validBusinessServiceYAML, types.KindBusinessService, "bs-consumer-lending"},
		{"BusinessServiceCapability", validBusinessServiceCapabilityYAML, types.KindBusinessServiceCapability, "bsc-lending-fraud"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if parsed.Doc.GetKind() != tt.expectedKind {
				t.Errorf("GetKind() = %q, want %q", parsed.Doc.GetKind(), tt.expectedKind)
			}
			if parsed.Doc.GetID() != tt.expectedID {
				t.Errorf("GetID() = %q, want %q", parsed.Doc.GetID(), tt.expectedID)
			}
			if parsed.Kind != parsed.Doc.GetKind() {
				t.Errorf("wrapper Kind %q != Doc.GetKind() %q", parsed.Kind, parsed.Doc.GetKind())
			}
			if parsed.ID != parsed.Doc.GetID() {
				t.Errorf("wrapper ID %q != Doc.GetID() %q", parsed.ID, parsed.Doc.GetID())
			}
		})
	}
}

// ============================================================================
// Current Behavior Documentation - Validation Deferred
// ============================================================================

func TestParseYAML_MissingID_CurrentBehavior(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "Surface without ID",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  name: Payment Execution
spec:
  category: financial`,
		},
		{
			name: "Agent without ID",
			yaml: `apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  name: Credit Scoring Agent
spec:
  type: llm_agent`,
		},
		{
			name: "Profile without ID",
			yaml: `apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  name: Payments Tier 1
spec:
  surface_id: payment.execute`,
		},
		{
			name: "Grant without ID",
			yaml: `apiVersion: midas.accept.io/v1
kind: Grant
spec:
  agent_id: agent-1
  profile_id: profile-1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("unexpected error; parser currently does not validate metadata.id presence: %v", err)
			}
			if doc.ID != "" {
				t.Errorf("expected empty ID when metadata.id is missing, got %q", doc.ID)
			}
		})
	}
}

func TestParseYAML_EmptyID_CurrentBehavior(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: ""
  name: Payment Execution
spec:
  category: financial`)

	doc, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.ID != "" {
		t.Errorf("expected empty ID to be preserved, got %q", doc.ID)
	}
}

func TestParseYAML_IDWithWhitespace_CurrentBehavior(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "leading space",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: " payment.execute"
  name: Payment Execution
spec:
  category: financial`,
			want: " payment.execute",
		},
		{
			name: "trailing space",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: "payment.execute "
  name: Payment Execution
spec:
  category: financial`,
			want: "payment.execute ",
		},
		{
			name: "internal spaces",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: "payment execute"
  name: Payment Execution
spec:
  category: financial`,
			want: "payment execute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if doc.ID != tt.want {
				t.Errorf("ID = %q, want %q", doc.ID, tt.want)
			}
		})
	}
}

func TestParseYAML_MissingMetadataBlock_CurrentBehavior(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
kind: Surface
spec:
  category: financial`)

	doc, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.ID != "" {
		t.Errorf("expected empty ID when metadata block is missing, got %q", doc.ID)
	}
}

// ============================================================================
// Error Path Tests - API Version and Kind
// ============================================================================

func TestParseYAML_MissingAPIVersion(t *testing.T) {
	data := []byte(`kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial`)

	_, err := ParseYAML(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing apiVersion") {
		t.Errorf("expected missing apiVersion error, got %v", err)
	}
}

func TestParseYAML_MissingKind(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
metadata:
  id: payment.execute
spec:
  category: financial`)

	_, err := ParseYAML(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing kind") {
		t.Errorf("expected missing kind error, got %v", err)
	}
}

func TestParseYAML_UnsupportedAPIVersion(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
	}{
		{"future version", "midas.accept.io/v2"},
		{"past version", "midas.accept.io/v0"},
		{"wrong format", "v1"},
		{"completely wrong", "kubernetes.io/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := `apiVersion: ` + tt.apiVersion + `
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial`

			_, err := ParseYAML([]byte(yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported apiVersion") {
				t.Errorf("expected unsupported apiVersion error, got %v", err)
			}
			if !strings.Contains(err.Error(), tt.apiVersion) {
				t.Errorf("expected error to contain %q, got %v", tt.apiVersion, err)
			}
		})
	}
}

func TestParseYAML_UnsupportedKind(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		{"PolicySet", "PolicySet"},
		{"Deployment", "Deployment"},
		{"lowercase", "surface"},
		{"plural", "Surfaces"},
		{"typo", "Agnet"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := `apiVersion: midas.accept.io/v1
kind: ` + tt.kind + `
metadata:
  id: test-id
spec: {}`

			_, err := ParseYAML([]byte(yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported kind") {
				t.Errorf("expected unsupported kind error, got %v", err)
			}
			if !strings.Contains(err.Error(), tt.kind) {
				t.Errorf("expected error to contain %q, got %v", tt.kind, err)
			}
		})
	}
}

// ============================================================================
// YAML Syntax Error Tests
// ============================================================================

func TestParseYAML_InvalidYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "unclosed array",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: [financial`,
		},
		{
			name: "malformed mapping",
			yaml: `apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial
  risk_tier`,
		},
		{
			name: "broken sequence indentation",
			yaml: `apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: payments-tier-1
spec:
  input_requirements:
    required_context:
      - customer_id
     - transaction_id`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseYAML([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// ============================================================================
// Type-Specific Field Parsing Tests
// ============================================================================

func TestParseYAML_Surface_SpecificFields(t *testing.T) {
	doc, err := ParseYAML([]byte(validSurfaceYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	surfaceDoc, ok := doc.Doc.(types.SurfaceDocument)
	if !ok {
		t.Fatalf("expected SurfaceDocument, got %T", doc.Doc)
	}

	if surfaceDoc.Metadata.Name != "Payment Execution" {
		t.Errorf("metadata.name = %q, want %q", surfaceDoc.Metadata.Name, "Payment Execution")
	}
	if surfaceDoc.Spec.Description != "Authorization for executing payment transactions" {
		t.Errorf("spec.description mismatch")
	}
	if surfaceDoc.Spec.Category != "financial" {
		t.Errorf("spec.category = %q, want %q", surfaceDoc.Spec.Category, "financial")
	}
	if surfaceDoc.Spec.RiskTier != "high" {
		t.Errorf("spec.risk_tier = %q, want %q", surfaceDoc.Spec.RiskTier, "high")
	}
	if surfaceDoc.Spec.Status != "active" {
		t.Errorf("spec.status = %q, want %q", surfaceDoc.Spec.Status, "active")
	}
}

func TestParseYAML_Agent_NestedRuntime(t *testing.T) {
	doc, err := ParseYAML([]byte(validAgentYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agentDoc, ok := doc.Doc.(types.AgentDocument)
	if !ok {
		t.Fatalf("expected AgentDocument, got %T", doc.Doc)
	}

	if agentDoc.Spec.Runtime.Model != "gpt-4" {
		t.Errorf("runtime.model = %q, want %q", agentDoc.Spec.Runtime.Model, "gpt-4")
	}
	if agentDoc.Spec.Runtime.Version != "2024-11-20" {
		t.Errorf("runtime.version = %q, want %q", agentDoc.Spec.Runtime.Version, "2024-11-20")
	}
	if agentDoc.Spec.Runtime.Provider != "openai" {
		t.Errorf("runtime.provider = %q, want %q", agentDoc.Spec.Runtime.Provider, "openai")
	}
}

func TestParseYAML_Profile_ComplexNesting(t *testing.T) {
	doc, err := ParseYAML([]byte(validProfileYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	profileDoc, ok := doc.Doc.(types.ProfileDocument)
	if !ok {
		t.Fatalf("expected ProfileDocument, got %T", doc.Doc)
	}

	if profileDoc.Spec.Authority.DecisionConfidenceThreshold != 0.85 {
		t.Errorf(
			"authority.decision_confidence_threshold = %v, want %v",
			profileDoc.Spec.Authority.DecisionConfidenceThreshold,
			0.85,
		)
	}
	if profileDoc.Spec.Authority.ConsequenceThreshold.Type != "monetary" {
		t.Errorf(
			"consequence_threshold.type = %q, want %q",
			profileDoc.Spec.Authority.ConsequenceThreshold.Type,
			"monetary",
		)
	}
	if profileDoc.Spec.Authority.ConsequenceThreshold.Amount != 10000 {
		t.Errorf(
			"consequence_threshold.amount = %v, want %v",
			profileDoc.Spec.Authority.ConsequenceThreshold.Amount,
			10000,
		)
	}
	if len(profileDoc.Spec.InputRequirements.RequiredContext) != 2 {
		t.Errorf(
			"required_context length = %d, want %d",
			len(profileDoc.Spec.InputRequirements.RequiredContext),
			2,
		)
	}
	if profileDoc.Spec.Policy.Reference != "rego://payments/auto_approve_v1" {
		t.Errorf(
			"policy.reference = %q, want %q",
			profileDoc.Spec.Policy.Reference,
			"rego://payments/auto_approve_v1",
		)
	}
}

func TestParseYAML_Agent_InvalidRuntimeShape(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-1
spec:
  type: llm_agent
  runtime: "this should be an object, not a string"`)

	_, err := ParseYAML(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse Agent document") {
		t.Errorf("expected Agent parse context in error, got %v", err)
	}
}

// TestParseYAML_BusinessServiceCapability_UnknownField verifies that
// strictUnmarshal rejects an unknown field in a BusinessServiceCapability spec
// — the strict-field guard must apply to the new Kind exactly as it does to
// every other Kind.
func TestParseYAML_BusinessServiceCapability_UnknownField(t *testing.T) {
	data := []byte(`apiVersion: midas.accept.io/v1
kind: BusinessServiceCapability
metadata:
  id: bsc-bad
spec:
  business_service_id: bs-x
  capability_id: cap-x
  unknown_field: should-be-rejected`)

	_, err := ParseYAML(data)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse BusinessServiceCapability document") {
		t.Errorf("expected BusinessServiceCapability parse context in error, got %v", err)
	}
}

// ============================================================================
// Multi-Document Stream Tests
// ============================================================================

func TestParseYAMLStream_MultipleDocuments(t *testing.T) {
	data := []byte(`---
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial
  risk_tier: high
  status: active
---
apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-credit-scoring-prod
spec:
  type: llm_agent
  status: active
---
apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: payments-tier-1
spec:
  surface_id: payment.execute
  authority:
    decision_confidence_threshold: 0.85
---
apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-credit-scoring-tier-1
spec:
  agent_id: agent-credit-scoring-prod
  profile_id: payments-tier-1
  status: active`)

	docs, err := ParseYAMLStream(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 4 {
		t.Fatalf("expected 4 documents, got %d", len(docs))
	}

	expectedKinds := []string{
		types.KindSurface,
		types.KindAgent,
		types.KindProfile,
		types.KindGrant,
	}
	expectedIDs := []string{
		"payment.execute",
		"agent-credit-scoring-prod",
		"payments-tier-1",
		"grant-credit-scoring-tier-1",
	}

	for i, doc := range docs {
		if doc.Kind != expectedKinds[i] {
			t.Errorf("doc %d: expected kind %q, got %q", i, expectedKinds[i], doc.Kind)
		}
		if doc.ID != expectedIDs[i] {
			t.Errorf("doc %d: expected id %q, got %q", i, expectedIDs[i], doc.ID)
		}
	}
}

func TestParseYAMLStream_PreservesOrder(t *testing.T) {
	data := []byte(`---
apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: third-in-logical-order
spec:
  agent_id: agent-1
  profile_id: profile-1
  status: active
---
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: first-in-logical-order
spec:
  category: financial
---
apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: second-in-logical-order
spec:
  type: llm_agent`)

	docs, err := ParseYAMLStream(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(docs))
	}

	expectedIDs := []string{
		"third-in-logical-order",
		"first-in-logical-order",
		"second-in-logical-order",
	}

	for i, doc := range docs {
		if doc.ID != expectedIDs[i] {
			t.Errorf("doc %d: expected id %q, got %q (order not preserved)", i, expectedIDs[i], doc.ID)
		}
	}
}

func TestParseYAMLStream_SkipsEmptyDocuments(t *testing.T) {
	data := []byte(`---
---
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial
---
---
---`)

	docs, err := ParseYAMLStream(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	if docs[0].Kind != types.KindSurface {
		t.Errorf("expected kind %q, got %q", types.KindSurface, docs[0].Kind)
	}
	if docs[0].ID != "payment.execute" {
		t.Errorf("expected id %q, got %q", "payment.execute", docs[0].ID)
	}
}

func TestParseYAMLStream_InvalidDocumentInStream(t *testing.T) {
	data := []byte(`---
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial
---
apiVersion: midas.accept.io/v1
kind: PolicySet
metadata:
  id: payments-policy
spec: {}`)

	_, err := ParseYAMLStream(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "document 2") {
		t.Errorf("expected error to reference document 2, got %v", err)
	}
	if !strings.Contains(err.Error(), `unsupported kind: "PolicySet"`) {
		t.Errorf("expected unsupported kind error, got %v", err)
	}
}

func TestParseYAMLStream_FirstInvalidDocumentReported(t *testing.T) {
	data := []byte(`---
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: valid-1
spec:
  category: financial
---
apiVersion: midas.accept.io/v1
kind: InvalidKind
metadata:
  id: invalid
spec: {}
---
apiVersion: midas.accept.io/v1
kind: AlsoInvalid
metadata:
  id: also-invalid
spec: {}`)

	_, err := ParseYAMLStream(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "document 2") {
		t.Errorf("expected error to reference document 2 (first invalid), got: %v", err)
	}
	if strings.Contains(err.Error(), "document 3") {
		t.Errorf("error should stop at first invalid document, not mention document 3: %v", err)
	}
}

func TestParseYAMLStream_AtomicFailure(t *testing.T) {
	data := []byte(`---
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: valid-1
spec:
  category: financial
---
apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: valid-2
spec:
  type: llm_agent
---
apiVersion: midas.accept.io/v1
kind: InvalidKind
metadata:
  id: invalid
spec: {}`)

	docs, err := ParseYAMLStream(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if docs != nil {
		t.Errorf("expected nil docs on error, got %d documents", len(docs))
	}
}

func TestParseYAMLStream_NoDocumentsFound(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"only separators", "---\n---\n"},
		{"empty input", ""},
		{"whitespace only", "   \n\n  \n"},
		{"comments only", "# comment\n# another comment\n---\n# more comments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseYAMLStream([]byte(tt.data))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "no YAML documents found") {
				t.Errorf("expected no documents error, got %v", err)
			}
		})
	}
}

// ============================================================================
// Edge Case Tests - Whitespace and Formatting
// ============================================================================

func TestParseYAML_WindowsLineEndings(t *testing.T) {
	data := "apiVersion: midas.accept.io/v1\r\nkind: Surface\r\nmetadata:\r\n  id: payment.execute\r\nspec:\r\n  category: financial\r\n"

	doc, err := ParseYAML([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error with Windows line endings: %v", err)
	}

	if doc.Kind != types.KindSurface {
		t.Errorf("Kind = %q, want %q", doc.Kind, types.KindSurface)
	}
	if doc.ID != "payment.execute" {
		t.Errorf("ID = %q, want %q", doc.ID, "payment.execute")
	}
}

func TestParseYAML_TabsInsteadOfSpaces(t *testing.T) {
	data := "apiVersion: midas.accept.io/v1\nkind: Surface\nmetadata:\n\tid: payment.execute\nspec:\n\tcategory: financial\n"

	_, err := ParseYAML([]byte(data))
	if err == nil {
		t.Fatal("expected error for tab-indented YAML, got nil")
	}
}

func TestParseYAML_LeadingTrailingWhitespace(t *testing.T) {
	data := []byte(`


apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: payment.execute
spec:
  category: financial


`)

	doc, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("unexpected error with leading/trailing whitespace: %v", err)
	}

	if doc.Kind != types.KindSurface {
		t.Errorf("Kind = %q, want %q", doc.Kind, types.KindSurface)
	}
}

// ============================================================================
// Type Assertion Safety Tests
// ============================================================================

func TestParseYAML_TypeAssertionSafety(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		expectedType interface{}
	}{
		{"Surface", validSurfaceYAML, types.SurfaceDocument{}},
		{"Agent", validAgentYAML, types.AgentDocument{}},
		{"Profile", validProfileYAML, types.ProfileDocument{}},
		{"Grant", validGrantYAML, types.GrantDocument{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			switch tt.expectedType.(type) {
			case types.SurfaceDocument:
				if _, ok := doc.Doc.(types.SurfaceDocument); !ok {
					t.Errorf("expected SurfaceDocument, got %T", doc.Doc)
				}
			case types.AgentDocument:
				if _, ok := doc.Doc.(types.AgentDocument); !ok {
					t.Errorf("expected AgentDocument, got %T", doc.Doc)
				}
			case types.ProfileDocument:
				if _, ok := doc.Doc.(types.ProfileDocument); !ok {
					t.Errorf("expected ProfileDocument, got %T", doc.Doc)
				}
			case types.GrantDocument:
				if _, ok := doc.Doc.(types.GrantDocument); !ok {
					t.Errorf("expected GrantDocument, got %T", doc.Doc)
				}
			}
		})
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkParseYAML_Surface(b *testing.B) {
	data := []byte(validSurfaceYAML)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseYAML(data)
	}
}

func BenchmarkParseYAML_Agent(b *testing.B) {
	data := []byte(validAgentYAML)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseYAML(data)
	}
}

func BenchmarkParseYAML_Profile(b *testing.B) {
	data := []byte(validProfileYAML)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseYAML(data)
	}
}

func BenchmarkParseYAML_Grant(b *testing.B) {
	data := []byte(validGrantYAML)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseYAML(data)
	}
}

func BenchmarkParseYAMLStream_Small(b *testing.B) {
	data := []byte(`---
` + validSurfaceYAML + `
---
` + validAgentYAML)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseYAMLStream(data)
	}
}

func BenchmarkParseYAMLStream_Large(b *testing.B) {
	var buf strings.Builder
	for i := 0; i < 100; i++ {
		buf.WriteString("---\n")
		buf.WriteString(validSurfaceYAML)
		buf.WriteString("\n")
	}
	data := []byte(buf.String())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseYAMLStream(data)
	}
}
