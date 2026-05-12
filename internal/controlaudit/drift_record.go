package controlaudit

// DriftDefinition control-audit constants and record constructors
// (Drift-1e). The five lifecycle actions surfaced via HTTP in this
// tranche each emit a distinct action kind so operators can replay
// the lifecycle of a single revision from the audit trail. Mirrors
// failmode_record.go in shape.

const (
	// ActionDriftDefinitionSubmitted is emitted on a successful
	// draft → review transition through the submit HTTP endpoint.
	ActionDriftDefinitionSubmitted Action = "drift_definition.submitted"

	// ActionDriftDefinitionApproved is emitted on a successful
	// review → active transition through the approve HTTP endpoint.
	ActionDriftDefinitionApproved Action = "drift_definition.approved"

	// ActionDriftDefinitionRejected is emitted on a successful
	// review → draft transition through the reject HTTP endpoint.
	// The operator-supplied reason (when present) lands on
	// Metadata.DeprecationReason — drift.DriftDefinition has no
	// dedicated rejection_reason column, so the audit record is the
	// sole store of operator intent. Naming reuse is deliberate; the
	// metadata field is the project's general-purpose lifecycle
	// reason channel.
	ActionDriftDefinitionRejected Action = "drift_definition.rejected"

	// ActionDriftDefinitionDeprecated is emitted on a successful
	// active → deprecated transition through the deprecate HTTP
	// endpoint. Reason and successor fields land on the metadata.
	ActionDriftDefinitionDeprecated Action = "drift_definition.deprecated"

	// ActionDriftDefinitionRetired is emitted on a successful
	// transition to retired (allowed from draft, review, active, or
	// deprecated; retired is terminal).
	ActionDriftDefinitionRetired Action = "drift_definition.retired"
)

// ResourceKindDriftDefinition mirrors the Kind constant in
// internal/controlplane/types (KindDriftDefinition = "DriftDefinition")
// in the control-audit's snake-case convention.
const ResourceKindDriftDefinition = "drift_definition"

// NewDriftDefinitionSubmittedRecord builds a record for a draft →
// review transition.
func NewDriftDefinitionSubmittedRecord(actor, definitionID string, version int, reason string) *ControlAuditRecord {
	var meta *Metadata
	if reason != "" {
		meta = &Metadata{DeprecationReason: reason}
	}
	return newRecord(
		actor,
		ActionDriftDefinitionSubmitted,
		ResourceKindDriftDefinition,
		definitionID,
		intPtr(version),
		"drift definition submitted for review: "+definitionID+" v"+itoa(version),
		meta,
	)
}

// NewDriftDefinitionApprovedRecord builds a record for a review →
// active transition.
func NewDriftDefinitionApprovedRecord(actor, definitionID string, version int) *ControlAuditRecord {
	return newRecord(
		actor,
		ActionDriftDefinitionApproved,
		ResourceKindDriftDefinition,
		definitionID,
		intPtr(version),
		"drift definition approved: "+definitionID+" v"+itoa(version),
		nil,
	)
}

// NewDriftDefinitionRejectedRecord builds a record for a review →
// draft transition. The operator-supplied reason (when present) is
// captured on Metadata.DeprecationReason — drift.DriftDefinition has
// no rejection_reason column.
func NewDriftDefinitionRejectedRecord(actor, definitionID string, version int, reason string) *ControlAuditRecord {
	var meta *Metadata
	if reason != "" {
		meta = &Metadata{DeprecationReason: reason}
	}
	return newRecord(
		actor,
		ActionDriftDefinitionRejected,
		ResourceKindDriftDefinition,
		definitionID,
		intPtr(version),
		"drift definition rejected: "+definitionID+" v"+itoa(version),
		meta,
	)
}

// NewDriftDefinitionDeprecatedRecord builds a record for an active →
// deprecated transition. The operator-supplied reason is captured on
// Metadata.DeprecationReason. Successor information is persisted on
// the drift definition row itself (SuccessorDefinitionID /
// SuccessorVersion); the audit metadata records only the operator
// intent (reason). This mirrors NewFailModePolicyDeprecatedRecord.
func NewDriftDefinitionDeprecatedRecord(actor, definitionID string, version int, reason string) *ControlAuditRecord {
	var meta *Metadata
	if reason != "" {
		meta = &Metadata{DeprecationReason: reason}
	}
	return newRecord(
		actor,
		ActionDriftDefinitionDeprecated,
		ResourceKindDriftDefinition,
		definitionID,
		intPtr(version),
		"drift definition deprecated: "+definitionID+" v"+itoa(version),
		meta,
	)
}

// NewDriftDefinitionRetiredRecord builds a record for a transition to
// retired (allowed from draft, review, active, or deprecated). The
// operator-supplied reason (when present) is captured on
// Metadata.DeprecationReason.
func NewDriftDefinitionRetiredRecord(actor, definitionID string, version int, reason string) *ControlAuditRecord {
	var meta *Metadata
	if reason != "" {
		meta = &Metadata{DeprecationReason: reason}
	}
	return newRecord(
		actor,
		ActionDriftDefinitionRetired,
		ResourceKindDriftDefinition,
		definitionID,
		intPtr(version),
		"drift definition retired: "+definitionID+" v"+itoa(version),
		meta,
	)
}
