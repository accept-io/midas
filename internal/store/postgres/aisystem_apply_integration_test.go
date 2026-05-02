package postgres

// aisystem_apply_integration_test.go — end-to-end AI System Registration
// apply against real Postgres (Epic 1, PR 2). Skipped automatically when
// DATABASE_URL is not set.
//
// This test pins:
//  1. A bundle containing AISystem + AISystemVersion + AISystemBinding lands
//     correctly through the apply path with all three rows visible.
//  2. ControlPlane audit records for ai_system.created and
//     ai_system_version.created persist successfully — the schema CHECK
//     constraint extension (controlplane_audit_events_action_check
//     and _resource_kind_check) must include 'ai_system' /
//     'ai_system_version' on a fresh DB. This is the load-bearing assertion
//     for the idempotent migration block in schema.sql.
//  3. AISystemBinding does NOT emit a control-plane audit record (junction
//     posture mirroring BSR/BSC).

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

func aisystemApplyIntegrationBundle() []parser.ParsedDocument {
	v := 1
	return []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-aisys-apply",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-aisys-apply", Name: "AI System apply BS"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindAISystem,
			ID:   "ai-aisys-apply",
			Doc: types.AISystemDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindAISystem,
				Metadata:   types.DocumentMetadata{ID: "ai-aisys-apply", Name: "AI System apply"},
				Spec: types.AISystemSpec{
					Description: "AI System apply integration",
					Owner:       "lending-platform-team",
					Vendor:      "internal",
					SystemType:  "llm",
					Status:      "active",
					Origin:      "manual",
				},
			},
		},
		{
			Kind: types.KindAISystemVersion,
			ID:   "aiv-aisys-apply-v1",
			Doc: types.AISystemVersionDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindAISystemVersion,
				Metadata:   types.DocumentMetadata{ID: "aiv-aisys-apply-v1"},
				Spec: types.AISystemVersionSpec{
					AISystemID:           "ai-aisys-apply",
					Version:              1,
					ReleaseLabel:         "2026.04-r1",
					Status:               "active",
					EffectiveFrom:        "2026-04-15T00:00:00Z",
					ComplianceFrameworks: []string{"iso-42001"},
				},
			},
		},
		{
			Kind: types.KindAISystemBinding,
			ID:   "bind-aisys-apply",
			Doc: types.AISystemBindingDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindAISystemBinding,
				Metadata:   types.DocumentMetadata{ID: "bind-aisys-apply"},
				Spec: types.AISystemBindingSpec{
					AISystemID:        "ai-aisys-apply",
					AISystemVersion:   &v,
					BusinessServiceID: "bs-aisys-apply",
					Role:              "primary-evaluator",
					Description:       "AI System binding apply integration",
				},
			},
		},
	}
}

