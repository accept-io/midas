package apply

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// stubBSRRepo is the minimal BSR repository surface needed for planner tests.
// Implements the BusinessServiceRelationshipRepository contract on a Go map.
type stubBSRRepo struct {
	items map[string]*businessservice.BusinessServiceRelationship
}

func newStubBSRRepo() *stubBSRRepo {
	return &stubBSRRepo{items: map[string]*businessservice.BusinessServiceRelationship{}}
}

func (r *stubBSRRepo) GetByID(_ context.Context, id string) (*businessservice.BusinessServiceRelationship, error) {
	rel, ok := r.items[id]
	if !ok {
		return nil, businessservice.ErrRelationshipNotFound
	}
	return rel, nil
}

func (r *stubBSRRepo) ListBySourceBusinessService(_ context.Context, sourceID string) ([]*businessservice.BusinessServiceRelationship, error) {
	out := make([]*businessservice.BusinessServiceRelationship, 0)
	for _, rel := range r.items {
		if rel.SourceBusinessService == sourceID {
			out = append(out, rel)
		}
	}
	return out, nil
}

func (r *stubBSRRepo) Create(_ context.Context, rel *businessservice.BusinessServiceRelationship) error {
	r.items[rel.ID] = rel
	return nil
}

func (r *stubBSRRepo) Update(_ context.Context, rel *businessservice.BusinessServiceRelationship) error {
	if _, ok := r.items[rel.ID]; !ok {
		return businessservice.ErrRelationshipNotFound
	}
	r.items[rel.ID].Description = rel.Description
	return nil
}

// stubBSExistsRepo provides a one-method BS Exists check needed by the planner
// helper checkBusinessServiceExists.
type stubBSExistsRepo struct {
	exists map[string]bool
}

func (r *stubBSExistsRepo) Exists(_ context.Context, id string) (bool, error) {
	return r.exists[id], nil
}

func (r *stubBSExistsRepo) GetByID(_ context.Context, id string) (*businessservice.BusinessService, error) {
	return nil, nil // unused on the planner path under test
}

func (r *stubBSExistsRepo) Create(_ context.Context, _ *businessservice.BusinessService) error {
	return nil // unused
}

func makeBSRDoc(id, source, target, relType, description string) parser.ParsedDocument {
	doc := types.BusinessServiceRelationshipDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindBusinessServiceRelationship,
		Metadata:   types.DocumentMetadata{ID: id},
		Spec: types.BusinessServiceRelationshipSpec{
			SourceBusinessServiceID: source,
			TargetBusinessServiceID: target,
			RelationshipType:        relType,
			Description:             description,
		},
	}
	return parser.ParsedDocument{
		Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc,
	}
}

// ──────────────────────────────────────────────────────────────────────────
// Mapper
// ──────────────────────────────────────────────────────────────────────────

func TestMapper_BusinessServiceRelationship_HappyPath(t *testing.T) {
	doc := types.BusinessServiceRelationshipDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindBusinessServiceRelationship,
		Metadata:   types.DocumentMetadata{ID: "rel-map-1"},
		Spec: types.BusinessServiceRelationshipSpec{
			SourceBusinessServiceID: "bs-a",
			TargetBusinessServiceID: "bs-b",
			RelationshipType:        "depends_on",
			Description:             "test",
		},
	}
	now := time.Now()
	rel, err := mapBusinessServiceRelationshipDocumentToBusinessServiceRelationship(doc, now, "operator:test")
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if rel.ID != "rel-map-1" || rel.SourceBusinessService != "bs-a" || rel.TargetBusinessService != "bs-b" {
		t.Errorf("mapper field mismatch: %+v", rel)
	}
	if rel.RelationshipType != "depends_on" || rel.Description != "test" {
		t.Errorf("mapper spec mismatch: %+v", rel)
	}
	if rel.CreatedBy != "operator:test" {
		t.Errorf("CreatedBy: got %q", rel.CreatedBy)
	}
	if !rel.CreatedAt.Equal(now.UTC()) {
		t.Errorf("CreatedAt should be UTC of now: got %v", rel.CreatedAt)
	}
}

func TestMapper_BusinessServiceRelationship_TrimsWhitespace(t *testing.T) {
	doc := types.BusinessServiceRelationshipDocument{
		Metadata: types.DocumentMetadata{ID: "  rel-trim  "},
		Spec: types.BusinessServiceRelationshipSpec{
			SourceBusinessServiceID: "  bs-a  ",
			TargetBusinessServiceID: " bs-b\n",
			RelationshipType:        " depends_on ",
			Description:             "  desc  ",
		},
	}
	rel, err := mapBusinessServiceRelationshipDocumentToBusinessServiceRelationship(doc, time.Now(), "  user  ")
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if rel.ID != "rel-trim" || rel.SourceBusinessService != "bs-a" || rel.TargetBusinessService != "bs-b" {
		t.Errorf("trim mismatch: %+v", rel)
	}
	if rel.RelationshipType != "depends_on" || rel.Description != "desc" || rel.CreatedBy != "user" {
		t.Errorf("trim mismatch: %+v", rel)
	}
}

// ──────────────────────────────────────────────────────────────────────────
// Planner
// ──────────────────────────────────────────────────────────────────────────

// newPlannerTestService wires a minimal Service with the BS-exists stub and
// the BSR stub. Other repos remain nil; the planner branch under test only
// touches these two surfaces.
func newPlannerTestService(bsExists []string, bsr *stubBSRRepo) *Service {
	exists := make(map[string]bool, len(bsExists))
	for _, id := range bsExists {
		exists[id] = true
	}
	return &Service{
		businessServiceRepo: &stubBSExistsRepo{exists: exists},
		bsRelationshipRepo:  bsr,
	}
}

