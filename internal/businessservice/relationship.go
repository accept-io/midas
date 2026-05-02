// Package businessservice — relationship.go defines the lightweight
// BusinessServiceRelationship junction.
//
// A BusinessServiceRelationship links two BusinessServices via a
// relationship_type ∈ {depends_on, supports, part_of}. It is structural
// metadata, not an authority artefact. The junction has no lifecycle of
// its own (no Status, EffectiveFrom, ApprovedBy) — it follows the
// immediate-apply junction posture of business_service_capabilities.
//
// Cycle-prevention scope (Epic 1, PR 1): only direct self-reference is
// rejected. Recursive cycle detection across the BSR graph is deferred
// to a follow-up PR.

package businessservice

import (
	"context"
	"errors"
	"time"
)

// Relationship type constants. Mirror the schema CHECK list in
// internal/store/postgres/schema.sql.
const (
	RelationshipTypeDependsOn = "depends_on"
	RelationshipTypeSupports  = "supports"
	RelationshipTypePartOf    = "part_of"
)

// ValidRelationshipTypes returns the canonical set of relationship-type
// values, in stable order. Mirrors the schema CHECK.
func ValidRelationshipTypes() []string {
	return []string{
		RelationshipTypeDependsOn,
		RelationshipTypeSupports,
		RelationshipTypePartOf,
	}
}

// IsValidRelationshipType reports whether t is one of the canonical values.
func IsValidRelationshipType(t string) bool {
	switch t {
	case RelationshipTypeDependsOn, RelationshipTypeSupports, RelationshipTypePartOf:
		return true
	}
	return false
}

// BusinessServiceRelationship is a directed link between two BusinessServices.
//
// SourceBusinessService and TargetBusinessService both reference
// business_services.business_service_id. Description is the only mutable
// field (per the apply path's plan/update behaviour); all other fields
// are write-once at Create time.
type BusinessServiceRelationship struct {
	ID                    string
	SourceBusinessService string
	TargetBusinessService string
	RelationshipType      string
	Description           string

	CreatedAt time.Time
	CreatedBy string
}

// Sentinel errors. Repository implementations return these where the
// repository contract calls for a "not found" or "duplicate" signal.
// Wrap with fmt.Errorf("...: %w", err) when adding context.
var (
	ErrRelationshipNotFound        = errors.New("business service relationship not found")
	ErrRelationshipDuplicateID     = errors.New("business service relationship id already exists")
	ErrRelationshipDuplicateTriple = errors.New("business service relationship (source, target, type) triple already exists")
	ErrRelationshipSelfReference   = errors.New("business service relationship source and target must differ")
	ErrRelationshipInvalidType     = errors.New("business service relationship type is not one of {depends_on, supports, part_of}")
)

// RelationshipRepository defines persistence operations for the
// business_service_relationships junction.
//
// Ordering contract: List, ListBySourceBusinessService, and
// ListByTargetBusinessService return rows ordered by CreatedAt DESC, then
// by ID ASC. The DESC-by-time ordering matches the read-side intuition
// "most recent first"; the ID tiebreaker keeps determinism when multiple
// rows share a created_at.
type RelationshipRepository interface {
	Create(ctx context.Context, rel *BusinessServiceRelationship) error
	GetByID(ctx context.Context, id string) (*BusinessServiceRelationship, error)
	List(ctx context.Context) ([]*BusinessServiceRelationship, error)
	ListBySourceBusinessService(ctx context.Context, sourceID string) ([]*BusinessServiceRelationship, error)
	ListByTargetBusinessService(ctx context.Context, targetID string) ([]*BusinessServiceRelationship, error)
	Update(ctx context.Context, rel *BusinessServiceRelationship) error
	Delete(ctx context.Context, id string) error
}
