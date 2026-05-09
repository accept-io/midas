package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
)

// businessservice_failmode_policy_test.go — round-trip regression for
// the optional BusinessService.FailModePolicyID column added in
// D27j-impl-2. Covers Create + GetByID, Update set/clear, and the
// COALESCE NULL→empty-string read path used by List/GetByID.

func TestBusinessServiceRepo_FailModePolicyID_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewBusinessServiceRepo(db)
	if err != nil {
		t.Fatalf("NewBusinessServiceRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	newSvc := func(id, fmpID string) *businessservice.BusinessService {
		return &businessservice.BusinessService{
			ID:               id,
			Name:             "FMP BS",
			ServiceType:      businessservice.ServiceTypeInternal,
			Status:           "active",
			Origin:           "manual",
			Managed:          true,
			CreatedAt:        now,
			UpdatedAt:        now,
			FailModePolicyID: fmpID,
		}
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id LIKE 'bs-fmp-rt-%'`)
	})

	t.Run("with fail_mode_policy_id round-trips correctly", func(t *testing.T) {
		svc := newSvc("bs-fmp-rt-001", "fmp-bs-default")
		if err := repo.Create(ctx, svc); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, svc.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got == nil {
			t.Fatal("expected service, got nil")
		}
		if got.FailModePolicyID != "fmp-bs-default" {
			t.Errorf("FailModePolicyID: want %q, got %q", "fmp-bs-default", got.FailModePolicyID)
		}
	})

	t.Run("empty fail_mode_policy_id round-trips as empty string", func(t *testing.T) {
		svc := newSvc("bs-fmp-rt-002", "")
		if err := repo.Create(ctx, svc); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, _ := repo.GetByID(ctx, svc.ID)
		if got == nil || got.FailModePolicyID != "" {
			t.Errorf("FailModePolicyID: want empty, got %+v", got)
		}
	})

	t.Run("Update can set and clear fail_mode_policy_id", func(t *testing.T) {
		svc := newSvc("bs-fmp-rt-003", "")
		if err := repo.Create(ctx, svc); err != nil {
			t.Fatalf("Create: %v", err)
		}

		svc.FailModePolicyID = "fmp-bs-update"
		if err := repo.Update(ctx, svc); err != nil {
			t.Fatalf("Update set: %v", err)
		}
		got, _ := repo.GetByID(ctx, svc.ID)
		if got == nil || got.FailModePolicyID != "fmp-bs-update" {
			t.Errorf("Update should persist FailModePolicyID; got %+v", got)
		}

		svc.FailModePolicyID = ""
		if err := repo.Update(ctx, svc); err != nil {
			t.Fatalf("Update clear: %v", err)
		}
		got, _ = repo.GetByID(ctx, svc.ID)
		if got == nil || got.FailModePolicyID != "" {
			t.Errorf("Update should clear FailModePolicyID; got %+v", got)
		}
	})
}
