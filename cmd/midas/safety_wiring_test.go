package main

import (
	"os"
	"strings"
	"testing"
)

// TestSafetyWiring_HandlerTimeoutPlumbed pins that cmd/midas/main.go calls
// srv.WithHandlerTimeout with the configured value. D27d added the safety
// middleware; this test prevents a future refactor from silently dropping
// the wiring (which would leave handlers without a wall-clock bound).
func TestSafetyWiring_HandlerTimeoutPlumbed(t *testing.T) {
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(body)

	required := []string{
		"srv.WithHandlerTimeout(cfg.Server.HandlerTimeout.D())",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("main.go must contain %q (D27d safety-wiring regression)", want)
		}
	}
}
