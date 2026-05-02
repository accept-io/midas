package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/accept-io/midas/internal/businessservice"
)

// BusinessServiceRelationshipRepo is an in-memory implementation of
// businessservice.RelationshipRepository.
//
// Constraints enforced in code (mirroring the Postgres CHECK / UNIQUE /
// FK constraints):
//
//   - source != target  (chk_bsr_no_self_reference)
//   - relationship_type ∈ {depends_on, supports, part_of}  (chk_bsr_relationship_type)
//   - (source, target, type) triple is unique  (uniq_bsr_triple)
//   - id is unique  (PK)
//   - source/target referenced business services exist (when validators are populated)
//
// Concurrency: a sync.RWMutex guards all reads and writes. Reads return
// defensive copies so callers cannot mutate stored values.
type BusinessServiceRelationshipRepo struct {
	mu    sync.RWMutex
	items map[string]*businessservice.BusinessServiceRelationship

	// Optional FK validators populated by NewRepositories. When set, Create
	// rejects relationships referencing non-existent business services.
	businessSvcs businessservice.BusinessServiceRepository
}

// NewBusinessServiceRelationshipRepo constructs an empty repo.
func NewBusinessServiceRelationshipRepo() *BusinessServiceRelationshipRepo {
	return &BusinessServiceRelationshipRepo{
		items: map[string]*businessservice.BusinessServiceRelationship{},
	}
}

func (r *BusinessServiceRelationshipRepo) Create(ctx context.Context, rel *businessservice.BusinessServiceRelationship) error {
	if rel == nil {
		return businessservice.ErrRelationshipNotFound // defensive — should never happen on a Create path
	}
	if rel.SourceBusinessService == rel.TargetBusinessService {
		return businessservice.ErrRelationshipSelfReference
	}
	if !businessservice.IsValidRelationshipType(rel.RelationshipType) {
		return businessservice.ErrRelationshipInvalidType
	}

	if r.businessSvcs != nil {
		ok, err := r.businessSvcs.Exists(ctx, rel.SourceBusinessService)
		if err != nil {
			return err
		}
		if !ok {
			return wrapMissingBSRef("source", rel.SourceBusinessService)
		}
		ok, err = r.businessSvcs.Exists(ctx, rel.TargetBusinessService)
		if err != nil {
			return err
		}
		if !ok {
			return wrapMissingBSRef("target", rel.TargetBusinessService)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[rel.ID]; exists {
		return businessservice.ErrRelationshipDuplicateID
	}
	for _, existing := range r.items {
		if existing.SourceBusinessService == rel.SourceBusinessService &&
			existing.TargetBusinessService == rel.TargetBusinessService &&
			existing.RelationshipType == rel.RelationshipType {
			return businessservice.ErrRelationshipDuplicateTriple
		}
	}

	r.items[rel.ID] = cloneRelationship(rel)
	return nil
}

func (r *BusinessServiceRelationshipRepo) GetByID(_ context.Context, id string) (*businessservice.BusinessServiceRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rel, ok := r.items[id]
	if !ok {
		return nil, businessservice.ErrRelationshipNotFound
	}
	return cloneRelationship(rel), nil
}

func (r *BusinessServiceRelationshipRepo) List(_ context.Context) ([]*businessservice.BusinessServiceRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*businessservice.BusinessServiceRelationship, 0, len(r.items))
	for _, rel := range r.items {
		out = append(out, cloneRelationship(rel))
	}
	sortRelationships(out)
	return out, nil
}

func (r *BusinessServiceRelationshipRepo) ListBySourceBusinessService(_ context.Context, sourceID string) ([]*businessservice.BusinessServiceRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*businessservice.BusinessServiceRelationship, 0)
	for _, rel := range r.items {
		if rel.SourceBusinessService == sourceID {
			out = append(out, cloneRelationship(rel))
		}
	}
	sortRelationships(out)
	return out, nil
}

func (r *BusinessServiceRelationshipRepo) ListByTargetBusinessService(_ context.Context, targetID string) ([]*businessservice.BusinessServiceRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*businessservice.BusinessServiceRelationship, 0)
	for _, rel := range r.items {
		if rel.TargetBusinessService == targetID {
			out = append(out, cloneRelationship(rel))
		}
	}
	sortRelationships(out)
	return out, nil
}

func (r *BusinessServiceRelationshipRepo) Update(_ context.Context, rel *businessservice.BusinessServiceRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.items[rel.ID]
	if !ok {
		return businessservice.ErrRelationshipNotFound
	}
	// Only Description is mutable. Other fields are write-once.
	existing.Description = rel.Description
	return nil
}

func (r *BusinessServiceRelationshipRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return businessservice.ErrRelationshipNotFound
	}
	delete(r.items, id)
	return nil
}

// cloneRelationship returns a defensive copy. Time and string fields are
// value types so a struct copy suffices.
func cloneRelationship(in *businessservice.BusinessServiceRelationship) *businessservice.BusinessServiceRelationship {
	cp := *in
	return &cp
}

// sortRelationships orders by CreatedAt DESC then ID ASC, matching the
// Postgres repo's ordering contract.
func sortRelationships(rels []*businessservice.BusinessServiceRelationship) {
	sort.Slice(rels, func(i, j int) bool {
		if !rels[i].CreatedAt.Equal(rels[j].CreatedAt) {
			return rels[i].CreatedAt.After(rels[j].CreatedAt)
		}
		return rels[i].ID < rels[j].ID
	})
}

// wrapMissingBSRef returns an error matching the Postgres FK-violation message
// shape. Both the memory and Postgres repos return errors of comparable
// readability when a referenced business service does not exist.
func wrapMissingBSRef(side, id string) error {
	return &missingBSRefErr{side: side, id: id}
}

type missingBSRefErr struct {
	side string
	id   string
}

func (e *missingBSRefErr) Error() string {
	return "create business service relationship: referenced " + e.side + " business service " + e.id + " not found"
}

var _ businessservice.RelationshipRepository = (*BusinessServiceRelationshipRepo)(nil)
