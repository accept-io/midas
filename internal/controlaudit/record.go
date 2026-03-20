// Package controlaudit defines the append-only audit trail for control-plane
// configuration mutations. It is separate from the runtime decision audit
// (internal/audit), which is a hash-chained evaluation audit trail.
//
// A ControlAuditRecord captures who changed what, when, and which version of
// which resource was affected. Records are written on successful persistence
// of a resource, not on attempted operations.
package controlaudit

import (
	"time"

	"github.com/google/uuid"
)

// Action is a typed constant for the kind of control-plane mutation recorded.
type Action string

const (
	// ActionSurfaceCreated is emitted when a new surface version is applied.
	ActionSurfaceCreated Action = "surface.created"

	// ActionProfileCreated is emitted when a profile is applied for the first time (version 1).
	ActionProfileCreated Action = "profile.created"

	// ActionProfileVersioned is emitted when a subsequent profile version (>1) is applied.
	ActionProfileVersioned Action = "profile.versioned"

	// ActionAgentCreated is emitted when an agent is applied.
	ActionAgentCreated Action = "agent.created"

	// ActionGrantCreated is emitted when a grant is applied.
	ActionGrantCreated Action = "grant.created"

	// ActionSurfaceApproved is emitted when a surface is approved and transitions to active.
	ActionSurfaceApproved Action = "surface.approved"

	// ActionSurfaceDeprecated is emitted when a surface is deprecated.
	ActionSurfaceDeprecated Action = "surface.deprecated"
)

// ResourceKind mirrors the control-plane document kinds to avoid a circular import.
const (
	ResourceKindSurface = "surface"
	ResourceKindProfile = "profile"
	ResourceKindAgent   = "agent"
	ResourceKindGrant   = "grant"
)

// Metadata carries structured context attached to a ControlAuditRecord.
// Fields are nullable — only the fields relevant to the action are populated.
type Metadata struct {
	// SurfaceID is the logical surface ID referenced by the mutated resource,
	// populated for profile records to capture the owning surface.
	SurfaceID string `json:"surface_id,omitempty"`

	// DeprecationReason is the operator-supplied reason for deprecation.
	DeprecationReason string `json:"deprecation_reason,omitempty"`

	// SuccessorSurfaceID is the successor surface ID recorded on deprecation.
	SuccessorSurfaceID string `json:"successor_surface_id,omitempty"`
}

// ControlAuditRecord is an immutable control-plane governance event. Once
// appended, it must not be modified. The ID is a UUID assigned at construction.
type ControlAuditRecord struct {
	ID              string    `json:"id"`
	OccurredAt      time.Time `json:"occurred_at"`
	Actor           string    `json:"actor"`
	Action          Action    `json:"action"`
	ResourceKind    string    `json:"resource_kind"`
	ResourceID      string    `json:"resource_id"`
	ResourceVersion *int      `json:"resource_version,omitempty"`
	Summary         string    `json:"summary"`
	Metadata        *Metadata `json:"metadata,omitempty"`
}

func newRecord(actor string, action Action, resourceKind, resourceID string, version *int, summary string, meta *Metadata) *ControlAuditRecord {
	return &ControlAuditRecord{
		ID:              uuid.NewString(),
		OccurredAt:      time.Now().UTC(),
		Actor:           actor,
		Action:          action,
		ResourceKind:    resourceKind,
		ResourceID:      resourceID,
		ResourceVersion: version,
		Summary:         summary,
		Metadata:        meta,
	}
}

func intPtr(v int) *int { return &v }

// NewSurfaceCreatedRecord builds a record for a new surface version being applied.
func NewSurfaceCreatedRecord(actor, surfaceID string, version int) *ControlAuditRecord {
	return newRecord(
		actor,
		ActionSurfaceCreated,
		ResourceKindSurface,
		surfaceID,
		intPtr(version),
		"surface applied: "+surfaceID+" v"+itoa(version),
		nil,
	)
}

// NewProfileCreatedRecord builds a record for a first-time profile creation (version 1).
func NewProfileCreatedRecord(actor, profileID, surfaceID string, version int) *ControlAuditRecord {
	return newRecord(
		actor,
		ActionProfileCreated,
		ResourceKindProfile,
		profileID,
		intPtr(version),
		"profile created: "+profileID+" v"+itoa(version),
		&Metadata{SurfaceID: surfaceID},
	)
}

// NewProfileVersionedRecord builds a record for a subsequent profile version (>1).
func NewProfileVersionedRecord(actor, profileID, surfaceID string, version int) *ControlAuditRecord {
	return newRecord(
		actor,
		ActionProfileVersioned,
		ResourceKindProfile,
		profileID,
		intPtr(version),
		"profile versioned: "+profileID+" v"+itoa(version),
		&Metadata{SurfaceID: surfaceID},
	)
}

// NewAgentCreatedRecord builds a record for an agent being applied.
func NewAgentCreatedRecord(actor, agentID string) *ControlAuditRecord {
	return newRecord(
		actor,
		ActionAgentCreated,
		ResourceKindAgent,
		agentID,
		nil,
		"agent created: "+agentID,
		nil,
	)
}

// NewGrantCreatedRecord builds a record for a grant being applied.
func NewGrantCreatedRecord(actor, grantID string) *ControlAuditRecord {
	return newRecord(
		actor,
		ActionGrantCreated,
		ResourceKindGrant,
		grantID,
		nil,
		"grant created: "+grantID,
		nil,
	)
}

// NewSurfaceApprovedRecord builds a record for a surface approval.
func NewSurfaceApprovedRecord(actor, surfaceID string, version int) *ControlAuditRecord {
	return newRecord(
		actor,
		ActionSurfaceApproved,
		ResourceKindSurface,
		surfaceID,
		intPtr(version),
		"surface approved: "+surfaceID+" v"+itoa(version),
		nil,
	)
}

// NewSurfaceDeprecatedRecord builds a record for a surface deprecation.
func NewSurfaceDeprecatedRecord(actor, surfaceID string, version int, reason, successorID string) *ControlAuditRecord {
	meta := &Metadata{
		DeprecationReason:  reason,
		SuccessorSurfaceID: successorID,
	}
	return newRecord(
		actor,
		ActionSurfaceDeprecated,
		ResourceKindSurface,
		surfaceID,
		intPtr(version),
		"surface deprecated: "+surfaceID+" v"+itoa(version),
		meta,
	)
}

// itoa converts an int to its decimal string representation without importing
// strconv at the package level. This is used only for summary message construction.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
