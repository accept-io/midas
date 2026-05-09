package controlaudit

// FailModePolicy control-audit constants and record constructors
// (D27j-impl-1c). Approval and deprecation are the only lifecycle actions
// surfaced via HTTP in this tranche; create/versioned events are deferred
// until the apply pipeline (D27j-impl-1b) gains audit emission alongside
// the surface/profile/governanceexpectation pattern.

const (
	// ActionFailModePolicyApproved is emitted on a successful review →
	// active transition through the approval HTTP endpoint.
	ActionFailModePolicyApproved Action = "fail_mode_policy.approved"

	// ActionFailModePolicyDeprecated is emitted on a successful active →
	// deprecated transition through the deprecation HTTP endpoint.
	// The operator-supplied reason (when present) lands on
	// Metadata.DeprecationReason rather than on the persisted policy row,
	// because failmode.FailModePolicy carries no DeprecationReason field.
	ActionFailModePolicyDeprecated Action = "fail_mode_policy.deprecated"
)

// ResourceKindFailModePolicy mirrors the kind constant in
// internal/controlplane/types (KindFailModePolicy = "FailModePolicy") in
// the control-audit's snake-case convention.
const ResourceKindFailModePolicy = "fail_mode_policy"

// NewFailModePolicyApprovedRecord builds a record for a fail-mode-policy
// approval (review → active). Mirrors NewProfileApprovedRecord in shape.
func NewFailModePolicyApprovedRecord(actor, policyID string, version int) *ControlAuditRecord {
	return newRecord(
		actor,
		ActionFailModePolicyApproved,
		ResourceKindFailModePolicy,
		policyID,
		intPtr(version),
		"fail-mode policy approved: "+policyID+" v"+itoa(version),
		nil,
	)
}

// NewFailModePolicyDeprecatedRecord builds a record for a fail-mode-policy
// deprecation (active → deprecated). When reason is non-empty it is
// captured on Metadata.DeprecationReason — failmode.FailModePolicy has no
// dedicated deprecation_reason column, so the audit record is the sole
// store of operator intent. Mirrors NewProfileDeprecatedRecord plus the
// metadata-on-grant-revoked pattern.
func NewFailModePolicyDeprecatedRecord(actor, policyID string, version int, reason string) *ControlAuditRecord {
	var meta *Metadata
	if reason != "" {
		meta = &Metadata{DeprecationReason: reason}
	}
	return newRecord(
		actor,
		ActionFailModePolicyDeprecated,
		ResourceKindFailModePolicy,
		policyID,
		intPtr(version),
		"fail-mode policy deprecated: "+policyID+" v"+itoa(version),
		meta,
	)
}
