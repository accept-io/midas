package governancecoverage

import (
	"sort"

	"github.com/accept-io/midas/internal/governanceexpectation"
)

// Match describes a single GovernanceExpectation that matched the
// runtime input. It carries enough identity for downstream consumers
// (#54 coverage events) to record which expectation matched without a
// second repository round-trip.
type Match struct {
	ExpectationID     string
	Version           int
	RequiredSurfaceID string
	ConditionType     governanceexpectation.ConditionType
}

// Matcher is the pure, deterministic matching primitive. It performs no
// I/O, emits no events, and never returns a package-level error. A
// malformed, unknown, or structurally-incomplete candidate becomes a
// non-match for that one row and matching continues for the rest.
type Matcher struct{}

// NewMatcher constructs a Matcher. The struct has no state today; the
// constructor exists so #54+ can add configuration without breaking
// callers.
func NewMatcher() *Matcher {
	return &Matcher{}
}

// Match returns every candidate that satisfies the input under the
// active-at-time, scope, and condition predicates. Output is sorted
// lexicographically by ExpectationID. When more than one candidate
// shares a logical ExpectationID, only the highest Version is returned;
// this is a defensive policy against a domain invariant violation
// (multiple active versions per logical ID).
//
// The matcher applies these filters in order, skipping the candidate on
// the first failed check:
//
//  1. ScopeKind must be ScopeKindProcess (defensive — apply (#52)
//     rejects other scope kinds, but the matcher must still skip them).
//  2. ScopeID must equal Input.ProcessID.
//  3. Status must be ExpectationStatusActive.
//  4. EffectiveDate <= Input.ObservedAt.
//  5. EffectiveUntil == nil OR > Input.ObservedAt.
//  6. RetiredAt == nil.
//  7. RequiredSurfaceID must be non-empty (defensive — validation
//     should prevent it).
//  8. ConditionType must be a known type. Unknown types are skipped.
//  9. ConditionPayload must decode under the typed grammar; an
//     undecodable payload (malformed JSON, unknown fields, trailing
//     tokens) makes the candidate non-matching.
//  10. Decoded payload must evaluate true under the candidate's
//     condition type.
func (m *Matcher) Match(in Input, candidates []*governanceexpectation.GovernanceExpectation) []Match {
	// First pass: collect every candidate that passes all predicates.
	// Indexed by logical ID so we can deduplicate to the highest
	// version per ID before returning.
	bestPerID := make(map[string]Match)
	for _, e := range candidates {
		if e == nil {
			continue
		}
		if !passesScopeAndLifecycle(e, in) {
			continue
		}
		if e.RequiredSurfaceID == "" {
			continue
		}
		if !evaluatesToTrue(e, in) {
			continue
		}

		current, seen := bestPerID[e.ID]
		if !seen || e.Version > current.Version {
			bestPerID[e.ID] = Match{
				ExpectationID:     e.ID,
				Version:           e.Version,
				RequiredSurfaceID: e.RequiredSurfaceID,
				ConditionType:     e.ConditionType,
			}
		}
	}

	// Stable lex sort by ExpectationID. Determinism is part of the
	// matcher's contract — downstream coverage consumers (#54) should
	// be able to rely on a stable order for replay-style audit.
	out := make([]Match, 0, len(bestPerID))
	for _, m := range bestPerID {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ExpectationID < out[j].ExpectationID
	})
	return out
}

// passesScopeAndLifecycle is the structural gate every candidate must
// clear before payload decode. Pulled into a helper so the order of
// checks is reviewable in one place.
func passesScopeAndLifecycle(e *governanceexpectation.GovernanceExpectation, in Input) bool {
	if e.ScopeKind != governanceexpectation.ScopeKindProcess {
		return false
	}
	if e.ScopeID == "" || e.ScopeID != in.ProcessID {
		return false
	}
	if e.Status != governanceexpectation.ExpectationStatusActive {
		return false
	}
	if e.EffectiveDate.After(in.ObservedAt) {
		return false
	}
	if e.EffectiveUntil != nil && !e.EffectiveUntil.After(in.ObservedAt) {
		return false
	}
	if e.RetiredAt != nil {
		return false
	}
	return true
}

// evaluatesToTrue is the per-ConditionType dispatch. Today only
// risk_condition exists; an unknown type makes the candidate
// non-matching (no error). Adding a new ConditionType is a code change
// to this switch plus a new <type>.go file in this package.
func evaluatesToTrue(e *governanceexpectation.GovernanceExpectation, in Input) bool {
	switch e.ConditionType {
	case governanceexpectation.ConditionTypeRiskCondition:
		rc, ok := decodeRiskCondition(e.ConditionPayload)
		if !ok {
			return false
		}
		return rc.matches(in)
	default:
		return false
	}
}
