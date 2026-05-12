package quickstart_test

// bundle_failure_mode_removed_test.go — D29i-1 quickstart pin.
//
// The legacy failure_mode field was stripped from the quickstart
// bundle in D29i-1. The bundle is loaded as YAML and applied
// through the strict-known-fields parser; a future commit
// re-adding failure_mode would silently fail at apply time on
// fresh deployments. This pin makes such a re-introduction loud
// at test time.

import (
	"os"
	"strings"
	"testing"
)

func TestD29i1_QuickstartBundle_FailureModeRemoved(t *testing.T) {
	body, err := os.ReadFile("bundle.yaml")
	if err != nil {
		t.Fatalf("read bundle.yaml: %v", err)
	}
	if strings.Contains(string(body), "failure_mode") {
		t.Error("internal/quickstart/bundle.yaml must not contain 'failure_mode' after D29i-1 removal")
	}
}
