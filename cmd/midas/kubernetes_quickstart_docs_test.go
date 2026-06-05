package main

import (
	"os"
	"strings"
	"testing"
)

func TestKubernetesQuickstart_KeyReferencesExist(t *testing.T) {
	quickstart := "../../docs/getting-started/kubernetes.md"
	body, err := os.ReadFile(quickstart)
	if err != nil {
		t.Fatalf("read %s: %v", quickstart, err)
	}
	doc := string(body)

	tests := []struct {
		name      string
		reference string
		path      string
		wantDir   bool
	}{
		{
			name:      "helm chart",
			reference: "charts/midas",
			path:      "../../charts/midas",
			wantDir:   true,
		},
		{
			name:      "helm chart readme",
			reference: "charts/midas/README.md",
			path:      "../../charts/midas/README.md",
		},
		{
			name:      "production values",
			reference: "charts/midas/values-production.yaml",
			path:      "../../charts/midas/values-production.yaml",
		},
		{
			name:      "kind values",
			reference: "examples/kubernetes/kind-values.yaml",
			path:      "../../examples/kubernetes/kind-values.yaml",
		},
		{
			name:      "control-plane examples",
			reference: "examples/control-plane",
			path:      "../../examples/control-plane",
			wantDir:   true,
		},
		{
			name:      "control-plane walkthrough",
			reference: "docs/examples/control-plane.md",
			path:      "../../docs/examples/control-plane.md",
		},
		{
			name:      "platform contract",
			reference: "docs/core/platform-contract.md",
			path:      "../../docs/core/platform-contract.md",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(doc, tc.reference) {
				t.Fatalf("%s must reference %q", quickstart, tc.reference)
			}

			info, err := os.Stat(tc.path)
			if err != nil {
				t.Fatalf("stat %s: %v", tc.path, err)
			}
			if tc.wantDir && !info.IsDir() {
				t.Fatalf("%s must be a directory", tc.path)
			}
			if !tc.wantDir && info.IsDir() {
				t.Fatalf("%s must be a file", tc.path)
			}
		})
	}
}
