package apply_test

// businessservice_integration_test.go — end-to-end tests for BusinessService
// document kind through the full apply pipeline.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store/memory"
)

// memBusinessServiceRepo is a minimal in-memory BusinessServiceRepository for tests.
type memBusinessServiceRepo struct {
	items map[string]*businessservice.BusinessService
}

func newMemBusinessServiceRepo() *memBusinessServiceRepo {
	return &memBusinessServiceRepo{items: make(map[string]*businessservice.BusinessService)}
}

func (r *memBusinessServiceRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.items[id]
	return ok, nil
}

func (r *memBusinessServiceRepo) GetByID(_ context.Context, id string) (*businessservice.BusinessService, error) {
	return r.items[id], nil
}

func (r *memBusinessServiceRepo) Create(_ context.Context, s *businessservice.BusinessService) error {
	r.items[s.ID] = s
	return nil
}

func validBusinessServiceDoc(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindBusinessService,
		ID:   id,
		Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindBusinessService,
			Metadata:   types.DocumentMetadata{ID: id, Name: "Consumer Lending"},
			Spec: types.BusinessServiceSpec{
				ServiceType: "customer_facing",
				Status:      "active",
				Description: "Retail lending products for consumers",
			},
		},
	}
}

// TestBusinessServiceApply_Create verifies that a valid BusinessService document
// is applied and persisted.
func TestBusinessServiceApply_Create(t *testing.T) {
	bsRepo := newMemBusinessServiceRepo()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: bsRepo,
	})

	ctx := context.Background()
	doc := validBusinessServiceDoc("bs-consumer-lending")

	result := svc.Apply(ctx, []parser.ParsedDocument{doc}, "test-actor")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Fatal("expected apply to succeed")
	}

	bs, err := bsRepo.GetByID(ctx, "bs-consumer-lending")
	if err != nil {
		t.Fatalf("GetByID: unexpected error: %v", err)
	}
	if bs == nil {
		t.Fatal("business service not persisted after apply")
	}
	if bs.Name != "Consumer Lending" {
		t.Errorf("expected name %q, got %q", "Consumer Lending", bs.Name)
	}
	if string(bs.ServiceType) != "customer_facing" {
		t.Errorf("expected service_type %q, got %q", "customer_facing", bs.ServiceType)
	}
	if bs.Origin != "manual" {
		t.Errorf("expected origin %q, got %q", "manual", bs.Origin)
	}
	if !bs.Managed {
		t.Error("expected managed=true for control-plane-applied business service")
	}
}

// TestBusinessServiceApply_Immutable_Conflict verifies that a second apply of
// the same business service ID returns a conflict.
func TestBusinessServiceApply_Immutable_Conflict(t *testing.T) {
	bsRepo := newMemBusinessServiceRepo()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: bsRepo,
	})

	ctx := context.Background()
	doc := validBusinessServiceDoc("bs-immutable")

	result1 := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")
	if result1.ValidationErrorCount() != 0 || result1.CreatedCount() != 1 {
		t.Fatalf("first apply: errors=%v created=%d", result1.ValidationErrors, result1.CreatedCount())
	}

	result2 := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")
	if result2.ValidationErrorCount() != 0 {
		t.Fatalf("second apply: unexpected errors: %v", result2.ValidationErrors)
	}
	if result2.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict, got %d", result2.ConflictCount())
	}
	if result2.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result2.CreatedCount())
	}
}

// TestBusinessServiceApply_NoRepo_ValidationOnly verifies that when no
// BusinessServiceRepository is configured the service reports created without
// persisting (validation-only mode).
func TestBusinessServiceApply_NoRepo_ValidationOnly(t *testing.T) {
	svc := apply.NewService()

	ctx := context.Background()
	doc := validBusinessServiceDoc("bs-no-repo")

	result := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %v", result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created (validation-only), got %d", result.CreatedCount())
	}
}

// TestBusinessServiceApply_WithCapabilityAndProcess verifies that a bundle
// containing BusinessService, Capability, and Process applies in the correct
// dependency order — BusinessService first, then Capability, then Process.
func TestBusinessServiceApply_WithCapabilityAndProcess(t *testing.T) {
	bsRepo := newMemBusinessServiceRepo()
	repos := memory.NewRepositories()

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: bsRepo,
		Capabilities:     repos.Capabilities,
		Processes:        repos.Processes,
	})

	ctx := context.Background()
	docs := []parser.ParsedDocument{
		validBusinessServiceDoc("bs-order-test"),
		{
			Kind: types.KindCapability,
			ID:   "cap-order-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-order-test", Name: "Order Test Cap"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-order-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-order-test", Name: "Order Test Process"},
				Spec:       types.ProcessSpec{CapabilityID: "cap-order-test", Status: "active"},
			},
		},
	}

	result := svc.Apply(ctx, docs, "test-actor")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	if bs, _ := bsRepo.GetByID(ctx, "bs-order-test"); bs == nil {
		t.Error("business service not persisted")
	}
	if c, _ := repos.Capabilities.GetByID(ctx, "cap-order-test"); c == nil {
		t.Error("capability not persisted")
	}
	if p, _ := repos.Processes.GetByID(ctx, "proc-order-test"); p == nil {
		t.Error("process not persisted")
	}
}
