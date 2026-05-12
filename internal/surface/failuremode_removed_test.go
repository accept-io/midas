package surface_test

// failuremode_removed_test.go — D29i-1 source pins.
//
// surface.FailureMode was the legacy surface-level policy-evaluator
// failure posture. It was runtime-dead (no decision-path read site)
// and was superseded by FailModePolicy through D29b–D29h. D29i-1
// removed it from the domain model.
//
// These pins make accidental reintroduction loud: a future commit
// that re-adds the type, the field, or either constant must update
// this file at the same time, forcing reviewer attention.

import (
	"os"
	"strings"
	"testing"
)

func TestD29i1_SurfaceDomain_FailureModeRemoved(t *testing.T) {
	body, err := os.ReadFile("surface.go")
	if err != nil {
		t.Fatalf("read surface.go: %v", err)
	}
	src := string(body)

	forbidden := []string{
		"type FailureMode",
		"FailureModeOpen",
		"FailureModeClosed",
		"FailureMode FailureMode",
		"failure_mode", // JSON / YAML tag form
	}
	for _, sub := range forbidden {
		if strings.Contains(src, sub) {
			t.Errorf("surface.go must not contain %q after D29i-1 removal (use FailModePolicy instead)", sub)
		}
	}
}
