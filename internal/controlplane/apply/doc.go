// Package apply applies validated MIDAS control-plane documents.
//
// The apply flow proceeds in two distinct phases:
//
//  1. Planning — validate the bundle, inspect persisted state, and resolve
//     cross-document referential integrity to construct an ApplyPlan describing
//     the intended action (create, unchanged, conflict, or invalid) for each
//     document. No persistence occurs during planning.
//
//  2. Execution — walk the ApplyPlan in dependency order and persist each
//     resource according to its planned action. If any entry is invalid, the
//     entire bundle is rejected without persisting anything.
//
// # Guarantees
//
//   - Validation happens before any resources are applied.
//   - Invalid bundles produce validation errors only; no resources are persisted.
//   - Conflict entries are not persisted; they surface in the ApplyResult.
//   - Valid, non-conflicting bundles produce created results.
//   - Mixed-validity bundles are rejected as a whole.
//   - Cross-document references are verified before execution begins.
//   - Execution respects dependency order: Surface → Agent → Profile → Grant.
//
// # Resource planning semantics
//
// Surface: apply always creates a new governed version entering the governance
// pipeline at review status. Unchanged detection is not supported: the versioning
// model intends a new version record on every valid submission. A conflict is
// detected when the latest persisted version is already in review status, because
// applying again before the pending review is resolved would create an ambiguous
// governance state.
//
// Agent: agents are identified by ID and are immutable once created in the apply
// path. A conflict is detected when an agent with the same ID already exists.
// Unchanged is not supported: the document schema does not carry enough field
// parity with the domain model to prove equality without a full field comparison.
//
// Profile: profiles are identified by ID and are immutable once created in the
// apply path. A conflict is detected when a profile with the same ID already
// exists. Unchanged is not supported for the same reason as Agent. The profile's
// spec.surface_id must resolve to a surface that either exists in persisted state
// or is being created in the same bundle.
//
// Grant: grants are identified by ID and are immutable once created in the apply
// path. A conflict is detected when a grant with the same ID already exists.
// Unchanged is not supported for the same reason as Agent. The grant's
// spec.agent_id and spec.profile_id must each resolve to resources that either
// exist in persisted state or are being created in the same bundle.
//
// # Bundle-level referential integrity
//
// After resource-local planning, the planner verifies that all cross-document
// references are satisfiable. A reference is satisfied when the referenced ID
// exists in persisted state (confirmed via repository) or when a non-invalid,
// non-conflict create entry for that (kind, id) pair is present in the same bundle.
// Entries whose references cannot be resolved are marked invalid, causing the
// entire bundle to be rejected.
//
// # Typed errors
//
// Domain conditions are signalled via typed sentinel errors (ErrInvalidBundle,
// ErrValidationFailed, ErrDuplicateResource, ErrResourceConflict,
// ErrReferentialIntegrity, etc.). Callers should use errors.Is to test for
// specific conditions. The original cause is always preserved in the error
// chain via fmt.Errorf wrapping.
package apply
