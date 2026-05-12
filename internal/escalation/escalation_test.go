package escalation

import (
	"errors"
	"testing"
	"time"
)

func TestKind_IsValid(t *testing.T) {
	for _, k := range []Kind{KindRole, KindAgent, KindSurface, KindExternal} {
		if !k.IsValid() {
			t.Errorf("canonical kind %q reported invalid", string(k))
		}
	}
	for _, k := range []Kind{"", "team", "ROLE", "service"} {
		if k.IsValid() {
			t.Errorf("non-canonical kind %q reported valid", string(k))
		}
	}
}

func TestStatus_IsValid(t *testing.T) {
	for _, s := range []Status{StatusDraft, StatusReview, StatusActive, StatusDeprecated, StatusRetired} {
		if !s.IsValid() {
			t.Errorf("canonical status %q reported invalid", string(s))
		}
	}
	for _, s := range []Status{"", "approved", "Active"} {
		if s.IsValid() {
			t.Errorf("non-canonical status %q reported valid", string(s))
		}
	}
}

func validTarget() *EscalationTarget {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	return &EscalationTarget{
		ID:            "et-1",
		Version:       1,
		Name:          "Governance Approver",
		Kind:          KindRole,
		Handle:        "governance.approver",
		Status:        StatusActive,
		EffectiveDate: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func TestValidate_Valid(t *testing.T) {
	if err := validTarget().Validate(); err != nil {
		t.Errorf("valid target rejected: %v", err)
	}
}

func TestValidate_RejectsNil(t *testing.T) {
	var t0 *EscalationTarget
	err := t0.Validate()
	if err == nil {
		t.Fatal("nil target should fail validation")
	}
	if !errors.Is(err, ErrInvalidEscalationTarget) {
		t.Errorf("expected ErrInvalidEscalationTarget chain; got %v", err)
	}
}

func TestValidate_RejectsEmptyID(t *testing.T) {
	tgt := validTarget()
	tgt.ID = ""
	err := tgt.Validate()
	if err == nil {
		t.Fatal("empty id should fail validation")
	}
	if !errors.Is(err, ErrInvalidEscalationTarget) {
		t.Errorf("expected ErrInvalidEscalationTarget chain; got %v", err)
	}
}

func TestValidate_RejectsBadVersion(t *testing.T) {
	for _, v := range []int{0, -1, -10} {
		tgt := validTarget()
		tgt.Version = v
		if err := tgt.Validate(); err == nil {
			t.Errorf("version %d should be invalid", v)
		}
	}
}

func TestValidate_RejectsEmptyName(t *testing.T) {
	tgt := validTarget()
	tgt.Name = ""
	if err := tgt.Validate(); err == nil {
		t.Error("empty name should fail")
	}
}

func TestValidate_RejectsUnknownKind(t *testing.T) {
	tgt := validTarget()
	tgt.Kind = "team"
	err := tgt.Validate()
	if err == nil {
		t.Fatal("unknown kind should fail")
	}
	if !errors.Is(err, ErrInvalidEscalationTarget) {
		t.Errorf("expected ErrInvalidEscalationTarget chain; got %v", err)
	}
}

func TestValidate_RejectsEmptyHandle(t *testing.T) {
	tgt := validTarget()
	tgt.Handle = ""
	if err := tgt.Validate(); err == nil {
		t.Error("empty handle should fail")
	}
}

func TestValidate_RejectsUnknownStatus(t *testing.T) {
	tgt := validTarget()
	tgt.Status = "approved"
	if err := tgt.Validate(); err == nil {
		t.Error("unknown status should fail")
	}
}

func TestValidate_RejectsZeroEffectiveDate(t *testing.T) {
	tgt := validTarget()
	tgt.EffectiveDate = time.Time{}
	if err := tgt.Validate(); err == nil {
		t.Error("zero effective_date should fail")
	}
}

func TestValidate_EffectiveUntilWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Until == effective is invalid (must be strictly after).
	eq := now
	tgt := validTarget()
	tgt.EffectiveDate = now
	tgt.EffectiveUntil = &eq
	if err := tgt.Validate(); err == nil {
		t.Error("until == effective_date should fail")
	}

	// Until before effective is invalid.
	before := now.Add(-time.Hour)
	tgt = validTarget()
	tgt.EffectiveDate = now
	tgt.EffectiveUntil = &before
	if err := tgt.Validate(); err == nil {
		t.Error("until before effective_date should fail")
	}

	// Until strictly after is valid.
	after := now.Add(time.Hour)
	tgt = validTarget()
	tgt.EffectiveDate = now
	tgt.EffectiveUntil = &after
	if err := tgt.Validate(); err != nil {
		t.Errorf("valid window rejected: %v", err)
	}
}

func TestCanTransitionTo(t *testing.T) {
	type row struct {
		from, to Status
		want     bool
	}
	cases := []row{
		{StatusDraft, StatusReview, true},
		{StatusDraft, StatusRetired, true},
		{StatusDraft, StatusActive, false},
		{StatusDraft, StatusDeprecated, false},

		{StatusReview, StatusActive, true},
		{StatusReview, StatusDraft, true},
		{StatusReview, StatusRetired, true},
		{StatusReview, StatusDeprecated, false},

		{StatusActive, StatusDeprecated, true},
		{StatusActive, StatusRetired, true},
		{StatusActive, StatusDraft, false},
		{StatusActive, StatusReview, false},

		{StatusDeprecated, StatusRetired, true},
		{StatusDeprecated, StatusActive, false},
		{StatusDeprecated, StatusDraft, false},

		{StatusRetired, StatusDraft, false},
		{StatusRetired, StatusReview, false},
		{StatusRetired, StatusActive, false},
		{StatusRetired, StatusDeprecated, false},
	}
	for _, c := range cases {
		tgt := &EscalationTarget{Status: c.from}
		if got := tgt.CanTransitionTo(c.to); got != c.want {
			t.Errorf("%s → %s: got %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

// TestCanonicalStringValues pins the wire-level enum literals. Changing
// these would silently break OpenAPI and storage compatibility.
func TestCanonicalStringValues(t *testing.T) {
	if string(KindRole) != "role" || string(KindAgent) != "agent" ||
		string(KindSurface) != "surface" || string(KindExternal) != "external" {
		t.Errorf("Kind literals drifted")
	}
	if string(StatusDraft) != "draft" || string(StatusReview) != "review" ||
		string(StatusActive) != "active" || string(StatusDeprecated) != "deprecated" ||
		string(StatusRetired) != "retired" {
		t.Errorf("Status literals drifted")
	}
}
