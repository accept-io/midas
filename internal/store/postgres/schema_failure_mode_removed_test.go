package postgres

// schema_failure_mode_removed_test.go — D29i-1 schema source pin.
//
// The legacy decision_surfaces.failure_mode column and its CHECK
// constraint were removed by D29i-1. The Postgres Surface
// repository never read or wrote the column (verified in D29i
// Step 0), so removal is binary-compatible with existing data;
// the column simply ceases to be referenced. A fresh deployment's
// schema must no longer declare it.

import (
	"os"
	"strings"
	"testing"
)

func TestD29i1_Schema_FailureModeColumnRemoved(t *testing.T) {
	body, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	src := string(body)
	for _, sub := range []string{
		"failure_mode TEXT",
		"chk_surfaces_failure_mode",
		// Catch any future re-introduction of a CHECK form.
		"failure_mode IN",
	} {
		if strings.Contains(src, sub) {
			t.Errorf("schema.sql must not contain %q after D29i-1 removal", sub)
		}
	}
}
