package main

// wiring_drift_source_test.go — Drift-1c-fix source pins for the
// production apply.RepositorySet construction sites in cmd/midas.
//
// cmd/midas is package main; the apply.RepositorySet is built inside
// func main() (and inside runQuickstart in init_quickstart.go), neither
// of which is directly callable from a unit test without invoking the
// full binary. A source-pin test is the project's idiomatic shape for
// catching regressions of this exact literal.
//
// What this test catches: a future refactor of the production daemon
// (or quickstart) wiring that drops the DriftDefinitions field from
// apply.RepositorySet, silently degrading the DriftDefinition apply
// path to validation-only.

import (
	"os"
	"strings"
	"testing"
)

func TestWiring_MainGo_PassesDriftDefinitions(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	wantSubstr := "DriftDefinitions:             repos.DriftDefinitions,"
	if !strings.Contains(string(src), wantSubstr) {
		t.Errorf("cmd/midas/main.go must wire DriftDefinitions through to apply.RepositorySet; missing literal:\n  %s", wantSubstr)
	}
}

func TestWiring_InitQuickstart_PassesDriftDefinitions(t *testing.T) {
	src, err := os.ReadFile("init_quickstart.go")
	if err != nil {
		t.Fatalf("read init_quickstart.go: %v", err)
	}
	wantSubstr := "DriftDefinitions:            repos.DriftDefinitions,"
	if !strings.Contains(string(src), wantSubstr) {
		t.Errorf("cmd/midas/init_quickstart.go must wire DriftDefinitions through to apply.RepositorySet; missing literal:\n  %s", wantSubstr)
	}
}
