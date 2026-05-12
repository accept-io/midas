package failmode

// triggers.go — D29k FailModePolicy trigger taxonomy.
//
// Centralises the trigger-to-correctness-class mapping that earlier
// tranches (D29c-2 for policy_evaluator_error, D29j for
// authority_resolution_failure) hard-coded at each runtime call site.
//
// The taxonomy is intentionally minimal: a private map plus two
// exported helpers. Runtime code consults CorrectnessClassForTrigger
// instead of naming CorrectnessClassResource at the call site; tests
// pin the supported set and the mapping table directly.
//
// D29k is a refactor: the supported set is the same two triggers and
// the mapping is unchanged. Adding or remapping a trigger is an
// out-of-tranche change that must be paired with a brief.

// TriggerDefinition describes a single FailModePolicy trigger
// condition and the correctness class it resolves to. The struct is
// exported so test code can compare definitions structurally, but the
// production registry below is private — runtime code reaches the
// data through the helper functions only.
type TriggerDefinition struct {
	Condition        TriggerCondition
	CorrectnessClass CorrectnessClass
}

// triggerDefinitions is the authoritative registry of supported
// FailModePolicy triggers. New triggers must be added here as part of
// the same change that introduces their runtime call site; the
// taxonomy and the dispatcher must not drift.
//
// Order in the source file mirrors introduction order; callers that
// need a deterministic slice use SupportedTriggerConditions, which
// returns the slice in that order.
var triggerDefinitions = map[TriggerCondition]TriggerDefinition{
	FailModePolicyTriggerPolicyEvaluatorError: {
		Condition:        FailModePolicyTriggerPolicyEvaluatorError,
		CorrectnessClass: CorrectnessClassResource,
	},
	FailModePolicyTriggerAuthorityResolutionFailure: {
		Condition:        FailModePolicyTriggerAuthorityResolutionFailure,
		CorrectnessClass: CorrectnessClassResource,
	},
}

// supportedTriggerOrder fixes the order SupportedTriggerConditions
// returns. Map iteration order in Go is randomised, so the slice is
// built from this constant list rather than from a range over the
// registry. Adding a new trigger requires appending it here as well
// as inserting the registry entry above.
var supportedTriggerOrder = []TriggerCondition{
	FailModePolicyTriggerPolicyEvaluatorError,
	FailModePolicyTriggerAuthorityResolutionFailure,
}

// CorrectnessClassForTrigger returns the correctness class the
// FailModePolicy runtime should select rules for when the supplied
// trigger condition fires. The second return value is false when the
// trigger is not in the supported set; callers must treat that as a
// no-trigger path (defensive: emit nothing, change no outcome).
//
// Runtime call sites use the supported compile-time constants and
// therefore always observe ok=true. The bool exists so future
// trigger additions cannot silently no-op a wired call site, and so
// fuzz-style tests can pin the unknown-trigger contract.
func CorrectnessClassForTrigger(trigger TriggerCondition) (CorrectnessClass, bool) {
	def, ok := triggerDefinitions[trigger]
	if !ok {
		return "", false
	}
	return def.CorrectnessClass, true
}

// SupportedTriggerConditions returns the supported trigger
// conditions in a stable order (introduction order). The returned
// slice is freshly allocated; callers may mutate it without
// affecting the registry.
func SupportedTriggerConditions() []TriggerCondition {
	out := make([]TriggerCondition, len(supportedTriggerOrder))
	copy(out, supportedTriggerOrder)
	return out
}
