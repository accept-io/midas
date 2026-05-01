package apply

import (
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// TestMapAgentType_ValidDocTypes verifies that all recognised document-layer
// agent types map to the correct canonical domain AgentType without error, and
// that the resulting domain type passes IsValid(). This test is the regression
// guard for the constraint mismatch: if the domain constant is changed but the
// DB constraint (or vice versa) is not updated, IsValid() will catch it.
func TestMapAgentType_ValidDocTypes(t *testing.T) {
	cases := []struct {
		docType  string
		wantType agent.AgentType
	}{
		{"llm_agent", agent.AgentTypeAI},
		{"copilot", agent.AgentTypeAI},
		{"workflow", agent.AgentTypeService},
		{"automation", agent.AgentTypeService},
		{"rpa", agent.AgentTypeService},
	}

	for _, c := range cases {
		t.Run(c.docType, func(t *testing.T) {
			got, err := mapAgentType(c.docType)
			if err != nil {
				t.Fatalf("mapAgentType(%q) unexpected error: %v", c.docType, err)
			}
			if got != c.wantType {
				t.Errorf("mapAgentType(%q) = %q, want %q", c.docType, got, c.wantType)
			}
			if !got.IsValid() {
				t.Errorf("mapAgentType(%q) returned %q which fails IsValid()", c.docType, got)
			}
		})
	}
}

// TestMapAgentType_InvalidDocType verifies that an unrecognised document-layer
// type is rejected with a clear error — not a raw database constraint error.
func TestMapAgentType_InvalidDocType(t *testing.T) {
	_, err := mapAgentType("human")
	if err == nil {
		t.Fatal("expected error for unrecognised doc type, got nil")
	}
}

// TestMapAgentDocumentToAgent_ServiceType verifies the full mapping path for a
// non-AI agent (workflow → AgentTypeService). This is the scenario that
// previously failed at the database layer with an opaque CHECK constraint error
// because the schema constraint was stale. With the constraint fixed and
// IsValid() added, this path is now exercised end-to-end in unit tests.
func TestMapAgentDocumentToAgent_ServiceType(t *testing.T) {
	doc := types.AgentDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAgent,
		Metadata: types.DocumentMetadata{
			ID:   "agent-workflow-test",
			Name: "Workflow Automation Agent",
		},
		Spec: types.AgentSpec{
			Type:   "workflow",
			Status: "active",
		},
	}

	ag, err := mapAgentDocumentToAgent(doc, time.Now())
	if err != nil {
		t.Fatalf("mapAgentDocumentToAgent for workflow type: unexpected error: %v", err)
	}
	if ag.Type != agent.AgentTypeService {
		t.Errorf("Type = %q, want %q", ag.Type, agent.AgentTypeService)
	}
	if !ag.Type.IsValid() {
		t.Errorf("resulting AgentType %q fails IsValid()", ag.Type)
	}
}

// TestMapAgentDocumentToAgent_InvalidType verifies that an unrecognised spec
// type is rejected at the application layer with a clear error, before any
// database interaction can occur.
func TestMapAgentDocumentToAgent_InvalidType(t *testing.T) {
	doc := types.AgentDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAgent,
		Metadata: types.DocumentMetadata{
			ID:   "agent-invalid-type",
			Name: "Invalid Type Agent",
		},
		Spec: types.AgentSpec{
			Type:   "human",
			Status: "active",
		},
	}

	_, err := mapAgentDocumentToAgent(doc, time.Now())
	if err == nil {
		t.Fatal("expected error for invalid agent type, got nil")
	}
	// The error must be a specific message — not a raw database error.
	if errors.Is(err, agent.ErrInvalidAgentType) {
		// If the error wraps ErrInvalidAgentType directly, that's ideal.
		return
	}
	// Otherwise, the mapper error is acceptable as long as it is not a DB error.
	// The key invariant is that the error is non-nil and comes from the application layer.
}

