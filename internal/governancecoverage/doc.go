// Package governancecoverage implements declared-expectation matching for
// the Governance Coverage Assurance (GCA) epic.
//
// The package answers exactly one question: given a runtime evaluation
// context and the set of declared GovernanceExpectations, which
// expectations apply right now? It does not emit events, does not detect
// missing surfaces, does not enforce decisions, and does not call into
// the evaluation orchestrator. Those concerns belong to later issues
// (#54 coverage-event emission, #55 missing-surface detection, #56 read
// model and Explorer).
//
// Two layers:
//
//   - Matcher (matcher.go): pure, deterministic, no I/O. Takes an
//     Input plus a candidate slice and returns matches sorted by
//     ExpectationID. Suitable for unit tests and direct in-process
//     use.
//
//   - Service (service.go): wires the
//     governanceexpectation.Repository's ListActiveByScope query to
//     the pure matcher. Suitable for the orchestrator wiring that #54
//     will introduce; today it is exercised only by tests.
//
// Defensive posture. Matching is intentionally quiet: malformed payloads,
// unknown ConditionType values, unsupported field names in
// condition_payload, and structurally-incomplete expectations are
// silently skipped — they make that one expectation non-matching but
// must never abort matching for other expectations. There is no
// package-level error path from the pure matcher.
//
// Scope. #52 supports only ScopeKindProcess at the apply boundary; the
// matcher mirrors this and skips business_service and capability scopes
// defensively. The matcher will never match an expectation whose scope
// kind is not "process".
//
// Grammar. The only ConditionType accepted today is
// ConditionTypeRiskCondition. Its payload is a flat key/value JSON
// object; see riskcondition.go for the supported fields and
// comparators. The grammar deliberately avoids AST shapes, custom
// operators, regex, and arbitrary expression evaluation.
package governancecoverage
