package adminaudit

import "context"

// ListFilter specifies zero or more constraints for listing admin-audit
// records. Unset (zero-value) fields are treated as "no constraint".
type ListFilter struct {
	// Action constrains results to a single action enum value.
	Action Action

	// Outcome constrains results to success or failure.
	Outcome Outcome

	// ActorID constrains results to records emitted for a specific actor.
	ActorID string

	// TargetType constrains results to a specific target type.
	TargetType string

	// TargetID constrains results to a specific target ID.
	TargetID string

	// Limit caps the number of records returned. Zero means "use default".
	// Values above MaxListLimit are clamped by the repository.
	Limit int
}

// DefaultListLimit is the number of records returned when Limit is zero.
const DefaultListLimit = 50

// MaxListLimit is the upper bound enforced by all repository implementations.
const MaxListLimit = 500

// Repository is the append-only persistence interface for admin-audit
// records. Implementations must not support update or delete operations.
// The interface surface is intentionally minimal; it mirrors
// controlaudit.Repository to keep operator mental models aligned.
type Repository interface {
	// Append persists a single record. Implementations must not mutate the
	// record after persistence.
	Append(ctx context.Context, rec *AdminAuditRecord) error

	// List returns records in descending occurred_at order (newest first),
	// honouring the filter constraints and limit.
	List(ctx context.Context, f ListFilter) ([]*AdminAuditRecord, error)
}
