package postgres

// bsr_apply_integration_test.go — end-to-end BusinessServiceRelationship
// apply against real Postgres (Epic 1, PR 1). Skipped automatically when
// DATABASE_URL is not set.
//
// This test pins the contract that two BusinessService documents and one
// BusinessServiceRelationship document can land via the apply path with
// the BSR row visible in business_service_relationships afterwards. Per
// the Step 0.5 inventory finding, BSR follows the BSC posture and emits
// no control-plane audit record — the test does NOT assert audit
// presence (mirroring the existing BSC apply behaviour).

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

func bsrApplyIntegrationBundle() []parser.ParsedDocument {
	return []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-bsr-apply-src",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-bsr-apply-src", Name: "BSR apply src"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindBusinessService,
			ID:   "bs-bsr-apply-tgt",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-bsr-apply-tgt", Name: "BSR apply tgt"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindBusinessServiceRelationship,
			ID:   "rel-bsr-apply-int",
			Doc: types.BusinessServiceRelationshipDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessServiceRelationship,
				Metadata:   types.DocumentMetadata{ID: "rel-bsr-apply-int"},
				Spec: types.BusinessServiceRelationshipSpec{
					SourceBusinessServiceID: "bs-bsr-apply-src",
					TargetBusinessServiceID: "bs-bsr-apply-tgt",
					RelationshipType:        "depends_on",
					Description:             "BSR apply integration round-trip",
				},
			},
		},
	}
}

func cleanupBSRApplyRows(t *testing.T, db interface {
	ExecContext(ctx context.Context, query string, args ...any) (any, error)
}) {
	// Reserved for future row-level cleanup if needed; the t.Cleanup
	// hook on the openTestDB call already deletes the rows we touch.
}

func TestApplyBusinessServiceRelationship_PostgresRoundTrips(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM business_service_relationships WHERE id = 'rel-bsr-apply-int'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM business_services WHERE business_service_id IN ('bs-bsr-apply-src','bs-bsr-apply-tgt')`)
		db.Close()
	})
	// Pre-clean in case a prior failed run left rows behind.
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM business_service_relationships WHERE id = 'rel-bsr-apply-int'`)
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM business_services WHERE business_service_id IN ('bs-bsr-apply-src','bs-bsr-apply-tgt')`)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:             repos.BusinessServices,
		BusinessServiceRelationships: repos.BusinessServiceRelationships,
		ControlAudit:                 repos.ControlAudit,
	})

	ctx := context.Background()
	result := svc.Apply(ctx, bsrApplyIntegrationBundle(), "operator:bsr-apply-test")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("validation errors: %+v", result.ValidationErrors)
	}
	if result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: %+v", result.Results)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("CreatedCount: want 3 (2 BS + 1 BSR), got %d (result=%+v)", result.CreatedCount(), result)
	}

	// Round-trip: the BSR row should now exist with matching fields.
	bsr, err := NewBusinessServiceRelationshipRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRelationshipRepo: %v", err)
	}
	got, err := bsr.GetByID(ctx, "rel-bsr-apply-int")
	if err != nil {
		t.Fatalf("GetByID after apply: %v", err)
	}
	if got.SourceBusinessService != "bs-bsr-apply-src" || got.TargetBusinessService != "bs-bsr-apply-tgt" {
		t.Errorf("BSR persisted with wrong refs: %+v", got)
	}
	if got.RelationshipType != "depends_on" {
		t.Errorf("BSR persisted with wrong type: %q", got.RelationshipType)
	}
	if got.Description != "BSR apply integration round-trip" {
		t.Errorf("BSR persisted with wrong description: %q", got.Description)
	}
	if got.CreatedBy != "operator:bsr-apply-test" {
		t.Errorf("BSR persisted with wrong created_by: %q", got.CreatedBy)
	}
}
