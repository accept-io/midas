// Package inference provides pure helper functions for deriving structural
// identifiers from surface IDs. All functions are stateless and have no
// database access, no side effects, and no external dependencies.
package inference

import (
	"errors"
	"fmt"
	"strings"
)

// ValidateSurfaceID checks that surfaceID satisfies the v1 surface ID contract.
//
// Allowed characters: lowercase a-z, digits 0-9, dot (.), underscore (_), hyphen (-).
//
// Structural rules:
//   - must not be empty
//   - first character must be alphanumeric (a-z or 0-9)
//   - last character must be alphanumeric (a-z or 0-9)
//   - must not contain consecutive dots (..)
//
// Regex equivalent: ^[a-z0-9][a-z0-9._-]*[a-z0-9]$|^[a-z0-9]$
//
// Errors are descriptive and identify the specific rule that was violated.
func ValidateSurfaceID(surfaceID string) error {
	if surfaceID == "" {
		return errors.New("surface ID must not be empty")
	}

	runes := []rune(surfaceID)
	first := runes[0]
	last := runes[len(runes)-1]

	if !isAlphanumeric(first) {
		if first == '.' {
			return errors.New("surface ID must not start with '.'")
		}
		if first >= 'A' && first <= 'Z' {
			return fmt.Errorf("surface ID must be lowercase; found uppercase %q — allowed characters are a-z, 0-9, '.', '_', '-'", string(first))
		}
		return fmt.Errorf("surface ID must start with an alphanumeric character (a-z or 0-9), got %q", string(first))
	}

	if len(runes) > 1 && !isAlphanumeric(last) {
		return fmt.Errorf("surface ID must end with an alphanumeric character (a-z or 0-9), got %q", string(last))
	}

	if strings.Contains(surfaceID, "..") {
		return errors.New(`surface ID must not contain consecutive dots ("..")`)
	}

	for i, r := range surfaceID {
		if r >= 'A' && r <= 'Z' {
			return fmt.Errorf("surface ID must be lowercase; found uppercase %q at position %d — allowed characters are a-z, 0-9, '.', '_', '-'", string(r), i)
		}
		if !isValidRune(r) {
			return fmt.Errorf("surface ID contains invalid character %q at position %d — allowed characters are a-z, 0-9, '.', '_', '-'", string(r), i)
		}
	}

	return nil
}

// InferStructure deterministically derives a capabilityID and processID from
// a surfaceID by splitting on the first dot.
//
// The caller is responsible for validating surfaceID with ValidateSurfaceID
// before calling this function. InferStructure does not validate its input: it
// never panics on invalid or empty input, but the returned IDs may be
// malformed if the input is malformed.
//
// Inference rules:
//   - capabilityID = "auto:" + the segment before the first "."; if no "."
//     is present, capabilityID = "auto:general"
//   - processID = "auto:" + surfaceID (always the full input)
//
// Examples:
//
//	InferStructure("loan.approve")         → "auto:loan",    "auto:loan.approve"
//	InferStructure("claims.review.manual") → "auto:claims",  "auto:claims.review.manual"
//	InferStructure("payment_execute")      → "auto:general", "auto:payment_execute"
//	InferStructure("a.b.c")               → "auto:a",       "auto:a.b.c"
//	InferStructure("")                     → "auto:general", "auto:"
func InferStructure(surfaceID string) (capabilityID, processID string) {
	processID = "auto:" + surfaceID
	dotIdx := strings.IndexByte(surfaceID, '.')
	if dotIdx == -1 {
		capabilityID = "auto:general"
	} else {
		capabilityID = "auto:" + surfaceID[:dotIdx]
	}
	return capabilityID, processID
}

// IsReservedNamespace reports whether id belongs to the "auto:" reserved
// namespace. IDs in this namespace are managed by the inference system and
// must not be created directly by operators.
//
// Examples:
//
//	IsReservedNamespace("auto:loan")   → true
//	IsReservedNamespace("auto:a.b.c") → true
//	IsReservedNamespace("lending-v1") → false
//	IsReservedNamespace("automatic")  → false  // "auto" without ":" is not reserved
//	IsReservedNamespace("")           → false
func IsReservedNamespace(id string) bool {
	return strings.HasPrefix(id, "auto:")
}

// isAlphanumeric reports whether r is a lowercase ASCII letter or ASCII digit.
func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// isValidRune reports whether r is a character allowed anywhere in a surface ID.
func isValidRune(r rune) bool {
	return isAlphanumeric(r) || r == '.' || r == '_' || r == '-'
}