func TestApply_BusinessServiceRelationship_PlanCreate_WhenNotPersisted(t *testing.T) {
	svc := newPlannerTestService([]string{"bs-a", "bs-b"}, newStubBSRRepo())
	entry := ApplyPlanEntry{Kind: types.KindBusinessServiceRelationship, ID: "rel-new", DocumentIndex: 1}
	doc := makeBSRDoc("rel-new", "bs-a", "bs-b", "depends_on", "")

	svc.planBusinessServiceRelationshipEntry(context.Background(), doc, map[string]struct{}{}, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create, got %s (msg=%s, errs=%+v)", entry.Action, entry.Message, entry.ValidationErrors)
	}
	if entry.CreateKind != CreateKindNew {
		t.Errorf("CreateKind: want %s, got %s", CreateKindNew, entry.CreateKind)
	}
}

func TestApply_BusinessServiceRelationship_PlanConflict_WhenIDExists(t *testing.T) {
	bsr := newStubBSRRepo()
	bsr.items["rel-existing"] = &businessservice.BusinessServiceRelationship{
		ID: "rel-existing", SourceBusinessService: "bs-a", TargetBusinessService: "bs-b", RelationshipType: "depends_on",
	}
	svc := newPlannerTestService([]string{"bs-a", "bs-b"}, bsr)
	entry := ApplyPlanEntry{Kind: types.KindBusinessServiceRelationship, ID: "rel-existing", DocumentIndex: 1}
	doc := makeBSRDoc("rel-existing", "bs-a", "bs-b", "depends_on", "")

	svc.planBusinessServiceRelationshipEntry(context.Background(), doc, map[string]struct{}{}, &entry)
	if entry.Action != ApplyActionConflict {
		t.Errorf("Action: want Conflict, got %s", entry.Action)
	}
	if !strings.Contains(entry.Message, "already exists") {
		t.Errorf("expected already-exists in message; got %q", entry.Message)
	}
}

func TestApply_BusinessServiceRelationship_PlanConflict_WhenTripleExists(t *testing.T) {
	bsr := newStubBSRRepo()
	bsr.items["rel-pre"] = &businessservice.BusinessServiceRelationship{
		ID: "rel-pre", SourceBusinessService: "bs-a", TargetBusinessService: "bs-b", RelationshipType: "depends_on",
	}
	svc := newPlannerTestService([]string{"bs-a", "bs-b"}, bsr)
	entry := ApplyPlanEntry{Kind: types.KindBusinessServiceRelationship, ID: "rel-different-id", DocumentIndex: 1}
	doc := makeBSRDoc("rel-different-id", "bs-a", "bs-b", "depends_on", "")

	svc.planBusinessServiceRelationshipEntry(context.Background(), doc, map[string]struct{}{}, &entry)
	if entry.Action != ApplyActionConflict {
		t.Errorf("Action: want Conflict, got %s", entry.Action)
	}
	if !strings.Contains(entry.Message, "triple already exists") {
		t.Errorf("expected triple-exists message; got %q", entry.Message)
	}
}

func TestApply_BusinessServiceRelationship_PlanInvalid_WhenSourceMissing(t *testing.T) {
	svc := newPlannerTestService([]string{"bs-b"}, newStubBSRRepo()) // bs-a not seeded
	entry := ApplyPlanEntry{Kind: types.KindBusinessServiceRelationship, ID: "rel-fk", DocumentIndex: 1}
	doc := makeBSRDoc("rel-fk", "bs-a", "bs-b", "depends_on", "")

	svc.planBusinessServiceRelationshipEntry(context.Background(), doc, map[string]struct{}{}, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid, got %s", entry.Action)
	}
	if len(entry.ValidationErrors) == 0 {
		t.Errorf("expected validation errors")
	}
}

func TestApply_BusinessServiceRelationship_PlanCreate_WhenSourceFromBundle(t *testing.T) {
	// bs-a not in store but is in bundle map → should resolve.
	svc := newPlannerTestService([]string{"bs-b"}, newStubBSRRepo())
	bundleBS := map[string]struct{}{"bs-a": {}}
	entry := ApplyPlanEntry{Kind: types.KindBusinessServiceRelationship, ID: "rel-bndl", DocumentIndex: 1}
	doc := makeBSRDoc("rel-bndl", "bs-a", "bs-b", "depends_on", "")

	svc.planBusinessServiceRelationshipEntry(context.Background(), doc, bundleBS, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create (bundle-resolved source), got %s; errs=%+v", entry.Action, entry.ValidationErrors)
	}
}

func TestApply_BusinessServiceRelationship_PlanCreate_WhenNoRepo(t *testing.T) {
	// No BSR repo configured (nil). The planner falls back to validator-decided Create.
	svc := &Service{
		businessServiceRepo: &stubBSExistsRepo{exists: map[string]bool{"bs-a": true, "bs-b": true}},
		// bsRelationshipRepo intentionally nil
	}
	entry := ApplyPlanEntry{Kind: types.KindBusinessServiceRelationship, ID: "rel-norepo", DocumentIndex: 1}
	doc := makeBSRDoc("rel-norepo", "bs-a", "bs-b", "depends_on", "")

	svc.planBusinessServiceRelationshipEntry(context.Background(), doc, map[string]struct{}{}, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create when no repo, got %s", entry.Action)
	}
	if entry.DecisionSource != DecisionSourceValidation {
		t.Errorf("DecisionSource: want validation, got %s", entry.DecisionSource)
	}
}
