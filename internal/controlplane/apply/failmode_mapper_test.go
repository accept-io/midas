package apply

import (
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/failmode"
)

// validFailModePolicyDoc returns a structurally-valid document used as the
// baseline for mapper tests. Tests mutate one field at a time.
func validFailModePolicyDoc() types.FailModePolicyDocument {
	return types.FailModePolicyDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindFailModePolicy,
		Metadata:   types.DocumentMetadata{ID: "default-fail-mode", Name: "Default Fail-Mode Policy"},
		Spec: types.FailModePolicySpec{
			Name:           "Default Fail-Mode Policy",
			Description:    "Closed-only baseline.",
			BusinessOwner:  "owner@example.com",
			TechnicalOwner: "platform-team",
			Rules: []types.FailModePolicyRuleSpec{
				{CorrectnessClass: "governance_integrity", PermittedMode: "closed"},
				{CorrectnessClass: "persistence", PermittedMode: "closed"},
				{CorrectnessClass: "input", PermittedMode: "not_applicable"},
				{CorrectnessClass: "resource", PermittedMode: "closed", Reason: "policy evaluator unreachable"},
				{CorrectnessClass: "consistency", PermittedMode: "closed"},
			},
		},
	}
}

func TestMapFailModePolicy_SetsReviewStatus(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Lifecycle.Status = "active" // any value is allowed by the validator
	now := time.Now().UTC()

	p, err := mapFailModePolicyDocumentToDomain(doc, now, "user-1", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.Status != failmode.FailModePolicyStatusReview {
		t.Errorf("Status: want review (forced), got %q", p.Status)
	}
}

func TestMapFailModePolicy_DefaultsOriginManaged(t *testing.T) {
	doc := validFailModePolicyDoc()
	// Origin and Managed unset
	now := time.Now().UTC()

	p, err := mapFailModePolicyDocumentToDomain(doc, now, "user-1", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.Origin != "manual" {
		t.Errorf("Origin: want manual default, got %q", p.Origin)
	}
	if !p.Managed {
		t.Errorf("Managed: want true default, got false")
	}
}

func TestMapFailModePolicy_PassesExplicitOriginManagedReplaces(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Spec.Origin = "manual"
	mngd := false
	doc.Spec.Managed = &mngd
	doc.Spec.Replaces = "older-policy"
	now := time.Now().UTC()

	p, err := mapFailModePolicyDocumentToDomain(doc, now, "user-1", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.Origin != "manual" {
		t.Errorf("Origin: want manual, got %q", p.Origin)
	}
	if p.Managed {
		t.Errorf("Managed: want explicit false, got true")
	}
	if p.Replaces != "older-policy" {
		t.Errorf("Replaces: want %q, got %q", "older-policy", p.Replaces)
	}
}

func TestMapFailModePolicy_ParsesLifecycleDates(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Lifecycle.EffectiveFrom = "2026-03-01T00:00:00Z"
	doc.Lifecycle.EffectiveUntil = "2026-12-31T23:59:59Z"
	now := time.Now().UTC()

	p, err := mapFailModePolicyDocumentToDomain(doc, now, "user-1", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	wantFrom, _ := time.Parse(time.RFC3339, "2026-03-01T00:00:00Z")
	if !p.EffectiveDate.Equal(wantFrom) {
		t.Errorf("EffectiveDate: want %v, got %v", wantFrom, p.EffectiveDate)
	}
	if p.EffectiveUntil == nil {
		t.Fatal("EffectiveUntil: want non-nil")
	}
	wantUntil, _ := time.Parse(time.RFC3339, "2026-12-31T23:59:59Z")
	if !p.EffectiveUntil.Equal(wantUntil) {
		t.Errorf("EffectiveUntil: want %v, got %v", wantUntil, *p.EffectiveUntil)
	}
}

func TestMapFailModePolicy_DefaultsEffectiveDateToNow(t *testing.T) {
	doc := validFailModePolicyDoc()
	// no lifecycle.effective_from
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	p, err := mapFailModePolicyDocumentToDomain(doc, now, "user-1", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if !p.EffectiveDate.Equal(now) {
		t.Errorf("EffectiveDate: want %v (now default), got %v", now, p.EffectiveDate)
	}
}

func TestMapFailModePolicy_LeavesApprovalFieldsEmpty(t *testing.T) {
	doc := validFailModePolicyDoc()
	now := time.Now().UTC()

	p, err := mapFailModePolicyDocumentToDomain(doc, now, "user-1", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.ApprovedBy != "" {
		t.Errorf("ApprovedBy: want empty, got %q", p.ApprovedBy)
	}
	if p.ApprovedAt != nil {
		t.Errorf("ApprovedAt: want nil, got %v", *p.ApprovedAt)
	}
	if p.RetiredAt != nil {
		t.Errorf("RetiredAt: want nil, got %v", *p.RetiredAt)
	}
}

func TestMapFailModePolicy_SetsCreatedByAndTimestamps(t *testing.T) {
	doc := validFailModePolicyDoc()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	p, err := mapFailModePolicyDocumentToDomain(doc, now, "user-applier", 7)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.CreatedBy != "user-applier" {
		t.Errorf("CreatedBy: want %q, got %q", "user-applier", p.CreatedBy)
	}
	if !p.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: want %v, got %v", now, p.CreatedAt)
	}
	if !p.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt: want %v, got %v", now, p.UpdatedAt)
	}
	if p.Version != 7 {
		t.Errorf("Version: want 7, got %d", p.Version)
	}
}

func TestMapFailModePolicy_RejectsSoftViaValidate(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "resource" {
			doc.Spec.Rules[i].PermittedMode = "soft"
		}
	}
	_, err := mapFailModePolicyDocumentToDomain(doc, time.Now(), "u", 1)
	if err == nil {
		t.Fatal("expected mapper-side rejection of soft mode; got nil")
	}
	if !strings.Contains(err.Error(), "not admitted") {
		t.Errorf("expected 'not admitted' in error; got %v", err)
	}
}

func TestMapFailModePolicy_RejectsOpenViaValidate(t *testing.T) {
	doc := validFailModePolicyDoc()
	for i := range doc.Spec.Rules {
		if doc.Spec.Rules[i].CorrectnessClass == "resource" {
			doc.Spec.Rules[i].PermittedMode = "open"
		}
	}
	_, err := mapFailModePolicyDocumentToDomain(doc, time.Now(), "u", 1)
	if err == nil {
		t.Fatal("expected mapper-side rejection of open mode; got nil")
	}
	if !strings.Contains(err.Error(), "not admitted") {
		t.Errorf("expected 'not admitted' in error; got %v", err)
	}
}

func TestMapFailModePolicy_TrimsStringFields(t *testing.T) {
	doc := validFailModePolicyDoc()
	doc.Metadata.ID = "  default-fail-mode  "
	doc.Spec.Name = "  Default Fail-Mode Policy  "
	doc.Spec.BusinessOwner = "  owner@example.com  "
	doc.Spec.TechnicalOwner = "  platform-team  "

	p, err := mapFailModePolicyDocumentToDomain(doc, time.Now(), "  user-1  ", 1)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.ID != "default-fail-mode" {
		t.Errorf("ID: want trimmed, got %q", p.ID)
	}
	if p.Name != "Default Fail-Mode Policy" {
		t.Errorf("Name: want trimmed, got %q", p.Name)
	}
	if p.BusinessOwner != "owner@example.com" {
		t.Errorf("BusinessOwner: want trimmed, got %q", p.BusinessOwner)
	}
	if p.TechnicalOwner != "platform-team" {
		t.Errorf("TechnicalOwner: want trimmed, got %q", p.TechnicalOwner)
	}
	if p.CreatedBy != "user-1" {
		t.Errorf("CreatedBy: want trimmed, got %q", p.CreatedBy)
	}
}
