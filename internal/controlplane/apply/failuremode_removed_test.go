package apply

// failuremode_removed_test.go — D29i-1 source pins for the control-
// plane removal of surface.FailureMode.
//
// After D29i-1:
//   - SurfaceSpec must not declare a FailureMode field or a
//     failure_mode JSON/YAML tag.
//   - surface_mapper.go must not reference FailureMode or
//     failure_mode (the mapper block and the isValidFailureMode
//     helper were removed).
//   - diff_surface.go must not emit a "spec.failure_mode" diff
//     entry.
//
// Pinning the absence guards against silent re-introduction.

import (
	"os"
	"strings"
	"testing"
)

func TestD29i1_SurfaceSpec_FailureModeFieldRemoved(t *testing.T) {
	body, err := os.ReadFile("../types/documents.go")
	if err != nil {
		t.Fatalf("read documents.go: %v", err)
	}
	src := string(body)
	for _, sub := range []string{
		"FailureMode",
		`json:"failure_mode`,
		`yaml:"failure_mode`,
	} {
		if strings.Contains(src, sub) {
			t.Errorf("controlplane/types/documents.go must not contain %q after D29i-1 removal", sub)
		}
	}
}

func TestD29i1_SurfaceMapper_FailureModeRemoved(t *testing.T) {
	body, err := os.ReadFile("surface_mapper.go")
	if err != nil {
		t.Fatalf("read surface_mapper.go: %v", err)
	}
	src := string(body)
	for _, sub := range []string{
		"FailureMode",
		"failure_mode",
		"isValidFailureMode",
	} {
		if strings.Contains(src, sub) {
			t.Errorf("surface_mapper.go must not contain %q after D29i-1 removal", sub)
		}
	}
}

func TestD29i1_DiffSurface_FailureModeRemoved(t *testing.T) {
	body, err := os.ReadFile("diff_surface.go")
	if err != nil {
		t.Fatalf("read diff_surface.go: %v", err)
	}
	src := string(body)
	for _, sub := range []string{
		"FailureMode",
		"failure_mode",
	} {
		if strings.Contains(src, sub) {
			t.Errorf("diff_surface.go must not contain %q after D29i-1 removal", sub)
		}
	}
}
