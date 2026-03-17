// Package apply applies validated MIDAS control-plane documents.
//
// The apply flow is intentionally simple:
//
//  1. accept a parsed bundle of control-plane documents
//  2. validate the bundle as a whole
//  3. if validation fails, return validation errors and apply nothing
//  4. if validation succeeds, record successful application results
//
// # Milestone 4.5 behavior
//
// In milestone 4.5, apply does not yet persist resources to a backing store.
// It provides the orchestration contract and result model that later milestones
// will extend with repository-backed creation, conflict detection, idempotency,
// and transactional semantics.
//
// Current guarantees
//
//   - validation happens before apply results are recorded
//   - invalid bundles produce validation errors only
//   - valid bundles produce created results
//   - mixed-validity bundles are rejected as a whole
//
// Future milestones may extend this package with:
//
//   - repository-backed persistence
//   - idempotent apply semantics
//   - conflict reporting against stored resources
//   - transactional bundle application
package apply
