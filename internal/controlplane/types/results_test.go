package types_test

import (
	"encoding/json"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/types"
)

// ---------------------------------------------------------------------------
// ApplyResult Builder Tests
// ---------------------------------------------------------------------------

func TestApplyResult_AddCreated(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "payment.execute")

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Kind != types.KindSurface {
		t.Errorf("expected kind %q, got %q", types.KindSurface, result.Results[0].Kind)
	}
	if result.Results[0].ID != "payment.execute" {
		t.Errorf("expected id %q, got %q", "payment.execute", result.Results[0].ID)
	}
	if result.Results[0].Status != types.ResourceStatusCreated {
		t.Errorf("expected status %q, got %q", types.ResourceStatusCreated, result.Results[0].Status)
	}
}

func TestApplyResult_AddConflict(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddConflict(types.KindAgent, "agent-1", "agent already exists")

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Status != types.ResourceStatusConflict {
		t.Errorf("expected status %q, got %q", types.ResourceStatusConflict, result.Results[0].Status)
	}
	if result.Results[0].Message != "agent already exists" {
		t.Errorf("expected message %q, got %q", "agent already exists", result.Results[0].Message)
	}
}

func TestApplyResult_AddError(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddError(types.KindProfile, "prof-1", "database connection failed")

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Status != types.ResourceStatusError {
		t.Errorf("expected status %q, got %q", types.ResourceStatusError, result.Results[0].Status)
	}
	if result.Results[0].Message != "database connection failed" {
		t.Errorf("expected message %q, got %q", "database connection failed", result.Results[0].Message)
	}
}

func TestApplyResult_AddFieldError(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddFieldError(types.KindGrant, "grant-1", "spec.profile_id", "profile not found")

	if len(result.ValidationErrors) != 1 {
		t.Fatalf("expected 1 validation error, got %d", len(result.ValidationErrors))
	}
	err := result.ValidationErrors[0]
	if err.Kind != types.KindGrant {
		t.Errorf("expected kind %q, got %q", types.KindGrant, err.Kind)
	}
	if err.ID != "grant-1" {
		t.Errorf("expected id %q, got %q", "grant-1", err.ID)
	}
	if err.Field != "spec.profile_id" {
		t.Errorf("expected field %q, got %q", "spec.profile_id", err.Field)
	}
	if err.Message != "profile not found" {
		t.Errorf("expected message %q, got %q", "profile not found", err.Message)
	}
}

func TestApplyResult_AddValidationError(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddValidationError(types.KindProfile, "prof-1", "invalid profile document")

	if len(result.ValidationErrors) != 1 {
		t.Fatalf("expected 1 validation error, got %d", len(result.ValidationErrors))
	}
	err := result.ValidationErrors[0]
	if err.Kind != types.KindProfile {
		t.Errorf("expected kind %q, got %q", types.KindProfile, err.Kind)
	}
	if err.ID != "prof-1" {
		t.Errorf("expected id %q, got %q", "prof-1", err.ID)
	}
	if err.Field != "" {
		t.Errorf("expected empty field, got %q", err.Field)
	}
	if err.Message != "invalid profile document" {
		t.Errorf("expected message %q, got %q", "invalid profile document", err.Message)
	}
}

func TestApplyResult_AddUnchanged(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddUnchanged(types.KindSurface, "surf-1")

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Status != types.ResourceStatusUnchanged {
		t.Errorf("expected status %q, got %q", types.ResourceStatusUnchanged, result.Results[0].Status)
	}
}

// ---------------------------------------------------------------------------
// ApplyResult Query Tests
// ---------------------------------------------------------------------------

func TestApplyResult_CreatedCount(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddCreated(types.KindAgent, "agent-1")
	result.AddConflict(types.KindProfile, "prof-1", "conflict")

	if result.CreatedCount() != 2 {
		t.Errorf("expected 2 created, got %d", result.CreatedCount())
	}
}

func TestApplyResult_ConflictCount(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddConflict(types.KindAgent, "agent-1", "conflict")
	result.AddConflict(types.KindProfile, "prof-1", "conflict")

	if result.ConflictCount() != 2 {
		t.Errorf("expected 2 conflicts, got %d", result.ConflictCount())
	}
}

