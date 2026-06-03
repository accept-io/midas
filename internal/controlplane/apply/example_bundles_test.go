package apply

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewerControlPlaneExamplesPlanAsCreates(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "examples", "control-plane", "*.yaml"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(paths) != 5 {
		t.Fatalf("expected 5 control-plane examples, got %d: %v", len(paths), paths)
	}

	svc := NewService()
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}

			plan, err := svc.PlanBundle(context.Background(), b)
			if err != nil {
				t.Fatalf("PlanBundle: %v", err)
			}
			if len(plan.Entries) != 8 {
				t.Fatalf("expected 8 plan entries, got %d", len(plan.Entries))
			}

			for _, entry := range plan.Entries {
				if entry.Action != ApplyActionCreate {
					t.Fatalf("%s/%s: action = %s, want %s; validation_errors=%v",
						entry.Kind, entry.ID, entry.Action, ApplyActionCreate, entry.ValidationErrors)
				}
			}
		})
	}
}
