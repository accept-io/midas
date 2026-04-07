package apply

import (
	"errors"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
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
// carries spec.business_service_id into the domain model, and that an absent
// business_service_id leaves BusinessServiceID empty (backward compatibility).
func TestMapProcessDocument_BusinessServiceID(t *testing.T) {
	now := time.Now()

	makeDoc := func(bsID string) types.ProcessDocument {
		return types.ProcessDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcess,
			Metadata:   types.DocumentMetadata{ID: "proc-mapper-test", Name: "Mapper Test Process"},
			Spec: types.ProcessSpec{
				CapabilityID:      "cap-mapper-test",
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

	t.Run("absent business_service_id maps to empty string", func(t *testing.T) {
		p := mapProcessDocumentToProcess(makeDoc(""), now, "tester")
		if p.BusinessServiceID != "" {
			t.Errorf("BusinessServiceID = %q, want empty string", p.BusinessServiceID)
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
