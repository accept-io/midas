package main

// wiring_failmode_test.go — D29d source pins for the production
// FailModePolicy wiring in cmd/midas.
//
// cmd/midas is package main; both wiring statements live inside
// func main() and are not directly callable from a unit test.
// Source-pin tests are the project's idiomatic shape for catching
// regressions of this exact literal — mirrors
// wiring_drift_source_test.go for the Drift read service.
//
// What these tests catch:
//   1. A future refactor that drops srv.WithFailModePolicyReadService,
//      silently degrading the /v1/fail_mode_policies/* read API to 501
//      even when a repository is configured.
//   2. A future refactor that drops the orchestrator option for the
//      deployment-default FailModePolicy id, silently disabling the
//      deployment-default level of the fail-mode resolution hierarchy.

import (
	"os"
	"strings"
	"testing"
)

// TestWiring_MainGo_WiresFailModePolicyReadService pins the D29d Part
// A wiring: main.go must construct a FailModePolicyReadService backed
// by repos.FailModePolicies and attach it to the server.
func TestWiring_MainGo_WiresFailModePolicyReadService(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)
	wantSubstrs := []string{
		"httpapi.NewFailModePolicyReadService(repos.FailModePolicies)",
		"srv.WithFailModePolicyReadService(",
	}
	for _, want := range wantSubstrs {
		if !strings.Contains(body, want) {
			t.Errorf("cmd/midas/main.go must wire the FailModePolicy read service; missing literal:\n  %s", want)
		}
	}
}

// TestWiring_MainGo_WiresFailModeDeploymentDefault pins the D29d Part
// B wiring: main.go must pipe cfg.FailMode.DeploymentDefaultPolicyID
// into the orchestrator via WithFailModeDeploymentDefaultPolicyID.
// Empty config is the default and produces a no-op call — what matters
// is that the wiring itself is present so the operator-controlled value
// can never be silently dropped.
func TestWiring_MainGo_WiresFailModeDeploymentDefault(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)
	want := "WithFailModeDeploymentDefaultPolicyID(cfg.FailMode.DeploymentDefaultPolicyID)"
	if !strings.Contains(body, want) {
		t.Errorf("cmd/midas/main.go must wire the deployment-default FailModePolicy id into the orchestrator; missing literal:\n  %s", want)
	}
}
