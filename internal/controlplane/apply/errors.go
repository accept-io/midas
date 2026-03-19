package apply

import "errors"

// Typed sentinel errors for control-plane apply operations.
//
// Callers should use errors.Is to test for specific conditions. All errors
// preserve the original cause via fmt.Errorf wrapping so the error chain
// remains intact.
var (
	// ErrInvalidBundle is returned when a bundle cannot be parsed or is
	// structurally malformed before validation begins.
	ErrInvalidBundle = errors.New("invalid bundle")

	// ErrValidationFailed is returned when one or more documents in the bundle
	// fail semantic validation. Validation errors are also available via the
	// ApplyResult; this sentinel signals that apply was halted by validation.
	ErrValidationFailed = errors.New("validation failed")

	// ErrDuplicateResource is returned when a bundle contains more than one
	// document with the same kind and ID.
	ErrDuplicateResource = errors.New("duplicate resource")

	// ErrResourceConflict is returned when a resource cannot be applied because
	// a conflicting version already exists in the store.
	ErrResourceConflict = errors.New("resource conflict")

	// ErrReferentialIntegrity is returned when a document references a resource
	// that does not exist (e.g. a Grant pointing to a missing Profile).
	ErrReferentialIntegrity = errors.New("referential integrity violation")

	// ErrUnsupportedUpdate is returned when an apply operation attempts a
	// change that is not permitted on the target resource type.
	ErrUnsupportedUpdate = errors.New("unsupported update")

	// ErrVersionConflict is returned when the submitted document version does
	// not match the stored version.
	ErrVersionConflict = errors.New("version conflict")

	// ErrNotFound is returned by repository lookups that produced no result.
	ErrNotFound = errors.New("not found")
)
