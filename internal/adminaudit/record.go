// Package adminaudit defines the append-only platform-administrative audit
// trail. It is deliberately separate from the runtime decision audit
// (internal/audit) which is a hash-chained per-envelope trail, and from the
// resource-centric control-plane audit (internal/controlaudit) which captures
// per-resource lifecycle events.
//
// adminaudit records answer principal-keyed investigation questions: who
// performed a platform-administrative action, from where, under what
// authority, when, and with what outcome. First-pass coverage is narrow by
// design — see docs for the canonical action list. The record shape is
// first-class and structured; it is not a generic event blob and does not
// carry secret material (passwords, hashes, tokens).
//
// Records are persisted via the Repository interface (repository.go) whose
// surface is explicitly append-only: there is no Update or Delete method and
// implementations must not support mutation.
package adminaudit

import (
	"time"

	"github.com/google/uuid"
)

// Action is a machine-stable identifier for an administrative action. The
// enum is deliberately small and explicit; new actions must be added here
// rather than encoded as free text.
type Action string

const (
	// ActionApplyInvoked records one entry per HTTP apply invocation. It is
	// request-level; the per-resource rows written by controlaudit remain
	// untouched.
	ActionApplyInvoked Action = "apply.invoked"

	// ActionPasswordChanged records a successful local-IAM password change.
	// Never carries password material.
	ActionPasswordChanged Action = "password.changed"

	// ActionBootstrapAdminCreated records first-run creation of the initial
	// platform admin account by the bootstrap path.
	ActionBootstrapAdminCreated Action = "bootstrap.admin_created"
)

// Outcome classifies whether the action completed successfully or failed.
// First-pass emission writes only success/failure outcomes from the service
// layer. Denial outcomes (permission/authn denials) are intentionally out of
// scope for this PR.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
)

// ActorType disambiguates the class of principal that performed the action.
// system is used for bootstrap and other unattended paths where there is no
// human principal.
type ActorType string

const (
	ActorTypeUser   ActorType = "user"
	ActorTypeSystem ActorType = "system"
)

// TargetType is a small, open-coded enumeration of admin-audit target kinds.
// Values are set by the emission site and are not constrained at the
// database level.
const (
	TargetTypeBundle   = "bundle"
	TargetTypeUser     = "user"
	TargetTypePlatform = "platform"
	TargetTypeProcess  = "process"
)

// Details carries a small, action-specific structured context attached to a
// record. Every field is optional. This is deliberately narrow — it is not a
// free-form JSON payload. Fields are added only when a specific action has
// investigation-valuable context that cannot be represented in the top-level
// columns.
//
// Secret material MUST NOT be set on any field.
type Details struct {
	// Apply-invoked context.
	BundleBytes    int `json:"bundle_bytes,omitempty"`
	CreatedCount   int `json:"created_count,omitempty"`
	ConflictCount  int `json:"conflict_count,omitempty"`
	UnchangedCount int `json:"unchanged_count,omitempty"`
	ErrorCount     int `json:"error_count,omitempty"`

	// Promote context.
	FromCapabilityID string `json:"from_capability_id,omitempty"`
	FromProcessID    string `json:"from_process_id,omitempty"`
	ToCapabilityID   string `json:"to_capability_id,omitempty"`
	ToProcessID      string `json:"to_process_id,omitempty"`
	SurfacesMigrated int    `json:"surfaces_migrated,omitempty"`

	// Cleanup context.
	OlderThanDays       int      `json:"older_than_days,omitempty"`
	ProcessesDeleted    []string `json:"processes_deleted,omitempty"`
	CapabilitiesDeleted []string `json:"capabilities_deleted,omitempty"`

	// Error / failure message — human-readable context, never secret.
	Error string `json:"error,omitempty"`
}

// AdminAuditRecord is the persisted administrative audit event. It is
// immutable once appended; repository implementations must not expose Update
// or Delete.
//
// Fields:
//
//   - ID:                 UUID assigned at construction
//   - OccurredAt:         UTC timestamp
//   - Action:             typed enum (see Action constants)
//   - Outcome:            typed enum (see Outcome constants)
//   - ActorType:          user | system (see ActorType constants)
//   - ActorID:            principal ID or system identifier (e.g.
//                         "system:bootstrap"); empty when unknown
//   - TargetType:         bundle | user | platform | process (string; open set)
//   - TargetID:           resource ID where applicable; empty otherwise
//   - RequestID:          HTTP request correlation ID; empty for non-HTTP events
//   - ClientIP:           request source IP; empty for non-HTTP events
//   - RequiredPermission: the authz permission the request was gated on;
//                         empty for non-permission-gated events
//   - Details:            small, action-specific structured extras
type AdminAuditRecord struct {
	ID                 string    `json:"id"`
	OccurredAt         time.Time `json:"occurred_at"`
	Action             Action    `json:"action"`
	Outcome            Outcome   `json:"outcome"`
	ActorType          ActorType `json:"actor_type"`
	ActorID            string    `json:"actor_id,omitempty"`
	TargetType         string    `json:"target_type,omitempty"`
	TargetID           string    `json:"target_id,omitempty"`
	RequestID          string    `json:"request_id,omitempty"`
	ClientIP           string    `json:"client_ip,omitempty"`
	RequiredPermission string    `json:"required_permission,omitempty"`
	Details            *Details  `json:"details,omitempty"`
}

// NewRecord constructs an AdminAuditRecord with a fresh UUID and current UTC
// timestamp. Callers set the remaining fields directly via the returned
// struct — keeping the constructor small avoids inventing a large option-bag
// API for what is a flat record.
func NewRecord(action Action, outcome Outcome, actorType ActorType) *AdminAuditRecord {
	return &AdminAuditRecord{
		ID:         uuid.NewString(),
		OccurredAt: time.Now().UTC(),
		Action:     action,
		Outcome:    outcome,
		ActorType:  actorType,
	}
}
