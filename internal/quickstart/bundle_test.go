package quickstart_test

import (
	"bytes"
	"sort"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
	"github.com/accept-io/midas/internal/quickstart"
)

// TestBundle_Parses asserts the embedded YAML stream parses into a
// non-empty slice of ParsedDocument values.
func TestBundle_Parses(t *testing.T) {
	docs, err := parser.ParseYAMLStream(quickstart.Bundle())
	if err != nil {
		t.Fatalf("ParseYAMLStream: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected non-empty parsed bundle")
	}
}

// TestBundle_Validates asserts the bundle survives validate.ValidateBundle
// with zero validation errors. A failure here indicates the bundle's
// content has drifted from the apply path's validation rules.
func TestBundle_Validates(t *testing.T) {
	docs, err := parser.ParseYAMLStream(quickstart.Bundle())
	if err != nil {
		t.Fatalf("ParseYAMLStream: %v", err)
	}
	verrs := validate.ValidateBundle(docs)
	if len(verrs) > 0 {
		for _, ve := range verrs {
			t.Errorf("validation error: kind=%s id=%s field=%s msg=%s",
				ve.Kind, ve.ID, ve.Field, ve.Message)
		}
		t.Fatalf("ValidateBundle returned %d error(s)", len(verrs))
	}
}

// TestBundle_KindCounts locks in the structural-skeleton scope: 2 BS,
// 4 Cap, 5 BSC, 4 Process, 6 Surface; zero Authority Kinds. Adjusting the
// bundle's scope must update this test deliberately.
func TestBundle_KindCounts(t *testing.T) {
	docs, err := parser.ParseYAMLStream(quickstart.Bundle())
	if err != nil {
		t.Fatalf("ParseYAMLStream: %v", err)
	}

	counts := map[string]int{}
	for _, d := range docs {
		counts[d.Kind]++
	}

	want := map[string]int{
		types.KindBusinessService:           2,
		types.KindCapability:                4,
		types.KindBusinessServiceCapability: 5,
		types.KindProcess:                   4,
		types.KindSurface:                   6,
	}
	for kind, n := range want {
		if got := counts[kind]; got != n {
			t.Errorf("Kind=%s: want %d, got %d", kind, n, got)
		}
	}
	for _, kind := range []string{types.KindAgent, types.KindProfile, types.KindGrant} {
		if got := counts[kind]; got != 0 {
			t.Errorf("Kind=%s: want 0 (structural skeleton excludes Authority), got %d", kind, got)
		}
	}
	if total := len(docs); total != 21 {
		t.Errorf("total docs: want 21, got %d", total)
	}
}

// TestBundle_AnchorCapabilityIDPresent asserts the preflight anchor
// constant matches an actual Capability in the bundle. If the bundle is
// reordered or the anchor renamed, this test catches the drift.
func TestBundle_AnchorCapabilityIDPresent(t *testing.T) {
	docs, err := parser.ParseYAMLStream(quickstart.Bundle())
	if err != nil {
		t.Fatalf("ParseYAMLStream: %v", err)
	}
	for _, d := range docs {
		if d.Kind == types.KindCapability && d.ID == quickstart.AnchorCapabilityID {
			return
		}
	}
	// Build a sorted list of Capability IDs to make the failure
	// message useful when the constant has drifted.
	var capIDs []string
	for _, d := range docs {
		if d.Kind == types.KindCapability {
			capIDs = append(capIDs, d.ID)
		}
	}
	sort.Strings(capIDs)
	t.Fatalf("anchor capability %q not found in bundle. Capabilities present: %v",
		quickstart.AnchorCapabilityID, capIDs)
}

// TestBundle_DoesNotContainRejectedSyntheticActor asserts the embedded
// bundle bytes contain no reference to the rejected `system:quickstart`
// synthetic actor pattern. The bundle ships only structural content.
func TestBundle_DoesNotContainRejectedSyntheticActor(t *testing.T) {
	if bytes.Contains(quickstart.Bundle(), []byte("system:quickstart")) {
		t.Fatal(`bundle bytes contain forbidden literal "system:quickstart"; ` +
			`the audit attribution string is "cli:init-quickstart" and lives only ` +
			`in cmd/midas/init_quickstart.go, not in bundle data`)
	}
}
