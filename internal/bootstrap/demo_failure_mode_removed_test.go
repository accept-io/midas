package bootstrap

// demo_failure_mode_removed_test.go — D29i-1 bootstrap pin.
//
// The legacy FailureMode assignments were removed from the demo
// seed in D29i-1. The demo seed now relies on FailModePolicy
// references (D29d) for fail-mode governance. This pin guards
// against accidental re-introduction.

import (
	"os"
	"strings"
	"testing"
)

func TestD29i1_DemoSeed_FailureModeRemoved(t *testing.T) {
	body, err := os.ReadFile("demo.go")
	if err != nil {
		t.Fatalf("read demo.go: %v", err)
	}
	src := string(body)
	for _, sub := range []string{
		"FailureMode:",
		"surface.FailureMode",
	} {
		if strings.Contains(src, sub) {
			t.Errorf("demo.go must not contain %q after D29i-1 removal", sub)
		}
	}
}