func TestApplyAISystem_PostgresRoundTrips(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() {
		// FK order: bindings → versions → systems; BS afterwards.
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_system_bindings WHERE id = 'bind-aisys-apply'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_system_versions WHERE ai_system_id = 'ai-aisys-apply'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM ai_systems WHERE id = 'ai-aisys-apply'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM business_services WHERE business_service_id = 'bs-aisys-apply'`)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM controlplane_audit_events WHERE resource_id IN ('ai-aisys-apply') OR (resource_kind = 'ai_system_version' AND resource_id = 'ai-aisys-apply')`)
		db.Close()
	})
	// Pre-clean to handle leftover state from a prior failed run.
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM ai_system_bindings WHERE id = 'bind-aisys-apply'`)
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM ai_system_versions WHERE ai_system_id = 'ai-aisys-apply'`)
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM ai_systems WHERE id = 'ai-aisys-apply'`)
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM business_services WHERE business_service_id = 'bs-aisys-apply'`)
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM controlplane_audit_events WHERE resource_id = 'ai-aisys-apply'`)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		AISystems:        repos.AISystems,
		AISystemVersions: repos.AISystemVersions,
		AISystemBindings: repos.AISystemBindings,
		ControlAudit:     repos.ControlAudit,
	})

	ctx := context.Background()
	result := svc.Apply(ctx, aisystemApplyIntegrationBundle(), "operator:aisys-apply-test")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("validation errors: %+v", result.ValidationErrors)
	}
	if result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: %+v", result.Results)
	}
	if result.CreatedCount() != 4 {
		t.Fatalf("CreatedCount: want 4 (BS + AISystem + AISystemVersion + AISystemBinding), got %d (result=%+v)", result.CreatedCount(), result)
	}

	// Round-trip: AISystem
	aiRepo, _ := NewAISystemRepo(db)
	gotSys, err := aiRepo.GetByID(ctx, "ai-aisys-apply")
	if err != nil {
		t.Fatalf("AISystem GetByID: %v", err)
	}
	if gotSys.SystemType != "llm" || gotSys.Status != "active" || gotSys.CreatedBy != "operator:aisys-apply-test" {
		t.Errorf("AISystem persisted incorrectly: %+v", gotSys)
	}

	// Round-trip: AISystemVersion
	verRepo, _ := NewAISystemVersionRepo(db)
	gotVer, err := verRepo.GetByIDAndVersion(ctx, "ai-aisys-apply", 1)
	if err != nil {
		t.Fatalf("AISystemVersion GetByIDAndVersion: %v", err)
	}
	if gotVer.Status != "active" {
		t.Errorf("AISystemVersion status not honoured: got %q (want 'active', not review-forced)", gotVer.Status)
	}
	if gotVer.ReleaseLabel != "2026.04-r1" || len(gotVer.ComplianceFrameworks) != 1 {
		t.Errorf("AISystemVersion fields persisted incorrectly: %+v", gotVer)
	}

	// Round-trip: AISystemBinding
	bRepo, _ := NewAISystemBindingRepo(db)
	gotBind, err := bRepo.GetByID(ctx, "bind-aisys-apply")
	if err != nil {
		t.Fatalf("AISystemBinding GetByID: %v", err)
	}
	if gotBind.AISystemVersion == nil || *gotBind.AISystemVersion != 1 {
		t.Errorf("AISystemBinding ai_system_version not preserved: %v", gotBind.AISystemVersion)
	}
	if gotBind.Role != "primary-evaluator" {
		t.Errorf("AISystemBinding role not preserved: %q", gotBind.Role)
	}

	// Audit emission: AISystem.created + AISystemVersion.created.
	// This is the LOAD-BEARING assertion for the schema CHECK migration:
	// if the controlplane_audit_events CHECK list does not include
	// 'ai_system' / 'ai_system_version', the Append calls return errors
	// (silently swallowed by ADR-041b best-effort), the rows do not
	// persist, and these assertions fail.
	auditList, err := repos.ControlAudit.List(ctx, controlaudit.ListFilter{
		ResourceID: "ai-aisys-apply",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("controlAudit.List: %v", err)
	}
	var sawSystemCreated, sawVersionCreated bool
	for _, rec := range auditList {
		switch rec.Action {
		case controlaudit.ActionAISystemCreated:
			sawSystemCreated = true
			if rec.ResourceKind != controlaudit.ResourceKindAISystem {
				t.Errorf("ai_system.created has wrong resource_kind: %q", rec.ResourceKind)
			}
		case controlaudit.ActionAISystemVersionCreated:
			sawVersionCreated = true
			if rec.ResourceKind != controlaudit.ResourceKindAISystemVersion {
				t.Errorf("ai_system_version.created has wrong resource_kind: %q", rec.ResourceKind)
			}
			if rec.ResourceVersion == nil || *rec.ResourceVersion != 1 {
				t.Errorf("ai_system_version.created has wrong resource_version: %v", rec.ResourceVersion)
			}
		}
	}
	if !sawSystemCreated {
		t.Errorf("ai_system.created audit record missing — schema CHECK extension may not have been applied; got %d records: %+v", len(auditList), auditList)
	}
	if !sawVersionCreated {
		t.Errorf("ai_system_version.created audit record missing — schema CHECK extension may not have been applied; got %d records: %+v", len(auditList), auditList)
	}

	// AISystemBinding must NOT emit audit (junction posture).
	bindingAudit, err := repos.ControlAudit.List(ctx, controlaudit.ListFilter{
		ResourceID: "bind-aisys-apply",
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("controlAudit.List(binding): %v", err)
	}
	if len(bindingAudit) != 0 {
		t.Errorf("AISystemBinding must not emit control-plane audit (junction posture); got %d records", len(bindingAudit))
	}
}