func TestApplyResult_ApplyErrorCount(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddError(types.KindAgent, "agent-1", "db error")
	result.AddError(types.KindProfile, "prof-1", "db error")

	if result.ApplyErrorCount() != 2 {
		t.Errorf("expected 2 apply errors, got %d", result.ApplyErrorCount())
	}
}

func TestApplyResult_ValidationErrorCount(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddFieldError(types.KindGrant, "grant-1", "spec.agent_id", "required")
	result.AddFieldError(types.KindGrant, "grant-2", "spec.profile_id", "required")

	if result.ValidationErrorCount() != 2 {
		t.Errorf("expected 2 validation errors, got %d", result.ValidationErrorCount())
	}
}

func TestApplyResult_UnchangedCount(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddUnchanged(types.KindAgent, "agent-1")
	result.AddUnchanged(types.KindProfile, "prof-1")

	if result.UnchangedCount() != 2 {
		t.Errorf("expected 2 unchanged, got %d", result.UnchangedCount())
	}
}

func TestApplyResult_HasValidationErrors(t *testing.T) {
	result := &types.ApplyResult{}
	if result.HasValidationErrors() {
		t.Error("expected no validation errors initially")
	}

	result.AddFieldError(types.KindGrant, "grant-1", "spec.agent_id", "required")
	if !result.HasValidationErrors() {
		t.Error("expected validation errors after AddFieldError")
	}
}

func TestApplyResult_IsValid(t *testing.T) {
	result := &types.ApplyResult{}
	if !result.IsValid() {
		t.Error("expected IsValid to be true initially")
	}

	result.AddFieldError(types.KindGrant, "grant-1", "spec.agent_id", "required")
	if result.IsValid() {
		t.Error("expected IsValid to be false after validation error")
	}
}

func TestApplyResult_Success_AllCreated(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddCreated(types.KindAgent, "agent-1")

	if !result.Success() {
		t.Error("expected Success() = true when all resources were created")
	}
}

func TestApplyResult_Success_WithConflicts(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddConflict(types.KindAgent, "agent-1", "already exists")

	if result.Success() {
		t.Error("expected Success() = false when conflicts are present")
	}
}

func TestApplyResult_Success_WithApplyErrors(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddError(types.KindAgent, "agent-1", "db error")

	if result.Success() {
		t.Error("expected Success() = false when apply errors are present")
	}
}

func TestApplyResult_Success_WithValidationErrors(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddFieldError(types.KindGrant, "grant-1", "spec.agent_id", "required")

	if result.Success() {
		t.Error("expected Success() = false when validation errors are present")
	}
}

// ---------------------------------------------------------------------------
// Serialization Tests
// ---------------------------------------------------------------------------

func TestApplyResult_JSONSerialization(t *testing.T) {
	result := &types.ApplyResult{}
	result.AddCreated(types.KindSurface, "surf-1")
	result.AddConflict(types.KindAgent, "agent-1", "already exists")
	result.AddFieldError(types.KindGrant, "grant-1", "spec.profile_id", "profile not found")

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal ApplyResult: %v", err)
	}

	var decoded types.ApplyResult
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ApplyResult: %v", err)
	}

	if len(decoded.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(decoded.Results))
	}
	if len(decoded.ValidationErrors) != 1 {
		t.Errorf("expected 1 validation error, got %d", len(decoded.ValidationErrors))
	}
	if decoded.Results[0].Status != types.ResourceStatusCreated {
		t.Errorf("expected first result status %q, got %q", types.ResourceStatusCreated, decoded.Results[0].Status)
	}
	if decoded.Results[1].Status != types.ResourceStatusConflict {
		t.Errorf("expected second result status %q, got %q", types.ResourceStatusConflict, decoded.Results[1].Status)
	}
}

// ---------------------------------------------------------------------------
// ResourceStatus Constants Tests
// ---------------------------------------------------------------------------

func TestResourceStatusConstants(t *testing.T) {
	tests := []struct {
		constant types.ResourceStatus
		expected string
	}{
		{types.ResourceStatusCreated, "created"},
		{types.ResourceStatusConflict, "conflict"},
		{types.ResourceStatusError, "error"},
		{types.ResourceStatusUnchanged, "unchanged"},
	}

	for _, tt := range tests {
		if string(tt.constant) != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, string(tt.constant))
		}
	}
}
