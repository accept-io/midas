package authority

import (
	"errors"
	"fmt"
)

// Capability names one of the five discrete actions an AuthorityGrant
// can authorise an agent to take on a decision surface.
//
// The MVP capability set is fixed and exhaustive: a grant either
// carries a capability or it does not, and an agent may only exercise
// the capabilities its grant explicitly carries. New capability values
// are NOT introduced through configuration — extending the set
// requires a domain change here, an OpenAPI enum addition, and a
// Migration/seed update so existing grants gain the new value.
//
// Stop is the most consequential capability: it authorises an agent
// to halt a decision flow (e.g. trigger a kill-switch, freeze further
// automated processing). It is intentionally modelled as a CAPABILITY
// VALUE — never as a failmode.Outcome value, never as a separate
// entity. The orchestrator enforces stop the same way it enforces
// approve / reject / escalate: the grant must explicitly carry the
// capability for the agent to exercise it.
type Capability string

const (
	CapabilityRecommend Capability = "recommend"
	CapabilityApprove   Capability = "approve"
	CapabilityReject    Capability = "reject"
	CapabilityEscalate  Capability = "escalate"
	CapabilityStop      Capability = "stop"
)

// ErrInvalidCapability is wrapped by Validate / ValidateCapabilities
// for any Capability value outside the canonical five.
var ErrInvalidCapability = errors.New("invalid capability")

// ErrDuplicateCapability is wrapped by ValidateCapabilities when the
// same capability appears more than once in a grant's capability set.
// Grants are a SET semantically; duplicates are a domain error rather
// than a silent dedupe so callers don't lose information.
var ErrDuplicateCapability = errors.New("duplicate capability")

// IsValid reports whether c is one of the canonical five Capability
// values. The empty string is NOT valid — callers that mean "no
// capability check requested" must check for empty before invoking
// this helper.
func (c Capability) IsValid() bool {
	switch c {
	case CapabilityRecommend, CapabilityApprove, CapabilityReject,
		CapabilityEscalate, CapabilityStop:
		return true
	default:
		return false
	}
}

// IsValidCapability is the package-level alias for Capability.IsValid,
// matching the agent.IsValidAgentType / failmode.IsValidOutcome
// helper-naming convention used elsewhere in the domain.
func IsValidCapability(c Capability) bool {
	return c.IsValid()
}

// ValidateCapabilities returns nil when caps is a valid grant
// capability set:
//
//   - every entry must be a canonical Capability value;
//   - duplicates are rejected;
//   - the empty slice is valid (a grant with no capabilities is a
//     domain state that produces an Authority Graph warning, but it
//     is not malformed).
//
// The function is total and side-effect-free.
func ValidateCapabilities(caps []Capability) error {
	seen := make(map[Capability]struct{}, len(caps))
	for _, c := range caps {
		if !c.IsValid() {
			return fmt.Errorf("%w: %q", ErrInvalidCapability, string(c))
		}
		if _, dup := seen[c]; dup {
			return fmt.Errorf("%w: %q", ErrDuplicateCapability, string(c))
		}
		seen[c] = struct{}{}
	}
	return nil
}

// HasCapability reports whether the want capability is present in
// caps. O(n) linear scan; grants carry at most 5 capabilities, so a
// map allocation would be more overhead than the scan.
func HasCapability(caps []Capability, want Capability) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}
