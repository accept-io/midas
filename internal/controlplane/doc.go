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
// # Package structure
//
// Subpackages are organised by responsibility:
//
//   - types: core document and result types
//   - parser: YAML parsing into typed control-plane documents
//   - validate: document and bundle validation
//   - apply: planning and execution of validated bundles
package controlplane
