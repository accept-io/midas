package surface_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/surface"
)

// TestValidateLifecycleTransition verifies every permitted and forbidden
// transition in the surface lifecycle model.
func TestValidateLifecycleTransition(t *testing.T) {
	tests := []struct {
		from    surface.SurfaceStatus
		to      surface.SurfaceStatus
		wantErr bool
	}{
		// --- Permitted transitions ---
		{surface.SurfaceStatusDraft, surface.SurfaceStatusReview, false},
		{surface.SurfaceStatusDraft, surface.SurfaceStatusRetired, false},
		{surface.SurfaceStatusReview, surface.SurfaceStatusActive, false},
		{surface.SurfaceStatusReview, surface.SurfaceStatusDraft, false},
		{surface.SurfaceStatusActive, surface.SurfaceStatusDeprecated, false},
		{surface.SurfaceStatusDeprecated, surface.SurfaceStatusRetired, false},

		// --- Forbidden transitions ---
		{surface.SurfaceStatusDraft, surface.SurfaceStatusActive, true},
		{surface.SurfaceStatusDraft, surface.SurfaceStatusDeprecated, true},
		{surface.SurfaceStatusReview, surface.SurfaceStatusDeprecated, true},
		{surface.SurfaceStatusReview, surface.SurfaceStatusRetired, true},
		{surface.SurfaceStatusActive, surface.SurfaceStatusDraft, true},
		{surface.SurfaceStatusActive, surface.SurfaceStatusReview, true},
		{surface.SurfaceStatusActive, surface.SurfaceStatusRetired, true},
		{surface.SurfaceStatusDeprecated, surface.SurfaceStatusActive, true},
		{surface.SurfaceStatusDeprecated, surface.SurfaceStatusDraft, true},
		{surface.SurfaceStatusDeprecated, surface.SurfaceStatusReview, true},
		{surface.SurfaceStatusRetired, surface.SurfaceStatusDraft, true},
		{surface.SurfaceStatusRetired, surface.SurfaceStatusReview, true},
		{surface.SurfaceStatusRetired, surface.SurfaceStatusActive, true},
		{surface.SurfaceStatusRetired, surface.SurfaceStatusDeprecated, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			err := surface.ValidateLifecycleTransition(tc.from, tc.to)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %s → %s, got nil", tc.from, tc.to)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %s → %s: %v", tc.from, tc.to, err)
			}
			if tc.wantErr && err != nil {
				if !errors.Is(err, surface.ErrInvalidLifecycleTransition) {
					t.Errorf("expected ErrInvalidLifecycleTransition, got: %v", err)
				}
			}
		})
	}
}

// TestValidateLifecycleTransition_RetiredIsTerminal verifies that retired is
// a terminal state with no permitted outbound transitions.
func TestValidateLifecycleTransition_RetiredIsTerminal(t *testing.T) {
	targets := []surface.SurfaceStatus{
		surface.SurfaceStatusDraft,
		surface.SurfaceStatusReview,
		surface.SurfaceStatusActive,
		surface.SurfaceStatusDeprecated,
	}
	for _, to := range targets {
		err := surface.ValidateLifecycleTransition(surface.SurfaceStatusRetired, to)
		if err == nil {
			t.Errorf("retired → %s should be forbidden but got no error", to)
		}
	}
}

// ---------------------------------------------------------------------------
// DefaultSurfaceValidator.ValidateSurface — structural field checks,
// including the Surface → Process invariant closed by Issue #33.
// ---------------------------------------------------------------------------

// validSurface returns the minimal DecisionSurface that passes every check
// in DefaultSurfaceValidator.ValidateSurface. Tests clone this and mutate
// a single field to exercise one negative case at a time.
func validSurface() *surface.DecisionSurface {
	return &surface.DecisionSurface{
		ID:                  "surf-validator-test",
		Name:                "Validator Test Surface",
		Domain:              "test",
		ProcessID:           "proc-validator-test",
		MinimumConfidence:   0.5,
		AuditRetentionHours: 0,
	}
}

// TestValidateSurface_Positive asserts a surface with every required field
// set passes ValidateSurface with no error.
func TestValidateSurface_Positive(t *testing.T) {
	v := surface.NewDefaultSurfaceValidator()
	if err := v.ValidateSurface(context.Background(), validSurface()); err != nil {
		t.Errorf("valid surface must pass validator: %v", err)
	}
}

// TestValidateSurface_ProcessIDRequired asserts the Issue #33 tightening:
// a surface with empty ProcessID is rejected.
func TestValidateSurface_ProcessIDRequired(t *testing.T) {
	v := surface.NewDefaultSurfaceValidator()
	s := validSurface()
	s.ProcessID = ""

	err := v.ValidateSurface(context.Background(), s)
	if err == nil {
		t.Fatal("expected error for empty ProcessID, got nil")
	}
	if !strings.Contains(err.Error(), "process_id") {
		t.Errorf("error message should mention process_id, got: %v", err)
	}
}

// TestValidateSurface_RejectsNegativeFields confirms the pre-existing
// checks still fire after the ProcessID check is added (guard against
// accidentally short-circuiting them).
func TestValidateSurface_RejectsNegativeFields(t *testing.T) {
	v := surface.NewDefaultSurfaceValidator()

	cases := []struct {
		name   string
		mutate func(*surface.DecisionSurface)
		want   string
	}{
		{"nil-surface", func(s *surface.DecisionSurface) {}, "nil"},
		{"empty-id", func(s *surface.DecisionSurface) { s.ID = "" }, "id"},
		{"empty-name", func(s *surface.DecisionSurface) { s.Name = "" }, "name"},
		{"empty-domain", func(s *surface.DecisionSurface) { s.Domain = "" }, "domain"},
		{"empty-process-id", func(s *surface.DecisionSurface) { s.ProcessID = "" }, "process_id"},
		{"confidence-below-zero", func(s *surface.DecisionSurface) { s.MinimumConfidence = -0.1 }, "minimum_confidence"},
		{"confidence-above-one", func(s *surface.DecisionSurface) { s.MinimumConfidence = 1.1 }, "minimum_confidence"},
		{"audit-retention-negative", func(s *surface.DecisionSurface) { s.AuditRetentionHours = -1 }, "audit_retention_hours"},
		{"audit-retention-between-zero-and-24", func(s *surface.DecisionSurface) { s.AuditRetentionHours = 12 }, "audit_retention_hours"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var s *surface.DecisionSurface
			if c.name == "nil-surface" {
				s = nil
			} else {
				s = validSurface()
				c.mutate(s)
			}
			err := v.ValidateSurface(context.Background(), s)
			if err == nil {
				t.Fatalf("expected error mentioning %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected error mentioning %q, got: %v", c.want, err)
			}
		})
	}
}
