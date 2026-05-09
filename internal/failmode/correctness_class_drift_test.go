package failmode_test

// This is the only file in internal/failmode that imports internal/decision.
// Production code in the package (policy.go, validate.go) defines its own
// CorrectnessClass type so structural domain does not depend on runtime
// evaluation. This test asserts the two five-element string sets remain
// identical; if either side adds, removes, or renames a value, the drift
// detector fails loudly.

import (
	"sort"
	"testing"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/failmode"
)

func TestCorrectnessClass_MatchesDecisionFailureClass(t *testing.T) {
	failmodeSet := []string{
		string(failmode.CorrectnessClassGovernanceIntegrity),
		string(failmode.CorrectnessClassPersistence),
		string(failmode.CorrectnessClassInput),
		string(failmode.CorrectnessClassResource),
		string(failmode.CorrectnessClassConsistency),
	}

	decisionSet := []string{
		string(decision.FailureClassGovernanceIntegrity),
		string(decision.FailureClassPersistence),
		string(decision.FailureClassInput),
		string(decision.FailureClassResource),
		string(decision.FailureClassConsistency),
	}

	if len(failmodeSet) != 5 {
		t.Fatalf("failmode CorrectnessClass cardinality drift: want 5, got %d", len(failmodeSet))
	}
	if len(decisionSet) != 5 {
		t.Fatalf("decision FailureClass cardinality drift: want 5, got %d", len(decisionSet))
	}

	sort.Strings(failmodeSet)
	sort.Strings(decisionSet)

	for i := range failmodeSet {
		if failmodeSet[i] != decisionSet[i] {
			t.Errorf("drift at index %d: failmode=%q decision=%q", i, failmodeSet[i], decisionSet[i])
		}
	}

	// Spot-check a known value pair to defend against both sides drifting in
	// the same direction with the same name but different semantics — the
	// pair should still be a string-equal pair.
	if string(failmode.CorrectnessClassResource) != string(decision.FailureClassResource) {
		t.Errorf("resource string drift: failmode=%q decision=%q",
			failmode.CorrectnessClassResource, decision.FailureClassResource)
	}
}
