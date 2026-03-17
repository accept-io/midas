// Package controlplane defines the declarative governance control plane for MIDAS.
//
// The control plane is responsible for describing and managing the governed
// resources that shape agent execution authority, including:
//
//   - Surfaces: governed action boundaries
//   - Agents: non-human or system identities
//   - Profiles: authority policies and execution constraints
//   - Grants: bindings that assign profiles to agents
//
// In this model, control-plane resources are authored as documents, parsed from
// YAML, validated against MIDAS rules, and then applied into the system.
//
// # Milestone 4.5 scope
//
// At this stage, the control plane provides:
//
//   - typed document models
//   - YAML parsing for single and multi-document bundles
//   - structural and semantic validation
//   - apply orchestration for validated bundles
//
// Persistence-backed apply is intentionally deferred. Current apply behavior is
// suitable for foundation work and contract validation, while repository-backed
// persistence will be introduced in a later milestone.
//
// # Package structure
//
// Subpackages are organised by responsibility:
//
//   - types: core document and result types
//   - parser: YAML parsing into typed control-plane documents
//   - validate: document and bundle validation
//   - apply: application workflow for validated bundles
//
// The control plane is intended to remain explicit, auditable, and deterministic
// so that governance state can be reviewed and reasoned about independently of
// runtime decision execution.
package controlplane