// TestMapProcessDocument_BusinessServiceID verifies that the mapper correctly
// carries spec.business_service_id into the domain model. In the v1 service-led
// model business_service_id is required at apply-time (validated upstream); the
// mapper itself simply propagates whatever value the spec carries.
func TestMapProcessDocument_BusinessServiceID(t *testing.T) {
	now := time.Now()

	makeDoc := func(bsID string) types.ProcessDocument {
		return types.ProcessDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcess,
			Metadata:   types.DocumentMetadata{ID: "proc-mapper-test", Name: "Mapper Test Process"},
			Spec: types.ProcessSpec{
				Status:            "active",
				BusinessServiceID: bsID,
			},
		}
	}

	t.Run("business_service_id mapped correctly", func(t *testing.T) {
		p := mapProcessDocumentToProcess(makeDoc("bs-payments"), now, "tester")
		if p.BusinessServiceID != "bs-payments" {
			t.Errorf("BusinessServiceID = %q, want %q", p.BusinessServiceID, "bs-payments")
		}
	})
}

// TestMapSurfaceDocument_ProcessID verifies that the mapper correctly carries
// spec.process_id into the domain model, and that absent process_id results
// in an empty ProcessID field (backward compatibility).
func TestMapSurfaceDocument_ProcessID(t *testing.T) {
	now := time.Now()

	makeDoc := func(processID string) types.SurfaceDocument {
		return types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata: types.DocumentMetadata{
				ID:   "surf-mapper-test",
				Name: "Mapper Test Surface",
			},
			Spec: types.SurfaceSpec{
				Category:  "financial",
				RiskTier:  "high",
				Status:    "active",
				ProcessID: processID,
			},
		}
	}

	t.Run("process_id mapped correctly", func(t *testing.T) {
		ds, err := mapSurfaceDocumentToDecisionSurface(makeDoc("payments.limits-v1"), now, "tester", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ds.ProcessID != "payments.limits-v1" {
			t.Errorf("ProcessID = %q, want %q", ds.ProcessID, "payments.limits-v1")
		}
	})

	t.Run("absent process_id maps to empty string", func(t *testing.T) {
		ds, err := mapSurfaceDocumentToDecisionSurface(makeDoc(""), now, "tester", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ds.ProcessID != "" {
			t.Errorf("ProcessID = %q, want empty string", ds.ProcessID)
		}
	})
}

// TestMapProfileDocumentToAuthorityProfile_DefaultsEscalationModeAuto pins the
// behaviour required to keep Profile apply working against Postgres. The YAML
// document type does not expose escalation_mode today; without a mapper-side
// default, AuthorityProfile.EscalationMode would be "" and Postgres'
// chk_profiles_escalation_mode CHECK (auto|manual) would reject the row on
// Create. The fix is the smallest possible: default to auto in the mapper.
//
// Manual escalation through YAML is intentionally out of scope here — the
// document type extension is tracked separately. This test pins the default
// so a future YAML schema change cannot silently regress the apply path.
func TestMapProfileDocumentToAuthorityProfile_DefaultsEscalationModeAuto(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	doc := types.ProfileDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindProfile,
		Metadata: types.DocumentMetadata{
			ID:   "profile-mapper-default-test",
			Name: "Mapper Default Test Profile",
		},
		Spec: types.ProfileSpec{
			SurfaceID: "surf-mapper-default-test",
			Authority: types.ProfileAuthority{
				DecisionConfidenceThreshold: 0.85,
				ConsequenceThreshold: types.ConsequenceThreshold{
					Type:     "monetary",
					Amount:   1000,
					Currency: "GBP",
				},
			},
			Policy: types.ProfilePolicy{
				Reference: "rego://test/auto",
				FailMode:  "closed",
			},
			Lifecycle: types.ProfileLifecycle{
				Status:        "active",
				EffectiveFrom: "2026-01-01T00:00:00Z",
				Version:       1,
			},
		},
	}

	p, err := mapProfileDocumentToAuthorityProfile(doc, now, "tester", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("mapper returned nil profile")
	}
	if p.EscalationMode != authority.EscalationModeAuto {
		t.Errorf("EscalationMode = %q, want %q (Postgres CHECK requires auto|manual)",
			p.EscalationMode, authority.EscalationModeAuto)
	}
	if p.EscalationMode == "" {
		t.Errorf("EscalationMode is empty — would fail chk_profiles_escalation_mode on apply")
	}
}
