package decision

// FailureClass groups failure categories by their governance correctness
// class. The five classes derive from D27i-a-rev1 §2 and constrain the
// max-permissible fail-mode per class.
//
// In chunk 1 of F1 (D27i-c) the class surfaces as one new field on the
// evaluation_failed slog event and through ClassifyFailure. Class-aware
// HTTP error mapping (chunk 2), the audit_status response marker
// (chunk 3), and the Prometheus correctness_class label (chunk 4) consume
// the same classification but are not introduced by this chunk.
type FailureClass string

const (
	FailureClassGovernanceIntegrity FailureClass = "governance_integrity"
	FailureClassPersistence         FailureClass = "persistence"
	FailureClassInput               FailureClass = "input"
	FailureClassResource            FailureClass = "resource"
	FailureClassConsistency         FailureClass = "consistency"
)

// failureClassByCategory maps each FailureCategory to its correctness
// class per D27i-a-rev1 §2.2. The mapping is exhaustive and table-driven:
// new FailureCategory* constants must add an entry in the same change.
//
// The three categories that are defined in code but not currently emitted
// from the inline orchestrator path (PolicyEvaluation, ResolveReview,
// Unknown) are mapped for completeness; D27i-a-rev1 §2.2 documents that
// status. Mapping coverage is not contingent on inline reachability.
var failureClassByCategory = map[FailureCategory]FailureClass{
	FailureCategoryEnvelopePersistence: FailureClassGovernanceIntegrity,
	FailureCategoryAuditAppend:         FailureClassGovernanceIntegrity,
	FailureCategoryInvalidTransition:   FailureClassConsistency,
	FailureCategoryAuthorityResolution: FailureClassConsistency,
	FailureCategoryIdempotencyConflict: FailureClassInput,
	FailureCategoryPolicyEvaluation:    FailureClassResource,
	FailureCategoryResolveReview:       FailureClassConsistency,
	FailureCategoryUnknown:             FailureClassConsistency,
}

// ClassifyFailure returns both the failure category and its correctness
// class for an evaluation failure. The category matches the string
// returned by the existing classifyFailure helper; the class is looked up
// in the rev1 §2.2 mapping. For nil err, both return values are empty.
//
// Composition rather than replacement: classifyFailure remains the
// authoritative single-string classifier (it backs the failure_kind label
// on D27c metrics); ClassifyFailure adds a second axis without renaming
// or restructuring its sibling.
func ClassifyFailure(err error) (FailureCategory, FailureClass) {
	if err == nil {
		return "", ""
	}
	cat := FailureCategory(classifyFailure(err))
	if cat == "" {
		return "", ""
	}
	cls, ok := failureClassByCategory[cat]
	if !ok {
		// Defensive: if classifyFailure ever returns a string outside the
		// FailureCategory* enum, treat it as Consistency. The same safe
		// posture applies to the documented FailureCategoryUnknown
		// mapping — uncategorised failures are not degradation-eligible.
		return cat, FailureClassConsistency
	}
	return cat, cls
}
