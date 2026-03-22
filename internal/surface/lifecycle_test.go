package surface_test

import (
	"errors"
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
