package controlaudit

import "context"

// ListFilter specifies zero or more constraints for listing control-plane audit
// records. Unset (zero-value) fields are treated as "no constraint".
type ListFilter struct {
	// ResourceKind constrains results to a specific resource kind
	// (e.g. "surface", "profile").
	ResourceKind string

	// ResourceID constrains results to a specific resource logical ID.
	ResourceID string

	// Actor constrains results to records emitted by a specific actor.
	Actor string

	// Action constrains results to a specific action constant.
	Action Action

	// Limit caps the number of records returned. Zero means "use default".
	// Values above MaxListLimit are clamped to MaxListLimit by the repository.
	Limit int
}

// DefaultListLimit is the number of records returned when Limit is zero.
const DefaultListLimit = 50

// MaxListLimit is the upper bound enforced by all repository implementations.
const MaxListLimit = 500

// Repository is the append-only persistence interface for control-plane audit
// records. Implementations must not support update or delete operations.
type Repository interface {
	// Append persists a single audit record. The record is immutable after append.
	Append(ctx context.Context, rec *ControlAuditRecord) error

	// List returns audit records in descending occurred_at order (newest first).
	// The filter may constrain results; a zero-value filter returns all records
	// up to the effective limit.
	List(ctx context.Context, f ListFilter) ([]*ControlAuditRecord, error)
}
