// Package aisystem defines the AI System Registration substrate
// (Epic 1, PR 2).
//
// The substrate is split across three domain types:
//
//   - AISystem            — the governance subject (status-honouring,
//     non-versioned). Mirrors the BusinessService apply posture: apply
//     accepts whatever status the bundle declares, no review-forcing.
//
//   - AISystemVersion     — a versioned snapshot of the AI system at a
//     point in time (model artifact / hash / endpoint / compliance
//     frameworks). Status-honouring like AISystem; the (id, version)
//     composite is immutable once created. There is no review-forced
//     activation; apply preserves the bundle-declared status verbatim
//     (deliberate divergence from Surface/Profile, which are
//     authority/control artefacts).
//
//   - AISystemBinding     — the immediate-apply junction linking an
//     AISystem (optionally pinned to a specific AISystemVersion) to one
//     or more existing MIDAS context entities (BusinessService,
//     Capability, Process, DecisionSurface). Mirrors the BSR/BSC junction
//     posture: no Status, no EffectiveFrom, no review.
//
// AISystem is conceptually distinct from agent.Agent. Agent is the
// runtime actor that calls /v1/evaluate; AISystem is the governance
// subject (the model behind the actor). An Agent of type "ai" may be
// associated with an AISystem at runtime, but the two entities are
// separate by design.
package aisystem

import (
	"context"
	"time"
)

// Allowed AISystem status values. AISystem status is status-honouring:
// apply persists whatever the bundle declares, with no review workflow.
const (
	AISystemStatusActive     = "active"
	AISystemStatusDeprecated = "deprecated"
	AISystemStatusRetired    = "retired"
)

// Allowed AISystem origin values.
const (
	AISystemOriginManual   = "manual"
	AISystemOriginInferred = "inferred"
)

// IsValidAISystemStatus reports whether s is one of the canonical
// AISystem status values. Mirrors the schema CHECK on ai_systems.status.
func IsValidAISystemStatus(s string) bool {
	switch s {
	case AISystemStatusActive, AISystemStatusDeprecated, AISystemStatusRetired:
		return true
	}
	return false
}

// IsValidAISystemOrigin reports whether o is one of the canonical
// origin values. Mirrors the schema CHECK on ai_systems.origin.
func IsValidAISystemOrigin(o string) bool {
	switch o {
	case AISystemOriginManual, AISystemOriginInferred:
		return true
	}
	return false
}

// AISystem is the governance subject. Identity-only registration —
// risk classification, regulatory scope, and external references are
// excluded by design (deferred to the future risk-classification epic
// and the cross-cutting ExternalRef PR respectively).
type AISystem struct {
	ID          string
	Name        string
	Description string
	Owner       string
	Vendor      string
	SystemType  string
	Status      string
	Origin      string
	Managed     bool

	// Replaces is the optional logical ID of an AI system this one
	// supersedes. Self-reference is rejected (chk_ai_systems_no_self_replace).
	Replaces string

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
}

// SystemRepository is the persistence interface for AISystem.
//
// Ordering contract: List returns rows ordered by CreatedAt DESC then
// ID ASC. The DESC-by-time ordering matches the read-side intuition
// "most recent first"; the ID tiebreaker keeps determinism when multiple
// rows share a created_at.
type SystemRepository interface {
	Create(ctx context.Context, sys *AISystem) error
	GetByID(ctx context.Context, id string) (*AISystem, error)
	Exists(ctx context.Context, id string) (bool, error)
	List(ctx context.Context) ([]*AISystem, error)
	Update(ctx context.Context, sys *AISystem) error
}
