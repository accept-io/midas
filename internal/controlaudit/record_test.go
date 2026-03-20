package controlaudit

import (
	"strings"
	"testing"
)

func TestNewSurfaceCreatedRecord(t *testing.T) {
	rec := NewSurfaceCreatedRecord("alice", "payments.execute", 1)

	if rec.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rec.Actor != "alice" {
		t.Errorf("expected actor 'alice', got %q", rec.Actor)
	}
	if rec.Action != ActionSurfaceCreated {
		t.Errorf("expected action %q, got %q", ActionSurfaceCreated, rec.Action)
	}
	if rec.ResourceKind != ResourceKindSurface {
		t.Errorf("expected kind %q, got %q", ResourceKindSurface, rec.ResourceKind)
	}
	if rec.ResourceID != "payments.execute" {
		t.Errorf("expected resource_id 'payments.execute', got %q", rec.ResourceID)
	}
	if rec.ResourceVersion == nil || *rec.ResourceVersion != 1 {
		t.Errorf("expected version 1, got %v", rec.ResourceVersion)
	}
	if rec.OccurredAt.IsZero() {
		t.Error("expected non-zero occurred_at")
	}
	if rec.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if rec.Metadata != nil {
		t.Error("expected nil metadata for surface.created")
	}
}

func TestNewProfileCreatedRecord(t *testing.T) {
	rec := NewProfileCreatedRecord("bob", "prof-payments", "payments.execute", 1)

	if rec.Action != ActionProfileCreated {
		t.Errorf("expected %q, got %q", ActionProfileCreated, rec.Action)
	}
	if rec.ResourceKind != ResourceKindProfile {
		t.Errorf("expected kind %q, got %q", ResourceKindProfile, rec.ResourceKind)
	}
	if rec.ResourceVersion == nil || *rec.ResourceVersion != 1 {
		t.Errorf("expected version 1, got %v", rec.ResourceVersion)
	}
	if rec.Metadata == nil || rec.Metadata.SurfaceID != "payments.execute" {
		t.Errorf("expected metadata.surface_id 'payments.execute'")
	}
}

func TestNewProfileVersionedRecord(t *testing.T) {
	rec := NewProfileVersionedRecord("bob", "prof-payments", "payments.execute", 2)

	if rec.Action != ActionProfileVersioned {
		t.Errorf("expected %q, got %q", ActionProfileVersioned, rec.Action)
	}
	if rec.ResourceVersion == nil || *rec.ResourceVersion != 2 {
		t.Errorf("expected version 2, got %v", rec.ResourceVersion)
	}
}

func TestNewAgentCreatedRecord(t *testing.T) {
	rec := NewAgentCreatedRecord("system", "agent-credit-scoring")

	if rec.Action != ActionAgentCreated {
		t.Errorf("expected %q, got %q", ActionAgentCreated, rec.Action)
	}
	if rec.ResourceKind != ResourceKindAgent {
		t.Errorf("expected kind %q, got %q", ResourceKindAgent, rec.ResourceKind)
	}
	if rec.ResourceVersion != nil {
		t.Errorf("expected nil version for agent, got %v", rec.ResourceVersion)
	}
}

func TestNewGrantCreatedRecord(t *testing.T) {
	rec := NewGrantCreatedRecord("system", "grant-001")

	if rec.Action != ActionGrantCreated {
		t.Errorf("expected %q, got %q", ActionGrantCreated, rec.Action)
	}
	if rec.ResourceKind != ResourceKindGrant {
		t.Errorf("expected kind %q, got %q", ResourceKindGrant, rec.ResourceKind)
	}
	if rec.ResourceVersion != nil {
		t.Errorf("expected nil version for grant, got %v", rec.ResourceVersion)
	}
}

func TestNewSurfaceApprovedRecord(t *testing.T) {
	rec := NewSurfaceApprovedRecord("approver-1", "payments.execute", 1)

	if rec.Action != ActionSurfaceApproved {
		t.Errorf("expected %q, got %q", ActionSurfaceApproved, rec.Action)
	}
	if rec.Actor != "approver-1" {
		t.Errorf("expected actor 'approver-1', got %q", rec.Actor)
	}
	if rec.Metadata != nil {
		t.Error("expected nil metadata for surface.approved")
	}
}

func TestNewSurfaceDeprecatedRecord(t *testing.T) {
	rec := NewSurfaceDeprecatedRecord("ops-team", "payments.execute", 2, "replaced by v3", "payments.execute.v3")

	if rec.Action != ActionSurfaceDeprecated {
		t.Errorf("expected %q, got %q", ActionSurfaceDeprecated, rec.Action)
	}
	if rec.Metadata == nil {
		t.Fatal("expected non-nil metadata for surface.deprecated")
	}
	if rec.Metadata.DeprecationReason != "replaced by v3" {
		t.Errorf("expected deprecation_reason 'replaced by v3', got %q", rec.Metadata.DeprecationReason)
	}
	if rec.Metadata.SuccessorSurfaceID != "payments.execute.v3" {
		t.Errorf("expected successor_surface_id 'payments.execute.v3', got %q", rec.Metadata.SuccessorSurfaceID)
	}
}

func TestRecord_IDIsUUID(t *testing.T) {
	rec := NewSurfaceCreatedRecord("alice", "surf-1", 1)
	// UUIDs are 36 chars with hyphens in the form 8-4-4-4-12
	if len(rec.ID) != 36 || !strings.Contains(rec.ID, "-") {
		t.Errorf("expected UUID-format ID, got %q", rec.ID)
	}
}

func TestRecord_IDsAreUnique(t *testing.T) {
	r1 := NewSurfaceCreatedRecord("alice", "surf-1", 1)
	r2 := NewSurfaceCreatedRecord("alice", "surf-1", 1)
	if r1.ID == r2.ID {
		t.Error("expected unique IDs for separate records")
	}
}

func TestSummaryContainsResourceID(t *testing.T) {
	rec := NewSurfaceCreatedRecord("alice", "payments.execute", 3)
	if !strings.Contains(rec.Summary, "payments.execute") {
		t.Errorf("expected summary to contain resource ID, got %q", rec.Summary)
	}
	if !strings.Contains(rec.Summary, "3") {
		t.Errorf("expected summary to contain version, got %q", rec.Summary)
	}
}
