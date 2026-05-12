package failmode_test

// triggers_test.go — D29k pins on the FailModePolicy trigger taxonomy.
//
// Asserts:
//
//   - SupportedTriggerConditions returns exactly the two triggers
//     introduced through D29c-2 and D29j: no expansion in D29k.
//   - The mapping table is unchanged: both triggers map to
//     CorrectnessClassResource. D29k is a refactor.
//   - CorrectnessClassForTrigger returns ok=false for an unrecognised
//     trigger; runtime call sites that observe that path must treat
//     it as a no-trigger no-op, never panic.
//   - SupportedTriggerConditions returns a deterministic order on
//     every invocation, so test output and future doc generation
//     stay stable across runs. The slice is also defensively a fresh
//     copy.

import (
	"reflect"
	"testing"

	"github.com/accept-io/midas/internal/failmode"
)

func TestTriggerTaxonomy_SupportedTriggersExactly(t *testing.T) {
	want := []failmode.TriggerCondition{
		failmode.FailModePolicyTriggerPolicyEvaluatorError,
		failmode.FailModePolicyTriggerAuthorityResolutionFailure,
	}
	got := failmode.SupportedTriggerConditions()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedTriggerConditions diverged from the pinned set:\n got=%v\nwant=%v",
			got, want)
	}
}

func TestTriggerTaxonomy_CorrectnessClassMappings(t *testing.T) {
	cases := []struct {
		trigger failmode.TriggerCondition
		want    failmode.CorrectnessClass
	}{
		{failmode.FailModePolicyTriggerPolicyEvaluatorError, failmode.CorrectnessClassResource},
		{failmode.FailModePolicyTriggerAuthorityResolutionFailure, failmode.CorrectnessClassResource},
	}
	for _, c := range cases {
		t.Run(string(c.trigger), func(t *testing.T) {
			got, ok := failmode.CorrectnessClassForTrigger(c.trigger)
			if !ok {
				t.Fatalf("CorrectnessClassForTrigger(%q): ok=false; want true", c.trigger)
			}
			if got != c.want {
				t.Errorf("CorrectnessClassForTrigger(%q): got %q, want %q",
					c.trigger, got, c.want)
			}
		})
	}

	// Cross-check: every supported trigger from the slice helper
	// also resolves through the mapping helper. This guards against
	// the order slice and the registry map drifting apart.
	for _, tc := range failmode.SupportedTriggerConditions() {
		if _, ok := failmode.CorrectnessClassForTrigger(tc); !ok {
			t.Errorf("supported trigger %q not present in mapping helper", tc)
		}
	}
}

func TestTriggerTaxonomy_UnknownTrigger(t *testing.T) {
	cases := []failmode.TriggerCondition{
		"",
		"unknown",
		"POLICY_EVALUATOR_ERROR",                 // wrong case
		"policy_evaluator_error_v2",              // close-but-not
		"authority_resolution_failure_unhandled", // close-but-not
		"drift_observation",                      // brief explicitly excludes
		"validation_failure",                     // brief explicitly excludes
	}
	for _, trigger := range cases {
		t.Run(string(trigger), func(t *testing.T) {
			cls, ok := failmode.CorrectnessClassForTrigger(trigger)
			if ok {
				t.Errorf("CorrectnessClassForTrigger(%q): want ok=false, got ok=true cls=%q",
					trigger, cls)
			}
			if cls != "" {
				t.Errorf("CorrectnessClassForTrigger(%q): unknown trigger must return empty CorrectnessClass; got %q",
					trigger, cls)
			}
		})
	}
}

func TestTriggerTaxonomy_SupportedTriggerConditionsDeterministicOrder(t *testing.T) {
	first := failmode.SupportedTriggerConditions()
	for i := 0; i < 16; i++ {
		next := failmode.SupportedTriggerConditions()
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("SupportedTriggerConditions order is non-deterministic; iteration %d diverged:\nfirst=%v\nnext =%v",
				i, first, next)
		}
	}

	// Defensive: the returned slice must be a copy, not the
	// underlying registry slice. Mutating the result must not affect
	// subsequent calls.
	if len(first) == 0 {
		t.Fatalf("supported trigger set is empty")
	}
	first[0] = "MUTATED_BY_TEST"
	fresh := failmode.SupportedTriggerConditions()
	if fresh[0] == "MUTATED_BY_TEST" {
		t.Errorf("SupportedTriggerConditions returned a shared slice; caller mutation leaked back into the registry")
	}
}
